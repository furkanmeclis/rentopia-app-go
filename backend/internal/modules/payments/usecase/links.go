package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Link expiry bounds (minutes).
const (
	MinLinkMinutes     = 5
	MaxLinkMinutes     = 7 * 24 * 60
	DefaultLinkMinutes = 24 * 60
)

// payable reports whether a reservation can still receive a link payment.
func payable(status string) bool {
	return status == resmodel.StatusAwaitingPayment || status == resmodel.StatusConfirmed
}

// CreateLink creates a pay-by-link link for a reservation (the active link
// of the reservation is replaced). The amount defaults to the prepayment
// due while the reservation waits for payment, else to the balance; it can
// never exceed the balance. While the reservation waits for payment the
// link expires with the payment window at the latest.
func (s *Service) CreateLink(ctx context.Context, orgID int64, reservationUUID uuid.UUID, in paymodel.CreateLinkInput) (paymodel.Link, error) {
	c, err := s.reservationFor(ctx, orgID, reservationUUID)
	if err != nil {
		return paymodel.Link{}, err
	}
	if !payable(c.Status) {
		return paymodel.Link{}, stateErr("reservation_not_payable", "only a reservation awaiting payment or confirmed can be paid by link")
	}
	total, paid, prepay := m(c.TotalAmount), m(c.PaidAmount), m(c.PrepaymentAmount)
	balance := total.Sub(paid)
	purpose := paymodel.PurposeBalance
	amount := balance
	if c.Status == resmodel.StatusAwaitingPayment {
		purpose, amount = paymodel.PurposePrepayment, prepay.Sub(paid)
	}
	if in.Amount != nil {
		amount = *in.Amount
		if !amount.Equal(prepay.Sub(paid)) || c.Status != resmodel.StatusAwaitingPayment {
			purpose = paymodel.PurposeCustom
		}
	}
	if !amount.IsPositive() {
		return paymodel.Link{}, validation("amount", "invalid", "nothing is due; the amount must be positive")
	}
	if amount.Cmp(balance) > 0 {
		return paymodel.Link{}, validation("amount", "too_large", "the amount exceeds the remaining balance")
	}
	now := s.clock()
	expires := now.Add(DefaultLinkMinutes * time.Minute)
	if in.ExpiresInMinutes != nil {
		if *in.ExpiresInMinutes < MinLinkMinutes || *in.ExpiresInMinutes > MaxLinkMinutes {
			return paymodel.Link{}, validation("expires_in_minutes", "out_of_range", "expires_in_minutes must be between 5 and 10080")
		}
		expires = now.Add(time.Duration(*in.ExpiresInMinutes) * time.Minute)
	}
	if c.Status == resmodel.StatusAwaitingPayment && c.PaymentDeadline.Valid {
		if in.ExpiresInMinutes == nil || c.PaymentDeadline.Time.Before(expires) {
			expires = c.PaymentDeadline.Time.UTC()
		}
	}
	if !expires.After(now) {
		return paymodel.Link{}, stateErr("payment_window_over", "the payment window of the reservation is over")
	}
	return s.createLink(ctx, orgID, c, amount, purpose, expires)
}

// CreateReservationLink implements reservations model.PaymentLinker (staff
// accept with "send payment link").
func (s *Service) CreateReservationLink(ctx context.Context, orgID int64, reservationUUID uuid.UUID, amount money.Money, expiresAt time.Time) (resmodel.PaymentLinkRef, error) {
	c, err := s.reservationFor(ctx, orgID, reservationUUID)
	if err != nil {
		return resmodel.PaymentLinkRef{}, err
	}
	if !amount.IsPositive() {
		return resmodel.PaymentLinkRef{}, validation("amount", "invalid", "nothing is due")
	}
	l, err := s.createLink(ctx, orgID, c, amount, paymodel.PurposePrepayment, expiresAt.UTC())
	if err != nil {
		return resmodel.PaymentLinkRef{}, err
	}
	return resmodel.PaymentLinkRef{UUID: l.UUID, URL: l.URL, Amount: l.Amount, ExpiresAt: l.ExpiresAt}, nil
}

func (s *Service) createLink(ctx context.Context, orgID int64, c db.GetReservationCoreRow, amount money.Money, purpose string, expires time.Time) (paymodel.Link, error) {
	acc, err := s.accounts.ActiveAccount(ctx, orgID, integrations.KindPayment)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return paymodel.Link{}, ErrNoAccount
	}
	if err != nil {
		return paymodel.Link{}, err
	}
	token, err := newToken()
	if err != nil {
		return paymodel.Link{}, err
	}
	id := uuid.New()
	now := s.clock()
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.CancelActivePaymentLinks(ctx, db.CancelActivePaymentLinksParams{
			OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: c.ID, Valid: true}, CancelledAt: ts(now),
		}); err != nil {
			return err
		}
		if _, err := q.InsertPaymentLink(ctx, db.InsertPaymentLinkParams{
			Uuid: id, OrganizationID: orgID, Token: token, ReservationID: pgtype.Int8{Int64: c.ID, Valid: true}, IntegrationAccountID: acc.ID,
			Purpose: purpose, Amount: amount.Numeric(), Currency: c.Currency, ExpiresAt: ts(expires), CreatedBy: actorID(ctx),
		}); err != nil {
			return err
		}
		return event(ctx, q, orgID, c.ID, "payment_link_created", "", map[string]any{
			"link_uuid": id, "amount": amount.String(), "expires_at": expires, "purpose": purpose,
		})
	})
	if err != nil {
		return paymodel.Link{}, err
	}
	s.publish(ctx, paymodel.ResourceLinks, "created", id)
	return s.GetLink(ctx, orgID, id)
}

