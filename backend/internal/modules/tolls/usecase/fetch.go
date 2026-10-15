package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tolls"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// fetchSlot is the cadence of the periodic fetch: one job per account and
// slot (the idempotency key), so a scheduler that fires twice creates
// nothing new.
const fetchSlot = time.Hour

// fetchPayload is the job payload (references and the window only).
type fetchPayload struct {
	AccountUUID string    `json:"account_uuid"`
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	Trigger     string    `json:"trigger"`
}

// fetchOperation is the integration runner operation "toll.fetch" of one
// provider. Fetching has no effect at the provider, so an unknown outcome
// is simply fetched again: Submit classifies every error as a retryable
// failure (definitive refusals are not retryable) and CheckStatus answers
// not_found, which lets the runner submit again.
type fetchOperation struct {
	svc      *Service
	provider tolls.Provider
}

func (o *fetchOperation) Kind() string { return model.JobKind }

// Submit fetches the window and ingests every pass.
func (o *fetchOperation) Submit(ctx context.Context, account integrations.Account, job integrations.Job) (integrations.Result, error) {
	var p fetchPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil || !p.To.After(p.From) {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Message: "invalid fetch window"}, nil
	}
	orgID := job.OrganizationID
	request := map[string]any{"from": p.From, "to": p.To, "trigger": p.Trigger}
	tags, err := o.svc.q.ListTollTagNumbersForFetch(ctx, db.ListTollTagNumbersForFetchParams{
		OrganizationID: orgID, WindowFrom: ts(p.From), WindowTo: ts(p.To),
	})
	if err != nil {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Retryable: true, Request: request, Message: "tags: " + err.Error()}, nil
	}
	request["tags"] = len(tags)
	passes, err := o.provider.FetchPasses(ctx, account, tolls.FetchRequest{From: p.From, To: p.To, Tags: tags})
	if err != nil {
		return integrations.Result{
			Outcome: integrations.OutcomeFailed, Retryable: !errors.Is(err, tolls.ErrRejected), Request: request,
			Message: err.Error(),
		}, nil
	}
	counts, failed, err := o.svc.ingestBatch(ctx, orgID, account, passes)
	response := map[string]any{
		"received": len(passes), "created": counts[model.OpCreated], "revised": counts[model.OpRevised],
		"unchanged": counts[model.OpUnchanged], "invalid": failed,
	}
	if err != nil {
		// Ingested rows stay (they are idempotent); the rest is fetched again.
		return integrations.Result{
			Outcome: integrations.OutcomeFailed, Retryable: true, Request: request, Response: response,
			Message: "ingest: " + err.Error(),
		}, nil
	}
	return integrations.Result{Outcome: integrations.OutcomeSucceeded, Request: request, Response: response}, nil
}

// CheckStatus: a fetch leaves nothing behind at the provider; answering
// not_found lets the runner fetch again.
func (o *fetchOperation) CheckStatus(context.Context, integrations.Account, integrations.Job) (integrations.Result, error) {
	return integrations.Result{
		Outcome:  integrations.OutcomeNotFound,
		Response: map[string]any{"source": "read_only_fetch"},
		Message:  "a fetch has no effect at the provider; fetching again is safe",
	}, nil
}

// ingestBatch stores fetched passes; invalid passes are counted and logged
// (never silently dropped), infrastructure errors stop the batch.
func (s *Service) ingestBatch(ctx context.Context, orgID int64, account integrations.Account, passes []tolls.Pass) (map[string]int, int, error) {
	counts := map[string]int{}
	failed := 0
	changed := []uuid.UUID{}
	for _, p := range passes {
		res, err := s.Ingest(ctx, orgID, model.PassInput{
			Provider: account.Provider, ExternalID: p.ExternalID, Source: model.SourceAPI, AccountID: account.ID,
			PassedAt: p.PassedAt, Plate: p.Plate, TagNumber: p.TagNumber, Station: p.Station, ExitStation: p.ExitStation,
			Amount: p.Amount, Currency: p.Currency, Raw: p.Raw,
		})
		var ve *ValidationError
		if errors.As(err, &ve) {
			failed++
			s.log.Warn("toll_pass_invalid", "organization_id", orgID, "external_id", p.ExternalID, "error", ve.Error())
			continue
		}
		if err != nil {
			s.publish(ctx, orgID, "bulk", changed, nil)
			return counts, failed, err
		}
		counts[res.Op]++
		if res.Op != model.OpUnchanged {
			changed = append(changed, res.UUID)
		}
	}
	if len(changed) > 0 {
		if len(changed) > 200 {
			changed = changed[:200]
		}
		s.publish(ctx, orgID, "bulk", changed, nil)
	}
	return counts, failed, nil
}

