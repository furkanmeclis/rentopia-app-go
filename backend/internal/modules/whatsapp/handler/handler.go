// Package handler is the HTTP layer of the WhatsApp channel (TEC-735):
// the organization's session (connect, QR, state, logout, remove,
// settings), test sends and the message log under /v1/tenant/whatsapp, and
// the provider webhook under /v1/public/whatsapp/webhook/{token}. The
// organization comes from orgctx (staff) or the webhook token, never from
// the request body.
package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Error codes.
const (
	CodeNotConnected        = "WHATSAPP_NOT_CONNECTED"
	CodeNotSetUp            = "WHATSAPP_NOT_SET_UP"
	CodeProviderUnavailable = "WHATSAPP_PROVIDER_UNAVAILABLE"
	CodeProviderError       = "WHATSAPP_PROVIDER_ERROR"
	CodeRateLimited         = "WHATSAPP_RATE_LIMITED"
	CodeNotOnWhatsApp       = "WHATSAPP_NOT_ON_WHATSAPP"
)

// Handler serves the WhatsApp routes.
type Handler struct {
	svc *usecase.Service
}

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

var sortColumns = func() apiquery.SortColumns {
	m := apiquery.SortColumns{}
	for _, f := range usecase.Sortable {
		m[f] = f
	}
	return m
}()

// Session serves GET /v1/tenant/whatsapp/session.
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Session(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateSettings serves PATCH /v1/tenant/whatsapp/session.
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in model.SettingsInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.UpdateSettings(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Remove serves DELETE /v1/tenant/whatsapp/session.
func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Remove(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Connect serves POST /v1/tenant/whatsapp/session/connect.
func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	var in model.ConnectInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.Connect(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// QR serves GET /v1/tenant/whatsapp/session/qr.
func (h *Handler) QR(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.QR(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// Refresh serves POST /v1/tenant/whatsapp/session/refresh.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Refresh(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Logout serves POST /v1/tenant/whatsapp/session/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Logout(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// TestSend serves POST /v1/tenant/whatsapp/test-messages.
func (h *Handler) TestSend(w http.ResponseWriter, r *http.Request) {
	var in model.TestInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.TestSend(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func csv(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Messages serves GET /v1/tenant/whatsapp/messages.
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, sortColumns); err != nil {
		writeError(w, r, err)
		return
	}
	f := model.MessageFilter{
		Statuses: csv(values.Get("status")), Kinds: csv(values.Get("kind")), Q: q.Q, Limit: q.Limit, Offset: q.Offset,
	}
	var details []response.Detail
	for _, key := range []string{"created_from", "created_to"} {
		raw := strings.TrimSpace(values.Get(key))
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, response.Detail{Field: key, Code: "invalid", Message: key + " must be an RFC 3339 time"})
			continue
		}
		if key == "created_from" {
			f.CreatedFrom = &t
		} else {
			f.CreatedTo = &t
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListMessages(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Webhook serves POST /v1/public/whatsapp/webhook/{token}: the provider's
// event delivery. The secret token names the session; unknown tokens
// answer 404 without detail.
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "Could not read the body")
		return
	}
	if err := h.svc.HandleWebhook(r.Context(), r.PathValue("token"), r.Header.Get("Content-Type"), body); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"received": true})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		out := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var pe *usecase.ProviderError
	if errors.As(err, &pe) {
		response.Error(w, r, http.StatusBadGateway, CodeProviderError, pe.Message)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrWebhookUnknown):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrNoAccount):
		response.Conflict(w, r, CodeNotSetUp, err.Error())
	case errors.Is(err, usecase.ErrNotConnected):
		response.Conflict(w, r, CodeNotConnected, err.Error())
	case errors.Is(err, usecase.ErrProviderUnavailable):
		response.ServiceUnavailable(w, r, CodeProviderUnavailable, err.Error())
	case errors.Is(err, usecase.ErrRateLimited):
		response.Error(w, r, http.StatusTooManyRequests, CodeRateLimited, err.Error())
	case errors.Is(err, usecase.ErrNotOnWhatsApp):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeNotOnWhatsApp, err.Error())
	case errors.Is(err, usecase.ErrInvalidPhone):
		response.ValidationError(w, r, []response.Detail{{Field: "phone", Code: "invalid", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
