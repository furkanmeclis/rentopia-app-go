package usecase_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/dbtest"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 1x1 PNG and WebP files so the MIME sniffer accepts the downloads.
var (
	pngBytes  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82")
	webpBytes = []byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00\x2f\x00\x00\x00\x00\x07\x10\x11\x11\x88\x88\xfe\x07\x00")
)

// fakeMirror is a tiny motomarks-mirror: brands with an ETag, batch match
// and logo files.
type fakeMirror struct {
	srv       *httptest.Server
	brands    []map[string]any
	hits      map[string]int
	downloads int
}

func newFakeMirror(t *testing.T) *fakeMirror {
	t.Helper()
	m := &fakeMirror{hits: map[string]int{}}
	mux := http.NewServeMux()
	logos := func(id string, variants ...string) map[string]any {
		out := map[string]any{}
		for _, v := range variants {
			out[v] = map[string]any{"md": map[string]any{"webp": m.srv.URL + "/files/" + id + "/" + v + ".webp"}}
		}
		return out
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /brands", func(w http.ResponseWriter, r *http.Request) {
		m.hits["brands"]++
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("If-None-Match") == `"e1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"e1"`)
		m.brands = []map[string]any{
			{"id": "fiat", "name": "Fiat", "color": "#E42313", "vehicleTypes": []string{"car"}, "logos": logos("fiat", "badge", "wordmark", "full")},
			{"id": "bmw", "name": "BMW", "color": "#0066B1", "vehicleTypes": []string{"car"}, "logos": logos("bmw", "full")},
			{"id": "land-rover", "name": "Land Rover", "color": "#005A2B", "vehicleTypes": []string{"car"}, "logos": logos("land-rover", "badge")},
		}
		_ = json.NewEncoder(w).Encode(m.brands)
	})
	mux.HandleFunc("POST /match", func(w http.ResponseWriter, r *http.Request) {
		m.hits["match"]++
		var in struct {
			Queries []string `json:"queries"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		results := make([]map[string]any, 0, len(in.Queries))
		for _, q := range in.Queries {
			switch strings.ToLower(q) {
			case "fiat":
				results = append(results, map[string]any{"query": q, "matched": true, "score": 100, "brand": map[string]any{"id": "fiat", "name": "Fiat"}})
			case "bmw":
				results = append(results, map[string]any{"query": q, "matched": true, "score": 100, "brand": map[string]any{"id": "bmw", "name": "BMW"}})
			case "range rover":
				// Below the threshold: becomes a suggestion only.
				results = append(results, map[string]any{"query": q, "matched": false, "score": 61, "brand": map[string]any{"id": "land-rover", "name": "Land Rover"}})
			default:
				results = append(results, map[string]any{"query": q, "matched": false, "score": 0, "brand": nil})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	mux.HandleFunc("GET /files/{id}/{file}", func(w http.ResponseWriter, r *http.Request) {
		m.downloads++
		if strings.HasSuffix(r.PathValue("file"), ".png") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(webpBytes)
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

// TestBrandLogoSyncAgainstPostgres runs a full sync on a private database:
// sources upserted, variants copied to storage, display brands matched (auto
// above the threshold, suggestion below), ETag revalidation on the second
// run, manual link and variant streaming.
func TestBrandLogoSyncAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("CATALOG_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CATALOG_TEST_DATABASE_URL not set")
	}
	dsn = dbtest.FreshDatabase(t, dsn)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mem := storage.NewMemory()
	q := db.New(pool)
	svc := usecase.New(repository.NewPostgres(pool, q), memDriver{m: mem}, nil, nil, nil)
	box, err := crypto.NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	svc.SetSecretBox(box)
	mirror := newFakeMirror(t)

	// Catalog with three display brands (Fiat from two TSB codes, BMW, Range Rover).
	sheet := &tsbcatalog.Sheet{Period: "2026-10", Rows: []tsbcatalog.Row{
		{BrandCode: "52", BrandName: "FIAT", TypeCode: "1", TypeName: "EGEA SEDAN URBAN 1.4 FIRE 95", Values: map[int]int64{2026: 900000}},
		{BrandCode: "100", BrandName: "TOFAS-FIAT", TypeCode: "1", TypeName: "EGEA CROSS STREET 1.4 FIRE 95", Values: map[int]int64{2025: 1000000}},
		{BrandCode: "21", BrandName: "BMW", TypeCode: "1", TypeName: "320d SEDAN 2.0 190", Values: map[int]int64{2026: 3000000}},
		{BrandCode: "125", BrandName: "RANGE ROVER", TypeCode: "1", TypeName: "3.0 D300 HSE", Values: map[int]int64{2026: 9000000}},
	}}
	if _, _, err := svc.Run(ctx, sheet, "", true); err != nil {
		t.Fatal(err)
	}

	// Not configured yet.
	if _, err := svc.RequestBrandLogoSync(ctx, 0); err == nil {
		t.Fatal("sync without settings must fail")
	}
	tok := "tok"
	enabled := true
	if _, err := svc.PatchBrandLogoSettings(ctx, model.PatchBrandLogoSettingsInput{BaseURL: &mirror.srv.URL, APIToken: &tok, Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	settings, err := svc.GetBrandLogoSettings(ctx)
	if err != nil || !settings.TokenConfigured || settings.BaseURL != mirror.srv.URL || settings.AutoMatchMinScore != 85 {
		t.Fatalf("settings: %v %+v", err, settings)
	}
	if err := svc.TestBrandLogoConnection(ctx); err != nil {
		t.Fatalf("test connection: %v", err)
	}

	// First run (inline: no queue).
	run, err := svc.RequestBrandLogoSync(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "done" || run.BrandsSeen != 3 || run.Downloaded != 5 || run.Matched != 2 || run.Unmatched != 1 || run.NotModified {
		t.Fatalf("first run: %+v", run)
	}
	if mirror.downloads != 5 {
		t.Fatalf("downloads = %d", mirror.downloads)
	}
	settings, _ = svc.GetBrandLogoSettings(ctx)
	if settings.SourcesCount != 3 || settings.MatchCounts["auto"] != 2 || settings.MatchCounts["none"] != 1 || settings.LastStatus == nil || *settings.LastStatus != "done" {
		t.Fatalf("after first run: %+v", settings)
	}

	// Fiat: auto matched, three variants, URLs built from the synced files.
	brands, _, err := svc.ListDisplayBrands(ctx, model.DisplayBrandFilter{Limit: 10, SortField: "name"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]model.DisplayBrand{}
	for _, b := range brands {
		byName[b.Name] = b
	}
	fiat := byName["Fiat"]
	if fiat.Motomarks == nil || fiat.Motomarks.MatchSource != "auto" || fiat.Motomarks.ID == nil || *fiat.Motomarks.ID != "fiat" {
		t.Fatalf("fiat link: %+v", fiat.Motomarks)
	}
	if fiat.LogoURL == nil || fiat.LogoBadgeURL == nil || fiat.LogoWordmarkURL == nil || fiat.BrandColor == nil || *fiat.BrandColor != "#E42313" {
		t.Fatalf("fiat urls: %+v", fiat)
	}
	bmw := byName["BMW"]
	if bmw.LogoWordmarkURL != nil || bmw.LogoBadgeURL == nil {
		t.Fatalf("bmw urls (full only): %+v", bmw)
	}
	rr := byName["Range Rover"]
	if rr.Motomarks == nil || rr.Motomarks.MatchSource != "none" || rr.Motomarks.SuggestedID == nil || *rr.Motomarks.SuggestedID != "land-rover" || rr.LogoURL != nil {
		t.Fatalf("range rover suggestion: %+v", rr.Motomarks)
	}
	unmatched, total, err := svc.ListUnmatchedDisplayBrands(ctx, "", 10, 0)
	if err != nil || total != 1 || len(unmatched) != 1 || unmatched[0].Suggestion == nil || unmatched[0].Suggestion.Name != "Land Rover" {
		t.Fatalf("unmatched: %v %d %+v", err, total, unmatched)
	}

	// Display brand streams: wordmark from the synced file, badge falls back to full.
	rc, mime, err := svc.OpenDisplayBrandLogoVariant(ctx, fiat.UUID, "wordmark")
	if err != nil || mime != "image/webp" {
		t.Fatalf("fiat wordmark: %v %q", err, mime)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != string(webpBytes) {
		t.Fatal("wordmark bytes differ")
	}
	if _, _, err := svc.OpenDisplayBrandLogoVariant(ctx, bmw.UUID, "wordmark"); err == nil {
		t.Fatal("bmw has no wordmark: want 404")
	}
	if rc, _, err := svc.OpenDisplayBrandLogoVariant(ctx, bmw.UUID, "badge"); err != nil {
		t.Fatalf("bmw badge falls back to full: %v", err)
	} else {
		_ = rc.Close()
	}

	// Second run: 304, nothing downloaded, matching retried for the one left.
	run2, err := svc.RequestBrandLogoSync(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !run2.NotModified || run2.Downloaded != 0 || run2.Matched != 0 || run2.Unmatched != 1 || mirror.downloads != 5 {
		t.Fatalf("second run: %+v downloads=%d", run2, mirror.downloads)
	}

	// Manual link keeps on later runs; skip clears the queue.
	linked, err := svc.SetDisplayBrandMotomarks(ctx, rr.UUID, model.SetDisplayBrandMotomarksInput{Mode: "link", MotomarksID: "land-rover"})
	if err != nil || linked.Motomarks.MatchSource != "manual" || linked.LogoBadgeURL == nil {
		t.Fatalf("manual link: %v %+v", err, linked.Motomarks)
	}
	if _, err := svc.SetDisplayBrandMotomarks(ctx, rr.UUID, model.SetDisplayBrandMotomarksInput{Mode: "link", MotomarksID: "nope"}); err == nil {
		t.Fatal("unknown mirror brand must fail")
	}
	run3, err := svc.RequestBrandLogoSync(ctx, 0)
	if err != nil || run3.Unmatched != 0 {
		t.Fatalf("third run: %v %+v", err, run3)
	}
	again, _ := svc.GetDisplayBrand(ctx, rr.UUID)
	if again.Motomarks.MatchSource != "manual" {
		t.Fatalf("manual link overwritten: %+v", again.Motomarks)
	}
	runs, n, err := svc.ListBrandLogoSyncRuns(ctx, 10, 0)
	if err != nil || n != 3 || len(runs) != 3 || runs[0].Status != "done" {
		t.Fatalf("runs: %v %d %+v", err, n, runs)
	}

	// Token masked: the settings payload never carries it.
	raw, _ := json.Marshal(settings)
	if strings.Contains(string(raw), "tok\"") {
		t.Fatalf("token leaked: %s", raw)
	}
}
