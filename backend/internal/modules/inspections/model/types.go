// Package model holds the types of vehicle inspections (TEC-712 handover,
// TEC-713 return, TEC-714 evidence): templates, forms, damages, photos,
// signatures, the charge preview and the inputs. docs/design/inspections.md
// is the design.
package model

import (
	"encoding/json"
	"strings"
	"time"

	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Live list / activity resources.
const (
	ResourceInspections = "tenant.inspections"
	ResourceTemplates   = "tenant.inspection_templates"
)

// Kinds.
const (
	KindHandover = "handover"
	KindReturn   = "return"
)

// Kinds lists the inspection kinds.
var Kinds = []string{KindHandover, KindReturn}

// Statuses.
const (
	StatusDraft      = "draft"
	StatusFinalized  = "finalized"
	StatusSuperseded = "superseded"
)

// Statuses lists the inspection statuses.
var Statuses = []string{StatusDraft, StatusFinalized, StatusSuperseded}

// Review statuses (exception path of a return).
const (
	ReviewNone     = "none"
	ReviewPending  = "pending"
	ReviewResolved = "resolved"
)

// ReviewStatuses lists the review statuses.
var ReviewStatuses = []string{ReviewNone, ReviewPending, ReviewResolved}

// Signature statuses.
const (
	SignaturePending = "pending"
	SignatureSigned  = "signed"
	// SignatureRefused: a return recorded without the customer's
	// signature (absent or refused) — exception path.
	SignatureRefused = "refused"
	// SignatureWaived: a handover finalized without a signature by a
	// member with tenant.signature.waive.
	SignatureWaived = "waived"
)

// Photo statuses.
const (
	PhotoPending   = "pending"
	PhotoUploaded  = "uploaded"
	PhotoCompleted = "completed"
)

// Special photo slots (besides the template's).
const (
	SlotDamage = "damage"
	SlotExtra  = "extra"
)

// Scan results (optional malware scanner).
const (
	ScanSkipped  = "skipped"
	ScanClean    = "clean"
	ScanInfected = "infected"
)

// Damage kinds and severities.
var (
	DamageTypes = []string{"scratch", "dent", "crack", "broken", "missing", "stain", "other"}
	Severities  = []string{"minor", "moderate", "major"}
)

// Zones are the parts of the vehicle diagram a damage can be placed on.
var Zones = []string{
	"front_bumper", "hood", "windshield", "roof", "rear_window", "trunk", "rear_bumper",
	"left_front_fender", "left_front_door", "left_rear_door", "left_rear_fender", "left_mirror",
	"right_front_fender", "right_front_door", "right_rear_door", "right_rear_fender", "right_mirror",
	"left_front_wheel", "left_rear_wheel", "right_front_wheel", "right_rear_wheel", "interior",
}

// Vehicle statuses a return may leave the vehicle in.
var VehicleStatusesAfter = []string{"cleaning", "available", "maintenance", "preparing"}

// Photo limits: images only, at most MaxPhotoBytes.
const MaxPhotoBytes = 15 << 20

// PhotoContentTypes are the accepted image types.
var PhotoContentTypes = []string{"image/jpeg", "image/png", "image/webp", "image/heic", "image/heif"}

// Ref is a uuid with a display label.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code,omitempty"`
	Name string    `json:"name"`
}

// Actor is a user embedded in a record.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ----------------------------------------------------------- templates

// Slot is a required (or optional) photo of the template.
type Slot struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

// Item is a checklist / equipment item. Price ("" = none) is charged when
// the item is missing at the return.
type Item struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Price    string `json:"price"`
}

