package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	customermodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// loadQuote returns a quote row in the caller's branch scope.
func (s *Service) loadQuote(ctx context.Context, orgID int64, id uuid.UUID) (db.GetQuoteRow, error) {
	row, err := s.q.GetQuote(ctx, db.GetQuoteParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, ErrQuoteNotFound
	}
	if err != nil {
		return row, err
	}
	if !inScope(ctx, row.PickupBranchID, row.ReturnBranchID) {
		return row, ErrQuoteNotFound
	}
	return row, nil
}

// specFromQuote rebuilds the priced input of a stored quote.
func specFromQuote(row db.GetQuoteRow, customer *db.GetReservationCustomerRow) quoteSpec {
	start, end := row.StartsAt.Time.UTC(), row.EndsAt.Time.UTC()
	pb, rb := row.PickupBranchUuid, row.ReturnBranchUuid
	w := availmodel.Where{PickupBranchUUID: &pb, ReturnBranchUUID: &rb, StartsAt: start, EndsAt: end, Channel: row.Channel}
	if row.PickupPointUuid.Valid {
		id := uuid.UUID(row.PickupPointUuid.Bytes)
		w.PickupPointUUID = &id
	}
	if row.ReturnPointUuid.Valid {
		id := uuid.UUID(row.ReturnPointUuid.Bytes)
		w.ReturnPointUUID = &id
	}
	spec := quoteSpec{purpose: row.Purpose, customer: customer, where: w, productUUID: row.ProductUuid, extras: []model.ExtraRequest{}}
	_ = json.Unmarshal(row.Extras, &spec.extras)
	if row.CouponCode.Valid {
		spec.coupon = row.CouponCode.String
	}
	if len(row.ManualDiscount) > 0 {
		var md model.ManualDiscountView
		if json.Unmarshal(row.ManualDiscount, &md) == nil {
			spec.manual = &model.ManualDiscountInput{Kind: md.Kind, Value: pricingDecimal(md.Value), Reason: md.Reason}
		}
	}
	spec.pickupDelivery, spec.returnDelivery = storedDelivery(row.PickupDelivery), storedDelivery(row.ReturnDelivery)
	spec.checkPickup, spec.checkReturn = true, true
	switch {
	case row.DeliveryOverride.Valid:
		spec.deliveryFee = money.FormatNumeric(row.DeliveryOverride)
	case !row.PickupPointID.Valid && !row.ReturnPointID.Valid && !money.FromNumericOrZero(row.DeliveryFee).IsZero():
		// A staff fee of a quote without delivery points (M2 quotes).
		spec.deliveryFee = money.FormatNumeric(row.DeliveryFee)
	}
	return spec
}

// revalidate re-prices a new-sale quote server-side: still open, not
// expired, same product version and same total now. A changed price
// supersedes the quote with a fresh one (QUOTE_PRICE_CHANGED carries it).
func (s *Service) revalidate(ctx context.Context, orgID int64, row db.GetQuoteRow, customer *db.GetReservationCustomerRow) (priced, quoteSpec, error) {
	switch {
	case row.Status != model.QuoteOpen:
		return priced{}, quoteSpec{}, &QuoteError{Code: "QUOTE_USED", Message: "the quote was already used"}
	case !row.ValidUntil.Time.After(s.clock()):
		return priced{}, quoteSpec{}, &QuoteError{Code: "QUOTE_EXPIRED", Message: "the quote has expired; create a new one"}
	}
	spec := specFromQuote(row, customer)
	ag, err := s.agreementByID(ctx, orgID, row.AgreementID)
	if err != nil {
		return priced{}, spec, err
	}
	spec.agreement = ag
	p, err := s.price(ctx, orgID, spec)
	if err != nil {
		return p, spec, err
	}
	if p.item.Product.Version == row.ProductVersion && p.side.Breakdown.Total.Equal(money.FromNumericOrZero(row.Total)) {
		return p, spec, nil
	}
	id, err := s.storeQuote(ctx, orgID, spec, p, storeExtra{})
	if err != nil {
		return p, spec, err
	}
	s.q.MarkQuoteSuperseded(ctx, db.MarkQuoteSupersededParams{OrganizationID: orgID, ID: row.ID}) //nolint:errcheck // best effort
	fresh, err := s.GetQuote(ctx, orgID, id)
	if err != nil {
		return p, spec, err
	}
	return p, spec, &QuoteError{Code: "QUOTE_PRICE_CHANGED", Message: "the price changed since the quote; review the new quote", Replacement: &fresh}
}

