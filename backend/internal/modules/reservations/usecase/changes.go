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
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

func snapshotOf(raw []byte) model.PriceSnapshot {
	var s model.PriceSnapshot
	_ = json.Unmarshal(raw, &s)
	if s.Extras == nil {
		s.Extras = []model.ExtraRequest{}
	}
	if s.Extensions == nil {
		s.Extensions = []model.Extension{}
	}
	return s
}

func changeable(c db.GetReservationCoreRow) error {
	if !model.HoldsCapacity(c.Status) || !allocationActive(c) {
		return stateErr("not_changeable", "only a reservation that holds capacity can be changed")
	}
	return nil
}

func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameExtras(a, b []model.ExtraRequest) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ChangeQuote prices a change of a reservation (dates / places / product /
// extras) as a new quote with the difference to its current total. Moving
// only the return later (same start, places, product and extras) is an
// extension: past lines stay as sold, the added days are priced with the
// band of the NEW total length.
func (s *Service) ChangeQuote(ctx context.Context, orgID int64, id uuid.UUID, in model.ChangeQuoteInput) (model.Quote, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Quote{}, err
	}
	if err := changeable(c); err != nil {
		return model.Quote{}, err
	}
	w, _, err := s.where(ctx, orgID, id)
	if err != nil {
		return model.Quote{}, err
	}
	snap := snapshotOf(c.QuoteSnapshot)
	nw := w
	if in.PickupBranchUUID != nil || in.PickupPointUUID != nil {
		nw.PickupBranchUUID, nw.PickupPointUUID = in.PickupBranchUUID, in.PickupPointUUID
	}
	if in.ReturnBranchUUID != nil || in.ReturnPointUUID != nil {
		nw.ReturnBranchUUID, nw.ReturnPointUUID = in.ReturnBranchUUID, in.ReturnPointUUID
	}
	if in.StartsAt != nil {
		nw.StartsAt = in.StartsAt.UTC()
	}
	if in.EndsAt != nil {
		nw.EndsAt = in.EndsAt.UTC()
	}
	product := c.ProductUuid
	if in.ProductUUID != nil {
		product = *in.ProductUUID
	}
	extras := snap.Extras
	if in.Extras != nil {
		extras = *in.Extras
	}
	samePlaces := sameUUID(nw.PickupBranchUUID, w.PickupBranchUUID) && sameUUID(nw.PickupPointUUID, w.PickupPointUUID) &&
		sameUUID(nw.ReturnBranchUUID, w.ReturnBranchUUID) && sameUUID(nw.ReturnPointUUID, w.ReturnPointUUID)
	if product == c.ProductUuid && samePlaces && nw.StartsAt.Equal(w.StartsAt) && nw.EndsAt.After(w.EndsAt) &&
		sameExtras(extras, snap.Extras) && in.ManualDiscount == nil {
		qid, _, err := s.extensionQuote(ctx, orgID, c, w, nw.EndsAt, snap, 0)
		if err != nil {
			return model.Quote{}, err
		}
		s.publish(ctx, model.ResourceQuotes, "created", qid)
		return s.GetQuote(ctx, orgID, qid)
	}
	purpose := model.PurposeChange
	if product != c.ProductUuid {
		purpose = model.PurposeUpgrade
	}
	qid, err := s.changeQuote(ctx, orgID, c, quoteSpec{
		purpose: purpose, reservationID: c.ID, where: nw, productUUID: product, extras: extras, manual: in.ManualDiscount,
	}, 0)
	if err != nil {
		return model.Quote{}, err
	}
	s.publish(ctx, model.ResourceQuotes, "created", qid)
	return s.GetQuote(ctx, orgID, qid)
}

