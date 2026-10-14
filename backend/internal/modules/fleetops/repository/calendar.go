package repository

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/jackc/pgx/v5/pgtype"
)

// Calendar row aliases.
type (
	CalendarVehicleRow     = db.ListFleetCalendarVehiclesRow
	CalendarIntervalRow    = db.ListFleetCalendarIntervalsRow
	CalendarMaintenanceRow = db.ListFleetCalendarMaintenanceRow
)

// CalendarVehicles returns one page of vehicles in f.BranchIDs (nil = every
// branch) and the number of matching vehicles.
func (r *Postgres) CalendarVehicles(ctx context.Context, orgID int64, f model.CalendarFilter) ([]CalendarVehicleRow, int64, error) {
	rows, err := r.q.ListFleetCalendarVehicles(ctx, db.ListFleetCalendarVehiclesParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, VehicleUuids: uuidsArg(f.VehicleUUIDs),
		IsActive: boolArg(f.IsActive), Statuses: stringsArg(f.Statuses),
		HomeBranchUuids: uuidsArg(f.HomeBranchUUIDs), CurrentBranchUuids: uuidsArg(f.CurrentBranchUUIDs),
		GroupUuids: uuidsArg(f.GroupUUIDs), SegmentUuids: uuidsArg(f.SegmentUUIDs), Q: textArg(f.Q),
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if len(rows) > 0 {
		total = rows[0].Total
	}
	return rows, total, nil
}

// CalendarIntervals returns the busy intervals of vehicleIDs that overlap
// [from, to), limited to kinds.
func (r *Postgres) CalendarIntervals(ctx context.Context, orgID int64, vehicleIDs []int64, from, to time.Time, kinds []string) ([]CalendarIntervalRow, error) {
	if len(vehicleIDs) == 0 || len(kinds) == 0 {
		return []CalendarIntervalRow{}, nil
	}
	return r.q.ListFleetCalendarIntervals(ctx, db.ListFleetCalendarIntervalsParams{
		OrganizationID: orgID, VehicleIds: vehicleIDs, FromAt: ts(from), ToAt: ts(to), Kinds: kinds,
	})
}

// CalendarMaintenance returns the maintenance items of vehicleIDs on a day
// in [fromDay, toDay).
func (r *Postgres) CalendarMaintenance(ctx context.Context, orgID int64, vehicleIDs []int64, fromDay, toDay time.Time) ([]CalendarMaintenanceRow, error) {
	if len(vehicleIDs) == 0 {
		return []CalendarMaintenanceRow{}, nil
	}
	return r.q.ListFleetCalendarMaintenance(ctx, db.ListFleetCalendarMaintenanceParams{
		OrganizationID: orgID, VehicleIds: vehicleIDs,
		FromDay: pgtype.Date{Time: fromDay, Valid: true}, ToDay: pgtype.Date{Time: toDay, Valid: true},
	})
}
