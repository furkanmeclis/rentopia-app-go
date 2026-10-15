package usecase

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// core loads the lifecycle facts of a reservation in the caller's branch
// scope (another organization's or an out-of-scope uuid is ErrNotFound).
func (s *Service) core(ctx context.Context, orgID int64, id uuid.UUID) (db.GetReservationCoreRow, error) {
	row, err := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if !inScope(ctx, row.PickupBranchID, row.ReturnBranchID) {
		return row, ErrNotFound
	}
	return row, nil
}

func allocationActive(c db.GetReservationCoreRow) bool {
	return c.AllocationStatus.Valid && c.AllocationStatus.String == availmodel.StatusActive
}

func requireReason(reason string) (string, error) {
	r := strings.TrimSpace(reason)
	if len(r) < 3 || len(r) > 500 {
		return "", validation("reason", "required", "a reason is required (3-500 characters)")
	}
	return r, nil
}

// where rebuilds the engine request of a reservation as it is.
func (s *Service) where(ctx context.Context, orgID int64, id uuid.UUID) (availmodel.Where, db.GetReservationViewRow, error) {
	v, err := s.q.GetReservationView(ctx, db.GetReservationViewParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return availmodel.Where{}, v, err
	}
	pb, rb := v.PickupBranchUuid, v.ReturnBranchUuid
	w := availmodel.Where{
		PickupBranchUUID: &pb, ReturnBranchUUID: &rb, StartsAt: v.StartsAt.Time.UTC(), EndsAt: v.EndsAt.Time.UTC(), Channel: v.Channel,
	}
	if v.PickupPointUuid.Valid {
		p := uuid.UUID(v.PickupPointUuid.Bytes)
		w.PickupPointUUID = &p
	}
	if v.ReturnPointUuid.Valid {
		p := uuid.UUID(v.ReturnPointUuid.Bytes)
		w.ReturnPointUUID = &p
	}
	return w, v, nil
}

// Submit places a request on the approval flow: capacity is reserved now
// (option or firm, by the settings) and the status follows waitingStatus.
func (s *Service) Submit(ctx context.Context, orgID int64, id uuid.UUID) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	if !model.CanAct(c.Status, model.ActionSubmit) {
		return model.Reservation{}, stateErr("invalid_transition", "only a request can be submitted")
	}
	settings, err := s.settingsOf(ctx, orgID)
	if err != nil {
		return model.Reservation{}, err
	}
	status, review, pay := s.waitingStatus(settings.Settings, money.FromNumericOrZero(c.PrepaymentAmount))
	w, _, err := s.where(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	kind := availmodel.KindHold
	holdUntil := review
	switch status {
	case model.StatusConfirmed:
		kind, holdUntil = availmodel.KindReservation, nil
	case model.StatusAwaitingPayment:
		holdUntil = pay
	}
	ref := c.Uuid
	_, err = s.avail.Reserve(ctx, orgID, availmodel.ReserveInput{
		Where: w, ProductUUID: c.ProductUuid, Kind: kind, HoldUntil: holdUntil, RefType: model.RefType, RefUUID: &ref,
		ActorID: actorID(ctx), Note: "reservation",
		Hook: hookOf(func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
			if err := s.apply(ctx, q, orgID, transition{
				id: c.ID, from: c.Status, to: status, action: model.ActionSubmit, reviewDeadline: review, payDeadline: pay,
				allocationID: r.AllocationID,
			}); err != nil {
				return err
			}
			if err := assignment(ctx, q, orgID, c.ID, r.VehicleID, r.ProductID, r.ProductVersion, r.PlateFixed, model.SourceEngine, ""); err != nil {
				return err
			}
			_, err := s.planEffects(ctx, q, orgID, r, "reservation", c.Uuid)
			return err
		}, s),
	})
	if err != nil {
		return model.Reservation{}, mapEngineErr(err)
	}
	return s.after(ctx, orgID, id)
}

