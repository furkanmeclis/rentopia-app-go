package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	fleetusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func numericFloat(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	return &f.Float64
}

func floatPtr(f pgtype.Float8) *float64 {
	if !f.Valid {
		return nil
	}
	return &f.Float64
}

func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

// Locations lists the active branches with their online-selectable
// service points.
func (s *Service) Locations(ctx context.Context, c model.Credential) ([]model.Location, error) {
	branches, err := s.q.ListPublicBranches(ctx, c.OrganizationID)
	if err != nil {
		return nil, err
	}
	points, err := s.q.ListPublicServicePoints(ctx, c.OrganizationID)
	if err != nil {
		return nil, err
	}
	byBranch := map[int64][]model.Point{}
	for _, p := range points {
		byBranch[p.BranchID] = append(byBranch[p.BranchID], model.Point{
			UUID: p.Uuid, Type: p.Type, Name: p.Name, Description: p.Description, ExtraFee: money.FromNumericOrZero(p.ExtraFee),
			Delivery: p.DeliveryEnabled, MinNoticeMinutes: int(p.MinNoticeMinutes), DeliveryConditions: p.DeliveryConditions,
			Latitude: floatPtr(p.Latitude), Longitude: floatPtr(p.Longitude), WorkingHours: rawJSON(p.WorkingHours),
		})
	}
	out := make([]model.Location, 0, len(branches))
	for _, b := range branches {
		pts := byBranch[b.ID]
		if pts == nil {
			pts = []model.Point{}
		}
		out = append(out, model.Location{
			UUID: b.Uuid, Code: b.Code, Name: b.Name, Address: b.Address, City: b.City, District: b.District,
			Latitude: numericFloat(b.Latitude), Longitude: numericFloat(b.Longitude), Phone: b.Phone, Email: b.Email,
			WorkingHours: rawJSON(b.WorkingHours), HolidayExceptions: rawJSON(b.HolidayExceptions), Points: pts,
		})
	}
	return out, nil
}

