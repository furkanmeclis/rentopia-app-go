// Package handler is the HTTP layer of /v1/tenant/integration-jobs (TEC-751):
// a job with its attempts, and the staff retry. Behind RequireOrganization +
// tenant.integrations.manage; another organization's uuid answers 404.
package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Resource is the activity resource of integration jobs.
const Resource = "tenant.integration_jobs"

// Error codes.
const (
	CodeNotRetryable = "INTEGRATION_JOB_NOT_RETRYABLE"
	CodeUnverified   = "INTEGRATION_JOB_UNVERIFIED"
	CodeBusy         = "INTEGRATION_JOB_BUSY"
	// CodeNotReconcilable: final, inbound, held or owned by another flow.
	CodeNotReconcilable = "INTEGRATION_JOB_NOT_RECONCILABLE"
)

// Handler serves integration jobs.
type Handler struct {
	runner   *usecase.Runner
	activity *activity.Recorder
}

// New builds the handler (rec may be nil).
func New(runner *usecase.Runner, rec *activity.Recorder) *Handler {
	return &Handler{runner: runner, activity: rec}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

// Get serves GET /v1/tenant/integration-jobs/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.runner.Detail(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// Retry serves POST /v1/tenant/integration-jobs/{uuid}/retry: CheckStatus
// first, resubmit only when the provider confirms the work was not done.
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	job, err := h.runner.Detail(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// A retry sends again: the provider's plan feature must be on (TEC-674).
	if f, ok := features.ForProvider(job.Provider); ok {
		if err := features.Require(r.Context(), nil, orgID(r), f); err != nil {
			features.WriteError(w, r, err)
			return
		}
	}
	res, err := h.runner.Retry(r.Context(), orgID(r), id)
	var ve *usecase.VerifyError
	if errors.As(err, &ve) {
		h.audit(r, id, map[string]any{"result": "unverified", "outcome": string(ve.Outcome)})
		d, _ := h.runner.Detail(r.Context(), orgID(r), id)
		response.ErrorWithData(w, r, http.StatusConflict, CodeUnverified, ve.Error(),
			[]response.Detail{{Code: "unverified", Message: ve.Message}}, d)
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, id, map[string]any{"result": res.Action, "status": res.Status})
	d, err := h.runner.Detail(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"action": res.Action, "job": d})
}

func (h *Handler) audit(r *http.Request, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, "integrations.job_retried", Resource, &id, payload, r)
}

// ReconcileRequest is POST /v1/tenant/integration-jobs/{uuid}/reconcile.
type ReconcileRequest struct {
	Note        string `json:"note"`
	ExternalRef string `json:"external_ref"`
}

// BulkReconcileRequest is POST /v1/tenant/integration-jobs/reconcile.
type BulkReconcileRequest struct {
	Before   *time.Time `json:"before"`
	Statuses []string   `json:"statuses"`
	Kinds    []string   `json:"kinds"`
	Note     string     `json:"note"`
}

func noteProblem(note string) []response.Detail {
	n := len([]rune(strings.TrimSpace(note)))
	if n < usecase.ReconcileNoteMin || n > usecase.ReconcileNoteMax {
		return []response.Detail{{Field: "note", Code: "required", Message: "a note is required (3-500 characters)"}}
	}
	return nil
}

// Reconcile serves POST /v1/tenant/integration-jobs/{uuid}/reconcile: the
// work was confirmed outside the runner (provider panel, delivery log);
// the job moves to reconciled and is never sent again. Sends nothing, so
// it is allowed whatever the plan (like disabling an account).
func (h *Handler) Reconcile(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if _, err := h.runner.Detail(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	var in ReconcileRequest
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	details := noteProblem(in.Note)
	if len(strings.TrimSpace(in.ExternalRef)) > usecase.ReconcileRefMax {
		details = append(details, response.Detail{Field: "external_ref", Code: "too_long", Message: "external_ref is at most 200 characters"})
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	p := authctx.MustPrincipal(r.Context())
	res, err := h.runner.Reconcile(r.Context(), orgID(r), id, usecase.ReconcileInput{
		Note: in.Note, ExternalRef: in.ExternalRef, ActorID: p.UserInternal,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordReconciled(r, res, strings.TrimSpace(in.Note), strings.TrimSpace(in.ExternalRef), false)
	d, err := h.runner.Detail(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// BulkReconcile serves POST /v1/tenant/integration-jobs/reconcile (restore
// runbook): every job of the organization created before the cut-off in
// one of the given states (and kinds) is marked reconciled. Jobs held by a
// worker are skipped. Answers the count and the job uuids.
func (h *Handler) BulkReconcile(w http.ResponseWriter, r *http.Request) {
	var in BulkReconcileRequest
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	f := usecase.BulkReconcileFilter{Statuses: in.Statuses, Kinds: in.Kinds}
	if in.Before != nil {
		f.Before = *in.Before
	}
	details := noteProblem(in.Note)
	if field, msg := f.Validate(); field != "" {
		details = append(details, response.Detail{Field: field, Code: "invalid", Message: msg})
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	p := authctx.MustPrincipal(r.Context())
	res, err := h.runner.ReconcileMatching(r.Context(), orgID(r), f, usecase.ReconcileInput{Note: in.Note, ActorID: p.UserInternal})
	if err != nil {
		writeError(w, r, err)
		return
	}
	ids := make([]uuid.UUID, 0, len(res))
	for _, j := range res {
		h.recordReconciled(r, j, strings.TrimSpace(in.Note), "", true)
		ids = append(ids, j.UUID)
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"count": len(res), "job_uuids": ids})
}

func (h *Handler) recordReconciled(r *http.Request, j usecase.Reconciled, note, ref string, bulk bool) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	id := j.UUID
	h.activity.Record(r.Context(), &p.UserInternal, "integrations.job_reconciled", Resource, &id, map[string]any{
		"kind": j.Kind, "previous_status": j.PreviousStatus, "note": note, "external_ref": ref, "bulk": bulk,
	}, r)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Integration job was not found")
	case errors.Is(err, usecase.ErrNotRetryable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNotRetryable, err.Error(),
			[]response.Detail{{Code: "not_retryable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrNotReconcilable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNotReconcilable, err.Error(),
			[]response.Detail{{Code: "not_reconcilable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrBusy):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeBusy, err.Error(),
			[]response.Detail{{Code: "busy", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
