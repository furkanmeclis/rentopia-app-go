// Package handler is the HTTP layer of /v1/tenant/invoices (TEC-722).
// Every handler runs behind RequireOrganization and reads the organization
// from orgctx.Scope; path uuids are resolved inside the organization and the
// branch scope first, so another organization's uuid answers 404.
package handler

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes.
const (
	CodeNotDraft          = "INVOICE_NOT_DRAFT"
	CodeNothingToInvoice  = "NOTHING_TO_INVOICE"
	CodeAlreadyInvoiced   = "ALREADY_INVOICED"
	CodeDocumentInvalid   = "INVOICE_DOCUMENT_INVALID"
	CodeNotRetryable      = "INVOICE_NOT_RETRYABLE"
	CodeNotCancellable    = "INVOICE_NOT_CANCELLABLE"
	CodeUseReturn         = "USE_RETURN_INVOICE"
	CodeNotReturnable     = "INVOICE_NOT_RETURNABLE"
	CodeBusy              = "INVOICE_BUSY"
	CodeDependency        = "EINVOICE_DEPENDENCY_PENDING"
	CodeUnverified        = "INVOICE_UNVERIFIED"
	CodeNoDocument        = "INVOICE_NO_DOCUMENT"
	CodePDFUnavailable    = "PDF_UNAVAILABLE"
	CodePreviewIncomplete = "INVOICE_PREVIEW_UNAVAILABLE"
)

// Activity actions.
const (
	ActionCreated         = "invoices.created"
	ActionUpdated         = "invoices.updated"
	ActionValidated       = "invoices.validated"
	ActionFinalized       = "invoices.finalized"
	ActionRetried         = "invoices.retried"
	ActionCancelled       = "invoices.cancelled"
	ActionReturnCreated   = "invoices.return_created"
	ActionDownloaded      = "invoices.downloaded"
	ActionSettingsUpdated = "invoices.settings_updated"
)

// QueryKeys are the list filters.
var QueryKeys = []string{"status", "profile", "invoice_type", "branch", "account", "rental", "from", "to", "needs_action"}

// Sortable maps sort fields (apiquery whitelist).
var Sortable = map[string]string{
	"issue_date": "issue_date", "created_at": "created_at", "number": "number", "payable_amount": "payable_amount",
}

// Handler serves invoices.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler (rec may be nil).
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func (h *Handler) audit(r *http.Request, action string, id *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, id, payload, r)
}

// Meta is the table contract of GET /v1/tenant/invoices.
func Meta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "invoices.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	faceted := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:         model.Resource,
		DefaultSort:      "-issue_date",
		DefaultFields:    []string{"number", "issue_date", "receiver", "profile", "status", "payable_amount", "branch"},
		Capabilities:     rm.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true, Export: true, Bulk: true},
		SearchableFields: []string{"number", "receiver", "rental", "account"},
		SortableFields:   []string{"issue_date", "created_at", "number", "payable_amount"},
		FilterableFields: QueryKeys,
		Columns: []rm.Column{
			col("number", s, true, true),
			col("issue_date", dt, true, true),
			col("receiver", s, false, true),
			faceted(col("profile", e, false, true)),
			faceted(col("invoice_type", e, false, false)),
			faceted(col("status", e, false, true)),
			col("payable_amount", s, true, true),
			faceted(col("branch", s, false, true)),
			col("rental", s, false, false),
			col("external_ref", s, false, false),
			col("created_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			{Key: "status", LabelKey: "invoices.columns.status", Variant: rm.FilterVariantFaceted, EnumLookup: "invoices.statuses"},
			{Key: "profile", LabelKey: "invoices.columns.profile", Variant: rm.FilterVariantFaceted, EnumLookup: "invoices.profiles"},
			{Key: "branch", LabelKey: "invoices.columns.branch", Variant: rm.FilterVariantFaceted},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

// MetaH serves GET /v1/tenant/invoices/meta.
func (h *Handler) MetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/invoices/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/invoices.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, Sortable)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range QueryKeys {
		filters[key] = strings.Join(httpkit.Values(r, key), ",")
	}
	f := adapters.Filter(filters)
	f.Limit, f.Offset = q.Limit, q.Offset
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	} else {
		f.SortField, f.SortDesc = "issue_date", true
	}
	items, total, err := h.svc.List(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func queryUUID(w http.ResponseWriter, r *http.Request, key string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: key, Code: "invalid", Message: "must be a uuid"}})
		return nil, false
	}
	return &id, true
}