// Subgroups lists the products on sale with their current conditions and
// API-owned image paths.
func (s *Service) Subgroups(ctx context.Context, c model.Credential) ([]model.Subgroup, error) {
	rows, err := s.q.ListPublicSubgroups(ctx, c.OrganizationID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	images := map[int64][]string{}
	if len(ids) > 0 {
		imgs, err := s.q.ListPublicSubgroupImages(ctx, db.ListPublicSubgroupImagesParams{OrganizationID: c.OrganizationID, ProductIds: ids})
		if err != nil {
			return nil, err
		}
		for _, i := range imgs {
			images[i.ProductID] = append(images[i.ProductID], fleetusecase.PublicImagePath(c.OrganizationUUID, i.Uuid))
		}
	}
	out := make([]model.Subgroup, 0, len(rows))
	for _, r := range rows {
		var km *int
		if r.IncludedKmPerDay.Valid {
			v := int(r.IncludedKmPerDay.Int32)
			km = &v
		}
		imgs := images[r.ID]
		if imgs == nil {
			imgs = []string{}
		}
		out = append(out, model.Subgroup{
			UUID: r.Uuid, Code: r.Code, Title: r.Title, Description: r.Description, Segment: r.SegmentName,
			SegmentCode: r.SegmentCode, RepresentativeModel: r.RepresentativeModel, Version: int(r.CurrentVersion),
			Conditions: model.Conditions{
				GuaranteeType: r.GuaranteeType, GuaranteeLabel: r.GuaranteeLabel, MinDriverAge: int(r.MinDriverAge),
				MinLicenseYears: int(r.MinLicenseYears), DepositAmount: money.FromNumericOrZero(r.DepositAmount),
				IncludedKmPerDay: km, ExtraKmFee: money.FromNumericOrZero(r.ExtraKmFee), FuelPolicy: r.FuelPolicy,
			},
			Images: imgs,
		})
	}
	return out, nil
}

// Extras lists the extras sold on the credential's channel. Prices depend
// on the subgroup and the dates: a quote prices them.
func (s *Service) Extras(ctx context.Context, c model.Credential) ([]model.Extra, error) {
	rows, err := s.q.ListPublicExtras(ctx, db.ListPublicExtrasParams{OrganizationID: c.OrganizationID, Channel: c.Channel})
	if err != nil {
		return nil, err
	}
	out := make([]model.Extra, 0, len(rows))
	for _, r := range rows {
		e := model.Extra{
			UUID: r.Uuid, Code: r.Code, Name: r.Name, Description: r.Description, Type: r.ExtraType, ChargeType: r.ChargeType,
			MinQuantity: int(r.MinQuantity), MaxQuantity: int(r.MaxQuantity),
		}
		if r.MaxTotal.Valid {
			m := money.FromNumericOrZero(r.MaxTotal)
			e.MaxTotal = &m
		}
		out = append(out, e)
	}
	return out, nil
}

// AvailabilityInput is GET /v1/public/availability.
type AvailabilityInput struct {
	PickupBranchUUID *uuid.UUID
	PickupPointUUID  *uuid.UUID
	ReturnBranchUUID *uuid.UUID
	ReturnPointUUID  *uuid.UUID
	StartsAt         *time.Time
	EndsAt           *time.Time
	SubgroupUUIDs    []uuid.UUID
}

func place(id uuid.UUID, name string) model.Place { return model.Place{UUID: id, Name: name} }

// Availability prices every subgroup on sale (or the given ones) for a
// request on the credential's sales channel: the same engine, channel
// rules and pricing as the panel's quote search, so the site, the widget
// and the API answer the same request the same way (RT-A27). Capacity is
// revealed only as a capped number of units.
func (s *Service) Availability(ctx context.Context, c model.Credential, in AvailabilityInput) (model.Availability, error) {
	ve := &ValidationError{}
	if in.StartsAt != nil && in.StartsAt.Before(s.clock()) {
		ve.add("starts_at", "in_past", "the pick-up time is in the past")
	}
	if len(in.SubgroupUUIDs) > resmodel.MaxQuoteSearchProducts {
		ve.add("subgroup_uuids", "too_many", "at most 100 subgroups")
	}
	if err := ve.orNil(); err != nil {
		return model.Availability{}, err
	}
	octx := credentialContext(ctx, c)
	res, err := s.reservations.Search(octx, c.OrganizationID, resmodel.QuoteSearchInput{
		Where: resmodel.Where{
			PickupBranchUUID: in.PickupBranchUUID, PickupPointUUID: in.PickupPointUUID, ReturnBranchUUID: in.ReturnBranchUUID,
			ReturnPointUUID: in.ReturnPointUUID, StartsAt: in.StartsAt, EndsAt: in.EndsAt,
		},
		Channel: c.Channel, ProductUUIDs: in.SubgroupUUIDs,
	})
	if err != nil {
		return model.Availability{}, publicFieldErr(err)
	}
	out := model.Availability{
		Request: model.SearchRequest{
			PickupBranch: place(res.Request.PickupBranch.UUID, res.Request.PickupBranch.Name),
			ReturnBranch: place(res.Request.ReturnBranch.UUID, res.Request.ReturnBranch.Name),
			StartsAt:     res.Request.StartsAt, EndsAt: res.Request.EndsAt, Channel: res.Request.Channel,
		},
		Items: []model.AvailabilityItem{}, Warnings: res.Warnings,
	}
	if p := res.Request.PickupPoint; p != nil {
		v := place(p.UUID, p.Name)
		out.Request.PickupPoint = &v
	}
	if p := res.Request.ReturnPoint; p != nil {
		v := place(p.UUID, p.Name)
		out.Request.ReturnPoint = &v
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	for _, it := range res.Items {
		item := model.AvailabilityItem{
			Subgroup: model.SubgroupRef{UUID: it.Product.UUID, Code: it.Product.Code, Title: it.Product.Title},
			Reasons:  append([]string{}, it.Reasons...),
		}
		if it.Price != nil {
			item.Price = &model.Price{
				Currency: string(it.Price.Currency), BillingDays: it.Price.BillingDays, DailyAverage: it.Price.DailyAverage,
				Total: it.Price.Total, Prepayment: it.Price.Prepayment,
			}
		} else {
			code := "no_price"
			if it.PriceError != nil && it.PriceError.Code != "" {
				code = it.PriceError.Code
			}
			item.Reasons = append(item.Reasons, code)
		}
		item.UnitsLeft = min(max(it.Sellable, 0), model.MaxUnitsLeft)
		item.Available = item.UnitsLeft > 0 && item.Price != nil
		if !item.Available && item.UnitsLeft == 0 && len(item.Reasons) == 0 {
			item.Reasons = append(item.Reasons, "sold_out")
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
