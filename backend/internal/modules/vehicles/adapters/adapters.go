// Package adapters connects the vehicles resource to the shared engines:
// ioengine (export / İdes fleet import with a dry-run diff), bulkengine
// (status, branch move, rental group, activate / deactivate) and
// searchengine (Cmd+K by plate and VIN). The organization and the branch
// scope always come from the context (orgctx): the request scope on tenant
// routes, the job scope in the worker. Without an organization every adapter
// call fails.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

// Resource is the engine slug of vehicles.
const Resource = model.Resource

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = usecase.MaxListTargets

var errNoOrganization = errors.New("vehicles: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// ---------------------------------------------------------------- io engine

// IO exports vehicles and imports an İdes fleet XLSX (or the export format).
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
		return ioengine.Column{Key: key, LabelKey: "vehicles." + key, Type: t}
	}
	s, b := ioengine.ColumnTypeString, ioengine.ColumnTypeBoolean
	return []ioengine.Column{
		col("plate", s), col("vin", s), col("brand", s), col("model", s), col("type_name", s),
		col("type_code", s), col("model_year", s), col("color", s), col("transmission", s), col("fuel", s),
		col("body_type", s), col("seats", s), col("luggage", s), col("home_branch", s), col("current_branch", s),
		col("fleet_pool", s), col("ownership", s), col("supplier_name", s), col("odometer_km", s),
		col("fuel_level", s), col("status", s), col("kasko_value", s), col("kasko_period", s),
		col("rental_groups", s), col("in_service_date", s), col("out_of_service_date", s),
		col("inspection_until", s), col("insurance_until", s), col("kasko_until", s), col("document_status", s),
		col("next_maintenance_date", s), col("next_maintenance_km", s), col("is_active", b),
		col("uuid", ioengine.ColumnTypeUUID), col("created_at", ioengine.ColumnTypeDatetime),
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
	for _, v := range rows {
		groups := make([]string, 0, len(v.RentalGroups))
		for _, g := range v.RentalGroups {
			groups = append(groups, g.Code)
		}
		typeCode := ""
		if v.Catalog != nil {
			typeCode = v.Catalog.TypeCode
		}
		out = append(out, map[string]any{
			"uuid": v.UUID.String(), "plate": v.Plate, "vin": v.VIN, "brand": v.Brand, "model": v.Model,
			"type_name": v.TypeName, "type_code": typeCode, "model_year": strconv.Itoa(int(v.ModelYear)),
			"color": v.Color, "transmission": str(v.Transmission), "fuel": str(v.Fuel), "body_type": str(v.BodyType),
			"seats": int16Str(v.Seats), "luggage": int16Str(v.Luggage),
			"home_branch": v.HomeBranch.Code, "current_branch": v.CurrentBranch.Code,
			"fleet_pool": v.FleetPool, "ownership": v.Ownership, "supplier_name": v.SupplierName,
			"odometer_km": strconv.Itoa(int(v.OdometerKm)), "fuel_level": int16Str(v.FuelLevel),
			"status": v.Status, "kasko_value": str(v.KaskoValue), "kasko_period": str(v.KaskoPeriod),
			"rental_groups": strings.Join(groups, ", "), "in_service_date": str(v.InServiceDate),
			"out_of_service_date": str(v.OutOfServiceDate), "is_active": v.IsActive, "created_at": v.CreatedAt,
			"inspection_until": str(v.InspectionUntil), "insurance_until": str(v.InsuranceUntil),
			"kasko_until": str(v.KaskoUntil), "document_status": v.DocumentStatus,
			"next_maintenance_date": str(v.NextMaintenanceDate), "next_maintenance_km": int32Str(v.NextMaintenanceKm),
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema follows the İdes fleet XLSX (plaka, şasi, marka, model, tip /
// kasko kodu, model yılı, renk, vites, yakıt, şube, mülkiyet, km). type_code
// comes before type_name so a "Tip/Kasko Kodu" header maps to the code.
func (a *IO) ImportSchema() []ioengine.ImportField {
	f := func(key string, required bool) ioengine.ImportField {
		return ioengine.ImportField{Key: key, LabelKey: "vehicles." + key, Type: ioengine.ColumnTypeString, Required: required}
	}
	pool := f("fleet_pool", false)
	pool.DefaultHint = model.DefaultPool
	ownership := f("ownership", false)
	ownership.DefaultHint = model.OwnershipOwned
	return []ioengine.ImportField{
		f("plate", true), f("vin", false), f("brand", false), f("model", false), f("type_code", false),
		f("type_name", false), f("model_year", true), f("color", false), f("transmission", false),
		f("fuel", false), f("branch", true), ownership, f("supplier_name", false), pool, f("odometer_km", false),
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{{
		"plate": "34 ABC 123", "vin": "NM4KXXTTDKSB12345", "brand": "Fiat", "model": "Egea",
		"type_code": "", "type_name": "1.4 Fire Easy", "model_year": "2024", "color": "Beyaz",
		"transmission": "Manuel", "fuel": "Benzin", "branch": "MRK", "ownership": "Özmal",
		"supplier_name": "", "fleet_pool": model.DefaultPool, "odometer_km": "12500",
	}}
}

// ApplyRow creates a vehicle, or updates the vehicle with the same plate.
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
	res := ioengine.RowResult{OK: true, EntityType: "vehicle", EntityUUID: id.String(), Op: "create"}
	if !created && prev != nil {
		res.Op = "update"
		res.Previous = snapshot(*prev)
	}
	return res, nil
}

// PreviewRow implements ioengine.RowPreviewer: validation and the fields the
// row would change, without writing.
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

// RevertRow deletes an imported vehicle or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "vehicle" {
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
	raw, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	var prev model.Vehicle
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	_, err = a.svc.Patch(ctx, orgID, id, InputFromVehicle(prev))
	return err
}

func rowError(err error) bool {
	var ve *usecase.ValidationError
	return errors.As(err, &ve) || errors.Is(err, usecase.ErrPlateTaken) || errors.Is(err, usecase.ErrNotFound)
}

func inputFromRow(row, defaults map[string]any) usecase.ImportInput {
	get := func(key string) string {
		if v := cell(row, key); v != "" {
			return v
		}
		return cell(defaults, key)
	}
	return usecase.ImportInput{
		Plate: get("plate"), VIN: get("vin"), Brand: get("brand"), Model: get("model"),
		TypeName: get("type_name"), TypeCode: get("type_code"), ModelYear: get("model_year"),
		Color: get("color"), Transmission: get("transmission"), Fuel: get("fuel"), Branch: get("branch"),
		Ownership: get("ownership"), SupplierName: get("supplier_name"), Pool: get("fleet_pool"),
		OdometerKm: get("odometer_km"),
	}
}

// InputFromVehicle is the patch that restores a vehicle snapshot (import
// rollback). Status is restored as a recorded transition.
func InputFromVehicle(v model.Vehicle) model.VehicleInput {
	in := model.VehicleInput{
		Plate: &v.Plate, VIN: &v.VIN, ManualEntry: &v.ManualEntry, ModelYear: &v.ModelYear,
		Transmission: optString(v.Transmission), Fuel: optString(v.Fuel), BodyType: optString(v.BodyType),
		Seats: optInt16(v.Seats), Luggage: optInt16(v.Luggage), AirConditioning: &v.AirConditioning,
		AttributesVerified: &v.AttributesVerified, Color: &v.Color,
		HomeBranchUUID: &v.HomeBranch.UUID, CurrentBranchUUID: &v.CurrentBranch.UUID,
		FleetPool: &v.FleetPool, Ownership: &v.Ownership, SupplierName: &v.SupplierName,
		InServiceDate: optString(v.InServiceDate), OutOfServiceDate: optString(v.OutOfServiceDate),
		OdometerKm: &v.OdometerKm, FuelLevel: optInt16(v.FuelLevel), Status: &v.Status, IsActive: &v.IsActive,
	}
	if v.ManualEntry {
		in.ManualBrand, in.ManualModel, in.ManualType = &v.Brand, &v.Model, &v.TypeName
	} else if v.Catalog != nil {
		id := v.Catalog.TypeUUID
		in.CatalogTypeUUID = model.OptionalUUID{Set: true, Value: &id}
	}
	return in
}

func optString(v *string) model.OptionalString {
	return model.OptionalString{Set: true, Value: v}
}

func optInt16(v *int16) model.OptionalInt {
	if v == nil {
		return model.OptionalInt{Set: true}
	}
	n := int64(*v)
	return model.OptionalInt{Set: true, Value: &n}
}

func snapshot(v model.Vehicle) map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
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

func int16Str(v *int16) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(int(*v))
}

func int32Str(v *int32) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(int(*v))
}

// -------------------------------------------------------------- bulk engine

// Bulk action ids. Params (BulkTarget.params): set_status {status, note?},
// move_branch {branch_uuid}, add_rental_group / replace_rental_group
// {rental_group_uuid}, start_maintenance {reason?, sync_status?
// ("true" / "false")}.
const (
	ActionSetStatus          = "set_status"
	ActionMoveBranch         = "move_branch"
	ActionAddRentalGroup     = "add_rental_group"
	ActionReplaceRentalGroup = "replace_rental_group"
	ActionActivate           = "activate"
	ActionDeactivate         = "deactivate"
	// ActionStartMaintenance is "Bakıma al" (TEC-694): an open-ended
	// maintenance block starting now, optionally with the status change.
	ActionStartMaintenance = "start_maintenance"
)

// Maintenance starts and undoes maintenance blocks for the
// start_maintenance action. The blocks live in the fleetops module, which
// depends on this one; it is injected with SetMaintenance.
type Maintenance interface {
	// StartMaintenance opens a maintenance block starting now and returns
	// it with the vehicle status before the change ("" = not changed).
	StartMaintenance(ctx context.Context, orgID int64, vehicleUUID uuid.UUID, reason string, syncStatus bool) (blockUUID uuid.UUID, previousStatus string, err error)
	// UndoMaintenance removes the block and restores previousStatus when
	// it is not empty (bulk rollback).
	UndoMaintenance(ctx context.Context, orgID int64, vehicleUUID, blockUUID uuid.UUID, previousStatus string) error
	// ItemError reports whether err refuses one vehicle (the item fails,
	// the job goes on) rather than a system failure.
	ItemError(err error) bool
}

// Bulk changes vehicles in bulk; every action is reversible.
type Bulk struct {
	svc         *usecase.Service
	maintenance Maintenance
}

// SetMaintenance enables the start_maintenance action.
func (a *Bulk) SetMaintenance(m Maintenance) {
	a.maintenance = m
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk {
	return &Bulk{svc: svc}
}

func (a *Bulk) Resource() string { return Resource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	def := func(id string, confirm bool) bulkengine.BulkActionDef {
		d := bulkengine.BulkActionDef{
			ID: id, LabelKey: "bulk.actions.vehicles." + id, Permission: rbac.PermTenantVehiclesBulk, Reversible: true,
		}
		if confirm {
			d.ConfirmKey = "bulk.confirm.vehicles." + id
		}
		return d
	}
	return []bulkengine.BulkActionDef{
		def(ActionSetStatus, false),
		def(ActionMoveBranch, false),
		def(ActionAddRentalGroup, false),
		def(ActionReplaceRentalGroup, true),
		def(ActionActivate, false),
		def(ActionDeactivate, true),
		{
			ID: ActionStartMaintenance, LabelKey: "bulk.actions.vehicles." + ActionStartMaintenance,
			Permission: rbac.PermTenantBlocksWrite, Reversible: true,
		},
	}
}

// ResolveTargets validates the action params and resolves the vehicles.
func (a *Bulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.validateParams(ctx, orgID, action, target.Params); err != nil {
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

func (a *Bulk) validateParams(ctx context.Context, orgID int64, action string, params map[string]string) error {
	switch action {
	case ActionStartMaintenance:
		if a.maintenance == nil {
			return fmt.Errorf("start_maintenance is not available")
		}
		if len([]rune(params["reason"])) > 2000 {
			return fmt.Errorf("params.reason must be at most 2000 characters")
		}
		if v := params["sync_status"]; v != "" && v != "true" && v != "false" {
			return fmt.Errorf("params.sync_status must be true or false")
		}
	case ActionSetStatus:
		if !model.Contains(model.Statuses, params["status"]) {
			return fmt.Errorf("params.status must be one of %s", strings.Join(model.Statuses, ", "))
		}
	case ActionMoveBranch:
		id, err := uuid.Parse(params["branch_uuid"])
		if err != nil {
			return fmt.Errorf("params.branch_uuid is required")
		}
		if ok, err := a.svc.BranchExists(ctx, orgID, id); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("params.branch_uuid: branch was not found")
		}
	case ActionAddRentalGroup, ActionReplaceRentalGroup:
		id, err := uuid.Parse(params["rental_group_uuid"])
		if err != nil {
			return fmt.Errorf("params.rental_group_uuid is required")
		}
		if ok, err := a.svc.GroupExists(ctx, orgID, id); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("params.rental_group_uuid: rental group was not found")
		}
	}
	return nil
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
	params := bulkengine.Params(ctx)
	fail := func(err error) (bulkengine.BulkItemResult, error) {
		var ve *usecase.ValidationError
		if _, ok := usecase.IsNotEligible(err); ok || errors.As(err, &ve) ||
			errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrPlateTaken) ||
			errors.Is(err, usecase.ErrGroupNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	ok := func(previous map[string]any) (bulkengine.BulkItemResult, error) {
		return bulkengine.BulkItemResult{
			EntityUUID: entityUUID, EntityType: "vehicle", OK: true, Op: "update", Previous: previous,
		}, nil
	}
	switch action {
	case ActionStartMaintenance:
		if a.maintenance == nil {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "maintenance is not available"}, nil
		}
		block, prev, err := a.maintenance.StartMaintenance(ctx, orgID, id, strings.TrimSpace(params["reason"]), params["sync_status"] == "true")
		if err != nil {
			if a.maintenance.ItemError(err) {
				return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
			}
			return fail(err)
		}
		return ok(map[string]any{"block_uuid": block.String(), "status": prev})
	case ActionSetStatus:
		prev, err := a.svc.SetStatus(ctx, orgID, id, params["status"], params["note"])
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"status": prev})
	case ActionMoveBranch:
		branch, err := uuid.Parse(params["branch_uuid"])
		if err != nil {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid branch"}, nil
		}
		prev, err := a.svc.MoveCurrentBranch(ctx, orgID, id, branch)
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"current_branch_uuid": prev.String()})
	case ActionAddRentalGroup, ActionReplaceRentalGroup:
		group, err := uuid.Parse(params["rental_group_uuid"])
		if err != nil {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid rental group"}, nil
		}
		prev, err := a.svc.SetGroups(ctx, orgID, id, []uuid.UUID{group}, action == ActionReplaceRentalGroup, true)
		if err != nil {
			return fail(err)
		}
		ids := make([]string, 0, len(prev))
		for _, p := range prev {
			ids = append(ids, p.String())
		}
		return ok(map[string]any{"rental_group_uuids": ids})
	case ActionActivate, ActionDeactivate:
		prev, err := a.svc.SetActive(ctx, orgID, id, action == ActionActivate)
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"is_active": prev})
	}
	return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
}

