package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Charges computes the charge proposal of a return form from the
// reservation's policy snapshot and product conditions (late return with
// the rentals rule, km overage, fuel difference, missing priced
// equipment) plus the staff amounts of new damages. Staff overrides
// replace computed amounts (the computed one stays visible). Nothing is
// stored until the form is finalized.
func (s *Service) Charges(ctx context.Context, orgID int64, id uuid.UUID) (model.ChargePreview, error) {
	row, r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.ChargePreview{}, err
	}
	cur, err := s.view(ctx, orgID, row, r)
	if err != nil {
		return model.ChargePreview{}, err
	}
	return s.preview(ctx, orgID, row, cur)
}

func (s *Service) preview(ctx context.Context, orgID int64, row db.GetInspectionRow, cur model.Inspection) (model.ChargePreview, error) {
	out := model.ChargePreview{Lines: []model.ChargeLine{}, Total: money.Zero()}
	facts, err := s.q.GetInspectionRentalFacts(ctx, db.GetInspectionRentalFactsParams{OrganizationID: orgID, Uuid: cur.Rental.UUID})
	if err != nil {
		return out, err
	}
	out.Currency = strings.TrimSpace(facts.Currency)
	if out.Currency == "" {
		out.Currency = "TRY"
	}
	policy := rentalmodel.PolicyFromSnapshot(facts.PolicySnapshot)
	out.SettingsVersion = policy.SettingsVersion
	if row.Kind != model.KindReturn {
		return out, nil
	}
	cond := rentalmodel.ConditionsFromSnapshot(facts.ProductSnapshot)
	overrides := map[string]model.ChargeOverride{}
	for _, o := range cur.ChargeOverrides {
		overrides[o.Key] = o
	}
	add := func(key, kind, source, reason string, fee *rentalmodel.Fee, disputed bool) {
		line := model.ChargeLine{Key: key, Kind: kind, Source: source, Currency: out.Currency, Reason: reason,
			Disputed: disputed || cur.Disputed, Calculation: json.RawMessage("{}")}
		if fee != nil {
			line.Amount = fee.Amount
			computed := fee.Amount
			line.Computed = &computed
			line.Calculation = mustJSON(fee.Calculation)
		}
		if o, ok := overrides[key]; ok {
			if fee != nil && !o.Amount.Equal(fee.Amount) {
				line.Overridden = true
			}
			line.Amount = o.Amount
			if o.Reason != "" {
				line.Reason = o.Reason
			}
		}
		out.Lines = append(out.Lines, line)
		out.Total = out.Total.Add(line.Amount)
	}

	// Late return: the rentals rule on the policy snapshot.
	at := s.clock()
	if cur.OccurredAt != nil {
		at = *cur.OccurredAt
	}
	planned := facts.PlannedReturnAt.Time
	if at.After(planned) {
		out.LateMinutes = int32(at.Sub(planned) / time.Minute)
	}
	if fee, ok := rentalmodel.LateFee(policy, out.LateMinutes, planned, at); ok {
		add("late_return", rentalmodel.ChargeLateReturn, rentalmodel.SourcePolicy, "late return", &fee, false)
	} else if _, ok := overrides["late_return"]; ok {
		add("late_return", rentalmodel.ChargeLateReturn, rentalmodel.SourcePolicy, "late return", nil, false)
	}

	// Km overage and fuel difference against the open leg.
	legs, err := s.q.GetRentalLegKm(ctx, db.GetRentalLegKmParams{OrganizationID: orgID, RentalID: row.RentalID})
	if err != nil {
		return out, err
	}
	out.DrivenKm = legs.DrivenKm
	if cur.Km != nil && legs.OpenOutKm > 0 && *cur.Km >= legs.OpenOutKm {
		out.DrivenKm += int64(*cur.Km - legs.OpenOutKm)
	}
	days := rentalmodel.RentalDays(facts.PlannedPickupAt.Time, planned)
	if fee, ok := rentalmodel.MileageFee(cond, policy.SettingsVersion, out.DrivenKm, days); ok {
		add("mileage", rentalmodel.ChargeMileage, rentalmodel.SourcePolicy, "km overage", &fee, false)
	}
	if cur.Fuel != nil && legs.OpenOutFuel >= 0 {
		if fee, ok := rentalmodel.FuelFee(cond, policy, int(legs.OpenOutFuel), int(*cur.Fuel)); ok {
			add("fuel", rentalmodel.ChargeFuel, rentalmodel.SourcePolicy, "fuel difference", &fee, false)
		}
	}

	// Equipment present at the handover and missing now, at its price.
	var handed map[string]bool
	if cur.Baseline != nil {
		handed = cur.Baseline.Equipment
	}
	for _, it := range cur.Template.Checklist {
		present, answered := cur.Equipment[it.Key]
		if !answered || present || (handed != nil && !handed[it.Key]) {
			continue
		}
		price, err := money.Parse(it.Price)
		if it.Price == "" || err != nil || !price.IsPositive() {
			continue
		}
		fee := rentalmodel.Fee{Amount: price, Calculation: map[string]any{
			"item": it.Key, "label": it.Label, "price": price.String(), "template_version": cur.Template.Version,
			"rule": "an item handed over and missing at the return costs its template price",
		}}
		add("equipment:"+it.Key, rentalmodel.ChargeEquipment, rentalmodel.SourcePolicy, "missing equipment: "+it.Label, &fee, false)
	}

	// New damages: the amount staff entered (none = to be assessed later).
	for _, d := range cur.Damages {
		if !d.IsNew {
			continue
		}
		key := "damage:" + d.UUID.String()
		reason := "damage: " + d.Zone
		if d.Description != "" {
			reason += " - " + d.Description
		}
		if len(reason) > 480 {
			reason = reason[:480]
		}
		line := model.ChargeLine{Key: key, Kind: rentalmodel.ChargeDamage, Source: rentalmodel.SourceManual, Currency: out.Currency,
			Reason: reason, Disputed: d.Disputed || cur.Disputed, Amount: money.Zero(), Calculation: mustJSON(map[string]any{
				"damage_uuid": d.UUID, "zone": d.Zone, "damage_type": d.DamageType, "severity": d.Severity,
			})}
		if o, ok := overrides[key]; ok {
			line.Amount = o.Amount
			if o.Reason != "" {
				line.Reason = o.Reason
			}
		}
		out.Lines = append(out.Lines, line)
		out.Total = out.Total.Add(line.Amount)
	}
	return out, nil
}
