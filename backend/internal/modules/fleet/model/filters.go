package model

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// List query keys per resource (HTTP list, export and bulk "all matching").
var (
	SegmentQueryKeys = []string{"is_active"}
	GroupQueryKeys   = []string{"is_active", "segment", "transmission", "fuel", "body_type"}
	ProductQueryKeys = []string{"is_active", "segment", "status", "guarantee_type", "rental_group"}
)

// SplitList parses a comma-separated faceted filter value.
func SplitList(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func splitUUIDs(raw string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range SplitList(raw) {
		if id, err := uuid.Parse(v); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func parseActive(raw string) *bool {
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &v
}

// SegmentFilterFromQuery maps list query params (q, is_active).
func SegmentFilterFromQuery(query map[string]string) SegmentFilter {
	return SegmentFilter{Q: strings.TrimSpace(query["q"]), IsActive: parseActive(query["is_active"])}
}

// GroupFilterFromQuery maps list query params (q, is_active, segment,
// transmission, fuel, body_type; multi-value params are comma-separated).
func GroupFilterFromQuery(query map[string]string) RentalGroupFilter {
	return RentalGroupFilter{
		Q: strings.TrimSpace(query["q"]), IsActive: parseActive(query["is_active"]),
		SegmentUUIDs: splitUUIDs(query["segment"]), Transmissions: SplitList(query["transmission"]),
		Fuels: SplitList(query["fuel"]), BodyTypes: SplitList(query["body_type"]),
	}
}

// ProductFilterFromQuery maps list query params (q, is_active, segment,
// status, guarantee_type, rental_group).
func ProductFilterFromQuery(query map[string]string) RentalProductFilter {
	f := RentalProductFilter{
		Q: strings.TrimSpace(query["q"]), IsActive: parseActive(query["is_active"]),
		SegmentUUIDs: splitUUIDs(query["segment"]), Statuses: SplitList(query["status"]),
		GuaranteeTypes: SplitList(query["guarantee_type"]),
	}
	if id, err := uuid.Parse(strings.TrimSpace(query["rental_group"])); err == nil {
		f.RentalGroupUUID = &id
	}
	return f
}
