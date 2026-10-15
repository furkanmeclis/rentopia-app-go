package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
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

// Rental statuses the field flow reads.
const (
	rentalPreparation = "preparation"
	rentalActive      = "active"
)

// view turns a row into the API task with the caller's actions.
func view(row db.GetFieldTaskRow, a access) model.Task {
	due := row.DueAt.Time.UTC()
	t := model.Task{
		UUID: row.Uuid, TaskType: row.TaskType, Status: row.Status, Origin: row.Origin,
		Branch: model.Ref{UUID: row.BranchUuid, Code: row.BranchCode, Name: row.BranchName},
		Location: model.Location{
			Kind: row.LocationKind, Address: row.Address, District: row.District, City: row.City,
			Latitude: floatPtr(row.Latitude), Longitude: floatPtr(row.Longitude),
		},
		Contact:      model.Contact{Name: row.ContactName, Phone: row.ContactPhone},
		FlightNumber: row.FlightNumber, MeetingNote: row.MeetingNote, Note: row.Note,
		DueAt: due, PlannedMinutes: row.PlannedMinutes, PlannedEndAt: due.Add(time.Duration(row.PlannedMinutes) * time.Minute),
		StartedAt: timePtr(row.StartedAt), CompletedAt: timePtr(row.CompletedAt), AssignedAt: timePtr(row.AssignedAt),
		Assignee:       actorOf(row.AssigneeUuid, row.AssigneeName, row.AssigneeSurname, row.AssigneeEmail),
		CompletedBy:    actorOf(row.CompletedByUuid, row.CompletedByName, row.CompletedBySurname, row.CompletedByEmail),
		CompletionNote: row.CompletionNote, IssueNote: row.IssueNote, IssueAt: timePtr(row.IssueReportedAt),
		Actions: []string{}, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	// Contact: the delivery details, else the customer (not kept on the task,
	// so an anonymized customer disappears here too).
	if t.Contact.Name == "" {
		t.Contact.Name = row.CustomerName
	}
	if t.Contact.Phone == "" {
		t.Contact.Phone = row.CustomerPhone
	}
	if row.ToBranchUuid.Valid {
		t.ToBranch = &model.Ref{UUID: row.ToBranchUuid.Bytes, Code: row.ToBranchCode.String, Name: row.ToBranchName.String}
	}
	if row.PointUuid.Valid {
		t.Location.Point = &model.PointRef{UUID: row.PointUuid.Bytes, Name: row.PointName.String, Type: row.PointType.String}
	}
	t.Delivery = (row.TaskType == model.TypeHandover || row.TaskType == model.TypeReturn) &&
		(row.LocationKind == model.LocationAddress || (row.LocationKind == model.LocationPoint && row.PointType.String != "office"))
	if row.RentalUuid.Valid {
		t.Rental = &model.RecordRef{UUID: row.RentalUuid.Bytes, Reference: row.RentalReference.String, Status: row.RentalStatus.String}
	}
	if row.ReservationUuid.Valid {
		t.Reservation = &model.RecordRef{UUID: row.ReservationUuid.Bytes, Reference: row.ReservationReference.String, Status: row.ReservationStatus.String}
	}
	if row.VehicleUuid.Valid {
		t.Vehicle = &model.VehicleRef{UUID: row.VehicleUuid.Bytes, Plate: row.VehiclePlate.String}
	}
	if row.TransferUuid.Valid {
		t.Transfer = &model.TransferRef{UUID: row.TransferUuid.Bytes, Status: row.TransferStatus.String}
	}
	if model.IsLive(row.Status) && row.RentalUuid.Valid {
		switch {
		case row.TaskType == model.TypeHandover && row.RentalStatus.String == rentalPreparation:
			t.Inspection = "handover"
		case row.TaskType == model.TypeReturn && row.RentalStatus.String == rentalActive:
			t.Inspection = "return"
		}
	}
	t.Actions = actionsOf(row, a, t.Inspection != "")
	return t
}

// actionsOf lists what the caller can do now (the panel shows these).
func actionsOf(row db.GetFieldTaskRow, a access, inspection bool) []string {
	out := []string{}
	if !model.IsLive(row.Status) {
		return out
	}
	works := a.works(row)
	if a.assign {
		out = append(out, model.ActionAssign, "edit")
	}
	if _, ok := model.Next(row.Status, model.ActionStart); ok && works {
		out = append(out, model.ActionStart)
	}
	if _, ok := model.Next(row.Status, model.ActionReport); ok && works {
		out = append(out, model.ActionReport)
	}
	// Handover / return complete through the inspection form.
	recorded := row.TaskType == model.TypeHandover || row.TaskType == model.TypeReturn
	if inspection && works {
		out = append(out, "inspection")
	}
	if _, ok := model.Next(row.Status, model.ActionComplete); ok && works && !recorded {
		out = append(out, model.ActionComplete)
	}
	if _, ok := model.Next(row.Status, model.ActionCancel); ok && a.assign {
		out = append(out, model.ActionCancel)
	}
	return out
}

// row loads a task the caller may see (404 otherwise).
func (s *Service) row(ctx context.Context, orgID int64, id uuid.UUID) (db.GetFieldTaskRow, error) {
	row, err := s.q.GetFieldTask(ctx, db.GetFieldTaskParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if !inScope(ctx, row.BranchID, row.ToBranchID.Int64) || !accessOf(ctx).sees(row) {
		return row, ErrNotFound
	}
	return row, nil
}

// Get returns one task.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Task, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Task{}, err
	}
	return view(row, accessOf(ctx)), nil
}

