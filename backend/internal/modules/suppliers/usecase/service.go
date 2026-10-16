// Package usecase implements the supplier cards (TEC-721,
// docs/design/suppliers.md): create / patch / archive, validation (VKN /
// TCKN checksum, IBAN, phone, e-mail), the generated code, the list with the
// payables figures and the free-text names that a new card adopts. Invoices
// and payments live in finance (they post to the ledger).
//
// Every method takes the organization id explicitly; another
// organization's uuid reads as not found.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound: no such supplier in the organization (404).
	ErrNotFound = errors.New("supplier not found")
	// ErrCodeTaken: the code is used by another supplier (409 on code).
	ErrCodeTaken = errors.New("the supplier code is already in use")
	// ErrTaxNumberTaken: an active supplier has this VKN / TCKN (409 on
	// tax_number).
	ErrTaxNumberTaken = errors.New("an active supplier already has this tax number")
)

// SortColumns are the sortable list columns.
var SortColumns = map[string]string{
	"code": "code", "title": "title", "balance": "balance", "open_amount": "open_amount",
	"overdue_amount": "overdue_amount", "next_due_date": "next_due_date", "created_at": "created_at",
}

// MaxListTargets caps exports and bulk "all matching".
const MaxListTargets = 10000

// DueSoonDays is the "due soon" window of the list.
const DueSoonDays = 7

var (
	codeRE    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)
	ibanRE    = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
	emailRE   = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	websiteRE = regexp.MustCompile(`^(https?://)?[A-Za-z0-9.-]+\.[A-Za-z]{2,}(/\S*)?$`)
)

// Detail is a field validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError answers 400.
type ValidationError struct{ Details []Detail }

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if e == nil || len(e.Details) == 0 {
		return nil
	}
	return e
}

// Finance is the ledger port: the supplier's account follows its card.
type Finance interface {
	SyncSupplierAccount(ctx context.Context, orgID, supplierID int64, title string, term int32) error
}

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service implements the supplier use cases.
type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	finance Finance
	indexer SearchIndexer
	events  resourceevents.Publisher
	log     *slog.Logger
	now     func() time.Time
}

// New builds the service. finance and indexer may be nil.
func New(pool *pgxpool.Pool, q *db.Queries, finance Finance, indexer SearchIndexer) *Service {
	return &Service{pool: pool, q: q, finance: finance, indexer: indexer, events: resourceevents.Nop{}, log: slog.Default(), now: time.Now}
}

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(p resourceevents.Publisher) {
	if p != nil {
		s.events = p
	}
}

// SetSearchIndexer wires the Cmd+K indexer (the worker builds it after the
// engines).
func (s *Service) SetSearchIndexer(i SearchIndexer) { s.indexer = i }

// SetLogger replaces the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) today() time.Time {
	l := s.now().In(rentaltime.Location())
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Service) publish(ctx context.Context, action string, id uuid.UUID) {
	s.events.Publish(ctx, model.Resource, action, id.String())
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, model.SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func uniqueConstraint(err error) (string, bool) {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return pe.ConstraintName, true
	}
	return "", false
}

// --------------------------------------------------------------- reading

// List lists suppliers with their payables figures (ledger balance over
// the entries in the caller's branch scope, open invoices of the scope).
func (s *Service) List(ctx context.Context, orgID int64, f model.Filter) ([]model.Supplier, int64, error) {
	sortField := f.SortField
	if _, ok := SortColumns[sortField]; !ok {
		sortField, f.SortDesc = "title", false
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	today := s.today()
	rows, err := s.q.ListSuppliers(ctx, db.ListSuppliersParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx),
		Today: pgtype.Date{Time: today, Valid: true}, Soon: pgtype.Date{Time: today.AddDate(0, 0, DueSoonDays), Valid: true},
		Ids: nilIDs(f.IDs), Types: nilStrings(f.Types), Statuses: nilStrings(f.Statuses), Cities: nilStrings(f.Cities),
		DueStates: nilStrings(f.DueStates), Q: textOf(f.Q), SortField: sortField, SortDesc: f.SortDesc,
		LimitCount: limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Supplier, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, supplierOf(r))
	}
	return out, total, nil
}

