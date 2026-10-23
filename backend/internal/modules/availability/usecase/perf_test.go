package usecase_test

import (
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
)

// TestSearchPilotLoad measures the availability search on a pilot-sized
// organization (TEC-744, docs/operations/observability.md): 3 branches,
// 150 vehicles of 5 models, 5 GROUP products and 2,000 reservations over
// 12 months, seeded with SQL into a throwaway TEST_DATABASE_URL database.
// It reports p50 / p95 / max of the full search (every product) over
// random 1–7 day requests and fails when p95 exceeds
// AVAILABILITY_P95_BUDGET_MS (default 1000).
//
//	TEST_DATABASE_URL=… go test ./internal/modules/availability/usecase -run SearchPilotLoad -v
func TestSearchPilotLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("load test")
	}
	f := newFx(t)
	const (
		branches     = 3
		vehicles     = 150
		models       = 5
		reservations = 2000
		samples      = 200
	)
	var br []ref
	for i := range branches {
		br = append(br, f.branch("PB"+strconv.Itoa(i+1), 30))
	}
	var products []ref
	var modelIDs []int64
	// f.model keys catalog codes on the first two letters: keep them apart.
	names := []string{"Albea", "Bravo", "Clio", "Doblo", "Egea"}
	for i := range models {
		m, _ := f.model(names[i])
		modelIDs = append(modelIDs, m)
		products = append(products, f.product("PP"+strconv.Itoa(i+1), "GROUP", m))
	}
	start := time.Now()

	// Vehicles: round-robin over branches and models (catalog types).
	if _, err := f.pool.Exec(f.ctx, `
		INSERT INTO vehicles (organization_id, plate, catalog_type_id, model_year, home_branch_id, current_branch_id)
		SELECT $1, '34 PL ' || lpad(g::text, 4, '0'),
		       (SELECT t.id FROM catalog_types t WHERE t.model_id = ($2::bigint[])[1 + g % cardinality($2::bigint[])]),
		       2024, ($3::bigint[])[1 + g % cardinality($3::bigint[])], ($3::bigint[])[1 + g % cardinality($3::bigint[])]
		FROM generate_series(0, $4 - 1) g`,
		f.org, modelIDs, []int64{br[0].id, br[1].id, br[2].id}, vehicles); err != nil {
		t.Fatal(err)
	}
	f.syncMembers()

	// Reservations: each vehicle gets ~13 non-overlapping 2–6 day rentals
	// at its home branch, spread over 12 months (one slot of 27 days each),
	// sold as the product of its model.
	if _, err := f.pool.Exec(f.ctx, `
		WITH v AS (
			SELECT v.id, v.home_branch_id, t.model_id, row_number() OVER (ORDER BY v.id) AS n
			FROM vehicles v JOIN catalog_types t ON t.id = v.catalog_type_id
			WHERE v.organization_id = $1
		), slots AS (
			SELECT v.*, s,
			       date_trunc('hour', $2::timestamptz) + make_interval(days => s * 27 + ((v.n * 7 + s * 11) % 18)::int) AS starts_at,
			       2 + ((v.n + s) % 5)::int AS days
			FROM v CROSS JOIN generate_series(0, 13) s
		), picked AS (
			SELECT * FROM slots ORDER BY s, n LIMIT $3
		)
		INSERT INTO vehicle_allocations (organization_id, vehicle_id, rental_product_id, product_version, kind,
			starts_at, ends_at, prep_minutes, period, pickup_branch_id, return_branch_id, fleet_pool, channel)
		SELECT $1, p.id, rp.product_id, 1, 'reservation',
		       p.starts_at, p.starts_at + make_interval(days => p.days), 0,
		       tstzrange(p.starts_at, p.starts_at + make_interval(days => p.days), '[)'),
		       p.home_branch_id, p.home_branch_id, 'MAIN', 'branch'
		FROM picked p
		JOIN rental_groups g ON g.organization_id = $1
		JOIN rental_group_catalog_models gm ON gm.rental_group_id = g.id AND gm.catalog_model_id = p.model_id
		JOIN rental_product_versions rp ON rp.organization_id = $1 AND rp.rental_group_id = g.id AND rp.version = 1`,
		f.org, f.base.Add(-30*24*time.Hour), reservations); err != nil {
		t.Fatal(err)
	}
	var seeded int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM vehicle_allocations WHERE organization_id = $1 AND status = 'active'`, f.org).Scan(&seeded); err != nil {
		t.Fatal(err)
	}
	if seeded != reservations {
		t.Fatalf("seeded %d reservations, want %d", seeded, reservations)
	}
	t.Logf("seeded %d branches, %d vehicles, %d products, %d reservations in %v", branches, vehicles, len(products), seeded, time.Since(start).Round(time.Millisecond))

	rng := rand.New(rand.NewPCG(7, 44))
	durations := make([]time.Duration, 0, samples)
	sellable := 0
	for i := range samples + 10 {
		pickup := br[rng.IntN(branches)]
		ret := pickup
		from := f.base.Add(time.Duration(rng.IntN(330*24)) * time.Hour)
		to := from.Add(time.Duration(24+rng.IntN(6*24)) * time.Hour)
		began := time.Now()
		res, err := f.svc.Search(f.ctx, f.org, model.SearchInput{Where: where(pickup, ret, from, to)})
		took := time.Since(began)
		if err != nil {
			t.Fatalf("search %d: %v", i, err)
		}
		if len(res.Items) != len(products) {
			t.Fatalf("search %d: %d items", i, len(res.Items))
		}
		if i < 10 {
			continue // warm-up (fleet snapshot cache, connections)
		}
		for _, it := range res.Items {
			sellable += it.Sellable
		}
		durations = append(durations, took)
	}
	slices.Sort(durations)
	pct := func(p float64) time.Duration { return durations[int(float64(len(durations)-1)*p)] }
	p50, p95, worst := pct(0.50), pct(0.95), durations[len(durations)-1]
	t.Logf("availability search over %d requests: p50=%v p95=%v max=%v (avg sellable per product %.1f)",
		len(durations), p50.Round(time.Microsecond), p95.Round(time.Microsecond), worst.Round(time.Microsecond),
		float64(sellable)/float64(len(durations)*len(products)))

	budget := 1000 * time.Millisecond
	if v := os.Getenv("AVAILABILITY_P95_BUDGET_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			budget = time.Duration(ms) * time.Millisecond
		}
	}
	if p95 > budget {
		t.Fatalf("p95 %v exceeds the budget %v", p95, budget)
	}
}
