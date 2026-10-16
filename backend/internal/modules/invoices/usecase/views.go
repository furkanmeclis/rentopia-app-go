package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/ubl"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// row loads an invoice of the organization inside the caller's branch
// scope (ErrNotFound otherwise: another organization's or branch's invoice
// does not exist for the caller).
func (s *Service) row(ctx context.Context, orgID int64, id uuid.UUID) (db.GetInvoiceRow, error) {
	r, err := s.q.GetInvoice(ctx, db.GetInvoiceParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if orgctx.CheckBranch(ctx, r.BranchID) != nil {
		return r, ErrNotFound
	}
	return r, nil
}

func moneyOf(n pgtype.Numeric, currency string) money.Money {
	m, _ := money.FromNumeric(n)
	return money.NewIn(m.Minor(), money.Currency(currency))
}

func problemsOf(raw []byte) []ubl.Problem {
	out := []ubl.Problem{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func needsAction(status string, jobStatus *string) bool {
	if status == model.StatusFailed {
		return true
	}
	if jobStatus == nil {
		return false
	}
	switch *jobStatus {
	case jobsusecase.StatusNeedsVerification, jobsusecase.StatusWaitingDependency, jobsusecase.StatusBlocked:
		return true
	}
	return false
}

func actorOf(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: uuid.UUID(id.Bytes), Name: name.String, Surname: surname.String, Email: email.String}
}

// Get returns an invoice with its lines, ledger links, problems, jobs and
// attempt history.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Detail, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Detail{}, err
	}
	cur := r.Currency
	payable, linked := moneyOf(r.PayableAmount, cur), moneyOf(r.LinkedAmount, cur)
	d := model.Detail{
		Invoice: model.Invoice{
			UUID: r.Uuid, Number: textPtr(r.Number), Profile: r.Profile, Type: r.InvoiceType, Status: r.Status,
			IssueDate: r.IssueDate.Time.Format(time.DateOnly), Currency: cur, ReceiverType: r.ReceiverType,
			ReceiverName: r.ReceiverName, ReceiverHint: r.ReceiverIdentityHint, EInvoice: r.ReceiverEinvoice,
			LineExtension: moneyOf(r.LineExtensionAmount, cur), Tax: moneyOf(r.TaxAmount, cur), Payable: payable,
			Linked: linked, Rounding: money.NewIn(0, money.Currency(cur)),
			Branch:      model.Ref{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
			ExternalRef: r.ExternalRef, HoldReason: r.HoldReason, LastError: r.LastError, JobStatus: textPtr(r.JobStatus),
			Returned: r.ReturnedAt.Valid, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		},
		Receiver: model.Receiver{
			Type: r.ReceiverType, IdentityType: textPtr(r.ReceiverIdentityType), IdentityHint: r.ReceiverIdentityHint,
			Name: r.ReceiverName, FirstName: r.ReceiverFirstName, LastName: r.ReceiverLastName, TaxOffice: r.ReceiverTaxOffice,
			Address: r.ReceiverAddress, District: r.ReceiverDistrict, City: r.ReceiverCity, PostalCode: r.ReceiverPostalCode,
			Country: r.ReceiverCountry, Email: r.ReceiverEmail, EInvoice: r.ReceiverEinvoice, Alias: r.ReceiverAlias,
			TaxpayerSource: r.TaxpayerSource, TaxpayerCheckedAt: timePtr(r.TaxpayerCheckedAt),
		},
		Note: r.Note, Lines: []model.Line{}, Links: []model.Link{}, ValidationErrors: problemsOf(r.ValidationErrors),
		ValidatedAt: timePtr(r.ValidatedAt), AttemptLog: []jobsusecase.Attempt{}, CancelAttemptLog: []jobsusecase.Attempt{},
		CancelReason: r.CancelReason, CancelledAt: timePtr(r.CancelledAt), FinalizedAt: timePtr(r.FinalizedAt),
		SentAt: timePtr(r.SentAt), SucceededAt: timePtr(r.SucceededAt), ReturnedAt: timePtr(r.ReturnedAt),
		CreatedBy:   actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail),
		CancelledBy: actorOf(r.CancelledByUuid, r.CancelledByName, r.CancelledBySurname, pgtype.Text{}),
	}
	if r.InvoiceType == model.TypeSatis {
		d.Rounding = payable.Sub(linked)
	}
	d.ProblemCount = len(d.ValidationErrors)
	if r.JobAttempts.Valid {
		d.Attempts = r.JobAttempts.Int32
	}
	d.NeedsAction = needsAction(r.Status, d.JobStatus)
	if r.AccountUuid.Valid {
		d.Account = &model.Ref{UUID: uuid.UUID(r.AccountUuid.Bytes), Code: r.AccountCode.String, Name: r.AccountName.String}
	}
	if r.RentalUuid.Valid {
		d.Rental = &model.RentalRef{UUID: uuid.UUID(r.RentalUuid.Bytes), Reference: r.RentalReference.String}
	}
	if r.ReturnsUuid.Valid {
		d.Returns = &model.InvoiceRef{UUID: uuid.UUID(r.ReturnsUuid.Bytes), Number: textPtr(r.ReturnsNumber)}
	}
	if r.ReturnedByUuid.Valid {
		d.ReturnedBy = &model.InvoiceRef{UUID: uuid.UUID(r.ReturnedByUuid.Bytes), Number: textPtr(r.ReturnedByNumber), Status: r.ReturnedByStatus.String}
	}
	lines, err := s.q.ListInvoiceLines(ctx, db.ListInvoiceLinesParams{OrganizationID: orgID, InvoiceID: r.ID})
	if err != nil {
		return model.Detail{}, err
	}
	for _, l := range lines {
		net, vat := moneyOf(l.NetAmount, cur), moneyOf(l.VatAmount, cur)
		rate, _ := money.RateFromNumeric(l.VatRate)
		line := model.Line{
			UUID: l.Uuid, Position: l.Position, Kind: l.Kind, Name: l.Name, Days: l.Days, Gross: moneyOf(l.GrossAmount, cur),
			Net: net, VATRate: rate, VAT: vat, Total: net.Add(vat), ExemptionCode: l.ExemptionCode, ExemptionReason: l.ExemptionReason,
		}
		if l.EntryUuid.Valid {
			e := uuid.UUID(l.EntryUuid.Bytes)
			line.EntryUUID = &e
		}
		d.Lines = append(d.Lines, line)
	}
	links, err := s.q.ListInvoiceLinks(ctx, db.ListInvoiceLinksParams{OrganizationID: orgID, InvoiceID: r.ID})
	if err != nil {
		return model.Detail{}, err
	}
	for _, l := range links {
		lk := model.Link{
			EntryUUID: l.EntryUuid, Kind: l.EntryKind, Purpose: l.EntryPurpose, EntryAmount: moneyOf(l.EntryAmount, cur),
			Amount: moneyOf(l.Amount, cur), EntryDate: l.EntryDate.Time.Format(time.DateOnly), Description: l.EntryDescription,
			Released: l.ReleasedAt.Valid, ReleasedAt: timePtr(l.ReleasedAt), ReleaseReason: l.ReleaseReason,
		}
		if l.RentalUuid.Valid {
			lk.Rental = &model.RentalRef{UUID: uuid.UUID(l.RentalUuid.Bytes), Reference: l.RentalReference.String}
		}
		d.Links = append(d.Links, lk)
	}
	if r.JobUuid.Valid {
		job := &model.JobState{
			UUID: uuid.UUID(r.JobUuid.Bytes), Status: r.JobStatus.String, Attempts: r.JobAttempts.Int32,
			MaxAttempts: r.JobMaxAttempts.Int32, LastError: r.JobLastError.String, Provider: r.JobProvider.String,
			ProviderRef: r.JobProviderRef.String, NextAttemptAt: timePtr(r.JobNextAttemptAt),
			Busy: r.JobLockedUntil.Valid && r.JobLockedUntil.Time.After(time.Now()),
		}
		d.Job = job
		if s.runner != nil {
			if jd, err := s.runner.Detail(ctx, orgID, job.UUID); err == nil {
				d.AttemptLog = jd.AttemptsLog
			}
		}
	}
	if r.CancelJobUuid.Valid {
		d.CancelJob = &model.JobState{UUID: uuid.UUID(r.CancelJobUuid.Bytes), Status: r.CancelJobStatus.String, LastError: r.CancelJobLastError.String}
		if s.runner != nil {
			if jd, err := s.runner.Detail(ctx, orgID, d.CancelJob.UUID); err == nil {
				d.CancelAttemptLog = jd.AttemptsLog
				d.CancelJob.Attempts, d.CancelJob.MaxAttempts, d.CancelJob.Provider = jd.Attempts, jd.MaxAttempts, jd.Provider
				d.CancelJob.ProviderRef, d.CancelJob.NextAttemptAt = jd.ProviderRef, jd.NextAttemptAt
			}
		}
	}
	d.Actions = actionsOf(r, d.Job, d.CancelJob)
	return d, nil
}

