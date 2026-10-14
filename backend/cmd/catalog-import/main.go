// Command catalog-import imports a TSB kasko value list (.xlsx) into the
// platform vehicle catalog straight from a file path, without object storage
// or the queue. It prints the dry-run diff and, with -apply, applies it.
//
//	go run ./cmd/catalog-import -file 202610R1.xlsx            # dry run
//	go run ./cmd/catalog-import -file 202610R1.xlsx -apply     # apply
//	go run ./cmd/catalog-import -rebuild-auto                  # re-derive auto models
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/logging"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
)

func main() {
	file := flag.String("file", "", "path to the TSB kasko list (.xlsx)")
	period := flag.String("period", "", "override the period (YYYY-MM); default: read from cell A1")
	apply := flag.Bool("apply", false, "apply the import (default: dry run only)")
	showDiff := flag.Bool("diff", false, "print the diff samples as JSON")
	rebuild := flag.Bool("rebuild-auto", false, "re-derive auto models of the stored catalog with the current rules (manual models and prefix rules are kept)")
	flag.Parse()
	if *file == "" && !*rebuild {
		fmt.Fprintln(os.Stderr, "usage: catalog-import -file <list.xlsx> [-period YYYY-MM] [-apply] [-diff] | -rebuild-auto")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config load failed:", err)
		os.Exit(1)
	}
	log := logging.New(cfg.Log.Level, cfg.Log.Format)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	defer pool.Close()
	svc := usecase.New(repository.NewPostgres(pool, database.NewQueries(pool)), nil, nil, nil, log)

	if *rebuild {
		res, err := svc.RebuildAutoModels(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rebuild:", err)
			os.Exit(1)
		}
		printJSON("rebuild_auto", res)
		if *file == "" {
			return
		}
	}

	started := time.Now()
	f, err := os.Open(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	sheet, err := tsbcatalog.Parse(f)
	_ = f.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(1)
	}
	parsed := time.Since(started)

	plan, _, counts, err := svc.ImportSheet(ctx, sheet, filepath.Base(*file), *period, *apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, "import:", err)
		os.Exit(1)
	}
	mode := "dry-run"
	if *apply {
		mode = "applied"
	}
	fmt.Printf("period: %s (%q)  mode: %s  rows: %d  parse: %s  total: %s\n",
		plan.Period, sheet.PeriodLabel, mode, len(sheet.Rows), parsed.Round(time.Millisecond), time.Since(started).Round(time.Millisecond))
	printJSON("dry_run_counts", plan.Counts)
	if *apply {
		printJSON("applied_counts", counts)
	}
	if *showDiff {
		printJSON("diff", plan.Diff)
	}
}

func printJSON(label string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(label, err)
		return
	}
	fmt.Printf("%s: %s\n", label, b)
}
