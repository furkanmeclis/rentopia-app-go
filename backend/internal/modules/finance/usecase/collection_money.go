package usecase

// Collections: refunds, voids, online payments and their settlement
// (TEC-718, docs/design/collections.md).
//
//   - A refund gives money of one collection back. An online (or any
//     payment-backed) collection is refunded through the payments module,
//     whose provider result decides: the finance entry is posted only for a
//     succeeded refund (PostRefund), a failed one leaves the collection as
//     it was. A manual collection posts the refund entry and the cash
//     movement out of the till / bank account at once. Either way the
//     collection's allocations shrink (newest first) so that what stays
//     allocated never exceeds what is left of it.
//   - A void cancels a manual collection recorded by mistake (no refund
//     yet): its allocations are released, its entry and cash movement are
//     reversed.
//   - An online payment becomes a collection when it is posted
//     (PostPayment): amount credited to the party, gross / commission / net
//     kept apart. When the provider's settlement account is known (a cash
//     account with provider = iyzico) the gross goes in and the commission
//     goes out as an expense (category payment_commission), leaving the net.

import (
	"context"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// PaymentRefunder is the payments port of an online refund (the existing
// payments refund: provider call, payment_refunds row, ledger posting on
// success).
type PaymentRefunder interface {
	Refund(ctx context.Context, orgID int64, id uuid.UUID, in paymodel.RefundInput, ip string) (paymodel.Payment, error)
}

// SetPayments wires the payments refund (online collection refunds).
func (s *Service) SetPayments(p PaymentRefunder) { s.payments = p }

// RefundCollection pays (part of) a collection back (tenant.payment.refund,
// reason required). See the file comment for online vs manual.
func (s *Service) RefundCollection(ctx context.Context, orgID int64, id uuid.UUID, in model.CollectionRefundInput, ip string) (model.Collection, error) {
	if !has(ctx, rbac.PermTenantPaymentRefund) {
		return model.Collection{}, ErrForbidden
	}
	c, err := s.GetCollection(ctx, orgID, id)
	if err != nil {
		return model.Collection{}, err
	}
	ve := &ValidationError{}
	reason, rerr := requireReason("reason", in.Reason)
	if rerr != nil {
		ve.add("reason", "required", "a reason is required (3-500 characters)")
	}
	if c.Status == model.CollectionVoid || !c.Refundable.IsPositive() {
		return model.Collection{}, stateErr("not_refundable", "the collection has nothing left to refund")
	}
	amount := c.Refundable
	if in.Amount != nil {
		amount = *in.Amount
	}
	if !amount.IsPositive() || amount.Cmp(c.Refundable) > 0 {
		ve.add("amount", "out_of_range", "the amount must be positive and at most "+c.Refundable.String())
	}
	date := optionalDate(ve, "refund_date", in.RefundDate, s.today())
	if c.PaymentUUID != nil && in.CashAccountUUID != nil {
		ve.add("cash_account_uuid", "not_applicable", "a payment is refunded through the payments module")
	}
	cash := in.CashAccountUUID
	if cash == nil && c.CashAccount != nil && c.PaymentUUID == nil {
		cash = &c.CashAccount.UUID
	}
	if c.PaymentUUID == nil && cash == nil && c.Method != model.MethodOther {
		ve.add("cash_account_uuid", "required", "choose the till or bank account the money leaves")
	}
	if err := ve.orNil(); err != nil {
		return model.Collection{}, err
	}
	if c.PaymentUUID != nil {
		if s.payments == nil {
			return model.Collection{}, stateErr("payments_unavailable", "online refunds are not available")
		}
		// The payments module calls the provider and posts the refund entry
		// (and the collection side, PostRefund) only when it succeeded.
		if _, err := s.payments.Refund(ctx, orgID, *c.PaymentUUID, paymodel.RefundInput{Amount: &amount, Reason: reason}, ip); err != nil {
			s.publish(ctx, model.ResourceCollections, "updated", id)
			return model.Collection{}, err
		}
		s.publish(ctx, model.ResourceCollections, "updated", id)
		return s.GetCollection(ctx, orgID, id)
	}
	row, err := s.collectionRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Collection{}, err
	}
	var entryUUID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: row.AccountID}); err != nil {
			return err
		}
		if err := s.applyRefund(ctx, q, orgID, row.ID, amount, reason); err != nil {
			return err
		}
		desc := strings.TrimSpace(row.Code + " · " + c.Account.Name)
		e, err := q.InsertCollectionRefundEntry(ctx, db.InsertCollectionRefundEntryParams{
			OrganizationID: orgID, AccountID: row.AccountID, Purpose: row.Purpose, Amount: amount.Numeric(), Currency: row.Currency,
			EntryDate: dateOf(date), Description: desc, Reason: reason, SourceUuid: uuidOf(row.Uuid), RentalID: row.RentalID,
			ReservationID: row.ReservationID, CollectionID: int8Of(row.ID), BranchID: row.BranchID, CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		entryUUID = e.Uuid
		if cash != nil {
			return s.settleEntry(ctx, q, orgID, *cash, e.ID, model.MoveRefund, amount, row.Currency, date, reason)
		}
		return nil
	})
	if err != nil {
		return model.Collection{}, err
	}
	s.publish(ctx, model.ResourceCollections, "updated", id)
	s.publish(ctx, model.ResourceAccounts, "updated", c.Account.UUID)
	s.publish(ctx, model.ResourceEntries, "created", entryUUID)
	return s.GetCollection(ctx, orgID, id)
}