func actionsOf(r db.GetInvoiceRow, job, cancelJob *model.JobState) model.Actions {
	a := model.Actions{Preview: r.Status != model.StatusCancelled || r.XmlKey.Valid, DownloadXML: r.XmlKey.Valid, DownloadPDF: r.XmlKey.Valid}
	cancelPending := cancelJob != nil && cancelJob.Status != jobsusecase.StatusFailed && cancelJob.Status != jobsusecase.StatusSucceeded
	switch r.Status {
	case model.StatusDraft:
		a.Edit, a.Validate, a.Finalize, a.Cancel = true, true, true, true
	case model.StatusQueued:
		a.Retry = job != nil
		a.Cancel = job != nil && (job.Status == jobsusecase.StatusWaitingDependency || job.Status == jobsusecase.StatusBlocked ||
			(job.Status == jobsusecase.StatusPending && job.Attempts == 0))
	case model.StatusSent, model.StatusAwaitingResult:
		a.Retry = job != nil
	case model.StatusFailed:
		a.Retry, a.Cancel = job != nil, true
	case model.StatusSucceeded:
		a.Cancel = r.Profile == model.ProfileEArsiv && r.InvoiceType == model.TypeSatis && !r.ReturnedAt.Valid &&
			!r.ReturnedByUuid.Valid && !cancelPending
		a.Return = r.InvoiceType == model.TypeSatis && !r.ReturnedAt.Valid && !r.ReturnedByUuid.Valid && !cancelPending
		a.Retry = cancelJob != nil && cancelJob.Status != jobsusecase.StatusSucceeded
	}
	return a
}