// changeQuote prices and stores a change / upgrade quote of a reservation.
func (s *Service) changeQuote(ctx context.Context, orgID int64, c db.GetReservationCoreRow, spec quoteSpec, vehicleID int64) (uuid.UUID, error) {
	p, err := s.price(ctx, orgID, spec)
	if err != nil {
		return uuid.Nil, err
	}
	diff := p.side.Breakdown.Total.Sub(money.FromNumericOrZero(c.TotalAmount))
	var prev model.Conditions
	_ = json.Unmarshal(c.ProductSnapshot, &prev)
	return s.storeQuote(ctx, orgID, spec, p, storeExtra{difference: &diff, previous: &prev, vehicleID: vehicleID})
}

// extensionPricing prices an extension: the whole new range is priced to
// pick the band of the NEW total length, then only the added days (index >
// the current billing days) are charged, each with its own date rule.
// Extras are charged for the added days only (new extras total − what the
// reservation already pays for extras). Discounts are not applied to the
// added days.
func (s *Service) extensionPricing(ctx context.Context, orgID int64, c db.GetReservationCoreRow, w availmodel.Where, newEnd time.Time, snap model.PriceSnapshot) (priced, model.Extension, quoteSpec, error) {
	full := w
	full.EndsAt = newEnd.UTC()
	spec := quoteSpec{purpose: model.PurposeExtension, reservationID: c.ID, where: full, productUUID: c.ProductUuid, extras: snap.Extras}
	p, err := s.price(ctx, orgID, spec)
	if err != nil {
		return p, model.Extension{}, spec, err
	}
	b := p.side.Breakdown
	oldRange, err := rentaltime.NewRange(w.StartsAt, w.EndsAt)
	if err != nil {
		return p, model.Extension{}, spec, err
	}
	oldDays := oldRange.BillingDays()
	ext := model.Extension{
		PreviousEndsAt: w.EndsAt, EndsAt: full.EndsAt, PreviousBillingDays: oldDays, BillingDays: b.BillingDays,
		Band: b.Band, Days: []pricing.DayLine{}, RentalAmount: money.Zero(), ExtrasAmount: money.Zero(),
	}
	if b.BillingDays > oldDays {
		ext.Days = append(ext.Days, b.Days[oldDays:]...)
	}
	ext.AddedDays = len(ext.Days)
	for _, d := range ext.Days {
		ext.RentalAmount = ext.RentalAmount.Add(d.UnitPrice)
	}
	newExtras, oldExtras := money.Zero(), money.Zero()
	for _, l := range b.Lines {
		if l.Kind == pricing.LineExtra {
			newExtras = newExtras.Add(l.Gross)
		}
	}
	if snap.Pricing != nil {
		for _, l := range snap.Pricing.Lines {
			if l.Kind == pricing.LineExtra {
				oldExtras = oldExtras.Add(l.Gross)
			}
		}
	}
	for _, e := range snap.Extensions {
		oldExtras = oldExtras.Add(e.ExtrasAmount)
	}
	ext.ExtrasAmount = newExtras.Sub(oldExtras).ClampZero()
	rb, rv := pricing.SplitVAT(ext.RentalAmount, b.VATRate)
	eb, ev := pricing.SplitVAT(ext.ExtrasAmount, b.VATRate)
	ext.Base, ext.VAT = rb.Add(eb), rv.Add(ev)
	ext.Amount = ext.RentalAmount.Add(ext.ExtrasAmount)
	return p, ext, spec, nil
}

// extensionQuote stores an extension quote (total = the extension charge).
func (s *Service) extensionQuote(ctx context.Context, orgID int64, c db.GetReservationCoreRow, w availmodel.Where, newEnd time.Time, snap model.PriceSnapshot, vehicleID int64) (uuid.UUID, model.Extension, error) {
	p, ext, spec, err := s.extensionPricing(ctx, orgID, c, w, newEnd, snap)
	if err != nil {
		return uuid.Nil, ext, err
	}
	var prev model.Conditions
	_ = json.Unmarshal(c.ProductSnapshot, &prev)
	diff := ext.Amount
	id, err := s.storeQuote(ctx, orgID, spec, p, storeExtra{
		difference: &diff, extension: &ext, vehicleID: vehicleID, previous: &prev,
		override: &quoteTotals{
			rentalSubtotal: ext.RentalAmount, rentalTotal: ext.RentalAmount, extras: ext.ExtrasAmount, base: ext.Base, vat: ext.VAT,
			total: ext.Amount, prepayment: money.Zero(), billingDays: ext.BillingDays,
		},
	})
	return id, ext, err
}

