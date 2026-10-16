package usecase

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Rental statuses whose rent is a firm debt: the rent accrues at the
// handover (the vehicle is with the customer) and stays accrued through
// the return; a rental cancelled before the handover never accrued.
var rentAccrues = map[string]bool{"active": true, "return_check": true, "returned": true}

// RentKey / ChargeKey are the accrual keys (single accrual, RT-A23).
func RentKey(rentalID int64) string              { return "rental:" + strconv.FormatInt(rentalID, 10) + ":rent" }
func ChargeKey(chargeID int64) string            { return "rental_charge:" + strconv.FormatInt(chargeID, 10) }
func dueAfter(d time.Time, term int32) time.Time { return d.AddDate(0, 0, int(term)) }

// account is the posting view of an account.
type account struct {
	ID       int64
	UUID     uuid.UUID
	Currency string
	Term     int32
}

// ensureCustomerAccount returns the account of a customer, opening it (at
// branch) on the first posting. Runs on q (the caller's transaction).
func (s *Service) ensureCustomerAccount(ctx context.Context, q *db.Queries, orgID, customerID, branchID int64, currency string) (account, error) {
	c, err := q.GetCustomerDisplay(ctx, db.GetCustomerDisplayParams{OrganizationID: orgID, ID: customerID})
	if err != nil {
		return account{}, fmt.Errorf("finance: customer %d: %w", customerID, err)
	}
	if currency == "" {
		currency = string(money.TRY)
	}
	name := customerName(c.CustomerType, c.FirstName, c.LastName, c.CompanyTitle, c.AnonymizedAt.Valid)
	// The code is a per-organization sequence: opening accounts of one
	// organization is serialised for the rest of the transaction.
	if err := q.LockOrganizationAccountCodes(ctx, orgID); err != nil {
		return account{}, err
	}
	row, err := q.EnsureCustomerAccount(ctx, db.EnsureCustomerAccountParams{
		OrganizationID: orgID, CustomerID: int8Of(customerID), Name: name, BranchID: int8Of(branchID),
		Currency: currency, CreatedBy: actorID(ctx),
	})
	if err != nil {
		return account{}, err
	}
	return account{ID: row.ID, UUID: row.Uuid, Currency: row.Currency, Term: row.PaymentTermDays}, nil
}

