package i18n

import "testing"

// TestReportsCatalogParity: the report keys exist in both locales, are
// translated and reach the merged catalogs.
func TestReportsCatalogParity(t *testing.T) {
	for k, v := range trReports {
		if enReports[k] == "" || v == "" {
			t.Errorf("key %s: tr %q en %q", k, v, enReports[k])
		}
		if Translate(LocaleTR, k) != v || Translate(LocaleEN, k) != enReports[k] {
			t.Errorf("key %s is not merged into the catalogs", k)
		}
	}
	for k := range enReports {
		if _, ok := trReports[k]; !ok {
			t.Errorf("tr missing %s", k)
		}
	}
}
