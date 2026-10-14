package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxUploadBytes caps a TSB list upload (the monthly file is ~2 MB).
const MaxUploadBytes = 30 << 20

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func mapImport(i db.CatalogImport, email pgtype.Text, withDiff bool) model.Import {
	out := model.Import{
		UUID: i.Uuid, Period: optString(i.Period), Filename: i.Filename, Status: i.Status,
		Counts: map[string]any{}, Error: optString(i.Error), CreatedByEmail: optString(email),
		CreatedAt: i.CreatedAt.Time, UpdatedAt: i.UpdatedAt.Time,
	}
	if out.Period != nil {
		p := strings.TrimSpace(*out.Period)
		out.Period = &p
	}
	_ = json.Unmarshal(i.Counts, &out.Counts)
	if withDiff {
		out.Diff = map[string]any{}
		_ = json.Unmarshal(i.Diff, &out.Diff)
	}
	if i.AppliedAt.Valid {
		t := i.AppliedAt.Time
		out.AppliedAt = &t
	}
	return out
}

// ListImports lists catalog imports, newest first.
func (s *Service) ListImports(ctx context.Context, f model.ImportFilter) ([]model.Import, int64, error) {
	rows, err := s.q.ListCatalogImports(ctx, db.ListCatalogImportsParams{
		Status: text(f.Status), Q: text(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCatalogImports(ctx, db.CountCatalogImportsParams{Status: text(f.Status), Q: text(f.Q)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Import, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapImport(db.CatalogImport{
			ID: r.ID, Uuid: r.Uuid, Period: r.Period, Filename: r.Filename, FileKey: r.FileKey,
			Status: r.Status, Counts: r.Counts, Diff: r.Diff, Error: r.Error, CreatedBy: r.CreatedBy,
			AppliedAt: r.AppliedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, r.CreatedByEmail, false))
	}
	return out, total, nil
}

// GetImport returns one import with its diff samples.
func (s *Service) GetImport(ctx context.Context, id uuid.UUID) (model.Import, error) {
	row, err := s.q.GetCatalogImportByUUID(ctx, id)
	if err != nil {
		return model.Import{}, mapErr(repository.MapErr(err))
	}
	return mapImport(row, pgtype.Text{}, true), nil
}

// Upload stores a TSB list privately and queues its dry run. period
// overrides the period read from cell A1 ("" = from the file).
func (s *Service) Upload(ctx context.Context, actorID int64, filename string, r io.Reader, period string) (model.Import, error) {
	period = strings.TrimSpace(period)
	if period != "" && !tsbcatalog.ValidPeriod(period) {
		return model.Import{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalidRequest)
	}
	if s.storage == nil {
		return model.Import{}, fmt.Errorf("%w: object storage is not configured", ErrInvalidRequest)
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxUploadBytes+1))
	if err != nil {
		return model.Import{}, err
	}
	if len(data) == 0 || len(data) > MaxUploadBytes {
		return model.Import{}, fmt.Errorf("%w: file is empty or too large", ErrInvalidRequest)
	}
	if !bytes.HasPrefix(data, []byte("PK")) {
		return model.Import{}, fmt.Errorf("%w: file must be an .xlsx workbook", ErrInvalidRequest)
	}
	var created pgtype.Int8
	if actorID > 0 {
		created = pgtype.Int8{Int64: actorID, Valid: true}
	}
	row, err := s.q.CreateCatalogImport(ctx, db.CreateCatalogImportParams{
		Filename: truncate(filename, 255), CreatedBy: created, Period: text(period),
	})
	if err != nil {
		return model.Import{}, err
	}
	key := storage.CatalogImportObjectKey(row.Uuid.String(), "xlsx")
	if err := s.storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: xlsxContentType, Filename: filename,
	}, key); err != nil {
		_, _ = s.q.UpdateCatalogImportState(ctx, db.UpdateCatalogImportStateParams{
			ID: row.ID, Status: model.ImportFailed, Error: text("storage upload failed"),
		})
		return model.Import{}, fmt.Errorf("catalog: upload source: %w", err)
	}
	if _, err := s.q.SetCatalogImportFileKey(ctx, db.SetCatalogImportFileKeyParams{
		ID: row.ID, FileKey: pgtype.Text{String: key, Valid: true},
	}); err != nil {
		return model.Import{}, err
	}
	if s.activity != nil {
		s.activity.Record(ctx, actorRef(actorID), "catalog.import_uploaded", "platform.catalog", &row.Uuid, map[string]any{"filename": filename}, nil)
	}
	if err := s.enqueue(ctx, row.ID, queue.CatalogImportDiff); err != nil {
		s.log.Error("catalog_import_enqueue_failed", "import", row.Uuid, "error", err)
		_, _ = s.q.UpdateCatalogImportState(ctx, db.UpdateCatalogImportStateParams{
			ID: row.ID, Status: model.ImportFailed, Error: text("could not queue the import"),
		})
	}
	s.publish(ctx, LiveImports, resourceevents.ActionCreated, row.Uuid)
	return s.GetImport(ctx, row.Uuid)
}

// RequestApply queues the apply of a dry-run import. Re-applying an applied
// import is allowed and changes nothing when the catalog already matches.
func (s *Service) RequestApply(ctx context.Context, actorID int64, id uuid.UUID) (model.Import, error) {
	row, err := s.q.GetCatalogImportByUUID(ctx, id)
	if err != nil {
		return model.Import{}, mapErr(repository.MapErr(err))
	}
	if !row.Period.Valid || !row.FileKey.Valid {
		return model.Import{}, ErrImportState
	}
	claimed, err := s.q.ClaimCatalogImport(ctx, db.ClaimCatalogImportParams{
		ID: row.ID, ToStatus: model.ImportApplying,
		FromStatuses: []string{model.ImportReady, model.ImportApplied, model.ImportFailed},
	})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return model.Import{}, ErrImportState
		}
		return model.Import{}, err
	}
	if s.activity != nil {
		s.activity.Record(ctx, actorRef(actorID), "catalog.import_apply_requested", "platform.catalog", &claimed.Uuid, map[string]any{"period": strings.TrimSpace(claimed.Period.String)}, nil)
	}
	if err := s.enqueue(ctx, claimed.ID, queue.CatalogImportApply); err != nil {
		s.log.Error("catalog_import_enqueue_failed", "import", claimed.Uuid, "error", err)
		_, _ = s.q.UpdateCatalogImportState(ctx, db.UpdateCatalogImportStateParams{
			ID: claimed.ID, Status: model.ImportFailed, Error: text("could not queue the apply"),
		})
	}
	s.publish(ctx, LiveImports, resourceevents.ActionUpdated, id)
	return s.GetImport(ctx, id)
}

