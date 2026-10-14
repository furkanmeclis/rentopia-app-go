// Package repository is the Postgres access path of vehicle documents,
// maintenance and reminders. Every method takes the organization id
// explicitly; one-vehicle methods also take the vehicle's internal id, which
// the use case resolved in the caller's branch scope.
package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrNotFound is returned for rows missing in the organization / vehicle.
var ErrNotFound = errors.New("not found")

// DocumentFields are the writable metadata columns of a document.
type DocumentFields struct {
	DocType        string
	DocumentNumber string
	Issuer         string
	ValidFrom      pgtype.Date
	ValidUntil     pgtype.Date
	Note           string
}

// FileFields describe a stored document file.
type FileFields struct {
	ObjectKey   string
	Name        string
	ContentType string
	SizeBytes   int64
}

// MaintenanceFields are the writable columns of a maintenance item.
type MaintenanceFields struct {
	Kind            string
	MaintenanceType string
	PlannedDate     pgtype.Date
	PlannedKm       pgtype.Int4
	DoneDate        pgtype.Date
	DoneKm          pgtype.Int4
	ServiceProvider string
	Cost            pgtype.Numeric
	Note            string
	CompletedBy     pgtype.Int8
}

// DocumentRow is a document with its creator.
type DocumentRow = db.ListVehicleDocumentsRow

// MaintenanceRow is a maintenance item with its creator.
type MaintenanceRow = db.ListVehicleMaintenanceRow

// Postgres implements the repository with sqlc queries.
type Postgres struct {
	q *db.Queries
}

// NewPostgres builds the repository.
func NewPostgres(q *db.Queries) *Postgres {
	return &Postgres{q: q}
}

// OrganizationUUID returns the public id of an organization (storage keys).
func (r *Postgres) OrganizationUUID(ctx context.Context, orgID int64) (uuid.UUID, error) {
	org, err := r.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return uuid.Nil, mapErr(err)
	}
	return org.Uuid, nil
}

// --- documents ------------------------------------------------------------------

func (r *Postgres) CreateDocument(ctx context.Context, orgID, vehicleID int64, f DocumentFields, createdBy pgtype.Int8) (db.VehicleDocument, error) {
	return r.q.CreateVehicleDocument(ctx, db.CreateVehicleDocumentParams{
		OrganizationID: orgID, VehicleID: vehicleID, DocType: f.DocType, DocumentNumber: f.DocumentNumber,
		Issuer: f.Issuer, ValidFrom: f.ValidFrom, ValidUntil: f.ValidUntil, Note: f.Note, CreatedBy: createdBy,
	})
}

func (r *Postgres) UpdateDocument(ctx context.Context, orgID, vehicleID int64, id uuid.UUID, f DocumentFields) (db.VehicleDocument, error) {
	row, err := r.q.UpdateVehicleDocument(ctx, db.UpdateVehicleDocumentParams{
		OrganizationID: orgID, VehicleID: vehicleID, Uuid: id, DocType: f.DocType, DocumentNumber: f.DocumentNumber,
		Issuer: f.Issuer, ValidFrom: f.ValidFrom, ValidUntil: f.ValidUntil, Note: f.Note,
	})
	return row, mapErr(err)
}

func (r *Postgres) SetDocumentFile(ctx context.Context, orgID, vehicleID int64, id uuid.UUID, f FileFields) (db.VehicleDocument, error) {
	row, err := r.q.SetVehicleDocumentFile(ctx, db.SetVehicleDocumentFileParams{
		OrganizationID: orgID, VehicleID: vehicleID, Uuid: id,
		FileObjectKey: pgtype.Text{String: f.ObjectKey, Valid: true}, FileName: f.Name,
		FileContentType: f.ContentType, FileSizeBytes: f.SizeBytes,
	})
	return row, mapErr(err)
}

