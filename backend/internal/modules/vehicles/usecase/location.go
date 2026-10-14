package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	"github.com/google/uuid"
)

// Movement states (the lifecycle of anything that moves a vehicle between
// branches: transfers today, one-way rentals in M3).
const (
	MovePlanned   = "planned"
	MoveInTransit = "in_transit"
	MoveDone      = "done"
)

// Movement is one move of a vehicle from a branch to another. Departure and
// Arrival are the actual times when known, the planned ones otherwise.
type Movement struct {
	FromBranchID int64
	ToBranchID   int64
	Departure    time.Time
	Arrival      time.Time
	State        string
}

// Location answers "where is the vehicle at time t". When InTransit is
// true the vehicle is on the road from FromBranchID to BranchID and is at
// neither branch.
type Location struct {
	BranchID     int64
	InTransit    bool
	FromBranchID int64
}

// ResolveLocation is the rule "a vehicle is not available at another branch
// by itself": the only thing that changes where a vehicle is, is a
// movement. current is the vehicle's current_branch_id as stored at now (it
// already reflects every done movement).
//
//   - t >= now: start at current and apply the planned and in-transit
//     movements in departure order. A movement whose arrival is at or
//     before t has moved the vehicle to its destination; one that departed
//     at or before t but arrives after t has it in transit; later ones do
//     not matter yet.
//   - t < now: start at current and undo the done movements that arrived
//     after t, newest first (the vehicle was at their origin before them,
//     in transit between departure and arrival). A movement still in
//     transit now that departed at or before t has the vehicle in transit
//     at t. Planned movements never happened, so they are ignored.
//
// Movement times are half-open like the busy intervals: at the arrival
// instant the vehicle is at the destination. M3 feeds one-way rentals as
// movements (picked at A, returned at B -> at B after the return); the
// planned vs. actual return location of a rental is M3's call.
func ResolveLocation(current int64, now time.Time, moves []Movement, t time.Time) Location {
	sorted := append([]Movement(nil), moves...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Departure.Before(sorted[j].Departure) })

	if !t.Before(now) {
		loc := Location{BranchID: current}
		for _, m := range sorted {
			if m.State == MoveDone {
				continue
			}
			switch {
			case !m.Arrival.After(t):
				loc = Location{BranchID: m.ToBranchID}
			case !m.Departure.After(t):
				return Location{BranchID: m.ToBranchID, InTransit: true, FromBranchID: m.FromBranchID}
			default:
				return loc
			}
		}
		return loc
	}

	for _, m := range sorted {
		if m.State == MoveInTransit && !m.Departure.After(t) {
			return Location{BranchID: m.ToBranchID, InTransit: true, FromBranchID: m.FromBranchID}
		}
	}
	done := make([]Movement, 0, len(sorted))
	for _, m := range sorted {
		if m.State == MoveDone && m.Arrival.After(t) {
			done = append(done, m)
		}
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].Arrival.After(done[j].Arrival) })
	loc := Location{BranchID: current}
	for _, m := range done {
		if !m.Departure.After(t) {
			return Location{BranchID: m.ToBranchID, InTransit: true, FromBranchID: m.FromBranchID}
		}
		loc = Location{BranchID: m.FromBranchID}
	}
	return loc
}

// MovementsFromTransfers maps the non-cancelled transfers of a vehicle to
// movements.
func MovementsFromTransfers(rows []db.ListVehicleMovementsRow) []Movement {
	out := make([]Movement, 0, len(rows))
	for _, row := range rows {
		m := Movement{
			FromBranchID: row.FromBranchID, ToBranchID: row.ToBranchID,
			Departure: row.PlannedDeparture.Time, Arrival: row.PlannedArrival.Time, State: MovePlanned,
		}
		if row.ActualDeparture.Valid {
			m.Departure, m.State = row.ActualDeparture.Time, MoveInTransit
			if m.Arrival.Before(m.Departure) {
				m.Arrival = m.Departure
			}
		}
		if row.ActualArrival.Valid {
			m.Arrival, m.State = row.ActualArrival.Time, MoveDone
		}
		out = append(out, m)
	}
	return out
}

// LocationAt returns where the vehicle is (or will be) at time t: its
// current branch adjusted by its completed, in-transit and planned
// transfers. The availability engine (TEC-697) uses it; a vehicle outside
// the caller's branch scope reads as not found.
func (s *Service) LocationAt(ctx context.Context, orgID int64, id uuid.UUID, t time.Time) (Location, error) {
	row, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return Location{}, err
	}
	rows, err := s.repo.ListMovements(ctx, orgID, row.Vehicle.ID)
	if err != nil {
		return Location{}, err
	}
	return ResolveLocation(row.Vehicle.CurrentBranchID, s.now(), MovementsFromTransfers(rows), t), nil
}

// OperationalChange is a status transition and / or a move of the current
// branch caused by another module (blocks, transfers).
type OperationalChange struct {
	// Status is the new status ("" = unchanged); a different status is a
	// recorded transition (vehicle_events).
	Status string
	// CurrentBranchID is the new current branch (0 = unchanged).
	CurrentBranchID int64
	Note            string
}

// WithQueries returns a service bound to q, typically another module's
// transaction, so its vehicle changes commit or roll back together with
// that module's rows.
func (s *Service) WithQueries(q *db.Queries) *Service {
	return &Service{repo: repository.NewPostgres(nil, q), indexer: s.indexer, now: s.now}
}

// ApplyOperationalChange applies a status transition and / or a current
// branch move and returns the previous status. The vehicle must be in the
// caller's branch scope.
func (s *Service) ApplyOperationalChange(ctx context.Context, orgID int64, id uuid.UUID, change OperationalChange) (string, error) {
	current, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return "", err
	}
	v := current.Vehicle
	if change.Status != "" && !model.Contains(model.Statuses, change.Status) {
		return "", validation("status", "invalid", "unknown vehicle status")
	}
	moved := change.CurrentBranchID != 0 && change.CurrentBranchID != v.CurrentBranchID
	changed := change.Status != "" && change.Status != v.Status
	if !moved && !changed {
		return v.Status, nil
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if moved {
			if _, err := tx.SetCurrentBranch(ctx, orgID, id, change.CurrentBranchID); err != nil {
				return err
			}
		}
		if changed {
			return transition(ctx, tx, orgID, v, change.Status, v.OdometerKm, v.FuelLevel, change.Note)
		}
		return nil
	})
	if err != nil {
		return "", mapNotFound(err, ErrNotFound)
	}
	s.index(ctx, orgID, id)
	return v.Status, nil
}
