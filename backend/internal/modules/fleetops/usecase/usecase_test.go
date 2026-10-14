package usecase

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func num(t *testing.T, v string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(v); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestQuoteFromRules(t *testing.T) {
	t.Parallel()
	product := db.ListOneWayRulesForRouteRow{
		Uuid: uuid.New(), RentalProductID: pgtype.Int8{Int64: 7, Valid: true}, Allowed: true, Fee: num(t, "250"),
	}
	generic := db.ListOneWayRulesForRouteRow{Uuid: uuid.New(), Allowed: true, Fee: num(t, "500.5")}
	denied := db.ListOneWayRulesForRouteRow{Uuid: uuid.New(), Allowed: false, Fee: num(t, "99")}

	if q := QuoteFromRules(nil); q.Allowed || q.Rule != nil || q.Fee != "0.00" {
		t.Errorf("no rule = %+v", q)
	}
	if q := QuoteFromRules([]db.ListOneWayRulesForRouteRow{product, generic}); !q.ProductSpecific || q.Fee != "250.00" || *q.Rule != product.Uuid {
		t.Errorf("product overrides generic = %+v", q)
	}
	if q := QuoteFromRules([]db.ListOneWayRulesForRouteRow{generic}); q.ProductSpecific || q.Fee != "500.50" || !q.Allowed {
		t.Errorf("generic = %+v", q)
	}
	if q := QuoteFromRules([]db.ListOneWayRulesForRouteRow{denied}); q.Allowed || q.Fee != "0.00" {
		t.Errorf("denied route = %+v", q)
	}
}

func TestBlockState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	cases := []struct {
		start time.Time
		end   *time.Time
		want  string
	}{
		{past, nil, model.BlockActive},
		{future, nil, model.BlockScheduled},
		{past, &future, model.BlockActive},
		{past.Add(-time.Hour), &past, model.BlockEnded},
		{past, &now, model.BlockEnded},
		{now, nil, model.BlockActive},
	}
	for i, tc := range cases {
		if got := blockState(tc.start, tc.end, now); got != tc.want {
			t.Errorf("case %d: %s, want %s", i, got, tc.want)
		}
	}
}

func TestParseMoney(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"0": "0.00", "1250.5": "1250.50", "12,34": "12.34", "999999999999.99": "999999999999.99"} {
		ve := &ValidationError{}
		n := json.Number(in)
		if got := FormatMoney(parseMoney(ve, "fee", &n)); got != want || ve.orNil() != nil {
			t.Errorf("%s = %s (%v), want %s", in, got, ve.orNil(), want)
		}
	}
	for _, in := range []string{"-1", "1.234", "abc", "1000000000000"} {
		ve := &ValidationError{}
		n := json.Number(in)
		if parseMoney(ve, "fee", &n); ve.orNil() == nil {
			t.Errorf("%s accepted", in)
		}
	}
}
