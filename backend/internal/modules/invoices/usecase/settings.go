package usecase

import (
	"context"
	"regexp"
	"strings"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Defaults of the invoice settings.
const (
	DefaultEFaturaSeries      = "EFT"
	DefaultEArsivSeries       = "EAR"
	DefaultTaxpayerCacheHours = 24
)

// DefaultTaxMappings are the Turkish defaults: car rental, extra services,
// the one-way fee, damage / fuel / km / late charges and re-invoiced tolls
// are all subject to the general 20 % VAT rate (KDV, 2023 onward). An
// organization whose accountant decides otherwise (an exemption, another
// rate) changes them in the settings.
func DefaultTaxMappings() []model.TaxMapping {
	out := make([]model.TaxMapping, 0, len(model.LineKinds))
	for _, k := range model.LineKinds {
		out = append(out, model.TaxMapping{LineKind: k, VATRate: money.Percent(20), Default: true})
	}
	return out
}

var seriesRE = regexp.MustCompile(`^[A-Z0-9]{3}$`)

type invoiceSettings struct {
	efatura, earsiv            string
	district, city, postalCode string
	note                       string
	cacheHours                 int32
	updatedAt                  pgtype.Timestamptz
}

func (s *Service) loadSettings(ctx context.Context, q *db.Queries, orgID int64) (invoiceSettings, error) {
	row, err := q.GetInvoiceSettings(ctx, orgID)
	if notFound(err) {
		return invoiceSettings{efatura: DefaultEFaturaSeries, earsiv: DefaultEArsivSeries, cacheHours: DefaultTaxpayerCacheHours}, nil
	}
	if err != nil {
		return invoiceSettings{}, err
	}
	return invoiceSettings{
		efatura: strings.TrimSpace(row.EfaturaSeries), earsiv: strings.TrimSpace(row.EarsivSeries),
		district: row.SupplierDistrict, city: row.SupplierCity, postalCode: row.SupplierPostalCode,
		note: row.DefaultNote, cacheHours: row.TaxpayerCacheHours, updatedAt: row.UpdatedAt,
	}, nil
}

// taxMappings returns the organization's mapping per line kind (defaults
// for kinds it never set).
func (s *Service) taxMappings(ctx context.Context, q *db.Queries, orgID int64) (map[string]model.TaxMapping, error) {
	out := map[string]model.TaxMapping{}
	for _, m := range DefaultTaxMappings() {
		out[m.LineKind] = m
	}
	rows, err := q.ListInvoiceTaxMappings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		rate, err := money.RateFromNumeric(r.VatRate)
		if err != nil {
			return nil, err
		}
		out[r.LineKind] = model.TaxMapping{LineKind: r.LineKind, VATRate: rate, ExemptionCode: r.ExemptionCode, ExemptionReason: r.ExemptionReason}
	}
	return out, nil
}

// supplier is the organization as supplier: company settings (legal name,
// tax number / office, address, contact) and the invoice settings'
// district / city / postal code.
func (s *Service) supplier(ctx context.Context, orgID int64, st invoiceSettings) (model.Supplier, error) {
	sup := model.Supplier{District: st.district, City: st.city, PostalCode: st.postalCode}
	if s.settings == nil {
		return sup, nil
	}
	v, err := s.settings.Current(ctx, orgID)
	if err != nil {
		return sup, err
	}
	c := v.Settings.Company
	sup.LegalName, sup.TaxNumber, sup.TaxOffice = strings.TrimSpace(c.LegalName), strings.TrimSpace(c.TaxNumber), strings.TrimSpace(c.TaxOffice)
	sup.Address, sup.Email, sup.Phone = strings.TrimSpace(c.Address), strings.TrimSpace(c.Email), strings.TrimSpace(c.Phone)
	return sup, nil
}

func supplierMissing(sup model.Supplier) []string {
	out := []string{}
	if sup.LegalName == "" {
		out = append(out, "supplier.legal_name")
	}
	if n := len(sup.TaxNumber); n != 10 && n != 11 {
		out = append(out, "supplier.tax_number")
	}
	if sup.TaxOffice == "" {
		out = append(out, "supplier.tax_office")
	}
	if sup.District == "" {
		out = append(out, "supplier.district")
	}
	if sup.City == "" {
		out = append(out, "supplier.city")
	}
	return out
}

