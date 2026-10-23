package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Restore idempotency (TEC-962, docs/design/payments.md §3.8): a refund
// made after the backup is missing from the restored ledger, and a
// checkout created after it is unknown to the webhook. The refund guard
// asks the provider before refunding; orphan notifications are kept for a
// person to match; the report lists our payments beside the provider's
// view.

var (
	// ErrOrphanNotFound: unknown orphan (or another organization's).
	ErrOrphanNotFound = errors.New("orphan notification not found")
	// ErrItemNotFound: unknown reconciliation item.
	ErrItemNotFound = errors.New("reconciliation item not found")
)

// CodeRefundStateMismatch is the handler error code of a refused refund.
const CodeRefundStateMismatch = "REFUND_STATE_MISMATCH"

// RefundGuard is the decision of the refund guard.
type RefundGuard string

// Refund guard decisions.
const (
	// GuardProceed: the provider's refunds match our ledger.
	GuardProceed RefundGuard = "proceed"
	// GuardSkipped: the provider cannot report refunds (no lookup).
	GuardSkipped RefundGuard = "skipped"
	// GuardMismatch: the provider shows refunds our ledger lacks.
	GuardMismatch RefundGuard = "mismatch"
	// GuardUnverified: the lookup failed; the refund goes ahead as before
	// the guard existed and an "unverified" item is kept for staff.
	GuardUnverified RefundGuard = "unverified"
)

// DecideRefund is the refund guard's decision table:
//
//	lookup unsupported (adapter or ErrUnsupported)  → skipped (refund as before)
//	lookup error                                    → unverified (refund as before; warning, Sentry, "unverified" item)
//	provider refunded > our refunded                → mismatch (refuse, reconciliation item)
//	provider refunded ≤ our refunded                → proceed
//
// "Our refunded" is payments.refunded_amount (succeeded refunds only), so
// a refund that reached the provider but whose row stayed pending (a crash
// mid-refund) is a mismatch too.
func DecideRefund(ourRefunded money.Money, state *payments.RefundState, lookupErr error) RefundGuard {
	switch {
	case errors.Is(lookupErr, integrations.ErrUnsupported):
		return GuardSkipped
	case lookupErr != nil:
		return GuardUnverified
	case state == nil:
		return GuardSkipped
	case state.Refunded.Cmp(ourRefunded) > 0:
		return GuardMismatch
	}
	return GuardProceed
}

// RefundGuardError refuses a refund the provider already shows: 409
// REFUND_STATE_MISMATCH (with the reconciliation item).
type RefundGuardError struct {
	Code             string
	Message          string
	ItemUUID         *uuid.UUID
	OurRefunded      money.Money
	ProviderRefunded money.Money
}

func (e *RefundGuardError) Error() string { return e.Message }

