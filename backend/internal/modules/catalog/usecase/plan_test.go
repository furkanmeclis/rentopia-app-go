package usecase

import (
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
)

func row(brandCode, brandName, typeCode, typeName string, values map[int]int64) tsbcatalog.Row {
	return tsbcatalog.Row{BrandCode: brandCode, BrandName: brandName, TypeCode: typeCode, TypeName: typeName, Values: values}
}

func sheetOf(period string, rows ...tsbcatalog.Row) *tsbcatalog.Sheet {
	return &tsbcatalog.Sheet{Period: period, PeriodLabel: period, Years: []int{2026, 2025, 2024}, Rows: rows}
}

func emptyState() model.State {
	return model.State{
		Brands:     map[string]model.StateBrand{},
		Models:     map[int64][]tsbcatalog.ModelRef{},
		Rules:      map[int64][]tsbcatalog.RuleRef{},
		Types:      map[model.TypeKey]model.StateType{},
		Valuations: map[model.ValuationKey]int64{},
	}
}

// stateAfter simulates what Apply stores for a plan, so the next plan can be
// computed against it.
func stateAfter(plan model.Plan) model.State {
	st := emptyState()
	var nextID int64 = 1
	modelIDs := map[string]int64{}
	for _, b := range plan.Brands {
		st.Brands[b.Code] = model.StateBrand{ID: nextID, Name: b.Name, Category: b.Category}
		nextID++
	}
	for _, m := range plan.NewModels {
		bid := st.Brands[m.BrandCode].ID
		st.Models[bid] = append(st.Models[bid], tsbcatalog.ModelRef{ID: nextID, Name: m.Name})
		modelIDs[m.BrandCode+"/"+m.Name] = nextID
		nextID++
	}
	typeIDs := map[model.TypeKey]int64{}
	for _, t := range plan.Types {
		key := model.TypeKey{BrandCode: t.BrandCode, TypeCode: t.Code}
		mid := t.ModelID
		if mid == 0 && t.ModelName != "" {
			mid = modelIDs[t.BrandCode+"/"+t.ModelName]
		}
		st.Types[key] = model.StateType{
			ID: nextID, BrandCode: t.BrandCode, ModelID: mid, RawName: t.RawName,
			Years: t.Years, IsActive: true, LastSeen: plan.Period,
		}
		typeIDs[key] = nextID
		nextID++
	}
	for _, v := range plan.Valuations {
		id := typeIDs[model.TypeKey{BrandCode: v.BrandCode, TypeCode: v.TypeCode}]
		st.Valuations[model.ValuationKey{TypeID: id, Year: v.Year}] = v.Value
	}
	return st
}

func TestBuildPlanFirstImport(t *testing.T) {
	t.Parallel()
	sheet := sheetOf("2026-10",
		row("100", "TOFAS-FIAT", "1402", "EGEA CROSS STREET 1.4 FIRE 95 E6D", map[int]int64{2025: 1051836, 2024: 971010}),
		row("100", "TOFAS-FIAT", "1403", "EGEA CROSS LOUNGE 1.6 M.JET 130 DCT", map[int]int64{2026: 1300000}),
		row("100", "TOFAS-FIAT", "1500", "EGEA SEDAN URBAN 1.4 FIRE 95", map[int]int64{2026: 900000}),
		row("900", "MOTORSIKLET", "1", "KAWASAKI NINJA 300", map[int]int64{2026: 300000}),
	)
	plan := BuildPlan(sheet, "2026-10", emptyState())
	c := plan.Counts
	if c.BrandsNew != 2 || c.TypesNew != 4 || c.ValuationsNew != 5 || c.ValuationsUpserted != 5 {
		t.Fatalf("counts: %+v", c)
	}
	if c.ModelsNew != 2 { // EGEA CROSS, EGEA
		t.Fatalf("models: %+v", plan.NewModels)
	}
	if c.UnresolvedModels != 1 || c.UnresolvedByCategory[tsbcatalog.CategoryOther] != 1 {
		t.Fatalf("unresolved: %+v", c)
	}
	if plan.Brands[0].DisplayName != "Fiat" || plan.Brands[1].Category != tsbcatalog.CategoryOther {
		t.Fatalf("brand defaults: %+v", plan.Brands)
	}
	if got := plan.Types[0]; got.ModelName != "EGEA CROSS" || got.Fuel != tsbcatalog.FuelPetrol {
		t.Fatalf("type 1402: %+v", got)
	}
	if got := plan.Types[1]; got.Transmission != tsbcatalog.TransmissionAutomatic || got.Fuel != tsbcatalog.FuelDiesel {
		t.Fatalf("type 1403: %+v", got)
	}
	if got := plan.Types[2]; got.ModelName != "EGEA" {
		t.Fatalf("type 1500: %+v", got)
	}
}

