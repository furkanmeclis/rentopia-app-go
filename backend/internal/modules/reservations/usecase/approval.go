package usecase

// Approval flows and payments of reservations (TEC-708 / TEC-707,
// docs/design/payments.md §5):
//
//   - Accept: the staff approval of a request (no stock yet: capacity is
//     reserved now, at the current price) or of a review (the option is
//     kept and moved to the payment window); optionally a pay-by-link link
//     for the prepayment due.
//   - Reject: the staff refusal (reason) of a request / review.
//   - RecordPayment: the payments module's port. A covered prepayment
//     confirms an awaiting_payment reservation; a payment that arrives
//     after the hold expired re-validates capacity (reinstate) or opens a
//     late_payment resolution task. Never a silent confirmation.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Timeline actions of the payment flows.
const (
	EventLatePayment = "late_payment"
	EventLinkCreated = "payment_link_created"
)

// SetPaymentLinks wires the pay-by-link creator (payments module).
func (s *Service) SetPaymentLinks(l model.PaymentLinker) { s.links = l }

// Accept is the staff approval of a request or a review (approval queue).
func (s *Service) Accept(ctx context.Context, orgID int64, id uuid.UUID, in model.AcceptInput) (model.AcceptResult, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.AcceptResult{}, err
	}
	if !model.CanAct(c.Status, model.ActionApprove) {
		return model.AcceptResult{}, stateErr("invalid_transition", "only a request or a reservation awaiting review can be accepted")
	}
	if len(in.Note) > 500 {
		return model.AcceptResult{}, validation("note", "too_long", "note is at most 500 characters")
	}
	if in.SendPaymentLink && !has(ctx, rbac.PermTenantPaymentsCreateLink) {
		return model.AcceptResult{}, ErrForbidden
	}
	out := model.AcceptResult{Warnings: []string{}}
	switch c.Status {
	case model.StatusRequest:
		w, err := s.acceptRequest(ctx, orgID, c, in)
		if err != nil {
			return model.AcceptResult{}, err
		}
		out.Warnings = append(out.Warnings, w...)
	default:
		if in.QuoteUUID != nil {
			return model.AcceptResult{}, validation("quote_uuid", "not_allowed", "a review keeps the price it holds")
		}
		if _, err := s.Approve(ctx, orgID, id); err != nil {
			return model.AcceptResult{}, err
		}
	}
	res, err := s.Get(ctx, orgID, id)
	if err != nil {
		return model.AcceptResult{}, err
	}
	out.Reservation = res
	if in.SendPaymentLink && res.Status == model.StatusAwaitingPayment && res.PaymentDeadline != nil {
		if s.links == nil {
			out.PaymentLinkError = "payments_unavailable"
		} else if link, err := s.links.CreateReservationLink(ctx, orgID, id, res.Price.PrepaymentDue, *res.PaymentDeadline); err != nil {
			out.PaymentLinkError = err.Error()
		} else {
			out.PaymentLink = &link
		}
	}
	return out, nil
}

