package model

import (
	"testing"
	"time"
)

func TestFees(t *testing.T) {
	t.Parallel()
	grace := 30
	p := Policy{SettingsVersion: 3, LateReturnGraceMinutes: &grace, ExtraHourFee: "150", FuelFeePerPercent: "10"}
	planned := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	if _, ok := LateFee(p, 30, planned, planned.Add(30*time.Minute)); ok {
		t.Fatal("inside the tolerance no fee applies")
	}
	if fee, ok := LateFee(p, 61, planned, planned.Add(61*time.Minute)); !ok || fee.Amount.String() != "300.00" {
		t.Fatalf("61 minutes late = %v %v", fee.Amount, ok)
	}

	km := int32(100)
	c := Conditions{IncludedKmPerDay: &km, ExtraKmFee: "2.5", FuelPolicy: "same_to_same"}
	if days := RentalDays(planned, planned.Add(49*time.Hour)); days != 3 {
		t.Fatalf("days = %d", days)
	}
	if _, ok := MileageFee(c, 3, 300, 3); ok {
		t.Fatal("the allowance was kept")
	}
	if fee, ok := MileageFee(c, 3, 340, 3); !ok || fee.Amount.String() != "100.00" {
		t.Fatalf("40 km over = %v", fee.Amount)
	}
	if _, ok := MileageFee(Conditions{ExtraKmFee: "2"}, 3, 9999, 1); ok {
		t.Fatal("unlimited km computes nothing")
	}

	if fee, ok := FuelFee(c, p, 80, 60); !ok || fee.Amount.String() != "200.00" {
		t.Fatalf("same_to_same 80 → 60 = %v", fee.Amount)
	}
	full := c
	full.FuelPolicy = "full_to_full"
	if fee, ok := FuelFee(full, p, 80, 90); !ok || fee.Amount.String() != "100.00" {
		t.Fatalf("full_to_full → 90 = %v", fee.Amount)
	}
	prepaid := c
	prepaid.FuelPolicy = "prepaid"
	if _, ok := FuelFee(prepaid, p, 100, 0); ok {
		t.Fatal("prepaid fuel computes nothing")
	}
	if _, ok := FuelFee(c, Policy{}, 100, 0); ok {
		t.Fatal("no fee per percent computes nothing")
	}
	if got := ConditionsFromSnapshot([]byte(`{"included_km_per_day": 250, "extra_km_fee": " 4.50 ", "deposit_amount": "5000.00"}`)); *got.IncludedKmPerDay != 250 ||
		got.ExtraKmFee != "4.50" || got.DepositAmount != "5000.00" {
		t.Fatalf("conditions %+v", got)
	}
}
