package usecase_test

import (
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
)

// TestRT_A03_MixedModelsOneCardAndExactVariant completes RT-A03 (TEC-691 /
// TEC-697) next to TestRT_A03_ThirtyVehiclesOneProduct:
//
//   - 10 Clio + 10 Corsa + 10 i20 in one rental group behind one
//     group-guaranteed product → the search answers one item (one card on
//     the site) with capacity 30;
//   - 20 Egea Cross 1.4 Fire behind an EXACT_VARIANT product → one more
//     item with capacity 20; a 21st Egea changes the capacity (21), never
//     the number of items;
//   - the model-guaranteed product counts no other model as a source
//     (eligible 20 / 21, not 50 / 51), and the group product counts no
//     Egea.
func TestRT_A03_MixedModelsOneCardAndExactVariant(t *testing.T) {
	f := newFx(t)
	mrk := f.branch("MRK", 60)
	clioModel, clioType := f.model("Clio")
	corsaModel, corsaType := f.model("Corsa")
	i20Model, i20Type := f.model("i20")
	for _, typ := range []int64{clioType, corsaType, i20Type} {
		for i := 0; i < 10; i++ {
			f.vehicle(typ, mrk, "MAIN")
		}
	}
	group := f.product("KOMPAKT", "GROUP", clioModel, corsaModel, i20Model)
	w := where(mrk, mrk, f.at(0), f.at(72))

	items := func() []model.ProductAvailability {
		t.Helper()
		res, err := f.svc.Search(f.ctx, f.org, model.SearchInput{Where: w})
		if err != nil {
			t.Fatal(err)
		}
		return res.Items
	}
	got := items()
	if len(got) != 1 || got[0].Product.UUID != group.uuid {
		t.Fatalf("RT-A03: %d items for one group product of three models, want 1", len(got))
	}
	if got[0].Sellable != 30 || got[0].Breakdown.Eligible != 30 || got[0].Breakdown.Physical != 30 {
		t.Fatalf("RT-A03: group capacity = %d (breakdown %+v), want 30", got[0].Sellable, got[0].Breakdown)
	}

	egeaModel, egeaType := f.model("Egea Cross 1.4 Fire")
	for i := 0; i < 20; i++ {
		f.vehicle(egeaType, mrk, "MAIN")
	}
	exact := f.product("EGEA", "EXACT_VARIANT", egeaModel)
	check := func(wantEgea int) {
		t.Helper()
		got := items()
		if len(got) != 2 {
			t.Fatalf("RT-A03: %d items with %d Egea, want 2 (one card per product)", len(got), wantEgea)
		}
		by := map[string]model.ProductAvailability{}
		for _, it := range got {
			by[it.Product.UUID.String()] = it
		}
		e, g := by[exact.uuid.String()], by[group.uuid.String()]
		if e.Sellable != wantEgea || e.Breakdown.Eligible != wantEgea {
			t.Fatalf("RT-A03: EXACT_VARIANT capacity %d eligible %d, want %d (other models are no source)", e.Sellable, e.Breakdown.Eligible, wantEgea)
		}
		if g.Sellable != 30 || g.Breakdown.Eligible != 30 {
			t.Fatalf("RT-A03: group product counts Egea: sellable %d eligible %d", g.Sellable, g.Breakdown.Eligible)
		}
	}
	check(20)
	f.vehicle(egeaType, mrk, "MAIN") // the 21st Egea
	check(21)
}
