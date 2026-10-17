package usecase

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// accountView builds the API view of an account row.
func (s *Service) accountView(r db.ListChannelAccountsRow) model.Account {
	a := model.Account{
		UUID: r.Uuid, Provider: r.Provider, DisplayName: r.DisplayName, Mode: r.Mode, Status: r.Status, Channel: r.Channel,
		Settings: model.AccountSettings{
			PushEnabled: r.PushEnabled, PullEnabled: r.PullEnabled, StopSale: r.StopSale, HorizonDays: r.HorizonDays,
			PushIntervalMinutes: r.PushIntervalMinutes, PullIntervalMinutes: r.PullIntervalMinutes,
			ValidityMinutes: r.ValidityMinutes, OfflinePolicy: r.OfflinePolicy, OfflineQuota: r.OfflineQuota,
			CollectedBy: r.CollectedBy, CommissionRate: money.FormatNumeric(r.CommissionRate),
		},
		Sync: model.SyncState{
			PushRequestedAt: timePtr(r.PushRequestedAt), LastPushAt: timePtr(r.LastPushAt), LastPushOkAt: timePtr(r.LastPushOkAt),
			LastPushStatus: r.LastPushStatus, LastPushError: r.LastPushError, PushedValidUntil: timePtr(r.PushedValidUntil),
			LastPullAt: timePtr(r.LastPullAt), LastPullOkAt: timePtr(r.LastPullOkAt), LastPullStatus: r.LastPullStatus,
			LastPullError: r.LastPullError, ConsecutiveFailures: r.ConsecutiveFailures,
		},
		OpenErrors: r.OpenErrors, ActiveMappings: r.ActiveMappings,
	}
	if p, ok := s.providers[r.Provider]; ok {
		a.Available = true
		a.Capabilities = p.Capabilities()
		_, a.Simulator = p.(channels.Simulator)
	}
	a.Sync.Health = health(a, s.clock())
	return a
}

// health summarises the sync state: disabled (account or both directions
// off), failing (the last push or pull failed), expired (the channel's
// copy of our capacity ran out: the offline policy applies), pending
// (nothing pushed yet) or ok.
func health(a model.Account, now time.Time) string {
	switch {
	case a.Status != "active" || (!a.Settings.PushEnabled && !a.Settings.PullEnabled) || !a.Available:
		return model.HealthDisabled
	case a.Sync.LastPushStatus == model.RunFailed || a.Sync.LastPullStatus == model.RunFailed:
		return model.HealthFailing
	case a.Settings.PushEnabled && a.Sync.PushedValidUntil == nil:
		return model.HealthPending
	case a.Settings.PushEnabled && a.Sync.PushedValidUntil.Before(now):
		return model.HealthExpired
	}
	return model.HealthOK
}

// ListAccounts lists the organization's channel accounts with their sync
// settings and state.
func (s *Service) ListAccounts(ctx context.Context, orgID int64) ([]model.Account, error) {
	if err := s.q.EnsureChannelAccountsForOrganization(ctx, orgID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListChannelAccounts(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.accountView(r))
	}
	return out, nil
}

// accountRow loads one channel account (state row ensured).
func (s *Service) accountRow(ctx context.Context, orgID int64, id uuid.UUID) (db.ListChannelAccountsRow, error) {
	if err := s.q.EnsureChannelAccountsForOrganization(ctx, orgID); err != nil {
		return db.ListChannelAccountsRow{}, err
	}
	r, err := s.q.GetChannelAccount(ctx, db.GetChannelAccountParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return db.ListChannelAccountsRow{}, ErrAccountNotFound
	}
	if err != nil {
		return db.ListChannelAccountsRow{}, err
	}
	return db.ListChannelAccountsRow(r), nil
}

// GetAccount returns one channel account.
func (s *Service) GetAccount(ctx context.Context, orgID int64, id uuid.UUID) (model.Account, error) {
	r, err := s.accountRow(ctx, orgID, id)
	if err != nil {
		return model.Account{}, err
	}
	return s.accountView(r), nil
}

// UpdateSettings changes the sync settings of an account; the next sweep
// pushes with them.
func (s *Service) UpdateSettings(ctx context.Context, orgID int64, id uuid.UUID, in model.SettingsInput) (model.Account, error) {
	if err := requireAllBranches(ctx); err != nil {
		return model.Account{}, err
	}
	r, err := s.accountRow(ctx, orgID, id)
	if err != nil {
		return model.Account{}, err
	}
	p := db.UpdateChannelAccountSettingsParams{
		OrganizationID: orgID, IntegrationAccountID: r.ID, PushEnabled: r.PushEnabled, PullEnabled: r.PullEnabled,
		StopSale: r.StopSale, HorizonDays: r.HorizonDays, PushIntervalMinutes: r.PushIntervalMinutes,
		PullIntervalMinutes: r.PullIntervalMinutes, ValidityMinutes: r.ValidityMinutes, OfflinePolicy: r.OfflinePolicy,
		OfflineQuota: r.OfflineQuota, CollectedBy: r.CollectedBy, CommissionRate: r.CommissionRate,
	}
	ve := &ValidationError{}
	if in.PushEnabled != nil {
		p.PushEnabled = *in.PushEnabled
	}
	if in.PullEnabled != nil {
		p.PullEnabled = *in.PullEnabled
	}
	if in.StopSale != nil {
		p.StopSale = *in.StopSale
	}
	rangeOf := func(field string, v *int32, lo, hi int32, dst *int32) {
		if v == nil {
			return
		}
		if *v < lo || *v > hi {
			ve.add(field, "out_of_range", field+" must be between "+itoa(int(lo))+" and "+itoa(int(hi)))
			return
		}
		*dst = *v
	}
	rangeOf("horizon_days", in.HorizonDays, 1, 90, &p.HorizonDays)
	rangeOf("push_interval_minutes", in.PushIntervalMinutes, 5, 1440, &p.PushIntervalMinutes)
	rangeOf("pull_interval_minutes", in.PullIntervalMinutes, 1, 1440, &p.PullIntervalMinutes)
	rangeOf("validity_minutes", in.ValidityMinutes, 15, 2880, &p.ValidityMinutes)
	rangeOf("offline_quota", in.OfflineQuota, 0, 100, &p.OfflineQuota)
	if in.OfflinePolicy != nil {
		v := strings.TrimSpace(*in.OfflinePolicy)
		if v != channels.OfflineClose && v != channels.OfflineQuota {
			ve.add("offline_policy", "invalid", "offline_policy must be close or quota")
		} else {
			p.OfflinePolicy = v
		}
	}
	if in.CollectedBy != nil {
		v := strings.TrimSpace(*in.CollectedBy)
		if v != channels.CollectedByChannel && v != channels.CollectedBySupplier {
			ve.add("collected_by", "invalid", "collected_by must be channel or us")
		} else {
			p.CollectedBy = v
		}
	}
	if in.CommissionRate != nil {
		rate, err := money.ParseRound(strings.TrimSpace(*in.CommissionRate))
		if err != nil || rate.IsNegative() || rate.Cmp(money.FromUnits(100)) > 0 {
			ve.add("commission_rate", "invalid", "commission_rate must be a percentage between 0 and 100")
		} else {
			p.CommissionRate = rate.Numeric()
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Account{}, err
	}
	if _, err := s.q.UpdateChannelAccountSettings(ctx, p); err != nil {
		return model.Account{}, err
	}
	s.publish(ctx, model.ResourceAccounts, "updated", id)
	return s.GetAccount(ctx, orgID, id)
}

func itoa(v int) string { return strconv.Itoa(v) }
