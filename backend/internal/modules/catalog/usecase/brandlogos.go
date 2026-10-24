package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/motomarks"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Brand logo sync (TEC-995): the motomarks-mirror service is asked on every
// run (its URL and token live in brand_logo_sync_settings, editable by the
// platform admin); the brand list lands in brand_logo_sources and the
// badge / wordmark / full files are copied into this app's object storage.
// Display brands are linked to a mirror brand automatically when the match
// score reaches the threshold, otherwise they wait in the unmatched queue
// for the admin. An uploaded logo always wins over the synced one.

// ErrSyncRunning means a run is queued or running already.
var ErrSyncRunning = errors.New("a brand logo sync is already running")

// ErrSyncNotConfigured means base URL or token is missing.
var ErrSyncNotConfigured = errors.New("brand logo sync is not configured")

// Match sources.
const (
	MatchNone   = "none"
	MatchAuto   = "auto"
	MatchManual = "manual"
	MatchSkip   = "skip"
)

// SetSecretBox attaches the encryption used for the mirror API token.
func (s *Service) SetSecretBox(box *crypto.SecretBox) { s.box = box }

// MirrorClientFactory builds the mirror client (tests swap the transport).
type MirrorClientFactory func(baseURL, token string) (*motomarks.Client, error)

// SetMirrorClientFactory overrides how the mirror client is built.
func (s *Service) SetMirrorClientFactory(f MirrorClientFactory) { s.mirror = f }

func (s *Service) newMirror(baseURL, token string) (*motomarks.Client, error) {
	if s.mirror != nil {
		return s.mirror(baseURL, token)
	}
	return motomarks.New(baseURL, token)
}

// ---------- settings ----------

func mapLogoSettings(row db.BrandLogoSyncSetting) model.BrandLogoSettings {
	out := model.BrandLogoSettings{
		BaseURL:           row.BaseUrl,
		TokenConfigured:   row.ApiTokenEnc.Valid && row.ApiTokenEnc.String != "",
		Enabled:           row.Enabled,
		AutoMatchMinScore: int(row.AutoMatchMinScore),
		LastStatus:        optString(row.LastStatus),
		LastError:         optString(row.LastError),
	}
	if row.LastRunAt.Valid {
		t := row.LastRunAt.Time
		out.LastRunAt = &t
	}
	return out
}

// GetBrandLogoSettings returns the masked settings plus match counters.
func (s *Service) GetBrandLogoSettings(ctx context.Context) (model.BrandLogoSettings, error) {
	row, err := s.q.GetBrandLogoSyncSettings(ctx)
	if err != nil {
		return model.BrandLogoSettings{}, err
	}
	out := mapLogoSettings(row)
	counts, err := s.q.CountCatalogDisplayBrandsByMatchSource(ctx)
	if err != nil {
		return model.BrandLogoSettings{}, err
	}
	out.MatchCounts = map[string]int64{MatchNone: 0, MatchAuto: 0, MatchManual: 0, MatchSkip: 0}
	for _, c := range counts {
		out.MatchCounts[c.MotomarksMatchSource] = c.Count
	}
	if out.SourcesCount, err = s.q.CountBrandLogoSources(ctx); err != nil {
		return model.BrandLogoSettings{}, err
	}
	if run, err := s.q.GetActiveBrandLogoSyncRun(ctx); err == nil {
		r := mapRun(run)
		out.ActiveRun = &r
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return model.BrandLogoSettings{}, err
	}
	return out, nil
}