// checkedDriver is a driver with its eligibility.
type checkedDriver struct {
	row  db.GetReservationDriverRow
	role string
	elig customermodel.Eligibility
}

// checkDrivers resolves the drivers and checks them against the product
// version's conditions on the pick-up day. Refused drivers block unless
// override is given with tenant.reservations.override_eligibility.
func (s *Service) checkDrivers(ctx context.Context, orgID int64, in []model.DriverInput, c *model.Conditions, at time.Time, override *model.OverrideInput) ([]checkedDriver, error) {
	ve := &ValidationError{}
	out := []checkedDriver{}
	seen := map[uuid.UUID]bool{}
	primary := 0
	if len(in) > 10 {
		return nil, validation("drivers", "too_many", "at most 10 drivers")
	}
	for i, d := range in {
		field := "drivers." + itoa(i)
		role := d.Role
		if role == "" {
			role = model.DriverPrimary
			if i > 0 {
				role = model.DriverAdditional
			}
		}
		if role != model.DriverPrimary && role != model.DriverAdditional {
			ve.add(field+".role", "invalid", "role must be primary or additional")
			continue
		}
		if role == model.DriverPrimary {
			primary++
		}
		if seen[d.DriverUUID] {
			ve.add(field+".driver_uuid", "duplicate", "the driver is listed twice")
			continue
		}
		seen[d.DriverUUID] = true
		row, err := s.q.GetReservationDriver(ctx, db.GetReservationDriverParams{OrganizationID: orgID, Uuid: d.DriverUUID})
		if notFound(err) {
			ve.add(field+".driver_uuid", "not_found", "driver was not found")
			continue
		}
		if err != nil {
			return nil, err
		}
		facts := customerusecase.DriverFacts(db.Driver{
			BirthDate: row.BirthDate, LicenceEnc: row.LicenceEnc, LicenceIssuedOn: row.LicenceIssuedOn, LicenceExpiresOn: row.LicenceExpiresOn,
		})
		elig := customermodel.EligibleForConditions(facts, customermodel.Conditions{MinDriverAge: c.MinDriverAge, MinLicenseYears: c.MinLicenseYears}, at)
		out = append(out, checkedDriver{row: row, role: role, elig: elig})
	}
	if primary > 1 {
		ve.add("drivers", "primary_count", "one primary driver at most")
	}
	if err := ve.orNil(); err != nil {
		return nil, err
	}
	refused := []model.Eligibility{}
	for _, d := range out {
		if !d.elig.Eligible {
			refused = append(refused, model.Eligibility{
				DriverUUID: d.row.Uuid, Name: strings.TrimSpace(d.row.FirstName + " " + d.row.LastName), Eligible: false, Reasons: d.elig.Reasons,
			})
		}
	}
	if len(refused) == 0 {
		return out, nil
	}
	if override == nil {
		return nil, &EligibilityError{Drivers: refused}
	}
	if !has(ctx, rbac.PermTenantReservationsOverrideEligibility) {
		return nil, ErrForbidden
	}
	if r := strings.TrimSpace(override.Reason); len(r) < 3 || len(r) > 500 {
		return nil, validation("eligibility_override.reason", "required", "an override needs a reason (3-500 characters)")
	}
	return out, nil
}

// writeDrivers stores the checked drivers of a reservation.
func writeDrivers(ctx context.Context, q *db.Queries, orgID, reservationID int64, drivers []checkedDriver, override *model.OverrideInput) error {
	for _, d := range drivers {
		reason := ""
		var by pgtype.Int8
		if !d.elig.Eligible && override != nil {
			reason = strings.TrimSpace(override.Reason)
			by = int8Ptr(actorID(ctx))
		}
		if err := q.InsertReservationDriver(ctx, db.InsertReservationDriverParams{
			OrganizationID: orgID, ReservationID: reservationID, DriverID: int8Of(d.row.ID), Role: d.role,
			Snapshot: mustJSON(map[string]any{
				"name": strings.TrimSpace(d.row.FirstName + " " + d.row.LastName), "licence_masked": d.row.LicenceHint,
			}),
			Eligible: d.elig.Eligible, Reasons: d.elig.Reasons, OverrideReason: reason, OverriddenBy: by,
		}); err != nil {
			return err
		}
	}
	return nil
}

