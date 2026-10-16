// Package handler is the HTTP layer of the channel manager (TEC-736):
// /v1/tenant/channel-accounts, /channel-mappings, /channel-messages and
// /channel-sync-runs. Every handler runs behind RequireOrganization and
// reads the organization from orgctx.Scope; path uuids are resolved inside
// the organization, so another organization's uuid answers 404.
package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes.
const (
	CodeBusy             = "CHANNEL_SYNC_BUSY"
	CodeAccountInactive  = "CHANNEL_ACCOUNT_INACTIVE"
	CodeAdapter          = "CHANNEL_ADAPTER_UNAVAILABLE"
	CodeNotSimulator     = "CHANNEL_NOT_SIMULATOR"
	CodeNotRetryable     = "CHANNEL_MESSAGE_NOT_RETRYABLE"
	CodeUnsupported      = "CHANNEL_UNSUPPORTED"
	CodeMappingTaken     = "CHANNEL_MAPPING_TAKEN"
	CodeChannelRejected  = "CHANNEL_REJECTED"
	CodeChannelUnreached = "CHANNEL_UNAVAILABLE"
)

// Activity actions.
const (
	ActionSettingsUpdated = "channels.settings_updated"
	ActionMappingCreated  = "channels.mapping_created"
	ActionMappingUpdated  = "channels.mapping_updated"
	ActionMappingDeleted  = "channels.mapping_deleted"
	ActionPushed          = "channels.pushed"
	ActionPulled          = "channels.pulled"
	ActionSimulated       = "channels.simulated"
	ActionMessageRetried  = "channels.message_retried"
	ActionMessageIgnored  = "channels.message_ignored"
)

// Handler serves the channel manager.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler (rec may be nil).
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func (h *Handler) audit(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func optionalUUID(r *http.Request, key string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, &model.QueryError{Field: key, Value: raw}
	}
	return &id, nil
}

func optionalBool(r *http.Request, key string) (*bool, error) {
	switch strings.TrimSpace(r.URL.Query().Get(key)) {
	case "":
		return nil, nil
	case "true", "1":
		v := true
		return &v, nil
	case "false", "0":
		v := false
		return &v, nil
	}
	return nil, &model.QueryError{Field: key, Value: r.URL.Query().Get(key)}
}

func joined(r *http.Request, key string) string { return strings.Join(httpkit.Values(r, key), ",") }

// ---------------------------------------------------------------- accounts

// ListAccounts serves GET /v1/tenant/channel-accounts.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListAccounts(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GetAccount serves GET /v1/tenant/channel-accounts/{uuid}.
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetAccount(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateAccount serves PATCH /v1/tenant/channel-accounts/{uuid}.
func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.SettingsInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.UpdateSettings(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionSettingsUpdated, model.ResourceAccounts, id, map[string]any{"settings": out.Settings})
	response.JSON(w, r, http.StatusOK, out)
}

// Push serves POST /v1/tenant/channel-accounts/{uuid}/push.
func (h *Handler) Push(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	run, err := h.svc.PushNow(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionPushed, model.ResourceAccounts, id, map[string]any{"status": run.Status, "items": run.ItemsTotal})
	response.JSON(w, r, http.StatusOK, run)
}

// Pull serves POST /v1/tenant/channel-accounts/{uuid}/pull.
func (h *Handler) Pull(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	run, err := h.svc.PullNow(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionPulled, model.ResourceAccounts, id, map[string]any{"status": run.Status, "items": run.ItemsTotal})
	response.JSON(w, r, http.StatusOK, run)
}

// Simulate serves POST /v1/tenant/channel-accounts/{uuid}/simulate
// (development adapters only).
func (h *Handler) Simulate(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.SimulateInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	run, err := h.svc.Simulate(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionSimulated, model.ResourceAccounts, id, map[string]any{"type": in.Type, "external_ref": in.ExternalRef})
	response.JSON(w, r, http.StatusOK, run)
}

