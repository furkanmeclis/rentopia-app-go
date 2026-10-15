package usecase

import (
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/jackc/pgx/v5/pgtype"
)

// Columns (associated data) and blind index purposes of the encrypted
// values. The organization is part of both, so a ciphertext or an index
// never matches across organizations.
const (
	colCustomerIdentity = "customers.identity"
	colDriverIdentity   = "drivers.identity"
	colDriverLicence    = "drivers.licence"

	bidxCustomerIdentity = "customer.identity"
	bidxDriverIdentity   = "driver.identity"
	bidxDriverLicence    = "driver.licence"
)

func aad(column string, orgID int64) string {
	return column + "|org:" + strconv.FormatInt(orgID, 10)
}

func bidxScope(orgID int64) string {
	return "org:" + strconv.FormatInt(orgID, 10)
}

// sealed is an encrypted value with its blind index and masked hint.
type sealed struct {
	enc, bidx, hint string
}

// seal encrypts a normalized value. kind prefixes the blind index input so
// a TCKN and a passport with the same characters never collide.
func (s *Service) seal(orgID int64, column, purpose, kind, normalized string) (sealed, error) {
	if s.cipher == nil {
		return sealed{}, ErrCipherUnavailable
	}
	enc, err := s.cipher.Encrypt(normalized, aad(column, orgID))
	if err != nil {
		return sealed{}, err
	}
	maskKind := kind
	if column == colDriverLicence {
		maskKind = pii.KindLicence
	}
	return sealed{
		enc:  enc,
		bidx: s.cipher.BlindIndex(purpose, bidxScope(orgID), kind+":"+normalized),
		hint: pii.MaskIdentity(maskKind, normalized),
	}, nil
}

// open decrypts a stored value ("" stays "").
func (s *Service) open(orgID int64, column, enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	if s.cipher == nil {
		return "", ErrCipherUnavailable
	}
	return s.cipher.Decrypt(enc, aad(column, orgID))
}

// identityIndex is the blind index of a customer identity number (import
// matching, lookups).
func (s *Service) identityIndex(orgID int64, kind, normalized string) string {
	if s.cipher == nil || normalized == "" {
		return ""
	}
	return s.cipher.BlindIndex(bidxCustomerIdentity, bidxScope(orgID), kind+":"+normalized)
}

// identityCandidates are the blind indexes a free-text query would have if
// it were a full identity number (TCKN / VKN by checksum, passport by
// shape). Partial numbers match nothing: exact match only.
func (s *Service) identityCandidates(orgID int64, q string) []string {
	out := []string{}
	d := pii.Digits(q)
	if len(d) == len(strings.TrimSpace(strings.ReplaceAll(q, " ", ""))) {
		if pii.ValidTCKN(d) {
			out = append(out, s.identityIndex(orgID, model.IdentityTCKN, d))
		}
		if pii.ValidVKN(d) {
			out = append(out, s.identityIndex(orgID, model.IdentityVKN, d))
		}
	}
	if doc := pii.Document(q); len(doc) >= 5 && len(doc) <= 20 && strings.IndexFunc(doc, unicode.IsLetter) >= 0 {
		out = append(out, s.identityIndex(orgID, model.IdentityPassport, doc))
	}
	return out
}

// digitsQuery returns the digits of a query that looks like (part of) a
// phone number, "" otherwise. A national leading 0 is dropped so "0532 12"
// finds +90532 12….
func digitsQuery(q string) string {
	for _, r := range q {
		if !unicode.IsDigit(r) && !strings.ContainsRune(" +-()", r) {
			return ""
		}
	}
	d := pii.Digits(q)
	if len(d) < 3 {
		return ""
	}
	return strings.TrimPrefix(d, "0")
}

// normalizeIdentity validates and normalizes an identity number of a kind.
func normalizeIdentity(field, kind, raw string) (string, *Detail) {
	switch kind {
	case model.IdentityTCKN:
		d := pii.Digits(raw)
		if len(d) != 11 {
			return "", &Detail{Field: field, Code: "invalid", Message: "TCKN must have 11 digits"}
		}
		if !pii.ValidTCKN(d) {
			return "", &Detail{Field: field, Code: "invalid_checksum", Message: "TCKN checksum is invalid"}
		}
		return d, nil
	case model.IdentityVKN:
		d := pii.Digits(raw)
		if len(d) != 10 {
			return "", &Detail{Field: field, Code: "invalid", Message: "VKN must have 10 digits"}
		}
		if !pii.ValidVKN(d) {
			return "", &Detail{Field: field, Code: "invalid_checksum", Message: "VKN checksum is invalid"}
		}
		return d, nil
	case model.IdentityPassport:
		doc := pii.Document(raw)
		if len(doc) < 5 || len(doc) > 20 {
			return "", &Detail{Field: field, Code: "invalid", Message: "passport number must have 5 to 20 letters or digits"}
		}
		return doc, nil
	}
	return "", &Detail{Field: field, Code: "invalid", Message: "unknown identity type"}
}

