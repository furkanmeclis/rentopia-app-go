// Package handler is the HTTP layer of the customer portal (TEC-715):
// /v1/customer/{orgSlug}/* for customers (own session middleware, never
// the staff JWT) and /v1/tenant/customer-requests for staff.
package handler

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/usecase"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// SessionHeader carries the customer session token from the BFF (which
// keeps it in an HttpOnly cookie).
const SessionHeader = "X-Customer-Session"

// Response codes.
const (
	CodeSessionRequired = "CUSTOMER_SESSION_REQUIRED"
	CodeOTPInvalid      = "OTP_INVALID"
	CodeCodeUnavailable = "CODE_UNAVAILABLE"
	CodeInvalidState    = "INVALID_STATE"
	CodePaymentUnavail  = "PAYMENT_UNAVAILABLE"
	CodeNothingDue      = "NOTHING_DUE"
	CodeRequestExists   = "REQUEST_EXISTS"
	CodePDFNotReady     = "PDF_NOT_READY"
	CodeRateLimited     = "RATE_LIMITED"
)

// RateLimiter is the per-IP / per-reference / per-contact window limiter
// (ratelimit.Limiter adapter; nil = unlimited).
type RateLimiter interface {
	Allow(r *http.Request, action, subject string, limit int, window time.Duration) bool
}

type limit struct {
	action string
	n      int
	window time.Duration
}

// Limits of the portal routes (Redis windows, fail open like the other
// public routes; the OTP service's DB counters hold without Redis).
var (
	limitInfoIP          = limit{"cp_info_ip", 60, time.Minute}
	limitStartIP         = limit{"cp_start_ip", 10, 10 * time.Minute}
	limitStartReference  = limit{"cp_start_ref", 5, 10 * time.Minute}
	limitStartContact    = limit{"cp_start_contact", 5, 10 * time.Minute}
	limitVerifyIP        = limit{"cp_verify_ip", 30, 10 * time.Minute}
	limitVerifyReference = limit{"cp_verify_ref", 10, 10 * time.Minute}
	limitVerifyContact   = limit{"cp_verify_contact", 10, 10 * time.Minute}
	limitSession         = limit{"cp_session", 240, time.Minute}
	limitPay             = limit{"cp_pay", 10, 10 * time.Minute}
	limitRequest         = limit{"cp_request", 10, time.Hour}
	limitPDF             = limit{"cp_pdf", 30, 10 * time.Minute}
)

// Handler serves the portal.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	limiter  RateLimiter
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetRateLimiter wires the limits.
func (h *Handler) SetRateLimiter(l RateLimiter) { h.limiter = l }

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, l limit, subject string) bool {
	if h.limiter == nil || subject == "" || h.limiter.Allow(r, l.action, subject, l.n, l.window) {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(l.window.Seconds())))
	response.ErrorWithDetails(w, r, http.StatusTooManyRequests, CodeRateLimited, "Too many attempts; try again later",
		[]response.Detail{{Code: "rate_limited", Message: "Too many attempts; try again later"}})
	return false
}

func client(r *http.Request) model.ClientInfo {
	return model.ClientInfo{IP: httpkit.ClientIP(r), UserAgent: r.UserAgent()}
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// RequireSession is the customer session middleware of
// /v1/customer/{orgSlug}/*: the bearer token must name a live session of
// the organization in the path. It never reads the staff Authorization
// header.
func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.svc.Authenticate(r.Context(), r.PathValue("orgSlug"), r.Header.Get(SessionHeader))
		if err != nil {
			if !errors.Is(err, usecase.ErrSessionInvalid) {
				response.InternalErr(w, r, err, "Unexpected server error")
				return
			}
			noStore(w)
			response.Error(w, r, http.StatusUnauthorized, CodeSessionRequired, "Sign in to continue")
			return
		}
		if !h.allow(w, r, limitSession, p.SessionUUID.String()) {
			return
		}
		noStore(w)
		next.ServeHTTP(w, r.WithContext(usecase.WithPrincipal(r.Context(), p)))
	})
}

func principal(r *http.Request) usecase.Principal {
	p, _ := usecase.PrincipalFrom(r.Context())
	return p
}

// keyed returns a rate-limit subject inside an organization slug.
func keyed(slug, v string) string {
	if v == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(slug)) + "|" + v
}

// Info serves GET /v1/customer/{orgSlug}.
func (h *Handler) Info(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, limitInfoIP, httpkit.ClientIP(r)) {
		return
	}
	out, err := h.svc.Info(r.Context(), r.PathValue("orgSlug"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	noStore(w)
	response.JSON(w, r, http.StatusOK, out)
}

// limitSignIn applies the per-IP, per-reference and per-contact windows.
func (h *Handler) limitSignIn(w http.ResponseWriter, r *http.Request, ip, ref, contact limit, reference, rawContact string) bool {
	slug := r.PathValue("orgSlug")
	if !h.allow(w, r, ip, httpkit.ClientIP(r)) {
		return false
	}
	refKey, contactKey := h.svc.RateKeys(r.Context(), slug, reference, rawContact)
	return h.allow(w, r, ref, keyed(slug, refKey)) && h.allow(w, r, contact, keyed(slug, contactKey))
}

// Start serves POST /v1/customer/{orgSlug}/auth/start.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	var in model.StartInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	if !h.limitSignIn(w, r, limitStartIP, limitStartReference, limitStartContact, in.Reference, in.Contact) {
		return
	}
	out, err := h.svc.Start(r.Context(), r.PathValue("orgSlug"), in, client(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	noStore(w)
	response.JSON(w, r, http.StatusAccepted, out)
}

// Verify serves POST /v1/customer/{orgSlug}/auth/verify. The answer holds
// session_token; the BFF moves it into the HttpOnly cookie.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	var in model.VerifyInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	if !h.limitSignIn(w, r, limitVerifyIP, limitVerifyReference, limitVerifyContact, in.Reference, in.Contact) {
		return
	}
	out, err := h.svc.Verify(r.Context(), r.PathValue("orgSlug"), in, client(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	noStore(w)
	response.JSON(w, r, http.StatusOK, out)
}

// Session serves GET /v1/customer/{orgSlug}/session.
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, principal(r).View())
}

