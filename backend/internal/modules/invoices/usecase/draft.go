package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/ubl"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func itoa(i int) string { return strconv.Itoa(i) }

// maxDayQuantity is the longest period written in days (longer ones are one
// piece: the unit price keeps four decimals).
const maxDayQuantity = 100

// chargeNames are the Turkish line names of rental charge kinds.
var chargeNames = map[string]string{
	"late_return":  "Geç iade ücreti",
	"early_return": "Erken iade bedeli",
	"damage":       "Hasar bedeli",
	"fuel":         "Yakıt farkı",
	"mileage":      "Kilometre aşım bedeli",
	"equipment":    "Ekipman bedeli",
	"toll":         "Köprü / otoyol geçiş ücreti",
	"toll_fee":     "Geçiş hizmet bedeli",
	"other":        "Ek hizmet bedeli",
}

// lineKindOf maps a ledger item onto its invoice line kind.
func lineKindOf(purpose string, chargeKind *string) string {
	if purpose == "rent" {
		return model.KindRent
	}
	if chargeKind != nil && *chargeKind == "toll" {
		return model.KindToll
	}
	return model.KindCharge
}

// draftLine is a line before the VAT split.
type draftLine struct {
	kind, name   string
	days         int32
	entryID      int64
	gross        money.Money
	rate         money.Rate
	code, reason string
}

