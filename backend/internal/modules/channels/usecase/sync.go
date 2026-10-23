package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/google/uuid"
)

// session is one claimed sync of an account.
type session struct {
	orgID    int64
	row      db.GetChannelAccountStateRow
	account  integrations.Account
	provider channels.Provider
	runID    int64
	runUUID  uuid.UUID
	started  time.Time
}

// runResult closes a sync run.
type runResult struct {
	status            string
	total, ok, failed int
	err               string
	details           map[string]any
}

// begin claims the account, opens a sync run and resolves the adapter.
func (s *Service) begin(ctx context.Context, orgID, accountID int64, direction, trigger string) (*session, error) {
	now := s.clock()
	n, err := s.q.LockChannelAccount(ctx, db.LockChannelAccountParams{
		OrganizationID: orgID, IntegrationAccountID: accountID, LockedUntil: ts(now.Add(lockFor)), Now: ts(now),
	})
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, ErrBusy
	}
	release := func() {
		_ = s.q.UnlockChannelAccount(context.WithoutCancel(ctx), db.UnlockChannelAccountParams{OrganizationID: orgID, IntegrationAccountID: accountID})
	}
	st, err := s.q.GetChannelAccountState(ctx, db.GetChannelAccountStateParams{OrganizationID: orgID, IntegrationAccountID: accountID})
	if err != nil {
		release()
		return nil, err
	}
	account, err := s.accounts.AccountByID(ctx, orgID, accountID)
	if err != nil {
		release()
		return nil, err
	}
	run, err := s.q.InsertChannelSyncRun(ctx, db.InsertChannelSyncRunParams{
		OrganizationID: orgID, IntegrationAccountID: accountID, Direction: direction, Trigger: trigger,
		ActorID: actorID(ctx), StartedAt: ts(now),
	})
	if err != nil {
		release()
		return nil, err
	}
	sess := &session{orgID: orgID, row: st, account: account, runID: run.ID, runUUID: run.Uuid, started: now}
	p, ok := s.providers[account.Provider]
	if !ok {
		s.finish(ctx, sess, runResult{status: model.RunFailed, err: ErrAdapterUnavailable.Error()})
		return nil, ErrAdapterUnavailable
	}
	sess.provider = p
	return sess, nil
}