// acceptRequest reserves capacity for a request at the current price: the
// sold price must still hold (else QUOTE_PRICE_CHANGED with a change quote
// the staff accepts by sending its uuid). A prepayment due → awaiting_payment
// on an option that lasts the payment window; nothing due → confirmed.
func (s *Service) acceptRequest(ctx context.Context, orgID int64, c db.GetReservationCoreRow, in model.AcceptInput) ([]string, error) {
	w, _, err := s.where(ctx, orgID, c.Uuid)
	if err != nil {
		return nil, err
	}
	snap := snapshotOf(c.QuoteSnapshot)
	total := money.FromNumericOrZero(c.TotalAmount)
	prepay := money.FromNumericOrZero(c.PrepaymentAmount)
	var quoteRow *db.GetQuoteRow
	var newSnap *model.PriceSnapshot
	switch {
	case in.QuoteUUID != nil:
		row, err := s.loadQuote(ctx, orgID, *in.QuoteUUID)
		if errors.Is(err, ErrQuoteNotFound) || (err == nil && (!row.ReservationID.Valid || row.ReservationID.Int64 != c.ID)) {
			return nil, validation("quote_uuid", "not_found", "quote was not found for this reservation")
		}
		if err != nil {
			return nil, err
		}
		switch {
		case row.Status != model.QuoteOpen:
			return nil, &QuoteError{Code: "QUOTE_USED", Message: "the quote was already used"}
		case !row.ValidUntil.Time.After(s.clock()):
			return nil, &QuoteError{Code: "QUOTE_EXPIRED", Message: "the quote has expired; accept again"}
		case row.RentalProductID != c.RentalProductID || !row.StartsAt.Time.Equal(c.StartsAt.Time) || !row.EndsAt.Time.Equal(c.EndsAt.Time):
			return nil, validation("quote_uuid", "mismatch", "the quote prices another product or range")
		}
		q, err := s.GetQuote(ctx, orgID, row.Uuid)
		if err != nil {
			return nil, err
		}
		ns := priceSnapshot(q)
		quoteRow, newSnap = &row, &ns
		total, prepay = q.Total, q.Prepayment
	case snap.ManualDiscount == nil && !snap.KeptPrice:
		// Re-price at today's rates (a manual discount or a kept price was
		// a staff decision at sale time: it stands).
		spec := quoteSpec{
			purpose: model.PurposeChange, reservationID: c.ID, where: w, productUUID: c.ProductUuid, extras: snap.Extras,
			pickupDelivery: storedDelivery(c.PickupDelivery), returnDelivery: storedDelivery(c.ReturnDelivery),
		}
		if snap.CouponCode != nil {
			spec.coupon = *snap.CouponCode
		}
		p, err := s.price(ctx, orgID, spec)
		if err != nil {
			return nil, err
		}
		if !p.side.Breakdown.Total.Equal(total) {
			qid, err := s.changeQuote(ctx, orgID, c, spec, 0)
			if err != nil {
				return nil, err
			}
			fresh, err := s.GetQuote(ctx, orgID, qid)
			if err != nil {
				return nil, err
			}
			return nil, &QuoteError{Code: "QUOTE_PRICE_CHANGED", Message: "the price changed since the request; accept the new price", Replacement: &fresh}
		}
	}
	settings, err := s.settingsOf(ctx, orgID)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	due := prepay.Sub(money.FromNumericOrZero(c.PaidAmount))
	status, kind := model.StatusConfirmed, availmodel.KindReservation
	var deadline *time.Time
	if due.IsPositive() {
		d := now.Add(time.Duration(minutesOr(settings.Settings.Reservation.PaymentHoldMinutes, model.DefaultPaymentMinutes)) * time.Minute)
		status, kind, deadline = model.StatusAwaitingPayment, availmodel.KindHold, &d
	}
	ref := c.Uuid
	var allocUUID uuid.UUID
	var touched []uuid.UUID
	commission := money.FromNumericOrZero(c.CommissionAmount)
	_, err = s.avail.Reserve(unscoped(ctx), orgID, availmodel.ReserveInput{
		Where: w, ProductUUID: c.ProductUuid, Kind: kind, HoldUntil: deadline, RefType: model.RefType, RefUUID: &ref,
		ActorID: actorID(ctx), Note: "reservation_accepted",
		Hook: hookOf(func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
			if quoteRow != nil {
				conds, err := s.conditionsByID(ctx, orgID, r.ProductID, r.ProductVersion)
				if err != nil {
					return err
				}
				n, err := q.UpdateReservationPlan(ctx, db.UpdateReservationPlanParams{
					OrganizationID: orgID, ID: c.ID, AllocationID: int8Of(r.AllocationID), StartsAt: c.StartsAt, EndsAt: c.EndsAt,
					PickupBranchID: c.PickupBranchID, ReturnBranchID: c.ReturnBranchID, PickupPointID: c.PickupPointID,
					ReturnPointID: c.ReturnPointID, PickupDelivery: c.PickupDelivery, ReturnDelivery: c.ReturnDelivery,
					RentalProductID: r.ProductID, ProductVersion: r.ProductVersion,
					GuaranteeType: conds.GuaranteeType, ProductSnapshot: mustJSON(conds), TotalAmount: total.Numeric(),
					PrepaymentAmount: prepay.Numeric(), NetReceivable: total.Sub(commission).Numeric(),
					QuoteID: int8Of(quoteRow.ID), QuoteSnapshot: mustJSON(newSnap),
				})
				if err != nil {
					return err
				}
				if n != 1 {
					return ErrConcurrent
				}
				used, err := q.MarkQuoteUsed(ctx, db.MarkQuoteUsedParams{OrganizationID: orgID, ID: quoteRow.ID, UsedAt: ts(now)})
				if err != nil {
					return err
				}
				if used != 1 {
					return &QuoteError{Code: "QUOTE_USED", Message: "the quote was already used"}
				}
			}
			data := map[string]any{"accepted_by_staff": true, "total": total.String(), "prepayment": prepay.String()}
			if note := strings.TrimSpace(in.Note); note != "" {
				data["note"] = note
			}
			if quoteRow != nil {
				data["quote_uuid"] = quoteRow.Uuid
			}
			if err := s.apply(ctx, q, orgID, transition{
				id: c.ID, from: c.Status, to: status, action: model.ActionApprove, payDeadline: deadline,
				allocationID: r.AllocationID, data: data,
			}); err != nil {
				return err
			}
			if err := assignment(ctx, q, orgID, c.ID, r.VehicleID, r.ProductID, r.ProductVersion, r.PlateFixed, model.SourceEngine, ""); err != nil {
				return err
			}
			allocUUID = r.AllocationUUID
			t, err := s.planEffects(ctx, q, orgID, r, "reservation", c.Uuid)
			touched = t
			return err
		}, s),
	})
	if err != nil {
		return nil, mapEngineErr(err)
	}
	warnings := []string{}
	extras := snap.Extras
	if newSnap != nil {
		extras = newSnap.Extras
	}
	if allocUUID != uuid.Nil && len(extras) > 0 {
		warnings = append(warnings, s.resyncEquipment(ctx, orgID, c.ID, nil, extras, allocUUID, c.Uuid)...)
	}
	if quoteRow != nil {
		s.publish(ctx, model.ResourceQuotes, "updated", quoteRow.Uuid)
	}
	s.publish(ctx, model.ResourceReservations, "updated", append(touched, c.Uuid)...)
	return warnings, nil
}