func days(n int) int32 {
	if n <= 0 || n > maxDayQuantity {
		return 0
	}
	return int32(n)
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// rentParts splits the rent accrual of a rental along the reservation's
// price snapshot: the rental itself, each extra service, the one-way fee,
// delivery / collection, and every extension. When the parts do not add
// up to the accrual (a kept price, a manual change) the difference goes to
// the rent part, or the accrual becomes one rent line.
func rentParts(e db.ListInvoiceableEntriesRow, facts db.GetInvoiceRentalFactsRow, amount money.Money) []draftLine {
	ref := facts.Reference
	single := []draftLine{{kind: model.KindRent, name: "Araç kiralama bedeli (" + ref + ")", entryID: e.ID, gross: amount}}
	var snap resmodel.PriceSnapshot
	if err := json.Unmarshal(facts.QuoteSnapshot, &snap); err != nil || snap.Pricing == nil {
		if e.BillingPeriodID.Valid {
			return periodParts(e, ref, nil, amount)
		}
		return single
	}
	cur := amount.Currency()
	parts := []draftLine{}
	add := func(kind, name string, d int32, gross money.Money) {
		if !gross.IsPositive() {
			return
		}
		parts = append(parts, draftLine{kind: kind, name: name, days: d, entryID: e.ID, gross: money.NewIn(gross.Minor(), cur)})
	}
	for _, l := range snap.Pricing.Lines {
		switch l.Kind {
		case pricing.LineRental:
			add(model.KindRent, "Araç kiralama bedeli ("+ref+")", days(snap.Pricing.BillingDays), l.Gross)
		case pricing.LineExtra:
			name := strings.TrimSpace(l.Name)
			if name == "" {
				name = "Ek hizmet"
			}
			if l.Quantity > 1 {
				name += " × " + strconv.Itoa(l.Quantity)
			}
			add(model.KindExtra, name, 0, l.Gross)
		case pricing.LineOneWay:
			add(model.KindOneWay, "Tek yön ücreti", 0, l.Gross)
		case pricing.LineDelivery:
			name := strings.TrimSpace(l.Name)
			if name == "" {
				name = "Teslim / alım hizmeti"
			}
			add(model.KindExtra, name, 0, l.Gross)
		}
	}
	for _, x := range snap.Extensions {
		add(model.KindRent, "Kiralama uzatma ("+strconv.Itoa(x.AddedDays)+" gün)", days(x.AddedDays), x.RentalAmount)
		add(model.KindExtra, "Uzatma ek hizmetleri", 0, x.ExtrasAmount)
	}
	if e.BillingPeriodID.Valid {
		return periodParts(e, ref, parts, amount)
	}
	sum := money.NewIn(0, cur)
	for _, p := range parts {
		sum = sum.Add(p.gross)
	}
	diff := amount.Sub(sum)
	if diff.IsZero() && len(parts) > 0 {
		return parts
	}
	for i := range parts {
		if parts[i].kind == model.KindRent && parts[i].gross.Add(diff).IsPositive() {
			parts[i].gross = parts[i].gross.Add(diff)
			return parts
		}
	}
	return single
}

// periodParts is the rent accrual of one billing period of a rental billed
// per period (corporate agreements, TEC-723): the period's share of the
// sale. Every non-rent part of the snapshot (extras, one-way, delivery)
// gets its share amount / sale total (rounded to the kuruş); the rent line
// takes the rest, so the lines add up to the accrual exactly. The rent
// line names the period and carries its days.
func periodParts(e db.ListInvoiceableEntriesRow, ref string, parts []draftLine, amount money.Money) []draftLine {
	cur := amount.Currency()
	name := "Araç kiralama bedeli (" + ref + ")"
	if e.PeriodNo.Valid && e.PeriodStartsOn.Valid && e.PeriodEndsOn.Valid {
		last := e.PeriodEndsOn.Time.AddDate(0, 0, -1)
		name += " — " + strconv.Itoa(int(e.PeriodNo.Int32)) + ". dönem " +
			e.PeriodStartsOn.Time.Format("02.01.2006") + "–" + last.Format("02.01.2006")
	}
	var d int32
	if e.PeriodDays.Valid {
		d = days(int(e.PeriodDays.Int32))
	}
	rent := draftLine{kind: model.KindRent, name: clip(name, 255), days: d, entryID: e.ID, gross: amount}
	total := money.NewIn(0, cur)
	for _, p := range parts {
		total = total.Add(p.gross)
	}
	if !total.IsPositive() {
		return []draftLine{rent}
	}
	out := []draftLine{rent}
	for _, p := range parts {
		if p.kind == model.KindRent {
			continue
		}
		share := money.NewIn(p.gross.Minor(), cur).Ratio(amount.Minor(), total.Minor())
		if !share.IsPositive() {
			continue
		}
		p.gross = share
		p.name = clip(p.name+" (dönem payı)", 255)
		out = append(out, p)
		out[0].gross = out[0].gross.Sub(share)
	}
	if !out[0].gross.IsPositive() {
		return []draftLine{rent}
	}
	return out
}

// linesFor builds the draft lines of the items (in item order) with the
// organization's tax mapping.
func (s *Service) linesFor(ctx context.Context, q *db.Queries, orgID int64, items []db.ListInvoiceableEntriesRow,
	maps map[string]model.TaxMapping) ([]draftLine, error) {
	out := []draftLine{}
	for _, e := range items {
		amount, err := money.FromNumeric(e.Amount)
		if err != nil {
			return nil, err
		}
		amount = money.NewIn(amount.Minor(), money.Currency(e.Currency))
		kind := lineKindOf(e.Purpose, textPtr(e.ChargeKind))
		var lines []draftLine
		switch {
		case kind == model.KindRent && e.RentalID.Valid:
			facts, err := q.GetInvoiceRentalFacts(ctx, db.GetInvoiceRentalFactsParams{OrganizationID: orgID, ID: e.RentalID.Int64})
			if err != nil {
				return nil, err
			}
			lines = rentParts(e, facts, amount)
		case e.ChargeKind.Valid:
			name := chargeNames[e.ChargeKind.String]
			if name == "" {
				name = chargeNames["other"]
			}
			if e.RentalReference.Valid {
				name += " (" + e.RentalReference.String + ")"
			}
			if r := clip(e.ChargeReason.String, 120); r != "" {
				name += " — " + r
			}
			lines = []draftLine{{kind: kind, name: clip(name, 255), entryID: e.ID, gross: amount}}
		default:
			name := clip(e.Description, 255)
			if name == "" {
				name = chargeNames["other"]
			}
			lines = []draftLine{{kind: kind, name: name, entryID: e.ID, gross: amount}}
		}
		for i := range lines {
			m := maps[lines[i].kind]
			lines[i].rate, lines[i].code, lines[i].reason = m.VATRate, m.ExemptionCode, m.ExemptionReason
		}
		out = append(out, lines...)
	}
	return out, nil
}

// totals is the VAT split of lines and the invoice sums.
type totals struct {
	splits   []ubl.Split
	net, vat money.Money
	payable  money.Money
}

func splitOf(lines []draftLine, currency string) totals {
	gross := make([]money.Money, len(lines))
	rates := make([]money.Rate, len(lines))
	for i, l := range lines {
		gross[i], rates[i] = l.gross, l.rate
	}
	splits, _ := ubl.SplitLines(gross, rates)
	cur := money.Currency(currency)
	t := totals{splits: splits, net: money.NewIn(0, cur), vat: money.NewIn(0, cur), payable: money.NewIn(0, cur)}
	for _, sp := range splits {
		t.net = t.net.Add(money.NewIn(sp.Net.Minor(), cur))
		t.vat = t.vat.Add(money.NewIn(sp.VAT.Minor(), cur))
	}
	t.payable = t.net.Add(t.vat)
	return t
}

// receiverSnapshot is the receiver taken from a customer record.
type receiverSnapshot struct {
	typ, identityType, identity, hint string
	name, first, last, taxOffice      string
	address, district, city, postal   string
	country, email                    string
}

func (s *Service) receiverFromCustomer(orgID int64, c db.GetInvoiceCustomerRow) (receiverSnapshot, error) {
	r := receiverSnapshot{
		typ: c.CustomerType, taxOffice: strings.TrimSpace(c.TaxOffice), address: strings.TrimSpace(c.BillingAddress),
		district: strings.TrimSpace(c.BillingDistrict), city: strings.TrimSpace(c.BillingCity),
		postal: strings.TrimSpace(c.BillingPostalCode), email: strings.TrimSpace(c.Email), country: "TR",
	}
	if c.BillingCountry.Valid && c.BillingCountry.String != "" {
		r.country = c.BillingCountry.String
	}
	identity, err := customersusecase.OpenSensitive(s.cipher, orgID, customersusecase.ColumnCustomerIdentity, c.IdentityEnc)
	if err != nil {
		return r, err
	}
	kind := ""
	if c.IdentityType.Valid {
		kind = c.IdentityType.String
	}
	if c.CustomerType == "corporate" {
		r.name = strings.TrimSpace(c.CompanyTitle)
	} else {
		r.first, r.last = strings.TrimSpace(c.FirstName), strings.TrimSpace(c.LastName)
		r.name = strings.TrimSpace(r.first + " " + r.last)
	}
	switch kind {
	case ubl.IdentityVKN, ubl.IdentityTCKN:
		r.identityType, r.identity = kind, identity
	default:
		// A final consumer without a Turkish identity number (passport):
		// GİB's placeholder TCKN for e-Arşiv.
		if c.CustomerType != "corporate" {
			r.identityType, r.identity = ubl.IdentityTCKN, ubl.AnonymousTCKN
		}
	}
	if r.identity != "" {
		r.hint = pii.MaskIdentity(r.identityType, r.identity)
	}
	return r, nil
}

func uniqueViolation(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && (constraint == "" || pe.ConstraintName == constraint)
}

// parseIssueDate validates an issue date: today or up to 7 days back
// (GİB: an invoice is issued within seven days of the delivery), never in
// the future.
func (s *Service) parseIssueDate(ve *ValidationError, v *string) pgtype.Date {
	day := s.today()
	if v != nil && strings.TrimSpace(*v) != "" {
		day = strings.TrimSpace(*v)
	}
	t, err := time.ParseInLocation(time.DateOnly, day, time.UTC)
	if err != nil {
		ve.add("issue_date", "invalid", "use YYYY-MM-DD")
		return pgtype.Date{}
	}
	today, _ := time.ParseInLocation(time.DateOnly, s.today(), time.UTC)
	if t.After(today) {
		ve.add("issue_date", "future", "the issue date cannot be in the future")
	}
	if t.Before(today.AddDate(0, 0, -7)) {
		ve.add("issue_date", "too_old", "the issue date can be at most 7 days back")
	}
	return pgtype.Date{Time: t, Valid: true}
}

// Items lists the invoiceable items of an account or a rental (inside the
// caller's branch scope).
func (s *Service) Items(ctx context.Context, orgID int64, f model.ItemFilter) ([]model.Item, error) {
	p := db.ListInvoiceableEntriesParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)}
	if f.AccountUUID != nil {
		acc, err := s.q.GetInvoiceAccountByUUID(ctx, db.GetInvoiceAccountByUUIDParams{OrganizationID: orgID, Uuid: *f.AccountUUID})
		// Sales invoices only: a supplier account has nothing to invoice.
		if notFound(err) || (err == nil && acc.PartyKind != "customer") {
			return []model.Item{}, nil
		}
		if err != nil {
			return nil, err
		}
		p.AccountID = pgtype.Int8{Int64: acc.ID, Valid: true}
	}
	if f.RentalUUID != nil {
		r, err := s.q.GetInvoiceRentalByUUID(ctx, db.GetInvoiceRentalByUUIDParams{OrganizationID: orgID, Uuid: *f.RentalUUID})
		if notFound(err) {
			return []model.Item{}, nil
		}
		if err != nil {
			return nil, err
		}
		p.RentalID = pgtype.Int8{Int64: r.ID, Valid: true}
	}
	rows, err := s.q.ListInvoiceableEntries(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]model.Item, 0, len(rows))
	for _, e := range rows {
		amount, _ := money.FromNumeric(e.Amount)
		it := model.Item{
			EntryUUID: e.Uuid, Purpose: e.Purpose, Kind: lineKindOf(e.Purpose, textPtr(e.ChargeKind)), ChargeKind: textPtr(e.ChargeKind),
			Amount: amount, Currency: e.Currency, EntryDate: e.EntryDate.Time.Format(time.DateOnly), Description: e.Description,
			Account: model.Ref{UUID: e.AccountUuid, Code: e.AccountCode, Name: e.AccountName},
		}
		if e.RentalUuid.Valid {
			it.Rental = &model.RentalRef{UUID: uuid.UUID(e.RentalUuid.Bytes), Reference: e.RentalReference.String}
		}
		if e.BranchUuid.Valid {
			it.Branch = &model.Ref{UUID: uuid.UUID(e.BranchUuid.Bytes), Code: e.BranchCode.String, Name: e.BranchName.String}
		}
		out = append(out, it)
	}
	return out, nil
}

