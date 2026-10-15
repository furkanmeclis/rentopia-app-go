// Package usecase is the payments core (TEC-707 / TEC-708,
// docs/design/payments.md): pay-by-link links, hosted checkouts on the
// organization's own provider account, verification, refunds, branch
// collections and the reservation port.
//
// The one rule: a payment is confirmed only by a provider retrieve —
// directly (staff verify, the pay page poll, the reconciliation sweep) or
// after a webhook whose signature verified with the organization's secret.
// The retrieved order reference, amount, currency and token must match the
// payment row of the organization; the browser return is never proof.
// Confirmation is idempotent: pending → succeeded happens once (conditional
// update, unique provider payment id), the reservation is told once
// (ClaimPaymentApply), a webhook delivered again is one integration job.
package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound            = errors.New("payment not found")
	ErrLinkNotFound        = errors.New("payment link not found")
	ErrReservationNotFound = errors.New("reservation not found")
	// ErrNoAccount: the organization has no active payment account.
	ErrNoAccount = errors.New("no active payment account; connect one under Settings → Integrations")
	// ErrUnknownProvider: no adapter for the provider.
	ErrUnknownProvider = errors.New("unknown payment provider")
	// ErrBadWebhook: the notification could not be read.
	ErrBadWebhook = errors.New("malformed webhook")
	// ErrUnknownPayment: the notification names no payment of ours.
	ErrUnknownPayment = errors.New("the notification names no known payment")
	// ErrInvalidSignature: the notification signature did not verify.
	ErrInvalidSignature = errors.New("invalid webhook signature")
	// ErrMismatch: the provider's answer does not match our payment row.
	ErrMismatch = errors.New("the provider result does not match the payment")
)

// StateError answers 409 INVALID_STATE.
type StateError struct{ Code, Message string }

func (e *StateError) Error() string { return e.Message }

func stateErr(code, msg string) error { return &StateError{Code: code, Message: msg} }

// Detail is a field validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError answers 400.
type ValidationError struct{ Details []Detail }

func (e *ValidationError) Error() string {
	if len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func validation(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// Accounts is the integration accounts port.
type Accounts interface {
	ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error)
	AccountByID(ctx context.Context, orgID, id int64) (integrations.Account, error)
}

// Service implements payments.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	accounts  Accounts
	providers map[string]payments.Provider
	recorder  resmodel.PaymentRecorder
	baseURL   string
	events    resourceevents.Publisher
	log       *slog.Logger
	now       func() time.Time
}

// Config wires the service.
type Config struct {
	// PublicBaseURL is the browser origin of the pay page (the Next.js app,
	// PUBLIC_FRONTEND_URL): /pay/{token} and its provider callback.
	PublicBaseURL string
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries, accounts Accounts, recorder resmodel.PaymentRecorder, cfg Config, providers ...payments.Provider) *Service {
	s := &Service{
		pool: pool, q: q, accounts: accounts, recorder: recorder, providers: map[string]payments.Provider{},
		baseURL: strings.TrimRight(cfg.PublicBaseURL, "/"), events: resourceevents.Nop{}, log: slog.Default(), now: time.Now,
	}
	for _, p := range providers {
		if p != nil {
			s.providers[p.Provider()] = p
		}
	}
	return s
}

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(p resourceevents.Publisher) {
	if p != nil {
		s.events = p
	}
}

// SetLogger replaces the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("payments: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) publish(ctx context.Context, resource, action string, ids ...uuid.UUID) {
	if len(ids) == 0 {
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	s.events.Publish(ctx, resource, action, out...)
}

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func actorOf(id pgtype.UUID, name, surname, email pgtype.Text) *paymodel.Actor {
	if !id.Valid {
		return nil
	}
	return &paymodel.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func uniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func m(n pgtype.Numeric) money.Money { return money.FromNumericOrZero(n) }

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilUUIDs(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}

// newToken is a pay-by-link bearer: 32 random bytes, base64url.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// LinkURL is the public page of a link token.
func (s *Service) LinkURL(token string) string { return s.baseURL + "/pay/" + token }

func (s *Service) provider(code string) (payments.Provider, error) {
	p, ok := s.providers[code]
	if !ok {
		return nil, ErrUnknownProvider
	}
	return p, nil
}

// reservationFor loads a reservation in the caller's branch scope.
func (s *Service) reservationFor(ctx context.Context, orgID int64, id uuid.UUID) (db.GetReservationCoreRow, error) {
	c, err := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return c, ErrReservationNotFound
	}
	if err != nil {
		return c, err
	}
	if orgctx.CheckBranch(ctx, c.PickupBranchID) != nil && orgctx.CheckBranch(ctx, c.ReturnBranchID) != nil {
		return c, ErrReservationNotFound
	}
	return c, nil
}