// applyRefund records amount more refunded on a collection (locked),
// releasing its newest allocations first so that what stays allocated
// never exceeds what is left of it.
func (s *Service) applyRefund(ctx context.Context, q *db.Queries, orgID, collectionID int64, amount money.Money, reason string) error {
	if _, err := q.LockCollection(ctx, db.LockCollectionParams{OrganizationID: orgID, ID: collectionID}); err != nil {
		return err
	}
	c, err := q.GetCollectionRowByID(ctx, db.GetCollectionRowByIDParams{OrganizationID: orgID, ID: collectionID})
	if err != nil {
		return err
	}
	if c.Status == model.CollectionVoid {
		return stateErr("not_refundable", "the collection is void")
	}
	total := m(c.Amount)
	refunded := m(c.RefundedAmount).Add(amount)
	if refunded.Cmp(total) > 0 {
		return stateErr("not_refundable", "the refund exceeds what is left of the collection")
	}
	allocated, err := q.CollectionAllocated(ctx, db.CollectionAllocatedParams{OrganizationID: orgID, CollectionID: collectionID})
	if err != nil {
		return err
	}
	if over := m(allocated).Sub(total.Sub(refunded)); over.IsPositive() {
		if _, err := s.release(ctx, q, orgID, collectionID, over, model.AllocSourceRefund, reason); err != nil {
			return err
		}
	}
	status := model.CollectionPartiallyRefunded
	if refunded.Equal(total) {
		status = model.CollectionRefunded
	}
	n, err := q.SetCollectionRefunded(ctx, db.SetCollectionRefundedParams{
		OrganizationID: orgID, ID: collectionID, RefundedAmount: refunded.Numeric(), Status: status,
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return stateErr("not_refundable", "the collection changed in between; reload")
	}
	return nil
}

// VoidCollection cancels a manual collection recorded by mistake
// (tenant.finance.ledger.adjust, reason): allocations released, entry and
// cash movement reversed.
func (s *Service) VoidCollection(ctx context.Context, orgID int64, id uuid.UUID, in model.CollectionVoidInput) (model.Collection, error) {
	if !has(ctx, rbac.PermTenantFinanceLedgerAdjust) {
		return model.Collection{}, ErrForbidden
	}
	row, err := s.collectionRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Collection{}, err
	}
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return model.Collection{}, err
	}
	full, err := s.q.GetCollectionRowByID(ctx, db.GetCollectionRowByIDParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Collection{}, err
	}
	if _, err := s.voidCollection(ctx, orgID, full, reason); err != nil {
		return model.Collection{}, err
	}
	return s.GetCollection(ctx, orgID, id)
}

