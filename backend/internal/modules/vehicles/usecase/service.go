// Package usecase holds the vehicles business rules (TEC-692). Every method
// takes the organization id of the caller explicitly (from orgctx.Scope in
// handlers, from the job in workers) and passes it to the repository.
//
// Branch scope: a vehicle belongs to its home branch and is physically at
// its current branch. A branch-restricted member sees and acts on a vehicle
// when either branch is in the scope; a vehicle outside it reads as not
// found (404). New vehicles and home-branch changes need the (new) home
// branch in the scope; the current branch may be any branch of the
// organization (vehicles move between branches).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Cmd+K spec id of vehicles.
const SearchSpec = "vehicles"

// MaxListTargets caps exports, bulk "all matching" scopes and the eligible
// vehicles of a product.
const MaxListTargets = 10000

var (
	ErrNotFound           = errors.New("vehicle not found")
	ErrPlateTaken         = errors.New("an active vehicle already has this plate")
	ErrGroupNotFound      = errors.New("rental group not found")
	ErrMembershipNotFound = errors.New("the vehicle is not in this rental group")
	ErrProductNotFound    = errors.New("rental product not found")
)

// NotEligibleError is returned when a vehicle does not meet the protected
// attributes / catalog criteria of a rental group (handlers answer 422).
type NotEligibleError struct {
	GroupUUID uuid.UUID
	GroupCode string
	Reasons   []string
}

func (e *NotEligibleError) Error() string {
	return "vehicle is not eligible for rental group " + e.GroupCode + ": " + strings.Join(e.Reasons, ", ")
}

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400 VALIDATION_ERROR.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) has(field string) bool {
	for _, d := range e.Details {
		if d.Field == field {
			return true
		}
	}
	return false
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

func validation(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service implements the vehicles use cases.
type Service struct {
	repo    *repository.Postgres
	indexer SearchIndexer
	now     func() time.Time
	guard   MembershipGuard
}

// New builds the service. indexer may be nil.
func New(repo *repository.Postgres, indexer SearchIndexer) *Service {
	return &Service{repo: repo, indexer: indexer, now: time.Now}
}

// SetSearchIndexer sets the Cmd+K indexer after construction (worker wiring).
func (s *Service) SetSearchIndexer(indexer SearchIndexer) {
	s.indexer = indexer
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"plate": "plate", "model_year": "model_year", "odometer_km": "odometer_km", "status": "status",
	"kasko_value": "kasko_value", "is_active": "is_active", "created_at": "created_at", "updated_at": "updated_at",
	"inspection_until": "inspection_until", "insurance_until": "insurance_until", "kasko_until": "kasko_until",
	"document_expires_at": "document_expires_at", "next_maintenance_date": "next_maintenance_date",
	"next_maintenance_km": "next_maintenance_km",
}

// List returns one page of the organization's vehicles in the caller's
// branch scope.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Vehicle, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	f.BranchIDs = orgctx.BranchScope(ctx)
	f.Today = model.Today(s.now())
	rows, total, err := s.repo.ListVehicles(ctx, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	out, err := s.hydrate(ctx, orgID, rows)
	return out, total, err
}

// Facets returns the dynamic filter options in the caller's branch scope.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.repo.Facets(ctx, orgID, orgctx.BranchScope(ctx), model.Today(s.now()))
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{
		Status: []model.Facet{}, Ownership: []model.Facet{}, Pool: []model.Facet{}, IsActive: []model.Facet{},
		DocumentStatus: []model.Facet{}, HomeBranch: []model.Facet{}, CurrentBranch: []model.Facet{}, RentalGroup: []model.Facet{}, Segment: []model.Facet{},
	}
	for _, row := range rows {
		f := model.Facet{Value: row.Value, Label: row.Label, Count: row.Count}
		switch row.Facet {
		case "status":
			out.Status = append(out.Status, f)
		case "ownership":
			out.Ownership = append(out.Ownership, f)
		case "pool":
			out.Pool = append(out.Pool, f)
		case "is_active":
			out.IsActive = append(out.IsActive, f)
		case "document_status":
			out.DocumentStatus = append(out.DocumentStatus, f)
		case "home_branch":
			out.HomeBranch = append(out.HomeBranch, f)
		case "current_branch":
			out.CurrentBranch = append(out.CurrentBranch, f)
		case "rental_group":
			out.RentalGroup = append(out.RentalGroup, f)
		case "segment":
			out.Segment = append(out.Segment, f)
		}
	}
	return out, nil
}

// Get returns one vehicle of the organization.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Vehicle, error) {
	row, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return model.Vehicle{}, err
	}
	out, err := s.hydrate(ctx, orgID, []repository.VehicleRow{row})
	if err != nil {
		return model.Vehicle{}, err
	}
	return out[0], nil
}

