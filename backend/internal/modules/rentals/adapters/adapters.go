// Package adapters connects rentals to the shared export engine and to the
// reservation core it builds on (plate fix, extension rule, plan effects,
// late return tasks). The organization of an engine call always comes from
// the context (orgctx).
package adapters

import (
	"context"
	"errors"
	"strings"
	"time"

	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Resource is the engine slug of rentals.
const Resource = model.ResourceRentals

var (
	errNoOrganization = errors.New("rentals: organization scope is required")
	errNoImport       = errors.New("rentals: import is not supported")
)

// ---------------------------------------------------------------- reservations

// Reservations adapts the reservation use case to the rentals port and
// maps its errors to the rentals answers.
type Reservations struct {
	svc *resusecase.Service
}

// NewReservations builds the port.
func NewReservations(svc *resusecase.Service) *Reservations { return &Reservations{svc: svc} }

var _ usecase.Reservations = (*Reservations)(nil)

// FixPlate promises the plate of a reservation (staff plate change).
func (a *Reservations) FixPlate(ctx context.Context, orgID int64, reservation, vehicle uuid.UUID) error {
	_, err := a.svc.Plate(ctx, orgID, reservation, resmodel.PlateInput{VehicleUUID: &vehicle, Fix: true, Reason: "rental"})
	return mapReservationErr(err, "vehicle_uuid")
}

// Extend applies the reservation's extension rule.
func (a *Reservations) Extend(ctx context.Context, orgID int64, reservation uuid.UUID, endsAt time.Time, reason string) (usecase.ExtendResult, error) {
	res, err := a.svc.Extend(ctx, orgID, reservation, resmodel.ExtendInput{EndsAt: &endsAt, Reason: reason})
	if err != nil {
		return usecase.ExtendResult{}, mapReservationErr(err, "ends_at")
	}
	out := usecase.ExtendResult{Total: res.Reservation.Price.Total.String()}
	if res.Difference != nil {
		out.Difference = res.Difference.String()
	}
	return out, nil
}

// RecordPlanEffects records engine moves on the other reservations.
func (a *Reservations) RecordPlanEffects(ctx context.Context, tx pgx.Tx, orgID int64, r availmodel.HookResult, cause string, causeRef uuid.UUID) ([]uuid.UUID, error) {
	return a.svc.RecordPlanEffects(ctx, tx, orgID, r, cause, causeRef)
}

// LateReturnTasks opens late_return tasks.
func (a *Reservations) LateReturnTasks(ctx context.Context, orgID int64, rental uuid.UUID, allocations []uuid.UUID, data map[string]any) []uuid.UUID {
	return a.svc.LateReturnTasks(ctx, orgID, rental, allocations, data)
}

// mapReservationErr turns reservation answers into rental answers (same
// codes and data).
func mapReservationErr(err error, field string) error {
	if err == nil {
		return nil
	}
	var ve *resusecase.ValidationError
	if errors.As(err, &ve) {
		out := &usecase.ValidationError{}
		for _, d := range ve.Details {
			out.Details = append(out.Details, usecase.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		return out
	}
	var se *resusecase.StateError
	if errors.As(err, &se) {
		return &usecase.StateError{Code: se.Code, Message: se.Message}
	}
	var xe *resusecase.ExtensionConflictError
	if errors.As(err, &xe) {
		return &usecase.ConflictError{Code: "EXTENSION_CONFLICT", Detail: "extension_conflict", Message: xe.Error(), Data: xe.Alternatives}
	}
	var qe *resusecase.QuoteError
	if errors.As(err, &qe) {
		return &usecase.ConflictError{Code: qe.Code, Detail: strings.ToLower(qe.Code), Message: qe.Message, Data: qe.Replacement}
	}
	var pe *resusecase.PricingError
	if errors.As(err, &pe) {
		return &usecase.ValidationError{Details: []usecase.Detail{{Field: field, Code: pe.Code, Message: pe.Message}}}
	}
	if _, ok := availusecase.IsCapacity(err); ok {
		return err
	}
	switch {
	case errors.Is(err, resusecase.ErrNotFound):
		return usecase.ErrNotFound
	case errors.Is(err, resusecase.ErrForbidden):
		return usecase.ErrForbidden
	case errors.Is(err, resusecase.ErrConcurrent):
		return usecase.ErrConcurrent
	}
	return err
}

// ---------------------------------------------------------------- io engine

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// IO exports rentals (list filters, branch scope of the job).
type IO struct {
	svc *usecase.Service
}

// NewIO builds the export adapter.
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

func (a *IO) Resource() string { return Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "rentals." + key, Type: t}
	}
	s, d := ioengine.ColumnTypeString, ioengine.ColumnTypeDatetime
	return []ioengine.Column{
		col("reference", s), col("status", s), col("customer", s), col("product", s), col("plate", s),
		col("pickup_branch", s), col("return_branch", s), col("actual_return_branch", s),
		col("planned_pickup_at", d), col("planned_return_at", d), col("actual_pickup_at", d), col("actual_return_at", d),
		col("late_minutes", ioengine.ColumnTypeString), col("uuid", ioengine.ColumnTypeUUID), col("created_at", d),
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := model.FilterFromQuery(query)
	f.SortField, f.SortDesc = "planned_pickup_at", false
	items, err := a.svc.ExportRows(ctx, orgID, f, usecase.MaxListTargets)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		actual := ""
		if it.ActualReturn != nil {
			actual = it.ActualReturn.Code + " " + it.ActualReturn.Name
		}
		row := map[string]any{
			"uuid": it.UUID.String(), "reference": it.Reference, "status": it.Status, "customer": it.Customer.DisplayName,
			"product": it.Product.Code + " " + it.Product.Name, "plate": it.Vehicle.Plate,
			"pickup_branch": it.PickupBranch.Code + " " + it.PickupBranch.Name, "return_branch": it.ReturnBranch.Code + " " + it.ReturnBranch.Name,
			"actual_return_branch": actual, "planned_pickup_at": it.PlannedPickupAt, "planned_return_at": it.PlannedReturnAt,
			"late_minutes": it.LateMinutes, "created_at": it.CreatedAt,
		}
		if it.ActualPickupAt != nil {
			row["actual_pickup_at"] = *it.ActualPickupAt
		}
		if it.ActualReturnAt != nil {
			row["actual_return_at"] = *it.ActualReturnAt
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *IO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *IO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *IO) RevertRow(context.Context, string, string, map[string]any) error { return errNoImport }
