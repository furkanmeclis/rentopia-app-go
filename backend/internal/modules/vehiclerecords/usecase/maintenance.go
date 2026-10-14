package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/repository"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaintenanceChange is the outcome of a maintenance mutation.
type MaintenanceChange struct {
	VehicleUUID uuid.UUID
	Plate       string
	Item        model.Maintenance
}

// ListMaintenance returns the live maintenance items of a vehicle: open
// plans first (soonest first), then the done history.
func (s *Service) ListMaintenance(ctx context.Context, orgID int64, vehicleUUID uuid.UUID) ([]model.Maintenance, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListMaintenance(ctx, orgID, v.ID, MaxVehicleItems)
	if err != nil {
		return nil, err
	}
	out := make([]model.Maintenance, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.mapMaintenance(row, v.OdometerKm))
	}
	return out, nil
}

// CreateMaintenance records a plan (kind planned: a date and / or km
// target) or a done maintenance (kind done: done date required).
func (s *Service) CreateMaintenance(ctx context.Context, orgID int64, vehicleUUID uuid.UUID, in model.MaintenanceInput) (MaintenanceChange, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return MaintenanceChange{}, err
	}
	fields := repository.MaintenanceFields{}
	if err := s.applyMaintenance(&fields, in, true); err != nil {
		return MaintenanceChange{}, err
	}
	if fields.Kind == model.KindDone {
		fields.CompletedBy = actorArg(ctx)
	}
	row, err := s.repo.CreateMaintenance(ctx, orgID, v.ID, fields, actorArg(ctx))
	if err != nil {
		return MaintenanceChange{}, err
	}
	return s.maintenanceChange(ctx, orgID, v, row.Uuid)
}

// PatchMaintenance updates the given fields; kind is fixed (use Complete).
func (s *Service) PatchMaintenance(ctx context.Context, orgID int64, vehicleUUID, id uuid.UUID, in model.MaintenanceInput) (MaintenanceChange, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return MaintenanceChange{}, err
	}
	current, err := s.repo.GetMaintenance(ctx, orgID, v.ID, id)
	if err != nil {
		return MaintenanceChange{}, mapNotFound(err, ErrMaintenanceNotFound)
	}
	if in.Kind != nil && strings.TrimSpace(*in.Kind) != current.Kind {
		return MaintenanceChange{}, validation("kind", "immutable", "kind cannot change; complete a plan instead")
	}
	fields := fieldsFromMaintenance(current)
	if err := s.applyMaintenance(&fields, in, false); err != nil {
		return MaintenanceChange{}, err
	}
	if _, err := s.repo.UpdateMaintenance(ctx, orgID, v.ID, id, fields); err != nil {
		return MaintenanceChange{}, mapNotFound(err, ErrMaintenanceNotFound)
	}
	return s.maintenanceChange(ctx, orgID, v, id)
}

// CompleteMaintenance turns a plan into a done record (same row). done_date
// defaults to today; it must not be in the future.
func (s *Service) CompleteMaintenance(ctx context.Context, orgID int64, vehicleUUID, id uuid.UUID, in model.CompleteInput) (MaintenanceChange, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return MaintenanceChange{}, err
	}
	current, err := s.repo.GetMaintenance(ctx, orgID, v.ID, id)
	if err != nil {
		return MaintenanceChange{}, mapNotFound(err, ErrMaintenanceNotFound)
	}
	if current.Kind != model.KindPlanned {
		return MaintenanceChange{}, validation("kind", "already_done", "the maintenance is already done")
	}
	fields := fieldsFromMaintenance(current)
	fields.Kind = model.KindDone
	fields.DoneDate = dateArg(s.today())
	ve := &ValidationError{}
	if in.DoneDate != nil && strings.TrimSpace(*in.DoneDate) != "" {
		applyDate(ve, "done_date", vehiclemodel.String(*in.DoneDate), &fields.DoneDate)
	}
	applyKm(ve, "done_km", in.DoneKm, &fields.DoneKm)
	applyText(ve, "service_provider", in.ServiceProvider, 255, &fields.ServiceProvider)
	applyCost(ve, in.Cost, &fields.Cost)
	applyText(ve, "note", in.Note, 2000, &fields.Note)
	if fields.DoneDate.Valid && fields.DoneDate.Time.After(s.today()) {
		ve.add("done_date", "in_future", "done_date must not be in the future")
	}
	if err := ve.orNil(); err != nil {
		return MaintenanceChange{}, err
	}
	fields.CompletedBy = actorArg(ctx)
	if _, err := s.repo.UpdateMaintenance(ctx, orgID, v.ID, id, fields); err != nil {
		return MaintenanceChange{}, mapNotFound(err, ErrMaintenanceNotFound)
	}
	return s.maintenanceChange(ctx, orgID, v, id)
}

// DeleteMaintenance soft-deletes a maintenance item.
func (s *Service) DeleteMaintenance(ctx context.Context, orgID int64, vehicleUUID, id uuid.UUID) (MaintenanceChange, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return MaintenanceChange{}, err
	}
	current, err := s.repo.GetMaintenance(ctx, orgID, v.ID, id)
	if err != nil {
		return MaintenanceChange{}, mapNotFound(err, ErrMaintenanceNotFound)
	}
	if err := s.repo.DeleteMaintenance(ctx, orgID, v.ID, id); err != nil {
		return MaintenanceChange{}, mapNotFound(err, ErrMaintenanceNotFound)
	}
	return MaintenanceChange{
		VehicleUUID: v.Uuid, Plate: v.Plate,
		Item: s.mapMaintenance(repository.MaintenanceRow{
			ID: current.ID, Uuid: current.Uuid, Kind: current.Kind, MaintenanceType: current.MaintenanceType,
			PlannedDate: current.PlannedDate, PlannedKm: current.PlannedKm, DoneDate: current.DoneDate,
			DoneKm: current.DoneKm, ServiceProvider: current.ServiceProvider, Cost: current.Cost,
			Currency: current.Currency, Note: current.Note, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt,
		}, v.OdometerKm),
	}, nil
}