// Items serves GET /v1/tenant/invoices/items?account=|rental= (the items a
// "Fatura kes" can cover).
func (h *Handler) Items(w http.ResponseWriter, r *http.Request) {
	acc, ok := queryUUID(w, r, "account")
	if !ok {
		return
	}
	rental, ok := queryUUID(w, r, "rental")
	if !ok {
		return
	}
	if acc == nil && rental == nil {
		response.ValidationError(w, r, []response.Detail{{Field: "account", Code: "required", Message: "name an account or a rental"}})
		return
	}
	items, err := h.svc.Items(r.Context(), orgID(r), model.ItemFilter{AccountUUID: acc, RentalUUID: rental})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// UninvoicedRentals serves GET /v1/tenant/invoices/uninvoiced-rentals.
func (h *Handler) UninvoicedRentals(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.UninvoicedRentals(r.Context(), orgID(r), model.RentalFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create serves POST /v1/tenant/invoices (a draft; Idempotency-Key header or
// idempotency_key in the body).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.CreateInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	if in.IdempotencyKey == "" {
		in.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	d, err := h.svc.CreateDraft(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionCreated, &d.UUID, map[string]any{"lines": len(d.Lines), "items": len(d.Links), "profile": d.Profile})
	response.JSON(w, r, http.StatusCreated, d)
}

// Get serves GET /v1/tenant/invoices/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// Patch serves PATCH /v1/tenant/invoices/{uuid} (drafts only).
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.PatchInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	d, err := h.svc.PatchDraft(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	fields := []string{}
	if in.IssueDate != nil {
		fields = append(fields, "issue_date")
	}
	if in.Note != nil {
		fields = append(fields, "note")
	}
	if in.Profile != nil {
		fields = append(fields, "profile")
	}
	if in.Receiver != nil {
		fields = append(fields, "receiver")
	}
	if len(in.Lines) > 0 {
		fields = append(fields, "lines")
	}
	if len(in.RemoveEntryUUIDs) > 0 {
		fields = append(fields, "items_removed")
	}
	h.audit(r, ActionUpdated, &id, map[string]any{"fields": fields})
	response.JSON(w, r, http.StatusOK, d)
}

// Validate serves POST /v1/tenant/invoices/{uuid}/validate.
func (h *Handler) Validate(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	problems, err := h.svc.Validate(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionValidated, &id, map[string]any{"problems": len(problems)})
	response.JSON(w, r, http.StatusOK, map[string]any{"valid": len(problems) == 0, "problems": problems})
}

// Finalize serves POST /v1/tenant/invoices/{uuid}/finalize.
func (h *Handler) Finalize(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.Finalize(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	number := ""
	if d.Number != nil {
		number = *d.Number
	}
	h.audit(r, ActionFinalized, &id, map[string]any{"number": number, "status": d.Status})
	response.JSON(w, r, http.StatusOK, d)
}

// Retry serves POST /v1/tenant/invoices/{uuid}/retry.
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	res, err := h.svc.Retry(r.Context(), orgID(r), id)
	if err != nil {
		var ve *jobsusecase.VerifyError
		if errors.As(err, &ve) {
			h.audit(r, ActionRetried, &id, map[string]any{"result": "unverified"})
			d, _ := h.svc.Get(r.Context(), orgID(r), id)
			response.ErrorWithData(w, r, http.StatusConflict, CodeUnverified, ve.Error(),
				[]response.Detail{{Code: "unverified", Message: ve.Message}}, d)
			return
		}
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionRetried, &id, map[string]any{"result": res.Action, "status": res.Status})
	d, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"action": res.Action, "invoice": d})
}

// Cancel serves POST /v1/tenant/invoices/{uuid}/cancel.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CancelInput
	if !httpkit.DecodeJSON(w, r, &in, true) {
		return
	}
	d, err := h.svc.Cancel(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionCancelled, &id, map[string]any{"reason": strings.TrimSpace(in.Reason), "status": d.Status})
	response.JSON(w, r, http.StatusOK, d)
}

