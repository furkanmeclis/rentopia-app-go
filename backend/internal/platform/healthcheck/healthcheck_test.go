package healthcheck

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSource struct {
	code  string
	rep   Report
	err   error
	panic bool
	wait  time.Duration
}

func (f fakeSource) Code() string { return f.code }

func (f fakeSource) Collect(ctx context.Context, _ time.Time) (Report, error) {
	if f.panic {
		panic("boom")
	}
	if f.wait > 0 {
		select {
		case <-ctx.Done():
			return Report{}, ctx.Err()
		case <-time.After(f.wait):
		}
	}
	return f.rep, f.err
}

func TestRegisterRejectsDuplicatesAndBadCodes(t *testing.T) {
	r := NewRegistry(fakeSource{code: "jobs"})
	if err := r.Register(fakeSource{code: "jobs"}); !errors.Is(err, ErrDuplicateSource) {
		t.Fatalf("duplicate: got %v", err)
	}
	if err := r.Register(fakeSource{code: "Bad Code"}); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("bad code: got %v", err)
	}
	if err := r.Register(nil); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("nil: got %v", err)
	}
	if err := r.Register(fakeSource{code: "channel_sync"}); err != nil {
		t.Fatal(err)
	}
	if got := r.Codes(); len(got) != 2 || got[0] != "jobs" || got[1] != "channel_sync" {
		t.Fatalf("codes: %v", got)
	}
}

func TestCollectDerivesStatusAndIsolatesFailures(t *testing.T) {
	now := time.Now()
	r := NewRegistry(
		fakeSource{code: "ok_source", rep: Report{Metrics: []Metric{{Key: "failed", Value: 0, Severity: SeverityCritical}}}},
		fakeSource{code: "warn_source", rep: Report{Issues: []Issue{{Kind: "x"}}}},
		fakeSource{code: "crit_source", rep: Report{Metrics: []Metric{{Key: "failed", Value: 2, Severity: SeverityCritical}}}},
		fakeSource{code: "err_source", err: errors.New("down")},
		fakeSource{code: "panic_source", panic: true},
		fakeSource{code: "slow_source", wait: time.Second},
	)
	r.SetTimeout(50 * time.Millisecond)
	res := r.Collect(context.Background(), now)
	want := []string{StatusHealthy, StatusDegraded, StatusFailing, StatusUnknown, StatusUnknown, StatusUnknown}
	for i, w := range want {
		if res[i].Status != w {
			t.Errorf("%s: status %s, want %s (err %v)", res[i].Code, res[i].Status, w, res[i].Err)
		}
	}
	if got := res[1].Report.Issues[0]; got.Source != "warn_source" || got.Severity != SeverityWarning {
		t.Fatalf("issue defaults: %+v", got)
	}
	statuses := make([]string, 0, len(res))
	for _, r := range res {
		statuses = append(statuses, r.Status)
	}
	if WorstStatus(statuses...) != StatusFailing {
		t.Fatal("worst status")
	}
	if WorstStatus(StatusHealthy, StatusUnknown, StatusDegraded) != StatusUnknown {
		t.Fatal("unknown outranks degraded")
	}
}

func TestSortIssues(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []Issue{
		{Kind: "old", OccurredAt: t0},
		{Kind: "new_warn", Severity: SeverityWarning, OccurredAt: t0.Add(time.Hour)},
		{Kind: "new_crit", Severity: SeverityCritical, OccurredAt: t0.Add(time.Hour)},
	}
	SortIssues(in)
	if in[0].Kind != "new_crit" || in[1].Kind != "new_warn" || in[2].Kind != "old" {
		t.Fatalf("order: %v %v %v", in[0].Kind, in[1].Kind, in[2].Kind)
	}
}
