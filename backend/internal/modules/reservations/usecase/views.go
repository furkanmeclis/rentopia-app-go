package usecase

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func actorOf(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

// Get returns one reservation in the caller's branch scope.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Reservation, error) {
	row, err := s.q.GetReservationView(ctx, db.GetReservationViewParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Reservation{}, ErrNotFound
	}
	if err != nil {
		return model.Reservation{}, err
	}
	if !inScope(ctx, row.PickupBranchID, row.ReturnBranchID) {
		return model.Reservation{}, ErrNotFound
	}
	drivers, err := s.q.ListReservationDrivers(ctx, db.ListReservationDriversParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return model.Reservation{}, err
	}
	history, err := s.q.ListVehicleAssignments(ctx, db.ListVehicleAssignmentsParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return model.Reservation{}, err
	}
	return s.view(row, drivers, history), nil
}

func (s *Service) view(row db.GetReservationViewRow, drivers []db.ListReservationDriversRow, history []db.ListVehicleAssignmentsRow) model.Reservation {
	m := money.FromNumericOrZero
	r := model.Reservation{
		UUID: row.Uuid, Reference: row.Reference, Status: row.Status, PaymentStatus: row.PaymentStatus,
		CapacityCommitment: row.CapacityCommitment,
		PickupBranch:       model.BranchRef{UUID: row.PickupBranchUuid, Code: row.PickupBranchCode, Name: row.PickupBranchName},
		ReturnBranch:       model.BranchRef{UUID: row.ReturnBranchUuid, Code: row.ReturnBranchCode, Name: row.ReturnBranchName},
		StartsAt:           row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(),
		Customer: customerRef(row.CustomerUuid, row.CustomerType, row.CustomerFirstName, row.CustomerLastName, row.CustomerCompanyTitle,
			row.CustomerIdentityHint, row.CustomerPhone, row.CustomerEmail),
		Drivers: []model.ReservationDriver{}, Channel: row.Channel, CollectingParty: row.CollectingParty,
		ApprovalProtocol: row.ApprovalProtocol,
		QuoteUUID:        uuidPtr(row.QuoteUuid), SettingsVersion: row.SettingsVersion, Policy: json.RawMessage(row.PolicySnapshot),
		ReviewDeadline: timePtr(row.ReviewDeadline), PaymentDeadline: timePtr(row.PaymentDeadline),
		AssignedUser: actorOf(row.AssignedUserUuid, row.AssignedUserName, row.AssignedUserSurname, row.AssignedUserEmail),
		Note:         row.Note, OpenTasks: row.OpenTasks, ConfirmedAt: timePtr(row.ConfirmedAt), CancelledAt: timePtr(row.CancelledAt),
		CancelReason: row.CancelReason,
		CreatedBy:    actorOf(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		CreatedAt:    row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if rng, err := rentaltime.NewRange(r.StartsAt, r.EndsAt); err == nil {
		r.BillingDays = rng.BillingDays()
	}
	if len(row.EligibilityOverride) > 0 {
		r.EligibilityOverride = json.RawMessage(row.EligibilityOverride)
	}
	if row.ExternalReference.Valid {
		e := row.ExternalReference.String
		r.ExternalReference = &e
	}
	if row.PickupPointUuid.Valid {
		r.PickupPoint = &model.PointRef{UUID: row.PickupPointUuid.Bytes, Name: row.PickupPointName.String}
	}
	if row.ReturnPointUuid.Valid {
		r.ReturnPoint = &model.PointRef{UUID: row.ReturnPointUuid.Bytes, Name: row.ReturnPointName.String}
	}
	r.PickupDelivery, r.ReturnDelivery = storedDelivery(row.PickupDelivery), storedDelivery(row.ReturnDelivery)
	if row.InvoiceCustomerUuid.Valid {
		r.InvoiceCustomer = &model.CustomerRef{
			UUID: row.InvoiceCustomerUuid.Bytes, CustomerType: row.InvoiceCustomerType.String,
			DisplayName: displayName(row.InvoiceCustomerType.String, row.InvoiceFirstName.String, row.InvoiceLastName.String, row.InvoiceCompanyTitle.String),
		}
	}
	total, paid := m(row.TotalAmount), m(row.PaidAmount)
	r.Price = model.Price{
		Currency: row.Currency, Total: total, Prepayment: m(row.PrepaymentAmount), Paid: paid, Balance: total.Sub(paid),
		PrepaymentDue: m(row.PrepaymentAmount).Sub(paid).ClampZero(),
		Commission:    m(row.CommissionAmount), NetReceivable: m(row.NetReceivable),
	}
	_ = json.Unmarshal(row.QuoteSnapshot, &r.Price.Snapshot)
	if r.Price.Snapshot.Extensions == nil {
		r.Price.Snapshot.Extensions = []model.Extension{}
	}
	if r.Price.Snapshot.Lines == nil {
		r.Price.Snapshot.Lines = []model.QuoteLine{}
	}
	if r.Price.Snapshot.Extras == nil {
		r.Price.Snapshot.Extras = []model.ExtraRequest{}
	}
	for _, d := range drivers {
		rd := model.ReservationDriver{
			DriverUUID: uuidPtr(d.DriverUuid), CustomerUUID: uuidPtr(d.CustomerUuid), Role: d.Role, Eligible: d.Eligible,
			Reasons: d.Reasons, OverrideReason: d.OverrideReason, LicenceClass: d.LicenceClass.String, LicenceMasked: d.LicenceHint.String,
			Name: strings.TrimSpace(d.FirstName.String + " " + d.LastName.String),
		}
		if rd.Reasons == nil {
			rd.Reasons = []string{}
		}
		if !d.DriverUuid.Valid {
			var snap struct {
				Name          string `json:"name"`
				LicenceMasked string `json:"licence_masked"`
			}
			_ = json.Unmarshal(d.Snapshot, &snap)
			rd.Name, rd.LicenceMasked = snap.Name, snap.LicenceMasked
		}
		r.Drivers = append(r.Drivers, rd)
	}
	plan := model.VehiclePlan{
		SoldProduct:   model.ProductRef{UUID: row.SoldProductUuid, Code: row.SoldProductCode, Title: row.SoldProductTitle, Version: row.SoldProductVersion},
		Product:       model.ProductRef{UUID: row.ProductUuid, Code: row.ProductCode, Title: row.ProductTitle, Version: row.ProductVersion},
		GuaranteeType: row.GuaranteeType, AllocationUUID: uuidPtr(row.AllocationUuid), AllocationKind: row.AllocationKind.String,
		AllocationState: row.AllocationStatus.String, History: []model.Assignment{},
	}
	_ = json.Unmarshal(row.SoldSnapshot, &plan.SoldConditions)
	_ = json.Unmarshal(row.ProductSnapshot, &plan.Conditions)
	active := row.AllocationStatus.Valid && row.AllocationStatus.String == availmodel.StatusActive
	if active {
		plan.HoldExpiresAt = timePtr(row.AllocationExpiresAt)
		if row.VehicleUuid.Valid {
			v := &model.VehicleRef{UUID: row.VehicleUuid.Bytes, Plate: row.VehiclePlate.String, Label: row.VehicleLabel}
			plan.PlannedVehicle = v
			if row.AllocationPlateFixed.Bool {
				plan.AssignedVehicle, plan.PlateFixed = v, true
			}
		}
	}
	for _, h := range history {
		a := model.Assignment{
			Kind: h.Kind, Source: h.Source, Note: h.Note, StartedAt: h.StartedAt.Time.UTC(), EndedAt: timePtr(h.EndedAt),
			Product: model.ProductRef{UUID: h.ProductUuid, Code: h.ProductCode, Title: h.ProductTitle, Version: h.ProductVersion},
			Actor:   actorOf(h.ActorUuid, h.ActorName, h.ActorSurname, h.ActorEmail),
		}
		if h.VehicleUuid.Valid {
			a.Vehicle = &model.VehicleRef{UUID: h.VehicleUuid.Bytes, Plate: h.VehiclePlate.String}
		}
		plan.History = append(plan.History, a)
	}
	r.Plan = plan
	holds := model.HoldsCapacity(r.Status) && active
	r.Actions = model.Actions{
		Submit:  model.CanAct(r.Status, model.ActionSubmit),
		Approve: model.CanAct(r.Status, model.ActionApprove),
		Reject:  model.CanAct(r.Status, model.ActionReject),
		Confirm: model.CanAct(r.Status, model.ActionConfirm),
		Cancel:  model.CanAct(r.Status, model.ActionCancel),
		NoShow:  model.CanAct(r.Status, model.ActionNoShow) && !s.clock().Before(r.StartsAt),
		Change:  holds,
		Extend:  holds,
		Plate:   holds,
	}
	return r
}

// listParams maps a filter to the list query (branch scope from orgctx).
func listParams(ctx context.Context, orgID int64, f model.ListFilter) db.ListReservationsParams {
	p := db.ListReservationsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses),
		PaymentStatuses: nilIfEmpty(f.PaymentStatuses), Channels: nilIfEmpty(f.Channels), ProductUuids: nilUUIDs(f.ProductUUIDs),
		CustomerUuids: nilUUIDs(f.CustomerUUIDs), BranchUuids: nilUUIDs(f.BranchUUIDs), AssignedUserUuids: nilUUIDs(f.AssignedUserUUIDs),
		StartsFrom: tsPtr(f.StartsFrom), StartsTo: tsPtr(f.StartsTo), Q: qArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	return p
}

// List returns a page of reservations in the caller's branch scope.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.ListItem, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if _, ok := model.Sortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	rows, err := s.q.ListReservations(ctx, listParams(ctx, orgID, f))
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ListItem, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		it := model.ListItem{
			UUID: r.Uuid, Reference: r.Reference, Status: r.Status, PaymentStatus: r.PaymentStatus, CapacityCommitment: r.CapacityCommitment,
			PickupBranch: model.BranchRef{UUID: r.PickupBranchUuid, Code: r.PickupBranchCode, Name: r.PickupBranchName},
			ReturnBranch: model.BranchRef{UUID: r.ReturnBranchUuid, Code: r.ReturnBranchCode, Name: r.ReturnBranchName},
			StartsAt:     r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(),
			Product: model.ProductRef{UUID: r.ProductUuid, Code: r.ProductCode, Title: r.ProductTitle},
			Customer: model.CustomerRef{UUID: r.CustomerUuid, CustomerType: r.CustomerType,
				DisplayName: displayName(r.CustomerType, r.CustomerFirstName, r.CustomerLastName, r.CustomerCompanyTitle)},
			Channel: r.Channel, Currency: r.Currency, TotalAmount: money.FromNumericOrZero(r.TotalAmount),
			PaidAmount: money.FromNumericOrZero(r.PaidAmount), PlateFixed: r.PlateFixed, OpenTasks: r.OpenTasks,
			AssignedUser: actorOf(r.AssignedUserUuid, r.AssignedUserName, r.AssignedUserSurname, pgtype.Text{}),
			CreatedAt:    r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
			ReviewDeadline: timePtr(r.ReviewDeadline), PrepaymentAmount: money.FromNumericOrZero(r.PrepaymentAmount),
			ApprovalProtocol: r.ApprovalProtocol,
		}
		if r.ExternalReference.Valid {
			e := r.ExternalReference.String
			it.ExternalReference = &e
		}
		if r.VehicleUuid.Valid {
			it.Vehicle = &model.VehicleRef{UUID: r.VehicleUuid.Bytes, Plate: r.VehiclePlate.String}
		}
		switch r.Status {
		case model.StatusAwaitingReview, model.StatusRequest:
			it.Deadline = timePtr(r.ReviewDeadline)
		case model.StatusAwaitingPayment:
			it.Deadline = timePtr(r.PaymentDeadline)
		}
		out = append(out, it)
	}
	return out, total, nil
}

