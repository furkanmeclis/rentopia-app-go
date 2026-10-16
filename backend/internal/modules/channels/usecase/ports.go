package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Availability is what one product can sell for a one-day rental starting
// on a day at a branch through the channel (channel rules applied).
type Availability struct {
	Sellable   int
	Version    int32
	DailyPrice money.Money
	HasPrice   bool
	Currency   string
}

// ExtraRequest is an extra of a booking (internal extra).
type ExtraRequest struct {
	ExtraUUID uuid.UUID
	Quantity  int
}

// BookingSpec is the reservation a channel booking asks for.
type BookingSpec struct {
	Channel      string
	ExternalRef  string
	ProductUUID  uuid.UUID
	PickupBranch uuid.UUID
	ReturnBranch uuid.UUID
	StartsAt     time.Time
	EndsAt       time.Time
	Extras       []ExtraRequest
	CustomerUUID uuid.UUID
	// CollectedBy: channel or us (reservation collecting party).
	CollectedBy string
	Commission  money.Money
	Note        string
}

// Booking is a reservation as the channel manager sees it.
type Booking struct {
	UUID         uuid.UUID
	Reference    string
	Status       string
	Total        money.Money
	Commission   money.Money
	Currency     string
	ProductUUID  uuid.UUID
	PickupBranch uuid.UUID
	ReturnBranch uuid.UUID
	StartsAt     time.Time
	EndsAt       time.Time
	Extras       []ExtraRequest
}

// BookingError is a refusal of the reservations module the error queue
// shows (code = one of model.ErrCode*).
type BookingError struct {
	Code    string
	Message string
	// Taken: a reservation with the external reference exists already
	// (an earlier attempt whose link was not stored).
	Taken bool
}

func (e *BookingError) Error() string { return e.Code + ": " + e.Message }

// Booker creates, changes and cancels reservations for channel bookings
// (implemented by channels/adapters over the reservations and customers
// modules). The context carries the organization's system scope.
type Booker interface {
	// Availability answers per product for [start, end) at a branch.
	Availability(ctx context.Context, orgID int64, channel string, branch uuid.UUID, start, end time.Time, products []uuid.UUID) (map[uuid.UUID]Availability, error)
	// CreateCustomer creates the renter of a booking.
	CreateCustomer(ctx context.Context, orgID int64, c channels.Customer, channel, externalRef string) (uuid.UUID, error)
	// Book creates a confirmed reservation (quote + reservation).
	Book(ctx context.Context, orgID int64, spec BookingSpec) (Booking, error)
	// Get reads a reservation.
	Get(ctx context.Context, orgID int64, id uuid.UUID) (Booking, error)
	// Change re-plans and re-prices a reservation to the spec.
	Change(ctx context.Context, orgID int64, id uuid.UUID, spec BookingSpec) (Booking, error)
	// SetCommission records the channel's commission (channel-collected).
	SetCommission(ctx context.Context, orgID int64, id uuid.UUID, commission money.Money) error
	// Cancel cancels a reservation (capacity released once).
	Cancel(ctx context.Context, orgID int64, id uuid.UUID, reason string) (Booking, error)
}
