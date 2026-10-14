package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// DocumentExportRow is one exported document.
type DocumentExportRow struct {
	Plate, Vehicle  string
	Document        model.Document
	VehicleUUIDText string
}

// MaintenanceExportRow is one exported maintenance item.
type MaintenanceExportRow struct {
	Plate, Vehicle  string
	Item            model.Maintenance
	VehicleUUIDText string
}

// ExportDocuments returns the documents matching f in the caller's branch
// scope (export engine; capped at MaxExportRows).
func (s *Service) ExportDocuments(ctx context.Context, orgID int64, f model.ExportFilter) ([]DocumentExportRow, error) {
	rows, err := s.repo.ExportDocuments(ctx, orgID, orgctx.BranchScope(ctx), f, MaxExportRows)
	if err != nil {
		return nil, err
	}
	today := s.today()
	out := make([]DocumentExportRow, 0, len(rows))
	for _, row := range rows {
		d := model.Document{
			UUID: row.Uuid, DocType: row.DocType, DocumentNumber: row.DocumentNumber, Issuer: row.Issuer,
			ValidFrom: datePtr(row.ValidFrom), ValidUntil: datePtr(row.ValidUntil),
			Status:    vehiclemodel.DocumentState(timePtr(row.ValidUntil), today),
			IsCurrent: row.IsCurrent, Note: row.Note, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		}
		if row.FileObjectKey.Valid {
			d.File = &model.FileInfo{Name: row.FileName, ContentType: row.FileContentType, SizeBytes: row.FileSizeBytes}
		}
		out = append(out, DocumentExportRow{
			Plate: row.Plate, Vehicle: row.VehicleLabel, Document: d, VehicleUUIDText: row.VehicleUuid.String(),
		})
	}
	return out, nil
}

// ExportMaintenance returns the maintenance items matching f in the caller's
// branch scope (export engine; capped at MaxExportRows).
func (s *Service) ExportMaintenance(ctx context.Context, orgID int64, f model.ExportFilter) ([]MaintenanceExportRow, error) {
	rows, err := s.repo.ExportMaintenance(ctx, orgID, orgctx.BranchScope(ctx), f, MaxExportRows)
	if err != nil {
		return nil, err
	}
	out := make([]MaintenanceExportRow, 0, len(rows))
	for _, row := range rows {
		m := model.Maintenance{
			UUID: row.Uuid, Kind: row.Kind, MaintenanceType: row.MaintenanceType,
			PlannedDate: datePtr(row.PlannedDate), PlannedKm: int4Ptr(row.PlannedKm),
			DoneDate: datePtr(row.DoneDate), DoneKm: int4Ptr(row.DoneKm), ServiceProvider: row.ServiceProvider,
			Currency: row.Currency, Note: row.Note, Status: row.Kind,
			CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		}
		if row.Cost.Valid {
			c := vehicleusecase.FormatMoney(row.Cost)
			m.Cost = &c
		}
		out = append(out, MaintenanceExportRow{
			Plate: row.Plate, Vehicle: row.VehicleLabel, Item: m, VehicleUUIDText: row.VehicleUuid.String(),
		})
	}
	return out, nil
}

// Today exposes the operating day (adapters, tests).
func (s *Service) Today() time.Time {
	return s.today()
}