func parseDay(raw string, field string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), istanbul)
	if err != nil {
		return time.Time{}, &model.QueryError{Field: field, Value: raw}
	}
	return t, nil
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*3600)
	}
	return loc
}()

// Reconciliation serves GET /v1/tenant/channel-accounts/{uuid}/reconciliation
// (?from=YYYY-MM-DD&to=YYYY-MM-DD, to inclusive; default: this month).
func (h *Handler) Reconciliation(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	now := time.Now().In(istanbul)
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, istanbul)
	to := from.AddDate(0, 1, 0)
	if raw := r.URL.Query().Get("from"); raw != "" {
		v, err := parseDay(raw, "from")
		if err != nil {
			writeError(w, r, err)
			return
		}
		from = v
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		v, err := parseDay(raw, "to")
		if err != nil {
			writeError(w, r, err)
			return
		}
		to = v.AddDate(0, 0, 1)
	}
	out, err := h.svc.Reconcile(r.Context(), orgID(r), id, from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out.To = to.AddDate(0, 0, -1).Format("2006-01-02")
	response.JSON(w, r, http.StatusOK, out)
}

// ---------------------------------------------------------------- mappings

var mappingSortable = map[string]string{"created_at": "created_at", "updated_at": "updated_at", "external_code": "external_code"}

