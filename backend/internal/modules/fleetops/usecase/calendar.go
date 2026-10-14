package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// ValidateCalendarWindow checks [from, to): both set, to after from and at
// most model.CalendarMaxDays wide.
func ValidateCalendarWindow(from, to time.Time) error {
	ve := &ValidationError{}
	if from.IsZero() {
		ve.add("from", "required", "from is required (RFC 3339)")
	}
	if to.IsZero() {
		ve.add("to", "required", "to is required (RFC 3339)")
	}
	if ve.orNil() != nil {
		return ve
	}
	if !to.After(from) {
		ve.add("to", "before_from", "to must be after from")
	} else if to.Sub(from) > model.CalendarMaxDays*24*time.Hour {
		ve.add("to", "range_too_large", "the window may span at most 93 days")
	}
	return ve.orNil()
}

// CalendarLayers are the bar kinds the caller's tenant permissions show.
// Maintenance belongs to the vehicle (tenant.vehicles.read, already
// required by the route).
func CalendarLayers(ctx context.Context) []string {
	layers := []string{}
	sc, _ := orgctx.From(ctx)
	if sc.Has(rbac.PermTenantBlocksRead) {
		layers = append(layers, model.LayerBlock)
	}
	if sc.Has(rbac.PermTenantTransfersRead) {
		layers = append(layers, model.LayerTransfer)
	}
	return append(layers, model.LayerMaintenance)
}

// Calendar returns one page of vehicles in the caller's branch scope with
// their busy intervals and maintenance days inside [f.From, f.To).
func (s *Service) Calendar(ctx context.Context, orgID int64, f model.CalendarFilter) (model.Calendar, error) {
	f.From, f.To = f.From.UTC(), f.To.UTC()
	if err := ValidateCalendarWindow(f.From, f.To); err != nil {
		return model.Calendar{}, err
	}
	if f.Limit <= 0 {
		f.Limit = model.CalendarDefaultLimit
	}
	if f.Limit > model.CalendarMaxLimit {
		f.Limit = model.CalendarMaxLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	f.BranchIDs = orgctx.BranchScope(ctx)
	layers := CalendarLayers(ctx)
	out := model.Calendar{
		From: f.From, To: f.To, Items: []model.CalendarRow{}, Limit: f.Limit, Offset: f.Offset, Layers: layers,
	}

	vehicles, total, err := s.repo.CalendarVehicles(ctx, orgID, f)
	if err != nil {
		return model.Calendar{}, err
	}
	out.Total = total
	if len(vehicles) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(vehicles))
	index := make(map[int64]int, len(vehicles))
	for i, v := range vehicles {
		ids = append(ids, v.ID)
		index[v.ID] = i
		out.Items = append(out.Items, model.CalendarRow{
			Vehicle:     calendarVehicle(v),
			Intervals:   []model.CalendarInterval{},
			Maintenance: []model.CalendarMaintenance{},
		})
	}

	kinds := []string{}
	for _, l := range layers {
		if l == model.LayerBlock || l == model.LayerTransfer {
			kinds = append(kinds, l)
		}
	}
	intervals, err := s.repo.CalendarIntervals(ctx, orgID, ids, f.From, f.To, kinds)
	if err != nil {
		return model.Calendar{}, err
	}
	for _, row := range intervals {
		i, ok := index[row.VehicleID]
		if !ok || !row.StartsAt.Valid {
			continue
		}
		item := CalendarIntervalFromRow(ctx, row, f.From, f.To)
		out.Items[i].Intervals = append(out.Items[i].Intervals, item)
	}

	today := vehiclemodel.Today(s.clock())
	fromDay, toDay := CalendarDays(f.From, f.To)
	maintenance, err := s.repo.CalendarMaintenance(ctx, orgID, ids, fromDay, toDay)
	if err != nil {
		return model.Calendar{}, err
	}
	for _, row := range maintenance {
		i, ok := index[row.VehicleID]
		if !ok || !row.Day.Valid {
			continue
		}
		out.Items[i].Maintenance = append(out.Items[i].Maintenance, calendarMaintenance(row, today))
	}
	return out, nil
}

// CalendarDays is the day range [fromDay, toDay) (Türkiye calendar days,
// midnight UTC like DATE columns) that covers the window [from, to).
func CalendarDays(from, to time.Time) (time.Time, time.Time) {
	return vehiclemodel.Today(from), vehiclemodel.Today(to.Add(-time.Nanosecond)).AddDate(0, 0, 1)
}

