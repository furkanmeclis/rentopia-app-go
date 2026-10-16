package usecase

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	portalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	portalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/usecase"
	customermodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Booking limits.
const (
	maxDrivers       = 5
	maxNote          = 1000
	maxExternalRef   = 64
	maxRequestNote   = 2000
	accessTokenBytes = 24
)

var fieldRenames = map[string]string{
	"product_uuid": "subgroup_uuid", "product_uuids": "subgroup_uuids", "customer_uuid": "customer", "drivers": "drivers",
}

// publicFieldErr renames the panel's field names in validation errors of
// the reservation use cases ("product_uuid" → "subgroup_uuid").
func publicFieldErr(err error) error {
	var ve *resusecase.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	out := &ValidationError{}
	for _, d := range ve.Details {
		field := d.Field
		head, rest, _ := strings.Cut(field, ".")
		if renamed, ok := fieldRenames[head]; ok {
			field = renamed
			if rest != "" {
				field += "." + rest
			}
		}
		out.add(field, d.Code, d.Message)
	}
	return out
}

// customerFieldErr prefixes the customers use case's validation details
// ("first_name" → "customer.first_name", "drivers.0.birth_date").
func customerFieldErr(err error, prefix string) error {
	var ve *customerusecase.ValidationError
	if errors.As(err, &ve) {
		out := &ValidationError{}
		for _, d := range ve.Details {
			out.add(prefix+"."+d.Field, d.Code, d.Message)
		}
		return out
	}
	switch {
	case errors.Is(err, customerusecase.ErrLicenceTaken):
		return validation(prefix+".licence_number", "duplicate", "two drivers have the same licence number")
	case errors.Is(err, customerusecase.ErrIdentityTaken):
		return validation(prefix+".identity_number", "taken", err.Error())
	}
	return err
}

func pointPlace(p *resmodel.PointRef) *model.Place {
	if p == nil {
		return nil
	}
	v := model.Place{UUID: p.UUID, Name: p.Name}
	return &v
}

// publicQuote is the public view of a quote.
func publicQuote(q resmodel.Quote) model.Quote {
	out := model.Quote{
		UUID: q.UUID, Status: q.Status, Expired: q.Expired, ValidUntil: q.ValidUntil,
		Subgroup:     model.SubgroupRef{UUID: q.Product.UUID, Code: q.Product.Code, Title: q.Product.Title},
		PickupBranch: model.Place{UUID: q.PickupBranch.UUID, Name: q.PickupBranch.Name},
		ReturnBranch: model.Place{UUID: q.ReturnBranch.UUID, Name: q.ReturnBranch.Name},
		PickupPoint:  pointPlace(q.PickupPoint), ReturnPoint: pointPlace(q.ReturnPoint),
		StartsAt: q.StartsAt, EndsAt: q.EndsAt, Channel: q.Channel, BillingDays: q.BillingDays, Currency: q.Currency,
		RentalTotal: q.RentalTotal, DiscountTotal: q.DiscountTotal, ExtrasTotal: q.ExtrasTotal, OneWayFee: q.OneWayFee,
		DeliveryFee: q.DeliveryFee, VATTotal: q.VATTotal, Total: q.Total, Prepayment: q.Prepayment,
		CouponCode: q.CouponCode, Extras: []model.ExtraRequest{}, Lines: []model.QuoteLine{},
	}
	for _, e := range q.Extras {
		out.Extras = append(out.Extras, model.ExtraRequest{ExtraUUID: e.ExtraUUID, Quantity: e.Quantity})
	}
	for _, l := range q.Lines {
		out.Lines = append(out.Lines, model.QuoteLine{
			Kind: l.Kind, Code: l.Code, Name: l.Name, DayDate: l.DayDate, Quantity: l.Quantity, UnitPrice: l.UnitPrice, Amount: l.Amount,
		})
	}
	return out
}