// Approve is the staff review: awaiting_review → awaiting_payment when a
// prepayment is still due (the option now lasts the payment window), else
// confirmed (the option becomes a firm commitment).
func (s *Service) Approve(ctx context.Context, orgID int64, id uuid.UUID) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	if !model.CanAct(c.Status, model.ActionApprove) {
		return model.Reservation{}, stateErr("invalid_transition", "only a reservation awaiting review can be approved")
	}
	due := money.FromNumericOrZero(c.PrepaymentAmount).Sub(money.FromNumericOrZero(c.PaidAmount))
	if !due.IsPositive() {
		return s.confirm(ctx, orgID, c, model.ActionApprove)
	}
	settings, err := s.settingsOf(ctx, orgID)
	if err != nil {
		return model.Reservation{}, err
	}
	now := s.clock()
	deadline := now.Add(time.Duration(minutesOr(settings.Settings.Reservation.PaymentHoldMinutes, model.DefaultPaymentMinutes)) * time.Minute)
	err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		n, err := q.SetHoldExpiry(ctx, db.SetHoldExpiryParams{
			OrganizationID: orgID, ID: c.AllocationID.Int64, ExpiresAt: ts(deadline), NowAt: ts(now),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return availusecase.ErrHoldExpired
		}
		return s.apply(ctx, q, orgID, transition{
			id: c.ID, from: c.Status, to: model.StatusAwaitingPayment, action: model.ActionApprove, payDeadline: &deadline,
		})
	})
	if errors.Is(err, availusecase.ErrHoldExpired) {
		s.expireOne(ctx, orgID, c.Uuid)
		return model.Reservation{}, mapEngineErr(err)
	}
	if err != nil {
		return model.Reservation{}, err
	}
	return s.after(ctx, orgID, id)
}

// Confirm moves a waiting reservation to confirmed: the option (hold)
// becomes a firm commitment on the same allocation (capacity is never
// consumed twice). A lapsed option expires the reservation instead.
func (s *Service) Confirm(ctx context.Context, orgID int64, id uuid.UUID) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	if !model.CanAct(c.Status, model.ActionConfirm) {
		return model.Reservation{}, stateErr("invalid_transition", "the reservation cannot be confirmed from "+c.Status)
	}
	return s.confirm(ctx, orgID, c, model.ActionConfirm)
}

func (s *Service) confirm(ctx context.Context, orgID int64, c db.GetReservationCoreRow, action string) (model.Reservation, error) {
	if !allocationActive(c) {
		s.expireOne(ctx, orgID, c.Uuid)
		return model.Reservation{}, stateErr("hold_expired", "the option (hold) of the reservation has expired")
	}
	tr := transition{id: c.ID, from: c.Status, to: model.StatusConfirmed, action: action}
	var err error
	if c.AllocationKind.String == availmodel.KindHold {
		ref := c.Uuid
		_, err = s.avail.ConfirmWith(unscoped(ctx), orgID, c.AllocationUuid.Bytes, model.RefType, &ref,
			hookOf(func(ctx context.Context, q *db.Queries, _ availmodel.HookResult) error {
				return s.apply(ctx, q, orgID, tr)
			}, s))
	} else {
		err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error { return s.apply(ctx, q, orgID, tr) })
	}
	if errors.Is(err, availusecase.ErrHoldExpired) {
		s.expireOne(ctx, orgID, c.Uuid)
	}
	if err != nil {
		return model.Reservation{}, mapEngineErr(err)
	}
	return s.after(ctx, orgID, c.Uuid)
}

// Cancel cancels a reservation with a reason; its capacity is released
// exactly once (the engine re-plans what depended on it; what it cannot
// keep valid opens resolution tasks).
func (s *Service) Cancel(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	if !model.CanAct(c.Status, model.ActionCancel) {
		return model.Reservation{}, stateErr("invalid_transition", "the reservation cannot be cancelled from "+c.Status)
	}
	r, err := requireReason(reason)
	if err != nil {
		return model.Reservation{}, err
	}
	if err := s.terminate(ctx, orgID, c, model.ActionCancel, model.StatusCancelled, r); err != nil {
		return model.Reservation{}, err
	}
	return s.after(ctx, orgID, id)
}

// NoShow marks a confirmed reservation whose pick-up time has passed as a
// no-show (capacity released).
func (s *Service) NoShow(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	if !model.CanAct(c.Status, model.ActionNoShow) {
		return model.Reservation{}, stateErr("invalid_transition", "only a confirmed reservation can be a no-show")
	}
	if s.clock().Before(c.StartsAt.Time) {
		return model.Reservation{}, stateErr("not_started", "the pick-up time has not come yet")
	}
	r, err := requireReason(reason)
	if err != nil {
		return model.Reservation{}, err
	}
	if err := s.terminate(ctx, orgID, c, model.ActionNoShow, model.StatusNoShow, r); err != nil {
		return model.Reservation{}, err
	}
	return s.after(ctx, orgID, id)
}

