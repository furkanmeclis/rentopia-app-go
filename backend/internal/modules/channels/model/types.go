// Package model holds the channel manager types (TEC-736, RT-A25,
// docs/design/channel-manager.md).
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Live list / activity resources.
const (
	ResourceAccounts = "tenant.channel_accounts"
	ResourceMappings = "tenant.channel_mappings"
	ResourceMessages = "tenant.channel_messages"
)

// Mapping entity types.
const (
	EntityBranch  = "branch"
	EntityProduct = "product"
	EntityExtra   = "extra"
)

// EntityTypes lists the mapping entity types.
var EntityTypes = []string{EntityBranch, EntityProduct, EntityExtra}

// Message statuses.
const (
	MessageReceived  = "received"
	MessageApplied   = "applied"
	MessageDuplicate = "duplicate"
	MessageStale     = "stale"
	MessageFailed    = "failed"
	MessageIgnored   = "ignored"
)

// MessageStatuses lists the message statuses.
var MessageStatuses = []string{MessageReceived, MessageApplied, MessageDuplicate, MessageStale, MessageFailed, MessageIgnored}

// MessageTypes lists the message types.
var MessageTypes = []string{channels.MessageBooking, channels.MessageModification, channels.MessageCancellation}

// Message sources.
const (
	SourcePull       = "pull"
	SourceWebhook    = "webhook"
	SourceSimulation = "simulation"
)

// Channel booking statuses (channel_reservations.status).
const (
	BookingPending   = "pending"
	BookingActive    = "active"
	BookingCancelled = "cancelled"
	BookingFailed    = "failed"
)

// Sync directions, triggers and statuses.
const (
	DirectionPush      = "push"
	DirectionPull      = "pull"
	DirectionReconcile = "reconcile"

	TriggerEvent    = "event"
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"

	RunRunning   = "running"
	RunSucceeded = "succeeded"
	RunPartial   = "partial"
	RunFailed    = "failed"
)

// SyncDirections, SyncStatuses list the sync log values.
var (
	SyncDirections = []string{DirectionPush, DirectionPull, DirectionReconcile}
	SyncStatuses   = []string{RunRunning, RunSucceeded, RunPartial, RunFailed}
)

// Error codes of a message that could not be applied (error queue).
const (
	ErrCodeInvalidMessage  = "invalid_message"
	ErrCodeUnknownLocation = "unknown_location"
	ErrCodeUnknownProduct  = "unknown_product"
	ErrCodeAmbiguous       = "ambiguous_product"
	ErrCodeUnknownExtra    = "unknown_extra"
	ErrCodeCapacity        = "capacity_unavailable"
	ErrCodePricing         = "pricing_unavailable"
	ErrCodeReinstate       = "reinstate_not_supported"
	ErrCodeCancelRefused   = "cancel_refused"
	ErrCodeChangeRefused   = "change_refused"
	ErrCodeBookingRefused  = "booking_refused"
	ErrCodeCustomer        = "customer_failed"
	ErrCodeInternal        = "internal_error"
)

// Health of an account's sync.
const (
	HealthOK       = "ok"
	HealthPending  = "pending"
	HealthFailing  = "failing"
	HealthExpired  = "expired"
	HealthDisabled = "disabled"
)

// Reconciliation results of one booking.
const (
	MatchOK            = "matched"
	MatchMissingLocal  = "missing_local"
	MatchMissingRemote = "missing_channel"
	MatchStatus        = "status_mismatch"
	MatchAmount        = "amount_mismatch"
	MatchCommission    = "commission_mismatch"
)

// MatchStatuses lists the reconciliation results.
var MatchStatuses = []string{MatchOK, MatchMissingLocal, MatchMissingRemote, MatchStatus, MatchAmount, MatchCommission}

// Contains reports whether list holds v.
func Contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Ref is a small reference.
type Ref struct {
	UUID      uuid.UUID `json:"uuid"`
	Code      string    `json:"code,omitempty"`
	Name      string    `json:"name,omitempty"`
	Reference string    `json:"reference,omitempty"`
	Status    string    `json:"status,omitempty"`
	Provider  string    `json:"provider,omitempty"`
}