// List returns a page of invoices (branch scope applied).
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Invoice, int64, error) {
	p := db.ListInvoicesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses),
		Profiles: nilIfEmpty(f.Profiles), Types: nilIfEmpty(f.Types), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	if len(f.BranchUUIDs) > 0 {
		ids, err := s.branchIDs(ctx, orgID, f.BranchUUIDs)
		if err != nil {
			return nil, 0, err
		}
		p.FilterBranchIds = ids
	}
	if f.AccountUUID != nil {
		acc, err := s.q.GetInvoiceAccountByUUID(ctx, db.GetInvoiceAccountByUUIDParams{OrganizationID: orgID, Uuid: *f.AccountUUID})
		if notFound(err) {
			return []model.Invoice{}, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		p.AccountID = pgtype.Int8{Int64: acc.ID, Valid: true}
	}
	if f.RentalUUID != nil {
		r, err := s.q.GetInvoiceRentalByUUID(ctx, db.GetInvoiceRentalByUUIDParams{OrganizationID: orgID, Uuid: *f.RentalUUID})
		if notFound(err) {
			return []model.Invoice{}, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		p.RentalID = pgtype.Int8{Int64: r.ID, Valid: true}
	}
	if f.From != nil {
		if t, err := time.Parse(time.DateOnly, *f.From); err == nil {
			p.DateFrom = pgtype.Date{Time: t, Valid: true}
		}
	}
	if f.To != nil {
		if t, err := time.Parse(time.DateOnly, *f.To); err == nil {
			p.DateTo = pgtype.Date{Time: t, Valid: true}
		}
	}
	if f.NeedsAction != nil {
		p.NeedsAction = pgtype.Bool{Bool: *f.NeedsAction, Valid: true}
	}
	rows, err := s.q.ListInvoices(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Invoice, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		cur := r.Currency
		payable, linked := moneyOf(r.PayableAmount, cur), moneyOf(r.LinkedAmount, cur)
		it := model.Invoice{
			UUID: r.Uuid, Number: textPtr(r.Number), Profile: r.Profile, Type: r.InvoiceType, Status: r.Status,
			IssueDate: r.IssueDate.Time.Format(time.DateOnly), Currency: cur, ReceiverType: r.ReceiverType,
			ReceiverName: r.ReceiverName, ReceiverHint: r.ReceiverIdentityHint, EInvoice: r.ReceiverEinvoice,
			LineExtension: moneyOf(r.LineExtensionAmount, cur), Tax: moneyOf(r.TaxAmount, cur), Payable: payable,
			Linked: linked, Rounding: money.NewIn(0, money.Currency(cur)),
			Branch:      model.Ref{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
			ExternalRef: r.ExternalRef, HoldReason: r.HoldReason, LastError: r.LastError, JobStatus: textPtr(r.JobStatus),
			Returned: r.ReturnedAt.Valid, ProblemCount: len(problemsOf(r.ValidationErrors)),
			CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if r.InvoiceType == model.TypeSatis {
			it.Rounding = payable.Sub(linked)
		}
		if r.JobAttempts.Valid {
			it.Attempts = r.JobAttempts.Int32
		}
		it.NeedsAction = needsAction(r.Status, it.JobStatus)
		if r.AccountUuid.Valid {
			it.Account = &model.Ref{UUID: uuid.UUID(r.AccountUuid.Bytes), Code: r.AccountCode.String, Name: r.AccountName.String}
		}
		if r.RentalUuid.Valid {
			it.Rental = &model.RentalRef{UUID: uuid.UUID(r.RentalUuid.Bytes), Reference: r.RentalReference.String}
		}
		if r.ReturnsUuid.Valid {
			it.Returns = &model.InvoiceRef{UUID: uuid.UUID(r.ReturnsUuid.Bytes), Number: textPtr(r.ReturnsNumber)}
		}
		out = append(out, it)
	}
	return out, total, nil
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

// branchIDs resolves branch filter uuids of the organization (an unknown
// uuid matches nothing).
func (s *Service) branchIDs(ctx context.Context, orgID int64, ids []uuid.UUID) ([]int64, error) {
	out := []int64{}
	for _, id := range ids {
		b, err := s.q.GetInvoiceBranchByUUID(ctx, db.GetInvoiceBranchByUUIDParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			out = append(out, 0)
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, b.ID)
	}
	return out, nil
}

// Facets returns the status, profile and branch counts and the number of
// invoices needing action (nav badge, finance screen).
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	scope := orgctx.BranchScope(ctx)
	out := model.Facets{Status: []model.Facet{}, Profile: []model.Facet{}, Branch: []model.Facet{}}
	st, err := s.q.InvoiceStatusFacets(ctx, db.InvoiceStatusFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range st {
		out.Status = append(out.Status, model.Facet{Value: r.Status, Count: r.Total})
	}
	pr, err := s.q.InvoiceProfileFacets(ctx, db.InvoiceProfileFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range pr {
		out.Profile = append(out.Profile, model.Facet{Value: r.Profile, Count: r.Total})
	}
	br, err := s.q.InvoiceBranchFacets(ctx, db.InvoiceBranchFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range br {
		out.Branch = append(out.Branch, model.Facet{Value: r.BranchUuid.String(), Label: r.BranchCode + " · " + r.BranchName, Count: r.Total})
	}
	out.NeedsAction, err = s.q.CountInvoicesNeedingAction(ctx, db.CountInvoicesNeedingActionParams{OrganizationID: orgID, BranchIds: scope})
	return out, err
}

// UninvoicedRentals lists rentals with invoiceable items (bulk picker).
func (s *Service) UninvoicedRentals(ctx context.Context, orgID int64, f model.RentalFilter) ([]model.UninvoicedRental, int64, error) {
	p := db.ListUninvoicedRentalsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), LimitCount: f.Limit, OffsetCount: f.Offset}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	if len(f.UUIDs) > 0 {
		p.RentalUuids = f.UUIDs
	}
	rows, err := s.q.ListUninvoicedRentals(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.UninvoicedRental, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		amount, _ := money.FromNumeric(r.OpenAmount)
		out = append(out, model.UninvoicedRental{
			UUID: r.Uuid, Reference: r.Reference, Status: r.Status, CustomerName: r.CustomerName,
			Branch: model.Ref{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName}, OpenAmount: amount,
			ItemCount: r.ItemCount, FirstDate: r.FirstDate.Time.Format(time.DateOnly),
			PickupAt: timePtr(r.ActualPickupAt), ReturnAt: timePtr(r.ActualReturnAt),
		})
	}
	return out, total, nil
}
