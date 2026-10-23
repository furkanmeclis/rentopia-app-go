package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// route is where a notification job goes: a working adapter (ready) or a
// dependency hold.
type route struct {
	provider  string
	accountID pgtype.Int8
	ready     bool
	hold      string
}

func holdMessage(reason string) string {
	switch reason {
	case model.HoldNoAccount:
		return "waiting on dependency: no active KABİS connection is set up for the organization"
	case model.HoldAdapterUnverified:
		return "waiting on dependency: the official KABİS connection method is not verified yet"
	case model.HoldAdapterUnavailable:
		return "waiting on dependency: the connected KABİS adapter is not available in this environment"
	}
	return "waiting on dependency"
}

// routeOf picks the organization's active KABİS account and checks that
// its adapter can send.
func (s *Service) routeOf(ctx context.Context, q *db.Queries, orgID int64) (route, error) {
	acc, err := q.GetActiveIntegrationAccount(ctx, db.GetActiveIntegrationAccountParams{OrganizationID: orgID, Kind: kabis.Kind})
	if notFound(err) {
		return route{provider: kabis.ProviderOfficial, hold: model.HoldNoAccount}, nil
	}
	if err != nil {
		return route{}, err
	}
	r := route{provider: acc.Provider, accountID: pgtype.Int8{Int64: acc.ID, Valid: true}}
	p, ok := s.providers[acc.Provider]
	switch {
	case !ok:
		r.hold = model.HoldAdapterUnavailable
	case p.Ready() != nil:
		r.hold = model.HoldAdapterUnverified
	default:
		r.ready = true
	}
	return r, nil
}

// decide is the job state of a built payload on a route.
func decide(b built, r route) (status, lastError, hold string) {
	if !r.ready {
		hold = r.hold
	}
	switch {
	case len(b.missing) > 0:
		return jobsusecase.StatusBlocked, "required data is missing: " + strings.Join(b.missing, ", "), hold
	case !r.ready:
		return jobsusecase.StatusWaitingDependency, holdMessage(r.hold), hold
	}
	return jobsusecase.StatusPending, "", ""
}

// RecordRentalEvent records the KABİS notification of an actual rental
// event INSIDE the rental's transaction (q is bound to it): the handover
// (cause handover → rental_start), the return (return → rental_end), and
// changes of a reported start (drivers_changed, vehicle_swap →
// correction, or an update of a start that was not sent yet). It is
// idempotent per rental and event: calling it again creates nothing. The
// returned function runs after the commit (first step of a pending job,
// live list); it may be nil.
func (s *Service) RecordRentalEvent(ctx context.Context, q *db.Queries, orgID, rentalID int64, cause string) (func(context.Context), error) {
	switch cause {
	case model.CauseHandover:
		return s.recordOnce(ctx, q, orgID, rentalID, kabis.EventRentalStart, cause)
	case model.CauseReturn:
		return s.recordOnce(ctx, q, orgID, rentalID, kabis.EventRentalEnd, cause)
	case model.CauseDriversChanged, model.CauseVehicleSwap:
		return s.recordChange(ctx, q, orgID, rentalID, cause)
	}
	return nil, nil
}

type created struct {
	id, jobID int64
	uuid      uuid.UUID
	status    string
	action    string
}

func (s *Service) after(orgID int64, items ...created) func(context.Context) {
	if len(items) == 0 {
		return nil
	}
	return func(ctx context.Context) {
		ctx = orgctx.WithOrganizationID(ctx, orgID)
		for _, it := range items {
			s.publish(ctx, it.action, it.uuid.String())
			if it.status == jobsusecase.StatusPending && s.runner != nil {
				if _, err := s.runner.Kick(ctx, orgID, it.jobID); err != nil {
					s.log.Warn("kabis_first_step_failed", "notification", it.uuid, "error", err)
				}
			}
		}
	}
}

func (s *Service) recordOnce(ctx context.Context, q *db.Queries, orgID, rentalID int64, event, cause string) (func(context.Context), error) {
	if _, err := q.GetKabisNotificationByRentalEvent(ctx, db.GetKabisNotificationByRentalEventParams{
		OrganizationID: orgID, RentalID: rentalID, Event: event, Sequence: 1,
	}); err == nil {
		return nil, nil
	} else if !notFound(err) {
		return nil, err
	}
	c, err := s.insert(ctx, q, orgID, rentalID, event, 1, cause, pgtype.Int8{}, "")
	if errors.Is(err, errNotHandedOver) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.after(orgID, c), nil
}