// SyncRental reconciles the accruals of a rental with its current state,
// on q (the caller's transaction: handover, extension, charge decision):
//
//   - rent (reservation total, collected by us) once the rental is under
//     way (handover);
//   - every approved charge; proposed, waived and disputed-but-unapproved
//     charges are never a firm debt.
//
// A live accrual whose amount or account no longer matches is reversed
// and accrued again; one that is no longer due is reversed. Calling it
// again changes nothing (idempotent).
func (s *Service) SyncRental(ctx context.Context, q *db.Queries, orgID, rentalID int64) error {
	if q == nil {
		q = s.q
	}
	f, err := q.GetRentalLedgerFacts(ctx, db.GetRentalLedgerFactsParams{OrganizationID: orgID, ID: rentalID})
	if err != nil {
		return fmt.Errorf("finance: rental %d: %w", rentalID, err)
	}
	type want struct {
		amount   money.Money
		purpose  string
		chargeID int64
		desc     string
	}
	desired := map[string]want{}
	if rentAccrues[f.Status] && f.CollectingParty == "us" {
		if total := m(f.TotalAmount); total.IsPositive() {
			desired[RentKey(f.ID)] = want{amount: total, purpose: model.PurposeRent, desc: f.Reference}
		}
	}
	charges, err := q.ListRentalChargesForLedger(ctx, db.ListRentalChargesForLedgerParams{OrganizationID: orgID, RentalID: f.ID})
	if err != nil {
		return err
	}
	reversedCharges := map[int64]bool{}
	for _, ch := range charges {
		// A reversal row (negative amount, reversal_of_id: toll charges)
		// is never accrued itself; the charge it reverses stops being due,
		// so its live accrual is reversed below. One accrual per charge.
		if ch.Reversed {
			reversedCharges[ch.ID] = true
		}
		if ch.Status != "approved" || ch.ReversalOfID.Valid || ch.Reversed || !m(ch.Amount).IsPositive() {
			continue
		}
		desired[ChargeKey(ch.ID)] = want{amount: m(ch.Amount), purpose: model.PurposeCharge, chargeID: ch.ID, desc: f.Reference + " · " + ch.Kind}
	}
	live, err := q.ListLiveRentalAccruals(ctx, db.ListLiveRentalAccrualsParams{OrganizationID: orgID, RentalID: int8Of(f.ID)})
	if err != nil {
		return err
	}
	if len(desired) == 0 && len(live) == 0 {
		return nil
	}
	customerID := f.CustomerID
	if f.InvoiceCustomerID.Valid {
		customerID = f.InvoiceCustomerID.Int64
	}
	branchID := f.PickupBranchID
	if f.ActualPickupBranchID.Valid {
		branchID = f.ActualPickupBranchID.Int64
	}
	var acc account
	if len(desired) > 0 {
		acc, err = s.ensureCustomerAccount(ctx, q, orgID, customerID, branchID, f.Currency)
		if err != nil {
			return err
		}
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
			return err
		}
	}
	today := s.today()
	kept := map[string]bool{}
	for _, l := range live {
		key := l.AccrualKey.String
		w, ok := desired[key]
		reason := ""
		switch {
		case !ok && f.Status == "cancelled":
			reason = model.ReasonRentalCancelled
		case !ok && reversedCharges[l.RentalChargeID.Int64]:
			reason = model.ReasonChargeReversed
		case !ok:
			reason = model.ReasonNoLongerDue
		case l.AccountID != acc.ID:
			reason = model.ReasonAccountChanged
		case !m(l.Amount).Equal(w.amount):
			reason = model.ReasonAmountChanged
		}
		if reason == "" {
			kept[key] = true
			continue
		}
		if _, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
			OrganizationID: orgID, AccountID: l.AccountID, Kind: model.KindReversal, Direction: model.Credit,
			Purpose: l.Purpose, Amount: l.Amount, Currency: l.Currency, EntryDate: dateOf(today), DueDate: l.DueDate,
			Description: f.Reference, Reason: reason, SourceType: sourceOfKey(l.RentalChargeID), SourceUuid: uuidOf(f.Uuid),
			RentalID: int8Of(f.ID), ReservationID: int8Of(f.ReservationID), RentalChargeID: l.RentalChargeID,
			ReversesEntryID: int8Of(l.ID), BranchID: l.BranchID, CreatedBy: actorID(ctx),
		}); err != nil {
			return fmt.Errorf("finance: reverse accrual %s: %w", key, err)
		}
	}
	for key, w := range desired {
		if kept[key] {
			continue
		}
		entryDate := today
		if w.purpose == model.PurposeRent && f.ActualPickupAt.Valid {
			entryDate = day(f.ActualPickupAt.Time)
		}
		if _, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindAccrual, Direction: model.Debit, Purpose: w.purpose,
			Amount: w.amount.Numeric(), Currency: f.Currency, EntryDate: dateOf(entryDate),
			DueDate: dateOf(dueAfter(entryDate, acc.Term)), Description: w.desc, SourceType: sourceOfKey(int8Of(w.chargeID)),
			SourceUuid: uuidOf(f.Uuid), RentalID: int8Of(f.ID), ReservationID: int8Of(f.ReservationID),
			RentalChargeID: int8Of(w.chargeID), AccrualKey: pgtype.Text{String: key, Valid: true},
			BranchID: int8Of(branchID), CreatedBy: actorID(ctx),
		}); err != nil {
			return fmt.Errorf("finance: accrue %s: %w", key, err)
		}
	}
	s.publish(ctx, model.ResourceAccounts, "updated", acc.UUID)
	s.publish(ctx, model.ResourceEntries, "created")
	return nil
}

func sourceOfKey(chargeID pgtype.Int8) string {
	if chargeID.Valid {
		return model.SourceRentalCharge
	}
	return model.SourceRental
}

// PostPayment posts the collection of a confirmed payment (payments calls
// it when it applies a payment; the catch-up sweep calls it too). Posting
// is idempotent: a payment has at most one collection entry.
func (s *Service) PostPayment(ctx context.Context, orgID, paymentID int64) error {
	return s.tx(ctx, func(q *db.Queries) error {
		p, err := q.GetPaymentForLedger(ctx, db.GetPaymentForLedgerParams{OrganizationID: orgID, ID: paymentID})
		if notFound(err) {
			return nil // not a reservation payment
		}
		if err != nil {
			return err
		}
		switch p.Status {
		case "succeeded", "partially_refunded", "refunded":
		default:
			return nil
		}
		customerID := p.CustomerID
		if p.InvoiceCustomerID.Valid {
			customerID = p.InvoiceCustomerID.Int64
		}
		acc, err := s.ensureCustomerAccount(ctx, q, orgID, customerID, p.PickupBranchID, p.Currency)
		if err != nil {
			return err
		}
		at := p.CreatedAt.Time
		if p.VerifiedAt.Valid {
			at = p.VerifiedAt.Time
		}
		purpose := model.PurposeGeneral
		if p.LinkPurpose.String == "prepayment" {
			purpose = model.PurposePrepayment
		}
		row, err := q.InsertPaymentCollectionEntry(ctx, db.InsertPaymentCollectionEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Purpose: purpose, Amount: p.Amount, Currency: p.Currency,
			EntryDate: dateOf(day(at)), Description: p.ReservationReference + " · " + p.Method,
			SourceUuid: uuidOf(p.Uuid), ReservationID: p.ReservationID, PaymentID: int8Of(p.ID),
			BranchID: int8Of(p.PickupBranchID), CreatedBy: p.CreatedBy,
		})
		if notFound(err) {
			return nil // already posted
		}
		if err != nil {
			return err
		}
		s.publish(ctx, model.ResourceAccounts, "updated", acc.UUID)
		s.publish(ctx, model.ResourceEntries, "created", row.Uuid)
		return nil
	})
}

