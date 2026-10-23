// Package msgreconcile marks outbound messages that were still unsent in
// restored data but were sent after the backup (TEC-962,
// docs/operations/backup-restore.md RT-A32 (c), cmd/messaging-reconcile).
//
// SMS (Netgsm) and e-mail (SMTP) providers take no key that lets them drop
// a repeat, and the restored database cannot know what happened after the
// backup. Before the worker starts on restored data, the operator lists
// the messages that were waiting at backup time (created before it, due
// by now), compares them with the provider logs (Netgsm panel, mail
// server log) and marks the ones that went out. Marked messages are never
// sent:
//
//   - notification engine dispatches (pending / sending) → "reconciled";
//   - SMS messages still queued without a provider message id →
//     "reconciled" (their send job then finishes without calling the
//     provider; SMS with a provider id are checked by delivery report and
//     never resent);
//   - platform / organization notifications (queued / processing) →
//     "cancelled" with the reconcile note.
//
// Netgsm's report API looks messages up by its own jobid only, which a
// message that was queued at backup time does not have in the restored
// data; the decision is therefore the operator's.
package msgreconcile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kind is a kind of outbound message.
type Kind string

// Message kinds.
const (
	KindDispatch     Kind = "dispatch"
	KindSMS          Kind = "sms"
	KindNotification Kind = "notification"
)

// Kinds lists every kind in list order.
var Kinds = []Kind{KindDispatch, KindSMS, KindNotification}

// ParseKinds parses a comma separated kind list ("" = every kind).
func ParseKinds(v string) ([]Kind, error) {
	var out []Kind
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k := Kind(part)
		if k != KindDispatch && k != KindSMS && k != KindNotification {
			return nil, fmt.Errorf("unknown kind %q (dispatch, sms, notification)", part)
		}
		out = append(out, k)
	}
	return out, nil
}

// DefaultNote is the reason stored on marked messages.
const DefaultNote = "reconciled after restore: sent before the restore point, not sent again"

// Filter selects messages. List and Mark use the same filter, so Mark
// changes exactly what List showed (the worker is stopped meanwhile).
type Filter struct {
	// Before: messages created before this moment (the backup time).
	Before time.Time
	// DueUntil: only messages due by this moment (default: now). A
	// reminder scheduled after the restore cannot have been sent.
	DueUntil time.Time
	// OrganizationID limits to one organization (nil: all, platform
	// notifications included).
	OrganizationID *int64
	// Kinds limits the kinds (empty: all).
	Kinds []Kind
	// UUIDs limits Mark to these messages (empty: every match).
	UUIDs []uuid.UUID
	// Limit caps List per kind (default 1000).
	Limit int32
}

// Validate checks the filter and fills the defaults.
func (f *Filter) Validate(now time.Time) error {
	if f.Before.IsZero() {
		return errors.New("the backup time (--before) is required")
	}
	if f.Before.After(now) {
		return errors.New("the backup time is in the future")
	}
	if f.DueUntil.IsZero() {
		f.DueUntil = now
	}
	if f.Limit <= 0 {
		f.Limit = 1000
	}
	return nil
}

func (f Filter) has(k Kind) bool {
	if len(f.Kinds) == 0 {
		return true
	}
	for _, x := range f.Kinds {
		if x == k {
			return true
		}
	}
	return false
}

func (f Filter) org() pgtype.Int8 {
	if f.OrganizationID == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *f.OrganizationID, Valid: true}
}

func (f Filter) uuids() []uuid.UUID {
	if len(f.UUIDs) == 0 {
		return nil
	}
	return f.UUIDs
}

