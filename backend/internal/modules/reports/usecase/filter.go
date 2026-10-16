package usecase

import (
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Query keys of each report (list query and export query).
var (
	OccupancyKeys     = []string{"from", "to", "branch", "rental_group", "group_by"}
	IncomeExpenseKeys = []string{"from", "to", "branch", "group_by"}
	ChannelKeys       = []string{"from", "to", "branch", "date_basis"}
	AgingKeys         = []string{"as_of", "branch", "party"}
	BranchKeys        = []string{"from", "to", "branch", "basis", "allocation"}
)

// ParseFilter reads every report filter from query values (the HTTP query
// or an export job's query). Unknown keys are ignored; each report uses
// its own keys and validates the enumerations it knows.
func ParseFilter(get func(string) string) (model.Filter, error) {
	f := model.Filter{
		GroupBy:    strings.TrimSpace(get("group_by")),
		DateBasis:  strings.TrimSpace(get("date_basis")),
		Party:      strings.TrimSpace(get("party")),
		Basis:      strings.TrimSpace(get("basis")),
		Allocation: strings.TrimSpace(get("allocation")),
	}
	ve := &ValidationError{}
	date := func(key string) *time.Time {
		raw := strings.TrimSpace(get(key))
		if raw == "" {
			return nil
		}
		t, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			ve.Details = append(ve.Details, FieldError{Field: key, Code: "invalid", Message: key + " must be YYYY-MM-DD"})
			return nil
		}
		return &t
	}
	f.From, f.To, f.AsOf = date("from"), date("to"), date("as_of")
	ids := func(key string) []uuid.UUID {
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
	f.BranchUUIDs = ids("branch")
	f.RentalGroupUUIDs = ids("rental_group")
	enum := func(field, value string, set []string) {
		if value != "" && !contains(set, value) {
			ve.Details = append(ve.Details, FieldError{Field: field, Code: "invalid", Message: field + " must be one of " + strings.Join(set, ", ")})
		}
	}
	enum("date_basis", f.DateBasis, model.DateBases)
	enum("party", f.Party, model.Parties)
	enum("basis", f.Basis, model.Bases)
	enum("allocation", f.Allocation, model.Allocations)
	if f.GroupBy != "" && !contains(model.OccupancyGroups, f.GroupBy) && !contains(model.IncomeExpenseGroups, f.GroupBy) {
		ve.Details = append(ve.Details, FieldError{Field: "group_by", Code: "invalid", Message: "group_by is not a known grouping"})
	}
	if len(ve.Details) > 0 {
		return f, ve
	}
	return f, nil
}

// groupBy validates a report's grouping (default first).
func groupBy(value string, set []string) (string, error) {
	if value == "" {
		return set[0], nil
	}
	if !contains(set, value) {
		return "", invalid("group_by", "invalid", "group_by must be one of "+strings.Join(set, ", "))
	}
	return value, nil
}