// route is where an invoice's jobs go: a working adapter (ready) or a
// dependency hold.
type route struct {
	provider  string
	accountID pgtype.Int8
	account   uuid.UUID
	name      string
	ready     bool
	hold      string
}

// routeOf picks the organization's active e-invoice account and checks that
// its adapter can send.
func (s *Service) routeOf(ctx context.Context, q *db.Queries, orgID int64) (route, error) {
	acc, err := q.GetActiveIntegrationAccount(ctx, db.GetActiveIntegrationAccountParams{OrganizationID: orgID, Kind: einvoice.Kind})
	if notFound(err) {
		return route{provider: einvoice.ProviderOfficial, hold: model.HoldNoAccount}, nil
	}
	if err != nil {
		return route{}, err
	}
	r := route{provider: acc.Provider, accountID: pgtype.Int8{Int64: acc.ID, Valid: true}, account: acc.Uuid, name: acc.DisplayName}
	p, ok := s.providers[acc.Provider]
	switch {
	case !ok:
		r.hold = model.HoldAdapterUnavailable
	case p.Ready() != nil:
		r.hold = model.HoldAdapterUnverified
	default:
		r.ready = true
	}
	return r, nil
}

func holdMessage(reason string) string {
	switch reason {
	case model.HoldNoAccount:
		return "waiting on dependency: no active e-invoice integrator is connected"
	case model.HoldAdapterUnverified:
		return "waiting on dependency: the e-invoice integrator is not chosen yet"
	case model.HoldAdapterUnavailable:
		return "waiting on dependency: the connected e-invoice adapter is not available in this environment"
	}
	return "waiting on dependency"
}

func connectionOf(r route) model.Connection {
	c := model.Connection{Provider: r.provider, Name: r.name}
	if r.accountID.Valid {
		id := r.account
		c.AccountUUID = &id
	}
	switch {
	case r.hold == model.HoldNoAccount:
		c.State = "none"
	case r.hold == model.HoldAdapterUnverified:
		c.State = "waiting"
	case r.hold == model.HoldAdapterUnavailable:
		c.State = "unavailable"
	case strings.HasSuffix(r.provider, "_fake"):
		c.State = "test"
	default:
		c.State = "ready"
	}
	return c
}

// GetSettings returns the invoice settings with the supplier data and what
// is still missing.
func (s *Service) GetSettings(ctx context.Context, orgID int64) (model.Settings, error) {
	st, err := s.loadSettings(ctx, s.q, orgID)
	if err != nil {
		return model.Settings{}, err
	}
	maps, err := s.taxMappings(ctx, s.q, orgID)
	if err != nil {
		return model.Settings{}, err
	}
	sup, err := s.supplier(ctx, orgID, st)
	if err != nil {
		return model.Settings{}, err
	}
	r, err := s.routeOf(ctx, s.q, orgID)
	if err != nil {
		return model.Settings{}, err
	}
	out := model.Settings{
		EFaturaSeries: st.efatura, EArsivSeries: st.earsiv, SupplierDistrict: st.district, SupplierCity: st.city,
		SupplierPostalCode: st.postalCode, DefaultNote: st.note, TaxpayerCacheHours: st.cacheHours, Supplier: sup,
		Missing: supplierMissing(sup), Connection: connectionOf(r), UpdatedAt: timePtr(st.updatedAt),
	}
	for _, k := range model.LineKinds {
		out.TaxMappings = append(out.TaxMappings, maps[k])
	}
	return out, nil
}