func (s *Service) listParams(ctx context.Context, orgID int64, f model.Filter) db.ListFieldTasksParams {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if !model.Sortable[f.SortField] {
		f.SortField, f.SortDesc = "due_at", false
	}
	p := db.ListFieldTasksParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses),
		TaskTypes: nilIfEmpty(f.TaskTypes), BranchUuids: nilUUIDs(f.BranchUUIDs), AssigneeUuids: nilUUIDs(f.AssigneeUUIDs),
		IncludeUnassigned: pgtype.Bool{Bool: f.IncludeUnassigned, Valid: f.IncludeUnassigned}, Unassigned: boolArg(f.Unassigned),
		LocationKinds: nilIfEmpty(f.LocationKinds), ReservationUuids: nilUUIDs(f.ReservationUUIDs), Q: qArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if f.AssigneeID > 0 {
		p.AssigneeID = pgtype.Int8{Int64: f.AssigneeID, Valid: true}
	}
	if f.DueFrom != nil {
		p.DueFrom = ts(*f.DueFrom)
	}
	if f.DueTo != nil {
		p.DueTo = ts(*f.DueTo)
	}
	return p
}

// List returns a page of the board (every task in the caller's scope).
func (s *Service) List(ctx context.Context, orgID int64, f model.Filter) ([]model.Task, int64, error) {
	f.AssigneeID = 0
	return s.list(ctx, orgID, f)
}

// Mine returns a page of the caller's own tasks.
func (s *Service) Mine(ctx context.Context, orgID int64, f model.Filter) ([]model.Task, int64, error) {
	me := actorID(ctx)
	if me <= 0 {
		return []model.Task{}, 0, nil
	}
	f.AssigneeID, f.AssigneeUUIDs, f.IncludeUnassigned, f.Unassigned = me, nil, false, nil
	return s.list(ctx, orgID, f)
}

func (s *Service) list(ctx context.Context, orgID int64, f model.Filter) ([]model.Task, int64, error) {
	rows, err := s.q.ListFieldTasks(ctx, s.listParams(ctx, orgID, f))
	if err != nil {
		return nil, 0, err
	}
	a := accessOf(ctx)
	out := make([]model.Task, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, view(listRow(r), a))
	}
	return out, total, nil
}

// TargetUUIDs resolves bulk "all matching" over the board.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.Filter) ([]uuid.UUID, error) {
	p := s.listParams(ctx, orgID, f)
	return s.q.ListFieldTaskUUIDs(ctx, db.ListFieldTaskUUIDsParams{
		OrganizationID: orgID, BranchIds: p.BranchIds, Statuses: p.Statuses, TaskTypes: p.TaskTypes,
		BranchUuids: p.BranchUuids, AssigneeUuids: p.AssigneeUuids, IncludeUnassigned: p.IncludeUnassigned,
		Unassigned: p.Unassigned, LocationKinds: p.LocationKinds, ReservationUuids: p.ReservationUuids,
		DueFrom: p.DueFrom, DueTo: p.DueTo, Q: p.Q, LimitCount: MaxTargets,
	})
}

