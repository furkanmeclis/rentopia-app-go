package middleware

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/logging"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// Logs written with a request context carry the organization the request
// was scoped to (RequireOrganization), next to request_id.
func init() {
	logging.OrganizationFromContext = func(ctx context.Context) string {
		if s, ok := orgctx.From(ctx); ok && s.UUID != uuid.Nil {
			return s.UUID.String()
		}
		return ""
	}
}
