// Package channelfake is the development / test channel provider
// (yolcu360_fake, obilet_fake). It is registered outside production only.
// One implementation stands in for every channel: New is the Yolcu360 fake,
// NewObilet the Obilet fake (TEC-737); only the provider code, the channel
// and the prefix of simulated booking numbers differ.
//
// It keeps, per account, the last inventory push, a queue of booking
// messages (scripted by tests with Enqueue or queued by the panel's
// "simulate booking" action), the acknowledgements it received and a
// statement derived from the latest state of every booking it sent. The
// credential scenario selects a behaviour: "" / ok, outage (every call
// fails before sending), reject (every call is refused).
package channelfake

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/google/uuid"
)

// ProviderCode is the provider code of the Yolcu360 fake.
const ProviderCode = "yolcu360_fake"

// Channel is the sales channel the Yolcu360 fake stands in for.
const Channel = "yolcu360"

// ObiletProviderCode is the provider code of the Obilet fake.
const ObiletProviderCode = "obilet_fake"

// ObiletChannel is the sales channel the Obilet fake stands in for.
const ObiletChannel = "obilet"

type state struct {
	lastPush *channels.InventoryPush
	pushes   int
	queue    []channels.Message
	sent     map[string]channels.Message // latest per booking (statement)
	acks     []channels.Ack
	seq      int
}

// Provider is the fake channel.
type Provider struct {
	provider  string
	channel   string
	refPrefix string
	mu        sync.Mutex
	accounts  map[uuid.UUID]*state
	failPush  []error
	failPull  []error
	statement map[uuid.UUID][]channels.StatementLine
}

// New builds the Yolcu360 fake (yolcu360_fake).
func New() *Provider { return newFake(ProviderCode, Channel, "Y360-") }

// NewObilet builds the Obilet fake (obilet_fake).
func NewObilet() *Provider { return newFake(ObiletProviderCode, ObiletChannel, "OBL-") }

func newFake(provider, channel, refPrefix string) *Provider {
	return &Provider{
		provider: provider, channel: channel, refPrefix: refPrefix,
		accounts: map[uuid.UUID]*state{}, statement: map[uuid.UUID][]channels.StatementLine{},
	}
}

func (p *Provider) st(id uuid.UUID) *state {
	s, ok := p.accounts[id]
	if !ok {
		s = &state{sent: map[string]channels.Message{}}
		p.accounts[id] = s
	}
	return s
}

// Kind implements integrations.Adapter.
func (p *Provider) Kind() string { return channels.Kind }

// Provider implements integrations.Adapter.
func (p *Provider) Provider() string { return p.provider }

// Channel implements channels.Provider.
func (p *Provider) Channel() string { return p.channel }

// Fields implements integrations.Adapter.
// The fields mirror the real adapter's identity and key fields.
func (p *Provider) Fields() []integrations.Field {
	if p.channel == ObiletChannel {
		return []integrations.Field{
			{Key: "partner_code", LabelKey: "payments.integrations.fields.partner_code", Secret: false, Required: true},
			{Key: "api_token", LabelKey: "payments.integrations.fields.api_token", Secret: true, Required: true},
			{Key: "scenario", LabelKey: "payments.integrations.fields.scenario", Secret: false, Required: false},
		}
	}
	return []integrations.Field{
		{Key: "supplier_code", LabelKey: "payments.integrations.fields.supplier_code", Secret: false, Required: true},
		{Key: "api_key", LabelKey: "payments.integrations.fields.api_key", Secret: true, Required: true},
		{Key: "scenario", LabelKey: "payments.integrations.fields.scenario", Secret: false, Required: false},
	}
}

// Capabilities implements channels.Provider.
func (p *Provider) Capabilities() channels.Capabilities {
	return channels.Capabilities{
		Availability: true, Rates: true, StopSale: true, Allotment: true, Pull: true, Acknowledge: true, Statement: true,
	}
}

func (p *Provider) scenario(account integrations.Account) error {
	switch strings.ToLower(strings.TrimSpace(account.Credentials["scenario"])) {
	case "outage":
		return fmt.Errorf("%w: %s outage scenario", channels.ErrUnavailable, p.provider)
	case "reject":
		return fmt.Errorf("%w: %s reject scenario", channels.ErrRejected, p.provider)
	}
	if strings.EqualFold(strings.TrimSpace(account.Credentials["api_key"]), "invalid") ||
		strings.EqualFold(strings.TrimSpace(account.Credentials["api_token"]), "invalid") {
		return fmt.Errorf("%w: invalid api key", channels.ErrRejected)
	}
	return nil
}

// Check implements integrations.Adapter.
func (p *Provider) Check(_ context.Context, account integrations.Account) error {
	return p.scenario(account)
}

// FailNextPush makes the next pushes fail with these errors.
func (p *Provider) FailNextPush(errs ...error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failPush = append(p.failPush, errs...)
}

// FailNextPull makes the next pulls fail with these errors.
func (p *Provider) FailNextPull(errs ...error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failPull = append(p.failPull, errs...)
}

