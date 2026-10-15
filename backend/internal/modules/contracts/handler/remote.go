package handler

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Remote approval response codes.
const (
	CodeLinkUnavailable = "LINK_UNAVAILABLE"
	CodeOTPInvalid      = "OTP_INVALID"
	CodeRateLimited     = "RATE_LIMITED"
	CodeCodeUnavailable = "CODE_UNAVAILABLE"
	CodeSMSAccount      = "SMS_ACCOUNT_REQUIRED"
	CodeRemoteDisabled  = "REMOTE_APPROVAL_UNAVAILABLE"
)

// RateLimiter is the per-IP / per-token window limiter of the public
// routes (ratelimit.Limiter adapter; nil = unlimited).
type RateLimiter interface {
	Allow(r *http.Request, action, subject string, limit int, window time.Duration) bool
}

// Public route limits (per window).
type limit struct {
	action string
	n      int
	window time.Duration
}

var (
	limitPageIP      = limit{"doc_page_ip", 60, time.Minute}
	limitCodeIP      = limit{"doc_code_ip", 10, 10 * time.Minute}
	limitCodeToken   = limit{"doc_code_token", 5, 10 * time.Minute}
	limitVerifyIP    = limit{"doc_verify_ip", 30, 10 * time.Minute}
	limitVerifyToken = limit{"doc_verify_token", 10, 10 * time.Minute}
	limitPDFIP       = limit{"doc_pdf_ip", 30, 10 * time.Minute}
	limitPDFToken    = limit{"doc_pdf_token", 10, 10 * time.Minute}
)

// SetRateLimiter wires the public route limits.
func (h *Handler) SetRateLimiter(l RateLimiter) { h.limiter = l }

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, l limit, subject string) bool {
	if h.limiter == nil || h.limiter.Allow(r, l.action, subject, l.n, l.window) {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(l.window.Seconds())))
	response.ErrorWithDetails(w, r, http.StatusTooManyRequests, CodeRateLimited, "Too many requests; try again later",
		[]response.Detail{{Code: "rate_limited", Message: "Too many requests; try again later"}})
	return false
}

func client(r *http.Request) model.ClientInfo {
	return model.ClientInfo{IP: httpkit.ClientIP(r), UserAgent: r.UserAgent()}
}

// ---------------------------------------------------------------- tenant

func (h *Handler) docAndLink(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	lid, ok := pathUUID(w, r, "linkUuid")
	return id, lid, ok
}

// Signers serves GET /v1/tenant/documents/{uuid}/signers.
func (h *Handler) Signers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Signers(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListApprovalLinks serves GET /v1/tenant/documents/{uuid}/approval-links.
func (h *Handler) ListApprovalLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.ListApprovalLinks(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// CreateApprovalLink serves POST …/versions/{documentVersionUuid}/approval-links.
func (h *Handler) CreateApprovalLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	vid, ok := pathUUID(w, r, "documentVersionUuid")
	if !ok {
		return
	}
	// Path uuids first: another organization's answer 404, not 400.
	if _, err := h.svc.GetVersion(r.Context(), orgID(r), id, vid); err != nil {
		writeError(w, r, err)
		return
	}
	var in model.CreateLinkInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.CreateApprovalLink(r.Context(), orgID(r), id, vid, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceDocuments, "documents.approval_link_created", id, map[string]any{
		"version_uuid": vid, "link_uuid": out.Link.UUID, "signer_role": out.Link.SignerRole, "phone": out.Link.PhoneMasked,
		"expires_at": out.Link.ExpiresAt, "sms": out.SMS != nil,
	})
	response.JSON(w, r, http.StatusCreated, out)
}

// SendApprovalLink serves POST …/approval-links/{linkUuid}/send-sms.
func (h *Handler) SendApprovalLink(w http.ResponseWriter, r *http.Request) {
	id, lid, ok := h.docAndLink(w, r)
	if !ok {
		return
	}
	var in struct{}
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.SendApprovalLink(r.Context(), orgID(r), id, lid)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceDocuments, "documents.approval_link_sent", id, map[string]any{
		"link_uuid": lid, "phone": out.PhoneMasked, "sms_message_uuid": out.MessageUUID,
	})
	response.JSON(w, r, http.StatusAccepted, out)
}

// RevokeApprovalLink serves POST …/approval-links/{linkUuid}/revoke.
func (h *Handler) RevokeApprovalLink(w http.ResponseWriter, r *http.Request) {
	id, lid, ok := h.docAndLink(w, r)
	if !ok {
		return
	}
	var in struct{}
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.RevokeApprovalLink(r.Context(), orgID(r), id, lid)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceDocuments, "documents.approval_link_revoked", id, map[string]any{"link_uuid": lid})
	response.JSON(w, r, http.StatusOK, out)
}

