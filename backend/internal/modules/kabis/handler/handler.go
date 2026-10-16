// Package handler is the HTTP layer of /v1/tenant/kabis-notifications
// (TEC-726). Every handler runs behind RequireOrganization and reads the
// organization from orgctx.Scope; path uuids are resolved inside the
// organization and the branch scope first, so another organization's uuid
// answers 404.
package handler

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/usecase"
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
	CodeNotRetryable  = "KABIS_NOT_RETRYABLE"
	CodeNotResolvable = "KABIS_NOT_RESOLVABLE"
	CodeBusy          = "KABIS_BUSY"
	CodeDataMissing   = "KABIS_DATA_MISSING"
	CodeDependency    = "KABIS_DEPENDENCY_PENDING"
	CodeUnverified    = "KABIS_UNVERIFIED"
)

// Activity actions.
const (
	ActionRetried              = "kabis.retried"
	ActionReportedManually     = "kabis.reported_manually"
	ActionReconciled           = "kabis.reconciled"
	ActionAttachmentDownloaded = "kabis.attachment_downloaded"
)

// Handler serves KABİS notifications.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler (rec may be nil).
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func (h *Handler) audit(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &id, payload, r)
}

// Meta is the table contract of GET /v1/tenant/kabis-notifications.
func Meta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "kabis.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	faceted := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:         model.Resource,
		DefaultSort:      "-occurred_at",
		DefaultFields:    []string{"occurred_at", "event", "rental", "plate", "status", "attempts", "branch"},
		Capabilities:     rm.Capabilities{Read: true, Search: true, Filter: true, Sort: true},
		SearchableFields: []string{"plate", "rental"},
		SortableFields:   []string{"occurred_at", "created_at", "updated_at"},
		FilterableFields: model.QueryKeys,
		Columns: []rm.Column{
			col("occurred_at", dt, true, true),
			faceted(col("event", e, false, true)),
			col("rental", s, false, true),
			col("plate", s, false, true),
			faceted(col("status", e, false, true)),
			col("attempts", s, false, true),
			col("branch", s, false, true),
			col("external_reference", s, false, false),
			col("last_verified_at", dt, false, false),
			col("last_error", s, false, false),
			col("created_at", dt, true, false),
			col("updated_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			{Key: "status", LabelKey: "kabis.columns.status", Variant: rm.FilterVariantFaceted, EnumLookup: "kabis.statuses"},
			{Key: "event", LabelKey: "kabis.columns.event", Variant: rm.FilterVariantFaceted, EnumLookup: "kabis.events"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

// MetaH serves GET /v1/tenant/kabis-notifications/meta.
func (h *Handler) MetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/kabis-notifications/facets (status and
// event counts, needs_action = the nav badge).
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/kabis-notifications.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, model.Sortable)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.QueryKeys {
		filters[key] = strings.Join(httpkit.Values(r, key), ",")
	}
	f, err := model.FilterFromQuery(filters)
	if err != nil {
		writeError(w, r, err)
		return
	}
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

// Get serves GET /v1/tenant/kabis-notifications/{uuid}.
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

// Retry serves POST /v1/tenant/kabis-notifications/{uuid}/retry.
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	res, err := h.svc.Retry(r.Context(), orgID(r), id)
	if err != nil {
		h.audit(r, ActionRetried, id, map[string]any{"result": failureCode(err)})
		var ve *jobsusecase.VerifyError
		if errors.As(err, &ve) {
			d, _ := h.svc.Get(r.Context(), orgID(r), id)
			response.ErrorWithData(w, r, http.StatusConflict, CodeUnverified, ve.Error(),
				[]response.Detail{{Code: "unverified", Message: ve.Message}}, d)
			return
		}
		writeError(w, r, err)
		return
	}
	h.audit(r, ActionRetried, id, map[string]any{"result": res.Action, "status": res.Status})
	d, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"action": res.Action, "notification": d})
}

