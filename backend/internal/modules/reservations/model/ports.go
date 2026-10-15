package model

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Narrow ports to the modules reservations build on but do not own
// (docs/design/reservations.md §7). Extras and coupons are PRICED by the
// pricing preview (TEC-702 / TEC-703 resolvers inside pricing.Quote); what
// reservations need on top is to hold physical equipment and to redeem the
// campaign. The adapters (reservations/adapters) wire the extras and
// campaigns services; the defaults below are no-ops for tests and for
// deployments without those modules.

// ExtraRequest is one requested extra service (catalog extra + quantity),
// the same shape as the pricing preview's extras.
type ExtraRequest struct {
	ExtraUUID uuid.UUID `json:"extra_uuid"`
	Quantity  int       `json:"quantity"`
}

// EquipmentRequest holds physical equipment for a reservation. It is tied
// to the reservation's vehicle allocation: the database trigger of the
// extras module then follows the vehicle allocation (hold → reservation,
// hold expiry, release / expiry), so confirmation, cancellation and expiry
// need no call of their own.
type EquipmentRequest struct {
	ExtraUUID             uuid.UUID
	Quantity              int
	VehicleAllocationUUID uuid.UUID
	ReservationUUID       uuid.UUID
	ActorID               *int64
}

// EquipmentReserver holds / frees equipment stock (TEC-702). Reserve
// returns uuid.Nil (no error) for an extra that is not physical; a stock
// shortage is an error the caller reports (EQUIPMENT_UNAVAILABLE).
type EquipmentReserver interface {
	ReserveEquipment(ctx context.Context, orgID int64, in EquipmentRequest) (uuid.UUID, error)
	ReleaseEquipment(ctx context.Context, orgID int64, allocationUUID uuid.UUID, reason string) error
}

// CouponRedemption redeems the campaign a quote applied.
type CouponRedemption struct {
	Code            string
	CampaignUUID    *uuid.UUID
	ProductID       int64
	SegmentID       int64
	Channel         string
	Subtotal        money.Money
	CustomerUUID    *uuid.UUID
	ReservationUUID uuid.UUID
	ActorID         *int64
}

// CouponRedeemer keeps the campaign redemption counter (TEC-703): Redeem
// when the reservation is created (one live redemption per reservation,
// atomic usage counter), Release when it is cancelled / expires / is a
// no-show, Consume when the rental completes (M3).
type CouponRedeemer interface {
	Redeem(ctx context.Context, orgID int64, in CouponRedemption) error
	Release(ctx context.Context, orgID int64, reservationUUID uuid.UUID, reason string) error
	Consume(ctx context.Context, orgID int64, reservationUUID uuid.UUID) error
}

// PaymentUpdate is what the payments module tells a reservation after a
// verified payment or a refund: Paid is the total received so far (refunds
// deducted), Refunded marks that refunds happened, Payment names the
// payment that caused the update (uuid.Nil for a plain recount) and
// Amount is that payment's amount.
type PaymentUpdate struct {
	Paid     money.Money
	Refunded bool
	Payment  uuid.UUID
	Amount   money.Money
	Source   string
}

// Payment outcomes (payments.outcome).
const (
	// OutcomeRecorded: the payment status moved, nothing else.
	OutcomeRecorded = "recorded"
	// OutcomeConfirmed: the prepayment is covered; awaiting_payment →
	// confirmed (hold → firm on the same allocation).
	OutcomeConfirmed = "confirmed"
	// OutcomeReinstated: a late payment after expiry found capacity again;
	// expired → confirmed on a new firm allocation (recorded).
	OutcomeReinstated = "reinstated"
	// OutcomeLateTask: a late payment could not be honoured (capacity gone
	// or the reservation was cancelled): a late_payment resolution task
	// (refund / re-book) is open; never confirmed silently.
	OutcomeLateTask = "late_payment_task"
)

// PaymentRecorder is the M3 payments entry point (TEC-707 / TEC-708): a
// verified payment / refund updates payment_status (unpaid /
// partially_paid / paid / refunded) and paid_amount; a reservation
// awaiting its prepayment is confirmed once it is covered (in both
// approval modes: in staff mode it only reaches awaiting_payment after the
// staff accepted it); a payment that arrives after the hold expired is
// re-validated against current capacity. Implemented by usecase.Service.
type PaymentRecorder interface {
	RecordPayment(ctx context.Context, orgID int64, reservationUUID uuid.UUID, in PaymentUpdate) (string, error)
}

// PaymentLinkRef is a created pay-by-link link.
type PaymentLinkRef struct {
	UUID      uuid.UUID   `json:"uuid"`
	URL       string      `json:"url"`
	Amount    money.Money `json:"amount"`
	ExpiresAt time.Time   `json:"expires_at"`
}

// PaymentLinker creates a pay-by-link link for a reservation (staff accept
// with "send payment link"); implemented by the payments module.
type PaymentLinker interface {
	CreateReservationLink(ctx context.Context, orgID int64, reservationUUID uuid.UUID, amount money.Money, expiresAt time.Time) (PaymentLinkRef, error)
}

// ErrCouponUnavailable: a coupon was used without the campaigns module.
var ErrCouponUnavailable = errors.New("coupons are not available")

// CouponError is a campaign refusal at redemption with the campaigns
// module's reason code (limit, customer_limit, expired …).
type CouponError struct {
	Reason string
	Params map[string]string
}

func (e *CouponError) Error() string { return "coupon rejected: " + e.Reason }

// EquipmentError is a stock shortage (409 EQUIPMENT_UNAVAILABLE).
type EquipmentError struct {
	ExtraUUID uuid.UUID
	Stock     int
	Available int
	Requested int
}

func (e *EquipmentError) Error() string { return "equipment unavailable" }

// NoEquipment holds nothing (extras module not wired).
type NoEquipment struct{}

// ReserveEquipment implements EquipmentReserver.
func (NoEquipment) ReserveEquipment(context.Context, int64, EquipmentRequest) (uuid.UUID, error) {
	return uuid.Nil, nil
}

// ReleaseEquipment implements EquipmentReserver.
func (NoEquipment) ReleaseEquipment(context.Context, int64, uuid.UUID, string) error { return nil }

// NoCoupons redeems nothing (campaigns module not wired): a code is refused.
type NoCoupons struct{}

// Redeem implements CouponRedeemer.
func (NoCoupons) Redeem(_ context.Context, _ int64, in CouponRedemption) error {
	if in.Code != "" || in.CampaignUUID != nil {
		return ErrCouponUnavailable
	}
	return nil
}

// Release implements CouponRedeemer.
func (NoCoupons) Release(context.Context, int64, uuid.UUID, string) error { return nil }

// Consume implements CouponRedeemer.
func (NoCoupons) Consume(context.Context, int64, uuid.UUID) error { return nil }
