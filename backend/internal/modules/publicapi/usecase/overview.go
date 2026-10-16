package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
)

// Overview is the header of the panel page: which channels the keys can
// sell on, failed / pending deliveries, the catalogs of scopes and events,
// and the base URL integrators call.
func (s *Service) Overview(ctx context.Context, orgID int64) (model.Overview, error) {
	out := model.Overview{Channels: []model.ChannelState{}, Events: model.WebhookEvents, Scopes: model.AllScopes, BaseURL: s.baseURL}
	active := map[string]bool{}
	if s.settings != nil {
		v, err := s.settings.Current(ctx, orgID)
		if err != nil {
			return model.Overview{}, err
		}
		for _, c := range v.Settings.SalesChannels {
			active[c.Code] = c.IsActive
		}
	}
	for _, c := range model.Channels {
		out.Channels = append(out.Channels, model.ChannelState{Code: c, Active: active[c]})
	}
	stats, err := s.DeliveryStats(ctx, orgID)
	if err != nil {
		return model.Overview{}, err
	}
	out.Deliveries = stats
	return out, nil
}
