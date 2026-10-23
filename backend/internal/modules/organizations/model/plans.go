package model

import "time"

// Plan is a plan of the catalog (GET /v1/platform/plans).
type Plan struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	NameEN   string   `json:"name_en"`
	Features []string `json:"features"`
}

// PlanRef names an organization's plan.
type PlanRef struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	NameEN string `json:"name_en"`
}

// FeatureState is one feature on the platform plan screen.
type FeatureState struct {
	Key         string `json:"key"`
	PlanDefault bool   `json:"plan_default"`
	// Override is nil when the plan default applies.
	Override *bool `json:"override"`
	Enabled  bool  `json:"enabled"`
	// Usage is a usage counter of the feature (nil when none is tracked).
	Usage *int64 `json:"usage"`
}

// OrganizationPlan is GET /v1/platform/organizations/{uuid}/plan.
type OrganizationPlan struct {
	Plan            PlanRef        `json:"plan"`
	PeriodStartsAt  *time.Time     `json:"period_starts_at"`
	PeriodEndsAt    *time.Time     `json:"period_ends_at"`
	UpdatedAt       *time.Time     `json:"updated_at"`
	Features        []FeatureState `json:"features"`
	EnabledFeatures []string       `json:"enabled_features"`
}

// PlanInput is PUT /v1/platform/organizations/{uuid}/plan.
type PlanInput struct {
	Code           string     `json:"code"`
	PeriodStartsAt *time.Time `json:"period_starts_at"`
	PeriodEndsAt   *time.Time `json:"period_ends_at"`
}

// FeatureInput is PUT /v1/platform/organizations/{uuid}/features/{key}:
// true / false overrides the plan, null clears the override.
type FeatureInput struct {
	Enabled *bool `json:"enabled"`
}