// insert builds, validates and stores one notification with its job.
func (s *Service) insert(ctx context.Context, q *db.Queries, orgID, rentalID int64, event string, seq int32, cause string,
	corrects pgtype.Int8, correctsUUID string) (created, error) {
	id := uuid.New()
	b, err := s.build(ctx, q, orgID, rentalID, event, id, correctsUUID)
	if err != nil {
		return created{}, err
	}
	r, err := s.routeOf(ctx, q, orgID)
	if err != nil {
		return created{}, err
	}
	status, lastErr, hold := decide(b, r)
	enc, err := s.seal(orgID, id, b.n)
	if err != nil {
		return created{}, err
	}
	summary, _ := json.Marshal(b.summary)
	payload, _ := json.Marshal(map[string]any{"notification_uuid": id.String(), "event": event, "rental_reference": b.n.RentalReference})
	jp := db.InsertKabisJobParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, IntegrationAccountID: r.accountID, Provider: r.provider,
		Kind: model.KindOf(event), IdempotencyKey: "org:" + strconv.FormatInt(orgID, 10) + ":kabis:" + id.String(),
		Payload: payload, RefUuid: pgtype.UUID{Bytes: id, Valid: true}, MaxAttempts: MaxAttempts, Status: status,
		LastError: lastErr,
	}
	if status == jobsusecase.StatusPending {
		jp.NextAttemptAt = ts(s.clock())
	}
	job, err := q.InsertKabisJob(ctx, jp)
	if err != nil {
		return created{}, err
	}
	missing := b.missing
	if missing == nil {
		missing = []string{}
	}
	nid, err := q.InsertKabisNotification(ctx, db.InsertKabisNotificationParams{
		Uuid: id, OrganizationID: orgID, RentalID: rentalID, BranchID: b.branchID, RentalBranchID: b.rentalBr,
		Event: event, Sequence: seq, Cause: cause, CorrectsID: corrects, IntegrationJobID: job.ID,
		OccurredAt: ts(b.occurredAt), Plate: b.plate, PayloadEnc: enc, PayloadHash: contentHash(b.n), Summary: summary,
		MissingFields: missing, HoldReason: hold,
	})
	if err != nil {
		return created{}, err
	}
	return created{id: nid, jobID: job.ID, uuid: id, status: status, action: resourceevents.ActionCreated}, nil
}

// recordChange handles a change of the person / vehicle data after the
// handover. Nothing changes when the reported facts are the same. A start
// that has not been sent yet takes the new data in place; one that was
// sent (or is being sent, or was reported by a person) gets a correction.
func (s *Service) recordChange(ctx context.Context, q *db.Queries, orgID, rentalID int64, cause string) (func(context.Context), error) {
	latest, err := q.GetLatestKabisStartChain(ctx, db.GetLatestKabisStartChainParams{OrganizationID: orgID, RentalID: rentalID})
	if notFound(err) {
		return nil, nil // not handed over (no start yet): nothing to amend
	}
	if err != nil {
		return nil, err
	}
	probe, err := s.build(ctx, q, orgID, rentalID, kabis.EventCorrection, latest.Uuid, "")
	if errors.Is(err, errNotHandedOver) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if contentHash(probe.n) == latest.PayloadHash {
		return nil, nil
	}
	unsent := latest.JobStatus == jobsusecase.StatusBlocked || latest.JobStatus == jobsusecase.StatusWaitingDependency ||
		(latest.JobStatus == jobsusecase.StatusPending && latest.JobAttempts == 0)
	if unsent {
		c, ok, err := s.refresh(ctx, q, orgID, latest.ID, latest.Uuid, latest.IntegrationJobID, rentalID, latest.Event)
		if err != nil {
			return nil, err
		}
		if ok {
			return s.after(orgID, c), nil
		}
	}
	seq, err := q.NextKabisCorrectionSequence(ctx, db.NextKabisCorrectionSequenceParams{OrganizationID: orgID, RentalID: rentalID})
	if err != nil {
		return nil, err
	}
	c, err := s.insert(ctx, q, orgID, rentalID, kabis.EventCorrection, seq, cause,
		pgtype.Int8{Int64: latest.ID, Valid: true}, latest.Uuid.String())
	if err != nil {
		return nil, err
	}
	return s.after(orgID, c), nil
}

// refresh rebuilds a notification that has sent nothing yet from the
// current data and re-routes its job. ok = false when the job moved on in
// the meantime (a worker holds it or made a call): the caller then adds a
// correction instead.
func (s *Service) refresh(ctx context.Context, q *db.Queries, orgID, id int64, nid uuid.UUID, jobID, rentalID int64, event string) (created, bool, error) {
	corrects := ""
	if event == kabis.EventCorrection {
		if row, err := q.GetKabisNotification(ctx, db.GetKabisNotificationParams{OrganizationID: orgID, Uuid: nid}); err == nil && row.CorrectsUuid.Valid {
			corrects = uuid.UUID(row.CorrectsUuid.Bytes).String()
		}
	}
	b, err := s.build(ctx, q, orgID, rentalID, event, nid, corrects)
	if err != nil {
		return created{}, false, err
	}
	r, err := s.routeOf(ctx, q, orgID)
	if err != nil {
		return created{}, false, err
	}
	status, lastErr, hold := decide(b, r)
	rp := db.RouteKabisJobParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: jobID, Status: status, Provider: r.provider,
		IntegrationAccountID: r.accountID, LastError: lastErr,
	}
	if status == jobsusecase.StatusPending {
		rp.NextAttemptAt = ts(s.clock())
	}
	n, err := q.RouteKabisJob(ctx, rp)
	if err != nil {
		return created{}, false, err
	}
	if n != 1 {
		return created{}, false, nil
	}
	enc, err := s.seal(orgID, nid, b.n)
	if err != nil {
		return created{}, false, err
	}
	summary, _ := json.Marshal(b.summary)
	missing := b.missing
	if missing == nil {
		missing = []string{}
	}
	if _, err := q.RefreshKabisNotification(ctx, db.RefreshKabisNotificationParams{
		OrganizationID: orgID, ID: id, Plate: b.plate, PayloadEnc: enc, PayloadHash: contentHash(b.n), Summary: summary,
		MissingFields: missing, HoldReason: hold,
	}); err != nil {
		return created{}, false, err
	}
	return created{id: id, jobID: jobID, uuid: nid, status: status, action: resourceevents.ActionUpdated}, true, nil
}
