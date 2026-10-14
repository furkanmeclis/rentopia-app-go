// Package model holds the API shapes of the fleet classification resources:
// vehicle segments, rental groups and rental products (TEC-691).
//
// Domain: a physical vehicle is not a model/variant, not a segment, not a
// rental group and not a rentable product. A segment is a commercial class
// ("araç grubu" in the beta doc). A rental group is the set of protected
// attributes an "X veya benzeri" product guarantees. A rental product ("alt
// grup") is what a channel sells; its guarantee and rental conditions are
// versioned and past versions never change.
package model

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Engine resource slugs (activity, io, bulk).
const (
	ResourceSegments       = "tenant.segments"
	ResourceRentalGroups   = "tenant.rental_groups"
	ResourceRentalProducts = "tenant.rental_products"
)

// Protected attribute values.
var (
	Transmissions = []string{"manual", "automatic"}
	Fuels         = []string{"petrol", "diesel", "hybrid", "electric", "lpg"}
	BodyTypes     = []string{
		"hatchback", "sedan", "suv", "crossover", "station_wagon", "mpv", "van", "pickup", "coupe", "cabrio",
	}
)

// Guarantee types: what the customer of a product is promised.
const (
	// GuaranteeExactVehicle promises one specific vehicle.
	GuaranteeExactVehicle = "EXACT_VEHICLE"
	// GuaranteeExactVariant promises a catalog model or type (VariantLevel).
	GuaranteeExactVariant = "EXACT_VARIANT"
	// GuaranteeGroup promises the protected attributes of a rental group
	// ("X veya benzeri").
	GuaranteeGroup = "GROUP"
)

// GuaranteeTypes lists the guarantee types.
var GuaranteeTypes = []string{GuaranteeExactVehicle, GuaranteeExactVariant, GuaranteeGroup}

// Variant levels of an EXACT_VARIANT guarantee.
const (
	VariantLevelModel = "model"
	VariantLevelType  = "type"
)

// Product publish statuses.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

// ProductStatuses lists the publish statuses.
var ProductStatuses = []string{StatusDraft, StatusPublished, StatusArchived}

// FuelPolicies lists the fuel policies of rental conditions.
var FuelPolicies = []string{"same_to_same", "full_to_full", "prepaid", "full_to_empty"}

// Guarantee badges (UI / channel label keys) per guarantee type.
const (
	BadgeOrSimilar         = "or_similar"
	BadgeModelGuaranteed   = "model_guaranteed"
	BadgeVehicleGuaranteed = "vehicle_guaranteed"
)

// --- references -------------------------------------------------------------

// SegmentRef is a segment embedded in another resource.
type SegmentRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// GroupRef is a rental group embedded in another resource.
type GroupRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// BrandRef is a platform catalog brand.
type BrandRef struct {
	UUID        uuid.UUID `json:"uuid"`
	DisplayName string    `json:"display_name"`
}

// NamedRef is a uuid + name pair.
type NamedRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// CatalogModelRef is a platform catalog model.
type CatalogModelRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Brand BrandRef  `json:"brand"`
	// Label is "<brand> <model>" ("Renault Clio").
	Label string `json:"label"`
}

// CatalogTypeRef is a platform catalog type (variant).
type CatalogTypeRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Model *NamedRef `json:"model"`
	Brand BrandRef  `json:"brand"`
	// Label is "<brand> <type>" ("Fiat Egea Cross 1.4 Fire").
	Label string `json:"label"`
}

// Actor is the user behind a version or an activity entry.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// --- segments ---------------------------------------------------------------

