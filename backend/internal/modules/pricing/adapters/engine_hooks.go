package adapters

import (
	"context"
	"errors"
	"time"

	campaignmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	campaignusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	extrasmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	extrasusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
)

// Extras plugs the extras catalog (TEC-702) into the preview / quote:
// pricing.Input.Extras.
type Extras struct {
	svc *extrasusecase.Service
}

// NewExtras builds the extras resolver.
func NewExtras(svc *extrasusecase.Service) *Extras { return &Extras{svc: svc} }

// ResolveExtras implements usecase.ExtrasResolver.
func (e *Extras) ResolveExtras(ctx context.Context, orgID, productID int64, channel string, items []model.PreviewExtraInput, at time.Time) ([]pricing.Extra, []usecase.Detail, error) {
	sel := make([]extrasmodel.Selection, 0, len(items))
	for _, it := range items {
		sel = append(sel, extrasmodel.Selection{ExtraUUID: it.ExtraUUID, Quantity: it.Quantity})
	}
	lines, err := e.svc.Resolve(ctx, orgID, extrasusecase.ResolveInput{ProductID: productID, Channel: channel, Items: sel, At: at, Field: "extras"})
	var ve *extrasusecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]usecase.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, usecase.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		return nil, details, nil
	}
	return lines, nil, err
}

// Coupons plugs the campaigns (TEC-703) into the preview / quote:
// pricing.Input.Discount.
type Coupons struct {
	svc *campaignusecase.Service
}

// NewCoupons builds the coupon evaluator.
func NewCoupons(svc *campaignusecase.Service) *Coupons { return &Coupons{svc: svc} }

// EvaluateCoupon implements usecase.CouponEvaluator.
func (c *Coupons) EvaluateCoupon(ctx context.Context, orgID int64, q usecase.CouponQuery) (*pricing.Discount, *model.CouponResult, error) {
	ev, err := c.svc.Evaluate(ctx, orgID, campaignusecase.Quote{
		Code: q.Code, ProductID: q.ProductID, SegmentID: q.SegmentID, Channel: q.Channel, Subtotal: q.Subtotal,
		CustomerUUID: q.CustomerUUID, At: q.At, Corporate: q.Corporate,
	})
	if err != nil {
		return nil, nil, err
	}
	return ev.Discount, couponResult(ev.Result), nil
}

func couponResult(r *campaignmodel.CouponResult) *model.CouponResult {
	if r == nil {
		return nil
	}
	return &model.CouponResult{
		Code: r.Code, CampaignUUID: r.CampaignUUID, CampaignName: r.CampaignName, CampaignType: r.CampaignType,
		Status: r.Status, Reason: r.Reason, Params: r.Params,
	}
}

var (
	_ usecase.ExtrasResolver  = (*Extras)(nil)
	_ usecase.CouponEvaluator = (*Coupons)(nil)
)
