package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Calendar serves GET /v1/tenant/fleet/calendar?from=&to=&limit=&offset=&q=
// &status=&home_branch=&current_branch=&rental_group=&segment=&vehicle=
// &is_active=: a page of vehicles (plate order) with their busy intervals
// and maintenance days in [from, to) (RFC 3339, at most 93 days).
func (h *Handler) Calendar(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	details := []response.Detail{}
	parseTime := func(key string) time.Time {
		raw := strings.TrimSpace(values.Get(key))
		if raw == "" {
			return time.Time{}
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, response.Detail{Field: key, Message: key + " must be an RFC 3339 instant", Code: "invalid"})
			return time.Time{}
		}
		return t
	}
	parseInt := func(key string, def int32) int32 {
		raw := strings.TrimSpace(values.Get(key))
		if raw == "" {
			return def
		}
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || n < 0 {
			details = append(details, response.Detail{Field: key, Message: key + " must be a non-negative integer", Code: "invalid"})
			return def
		}
		return int32(n)
	}

	filters := map[string]string{"q": values.Get("q")}
	for _, key := range model.CalendarQueryKeys {
		filters[key] = joinValues(values, key)
	}
	f := model.CalendarFilterFromQuery(filters)
	if filters["vehicle"] != "" && len(f.VehicleUUIDs) == 0 {
		details = append(details, response.Detail{Field: "vehicle", Message: "vehicle must be a uuid list", Code: "invalid"})
	}
	f.From, f.To = parseTime("from"), parseTime("to")
	f.Limit = parseInt("limit", model.CalendarDefaultLimit)
	f.Offset = parseInt("offset", 0)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}

	out, err := h.svc.Calendar(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
