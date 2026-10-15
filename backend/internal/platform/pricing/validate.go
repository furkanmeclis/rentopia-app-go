package pricing

import (
	"fmt"
	"sort"
)

// Issue is one activation problem: Field is a dot path of the form
// ("bands.2.max_days"), Code a stable code, Params the numbers involved.
type Issue struct {
	Field   string         `json:"field"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Params  map[string]int `json:"params,omitempty"`
}

// Band issue codes.
const (
	IssueNoBands      = "no_bands"
	IssueBandRange    = "band_range"
	IssueBandPrice    = "band_price"
	IssueBandOverlap  = "band_overlap"
	IssueCoverageGap  = "coverage_gap"
	IssueCoverageEnd  = "coverage_short"
	IssueCoverageFrom = "coverage_start"
)

// ValidateBands checks a plan version's bands: each band is well formed
// (1 <= min <= max, price >= 0), no two bands overlap, and, when
// gaplessUpTo > 0 (the plan prices online sales), the bands cover every
// length from 1 day to gaplessUpTo days without a gap.
func ValidateBands(bands []Band, gaplessUpTo int) []Issue {
	issues := []Issue{}
	if len(bands) == 0 {
		return append(issues, Issue{Field: "bands", Code: IssueNoBands, Message: "at least one duration band is required"})
	}
	type idx struct {
		Band
		i int
	}
	sorted := make([]idx, 0, len(bands))
	for i, b := range bands {
		field := fmt.Sprintf("bands.%d", i)
		if b.MinDays < 1 || (b.MaxDays != nil && *b.MaxDays < b.MinDays) {
			issues = append(issues, Issue{
				Field: field + ".max_days", Code: IssueBandRange,
				Message: "a band needs 1 <= min_days <= max_days", Params: map[string]int{"min": b.MinDays},
			})
			continue
		}
		if b.DailyPrice.IsNegative() {
			issues = append(issues, Issue{Field: field + ".daily_price", Code: IssueBandPrice, Message: "daily price must not be negative"})
		}
		sorted = append(sorted, idx{b, i})
	}
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].MinDays < sorted[b].MinDays })
	for k := 1; k < len(sorted); k++ {
		prev, cur := sorted[k-1], sorted[k]
		if prev.MaxDays == nil || *prev.MaxDays >= cur.MinDays {
			issues = append(issues, Issue{
				Field: fmt.Sprintf("bands.%d.min_days", cur.i), Code: IssueBandOverlap,
				Message: fmt.Sprintf("band starting at %d days overlaps the band starting at %d days", cur.MinDays, prev.MinDays),
				Params:  map[string]int{"min": cur.MinDays, "other_min": prev.MinDays},
			})
		}
	}
	if gaplessUpTo <= 0 || len(sorted) == 0 {
		return issues
	}
	if sorted[0].MinDays != 1 {
		return append(issues, Issue{
			Field: fmt.Sprintf("bands.%d.min_days", sorted[0].i), Code: IssueCoverageFrom,
			Message: "online channels need a band starting at 1 day", Params: map[string]int{"min": sorted[0].MinDays},
		})
	}
	reach := 0 // longest length covered contiguously from 1 day
	for _, b := range sorted {
		if b.MinDays > reach+1 {
			return append(issues, Issue{
				Field: fmt.Sprintf("bands.%d.min_days", b.i), Code: IssueCoverageGap,
				Message: fmt.Sprintf("days %d-%d are not covered", reach+1, b.MinDays-1),
				Params:  map[string]int{"from": reach + 1, "to": b.MinDays - 1},
			})
		}
		if b.MaxDays == nil {
			reach = gaplessUpTo
			break
		}
		reach = max(reach, *b.MaxDays)
		if reach >= gaplessUpTo {
			break
		}
	}
	if reach < gaplessUpTo {
		issues = append(issues, Issue{
			Field: "bands", Code: IssueCoverageEnd,
			Message: fmt.Sprintf("online channels need bands up to %d days (covered up to %d)", gaplessUpTo, reach),
			Params:  map[string]int{"max": gaplessUpTo, "covered": reach},
		})
	}
	return issues
}

// PlanKey identifies the target of a plan: product + scope + branch +
// channel. Two active plans with the same key and priority are ambiguous.
func PlanKey(productID string, p Plan) string {
	return productID + "|" + p.Scope + "|" + p.BranchID + "|" + p.Channel
}

// RulesOverlap reports whether two date rules can match the same day of the
// same rental: their date ranges overlap and their product, branch and
// channel targets intersect. productSegment maps product ids to segment ids
// (to intersect a product list with a segment list); allProducts lists the
// organization's products (to intersect "all" with a list).
func RulesOverlap(a, b DateRule, productSegment map[string]string) bool {
	if !a.Range.Overlaps(b.Range) {
		return false
	}
	if !intersects(a.BranchIDs, b.BranchIDs) || !intersects(a.Channels, b.Channels) {
		return false
	}
	return productsIntersect(a, b, productSegment)
}

// intersects treats nil as "everything".
func intersects(a, b []string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil:
		return len(b) > 0
	case b == nil:
		return len(a) > 0
	}
	for _, x := range a {
		if contains(b, x) {
			return true
		}
	}
	return false
}

func productsIntersect(a, b DateRule, productSegment map[string]string) bool {
	if a.ProductScope == ProductsAll || b.ProductScope == ProductsAll {
		return true
	}
	switch {
	case a.ProductScope == ProductsSegments && b.ProductScope == ProductsSegments:
		return intersects(a.SegmentIDs, b.SegmentIDs)
	case a.ProductScope == ProductsSelected && b.ProductScope == ProductsSelected:
		return intersects(a.ProductIDs, b.ProductIDs)
	case a.ProductScope == ProductsSegments:
		return productInSegments(b.ProductIDs, a.SegmentIDs, productSegment)
	default:
		return productInSegments(a.ProductIDs, b.SegmentIDs, productSegment)
	}
}

func productInSegments(products, segments []string, productSegment map[string]string) bool {
	for _, p := range products {
		if contains(segments, productSegment[p]) {
			return true
		}
	}
	return false
}
