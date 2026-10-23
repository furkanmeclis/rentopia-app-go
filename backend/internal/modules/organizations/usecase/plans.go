package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrFeatureNotFound: the feature key is not in the catalog (404).
var ErrFeatureNotFound = errors.New("feature not found")

// PlanRepository is the persistence of plans and entitlements.
type PlanRepository interface {
	ListPlans(ctx context.Context) ([]db.ListPlansRow, error)
	GetPlanByCode(ctx context.Context, code string) (db.GetPlanByCodeRow, error)
	UpsertOrganizationPlan(ctx context.Context, p db.UpsertOrganizationPlanParams) error
	UpsertOrganizationFeature(ctx context.Context, p db.UpsertOrganizationFeatureParams) error
	DeleteOrganizationFeature(ctx context.Context, p db.DeleteOrganizationFeatureParams) error
	GetOrganizationFeatureUsage(ctx context.Context, orgID int64) (db.GetOrganizationFeatureUsageRow, error)
	AssignOrganizationPlan(ctx context.Context, q *db.Queries, p db.AssignOrganizationPlanByCodeParams) (int64, error)
}

// FeatureSource gives scopes and /v1/auth/me an organization's enabled
// features and plan: features.Source, normally the per-organization cache
// (orgcache.Features), so /me adds no query per membership.
type FeatureSource = features.Source

// PlanSource reads the full entitlement state for the platform plan
// screen (features.Store, uncached: it must show the latest write).
type PlanSource interface {
	States(ctx context.Context, organizationID int64) ([]features.State, error)
	Plan(ctx context.Context, organizationID int64) (features.PlanRef, error)
}

// Plans manages organization plans and feature overrides (TEC-674): the
// platform plan screen and the plan of new organizations. Every change
// invalidates the organization's cached scope and features.
type Plans struct {
	orgs   *Service
	repo   PlanRepository
	source PlanSource
}

// NewPlans builds the plan service and makes orgs give new organizations
// their plan.
func NewPlans(orgs *Service, repo PlanRepository, source PlanSource) *Plans {
	p := &Plans{orgs: orgs, repo: repo, source: source}
	orgs.plans = p
	return p
}

// ListPlans returns the plan catalog.
func (p *Plans) ListPlans(ctx context.Context) ([]orgmodel.Plan, error) {
	rows, err := p.repo.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]orgmodel.Plan, 0, len(rows))
	for _, r := range rows {
		keys := r.Features
		if keys == nil {
			keys = []string{}
		}
		out = append(out, orgmodel.Plan{Code: r.Code, Name: r.Name, NameEN: r.NameEn, Features: keys})
	}
	return out, nil
}

// Get returns the plan, period, feature states and usage of an
// organization.
func (p *Plans) Get(ctx context.Context, orgUUID uuid.UUID) (orgmodel.OrganizationPlan, error) {
	org, err := p.orgs.repo.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return orgmodel.OrganizationPlan{}, mapRepoNotFound(err, ErrNotFound)
	}
	return p.view(ctx, org.ID)
}

func (p *Plans) view(ctx context.Context, orgID int64) (orgmodel.OrganizationPlan, error) {
	plan, err := p.source.Plan(ctx, orgID)
	if err != nil {
		return orgmodel.OrganizationPlan{}, err
	}
	states, err := p.source.States(ctx, orgID)
	if err != nil {
		return orgmodel.OrganizationPlan{}, err
	}
	usage, err := p.repo.GetOrganizationFeatureUsage(ctx, orgID)
	if err != nil {
		return orgmodel.OrganizationPlan{}, err
	}
	counts := usageCounts(usage)
	out := orgmodel.OrganizationPlan{
		Plan:           orgmodel.PlanRef{Code: plan.Code, Name: plan.Name, NameEN: plan.NameEN},
		PeriodStartsAt: plan.PeriodStartsAt, PeriodEndsAt: plan.PeriodEndsAt, UpdatedAt: plan.UpdatedAt,
		Features: make([]orgmodel.FeatureState, 0, len(states)), EnabledFeatures: features.Strings(features.Effective(states)),
	}
	for _, st := range states {
		fs := orgmodel.FeatureState{Key: string(st.Key), PlanDefault: st.InPlan, Override: st.Override, Enabled: st.Enabled()}
		if n, ok := counts[st.Key]; ok {
			v := n
			fs.Usage = &v
		}
		out.Features = append(out.Features, fs)
	}
	return out, nil
}

