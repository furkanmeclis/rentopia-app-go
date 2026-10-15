// Package adapters connects the customers resource to the shared engines:
// ioengine (export, İdes customer list import with a dry-run diff matched by
// the identity blind index), bulkengine (add / remove tag) and searchengine
// (Cmd+K by name, masked identity and phone last digits). The organization
// always comes from the context (orgctx); without one every call fails.
//
// Privacy: exports carry masked identity / phone / e-mail values unless the
// query has include_sensitive=true AND the job scope grants
// tenant.customers.reveal (the HTTP route records the request as an
// activity event). Search documents never hold a plaintext identity number,
// phone or e-mail.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

// Resource is the engine slug of customers.
const Resource = model.Resource

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = usecase.MaxListTargets

var errNoOrganization = errors.New("customers: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// IncludeSensitive reports whether an export may carry plaintext values:
// the query asks for it and the (job) scope grants tenant.customers.reveal.
func IncludeSensitive(ctx context.Context, query map[string]string) bool {
	if !strings.EqualFold(strings.TrimSpace(query["include_sensitive"]), "true") {
		return false
	}
	scope, ok := orgctx.From(ctx)
	return ok && scope.Has(rbac.PermTenantCustomersReveal)
}

// ---------------------------------------------------------------- io engine

// IO exports customers and imports an İdes customer list (or the export).
type IO struct {
	svc *usecase.Service
}

// NewIO builds the io adapter.
func NewIO(svc *usecase.Service) *IO {
	return &IO{svc: svc}
}

func (a *IO) Resource() string { return Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "customers." + key, Type: t}
	}
	s, b := ioengine.ColumnTypeString, ioengine.ColumnTypeBoolean
	return []ioengine.Column{
		col("display_name", s), col("customer_type", s), col("first_name", s), col("last_name", s),
		col("company_title", s), col("identity_type", s), col("identity_number", s), col("nationality", s),
		col("tax_office", s), col("phone", s), col("email", s), col("billing_address", s), col("billing_city", s),
		col("billing_district", s), col("billing_postal_code", s), col("billing_country", s), col("e_invoice", b),
		col("tags", s), col("consent_channels", s), col("drivers_count", s), col("anonymized", b),
		col("uuid", ioengine.ColumnTypeUUID), col("created_at", ioengine.ColumnTypeDatetime),
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportRows(ctx, orgID, model.FilterFromQuery(query), IncludeSensitive(ctx, query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		c := r.Customer
		identity, phone, email := c.IdentityMasked, c.PhoneMasked, c.EmailMasked
		if r.Plain != nil {
			identity, phone, email = r.Plain.IdentityNumber, r.Plain.Phone, r.Plain.Email
		}
		out = append(out, map[string]any{
			"uuid": c.UUID.String(), "display_name": c.DisplayName, "customer_type": c.CustomerType,
			"first_name": c.FirstName, "last_name": c.LastName, "company_title": c.CompanyTitle,
			"identity_type": str(c.IdentityType), "identity_number": identity, "nationality": str(c.Nationality),
			"tax_office": c.TaxOffice, "phone": phone, "email": email, "billing_address": c.Billing.Address,
			"billing_city": c.Billing.City, "billing_district": c.Billing.District,
			"billing_postal_code": c.Billing.PostalCode, "billing_country": str(c.Billing.Country),
			"e_invoice": c.Billing.EInvoice, "tags": strings.Join(c.Tags, ", "),
			"consent_channels": strings.Join(c.ConsentChannels, ", "), "drivers_count": strconv.FormatInt(c.DriversCount, 10),
			"anonymized": c.Anonymized, "created_at": c.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema follows the İdes customer list XLSX (müşteri tipi, ad, soyad,
// unvan, TC kimlik, vergi no, pasaport, uyruk, telefon, e-posta, vergi
// dairesi, adres, il, ilçe, etiketler, not).
func (a *IO) ImportSchema() []ioengine.ImportField {
	f := func(key string) ioengine.ImportField {
		return ioengine.ImportField{Key: key, LabelKey: "customers." + key, Type: ioengine.ColumnTypeString}
	}
	ctype := f("customer_type")
	ctype.DefaultHint = model.TypeIndividual
	return []ioengine.ImportField{
		ctype, f("first_name"), f("last_name"), f("company_title"), f("tckn"), f("vkn"), f("passport_no"),
		f("nationality"), f("phone"), f("email"), f("tax_office"), f("billing_address"), f("billing_city"),
		f("billing_district"), f("tags"), f("note"),
	}
}

// SampleRows implements ioengine.SampleProvider (checksum-valid example
// numbers, not real people).
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{
		{
			"customer_type": "Bireysel", "first_name": "Ayşe", "last_name": "Yılmaz", "company_title": "",
			"tckn": "10000000146", "vkn": "", "passport_no": "", "nationality": "TR", "phone": "0532 000 00 00",
			"email": "ayse@example.com", "tax_office": "", "billing_address": "Örnek Mah. 1. Sok. No:1",
			"billing_city": "İstanbul", "billing_district": "Kadıköy", "tags": "VIP", "note": "",
		},
		{
			"customer_type": "Kurumsal", "first_name": "", "last_name": "", "company_title": "Örnek Lojistik A.Ş.",
			"tckn": "", "vkn": "4540536920", "passport_no": "", "nationality": "TR", "phone": "0212 000 00 00",
			"email": "filo@example.com", "tax_office": "Kadıköy", "billing_address": "Örnek Cad. No:2",
			"billing_city": "İstanbul", "billing_district": "Ataşehir", "tags": "Filo", "note": "",
		},
	}
}

// ApplyRow creates a customer, or updates the one with the same identity
// number (blind index match).
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	id, created, prev, err := a.svc.ImportRow(ctx, orgID, inputFromRow(row, defaults))
	if err != nil {
		if rowError(err) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	res := ioengine.RowResult{OK: true, EntityType: "customer", EntityUUID: id.String(), Op: "create"}
	if !created {
		res.Op = "update"
		res.Previous = prev
	}
	return res, nil
}

// PreviewRow implements ioengine.RowPreviewer (masked values in changes).
func (a *IO) PreviewRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowDiff, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowDiff{}, err
	}
	p, err := a.svc.PreviewImport(ctx, orgID, inputFromRow(row, defaults))
	if err != nil {
		if rowError(err) {
			return ioengine.RowDiff{Error: err.Error()}, nil
		}
		return ioengine.RowDiff{}, err
	}
	out := ioengine.RowDiff{Op: p.Op, Changes: make([]ioengine.FieldChange, 0, len(p.Changes))}
	for _, c := range p.Changes {
		out.Changes = append(out.Changes, ioengine.FieldChange{Field: c.Field, From: c.From, To: c.To})
	}
	return out, nil
}

// RevertRow deletes an imported customer or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "customer" {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	if len(previous) == 0 {
		return a.svc.Delete(ctx, orgID, id)
	}
	return a.svc.RestoreSnapshot(ctx, orgID, id, previous)
}

func rowError(err error) bool {
	var ve *usecase.ValidationError
	return errors.As(err, &ve) || errors.Is(err, usecase.ErrIdentityTaken) || errors.Is(err, usecase.ErrNotFound) ||
		errors.Is(err, usecase.ErrAnonymized)
}

func inputFromRow(row, defaults map[string]any) usecase.ImportInput {
	get := func(key string) string {
		if v := cell(row, key); v != "" {
			return v
		}
		return cell(defaults, key)
	}
	return usecase.ImportInput{
		CustomerType: get("customer_type"), FirstName: get("first_name"), LastName: get("last_name"),
		CompanyTitle: get("company_title"), TCKN: get("tckn"), VKN: get("vkn"), Passport: get("passport_no"),
		Nationality: get("nationality"), Phone: get("phone"), Email: get("email"), TaxOffice: get("tax_office"),
		BillingAddress: get("billing_address"), BillingCity: get("billing_city"), BillingDist: get("billing_district"),
		Tags: get("tags"), Note: get("note"),
	}
}

func cell(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func str(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// -------------------------------------------------------------- bulk engine

// Bulk action ids. Params (BulkTarget.params): add_tag / remove_tag {tag}.
const (
	ActionAddTag    = "add_tag"
	ActionRemoveTag = "remove_tag"
)

// Bulk tags customers in bulk; both actions are reversible.
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk {
	return &Bulk{svc: svc}
}

func (a *Bulk) Resource() string { return Resource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	def := func(id string) bulkengine.BulkActionDef {
		return bulkengine.BulkActionDef{
			ID: id, LabelKey: "bulk.actions.customers." + id, Permission: rbac.PermTenantCustomersWrite, Reversible: true,
		}
	}
	return []bulkengine.BulkActionDef{def(ActionAddTag), def(ActionRemoveTag)}
}

// ResolveTargets validates the tag param and resolves the customers.
func (a *Bulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	tag := strings.TrimSpace(target.Params["tag"])
	if tag == "" || len([]rune(tag)) > 40 {
		return nil, fmt.Errorf("params.tag is required (at most 40 characters)")
	}
	switch target.Scope {
	case "ids":
		seen := map[string]bool{}
		out := make([]string, 0, len(target.IDs))
		for _, raw := range target.IDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil || seen[id.String()] {
				continue
			}
			seen[id.String()] = true
			out = append(out, id.String())
		}
		if len(out) > MaxBulkTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", MaxBulkTargets)
		}
		return out, nil
	case "query":
		ids, err := a.svc.TargetUUIDs(ctx, orgID, model.FilterFromQuery(target.Query), MaxBulkTargets)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, id.String())
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid target scope")
	}
}