// Template is an inspection template version.
type Template struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Version     int32     `json:"version"`
	Status      string    `json:"status"`
	PhotoSlots  []Slot    `json:"photo_slots"`
	Checklist   []Item    `json:"checklist"`
	Inspections int64     `json:"inspections"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TemplateInput saves a new template version (it becomes the active one).
type TemplateInput struct {
	Name       string `json:"name"`
	PhotoSlots []Slot `json:"photo_slots"`
	Checklist  []Item `json:"checklist"`
}

// DefaultTemplate is used when an organization has none yet: the six
// standard photos and the usual equipment.
func DefaultTemplate() TemplateInput {
	return TemplateInput{
		Name: "Standart teslim / iade formu",
		PhotoSlots: []Slot{
			{Key: "front", Label: "Ön", Required: true}, {Key: "rear", Label: "Arka", Required: true},
			{Key: "left", Label: "Sol", Required: true}, {Key: "right", Label: "Sağ", Required: true},
			{Key: "interior", Label: "İç", Required: true}, {Key: "dashboard", Label: "Gösterge", Required: true},
		},
		Checklist: []Item{
			{Key: "spare_tire", Label: "Stepne", Required: true}, {Key: "jack", Label: "Kriko", Required: true},
			{Key: "warning_triangle", Label: "Reflektör", Required: true}, {Key: "first_aid", Label: "İlk yardım çantası", Required: true},
			{Key: "fire_extinguisher", Label: "Yangın tüpü", Required: true}, {Key: "registration", Label: "Ruhsat", Required: true},
			{Key: "keys", Label: "Anahtar", Required: true},
		},
	}
}

// TemplateSnapshot is the template copy stored on an inspection.
type TemplateSnapshot struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Version    int32     `json:"version"`
	PhotoSlots []Slot    `json:"photo_slots"`
	Checklist  []Item    `json:"checklist"`
}

// --------------------------------------------------------- inspections

// Verification is step 1 of the wizard.
type Verification struct {
	PlateConfirmed  bool       `json:"plate_confirmed"`
	VehicleOK       bool       `json:"vehicle_ok"`
	DriverUUID      *uuid.UUID `json:"driver_uuid"`
	LicenceChecked  bool       `json:"licence_checked"`
	IdentityChecked bool       `json:"identity_checked"`
}

// ChargeOverride is a staff change of a computed / proposed amount in a
// return draft. Keys: late_return, mileage, fuel, equipment:<item>,
// damage:<damage uuid>.
type ChargeOverride struct {
	Key    string      `json:"key"`
	Amount money.Money `json:"amount"`
	Reason string      `json:"reason"`
}

// Damage is a finding on the vehicle diagram.
type Damage struct {
	UUID        uuid.UUID `json:"uuid"`
	Zone        string    `json:"zone"`
	PosX        *float64  `json:"pos_x"`
	PosY        *float64  `json:"pos_y"`
	DamageType  string    `json:"damage_type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	IsNew       bool      `json:"is_new"`
	Disputed    bool      `json:"disputed"`
	CreatedAt   time.Time `json:"created_at"`
}

// PhotoEXIF are the device facts read from the file (never server time).
type PhotoEXIF struct {
	TakenAt    *time.Time `json:"taken_at"`
	TakenLocal string     `json:"taken_local"`
	GPSLat     *float64   `json:"gps_lat"`
	GPSLng     *float64   `json:"gps_lng"`
	Make       string     `json:"make"`
	Model      string     `json:"model"`
}

// Photo is one piece of evidence.
type Photo struct {
	UUID              uuid.UUID  `json:"uuid"`
	Slot              string     `json:"slot"`
	DamageUUID        *uuid.UUID `json:"damage_uuid"`
	ClientRef         string     `json:"client_ref"`
	Status            string     `json:"status"`
	ContentType       string     `json:"content_type"`
	DeclaredSizeBytes int64      `json:"declared_size_bytes"`
	SizeBytes         int64      `json:"size_bytes"`
	SHA256            string     `json:"sha256"`
	InspectionVersion int32      `json:"inspection_version"`
	// UploadedAt is the server time of the completion.
	UploadedAt *time.Time `json:"uploaded_at"`
	UploadedBy *Actor     `json:"uploaded_by"`
	// DeviceCapturedAt is the device clock (file time) the client sent.
	DeviceCapturedAt *time.Time `json:"device_captured_at"`
	EXIF             PhotoEXIF  `json:"exif"`
	ScanStatus       string     `json:"scan_status"`
	Copied           bool       `json:"copied"`
	CreatedAt        time.Time  `json:"created_at"`
}

// UploadTarget is the API-issued upload address of a photo (same origin:
// the browser prefixes the BFF base, never an object store host).
type UploadTarget struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Field    string `json:"field"`
	MaxBytes int64  `json:"max_bytes"`
}

// PhotoUpload answers a photo creation.
type PhotoUpload struct {
	Photo  Photo        `json:"photo"`
	Upload UploadTarget `json:"upload"`
}

// Signature is the signature canvas of a form.
type Signature struct {
	UUID           uuid.UUID  `json:"uuid"`
	SignerName     string     `json:"signer_name"`
	SignerRole     string     `json:"signer_role"`
	SHA256         string     `json:"sha256"`
	SizeBytes      int64      `json:"size_bytes"`
	PointCount     int32      `json:"point_count"`
	SignedAt       time.Time  `json:"signed_at"`
	DeviceSignedAt *time.Time `json:"device_signed_at"`
	// The contract approval given with this signature (if any).
	ContractApproval *ContractApproval `json:"contract_approval"`
}

