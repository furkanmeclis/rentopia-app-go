// Package handler is the HTTP layer of the customers tenant resource. Every
// handler runs behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
//
// Activity payloads never carry personal values: they name the changed
// fields, masked hints and the reveal / anonymization reason only.
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Activity actions of the customers resource.
const (
	ActionCreated            = "customers.created"
	ActionUpdated            = "customers.updated"
	ActionDeleted            = "customers.deleted"
	ActionRevealed           = "customers.revealed"
	ActionAnonymized         = "customers.anonymized"
	ActionSensitiveExport    = "customers.sensitive_export_requested"
	ActionContactCreated     = "customers.contact_created"
	ActionContactUpdated     = "customers.contact_updated"
	ActionContactDeleted     = "customers.contact_deleted"
	ActionConsentGranted     = "customers.consent_granted"
	ActionConsentWithdrawn   = "customers.consent_withdrawn"
	ActionDriverCreated      = "customers.driver_created"
	ActionDriverUpdated      = "customers.driver_updated"
	ActionDriverDeleted      = "customers.driver_deleted"
	ActionDocumentCreated    = "customers.document_created"
	ActionDocumentDeleted    = "customers.document_deleted"
	ActionDocumentUploaded   = "customers.document_file_uploaded"
	ActionDocumentDownloaded = "customers.document_downloaded"
)

// Handler serves /v1/tenant/customers.
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
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// Meta serves GET /v1/tenant/customers/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/customers/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	facets, err := h.svc.Facets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// List serves GET /v1/tenant/customers.
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

// Create serves POST /v1/tenant/customers.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.CustomerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionCreated, c.UUID, map[string]any{"customer_type": c.CustomerType, "identity": c.IdentityMasked})
	response.JSON(w, r, http.StatusCreated, c)
}

// Get serves GET /v1/tenant/customers/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	c, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// Patch serves PATCH /v1/tenant/customers/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CustomerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionUpdated, c.UUID, map[string]any{"fields": changedFields(in)})
	response.JSON(w, r, http.StatusOK, c)
}

