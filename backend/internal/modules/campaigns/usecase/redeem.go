package usecase

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Quote is the sale a campaign is checked against.
type Quote struct {
	// Code is the coupon code; empty = automatic (repeat-customer)
	// campaigns for CustomerUUID.
	Code string
	// CampaignUUID names the campaign directly (Redeem of an automatic
	// campaign chosen by Evaluate); takes precedence over Code.
	CampaignUUID *uuid.UUID
	// ProductID / SegmentID are the rental product being sold and its
	// segment (internal ids).
	ProductID, SegmentID int64
	Channel              string
	// Subtotal is the rental subtotal (after date rules, before the
	// discount) the campaign applies to.
	Subtotal     money.Money
	CustomerUUID *uuid.UUID
	// Corporate is true when the sale uses corporate agreement prices.
	Corporate bool
	// At is the moment the code is used (zero = now). Validity dates are
	// checked on its Istanbul calendar day.
	At time.Time
}

// Evaluation is the outcome of Evaluate.
type Evaluation struct {
	// Discount is the engine input when the campaign applies (nil when
	// rejected or when there is nothing to apply).
	Discount *pricing.Discount
	// Result explains the decision (nil when no code was given and no
	// automatic campaign applies).
	Result *model.CouponResult
}

// Evaluate checks a coupon code (or, without a code, the repeat-customer
// campaigns of the customer) against a quote, without consuming anything.
// A rejection is not an error: Result.Status = "rejected" with the reason
// (not_found, inactive, not_started, expired, channel, scope,
// customer_scope, customer_limit, limit, min_amount, corporate). The
// per-customer limit is checked only when a customer is given.
func (s *Service) Evaluate(ctx context.Context, orgID int64, in Quote) (Evaluation, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" && in.CampaignUUID == nil {
		return s.bestAutomatic(ctx, orgID, in)
	}
	r, err := s.lookup(ctx, s.q, orgID, in)
	if errors.Is(err, ErrNotFound) {
		return Evaluation{Result: &model.CouponResult{Code: code, Status: "rejected", Reason: model.ReasonNotFound, Params: map[string]string{}}}, nil
	}
	if err != nil {
		return Evaluation{}, err
	}
	res := resultFor(r, code)
	reason, params, err := s.check(ctx, s.q, orgID, r, in, false)
	if err != nil {
		return Evaluation{}, err
	}
	if reason == "" && r.UsageLimit.Valid && r.UsedCount >= r.UsageLimit.Int32 {
		reason = model.ReasonLimit
	}
	if reason != "" {
		res.Status, res.Reason, res.Params = "rejected", reason, params
		return Evaluation{Result: res}, nil
	}
	return Evaluation{Discount: discountFor(r, code), Result: res}, nil
}

// bestAutomatic picks the repeat-customer campaign with the largest
// discount the customer qualifies for (none without a customer).
func (s *Service) bestAutomatic(ctx context.Context, orgID int64, in Quote) (Evaluation, error) {
	if in.CustomerUUID == nil {
		return Evaluation{}, nil
	}
	list, err := s.q.ListRepeatCustomerCampaigns(ctx, orgID)
	if err != nil {
		return Evaluation{}, err
	}
	var best *row
	var bestAmount money.Money
	for _, x := range list {
		r := row(x)
		reason, _, err := s.check(ctx, s.q, orgID, r, in, false)
		if err != nil {
			return Evaluation{}, err
		}
		if reason != "" || (r.UsageLimit.Valid && r.UsedCount >= r.UsageLimit.Int32) {
			continue
		}
		amount := DiscountAmount(r, in.Subtotal)
		if best == nil || amount.Cmp(bestAmount) > 0 {
			rr := r
			best, bestAmount = &rr, amount
		}
	}
	if best == nil {
		return Evaluation{}, nil
	}
	return Evaluation{Discount: discountFor(*best, ""), Result: resultFor(*best, "")}, nil
}

func resultFor(r row, code string) *model.CouponResult {
	id := r.Uuid
	if code == "" && r.Code.Valid {
		code = r.Code.String
	}
	return &model.CouponResult{
		Code: code, CampaignUUID: &id, CampaignName: r.Name, CampaignType: r.CampaignType,
		Status: "applied", Params: map[string]string{},
	}
}

