// Package handler is the HTTP layer of vehicle documents, maintenance and
// fleet alerts. Every handler runs behind RequireOrganization and reads the
// organization from orgctx.Scope.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/usecase"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the vehicle documents / maintenance routes and
// /v1/tenant/fleet/alerts.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetResourceEvents publishes the vehicle as updated after document and
// maintenance changes (lists show expiry columns).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// --- documents ------------------------------------------------------------------

// ListDocuments serves GET /v1/tenant/vehicles/{uuid}/documents.
func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.ListDocuments(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateDocument serves POST /v1/tenant/vehicles/{uuid}/documents.
func (h *Handler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.DocumentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.CreateDocument(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.document_added", res.VehicleUUID, documentPayload(res.Plate, res.Document), true)
	response.JSON(w, r, http.StatusCreated, res.Document)
}

// PatchDocument serves PATCH /v1/tenant/vehicles/{uuid}/documents/{documentUuid}.
func (h *Handler) PatchDocument(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, docID, ok := pathUUIDs(w, r, "documentUuid")
	if !ok {
		return
	}
	var in model.DocumentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.PatchDocument(r.Context(), scope.ID, id, docID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.document_updated", res.VehicleUUID, documentPayload(res.Plate, res.Document), true)
	response.JSON(w, r, http.StatusOK, res.Document)
}

// DeleteDocument serves DELETE /v1/tenant/vehicles/{uuid}/documents/{documentUuid}.
func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, docID, ok := pathUUIDs(w, r, "documentUuid")
	if !ok {
		return
	}
	res, err := h.svc.DeleteDocument(r.Context(), scope.ID, id, docID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.document_deleted", res.VehicleUUID, documentPayload(res.Plate, res.Document), true)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// UploadDocumentFile serves PUT /v1/tenant/vehicles/{uuid}/documents/{documentUuid}/file
// (multipart "file").
func (h *Handler) UploadDocumentFile(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, docID, ok := pathUUIDs(w, r, "documentUuid")
	if !ok {
		return
	}
	// Cap the whole body: ParseMultipartForm only bounds memory.
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxDocumentFileBytes+(1<<20))
	if err := r.ParseMultipartForm(usecase.MaxDocumentFileBytes); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file must be at most 10 MiB", Code: "too_large"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	res, err := h.svc.UploadDocumentFile(r.Context(), scope.ID, id, docID, usecase.FileUpload{
		Body: file, Size: header.Size, Filename: header.Filename,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.document_file_uploaded", res.VehicleUUID, documentPayload(res.Plate, res.Document), true)
	response.JSON(w, r, http.StatusOK, res.Document)
}

// DownloadDocumentFile serves GET /v1/tenant/vehicles/{uuid}/documents/{documentUuid}/file:
// the file streamed by the API (never a storage URL).
func (h *Handler) DownloadDocumentFile(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, docID, ok := pathUUIDs(w, r, "documentUuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenDocumentFile(r.Context(), scope.ID, id, docID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	h.record(r, "vehicles.document_downloaded", f.VehicleUUID, map[string]any{
		"document_uuid": f.Document.UUID.String(), "doc_type": f.Document.DocType,
	}, false)
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", attachment(f.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// --- maintenance ------------------------------------------------------------------

// ListMaintenance serves GET /v1/tenant/vehicles/{uuid}/maintenance.
func (h *Handler) ListMaintenance(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.ListMaintenance(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateMaintenance serves POST /v1/tenant/vehicles/{uuid}/maintenance.
func (h *Handler) CreateMaintenance(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.MaintenanceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.CreateMaintenance(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	action := "vehicles.maintenance_planned"
	if res.Item.Kind == model.KindDone {
		action = "vehicles.maintenance_recorded"
	}
	h.record(r, action, res.VehicleUUID, maintenancePayload(res.Plate, res.Item), true)
	response.JSON(w, r, http.StatusCreated, res.Item)
}

// PatchMaintenance serves PATCH /v1/tenant/vehicles/{uuid}/maintenance/{itemUuid}.
func (h *Handler) PatchMaintenance(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, itemID, ok := pathUUIDs(w, r, "itemUuid")
	if !ok {
		return
	}
	var in model.MaintenanceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.PatchMaintenance(r.Context(), scope.ID, id, itemID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.maintenance_updated", res.VehicleUUID, maintenancePayload(res.Plate, res.Item), true)
	response.JSON(w, r, http.StatusOK, res.Item)
}

// CompleteMaintenance serves POST /v1/tenant/vehicles/{uuid}/maintenance/{itemUuid}/complete.
func (h *Handler) CompleteMaintenance(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, itemID, ok := pathUUIDs(w, r, "itemUuid")
	if !ok {
		return
	}
	var in model.CompleteInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.CompleteMaintenance(r.Context(), scope.ID, id, itemID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.maintenance_completed", res.VehicleUUID, maintenancePayload(res.Plate, res.Item), true)
	response.JSON(w, r, http.StatusOK, res.Item)
}

// DeleteMaintenance serves DELETE /v1/tenant/vehicles/{uuid}/maintenance/{itemUuid}.
func (h *Handler) DeleteMaintenance(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, itemID, ok := pathUUIDs(w, r, "itemUuid")
	if !ok {
		return
	}
	res, err := h.svc.DeleteMaintenance(r.Context(), scope.ID, id, itemID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicles.maintenance_deleted", res.VehicleUUID, maintenancePayload(res.Plate, res.Item), true)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- alerts -------------------------------------------------------------------------

// Alerts serves GET /v1/tenant/fleet/alerts.
func (h *Handler) Alerts(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	view, err := h.svc.Alerts(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, view)
}

// --- helpers --------------------------------------------------------------------------

// record writes a fail-soft activity entry on the vehicle (resource
// tenant.vehicles, so it shows in the vehicle's activity) and, for
// mutations, tells open vehicle lists that the vehicle changed (expiry
// columns).
func (h *Handler) record(r *http.Request, action string, vehicleID uuid.UUID, payload map[string]any, publish bool) {
	if publish && h.events != nil {
		h.events.Publish(r.Context(), vehiclemodel.Resource, resourceevents.ActionUpdated, vehicleID.String())
	}
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, vehiclemodel.Resource, &vehicleID, payload, r)
}

func documentPayload(plate string, d model.Document) map[string]any {
	return map[string]any{
		"plate": plate, "document_uuid": d.UUID.String(), "doc_type": d.DocType,
		"document_number": d.DocumentNumber, "valid_until": d.ValidUntil,
	}
}

func maintenancePayload(plate string, m model.Maintenance) map[string]any {
	return map[string]any{
		"plate": plate, "maintenance_uuid": m.UUID.String(), "kind": m.Kind, "maintenance_type": m.MaintenanceType,
	}
}

func attachment(filename string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(filename))
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
		return uuid.Nil, false
	}
	return id, true
}

func pathUUIDs(w http.ResponseWriter, r *http.Request, child string) (uuid.UUID, uuid.UUID, bool) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	childID, ok := pathUUID(w, r, child)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return id, childID, true
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
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrVehicleNotFound):
		response.NotFound(w, r, "Vehicle was not found")
	case errors.Is(err, usecase.ErrDocumentNotFound):
		response.NotFound(w, r, "Document was not found")
	case errors.Is(err, usecase.ErrMaintenanceNotFound):
		response.NotFound(w, r, "Maintenance item was not found")
	case errors.Is(err, usecase.ErrNoFile):
		response.NotFound(w, r, "The document has no file")
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, "STORAGE_UNAVAILABLE", err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
