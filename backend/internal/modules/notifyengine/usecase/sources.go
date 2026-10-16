package usecase

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Reservation statuses / actions the engine reacts to (reservations
// state machine).
const (
	reservationConfirmed       = "confirmed"
	reservationAwaitingPayment = "awaiting_payment"
	reservationCancelled       = "cancelled"
	reservationExpired         = "expired"
	actionReject               = "reject"
)

// ReservationChanged implements the reservations' notifier port: called
// inside the transition's transaction (q), so the notification commits
// with the status change and never exists for a rolled-back transition.
func (e *Engine) ReservationChanged(ctx context.Context, q *db.Queries, orgID, reservationID int64, action, from, to string) error {
	var code string
	switch {
	case to == reservationConfirmed:
		code = catalog.ReservationConfirmed
	case to == reservationAwaitingPayment:
		code = catalog.ReservationPaymentPending
	case to == reservationCancelled && action == actionReject:
		code = catalog.ReservationRejected
	case to == reservationCancelled, to == reservationExpired && from == reservationAwaitingPayment:
		code = catalog.ReservationCancelled
	default:
		return nil
	}
	r, err := q.GetNotifyReservation(ctx, db.GetNotifyReservationParams{OrganizationID: orgID, ID: reservationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	vars := map[string]string{
		"reference": r.Reference, "pickup_at": FormatTime(r.StartsAt.Time), "pickup_branch": r.PickupBranchName,
		"return_at": FormatTime(r.EndsAt.Time), "return_branch": r.ReturnBranchName,
	}
	if r.PaymentDeadline.Valid {
		vars["payment_deadline"] = FormatTime(r.PaymentDeadline.Time)
	}
	id := r.Uuid
	_, err = e.EmitTx(ctx, q, orgID, EmitInput{
		Event: code, CustomerID: r.CustomerID, RefType: "reservation", RefUUID: &id, Key: "status:" + to, Vars: vars,
	})
	return err
}

// rules caches the effective rules of one organization during a sweep.
type rules struct {
	e     *Engine
	orgID int64
	m     map[string]Rule
}

func (r *rules) get(ctx context.Context, code string) Rule {
	if v, ok := r.m[code]; ok {
		return v
	}
	v, err := r.e.RuleFor(ctx, r.e.q, r.orgID, code)
	if err != nil {
		v = Rule{Event: code}
	}
	r.m[code] = v
	return v
}

// offsetDue picks the reminder to send for a deadline: the smallest offset
// whose moment has passed (a later reminder supersedes the earlier ones
// that were missed, so a customer never gets two at once). ok is false
// when no reminder is due yet.
func offsetDue(offsets []int32, at, now time.Time) (int32, bool) {
	best, ok := int32(0), false
	for _, o := range offsets {
		if o < 0 || o > catalog.MaxOffsetMinutes {
			continue
		}
		if !now.Before(at.Add(-time.Duration(o) * time.Minute)) {
			if !ok || o < best {
				best, ok = o, true
			}
		}
	}
	return best, ok
}

func maxOffset(offsets []int32) time.Duration {
	m := int32(0)
	for _, o := range offsets {
		if o > m && o <= catalog.MaxOffsetMinutes {
			m = o
		}
	}
	return time.Duration(m) * time.Minute
}

// sweepWindow is how far back sweep-detected facts are picked up (a worker
// that was down for a while still sends them, older ones are dropped).
const sweepWindow = 24 * time.Hour

// Reminders creates the dispatches of reminder events and of facts the
// sweep detects (overdue rentals, issued invoices, field tasks) for every
// active organization. Idempotency keys make every run safe to repeat.
func (e *Engine) Reminders(ctx context.Context) error {
	orgs, err := e.q.ListNotificationOrganizations(ctx)
	if err != nil {
		return err
	}
	for _, orgID := range orgs {
		e.RemindOrganization(ctx, orgID)
	}
	return nil
}

// RemindOrganization runs the reminder sources of one organization.
func (e *Engine) RemindOrganization(ctx context.Context, orgID int64) {
	e.remindOrg(ctx, orgID, &rules{e: e, orgID: orgID, m: map[string]Rule{}})
}

func (e *Engine) emitSoft(ctx context.Context, orgID int64, in EmitInput) {
	if _, err := e.EmitTx(ctx, e.q, orgID, in); err != nil {
		e.log.Warn("notify_emit_failed", "organization", orgID, "event", in.Event, "error", err)
	}
}

func (e *Engine) remindOrg(ctx context.Context, orgID int64, r *rules) {
	now := e.clock()
	// Pickup approaching.
	if rule := r.get(ctx, catalog.RentalPickupUpcoming); rule.Enabled && len(rule.Offsets) > 0 {
		rows, err := e.q.ListNotifyPickupCandidates(ctx, db.ListNotifyPickupCandidatesParams{
			OrganizationID: orgID, NowAt: ts(now), UntilAt: ts(now.Add(maxOffset(rule.Offsets))),
		})
		e.warn(err, orgID, catalog.RentalPickupUpcoming)
		for _, c := range rows {
			if o, ok := offsetDue(rule.Offsets, c.At.Time, now); ok {
				id := c.Uuid
				e.emitSoft(ctx, orgID, EmitInput{
					Event: catalog.RentalPickupUpcoming, CustomerID: c.CustomerID, RefType: "reservation", RefUUID: &id,
					Key: offsetKey(o, c.At.Time), Vars: map[string]string{
						"reference": c.Reference, "pickup_at": FormatTime(c.At.Time), "pickup_branch": c.BranchName,
					},
				})
			}
		}
	}
	// Return approaching.
	if rule := r.get(ctx, catalog.RentalReturnUpcoming); rule.Enabled && len(rule.Offsets) > 0 {
		rows, err := e.q.ListNotifyReturnCandidates(ctx, db.ListNotifyReturnCandidatesParams{
			OrganizationID: orgID, NowAt: ts(now), UntilAt: ts(now.Add(maxOffset(rule.Offsets))),
		})
		e.warn(err, orgID, catalog.RentalReturnUpcoming)
		for _, c := range rows {
			if o, ok := offsetDue(rule.Offsets, c.At.Time, now); ok {
				id := c.Uuid
				e.emitSoft(ctx, orgID, EmitInput{
					Event: catalog.RentalReturnUpcoming, CustomerID: c.CustomerID, RefType: "rental", RefUUID: &id,
					Key: offsetKey(o, c.At.Time), Vars: map[string]string{
						"reference": c.Reference, "return_at": FormatTime(c.At.Time), "return_branch": c.BranchName,
					},
				})
			}
		}
	}
	// Payment deadline approaching.
	if rule := r.get(ctx, catalog.ReservationPaymentPending); rule.Enabled && len(rule.Offsets) > 0 {
		rows, err := e.q.ListNotifyPaymentCandidates(ctx, db.ListNotifyPaymentCandidatesParams{
			OrganizationID: orgID, NowAt: ts(now), UntilAt: ts(now.Add(maxOffset(rule.Offsets))),
		})
		e.warn(err, orgID, catalog.ReservationPaymentPending)
		for _, c := range rows {
			if o, ok := offsetDue(rule.Offsets, c.At.Time, now); ok {
				id := c.Uuid
				e.emitSoft(ctx, orgID, EmitInput{
					Event: catalog.ReservationPaymentPending, CustomerID: c.CustomerID, RefType: "reservation", RefUUID: &id,
					Key: offsetKey(o, c.At.Time), Vars: map[string]string{
						"reference": c.Reference, "payment_deadline": FormatTime(c.At.Time),
					},
				})
			}
		}
	}
	// Overdue rentals.
	if rule := r.get(ctx, catalog.RentalOverdue); rule.Enabled {
		rows, err := e.q.ListNotifyOverdueRentals(ctx, db.ListNotifyOverdueRentalsParams{OrganizationID: orgID, SinceAt: ts(now.Add(-sweepWindow))})
		e.warn(err, orgID, catalog.RentalOverdue)
		for _, c := range rows {
			id := c.Uuid
			e.emitSoft(ctx, orgID, EmitInput{
				Event: catalog.RentalOverdue, CustomerID: c.CustomerID, RefType: "rental", RefUUID: &id, Key: "overdue",
				Vars: map[string]string{"reference": c.Reference, "return_branch": c.BranchName},
			})
		}
	}
	// Issued invoices.
	if rule := r.get(ctx, catalog.InvoiceIssued); rule.Enabled {
		rows, err := e.q.ListNotifyIssuedInvoices(ctx, db.ListNotifyIssuedInvoicesParams{OrganizationID: orgID, SinceAt: ts(now.Add(-sweepWindow))})
		e.warn(err, orgID, catalog.InvoiceIssued)
		for _, c := range rows {
			id := c.Uuid
			e.emitSoft(ctx, orgID, EmitInput{
				Event: catalog.InvoiceIssued, CustomerID: c.CustomerID, RefType: "invoice", RefUUID: &id, Key: "issued",
				Vars: map[string]string{"invoice_number": c.Reference},
			})
		}
	}
	// Account payments falling due (due date at 09:00 Istanbul).
	if rule := r.get(ctx, catalog.AccountPaymentDue); rule.Enabled && len(rule.Offsets) > 0 {
		today := rentaltime.Local(now)
		from := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
		to := from.Add(maxOffset(rule.Offsets) + 24*time.Hour)
		rows, err := e.q.ListNotifyAccountDue(ctx, db.ListNotifyAccountDueParams{
			OrganizationID: orgID, FromDate: pgtype.Date{Time: from, Valid: true}, ToDate: pgtype.Date{Time: to, Valid: true},
		})
		e.warn(err, orgID, catalog.AccountPaymentDue)
		for _, c := range rows {
			d := c.DueDate.Time
			at := time.Date(d.Year(), d.Month(), d.Day(), 9, 0, 0, 0, rentaltime.Location())
			if o, ok := offsetDue(rule.Offsets, at, now); ok {
				id := c.Uuid
				e.emitSoft(ctx, orgID, EmitInput{
					Event: catalog.AccountPaymentDue, CustomerID: c.CustomerID, RefType: "account", RefUUID: &id,
					Key: "due:" + d.Format("2006-01-02") + ":" + strconv.Itoa(int(o)), Vars: map[string]string{"due_date": FormatDate(d)},
				})
			}
		}
	}
	// Field tasks: new assignments and overdue tasks (staff).
	if rule := r.get(ctx, catalog.FieldTaskAssigned); rule.Enabled {
		rows, err := e.q.ListNotifyAssignedTasks(ctx, db.ListNotifyAssignedTasksParams{OrganizationID: orgID, SinceAt: ts(now.Add(-sweepWindow))})
		e.warn(err, orgID, catalog.FieldTaskAssigned)
		for _, c := range rows {
			e.emitTask(ctx, orgID, catalog.FieldTaskAssigned, c.Uuid, c.AssigneeID, c.TaskType, c.DueAt.Time, c.BranchName,
				"assigned:"+strconv.FormatInt(c.AssigneeID, 10)+":"+strconv.FormatInt(c.AssignedAt.Time.Unix(), 10))
		}
	}
	if rule := r.get(ctx, catalog.FieldTaskOverdue); rule.Enabled {
		rows, err := e.q.ListNotifyOverdueTasks(ctx, db.ListNotifyOverdueTasksParams{
			OrganizationID: orgID, NowAt: ts(now), SinceAt: ts(now.Add(-sweepWindow)),
		})
		e.warn(err, orgID, catalog.FieldTaskOverdue)
		for _, c := range rows {
			e.emitTask(ctx, orgID, catalog.FieldTaskOverdue, c.Uuid, c.AssigneeID, c.TaskType, c.DueAt.Time, c.BranchName,
				"overdue:"+strconv.FormatInt(c.DueAt.Time.Unix(), 10))
		}
	}
}

func (e *Engine) emitTask(ctx context.Context, orgID int64, code string, id uuid.UUID, userID int64, taskType string, due time.Time, branch, key string) {
	link := ""
	if e.portalBase != "" {
		if org, err := e.q.GetNotificationOrganization(ctx, orgID); err == nil {
			link = e.portalBase + "/t/" + org.Slug + "/field/my/" + id.String()
		}
	}
	lang := "tr"
	if u, err := e.q.GetNotificationStaff(ctx, db.GetNotificationStaffParams{OrganizationID: orgID, UserID: userID}); err == nil && u.Locale == "en" {
		lang = "en"
	} else if err != nil {
		return // no longer a member
	}
	e.emitSoft(ctx, orgID, EmitInput{
		Event: code, UserID: userID, RefType: "field_task", RefUUID: &id, Key: key, Link: link,
		Vars: map[string]string{"task_type": catalog.TaskTypeLabel(taskType, lang), "due_at": FormatTime(due), "branch": branch},
	})
}

// offsetKey: one reminder per offset and deadline (a moved deadline gets
// its own reminders).
func offsetKey(offset int32, at time.Time) string {
	return "remind:" + strconv.Itoa(int(offset)) + ":" + strconv.FormatInt(at.Unix(), 10)
}

func (e *Engine) warn(err error, orgID int64, code string) {
	if err != nil {
		e.log.Warn("notify_source_failed", "organization", orgID, "event", code, "error", err)
	}
}
