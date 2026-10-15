package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	pricingmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Quotes (TEC-704): a priced, stored search answer for one product. It
// consumes no capacity; the availability answer is informative and is
// re-checked when the reservation is created.

// quoteSpec is everything a quote is priced from.
type quoteSpec struct {
	purpose       string
	reservationID int64
	customer      *db.GetReservationCustomerRow
	where         availmodel.Where
	productUUID   uuid.UUID
	extras        []model.ExtraRequest
	coupon        string
	manual        *model.ManualDiscountInput
	deliveryFee   string
}

// priced is the outcome of pricing a spec.
type priced struct {
	search   availmodel.SearchResult
	item     availmodel.ProductAvailability
	side     pricingmodel.PreviewSide
	warnings []string
	settings int32
}

// pricingFieldMap renames pricing preview fields to the quote body's.
var pricingFieldMap = map[string]string{
	"start_at": "starts_at", "end_at": "ends_at", "branch_uuid": "pickup_branch_uuid",
	"return_branch_uuid": "return_branch_uuid", "rental_product_uuid": "product_uuid",
	"discount.value": "manual_discount.value", "discount.kind": "manual_discount.kind",
}

func mapPricingErr(err error) error {
	var ve *pricingusecase.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	out := &ValidationError{}
	for _, d := range ve.Details {
		field := d.Field
		if m, ok := pricingFieldMap[field]; ok {
			field = m
		}
		out.add(field, d.Code, d.Message)
	}
	return out
}

func mapAvailabilitySearchErr(err error) error {
	var ve *availusecase.ValidationError
	if errors.As(err, &ve) {
		out := &ValidationError{}
		for _, d := range ve.Details {
			field := d.Field
			if strings.HasPrefix(field, "product_uuids") {
				field = "product_uuid"
			}
			out.add(field, d.Code, d.Message)
		}
		return out
	}
	return err
}

// price runs availability and pricing for a spec (nothing is written).
func (s *Service) price(ctx context.Context, orgID int64, spec quoteSpec) (priced, error) {
	out := priced{}
	ve := &ValidationError{}
	if spec.manual != nil {
		if !has(ctx, rbac.PermTenantDiscountApply) {
			return out, ErrForbidden
		}
		if strings.TrimSpace(spec.manual.Reason) == "" {
			ve.add("manual_discount.reason", "required", "a manual discount needs a reason")
		}
		if spec.coupon != "" {
			ve.add("coupon_code", "not_allowed", "send a coupon code or a manual discount, not both")
		}
	}
	for i, e := range spec.extras {
		if e.ExtraUUID == uuid.Nil {
			ve.add("extras."+itoa(i)+".extra_uuid", "required", "extra_uuid is required")
		}
	}
	if err := ve.orNil(); err != nil {
		return out, err
	}
	res, err := s.avail.Search(ctx, orgID, availmodel.SearchInput{Where: spec.where, ProductUUIDs: []uuid.UUID{spec.productUUID}})
	if err != nil {
		return out, mapAvailabilitySearchErr(err)
	}
	if len(res.Items) != 1 {
		return out, validation("product_uuid", "not_found", "rental product was not found or is not on sale")
	}
	out.search, out.item = res, res.Items[0]
	in := pricingmodel.PreviewInput{
		RentalProductUUID: &spec.productUUID, BranchUUID: &res.Request.PickupBranch.UUID,
		ReturnBranchUUID: &res.Request.ReturnBranch.UUID, Channel: res.Request.Channel,
		StartAt: &res.Request.StartsAt, EndAt: &res.Request.EndsAt, CouponCode: spec.coupon,
	}
	if spec.customer != nil {
		id := spec.customer.Uuid
		in.CustomerUUID = &id
	}
	if spec.deliveryFee != "" {
		fee := pricingmodel.Decimal(spec.deliveryFee)
		in.DeliveryFee = &fee
	}
	if spec.manual != nil {
		in.Discount = &pricingmodel.DiscountInput{Name: strings.TrimSpace(spec.manual.Reason), Kind: spec.manual.Kind, Value: spec.manual.Value}
	}
	for _, e := range spec.extras {
		id, qty := e.ExtraUUID, e.Quantity
		in.Extras = append(in.Extras, pricingmodel.PreviewExtraInput{ExtraUUID: &id, Quantity: &qty})
	}
	preview, err := s.pricing.Preview(ctx, orgID, in)
	if err != nil {
		return out, mapPricingErr(err)
	}
	out.side = preview.Current
	out.warnings = []string{}
	for _, w := range preview.Warnings {
		if w != "no_changes" {
			out.warnings = append(out.warnings, w)
		}
	}
	if out.side.Error != nil {
		return out, &PricingError{Code: out.side.Error.Code, Message: out.side.Error.Message}
	}
	if c := out.side.Coupon; spec.coupon != "" && c != nil && c.Status != "applied" {
		return out, &ValidationError{Details: []Detail{{Field: "coupon_code", Code: "coupon_" + c.Reason, Message: "the coupon does not apply: " + c.Reason}}}
	}
	v, err := s.settingsOf(ctx, orgID)
	if err != nil {
		return out, err
	}
	out.settings = int32(v.Version)
	return out, nil
}

