// Package adapters connects suppliers to the shared engines: ioengine
// (export / import by code or tax number), bulkengine (archive) and
// searchengine (Cmd+K by title, code and tax number). The organization
// always comes from the context (request scope or job row).
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Resource is the engine slug of suppliers.
const Resource = model.Resource

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

var errNoOrganization = errors.New("suppliers: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// FilterFromQuery maps list query params to a filter.
func FilterFromQuery(q map[string]string) model.Filter {
	return model.Filter{
		Q: strings.TrimSpace(q["q"]), Types: apiquery.SplitCSV(q["supplier_type"]), Statuses: apiquery.SplitCSV(q["status"]),
		Cities: apiquery.SplitCSV(q["city"]), DueStates: apiquery.SplitCSV(q["due_state"]),
	}
}

// QueryKeys are the list filters exports and bulk honour.
var QueryKeys = []string{"q", "supplier_type", "status", "city", "due_state"}

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

func label(locale i18n.Locale, prefix, v string) string {
	if v == "" {
		return ""
	}
	key := prefix + v
	if t := i18n.Translate(locale, key); t != key {
		return t
	}
	return v
}

// ---------------------------------------------------------------- io engine

// IO exports and imports suppliers (an import row updates the supplier
// with the same code, or else the same tax number, and creates otherwise).
type IO struct{ svc *usecase.Service }

// NewIO builds the io adapter.
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

func (a *IO) Resource() string { return Resource }

var ioKeys = []string{
	"code", "title", "supplier_type", "tax_number", "tax_office", "phone", "email", "website", "address", "city",
	"billing_address", "billing_email", "e_invoice", "bank_name", "iban", "payment_term_days", "status", "note",
}

func (a *IO) ExportColumns() []ioengine.Column {
	out := make([]ioengine.Column, 0, len(ioKeys)+5)
	for _, k := range ioKeys {
		out = append(out, ioengine.Column{Key: k, LabelKey: "suppliers." + k, Type: ioengine.ColumnTypeString})
	}
	for _, k := range []string{"contacts", "payable", "open_amount", "overdue_amount"} {
		out = append(out, ioengine.Column{Key: k, LabelKey: "suppliers." + k, Type: ioengine.ColumnTypeString})
	}
	return append(out, ioengine.Column{Key: "uuid", LabelKey: "suppliers.uuid", Type: ioengine.ColumnTypeUUID})
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := FilterFromQuery(query)
	f.Limit, f.SortField = usecase.MaxListTargets, "code"
	items, _, err := a.svc.List(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		contacts := make([]string, 0, len(it.Contacts))
		for _, c := range it.Contacts {
			contacts = append(contacts, strings.TrimSpace(strings.Join([]string{c.Name, c.Role, c.Phone, c.Email}, " ")))
		}
		einv := i18n.Translate(locale, "finance.no")
		if it.EInvoice {
			einv = i18n.Translate(locale, "finance.yes")
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "code": it.Code, "title": it.Title,
			"supplier_type": label(locale, "suppliers.type.", it.SupplierType), "tax_number": it.TaxNumber,
			"tax_office": it.TaxOffice, "phone": it.Phone, "email": it.Email, "website": it.Website, "address": it.Address,
			"city": it.City, "billing_address": it.BillingAddress, "billing_email": it.BillingEmail, "e_invoice": einv,
			"bank_name": it.BankName, "iban": it.IBAN, "payment_term_days": strconv.Itoa(int(it.PaymentTermDays)),
			"status": label(locale, "suppliers.status.", it.Status), "note": it.Note, "contacts": strings.Join(contacts, "; "),
			"payable": it.Payable.String(), "open_amount": it.OpenAmount.String(), "overdue_amount": it.OverdueAmount.String(),
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *IO) ImportSchema() []ioengine.ImportField {
	out := make([]ioengine.ImportField, 0, len(ioKeys))
	for _, k := range ioKeys {
		f := ioengine.ImportField{Key: k, LabelKey: "suppliers." + k, Type: ioengine.ColumnTypeString, Required: k == "title"}
		switch k {
		case "supplier_type":
			f.DefaultHint = model.TypeOther
		case "payment_term_days":
			f.DefaultHint = strconv.Itoa(model.DefaultPaymentTermDays)
		case "status":
			f.DefaultHint = model.StatusActive
		}
		out = append(out, f)
	}
	return out
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{
		{"title": "Örnek Oto Servis Ltd. Şti.", "supplier_type": "service", "tax_number": "1234567890", "tax_office": "Kadıköy",
			"phone": "0216 555 00 00", "email": "info@ornekservis.com.tr", "city": "İstanbul", "payment_term_days": "30"},
		{"code": "SIG-01", "title": "Örnek Sigorta A.Ş.", "supplier_type": "insurance", "iban": "TR330006100519786457841326",
			"payment_term_days": "15", "e_invoice": "true"},
	}
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "evet", "var":
		return true, nil
	case "0", "false", "no", "hayır", "hayir", "yok":
		return false, nil
	}
	return false, fmt.Errorf("e_invoice must be true or false")
}

func inputFromRow(row, defaults map[string]any) (model.Input, error) {
	get := func(key string) *string {
		if v := cell(row, key); v != "" {
			return &v
		}
		if v := cell(defaults, key); v != "" {
			return &v
		}
		return nil
	}
	in := model.Input{
		Title: get("title"), SupplierType: get("supplier_type"), TaxNumber: get("tax_number"), TaxOffice: get("tax_office"),
		Phone: get("phone"), Email: get("email"), Website: get("website"), Address: get("address"), City: get("city"),
		BillingAddress: get("billing_address"), BillingEmail: get("billing_email"), BankName: get("bank_name"),
		IBAN: get("iban"), Status: get("status"), Note: get("note"),
	}
	if c := get("code"); c != nil {
		in.Code = c
	}
	if v := get("e_invoice"); v != nil {
		b, err := parseBool(*v)
		if err != nil {
			return in, err
		}
		in.EInvoice = &b
	}
	if v := get("payment_term_days"); v != nil {
		n, err := strconv.Atoi(*v)
		if err != nil {
			return in, fmt.Errorf("payment_term_days is not a whole number")
		}
		d := int32(n)
		in.PaymentTermDays = &d
	}
	return in, nil
}

func inputFromSupplier(sp model.Supplier) model.Input {
	contacts := sp.Contacts
	return model.Input{
		Code: &sp.Code, Title: &sp.Title, SupplierType: &sp.SupplierType, TaxNumber: &sp.TaxNumber, TaxOffice: &sp.TaxOffice,
		Phone: &sp.Phone, Email: &sp.Email, Website: &sp.Website, Address: &sp.Address, City: &sp.City,
		BillingAddress: &sp.BillingAddress, BillingEmail: &sp.BillingEmail, EInvoice: &sp.EInvoice, BankName: &sp.BankName,
		IBAN: &sp.IBAN, PaymentTermDays: &sp.PaymentTermDays, Contacts: &contacts, Status: &sp.Status, Note: &sp.Note,
	}
}

func snapshot(sp model.Supplier) map[string]any {
	raw, err := json.Marshal(sp)
	if err != nil {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func rowError(err error) (ioengine.RowResult, error) {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) || errors.Is(err, usecase.ErrCodeTaken) || errors.Is(err, usecase.ErrTaxNumberTaken) ||
		errors.Is(err, usecase.ErrNotFound) {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	return ioengine.RowResult{}, err
}

// ApplyRow creates a supplier, or updates the one with the same code (or
// tax number).
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, err := inputFromRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	cur, found, err := a.svc.ByNaturalKey(ctx, orgID, cell(row, "code"), cell(row, "tax_number"))
	if err != nil {
		return ioengine.RowResult{}, err
	}
	if found {
		prev := usecase.ToModel(cur)
		if _, err := a.svc.Patch(ctx, orgID, cur.Uuid, in); err != nil {
			return rowError(err)
		}
		return ioengine.RowResult{OK: true, EntityType: "supplier", EntityUUID: cur.Uuid.String(), Op: "update", Previous: snapshot(prev)}, nil
	}
	sp, err := a.svc.Create(ctx, orgID, in)
	if err != nil {
		return rowError(err)
	}
	return ioengine.RowResult{OK: true, EntityType: "supplier", EntityUUID: sp.UUID.String(), Op: "create"}, nil
}

// RevertRow removes (or archives, when referenced) an imported supplier or
// restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "supplier" {
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
		return a.svc.RemoveImported(ctx, orgID, id)
	}
	raw, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	var prev model.Supplier
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	_, err = a.svc.Patch(ctx, orgID, id, inputFromSupplier(prev))
	return err
}

// -------------------------------------------------------------- bulk engine

// Bulk archives suppliers (reversible: the rollback restores the status).
type Bulk struct{ svc *usecase.Service }

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk { return &Bulk{svc: svc} }

func (a *Bulk) Resource() string { return Resource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{{
		ID: "archive", LabelKey: "bulk.actions.suppliers.archive", Permission: rbac.PermTenantSuppliersBulkArchive,
		Reversible: true, ConfirmKey: "bulk.confirm.suppliers.archive",
	}}
}

func (a *Bulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	if action != "archive" {
		return nil, fmt.Errorf("unknown action")
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
		f := FilterFromQuery(target.Query)
		f.Limit = MaxBulkTargets
		items, _, err := a.svc.List(ctx, orgID, f)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, it.UUID.String())
		}
		return out, nil
	}
	return nil, fmt.Errorf("invalid target scope")
}