// ListMappings serves GET /v1/tenant/channel-mappings.
func (h *Handler) ListMappings(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, mappingSortable)
	if !ok {
		return
	}
	f := model.MappingFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.AccountUUID, err = optionalUUID(r, "account"); err != nil {
		writeError(w, r, err)
		return
	}
	if f.EntityTypes, err = model.SplitValues("entity_type", joined(r, "entity_type"), model.EntityTypes); err != nil {
		writeError(w, r, err)
		return
	}
	if f.Active, err = optionalBool(r, "active"); err != nil {
		writeError(w, r, err)
		return
	}
	if f.StopSale, err = optionalBool(r, "stop_sale"); err != nil {
		writeError(w, r, err)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListMappings(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// MappingFacets serves GET /v1/tenant/channel-mappings/facets.
func (h *Handler) MappingFacets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.MappingFacets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GetMapping serves GET /v1/tenant/channel-mappings/{uuid}.
func (h *Handler) GetMapping(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetMapping(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreateMapping serves POST /v1/tenant/channel-mappings.
func (h *Handler) CreateMapping(w http.ResponseWriter, r *http.Request) {
	var in model.MappingInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.CreateMapping(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionMappingCreated, model.ResourceMappings, out.UUID, map[string]any{"entity_type": out.EntityType, "external_code": out.ExternalCode})
	response.JSON(w, r, http.StatusCreated, out)
}

// UpdateMapping serves PATCH /v1/tenant/channel-mappings/{uuid}.
func (h *Handler) UpdateMapping(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.MappingInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.UpdateMapping(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionMappingUpdated, model.ResourceMappings, id, map[string]any{
		"external_code": out.ExternalCode, "stop_sale": out.StopSale, "max_quota": out.MaxQuota, "is_active": out.IsActive,
	})
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteMapping serves DELETE /v1/tenant/channel-mappings/{uuid}.
func (h *Handler) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteMapping(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionMappingDeleted, model.ResourceMappings, id, nil)
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ---------------------------------------------------------------- messages

var messageSortable = map[string]string{"created_at": "created_at", "occurred_at": "occurred_at"}

// ListMessages serves GET /v1/tenant/channel-messages.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, messageSortable)
	if !ok {
		return
	}
	f := model.MessageFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset, ExternalRef: r.URL.Query().Get("external_ref")}
	var err error
	if f.AccountUUID, err = optionalUUID(r, "account"); err != nil {
		writeError(w, r, err)
		return
	}
	if f.Statuses, err = model.SplitValues("status", joined(r, "status"), model.MessageStatuses); err != nil {
		writeError(w, r, err)
		return
	}
	if f.Types, err = model.SplitValues("type", joined(r, "type"), model.MessageTypes); err != nil {
		writeError(w, r, err)
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

// MessageFacets serves GET /v1/tenant/channel-messages/facets.
func (h *Handler) MessageFacets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.MessageFacets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GetMessage serves GET /v1/tenant/channel-messages/{uuid}.
func (h *Handler) GetMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetMessage(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RetryMessage serves POST /v1/tenant/channel-messages/{uuid}/retry.
func (h *Handler) RetryMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.RetryMessage(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionMessageRetried, model.ResourceMessages, id, map[string]any{"status": out.Status, "error_code": out.ErrorCode})
	response.JSON(w, r, http.StatusOK, out)
}

// IgnoreMessage serves POST /v1/tenant/channel-messages/{uuid}/ignore.
func (h *Handler) IgnoreMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.IgnoreMessage(r.Context(), orgID(r), id, in.Note)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionMessageIgnored, model.ResourceMessages, id, map[string]any{"note": strings.TrimSpace(in.Note)})
	response.JSON(w, r, http.StatusOK, out)
}

// ---------------------------------------------------------------- sync log

// ListSyncRuns serves GET /v1/tenant/channel-sync-runs.
func (h *Handler) ListSyncRuns(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, map[string]string{"started_at": "started_at"})
	if !ok {
		return
	}
	f := model.SyncRunFilter{Limit: q.Limit, Offset: q.Offset, SortDesc: true}
	var err error
	if f.AccountUUID, err = optionalUUID(r, "account"); err != nil {
		writeError(w, r, err)
		return
	}
	if f.Directions, err = model.SplitValues("direction", joined(r, "direction"), model.SyncDirections); err != nil {
		writeError(w, r, err)
		return
	}
	if f.Statuses, err = model.SplitValues("status", joined(r, "status"), model.SyncStatuses); err != nil {
		writeError(w, r, err)
		return
	}
	if len(q.Sort) > 0 {
		f.SortDesc = q.Sort[0].Desc
	}
	items, total, err := h.svc.ListSyncRuns(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qe *model.QueryError
	if errors.As(err, &qe) {
		response.ValidationError(w, r, []response.Detail{{Field: qe.Field, Message: qe.Error(), Code: "invalid"}})
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
	conflict := func(code string) {
		response.ErrorWithDetails(w, r, http.StatusConflict, code, err.Error(), []response.Detail{{Code: strings.ToLower(code), Message: err.Error()}})
	}
	switch {
	case errors.Is(err, usecase.ErrAccountNotFound):
		response.NotFound(w, r, "Channel account was not found")
	case errors.Is(err, usecase.ErrMappingNotFound):
		response.NotFound(w, r, "Channel mapping was not found")
	case errors.Is(err, usecase.ErrMessageNotFound):
		response.NotFound(w, r, "Channel message was not found")
	case errors.Is(err, orgctx.ErrAllBranchesRequired):
		response.Forbidden(w, r, "The channel manager needs access to every branch")
	case errors.Is(err, usecase.ErrBusy):
		conflict(CodeBusy)
	case errors.Is(err, usecase.ErrAccountInactive):
		conflict(CodeAccountInactive)
	case errors.Is(err, usecase.ErrAdapterUnavailable):
		conflict(CodeAdapter)
	case errors.Is(err, usecase.ErrNotSimulator):
		conflict(CodeNotSimulator)
	case errors.Is(err, usecase.ErrNotRetryable):
		conflict(CodeNotRetryable)
	case errors.Is(err, usecase.ErrUnsupported):
		conflict(CodeUnsupported)
	case errors.Is(err, usecase.ErrMappingTaken):
		conflict(CodeMappingTaken)
	case errors.Is(err, channels.ErrRejected):
		response.ErrorWithDetails(w, r, http.StatusBadGateway, CodeChannelRejected, "The channel rejected the request",
			[]response.Detail{{Code: "rejected", Message: err.Error()}})
	case errors.Is(err, channels.ErrUnavailable):
		response.ErrorWithDetails(w, r, http.StatusBadGateway, CodeChannelUnreached, "The channel could not be reached",
			[]response.Detail{{Code: "unavailable", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