// discountFor builds the engine discount of a campaign.
func discountFor(r row, code string) *pricing.Discount {
	if code == "" && r.Code.Valid {
		code = r.Code.String
	}
	d := &pricing.Discount{CampaignID: r.Uuid.String(), Code: code, Name: r.Name, Kind: r.DiscountKind, MaxAmount: moneyPtr(r.MaxDiscount)}
	v := money.FromNumericOrZero(r.DiscountValue)
	if r.DiscountKind == model.KindPercent {
		// NUMERIC(14,2) percent -> basis points (20.00 % = 2000).
		d.Percent = money.Rate(v.Minor())
	} else {
		d.Amount = v
	}
	return d
}

// DiscountAmount is the discount a campaign gives on a subtotal (the same
// rules as the engine: percent or amount, capped by max_discount, never
// above the subtotal).
func DiscountAmount(r row, subtotal money.Money) money.Money {
	d := discountFor(r, "")
	amount := d.Amount
	if d.Kind == model.KindPercent {
		amount = subtotal.MulRate(d.Percent)
	}
	if d.MaxAmount != nil && amount.Cmp(*d.MaxAmount) > 0 {
		amount = *d.MaxAmount
	}
	return amount.Min(subtotal).ClampZero()
}

func (s *Service) lookup(ctx context.Context, q *db.Queries, orgID int64, in Quote) (row, error) {
	if in.CampaignUUID != nil {
		return s.getRow(ctx, q, orgID, *in.CampaignUUID)
	}
	r, err := q.GetCampaignByCode(ctx, db.GetCampaignByCodeParams{OrganizationID: orgID, Code: strings.TrimSpace(in.Code)})
	if errors.Is(err, pgx.ErrNoRows) {
		return row{}, ErrNotFound
	}
	return row(r), err
}

// check evaluates every rule except the total usage limit (the atomic
// counter decides that in Redeem). requireCustomer: a per-customer limit or
// a customer list needs a customer (Redeem); the preview skips the
// per-customer count without one.
func (s *Service) check(ctx context.Context, q *db.Queries, orgID int64, r row, in Quote, requireCustomer bool) (string, map[string]string, error) {
	params := map[string]string{}
	at := in.At
	if at.IsZero() {
		at = s.clock()
	}
	day := rentaltime.LocalDate(at)
	switch {
	case !r.IsActive:
		return model.ReasonInactive, params, nil
	case r.StartDate.Valid && rentaltime.DateOf(r.StartDate.Time).After(day):
		params["start_date"] = rentaltime.DateOf(r.StartDate.Time).String()
		return model.ReasonNotStarted, params, nil
	case r.EndDate.Valid && rentaltime.DateOf(r.EndDate.Time).Before(day):
		params["end_date"] = rentaltime.DateOf(r.EndDate.Time).String()
		return model.ReasonExpired, params, nil
	case r.Channels != nil && !slices.Contains(r.Channels, in.Channel):
		params["channel"] = in.Channel
		return model.ReasonChannel, params, nil
	}
	if r.ProductScope != model.ScopeAll {
		targets, err := q.ListCampaignTargets(ctx, db.ListCampaignTargetsParams{OrganizationID: orgID, CampaignIds: []int64{r.ID}})
		if err != nil {
			return "", nil, err
		}
		ok := false
		for _, t := range targets {
			if (r.ProductScope == model.ScopeProducts && t.RentalProductID.Valid && t.RentalProductID.Int64 == in.ProductID) ||
				(r.ProductScope == model.ScopeSegments && t.SegmentID.Valid && t.SegmentID.Int64 == in.SegmentID) {
				ok = true
				break
			}
		}
		if !ok {
			return model.ReasonScope, params, nil
		}
	}
	if r.CustomerUuids != nil {
		if in.CustomerUUID == nil {
			return model.ReasonCustomerRequired, params, nil
		}
		if !slices.Contains(r.CustomerUuids, *in.CustomerUUID) {
			return model.ReasonCustomerScope, params, nil
		}
	}
	if r.CampaignType == model.TypeRepeatCustomer {
		if in.CustomerUUID == nil {
			return model.ReasonCustomerRequired, params, nil
		}
		n, err := s.rentals.CompletedRentals(ctx, orgID, *in.CustomerUUID)
		if err != nil {
			return "", nil, err
		}
		if n < int(r.MinCompletedRentals) {
			params["min_completed_rentals"] = strconv.Itoa(int(r.MinCompletedRentals))
			params["completed_rentals"] = strconv.Itoa(n)
			return model.ReasonNotEligible, params, nil
		}
	}
	if r.PerCustomerLimit.Valid {
		switch {
		case in.CustomerUUID != nil:
			n, err := q.CountCustomerRedemptions(ctx, db.CountCustomerRedemptionsParams{
				OrganizationID: orgID, CampaignID: r.ID, CustomerUuid: pgtype.UUID{Bytes: *in.CustomerUUID, Valid: true},
			})
			if err != nil {
				return "", nil, err
			}
			if n >= int64(r.PerCustomerLimit.Int32) {
				params["per_customer_limit"] = strconv.Itoa(int(r.PerCustomerLimit.Int32))
				return model.ReasonCustomerLimit, params, nil
			}
		case requireCustomer:
			return model.ReasonCustomerRequired, params, nil
		}
	}
	if in.Corporate && !r.CorporateEligible {
		return model.ReasonCorporate, params, nil
	}
	if r.MinSubtotal.Valid {
		minSub := money.FromNumericOrZero(r.MinSubtotal)
		if in.Subtotal.Cmp(minSub) < 0 {
			params["min_subtotal"] = minSub.String()
			params["subtotal"] = in.Subtotal.String()
			return model.ReasonMinAmount, params, nil
		}
	}
	return "", params, nil
}

