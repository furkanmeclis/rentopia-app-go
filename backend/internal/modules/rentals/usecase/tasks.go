package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func actorOf(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

func (s *Service) taskRow(ctx context.Context, orgID int64, id uuid.UUID) (db.GetOperationTaskRow, error) {
	row, err := s.q.GetOperationTask(ctx, db.GetOperationTaskParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, ErrTaskNotFound
	}
	if err != nil {
		return row, err
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return row, ErrTaskNotFound
	}
	return row, nil
}

func taskView(row db.GetOperationTaskRow) model.Task {
	t := model.Task{
		UUID: row.Uuid, TaskType: row.TaskType, Status: row.Status, DueAt: row.DueAt.Time.UTC(), Note: row.Note,
		Branch:    model.Ref{UUID: row.BranchUuid, Code: row.BranchCode, Name: row.BranchName},
		StartedAt: timePtr(row.StartedAt), CompletedAt: timePtr(row.CompletedAt), CompletionNote: row.CompletionNote,
		Assignee:    actorOf(row.AssigneeUuid, row.AssigneeName, row.AssigneeSurname, row.AssigneeEmail),
		CompletedBy: actorOf(row.CompletedByUuid, row.CompletedByName, row.CompletedBySurname, row.CompletedByEmail),
		CreatedAt:   row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.RentalUuid.Valid {
		t.Rental = &model.TaskRental{UUID: row.RentalUuid.Bytes, Reference: row.RentalReference.String, Status: row.RentalStatus.String}
	}
	if row.VehicleUuid.Valid {
		t.Vehicle = &model.VehicleRef{UUID: row.VehicleUuid.Bytes, Plate: row.VehiclePlate.String}
	}
	return t
}

// GetTask returns one operation task in the caller's branch scope.
func (s *Service) GetTask(ctx context.Context, orgID int64, id uuid.UUID) (model.Task, error) {
	row, err := s.taskRow(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	return taskView(row), nil
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilUUIDs(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}

func qArg(q string) pgtype.Text {
	q = strings.TrimSpace(q)
	return pgtype.Text{String: q, Valid: q != ""}
}

// ListTasks returns a page of operation tasks in the caller's branch scope.
func (s *Service) ListTasks(ctx context.Context, orgID int64, f model.TaskFilter) ([]model.Task, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if _, ok := model.TaskSortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "due_at", false
	}
	p := db.ListOperationTasksParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses),
		TaskTypes: nilIfEmpty(f.TaskTypes), BranchUuids: nilUUIDs(f.BranchUUIDs), RentalUuids: nilUUIDs(f.RentalUUIDs),
		VehicleUuids: nilUUIDs(f.VehicleUUIDs), AssigneeUuids: nilUUIDs(f.AssigneeUUIDs), Q: qArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if f.DueFrom != nil {
		p.DueFrom = ts(*f.DueFrom)
	}
	if f.DueTo != nil {
		p.DueTo = ts(*f.DueTo)
	}
	rows, err := s.q.ListOperationTasks(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Task, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, taskView(db.GetOperationTaskRow{
			Uuid: r.Uuid, TaskType: r.TaskType, Status: r.Status, DueAt: r.DueAt, Note: r.Note, StartedAt: r.StartedAt,
			CompletedAt: r.CompletedAt, CompletionNote: r.CompletionNote, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			BranchUuid: r.BranchUuid, BranchCode: r.BranchCode, BranchName: r.BranchName, RentalUuid: r.RentalUuid,
			RentalReference: r.RentalReference, RentalStatus: r.RentalStatus, VehicleUuid: r.VehicleUuid, VehiclePlate: r.VehiclePlate,
			AssigneeUuid: r.AssigneeUuid, AssigneeName: r.AssigneeName, AssigneeSurname: r.AssigneeSurname, AssigneeEmail: r.AssigneeEmail,
		}))
	}
	return out, total, nil
}

// TaskFacets returns the facet counts of the task list.
func (s *Service) TaskFacets(ctx context.Context, orgID int64) (model.TaskFacets, error) {
	rows, err := s.q.ListOperationTaskFacets(ctx, db.ListOperationTaskFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.TaskFacets{}, err
	}
	out := model.TaskFacets{Status: []model.FacetValue{}, TaskType: []model.FacetValue{}, Branch: []model.FacetValue{}}
	for _, r := range rows {
		v := model.FacetValue{Value: r.Value, Label: r.Label, Count: r.Count}
		switch r.Facet {
		case "status":
			out.Status = append(out.Status, v)
		case "task_type":
			out.TaskType = append(out.TaskType, v)
		case "branch":
			out.Branch = append(out.Branch, v)
		}
	}
	return out, nil
}

// AssignTask assigns a member of the organization (nil = unassign).
func (s *Service) AssignTask(ctx context.Context, orgID int64, id uuid.UUID, userUUID *uuid.UUID) (model.Task, error) {
	row, err := s.taskRow(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	var assignee pgtype.Int8
	if userUUID != nil {
		u, err := s.q.GetReservationUserInOrganization(ctx, db.GetReservationUserInOrganizationParams{OrganizationID: orgID, Uuid: *userUUID})
		if notFound(err) {
			return model.Task{}, validation("user_uuid", "not_found", "member was not found")
		}
		if err != nil {
			return model.Task{}, err
		}
		assignee = pgtype.Int8{Int64: u.ID, Valid: true}
	}
	n, err := s.q.AssignOperationTask(ctx, db.AssignOperationTaskParams{OrganizationID: orgID, ID: row.ID, AssigneeID: assignee})
	if err != nil {
		return model.Task{}, err
	}
	if n != 1 {
		return model.Task{}, stateErr("task_closed", "the task is closed")
	}
	return s.afterTask(ctx, orgID, id)
}

// StartTask moves an open task to in progress (assigning the caller when
// nobody is assigned).
func (s *Service) StartTask(ctx context.Context, orgID int64, id uuid.UUID) (model.Task, error) {
	row, err := s.taskRow(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	n, err := s.q.StartOperationTask(ctx, db.StartOperationTaskParams{OrganizationID: orgID, ID: row.ID, StartedAt: ts(s.clock()), ActorID: int8Ptr(actorID(ctx))})
	if err != nil {
		return model.Task{}, err
	}
	if n != 1 {
		return model.Task{}, stateErr("invalid_transition", "only an open task can be started")
	}
	return s.afterTask(ctx, orgID, id)
}

// FinishTask completes or cancels a task. Handover and return tasks are
// completed by the rental's handover / return (refused here while the
// rental still waits for them); completing a cleaning task makes the
// vehicle available again when it is still in cleaning.
func (s *Service) FinishTask(ctx context.Context, orgID int64, id uuid.UUID, done bool, note string) (model.Task, error) {
	row, err := s.taskRow(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	note = strings.TrimSpace(note)
	if len(note) > 1000 {
		return model.Task{}, validation("note", "too_long", "note is at most 1000 characters")
	}
	status := model.TaskDone
	if !done {
		status = model.TaskCancelled
		if _, err := requireReason("note", note); err != nil {
			return model.Task{}, err
		}
	}
	if done && row.RentalUuid.Valid {
		switch {
		case row.TaskType == model.TaskHandover && row.RentalStatus.String == model.StatusPreparation:
			return model.Task{}, stateErr("record_handover", "record the handover on the rental to complete this task")
		case row.TaskType == model.TaskReturn && row.RentalStatus.String == model.StatusActive:
			return model.Task{}, stateErr("record_return", "record the return on the rental to complete this task")
		}
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.FinishOperationTask(ctx, db.FinishOperationTaskParams{
			OrganizationID: orgID, ID: row.ID, Status: status, CompletedAt: ts(s.clock()), ActorID: int8Ptr(actorID(ctx)), CompletionNote: note,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("task_closed", "the task is already closed")
		}
		// Finishing the cleaning / maintenance / preparation of a vehicle makes
		// it available when it is still in that state.
		after := map[string]string{
			model.TaskCleaning: vehiclemodel.StatusCleaning, model.TaskMaintenance: vehiclemodel.StatusMaintenance,
			model.TaskPreparation: vehiclemodel.StatusPreparing,
		}[row.TaskType]
		if done && after != "" && row.VehicleUuid.Valid {
			v, err := q.GetRentalVehicleByUUID(ctx, db.GetRentalVehicleByUUIDParams{OrganizationID: orgID, Uuid: row.VehicleUuid.Bytes})
			if err == nil && v.Status == after {
				return s.vehicleChange(ctx, q, orgID, v.Uuid, vehicleusecase.OperationalChange{Status: vehiclemodel.StatusAvailable, Note: row.TaskType + " done"})
			}
		}
		return nil
	})
	if err != nil {
		return model.Task{}, err
	}
	return s.afterTask(ctx, orgID, id)
}

func (s *Service) afterTask(ctx context.Context, orgID int64, id uuid.UUID) (model.Task, error) {
	s.publish(ctx, model.ResourceOperationTasks, "updated", id)
	return s.GetTask(ctx, orgID, id)
}