// PutSettings saves the invoice settings and tax mappings.
func (s *Service) PutSettings(ctx context.Context, orgID int64, in model.SettingsInput) (model.Settings, error) {
	if err := s.requireEInvoice(ctx, orgID); err != nil {
		return model.Settings{}, err
	}
	st, err := s.loadSettings(ctx, s.q, orgID)
	if err != nil {
		return model.Settings{}, err
	}
	ve := &ValidationError{}
	str := func(p *string, cur string) string {
		if p == nil {
			return cur
		}
		return strings.TrimSpace(*p)
	}
	st.efatura = strings.ToUpper(str(in.EFaturaSeries, st.efatura))
	st.earsiv = strings.ToUpper(str(in.EArsivSeries, st.earsiv))
	if !seriesRE.MatchString(st.efatura) {
		ve.add("efatura_series", "invalid", "the series is three capital letters or digits")
	}
	if !seriesRE.MatchString(st.earsiv) {
		ve.add("earsiv_series", "invalid", "the series is three capital letters or digits")
	}
	if st.efatura == st.earsiv && st.efatura != "" {
		ve.add("earsiv_series", "duplicate", "e-Fatura and e-Arşiv use different series")
	}
	st.district = str(in.SupplierDistrict, st.district)
	st.city = str(in.SupplierCity, st.city)
	st.postalCode = str(in.SupplierPostalCode, st.postalCode)
	st.note = str(in.DefaultNote, st.note)
	if len(st.district) > 128 {
		ve.add("supplier_district", "too_long", "at most 128 characters")
	}
	if len(st.city) > 128 {
		ve.add("supplier_city", "too_long", "at most 128 characters")
	}
	if len(st.postalCode) > 16 {
		ve.add("supplier_postal_code", "too_long", "at most 16 characters")
	}
	if len(st.note) > 1000 {
		ve.add("default_note", "too_long", "at most 1000 characters")
	}
	if in.TaxpayerCacheHours != nil {
		st.cacheHours = *in.TaxpayerCacheHours
	}
	if st.cacheHours < 1 || st.cacheHours > 720 {
		ve.add("taxpayer_cache_hours", "invalid", "between 1 and 720 hours")
	}
	type mapping struct {
		kind, code, reason string
		rate               money.Rate
	}
	maps := []mapping{}
	seen := map[string]bool{}
	for i, m := range in.TaxMappings {
		p := "tax_mappings." + itoa(i)
		kind := strings.TrimSpace(m.LineKind)
		if !model.Contains(model.LineKinds, kind) || seen[kind] {
			ve.add(p+".line_kind", "invalid", "unknown or repeated line kind")
			continue
		}
		seen[kind] = true
		rate, err := money.ParseRate(strings.TrimSpace(m.VATRate))
		if err != nil || rate < 0 || rate > money.Percent(100) {
			ve.add(p+".vat_rate", "invalid", "VAT rate is between 0 and 100")
			continue
		}
		code, reason := strings.TrimSpace(m.ExemptionCode), strings.TrimSpace(m.ExemptionReason)
		if code != "" && !codelist.ValidExemption(code) && !codelist.ValidExemptionIstisna(code) {
			ve.add(p+".exemption_code", "invalid", "unknown GİB exemption code")
		}
		if rate == 0 && code == "" {
			ve.add(p+".exemption_code", "required", "a 0 % rate needs an exemption code")
		}
		if rate == 0 && reason == "" {
			ve.add(p+".exemption_reason", "required", "a 0 % rate needs an exemption reason")
		}
		if len(reason) > 255 {
			ve.add(p+".exemption_reason", "too_long", "at most 255 characters")
		}
		maps = append(maps, mapping{kind: kind, code: code, reason: reason, rate: rate})
	}
	if err := ve.err(); err != nil {
		return model.Settings{}, err
	}
	actor := actorID(ctx)
	err = s.tx(ctx, func(q *db.Queries) error {
		if err := q.UpsertInvoiceSettings(ctx, db.UpsertInvoiceSettingsParams{
			OrganizationID: orgID, EfaturaSeries: st.efatura, EarsivSeries: st.earsiv, SupplierDistrict: st.district,
			SupplierCity: st.city, SupplierPostalCode: st.postalCode, DefaultNote: st.note,
			TaxpayerCacheHours: st.cacheHours, UpdatedBy: actor,
		}); err != nil {
			return err
		}
		for _, m := range maps {
			if err := q.UpsertInvoiceTaxMapping(ctx, db.UpsertInvoiceTaxMappingParams{
				OrganizationID: orgID, LineKind: m.kind, VatRate: m.rate.Numeric(), ExemptionCode: m.code,
				ExemptionReason: m.reason, UpdatedBy: actor,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Settings{}, err
	}
	return s.GetSettings(ctx, orgID)
}