// Return serves POST /v1/tenant/invoices/{uuid}/return (the IADE draft).
func (h *Handler) Return(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ReturnInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	d, err := h.svc.Return(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionReturnCreated, &id, map[string]any{"return_uuid": d.UUID.String(), "reason": strings.TrimSpace(in.Reason)})
	response.JSON(w, r, http.StatusCreated, d)
}

// Preview serves GET /v1/tenant/invoices/{uuid}/preview: the go-ubltr HTML
// render (no scripts; shown in a sandboxed frame).
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	html, err := h.svc.Preview(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

func (h *Handler) file(w http.ResponseWriter, r *http.Request, id uuid.UUID, data []byte, name, contentType string) {
	h.audit(r, ActionDownloaded, &id, map[string]any{"file": name})
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// XML serves GET /v1/tenant/invoices/{uuid}/xml (streamed by the API).
func (h *Handler) XML(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	data, name, err := h.svc.XML(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.file(w, r, id, data, name, "application/xml")
}

// PDF serves GET /v1/tenant/invoices/{uuid}/pdf (streamed by the API).
func (h *Handler) PDF(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	data, name, err := h.svc.PDF(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.file(w, r, id, data, name, "application/pdf")
}

// Settings serves GET /v1/tenant/invoices/settings.
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetSettings(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutSettings serves PUT /v1/tenant/invoices/settings.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in model.SettingsInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.PutSettings(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	kinds := []string{}
	for _, m := range in.TaxMappings {
		kinds = append(kinds, m.LineKind)
	}
	h.audit(r, ActionSettingsUpdated, nil, map[string]any{"tax_mappings": kinds})
	response.JSON(w, r, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var doc *usecase.DocumentError
	if errors.As(err, &doc) {
		out := make([]response.Detail, 0, len(doc.Problems))
		for _, p := range doc.Problems {
			out = append(out, response.Detail{Field: p.Field, Message: p.Message, Code: p.Code})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeDocumentInvalid,
			"the invoice document is not valid; nothing was numbered or sent", out)
		return
	}
	var de *usecase.DependencyError
	if errors.As(err, &de) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeDependency, de.Error(),
			[]response.Detail{{Code: de.Reason, Message: de.Error()}})
		return
	}
	conflict := func(code, detail string) {
		response.ErrorWithDetails(w, r, http.StatusConflict, code, err.Error(), []response.Detail{{Code: detail, Message: err.Error()}})
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Invoice was not found")
	case errors.Is(err, usecase.ErrNotDraft):
		conflict(CodeNotDraft, "not_draft")
	case errors.Is(err, usecase.ErrNothingToInvoice):
		conflict(CodeNothingToInvoice, "nothing_to_invoice")
	case errors.Is(err, usecase.ErrAlreadyInvoiced):
		conflict(CodeAlreadyInvoiced, "already_invoiced")
	case errors.Is(err, usecase.ErrNotRetryable):
		conflict(CodeNotRetryable, "not_retryable")
	case errors.Is(err, usecase.ErrUseReturn):
		conflict(CodeUseReturn, "use_return")
	case errors.Is(err, usecase.ErrNotCancellable):
		conflict(CodeNotCancellable, "not_cancellable")
	case errors.Is(err, usecase.ErrNotReturnable):
		conflict(CodeNotReturnable, "not_returnable")
	case errors.Is(err, usecase.ErrBusy):
		conflict(CodeBusy, "busy")
	case errors.Is(err, usecase.ErrNoDocument):
		conflict(CodeNoDocument, "no_document")
	case errors.Is(err, usecase.ErrPDFUnavailable):
		response.ServiceUnavailable(w, r, CodePDFUnavailable, "The PDF renderer is not available")
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, "STORAGE_UNAVAILABLE", "Object storage is not configured")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
