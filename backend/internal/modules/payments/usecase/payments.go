package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Sortable payment list fields.
var Sortable = map[string]string{"amount": "amount", "created_at": "created_at"}

// List returns a page of payments in the caller's branch scope.
func (s *Service) List(ctx context.Context, orgID int64, f paymodel.ListFilter) ([]paymodel.Payment, int64, error) {
	if _, ok := Sortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	p := db.ListPaymentsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), ReservationUuids: nilUUIDs(f.ReservationUUIDs),
		Statuses: nilIfEmpty(f.Statuses), Providers: nilIfEmpty(f.Providers), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListPayments(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]paymodel.Payment, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		it := paymodel.Payment{
			UUID: r.Uuid, Reservation: paymodel.ReservationRef{UUID: r.ReservationUuid, Reference: r.ReservationReference},
			Provider: r.Provider, Method: r.Method, Status: r.Status, Currency: r.Currency, Amount: m(r.Amount),
			GrossAmount: m(r.GrossAmount), CommissionAmount: m(r.CommissionAmount), NetAmount: m(r.NetAmount),
			RefundedAmount: m(r.RefundedAmount), ProviderPaymentID: textPtr(r.ProviderPaymentID), FailureReason: r.FailureReason,
			VerifiedVia: r.VerifiedVia, VerifiedAt: timePtr(r.VerifiedAt), Outcome: r.Outcome, Note: r.Note,
			CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail), CreatedAt: r.CreatedAt.Time.UTC(),
		}
		it.Refundable = refundable(r.Status, it.Amount, it.RefundedAmount)
		if r.LinkUuid.Valid {
			id := uuid.UUID(r.LinkUuid.Bytes)
			it.LinkUUID = &id
		}
		out = append(out, it)
	}
	return out, total, nil
}

func refundable(status string, amount, refunded money.Money) money.Money {
	if status != paymodel.StatusSucceeded && status != paymodel.StatusPartiallyRefunded {
		return money.Zero()
	}
	return amount.Sub(refunded).ClampZero()
}

func (s *Service) row(ctx context.Context, orgID int64, id uuid.UUID) (db.GetPaymentRow, error) {
	r, err := s.q.GetPayment(ctx, db.GetPaymentParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if !inScope(ctx, r.PickupBranchID, r.ReturnBranchID) {
		return r, ErrNotFound
	}
	return r, nil
}

// Get returns one payment with its refunds.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (paymodel.Payment, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return paymodel.Payment{}, err
	}
	p := paymodel.Payment{
		UUID: r.Uuid, Reservation: paymodel.ReservationRef{UUID: r.ReservationUuid, Reference: r.ReservationReference},
		Provider: r.Provider, Method: r.Method, Status: r.Status, Currency: r.Currency, Amount: m(r.Amount),
		GrossAmount: m(r.GrossAmount), CommissionAmount: m(r.CommissionAmount), NetAmount: m(r.NetAmount),
		RefundedAmount: m(r.RefundedAmount), ProviderPaymentID: textPtr(r.ProviderPaymentID), ProviderStatus: r.ProviderStatus,
		ProviderData: json.RawMessage(r.ProviderData), FailureReason: r.FailureReason, VerifiedVia: r.VerifiedVia,
		VerifiedAt: timePtr(r.VerifiedAt), Outcome: r.Outcome, Note: r.Note, Refunds: []paymodel.Refund{},
		CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail), CreatedAt: r.CreatedAt.Time.UTC(),
	}
	p.Refundable = refundable(r.Status, p.Amount, p.RefundedAmount)
	if r.LinkUuid.Valid {
		lid := uuid.UUID(r.LinkUuid.Bytes)
		p.LinkUUID = &lid
	}
	refunds, err := s.q.ListPaymentRefunds(ctx, db.ListPaymentRefundsParams{OrganizationID: orgID, PaymentID: r.ID})
	if err != nil {
		return paymodel.Payment{}, err
	}
	for _, f := range refunds {
		p.Refunds = append(p.Refunds, paymodel.Refund{
			UUID: f.Uuid, Amount: m(f.Amount), Currency: f.Currency, Status: f.Status, ProviderRefundID: textPtr(f.ProviderRefundID),
			Reason: f.Reason, FailureReason: f.FailureReason,
			CreatedBy: actorOf(f.CreatedByUuid, f.CreatedByName, f.CreatedBySurname, f.CreatedByEmail), CreatedAt: f.CreatedAt.Time.UTC(),
		})
	}
	return p, nil
}

