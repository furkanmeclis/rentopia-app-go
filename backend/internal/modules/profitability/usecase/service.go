// Package usecase computes the vehicle profitability report (TEC-727,
// §11.5, docs/design/vehicle-profitability.md): per vehicle and period,
// realized income (rent, extras, charges, toll fees), commissions, direct
// costs, allocated shared overhead and the operating contribution, with
// refunds, deposits, pass-through tolls and vehicle procurement kept apart,
// on an accrual or a cash basis.
//
// The report owns no table. Its queries (queries/profitability.sql) read
// the ledger, collections, expenses, supplier payments, rental legs,
// blocks, maintenance and transfers of the caller's organization; the
// attribution is done in Go (engine.go) at query time.
package usecase

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxPeriodDays caps the period (one year, like the other finance
// reports).
const MaxPeriodDays = 366

// ErrVehicleNotFound answers a vehicle of another organization or outside
// the caller's branch scope.
var ErrVehicleNotFound = errors.New("vehicle not found")

// FieldError is one invalid filter.
type FieldError struct {
	Field, Code, Message string
}

// ValidationError lists invalid filters (400).
type ValidationError struct{ Details []FieldError }

func (e *ValidationError) Error() string {
	if len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func invalid(field, code, msg string) error {
	return &ValidationError{Details: []FieldError{{Field: field, Code: code, Message: msg}}}
}

// Service computes the report.
type Service struct {
	q     db.Querier
	clock func() time.Time
}

// New builds the service.
func New(q db.Querier) *Service { return &Service{q: q, clock: time.Now} }

// SetClock replaces the clock (tests).
func (s *Service) SetClock(clock func() time.Time) { s.clock = clock }

// Period resolves the period: the current month to today by default
// (Europe/Istanbul), swapped when reversed, capped at MaxPeriodDays (the
// start moves).
func (s *Service) Period(from, to *time.Time) (rentaltime.Date, rentaltime.Date) {
	today := rentaltime.LocalDate(s.clock())
	f := rentaltime.NewDate(today.Year, today.Month, 1)
	t := today
	if from != nil {
		f = rentaltime.DateOf(*from)
	}
	if to != nil {
		t = rentaltime.DateOf(*to)
	}
	if t.Before(f) {
		f, t = t, f
	}
	if t.Time().Sub(f.Time()) >= MaxPeriodDays*24*time.Hour {
		f = t.AddDays(-(MaxPeriodDays - 1))
	}
	return f, t
}

func (s *Service) normalize(f *model.Filter) error {
	if f.Basis == "" {
		f.Basis = model.BasisAccrual
	}
	if f.Allocation == "" {
		f.Allocation = model.AllocationRentalDays
	}
	if !contains(model.Bases, f.Basis) {
		return invalid("basis", "invalid", "basis must be accrual or cash")
	}
	if !contains(model.Allocations, f.Allocation) {
		return invalid("allocation", "invalid", "allocation must be rental_days, vehicle_count or revenue")
	}
	return nil
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// computed is the engine result with the row selection applied.
type computed struct {
	from, to rentaltime.Date
	now      time.Time
	method   model.Method
	res      result
	vehicles []vehicleFact
	rows     []vehicleFact
}

// Report is the fleet report for the caller in ctx (orgctx scope).
func (s *Service) Report(ctx context.Context, orgID int64, f model.Filter) (model.Report, error) {
	c, err := s.compute(ctx, orgID, f)
	if err != nil {
		return model.Report{}, err
	}
	out := model.Report{
		From: c.from.String(), To: c.to.String(), Method: c.method, Rows: []model.Row{},
	}
	trend := make([]acc, len(c.res.buckets))
	var tot acc
	rentals := 0
	for _, v := range c.rows {
		r := c.res.vehicles[v.id]
		var sum acc
		for b, a := range r.buckets {
			sum = add(sum, a)
			trend[b] = add(trend[b], a)
		}
		tot = add(tot, sum)
		rentals += len(r.rentals)
		out.Rows = append(out.Rows, model.Row{Vehicle: v.ref, Figures: figures(sum, len(r.rentals))})
	}
	out.Totals = figures(tot, rentals)
	out.Trend = trendPoints(c.res.buckets, trend)
	return out, nil
}

// Vehicle is the profitability of one vehicle (vehicle detail tab).
func (s *Service) Vehicle(ctx context.Context, orgID int64, id uuid.UUID, f model.Filter) (model.VehicleReport, error) {
	f.VehicleUUID = &id
	f.BranchUUIDs, f.RentalGroupUUIDs, f.Q = nil, nil, ""
	c, err := s.compute(ctx, orgID, f)
	if err != nil {
		return model.VehicleReport{}, err
	}
	var v *vehicleFact
	for i := range c.vehicles {
		if c.vehicles[i].ref.UUID == id {
			v = &c.vehicles[i]
		}
	}
	if v == nil || !visible(ctx, *v) {
		return model.VehicleReport{}, ErrVehicleNotFound
	}
	r := c.res.vehicles[v.id]
	var sum acc
	for _, a := range r.buckets {
		sum = add(sum, a)
	}
	out := model.VehicleReport{
		From: c.from.String(), To: c.to.String(), Method: c.method, Vehicle: v.ref,
		Figures: figures(sum, len(r.rentals)), Trend: trendPoints(c.res.buckets, r.buckets), Categories: []model.CategoryAmount{},
	}
	for _, cat := range r.categories {
		if cat.amount != 0 {
			out.Categories = append(out.Categories, model.CategoryAmount{Code: cat.code, Name: cat.name, Amount: money.New(cat.amount)})
		}
	}
	sort.Slice(out.Categories, func(i, j int) bool {
		a, b := out.Categories[i], out.Categories[j]
		if a.Amount.Minor() != b.Amount.Minor() {
			return a.Amount.Minor() > b.Amount.Minor()
		}
		return a.Code < b.Code
	})
	return out, nil
}

// visible: the vehicle's home or current branch is in the caller's scope
// (the vehicles module rule).
func visible(ctx context.Context, v vehicleFact) bool {
	return orgctx.CheckBranch(ctx, v.home) == nil || orgctx.CheckBranch(ctx, v.current) == nil
}

func (s *Service) compute(ctx context.Context, orgID int64, f model.Filter) (computed, error) {
	if err := s.normalize(&f); err != nil {
		return computed{}, err
	}
	branchSet, err := s.branchFilter(ctx, orgID, f.BranchUUIDs)
	if err != nil {
		return computed{}, err
	}
	groupVehicles, err := s.groupFilter(ctx, orgID, f.RentalGroupUUIDs)
	if err != nil {
		return computed{}, err
	}
	now := s.clock().UTC().Truncate(time.Second)
	from, to := s.Period(f.From, f.To)
	in, err := s.load(ctx, orgID, f.Basis, from, to, now)
	if err != nil {
		return computed{}, err
	}
	in.allocation = f.Allocation
	scope, _ := orgctx.From(ctx)
	in.scopeAll = scope.AllBranches()
	in.scopeBranches = map[int64]bool{}
	for _, id := range scope.BranchIDs {
		in.scopeBranches[id] = true
	}
	res := compute(in)
	c := computed{from: from, to: to, now: now, res: res, vehicles: in.vehicles}
	c.method = model.Method{
		Basis: f.Basis, Allocation: f.Allocation, Timezone: "Europe/Istanbul", Currency: "TRY", VATInclusive: true,
		OverheadScope: model.OverheadAll, AllocationFallback: res.fallback, UnallocatedOverhead: money.New(res.unallocated),
	}
	if !in.scopeAll {
		c.method.OverheadScope = model.OverheadBranches
	}
	q := strings.ToLower(strings.TrimSpace(f.Q))
	for _, v := range in.vehicles {
		if !visible(ctx, v) {
			continue
		}
		if branchSet != nil && !branchSet[v.home] {
			continue
		}
		if groupVehicles != nil && !groupVehicles[v.id] {
			continue
		}
		if f.VehicleUUID != nil && v.ref.UUID != *f.VehicleUUID {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.ref.Plate+" "+plateKey(v.ref.Plate)+" "+v.ref.Label), q) {
			continue
		}
		if !active(res.vehicles[v.id]) {
			continue
		}
		c.rows = append(c.rows, v)
	}
	return c, nil
}

func plateKey(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r != ' ' && r != '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// active: the vehicle was in service or moved money in the period.
func active(v *vehicleResult) bool {
	if v == nil {
		return false
	}
	if len(v.rentals) > 0 {
		return true
	}
	for _, a := range v.buckets {
		if a.available > 0 || a.rented > 0 {
			return true
		}
		for _, m := range a.money {
			if m != 0 {
				return true
			}
		}
	}
	return false
}

// branchFilter resolves the branch filter: an unknown branch, or one
// outside the caller's scope, is not_found (the same answer for both).
func (s *Service) branchFilter(ctx context.Context, orgID int64, ids []uuid.UUID) (map[int64]bool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q.ProfitBranchesByUUID(ctx, db.ProfitBranchesByUUIDParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	set := map[int64]bool{}
	found := map[uuid.UUID]bool{}
	for _, r := range rows {
		if orgctx.CheckBranch(ctx, r.ID) != nil {
			continue
		}
		set[r.ID] = true
		found[r.Uuid] = true
	}
	for _, id := range ids {
		if !found[id] {
			return nil, invalid("branch", "not_found", "branch not found")
		}
	}
	return set, nil
}

func (s *Service) groupFilter(ctx context.Context, orgID int64, ids []uuid.UUID) (map[int64]bool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q.ProfitRentalGroupsByUUID(ctx, db.ProfitRentalGroupsByUUIDParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	found := map[uuid.UUID]bool{}
	groupIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		found[r.Uuid] = true
		groupIDs = append(groupIDs, r.ID)
	}
	for _, id := range ids {
		if !found[id] {
			return nil, invalid("rental_group", "not_found", "rental group not found")
		}
	}
	vids, err := s.q.ProfitVehicleIDsInGroups(ctx, db.ProfitVehicleIDsInGroupsParams{OrganizationID: orgID, GroupIds: groupIDs})
	if err != nil {
		return nil, err
	}
	set := map[int64]bool{}
	for _, id := range vids {
		set[id] = true
	}
	return set, nil
}

// farFuture closes open service lives and legs.
var farFuture = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

func kurus(n pgtype.Numeric) int64 { return money.FromNumericOrZero(n).Minor() }

func dateOf(d pgtype.Date) rentaltime.Date { return rentaltime.DateOf(d.Time) }

func pgDate(d rentaltime.Date) pgtype.Date { return pgtype.Date{Time: d.Time(), Valid: true} }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func optInt(v pgtype.Int8) int64 {
	if !v.Valid {
		return 0
	}
	return v.Int64
}

// load reads the facts of the organization for the period.
func (s *Service) load(ctx context.Context, orgID int64, basis string, from, to rentaltime.Date, now time.Time) (input, error) {
	in := input{
		basis: basis, from: from, to: to, now: now,
		rentals: map[int64]rentalFact{}, legs: map[int64][]legFact{}, billed: map[int64]billed{},
		paymentComm: map[int64]int64{}, blocks: map[int64][]interval{},
	}
	start, end := from.StartIn().UTC(), to.AddDays(1).StartIn().UTC()

	vehicles, err := s.q.ProfitVehicles(ctx, orgID)
	if err != nil {
		return in, err
	}
	for _, v := range vehicles {
		svcStart := rentaltime.LocalDate(v.CreatedAt.Time).StartIn().UTC()
		if v.InServiceDate.Valid {
			svcStart = dateOf(v.InServiceDate).StartIn().UTC()
		}
		svcEnd := farFuture
		if v.OutOfServiceDate.Valid {
			svcEnd = dateOf(v.OutOfServiceDate).AddDays(1).StartIn().UTC()
		}
		if v.DeletedAt.Valid && v.DeletedAt.Time.Before(svcEnd) {
			svcEnd = v.DeletedAt.Time.UTC()
		}
		in.vehicles = append(in.vehicles, vehicleFact{
			id: v.ID, home: v.HomeBranchID, current: v.CurrentBranchID, service: interval{svcStart, svcEnd},
			ref: model.VehicleRef{UUID: v.Uuid, Plate: v.Plate, Label: v.Label, HomeBranch: model.BranchRef{
				UUID: v.HomeBranchUuid, Code: v.HomeBranchCode, Name: v.HomeBranchName,
			}},
		})
	}

	rentalIDs := map[int64]bool{}
	overlapping, err := s.q.ProfitLegsOverlapping(ctx, db.ProfitLegsOverlappingParams{OrganizationID: orgID, FromTs: ts(start), ToTs: ts(end)})
	if err != nil {
		return in, err
	}
	for _, l := range overlapping {
		rentalIDs[l.RentalID] = true
	}
	facts, err := s.q.ProfitLedgerFacts(ctx, db.ProfitLedgerFactsParams{OrganizationID: orgID, DateFrom: pgDate(from), DateTo: pgDate(to)})
	if err != nil {
		return in, err
	}
	for _, e := range facts {
		rentalIDs[e.RentalID] = true
		in.ledger = append(in.ledger, ledgerFact{
			id: e.ID, kind: e.Kind, direction: e.Direction, purpose: e.Purpose, origKind: e.OriginalKind,
			origPurpose: e.OriginalPurpose, ckind: e.ChargeKind, amount: kurus(e.Amount), day: dateOf(e.EntryDate), rental: e.RentalID,
		})
	}
	ids := make([]int64, 0, len(rentalIDs))
	for id := range rentalIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > 0 {
		rentals, err := s.q.ProfitRentals(ctx, db.ProfitRentalsParams{OrganizationID: orgID, RentalIds: ids})
		if err != nil {
			return in, err
		}
		for _, r := range rentals {
			rf := rentalFact{id: r.ID, vehicle: r.VehicleID, endsAt: r.EndsAt.Time.UTC(), total: kurus(r.TotalAmount), extras: kurus(r.ExtrasAmount)}
			if r.CollectingParty == "us" {
				rf.channelComm = kurus(r.CommissionAmount)
			}
			in.rentals[r.ID] = rf
		}
		legs, err := s.q.ProfitLegsForRentals(ctx, db.ProfitLegsForRentalsParams{OrganizationID: orgID, RentalIds: ids})
		if err != nil {
			return in, err
		}
		for _, l := range legs {
			if !l.OutAt.Valid {
				continue
			}
			in.legs[l.RentalID] = append(in.legs[l.RentalID], legFact{rental: l.RentalID, vehicle: l.VehicleID, out: l.OutAt.Time.UTC(), in: timePtr(l.InAt)})
		}
		bl, err := s.q.ProfitBilledAccruals(ctx, db.ProfitBilledAccrualsParams{OrganizationID: orgID, RentalIds: ids})
		if err != nil {
			return in, err
		}
		for _, b := range bl {
			cur := in.billed[b.RentalID]
			amt := kurus(b.Amount)
			switch {
			case b.Purpose == "rent":
				cur.rentTotal += amt
			case b.ChargeKind == "toll":
				cur.toll += amt
			case b.ChargeKind == "toll_fee":
				cur.tollFee += amt
			default:
				cur.charges += amt
			}
			in.billed[b.RentalID] = cur
		}
		pc, err := s.q.ProfitPaymentCommissions(ctx, db.ProfitPaymentCommissionsParams{OrganizationID: orgID, RentalIds: ids})
		if err != nil {
			return in, err
		}
		for _, p := range pc {
			in.paymentComm[p.RentalID] += kurus(p.Amount)
		}
	}

	expenses, err := s.q.ProfitExpenses(ctx, db.ProfitExpensesParams{OrganizationID: orgID, DateFrom: pgDate(from), DateTo: pgDate(to)})
	if err != nil {
		return in, err
	}
	for _, e := range expenses {
		// Cash basis: a supplier invoice's expense is dated by the payments
		// allocated to it (below), not by the invoice date.
		if basis == model.BasisCash && e.SupplierInvoiceID.Valid {
			continue
		}
		in.expenses = append(in.expenses, costFact{
			vehicle: optInt(e.VehicleID), branch: optInt(e.BranchID), amount: kurus(e.TotalAmount), day: dateOf(e.ExpenseDate),
			code: e.CategoryCode, name: e.CategoryName,
		})
	}
	if basis == model.BasisCash {
		pays, err := s.q.ProfitSupplierPayments(ctx, db.ProfitSupplierPaymentsParams{OrganizationID: orgID, DateFrom: pgDate(from), DateTo: pgDate(to)})
		if err != nil {
			return in, err
		}
		for _, p := range pays {
			in.expenses = append(in.expenses, costFact{
				vehicle: optInt(p.VehicleID), branch: optInt(p.BranchID), amount: kurus(p.Amount), day: dateOf(p.PaymentDate),
				code: p.CategoryCode, name: p.CategoryName,
			})
		}
	}
	maint, err := s.q.ProfitMaintenance(ctx, db.ProfitMaintenanceParams{OrganizationID: orgID, DateFrom: pgDate(from), DateTo: pgDate(to)})
	if err != nil {
		return in, err
	}
	for _, m := range maint {
		in.maintenance = append(in.maintenance, costFact{vehicle: m.VehicleID, amount: kurus(m.Cost), day: dateOf(m.DoneDate)})
	}
	transfers, err := s.q.ProfitTransfers(ctx, db.ProfitTransfersParams{OrganizationID: orgID, FromTs: ts(start), ToTs: ts(end)})
	if err != nil {
		return in, err
	}
	for _, t := range transfers {
		in.transfers = append(in.transfers, costFact{vehicle: t.VehicleID, amount: kurus(t.Cost), day: rentaltime.LocalDate(t.ActualArrival.Time)})
	}
	blocks, err := s.q.ProfitBlocks(ctx, db.ProfitBlocksParams{OrganizationID: orgID, FromTs: ts(start), ToTs: ts(end)})
	if err != nil {
		return in, err
	}
	for _, b := range blocks {
		in.blocks[b.VehicleID] = append(in.blocks[b.VehicleID], interval{b.StartsAt.Time.UTC(), b.EndsAt.Time.UTC()})
	}
	return in, nil
}

func add(a, b acc) acc {
	for i := range a.money {
		a.money[i] += b.money[i]
	}
	a.rented += b.rented
	a.available += b.available
	return a
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// figures turns an accumulator into the API figures.
func figures(a acc, rentals int) model.Figures {
	m := func(slot int) money.Money { return money.New(a.money[slot]) }
	gross := a.money[fRent] + a.money[fExtras] + a.money[fCharges] + a.money[fTollFees]
	comm := a.money[fPayComm] + a.money[fChanComm]
	direct := a.money[fExpenses] + a.money[fMaint] + a.money[fTransfers]
	contribution := gross - comm - direct - a.money[fOverhead]
	f := model.Figures{
		Rent: m(fRent), Extras: m(fExtras), Charges: m(fCharges), TollFees: m(fTollFees), GrossIncome: money.New(gross),
		PaymentCommission: m(fPayComm), ChannelCommission: m(fChanComm), Commissions: money.New(comm),
		Expenses: m(fExpenses), Maintenance: m(fMaint), Transfers: m(fTransfers), DirectCosts: money.New(direct),
		Overhead: m(fOverhead), Contribution: money.New(contribution),
		Refunded: m(fRefunded), DepositsHeld: m(fHeld), DepositsReturned: m(fReturned), PassThrough: m(fPass), Capex: m(fCapex),
		RentalDays:    round(float64(a.rented)/86400, 2),
		AvailableDays: round(float64(a.available)/86400, 2),
		RentalCount:   rentals,
	}
	if gross != 0 {
		v := round(float64(contribution)*100/float64(gross), 1)
		f.Margin = &v
	}
	if a.available > 0 {
		v := round(float64(a.rented)*100/float64(a.available), 1)
		f.Occupancy = &v
	}
	return f
}

func trendPoints(buckets []bucket, accs []acc) []model.TrendPoint {
	out := make([]model.TrendPoint, 0, len(buckets))
	for i, b := range buckets {
		out = append(out, model.TrendPoint{Month: b.key, From: b.from.String(), To: b.to.String(), Figures: figures(accs[i], 0)})
	}
	return out
}
