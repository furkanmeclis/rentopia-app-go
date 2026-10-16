package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/model"
	pricingmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Rentals lists the reservations sold under the agreement (in the caller's
// branch scope) with their rental and what the ledger accrued of the rent.
func (s *Service) Rentals(ctx context.Context, orgID int64, id uuid.UUID) ([]model.RentalRow, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListAgreementReservations(ctx, db.ListAgreementReservationsParams{
		OrganizationID: orgID, AgreementID: int8Of(r.ID), BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.RentalRow, 0, len(rows))
	for _, x := range rows {
		total, accrued := money.FromNumericOrZero(x.TotalAmount), money.FromNumericOrZero(x.RentAccrued)
		item := model.RentalRow{
			ReservationUUID: x.Uuid, Reference: x.Reference, Status: x.Status, StartsAt: x.StartsAt.Time.UTC(), EndsAt: x.EndsAt.Time.UTC(),
			Product: model.Ref{UUID: x.ProductUuid, Code: x.ProductCode, Name: x.ProductTitle},
			Customer: model.CustomerRef{
				UUID: x.CustomerUuid, CustomerType: x.CustomerType,
				DisplayName: displayName(x.CustomerType, x.CustomerCompanyTitle, x.CustomerFirstName, x.CustomerLastName),
			},
			Total: total, Currency: x.Currency, OpenAccount: x.OpenAccount, RentAccrued: accrued,
			RentOpen: total.Sub(accrued).ClampZero(), Plate: x.VehiclePlate.String, VehicleUUID: uuidPtr(x.VehicleUuid),
		}
		if x.RentalUuid.Valid {
			item.Rental = &model.RentalRef{
				UUID: x.RentalUuid.Bytes, Status: x.RentalStatus.String, RentBilling: x.RentBilling.String,
				ActualPickupAt: timePtr(x.ActualPickupAt), ActualReturnAt: timePtr(x.ActualReturnAt),
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// Billing lists the billing plans of the agreement's periodically billed
// rentals: every period revision with its status and, once posted, its
// accrual entry (invoicing links to it, TEC-722). rental narrows to one
// rental.
func (s *Service) Billing(ctx context.Context, orgID int64, id uuid.UUID, rental *uuid.UUID) (model.BillingPlan, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.BillingPlan{}, err
	}
	var rentalArg pgtype.UUID
	if rental != nil {
		rentalArg = pgtype.UUID{Bytes: *rental, Valid: true}
	}
	rows, err := s.q.ListAgreementBillingPeriods(ctx, db.ListAgreementBillingPeriodsParams{
		OrganizationID: orgID, AgreementID: int8Of(r.ID), RentalUuid: rentalArg, BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return model.BillingPlan{}, err
	}
	out := model.BillingPlan{Periods: make([]model.Period, 0, len(rows)), Planned: money.Zero(), Posted: money.Zero()}
	interval := int32(0)
	if r.MaintenanceIntervalDays.Valid {
		interval = r.MaintenanceIntervalDays.Int32
	}
	for _, x := range rows {
		amount := money.FromNumericOrZero(x.Amount)
		p := model.Period{
			UUID: x.Uuid, Rental: model.Ref{UUID: x.RentalUuid, Name: x.RentalReference}, RentalStatus: x.RentalStatus,
			No: x.PeriodNo, Revision: x.Revision, StartsOn: rentaltime.DateOf(x.StartsOn.Time), EndsOn: rentaltime.DateOf(x.EndsOn.Time),
			Days: x.Days, Partial: x.Partial, Amount: amount, Currency: x.Currency, KmAllowance: int4Ptr(x.KmAllowance),
			MaintenanceDue: maintenanceDue(interval, x.DayFrom, x.DayTo), AccrualKey: x.AccrualKey, Status: x.Status,
			PostedAt: timePtr(x.PostedAt), ClosedAt: timePtr(x.ClosedAt), EntryUUID: uuidPtr(x.EntryUuid),
			EntryDate: datePtr(x.EntryDate), DueDate: datePtr(x.DueDate),
		}
		switch x.Status {
		case "planned":
			out.Planned = out.Planned.Add(amount)
		case "posted":
			out.Posted = out.Posted.Add(amount)
		}
		out.Periods = append(out.Periods, p)
	}
	return out, nil
}

// maintenanceDue reports whether a maintenance falls in the billing days
// [from, to): every interval days from the rental start (day interval,
// 2·interval, …).
func maintenanceDue(interval, from, to int32) bool {
	if interval <= 0 {
		return false
	}
	// The first multiple of interval at or after from (day 0 excluded).
	k := (from + interval - 1) / interval
	if k == 0 {
		k = 1
	}
	return k*interval < to
}

// Statement lists the ledger entries of the agreement's reservations and
// rentals (deposits aside, like balances) with a running balance.
func (s *Service) Statement(ctx context.Context, orgID int64, id uuid.UUID) (model.Statement, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.Statement{}, err
	}
	rows, err := s.q.ListAgreementLedgerEntries(ctx, db.ListAgreementLedgerEntriesParams{
		OrganizationID: orgID, AgreementID: int8Of(r.ID), BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return model.Statement{}, err
	}
	out := model.Statement{Rows: make([]model.StatementRow, 0, len(rows)), Debit: money.Zero(), Credit: money.Zero(), Balance: money.Zero()}
	for _, x := range rows {
		amount := money.FromNumericOrZero(x.Amount)
		row := model.StatementRow{
			EntryUUID: x.Uuid, EntryDate: rentaltime.DateOf(x.EntryDate.Time), DueDate: rentaltime.DateOf(x.DueDate.Time),
			Kind: x.Kind, Direction: x.Direction, Purpose: x.Purpose, Amount: amount, Debit: money.Zero(), Credit: money.Zero(),
			Description: x.Description, Reason: x.Reason, AccrualKey: x.AccrualKey.String, Periodic: x.BillingPeriodID.Valid,
			Reservation: model.Ref{UUID: x.ReservationUuid, Name: x.ReservationReference}, RentalUUID: uuidPtr(x.RentalUuid),
		}
		if x.Direction == "debit" {
			row.Debit = amount
			out.Debit = out.Debit.Add(amount)
			out.Balance = out.Balance.Add(amount)
		} else {
			row.Credit = amount
			out.Credit = out.Credit.Add(amount)
			out.Balance = out.Balance.Sub(amount)
		}
		row.Balance = out.Balance
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// Tariffs lists the agreement's rate plans (scope agreement). Editing them
// is the rate plan API (tenant.pricing.write / activate).
func (s *Service) Tariffs(ctx context.Context, orgID int64, id uuid.UUID) ([]pricingmodel.RatePlan, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return nil, err
	}
	if s.tariffs == nil {
		return []pricingmodel.RatePlan{}, nil
	}
	items, _, err := s.tariffs.ListPlans(ctx, orgID, pricingmodel.RatePlanFilter{
		AgreementUUIDs: []uuid.UUID{r.Uuid}, Limit: 200, SortField: "rental_product",
	})
	if items == nil {
		items = []pricingmodel.RatePlan{}
	}
	return items, err
}
