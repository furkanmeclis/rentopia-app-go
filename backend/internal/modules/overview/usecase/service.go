// Package usecase builds the operations overview of the tenant panel home
// page (TEC-716, docs/design/operations-overview.md): KPIs and the
// sections that need attention today, each computed only when the caller's
// tenant role grants the permission behind it, and every one limited to the
// caller's branch scope (optionally narrowed to one branch).
//
// The overview owns no table. Its queries (queries/overview.sql) read the
// reservations, rentals, operation tasks, payments, integration jobs and
// documents tables with the visibility rule of the module that owns each
// one.
package usecase

import (
	"context"
	"errors"
	"math"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// TodayLimit caps today's handover / return rows (the panel's table).
	TodayLimit = 100
	// CardLimit caps the rows of the compact cards ("Tümünü gör" opens the
	// full list).
	CardLimit = 5
	// DocumentWindow is how far ahead unapproved contracts are listed.
	DocumentWindow = 48 * time.Hour
	// lastErrorMax trims job errors in the card.
	lastErrorMax = 200
)

// Reservation statuses of the waiting sections.
var (
	approvalStatuses = []string{"request", "awaiting_review"}
	paymentStatuses  = []string{"awaiting_payment"}
)

// ErrBranchNotFound answers a branch filter that is unknown in the
// organization or outside the caller's branch scope (the same answer for
// both, so out-of-scope branches do not leak).
var ErrBranchNotFound = errors.New("branch not found")

// Service computes the overview.
type Service struct {
	q     db.Querier
	clock func() time.Time
}

