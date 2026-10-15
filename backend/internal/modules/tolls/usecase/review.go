package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// lockVisible loads a pass FOR UPDATE and checks the caller's branch scope
// (a pass outside the scope answers ErrNotFound).
func (s *Service) lockVisible(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (pass, error) {
	p, err := q.LockTollPassByUUID(ctx, db.LockTollPassByUUIDParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	ok, err := s.passVisible(ctx, q, orgID, p.VehicleID, p.RentalID)
	if err != nil {
		return p, err
	}
	if !ok {
		return p, ErrNotFound
	}
	return p, nil
}

// passVisible: the vehicle's home / current branch or the rental's
// branches are in the caller's scope (a pass with neither: every branch
// only).
func (s *Service) passVisible(ctx context.Context, q *db.Queries, orgID int64, vehicleID, rentalID pgtype.Int8) (bool, error) {
	ids := []pgtype.Int8{}
	if vehicleID.Valid {
		v, err := q.GetTollVehicleByID(ctx, db.GetTollVehicleByIDParams{OrganizationID: orgID, ID: vehicleID.Int64})
		if err != nil && !notFound(err) {
			return false, err
		}
		if err == nil {
			ids = append(ids, i8(v.HomeBranchID), i8(v.CurrentBranchID))
		}
	}
	if rentalID.Valid {
		r, err := q.GetTollRental(ctx, db.GetTollRentalParams{OrganizationID: orgID, ID: rentalID.Int64})
		if err != nil && !notFound(err) {
			return false, err
		}
		if err == nil {
			ids = append(ids, i8(r.PickupBranchID), i8(r.ReturnBranchID), r.ActualReturnBranchID)
		}
	}
	if len(ids) == 0 {
		ids = append(ids, pgtype.Int8{})
	}
	return visible(ctx, ids...), nil
}

// decision runs one manual decision on a pass in a transaction and
// publishes the change.
func (s *Service) decision(ctx context.Context, orgID int64, id uuid.UUID, fn func(q *db.Queries, tx pgx.Tx, p *pass) ([]uuid.UUID, error)) (model.PassDetail, error) {
	var rentals []uuid.UUID
	err := s.tx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		p, err := s.lockVisible(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		rentals, err = fn(q, tx, &p)
		return err
	})
	if err != nil {
		return model.PassDetail{}, err
	}
	s.publish(ctx, orgID, "updated", []uuid.UUID{id}, rentals)
	return s.Get(ctx, orgID, id)
}

func rentalRef(r db.GetTollRentalRow) map[string]any {
	return map[string]any{"rental_uuid": r.Uuid, "rental_reference": r.Reference}
}

// Assign puts a pass on a rental chosen by a person (mandatory reason,
// audited): the old charge is reversed and the new one created. Automatic
// matching keeps the assignment from now on.
func (s *Service) Assign(ctx context.Context, orgID int64, id uuid.UUID, in model.AssignInput) (model.PassDetail, error) {
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return model.PassDetail{}, err
	}
	if in.RentalUUID == nil {
		return model.PassDetail{}, validation("rental_uuid", "required", "rental_uuid is required")
	}
	return s.decision(ctx, orgID, id, func(q *db.Queries, tx pgx.Tx, p *pass) ([]uuid.UUID, error) {
		r, err := q.GetTollRentalByUUID(ctx, db.GetTollRentalByUUIDParams{OrganizationID: orgID, Uuid: *in.RentalUUID})
		if notFound(err) {
			return nil, validation("rental_uuid", "not_found", "rental not found")
		}
		if err != nil {
			return nil, err
		}
		if !visible(ctx, i8(r.PickupBranchID), i8(r.ReturnBranchID), r.ActualReturnBranchID) {
			return nil, validation("rental_uuid", "not_found", "rental not found")
		}
		if r.Status == "cancelled" || r.Status == "preparation" {
			return nil, validation("rental_uuid", "invalid_state", "the rental was not handed over (or is cancelled)")
		}
		fromStatus, fromRental := p.Status, p.RentalID
		vehicle := p.VehicleID
		if !vehicle.Valid {
			vehicle = i8(r.VehicleID)
		}
		rentals, err := s.settle(ctx, q, tx, orgID, p, target{
			status: model.StatusMatched, vehicleID: vehicle, rentalID: i8(r.ID), method: model.MethodManual, manual: true, decided: true,
		}, reason)
		if err != nil {
			return nil, err
		}
		data := rentalRef(db.GetTollRentalRow(r))
		if fromRental.Valid && fromRental.Int64 != r.ID {
			if old, err := q.GetTollRental(ctx, db.GetTollRentalParams{OrganizationID: orgID, ID: fromRental.Int64}); err == nil {
				data["from_rental_uuid"], data["from_rental_reference"] = old.Uuid, old.Reference
			}
		}
		return rentals, passEvent(ctx, q, orgID, p.ID, model.EventAssigned, fromStatus, p.Status, reason, data)
	})
}

