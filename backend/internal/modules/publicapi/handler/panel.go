package handler

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Panel serves /v1/tenant/public-api/* (behind RequireOrganization).
type Panel struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// NewPanel builds the panel handler. rec may be nil.
func NewPanel(svc *usecase.Service, rec *activity.Recorder) *Panel {
	return &Panel{svc: svc, activity: rec}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func (h *Panel) record(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func page[T any](w http.ResponseWriter, r *http.Request, items []T, total int64, q apiquery.Query, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func sortOf(q apiquery.Query) (string, bool) {
	if len(q.Sort) == 0 {
		return "", true
	}
	return q.Sort[0].Field, q.Sort[0].Desc
}

// Overview serves GET /v1/tenant/public-api/overview.
func (h *Panel) Overview(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Overview(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListKeys serves GET /v1/tenant/public-api/keys.
func (h *Panel) ListKeys(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, usecase.KeySortColumns)
	if !ok {
		return
	}
	field, desc := sortOf(q)
	items, total, err := h.svc.ListKeys(r.Context(), orgID(r), model.KeyFilter{
		Q: q.Q, Kinds: httpkit.Values(r, "kind"), Channels: httpkit.Values(r, "channel"), Statuses: httpkit.Values(r, "status"),
		SortField: field, SortDesc: desc, Limit: q.Limit, Offset: q.Offset,
	})
	page(w, r, items, total, q, err)
}

// CreateKey serves POST /v1/tenant/public-api/keys.
func (h *Panel) CreateKey(w http.ResponseWriter, r *http.Request) {
	var in model.KeyInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.CreateKey(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.key_created", model.ResourceKeys, out.Key.UUID, map[string]any{
		"kind": out.Key.Kind, "channel": out.Key.Channel, "name": out.Key.Name, "prefix": out.Key.KeyPrefix,
	})
	response.JSON(w, r, http.StatusCreated, out)
}

// GetKey serves GET /v1/tenant/public-api/keys/{uuid}.
func (h *Panel) GetKey(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetKey(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PatchKey serves PATCH /v1/tenant/public-api/keys/{uuid}.
func (h *Panel) PatchKey(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.KeyInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.PatchKey(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.key_updated", model.ResourceKeys, id, map[string]any{
		"scopes": out.Scopes, "allowed_origins": out.AllowedOrigins, "rate_limit_per_minute": out.RateLimitPerMinute,
	})
	response.JSON(w, r, http.StatusOK, out)
}

// RotateKey serves POST /v1/tenant/public-api/keys/{uuid}/rotate.
func (h *Panel) RotateKey(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.RotateInput
	if !httpkit.DecodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.RotateKey(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.key_rotated", model.ResourceKeys, id, map[string]any{
		"successor_uuid": out.Key.UUID, "grace_hours": in.GraceHours,
	})
	response.JSON(w, r, http.StatusCreated, out)
}

// RevokeKey serves POST /v1/tenant/public-api/keys/{uuid}/revoke.
func (h *Panel) RevokeKey(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.RevokeKey(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.key_revoked", model.ResourceKeys, id, map[string]any{"prefix": out.KeyPrefix})
	response.JSON(w, r, http.StatusOK, out)
}

// ListEndpoints serves GET /v1/tenant/public-api/webhooks.
func (h *Panel) ListEndpoints(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, usecase.EndpointSortColumns)
	if !ok {
		return
	}
	field, desc := sortOf(q)
	items, total, err := h.svc.ListEndpoints(r.Context(), orgID(r), model.EndpointFilter{
		Q: q.Q, Statuses: httpkit.Values(r, "status"), SortField: field, SortDesc: desc, Limit: q.Limit, Offset: q.Offset,
	})
	page(w, r, items, total, q, err)
}

// CreateEndpoint serves POST /v1/tenant/public-api/webhooks.
func (h *Panel) CreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var in model.EndpointInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.CreateEndpoint(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.webhook_created", model.ResourceWebhooks, out.Endpoint.UUID, map[string]any{
		"url": out.Endpoint.URL, "events": out.Endpoint.Events,
	})
	response.JSON(w, r, http.StatusCreated, out)
}

// GetEndpoint serves GET /v1/tenant/public-api/webhooks/{uuid}.
func (h *Panel) GetEndpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetEndpoint(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PatchEndpoint serves PATCH /v1/tenant/public-api/webhooks/{uuid}.
func (h *Panel) PatchEndpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.EndpointInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.PatchEndpoint(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.webhook_updated", model.ResourceWebhooks, id, map[string]any{
		"url": out.URL, "events": out.Events, "status": out.Status,
	})
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteEndpoint serves DELETE /v1/tenant/public-api/webhooks/{uuid}.
func (h *Panel) DeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteEndpoint(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.webhook_deleted", model.ResourceWebhooks, id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// RotateEndpointSecret serves POST /v1/tenant/public-api/webhooks/{uuid}/rotate-secret.
func (h *Panel) RotateEndpointSecret(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.RotateEndpointSecret(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.webhook_secret_rotated", model.ResourceWebhooks, id, nil)
	response.JSON(w, r, http.StatusOK, out)
}

// TestEndpoint serves POST /v1/tenant/public-api/webhooks/{uuid}/test.
func (h *Panel) TestEndpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.TestEndpoint(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListDeliveries serves GET /v1/tenant/public-api/webhook-deliveries.
func (h *Panel) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, map[string]string{"created_at": "created_at"})
	if !ok {
		return
	}
	desc := true
	if len(q.Sort) > 0 {
		desc = q.Sort[0].Desc
	}
	f := model.DeliveryFilter{
		Statuses: httpkit.Values(r, "status"), EventTypes: httpkit.Values(r, "event_type"), SortDesc: desc,
		Limit: q.Limit, Offset: q.Offset,
	}
	for _, v := range httpkit.Values(r, "endpoint") {
		id, err := uuid.Parse(v)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "endpoint", Code: "invalid", Message: "endpoint is a uuid"}})
			return
		}
		f.EndpointUUIDs = append(f.EndpointUUIDs, id)
	}
	items, total, err := h.svc.ListDeliveries(r.Context(), orgID(r), f)
	page(w, r, items, total, q, err)
}

// GetDelivery serves GET /v1/tenant/public-api/webhook-deliveries/{uuid}.
func (h *Panel) GetDelivery(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetDelivery(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RetryDelivery serves POST /v1/tenant/public-api/webhook-deliveries/{uuid}/retry.
func (h *Panel) RetryDelivery(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.RetryDelivery(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "public_api.webhook_retried", model.ResourceWebhooks, out.Endpoint.UUID, map[string]any{"delivery_uuid": id})
	response.JSON(w, r, http.StatusOK, out)
}