func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	tag := strings.TrimSpace(bulkengine.Params(ctx)["tag"])
	current, err := a.svc.Get(ctx, orgID, id)
	if err != nil {
		return a.fail(entityUUID, err)
	}
	next := make([]string, 0, len(current.Tags)+1)
	found := false
	for _, t := range current.Tags {
		if strings.EqualFold(t, tag) {
			found = true
			if action == ActionRemoveTag {
				continue
			}
		}
		next = append(next, t)
	}
	switch action {
	case ActionAddTag:
		if !found {
			next = append(next, tag)
		}
	case ActionRemoveTag:
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	prev, err := a.svc.SetTags(ctx, orgID, id, next)
	if err != nil {
		return a.fail(entityUUID, err)
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "customer", OK: true, Op: "update", Previous: map[string]any{"tags": prev},
	}, nil
}

func (a *Bulk) fail(entityUUID string, err error) (bulkengine.BulkItemResult, error) {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) || errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrAnonymized) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return bulkengine.BulkItemResult{}, err
}

func (a *Bulk) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	raw, _ := previous["tags"].([]any)
	tags := make([]string, 0, len(raw))
	for _, v := range raw {
		tags = append(tags, fmt.Sprint(v))
	}
	_, err = a.svc.SetTags(ctx, orgID, id, tags)
	return err
}