func (s *Service) maintenanceChange(ctx context.Context, orgID int64, v db.Vehicle, id uuid.UUID) (MaintenanceChange, error) {
	rows, err := s.repo.ListMaintenance(ctx, orgID, v.ID, MaxVehicleItems)
	if err != nil {
		return MaintenanceChange{}, err
	}
	for _, row := range rows {
		if row.Uuid == id {
			return MaintenanceChange{VehicleUUID: v.Uuid, Plate: v.Plate, Item: s.mapMaintenance(row, v.OdometerKm)}, nil
		}
	}
	return MaintenanceChange{}, ErrMaintenanceNotFound
}

func fieldsFromMaintenance(m db.VehicleMaintenance) repository.MaintenanceFields {
	return repository.MaintenanceFields{
		Kind: m.Kind, MaintenanceType: m.MaintenanceType, PlannedDate: m.PlannedDate, PlannedKm: m.PlannedKm,
		DoneDate: m.DoneDate, DoneKm: m.DoneKm, ServiceProvider: m.ServiceProvider, Cost: m.Cost, Note: m.Note,
		CompletedBy: m.CompletedBy,
	}
}

func (s *Service) applyMaintenance(f *repository.MaintenanceFields, in model.MaintenanceInput, create bool) error {
	ve := &ValidationError{}
	if create {
		kind := ""
		if in.Kind != nil {
			kind = strings.TrimSpace(*in.Kind)
		}
		if !vehiclemodel.Contains(model.Kinds, kind) {
			ve.add("kind", "invalid", "kind must be planned or done")
		}
		f.Kind = kind
	}
	if in.MaintenanceType != nil {
		t := strings.TrimSpace(*in.MaintenanceType)
		if !vehiclemodel.Contains(model.MaintenanceTypes, t) {
			ve.add("maintenance_type", "invalid", "maintenance_type must be one of "+strings.Join(model.MaintenanceTypes, ", "))
		} else {
			f.MaintenanceType = t
		}
	} else if create {
		ve.add("maintenance_type", "required", "maintenance_type is required")
	}
	applyDate(ve, "planned_date", in.PlannedDate, &f.PlannedDate)
	applyKm(ve, "planned_km", in.PlannedKm, &f.PlannedKm)
	applyDate(ve, "done_date", in.DoneDate, &f.DoneDate)
	applyKm(ve, "done_km", in.DoneKm, &f.DoneKm)
	applyText(ve, "service_provider", in.ServiceProvider, 255, &f.ServiceProvider)
	applyText(ve, "note", in.Note, 2000, &f.Note)
	applyCost(ve, in.Cost, &f.Cost)
	switch f.Kind {
	case model.KindPlanned:
		if !f.PlannedDate.Valid && !f.PlannedKm.Valid {
			ve.add("planned_date", "required", "a plan needs planned_date or planned_km")
		}
		// A plan has no done values yet.
		f.DoneDate, f.DoneKm = pgtype.Date{}, pgtype.Int4{}
	case model.KindDone:
		if !f.DoneDate.Valid {
			ve.add("done_date", "required", "done_date is required")
		} else if f.DoneDate.Time.After(s.today()) {
			ve.add("done_date", "in_future", "done_date must not be in the future")
		}
	}
	return ve.orNil()
}

func (s *Service) mapMaintenance(row repository.MaintenanceRow, odometer int32) model.Maintenance {
	today := s.today()
	m := model.Maintenance{
		UUID: row.Uuid, Kind: row.Kind, MaintenanceType: row.MaintenanceType,
		PlannedDate: datePtr(row.PlannedDate), PlannedKm: int4Ptr(row.PlannedKm),
		DoneDate: datePtr(row.DoneDate), DoneKm: int4Ptr(row.DoneKm), ServiceProvider: row.ServiceProvider,
		Currency: row.Currency, Note: row.Note, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if m.Currency == "" {
		m.Currency = model.Currency
	}
	if row.Cost.Valid {
		c := vehicleusecase.FormatMoney(row.Cost)
		m.Cost = &c
	}
	if row.Kind == model.KindDone {
		m.Status = model.KindDone
	} else {
		m.Status = vehiclemodel.MaintenanceState(timePtr(row.PlannedDate), int4Ptr(row.PlannedKm), odometer, today)
		if row.PlannedDate.Valid {
			days := vehiclemodel.DaysUntil(today, row.PlannedDate.Time)
			m.DaysLeft = &days
		}
		if row.PlannedKm.Valid {
			left := row.PlannedKm.Int32 - odometer
			m.KmLeft = &left
		}
	}
	if row.CreatorUuid.Valid {
		m.CreatedBy = &model.Actor{
			UUID: row.CreatorUuid.Bytes, Name: row.CreatorName.String,
			Surname: row.CreatorSurname.String, Email: row.CreatorEmail.String,
		}
	}
	return m
}
