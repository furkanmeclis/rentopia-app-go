// Package handler is the HTTP layer of contract templates and documents.
// Every handler runs behind RequireOrganization; path uuids are resolved
// before body validation (another organization's uuid answers 404). PDFs
// are streamed by the API only.
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

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Response codes.
const (
	CodeInvalidState   = "INVALID_STATE"
	CodeContentChanged = "CONTENT_CHANGED"
	CodeNoTemplate     = "NO_ACTIVE_TEMPLATE"
	CodePDFNotReady    = "PDF_NOT_READY"
)

// Handler serves /v1/tenant/contract-templates and /v1/tenant/documents.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	limiter  RateLimiter
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func (h *Handler) audit(r *http.Request, resource, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

// ---------------------------------------------------------------- templates

// TemplatesMeta serves GET /v1/tenant/contract-templates/meta.
func (h *Handler) TemplatesMeta(w http.ResponseWriter, r *http.Request) {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "contracts.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	faceted := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	response.JSON(w, r, http.StatusOK, rm.ResourceMeta{
		Resource: model.ResourceTemplates, DefaultSort: "-created_at",
		DefaultFields:    []string{"name", "kind", "version", "status", "activated_at", "created_at"},
		Capabilities:     rm.Capabilities{Create: true, Read: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true},
		SearchableFields: []string{"name"}, SortableFields: []string{"name", "version", "created_at"},
		FilterableFields: []string{"kind", "status"},
		Columns: []rm.Column{
			col("name", s, true, true), faceted(col("kind", e, false, true)), col("version", s, true, true),
			faceted(col("status", e, false, true)), col("placeholders", s, false, false), col("activated_at", dt, false, true),
			col("created_by", s, false, false), col("created_at", dt, true, true),
		},
		Filters: []rm.Filter{
			{Key: "kind", LabelKey: "contracts.columns.kind", Variant: rm.FilterVariantFaceted, EnumLookup: "contracts.kinds"},
			{Key: "status", LabelKey: "contracts.columns.status", Variant: rm.FilterVariantFaceted, EnumLookup: "contracts.template_statuses"},
		},
		Includes: []string{},
	})
}

// Placeholders serves GET /v1/tenant/contract-templates/placeholders.
func (h *Handler) Placeholders(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"groups": model.PlaceholderGroups, "items": model.Catalog})
}

// ListTemplates serves GET /v1/tenant/contract-templates.
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, model.TemplateSortable)
	if !ok {
		return
	}
	v := r.URL.Query()
	f := model.TemplateFilter{
		Q: q.Q, Kinds: model.Allowed(model.SplitList(joinValues(v, "kind")), model.Kinds),
		Statuses: model.Allowed(model.SplitList(joinValues(v, "status")), model.TemplateStatuses),
		Limit:    q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListTemplates(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateTemplate serves POST /v1/tenant/contract-templates.
func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var in model.TemplateInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	t, err := h.svc.CreateTemplate(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceTemplates, "contract_templates.created", t.UUID, map[string]any{"kind": t.Kind, "version": t.Version})
	response.JSON(w, r, http.StatusCreated, t)
}

// Preview serves POST /v1/tenant/contract-templates/preview.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var in model.PreviewInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.Preview(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GetTemplate serves GET /v1/tenant/contract-templates/{uuid}.
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

// UpdateTemplate serves PATCH /v1/tenant/contract-templates/{uuid}.
func (h *Handler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if _, err := h.svc.GetTemplate(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	var in model.TemplateInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	t, err := h.svc.UpdateTemplate(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceTemplates, "contract_templates.updated", id, map[string]any{"version": t.Version})
	response.JSON(w, r, http.StatusOK, t)
}

// DeleteTemplate serves DELETE /v1/tenant/contract-templates/{uuid}.
func (h *Handler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteTemplate(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceTemplates, "contract_templates.deleted", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) templateAction(w http.ResponseWriter, r *http.Request, name string, status int, fn func(id uuid.UUID) (model.Template, error)) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct{}
	if !decodeJSON(w, r, &in, true) {
		return
	}
	t, err := fn(id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceTemplates, "contract_templates."+name, t.UUID, map[string]any{"kind": t.Kind, "version": t.Version})
	response.JSON(w, r, status, t)
}

// ActivateTemplate serves POST …/{uuid}/activate.
func (h *Handler) ActivateTemplate(w http.ResponseWriter, r *http.Request) {
	h.templateAction(w, r, "activated", http.StatusOK, func(id uuid.UUID) (model.Template, error) {
		return h.svc.ActivateTemplate(r.Context(), orgID(r), id)
	})
}

// NewVersion serves POST …/{uuid}/new-version.
func (h *Handler) NewVersion(w http.ResponseWriter, r *http.Request) {
	h.templateAction(w, r, "versioned", http.StatusCreated, func(id uuid.UUID) (model.Template, error) {
		return h.svc.NewVersion(r.Context(), orgID(r), id)
	})
}

// ---------------------------------------------------------------- documents

// ListDocuments serves GET /v1/tenant/documents.
func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, map[string]string{})
	if !ok {
		return
	}
	v := r.URL.Query()
	f := model.DocumentFilter{
		Q: q.Q, Kinds: model.Allowed(model.SplitList(joinValues(v, "kind")), model.Kinds),
		Statuses:         model.Allowed(model.SplitList(joinValues(v, "status")), model.DocumentStatuses),
		ReservationUUIDs: model.UUIDs(joinValues(v, "reservation")), RentalUUIDs: model.UUIDs(joinValues(v, "rental")),
		Limit: q.Limit, Offset: q.Offset,
	}
	items, total, err := h.svc.ListDocuments(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// PrepareDocument serves POST /v1/tenant/documents.
func (h *Handler) PrepareDocument(w http.ResponseWriter, r *http.Request) {
	var in model.PrepareInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.PrepareDocument(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceDocuments, "documents.prepared", out.Document.UUID, map[string]any{"kind": in.Kind, "created": out.Created})
	response.JSON(w, r, http.StatusOK, out)
}

// GetDocument serves GET /v1/tenant/documents/{uuid}.
func (h *Handler) GetDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.GetDocument(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// RefreshDocument serves POST /v1/tenant/documents/{uuid}/refresh.
func (h *Handler) RefreshDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct{}
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.RefreshDocument(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceDocuments, "documents.refreshed", id, map[string]any{"created": out.Created})
	response.JSON(w, r, http.StatusOK, out)
}

// GetVersion serves GET /v1/tenant/documents/{uuid}/versions/{documentVersionUuid}.
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	vid, ok := pathUUID(w, r, "documentVersionUuid")
	if !ok {
		return
	}
	v, err := h.svc.GetVersion(r.Context(), orgID(r), id, vid)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// IssueVersion serves POST …/versions/{documentVersionUuid}/issue.
func (h *Handler) IssueVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	vid, ok := pathUUID(w, r, "documentVersionUuid")
	if !ok {
		return
	}
	var in struct{}
	if !decodeJSON(w, r, &in, true) {
		return
	}
	d, err := h.svc.IssueVersion(r.Context(), orgID(r), id, vid)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceDocuments, "documents.issued", id, map[string]any{"version_uuid": vid})
	response.JSON(w, r, http.StatusOK, d)
}

