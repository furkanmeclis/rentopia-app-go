package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/jackc/pgx/v5/pgtype"
)

// taxpayer is the answer of the receiver type decision.
type taxpayer struct {
	einvoice  bool
	alias     string
	source    string
	checkedAt pgtype.Timestamptz
}

// lookupTaxpayer decides e-Fatura vs e-Arşiv for a VKN / TCKN ("mükellef
// sorgusu"): the cached answer while it is fresh (TTL of the settings),
// else the integrator's lookup (cached), else — no working integrator or
// the lookup failed — the customer's own e-invoice flag. The identity
// number is only hashed (keyed, per organization) for the cache.
func (s *Service) lookupTaxpayer(ctx context.Context, q *db.Queries, orgID int64, identity string, customerFlag bool) (taxpayer, error) {
	fallback := taxpayer{einvoice: customerFlag, source: model.TaxpayerCustomerFlag}
	if identity == "" {
		return fallback, nil
	}
	now := s.clock()
	bidx := s.cipher.BlindIndex(TaxpayerIndexPurpose, orgKey(orgID), identity)
	if row, err := q.GetEInvoiceTaxpayer(ctx, db.GetEInvoiceTaxpayerParams{
		OrganizationID: orgID, IdentityBidx: bidx, Now: ts(now),
	}); err == nil {
		return taxpayer{einvoice: row.IsEinvoice, alias: row.Alias, source: model.TaxpayerCache, checkedAt: row.CheckedAt}, nil
	} else if !notFound(err) {
		return taxpayer{}, err
	}
	r, err := s.routeOf(ctx, q, orgID)
	if err != nil {
		return taxpayer{}, err
	}
	if !r.ready || s.accounts == nil {
		return fallback, nil
	}
	acc, err := s.accounts.AccountByID(ctx, orgID, r.accountID.Int64)
	if err != nil {
		s.log.Warn("invoice_taxpayer_account_failed", "error", err)
		return fallback, nil
	}
	t, err := s.providers[r.provider].LookupTaxpayer(ctx, acc, identity)
	if err != nil {
		s.log.Warn("invoice_taxpayer_lookup_failed", "provider", r.provider, "error", err)
		return fallback, nil
	}
	st, err := s.loadSettings(ctx, q, orgID)
	if err != nil {
		return taxpayer{}, err
	}
	if err := q.UpsertEInvoiceTaxpayer(ctx, db.UpsertEInvoiceTaxpayerParams{
		OrganizationID: orgID, IdentityBidx: bidx, IsEinvoice: t.EInvoice, Alias: t.Alias, Title: t.Title,
		Provider: r.provider, CheckedAt: ts(now), ExpiresAt: ts(now.Add(time.Duration(st.cacheHours) * time.Hour)),
	}); err != nil {
		return taxpayer{}, err
	}
	return taxpayer{einvoice: t.EInvoice, alias: t.Alias, source: model.TaxpayerProvider, checkedAt: ts(now)}, nil
}

// profileFor is the default profile of a receiver: TEMELFATURA for an
// e-Fatura taxpayer (TICARIFATURA can be chosen on the draft), EARSIVFATURA
// otherwise.
func profileFor(einvoiceUser bool) string {
	if einvoiceUser {
		return model.ProfileTemel
	}
	return model.ProfileEArsiv
}
