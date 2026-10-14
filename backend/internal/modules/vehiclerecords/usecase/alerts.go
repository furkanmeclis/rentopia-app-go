package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// alert is an Alert with the internal ids the reminder job needs.
type alert struct {
	model.Alert
	subjectID       int64
	vehicleID       int64
	homeBranchID    int64
	currentBranchID int64
}

// Alerts lists the upcoming and overdue documents and maintenance plans of
// the organization's vehicles in the caller's branch scope ("Operasyon
// özeti"): overdue first, then by due date.
func (s *Service) Alerts(ctx context.Context, orgID int64) (model.AlertsView, error) {
	items, err := s.alerts(ctx, orgID, orgctx.BranchScope(ctx), s.today())
	if err != nil {
		return model.AlertsView{}, err
	}
	out := model.AlertsView{
		Items: make([]model.Alert, 0, len(items)), Total: len(items),
		DocumentHorizonDays:    model.DocumentReminderDays,
		MaintenanceHorizonDays: model.MaintenanceReminderDays,
		MaintenanceHorizonKm:   model.MaintenanceReminderKm,
	}
	for _, a := range items {
		out.Items = append(out.Items, a.Alert)
	}
	return out, nil
}

// alerts computes the alerts of an organization for a day; branchIDs nil =
// every branch (reminder job).
func (s *Service) alerts(ctx context.Context, orgID int64, branchIDs []int64, today time.Time) ([]alert, error) {
	docs, err := s.repo.DocumentAlerts(ctx, orgID, branchIDs, today.AddDate(0, 0, model.DocumentReminderDays), MaxAlerts)
	if err != nil {
		return nil, err
	}
	plans, err := s.repo.MaintenanceAlerts(ctx, orgID, branchIDs,
		today.AddDate(0, 0, model.MaintenanceReminderDays), model.MaintenanceReminderKm, MaxAlerts)
	if err != nil {
		return nil, err
	}
	out := make([]alert, 0, len(docs)+len(plans))
	for _, d := range docs {
		out = append(out, documentAlert(d, today))
	}
	for _, m := range plans {
		out = append(out, maintenanceAlert(m, today))
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.State == model.AlertOverdue) != (b.State == model.AlertOverdue) {
			return a.State == model.AlertOverdue
		}
		dueA, dueB := dueSortKey(a.Alert), dueSortKey(b.Alert)
		if dueA != dueB {
			return dueA < dueB
		}
		return a.Vehicle.Plate < b.Vehicle.Plate
	})
	return out, nil
}

// dueSortKey orders by days left; km-only plans sort after dated ones.
func dueSortKey(a model.Alert) int {
	if a.DaysLeft != nil {
		return *a.DaysLeft
	}
	return 1 << 20
}

func documentAlert(d db.ListVehicleDocumentAlertsRow, today time.Time) alert {
	days := vehiclemodel.DaysUntil(today, d.ValidUntil.Time)
	state := model.AlertExpiring
	if days < 0 {
		state = model.AlertOverdue
	}
	return alert{
		Alert: model.Alert{
			Kind: model.AlertDocument, State: state, SubjectUUID: d.Uuid, Type: d.DocType,
			DocumentNumber: d.DocumentNumber, DueDate: datePtr(d.ValidUntil), DaysLeft: &days,
			Vehicle: alertVehicle(d.VehicleUuid, d.Plate, d.VehicleLabel, d.OdometerKm, d.CurrentBranchName),
		},
		subjectID: d.ID, vehicleID: d.VehicleID, homeBranchID: d.HomeBranchID, currentBranchID: d.CurrentBranchID,
	}
}

func maintenanceAlert(m db.ListVehicleMaintenanceAlertsRow, today time.Time) alert {
	a := model.Alert{
		Kind: model.AlertMaintenance, SubjectUUID: m.Uuid, Type: m.MaintenanceType,
		DueDate: datePtr(m.PlannedDate), DueKm: int4Ptr(m.PlannedKm),
		Vehicle: alertVehicle(m.VehicleUuid, m.Plate, m.VehicleLabel, m.OdometerKm, m.CurrentBranchName),
	}
	if m.PlannedDate.Valid {
		days := vehiclemodel.DaysUntil(today, m.PlannedDate.Time)
		a.DaysLeft = &days
	}
	if m.PlannedKm.Valid {
		left := m.PlannedKm.Int32 - m.OdometerKm
		a.KmLeft = &left
	}
	a.State = model.AlertExpiring
	if vehiclemodel.MaintenanceState(timePtr(m.PlannedDate), int4Ptr(m.PlannedKm), m.OdometerKm, today) == vehiclemodel.MaintenanceOverdue {
		a.State = model.AlertOverdue
	}
	return alert{
		Alert: a, subjectID: m.ID, vehicleID: m.VehicleID, homeBranchID: m.HomeBranchID, currentBranchID: m.CurrentBranchID,
	}
}

func alertVehicle(id uuid.UUID, plate, label string, km int32, branch string) model.AlertVehicle {
	return model.AlertVehicle{UUID: id, Plate: plate, Label: label, OdometerKm: km, CurrentBranch: branch}
}
