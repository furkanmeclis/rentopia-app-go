package model

import (
	"context"
	"errors"

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

// PaymentRecorder is the M3 payments entry point: a collected / refunded
// amount updates payment_status (unpaid / partially_paid / paid / refunded)
// and, in auto approval mode, confirms a reservation waiting for its
// prepayment. Implemented by usecase.Service.
type PaymentRecorder interface {
	RecordPayment(ctx context.Context, orgID int64, reservationUUID uuid.UUID, paid money.Money, refunded bool) error
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
