package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Retry is the staff "Yeniden dene" of a send (or of a stuck e-Arşiv
// cancellation).
//
//   - waiting on a dependency: the organization's integrator is looked up
//     again; still none → *DependencyError (409, nothing sent).
//   - pending / failed / needs_verification / submitted: the runner's retry —
//     a status check first; the document is sent again only when the
//     integrator confirms it does not have it (*jobsusecase.VerifyError when
//     the answer is unknown).
func (s *Service) Retry(ctx context.Context, orgID int64, id uuid.UUID) (model.RetryResult, error) {
	if err := s.requireEInvoice(ctx, orgID); err != nil {
		return model.RetryResult{}, err
	}
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.RetryResult{}, err
	}
	jobUUID, jobID, jobStatus := r.JobUuid, r.IntegrationJobID, r.JobStatus
	if r.Status == model.StatusSucceeded && r.CancelJobUuid.Valid {
		jobUUID, jobID, jobStatus = r.CancelJobUuid, r.CancelJobID, r.CancelJobStatus
	}
	if !jobUUID.Valid || !jobID.Valid {
		return model.RetryResult{}, ErrNotRetryable
	}
	switch jobStatus.String {
	case jobsusecase.StatusWaitingDependency, jobsusecase.StatusBlocked:
		return s.reroute(ctx, orgID, r.Uuid, jobID.Int64)
	case jobsusecase.StatusPending, jobsusecase.StatusFailed, jobsusecase.StatusNeedsVerification, jobsusecase.StatusSubmitted:
	default:
		return model.RetryResult{}, ErrNotRetryable
	}
	if r.Status == model.StatusCancelled {
		return model.RetryResult{}, ErrNotRetryable
	}
	res, err := s.runner.Retry(ctx, orgID, uuid.UUID(jobUUID.Bytes))
	switch {
	case errors.Is(err, jobsusecase.ErrBusy):
		return model.RetryResult{}, ErrBusy
	case errors.Is(err, jobsusecase.ErrNotRetryable):
		return model.RetryResult{}, ErrNotRetryable
	case err != nil:
		return model.RetryResult{}, err
	}
	return model.RetryResult{Action: res.Action, Status: res.Status}, nil
}

// reroute sends a held job to the organization's integrator when it now has
// a working one.
func (s *Service) reroute(ctx context.Context, orgID int64, invUUID uuid.UUID, jobID int64) (model.RetryResult, error) {
	r, err := s.routeOf(ctx, s.q, orgID)
	if err != nil {
		return model.RetryResult{}, err
	}
	if !r.ready {
		row, err := s.q.GetInvoice(ctx, db.GetInvoiceParams{OrganizationID: orgID, Uuid: invUUID})
		if err == nil && row.HoldReason != r.hold {
			_ = s.q.SetInvoiceHold(ctx, db.SetInvoiceHoldParams{OrganizationID: orgID, ID: row.ID, HoldReason: r.hold, LastError: holdMessage(r.hold)})
		}
		return model.RetryResult{}, &DependencyError{Reason: r.hold}
	}
	n, err := s.q.RouteInvoiceJob(ctx, db.RouteInvoiceJobParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: jobID, Status: jobsusecase.StatusPending,
		Provider: r.provider, IntegrationAccountID: r.accountID, NextAttemptAt: ts(s.clock()),
	})
	if err != nil {
		return model.RetryResult{}, err
	}
	if n != 1 {
		return model.RetryResult{}, ErrBusy
	}
	st, err := s.runner.Kick(orgctx.WithOrganizationID(ctx, orgID), orgID, jobID)
	if err != nil {
		return model.RetryResult{}, err
	}
	status := st.Status
	if status == "" {
		status = jobsusecase.StatusPending
	}
	return model.RetryResult{Action: "rerouted", Status: status}, nil
}