// Verify asks the provider for the result of a pending checkout (staff
// "verify"); a confirmed payment is applied if it was not yet.
func (s *Service) Verify(ctx context.Context, orgID int64, id uuid.UUID) (paymodel.Payment, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return paymodel.Payment{}, err
	}
	if _, err := s.verify(ctx, orgID, r.ID, "retrieve"); err != nil && err != ErrMismatch {
		return paymodel.Payment{}, err
	}
	return s.Get(ctx, orgID, id)
}

// Collect records money taken at the branch (cash / card POS / bank
// transfer): a manual payment, confirmed by the member who took it
// (tenant.payment.collect), applied to the reservation at once.
func (s *Service) Collect(ctx context.Context, orgID int64, reservationUUID uuid.UUID, in paymodel.CollectInput) (paymodel.Payment, error) {
	c, err := s.reservationFor(ctx, orgID, reservationUUID)
	if err != nil {
		return paymodel.Payment{}, err
	}
	ve := &ValidationError{}
	if !slices.Contains(paymodel.ManualMethods, in.Method) {
		ve.Details = append(ve.Details, Detail{"method", "invalid", "method must be cash, card_pos or bank_transfer"})
	}
	if len(in.Note) > 500 {
		ve.Details = append(ve.Details, Detail{"note", "too_long", "note is at most 500 characters"})
	}
	balance := m(c.TotalAmount).Sub(m(c.PaidAmount))
	amount := balance
	if in.Amount != nil {
		amount = *in.Amount
	}
	if !amount.IsPositive() {
		ve.Details = append(ve.Details, Detail{"amount", "invalid", "the amount must be positive"})
	} else if amount.Cmp(balance) > 0 {
		ve.Details = append(ve.Details, Detail{"amount", "too_large", "the amount exceeds the remaining balance"})
	}
	if len(ve.Details) > 0 {
		return paymodel.Payment{}, ve
	}
	if resmodel.IsTerminal(c.Status) || c.Status == resmodel.StatusRequest {
		return paymodel.Payment{}, stateErr("reservation_not_payable", "the reservation cannot take payments in its state")
	}
	id := uuid.New()
	now := s.clock()
	var paymentID int64
	err = s.tx(ctx, func(q *db.Queries) error {
		pid, err := q.InsertPayment(ctx, db.InsertPaymentParams{
			Uuid: id, OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: c.ID, Valid: true}, Provider: paymodel.ProviderManual,
			Method: in.Method, Status: paymodel.StatusSucceeded, Currency: c.Currency, Amount: amount.Numeric(),
			GrossAmount: amount.Numeric(), NetAmount: amount.Numeric(), ConversationID: id.String(), VerifiedVia: "manual",
			VerifiedAt: ts(now), Note: strings.TrimSpace(in.Note), CreatedBy: actorID(ctx),
		})
		paymentID = pid
		return err
	})
	if err != nil {
		return paymodel.Payment{}, err
	}
	if err := s.apply(ctx, orgID, paymentID); err != nil {
		return paymodel.Payment{}, err
	}
	s.publish(ctx, paymodel.ResourcePayments, "created", id)
	return s.Get(ctx, orgID, id)
}

// ProviderFailure wraps a provider refusal of a refund (502).
type ProviderFailure struct{ Err error }

func (e *ProviderFailure) Error() string { return e.Err.Error() }

func (e *ProviderFailure) Unwrap() error { return e.Err }