// ApplyChange applies a change / extension / upgrade quote of the
// reservation. The quote is re-validated (open, not expired, same price
// now); capacity moves with the engine atomically (ChangeDates /
// ChangeProduct keep the old plan when no new one exists). A refused
// extension answers EXTENSION_CONFLICT with alternatives (RT-A31).
func (s *Service) ApplyChange(ctx context.Context, orgID int64, id uuid.UUID, in model.ApplyChangeInput) (model.ChangeResult, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.ChangeResult{}, err
	}
	if in.QuoteUUID == nil {
		return model.ChangeResult{}, validation("quote_uuid", "required", "quote_uuid is required")
	}
	if err := changeable(c); err != nil {
		return model.ChangeResult{}, err
	}
	if in.KeepPrice && !has(ctx, rbac.PermTenantPriceOverride) {
		return model.ChangeResult{}, ErrForbidden
	}
	if len(in.Reason) > 500 {
		return model.ChangeResult{}, validation("reason", "too_long", "reason is at most 500 characters")
	}
	row, err := s.loadQuote(ctx, orgID, *in.QuoteUUID)
	if errors.Is(err, ErrQuoteNotFound) || (err == nil && (!row.ReservationID.Valid || row.ReservationID.Int64 != c.ID)) {
		return model.ChangeResult{}, validation("quote_uuid", "not_found", "quote was not found for this reservation")
	}
	if err != nil {
		return model.ChangeResult{}, err
	}
	switch {
	case row.Status != model.QuoteOpen:
		return model.ChangeResult{}, &QuoteError{Code: "QUOTE_USED", Message: "the quote was already used"}
	case !row.ValidUntil.Time.After(s.clock()):
		return model.ChangeResult{}, &QuoteError{Code: "QUOTE_EXPIRED", Message: "the quote has expired; create a new one"}
	}
	w, _, err := s.where(ctx, orgID, id)
	if err != nil {
		return model.ChangeResult{}, err
	}
	snap := snapshotOf(c.QuoteSnapshot)
	current := money.FromNumericOrZero(c.TotalAmount)
	var newSnap model.PriceSnapshot
	var diff money.Money
	newPrepay := money.FromNumericOrZero(c.PrepaymentAmount)
	if row.Purpose == model.PurposeExtension {
		_, ext, _, err := s.extensionPricing(ctx, orgID, c, w, row.EndsAt.Time, snap)
		if err != nil {
			return model.ChangeResult{}, err
		}
		if !ext.Amount.Equal(money.FromNumericOrZero(row.Total)) {
			return model.ChangeResult{}, s.freshExtension(ctx, orgID, c, w, row, snap)
		}
		diff = ext.Amount
		newSnap = snap
		newSnap.Extensions = append(newSnap.Extensions, ext)
		newSnap.Total = snap.Total.Add(ext.Amount)
		newSnap.BaseTotal = snap.BaseTotal.Add(ext.Base)
		newSnap.VATTotal = snap.VATTotal.Add(ext.VAT)
	} else {
		spec := specFromQuote(row, nil)
		spec.reservationID = c.ID
		p, err := s.price(ctx, orgID, spec)
		if err != nil {
			return model.ChangeResult{}, err
		}
		if p.item.Product.Version != row.ProductVersion || !p.side.Breakdown.Total.Equal(money.FromNumericOrZero(row.Total)) {
			fresh, err := s.changeQuote(ctx, orgID, c, spec, 0)
			if err != nil {
				return model.ChangeResult{}, err
			}
			s.q.MarkQuoteSuperseded(ctx, db.MarkQuoteSupersededParams{OrganizationID: orgID, ID: row.ID}) //nolint:errcheck // best effort
			q, err := s.GetQuote(ctx, orgID, fresh)
			if err != nil {
				return model.ChangeResult{}, err
			}
			return model.ChangeResult{}, &QuoteError{Code: "QUOTE_PRICE_CHANGED", Message: "the price changed since the quote", Replacement: &q}
		}
		q, err := s.GetQuote(ctx, orgID, row.Uuid)
		if err != nil {
			return model.ChangeResult{}, err
		}
		newSnap = priceSnapshot(q)
		diff = q.Total.Sub(current)
		newPrepay = q.Prepayment
	}
	if in.KeepPrice {
		newSnap = snap
		newSnap.KeptPrice = true
		diff = money.Zero()
		newPrepay = money.FromNumericOrZero(c.PrepaymentAmount)
	}
	newTotal := current.Add(diff)
	target := specFromQuote(row, nil).where
	var vehicle *uuid.UUID
	if row.VehicleUuid.Valid {
		v := uuid.UUID(row.VehicleUuid.Bytes)
		vehicle = &v
	}
	action := EventChanged
	source := model.SourceStaff
	switch row.Purpose {
	case model.PurposeExtension:
		action, source = EventExtended, model.SourceExtension
	case model.PurposeUpgrade:
		action, source = EventUpgraded, model.SourceUpgrade
	}
	commission := money.FromNumericOrZero(c.CommissionAmount)
	var newAlloc uuid.UUID
	reassigned := 0
	var touched []uuid.UUID
	hook := func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
		conds, err := s.conditionsByID(ctx, orgID, r.ProductID, r.ProductVersion)
		if err != nil {
			return err
		}
		n, err := q.UpdateReservationPlan(ctx, db.UpdateReservationPlanParams{
			OrganizationID: orgID, ID: c.ID, AllocationID: int8Of(r.AllocationID), StartsAt: row.StartsAt, EndsAt: row.EndsAt,
			PickupBranchID: row.PickupBranchID, ReturnBranchID: row.ReturnBranchID, PickupPointID: row.PickupPointID,
			ReturnPointID: row.ReturnPointID, RentalProductID: r.ProductID, ProductVersion: r.ProductVersion,
			GuaranteeType: conds.GuaranteeType, ProductSnapshot: mustJSON(conds), TotalAmount: newTotal.Numeric(),
			PrepaymentAmount: newPrepay.Numeric(), NetReceivable: newTotal.Sub(commission).Numeric(), QuoteID: int8Of(row.ID),
			QuoteSnapshot: mustJSON(newSnap),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConcurrent
		}
		used, err := q.MarkQuoteUsed(ctx, db.MarkQuoteUsedParams{OrganizationID: orgID, ID: row.ID, UsedAt: ts(s.clock())})
		if err != nil {
			return err
		}
		if used != 1 {
			return &QuoteError{Code: "QUOTE_USED", Message: "the quote was already used"}
		}
		if err := event(ctx, q, orgID, c.ID, action, "", "", strings.TrimSpace(in.Reason), map[string]any{
			"quote_uuid": row.Uuid, "difference": diff.String(), "total": newTotal.String(), "kept_price": in.KeepPrice,
			"from": map[string]any{"starts_at": c.StartsAt.Time.UTC(), "ends_at": c.EndsAt.Time.UTC(), "product_uuid": c.ProductUuid},
			"to":   map[string]any{"starts_at": row.StartsAt.Time.UTC(), "ends_at": row.EndsAt.Time.UTC(), "product_uuid": row.ProductUuid},
		}); err != nil {
			return err
		}
		if r.VehicleID != c.AllocationVehicleID.Int64 || r.ProductID != c.RentalProductID || r.AllocationID != c.AllocationID.Int64 {
			if err := assignment(ctx, q, orgID, c.ID, r.VehicleID, r.ProductID, r.ProductVersion, r.PlateFixed, source, strings.TrimSpace(in.Reason)); err != nil {
				return err
			}
		}
		newAlloc = r.AllocationUUID
		reassigned = len(r.Moved)
		t, err := s.planEffects(ctx, q, orgID, r, "reservation", c.Uuid)
		touched = t
		return err
	}
	alloc := uuid.UUID(c.AllocationUuid.Bytes)
	actx := unscoped(ctx)
	if row.RentalProductID == c.RentalProductID {
		_, err = s.avail.ChangeDates(actx, orgID, alloc, availmodel.ChangeInput{
			Where: target, VehicleUUID: vehicle, FixPlate: vehicle != nil, Hook: hookOf(hook, s),
		})
	} else {
		_, err = s.avail.ChangeProduct(actx, orgID, alloc, row.ProductUuid, availmodel.ChangeInput{Where: target, Hook: hookOf(hook, s)})
		if errors.Is(err, availusecase.ErrOtherComponent) {
			err = s.reserveThenRelease(ctx, orgID, c, row, target, hook)
		}
	}
	if err != nil {
		if _, ok := capacityErr(err); ok && row.Purpose == model.PurposeExtension {
			return model.ChangeResult{}, s.extensionConflict(ctx, orgID, c, w, row, snap)
		}
		return model.ChangeResult{}, mapEngineErr(err)
	}
	result := model.ChangeResult{Reassigned: reassigned, Warnings: []string{}}
	if !diff.IsZero() {
		result.Difference = &diff
	}
	datesMoved := !row.StartsAt.Time.Equal(c.StartsAt.Time) || !row.EndsAt.Time.Equal(c.EndsAt.Time)
	if newAlloc != alloc || datesMoved || !sameExtras(newSnap.Extras, snap.Extras) {
		result.Warnings = append(result.Warnings, s.resyncEquipment(ctx, orgID, c.ID, c.EquipmentAllocations, newSnap.Extras, newAlloc, c.Uuid)...)
	}
	s.publish(ctx, model.ResourceReservations, "updated", append(touched, c.Uuid)...)
	s.publish(ctx, model.ResourceQuotes, "updated", row.Uuid)
	res, err := s.Get(ctx, orgID, id)
	result.Reservation = res
	return result, err
}