func overrideJSON(drivers []checkedDriver, override *model.OverrideInput, actor *int64) []byte {
	if override == nil {
		return nil
	}
	refused := []map[string]any{}
	for _, d := range drivers {
		if !d.elig.Eligible {
			refused = append(refused, map[string]any{"driver_uuid": d.row.Uuid, "reasons": d.elig.Reasons})
		}
	}
	if len(refused) == 0 {
		return nil
	}
	return mustJSON(map[string]any{"reason": strings.TrimSpace(override.Reason), "drivers": refused, "actor_id": actor})
}

// waitingStatus is the status a new reservation starts in and the approval
// protocol it is created under (TEC-708, docs/design/payments.md §5). The
// channel's protocol decides (settings.ChannelProtocol):
//
//   - channel: the sales channel confirmed the booking → confirmed (firm),
//     whatever the organization's online approval mode;
//   - staff, online channel: a request without stock ("değerlendirme
//     bekliyor") with the review window as the staff SLA; staff accept
//     reserves current capacity (Accept);
//   - staff, the branch counter: awaiting_review on an option that lasts
//     the review window (the employee selling is at the desk);
//   - auto: awaiting_payment on an option for the payment hold window when
//     a prepayment is due, else confirmed.
func (s *Service) waitingStatus(settings osmodel.Settings, prepayment money.Money, channel string) (string, string, *time.Time, *time.Time) {
	now := s.clock()
	r := settings.Reservation
	switch settings.ChannelProtocol(channel) {
	case osmodel.ProtocolChannel:
		return model.StatusConfirmed, model.ProtocolChannel, nil, nil
	case osmodel.ProtocolStaff:
		d := now.Add(time.Duration(minutesOr(r.StaffReviewWindowMinutes, model.DefaultReviewMinutes)) * time.Minute)
		if osmodel.IsOnlineChannel(channel) {
			return model.StatusRequest, model.ProtocolStaff, &d, nil
		}
		return model.StatusAwaitingReview, model.ProtocolStaff, &d, nil
	}
	if prepayment.IsPositive() {
		d := now.Add(time.Duration(minutesOr(r.PaymentHoldMinutes, model.DefaultPaymentMinutes)) * time.Minute)
		return model.StatusAwaitingPayment, model.ProtocolAuto, nil, &d
	}
	return model.StatusConfirmed, model.ProtocolAuto, nil, nil
}

// policySnapshot is the policy part of the settings version a reservation
// is sold under.
func policySnapshot(v osmodel.Version) map[string]any {
	return map[string]any{
		"settings_version":            v.Version,
		"approval_mode":               v.Settings.Reservation.ApprovalMode,
		"staff_review_window_minutes": v.Settings.Reservation.StaffReviewWindowMinutes,
		"payment_hold_minutes":        v.Settings.Reservation.PaymentHoldMinutes,
		"quote_validity_minutes":      v.Settings.Reservation.QuoteValidityMinutes,
		"prepayment_rule":             v.Settings.Payment.PrepaymentRule,
		"prepayment_value":            v.Settings.Payment.PrepaymentValue,
		"policies":                    v.Settings.Policies,
	}
}

// priceSnapshot builds the reservation's price record from a quote.
func priceSnapshot(q model.Quote) model.PriceSnapshot {
	return model.PriceSnapshot{
		QuoteUUID: q.UUID, Purpose: q.Purpose, Currency: q.Currency, Pricing: q.Breakdown.Pricing, Lines: q.Lines,
		Extras: q.Extras, CouponCode: q.CouponCode, ManualDiscount: q.ManualDiscount, Extensions: []model.Extension{},
		BaseTotal: q.BaseTotal, VATTotal: q.VATTotal, Total: q.Total, Prepayment: q.Prepayment, SettingsVersion: q.SettingsVersion,
	}
}