// New builds the service.
func New(q db.Querier) *Service {
	return &Service{q: q, clock: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(clock func() time.Time) { s.clock = clock }

// DayWindow returns the Europe/Istanbul calendar day of now.
func DayWindow(now time.Time) Window {
	local := rentaltime.Local(now)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, rentaltime.Location())
	return Window{Start: start.UTC(), End: start.AddDate(0, 0, 1).UTC()}
}

// filter resolves the effective branch filter: the caller's branch scope,
// narrowed to branchUUID when given. A branch outside the scope is
// ErrBranchNotFound.
func (s *Service) filter(ctx context.Context, orgID int64, branchUUID *uuid.UUID) ([]int64, *BranchRef, error) {
	scope := orgctx.BranchScope(ctx)
	if branchUUID == nil {
		return scope, nil, nil
	}
	b, err := s.q.OverviewBranchByUUID(ctx, db.OverviewBranchByUUIDParams{OrganizationID: orgID, Uuid: *branchUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrBranchNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if orgctx.CheckBranch(ctx, b.ID) != nil {
		return nil, nil, ErrBranchNotFound
	}
	return []int64{b.ID}, &BranchRef{UUID: b.Uuid, Code: b.Code, Name: b.Name}, nil
}

// Overview builds the overview for the caller in ctx (orgctx scope).
func (s *Service) Overview(ctx context.Context, orgID int64, branchUUID *uuid.UUID) (Overview, error) {
	scope, _ := orgctx.From(ctx)
	branchIDs, branch, err := s.filter(ctx, orgID, branchUUID)
	if err != nil {
		return Overview{}, err
	}
	now := s.clock().UTC().Truncate(time.Second)
	day := DayWindow(now)
	out := Overview{
		GeneratedAt: now,
		Today:       day,
		BranchScope: orgctx.BranchScopeAll,
		Branch:      branch,
		Branches:    []BranchRef{},
	}
	if !scope.AllBranches() {
		out.BranchScope = orgctx.BranchScopeSelected
	}
	branches, err := s.q.OverviewBranches(ctx, db.OverviewBranchesParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return Overview{}, err
	}
	for _, b := range branches {
		out.Branches = append(out.Branches, BranchRef{UUID: b.Uuid, Code: b.Code, Name: b.Name})
	}

	has := scope.Has
	steps := []struct {
		allowed bool
		run     func() error
	}{
		{has(rbac.PermTenantRentalsRead) && has(rbac.PermTenantVehiclesRead), func() error {
			out.KPIs.Occupancy, err = s.occupancy(ctx, orgID, branchIDs, day)
			return err
		}},
		{has(rbac.PermTenantRentalsRead), func() error {
			out.KPIs.ActiveRentals, err = s.activeRentals(ctx, orgID, branchIDs, now)
			return err
		}},
		{has(rbac.PermTenantPaymentsRead), func() error {
			out.KPIs.Collections, err = s.collections(ctx, orgID, branchIDs, day)
			return err
		}},
		{has(rbac.PermTenantRentalsRead), func() error {
			out.TodayOperations, err = s.todayOperations(ctx, orgID, branchIDs, day)
			return err
		}},
		{has(rbac.PermTenantRentalsRead), func() error {
			out.OverdueReturns, err = s.overdueReturns(ctx, orgID, branchIDs, now)
			return err
		}},
		{has(rbac.PermTenantReservationsRead) && has(rbac.PermTenantReservationsApprove), func() error {
			out.Approvals, err = s.waiting(ctx, orgID, branchIDs, now, approvalStatuses)
			return err
		}},
		{has(rbac.PermTenantReservationsRead), func() error {
			out.AwaitingPayment, err = s.waiting(ctx, orgID, branchIDs, now, paymentStatuses)
			return err
		}},
		{has(rbac.PermTenantResolutionTasksRead), func() error {
			out.Availability, err = s.resolutionTasks(ctx, orgID, branchIDs)
			return err
		}},
		{has(rbac.PermTenantIntegrationsManage), func() error {
			out.IntegrationJobs, err = s.integrationJobs(ctx, orgID)
			return err
		}},
		{has(rbac.PermTenantContractsRead), func() error {
			out.Documents, err = s.documents(ctx, orgID, branchIDs, now, day)
			return err
		}},
	}
	for _, step := range steps {
		if !step.allowed {
			continue
		}
		if err := step.run(); err != nil {
			return Overview{}, err
		}
	}
	return out, nil
}

// Badges returns the sidebar counts for the caller's branch scope.
func (s *Service) Badges(ctx context.Context, orgID int64) (Badges, error) {
	scope, _ := orgctx.From(ctx)
	approvals := scope.Has(rbac.PermTenantReservationsRead) && scope.Has(rbac.PermTenantReservationsApprove)
	tasks := scope.Has(rbac.PermTenantResolutionTasksRead)
	var out Badges
	if !approvals && !tasks {
		return out, nil
	}
	row, err := s.q.OverviewBadgeCounts(ctx, db.OverviewBadgeCountsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Now: ts(s.clock()),
	})
	if err != nil {
		return Badges{}, err
	}
	if approvals {
		out.Approvals = &ApprovalBadge{Pending: row.Approvals, Overdue: row.ApprovalsOverdue}
	}
	if tasks {
		out.ResolutionTasks = &ResolutionBadge{Open: row.ResolutionTasks}
	}
	return out, nil
}

func (s *Service) occupancy(ctx context.Context, orgID int64, branchIDs []int64, day Window) (*Occupancy, error) {
	local := rentaltime.Local(day.Start)
	row, err := s.q.OverviewFleetUtilisation(ctx, db.OverviewFleetUtilisationParams{
		OrganizationID: orgID, BranchIds: branchIDs,
		Today: pgtype.Date{Time: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := &Occupancy{Rented: row.Rented, Rentable: row.Rentable}
	if row.Rentable > 0 {
		out.Rate = math.Round(float64(row.Rented)*1000/float64(row.Rentable)) / 10
	}
	return out, nil
}

func (s *Service) activeRentals(ctx context.Context, orgID int64, branchIDs []int64, now time.Time) (*ActiveRentals, error) {
	row, err := s.q.OverviewActiveRentals(ctx, db.OverviewActiveRentalsParams{OrganizationID: orgID, BranchIds: branchIDs, Now: ts(now)})
	if err != nil {
		return nil, err
	}
	return &ActiveRentals{Active: row.Active, Overdue: row.Overdue}, nil
}

func (s *Service) collections(ctx context.Context, orgID int64, branchIDs []int64, day Window) (*Collections, error) {
	paid, err := s.q.OverviewCollections(ctx, db.OverviewCollectionsParams{
		OrganizationID: orgID, BranchIds: branchIDs, DayStart: ts(day.Start), DayEnd: ts(day.End),
	})
	if err != nil {
		return nil, err
	}
	refunds, err := s.q.OverviewRefunds(ctx, db.OverviewRefundsParams{
		OrganizationID: orgID, BranchIds: branchIDs, DayStart: ts(day.Start), DayEnd: ts(day.End),
	})
	if err != nil {
		return nil, err
	}
	amount := money.FromNumericOrZero(paid.Amount)
	refunded := money.FromNumericOrZero(refunds.Amount)
	return &Collections{
		Currency: string(money.TRY), Count: paid.Count, Amount: amount, Refunded: refunded, Net: amount.Sub(refunded),
	}, nil
}

func (s *Service) todayOperations(ctx context.Context, orgID int64, branchIDs []int64, day Window) (*TodayOperations, error) {
	counts, err := s.q.OverviewTodayOperationCounts(ctx, db.OverviewTodayOperationCountsParams{
		OrganizationID: orgID, BranchIds: branchIDs, DayStart: ts(day.Start), DayEnd: ts(day.End),
	})
	if err != nil {
		return nil, err
	}
	out := &TodayOperations{Items: []OperationItem{}}
	for _, c := range counts {
		target := &out.Handovers
		if c.TaskType == "return" {
			target = &out.Returns
		}
		target.Total += c.Count
		if c.Status == "completed" {
			target.Done += c.Count
		} else {
			target.Open += c.Count
		}
		out.Total += c.Count
	}
	rows, err := s.q.OverviewTodayOperations(ctx, db.OverviewTodayOperationsParams{
		OrganizationID: orgID, BranchIds: branchIDs, DayStart: ts(day.Start), DayEnd: ts(day.End), LimitCount: TodayLimit,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Items = append(out.Items, OperationItem{
			UUID: r.Uuid, TaskType: r.TaskType, Status: r.Status, DueAt: r.DueAt.Time, CompletedAt: timePtr(r.CompletedAt),
			Branch:          BranchRef{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
			Rental:          RentalRef{UUID: r.RentalUuid, Reference: r.RentalReference, Status: r.RentalStatus},
			ReservationUUID: r.ReservationUuid, Plate: r.VehiclePlate,
			CustomerName: displayName(r.CustomerType, r.CustomerFirstName, r.CustomerLastName, r.CustomerCompanyTitle),
		})
	}
	return out, nil
}

func (s *Service) overdueReturns(ctx context.Context, orgID int64, branchIDs []int64, now time.Time) (*OverdueReturns, error) {
	rows, err := s.q.OverviewOverdueRentals(ctx, db.OverviewOverdueRentalsParams{
		OrganizationID: orgID, BranchIds: branchIDs, Now: ts(now), LimitCount: CardLimit,
	})
	if err != nil {
		return nil, err
	}
	out := &OverdueReturns{Items: []OverdueItem{}}
	for _, r := range rows {
		out.Total = r.Total
		late := int64(now.Sub(r.PlannedReturnAt.Time) / time.Minute)
		if late < 0 {
			late = 0
		}
		out.Items = append(out.Items, OverdueItem{
			UUID: r.Uuid, Reference: r.Reference, ReservationUUID: r.ReservationUuid,
			PlannedReturnAt: r.PlannedReturnAt.Time, OverdueAt: timePtr(r.OverdueAt), LateMinutes: late,
			ReturnBranch: BranchRef{UUID: r.ReturnBranchUuid, Code: r.ReturnBranchCode, Name: r.ReturnBranchName},
			Plate:        r.VehiclePlate,
			CustomerName: displayName(r.CustomerType, r.CustomerFirstName, r.CustomerLastName, r.CustomerCompanyTitle),
		})
	}
	return out, nil
}

func (s *Service) waiting(ctx context.Context, orgID int64, branchIDs []int64, now time.Time, statuses []string) (*WaitingSection, error) {
	rows, err := s.q.OverviewWaitingReservations(ctx, db.OverviewWaitingReservationsParams{
		OrganizationID: orgID, BranchIds: branchIDs, Now: ts(now), Statuses: statuses, LimitCount: CardLimit,
	})
	if err != nil {
		return nil, err
	}
	out := &WaitingSection{Items: []WaitingItem{}}
	for _, r := range rows {
		out.Total, out.Overdue = r.Total, r.Overdue
		deadline := r.ReviewDeadline
		if r.Status == "awaiting_payment" {
			deadline = r.PaymentDeadline
		}
		due := money.FromNumericOrZero(r.PrepaymentAmount).Sub(money.FromNumericOrZero(r.PaidAmount)).ClampZero()
		out.Items = append(out.Items, WaitingItem{
			UUID: r.Uuid, Reference: r.Reference, Status: r.Status, Channel: r.Channel,
			StartsAt: r.StartsAt.Time, EndsAt: r.EndsAt.Time, Deadline: timePtr(deadline),
			PickupBranch: BranchRef{UUID: r.PickupBranchUuid, Code: r.PickupBranchCode, Name: r.PickupBranchName},
			CustomerName: displayName(r.CustomerType, r.CustomerFirstName, r.CustomerLastName, r.CustomerCompanyTitle),
			Currency:     r.Currency, TotalAmount: money.FromNumericOrZero(r.TotalAmount), AmountDue: due,
		})
	}
	return out, nil
}

func (s *Service) resolutionTasks(ctx context.Context, orgID int64, branchIDs []int64) (*ResolutionTasks, error) {
	counts, err := s.q.OverviewResolutionTaskCounts(ctx, db.OverviewResolutionTaskCountsParams{OrganizationID: orgID, BranchIds: branchIDs})
	if err != nil {
		return nil, err
	}
	out := &ResolutionTasks{ByType: map[string]int64{}, Items: []ResolutionItem{}}
	for _, c := range counts {
		out.ByType[c.TaskType] = c.Count
		out.Total += c.Count
	}
	if out.Total == 0 {
		return out, nil
	}
	rows, err := s.q.OverviewOpenResolutionTasks(ctx, db.OverviewOpenResolutionTasksParams{
		OrganizationID: orgID, BranchIds: branchIDs, LimitCount: CardLimit,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Items = append(out.Items, ResolutionItem{
			UUID: r.Uuid, TaskType: r.TaskType, Reason: r.Reason, CreatedAt: r.CreatedAt.Time,
			Reservation:  ReservationRef{UUID: r.ReservationUuid, Reference: r.ReservationReference, StartsAt: r.ReservationStartsAt.Time},
			PickupBranch: BranchRef{UUID: r.PickupBranchUuid, Code: r.PickupBranchCode, Name: r.PickupBranchName},
		})
	}
	return out, nil
}

func (s *Service) integrationJobs(ctx context.Context, orgID int64) (*IntegrationJobs, error) {
	rows, err := s.q.OverviewAttentionJobs(ctx, db.OverviewAttentionJobsParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, LimitCount: CardLimit,
	})
	if err != nil {
		return nil, err
	}
	out := &IntegrationJobs{Items: []JobItem{}}
	for _, r := range rows {
		out.Total = r.Total
		item := JobItem{
			UUID: r.Uuid, Direction: r.Direction, Provider: r.Provider, Kind: r.Kind, Status: r.Status,
			Attempts: r.Attempts, LastError: trim(r.LastError, lastErrorMax), RefType: r.RefType,
			RefUUID: uuidPtr(r.RefUuid), UpdatedAt: r.UpdatedAt.Time,
		}
		if r.AccountUuid.Valid {
			item.Account = &AccountRef{UUID: r.AccountUuid.Bytes, Kind: r.AccountKind.String, Name: r.AccountName.String}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Service) documents(ctx context.Context, orgID int64, branchIDs []int64, now time.Time, day Window) (*PendingDocuments, error) {
	window := Window{Start: day.Start, End: now.Add(DocumentWindow)}
	out := &PendingDocuments{WindowHours: int(DocumentWindow / time.Hour), Window: window, Items: []DocumentItem{}}
	required, err := s.q.OverviewContractRequired(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out.Required = required
	if !required {
		return out, nil
	}
	rows, err := s.q.OverviewPendingContracts(ctx, db.OverviewPendingContractsParams{
		OrganizationID: orgID, BranchIds: branchIDs, WindowStart: ts(window.Start), WindowEnd: ts(window.End), LimitCount: CardLimit,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Total = r.Total
		item := DocumentItem{
			Reservation:  ReservationRef{UUID: r.Uuid, Reference: r.Reference, StartsAt: r.StartsAt.Time},
			PickupBranch: BranchRef{UUID: r.PickupBranchUuid, Code: r.PickupBranchCode, Name: r.PickupBranchName},
			CustomerName: displayName(r.CustomerType, r.CustomerFirstName, r.CustomerLastName, r.CustomerCompanyTitle),
		}
		if r.RentalUuid.Valid {
			item.Rental = &RentalRef{UUID: r.RentalUuid.Bytes, Reference: r.RentalReference.String}
		}
		if r.DocumentUuid.Valid {
			item.Document = &DocumentRef{UUID: r.DocumentUuid.Bytes, Status: r.DocumentStatus.String}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// displayName is the company title of a corporate customer, else the
// person's name (customers.DisplayName).
func displayName(customerType, first, last, title string) string {
	if customerType == "corporate" && title != "" {
		return title
	}
	name := first
	if last != "" {
		if name != "" {
			name += " "
		}
		name += last
	}
	return name
}

func trim(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "…"
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func uuidPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	v := uuid.UUID(u.Bytes)
	return &v
}
