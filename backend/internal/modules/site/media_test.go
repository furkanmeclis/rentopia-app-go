package site_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	"github.com/google/uuid"
)

func mediaPNG() []byte {
	b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jXZkAAAAASUVORK5CYII=")
	return b
}

func (e *env) uploadMedia(o org) model.Media {
	e.t.Helper()
	var orgUUID uuid.UUID
	e.must(e.pool.QueryRow(context.Background(), "SELECT uuid FROM organizations WHERE id=$1", o.id).Scan(&orgUUID))
	out, err := e.svc.UploadMedia(context.Background(), o.id, orgUUID, 0, usecase.ImageUpload{Body: bytes.NewReader(mediaPNG())}, model.MediaInput{Alt: map[string]string{"tr": "Araç", "en": "Car"}})
	e.must(err)
	return out
}

func TestSiteMediaLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.uploadMedia(e.a)
	b := e.uploadMedia(e.b)
	if a.Width != 1 || a.Height != 1 || a.MIME != "image/png" {
		t.Fatal(a)
	}
	list, err := e.svc.ListMedia(ctx, e.a.id, 1, 0)
	e.must(err)
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].UUID != a.UUID {
		t.Fatal(list)
	}
	_, err = e.svc.PatchMedia(ctx, e.a.id, b.UUID, model.MediaInput{})
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatal(err)
	}
	if err = e.svc.DeleteMedia(ctx, e.a.id, b.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatal(err)
	}
	cfg, err := usecase.NormalizeConfig(model.DefaultConfig())
	e.must(err)
	cfg.About.Image = b.UUID.String()
	_, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Config: &cfg})
	var ve *usecase.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("foreign media accepted: %v", err)
	}
	cfg.About.Image = a.UUID.String()
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Config: &cfg})
	e.must(err)
	if err = e.svc.DeleteMedia(ctx, e.a.id, a.UUID); !errors.Is(err, usecase.ErrMediaInUse) {
		t.Fatal("draft reference", err)
	}
	preview, err := e.svc.IssuePreview(ctx, e.a.id, e.a.slug, 0)
	e.must(err)
	var siteID uuid.UUID
	e.must(e.pool.QueryRow(ctx, "SELECT uuid FROM site_settings WHERE organization_id=$1", e.a.id).Scan(&siteID))
	if rc, _, err := e.svc.PublicMedia(ctx, siteID, a.UUID, ""); err == nil {
		_ = rc.Close()
		t.Fatal("unpublished media public")
	}
	rc, _, err := e.svc.PublicMedia(ctx, siteID, a.UUID, preview.Token)
	e.must(err)
	_ = rc.Close()
	if rc, _, err := e.svc.PublicMedia(ctx, siteID, b.UUID, preview.Token); err == nil {
		_ = rc.Close()
		t.Fatal("cross-org media public")
	}
	e.publish(e.a, saved.Version)
	rc, _, err = e.svc.PublicMedia(ctx, siteID, a.UUID, "")
	e.must(err)
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, mediaPNG()) {
		t.Fatal("wrong bytes")
	}
	// Remove from draft and publish another revision: historical snapshots protect it.
	cfg.About.Image = ""
	saved, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: &saved.Version, Config: &cfg})
	e.must(err)
	if err = e.svc.DeleteMedia(ctx, e.a.id, a.UUID); !errors.Is(err, usecase.ErrMediaInUse) {
		t.Fatal("snapshot reference", err)
	}
	e.publish(e.a, saved.Version)
	if rc, _, err := e.svc.PublicMedia(ctx, siteID, a.UUID, ""); err == nil {
		_ = rc.Close()
		t.Fatal("historical reference authorized public access")
	}
	// An unreferenced image can be patched and deleted.
	unused := e.uploadMedia(e.a)
	patched, err := e.svc.PatchMedia(ctx, e.a.id, unused.UUID, model.MediaInput{Alt: map[string]string{"tr": "Yeni"}})
	e.must(err)
	if patched.Alt["tr"] != "Yeni" {
		t.Fatal(patched)
	}
	e.must(e.svc.DeleteMedia(ctx, e.a.id, unused.UUID))
	cfg.About.Image = unused.UUID.String()
	_, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: &saved.Version, Config: &cfg})
	if !errors.As(err, &ve) {
		t.Fatal("deleted reference accepted", err)
	}
}

func TestSiteMediaUploadValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, data := range [][]byte{[]byte("<svg></svg>"), []byte("<html>oops</html>"), []byte("\x89PNG\r\n\x1a\n"), {}, bytes.Repeat([]byte{'a'}, int(usecase.MaxMediaBytes)+1)} {
		_, err := e.svc.UploadMedia(ctx, e.a.id, uuid.New(), 0, usecase.ImageUpload{Body: bytes.NewReader(data), ContentType: "image/png", Size: 1}, model.MediaInput{})
		if err == nil {
			t.Fatal("invalid upload accepted")
		}
	}
	_, err := e.svc.UploadMedia(ctx, e.a.id, uuid.New(), 0, usecase.ImageUpload{Body: bytes.NewReader(mediaPNG()), Size: usecase.MaxMediaBytes + 1}, model.MediaInput{})
	if err == nil {
		t.Fatal("oversized declared upload accepted")
	}
}

func TestSiteMediaQuotaConcurrent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// Real metadata rows leave one slot; concurrent uploads compete for it.
	_, err := e.pool.Exec(ctx, `INSERT INTO site_media (organization_id,mime,size,width,height,object_key) SELECT $1,'image/png',1,1,1,'quota/'||gen_random_uuid() FROM generate_series(1,199)`, e.a.id)
	e.must(err)
	var orgUUID uuid.UUID
	e.must(e.pool.QueryRow(ctx, "SELECT uuid FROM organizations WHERE id=$1", e.a.id).Scan(&orgUUID))
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.UploadMedia(ctx, e.a.id, orgUUID, 0, usecase.ImageUpload{Body: bytes.NewReader(mediaPNG())}, model.MediaInput{})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, quota := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, usecase.ErrMediaQuota) {
			quota++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || quota != 1 {
		t.Fatalf("success=%d quota=%d", success, quota)
	}
	// Byte quota is independent of file count.
	_, err = e.pool.Exec(ctx, `INSERT INTO site_media (organization_id,mime,size,width,height,object_key) SELECT $1,'image/png',10485760,1,1,'bytes/'||gen_random_uuid() FROM generate_series(1,50)`, e.b.id)
	e.must(err)
	_, err = e.svc.UploadMedia(ctx, e.b.id, uuid.New(), 0, usecase.ImageUpload{Body: bytes.NewReader(mediaPNG())}, model.MediaInput{})
	if !errors.Is(err, usecase.ErrMediaQuota) {
		t.Fatal(err)
	}
}

func TestSiteMediaPublicHTTP(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.uploadMedia(e.a)
	cfg := model.DefaultConfig()
	cfg.About.Image = a.UUID.String()
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Config: &cfg})
	e.must(err)
	preview, err := e.svc.IssuePreview(ctx, e.a.id, e.a.slug, 0)
	e.must(err)
	resolved, err := e.svc.Resolve(ctx, saved.Hosts[0].Host, preview.Token)
	e.must(err)
	path := resolved.Media[a.UUID.String()]
	if !strings.Contains(path, "/v1/public/site/media/") {
		t.Fatal(path)
	}
	response, err := http.Get(e.server.URL + path)
	e.must(err)
	_ = response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatal(response.Status, response.Header)
	}
	e.publish(e.a, saved.Version)
	resolved, err = e.svc.Resolve(ctx, saved.Hosts[0].Host, "")
	e.must(err)
	response, err = http.Get(e.server.URL + resolved.Media[a.UUID.String()])
	e.must(err)
	_ = response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(response.Header.Get("Cache-Control"), "immutable") {
		t.Fatal(response.Status, response.Header)
	}
	response, err = http.Get(e.server.URL + resolved.Media[a.UUID.String()] + "?preview=bad")
	e.must(err)
	_ = response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal(response.Status)
	}
	// Disabled/suspended organizations must never authorize an uncached fetch.
	_, err = e.pool.Exec(ctx, "UPDATE organizations SET status='suspended' WHERE id=$1", e.a.id)
	e.must(err)
	response, err = http.Get(e.server.URL + resolved.Media[a.UUID.String()])
	e.must(err)
	_ = response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal(response.Status)
	}
}