// Reject is the staff refusal of a request / review: cancelled with the
// reason (an option is released exactly once, a coupon use returned).
func (s *Service) Reject(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	if !model.CanAct(c.Status, model.ActionReject) {
		return model.Reservation{}, stateErr("invalid_transition", "only a request or a reservation awaiting review can be rejected")
	}
	r, err := requireReason(reason)
	if err != nil {
		return model.Reservation{}, err
	}
	if err := s.terminate(ctx, orgID, c, model.ActionReject, model.StatusCancelled, r); err != nil {
		return model.Reservation{}, err
	}
	return s.after(ctx, orgID, id)
}

func paymentStatusOf(paid, total money.Money, refunded bool) string {
	switch {
	case refunded && !paid.IsPositive():
		return model.PaymentRefunded
	case paid.IsPositive() && paid.Cmp(total) >= 0:
		return model.PaymentPaid
	case paid.IsPositive():
		return model.PaymentPartially
	case refunded:
		return model.PaymentRefunded
	}
	return model.PaymentUnpaid
}

// RecordPayment implements model.PaymentRecorder. in.Paid is the total
// received so far (refunds deducted). A new payment (in.Payment set) on an
// awaiting_payment reservation whose prepayment is now covered confirms it
// (the option becomes firm on the same allocation); on an expired one it
// re-validates capacity (reinstate) and otherwise — or on a cancelled /
// no-show / already lapsed one — opens a late_payment task.
func (s *Service) RecordPayment(ctx context.Context, orgID int64, id uuid.UUID, in model.PaymentUpdate) (string, error) {
	c, err := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if in.Paid.IsNegative() {
		return "", validation("paid", "invalid", "paid must not be negative")
	}
	total := money.FromNumericOrZero(c.TotalAmount)
	status := paymentStatusOf(in.Paid, total, in.Refunded)
	ctx = unscoped(ctx)
	err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		if _, err := q.SetReservationPayment(ctx, db.SetReservationPaymentParams{
			OrganizationID: orgID, ID: c.ID, PaymentStatus: status, PaidAmount: in.Paid.Numeric(),
		}); err != nil {
			return err
		}
		data := map[string]any{"paid": in.Paid.String(), "payment_status": status, "source": in.Source}
		if in.Payment != uuid.Nil {
			data["payment_uuid"] = in.Payment
			data["amount"] = in.Amount.String()
		}
		return event(ctx, q, orgID, c.ID, EventPaymentRecorded, "", "", "", data)
	})
	if err != nil {
		return "", err
	}
	outcome := model.OutcomeRecorded
	newPayment := in.Payment != uuid.Nil && in.Amount.IsPositive()
	covered := in.Paid.Cmp(money.FromNumericOrZero(c.PrepaymentAmount)) >= 0
	switch {
	case c.Status == model.StatusAwaitingPayment && covered:
		c.PaidAmount = in.Paid.Numeric()
		_, cerr := s.confirm(ctx, orgID, c, model.ActionConfirm)
		var se *StateError
		switch {
		case cerr == nil:
			outcome = model.OutcomeConfirmed
		case errors.As(cerr, &se) && se.Code == "hold_expired" && newPayment:
			// The option lapsed before the sweep: the reservation expired
			// just now; the payment is late.
			c2, err := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: id})
			if err != nil {
				return "", err
			}
			outcome, err = s.latePayment(ctx, orgID, c2, in)
			if err != nil {
				return "", err
			}
		default:
			return "", cerr
		}
	case newPayment && model.IsTerminal(c.Status):
		outcome, err = s.latePayment(ctx, orgID, c, in)
		if err != nil {
			return "", err
		}
	}
	s.publish(ctx, model.ResourceReservations, "updated", id)
	return outcome, nil
}

