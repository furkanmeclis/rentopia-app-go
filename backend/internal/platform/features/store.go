package features

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// PlanRef is an organization's plan.
type PlanRef struct {
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	NameEN         string     `json:"name_en"`
	PeriodStartsAt *time.Time `json:"period_starts_at"`
	PeriodEndsAt   *time.Time `json:"period_ends_at"`
	// Assigned is false when the organization has no plan row and reads
	// the default plan.
	Assigned  bool       `json:"-"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// Store reads plans and entitlements (migration 000134). It is the
// uncached Loader; orgcache wraps it per organization.
type Store struct {
	q *db.Queries
}

// NewStore builds the store.
func NewStore(q *db.Queries) *Store { return &Store{q: q} }

// States returns every feature of the organization with its plan default
// and override, in catalog order.
func (s *Store) States(ctx context.Context, organizationID int64) ([]State, error) {
	rows, err := s.q.ListOrganizationFeatureStates(ctx, db.ListOrganizationFeatureStatesParams{
		OrganizationID: organizationID, DefaultPlanCode: DefaultPlan,
	})
	if err != nil {
		return nil, err
	}
	out := make([]State, 0, len(rows))
	for _, r := range rows {
		st := State{Key: Feature(r.Key), InPlan: r.InPlan}
		if r.Override.Valid {
			v := r.Override.Bool
			st.Override = &v
		}
		out = append(out, st)
	}
	return out, nil
}

// EnabledFeatures implements Loader.
func (s *Store) EnabledFeatures(ctx context.Context, organizationID int64) ([]Feature, error) {
	states, err := s.States(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return Effective(states), nil
}

// Plan returns the organization's plan (the default plan without a row).
func (s *Store) Plan(ctx context.Context, organizationID int64) (PlanRef, error) {
	row, err := s.q.GetOrganizationPlan(ctx, db.GetOrganizationPlanParams{
		OrganizationID: organizationID, DefaultPlanCode: DefaultPlan,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// No plan row and no default plan seeded (a broken database):
		// report the default code so callers still answer.
		return PlanRef{Code: DefaultPlan, Name: DefaultPlan, NameEN: DefaultPlan}, nil
	}
	if err != nil {
		return PlanRef{}, err
	}
	out := PlanRef{Code: row.Code, Name: row.Name, NameEN: row.NameEn, Assigned: row.Assigned}
	if row.PeriodStartsAt.Valid {
		t := row.PeriodStartsAt.Time.UTC()
		out.PeriodStartsAt = &t
	}
	if row.PeriodEndsAt.Valid {
		t := row.PeriodEndsAt.Time.UTC()
		out.PeriodEndsAt = &t
	}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time.UTC()
		out.UpdatedAt = &t
	}
	return out, nil
}
