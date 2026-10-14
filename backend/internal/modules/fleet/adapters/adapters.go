// Package adapters connects the fleet resources to the shared engines:
// ioengine (export of segments, rental groups and products), bulkengine
// (activate / deactivate groups and products) and searchengine (Cmd+K for
// products). The organization always comes from the context
// (orgctx.OrganizationID): the request scope on tenant routes, the job row in
// the worker. Without one every adapter call fails.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = usecase.MaxListTargets

var (
	errNoOrganization = errors.New("fleet: organization scope is required")
	errNoImport       = errors.New("fleet: import is not supported for this resource")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

func strPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func int16Ptr(v *int16) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(int(*v))
}

// exportOnly implements the import half of ioengine.ResourceAdapter for
// export-only resources (import routes are not mounted).
type exportOnly struct{}

func (exportOnly) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (exportOnly) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (exportOnly) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// ---------------------------------------------------------------- segments

// SegmentsIO exports segments.
type SegmentsIO struct {
	exportOnly
	svc *usecase.Service
}

// NewSegmentsIO builds the segments io adapter.
func NewSegmentsIO(svc *usecase.Service) *SegmentsIO { return &SegmentsIO{svc: svc} }

func (a *SegmentsIO) Resource() string { return model.ResourceSegments }

func (a *SegmentsIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "code", LabelKey: "fleet.code", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "fleet.name", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "fleet.description", Type: ioengine.ColumnTypeString},
		{Key: "sort_order", LabelKey: "fleet.sort_order", Type: ioengine.ColumnTypeString},
		{Key: "groups_count", LabelKey: "fleet.groups_count", Type: ioengine.ColumnTypeString},
		{Key: "products_count", LabelKey: "fleet.products_count", Type: ioengine.ColumnTypeString},
		{Key: "is_active", LabelKey: "fleet.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "uuid", LabelKey: "fleet.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "fleet.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *SegmentsIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.SegmentExportRows(ctx, orgID, model.SegmentFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, s := range rows {
		out = append(out, map[string]any{
			"uuid": s.UUID.String(), "code": s.Code, "name": s.Name, "description": s.Description,
			"sort_order": strconv.Itoa(int(s.SortOrder)), "groups_count": strconv.FormatInt(s.GroupsCount, 10),
			"products_count": strconv.FormatInt(s.ProductsCount, 10), "is_active": s.IsActive, "created_at": s.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceSegments, Columns: a.ExportColumns(), Rows: out}, nil
}

// ----------------------------------------------------------- rental groups

// GroupsIO exports rental groups.
type GroupsIO struct {
	exportOnly
	svc *usecase.Service
}

// NewGroupsIO builds the rental groups io adapter.
func NewGroupsIO(svc *usecase.Service) *GroupsIO { return &GroupsIO{svc: svc} }

func (a *GroupsIO) Resource() string { return model.ResourceRentalGroups }

func (a *GroupsIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "code", LabelKey: "fleet.code", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "fleet.name", Type: ioengine.ColumnTypeString},
		{Key: "segment", LabelKey: "fleet.segment", Type: ioengine.ColumnTypeString},
		{Key: "transmission", LabelKey: "fleet.transmission", Type: ioengine.ColumnTypeString},
		{Key: "fuel", LabelKey: "fleet.fuel", Type: ioengine.ColumnTypeString},
		{Key: "body_type", LabelKey: "fleet.body_type", Type: ioengine.ColumnTypeString},
		{Key: "min_seats", LabelKey: "fleet.min_seats", Type: ioengine.ColumnTypeString},
		{Key: "min_luggage", LabelKey: "fleet.min_luggage", Type: ioengine.ColumnTypeString},
		{Key: "air_conditioning", LabelKey: "fleet.air_conditioning", Type: ioengine.ColumnTypeBoolean},
		{Key: "acriss_code", LabelKey: "fleet.acriss_code", Type: ioengine.ColumnTypeString},
		{Key: "catalog_models", LabelKey: "fleet.catalog_models", Type: ioengine.ColumnTypeString},
		{Key: "catalog_types", LabelKey: "fleet.catalog_types", Type: ioengine.ColumnTypeString},
		{Key: "products_count", LabelKey: "fleet.products_count", Type: ioengine.ColumnTypeString},
		{Key: "is_active", LabelKey: "fleet.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "uuid", LabelKey: "fleet.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "fleet.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *GroupsIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.GroupExportRows(ctx, orgID, model.GroupFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, g := range rows {
		models := make([]string, 0, len(g.CatalogModels))
		for _, m := range g.CatalogModels {
			models = append(models, m.Label)
		}
		types := make([]string, 0, len(g.CatalogTypes))
		for _, t := range g.CatalogTypes {
			types = append(types, t.Label)
		}
		out = append(out, map[string]any{
			"uuid": g.UUID.String(), "code": g.Code, "name": g.Name, "segment": g.Segment.Name,
			"transmission": strPtr(g.Transmission), "fuel": strPtr(g.Fuel), "body_type": strPtr(g.BodyType),
			"min_seats": int16Ptr(g.MinSeats), "min_luggage": int16Ptr(g.MinLuggage),
			"air_conditioning": g.AirConditioning, "acriss_code": strPtr(g.AcrissCode),
			"catalog_models": strings.Join(models, ", "), "catalog_types": strings.Join(types, ", "),
			"products_count": strconv.FormatInt(g.ProductsCount, 10), "is_active": g.IsActive,
			"created_at": g.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceRentalGroups, Columns: a.ExportColumns(), Rows: out}, nil
}

// ---------------------------------------------------------- rental products

// ProductsIO exports rental products with their current version.
type ProductsIO struct {
	exportOnly
	svc *usecase.Service
}

// NewProductsIO builds the rental products io adapter.
func NewProductsIO(svc *usecase.Service) *ProductsIO { return &ProductsIO{svc: svc} }

func (a *ProductsIO) Resource() string { return model.ResourceRentalProducts }

func (a *ProductsIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "code", LabelKey: "fleet.code", Type: ioengine.ColumnTypeString},
		{Key: "title", LabelKey: "fleet.title", Type: ioengine.ColumnTypeString},
		{Key: "segment", LabelKey: "fleet.segment", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "fleet.status", Type: ioengine.ColumnTypeString},
		{Key: "guarantee_type", LabelKey: "fleet.guarantee_type", Type: ioengine.ColumnTypeString},
		{Key: "display_text", LabelKey: "fleet.display_text", Type: ioengine.ColumnTypeString},
		{Key: "current_version", LabelKey: "fleet.current_version", Type: ioengine.ColumnTypeString},
		{Key: "deposit_amount", LabelKey: "fleet.deposit_amount", Type: ioengine.ColumnTypeString},
		{Key: "min_driver_age", LabelKey: "fleet.min_driver_age", Type: ioengine.ColumnTypeString},
		{Key: "min_license_years", LabelKey: "fleet.min_license_years", Type: ioengine.ColumnTypeString},
		{Key: "included_km_per_day", LabelKey: "fleet.included_km_per_day", Type: ioengine.ColumnTypeString},
		{Key: "extra_km_fee", LabelKey: "fleet.extra_km_fee", Type: ioengine.ColumnTypeString},
		{Key: "fuel_policy", LabelKey: "fleet.fuel_policy", Type: ioengine.ColumnTypeString},
		{Key: "is_active", LabelKey: "fleet.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "uuid", LabelKey: "fleet.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "fleet.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *ProductsIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ProductExportRows(ctx, orgID, model.ProductFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		c := p.Version.Conditions
		km := ""
		if c.IncludedKmPerDay != nil {
			km = strconv.Itoa(int(*c.IncludedKmPerDay))
		}
		out = append(out, map[string]any{
			"uuid": p.UUID.String(), "code": p.Code, "title": p.Title, "segment": p.Segment.Name,
			"status": p.Status, "guarantee_type": p.Version.Guarantee.Type, "display_text": p.DisplayText,
			"current_version": strconv.Itoa(int(p.CurrentVersion)), "deposit_amount": c.DepositAmount,
			"min_driver_age": strconv.Itoa(int(c.MinDriverAge)), "min_license_years": strconv.Itoa(int(c.MinLicenseYears)),
			"included_km_per_day": km, "extra_km_fee": c.ExtraKmFee, "fuel_policy": c.FuelPolicy,
			"is_active": p.IsActive, "created_at": p.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceRentalProducts, Columns: a.ExportColumns(), Rows: out}, nil
}

// -------------------------------------------------------------- bulk engine

// activeBulk activates / deactivates one resource (groups or products).
type activeBulk struct {
	resource   string
	entityType string
	labelBase  string
	targets    func(ctx context.Context, orgID int64, query map[string]string) ([]uuid.UUID, error)
	setActive  func(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error)
	notFound   error
}

// NewGroupsBulk builds the rental groups bulk adapter. svc may be nil for
// metadata only.
func NewGroupsBulk(svc *usecase.Service) bulkengine.BulkAdapter {
	b := &activeBulk{
		resource: model.ResourceRentalGroups, entityType: "rental_group", labelBase: "rental_groups",
		notFound: usecase.ErrGroupNotFound,
	}
	if svc != nil {
		b.targets = func(ctx context.Context, orgID int64, query map[string]string) ([]uuid.UUID, error) {
			return svc.GroupTargetUUIDs(ctx, orgID, model.GroupFilterFromQuery(query))
		}
		b.setActive = svc.SetGroupActive
	}
	return b
}

// NewProductsBulk builds the rental products bulk adapter. svc may be nil
// for metadata only.
func NewProductsBulk(svc *usecase.Service) bulkengine.BulkAdapter {
	b := &activeBulk{
		resource: model.ResourceRentalProducts, entityType: "rental_product", labelBase: "rental_products",
		notFound: usecase.ErrProductNotFound,
	}
	if svc != nil {
		b.targets = func(ctx context.Context, orgID int64, query map[string]string) ([]uuid.UUID, error) {
			return svc.ProductTargetUUIDs(ctx, orgID, model.ProductFilterFromQuery(query))
		}
		b.setActive = svc.SetProductActive
	}
	return b
}

func (a *activeBulk) Resource() string { return a.resource }

func (a *activeBulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions." + a.labelBase + ".activate",
			Permission: rbac.PermTenantFleetWrite, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions." + a.labelBase + ".deactivate",
			Permission: rbac.PermTenantFleetWrite, Reversible: true,
			ConfirmKey: "bulk.confirm." + a.labelBase + ".deactivate",
		},
	}
}

func (a *activeBulk) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
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
		if a.targets == nil {
			return nil, fmt.Errorf("fleet bulk: no service")
		}
		ids, err := a.targets(ctx, orgID, target.Query)
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

func (a *activeBulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	var active bool
	switch action {
	case "activate":
		active = true
	case "deactivate":
		active = false
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	if a.setActive == nil {
		return bulkengine.BulkItemResult{}, fmt.Errorf("fleet bulk: no service")
	}
	prev, err := a.setActive(ctx, orgID, id, active)
	if err != nil {
		if errors.Is(err, a.notFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: a.entityType, OK: true, Op: "update",
		Previous: map[string]any{"is_active": prev},
	}, nil
}

func (a *activeBulk) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	prev, ok := previous["is_active"].(bool)
	if !ok {
		return fmt.Errorf("missing previous is_active")
	}
	if a.setActive == nil {
		return fmt.Errorf("fleet bulk: no service")
	}
	_, err = a.setActive(ctx, orgID, id, prev)
	return err
}

// ------------------------------------------------------------ search engine

// ProductsSearch indexes rental products for the tenant Cmd+K palette.
type ProductsSearch struct {
	q *db.Queries
}

// NewProductsSearch builds the search adapter.
func NewProductsSearch(q *db.Queries) *ProductsSearch { return &ProductsSearch{q: q} }

func (a *ProductsSearch) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         usecase.SearchSpecProducts,
		LabelKey:   "search.specs_rental_products",
		Permission: rbac.PermTenantFleetRead,
		Icon:       "products",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
	}
}

