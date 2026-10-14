package handler

import (
	"context"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// TransfersMeta serves GET /v1/tenant/transfers/meta.
func (h *Handler) TransfersMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, TransfersMeta())
}

// TransferFacets serves GET /v1/tenant/transfers/facets.
func (h *Handler) TransferFacets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	facets, err := h.svc.TransferFacets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// ListTransfers serves GET /v1/tenant/transfers.
func (h *Handler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	q, ok := listQuery(w, r, usecase.TransferSortColumns)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.TransferQueryKeys {
		filters[key] = joinValues(r.URL.Query(), key)
	}
	f := model.TransferFilterFromQuery(filters)
	f.Limit, f.Offset, f.SortField, f.SortDesc = q.Limit, q.Offset, "planned_departure", true
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListTransfers(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// TransferAssignees serves GET /v1/tenant/transfers/assignees: members who
// can be responsible for a transfer (no members permission needed).
func (h *Handler) TransferAssignees(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	items, err := h.svc.Assignees(r.Context(), scope.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateTransfer serves POST /v1/tenant/transfers.
func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.TransferInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	t, err := h.svc.CreateTransfer(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordTransfer(r, "transfers.created", resourceevents.ActionCreated, t, nil)
	response.JSON(w, r, http.StatusCreated, t)
}

// GetTransfer serves GET /v1/tenant/transfers/{uuid}.
func (h *Handler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	t, err := h.svc.GetTransfer(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// PatchTransfer serves PATCH /v1/tenant/transfers/{uuid}.
func (h *Handler) PatchTransfer(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.TransferInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	t, err := h.svc.PatchTransfer(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordTransfer(r, "transfers.updated", resourceevents.ActionUpdated, t, nil)
	response.JSON(w, r, http.StatusOK, t)
}

type transferAction func(ctx context.Context, orgID int64, id uuid.UUID, in model.TransferActionInput) (model.Transfer, error)

// lifecycle runs start / complete / cancel and publishes the transfer and
// the vehicle (its status / branch changed).
func (h *Handler) lifecycle(w http.ResponseWriter, r *http.Request, action string, fn transferAction) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.TransferActionInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	t, err := fn(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordTransfer(r, action, resourceevents.ActionUpdated, t, map[string]any{"note": in.Note})
	h.publish(r, model.ResourceVehicles, resourceevents.ActionUpdated, t.Vehicle.UUID)
	response.JSON(w, r, http.StatusOK, t)
}

// StartTransfer serves POST /v1/tenant/transfers/{uuid}/start.
func (h *Handler) StartTransfer(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "transfers.started", h.svc.StartTransfer)
}

// CompleteTransfer serves POST /v1/tenant/transfers/{uuid}/complete.
func (h *Handler) CompleteTransfer(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "transfers.completed", h.svc.CompleteTransfer)
}

// CancelTransfer serves POST /v1/tenant/transfers/{uuid}/cancel.
func (h *Handler) CancelTransfer(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "transfers.cancelled", h.svc.CancelTransfer)
}

// TransferActivity serves GET /v1/tenant/transfers/{uuid}/activity.
func (h *Handler) TransferActivity(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.TransferActivity(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) recordTransfer(r *http.Request, action, event string, t model.Transfer, extra map[string]any) {
	payload := map[string]any{
		"plate": t.Vehicle.Plate, "from": t.FromBranch.Code, "to": t.ToBranch.Code, "status": t.Status,
	}
	for k, v := range extra {
		if v != "" && v != nil {
			payload[k] = v
		}
	}
	h.record(r, action, model.ResourceTransfers, t.UUID, payload)
	h.publish(r, model.ResourceTransfers, event, t.UUID)
}
