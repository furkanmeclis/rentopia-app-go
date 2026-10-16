// Package handler is the HTTP layer of the notification engine (TEC-733):
// rules, templates, test sends and the delivery history under
// /v1/tenant/notification-*, and the customer portal's web push
// subscription. The organization comes from orgctx (staff) or the portal
// session (customers), never from the request.
package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/model"
	engine "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves the engine routes.
type Handler struct {
	svc            *engine.Engine
	vapidPublicKey string
}

// New builds the handler. vapidPublicKey is the web push public key
// handed to the customer portal (empty: customer push off).
func New(svc *engine.Engine, vapidPublicKey string) *Handler {
	return &Handler{svc: svc, vapidPublicKey: strings.TrimSpace(vapidPublicKey)}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

var sortColumns = func() apiquery.SortColumns {
	m := apiquery.SortColumns{}
	for _, f := range engine.Sortable {
		m[f] = f
	}
	return m
}()

type eventsView struct {
	Channels []model.ChannelInfo `json:"channels"`
	Events   []model.EventInfo   `json:"events"`
}

// Events serves GET /v1/tenant/notification-events.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	events, err := h.svc.ListEvents(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, eventsView{Channels: h.svc.Channels(), Events: events})
}

// PutRule serves PUT /v1/tenant/notification-events/{event}/rule.
func (h *Handler) PutRule(w http.ResponseWriter, r *http.Request) {
	var in model.RuleInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.PutRule(r.Context(), orgID(r), r.PathValue("event"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ResetRule serves DELETE /v1/tenant/notification-events/{event}/rule.
func (h *Handler) ResetRule(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ResetRule(r.Context(), orgID(r), r.PathValue("event"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Templates serves GET /v1/tenant/notification-templates.
func (h *Handler) Templates(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	out, err := h.svc.ListTemplates(r.Context(), orgID(r), v.Get("event"), v.Get("channel"), v.Get("language"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutTemplate serves PUT /v1/tenant/notification-templates/{event}/{channel}/{language}.
func (h *Handler) PutTemplate(w http.ResponseWriter, r *http.Request) {
	var in model.TemplateInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.PutTemplate(r.Context(), orgID(r), r.PathValue("event"), r.PathValue("channel"), r.PathValue("language"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ResetTemplate serves DELETE /v1/tenant/notification-templates/{event}/{channel}/{language}.
func (h *Handler) ResetTemplate(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ResetTemplate(r.Context(), orgID(r), r.PathValue("event"), r.PathValue("channel"), r.PathValue("language"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Preview serves POST /v1/tenant/notification-templates/preview.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var in model.PreviewInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.Preview(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Test serves POST /v1/tenant/notification-tests.
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	var in model.TestInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.Test(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// DeliveriesMeta serves GET /v1/tenant/notification-deliveries/meta.
func (h *Handler) DeliveriesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, DeliveriesMeta())
}

// DeliveryFacets serves GET /v1/tenant/notification-deliveries/facets.
func (h *Handler) DeliveryFacets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
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

// Deliveries serves GET /v1/tenant/notification-deliveries.
func (h *Handler) Deliveries(w http.ResponseWriter, r *http.Request) {
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
	f := model.DeliveryFilter{
		Channels: csv(values.Get("channel")), Statuses: csv(values.Get("status")), Events: csv(values.Get("event")),
		Audiences: csv(values.Get("audience")), Q: q.Q, Limit: q.Limit, Offset: q.Offset,
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
	items, total, err := h.svc.ListDeliveries(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Dispatch serves GET /v1/tenant/notification-dispatches/{uuid}.
func (h *Handler) Dispatch(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetDispatch(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CustomerVAPIDKey serves GET /v1/customer/{orgSlug}/push/vapid-public-key.
func (h *Handler) CustomerVAPIDKey(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]string{"vapid_public_key": h.vapidPublicKey})
}

type endpointInput struct {
	Endpoint string `json:"endpoint"`
}

// CustomerSubscribe serves POST /v1/customer/{orgSlug}/push-subscriptions.
func (h *Handler) CustomerSubscribe(w http.ResponseWriter, r *http.Request) {
	p, ok := usecase.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Sign in to continue")
		return
	}
	if h.vapidPublicKey == "" {
		response.ServiceUnavailable(w, r, "PUSH_NOT_CONFIGURED", "Web push is not configured")
		return
	}
	var in model.PushSubscriptionInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	if err := h.svc.SubscribeCustomerPush(r.Context(), p.Org.ID, p.CustomerID, in); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"subscribed": true})
}

// CustomerUnsubscribe serves DELETE /v1/customer/{orgSlug}/push-subscriptions.
func (h *Handler) CustomerUnsubscribe(w http.ResponseWriter, r *http.Request) {
	p, ok := usecase.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Sign in to continue")
		return
	}
	var in endpointInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	if err := h.svc.UnsubscribeCustomerPush(r.Context(), p.Org.ID, p.CustomerID, in.Endpoint); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"subscribed": false})
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
	var ve *engine.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	switch {
	case errors.Is(err, engine.ErrNotFound), errors.Is(err, engine.ErrUnknownEvent):
		response.NotFound(w, r, "Not found")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
