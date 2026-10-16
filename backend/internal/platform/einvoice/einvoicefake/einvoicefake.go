// Package einvoicefake is the development / test e-invoice adapter
// (einvoice_fake). It is registered outside production only, so it is never
// selectable there, and it never contacts an integrator or GİB.
//
// It keeps the documents it "received" in memory by UUID (ETTN), like an
// integrator: a second send of the same UUID creates nothing new and
// answers the first reference, and a status query answers by UUID. The
// account's non-secret "scenario" field drives development checks:
// "" / "ok" delivers at once, "slow" answers processing on the send and
// succeeded on the first status query, "outage" refuses before sending
// (ErrUnavailable), "timeout" keeps the document but answers no result
// (ErrTimeout: outcome unknown, the status query then finds it), "reject"
// refuses it. Tests script exact failures with FailNextSend /
// FailNextStatus.
//
// Taxpayer lookup: a 10 digit VKN is an e-Fatura taxpayer, an 11 digit
// TCKN is not, unless SetTaxpayer says otherwise.
package einvoicefake

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

// ProviderCode is the provider code of the fake.
const ProviderCode = "einvoice_fake"

// Scenarios of the account's "scenario" field.
const (
	ScenarioOK      = "ok"
	ScenarioSlow    = "slow"
	ScenarioOutage  = "outage"
	ScenarioTimeout = "timeout"
	ScenarioReject  = "reject"
)

type sendFailure struct {
	err      error
	recorded bool
}

type record struct {
	doc       einvoice.Document
	reference string
	state     string
	checks    int
}

// Provider is the fake e-invoice adapter.
type Provider struct {
	mu          sync.Mutex
	docs        map[string]*record
	taxpayers   map[string]bool
	sendFails   []sendFailure
	statusFails []error
	sends       int
	statuses    int
	lookups     int
	cancels     int
	seq         int
}

// New builds the fake.
func New() *Provider {
	return &Provider{docs: map[string]*record{}, taxpayers: map[string]bool{}}
}

// Kind implements integrations.Adapter.
func (p *Provider) Kind() string { return einvoice.Kind }

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

func scenario(account integrations.Account) string {
	return strings.ToLower(strings.TrimSpace(account.Credentials["scenario"]))
}

// Check accepts any password except "invalid".
func (p *Provider) Check(_ context.Context, account integrations.Account) error {
	if strings.EqualFold(strings.TrimSpace(account.Credentials["password"]), "invalid") {
		return fmt.Errorf("%w: invalid credentials", einvoice.ErrRejected)
	}
	if scenario(account) == ScenarioOutage {
		return einvoice.ErrUnavailable
	}
	return nil
}

// Ready implements einvoice.Provider: the fake can always send.
func (p *Provider) Ready() error { return nil }

// SetTaxpayer overrides the lookup answer of an identifier.
func (p *Provider) SetTaxpayer(identifier string, einvoiceUser bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.taxpayers[identifier] = einvoiceUser
}

// FailNextSend makes the next send fail with err; recorded = the fake keeps
// the document anyway (a timeout after the integrator stored it).
func (p *Provider) FailNextSend(err error, recorded bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sendFails = append(p.sendFails, sendFailure{err: err, recorded: recorded})
}

// FailNextStatus makes the next status queries fail with these errors.
func (p *Provider) FailNextStatus(errs ...error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statusFails = append(p.statusFails, errs...)
}

// Sends is the number of send calls so far.
func (p *Provider) Sends() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sends
}

// Statuses is the number of status calls so far.
func (p *Provider) Statuses() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.statuses
}

// Lookups is the number of taxpayer lookups so far.
func (p *Provider) Lookups() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lookups
}

// Documents is the number of distinct documents (UUIDs) the fake holds:
// what an integrator would have issued.
func (p *Provider) Documents() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.docs)
}

// Received returns the document the fake holds for a UUID and its state.
func (p *Provider) Received(uuid string) (einvoice.Document, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.docs[uuid]
	if !ok {
		return einvoice.Document{}, "", false
	}
	return r.doc, r.state, true
}

func (p *Provider) keep(doc einvoice.Document, state string) *record {
	if r, ok := p.docs[doc.UUID]; ok {
		return r
	}
	p.seq++
	r := &record{doc: doc, reference: fmt.Sprintf("FAKE-EINV-%06d", p.seq), state: state}
	p.docs[doc.UUID] = r
	return r
}