// ReleaseWaiting is the sweep for sends waiting on a dependency: once the
// organization has a working integrator, each waiting send is routed to it.
func (s *Service) ReleaseWaiting(ctx context.Context) (int, error) {
	rows, err := s.q.ListWaitingInvoiceJobs(ctx, 200)
	if err != nil {
		return 0, err
	}
	ready := map[int64]bool{}
	n := 0
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		octx := orgctx.WithOrganizationID(ctx, row.OrganizationID)
		ok, seen := ready[row.OrganizationID]
		if !seen {
			r, err := s.routeOf(octx, s.q, row.OrganizationID)
			ok = err == nil && r.ready
			ready[row.OrganizationID] = ok
		}
		if !ok {
			continue
		}
		if _, err := s.reroute(octx, row.OrganizationID, row.Uuid, row.JobID); err != nil {
			s.log.Warn("invoice_release_failed", "invoice", row.Uuid, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

func reasonOf(ve *ValidationError, reason string, required bool) string {
	r := strings.TrimSpace(reason)
	switch {
	case required && utf8.RuneCountInString(r) < 3:
		ve.add("reason", "required", "give a reason (at least 3 characters)")
	case utf8.RuneCountInString(r) > 500:
		ve.add("reason", "too_long", "at most 500 characters")
	}
	return r
}

// Cancel cancels an invoice and frees its ledger items:
//   - a draft, an invoice whose send has not started (held, or pending with
//     no attempt) and a failed one (the integrator confirmed it was not
//     issued) are cancelled at once;
//   - an issued e-Arşiv invoice is cancelled through the integrator (a cancel
//     job; the invoice becomes cancelled when the integrator confirms);
//   - an issued e-Fatura answers ErrUseReturn (a return invoice is issued);
//   - a send in flight answers ErrNotCancellable.
func (s *Service) Cancel(ctx context.Context, orgID int64, id uuid.UUID, in model.CancelInput) (model.Detail, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Detail{}, err
	}
	// A draft is a plain record; anything past it is an e-document.
	if r.Status != model.StatusDraft {
		if err := s.requireEInvoice(ctx, orgID); err != nil {
			return model.Detail{}, err
		}
	}
	ve := &ValidationError{}
	reason := reasonOf(ve, in.Reason, r.Status != model.StatusDraft)
	if err := ve.err(); err != nil {
		return model.Detail{}, err
	}
	now := s.clock()
	actor := actorID(ctx)
	local := func(from string, stopJob bool) error {
		return s.tx(ctx, func(q *db.Queries) error {
			if stopJob && r.IntegrationJobID.Valid {
				n, err := q.StopUnsentInvoiceJob(ctx, db.StopUnsentInvoiceJobParams{
					OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: r.IntegrationJobID.Int64,
					LastError: "cancelled before sending: " + reason,
				})
				if err != nil {
					return err
				}
				if n != 1 {
					return ErrBusy
				}
			}
			n, err := q.CancelInvoice(ctx, db.CancelInvoiceParams{
				OrganizationID: orgID, ID: r.ID, Reason: reason, CancelledBy: actor, CancelledAt: ts(now), FromStatuses: []string{from},
			})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrNotCancellable
			}
			_, err = q.ReleaseInvoiceLinks(ctx, db.ReleaseInvoiceLinksParams{
				OrganizationID: orgID, InvoiceID: r.ID, ReleasedAt: ts(now), Reason: model.ReleaseCancelled,
			})
			return err
		})
	}
	switch r.Status {
	case model.StatusDraft:
		err = local(model.StatusDraft, false)
	case model.StatusQueued:
		err = local(model.StatusQueued, true)
		if err == nil && s.runner != nil && r.IntegrationJobID.Valid {
			s.runner.Changed(ctx, orgID, r.IntegrationJobID.Int64)
		}
	case model.StatusFailed:
		err = local(model.StatusFailed, false)
	case model.StatusSucceeded:
		switch {
		case r.InvoiceType != model.TypeSatis || r.ReturnedAt.Valid || r.ReturnedByUuid.Valid:
			return model.Detail{}, ErrNotCancellable
		case r.Profile != model.ProfileEArsiv:
			return model.Detail{}, ErrUseReturn
		case r.CancelJobStatus.Valid && r.CancelJobStatus.String != jobsusecase.StatusFailed:
			return s.Get(ctx, orgID, id) // a cancellation is already on its way
		}
		err = s.requestCancel(ctx, orgID, r, reason)
	default:
		return model.Detail{}, ErrNotCancellable
	}
	if err != nil {
		return model.Detail{}, err
	}
	s.publish(ctx, resourceevents.ActionUpdated, r.Uuid.String())
	return s.Get(ctx, orgID, id)
}

// requestCancel queues the e-Arşiv cancellation on the send's integrator.
func (s *Service) requestCancel(ctx context.Context, orgID int64, r db.GetInvoiceRow, reason string) error {
	if !r.JobProvider.Valid {
		return ErrNotCancellable
	}
	provider := r.JobProvider.String
	if p, ok := s.providers[provider]; !ok || p.Ready() != nil {
		return &DependencyError{Reason: model.HoldAdapterUnavailable}
	}
	now := s.clock()
	var jobID int64
	key := orgKey(orgID) + ":einvoice:cancel:" + r.Uuid.String()
	if r.CancelJobID.Valid {
		// A failed cancellation is tried again under a new key.
		key += ":" + now.Format("20060102150405")
	}
	err := s.tx(ctx, func(q *db.Queries) error {
		payload, _ := json.Marshal(map[string]any{"invoice_uuid": r.Uuid.String(), "number": r.Number.String})
		job, err := q.InsertInvoiceJob(ctx, db.InsertInvoiceJobParams{
			OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, IntegrationAccountID: r.JobAccountID, Provider: provider,
			Kind: model.JobKindCancel, IdempotencyKey: key, Payload: payload, RefUuid: pgtype.UUID{Bytes: r.Uuid, Valid: true},
			MaxAttempts: MaxAttempts, Status: jobsusecase.StatusPending, NextAttemptAt: ts(now),
		})
		if err != nil {
			if notFound(err) {
				return ErrBusy // the same key exists: a cancellation is on its way
			}
			return err
		}
		jobID = job.ID
		return q.SetInvoiceCancelJob(ctx, db.SetInvoiceCancelJobParams{
			OrganizationID: orgID, ID: r.ID, CancelJobID: pgtype.Int8{Int64: job.ID, Valid: true}, Reason: reason,
			CancelledBy: actorID(ctx),
		})
	})
	if err != nil {
		return err
	}
	if s.runner != nil {
		if _, err := s.runner.Kick(orgctx.WithOrganizationID(ctx, orgID), orgID, jobID); err != nil {
			s.log.Warn("invoice_cancel_step_failed", "invoice", r.Uuid, "error", err)
		}
	}
	return nil
}

// Return creates the IADE (return) draft of an issued sales invoice: the
// same receiver and lines, TEMELFATURA instead of TICARIFATURA, and a
// billing reference to the returned invoice. When the return invoice is
// issued, the returned invoice is marked returned and its ledger items are
// freed. Nothing is posted to the ledger: correcting the debt (a reversal
// or an adjustment) is a separate finance action.
func (s *Service) Return(ctx context.Context, orgID int64, id uuid.UUID, in model.ReturnInput) (model.Detail, error) {
	if err := s.requireEInvoice(ctx, orgID); err != nil {
		return model.Detail{}, err
	}
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Detail{}, err
	}
	if r.Status != model.StatusSucceeded || r.InvoiceType != model.TypeSatis || r.ReturnedAt.Valid {
		return model.Detail{}, ErrNotReturnable
	}
	if r.ReturnedByUuid.Valid {
		return s.Get(ctx, orgID, uuid.UUID(r.ReturnedByUuid.Bytes))
	}
	ve := &ValidationError{}
	reason := reasonOf(ve, in.Reason, true)
	issue := s.parseIssueDate(ve, in.IssueDate)
	if err := ve.err(); err != nil {
		return model.Detail{}, err
	}
	lines, err := s.q.ListInvoiceLines(ctx, db.ListInvoiceLinesParams{OrganizationID: orgID, InvoiceID: r.ID})
	if err != nil {
		return model.Detail{}, err
	}
	identity, err := s.openIdentity(orgID, r.Uuid, r.ReceiverIdentityEnc)
	if err != nil {
		return model.Detail{}, err
	}
	profile := r.Profile
	if profile == model.ProfileTicari {
		profile = model.ProfileTemel
	}
	newID := uuid.New()
	enc, err := s.sealIdentity(orgID, newID, identity)
	if err != nil {
		return model.Detail{}, err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		invID, err := q.InsertInvoice(ctx, db.InsertInvoiceParams{
			Uuid: newID, OrganizationID: orgID, BranchID: r.BranchID, AccountID: r.AccountID, CustomerID: r.CustomerID,
			RentalID: r.RentalID, ReturnsInvoiceID: pgtype.Int8{Int64: r.ID, Valid: true}, Profile: profile,
			InvoiceType: model.TypeIade, IssueDate: issue, Currency: r.Currency, ReceiverType: r.ReceiverType,
			ReceiverIdentityType: r.ReceiverIdentityType, ReceiverIdentityEnc: enc, ReceiverIdentityHint: r.ReceiverIdentityHint,
			ReceiverName: r.ReceiverName, ReceiverFirstName: r.ReceiverFirstName, ReceiverLastName: r.ReceiverLastName,
			ReceiverTaxOffice: r.ReceiverTaxOffice, ReceiverAddress: r.ReceiverAddress, ReceiverDistrict: r.ReceiverDistrict,
			ReceiverCity: r.ReceiverCity, ReceiverPostalCode: r.ReceiverPostalCode, ReceiverCountry: r.ReceiverCountry,
			ReceiverEmail: r.ReceiverEmail, ReceiverEinvoice: r.ReceiverEinvoice, ReceiverAlias: r.ReceiverAlias,
			TaxpayerSource: r.TaxpayerSource, TaxpayerCheckedAt: r.TaxpayerCheckedAt,
			LineExtensionAmount: r.LineExtensionAmount, TaxAmount: r.TaxAmount, PayableAmount: r.PayableAmount,
			LinkedAmount: r.PayableAmount, Note: "İade nedeni: " + reason, CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		for _, l := range lines {
			if err := q.InsertInvoiceLine(ctx, db.InsertInvoiceLineParams{
				OrganizationID: orgID, InvoiceID: invID, Position: l.Position, Kind: l.Kind, Name: l.Name, Days: l.Days,
				GrossAmount: l.GrossAmount, NetAmount: l.NetAmount, VatRate: l.VatRate, VatAmount: l.VatAmount,
				ExemptionCode: l.ExemptionCode, ExemptionReason: l.ExemptionReason,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if uniqueViolation(err, "uq_invoices_organization_returns") {
		fresh, gerr := s.row(ctx, orgID, id)
		if gerr == nil && fresh.ReturnedByUuid.Valid {
			return s.Get(ctx, orgID, uuid.UUID(fresh.ReturnedByUuid.Bytes))
		}
		return model.Detail{}, ErrNotReturnable
	}
	if err != nil {
		return model.Detail{}, err
	}
	s.publish(ctx, resourceevents.ActionCreated, newID.String())
	s.publish(ctx, resourceevents.ActionUpdated, r.Uuid.String())
	return s.Get(ctx, orgID, newID)
}

// PortalInvoices lists the issued invoices of a reservation for the
// customer portal (the caller resolved the reservation inside the customer's
// session scope).
func (s *Service) PortalInvoices(ctx context.Context, orgID, reservationID int64, pathPrefix string) ([]model.PortalInvoice, error) {
	rows, err := s.q.ListPortalReservationInvoices(ctx, db.ListPortalReservationInvoicesParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return nil, err
	}
	out := make([]model.PortalInvoice, 0, len(rows))
	for _, r := range rows {
		amount, _ := money.FromNumeric(r.PayableAmount)
		out = append(out, model.PortalInvoice{
			UUID: r.Uuid, Number: r.Number.String, Profile: r.Profile, Type: r.InvoiceType,
			IssueDate: r.IssueDate.Time.Format(time.DateOnly), Payable: amount, Currency: r.Currency,
			DownloadPath: pathPrefix + "/invoices/" + r.Uuid.String() + "/pdf",
		})
	}
	return out, nil
}

// PortalInvoicePDF returns the PDF of an issued invoice of the reservation
// (ErrNotFound for any other invoice).
func (s *Service) PortalInvoicePDF(ctx context.Context, orgID, reservationID int64, invoiceUUID uuid.UUID) ([]byte, string, error) {
	rows, err := s.q.ListPortalReservationInvoices(ctx, db.ListPortalReservationInvoicesParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return nil, "", err
	}
	for _, r := range rows {
		if r.Uuid != invoiceUUID {
			continue
		}
		full, err := s.q.GetInvoice(ctx, db.GetInvoiceParams{OrganizationID: orgID, Uuid: invoiceUUID})
		if err != nil {
			return nil, "", err
		}
		return s.pdfOf(ctx, orgID, full.ID, full.Uuid, full.XmlKey, full.PdfKey, full.Number)
	}
	return nil, "", ErrNotFound
}