func supplierOf(r db.ListSuppliersRow) model.Supplier {
	bal := money.FromNumericOrZero(r.Balance)
	out := model.Supplier{
		UUID: r.Uuid, Code: r.Code, Title: r.Title, SupplierType: r.SupplierType, TaxNumber: r.TaxNumber,
		TaxKind: taxKind(r.TaxNumber), TaxOffice: r.TaxOffice, Phone: r.Phone, Email: r.Email, Website: r.Website,
		Address: r.Address, City: r.City, BillingAddress: r.BillingAddress, BillingEmail: r.BillingEmail,
		EInvoice: r.EInvoice, BankName: r.BankName, IBAN: r.Iban, PaymentTermDays: r.PaymentTermDays,
		Contacts: contactsOf(r.Contacts), Status: r.Status, Note: r.Note, Balance: bal, Payable: bal.Neg().ClampZero(),
		OpenAmount: money.FromNumericOrZero(r.OpenAmount), OverdueAmount: money.FromNumericOrZero(r.OverdueAmount),
		DueSoonAmount: money.FromNumericOrZero(r.DueSoonAmount), OpenInvoices: r.OpenInvoices,
		VehicleCount: r.VehicleCount, DueState: r.DueState, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if r.AccountUuid.Valid {
		out.Account = &model.Ref{UUID: r.AccountUuid.Bytes, Code: r.AccountCode.String}
	}
	if r.NextDueDate.Valid {
		d := r.NextDueDate.Time.Format("2006-01-02")
		out.NextDueDate = &d
	}
	return out
}

func contactsOf(raw []byte) []model.Contact {
	out := []model.Contact{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if out == nil {
		out = []model.Contact{}
	}
	return out
}

func taxKind(v string) string {
	switch len(v) {
	case 10:
		return model.TaxKindVKN
	case 11:
		return model.TaxKindTCKN
	}
	return ""
}

func (s *Service) row(ctx context.Context, orgID int64, id uuid.UUID) (db.Supplier, error) {
	row, err := s.q.GetSupplierByUUID(ctx, db.GetSupplierByUUIDParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

// Get returns one supplier with its figures.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Supplier, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Supplier{}, err
	}
	items, _, err := s.List(ctx, orgID, model.Filter{IDs: []int64{row.ID}, Limit: 1})
	if err != nil {
		return model.Supplier{}, err
	}
	if len(items) == 0 {
		return model.Supplier{}, ErrNotFound
	}
	return items[0], nil
}

// Facets returns the dynamic filter options.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.ListSupplierFacets(ctx, orgID)
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{SupplierType: []model.Facet{}, Status: []model.Facet{}, City: []model.Facet{}}
	for _, r := range rows {
		f := model.Facet{Value: r.Value, Count: r.Count}
		switch r.Facet {
		case "supplier_type":
			out.SupplierType = append(out.SupplierType, f)
		case "status":
			out.Status = append(out.Status, f)
		case "city":
			out.City = append(out.City, f)
		}
	}
	return out, nil
}

// Activity lists the audit trail of a supplier.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.row(ctx, orgID, id); err != nil {
		return nil, 0, err
	}
	org := pgtype.Int8{Int64: orgID, Valid: true}
	ru := pgtype.UUID{Bytes: id, Valid: true}
	rows, err := s.q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ru, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ru,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, r := range rows {
		e := model.ActivityEntry{UUID: r.Uuid, Action: r.Action, Payload: json.RawMessage(r.Payload), CreatedAt: r.CreatedAt.Time.UTC()}
		if len(e.Payload) == 0 {
			e.Payload = json.RawMessage("{}")
		}
		if r.ActorUuid.Valid {
			e.Actor = &model.ActivityActor{UUID: r.ActorUuid.Bytes, Name: r.ActorName.String, Surname: r.ActorSurname.String, Email: r.ActorEmail.String}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// ------------------------------------------------------------ validation

// ValidTaxNumber reports whether v (digits only) is a valid VKN (10 digits,
// GİB checksum) or TCKN (11 digits, two check digits).
func ValidTaxNumber(v string) bool {
	switch len(v) {
	case 10:
		return pii.ValidVKN(v)
	case 11:
		return pii.ValidTCKN(v)
	}
	return false
}

// ValidIBAN checks the format and the ISO 13616 mod-97 checksum.
func ValidIBAN(v string) bool {
	if !ibanRE.MatchString(v) {
		return false
	}
	re := v[4:] + v[:4]
	var b strings.Builder
	for _, r := range re {
		if r >= 'A' && r <= 'Z' {
			fmt.Fprintf(&b, "%d", r-'A'+10)
		} else {
			b.WriteRune(r)
		}
	}
	n, ok := new(big.Int).SetString(b.String(), 10)
	return ok && new(big.Int).Mod(n, big.NewInt(97)).Int64() == 1
}

func normalizeIBAN(v string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v), " ", ""))
}

