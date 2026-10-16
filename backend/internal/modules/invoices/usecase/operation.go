package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/jackc/pgx/v5/pgtype"
)

// sendOperation transports an invoice (einvoice.send). Submit sends the
// stored XML under the invoice's UUID; CheckStatus asks the integrator by
// that UUID, which is what makes a retry safe: a document the integrator
// already has is never sent again.
type sendOperation struct {
	svc      *Service
	provider einvoice.Provider
}

func (o *sendOperation) Kind() string { return model.JobKindSend }

func (o *sendOperation) load(ctx context.Context, job integrations.Job) (db.GetInvoiceForJobRow, map[string]any, error) {
	row, err := o.svc.q.GetInvoiceForJob(ctx, db.GetInvoiceForJobParams{
		OrganizationID: job.OrganizationID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	})
	if err != nil {
		return row, nil, err
	}
	return row, map[string]any{"invoice_uuid": row.Uuid.String(), "number": row.Number.String, "profile": row.Profile, "type": row.InvoiceType}, nil
}

// Submit sends the document once and classifies the answer: delivered or
// accepted; "unavailable" means nothing left (retryable); a refusal is
// final; anything else is unknown and goes to a status check, never
// straight to a second send.
func (o *sendOperation) Submit(ctx context.Context, account integrations.Account, job integrations.Job) (integrations.Result, error) {
	if err := o.provider.Ready(); err != nil {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Message: err.Error()}, nil
	}
	row, request, err := o.load(ctx, job)
	if err != nil {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Message: "invoice not found: " + err.Error()}, nil
	}
	if row.Status == model.StatusCancelled {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Request: request, Message: "the invoice was cancelled"}, nil
	}
	xml, _, err := o.svc.xmlOf(ctx, job.OrganizationID, row.XmlKey, row.Number)
	if err != nil {
		// Nothing was sent: the stored document could not be read.
		return integrations.Result{Outcome: integrations.OutcomeFailed, Retryable: true, Request: request, Message: err.Error()}, nil
	}
	identity, err := o.svc.openIdentity(job.OrganizationID, row.Uuid, row.ReceiverIdentityEnc)
	if err != nil {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Request: request, Message: err.Error()}, nil
	}
	res, err := o.provider.Send(ctx, account, einvoice.Document{
		UUID: row.Uuid.String(), Number: row.Number.String, Profile: row.Profile, Type: row.InvoiceType,
		Receiver: identity, ReceiverAlias: row.ReceiverAlias, XML: xml,
	})
	switch {
	case err == nil && res.State == einvoice.StateSucceeded:
		return integrations.Result{
			Outcome: integrations.OutcomeSucceeded, ProviderRef: res.Reference, Request: request,
			Response: map[string]any{"reference": res.Reference, "state": res.State},
		}, nil
	case err == nil:
		return integrations.Result{
			Outcome: integrations.OutcomeAccepted, ProviderRef: res.Reference, Request: request,
			Response: map[string]any{"reference": res.Reference, "state": res.State},
		}, nil
	case errors.Is(err, einvoice.ErrUnavailable):
		return integrations.Result{Outcome: integrations.OutcomeFailed, Retryable: true, Request: request, Message: err.Error()}, nil
	case errors.Is(err, einvoice.ErrRejected):
		return integrations.Result{Outcome: integrations.OutcomeFailed, Request: request, Message: err.Error()}, nil
	case errors.Is(err, integrations.ErrUnsupported):
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Request: request, Message: err.Error()}, err
	}
	return integrations.Uncertain(err, request)
}