// CreateDraft cuts a draft invoice from a rental's invoiceable items, or an
// account's (all, or the entries named). The draft links the items
// (ledger_entry_links) and posts nothing; a repeated idempotency key
// answers the first draft.
func (s *Service) CreateDraft(ctx context.Context, orgID int64, in model.CreateInput) (model.Detail, error) {
	key := strings.TrimSpace(in.IdempotencyKey)
	if len(key) > 64 {
		return model.Detail{}, &ValidationError{Details: []Detail{{"idempotency_key", "too_long", "at most 64 characters"}}}
	}
	if key != "" {
		if id, err := s.q.GetInvoiceUUIDByIdempotencyKey(ctx, db.GetInvoiceUUIDByIdempotencyKeyParams{
			OrganizationID: orgID, IdempotencyKey: pgtype.Text{String: key, Valid: true},
		}); err == nil {
			return s.Get(ctx, orgID, id)
		} else if !notFound(err) {
			return model.Detail{}, err
		}
	}
	ve := &ValidationError{}
	if (in.RentalUUID == nil) == (in.AccountUUID == nil) {
		ve.add("rental_uuid", "required", "name either a rental or an account")
		return model.Detail{}, ve
	}
	issue := s.parseIssueDate(ve, in.IssueDate)
	note := ""
	if in.Note != nil {
		note = clip(*in.Note, 1000)
	}
	p := db.ListInvoiceableEntriesParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)}
	var rentalID pgtype.Int8
	var rentalBranch int64
	if in.RentalUUID != nil {
		r, err := s.q.GetInvoiceRentalByUUID(ctx, db.GetInvoiceRentalByUUIDParams{OrganizationID: orgID, Uuid: *in.RentalUUID})
		if notFound(err) {
			ve.add("rental_uuid", "not_found", "unknown rental")
			return model.Detail{}, ve
		}
		if err != nil {
			return model.Detail{}, err
		}
		rentalID, rentalBranch = pgtype.Int8{Int64: r.ID, Valid: true}, r.PickupBranchID
		p.RentalID = rentalID
	}
	if in.AccountUUID != nil {
		acc, err := s.q.GetInvoiceAccountByUUID(ctx, db.GetInvoiceAccountByUUIDParams{OrganizationID: orgID, Uuid: *in.AccountUUID})
		if notFound(err) || (err == nil && acc.PartyKind != "customer") {
			ve.add("account_uuid", "not_found", "unknown customer account")
			return model.Detail{}, ve
		}
		if err != nil {
			return model.Detail{}, err
		}
		p.AccountID = pgtype.Int8{Int64: acc.ID, Valid: true}
	}
	wanted := map[uuid.UUID]bool{}
	for _, id := range in.EntryUUIDs {
		wanted[id] = true
	}
	if len(wanted) > 0 {
		p.EntryUuids = make([]uuid.UUID, 0, len(wanted))
		for id := range wanted {
			p.EntryUuids = append(p.EntryUuids, id)
		}
	}
	var branchID int64
	if in.BranchUUID != nil {
		b, err := s.q.GetInvoiceBranchByUUID(ctx, db.GetInvoiceBranchByUUIDParams{OrganizationID: orgID, Uuid: *in.BranchUUID})
		if notFound(err) || (err == nil && orgctx.CheckBranch(ctx, b.ID) != nil) {
			ve.add("branch_uuid", "not_found", "unknown branch")
		} else if err != nil {
			return model.Detail{}, err
		} else {
			branchID = b.ID
		}
	}
	if err := ve.err(); err != nil {
		return model.Detail{}, err
	}
	items, err := s.q.ListInvoiceableEntries(ctx, p)
	if err != nil {
		return model.Detail{}, err
	}
	if len(wanted) > 0 && len(items) != len(wanted) {
		ve.add("entry_uuids", "not_invoiceable", "an item is unknown, not an open accrual, or already invoiced")
		return model.Detail{}, ve
	}
	if len(items) == 0 {
		return model.Detail{}, ErrNothingToInvoice
	}
	account, currency := items[0].AccountID, items[0].Currency
	sameRental := items[0].RentalID
	for _, e := range items[1:] {
		if e.AccountID != account {
			ve.add("entry_uuids", "mixed_accounts", "an invoice covers the items of one account")
			return model.Detail{}, ve
		}
		if e.Currency != currency {
			ve.add("entry_uuids", "mixed_currencies", "an invoice covers items of one currency")
			return model.Detail{}, ve
		}
		if e.RentalID != sameRental {
			sameRental = pgtype.Int8{}
		}
	}
	if !rentalID.Valid && sameRental.Valid {
		rentalID = sameRental
	}
	if !items[0].AccountCustomerID.Valid {
		ve.add("account_uuid", "no_customer", "the account has no customer to invoice")
		return model.Detail{}, ve
	}
	if branchID == 0 {
		switch {
		case rentalBranch != 0 && orgctx.CheckBranch(ctx, rentalBranch) == nil:
			branchID = rentalBranch
		case items[0].BranchID.Valid:
			branchID = items[0].BranchID.Int64
		default:
			if b := orgctx.BranchScope(ctx); len(b) > 0 {
				branchID = b[0]
			}
		}
	}
	if branchID == 0 {
		// Head office items of a member with every branch: the first branch
		// of a rental item, else the first branch of the organization.
		for _, e := range items {
			if e.RentalID.Valid {
				if f, err := s.q.GetInvoiceRentalFacts(ctx, db.GetInvoiceRentalFactsParams{OrganizationID: orgID, ID: e.RentalID.Int64}); err == nil {
					branchID = f.PickupBranchID
					break
				}
			}
		}
	}
	if branchID == 0 {
		ve.add("branch_uuid", "required", "choose the issuing branch")
		return model.Detail{}, ve
	}

	id := uuid.New()
	actor := actorID(ctx)
	err = s.tx(ctx, func(q *db.Queries) error {
		cust, err := q.GetInvoiceCustomer(ctx, db.GetInvoiceCustomerParams{OrganizationID: orgID, ID: items[0].AccountCustomerID.Int64})
		if err != nil {
			return err
		}
		recv, err := s.receiverFromCustomer(orgID, cust)
		if err != nil {
			return err
		}
		tp, err := s.lookupTaxpayer(ctx, q, orgID, recv.identity, cust.EInvoice)
		if err != nil {
			return err
		}
		maps, err := s.taxMappings(ctx, q, orgID)
		if err != nil {
			return err
		}
		st, err := s.loadSettings(ctx, q, orgID)
		if err != nil {
			return err
		}
		if note == "" {
			note = clip(st.note, 1000)
		}
		lines, err := s.linesFor(ctx, q, orgID, items, maps)
		if err != nil {
			return err
		}
		t := splitOf(lines, currency)
		linked := money.NewIn(0, money.Currency(currency))
		for _, e := range items {
			a, _ := money.FromNumeric(e.Amount)
			linked = linked.Add(money.NewIn(a.Minor(), money.Currency(currency)))
		}
		enc, err := s.sealIdentity(orgID, id, recv.identity)
		if err != nil {
			return err
		}
		var idType pgtype.Text
		if recv.identityType != "" {
			idType = pgtype.Text{String: recv.identityType, Valid: true}
		}
		var idem pgtype.Text
		if key != "" {
			idem = pgtype.Text{String: key, Valid: true}
		}
		invID, err := q.InsertInvoice(ctx, db.InsertInvoiceParams{
			Uuid: id, OrganizationID: orgID, BranchID: branchID, AccountID: pgtype.Int8{Int64: account, Valid: true},
			CustomerID: pgtype.Int8{Int64: cust.ID, Valid: true}, RentalID: rentalID, Profile: profileFor(tp.einvoice),
			InvoiceType: model.TypeSatis, IssueDate: issue, Currency: currency, ReceiverType: recv.typ,
			ReceiverIdentityType: idType, ReceiverIdentityEnc: enc, ReceiverIdentityHint: recv.hint,
			ReceiverName: clip(recv.name, 255), ReceiverFirstName: clip(recv.first, 128), ReceiverLastName: clip(recv.last, 128),
			ReceiverTaxOffice: clip(recv.taxOffice, 128), ReceiverAddress: recv.address, ReceiverDistrict: clip(recv.district, 128),
			ReceiverCity: clip(recv.city, 128), ReceiverPostalCode: clip(recv.postal, 16), ReceiverCountry: recv.country,
			ReceiverEmail: clip(recv.email, 255), ReceiverEinvoice: tp.einvoice, ReceiverAlias: tp.alias,
			TaxpayerSource: tp.source, TaxpayerCheckedAt: tp.checkedAt, LineExtensionAmount: t.net.Numeric(),
			TaxAmount: t.vat.Numeric(), PayableAmount: t.payable.Numeric(), LinkedAmount: linked.Numeric(), Note: note,
			IdempotencyKey: idem, CreatedBy: actor,
		})
		if err != nil {
			return err
		}
		for i, l := range lines {
			if err := q.InsertInvoiceLine(ctx, db.InsertInvoiceLineParams{
				OrganizationID: orgID, InvoiceID: invID, Position: int32(i + 1), Kind: l.kind, Name: clip(l.name, 255),
				Days: l.days, EntryID: pgtype.Int8{Int64: l.entryID, Valid: l.entryID > 0}, GrossAmount: l.gross.Numeric(),
				NetAmount: t.splits[i].Net.Numeric(), VatRate: l.rate.Numeric(), VatAmount: t.splits[i].VAT.Numeric(),
				ExemptionCode: l.code, ExemptionReason: l.reason,
			}); err != nil {
				return err
			}
		}
		for _, e := range items {
			if err := q.InsertInvoiceLink(ctx, db.InsertInvoiceLinkParams{
				OrganizationID: orgID, EntryID: e.ID, DocumentRef: id.String(), InvoiceID: pgtype.Int8{Int64: invID, Valid: true},
				Amount: e.Amount, CreatedBy: actor,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	switch {
	case uniqueViolation(err, "uq_ledger_entry_links_invoiced"):
		return model.Detail{}, ErrAlreadyInvoiced
	case key != "" && uniqueViolation(err, "uq_invoices_organization_idempotency"):
		if id, gerr := s.q.GetInvoiceUUIDByIdempotencyKey(ctx, db.GetInvoiceUUIDByIdempotencyKeyParams{
			OrganizationID: orgID, IdempotencyKey: pgtype.Text{String: key, Valid: true},
		}); gerr == nil {
			return s.Get(ctx, orgID, id)
		}
		return model.Detail{}, err
	case err != nil:
		return model.Detail{}, err
	}
	s.publish(ctx, resourceevents.ActionCreated, id.String())
	return s.Get(ctx, orgID, id)
}

// draftRow loads a draft of the caller's scope.
func (s *Service) draftRow(ctx context.Context, orgID int64, id uuid.UUID) (db.GetInvoiceRow, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return row, err
	}
	if row.Status != model.StatusDraft {
		return row, ErrNotDraft
	}
	return row, nil
}

// PatchDraft edits a draft: issue date, note, profile, receiver, line
// names / VAT, and items taken off (their links are released). Amounts
// follow the ledger items and are not edited.
func (s *Service) PatchDraft(ctx context.Context, orgID int64, id uuid.UUID, in model.PatchInput) (model.Detail, error) {
	row, err := s.draftRow(ctx, orgID, id)
	if err != nil {
		return model.Detail{}, err
	}
	ve := &ValidationError{}
	issue := row.IssueDate
	if in.IssueDate != nil {
		issue = s.parseIssueDate(ve, in.IssueDate)
	}
	note := row.Note
	if in.Note != nil {
		note = clip(*in.Note, 1000)
	}
	identity, err := s.openIdentity(orgID, row.Uuid, row.ReceiverIdentityEnc)
	if err != nil {
		return model.Detail{}, err
	}
	recv := receiverSnapshot{
		typ: row.ReceiverType, identity: identity, hint: row.ReceiverIdentityHint, name: row.ReceiverName,
		first: row.ReceiverFirstName, last: row.ReceiverLastName, taxOffice: row.ReceiverTaxOffice,
		address: row.ReceiverAddress, district: row.ReceiverDistrict, city: row.ReceiverCity,
		postal: row.ReceiverPostalCode, country: row.ReceiverCountry, email: row.ReceiverEmail,
	}
	if row.ReceiverIdentityType.Valid {
		recv.identityType = row.ReceiverIdentityType.String
	}
	identityChanged := false
	if r := in.Receiver; r != nil {
		set := func(dst *string, v *string, field string, max int) {
			if v == nil {
				return
			}
			t := strings.TrimSpace(*v)
			if utf8.RuneCountInString(t) > max {
				ve.add("receiver."+field, "too_long", fmt.Sprintf("at most %d characters", max))
				return
			}
			*dst = t
		}
		if r.Type != nil {
			if *r.Type != "individual" && *r.Type != "corporate" {
				ve.add("receiver.type", "invalid", "individual or corporate")
			} else {
				recv.typ = *r.Type
			}
		}
		if r.IdentityType != nil {
			if *r.IdentityType != ubl.IdentityVKN && *r.IdentityType != ubl.IdentityTCKN {
				ve.add("receiver.identity_type", "invalid", "vkn or tckn")
			} else if *r.IdentityType != recv.identityType {
				recv.identityType, identityChanged = *r.IdentityType, true
			}
		}
		if r.IdentityNumber != nil {
			v := strings.TrimSpace(*r.IdentityNumber)
			if v != recv.identity {
				recv.identity, identityChanged = v, true
			}
		}
		set(&recv.name, r.Name, "name", 255)
		set(&recv.first, r.FirstName, "first_name", 128)
		set(&recv.last, r.LastName, "last_name", 128)
		set(&recv.taxOffice, r.TaxOffice, "tax_office", 128)
		set(&recv.address, r.Address, "address", 500)
		set(&recv.district, r.District, "district", 128)
		set(&recv.city, r.City, "city", 128)
		set(&recv.postal, r.PostalCode, "postal_code", 16)
		set(&recv.email, r.Email, "email", 255)
		if r.Country != nil {
			c := strings.ToUpper(strings.TrimSpace(*r.Country))
			if len(c) != 2 {
				ve.add("receiver.country", "invalid", "ISO 3166-1 alpha-2 country code")
			} else {
				recv.country = c
			}
		}
		if recv.typ == "individual" && (r.FirstName != nil || r.LastName != nil) {
			recv.name = strings.TrimSpace(recv.first + " " + recv.last)
		}
	}
	if identityChanged {
		switch {
		case recv.identityType == ubl.IdentityVKN && (len(recv.identity) != 10 || !allDigits(recv.identity)):
			ve.add("receiver.identity_number", "invalid", "VKN must be 10 digits")
		case recv.identityType == ubl.IdentityTCKN && (len(recv.identity) != 11 || !allDigits(recv.identity)):
			ve.add("receiver.identity_number", "invalid", "TCKN must be 11 digits")
		}
		if recv.identity != "" {
			recv.hint = pii.MaskIdentity(recv.identityType, recv.identity)
		} else {
			recv.hint = ""
		}
	}
	lines, err := s.q.ListInvoiceLines(ctx, db.ListInvoiceLinesParams{OrganizationID: orgID, InvoiceID: row.ID})
	if err != nil {
		return model.Detail{}, err
	}
	byUUID := map[uuid.UUID]int{}
	for i, l := range lines {
		byUUID[l.Uuid] = i
	}
	type linePatch struct {
		name, code, reason string
		rate               money.Rate
	}
	patched := map[int]linePatch{}
	for i, l := range lines {
		rate, _ := money.RateFromNumeric(l.VatRate)
		patched[i] = linePatch{name: l.Name, code: l.ExemptionCode, reason: l.ExemptionReason, rate: rate}
	}
	for n, lp := range in.Lines {
		p := "lines." + itoa(n)
		i, ok := byUUID[lp.UUID]
		if !ok {
			ve.add(p+".uuid", "not_found", "unknown line")
			continue
		}
		cur := patched[i]
		if lp.Name != nil {
			v := strings.TrimSpace(*lp.Name)
			if v == "" || utf8.RuneCountInString(v) > 255 {
				ve.add(p+".name", "invalid", "1 to 255 characters")
			} else {
				cur.name = v
			}
		}
		if lp.VATRate != nil {
			r, err := money.ParseRate(strings.TrimSpace(*lp.VATRate))
			if err != nil || r < 0 || r > money.Percent(100) {
				ve.add(p+".vat_rate", "invalid", "VAT rate is between 0 and 100")
			} else {
				cur.rate = r
			}
		}
		if lp.ExemptionCode != nil {
			cur.code = strings.TrimSpace(*lp.ExemptionCode)
		}
		if lp.ExemptionReason != nil {
			cur.reason = clip(*lp.ExemptionReason, 255)
		}
		patched[i] = cur
	}
	removeIDs := []int64{}
	removed := map[int64]bool{}
	if len(in.RemoveEntryUUIDs) > 0 {
		links, err := s.q.ListInvoiceLinks(ctx, db.ListInvoiceLinksParams{OrganizationID: orgID, InvoiceID: row.ID})
		if err != nil {
			return model.Detail{}, err
		}
		live := map[uuid.UUID]int64{}
		for _, l := range links {
			if !l.ReleasedAt.Valid {
				live[l.EntryUuid] = l.EntryID
			}
		}
		for n, e := range in.RemoveEntryUUIDs {
			eid, ok := live[e]
			if !ok {
				ve.add("remove_entry_uuids."+itoa(n), "not_found", "the item is not on this invoice")
				continue
			}
			if !removed[eid] {
				removed[eid] = true
				removeIDs = append(removeIDs, eid)
			}
		}
	}
	// Remaining lines, re-split.
	keep := []draftLine{}
	keepIdx := []int{}
	for i, l := range lines {
		if l.EntryID.Valid && removed[l.EntryID.Int64] {
			continue
		}
		g, _ := money.FromNumeric(l.GrossAmount)
		pl := patched[i]
		keep = append(keep, draftLine{kind: l.Kind, name: pl.name, days: l.Days, gross: money.NewIn(g.Minor(), money.Currency(row.Currency)),
			rate: pl.rate, code: pl.code, reason: pl.reason})
		keepIdx = append(keepIdx, i)
	}
	if len(keep) == 0 {
		ve.add("remove_entry_uuids", "empty", "an invoice keeps at least one line; cancel the draft instead")
	}
	if err := ve.err(); err != nil {
		return model.Detail{}, err
	}
	tp := taxpayer{einvoice: row.ReceiverEinvoice, alias: row.ReceiverAlias, source: row.TaxpayerSource, checkedAt: row.TaxpayerCheckedAt}
	profile := row.Profile
	err = s.tx(ctx, func(q *db.Queries) error {
		if identityChanged {
			flag := false
			if row.CustomerID.Valid {
				if c, err := q.GetInvoiceCustomer(ctx, db.GetInvoiceCustomerParams{OrganizationID: orgID, ID: row.CustomerID.Int64}); err == nil {
					flag = c.EInvoice
				}
			}
			t, err := s.lookupTaxpayer(ctx, q, orgID, recv.identity, flag)
			if err != nil {
				return err
			}
			tp = t
			profile = profileFor(tp.einvoice)
		}
		if in.Profile != nil {
			profile = strings.TrimSpace(*in.Profile)
		}
		switch {
		case !model.Contains(model.Profiles, profile):
			ve.add("profile", "invalid", "TEMELFATURA, TICARIFATURA or EARSIVFATURA")
		case row.InvoiceType == model.TypeIade && profile == model.ProfileTicari:
			ve.add("profile", "invalid", "a return invoice cannot use TICARIFATURA")
		case tp.einvoice && profile == model.ProfileEArsiv:
			ve.add("profile", "einvoice_receiver", "the receiver is an e-Fatura taxpayer: use TEMELFATURA or TICARIFATURA")
		case !tp.einvoice && profile != model.ProfileEArsiv:
			ve.add("profile", "earchive_receiver", "the receiver is not an e-Fatura taxpayer: use EARSIVFATURA")
		}
		if err := ve.err(); err != nil {
			return err
		}
		if len(removeIDs) > 0 {
			if err := q.DeleteInvoiceLinesOfEntries(ctx, db.DeleteInvoiceLinesOfEntriesParams{
				OrganizationID: orgID, InvoiceID: row.ID, EntryIds: removeIDs,
			}); err != nil {
				return err
			}
			if _, err := q.ReleaseInvoiceLinks(ctx, db.ReleaseInvoiceLinksParams{
				OrganizationID: orgID, InvoiceID: row.ID, EntryIds: removeIDs, ReleasedAt: ts(s.clock()), Reason: model.ReleaseRemoved,
			}); err != nil {
				return err
			}
			if err := q.RenumberInvoiceLines(ctx, db.RenumberInvoiceLinesParams{OrganizationID: orgID, InvoiceID: row.ID}); err != nil {
				return err
			}
		}
		t := splitOf(keep, row.Currency)
		for n, i := range keepIdx {
			if err := q.UpdateInvoiceLine(ctx, db.UpdateInvoiceLineParams{
				OrganizationID: orgID, ID: lines[i].ID, Name: keep[n].name, NetAmount: t.splits[n].Net.Numeric(),
				VatRate: keep[n].rate.Numeric(), VatAmount: t.splits[n].VAT.Numeric(), ExemptionCode: keep[n].code,
				ExemptionReason: keep[n].reason,
			}); err != nil {
				return err
			}
		}
		linked, err := money.FromNumeric(row.LinkedAmount)
		if err != nil {
			return err
		}
		if len(removeIDs) > 0 && row.InvoiceType == model.TypeSatis {
			linked = money.NewIn(0, money.Currency(row.Currency))
			for _, l := range keep {
				linked = linked.Add(l.gross)
			}
		}
		enc, err := s.sealIdentity(orgID, row.Uuid, recv.identity)
		if err != nil {
			return err
		}
		var idType pgtype.Text
		if recv.identityType != "" {
			idType = pgtype.Text{String: recv.identityType, Valid: true}
		}
		n, err := q.UpdateInvoiceDraft(ctx, db.UpdateInvoiceDraftParams{
			OrganizationID: orgID, ID: row.ID, Profile: profile, IssueDate: issue, Note: note, ReceiverType: recv.typ,
			ReceiverIdentityType: idType, ReceiverIdentityEnc: enc, ReceiverIdentityHint: recv.hint,
			ReceiverName: recv.name, ReceiverFirstName: recv.first, ReceiverLastName: recv.last,
			ReceiverTaxOffice: recv.taxOffice, ReceiverAddress: recv.address, ReceiverDistrict: recv.district,
			ReceiverCity: recv.city, ReceiverPostalCode: recv.postal, ReceiverCountry: recv.country,
			ReceiverEmail: recv.email, ReceiverEinvoice: tp.einvoice, ReceiverAlias: tp.alias,
			TaxpayerSource: tp.source, TaxpayerCheckedAt: tp.checkedAt, LineExtensionAmount: t.net.Numeric(),
			TaxAmount: t.vat.Numeric(), PayableAmount: t.payable.Numeric(), LinkedAmount: linked.Numeric(),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotDraft
		}
		return nil
	})
	if err != nil {
		return model.Detail{}, err
	}
	s.publish(ctx, resourceevents.ActionUpdated, row.Uuid.String())
	return s.Get(ctx, orgID, row.Uuid)
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
