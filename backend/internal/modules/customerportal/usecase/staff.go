package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Staff side (/v1/tenant/customer-requests): the requests of the
// reservations in the member's branch scope (pick-up or return branch).

func staffView(r db.ListCustomerRequestsRow) model.StaffRequest {
	out := model.StaffRequest{
		Request: model.Request{
			UUID: r.Uuid, Type: r.RequestType, Status: r.Status, Message: r.Message, RequestedStartsAt: timePtr(r.RequestedStartsAt),
			RequestedEndsAt: timePtr(r.RequestedEndsAt), PolicyText: r.PolicyText, CancellationFee: feeOf(r.CancellationFee.Valid, r.CancellationFee),
			Currency: r.Currency, Outcome: r.Outcome, ResolutionNote: r.ResolutionNote, ResolvedAt: timePtr(r.ResolvedAt),
			CreatedAt: r.CreatedAt.Time.UTC(),
		},
		Reservation: model.ReservationRef{
			UUID: r.ReservationUuid, Reference: r.ReservationReference, Status: r.ReservationStatus,
			StartsAt: r.ReservationStartsAt.Time.UTC(), EndsAt: r.ReservationEndsAt.Time.UTC(),
		},
		CustomerUUID: r.CustomerUuid, CustomerName: customerFullName(r.CustomerType, r.FirstName, r.LastName, r.CompanyTitle),
		RequesterRole: model.RoleRenter, ContactMasked: r.ContactMasked, UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	out.RequesterName = out.CustomerName
	if r.DriverFirstName.Valid {
		out.RequesterRole = model.RoleDriver
		out.RequesterName = strings.TrimSpace(r.DriverFirstName.String + " " + r.DriverLastName.String)
	}
	if r.ResolvedByUuid.Valid {
		out.ResolvedBy = &model.Actor{UUID: r.ResolvedByUuid.Bytes, Name: r.ResolvedByName.String, Surname: r.ResolvedBySurname.String, Email: r.ResolvedByEmail.String}
	}
	return out
}

func customerFullName(customerType, first, last, company string) string {
	if customerType == "corporate" && strings.TrimSpace(company) != "" {
		return strings.TrimSpace(company)
	}
	return strings.TrimSpace(first + " " + last)
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

// ListRequests returns a page of customer requests.
func (s *Service) ListRequests(ctx context.Context, orgID int64, f model.RequestFilter) ([]model.StaffRequest, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	var reservations []uuid.UUID
	if len(f.ReservationUUIDs) > 0 {
		reservations = f.ReservationUUIDs
	}
	q := strings.TrimSpace(f.Q)
	rows, err := s.q.ListCustomerRequests(ctx, db.ListCustomerRequestsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses), RequestTypes: nilIfEmpty(f.Types),
		ReservationUuids: reservations, Q: pgtype.Text{String: q, Valid: q != ""}, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.StaffRequest, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, staffView(r))
	}
	return out, total, nil
}

func (s *Service) requestRow(ctx context.Context, orgID int64, id uuid.UUID) (db.ListCustomerRequestsRow, error) {
	rows, err := s.q.ListCustomerRequests(ctx, db.ListCustomerRequestsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Uuid: pgtype.UUID{Bytes: id, Valid: true}, SortDesc: true,
		LimitCount: 1,
	})
	if err != nil {
		return db.ListCustomerRequestsRow{}, err
	}
	if len(rows) == 0 {
		return db.ListCustomerRequestsRow{}, ErrRequestNotFound
	}
	return rows[0], nil
}

// GetRequest returns one request (another organization's or outside the
// branch scope: ErrRequestNotFound).
func (s *Service) GetRequest(ctx context.Context, orgID int64, id uuid.UUID) (model.StaffRequest, error) {
	row, err := s.requestRow(ctx, orgID, id)
	if err != nil {
		return model.StaffRequest{}, err
	}
	return staffView(row), nil
}

// RequestStats counts the open requests (nav badge).
func (s *Service) RequestStats(ctx context.Context, orgID int64) (model.RequestStats, error) {
	n, err := s.q.CountOpenCustomerRequests(ctx, db.CountOpenCustomerRequestsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	return model.RequestStats{Open: n}, err
}

// ResolveRequest closes an open request (resolved / rejected, with a note
// the customer sees). Acting on the reservation (change, cancel) stays a
// separate staff action with its own permission.
func (s *Service) ResolveRequest(ctx context.Context, orgID int64, id uuid.UUID, in model.ResolveInput) (model.StaffRequest, error) {
	row, err := s.requestRow(ctx, orgID, id)
	if err != nil {
		return model.StaffRequest{}, err
	}
	cur := staffView(row)
	note := strings.TrimSpace(in.Note)
	ve := &ValidationError{}
	if in.Status != model.RequestResolved && in.Status != model.RequestRejected {
		ve.Details = append(ve.Details, Detail{Field: "status", Code: "invalid", Message: "status is resolved or rejected"})
	}
	if in.Status == model.RequestRejected && note == "" {
		ve.Details = append(ve.Details, Detail{Field: "note", Code: "required", Message: "tell the customer why"})
	}
	if len([]rune(note)) > maxMessage {
		ve.Details = append(ve.Details, Detail{Field: "note", Code: "too_long", Message: "at most 2000 characters"})
	}
	if len(ve.Details) > 0 {
		return model.StaffRequest{}, ve
	}
	if cur.Status != model.RequestOpen {
		return model.StaffRequest{}, stateErr("request_closed", "this request is already closed")
	}
	var actor pgtype.Int8
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		actor = pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.ResolveCustomerRequest(ctx, db.ResolveCustomerRequestParams{
			OrganizationID: orgID, ID: row.ID, Status: in.Status, ResolutionNote: note, ResolvedBy: actor, ResolvedAt: ts(s.clock()),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("request_closed", "this request is already closed")
		}
		return q.InsertReservationEvent(ctx, db.InsertReservationEventParams{
			OrganizationID: orgID, ReservationID: row.ReservationID, Action: "customer_request_" + in.Status, ActorID: actor,
			Data: mustJSON(map[string]any{"request_uuid": id, "type": cur.Type, "note": note}),
		})
	})
	if err != nil {
		return model.StaffRequest{}, err
	}
	s.publish(ctx, model.ResourceRequests, "updated", id)
	s.publish(ctx, resmodel.ResourceReservations, "updated", cur.Reservation.UUID)
	return s.GetRequest(ctx, orgID, id)
}
