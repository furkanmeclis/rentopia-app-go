package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// operation is the runner operation of one notification kind on one
// provider. Submit files the notification; CheckStatus looks it up by our
// reference (the notification uuid), which is what makes a retry safe.
type operation struct {
	svc      *Service
	provider kabis.Provider
	kind     string
}

func (o *operation) Kind() string { return o.kind }

func (o *operation) load(ctx context.Context, job integrations.Job) (kabis.Notification, map[string]any, error) {
	row, err := o.svc.q.GetKabisNotificationForJob(ctx, db.GetKabisNotificationForJobParams{
		OrganizationID: job.OrganizationID, IntegrationJobID: job.ID,
	})
	if err != nil {
		return kabis.Notification{}, nil, err
	}
	request := map[string]any{"notification_uuid": row.Uuid.String(), "event": row.Event}
	if len(row.MissingFields) > 0 {
		return kabis.Notification{}, request, &DataMissingError{Fields: row.MissingFields}
	}
	n, err := o.svc.unseal(job.OrganizationID, row.Uuid, row.PayloadEnc)
	if err != nil {
		return kabis.Notification{}, request, err
	}
	request["plate"] = n.Vehicle.Plate
	request["drivers"] = len(n.Drivers)
	return n, request, nil
}

// Submit files the notification once and classifies the answer: a receipt
// is done (with the authority's reference); "unavailable" means nothing
// left (retryable); a refusal is final; anything else is unknown and goes
// to a status check, never straight to a second report.
func (o *operation) Submit(ctx context.Context, account integrations.Account, job integrations.Job) (integrations.Result, error) {
	if err := o.provider.Ready(); err != nil {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Message: err.Error()}, nil
	}
	n, request, err := o.load(ctx, job)
	if err != nil {
		// Nothing was sent: the stored notification could not be used.
		return integrations.Result{Outcome: integrations.OutcomeFailed, Request: request, Message: err.Error()}, nil
	}
	receipt, err := o.provider.Report(ctx, account, n)
	switch {
	case err == nil:
		return integrations.Result{
			Outcome: integrations.OutcomeSucceeded, ProviderRef: receipt.Reference, Request: request,
			Response: map[string]any{"reference": receipt.Reference},
		}, nil
	case errors.Is(err, kabis.ErrUnavailable):
		return integrations.Result{Outcome: integrations.OutcomeFailed, Retryable: true, Request: request, Message: err.Error()}, nil
	case errors.Is(err, kabis.ErrRejected):
		return integrations.Result{Outcome: integrations.OutcomeFailed, Request: request, Message: err.Error()}, nil
	case errors.Is(err, integrations.ErrUnsupported):
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Request: request, Message: err.Error()}, err
	}
	return integrations.Uncertain(err, request)
}

// CheckStatus asks the authority whether it has the notification.
func (o *operation) CheckStatus(ctx context.Context, account integrations.Account, job integrations.Job) (integrations.Result, error) {
	var p struct {
		NotificationUUID string `json:"notification_uuid"`
	}
	_ = json.Unmarshal(job.Payload, &p)
	request := map[string]any{"notification_uuid": p.NotificationUUID, "provider_ref": job.ProviderRef}
	res, err := o.provider.Lookup(ctx, account, p.NotificationUUID, job.ProviderRef)
	switch {
	case errors.Is(err, integrations.ErrUnsupported):
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Request: request, Message: err.Error()}, err
	case err != nil:
		return integrations.Result{Outcome: integrations.OutcomeUncertain, Request: request, Message: err.Error()}, nil
	case res.Found:
		return integrations.Result{
			Outcome: integrations.OutcomeSucceeded, ProviderRef: res.Reference, Request: request,
			Response: map[string]any{"found": true, "reference": res.Reference},
		}, nil
	}
	return integrations.Result{
		Outcome: integrations.OutcomeNotFound, Request: request, Response: map[string]any{"found": false},
		Message: "the authority has no record of the notification",
	}, nil
}

// IntegrationJobChanged follows the runner (jobsusecase.Listener): a
// status check stamps the last verification time; every change refreshes
// the live list.
func (s *Service) IntegrationJobChanged(ctx context.Context, orgID int64, st jobsusecase.State, _ integrations.Result) {
	if !strings.HasPrefix(st.Kind, "kabis.") || orgID <= 0 {
		return
	}
	if st.Operation == jobsusecase.OpCheckStatus {
		if err := s.q.TouchKabisVerified(ctx, db.TouchKabisVerifiedParams{
			OrganizationID: orgID, IntegrationJobID: st.ID, VerifiedAt: ts(s.clock()),
		}); err != nil {
			s.log.Warn("kabis_touch_verified_failed", "job", st.UUID, "error", err)
		}
	}
	var p struct {
		NotificationUUID string `json:"notification_uuid"`
	}
	if json.Unmarshal(st.Payload, &p) == nil && p.NotificationUUID != "" {
		s.publish(orgctx.WithOrganizationID(ctx, orgID), resourceevents.ActionUpdated, p.NotificationUUID)
	}
}
