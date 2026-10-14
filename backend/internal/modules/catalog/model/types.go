// Package model holds the vehicle catalog DTOs and import plan types.
package model

import (
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/google/uuid"
)

// Import statuses (match the catalog_imports CHECK).
const (
	ImportQueued     = "queued"
	ImportProcessing = "processing"
	ImportReady      = "ready"
	ImportApplying   = "applying"
	ImportApplied    = "applied"
	ImportFailed     = "failed"
)

// Model sources.
const (
	SourceAuto   = "auto"
	SourceManual = "manual"
)

// Brand is a catalog brand as returned by the platform API.
type Brand struct {
	UUID            uuid.UUID       `json:"uuid"`
	TSBCode         string          `json:"tsb_code"`
	TSBName         string          `json:"tsb_name"`
	DisplayName     string          `json:"display_name"`
	Category        string          `json:"category"`
	IsActive        bool            `json:"is_active"`
	TypesCount      int64           `json:"types_count"`
	ModelsCount     int64           `json:"models_count"`
	UnresolvedCount int64           `json:"unresolved_count"`
	DisplayBrand    DisplayBrandRef `json:"display_brand"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// BrandRef is the short brand embedded in models and types.
type BrandRef struct {
	UUID        uuid.UUID `json:"uuid"`
	DisplayName string    `json:"display_name"`
	TSBCode     string    `json:"tsb_code,omitempty"`
	TSBName     string    `json:"tsb_name,omitempty"`
}

// DisplayBrandRef is the short display brand embedded in brands, models
// and types.
type DisplayBrandRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// DisplayBrand is the brand users see; one or more TSB brands map to it.
type DisplayBrand struct {
	UUID        uuid.UUID            `json:"uuid"`
	Name        string               `json:"name"`
	Slug        string               `json:"slug"`
	LogoURL     *string              `json:"logo_url"`
	IsActive    bool                 `json:"is_active"`
	BrandsCount int64                `json:"brands_count"`
	TypesCount  int64                `json:"types_count"`
	ModelsCount int64                `json:"models_count"`
	TSBNames    []string             `json:"tsb_names"`
	Brands      []DisplayBrandMember `json:"brands,omitempty"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// DisplayBrandMember is a TSB brand under a display brand.
type DisplayBrandMember struct {
	UUID       uuid.UUID `json:"uuid"`
	TSBCode    string    `json:"tsb_code"`
	TSBName    string    `json:"tsb_name"`
	Category   string    `json:"category"`
	IsActive   bool      `json:"is_active"`
	TypesCount int64     `json:"types_count"`
}

// NamedRef is a uuid + name pair.
type NamedRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ModelRef is the short model embedded in types.
type ModelRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Name   string    `json:"name"`
	Source string    `json:"source"`
}