func TestBuildPlanIsIdempotentAndTracksChanges(t *testing.T) {
	t.Parallel()
	first := sheetOf("2026-10",
		row("100", "TOFAS-FIAT", "1402", "EGEA CROSS STREET 1.4 FIRE 95 E6D", map[int]int64{2025: 1051836}),
		row("100", "TOFAS-FIAT", "1403", "EGEA CROSS LOUNGE 1.6 M.JET 130 DCT", map[int]int64{2026: 1300000}),
	)
	p1 := BuildPlan(first, "2026-10", emptyState())
	st := stateAfter(p1)

	// Same file again: nothing to do.
	again := BuildPlan(first, "2026-10", st)
	c := again.Counts
	if c.BrandsNew+c.BrandsRenamed+c.ModelsNew+c.TypesNew+c.TypesRenamed+c.TypesUpdated+
		c.TypesDeactivated+c.TypesReactivated+c.ValuationsUpserted != 0 || len(again.Deactivate) != 0 {
		t.Fatalf("re-plan should be a no-op: %+v", c)
	}

	// Next month: 1403 dropped, 1402 renamed with a new value, a new type.
	next := sheetOf("2026-11",
		row("100", "TOFAS-FIAT", "1402", "EGEA CROSS STREET 1.4 FIRE 95 E6D MY26", map[int]int64{2025: 1060000}),
		row("100", "TOFAS-FIAT", "1404", "EGEA CROSS LIMITED 1.5 T4 HIBRIT 130 AT", map[int]int64{2026: 1500000}),
	)
	// LoadState only loads valuations of the planned period.
	stNext := st
	stNext.Valuations = map[model.ValuationKey]int64{}
	p2 := BuildPlan(next, "2026-11", stNext)
	c = p2.Counts
	if c.TypesNew != 1 || c.TypesRenamed != 1 || c.TypesDeactivated != 1 || c.ModelsNew != 0 {
		t.Fatalf("next month counts: %+v", c)
	}
	// Valuations are per period: both values are new rows for 2026-11.
	if c.ValuationsNew != 2 || c.ValuationsChanged != 0 {
		t.Fatalf("valuations: %+v", c)
	}
	// The new type reuses the existing EGEA CROSS model.
	if p2.Types[1].ModelID == 0 {
		t.Fatalf("new type should match the existing model: %+v", p2.Types[1])
	}
	// The renamed type keeps its model.
	if p2.Types[0].ModelID != st.Types[model.TypeKey{BrandCode: "100", TypeCode: "1402"}].ModelID {
		t.Fatalf("model assignment lost: %+v", p2.Types[0])
	}

	// An older file never deactivates newer types.
	st2 := stateAfter(p2)
	older := BuildPlan(first, "2026-10", st2)
	if older.Counts.TypesDeactivated != 0 {
		t.Fatalf("older period deactivated types: %+v", older.Counts)
	}
}

func TestBuildPlanRulesAndChangedValues(t *testing.T) {
	t.Parallel()
	st := emptyState()
	st.Brands["100"] = model.StateBrand{ID: 1, Name: "TOFAS-FIAT", Category: tsbcatalog.CategoryPassenger}
	st.Models[1] = []tsbcatalog.ModelRef{{ID: 10, Name: "Egea"}, {ID: 11, Name: "Egea Cross"}}
	st.Rules[1] = []tsbcatalog.RuleRef{{Prefix: "EGEA CROSS STREET", ModelID: 12}}
	st.Types[model.TypeKey{BrandCode: "100", TypeCode: "1"}] = model.StateType{
		ID: 50, BrandCode: "100", RawName: "EGEA URBAN 1.4", Years: []int32{2026}, IsActive: false, LastSeen: "2026-09",
	}
	st.Valuations[model.ValuationKey{TypeID: 50, Year: 2026}] = 100

	sheet := sheetOf("2026-10",
		row("100", "TOFAS-FIAT", "1", "EGEA URBAN 1.4", map[int]int64{2026: 120}),
		row("100", "TOFAS-FIAT", "2", "EGEA CROSS STREET 1.4", map[int]int64{2026: 1}),
	)
	p := BuildPlan(sheet, "2026-10", st)
	if p.Counts.TypesReactivated != 1 || p.Counts.ValuationsChanged != 1 || p.Counts.BrandsRenamed != 0 {
		t.Fatalf("counts: %+v", p.Counts)
	}
	if p.Types[0].ModelID != 10 { // unresolved existing type picks up "Egea"
		t.Fatalf("existing unresolved type: %+v", p.Types[0])
	}
	if p.Types[1].ModelID != 12 { // rule wins over the known "Egea Cross"
		t.Fatalf("rule: %+v", p.Types[1])
	}
	if len(p.Diff.ChangedValuations) != 1 || p.Diff.ChangedValuations[0].Old != 100 {
		t.Fatalf("diff: %+v", p.Diff.ChangedValuations)
	}
}