// voidCollection is the void (also the reversal of a collection's entry).
// Returns the reversal entry's uuid.
func (s *Service) voidCollection(ctx context.Context, orgID int64, c db.GetCollectionRowByIDRow, reason string) (uuid.UUID, error) {
	if c.PaymentID.Valid {
		return uuid.Nil, stateErr("payment_collection", "a payment's collection is refunded, not voided")
	}
	if c.Status != model.CollectionRecorded || m(c.RefundedAmount).IsPositive() {
		return uuid.Nil, stateErr("not_voidable", "only a collection without refunds can be voided")
	}
	e, err := s.q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: c.LedgerEntryUuid})
	if err != nil {
		return uuid.Nil, err
	}
	if e.Reversed {
		return uuid.Nil, stateErr("already_reversed", "the entry is already reversed")
	}
	var out uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: c.AccountID}); err != nil {
			return err
		}
		allocated, err := q.CollectionAllocated(ctx, db.CollectionAllocatedParams{OrganizationID: orgID, CollectionID: c.ID})
		if err != nil {
			return err
		}
		if _, err := s.release(ctx, q, orgID, c.ID, m(allocated), model.AllocSourceVoid, reason); err != nil {
			return err
		}
		if out, err = s.insertReversal(ctx, q, orgID, e, reason); err != nil {
			return err
		}
		n, err := q.VoidCollection(ctx, db.VoidCollectionParams{
			OrganizationID: orgID, ID: c.ID, VoidReason: reason, VoidedBy: actorID(ctx),
			VoidedAt: pgtype.Timestamptz{Time: s.clock(), Valid: true},
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("not_voidable", "the collection changed in between; reload")
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	s.publish(ctx, model.ResourceCollections, "updated", c.Uuid)
	s.publish(ctx, model.ResourceEntries, "created", out)
	return out, nil
}

// insertReversal posts the exact opposite of one entry and reverses the
// cash movement that settled it (caller's transaction, account locked).
func (s *Service) insertReversal(ctx context.Context, q *db.Queries, orgID int64, e db.GetLedgerEntryRowRow, reason string) (uuid.UUID, error) {
	dir := model.Credit
	if e.Direction == model.Credit {
		dir = model.Debit
	}
	row, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		OrganizationID: orgID, AccountID: e.AccountID, Kind: model.KindReversal, Direction: dir, Purpose: e.Purpose,
		Amount: e.Amount, Currency: e.Currency, EntryDate: dateOf(s.today()), DueDate: e.DueDate, Description: e.Description,
		Reason: reason, SourceType: e.SourceType, SourceUuid: e.SourceUuid, RentalID: e.RentalID, ReservationID: e.ReservationID,
		RentalChargeID: e.RentalChargeID, PaymentID: e.PaymentID, PaymentRefundID: e.PaymentRefundID,
		ReversesEntryID: int8Of(e.ID), BranchID: e.BranchID, CreatedBy: actorID(ctx),
	})
	if uniqueViolation(err) {
		return uuid.Nil, stateErr("already_reversed", "the entry is already reversed")
	}
	if err != nil {
		return uuid.Nil, err
	}
	return row.Uuid, s.reverseSettlement(ctx, q, orgID, e.ID, reason)
}

// ------------------------------------------------------- online payments