// allowedIdentity lists the identity kinds of a customer type.
func allowedIdentity(customerType string) []string {
	if customerType == model.TypeCorporate {
		return []string{model.IdentityVKN, model.IdentityTCKN}
	}
	return []string{model.IdentityTCKN, model.IdentityPassport}
}

// applyCustomer validates the input and writes it into f (encrypting the
// identity number). create marks a new customer (required fields).
func (s *Service) applyCustomer(orgID int64, f *repository.CustomerFields, in model.CustomerInput, create bool) error {
	ve := &ValidationError{}
	if in.CustomerType != nil {
		t := strings.TrimSpace(*in.CustomerType)
		if !model.Contains(model.CustomerTypes, t) {
			ve.add("customer_type", "invalid", "customer_type must be individual or corporate")
		} else {
			f.CustomerType = t
		}
	}
	applyText(ve, "first_name", in.FirstName, 128, &f.FirstName)
	applyText(ve, "last_name", in.LastName, 128, &f.LastName)
	applyText(ve, "company_title", in.CompanyTitle, 255, &f.CompanyTitle)
	switch f.CustomerType {
	case model.TypeIndividual:
		if f.FirstName == "" {
			ve.add("first_name", "required", "first_name is required")
		}
		if f.LastName == "" {
			ve.add("last_name", "required", "last_name is required")
		}
	case model.TypeCorporate:
		if f.CompanyTitle == "" {
			ve.add("company_title", "required", "company_title is required")
		}
	}

	// Identity: number and type together; an empty number clears it.
	kind := f.IdentityType.String
	if !f.IdentityType.Valid {
		kind = ""
	}
	if in.IdentityType.Set {
		kind = ""
		if in.IdentityType.Value != nil {
			kind = strings.TrimSpace(*in.IdentityType.Value)
		}
	}
	switch {
	case in.IdentityNumber != nil && strings.TrimSpace(*in.IdentityNumber) == "":
		f.IdentityType, f.IdentityEnc, f.IdentityBidx, f.IdentityHint = pgtype.Text{}, "", "", ""
	case in.IdentityNumber != nil:
		if kind == "" {
			ve.add("identity_type", "required", "identity_type is required with identity_number")
			break
		}
		if !model.Contains(allowedIdentity(f.CustomerType), kind) {
			ve.add("identity_type", "invalid", "identity_type must be one of "+strings.Join(allowedIdentity(f.CustomerType), ", "))
			break
		}
		norm, d := normalizeIdentity("identity_number", kind, *in.IdentityNumber)
		if d != nil {
			ve.Details = append(ve.Details, *d)
			break
		}
		sv, err := s.seal(orgID, colCustomerIdentity, bidxCustomerIdentity, kind, norm)
		if err != nil {
			return err
		}
		f.IdentityType = pgtype.Text{String: kind, Valid: true}
		f.IdentityEnc, f.IdentityBidx, f.IdentityHint = sv.enc, sv.bidx, sv.hint
	case in.IdentityType.Set && kind != f.IdentityType.String:
		if kind == "" {
			f.IdentityType, f.IdentityEnc, f.IdentityBidx, f.IdentityHint = pgtype.Text{}, "", "", ""
		} else {
			ve.add("identity_number", "required", "identity_number is required when identity_type changes")
		}
	}
	if f.IdentityType.Valid && !model.Contains(allowedIdentity(f.CustomerType), f.IdentityType.String) {
		ve.add("identity_type", "invalid", "identity_type must be one of "+strings.Join(allowedIdentity(f.CustomerType), ", "))
	}

	applyCountry(ve, "nationality", in.Nationality, &f.Nationality)
	if f.IdentityType.Valid && f.IdentityType.String == model.IdentityPassport && !f.Nationality.Valid {
		ve.add("nationality", "required", "nationality is required with a passport")
	}
	applyText(ve, "tax_office", in.TaxOffice, 128, &f.TaxOffice)
	applyPhone(ve, "phone", in.Phone, &f.Phone)
	applyEmail(ve, "email", in.Email, &f.Email)
	applyText(ve, "billing_address", in.BillingAddress, 1000, &f.BillingAddress)
	applyText(ve, "billing_city", in.BillingCity, 128, &f.BillingCity)
	applyText(ve, "billing_district", in.BillingDistrict, 128, &f.BillingDistrict)
	applyText(ve, "billing_postal_code", in.BillingPostalCode, 16, &f.BillingPostalCode)
	applyCountry(ve, "billing_country", in.BillingCountry, &f.BillingCountry)
	if in.EInvoice != nil {
		f.EInvoice = *in.EInvoice
	}
	applyText(ve, "note", in.Note, 5000, &f.Note)
	if in.Tags != nil {
		tags, err := normalizeTags(*in.Tags)
		if err != nil {
			ve.add("tags", "invalid", err.Error())
		} else {
			f.Tags = tags
		}
	}
	_ = create
	return ve.orNil()
}

func applyText(ve *ValidationError, field string, in *string, max int, dst *string) {
	if in == nil {
		return
	}
	v := strings.TrimSpace(*in)
	if utf8.RuneCountInString(v) > max {
		ve.add(field, "too_long", field+" must be at most "+strconv.Itoa(max)+" characters")
		return
	}
	*dst = v
}

