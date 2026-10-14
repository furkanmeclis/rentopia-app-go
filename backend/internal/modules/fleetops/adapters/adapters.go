// Package adapters connects the fleet operations resources to the shared
// engines: ioengine (export of transfers and one-way rules), bulkengine
// (activate / deactivate one-way rules) and the vehicles bulk action
// "Bakıma al" (Maintenance). The organization and the branch scope always
// come from the context (orgctx): the request scope on tenant routes, the
// job scope in the worker. Without an organization every call fails.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	vehicleadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = usecase.MaxListTargets

var (
	errNoOrganization = errors.New("fleetops: organization scope is required")
	errNoImport       = errors.New("fleetops: import is not supported for this resource")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
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

func str(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// ---------------------------------------------------------------- transfers

// TransfersIO exports transfers.
type TransfersIO struct {
	exportOnly
	svc *usecase.Service
}

// NewTransfersIO builds the transfers io adapter.
func NewTransfersIO(svc *usecase.Service) *TransfersIO { return &TransfersIO{svc: svc} }

func (a *TransfersIO) Resource() string { return model.ResourceTransfers }

func (a *TransfersIO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "transfers." + key, Type: t}
	}
	s, dt := ioengine.ColumnTypeString, ioengine.ColumnTypeDatetime
	return []ioengine.Column{
		col("plate", s), col("vehicle", s), col("from_branch", s), col("to_branch", s), col("responsible", s),
		col("planned_departure", dt), col("planned_arrival", dt), col("actual_departure", dt),
		col("actual_arrival", dt), col("status", s), col("cost", s), col("note", s),
		col("uuid", ioengine.ColumnTypeUUID), col("created_at", dt),
	}
}

func (a *TransfersIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.TransferExportRows(ctx, orgID, model.TransferFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, t := range rows {
		responsible := ""
		if t.Responsible != nil {
			responsible = strings.TrimSpace(t.Responsible.Name + " " + t.Responsible.Surname)
		}
		row := map[string]any{
			"uuid": t.UUID.String(), "plate": t.Vehicle.Plate, "vehicle": t.Vehicle.Label,
			"from_branch": t.FromBranch.Code, "to_branch": t.ToBranch.Code, "responsible": responsible,
			"planned_departure": t.PlannedDeparture, "planned_arrival": t.PlannedArrival,
			"actual_departure": "", "actual_arrival": "", "status": t.Status, "cost": str(t.Cost),
			"note": t.Note, "created_at": t.CreatedAt,
		}
		if t.ActualDeparture != nil {
			row["actual_departure"] = *t.ActualDeparture
		}
		if t.ActualArrival != nil {
			row["actual_arrival"] = *t.ActualArrival
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: model.ResourceTransfers, Columns: a.ExportColumns(), Rows: out}, nil
}

// ------------------------------------------------------------ one-way rules

// OneWayIO exports one-way rules.
type OneWayIO struct {
	exportOnly
	svc *usecase.Service
}

// NewOneWayIO builds the one-way rules io adapter.
func NewOneWayIO(svc *usecase.Service) *OneWayIO { return &OneWayIO{svc: svc} }

func (a *OneWayIO) Resource() string { return model.ResourceOneWayRules }

func (a *OneWayIO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "one_way_rules." + key, Type: t}
	}
	s, b := ioengine.ColumnTypeString, ioengine.ColumnTypeBoolean
	return []ioengine.Column{
		col("from_branch", s), col("to_branch", s), col("rental_product", s), col("allowed", b),
		col("fee", s), col("note", s), col("is_active", b),
		col("uuid", ioengine.ColumnTypeUUID), col("created_at", ioengine.ColumnTypeDatetime),
	}
}

func (a *OneWayIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.RuleExportRows(ctx, orgID, model.OneWayFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		product := ""
		if r.Product != nil {
			product = r.Product.Code
		}
		out = append(out, map[string]any{
			"uuid": r.UUID.String(), "from_branch": r.FromBranch.Code, "to_branch": r.ToBranch.Code,
			"rental_product": product, "allowed": r.Allowed, "fee": r.Fee, "note": r.Note,
			"is_active": r.IsActive, "created_at": r.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceOneWayRules, Columns: a.ExportColumns(), Rows: out}, nil
}

// OneWayBulk activates / deactivates one-way rules (reversible).
type OneWayBulk struct {
	svc *usecase.Service
}

// NewOneWayBulk builds the bulk adapter. svc may be nil for metadata only.
func NewOneWayBulk(svc *usecase.Service) *OneWayBulk { return &OneWayBulk{svc: svc} }

func (a *OneWayBulk) Resource() string { return model.ResourceOneWayRules }

func (a *OneWayBulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions.one_way_rules.activate",
			Permission: rbac.PermTenantOneWayWrite, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.one_way_rules.deactivate",
			Permission: rbac.PermTenantOneWayWrite, Reversible: true,
			ConfirmKey: "bulk.confirm.one_way_rules.deactivate",
		},
	}
}

func (a *OneWayBulk) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
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
		if a.svc == nil {
			return nil, fmt.Errorf("one-way bulk: no service")
		}
		ids, err := a.svc.RuleTargetUUIDs(ctx, orgID, model.OneWayFilterFromQuery(target.Query))
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

func (a *OneWayBulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	if action != "activate" && action != "deactivate" {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	if a.svc == nil {
		return bulkengine.BulkItemResult{}, fmt.Errorf("one-way bulk: no service")
	}
	prev, err := a.svc.SetRuleActive(ctx, orgID, id, action == "activate")
	if err != nil {
		if errors.Is(err, usecase.ErrRuleNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "one_way_rule", OK: true, Op: "update",
		Previous: map[string]any{"is_active": prev},
	}, nil
}

func (a *OneWayBulk) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
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
	if a.svc == nil {
		return fmt.Errorf("one-way bulk: no service")
	}
	_, err = a.svc.SetRuleActive(ctx, orgID, id, prev)
	return err
}

// ------------------------------------------------- vehicles "Bakıma al"

// Maintenance plugs the blocks use case into the vehicles bulk adapter
// (start_maintenance): vehicleadapters.Bulk.SetMaintenance(NewMaintenance(svc)).
type Maintenance struct {
	svc *usecase.Service
}

// NewMaintenance builds the maintenance hook of the vehicles bulk adapter.
func NewMaintenance(svc *usecase.Service) *Maintenance { return &Maintenance{svc: svc} }

func (m *Maintenance) StartMaintenance(ctx context.Context, orgID int64, vehicleUUID uuid.UUID, reason string, syncStatus bool) (uuid.UUID, string, error) {
	return m.svc.StartMaintenance(ctx, orgID, vehicleUUID, reason, syncStatus)
}

func (m *Maintenance) UndoMaintenance(ctx context.Context, orgID int64, vehicleUUID, blockUUID uuid.UUID, previousStatus string) error {
	return m.svc.UndoMaintenance(ctx, orgID, vehicleUUID, blockUUID, previousStatus)
}

func (m *Maintenance) ItemError(err error) bool {
	return m.svc.ItemError(err)
}

var (
	_ ioengine.ResourceAdapter    = (*TransfersIO)(nil)
	_ ioengine.ResourceAdapter    = (*OneWayIO)(nil)
	_ bulkengine.BulkAdapter      = (*OneWayBulk)(nil)
	_ vehicleadapters.Maintenance = (*Maintenance)(nil)
)
