package usecase

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// AffectedSummary is what the re-plan of affected allocations did.
type AffectedSummary struct {
	Replanned []uuid.UUID
	Tasks     []uuid.UUID
}

// AllocationsAffected is the hook of operational changes over sold
// capacity (TEC-698): a block over an allocation (fleetops reports
// affected[]), M3 late returns, vehicle changes. For each allocation that
// belongs to a reservation the engine re-plans it automatically (another
// eligible vehicle, moving flexible commitments); a successful re-plan is
// recorded on the reservation, a failed one opens a resolution task. The
// reservation is never cancelled automatically.
func (s *Service) AllocationsAffected(ctx context.Context, orgID int64, sourceType string, source uuid.UUID, allocations []uuid.UUID) AffectedSummary {
	out := AffectedSummary{}
	sys := unscoped(ctx)
	for _, alloc := range allocations {
		res, err := s.q.GetReservationByAllocationUUID(sys, db.GetReservationByAllocationUUIDParams{OrganizationID: orgID, AllocationUuid: alloc})
		if err != nil || !model.HoldsCapacity(res.Status) {
			continue // a plain hold, or not a live reservation
		}
		if s.replanReservation(sys, orgID, res.ID, res.Uuid, alloc, sourceType, source) {
			out.Replanned = append(out.Replanned, res.Uuid)
			continue
		}
		err = s.tx(sys, func(q *db.Queries, _ pgxTx) error {
			return s.openTask(sys, q, orgID, res.ID, model.TaskBlockConflict,
				"an operational change ("+sourceType+") broke the vehicle plan and no other vehicle can take it", sourceType, source,
				map[string]any{"allocation_uuid": alloc})
		})
		if err == nil {
			out.Tasks = append(out.Tasks, res.Uuid)
		}
	}
	s.publish(ctx, model.ResourceReservations, "updated", append(append([]uuid.UUID{}, out.Replanned...), out.Tasks...)...)
	if len(out.Tasks) > 0 {
		s.publish(ctx, model.ResourceTasks, "created", out.Tasks...)
	}
	return out
}

// replanReservation asks the engine to re-plan a reservation's allocation
// as it is; success is recorded (event + assignment history).
func (s *Service) replanReservation(ctx context.Context, orgID, resID int64, resUUID, alloc uuid.UUID, sourceType string, source uuid.UUID) bool {
	before, _ := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: resUUID})
	_, err := s.avail.Replan(ctx, orgID, alloc, hookOf(func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
		from, _ := q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: before.AllocationVehicleID.Int64})
		to, _ := q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: r.VehicleID})
		if err := event(ctx, q, orgID, resID, EventReplanned, "", "", "", map[string]any{
			"cause": sourceType, "cause_uuid": source, "from_plate": from.Plate, "to_plate": to.Plate, "reassigned": len(r.Moved),
		}); err != nil {
			return err
		}
		if r.VehicleID != before.AllocationVehicleID.Int64 {
			if err := assignment(ctx, q, orgID, resID, r.VehicleID, r.ProductID, r.ProductVersion, r.PlateFixed, model.SourceReplan, sourceType); err != nil {
				return err
			}
		}
		_, err := s.planEffects(ctx, q, orgID, r, sourceType, source)
		return err
	}, s))
	return err == nil
}

// --------------------------------------------------------------- tasks

func (s *Service) taskRow(ctx context.Context, orgID int64, id uuid.UUID) (db.GetResolutionTaskRow, error) {
	row, err := s.q.GetResolutionTask(ctx, db.GetResolutionTaskParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, ErrTaskNotFound
	}
	if err != nil {
		return row, err
	}
	if !inScope(ctx, row.PickupBranchID, row.ReturnBranchID) {
		return row, ErrTaskNotFound
	}
	return row, nil
}

func taskView(row db.GetResolutionTaskRow) model.Task {
	t := model.Task{
		UUID: row.Uuid, TaskType: row.TaskType, Status: row.Status, Reason: row.Reason, SourceType: row.SourceType,
		SourceUUID: uuidPtr(row.SourceUuid), Data: json.RawMessage(row.Data), ResolvedAt: timePtr(row.ResolvedAt),
		ResolutionNote: row.ResolutionNote, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		Reservation: model.TaskReservation{
			UUID: row.ReservationUuid, Reference: row.ReservationReference, Status: row.ReservationStatus,
			StartsAt: row.ReservationStartsAt.Time.UTC(), EndsAt: row.ReservationEndsAt.Time.UTC(),
		},
		Assignee:   actorOf(row.AssigneeUuid, row.AssigneeName, row.AssigneeSurname, row.AssigneeEmail),
		ResolvedBy: actorOf(row.ResolvedByUuid, row.ResolvedByName, row.ResolvedBySurname, row.ResolvedByEmail),
	}
	return t
}

