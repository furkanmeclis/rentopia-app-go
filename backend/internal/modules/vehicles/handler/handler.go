// Package handler is the HTTP layer of the vehicles tenant resource. Every
// handler runs behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeNotEligible is the error code of a rental group membership the
// vehicle does not qualify for (422); details list the failed criteria.
const CodeNotEligible = "NOT_ELIGIBLE"

// CodeMembershipInUse refuses a rental group membership removal (409):
// future commitments on the vehicle rely on it and cannot be re-planned;
// data.allocations lists them.
const CodeMembershipInUse = "MEMBERSHIP_IN_USE"

// blockingAllocation is one entry of data.allocations of a 409
// MEMBERSHIP_IN_USE.
type blockingAllocation struct {
	UUID        uuid.UUID            `json:"uuid"`
	Kind        string               `json:"kind"`
	StartsAt    string               `json:"starts_at"`
	EndsAt      string               `json:"ends_at"`
	Reason      string               `json:"reason"`
	Reservation *blockingReservation `json:"reservation"`
}

type blockingReservation struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
}

func membershipInUse(w http.ResponseWriter, r *http.Request, e *usecase.MembershipInUseError) {
	out := make([]blockingAllocation, 0, len(e.Allocations))
	details := make([]response.Detail, 0, len(e.Allocations))
	for _, a := range e.Allocations {
		b := blockingAllocation{
			UUID: a.UUID, Kind: a.Kind, StartsAt: a.StartsAt.UTC().Format(time.RFC3339),
			EndsAt: a.EndsAt.UTC().Format(time.RFC3339), Reason: a.Reason,
		}
		label := a.UUID.String()
		if a.Reservation != nil {
			b.Reservation = &blockingReservation{UUID: a.Reservation.UUID, Reference: a.Reservation.Reference}
			label = a.Reservation.Reference
		}
		out = append(out, b)
		details = append(details, response.Detail{Field: "allocations", Message: label, Code: a.Reason})
	}
	response.ErrorWithData(w, r, http.StatusConflict, CodeMembershipInUse, e.Error(), details,
		map[string]any{"allocations": out})
}

// Handler serves /v1/tenant/vehicles.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetResourceEvents publishes live list changes of the organization's
// vehicles (optional).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// Meta serves GET /v1/tenant/vehicles/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/vehicles/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	facets, err := h.svc.Facets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// List serves GET /v1/tenant/vehicles.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.SortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.QueryKeys {
		filters[key] = joinValues(values, key)
	}
	f := model.FilterFromQuery(filters)
	f.Limit, f.Offset, f.SortField, f.SortDesc = q.Limit, q.Offset, "created_at", true
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create serves POST /v1/tenant/vehicles.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.VehicleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	v, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.created", v.UUID, map[string]any{"plate": v.Plate, "label": v.Label})
	response.JSON(w, r, http.StatusCreated, v)
}

// Get serves GET /v1/tenant/vehicles/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Patch serves PATCH /v1/tenant/vehicles/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.VehicleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	v, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.updated", v.UUID, map[string]any{"plate": v.Plate, "fields": changedFields(in)})
	response.JSON(w, r, http.StatusOK, v)
}

// Delete serves DELETE /v1/tenant/vehicles/{uuid} (soft delete).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// ChangeStatus serves POST /v1/tenant/vehicles/{uuid}/status.
func (h *Handler) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.StatusInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.ChangeStatus(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if res.Changed {
		h.record(r, "vehicles.status_changed", id, map[string]any{
			"plate": res.Vehicle.Plate, "from": res.FromStatus, "to": res.Vehicle.Status,
			"odometer_km": res.Vehicle.OdometerKm, "note": strings.TrimSpace(in.Note),
		})
	} else {
		h.publish(r, "vehicles.updated", id)
	}
	response.JSON(w, r, http.StatusOK, res.Vehicle)
}

// Events serves GET /v1/tenant/vehicles/{uuid}/events.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Events(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Groups serves GET /v1/tenant/vehicles/{uuid}/rental-groups.
func (h *Handler) Groups(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	view, err := h.svc.Groups(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, view)
}

// AddGroup serves POST /v1/tenant/vehicles/{uuid}/rental-groups.
func (h *Handler) AddGroup(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.GroupMembershipInput
	if !decodeJSON(w, r, &in) {
		return
	}
	m, err := h.svc.AddGroup(r.Context(), scope.ID, id, in.RentalGroupUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.group_added", id, map[string]any{
		"rental_group_uuid": m.Group.UUID.String(), "code": m.Group.Code, "name": m.Group.Name,
	})
	response.JSON(w, r, http.StatusCreated, m)
}

