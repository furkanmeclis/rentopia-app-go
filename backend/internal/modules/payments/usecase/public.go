package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// resolveToken finds a link by its public token (organization included).
func (s *Service) resolveToken(ctx context.Context, token string) (context.Context, int64, db.GetPaymentLinkPublicRow, error) {
	token = strings.TrimSpace(token)
	if len(token) < 20 || len(token) > 64 {
		return ctx, 0, db.GetPaymentLinkPublicRow{}, ErrLinkNotFound
	}
	ref, err := s.q.ResolvePaymentLinkToken(ctx, token)
	if notFound(err) {
		return ctx, 0, db.GetPaymentLinkPublicRow{}, ErrLinkNotFound
	}
	if err != nil {
		return ctx, 0, db.GetPaymentLinkPublicRow{}, err
	}
	octx := orgctx.WithOrganizationID(ctx, ref.OrganizationID)
	row, err := s.q.GetPaymentLinkPublic(octx, db.GetPaymentLinkPublicParams{OrganizationID: ref.OrganizationID, ID: ref.ID})
	if notFound(err) {
		return ctx, 0, row, ErrLinkNotFound
	}
	return octx, ref.OrganizationID, row, err
}

// PublicLink answers the pay page; verify=true asks the provider about a
// pending checkout first (the page poll; rate-limited by the handler).
func (s *Service) PublicLink(ctx context.Context, token string, verify bool) (paymodel.PublicLink, error) {
	octx, orgID, row, err := s.resolveToken(ctx, token)
	if err != nil {
		return paymodel.PublicLink{}, err
	}
	if verify && row.Status != paymodel.LinkCancelled {
		if p, err := s.q.GetLatestPendingLinkPayment(octx, db.GetLatestPendingLinkPaymentParams{
			OrganizationID: orgID, PaymentLinkID: pgtype.Int8{Int64: row.ID, Valid: true},
		}); err == nil {
			if _, err := s.verify(octx, orgID, p.ID, "retrieve"); err != nil && !errors.Is(err, ErrMismatch) {
				s.log.Warn("pay_page_verify_failed", "organization_id", orgID, "payment", p.Uuid, "error", err)
			}
			if row, err = s.q.GetPaymentLinkPublic(octx, db.GetPaymentLinkPublicParams{OrganizationID: orgID, ID: row.ID}); err != nil {
				return paymodel.PublicLink{}, err
			}
		}
	}
	out := paymodel.PublicLink{
		OrganizationName: row.OrganizationName, Reference: row.ReservationReference, ProductTitle: row.ProductTitle,
		StartsAt: row.ReservationStartsAt.Time.UTC(), EndsAt: row.ReservationEndsAt.Time.UTC(), Amount: m(row.Amount),
		Currency: row.Currency, ExpiresAt: row.ExpiresAt.Time.UTC(),
	}
	status := s.effectiveLinkStatus(octx, orgID, row.ID, row.Status, row.ExpiresAt.Time)
	latest, lerr := s.q.GetLatestLinkPayment(octx, db.GetLatestLinkPaymentParams{
		OrganizationID: orgID, PaymentLinkID: pgtype.Int8{Int64: row.ID, Valid: true},
	})
	switch {
	case status == paymodel.LinkPaid:
		out.State = paymodel.PublicPaid
	case lerr == nil && latest.Status == paymodel.StatusPending && status == paymodel.LinkActive:
		out.State = paymodel.PublicProcessing
	case status == paymodel.LinkExpired:
		out.State = paymodel.PublicExpired
	case status == paymodel.LinkCancelled:
		out.State = paymodel.PublicCancelled
	case lerr == nil && latest.Status == paymodel.StatusFailed:
		out.State = paymodel.PublicFailed
		out.FailureReason = "payment_failed"
	default:
		out.State = paymodel.PublicReady
	}
	if lerr == nil && out.State == paymodel.PublicPaid {
		out.Result = latest.Outcome
		if out.Result == "applying" {
			out.Result = ""
		}
	}
	return out, nil
}

