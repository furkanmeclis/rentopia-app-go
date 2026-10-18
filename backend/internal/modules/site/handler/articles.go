package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

func decodeArticle(w http.ResponseWriter, r *http.Request, dst any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "body", Code: "invalid_json", Message: "invalid, unknown or oversized request fields"}})
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		response.ValidationError(w, r, []response.Detail{{Field: "body", Code: "invalid_json", Message: "one JSON object required"}})
		return false
	}
	return true
}

func (h *Handler) recordArticle(r *http.Request, action string, id uuid.UUID) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.ArticleResource, &id, nil, r)
}

func (h *Handler) CreateArticle(w http.ResponseWriter, r *http.Request) {
	var in model.ArticleInput
	if !decodeArticle(w, r, &in) {
		return
	}
	out, err := h.svc.CreateArticle(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordArticle(r, "site_articles.created", out.UUID)
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) GetArticle(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.GetArticle(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) UpdateArticle(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ArticleInput
	if !decodeArticle(w, r, &in) {
		return
	}
	out, err := h.svc.UpdateArticle(r.Context(), orgctx.Must(r.Context()).ID, id, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordArticle(r, "site_articles.updated", id)
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) DeleteArticle(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if _, err := h.svc.GetArticle(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	n, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "version", Code: "required", Message: "article version query parameter required"}})
		return
	}
	err = h.svc.DeleteArticle(r.Context(), orgctx.Must(r.Context()).ID, id, authctx.MustPrincipal(r.Context()).UserInternal, model.VersionInput{Version: &n})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordArticle(r, "site_articles.deleted", id)
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handler) setArticleStatus(w http.ResponseWriter, r *http.Request, published bool) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.VersionInput
	if !decodeArticle(w, r, &in) {
		return
	}
	out, err := h.svc.SetArticleStatus(r.Context(), orgctx.Must(r.Context()).ID, id, authctx.MustPrincipal(r.Context()).UserInternal, in, published)
	if err != nil {
		writeError(w, r, err)
		return
	}
	action := "site_articles.unpublished"
	if published {
		action = "site_articles.published"
	}
	h.recordArticle(r, action, id)
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) PublishArticle(w http.ResponseWriter, r *http.Request) {
	h.setArticleStatus(w, r, true)
}
func (h *Handler) UnpublishArticle(w http.ResponseWriter, r *http.Request) {
	h.setArticleStatus(w, r, false)
}

func (h *Handler) ListArticles(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	q := apiquery.Parse(v)
	p := db.ListSiteArticlesParams{OrganizationID: orgctx.Must(r.Context()).ID, PageLimit: q.Limit, PageOffset: q.Offset, Q: q.Q,
		Status: v.Get("status"), Locale: v.Get("locale"), SortField: "created_at", SortDesc: true}
	details := []response.Detail{}
	for _, key := range apiquery.ForbiddenParams(v) {
		details = append(details, response.Detail{Field: key, Code: "forbidden", Message: "use limit/offset/q"})
	}
	if p.Status != "" && !slices.Contains([]string{"draft", "published"}, p.Status) {
		details = append(details, response.Detail{Field: "status", Code: "invalid", Message: "draft or published required"})
	}
	if p.Locale != "" && !slices.Contains(model.Locales, p.Locale) {
		details = append(details, response.Detail{Field: "locale", Code: "invalid", Message: "tr or en required"})
	}
	if sort := v.Get("sort"); sort != "" {
		p.SortField = strings.TrimPrefix(sort, "-")
		p.SortDesc = strings.HasPrefix(sort, "-")
		if !slices.Contains([]string{"created_at", "updated_at", "sort"}, p.SortField) {
			details = append(details, response.Detail{Field: "sort", Code: "invalid", Message: "unsupported sort field"})
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	out, err := h.svc.ListArticles(r.Context(), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) ArticlesMeta(w http.ResponseWriter, r *http.Request) {
	columns := []resourcemeta.Column{}
	for _, key := range []string{"title", "status", "slug", "featured", "sort", "published_at", "created_at", "updated_at"} {
		typ := resourcemeta.ColumnTypeString
		if strings.HasSuffix(key, "_at") {
			typ = resourcemeta.ColumnTypeDatetime
		}
		if key == "featured" {
			typ = resourcemeta.ColumnTypeBoolean
		}
		columns = append(columns, resourcemeta.Column{Key: key, LabelKey: "site.articles.columns." + key, Type: typ,
			DefaultVisible: true, Sortable: slices.Contains([]string{"sort", "created_at", "updated_at"}, key)})
	}
	response.JSON(w, r, http.StatusOK, resourcemeta.ResourceMeta{
		Resource: model.ArticleResource, DefaultSort: "-created_at", DefaultFields: []string{"uuid", "title", "slug", "status", "featured", "created_at", "version"},
		Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true},
		Columns:      columns, SearchableFields: []string{"title", "summary", "slug"}, SortableFields: []string{"created_at", "updated_at", "sort"},
		FilterableFields: []string{"status", "locale"}, Filters: []resourcemeta.Filter{
			{Key: "status", LabelKey: "site.articles.columns.status", Variant: resourcemeta.FilterVariantFaceted},
			{Key: "locale", LabelKey: "site.articles.columns.locale", Variant: resourcemeta.FilterVariantFaceted},
		}, Includes: []string{}, BulkActions: []bulkengine.BulkActionDef{},
	})
}

func (h *Handler) PublicArticles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.allow(w, r, "site:articles:ip", 600) {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	out, err := h.svc.PublicArticles(r.Context(), publicSiteHost(r), r.URL.Query().Get("locale"), q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) PublicArticle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.allow(w, r, "site:articles:ip", 600) {
		return
	}
	out, err := h.svc.PublicArticle(r.Context(), publicSiteHost(r), r.PathValue("locale"), r.PathValue("slug"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) Sitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.allow(w, r, "site:sitemap:ip", 120) {
		return
	}
	out, err := h.svc.Sitemap(r.Context(), publicSiteHost(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func publicSiteHost(r *http.Request) string {
	if host := r.URL.Query().Get("host"); host != "" {
		return host
	}
	return r.Host
}