// window is the fetch window ending now (look-back days of the settings).
func (s *Service) window(ctx context.Context, orgID int64) (time.Time, time.Time, error) {
	st, err := s.Settings(ctx, orgID)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to := s.clock().Truncate(time.Minute)
	return to.AddDate(0, 0, -int(st.LookbackDays)), to, nil
}

// FetchNow queues a fetch of the look-back window for the organization's
// active toll account ("Şimdi sorgula"); one per minute at most.
func (s *Service) FetchNow(ctx context.Context, orgID int64) (model.FetchResult, error) {
	if s.accounts == nil || s.runner == nil {
		return model.FetchResult{}, ErrNoAccount
	}
	acc, err := s.accounts.ActiveAccount(ctx, orgID, tolls.Kind)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return model.FetchResult{}, ErrNoAccount
	}
	if err != nil {
		return model.FetchResult{}, err
	}
	if _, ok := s.providers[acc.Provider]; !ok {
		return model.FetchResult{}, ErrNoAccount
	}
	from, to, err := s.window(ctx, orgID)
	if err != nil {
		return model.FetchResult{}, err
	}
	key := fmt.Sprintf("org:%d:%s:%s:manual:%s", orgID, model.JobKind, acc.UUID, to.Format("200601021504"))
	return s.enqueue(ctx, orgID, acc, from, to, key, "manual")
}

func (s *Service) enqueue(ctx context.Context, orgID int64, acc integrations.Account, from, to time.Time, key, trigger string) (model.FetchResult, error) {
	accUUID := acc.UUID
	job, err := s.runner.Enqueue(ctx, jobsusecase.EnqueueInput{
		OrganizationID: orgID, AccountID: acc.ID, Provider: acc.Provider, Kind: model.JobKind, IdempotencyKey: key,
		Payload: map[string]any{"account_uuid": acc.UUID.String(), "from": from, "to": to, "trigger": trigger},
		RefType: "toll_account", RefUUID: &accUUID, MaxAttempts: 5,
	})
	if err != nil {
		return model.FetchResult{}, err
	}
	return model.FetchResult{JobUUID: job.UUID, Status: job.Status, From: from, To: to}, nil
}