// ------------------------------------------------------------ search engine

// Search indexes customers for the tenant Cmd+K palette: the name, the
// masked identity hint and its last digits, the last four phone digits.
// Customers are organization-wide (not branch scoped): restricted members
// find them at the counter; opening the record still masks everything.
type Search struct {
	q *db.Queries
}

// NewSearch builds the search adapter.
func NewSearch(q *db.Queries) *Search {
	return &Search{q: q}
}

func (a *Search) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         usecase.SearchSpec,
		LabelKey:   "search.specs_customers",
		Permission: rbac.PermTenantCustomersRead,
		Icon:       "customers",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
	}
}

// ListAll walks every live organization (each query is organization-scoped).
func (a *Search) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	orgIDs, err := a.q.ListLiveOrganizationIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0)
	for _, orgID := range orgIDs {
		rows, err := a.q.ListCustomerSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, SearchDocument(db.GetCustomerSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one customer by its tenant document id ("<org>_<uuid>").
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("customers search: invalid uuid")
	}
	row, err := a.q.GetCustomerSearchDocument(ctx, db.GetCustomerSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return SearchDocument(row), nil
}

// SearchDocument builds the Cmd+K document of a customer. It holds no
// plaintext identity number, phone or e-mail: the masked identity hint, the
// last digits of the hint and the last four phone digits only.
func SearchDocument(row db.GetCustomerSearchDocumentRow) searchengine.Document {
	title := usecase.DisplayName(row.CustomerType, row.FirstName, row.LastName, row.CompanyTitle)
	keywords := []string{}
	if row.AnonymizedAt.Valid {
		title = ""
	} else {
		if row.FirstName != "" {
			keywords = append(keywords, row.FirstName, row.LastName)
		}
		if row.CompanyTitle != "" {
			keywords = append(keywords, row.CompanyTitle)
		}
		if tail := pii.LastDigits(row.IdentityHint, 2); tail != "" {
			keywords = append(keywords, tail)
		}
		if tail := pii.LastDigits(row.Phone, 4); len(tail) == 4 {
			keywords = append(keywords, tail)
		}
	}
	subtitle := row.IdentityHint
	if p := pii.MaskPhone(row.Phone); p != "" {
		if subtitle != "" {
			subtitle += " · "
		}
		subtitle += p
	}
	if title == "" {
		title = "—"
		subtitle = ""
	}
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          title,
		Subtitle:       subtitle,
		Keywords:       keywords,
		Href:           "/t/" + row.OrganizationSlug + "/customers/" + row.Uuid.String(),
		Icon:           "customers",
		OrganizationID: row.OrganizationID,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ ioengine.RowPreviewer    = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
