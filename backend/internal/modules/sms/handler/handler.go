// Package handler is the HTTP layer of the tenant SMS routes (TEC-734):
// the integration panel's test send. The account is looked up before the
// body is validated (another organization's uuid answers 404). Numbers are
// answered and logged masked only.
package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Error codes.
const (
	CodeNoAccount   = "SMS_NO_ACCOUNT"
	CodeRateLimited = "SMS_RATE_LIMITED"
	CodeNoConsent   = "SMS_NO_CONSENT"
)

// Handler serves the SMS routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler (rec may be nil).
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

type testInput struct {
	Phone string `json:"phone"`
}

// TestSend serves POST /v1/tenant/integrations/{uuid}/test-sms.
func (h *Handler) TestSend(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	org := orgctx.Must(r.Context()).ID
	if err := h.svc.AccountExists(r.Context(), org, id); err != nil {
		writeError(w, r, err)
		return
	}
	var in testInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	sent, err := h.svc.TestSend(r.Context(), org, id, in.Phone)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		p := authctx.MustPrincipal(r.Context())
		h.activity.Record(r.Context(), &p.UserInternal, "integrations.sms_test_sent", usecase.ResourceSMS, &id, map[string]any{
			"phone": sent.PhoneMasked, "sms_message_uuid": sent.MessageUUID, "job_uuid": sent.JobUUID,
		}, r)
	}
	response.JSON(w, r, http.StatusAccepted, sent)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Code: ve.Code, Message: ve.Message}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrAccountNotFound):
		response.NotFound(w, r, "Integration account was not found")
	case errors.Is(err, usecase.ErrNoAccount):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeNoAccount, err.Error(),
			[]response.Detail{{Code: "no_account", Message: err.Error()}})
	case errors.Is(err, usecase.ErrRateLimited):
		response.ErrorWithDetails(w, r, http.StatusTooManyRequests, CodeRateLimited, err.Error(),
			[]response.Detail{{Code: "rate_limited", Message: err.Error()}})
	case errors.Is(err, usecase.ErrNoConsent):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeNoConsent, err.Error(),
			[]response.Detail{{Code: "no_consent", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
