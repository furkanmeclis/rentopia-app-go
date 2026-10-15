// Package model holds the toll pass types (TEC-725, docs/design/tolls.md):
// statuses, review reasons, API views, inputs and the ledger port.
package model

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Resource is the engine / live / activity slug of toll passes.
const Resource = "tenant.toll_passes"

// ResourceVehicles is the activity resource of tag changes (vehicle data).
const ResourceVehicles = "tenant.vehicles"

// ResourceRentals is the live resource notified when toll charges change.
const ResourceRentals = "tenant.rentals"

// JobKind is the integration job kind of a provider fetch.
const JobKind = "toll.fetch"

// Pass statuses.
const (
	StatusUnmatched   = "unmatched"
	StatusMatched     = "matched"
	StatusNeedsReview = "needs_review"
	StatusIgnored     = "ignored"
	StatusCharged     = "charged"
)

// Statuses lists the pass statuses.
var Statuses = []string{StatusUnmatched, StatusNeedsReview, StatusMatched, StatusCharged, StatusIgnored}

// ReviewStatuses are the statuses of the review queue.
var ReviewStatuses = []string{StatusUnmatched, StatusNeedsReview}

// Review reasons: why a pass waits for a person. Missing data is a reason,
// never "no debt".
const (
	// ReasonVehicleUnknown: neither the plate nor the tag names a vehicle.
	ReasonVehicleUnknown = "vehicle_unknown"
	// ReasonVehicleConflict: the plate and the tag name different vehicles.
	ReasonVehicleConflict = "vehicle_conflict"
	// ReasonNoRental: the vehicle was on no rental at the pass time.
	ReasonNoRental = "no_rental"
	// ReasonOverlap: more than one rental leg contains the pass time.
	ReasonOverlap = "overlap"
	// ReasonMissingActualTimes: a rental is planned around the pass time
	// but its handover is not recorded.
	ReasonMissingActualTimes = "missing_actual_times"
	// ReasonOutsideActualWindow: the pass is inside a rental's plan but
	// outside its recorded handover → return.
	ReasonOutsideActualWindow = "outside_actual_window"
	// ReasonInTransfer: the vehicle was in a transfer at the pass time.
	ReasonInTransfer = "in_transfer"
	// ReasonRentalCancelled: the assigned rental was cancelled.
	ReasonRentalCancelled = "rental_cancelled"
	// ReasonUnassigned: a person removed the assignment.
	ReasonUnassigned = "unassigned"
)

// ReviewReasons lists the reasons (facets, i18n).
var ReviewReasons = []string{
	ReasonVehicleUnknown, ReasonVehicleConflict, ReasonNoRental, ReasonOverlap, ReasonMissingActualTimes,
	ReasonOutsideActualWindow, ReasonInTransfer, ReasonRentalCancelled, ReasonUnassigned,
}

// Sources.
const (
	SourceAPI  = "api"
	SourceFile = "file"
)

// Match methods.
const (
	MethodPlate  = "plate"
	MethodTag    = "tag"
	MethodManual = "manual"
)

// Pass timeline actions (toll_pass_events.action).
const (
	EventReceived       = "received"
	EventRevised        = "revised"
	EventMatched        = "matched"
	EventReview         = "review"
	EventAssigned       = "assigned"
	EventUnassigned     = "unassigned"
	EventIgnored        = "ignored"
	EventRematched      = "rematched"
	EventChargeCreated  = "charge_created"
	EventChargeReversed = "charge_reversed"
)

// Rental timeline actions written on the rental of a toll charge.
const (
	RentalEventTollCharged  = "toll_charged"
	RentalEventTollReversed = "toll_reversed"
)

// Charge kinds (rental_charges.kind).
const (
	ChargeToll    = "toll"
	ChargeTollFee = "toll_fee"
)

// Ref is a small reference.
type Ref struct {
	UUID  uuid.UUID `json:"uuid"`
	Plate string    `json:"plate,omitempty"`
}

