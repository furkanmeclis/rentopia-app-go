// Package handler is the HTTP layer of the branded site (TEC-729):
// GET / PUT /v1/tenant/site (the settings the panel edits) and
// /v1/public/site/* (host resolution and images for the `site` app).
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
	CodeSiteNotFound    = "SITE_NOT_FOUND"
	CodeSubdomainTaken  = "SUBDOMAIN_TAKEN"
	CodeVersionConflict = "VERSION_CONFLICT"
	CodeRateLimited     = "RATE_LIMITED"
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

// Put serves PUT /v1/tenant/site.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	var in model.SaveInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, "Invalid request body",
			[]response.Detail{{Code: "invalid_json", Message: err.Error()}})
		return
	}
	out, err := h.svc.Save(r.Context(), scope.ID, scope.Slug, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), &p.UserInternal, "site.updated", model.Resource, nil, map[string]any{
			"version": out.Version, "enabled": out.Enabled, "subdomain": out.Subdomain,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, out)
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

// Resolve serves GET /v1/public/site/resolve?host=: the site of a request
// host. Unknown hosts answer 404 SITE_NOT_FOUND.
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
	out, err := h.svc.Resolve(r.Context(), host)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Asset serves GET /v1/public/site/assets/{uuid}/{kind}.
func (h *Handler) Asset(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "site:asset:ip", 1200) {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, CodeSiteNotFound, "Image was not found")
		return
	}
	rc, contentType, err := h.svc.Asset(r.Context(), id, r.PathValue("kind"))
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
	// The URL carries the site version (?v=), which changes on every save.
	w.Header().Set("Cache-Control", "public, max-age=86400")
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
	case errors.Is(err, usecase.ErrSubdomainTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeSubdomainTaken, err.Error(), []response.Detail{{
			Field: "subdomain", Code: "taken", Message: err.Error(),
		}})
	case errors.Is(err, usecase.ErrVersionConflict):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeVersionConflict, err.Error(), []response.Detail{{
			Field: "version", Code: "stale", Message: err.Error(),
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
