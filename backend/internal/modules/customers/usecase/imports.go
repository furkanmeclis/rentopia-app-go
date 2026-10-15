package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- export ----------------------------------------------------------------------

// ExportRow is one exported customer. Plain is set only for an export with
// include_sensitive (the adapter checks tenant.customers.reveal).
type ExportRow struct {
	Customer model.Customer
	Plain    *PlainValues
}

// PlainValues are the decrypted sensitive values of a customer.
type PlainValues struct {
	IdentityNumber string
	Phone          string
	Email          string
}

// ExportRows returns the customers matching a list filter (export engine),
// name order, capped at MaxListTargets. Sensitive values are masked unless
// includeSensitive.
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter, includeSensitive bool) ([]ExportRow, error) {
	f.Limit, f.Offset, f.SortField, f.SortDesc = MaxListTargets, 0, "display_name", false
	rows, _, err := s.repo.ListCustomers(ctx, orgID, s.listParams(orgID, f))
	if err != nil {
		return nil, err
	}
	out := make([]ExportRow, 0, len(rows))
	for _, row := range rows {
		er := ExportRow{Customer: mapListRow(row)}
		if includeSensitive {
			identity, err := s.open(orgID, colCustomerIdentity, row.IdentityEnc)
			if err != nil {
				return nil, err
			}
			er.Plain = &PlainValues{IdentityNumber: identity, Phone: row.Phone, Email: row.Email}
		}
		out = append(out, er)
	}
	return out, nil
}

// --- import (İdes customer list XLSX or the export format) ---------------------------

// ImportInput is one import row (strings as read from the file).
type ImportInput struct {
	CustomerType   string
	FirstName      string
	LastName       string
	CompanyTitle   string
	TCKN           string
	VKN            string
	Passport       string
	Nationality    string
	Phone          string
	Email          string
	TaxOffice      string
	BillingAddress string
	BillingCity    string
	BillingDist    string
	Tags           string
	Note           string
}

// FieldChange is one field an import row would change (sensitive values
// appear masked).
type FieldChange struct {
	Field string
	From  string
	To    string
}

// ImportPreview is the dry-run result of one row.
type ImportPreview struct {
	Op      string
	Changes []FieldChange
}

// importSnapshot is the stored previous state of an updated customer (import
// rollback). Identity values are kept encrypted: the snapshot never holds
// a plaintext identity number.
type importSnapshot struct {
	UUID              string   `json:"uuid"`
	CustomerType      string   `json:"customer_type"`
	FirstName         string   `json:"first_name"`
	LastName          string   `json:"last_name"`
	CompanyTitle      string   `json:"company_title"`
	IdentityType      *string  `json:"identity_type"`
	IdentityEnc       string   `json:"identity_enc"`
	IdentityBidx      string   `json:"identity_bidx"`
	IdentityHint      string   `json:"identity_hint"`
	Nationality       *string  `json:"nationality"`
	TaxOffice         string   `json:"tax_office"`
	Phone             string   `json:"phone"`
	Email             string   `json:"email"`
	BillingAddress    string   `json:"billing_address"`
	BillingCity       string   `json:"billing_city"`
	BillingDistrict   string   `json:"billing_district"`
	BillingPostalCode string   `json:"billing_postal_code"`
	BillingCountry    *string  `json:"billing_country"`
	EInvoice          bool     `json:"e_invoice"`
	Note              string   `json:"note"`
	Tags              []string `json:"tags"`
}

// customerTypeAliases maps İdes / Turkish labels to customer types.
var customerTypeAliases = map[string]string{
	"individual": model.TypeIndividual, "bireysel": model.TypeIndividual, "şahıs": model.TypeIndividual,
	"sahis": model.TypeIndividual, "kişi": model.TypeIndividual, "kisi": model.TypeIndividual,
	"corporate": model.TypeCorporate, "kurumsal": model.TypeCorporate, "şirket": model.TypeCorporate,
	"sirket": model.TypeCorporate, "firma": model.TypeCorporate, "tüzel": model.TypeCorporate,
}