// Segment is the API projection of a vehicle segment.
type Segment struct {
	UUID          uuid.UUID `json:"uuid"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	SortOrder     int32     `json:"sort_order"`
	IsActive      bool      `json:"is_active"`
	GroupsCount   int64     `json:"groups_count"`
	ProductsCount int64     `json:"products_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SegmentInput is the create / patch body of a segment.
type SegmentInput struct {
	Code        *string `json:"code"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	SortOrder   *int32  `json:"sort_order"`
	IsActive    *bool   `json:"is_active"`
}

// SegmentFilter selects segments of one organization.
type SegmentFilter struct {
	Limit, Offset int32
	Q             string
	IsActive      *bool
	SortField     string
	SortDesc      bool
}

// --- rental groups ----------------------------------------------------------

// RentalGroup is the API projection of a rental group.
type RentalGroup struct {
	UUID        uuid.UUID  `json:"uuid"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Segment     SegmentRef `json:"segment"`
	// Protected attributes; null = not guaranteed.
	Transmission    *string `json:"transmission"`
	Fuel            *string `json:"fuel"`
	BodyType        *string `json:"body_type"`
	MinSeats        *int16  `json:"min_seats"`
	MinLuggage      *int16  `json:"min_luggage"`
	AirConditioning bool    `json:"air_conditioning"`
	// AcrissCode is a classification hint, never a guarantee.
	AcrissCode *string `json:"acriss_code"`
	// Membership criteria; both empty = any vehicle with the attributes.
	CatalogModels []CatalogModelRef `json:"catalog_models"`
	CatalogTypes  []CatalogTypeRef  `json:"catalog_types"`
	IsActive      bool              `json:"is_active"`
	// ProductsCount counts live products whose current version guarantees
	// this group.
	ProductsCount int64     `json:"products_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// RentalGroupInput is the create / patch body of a rental group.
type RentalGroupInput struct {
	Code              *string        `json:"code"`
	Name              *string        `json:"name"`
	Description       *string        `json:"description"`
	SegmentUUID       *uuid.UUID     `json:"segment_uuid"`
	Transmission      OptionalString `json:"transmission"`
	Fuel              OptionalString `json:"fuel"`
	BodyType          OptionalString `json:"body_type"`
	MinSeats          OptionalInt    `json:"min_seats"`
	MinLuggage        OptionalInt    `json:"min_luggage"`
	AirConditioning   *bool          `json:"air_conditioning"`
	AcrissCode        OptionalString `json:"acriss_code"`
	CatalogModelUUIDs *[]uuid.UUID   `json:"catalog_model_uuids"`
	CatalogTypeUUIDs  *[]uuid.UUID   `json:"catalog_type_uuids"`
	IsActive          *bool          `json:"is_active"`
}

// RentalGroupFilter selects rental groups of one organization.
type RentalGroupFilter struct {
	Limit, Offset int32
	Q             string
	IsActive      *bool
	SegmentUUIDs  []uuid.UUID
	Transmissions []string
	Fuels         []string
	BodyTypes     []string
	SortField     string
	SortDesc      bool
}

// --- rental products --------------------------------------------------------

// ProductImage is one image of a product. PublicURL is API-owned: clients
// use it as is and never build object-store URLs.
type ProductImage struct {
	UUID        uuid.UUID `json:"uuid"`
	PublicURL   string    `json:"public_url"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	SortOrder   int32     `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
}

// Guarantee is the promise of a product version.
type Guarantee struct {
	Type string `json:"type"`
	// VehicleID is the guaranteed vehicle of EXACT_VEHICLE: a live vehicle
	// of the organization, checked on save (no foreign key; may be null).
	VehicleID    *uuid.UUID       `json:"vehicle_id"`
	VariantLevel *string          `json:"variant_level"`
	CatalogModel *CatalogModelRef `json:"catalog_model"`
	CatalogType  *CatalogTypeRef  `json:"catalog_type"`
	RentalGroup  *GroupRef        `json:"rental_group"`
	// Label is the snapshot of the guaranteed reference taken when the
	// version was created.
	Label string `json:"label"`
}

// Conditions are the rental conditions of a product version. Money is a
// decimal string with two fraction digits.
type Conditions struct {
	DepositAmount   string `json:"deposit_amount"`
	MinDriverAge    int16  `json:"min_driver_age"`
	MinLicenseYears int16  `json:"min_license_years"`
	// IncludedKmPerDay is null for unlimited kilometres.
	IncludedKmPerDay *int32 `json:"included_km_per_day"`
	ExtraKmFee       string `json:"extra_km_fee"`
	FuelPolicy       string `json:"fuel_policy"`
	Notes            string `json:"notes"`
}

// ProductVersion is one immutable version of a product's guarantee and
// conditions.
type ProductVersion struct {
	UUID       uuid.UUID  `json:"uuid"`
	Version    int32      `json:"version"`
	IsCurrent  bool       `json:"is_current"`
	Guarantee  Guarantee  `json:"guarantee"`
	Conditions Conditions `json:"conditions"`
	ChangeNote string     `json:"change_note"`
	CreatedBy  *Actor     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

// RentalProduct is the API projection of a rentable product.
type RentalProduct struct {
	UUID                uuid.UUID        `json:"uuid"`
	Code                string           `json:"code"`
	Title               string           `json:"title"`
	Description         string           `json:"description"`
	Segment             SegmentRef       `json:"segment"`
	RepresentativeModel *CatalogModelRef `json:"representative_model"`
	RepresentativeType  *CatalogTypeRef  `json:"representative_type"`
	Status              string           `json:"status"`
	IsActive            bool             `json:"is_active"`
	CurrentVersion      int32            `json:"current_version"`
	// Version is the current (latest) version.
	Version ProductVersion `json:"version"`
	// DisplayText is the customer-facing line: "Renault Clio veya benzeri",
	// "Fiat Egea Cross 1.4 Fire (model garantili)".
	DisplayText    string         `json:"display_text"`
	GuaranteeBadge string         `json:"guarantee_badge"`
	Images         []ProductImage `json:"images"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// GuaranteeInput replaces the guarantee of a product (creates a version).
type GuaranteeInput struct {
	Type             string     `json:"type"`
	VehicleID        *uuid.UUID `json:"vehicle_id"`
	VariantLevel     *string    `json:"variant_level"`
	CatalogModelUUID *uuid.UUID `json:"catalog_model_uuid"`
	CatalogTypeUUID  *uuid.UUID `json:"catalog_type_uuid"`
	RentalGroupUUID  *uuid.UUID `json:"rental_group_uuid"`
}

// ConditionsInput changes rental conditions; nil fields keep their value.
type ConditionsInput struct {
	DepositAmount    *json.Number `json:"deposit_amount"`
	MinDriverAge     *int16       `json:"min_driver_age"`
	MinLicenseYears  *int16       `json:"min_license_years"`
	IncludedKmPerDay OptionalInt  `json:"included_km_per_day"`
	ExtraKmFee       *json.Number `json:"extra_km_fee"`
	FuelPolicy       *string      `json:"fuel_policy"`
	Notes            *string      `json:"notes"`
}

// RentalProductInput is the create / patch body of a product. Guarantee and
// Conditions changes create a new version; the other fields do not.
type RentalProductInput struct {
	Code                    *string          `json:"code"`
	Title                   *string          `json:"title"`
	Description             *string          `json:"description"`
	SegmentUUID             *uuid.UUID       `json:"segment_uuid"`
	RepresentativeModelUUID OptionalUUID     `json:"representative_model_uuid"`
	RepresentativeTypeUUID  OptionalUUID     `json:"representative_type_uuid"`
	Status                  *string          `json:"status"`
	IsActive                *bool            `json:"is_active"`
	Guarantee               *GuaranteeInput  `json:"guarantee"`
	Conditions              *ConditionsInput `json:"conditions"`
	ChangeNote              *string          `json:"change_note"`
}

// RentalProductFilter selects products of one organization.
type RentalProductFilter struct {
	Limit, Offset   int32
	Q               string
	IsActive        *bool
	Statuses        []string
	SegmentUUIDs    []uuid.UUID
	GuaranteeTypes  []string
	RentalGroupUUID *uuid.UUID
	SortField       string
	SortDesc        bool
}

// ImageOrderInput is the body of the image reorder route.
type ImageOrderInput struct {
	ImageUUIDs []uuid.UUID `json:"image_uuids"`
}

// --- shared -----------------------------------------------------------------

// Facet is one value of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// SegmentFacets are the dynamic filter options of the segments list.
type SegmentFacets struct {
	IsActive []Facet `json:"is_active"`
}

// RentalGroupFacets are the dynamic filter options of the groups list.
type RentalGroupFacets struct {
	Segment      []Facet `json:"segment"`
	Transmission []Facet `json:"transmission"`
	Fuel         []Facet `json:"fuel"`
	BodyType     []Facet `json:"body_type"`
}

// RentalProductFacets are the dynamic filter options of the products list.
type RentalProductFacets struct {
	Segment       []Facet `json:"segment"`
	Status        []Facet `json:"status"`
	GuaranteeType []Facet `json:"guarantee_type"`
}

// ActivityEntry is one audit event of a fleet record.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *Actor          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

// OptionalString distinguishes an absent JSON field from an explicit null.
type OptionalString struct {
	Set   bool
	Value *string
}

func (o *OptionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if isNull(b) {
		o.Value = nil
		return nil
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalInt distinguishes an absent JSON field from an explicit null.
type OptionalInt struct {
	Set   bool
	Value *int64
}

func (o *OptionalInt) UnmarshalJSON(b []byte) error {
	o.Set = true
	if isNull(b) {
		o.Value = nil
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalUUID distinguishes an absent JSON field from an explicit null.
type OptionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (o *OptionalUUID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if isNull(b) {
		o.Value = nil
		return nil
	}
	var v uuid.UUID
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

func isNull(b []byte) bool {
	return bytes.Equal(bytes.TrimSpace(b), []byte("null"))
}

// Contains reports whether list holds v.
func Contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