// StartCheckout opens a hosted checkout for an active link on the
// organization's active payment account and returns the provider page.
func (s *Service) StartCheckout(ctx context.Context, token, ip, locale string) (paymodel.CheckoutStart, error) {
	octx, orgID, row, err := s.resolveToken(ctx, token)
	if err != nil {
		return paymodel.CheckoutStart{}, err
	}
	switch s.effectiveLinkStatus(octx, orgID, row.ID, row.Status, row.ExpiresAt.Time) {
	case paymodel.LinkPaid:
		return paymodel.CheckoutStart{}, stateErr("link_paid", "this link is already paid")
	case paymodel.LinkExpired:
		return paymodel.CheckoutStart{}, stateErr("link_expired", "this link has expired")
	case paymodel.LinkCancelled:
		return paymodel.CheckoutStart{}, stateErr("link_cancelled", "this link was cancelled")
	}
	acc, err := s.accounts.ActiveAccount(octx, orgID, integrations.KindPayment)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return paymodel.CheckoutStart{}, ErrNoAccount
	}
	if err != nil {
		return paymodel.CheckoutStart{}, err
	}
	prov, err := s.provider(acc.Provider)
	if err != nil {
		return paymodel.CheckoutStart{}, err
	}
	view, err := s.q.GetReservationView(octx, db.GetReservationViewParams{OrganizationID: orgID, Uuid: row.ReservationUuid})
	if err != nil {
		return paymodel.CheckoutStart{}, err
	}
	if resmodel.IsTerminal(view.Status) && view.Status != resmodel.StatusExpired {
		return paymodel.CheckoutStart{}, stateErr("reservation_not_payable", "the reservation can no longer be paid")
	}
	id := uuid.New()
	amount := m(row.Amount)
	paymentID, err := s.q.InsertPayment(octx, db.InsertPaymentParams{
		Uuid: id, OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: row.ReservationID, Valid: true},
		PaymentLinkID: pgtype.Int8{Int64: row.ID, Valid: true}, IntegrationAccountID: pgtype.Int8{Int64: acc.ID, Valid: true},
		Provider: acc.Provider, Method: paymodel.MethodCheckoutForm, Status: paymodel.StatusPending, Currency: row.Currency,
		Amount: amount.Numeric(), GrossAmount: amount.Numeric(), NetAmount: amount.Numeric(), ConversationID: id.String(),
	})
	if err != nil {
		return paymodel.CheckoutStart{}, err
	}
	name, surname := buyerName(view)
	if locale != "en" {
		locale = "tr"
	}
	co, err := prov.CreateCheckout(octx, acc, payments.CheckoutRequest{
		ConversationID: id.String(), BasketID: id.String(), Amount: amount, Currency: row.Currency,
		CallbackURL: s.LinkURL(row.Token) + "/callback", Locale: locale,
		ItemID: row.ReservationReference, ItemName: row.ProductTitle, ItemCategory: "Araç kiralama",
		Buyer: payments.Buyer{
			ID: view.CustomerUuid.String(), Name: name, Surname: surname, Email: view.CustomerEmail, Phone: view.CustomerPhone,
			// The national id is not sent (encrypted at rest, not needed
			// for a hosted 3DS checkout): iyzico's documented placeholder.
			IdentityNumber: "11111111111", Address: "Türkiye", City: "Istanbul", Country: "Turkey", IP: ip,
		},
	})
	if err != nil {
		_, _ = s.q.FailPayment(octx, db.FailPaymentParams{
			OrganizationID: orgID, ID: paymentID, ProviderStatus: "", FailureReason: "checkout_failed: " + err.Error(),
			VerifiedVia: "retrieve", VerifiedAt: ts(s.clock()),
		})
		return paymodel.CheckoutStart{}, &ProviderFailure{Err: err}
	}
	if _, err := s.q.SetPaymentCheckout(octx, db.SetPaymentCheckoutParams{
		OrganizationID: orgID, ID: paymentID, ProviderToken: pgtype.Text{String: co.Token, Valid: true},
	}); err != nil {
		return paymodel.CheckoutStart{}, err
	}
	s.publish(octx, paymodel.ResourcePayments, "created", id)
	return paymodel.CheckoutStart{PaymentPageURL: co.PaymentPageURL}, nil
}

func buyerName(v db.GetReservationViewRow) (string, string) {
	name, surname := strings.TrimSpace(v.CustomerFirstName), strings.TrimSpace(v.CustomerLastName)
	if name == "" && surname == "" {
		title := strings.TrimSpace(v.CustomerCompanyTitle)
		if title == "" {
			title = "Müşteri"
		}
		parts := strings.SplitN(title, " ", 2)
		name = parts[0]
		surname = name
		if len(parts) == 2 {
			surname = parts[1]
		}
	}
	if surname == "" {
		surname = name
	}
	return name, surname
}

