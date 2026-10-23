package repository

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
)

// Plans is the persistence of plans and organization entitlements
// (migration 000134).
type Plans struct {
	q *db.Queries
}

// NewPlans builds the plan repository.
func NewPlans(q *db.Queries) *Plans { return &Plans{q: q} }

func (r *Plans) ListPlans(ctx context.Context) ([]db.ListPlansRow, error) {
	return r.q.ListPlans(ctx)
}

func (r *Plans) GetPlanByCode(ctx context.Context, code string) (db.GetPlanByCodeRow, error) {
	row, err := r.q.GetPlanByCode(ctx, code)
	return row, mapErr(err)
}

func (r *Plans) UpsertOrganizationPlan(ctx context.Context, p db.UpsertOrganizationPlanParams) error {
	return r.q.UpsertOrganizationPlan(ctx, p)
}

func (r *Plans) UpsertOrganizationFeature(ctx context.Context, p db.UpsertOrganizationFeatureParams) error {
	return r.q.UpsertOrganizationFeature(ctx, p)
}

func (r *Plans) DeleteOrganizationFeature(ctx context.Context, p db.DeleteOrganizationFeatureParams) error {
	_, err := r.q.DeleteOrganizationFeature(ctx, p)
	return err
}

func (r *Plans) GetOrganizationFeatureUsage(ctx context.Context, orgID int64) (db.GetOrganizationFeatureUsageRow, error) {
	return r.q.GetOrganizationFeatureUsage(ctx, orgID)
}

// AssignOrganizationPlan gives a new organization its plan inside the
// create transaction (q); it returns 0 rows for an unknown code.
func (r *Plans) AssignOrganizationPlan(ctx context.Context, q *db.Queries, p db.AssignOrganizationPlanByCodeParams) (int64, error) {
	if q == nil {
		q = r.q
	}
	return q.AssignOrganizationPlanByCode(ctx, p)
}