// PatchBrandLogoSettings updates endpoint, token, switch and threshold.
func (s *Service) PatchBrandLogoSettings(ctx context.Context, in model.PatchBrandLogoSettingsInput) (model.BrandLogoSettings, error) {
	params := db.UpdateBrandLogoSyncSettingsParams{}
	if in.BaseURL != nil {
		u := strings.TrimRight(strings.TrimSpace(*in.BaseURL), "/")
		if u != "" {
			if _, err := motomarks.New(u, ""); err != nil {
				return model.BrandLogoSettings{}, fmt.Errorf("%w: base_url must be an http(s) origin", ErrInvalidRequest)
			}
		}
		params.BaseUrl = text(u)
	}
	if in.APIToken != nil {
		tok := strings.TrimSpace(*in.APIToken)
		if tok == "" {
			params.ClearToken = true
		} else {
			if s.box == nil {
				return model.BrandLogoSettings{}, fmt.Errorf("%w: encryption is not configured", ErrInvalidRequest)
			}
			enc, err := s.box.Encrypt(tok)
			if err != nil {
				return model.BrandLogoSettings{}, err
			}
			params.ApiTokenEnc = text(enc)
		}
	}
	if in.Enabled != nil {
		params.Enabled = pgtype.Bool{Bool: *in.Enabled, Valid: true}
	}
	if in.AutoMatchMinScore != nil {
		if *in.AutoMatchMinScore < 0 || *in.AutoMatchMinScore > 100 {
			return model.BrandLogoSettings{}, fmt.Errorf("%w: auto_match_min_score must be between 0 and 100", ErrInvalidRequest)
		}
		params.AutoMatchMinScore = pgtype.Int4{Int32: int32(*in.AutoMatchMinScore), Valid: true}
	}
	if _, err := s.q.UpdateBrandLogoSyncSettings(ctx, params); err != nil {
		return model.BrandLogoSettings{}, err
	}
	return s.GetBrandLogoSettings(ctx)
}

// TestBrandLogoConnection calls the live endpoint with the stored token.
func (s *Service) TestBrandLogoConnection(ctx context.Context) error {
	cli, _, err := s.mirrorFromSettings(ctx)
	if err != nil {
		return err
	}
	if err := cli.Health(ctx); err != nil {
		if errors.Is(err, motomarks.ErrUnauthorized) {
			return fmt.Errorf("%w: the mirror rejected the token", ErrInvalidRequest)
		}
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	return nil
}

func (s *Service) mirrorFromSettings(ctx context.Context) (*motomarks.Client, db.BrandLogoSyncSetting, error) {
	row, err := s.q.GetBrandLogoSyncSettings(ctx)
	if err != nil {
		return nil, row, err
	}
	if strings.TrimSpace(row.BaseUrl) == "" || !row.ApiTokenEnc.Valid || row.ApiTokenEnc.String == "" {
		return nil, row, ErrSyncNotConfigured
	}
	if s.box == nil {
		return nil, row, fmt.Errorf("%w: encryption is not configured", ErrSyncNotConfigured)
	}
	token, err := s.box.Decrypt(row.ApiTokenEnc.String)
	if err != nil {
		return nil, row, fmt.Errorf("brand logos: decrypt token: %w", err)
	}
	cli, err := s.newMirror(row.BaseUrl, token)
	if err != nil {
		return nil, row, fmt.Errorf("%w: %v", ErrSyncNotConfigured, err)
	}
	return cli, row, nil
}

// ---------- runs ----------

func mapRun(r db.BrandLogoSyncRun) model.BrandLogoSyncRun {
	out := model.BrandLogoSyncRun{
		UUID: r.Uuid, Status: r.Status, NotModified: r.NotModified,
		Requests: int(r.Requests), BrandsSeen: int(r.BrandsSeen), Downloaded: int(r.Downloaded),
		Matched: int(r.Matched), Unmatched: int(r.Unmatched), Error: optString(r.Error),
		CreatedAt: r.CreatedAt.Time,
	}
	if r.StartedAt.Valid {
		t := r.StartedAt.Time
		out.StartedAt = &t
	}
	if r.FinishedAt.Valid {
		t := r.FinishedAt.Time
		out.FinishedAt = &t
	}
	return out
}

// RequestBrandLogoSync creates a run and queues it (inline without a queue).
func (s *Service) RequestBrandLogoSync(ctx context.Context, actorID int64) (model.BrandLogoSyncRun, error) {
	row, err := s.q.GetBrandLogoSyncSettings(ctx)
	if err != nil {
		return model.BrandLogoSyncRun{}, err
	}
	if !row.Enabled {
		return model.BrandLogoSyncRun{}, fmt.Errorf("%w: sync is disabled", ErrInvalidRequest)
	}
	if strings.TrimSpace(row.BaseUrl) == "" || !row.ApiTokenEnc.Valid {
		return model.BrandLogoSyncRun{}, fmt.Errorf("%w: set the mirror base URL and token first", ErrInvalidRequest)
	}
	if _, err := s.q.GetActiveBrandLogoSyncRun(ctx); err == nil {
		return model.BrandLogoSyncRun{}, ErrSyncRunning
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return model.BrandLogoSyncRun{}, err
	}
	run, err := s.q.CreateBrandLogoSyncRun(ctx, pgtype.Int8{Int64: actorID, Valid: actorID > 0})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrConflict) {
			return model.BrandLogoSyncRun{}, ErrSyncRunning
		}
		return model.BrandLogoSyncRun{}, err
	}
	if s.queue == nil {
		if err := s.ProcessBrandLogoSync(ctx, run.ID); err != nil {
			return model.BrandLogoSyncRun{}, err
		}
		fresh, err := s.q.GetBrandLogoSyncRunByID(ctx, run.ID)
		if err != nil {
			return model.BrandLogoSyncRun{}, err
		}
		return mapRun(fresh), nil
	}
	task, err := queue.NewBrandLogoSyncTask(run.ID)
	if err != nil {
		return model.BrandLogoSyncRun{}, err
	}
	if _, err := s.queue.Enqueue(task, asynq.Queue(queue.QueueImports)); err != nil {
		_, _ = s.q.FinishBrandLogoSyncRun(ctx, db.FinishBrandLogoSyncRunParams{
			ID: run.ID, Status: "failed", Error: text("enqueue: " + err.Error()),
		})
		return model.BrandLogoSyncRun{}, err
	}
	if s.activity != nil {
		s.activity.Record(ctx, actorRef(actorID), "catalog.brand_logos_sync_requested", "platform.catalog", &run.Uuid, nil, nil)
	}
	return mapRun(run), nil
}