// lookupRefunds asks the provider what it has refunded on a payment.
func (s *Service) lookupRefunds(ctx context.Context, prov payments.Provider, acc integrations.Account, providerPaymentID, conversationID string) (*payments.RefundState, error) {
	lookup, ok := prov.(payments.RefundLookup)
	if !ok {
		return nil, integrations.ErrUnsupported
	}
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	st, err := lookup.RefundState(rctx, acc, payments.RefundStateRequest{PaymentID: providerPaymentID, ConversationID: conversationID})
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// guardRefund runs before a provider refund: a refund the provider already
// shows and our (restored) ledger lacks is never made again; a
// reconciliation item records it instead. When the provider's refunds
// cannot be read (e.g. the reporting call is not available for the
// merchant), the refund is not blocked: it goes ahead as before the guard
// existed, with a warning, a Sentry event and an open "unverified" item so
// staff can check it later.
func (s *Service) guardRefund(ctx context.Context, orgID int64, r db.GetPaymentRow, prov payments.Provider, acc integrations.Account,
	amount money.Money, reason string) error {
	ours := m(r.RefundedAmount)
	st, err := s.lookupRefunds(ctx, prov, acc, r.ProviderPaymentID.String, r.ConversationID)
	switch DecideRefund(ours, st, err) {
	case GuardSkipped, GuardProceed:
		return nil
	case GuardUnverified:
		s.unverifiedRefund(ctx, orgID, r, ours, amount, reason, err)
		return nil
	}
	refunds := make([]map[string]any, 0, len(st.Refunds))
	for _, f := range st.Refunds {
		refunds = append(refunds, map[string]any{"refund_id": f.RefundID, "amount": f.Amount.String(), "kind": f.Kind})
	}
	data := mustJSON(map[string]any{
		"requested_amount": amount.String(), "requested_reason": reason, "provider_status": st.ProviderStatus,
		"provider_refunds": refunds,
	})
	item, err := s.q.UpsertRefundGuardItem(ctx, db.UpsertRefundGuardItemParams{
		OrganizationID: orgID, PaymentID: r.ID, Kind: paymodel.ItemKindRefundMismatch, Currency: r.Currency, OurRefunded: ours.Numeric(),
		ProviderRefunded: st.Refunded.Numeric(), Data: data, CreatedBy: actorID(ctx),
	})
	if err != nil {
		return err
	}
	s.log.Error("payment_refund_state_mismatch", "organization_id", orgID, "payment", r.Uuid,
		"our_refunded", ours.String(), "provider_refunded", st.Refunded.String())
	return &RefundGuardError{
		Code: CodeRefundStateMismatch, ItemUUID: &item, OurRefunded: ours, ProviderRefunded: st.Refunded,
		Message: fmt.Sprintf("the provider already shows %s refunded, our records %s; nothing was refunded — reconcile first",
			st.Refunded.String(), ours.String()),
	}
}

// unverifiedRefund records that a refund went ahead without the provider's
// refund state (fail-soft: the refund never depends on it): a warning, a
// Sentry event (throttled per payment) and one open "unverified" item per
// payment.
func (s *Service) unverifiedRefund(ctx context.Context, orgID int64, r db.GetPaymentRow, ours, amount money.Money, reason string, lookupErr error) {
	s.log.Warn("payment_refund_guard_unverified", "organization_id", orgID, "payment", r.Uuid, "provider", r.Provider,
		"error", lookupErr)
	if observability.Enabled() && observability.Allow("payment-refund-unverified:"+r.Uuid.String(), time.Hour) {
		observability.CaptureError(ctx, fmt.Errorf("payments: refund state unverified: %w", lookupErr), observability.Options{
			Level:       sentry.LevelWarning,
			Transaction: "payments refund guard",
			Fingerprint: []string{"payment-refund-guard-unverified", r.Provider},
			Tags: map[string]string{
				"payment_uuid": r.Uuid.String(), "provider": r.Provider, "organization_id": strconv.FormatInt(orgID, 10),
			},
		})
	}
	data := mustJSON(map[string]any{
		"requested_amount": amount.String(), "requested_reason": reason, "lookup_error": trimTo(lookupErr.Error(), 500),
	})
	if _, err := s.q.UpsertRefundGuardItem(ctx, db.UpsertRefundGuardItemParams{
		OrganizationID: orgID, PaymentID: r.ID, Kind: paymodel.ItemKindUnverified, Currency: r.Currency, OurRefunded: ours.Numeric(),
		ProviderRefunded: money.Zero().Numeric(), Data: data, CreatedBy: actorID(ctx),
	}); err != nil {
		s.log.Warn("payment_refund_unverified_item_failed", "organization_id", orgID, "payment", r.Uuid, "error", err)
	}
}

// ---------------------------------------------------------------- orphans

// orphanVerifyLimit caps the accounts an orphan signature is tried on.
const orphanVerifyLimit = 500

// orphan keeps a notification that names no payment of ours. The
// organization is the one whose account secret verifies the signature
// (the notification is then authentic); otherwise the row is
// platform-level and the answer stays 404. Deposit pre-authorizations are
// not orphans (unchanged: 404).
func (s *Service) orphan(ctx context.Context, prov payments.Provider, providerCode string, hook payments.Webhook,
	header http.Header, body []byte) (paymodel.WebhookResult, error) {
	if id, err := uuid.Parse(hook.ConversationID); err == nil {
		if _, err := s.q.FindSecurityDepositByConversation(ctx, id); err == nil {
			return paymodel.WebhookResult{}, ErrUnknownPayment
		}
	}
	var orgID, accountID int64
	cands, err := s.q.ListPaymentAccountsForProvider(ctx, db.ListPaymentAccountsForProviderParams{Provider: providerCode, RowLimit: orphanVerifyLimit})
	if err != nil {
		return paymodel.WebhookResult{}, err
	}
	for _, c := range cands {
		octx := orgctx.WithOrganizationID(ctx, c.OrganizationID)
		acc, err := s.accounts.AccountByID(octx, c.OrganizationID, c.ID)
		if err != nil {
			continue
		}
		if prov.VerifyWebhook(acc, hook, header, body) == nil {
			orgID, accountID = c.OrganizationID, c.ID
			break
		}
	}
	payload := body
	var probe map[string]any
	if json.Unmarshal(body, &probe) != nil {
		payload = []byte("{}")
	}
	subject := hook.PaymentID
	if subject == "" {
		subject = hook.Token
	}
	if subject == "" {
		subject = hook.ConversationID
	}
	key := fmt.Sprintf("%s:%s:%s", subject, hook.EventType, strings.ToUpper(hook.Status))
	if orgID == 0 {
		// An unauthenticated delivery never merges into a verified row.
		key = "unverified:" + key
	}
	p := db.UpsertPaymentWebhookOrphanParams{
		Provider: providerCode, DedupeKey: trimTo(key, 300),
		EventType: trimTo(hook.EventType, 64), ProviderStatus: trimTo(hook.Status, 32), ProviderPaymentID: trimTo(hook.PaymentID, 64),
		ConversationID: trimTo(hook.ConversationID, 64), ProviderToken: trimTo(hook.Token, 255),
		ReferenceCode: trimTo(hook.ReferenceCode, 64), MerchantID: trimTo(hook.MerchantID, 64), SignatureVerified: orgID > 0,
		Payload: payload,
	}
	if orgID > 0 {
		p.OrganizationID = pgtype.Int8{Int64: orgID, Valid: true}
		p.IntegrationAccountID = pgtype.Int8{Int64: accountID, Valid: true}
	}
	row, err := s.q.UpsertPaymentWebhookOrphan(ctx, p)
	if err != nil {
		return paymodel.WebhookResult{}, err
	}
	s.log.Warn("payment_webhook_orphan", "organization_id", orgID, "orphan", row.Uuid, "provider_payment_id", hook.PaymentID,
		"verified", orgID > 0, "deliveries_inserted", row.Inserted)
	if orgID == 0 {
		// Not authentic for any account we know: kept for the platform,
		// answered like before.
		return paymodel.WebhookResult{}, ErrUnknownPayment
	}
	s.publish(orgctx.WithOrganizationID(ctx, orgID), paymodel.ResourceOrphans, "created", row.Uuid)
	return paymodel.WebhookResult{Duplicate: !row.Inserted, Status: row.Status, Orphan: true}, nil
}

func trimTo(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}

// ListOrphans returns a page of the organization's orphan notifications.
func (s *Service) ListOrphans(ctx context.Context, orgID int64, f paymodel.OrphanFilter) ([]paymodel.Orphan, int64, error) {
	p := db.ListPaymentWebhookOrphansParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Statuses: nilIfEmpty(f.Statuses), SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	if f.Since != nil {
		p.Since = ts(*f.Since)
	}
	rows, err := s.q.ListPaymentWebhookOrphans(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]paymodel.Orphan, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		o := paymodel.Orphan{
			UUID: r.Uuid, Provider: r.Provider, EventType: r.EventType, ProviderStatus: r.ProviderStatus,
			ProviderPaymentID: r.ProviderPaymentID, ConversationID: r.ConversationID, ReferenceCode: r.ReferenceCode,
			SignatureVerified: r.SignatureVerified, Deliveries: r.Deliveries, Status: r.Status, AccountName: r.AccountName.String,
			MatchedBy: actorOf(r.MatchedByUuid, r.MatchedByName, r.MatchedBySurname, r.MatchedByEmail), MatchedAt: timePtr(r.MatchedAt),
			MatchNote: r.MatchNote, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if r.AccountUuid.Valid {
			id := uuid.UUID(r.AccountUuid.Bytes)
			o.AccountUUID = &id
		}
		if r.PaymentUuid.Valid {
			id := uuid.UUID(r.PaymentUuid.Bytes)
			o.PaymentUUID = &id
		}
		if r.ReservationUuid.Valid {
			o.Reservation = &paymodel.ReservationRef{UUID: r.ReservationUuid.Bytes, Reference: r.ReservationReference.String}
		}
		out = append(out, o)
	}
	return out, total, nil
}

// OrphanFacts are the references a notification carried.
type OrphanFacts struct {
	ConversationID    string
	ProviderPaymentID string
}

// CheckOrphanMatch decides whether a provider retrieve proves an orphan
// notification can become a payment of a reservation: the checkout is
// paid, it is the notified payment (payment id, conversation id), its
// order reference is one of our payment uuids (basketId) and the currency
// is the reservation's. Returns the payment uuid to recreate and the
// amount owed.
func CheckOrphanMatch(o OrphanFacts, res payments.Result, currency string) (uuid.UUID, money.Money, error) {
	if res.Status != payments.StatusSucceeded {
		return uuid.Nil, money.Zero(), stateErr("orphan_not_paid", "the provider does not show this checkout as paid")
	}
	if res.PaymentID == "" || (o.ProviderPaymentID != "" && res.PaymentID != o.ProviderPaymentID) {
		return uuid.Nil, money.Zero(), stateErr("orphan_mismatch", "the provider answered for another payment")
	}
	if res.ConversationID != "" && res.ConversationID != o.ConversationID {
		return uuid.Nil, money.Zero(), stateErr("orphan_mismatch", "the provider answered for another order")
	}
	id, err := uuid.Parse(res.BasketID)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, money.Zero(), stateErr("orphan_unmatchable", "the checkout carries no order reference of ours; reconcile in the provider panel")
	}
	if !strings.EqualFold(res.Currency, currency) {
		return uuid.Nil, money.Zero(), stateErr("currency_mismatch", "the payment currency differs from the reservation's")
	}
	if !res.Price.IsPositive() {
		return uuid.Nil, money.Zero(), stateErr("orphan_mismatch", "the provider answered without an amount")
	}
	return id, res.Price, nil
}