// Create validates and stores a new vehicle and records its initial status.
func (s *Service) Create(ctx context.Context, orgID int64, in model.VehicleInput) (model.Vehicle, error) {
	fields := repository.VehicleFields{
		FleetPool: model.DefaultPool, Ownership: model.OwnershipOwned, Status: model.StatusAvailable,
		AirConditioning: true, IsActive: true,
	}
	if err := s.applyInput(ctx, orgID, &fields, in, nil); err != nil {
		return model.Vehicle{}, err
	}
	if fields.IsActive {
		if taken, err := s.repo.PlateTaken(ctx, orgID, fields.Plate, nil); err != nil {
			return model.Vehicle{}, err
		} else if taken {
			return model.Vehicle{}, ErrPlateTaken
		}
	}
	var created db.Vehicle
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		row, err := tx.CreateVehicle(ctx, orgID, fields)
		if err != nil {
			return err
		}
		created = row
		_, err = tx.InsertEvent(ctx, orgID, row.ID, repository.EventFields{
			EventType: "created", ToStatus: row.Status,
			OdometerKm: pgtype.Int4{Int32: row.OdometerKm, Valid: true}, FuelLevel: row.FuelLevel,
			ActorID: actorArg(ctx),
		})
		return err
	})
	if err != nil {
		return model.Vehicle{}, mapPlateTaken(err)
	}
	s.index(ctx, orgID, created.Uuid)
	return s.Get(ctx, orgID, created.Uuid)
}

// Patch updates the given fields of a vehicle. A different status is a
// recorded transition.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.VehicleInput) (model.Vehicle, error) {
	current, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return model.Vehicle{}, err
	}
	fields := fieldsFromVehicle(current.Vehicle)
	if err := s.applyInput(ctx, orgID, &fields, in, &current); err != nil {
		return model.Vehicle{}, err
	}
	reactivated := fields.IsActive && !current.Vehicle.IsActive
	if fields.IsActive && (reactivated || plateKey(fields.Plate) != plateKey(current.Vehicle.Plate)) {
		if taken, err := s.repo.PlateTaken(ctx, orgID, fields.Plate, &id); err != nil {
			return model.Vehicle{}, err
		} else if taken {
			return model.Vehicle{}, ErrPlateTaken
		}
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		row, err := tx.UpdateVehicle(ctx, orgID, id, fields)
		if err != nil {
			return err
		}
		if fields.Status != row.Status {
			return transition(ctx, tx, orgID, row, fields.Status, row.OdometerKm, row.FuelLevel, "")
		}
		return nil
	})
	if err != nil {
		return model.Vehicle{}, mapNotFound(mapPlateTaken(err), ErrNotFound)
	}
	s.index(ctx, orgID, id)
	return s.Get(ctx, orgID, id)
}

// StatusChange is the outcome of ChangeStatus.
type StatusChange struct {
	Vehicle    model.Vehicle
	FromStatus string
	Changed    bool
}

// ChangeStatus records an operational status transition with the km / fuel
// reading and a note. The same status is a no-op (nothing is recorded) unless
// the km or fuel reading changed, which updates the vehicle only.
func (s *Service) ChangeStatus(ctx context.Context, orgID int64, id uuid.UUID, in model.StatusInput) (StatusChange, error) {
	current, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return StatusChange{}, err
	}
	v := current.Vehicle
	ve := &ValidationError{}
	status := strings.TrimSpace(in.Status)
	if !model.Contains(model.Statuses, status) {
		ve.add("status", "invalid", "status must be one of "+strings.Join(model.Statuses, ", "))
	}
	km := v.OdometerKm
	if in.OdometerKm != nil {
		if *in.OdometerKm < 0 || *in.OdometerKm > 5000000 {
			ve.add("odometer_km", "out_of_range", "odometer_km must be between 0 and 5000000")
		} else {
			km = *in.OdometerKm
		}
	}
	fuel := v.FuelLevel
	applyInt2(ve, "fuel_level", in.FuelLevel, 0, 100, &fuel)
	note := strings.TrimSpace(in.Note)
	if len([]rune(note)) > 2000 {
		ve.add("note", "too_long", "note must be at most 2000 characters")
	}
	if err := ve.orNil(); err != nil {
		return StatusChange{}, err
	}
	out := StatusChange{FromStatus: v.Status, Changed: status != v.Status}
	if !out.Changed && km == v.OdometerKm && fuel == v.FuelLevel {
		out.Vehicle, err = s.Get(ctx, orgID, id)
		return out, err
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if !out.Changed {
			_, err := tx.SetStatus(ctx, orgID, id, v.Status, km, fuel)
			return err
		}
		return transition(ctx, tx, orgID, v, status, km, fuel, note)
	})
	if err != nil {
		return StatusChange{}, mapNotFound(err, ErrNotFound)
	}
	if out.Changed {
		s.index(ctx, orgID, id)
	}
	out.Vehicle, err = s.Get(ctx, orgID, id)
	return out, err
}