func failureCode(err error) string {
	var dm *usecase.DataMissingError
	var de *usecase.DependencyError
	var ve *jobsusecase.VerifyError
	switch {
	case errors.As(err, &dm):
		return "data_missing"
	case errors.As(err, &de):
		return "dependency_pending"
	case errors.As(err, &ve):
		return "unverified"
	case errors.Is(err, usecase.ErrNotFound):
		return "not_found"
	}
	return "refused"
}

// MarkManual serves POST /v1/tenant/kabis-notifications/{uuid}/mark-manual.
func (h *Handler) MarkManual(w http.ResponseWriter, r *http.Request) {
	h.resolve(w, r, model.ResolutionManual, ActionReportedManually)
}

// Reconcile serves POST /v1/tenant/kabis-notifications/{uuid}/reconcile.
func (h *Handler) Reconcile(w http.ResponseWriter, r *http.Request) {
	h.resolve(w, r, model.ResolutionReconciled, ActionReconciled)
}

// resolve reads multipart/form-data (reason, reference, optional file) or
// a JSON body ({reason, reference}).
func (h *Handler) resolve(w http.ResponseWriter, r *http.Request, resolution, action string) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ResolveInput
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct == "multipart/form-data" {
		r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxAttachmentBytes+(1<<20))
		if err := r.ParseMultipartForm(usecase.MaxAttachmentBytes); err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "the attachment must be at most 10 MiB", Code: "too_large"}})
			return
		}
		in.Reason, in.Reference = r.FormValue("reason"), r.FormValue("reference")
		if file, header, err := r.FormFile("file"); err == nil {
			defer func() { _ = file.Close() }()
			in.File = &model.Upload{Body: file, Size: header.Size, Filename: header.Filename}
		}
	} else {
		var body struct {
			Reason    string `json:"reason"`
			Reference string `json:"reference"`
		}
		if !httpkit.DecodeJSON(w, r, &body, false) {
			return
		}
		in.Reason, in.Reference = body.Reason, body.Reference
	}
	d, err := h.svc.Resolve(r.Context(), orgID(r), id, resolution, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"reason": strings.TrimSpace(in.Reason), "reference": strings.TrimSpace(in.Reference), "status": d.Status}
	if d.Attachment != nil {
		payload["attachment"] = d.Attachment.Name
	}
	h.audit(r, action, id, payload)
	response.JSON(w, r, http.StatusOK, d)
}

// Attachment serves GET /v1/tenant/kabis-notifications/{uuid}/attachment,
// streamed by the API (never a storage URL); every download is audited.
func (h *Handler) Attachment(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenAttachment(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	h.audit(r, ActionAttachmentDownloaded, id, map[string]any{"name": f.Name})
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
	var dm *usecase.DataMissingError
	if errors.As(err, &dm) {
		out := make([]response.Detail, 0, len(dm.Fields))
		for _, f := range dm.Fields {
			out = append(out, response.Detail{Field: f, Code: "missing", Message: f + " is required for the notification"})
		}
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeDataMissing, "required data is missing; nothing was sent", out)
		return
	}
	var de *usecase.DependencyError
	if errors.As(err, &de) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeDependency, de.Error(),
			[]response.Detail{{Code: de.Reason, Message: de.Error()}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "KABİS notification was not found")
	case errors.Is(err, usecase.ErrNoAttachment):
		response.NotFound(w, r, "The notification has no attachment")
	case errors.Is(err, usecase.ErrNotRetryable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNotRetryable, err.Error(),
			[]response.Detail{{Code: "not_retryable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrNotResolvable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNotResolvable, err.Error(),
			[]response.Detail{{Code: "not_resolvable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrBusy):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeBusy, err.Error(),
			[]response.Detail{{Code: "busy", Message: err.Error()}})
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, "STORAGE_UNAVAILABLE", "Object storage is not configured")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