// Create turns a quote into a reservation. The quote is re-validated
// server-side (expiry, amount, product version, capacity); drivers are
// checked against the sold version's conditions; the breakdown and the
// policy version are copied as an immutable snapshot. Capacity: an option
// (hold) for awaiting_review / awaiting_payment, a firm commitment for
// confirmed, nothing for a request; the allocation and the reservation row
// are written in one transaction.
func (s *Service) Create(ctx context.Context, orgID int64, in model.CreateInput) (model.Reservation, error) {
	ve := &ValidationError{}
	if in.QuoteUUID == nil {
		ve.add("quote_uuid", "required", "quote_uuid is required")
	}
	collecting := in.CollectingParty
	if collecting == "" {
		collecting = model.CollectorUs
	}
	if collecting != model.CollectorUs && collecting != model.CollectorChannel {
		ve.add("collecting_party", "invalid", "collecting_party must be us or channel")
	}
	commission := money.Zero()
	if raw := strings.TrimSpace(string(in.CommissionAmount)); raw != "" {
		if m, err := money.ParseRound(raw); err != nil || m.IsNegative() {
			ve.add("commission_amount", "invalid", "commission must be a non-negative amount")
		} else {
			commission = m
		}
	}
	external := strings.TrimSpace(in.ExternalReference)
	if len(external) > 64 {
		ve.add("external_reference", "too_long", "external_reference is at most 64 characters")
	}
	if len(in.Note) > 2000 {
		ve.add("note", "too_long", "note is at most 2000 characters")
	}
	if in.RequestOnly && in.ConfirmNow {
		ve.add("confirm_now", "not_allowed", "a request cannot be confirmed at once")
	}
	if err := ve.orNil(); err != nil {
		return model.Reservation{}, err
	}
	if in.ConfirmNow && !has(ctx, rbac.PermTenantReservationsConfirm) {
		return model.Reservation{}, ErrForbidden
	}
	row, err := s.loadQuote(ctx, orgID, *in.QuoteUUID)
	if errors.Is(err, ErrQuoteNotFound) {
		return model.Reservation{}, validation("quote_uuid", "not_found", "quote was not found")
	}
	if err != nil {
		return model.Reservation{}, err
	}
	if row.Purpose != model.PurposeNew {
		return model.Reservation{}, validation("quote_uuid", "wrong_purpose", "the quote belongs to a change of a reservation")
	}
	customerUUID := in.CustomerUUID
	if customerUUID == nil && row.CustomerUuid.Valid {
		id := uuid.UUID(row.CustomerUuid.Bytes)
		customerUUID = &id
	}
	if customerUUID == nil {
		return model.Reservation{}, validation("customer_uuid", "required", "customer_uuid is required")
	}
	customer, err := s.customerOf(ctx, orgID, customerUUID, "customer_uuid")
	if err != nil {
		return model.Reservation{}, err
	}
	invoice, err := s.customerOf(ctx, orgID, in.InvoiceCustomerUUID, "invoice_customer_uuid")
	if err != nil {
		return model.Reservation{}, err
	}
	// A sale under a corporate agreement (TEC-723): still active and valid
	// on the pick-up day, sold to (or invoiced to) its customer.
	var ag *agreement
	if row.AgreementUuid.Valid {
		if ag, err = s.agreementForSale(ctx, orgID, row.AgreementUuid.Bytes, row.StartsAt.Time, "quote_uuid"); err != nil {
			return model.Reservation{}, err
		}
		if err := s.checkAgreementParties(ctx, orgID, ag, customer, &invoice); err != nil {
			return model.Reservation{}, err
		}
	}
	p, spec, err := s.revalidate(ctx, orgID, row, customer)
	if err != nil {
		return model.Reservation{}, err
	}
	conds, err := s.conditionsByID(ctx, orgID, row.RentalProductID, row.ProductVersion)
	if err != nil {
		return model.Reservation{}, err
	}
	drivers, err := s.checkDrivers(ctx, orgID, in.Drivers, conds, row.StartsAt.Time, in.EligibilityOverride)
	if err != nil {
		return model.Reservation{}, err
	}
	if ag != nil {
		if err := s.checkAgreementDrivers(ctx, orgID, ag, drivers); err != nil {
			return model.Reservation{}, err
		}
	}
	quote, err := s.GetQuote(ctx, orgID, row.Uuid)
	if err != nil {
		return model.Reservation{}, err
	}
	settings, err := s.settingsOf(ctx, orgID)
	if err != nil {
		return model.Reservation{}, err
	}
	status, protocol, reviewDeadline, payDeadline := s.waitingStatus(settings.Settings, quote.Prepayment, row.Channel)
	// Open account (TEC-723): the agreement lets the sale be confirmed
	// without prepayment while the customer's exposure stays within the
	// credit limit; above it the confirmation is refused.
	openAccount := ag != nil && ag.OpenAccount && !in.RequestOnly && (in.UseOpenAccount == nil || *in.UseOpenAccount)
	prepayment := quote.Prepayment
	if openAccount {
		if err := s.checkCredit(ctx, nil, orgID, ag, quote.Total, 0); err != nil {
			return model.Reservation{}, err
		}
		status, reviewDeadline, payDeadline = model.StatusConfirmed, nil, nil
		prepayment = money.NewIn(0, prepayment.Currency())
	}
	switch {
	case in.RequestOnly:
		status, reviewDeadline, payDeadline = model.StatusRequest, nil, nil
	case in.ConfirmNow:
		status, reviewDeadline, payDeadline = model.StatusConfirmed, nil, nil
	}
	resUUID := uuid.New()
	total := quote.Total
	// Net receivable: what the organization keeps after the channel's
	// commission (whoever collects; the collecting party decides who owes
	// whom, M3 settlements).
	net := total.Sub(commission)
	params := db.InsertReservationParams{
		Uuid: resUUID, OrganizationID: orgID, Status: status, CapacityCommitment: model.CommitmentOf(status),
		PickupBranchID: row.PickupBranchID, ReturnBranchID: row.ReturnBranchID, PickupPointID: row.PickupPointID,
		ReturnPointID: row.ReturnPointID, StartsAt: row.StartsAt, EndsAt: row.EndsAt,
		RentalProductID: row.RentalProductID, ProductVersion: row.ProductVersion, GuaranteeType: conds.GuaranteeType,
		ProductSnapshot: mustJSON(conds), CustomerID: customer.ID, Channel: row.Channel, ExternalReference: textOf(external),
		CollectingParty: collecting, CommissionAmount: commission.Numeric(), NetReceivable: net.Numeric(), Currency: row.Currency,
		TotalAmount: total.Numeric(), PrepaymentAmount: prepayment.Numeric(), QuoteID: int8Of(row.ID),
		QuoteSnapshot: mustJSON(priceSnapshot(quote)), SettingsVersion: int32(settings.Version),
		PolicySnapshot: mustJSON(policySnapshot(settings)), ReviewDeadline: tsPtr(reviewDeadline), PaymentDeadline: tsPtr(payDeadline),
		Note: strings.TrimSpace(in.Note), EligibilityOverride: overrideJSON(drivers, in.EligibilityOverride, actorID(ctx)),
		CreatedBy: int8Ptr(actorID(ctx)), ApprovalProtocol: protocol,
		PickupDelivery: deliveryJSON(spec.pickupDelivery), ReturnDelivery: deliveryJSON(spec.returnDelivery),
	}
	if invoice != nil {
		params.InvoiceCustomerID = int8Of(invoice.ID)
	}
	if ag != nil {
		params.AgreementID, params.OpenAccount = int8Of(ag.ID), openAccount
	}
	if status == model.StatusConfirmed {
		params.ConfirmedAt = ts(s.clock())
	}
	// The campaign use is reserved before capacity (the reservation uuid
	// is known): a refused coupon writes nothing, a failed sale gives it
	// back.
	redeemed, err := s.redeemCoupon(ctx, orgID, quote, p, spec, resUUID)
	if err != nil {
		return model.Reservation{}, err
	}
	var resID int64
	var allocUUID uuid.UUID
	write := func(ctx context.Context, q *db.Queries, r *availmodel.HookResult) error {
		ref, err := s.nextReference(ctx, q, orgID, settings.Settings)
		if err != nil {
			return err
		}
		params.Reference = ref
		if r != nil {
			params.AllocationID = int8Of(r.AllocationID)
		}
		id, err := q.InsertReservation(ctx, params)
		if err != nil {
			if uniqueViolation(err, "uq_reservations_organization_channel_external") {
				return ErrExternalReferenceTaken
			}
			return err
		}
		resID = id
		n, err := q.MarkQuoteUsed(ctx, db.MarkQuoteUsedParams{OrganizationID: orgID, ID: row.ID, UsedAt: ts(s.clock())})
		if err != nil {
			return err
		}
		if n != 1 {
			return &QuoteError{Code: "QUOTE_USED", Message: "the quote was already used"}
		}
		if err := writeDrivers(ctx, q, orgID, id, drivers, in.EligibilityOverride); err != nil {
			return err
		}
		data := map[string]any{"quote_uuid": row.Uuid, "total": total.String(), "channel": row.Channel}
		if params.EligibilityOverride != nil {
			data["eligibility_override"] = strings.TrimSpace(in.EligibilityOverride.Reason)
		}
		if in.ConfirmNow {
			data["confirmed_by_staff"] = true
		}
		if ag != nil {
			data["agreement"] = ag.Code
			if openAccount {
				data["open_account"] = true
				// Re-checked under the transaction: two open-account sales
				// racing for the last of the credit limit cannot both pass.
				if _, err := q.LockAgreement(ctx, db.LockAgreementParams{OrganizationID: orgID, ID: ag.ID}); err != nil {
					return err
				}
				if err := s.checkCredit(ctx, q, orgID, ag, total, id); err != nil {
					return err
				}
			}
		}
		if err := event(ctx, q, orgID, id, EventCreated, "", status, "", data); err != nil {
			return err
		}
		if r != nil {
			allocUUID = r.AllocationUUID
			if err := assignment(ctx, q, orgID, id, r.VehicleID, r.ProductID, r.ProductVersion, r.PlateFixed, model.SourceEngine, ""); err != nil {
				return err
			}
			if _, err := s.planEffects(ctx, q, orgID, *r, "reservation", resUUID); err != nil {
				return err
			}
		}
		return nil
	}
	if status == model.StatusRequest {
		err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error { return write(ctx, q, nil) })
	} else {
		kind := availmodel.KindHold
		var holdUntil *time.Time
		switch status {
		case model.StatusConfirmed:
			kind = availmodel.KindReservation
		case model.StatusAwaitingReview:
			holdUntil = reviewDeadline
		case model.StatusAwaitingPayment:
			holdUntil = payDeadline
		}
		_, err = s.avail.Reserve(ctx, orgID, availmodel.ReserveInput{
			Where: spec.where, ProductUUID: row.ProductUuid, Kind: kind, HoldUntil: holdUntil,
			RefType: model.RefType, RefUUID: &resUUID, ActorID: actorID(ctx), Note: "reservation",
			Hook: hookOf(func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
				if r.ProductVersion != row.ProductVersion {
					return &QuoteError{Code: "QUOTE_PRICE_CHANGED", Message: "the product changed since the quote"}
				}
				return write(ctx, q, &r)
			}, s),
		})
	}
	if err != nil {
		if redeemed {
			s.releaseCoupon(ctx, orgID, resUUID, "create_failed")
		}
		return model.Reservation{}, mapEngineErr(err)
	}
	if allocUUID != uuid.Nil {
		if err := s.reserveEquipment(ctx, orgID, resID, quote.Extras, allocUUID, resUUID); err != nil {
			s.undoCreate(ctx, orgID, resID, allocUUID, row.ID, resUUID, redeemed)
			return model.Reservation{}, err
		}
	}
	s.publish(ctx, model.ResourceReservations, "created", resUUID)
	s.publish(ctx, model.ResourceQuotes, "updated", row.Uuid)
	if status == model.StatusConfirmed {
		// Confirmed at once: the listeners (rentals, field tasks) hear it
		// like any other confirmation.
		s.notify(ctx, orgID, resUUID, LifecycleConfirmed)
	}
	return s.Get(ctx, orgID, resUUID)
}