// reserveThenRelease upgrades to a product that shares no vehicles with
// the current one: the new unit is secured first (same kind, same option
// expiry; the reservation row moves to it in that transaction), then the
// old allocation is released. At worst both units are held for a moment;
// the customer never loses the car.
func (s *Service) reserveThenRelease(ctx context.Context, orgID int64, c db.GetReservationCoreRow, row db.GetQuoteRow, target availmodel.Where, hook func(context.Context, *db.Queries, availmodel.HookResult) error) error {
	kind := availmodel.KindReservation
	var until *time.Time
	if c.AllocationKind.String == availmodel.KindHold {
		kind = availmodel.KindHold
		until = timePtr(c.AllocationExpiresAt)
	}
	ref := c.Uuid
	_, err := s.avail.Reserve(unscoped(ctx), orgID, availmodel.ReserveInput{
		Where: target, ProductUUID: row.ProductUuid, Kind: kind, HoldUntil: until, RefType: model.RefType, RefUUID: &ref,
		ActorID: actorID(ctx), Note: "reservation upgrade", Hook: hookOf(hook, s),
	})
	if err != nil {
		return err
	}
	_, err = s.avail.ReleaseWith(unscoped(ctx), orgID, c.AllocationUuid.Bytes, "reservation_upgraded", actorID(ctx),
		hookOf(func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
			_, err := s.planEffects(ctx, q, orgID, r, "reservation", c.Uuid)
			return err
		}, s))
	if err != nil && !errors.Is(err, availusecase.ErrNotActive) {
		slog.Default().Error("reservation_upgrade_release_failed", "reservation", c.Uuid, "error", err)
	}
	return nil
}

