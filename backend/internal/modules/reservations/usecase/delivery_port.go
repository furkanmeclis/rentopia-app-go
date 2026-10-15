package usecase

import (
	"context"
	"encoding/json"
	"time"

	fieldmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/placement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/google/uuid"
)

// Delivery to an address or an airport (TEC-724): a quote whose pick-up or
// return is a delivery point is checked for eligibility (area, hours,
// notice) and gets the delivery fee as a price line. The field tasks
// module implements the port (service points, the delivery extra's price);
// docs/design/field-tasks.md §3.

// DeliverySide is one side (pick-up / return) of a quote at a point.
type DeliverySide struct {
	PointUUID uuid.UUID
	Details   *fieldmodel.Delivery
	// Check applies the eligibility rules (false: an unchanged side of a
	// change quote, the started pick-up of an extension; only the fee).
	Check bool
}

// DeliveryInput is what a quote asks of the delivery port.
type DeliveryInput struct {
	Channel     string
	ProductUUID uuid.UUID
	StartsAt    time.Time
	EndsAt      time.Time
	Pickup      *DeliverySide
	Return      *DeliverySide
}

// DeliveryQuote is the answer: the fee lines and the refused rules (field
// details on <side>_point_uuid / <side>_delivery.*).
type DeliveryQuote struct {
	Fees     []pricing.DeliveryFee
	Problems []Detail
}

// DeliveryQuoter checks delivery points and prices their fees.
type DeliveryQuoter interface {
	QuoteDelivery(ctx context.Context, orgID int64, in DeliveryInput) (DeliveryQuote, error)
}

// SetDeliveryQuoter wires the field tasks module (nil = delivery points
// carry no fee and no eligibility rule, the M2 behaviour).
func (s *Service) SetDeliveryQuoter(d DeliveryQuoter) { s.delivery = d }

// Listeners fans one lifecycle event out to several listeners, in order
// (rentals first: a fixed plate creates the rental, then field tasks
// place the delivery).
type Listeners []LifecycleListener

// ReservationChanged implements LifecycleListener.
func (l Listeners) ReservationChanged(ctx context.Context, orgID int64, reservation uuid.UUID, event string) {
	for _, x := range l {
		if x != nil {
			x.ReservationChanged(ctx, orgID, reservation, event)
		}
	}
}

func deliveryJSON(d *fieldmodel.Delivery) []byte {
	if d == nil {
		return nil
	}
	d.Normalize()
	if d.Empty() {
		return nil
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil
	}
	return b
}

func sameDelivery(a, b *fieldmodel.Delivery) bool {
	return string(deliveryJSON(a)) == string(deliveryJSON(b))
}

// cleanDelivery normalizes a request's delivery details (nil when empty).
func cleanDelivery(d *fieldmodel.Delivery) *fieldmodel.Delivery {
	if d == nil {
		return nil
	}
	c := *d
	c.Normalize()
	if c.Empty() {
		return nil
	}
	return &c
}

// validateDelivery checks the shape of delivery details (lengths,
// coordinates); a side without a point cannot carry an address.
func validateDelivery(ve *ValidationError, prefix string, d *fieldmodel.Delivery, hasPoint bool) {
	if d == nil {
		return
	}
	field := prefix + "_delivery."
	if d.Address != "" && !hasPoint {
		ve.add(field+"address", "point_required", "choose a delivery point for an address")
	}
	if len([]rune(d.Address)) > 500 {
		ve.add(field+"address", "too_long", "address is at most 500 characters")
	}
	if len([]rune(d.District)) > 128 {
		ve.add(field+"district", "too_long", "district is at most 128 characters")
	}
	if len([]rune(d.City)) > 128 {
		ve.add(field+"city", "too_long", "city is at most 128 characters")
	}
	if len([]rune(d.ContactName)) > 255 {
		ve.add(field+"contact_name", "too_long", "contact_name is at most 255 characters")
	}
	if len([]rune(d.ContactPhone)) > 32 {
		ve.add(field+"contact_phone", "too_long", "contact_phone is at most 32 characters")
	}
	if len([]rune(d.FlightNumber)) > 16 {
		ve.add(field+"flight_number", "too_long", "flight_number is at most 16 characters")
	}
	if len([]rune(d.MeetingNote)) > 1000 {
		ve.add(field+"meeting_note", "too_long", "meeting_note is at most 1000 characters")
	}
	if (d.Latitude == nil) != (d.Longitude == nil) {
		ve.add(field+"latitude", "incomplete", "give both latitude and longitude")
	}
	if d.Latitude != nil && (*d.Latitude < -90 || *d.Latitude > 90) {
		ve.add(field+"latitude", "out_of_range", "latitude is -90..90")
	}
	if d.Longitude != nil && (*d.Longitude < -180 || *d.Longitude > 180) {
		ve.add(field+"longitude", "out_of_range", "longitude is -180..180")
	}
}

// storedDelivery reads a stored delivery JSON.
func storedDelivery(raw []byte) *fieldmodel.Delivery { return placement.ParseDelivery(raw) }