// CheckStatus asks the integrator what happened to the document.
func (o *sendOperation) CheckStatus(ctx context.Context, account integrations.Account, job integrations.Job) (integrations.Result, error) {
	row, request, err := o.load(ctx, job)
	if err != nil {
		return integrations.Result{Outcome: integrations.OutcomeUncertain, Message: err.Error()}, nil
	}
	request["provider_ref"] = job.ProviderRef
	st, err := o.provider.Status(ctx, account, row.Uuid.String(), job.ProviderRef)
	switch {
	case errors.Is(err, integrations.ErrUnsupported):
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Request: request, Message: err.Error()}, err
	case err != nil:
		return integrations.Result{Outcome: integrations.OutcomeUncertain, Request: request, Message: err.Error()}, nil
	case !st.Found:
		return integrations.Result{
			Outcome: integrations.OutcomeNotFound, Request: request, Response: map[string]any{"found": false},
			Message: "the integrator has no document with this UUID",
		}, nil
	}
	resp := map[string]any{"found": true, "state": st.State, "reference": st.Reference}
	switch st.State {
	case einvoice.StateSucceeded, einvoice.StateCancelled:
		return integrations.Result{Outcome: integrations.OutcomeSucceeded, ProviderRef: st.Reference, Request: request, Response: resp}, nil
	case einvoice.StateFailed:
		return integrations.Result{Outcome: integrations.OutcomeFailed, ProviderRef: st.Reference, Request: request, Response: resp, Message: st.Message}, nil
	}
	return integrations.Result{Outcome: integrations.OutcomePending, ProviderRef: st.Reference, Request: request, Response: resp}, nil
}

// cancelOperation cancels an issued e-Arşiv invoice (einvoice.cancel).
type cancelOperation struct {
	svc      *Service
	provider einvoice.Provider
}

func (o *cancelOperation) Kind() string { return model.JobKindCancel }

func (o *cancelOperation) Submit(ctx context.Context, account integrations.Account, job integrations.Job) (integrations.Result, error) {
	row, err := o.svc.q.GetInvoiceForJob(ctx, db.GetInvoiceForJobParams{
		OrganizationID: job.OrganizationID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	})
	if err != nil {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Message: "invoice not found: " + err.Error()}, nil
	}
	request := map[string]any{"invoice_uuid": row.Uuid.String(), "number": row.Number.String}
	err = o.provider.Cancel(ctx, account, row.Uuid.String(), row.ExternalRef, row.CancelReason)
	switch {
	case err == nil:
		return integrations.Result{Outcome: integrations.OutcomeSucceeded, ProviderRef: row.ExternalRef, Request: request,
			Response: map[string]any{"cancelled": true}}, nil
	case errors.Is(err, einvoice.ErrUnavailable):
		return integrations.Result{Outcome: integrations.OutcomeFailed, Retryable: true, Request: request, Message: err.Error()}, nil
	case errors.Is(err, einvoice.ErrNotCancellable), errors.Is(err, einvoice.ErrRejected):
		return integrations.Result{Outcome: integrations.OutcomeFailed, Request: request, Message: err.Error()}, nil
	case errors.Is(err, integrations.ErrUnsupported):
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Request: request, Message: err.Error()}, err
	}
	return integrations.Uncertain(err, request)
}

// CheckStatus: a cancelled document means done; a document still standing
// means the cancellation did not happen (it may be sent again).
func (o *cancelOperation) CheckStatus(ctx context.Context, account integrations.Account, job integrations.Job) (integrations.Result, error) {
	row, err := o.svc.q.GetInvoiceForJob(ctx, db.GetInvoiceForJobParams{
		OrganizationID: job.OrganizationID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	})
	if err != nil {
		return integrations.Result{Outcome: integrations.OutcomeUncertain, Message: err.Error()}, nil
	}
	request := map[string]any{"invoice_uuid": row.Uuid.String()}
	st, err := o.provider.Status(ctx, account, row.Uuid.String(), row.ExternalRef)
	switch {
	case errors.Is(err, integrations.ErrUnsupported):
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Request: request, Message: err.Error()}, err
	case err != nil:
		return integrations.Result{Outcome: integrations.OutcomeUncertain, Request: request, Message: err.Error()}, nil
	case st.Found && st.State == einvoice.StateCancelled:
		return integrations.Result{Outcome: integrations.OutcomeSucceeded, Request: request, Response: map[string]any{"state": st.State}}, nil
	case st.Found:
		return integrations.Result{Outcome: integrations.OutcomeNotFound, Request: request, Response: map[string]any{"state": st.State},
			Message: "the document is not cancelled"}, nil
	}
	return integrations.Result{Outcome: integrations.OutcomeFailed, Request: request, Message: "the integrator has no such document"}, nil
}

