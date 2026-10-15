package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/placement"
	fleetopsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Default planned durations (minutes) per type.
var defaultMinutes = map[string]int32{
	model.TypeHandover: 30, model.TypeReturn: 30, model.TypeTransfer: 60, model.TypePreparation: 45,
	model.TypeCleaning: 45, model.TypeMaintenance: 60, model.TypeDeposit: 15, model.TypeOther: 30,
}

func minutesOf(typ string, in *int32, ve *ValidationError) int32 {
	if in == nil {
		if m, ok := defaultMinutes[typ]; ok {
			return m
		}
		return 30
	}
	if *in < 5 || *in > 1440 {
		ve.add("planned_minutes", "out_of_range", "planned_minutes is 5..1440")
	}
	return *in
}

func text(ve *ValidationError, field string, v *string, maxLen int, current string) string {
	if v == nil {
		return current
	}
	s := strings.TrimSpace(*v)
	if len([]rune(s)) > maxLen {
		ve.add(field, "too_long", field+" is too long")
	}
	return s
}

// resolvePlace applies a place input onto a place (the task's branch is
// the operating branch: a point must belong to it).
func (s *Service) resolvePlace(ctx context.Context, orgID, branchID int64, in model.PlaceInput, clearPoint bool, p placement.Place, ve *ValidationError) placement.Place {
	if clearPoint {
		p.PointID = pgtype.Int8{}
	}
	if in.PointUUID != nil {
		pt, err := s.q.GetFieldPoint(ctx, db.GetFieldPointParams{OrganizationID: orgID, Uuid: *in.PointUUID})
		switch {
		case err != nil:
			ve.add("point_uuid", "not_found", "service point was not found")
		case pt.BranchID != branchID:
			ve.add("point_uuid", "branch_mismatch", "the service point belongs to another branch")
		case !pt.IsActive:
			ve.add("point_uuid", "inactive", "service point is not active")
		default:
			p.PointID = pgtype.Int8{Int64: pt.ID, Valid: true}
		}
	}
	p.Address = text(ve, "address", in.Address, 500, p.Address)
	p.District = text(ve, "district", in.District, 128, p.District)
	p.City = text(ve, "city", in.City, 128, p.City)
	p.ContactName = text(ve, "contact_name", in.ContactName, 255, p.ContactName)
	p.ContactPhone = text(ve, "contact_phone", in.ContactPhone, 32, p.ContactPhone)
	if in.FlightNumber != nil {
		f := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(*in.FlightNumber), " ", ""))
		p.FlightNumber = text(ve, "flight_number", &f, 16, p.FlightNumber)
	}
	p.MeetingNote = text(ve, "meeting_note", in.MeetingNote, 1000, p.MeetingNote)
	if in.Latitude != nil || in.Longitude != nil {
		if in.Latitude == nil || in.Longitude == nil {
			ve.add("latitude", "incomplete", "give both latitude and longitude")
		} else if *in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180 {
			ve.add("latitude", "out_of_range", "coordinates are out of range")
		} else {
			p.Latitude, p.Longitude = float8(in.Latitude), float8(in.Longitude)
		}
	}
	switch {
	case p.Address != "":
		p.Kind = model.LocationAddress
	case p.PointID.Valid:
		p.Kind = model.LocationPoint
	default:
		p.Kind = model.LocationBranch
		p.Latitude, p.Longitude = pgtype.Float8{}, pgtype.Float8{}
	}
	return p
}

func placeOfRow(row db.GetFieldTaskRow, pointID pgtype.Int8) placement.Place {
	return placement.Place{
		Kind: row.LocationKind, PointID: pointID, Address: row.Address, District: row.District, City: row.City,
		Latitude: row.Latitude, Longitude: row.Longitude, ContactName: row.ContactName, ContactPhone: row.ContactPhone,
		FlightNumber: row.FlightNumber, MeetingNote: row.MeetingNote,
	}
}