func inScope(ctx context.Context, pickup, ret int64) bool {
	return orgctx.CheckBranch(ctx, pickup) == nil || orgctx.CheckBranch(ctx, ret) == nil
}

// event writes a reservation timeline entry.
func event(ctx context.Context, q *db.Queries, orgID, reservationID int64, action, reason string, data map[string]any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		raw = []byte("{}")
	}
	return q.InsertReservationEvent(ctx, db.InsertReservationEventParams{
		OrganizationID: orgID, ReservationID: reservationID, Action: action, Reason: reason, Data: raw, ActorID: actorID(ctx),
	})
}

// ------------------------------------------------------------ verification

// verify reads the provider result of a pending checkout and records it.
// via names who asked (webhook / retrieve). Returns the payment status.
func (s *Service) verify(ctx context.Context, orgID, paymentID int64, via string) (string, error) {
	p, err := s.q.GetPaymentByID(ctx, db.GetPaymentByIDParams{OrganizationID: orgID, ID: paymentID})
	if notFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if p.Status != paymodel.StatusPending {
		return p.Status, s.apply(ctx, orgID, paymentID)
	}
	if !p.ProviderToken.Valid || !p.IntegrationAccountID.Valid {
		return p.Status, nil // checkout never opened
	}
	prov, err := s.provider(p.Provider)
	if err != nil {
		return "", err
	}
	acc, err := s.accounts.AccountByID(ctx, orgID, p.IntegrationAccountID.Int64)
	if err != nil {
		return "", err
	}
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	res, err := prov.Retrieve(rctx, acc, payments.RetrieveRequest{Token: p.ProviderToken.String, ConversationID: p.ConversationID})
	if err != nil {
		return p.Status, err
	}
	now := s.clock()
	switch res.Status {
	case payments.StatusPending:
		return p.Status, nil
	case payments.StatusFailed:
		reason := res.FailureReason
		if reason == "" {
			reason = "payment_failed"
		}
		if _, err := s.q.FailPayment(ctx, db.FailPaymentParams{
			OrganizationID: orgID, ID: p.ID, ProviderStatus: res.ProviderStatus, FailureReason: reason,
			VerifiedVia: via, VerifiedAt: ts(now),
		}); err != nil {
			return "", err
		}
		s.publishPayment(ctx, orgID, p)
		return paymodel.StatusFailed, nil
	}
	// Succeeded: the answer must be about THIS payment of THIS organization.
	amount := m(p.Amount)
	problems := []string{}
	if res.PaymentID == "" {
		problems = append(problems, "payment id missing")
	}
	if res.ConversationID != "" && res.ConversationID != p.ConversationID {
		problems = append(problems, "conversation id")
	}
	if res.BasketID != p.Uuid.String() {
		problems = append(problems, "order reference")
	}
	if res.Token != "" && res.Token != p.ProviderToken.String {
		problems = append(problems, "token")
	}
	if !strings.EqualFold(res.Currency, p.Currency) {
		problems = append(problems, "currency")
	}
	if !res.Price.Equal(amount) || res.PaidPrice.Cmp(amount) < 0 {
		problems = append(problems, "amount")
	}
	if len(problems) > 0 {
		reason := "verification_mismatch: " + strings.Join(problems, ", ")
		s.log.Error("payment_verification_mismatch", "organization_id", orgID, "payment", p.Uuid, "problems", problems)
		if _, err := s.q.FailPayment(ctx, db.FailPaymentParams{
			OrganizationID: orgID, ID: p.ID, ProviderStatus: res.ProviderStatus, FailureReason: reason,
			VerifiedVia: via, VerifiedAt: ts(now),
		}); err != nil {
			return "", err
		}
		s.publishPayment(ctx, orgID, p)
		return paymodel.StatusFailed, ErrMismatch
	}
	data, _ := json.Marshal(res.Data)
	commission := res.Commission
	if commission.IsNegative() {
		commission = money.Zero()
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.ConfirmPayment(ctx, db.ConfirmPaymentParams{
			OrganizationID: orgID, ID: p.ID, ProviderPaymentID: pgtype.Text{String: res.PaymentID, Valid: true},
			ProviderStatus: res.ProviderStatus, GrossAmount: res.PaidPrice.Numeric(), CommissionAmount: commission.Numeric(),
			NetAmount: res.PaidPrice.Sub(commission).Numeric(), ProviderData: data, VerifiedVia: via, VerifiedAt: ts(now),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return nil // confirmed by a concurrent delivery
		}
		if p.PaymentLinkID.Valid {
			if _, err := q.MarkPaymentLinkPaid(ctx, db.MarkPaymentLinkPaidParams{
				OrganizationID: orgID, ID: p.PaymentLinkID.Int64, PaidAt: ts(now),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if uniqueViolation(err) {
		// The provider payment id is recorded on another row already:
		// one payment per provider payment id (RT-A16).
		s.log.Error("payment_duplicate_provider_id", "organization_id", orgID, "payment", p.Uuid, "provider_payment_id", res.PaymentID)
		return p.Status, ErrMismatch
	}
	if err != nil {
		return "", err
	}
	s.publishPayment(ctx, orgID, p)
	return paymodel.StatusSucceeded, s.apply(ctx, orgID, p.ID)
}

func (s *Service) publishPayment(ctx context.Context, orgID int64, p db.GetPaymentByIDRow) {
	s.publish(ctx, paymodel.ResourcePayments, "updated", p.Uuid)
	if p.PaymentLinkID.Valid {
		s.publish(ctx, paymodel.ResourceLinks, "updated")
	}
	_ = orgID
}

// apply tells the reservation about a confirmed payment exactly once.
func (s *Service) apply(ctx context.Context, orgID, paymentID int64) error {
	n, err := s.q.ClaimPaymentApply(ctx, db.ClaimPaymentApplyParams{
		OrganizationID: orgID, ID: paymentID, StaleBefore: ts(s.clock().Add(-2 * time.Minute)),
	})
	if err != nil || n != 1 {
		return err
	}
	p, err := s.q.GetPaymentByID(ctx, db.GetPaymentByIDParams{OrganizationID: orgID, ID: paymentID})
	if err != nil {
		return err
	}
	outcome, err := s.recount(ctx, orgID, p, p.Uuid, m(p.Amount), p.Provider)
	if err != nil {
		// Leave the claim to lapse: the next delivery / poll / sweep retries.
		return err
	}
	return s.q.MarkPaymentApplied(ctx, db.MarkPaymentAppliedParams{
		OrganizationID: orgID, ID: paymentID, AppliedAt: ts(s.clock()), Outcome: outcome,
	})
}

// recount sends the reservation its received total (refunds deducted).
func (s *Service) recount(ctx context.Context, orgID int64, p db.GetPaymentByIDRow, payment uuid.UUID, amount money.Money, source string) (string, error) {
	if !p.ReservationID.Valid || s.recorder == nil {
		return resmodel.OutcomeRecorded, nil
	}
	sums, err := s.q.SumReservationPayments(ctx, db.SumReservationPaymentsParams{OrganizationID: orgID, ReservationID: p.ReservationID})
	if err != nil {
		return "", err
	}
	resUUID, err := s.reservationUUID(ctx, orgID, p.ReservationID.Int64)
	if err != nil {
		return "", err
	}
	return s.recorder.RecordPayment(ctx, orgID, resUUID, resmodel.PaymentUpdate{
		Paid: m(sums.Paid), Refunded: m(sums.Refunded).IsPositive(), Payment: payment, Amount: amount, Source: source,
	})
}

func (s *Service) reservationUUID(ctx context.Context, orgID, id int64) (uuid.UUID, error) {
	r, err := s.q.GetReservationRefByID(ctx, db.GetReservationRefByIDParams{OrganizationID: orgID, ID: id})
	if err != nil {
		return uuid.Nil, err
	}
	return r.Uuid, nil
}

// ReconcilePending is the sweep (worker, every minute with the hold
// expiry): checkouts pending for more than two minutes and less than two
// days are retrieved from the provider, so a lost webhook never leaves a
// paid reservation unconfirmed. Returns the number of payments that moved.
func (s *Service) ReconcilePending(ctx context.Context) (int, error) {
	now := s.clock()
	older, newer := ts(now.Add(-2*time.Minute)), ts(now.Add(-48*time.Hour))
	orgs, err := s.q.ListPendingPaymentOrganizations(ctx, db.ListPendingPaymentOrganizationsParams{OlderThan: older, NewerThan: newer})
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, orgID := range orgs {
		octx := orgctx.WithOrganizationID(ctx, orgID)
		ids, err := s.q.ListPendingPayments(octx, db.ListPendingPaymentsParams{OrganizationID: orgID, OlderThan: older, NewerThan: newer})
		if err != nil {
			return moved, err
		}
		for _, id := range ids {
			p, err := s.q.GetPayment(octx, db.GetPaymentParams{OrganizationID: orgID, Uuid: id})
			if err != nil {
				continue
			}
			status, err := s.verify(octx, orgID, p.ID, "retrieve")
			if err != nil {
				s.log.Warn("payment_reconcile_failed", "organization_id", orgID, "payment", id, "error", err)
				continue
			}
			if status != paymodel.StatusPending {
				moved++
			}
		}
	}
	return moved, nil
}
