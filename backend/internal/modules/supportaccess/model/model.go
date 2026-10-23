// Package model holds the support access types (TEC-678).
package model

import (
	"time"

	"github.com/google/uuid"
)

// Resource is the activity / engine slug of support access grants.
const Resource = "tenant.support_access"

// Grant scopes.
const (
	ScopeReadOnly = "read_only"
	ScopeFull     = "full"
)

// Grant statuses (derived at read time from revoked_at / expires_at).
const (
	StatusActive  = "active"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
)

// Duration bounds of a grant (1 hour – 7 days).
const (
	MinDurationHours = 1
	MaxDurationHours = 7 * 24
	MaxNoteLength    = 500
)

// UserRef names the member who opened or revoked a grant.
type UserRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Grant is one support access grant of an organization.
type Grant struct {
	ID        int64      `json:"-"`
	UUID      uuid.UUID  `json:"uuid"`
	Scope     string     `json:"scope"`
	Status    string     `json:"status"`
	Note      string     `json:"note"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *UserRef   `json:"created_by"`
	RevokedBy *UserRef   `json:"revoked_by"`
}

// StatusAt derives the status of a grant at t.
func StatusAt(revokedAt *time.Time, expiresAt, t time.Time) string {
	switch {
	case revokedAt != nil:
		return StatusRevoked
	case !expiresAt.After(t):
		return StatusExpired
	default:
		return StatusActive
	}
}

// OpenInput is the body of POST /v1/tenant/support-access.
type OpenInput struct {
	Scope         string `json:"scope"`
	DurationHours int    `json:"duration_hours"`
	Note          string `json:"note"`
}

// ListFilter narrows GET /v1/tenant/support-access.
type ListFilter struct {
	Statuses []string
	Limit    int32
	Offset   int32
}

// PlatformGrant is an active grant in the platform overview.
type PlatformGrant struct {
	UUID         uuid.UUID       `json:"uuid"`
	Scope        string          `json:"scope"`
	Note         string          `json:"note"`
	ExpiresAt    time.Time       `json:"expires_at"`
	CreatedAt    time.Time       `json:"created_at"`
	Organization OrganizationRef `json:"organization"`
	CreatedBy    *UserRef        `json:"created_by"`
}

// OrganizationRef names the organization of a platform grant row.
type OrganizationRef struct {
	UUID uuid.UUID `json:"uuid"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}