// ListBrandLogoSyncRuns pages the run history, newest first.
func (s *Service) ListBrandLogoSyncRuns(ctx context.Context, limit, offset int32) ([]model.BrandLogoSyncRun, int64, error) {
	rows, err := s.q.ListBrandLogoSyncRuns(ctx, db.ListBrandLogoSyncRunsParams{LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountBrandLogoSyncRuns(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.BrandLogoSyncRun, 0, len(rows))
	for _, r := range rows {
		run := mapRun(db.BrandLogoSyncRun{
			ID: r.ID, Uuid: r.Uuid, Status: r.Status, TriggeredBy: r.TriggeredBy, StartedAt: r.StartedAt,
			FinishedAt: r.FinishedAt, NotModified: r.NotModified, Requests: r.Requests, BrandsSeen: r.BrandsSeen,
			Downloaded: r.Downloaded, Matched: r.Matched, Unmatched: r.Unmatched, Error: r.Error, CreatedAt: r.CreatedAt,
		})
		if r.TriggeredByName.Valid {
			run.TriggeredBy = &model.NamedRef{UUID: uuid.UUID(r.TriggeredByUuid.Bytes), Name: r.TriggeredByName.String}
		}
		out = append(out, run)
	}
	return out, total, nil
}

type syncStats struct {
	brandsSeen, downloaded, matched, unmatched int
	notModified                                bool
}

// ProcessBrandLogoSync is the queue handler of one run.
func (s *Service) ProcessBrandLogoSync(ctx context.Context, runID int64) error {
	run, err := s.q.StartBrandLogoSyncRun(ctx, runID)
	if err != nil {
		s.log.Warn("brand_logo_sync_skipped", "run_id", runID, "error", err)
		return nil
	}
	cli, settings, err := s.mirrorFromSettings(ctx)
	if err != nil {
		return s.finishRun(ctx, run.ID, nil, syncStats{}, err)
	}
	stats, etag, err := s.runSync(ctx, cli, settings)
	return s.finishRun(ctx, run.ID, cli, stats, err, etag)
}

func (s *Service) finishRun(ctx context.Context, id int64, cli *motomarks.Client, st syncStats, cause error, etag ...string) error {
	status := "done"
	var errText pgtype.Text
	if cause != nil {
		status = "failed"
		msg := cause.Error()
		if len(msg) > 1000 {
			msg = msg[:1000]
		}
		errText = text(msg)
		s.log.Warn("brand_logo_sync_failed", "run_id", id, "error", cause)
	}
	requests := 0
	if cli != nil {
		requests = cli.Requests
	}
	if _, err := s.q.FinishBrandLogoSyncRun(ctx, db.FinishBrandLogoSyncRunParams{
		ID: id, Status: status, NotModified: st.notModified, Requests: int32(requests),
		BrandsSeen: int32(st.brandsSeen), Downloaded: int32(st.downloaded), Matched: int32(st.matched),
		Unmatched: int32(st.unmatched), Error: errText,
	}); err != nil {
		return err
	}
	var etagText pgtype.Text
	if len(etag) > 0 && etag[0] != "" {
		etagText = text(etag[0])
	}
	if err := s.q.SetBrandLogoSyncState(ctx, db.SetBrandLogoSyncStateParams{LastEtag: etagText, LastStatus: text(status), LastError: errText}); err != nil {
		return err
	}
	s.publish(ctx, LiveBrands, resourceevents.ActionBulk)
	// The failure is recorded on the row; Asynq must not retry it.
	return nil
}

func (s *Service) runSync(ctx context.Context, cli *motomarks.Client, settings db.BrandLogoSyncSetting) (syncStats, string, error) {
	var st syncStats
	storedEtag := ""
	if settings.LastEtag.Valid {
		storedEtag = settings.LastEtag.String
	}
	res, err := cli.Brands(ctx, storedEtag)
	if err != nil {
		return st, "", err
	}
	st.notModified = res.NotModified
	if !res.NotModified {
		if s.storage == nil {
			return st, "", errors.New("object storage is not configured")
		}
		for _, b := range res.Brands {
			if strings.TrimSpace(b.ID) == "" {
				continue
			}
			st.brandsSeen++
			n, err := s.syncSource(ctx, cli, b)
			if err != nil {
				return st, "", fmt.Errorf("brand %s: %w", b.ID, err)
			}
			st.downloaded += n
		}
	}
	matched, unmatched, err := s.matchDisplayBrands(ctx, cli, int(settings.AutoMatchMinScore))
	if err != nil {
		return st, "", err
	}
	st.matched, st.unmatched = matched, unmatched
	if err := s.refreshSyncedLogos(ctx); err != nil {
		return st, "", err
	}
	etag := res.ETag
	if res.NotModified {
		etag = storedEtag
	}
	return st, etag, nil
}

// syncSource upserts the mirror brand and downloads its variants when the
// hash changed (or a variant file is still missing). Returns files copied.
func (s *Service) syncSource(ctx context.Context, cli *motomarks.Client, b motomarks.Brand) (int, error) {
	hash := b.Hash()
	existing, err := s.q.GetBrandLogoSourceByMotomarksID(ctx, b.ID)
	hadRow := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	colors := b.Colors
	if colors == nil {
		colors = []string{}
	}
	vehicleTypes := b.VehicleTypes
	if vehicleTypes == nil {
		vehicleTypes = []string{}
	}
	logos := b.Logos
	if logos == nil {
		logos = map[string]map[string]map[string]string{}
	}
	row, err := s.q.UpsertBrandLogoSource(ctx, db.UpsertBrandLogoSourceParams{
		MotomarksID: b.ID, Name: strings.TrimSpace(b.Name), Color: strings.TrimSpace(b.Color),
		Colors: mustJSON(colors), VehicleTypes: vehicleTypes, Logos: mustJSON(logos), SourceHash: hash,
	})
	if err != nil {
		return 0, err
	}
	unchanged := hadRow && existing.SourceHash == hash && existing.SyncedAt.Valid
	complete := true
	for _, v := range motomarks.Variants {
		if b.LogoURL(v) != "" && !variantKey(existing, v).Valid {
			complete = false
		}
	}
	if unchanged && complete {
		return 0, nil
	}
	keys := map[string]pgtype.Text{}
	downloaded := 0
	for _, v := range motomarks.Variants {
		src := b.LogoURL(v)
		if src == "" {
			continue
		}
		data, contentType, err := cli.Download(ctx, src)
		if err != nil {
			s.log.Warn("brand_logo_download_failed", "brand", b.ID, "variant", v, "error", err)
			// Keep the previous copy for this variant, if any.
			keys[v] = variantKey(existing, v)
			continue
		}
		head := data
		if len(head) > 512 {
			head = head[:512]
		}
		mime, err := storage.DetectLogoMIME(contentType, head)
		if err != nil {
			s.log.Warn("brand_logo_type_rejected", "brand", b.ID, "variant", v, "error", err)
			keys[v] = variantKey(existing, v)
			continue
		}
		ext, _ := storage.LogoExtForMIME(mime)
		key := storage.BrandLogoObjectKey(b.ID, v, ext)
		if err := s.storage.Upload(ctx, storage.File{
			Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: mime, Filename: v + "." + ext,
		}, key); err != nil {
			return downloaded, fmt.Errorf("upload %s: %w", v, err)
		}
		if old := variantKey(existing, v); old.Valid && old.String != key {
			s.dropLogo(ctx, &old.String)
		}
		keys[v] = text(key)
		downloaded++
	}
	// Variants the mirror no longer lists are dropped.
	for _, v := range motomarks.Variants {
		if b.LogoURL(v) == "" {
			if old := variantKey(existing, v); old.Valid {
				s.dropLogo(ctx, &old.String)
			}
		}
	}
	if err := s.q.SetBrandLogoSourceObjects(ctx, db.SetBrandLogoSourceObjectsParams{
		ID: row.ID, BadgeObjectKey: keys[motomarks.VariantBadge],
		WordmarkObjectKey: keys[motomarks.VariantWordmark], FullObjectKey: keys[motomarks.VariantFull],
	}); err != nil {
		return downloaded, err
	}
	return downloaded, nil
}

func variantKey(row db.BrandLogoSource, variant string) pgtype.Text {
	switch variant {
	case motomarks.VariantBadge:
		return row.BadgeObjectKey
	case motomarks.VariantWordmark:
		return row.WordmarkObjectKey
	case motomarks.VariantFull:
		return row.FullObjectKey
	}
	return pgtype.Text{}
}

func numeric(score float64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(int64(score*100 + 0.5)), Exp: -2, Valid: true}
}

// matchDisplayBrands links display brands that were never matched. Below
// the threshold the best candidate is kept as a suggestion for the queue.
func (s *Service) matchDisplayBrands(ctx context.Context, cli *motomarks.Client, minScore int) (int, int, error) {
	pending, err := s.q.ListCatalogDisplayBrandsToMatch(ctx)
	if err != nil || len(pending) == 0 {
		return 0, 0, err
	}
	names := make([]string, 0, len(pending))
	for _, p := range pending {
		names = append(names, p.Name)
	}
	results, err := cli.Match(ctx, names, 0)
	if err != nil {
		return 0, 0, err
	}
	matched, unmatched := 0, 0
	for i, p := range pending {
		r := results[i]
		if r.Brand == nil || strings.TrimSpace(r.Brand.ID) == "" {
			unmatched++
			continue
		}
		// The brand must be one we store (the list is synced before matching).
		if _, err := s.q.GetBrandLogoSourceByMotomarksID(ctx, r.Brand.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				unmatched++
				continue
			}
			return matched, unmatched, err
		}
		params := db.SetCatalogDisplayBrandMotomarksParams{ID: p.ID, MotomarksMatchSource: MatchNone}
		if r.Score >= float64(minScore) {
			params.MotomarksID = text(r.Brand.ID)
			params.MotomarksMatchScore = numeric(r.Score)
			params.MotomarksMatchSource = MatchAuto
			matched++
		} else {
			params.MotomarksSuggestedID = text(r.Brand.ID)
			params.MotomarksSuggestedScore = numeric(r.Score)
			unmatched++
		}
		if _, err := s.q.SetCatalogDisplayBrandMotomarks(ctx, params); err != nil {
			return matched, unmatched, err
		}
	}
	return matched, unmatched, nil
}

