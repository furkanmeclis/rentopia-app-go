package usecase

import (
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// QueryKeys are the report filters (list query and export query).
var QueryKeys = []string{"from", "to", "basis", "allocation", "branch", "rental_group", "q"}

// ParseFilter reads the report filters from query values (the HTTP query
// or an export job's query): from / to (YYYY-MM-DD), basis, allocation,
// branch and rental_group (comma separated uuids), q.
func ParseFilter(get func(string) string) (model.Filter, error) {
	f := model.Filter{
		Basis:      strings.TrimSpace(get("basis")),
		Allocation: strings.TrimSpace(get("allocation")),
		Q:          strings.TrimSpace(get("q")),
	}
	ve := &ValidationError{}
	for _, key := range []string{"from", "to"} {
		raw := strings.TrimSpace(get(key))
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			ve.Details = append(ve.Details, FieldError{Field: key, Code: "invalid", Message: key + " must be YYYY-MM-DD"})
			continue
		}
		if key == "from" {
			f.From = &t
		} else {
			f.To = &t
		}
	}
	parse := func(key string) []uuid.UUID {
		var out []uuid.UUID
		for _, raw := range apiquery.SplitCSV(get(key)) {
			id, err := uuid.Parse(raw)
			if err != nil {
				ve.Details = append(ve.Details, FieldError{Field: key, Code: "invalid", Message: key + " must be uuids"})
				return nil
			}
			out = append(out, id)
		}
		return out
	}
	f.BranchUUIDs = parse("branch")
	f.RentalGroupUUIDs = parse("rental_group")
	if f.Basis != "" && !contains(model.Bases, f.Basis) {
		ve.Details = append(ve.Details, FieldError{Field: "basis", Code: "invalid", Message: "basis must be accrual or cash"})
	}
	if f.Allocation != "" && !contains(model.Allocations, f.Allocation) {
		ve.Details = append(ve.Details, FieldError{Field: "allocation", Code: "invalid", Message: "allocation must be rental_days, vehicle_count or revenue"})
	}
	if len(ve.Details) > 0 {
		return f, ve
	}
	return f, nil
}