// invoiceStatusOf maps the send job's status onto the invoice.
func invoiceStatusOf(jobStatus string) string {
	switch jobStatus {
	case jobsusecase.StatusPending, jobsusecase.StatusWaitingDependency, jobsusecase.StatusBlocked:
		return model.StatusQueued
	case jobsusecase.StatusProcessing, jobsusecase.StatusNeedsVerification:
		return model.StatusSent
	case jobsusecase.StatusSubmitted:
		return model.StatusAwaitingResult
	case jobsusecase.StatusSucceeded, jobsusecase.StatusReconciled, jobsusecase.StatusReportedManually:
		return model.StatusSucceeded
	case jobsusecase.StatusFailed:
		return model.StatusFailed
	}
	return model.StatusQueued
}

// IntegrationJobChanged follows the runner (jobsusecase.Listener): the send
// job's state is mirrored onto the invoice; a succeeded return invoice
// marks the returned invoice and frees its items; a succeeded cancellation
// cancels the invoice and frees its items. The ledger is never touched.
func (s *Service) IntegrationJobChanged(ctx context.Context, orgID int64, st jobsusecase.State, _ integrations.Result) {
	if !strings.HasPrefix(st.Kind, "einvoice.") || orgID <= 0 {
		return
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	row, err := s.q.GetInvoiceForJob(ctx, db.GetInvoiceForJobParams{OrganizationID: orgID, JobID: pgtype.Int8{Int64: st.ID, Valid: true}})
	if err != nil {
		if !notFound(err) {
			s.log.Warn("invoice_job_listener_failed", "job", st.UUID, "error", err)
		}
		return
	}
	now := s.clock()
	also := []string{}
	switch st.Kind {
	case model.JobKindSend:
		status := invoiceStatusOf(st.Status)
		p := db.SyncInvoiceSendStateParams{
			OrganizationID: orgID, ID: row.ID, Status: status, ExternalRef: st.ProviderRef, LastError: st.LastError,
		}
		p.ClearHold = st.Status != jobsusecase.StatusWaitingDependency && st.Status != jobsusecase.StatusBlocked
		if st.Operation != "" {
			p.SentAt = ts(now)
		}
		if status == model.StatusSucceeded {
			p.SucceededAt = ts(now)
		}
		if st.Status == jobsusecase.StatusSucceeded {
			p.LastError = ""
		}
		err = s.tx(ctx, func(q *db.Queries) error {
			if err := q.SyncInvoiceSendState(ctx, p); err != nil {
				return err
			}
			if status == model.StatusSucceeded && row.InvoiceType == model.TypeIade && row.ReturnsInvoiceID.Valid {
				if err := q.MarkInvoiceReturned(ctx, db.MarkInvoiceReturnedParams{
					OrganizationID: orgID, ID: row.ReturnsInvoiceID.Int64, ReturnedAt: ts(now),
				}); err != nil {
					return err
				}
				if _, err := q.ReleaseInvoiceLinks(ctx, db.ReleaseInvoiceLinksParams{
					OrganizationID: orgID, InvoiceID: row.ReturnsInvoiceID.Int64, ReleasedAt: ts(now), Reason: model.ReleaseReturned,
				}); err != nil {
					return err
				}
				if orig, err := q.GetInvoiceRef(ctx, db.GetInvoiceRefParams{OrganizationID: orgID, ID: row.ReturnsInvoiceID.Int64}); err == nil {
					also = append(also, orig.Uuid.String())
				}
			}
			return nil
		})
	case model.JobKindCancel:
		if st.Status != jobsusecase.StatusSucceeded {
			break
		}
		err = s.tx(ctx, func(q *db.Queries) error {
			n, err := q.CancelInvoice(ctx, db.CancelInvoiceParams{
				OrganizationID: orgID, ID: row.ID, Reason: row.CancelReason, CancelledAt: ts(now),
				FromStatuses: []string{model.StatusSucceeded},
			})
			if err != nil || n == 0 {
				return err
			}
			_, err = q.ReleaseInvoiceLinks(ctx, db.ReleaseInvoiceLinksParams{
				OrganizationID: orgID, InvoiceID: row.ID, ReleasedAt: ts(now), Reason: model.ReleaseCancelled,
			})
			return err
		})
	}
	if err != nil {
		s.log.Warn("invoice_job_sync_failed", "invoice", row.Uuid, "job", st.UUID, "error", err)
	}
	s.publish(ctx, resourceevents.ActionUpdated, append([]string{row.Uuid.String()}, also...)...)
}
