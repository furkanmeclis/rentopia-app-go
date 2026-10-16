// Package adapters connects the channel manager to the modules it builds
// on without owning them: reservations (quotes, the reservation state
// machine on the availability engine) and customers. The organization and
// its system scope come from the context the channel manager passes.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/usecase"
	customermodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	pricingmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Booker implements usecase.Booker over reservations and customers.
type Booker struct {
	res       *resusecase.Service
	customers *customerusecase.Service
}

// NewBooker builds the adapter.
func NewBooker(res *resusecase.Service, customers *customerusecase.Service) *Booker {
	return &Booker{res: res, customers: customers}
}

var _ usecase.Booker = (*Booker)(nil)

// Availability implements usecase.Booker (a quote search: capacity with
// the channel's rules and the price of the one-day rental).
func (b *Booker) Availability(ctx context.Context, orgID int64, channel string, branch uuid.UUID, start, end time.Time, products []uuid.UUID) (map[uuid.UUID]usecase.Availability, error) {
	res, err := b.res.Search(ctx, orgID, resmodel.QuoteSearchInput{
		Where: resmodel.Where{PickupBranchUUID: &branch, StartsAt: &start, EndsAt: &end}, Channel: channel, ProductUUIDs: products,
	})
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID]usecase.Availability{}
	for _, it := range res.Items {
		a := usecase.Availability{Sellable: it.Sellable, Version: it.Product.Version}
		if it.Price != nil {
			a.HasPrice, a.DailyPrice, a.Currency = true, it.Price.DailyAverage, string(it.Price.Currency)
		}
		out[it.Product.UUID] = a
	}
	return out, nil
}

func strPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

// CreateCustomer implements usecase.Booker: an individual customer with
// the booking's name and contact data; contact values the customer rules
// refuse are left out rather than losing the booking.
func (b *Booker) CreateCustomer(ctx context.Context, orgID int64, c channels.Customer, channel, externalRef string) (uuid.UUID, error) {
	first, last := strings.TrimSpace(c.FirstName), strings.TrimSpace(c.LastName)
	if first == "" {
		first = channel
	}
	if last == "" {
		last = externalRef
	}
	individual := customermodel.TypeIndividual
	note := fmt.Sprintf("%s rezervasyonu %s", channel, externalRef)
	tags := []string{channel}
	in := customermodel.CustomerInput{
		CustomerType: &individual, FirstName: &first, LastName: &last, Phone: strPtr(c.Phone), Email: strPtr(c.Email),
		Note: &note, Tags: &tags,
	}
	if n := strings.ToUpper(strings.TrimSpace(c.Nationality)); len(n) == 2 {
		in.Nationality = customermodel.OptionalString{Set: true, Value: &n}
	}
	created, err := b.customers.Create(ctx, orgID, in)
	var ve *customerusecase.ValidationError
	if errors.As(err, &ve) {
		in.Phone, in.Email, in.Nationality = nil, nil, customermodel.OptionalString{}
		created, err = b.customers.Create(ctx, orgID, in)
	}
	if err != nil {
		return uuid.Nil, &usecase.BookingError{Code: model.ErrCodeCustomer, Message: err.Error()}
	}
	return created.UUID, nil
}

func extrasOf(in []usecase.ExtraRequest) []resmodel.ExtraRequest {
	out := make([]resmodel.ExtraRequest, 0, len(in))
	for _, x := range in {
		out = append(out, resmodel.ExtraRequest{ExtraUUID: x.ExtraUUID, Quantity: max(x.Quantity, 1)})
	}
	return out
}

// Book implements usecase.Booker: a quote for the channel, then a
// reservation confirmed at once (the channel confirmed the booking on its
// side), with the external number, the collecting party and commission.
func (b *Booker) Book(ctx context.Context, orgID int64, spec usecase.BookingSpec) (usecase.Booking, error) {
	start, end := spec.StartsAt.Truncate(time.Minute), spec.EndsAt.Truncate(time.Minute)
	customer := spec.CustomerUUID
	quote, err := b.res.CreateQuote(ctx, orgID, resmodel.QuoteInput{
		Where: resmodel.Where{
			PickupBranchUUID: &spec.PickupBranch, ReturnBranchUUID: &spec.ReturnBranch, StartsAt: &start, EndsAt: &end,
		},
		Channel: spec.Channel, ProductUUID: &spec.ProductUUID, CustomerUUID: &customer, Extras: extrasOf(spec.Extras),
	})
	if err != nil {
		return usecase.Booking{}, mapErr(err, model.ErrCodeBookingRefused)
	}
	collecting := resmodel.CollectorChannel
	if spec.CollectedBy == channels.CollectedBySupplier {
		collecting = resmodel.CollectorUs
	}
	r, err := b.res.Create(ctx, orgID, resmodel.CreateInput{
		QuoteUUID: &quote.UUID, CustomerUUID: &customer, ExternalReference: spec.ExternalRef, CollectingParty: collecting,
		CommissionAmount: pricingmodel.Decimal(spec.Commission.String()), Note: spec.Note, ConfirmNow: true,
	})
	if err != nil {
		return usecase.Booking{}, mapErr(err, model.ErrCodeBookingRefused)
	}
	return bookingOf(r), nil
}

