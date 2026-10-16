// Package channels is the provider layer of the channel manager (TEC-736,
// docs/design/channel-manager.md): marketplaces that sell the
// organization's rental products (Yolcu360 first, Obilet next).
//
// A channel Provider is an integrations.Adapter (the organization connects
// it with its own credentials, sealed by the integrations Vault) that can:
//
//   - PushInventory: send availability, price, stop-sale and the offline
//     policy for the mapped products and locations, day by day. A push is a
//     full snapshot of the horizon, so sending it again is always safe;
//   - PullMessages: read booking / modification / cancellation messages
//     after a cursor. Every message is a full snapshot of the booking with
//     a version, so a repeated or out-of-order message is recognised by its
//     id and version (the module never books twice);
//   - Acknowledge: tell the channel what happened to each message (our
//     reference, or why it was refused);
//   - Statement: the channel's own list of bookings in a period, for the
//     reconciliation report.
//
// Operations a provider does not support answer integrations.ErrUnsupported.
// The module depends only on this interface: the Yolcu360 adapter
// (package yolcu360) is built from the documented contract, and
// yolcu360_fake (package channelfake) serves tests and development.
package channels

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Kind is the integration kind of channel accounts.
const Kind = "channel"

// Message types.
const (
	MessageBooking      = "booking"
	MessageModification = "modification"
	MessageCancellation = "cancellation"
)

// Booking statuses a message reports.
const (
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
)

// Collecting parties: who took the customer's money.
const (
	CollectedByChannel  = "channel"
	CollectedBySupplier = "us"
)

// Offline policies: what the channel does when it stops hearing from us
// (the validity of the last push ran out).
const (
	OfflineClose = "close"
	OfflineQuota = "quota"
)

// Capabilities say which operations a provider supports.
type Capabilities struct {
	Availability bool `json:"availability"`
	Rates        bool `json:"rates"`
	StopSale     bool `json:"stop_sale"`
	Allotment    bool `json:"allotment"`
	Pull         bool `json:"pull"`
	Acknowledge  bool `json:"acknowledge"`
	Statement    bool `json:"statement"`
	// MaxItemsPerPush splits large pushes (0 = no limit).
	MaxItemsPerPush int `json:"max_items_per_push"`
}

// OfflinePolicy is sent with every push: after ValidUntil the channel
// closes the sale (close) or sells at most Quota units per product,
// location and day (quota). The last known capacity is never sold
// indefinitely.
type OfflinePolicy struct {
	Mode  string `json:"mode"`
	Quota int    `json:"quota"`
}

// InventoryItem is one product at one location on one day.
type InventoryItem struct {
	ProductCode  string `json:"product_code"`
	LocationCode string `json:"location_code"`
	// Date is the pick-up day (YYYY-MM-DD, Türkiye).
	Date string `json:"date"`
	// Available is the number of units the channel may sell that day
	// (after stop-sale and the mapping's quota).
	Available int  `json:"available"`
	StopSale  bool `json:"stop_sale"`
	// DailyPrice is the price of one rental day starting that day (zero
	// with HasPrice = false when the product has no price then).
	DailyPrice money.Money `json:"daily_price"`
	HasPrice   bool        `json:"has_price"`
	Currency   string      `json:"currency"`
	// OfferRef identifies the internal product / version behind the item;
	// the channel echoes it in a booking (several internal products may
	// share one external code).
	OfferRef string `json:"offer_ref"`
}

// InventoryPush is one push of the horizon.
type InventoryPush struct {
	Items      []InventoryItem `json:"items"`
	ValidUntil time.Time       `json:"valid_until"`
	Offline    OfflinePolicy   `json:"offline"`
}

