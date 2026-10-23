// Package handler serves /v1/tenant/support-access and
// /v1/platform/support-grants. The organization comes from orgctx.Scope.
package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves the support access routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// List serves GET /v1/tenant/support-access (query: limit, offset,
// status: comma-separated active, expired, revoked).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, map[string]string{})
	if !ok {
		return
	}
	items, total, err := h.svc.List(r.Context(), orgctx.Must(r.Context()).ID, model.ListFilter{
		Statuses: httpkit.Values(r, "status"), Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Open serves POST /v1/tenant/support-access.
func (h *Handler) Open(w http.ResponseWriter, r *http.Request) {
	p, ok := memberOnly(w, r)
	if !ok {
		return
	}
	var in model.OpenInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	g, err := h.svc.Open(r.Context(), orgctx.Must(r.Context()).ID, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, p, "support_access.opened", g, map[string]any{
		"scope": g.Scope, "expires_at": g.ExpiresAt, "duration_hours": in.DurationHours, "note": g.Note,
	})
	response.JSON(w, r, http.StatusCreated, g)
}

// Revoke serves POST /v1/tenant/support-access/{uuid}/revoke.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, ok := memberOnly(w, r)
	if !ok {
		return
	}
	g, err := h.svc.Revoke(r.Context(), orgctx.Must(r.Context()).ID, p.UserInternal, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, p, "support_access.revoked", g, map[string]any{"scope": g.Scope, "expires_at": g.ExpiresAt})
	response.JSON(w, r, http.StatusOK, g)
}

// PlatformActive serves GET /v1/platform/support-grants: active grants of
// every organization, soonest expiry first.
func (h *Handler) PlatformActive(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListActivePlatform(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, int64(len(items)), int32(len(items)), 0))
}

// memberOnly refuses impersonated sessions: only the organization's own
// members open or revoke support access.
func memberOnly(w http.ResponseWriter, r *http.Request) (authctx.Principal, bool) {
	p := authctx.MustPrincipal(r.Context())
	if p.ImpersonatorUserID != nil {
		response.Forbidden(w, r, "Support sessions cannot change support access")
		return p, false
	}
	return p, true
}

func (h *Handler) record(r *http.Request, p authctx.Principal, action string, g model.Grant, payload map[string]any) {
	if h.activity == nil {
		return
	}
	id := g.UUID
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &id, payload, r)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *apiquery.ValidationError
	switch {
	case errors.As(err, &ve):
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Support grant was not found or is no longer active")
	default:
		response.InternalErr(w, r, err, "")
	}
}
