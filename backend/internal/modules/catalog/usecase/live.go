package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
)

// Live list resources of the platform catalog screens (resource:{name}
// channels, permission platform.catalog.read).
const (
	LiveBrands  = "platform.catalog.brands"
	LiveModels  = "platform.catalog.models"
	LiveTypes   = "platform.catalog.types"
	LiveImports = "platform.catalog.imports"
)

// SetResourceEvents attaches the live list publisher (optional).
func (s *Service) SetResourceEvents(pub resourceevents.Publisher) {
	s.events = pub
}

func (s *Service) publish(ctx context.Context, resource, action string, ids ...uuid.UUID) {
	if s.events == nil {
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	s.events.Publish(ctx, resource, action, out...)
}

// publishCatalogApplied tells the brand / model / type lists that an import
// apply may have changed any of their rows.
func (s *Service) publishCatalogApplied(ctx context.Context) {
	for _, resource := range []string{LiveBrands, LiveModels, LiveTypes} {
		s.publish(ctx, resource, resourceevents.ActionBulk)
	}
}