// postPaymentTx posts a confirmed payment on q: the collection entry
// (once, uq_ledger_entries_payment) and its collection document (once,
// uq_collections_organization_payment, serialised by the account lock),
// allocated to the open items of the reservation's rental and settled when
// the provider's settlement account is known. Returns the collection id
// (0 when the payment is not posted: not a reservation payment, not
// confirmed).
func (s *Service) postPaymentTx(ctx context.Context, q *db.Queries, orgID, paymentID int64) (int64, uuid.UUID, uuid.UUID, error) {
	p, err := q.GetPaymentForLedger(ctx, db.GetPaymentForLedgerParams{OrganizationID: orgID, ID: paymentID})
	if notFound(err) {
		return 0, uuid.Nil, uuid.Nil, nil // not a reservation payment
	}
	if err != nil {
		return 0, uuid.Nil, uuid.Nil, err
	}
	switch p.Status {
	case "succeeded", "partially_refunded", "refunded":
	default:
		return 0, uuid.Nil, uuid.Nil, nil
	}
	customerID := p.CustomerID
	if p.InvoiceCustomerID.Valid {
		customerID = p.InvoiceCustomerID.Int64
	}
	acc, err := s.ensureCustomerAccount(ctx, q, orgID, customerID, p.PickupBranchID, p.Currency)
	if err != nil {
		return 0, uuid.Nil, uuid.Nil, err
	}
	if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
		return 0, uuid.Nil, uuid.Nil, err
	}
	if c, err := q.GetCollectionIDByPayment(ctx, db.GetCollectionIDByPaymentParams{OrganizationID: orgID, PaymentID: int8Of(p.ID)}); err == nil {
		return c.ID, c.Uuid, acc.UUID, nil
	} else if !notFound(err) {
		return 0, uuid.Nil, uuid.Nil, err
	}
	at := p.CreatedAt.Time
	if p.VerifiedAt.Valid {
		at = p.VerifiedAt.Time
	}
	purpose := model.PurposeGeneral
	if p.LinkPurpose.String == "prepayment" {
		purpose = model.PurposePrepayment
	}
	if err := q.LockOrganizationCollectionCodes(ctx, orgID); err != nil {
		return 0, uuid.Nil, uuid.Nil, err
	}
	code, err := q.NextCollectionCode(ctx, orgID)
	if err != nil {
		return 0, uuid.Nil, uuid.Nil, err
	}
	entryID := int64(0)
	entryDate := dateOf(day(at))
	row, err := q.InsertPaymentCollectionEntry(ctx, db.InsertPaymentCollectionEntryParams{
		OrganizationID: orgID, AccountID: acc.ID, Purpose: purpose, Amount: p.Amount, Currency: p.Currency,
		EntryDate: entryDate, Description: joinParts(p.ReservationReference, code),
		SourceUuid: uuidOf(p.Uuid), ReservationID: p.ReservationID, PaymentID: int8Of(p.ID),
		BranchID: int8Of(p.PickupBranchID), CreatedBy: p.CreatedBy,
	})
	switch {
	case err == nil:
		entryID = row.ID
		s.publish(ctx, model.ResourceEntries, "created", row.Uuid)
	case notFound(err):
		prev, err := q.GetPaymentCollectionEntry(ctx, db.GetPaymentCollectionEntryParams{OrganizationID: orgID, PaymentID: int8Of(p.ID)})
		if err != nil {
			return 0, uuid.Nil, uuid.Nil, err
		}
		entryID, entryDate, purpose = prev.ID, prev.EntryDate, prev.Purpose
	default:
		return 0, uuid.Nil, uuid.Nil, err
	}
	facts, err := q.GetPaymentSettlementFacts(ctx, db.GetPaymentSettlementFactsParams{OrganizationID: orgID, ID: p.ID})
	if err != nil {
		return 0, uuid.Nil, uuid.Nil, err
	}
	amount := m(p.Amount)
	method, provider, settlement := p.Method, model.ProviderManual, model.SettlementNone
	gross, commission, net := amount, money.Zero(), amount
	if p.Provider != model.ProviderManual {
		method, provider, settlement = model.MethodOnline, p.Provider, model.SettlementPending
		if g := m(facts.GrossAmount); g.IsPositive() {
			gross, commission, net = g, m(facts.CommissionAmount), m(facts.NetAmount)
		}
	}
	if !contains(model.CollectionMethods, method) {
		method = model.MethodOther
	}
	if purpose != model.PurposeGeneral && purpose != model.PurposePrepayment && purpose != model.PurposeDeposit {
		purpose = model.PurposeGeneral
	}
	var rentalID pgtype.Int8
	if p.ReservationID.Valid {
		if id, err := q.GetReservationRentalID(ctx, db.GetReservationRentalIDParams{OrganizationID: orgID, ReservationID: p.ReservationID.Int64}); err == nil {
			rentalID = int8Of(id)
		} else if !notFound(err) {
			return 0, uuid.Nil, uuid.Nil, err
		}
	}
	coll, err := q.InsertCollection(ctx, db.InsertCollectionParams{
		Uuid: uuid.New(), OrganizationID: orgID, Code: code, AccountID: acc.ID, Method: method, Provider: provider, Purpose: purpose,
		Currency: p.Currency, Amount: p.Amount, GrossAmount: gross.Numeric(), CommissionAmount: commission.Numeric(),
		NetAmount: net.Numeric(), CollectionDate: entryDate, ReferenceNo: facts.ProviderPaymentID.String,
		BranchID: int8Of(p.PickupBranchID), RentalID: rentalID, ReservationID: p.ReservationID, PaymentID: int8Of(p.ID),
		LedgerEntryID: entryID, SettlementStatus: settlement, CreatedBy: p.CreatedBy,
	})
	if err != nil {
		return 0, uuid.Nil, uuid.Nil, err
	}
	if rentalID.Valid && purpose != model.PurposeDeposit {
		if _, err := s.allocate(ctx, q, orgID, allocation{
			collectionID: coll.ID, accountID: acc.ID, mode: model.AllocateAuto, rentalID: rentalID.Int64, source: model.AllocSourceAuto,
		}); err != nil {
			return 0, uuid.Nil, uuid.Nil, err
		}
	}
	if settlement == model.SettlementPending {
		if err := s.settle(ctx, q, orgID, coll.ID); err != nil {
			return 0, uuid.Nil, uuid.Nil, err
		}
	}
	s.publish(ctx, model.ResourceCollections, "created", coll.Uuid)
	return coll.ID, coll.Uuid, acc.UUID, nil
}

