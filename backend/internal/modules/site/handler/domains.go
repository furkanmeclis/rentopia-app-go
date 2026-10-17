package handler

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) SetDomains(d *usecase.Domains, secret string) {
	h.domains, h.domainSecret = d, secret
}
func (h *Handler) ListDomains(w http.ResponseWriter, r *http.Request) {
	out, err := h.domains.List(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Host string `json:"host"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.domains.Create(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, in.Host)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	h.record(r, "site.domain_created", map[string]any{"uuid": out.UUID, "host": out.Host})
	response.JSON(w, r, http.StatusCreated, out)
}
func domainID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Domain not found")
		return uuid.Nil, false
	}
	return id, true
}
func (h *Handler) GetDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := domainID(w, r)
	if !ok {
		return
	}
	out, err := h.domains.Get(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := domainID(w, r)
	if !ok {
		return
	}
	if err := h.domains.Remove(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeDomainError(w, r, err)
		return
	}
	h.record(r, "site.domain_removed", map[string]any{"uuid": id})
	response.JSON(w, r, http.StatusOK, map[string]bool{"removed": true})
}
func (h *Handler) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	h.recheckDomain(w, r, orgctx.Must(r.Context()).ID)
}
func (h *Handler) RecheckDomain(w http.ResponseWriter, r *http.Request) { h.recheckDomain(w, r, 0) }
func (h *Handler) recheckDomain(w http.ResponseWriter, r *http.Request, orgID int64) {
	id, ok := domainID(w, r)
	if !ok {
		return
	}
	out, err := h.domains.Recheck(r.Context(), orgID, id)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, out)
}
func (h *Handler) PlatformDomains(w http.ResponseWriter, r *http.Request) {
	limit, offset := 50, 0
	if value := r.URL.Query().Get("limit"); value != "" {
		n, err := strconv.Atoi(value)
		if err == nil {
			limit = n
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		n, err := strconv.Atoi(value)
		if err == nil {
			offset = n
		}
	}
	if limit < 1 || limit > 200 || offset < 0 || offset > 1000000 {
		response.ValidationError(w, r, []response.Detail{{Field: "limit", Code: "invalid", Message: "limit 1–200; offset 0–1000000"}})
		return
	}
	out, err := h.domains.ListPlatform(r.Context(), int32(limit), int32(offset))
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) MarkDomainTLSActive(w http.ResponseWriter, r *http.Request) {
	id, ok := domainID(w, r)
	if !ok {
		return
	}
	out, err := h.domains.MarkTLSActive(r.Context(), id)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DomainAllowed is an internal certificate authorization endpoint; empty secrets fail closed.
func (h *Handler) DomainAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	token := r.Header.Get("X-Auth-Adapter-Key")
	if h.domainSecret == "" || subtle.ConstantTimeCompare([]byte(token), []byte(h.domainSecret)) != 1 {
		response.Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid internal credential")
		return
	}
	if h.domains == nil {
		response.NotFound(w, r, "Domain not found")
		return
	}
	_, err := h.domains.Lookup(r.Context(), r.URL.Query().Get("host"))
	if errors.Is(err, usecase.ErrNotFound) {
		response.NotFound(w, r, "Domain not found")
		return
	}
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"allowed": true})
}
func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrDomainLimit):
		response.Error(w, r, http.StatusConflict, "DOMAIN_LIMIT", err.Error())
	case errors.Is(err, usecase.ErrDomainTaken):
		response.Error(w, r, http.StatusConflict, "DOMAIN_TAKEN", err.Error())
	case errors.Is(err, usecase.ErrDomainMode):
		response.Error(w, r, http.StatusConflict, "DOMAIN_TLS_MODE", err.Error())
	default:
		writeError(w, r, err)
	}
}