// member checks a would-be assignee: a member whose roles work field tasks
// and whose branch scope covers the task.
func (s *Service) member(ctx context.Context, orgID int64, user uuid.UUID, branchIDs ...int64) (db.GetFieldMemberRow, error) {
	m, err := s.q.GetFieldMember(ctx, db.GetFieldMemberParams{OrganizationID: orgID, Uuid: user})
	if notFound(err) {
		return m, validation("user_uuid", "not_found", "member was not found")
	}
	if err != nil {
		return m, err
	}
	if !m.CanWork {
		return m, validation("user_uuid", "not_field_staff", "the member's roles do not work field tasks")
	}
	if m.BranchScope == "selected" {
		ok := false
		for _, b := range branchIDs {
			for _, id := range m.BranchIds {
				if b != 0 && b == id {
					ok = true
				}
			}
		}
		if !ok {
			return m, validation("user_uuid", "branch_not_allowed", "the member does not work at this branch")
		}
	}
	return m, nil
}

// Create opens a task by hand (transfer, preparation, cleaning,
// maintenance, other). A transfer plans the fleet transfer that moves the
// vehicle when the task completes.
func (s *Service) Create(ctx context.Context, orgID int64, in model.CreateInput) (model.Task, error) {
	ve := &ValidationError{}
	typ := strings.TrimSpace(in.TaskType)
	if !model.Contains(model.ManualTypes, typ) {
		ve.add("task_type", "invalid", "task_type must be one of "+strings.Join(model.ManualTypes, ", "))
	}
	var branch db.GetFieldBranchRow
	if in.BranchUUID == nil {
		ve.add("branch_uuid", "required", "branch_uuid is required")
	} else {
		b, err := s.q.GetFieldBranch(ctx, db.GetFieldBranchParams{OrganizationID: orgID, Uuid: *in.BranchUUID})
		if err != nil || !inScope(ctx, b.ID) {
			ve.add("branch_uuid", "not_found", "branch was not found")
		} else {
			branch = b
		}
	}
	if in.DueAt == nil {
		ve.add("due_at", "required", "due_at is required")
	}
	minutes := minutesOf(typ, in.PlannedMinutes, ve)
	note := text(ve, "note", &in.Note, 1000, "")
	var vehicle *db.GetFieldVehicleRow
	if in.VehicleUUID != nil {
		v, err := s.q.GetFieldVehicle(ctx, db.GetFieldVehicleParams{OrganizationID: orgID, Uuid: *in.VehicleUUID})
		if err != nil || !inScope(ctx, v.HomeBranchID, v.CurrentBranchID) {
			ve.add("vehicle_uuid", "not_found", "vehicle was not found")
		} else {
			vehicle = &v
		}
	}
	var toBranch *db.GetFieldBranchRow
	if typ == model.TypeTransfer {
		if in.VehicleUUID == nil {
			ve.add("vehicle_uuid", "required", "a transfer needs a vehicle")
		}
		if in.ToBranchUUID == nil {
			ve.add("to_branch_uuid", "required", "a transfer needs a destination")
		} else if b, err := s.q.GetFieldBranch(ctx, db.GetFieldBranchParams{OrganizationID: orgID, Uuid: *in.ToBranchUUID}); err != nil {
			ve.add("to_branch_uuid", "not_found", "branch was not found")
		} else if b.ID == branch.ID {
			ve.add("to_branch_uuid", "same_branch", "the destination is the origin")
		} else {
			toBranch = &b
		}
	} else if in.ToBranchUUID != nil {
		ve.add("to_branch_uuid", "not_allowed", "only a transfer has a destination")
	}
	place := s.resolvePlace(ctx, orgID, branch.ID, in.PlaceInput, false, placement.Place{}, ve)
	if in.AssigneeUUID != nil && branch.ID != 0 {
		if _, err := s.member(ctx, orgID, *in.AssigneeUUID, branch.ID); err != nil {
			var mve *ValidationError
			if !errors.As(err, &mve) {
				return model.Task{}, err
			}
			ve.add("assignee_uuid", mve.Details[0].Code, mve.Details[0].Message)
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Task{}, err
	}
	due := in.DueAt.UTC().Truncate(time.Microsecond)
	params := db.InsertFieldTaskParams{
		OrganizationID: orgID, TaskType: typ, Status: model.StatusPlanned, Origin: model.OriginManual, BranchID: branch.ID,
		DueAt: ts(due), PlannedMinutes: minutes, Note: note, LocationKind: place.Kind, PointID: place.PointID,
		Address: place.Address, District: place.District, City: place.City, Latitude: place.Latitude, Longitude: place.Longitude,
		ContactName: place.ContactName, ContactPhone: place.ContactPhone, FlightNumber: place.FlightNumber,
		MeetingNote: place.MeetingNote, CreatedBy: actorArg(ctx),
	}
	if vehicle != nil {
		params.VehicleID = pgtype.Int8{Int64: vehicle.ID, Valid: true}
	}
	var transfer uuid.UUID
	if typ == model.TypeTransfer {
		params.ToBranchID = pgtype.Int8{Int64: toBranch.ID, Valid: true}
		if s.transfers == nil {
			return model.Task{}, stateErr("transfers_unavailable", "fleet transfers are not available")
		}
		arrival := due.Add(time.Duration(minutes) * time.Minute)
		from, to, v := branch.Uuid, toBranch.Uuid, vehicle.Uuid
		tr, err := s.transfers.CreateTransfer(ctx, orgID, fleetopsmodel.TransferInput{
			VehicleUUID: &v, FromBranchUUID: &from, ToBranchUUID: &to, PlannedDeparture: &due, PlannedArrival: &arrival, Note: &note,
		})
		if err != nil {
			return model.Task{}, mapTransferErr(err)
		}
		transfer = tr.UUID
		id, err := s.q.GetFieldTransferIDByUUID(ctx, db.GetFieldTransferIDByUUIDParams{OrganizationID: orgID, Uuid: tr.UUID})
		if err != nil {
			return model.Task{}, err
		}
		params.TransferID = pgtype.Int8{Int64: id, Valid: true}
	}
	row, err := s.q.InsertFieldTask(ctx, params)
	if err != nil {
		if transfer != uuid.Nil {
			_, _ = s.transfers.CancelTransfer(ctx, orgID, transfer, fleetopsmodel.TransferActionInput{Note: "field task not created"})
		}
		return model.Task{}, err
	}
	s.publish(ctx, "created", row.Uuid)
	if in.AssigneeUUID != nil {
		return s.Assign(ctx, orgID, row.Uuid, in.AssigneeUUID)
	}
	return s.Get(ctx, orgID, row.Uuid)
}

// Patch re-plans a live task: time, duration, note, place, contact and
// meeting details. A planned transfer moves its fleet transfer window.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.PatchInput) (model.Task, error) {
	if !has(ctx, permAssign) {
		return model.Task{}, ErrForbidden
	}
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	if !model.IsLive(row.Status) {
		return model.Task{}, stateErr("task_closed", "the task is closed")
	}
	ve := &ValidationError{}
	due := row.DueAt.Time.UTC()
	if in.DueAt != nil {
		due = in.DueAt.UTC().Truncate(time.Microsecond)
	}
	minutes := row.PlannedMinutes
	if in.PlannedMinutes != nil {
		minutes = minutesOf(row.TaskType, in.PlannedMinutes, ve)
	}
	note := text(ve, "note", in.Note, 1000, row.Note)
	pointID := pgtype.Int8{}
	if row.PointUuid.Valid {
		if pt, err := s.q.GetFieldPoint(ctx, db.GetFieldPointParams{OrganizationID: orgID, Uuid: row.PointUuid.Bytes}); err == nil {
			pointID = pgtype.Int8{Int64: pt.ID, Valid: true}
		}
	}
	place := s.resolvePlace(ctx, orgID, row.BranchID, in.PlaceInput, in.ClearPoint, placeOfRow(row, pointID), ve)
	if in.Address != nil && strings.TrimSpace(*in.Address) == "" {
		place.Address, place.District, place.City = "", "", ""
		if place.PointID.Valid {
			place.Kind = model.LocationPoint
		} else {
			place.Kind = model.LocationBranch
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Task{}, err
	}
	if row.TaskType == model.TypeTransfer && row.TransferUuid.Valid && row.TransferStatus.String == fleetopsmodel.TransferPlanned &&
		(!due.Equal(row.DueAt.Time.UTC()) || minutes != row.PlannedMinutes) && s.transfers != nil {
		arrival := due.Add(time.Duration(minutes) * time.Minute)
		if _, err := s.transfers.PatchTransfer(ctx, orgID, row.TransferUuid.Bytes, fleetopsmodel.TransferInput{
			PlannedDeparture: &due, PlannedArrival: &arrival,
		}); err != nil {
			return model.Task{}, mapTransferErr(err)
		}
	}
	n, err := s.q.UpdateFieldTaskPlan(ctx, db.UpdateFieldTaskPlanParams{
		OrganizationID: orgID, ID: row.ID, DueAt: ts(due), PlannedMinutes: minutes, Note: note, BranchID: row.BranchID,
		LocationKind: place.Kind, PointID: place.PointID, Address: place.Address, District: place.District, City: place.City,
		Latitude: place.Latitude, Longitude: place.Longitude, ContactName: place.ContactName, ContactPhone: place.ContactPhone,
		FlightNumber: place.FlightNumber, MeetingNote: place.MeetingNote, ActorID: actorArg(ctx),
	})
	if err != nil {
		return model.Task{}, err
	}
	if n != 1 {
		return model.Task{}, stateErr("task_closed", "the task is closed")
	}
	s.publish(ctx, "updated", id)
	return s.Get(ctx, orgID, id)
}

// Assign gives the task to a member (nil = unassign). A new assignee
// restarts the task as assigned and is notified.
func (s *Service) Assign(ctx context.Context, orgID int64, id uuid.UUID, user *uuid.UUID) (model.Task, error) {
	if !has(ctx, permAssign) {
		return model.Task{}, ErrForbidden
	}
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	action := model.ActionAssign
	if user == nil {
		action = model.ActionUnassign
	}
	if _, ok := model.Next(row.Status, action); !ok {
		if !model.IsLive(row.Status) {
			return model.Task{}, stateErr("task_closed", "the task is closed")
		}
		return model.Task{}, stateErr("invalid_transition", "the task cannot be "+action+"ed now")
	}
	var assignee pgtype.Int8
	if user != nil {
		m, err := s.member(ctx, orgID, *user, row.BranchID, row.ToBranchID.Int64)
		if err != nil {
			return model.Task{}, err
		}
		assignee = pgtype.Int8{Int64: m.ID, Valid: true}
	}
	n, err := s.q.AssignOperationTask(ctx, db.AssignOperationTaskParams{
		OrganizationID: orgID, ID: row.ID, AssigneeID: assignee, ActorID: actorArg(ctx),
	})
	if err != nil {
		return model.Task{}, err
	}
	if n != 1 {
		return model.Task{}, stateErr("task_closed", "the task is closed")
	}
	if assignee.Valid && (!row.AssigneeID.Valid || row.AssigneeID.Int64 != assignee.Int64) {
		s.notifyAssigned(ctx, orgID, id, assignee.Int64)
	}
	s.publish(ctx, "updated", id)
	return s.Get(ctx, orgID, id)
}

// Start sets off (en route); an issue reported task resumes. A transfer
// starts its fleet transfer (the vehicle is in transfer).
func (s *Service) Start(ctx context.Context, orgID int64, id uuid.UUID) (model.Task, error) {
	row, err := s.workable(ctx, orgID, id, model.ActionStart)
	if err != nil {
		return model.Task{}, err
	}
	if row.TaskType == model.TypeTransfer {
		if err := s.startTransfer(ctx, orgID, row); err != nil {
			return model.Task{}, err
		}
	}
	var n int64
	if row.Status == model.StatusIssueReported {
		n, err = s.q.TransitionFieldTask(ctx, db.TransitionFieldTaskParams{
			OrganizationID: orgID, ID: row.ID, FromStatus: row.Status, ToStatus: model.StatusEnRoute, At: ts(s.clock()), ActorID: actorArg(ctx),
		})
	} else {
		n, err = s.q.StartOperationTask(ctx, db.StartOperationTaskParams{
			OrganizationID: orgID, ID: row.ID, StartedAt: ts(s.clock()), ActorID: actorArg(ctx),
		})
	}
	if err != nil {
		return model.Task{}, err
	}
	if n != 1 {
		return model.Task{}, stateErr("concurrent", "the task was changed by someone else; reload")
	}
	s.publish(ctx, "updated", id)
	return s.Get(ctx, orgID, id)
}

// ReportIssue records a problem on the way (a note is required); the task
// stays open for the planner.
func (s *Service) ReportIssue(ctx context.Context, orgID int64, id uuid.UUID, note string) (model.Task, error) {
	row, err := s.workable(ctx, orgID, id, model.ActionReport)
	if err != nil {
		return model.Task{}, err
	}
	n, err := requireNote("note", note)
	if err != nil {
		return model.Task{}, err
	}
	return s.transition(ctx, orgID, row, model.StatusIssueReported, n)
}

// Complete finishes a task. Handover and return complete through the
// inspection form of the rental (record_handover / record_return); a
// transfer completes its fleet transfer (the vehicle's current branch
// becomes the destination); cleaning, maintenance and preparation make the
// vehicle available (rentals' rule).
func (s *Service) Complete(ctx context.Context, orgID int64, id uuid.UUID, note string) (model.Task, error) {
	row, err := s.workable(ctx, orgID, id, model.ActionComplete)
	if err != nil {
		return model.Task{}, err
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > 1000 {
		return model.Task{}, validation("note", "too_long", "note is at most 1000 characters")
	}
	switch row.TaskType {
	case model.TypeHandover:
		return model.Task{}, stateErr("record_handover", "complete the handover form of the rental to finish this task")
	case model.TypeReturn:
		return model.Task{}, stateErr("record_return", "complete the return form of the rental to finish this task")
	case model.TypeTransfer:
		if err := s.completeTransfer(ctx, orgID, row, note); err != nil {
			return model.Task{}, err
		}
		return s.transition(ctx, orgID, row, model.StatusCompleted, note)
	}
	if s.finisher == nil {
		return s.transition(ctx, orgID, row, model.StatusCompleted, note)
	}
	if err := s.finisher.FinishTask(ctx, orgID, id, true, note); err != nil {
		return model.Task{}, err
	}
	s.publish(ctx, "updated", id)
	return s.Get(ctx, orgID, id)
}

// Cancel cancels a task with a reason (planners only). A transfer cancels
// its fleet transfer.
func (s *Service) Cancel(ctx context.Context, orgID int64, id uuid.UUID, note string) (model.Task, error) {
	if !has(ctx, permAssign) {
		return model.Task{}, ErrForbidden
	}
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	if _, ok := model.Next(row.Status, model.ActionCancel); !ok {
		return model.Task{}, stateErr("task_closed", "the task is closed")
	}
	n, err := requireNote("note", note)
	if err != nil {
		return model.Task{}, err
	}
	if row.TaskType == model.TypeTransfer || s.finisher == nil {
		if row.TaskType == model.TypeTransfer && row.TransferUuid.Valid && s.transfers != nil &&
			(row.TransferStatus.String == fleetopsmodel.TransferPlanned || row.TransferStatus.String == fleetopsmodel.TransferInTransit) {
			if _, err := s.transfers.CancelTransfer(ctx, orgID, row.TransferUuid.Bytes, fleetopsmodel.TransferActionInput{Note: n}); err != nil {
				return model.Task{}, mapTransferErr(err)
			}
		}
		return s.transition(ctx, orgID, row, model.StatusCancelled, n)
	}
	if err := s.finisher.FinishTask(ctx, orgID, id, false, n); err != nil {
		return model.Task{}, err
	}
	s.publish(ctx, "updated", id)
	return s.Get(ctx, orgID, id)
}

// workable loads a task the caller may work and checks the move.
func (s *Service) workable(ctx context.Context, orgID int64, id uuid.UUID, action string) (db.GetFieldTaskRow, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return row, err
	}
	if !accessOf(ctx).works(row) {
		return row, ErrForbidden
	}
	if _, ok := model.Next(row.Status, action); !ok {
		if !model.IsLive(row.Status) {
			return row, stateErr("task_closed", "the task is closed")
		}
		return row, stateErr("invalid_transition", "this step is not possible from "+row.Status)
	}
	return row, nil
}

func (s *Service) transition(ctx context.Context, orgID int64, row db.GetFieldTaskRow, to, note string) (model.Task, error) {
	n, err := s.q.TransitionFieldTask(ctx, db.TransitionFieldTaskParams{
		OrganizationID: orgID, ID: row.ID, FromStatus: row.Status, ToStatus: to, At: ts(s.clock()), Note: note, ActorID: actorArg(ctx),
	})
	if err != nil {
		return model.Task{}, err
	}
	if n != 1 {
		return model.Task{}, stateErr("concurrent", "the task was changed by someone else; reload")
	}
	s.publish(ctx, "updated", row.Uuid)
	return s.Get(ctx, orgID, row.Uuid)
}

func (s *Service) startTransfer(ctx context.Context, orgID int64, row db.GetFieldTaskRow) error {
	if !row.TransferUuid.Valid || s.transfers == nil {
		return stateErr("transfer_missing", "the transfer of this task is missing")
	}
	if row.TransferStatus.String != fleetopsmodel.TransferPlanned {
		return nil
	}
	_, err := s.transfers.StartTransfer(unscoped(ctx), orgID, row.TransferUuid.Bytes, fleetopsmodel.TransferActionInput{})
	return mapTransferErr(err)
}

func (s *Service) completeTransfer(ctx context.Context, orgID int64, row db.GetFieldTaskRow, note string) error {
	if err := s.startTransfer(ctx, orgID, row); err != nil {
		return err
	}
	_, err := s.transfers.CompleteTransfer(unscoped(ctx), orgID, row.TransferUuid.Bytes, fleetopsmodel.TransferActionInput{
		ArrivalStatus: "available", Note: note,
	})
	return mapTransferErr(err)
}

// transferFields renames fleet transfer fields to the task's.
var transferFields = map[string]string{
	"planned_departure": "due_at", "planned_arrival": "planned_minutes", "from_branch_uuid": "branch_uuid",
}

func mapTransferErr(err error) error {
	if err == nil {
		return nil
	}
	var ve *fleetopsusecase.ValidationError
	if errors.As(err, &ve) {
		out := &ValidationError{}
		for _, d := range ve.Details {
			f := d.Field
			if m, ok := transferFields[f]; ok {
				f = m
			}
			out.add(f, d.Code, d.Message)
		}
		return out
	}
	if ce, ok := fleetopsusecase.IsConflict(err); ok {
		return stateErr(ce.Code, ce.Message)
	}
	switch {
	case errors.Is(err, fleetopsusecase.ErrVehicleNotFound):
		return validation("vehicle_uuid", "not_found", "vehicle was not found")
	case errors.Is(err, fleetopsusecase.ErrTransferNotFound):
		return stateErr("transfer_missing", "the transfer of this task is missing")
	}
	return err
}

// notifyAssigned tells the new assignee (in-app; the notification center
// mirrors it to the member's realtime channel and to web push).
func (s *Service) notifyAssigned(ctx context.Context, orgID int64, id uuid.UUID, userID int64) {
	if s.notifier == nil || userID == actorID(ctx) {
		return
	}
	row, err := s.q.GetFieldTask(ctx, db.GetFieldTaskParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return
	}
	reference := row.VehiclePlate.String
	if row.ReservationReference.Valid {
		reference = strings.TrimSpace(row.ReservationReference.String + " " + reference)
	}
	place := row.BranchName
	switch {
	case row.Address != "":
		place = row.Address
	case row.PointName.Valid:
		place = row.PointName.String
	}
	due := row.DueAt.Time.In(rentaltime.Location()).Format("02.01 15:04")
	org := orgID
	uid := userID
	var action *string
	if sc, ok := orgctx.From(ctx); ok && sc.Slug != "" {
		u := "/t/" + sc.Slug + "/field/my/" + id.String()
		action = &u
	}
	_, err = s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		OrganizationID: &org, UserID: &uid, Channels: []string{notifmodel.ChannelInapp}, Priority: notifmodel.PriorityHigh,
		Title: "Yeni saha görevi", Body: reference + " · " + due + " · " + place, TemplateCode: "field_tasks.assigned",
		TemplateVars: map[string]string{"reference": reference, "due": due, "place": place},
		SourceEvent:  "field_tasks.assigned", ActionURL: action,
		Payload: map[string]any{"task_uuid": id.String(), "task_type": row.TaskType, "resource": model.Resource},
	})
	if err != nil {
		s.log.Warn("field_task_notify_failed", "task", id, "error", err)
	}
}