func (a *Bulk) RevertItem(ctx context.Context, action string, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	switch action {
	case ActionStartMaintenance:
		if a.maintenance == nil {
			return fmt.Errorf("maintenance is not available")
		}
		block, err := uuid.Parse(fmt.Sprint(previous["block_uuid"]))
		if err != nil {
			return fmt.Errorf("missing maintenance block")
		}
		prev, _ := previous["status"].(string)
		return a.maintenance.UndoMaintenance(ctx, orgID, id, block, prev)
	case ActionSetStatus:
		prev, ok := previous["status"].(string)
		if !ok {
			return fmt.Errorf("missing previous status")
		}
		_, err = a.svc.SetStatus(ctx, orgID, id, prev, "rollback")
		return err
	case ActionMoveBranch:
		prev, err := uuid.Parse(fmt.Sprint(previous["current_branch_uuid"]))
		if err != nil {
			return fmt.Errorf("missing previous branch")
		}
		_, err = a.svc.MoveCurrentBranch(ctx, orgID, id, prev)
		return err
	case ActionAddRentalGroup, ActionReplaceRentalGroup:
		raw, _ := previous["rental_group_uuids"].([]any)
		ids := make([]uuid.UUID, 0, len(raw))
		for _, v := range raw {
			if gid, err := uuid.Parse(fmt.Sprint(v)); err == nil {
				ids = append(ids, gid)
			}
		}
		_, err = a.svc.SetGroups(ctx, orgID, id, ids, true, false)
		return err
	case ActionActivate, ActionDeactivate:
		prev, ok := previous["is_active"].(bool)
		if !ok {
			return fmt.Errorf("missing previous is_active")
		}
		_, err = a.svc.SetActive(ctx, orgID, id, prev)
		return err
	}
	return fmt.Errorf("unknown action %q", action)
}