// settle records the net settlement of an online collection in the
// provider's settlement account when it is known: the gross comes in, the
// commission goes out as an expense (category payment_commission). Without
// an active settlement account the collection stays pending (the sweep
// settles it once one exists). Runs on q; idempotent (settlement_status).
func (s *Service) settle(ctx context.Context, q *db.Queries, orgID, collectionID int64) error {
	c, err := q.GetCollectionRowByID(ctx, db.GetCollectionRowByIDParams{OrganizationID: orgID, ID: collectionID})
	if err != nil {
		return err
	}
	if c.SettlementStatus != model.SettlementPending || c.Status == model.CollectionVoid {
		return nil
	}
	acct, err := q.GetProviderSettlementAccount(ctx, db.GetProviderSettlementAccountParams{
		OrganizationID: orgID, Provider: pgtype.Text{String: c.Provider, Valid: true}, Currency: c.Currency,
	})
	if notFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := q.LockCashAccounts(ctx, db.LockCashAccountsParams{OrganizationID: orgID, Ids: []int64{acct.ID}}); err != nil {
		return err
	}
	gross := m(c.GrossAmount)
	if !gross.IsPositive() {
		gross = m(c.Amount)
	}
	date := c.CollectionDate
	desc := strings.TrimSpace(c.Code + " · " + c.Provider + " " + c.ReferenceNo)
	var movementID int64
	var movementUUID uuid.UUID
	if prev, err := q.GetMovementByEntry(ctx, db.GetMovementByEntryParams{OrganizationID: orgID, LedgerEntryID: int8Of(c.LedgerEntryID)}); err == nil {
		movementID, movementUUID = prev.ID, prev.Uuid
	} else if !notFound(err) {
		return err
	} else {
		mv, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
			OrganizationID: orgID, CashAccountID: acct.ID, Kind: model.MoveCollection, Direction: model.DirIn, Amount: gross.Numeric(),
			Currency: c.Currency, MovementDate: date, Description: desc, BranchID: acct.BranchID,
			LedgerEntryID: int8Of(c.LedgerEntryID), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		movementID, movementUUID = mv.ID, mv.Uuid
	}
	var expenseID pgtype.Int8
	var expenseUUID uuid.UUID
	if commission := m(c.CommissionAmount); commission.IsPositive() {
		cat, err := q.GetCommissionCategoryID(ctx, orgID)
		if notFound(err) {
			if err := SeedCategories(ctx, q, orgID); err != nil {
				return err
			}
			cat, err = q.GetCommissionCategoryID(ctx, orgID)
		}
		if err != nil {
			return err
		}
		text := strings.TrimSpace(c.Provider + " komisyonu · " + c.Code)
		ex, err := q.InsertExpense(ctx, db.InsertExpenseParams{
			OrganizationID: orgID, CategoryID: cat, ExpenseDate: date, Description: text, SupplierName: c.Provider,
			DocumentNo: c.ReferenceNo, Currency: c.Currency, NetAmount: commission.Numeric(), VatRate: money.Zero().Numeric(),
			VatAmount: money.Zero().Numeric(), TotalAmount: commission.Numeric(), BranchID: c.BranchID,
			CashAccountID: int8Of(acct.ID), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		if _, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
			OrganizationID: orgID, CashAccountID: acct.ID, Kind: model.MoveExpense, Direction: model.DirOut, Amount: commission.Numeric(),
			Currency: c.Currency, MovementDate: date, Description: text, BranchID: acct.BranchID, CategoryID: int8Of(cat),
			ExpenseID: int8Of(ex.ID), CreatedBy: actorID(ctx),
		}); err != nil {
			return err
		}
		expenseID, expenseUUID = int8Of(ex.ID), ex.Uuid
	}
	if _, err := q.SettleCollection(ctx, db.SettleCollectionParams{
		OrganizationID: orgID, ID: c.ID, SettledAt: pgtype.Timestamptz{Time: s.clock(), Valid: true},
		CashAccountID: int8Of(acct.ID), CashMovementID: int8Of(movementID), CommissionExpenseID: expenseID,
	}); err != nil {
		return err
	}
	s.publish(ctx, model.ResourceMovements, "created", movementUUID)
	s.publish(ctx, model.ResourceCashAccounts, "updated", acct.Uuid)
	if expenseUUID != uuid.Nil {
		s.publish(ctx, model.ResourceExpenses, "created", expenseUUID)
	}
	s.publish(ctx, model.ResourceCollections, "updated", c.Uuid)
	return nil
}