func TestConfigV2MigrationDraftAndSnapshots(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0)})
	e.must(err)
	e.publish(e.a, saved.Version)
	legacy := []byte(`{"theme":{"radius":"none"},"content":{"tr":{"about":"Legacy story","hero_title":"Legacy"}}}`)
	_, err = e.pool.Exec(ctx, `UPDATE site_settings SET config=$2 WHERE organization_id=$1`, e.a.id, legacy)
	e.must(err)
	_, err = e.pool.Exec(ctx, `UPDATE site_setting_versions SET config=$2 WHERE organization_id=$1`, e.a.id, legacy)
	e.must(err)
	for _, table := range []string{"site_settings", "site_setting_versions"} {
		_, err = e.pool.Exec(ctx, "UPDATE "+table+" SET config=site_config_v2(config) WHERE organization_id=$1", e.a.id)
		e.must(err)
		var raw []byte
		e.must(e.pool.QueryRow(ctx, "SELECT config FROM "+table+" WHERE organization_id=$1", e.a.id).Scan(&raw))
		var cfg model.Config
		e.must(json.Unmarshal(raw, &cfg))
		if cfg.Template != "technowide-design" || cfg.ConfigVersion != 2 || cfg.Theme.Radius != "sharp" || cfg.Content["tr"].About != "Legacy story" {
			t.Fatal(string(raw))
		}
		var same bool
		e.must(e.pool.QueryRow(ctx, "SELECT config=site_config_v2(config) FROM "+table+" WHERE organization_id=$1", e.a.id).Scan(&same))
		if !same {
			t.Fatal("migration not idempotent")
		}
	}
}

func TestSiteMediaJPEGAndWebP(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var jpegBytes bytes.Buffer
	e.must(jpeg.Encode(&jpegBytes, image.NewRGBA(image.Rect(0, 0, 3, 2)), nil))
	webpBytes, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	e.must(err)
	for _, tt := range []struct {
		mime          string
		data          []byte
		width, height int
	}{
		{"image/jpeg", jpegBytes.Bytes(), 3, 2},
		{"image/webp", webpBytes, 1, 1},
	} {
		got, err := e.svc.UploadMedia(ctx, e.a.id, uuid.New(), 0, usecase.ImageUpload{Body: bytes.NewReader(tt.data), ContentType: "text/html"}, model.MediaInput{})
		e.must(err)
		if got.MIME != tt.mime || got.Width != tt.width || got.Height != tt.height {
			t.Fatal(got)
		}
	}
}

func TestSiteMediaSaveDeleteSerialization(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := e.uploadMedia(e.a)
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0)})
	e.must(err)
	cfg := saved.Config
	cfg.About.Image = m.UUID.String()
	raw, err := json.Marshal(cfg)
	e.must(err)
	tx, err := e.pool.Begin(ctx)
	e.must(err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx, "UPDATE site_settings SET config=$2 WHERE organization_id=$1", e.a.id, raw)
	e.must(err)
	result := make(chan error, 1)
	go func() { result <- e.svc.DeleteMedia(ctx, e.a.id, m.UUID) }()
	e.must(tx.Commit(ctx))
	if err := <-result; !errors.Is(err, usecase.ErrMediaInUse) {
		t.Fatal("concurrent deletion lost draft reference", err)
	}

	// Reverse ordering: deletion holds the organization lock, then a save
	// must see the committed absence and reject the media UUID.
	unused := e.uploadMedia(e.a)
	tx, err = e.pool.Begin(ctx)
	e.must(err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx, "DELETE FROM site_media WHERE organization_id=$1 AND uuid=$2", e.a.id, unused.UUID)
	e.must(err)
	cfg.About.Image = unused.UUID.String()
	go func() {
		_, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: &saved.Version, Config: &cfg})
		result <- err
	}()
	e.must(tx.Commit(ctx))
	var ve *usecase.ValidationError
	if err := <-result; !errors.As(err, &ve) {
		t.Fatal("concurrent save accepted deleted UUID", err)
	}
}