// ------------------------------------------------------------ search engine

// Search indexes vehicles (plate, VIN) for the tenant Cmd+K palette.
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
		LabelKey:   "search.specs_vehicles",
		Permission: rbac.PermTenantVehiclesRead,
		Icon:       "vehicles",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// Documents carry the home and current branch; restricted members
		// find the vehicles of their branches only.
		BranchScoped: true,
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
		rows, err := a.q.ListVehicleSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetVehicleSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one vehicle by its tenant document id ("<org>_<uuid>").
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("vehicles search: invalid uuid")
	}
	row, err := a.q.GetVehicleSearchDocument(ctx, db.GetVehicleSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetVehicleSearchDocumentRow) searchengine.Document {
	key := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(row.Plate))
	subtitle := strings.TrimSpace(row.VehicleLabel + " · " + strconv.Itoa(int(row.ModelYear)))
	keywords := []string{row.Plate, key, row.VehicleLabel}
	if row.Vin != "" {
		keywords = append(keywords, row.Vin)
	}
	branches := []int64{row.HomeBranchID}
	if row.CurrentBranchID != row.HomeBranchID {
		branches = append(branches, row.CurrentBranchID)
	}
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          row.Plate,
		Subtitle:       subtitle,
		Keywords:       keywords,
		Href:           "/t/" + row.OrganizationSlug + "/fleet/vehicles/" + row.Uuid.String(),
		Icon:           "vehicles",
		OrganizationID: row.OrganizationID,
		BranchIDs:      branches,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ ioengine.RowPreviewer    = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
