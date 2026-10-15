// Package adapters connects the extras resource to the shared engines:
// ioengine (catalog export with the current default price) and bulkengine
// (publish / archive). The organization always comes from the context
// (orgctx.OrganizationID): the request scope on tenant routes, the job row
// in the worker. Without one every adapter call fails.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

var (
	errNoOrganization = errors.New("extras: organization scope is required")
	errNoImport       = errors.New("extras: import is not supported")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// ---------------------------------------------------------------- io engine

// IO exports the extras catalog.
type IO struct {
	svc *usecase.Service
}

// NewIO builds the io adapter. svc may be nil for metadata only.
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

func (a *IO) Resource() string { return model.Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "extras." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("code"), col("name"), col("extra_type"), col("charge_type"), col("default_price"), col("product_price_count"),
		col("min_quantity"), col("max_quantity"), col("max_total"), col("channels"), col("status"),
		{Key: "is_physical", LabelKey: "extras.is_physical", Type: ioengine.ColumnTypeBoolean},
		col("stock_total"),
		{Key: "uuid", LabelKey: "extras.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "extras.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportRows(ctx, orgID, model.FilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, e := range rows {
		price, maxTotal := "", ""
		if e.DefaultPrice != nil {
			price = e.DefaultPrice.String()
		}
		if e.MaxTotal != nil {
			maxTotal = e.MaxTotal.String()
		}
		out = append(out, map[string]any{
			"uuid": e.UUID.String(), "code": e.Code, "name": e.Name, "extra_type": e.ExtraType, "charge_type": e.ChargeType,
			"default_price": price, "product_price_count": strconv.FormatInt(e.ProductPriceCount, 10),
			"min_quantity": strconv.Itoa(e.MinQuantity), "max_quantity": strconv.Itoa(e.MaxQuantity), "max_total": maxTotal,
			"channels": strings.Join(e.Channels, ","), "status": e.Status, "is_physical": e.IsPhysical,
			"stock_total": strconv.FormatInt(e.StockTotal, 10), "created_at": e.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: model.Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema / ApplyRow / RevertRow: no import route is mounted.
func (a *IO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *IO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *IO) RevertRow(context.Context, string, string, map[string]any) error { return errNoImport }

// -------------------------------------------------------------- bulk engine

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

// Bulk publishes / archives extras.
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk { return &Bulk{svc: svc} }

func (a *Bulk) Resource() string { return model.Resource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{ID: "publish", LabelKey: "bulk.actions.extras.publish", Permission: rbac.PermTenantExtrasWrite, Reversible: true},
		{
			ID: "archive", LabelKey: "bulk.actions.extras.archive", Permission: rbac.PermTenantExtrasWrite, Reversible: true,
			ConfirmKey: "bulk.confirm.extras.archive",
		},
	}
}

func (a *Bulk) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
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
	status := ""
	switch action {
	case "publish":
		status = model.StatusPublished
	case "archive":
		status = model.StatusArchived
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	prev, err := a.svc.SetStatus(ctx, orgID, id, status)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "extra", OK: true, Op: "update",
		Previous: map[string]any{"status": prev},
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
	prev, ok := previous["status"].(string)
	if !ok {
		return fmt.Errorf("missing previous status")
	}
	_, err = a.svc.SetStatus(ctx, orgID, id, prev)
	return err
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
)