// freshExtension supersedes a stale extension quote with a fresh one.
func (s *Service) freshExtension(ctx context.Context, orgID int64, c db.GetReservationCoreRow, w availmodel.Where, row db.GetQuoteRow, snap model.PriceSnapshot) error {
	vehicleID := int64(0)
	if row.VehicleID.Valid {
		vehicleID = row.VehicleID.Int64
	}
	fresh, _, err := s.extensionQuote(ctx, orgID, c, w, row.EndsAt.Time, snap, vehicleID)
	if err != nil {
		return err
	}
	s.q.MarkQuoteSuperseded(ctx, db.MarkQuoteSupersededParams{OrganizationID: orgID, ID: row.ID}) //nolint:errcheck // best effort
	q, err := s.GetQuote(ctx, orgID, fresh)
	if err != nil {
		return err
	}
	return &QuoteError{Code: "QUOTE_PRICE_CHANGED", Message: "the price changed since the quote", Replacement: &q}
}

// extensionConflict builds the alternatives of a refused extension
// (RT-A31): other vehicles that can carry the whole extended range (same
// product: the extension quote's price, with the plate moved) and other
// products sellable for the extended range (upgrade quotes with their
// difference). A resolution task records the refused request.
func (s *Service) extensionConflict(ctx context.Context, orgID int64, c db.GetReservationCoreRow, w availmodel.Where, row db.GetQuoteRow, snap model.PriceSnapshot) error {
	alts := model.ExtensionAlternatives{Reason: "extension_conflict", Vehicles: []model.VehicleAlternative{}, Products: []model.ProductAlternative{}}
	if q, err := s.GetQuote(ctx, orgID, row.Uuid); err == nil {
		alts.ExtensionQuote = &q
	}
	newEnd := row.EndsAt.Time.UTC()
	extended := w
	extended.EndsAt = newEnd
	vehicles, err := s.avail.CandidateVehicles(unscoped(ctx), orgID, c.AllocationUuid.Bytes, extended, 5)
	if err != nil {
		return mapEngineErr(err)
	}
	for _, v := range vehicles {
		vrow, err := s.q.GetReservationVehicleByUUID(ctx, db.GetReservationVehicleByUUIDParams{OrganizationID: orgID, Uuid: v.UUID})
		if err != nil {
			continue
		}
		qid, _, err := s.extensionQuote(ctx, orgID, c, w, newEnd, snap, vrow.ID)
		if err != nil {
			continue
		}
		q, err := s.GetQuote(ctx, orgID, qid)
		if err != nil {
			continue
		}
		alts.Vehicles = append(alts.Vehicles, model.VehicleAlternative{Vehicle: model.VehicleRef{UUID: v.UUID, Plate: v.Plate, Label: vrow.Label}, Quote: q})
	}
	if res, err := s.avail.Search(unscoped(ctx), orgID, availmodel.SearchInput{Where: extended}); err == nil {
		for _, item := range res.Items {
			if item.Product.UUID == c.ProductUuid || item.Sellable < 1 || len(alts.Products) >= 5 {
				continue
			}
			qid, err := s.changeQuote(ctx, orgID, c, quoteSpec{
				purpose: model.PurposeUpgrade, reservationID: c.ID, where: extended, productUUID: item.Product.UUID, extras: snap.Extras,
			}, 0)
			if err != nil {
				continue
			}
			q, err := s.GetQuote(ctx, orgID, qid)
			if err != nil {
				continue
			}
			alts.Products = append(alts.Products, model.ProductAlternative{Product: item.Product, Sellable: item.Sellable, Quote: q})
		}
	}
	_ = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		return s.openTask(ctx, q, orgID, c.ID, model.TaskExtensionConflict,
			"the requested extension collides with another commitment", "quote", row.Uuid, map[string]any{
				"ends_at": newEnd, "vehicle_alternatives": len(alts.Vehicles), "product_alternatives": len(alts.Products),
			})
	})
	s.publish(ctx, model.ResourceTasks, "created", c.Uuid)
	return &ExtensionConflictError{Alternatives: alts}
}

