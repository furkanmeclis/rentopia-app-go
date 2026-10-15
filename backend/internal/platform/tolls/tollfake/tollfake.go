// Package tollfake is the development / test toll provider (hgs_fake). It
// is registered outside production only.
//
// Without a script it generates a deterministic, plausible history for the
// tags it is asked about: 1–2 passes per tag and day at well-known bridges
// and motorways, each with a stable transaction id, so fetching the same
// window again returns the same passes. Tests script exact passes
// (Script) and failures (FailNext) instead.
package tollfake

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tolls"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// ProviderCode is the provider code of the fake.
const ProviderCode = "hgs_fake"

type station struct {
	entry, exit string
	minor       int64
}

// stations are realistic 2026-style tariffs of a car (class 1).
var stations = []station{
	{"15 Temmuz Şehitler Köprüsü", "", 4700},
	{"Fatih Sultan Mehmet Köprüsü", "", 4700},
	{"Yavuz Sultan Selim Köprüsü", "", 9500},
	{"Osmangazi Köprüsü", "", 79500},
	{"Avrasya Tüneli", "", 25000},
	{"Gebze", "Orhangazi", 41250},
	{"Çamlıca", "Kurtköy", 3150},
	{"Ankara Batı", "Gölbaşı", 2400},
}

// Provider is the fake toll provider.
type Provider struct {
	mu       sync.Mutex
	scripted []tolls.Pass
	scriptOn bool
	fail     []error
	calls    int
	now      func() time.Time
}

// New builds the fake.
func New() *Provider { return &Provider{now: time.Now} }

// Kind implements integrations.Adapter.
func (p *Provider) Kind() string { return tolls.Kind }

// Provider implements integrations.Adapter.
func (p *Provider) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (p *Provider) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "api_key", LabelKey: "payments.integrations.fields.api_key", Secret: true, Required: true},
		{Key: "customer_no", LabelKey: "payments.integrations.fields.customer_no", Secret: false, Required: false},
	}
}

// Check accepts any key except "invalid" (a rejected key).
func (p *Provider) Check(_ context.Context, account integrations.Account) error {
	if strings.EqualFold(strings.TrimSpace(account.Credentials["api_key"]), "invalid") {
		return fmt.Errorf("%w: invalid api key", tolls.ErrRejected)
	}
	return nil
}

// SetClock replaces the clock (no pass is generated after "now").
func (p *Provider) SetClock(now func() time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.now = now
}

// Script makes the fake answer exactly these passes (those in the window)
// from now on; Script() with no passes answers an empty list.
func (p *Provider) Script(passes ...tolls.Pass) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scriptOn = true
	p.scripted = append([]tolls.Pass(nil), passes...)
}

// FailNext makes the next len(errs) fetches fail with these errors.
func (p *Provider) FailNext(errs ...error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fail = append(p.fail, errs...)
}

// Calls is the number of fetches so far.
func (p *Provider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// FetchPasses implements tolls.Provider.
func (p *Provider) FetchPasses(ctx context.Context, account integrations.Account, req tolls.FetchRequest) ([]tolls.Pass, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := p.Check(ctx, account); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if len(p.fail) > 0 {
		err := p.fail[0]
		p.fail = p.fail[1:]
		return nil, err
	}
	if p.scriptOn {
		out := []tolls.Pass{}
		for _, ps := range p.scripted {
			if !ps.PassedAt.Before(req.From) && ps.PassedAt.Before(req.To) {
				out = append(out, ps)
			}
		}
		return out, nil
	}
	return generate(req, p.now()), nil
}

// generate builds the deterministic history of the requested tags.
func generate(req tolls.FetchRequest, now time.Time) []tolls.Pass {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		loc = time.FixedZone("TRT", 3*3600)
	}
	out := []tolls.Pass{}
	if !req.To.After(req.From) {
		return out
	}
	start := req.From.In(loc)
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	for ; day.Before(req.To); day = day.AddDate(0, 0, 1) {
		for _, tag := range req.Tags {
			key := strings.ToUpper(strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
					return r
				}
				return -1
			}, tag))
			if key == "" {
				continue
			}
			seed := hash(key + day.Format("20060102"))
			n := 1 + int(seed%2)
			for i := 0; i < n; i++ {
				st := stations[int((seed>>(8+uint(i)*4))%uint64(len(stations)))]
				minute := int((seed>>(16+uint(i)*8))%(12*60)) + 7*60 + i*95
				at := day.Add(time.Duration(minute) * time.Minute)
				if at.Before(req.From) || !at.Before(req.To) || at.After(now) {
					continue
				}
				out = append(out, tolls.Pass{
					ExternalID: fmt.Sprintf("FAKE-%s-%s-%d", key, day.Format("20060102"), i+1),
					PassedAt:   at.UTC(), TagNumber: tag, Station: st.entry, ExitStation: st.exit,
					Amount: money.New(st.minor), Currency: "TRY",
					Raw: map[string]any{"vehicle_class": 1, "lane": fmt.Sprintf("%d", 1+seed%6)},
				})
			}
		}
	}
	return out
}

func hash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// ErrUnavailable is a convenient transient error for tests.
var ErrUnavailable = errors.New("hgs_fake: service temporarily unavailable")