// latePayment handles a payment that arrived after the reservation left
// the waiting states. An expired reservation whose prepayment is covered is
// re-validated against current capacity: a firm allocation found again
// reinstates it (expired → confirmed, recorded with the payment); anything
// else — capacity gone, a cancelled / no-show reservation, a partial
// payment — opens a late_payment task (refund or re-book). Never silent.
func (s *Service) latePayment(ctx context.Context, orgID int64, c db.GetReservationCoreRow, in model.PaymentUpdate) (string, error) {
	covered := in.Paid.Cmp(money.FromNumericOrZero(c.PrepaymentAmount)) >= 0
	reason := "a payment arrived for a " + c.Status + " reservation: refund the customer or re-book"
	data := map[string]any{"payment_uuid": in.Payment, "amount": in.Amount.String(), "paid": in.Paid.String(), "status": c.Status}
	if c.Status == model.StatusExpired && covered {
		w, _, err := s.where(ctx, orgID, c.Uuid)
		if err != nil {
			return "", err
		}
		ref := c.Uuid
		var allocUUID uuid.UUID
		_, rerr := s.avail.Reserve(ctx, orgID, availmodel.ReserveInput{
			Where: w, ProductUUID: c.ProductUuid, Kind: availmodel.KindReservation, RefType: model.RefType, RefUUID: &ref,
			Note: "late_payment",
			Hook: hookOf(func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
				n, err := q.ReinstateReservation(ctx, db.ReinstateReservationParams{
					OrganizationID: orgID, ID: c.ID, AllocationID: pgtype.Int8{Int64: r.AllocationID, Valid: true}, ConfirmedAt: ts(s.clock()),
				})
				if err != nil {
					return err
				}
				if n != 1 {
					return ErrConcurrent
				}
				if err := event(ctx, q, orgID, c.ID, model.ActionReinstate, model.StatusExpired, model.StatusConfirmed, "late_payment", data); err != nil {
					return err
				}
				if err := assignment(ctx, q, orgID, c.ID, r.VehicleID, r.ProductID, r.ProductVersion, r.PlateFixed, model.SourceEngine, "late_payment"); err != nil {
					return err
				}
				allocUUID = r.AllocationUUID
				_, err = s.planEffects(ctx, q, orgID, r, "reservation", c.Uuid)
				return err
			}, s),
		})
		if rerr == nil {
			snap := snapshotOf(c.QuoteSnapshot)
			if len(snap.Extras) > 0 {
				s.resyncEquipment(ctx, orgID, c.ID, nil, snap.Extras, allocUUID, c.Uuid)
			}
			return model.OutcomeReinstated, nil
		}
		if _, ok := capacityErr(rerr); ok {
			reason = "the payment arrived after the hold expired and the capacity is gone: refund the customer or re-book"
			data["reason_code"] = "capacity_unavailable"
		} else {
			slog.Default().Warn("late_payment_reinstate_failed", "reservation", c.Uuid, "error", rerr)
			reason = "the payment arrived after the hold expired and the reservation could not be re-confirmed: refund or re-book"
			data["reason_code"] = "reinstate_failed"
		}
	} else if c.Status == model.StatusExpired {
		data["reason_code"] = "partial_after_expiry"
	} else {
		data["reason_code"] = "reservation_" + c.Status
	}
	err := s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		taskUUID, err := q.InsertResolutionTask(ctx, db.InsertResolutionTaskParams{
			OrganizationID: orgID, ReservationID: c.ID, TaskType: model.TaskLatePayment, Reason: reason,
			SourceType: "payment", SourceUuid: pgtype.UUID{Bytes: in.Payment, Valid: true}, Data: mustJSON(data),
		})
		if notFound(err) {
			return nil // the task of this payment is open already
		}
		if err != nil {
			return err
		}
		data["task_uuid"] = taskUUID
		return event(ctx, q, orgID, c.ID, EventLatePayment, "", "", reason, data)
	})
	if err != nil {
		return "", err
	}
	s.publish(ctx, model.ResourceTasks, "created", c.Uuid)
	return model.OutcomeLateTask, nil
}