// ClipInterval clips [start, end) (end nil = unbounded) to the window
// [from, to) and reports whether the interval runs past either edge.
func ClipInterval(start time.Time, end *time.Time, from, to time.Time) (clipStart, clipEnd time.Time, before, after bool) {
	clipStart, clipEnd = start, to
	if start.Before(from) {
		clipStart, before = from, true
	}
	if end == nil || end.After(to) {
		after = true
	} else {
		clipEnd = *end
	}
	return clipStart, clipEnd, before, after
}

// CalendarIntervalFromRow maps a busy interval row to a bar clipped to
// [from, to).
func CalendarIntervalFromRow(ctx context.Context, row repository.CalendarIntervalRow, from, to time.Time) model.CalendarInterval {
	start := row.StartsAt.Time.UTC()
	end := timePtr(row.EndsAt)
	clipStart, clipEnd, before, after := ClipInterval(start, end, from, to)
	item := model.CalendarInterval{
		Kind: row.Kind, UUID: row.RefUuid, StartsAt: start, EndsAt: end,
		ClipStart: clipStart, ClipEnd: clipEnd, ContinuesBefore: before, ContinuesAfter: after,
	}
	switch row.Kind {
	case model.LayerBlock:
		item.Block = &model.CalendarBlock{
			Kind: row.BlockKind, Reason: row.BlockReason, ReleasedAt: timePtr(row.BlockReleasedAt),
		}
	case model.LayerTransfer:
		item.Transfer = &model.CalendarTransfer{
			Status:           row.TransferStatus,
			PlannedDeparture: row.PlannedDeparture.Time.UTC(),
			PlannedArrival:   row.PlannedArrival.Time.UTC(),
			ActualDeparture:  timePtr(row.ActualDeparture),
			ActualArrival:    timePtr(row.ActualArrival),
			FromBranch:       branchRef(row.FromBranchUuid, row.FromBranchCode, row.FromBranchName),
			ToBranch:         branchRef(row.ToBranchUuid, row.ToBranchCode, row.ToBranchName),
			InScope:          eitherInScope(ctx, row.FromBranchID, row.ToBranchID),
		}
	}
	return item
}

func calendarVehicle(v repository.CalendarVehicleRow) model.CalendarVehicle {
	return model.CalendarVehicle{
		UUID: v.Uuid, Plate: v.Plate, Label: v.VehicleLabel, ModelYear: v.ModelYear,
		Status: v.Status, IsActive: v.IsActive,
		HomeBranch:    model.BranchRef{UUID: v.HomeBranchUuid, Code: v.HomeBranchCode, Name: v.HomeBranchName},
		CurrentBranch: model.BranchRef{UUID: v.CurrentBranchUuid, Code: v.CurrentBranchCode, Name: v.CurrentBranchName},
	}
}

func calendarMaintenance(row repository.CalendarMaintenanceRow, today time.Time) model.CalendarMaintenance {
	day := row.Day.Time.UTC()
	item := model.CalendarMaintenance{
		UUID: row.Uuid, Kind: row.Kind, MaintenanceType: row.MaintenanceType,
		Date: day.Format(time.DateOnly), State: MaintenanceDayState(row.Kind, day, today),
		PlannedKm: int4Ptr(row.PlannedKm), DoneKm: int4Ptr(row.DoneKm),
		ServiceProvider: row.ServiceProvider, Note: row.Note,
	}
	return item
}

// MaintenanceDayState is the marker state of a maintenance day: done items
// are done; open plans are overdue before today, due within the reminder
// horizon, scheduled otherwise.
func MaintenanceDayState(kind string, day, today time.Time) string {
	switch {
	case kind == "done":
		return model.MaintenanceStateDone
	case day.Before(today):
		return model.MaintenanceStateOverdue
	case vehiclemodel.DaysUntil(today, day) <= vehiclemodel.MaintenanceReminderDays:
		return model.MaintenanceStateDue
	default:
		return model.MaintenanceStateScheduled
	}
}

func branchRef(id pgtype.UUID, code, name pgtype.Text) model.BranchRef {
	if !id.Valid {
		return model.BranchRef{}
	}
	return model.BranchRef{UUID: id.Bytes, Code: code.String, Name: name.String}
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}
