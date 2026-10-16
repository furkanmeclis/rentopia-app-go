package usecase

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func objectOf(raw []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func orNone(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// List returns a page of notifications in the caller's branch scope.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Notification, int64, error) {
	p := db.ListKabisNotificationsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if p.SortField == "" {
		p.SortField, p.SortDesc = "occurred_at", true
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	if len(f.Statuses) > 0 {
		p.Statuses = f.Statuses
	}
	if len(f.Events) > 0 {
		p.Events = f.Events
	}
	if f.RentalUUID != nil {
		p.RentalUuid = pgtype.UUID{Bytes: *f.RentalUUID, Valid: true}
	}
	if f.From != nil {
		p.OccurredFrom = ts(*f.From)
	}
	if f.To != nil {
		p.OccurredTo = ts(*f.To)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListKabisNotifications(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Notification, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, model.Notification{
			UUID: r.Uuid, Event: r.Event, Sequence: r.Sequence, Cause: r.Cause, OccurredAt: r.OccurredAt.Time.UTC(),
			Plate: r.Plate, Status: r.JobStatus, Official: model.Contains(model.OfficialStatuses, r.JobStatus),
			NeedsAction: model.Contains(model.NeedsActionStatuses, r.JobStatus), Attempts: r.JobAttempts,
			MaxAttempts: r.JobMaxAttempts, LastError: r.JobLastError, Provider: r.JobProvider,
			ExternalReference: r.JobProviderRef, NextAttemptAt: timePtr(r.JobNextAttemptAt),
			SubmittedAt: timePtr(r.JobSubmittedAt), CompletedAt: timePtr(r.JobCompletedAt),
			LastVerifiedAt: timePtr(r.LastVerifiedAt), MissingFields: orNone(r.MissingFields), HoldReason: r.HoldReason,
			Resolution: textPtr(r.Resolution), ResolutionReference: r.ResolutionReference, ResolvedAt: timePtr(r.ResolvedAt),
			Rental:  model.Ref{UUID: r.RentalUuid, Reference: r.RentalReference},
			Branch:  model.Ref{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
			JobUUID: r.JobUuid, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		})
	}
	return out, total, nil
}

// Facets returns the status / event counts in the caller's branch scope.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	scope := orgctx.BranchScope(ctx)
	out := model.Facets{Statuses: map[string]int64{}, Events: map[string]int64{}}
	rows, err := s.q.KabisStatusFacets(ctx, db.KabisStatusFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		out.Statuses[r.Status] = r.Total
		out.Total += r.Total
		if model.Contains(model.NeedsActionStatuses, r.Status) {
			out.NeedsAction += r.Total
		}
	}
	events, err := s.q.KabisEventFacets(ctx, db.KabisEventFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range events {
		out.Events[r.Event] = r.Total
	}
	return out, nil
}

// row loads a notification in the organization and the branch scope.
func (s *Service) row(ctx context.Context, orgID int64, id uuid.UUID) (db.GetKabisNotificationRow, error) {
	r, err := s.q.GetKabisNotification(ctx, db.GetKabisNotificationParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if orgctx.CheckBranch(ctx, r.BranchID) != nil && orgctx.CheckBranch(ctx, r.RentalBranchID) != nil {
		return r, ErrNotFound
	}
	return r, nil
}

func has(ctx context.Context, slug string) bool {
	sc, ok := orgctx.From(ctx)
	return ok && sc.Has(slug)
}

// Retryable: held (re-validate / look for an adapter) or a runner state.
func retryable(status string) bool {
	switch status {
	case jobsusecase.StatusBlocked, jobsusecase.StatusWaitingDependency, jobsusecase.StatusPending,
		jobsusecase.StatusFailed, jobsusecase.StatusNeedsVerification, jobsusecase.StatusSubmitted:
		return true
	}
	return false
}

// manualFrom are the states a "reported manually" resolution applies to:
// everything that is not final (TEC-682 procedure during an outage, with
// no adapter, or with incomplete data filed by hand).
var manualFrom = []string{
	jobsusecase.StatusBlocked, jobsusecase.StatusWaitingDependency, jobsusecase.StatusPending,
	jobsusecase.StatusFailed, jobsusecase.StatusNeedsVerification,
}

// reconcileFrom are the states a "reconciled" resolution applies to: the
// outcome is unknown or disputed and a person confirmed it with the
// authority.
var reconcileFrom = []string{
	jobsusecase.StatusNeedsVerification, jobsusecase.StatusSubmitted, jobsusecase.StatusFailed,
}

func (s *Service) detail(ctx context.Context, orgID int64, r db.GetKabisNotificationRow) (model.Detail, error) {
	d := model.Detail{
		Notification: model.Notification{
			UUID: r.Uuid, Event: r.Event, Sequence: r.Sequence, Cause: r.Cause, OccurredAt: r.OccurredAt.Time.UTC(),
			Plate: r.Plate, Status: r.JobStatus, Official: model.Contains(model.OfficialStatuses, r.JobStatus),
			NeedsAction: model.Contains(model.NeedsActionStatuses, r.JobStatus), Attempts: r.JobAttempts,
			MaxAttempts: r.JobMaxAttempts, LastError: r.JobLastError, Provider: r.JobProvider,
			ExternalReference: r.JobProviderRef, NextAttemptAt: timePtr(r.JobNextAttemptAt),
			SubmittedAt: timePtr(r.JobSubmittedAt), CompletedAt: timePtr(r.JobCompletedAt),
			LastVerifiedAt: timePtr(r.LastVerifiedAt), MissingFields: orNone(r.MissingFields), HoldReason: r.HoldReason,
			Resolution: textPtr(r.Resolution), ResolutionReference: r.ResolutionReference, ResolvedAt: timePtr(r.ResolvedAt),
			Rental:  model.Ref{UUID: r.RentalUuid, Reference: r.RentalReference},
			Branch:  model.Ref{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
			JobUUID: r.JobUuid, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		},
		Summary: objectOf(r.Summary), ResolutionReason: r.ResolutionReason, AttemptLog: []model.Attempt{},
	}
	if r.CorrectsUuid.Valid {
		v := uuid.UUID(r.CorrectsUuid.Bytes)
		d.CorrectsUUID = &v
	}
	if r.ResolvedByUuid.Valid {
		d.ResolvedBy = &model.Actor{
			UUID: r.ResolvedByUuid.Bytes, Name: r.ResolvedByName.String, Surname: r.ResolvedBySurname.String,
			Email: r.ResolvedByEmail.String,
		}
	}
	if r.AttachmentKey.Valid && r.AttachmentKey.String != "" {
		d.Attachment = &model.Attachment{Name: r.AttachmentName, ContentType: r.AttachmentContentType, Size: r.AttachmentSize}
	}
	if r.AccountUuid.Valid {
		d.Account = &model.Ref{UUID: r.AccountUuid.Bytes, Name: r.AccountName.String}
	}
	d.Actions = model.Actions{
		Retry:       retryable(r.JobStatus) && has(ctx, rbac.PermTenantKabisRetry),
		MarkManual:  model.Contains(manualFrom, r.JobStatus) && has(ctx, rbac.PermTenantKabisReconcile),
		Reconcile:   model.Contains(reconcileFrom, r.JobStatus) && has(ctx, rbac.PermTenantKabisReconcile),
		Revalidates: r.JobStatus == jobsusecase.StatusBlocked || r.JobStatus == jobsusecase.StatusWaitingDependency,
	}
	attempts, err := s.q.ListIntegrationAttempts(ctx, db.ListIntegrationAttemptsParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, IntegrationJobID: r.IntegrationJobID,
	})
	if err != nil {
		return d, err
	}
	for _, a := range attempts {
		d.AttemptLog = append(d.AttemptLog, model.Attempt{
			UUID: a.Uuid, AttemptNo: a.AttemptNo, Operation: a.Operation, Trigger: a.Trigger, Outcome: a.Outcome,
			RequestSummary: objectOf(a.RequestSummary), ResponseSummary: objectOf(a.ResponseSummary), Error: a.Error,
			StartedAt: a.StartedAt.Time.UTC(), FinishedAt: a.FinishedAt.Time.UTC(), DurationMs: a.DurationMs,
		})
	}
	return d, nil
}

// Get returns one notification with its masked facts and attempt history.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Detail, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Detail{}, err
	}
	return s.detail(ctx, orgID, r)
}
