// Package model holds the API shapes of vehicle documents, maintenance and
// expiry alerts (TEC-693). Documents and maintenance are sub-resources of a
// vehicle: they use the vehicle permissions (tenant.vehicles.read / write;
// file downloads also need tenant.document.download) and the vehicle's
// branch scope.
package model

import (
	"strings"
	"time"

	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/google/uuid"
)

// Engine slugs of the export-only io resources.
const (
	ResourceDocuments   = "tenant.vehicle_documents"
	ResourceMaintenance = "tenant.vehicle_maintenance"
)

// Document types.
const (
	DocRegistration = "ruhsat"
	DocTrafficIns   = "trafik_sigortasi"
	DocKasko        = "kasko"
	DocInspection   = "muayene"
	DocEmission     = "egzoz"
	DocOther        = "diger"
)

// DocumentTypes lists the document types.
var DocumentTypes = []string{DocRegistration, DocTrafficIns, DocKasko, DocInspection, DocEmission, DocOther}

// Maintenance kinds: a plan (date and / or km target) or a done record.
// Completing a plan turns the same row into done.
const (
	KindPlanned = "planned"
	KindDone    = "done"
)

// Kinds lists the maintenance kinds.
var Kinds = []string{KindPlanned, KindDone}

// MaintenanceTypes lists the maintenance types.
var MaintenanceTypes = []string{"periodic", "tire", "brake", "oil", "repair", "other"}

// Currency is the only maintenance cost currency (expense linkage: M4).
const Currency = "TRY"

// Alert states (documents: expiring / expired → overdue; maintenance: due /
// overdue).
const (
	AlertExpiring = "expiring"
	AlertOverdue  = "overdue"
)

// Alert kinds.
const (
	AlertDocument    = "document"
	AlertMaintenance = "maintenance"
)

// Re-exported horizons (see vehicles/model/expiry.go).
const (
	DocumentReminderDays    = vehiclemodel.DocumentReminderDays
	MaintenanceReminderDays = vehiclemodel.MaintenanceReminderDays
	MaintenanceReminderKm   = vehiclemodel.MaintenanceReminderKm
)

// Actor is the user who recorded a document or a maintenance item.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// FileInfo describes the scanned file of a document. The object key never
// leaves the API: downloads go through GET …/documents/{uuid}/file.
type FileInfo struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