func text(ve *ValidationError, field string, in *string, max int, dst *string) {
	if in == nil {
		return
	}
	v := strings.TrimSpace(*in)
	if utf8.RuneCountInString(v) > max {
		ve.add(field, "too_long", fmt.Sprintf("%s is at most %d characters", field, max))
		return
	}
	*dst = v
}

func phoneField(ve *ValidationError, field string, in *string, dst *string) {
	if in == nil {
		return
	}
	v := pii.Phone(*in)
	if v != "" && !pii.ValidPhone(v) {
		ve.add(field, "invalid", "phone number is not valid")
		return
	}
	*dst = v
}

func emailField(ve *ValidationError, field string, in *string, dst *string) {
	if in == nil {
		return
	}
	v := pii.Email(*in)
	if v != "" && (!emailRE.MatchString(v) || len(v) > 255) {
		ve.add(field, "invalid", "e-mail address is not valid")
		return
	}
	*dst = v
}

// apply validates in onto p (the current values on patch, defaults on
// create).
func (s *Service) apply(ve *ValidationError, p *db.InsertSupplierParams, in model.Input, create bool) {
	if in.Code != nil {
		code := strings.ToUpper(strings.TrimSpace(*in.Code))
		switch {
		case code == "" && create:
			p.Code = ""
		case !codeRE.MatchString(code):
			ve.add("code", "invalid", "code must be 1-32 characters: letters, digits, - _ .")
		default:
			p.Code = code
		}
	}
	if in.Title != nil || create {
		title := ""
		if in.Title != nil {
			title = strings.Join(strings.Fields(*in.Title), " ")
		}
		if title == "" || utf8.RuneCountInString(title) > 255 {
			ve.add("title", "required", "title is required (at most 255 characters)")
		} else {
			p.Title = title
		}
	}
	if in.SupplierType != nil {
		v := strings.TrimSpace(*in.SupplierType)
		if !contains(model.Types, v) {
			ve.add("supplier_type", "invalid", "supplier_type must be one of "+strings.Join(model.Types, ", "))
		} else {
			p.SupplierType = v
		}
	}
	if in.TaxNumber != nil {
		raw := strings.TrimSpace(*in.TaxNumber)
		d := pii.Digits(raw)
		switch {
		case raw == "":
			p.TaxNumber = ""
		case len(d) != len(strings.ReplaceAll(raw, " ", "")) || (len(d) != 10 && len(d) != 11):
			ve.add("tax_number", "invalid", "tax_number must be a 10-digit VKN or an 11-digit TCKN")
		case !ValidTaxNumber(d):
			ve.add("tax_number", "invalid_checksum", "the tax number checksum is invalid")
		default:
			p.TaxNumber = d
		}
	}
	text(ve, "tax_office", in.TaxOffice, 128, &p.TaxOffice)
	phoneField(ve, "phone", in.Phone, &p.Phone)
	emailField(ve, "email", in.Email, &p.Email)
	if in.Website != nil {
		v := strings.TrimSpace(*in.Website)
		if v != "" && (!websiteRE.MatchString(v) || len(v) > 255) {
			ve.add("website", "invalid", "website is not a valid address")
		} else {
			p.Website = v
		}
	}
	text(ve, "address", in.Address, 1000, &p.Address)
	text(ve, "city", in.City, 64, &p.City)
	text(ve, "billing_address", in.BillingAddress, 1000, &p.BillingAddress)
	emailField(ve, "billing_email", in.BillingEmail, &p.BillingEmail)
	if in.EInvoice != nil {
		p.EInvoice = *in.EInvoice
	}
	text(ve, "bank_name", in.BankName, 128, &p.BankName)
	if in.IBAN != nil {
		v := normalizeIBAN(*in.IBAN)
		if v != "" && !ValidIBAN(v) {
			ve.add("iban", "invalid", "IBAN is not valid")
		} else {
			p.Iban = v
		}
	}
	if in.PaymentTermDays != nil {
		if *in.PaymentTermDays < 0 || *in.PaymentTermDays > 365 {
			ve.add("payment_term_days", "out_of_range", "payment_term_days must be between 0 and 365")
		} else {
			p.PaymentTermDays = *in.PaymentTermDays
		}
	}
	if in.Contacts != nil {
		contacts := *in.Contacts
		if len(contacts) > model.MaxContacts {
			ve.add("contacts", "too_many", "at most 10 contact persons")
		}
		clean := make([]model.Contact, 0, len(contacts))
		for i, c := range contacts {
			field := fmt.Sprintf("contacts.%d", i)
			out := model.Contact{}
			name := strings.TrimSpace(c.Name)
			if name == "" || utf8.RuneCountInString(name) > 128 {
				ve.add(field+".name", "required", "name is required (at most 128 characters)")
			}
			out.Name = name
			text(ve, field+".role", &c.Role, 128, &out.Role)
			phoneField(ve, field+".phone", &c.Phone, &out.Phone)
			emailField(ve, field+".email", &c.Email, &out.Email)
			clean = append(clean, out)
		}
		raw, _ := json.Marshal(clean)
		p.Contacts = raw
	}
	if in.Status != nil {
		if !contains(model.Statuses, *in.Status) {
			ve.add("status", "invalid", "status must be active or archived")
		} else {
			p.Status = *in.Status
		}
	}
	text(ve, "note", in.Note, 2000, &p.Note)
}

