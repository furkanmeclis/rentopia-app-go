// Command integration-jobs-reconcile marks integration jobs "reconciled"
// across organizations (TEC-962, docs/operations/backup-restore.md RT-A32
// (c)). After a restore to production, outbound jobs that were actually
// carried out after the backup sit in pending / submitted /
// needs_verification and would be sent again once the worker starts. The
// operator compares them with the provider panels and marks the confirmed
// ones reconciled, before the worker starts:
//
//	integration-jobs-reconcile --before 2026-10-23T03:00:00Z                       # dry run: list
//	integration-jobs-reconcile --before ... --kinds sms.send --apply --note "..."  # mark them
//	integration-jobs-reconcile --jobs <uuid>,<uuid> --apply --note "..."           # named jobs
//
// It sends nothing. Payment refunds and KABİS notifications are never
// touched (their own flows). Each job gets the who / when / why columns
// (reconciled_by stays empty: an operator tool), an activity event
// (integrations.job_reconciled, source cli) and the SMS log / e-invoice
// mirrors follow the new state like a panel reconcile.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/handler"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	invoiceusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "integration-jobs-reconcile:", err)
		os.Exit(1)
	}
}

func splitList(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func uuids(s, flagName string) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for _, p := range splitList(s) {
		id, err := uuid.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("--%s: %q is not a uuid", flagName, p)
		}
		out = append(out, id)
	}
	return out, nil
}

func run() error {
	before := flag.String("before", "", "cut-off (RFC 3339): only jobs created before it, e.g. the backup set's UTC time from manifest.txt; default now with --jobs")
	statuses := flag.String("statuses", "pending,submitted,needs_verification", "comma-separated states among pending, submitted, needs_verification, failed")
	kinds := flag.String("kinds", "", "comma-separated job kinds (e.g. sms.send,einvoice.send); empty = every kind")
	orgs := flag.String("orgs", "", "comma-separated organization uuids; empty = every organization")
	jobs := flag.String("jobs", "", "comma-separated job uuids confirmed with the provider")
	note := flag.String("note", "", "why (3-500 characters); required with --apply")
	apply := flag.Bool("apply", false, "mark the listed jobs reconciled (default: dry run, list only)")
	flag.Parse()

	f := jobsusecase.BulkReconcileFilter{Statuses: splitList(*statuses), Kinds: splitList(*kinds)}
	var err error
	if f.OrganizationUUIDs, err = uuids(*orgs, "orgs"); err != nil {
		return err
	}
	if f.JobUUIDs, err = uuids(*jobs, "jobs"); err != nil {
		return err
	}
	switch {
	case *before != "":
		t, err := time.Parse(time.RFC3339, *before)
		if err != nil {
			return fmt.Errorf("--before: %w", err)
		}
		f.Before = t
	case len(f.JobUUIDs) > 0:
		f.Before = time.Now().Add(time.Minute)
	default:
		return errors.New("--before is required (or name the jobs with --jobs)")
	}
	if field, msg := f.Validate(); field != "" {
		return fmt.Errorf("--%s: %s", field, msg)
	}
	n := len([]rune(strings.TrimSpace(*note)))
	if *apply && (n < jobsusecase.ReconcileNoteMin || n > jobsusecase.ReconcileNoteMax) {
		return errors.New("--note is required with --apply (3-500 characters)")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()
	queries := database.NewQueries(pool)

	// The runner with the listeners that mirror job states (SMS log,
	// e-invoices); no provider is registered: nothing can be sent.
	fieldCipher, err := crypto.NewFieldCipher(cfg.Encryption.Key)
	if err != nil {
		return fmt.Errorf("field cipher: %w", err)
	}
	vault, err := integrations.NewVault(fieldCipher)
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	accounts := accountsusecase.New(queries, vault, integrations.NewRegistry())
	ops := integrations.NewOperationRegistry()
	runner := jobsusecase.NewRunner(queries, accounts, ops)
	if _, err := smsusecase.New(pool, queries, fieldCipher, accounts, runner, ops); err != nil {
		return fmt.Errorf("sms: %w", err)
	}
	if _, err := invoiceusecase.New(pool, queries, fieldCipher, orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, queries)),
		accounts, runner, ops); err != nil {
		return fmt.Errorf("invoices: %w", err)
	}

	candidates, err := runner.ReconcileCandidates(ctx, f)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ORGANIZATION\tJOB\tPROVIDER\tKIND\tSTATUS\tATTEMPTS\tCREATED\tUPDATED")
	for _, c := range candidates {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", c.OrganizationSlug, c.UUID, c.Provider, c.Kind, c.Status, c.Attempts,
			c.CreatedAt.Format(time.RFC3339), c.UpdatedAt.Format(time.RFC3339))
	}
	_ = tw.Flush()
	if len(f.JobUUIDs) > 0 && len(candidates) < len(f.JobUUIDs) {
		fmt.Printf("note: %d of the named jobs are not reconcilable (unknown, final, inbound, refunds, KABİS or created after --before)\n",
			len(f.JobUUIDs)-len(candidates))
	}
	if !*apply {
		fmt.Printf("dry run: %d job(s) would be marked reconciled; rerun with --apply --note \"...\"\n", len(candidates))
		return nil
	}

	rec := activity.NewRecorder(queries, nil)
	done, busy, skipped := 0, 0, 0
	for _, c := range candidates {
		octx := orgctx.WithOrganizationID(ctx, c.OrganizationID)
		res, err := runner.Reconcile(octx, c.OrganizationID, c.UUID, jobsusecase.ReconcileInput{Note: *note})
		switch {
		case errors.Is(err, jobsusecase.ErrBusy):
			busy++
			fmt.Printf("busy (held by a worker, try again): %s\n", c.UUID)
			continue
		case errors.Is(err, jobsusecase.ErrNotReconcilable), errors.Is(err, jobsusecase.ErrNotFound):
			skipped++
			fmt.Printf("skipped (state changed): %s\n", c.UUID)
			continue
		case err != nil:
			return fmt.Errorf("job %s: %w", c.UUID, err)
		}
		id := res.UUID
		rec.Record(octx, nil, "integrations.job_reconciled", jobshandler.Resource, &id, map[string]any{
			"kind": res.Kind, "previous_status": res.PreviousStatus, "note": strings.TrimSpace(*note), "source": "cli", "bulk": true,
		}, nil)
		done++
	}
	fmt.Printf("reconciled: %d, busy: %d, skipped: %d\n", done, busy, skipped)
	if busy > 0 {
		return errors.New("some jobs were held by a worker; stop the worker and run again")
	}
	return nil
}
