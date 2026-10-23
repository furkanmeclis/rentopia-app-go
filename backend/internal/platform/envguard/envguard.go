// Package envguard holds the deployment environment rules (APP_ENV) shared
// by the API, the worker and the provider adapters (TEC-744,
// docs/operations/observability.md):
//
//   - APP_ENV is one of development, staging, production (test is accepted
//     for test runs); anything else is a configuration error.
//   - Fake / log providers (kabis_fake, einvoice_fake, the SMS log
//     provider …) are registered outside production only.
//   - Outside production the official endpoints of KABİS, the e-invoice
//     integrator and Netgsm are refused (a restored production database on
//     staging carries live integration accounts). An explicit
//     <PROVIDER>_ALLOW_PRODUCTION_IN_NON_PROD=true lifts the refusal for one
//     provider, e.g. for a supervised acceptance run.
package envguard

import (
	"errors"
	"fmt"
	"strings"
)

// Environment names (APP_ENV).
const (
	Development = "development"
	Staging     = "staging"
	Production  = "production"
	// Test is used by test runs only.
	Test = "test"
)

// Normalize lower-cases and trims APP_ENV; empty means development.
func Normalize(env string) string {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		return Development
	}
	return env
}

// Valid reports whether env (normalized) is a known environment.
func Valid(env string) bool {
	switch Normalize(env) {
	case Development, Staging, Production, Test:
		return true
	}
	return false
}

// IsProduction reports whether env is production.
func IsProduction(env string) bool { return Normalize(env) == Production }

// ErrProductionRefused marks a call to an official (production) provider
// endpoint refused outside production.
var ErrProductionRefused = errors.New("production endpoint refused outside production")

// RefusedError names the provider, the environment and the key that lifts
// the refusal.
type RefusedError struct {
	Provider string
	Env      string
	AllowKey string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("%s: the official (production) endpoint is refused in APP_ENV=%s; use the sandbox / fake adapter or set %s=true",
		e.Provider, e.Env, e.AllowKey)
}

// Unwrap lets errors.Is(err, ErrProductionRefused) match.
func (e *RefusedError) Unwrap() error { return ErrProductionRefused }

// Refusal returns nil when provider may reach its official endpoint: in
// production, or when the operator set allowKey (allowed). Otherwise it
// returns a *RefusedError.
func Refusal(appEnv, provider, allowKey string, allowed bool) error {
	env := Normalize(appEnv)
	if env == Production || allowed {
		return nil
	}
	return &RefusedError{Provider: provider, Env: env, AllowKey: allowKey}
}

// Allow keys of the production guard.
const (
	AllowKabisKey    = "KABIS_ALLOW_PRODUCTION_IN_NON_PROD"
	AllowEInvoiceKey = "EINVOICE_ALLOW_PRODUCTION_IN_NON_PROD"
	AllowNetgsmKey   = "NETGSM_ALLOW_PRODUCTION_IN_NON_PROD"
)
