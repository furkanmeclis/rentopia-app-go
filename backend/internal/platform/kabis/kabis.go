// Package kabis is the provider layer of KABİS, the police vehicle rental
// notification system (TEC-726, docs/design/kabis.md).
//
// The official connection method is NOT verified yet (TEC-682): no
// undocumented endpoint or automation is assumed. The official adapter
// slot (Official, provider "kabis") can be connected with the
// organization's credentials but reports ErrDependencyPending ("waiting on
// dependency") until the method is verified; jobs for it stay visible in
// that state and are never sent. The development / test adapter
// (package kabisfake, provider "kabis_fake") is registered outside
// production only.
//
// A Provider reports one notification (Report) and looks one up by our own
// reference (Lookup), which is what the integration runner's status check
// needs before anything is sent again.
package kabis

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

// Kind is the integration kind of KABİS accounts.
const Kind = "kabis"

// ProviderOfficial is the provider code of the official adapter slot.
const ProviderOfficial = "kabis"

// Events a notification reports.
const (
	EventRentalStart = "rental_start"
	EventRentalEnd   = "rental_end"
	EventCorrection  = "correction"
)

// Notification is the full payload of one notification. It is built only
// from actual events (handover, return) and the person / vehicle records;
// it carries identity and licence numbers, so it is sealed at rest and
// never logged or returned to a browser.
type Notification struct {
	// Reference is our own reference of the notification (its uuid): a
	// provider lookup by it tells whether the authority has the record.
	Reference string `json:"reference"`
	Event     string `json:"event"`
	// Corrects is the reference of the notification a correction amends.
	Corrects        string    `json:"corrects,omitempty"`
	RentalReference string    `json:"rental_reference"`
	OccurredAt      time.Time `json:"occurred_at"`
	Place           Place     `json:"place"`
	Vehicle         Vehicle   `json:"vehicle"`
	Renter          Person    `json:"renter"`
	Drivers         []Person  `json:"drivers"`
}

// Place is where the event happened (the branch, or its service point).
type Place struct {
	BranchCode string `json:"branch_code"`
	Name       string `json:"name"`
	City       string `json:"city"`
	District   string `json:"district"`
	Address    string `json:"address"`
	Point      string `json:"point,omitempty"`
}

// Vehicle is the rented vehicle.
type Vehicle struct {
	Plate     string `json:"plate"`
	BrandName string `json:"brand_model,omitempty"`
}

// Person is the renter or a driver.
type Person struct {
	// Role: renter, primary, additional.
	Role string `json:"role"`
	// Type: individual or corporate (renter only).
	Type           string `json:"type,omitempty"`
	FirstName      string `json:"first_name,omitempty"`
	LastName       string `json:"last_name,omitempty"`
	CompanyTitle   string `json:"company_title,omitempty"`
	IdentityType   string `json:"identity_type,omitempty"`
	IdentityNumber string `json:"identity_number,omitempty"`
	Nationality    string `json:"nationality,omitempty"`
	BirthDate      string `json:"birth_date,omitempty"`
	LicenceNumber  string `json:"licence_number,omitempty"`
	LicenceClass   string `json:"licence_class,omitempty"`
	LicenceCountry string `json:"licence_country,omitempty"`
}

// Receipt is the authority's answer to a report.
type Receipt struct {
	// Reference is the authority's record number.
	Reference string
}

// LookupResult says whether the authority has the notification.
type LookupResult struct {
	Found     bool
	Reference string
}

// Provider is one KABİS adapter.
type Provider interface {
	integrations.Adapter
	// Ready reports whether the adapter can send at all: nil, or
	// ErrDependencyPending (the official method is not verified).
	Ready() error
	// Report files one notification. ErrUnavailable: the request did not
	// leave (connection refused, DNS): nothing was filed. ErrRejected: the
	// authority refused it (definitive). Any other error: the outcome is
	// unknown (a timeout after sending).
	Report(ctx context.Context, account integrations.Account, n Notification) (Receipt, error)
	// Lookup asks whether the authority has the notification (by
	// n.Reference / the provider reference). An error means "unknown";
	// integrations.ErrUnsupported when the provider has no lookup.
	Lookup(ctx context.Context, account integrations.Account, reference, providerRef string) (LookupResult, error)
}

var (
	// ErrDependencyPending: the official connection method is not verified
	// (TEC-682); nothing can be sent.
	ErrDependencyPending = errors.New("kabis: waiting on dependency: the official connection method is not verified")
	// ErrUnavailable: the request did not reach the authority; nothing was
	// filed, sending again is safe.
	ErrUnavailable = errors.New("kabis: service unavailable (nothing was sent)")
	// ErrRejected: the authority refused the notification.
	ErrRejected = errors.New("kabis: the notification was rejected")
	// ErrTimeout: no answer after the request left; the outcome is unknown.
	ErrTimeout = errors.New("kabis: no answer after sending (outcome unknown)")
)

// Official is the official adapter slot. Until the connection method is
// verified (TEC-682) it stores the organization's credentials but cannot
// send: Check and Ready answer ErrDependencyPending, Report and Lookup
// ErrUnsupported (never called by the module).
type Official struct{}

// Kind implements integrations.Adapter.
func (Official) Kind() string { return Kind }

// Provider implements integrations.Adapter.
func (Official) Provider() string { return ProviderOfficial }

// Fields implements integrations.Adapter: the user code and password the
// organization receives from the authority (to be confirmed in TEC-682).
func (Official) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "username", LabelKey: "payments.integrations.fields.username", Secret: false, Required: true},
		{Key: "password", LabelKey: "payments.integrations.fields.password", Secret: true, Required: true},
	}
}

// Check implements integrations.Adapter.
func (Official) Check(context.Context, integrations.Account) error { return ErrDependencyPending }

// Ready implements Provider.
func (Official) Ready() error { return ErrDependencyPending }

// Report implements Provider (never called: Ready fails).
func (Official) Report(context.Context, integrations.Account, Notification) (Receipt, error) {
	return Receipt{}, integrations.ErrUnsupported
}

// Lookup implements Provider (never called: Ready fails).
func (Official) Lookup(context.Context, integrations.Account, string, string) (LookupResult, error) {
	return LookupResult{}, integrations.ErrUnsupported
}
