package usecase

import (
	"context"
	"encoding/json"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/delivery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Fee sources of a delivery point.
const (
	FeeExtra = "extra"
	FeeFlat  = "flat"
	FeeNone  = "none"
)

func pointRules(p db.GetDeliveryPointRow) delivery.Point {
	out := delivery.Point{
		Type: p.Type, Active: p.IsActive, OnlineSelectable: p.OnlineSelectable, DeliveryEnabled: p.DeliveryEnabled,
		MinNoticeMinutes: p.MinNoticeMinutes, Hours: delivery.ParseHours(p.WorkingHours),
		Holidays: delivery.ParseHolidays(p.BranchHolidayExceptions), ServiceDistricts: p.ServiceDistricts,
		Latitude: floatPtr(p.Latitude), Longitude: floatPtr(p.Longitude),
	}
	if h := delivery.ParseHours(p.BranchWorkingHours); h != nil {
		out.BranchHours = *h
	}
	if p.ServiceRadiusKm.Valid {
		if f, err := p.ServiceRadiusKm.Float64Value(); err == nil && f.Valid {
			v := f.Float64
			out.RadiusKm = &v
		}
	}
	return out
}

// fee is the fee of a point for a product and a rental length: the
// published delivery extra's price (product row, else default row; per day
// times the billing days; capped), else the point's flat fee.
func (s *Service) fee(ctx context.Context, orgID int64, p db.GetDeliveryPointRow, product *uuid.UUID, days int) (money.Money, string, *db.GetDeliveryExtraPriceRow, error) {
	if p.DeliveryExtraID.Valid {
		arg := db.GetDeliveryExtraPriceParams{OrganizationID: orgID, ExtraID: p.DeliveryExtraID.Int64, At: ts(s.clock())}
		if product != nil {
			arg.ProductUuid = pgtype.UUID{Bytes: *product, Valid: true}
		}
		price, err := s.q.GetDeliveryExtraPrice(ctx, arg)
		switch {
		case err == nil && price.Status == "published":
			amount := money.FromNumericOrZero(price.Price)
			if price.ChargeType == "per_day" && days > 1 {
				amount = amount.Mul(int64(days))
			}
			if price.MaxTotal.Valid {
				if limit := money.FromNumericOrZero(price.MaxTotal); amount.Cmp(limit) > 0 {
					amount = limit
				}
			}
			return amount, FeeExtra, &price, nil
		case err != nil && !notFound(err):
			return money.Zero(), "", nil, err
		}
	}
	flat := money.FromNumericOrZero(p.ExtraFee)
	if flat.IsZero() {
		return flat, FeeNone, nil, nil
	}
	return flat, FeeFlat, nil, nil
}

// QuoteDelivery implements the reservations' DeliveryQuoter: eligibility
// of the delivery points of a quote and their fee lines.
func (s *Service) QuoteDelivery(ctx context.Context, orgID int64, in resusecase.DeliveryInput) (resusecase.DeliveryQuote, error) {
	out := resusecase.DeliveryQuote{}
	days := 1
	if r, err := rentaltime.NewRange(in.StartsAt, in.EndsAt); err == nil {
		days = r.BillingDays()
	}
	product := in.ProductUUID
	now := s.clock()
	for _, side := range []struct {
		prefix, code string
		at           pgtype.Timestamptz
		in           *resusecase.DeliverySide
	}{
		{"pickup", "delivery", ts(in.StartsAt), in.Pickup},
		{"return", "collection", ts(in.EndsAt), in.Return},
	} {
		if side.in == nil {
			continue
		}
		p, err := s.q.GetDeliveryPoint(ctx, db.GetDeliveryPointParams{OrganizationID: orgID, Uuid: side.in.PointUUID})
		if notFound(err) {
			out.Problems = append(out.Problems, resusecase.Detail{Field: side.prefix + "_point_uuid", Code: "not_found", Message: "service point was not found"})
			continue
		}
		if err != nil {
			return out, err
		}
		details := side.in.Details
		if details != nil {
			d := *details
			d.Normalize()
			details = &d
		}
		if side.in.Check {
			for _, pr := range delivery.Check(side.prefix, pointRules(p), side.at.Time, now, details, in.Channel) {
				out.Problems = append(out.Problems, resusecase.Detail{Field: pr.Field, Code: pr.Code, Message: pr.Message})
			}
		} else if !p.DeliveryEnabled && details != nil && details.Address != "" {
			out.Problems = append(out.Problems, resusecase.Detail{
				Field: side.prefix + "_delivery.address", Code: delivery.CodeNotDelivery, Message: "this service point does not deliver to an address",
			})
		}
		amount, source, price, err := s.fee(ctx, orgID, p, &product, days)
		if err != nil {
			return out, err
		}
		if source == FeeNone || amount.IsZero() {
			continue
		}
		line := pricing.DeliveryFee{Code: side.code, Name: p.Name, Fee: amount}
		if price != nil {
			line.ExtraID, line.PriceID = price.ExtraUuid.String(), price.PriceUuid.String()
		}
		out.Fees = append(out.Fees, line)
	}
	return out, nil
}

// DeliveryPoints lists the delivery points (optionally of one branch) with
// their default fee, hours and conditions for the wizard. product prices
// the fee for that rental product (one billing day).
func (s *Service) DeliveryPoints(ctx context.Context, orgID int64, branch, product *uuid.UUID) ([]model.DeliveryPoint, error) {
	arg := db.ListDeliveryPointsParams{OrganizationID: orgID}
	if branch != nil {
		arg.BranchUuid = pgtype.UUID{Bytes: *branch, Valid: true}
	}
	rows, err := s.q.ListDeliveryPoints(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := []model.DeliveryPoint{}
	for _, r := range rows {
		if !inScope(ctx, r.BranchID) {
			continue
		}
		p := db.GetDeliveryPointRow{ExtraFee: r.ExtraFee, DeliveryExtraID: r.DeliveryExtraID}
		amount, source, _, err := s.fee(ctx, orgID, p, product, 1)
		if err != nil {
			return nil, err
		}
		dp := model.DeliveryPoint{
			UUID: r.Uuid, Name: r.Name, Type: r.Type, Description: r.Description,
			Branch: model.Ref{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
			Fee:    amount.String(), FeeSource: source, RequiresAddress: r.Type == "address_zone",
			ServiceDistricts: r.ServiceDistricts, MinNoticeMinutes: r.MinNoticeMinutes, Conditions: r.DeliveryConditions,
			OnlineSelectable: r.OnlineSelectable,
		}
		if dp.ServiceDistricts == nil {
			dp.ServiceDistricts = []string{}
		}
		if r.DeliveryExtraUuid.Valid {
			dp.DeliveryExtra = &model.Ref{UUID: r.DeliveryExtraUuid.Bytes, Name: r.DeliveryExtraName.String}
		}
		if len(r.WorkingHours) > 0 && string(r.WorkingHours) != "null" {
			var h any
			if json.Unmarshal(r.WorkingHours, &h) == nil {
				dp.WorkingHours = h
			}
		}
		out = append(out, dp)
	}
	return out, nil
}
