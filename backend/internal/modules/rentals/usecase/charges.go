package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// AddCharge proposes a manual charge (damage, fuel, mileage, early return
// adjustment, other) with a reason. Like policy fees it is only a proposal
// until an authorized person approves it.
func (s *Service) AddCharge(ctx context.Context, orgID int64, id uuid.UUID, in model.ChargeInput) (model.Rental, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Rental{}, err
	}
	if c.Status == model.StatusCancelled || c.Status == model.StatusPreparation {
		return model.Rental{}, stateErr("invalid_state", "charges are recorded after the handover")
	}
	ve := &ValidationError{}
	if !model.Contains(model.ChargeKinds, in.Kind) {
		ve.add("kind", "invalid", "unknown charge kind")
	}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	reason, rerr := requireReason("reason", in.Reason)
	if rerr != nil {
		ve.add("reason", "required", "a reason is required (3-500 characters)")
	}
	if err := ve.orNil(); err != nil {
		return model.Rental{}, err
	}
	policy := model.PolicyFromSnapshot(c.PolicySnapshot)
	currency := strings.TrimSpace(c.Currency)
	if currency == "" {
		currency = "TRY"
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		chargeID, err := q.InsertRentalCharge(ctx, db.InsertRentalChargeParams{
			OrganizationID: orgID, RentalID: c.ID, Kind: in.Kind, Source: model.SourceManual, Currency: currency,
			Amount: in.Amount.Numeric(), SettingsVersion: int32(policy.SettingsVersion), Calculation: []byte("{}"),
			Reason: reason, CreatedBy: int8Ptr(actorID(ctx)),
		})
		if err != nil {
			return err
		}
		return event(ctx, q, orgID, c.ID, model.EventChargeProposed, "", "", reason, map[string]any{
			"charge_uuid": chargeID, "kind": in.Kind, "amount": in.Amount.String(), "source": model.SourceManual,
		})
	})
	if err != nil {
		return model.Rental{}, err
	}
	return s.after(ctx, orgID, id)
}

// DecideCharge approves or waives a proposed charge. Approving a damage
// charge needs tenant.damage.approve, any other approval and every waiver
// tenant.rentals.close; a waiver needs a reason.
func (s *Service) DecideCharge(ctx context.Context, orgID int64, id, chargeID uuid.UUID, approve bool, note string) (model.Rental, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Rental{}, err
	}
	ch, err := s.q.GetRentalCharge(ctx, db.GetRentalChargeParams{OrganizationID: orgID, RentalID: c.ID, Uuid: chargeID})
	if notFound(err) {
		return model.Rental{}, ErrChargeNotFound
	}
	if err != nil {
		return model.Rental{}, err
	}
	switch {
	case approve && ch.Kind == model.ChargeDamage:
		if !has(ctx, rbac.PermTenantDamageApprove) {
			return model.Rental{}, ErrForbidden
		}
	case !has(ctx, rbac.PermTenantRentalsClose):
		return model.Rental{}, ErrForbidden
	}
	note = strings.TrimSpace(note)
	if len(note) > 500 {
		return model.Rental{}, validation("note", "too_long", "note is at most 500 characters")
	}
	status, action := model.ChargeApproved, model.EventChargeApproved
	if !approve {
		if _, err := requireReason("note", note); err != nil {
			return model.Rental{}, err
		}
		status, action = model.ChargeWaived, model.EventChargeWaived
	}
	if ch.Status != model.ChargeProposed {
		return model.Rental{}, stateErr("already_decided", "the charge was already decided")
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.DecideRentalCharge(ctx, db.DecideRentalChargeParams{
			OrganizationID: orgID, ID: ch.ID, Status: status, DecidedBy: int8Ptr(actorID(ctx)), DecidedAt: ts(s.clock()), DecisionNote: note,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConcurrent
		}
		return event(ctx, q, orgID, c.ID, action, "", "", note, map[string]any{"charge_uuid": ch.Uuid, "kind": ch.Kind})
	})
	if err != nil {
		return model.Rental{}, err
	}
	return s.after(ctx, orgID, id)
}
