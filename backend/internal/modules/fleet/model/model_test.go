package model

import "testing"

func TestValidateACRISS(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code string
		ok   bool
	}{
		{"ECMR", true},  // economy, 2/4 door, manual, A/C
		{"CDMR", true},  // compact 4-5 door manual
		{"EDAR", true},  // economy 4-5 door automatic
		{"IFAD", true},  // intermediate SUV automatic diesel
		{"SWMV", true},  // standard wagon manual petrol A/C
		{"MBMN", true},  // mini 2-3 door manual no A/C
		{"XXXX", false}, // X is not a transmission / fuel letter
		{"AXMR", false}, // A is not a category
		{"EAMR", false}, // A is not a type
		{"EDTR", false}, // T is not a transmission
		{"EDMK", false}, // K is not a fuel/air letter
		{"ECM", false},
		{"ECMRX", false},
		{"ecmr", false}, // callers normalize first
		{"EC1R", false},
	}
	for _, tc := range cases {
		err := ValidateACRISS(tc.code)
		if (err == nil) != tc.ok {
			t.Errorf("ValidateACRISS(%q) = %v, want ok=%v", tc.code, err, tc.ok)
		}
	}
	if got := NormalizeACRISS("  ecmr "); got != "ECMR" {
		t.Errorf("NormalizeACRISS = %q", got)
	}
}

func TestDisplayText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, gType, rep, label, title, want string
	}{
		{"group with representative", GuaranteeGroup, "Renault Clio", "Ekonomi Manuel", "Ekonomi", "Renault Clio veya benzeri"},
		{"group falls back to the group label", GuaranteeGroup, "", "Clio/Corsa/i20", "Ekonomi", "Clio/Corsa/i20 veya benzeri"},
		{"exact variant type level", GuaranteeExactVariant, "Fiat Egea", "Fiat Egea Cross 1.4 Fire", "Egea Cross", "Fiat Egea Cross 1.4 Fire (model garantili)"},
		{"exact variant model level", GuaranteeExactVariant, "", "Fiat Egea Cross", "x", "Fiat Egea Cross (model garantili)"},
		{"exact vehicle", GuaranteeExactVehicle, "", "", "34 ABC 123 Egea", "34 ABC 123 Egea (araç garantili)"},
		{"unknown type", "STOCK", "a", "b", "Title", "Title"},
	}
	for _, tc := range cases {
		if got := DisplayText(tc.gType, tc.rep, tc.label, tc.title); got != tc.want {
			t.Errorf("%s: DisplayText = %q, want %q", tc.name, got, tc.want)
		}
	}
	if GuaranteeBadge(GuaranteeGroup) != BadgeOrSimilar || GuaranteeBadge(GuaranteeExactVariant) != BadgeModelGuaranteed ||
		GuaranteeBadge(GuaranteeExactVehicle) != BadgeVehicleGuaranteed {
		t.Error("unexpected guarantee badges")
	}
	if got := CatalogModelLabel("Renault", "Clio"); got != "Renault Clio" {
		t.Errorf("CatalogModelLabel = %q", got)
	}
	if got := CatalogTypeLabel("Fiat", "Fiat Egea Cross 1.4 Fire"); got != "Fiat Egea Cross 1.4 Fire" {
		t.Errorf("CatalogTypeLabel keeps a leading brand once: %q", got)
	}
}