// ----------------------------------------------------------------- writes

// Create adds a supplier. Without a code one is generated (TED-000001).
// The new card adopts the vehicles and expenses that name it in free text.
func (s *Service) Create(ctx context.Context, orgID int64, in model.Input) (model.Supplier, error) {
	ve := &ValidationError{}
	p := db.InsertSupplierParams{
		OrganizationID: orgID, SupplierType: model.TypeOther, PaymentTermDays: model.DefaultPaymentTermDays,
		Contacts: []byte("[]"), Status: model.StatusActive, CreatedBy: actorID(ctx),
	}
	s.apply(ve, &p, in, true)
	if err := ve.orNil(); err != nil {
		return model.Supplier{}, err
	}
	if p.TaxNumber != "" && p.Status == model.StatusActive {
		taken, err := s.q.SupplierTaxNumberTaken(ctx, db.SupplierTaxNumberTakenParams{OrganizationID: orgID, TaxNumber: p.TaxNumber})
		if err != nil {
			return model.Supplier{}, err
		}
		if taken {
			return model.Supplier{}, ErrTaxNumberTaken
		}
	}
	var row db.Supplier
	err := s.tx(ctx, func(q *db.Queries) error {
		if p.Code == "" {
			if err := q.LockOrganizationSupplierCodes(ctx, orgID); err != nil {
				return err
			}
			n, err := q.NextSupplierCodeNumber(ctx, orgID)
			if err != nil {
				return err
			}
			p.Code = fmt.Sprintf("TED-%06d", n)
		}
		var err error
		row, err = q.InsertSupplier(ctx, p)
		if _, ok := uniqueConstraint(err); ok {
			return ErrCodeTaken
		}
		if err != nil {
			return err
		}
		_, err = q.LinkFreeTextToSupplier(ctx, db.LinkFreeTextToSupplierParams{OrganizationID: orgID, SupplierID: row.ID, Title: row.Title})
		return err
	})
	if err != nil {
		return model.Supplier{}, err
	}
	s.publish(ctx, "created", row.Uuid)
	s.index(ctx, orgID, row.Uuid)
	return s.Get(ctx, orgID, row.Uuid)
}

