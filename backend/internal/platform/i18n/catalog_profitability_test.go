package i18n

import "testing"

// TestProfitabilityCatalogParity: the profitability keys exist in both
// locales, are translated and reach the merged catalogs.
func TestProfitabilityCatalogParity(t *testing.T) {
	for k, v := range trProfitability {
		if enProfitability[k] == "" || v == "" {
			t.Errorf("key %s: tr %q en %q", k, v, enProfitability[k])
		}
		if Translate(LocaleTR, k) != v || Translate(LocaleEN, k) != enProfitability[k] {
			t.Errorf("key %s is not merged into the catalogs", k)
		}
	}
	for k := range enProfitability {
		if _, ok := trProfitability[k]; !ok {
			t.Errorf("tr missing %s", k)
		}
	}
}
