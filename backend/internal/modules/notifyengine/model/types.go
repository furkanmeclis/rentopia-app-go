// Package model holds the API shapes of the notification engine (TEC-733).
package model

import (
	"time"

	"github.com/google/uuid"
)

// ChannelInfo is a channel the rules may use.
type ChannelInfo struct {
	Name string `json:"name"`
	// Available: an adapter is installed (WhatsApp before TEC-735 is not);
	// a rule may still list an unavailable channel, it is then skipped.
	Available bool `json:"available"`
}

// EventInfo is a catalog event with its effective rule.
type EventInfo struct {
	Code      string     `json:"code"`
	Group     string     `json:"group"`
	Audience  string     `json:"audience"`
	Instant   bool       `json:"instant"`
	Reminder  bool       `json:"reminder"`
	Variables []string   `json:"variables"`
	Rule      RuleView   `json:"rule"`
	Default   RuleView   `json:"default_rule"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// RuleView is a rule.
type RuleView struct {
	Enabled         bool     `json:"enabled"`
	Channels        []string `json:"channels"`
	Fallback        bool     `json:"fallback"`
	ReminderOffsets []int32  `json:"reminder_offsets"`
	Custom          bool     `json:"custom"`
}

// RuleInput replaces an event's rule.
type RuleInput struct {
	Event           string   `json:"event"`
	Enabled         bool     `json:"enabled"`
	Channels        []string `json:"channels"`
	Fallback        bool     `json:"fallback"`
	ReminderOffsets []int32  `json:"reminder_offsets"`
}

// Template is one event / channel / language message.
type Template struct {
	Event     string     `json:"event"`
	Channel   string     `json:"channel"`
	Language  string     `json:"language"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	Custom    bool       `json:"custom"`
	Variables []string   `json:"variables"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// TemplateKey names a rule (event) or a template (event, channel,
// language).
type TemplateKey struct {
	Event    string `json:"event"`
	Channel  string `json:"channel"`
	Language string `json:"language"`
}

// TemplateInput replaces a template.
type TemplateInput struct {
	Event    string `json:"event"`
	Channel  string `json:"channel"`
	Language string `json:"language"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

// PreviewInput renders a template with sample values.
type PreviewInput struct {
	Event    string `json:"event"`
	Channel  string `json:"channel"`
	Language string `json:"language"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

// Preview is a rendered template.
type Preview struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	// Unknown lists {{placeholders}} that are not variables of the event.
	Unknown []string `json:"unknown"`
}

// Delivery is one row of the delivery history.
type Delivery struct {
	UUID            uuid.UUID  `json:"uuid"`
	DispatchUUID    uuid.UUID  `json:"dispatch_uuid"`
	Event           string     `json:"event"`
	Audience        string     `json:"audience"`
	Channel         string     `json:"channel"`
	AttemptNo       int32      `json:"attempt_no"`
	Status          string     `json:"status"`
	DispatchStatus  string     `json:"dispatch_status"`
	Provider        string     `json:"provider"`
	ProviderRef     string     `json:"provider_ref"`
	RecipientMasked string     `json:"recipient_masked"`
	RecipientName   string     `json:"recipient_name"`
	CustomerUUID    *uuid.UUID `json:"customer_uuid,omitempty"`
	UserUUID        *uuid.UUID `json:"user_uuid,omitempty"`
	Subject         string     `json:"subject"`
	Error           string     `json:"error"`
	RefType         string     `json:"ref_type"`
	RefUUID         *uuid.UUID `json:"ref_uuid,omitempty"`
	Test            bool       `json:"test"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// DeliveryFilter filters the history.
type DeliveryFilter struct {
	Channels    []string
	Statuses    []string
	Events      []string
	Audiences   []string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Q           string
	SortField   string
	SortDesc    bool
	Limit       int32
	Offset      int32
}

// Attempt is one channel attempt of a dispatch.
type Attempt struct {
	UUID            uuid.UUID  `json:"uuid"`
	Channel         string     `json:"channel"`
	AttemptNo       int32      `json:"attempt_no"`
	Status          string     `json:"status"`
	Provider        string     `json:"provider"`
	ProviderRef     string     `json:"provider_ref"`
	RecipientMasked string     `json:"recipient_masked"`
	Subject         string     `json:"subject"`
	Body            string     `json:"body"`
	Error           string     `json:"error"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Dispatch is one event for one recipient with its attempts.
type Dispatch struct {
	UUID          uuid.UUID  `json:"uuid"`
	Event         string     `json:"event"`
	Audience      string     `json:"audience"`
	Status        string     `json:"status"`
	Channels      []string   `json:"channels"`
	Fallback      bool       `json:"fallback"`
	Language      string     `json:"language"`
	RecipientName string     `json:"recipient_name"`
	CustomerUUID  *uuid.UUID `json:"customer_uuid,omitempty"`
	UserUUID      *uuid.UUID `json:"user_uuid,omitempty"`
	RefType       string     `json:"ref_type"`
	RefUUID       *uuid.UUID `json:"ref_uuid,omitempty"`
	LastError     string     `json:"last_error"`
	Test          bool       `json:"test"`
	ScheduledAt   time.Time  `json:"scheduled_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	Attempts      []Attempt  `json:"attempts"`
}

// TestInput sends an event to a customer (or the caller for staff events)
// with sample values.
type TestInput struct {
	Event        string     `json:"event"`
	CustomerUUID *uuid.UUID `json:"customer_uuid"`
	// Channel limits the test to one channel; empty runs the rule.
	Channel string `json:"channel"`
}

// FacetValue is one facet option.
type FacetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the history filter options.
type Facets struct {
	Channel []FacetValue `json:"channel"`
	Status  []FacetValue `json:"status"`
	Event   []FacetValue `json:"event"`
}

// PushSubscriptionInput is a browser push subscription.
type PushSubscriptionInput struct {
	Endpoint  string `json:"endpoint"`
	KeyP256dh string `json:"key_p256dh"`
	KeyAuth   string `json:"key_auth"`
}