// RemoveGroup serves DELETE /v1/tenant/vehicles/{uuid}/rental-groups/{groupUuid}.
func (h *Handler) RemoveGroup(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	groupID, ok := pathUUID(w, r, "groupUuid")
	if !ok {
		return
	}
	if err := h.svc.RemoveGroup(r.Context(), scope.ID, id, groupID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.group_removed", id, map[string]any{"rental_group_uuid": groupID.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Kasko serves GET /v1/tenant/vehicles/{uuid}/kasko.
func (h *Handler) Kasko(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	view, err := h.svc.Kasko(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, view)
}

// Activity serves GET /v1/tenant/vehicles/{uuid}/activity.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Activity(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ProductVehicles serves GET /v1/tenant/products/{uuid}/eligible-vehicles:
// the vehicles that keep the promise of the product's current version.
func (h *Handler) ProductVehicles(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	out, err := h.svc.ProductVehicles(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GroupMembers serves GET /v1/tenant/rental-groups/{uuid}/members: the
// member vehicles of a rental group with their current eligibility.
func (h *Handler) GroupMembers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	out, err := h.svc.GroupMembers(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// AddEligibleMembers serves POST
// /v1/tenant/rental-groups/{uuid}/members/add-eligible: every active
// vehicle in scope that meets the group's criteria becomes a member.
func (h *Handler) AddEligibleMembers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.AddEligibleMembers(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	for _, v := range out.VehicleUUIDs {
		h.record(r, "vehicles.group_added", v, map[string]any{"rental_group_uuid": id.String(), "source": "add_eligible"})
	}
	response.JSON(w, r, http.StatusOK, out)
}

// record publishes the live list change and writes a fail-soft activity
// event; the recorder stamps the organization from the request scope.
func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	h.publish(r, action, id)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &id, payload, r)
}

// publish tells open vehicle lists of the organization what changed. Status
// and membership changes count as an update. Engine jobs (import / bulk)
// publish their own single event.
func (h *Handler) publish(r *http.Request, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	ev := resourceevents.ActionUpdated
	switch action {
	case "vehicles.created":
		ev = resourceevents.ActionCreated
	case "vehicles.deleted":
		ev = resourceevents.ActionDeleted
	}
	h.events.Publish(r.Context(), model.Resource, ev, id.String())
}

func changedFields(in model.VehicleInput) []string {
	out := []string{}
	add := func(set bool, name string) {
		if set {
			out = append(out, name)
		}
	}
	add(in.Plate != nil, "plate")
	add(in.VIN != nil, "vin")
	add(in.ManualEntry != nil, "manual_entry")
	add(in.CatalogTypeUUID.Set, "catalog_type_uuid")
	add(in.ManualBrand != nil, "manual_brand")
	add(in.ManualModel != nil, "manual_model")
	add(in.ManualType != nil, "manual_type")
	add(in.ModelYear != nil, "model_year")
	add(in.Transmission.Set, "transmission")
	add(in.Fuel.Set, "fuel")
	add(in.BodyType.Set, "body_type")
	add(in.Seats.Set, "seats")
	add(in.Luggage.Set, "luggage")
	add(in.AirConditioning != nil, "air_conditioning")
	add(in.AttributesVerified != nil, "attributes_verified")
	add(in.Color != nil, "color")
	add(in.HomeBranchUUID != nil, "home_branch_uuid")
	add(in.CurrentBranchUUID != nil, "current_branch_uuid")
	add(in.FleetPool != nil, "fleet_pool")
	add(in.Ownership != nil, "ownership")
	add(in.SupplierName != nil, "supplier_name")
	add(in.InServiceDate.Set, "in_service_date")
	add(in.OutOfServiceDate.Set, "out_of_service_date")
	add(in.OdometerKm != nil, "odometer_km")
	add(in.FuelLevel.Set, "fuel_level")
	add(in.Status != nil, "status")
	add(in.IsActive != nil, "is_active")
	return out
}

func joinValues(values url.Values, key string) string {
	parts := []string{}
	for _, v := range values[key] {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ",")
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
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
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
	var mu *usecase.MembershipInUseError
	if errors.As(err, &mu) {
		membershipInUse(w, r, mu)
		return
	}
	if ne, ok := usecase.IsNotEligible(err); ok {
		details := make([]response.Detail, 0, len(ne.Reasons))
		for _, reason := range ne.Reasons {
			details = append(details, response.Detail{Field: "rental_group_uuid", Message: reason, Code: reason})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeNotEligible, ne.Error(), details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Vehicle was not found")
	case errors.Is(err, usecase.ErrGroupNotFound):
		response.NotFound(w, r, "Rental group was not found")
	case errors.Is(err, usecase.ErrMembershipNotFound):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, usecase.ErrProductNotFound):
		response.NotFound(w, r, "Product was not found")
	case errors.Is(err, usecase.ErrPlateTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "plate", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