// Delete serves DELETE /v1/tenant/customers/{uuid} (soft delete).
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
	h.record(r, ActionDeleted, id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Reveal serves POST /v1/tenant/customers/{uuid}/reveal: the plaintext
// identity, licence and contact values of the customer, its contacts and
// drivers. Needs tenant.customers.reveal (route) and a reason; every reveal
// is an activity event.
func (h *Handler) Reveal(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.RevealInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.Reveal(r.Context(), scope.ID, id, in.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	reason, _ := usecase.CheckReason(in.Reason)
	h.audit(r, ActionRevealed, id, map[string]any{
		"reason": reason, "drivers": len(out.Drivers), "contacts": len(out.Contacts),
	})
	w.Header().Set("Cache-Control", "private, no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// Anonymize serves POST /v1/tenant/customers/{uuid}/anonymize (data subject
// request; irreversible).
func (h *Handler) Anonymize(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.AnonymizeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.Anonymize(r.Context(), scope.ID, id, in.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	reason, _ := usecase.CheckReason(in.Reason)
	h.record(r, ActionAnonymized, id, map[string]any{
		"reason": reason, "drivers": res.Drivers, "documents": res.Documents, "contacts": res.Contacts,
	})
	c, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// Activity serves GET /v1/tenant/customers/{uuid}/activity.
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

// ExportGuard wraps the shared export handler of the resource: an export
// with query.include_sensitive = "true" needs tenant.customers.reveal (403
// otherwise) and is recorded as an activity event before the job starts.
// The adapter checks the permission again in the job scope.
func (h *Handler) ExportGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
			return
		}
		var body struct {
			Format string            `json:"format"`
			Query  map[string]string `json:"query"`
		}
		_ = json.Unmarshal(raw, &body)
		if strings.EqualFold(strings.TrimSpace(body.Query["include_sensitive"]), "true") {
			scope := orgctx.Must(r.Context())
			if !scope.Has(rbac.PermTenantCustomersReveal) {
				response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantCustomersReveal)
				return
			}
			filters := map[string]string{}
			for k, v := range body.Query {
				if k != "include_sensitive" && v != "" {
					filters[k] = v
				}
			}
			h.audit(r, ActionSensitiveExport, uuid.Nil, map[string]any{"format": body.Format, "filters": filters})
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		next(w, r)
	}
}

// --- contacts ---------------------------------------------------------------------

// ListContacts serves GET /{uuid}/contacts.
func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.ListContacts(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateContact serves POST /{uuid}/contacts.
func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ContactInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.CreateContact(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionContactCreated, id, map[string]any{"contact_uuid": c.UUID.String()})
	response.JSON(w, r, http.StatusCreated, c)
}

// PatchContact serves PATCH /{uuid}/contacts/{contactUuid}.
func (h *Handler) PatchContact(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, contactID, ok := pathUUIDs(w, r, "contactUuid")
	if !ok {
		return
	}
	var in model.ContactInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.PatchContact(r.Context(), scope.ID, id, contactID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionContactUpdated, id, map[string]any{"contact_uuid": c.UUID.String()})
	response.JSON(w, r, http.StatusOK, c)
}

// DeleteContact serves DELETE /{uuid}/contacts/{contactUuid}.
func (h *Handler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, contactID, ok := pathUUIDs(w, r, "contactUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteContact(r.Context(), scope.ID, id, contactID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionContactDeleted, id, map[string]any{"contact_uuid": contactID.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- consents ---------------------------------------------------------------------

// Consents serves GET /{uuid}/consents.
func (h *Handler) Consents(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	view, err := h.svc.Consents(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, view)
}

// RecordConsent serves POST /{uuid}/consents.
func (h *Handler) RecordConsent(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ConsentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.RecordConsent(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	action := ActionConsentGranted
	if res.Action == "withdraw" {
		action = ActionConsentWithdrawn
	}
	h.record(r, action, id, map[string]any{
		"channel": res.Channel, "source": strings.TrimSpace(in.Source), "text_version": strings.TrimSpace(in.TextVersion),
	})
	response.JSON(w, r, http.StatusOK, res.View)
}

// --- drivers ----------------------------------------------------------------------

// ListDrivers serves GET /{uuid}/drivers.
func (h *Handler) ListDrivers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.ListDrivers(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GetDriver serves GET /{uuid}/drivers/{driverUuid}.
func (h *Handler) GetDriver(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, driverID, ok := pathUUIDs(w, r, "driverUuid")
	if !ok {
		return
	}
	d, err := h.svc.GetDriver(r.Context(), scope.ID, id, driverID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// CreateDriver serves POST /{uuid}/drivers.
func (h *Handler) CreateDriver(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.DriverInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.CreateDriver(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionDriverCreated, id, map[string]any{
		"driver_uuid": res.Driver.UUID.String(), "licence": res.Driver.LicenceMasked, "customer_self": res.Driver.IsCustomerSelf,
	})
	response.JSON(w, r, http.StatusCreated, res.Driver)
}

// PatchDriver serves PATCH /{uuid}/drivers/{driverUuid}.
func (h *Handler) PatchDriver(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, driverID, ok := pathUUIDs(w, r, "driverUuid")
	if !ok {
		return
	}
	var in model.DriverInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.PatchDriver(r.Context(), scope.ID, id, driverID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionDriverUpdated, id, map[string]any{"driver_uuid": driverID.String(), "fields": driverFields(in)})
	response.JSON(w, r, http.StatusOK, res.Driver)
}

// DeleteDriver serves DELETE /{uuid}/drivers/{driverUuid}.
func (h *Handler) DeleteDriver(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, driverID, ok := pathUUIDs(w, r, "driverUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteDriver(r.Context(), scope.ID, id, driverID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionDriverDeleted, id, map[string]any{"driver_uuid": driverID.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// DriverEligibility serves GET /{uuid}/drivers/{driverUuid}/eligibility
// (?min_driver_age=&min_license_years=&at=RFC3339): the rental condition
// check reservations use.
func (h *Handler) DriverEligibility(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, driverID, ok := pathUUIDs(w, r, "driverUuid")
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	intParam := func(key string) int {
		raw := strings.TrimSpace(values.Get(key))
		if raw == "" {
			return 0
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 120 {
			details = append(details, response.Detail{Field: key, Code: "invalid", Message: key + " must be between 0 and 120"})
			return 0
		}
		return n
	}
	cond := model.Conditions{MinDriverAge: intParam("min_driver_age"), MinLicenseYears: intParam("min_license_years")}
	at := time.Now()
	if raw := strings.TrimSpace(values.Get("at")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, response.Detail{Field: "at", Code: "invalid", Message: "at must be an RFC 3339 time"})
		} else {
			at = t
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	out, err := h.svc.DriverEligibility(r.Context(), scope.ID, id, driverID, cond, at)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// --- documents --------------------------------------------------------------------

// ListDocuments serves GET /{uuid}/documents.
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

// CreateDocument serves POST /{uuid}/documents.
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
	h.record(r, ActionDocumentCreated, id, map[string]any{"document_uuid": res.Document.UUID.String(), "doc_type": res.Document.DocType})
	response.JSON(w, r, http.StatusCreated, res.Document)
}

// DeleteDocument serves DELETE /{uuid}/documents/{docUuid}.
func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, docID, ok := pathUUIDs(w, r, "docUuid")
	if !ok {
		return
	}
	res, err := h.svc.DeleteDocument(r.Context(), scope.ID, id, docID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, ActionDocumentDeleted, id, map[string]any{"document_uuid": docID.String(), "doc_type": res.Document.DocType})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// UploadDocumentFile serves PUT /{uuid}/documents/{docUuid}/file (multipart
// "file").
func (h *Handler) UploadDocumentFile(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, docID, ok := pathUUIDs(w, r, "docUuid")
	if !ok {
		return
	}
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
	h.record(r, ActionDocumentUploaded, id, map[string]any{"document_uuid": docID.String(), "doc_type": res.Document.DocType})
	response.JSON(w, r, http.StatusOK, res.Document)
}

// DownloadDocumentFile serves GET /{uuid}/documents/{docUuid}/file, streamed
// by the API (never a storage URL). The route needs tenant.document.download
// and tenant.customers.reveal; every download is an activity event.
func (h *Handler) DownloadDocumentFile(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, docID, ok := pathUUIDs(w, r, "docUuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenDocumentFile(r.Context(), scope.ID, id, docID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	h.audit(r, ActionDocumentDownloaded, id, map[string]any{"document_uuid": docID.String(), "doc_type": f.Document.DocType})
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// --- helpers ----------------------------------------------------------------------

// record publishes the live list change and writes the activity event.
func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	h.publish(r, action, id)
	h.audit(r, action, id, payload)
}

// audit writes an activity event without a list change (reveal, download).
// A nil id records an organization-level event (sensitive export).
func (h *Handler) audit(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	var ref *uuid.UUID
	if id != uuid.Nil {
		ref = &id
	}
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, ref, payload, r)
}

func (h *Handler) publish(r *http.Request, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	ev := resourceevents.ActionUpdated
	switch action {
	case ActionCreated:
		ev = resourceevents.ActionCreated
	case ActionDeleted:
		ev = resourceevents.ActionDeleted
	}
	h.events.Publish(r.Context(), model.Resource, ev, id.String())
}

func changedFields(in model.CustomerInput) []string {
	out := []string{}
	add := func(set bool, name string) {
		if set {
			out = append(out, name)
		}
	}
	add(in.CustomerType != nil, "customer_type")
	add(in.FirstName != nil, "first_name")
	add(in.LastName != nil, "last_name")
	add(in.CompanyTitle != nil, "company_title")
	add(in.IdentityType.Set, "identity_type")
	add(in.IdentityNumber != nil, "identity_number")
	add(in.Nationality.Set, "nationality")
	add(in.TaxOffice != nil, "tax_office")
	add(in.Phone != nil, "phone")
	add(in.Email != nil, "email")
	add(in.BillingAddress != nil, "billing_address")
	add(in.BillingCity != nil, "billing_city")
	add(in.BillingDistrict != nil, "billing_district")
	add(in.BillingPostalCode != nil, "billing_postal_code")
	add(in.BillingCountry.Set, "billing_country")
	add(in.EInvoice != nil, "e_invoice")
	add(in.Note != nil, "note")
	add(in.Tags != nil, "tags")
	return out
}

func driverFields(in model.DriverInput) []string {
	out := []string{}
	add := func(set bool, name string) {
		if set {
			out = append(out, name)
		}
	}
	add(in.FirstName != nil, "first_name")
	add(in.LastName != nil, "last_name")
	add(in.BirthDate.Set, "birth_date")
	add(in.Phone != nil, "phone")
	add(in.Email != nil, "email")
	add(in.IdentityType.Set, "identity_type")
	add(in.IdentityNumber != nil, "identity_number")
	add(in.Nationality.Set, "nationality")
	add(in.LicenceNumber != nil, "licence_number")
	add(in.LicenceClass != nil, "licence_class")
	add(in.LicenceIssuedOn.Set, "licence_issued_on")
	add(in.LicenceExpiresOn.Set, "licence_expires_on")
	add(in.LicenceCountry.Set, "licence_country")
	add(in.Note != nil, "note")
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
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Customer was not found")
	case errors.Is(err, usecase.ErrContactNotFound):
		response.NotFound(w, r, "Contact was not found")
	case errors.Is(err, usecase.ErrDriverNotFound):
		response.NotFound(w, r, "Driver was not found")
	case errors.Is(err, usecase.ErrDocumentNotFound):
		response.NotFound(w, r, "Document was not found")
	case errors.Is(err, usecase.ErrNoFile):
		response.NotFound(w, r, "The document has no file")
	case errors.Is(err, usecase.ErrIdentityTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "identity_number", Message: err.Error(), Code: "taken",
		}})
	case errors.Is(err, usecase.ErrLicenceTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "licence_number", Message: err.Error(), Code: "taken",
		}})
	case errors.Is(err, usecase.ErrAnonymized):
		response.Conflict(w, r, "ANONYMIZED", err.Error())
	case errors.Is(err, usecase.ErrStorageUnavailable), errors.Is(err, usecase.ErrCipherUnavailable):
		response.ServiceUnavailable(w, r, "UNAVAILABLE", err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
