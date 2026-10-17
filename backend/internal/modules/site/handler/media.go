package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) ListMedia(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	out, err := h.svc.ListMedia(r.Context(), scope.ID, limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxMediaBytes+(1<<20))
	if err := r.ParseMultipartForm(usecase.MaxMediaBytes + (1 << 19)); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Code: "too_large", Message: "invalid or oversized multipart upload"}})
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
	in := model.MediaInput{}
	if raw := r.FormValue("alt"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &in.Alt); err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "alt", Code: "invalid", Message: "alt must be a localized JSON object"}})
			return
		}
	}
	out, err := h.svc.UploadMedia(r.Context(), scope.ID, scope.UUID, p.UserInternal, usecase.ImageUpload{Body: file, Size: header.Size, Filename: header.Filename}, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "site.media_uploaded", map[string]any{"uuid": out.UUID})
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) PatchMedia(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Media not found")
		return
	}
	var in model.MediaInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.PatchMedia(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "site.media_updated", map[string]any{"uuid": id})
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Media not found")
		return
	}
	if err := h.svc.DeleteMedia(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "site.media_deleted", map[string]any{"uuid": id})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handler) PublicMedia(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "site:media:ip", 1200) {
		return
	}
	site, err := uuid.Parse(r.PathValue("siteUuid"))
	id, errID := uuid.Parse(r.PathValue("mediaUuid"))
	if err != nil || errID != nil {
		response.NotFound(w, r, "Media not found")
		return
	}
	preview := r.URL.Query().Get("preview")
	rc, mime, err := h.svc.PublicMedia(r.Context(), site, id, preview)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if preview == "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}