// Refund refunds (part of) a confirmed payment through its provider
// (tenant.payment.refund). Manual payments are refunded by recording it
// (the money goes back at the branch). The reservation's paid amount is
// recounted afterwards.
func (s *Service) Refund(ctx context.Context, orgID int64, id uuid.UUID, in paymodel.RefundInput, ip string) (paymodel.Payment, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return paymodel.Payment{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	if len(reason) < 3 || len(reason) > 500 {
		return paymodel.Payment{}, validation("reason", "required", "a reason is required (3-500 characters)")
	}
	left := refundable(r.Status, m(r.Amount), m(r.RefundedAmount))
	if !left.IsPositive() {
		return paymodel.Payment{}, stateErr("not_refundable", "the payment has nothing left to refund")
	}
	amount := left
	if in.Amount != nil {
		amount = *in.Amount
	}
	if !amount.IsPositive() || amount.Cmp(left) > 0 {
		return paymodel.Payment{}, validation("amount", "out_of_range", "the amount must be positive and at most "+left.String())
	}
	// Provider payments: the refund guard asks the provider what it has
	// refunded already (TEC-962) before anything is recorded or sent.
	var prov payments.Provider
	var acc integrations.Account
	if r.Provider != paymodel.ProviderManual {
		if prov, err = s.provider(r.Provider); err != nil {
			return paymodel.Payment{}, err
		}
		if acc, err = s.accounts.AccountByID(ctx, orgID, r.IntegrationAccountID.Int64); err != nil {
			return paymodel.Payment{}, err
		}
		if err := s.guardRefund(ctx, orgID, r, prov, acc, amount, reason); err != nil {
			return paymodel.Payment{}, err
		}
	}
	refund, err := s.q.InsertPaymentRefund(ctx, db.InsertPaymentRefundParams{
		OrganizationID: orgID, PaymentID: r.ID, Amount: amount.Numeric(), Currency: r.Currency, Reason: reason, CreatedBy: actorID(ctx),
	})
	if err != nil {
		return paymodel.Payment{}, err
	}
	providerRefundID := pgtype.Text{}
	if prov != nil {
		job, err := s.q.UpsertIntegrationJob(ctx, db.UpsertIntegrationJobParams{
			OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, IntegrationAccountID: pgtype.Int8{Int64: acc.ID, Valid: true},
			Direction: "outbound", Provider: r.Provider, Kind: "payment.refund",
			IdempotencyKey: fmt.Sprintf("org:%d:refund:%s", orgID, refund.Uuid),
			Payload:        mustJSON(map[string]any{"payment_uuid": r.Uuid, "refund_uuid": refund.Uuid, "amount": amount.String()}),
			RefType:        "payment_refund", RefUuid: pgtype.UUID{Bytes: refund.Uuid, Valid: true},
		})
		if err != nil {
			return paymodel.Payment{}, err
		}
		_, _ = s.q.ClaimIntegrationJob(ctx, db.ClaimIntegrationJobParams{OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: job.ID})
		started := time.Now()
		res, perr := prov.Refund(ctx, acc, payments.RefundRequest{
			PaymentID: r.ProviderPaymentID.String, ConversationID: refund.Uuid.String(), Amount: amount, Currency: r.Currency, IP: ip,
		})
		// The call is an integration_attempts row like every runner call
		// (TEC-751); the refund flow itself is unchanged.
		s.recordAttempt(ctx, orgID, job.ID, job.Attempts+1, started, perr,
			map[string]any{"refund_uuid": refund.Uuid, "amount": amount.String(), "currency": r.Currency},
			map[string]any{"refund_id": res.RefundID})
		if perr != nil {
			_ = s.q.FinishPaymentRefund(ctx, db.FinishPaymentRefundParams{OrganizationID: orgID, ID: refund.ID, Status: "failed", FailureReason: perr.Error()})
			_ = s.q.FinishIntegrationJob(ctx, db.FinishIntegrationJobParams{
				OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: job.ID, Status: "failed", LastError: perr.Error(), Result: []byte("{}"),
			})
			return paymodel.Payment{}, &ProviderFailure{Err: perr}
		}
		providerRefundID = pgtype.Text{String: res.RefundID, Valid: res.RefundID != ""}
		_ = s.q.FinishIntegrationJob(ctx, db.FinishIntegrationJobParams{
			OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: job.ID, Status: "succeeded",
			Result: mustJSON(map[string]any{"refund_id": res.RefundID}),
		})
	}
	if err := s.finishRefund(ctx, orgID, r, refund.ID, refund.Uuid, amount, reason, providerRefundID); err != nil {
		return paymodel.Payment{}, err
	}
	return s.Get(ctx, orgID, id)
}

// finishRefund records a refund that happened (the provider answered, or
// a manual refund): refundTx in one transaction, then afterRefund.
func (s *Service) finishRefund(ctx context.Context, orgID int64, r db.GetPaymentRow, refundID int64, refundUUID uuid.UUID,
	amount money.Money, reason string, providerRefundID pgtype.Text) error {
	if err := s.tx(ctx, func(q *db.Queries) error {
		return refundTx(ctx, q, orgID, r, refundID, refundUUID, amount, reason, providerRefundID)
	}); err != nil {
		return err
	}
	s.afterRefund(ctx, orgID, r, refundID, refundUUID)
	return nil
}

// refundTx marks the refund row succeeded, adds it to the payment's
// refunded amount and writes the reservation timeline entry.
func refundTx(ctx context.Context, q *db.Queries, orgID int64, r db.GetPaymentRow, refundID int64, refundUUID uuid.UUID,
	amount money.Money, reason string, providerRefundID pgtype.Text) error {
	if err := q.FinishPaymentRefund(ctx, db.FinishPaymentRefundParams{
		OrganizationID: orgID, ID: refundID, Status: "succeeded", ProviderRefundID: providerRefundID,
	}); err != nil {
		return err
	}
	n, err := q.AddPaymentRefunded(ctx, db.AddPaymentRefundedParams{OrganizationID: orgID, ID: r.ID, Amount: amount.Numeric()})
	if err != nil {
		return err
	}
	if n != 1 {
		return stateErr("not_refundable", "the payment changed in between; reload")
	}
	return event(ctx, q, orgID, r.ReservationID, "payment_refunded", reason, map[string]any{
		"payment_uuid": r.Uuid, "refund_uuid": refundUUID, "amount": amount.String(),
	})
}

// afterRefund posts the refund to the ledger and recounts the
// reservation's paid total (fail-soft; the finance sweep catches up).
func (s *Service) afterRefund(ctx context.Context, orgID int64, r db.GetPaymentRow, refundID int64, refundUUID uuid.UUID) {
	if s.ledger != nil {
		if err := s.ledger.PostRefund(ctx, orgID, refundID); err != nil {
			s.log.Warn("payment_refund_ledger_post_failed", "refund", refundUUID, "error", err)
		}
	}
	p, err := s.q.GetPaymentByID(ctx, db.GetPaymentByIDParams{OrganizationID: orgID, ID: r.ID})
	if err == nil {
		if _, err := s.recount(ctx, orgID, p, uuid.Nil, money.Zero(), "refund"); err != nil {
			s.log.Warn("payment_refund_recount_failed", "payment", r.Uuid, "error", err)
		}
	}
	s.publish(ctx, paymodel.ResourcePayments, "updated", r.Uuid)
}

// recordAttempt writes the integration_attempts row of a provider call made
// inline (fail-soft: the payment flow never depends on it).
func (s *Service) recordAttempt(ctx context.Context, orgID, jobID int64, attemptNo int32, started time.Time, callErr error,
	request, response map[string]any) {
	finished := time.Now()
	outcome, msg := "succeeded", ""
	if callErr != nil {
		outcome, msg = "failed", callErr.Error()
		response = map[string]any{}
	}
	if attemptNo < 1 {
		attemptNo = 1
	}
	if _, err := s.q.InsertIntegrationAttempt(ctx, db.InsertIntegrationAttemptParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, IntegrationJobID: jobID, AttemptNo: attemptNo,
		Operation: "submit", Trigger: "inline", Outcome: outcome, RequestSummary: mustJSON(request),
		ResponseSummary: mustJSON(response), Error: msg, StartedAt: pgtype.Timestamptz{Time: started, Valid: true},
		FinishedAt: pgtype.Timestamptz{Time: finished, Valid: true}, DurationMs: int32(finished.Sub(started) / time.Millisecond),
	}); err != nil {
		s.log.Warn("payment_attempt_record_failed", "job_id", jobID, "error", err)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
