// Package model holds the API shapes of the customers tenant resource
// (TEC-705): customers (individual / corporate), corporate contacts,
// marketing consents, drivers and customer documents.
//
// Sensitive values (identity / licence numbers, phone, e-mail) are always
// masked in these shapes (*_masked); the plaintext only leaves the API
// through the reveal endpoint (Revealed), which needs
// tenant.customers.reveal and records an activity event with the reason.
package model

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Resource is the stable slug of the customers resource (activity, io,
// bulk, live events).
const Resource = "tenant.customers"

// Customer types.
const (
	TypeIndividual = "individual"
	TypeCorporate  = "corporate"
)

// CustomerTypes lists the customer types.
var CustomerTypes = []string{TypeIndividual, TypeCorporate}

// Identity types.
const (
	IdentityTCKN     = "tckn"
	IdentityVKN      = "vkn"
	IdentityPassport = "passport"
)

// Consent channels, actions and sources.
var (
	ConsentChannels = []string{"sms", "email", "call", "whatsapp"}
	ConsentSources  = []string{"counter", "web", "phone", "import", "other"}
)

// Document types of customer documents.
var DocumentTypes = []string{
	"identity", "passport", "driving_licence", "tax_certificate", "signature_circular", "contract", "other",
}

// RevealDisplaySeconds is how long the UI shows revealed values before it
// masks them again (a hint; the server keeps no reveal state).
const RevealDisplaySeconds = 120

// Billing is the invoice information of a customer.
type Billing struct {
	Address    string  `json:"address"`
	City       string  `json:"city"`
	District   string  `json:"district"`
	PostalCode string  `json:"postal_code"`
	Country    *string `json:"country"`
	EInvoice   bool    `json:"e_invoice"`
}

// Customer is the API projection of a customer (sensitive values masked).
type Customer struct {
	UUID         uuid.UUID `json:"uuid"`
	CustomerType string    `json:"customer_type"`
	// DisplayName is the company title or "<first> <last>".
	DisplayName    string   `json:"display_name"`
	FirstName      string   `json:"first_name"`
	LastName       string   `json:"last_name"`
	CompanyTitle   string   `json:"company_title"`
	IdentityType   *string  `json:"identity_type"`
	IdentityMasked string   `json:"identity_masked"`
	Nationality    *string  `json:"nationality"`
	TaxOffice      string   `json:"tax_office"`
	PhoneMasked    string   `json:"phone_masked"`
	EmailMasked    string   `json:"email_masked"`
	Billing        Billing  `json:"billing"`
	Note           string   `json:"note"`
	Tags           []string `json:"tags"`
	// ConsentChannels are the channels with an open marketing consent.
	ConsentChannels []string   `json:"consent_channels"`
	DriversCount    int64      `json:"drivers_count"`
	Anonymized      bool       `json:"anonymized"`
	AnonymizedAt    *time.Time `json:"anonymized_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CustomerInput is the create / patch body. Nil fields are left unchanged
// on patch. Sensitive values are write-only: identity_number, phone and
// email are accepted in plaintext and answered masked. An empty
// identity_number clears the identity.
type CustomerInput struct {
	CustomerType      *string        `json:"customer_type"`
	FirstName         *string        `json:"first_name"`
	LastName          *string        `json:"last_name"`
	CompanyTitle      *string        `json:"company_title"`
	IdentityType      OptionalString `json:"identity_type"`
	IdentityNumber    *string        `json:"identity_number"`
	Nationality       OptionalString `json:"nationality"`
	TaxOffice         *string        `json:"tax_office"`
	Phone             *string        `json:"phone"`
	Email             *string        `json:"email"`
	BillingAddress    *string        `json:"billing_address"`
	BillingCity       *string        `json:"billing_city"`
	BillingDistrict   *string        `json:"billing_district"`
	BillingPostalCode *string        `json:"billing_postal_code"`
	BillingCountry    OptionalString `json:"billing_country"`
	EInvoice          *bool          `json:"e_invoice"`
	Note              *string        `json:"note"`
	Tags              *[]string      `json:"tags"`
}

// RevealInput is the body of POST /{uuid}/reveal.
type RevealInput struct {
	Reason string `json:"reason"`
}

// RevealedContact is the plaintext contact data of a corporate contact.
type RevealedContact struct {
	UUID  uuid.UUID `json:"uuid"`
	Phone string    `json:"phone"`
	Email string    `json:"email"`
}

// RevealedDriver is the plaintext identity / licence / contact data of a
// driver.
type RevealedDriver struct {
	UUID           uuid.UUID `json:"uuid"`
	IdentityType   *string   `json:"identity_type"`
	IdentityNumber string    `json:"identity_number"`
	LicenceNumber  string    `json:"licence_number"`
	Phone          string    `json:"phone"`
	Email          string    `json:"email"`
}

// Revealed is the answer of POST /{uuid}/reveal.
type Revealed struct {
	CustomerUUID   uuid.UUID         `json:"customer_uuid"`
	IdentityType   *string           `json:"identity_type"`
	IdentityNumber string            `json:"identity_number"`
	Phone          string            `json:"phone"`
	Email          string            `json:"email"`
	Contacts       []RevealedContact `json:"contacts"`
	Drivers        []RevealedDriver  `json:"drivers"`
	RevealedAt     time.Time         `json:"revealed_at"`
	// DisplaySeconds is how long the UI may show the values.
	DisplaySeconds int `json:"display_seconds"`
}

// AnonymizeInput is the body of POST /{uuid}/anonymize.
type AnonymizeInput struct {
	Reason string `json:"reason"`
}

// Contact is a contact person of a corporate customer.
type Contact struct {
	UUID        uuid.UUID `json:"uuid"`
	FullName    string    `json:"full_name"`
	Title       string    `json:"title"`
	PhoneMasked string    `json:"phone_masked"`
	EmailMasked string    `json:"email_masked"`
	IsPrimary   bool      `json:"is_primary"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

// ContactInput is the create / patch body of a contact.
type ContactInput struct {
	FullName  *string `json:"full_name"`
	Title     *string `json:"title"`
	Phone     *string `json:"phone"`
	Email     *string `json:"email"`
	IsPrimary *bool   `json:"is_primary"`
	Note      *string `json:"note"`
}

// Actor is the user behind a record or an activity entry.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email,omitempty"`
}

// Consent is one marketing consent record (a grant and its withdrawal).
type Consent struct {
	UUID             uuid.UUID  `json:"uuid"`
	Channel          string     `json:"channel"`
	Active           bool       `json:"active"`
	GrantedAt        time.Time  `json:"granted_at"`
	WithdrawnAt      *time.Time `json:"withdrawn_at"`
	Source           string     `json:"source"`
	WithdrawalSource *string    `json:"withdrawal_source"`
	TextVersion      string     `json:"text_version"`
	Note             string     `json:"note"`
	RecordedBy       *Actor     `json:"recorded_by"`
	WithdrawnBy      *Actor     `json:"withdrawn_by"`
}

// ConsentState is the current state of one channel.
type ConsentState struct {
	Channel     string     `json:"channel"`
	Granted     bool       `json:"granted"`
	Since       *time.Time `json:"since"`
	TextVersion string     `json:"text_version"`
}

// ConsentsView is the answer of GET /{uuid}/consents.
type ConsentsView struct {
	Current []ConsentState `json:"current"`
	Items   []Consent      `json:"items"`
}

// ConsentInput is the body of POST /{uuid}/consents: action grant opens a
// consent of the channel, withdraw closes the open one.
type ConsentInput struct {
	Channel     string  `json:"channel"`
	Action      string  `json:"action"`
	Source      string  `json:"source"`
	TextVersion string  `json:"text_version"`
	Note        string  `json:"note"`
	At          *string `json:"at"`
}

// Driver is the API projection of a driver (sensitive values masked).
type Driver struct {
	UUID           uuid.UUID `json:"uuid"`
	CustomerUUID   uuid.UUID `json:"customer_uuid"`
	IsCustomerSelf bool      `json:"is_customer_self"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	FullName       string    `json:"full_name"`
	BirthDate      *string   `json:"birth_date"`
	// Age is in full years today (Europe/Istanbul).
	Age              *int      `json:"age"`
	PhoneMasked      string    `json:"phone_masked"`
	EmailMasked      string    `json:"email_masked"`
	IdentityType     *string   `json:"identity_type"`
	IdentityMasked   string    `json:"identity_masked"`
	Nationality      *string   `json:"nationality"`
	LicenceMasked    string    `json:"licence_masked"`
	LicenceClass     string    `json:"licence_class"`
	LicenceIssuedOn  *string   `json:"licence_issued_on"`
	LicenceExpiresOn *string   `json:"licence_expires_on"`
	LicenceYears     *int      `json:"licence_years"`
	LicenceCountry   *string   `json:"licence_country"`
	ForeignLicence   bool      `json:"foreign_licence"`
	LicenceExpired   bool      `json:"licence_expired"`
	Note             string    `json:"note"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// DriverInput is the create / patch body of a driver. CopyFromCustomer
// (create only) fills the person from an individual customer ("customer is
// also driver"); given fields still win.
type DriverInput struct {
	CopyFromCustomer bool           `json:"copy_from_customer"`
	FirstName        *string        `json:"first_name"`
	LastName         *string        `json:"last_name"`
	BirthDate        OptionalString `json:"birth_date"`
	Phone            *string        `json:"phone"`
	Email            *string        `json:"email"`
	IdentityType     OptionalString `json:"identity_type"`
	IdentityNumber   *string        `json:"identity_number"`
	Nationality      OptionalString `json:"nationality"`
	LicenceNumber    *string        `json:"licence_number"`
	LicenceClass     *string        `json:"licence_class"`
	LicenceIssuedOn  OptionalString `json:"licence_issued_on"`
	LicenceExpiresOn OptionalString `json:"licence_expires_on"`
	LicenceCountry   OptionalString `json:"licence_country"`
	Note             *string        `json:"note"`
}

// Document is a customer document (identity / licence scan, …).
type Document struct {
	UUID        uuid.UUID  `json:"uuid"`
	DocType     string     `json:"doc_type"`
	Driver      *DriverRef `json:"driver"`
	ValidUntil  *string    `json:"valid_until"`
	HasFile     bool       `json:"has_file"`
	FileName    string     `json:"file_name"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	Note        string     `json:"note"`
	CreatedAt   time.Time  `json:"created_at"`
}

// DriverRef is a driver embedded in a document.
type DriverRef struct {
	UUID     uuid.UUID `json:"uuid"`
	FullName string    `json:"full_name"`
}

// DocumentInput is the create body of a customer document.
type DocumentInput struct {
	DocType    *string        `json:"doc_type"`
	DriverUUID *uuid.UUID     `json:"driver_uuid"`
	ValidUntil OptionalString `json:"valid_until"`
	Note       *string        `json:"note"`
}

// ListFilter selects customers of one organization.
type ListFilter struct {
	Limit, Offset int32
	Q             string
	Types         []string
	Nationalities []string
	Tags          []string
	Consents      []string
	Anonymized    *bool
	SortField     string
	SortDesc      bool
}

// QueryKeys are the list filters (HTTP list, export and bulk "all matching").
var QueryKeys = []string{"customer_type", "nationality", "tag", "consent", "anonymized"}

// ExportOnlyKeys are export query keys that are not list filters:
// include_sensitive ("true") puts plaintext identity / contact values into
// the file (tenant.customers.reveal and an activity event required).
var ExportOnlyKeys = []string{"include_sensitive"}

// FilterFromQuery maps list query params to a filter; multi-value params
// are comma-separated.
func FilterFromQuery(query map[string]string) ListFilter {
	f := ListFilter{
		Q:             strings.TrimSpace(query["q"]),
		Types:         SplitList(query["customer_type"]),
		Nationalities: upper(SplitList(query["nationality"])),
		Tags:          SplitList(query["tag"]),
		Consents:      SplitList(query["consent"]),
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["anonymized"])); err == nil {
		f.Anonymized = &v
	}
	return f
}

// SplitList parses a comma-separated faceted filter value.
func SplitList(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func upper(list []string) []string {
	for i := range list {
		list[i] = strings.ToUpper(list[i])
	}
	return list
}

// Facet is one value of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the customers list.
type Facets struct {
	CustomerType []Facet `json:"customer_type"`
	Nationality  []Facet `json:"nationality"`
	Tag          []Facet `json:"tag"`
	Consent      []Facet `json:"consent"`
}

// ActivityEntry is one audit event of a customer.
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
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
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

// String returns an OptionalString set to v.
func String(v string) OptionalString { return OptionalString{Set: true, Value: &v} }

// Str returns a pointer to v.
func Str(v string) *string { return &v }

// Contains reports whether list holds v.
func Contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