// ContractApproval is the contract version a signature approved.
type ContractApproval struct {
	ApprovalUUID uuid.UUID `json:"approval_uuid"`
	DocumentUUID uuid.UUID `json:"document_uuid"`
	VersionUUID  uuid.UUID `json:"version_uuid"`
	ContentHash  string    `json:"content_hash"`
}

// Readiness tells what still blocks the completion (RT-A17: every photo
// row uploaded and completed, every required slot and damage covered).
type Readiness struct {
	RequiredSlots       []string    `json:"required_slots"`
	MissingSlots        []string    `json:"missing_slots"`
	DamagesWithoutPhoto []uuid.UUID `json:"damages_without_photo"`
	IncompletePhotos    int         `json:"incomplete_photos"`
	MissingFields       []string    `json:"missing_fields"`
	PhotosReady         bool        `json:"photos_ready"`
	CanComplete         bool        `json:"can_complete"`
}

// Amounts is the money side of the rental (display only: payments).
type Amounts struct {
	Currency      string      `json:"currency"`
	Total         money.Money `json:"total"`
	Paid          money.Money `json:"paid"`
	Balance       money.Money `json:"balance"`
	PaymentStatus string      `json:"payment_status"`
	DepositAmount string      `json:"deposit_amount"`
	// DepositStatus: not_tracked (deposits / provisions are taken at the
	// branch; payments keeps no deposit record yet).
	DepositStatus string `json:"deposit_status"`
}

// RentalSummary is what the wizard shows of the rental.
type RentalSummary struct {
	UUID            uuid.UUID                 `json:"uuid"`
	Reference       string                    `json:"reference"`
	Status          string                    `json:"status"`
	Customer        rentalmodel.CustomerRef   `json:"customer"`
	Vehicle         rentalmodel.VehicleRef    `json:"vehicle"`
	Product         rentalmodel.Ref           `json:"product"`
	Drivers         []rentalmodel.Driver      `json:"drivers"`
	PlannedPickupAt time.Time                 `json:"planned_pickup_at"`
	PlannedReturnAt time.Time                 `json:"planned_return_at"`
	PlannedPickup   rentalmodel.Place         `json:"planned_pickup"`
	PlannedReturn   rentalmodel.Place         `json:"planned_return"`
	ActualPickupAt  *time.Time                `json:"actual_pickup_at"`
	ActualReturnAt  *time.Time                `json:"actual_return_at"`
	Contract        rentalmodel.ContractState `json:"contract"`
	Amounts         Amounts                   `json:"amounts"`
	Legs            []rentalmodel.Leg         `json:"legs"`
}

// Baseline is the finalized handover record a return is compared with.
type Baseline struct {
	UUID       uuid.UUID       `json:"uuid"`
	Version    int32           `json:"version"`
	OccurredAt *time.Time      `json:"occurred_at"`
	Km         *int32          `json:"km"`
	Fuel       *int16          `json:"fuel"`
	Equipment  map[string]bool `json:"equipment"`
	Damages    []Damage        `json:"damages"`
	Photos     []Photo         `json:"photos"`
}