func (s *Service) quoteValidity(ctx context.Context, orgID int64) (time.Duration, error) {
	v, err := s.settingsOf(ctx, orgID)
	if err != nil {
		return 0, err
	}
	return time.Duration(minutesOr(v.Settings.Reservation.QuoteValidityMinutes, model.DefaultQuoteValidityMinutes)) * time.Minute, nil
}

// whereOf maps the quote body's places / range to the engine's.
func whereOf(w model.Where, channel string) availmodel.Where {
	out := availmodel.Where{
		PickupBranchUUID: w.PickupBranchUUID, PickupPointUUID: w.PickupPointUUID,
		ReturnBranchUUID: w.ReturnBranchUUID, ReturnPointUUID: w.ReturnPointUUID, Channel: channel,
	}
	if w.StartsAt != nil {
		out.StartsAt = *w.StartsAt
	}
	if w.EndsAt != nil {
		out.EndsAt = *w.EndsAt
	}
	return out
}

// customerOf resolves an optional customer uuid of a body.
func (s *Service) customerOf(ctx context.Context, orgID int64, id *uuid.UUID, field string) (*db.GetReservationCustomerRow, error) {
	if id == nil {
		return nil, nil
	}
	c, err := s.q.GetReservationCustomer(ctx, db.GetReservationCustomerParams{OrganizationID: orgID, Uuid: *id})
	if notFound(err) {
		return nil, validation(field, "not_found", "customer was not found")
	}
	if err != nil {
		return nil, err
	}
	if c.AnonymizedAt.Valid {
		return nil, validation(field, "anonymized", "the customer was anonymized")
	}
	return &c, nil
}

