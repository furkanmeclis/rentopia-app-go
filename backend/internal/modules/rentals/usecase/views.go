package usecase

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func customerName(typ, first, last, company string) string {
	if typ == "corporate" && strings.TrimSpace(company) != "" {
		return company
	}
	return strings.TrimSpace(first + " " + last)
}

func refOf(id pgtype.UUID, code, name pgtype.Text) *model.Ref {
	if !id.Valid {
		return nil
	}
	return &model.Ref{UUID: id.Bytes, Code: code.String, Name: name.String}
}

func pointOf(id pgtype.UUID, name pgtype.Text) *model.Ref {
	if !id.Valid {
		return nil
	}
	return &model.Ref{UUID: id.Bytes, Name: name.String}
}

func i32(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	x := v.Int32
	return &x
}

func i16(v pgtype.Int2) *int16 {
	if !v.Valid {
		return nil
	}
	x := v.Int16
	return &x
}

// Get returns one rental in the caller's branch scope.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Rental, error) {
	scope, err := s.q.GetRentalScope(ctx, db.GetRentalScopeParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Rental{}, ErrNotFound
	}
	if err != nil {
		return model.Rental{}, err
	}
	if !inScope(ctx, scope.PickupBranchID, scope.ReturnBranchID, scope.ActualReturnBranchID) {
		return model.Rental{}, ErrNotFound
	}
	row, err := s.q.GetRentalView(ctx, db.GetRentalViewParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Rental{}, err
	}
	legs, err := s.q.ListRentalVehicles(ctx, db.ListRentalVehiclesParams{OrganizationID: orgID, RentalID: row.ID})
	if err != nil {
		return model.Rental{}, err
	}
	drivers, err := s.q.ListRentalDrivers(ctx, db.ListRentalDriversParams{OrganizationID: orgID, RentalID: row.ID})
	if err != nil {
		return model.Rental{}, err
	}
	charges, err := s.q.ListRentalCharges(ctx, db.ListRentalChargesParams{OrganizationID: orgID, RentalID: row.ID})
	if err != nil {
		return model.Rental{}, err
	}
	out := model.Rental{
		UUID: row.Uuid, Reference: row.Reference, Status: row.Status,
		Reservation: model.ReservationRef{
			UUID: row.ReservationUuid, Reference: row.ReservationReference, Status: row.ReservationStatus, Currency: strings.TrimSpace(row.Currency),
			TotalAmount: money.FromNumericOrZero(row.TotalAmount), PaidAmount: money.FromNumericOrZero(row.PaidAmount), PaymentStatus: row.PaymentStatus,
		},
		Customer: model.CustomerRef{
			UUID: row.CustomerUuid, CustomerType: row.CustomerType,
			DisplayName: customerName(row.CustomerType, row.CustomerFirstName, row.CustomerLastName, row.CustomerCompanyTitle),
		},
		Product:         model.Ref{UUID: row.ProductUuid, Code: row.ProductCode, Name: row.ProductTitle},
		Vehicle:         model.VehicleRef{UUID: row.VehicleUuid, Plate: row.VehiclePlate, Label: row.VehicleLabel},
		PlannedPickupAt: row.PlannedPickupAt.Time.UTC(), PlannedReturnAt: row.PlannedReturnAt.Time.UTC(),
		PlannedPickup: model.Place{
			Branch: &model.Ref{UUID: row.PickupBranchUuid, Code: row.PickupBranchCode, Name: row.PickupBranchName},
			Point:  pointOf(row.PickupPointUuid, row.PickupPointName),
		},
		PlannedReturn: model.Place{
			Branch: &model.Ref{UUID: row.ReturnBranchUuid, Code: row.ReturnBranchCode, Name: row.ReturnBranchName},
			Point:  pointOf(row.ReturnPointUuid, row.ReturnPointName),
		},
		ActualPickupAt: timePtr(row.ActualPickupAt), ActualReturnAt: timePtr(row.ActualReturnAt),
		ActualPickup: model.Place{
			Branch: refOf(row.ActualPickupBranchUuid, row.ActualPickupBranchCode, row.ActualPickupBranchName),
			Point:  pointOf(row.ActualPickupPointUuid, row.ActualPickupPointName),
		},
		ActualReturn: model.Place{
			Branch: refOf(row.ActualReturnBranchUuid, row.ActualReturnBranchCode, row.ActualReturnBranchName),
			Point:  pointOf(row.ActualReturnPointUuid, row.ActualReturnPointName),
		},
		OverdueAt: timePtr(row.OverdueAt), LateMinutes: row.LateMinutes, AllocationEndsAt: timePtr(row.AllocationEndsAt),
		AllocationKind: row.AllocationKind.String, Policy: model.PolicyFromSnapshot(row.PolicySnapshot),
		Legs: []model.Leg{}, Drivers: []model.Driver{}, Charges: []model.Charge{},
		ReturnNote: row.ReturnNote, DamageNote: row.DamageNote, Note: row.Note, CancelReason: row.CancelReason,
		CheckedAt: timePtr(row.CheckedAt), CancelledAt: timePtr(row.CancelledAt), ClosedAt: timePtr(row.ClosedAt),
		CreatedBy:   actorOf(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		DeliveredBy: actorOf(row.DeliveredByUuid, row.DeliveredByName, row.DeliveredBySurname, row.DeliveredByEmail),
		ReturnedBy:  actorOf(row.ReturnedByUuid, row.ReturnedByName, row.ReturnedBySurname, row.ReturnedByEmail),
		CreatedAt:   row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if len(row.ContractWaiver) > 0 {
		out.ContractWaiver = json.RawMessage(row.ContractWaiver)
	}
	for _, l := range legs {
		leg := model.Leg{
			UUID: l.Uuid, Reason: l.Reason, Vehicle: model.VehicleRef{UUID: l.VehicleUuid, Plate: l.VehiclePlate, Label: l.VehicleLabel},
			OutAt: timePtr(l.OutAt), OutBranch: refOf(l.OutBranchUuid, l.OutBranchCode, l.OutBranchName), OutKm: i32(l.OutKm), OutFuel: i16(l.OutFuel),
			InAt: timePtr(l.InAt), InBranch: refOf(l.InBranchUuid, l.InBranchCode, l.InBranchName), InKm: i32(l.InKm), InFuel: i16(l.InFuel),
			Note: l.Note,
		}
		if l.OutKm.Valid && l.InKm.Valid {
			d := l.InKm.Int32 - l.OutKm.Int32
			leg.DrivenKm = &d
		}
		out.Legs = append(out.Legs, leg)
	}
	for _, d := range drivers {
		var snap struct {
			Name        string `json:"name"`
			LicenceHint string `json:"licence_hint"`
		}
		_ = json.Unmarshal(d.Snapshot, &snap)
		dr := model.Driver{Role: d.Role, Name: snap.Name, LicenceHint: snap.LicenceHint}
		if d.DriverUuid.Valid {
			id := uuid.UUID(d.DriverUuid.Bytes)
			dr.UUID = &id
			if n := strings.TrimSpace(d.FirstName.String + " " + d.LastName.String); n != "" {
				dr.Name = n
			}
			if d.LicenceHint.Valid && d.LicenceHint.String != "" {
				dr.LicenceHint = d.LicenceHint.String
			}
		}
		out.Drivers = append(out.Drivers, dr)
	}
	for _, ch := range charges {
		item := model.Charge{
			UUID: ch.Uuid, Kind: ch.Kind, Source: ch.Source, Status: ch.Status, Currency: strings.TrimSpace(ch.Currency),
			Amount: money.FromNumericOrZero(ch.Amount), SettingsVersion: ch.SettingsVersion, Calculation: json.RawMessage(ch.Calculation),
			Reason: ch.Reason, DecidedAt: timePtr(ch.DecidedAt), DecisionNote: ch.DecisionNote, CreatedAt: ch.CreatedAt.Time.UTC(),
			DecidedBy: actorOf(ch.DecidedByUuid, ch.DecidedByName, ch.DecidedBySurname, ch.DecidedByEmail),
			CreatedBy: actorOf(ch.CreatedByUuid, ch.CreatedByName, ch.CreatedBySurname, ch.CreatedByEmail),
			Disputed:  ch.Disputed, OverrideReason: ch.OverrideReason, OverriddenAt: timePtr(ch.OverriddenAt),
			OverriddenBy: actorOf(ch.OverriddenByUuid, ch.OverriddenByName, ch.OverriddenBySurname, ch.OverriddenByEmail),
		}
		if ch.InspectionUuid.Valid {
			id := uuid.UUID(ch.InspectionUuid.Bytes)
			item.InspectionUUID = &id
		}
		if ch.OriginalAmount.Valid {
			orig := money.FromNumericOrZero(ch.OriginalAmount)
			item.OriginalAmount = &orig
		}
		if ch.TollPassUuid.Valid {
			id := uuid.UUID(ch.TollPassUuid.Bytes)
			item.TollPassUUID = &id
		}
		if ch.ReversalOfUuid.Valid {
			id := uuid.UUID(ch.ReversalOfUuid.Bytes)
			item.ReversalOfUUID = &id
		}
		item.Reversed = ch.Reversed
		out.Charges = append(out.Charges, item)
	}
	resID, _ := s.reservationID(ctx, orgID, row.ReservationUuid)
	if state, err := s.documents.ContractState(unscoped(ctx), orgID, resID); err == nil {
		out.Contract = state
	}
	return out, nil
}

func (s *Service) reservationID(ctx context.Context, orgID int64, id uuid.UUID) (int64, error) {
	r, err := s.q.GetRentalReservationFacts(ctx, db.GetRentalReservationFactsParams{OrganizationID: orgID, Uuid: id})
	return r.ID, err
}

func (s *Service) listParams(ctx context.Context, orgID int64, f model.ListFilter) db.ListRentalsParams {
	if _, ok := model.Sortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	return db.ListRentalsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses),
		BranchUuids: nilUUIDs(f.BranchUUIDs), ProductUuids: nilUUIDs(f.ProductUUIDs), VehicleUuids: nilUUIDs(f.VehicleUUIDs),
		CustomerUuids: nilUUIDs(f.CustomerUUIDs), ReservationUuids: nilUUIDs(f.ReservationUUIDs), OverdueOnly: f.OverdueOnly,
		Q: qArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
}

