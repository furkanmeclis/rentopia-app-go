package features

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// seedMigration seeds the features table (migration 000134).
var seedMigration = filepath.Join("..", "..", "..", "migrations", "000134_plans_features.up.sql")

// frontendCatalog is the panel's copy of the keys.
var frontendCatalog = filepath.Join("..", "..", "..", "..", "frontend", "src", "config", "features.ts")

var quoted = regexp.MustCompile(`'([^']+)'|"([^"]+)"`)

func literals(s string) []string {
	out := []string{}
	for _, m := range quoted.FindAllStringSubmatch(s, -1) {
		if m[1] != "" {
			out = append(out, m[1])
		} else {
			out = append(out, m[2])
		}
	}
	return out
}

func sameKeys(t *testing.T, source string, got []string) {
	t.Helper()
	want := AllStrings()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s lists %v; the Go catalog (same order) is %v", source, got, want)
	}
}

// The Go catalog and the migration seed list the same keys in the same
// order (the seed's sort_order is the display order).
func TestCatalogMatchesMigrationSeed(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(seedMigration)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)INSERT INTO features \(key, sort_order\) VALUES(.*?)ON CONFLICT`).FindSubmatch(raw)
	if m == nil {
		t.Fatal("features seed not found in the migration")
	}
	keys := []string{}
	for _, row := range regexp.MustCompile(`\('([^']+)',\s*\d+\)`).FindAllSubmatch(m[1], -1) {
		keys = append(keys, string(row[1]))
	}
	sameKeys(t, seedMigration, keys)
}

// frontend/src/config/features.ts keeps the same list (FEATURE_KEYS).
func TestCatalogMatchesFrontend(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(frontendCatalog)
	if os.IsNotExist(err) {
		t.Skip("frontend not checked out")
	}
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)export const FEATURE_KEYS = \[(.*?)\]`).FindSubmatch(raw)
	if m == nil {
		t.Fatal("FEATURE_KEYS not found in " + frontendCatalog)
	}
	sameKeys(t, frontendCatalog, literals(string(m[1])))
}
