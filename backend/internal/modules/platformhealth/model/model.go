// Package model holds the DTOs of platform usage and connection health
// (TEC-739).
package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/google/uuid"
)

// Resource names (resourcemeta / DataTable).
const (
	ResourceUsage  = "platform.usage"
	ResourceIssues = "platform.health_issues"
)

// UsageFilter is the usage list query.
type UsageFilter struct {
	Q          string
	Statuses   []string
	MonthStart time.Time
	MonthEnd   time.Time
	SortField  string
	SortDesc   bool
	Limit      int32
	Offset     int32
}

// OrganizationUsage is one row of the usage list.
type OrganizationUsage struct {
	UUID              uuid.UUID  `json:"uuid"`
	Slug              string     `json:"slug"`
	Name              string     `json:"name"`
	Status            string     `json:"status"`
	ActiveVehicles    int64      `json:"active_vehicles"`
	TotalVehicles     int64      `json:"total_vehicles"`
	ReservationsMonth int64      `json:"reservations_month"`
	MemberCount       int64      `json:"member_count"`
	StorageBytes      int64      `json:"storage_bytes"`
	LastLoginAt       *time.Time `json:"last_login_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// UsageSummary holds the platform-wide totals.
type UsageSummary struct {
	Month               string `json:"month"`
	Organizations       int64  `json:"organizations"`
	ActiveOrganizations int64  `json:"active_organizations"`
	ActiveVehicles      int64  `json:"active_vehicles"`
	ReservationsMonth   int64  `json:"reservations_month"`
	Members             int64  `json:"members"`
	StorageBytes        int64  `json:"storage_bytes"`
}

// SourceHealth is one health source on the dashboard.
type SourceHealth struct {
	Code       string               `json:"code"`
	Status     string               `json:"status"`
	Error      string               `json:"error,omitempty"`
	Metrics    []healthcheck.Metric `json:"metrics"`
	IssueCount int                  `json:"issue_count"`
	Critical   int                  `json:"critical"`
	Warning    int                  `json:"warning"`
	Truncated  bool                 `json:"truncated"`
	DurationMs int64                `json:"duration_ms"`
}

// AlertState is the alert sweep's cursor of a source.
type AlertState struct {
	Source         string     `json:"source"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	LastAlertAt    *time.Time `json:"last_alert_at"`
	LastAlertCount int32      `json:"last_alert_count"`
}

// HealthSummary is the dashboard payload.
type HealthSummary struct {
	Status    string         `json:"status"`
	CheckedAt time.Time      `json:"checked_at"`
	Critical  int            `json:"critical"`
	Warning   int            `json:"warning"`
	Sources   []SourceHealth `json:"sources"`
	Alerts    []AlertState   `json:"alerts"`
}

// OrganizationRef names an issue's organization.
type OrganizationRef struct {
	UUID uuid.UUID `json:"uuid"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

// Issue is one row of the issue list.
type Issue struct {
	Key          string           `json:"key"`
	Source       string           `json:"source"`
	Kind         string           `json:"kind"`
	Severity     string           `json:"severity"`
	Organization *OrganizationRef `json:"organization"`
	Provider     string           `json:"provider"`
	Subject      string           `json:"subject"`
	Message      string           `json:"message"`
	RefType      string           `json:"ref_type"`
	RefUUID      string           `json:"ref_uuid"`
	OccurredAt   time.Time        `json:"occurred_at"`
}

// IssueFilter is the issue list query.
type IssueFilter struct {
	Q          string
	Sources    []string
	Severities []string
	SortField  string
	SortDesc   bool
	Limit      int32
	Offset     int32
}

// UsageSortFields are the sortable usage columns.
var UsageSortFields = []string{
	"name", "active_vehicles", "reservations_month", "member_count", "storage_bytes", "last_login_at", "created_at",
}

// IssueSortFields are the sortable issue columns.
var IssueSortFields = []string{"occurred_at", "severity", "source", "organization"}

// UsageMeta describes the usage list.
func UsageMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      ResourceUsage,
		DefaultSort:   "-active_vehicles",
		DefaultFields: []string{"name", "status", "active_vehicles", "reservations_month", "member_count", "storage_bytes", "last_login_at"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"name", "slug"},
		SortableFields:   UsageSortFields,
		FilterableFields: []string{"status", "month"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "platform_health.usage.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "status", LabelKey: "platform_health.usage.columns.status", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "active_vehicles", LabelKey: "platform_health.usage.columns.active_vehicles", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "reservations_month", LabelKey: "platform_health.usage.columns.reservations_month", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "member_count", LabelKey: "platform_health.usage.columns.member_count", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "storage_bytes", LabelKey: "platform_health.usage.columns.storage_bytes", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "last_login_at", LabelKey: "platform_health.usage.columns.last_login_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "created_at", LabelKey: "platform_health.usage.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "status", LabelKey: "platform_health.usage.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "organization_status"},
		},
	}
}

// IssuesMeta describes the issue list.
func IssuesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      ResourceIssues,
		DefaultSort:   "-occurred_at",
		DefaultFields: []string{"severity", "source", "organization", "provider", "subject", "message", "occurred_at"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"organization", "provider", "subject", "message"},
		SortableFields:   IssueSortFields,
		FilterableFields: []string{"source", "severity"},
		Columns: []resourcemeta.Column{
			{Key: "severity", LabelKey: "platform_health.issues.columns.severity", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "source", LabelKey: "platform_health.issues.columns.source", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "organization", LabelKey: "platform_health.issues.columns.organization", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "provider", LabelKey: "platform_health.issues.columns.provider", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "subject", LabelKey: "platform_health.issues.columns.subject", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "message", LabelKey: "platform_health.issues.columns.message", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "occurred_at", LabelKey: "platform_health.issues.columns.occurred_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
		},
		Filters: []resourcemeta.Filter{
			{Key: "severity", LabelKey: "platform_health.issues.columns.severity", Variant: resourcemeta.FilterVariantFaceted},
			{Key: "source", LabelKey: "platform_health.issues.columns.source", Variant: resourcemeta.FilterVariantFaceted},
		},
	}
}
