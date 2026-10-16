// Package handler is the HTTP layer of platform usage and connection
// health (TEC-739): /v1/platform/usage* and /v1/platform/health*.
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves the platform health routes.
type Handler struct {
	svc *usecase.Service
}

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

var (
	usageSort = sortColumns(model.UsageSortFields)
	issueSort = sortColumns(model.IssueSortFields)
	statuses  = map[string]bool{"active": true, "suspended": true}
	severity  = map[string]bool{healthcheck.SeverityCritical: true, healthcheck.SeverityWarning: true, healthcheck.SeverityInfo: true}
)

func sortColumns(fields []string) apiquery.SortColumns {
	out := apiquery.SortColumns{}
	for _, f := range fields {
		out[f] = f
	}
	return out
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Code: ve.Code, Message: ve.Message}})
		return
	}
	var ae *apiquery.ValidationError
	if errors.As(err, &ae) {
		out := make([]response.Detail, 0, len(ae.Details))
		for _, d := range ae.Details {
			out = append(out, response.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, out)
		return
	}
	response.InternalErr(w, r, err, "could not load platform health")
}

func parseList(w http.ResponseWriter, r *http.Request, allowed apiquery.SortColumns) (apiquery.Query, bool) {
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(r.URL.Query())
	if err := apiquery.ValidateSort(q.Sort, allowed); err != nil {
		writeError(w, r, err)
		return apiquery.Query{}, false
	}
	return q, true
}

// csvEnum reads a comma-separated enum filter; allowed nil accepts any
// lowercase code.
func csvEnum(w http.ResponseWriter, r *http.Request, key string, allowed map[string]bool) ([]string, bool) {
	vals := apiquery.SplitCSV(r.URL.Query().Get(key))
	for _, v := range vals {
		ok := allowed[v]
		if allowed == nil {
			ok = v != "" && strings.ToLower(v) == v && len(v) <= 48
		}
		if !ok {
			response.ValidationError(w, r, []response.Detail{{Field: key, Code: "invalid", Message: "unknown value " + v}})
			return nil, false
		}
	}
	return vals, true
}

// UsageMeta serves GET /v1/platform/usage/meta.
func (h *Handler) UsageMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, model.UsageMeta())
}

// ListUsage serves GET /v1/platform/usage.
func (h *Handler) ListUsage(w http.ResponseWriter, r *http.Request) {
	q, ok := parseList(w, r, usageSort)
	if !ok {
		return
	}
	sts, ok := csvEnum(w, r, "status", statuses)
	if !ok {
		return
	}
	start, end, _, err := h.svc.MonthRange(r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	f := model.UsageFilter{
		Q: q.Q, Statuses: sts, MonthStart: start, MonthEnd: end,
		SortField: "active_vehicles", SortDesc: true, Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListUsage(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// UsageSummary serves GET /v1/platform/usage/summary.
func (h *Handler) UsageSummary(w http.ResponseWriter, r *http.Request) {
	start, end, label, err := h.svc.MonthRange(r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.svc.UsageSummary(r.Context(), start, end, label)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Health serves GET /v1/platform/health.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Health(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// IssuesMeta serves GET /v1/platform/health/issues/meta.
func (h *Handler) IssuesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, model.IssuesMeta())
}

// ListIssues serves GET /v1/platform/health/issues.
func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	q, ok := parseList(w, r, issueSort)
	if !ok {
		return
	}
	sources, ok := csvEnum(w, r, "source", nil)
	if !ok {
		return
	}
	sevs, ok := csvEnum(w, r, "severity", severity)
	if !ok {
		return
	}
	f := model.IssueFilter{
		Q: q.Q, Sources: sources, Severities: sevs,
		SortField: "occurred_at", SortDesc: true, Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListIssues(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}