// Extend prices and applies an extension in one call (the new return time
// on the same vehicle plan, or a re-plan that keeps the product). A
// collision answers EXTENSION_CONFLICT with alternatives and difference
// quotes (RT-A31); the reservation stays as it was.
func (s *Service) Extend(ctx context.Context, orgID int64, id uuid.UUID, in model.ExtendInput) (model.ChangeResult, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.ChangeResult{}, err
	}
	if err := changeable(c); err != nil {
		return model.ChangeResult{}, err
	}
	if in.EndsAt == nil {
		return model.ChangeResult{}, validation("ends_at", "required", "ends_at is required")
	}
	if !in.EndsAt.After(c.EndsAt.Time) {
		return model.ChangeResult{}, validation("ends_at", "not_later", "an extension must end after the current return time")
	}
	w, _, err := s.where(ctx, orgID, id)
	if err != nil {
		return model.ChangeResult{}, err
	}
	qid, _, err := s.extensionQuote(ctx, orgID, c, w, in.EndsAt.UTC(), snapshotOf(c.QuoteSnapshot), 0)
	if err != nil {
		return model.ChangeResult{}, err
	}
	return s.ApplyChange(ctx, orgID, id, model.ApplyChangeInput{QuoteUUID: &qid, Reason: in.Reason})
}

