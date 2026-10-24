// Command messaging-reconcile lists the outbound messages (notification
// engine dispatches, SMS, notifications / e-mail) that were still unsent
// in restored data and marks the ones sent after the backup, so the worker
// never sends them again (TEC-962, docs/operations/backup-restore.md
// RT-A32 (c), internal/platform/msgreconcile). Run it on the restored
// database before the worker starts (restore.sh --no-start).
//
// Usage:
//
//	messaging-reconcile list --before 20261020T031500Z [--due-until <time>] [--org <slug>] [--kind dispatch,sms,notification]
//	messaging-reconcile mark --before 20261020T031500Z [same filters] [--uuids <uuid,...>] [--note <text>] [--yes]
//
// Times are RFC 3339 or the backup set timestamp (UTC). mark without --yes
// is a dry run. Repo root: make messaging-reconcile ARGS="list --before ...";
// production: make prod-messaging-reconcile ARGS="...".
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/msgreconcile"
	"github.com/google/uuid"
)

const usage = `usage:
  messaging-reconcile list --before <backup time> [--due-until <time>] [--org <slug>] [--kind dispatch,sms,notification] [--limit N]
  messaging-reconcile mark --before <backup time> [same filters] [--uuids <uuid,...>] [--note <text>] [--yes]

Times: RFC 3339 (2026-10-20T03:15:00Z) or the backup set timestamp (20261020T031500Z).
mark without --yes only shows what it would mark.`

type options struct {
	cmd    string
	filter msgreconcile.Filter
	org    string
	note   string
	yes    bool
}

// usageText is the usage block as an error message (staticcheck ST1005: no
// trailing punctuation or newline).
func usageText() string {
	return strings.TrimRight(usage, ".\n ")
}

func parse(args []string) (options, error) {
	var o options
	if len(args) == 0 || (args[0] != "list" && args[0] != "mark") {
		return o, errors.New(usageText())
	}
	o.cmd = args[0]
	fs := flag.NewFlagSet("messaging-reconcile "+o.cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	before := fs.String("before", "", "backup time: messages created before it")
	dueUntil := fs.String("due-until", "", "only messages due by this time (default: now)")
	fs.StringVar(&o.org, "org", "", "organization slug (default: all, platform notifications included)")
	kinds := fs.String("kind", "", "dispatch,sms,notification (default: all)")
	uuids := fs.String("uuids", "", "mark only these messages (comma separated)")
	limit := fs.Int("limit", 1000, "list at most N messages per kind")
	fs.StringVar(&o.note, "note", "", "reason stored on marked messages")
	fs.BoolVar(&o.yes, "yes", false, "mark (without it, mark is a dry run)")
	if err := fs.Parse(args[1:]); err != nil {
		return o, fmt.Errorf("%v\n%s", err, usage)
	}
	var err error
	if o.filter.Before, err = msgreconcile.ParseTime(*before); err != nil {
		return o, err
	}
	if o.filter.Before.IsZero() {
		return o, fmt.Errorf("--before (the backup time) is required\n%s", usageText())
	}
	if o.filter.DueUntil, err = msgreconcile.ParseTime(*dueUntil); err != nil {
		return o, err
	}
	if o.filter.Kinds, err = msgreconcile.ParseKinds(*kinds); err != nil {
		return o, err
	}
	for _, part := range strings.Split(*uuids, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := uuid.Parse(part)
		if err != nil {
			return o, fmt.Errorf("invalid uuid %q", part)
		}
		o.filter.UUIDs = append(o.filter.UUIDs, id)
	}
	if o.cmd == "list" && len(o.filter.UUIDs) > 0 {
		return o, errors.New("--uuids applies to mark only")
	}
	if *limit <= 0 || *limit > 100000 {
		return o, errors.New("--limit must be between 1 and 100000")
	}
	o.filter.Limit = int32(*limit)
	return o, nil
}

func main() {
	o, err := parse(os.Args[1:])
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()
	r := msgreconcile.New(pool, database.NewQueries(pool))
	if o.org != "" {
		id, err := r.OrganizationBySlug(ctx, o.org)
		if err != nil {
			fatal("%v", err)
		}
		o.filter.OrganizationID = &id
	}
	list, err := r.List(ctx, o.filter)
	if err != nil {
		fatal("%v", err)
	}
	if o.cmd == "mark" && len(o.filter.UUIDs) > 0 {
		list = only(list, o.filter.UUIDs)
	}
	printList(os.Stdout, list)
	if o.cmd == "list" {
		return
	}
	if len(o.filter.UUIDs) == 0 && truncated(list, o.filter.Limit) {
		fatal("a kind has --limit (%d) or more matches: raise --limit to see every message mark would change", o.filter.Limit)
	}
	if !o.yes {
		fmt.Printf("\ndry run: %d message(s) would be marked; add --yes to mark them\n", len(list))
		return
	}
	res, err := r.Mark(ctx, o.filter, o.note)
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("\nmarked %d message(s): %d dispatch(es) reconciled, %d SMS reconciled, %d notification(s) cancelled\n",
		res.Total(), res.Dispatches, res.SMS, res.Notifications)
}

// truncated reports whether a kind reached the list limit (mark would
// change rows the list did not show).
func truncated(list []msgreconcile.Candidate, limit int32) bool {
	counts := map[msgreconcile.Kind]int32{}
	for _, c := range list {
		counts[c.Kind]++
		if counts[c.Kind] >= limit {
			return true
		}
	}
	return false
}

func only(list []msgreconcile.Candidate, ids []uuid.UUID) []msgreconcile.Candidate {
	want := map[uuid.UUID]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := list[:0]
	for _, c := range list {
		if want[c.UUID] {
			out = append(out, c)
		}
	}
	return out
}

func printList(w io.Writer, list []msgreconcile.Candidate) {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KIND\tUUID\tORGANIZATION\tWHAT\tCHANNEL\tRECIPIENT\tSTATUS\tCREATED (UTC)\tDUE (UTC)")
	counts := map[msgreconcile.Kind]int{}
	for _, c := range list {
		counts[c.Kind]++
		org := c.Organization
		if org == "" && c.OrganizationID != nil {
			org = fmt.Sprintf("#%d", *c.OrganizationID)
		}
		due := ""
		if c.DueAt != nil {
			due = c.DueAt.UTC().Format(time.DateTime)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", c.Kind, c.UUID, org, c.What, c.Channel, c.Recipient,
			c.Status, c.CreatedAt.UTC().Format(time.DateTime), due)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "\n%d dispatch(es), %d SMS, %d notification(s)\n", counts[msgreconcile.KindDispatch],
		counts[msgreconcile.KindSMS], counts[msgreconcile.KindNotification])
	if counts[msgreconcile.KindSMS] > 0 {
		_, _ = fmt.Fprintln(w, "SMS without a provider id cannot be looked up at Netgsm (reports are by jobid): compare with the Netgsm panel's sent messages.")
	}
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "messaging-reconcile: "+format+"\n", args...)
	os.Exit(1)
}
