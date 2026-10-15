// Package adapters connects the campaigns resource to the shared engines:
// ioengine (export) and bulkengine (activate / deactivate). The
// organization always comes from the context (orgctx.OrganizationID).
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

var (
	errNoOrganization = errors.New("campaigns: organization scope is required")
	errNoImport       = errors.New("campaigns: import is not supported")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// IO exports campaigns.
type IO struct {
	svc *usecase.Service
}

// NewIO builds the io adapter. svc may be nil for metadata only.
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

func (a *IO) Resource() string { return model.Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "campaigns." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("name"), col("code"), col("campaign_type"), col("discount_kind"), col("discount_value"), col("max_discount"),
		col("min_subtotal"), col("start_date"), col("end_date"), col("channels"), col("product_scope"),
		col("usage_limit"), col("per_customer_limit"), col("used_count"), col("state"),
		{Key: "is_active", LabelKey: "campaigns.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "uuid", LabelKey: "campaigns.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "campaigns.created_at", Type: ioengine.ColumnTypeDatetime},
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
	str := func(v fmt.Stringer, ok bool) string {
		if !ok {
			return ""
		}
		return v.String()
	}
	num := func(v *int) string {
		if v == nil {
			return ""
		}
		return strconv.Itoa(*v)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		code := ""
		if c.Code != nil {
			code = *c.Code
		}
		out = append(out, map[string]any{
			"uuid": c.UUID.String(), "name": c.Name, "code": code, "campaign_type": c.CampaignType,
			"discount_kind": c.DiscountKind, "discount_value": c.DiscountValue.String(),
			"max_discount": str(c.MaxDiscount, c.MaxDiscount != nil), "min_subtotal": str(c.MinSubtotal, c.MinSubtotal != nil),
			"start_date": str(c.StartDate, c.StartDate != nil), "end_date": str(c.EndDate, c.EndDate != nil),
			"channels": strings.Join(c.Channels, ","), "product_scope": c.ProductScope,
			"usage_limit": num(c.UsageLimit), "per_customer_limit": num(c.PerCustomerLimit),
			"used_count": strconv.Itoa(c.UsedCount), "state": c.State, "is_active": c.IsActive, "created_at": c.CreatedAt,
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

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

// Bulk activates / deactivates campaigns.
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk { return &Bulk{svc: svc} }

func (a *Bulk) Resource() string { return model.Resource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{ID: "activate", LabelKey: "bulk.actions.campaigns.activate", Permission: rbac.PermTenantCampaignsWrite, Reversible: true},
		{
			ID: "deactivate", LabelKey: "bulk.actions.campaigns.deactivate", Permission: rbac.PermTenantCampaignsWrite,
			Reversible: true, ConfirmKey: "bulk.confirm.campaigns.deactivate",
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
	var active bool
	switch action {
	case "activate":
		active = true
	case "deactivate":
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	prev, err := a.svc.SetActive(ctx, orgID, id, active)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "campaign", OK: true, Op: "update",
		Previous: map[string]any{"is_active": prev},
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
	prev, ok := previous["is_active"].(bool)
	if !ok {
		return fmt.Errorf("missing previous is_active")
	}
	_, err = a.svc.SetActive(ctx, orgID, id, prev)
	return err
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
)