// Inspection is a handover / return form as the API shows it.
type Inspection struct {
	UUID               uuid.UUID        `json:"uuid"`
	Kind               string           `json:"kind"`
	Version            int32            `json:"version"`
	Status             string           `json:"status"`
	Rental             RentalSummary    `json:"rental"`
	Template           TemplateSnapshot `json:"template"`
	PreviousUUID       *uuid.UUID       `json:"previous_uuid"`
	RevisionReason     string           `json:"revision_reason"`
	OccurredAt         *time.Time       `json:"occurred_at"`
	Branch             *Ref             `json:"branch"`
	Point              *Ref             `json:"point"`
	Km                 *int32           `json:"km"`
	Fuel               *int16           `json:"fuel"`
	Equipment          map[string]bool  `json:"equipment"`
	Verification       Verification     `json:"verification"`
	Note               string           `json:"note"`
	CustomerStatement  string           `json:"customer_statement"`
	CustomerReviewedAt *time.Time       `json:"customer_reviewed_at"`
	ChargeOverrides    []ChargeOverride `json:"charge_overrides"`
	VehicleStatusAfter *string          `json:"vehicle_status_after"`
	SignatureStatus    string           `json:"signature_status"`
	SignatureNote      string           `json:"signature_note"`
	Signature          *Signature       `json:"signature"`
	Disputed           bool             `json:"disputed"`
	DisputeNote        string           `json:"dispute_note"`
	ReviewStatus       string           `json:"review_status"`
	ReviewedAt         *time.Time       `json:"reviewed_at"`
	ReviewedBy         *Actor           `json:"reviewed_by"`
	ReviewNote         string           `json:"review_note"`
	FinalizedAt        *time.Time       `json:"finalized_at"`
	FinalizedBy        *Actor           `json:"finalized_by"`
	CreatedBy          *Actor           `json:"created_by"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
	Damages            []Damage         `json:"damages"`
	Photos             []Photo          `json:"photos"`
	Readiness          Readiness        `json:"readiness"`
	Baseline           *Baseline        `json:"baseline"`
	Versions           []VersionRef     `json:"versions"`
}

// VersionRef is one version of the rental's form of the same kind.
type VersionRef struct {
	UUID        uuid.UUID  `json:"uuid"`
	Version     int32      `json:"version"`
	Status      string     `json:"status"`
	FinalizedAt *time.Time `json:"finalized_at"`
}

// ListItem is a row of the inspections list (review queue).
type ListItem struct {
	UUID            uuid.UUID               `json:"uuid"`
	Kind            string                  `json:"kind"`
	Version         int32                   `json:"version"`
	Status          string                  `json:"status"`
	SignatureStatus string                  `json:"signature_status"`
	Disputed        bool                    `json:"disputed"`
	ReviewStatus    string                  `json:"review_status"`
	Km              *int32                  `json:"km"`
	Fuel            *int16                  `json:"fuel"`
	Photos          int64                   `json:"photos"`
	Damages         int64                   `json:"damages"`
	Rental          RentalRef               `json:"rental"`
	Vehicle         rentalmodel.VehicleRef  `json:"vehicle"`
	Customer        rentalmodel.CustomerRef `json:"customer"`
	OccurredAt      *time.Time              `json:"occurred_at"`
	FinalizedAt     *time.Time              `json:"finalized_at"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

// RentalRef is the rental of a list row.
type RentalRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	Status    string    `json:"status"`
}

// FacetValue is one option of a list facet.
type FacetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the filter options of the inspections list.
type Facets struct {
	Kind         []FacetValue `json:"kind"`
	Status       []FacetValue `json:"status"`
	ReviewStatus []FacetValue `json:"review_status"`
}

// ListFilter selects a page of inspections.
type ListFilter struct {
	Q              string
	Kinds          []string
	Statuses       []string
	ReviewStatuses []string
	RentalUUIDs    []uuid.UUID
	DisputedOnly   bool
	SortField      string
	SortDesc       bool
	Limit, Offset  int32
}

// QueryKeys are the list filter query keys.
var QueryKeys = []string{"kind", "status", "review_status", "rental", "disputed"}

// Sortable maps the sortable list fields.
var Sortable = map[string]string{"created_at": "created_at", "finalized_at": "finalized_at", "kind": "kind"}

// FilterFromQuery builds a filter from comma-joined query values.
func FilterFromQuery(v map[string]string) ListFilter {
	f := ListFilter{Q: strings.TrimSpace(v["q"])}
	f.Kinds = allowed(split(v["kind"]), Kinds)
	f.Statuses = allowed(split(v["status"]), Statuses)
	f.ReviewStatuses = allowed(split(v["review_status"]), ReviewStatuses)
	for _, raw := range split(v["rental"]) {
		if id, err := uuid.Parse(raw); err == nil {
			f.RentalUUIDs = append(f.RentalUUIDs, id)
		}
	}
	f.DisputedOnly = v["disputed"] == "true" || v["disputed"] == "1"
	return f
}

// ----------------------------------------------------------- charges

// ChargeLine is one computed (or staff entered) return charge.
type ChargeLine struct {
	Key         string          `json:"key"`
	Kind        string          `json:"kind"`
	Source      string          `json:"source"`
	Currency    string          `json:"currency"`
	Amount      money.Money     `json:"amount"`
	Computed    *money.Money    `json:"computed"`
	Overridden  bool            `json:"overridden"`
	Reason      string          `json:"reason"`
	Calculation json.RawMessage `json:"calculation"`
	Disputed    bool            `json:"disputed"`
}

// ChargePreview is the charge proposal of a return draft (nothing is
// stored until the form is finalized).
type ChargePreview struct {
	Currency        string       `json:"currency"`
	SettingsVersion int          `json:"settings_version"`
	LateMinutes     int32        `json:"late_minutes"`
	DrivenKm        int64        `json:"driven_km"`
	Lines           []ChargeLine `json:"lines"`
	Total           money.Money  `json:"total"`
}