// PostRefund posts a succeeded payment refund (idempotent).
func (s *Service) PostRefund(ctx context.Context, orgID, refundID int64) error {
	return s.tx(ctx, func(q *db.Queries) error {
		f, err := q.GetRefundForLedger(ctx, db.GetRefundForLedgerParams{OrganizationID: orgID, ID: refundID})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if f.Status != "succeeded" {
			return nil
		}
		customerID := f.CustomerID
		if f.InvoiceCustomerID.Valid {
			customerID = f.InvoiceCustomerID.Int64
		}
		acc, err := s.ensureCustomerAccount(ctx, q, orgID, customerID, f.PickupBranchID, f.Currency)
		if err != nil {
			return err
		}
		row, err := q.InsertPaymentRefundEntry(ctx, db.InsertPaymentRefundEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Purpose: model.PurposeGeneral, Amount: f.Amount, Currency: f.Currency,
			EntryDate: dateOf(day(f.CreatedAt.Time)), Description: f.ReservationReference, SourceUuid: uuidOf(f.Uuid),
			ReservationID: f.ReservationID, PaymentID: int8Of(f.PaymentID), PaymentRefundID: int8Of(f.ID),
			BranchID: int8Of(f.PickupBranchID), CreatedBy: f.CreatedBy,
		})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		s.publish(ctx, model.ResourceAccounts, "updated", acc.UUID)
		s.publish(ctx, model.ResourceEntries, "created", row.Uuid)
		return nil
	})
}

// PostPending is the catch-up sweep (worker, every minute): confirmed
// payments and succeeded refunds without their entry are posted. Returns
// how many were posted.
func (s *Service) PostPending(ctx context.Context) (int, error) {
	n := 0
	payments, err := s.q.ListUnpostedPayments(ctx, 200)
	if err != nil {
		return 0, err
	}
	for _, p := range payments {
		if err := s.PostPayment(orgctx.WithOrganizationID(ctx, p.OrganizationID), p.OrganizationID, p.ID); err != nil {
			s.log.Warn("ledger_post_payment_failed", "organization_id", p.OrganizationID, "payment_id", p.ID, "error", err)
			continue
		}
		n++
	}
	refunds, err := s.q.ListUnpostedRefunds(ctx, 200)
	if err != nil {
		return n, err
	}
	for _, f := range refunds {
		if err := s.PostRefund(orgctx.WithOrganizationID(ctx, f.OrganizationID), f.OrganizationID, f.ID); err != nil {
			s.log.Warn("ledger_post_refund_failed", "organization_id", f.OrganizationID, "refund_id", f.ID, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// LinkDocument points a document (an invoice of TEC-722) at an entry. It
// posts nothing: billing a rental never accrues it a second time (RT-A23).
func (s *Service) LinkDocument(ctx context.Context, orgID int64, entry uuid.UUID, documentType, documentRef string) error {
	if !contains([]string{model.DocumentInvoice, model.DocumentStatement, model.DocumentTollCharge}, documentType) {
		return validation("document_type", "invalid", "unknown document type")
	}
	if documentRef == "" || len(documentRef) > 64 {
		return validation("document_ref", "invalid", "document_ref is required (at most 64 characters)")
	}
	e, err := s.q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: entry})
	if notFound(err) {
		return ErrEntryNotFound
	}
	if err != nil {
		return err
	}
	return s.q.InsertLedgerEntryLink(ctx, db.InsertLedgerEntryLinkParams{
		OrganizationID: orgID, EntryID: e.ID, DocumentType: documentType, DocumentRef: documentRef, CreatedBy: actorID(ctx),
	})
}
