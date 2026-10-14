package searchengine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Spec scopes.
const (
	// ScopePlatform specs are searched from the platform shell (/v1/search).
	ScopePlatform = ""
	// ScopeTenant specs hold organization-owned records. They are searched
	// only through /v1/tenant/search, filtered by organization_id.
	ScopeTenant = "tenant"
)

// Spec describes one searchable resource catalog entry.
type Spec struct {
	ID         string   `json:"id"`
	LabelKey   string   `json:"label_key"`
	Permission string   `json:"permission,omitempty"`
	Icon       string   `json:"icon,omitempty"`
	Searchable []string `json:"searchable_fields,omitempty"`
	Filterable []string `json:"filterable_fields,omitempty"`
	// Scope is ScopePlatform (empty) or ScopeTenant.
	Scope string `json:"scope,omitempty"`
}

// Document is the Meilisearch index payload for one entity.
type Document struct {
	ID       string   `json:"id"`
	Spec     string   `json:"spec"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	Href     string   `json:"href"`
	Icon     string   `json:"icon,omitempty"`
	// OrganizationID is set on tenant documents; tenant searches filter on it.
	OrganizationID int64 `json:"organization_id,omitempty"`
}

// Hit is a normalized search result returned to clients.
type Hit struct {
	Spec     string  `json:"spec"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle,omitempty"`
	Href     string  `json:"href"`
	Icon     string  `json:"icon,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

// Adapter connects searchengine to a list resource.
type Adapter interface {
	Spec() Spec
	ListAll(ctx context.Context) ([]Document, error)
	Document(ctx context.Context, id string) (Document, error)
}

// TenantDocumentID builds the document id of an organization-owned record:
// "<organization id>_<record id>". The indexer passes it back to
// Adapter.Document, so tenant adapters can look the record up by
// organization (every tenant query filters by organization_id).
func TenantDocumentID(organizationID int64, id string) string {
	return strconv.FormatInt(organizationID, 10) + "_" + id
}

// ParseTenantDocumentID splits a TenantDocumentID.
func ParseTenantDocumentID(docID string) (int64, string, error) {
	org, id, ok := strings.Cut(strings.TrimSpace(docID), "_")
	if !ok || id == "" {
		return 0, "", fmt.Errorf("searchengine: invalid tenant document id %q", docID)
	}
	orgID, err := strconv.ParseInt(org, 10, 64)
	if err != nil || orgID <= 0 {
		return 0, "", fmt.Errorf("searchengine: invalid tenant document id %q", docID)
	}
	return orgID, id, nil
}
