package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	"github.com/jackc/pgx/v5"
)

// DefaultStuckSending: a dispatch "sending" for this long without a change
// is looked at by ReclaimStuck. A send itself takes seconds; channels that
// wait for a delivery report (SMS) keep their dispatch "sending" longer,
// and the reclaim then only asks their ledger whether the report was lost.
const DefaultStuckSending = 10 * time.Minute

// reclaimAction is what ReclaimStuck does with a dispatch left "sending",
// decided from the attempt row of its current channel.
type reclaimAction int

const (
	// reclaimRelease: no attempt row; the claim never committed one.
	// Back to pending (the next sweep sends it).
	reclaimRelease reclaimAction = iota
	// reclaimInterrupted: the attempt was claimed but nothing came back
	// (crash, restore of a backup taken mid-send). The channel's ledger
	// decides: a message it took stands in for the send result; otherwise
	// the dispatch goes back to pending and the same attempt row resumes.
	reclaimInterrupted
	// reclaimSettle: the attempt has a final result but the dispatch did
	// not move (a crash between the two writes before they shared a
	// transaction). The dispatch follows the attempt.
	reclaimSettle
	// reclaimAwait: accepted by the channel, waiting for its delivery
	// report. The ledger may hold the final state of a lost report;
	// otherwise the dispatch keeps waiting.
	reclaimAwait
)

// interrupted reports whether an attempt was claimed but nothing was
// recorded for it: no provider reference, no channel record, not sent.
func interrupted(dl *db.NotificationDelivery) bool {
	return dl != nil && dl.Status == DeliverySending && !dl.SentAt.Valid && !dl.FinishedAt.Valid &&
		dl.ProviderRef == "" && !dl.ExternalUuid.Valid
}

func decideReclaim(dl *db.NotificationDelivery) reclaimAction {
	switch {
	case dl == nil:
		return reclaimRelease
	case dl.FinishedAt.Valid:
		return reclaimSettle
	case interrupted(dl):
		return reclaimInterrupted
	case dl.Status == DeliverySending || dl.Status == DeliverySent:
		// Accepted (provider reference / channel record / sent_at),
		// waiting for the report.
		return reclaimAwait
	default:
		return reclaimSettle
	}
}

// finalOutcome reports whether a ledger result is a final delivery result
// (and whether it was delivered).
func finalOutcome(o channel.Outcome) (final, delivered bool) {
	switch o {
	case channel.OutcomeDelivered, channel.OutcomeSent:
		return true, true
	case channel.OutcomeFailed, channel.OutcomeSkipped:
		return true, false
	default:
		return false, false
	}
}

// ReclaimStuck moves dispatches left "sending" for olderThan (TEC-962): a
// worker crash or a restore of a backup taken while a dispatch was being
// sent leaves it "sending", and nothing else would ever move it.
//
//   - Nothing recorded for the attempt (no provider reference, no channel
//     record): back to pending, resumed on the same attempt row (at least
//     once for channels without a ledger: e-mail, web push).
//   - The channel's ledger holds a message for the attempt (SMS: the
//     sms_messages row of the delivery): recorded as sent, never sent
//     again; its final state settles the dispatch.
//   - A final attempt result the dispatch did not follow: settled.
//   - Waiting for a delivery report: kept, unless the ledger shows the
//     message is final (a lost report, a reconciled message).
//
// It returns the number of dispatches moved. The worker runs it at start
// and in every notification engine sweep.
func (e *Engine) ReclaimStuck(ctx context.Context, olderThan time.Duration) (int, error) {
	before := e.clock().Add(-olderThan)
	orgs, err := e.q.ListStuckNotificationDispatchOrganizations(ctx, ts(before))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, orgID := range orgs {
		m, err := e.reclaimOrganization(ctx, orgID, before)
		if err != nil {
			e.log.Warn("notify_reclaim_list_failed", "organization", orgID, "error", err)
		}
		n += m
	}
	if n > 0 {
		e.log.Info("notify_reclaimed", "dispatches", n)
	}
	return n, nil
}

// ReclaimStuckOrganization is ReclaimStuck for one organization.
func (e *Engine) ReclaimStuckOrganization(ctx context.Context, orgID int64, olderThan time.Duration) (int, error) {
	return e.reclaimOrganization(ctx, orgID, e.clock().Add(-olderThan))
}

