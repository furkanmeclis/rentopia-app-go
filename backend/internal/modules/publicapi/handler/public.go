// Package handler is the HTTP layer of the public booking API (TEC-728):
// /v1/public/* for sites, widgets and integrations (CORS, credentials,
// rate limits, idempotency) and /v1/tenant/public-api/* where staff manage
// keys and webhooks.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	portalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Request headers.
const (
	HeaderIdempotencyKey   = "Idempotency-Key"
	HeaderReservationToken = "X-Reservation-Token"
	// HeaderForwardedOrigin carries the browser's Origin through the BFF
	// (which never forwards Origin itself).
	HeaderForwardedOrigin = "X-Forwarded-Origin"
	HeaderReplayed        = "Idempotent-Replayed"
)

// maxBody bounds a public request body.
const maxBody = 64 << 10

// RateLimiter is the fixed-window limiter (ratelimit.Limiter); it fails
// open without Redis.
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Public serves /v1/public/*.
type Public struct {
	svc     *usecase.Service
	limiter RateLimiter
}

// NewPublic builds the public handler. limiter may be nil.
func NewPublic(svc *usecase.Service, limiter RateLimiter) *Public {
	return &Public{svc: svc, limiter: limiter}
}

type ctxKey struct{}

func credential(r *http.Request) model.Credential {
	c, _ := r.Context().Value(ctxKey{}).(model.Credential)
	return c
}

// requestOrigin is the browser origin of a request ("" for server calls).
func requestOrigin(r *http.Request) string {
	o := strings.TrimSpace(r.Header.Get("Origin"))
	if o == "" {
		o = strings.TrimSpace(r.Header.Get(HeaderForwardedOrigin))
	}
	if o == "" {
		return ""
	}
	if norm, ok := usecase.NormalizeOrigin(o); ok {
		return norm
	}
	// An opaque or malformed origin is still a browser: keep it so that
	// secret keys and sessions refuse it.
	return "invalid:" + o
}

// cors sets the CORS answer headers. Origins are checked against the
// credential after authentication; no cookies are involved, so the
// requesting origin is reflected.
func cors(w http.ResponseWriter, r *http.Request) {
	o := strings.TrimSpace(r.Header.Get("Origin"))
	if o == "" {
		o = strings.TrimSpace(r.Header.Get(HeaderForwardedOrigin))
	}
	h := w.Header()
	h.Add("Vary", "Origin")
	if o == "" {
		return
	}
	h.Set("Access-Control-Allow-Origin", o)
	h.Set("Access-Control-Expose-Headers", "Retry-After, Idempotent-Replayed, X-Request-Id, X-RateLimit-Limit")
}

// Preflight answers CORS preflight requests of /v1/public/*.
func (h *Public) Preflight(w http.ResponseWriter, r *http.Request) {
	cors(w, r)
	hd := w.Header()
	hd.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	hd.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Reservation-Token")
	hd.Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Public) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int, window time.Duration) bool {
	if h.limiter == nil || subject == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, subject, limit, window)
	if ok {
		return true
	}
	if retry <= 0 {
		retry = window
	}
	w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
	response.ErrorWithDetails(w, r, http.StatusTooManyRequests, CodeRateLimited, "Too many requests; try again later",
		[]response.Detail{{Code: "rate_limited", Message: "Too many requests; try again later"}})
	return false
}

// route options of a public endpoint.
type route struct {
	scope string
	// write: an Idempotency-Key is required and the answer is stored.
	write bool
	// limit / window: an extra per-caller limit of the route (0 = none).
	limit  int
	window time.Duration
	action string
}

// capture records the answer of a write for the idempotency store.
type capture struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (c *capture) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *capture) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}

func bearer(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return ""
}

// guard wraps a public endpoint: CORS, credential, rate limits, scope and
// (writes) idempotency.
func (h *Public) guard(opt route, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)
		w.Header().Set("Cache-Control", "no-store")
		ip := httpkit.ClientIP(r)
		if !h.allow(w, r, "pubapi:ip", ip, 600, time.Minute) {
			return
		}
		c, err := h.svc.Authenticate(r.Context(), bearer(r), requestOrigin(r), ip)
		if err != nil {
			writeError(w, r, err)
			return
		}
		subject := c.KeyUUID.String()
		if c.SessionID != "" {
			subject += ":" + c.SessionID
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(c.RateLimit))
		if !h.allow(w, r, "pubapi:cred", subject, c.RateLimit, time.Minute) {
			return
		}
		if opt.limit > 0 && !h.allow(w, r, "pubapi:"+opt.action, subject, opt.limit, opt.window) {
			return
		}
		if opt.scope != "" && !c.Has(opt.scope) {
			writeError(w, r, usecase.ErrScope)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, c))
		if !opt.write {
			fn(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
			return
		}
		stored, claim, err := h.svc.BeginIdempotent(r.Context(), c, strings.TrimSpace(r.Header.Get(HeaderIdempotencyKey)),
			r.Method, r.URL.Path, body)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if stored != nil {
			w.Header().Set(HeaderReplayed, "true")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(stored.Status)
			_, _ = w.Write(stored.Body)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		rec := &capture{ResponseWriter: w}
		defer func() {
			status := rec.status
			if p := recover(); p != nil {
				h.svc.FinishIdempotent(context.WithoutCancel(r.Context()), c, claim, http.StatusInternalServerError, nil)
				panic(p)
			}
			h.svc.FinishIdempotent(context.WithoutCancel(r.Context()), c, claim, status, rec.buf.Bytes())
		}()
		fn(rec, r)
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, "Invalid request body",
			[]response.Detail{{Code: "invalid_json", Message: err.Error()}})
		return false
	}
	return true
}

func clientInfo(r *http.Request) portalmodel.ClientInfo {
	return portalmodel.ClientInfo{IP: httpkit.ClientIP(r), UserAgent: r.UserAgent()}
}

// ------------------------------------------------------------- routes

// Session serves POST /v1/public/sessions: a publishable key used from an
// allowed origin gets a short-lived booking session token.
func (h *Public) Session(w http.ResponseWriter, r *http.Request) {
	cors(w, r)
	w.Header().Set("Cache-Control", "no-store")
	ip := httpkit.ClientIP(r)
	if !h.allow(w, r, "pubapi:session:ip", ip, 30, time.Minute) {
		return
	}
	var in model.SessionInput
	if !decode(w, r, &in) {
		return
	}
	key := strings.TrimSpace(in.PublishableKey)
	if key == "" {
		response.ValidationError(w, r, []response.Detail{{Field: "publishable_key", Code: "required", Message: "publishable_key is required"}})
		return
	}
	if !h.allow(w, r, "pubapi:session:key", usecase.TokenHash(key)[:32], 1200, time.Minute) {
		return
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		origin = strings.TrimSpace(r.Header.Get(HeaderForwardedOrigin))
	}
	s, _, err := h.svc.MintSession(r.Context(), key, origin, ip)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, s)
}

// Locations serves GET /v1/public/locations.
func (h *Public) Locations(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Locations(r.Context(), credential(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Subgroups serves GET /v1/public/vehicle-subgroups.
func (h *Public) Subgroups(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Subgroups(r.Context(), credential(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Extras serves GET /v1/public/extras.
func (h *Public) Extras(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Extras(r.Context(), credential(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func queryUUID(ve *[]response.Detail, r *http.Request, key string) *uuid.UUID {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		*ve = append(*ve, response.Detail{Field: key, Code: "invalid", Message: key + " is not a uuid"})
		return nil
	}
	return &id
}

func queryTime(ve *[]response.Detail, r *http.Request, key string) *time.Time {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		*ve = append(*ve, response.Detail{Field: key, Code: "invalid", Message: key + " is RFC 3339 (2026-07-01T10:00:00+03:00)"})
		return nil
	}
	return &t
}

// Availability serves GET /v1/public/availability.
func (h *Public) Availability(w http.ResponseWriter, r *http.Request) {
	ve := []response.Detail{}
	in := usecase.AvailabilityInput{
		PickupBranchUUID: queryUUID(&ve, r, "pickup_branch_uuid"), PickupPointUUID: queryUUID(&ve, r, "pickup_point_uuid"),
		ReturnBranchUUID: queryUUID(&ve, r, "return_branch_uuid"), ReturnPointUUID: queryUUID(&ve, r, "return_point_uuid"),
		StartsAt: queryTime(&ve, r, "starts_at"), EndsAt: queryTime(&ve, r, "ends_at"),
	}
	for i, v := range httpkit.Values(r, "subgroup_uuids") {
		id, err := uuid.Parse(v)
		if err != nil {
			ve = append(ve, response.Detail{Field: "subgroup_uuids." + strconv.Itoa(i), Code: "invalid", Message: "not a uuid"})
			continue
		}
		in.SubgroupUUIDs = append(in.SubgroupUUIDs, id)
	}
	if len(ve) > 0 {
		response.ValidationError(w, r, ve)
		return
	}
	out, err := h.svc.Availability(r.Context(), credential(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreateQuote serves POST /v1/public/quotes.
func (h *Public) CreateQuote(w http.ResponseWriter, r *http.Request) {
	var in model.QuoteInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CreateQuote(r.Context(), credential(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// GetQuote serves GET /v1/public/quotes/{uuid}.
func (h *Public) GetQuote(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetQuote(r.Context(), credential(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreateReservation serves POST /v1/public/reservations.
func (h *Public) CreateReservation(w http.ResponseWriter, r *http.Request) {
	var in model.ReservationInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CreateReservation(r.Context(), credential(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func reservationToken(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get(HeaderReservationToken))
}

// Reservation serves GET /v1/public/reservations/{reference}.
func (h *Public) Reservation(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Reservation(r.Context(), credential(r), r.PathValue("reference"), reservationToken(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ChangeRequest serves POST /v1/public/reservations/{reference}/change-requests.
func (h *Public) ChangeRequest(w http.ResponseWriter, r *http.Request) {
	var in model.ChangeRequestInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.ChangeRequest(r.Context(), credential(r), r.PathValue("reference"), reservationToken(r), in, clientInfo(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// CancellationRequest serves POST /v1/public/reservations/{reference}/cancellation-requests.
func (h *Public) CancellationRequest(w http.ResponseWriter, r *http.Request) {
	var in model.CancellationRequestInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.CancellationRequest(r.Context(), credential(r), r.PathValue("reference"), reservationToken(r), in, clientInfo(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// PaymentLink serves POST /v1/public/reservations/{reference}/payment-links.
func (h *Public) PaymentLink(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.PaymentLink(r.Context(), credential(r), r.PathValue("reference"), reservationToken(r), clientInfo(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Mount registers the public routes on mux.
func (h *Public) Mount(mux *http.ServeMux) {
	read := func(scope string) route { return route{scope: scope} }
	write := func(scope string, limit int, window time.Duration, action string) route {
		return route{scope: scope, write: true, limit: limit, window: window, action: action}
	}
	paths := []string{
		"/v1/public/sessions", "/v1/public/locations", "/v1/public/vehicle-subgroups", "/v1/public/extras",
		"/v1/public/availability", "/v1/public/quotes", "/v1/public/quotes/{uuid}", "/v1/public/reservations",
		"/v1/public/reservations/{reference}", "/v1/public/reservations/{reference}/change-requests",
		"/v1/public/reservations/{reference}/cancellation-requests", "/v1/public/reservations/{reference}/payment-links",
	}
	for _, p := range paths {
		mux.HandleFunc("OPTIONS "+p, h.Preflight)
	}
	mux.HandleFunc("POST /v1/public/sessions", h.Session)
	mux.HandleFunc("GET /v1/public/locations", h.guard(read(model.ScopeCatalogRead), h.Locations))
	mux.HandleFunc("GET /v1/public/vehicle-subgroups", h.guard(read(model.ScopeCatalogRead), h.Subgroups))
	mux.HandleFunc("GET /v1/public/extras", h.guard(read(model.ScopeCatalogRead), h.Extras))
	mux.HandleFunc("GET /v1/public/availability", h.guard(read(model.ScopeCatalogRead), h.Availability))
	mux.HandleFunc("POST /v1/public/quotes", h.guard(write(model.ScopeQuotesWrite, 120, time.Minute, "quotes"), h.CreateQuote))
	mux.HandleFunc("GET /v1/public/quotes/{uuid}", h.guard(read(model.ScopeQuotesWrite), h.GetQuote))
	mux.HandleFunc("POST /v1/public/reservations",
		h.guard(write(model.ScopeReservationsWrite, 20, 10*time.Minute, "reservations"), h.CreateReservation))
	mux.HandleFunc("GET /v1/public/reservations/{reference}", h.guard(read(model.ScopeReservationsRead), h.Reservation))
	mux.HandleFunc("POST /v1/public/reservations/{reference}/change-requests",
		h.guard(write(model.ScopeReservationsWrite, 10, time.Hour, "requests"), h.ChangeRequest))
	mux.HandleFunc("POST /v1/public/reservations/{reference}/cancellation-requests",
		h.guard(write(model.ScopeReservationsWrite, 10, time.Hour, "requests"), h.CancellationRequest))
	mux.HandleFunc("POST /v1/public/reservations/{reference}/payment-links",
		h.guard(write(model.ScopeReservationsWrite, 20, 10*time.Minute, "payment_links"), h.PaymentLink))
}