// refreshSyncedLogos denormalizes variants / colors onto display brands.
func (s *Service) refreshSyncedLogos(ctx context.Context) error {
	if _, err := s.q.RefreshCatalogDisplayBrandsSyncedLogos(ctx); err != nil {
		return err
	}
	_, err := s.q.ClearCatalogDisplayBrandsSyncedLogos(ctx)
	return err
}

// ---------- manual matching ----------

// ListUnmatchedDisplayBrands pages display brands with no mirror link.
func (s *Service) ListUnmatchedDisplayBrands(ctx context.Context, q string, limit, offset int32) ([]model.UnmatchedDisplayBrand, int64, error) {
	rows, err := s.q.ListUnmatchedCatalogDisplayBrands(ctx, db.ListUnmatchedCatalogDisplayBrandsParams{Q: text(q), LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountUnmatchedCatalogDisplayBrands(ctx, text(q))
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.UnmatchedDisplayBrand, 0, len(rows))
	for _, r := range rows {
		u := model.UnmatchedDisplayBrand{
			UUID: r.Uuid, Name: r.Name, Slug: r.Slug, IsActive: r.IsActive, TypesCount: r.TypesCount,
			LogoURL: DisplayBrandLogoURL(r.Uuid, r.LogoObjectKey, r.UpdatedAt),
		}
		if r.MotomarksSuggestedID.Valid {
			u.Suggestion = &model.BrandLogoSourceRef{ID: r.MotomarksSuggestedID.String, Name: r.SuggestedName.String}
			if v, ok := repository.NumericToFloat(r.MotomarksSuggestedScore); ok {
				u.Suggestion.Score = &v
			}
		}
		out = append(out, u)
	}
	return out, total, nil
}

// SearchBrandLogoSources is the picker over synced mirror brands.
func (s *Service) SearchBrandLogoSources(ctx context.Context, q string, limit int32) ([]model.BrandLogoSource, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.q.SearchBrandLogoSources(ctx, db.SearchBrandLogoSourcesParams{Q: text(q), LimitCount: limit})
	if err != nil {
		return nil, err
	}
	out := make([]model.BrandLogoSource, 0, len(rows))
	for _, r := range rows {
		src := model.BrandLogoSource{
			ID: r.MotomarksID, Name: r.Name, Color: optString(text(r.Color)), VehicleTypes: r.VehicleTypes, Variants: []string{},
		}
		if src.VehicleTypes == nil {
			src.VehicleTypes = []string{}
		}
		for v, k := range map[string]pgtype.Text{
			motomarks.VariantBadge: r.BadgeObjectKey, motomarks.VariantWordmark: r.WordmarkObjectKey, motomarks.VariantFull: r.FullObjectKey,
		} {
			if k.Valid {
				src.Variants = append(src.Variants, v)
			}
		}
		if r.SyncedAt.Valid {
			t := r.SyncedAt.Time
			src.SyncedAt = &t
		}
		for _, v := range motomarks.Variants {
			if contains(src.Variants, v) {
				u := fmt.Sprintf("/v1/platform/catalog/brand-logos/sources/%s/logo?variant=%s", r.MotomarksID, v)
				switch v {
				case motomarks.VariantBadge:
					src.BadgeURL = &u
				case motomarks.VariantWordmark:
					src.WordmarkURL = &u
				case motomarks.VariantFull:
					src.FullURL = &u
				}
			}
		}
		out = append(out, src)
	}
	return out, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// SetDisplayBrandMotomarks links a display brand to a mirror brand by hand
// (manual), marks it as having no logo (skip) or clears the link (none, so
// the next run matches it again).
func (s *Service) SetDisplayBrandMotomarks(ctx context.Context, id uuid.UUID, in model.SetDisplayBrandMotomarksInput) (model.DisplayBrand, error) {
	d, err := s.q.GetCatalogDisplayBrandByUUID(ctx, id)
	if err != nil {
		return model.DisplayBrand{}, mapErr(repository.MapErr(err))
	}
	params := db.SetCatalogDisplayBrandMotomarksParams{ID: d.ID, MotomarksMatchSource: MatchNone}
	switch in.Mode {
	case "link":
		mid := strings.TrimSpace(in.MotomarksID)
		if mid == "" {
			return model.DisplayBrand{}, fmt.Errorf("%w: motomarks_id is required", ErrInvalidRequest)
		}
		if _, err := s.q.GetBrandLogoSourceByMotomarksID(ctx, mid); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return model.DisplayBrand{}, fmt.Errorf("%w: unknown mirror brand %q (run a sync first)", ErrInvalidRequest, mid)
			}
			return model.DisplayBrand{}, err
		}
		params.MotomarksID = text(mid)
		params.MotomarksMatchSource = MatchManual
		params.MotomarksMatchScore = numeric(100)
	case "skip":
		params.MotomarksMatchSource = MatchSkip
	case "unlink":
		// defaults: none, no id, no suggestion
	default:
		return model.DisplayBrand{}, fmt.Errorf("%w: mode must be link, skip or unlink", ErrInvalidRequest)
	}
	if _, err := s.q.SetCatalogDisplayBrandMotomarks(ctx, params); err != nil {
		return model.DisplayBrand{}, err
	}
	if err := s.refreshSyncedLogos(ctx); err != nil {
		return model.DisplayBrand{}, err
	}
	s.publish(ctx, LiveBrands, resourceevents.ActionBulk)
	return s.GetDisplayBrand(ctx, id)
}