// PushInventory implements channels.Provider: items with an empty product
// or location code are refused one by one, the rest is stored.
func (p *Provider) PushInventory(ctx context.Context, account integrations.Account, push channels.InventoryPush) (channels.PushReceipt, error) {
	if err := ctx.Err(); err != nil {
		return channels.PushReceipt{}, err
	}
	if err := p.scenario(account); err != nil {
		return channels.PushReceipt{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.failPush) > 0 {
		err := p.failPush[0]
		p.failPush = p.failPush[1:]
		return channels.PushReceipt{}, err
	}
	s := p.st(account.UUID)
	s.pushes++
	cp := push
	cp.Items = append([]channels.InventoryItem(nil), push.Items...)
	s.lastPush = &cp
	r := channels.PushReceipt{Reference: "FAKE-PUSH-" + strconv.Itoa(s.pushes), Errors: []channels.ItemError{}}
	for i, it := range push.Items {
		if it.ProductCode == "" || it.LocationCode == "" {
			r.Rejected++
			r.Errors = append(r.Errors, channels.ItemError{Index: i, Code: "unknown_code", Message: "product or location code missing"})
			continue
		}
		r.Accepted++
	}
	return r, nil
}

// LastPush returns the last snapshot pushed for the account (nil = none)
// and the number of pushes.
func (p *Provider) LastPush(account uuid.UUID) (*channels.InventoryPush, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.st(account)
	return s.lastPush, s.pushes
}

// Enqueue queues messages for the account's next pull (tests). A message
// without an id gets one.
func (p *Provider) Enqueue(account uuid.UUID, msgs ...channels.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.st(account)
	for _, m := range msgs {
		s.seq++
		if m.ID == "" {
			m.ID = "FAKE-EVT-" + strconv.Itoa(s.seq) + "-" + uuid.NewString()[:8]
		}
		s.queue = append(s.queue, m)
	}
}

// Simulate implements channels.Simulator: the message is queued with an id,
// a version after the booking's last one and defaults for missing facts.
func (p *Provider) Simulate(account integrations.Account, m channels.Message) channels.Message {
	p.mu.Lock()
	s := p.st(account.UUID)
	if m.ExternalRef == "" {
		m.ExternalRef = p.refPrefix + strings.ToUpper(uuid.NewString()[:8])
	}
	if m.Version == 0 {
		var latest int64
		for _, q := range s.queue {
			if q.ExternalRef == m.ExternalRef && q.Version > latest {
				latest = q.Version
			}
		}
		if prev, ok := s.sent[m.ExternalRef]; ok && prev.Version > latest {
			latest = prev.Version
		}
		m.Version = latest + 1
	}
	if m.OccurredAt.IsZero() {
		m.OccurredAt = time.Now().UTC()
	}
	if m.Status == "" {
		m.Status = channels.StatusConfirmed
		if m.Type == channels.MessageCancellation {
			m.Status = channels.StatusCancelled
		}
	}
	p.mu.Unlock()
	p.Enqueue(account.UUID, m)
	p.mu.Lock()
	defer p.mu.Unlock()
	q := p.st(account.UUID).queue
	return q[len(q)-1]
}

// PullMessages implements channels.Provider. The cursor is the number of
// messages already read; messages stay readable (a repeated pull with an
// old cursor answers them again).
func (p *Provider) PullMessages(ctx context.Context, account integrations.Account, cursor string) (channels.MessageBatch, error) {
	if err := ctx.Err(); err != nil {
		return channels.MessageBatch{}, err
	}
	if err := p.scenario(account); err != nil {
		return channels.MessageBatch{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.failPull) > 0 {
		err := p.failPull[0]
		p.failPull = p.failPull[1:]
		return channels.MessageBatch{}, err
	}
	s := p.st(account.UUID)
	from, _ := strconv.Atoi(cursor)
	if from < 0 || from > len(s.queue) {
		from = 0
	}
	const page = 50
	to := min(from+page, len(s.queue))
	out := append([]channels.Message(nil), s.queue[from:to]...)
	for _, m := range out {
		if prev, ok := s.sent[m.ExternalRef]; !ok || m.Version >= prev.Version {
			s.sent[m.ExternalRef] = m
		}
	}
	return channels.MessageBatch{Messages: out, Cursor: strconv.Itoa(to), More: to < len(s.queue)}, nil
}

// Acknowledge implements channels.Provider.
func (p *Provider) Acknowledge(_ context.Context, account integrations.Account, acks []channels.Ack) error {
	if err := p.scenario(account); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.st(account.UUID)
	s.acks = append(s.acks, acks...)
	return nil
}

// Acks returns the acknowledgements received for the account.
func (p *Provider) Acks(account uuid.UUID) []channels.Ack {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]channels.Ack(nil), p.st(account).acks...)
}

// ScriptStatement replaces the derived statement of the account (tests).
func (p *Provider) ScriptStatement(account uuid.UUID, lines ...channels.StatementLine) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statement[account] = append([]channels.StatementLine(nil), lines...)
}

// Statement implements channels.Provider: the scripted lines, else the
// latest state of every booking the fake has sent, picked up in [from, to).
func (p *Provider) Statement(_ context.Context, account integrations.Account, from, to time.Time) ([]channels.StatementLine, error) {
	if err := p.scenario(account); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	lines, scripted := p.statement[account.UUID]
	if !scripted {
		for _, m := range p.st(account.UUID).sent {
			lines = append(lines, channels.StatementLine{
				ExternalRef: m.ExternalRef, Status: m.Status, StartsAt: m.StartsAt, Currency: m.Currency,
				Total: m.Total, Commission: m.Commission, Net: m.Total.Sub(m.Commission), CollectedBy: m.CollectedBy,
			})
		}
	}
	out := []channels.StatementLine{}
	for _, l := range lines {
		if !l.StartsAt.Before(from) && l.StartsAt.Before(to) {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExternalRef < out[j].ExternalRef })
	return out, nil
}