// Model is a catalog model.
type Model struct {
	UUID         uuid.UUID       `json:"uuid"`
	Name         string          `json:"name"`
	Source       string          `json:"source"`
	Brand        BrandRef        `json:"brand"`
	DisplayBrand DisplayBrandRef `json:"display_brand"`
	TypesCount   int64           `json:"types_count"`
	// GroupSize is how many models (one per TSB brand) share this model's
	// name under its display brand; tenants see them as one model.
	GroupSize int64       `json:"group_size"`
	Rules     []ModelRule `json:"rules,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// BulkRulesResult reports a bulk "prefix rule = model name" action.
type BulkRulesResult struct {
	RulesSaved    int `json:"rules_saved"`
	TypesAssigned int `json:"types_assigned"`
}

// RebuildResult reports a re-derivation of auto models.
type RebuildResult struct {
	TypesChanged     int `json:"types_changed"`
	ModelsCreated    int `json:"models_created"`
	ModelsDeleted    int `json:"models_deleted"`
	UnresolvedBefore int `json:"unresolved_before"`
	UnresolvedAfter  int `json:"unresolved_after"`
	AutoModelsBefore int `json:"auto_models_before"`
	AutoModelsAfter  int `json:"auto_models_after"`
}

// ModelRule maps a type-name prefix to a model.
type ModelRule struct {
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

// Type is a catalog type (TSB type line).
type Type struct {
	UUID                  uuid.UUID       `json:"uuid"`
	TSBTypeCode           string          `json:"tsb_type_code"`
	RawName               string          `json:"raw_name"`
	Years                 []int32         `json:"years"`
	SuggestedTransmission *string         `json:"suggested_transmission"`
	SuggestedFuel         *string         `json:"suggested_fuel"`
	FirstSeenPeriod       string          `json:"first_seen_period"`
	LastSeenPeriod        string          `json:"last_seen_period"`
	IsActive              bool            `json:"is_active"`
	Brand                 BrandRef        `json:"brand"`
	DisplayBrand          DisplayBrandRef `json:"display_brand"`
	Model                 *ModelRef       `json:"model"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// Valuation is one kasko value.
type Valuation struct {
	ModelYear int32   `json:"model_year"`
	Period    string  `json:"period"`
	Value     float64 `json:"value"`
}

// TypeDetail is a type with its latest-period valuations and history.
type TypeDetail struct {
	Type
	BrandTSBName string      `json:"brand_tsb_name"`
	LatestPeriod string      `json:"latest_period"`
	Valuations   []Valuation `json:"valuations"`
	History      []Valuation `json:"history"`
}

// Import is a catalog import job.
type Import struct {
	UUID           uuid.UUID      `json:"uuid"`
	Period         *string        `json:"period"`
	Filename       string         `json:"filename"`
	Status         string         `json:"status"`
	Counts         map[string]any `json:"counts"`
	Diff           map[string]any `json:"diff,omitempty"`
	Error          *string        `json:"error"`
	CreatedByEmail *string        `json:"created_by_email,omitempty"`
	AppliedAt      *time.Time     `json:"applied_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// TenantBrand / TenantModel / TenantType are the read-only shapes for
// organization vehicle forms.
type TenantBrand struct {
	UUID        uuid.UUID `json:"uuid"`
	DisplayName string    `json:"display_name"`
	Slug        string    `json:"slug"`
	Category    string    `json:"category"`
	LogoURL     *string   `json:"logo_url"`
}

type TenantModel struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Brand BrandRef  `json:"brand"`
}

type TenantType struct {
	UUID                  uuid.UUID `json:"uuid"`
	TSBTypeCode           string    `json:"tsb_type_code"`
	Name                  string    `json:"name"`
	Years                 []int32   `json:"years"`
	SuggestedTransmission *string   `json:"suggested_transmission"`
	SuggestedFuel         *string   `json:"suggested_fuel"`
	Brand                 BrandRef  `json:"brand"`
	Model                 NamedRef  `json:"model"`
	KaskoValue            *float64  `json:"kasko_value"`
	KaskoPeriod           *string   `json:"kasko_period"`
}

// Filters.

type BrandFilter struct {
	Limit, Offset    int32
	Q, Category      string
	IsActive         *bool
	DisplayBrandUUID *uuid.UUID
	SortField        string
	SortDesc         bool
}

type DisplayBrandFilter struct {
	Limit, Offset int32
	Q             string
	IsActive      *bool
	MergedOnly    bool
	SortField     string
	SortDesc      bool
}

type ModelFilter struct {
	Limit, Offset    int32
	Q, Source        string
	BrandUUID        *uuid.UUID
	DisplayBrandUUID *uuid.UUID
	SortField        string
	SortDesc         bool
}

type TypeFilter struct {
	Limit, Offset        int32
	Q, Category          string
	BrandUUID, ModelUUID *uuid.UUID
	DisplayBrandUUID     *uuid.UUID
	Year                 *int32
	Unresolved           bool
	IsActive             *bool
	Fuel, Transmission   string
	SortField            string
	SortDesc             bool
}

type ImportFilter struct {
	Limit, Offset int32
	Q, Status     string
	SortField     string
	SortDesc      bool
}

type TenantFilter struct {
	Limit, Offset int32
	Q, Category   string
	Year          *int32
	BrandUUID     uuid.UUID
	ModelUUID     uuid.UUID
}

// Inputs.

type PatchBrandInput struct {
	// DisplayName maps the TSB brand to the display brand of that name
	// (created when missing); DisplayBrandUUID to an existing one.
	DisplayName      *string
	DisplayBrandUUID *uuid.UUID
	Category         *string
	IsActive         *bool
}

type PatchDisplayBrandInput struct {
	Name     *string
	IsActive *bool
}

type PatchModelInput struct {
	Name   *string
	Source *string
}

type PatchTypeInput struct {
	// SetModel: ModelUUID nil clears the model.
	SetModel  bool
	ModelUUID *uuid.UUID
	// SaveRule stores RulePrefix (default: the model name) as a prefix rule
	// for the type's brand and applies it to unresolved / auto types.
	SaveRule              bool
	RulePrefix            string
	SetTransmission       bool
	SuggestedTransmission *string
	SetFuel               bool
	SuggestedFuel         *string
}

// Import engine state & plan.

// StateBrand is an existing brand.
type StateBrand struct {
	ID       int64
	Name     string
	Category string
}

// DisplayKey is the lookup key of a display brand name.
func DisplayKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// TypeKey identifies a type by brand and TSB type code.
type TypeKey struct {
	BrandCode string
	TypeCode  string
}

// StateType is an existing type.
type StateType struct {
	ID        int64
	BrandCode string
	ModelID   int64
	RawName   string
	Years     []int32
	IsActive  bool
	LastSeen  string
}

// ValuationKey identifies a valuation of a type for the plan period.
type ValuationKey struct {
	TypeID int64
	Year   int32
}

// State is the catalog as stored, loaded before planning.
type State struct {
	Brands map[string]StateBrand // by TSB code
	// DisplayBrands holds existing display brand ids by DisplayKey(name).
	DisplayBrands map[string]int64
	Models        map[int64][]tsbcatalog.ModelRef
	Rules         map[int64][]tsbcatalog.RuleRef
	Types         map[TypeKey]StateType
	Valuations    map[ValuationKey]int64 // values of the plan period
}

// PlanBrand is a brand line of the file.
type PlanBrand struct {
	Code        string
	Name        string
	DisplayName string
	Category    string
	Op          string // new | renamed | same
	OldName     string
}

// PlanModel is an auto model to create.
type PlanModel struct {
	BrandCode string
	Name      string
}

// PlanType is a type line of the file.
type PlanType struct {
	BrandCode    string
	Code         string
	RawName      string
	Years        []int32
	Transmission string
	Fuel         string
	// ModelID is an existing model; ModelName a new auto model to create.
	ModelID   int64
	ModelName string
	Op        string // new | renamed | reactivated | updated | same
	OldName   string
}

// PlanValuation is one non-zero kasko value of the file.
type PlanValuation struct {
	BrandCode string
	TypeCode  string
	Year      int32
	Value     int64
}

// Counts summarizes a dry run or an apply.
type Counts struct {
	Rows                 int            `json:"rows"`
	BrandsNew            int            `json:"brands_new"`
	BrandsRenamed        int            `json:"brands_renamed"`
	DisplayBrandsNew     int            `json:"display_brands_new"`
	ModelsNew            int            `json:"models_new"`
	TypesNew             int            `json:"types_new"`
	TypesRenamed         int            `json:"types_renamed"`
	TypesReactivated     int            `json:"types_reactivated"`
	TypesUpdated         int            `json:"types_updated"`
	TypesDeactivated     int            `json:"types_deactivated"`
	ValuationsNew        int            `json:"valuations_new"`
	ValuationsChanged    int            `json:"valuations_changed"`
	ValuationsUpserted   int            `json:"valuations_upserted"`
	UnresolvedModels     int            `json:"unresolved_models"`
	UnresolvedByCategory map[string]int `json:"unresolved_by_category"`
}

// DiffSample lines shown on the import page.
type SampleBrand struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	OldName     string `json:"old_name,omitempty"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
}

type SampleModel struct {
	Brand string `json:"brand"`
	Name  string `json:"name"`
	Types int    `json:"types"`
}

type SampleType struct {
	Brand   string `json:"brand"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	OldName string `json:"old_name,omitempty"`
	Model   string `json:"model,omitempty"`
}

type SampleValuation struct {
	Brand string `json:"brand"`
	Code  string `json:"code"`
	Name  string `json:"name"`
	Year  int32  `json:"year"`
	Old   int64  `json:"old"`
	New   int64  `json:"new"`
}

// Diff holds samples of each change kind (capped).
type Diff struct {
	PeriodLabel       string            `json:"period_label,omitempty"`
	Years             []int             `json:"years,omitempty"`
	NewBrands         []SampleBrand     `json:"new_brands"`
	RenamedBrands     []SampleBrand     `json:"renamed_brands"`
	NewModels         []SampleModel     `json:"new_models"`
	NewTypes          []SampleType      `json:"new_types"`
	RenamedTypes      []SampleType      `json:"renamed_types"`
	DeactivatedTypes  []SampleType      `json:"deactivated_types"`
	ReactivatedTypes  []SampleType      `json:"reactivated_types"`
	UnresolvedTypes   []SampleType      `json:"unresolved_types"`
	ChangedValuations []SampleValuation `json:"changed_valuations"`
}

// Plan is the computed (not yet applied) result of importing a file.
type Plan struct {
	Period     string
	Brands     []PlanBrand
	NewModels  []PlanModel
	Types      []PlanType
	Deactivate []int64
	Valuations []PlanValuation
	Counts     Counts
	Diff       Diff
}

// ApplyResult reports what an apply actually changed in the database.
type ApplyResult struct {
	DisplayBrandsInserted int
	BrandsInserted        int
	BrandsUpdated         int
	ModelsInserted        int
	TypesInserted         int
	TypesUpdated          int
	TypesDeactivated      int
	ValuationsUpserted    int
	UnresolvedByCategory  map[string]int
}
