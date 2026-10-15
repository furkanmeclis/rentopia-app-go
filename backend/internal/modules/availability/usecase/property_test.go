package usecase_test

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/google/uuid"
)

// Randomized test against Postgres: several workers (own connections, own
// service instances) run random holds, releases, date changes and plate
// assignments over overlapping products (exact model, model group, any
// vehicle) with one-way returns. Afterwards, and after every operation of
// a sequential phase, the database is checked independently of the engine
// (fx.invariants): no double booking, every allocation on a vehicle that
// keeps its product's guarantee, location continuity per vehicle. The
// allocations are their own witness: a valid assignment means every
// commitment is satisfiable together. The engine-level property test
// (engine/property_test.go) compares refusals with brute force.
func TestRandomOperationsKeepInvariants(t *testing.T) {
	f := newFx(t)
	a, b := f.branch("AAA", 0), f.branch("BBB", 30)
	f.allowOneWay(a, b)
	f.allowOneWay(b, a)
	egeaModel, egeaType := f.model("Egea")
	clioModel, clioType := f.model("Clio")
	e1, e2 := f.vehicle(egeaType, a, "MAIN"), f.vehicle(egeaType, b, "MAIN")
	c1, c2 := f.vehicle(clioType, a, "MAIN"), f.vehicle(clioType, a, "MAIN")
	m1 := f.vehicle(0, b, "MAIN")
	exact := f.product("EGEA", "EXACT_VARIANT", egeaModel)
	compact := f.product("KOMPAKT", "GROUP", egeaModel, clioModel)
	anyCar := f.product("HERHANGI", "GROUP")
	eligible := map[int64]map[int64]bool{
		exact.id:   {e1.id: true, e2.id: true},
		compact.id: {e1.id: true, e2.id: true, c1.id: true, c2.id: true},
		anyCar.id:  {e1.id: true, e2.id: true, c1.id: true, c2.id: true, m1.id: true},
	}
	products := []ref{exact, compact, anyCar}
	branches := []ref{a, b}
	vehicles := []ref{e1, e2, c1, c2, m1}
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)

	type stats struct{ holds, refused, released, changed, plates, reassigned int }
	var mu sync.Mutex
	affected := map[uuid.UUID]bool{}
	run := func(rng *rand.Rand, svc *usecase.Service, own *[]uuid.UUID, st *stats, check bool) {
		randomWhere := func() model.Where {
			start := rng.Intn(120)
			pickup := branches[rng.Intn(2)]
			ret := pickup
			if rng.Intn(3) == 0 {
				ret = branches[1-indexOf(branches, pickup)]
			}
			return where(pickup, ret, f.at(start), f.at(start+1+rng.Intn(24)))
		}
		for i := 0; i < 25; i++ {
			op := rng.Intn(10)
			switch {
			case op < 6 || len(*own) == 0:
				res, err := svc.Reserve(f.ctx, f.org, model.ReserveInput{
					Where: randomWhere(), ProductUUID: products[rng.Intn(3)].uuid, Kind: model.KindHold,
				})
				switch {
				case err == nil:
					st.holds++
					st.reassigned += res.Reassigned
					*own = append(*own, res.Allocation.UUID)
				case isCapacity(err):
					st.refused++
				default:
					t.Errorf("reserve: %v", err)
					return
				}
			case op < 7:
				k := rng.Intn(len(*own))
				res, err := svc.Release(f.ctx, f.org, (*own)[k], "", nil)
				if err != nil {
					t.Errorf("release: %v", err)
					return
				}
				mu.Lock()
				for _, a := range res.Affected {
					affected[a.UUID] = true
				}
				mu.Unlock()
				*own = append((*own)[:k], (*own)[k+1:]...)
				st.released++
			case op < 9:
				res, err := svc.ChangeDates(f.ctx, f.org, (*own)[rng.Intn(len(*own))], model.ChangeInput{Where: randomWhere()})
				if err == nil {
					st.changed++
					st.reassigned += res.Reassigned
				} else if !isCapacity(err) {
					t.Errorf("change dates: %v", err)
					return
				}
			default:
				res, err := svc.AssignPlate(f.ctx, f.org, (*own)[rng.Intn(len(*own))], vehicles[rng.Intn(len(vehicles))].uuid)
				if err == nil {
					st.plates++
					st.reassigned += res.Reassigned
				} else if _, bad := err.(*usecase.ValidationError); !bad && !isCapacity(err) {
					t.Errorf("assign plate: %v", err)
					return
				}
			}
			if check {
				f.invariantsExcept(eligible, affected)
			}
		}
	}

	// Concurrent phase.
	const workers = 4
	var wg sync.WaitGroup
	all := make([]stats, workers)
	owned := make([][]uuid.UUID, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			run(rand.New(rand.NewSource(seed+int64(w))), f.newService(), &owned[w], &all[w], false)
		}(w)
	}
	wg.Wait()
	f.invariantsExcept(eligible, affected)
	// Sequential phase, checked after every operation.
	seq := stats{}
	var mine []uuid.UUID
	for _, list := range owned {
		mine = append(mine, list...)
	}
	run(rand.New(rand.NewSource(seed+99)), f.svc, &mine, &seq, true)
	total := seq
	for _, s := range all {
		total.holds += s.holds
		total.refused += s.refused
		total.released += s.released
		total.changed += s.changed
		total.plates += s.plates
		total.reassigned += s.reassigned
	}
	t.Logf("holds %d, refused %d, released %d (left affected %d), date changes %d, plates %d, reassignments %d",
		total.holds, total.refused, total.released, len(affected), total.changed, total.plates, total.reassigned)
	if total.holds == 0 || total.refused == 0 {
		t.Fatalf("degenerate run: %+v", total)
	}
}

func indexOf(list []ref, r ref) int {
	for i, x := range list {
		if x.id == r.id {
			return i
		}
	}
	return -1
}