// importPlan resolves a row: the input to apply and the existing customer
// matched by the blind index of its identity number (nil = create).
func (s *Service) importPlan(ctx context.Context, orgID int64, in ImportInput) (model.CustomerInput, *db.Customer, error) {
	out := model.CustomerInput{}
	t := strings.ToLower(strings.TrimSpace(in.CustomerType))
	ctype := customerTypeAliases[t]
	switch {
	case t != "" && ctype == "":
		return out, nil, validation("customer_type", "invalid", "customer_type must be individual (bireysel) or corporate (kurumsal)")
	case ctype == "" && strings.TrimSpace(in.CompanyTitle) != "" && strings.TrimSpace(in.FirstName) == "":
		ctype = model.TypeCorporate
	case ctype == "":
		ctype = model.TypeIndividual
	}
	out.CustomerType = &ctype
	set := func(dst **string, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = &v
		}
	}
	set(&out.FirstName, in.FirstName)
	set(&out.LastName, in.LastName)
	set(&out.CompanyTitle, in.CompanyTitle)
	set(&out.Phone, in.Phone)
	set(&out.Email, in.Email)
	set(&out.TaxOffice, in.TaxOffice)
	set(&out.BillingAddress, in.BillingAddress)
	set(&out.BillingCity, in.BillingCity)
	set(&out.BillingDistrict, in.BillingDist)
	set(&out.Note, in.Note)
	if v := strings.TrimSpace(in.Nationality); v != "" {
		out.Nationality = model.String(v)
	}
	if v := strings.TrimSpace(in.Tags); v != "" {
		tags := model.SplitList(strings.ReplaceAll(v, ";", ","))
		out.Tags = &tags
	}
	kind, number := "", ""
	switch {
	case strings.TrimSpace(in.TCKN) != "":
		kind, number = model.IdentityTCKN, in.TCKN
	case strings.TrimSpace(in.VKN) != "":
		kind, number = model.IdentityVKN, in.VKN
	case strings.TrimSpace(in.Passport) != "":
		kind, number = model.IdentityPassport, in.Passport
	}
	if kind == "" {
		return out, nil, nil
	}
	out.IdentityType = model.String(kind)
	out.IdentityNumber = &number
	norm, d := normalizeIdentity("identity_number", kind, number)
	if d != nil {
		return out, nil, &ValidationError{Details: []Detail{*d}}
	}
	existing, err := s.repo.GetCustomerByIdentity(ctx, orgID, s.identityIndex(orgID, kind, norm))
	if errors.Is(err, repository.ErrNotFound) {
		return out, nil, nil
	}
	if err != nil {
		return out, nil, err
	}
	if t == "" {
		// The file does not name a type: keep the matched customer's.
		out.CustomerType = nil
	}
	return out, &existing, nil
}

// PreviewImport validates a row and lists what it would create or change
// without writing (identity / phone / e-mail changes are shown masked).
func (s *Service) PreviewImport(ctx context.Context, orgID int64, in ImportInput) (ImportPreview, error) {
	input, existing, err := s.importPlan(ctx, orgID, in)
	if err != nil {
		return ImportPreview{}, err
	}
	base := repository.CustomerFields{CustomerType: model.TypeIndividual, Tags: []string{}}
	op := "create"
	if existing != nil {
		if existing.AnonymizedAt.Valid {
			return ImportPreview{}, ErrAnonymized
		}
		base = repository.FieldsFromCustomer(*existing)
		op = "update"
	}
	next := base
	if err := s.applyCustomer(orgID, &next, input, existing == nil); err != nil {
		return ImportPreview{}, err
	}
	return ImportPreview{Op: op, Changes: diffFields(base, next, op == "create")}, nil
}

// ImportRow creates a customer, or updates the customer with the same
// identity number. prev is the snapshot of an updated customer (rollback).
func (s *Service) ImportRow(ctx context.Context, orgID int64, in ImportInput) (uuid.UUID, bool, map[string]any, error) {
	input, existing, err := s.importPlan(ctx, orgID, in)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	if existing == nil {
		c, err := s.Create(ctx, orgID, input)
		if err != nil {
			return uuid.Nil, false, nil, err
		}
		return c.UUID, true, nil, nil
	}
	if existing.AnonymizedAt.Valid {
		return uuid.Nil, false, nil, ErrAnonymized
	}
	prev := snapshotOf(*existing)
	if _, err := s.Patch(ctx, orgID, existing.Uuid, input); err != nil {
		return uuid.Nil, false, nil, err
	}
	return existing.Uuid, false, prev, nil
}