// Search prices every product on sale (or the given ones) for a request,
// with the availability breakdown of each (the wizard's product picker).
// Nothing is stored.
func (s *Service) Search(ctx context.Context, orgID int64, in model.QuoteSearchInput) (model.QuoteSearchResult, error) {
	if len(in.ProductUUIDs) > model.MaxQuoteSearchProducts {
		return model.QuoteSearchResult{}, validation("product_uuids", "too_many", "at most 100 products")
	}
	res, err := s.avail.Search(ctx, orgID, availmodel.SearchInput{Where: whereOf(in.Where, in.Channel), ProductUUIDs: in.ProductUUIDs})
	if err != nil {
		return model.QuoteSearchResult{}, mapAvailabilitySearchErr(err)
	}
	out := model.QuoteSearchResult{Request: res.Request, Items: []model.QuoteSearchItem{}, Warnings: []string{}}
	seen := map[string]bool{}
	for _, item := range res.Items {
		qi := model.QuoteSearchItem{
			Product: item.Product, Availability: item.Breakdown, Sellable: item.Sellable, Reasons: item.Reasons,
		}
		id := item.Product.UUID
		preview, err := s.pricing.Preview(ctx, orgID, pricingmodel.PreviewInput{
			RentalProductUUID: &id, BranchUUID: &res.Request.PickupBranch.UUID, ReturnBranchUUID: &res.Request.ReturnBranch.UUID,
			Channel: res.Request.Channel, StartAt: &res.Request.StartsAt, EndAt: &res.Request.EndsAt,
		})
		switch {
		case err != nil:
			qi.PriceError = &model.PriceError{Code: "invalid_input", Message: err.Error()}
		case preview.Current.Error != nil:
			qi.PriceError = &model.PriceError{Code: preview.Current.Error.Code, Message: preview.Current.Error.Message}
		default:
			b := preview.Current.Breakdown
			qi.Price = &model.PriceSummary{
				Currency: b.Currency, BillingDays: b.BillingDays, Total: b.Total, Prepayment: b.Prepayment,
				DailyAverage: b.RentalTotal.Ratio(1, int64(max(b.BillingDays, 1))),
			}
			for _, w := range preview.Warnings {
				if w != "no_changes" && !seen[w] {
					seen[w] = true
					out.Warnings = append(out.Warnings, w)
				}
			}
		}
		out.Items = append(out.Items, qi)
	}
	return out, nil
}

// CreateQuote prices and stores a new-sale quote (valid until now + the
// settings' quote validity).
func (s *Service) CreateQuote(ctx context.Context, orgID int64, in model.QuoteInput) (model.Quote, error) {
	ve := &ValidationError{}
	if in.ProductUUID == nil {
		ve.add("product_uuid", "required", "product_uuid is required")
	}
	if in.StartsAt == nil {
		ve.add("starts_at", "required", "starts_at is required")
	}
	if in.EndsAt == nil {
		ve.add("ends_at", "required", "ends_at is required")
	}
	if len([]rune(in.CouponCode)) > 64 {
		ve.add("coupon_code", "too_long", "coupon_code is too long")
	}
	if err := ve.orNil(); err != nil {
		return model.Quote{}, err
	}
	customer, err := s.customerOf(ctx, orgID, in.CustomerUUID, "customer_uuid")
	if err != nil {
		return model.Quote{}, err
	}
	spec := quoteSpec{
		purpose: model.PurposeNew, customer: customer, where: whereOf(in.Where, in.Channel), productUUID: *in.ProductUUID,
		extras: in.Extras, coupon: strings.ToUpper(strings.TrimSpace(in.CouponCode)), manual: in.ManualDiscount,
		deliveryFee: string(in.DeliveryFee),
	}
	p, err := s.price(ctx, orgID, spec)
	if err != nil {
		return model.Quote{}, err
	}
	id, err := s.storeQuote(ctx, orgID, spec, p, storeExtra{})
	if err != nil {
		return model.Quote{}, err
	}
	q, err := s.GetQuote(ctx, orgID, id)
	if err != nil {
		return model.Quote{}, err
	}
	q.Availability = &model.QuoteAvailability{Sellable: p.item.Sellable, Breakdown: p.item.Breakdown, Reasons: p.item.Reasons}
	s.publish(ctx, model.ResourceQuotes, "created", id)
	return q, nil
}

// storeExtra carries the change / extension facts of a stored quote.
type storeExtra struct {
	difference *money.Money
	extension  *model.Extension
	vehicleID  int64
	previous   *model.Conditions
	// totals override the breakdown's (extension quotes charge the
	// extension only).
	override *quoteTotals
}

type quoteTotals struct {
	rentalSubtotal, rentalTotal, extras, base, vat, total, prepayment money.Money
	billingDays                                                       int
}