// Patch changes a supplier. A new title follows onto the linked rows'
// free-text names and the ledger account.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.Input) (model.Supplier, error) {
	cur, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Supplier{}, err
	}
	ve := &ValidationError{}
	p := insertOf(cur)
	s.apply(ve, &p, in, false)
	if err := ve.orNil(); err != nil {
		return model.Supplier{}, err
	}
	if p.TaxNumber != "" && p.Status == model.StatusActive && (p.TaxNumber != cur.TaxNumber || cur.Status != model.StatusActive) {
		taken, err := s.q.SupplierTaxNumberTaken(ctx, db.SupplierTaxNumberTakenParams{
			OrganizationID: orgID, TaxNumber: p.TaxNumber, ExcludeID: pgtype.Int8{Int64: cur.ID, Valid: true},
		})
		if err != nil {
			return model.Supplier{}, err
		}
		if taken {
			return model.Supplier{}, ErrTaxNumberTaken
		}
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		row, err := q.UpdateSupplier(ctx, db.UpdateSupplierParams{
			OrganizationID: orgID, ID: cur.ID, Code: p.Code, Title: p.Title, SupplierType: p.SupplierType,
			TaxNumber: p.TaxNumber, TaxOffice: p.TaxOffice, Phone: p.Phone, Email: p.Email, Website: p.Website,
			Address: p.Address, City: p.City, BillingAddress: p.BillingAddress, BillingEmail: p.BillingEmail,
			EInvoice: p.EInvoice, BankName: p.BankName, Iban: p.Iban, PaymentTermDays: p.PaymentTermDays,
			Contacts: p.Contacts, Status: p.Status, Note: p.Note,
		})
		if _, ok := uniqueConstraint(err); ok {
			return ErrCodeTaken
		}
		if err != nil {
			return err
		}
		if row.Title != cur.Title {
			return q.RenameSupplierFreeText(ctx, db.RenameSupplierFreeTextParams{OrganizationID: orgID, SupplierID: cur.ID, Title: row.Title})
		}
		return nil
	})
	if err != nil {
		return model.Supplier{}, err
	}
	if s.finance != nil && (p.Title != cur.Title || p.PaymentTermDays != cur.PaymentTermDays) {
		if err := s.finance.SyncSupplierAccount(ctx, orgID, cur.ID, p.Title, p.PaymentTermDays); err != nil {
			s.log.Warn("supplier_account_sync_failed", "supplier", id.String(), "error", err)
		}
	}
	s.publish(ctx, "updated", id)
	s.index(ctx, orgID, id)
	return s.Get(ctx, orgID, id)
}