// ItemError is a refused item of a push.
type ItemError struct {
	Index   int    `json:"index"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PushReceipt is the channel's answer to a push.
type PushReceipt struct {
	Reference string      `json:"reference"`
	Accepted  int         `json:"accepted"`
	Rejected  int         `json:"rejected"`
	Errors    []ItemError `json:"errors"`
}

// Customer is the renter a booking names (personal data: sealed at rest).
type Customer struct {
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Nationality string `json:"nationality"`
}

// ExtraLine is an extra service of a booking (external code).
type ExtraLine struct {
	Code     string `json:"code"`
	Quantity int    `json:"quantity"`
}

// Message is one booking message: a full snapshot of the booking at
// Version. Messages of a booking may arrive repeated or out of order.
type Message struct {
	// ID identifies the message at the channel (idempotency key).
	ID string `json:"id"`
	// Type: booking, modification, cancellation.
	Type string `json:"type"`
	// ExternalRef is the channel's booking number (unique per channel).
	ExternalRef string `json:"external_ref"`
	// Version orders the messages of a booking (higher = newer).
	Version    int64     `json:"version"`
	OccurredAt time.Time `json:"occurred_at"`
	// Status is the booking state after the message: confirmed, cancelled.
	Status             string      `json:"status"`
	ProductCode        string      `json:"product_code"`
	PickupLocationCode string      `json:"pickup_location_code"`
	ReturnLocationCode string      `json:"return_location_code"`
	OfferRef           string      `json:"offer_ref"`
	StartsAt           time.Time   `json:"starts_at"`
	EndsAt             time.Time   `json:"ends_at"`
	Customer           Customer    `json:"customer"`
	Extras             []ExtraLine `json:"extras"`
	Currency           string      `json:"currency"`
	// Total is the amount the customer pays for the booking; Commission
	// the channel's share; Prepaid what the channel already collected.
	Total      money.Money `json:"total"`
	Commission money.Money `json:"commission"`
	Prepaid    money.Money `json:"prepaid"`
	// CollectedBy: channel (the channel took the money) or us (the
	// customer pays at the counter). Empty = the account's default.
	CollectedBy  string `json:"collected_by"`
	FlightNumber string `json:"flight_number"`
	Note         string `json:"note"`
}

// MessageBatch is one page of messages after a cursor.
type MessageBatch struct {
	Messages []Message
	// Cursor is where the next pull continues ("" = unchanged).
	Cursor string
	More   bool
}

// Ack is the result of one message, told back to the channel.
type Ack struct {
	MessageID   string `json:"message_id"`
	ExternalRef string `json:"external_ref"`
	Accepted    bool   `json:"accepted"`
	// Reference is our reservation reference (accepted bookings).
	Reference string `json:"reference"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// StatementLine is one booking as the channel accounts it.
type StatementLine struct {
	ExternalRef string      `json:"external_ref"`
	Status      string      `json:"status"`
	StartsAt    time.Time   `json:"starts_at"`
	Currency    string      `json:"currency"`
	Total       money.Money `json:"total"`
	Commission  money.Money `json:"commission"`
	Net         money.Money `json:"net"`
	CollectedBy string      `json:"collected_by"`
}

// Provider is one channel adapter.
type Provider interface {
	integrations.Adapter
	// Channel is the sales channel code of the bookings (orgsettings
	// SalesChannels: "yolcu360").
	Channel() string
	Capabilities() Capabilities
	// PushInventory sends one snapshot. ErrUnavailable: nothing reached
	// the channel; ErrRejected: refused as a whole (credentials, format);
	// any other error: unknown (the next push replaces it anyway).
	PushInventory(ctx context.Context, account integrations.Account, push InventoryPush) (PushReceipt, error)
	// PullMessages reads the messages after cursor. Read only: asking
	// again is always safe.
	PullMessages(ctx context.Context, account integrations.Account, cursor string) (MessageBatch, error)
	// Acknowledge reports the results of messages (idempotent).
	Acknowledge(ctx context.Context, account integrations.Account, acks []Ack) error
	// Statement lists the channel's bookings picked up in [from, to).
	Statement(ctx context.Context, account integrations.Account, from, to time.Time) ([]StatementLine, error)
}

// Simulator is implemented by development providers: it queues a message
// as if the channel had sent it (the "simulate booking" action).
type Simulator interface {
	Simulate(account integrations.Account, m Message) Message
}

var (
	// ErrUnavailable: the request did not reach the channel (connection
	// refused, DNS); nothing happened there.
	ErrUnavailable = errors.New("channels: channel unavailable (nothing was sent)")
	// ErrRejected: the channel refused the request (credentials, format).
	ErrRejected = errors.New("channels: the channel rejected the request")
)