// RestoreSnapshot writes an import snapshot back (rollback of an updated
// customer).
func (s *Service) RestoreSnapshot(ctx context.Context, orgID int64, id uuid.UUID, previous map[string]any) error {
	raw, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	var snap importSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return err
	}
	if _, err := s.liveCustomer(ctx, orgID, id); err != nil {
		return err
	}
	f := repository.CustomerFields{
		CustomerType: snap.CustomerType, FirstName: snap.FirstName, LastName: snap.LastName,
		CompanyTitle: snap.CompanyTitle, IdentityType: textArg(snap.IdentityType), IdentityEnc: snap.IdentityEnc,
		IdentityBidx: snap.IdentityBidx, IdentityHint: snap.IdentityHint, Nationality: textArg(snap.Nationality),
		TaxOffice: snap.TaxOffice, Phone: snap.Phone, Email: snap.Email, BillingAddress: snap.BillingAddress,
		BillingCity: snap.BillingCity, BillingDistrict: snap.BillingDistrict, BillingPostalCode: snap.BillingPostalCode,
		BillingCountry: textArg(snap.BillingCountry), EInvoice: snap.EInvoice, Note: snap.Note, Tags: snap.Tags,
	}
	if f.Tags == nil {
		f.Tags = []string{}
	}
	if _, err := s.repo.UpdateCustomer(ctx, orgID, id, f); err != nil {
		return mapErr(err)
	}
	s.index(ctx, orgID, id)
	return nil
}

func snapshotOf(c db.Customer) map[string]any {
	snap := importSnapshot{
		UUID: c.Uuid.String(), CustomerType: c.CustomerType, FirstName: c.FirstName, LastName: c.LastName,
		CompanyTitle: c.CompanyTitle, IdentityType: textPtr(c.IdentityType), IdentityEnc: c.IdentityEnc,
		IdentityBidx: c.IdentityBidx, IdentityHint: c.IdentityHint, Nationality: textPtr(c.Nationality),
		TaxOffice: c.TaxOffice, Phone: c.Phone, Email: c.Email, BillingAddress: c.BillingAddress,
		BillingCity: c.BillingCity, BillingDistrict: c.BillingDistrict, BillingPostalCode: c.BillingPostalCode,
		BillingCountry: textPtr(c.BillingCountry), EInvoice: c.EInvoice, Note: c.Note, Tags: c.Tags,
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func textArg(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

// diffFields lists the changed fields between two states; sensitive values
// are masked.
func diffFields(a, b repository.CustomerFields, create bool) []FieldChange {
	out := []FieldChange{}
	add := func(field, from, to string) {
		if from != to || (create && to != "") {
			out = append(out, FieldChange{Field: field, From: from, To: to})
		}
	}
	txt := func(t pgtype.Text) string {
		if !t.Valid {
			return ""
		}
		return t.String
	}
	add("customer_type", a.CustomerType, b.CustomerType)
	add("first_name", a.FirstName, b.FirstName)
	add("last_name", a.LastName, b.LastName)
	add("company_title", a.CompanyTitle, b.CompanyTitle)
	if a.IdentityBidx != b.IdentityBidx || create {
		add("identity_number", a.IdentityHint, b.IdentityHint)
	}
	add("nationality", txt(a.Nationality), txt(b.Nationality))
	add("tax_office", a.TaxOffice, b.TaxOffice)
	add("phone", pii.MaskPhone(a.Phone), pii.MaskPhone(b.Phone))
	add("email", pii.MaskEmail(a.Email), pii.MaskEmail(b.Email))
	add("billing_address", a.BillingAddress, b.BillingAddress)
	add("billing_city", a.BillingCity, b.BillingCity)
	add("billing_district", a.BillingDistrict, b.BillingDistrict)
	add("tags", strings.Join(a.Tags, ", "), strings.Join(b.Tags, ", "))
	add("note", a.Note, b.Note)
	return out
}
