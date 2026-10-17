// Package handler is the HTTP layer of the branded site (TEC-729 /
// TEC-730): /v1/tenant/site (the settings the panel edits: live fields,
// the draft, publish history, preview links, images, catalog publish
// state, the site key) and /v1/public/site/* (host resolution and images
// for the `site` app).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the site routes.
const (
	CodeSiteNotFound      = "SITE_NOT_FOUND"
	CodeSubdomainTaken    = "SUBDOMAIN_TAKEN"
	CodeVersionConflict   = "VERSION_CONFLICT"
	CodeNothingToPublish  = "NOTHING_TO_PUBLISH"
	CodePreviewInvalid    = "PREVIEW_INVALID"
	CodeRateLimited       = "RATE_LIMITED"
	CodeStorageNotEnabled = "STORAGE_UNAVAILABLE"
)

// RateLimiter is the fixed-window limiter (ratelimit.Limiter); it fails
// open without Redis.
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Handler serves the site routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	limiter  RateLimiter
}

// New builds the handler. rec and limiter may be nil.
func New(svc *usecase.Service, rec *activity.Recorder, limiter RateLimiter) *Handler {
	return &Handler{svc: svc, activity: rec, limiter: limiter}
}

// ----------------------------------------------------------------- panel

func (h *Handler) record(r *http.Request, action string, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, nil, payload, r)
}

// decode reads a strict JSON body (an empty body reads as {}).
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, "Invalid request body",
			[]response.Detail{{Code: "invalid_json", Message: err.Error()}})
		return false
	}
	return true
}

// Get serves GET /v1/tenant/site.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.Get(r.Context(), scope.ID, scope.Slug)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Put serves PUT /v1/tenant/site: the live fields and the draft.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	var in model.SaveInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Save(r.Context(), scope.ID, scope.Slug, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "site.updated", map[string]any{"version": out.Version, "enabled": out.Enabled, "subdomain": out.Subdomain})
	response.JSON(w, r, http.StatusOK, out)
}

// Publish serves POST /v1/tenant/site/publish.
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	var in model.VersionInput
	if !decode(w, r, &in) {
		return
	}
	out, rev, err := h.svc.Publish(r.Context(), scope.ID, scope.Slug, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "site.published", map[string]any{"revision": rev, "version": out.Version})
	response.JSON(w, r, http.StatusOK, out)
}

// Versions serves GET /v1/tenant/site/versions.
func (h *Handler) Versions(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.Versions(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Restore serves POST /v1/tenant/site/versions/{revisionUuid}/restore.
func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("revisionUuid"))
	if err != nil {
		response.NotFound(w, r, "Revision not found")
		return
	}
	var in model.VersionInput
	if !decode(w, r, &in) {
		return
	}
	out, rev, err := h.svc.Restore(r.Context(), scope.ID, scope.Slug, p.UserInternal, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "site.restored", map[string]any{"revision": rev, "version": out.Version})
	response.JSON(w, r, http.StatusOK, out)
}

// PreviewToken serves POST /v1/tenant/site/preview-token.
func (h *Handler) PreviewToken(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.IssuePreview(r.Context(), scope.ID, scope.Slug, p.UserInternal)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// RenewKey serves POST /v1/tenant/site/renew-key.
func (h *Handler) RenewKey(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	var in model.VersionInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.RenewKey(r.Context(), scope.ID, scope.Slug, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"version": out.Version}
	if out.APIKey != nil {
		payload["key_uuid"] = out.APIKey.UUID
	}
	h.record(r, "site.key_renewed", payload)
	response.JSON(w, r, http.StatusOK, out)
}

func formVersion(raw string) *int {
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		v = -1
	}
	return &v
}

// PutImage serves PUT /v1/tenant/site/assets/{logo|favicon|cover}
// (multipart: file, version).
func (h *Handler) PutImage(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := orgctx.Must(r.Context())
		p := authctx.MustPrincipal(r.Context())
		limit := usecase.MaxImageBytes[kind]
		r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
		if err := r.ParseMultipartForm(limit + (1 << 19)); err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "file", Code: "too_large", Message: "the upload is too large or not a multipart form"}})
			return
		}
		defer func() {
			if r.MultipartForm != nil {
				_ = r.MultipartForm.RemoveAll()
			}
		}()
		file, header, err := r.FormFile("file")
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "file", Code: "required", Message: "a file is required"}})
			return
		}
		defer func() { _ = file.Close() }()
		out, err := h.svc.SetImage(r.Context(), scope.ID, scope.UUID, scope.Slug, p.UserInternal, kind,
			formVersion(r.FormValue("version")), usecase.ImageUpload{
				Body: file, Size: header.Size, ContentType: header.Header.Get("Content-Type"), Filename: header.Filename,
			})
		if err != nil {
			writeError(w, r, err)
			return
		}
		h.record(r, "site.image_updated", map[string]any{"kind": kind, "version": out.Version})
		response.JSON(w, r, http.StatusOK, out)
	}
}

