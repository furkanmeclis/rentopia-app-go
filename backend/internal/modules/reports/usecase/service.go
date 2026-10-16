// Package usecase computes the reports (TEC-738, §15.1).
// The reports own no table: occupancy, the branch
// comparison and the vehicle contribution reuse the vehicle profitability
// engine (TEC-727); income / expense, channels and aging read the ledger,
// expenses and reservations of the caller's organization through
// queries/reports.sql. Branch scope comes from orgctx.
package usecase

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	profitmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	profitusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxPeriodDays caps the period (one year, like the other finance
// reports).
const MaxPeriodDays = 366

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

// Profitability is the vehicle profitability engine (TEC-727).
type Profitability interface {
	Breakdown(ctx context.Context, orgID int64, f profitmodel.Filter) (profitmodel.Breakdown, error)
}

// Service computes the reports.
type Service struct {
	q      db.Querier
	profit Profitability
	clock  func() time.Time
}

// New builds the service.
func New(q db.Querier, profit Profitability) *Service {
	return &Service{q: q, profit: profit, clock: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(clock func() time.Time) { s.clock = clock }

// Period resolves the period like the profitability report: the current
// month to today by default (Europe/Istanbul), swapped when reversed,
// capped at MaxPeriodDays (the start moves).
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

// months lists the calendar months ("YYYY-MM") of [from, to].
func months(from, to rentaltime.Date) []string {
	var out []string
	cur := rentaltime.NewDate(from.Year, from.Month, 1)
	for !cur.After(to) {
		out = append(out, cur.Time().Format("2006-01"))
		cur = rentaltime.NewDate(cur.Year, cur.Month+1, 1)
	}
	return out
}

// method is the common method of the caller's scope.
func method(ctx context.Context) model.Method {
	m := model.Method{Timezone: "Europe/Istanbul", Currency: "TRY", VATInclusive: true, BranchScope: model.ScopeAll}
	if orgctx.BranchScope(ctx) != nil {
		m.BranchScope = model.ScopeBranches
	}
	return m
}

type branchInfo struct {
	id   int64
	uuid uuid.UUID
	code string
	name string
}

// branches loads the organization's branches by id.
func (s *Service) branches(ctx context.Context, orgID int64) (map[int64]branchInfo, error) {
	rows, err := s.q.ReportBranches(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]branchInfo, len(rows))
	for _, r := range rows {
		out[r.ID] = branchInfo{id: r.ID, uuid: r.Uuid, code: r.Code, name: r.Name}
	}
	return out, nil
}

// branchFilter resolves the branch filter: an unknown branch, or one
// outside the caller's scope, is not_found (the same answer for both).
func branchFilter(ctx context.Context, all map[int64]branchInfo, ids []uuid.UUID) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	byUUID := make(map[uuid.UUID]branchInfo, len(all))
	for _, b := range all {
		byUUID[b.uuid] = b
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		b, ok := byUUID[id]
		if !ok || orgctx.CheckBranch(ctx, b.id) != nil {
			return nil, invalid("branch", "not_found", "branch not found")
		}
		out = append(out, b.id)
	}
	return out, nil
}

// effectiveBranches: the filter when set, else the caller's scope (nil =
// every branch and the head office).
func effectiveBranches(ctx context.Context, filter []int64) []int64 {
	if filter != nil {
		return filter
	}
	return orgctx.BranchScope(ctx)
}

// profitError maps a profitability validation error to the reports one.
func profitError(err error) error {
	var pve *profitusecase.ValidationError
	if errors.As(err, &pve) {
		out := &ValidationError{}
		for _, d := range pve.Details {
			out.Details = append(out.Details, FieldError{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		return out
	}
	return err
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// pct is num / den in percent (one decimal); nil when den is zero.
func pct(num, den float64) *float64 {
	if den == 0 {
		return nil
	}
	v := round(num*100/den, 1)
	return &v
}

func kurus(n pgtype.Numeric) int64 { return money.FromNumericOrZero(n).Minor() }

func pgDate(d rentaltime.Date) pgtype.Date { return pgtype.Date{Time: d.Time(), Valid: true} }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// perDay is amount / days (kuruş, rounded); nil without days.
func perDay(amount money.Money, days float64) *money.Money {
	if days <= 0 {
		return nil
	}
	v := money.New(int64(math.Round(float64(amount.Minor()) / days)))
	return &v
}