// Webhook handles a provider notification: parse → find our payment by
// the echoed conversation id (which names the organization) → verify the
// signature with that organization's secret → one integration job per
// notification key → provider retrieve and confirm (verify). The same
// notification delivered again is answered as a duplicate.
func (s *Service) Webhook(ctx context.Context, providerCode string, header http.Header, body []byte) (paymodel.WebhookResult, error) {
	prov, err := s.provider(providerCode)
	if err != nil {
		return paymodel.WebhookResult{}, err
	}
	hook, err := prov.ParseWebhook(header, body)
	if err != nil {
		return paymodel.WebhookResult{}, ErrBadWebhook
	}
	refs, err := s.q.ResolvePaymentConversation(ctx, hook.ConversationID)
	if err != nil {
		return paymodel.WebhookResult{}, err
	}
	if len(refs) != 1 {
		return paymodel.WebhookResult{}, ErrUnknownPayment
	}
	orgID := refs[0].OrganizationID
	octx := orgctx.WithOrganizationID(ctx, orgID)
	p, err := s.q.GetPaymentByID(octx, db.GetPaymentByIDParams{OrganizationID: orgID, ID: refs[0].ID})
	if err != nil {
		return paymodel.WebhookResult{}, err
	}
	if p.Provider != providerCode || !p.IntegrationAccountID.Valid {
		return paymodel.WebhookResult{}, ErrUnknownPayment
	}
	acc, err := s.accounts.AccountByID(octx, orgID, p.IntegrationAccountID.Int64)
	if err != nil {
		return paymodel.WebhookResult{}, err
	}
	if err := prov.VerifyWebhook(acc, hook, header, body); err != nil {
		s.log.Warn("payment_webhook_signature_invalid", "organization_id", orgID, "payment", p.Uuid)
		return paymodel.WebhookResult{}, ErrInvalidSignature
	}
	if hook.Token != "" && p.ProviderToken.Valid && hook.Token != p.ProviderToken.String {
		return paymodel.WebhookResult{}, ErrInvalidSignature
	}
	subject := hook.PaymentID
	if subject == "" {
		subject = hook.Token
	}
	job, err := s.q.UpsertIntegrationJob(octx, db.UpsertIntegrationJobParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, IntegrationAccountID: pgtype.Int8{Int64: acc.ID, Valid: true},
		Direction: "inbound", Provider: providerCode, Kind: "payment.webhook",
		IdempotencyKey: fmt.Sprintf("org:%d:webhook:%s:%s:%s", orgID, subject, hook.EventType, strings.ToUpper(hook.Status)),
		Payload: mustJSON(map[string]any{
			"event_type": hook.EventType, "status": hook.Status, "provider_payment_id": hook.PaymentID,
			"conversation_id": hook.ConversationID, "reference_code": hook.ReferenceCode,
		}),
		RefType: "payment", RefUuid: pgtype.UUID{Bytes: p.Uuid, Valid: true},
	})
	if err != nil {
		return paymodel.WebhookResult{}, err
	}
	if !job.Inserted && (job.Status == "succeeded" || job.Status == "ignored") {
		return paymodel.WebhookResult{Duplicate: true, Status: p.Status}, nil
	}
	n, err := s.q.ClaimIntegrationJob(octx, db.ClaimIntegrationJobParams{OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: job.ID})
	if err != nil {
		return paymodel.WebhookResult{}, err
	}
	if n != 1 {
		// Another delivery is processing the same notification.
		return paymodel.WebhookResult{Duplicate: true, Status: p.Status}, nil
	}
	status, verr := s.verify(octx, orgID, p.ID, "webhook")
	finish := db.FinishIntegrationJobParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: job.ID, Status: "succeeded",
		Result: mustJSON(map[string]any{"payment_status": status}),
	}
	if verr != nil && !errors.Is(verr, ErrMismatch) {
		finish.Status, finish.LastError = "failed", verr.Error()
	}
	if errors.Is(verr, ErrMismatch) {
		finish.LastError = verr.Error()
	}
	if err := s.q.FinishIntegrationJob(octx, finish); err != nil {
		return paymodel.WebhookResult{}, err
	}
	if finish.Status == "failed" {
		return paymodel.WebhookResult{}, verr
	}
	return paymodel.WebhookResult{Status: status, Duplicate: !job.Inserted}, nil
}