// ListAll walks every live organization (each query is organization-scoped).
func (a *ProductsSearch) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	orgIDs, err := a.q.ListLiveOrganizationIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0)
	for _, orgID := range orgIDs {
		rows, err := a.q.ListRentalProductSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, productDocument(db.GetRentalProductSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one product by its tenant document id ("<org>_<uuid>").
func (a *ProductsSearch) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("rental products search: invalid uuid")
	}
	row, err := a.q.GetRentalProductSearchDocument(ctx, db.GetRentalProductSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return productDocument(row), nil
}

func productDocument(row db.GetRentalProductSearchDocumentRow) searchengine.Document {
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpecProducts,
		Title:          row.Code + " · " + row.Title,
		Subtitle:       row.SegmentName,
		Keywords:       []string{row.Code, row.Title, row.SegmentName, row.Status},
		Href:           "/t/" + row.OrganizationSlug + "/fleet/products/" + row.Uuid.String(),
		Icon:           "products",
		OrganizationID: row.OrganizationID,
	}
}

var (
	_ ioengine.ResourceAdapter = (*SegmentsIO)(nil)
	_ ioengine.ResourceAdapter = (*GroupsIO)(nil)
	_ ioengine.ResourceAdapter = (*ProductsIO)(nil)
	_ bulkengine.BulkAdapter   = (*activeBulk)(nil)
	_ searchengine.Adapter     = (*ProductsSearch)(nil)
)
