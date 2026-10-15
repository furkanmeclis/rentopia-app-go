// Package tolls is the HGS / OGS toll pass provider layer (TEC-725,
// docs/design/tolls.md). The provider is not chosen yet (TEC-683: PTT or a
// bank API, limited to its own tags), so the module depends only on this
// interface; hgs_fake (package tollfake) serves tests and local
// development, and file import (ioengine) is the operational fallback.
//
// A provider is an integrations.Adapter (an organization connects it with
// its own credentials, sealed by the integrations Vault) that can list the
// passes of the organization's tags in a time window. Fetching is read
// only: asking again for the same window is always safe, and a pass that
// comes back is recognised by its transaction id.
package tolls

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Kind is the integration kind of toll providers.
const Kind = "toll"

// Pass is one toll pass as a provider reports it.
type Pass struct {
	// ExternalID is the provider's transaction id (unique per provider).
	ExternalID string
	PassedAt   time.Time
	Plate      string
	TagNumber  string
	// Station is the bridge / plaza, or the entry of a motorway route.
	Station string
	// ExitStation is the exit of a route ("" for a single station).
	ExitStation string
	Amount      money.Money
	Currency    string
	// Raw holds non-sensitive provider fields for reference.
	Raw map[string]any
}

// FetchRequest asks for the passes of the given tags (and plates, for
// providers that look up by plate) in [From, To).
type FetchRequest struct {
	From, To time.Time
	Tags     []string
	Plates   []string
}

// Provider lists toll passes for an organization's account.
type Provider interface {
	integrations.Adapter
	// FetchPasses returns the passes in the window. An error means the
	// list is incomplete: nothing is concluded from it (the job retries).
	FetchPasses(ctx context.Context, account integrations.Account, req FetchRequest) ([]Pass, error)
}

// ErrRejected is a definitive provider refusal (bad credentials, unknown
// account): retrying the same request does not help.
var ErrRejected = errors.New("tolls: the provider rejected the request")