// transition updates the status and writes the event (inside a transaction).
func transition(ctx context.Context, tx *repository.Postgres, orgID int64, v db.Vehicle, status string, km int32, fuel pgtype.Int2, note string) error {
	row, err := tx.SetStatus(ctx, orgID, v.Uuid, status, km, fuel)
	if err != nil {
		return err
	}
	_, err = tx.InsertEvent(ctx, orgID, row.ID, repository.EventFields{
		EventType: "status", FromStatus: pgtype.Text{String: v.Status, Valid: true}, ToStatus: status,
		OdometerKm: pgtype.Int4{Int32: km, Valid: true}, FuelLevel: fuel, Note: note, ActorID: actorArg(ctx),
	})
	return err
}

// SetStatus is the bulk form of ChangeStatus: it returns the previous
// status (for rollback) and records the transition.
func (s *Service) SetStatus(ctx context.Context, orgID int64, id uuid.UUID, status, note string) (string, error) {
	res, err := s.ChangeStatus(ctx, orgID, id, model.StatusInput{Status: status, Note: note})
	if err != nil {
		return "", err
	}
	return res.FromStatus, nil
}

// MoveCurrentBranch sets the physical branch of a vehicle and returns the
// previous one (bulk engine). Any branch of the organization is allowed.
func (s *Service) MoveCurrentBranch(ctx context.Context, orgID int64, id, branchUUID uuid.UUID) (uuid.UUID, error) {
	current, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return uuid.Nil, err
	}
	branch, err := s.repo.GetBranch(ctx, orgID, branchUUID)
	if err != nil {
		return uuid.Nil, mapNotFound(err, validation("current_branch_uuid", "not_found", "branch was not found"))
	}
	if branch.ID != current.Vehicle.CurrentBranchID {
		if _, err := s.repo.SetCurrentBranch(ctx, orgID, id, branch.ID); err != nil {
			return uuid.Nil, mapNotFound(err, ErrNotFound)
		}
		s.index(ctx, orgID, id)
	}
	return current.CurrentBranchUuid, nil
}

// SetActive switches is_active and returns the previous value (bulk engine).
// Activating a vehicle whose plate is used by another active vehicle fails
// with ErrPlateTaken.
func (s *Service) SetActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	current, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return false, err
	}
	prev := current.Vehicle.IsActive
	if prev == active {
		return prev, nil
	}
	if active {
		if taken, err := s.repo.PlateTaken(ctx, orgID, current.Vehicle.Plate, &id); err != nil {
			return prev, err
		} else if taken {
			return prev, ErrPlateTaken
		}
	}
	if _, err := s.repo.SetActive(ctx, orgID, id, active); err != nil {
		return prev, mapNotFound(mapPlateTaken(err), ErrNotFound)
	}
	s.index(ctx, orgID, id)
	return prev, nil
}

// Delete soft-deletes a vehicle (its history stays).
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	if _, err := s.getVehicle(ctx, orgID, id); err != nil {
		return err
	}
	if err := s.repo.DeleteVehicle(ctx, orgID, id); err != nil {
		return mapNotFound(err, ErrNotFound)
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return nil
}

// TargetUUIDs resolves the vehicles matching a list filter (bulk "all
// matching" scope) in the caller's branch scope, capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	f.BranchIDs = orgctx.BranchScope(ctx)
	f.Today = model.Today(s.now())
	return s.repo.ListVehicleUUIDs(ctx, orgID, f, limit)
}

// ExportRows returns the vehicles matching a list filter (export engine),
// plate order, capped at MaxListTargets.
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Vehicle, error) {
	f.Limit, f.Offset, f.SortField, f.SortDesc = MaxListTargets, 0, "plate", false
	f.BranchIDs = orgctx.BranchScope(ctx)
	f.Today = model.Today(s.now())
	rows, _, err := s.repo.ListVehicles(ctx, orgID, f)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, orgID, rows)
}