// CreateQuote prices and stores a quote on the credential's channel. The
// quote is recorded as made by the public API: only such quotes can be
// booked through it.
func (s *Service) CreateQuote(ctx context.Context, c model.Credential, in model.QuoteInput) (model.Quote, error) {
	ve := &ValidationError{}
	if in.SubgroupUUID == nil {
		ve.add("subgroup_uuid", "required", "subgroup_uuid is required")
	}
	if in.StartsAt != nil && in.StartsAt.Before(s.clock()) {
		ve.add("starts_at", "in_past", "the pick-up time is in the past")
	}
	if len(in.Extras) > 20 {
		ve.add("extras", "too_many", "at most 20 extras")
	}
	if err := ve.orNil(); err != nil {
		return model.Quote{}, err
	}
	extras := make([]resmodel.ExtraRequest, 0, len(in.Extras))
	for _, e := range in.Extras {
		extras = append(extras, resmodel.ExtraRequest{ExtraUUID: e.ExtraUUID, Quantity: e.Quantity})
	}
	octx := credentialContext(ctx, c)
	q, err := s.reservations.CreateQuote(octx, c.OrganizationID, resmodel.QuoteInput{
		Where: resmodel.Where{
			PickupBranchUUID: in.PickupBranchUUID, PickupPointUUID: in.PickupPointUUID, ReturnBranchUUID: in.ReturnBranchUUID,
			ReturnPointUUID: in.ReturnPointUUID, StartsAt: in.StartsAt, EndsAt: in.EndsAt,
			PickupDelivery: in.PickupDelivery, ReturnDelivery: in.ReturnDelivery,
		},
		Channel: c.Channel, ProductUUID: in.SubgroupUUID, Extras: extras, CouponCode: in.CouponCode,
	})
	if err != nil {
		return model.Quote{}, publicFieldErr(err)
	}
	if err := s.q.InsertPublicQuote(ctx, db.InsertPublicQuoteParams{
		OrganizationID: c.OrganizationID, QuoteUuid: q.UUID, KeyID: keyIDArg(c), Channel: c.Channel,
	}); err != nil {
		return model.Quote{}, err
	}
	return publicQuote(q), nil
}

