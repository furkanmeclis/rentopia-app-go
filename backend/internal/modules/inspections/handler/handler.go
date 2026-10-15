// Package handler is the HTTP layer of vehicle inspections. Every tenant
// handler runs behind RequireOrganization and reads the organization from
// orgctx; path uuids are resolved before body validation, so another
// organization's uuid answers 404. The public media route serves an
// object only for a valid, unexpired signed token.
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	contractshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/handler"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/usecase"
	rentalhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/handler"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeIncomplete answers a completion of a form that is not complete
// (RT-A17).
const CodeIncomplete = "INSPECTION_INCOMPLETE"

// Handler serves /v1/tenant/inspections, /v1/tenant/inspection-templates
// and the public media links.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func (h *Handler) audit(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.ResourceInspections, &id, payload, r)
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

// ------------------------------------------------------------ templates

// ListTemplates serves GET /v1/tenant/inspection-templates.
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, map[string]string{"version": "version", "created_at": "created_at"})
	if !ok {
		return
	}
	sortField, desc := "version", true
	if len(q.Sort) > 0 {
		sortField, desc = q.Sort[0].Field, q.Sort[0].Desc
	}
	statuses := []string{}
	for _, v := range strings.Split(joinValues(r.URL.Query(), "status"), ",") {
		if v == "active" || v == "archived" {
			statuses = append(statuses, v)
		}
	}
	items, total, err := h.svc.ListTemplates(r.Context(), orgID(r), q.Q, statuses, sortField, desc, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ActiveTemplate serves GET /v1/tenant/inspection-templates/active.
func (h *Handler) ActiveTemplate(w http.ResponseWriter, r *http.Request) {
	t, _, err := h.svc.ActiveTemplate(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// GetTemplate serves GET /v1/tenant/inspection-templates/{uuid}.
func (h *Handler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	t, err := h.svc.GetTemplate(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// SaveTemplate serves POST /v1/tenant/inspection-templates.
func (h *Handler) SaveTemplate(w http.ResponseWriter, r *http.Request) {
	var in model.TemplateInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	t, err := h.svc.SaveTemplate(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		p := authctx.MustPrincipal(r.Context())
		h.activity.Record(r.Context(), &p.UserInternal, "inspection_templates.saved", model.ResourceTemplates, &t.UUID,
			map[string]any{"version": t.Version}, r)
	}
	response.JSON(w, r, http.StatusCreated, t)
}

// ---------------------------------------------------------- inspections

// Facets serves GET /v1/tenant/inspections/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/inspections (the review queue is
// ?review_status=pending).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, model.Sortable)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.QueryKeys {
		filters[key] = joinValues(r.URL.Query(), key)
	}
	f := model.FilterFromQuery(filters)
	f.Limit, f.Offset = q.Limit, q.Offset
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Start serves POST /v1/tenant/inspections.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	var in model.StartInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, created, err := h.svc.Start(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		h.audit(r, "inspections.started", out.UUID, map[string]any{"kind": out.Kind, "rental_uuid": out.Rental.UUID})
	}
	response.JSON(w, r, status, out)
}

// Get serves GET /v1/tenant/inspections/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Charges serves GET /v1/tenant/inspections/{uuid}/charges.
func (h *Handler) Charges(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Charges(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// action decodes the body of an action on a form, runs it and audits it.
func action[T any](h *Handler, w http.ResponseWriter, r *http.Request, name string, optional bool, fn func(id uuid.UUID, in T) (model.Inspection, error)) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in T
	if !decodeJSON(w, r, &in, optional) {
		return
	}
	out, err := fn(id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if name != "" {
		h.audit(r, "inspections."+name, id, map[string]any{"status": out.Status, "kind": out.Kind, "version": out.Version})
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Update serves PATCH /v1/tenant/inspections/{uuid}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "", true, func(id uuid.UUID, in model.DraftInput) (model.Inspection, error) {
		return h.svc.UpdateDraft(r.Context(), orgID(r), id, in)
	})
}

// Complete serves POST /v1/tenant/inspections/{uuid}/complete.
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "finalized", true, func(id uuid.UUID, in model.CompleteInput) (model.Inspection, error) {
		return h.svc.Complete(r.Context(), orgID(r), id, in)
	})
}

// Revise serves POST /v1/tenant/inspections/{uuid}/revise.
func (h *Handler) Revise(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "revised", true, func(id uuid.UUID, in model.ReasonInput) (model.Inspection, error) {
		return h.svc.Revise(r.Context(), orgID(r), id, in.Reason)
	})
}

// Review serves POST /v1/tenant/inspections/{uuid}/review.
func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "reviewed", true, func(id uuid.UUID, in model.ReviewInput) (model.Inspection, error) {
		return h.svc.Review(r.Context(), orgID(r), id, in)
	})
}

