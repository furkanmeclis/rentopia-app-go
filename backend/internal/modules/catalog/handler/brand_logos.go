package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Brand logo sync (motomarks-mirror) admin endpoints.

func (h *Handler) GetBrandLogoSettings(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetBrandLogoSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) PatchBrandLogoSettings(w http.ResponseWriter, r *http.Request) {
	var in model.PatchBrandLogoSettingsInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	out, err := h.svc.PatchBrandLogoSettings(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.brand_logos_settings_updated", "platform.catalog", nil, map[string]any{
			"base_url": out.BaseURL, "enabled": out.Enabled, "auto_match_min_score": out.AutoMatchMinScore,
			"token_changed": in.APIToken != nil,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) TestBrandLogoConnection(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.TestBrandLogoConnection(r.Context()); err != nil {
		if errors.Is(err, usecase.ErrSyncNotConfigured) {
			response.BadRequest(w, r, response.CodeValidationError, "set the mirror base URL and token first")
			return
		}
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) RequestBrandLogoSync(w http.ResponseWriter, r *http.Request) {
	run, err := h.svc.RequestBrandLogoSync(r.Context(), *actor(r))
	if err != nil {
		if errors.Is(err, usecase.ErrSyncRunning) {
			response.Conflict(w, r, response.CodeConflict, err.Error())
			return
		}
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, run)
}

func (h *Handler) ListBrandLogoSyncRuns(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, nil)
	if !ok {
		return
	}
	items, total, err := h.svc.ListBrandLogoSyncRuns(r.Context(), q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) ListUnmatchedDisplayBrands(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, nil)
	if !ok {
		return
	}
	items, total, err := h.svc.ListUnmatchedDisplayBrands(r.Context(), q.Q, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) SearchBrandLogoSources(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, err := h.svc.SearchBrandLogoSources(r.Context(), q.Q, q.Limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) SetDisplayBrandMotomarks(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.SetDisplayBrandMotomarksInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	d, err := h.svc.SetDisplayBrandMotomarks(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.display_brand_logo_linked", "platform.catalog", &d.UUID, map[string]any{
			"name": d.Name, "mode": in.Mode, "motomarks_id": in.MotomarksID,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, d)
}

func logoVariant(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(r.URL.Query().Get("variant")))
}

func streamLogo(w http.ResponseWriter, r *http.Request, rc io.ReadCloser, mime string, err error, cacheControl string) {
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.NotFound(w, r, "logo not set")
			return
		}
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", cacheControl)
	_, _ = io.Copy(w, rc)
}

// StreamSourceLogo previews a synced mirror brand file (admin picker).
func (h *Handler) StreamSourceLogo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		response.BadRequest(w, r, response.CodeValidationError, "id is invalid")
		return
	}
	rc, mime, err := h.svc.OpenSourceLogo(r.Context(), id, logoVariant(r))
	streamLogo(w, r, rc, mime, err, "private, max-age=86400")
}
