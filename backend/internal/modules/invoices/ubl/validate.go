package ubl

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
)

// Problem is one reason the document cannot be sent, mapped onto the field
// of the invoice form it belongs to (receiver.city, lines.0.vat_rate,
// supplier.tax_number, …; "document" when no field fits).
type Problem struct {
	Layer   string `json:"layer"` // check | builder | xsd | schematron
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// Check validates the document before go-ubltr sees it, so that every
// missing value is reported at once with its form field (the builder stops
// at the first one). It is not a replacement for the XSD / Schematron
// checks of Validate.
func Check(doc Document) []Problem {
	var out []Problem
	add := func(field, code, msg string) {
		out = append(out, Problem{Layer: "check", Field: field, Code: code, Message: msg})
	}
	if !codelist.ValidProfile(doc.Profile) && !codelist.ValidProfileEArchive(doc.Profile) {
		add("profile", "invalid", "unknown invoice profile")
	}
	if !codelist.ValidInvoiceType(doc.Type) {
		add("invoice_type", "invalid", "unknown invoice type")
	}
	if doc.Type == TypeIade && doc.Profile == ProfileTicari {
		add("profile", "invalid", "a return invoice cannot use the TICARIFATURA profile")
	}
	checkParty := func(prefix string, p Party, supplier bool) {
		switch p.IdentityType {
		case IdentityVKN:
			if !validVKN(p.Identity) {
				add(prefix+".identity_number", "invalid", "VKN must be 10 digits")
			}
			if strings.TrimSpace(p.Name) == "" {
				add(prefix+".name", "required", "title is required")
			}
			if strings.TrimSpace(p.TaxOffice) == "" {
				add(prefix+".tax_office", "required", "tax office is required")
			}
		case IdentityTCKN:
			if !validTCKN(p.Identity) {
				add(prefix+".identity_number", "invalid", "TCKN must be 11 digits")
			}
			if !supplier && (strings.TrimSpace(p.FirstName) == "" || strings.TrimSpace(p.LastName) == "") {
				add(prefix+".name", "required", "first and last name are required")
			}
		default:
			add(prefix+".identity_number", "required", "VKN or TCKN is required")
		}
		if strings.TrimSpace(p.City) == "" {
			add(prefix+".city", "required", "city is required")
		}
		if strings.TrimSpace(p.District) == "" {
			add(prefix+".district", "required", "district is required")
		}
	}
	checkParty("supplier", doc.Supplier, true)
	checkParty("receiver", doc.Customer, false)
	if len(doc.Number) != 16 {
		add("number", "invalid", "the invoice number must be 16 characters")
	}
	if doc.IssueDate == "" {
		add("issue_date", "required", "issue date is required")
	}
	if len(doc.Lines) == 0 {
		add("lines", "required", "at least one line is required")
	}
	for i, l := range doc.Lines {
		p := "lines." + strconv.Itoa(i)
		if strings.TrimSpace(l.Name) == "" {
			add(p+".name", "required", "description is required")
		}
		if l.Rate < 0 || l.Rate > 10000 {
			add(p+".vat_rate", "invalid", "VAT rate must be between 0 and 100")
		}
		if l.ExemptionCode != "" && !codelist.ValidExemption(l.ExemptionCode) && !codelist.ValidExemptionIstisna(l.ExemptionCode) {
			add(p+".exemption_code", "invalid", "unknown GİB exemption code")
		}
		if l.Rate == 0 && doc.Type != TypeIade {
			if l.ExemptionCode == "" {
				add(p+".exemption_code", "required", "a 0 % line needs an exemption code")
			}
			if strings.TrimSpace(l.ExemptionReason) == "" {
				add(p+".exemption_reason", "required", "a 0 % line needs an exemption reason")
			}
		}
		if l.Net.IsNegative() {
			add(p+".amount", "invalid", "amount must not be negative")
		}
	}
	if doc.Type == TypeIade && (doc.Returns == nil || len(doc.Returns.Number) != 16) {
		add("returns", "required", "a return invoice names the invoice it returns")
	}
	return out
}

// Validate runs Check, builds the document and checks the XML against the
// GİB XSD and Schematron (e-Arşiv rules for EARSIVFATURA). An empty result
// means the document may be sent; the XML is returned either way when it
// could be built.
func Validate(ctx context.Context, doc Document) ([]byte, Totals, []Problem, error) {
	if problems := Check(doc); len(problems) > 0 {
		return nil, Totals{}, problems, nil
	}
	data, totals, err := Build(doc)
	if err != nil {
		var be *builder.BuilderError
		if errors.As(err, &be) {
			return nil, Totals{}, []Problem{{Layer: "builder", Field: builderField(be.Field), Code: "invalid", Message: be.Error()}}, nil
		}
		return nil, Totals{}, nil, err
	}
	problems, err := ValidateXML(ctx, data, doc.Profile)
	return data, totals, problems, err
}

// ValidateXML checks built XML against the XSD and the Schematron.
func ValidateXML(ctx context.Context, data []byte, profile string) ([]Problem, error) {
	var out []Problem
	if err := ubltr.ValidateXSD(ctx, data); err != nil {
		var xe *ubltr.XSDValidationError
		if !errors.As(err, &xe) {
			return nil, err
		}
		for _, v := range xe.Violations {
			out = append(out, Problem{Layer: "xsd", Field: fieldOf(v.Path, v.Message), Code: "xsd", Message: v.Message, Path: v.Path})
		}
		// Schematron on a schema-invalid document only repeats the errors.
		return out, nil
	}
	docType := ubltr.TypeEFatura
	if profile == ProfileEArsiv {
		docType = ubltr.TypeEArchive
	}
	if err := ubltr.ValidateSchematronType(ctx, data, docType); err != nil {
		var se *ubltr.SchematronValidationError
		if !errors.As(err, &se) {
			return nil, err
		}
		for _, v := range se.Violations {
			code := "schematron"
			if v.RuleID != "" {
				code = v.RuleID
			}
			out = append(out, Problem{Layer: "schematron", Field: fieldOf(v.Path, v.Message), Code: code, Message: v.Message, Path: v.Path})
		}
	}
	return out, nil
}

func builderField(f string) string {
	switch {
	case strings.HasPrefix(f, "customer"):
		return "receiver"
	case strings.HasPrefix(f, "supplier"):
		return "supplier"
	case strings.HasPrefix(f, "line"):
		return "lines"
	case f == "issueDate":
		return "issue_date"
	case f == "id":
		return "number"
	}
	return "document"
}

var lineIndex = regexp.MustCompile(`InvoiceLine\[(\d+)\]`)

// fieldOf maps an XPath (or, when it names nothing specific, the message)
// onto a form field.
func fieldOf(path, msg string) string {
	if f := fieldIn(path); f != "document" {
		return f
	}
	return fieldIn(msg)
}

func fieldIn(s string) string {
	party := ""
	switch {
	case strings.Contains(s, "AccountingCustomerParty"):
		party = "receiver"
	case strings.Contains(s, "AccountingSupplierParty"), strings.Contains(s, "Signature"):
		party = "supplier"
	}
	if party != "" {
		switch {
		case strings.Contains(s, "CitySubdivisionName"):
			return party + ".district"
		case strings.Contains(s, "CityName"):
			return party + ".city"
		case strings.Contains(s, "PostalZone"):
			return party + ".postal_code"
		case strings.Contains(s, "Country"):
			return party + ".country"
		case strings.Contains(s, "PartyTaxScheme"):
			return party + ".tax_office"
		case strings.Contains(s, "PartyIdentification"):
			return party + ".identity_number"
		case strings.Contains(s, "PartyName"), strings.Contains(s, "Person"):
			return party + ".name"
		case strings.Contains(s, "ElectronicMail"):
			return party + ".email"
		}
		return party
	}
	if m := lineIndex.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		p := "lines." + strconv.Itoa(n-1)
		switch {
		case strings.Contains(s, "TaxExemptionReasonCode"):
			return p + ".exemption_code"
		case strings.Contains(s, "TaxExemptionReason"):
			return p + ".exemption_reason"
		case strings.Contains(s, "Percent"), strings.Contains(s, "TaxSubtotal"):
			return p + ".vat_rate"
		case strings.Contains(s, "Item"):
			return p + ".name"
		}
		return p
	}
	switch {
	case strings.Contains(s, "InvoiceLine"):
		return "lines"
	case strings.Contains(s, "TaxExemptionReason"):
		return "lines"
	case strings.Contains(s, "IssueDate"):
		return "issue_date"
	case strings.Contains(s, "ProfileID"):
		return "profile"
	case strings.Contains(s, "InvoiceTypeCode"):
		return "invoice_type"
	case strings.Contains(s, "BillingReference"):
		return "returns"
	}
	return "document"
}

func digits(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func validVKN(s string) bool { return digits(s, 10) }

func validTCKN(s string) bool { return digits(s, 11) && s[0] != '0' }