// RentalRef is the rental of a pass.
type RentalRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	Status    string    `json:"status"`
}

// Actor is a user reference.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Pass is a toll pass in the list.
type Pass struct {
	UUID          uuid.UUID    `json:"uuid"`
	Provider      string       `json:"provider"`
	ExternalID    string       `json:"external_id"`
	Source        string       `json:"source"`
	PassedAt      time.Time    `json:"passed_at"`
	Plate         string       `json:"plate"`
	TagNumber     string       `json:"tag_number"`
	Station       string       `json:"station"`
	ExitStation   string       `json:"exit_station"`
	Currency      string       `json:"currency"`
	Amount        money.Money  `json:"amount"`
	Status        string       `json:"status"`
	ReviewReason  string       `json:"review_reason"`
	MatchMethod   string       `json:"match_method"`
	Manual        bool         `json:"manual"`
	Revision      int32        `json:"revision"`
	Vehicle       *Ref         `json:"vehicle"`
	Rental        *RentalRef   `json:"rental"`
	FeeAmount     *money.Money `json:"fee_amount"`
	LastFetchedAt time.Time    `json:"last_fetched_at"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// Revision is a changed fact of a pass that arrived again.
type Revision struct {
	UUID          uuid.UUID       `json:"uuid"`
	Revision      int32           `json:"revision"`
	Source        string          `json:"source"`
	ChangedFields []string        `json:"changed_fields"`
	Previous      json.RawMessage `json:"previous"`
	Current       json.RawMessage `json:"current"`
	AmountBefore  money.Money     `json:"amount_before"`
	AmountAfter   money.Money     `json:"amount_after"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Event is one entry of the pass timeline.
type Event struct {
	UUID       uuid.UUID       `json:"uuid"`
	Action     string          `json:"action"`
	FromStatus string          `json:"from_status"`
	ToStatus   string          `json:"to_status"`
	Reason     string          `json:"reason"`
	Data       json.RawMessage `json:"data"`
	Actor      *Actor          `json:"actor"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Charge is a rental charge of the pass (live, reversed or a reversal).
type Charge struct {
	UUID           uuid.UUID   `json:"uuid"`
	Kind           string      `json:"kind"`
	Status         string      `json:"status"`
	Currency       string      `json:"currency"`
	Amount         money.Money `json:"amount"`
	Reason         string      `json:"reason"`
	ReversalOfUUID *uuid.UUID  `json:"reversal_of_uuid"`
	Rental         RentalRef   `json:"rental"`
	CreatedAt      time.Time   `json:"created_at"`
}

// AccountRef is the provider account a pass came from.
type AccountRef struct {
	UUID        uuid.UUID `json:"uuid"`
	DisplayName string    `json:"display_name"`
}

// PassDetail is GET /v1/tenant/toll-passes/{uuid}.
type PassDetail struct {
	Pass
	FirstFetchedAt time.Time       `json:"first_fetched_at"`
	Raw            json.RawMessage `json:"raw"`
	Account        *AccountRef     `json:"account"`
	DecidedAt      *time.Time      `json:"decided_at"`
	DecidedBy      *Actor          `json:"decided_by"`
	Revisions      []Revision      `json:"revisions"`
	Events         []Event         `json:"events"`
	Charges        []Charge        `json:"charges"`
}

// Candidate is a rental of the pass's vehicle around the pass time.
type Candidate struct {
	UUID            uuid.UUID  `json:"uuid"`
	Reference       string     `json:"reference"`
	Status          string     `json:"status"`
	Customer        string     `json:"customer"`
	PlannedPickupAt time.Time  `json:"planned_pickup_at"`
	PlannedReturnAt time.Time  `json:"planned_return_at"`
	ActualPickupAt  *time.Time `json:"actual_pickup_at"`
	ActualReturnAt  *time.Time `json:"actual_return_at"`
	// ContainsPass: the actual handover → return interval contains the
	// pass time.
	ContainsPass bool `json:"contains_pass"`
}

// Settings is the organization's toll settings.
type Settings struct {
	ProcessingFee money.Money `json:"processing_fee"`
	Currency      string      `json:"currency"`
	AutoFetch     bool        `json:"auto_fetch"`
	LookbackDays  int32       `json:"lookback_days"`
	// ProviderConnected: an active toll provider account exists.
	ProviderConnected bool       `json:"provider_connected"`
	UpdatedAt         *time.Time `json:"updated_at"`
}

// SettingsInput is PATCH /v1/tenant/toll-settings.
type SettingsInput struct {
	ProcessingFee *money.Money `json:"processing_fee"`
	AutoFetch     *bool        `json:"auto_fetch"`
	LookbackDays  *int32       `json:"lookback_days"`
}

// Tag is a vehicle ↔ tag link.
type Tag struct {
	UUID      uuid.UUID  `json:"uuid"`
	Provider  string     `json:"provider"`
	Network   string     `json:"network"`
	TagNumber string     `json:"tag_number"`
	ValidFrom time.Time  `json:"valid_from"`
	ValidTo   *time.Time `json:"valid_to"`
	Active    bool       `json:"active"`
	Note      string     `json:"note"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TagInput creates or changes a tag link.
type TagInput struct {
	Provider  *string    `json:"provider"`
	Network   *string    `json:"network"`
	TagNumber *string    `json:"tag_number"`
	ValidFrom *time.Time `json:"valid_from"`
	ValidTo   *time.Time `json:"valid_to"`
	// ClearValidTo puts an ended tag back on the vehicle (PATCH).
	ClearValidTo bool    `json:"clear_valid_to"`
	Note         *string `json:"note"`
}

// Networks are the tag networks.
var Networks = []string{"hgs", "ogs"}

// PassInput is one pass to ingest (provider fetch or file row).
type PassInput struct {
	Provider    string
	ExternalID  string
	Source      string
	AccountID   int64
	PassedAt    time.Time
	Plate       string
	TagNumber   string
	Station     string
	ExitStation string
	Amount      money.Money
	Currency    string
	Raw         map[string]any
}

// Ingest outcomes.
const (
	OpCreated   = "created"
	OpRevised   = "revised"
	OpUnchanged = "unchanged"
)

// IngestResult is what Ingest did with a pass.
type IngestResult struct {
	UUID          uuid.UUID
	Op            string
	Status        string
	ChangedFields []string
	// Previous are the facts before a revision (import rollback).
	Previous map[string]any
}

// ReasonInput carries the mandatory reason of a manual decision.
type ReasonInput struct {
	Reason string `json:"reason"`
}

// AssignInput is POST /v1/tenant/toll-passes/{uuid}/assign.
type AssignInput struct {
	RentalUUID *uuid.UUID `json:"rental_uuid"`
	Reason     string     `json:"reason"`
}

// FetchResult is POST /v1/tenant/toll-passes/fetch.
type FetchResult struct {
	JobUUID uuid.UUID `json:"job_uuid"`
	Status  string    `json:"status"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
}

// Facet is one value count.
type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list.
type Facets struct {
	Status       []Facet `json:"status"`
	Source       []Facet `json:"source"`
	Provider     []Facet `json:"provider"`
	ReviewReason []Facet `json:"review_reason"`
	// Review is the review queue count (unmatched + needs_review).
	Review int64 `json:"review"`
}

// ListFilter selects a page of passes.
type ListFilter struct {
	Q             string
	Statuses      []string
	Sources       []string
	Providers     []string
	ReviewReasons []string
	VehicleUUIDs  []uuid.UUID
	RentalUUIDs   []uuid.UUID
	HasRental     *bool
	From, To      *time.Time
	SortField     string
	SortDesc      bool
	Limit, Offset int32
	// BranchIDs is set by the use case from the caller's scope.
	BranchIDs []int64
}

// QueryKeys are the list filters (also honoured by exports).
var QueryKeys = []string{"status", "source", "provider", "review_reason", "vehicle", "rental", "has_rental", "review", "from", "to"}

// Sortable maps sort fields to themselves (apiquery whitelist).
var Sortable = map[string]string{
	"passed_at": "passed_at", "amount": "amount", "status": "status", "plate": "plate", "created_at": "created_at",
}

// FilterFromQuery builds the list filter from query values (comma lists).
// Malformed uuids / dates answer an error naming the parameter.
func FilterFromQuery(v map[string]string) (ListFilter, error) {
	f := ListFilter{Q: strings.TrimSpace(v["q"])}
	f.Statuses = SplitList(v["status"])
	if b := strings.TrimSpace(v["review"]); b == "1" || b == "true" {
		f.Statuses = append([]string(nil), ReviewStatuses...)
	}
	f.Sources = SplitList(v["source"])
	f.Providers = SplitList(v["provider"])
	f.ReviewReasons = SplitList(v["review_reason"])
	for _, key := range []string{"vehicle", "rental"} {
		for _, s := range SplitList(v[key]) {
			id, err := uuid.Parse(s)
			if err != nil {
				return f, &QueryError{Field: key}
			}
			if key == "vehicle" {
				f.VehicleUUIDs = append(f.VehicleUUIDs, id)
			} else {
				f.RentalUUIDs = append(f.RentalUUIDs, id)
			}
		}
	}
	switch strings.TrimSpace(v["has_rental"]) {
	case "true", "1":
		t := true
		f.HasRental = &t
	case "false", "0":
		b := false
		f.HasRental = &b
	}
	for _, key := range []string{"from", "to"} {
		s := strings.TrimSpace(v[key])
		if s == "" {
			continue
		}
		t, err := ParseDateBound(s, key == "to")
		if err != nil {
			return f, &QueryError{Field: key}
		}
		if key == "from" {
			f.From = &t
		} else {
			f.To = &t
		}
	}
	return f, nil
}

// QueryError is a malformed list parameter (400).
type QueryError struct{ Field string }

func (e *QueryError) Error() string { return e.Field + " is invalid" }

// Istanbul is the organizations' business time zone (dates without a zone).
var Istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*3600)
	}
	return loc
}()

// ParseDateBound parses a date (YYYY-MM-DD, Istanbul day; the end bound is
// exclusive, so "to" moves to the next day) or an RFC 3339 time.
func ParseDateBound(s string, end bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	d, err := time.ParseInLocation("2006-01-02", s, Istanbul)
	if err != nil {
		return time.Time{}, err
	}
	if end {
		d = d.AddDate(0, 0, 1)
	}
	return d.UTC(), nil
}

// SplitList splits a comma list, trimming blanks.
func SplitList(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Contains reports whether set has v.
func Contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// LedgerEntry is a toll charge (or its reversal) as the finance ledger
// (TEC-717) will post it to the customer's account.
type LedgerEntry struct {
	OrganizationID int64
	RentalID       int64
	RentalUUID     uuid.UUID
	CustomerID     int64
	ChargeID       int64
	ChargeUUID     uuid.UUID
	// Kind is toll or toll_fee.
	Kind     string
	Amount   money.Money
	Currency string
	// ReversalOf is the charge this entry reverses (negative amount).
	ReversalOf   *uuid.UUID
	TollPassUUID uuid.UUID
	PassedAt     time.Time
}

// LedgerPoster is the plug for the finance ledger (TEC-717, built in
// parallel): every toll charge and reversal the tolls module writes is
// handed to it inside the same database transaction, so a ledger posting
// commits or rolls back with the charge. Until the ledger exists the
// module uses NoLedger and rental_charges is the only record.
type LedgerPoster interface {
	PostTollCharge(ctx context.Context, tx pgx.Tx, entry LedgerEntry) error
}

// NoLedger posts nothing.
type NoLedger struct{}

// PostTollCharge implements LedgerPoster.
func (NoLedger) PostTollCharge(context.Context, pgx.Tx, LedgerEntry) error { return nil }