// Send implements einvoice.Provider. A UUID it already holds is not issued
// again: the first reference is answered.
func (p *Provider) Send(ctx context.Context, account integrations.Account, doc einvoice.Document) (einvoice.SendResult, error) {
	if err := ctx.Err(); err != nil {
		return einvoice.SendResult{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sends++
	if len(p.sendFails) > 0 {
		f := p.sendFails[0]
		p.sendFails = p.sendFails[1:]
		if f.recorded {
			p.keep(doc, einvoice.StateSucceeded)
		}
		return einvoice.SendResult{}, f.err
	}
	switch scenario(account) {
	case ScenarioOutage:
		return einvoice.SendResult{}, einvoice.ErrUnavailable
	case ScenarioReject:
		return einvoice.SendResult{}, fmt.Errorf("%w: test scenario", einvoice.ErrRejected)
	case ScenarioTimeout:
		p.keep(doc, einvoice.StateSucceeded)
		return einvoice.SendResult{}, einvoice.ErrTimeout
	case ScenarioSlow:
		r := p.keep(doc, einvoice.StateProcessing)
		return einvoice.SendResult{Reference: r.reference, State: r.state}, nil
	}
	r := p.keep(doc, einvoice.StateSucceeded)
	return einvoice.SendResult{Reference: r.reference, State: r.state}, nil
}

// Status implements einvoice.Provider.
func (p *Provider) Status(ctx context.Context, account integrations.Account, uuid, _ string) (einvoice.StatusResult, error) {
	if err := ctx.Err(); err != nil {
		return einvoice.StatusResult{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statuses++
	if len(p.statusFails) > 0 {
		err := p.statusFails[0]
		p.statusFails = p.statusFails[1:]
		return einvoice.StatusResult{}, err
	}
	if scenario(account) == ScenarioOutage {
		return einvoice.StatusResult{}, einvoice.ErrUnavailable
	}
	r, ok := p.docs[uuid]
	if !ok {
		return einvoice.StatusResult{Found: false}, nil
	}
	r.checks++
	if r.state == einvoice.StateProcessing && r.checks >= 1 {
		r.state = einvoice.StateSucceeded
	}
	return einvoice.StatusResult{Found: true, State: r.state, Reference: r.reference}, nil
}

// Cancel implements einvoice.Provider (e-Arşiv only).
func (p *Provider) Cancel(ctx context.Context, _ integrations.Account, uuid, _, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancels++
	r, ok := p.docs[uuid]
	if !ok {
		return fmt.Errorf("%w: unknown document", einvoice.ErrNotCancellable)
	}
	if r.doc.Profile != "EARSIVFATURA" {
		return fmt.Errorf("%w: only e-Arşiv invoices are cancelled", einvoice.ErrNotCancellable)
	}
	r.state = einvoice.StateCancelled
	return nil
}

// Download implements einvoice.Provider: the XML it received; no PDF (the
// module renders its own).
func (p *Provider) Download(_ context.Context, _ integrations.Account, uuid, _, format string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.docs[uuid]
	if !ok {
		return nil, fmt.Errorf("einvoicefake: unknown document %s", uuid)
	}
	if format != einvoice.FormatXML {
		return nil, integrations.ErrUnsupported
	}
	return append([]byte(nil), r.doc.XML...), nil
}

// LookupTaxpayer implements einvoice.Provider.
func (p *Provider) LookupTaxpayer(ctx context.Context, account integrations.Account, identifier string) (einvoice.Taxpayer, error) {
	if err := ctx.Err(); err != nil {
		return einvoice.Taxpayer{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lookups++
	if scenario(account) == ScenarioOutage {
		return einvoice.Taxpayer{}, einvoice.ErrUnavailable
	}
	user, ok := p.taxpayers[identifier]
	if !ok {
		user = len(identifier) == 10
	}
	t := einvoice.Taxpayer{Identifier: identifier, EInvoice: user}
	if user {
		t.Alias = "urn:mail:defaultpk@" + identifier + ".fake"
	}
	return t, nil
}

var _ einvoice.Provider = (*Provider)(nil)
