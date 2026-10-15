// Package handler is the HTTP layer of the operations overview. Both
// routes run behind RequireOrganization; the organization and the branch
// scope come from orgctx, never from the client.
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/overview/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/overview.
type Handler struct {
	svc *usecase.Service
}

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Overview serves GET /v1/tenant/overview[?branch=<uuid>].
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	var branch *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("branch")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "branch", Code: "invalid", Message: "branch must be a uuid"}})
			return
		}
		branch = &id
	}
	out, err := h.svc.Overview(r.Context(), orgctx.Must(r.Context()).ID, branch)
	if errors.Is(err, usecase.ErrBranchNotFound) {
		response.ValidationError(w, r, []response.Detail{{Field: "branch", Code: "not_found", Message: "branch not found"}})
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load the overview")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Badges serves GET /v1/tenant/overview/badges.
func (h *Handler) Badges(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Badges(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		response.InternalErr(w, r, err, "could not load the badges")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