// ScheduleFetches is the periodic fetch (worker, hourly): one job per
// active toll account whose organization keeps the automatic fetch on.
// Failures surface as integration jobs (operations overview).
func (s *Service) ScheduleFetches(ctx context.Context) (int, error) {
	if s.runner == nil {
		return 0, nil
	}
	rows, err := s.q.ListTollFetchAccounts(ctx)
	if err != nil {
		return 0, err
	}
	slot := s.clock().Truncate(fetchSlot)
	n := 0
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		if _, ok := s.providers[row.Provider]; !ok {
			continue
		}
		octx := orgctx.WithOrganizationID(ctx, row.OrganizationID)
		acc := integrations.Account{ID: row.ID, OrganizationID: row.OrganizationID, UUID: row.Uuid, Kind: tolls.Kind, Provider: row.Provider}
		to := s.clock().Truncate(time.Minute)
		from := to.AddDate(0, 0, -int(row.LookbackDays))
		key := fmt.Sprintf("org:%d:%s:%s:slot:%s", row.OrganizationID, model.JobKind, row.Uuid, slot.Format("2006010215"))
		if _, err := s.enqueue(octx, row.OrganizationID, acc, from, to, key, "schedule"); err != nil {
			s.log.Warn("toll_fetch_schedule_failed", "organization_id", row.OrganizationID, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// RematchDue re-runs the automatic matching of review passes of the last
// 60 days (a handover recorded later, a tag added since): passes nobody
// decided on only.
func (s *Service) RematchDue(ctx context.Context) (int, error) {
	since := s.clock().AddDate(0, 0, -60)
	orgs, err := s.q.ListOrganizationsWithReviewTollPasses(ctx, ts(since))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, orgID := range orgs {
		ids, err := s.q.ListTollPassesToRematch(ctx, db.ListTollPassesToRematchParams{OrganizationID: orgID, Since: ts(since), LimitCount: 500})
		if err != nil {
			return n, err
		}
		octx := orgctx.WithOrganizationID(ctx, orgID)
		changed := []uuid.UUID{}
		for _, id := range ids {
			moved, err := s.rematchOne(octx, orgID, id)
			if err != nil {
				s.log.Warn("toll_rematch_failed", "organization_id", orgID, "pass", id, "error", err)
				continue
			}
			if moved {
				changed = append(changed, id)
				n++
			}
		}
		if len(changed) > 0 {
			s.publish(octx, orgID, "bulk", changed, nil)
		}
	}
	return n, nil
}

func (s *Service) rematchOne(ctx context.Context, orgID int64, id uuid.UUID) (bool, error) {
	moved := false
	var rentals []uuid.UUID
	err := s.tx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		p, err := q.LockTollPassByUUID(ctx, db.LockTollPassByUUIDParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return err
		}
		if p.Manual || (p.Status != model.StatusNeedsReview && p.Status != model.StatusUnmatched) {
			return nil
		}
		before := p.Status + "|" + p.ReviewReason
		rentals, err = s.automatic(ctx, q, tx, orgID, &p, "")
		moved = before != p.Status+"|"+p.ReviewReason
		return err
	})
	if len(rentals) > 0 {
		s.publish(ctx, orgID, "updated", nil, rentals)
	}
	return moved, err
}

// Settings returns the organization's toll settings (defaults when none
// were saved).
func (s *Service) Settings(ctx context.Context, orgID int64) (model.Settings, error) {
	out := model.Settings{ProcessingFee: money.Zero(), Currency: "TRY", AutoFetch: true, LookbackDays: 7}
	st, err := s.q.GetTollSettings(ctx, orgID)
	switch {
	case notFound(err):
	case err != nil:
		return out, err
	default:
		out.ProcessingFee = money.FromNumericOrZero(st.ProcessingFee)
		out.Currency = strings.TrimSpace(st.Currency)
		out.AutoFetch, out.LookbackDays = st.AutoFetch, st.LookbackDays
		out.UpdatedAt = timePtr(st.UpdatedAt)
	}
	if s.accounts != nil {
		if acc, err := s.accounts.ActiveAccount(ctx, orgID, tolls.Kind); err == nil {
			_, out.ProviderConnected = s.providers[acc.Provider]
		}
	}
	return out, nil
}

// UpdateSettings changes the processing fee / fetch options. A new fee
// applies to passes charged from now on (existing fee lines stay).
func (s *Service) UpdateSettings(ctx context.Context, orgID int64, in model.SettingsInput) (model.Settings, error) {
	cur, err := s.Settings(ctx, orgID)
	if err != nil {
		return cur, err
	}
	ve := &ValidationError{}
	fee, auto, days := cur.ProcessingFee, cur.AutoFetch, cur.LookbackDays
	if in.ProcessingFee != nil {
		if in.ProcessingFee.IsNegative() || in.ProcessingFee.Cmp(money.FromUnits(100000)) > 0 {
			ve.add("processing_fee", "invalid", "processing fee must be between 0 and 100000")
		}
		fee = *in.ProcessingFee
	}
	if in.AutoFetch != nil {
		auto = *in.AutoFetch
	}
	if in.LookbackDays != nil {
		if *in.LookbackDays < 1 || *in.LookbackDays > 60 {
			ve.add("lookback_days", "invalid", "look-back must be 1-60 days")
		}
		days = *in.LookbackDays
	}
	if err := ve.orNil(); err != nil {
		return cur, err
	}
	if _, err := s.q.UpsertTollSettings(ctx, db.UpsertTollSettingsParams{
		OrganizationID: orgID, ProcessingFee: fee.Numeric(), Currency: "TRY", AutoFetch: auto, LookbackDays: days,
		UpdatedBy: actorID(ctx),
	}); err != nil {
		return cur, err
	}
	return s.Settings(ctx, orgID)
}
