// Package handler is the HTTP layer of the availability engine: capacity
// search, holds ("opsiyon") and channel capacity rules. Every handler runs
// behind RequireOrganization and reads the organization from orgctx.Scope.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the availability routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetResourceEvents publishes live list changes (optional).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) { h.events = pub }

func (h *Handler) record(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func (h *Handler) publish(r *http.Request, resource, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	h.events.Publish(r.Context(), resource, action, id.String())
}

func actorID(r *http.Request) *int64 {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok || p.UserInternal == 0 {
		return nil
	}
	id := p.UserInternal
	return &id
}

// Search serves POST /v1/tenant/availability/search.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	var in model.SearchInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.Search(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// ListHolds serves GET /v1/tenant/availability/holds?status=&kind=.
func (h *Handler) ListHolds(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, nil)
	if !ok {
		return
	}
	values := r.URL.Query()
	f := model.AllocationFilter{Limit: q.Limit, Offset: q.Offset, Kinds: []string{model.KindHold}}
	if v := splitList(values.Get("status")); len(v) > 0 {
		f.Statuses = v
	} else {
		f.Statuses = []string{model.StatusActive}
	}
	items, total, err := h.svc.ListAllocations(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateHold serves POST /v1/tenant/availability/holds.
func (h *Handler) CreateHold(w http.ResponseWriter, r *http.Request) {
	var in model.HoldInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.Reserve(r.Context(), orgctx.Must(r.Context()).ID, model.ReserveInput{
		Where: in.Where, ProductUUID: in.ProductUUID, Kind: model.KindHold, HoldMinutes: in.HoldMinutes,
		VehicleUUID: in.VehicleUUID, Note: strings.TrimSpace(in.Note), ActorID: actorID(r),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "created", model.ResourceHolds, res.Allocation.UUID, map[string]any{
		"product": res.Allocation.Product.Code, "starts_at": res.Allocation.StartsAt, "ends_at": res.Allocation.EndsAt,
		"reassigned": res.Reassigned,
	})
	h.publish(r, model.ResourceHolds, resourceevents.ActionCreated, res.Allocation.UUID)
	response.JSON(w, r, http.StatusCreated, res)
}

// GetHold serves GET /v1/tenant/availability/holds/{uuid}.
func (h *Handler) GetHold(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	a, err := h.svc.GetAllocation(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err == nil && a.Kind != model.KindHold && a.Kind != model.KindReservation {
		err = usecase.ErrAllocationNotFound
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

// ReleaseHold serves DELETE /v1/tenant/availability/holds/{uuid}.
func (h *Handler) ReleaseHold(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	scope := orgctx.Must(r.Context())
	a, err := h.svc.GetAllocation(r.Context(), scope.ID, id)
	if err == nil && a.Kind != model.KindHold {
		err = usecase.ErrNotHold
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	res, err := h.svc.Release(r.Context(), scope.ID, id, "released", actorID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "released", model.ResourceHolds, id, map[string]any{"reassigned": res.Reassigned, "affected": len(res.Affected)})
	h.publish(r, model.ResourceHolds, resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, res)
}

// ExtendHold serves POST /v1/tenant/availability/holds/{uuid}/extend.
func (h *Handler) ExtendHold(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ExtendInput
	if !decodeJSON(w, r, &in) {
		return
	}
	a, err := h.svc.ExtendHold(r.Context(), orgctx.Must(r.Context()).ID, id, in.Minutes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "extended", model.ResourceHolds, id, map[string]any{"minutes": in.Minutes, "expires_at": a.ExpiresAt})
	h.publish(r, model.ResourceHolds, resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, a)
}

// ---------------------------------------------------------- capacity rules

// ListRules serves GET /v1/tenant/capacity-rules.
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, model.RuleSortable)
	if !ok {
		return
	}
	values := r.URL.Query()
	f := model.RuleFilter{
		Q: q.Q, Limit: q.Limit, Offset: q.Offset,
		RuleTypes: splitList(values.Get("rule_type")), Channels: splitList(values.Get("channel")),
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListRules(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetRule serves GET /v1/tenant/capacity-rules/{uuid}.
func (h *Handler) GetRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	rule, err := h.svc.GetRule(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rule)
}

// CreateRule serves POST /v1/tenant/capacity-rules.
func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var in model.CapacityRuleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	rule, err := h.svc.CreateRule(r.Context(), orgctx.Must(r.Context()).ID, in, actorID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "created", model.ResourceCapacityRules, rule.UUID, map[string]any{"rule_type": rule.RuleType, "quantity": rule.Quantity})
	h.publish(r, model.ResourceCapacityRules, resourceevents.ActionCreated, rule.UUID)
	response.JSON(w, r, http.StatusCreated, rule)
}

// UpdateRule serves PUT /v1/tenant/capacity-rules/{uuid}.
func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CapacityRuleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	rule, err := h.svc.UpdateRule(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "updated", model.ResourceCapacityRules, rule.UUID, map[string]any{"rule_type": rule.RuleType, "quantity": rule.Quantity})
	h.publish(r, model.ResourceCapacityRules, resourceevents.ActionUpdated, rule.UUID)
	response.JSON(w, r, http.StatusOK, rule)
}

// DeleteRule serves DELETE /v1/tenant/capacity-rules/{uuid}.
func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "deleted", model.ResourceCapacityRules, id, nil)
	h.publish(r, model.ResourceCapacityRules, resourceevents.ActionDeleted, id)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// ------------------------------------------------------------------ helpers

func splitList(v string) []string {
	out := []string{}
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func listQuery(w http.ResponseWriter, r *http.Request, sortable map[string]string) (apiquery.Query, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(values)
	if sortable != nil {
		if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortable)); err != nil {
			writeError(w, r, err)
			return apiquery.Query{}, false
		}
	}
	return q, true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		details := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	if ce, ok := usecase.IsCapacity(err); ok {
		response.ErrorWithData(w, r, http.StatusConflict, response.CodeCapacityUnavailable,
			"No vehicle can serve this request for the whole range",
			[]response.Detail{{Code: ce.Reason, Message: ce.Reason}}, ce.Alternatives)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrAllocationNotFound):
		response.NotFound(w, r, "Hold was not found")
	case errors.Is(err, usecase.ErrRuleNotFound):
		response.NotFound(w, r, "Capacity rule was not found")
	case errors.Is(err, usecase.ErrNotActive):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(),
			[]response.Detail{{Code: "not_active", Message: err.Error()}})
	case errors.Is(err, usecase.ErrHoldExpired):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(),
			[]response.Detail{{Code: "hold_expired", Message: err.Error()}})
	case errors.Is(err, usecase.ErrNotHold):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(),
			[]response.Detail{{Code: "not_hold", Message: err.Error()}})
	case errors.Is(err, usecase.ErrRuleTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(),
			[]response.Detail{{Field: "rule_type", Code: "taken", Message: err.Error()}})
	case errors.Is(err, orgctx.ErrAllBranchesRequired):
		response.Forbidden(w, r, "This change needs access to every branch")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
