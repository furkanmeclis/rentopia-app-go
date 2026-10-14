package handler

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// ListBlocks serves GET /v1/tenant/vehicles/{uuid}/blocks (state / kind
// filters, comma-separated).
func (h *Handler) ListBlocks(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q, ok := listQuery(w, r, nil)
	if !ok {
		return
	}
	f := model.BlockFilter{
		Limit: q.Limit, Offset: q.Offset,
		States: model.SplitList(joinValues(r.URL.Query(), "state")),
		Kinds:  model.SplitList(joinValues(r.URL.Query(), "kind")),
	}
	items, total, err := h.svc.ListBlocks(r.Context(), scope.ID, id, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateBlock serves POST /v1/tenant/vehicles/{uuid}/blocks ("Bakıma al"
// is kind maintenance without dates).
func (h *Handler) CreateBlock(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.BlockInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	res, err := h.svc.CreateBlock(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordBlock(r, "vehicles.block_created", id, res)
	response.JSON(w, r, http.StatusCreated, res)
}

// PatchBlock serves PATCH /v1/tenant/vehicles/{uuid}/blocks/{blockUuid}.
func (h *Handler) PatchBlock(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	blockID, ok := pathUUID(w, r, "blockUuid")
	if !ok {
		return
	}
	var in model.BlockInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	res, err := h.svc.PatchBlock(r.Context(), scope.ID, id, blockID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordBlock(r, "vehicles.block_updated", id, res)
	response.JSON(w, r, http.StatusOK, res)
}

// ReleaseBlock serves POST /v1/tenant/vehicles/{uuid}/blocks/{blockUuid}/release
// ("Engeli kapat").
func (h *Handler) ReleaseBlock(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	blockID, ok := pathUUID(w, r, "blockUuid")
	if !ok {
		return
	}
	var in model.ReleaseInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := h.svc.ReleaseBlock(r.Context(), scope.ID, id, blockID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordBlock(r, "vehicles.block_released", id, res)
	response.JSON(w, r, http.StatusOK, res)
}

// DeleteBlock serves DELETE /v1/tenant/vehicles/{uuid}/blocks/{blockUuid}.
func (h *Handler) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	blockID, ok := pathUUID(w, r, "blockUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteBlock(r.Context(), scope.ID, id, blockID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.block_deleted", model.ResourceVehicles, id, map[string]any{"block_uuid": blockID.String()})
	h.publish(r, model.ResourceBlocks, resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// recordBlock writes the activity on the vehicle (blocks are a child of
// the vehicle) and refreshes the vehicle's block list and the vehicles
// list (the status may have changed).
func (h *Handler) recordBlock(r *http.Request, action string, vehicleID uuid.UUID, res model.BlockResult) {
	payload := map[string]any{
		"block_uuid": res.Block.UUID.String(), "kind": res.Block.Kind,
		"starts_at": res.Block.StartsAt, "ends_at": res.Block.EndsAt, "reason": res.Block.Reason,
		"affected": len(res.Affected),
	}
	if res.StatusChanged {
		payload["vehicle_status"] = res.VehicleStatus
	}
	h.record(r, action, model.ResourceVehicles, vehicleID, payload)
	h.publish(r, model.ResourceBlocks, resourceevents.ActionUpdated, vehicleID)
	h.publish(r, model.ResourceVehicles, resourceevents.ActionUpdated, vehicleID)
}
