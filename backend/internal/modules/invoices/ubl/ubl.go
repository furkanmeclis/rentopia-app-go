// Package ubl turns an invoice of the invoices module into a GİB UBL-TR
// document with go-ubltr (github.com/furkanmeclis/go-ubltr): the builder
// and code lists produce the XML, ValidateXSD + ValidateSchematron check
// it, render (XSLT → HTML) and render/chromium (through pdfrender) show it.
// Nothing here writes UBL by hand: every element comes from the builder or
// the generated types it returns.
//
// Amounts: the invoice module stores the VAT split of every line (net,
// VAT, total) in minor units; the builder recomputes the line VAT from the
// net it is given (round(net × rate)), and Build checks that the totals it
// wrote are the ones the module computed (SplitLines uses the same rule).
package ubl

import (
	"errors"
	"fmt"
	"strings"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Profiles and types the module issues.
const (
	ProfileTemel   = codelist.ProfileTemelFatura
	ProfileTicari  = codelist.ProfileTicariFatura
	ProfileEArsiv  = codelist.ProfileEArsivFatura
	TypeSatis      = codelist.InvoiceTypeSatis
	TypeIade       = codelist.InvoiceTypeIade
	IdentityVKN    = "vkn"
	IdentityTCKN   = "tckn"
	unitDay        = codelist.UnitDAY
	unitPiece      = codelist.UnitC62
	defaultCountry = "Türkiye"
)

// AnonymousTCKN is the identity GİB expects for a final consumer without a
// Turkish identity number on an e-Arşiv invoice.
const AnonymousTCKN = "11111111111"

// Party is the supplier (the organization) or the receiver.
type Party struct {
	// IdentityType is vkn or tckn.
	IdentityType string
	Identity     string
	// Name is the title of a company (VKN); FirstName / LastName name a
	// person (TCKN).
	Name        string
	FirstName   string
	LastName    string
	TaxOffice   string
	Street      string
	District    string
	City        string
	PostalCode  string
	Country     string
	CountryCode string
	Email       string
	Phone       string
}

// Line is one invoice line. Net / VAT come from SplitLines.
type Line struct {
	Name string
	// Days > 0 writes the quantity in days (rent); otherwise one piece.
	Days            int
	Net             money.Money
	VAT             money.Money
	Rate            money.Rate
	ExemptionCode   string
	ExemptionReason string
}

// Reference is the original invoice an IADE invoice returns.
type Reference struct {
	Number    string
	IssueDate string
}

// Document is everything an invoice needs.
type Document struct {
	UUID      string
	Number    string
	Profile   string
	Type      string
	IssueDate string // YYYY-MM-DD (Europe/Istanbul)
	IssueTime string // HH:MM:SS
	Currency  string
	Supplier  Party
	Customer  Party
	Lines     []Line
	Notes     []string
	// Returns is set on an IADE invoice.
	Returns *Reference
}

// Totals are the amounts the built document carries.
type Totals struct {
	LineExtension money.Money
	TaxTotal      money.Money
	Payable       money.Money
}

// ErrTotalsMismatch: the builder wrote other totals than the module
// computed (a rounding rule drifted). Never sent.
var ErrTotalsMismatch = errors.New("ubl: built totals differ from the computed totals")

func party(p Party) *builder.PartyBuilder {
	pb := builder.NewPartyBuilder()
	switch p.IdentityType {
	case IdentityVKN:
		pb.VKN(p.Identity)
	case IdentityTCKN:
		pb.TCKN(p.Identity)
	default:
		pb.Identification("", "")
	}
	if p.IdentityType == IdentityTCKN && (p.FirstName != "" || p.LastName != "") {
		pb.Person(p.FirstName, p.LastName)
	}
	if strings.TrimSpace(p.Name) != "" {
		pb.Name(p.Name)
	}
	if p.TaxOffice != "" {
		pb.TaxOffice(p.TaxOffice)
	}
	if p.Email != "" {
		pb.Email(p.Email)
	}
	if p.Phone != "" {
		pb.Phone(p.Phone)
	}
	country := p.Country
	if country == "" {
		country = defaultCountry
	}
	ab := builder.NewAddressBuilder().Street(p.Street).District(p.District).City(p.City).Country(country)
	if p.PostalCode != "" {
		ab.PostalZone(p.PostalCode)
	}
	if p.CountryCode != "" {
		ab.CountryCode(p.CountryCode)
	}
	return pb.AddressBuilder(ab)
}

func line(currency string, l Line) (*builder.InvoiceLineBuilder, error) {
	unit, qty := unitPiece, int64(1)
	if l.Days > 0 {
		unit, qty = unitDay, int64(l.Days)
	}
	q, err := builder.QuantityIn(unit, fmt.Sprint(qty))
	if err != nil {
		return nil, err
	}
	// Unit price with four decimals: qty × price rounds back to the net
	// amount for any quantity up to 100 (the caller writes longer periods
	// as one piece).
	price, err := builder.MoneyIn(currency, unitPrice(l.Net, qty))
	if err != nil {
		return nil, err
	}
	tax := builder.NewTaxBuilder().KDV(l.Rate.String())
	if l.ExemptionCode != "" || l.ExemptionReason != "" {
		tax.Exemption(l.ExemptionCode, l.ExemptionReason)
	}
	return builder.NewInvoiceLineBuilder().Name(l.Name).Quantity(q).Price(price).Tax(tax), nil
}

// unitPrice is net / qty with four decimals.
func unitPrice(net money.Money, qty int64) string {
	if qty <= 1 {
		return net.String()
	}
	// 4 decimals: minor units × 100 / qty, rounded half away from zero.
	v := money.New(net.Minor()*100).Ratio(1, qty).Minor()
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%04d", sign, v/10000, v%10000)
}

// Build builds the document with go-ubltr and marshals it. It returns the
// XML and the totals the document carries; a document whose totals differ
// from the lines' Net + VAT is refused (ErrTotalsMismatch).
func Build(doc Document) ([]byte, Totals, error) {
	inv, err := BuildInvoice(doc)
	if err != nil {
		return nil, Totals{}, err
	}
	data, err := ubltr.Marshal(inv)
	if err != nil {
		return nil, Totals{}, fmt.Errorf("ubl: marshal: %w", err)
	}
	t, err := totalsOf(inv)
	if err != nil {
		return nil, Totals{}, err
	}
	want := Totals{LineExtension: money.Zero(), TaxTotal: money.Zero()}
	for _, l := range doc.Lines {
		want.LineExtension = want.LineExtension.Add(l.Net)
		want.TaxTotal = want.TaxTotal.Add(l.VAT)
	}
	want.Payable = want.LineExtension.Add(want.TaxTotal)
	if t.LineExtension.Minor() != want.LineExtension.Minor() || t.TaxTotal.Minor() != want.TaxTotal.Minor() ||
		t.Payable.Minor() != want.Payable.Minor() {
		return nil, t, fmt.Errorf("%w: built %s / %s / %s, computed %s / %s / %s", ErrTotalsMismatch,
			t.LineExtension, t.TaxTotal, t.Payable, want.LineExtension, want.TaxTotal, want.Payable)
	}
	return data, t, nil
}

// BuildInvoice returns the generated invoice (no XML yet).
func BuildInvoice(doc Document) (*invoice.Invoice, error) {
	currency := doc.Currency
	if currency == "" {
		currency = codelist.TRY
	}
	b := builder.NewInvoiceBuilder().
		Profile(doc.Profile).
		Type(doc.Type).
		ID(doc.Number).
		UUID(doc.UUID).
		IssueDate(doc.IssueDate).
		Currency(currency).
		SupplierBuilder(party(doc.Supplier)).
		CustomerBuilder(party(doc.Customer))
	if doc.IssueTime != "" {
		b.IssueTime(doc.IssueTime)
	}
	for _, n := range doc.Notes {
		if strings.TrimSpace(n) != "" {
			b.Note(n)
		}
	}
	for _, l := range doc.Lines {
		lb, err := line(currency, l)
		if err != nil {
			return nil, err
		}
		b.Line(lb)
	}
	inv, err := b.Build()
	if err != nil {
		return nil, err
	}
	carryExemptions(inv)
	if doc.Returns != nil {
		// A return invoice names the invoice it returns (GİB: document
		// type code IADE and the 16 character invoice number).
		code := cbc.DocumentTypeCodeType{Value: TypeIade}
		inv.BillingReference = append(inv.BillingReference, cac.BillingReferenceType{
			InvoiceDocumentReference: &cac.DocumentReferenceType{
				ID:               cbc.IDType{Value: doc.Returns.Number},
				IssueDate:        cbc.IssueDateType{Value: doc.Returns.IssueDate},
				DocumentTypeCode: &code,
			},
		})
	}
	return inv, nil
}

// carryExemptions copies the exemption code / reason of the lines onto the
// document-level tax subtotal of the same rate. go-ubltr merges the line
// taxes per (tax code, percent) into the document TaxTotal but leaves the
// exemption out of the merged subtotal, and the GİB Schematron requires
// TaxExemptionReason on every 0-amount KDV subtotal, at the document level
// too. Only the generated struct's fields are set; amounts are untouched.
func carryExemptions(inv *invoice.Invoice) {
	type exemption struct {
		code   *cbc.TaxExemptionReasonCodeType
		reason *cbc.TaxExemptionReasonType
	}
	byPercent := map[string]exemption{}
	for _, l := range inv.InvoiceLine {
		if l.TaxTotal == nil {
			continue
		}
		for _, sub := range l.TaxTotal.TaxSubtotal {
			if sub.Percent == nil || sub.TaxCategory.TaxExemptionReason == nil {
				continue
			}
			if _, ok := byPercent[sub.Percent.Value]; !ok {
				byPercent[sub.Percent.Value] = exemption{code: sub.TaxCategory.TaxExemptionReasonCode, reason: sub.TaxCategory.TaxExemptionReason}
			}
		}
	}
	for i := range inv.TaxTotal {
		for j := range inv.TaxTotal[i].TaxSubtotal {
			sub := &inv.TaxTotal[i].TaxSubtotal[j]
			if sub.Percent == nil || sub.TaxCategory.TaxExemptionReason != nil {
				continue
			}
			if e, ok := byPercent[sub.Percent.Value]; ok {
				sub.TaxCategory.TaxExemptionReasonCode, sub.TaxCategory.TaxExemptionReason = e.code, e.reason
			}
		}
	}
}

func totalsOf(inv *invoice.Invoice) (Totals, error) {
	parse := func(s string) (money.Money, error) {
		m, err := money.Parse(strings.TrimSpace(s))
		if err != nil {
			return money.Money{}, fmt.Errorf("ubl: amount %q: %w", s, err)
		}
		return m, nil
	}
	var t Totals
	var err error
	if t.LineExtension, err = parse(inv.LegalMonetaryTotal.LineExtensionAmount.Value); err != nil {
		return t, err
	}
	if t.Payable, err = parse(inv.LegalMonetaryTotal.PayableAmount.Value); err != nil {
		return t, err
	}
	t.TaxTotal = money.Zero()
	for _, tt := range inv.TaxTotal {
		v, err := parse(tt.TaxAmount.Value)
		if err != nil {
			return t, err
		}
		t.TaxTotal = t.TaxTotal.Add(v)
	}
	return t, nil
}