// finish closes the run and releases the account.
func (s *Service) finish(ctx context.Context, sess *session, r runResult) {
	ctx = context.WithoutCancel(ctx)
	if r.details == nil {
		r.details = map[string]any{}
	}
	details, _ := json.Marshal(r.details)
	if err := s.q.FinishChannelSyncRun(ctx, db.FinishChannelSyncRunParams{
		OrganizationID: sess.orgID, ID: sess.runID, Status: r.status, ItemsTotal: int32(r.total), ItemsOk: int32(r.ok),
		ItemsFailed: int32(r.failed), Error: truncate(r.err, 2000), Details: details, FinishedAt: ts(s.clock()),
	}); err != nil {
		s.log.Warn("channel_sync_run_finish_failed", "organization_id", sess.orgID, "error", err)
	}
	_ = s.q.UnlockChannelAccount(ctx, db.UnlockChannelAccountParams{OrganizationID: sess.orgID, IntegrationAccountID: sess.row.IntegrationAccountID})
	s.publish(ctx, model.ResourceAccounts, "updated", sess.account.UUID)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// accountForAction loads an account for a manual action: active, adapter
// available.
func (s *Service) accountForAction(ctx context.Context, orgID int64, id uuid.UUID) (db.ListChannelAccountsRow, error) {
	if err := requireAllBranches(ctx); err != nil {
		return db.ListChannelAccountsRow{}, err
	}
	r, err := s.accountRow(ctx, orgID, id)
	if err != nil {
		return r, err
	}
	if err := s.requireChannelFeature(ctx, orgID, r.Channel); err != nil {
		return r, err
	}
	if r.Status != "active" {
		return r, ErrAccountInactive
	}
	if _, ok := s.providers[r.Provider]; !ok {
		return r, ErrAdapterUnavailable
	}
	return r, nil
}

// PushNow pushes the account's inventory at once (manual).
func (s *Service) PushNow(ctx context.Context, orgID int64, id uuid.UUID) (model.SyncRun, error) {
	r, err := s.accountForAction(ctx, orgID, id)
	if err != nil {
		return model.SyncRun{}, err
	}
	runID, err := s.push(systemContext(ctx, orgID), orgID, r.ID, model.TriggerManual)
	if err != nil && runID == uuid.Nil {
		return model.SyncRun{}, err
	}
	return s.syncRun(ctx, orgID, runID)
}

// PullNow reads the account's messages at once (manual).
func (s *Service) PullNow(ctx context.Context, orgID int64, id uuid.UUID) (model.SyncRun, error) {
	r, err := s.accountForAction(ctx, orgID, id)
	if err != nil {
		return model.SyncRun{}, err
	}
	runID, err := s.pull(systemContext(ctx, orgID), orgID, r.ID, model.TriggerManual)
	if err != nil && runID == uuid.Nil {
		return model.SyncRun{}, err
	}
	return s.syncRun(ctx, orgID, runID)
}

func (s *Service) syncRun(ctx context.Context, orgID int64, id uuid.UUID) (model.SyncRun, error) {
	runs, _, err := s.listRuns(ctx, orgID, model.SyncRunFilter{Limit: 1}, &id)
	if err != nil {
		return model.SyncRun{}, err
	}
	if len(runs) == 0 {
		return model.SyncRun{}, fmt.Errorf("channels: sync run %s not found", id)
	}
	return runs[0], nil
}

// SyncDue is the worker sweep (every minute): for every active channel
// account, a push when an event asked for one or the push interval passed,
// and a pull when the pull interval passed. Accounts held by another sync
// are skipped. Returns the number of syncs run.
func (s *Service) SyncDue(ctx context.Context) (int, error) {
	accounts, err := s.q.ListActiveChannelIntegrationAccounts(ctx)
	if err != nil {
		return 0, err
	}
	ran := 0
	now := s.clock()
	for _, a := range accounts {
		if ctx.Err() != nil {
			return ran, ctx.Err()
		}
		p, ok := s.providers[a.Provider]
		if !ok {
			continue
		}
		orgID := a.OrganizationID
		// A channel outside the plan is not synced (its account and
		// history stay; TEC-674).
		if err := s.requireChannelFeature(ctx, orgID, p.Channel()); err != nil {
			if _, off := features.FeatureOf(err); !off {
				s.log.Warn("channel_feature_check_failed", "organization_id", orgID, "error", err)
			}
			continue
		}
		if err := s.q.EnsureChannelAccount(ctx, db.EnsureChannelAccountParams{OrganizationID: orgID, IntegrationAccountID: a.ID, Channel: p.Channel()}); err != nil {
			s.log.Warn("channel_state_failed", "organization_id", orgID, "error", err)
			continue
		}
		st, err := s.q.GetChannelAccountState(ctx, db.GetChannelAccountStateParams{OrganizationID: orgID, IntegrationAccountID: a.ID})
		if err != nil {
			continue
		}
		octx := systemContext(ctx, orgID)
		if st.PushEnabled {
			requested := st.PushRequestedAt.Valid && (!st.LastPushAt.Valid || st.PushRequestedAt.Time.After(st.LastPushAt.Time))
			interval := time.Duration(st.PushIntervalMinutes) * time.Minute
			periodic := !st.LastPushAt.Valid || !now.Before(st.LastPushAt.Time.Add(interval))
			if requested || periodic {
				trigger := model.TriggerSchedule
				if requested {
					trigger = model.TriggerEvent
				}
				if _, err := s.push(octx, orgID, a.ID, trigger); err == nil || !errors.Is(err, ErrBusy) {
					ran++
				}
			}
		}
		if st.PullEnabled && p.Capabilities().Pull {
			interval := time.Duration(st.PullIntervalMinutes) * time.Minute
			if !st.LastPullAt.Valid || !now.Before(st.LastPullAt.Time.Add(interval)) {
				if _, err := s.pull(octx, orgID, a.ID, model.TriggerSchedule); err == nil || !errors.Is(err, ErrBusy) {
					ran++
				}
			}
		}
	}
	return ran, nil
}