// ---------- logo streaming ----------

// Logo variants accepted by the stream endpoints.
const (
	LogoVariantDefault  = ""
	LogoVariantBadge    = motomarks.VariantBadge
	LogoVariantWordmark = motomarks.VariantWordmark
	LogoVariantFull     = motomarks.VariantFull
)

// ValidLogoVariant reports whether variant is one of the known values.
func ValidLogoVariant(v string) bool {
	switch v {
	case LogoVariantDefault, LogoVariantBadge, LogoVariantWordmark, LogoVariantFull:
		return true
	}
	return false
}

// resolveLogoKey picks the object to serve. uploaded is the admin logo.
//   - default / full: uploaded → full → badge → wordmark
//   - badge:          badge → uploaded → full
//   - wordmark:       wordmark only
func resolveLogoKey(variant string, uploaded, badge, wordmark, full pgtype.Text) (string, bool) {
	pick := func(keys ...pgtype.Text) (string, bool) {
		for _, k := range keys {
			if k.Valid && strings.TrimSpace(k.String) != "" {
				return k.String, true
			}
		}
		return "", false
	}
	switch variant {
	case LogoVariantBadge:
		return pick(badge, uploaded, full)
	case LogoVariantWordmark:
		return pick(wordmark)
	default:
		return pick(uploaded, full, badge, wordmark)
	}
}