func usageCounts(u db.GetOrganizationFeatureUsageRow) map[features.Feature]int64 {
	return map[features.Feature]int64{
		features.Site:            u.SiteDomains,
		features.PublicAPI:       u.PublicApiKeys,
		features.ChannelYolcu360: u.Yolcu360Accounts,
		features.ChannelObilet:   u.ObiletAccounts,
		features.Kabis:           u.KabisNotifications,
		features.HGS:             u.TollPasses,
		features.EInvoice:        u.Einvoices,
		features.WhatsApp:        u.WhatsappSessions,
		features.SMS:             u.SmsMessages,
	}
}

func planValidation(field, code, msg string) error {
	return &ValidationError{Details: []FieldError{{Field: field, Code: code, Message: msg}}}
}

func actorArg(actorID int64) pgtype.Int8 {
	return pgtype.Int8{Int64: actorID, Valid: actorID > 0}
}

func tsArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

// SetPlan changes an organization's plan and period. Past records stay:
// only the effective features change. It returns the new view and the
// previous plan code.
func (p *Plans) SetPlan(ctx context.Context, orgUUID uuid.UUID, in orgmodel.PlanInput, actorID int64) (orgmodel.OrganizationPlan, string, error) {
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return orgmodel.OrganizationPlan{}, "", planValidation("code", "required", "code is required")
	}
	if in.PeriodStartsAt != nil && in.PeriodEndsAt != nil && !in.PeriodEndsAt.After(*in.PeriodStartsAt) {
		return orgmodel.OrganizationPlan{}, "", planValidation("period_ends_at", "invalid", "period_ends_at must be after period_starts_at")
	}
	org, err := p.orgs.repo.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return orgmodel.OrganizationPlan{}, "", mapRepoNotFound(err, ErrNotFound)
	}
	plan, err := p.repo.GetPlanByCode(ctx, code)
	if errors.Is(err, repository.ErrNotFound) {
		return orgmodel.OrganizationPlan{}, "", planValidation("code", "invalid", "unknown plan")
	}
	if err != nil {
		return orgmodel.OrganizationPlan{}, "", err
	}
	before, err := p.source.Plan(ctx, org.ID)
	if err != nil {
		return orgmodel.OrganizationPlan{}, "", err
	}
	if err := p.repo.UpsertOrganizationPlan(ctx, db.UpsertOrganizationPlanParams{
		OrganizationID: org.ID, PlanID: plan.ID, PeriodStartsAt: tsArg(in.PeriodStartsAt),
		PeriodEndsAt: tsArg(in.PeriodEndsAt), UpdatedBy: actorArg(actorID),
	}); err != nil {
		return orgmodel.OrganizationPlan{}, "", err
	}
	_ = orgctx.Invalidate(ctx, p.orgs.invalidator, org.ID)
	out, err := p.view(ctx, org.ID)
	return out, before.Code, err
}

// SetFeature overrides one feature (enabled true / false) or clears the
// override (nil). It returns the new view and the previous override.
func (p *Plans) SetFeature(ctx context.Context, orgUUID uuid.UUID, key string, enabled *bool, actorID int64) (orgmodel.OrganizationPlan, *bool, error) {
	f, ok := features.Parse(key)
	if !ok {
		return orgmodel.OrganizationPlan{}, nil, ErrFeatureNotFound
	}
	org, err := p.orgs.repo.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return orgmodel.OrganizationPlan{}, nil, mapRepoNotFound(err, ErrNotFound)
	}
	var previous *bool
	if states, err := p.source.States(ctx, org.ID); err == nil {
		for _, st := range states {
			if st.Key == f {
				previous = st.Override
			}
		}
	} else {
		return orgmodel.OrganizationPlan{}, nil, err
	}
	if enabled == nil {
		err = p.repo.DeleteOrganizationFeature(ctx, db.DeleteOrganizationFeatureParams{OrganizationID: org.ID, FeatureKey: string(f)})
	} else {
		err = p.repo.UpsertOrganizationFeature(ctx, db.UpsertOrganizationFeatureParams{
			OrganizationID: org.ID, FeatureKey: string(f), Enabled: *enabled, UpdatedBy: actorArg(actorID),
		})
	}
	if err != nil {
		return orgmodel.OrganizationPlan{}, nil, err
	}
	_ = orgctx.Invalidate(ctx, p.orgs.invalidator, org.ID)
	out, err := p.view(ctx, org.ID)
	return out, previous, err
}

// assign gives a new organization its plan inside the create transaction.
func (p *Plans) assign(ctx context.Context, q *db.Queries, orgID int64, code string, actorID int64) error {
	code = strings.TrimSpace(code)
	if code == "" {
		code = features.DefaultPlan
	}
	n, err := p.repo.AssignOrganizationPlan(ctx, q, db.AssignOrganizationPlanByCodeParams{
		OrganizationID: orgID, Code: code, UpdatedBy: actorArg(actorID),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return planValidation("plan_code", "invalid", "unknown plan")
	}
	return nil
}
