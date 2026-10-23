package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	ChannelInapp    = "inapp"
	ChannelEmail    = "email"
	ChannelRealtime = "realtime"
	ChannelSMS      = "sms"
	ChannelPush     = "push"

	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSent       = "sent"
	StatusDelivered  = "delivered"
	StatusRead       = "read"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"

	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

// Notification is the API projection.
type Notification struct {
	UUID            uuid.UUID         `json:"uuid"`
	Channel         string            `json:"channel"`
	Status          string            `json:"status"`
	Priority        string            `json:"priority"`
	Title           string            `json:"title"`
	Body            string            `json:"body"`
	Payload         map[string]any    `json:"payload"`
	ActionURL       *string           `json:"action_url,omitempty"`
	SignedActionURL *string           `json:"signed_action_url,omitempty"`
	Recipient       *string           `json:"recipient,omitempty"`
	TemplateCode    *string           `json:"template_code,omitempty"`
	SourceEvent     *string           `json:"source_event,omitempty"`
	SentAt          *time.Time        `json:"sent_at,omitempty"`
	ReadAt          *time.Time        `json:"read_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	User            *NotificationUser `json:"user,omitempty"`
	// OrganizationUUID is set on notifications of an organization (tenant
	// engine jobs); nil for platform and personal notifications.
	OrganizationUUID *uuid.UUID `json:"organization_uuid,omitempty"`
}

// NotificationUser is the target user on a platform notification row.
type NotificationUser struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// PushDevice is a registered mobile device (Expo push token).
type PushDevice struct {
	UUID       uuid.UUID `json:"uuid"`
	Token      string    `json:"token"`
	Platform   string    `json:"platform"`
	Locale     string    `json:"locale"`
	AppVersion string    `json:"app_version"`
	DeviceName string    `json:"device_name"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// Preferences is the user notification preference set.
type Preferences struct {
	EmailEnabled    bool `json:"email_enabled"`
	InappEnabled    bool `json:"inapp_enabled"`
	RealtimeEnabled bool `json:"realtime_enabled"`
	PushEnabled     bool `json:"push_enabled"`
}

// EnqueueInput creates one or more queued notifications.
type EnqueueInput struct {
	// OrganizationID stamps the notification with an organization: it then
	// shows only in that organization's panel (and never in the platform
	// shell). Tenant engine jobs pass the job's organization.
	OrganizationID *int64
	UserID         *int64
	UserUUID       *uuid.UUID
	Channels       []string
	Priority       string
	Title          string
	Body           string
	Payload        map[string]any
	ActionURL      *string
	Recipient      *string
	TemplateCode   string
	TemplateVars   map[string]string
	SourceEvent    string
	Language       string
	// SecurityEmail bypasses email preference (password reset / verification).
	SecurityEmail bool
	// IdempotencyKey (optional) names the cause of the notification and its
	// recipient (EventKey for outbox events, "notify:<delivery>" for the
	// notification engine). Each channel's row stores RowKey(key, channel)
	// under a unique index: a replayed event or a retried caller gets the
	// existing row back and nothing is delivered twice (TEC-962).
	IdempotencyKey string
}

// EventKey is the idempotency key of what one domain event notifies one
// recipient of: the event id (stable across outbox replays), the kind
// (template code) and the recipient.
func EventKey(eventID uuid.UUID, kind string, userID int64) string {
	return "event:" + eventID.String() + "|" + kind + "|user:" + strconv.FormatInt(userID, 10)
}

// maxKeyLen is notifications.idempotency_key's length.
const maxKeyLen = 255

// RowKey is the stored key of one channel's row. A key that does not fit
// the column is replaced by its SHA-256 (still deterministic).
func RowKey(base, channel string) string {
	if base == "" {
		return ""
	}
	k := base + "|" + channel
	if len(k) <= maxKeyLen {
		return k
	}
	sum := sha256.Sum256([]byte(k))
	return "sha256:" + hex.EncodeToString(sum[:])
}