// Events lists the status history of a vehicle (newest first).
func (s *Service) Events(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.Event, int64, error) {
	v, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListEvents(ctx, orgID, v.Vehicle.ID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Event, 0, len(rows))
	for _, row := range rows {
		e := model.Event{
			UUID: row.Uuid, EventType: row.EventType, FromStatus: textPtr(row.FromStatus), ToStatus: row.ToStatus,
			OdometerKm: int4Ptr(row.OdometerKm), FuelLevel: int2Ptr(row.FuelLevel), Note: row.Note,
			CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if row.ActorUuid.Valid {
			e.Actor = &model.Actor{
				UUID: row.ActorUuid.Bytes, Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// Kasko returns the TSB valuation history of the vehicle's catalog type and
// model year (newest period first). Manual entries have none.
func (s *Service) Kasko(ctx context.Context, orgID int64, id uuid.UUID) (model.KaskoView, error) {
	row, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return model.KaskoView{}, err
	}
	out := model.KaskoView{ModelYear: row.Vehicle.ModelYear, Catalog: catalogRef(row), Items: []model.Valuation{}}
	if !row.Vehicle.CatalogTypeID.Valid {
		return out, nil
	}
	vals, err := s.repo.Valuations(ctx, row.Vehicle.CatalogTypeID.Int64, row.Vehicle.ModelYear)
	if err != nil {
		return model.KaskoView{}, err
	}
	for _, v := range vals {
		out.Items = append(out.Items, model.Valuation{Period: v.Period, Value: FormatMoney(v.Value)})
	}
	return out, nil
}

// Activity lists the audit events of a vehicle.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getVehicle(ctx, orgID, id); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListActivity(ctx, orgID, id, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		entry := model.ActivityEntry{
			UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload),
			CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if len(entry.Payload) == 0 {
			entry.Payload = json.RawMessage("{}")
		}
		if row.ActorUuid.Valid {
			entry.Actor = &model.Actor{
				UUID: row.ActorUuid.Bytes, Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, entry)
	}
	return out, total, nil
}

// BranchExists reports whether uuid is a live branch of the organization
// (bulk param validation).
func (s *Service) BranchExists(ctx context.Context, orgID int64, id uuid.UUID) (bool, error) {
	_, err := s.repo.GetBranch(ctx, orgID, id)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// GroupExists reports whether uuid is a live rental group of the
// organization (bulk param validation).
func (s *Service) GroupExists(ctx context.Context, orgID int64, id uuid.UUID) (bool, error) {
	_, err := s.repo.GetGroup(ctx, orgID, id)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Lookup loads a live vehicle of the organization in the caller's branch
// scope (home or current branch). Vehicle sub-resources (documents,
// maintenance) use it so they answer exactly like the vehicle: another
// organization's uuid or a vehicle outside the scope is ErrNotFound (404).
func (s *Service) Lookup(ctx context.Context, orgID int64, id uuid.UUID) (db.Vehicle, error) {
	row, err := s.getVehicle(ctx, orgID, id)
	if err != nil {
		return db.Vehicle{}, err
	}
	return row.Vehicle, nil
}

// getVehicle loads a vehicle of the organization and applies the caller's
// branch scope: a vehicle whose home and current branch are both outside it
// reads as not found (404), like another organization's uuid.
func (s *Service) getVehicle(ctx context.Context, orgID int64, id uuid.UUID) (repository.VehicleRow, error) {
	row, err := s.repo.GetVehicle(ctx, orgID, id)
	if err != nil {
		return repository.VehicleRow{}, mapNotFound(err, ErrNotFound)
	}
	if !inScope(ctx, row.Vehicle) {
		return repository.VehicleRow{}, ErrNotFound
	}
	return row, nil
}

// inScope reports whether the caller's branch scope covers the vehicle's
// home or current branch.
func inScope(ctx context.Context, v db.Vehicle) bool {
	return orgctx.CheckBranch(ctx, v.HomeBranchID) == nil || orgctx.CheckBranch(ctx, v.CurrentBranchID) == nil
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// actorArg is the signed-in user of a request context (null in workers).
func actorArg(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func mapPlateTaken(err error) error {
	if errors.Is(err, repository.ErrPlateTaken) {
		return ErrPlateTaken
	}
	return err
}