func listItem(r db.ListRentalsRow) model.ListItem {
	return model.ListItem{
		UUID: r.Uuid, Reference: r.Reference, Status: r.Status, ReservationUUID: r.ReservationUuid,
		Customer: model.CustomerRef{
			UUID: r.CustomerUuid, CustomerType: r.CustomerType,
			DisplayName: customerName(r.CustomerType, r.CustomerFirstName, r.CustomerLastName, r.CustomerCompanyTitle),
		},
		Product:         model.Ref{UUID: r.ProductUuid, Code: r.ProductCode, Name: r.ProductTitle},
		Vehicle:         model.VehicleRef{UUID: r.VehicleUuid, Plate: r.VehiclePlate},
		PickupBranch:    model.Ref{UUID: r.PickupBranchUuid, Code: r.PickupBranchCode, Name: r.PickupBranchName},
		ReturnBranch:    model.Ref{UUID: r.ReturnBranchUuid, Code: r.ReturnBranchCode, Name: r.ReturnBranchName},
		ActualReturn:    refOf(r.ActualReturnBranchUuid, r.ActualReturnBranchCode, r.ActualReturnBranchName),
		PlannedPickupAt: r.PlannedPickupAt.Time.UTC(), PlannedReturnAt: r.PlannedReturnAt.Time.UTC(),
		ActualPickupAt: timePtr(r.ActualPickupAt), ActualReturnAt: timePtr(r.ActualReturnAt), OverdueAt: timePtr(r.OverdueAt),
		LateMinutes: r.LateMinutes, OpenCharges: r.OpenCharges, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
}

// List returns a page of rentals in the caller's branch scope.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.ListItem, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	rows, err := s.q.ListRentals(ctx, s.listParams(ctx, orgID, f))
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ListItem, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, listItem(r))
	}
	return out, total, nil
}

// ExportRows returns up to limit list rows of the filter (export engine).
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]model.ListItem, error) {
	f.Limit, f.Offset = limit, 0
	rows, err := s.q.ListRentals(ctx, s.listParams(ctx, orgID, f))
	if err != nil {
		return nil, err
	}
	out := make([]model.ListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, listItem(r))
	}
	return out, nil
}

// Facets returns the facet counts of the rentals list.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.ListRentalFacets(ctx, db.ListRentalFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{Status: []model.FacetValue{}, Branch: []model.FacetValue{}, Product: []model.FacetValue{}}
	for _, r := range rows {
		v := model.FacetValue{Value: r.Value, Label: r.Label, Count: r.Count}
		switch r.Facet {
		case "status":
			out.Status = append(out.Status, v)
		case "branch":
			out.Branch = append(out.Branch, v)
		case "product":
			out.Product = append(out.Product, v)
		}
	}
	return out, nil
}

// Events returns a page of the rental timeline (newest first).
func (s *Service) Events(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.Event, int64, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.q.ListRentalEvents(ctx, db.ListRentalEventsParams{OrganizationID: orgID, RentalID: c.ID, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountRentalEvents(ctx, db.CountRentalEventsParams{OrganizationID: orgID, RentalID: c.ID})
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