// OpenDisplayBrandLogoVariant streams a display brand logo variant.
func (s *Service) OpenDisplayBrandLogoVariant(ctx context.Context, id uuid.UUID, variant string) (io.ReadCloser, string, error) {
	if !ValidLogoVariant(variant) {
		return nil, "", fmt.Errorf("%w: unknown logo variant", ErrInvalidRequest)
	}
	row, err := s.q.GetCatalogDisplayBrandLogoKeys(ctx, id)
	if err != nil {
		return nil, "", mapErr(repository.MapErr(err))
	}
	key, ok := resolveLogoKey(variant, row.LogoObjectKey, row.BadgeObjectKey, row.WordmarkObjectKey, row.FullObjectKey)
	if !ok || s.storage == nil {
		return nil, "", ErrNotFound
	}
	return s.openKey(ctx, key)
}

// OpenSourceLogo streams a synced mirror brand file (admin picker preview).
func (s *Service) OpenSourceLogo(ctx context.Context, motomarksID, variant string) (io.ReadCloser, string, error) {
	row, err := s.q.GetBrandLogoSourceByMotomarksID(ctx, motomarksID)
	if err != nil {
		return nil, "", mapErr(repository.MapErr(err))
	}
	key, ok := resolveLogoKey(variant, pgtype.Text{}, row.BadgeObjectKey, row.WordmarkObjectKey, row.FullObjectKey)
	if !ok || s.storage == nil {
		return nil, "", ErrNotFound
	}
	return s.openKey(ctx, key)
}

func (s *Service) openKey(ctx context.Context, key string) (io.ReadCloser, string, error) {
	rc, _, err := s.storage.Download(ctx, key)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return rc, storage.MIMEFromLogoKey(key), nil
}

// DisplayBrandLogoURLs builds the default, badge and wordmark URLs of a
// display brand from its uploaded key and synced variants.
func DisplayBrandLogoURLs(id uuid.UUID, uploaded pgtype.Text, variants []string, updated pgtype.Timestamptz) model.BrandLogoURLs {
	base := fmt.Sprintf("/v1/platform/catalog/display-brands/%s/logo", id)
	v := fmt.Sprintf("v=%d", updated.Time.Unix())
	hasUploaded := uploaded.Valid && strings.TrimSpace(uploaded.String) != ""
	var out model.BrandLogoURLs
	if hasUploaded || len(variants) > 0 {
		u := base + "?" + v
		out.LogoURL = &u
	}
	if hasUploaded || contains(variants, LogoVariantBadge) || contains(variants, LogoVariantFull) {
		u := base + "?variant=badge&" + v
		out.BadgeURL = &u
	}
	if contains(variants, LogoVariantWordmark) {
		u := base + "?variant=wordmark&" + v
		out.WordmarkURL = &u
	}
	return out
}
