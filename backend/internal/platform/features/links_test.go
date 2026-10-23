package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RT-A28: the customer's contract, document and payment links and the
// customer portal work on every plan. Their route files must never mount a
// feature gate.
func TestCustomerLinksAreNeverGated(t *testing.T) {
	t.Parallel()
	modules := filepath.Join("..", "..", "modules")
	for _, file := range []string{
		filepath.Join(modules, "customerportal", "routes.go"),
		filepath.Join(modules, "contracts", "routes.go"),
		filepath.Join(modules, "payments", "routes.go"),
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		for _, marker := range []string{"RequireFeature", "RequireAnyFeature", "platform/features"} {
			if strings.Contains(src, marker) {
				t.Errorf("%s mentions %s: customer links are never gated by the plan", file, marker)
			}
		}
	}
}