func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "invalid uuid"}, nil
	}
	if action != "archive" {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "unknown action"}, nil
	}
	prev, err := a.svc.SetStatus(ctx, orgID, id, model.StatusArchived)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "supplier", OK: true, Op: "update", Previous: map[string]any{"status": prev},
	}, nil
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
	prev := cell(previous, "status")
	if prev == "" {
		return fmt.Errorf("missing previous status")
	}
	_, err = a.svc.SetStatus(ctx, orgID, id, prev)
	return err
}

// ------------------------------------------------------------ search engine

// Search indexes suppliers (title, code, tax number) for the tenant Cmd+K
// palette. Suppliers belong to the organization, not to a branch.
type Search struct{ q *db.Queries }

// NewSearch builds the search adapter.
func NewSearch(q *db.Queries) *Search { return &Search{q: q} }

func (a *Search) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID: model.SearchSpec, LabelKey: "search.specs_suppliers", Permission: rbac.PermTenantSuppliersRead,
		Icon: "suppliers", Searchable: []string{"title", "subtitle", "keywords"}, Scope: searchengine.ScopeTenant,
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
		rows, err := a.q.ListSupplierSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetSupplierSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one supplier by its tenant document id ("<org>_<uuid>").
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("suppliers search: invalid uuid")
	}
	row, err := a.q.GetSupplierSearchDocument(ctx, db.GetSupplierSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetSupplierSearchDocumentRow) searchengine.Document {
	subtitle := strings.Trim(row.Code+" · "+row.City, " ·")
	keywords := []string{row.Code, row.Title}
	if row.TaxNumber != "" {
		keywords = append(keywords, row.TaxNumber)
	}
	if row.Status != model.StatusActive {
		subtitle = strings.Trim(subtitle+" · "+row.Status, " ·")
	}
	return searchengine.Document{
		ID: searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()), Spec: model.SearchSpec, Title: row.Title,
		Subtitle: subtitle, Keywords: keywords, Href: "/t/" + row.OrganizationSlug + "/finance/suppliers/" + row.Uuid.String(),
		Icon: "suppliers", OrganizationID: row.OrganizationID,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