// PlateCandidates lists the vehicles (other than the planned one) the
// engine could move the reservation to as it is: each one checked with the
// planner, read-only. The plate dialog offers only these.
func (s *Service) PlateCandidates(ctx context.Context, orgID int64, id uuid.UUID) ([]model.VehicleRef, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if err := changeable(c); err != nil {
		return nil, err
	}
	w, _, err := s.where(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	vehicles, err := s.avail.CandidateVehicles(unscoped(ctx), orgID, c.AllocationUuid.Bytes, w, 25)
	if err != nil {
		return nil, mapEngineErr(err)
	}
	out := make([]model.VehicleRef, 0, len(vehicles))
	for _, v := range vehicles {
		ref := model.VehicleRef{UUID: v.UUID, Plate: v.Plate, Label: v.Label}
		if row, err := s.q.GetReservationVehicleByUUID(ctx, db.GetReservationVehicleByUUIDParams{OrganizationID: orgID, Uuid: v.UUID}); err == nil {
			ref.Label = row.Label
		}
		out = append(out, ref)
	}
	return out, nil
}

// Plate changes the vehicle plan of a reservation: fix promises the plate
// (staff "plaka ata"), otherwise the vehicle becomes the new provisional
// plate (calendar drag). The engine re-validates the whole plan (the
// vehicle must keep the sold guarantee — a higher product is an upgrade —
// and later commitments are moved or the change is refused).
func (s *Service) Plate(ctx context.Context, orgID int64, id uuid.UUID, in model.PlateInput) (model.ChangeResult, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.ChangeResult{}, err
	}
	if in.VehicleUUID == nil {
		return model.ChangeResult{}, validation("vehicle_uuid", "required", "vehicle_uuid is required")
	}
	if err := changeable(c); err != nil {
		return model.ChangeResult{}, err
	}
	source := model.SourceStaff
	if in.Source == model.SourceDrag {
		source = model.SourceDrag
	} else if in.Source != "" && in.Source != model.SourceStaff {
		return model.ChangeResult{}, validation("source", "invalid", "source must be staff or drag")
	}
	if len(in.Reason) > 500 {
		return model.ChangeResult{}, validation("reason", "too_long", "reason is at most 500 characters")
	}
	from, _ := s.q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: c.AllocationVehicleID.Int64})
	reassigned := 0
	var touched []uuid.UUID
	_, err = s.avail.MovePlan(unscoped(ctx), orgID, c.AllocationUuid.Bytes, availmodel.PlanOptions{
		VehicleUUID: in.VehicleUUID, FixPlate: in.Fix,
		Hook: hookOf(func(ctx context.Context, q *db.Queries, r availmodel.HookResult) error {
			to, _ := q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: r.VehicleID})
			if err := event(ctx, q, orgID, c.ID, EventPlateChanged, "", "", strings.TrimSpace(in.Reason), map[string]any{
				"from_plate": from.Plate, "to_plate": to.Plate, "fixed": r.PlateFixed, "source": source,
			}); err != nil {
				return err
			}
			if err := assignment(ctx, q, orgID, c.ID, r.VehicleID, r.ProductID, r.ProductVersion, r.PlateFixed, source, strings.TrimSpace(in.Reason)); err != nil {
				return err
			}
			reassigned = len(r.Moved)
			t, err := s.planEffects(ctx, q, orgID, r, "reservation", c.Uuid)
			touched = t
			return err
		}, s),
	})
	if err != nil {
		return model.ChangeResult{}, mapEngineErr(err)
	}
	s.publish(ctx, model.ResourceReservations, "updated", append(touched, c.Uuid)...)
	res, err := s.Get(ctx, orgID, id)
	return model.ChangeResult{Reservation: res, Reassigned: reassigned, Warnings: []string{}}, err
}