// SetStatus archives / activates a supplier (bulk); returns the previous
// status.
func (s *Service) SetStatus(ctx context.Context, orgID int64, id uuid.UUID, status string) (string, error) {
	if !contains(model.Statuses, status) {
		return "", &ValidationError{Details: []Detail{{Field: "status", Code: "invalid", Message: "status must be active or archived"}}}
	}
	cur, err := s.row(ctx, orgID, id)
	if err != nil {
		return "", err
	}
	if status == model.StatusActive && cur.Status != model.StatusActive && cur.TaxNumber != "" {
		taken, err := s.q.SupplierTaxNumberTaken(ctx, db.SupplierTaxNumberTakenParams{
			OrganizationID: orgID, TaxNumber: cur.TaxNumber, ExcludeID: pgtype.Int8{Int64: cur.ID, Valid: true},
		})
		if err != nil {
			return "", err
		}
		if taken {
			return "", ErrTaxNumberTaken
		}
	}
	prev, err := s.q.SetSupplierStatus(ctx, db.SetSupplierStatusParams{OrganizationID: orgID, ID: cur.ID, Status: status})
	if err != nil {
		return "", err
	}
	s.publish(ctx, "updated", id)
	s.index(ctx, orgID, id)
	return prev, nil
}

// ByNaturalKey finds the supplier an import row means: by code, then by
// tax number.
func (s *Service) ByNaturalKey(ctx context.Context, orgID int64, code, taxNumber string) (db.Supplier, bool, error) {
	if code = strings.TrimSpace(code); code != "" {
		row, err := s.q.GetSupplierByCode(ctx, db.GetSupplierByCodeParams{OrganizationID: orgID, Code: code})
		if err == nil {
			return row, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return row, false, err
		}
	}
	if d := pii.Digits(taxNumber); d != "" {
		row, err := s.q.GetSupplierByTaxNumber(ctx, db.GetSupplierByTaxNumberParams{OrganizationID: orgID, TaxNumber: d})
		if err == nil {
			return row, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return row, false, err
		}
	}
	return db.Supplier{}, false, nil
}

// RemoveImported undoes an imported (created) card: deleted while nothing
// refers to it, archived otherwise.
func (s *Service) RemoveImported(ctx context.Context, orgID int64, id uuid.UUID) error {
	cur, err := s.row(ctx, orgID, id)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteUnreferencedSupplier(ctx, db.DeleteUnreferencedSupplierParams{OrganizationID: orgID, ID: cur.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = s.SetStatus(ctx, orgID, id, model.StatusArchived)
		return err
	}
	s.publish(ctx, "deleted", id)
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, model.SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return nil
}

// ToModel maps a supplier row without figures (import snapshots).
func ToModel(r db.Supplier) model.Supplier {
	return model.Supplier{
		UUID: r.Uuid, Code: r.Code, Title: r.Title, SupplierType: r.SupplierType, TaxNumber: r.TaxNumber,
		TaxKind: taxKind(r.TaxNumber), TaxOffice: r.TaxOffice, Phone: r.Phone, Email: r.Email, Website: r.Website,
		Address: r.Address, City: r.City, BillingAddress: r.BillingAddress, BillingEmail: r.BillingEmail,
		EInvoice: r.EInvoice, BankName: r.BankName, IBAN: r.Iban, PaymentTermDays: r.PaymentTermDays,
		Contacts: contactsOf(r.Contacts), Status: r.Status, Note: r.Note,
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
}

func insertOf(r db.Supplier) db.InsertSupplierParams {
	return db.InsertSupplierParams{
		OrganizationID: r.OrganizationID, Code: r.Code, Title: r.Title, SupplierType: r.SupplierType,
		TaxNumber: r.TaxNumber, TaxOffice: r.TaxOffice, Phone: r.Phone, Email: r.Email, Website: r.Website,
		Address: r.Address, City: r.City, BillingAddress: r.BillingAddress, BillingEmail: r.BillingEmail,
		EInvoice: r.EInvoice, BankName: r.BankName, Iban: r.Iban, PaymentTermDays: r.PaymentTermDays,
		Contacts: r.Contacts, Status: r.Status, Note: r.Note,
	}
}

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("suppliers: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func contains(set []string, v string) bool {
	for _, x := range set {
		if x == v {
			return true
		}
	}
	return false
}

func nilIDs(v []int64) []int64 {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilStrings(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func textOf(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	return pgtype.Text{String: v, Valid: v != ""}
}