// refundSettled pays a succeeded provider refund out of the settlement
// account the collection was settled into (caller's transaction).
func (s *Service) refundSettled(ctx context.Context, q *db.Queries, orgID, collectionID, entryID int64, amount money.Money, date pgtype.Date) error {
	c, err := q.GetCollectionRowByID(ctx, db.GetCollectionRowByIDParams{OrganizationID: orgID, ID: collectionID})
	if err != nil {
		return err
	}
	if c.SettlementStatus != model.SettlementSettled || !c.CashAccountID.Valid {
		return nil
	}
	locked, err := q.LockCashAccounts(ctx, db.LockCashAccountsParams{OrganizationID: orgID, Ids: []int64{c.CashAccountID.Int64}})
	if err != nil || len(locked) == 0 {
		return err
	}
	branch := locked[0].BranchID
	mv, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
		OrganizationID: orgID, CashAccountID: c.CashAccountID.Int64, Kind: model.MoveRefund, Direction: model.DirOut,
		Amount: amount.Numeric(), Currency: c.Currency, MovementDate: date,
		Description: strings.TrimSpace(c.Code + " · " + c.Provider + " iade"), BranchID: branch,
		LedgerEntryID: int8Of(entryID), CreatedBy: actorID(ctx),
	})
	if err != nil {
		return err
	}
	s.publish(ctx, model.ResourceMovements, "created", mv.Uuid)
	return nil
}
