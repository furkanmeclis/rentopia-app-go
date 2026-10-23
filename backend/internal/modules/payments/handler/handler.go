// Package handler is the HTTP layer of payments (TEC-707): the tenant
// routes (/v1/tenant/payments, /payment-links, reservation links and branch
// collections) behind RequireOrganization, and the public routes (pay page,
// checkout start, provider webhook) that resolve the organization from the
// link token / the verified notification. Path uuids are looked up before
// bodies are validated (another organization's uuid answers 404).
package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Response codes.
const (
	CodeInvalidState    = "INVALID_STATE"
	CodeAccountRequired = "PAYMENT_ACCOUNT_REQUIRED"
	CodeProviderError   = "PAYMENT_PROVIDER_ERROR"
)

// Handler serves payments.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	limiter  RateLimiter
}

// RateLimiter is the subset of ratelimit.Limiter the public routes use.
type RateLimiter interface {
	Allow(r *http.Request, action, subject string, limit int, window time.Duration) bool
}

// New builds the handler (rec may be nil).
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetRateLimiter wires the public route limits (nil = unlimited).
func (h *Handler) SetRateLimiter(l RateLimiter) { h.limiter = l }

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func (h *Handler) audit(r *http.Request, resource, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int, window time.Duration) bool {
	if h.limiter == nil || h.limiter.Allow(r, action, subject, limit, window) {
		return true
	}
	response.TooManyRequests(w, r, "Too many requests; try again shortly")
	return false
}

// ------------------------------------------------------------------ tenant

// List serves GET /v1/tenant/payments.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, usecase.Sortable)
	if !ok {
		return
	}
	f := paymodel.ListFilter{
		Q: q.Q, Statuses: httpkit.Values(r, "status"), Providers: httpkit.Values(r, "provider"),
		ReservationUUIDs: uuids(httpkit.Values(r, "reservation")), Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func uuids(values []string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range values {
		if id, err := uuid.Parse(v); err == nil {
			out = append(out, id)
		} else {
			out = append(out, uuid.Nil) // an invalid id matches nothing
		}
	}
	return out
}

// Get serves GET /v1/tenant/payments/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// Verify serves POST /v1/tenant/payments/{uuid}/verify.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := h.svc.Verify(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, paymodel.ResourcePayments, "payments.verified", id, map[string]any{"status": p.Status})
	response.JSON(w, r, http.StatusOK, p)
}

// Refund serves POST /v1/tenant/payments/{uuid}/refund.
func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if _, err := h.svc.Get(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	var in paymodel.RefundInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	p, err := h.svc.Refund(r.Context(), orgID(r), id, in, httpkit.ClientIP(r))
	var ge *usecase.RefundGuardError
	if errors.As(err, &ge) && ge.Code == usecase.CodeRefundStateMismatch {
		h.audit(r, paymodel.ResourcePayments, "payments.refund_refused", id, map[string]any{
			"code": ge.Code, "our_refunded": ge.OurRefunded.String(), "provider_refunded": ge.ProviderRefunded.String(),
		})
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"status": p.Status, "refunded_amount": p.RefundedAmount.String(), "reason": strings.TrimSpace(in.Reason)}
	h.audit(r, paymodel.ResourcePayments, "payments.refunded", id, payload)
	response.JSON(w, r, http.StatusOK, p)
}