func (e *Engine) reclaimOrganization(ctx context.Context, orgID int64, before time.Time) (int, error) {
	ids, err := e.q.ListStuckNotificationDispatches(ctx, db.ListStuckNotificationDispatchesParams{
		OrganizationID: orgID, BeforeAt: ts(before), LimitCount: e.batch,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		moved, err := e.reclaimOne(ctx, orgID, id, before)
		if err != nil {
			e.log.Warn("notify_reclaim_failed", "organization", orgID, "dispatch", id, "error", err)
			continue
		}
		if moved {
			n++
		}
	}
	return n, nil
}

func (e *Engine) reclaimOne(ctx context.Context, orgID, id int64, before time.Time) (bool, error) {
	var (
		moved bool
		after func() error
	)
	err := e.tx(ctx, func(q *db.Queries) error {
		d, err := q.LockNotificationDispatch(ctx, db.LockNotificationDispatchParams{OrganizationID: orgID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // another worker holds it, or gone
		}
		if err != nil {
			return err
		}
		if d.Status != DispatchSending || !d.UpdatedAt.Time.Before(before) {
			return nil
		}
		idx := d.NextChannelIndex
		if int(idx) >= len(d.Channels) {
			moved = true
			return q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
				OrganizationID: orgID, ID: d.ID, Status: DispatchFailed, NextChannelIndex: idx,
				LastError: "no channel left", CompletedAt: ts(e.clock()),
			})
		}
		ch := d.Channels[idx]
		var dl *db.NotificationDelivery
		row, err := q.GetNotificationDeliveryForChannel(ctx, db.GetNotificationDeliveryForChannelParams{
			OrganizationID: orgID, DispatchID: d.ID, Channel: ch,
		})
		switch {
		case err == nil:
			dl = &row
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		release := func() error {
			moved = true
			return q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
				OrganizationID: orgID, ID: d.ID, Status: DispatchPending, NextChannelIndex: idx, LastError: d.LastError,
			})
		}
		switch decideReclaim(dl) {
		case reclaimRelease:
			return release()
		case reclaimInterrupted:
			res, found, err := e.lookup(ctx, orgID, ch, dl.Uuid)
			if err != nil {
				return err
			}
			if !found {
				return release()
			}
			st := &step{dispatch: d, delivery: *dl, channel: ch}
			moved = true
			after = func() error {
				next, err := e.record(ctx, orgID, st, res)
				if err != nil || !next {
					return err
				}
				return e.Process(ctx, orgID, d.ID)
			}
			return nil
		case reclaimSettle:
			moved = true
			if dl.Status == DeliverySent || dl.Status == DeliveryDelivered {
				return q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
					OrganizationID: orgID, ID: d.ID, Status: DispatchDelivered, NextChannelIndex: idx,
					LastError: d.LastError, CompletedAt: dl.FinishedAt,
				})
			}
			reason := dl.Error
			if reason == "" {
				reason = "not delivered"
			}
			next, err := e.advance(ctx, q, orgID, d, ch, reason, false)
			if next {
				after = func() error { return e.Process(ctx, orgID, d.ID) }
			}
			return err
		default: // reclaimAwait
			res, found, err := e.lookup(ctx, orgID, ch, dl.Uuid)
			if err != nil {
				return err
			}
			if final, delivered := finalOutcome(res.Outcome); found && final {
				r := channel.Report{
					Channel: ch, DeliveryUUID: &dl.Uuid, Delivered: delivered, ProviderRef: res.ProviderRef, Error: res.Error,
				}
				if res.ExternalUUID != nil {
					r.ExternalUUID = *res.ExternalUUID
				}
				if !delivered && r.Error == "" {
					r.Error = "not delivered"
				}
				moved = true
				after = func() error {
					e.ReportDelivery(ctx, orgID, r)
					return nil
				}
				return nil
			}
			// Still waiting: to the back of the sweep.
			return q.TouchNotificationDispatch(ctx, db.TouchNotificationDispatchParams{OrganizationID: orgID, ID: d.ID})
		}
	})
	if err != nil {
		return false, err
	}
	if after != nil {
		if err := after(); err != nil {
			return moved, err
		}
	}
	return moved, nil
}
