package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// reuseMargin: an active link is reused only while it stays valid at least
// this long (time to finish the checkout).
const reuseMargin = 10 * time.Minute

// Pay returns the pay page of the amount due now: the reservation's active
// pay-by-link link when it is for exactly that amount and still valid for
// a while, else a new link through the payments service (which replaces
// the active one). The customer is redirected to /pay/{token}.
func (s *Service) Pay(ctx context.Context, p Principal, id uuid.UUID, client model.ClientInfo) (model.PayResult, error) {
	row, err := s.scoped(ctx, p, id)
	if err != nil {
		return model.PayResult{}, err
	}
	octx := orgContext(ctx, p.Org)
	if !s.paymentsOn(octx, p.Org.ID) {
		return model.PayResult{}, ErrPaymentUnavailable
	}
	r, err := s.reservations.Get(octx, p.Org.ID, row.Uuid)
	if err != nil {
		return model.PayResult{}, err
	}
	due := payDue(r)
	if !due.IsPositive() {
		return model.PayResult{}, ErrNothingDue
	}
	now := s.clock()
	link, err := s.q.GetActivePaymentLinkForReservation(octx, db.GetActivePaymentLinkForReservationParams{
		OrganizationID: p.Org.ID, ReservationID: pgtype.Int8{Int64: row.ID, Valid: true}, Now: ts(now.Add(reuseMargin)),
	})
	switch {
	case err == nil && money.FromNumericOrZero(link.Amount).Equal(due):
		out := model.PayResult{URL: s.payments.LinkURL(link.Token), Amount: due, Currency: link.Currency, ExpiresAt: link.ExpiresAt.Time.UTC(), Reused: true}
		s.audit(octx, "customer_portal.payment_link_reused", model.ResourcePortal, &row.Uuid, map[string]any{
			"link_uuid": link.Uuid, "amount": due.String(), "session_uuid": p.SessionUUID,
		}, &client)
		return out, nil
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return model.PayResult{}, err
	}
	created, err := s.payments.CreateLink(octx, p.Org.ID, row.Uuid, paymodel.CreateLinkInput{})
	var se *paymentsusecase.StateError
	var ve *paymentsusecase.ValidationError
	switch {
	case errors.Is(err, paymentsusecase.ErrNoAccount):
		return model.PayResult{}, ErrPaymentUnavailable
	case errors.As(err, &se):
		return model.PayResult{}, stateErr(se.Code, se.Message)
	case errors.As(err, &ve):
		return model.PayResult{}, ErrNothingDue
	case err != nil:
		return model.PayResult{}, err
	}
	s.audit(octx, "customer_portal.payment_link_created", model.ResourcePortal, &row.Uuid, map[string]any{
		"link_uuid": created.UUID, "amount": created.Amount.String(), "purpose": created.Purpose, "session_uuid": p.SessionUUID,
	}, &client)
	return model.PayResult{URL: created.URL, Amount: created.Amount, Currency: created.Currency, ExpiresAt: created.ExpiresAt}, nil
}

const maxMessage = 2000