// ProcessImport runs one import step; called by the worker (or inline).
func (s *Service) ProcessImport(ctx context.Context, importID int64, action string) error {
	switch action {
	case queue.CatalogImportDiff:
		row, err := s.q.ClaimCatalogImport(ctx, db.ClaimCatalogImportParams{
			ID: importID, ToStatus: model.ImportProcessing, FromStatuses: []string{model.ImportQueued},
		})
		if err != nil {
			// Already processed (retry after success) or gone: nothing to do.
			s.log.Warn("catalog_import_diff_skipped", "id", importID, "error", err)
			return nil
		}
		plan, _, err := s.runFromStorage(ctx, row, false)
		if err != nil {
			return s.fail(ctx, row.ID, err)
		}
		_, err = s.q.UpdateCatalogImportState(ctx, db.UpdateCatalogImportStateParams{
			ID: row.ID, Status: model.ImportReady, Period: text(plan.Period),
			Counts: mustJSON(plan.Counts), Diff: mustJSON(plan.Diff),
		})
		if err == nil {
			s.publish(ctx, LiveImports, resourceevents.ActionUpdated, row.Uuid)
		}
		return err
	case queue.CatalogImportApply:
		row, err := s.q.GetCatalogImportByID(ctx, importID)
		if err != nil {
			return err
		}
		if row.Status != model.ImportApplying {
			s.log.Warn("catalog_import_apply_skipped", "id", importID, "status", row.Status)
			return nil
		}
		plan, res, err := s.runFromStorage(ctx, row, true)
		if err != nil {
			return s.fail(ctx, row.ID, err)
		}
		counts := appliedCounts(plan, res)
		diff := map[string]any{}
		_ = json.Unmarshal(row.Diff, &diff)
		diff["dry_run_counts"] = plan.Counts
		if _, err := s.q.UpdateCatalogImportState(ctx, db.UpdateCatalogImportStateParams{
			ID: row.ID, Status: model.ImportApplied, Counts: mustJSON(counts), Diff: mustJSON(diff),
		}); err != nil {
			return err
		}
		var actor int64
		if row.CreatedBy.Valid {
			actor = row.CreatedBy.Int64
		}
		if s.activity != nil {
			s.activity.Record(ctx, actorRef(actor), "catalog.import_applied", "platform.catalog", &row.Uuid, map[string]any{
				"period": plan.Period, "types_new": counts.TypesNew, "valuations_upserted": counts.ValuationsUpserted,
			}, nil)
		}
		s.publish(ctx, LiveImports, resourceevents.ActionUpdated, row.Uuid)
		s.publishCatalogApplied(ctx)
		return nil
	}
	return fmt.Errorf("catalog: unknown import action %q", action)
}