func tsOf(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

// Candidate is one message that was waiting at backup time.
type Candidate struct {
	Kind           Kind
	UUID           uuid.UUID
	OrganizationID *int64
	Organization   string
	// What: the event (dispatch), purpose (SMS) or template / source event
	// (notification).
	What string
	// Channel: the dispatch's current channel, "sms", or the notification
	// channel.
	Channel string
	// Recipient: masked (SMS number, e-mail); empty for dispatches (the
	// recipient is read at send time).
	Recipient string
	Status    string
	CreatedAt time.Time
	DueAt     *time.Time
}

// Result counts marked messages per kind.
type Result struct {
	Dispatches    int
	SMS           int
	Notifications int
}

// Total is the number of marked messages.
func (r Result) Total() int { return r.Dispatches + r.SMS + r.Notifications }

// Reconciler lists and marks messages.
type Reconciler struct {
	pool *pgxpool.Pool
	q    *db.Queries
	now  func() time.Time
}

// New builds a reconciler.
func New(pool *pgxpool.Pool, q *db.Queries) *Reconciler {
	return &Reconciler{pool: pool, q: q, now: time.Now}
}

// OrganizationBySlug resolves an organization slug.
func (r *Reconciler) OrganizationBySlug(ctx context.Context, slug string) (int64, error) {
	id, err := r.q.GetReconcileOrganizationBySlug(ctx, strings.TrimSpace(slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("unknown organization %q", slug)
	}
	return id, err
}

// List returns the messages the filter selects, oldest first per kind.
func (r *Reconciler) List(ctx context.Context, f Filter) ([]Candidate, error) {
	if err := f.Validate(r.now()); err != nil {
		return nil, err
	}
	var out []Candidate
	if f.has(KindDispatch) {
		rows, err := r.q.ListReconcileDispatches(ctx, db.ListReconcileDispatchesParams{
			BeforeAt: tsOf(f.Before), DueUntil: tsOf(f.DueUntil), OrganizationID: f.org(), LimitCount: f.Limit,
		})
		if err != nil {
			return nil, fmt.Errorf("list dispatches: %w", err)
		}
		for _, d := range rows {
			org := d.OrganizationID
			c := Candidate{
				Kind: KindDispatch, UUID: d.Uuid, OrganizationID: &org, Organization: d.OrganizationSlug,
				What: d.EventCode, Status: d.Status, CreatedAt: d.CreatedAt.Time,
			}
			if int(d.NextChannelIndex) < len(d.Channels) {
				c.Channel = d.Channels[d.NextChannelIndex]
			}
			if d.Test {
				c.What += " (test)"
			}
			due := d.ScheduledAt.Time
			c.DueAt = &due
			out = append(out, c)
		}
	}
	if f.has(KindSMS) {
		rows, err := r.q.ListReconcileSMSMessages(ctx, db.ListReconcileSMSMessagesParams{
			BeforeAt: tsOf(f.Before), OrganizationID: f.org(), LimitCount: f.Limit,
		})
		if err != nil {
			return nil, fmt.Errorf("list sms: %w", err)
		}
		for _, m := range rows {
			org := m.OrganizationID
			out = append(out, Candidate{
				Kind: KindSMS, UUID: m.Uuid, OrganizationID: &org, Organization: m.OrganizationSlug,
				What: m.Purpose + " (" + m.Class + ")", Channel: "sms:" + m.Provider, Recipient: m.PhoneMasked,
				Status: m.Status, CreatedAt: m.CreatedAt.Time,
			})
		}
	}
	if f.has(KindNotification) {
		rows, err := r.q.ListReconcileNotifications(ctx, db.ListReconcileNotificationsParams{
			BeforeAt: tsOf(f.Before), DueUntil: tsOf(f.DueUntil), OrganizationID: f.org(), LimitCount: f.Limit,
		})
		if err != nil {
			return nil, fmt.Errorf("list notifications: %w", err)
		}
		for _, n := range rows {
			c := Candidate{
				Kind: KindNotification, UUID: n.Uuid, Channel: n.Channel, Recipient: MaskRecipient(n.Recipient),
				Status: n.Status, CreatedAt: n.CreatedAt.Time, What: n.TemplateCode,
			}
			if c.What == "" {
				c.What = n.SourceEvent
			}
			if n.OrganizationID.Valid {
				org := n.OrganizationID.Int64
				c.OrganizationID = &org
			} else {
				c.Organization = "(platform)"
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// Mark marks every message the filter selects (restricted to f.UUIDs when
// set) in one transaction.
func (r *Reconciler) Mark(ctx context.Context, f Filter, note string) (Result, error) {
	if err := f.Validate(r.now()); err != nil {
		return Result{}, err
	}
	note = strings.TrimSpace(note)
	if note == "" {
		note = DefaultNote
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	var res Result
	if f.has(KindDispatch) {
		rows, err := q.ReconcileDispatches(ctx, db.ReconcileDispatchesParams{
			Note: note, BeforeAt: tsOf(f.Before), DueUntil: tsOf(f.DueUntil), OrganizationID: f.org(), Uuids: f.uuids(),
		})
		if err != nil {
			return Result{}, fmt.Errorf("mark dispatches: %w", err)
		}
		res.Dispatches = len(rows)
	}
	if f.has(KindSMS) {
		rows, err := q.ReconcileSMSMessages(ctx, db.ReconcileSMSMessagesParams{
			Note: note, BeforeAt: tsOf(f.Before), OrganizationID: f.org(), Uuids: f.uuids(),
		})
		if err != nil {
			return Result{}, fmt.Errorf("mark sms: %w", err)
		}
		res.SMS = len(rows)
	}
	if f.has(KindNotification) {
		rows, err := q.ReconcileNotifications(ctx, db.ReconcileNotificationsParams{
			Note: note, BeforeAt: tsOf(f.Before), DueUntil: tsOf(f.DueUntil), OrganizationID: f.org(), Uuids: f.uuids(),
		})
		if err != nil {
			return Result{}, fmt.Errorf("mark notifications: %w", err)
		}
		for _, n := range rows {
			if _, err := q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
				NotificationID: n.ID, Event: "reconciled", Metadata: []byte(`{"source":"messaging-reconcile"}`),
			}); err != nil {
				return Result{}, fmt.Errorf("notification history: %w", err)
			}
		}
		res.Notifications = len(rows)
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return res, nil
}

// MaskRecipient masks an e-mail address (first letter + domain) or keeps
// the last two digits of anything else.
func MaskRecipient(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if at := strings.LastIndex(v, "@"); at > 0 {
		return v[:1] + "***" + v[at:]
	}
	r := []rune(v)
	if len(r) <= 2 {
		return "***"
	}
	return "***" + string(r[len(r)-2:])
}

// ParseTime reads a moment as RFC 3339 ("2026-10-20T03:15:00Z") or in the
// backup set name format ("20261020T031500Z", always UTC).
func ParseTime(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "20060102T150405Z"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q (RFC 3339, e.g. 2026-10-20T03:15:00Z, or a set timestamp 20261020T031500Z)", v)
}
