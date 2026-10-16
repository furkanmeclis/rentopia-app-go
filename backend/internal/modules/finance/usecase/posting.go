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
	released := []int64{}
	carried := map[string][]releasedPair{}
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
		// What collections paid on the accrual goes back to them first
		// (re-allocated below to the accrual that replaces it).
		pairs, err := s.releaseEntry(ctx, q, orgID, l.ID, reason)
		if err != nil {
			return fmt.Errorf("finance: release allocations of %s: %w", key, err)
		}
		for _, p := range pairs {
			released = append(released, p.collectionID)
		}
		if l.AccountID == acc.ID {
			carried[key] = pairs
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
		row, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindAccrual, Direction: model.Debit, Purpose: w.purpose,
			Amount: w.amount.Numeric(), Currency: f.Currency, EntryDate: dateOf(entryDate),
			DueDate: dateOf(dueAfter(entryDate, acc.Term)), Description: w.desc, SourceType: sourceOfKey(int8Of(w.chargeID)),
			SourceUuid: uuidOf(f.Uuid), RentalID: int8Of(f.ID), ReservationID: int8Of(f.ReservationID),
			RentalChargeID: int8Of(w.chargeID), AccrualKey: pgtype.Text{String: key, Valid: true},
			BranchID: int8Of(branchID), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return fmt.Errorf("finance: accrue %s: %w", key, err)
		}
		// The allocations of the accrual this one replaces follow it.
		if err := s.carryAllocations(ctx, q, orgID, row.ID, w.amount, carried[key]); err != nil {
			return fmt.Errorf("finance: carry allocations of %s: %w", key, err)
		}
	}
	if acc.ID != 0 {
		// The rental's own collections (prepayments) and the money just
		// released pay its open items, oldest first.
		if err := s.reallocateRental(ctx, q, orgID, acc.ID, f.ID, f.ReservationID, released); err != nil {
			return fmt.Errorf("finance: allocate rental %d: %w", f.ID, err)
		}
	}
	if len(released) > 0 {
		s.publish(ctx, model.ResourceCollections, "updated")
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
// it when it applies a payment; the catch-up sweep calls it too): the
// collection entry and its collection document (TEC-718), allocated to the
// open items of the reservation's rental and, for an online payment,
// settled into the provider's settlement account when it is known. Posting
// is idempotent: a payment has at most one collection entry and one
// collection.
func (s *Service) PostPayment(ctx context.Context, orgID, paymentID int64) error {
	var accUUID uuid.UUID
	var collID int64
	err := s.tx(ctx, func(q *db.Queries) error {
		var err error
		collID, _, accUUID, err = s.postPaymentTx(ctx, q, orgID, paymentID)
		return err
	})
	if err != nil {
		return err
	}
	if collID != 0 {
		s.publish(ctx, model.ResourceAccounts, "updated", accUUID)
	}
	return nil
}

// PostRefund posts a succeeded payment refund (idempotent): the refund
// entry naming the payment's collection, the collection's refunded amount
// (allocations released newest first) and, when the collection was
// settled, the money leaving the settlement account. Pending and failed
// refunds post nothing.
func (s *Service) PostRefund(ctx context.Context, orgID, refundID int64) error {
	var accUUID, collUUID, entryUUID uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
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
		// The payment's collection first (it may not be posted yet).
		collID, cu, au, err := s.postPaymentTx(ctx, q, orgID, f.PaymentID)
		if err != nil {
			return err
		}
		customerID := f.CustomerID
		if f.InvoiceCustomerID.Valid {
			customerID = f.InvoiceCustomerID.Int64
		}
		acc, err := s.ensureCustomerAccount(ctx, q, orgID, customerID, f.PickupBranchID, f.Currency)
		if err != nil {
			return err
		}
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
			return err
		}
		date := dateOf(day(f.CreatedAt.Time))
		row, err := q.InsertPaymentRefundCollectionEntry(ctx, db.InsertPaymentRefundCollectionEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Purpose: model.PurposeGeneral, Amount: f.Amount, Currency: f.Currency,
			EntryDate: date, Description: f.ReservationReference, SourceUuid: uuidOf(f.Uuid),
			ReservationID: f.ReservationID, PaymentID: int8Of(f.PaymentID), PaymentRefundID: int8Of(f.ID),
			CollectionID: int8Of(collID), BranchID: int8Of(f.PickupBranchID), CreatedBy: f.CreatedBy,
		})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		accUUID, entryUUID = acc.UUID, row.Uuid
		if collID == 0 {
			return nil
		}
		collUUID = cu
		_ = au
		if err := s.applyRefund(ctx, q, orgID, collID, m(f.Amount), "system:payment_refund"); err != nil {
			return err
		}
		return s.refundSettled(ctx, q, orgID, collID, row.ID, m(f.Amount), date)
	})
	if err != nil {
		return err
	}
	if entryUUID != uuid.Nil {
		s.publish(ctx, model.ResourceAccounts, "updated", accUUID)
		s.publish(ctx, model.ResourceEntries, "created", entryUUID)
	}
	if collUUID != uuid.Nil {
		s.publish(ctx, model.ResourceCollections, "updated", collUUID)
	}
	return nil
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
	// Posted payments without their collection document (TEC-718).
	missing, err := s.q.ListPaymentsWithoutCollection(ctx, 200)
	if err != nil {
		return n, err
	}
	for _, p := range missing {
		if err := s.PostPayment(orgctx.WithOrganizationID(ctx, p.OrganizationID), p.OrganizationID, p.PaymentID.Int64); err != nil {
			s.log.Warn("ledger_post_collection_failed", "organization_id", p.OrganizationID, "payment_id", p.PaymentID.Int64, "error", err)
			continue
		}
		n++
	}
	// Online collections whose settlement account exists now.
	pending, err := s.q.ListPendingSettlements(ctx, 200)
	if err != nil {
		return n, err
	}
	for _, c := range pending {
		octx := orgctx.WithOrganizationID(ctx, c.OrganizationID)
		if err := s.tx(octx, func(q *db.Queries) error { return s.settle(octx, q, c.OrganizationID, c.ID) }); err != nil {
			s.log.Warn("collection_settlement_failed", "organization_id", c.OrganizationID, "collection_id", c.ID, "error", err)
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