// Logout serves POST /v1/customer/{orgSlug}/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context(), principal(r), client(r)); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"signed_out": true})
}

// Reservations serves GET /v1/customer/{orgSlug}/reservations.
func (h *Handler) Reservations(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Reservations(r.Context(), principal(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		// Unknown and malformed ids answer the same.
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

// Reservation serves GET /v1/customer/{orgSlug}/reservations/{uuid}.
func (h *Handler) Reservation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Reservation(r.Context(), principal(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DocumentPDF serves GET …/reservations/{uuid}/documents/{documentUuid}/pdf.
func (h *Handler) DocumentPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	doc, ok := pathID(w, r, "documentUuid")
	if !ok {
		return
	}
	p := principal(r)
	if !h.allow(w, r, limitPDF, p.SessionUUID.String()) {
		return
	}
	f, err := h.svc.DocumentPDF(r.Context(), p, id, doc, client(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// Pay serves POST /v1/customer/{orgSlug}/reservations/{uuid}/pay.
func (h *Handler) Pay(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct{}
	if !httpkit.DecodeJSON(w, r, &in, true) {
		return
	}
	p := principal(r)
	if !h.allow(w, r, limitPay, p.SessionUUID.String()) {
		return
	}
	out, err := h.svc.Pay(r.Context(), p, id, client(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreateRequest serves POST /v1/customer/{orgSlug}/reservations/{uuid}/requests.
func (h *Handler) CreateRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CreateRequestInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	p := principal(r)
	if !h.allow(w, r, limitRequest, p.SessionUUID.String()) {
		return
	}
	out, err := h.svc.CreateRequest(r.Context(), p, id, in, client(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// ---------------------------------------------------------------- staff

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

// ListRequests serves GET /v1/tenant/customer-requests.
func (h *Handler) ListRequests(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, map[string]string{"created_at": "created_at"})
	if !ok {
		return
	}
	f := model.RequestFilter{
		Q: q.Q, Statuses: httpkit.Values(r, "status"), Types: httpkit.Values(r, "request_type"), Limit: q.Limit, Offset: q.Offset,
		SortDesc: true,
	}
	if len(q.Sort) > 0 {
		f.SortDesc = q.Sort[0].Desc
	}
	for _, v := range httpkit.Values(r, "reservation") {
		id, err := uuid.Parse(v)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "reservation", Code: "invalid", Message: "reservation is invalid"}})
			return
		}
		f.ReservationUUIDs = append(f.ReservationUUIDs, id)
	}
	items, total, err := h.svc.ListRequests(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "total": total, "limit": f.Limit, "offset": f.Offset})
}

// RequestStats serves GET /v1/tenant/customer-requests/stats.
func (h *Handler) RequestStats(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.RequestStats(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GetRequest serves GET /v1/tenant/customer-requests/{uuid}.
func (h *Handler) GetRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetRequest(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ResolveRequest serves POST /v1/tenant/customer-requests/{uuid}/resolve.
func (h *Handler) ResolveRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	// Path uuid first: another organization's request answers 404.
	if _, err := h.svc.GetRequest(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	var in model.ResolveInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.ResolveRequest(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		if p, ok := authctx.PrincipalFrom(r.Context()); ok {
			h.activity.Record(r.Context(), &p.UserInternal, "customer_requests."+in.Status, model.ResourceRequests, &id, map[string]any{
				"reservation_uuid": out.Reservation.UUID, "type": out.Type,
			}, r)
		}
	}
	response.JSON(w, r, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
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
	var rse *resusecase.StateError
	switch {
	case errors.As(err, &se):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}})
	case errors.As(err, &rse):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, rse.Message, []response.Detail{{Code: rse.Code, Message: rse.Message}})
	case errors.Is(err, usecase.ErrPortalNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, usecase.ErrRequestNotFound), errors.Is(err, resusecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrSessionInvalid):
		response.Error(w, r, http.StatusUnauthorized, CodeSessionRequired, "Sign in to continue")
	case errors.Is(err, usecase.ErrCodeInvalid):
		response.ErrorWithDetails(w, r, http.StatusBadRequest, CodeOTPInvalid, err.Error(),
			[]response.Detail{{Field: "code", Code: "invalid", Message: err.Error()}})
	case errors.Is(err, usecase.ErrChannelUnavailable):
		response.ErrorWithDetails(w, r, http.StatusServiceUnavailable, CodeCodeUnavailable, err.Error(),
			[]response.Detail{{Code: "code_unavailable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrPaymentUnavailable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodePaymentUnavail, err.Error(),
			[]response.Detail{{Code: "payment_unavailable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrNothingDue):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNothingDue, err.Error(),
			[]response.Detail{{Code: "nothing_due", Message: err.Error()}})
	case errors.Is(err, usecase.ErrRequestExists):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeRequestExists, err.Error(),
			[]response.Detail{{Code: "request_exists", Message: err.Error()}})
	case errors.Is(err, usecase.ErrPDFNotReady):
		w.Header().Set("Retry-After", "5")
		response.ErrorWithDetails(w, r, http.StatusConflict, CodePDFNotReady, err.Error(),
			[]response.Detail{{Code: "pdf_not_ready", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