// ------------------------------------------------------------- inputs

// StartInput starts (or resumes) the form of a rental.
type StartInput struct {
	RentalUUID *uuid.UUID `json:"rental_uuid"`
	Kind       string     `json:"kind"`
}

// DraftInput patches a draft (omitted fields keep their value).
type DraftInput struct {
	OccurredAt         *time.Time        `json:"occurred_at"`
	BranchUUID         *uuid.UUID        `json:"branch_uuid"`
	PointUUID          *uuid.UUID        `json:"point_uuid"`
	Km                 *int32            `json:"km"`
	Fuel               *int16            `json:"fuel"`
	Equipment          map[string]bool   `json:"equipment"`
	Verification       *Verification     `json:"verification"`
	Note               *string           `json:"note"`
	CustomerStatement  *string           `json:"customer_statement"`
	CustomerReviewed   *bool             `json:"customer_reviewed"`
	ChargeOverrides    *[]ChargeOverride `json:"charge_overrides"`
	VehicleStatusAfter *string           `json:"vehicle_status_after"`
	Disputed           *bool             `json:"disputed"`
	DisputeNote        *string           `json:"dispute_note"`
}

// DamageInput creates or replaces a damage.
type DamageInput struct {
	Zone        string   `json:"zone"`
	PosX        *float64 `json:"pos_x"`
	PosY        *float64 `json:"pos_y"`
	DamageType  string   `json:"damage_type"`
	Severity    string   `json:"severity"`
	Description string   `json:"description"`
	IsNew       *bool    `json:"is_new"`
	Disputed    bool     `json:"disputed"`
}

// PhotoInput announces a photo (the upload follows).
type PhotoInput struct {
	Slot             string     `json:"slot"`
	DamageUUID       *uuid.UUID `json:"damage_uuid"`
	ClientRef        string     `json:"client_ref"`
	ContentType      string     `json:"content_type"`
	SizeBytes        int64      `json:"size_bytes"`
	DeviceCapturedAt *time.Time `json:"device_captured_at"`
}

// SignatureInput records the signature canvas. Contract approves the
// rental contract version shown to the customer with this signature.
type SignatureInput struct {
	SignerName     string             `json:"signer_name"`
	SignerRole     string             `json:"signer_role"`
	PNGBase64      string             `json:"png_base64"`
	Strokes        json.RawMessage    `json:"strokes"`
	DeviceSignedAt *time.Time         `json:"device_signed_at"`
	Contract       *ContractSignInput `json:"contract"`
}

// ContractSignInput names the contract version the customer signed.
type ContractSignInput struct {
	DocumentUUID uuid.UUID `json:"document_uuid"`
	VersionUUID  uuid.UUID `json:"version_uuid"`
	ContentHash  string    `json:"content_hash"`
}

// CompleteInput finalizes the form.
type CompleteInput struct {
	// NoSignatureReason: handover without a signature (waived,
	// tenant.signature.waive) or a return the customer did not sign
	// (exception path, review queue).
	NoSignatureReason string `json:"no_signature_reason"`
	// ContractWaiverReason: hand over without an approved rental contract
	// (tenant.signature.waive, the rentals rule).
	ContractWaiverReason string `json:"contract_waiver_reason"`
}

// ReasonInput carries a reason.
type ReasonInput struct {
	Reason string `json:"reason"`
}

// ReviewInput resolves the review of a disputed / unsigned return.
type ReviewInput struct {
	Note          string `json:"note"`
	VehicleStatus string `json:"vehicle_status"`
}

// MediaLinksInput asks for short-lived links (empty = every completed
// photo and the signature of the form).
type MediaLinksInput struct {
	PhotoUUIDs []uuid.UUID `json:"photo_uuids"`
	Signature  bool        `json:"signature"`
}

// MediaLink is a short-lived API link to a photo or signature.
type MediaLink struct {
	UUID      uuid.UUID `json:"uuid"`
	Kind      string    `json:"kind"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// MediaLinks answers a media link request.
type MediaLinks struct {
	Items []MediaLink `json:"items"`
}

// --------------------------------------------------------------- helpers

func split(raw string) []string {
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func allowed(values, set []string) []string {
	out := []string{}
	for _, v := range values {
		if Contains(set, v) {
			out = append(out, v)
		}
	}
	return out
}

// Contains reports whether set holds v.
func Contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