// ApproveVersion serves POST …/versions/{documentVersionUuid}/approve.
func (h *Handler) ApproveVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	vid, ok := pathUUID(w, r, "documentVersionUuid")
	if !ok {
		return
	}
	var in model.ApproveInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	d, err := h.svc.ApproveVersion(r.Context(), orgID(r), id, vid, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceDocuments, "documents.approved", id, map[string]any{"version_uuid": vid, "content_hash": in.ContentHash})
	response.JSON(w, r, http.StatusOK, d)
}

// DownloadPDF serves GET …/versions/{documentVersionUuid}/pdf (streamed by the API,
// tenant.document.download; every download is an activity event).
func (h *Handler) DownloadPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	vid, ok := pathUUID(w, r, "documentVersionUuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenPDF(r.Context(), orgID(r), id, vid)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	h.audit(r, model.ResourceDocuments, "documents.downloaded", id, map[string]any{"version_uuid": vid, "kind": f.Kind})
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
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
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
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
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var se *usecase.StateError
	if errors.As(err, &se) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}})
		return
	}
	var cc *usecase.ContentChangedError
	if errors.As(err, &cc) {
		response.ErrorWithData(w, r, http.StatusConflict, CodeContentChanged, cc.Error(),
			[]response.Detail{{Field: "content_hash", Code: "content_changed", Message: cc.Error()}},
			map[string]any{"current_version_uuid": cc.Current})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrTemplateNotFound):
		response.NotFound(w, r, "Contract template was not found")
	case errors.Is(err, usecase.ErrDocumentNotFound):
		response.NotFound(w, r, "Document was not found")
	case errors.Is(err, usecase.ErrVersionNotFound):
		response.NotFound(w, r, "Document version was not found")
	case errors.Is(err, usecase.ErrLinkNotFound):
		response.NotFound(w, r, "Approval link was not found")
	case errors.Is(err, usecase.ErrApprovalNotFound):
		response.NotFound(w, r, "Approval was not found")
	case errors.Is(err, usecase.ErrRemoteUnavailable):
		response.ErrorWithDetails(w, r, http.StatusServiceUnavailable, CodeRemoteDisabled, err.Error(),
			[]response.Detail{{Code: "remote_approval_unavailable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrNoSMSAccount):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeSMSAccount, err.Error(),
			[]response.Detail{{Code: "sms_account_required", Message: err.Error()}})
	case errors.Is(err, usecase.ErrSMSRateLimited):
		response.ErrorWithDetails(w, r, http.StatusTooManyRequests, CodeRateLimited, err.Error(),
			[]response.Detail{{Code: "rate_limited", Message: err.Error()}})
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Missing permission for this action")
	case errors.Is(err, usecase.ErrNoTemplate):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNoTemplate, err.Error(), []response.Detail{{Field: "kind", Code: "no_active_template", Message: err.Error()}})
	case errors.Is(err, usecase.ErrPDFNotReady), errors.Is(err, usecase.ErrStorageUnavailable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodePDFNotReady, err.Error(), []response.Detail{{Code: "pdf_not_ready", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