// RedeemInput is Redeem: the campaign (code or uuid), the reservation it
// belongs to and the quote it is checked against.
type RedeemInput struct {
	Quote
	ReservationUUID uuid.UUID
	ActorID         *int64
}

// Redeem reserves one use of a campaign for a reservation (TEC-706 calls
// it when the reservation is created). One transaction:
//
//  1. every rule except the total limit (*RejectionError otherwise);
//  2. the atomic counter: UPDATE campaigns SET used_count = used_count + 1
//     WHERE id = $1 AND (usage_limit IS NULL OR used_count < usage_limit)
//     RETURNING — no row = the limit is reached (RejectionError "limit").
//     The UPDATE row-locks the campaign, so concurrent redemptions of one
//     campaign queue here and each re-checks the condition on the latest
//     row: the limit can never be exceeded (and the CHECK constraint
//     used_count <= usage_limit backs it up);
//  3. the per-customer limit, counted after the lock (so two concurrent
//     uses by one customer cannot both pass);
//  4. the redemption row; a reservation carries at most one live
//     redemption (unique index): ErrAlreadyRedeemed.
func (s *Service) Redeem(ctx context.Context, orgID int64, in RedeemInput) (model.Redemption, error) {
	if in.ReservationUUID == uuid.Nil {
		return model.Redemption{}, &ValidationError{Details: []Detail{{Field: "reservation_uuid", Code: "required", Message: "reservation_uuid is required"}}}
	}
	var created uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		r, err := s.lookup(ctx, q, orgID, in.Quote)
		if errors.Is(err, ErrNotFound) {
			return &RejectionError{Reason: model.ReasonNotFound, Params: map[string]string{}}
		}
		if err != nil {
			return err
		}
		reason, params, err := s.check(ctx, q, orgID, r, in.Quote, true)
		if err != nil {
			return err
		}
		if reason != "" {
			return &RejectionError{Reason: reason, Params: params}
		}
		if _, err := q.IncrementCampaignUsage(ctx, db.IncrementCampaignUsageParams{OrganizationID: orgID, ID: r.ID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &RejectionError{Reason: model.ReasonLimit, Params: map[string]string{}}
			}
			return err
		}
		// Per-customer limit after the row lock (step 3).
		if r.PerCustomerLimit.Valid && in.CustomerUUID != nil {
			n, err := q.CountCustomerRedemptions(ctx, db.CountCustomerRedemptionsParams{
				OrganizationID: orgID, CampaignID: r.ID, CustomerUuid: pgtype.UUID{Bytes: *in.CustomerUUID, Valid: true},
			})
			if err != nil {
				return err
			}
			if n >= int64(r.PerCustomerLimit.Int32) {
				return &RejectionError{Reason: model.ReasonCustomerLimit, Params: map[string]string{"per_customer_limit": strconv.Itoa(int(r.PerCustomerLimit.Int32))}}
			}
		}
		params2 := db.InsertCampaignRedemptionParams{
			OrganizationID: orgID, CampaignID: r.ID, ReservationUuid: in.ReservationUUID, Code: r.Code,
			Subtotal: in.Subtotal.Numeric(), DiscountAmount: DiscountAmount(r, in.Subtotal).Numeric(),
		}
		if in.CustomerUUID != nil {
			params2.CustomerUuid = pgtype.UUID{Bytes: *in.CustomerUUID, Valid: true}
		}
		if in.ActorID != nil {
			params2.CreatedBy = pgtype.Int8{Int64: *in.ActorID, Valid: true}
		}
		red, err := q.InsertCampaignRedemption(ctx, params2)
		if err != nil {
			return mapErr(err)
		}
		created = red.Uuid
		return nil
	})
	if err != nil {
		return model.Redemption{}, err
	}
	return s.redemption(ctx, orgID, created)
}