func (s *Service) fail(ctx context.Context, id int64, cause error) error {
	msg := cause.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	row, err := s.q.UpdateCatalogImportState(ctx, db.UpdateCatalogImportStateParams{
		ID: id, Status: model.ImportFailed, Error: text(msg),
	})
	if err != nil {
		return err
	}
	s.publish(ctx, LiveImports, resourceevents.ActionUpdated, row.Uuid)
	s.log.Warn("catalog_import_failed", "id", id, "error", cause)
	// The failure is recorded on the row; do not make Asynq retry it.
	return nil
}

func (s *Service) runFromStorage(ctx context.Context, row db.CatalogImport, apply bool) (model.Plan, model.ApplyResult, error) {
	if s.storage == nil || !row.FileKey.Valid {
		return model.Plan{}, model.ApplyResult{}, errors.New("source file is not available")
	}
	rc, _, err := s.storage.Download(ctx, row.FileKey.String)
	if err != nil {
		return model.Plan{}, model.ApplyResult{}, fmt.Errorf("download source: %w", err)
	}
	defer func() { _ = rc.Close() }()
	sheet, err := tsbcatalog.Parse(rc)
	if err != nil {
		return model.Plan{}, model.ApplyResult{}, err
	}
	period := strings.TrimSpace(row.Period.String)
	return s.Run(ctx, sheet, period, apply)
}

// Run plans (and optionally applies) a parsed sheet. period overrides the
// sheet's own period when set.
func (s *Service) Run(ctx context.Context, sheet *tsbcatalog.Sheet, period string, apply bool) (model.Plan, model.ApplyResult, error) {
	if period == "" {
		period = sheet.Period
	}
	if !tsbcatalog.ValidPeriod(period) {
		return model.Plan{}, model.ApplyResult{}, fmt.Errorf("could not read the period from cell A1 (%q); pass it explicitly", sheet.PeriodLabel)
	}
	st, err := s.repo.LoadState(ctx, period)
	if err != nil {
		return model.Plan{}, model.ApplyResult{}, err
	}
	plan := BuildPlan(sheet, period, st)
	if !apply {
		return plan, model.ApplyResult{}, nil
	}
	res, err := s.repo.Apply(ctx, plan)
	if err != nil {
		return plan, res, err
	}
	return plan, res, nil
}

// ImportSheet runs a parsed sheet end to end outside the queue (CLI) and
// records it as an import row so it shows up in the import history.
func (s *Service) ImportSheet(ctx context.Context, sheet *tsbcatalog.Sheet, filename, period string, apply bool) (model.Plan, model.ApplyResult, model.Counts, error) {
	plan, res, err := s.Run(ctx, sheet, period, apply)
	if err != nil {
		return plan, res, model.Counts{}, err
	}
	counts := plan.Counts
	status := model.ImportReady
	diff := map[string]any{}
	_ = json.Unmarshal(mustJSON(plan.Diff), &diff)
	if apply {
		counts = appliedCounts(plan, res)
		status = model.ImportApplied
		diff["dry_run_counts"] = plan.Counts
	}
	row, err := s.q.CreateCatalogImport(ctx, db.CreateCatalogImportParams{
		Filename: truncate(filename, 255), Period: text(plan.Period),
	})
	if err != nil {
		return plan, res, counts, err
	}
	if _, err := s.q.UpdateCatalogImportState(ctx, db.UpdateCatalogImportStateParams{
		ID: row.ID, Status: status, Counts: mustJSON(counts), Diff: mustJSON(diff),
	}); err != nil {
		return plan, res, counts, err
	}
	return plan, res, counts, nil
}

// appliedCounts reports what the apply changed in the database. Type renames
// and reactivations come from the plan (the upsert reports them as updates).
func appliedCounts(plan model.Plan, res model.ApplyResult) model.Counts {
	c := model.Counts{
		Rows:                 plan.Counts.Rows,
		BrandsNew:            res.BrandsInserted,
		BrandsRenamed:        res.BrandsUpdated,
		ModelsNew:            res.ModelsInserted,
		TypesNew:             res.TypesInserted,
		TypesUpdated:         res.TypesUpdated,
		TypesDeactivated:     res.TypesDeactivated,
		ValuationsUpserted:   res.ValuationsUpserted,
		UnresolvedByCategory: res.UnresolvedByCategory,
	}
	if res.TypesUpdated > 0 {
		c.TypesRenamed = plan.Counts.TypesRenamed
		c.TypesReactivated = plan.Counts.TypesReactivated
	}
	if res.ValuationsUpserted > 0 {
		c.ValuationsNew = plan.Counts.ValuationsNew
		c.ValuationsChanged = plan.Counts.ValuationsChanged
	}
	for _, n := range res.UnresolvedByCategory {
		c.UnresolvedModels += n
	}
	return c
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