// GetQuote reads back a quote the public API made.
func (s *Service) GetQuote(ctx context.Context, c model.Credential, id uuid.UUID) (model.Quote, error) {
	pq, err := s.q.GetPublicQuote(ctx, db.GetPublicQuoteParams{OrganizationID: c.OrganizationID, QuoteUuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && pq.Channel != c.Channel) {
		return model.Quote{}, ErrNotFound
	}
	if err != nil {
		return model.Quote{}, err
	}
	q, err := s.reservations.GetQuote(credentialContext(ctx, c), c.OrganizationID, id)
	if errors.Is(err, resusecase.ErrQuoteNotFound) {
		return model.Quote{}, ErrNotFound
	}
	if err != nil {
		return model.Quote{}, err
	}
	return publicQuote(q), nil
}

func validateReservation(c model.Credential, in model.ReservationInput) error {
	ve := &ValidationError{}
	if in.QuoteUUID == nil {
		ve.add("quote_uuid", "required", "quote_uuid is required")
	}
	if in.Customer == nil {
		ve.add("customer", "required", "customer is required")
	} else {
		p := in.Customer
		if p.Type == "" {
			p.Type = customermodel.TypeIndividual
		}
		if p.Type != customermodel.TypeIndividual && p.Type != customermodel.TypeCorporate {
			ve.add("customer.type", "invalid", "type is individual or corporate")
		}
		if strings.TrimSpace(p.Phone) == "" {
			ve.add("customer.phone", "required", "phone is required")
		}
		if strings.TrimSpace(p.Email) == "" {
			ve.add("customer.email", "required", "email is required")
		}
	}
	switch {
	case len(in.Drivers) == 0:
		ve.add("drivers", "required", "at least the main driver is required")
	case len(in.Drivers) > maxDrivers:
		ve.add("drivers", "too_many", "at most 5 drivers")
	}
	for i, d := range in.Drivers {
		f := "drivers." + itoa(i)
		if d.SameAsCustomer {
			if in.Customer != nil && in.Customer.Type == customermodel.TypeCorporate {
				ve.add(f+".same_as_customer", "not_individual", "a corporate customer cannot be a driver")
			}
		} else {
			if strings.TrimSpace(d.FirstName) == "" {
				ve.add(f+".first_name", "required", "first_name is required")
			}
			if strings.TrimSpace(d.LastName) == "" {
				ve.add(f+".last_name", "required", "last_name is required")
			}
		}
		if strings.TrimSpace(d.BirthDate) == "" {
			ve.add(f+".birth_date", "required", "birth_date is required (YYYY-MM-DD)")
		}
		if strings.TrimSpace(d.LicenceNumber) == "" {
			ve.add(f+".licence_number", "required", "licence_number is required")
		}
		if strings.TrimSpace(d.LicenceIssuedOn) == "" {
			ve.add(f+".licence_issued_on", "required", "licence_issued_on is required (YYYY-MM-DD)")
		}
	}
	if utf8.RuneCountInString(in.Note) > maxNote {
		ve.add("note", "too_long", "note is at most 1000 characters")
	}
	if ext := strings.TrimSpace(in.ExternalReference); ext != "" {
		switch {
		case c.Browser():
			ve.add("external_reference", "not_allowed", "external_reference is for server integrations")
		case len(ext) > maxExternalRef:
			ve.add("external_reference", "too_long", "external_reference is at most 64 characters")
		}
	}
	return ve.orNil()
}

func optional(v string) customermodel.OptionalString {
	v = strings.TrimSpace(v)
	if v == "" {
		return customermodel.OptionalString{}
	}
	return customermodel.String(v)
}

func strPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

// channelLabel names the source of a public request in the staff views.
func channelLabel(channel string) string {
	switch channel {
	case "web":
		return "Web sitesi"
	case "widget":
		return "Rezervasyon widget'ı"
	default:
		return "API"
	}
}

// CreateReservation books a quote the public API made: the renter and the
// drivers become a new customer card (never matched to an existing one),
// then the reservation is created through the same use case as the panel
// (quote expiry, amount and capacity re-validated server-side, drivers
// checked against the sold product's conditions, the channel's approval
// protocol). A failure removes the new customer again. The answer carries
// the reservation access token once.
func (s *Service) CreateReservation(ctx context.Context, c model.Credential, in model.ReservationInput) (model.Booked, error) {
	if err := validateReservation(c, in); err != nil {
		return model.Booked{}, err
	}
	pq, err := s.q.GetPublicQuote(ctx, db.GetPublicQuoteParams{OrganizationID: c.OrganizationID, QuoteUuid: *in.QuoteUUID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (pq.Channel != c.Channel || pq.QuotePurpose != resmodel.PurposeNew)) {
		return model.Booked{}, validation("quote_uuid", "not_found", "quote was not found")
	}
	if err != nil {
		return model.Booked{}, err
	}
	octx := credentialContext(ctx, c)
	p := in.Customer
	if p.Type == "" {
		p.Type = customermodel.TypeIndividual
	}
	cin := customermodel.CustomerInput{
		CustomerType: &p.Type, Phone: strPtr(p.Phone), Email: strPtr(p.Email), Nationality: optional(p.Nationality),
	}
	if p.Type == customermodel.TypeCorporate {
		cin.CompanyTitle, cin.TaxOffice = strPtr(p.CompanyTitle), strPtr(p.TaxOffice)
		cin.FirstName, cin.LastName = strPtr(p.FirstName), strPtr(p.LastName)
	} else {
		cin.FirstName, cin.LastName = strPtr(p.FirstName), strPtr(p.LastName)
	}
	note := "Online rezervasyon (" + channelLabel(c.Channel) + ")"
	cin.Note = &note
	customer, err := s.customers.Create(octx, c.OrganizationID, cin)
	if err != nil {
		return model.Booked{}, customerFieldErr(err, "customer")
	}
	booked := false
	defer func() {
		if !booked {
			if _, err := s.q.DeleteUnusedPublicCustomer(context.WithoutCancel(ctx), db.DeleteUnusedPublicCustomerParams{
				OrganizationID: c.OrganizationID, Uuid: customer.UUID,
			}); err != nil {
				s.log.Warn("public_api_customer_undo_failed", "error", err)
			}
		}
	}()
	drivers := make([]resmodel.DriverInput, 0, len(in.Drivers))
	for i, d := range in.Drivers {
		din := customermodel.DriverInput{
			CopyFromCustomer: d.SameAsCustomer, BirthDate: optional(d.BirthDate), LicenceNumber: strPtr(d.LicenceNumber),
			LicenceClass: strPtr(d.LicenceClass), LicenceIssuedOn: optional(d.LicenceIssuedOn),
			LicenceExpiresOn: optional(d.LicenceExpiresOn), LicenceCountry: optional(d.LicenceCountry),
		}
		if !d.SameAsCustomer {
			din.FirstName, din.LastName = strPtr(d.FirstName), strPtr(d.LastName)
			din.Phone, din.Email = strPtr(d.Phone), strPtr(d.Email)
		}
		ch, err := s.customers.CreateDriver(octx, c.OrganizationID, customer.UUID, din)
		if err != nil {
			return model.Booked{}, customerFieldErr(err, "drivers."+itoa(i))
		}
		drivers = append(drivers, resmodel.DriverInput{DriverUUID: ch.Driver.UUID})
	}
	r, err := s.reservations.Create(octx, c.OrganizationID, resmodel.CreateInput{
		QuoteUUID: in.QuoteUUID, CustomerUUID: &customer.UUID, Drivers: drivers,
		ExternalReference: strings.TrimSpace(in.ExternalReference), Note: strings.TrimSpace(in.Note),
	})
	if err != nil {
		return model.Booked{}, s.reservationErr(ctx, c, err)
	}
	token, err := randomToken(accessTokenBytes)
	if err != nil {
		return model.Booked{}, err
	}
	if err := s.q.InsertPublicReservation(ctx, db.InsertPublicReservationParams{
		OrganizationID: c.OrganizationID, ReservationUuid: r.UUID, KeyID: keyIDArg(c), Channel: c.Channel,
		AccessTokenHash: TokenHash(token),
	}); err != nil {
		// The reservation exists; it stays (staff see it), but the caller
		// gets no token for it.
		booked = true
		return model.Booked{}, err
	}
	booked = true
	s.Emit(ctx, c.OrganizationID, model.EventReservationCreated, r.UUID)
	view, err := s.reservationView(ctx, c, r.Reference)
	if err != nil {
		return model.Booked{}, err
	}
	return model.Booked{Reference: r.Reference, Status: r.Status, AccessToken: token, Reservation: view}, nil
}

// reservationErr maps a create failure: a changed price comes back with
// the replacement quote, re-issued as a public quote.
func (s *Service) reservationErr(ctx context.Context, c model.Credential, err error) error {
	var qe *resusecase.QuoteError
	if errors.As(err, &qe) && qe.Replacement != nil {
		if ierr := s.q.InsertPublicQuote(ctx, db.InsertPublicQuoteParams{
			OrganizationID: c.OrganizationID, QuoteUuid: qe.Replacement.UUID, KeyID: keyIDArg(c), Channel: c.Channel,
		}); ierr != nil {
			return ierr
		}
		return &QuoteReplacedError{Err: qe, Replacement: publicQuote(*qe.Replacement)}
	}
	return publicFieldErr(err)
}

// keyLineage is the key and the keys it was rotated from.
func (s *Service) keyLineage(ctx context.Context, c model.Credential) ([]int64, error) {
	return s.q.PublicAPIKeyLineage(ctx, db.PublicAPIKeyLineageParams{OrganizationID: c.OrganizationID, ID: c.KeyID})
}

// authorizeReservation finds a public reservation by reference for the
// caller. A booking session always needs the reservation access token; a
// secret key needs it unless the reservation was made with that key (or a
// key it was rotated from). Every failure reads as not found.
func (s *Service) authorizeReservation(ctx context.Context, c model.Credential, reference, token string) (portalusecase.Principal, uuid.UUID, error) {
	ref := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(reference), " ", ""))
	if ref == "" || len(ref) > 64 {
		return portalusecase.Principal{}, uuid.Nil, ErrReservationToken
	}
	row, err := s.q.GetPublicReservationByReference(ctx, db.GetPublicReservationByReferenceParams{OrganizationID: c.OrganizationID, Reference: ref})
	if errors.Is(err, pgx.ErrNoRows) {
		return portalusecase.Principal{}, uuid.Nil, ErrReservationToken
	}
	if err != nil {
		return portalusecase.Principal{}, uuid.Nil, err
	}
	token = strings.TrimSpace(token)
	allowed := false
	if token != "" {
		allowed = subtle.ConstantTimeCompare([]byte(TokenHash(token)), []byte(row.AccessTokenHash)) == 1
	} else if !c.Browser() && row.KeyID.Valid {
		lineage, err := s.keyLineage(ctx, c)
		if err != nil {
			return portalusecase.Principal{}, uuid.Nil, err
		}
		for _, id := range lineage {
			if id == row.KeyID.Int64 {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return portalusecase.Principal{}, uuid.Nil, ErrReservationToken
	}
	p, id, err := s.portal.PrincipalForReference(ctx, c.OrganizationSlug, row.Reference, channelLabel(c.Channel))
	if errors.Is(err, portalusecase.ErrNotFound) || errors.Is(err, portalusecase.ErrPortalNotFound) {
		return portalusecase.Principal{}, uuid.Nil, ErrReservationToken
	}
	return p, id, err
}

func publicRequests(list []portalmodel.Request) []model.Request {
	out := make([]model.Request, 0, len(list))
	for _, r := range list {
		out = append(out, model.Request{
			UUID: r.UUID, Type: r.Type, Status: r.Status, Message: r.Message, RequestedStartsAt: r.RequestedStartsAt,
			RequestedEndsAt: r.RequestedEndsAt, PolicyText: r.PolicyText, CancellationFee: r.CancellationFee, Outcome: r.Outcome,
			ResolutionNote: r.ResolutionNote, ResolvedAt: r.ResolvedAt, CreatedAt: r.CreatedAt,
		})
	}
	return out
}

// publicReservation maps the customer portal's view (documents and
// invoices stay in the portal, behind its own sign-in).
func publicReservation(r portalmodel.Reservation) model.Reservation {
	out := model.Reservation{
		Reference: r.Reference, Status: r.Status, PaymentStatus: r.PaymentStatus, StartsAt: r.StartsAt, EndsAt: r.EndsAt,
		PickupBranch: r.PickupBranch, ReturnBranch: r.ReturnBranch, PickupPoint: r.PickupPoint, ReturnPoint: r.ReturnPoint,
		Subgroup: r.Product, Vehicle: r.Vehicle, Drivers: r.Drivers, Extras: []model.ReservationExtra{},
		Price: model.Money{
			Currency: r.Price.Currency, Total: r.Price.Total, Prepayment: r.Price.Prepayment, Paid: r.Price.Paid,
			PrepaymentDue: r.Price.PrepaymentDue, Balance: r.Price.Balance,
		},
		PaymentDeadline: r.PaymentDeadline,
		Policies: model.Policies{
			Cancellation: r.Policies.Cancellation, NoShow: r.Policies.NoShow, EarlyReturn: r.Policies.EarlyReturn,
			Extension: r.Policies.Extension, Fuel: r.Policies.Fuel, Mileage: r.Policies.Mileage,
		},
		Cancellation: model.Cancellation{PolicyText: r.Cancellation.PolicyText, Fee: r.Cancellation.Fee, SelfCancel: r.Cancellation.SelfCancel},
		Actions: model.Actions{
			Pay: r.Actions.Pay, PayAmount: r.Actions.PayAmount, RequestChange: r.Actions.RequestChange,
			RequestCancellation: r.Actions.RequestCancel,
		},
		Requests: publicRequests(r.Requests),
	}
	if out.Drivers == nil {
		out.Drivers = []string{}
	}
	for _, e := range r.Extras {
		out.Extras = append(out.Extras, model.ReservationExtra{Name: e.Name, Quantity: e.Quantity, Amount: e.Amount})
	}
	return out
}

// reservationView is the public view of a reservation the caller is
// already allowed to see (right after booking).
func (s *Service) reservationView(ctx context.Context, c model.Credential, reference string) (model.Reservation, error) {
	p, id, err := s.portal.PrincipalForReference(ctx, c.OrganizationSlug, reference, channelLabel(c.Channel))
	if err != nil {
		return model.Reservation{}, err
	}
	r, err := s.portal.Reservation(ctx, p, id)
	if err != nil {
		return model.Reservation{}, err
	}
	return publicReservation(r), nil
}

// Reservation is the status of a public reservation.
func (s *Service) Reservation(ctx context.Context, c model.Credential, reference, token string) (model.Reservation, error) {
	p, id, err := s.authorizeReservation(ctx, c, reference, token)
	if err != nil {
		return model.Reservation{}, err
	}
	r, err := s.portal.Reservation(ctx, p, id)
	if errors.Is(err, portalusecase.ErrNotFound) {
		return model.Reservation{}, ErrReservationToken
	}
	if err != nil {
		return model.Reservation{}, err
	}
	return publicReservation(r), nil
}

func publicRequest(r portalmodel.Request) model.Request {
	return publicRequests([]portalmodel.Request{r})[0]
}

// ChangeRequest asks staff for a change (dates and / or a message).
func (s *Service) ChangeRequest(ctx context.Context, c model.Credential, reference, token string, in model.ChangeRequestInput, client portalmodel.ClientInfo) (model.Request, error) {
	p, id, err := s.authorizeReservation(ctx, c, reference, token)
	if err != nil {
		return model.Request{}, err
	}
	if utf8.RuneCountInString(in.Message) > maxRequestNote {
		return model.Request{}, validation("message", "too_long", "at most 2000 characters")
	}
	r, err := s.portal.CreateRequest(ctx, p, id, portalmodel.CreateRequestInput{
		Type: portalmodel.RequestChange, Message: in.Message, RequestedStartsAt: in.RequestedStartsAt, RequestedEndsAt: in.RequestedEndsAt,
	}, client)
	if err != nil {
		return model.Request{}, err
	}
	return publicRequest(r), nil
}

// CancellationRequest asks for a cancellation: staff decide unless the
// reservation's policy snapshot allows self-cancellation (then it is
// cancelled at once and the request comes back resolved).
func (s *Service) CancellationRequest(ctx context.Context, c model.Credential, reference, token string, in model.CancellationRequestInput, client portalmodel.ClientInfo) (model.Request, error) {
	p, id, err := s.authorizeReservation(ctx, c, reference, token)
	if err != nil {
		return model.Request{}, err
	}
	if utf8.RuneCountInString(in.Message) > maxRequestNote {
		return model.Request{}, validation("message", "too_long", "at most 2000 characters")
	}
	r, err := s.portal.CreateRequest(ctx, p, id, portalmodel.CreateRequestInput{Type: portalmodel.RequestCancel, Message: in.Message}, client)
	if err != nil {
		return model.Request{}, err
	}
	return publicRequest(r), nil
}

// PaymentLink returns the hosted pay page of the amount due now (an active
// link for exactly that amount is reused).
func (s *Service) PaymentLink(ctx context.Context, c model.Credential, reference, token string, client portalmodel.ClientInfo) (model.PaymentLink, error) {
	p, id, err := s.authorizeReservation(ctx, c, reference, token)
	if err != nil {
		return model.PaymentLink{}, err
	}
	r, err := s.portal.Pay(ctx, p, id, client)
	if err != nil {
		return model.PaymentLink{}, err
	}
	return model.PaymentLink{URL: r.URL, Amount: r.Amount, Currency: r.Currency, ExpiresAt: r.ExpiresAt, Reused: r.Reused}, nil
}
