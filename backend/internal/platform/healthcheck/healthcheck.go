// Package healthcheck is the pluggable registry behind the platform
// connection health dashboard (TEC-739). A module that talks to the
// outside world (integration accounts and jobs today; the channel manager,
// the notification engine, WhatsApp sessions or custom domains later)
// implements Source and registers it in the Registry at the composition
// root. The dashboard, the issue list and the failed-work alerts read
// every registered source the same way; adding a source needs no change
// in the platform health module.
//
// A source reports platform-wide figures (Metrics) and the individual
// problems that need a platform administrator (Issues). Issues carry the
// organization they belong to, never secrets or personal data: provider,
// subject (a job kind, an account name), the last error text and a
// reference uuid.
package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Severities of an issue or a metric.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Statuses of a source.
const (
	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"
	StatusFailing  = "failing"
	// StatusUnknown: the source could not be read (Collect failed).
	StatusUnknown = "unknown"
)

// Metric is one platform-wide figure of a source ("failed", 3). Severity
// marks a non-zero value as a problem ("" = informational).
type Metric struct {
	Key      string `json:"key"`
	Value    int64  `json:"value"`
	Severity string `json:"severity,omitempty"`
}

// Issue is one problem that needs attention.
type Issue struct {
	// Source is filled by the registry from the source code.
	Source string
	// Kind is a stable machine code ("job_failed", "account_check_failed").
	Kind     string
	Severity string
	// OrganizationID is nil for platform-level work.
	OrganizationID *int64
	Provider       string
	// Subject names the failing thing (a job kind, an account name).
	Subject string
	// Message is the last error text (no secrets).
	Message string
	RefType string
	RefUUID string
	// OccurredAt is when the problem was last seen (the alert sweep
	// reports issues newer than its cursor).
	OccurredAt time.Time
}

// Report is what a source returns from one collection.
type Report struct {
	// Status is optional; when empty the registry derives it from the
	// metrics and issues (critical → failing, warning → degraded).
	Status  string
	Metrics []Metric
	Issues  []Issue
	// Truncated: the source capped its issue list.
	Truncated bool
}

// Source is one health source.
type Source interface {
	// Code is the stable source code (lowercase, underscores).
	Code() string
	// Collect reads the source's current state at now.
	Collect(ctx context.Context, now time.Time) (Report, error)
}

// Result is the outcome of collecting one source.
type Result struct {
	Code     string
	Status   string
	Report   Report
	Err      error
	Duration time.Duration
}

var codeRE = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)

// ErrDuplicateSource: a source with the code is already registered.
var ErrDuplicateSource = errors.New("healthcheck: duplicate source")

// ErrInvalidSource: nil source or malformed code.
var ErrInvalidSource = errors.New("healthcheck: invalid source")

// Registry holds the sources in registration order. It is safe for
// concurrent use.
type Registry struct {
	mu      sync.RWMutex
	sources []Source
	timeout time.Duration
}

// DefaultTimeout bounds one source's Collect.
const DefaultTimeout = 10 * time.Second

// NewRegistry builds a registry; nil sources are skipped, invalid or
// duplicate ones panic (a wiring error at start-up).
func NewRegistry(sources ...Source) *Registry {
	r := &Registry{timeout: DefaultTimeout}
	for _, s := range sources {
		if s == nil {
			continue
		}
		if err := r.Register(s); err != nil {
			panic(err)
		}
	}
	return r
}

// SetTimeout changes the per-source timeout (<= 0 keeps the default).
func (r *Registry) SetTimeout(d time.Duration) {
	if d > 0 {
		r.mu.Lock()
		r.timeout = d
		r.mu.Unlock()
	}
}

// Register adds a source.
func (r *Registry) Register(s Source) error {
	if s == nil || !codeRE.MatchString(s.Code()) {
		return ErrInvalidSource
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.sources {
		if existing.Code() == s.Code() {
			return fmt.Errorf("%w: %s", ErrDuplicateSource, s.Code())
		}
	}
	r.sources = append(r.sources, s)
	return nil
}

// Codes lists the registered source codes in registration order.
func (r *Registry) Codes() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.sources))
	for _, s := range r.sources {
		out = append(out, s.Code())
	}
	return out
}

// Collect reads every source concurrently (each bounded by the timeout)
// and returns the results in registration order. A failing source yields
// StatusUnknown with its error; the others are unaffected.
func (r *Registry) Collect(ctx context.Context, now time.Time) []Result {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	sources := append([]Source(nil), r.sources...)
	timeout := r.timeout
	r.mu.RUnlock()

	out := make([]Result, len(sources))
	var wg sync.WaitGroup
	for i, s := range sources {
		wg.Add(1)
		go func(i int, s Source) {
			defer wg.Done()
			out[i] = collectOne(ctx, s, now, timeout)
		}(i, s)
	}
	wg.Wait()
	return out
}

func collectOne(ctx context.Context, s Source, now time.Time, timeout time.Duration) (res Result) {
	res.Code = s.Code()
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			res = Result{Code: s.Code(), Status: StatusUnknown, Err: fmt.Errorf("healthcheck: source %s panicked: %v", s.Code(), p)}
		}
		res.Duration = time.Since(start)
	}()
	rep, err := s.Collect(cctx, now)
	if err != nil {
		return Result{Code: s.Code(), Status: StatusUnknown, Err: err}
	}
	for i := range rep.Issues {
		rep.Issues[i].Source = s.Code()
		if rep.Issues[i].Severity == "" {
			rep.Issues[i].Severity = SeverityWarning
		}
	}
	res.Report = rep
	res.Status = rep.Status
	if res.Status == "" {
		res.Status = DeriveStatus(rep)
	}
	return res
}

// DeriveStatus: any critical issue or non-zero critical metric → failing;
// any warning → degraded; else healthy.
func DeriveStatus(rep Report) string {
	worst := ""
	note := func(sev string) {
		switch sev {
		case SeverityCritical:
			worst = SeverityCritical
		case SeverityWarning:
			if worst == "" {
				worst = SeverityWarning
			}
		}
	}
	for _, m := range rep.Metrics {
		if m.Value > 0 {
			note(m.Severity)
		}
	}
	for _, is := range rep.Issues {
		note(is.Severity)
	}
	switch worst {
	case SeverityCritical:
		return StatusFailing
	case SeverityWarning:
		return StatusDegraded
	default:
		return StatusHealthy
	}
}

// WorstStatus folds source statuses into the platform status (failing >
// unknown > degraded > healthy).
func WorstStatus(statuses ...string) string {
	rank := map[string]int{StatusHealthy: 0, StatusDegraded: 1, StatusUnknown: 2, StatusFailing: 3}
	worst := StatusHealthy
	for _, s := range statuses {
		if rank[s] > rank[worst] {
			worst = s
		}
	}
	return worst
}

// SeverityRank orders severities (critical first).
func SeverityRank(sev string) int {
	switch sev {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

// SortIssues orders issues newest first, then by severity.
func SortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if !issues[i].OccurredAt.Equal(issues[j].OccurredAt) {
			return issues[i].OccurredAt.After(issues[j].OccurredAt)
		}
		return SeverityRank(issues[i].Severity) < SeverityRank(issues[j].Severity)
	})
}