// ListLinks serves GET /v1/tenant/payment-links.
func (h *Handler) ListLinks(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, map[string]string{"created_at": "created_at"})
	if !ok {
		return
	}
	items, total, err := h.svc.ListLinks(r.Context(), orgID(r), paymodel.LinkFilter{
		ReservationUUIDs: uuids(httpkit.Values(r, "reservation")), Statuses: httpkit.Values(r, "status"),
		Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetLink serves GET /v1/tenant/payment-links/{uuid}.
func (h *Handler) GetLink(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	l, err := h.svc.GetLink(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, l)
}

// CancelLink serves POST /v1/tenant/payment-links/{uuid}/cancel.
func (h *Handler) CancelLink(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	l, err := h.svc.CancelLink(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, paymodel.ResourceLinks, "payment_links.cancelled", id, map[string]any{"reservation": l.Reservation.Reference})
	response.JSON(w, r, http.StatusOK, l)
}

// CreateLink serves POST /v1/tenant/reservations/{uuid}/payment-links.
func (h *Handler) CreateLink(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in paymodel.CreateLinkInput
	if !httpkit.DecodeJSON(w, r, &in, true) {
		return
	}
	l, err := h.svc.CreateLink(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, paymodel.ResourceLinks, "payment_links.created", l.UUID, map[string]any{
		"reservation": l.Reservation.Reference, "amount": l.Amount.String(), "expires_at": l.ExpiresAt,
	})
	response.JSON(w, r, http.StatusCreated, l)
}

// Collect serves POST /v1/tenant/reservations/{uuid}/collections.
func (h *Handler) Collect(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in paymodel.CollectInput
	if !httpkit.DecodeJSON(w, r, &in, true) {
		return
	}
	p, err := h.svc.Collect(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, paymodel.ResourcePayments, "payments.collected", p.UUID, map[string]any{
		"reservation": p.Reservation.Reference, "amount": p.Amount.String(), "method": p.Method,
	})
	response.JSON(w, r, http.StatusCreated, p)
}

// ------------------------------------------------------------------ public

// PublicLink serves GET /v1/public/pay/{token} (?verify=1 asks the provider
// about a pending checkout: the page poll).
func (h *Handler) PublicLink(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !h.allow(w, r, "pay_page", httpkit.ClientIP(r), 120, time.Minute) {
		return
	}
	verify := r.URL.Query().Get("verify") == "1" && (h.limiter == nil || h.limiter.Allow(r, "pay_verify", token, 30, time.Minute))
	out, err := h.svc.PublicLink(r.Context(), token, verify)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// StartCheckout serves POST /v1/public/pay/{token}/checkout.
func (h *Handler) StartCheckout(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !h.allow(w, r, "pay_checkout_ip", httpkit.ClientIP(r), 20, 10*time.Minute) ||
		!h.allow(w, r, "pay_checkout", token, 10, 10*time.Minute) {
		return
	}
	var in struct {
		Locale string `json:"locale"`
	}
	if !httpkit.DecodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.StartCheckout(r.Context(), token, httpkit.ClientIP(r), in.Locale)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// Webhook serves POST /v1/public/payments/{provider}/webhook (no auth:
// the signature is verified with the organization's secret).
func (h *Handler) Webhook(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.allow(w, r, "payment_webhook", httpkit.ClientIP(r), 300, time.Minute) {
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
			return
		}
		out, err := h.svc.Webhook(r.Context(), provider, r.Header, body)
		switch {
		case errors.Is(err, usecase.ErrBadWebhook):
			response.BadRequest(w, r, response.CodeValidationError, "Malformed notification")
		case errors.Is(err, usecase.ErrInvalidSignature):
			response.Unauthorized(w, r, "Invalid signature")
		case errors.Is(err, usecase.ErrUnknownPayment), errors.Is(err, usecase.ErrUnknownProvider):
			response.NotFound(w, r, "Unknown payment")
		case err != nil:
			writeError(w, r, err)
		default:
			response.JSON(w, r, http.StatusOK, out)
		}
	}
}

// ListOrphans serves GET /v1/tenant/payments/orphans: provider
// notifications that named no payment of ours (TEC-962).
func (h *Handler) ListOrphans(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, map[string]string{"created_at": "created_at"})
	if !ok {
		return
	}
	f := paymodel.OrphanFilter{Q: q.Q, Statuses: httpkit.Values(r, "status"), SortDesc: true, Limit: q.Limit, Offset: q.Offset}
	if len(q.Sort) > 0 {
		f.SortDesc = q.Sort[0].Desc
	}
	if v := strings.TrimSpace(r.URL.Query().Get("since")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "since", Code: "invalid", Message: "since must be an RFC 3339 time"}})
			return
		}
		f.Since = &t
	}
	items, total, err := h.svc.ListOrphans(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// MatchOrphan serves POST /v1/tenant/payments/orphans/{uuid}/match.
func (h *Handler) MatchOrphan(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in paymodel.MatchOrphanInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	p, err := h.svc.MatchOrphan(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, paymodel.ResourceOrphans, "payments.orphan_matched", id, map[string]any{
		"payment_uuid": p.UUID, "reservation_uuid": p.Reservation.UUID, "note": strings.TrimSpace(in.Note),
	})
	response.JSON(w, r, http.StatusOK, p)
}

// Reconciliation serves GET /v1/tenant/payments/reconciliation?since=.
func (h *Handler) Reconciliation(w http.ResponseWriter, r *http.Request) {
	v := strings.TrimSpace(r.URL.Query().Get("since"))
	since, err := time.Parse(time.RFC3339, v)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "since", Code: "required", Message: "since (RFC 3339) is required"}})
		return
	}
	out, err := h.svc.Reconcile(r.Context(), orgID(r), since)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ResolveItem serves POST
// /v1/tenant/payments/reconciliation-items/{uuid}/resolve.
func (h *Handler) ResolveItem(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in paymodel.ResolveItemInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	p, err := h.svc.ResolveItem(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, paymodel.ResourcePayments, "payments.reconciliation_item_resolved", id, map[string]any{
		"payment_uuid": p.UUID, "resolution": in.Resolution, "refunded_amount": p.RefundedAmount.String(), "note": strings.TrimSpace(in.Note),
	})
	response.JSON(w, r, http.StatusOK, p)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ge *usecase.RefundGuardError
	if errors.As(err, &ge) {
		data := map[string]any{"our_refunded": ge.OurRefunded, "provider_refunded": ge.ProviderRefunded, "reconciliation_item_uuid": ge.ItemUUID}
		response.ErrorWithData(w, r, http.StatusConflict, ge.Code, ge.Message,
			[]response.Detail{{Code: strings.ToLower(ge.Code), Message: ge.Message}}, data)
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, out)
		return
	}
	var se *usecase.StateError
	if errors.As(err, &se) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}})
		return
	}
	var pf *usecase.ProviderFailure
	var pe *payments.ProviderError
	if errors.As(err, &pf) || errors.As(err, &pe) {
		code, msg := "provider_error", err.Error()
		if errors.As(err, &pe) && pe.Code != "" {
			code = pe.Code
			msg = pe.Message
		}
		response.ErrorWithDetails(w, r, http.StatusBadGateway, CodeProviderError, "The payment provider refused the request",
			[]response.Detail{{Code: code, Message: msg}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Payment was not found")
	case errors.Is(err, usecase.ErrOrphanNotFound):
		response.NotFound(w, r, "Orphan notification was not found")
	case errors.Is(err, usecase.ErrItemNotFound):
		response.NotFound(w, r, "Reconciliation item was not found")
	case errors.Is(err, usecase.ErrLinkNotFound):
		response.NotFound(w, r, "Payment link was not found")
	case errors.Is(err, usecase.ErrReservationNotFound):
		response.NotFound(w, r, "Reservation was not found")
	case errors.Is(err, usecase.ErrNoAccount):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeAccountRequired, err.Error(),
			[]response.Detail{{Code: "payment_account_required", Message: err.Error()}})
	case errors.Is(err, usecase.ErrMismatch):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, err.Error(),
			[]response.Detail{{Code: "verification_mismatch", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