// effectiveLinkStatus marks a lapsed active link expired (lazily).
func (s *Service) effectiveLinkStatus(ctx context.Context, orgID, id int64, status string, expires time.Time) string {
	if status == paymodel.LinkActive && !expires.After(s.clock()) {
		_, _ = s.q.ExpirePaymentLink(ctx, db.ExpirePaymentLinkParams{OrganizationID: orgID, ID: id, NowAt: ts(s.clock())})
		return paymodel.LinkExpired
	}
	return status
}

// GetLink returns one link of the caller's branch scope.
func (s *Service) GetLink(ctx context.Context, orgID int64, id uuid.UUID) (paymodel.Link, error) {
	r, err := s.q.GetPaymentLink(ctx, db.GetPaymentLinkParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return paymodel.Link{}, ErrLinkNotFound
	}
	if err != nil {
		return paymodel.Link{}, err
	}
	if !inScope(ctx, r.PickupBranchID, r.ReturnBranchID) {
		return paymodel.Link{}, ErrLinkNotFound
	}
	return paymodel.Link{
		UUID: r.Uuid, URL: s.LinkURL(r.Token), Reservation: paymodel.ReservationRef{UUID: r.ReservationUuid, Reference: r.ReservationReference},
		Purpose: r.Purpose, Amount: m(r.Amount), Currency: r.Currency,
		Status:    s.effectiveLinkStatus(ctx, orgID, r.ID, r.Status, r.ExpiresAt.Time),
		ExpiresAt: r.ExpiresAt.Time.UTC(), PaidAt: timePtr(r.PaidAt), CancelledAt: timePtr(r.CancelledAt),
		CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail), CreatedAt: r.CreatedAt.Time.UTC(),
	}, nil
}

// CancelLink cancels an active link.
func (s *Service) CancelLink(ctx context.Context, orgID int64, id uuid.UUID) (paymodel.Link, error) {
	r, err := s.q.GetPaymentLink(ctx, db.GetPaymentLinkParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) || (err == nil && !inScope(ctx, r.PickupBranchID, r.ReturnBranchID)) {
		return paymodel.Link{}, ErrLinkNotFound
	}
	if err != nil {
		return paymodel.Link{}, err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.CancelPaymentLink(ctx, db.CancelPaymentLinkParams{OrganizationID: orgID, ID: r.ID, CancelledAt: ts(s.clock())})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("link_not_active", "only an active link can be cancelled")
		}
		return event(ctx, q, orgID, r.ReservationID, "payment_link_cancelled", "", map[string]any{"link_uuid": r.Uuid})
	})
	if err != nil {
		return paymodel.Link{}, err
	}
	s.publish(ctx, paymodel.ResourceLinks, "updated", id)
	return s.GetLink(ctx, orgID, id)
}

// ListLinks returns a page of links in the caller's branch scope.
func (s *Service) ListLinks(ctx context.Context, orgID int64, f paymodel.LinkFilter) ([]paymodel.Link, int64, error) {
	rows, err := s.q.ListPaymentLinks(ctx, db.ListPaymentLinksParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), ReservationUuids: nilUUIDs(f.ReservationUUIDs),
		Statuses: nilIfEmpty(f.Statuses), LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]paymodel.Link, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		status := r.Status
		if status == paymodel.LinkActive && !r.ExpiresAt.Time.After(s.clock()) {
			status = paymodel.LinkExpired
		}
		out = append(out, paymodel.Link{
			UUID: r.Uuid, URL: s.LinkURL(r.Token), Reservation: paymodel.ReservationRef{UUID: r.ReservationUuid, Reference: r.ReservationReference},
			Purpose: r.Purpose, Amount: m(r.Amount), Currency: r.Currency, Status: status, ExpiresAt: r.ExpiresAt.Time.UTC(),
			PaidAt: timePtr(r.PaidAt), CancelledAt: timePtr(r.CancelledAt),
			CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail), CreatedAt: r.CreatedAt.Time.UTC(),
		})
	}
	return out, total, nil
}

// ensure the accounts service satisfies the port.
var _ Accounts = (*accountsusecase.Service)(nil)
