package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/google/uuid"
)

// SearchCap bounds the units counted per product (the answer says "at
// least SearchCap" beyond it).
const SearchCap = 500

// Search answers "how many units of each product can this channel sell for
// this pick-up / return and range", with the breakdown behind each number.
// A quote consumes nothing. It runs in one read-only snapshot; due holds no
// longer count.
func (s *Service) Search(ctx context.Context, orgID int64, in model.SearchInput) (model.SearchResult, error) {
	r, err := s.resolve(ctx, orgID, in.Where)
	if err != nil {
		return model.SearchResult{}, err
	}
	snap, err := s.snapshot(ctx, orgID)
	if err != nil {
		return model.SearchResult{}, err
	}
	products := snap.products
	if len(in.ProductUUIDs) > 0 {
		products, err = s.pickProducts(snap, in.ProductUUIDs)
		if err != nil {
			return model.SearchResult{}, err
		}
	}
	out := model.SearchResult{Request: r.echo(), Items: []model.ProductAvailability{}}
	now := s.clock()
	err = s.read(ctx, func(q *db.Queries) error {
		rules, err := q.ListActiveCapacityRules(ctx, orgID)
		if err != nil {
			return err
		}
		states := map[int64]*componentState{}
		for _, p := range products {
			item, err := s.productAvailability(ctx, q, orgID, snap, states, rules, p, r, now)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	return out, err
}

func (s *Service) pickProducts(snap *fleetSnapshot, ids []uuid.UUID) ([]*versionInfo, error) {
	byUUID := map[uuid.UUID]*versionInfo{}
	for _, p := range snap.products {
		byUUID[p.ProductUUID] = p
	}
	out := []*versionInfo{}
	ve := &ValidationError{}
	for i, id := range ids {
		p, ok := byUUID[id]
		if !ok {
			ve.add("product_uuids."+itoa(i), "not_found", "rental product was not found or is not on sale")
			continue
		}
		out = append(out, p)
	}
	return out, ve.orNil()
}

func (s *Service) productAvailability(
	ctx context.Context, q *db.Queries, orgID int64, snap *fleetSnapshot, states map[int64]*componentState,
	rules []ruleRow, p *versionInfo, r resolved, now time.Time,
) (model.ProductAvailability, error) {
	key := versionKey(p.ProductID, p.Version)
	item := model.ProductAvailability{
		Product: productRef(p), Reasons: []string{},
	}
	allowed, err := s.oneWayAllowed(ctx, q, orgID, r, p.ProductID)
	if err != nil {
		return item, err
	}
	comp := snap.componentOf(key)
	if comp == 0 {
		item.Reasons = append(item.Reasons, model.ReasonNoEligibleVehicle)
		return item, nil
	}
	cs, ok := states[comp]
	if !ok {
		cs, err = s.loadComponent(ctx, q, orgID, snap, comp, now)
		if err != nil {
			return item, err
		}
		cs.dropDue()
		states[comp] = cs
	}
	b := cs.st.Capacity(r.request(key), SearchCap, s.opts)
	item.Breakdown.Breakdown = b
	u, err := s.productUsage(ctx, q, orgID, p.ProductID, r, now)
	if err != nil {
		return item, err
	}
	evaluate(pickRules(rules, p.ProductID, r.pickup.ID), u, r.channel).apply(b.Physical, &item.Breakdown)
	if !allowed {
		item.Reasons = append(item.Reasons, model.ReasonOneWayNotAllowed)
		item.Breakdown.Sellable = 0
	}
	item.Sellable = item.Breakdown.Sellable
	return item, nil
}

func productRef(p *versionInfo) model.ProductRef {
	return model.ProductRef{UUID: p.ProductUUID, Code: p.Code, Title: p.Title, GuaranteeType: p.GuaranteeType, Version: p.Version}
}

// alternatives are offered with CAPACITY_UNAVAILABLE: other products
// sellable for the same request, and nearby ranges (±2 h, ±1 day) where the
// product is sellable. Best effort: errors drop the suggestions.
func (s *Service) alternatives(ctx context.Context, orgID int64, in model.Where, productUUID uuid.UUID) *model.Alternatives {
	out := &model.Alternatives{Products: []model.ProductAlternative{}, Times: []model.TimeAlternative{}}
	if res, err := s.Search(ctx, orgID, model.SearchInput{Where: in}); err == nil {
		for _, item := range res.Items {
			if item.Product.UUID != productUUID && item.Sellable > 0 && len(out.Products) < 5 {
				out.Products = append(out.Products, model.ProductAlternative{Product: item.Product, Sellable: item.Sellable})
			}
		}
	}
	for _, shift := range []time.Duration{-2 * time.Hour, 2 * time.Hour, -24 * time.Hour, 24 * time.Hour} {
		w := in
		w.StartsAt, w.EndsAt = in.StartsAt.Add(shift), in.EndsAt.Add(shift)
		if w.StartsAt.Before(s.clock()) {
			continue
		}
		res, err := s.Search(ctx, orgID, model.SearchInput{Where: w, ProductUUIDs: []uuid.UUID{productUUID}})
		if err != nil || len(res.Items) == 0 || res.Items[0].Sellable == 0 {
			continue
		}
		out.Times = append(out.Times, model.TimeAlternative{StartsAt: w.StartsAt.UTC(), EndsAt: w.EndsAt.UTC(), Sellable: res.Items[0].Sellable})
	}
	return out
}
