package model

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Fee is a computed charge: its amount and how it was computed (the
// calculation is stored with the charge).
type Fee struct {
	Amount      money.Money
	Calculation map[string]any
}

// LateFee computes the policy late fee: beyond the tolerance of the
// reservation's policy version, every started hour since the planned
// return costs the policy's extra hour fee. ok is false when no fee
// applies (inside the tolerance, or no fee defined).
func LateFee(p Policy, lateMinutes int32, plannedReturn, actualReturn time.Time) (Fee, bool) {
	if lateMinutes <= int32(p.Grace()) || p.ExtraHourFee == "" {
		return Fee{}, false
	}
	fee, err := money.Parse(p.ExtraHourFee)
	if err != nil || !fee.IsPositive() {
		return Fee{}, false
	}
	hours := int64((lateMinutes + 59) / 60)
	return Fee{Amount: fee.Mul(hours), Calculation: map[string]any{
		"late_minutes": lateMinutes, "grace_minutes": p.Grace(), "hours": hours, "extra_hour_fee": fee.String(),
		"settings_version": p.SettingsVersion, "planned_return_at": plannedReturn.UTC(), "actual_return_at": actualReturn.UTC(),
		"rule": "every started hour after the planned return, when the delay exceeds the tolerance",
	}}, true
}

// Conditions are the product conditions of the reservation's product
// snapshot that return charges use.
type Conditions struct {
	IncludedKmPerDay *int32 `json:"included_km_per_day"`
	ExtraKmFee       string `json:"extra_km_fee"`
	FuelPolicy       string `json:"fuel_policy"`
	DepositAmount    string `json:"deposit_amount"`
}

// ConditionsFromSnapshot reads the reservation product snapshot.
func ConditionsFromSnapshot(raw []byte) Conditions {
	var c Conditions
	_ = json.Unmarshal(raw, &c)
	c.ExtraKmFee = strings.TrimSpace(c.ExtraKmFee)
	c.FuelPolicy = strings.TrimSpace(c.FuelPolicy)
	c.DepositAmount = strings.TrimSpace(c.DepositAmount)
	return c
}

// RentalDays is the number of started days of the planned period (at
// least one).
func RentalDays(plannedPickup, plannedReturn time.Time) int64 {
	days := int64(math.Ceil(plannedReturn.Sub(plannedPickup).Hours() / 24))
	if days < 1 {
		return 1
	}
	return days
}

// MileageFee computes the km overage: driven km beyond included km per day
// × planned days costs the product's extra km fee. ok is false when the
// product has no limit or fee, or the limit was kept.
func MileageFee(c Conditions, settingsVersion int, drivenKm int64, days int64) (Fee, bool) {
	if c.IncludedKmPerDay == nil || *c.IncludedKmPerDay <= 0 || c.ExtraKmFee == "" {
		return Fee{}, false
	}
	fee, err := money.Parse(c.ExtraKmFee)
	if err != nil || !fee.IsPositive() {
		return Fee{}, false
	}
	allowed := int64(*c.IncludedKmPerDay) * days
	over := drivenKm - allowed
	if over <= 0 {
		return Fee{}, false
	}
	return Fee{Amount: fee.Mul(over), Calculation: map[string]any{
		"driven_km": drivenKm, "included_km_per_day": *c.IncludedKmPerDay, "days": days, "allowed_km": allowed,
		"extra_km": over, "extra_km_fee": fee.String(), "settings_version": settingsVersion,
		"rule": "driven km beyond the included km per planned day",
	}}, true
}

// FuelFee computes the refuelling fee: the tank level the product's fuel
// policy expects back (full_to_full: 100, same_to_same: the level at the
// handover) minus the returned level, in percent, times the policy's fee
// per percent. prepaid / full_to_empty policies compute nothing.
func FuelFee(c Conditions, p Policy, outFuel, inFuel int) (Fee, bool) {
	if p.FuelFeePerPercent == "" {
		return Fee{}, false
	}
	fee, err := money.Parse(p.FuelFeePerPercent)
	if err != nil || !fee.IsPositive() {
		return Fee{}, false
	}
	expected := outFuel
	switch c.FuelPolicy {
	case "full_to_full":
		expected = 100
	case "prepaid", "full_to_empty":
		return Fee{}, false
	}
	missing := expected - inFuel
	if missing <= 0 {
		return Fee{}, false
	}
	return Fee{Amount: fee.Mul(int64(missing)), Calculation: map[string]any{
		"fuel_policy": c.FuelPolicy, "out_fuel": outFuel, "in_fuel": inFuel, "expected_fuel": expected,
		"missing_percent": missing, "fee_per_percent": fee.String(), "settings_version": p.SettingsVersion,
		"rule": "missing tank percent against the fuel policy",
	}}, true
}