// storeQuote writes the quote and its lines.
func (s *Service) storeQuote(ctx context.Context, orgID int64, spec quoteSpec, p priced, extra storeExtra) (uuid.UUID, error) {
	b := p.side.Breakdown
	validity, err := s.quoteValidity(ctx, orgID)
	if err != nil {
		return uuid.Nil, err
	}
	conditions, err := s.conditions(ctx, orgID, p.item.Product.UUID, p.item.Product.Version)
	if err != nil {
		return uuid.Nil, err
	}
	breakdown := model.QuoteBreakdown{
		Pricing: b, Extension: extra.extension, Conditions: conditions, PreviousConditions: extra.previous, Warnings: p.warnings,
	}
	product, err := s.q.GetReservationProductByUUID(ctx, db.GetReservationProductByUUIDParams{OrganizationID: orgID, Uuid: p.item.Product.UUID})
	if err != nil {
		return uuid.Nil, err
	}
	pickup, err := s.branchByUUID(ctx, orgID, p.search.Request.PickupBranch.UUID)
	if err != nil {
		return uuid.Nil, err
	}
	ret, err := s.branchByUUID(ctx, orgID, p.search.Request.ReturnBranch.UUID)
	if err != nil {
		return uuid.Nil, err
	}
	discount, extrasTotal, oneWay, delivery := money.Zero(), money.Zero(), money.Zero(), money.Zero()
	if b.Discount != nil {
		discount = b.Discount.Amount
	}
	for _, l := range b.Lines {
		switch l.Kind {
		case pricing.LineExtra:
			extrasTotal = extrasTotal.Add(l.Gross)
		case pricing.LineOneWay:
			oneWay = oneWay.Add(l.Gross)
		case pricing.LineDelivery:
			delivery = delivery.Add(l.Gross)
		}
	}
	t := quoteTotals{
		rentalSubtotal: b.RentalSubtotal, rentalTotal: b.RentalTotal, extras: extrasTotal, base: b.BaseTotal,
		vat: b.VATTotal, total: b.Total, prepayment: b.Prepayment, billingDays: b.BillingDays,
	}
	if extra.override != nil {
		t = *extra.override
		discount, oneWay, delivery = money.Zero(), money.Zero(), money.Zero()
	}
	params := db.InsertQuoteParams{
		OrganizationID: orgID, Purpose: spec.purpose, ReservationID: int8Of(spec.reservationID),
		RentalProductID: product.ID, ProductVersion: p.item.Product.Version,
		PickupBranchID: pickup.ID, ReturnBranchID: ret.ID,
		StartsAt: ts(p.search.Request.StartsAt), EndsAt: ts(p.search.Request.EndsAt), Channel: p.search.Request.Channel,
		BillingDays: int32(t.billingDays), Currency: string(b.Currency),
		RentalSubtotal: t.rentalSubtotal.Numeric(), DiscountTotal: discount.Numeric(), RentalTotal: t.rentalTotal.Numeric(),
		ExtrasTotal: t.extras.Numeric(), OneWayFee: oneWay.Numeric(), DeliveryFee: delivery.Numeric(),
		BaseTotal: t.base.Numeric(), VatTotal: t.vat.Numeric(), Total: t.total.Numeric(), Prepayment: t.prepayment.Numeric(),
		PrepaymentRule: b.PrepaymentRule, SettingsVersion: p.settings,
		Extras: mustJSON(nonNilExtras(spec.extras)), CouponCode: textOf(spec.coupon), Breakdown: mustJSON(breakdown),
		ValidUntil: ts(s.clock().Add(validity)), CreatedBy: int8Ptr(actorID(ctx)),
	}
	if p.search.Request.PickupPoint != nil {
		pt, err := s.pointByUUID(ctx, orgID, p.search.Request.PickupPoint.UUID)
		if err != nil {
			return uuid.Nil, err
		}
		params.PickupPointID = int8Of(pt.ID)
	}
	if p.search.Request.ReturnPoint != nil {
		pt, err := s.pointByUUID(ctx, orgID, p.search.Request.ReturnPoint.UUID)
		if err != nil {
			return uuid.Nil, err
		}
		params.ReturnPointID = int8Of(pt.ID)
	}
	if spec.customer != nil {
		params.CustomerID = int8Of(spec.customer.ID)
	}
	if b.VATRate != nil {
		params.VatRateBp = pgtype.Int4{Int32: int32(*b.VATRate), Valid: true}
	}
	if extra.difference != nil {
		params.Difference = extra.difference.Numeric()
	}
	if id, err := uuid.Parse(b.Plan.VersionID); err == nil {
		params.RatePlanVersionUuid = pgtype.UUID{Bytes: id, Valid: true}
		params.RatePlanVersion = pgtype.Int4{Int32: int32(b.Plan.Version), Valid: true}
	}
	if spec.manual != nil {
		params.ManualDiscount = mustJSON(model.ManualDiscountView{
			Kind: spec.manual.Kind, Value: string(spec.manual.Value), Reason: strings.TrimSpace(spec.manual.Reason),
		})
	}
	if extra.vehicleID != 0 {
		params.VehicleID = int8Of(extra.vehicleID)
	}
	lines := quoteLines(b, extra.extension)
	var created uuid.UUID
	err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		row, err := q.InsertQuote(ctx, params)
		if err != nil {
			return err
		}
		created = row.Uuid
		for _, l := range lines {
			lp := db.InsertQuoteLineParams{
				OrganizationID: orgID, QuoteID: row.ID, Position: int32(l.Position), Kind: l.Kind,
				DateRuleName: l.DateRuleName, Code: l.Code, Name: l.Name, Quantity: int32(l.Quantity),
				UnitPrice: l.UnitPrice.Numeric(), Adjustment: l.Adjustment.Numeric(), Amount: l.Amount.Numeric(),
			}
			if l.DayIndex != nil {
				lp.DayIndex = pgtype.Int4{Int32: int32(*l.DayIndex), Valid: true}
			}
			if l.DayDate != nil {
				if d, err := time.Parse("2006-01-02", *l.DayDate); err == nil {
					lp.DayDate = pgtype.Date{Time: d, Valid: true}
				}
			}
			if l.DateRuleVersionUUID != nil {
				lp.DateRuleVersionUuid = pgtype.UUID{Bytes: *l.DateRuleVersionUUID, Valid: true}
			}
			if err := q.InsertQuoteLine(ctx, lp); err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}

func nonNilExtras(v []model.ExtraRequest) []model.ExtraRequest {
	if v == nil {
		return []model.ExtraRequest{}
	}
	return v
}

// quoteLines turns a breakdown (or the added days of an extension) into
// display lines: one per billing day, extra, fee, and the discount.
func quoteLines(b *pricing.Breakdown, ext *model.Extension) []model.QuoteLine {
	out := []model.QuoteLine{}
	pos := 0
	add := func(l model.QuoteLine) {
		pos++
		l.Position = pos
		out = append(out, l)
	}
	days := b.Days
	if ext != nil {
		days = ext.Days
	}
	for _, d := range days {
		idx := d.Index
		date := d.Date.String()
		l := model.QuoteLine{
			Kind: model.LineDay, DayIndex: &idx, DayDate: &date, Quantity: 1,
			UnitPrice: d.BasePrice, Adjustment: d.Adjustment, Amount: d.UnitPrice,
		}
		if d.Rule != nil {
			l.DateRuleName = d.Rule.Name
			if id, err := uuid.Parse(d.Rule.VersionID); err == nil {
				l.DateRuleVersionUUID = &id
			}
		}
		add(l)
	}
	if ext != nil {
		if !ext.ExtrasAmount.IsZero() {
			add(model.QuoteLine{Kind: model.LineExtra, Code: "extension", Name: "extras", Quantity: 1, UnitPrice: ext.ExtrasAmount, Adjustment: money.Zero(), Amount: ext.ExtrasAmount})
		}
		return out
	}
	for _, l := range b.Lines {
		kind := ""
		switch l.Kind {
		case pricing.LineExtra:
			kind = model.LineExtra
		case pricing.LineOneWay:
			kind = model.LineOneWay
		case pricing.LineDelivery:
			kind = model.LineDelivery
		default:
			continue
		}
		unit := l.Gross
		if l.UnitPrice != nil {
			unit = *l.UnitPrice
		}
		code := l.Code
		if code == "" {
			code = l.ExtraID
		}
		add(model.QuoteLine{Kind: kind, Code: code, Name: l.Name, Quantity: l.Quantity, UnitPrice: unit, Adjustment: money.Zero(), Amount: l.Gross})
	}
	if d := b.Discount; d != nil && !d.Amount.IsZero() {
		add(model.QuoteLine{Kind: model.LineDiscount, Code: d.Code, Name: d.Name, Quantity: 1, UnitPrice: d.Amount.Neg(), Adjustment: money.Zero(), Amount: d.Amount.Neg()})
	}
	return out
}

func (s *Service) branchByUUID(ctx context.Context, orgID int64, id uuid.UUID) (db.GetReservationBranchByUUIDRow, error) {
	return s.q.GetReservationBranchByUUID(ctx, db.GetReservationBranchByUUIDParams{OrganizationID: orgID, Uuid: id})
}

func (s *Service) pointByUUID(ctx context.Context, orgID int64, id uuid.UUID) (db.GetReservationPointByUUIDRow, error) {
	return s.q.GetReservationPointByUUID(ctx, db.GetReservationPointByUUIDParams{OrganizationID: orgID, Uuid: id})
}

// conditions returns the guarantee / conditions of a product version.
func (s *Service) conditions(ctx context.Context, orgID int64, productUUID uuid.UUID, version int32) (*model.Conditions, error) {
	p, err := s.q.GetReservationProductByUUID(ctx, db.GetReservationProductByUUIDParams{OrganizationID: orgID, Uuid: productUUID})
	if err != nil {
		return nil, err
	}
	return s.conditionsByID(ctx, orgID, p.ID, version)
}

func (s *Service) conditionsByID(ctx context.Context, orgID, productID int64, version int32) (*model.Conditions, error) {
	v, err := s.q.GetReservationProductVersion(ctx, db.GetReservationProductVersionParams{OrganizationID: orgID, ProductID: productID, Version: version})
	if err != nil {
		return nil, err
	}
	vu := v.Uuid.String()
	c := &model.Conditions{
		GuaranteeType: v.GuaranteeType, GuaranteeLabel: v.GuaranteeLabel, MinDriverAge: int(v.MinDriverAge),
		MinLicenseYears: int(v.MinLicenseYears), DepositAmount: money.FormatNumeric(v.DepositAmount),
		ExtraKmFee: money.FormatNumeric(v.ExtraKmFee), FuelPolicy: v.FuelPolicy,
		ProductUUID: v.ProductUuid.String(), ProductCode: v.ProductCode, ProductTitle: v.ProductTitle, Version: v.Version,
		VersionUUID: &vu,
	}
	if v.IncludedKmPerDay.Valid {
		km := v.IncludedKmPerDay.Int32
		c.IncludedKmPerDay = &km
	}
	return c, nil
}

// GetQuote returns one quote in the caller's branch scope.
func (s *Service) GetQuote(ctx context.Context, orgID int64, id uuid.UUID) (model.Quote, error) {
	row, err := s.q.GetQuote(ctx, db.GetQuoteParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Quote{}, ErrQuoteNotFound
	}
	if err != nil {
		return model.Quote{}, err
	}
	if !inScope(ctx, row.PickupBranchID, row.ReturnBranchID) {
		return model.Quote{}, ErrQuoteNotFound
	}
	lines, err := s.q.ListQuoteLines(ctx, db.ListQuoteLinesParams{OrganizationID: orgID, QuoteID: row.ID})
	if err != nil {
		return model.Quote{}, err
	}
	return s.quoteView(row, lines), nil
}

func (s *Service) quoteView(row db.GetQuoteRow, lines []db.ListQuoteLinesRow) model.Quote {
	m := money.FromNumericOrZero
	q := model.Quote{
		UUID: row.Uuid, Purpose: row.Purpose, Status: row.Status, Expired: row.Status == model.QuoteOpen && !row.ValidUntil.Time.After(s.clock()),
		Product:      model.ProductRef{UUID: row.ProductUuid, Code: row.ProductCode, Title: row.ProductTitle, Version: row.ProductVersion},
		PickupBranch: model.BranchRef{UUID: row.PickupBranchUuid, Code: row.PickupBranchCode, Name: row.PickupBranchName},
		ReturnBranch: model.BranchRef{UUID: row.ReturnBranchUuid, Code: row.ReturnBranchCode, Name: row.ReturnBranchName},
		StartsAt:     row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(), Channel: row.Channel, BillingDays: int(row.BillingDays),
		Currency: row.Currency, RentalSubtotal: m(row.RentalSubtotal), DiscountTotal: m(row.DiscountTotal),
		RentalTotal: m(row.RentalTotal), ExtrasTotal: m(row.ExtrasTotal), OneWayFee: m(row.OneWayFee),
		DeliveryFee: m(row.DeliveryFee), BaseTotal: m(row.BaseTotal), VATTotal: m(row.VatTotal), Total: m(row.Total),
		Prepayment: m(row.Prepayment), PrepaymentRule: row.PrepaymentRule, SettingsVersion: row.SettingsVersion,
		Extras: []model.ExtraRequest{}, Lines: []model.QuoteLine{}, ValidUntil: row.ValidUntil.Time.UTC(),
		UsedAt: timePtr(row.UsedAt), CreatedAt: row.CreatedAt.Time.UTC(), RatePlanVersionUUID: uuidPtr(row.RatePlanVersionUuid),
	}
	if row.RatePlanVersion.Valid {
		v := row.RatePlanVersion.Int32
		q.RatePlanVersion = &v
	}
	if row.VatRateBp.Valid {
		r := money.Rate(row.VatRateBp.Int32)
		q.VATRate = &r
	}
	if row.Difference.Valid {
		d := m(row.Difference)
		q.Difference = &d
	}
	if row.CouponCode.Valid {
		c := row.CouponCode.String
		q.CouponCode = &c
	}
	_ = json.Unmarshal(row.Extras, &q.Extras)
	if len(row.ManualDiscount) > 0 {
		var md model.ManualDiscountView
		if json.Unmarshal(row.ManualDiscount, &md) == nil {
			q.ManualDiscount = &md
		}
	}
	_ = json.Unmarshal(row.Breakdown, &q.Breakdown)
	if q.Breakdown.Warnings == nil {
		q.Breakdown.Warnings = []string{}
	}
	if row.PickupPointUuid.Valid {
		q.PickupPoint = &model.PointRef{UUID: row.PickupPointUuid.Bytes, Name: row.PickupPointName.String}
	}
	if row.ReturnPointUuid.Valid {
		q.ReturnPoint = &model.PointRef{UUID: row.ReturnPointUuid.Bytes, Name: row.ReturnPointName.String}
	}
	if row.CustomerUuid.Valid {
		q.Customer = &model.CustomerRef{
			UUID: row.CustomerUuid.Bytes, CustomerType: row.CustomerType.String,
			DisplayName: displayName(row.CustomerType.String, row.CustomerFirstName.String, row.CustomerLastName.String, row.CustomerCompanyTitle.String),
		}
	}
	if row.ReservationUuid.Valid {
		q.Reservation = &model.ReservationRef{UUID: row.ReservationUuid.Bytes, Reference: row.ReservationReference.String}
	}
	if row.VehicleUuid.Valid {
		q.Vehicle = &model.VehicleRef{UUID: row.VehicleUuid.Bytes, Plate: row.VehiclePlate.String}
	}
	if row.CreatedByUuid.Valid {
		q.CreatedBy = &model.Actor{UUID: row.CreatedByUuid.Bytes, Name: row.CreatedByName.String, Surname: row.CreatedBySurname.String, Email: row.CreatedByEmail.String}
	}
	for _, l := range lines {
		ql := model.QuoteLine{
			Position: int(l.Position), Kind: l.Kind, DateRuleName: l.DateRuleName, Code: l.Code, Name: l.Name,
			Quantity: int(l.Quantity), UnitPrice: m(l.UnitPrice), Adjustment: m(l.Adjustment), Amount: m(l.Amount),
			DateRuleVersionUUID: uuidPtr(l.DateRuleVersionUuid),
		}
		if l.DayIndex.Valid {
			i := int(l.DayIndex.Int32)
			ql.DayIndex = &i
		}
		if l.DayDate.Valid {
			d := l.DayDate.Time.Format("2006-01-02")
			ql.DayDate = &d
		}
		q.Lines = append(q.Lines, ql)
	}
	return q
}

// displayName is the customer's display name.
func displayName(customerType, first, last, company string) string {
	if customerType == "corporate" {
		return company
	}
	return strings.TrimSpace(first + " " + last)
}

// customerRef builds a masked customer reference.
func customerRef(id uuid.UUID, customerType, first, last, company, identityHint, phone, email string) model.CustomerRef {
	return model.CustomerRef{
		UUID: id, CustomerType: customerType, DisplayName: displayName(customerType, first, last, company),
		IdentityMasked: identityHint, PhoneMasked: pii.MaskPhone(phone), EmailMasked: pii.MaskEmail(email),
	}
}

// ListQuotes returns a page of quotes in the caller's branch scope.
func (s *Service) ListQuotes(ctx context.Context, orgID int64, f model.QuoteFilter) ([]model.QuoteListItem, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if _, ok := model.QuoteSortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	rows, err := s.q.ListQuotes(ctx, db.ListQuotesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Purposes: nilIfEmpty(f.Purposes), Statuses: nilIfEmpty(f.Statuses),
		ProductUuids: nilUUIDs(f.ProductUUIDs), Channels: nilIfEmpty(f.Channels), ReservationUuids: nilUUIDs(f.ReservationUUIDs),
		Q: qArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.QuoteListItem, 0, len(rows))
	var total int64
	now := s.clock()
	for _, r := range rows {
		total = r.TotalCount
		it := model.QuoteListItem{
			UUID: r.Uuid, Purpose: r.Purpose, Status: r.Status, Expired: r.Status == model.QuoteOpen && !r.ValidUntil.Time.After(now),
			Product:      model.ProductRef{UUID: r.ProductUuid, Code: r.ProductCode, Title: r.ProductTitle},
			PickupBranch: model.BranchRef{UUID: r.PickupBranchUuid, Code: r.PickupBranchCode, Name: r.PickupBranchName},
			ReturnBranch: model.BranchRef{UUID: r.ReturnBranchUuid, Code: r.ReturnBranchCode, Name: r.ReturnBranchName},
			StartsAt:     r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(), Channel: r.Channel, BillingDays: int(r.BillingDays),
			Currency: r.Currency, Total: money.FromNumericOrZero(r.Total), ValidUntil: r.ValidUntil.Time.UTC(), CreatedAt: r.CreatedAt.Time.UTC(),
		}
		if r.Difference.Valid {
			d := money.FromNumericOrZero(r.Difference)
			it.Difference = &d
		}
		if r.CustomerUuid.Valid {
			it.Customer = &model.CustomerRef{
				UUID: r.CustomerUuid.Bytes, CustomerType: r.CustomerType.String,
				DisplayName: displayName(r.CustomerType.String, r.CustomerFirstName.String, r.CustomerLastName.String, r.CustomerCompanyTitle.String),
			}
		}
		if r.ReservationUuid.Valid {
			it.Reservation = &model.ReservationRef{UUID: r.ReservationUuid.Bytes, Reference: r.ReservationReference.String}
		}
		out = append(out, it)
	}
	return out, total, nil
}

func pricingDecimal(v string) pricingmodel.Decimal { return pricingmodel.Decimal(v) }

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilUUIDs(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}

func qArg(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	return pgtype.Text{String: v, Valid: v != ""}
}