func bookingOf(r resmodel.Reservation) usecase.Booking {
	out := usecase.Booking{
		UUID: r.UUID, Reference: r.Reference, Status: r.Status, Total: r.Price.Total, Commission: r.Price.Commission,
		Currency: r.Price.Currency, ProductUUID: r.Plan.Product.UUID, PickupBranch: r.PickupBranch.UUID,
		ReturnBranch: r.ReturnBranch.UUID, StartsAt: r.StartsAt.UTC(), EndsAt: r.EndsAt.UTC(), Extras: []usecase.ExtraRequest{},
	}
	if out.ProductUUID == uuid.Nil {
		out.ProductUUID = r.Plan.SoldProduct.UUID
	}
	for _, x := range r.Price.Snapshot.Extras {
		out.Extras = append(out.Extras, usecase.ExtraRequest{ExtraUUID: x.ExtraUUID, Quantity: x.Quantity})
	}
	return out
}

// Get implements usecase.Booker.
func (b *Booker) Get(ctx context.Context, orgID int64, id uuid.UUID) (usecase.Booking, error) {
	r, err := b.res.Get(ctx, orgID, id)
	if err != nil {
		return usecase.Booking{}, mapErr(err, model.ErrCodeChangeRefused)
	}
	return bookingOf(r), nil
}

// Change implements usecase.Booker: a change quote for the new dates,
// places, product and extras, applied at the new price (the difference is
// the channel's to settle).
func (b *Booker) Change(ctx context.Context, orgID int64, id uuid.UUID, spec usecase.BookingSpec) (usecase.Booking, error) {
	start, end := spec.StartsAt.Truncate(time.Minute), spec.EndsAt.Truncate(time.Minute)
	extras := extrasOf(spec.Extras)
	quote, err := b.res.ChangeQuote(ctx, orgID, id, resmodel.ChangeQuoteInput{
		Where: resmodel.Where{
			PickupBranchUUID: &spec.PickupBranch, ReturnBranchUUID: &spec.ReturnBranch, StartsAt: &start, EndsAt: &end,
		},
		ProductUUID: &spec.ProductUUID, Extras: &extras,
	})
	if err != nil {
		return usecase.Booking{}, mapErr(err, model.ErrCodeChangeRefused)
	}
	res, err := b.res.ApplyChange(ctx, orgID, id, resmodel.ApplyChangeInput{
		QuoteUUID: &quote.UUID, Reason: "Kanal değişikliği (" + spec.Channel + " " + spec.ExternalRef + ")",
	})
	if err != nil {
		return usecase.Booking{}, mapErr(err, model.ErrCodeChangeRefused)
	}
	return bookingOf(res.Reservation), nil
}

// SetCommission implements usecase.Booker.
func (b *Booker) SetCommission(ctx context.Context, orgID int64, id uuid.UUID, commission money.Money) error {
	if _, err := b.res.SetCommission(ctx, orgID, id, commission); err != nil {
		return mapErr(err, model.ErrCodeChangeRefused)
	}
	return nil
}

// Cancel implements usecase.Booker.
func (b *Booker) Cancel(ctx context.Context, orgID int64, id uuid.UUID, reason string) (usecase.Booking, error) {
	r, err := b.res.Cancel(ctx, orgID, id, reason)
	if err != nil {
		return usecase.Booking{}, mapErr(err, model.ErrCodeCancelRefused)
	}
	return bookingOf(r), nil
}

// mapErr turns a refusal of reservations into a BookingError the error
// queue shows; anything else stays an internal error.
func mapErr(err error, fallback string) error {
	var ce *availusecase.CapacityError
	var ec *resusecase.ExtensionConflictError
	var eq *resmodel.EquipmentError
	var pe *resusecase.PricingError
	var ve *resusecase.ValidationError
	var se *resusecase.StateError
	var qe *resusecase.QuoteError
	var el *resusecase.EligibilityError
	switch {
	case errors.As(err, &ce):
		return &usecase.BookingError{Code: model.ErrCodeCapacity, Message: "no capacity for the booking (" + ce.Reason + ")"}
	case errors.As(err, &ec):
		return &usecase.BookingError{Code: model.ErrCodeCapacity, Message: ec.Error()}
	case errors.As(err, &eq):
		return &usecase.BookingError{Code: model.ErrCodeCapacity, Message: "an extra is out of stock"}
	case errors.As(err, &pe):
		return &usecase.BookingError{Code: model.ErrCodePricing, Message: pe.Message}
	case errors.Is(err, resusecase.ErrExternalReferenceTaken):
		return &usecase.BookingError{Code: fallback, Message: err.Error(), Taken: true}
	case errors.As(err, &ve):
		msgs := []string{}
		for _, d := range ve.Details {
			msgs = append(msgs, d.Field+": "+d.Message)
		}
		return &usecase.BookingError{Code: fallback, Message: strings.Join(msgs, "; ")}
	case errors.As(err, &se):
		return &usecase.BookingError{Code: fallback, Message: se.Message}
	case errors.As(err, &qe):
		return &usecase.BookingError{Code: fallback, Message: qe.Message}
	case errors.As(err, &el):
		return &usecase.BookingError{Code: fallback, Message: el.Error()}
	case errors.Is(err, resusecase.ErrNotFound), errors.Is(err, resusecase.ErrForbidden):
		return &usecase.BookingError{Code: fallback, Message: err.Error()}
	}
	return err
}