// MatchOrphan attaches an orphan notification to a reservation: the
// checkout is retrieved from the provider and, when it is paid and ours
// (CheckOrphanMatch), recreated as a payment under its own order reference
// and confirmed through the usual verification (once; the reservation is
// told once). tenant.payment.collect.
func (s *Service) MatchOrphan(ctx context.Context, orgID int64, id uuid.UUID, in paymodel.MatchOrphanInput) (paymodel.Payment, error) {
	o, err := s.q.GetPaymentWebhookOrphan(ctx, db.GetPaymentWebhookOrphanParams{OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Uuid: id})
	if notFound(err) {
		return paymodel.Payment{}, ErrOrphanNotFound
	}
	if err != nil {
		return paymodel.Payment{}, err
	}
	note := strings.TrimSpace(in.Note)
	ve := &ValidationError{}
	if in.ReservationUUID == nil {
		ve.Details = append(ve.Details, Detail{"reservation_uuid", "required", "the reservation is required"})
	}
	if n := len([]rune(note)); n < 3 || n > 500 {
		ve.Details = append(ve.Details, Detail{"note", "required", "a note is required (3-500 characters)"})
	}
	if len(ve.Details) > 0 {
		return paymodel.Payment{}, ve
	}
	if o.Status != paymodel.OrphanUnmatched {
		return paymodel.Payment{}, stateErr("orphan_matched", "the notification is already matched")
	}
	c, err := s.reservationFor(ctx, orgID, *in.ReservationUUID)
	if err != nil {
		return paymodel.Payment{}, err
	}
	if !o.IntegrationAccountID.Valid {
		return paymodel.Payment{}, stateErr("orphan_unverified", "the notification was not verified with one of the organization's accounts")
	}
	if o.ProviderToken == "" {
		return paymodel.Payment{}, stateErr("orphan_no_token", "the notification carries no checkout token; reconcile in the provider panel")
	}
	prov, err := s.provider(o.Provider)
	if err != nil {
		return paymodel.Payment{}, err
	}
	acc, err := s.accounts.AccountByID(ctx, orgID, o.IntegrationAccountID.Int64)
	if err != nil {
		return paymodel.Payment{}, err
	}
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	res, err := prov.Retrieve(rctx, acc, payments.RetrieveRequest{Token: o.ProviderToken, ConversationID: o.ConversationID})
	cancel()
	if err != nil {
		return paymodel.Payment{}, &ProviderFailure{Err: err}
	}
	paymentUUID, amount, err := CheckOrphanMatch(OrphanFacts{ConversationID: o.ConversationID, ProviderPaymentID: o.ProviderPaymentID}, res, c.Currency)
	if err != nil {
		return paymodel.Payment{}, err
	}
	var paymentID int64
	err = s.tx(ctx, func(q *db.Queries) error {
		pid, err := q.InsertMatchedPayment(ctx, db.InsertMatchedPaymentParams{
			Uuid: paymentUUID, OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: c.ID, Valid: true},
			IntegrationAccountID: pgtype.Int8{Int64: acc.ID, Valid: true}, Provider: o.Provider, Currency: c.Currency,
			Amount: amount.Numeric(), ConversationID: o.ConversationID, ProviderToken: pgtype.Text{String: o.ProviderToken, Valid: true},
			Note: note, CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		paymentID = pid
		n, err := q.MarkPaymentWebhookOrphanMatched(ctx, db.MarkPaymentWebhookOrphanMatchedParams{
			PaymentID: pgtype.Int8{Int64: pid, Valid: true}, MatchedBy: actorID(ctx), Note: note,
			OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: o.ID,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("orphan_matched", "the notification is already matched")
		}
		return event(ctx, q, orgID, c.ID, "payment_matched", note, map[string]any{
			"payment_uuid": paymentUUID, "orphan_uuid": o.Uuid, "provider_payment_id": res.PaymentID, "amount": amount.String(),
		})
	})
	if uniqueViolation(err) {
		return paymodel.Payment{}, stateErr("orphan_conflict", "a payment with this order reference or token exists already")
	}
	if err != nil {
		return paymodel.Payment{}, err
	}
	if _, err := s.verify(ctx, orgID, paymentID, "retrieve"); err != nil {
		if errors.Is(err, ErrMismatch) {
			return paymodel.Payment{}, err
		}
		s.log.Warn("payment_orphan_verify_failed", "organization_id", orgID, "payment", paymentUUID, "error", err)
	}
	s.publish(ctx, paymodel.ResourcePayments, "created", paymentUUID)
	s.publish(ctx, paymodel.ResourceOrphans, "updated", o.Uuid)
	return s.Get(ctx, orgID, paymentUUID)
}

// ------------------------------------------------------ reconciliation items

func (s *Service) items(ctx context.Context, orgID int64, since time.Time) ([]paymodel.ReconciliationItem, error) {
	rows, err := s.q.ListPaymentReconciliationItems(ctx, db.ListPaymentReconciliationItemsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Since: ts(since),
	})
	if err != nil {
		return nil, err
	}
	out := make([]paymodel.ReconciliationItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, paymodel.ReconciliationItem{
			UUID: r.Uuid, Kind: r.Kind, Status: r.Status, Currency: r.Currency, OurRefunded: m(r.OurRefunded),
			ProviderRefunded: m(r.ProviderRefunded), Data: json.RawMessage(r.Data), Resolution: r.Resolution,
			ResolutionNote: r.ResolutionNote, ResolvedAt: timePtr(r.ResolvedAt), PaymentUUID: r.PaymentUuid,
			Reservation: paymodel.ReservationRef{UUID: r.ReservationUuid, Reference: r.ReservationReference},
			CreatedAt:   r.CreatedAt.Time.UTC(),
		})
	}
	return out, nil
}

