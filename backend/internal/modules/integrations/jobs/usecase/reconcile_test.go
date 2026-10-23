package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

func TestCanReconcile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind, direction, status string
		want                    bool
	}{
		{"sms.send", "outbound", usecase.StatusPending, true},
		{"sms.send", "outbound", usecase.StatusSubmitted, true},
		{"sms.send", "outbound", usecase.StatusNeedsVerification, true},
		{"einvoice.send", "outbound", usecase.StatusFailed, true},
		{"sms.send", "outbound", usecase.StatusSucceeded, false},
		{"sms.send", "outbound", usecase.StatusReconciled, false},
		{"sms.send", "outbound", usecase.StatusReportedManually, false},
		{"sms.send", "outbound", usecase.StatusIgnored, false},
		{"einvoice.send", "outbound", usecase.StatusWaitingDependency, false},
		{"einvoice.send", "outbound", usecase.StatusBlocked, false},
		{"payment.webhook", "inbound", usecase.StatusFailed, false},
		{"payment.refund", "outbound", usecase.StatusFailed, false},
		{"kabis.start", "outbound", usecase.StatusNeedsVerification, false},
	}
	for _, c := range cases {
		if got := usecase.CanReconcile(c.kind, c.direction, c.status); got != c.want {
			t.Errorf("CanReconcile(%s, %s, %s) = %v, want %v", c.kind, c.direction, c.status, got, c.want)
		}
	}
}

func TestBulkReconcileFilterValidate(t *testing.T) {
	t.Parallel()
	ok := usecase.BulkReconcileFilter{Before: time.Now(), Statuses: []string{"pending"}}
	if f, _ := ok.Validate(); f != "" {
		t.Fatalf("valid filter refused on %s", f)
	}
	for name, f := range map[string]usecase.BulkReconcileFilter{
		"before":   {Statuses: []string{"pending"}},
		"statuses": {Before: time.Now()},
		"final":    {Before: time.Now(), Statuses: []string{"succeeded"}},
		"kinds":    {Before: time.Now(), Statuses: []string{"pending"}, Kinds: []string{""}},
	} {
		if field, _ := f.Validate(); field == "" {
			t.Errorf("%s: accepted %+v", name, f)
		}
	}
}