// AccountSettings are the sync settings of a channel account.
type AccountSettings struct {
	PushEnabled         bool   `json:"push_enabled"`
	PullEnabled         bool   `json:"pull_enabled"`
	StopSale            bool   `json:"stop_sale"`
	HorizonDays         int32  `json:"horizon_days"`
	PushIntervalMinutes int32  `json:"push_interval_minutes"`
	PullIntervalMinutes int32  `json:"pull_interval_minutes"`
	ValidityMinutes     int32  `json:"validity_minutes"`
	OfflinePolicy       string `json:"offline_policy"`
	OfflineQuota        int32  `json:"offline_quota"`
	CollectedBy         string `json:"collected_by"`
	CommissionRate      string `json:"commission_rate"`
}

// SyncState is the sync state of a channel account.
type SyncState struct {
	Health              string     `json:"health"`
	PushRequestedAt     *time.Time `json:"push_requested_at"`
	LastPushAt          *time.Time `json:"last_push_at"`
	LastPushOkAt        *time.Time `json:"last_push_ok_at"`
	LastPushStatus      string     `json:"last_push_status"`
	LastPushError       string     `json:"last_push_error"`
	PushedValidUntil    *time.Time `json:"pushed_valid_until"`
	LastPullAt          *time.Time `json:"last_pull_at"`
	LastPullOkAt        *time.Time `json:"last_pull_ok_at"`
	LastPullStatus      string     `json:"last_pull_status"`
	LastPullError       string     `json:"last_pull_error"`
	ConsecutiveFailures int32      `json:"consecutive_failures"`
}

// Account is a channel account with its settings and sync state.
type Account struct {
	UUID           uuid.UUID             `json:"uuid"`
	Provider       string                `json:"provider"`
	DisplayName    string                `json:"display_name"`
	Mode           string                `json:"mode"`
	Status         string                `json:"status"`
	Channel        string                `json:"channel"`
	Available      bool                  `json:"available"`
	Simulator      bool                  `json:"simulator"`
	Capabilities   channels.Capabilities `json:"capabilities"`
	Settings       AccountSettings       `json:"settings"`
	Sync           SyncState             `json:"sync"`
	OpenErrors     int64                 `json:"open_errors"`
	ActiveMappings int64                 `json:"active_mappings"`
}

// SettingsInput is PATCH /v1/tenant/channel-accounts/{uuid} (nil = keep).
type SettingsInput struct {
	PushEnabled         *bool   `json:"push_enabled"`
	PullEnabled         *bool   `json:"pull_enabled"`
	StopSale            *bool   `json:"stop_sale"`
	HorizonDays         *int32  `json:"horizon_days"`
	PushIntervalMinutes *int32  `json:"push_interval_minutes"`
	PullIntervalMinutes *int32  `json:"pull_interval_minutes"`
	ValidityMinutes     *int32  `json:"validity_minutes"`
	OfflinePolicy       *string `json:"offline_policy"`
	OfflineQuota        *int32  `json:"offline_quota"`
	CollectedBy         *string `json:"collected_by"`
	CommissionRate      *string `json:"commission_rate"`
}