// DeleteImage serves DELETE /v1/tenant/site/assets/{logo|favicon|cover}?version=.
func (h *Handler) DeleteImage(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := orgctx.Must(r.Context())
		p := authctx.MustPrincipal(r.Context())
		out, err := h.svc.RemoveImage(r.Context(), scope.ID, scope.Slug, p.UserInternal, kind,
			formVersion(r.URL.Query().Get("version")))
		if err != nil {
			writeError(w, r, err)
			return
		}
		h.record(r, "site.image_removed", map[string]any{"kind": kind, "version": out.Version})
		response.JSON(w, r, http.StatusOK, out)
	}
}

// Catalog serves GET /v1/tenant/site/catalog.
func (h *Handler) Catalog(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.Catalog(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// catalogPatch serves PATCH /v1/tenant/site/catalog/{kind}/{uuid}.
func catalogPatch[T any](h *Handler, kind string, set func(context.Context, int64, uuid.UUID, model.VisibilityInput) (T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := orgctx.Must(r.Context())
		id, err := uuid.Parse(r.PathValue("uuid"))
		if err != nil {
			response.NotFound(w, r, "Record not found")
			return
		}
		var in model.VisibilityInput
		if !decode(w, r, &in) {
			return
		}
		out, err := set(r.Context(), scope.ID, id, in)
		if err != nil {
			writeError(w, r, err)
			return
		}
		h.record(r, "site.catalog_updated", map[string]any{"kind": kind, "uuid": id, "show_online": *in.ShowOnline})
		response.JSON(w, r, http.StatusOK, out)
	}
}

// PatchBranch serves PATCH /v1/tenant/site/catalog/branches/{uuid}.
func (h *Handler) PatchBranch(w http.ResponseWriter, r *http.Request) {
	catalogPatch(h, "branch", h.svc.SetBranchOnline)(w, r)
}

// PatchSubgroup serves PATCH /v1/tenant/site/catalog/products/{uuid}.
func (h *Handler) PatchSubgroup(w http.ResponseWriter, r *http.Request) {
	catalogPatch(h, "subgroup", h.svc.SetSubgroupOnline)(w, r)
}

// PatchExtra serves PATCH /v1/tenant/site/catalog/extras/{uuid}.
func (h *Handler) PatchExtra(w http.ResponseWriter, r *http.Request) {
	catalogPatch(h, "extra", h.svc.SetExtraOnline)(w, r)
}

// ---------------------------------------------------------------- public

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action string, limit int) bool {
	if h.limiter == nil {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, httpkit.ClientIP(r), limit, time.Minute)
	if ok {
		return true
	}
	if retry <= 0 {
		retry = time.Minute
	}
	w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
	response.Error(w, r, http.StatusTooManyRequests, CodeRateLimited, "Too many requests; try again later")
	return false
}

// Resolve serves GET /v1/public/site/resolve?host=[&preview=]: the site of
// a request host (the published snapshot, or the draft for a valid
// preview token). Unknown hosts answer 404 SITE_NOT_FOUND.
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.allow(w, r, "site:resolve:ip", 600) {
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		response.ValidationError(w, r, []response.Detail{{Field: "host", Code: "required", Message: "host is required"}})
		return
	}
	out, err := h.svc.Resolve(r.Context(), host, r.URL.Query().Get("preview"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Asset serves GET /v1/public/site/assets/{uuid}/{kind}[?preview=].
func (h *Handler) Asset(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "site:asset:ip", 1200) {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, CodeSiteNotFound, "Image was not found")
		return
	}
	preview := r.URL.Query().Get("preview")
	rc, contentType, err := h.svc.Asset(r.Context(), id, r.PathValue("kind"), preview)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.Error(w, r, http.StatusNotFound, CodeSiteNotFound, "Image was not found")
			return
		}
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if preview != "" {
		w.Header().Set("Cache-Control", "private, no-store")
	} else {
		// The URL carries the published revision (?v=), which changes on
		// every publish.
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.Error(w, r, http.StatusNotFound, CodeSiteNotFound, "No site answers this address")
	case errors.Is(err, usecase.ErrRevisionNotFound), errors.Is(err, usecase.ErrCatalogItemNotFound):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, usecase.ErrPreviewInvalid):
		response.Error(w, r, http.StatusUnauthorized, CodePreviewInvalid, err.Error())
	case errors.Is(err, usecase.ErrSubdomainTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeSubdomainTaken, err.Error(), []response.Detail{{
			Field: "subdomain", Code: "taken", Message: err.Error(),
		}})
	case errors.Is(err, usecase.ErrVersionConflict):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeVersionConflict, err.Error(), []response.Detail{{
			Field: "version", Code: "stale", Message: err.Error(),
		}})
	case errors.Is(err, usecase.ErrNothingToPublish):
		response.Error(w, r, http.StatusConflict, CodeNothingToPublish, err.Error())
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, CodeStorageNotEnabled, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