// redeemCoupon reserves the campaign the quote applied (coupon code or an
// automatic repeat-customer campaign). Returns whether a use was taken.
func (s *Service) redeemCoupon(ctx context.Context, orgID int64, q model.Quote, p priced, spec quoteSpec, resUUID uuid.UUID) (bool, error) {
	b := q.Breakdown.Pricing
	if b == nil || b.Discount == nil || b.Discount.CampaignID == "" {
		return false, nil
	}
	campaign, err := uuid.Parse(b.Discount.CampaignID)
	if err != nil {
		return false, nil
	}
	product, err := s.q.GetReservationProductByUUID(ctx, db.GetReservationProductByUUIDParams{OrganizationID: orgID, Uuid: q.Product.UUID})
	if err != nil {
		return false, err
	}
	in := model.CouponRedemption{
		Code: spec.coupon, CampaignUUID: &campaign, ProductID: product.ID, SegmentID: product.SegmentID, Channel: q.Channel,
		Subtotal: b.RentalSubtotal, ReservationUUID: resUUID, ActorID: actorID(ctx), Corporate: spec.agreement != nil,
	}
	if spec.customer != nil {
		id := spec.customer.Uuid
		in.CustomerUUID = &id
	}
	if err := s.coupons.Redeem(ctx, orgID, in); err != nil {
		var ce *model.CouponError
		if errors.As(err, &ce) {
			return false, &ValidationError{Details: []Detail{{Field: "coupon_code", Code: "coupon_" + ce.Reason, Message: "the coupon can no longer be used: " + ce.Reason}}}
		}
		if errors.Is(err, model.ErrCouponUnavailable) {
			return false, validation("coupon_code", "coupon_unavailable", err.Error())
		}
		return false, err
	}
	_ = p
	return true, nil
}