// Document is the API projection of a vehicle document.
type Document struct {
	UUID           uuid.UUID `json:"uuid"`
	DocType        string    `json:"doc_type"`
	DocumentNumber string    `json:"document_number"`
	Issuer         string    `json:"issuer"`
	ValidFrom      *string   `json:"valid_from"`
	ValidUntil     *string   `json:"valid_until"`
	// Status: none (no valid_until) | valid | expiring | expired.
	Status string `json:"status"`
	// DaysLeft until valid_until (negative when expired); nil without one.
	DaysLeft *int `json:"days_left"`
	// IsCurrent: the live document of its type with the latest valid_until.
	IsCurrent bool      `json:"is_current"`
	File      *FileInfo `json:"file"`
	Note      string    `json:"note"`
	CreatedBy *Actor    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DocumentInput is the create / patch body of a document. Nil fields are
// unchanged on patch.
type DocumentInput struct {
	DocType        *string                     `json:"doc_type"`
	DocumentNumber *string                     `json:"document_number"`
	Issuer         *string                     `json:"issuer"`
	ValidFrom      vehiclemodel.OptionalString `json:"valid_from"`
	ValidUntil     vehiclemodel.OptionalString `json:"valid_until"`
	Note           *string                     `json:"note"`
}

// Maintenance is the API projection of a maintenance item.
type Maintenance struct {
	UUID            uuid.UUID `json:"uuid"`
	Kind            string    `json:"kind"`
	MaintenanceType string    `json:"maintenance_type"`
	PlannedDate     *string   `json:"planned_date"`
	PlannedKm       *int32    `json:"planned_km"`
	DoneDate        *string   `json:"done_date"`
	DoneKm          *int32    `json:"done_km"`
	ServiceProvider string    `json:"service_provider"`
	Cost            *string   `json:"cost"`
	Currency        string    `json:"currency"`
	Note            string    `json:"note"`
	// Status: done, or for a plan scheduled | due | overdue.
	Status string `json:"status"`
	// DaysLeft / KmLeft until the plan's targets (plans only).
	DaysLeft  *int      `json:"days_left"`
	KmLeft    *int32    `json:"km_left"`
	CreatedBy *Actor    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MaintenanceInput is the create / patch body of a maintenance item. kind is
// fixed after create (POST …/complete turns a plan into done).
type MaintenanceInput struct {
	Kind            *string                     `json:"kind"`
	MaintenanceType *string                     `json:"maintenance_type"`
	PlannedDate     vehiclemodel.OptionalString `json:"planned_date"`
	PlannedKm       vehiclemodel.OptionalInt    `json:"planned_km"`
	DoneDate        vehiclemodel.OptionalString `json:"done_date"`
	DoneKm          vehiclemodel.OptionalInt    `json:"done_km"`
	ServiceProvider *string                     `json:"service_provider"`
	Cost            vehiclemodel.OptionalString `json:"cost"`
	Note            *string                     `json:"note"`
}

// CompleteInput is the body of POST …/maintenance/{uuid}/complete. done_date
// defaults to today; other fields keep the plan's values when omitted.
type CompleteInput struct {
	DoneDate        *string                     `json:"done_date"`
	DoneKm          vehiclemodel.OptionalInt    `json:"done_km"`
	ServiceProvider *string                     `json:"service_provider"`
	Cost            vehiclemodel.OptionalString `json:"cost"`
	Note            *string                     `json:"note"`
}

// AlertVehicle is the vehicle of an alert.
type AlertVehicle struct {
	UUID          uuid.UUID `json:"uuid"`
	Plate         string    `json:"plate"`
	Label         string    `json:"label"`
	OdometerKm    int32     `json:"odometer_km"`
	CurrentBranch string    `json:"current_branch"`
}

// Alert is one upcoming or overdue document / maintenance plan.
type Alert struct {
	Kind        string    `json:"kind"`
	State       string    `json:"state"`
	SubjectUUID uuid.UUID `json:"subject_uuid"`
	// Type is the document type or the maintenance type.
	Type           string       `json:"type"`
	DocumentNumber string       `json:"document_number"`
	DueDate        *string      `json:"due_date"`
	DueKm          *int32       `json:"due_km"`
	DaysLeft       *int         `json:"days_left"`
	KmLeft         *int32       `json:"km_left"`
	Vehicle        AlertVehicle `json:"vehicle"`
}

// AlertsView is the answer of GET /v1/tenant/fleet/alerts.
type AlertsView struct {
	Items                  []Alert `json:"items"`
	Total                  int     `json:"total"`
	DocumentHorizonDays    int     `json:"document_horizon_days"`
	MaintenanceHorizonDays int     `json:"maintenance_horizon_days"`
	MaintenanceHorizonKm   int     `json:"maintenance_horizon_km"`
}

// ExportFilter selects documents / maintenance items for an export.
type ExportFilter struct {
	VehicleUUIDs     []uuid.UUID
	DocTypes         []string
	Kinds            []string
	MaintenanceTypes []string
}

// Export query keys (POST …/export query).
var (
	DocumentExportKeys    = []string{"vehicle", "doc_type"}
	MaintenanceExportKeys = []string{"vehicle", "kind", "maintenance_type"}
)

// ExportFilterFromQuery maps export query params (comma-separated values).
func ExportFilterFromQuery(query map[string]string) ExportFilter {
	f := ExportFilter{
		DocTypes:         vehiclemodel.SplitList(query["doc_type"]),
		Kinds:            vehiclemodel.SplitList(query["kind"]),
		MaintenanceTypes: vehiclemodel.SplitList(query["maintenance_type"]),
	}
	for _, v := range vehiclemodel.SplitList(query["vehicle"]) {
		// An invalid uuid matches nothing (uuid.Nil) rather than dropping
		// the filter.
		id, err := uuid.Parse(strings.TrimSpace(v))
		if err != nil {
			id = uuid.Nil
		}
		f.VehicleUUIDs = append(f.VehicleUUIDs, id)
	}
	return f
}