// Mapping is one internal record ↔ external code of an account.
type Mapping struct {
	UUID         uuid.UUID `json:"uuid"`
	Account      Ref       `json:"account"`
	EntityType   string    `json:"entity_type"`
	Branch       *Ref      `json:"branch"`
	Product      *Ref      `json:"product"`
	Extra        *Ref      `json:"extra"`
	ExternalCode string    `json:"external_code"`
	ExternalName string    `json:"external_name"`
	Priority     int32     `json:"priority"`
	StopSale     bool      `json:"stop_sale"`
	MaxQuota     *int32    `json:"max_quota"`
	IsActive     bool      `json:"is_active"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OptionalInt distinguishes "absent" from an explicit null in a PATCH.
type OptionalInt struct {
	Set   bool
	Value *int32
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *OptionalInt) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v int32
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalUUID distinguishes "absent" from an explicit null in a PATCH.
type OptionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *OptionalUUID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
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

// MappingInput is the body of POST / PATCH /v1/tenant/channel-mappings.
// The account, entity type, product and extra are fixed after creation;
// a product mapping's location scope (branch_uuid) may change.
type MappingInput struct {
	AccountUUID  *uuid.UUID   `json:"account_uuid"`
	EntityType   string       `json:"entity_type"`
	BranchUUID   OptionalUUID `json:"branch_uuid"`
	ProductUUID  *uuid.UUID   `json:"product_uuid"`
	ExtraUUID    *uuid.UUID   `json:"extra_uuid"`
	ExternalCode *string      `json:"external_code"`
	ExternalName *string      `json:"external_name"`
	Priority     *int32       `json:"priority"`
	StopSale     *bool        `json:"stop_sale"`
	MaxQuota     OptionalInt  `json:"max_quota"`
	IsActive     *bool        `json:"is_active"`
	Note         *string      `json:"note"`
}

// MappingFilter selects a page of mappings.
type MappingFilter struct {
	Q             string
	AccountUUID   *uuid.UUID
	EntityTypes   []string
	Active        *bool
	StopSale      *bool
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// MappingFacets are the counts per entity type.
type MappingFacets struct {
	EntityTypes map[string]int64 `json:"entity_types"`
	StopSale    int64            `json:"stop_sale"`
	Inactive    int64            `json:"inactive"`
}

// Message is one inbound channel message.
type Message struct {
	UUID           uuid.UUID      `json:"uuid"`
	Account        Ref            `json:"account"`
	Channel        string         `json:"channel"`
	MessageKey     string         `json:"message_key"`
	ExternalRef    string         `json:"external_ref"`
	Type           string         `json:"type"`
	Version        int64          `json:"version"`
	OccurredAt     *time.Time     `json:"occurred_at"`
	Source         string         `json:"source"`
	Status         string         `json:"status"`
	ErrorCode      string         `json:"error_code"`
	ErrorMessage   string         `json:"error_message"`
	Attempts       int32          `json:"attempts"`
	Deliveries     int32          `json:"deliveries"`
	ProcessedAt    *time.Time     `json:"processed_at"`
	AckedAt        *time.Time     `json:"acked_at"`
	AckError       string         `json:"ack_error"`
	Summary        map[string]any `json:"summary"`
	Reservation    *Ref           `json:"reservation"`
	ResolutionNote string         `json:"resolution_note"`
	ResolvedAt     *time.Time     `json:"resolved_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Booking is the channel booking a message belongs to.
type Booking struct {
	UUID            uuid.UUID    `json:"uuid"`
	Status          string       `json:"status"`
	AppliedVersion  int64        `json:"applied_version"`
	LastVersion     int64        `json:"last_version"`
	CollectedBy     string       `json:"collected_by"`
	Currency        string       `json:"currency"`
	Gross           money.Money  `json:"gross"`
	Commission      money.Money  `json:"commission"`
	Net             money.Money  `json:"net"`
	PriceDifference *money.Money `json:"price_difference"`
}

// MessageActions say what the caller may do with a message.
type MessageActions struct {
	Retry  bool `json:"retry"`
	Ignore bool `json:"ignore"`
}

// MessageDetail is a message with its booking and actions.
type MessageDetail struct {
	Message
	Booking    *Booking       `json:"booking"`
	ResolvedBy string         `json:"resolved_by"`
	Actions    MessageActions `json:"actions"`
}

// MessageFilter selects a page of messages.
type MessageFilter struct {
	Q             string
	AccountUUID   *uuid.UUID
	Statuses      []string
	Types         []string
	ExternalRef   string
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// MessageFacets are the counts per status and type; NeedsAction = failed.
type MessageFacets struct {
	Statuses    map[string]int64 `json:"statuses"`
	Types       map[string]int64 `json:"types"`
	NeedsAction int64            `json:"needs_action"`
}

// SyncRun is one row of the sync log.
type SyncRun struct {
	UUID        uuid.UUID      `json:"uuid"`
	Account     Ref            `json:"account"`
	Direction   string         `json:"direction"`
	Trigger     string         `json:"trigger"`
	Status      string         `json:"status"`
	ItemsTotal  int32          `json:"items_total"`
	ItemsOK     int32          `json:"items_ok"`
	ItemsFailed int32          `json:"items_failed"`
	Error       string         `json:"error"`
	Details     map[string]any `json:"details"`
	Actor       string         `json:"actor"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  *time.Time     `json:"finished_at"`
}

// SyncRunFilter selects a page of the sync log.
type SyncRunFilter struct {
	AccountUUID   *uuid.UUID
	Directions    []string
	Statuses      []string
	SortDesc      bool
	Limit, Offset int32
}

// SimulateInput queues a booking message on a development adapter.
type SimulateInput struct {
	Type               string               `json:"type"`
	ExternalRef        string               `json:"external_ref"`
	ProductCode        string               `json:"product_code"`
	PickupLocationCode string               `json:"pickup_location_code"`
	ReturnLocationCode string               `json:"return_location_code"`
	OfferRef           string               `json:"offer_ref"`
	StartsAt           *time.Time           `json:"starts_at"`
	EndsAt             *time.Time           `json:"ends_at"`
	Customer           channels.Customer    `json:"customer"`
	Extras             []channels.ExtraLine `json:"extras"`
	Total              string               `json:"total"`
	Commission         string               `json:"commission"`
	CollectedBy        string               `json:"collected_by"`
}

// ReconciliationLine is one booking in the channel vs Rentopia report.
type ReconciliationLine struct {
	ExternalRef       string       `json:"external_ref"`
	Match             string       `json:"match"`
	StartsAt          *time.Time   `json:"starts_at"`
	ChannelStatus     string       `json:"channel_status"`
	ChannelTotal      *money.Money `json:"channel_total"`
	ChannelCommission *money.Money `json:"channel_commission"`
	ChannelNet        *money.Money `json:"channel_net"`
	CollectedBy       string       `json:"collected_by"`
	Reservation       *Ref         `json:"reservation"`
	LocalTotal        *money.Money `json:"local_total"`
	LocalCommission   *money.Money `json:"local_commission"`
	Difference        *money.Money `json:"difference"`
	ErrorCode         string       `json:"error_code"`
}

// ReconciliationTotals sum a report.
type ReconciliationTotals struct {
	ChannelGross      money.Money `json:"channel_gross"`
	ChannelCommission money.Money `json:"channel_commission"`
	ChannelNet        money.Money `json:"channel_net"`
	LocalGross        money.Money `json:"local_gross"`
	LocalCommission   money.Money `json:"local_commission"`
	// CollectedByChannel is the net the channel owes the organization;
	// CollectedByUs the commission the organization owes the channel.
	CollectedByChannel money.Money `json:"collected_by_channel"`
	CollectedByUs      money.Money `json:"collected_by_us"`
}

// Reconciliation is the channel vs Rentopia report of a period.
type Reconciliation struct {
	Account  Ref                  `json:"account"`
	From     string               `json:"from"`
	To       string               `json:"to"`
	Source   string               `json:"source"`
	Warning  string               `json:"warning"`
	Counts   map[string]int       `json:"counts"`
	Totals   ReconciliationTotals `json:"totals"`
	Lines    []ReconciliationLine `json:"lines"`
	Currency string               `json:"currency"`
}

// QueryError is an invalid list query value.
type QueryError struct{ Field, Value string }

func (e *QueryError) Error() string { return fmt.Sprintf("invalid %s value %q", e.Field, e.Value) }

// SplitValues splits a comma-separated filter and checks each value.
func SplitValues(field, raw string, allowed []string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	out := []string{}
	for _, v := range strings.Split(raw, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if allowed != nil && !Contains(allowed, v) {
			return nil, &QueryError{Field: field, Value: v}
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
