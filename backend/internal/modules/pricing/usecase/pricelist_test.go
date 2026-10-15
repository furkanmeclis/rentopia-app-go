package usecase

import (
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func TestParseBand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		min, max int // max -1 = open
		bad      bool
	}{
		{"1-3", 1, 3, false},
		{"1 – 3", 1, 3, false},
		{"4-7 gün", 4, 7, false},
		{"15+", 15, -1, false},
		{"15+ gün", 15, -1, false},
		{"30-", 30, -1, false},
		{"7", 7, 7, false},
		{"0-3", 0, 0, true},
		{"5-3", 0, 0, true},
		{"abc", 0, 0, true},
		{"", 0, 0, true},
	}
	for _, c := range cases {
		min, max, err := ParseBand(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("ParseBand(%q) must fail", c.in)
			}
			continue
		}
		gotMax := -1
		if max != nil {
			gotMax = *max
		}
		if err != nil || min != c.min || gotMax != c.max {
			t.Errorf("ParseBand(%q) = %d %d %v, want %d %d", c.in, min, gotMax, err, c.min, c.max)
		}
		if back, _, _ := ParseBand(FormatBand(min, max)); back != min {
			t.Errorf("FormatBand round trip of %q", c.in)
		}
	}
}

func TestSameBandsAndOverlap(t *testing.T) {
	t.Parallel()
	three := 3
	a := []model.Band{{MinDays: 1, MaxDays: &three, DailyPrice: money.MustParse("10")}, {MinDays: 4, DailyPrice: money.MustParse("9")}}
	b := []model.Band{a[1], a[0]}
	if !sameBands(a, b) {
		t.Fatal("order must not matter")
	}
	c := []model.Band{a[0], {MinDays: 4, DailyPrice: money.MustParse("8")}}
	if sameBands(a, c) {
		t.Fatal("price change must be detected")
	}
	if !overlaps(1, &three, 3, nil) || overlaps(1, &three, 4, nil) {
		t.Fatal("overlap")
	}
}

func TestParseBandsValidation(t *testing.T) {
	t.Parallel()
	ve := &ValidationError{}
	three := 3
	parseBands(ve, "", []model.BandInput{
		{MinDays: 1, MaxDays: &three, DailyPrice: "1300,50"},
		{MinDays: 3, DailyPrice: "100"},
		{MinDays: 9, DailyPrice: "-1"},
	})
	if !ve.has("bands.2.daily_price") {
		t.Fatalf("negative price: %+v", ve.Details)
	}
	ve = &ValidationError{}
	parseBands(ve, "", []model.BandInput{{MinDays: 1, MaxDays: &three, DailyPrice: "1300,50"}, {MinDays: 3, DailyPrice: "100"}})
	if !ve.has("bands.1.min_days") {
		t.Fatalf("overlap: %+v", ve.Details)
	}
	ve = &ValidationError{}
	if out := parseBands(ve, "", nil); len(out) != 0 || ve.orNil() != nil {
		t.Fatalf("empty draft bands are allowed: %+v", ve.Details)
	}
}