// GetTask returns one resolution task in the caller's branch scope.
func (s *Service) GetTask(ctx context.Context, orgID int64, id uuid.UUID) (model.Task, error) {
	row, err := s.taskRow(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	return taskView(row), nil
}

// ListTasks returns a page of resolution tasks in the caller's branch scope.
func (s *Service) ListTasks(ctx context.Context, orgID int64, f model.TaskFilter) ([]model.Task, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if _, ok := model.TaskSortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	rows, err := s.q.ListResolutionTasks(ctx, db.ListResolutionTasksParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses), TaskTypes: nilIfEmpty(f.TaskTypes),
		ReservationUuids: nilUUIDs(f.ReservationUUIDs), AssigneeUuids: nilUUIDs(f.AssigneeUUIDs), Q: qArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Task, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		t := model.Task{
			UUID: r.Uuid, TaskType: r.TaskType, Status: r.Status, Reason: r.Reason, SourceType: r.SourceType,
			SourceUUID: uuidPtr(r.SourceUuid), Data: json.RawMessage(r.Data), ResolvedAt: timePtr(r.ResolvedAt),
			ResolutionNote: r.ResolutionNote, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
			Reservation: model.TaskReservation{
				UUID: r.ReservationUuid, Reference: r.ReservationReference, Status: r.ReservationStatus,
				StartsAt: r.ReservationStartsAt.Time.UTC(), EndsAt: r.ReservationEndsAt.Time.UTC(),
				PickupBranch: &model.BranchRef{UUID: r.PickupBranchUuid, Code: r.PickupBranchCode, Name: r.PickupBranchName},
			},
			Assignee: actorOf(r.AssigneeUuid, r.AssigneeName, r.AssigneeSurname, r.AssigneeEmail),
		}
		out = append(out, t)
	}
	return out, total, nil
}

// ResolveTask closes an open task with a note.
func (s *Service) ResolveTask(ctx context.Context, orgID int64, id uuid.UUID, note string) (model.Task, error) {
	row, err := s.taskRow(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	if row.Status != model.TaskOpen {
		return model.Task{}, stateErr("task_resolved", "the task is already resolved")
	}
	note = strings.TrimSpace(note)
	if len(note) > 1000 {
		return model.Task{}, validation("note", "too_long", "note is at most 1000 characters")
	}
	err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		n, err := q.ResolveResolutionTask(ctx, db.ResolveResolutionTaskParams{
			OrganizationID: orgID, ID: row.ID, ResolvedAt: ts(s.clock()), ResolvedBy: int8Ptr(actorID(ctx)), ResolutionNote: note,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("task_resolved", "the task is already resolved")
		}
		return event(ctx, q, orgID, row.ReservationID, "task_resolved", "", "", note, map[string]any{"task_uuid": row.Uuid, "task_type": row.TaskType})
	})
	if err != nil {
		return model.Task{}, err
	}
	s.publish(ctx, model.ResourceTasks, "updated", id)
	s.publish(ctx, model.ResourceReservations, "updated", row.ReservationUuid)
	return s.GetTask(ctx, orgID, id)
}

// AssignTask sets (or clears) the assignee of a task.
func (s *Service) AssignTask(ctx context.Context, orgID int64, id uuid.UUID, userUUID *uuid.UUID) (model.Task, error) {
	row, err := s.taskRow(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	var userID *int64
	if userUUID != nil {
		u, err := s.q.GetReservationUserInOrganization(ctx, db.GetReservationUserInOrganizationParams{OrganizationID: orgID, Uuid: *userUUID})
		if notFound(err) {
			return model.Task{}, validation("user_uuid", "not_found", "member was not found")
		}
		if err != nil {
			return model.Task{}, err
		}
		userID = &u.ID
	}
	if _, err := s.q.AssignResolutionTask(ctx, db.AssignResolutionTaskParams{OrganizationID: orgID, ID: row.ID, AssigneeID: int8Ptr(userID)}); err != nil {
		return model.Task{}, err
	}
	s.publish(ctx, model.ResourceTasks, "updated", id)
	return s.GetTask(ctx, orgID, id)
}

// ReplanTask retries the engine re-plan of the task's reservation; on
// success the task is resolved automatically.
func (s *Service) ReplanTask(ctx context.Context, orgID int64, id uuid.UUID) (model.TaskReplanResult, error) {
	row, err := s.taskRow(ctx, orgID, id)
	if err != nil {
		return model.TaskReplanResult{}, err
	}
	if row.Status != model.TaskOpen {
		return model.TaskReplanResult{}, stateErr("task_resolved", "the task is already resolved")
	}
	c, err := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: row.ReservationUuid})
	if err != nil {
		return model.TaskReplanResult{}, err
	}
	out := model.TaskReplanResult{}
	if allocationActive(c) && s.replanReservation(unscoped(ctx), orgID, c.ID, c.Uuid, c.AllocationUuid.Bytes, "task", row.Uuid) {
		out.Replanned = true
		_ = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
			_, err := q.ResolveResolutionTask(ctx, db.ResolveResolutionTaskParams{
				OrganizationID: orgID, ID: row.ID, ResolvedAt: ts(s.clock()), ResolvedBy: int8Ptr(actorID(ctx)), ResolutionNote: "replanned",
			})
			return err
		})
		if after, err := s.q.GetReservationCore(ctx, db.GetReservationCoreParams{OrganizationID: orgID, Uuid: c.Uuid}); err == nil {
			if v, err := s.q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: after.AllocationVehicleID.Int64}); err == nil {
				out.Vehicle = &model.VehicleRef{UUID: v.Uuid, Plate: v.Plate}
			}
		}
		s.publish(ctx, model.ResourceReservations, "updated", c.Uuid)
	}
	s.publish(ctx, model.ResourceTasks, "updated", id)
	t, err := s.GetTask(ctx, orgID, id)
	out.Task = t
	return out, err
}