// UpdateDrivers replaces the drivers of a reservation (checked against the
// current product version's conditions on the pick-up day).
func (s *Service) UpdateDrivers(ctx context.Context, orgID int64, id uuid.UUID, in model.DriversInput) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	if model.IsTerminal(c.Status) {
		return model.Reservation{}, stateErr("terminal", "the reservation is closed")
	}
	var conds model.Conditions
	_ = json.Unmarshal(c.ProductSnapshot, &conds)
	drivers, err := s.checkDrivers(ctx, orgID, in.Drivers, &conds, c.StartsAt.Time, in.EligibilityOverride)
	if err != nil {
		return model.Reservation{}, err
	}
	err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		if _, err := q.LockReservation(ctx, db.LockReservationParams{OrganizationID: orgID, ID: c.ID}); err != nil {
			return err
		}
		if err := q.DeleteReservationDrivers(ctx, db.DeleteReservationDriversParams{OrganizationID: orgID, ReservationID: c.ID}); err != nil {
			return err
		}
		if err := writeDrivers(ctx, q, orgID, c.ID, drivers, in.EligibilityOverride); err != nil {
			return err
		}
		data := map[string]any{"count": len(drivers)}
		if o := overrideJSON(drivers, in.EligibilityOverride, actorID(ctx)); o != nil {
			data["eligibility_override"] = strings.TrimSpace(in.EligibilityOverride.Reason)
		}
		return event(ctx, q, orgID, c.ID, EventDriversChanged, "", "", "", data)
	})
	if err != nil {
		return model.Reservation{}, err
	}
	return s.after(ctx, orgID, id)
}

// IsMember reports whether the user is a member of the organization.
func (s *Service) IsMember(ctx context.Context, orgID int64, userUUID uuid.UUID) bool {
	_, err := s.q.GetReservationUserInOrganization(ctx, db.GetReservationUserInOrganizationParams{OrganizationID: orgID, Uuid: userUUID})
	return err == nil
}

// Assign sets (or clears) the staff member responsible for a reservation.
func (s *Service) Assign(ctx context.Context, orgID int64, id uuid.UUID, userUUID *uuid.UUID) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	var userID *int64
	name := ""
	if userUUID != nil {
		u, err := s.q.GetReservationUserInOrganization(ctx, db.GetReservationUserInOrganizationParams{OrganizationID: orgID, Uuid: *userUUID})
		if notFound(err) {
			return model.Reservation{}, validation("user_uuid", "not_found", "member was not found")
		}
		if err != nil {
			return model.Reservation{}, err
		}
		userID = &u.ID
		name = strings.TrimSpace(u.Name + " " + u.Surname)
	}
	err = s.tx(ctx, func(q *db.Queries, _ pgxTx) error {
		if _, err := q.SetReservationAssignee(ctx, db.SetReservationAssigneeParams{OrganizationID: orgID, ID: c.ID, AssignedUserID: int8Ptr(userID)}); err != nil {
			return err
		}
		return event(ctx, q, orgID, c.ID, EventAssigned, "", "", "", map[string]any{"user_uuid": userUUID, "name": name})
	})
	if err != nil {
		return model.Reservation{}, err
	}
	return s.after(ctx, orgID, id)
}
