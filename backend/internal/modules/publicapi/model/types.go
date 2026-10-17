// Package model holds the public booking API types (TEC-728,
// docs/design/public-api.md): credentials, the panel's key and webhook
// resources, and the public views of the catalog, quotes and reservations.
// Public views never carry internal ids, staff data, storage keys or rate
// plan internals.
package model

import (
	"encoding/json"
	"time"

	fieldmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Resources (activity entries, live lists).
const (
	ResourceKeys     = "tenant.public_api_keys"
	ResourceWebhooks = "tenant.public_webhooks"
)

// Key kinds.
const (
	KindSecret      = "secret"
	KindPublishable = "publishable"
)

// KeyKinds lists the kinds in display order.
var KeyKinds = []string{KindSecret, KindPublishable}

// Key statuses.
const (
	KeyActive  = "active"
	KeyRevoked = "revoked"
)

// KeyStatuses lists the statuses in display order.
var KeyStatuses = []string{KeyActive, KeyRevoked}

// Channels are the sales channels a key can sell as (orgsettings codes).
var Channels = []string{"web", "widget", "api"}

// Scopes of a key.
const (
	ScopeCatalogRead       = "catalog.read"
	ScopeQuotesWrite       = "quotes.write"
	ScopeReservationsWrite = "reservations.write"
	ScopeReservationsRead  = "reservations.read"
)

// AllScopes lists every scope (the default of a new key).
var AllScopes = []string{ScopeCatalogRead, ScopeQuotesWrite, ScopeReservationsWrite, ScopeReservationsRead}

// Key prefixes. A booking session token is minted from a publishable key.
const (
	PrefixSecret      = "rtp_sk_"
	PrefixPublishable = "rtp_pk_"
	PrefixSession     = "rtp_ps_"
)

// Webhook event types.
const (
	EventReservationCreated    = "reservation.created"
	EventReservationConfirmed  = "reservation.confirmed"
	EventReservationChanged    = "reservation.changed"
	EventReservationCancelled  = "reservation.cancelled"
	EventReservationPlateFixed = "reservation.plate_fixed"
	EventPing                  = "ping"
)

// WebhookEvents lists the events an endpoint can subscribe to.
var WebhookEvents = []string{
	EventReservationCreated, EventReservationConfirmed, EventReservationChanged, EventReservationCancelled,
	EventReservationPlateFixed,
}

// Webhook endpoint and delivery statuses.
const (
	EndpointActive   = "active"
	EndpointDisabled = "disabled"

	DeliveryPending   = "pending"
	DeliverySucceeded = "succeeded"
	DeliveryFailed    = "failed"
)

// EndpointStatuses / DeliveryStatuses list the values in display order.
var (
	EndpointStatuses = []string{EndpointActive, EndpointDisabled}
	DeliveryStatuses = []string{DeliveryPending, DeliverySucceeded, DeliveryFailed}
)

// Credential is the verified caller of a /v1/public request.
type Credential struct {
	OrganizationID   int64
	OrganizationUUID uuid.UUID
	OrganizationSlug string
	OrganizationName string
	OrganizationStat string
	KeyID            int64
	KeyUUID          uuid.UUID
	Kind             string
	Channel          string
	Scopes           []string
	RateLimit        int
	// SessionID is set for booking sessions (browser); "" for secret keys.
	SessionID string
	// Origin is the session's bound origin.
	Origin string
}

// Storefront reports whether the caller is a storefront: the branded site
// or the embeddable widget, which call with a publishable key and browser
// booking sessions. Secret server keys (integrations) are not; the
// storefront publish state of the catalog (show_online, TEC-730) only
// applies to storefronts.
func (c Credential) Storefront() bool { return c.Kind == KindPublishable }

// Has reports whether the credential has a scope.
func (c Credential) Has(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Browser reports whether the caller is a booking session (site / widget).
func (c Credential) Browser() bool { return c.SessionID != "" }

// ------------------------------------------------------------------ panel

// Actor is a staff member embedded in a record.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// Key is an API key in the panel (never the secret itself).
type Key struct {
	UUID               uuid.UUID  `json:"uuid"`
	Kind               string     `json:"kind"`
	Name               string     `json:"name"`
	Channel            string     `json:"channel"`
	KeyPrefix          string     `json:"key_prefix"`
	KeyLast4           string     `json:"key_last4"`
	PublishableKey     *string    `json:"publishable_key"`
	Scopes             []string   `json:"scopes"`
	AllowedOrigins     []string   `json:"allowed_origins"`
	RateLimitPerMinute int        `json:"rate_limit_per_minute"`
	Status             string     `json:"status"`
	ExpiresAt          *time.Time `json:"expires_at"`
	RotatedFromUUID    *uuid.UUID `json:"rotated_from_uuid"`
	LastUsedAt         *time.Time `json:"last_used_at"`
	CreatedBy          *Actor     `json:"created_by"`
	RevokedAt          *time.Time `json:"revoked_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// IssuedKey is a new key: Secret is shown once (secret keys) or is the
// publishable key.
type IssuedKey struct {
	Key    Key    `json:"key"`
	Secret string `json:"secret"`
}

// KeyInput is the create / patch body of a key.
type KeyInput struct {
	Kind               *string   `json:"kind"`
	Name               *string   `json:"name"`
	Channel            *string   `json:"channel"`
	Scopes             *[]string `json:"scopes"`
	AllowedOrigins     *[]string `json:"allowed_origins"`
	RateLimitPerMinute *int      `json:"rate_limit_per_minute"`
}

// RotateInput is POST …/keys/{uuid}/rotate: the old key keeps working for
// GraceHours (0 = revoke now; at most 168).
type RotateInput struct {
	GraceHours *int `json:"grace_hours"`
}

// KeyFilter selects a page of keys.
type KeyFilter struct {
	Q                         string
	Kinds, Channels, Statuses []string
	SortField                 string
	SortDesc                  bool
	Limit, Offset             int32
}

// Endpoint is a webhook endpoint in the panel.
type Endpoint struct {
	UUID                uuid.UUID  `json:"uuid"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	SecretLast4         string     `json:"secret_last4"`
	Events              []string   `json:"events"`
	Status              string     `json:"status"`
	LastSuccessAt       *time.Time `json:"last_success_at"`
	LastFailureAt       *time.Time `json:"last_failure_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// IssuedEndpoint is a new endpoint (or a rotated secret): Secret is shown
// once.
type IssuedEndpoint struct {
	Endpoint Endpoint `json:"endpoint"`
	Secret   string   `json:"secret"`
}

// EndpointInput is the create / patch body of an endpoint.
type EndpointInput struct {
	Name   *string   `json:"name"`
	URL    *string   `json:"url"`
	Events *[]string `json:"events"`
	Status *string   `json:"status"`
}

// EndpointFilter selects a page of endpoints.
type EndpointFilter struct {
	Q             string
	Statuses      []string
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// EndpointRef names the endpoint of a delivery.
type EndpointRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	URL  string    `json:"url"`
}

// Delivery is a webhook delivery in the panel.
type Delivery struct {
	UUID           uuid.UUID       `json:"uuid"`
	Endpoint       EndpointRef     `json:"endpoint"`
	EventType      string          `json:"event_type"`
	EventUUID      uuid.UUID       `json:"event_uuid"`
	Status         string          `json:"status"`
	Attempts       int             `json:"attempts"`
	NextAttemptAt  *time.Time      `json:"next_attempt_at"`
	LastStatusCode *int            `json:"last_status_code"`
	LastError      string          `json:"last_error"`
	DeliveredAt    *time.Time      `json:"delivered_at"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// DeliveryFilter selects a page of deliveries.
type DeliveryFilter struct {
	EndpointUUIDs []uuid.UUID
	Statuses      []string
	EventTypes    []string
	SortDesc      bool
	Limit, Offset int32
}

// DeliveryStats feeds the panel header.
type DeliveryStats struct {
	Failed  int64 `json:"failed"`
	Pending int64 `json:"pending"`
}

// Overview is the panel header of the public API page.
type Overview struct {
	Channels   []ChannelState `json:"channels"`
	Deliveries DeliveryStats  `json:"deliveries"`
	Events     []string       `json:"events"`
	Scopes     []string       `json:"scopes"`
	// BaseURL is where integrators call the API (the app's /api/v1).
	BaseURL string `json:"base_url"`
}

// ChannelState says whether a sales channel the keys sell as is active.
type ChannelState struct {
	Code   string `json:"code"`
	Active bool   `json:"active"`
}

// ------------------------------------------------------------------ public

// SessionInput is POST /v1/public/sessions.
type SessionInput struct {
	PublishableKey string `json:"publishable_key"`
}

// Organization names the organization of a credential.
type Organization struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// Session is a booking session token for a browser.
type Session struct {
	Token        string       `json:"token"`
	ExpiresAt    time.Time    `json:"expires_at"`
	Channel      string       `json:"channel"`
	Organization Organization `json:"organization"`
}

// Point is a service point (office, airport, hotel, address zone).
type Point struct {
	UUID               uuid.UUID       `json:"uuid"`
	Type               string          `json:"type"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	ExtraFee           money.Money     `json:"extra_fee"`
	Delivery           bool            `json:"delivery"`
	MinNoticeMinutes   int             `json:"min_notice_minutes"`
	DeliveryConditions string          `json:"delivery_conditions"`
	Latitude           *float64        `json:"latitude"`
	Longitude          *float64        `json:"longitude"`
	WorkingHours       json.RawMessage `json:"working_hours"`
}

// Location is a branch with its service points.
type Location struct {
	UUID              uuid.UUID       `json:"uuid"`
	Code              string          `json:"code"`
	Name              string          `json:"name"`
	Address           string          `json:"address"`
	City              string          `json:"city"`
	District          string          `json:"district"`
	Latitude          *float64        `json:"latitude"`
	Longitude         *float64        `json:"longitude"`
	Phone             string          `json:"phone"`
	Email             string          `json:"email"`
	WorkingHours      json.RawMessage `json:"working_hours"`
	HolidayExceptions json.RawMessage `json:"holiday_exceptions"`
	Points            []Point         `json:"points"`
}

// Conditions are the rental conditions of a subgroup's current version.
type Conditions struct {
	GuaranteeType    string      `json:"guarantee_type"`
	GuaranteeLabel   string      `json:"guarantee_label"`
	MinDriverAge     int         `json:"min_driver_age"`
	MinLicenseYears  int         `json:"min_license_years"`
	DepositAmount    money.Money `json:"deposit_amount"`
	IncludedKmPerDay *int        `json:"included_km_per_day"`
	ExtraKmFee       money.Money `json:"extra_km_fee"`
	FuelPolicy       string      `json:"fuel_policy"`
}

// Subgroup is a sellable rental product ("alt grup").
type Subgroup struct {
	UUID                uuid.UUID  `json:"uuid"`
	Code                string     `json:"code"`
	Title               string     `json:"title"`
	Description         string     `json:"description"`
	Segment             string     `json:"segment"`
	SegmentCode         string     `json:"segment_code"`
	RepresentativeModel string     `json:"representative_model"`
	Version             int        `json:"version"`
	Conditions          Conditions `json:"conditions"`
	// Images are API paths (relative to the API base, /v1/public/media/…).
	Images []string `json:"images"`
}

// Extra is an extra service sold on the channel (priced in quotes).
type Extra struct {
	UUID        uuid.UUID    `json:"uuid"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Type        string       `json:"type"`
	ChargeType  string       `json:"charge_type"`
	MinQuantity int          `json:"min_quantity"`
	MaxQuantity int          `json:"max_quantity"`
	MaxTotal    *money.Money `json:"max_total"`
}

// Place names a branch or a service point.
type Place struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// SubgroupRef names a subgroup.
type SubgroupRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Code  string    `json:"code"`
	Title string    `json:"title"`
}

// SearchRequest is the resolved availability request.
type SearchRequest struct {
	PickupBranch Place     `json:"pickup_branch"`
	ReturnBranch Place     `json:"return_branch"`
	PickupPoint  *Place    `json:"pickup_point"`
	ReturnPoint  *Place    `json:"return_point"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	Channel      string    `json:"channel"`
}

// Price is the price summary of a subgroup for a request.
type Price struct {
	Currency     string      `json:"currency"`
	BillingDays  int         `json:"billing_days"`
	DailyAverage money.Money `json:"daily_average"`
	Total        money.Money `json:"total"`
	Prepayment   money.Money `json:"prepayment"`
}

// MaxUnitsLeft caps the capacity a public answer reveals.
const MaxUnitsLeft = 10

// AvailabilityItem is one subgroup of an availability answer.
type AvailabilityItem struct {
	Subgroup  SubgroupRef `json:"subgroup"`
	Available bool        `json:"available"`
	// UnitsLeft is the sellable capacity capped at MaxUnitsLeft.
	UnitsLeft int `json:"units_left"`
	// Reasons explain an unavailable subgroup (one_way_not_allowed,
	// no_eligible_vehicle, no_price …).
	Reasons []string `json:"reasons"`
	Price   *Price   `json:"price"`
}

// Availability is GET /v1/public/availability.
type Availability struct {
	Request  SearchRequest      `json:"request"`
	Items    []AvailabilityItem `json:"items"`
	Warnings []string           `json:"warnings"`
}

// ExtraRequest is an extra asked for in a quote.
type ExtraRequest struct {
	ExtraUUID uuid.UUID `json:"extra_uuid"`
	Quantity  int       `json:"quantity"`
}

// QuoteInput is POST /v1/public/quotes.
type QuoteInput struct {
	SubgroupUUID     *uuid.UUID           `json:"subgroup_uuid"`
	PickupBranchUUID *uuid.UUID           `json:"pickup_branch_uuid"`
	PickupPointUUID  *uuid.UUID           `json:"pickup_point_uuid"`
	ReturnBranchUUID *uuid.UUID           `json:"return_branch_uuid"`
	ReturnPointUUID  *uuid.UUID           `json:"return_point_uuid"`
	StartsAt         *time.Time           `json:"starts_at"`
	EndsAt           *time.Time           `json:"ends_at"`
	Extras           []ExtraRequest       `json:"extras"`
	CouponCode       string               `json:"coupon_code"`
	PickupDelivery   *fieldmodel.Delivery `json:"pickup_delivery"`
	ReturnDelivery   *fieldmodel.Delivery `json:"return_delivery"`
}

// QuoteLine is one priced line of a quote.
type QuoteLine struct {
	Kind      string      `json:"kind"`
	Code      string      `json:"code"`
	Name      string      `json:"name"`
	DayDate   *string     `json:"day_date"`
	Quantity  int         `json:"quantity"`
	UnitPrice money.Money `json:"unit_price"`
	Amount    money.Money `json:"amount"`
}

// Quote is a stored price snapshot the customer can book until ValidUntil.
type Quote struct {
	UUID          uuid.UUID      `json:"uuid"`
	Status        string         `json:"status"`
	Expired       bool           `json:"expired"`
	ValidUntil    time.Time      `json:"valid_until"`
	Subgroup      SubgroupRef    `json:"subgroup"`
	PickupBranch  Place          `json:"pickup_branch"`
	ReturnBranch  Place          `json:"return_branch"`
	PickupPoint   *Place         `json:"pickup_point"`
	ReturnPoint   *Place         `json:"return_point"`
	StartsAt      time.Time      `json:"starts_at"`
	EndsAt        time.Time      `json:"ends_at"`
	Channel       string         `json:"channel"`
	BillingDays   int            `json:"billing_days"`
	Currency      string         `json:"currency"`
	RentalTotal   money.Money    `json:"rental_total"`
	DiscountTotal money.Money    `json:"discount_total"`
	ExtrasTotal   money.Money    `json:"extras_total"`
	OneWayFee     money.Money    `json:"one_way_fee"`
	DeliveryFee   money.Money    `json:"delivery_fee"`
	VATTotal      money.Money    `json:"vat_total"`
	Total         money.Money    `json:"total"`
	Prepayment    money.Money    `json:"prepayment"`
	Extras        []ExtraRequest `json:"extras"`
	CouponCode    *string        `json:"coupon_code"`
	Lines         []QuoteLine    `json:"lines"`
}

// Person is the renter of a public reservation.
type Person struct {
	Type         string `json:"type"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	CompanyTitle string `json:"company_title"`
	TaxOffice    string `json:"tax_office"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Nationality  string `json:"nationality"`
}

// DriverInput is a driver of a public reservation (the first is the main
// driver). Licence data is needed for the sold product's conditions.
type DriverInput struct {
	SameAsCustomer   bool   `json:"same_as_customer"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name"`
	BirthDate        string `json:"birth_date"`
	Phone            string `json:"phone"`
	Email            string `json:"email"`
	LicenceNumber    string `json:"licence_number"`
	LicenceClass     string `json:"licence_class"`
	LicenceIssuedOn  string `json:"licence_issued_on"`
	LicenceExpiresOn string `json:"licence_expires_on"`
	LicenceCountry   string `json:"licence_country"`
}

// ReservationInput is POST /v1/public/reservations.
type ReservationInput struct {
	QuoteUUID         *uuid.UUID    `json:"quote_uuid"`
	Customer          *Person       `json:"customer"`
	Drivers           []DriverInput `json:"drivers"`
	Note              string        `json:"note"`
	ExternalReference string        `json:"external_reference"`
}

// Money is the money summary of a public reservation.
type Money struct {
	Currency      string      `json:"currency"`
	Total         money.Money `json:"total"`
	Prepayment    money.Money `json:"prepayment"`
	Paid          money.Money `json:"paid"`
	PrepaymentDue money.Money `json:"prepayment_due"`
	Balance       money.Money `json:"balance"`
}

// ReservationExtra is an extra line of a reservation.
type ReservationExtra struct {
	Name     string      `json:"name"`
	Quantity int         `json:"quantity"`
	Amount   money.Money `json:"amount"`
}

// Policies are the policy texts the reservation was sold under.
type Policies struct {
	Cancellation string `json:"cancellation"`
	NoShow       string `json:"no_show"`
	EarlyReturn  string `json:"early_return"`
	Extension    string `json:"extension"`
	Fuel         string `json:"fuel"`
	Mileage      string `json:"mileage"`
}

// Cancellation is the cancellation terms of the reservation.
type Cancellation struct {
	PolicyText string       `json:"policy_text"`
	Fee        *money.Money `json:"fee"`
	SelfCancel bool         `json:"self_cancel"`
}

// Actions are what the customer may ask for now.
type Actions struct {
	Pay                 bool         `json:"pay"`
	PayAmount           *money.Money `json:"pay_amount"`
	RequestChange       bool         `json:"request_change"`
	RequestCancellation bool         `json:"request_cancellation"`
}

// Request is a change or cancellation request.
type Request struct {
	UUID              uuid.UUID    `json:"uuid"`
	Type              string       `json:"type"`
	Status            string       `json:"status"`
	Message           string       `json:"message"`
	RequestedStartsAt *time.Time   `json:"requested_starts_at"`
	RequestedEndsAt   *time.Time   `json:"requested_ends_at"`
	PolicyText        string       `json:"policy_text"`
	CancellationFee   *money.Money `json:"cancellation_fee"`
	Outcome           string       `json:"outcome"`
	ResolutionNote    string       `json:"resolution_note"`
	ResolvedAt        *time.Time   `json:"resolved_at"`
	CreatedAt         time.Time    `json:"created_at"`
}

// Reservation is the public view of a reservation.
type Reservation struct {
	Reference       string             `json:"reference"`
	Status          string             `json:"status"`
	PaymentStatus   string             `json:"payment_status"`
	StartsAt        time.Time          `json:"starts_at"`
	EndsAt          time.Time          `json:"ends_at"`
	PickupBranch    string             `json:"pickup_branch"`
	ReturnBranch    string             `json:"return_branch"`
	PickupPoint     string             `json:"pickup_point"`
	ReturnPoint     string             `json:"return_point"`
	Subgroup        string             `json:"subgroup"`
	Vehicle         string             `json:"vehicle"`
	Drivers         []string           `json:"drivers"`
	Extras          []ReservationExtra `json:"extras"`
	Price           Money              `json:"price"`
	PaymentDeadline *time.Time         `json:"payment_deadline"`
	Policies        Policies           `json:"policies"`
	Cancellation    Cancellation       `json:"cancellation"`
	Actions         Actions            `json:"actions"`
	Requests        []Request          `json:"requests"`
}

// Booked is the answer of POST /v1/public/reservations. AccessToken is
// shown once: status, requests and payment links need it
// (X-Reservation-Token).
type Booked struct {
	Reference   string      `json:"reference"`
	Status      string      `json:"status"`
	AccessToken string      `json:"access_token"`
	Reservation Reservation `json:"reservation"`
}

// ChangeRequestInput is POST …/change-requests.
type ChangeRequestInput struct {
	Message           string     `json:"message"`
	RequestedStartsAt *time.Time `json:"requested_starts_at"`
	RequestedEndsAt   *time.Time `json:"requested_ends_at"`
}

// CancellationRequestInput is POST …/cancellation-requests.
type CancellationRequestInput struct {
	Message string `json:"message"`
}

// PaymentLink is POST …/payment-links: the hosted pay page of the amount
// due now.
type PaymentLink struct {
	URL       string      `json:"url"`
	Amount    money.Money `json:"amount"`
	Currency  string      `json:"currency"`
	ExpiresAt time.Time   `json:"expires_at"`
	Reused    bool        `json:"reused"`
}

// WebhookReservation is the reservation in a webhook payload.
type WebhookReservation struct {
	Reference     string      `json:"reference"`
	Status        string      `json:"status"`
	PaymentStatus string      `json:"payment_status"`
	StartsAt      time.Time   `json:"starts_at"`
	EndsAt        time.Time   `json:"ends_at"`
	PickupBranch  Place       `json:"pickup_branch"`
	ReturnBranch  Place       `json:"return_branch"`
	Subgroup      SubgroupRef `json:"subgroup"`
	Channel       string      `json:"channel"`
	Currency      string      `json:"currency"`
	Total         money.Money `json:"total"`
	Prepayment    money.Money `json:"prepayment"`
	Paid          money.Money `json:"paid"`
	External      string      `json:"external_reference"`
}

// WebhookEvent is the JSON body POSTed to an endpoint.
type WebhookEvent struct {
	ID           uuid.UUID `json:"id"`
	Type         string    `json:"type"`
	CreatedAt    time.Time `json:"created_at"`
	Organization string    `json:"organization"`
	Data         any       `json:"data"`
}