// Signature serves POST /v1/tenant/inspections/{uuid}/signature.
func (h *Handler) Signature(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.SignatureInput
	if !decodeJSONLimit(w, r, &in, true, 4<<20) {
		return
	}
	out, err := h.svc.SaveSignature(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"signer_role": in.SignerRole}
	if out.Signature != nil && out.Signature.ContractApproval != nil {
		payload["contract_approval_uuid"] = out.Signature.ContractApproval.ApprovalUUID
	}
	h.audit(r, "inspections.signed", id, payload)
	response.JSON(w, r, http.StatusOK, out)
}

// MediaLinks serves POST /v1/tenant/inspections/{uuid}/media-links.
func (h *Handler) MediaLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.MediaLinksInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.MediaLinks(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// AddDamage serves POST /v1/tenant/inspections/{uuid}/damages.
func (h *Handler) AddDamage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.DamageInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, damage, err := h.svc.AddDamage(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "inspections.damage_added", id, map[string]any{"damage_uuid": damage, "zone": in.Zone})
	response.JSON(w, r, http.StatusCreated, out)
}

// UpdateDamage serves PATCH /v1/tenant/inspections/{uuid}/damages/{damageUuid}.
func (h *Handler) UpdateDamage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	damage, ok := pathUUID(w, r, "damageUuid")
	if !ok {
		return
	}
	var in model.DamageInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.UpdateDamage(r.Context(), orgID(r), id, damage, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteDamage serves DELETE /v1/tenant/inspections/{uuid}/damages/{damageUuid}.
func (h *Handler) DeleteDamage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	damage, ok := pathUUID(w, r, "damageUuid")
	if !ok {
		return
	}
	out, err := h.svc.DeleteDamage(r.Context(), orgID(r), id, damage)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "inspections.damage_removed", id, map[string]any{"damage_uuid": damage})
	response.JSON(w, r, http.StatusOK, out)
}

// AddPhoto serves POST /v1/tenant/inspections/{uuid}/photos.
func (h *Handler) AddPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.PhotoInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, created, err := h.svc.AddPhoto(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	response.JSON(w, r, status, out)
}

// UploadPhoto serves PUT /v1/tenant/inspections/{uuid}/photos/{photoUuid}/file
// (multipart field "file"; the API-issued upload address).
func (h *Handler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	photo, ok := pathUUID(w, r, "photoUuid")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, model.MaxPhotoBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "a photo is at most 15 MiB", Code: "too_large"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	out, err := h.svc.UploadPhoto(r.Context(), orgID(r), id, photo, usecase.FileUpload{Body: file, Size: header.Size})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CompletePhoto serves POST /v1/tenant/inspections/{uuid}/photos/{photoUuid}/complete.
func (h *Handler) CompletePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	photo, ok := pathUUID(w, r, "photoUuid")
	if !ok {
		return
	}
	out, err := h.svc.CompletePhoto(r.Context(), orgID(r), id, photo)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "inspections.photo_completed", id, map[string]any{"photo_uuid": photo, "slot": out.Slot, "sha256": out.SHA256})
	response.JSON(w, r, http.StatusOK, out)
}