// Facets returns the board's filter options.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.ListFieldTaskFacets(ctx, db.ListFieldTaskFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{
		Status: []model.FacetValue{}, TaskType: []model.FacetValue{}, LocationKind: []model.FacetValue{},
		Branch: []model.FacetValue{}, Assignee: []model.FacetValue{},
	}
	for _, r := range rows {
		v := model.FacetValue{Value: r.Value, Label: r.Label, Count: r.Count}
		switch r.Facet {
		case "status":
			out.Status = append(out.Status, v)
		case "task_type":
			out.TaskType = append(out.TaskType, v)
		case "location_kind":
			out.LocationKind = append(out.LocationKind, v)
		case "branch":
			out.Branch = append(out.Branch, v)
		case "assignee":
			if v.Value == "" {
				v.Value = "none"
			}
			out.Assignee = append(out.Assignee, v)
		}
	}
	return out, nil
}

// Events returns the timeline of a task.
func (s *Service) Events(ctx context.Context, orgID int64, id uuid.UUID) ([]model.Event, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListFieldTaskEvents(ctx, db.ListFieldTaskEventsParams{OrganizationID: orgID, TaskID: row.ID})
	if err != nil {
		return nil, err
	}
	out := make([]model.Event, 0, len(rows))
	for _, r := range rows {
		e := model.Event{
			UUID: r.Uuid, Action: r.Action, Note: r.Note, Data: map[string]any{}, CreatedAt: r.CreatedAt.Time.UTC(),
			Actor: actorOf(r.ActorUuid, r.ActorName, r.ActorSurname, r.ActorEmail),
		}
		if r.FromStatus.Valid {
			v := r.FromStatus.String
			e.FromStatus = &v
		}
		if r.ToStatus.Valid {
			v := r.ToStatus.String
			e.ToStatus = &v
		}
		_ = json.Unmarshal(r.Data, &e.Data)
		out = append(out, e)
	}
	return out, nil
}

// Assignees lists the members who can work field tasks (all of them when
// the caller sees every branch; else those sharing a branch with the
// caller's scope).
func (s *Service) Assignees(ctx context.Context, orgID int64, q string) ([]model.Assignee, error) {
	rows, err := s.q.ListFieldAssignees(ctx, db.ListFieldAssigneesParams{OrganizationID: orgID, Q: qArg(q)})
	if err != nil {
		return nil, err
	}
	branchUUIDs := map[int64]uuid.UUID{}
	scope := orgctx.BranchScope(ctx)
	out := []model.Assignee{}
	for _, r := range rows {
		if scope != nil && r.BranchScope != "all" && !overlaps(scope, r.BranchIds) {
			continue
		}
		a := model.Assignee{
			UUID: r.Uuid, Name: r.Name, Surname: r.Surname, Email: r.Email, BranchScope: r.BranchScope, BranchUUIDs: []uuid.UUID{},
		}
		for _, id := range r.BranchIds {
			u, ok := branchUUIDs[id]
			if !ok {
				if b, err := s.q.GetBranchUUIDByID(ctx, db.GetBranchUUIDByIDParams{OrganizationID: orgID, ID: id}); err == nil {
					u = b
					branchUUIDs[id] = b
				}
			}
			if u != uuid.Nil {
				a.BranchUUIDs = append(a.BranchUUIDs, u)
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// IsFieldMember reports whether a user is a member who can work field
// tasks (bulk assign params).
func (s *Service) IsFieldMember(ctx context.Context, orgID int64, user uuid.UUID) bool {
	m, err := s.q.GetFieldMember(ctx, db.GetFieldMemberParams{OrganizationID: orgID, Uuid: user})
	return err == nil && m.CanWork
}

func overlaps(a, b []int64) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
