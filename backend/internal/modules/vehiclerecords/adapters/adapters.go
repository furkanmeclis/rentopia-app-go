// Package adapters connects vehicle documents and maintenance to the export
// engine (export only; documents and maintenance are not imported). The
// organization and the branch scope come from the context (request scope or
// the export job's scope).
package adapters

import (
	"context"
	"errors"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

var (
	errNoOrganization = errors.New("vehicle records: organization scope is required")
	errNoImport       = errors.New("vehicle records: import is not supported")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

type exportOnly struct{}

func (exportOnly) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (exportOnly) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (exportOnly) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// ---------------------------------------------------------------- documents

// DocumentsIO exports vehicle documents.
type DocumentsIO struct {
	exportOnly
	svc *usecase.Service
}

// NewDocumentsIO builds the documents export adapter.
func NewDocumentsIO(svc *usecase.Service) *DocumentsIO { return &DocumentsIO{svc: svc} }

func (a *DocumentsIO) Resource() string { return model.ResourceDocuments }

func (a *DocumentsIO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "vehicle_documents." + key, Type: t}
	}
	s := ioengine.ColumnTypeString
	return []ioengine.Column{
		col("plate", s), col("vehicle", s), col("doc_type", s), col("document_number", s), col("issuer", s),
		col("valid_from", s), col("valid_until", s), col("status", s), col("is_current", ioengine.ColumnTypeBoolean),
		col("file_name", s), col("note", s), col("uuid", ioengine.ColumnTypeUUID),
		col("created_at", ioengine.ColumnTypeDatetime),
	}
}

func (a *DocumentsIO) Export(ctx context.Context, query ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportDocuments(ctx, orgID, model.ExportFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		d := row.Document
		fileName := ""
		if d.File != nil {
			fileName = d.File.Name
		}
		out = append(out, map[string]any{
			"plate": row.Plate, "vehicle": row.Vehicle,
			"doc_type":        i18n.Translate(loc, "vehicle_documents.type."+d.DocType),
			"document_number": d.DocumentNumber, "issuer": d.Issuer,
			"valid_from": str(d.ValidFrom), "valid_until": str(d.ValidUntil),
			"status":     i18n.Translate(loc, "vehicle_documents.status."+d.Status),
			"is_current": d.IsCurrent, "file_name": fileName, "note": d.Note,
			"uuid": d.UUID.String(), "created_at": d.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceDocuments, Columns: a.ExportColumns(), Rows: out}, nil
}

// -------------------------------------------------------------- maintenance

// MaintenanceIO exports vehicle maintenance.
type MaintenanceIO struct {
	exportOnly
	svc *usecase.Service
}

// NewMaintenanceIO builds the maintenance export adapter.
func NewMaintenanceIO(svc *usecase.Service) *MaintenanceIO { return &MaintenanceIO{svc: svc} }

func (a *MaintenanceIO) Resource() string { return model.ResourceMaintenance }

func (a *MaintenanceIO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "vehicle_maintenance." + key, Type: t}
	}
	s := ioengine.ColumnTypeString
	return []ioengine.Column{
		col("plate", s), col("vehicle", s), col("kind", s), col("maintenance_type", s),
		col("planned_date", s), col("planned_km", s), col("done_date", s), col("done_km", s),
		col("service_provider", s), col("cost", s), col("currency", s), col("note", s),
		col("uuid", ioengine.ColumnTypeUUID), col("created_at", ioengine.ColumnTypeDatetime),
	}
}

func (a *MaintenanceIO) Export(ctx context.Context, query ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportMaintenance(ctx, orgID, model.ExportFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m := row.Item
		out = append(out, map[string]any{
			"plate": row.Plate, "vehicle": row.Vehicle,
			"kind":             i18n.Translate(loc, "vehicle_maintenance.kind."+m.Kind),
			"maintenance_type": i18n.Translate(loc, "vehicle_maintenance.type."+m.MaintenanceType),
			"planned_date":     str(m.PlannedDate), "planned_km": int32Str(m.PlannedKm),
			"done_date": str(m.DoneDate), "done_km": int32Str(m.DoneKm),
			"service_provider": m.ServiceProvider, "cost": str(m.Cost), "currency": m.Currency, "note": m.Note,
			"uuid": m.UUID.String(), "created_at": m.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceMaintenance, Columns: a.ExportColumns(), Rows: out}, nil
}

func str(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func int32Str(v *int32) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(int(*v))
}
