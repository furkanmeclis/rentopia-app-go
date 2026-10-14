package model

import "strings"

// DisplayText is the customer-facing guarantee line of a product, used by
// the admin preview card and later by the website and channels:
//
//   - GROUP: "<representative> veya benzeri" ("Renault Clio veya benzeri")
//   - EXACT_VARIANT: "<model> (model garantili)"
//   - EXACT_VEHICLE: "<representative or title> (araç garantili)"
//
// representative is the product's representative catalog type / model label
// (falls back to the guarantee label, then the title). guaranteeLabel is the
// version snapshot of the guaranteed reference.
func DisplayText(guaranteeType, representative, guaranteeLabel, title string) string {
	representative = strings.TrimSpace(representative)
	guaranteeLabel = strings.TrimSpace(guaranteeLabel)
	title = strings.TrimSpace(title)
	first := func(values ...string) string {
		for _, v := range values {
			if v != "" {
				return v
			}
		}
		return ""
	}
	switch guaranteeType {
	case GuaranteeGroup:
		return first(representative, guaranteeLabel, title) + " veya benzeri"
	case GuaranteeExactVariant:
		return first(guaranteeLabel, representative, title) + " (model garantili)"
	case GuaranteeExactVehicle:
		return first(representative, guaranteeLabel, title) + " (araç garantili)"
	}
	return title
}

// GuaranteeBadge is the badge key of a guarantee type.
func GuaranteeBadge(guaranteeType string) string {
	switch guaranteeType {
	case GuaranteeGroup:
		return BadgeOrSimilar
	case GuaranteeExactVariant:
		return BadgeModelGuaranteed
	case GuaranteeExactVehicle:
		return BadgeVehicleGuaranteed
	}
	return ""
}

// CatalogModelLabel is "<brand> <model>".
func CatalogModelLabel(brand, model string) string {
	return joinLabel(brand, model)
}

// CatalogTypeLabel is "<brand> <type>" (the brand is not repeated when the
// type name already starts with it).
func CatalogTypeLabel(brand, typeName string) string {
	return joinLabel(brand, typeName)
}

func joinLabel(brand, name string) string {
	brand = strings.TrimSpace(brand)
	name = strings.TrimSpace(name)
	if brand != "" && strings.HasPrefix(strings.ToLower(name), strings.ToLower(brand)+" ") {
		return name
	}
	return strings.TrimSpace(brand + " " + name)
}