// DeletePhoto serves DELETE /v1/tenant/inspections/{uuid}/photos/{photoUuid}.
func (h *Handler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	photo, ok := pathUUID(w, r, "photoUuid")
	if !ok {
		return
	}
	out, err := h.svc.DeletePhoto(r.Context(), orgID(r), id, photo)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Media serves GET /v1/public/media/inspections/{token}: the object of a
// signed, unexpired media link (private, never cached by shared caches).
func (h *Handler) Media(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.OpenMedia(r.Context(), r.PathValue("token"))
	if err != nil {
		response.NotFound(w, r, "Media link is invalid or expired")
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Disposition", "inline")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// ---------------------------------------------------------------- helpers

func listQuery(w http.ResponseWriter, r *http.Request, sortable map[string]string) (apiquery.Query, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortable)); err != nil {
		writeError(w, r, err)
		return apiquery.Query{}, false
	}
	return q, true
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

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, optional bool) bool {
	return decodeJSONLimit(w, r, dst, optional, 1<<20)
}

// decodeJSONLimit decodes a strict JSON body; optional bodies may be empty.
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, optional bool, limit int64) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	if optional && len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func details(ve []usecase.Detail) []response.Detail {
	out := make([]response.Detail, 0, len(ve))
	for _, d := range ve {
		out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
	}
	return out
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		out := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, details(ve.Details))
		return
	}
	var se *usecase.StateError
	if errors.As(err, &se) {
		code := rentalhandler.CodeInvalidState
		if se.Code == "inspection_incomplete" || se.Code == "signature_required" {
			code = CodeIncomplete
		}
		response.ErrorWithData(w, r, http.StatusConflict, code, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}}, se.Data)
		return
	}
	var cc *contractsusecase.ContentChangedError
	if errors.As(err, &cc) {
		response.ErrorWithData(w, r, http.StatusConflict, contractshandler.CodeContentChanged, cc.Error(),
			[]response.Detail{{Field: "contract.content_hash", Code: "content_changed", Message: cc.Error()}},
			map[string]any{"current_version_uuid": cc.Current})
		return
	}
	// Errors of the rentals use case the completion ran.
	var rve *rentalusecase.ValidationError
	if errors.As(err, &rve) {
		out := make([]response.Detail, 0, len(rve.Details))
		for _, d := range rve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var rse *rentalusecase.StateError
	if errors.As(err, &rse) {
		response.ErrorWithDetails(w, r, http.StatusConflict, rentalhandler.CodeInvalidState, rse.Message, []response.Detail{{Code: rse.Code, Message: rse.Message}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Inspection was not found")
	case errors.Is(err, usecase.ErrTemplateNotFound):
		response.NotFound(w, r, "Inspection template was not found")
	case errors.Is(err, usecase.ErrDamageNotFound):
		response.NotFound(w, r, "Damage was not found")
	case errors.Is(err, usecase.ErrPhotoNotFound):
		response.NotFound(w, r, "Photo was not found")
	case errors.Is(err, usecase.ErrRentalNotFound), errors.Is(err, rentalusecase.ErrNotFound):
		response.NotFound(w, r, "Rental was not found")
	case errors.Is(err, contractsusecase.ErrDocumentNotFound), errors.Is(err, contractsusecase.ErrVersionNotFound):
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, "Contract was not found",
			[]response.Detail{{Field: "contract.document_uuid", Code: "not_found", Message: "contract document was not found"}})
	case errors.Is(err, usecase.ErrForbidden), errors.Is(err, rentalusecase.ErrForbidden), errors.Is(err, contractsusecase.ErrForbidden):
		response.Forbidden(w, r, "Missing permission for this action")
	case errors.Is(err, usecase.ErrConcurrent), errors.Is(err, rentalusecase.ErrConcurrent):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, "STORAGE_UNAVAILABLE", "Storage is not configured")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
