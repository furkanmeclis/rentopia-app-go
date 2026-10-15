package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResolveInput is what the resolver needs to price selected extras.
type ResolveInput struct {
	// ProductID is the rental product being priced (internal id).
	ProductID int64
	Channel   string
	Items     []model.Selection
	// At is the quote instant that picks the effective price rows (zero =
	// now).
	At time.Time
	// Field prefixes the validation detail paths ("extras").
	Field string
}

// resolvedExtra is one catalog row the resolver checks.
type resolvedExtra struct {
	row   db.GetExtrasByUUIDsRow
	qty   int
	index int
}

// Resolve turns selected extras into pricing engine lines (TEC-702):
// the price of the rental product effective at the quote instant (else
// the default price), the charge type (per day x billing days in the
// engine, per unit x quantity, per rental once), the quantity bounds and
// the line cap (max_total). A problem with any selection is a validation
// error on that selection ("extras.N.extra_uuid" / "extras.N.quantity"):
// not_found, not_available (not published), channel_not_allowed,
// out_of_range, duplicate, no_price.
func (s *Service) Resolve(ctx context.Context, orgID int64, in ResolveInput) ([]pricing.Extra, error) {
	if len(in.Items) == 0 {
		return nil, nil
	}
	prefix := in.Field
	if prefix == "" {
		prefix = "extras"
	}
	at := in.At
	if at.IsZero() {
		at = s.clock()
	}
	ids := []uuid.UUID{}
	for _, it := range in.Items {
		if it.ExtraUUID != nil {
			ids = append(ids, *it.ExtraUUID)
		}
	}
	rows, err := s.q.GetExtrasByUUIDs(ctx, db.GetExtrasByUUIDsParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	ve := &ValidationError{}
	picked := make([]resolvedExtra, 0, len(in.Items))
	seen := map[uuid.UUID]bool{}
	for i, it := range in.Items {
		field := fmt.Sprintf("%s.%d", prefix, i)
		if it.ExtraUUID == nil {
			ve.add(field+".extra_uuid", "required", "extra_uuid is required")
			continue
		}
		idx := slices.IndexFunc(rows, func(r db.GetExtrasByUUIDsRow) bool { return r.Uuid == *it.ExtraUUID })
		if idx < 0 {
			ve.add(field+".extra_uuid", "not_found", "extra was not found")
			continue
		}
		row := rows[idx]
		if seen[row.Uuid] {
			ve.add(field+".extra_uuid", "duplicate", "the extra is selected twice")
			continue
		}
		seen[row.Uuid] = true
		if row.Status != model.StatusPublished {
			ve.add(field+".extra_uuid", "not_available", "the extra is not published")
			continue
		}
		if row.Channels != nil && !slices.Contains(row.Channels, in.Channel) {
			ve.add(field+".extra_uuid", "channel_not_allowed", "the extra is not sold through this channel")
			continue
		}
		qty := int(row.MinQuantity)
		if it.Quantity != nil {
			qty = *it.Quantity
		}
		if row.ChargeType == model.ChargePerRental {
			qty = 1
		}
		if qty < int(row.MinQuantity) || qty > int(row.MaxQuantity) {
			ve.add(field+".quantity", "out_of_range", fmt.Sprintf("quantity must be between %d and %d", row.MinQuantity, row.MaxQuantity))
			continue
		}
		picked = append(picked, resolvedExtra{row: row, qty: qty, index: i})
	}
	if err := ve.orNil(); err != nil {
		return nil, err
	}
	extraIDs := make([]int64, 0, len(picked))
	for _, p := range picked {
		extraIDs = append(extraIDs, p.row.ID)
	}
	prices, err := s.q.ResolveExtraPrices(ctx, db.ResolveExtraPricesParams{
		OrganizationID: orgID, ExtraIds: extraIDs, RentalProductID: pgtype.Int8{Int64: in.ProductID, Valid: true}, NowAt: ts(at),
	})
	if err != nil {
		return nil, err
	}
	out := make([]pricing.Extra, 0, len(picked))
	for _, p := range picked {
		idx := slices.IndexFunc(prices, func(r db.ResolveExtraPricesRow) bool { return r.ExtraID == p.row.ID })
		if idx < 0 {
			ve.add(fmt.Sprintf("%s.%d.extra_uuid", prefix, p.index), "no_price", "the extra has no price for this product")
			continue
		}
		price := money.FromNumericOrZero(prices[idx].Price)
		out = append(out, pricing.Extra{
			Code: p.row.Code, Name: p.row.Name, UnitPrice: price, Quantity: p.qty,
			PerDay: p.row.ChargeType == model.ChargePerDay, MaxTotal: moneyPtr(p.row.MaxTotal),
			ExtraID: p.row.Uuid.String(), PriceID: prices[idx].Uuid.String(),
		})
	}
	if err := ve.orNil(); err != nil {
		return nil, err
	}
	return out, nil
}
