package i18n

import "testing"

// TestPlatformHealthCatalogParity: the platform health keys exist in both
// locales and reach the merged catalogs.
func TestPlatformHealthCatalogParity(t *testing.T) {
	for k, v := range trPlatformHealth {
		if enPlatformHealth[k] == "" || v == "" {
			t.Errorf("key %s: tr %q en %q", k, v, enPlatformHealth[k])
		}
		if Translate(LocaleTR, k) != v || Translate(LocaleEN, k) != enPlatformHealth[k] {
			t.Errorf("key %s is not merged into the catalogs", k)
		}
	}
	for k := range enPlatformHealth {
		if _, ok := trPlatformHealth[k]; !ok {
			t.Errorf("tr missing %s", k)
		}
	}
}