// releaseCoupon gives a campaign use back (best effort, logged).
func (s *Service) releaseCoupon(ctx context.Context, orgID int64, resUUID uuid.UUID, reason string) {
	if err := s.coupons.Release(ctx, orgID, resUUID, reason); err != nil {
		slog.Default().Warn("reservation_coupon_release_failed", "reservation", resUUID, "error", err)
	}
}

// reserveEquipment holds the physical extras of a reservation, tied to its
// vehicle allocation (the extras trigger follows the vehicle allocation
// afterwards: confirm, hold expiry, release).
func (s *Service) reserveEquipment(ctx context.Context, orgID, resID int64, extras []model.ExtraRequest, allocUUID, resUUID uuid.UUID) error {
	taken := []uuid.UUID{}
	for _, e := range extras {
		id, err := s.equipment.ReserveEquipment(ctx, orgID, model.EquipmentRequest{
			ExtraUUID: e.ExtraUUID, Quantity: max(e.Quantity, 1), VehicleAllocationUUID: allocUUID, ReservationUUID: resUUID, ActorID: actorID(ctx),
		})
		if err != nil {
			for _, t := range taken {
				_ = s.equipment.ReleaseEquipment(ctx, orgID, t, "reservation_failed")
			}
			return err
		}
		if id != uuid.Nil {
			taken = append(taken, id)
		}
	}
	return s.q.SetReservationEquipment(ctx, db.SetReservationEquipmentParams{OrganizationID: orgID, ID: resID, EquipmentAllocations: taken})
}