// Facets returns the dynamic filter options in the caller's branch scope.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.ListReservationFacets(ctx, db.ListReservationFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{
		Status: []model.Facet{}, PaymentStatus: []model.Facet{}, Channel: []model.Facet{},
		Product: []model.Facet{}, Branch: []model.Facet{}, Customer: []model.Facet{},
	}
	for _, r := range rows {
		f := model.Facet{Value: r.Value, Label: r.Label, Count: r.Count}
		switch r.Facet {
		case "status":
			out.Status = append(out.Status, f)
		case "payment_status":
			out.PaymentStatus = append(out.PaymentStatus, f)
		case "channel":
			out.Channel = append(out.Channel, f)
		case "product":
			out.Product = append(out.Product, f)
		case "branch":
			out.Branch = append(out.Branch, f)
		case "customer":
			if len(out.Customer) < 200 {
				out.Customer = append(out.Customer, f)
			}
		}
	}
	return out, nil
}

// TargetUUIDs returns the uuids of a filter (bulk "all matching").
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	return s.q.ListReservationTargetUUIDs(ctx, db.ListReservationTargetUUIDsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses),
		PaymentStatuses: nilIfEmpty(f.PaymentStatuses), Channels: nilIfEmpty(f.Channels), ProductUuids: nilUUIDs(f.ProductUUIDs),
		CustomerUuids: nilUUIDs(f.CustomerUUIDs), BranchUuids: nilUUIDs(f.BranchUUIDs),
		StartsFrom: tsPtr(f.StartsFrom), StartsTo: tsPtr(f.StartsTo), Q: qArg(f.Q), LimitCount: limit,
	})
}

// Events returns a page of the reservation timeline (newest first).
func (s *Service) Events(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.Event, int64, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.q.ListReservationEvents(ctx, db.ListReservationEventsParams{OrganizationID: orgID, ReservationID: c.ID, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountReservationEvents(ctx, db.CountReservationEventsParams{OrganizationID: orgID, ReservationID: c.ID})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Event, 0, len(rows))
	for _, r := range rows {
		e := model.Event{
			UUID: r.Uuid, Action: r.Action, Reason: r.Reason, Data: json.RawMessage(r.Data), CreatedAt: r.CreatedAt.Time.UTC(),
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
		out = append(out, e)
	}
	return out, total, nil
}