// ResolveItem closes a refund reconciliation item: record_refund enters
// the refund the provider shows (and our ledger lacks) as a succeeded
// refund without calling the provider — the provider is asked again for
// the current figure first; dismiss closes it with the note only.
// tenant.payment.refund.
func (s *Service) ResolveItem(ctx context.Context, orgID int64, id uuid.UUID, in paymodel.ResolveItemInput) (paymodel.Payment, error) {
	it, err := s.q.GetPaymentReconciliationItem(ctx, db.GetPaymentReconciliationItemParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return paymodel.Payment{}, ErrItemNotFound
	}
	if err != nil {
		return paymodel.Payment{}, err
	}
	note := strings.TrimSpace(in.Note)
	ve := &ValidationError{}
	if in.Resolution != paymodel.ResolutionRecordRefund && in.Resolution != paymodel.ResolutionDismiss {
		ve.Details = append(ve.Details, Detail{"resolution", "invalid", "resolution must be record_refund or dismiss"})
	}
	if n := len([]rune(note)); n < 3 || n > 500 {
		ve.Details = append(ve.Details, Detail{"note", "required", "a note is required (3-500 characters)"})
	}
	if len(ve.Details) > 0 {
		return paymodel.Payment{}, ve
	}
	if it.Status != "open" {
		return paymodel.Payment{}, stateErr("item_resolved", "the item is resolved already")
	}
	r, err := s.row(ctx, orgID, it.PaymentUuid)
	if err != nil {
		return paymodel.Payment{}, err
	}
	var missing money.Money
	if in.Resolution == paymodel.ResolutionRecordRefund {
		prov, err := s.provider(r.Provider)
		if err != nil {
			return paymodel.Payment{}, err
		}
		acc, err := s.accounts.AccountByID(ctx, orgID, r.IntegrationAccountID.Int64)
		if err != nil {
			return paymodel.Payment{}, err
		}
		st, err := s.lookupRefunds(ctx, prov, acc, r.ProviderPaymentID.String, r.ConversationID)
		if err != nil {
			// Recording needs the provider's figure; dismiss works without it.
			return paymodel.Payment{}, &ProviderFailure{Err: err}
		}
		missing = st.Refunded.Sub(m(r.RefundedAmount))
		if left := m(r.Amount).Sub(m(r.RefundedAmount)); missing.Cmp(left) > 0 {
			missing = left
		}
	}
	reason := "provider refund recorded: " + note
	var refund db.InsertPaymentRefundRow
	err = s.tx(ctx, func(q *db.Queries) error {
		// The item is the claim: a second resolve finds it resolved.
		n, err := q.ResolvePaymentReconciliationItem(ctx, db.ResolvePaymentReconciliationItemParams{
			Resolution: in.Resolution, Note: note, ResolvedBy: actorID(ctx), OrganizationID: orgID, ID: it.ID,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("item_resolved", "the item is resolved already")
		}
		if !missing.IsPositive() {
			return nil
		}
		if refund, err = q.InsertPaymentRefund(ctx, db.InsertPaymentRefundParams{
			OrganizationID: orgID, PaymentID: r.ID, Amount: missing.Numeric(), Currency: r.Currency, Reason: reason, CreatedBy: actorID(ctx),
		}); err != nil {
			return err
		}
		return refundTx(ctx, q, orgID, r, refund.ID, refund.Uuid, missing, reason, pgtype.Text{})
	})
	if err != nil {
		return paymodel.Payment{}, err
	}
	if missing.IsPositive() {
		s.afterRefund(ctx, orgID, r, refund.ID, refund.Uuid)
	}
	return s.Get(ctx, orgID, it.PaymentUuid)
}

// ---------------------------------------------------------------- report

// Report limits.
const (
	reportRowLimit = 100
	reportBudget   = 45 * time.Second
)

// Reconcile is GET /v1/tenant/payments/reconciliation: our provider
// payments created since a time, each beside the provider's retrieve and
// refund lookup (the iyzico adapter has no list call, so payments made
// after the backup that we never recorded show up only as orphan
// notifications), plus the orphans and the refund items. At most 100
// payments per call (truncated: ask again with a later since).
func (s *Service) Reconcile(ctx context.Context, orgID int64, since time.Time) (paymodel.Reconciliation, error) {
	now := s.clock()
	out := paymodel.Reconciliation{Since: since.UTC(), GeneratedAt: now, Payments: []paymodel.ReconciliationRow{}}
	rows, err := s.q.ListProviderPaymentsSince(ctx, db.ListProviderPaymentsSinceParams{
		OrganizationID: orgID, Since: ts(since), BranchIds: orgctx.BranchScope(ctx), RowLimit: reportRowLimit + 1,
	})
	if err != nil {
		return out, err
	}
	if len(rows) > reportRowLimit {
		rows, out.Truncated = rows[:reportRowLimit], true
	}
	budget, cancel := context.WithTimeout(ctx, reportBudget)
	defer cancel()
	accounts := map[int64]integrations.Account{}
	for _, r := range rows {
		row := paymodel.ReconciliationRow{
			PaymentUUID: r.Uuid, Reservation: paymodel.ReservationRef{UUID: r.ReservationUuid, Reference: r.ReservationReference},
			Provider: r.Provider, Status: r.Status, Currency: r.Currency, Amount: m(r.Amount), RefundedAmount: m(r.RefundedAmount),
			PendingRefunds: r.PendingRefunds, ProviderPaymentID: textPtr(r.ProviderPaymentID), Mismatches: []string{},
			CreatedAt: r.CreatedAt.Time.UTC(),
		}
		if r.PendingRefunds > 0 {
			row.Mismatches = append(row.Mismatches, "pending_refund")
		}
		if budget.Err() != nil {
			out.Truncated = true
			break
		}
		s.compare(budget, orgID, r, &row, accounts)
		out.Payments = append(out.Payments, row)
	}
	orphans, _, err := s.ListOrphans(ctx, orgID, paymodel.OrphanFilter{Statuses: []string{paymodel.OrphanUnmatched}, Since: &since, SortDesc: true, Limit: 200})
	if err != nil {
		return out, err
	}
	out.Orphans = orphans
	if out.Items, err = s.items(ctx, orgID, since); err != nil {
		return out, err
	}
	return out, nil
}

// compare fills the provider's view of one payment and the mismatches.
func (s *Service) compare(ctx context.Context, orgID int64, r db.ListProviderPaymentsSinceRow, row *paymodel.ReconciliationRow,
	accounts map[int64]integrations.Account) {
	fail := func(err error) {
		row.Mismatches = append(row.Mismatches, "provider_unreachable")
		row.Error = err.Error()
	}
	prov, err := s.provider(r.Provider)
	if err != nil || !r.IntegrationAccountID.Valid {
		fail(ErrUnknownProvider)
		return
	}
	acc, ok := accounts[r.IntegrationAccountID.Int64]
	if !ok {
		if acc, err = s.accounts.AccountByID(ctx, orgID, r.IntegrationAccountID.Int64); err != nil {
			fail(err)
			return
		}
		accounts[r.IntegrationAccountID.Int64] = acc
	}
	if r.ProviderToken.Valid {
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		res, err := prov.Retrieve(rctx, acc, payments.RetrieveRequest{Token: r.ProviderToken.String, ConversationID: r.ConversationID})
		cancel()
		if err != nil {
			fail(err)
			return
		}
		row.ProviderStatus = res.ProviderStatus
		if res.PaymentID != "" && !r.ProviderPaymentID.Valid {
			id := res.PaymentID
			row.ProviderPaymentID = &id
		}
		if res.Status == payments.StatusSucceeded {
			paid := res.PaidPrice
			row.ProviderPaid = &paid
			if !res.Price.Equal(m(r.Amount)) {
				row.Mismatches = append(row.Mismatches, "amount_mismatch")
			}
		}
		switch {
		case res.Status == payments.StatusSucceeded && r.Status == paymodel.StatusPending:
			row.Mismatches = append(row.Mismatches, "unconfirmed_payment")
		case res.Status == payments.StatusSucceeded && r.Status == paymodel.StatusFailed:
			row.Mismatches = append(row.Mismatches, "status_mismatch")
		case res.Status == payments.StatusFailed && r.Status != paymodel.StatusFailed && r.Status != paymodel.StatusCancelled:
			row.Mismatches = append(row.Mismatches, "status_mismatch")
		}
	}
	providerPaymentID := r.ProviderPaymentID.String
	if providerPaymentID == "" && row.ProviderPaymentID != nil {
		providerPaymentID = *row.ProviderPaymentID
	}
	if providerPaymentID == "" {
		return
	}
	st, err := s.lookupRefunds(ctx, prov, acc, providerPaymentID, r.ConversationID)
	switch {
	case errors.Is(err, integrations.ErrUnsupported):
		row.Mismatches = append(row.Mismatches, "refund_lookup_unsupported")
		return
	case err != nil:
		fail(err)
		return
	}
	refunded := st.Refunded
	row.ProviderRefunded = &refunded
	switch c := refunded.Cmp(m(r.RefundedAmount)); {
	case c > 0:
		row.Mismatches = append(row.Mismatches, "refund_missing_in_ledger")
	case c < 0:
		row.Mismatches = append(row.Mismatches, "refund_missing_at_provider")
	}
}
