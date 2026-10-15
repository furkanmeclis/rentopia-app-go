// Package kabisfake is the development / test KABİS adapter (kabis_fake).
// It is registered outside production only, so it is never selectable
// there, and it never contacts the authority.
//
// It keeps the notifications it "received" in memory (by our reference),
// so a lookup answers like a real status query. The account's non-secret
// "scenario" field drives development checks: "" / "ok" files normally,
// "outage" refuses before sending (ErrUnavailable), "timeout" files the
// notification but answers no receipt (ErrTimeout: outcome unknown, the
// lookup then finds it), "reject" refuses the notification. Tests script
// exact failures with FailNextReport / FailNextLookup.
package kabisfake

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
)

// ProviderCode is the provider code of the fake.
const ProviderCode = "kabis_fake"

// Scenarios of the account's "scenario" field.
const (
	ScenarioOK      = "ok"
	ScenarioOutage  = "outage"
	ScenarioTimeout = "timeout"
	ScenarioReject  = "reject"
)

type reportFailure struct {
	err      error
	recorded bool
}

// Provider is the fake KABİS adapter.
type Provider struct {
	mu          sync.Mutex
	records     map[string]kabis.Notification
	refs        map[string]string
	reportFails []reportFailure
	lookupFails []error
	reports     int
	lookups     int
	seq         int
}

// New builds the fake.
func New() *Provider {
	return &Provider{records: map[string]kabis.Notification{}, refs: map[string]string{}}
}

// Kind implements integrations.Adapter.
func (p *Provider) Kind() string { return kabis.Kind }

// Provider implements integrations.Adapter.
func (p *Provider) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (p *Provider) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "username", LabelKey: "payments.integrations.fields.username", Secret: false, Required: true},
		{Key: "password", LabelKey: "payments.integrations.fields.password", Secret: true, Required: true},
		{Key: "scenario", LabelKey: "payments.integrations.fields.scenario", Secret: false, Required: false},
	}
}

// Check accepts any password except "invalid".
func (p *Provider) Check(_ context.Context, account integrations.Account) error {
	if strings.EqualFold(strings.TrimSpace(account.Credentials["password"]), "invalid") {
		return fmt.Errorf("%w: invalid credentials", kabis.ErrRejected)
	}
	if scenario(account) == ScenarioOutage {
		return kabis.ErrUnavailable
	}
	return nil
}

// Ready implements kabis.Provider: the fake can always send.
func (p *Provider) Ready() error { return nil }

func scenario(account integrations.Account) string {
	return strings.ToLower(strings.TrimSpace(account.Credentials["scenario"]))
}

// FailNextReport makes the next report fail with err; recorded = the fake
// keeps the notification anyway (a timeout after the authority stored it).
func (p *Provider) FailNextReport(err error, recorded bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reportFails = append(p.reportFails, reportFailure{err: err, recorded: recorded})
}

// FailNextLookup makes the next lookups fail with these errors.
func (p *Provider) FailNextLookup(errs ...error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lookupFails = append(p.lookupFails, errs...)
}

// Reports is the number of report calls so far.
func (p *Provider) Reports() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reports
}

// Lookups is the number of lookup calls so far.
func (p *Provider) Lookups() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lookups
}

// Received returns the notification the fake holds for a reference.
func (p *Provider) Received(reference string) (kabis.Notification, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n, ok := p.records[reference]
	return n, ok
}

func (p *Provider) record(n kabis.Notification) string {
	if ref, ok := p.refs[n.Reference]; ok {
		return ref
	}
	p.seq++
	ref := fmt.Sprintf("FAKE-KABIS-%06d", p.seq)
	p.records[n.Reference] = n
	p.refs[n.Reference] = ref
	return ref
}

// Report implements kabis.Provider.
func (p *Provider) Report(ctx context.Context, account integrations.Account, n kabis.Notification) (kabis.Receipt, error) {
	if err := ctx.Err(); err != nil {
		return kabis.Receipt{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reports++
	if len(p.reportFails) > 0 {
		f := p.reportFails[0]
		p.reportFails = p.reportFails[1:]
		if f.recorded {
			p.record(n)
		}
		return kabis.Receipt{}, f.err
	}
	switch scenario(account) {
	case ScenarioOutage:
		return kabis.Receipt{}, kabis.ErrUnavailable
	case ScenarioReject:
		return kabis.Receipt{}, fmt.Errorf("%w: test scenario", kabis.ErrRejected)
	case ScenarioTimeout:
		p.record(n)
		return kabis.Receipt{}, kabis.ErrTimeout
	}
	return kabis.Receipt{Reference: p.record(n)}, nil
}

// Lookup implements kabis.Provider.
func (p *Provider) Lookup(ctx context.Context, account integrations.Account, reference, _ string) (kabis.LookupResult, error) {
	if err := ctx.Err(); err != nil {
		return kabis.LookupResult{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lookups++
	if len(p.lookupFails) > 0 {
		err := p.lookupFails[0]
		p.lookupFails = p.lookupFails[1:]
		return kabis.LookupResult{}, err
	}
	if scenario(account) == ScenarioOutage {
		return kabis.LookupResult{}, kabis.ErrUnavailable
	}
	ref, ok := p.refs[reference]
	return kabis.LookupResult{Found: ok, Reference: ref}, nil
}