func (r *Postgres) DeleteDocument(ctx context.Context, orgID, vehicleID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteVehicleDocument(ctx, db.SoftDeleteVehicleDocumentParams{
		OrganizationID: orgID, VehicleID: vehicleID, Uuid: id,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) GetDocument(ctx context.Context, orgID, vehicleID int64, id uuid.UUID) (DocumentRow, error) {
	row, err := r.q.GetVehicleDocument(ctx, db.GetVehicleDocumentParams{OrganizationID: orgID, VehicleID: vehicleID, Uuid: id})
	return DocumentRow(row), mapErr(err)
}

func (r *Postgres) ListDocuments(ctx context.Context, orgID, vehicleID int64, limit int32) ([]DocumentRow, error) {
	return r.q.ListVehicleDocuments(ctx, db.ListVehicleDocumentsParams{
		OrganizationID: orgID, VehicleID: vehicleID, LimitCount: limit,
	})
}

func (r *Postgres) ExportDocuments(ctx context.Context, orgID int64, branchIDs []int64, f model.ExportFilter, limit int32) ([]db.ListVehicleDocumentsForExportRow, error) {
	return r.q.ListVehicleDocumentsForExport(ctx, db.ListVehicleDocumentsForExportParams{
		OrganizationID: orgID, BranchIds: branchIDs, VehicleUuids: uuidsArg(f.VehicleUUIDs),
		DocTypes: stringsArg(f.DocTypes), LimitCount: limit,
	})
}

// --- maintenance ------------------------------------------------------------------

func (r *Postgres) CreateMaintenance(ctx context.Context, orgID, vehicleID int64, f MaintenanceFields, createdBy pgtype.Int8) (db.VehicleMaintenance, error) {
	return r.q.CreateVehicleMaintenance(ctx, db.CreateVehicleMaintenanceParams{
		OrganizationID: orgID, VehicleID: vehicleID, Kind: f.Kind, MaintenanceType: f.MaintenanceType,
		PlannedDate: f.PlannedDate, PlannedKm: f.PlannedKm, DoneDate: f.DoneDate, DoneKm: f.DoneKm,
		ServiceProvider: f.ServiceProvider, Cost: f.Cost, Note: f.Note, CreatedBy: createdBy,
		CompletedBy: f.CompletedBy,
	})
}

func (r *Postgres) UpdateMaintenance(ctx context.Context, orgID, vehicleID int64, id uuid.UUID, f MaintenanceFields) (db.VehicleMaintenance, error) {
	row, err := r.q.UpdateVehicleMaintenance(ctx, db.UpdateVehicleMaintenanceParams{
		OrganizationID: orgID, VehicleID: vehicleID, Uuid: id, Kind: f.Kind, MaintenanceType: f.MaintenanceType,
		PlannedDate: f.PlannedDate, PlannedKm: f.PlannedKm, DoneDate: f.DoneDate, DoneKm: f.DoneKm,
		ServiceProvider: f.ServiceProvider, Cost: f.Cost, Note: f.Note, CompletedBy: f.CompletedBy,
	})
	return row, mapErr(err)
}

func (r *Postgres) DeleteMaintenance(ctx context.Context, orgID, vehicleID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteVehicleMaintenance(ctx, db.SoftDeleteVehicleMaintenanceParams{
		OrganizationID: orgID, VehicleID: vehicleID, Uuid: id,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) GetMaintenance(ctx context.Context, orgID, vehicleID int64, id uuid.UUID) (db.VehicleMaintenance, error) {
	row, err := r.q.GetVehicleMaintenance(ctx, db.GetVehicleMaintenanceParams{OrganizationID: orgID, VehicleID: vehicleID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) ListMaintenance(ctx context.Context, orgID, vehicleID int64, limit int32) ([]MaintenanceRow, error) {
	return r.q.ListVehicleMaintenance(ctx, db.ListVehicleMaintenanceParams{
		OrganizationID: orgID, VehicleID: vehicleID, LimitCount: limit,
	})
}

func (r *Postgres) ExportMaintenance(ctx context.Context, orgID int64, branchIDs []int64, f model.ExportFilter, limit int32) ([]db.ListVehicleMaintenanceForExportRow, error) {
	return r.q.ListVehicleMaintenanceForExport(ctx, db.ListVehicleMaintenanceForExportParams{
		OrganizationID: orgID, BranchIds: branchIDs, VehicleUuids: uuidsArg(f.VehicleUUIDs),
		Kinds: stringsArg(f.Kinds), MaintenanceTypes: stringsArg(f.MaintenanceTypes), LimitCount: limit,
	})
}

// --- alerts / reminders -------------------------------------------------------------

func (r *Postgres) DocumentAlerts(ctx context.Context, orgID int64, branchIDs []int64, dueBefore time.Time, limit int32) ([]db.ListVehicleDocumentAlertsRow, error) {
	return r.q.ListVehicleDocumentAlerts(ctx, db.ListVehicleDocumentAlertsParams{
		OrganizationID: orgID, BranchIds: branchIDs, DueBefore: DateArg(dueBefore), LimitCount: limit,
	})
}

func (r *Postgres) MaintenanceAlerts(ctx context.Context, orgID int64, branchIDs []int64, dueBefore time.Time, kmHorizon int32, limit int32) ([]db.ListVehicleMaintenanceAlertsRow, error) {
	return r.q.ListVehicleMaintenanceAlerts(ctx, db.ListVehicleMaintenanceAlertsParams{
		OrganizationID: orgID, BranchIds: branchIDs, DueBefore: DateArg(dueBefore), KmHorizon: kmHorizon,
		LimitCount: limit,
	})
}

func (r *Postgres) ReminderOrganizations(ctx context.Context) ([]db.ListVehicleReminderOrganizationsRow, error) {
	return r.q.ListVehicleReminderOrganizations(ctx)
}

func (r *Postgres) ReminderRecipients(ctx context.Context, orgID int64, permission string) ([]db.ListVehicleReminderRecipientsRow, error) {
	return r.q.ListVehicleReminderRecipients(ctx, db.ListVehicleReminderRecipientsParams{
		OrganizationID: orgID, Permission: permission,
	})
}

// ClaimReminder records the reminder of a subject for day; false when it was
// already recorded (the job ran before on that day).
func (r *Postgres) ClaimReminder(ctx context.Context, orgID, vehicleID int64, kind string, subjectID int64, day time.Time, state string) (bool, error) {
	n, err := r.q.ClaimVehicleReminder(ctx, db.ClaimVehicleReminderParams{
		OrganizationID: orgID, VehicleID: vehicleID, SubjectKind: kind, SubjectID: subjectID,
		ReminderDate: DateArg(day), AlertState: state,
	})
	return n > 0, err
}

func (r *Postgres) SetReminderRecipients(ctx context.Context, orgID int64, kind string, subjectID int64, day time.Time, recipients int32) error {
	return r.q.SetVehicleReminderRecipients(ctx, db.SetVehicleReminderRecipientsParams{
		OrganizationID: orgID, SubjectKind: kind, SubjectID: subjectID, ReminderDate: DateArg(day), Recipients: recipients,
	})
}

// LastReminder is the latest earlier reminder of a subject.
type LastReminder struct {
	Day   time.Time
	State string
}

// LastReminders maps "kind:id" to the latest reminder in [since, before).
func (r *Postgres) LastReminders(ctx context.Context, orgID int64, since, before time.Time) (map[string]LastReminder, error) {
	rows, err := r.q.ListLastVehicleReminders(ctx, db.ListLastVehicleRemindersParams{
		OrganizationID: orgID, Since: DateArg(since), Before: DateArg(before),
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]LastReminder, len(rows))
	for _, row := range rows {
		out[ReminderKey(row.SubjectKind, row.SubjectID)] = LastReminder{Day: row.ReminderDate.Time, State: row.AlertState}
	}
	return out, nil
}

// ReminderKey identifies a reminder subject.
func ReminderKey(kind string, id int64) string {
	return kind + ":" + strconv.FormatInt(id, 10)
}

// --- helpers ------------------------------------------------------------------------

// DateArg is the DATE of t (zero = NULL).
func DateArg(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func stringsArg(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func uuidsArg(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}