func (s *Service) redemption(ctx context.Context, orgID int64, id uuid.UUID) (model.Redemption, error) {
	x, err := s.q.GetCampaignRedemption(ctx, db.GetCampaignRedemptionParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Redemption{}, ErrRedemptionNotFound
	}
	if err != nil {
		return model.Redemption{}, err
	}
	out := model.Redemption{
		UUID: x.Uuid, ReservationUUID: x.ReservationUuid, Subtotal: money.FromNumericOrZero(x.Subtotal),
		DiscountAmount: money.FromNumericOrZero(x.DiscountAmount), Status: x.Status, ReleaseReason: x.ReleaseReason,
		UsageReturned: x.UsageReturned, CreatedAt: x.CreatedAt.Time.UTC(), ConsumedAt: timePtr(x.ConsumedAt),
		ReleasedAt: timePtr(x.ReleasedAt),
	}
	if x.CustomerUuid.Valid {
		u := uuid.UUID(x.CustomerUuid.Bytes)
		out.CustomerUUID = &u
	}
	if x.Code.Valid {
		v := x.Code.String
		out.Code = &v
	}
	return out, nil
}

// Consume marks the reservation's redemption consumed (the rental was
// completed / paid; TEC-706 / M3).
func (s *Service) Consume(ctx context.Context, orgID int64, reservation uuid.UUID) (model.Redemption, error) {
	var id uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		r, err := q.GetLiveRedemptionForReservation(ctx, db.GetLiveRedemptionForReservationParams{OrganizationID: orgID, ReservationUuid: reservation})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRedemptionNotFound
		}
		if err != nil {
			return err
		}
		id = r.Uuid
		if r.Status == model.RedemptionConsumed {
			return nil
		}
		_, err = q.ConsumeCampaignRedemption(ctx, db.ConsumeCampaignRedemptionParams{OrganizationID: orgID, ID: r.ID, NowAt: pgtype.Timestamptz{Time: s.clock(), Valid: true}})
		return err
	})
	if err != nil {
		return model.Redemption{}, err
	}
	return s.redemption(ctx, orgID, id)
}

// Release releases the reservation's redemption (cancel / refund). The
// campaign's release_on_cancel setting decides whether the use is given
// back (used_count - 1, usage_returned = true) or stays counted.
func (s *Service) Release(ctx context.Context, orgID int64, reservation uuid.UUID, reason string) (model.Redemption, error) {
	if reason == "" {
		reason = "cancelled"
	}
	if len(reason) > 32 {
		reason = reason[:32]
	}
	var id uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		r, err := q.GetLiveRedemptionForReservation(ctx, db.GetLiveRedemptionForReservationParams{OrganizationID: orgID, ReservationUuid: reservation})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRedemptionNotFound
		}
		if err != nil {
			return err
		}
		id = r.Uuid
		returned := false
		if r.ReleaseOnCancel {
			n, err := q.DecrementCampaignUsage(ctx, db.DecrementCampaignUsageParams{OrganizationID: orgID, ID: r.CampaignID})
			if err != nil {
				return err
			}
			returned = n == 1
		}
		_, err = q.ReleaseCampaignRedemption(ctx, db.ReleaseCampaignRedemptionParams{
			OrganizationID: orgID, ID: r.ID, NowAt: pgtype.Timestamptz{Time: s.clock(), Valid: true},
			ReleaseReason: reason, UsageReturned: returned,
		})
		return err
	})
	if err != nil {
		return model.Redemption{}, err
	}
	return s.redemption(ctx, orgID, id)
}