// CreateRequest records a change or cancellation request for staff. A
// cancellation shows (and stores) the policy text and fee of the
// reservation's policy snapshot; it cancels at once only when that
// snapshot explicitly allows self-cancellation, otherwise staff decide.
func (s *Service) CreateRequest(ctx context.Context, p Principal, id uuid.UUID, in model.CreateRequestInput, client model.ClientInfo) (model.Request, error) {
	row, err := s.scoped(ctx, p, id)
	if err != nil {
		return model.Request{}, err
	}
	octx := orgContext(ctx, p.Org)
	msg := strings.TrimSpace(in.Message)
	ve := &ValidationError{}
	switch in.Type {
	case model.RequestChange:
		if len([]rune(msg)) < 3 && in.RequestedStartsAt == nil && in.RequestedEndsAt == nil {
			ve.Details = append(ve.Details, Detail{Field: "message", Code: "required", Message: "describe the change you need"})
		}
		if in.RequestedStartsAt != nil && in.RequestedEndsAt != nil && !in.RequestedEndsAt.After(*in.RequestedStartsAt) {
			ve.Details = append(ve.Details, Detail{Field: "requested_ends_at", Code: "invalid", Message: "the return must be after the pick-up"})
		}
	case model.RequestCancel:
		if in.RequestedStartsAt != nil || in.RequestedEndsAt != nil {
			ve.Details = append(ve.Details, Detail{Field: "requested_starts_at", Code: "invalid", Message: "dates belong to a change request"})
		}
	default:
		ve.Details = append(ve.Details, Detail{Field: "type", Code: "invalid", Message: "type is change or cancel"})
	}
	if len([]rune(msg)) > maxMessage {
		ve.Details = append(ve.Details, Detail{Field: "message", Code: "too_long", Message: "at most 2000 characters"})
	}
	if len(ve.Details) > 0 {
		return model.Request{}, ve
	}
	r, err := s.reservations.Get(octx, p.Org.ID, row.Uuid)
	if err != nil {
		return model.Request{}, err
	}
	_, cancellation := parsePolicy(r.Policy)
	now := s.clock()
	params := db.InsertCustomerRequestParams{
		OrganizationID: p.Org.ID, ReservationID: row.ID, CustomerID: p.CustomerID, DriverID: p.DriverID,
		SessionID: pgtype.Int8{Int64: p.SessionID, Valid: p.SessionID != 0}, RequestType: in.Type, Status: model.RequestOpen, Message: msg,
		RequestedStartsAt: tsPtr(in.RequestedStartsAt), RequestedEndsAt: tsPtr(in.RequestedEndsAt), Currency: r.Price.Currency,
		ContactMasked: p.ContactMasked,
	}
	if params.Currency == "" {
		params.Currency = "TRY"
	}
	switch in.Type {
	case model.RequestChange:
		if !changeable(r.Status) {
			return model.Request{}, stateErr("not_changeable", "this reservation can no longer be changed")
		}
	case model.RequestCancel:
		if !resmodel.CanAct(r.Status, resmodel.ActionCancel) {
			return model.Request{}, stateErr("not_cancellable", "this reservation can no longer be cancelled")
		}
		params.PolicyText = cancellation.PolicyText
		if cancellation.Fee != nil {
			params.CancellationFee = cancellation.Fee.Numeric()
		}
	}
	selfCancel := in.Type == model.RequestCancel && cancellation.SelfCancel
	if selfCancel {
		if _, err := s.reservations.Cancel(octx, p.Org.ID, row.Uuid, "Müşteri portalından iptal (politika izin veriyor)"); err != nil {
			return model.Request{}, err
		}
		params.Status, params.Outcome, params.ResolvedAt = model.RequestResolved, model.OutcomeSelfCancelled, ts(now)
		params.ResolutionNote = "self_cancelled"
	}
	var created db.InsertCustomerRequestRow
	err = s.tx(octx, func(q *db.Queries) error {
		var err error
		created, err = q.InsertCustomerRequest(octx, params)
		if err != nil {
			return err
		}
		return q.InsertReservationEvent(octx, db.InsertReservationEventParams{
			OrganizationID: p.Org.ID, ReservationID: row.ID, Action: "customer_request",
			Data: mustJSON(map[string]any{
				"request_uuid": created.Uuid, "type": in.Type, "status": params.Status, "outcome": params.Outcome, "role": p.Role,
				"contact": p.ContactMasked,
			}),
		})
	})
	if uniqueViolation(err, "uq_customer_requests_open") {
		return model.Request{}, ErrRequestExists
	}
	if err != nil {
		return model.Request{}, err
	}
	s.publish(octx, model.ResourceRequests, "created", created.Uuid)
	s.publish(octx, resmodel.ResourceReservations, "updated", row.Uuid)
	s.audit(octx, "customer_portal.request_created", model.ResourceRequests, &created.Uuid, map[string]any{
		"reservation_uuid": row.Uuid, "type": in.Type, "status": params.Status, "outcome": params.Outcome, "role": p.Role,
		"session_uuid": p.SessionUUID,
	}, &client)
	list, err := s.requests(ctx, p.Org.ID, row.ID)
	if err != nil {
		return model.Request{}, err
	}
	for _, q := range list {
		if q.UUID == created.Uuid {
			return q, nil
		}
	}
	return model.Request{}, ErrNotFound
}

func feeOf(valid bool, n pgtype.Numeric) *money.Money {
	if !valid {
		return nil
	}
	v := money.FromNumericOrZero(n)
	return &v
}