// DownloadApprovalPDF serves GET …/approvals/{approvalUuid}/pdf.
func (h *Handler) DownloadApprovalPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	aid, ok := pathUUID(w, r, "approvalUuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenApprovalPDF(r.Context(), orgID(r), id, aid)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	h.audit(r, model.ResourceDocuments, "documents.downloaded", id, map[string]any{"approval_uuid": aid, "kind": f.Kind})
	streamPDF(w, f)
}

func streamPDF(w http.ResponseWriter, f usecase.PDFFile) {
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

// ---------------------------------------------------------------- public

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// PublicDocument serves GET /v1/public/documents/{token}.
func (h *Handler) PublicDocument(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, limitPageIP, httpkit.ClientIP(r)) {
		return
	}
	out, err := h.svc.PublicDocument(r.Context(), r.PathValue("token"))
	if err != nil {
		writePublicError(w, r, err)
		return
	}
	noStore(w)
	response.JSON(w, r, http.StatusOK, out)
}

// PublicRequestCode serves POST /v1/public/documents/{token}/code.
func (h *Handler) PublicRequestCode(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !h.allow(w, r, limitCodeIP, httpkit.ClientIP(r)) || !h.allow(w, r, limitCodeToken, usecase.TokenKey(token)) {
		return
	}
	var in struct{}
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.RequestCode(r.Context(), token, client(r))
	if err != nil {
		writePublicError(w, r, err)
		return
	}
	noStore(w)
	response.JSON(w, r, http.StatusOK, out)
}

// PublicApprove serves POST /v1/public/documents/{token}/approve.
func (h *Handler) PublicApprove(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !h.allow(w, r, limitVerifyIP, httpkit.ClientIP(r)) || !h.allow(w, r, limitVerifyToken, usecase.TokenKey(token)) {
		return
	}
	var in model.PublicApproveInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.ApproveRemote(r.Context(), token, in, client(r))
	if err != nil {
		writePublicError(w, r, err)
		return
	}
	noStore(w)
	response.JSON(w, r, http.StatusOK, out)
}

// PublicPDF serves GET /v1/public/documents/{token}/pdf: the approval PDF
// of the link's own approval only.
func (h *Handler) PublicPDF(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !h.allow(w, r, limitPDFIP, httpkit.ClientIP(r)) || !h.allow(w, r, limitPDFToken, usecase.TokenKey(token)) {
		return
	}
	f, err := h.svc.OpenPublicPDF(r.Context(), token)
	if err != nil {
		writePublicError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Referrer-Policy", "no-referrer")
	streamPDF(w, f)
}

// writePublicError answers the public routes with generic messages: no
// answer tells whether a number exists, was reachable or which check of a
// code failed.
func writePublicError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var cc *usecase.ContentChangedError
	var ls *usecase.LinkStateError
	var cd *usecase.CooldownError
	switch {
	case errors.As(err, &cc):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeContentChanged,
			"The document changed since it was shown; reload the page",
			[]response.Detail{{Field: "content_hash", Code: "content_changed", Message: "The document changed since it was shown"}})
	case errors.As(err, &ls):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeLinkUnavailable, "This approval link can no longer be used",
			[]response.Detail{{Code: ls.State, Message: "This approval link can no longer be used"}})
	case errors.As(err, &cd):
		secs := int(cd.RetryAfter.Seconds() + 0.999)
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		response.ErrorWithDetails(w, r, http.StatusTooManyRequests, CodeRateLimited, "A code was just sent; try again shortly",
			[]response.Detail{{Code: "cooldown", Message: strconv.Itoa(secs)}})
	case errors.Is(err, usecase.ErrRemoteRateLimited):
		response.ErrorWithDetails(w, r, http.StatusTooManyRequests, CodeRateLimited, "Too many attempts; try again later",
			[]response.Detail{{Code: "rate_limited", Message: "Too many attempts; try again later"}})
	case errors.Is(err, usecase.ErrCodeInvalid):
		response.ErrorWithDetails(w, r, http.StatusBadRequest, CodeOTPInvalid, err.Error(),
			[]response.Detail{{Field: "code", Code: "invalid", Message: err.Error()}})
	case errors.Is(err, usecase.ErrCodeUnavailable):
		response.ErrorWithDetails(w, r, http.StatusServiceUnavailable, CodeCodeUnavailable, err.Error(),
			[]response.Detail{{Code: "code_unavailable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrLinkNotFound), errors.Is(err, usecase.ErrApprovalNotFound):
		response.NotFound(w, r, "Approval link was not found")
	case errors.Is(err, usecase.ErrPDFNotReady), errors.Is(err, usecase.ErrStorageUnavailable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodePDFNotReady, "The PDF is not ready yet; try again shortly",
			[]response.Detail{{Code: "pdf_not_ready", Message: "The PDF is not ready yet"}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