// terminate moves to a terminal state and frees the capacity once.
func (s *Service) terminate(ctx context.Context, orgID int64, c db.GetReservationCoreRow, action, to, reason string) error {
	tr := transition{id: c.ID, from: c.Status, to: to, action: action, reason: reason}
	var touched []uuid.UUID
	plain := func() error {
		return s.tx(ctx, func(q *db.Queries, _ pgxTx) error { return s.apply(ctx, q, orgID, tr) })
	}
	var err error
	if allocationActive(c) {
		_, err = s.avail.ReleaseWith(unscoped(ctx), orgID, c.AllocationUuid.Bytes, "reservation_"+action, actorID(ctx),
			hookOf(func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
				if err := s.apply(ctx, q, orgID, tr); err != nil {
					return err
				}
				t, err := s.planEffects(ctx, q, orgID, r, "reservation", c.Uuid)
				touched = t
				return err
			}, s))
		if errors.Is(err, availusecase.ErrNotActive) {
			// The option lapsed in between (the engine expired it, exactly
			// once): only the reservation moves.
			err = plain()
		}
	} else {
		err = plain()
	}
	if err != nil {
		return mapEngineErr(err)
	}
	s.releaseCoupon(ctx, orgID, c.Uuid, action)
	s.publish(ctx, model.ResourceReservations, "updated", touched...)
	return nil
}

// expireOne expires a waiting reservation (system; errors are logged).
func (s *Service) expireOne(ctx context.Context, orgID int64, id uuid.UUID) bool {
	c, err := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: id})
	if err != nil || !model.CanAct(c.Status, model.ActionExpire) {
		return false
	}
	if err := s.terminate(unscoped(ctx), orgID, c, model.ActionExpire, model.StatusExpired, "window_elapsed"); err != nil {
		if !errors.Is(err, ErrConcurrent) {
			slog.Default().Warn("reservation_expire_failed", "reservation", id, "error", err)
		}
		return false
	}
	s.publish(ctx, model.ResourceReservations, "updated", id)
	return true
}

// ExpireDue is the minute sweep (worker, app:availability:expire-holds):
// every waiting reservation whose review / payment window elapsed expires
// and releases its option exactly once. Returns the number expired.
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	now := s.clock()
	orgs, err := s.q.ListDueReservationOrganizations(ctx, ts(now))
	if err != nil {
		return 0, err
	}
	total := 0
	for _, orgID := range orgs {
		octx := orgctx.WithOrganizationID(ctx, orgID)
		due, err := s.q.ListDueReservations(octx, db.ListDueReservationsParams{OrganizationID: orgID, NowAt: ts(now)})
		if err != nil {
			return total, err
		}
		for _, id := range due {
			if s.expireOne(octx, orgID, id) {
				total++
			}
		}
	}
	return total, nil
}

// RecordPayment implements model.PaymentRecorder (M3 payments): paid is the
// total collected so far; refunded marks a refund. In auto approval mode a
// reservation awaiting its prepayment is confirmed once it is covered.
func (s *Service) RecordPayment(ctx context.Context, orgID int64, id uuid.UUID, paid money.Money, refunded bool) error {
	c, err := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if paid.IsNegative() {
		return validation("paid", "invalid", "paid must not be negative")
	}
	total := money.FromNumericOrZero(c.TotalAmount)
	status := model.PaymentUnpaid
	switch {
	case refunded:
		status = model.PaymentRefunded
	case paid.Cmp(total) >= 0 && paid.IsPositive():
		status = model.PaymentPaid
	case paid.IsPositive():
		status = model.PaymentPartially
	}
	err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		if _, err := q.SetReservationPayment(ctx, db.SetReservationPaymentParams{
			OrganizationID: orgID, ID: c.ID, PaymentStatus: status, PaidAmount: paid.Numeric(),
		}); err != nil {
			return err
		}
		return event(ctx, q, orgID, c.ID, EventPaymentRecorded, "", "", "", map[string]any{"paid": paid.String(), "payment_status": status})
	})
	if err != nil {
		return err
	}
	if c.Status == model.StatusAwaitingPayment && paid.Cmp(money.FromNumericOrZero(c.PrepaymentAmount)) >= 0 {
		settings, err := s.settingsOf(ctx, orgID)
		if err != nil {
			return err
		}
		if settings.Settings.Reservation.ApprovalMode != osmodel.ApprovalStaff {
			c.PaidAmount = paid.Numeric()
			if _, err := s.confirm(unscoped(ctx), orgID, c, model.ActionConfirm); err != nil {
				return err
			}
		}
	}
	s.publish(ctx, model.ResourceReservations, "updated", id)
	return nil
}

// after publishes the change and returns the fresh reservation.
func (s *Service) after(ctx context.Context, orgID int64, id uuid.UUID) (model.Reservation, error) {
	s.publish(ctx, model.ResourceReservations, "updated", id)
	return s.Get(ctx, orgID, id)
}