func applyCountry(ve *ValidationError, field string, in model.OptionalString, dst *pgtype.Text) {
	if !in.Set {
		return
	}
	if in.Value == nil || strings.TrimSpace(*in.Value) == "" {
		*dst = pgtype.Text{}
		return
	}
	v := strings.ToUpper(strings.TrimSpace(*in.Value))
	if len(v) != 2 || v[0] < 'A' || v[0] > 'Z' || v[1] < 'A' || v[1] > 'Z' {
		ve.add(field, "invalid", field+" must be an ISO 3166-1 alpha-2 country code")
		return
	}
	*dst = pgtype.Text{String: v, Valid: true}
}

func applyPhone(ve *ValidationError, field string, in *string, dst *string) {
	if in == nil {
		return
	}
	if strings.TrimSpace(*in) == "" {
		*dst = ""
		return
	}
	v := pii.Phone(*in)
	if !pii.ValidPhone(v) {
		ve.add(field, "invalid", "phone number is invalid")
		return
	}
	*dst = v
}

func applyEmail(ve *ValidationError, field string, in *string, dst *string) {
	if in == nil {
		return
	}
	v := pii.Email(*in)
	if v == "" {
		*dst = ""
		return
	}
	if addr, err := mail.ParseAddress(v); err != nil || addr.Address != v || len(v) > 255 {
		ve.add(field, "invalid", "email is invalid")
		return
	}
	*dst = v
}

func applyDate(ve *ValidationError, field string, in model.OptionalString, dst *pgtype.Date) {
	if !in.Set {
		return
	}
	if in.Value == nil || strings.TrimSpace(*in.Value) == "" {
		*dst = pgtype.Date{}
		return
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(*in.Value))
	if err != nil {
		ve.add(field, "invalid", field+" must be a date (YYYY-MM-DD)")
		return
	}
	*dst = pgtype.Date{Time: t, Valid: true}
}

// normalizeTags trims, deduplicates (case-insensitive) and caps tags.
func normalizeTags(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > 40 {
			return nil, errTag("a tag must be at most 40 characters")
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	if len(out) > 20 {
		return nil, errTag("at most 20 tags are allowed")
	}
	return out, nil
}

type errTag string

func (e errTag) Error() string { return string(e) }

func sortStrings(v []string) {
	sort.Strings(v)
}

// --- mapping (always masked) -------------------------------------------------------

// DisplayName is the company title of a corporate customer, else the
// person's name. An anonymized customer has none.
func DisplayName(customerType, first, last, title string) string {
	if customerType == model.TypeCorporate && title != "" {
		return title
	}
	return strings.TrimSpace(first + " " + last)
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	v := d.Time.Format("2006-01-02")
	return &v
}

func mapCustomer(c db.Customer) model.Customer {
	tags := c.Tags
	if tags == nil {
		tags = []string{}
	}
	out := model.Customer{
		UUID: c.Uuid, CustomerType: c.CustomerType,
		DisplayName: DisplayName(c.CustomerType, c.FirstName, c.LastName, c.CompanyTitle),
		FirstName:   c.FirstName, LastName: c.LastName, CompanyTitle: c.CompanyTitle,
		IdentityType: textPtr(c.IdentityType), IdentityMasked: c.IdentityHint, Nationality: textPtr(c.Nationality),
		TaxOffice: c.TaxOffice, PhoneMasked: pii.MaskPhone(c.Phone), EmailMasked: pii.MaskEmail(c.Email),
		Billing: model.Billing{
			Address: c.BillingAddress, City: c.BillingCity, District: c.BillingDistrict,
			PostalCode: c.BillingPostalCode, Country: textPtr(c.BillingCountry), EInvoice: c.EInvoice,
		},
		Note: c.Note, Tags: tags, ConsentChannels: []string{}, Anonymized: c.AnonymizedAt.Valid,
		CreatedAt: c.CreatedAt.Time.UTC(), UpdatedAt: c.UpdatedAt.Time.UTC(),
	}
	if c.AnonymizedAt.Valid {
		t := c.AnonymizedAt.Time.UTC()
		out.AnonymizedAt = &t
	}
	return out
}

func mapListRow(r repository.ListRow) model.Customer {
	out := mapCustomer(db.Customer{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, CustomerType: r.CustomerType,
		FirstName: r.FirstName, LastName: r.LastName, CompanyTitle: r.CompanyTitle, IdentityType: r.IdentityType,
		IdentityHint: r.IdentityHint, Nationality: r.Nationality, TaxOffice: r.TaxOffice, Phone: r.Phone,
		Email: r.Email, BillingAddress: r.BillingAddress, BillingCity: r.BillingCity,
		BillingDistrict: r.BillingDistrict, BillingPostalCode: r.BillingPostalCode, BillingCountry: r.BillingCountry,
		EInvoice: r.EInvoice, Note: r.Note, Tags: r.Tags, AnonymizedAt: r.AnonymizedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
	if r.ConsentChannels != nil {
		out.ConsentChannels = r.ConsentChannels
	}
	out.DriversCount = r.DriversCount
	return out
}