// A job whose outcome is unknown is marked reconciled: who / when / why
// are kept, the runner never calls the provider for it again, a second
// reconcile and a retry are refused.
func TestRunnerReconcile(t *testing.T) {
	op := &fakeOp{submits: []integrations.Result{{Outcome: integrations.OutcomeUncertain}}}
	e := newEnv(t, op)
	j := e.enqueue(t)
	if d := e.detail(t, j.UUID); d.Status != usecase.StatusNeedsVerification || !d.Reconcilable {
		t.Fatalf("before: %s reconcilable=%v", d.Status, d.Reconcilable)
	}
	res, err := e.runner.Reconcile(e.ctx, e.org, j.UUID, usecase.ReconcileInput{Note: "seen in the provider panel", ExternalRef: "NG-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.PreviousStatus != usecase.StatusNeedsVerification {
		t.Fatalf("previous status %s", res.PreviousStatus)
	}
	d := e.detail(t, j.UUID)
	if d.Status != usecase.StatusReconciled || d.Reconcilable || d.Retryable || d.NextAttemptAt != nil {
		t.Fatalf("after: %+v", d)
	}
	if d.Reconciliation == nil || d.Reconciliation.Note != "seen in the provider panel" || d.Reconciliation.ExternalRef != "NG-1" {
		t.Fatalf("reconciliation: %+v", d.Reconciliation)
	}
	if _, err := e.runner.Process(e.ctx, e.org, jobID(t, e, j.UUID), usecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	if op.nSubmit != 1 || op.nCheck != 0 {
		t.Fatalf("provider called after reconcile: submits=%d checks=%d", op.nSubmit, op.nCheck)
	}
	if _, err := e.runner.Reconcile(e.ctx, e.org, j.UUID, usecase.ReconcileInput{Note: "again"}); !errors.Is(err, usecase.ErrNotReconcilable) {
		t.Fatalf("second reconcile: %v", err)
	}
	if _, err := e.runner.Retry(e.ctx, e.org, j.UUID); !errors.Is(err, usecase.ErrNotRetryable) {
		t.Fatalf("retry of a reconciled job: %v", err)
	}
	// Another organization's uuid answers not found.
	other := newEnv(t, op)
	if _, err := other.runner.Reconcile(other.ctx, other.org, j.UUID, usecase.ReconcileInput{Note: "nope"}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("reconcile from another organization: %v", err)
	}
}

// A succeeded job is final; a job held by a worker answers busy.
func TestRunnerReconcileRefusals(t *testing.T) {
	op := &fakeOp{submits: []integrations.Result{{Outcome: integrations.OutcomeSucceeded}}}
	e := newEnv(t, op)
	done := e.enqueue(t)
	if _, err := e.runner.Reconcile(e.ctx, e.org, done.UUID, usecase.ReconcileInput{Note: "done"}); !errors.Is(err, usecase.ErrNotReconcilable) {
		t.Fatalf("reconcile of a succeeded job: %v", err)
	}
	op2 := &fakeOp{submits: []integrations.Result{{Outcome: integrations.OutcomeUncertain}}}
	e2 := newEnv(t, op2)
	held := e2.enqueue(t)
	if _, err := e2.pool.Exec(e2.ctx, "UPDATE integration_jobs SET locked_until = NOW() + interval '1 minute' WHERE organization_id = $1 AND uuid = $2",
		e2.org, held.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := e2.runner.Reconcile(e2.ctx, e2.org, held.UUID, usecase.ReconcileInput{Note: "held"}); !errors.Is(err, usecase.ErrBusy) {
		t.Fatalf("reconcile of a held job: %v", err)
	}
}

// The bulk reconcile takes the matching jobs created before the cut-off
// and leaves later ones, other states and other organizations alone.
func TestRunnerReconcileMatching(t *testing.T) {
	op := &fakeOp{submits: []integrations.Result{{Outcome: integrations.OutcomeUncertain}}}
	e := newEnv(t, op)
	a := e.enqueue(t)
	b := e.enqueue(t)
	cut := time.Now()
	if _, err := e.pool.Exec(e.ctx, "UPDATE integration_jobs SET created_at = $3 WHERE organization_id = $1 AND uuid = ANY($2)",
		e.org, []any{a.UUID, b.UUID}, cut.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	later := e.enqueue(t) // created after the cut-off
	other := newEnv(t, op)
	foreign := other.enqueue(t)
	if _, err := other.pool.Exec(other.ctx, "UPDATE integration_jobs SET created_at = $3 WHERE organization_id = $1 AND uuid = $2",
		other.org, foreign.UUID, cut.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	f := usecase.BulkReconcileFilter{Before: cut, Statuses: []string{usecase.StatusNeedsVerification}, Kinds: []string{"test.op"}}
	res, err := e.runner.ReconcileMatching(e.ctx, e.org, f, usecase.ReconcileInput{Note: "restore: confirmed in panel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("reconciled %d jobs, want 2", len(res))
	}
	if d := e.detail(t, a.UUID); d.Status != usecase.StatusReconciled {
		t.Fatalf("a: %s", d.Status)
	}
	if d := e.detail(t, later.UUID); d.Status != usecase.StatusNeedsVerification {
		t.Fatalf("later job touched: %s", d.Status)
	}
	if d, err := other.runner.Detail(other.ctx, other.org, foreign.UUID); err != nil || d.Status != usecase.StatusNeedsVerification {
		t.Fatalf("other organization touched: %v %s", err, d.Status)
	}
	// The operator listing sees the other organization's job.
	c, err := e.runner.ReconcileCandidates(e.ctx, usecase.BulkReconcileFilter{Before: cut, Statuses: []string{usecase.StatusNeedsVerification}, Kinds: []string{"test.op"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range c {
		if x.UUID == foreign.UUID {
			found = true
		}
		if x.UUID == a.UUID {
			t.Fatal("a reconciled job is still a candidate")
		}
	}
	if !found {
		t.Fatal("candidate of another organization missing")
	}
}