// Unassign removes the rental of a pass (mandatory reason, audited): its
// charges are reversed and the pass waits in the queue as unmatched.
func (s *Service) Unassign(ctx context.Context, orgID int64, id uuid.UUID, in model.ReasonInput) (model.PassDetail, error) {
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return model.PassDetail{}, err
	}
	return s.decision(ctx, orgID, id, func(q *db.Queries, tx pgx.Tx, p *pass) ([]uuid.UUID, error) {
		if !p.RentalID.Valid {
			return nil, &StateError{Code: "not_assigned", Message: "the pass is not on a rental"}
		}
		from := p.Status
		old, err := q.GetTollRental(ctx, db.GetTollRentalParams{OrganizationID: orgID, ID: p.RentalID.Int64})
		if err != nil {
			return nil, err
		}
		rentals, err := s.settle(ctx, q, tx, orgID, p, target{
			status: model.StatusUnmatched, reason: model.ReasonUnassigned, vehicleID: p.VehicleID, method: p.MatchMethod,
			manual: true, decided: true,
		}, reason)
		if err != nil {
			return nil, err
		}
		return rentals, passEvent(ctx, q, orgID, p.ID, model.EventUnassigned, from, p.Status, reason, rentalRef(old))
	})
}

// Ignore marks a pass as not billed (mandatory reason, audited); any live
// charge is reversed.
func (s *Service) Ignore(ctx context.Context, orgID int64, id uuid.UUID, in model.ReasonInput) (model.PassDetail, error) {
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return model.PassDetail{}, err
	}
	return s.decision(ctx, orgID, id, func(q *db.Queries, tx pgx.Tx, p *pass) ([]uuid.UUID, error) {
		if p.Status == model.StatusIgnored {
			return nil, &StateError{Code: "already_ignored", Message: "the pass is already ignored"}
		}
		from := p.Status
		rentals, err := s.settle(ctx, q, tx, orgID, p, target{
			status: model.StatusIgnored, vehicleID: p.VehicleID, method: p.MatchMethod, manual: true, decided: true,
		}, reason)
		if err != nil {
			return nil, err
		}
		return rentals, passEvent(ctx, q, orgID, p.ID, model.EventIgnored, from, p.Status, reason, nil)
	})
}

// Rematch hands a pass back to the automatic matching (a person's earlier
// decision is dropped; mandatory reason, audited).
func (s *Service) Rematch(ctx context.Context, orgID int64, id uuid.UUID, in model.ReasonInput) (model.PassDetail, error) {
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return model.PassDetail{}, err
	}
	return s.decision(ctx, orgID, id, func(q *db.Queries, tx pgx.Tx, p *pass) ([]uuid.UUID, error) {
		from := p.Status
		p.Manual = false
		if p.Status == model.StatusIgnored {
			p.Status = model.StatusNeedsReview
		}
		t, err := s.decide(ctx, q, orgID, *p)
		if err != nil {
			return nil, err
		}
		t.decided = true
		p.Status = from
		rentals, err := s.settle(ctx, q, tx, orgID, p, t, reason)
		if err != nil {
			return nil, err
		}
		return rentals, passEvent(ctx, q, orgID, p.ID, model.EventRematched, from, p.Status, reason, map[string]any{
			"review_reason": p.ReviewReason,
		})
	})
}

// StateError is answered with 409 INVALID_STATE.
type StateError struct{ Code, Message string }

func (e *StateError) Error() string { return e.Message }

// Candidates lists the rentals of the pass's vehicle within three days of
// the pass (the assign dialog), in the caller's branch scope.
func (s *Service) Candidates(ctx context.Context, orgID int64, id uuid.UUID) ([]model.Candidate, error) {
	row, err := s.q.GetTollPass(ctx, db.GetTollPassParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.passVisible(ctx, s.q, orgID, row.VehicleID, row.RentalID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	out := []model.Candidate{}
	vehicleID := row.VehicleID
	if !vehicleID.Valid && strings.TrimSpace(row.Plate) != "" {
		// No vehicle matched: try the plate once more (a vehicle added
		// since the pass arrived).
		key := strings.ToUpper(strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
				return r
			}
			return -1
		}, row.Plate))
		if vs, err := s.q.FindTollVehiclesByPlate(ctx, db.FindTollVehiclesByPlateParams{OrganizationID: orgID, PlateKey: pgtype.Text{String: key, Valid: true}}); err == nil && len(vs) > 0 {
			vehicleID = i8(vs[0].ID)
		}
	}
	if !vehicleID.Valid {
		return out, nil
	}
	at := row.PassedAt.Time
	rows, err := s.q.ListTollCandidateRentals(ctx, db.ListTollCandidateRentalsParams{
		OrganizationID: orgID, VehicleID: vehicleID.Int64, At: row.PassedAt,
		WindowFrom: ts(at.Add(-72 * time.Hour)), WindowTo: ts(at.Add(72 * time.Hour)),
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if !visible(ctx, i8(r.PickupBranchID), i8(r.ReturnBranchID), r.ActualReturnBranchID) {
			continue
		}
		name := strings.TrimSpace(r.FirstName + " " + r.LastName)
		if r.CustomerType == "corporate" && strings.TrimSpace(r.CompanyTitle) != "" {
			name = r.CompanyTitle
		}
		out = append(out, model.Candidate{
			UUID: r.Uuid, Reference: r.Reference, Status: r.Status, Customer: name,
			PlannedPickupAt: r.PlannedPickupAt.Time.UTC(), PlannedReturnAt: r.PlannedReturnAt.Time.UTC(),
			ActualPickupAt: timePtr(r.ActualPickupAt), ActualReturnAt: timePtr(r.ActualReturnAt), ContainsPass: r.ContainsPass,
		})
	}
	return out, nil
}
