package usecase

import (
	"slices"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
)

// sampleLimit caps each diff sample list stored on the import.
const sampleLimit = 25

// BuildPlan compares a parsed TSB sheet with the stored catalog and returns
// what an apply would change. It does not touch the database.
//
// Rules:
//   - brands are matched by TSB code; a new brand gets the default display
//     name / category, an existing one keeps its admin-edited fields.
//   - types are matched by (brand code, type code). Existing model
//     assignments are kept; new types and types without a model are resolved
//     with the brand's prefix rules, known models and, for passenger / light
//     commercial brands, a derived "auto" model.
//   - active types missing from the file are deactivated when the file is
//     not older than what they were last seen in.
//   - every non-zero value becomes a valuation for the period.
func BuildPlan(sheet *tsbcatalog.Sheet, period string, st model.State) model.Plan {
	plan := model.Plan{Period: period}
	plan.Counts.Rows = len(sheet.Rows)
	plan.Counts.UnresolvedByCategory = map[string]int{}
	plan.Diff = emptyDiff()
	plan.Diff.PeriodLabel = sheet.PeriodLabel
	plan.Diff.Years = sheet.Years

	// Brands, in file order.
	brandCategory := map[string]string{}
	brandLabel := map[string]string{}
	namesByBrand := map[string][]string{}
	for _, r := range sheet.Rows {
		namesByBrand[r.BrandCode] = append(namesByBrand[r.BrandCode], r.TypeName)
		if _, done := brandCategory[r.BrandCode]; done {
			continue
		}
		pb := model.PlanBrand{Code: r.BrandCode, Name: r.BrandName, Op: "same"}
		if existing, ok := st.Brands[r.BrandCode]; ok {
			pb.Category = existing.Category
			if existing.Name != r.BrandName {
				pb.Op = "renamed"
				pb.OldName = existing.Name
				plan.Counts.BrandsRenamed++
				appendSample(&plan.Diff.RenamedBrands, model.SampleBrand{
					Code: r.BrandCode, Name: r.BrandName, OldName: existing.Name, Category: existing.Category,
				})
			}
		} else {
			def := tsbcatalog.DefaultsForBrand(r.BrandName)
			pb.Op = "new"
			pb.DisplayName = def.DisplayName
			pb.Category = def.Category
			plan.Counts.BrandsNew++
			appendSample(&plan.Diff.NewBrands, model.SampleBrand{
				Code: r.BrandCode, Name: r.BrandName, DisplayName: def.DisplayName, Category: def.Category,
			})
		}
		brandCategory[r.BrandCode] = pb.Category
		brandLabel[r.BrandCode] = r.BrandName
		plan.Brands = append(plan.Brands, pb)
	}

	// One resolver per brand, primed with every name of the brand.
	resolvers := map[string]*tsbcatalog.Resolver{}
	resolver := func(code string) *tsbcatalog.Resolver {
		if r, ok := resolvers[code]; ok {
			return r
		}
		var rules []tsbcatalog.RuleRef
		var models []tsbcatalog.ModelRef
		if b, ok := st.Brands[code]; ok {
			rules = st.Rules[b.ID]
			models = st.Models[b.ID]
		}
		r := tsbcatalog.NewResolver(rules, models, brandCategory[code] != tsbcatalog.CategoryOther)
		r.Prime(namesByBrand[code])
		resolvers[code] = r
		return r
	}

	newModels := map[string]map[string]*model.SampleModel{} // brand -> upper(name)
	seen := make(map[model.TypeKey]struct{}, len(sheet.Rows))
	for _, r := range sheet.Rows {
		key := model.TypeKey{BrandCode: r.BrandCode, TypeCode: r.TypeCode}
		seen[key] = struct{}{}
		years := toInt32(r.Years())
		pt := model.PlanType{
			BrandCode:    r.BrandCode,
			Code:         r.TypeCode,
			RawName:      r.TypeName,
			Years:        years,
			Transmission: tsbcatalog.SuggestTransmission(r.TypeName),
			Fuel:         tsbcatalog.SuggestFuel(r.TypeName),
			Op:           "same",
		}
		existing, exists := st.Types[key]
		if exists {
			pt.ModelID = existing.ModelID
			switch {
			case !existing.IsActive:
				pt.Op = "reactivated"
				plan.Counts.TypesReactivated++
				appendSample(&plan.Diff.ReactivatedTypes, model.SampleType{Brand: r.BrandName, Code: r.TypeCode, Name: r.TypeName})
			case existing.RawName != r.TypeName:
				pt.Op = "renamed"
				pt.OldName = existing.RawName
				plan.Counts.TypesRenamed++
				appendSample(&plan.Diff.RenamedTypes, model.SampleType{
					Brand: r.BrandName, Code: r.TypeCode, Name: r.TypeName, OldName: existing.RawName,
				})
			case !slices.Equal(existing.Years, years):
				pt.Op = "updated"
				plan.Counts.TypesUpdated++
			}
		} else {
			pt.Op = "new"
			plan.Counts.TypesNew++
		}

		if pt.ModelID == 0 {
			m := resolver(r.BrandCode).Resolve(r.TypeName)
			switch m.Source {
			case "rule", "model":
				pt.ModelID = m.ModelID
				pt.ModelName = m.Name
			case "derived":
				pt.ModelName = m.Name
				byName := newModels[r.BrandCode]
				if byName == nil {
					byName = map[string]*model.SampleModel{}
					newModels[r.BrandCode] = byName
				}
				k := strings.ToUpper(m.Name)
				if sm, ok := byName[k]; ok {
					sm.Types++
				} else {
					byName[k] = &model.SampleModel{Brand: r.BrandName, Name: m.Name, Types: 1}
					plan.NewModels = append(plan.NewModels, model.PlanModel{BrandCode: r.BrandCode, Name: m.Name})
				}
			default:
				plan.Counts.UnresolvedModels++
				plan.Counts.UnresolvedByCategory[brandCategory[r.BrandCode]]++
				appendSample(&plan.Diff.UnresolvedTypes, model.SampleType{Brand: r.BrandName, Code: r.TypeCode, Name: r.TypeName})
			}
		}
		if pt.Op == "new" {
			appendSample(&plan.Diff.NewTypes, model.SampleType{
				Brand: r.BrandName, Code: r.TypeCode, Name: r.TypeName, Model: pt.ModelName,
			})
		}
		plan.Types = append(plan.Types, pt)

		for _, y := range years {
			v := r.Values[int(y)]
			plan.Valuations = append(plan.Valuations, model.PlanValuation{
				BrandCode: r.BrandCode, TypeCode: r.TypeCode, Year: y, Value: v,
			})
			if !exists {
				plan.Counts.ValuationsNew++
				continue
			}
			old, had := st.Valuations[model.ValuationKey{TypeID: existing.ID, Year: y}]
			switch {
			case !had:
				plan.Counts.ValuationsNew++
			case old != v:
				plan.Counts.ValuationsChanged++
				appendSample(&plan.Diff.ChangedValuations, model.SampleValuation{
					Brand: r.BrandName, Code: r.TypeCode, Name: r.TypeName, Year: y, Old: old, New: v,
				})
			}
		}
	}
	plan.Counts.ValuationsUpserted = plan.Counts.ValuationsNew + plan.Counts.ValuationsChanged
	plan.Counts.ModelsNew = len(plan.NewModels)
	for _, nm := range plan.NewModels {
		if sm := newModels[nm.BrandCode][strings.ToUpper(nm.Name)]; sm != nil {
			appendSample(&plan.Diff.NewModels, *sm)
		}
	}

	// Deactivations: active types not in this file, last seen before it.
	for key, t := range st.Types {
		if !t.IsActive || t.LastSeen >= period {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		plan.Deactivate = append(plan.Deactivate, t.ID)
		label := brandLabel[key.BrandCode]
		if label == "" {
			if b, ok := st.Brands[key.BrandCode]; ok {
				label = b.Name
			}
		}
		appendSample(&plan.Diff.DeactivatedTypes, model.SampleType{Brand: label, Code: key.TypeCode, Name: t.RawName})
	}
	slices.Sort(plan.Deactivate)
	plan.Counts.TypesDeactivated = len(plan.Deactivate)
	return plan
}

func emptyDiff() model.Diff {
	return model.Diff{
		NewBrands:         []model.SampleBrand{},
		RenamedBrands:     []model.SampleBrand{},
		NewModels:         []model.SampleModel{},
		NewTypes:          []model.SampleType{},
		RenamedTypes:      []model.SampleType{},
		DeactivatedTypes:  []model.SampleType{},
		ReactivatedTypes:  []model.SampleType{},
		UnresolvedTypes:   []model.SampleType{},
		ChangedValuations: []model.SampleValuation{},
	}
}

func appendSample[T any](dst *[]T, v T) {
	if len(*dst) < sampleLimit {
		*dst = append(*dst, v)
	}
}

func toInt32(in []int) []int32 {
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v)
	}
	return out
}