// resyncEquipment re-holds the equipment of a reservation for its new
// allocation / range after a change (the old holds are released first).
// Best effort after the vehicle change committed: a shortage is returned
// as a warning and recorded on the timeline.
func (s *Service) resyncEquipment(ctx context.Context, orgID, resID int64, old []uuid.UUID, extras []model.ExtraRequest, allocUUID, resUUID uuid.UUID) []string {
	for _, id := range old {
		_ = s.equipment.ReleaseEquipment(ctx, orgID, id, "reservation_changed")
	}
	if len(extras) == 0 {
		_ = s.q.SetReservationEquipment(ctx, db.SetReservationEquipmentParams{OrganizationID: orgID, ID: resID, EquipmentAllocations: []uuid.UUID{}})
		return nil
	}
	if err := s.reserveEquipment(ctx, orgID, resID, extras, allocUUID, resUUID); err != nil {
		_ = s.q.SetReservationEquipment(ctx, db.SetReservationEquipmentParams{OrganizationID: orgID, ID: resID, EquipmentAllocations: []uuid.UUID{}})
		_ = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
			return event(ctx, q, orgID, resID, EventEquipmentWarning, "", "", "", map[string]any{"error": err.Error()})
		})
		return []string{"equipment_unavailable"}
	}
	return nil
}

// undoCreate compensates a create whose equipment could not be held: the
// vehicle allocation is released (its equipment follows) and the
// reservation row removed in the same engine transaction; the quote is
// usable again and the campaign use is given back.
func (s *Service) undoCreate(ctx context.Context, orgID, resID int64, allocUUID uuid.UUID, quoteID int64, resUUID uuid.UUID, redeemed bool) {
	_, err := s.avail.ReleaseWith(unscoped(ctx), orgID, allocUUID, "reservation_failed", actorID(ctx), hookOf(func(ctx context.Context, q *db.Queries, _ availmodel.HookResult) error {
		if _, err := q.DeleteReservation(ctx, db.DeleteReservationParams{OrganizationID: orgID, ID: resID}); err != nil {
			return err
		}
		return q.ReopenQuote(ctx, db.ReopenQuoteParams{OrganizationID: orgID, ID: quoteID})
	}, s))
	if err != nil {
		slog.Default().Error("reservation_undo_failed", "reservation", resUUID, "error", err)
	}
	if redeemed {
		s.releaseCoupon(ctx, orgID, resUUID, "create_failed")
	}
}
