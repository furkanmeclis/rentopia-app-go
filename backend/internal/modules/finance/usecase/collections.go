package usecase

// Collections (TEC-718, docs/design/collections.md): a collection is the
// document behind one ledger collection entry. Its money is applied to open
// items (live accruals: rent, charges) through append-only allocation rows;
// what is not allocated is the party's credit. Every allocation path locks
// the account row first, and the database guard (collection_allocations
// insert trigger) refuses over-allocating an item or a collection, so two
// concurrent writers can never apply the same money twice.

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrCollectionNotFound: unknown collection, another organization's, or
// outside the caller's branch scope (404).
var ErrCollectionNotFound = errors.New("collection not found")

// CollectionSortColumns are the sortable collection columns.
var CollectionSortColumns = map[string]string{
	"collection_date": "collection_date", "amount": "amount", "code": "code", "created_at": "created_at",
}

// collectionVisible applies the branch scope to a collection (head office
// collections need every branch).
func collectionVisible(ctx context.Context, branch pgtype.Int8) bool {
	if branch.Valid {
		return orgctx.CheckBranch(ctx, branch.Int64) == nil
	}
	return orgctx.RequireAllBranches(ctx) == nil
}

// collectionRow loads one collection the caller may see.
func (s *Service) collectionRow(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetCollectionRowRow, error) {
	if q == nil {
		q = s.q
	}
	row, err := q.GetCollectionRow(ctx, db.GetCollectionRowParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) || (err == nil && !collectionVisible(ctx, row.BranchID)) {
		return row, ErrCollectionNotFound
	}
	return row, err
}

func allocationStatus(status, purpose string, allocated, unallocated money.Money) string {
	switch {
	case status == model.CollectionVoid || purpose == model.PurposeDeposit:
		return model.AllocNotAllocatable
	case !allocated.IsPositive() && unallocated.IsPositive():
		return model.AllocUnallocated
	case unallocated.IsPositive():
		return model.AllocPartiallyAllocated
	default:
		return model.AllocAllocated
	}
}

func itemStatus(amount, allocated money.Money) string {
	switch {
	case !allocated.IsPositive():
		return model.ItemOpen
	case allocated.Cmp(amount) < 0:
		return model.ItemPartiallyPaid
	default:
		return model.ItemPaid
	}
}

func collectionOf(r db.ListCollectionsRow) model.Collection {
	c := model.Collection{
		UUID: r.Uuid, Code: r.Code, Account: model.Ref{UUID: r.AccountUuid, Code: r.AccountCode, Name: r.AccountName},
		Method: r.Method, Provider: r.Provider, Purpose: r.Purpose, Status: r.Status, Currency: r.Currency,
		Amount: m(r.Amount), GrossAmount: m(r.GrossAmount), CommissionAmount: m(r.CommissionAmount), NetAmount: m(r.NetAmount),
		RefundedAmount: m(r.RefundedAmount), AllocatedAmount: m(r.Allocated), UnallocatedAmount: m(r.Unallocated).ClampZero(),
		CollectionDate: dateString(r.CollectionDate), ReferenceNo: r.ReferenceNo, PayerName: r.PayerName, Note: r.Note,
		Branch: refOf(r.BranchUuid, r.BranchCode, r.BranchName), CashAccount: refOf(r.CashAccountUuid, r.CashAccountCode, r.CashAccountName),
		CashAccountKind: r.CashAccountKind.String, Rental: refOf(r.RentalUuid, r.RentalReference, pgtype.Text{}),
		Reservation: refOf(r.ReservationUuid, r.ReservationReference, pgtype.Text{}), PaymentUUID: uuidPtr(r.PaymentUuid),
		EntryUUID: r.LedgerEntryUuid, MovementUUID: uuidPtr(r.CashMovementUuid), CommissionExpense: uuidPtr(r.CommissionExpenseUuid),
		SettlementStatus: r.SettlementStatus, SettledAt: timePtr(r.SettledAt), VoidReason: r.VoidReason, VoidedAt: timePtr(r.VoidedAt),
		CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail),
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if c.Status == model.CollectionVoid {
		c.UnallocatedAmount = money.Zero()
	}
	c.AllocationStatus = allocationStatus(c.Status, c.Purpose, c.AllocatedAmount, c.UnallocatedAmount)
	if c.Status != model.CollectionVoid {
		c.Refundable = c.Amount.Sub(c.RefundedAmount).ClampZero()
	}
	c.Voidable = c.Status == model.CollectionRecorded && c.RefundedAmount.IsZero() && c.PaymentUUID == nil
	return c
}

// ListCollections lists collections in the caller's branch scope.
func (s *Service) ListCollections(ctx context.Context, orgID int64, f model.CollectionFilter) ([]model.Collection, int64, error) {
	branches, err := s.branchIDs(ctx, orgID, "branch", f.BranchUUIDs)
	if err != nil {
		return nil, 0, err
	}
	var cashIDs []int64
	for _, id := range f.CashAccountUUIDs {
		row, err := s.q.GetCashAccountRow(ctx, db.GetCashAccountRowParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil, 0, validation("cash_account", "not_found", "unknown cash account")
		}
		if err != nil {
			return nil, 0, err
		}
		cashIDs = append(cashIDs, row.ID)
	}
	p := db.ListCollectionsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Ids: nilIDs(f.IDs), Methods: nilIfEmpty(f.Methods),
		Statuses: nilIfEmpty(f.Statuses), FilterBranchIds: nilIDs(branches), CashAccountIds: nilIDs(cashIDs),
		AllocationStatuses: nilIfEmpty(f.AllocationStatuses), DateFrom: dateNarg(f.From), DateTo: dateNarg(f.To), Q: textOf(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if _, ok := CollectionSortColumns[p.SortField]; !ok {
		p.SortField, p.SortDesc = "collection_date", true
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	if f.AccountUUID != nil {
		acc, err := s.q.GetAccountRow(ctx, db.GetAccountRowParams{OrganizationID: orgID, Uuid: *f.AccountUUID})
		if notFound(err) {
			return nil, 0, validation("account", "not_found", "unknown account")
		}
		if err != nil {
			return nil, 0, err
		}
		p.AccountID = int8Of(acc.ID)
	}
	if f.RentalUUID != nil {
		r, err := s.q.GetRentalRef(ctx, db.GetRentalRefParams{OrganizationID: orgID, Uuid: *f.RentalUUID})
		if notFound(err) {
			return nil, 0, validation("rental", "not_found", "unknown rental")
		}
		if err != nil {
			return nil, 0, err
		}
		p.RentalID = int8Of(r.ID)
	}
	rows, err := s.q.ListCollections(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Collection, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, collectionOf(r))
	}
	return out, total, nil
}

// GetCollection returns one collection with its allocations and refunds.
func (s *Service) GetCollection(ctx context.Context, orgID int64, id uuid.UUID) (model.Collection, error) {
	row, err := s.collectionRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Collection{}, err
	}
	items, _, err := s.ListCollections(ctx, orgID, model.CollectionFilter{IDs: []int64{row.ID}, Limit: 1})
	if err != nil {
		return model.Collection{}, err
	}
	if len(items) == 0 {
		return model.Collection{}, ErrCollectionNotFound
	}
	c := items[0]
	allocs, err := s.q.ListCollectionAllocations(ctx, db.ListCollectionAllocationsParams{OrganizationID: orgID, CollectionID: row.ID})
	if err != nil {
		return model.Collection{}, err
	}
	c.Allocations = make([]model.Allocation, 0, len(allocs))
	for _, a := range allocs {
		if !m(a.Allocated).IsPositive() {
			continue
		}
		amount, all := m(a.EntryAmount), m(a.EntryAllocated)
		st := itemStatus(amount, all)
		c.Allocations = append(c.Allocations, model.Allocation{
			EntryUUID: a.EntryUuid, Purpose: a.Purpose, Description: a.Description, EntryDate: dateString(a.EntryDate),
			DueDate: dateString(a.DueDate), ItemAmount: amount, Amount: m(a.Allocated), ItemOpen: amount.Sub(all).ClampZero(),
			ItemStatus: st, Reversed: a.Reversed, Rental: refOf(a.RentalUuid, a.RentalReference, pgtype.Text{}),
			ChargeKind: a.ChargeKind.String, AllocatedAt: a.FirstAt.Time.UTC(),
		})
	}
	refunds, err := s.q.ListCollectionRefunds(ctx, db.ListCollectionRefundsParams{OrganizationID: orgID, CollectionID: row.ID})
	if err != nil {
		return model.Collection{}, err
	}
	c.Refunds = make([]model.CollectionRefund, 0, len(refunds))
	for _, f := range refunds {
		c.Refunds = append(c.Refunds, model.CollectionRefund{
			UUID: f.Uuid, Amount: m(f.Amount), Currency: f.Currency, Status: f.Status, Reason: f.Reason, FailureReason: f.FailureReason,
			ProviderRefundID: textPtr(f.ProviderRefundID), EntryUUID: uuidPtr(f.EntryUuid), MovementUUID: uuidPtr(f.MovementUuid),
			CashAccount: refOf(f.CashAccountUuid, f.CashAccountCode, f.CashAccountName),
			CreatedBy:   actorOf(f.CreatedByUuid, f.CreatedByName, f.CreatedBySurname, f.CreatedByEmail), CreatedAt: f.CreatedAt.Time.UTC(),
		})
	}
	return c, nil
}

// OpenItems lists the open items of an account (live accruals with their
// allocation state) and the account's unallocated credit. rental narrows
// the items to one rental.
func (s *Service) OpenItems(ctx context.Context, orgID int64, accountUUID uuid.UUID, rental *uuid.UUID, openOnly bool) (model.OpenItems, error) {
	acc, _, err := s.account(ctx, orgID, accountUUID)
	if err != nil {
		return model.OpenItems{}, err
	}
	p := db.ListOpenItemsParams{OrganizationID: orgID, AccountID: acc.ID, BranchIds: orgctx.BranchScope(ctx), OpenOnly: openOnly}
	if rental != nil {
		r, err := s.rentalRef(ctx, orgID, *rental)
		if err != nil {
			return model.OpenItems{}, validation("rental", "not_found", "unknown rental")
		}
		p.RentalID = int8Of(r.ID)
	}
	rows, err := s.q.ListOpenItems(ctx, p)
	if err != nil {
		return model.OpenItems{}, err
	}
	out := model.OpenItems{
		Account: model.Ref{UUID: acc.Uuid, Code: acc.Code, Name: acc.Name}, Items: make([]model.OpenItem, 0, len(rows)),
		Open: money.Zero(), Credit: money.Zero(), Currency: acc.Currency,
	}
	today := s.today()
	for _, r := range rows {
		amount, allocated := m(r.Amount), m(r.Allocated)
		open := amount.Sub(allocated).ClampZero()
		out.Open = out.Open.Add(open)
		out.Items = append(out.Items, model.OpenItem{
			EntryUUID: r.Uuid, Purpose: r.Purpose, Description: r.Description, EntryDate: dateString(r.EntryDate),
			DueDate: dateString(r.DueDate), Amount: amount, Allocated: allocated, Open: open, Status: itemStatus(amount, allocated),
			Overdue: open.IsPositive() && r.DueDate.Valid && r.DueDate.Time.Before(today), Currency: r.Currency,
			Rental: refOf(r.RentalUuid, r.RentalReference, pgtype.Text{}), ChargeKind: r.ChargeKind.String,
		})
	}
	credits, err := s.q.ListCreditCollections(ctx, db.ListCreditCollectionsParams{
		OrganizationID: orgID, AccountID: acc.ID, BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return model.OpenItems{}, err
	}
	for _, c := range credits {
		out.Credit = out.Credit.Add(m(c.Unallocated))
	}
	return out, nil
}

// ------------------------------------------------------------- recording

// collectionTarget is the account (and rental) a new collection posts to.
type collectionTarget struct {
	acc       db.GetAccountRowRow
	hasAcc    bool
	rental    db.GetRentalRefRow
	hasRental bool
}

func (s *Service) resolveTarget(ctx context.Context, orgID int64, accountUUID, rentalUUID *uuid.UUID) (collectionTarget, error) {
	t := collectionTarget{}
	if accountUUID != nil {
		acc, _, err := s.account(ctx, orgID, *accountUUID)
		if errors.Is(err, ErrAccountNotFound) {
			return t, validation("account_uuid", "not_found", "unknown account")
		}
		if err != nil {
			return t, err
		}
		t.acc, t.hasAcc = acc, true
		r, ok, err := s.postingRental(ctx, orgID, acc, rentalUUID)
		if err != nil {
			return t, err
		}
		t.rental, t.hasRental = r, ok
		return t, nil
	}
	if rentalUUID == nil {
		return t, validation("account_uuid", "required", "choose an account or a rental")
	}
	r, err := s.rentalRef(ctx, orgID, *rentalUUID)
	if err != nil {
		return t, validation("rental_uuid", "not_found", "unknown rental")
	}
	t.rental, t.hasRental = r, true
	party := r.CustomerID
	if r.InvoiceCustomerID.Valid {
		party = r.InvoiceCustomerID.Int64
	}
	if acc, err := s.q.GetAccountByCustomer(ctx, db.GetAccountByCustomerParams{OrganizationID: orgID, CustomerID: int8Of(party)}); err == nil {
		row, err := s.q.GetAccountRow(ctx, db.GetAccountRowParams{OrganizationID: orgID, Uuid: acc.Uuid})
		if err != nil {
			return t, err
		}
		t.acc, t.hasAcc = row, true
	} else if !notFound(err) {
		return t, err
	}
	return t, nil
}

// createOptions are the knobs of the two collection entry points.
type createOptions struct {
	// strict: the panel's collection dialog (method required, a till / bank
	// account required for cash, transfer and POS).
	strict      bool
	description string
}

// CreateCollection records money taken at the branch (cash, bank transfer /
// EFT, physical POS): the ledger collection entry, the cash movement into
// the till / bank account, the collection document and its allocation to
// open items (oldest first by default) in one transaction. A repeated
// request with the same idempotency key returns the first collection
// (created = false).
func (s *Service) CreateCollection(ctx context.Context, orgID int64, in model.CollectionCreateInput) (model.Collection, bool, error) {
	id, created, err := s.createCollection(ctx, orgID, in, createOptions{strict: true})
	if err != nil {
		return model.Collection{}, false, err
	}
	c, err := s.GetCollection(ctx, orgID, id)
	return c, created, err
}

func (s *Service) createCollection(ctx context.Context, orgID int64, in model.CollectionCreateInput, opt createOptions) (uuid.UUID, bool, error) {
	if !has(ctx, rbac.PermTenantPaymentCollect) {
		return uuid.Nil, false, ErrForbidden
	}
	if in.IdempotencyKey != nil {
		if prev, err := s.q.GetCollectionByIdempotencyKey(ctx, db.GetCollectionByIdempotencyKeyParams{
			OrganizationID: orgID, IdempotencyKey: uuidOf(*in.IdempotencyKey),
		}); err == nil {
			return prev.Uuid, false, nil
		} else if !notFound(err) {
			return uuid.Nil, false, err
		}
	}
	ve := &ValidationError{}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	method := strings.TrimSpace(in.Method)
	if method == "" && !opt.strict {
		method = model.MethodOther
	}
	if !contains(model.BranchMethods, method) {
		ve.add("method", "invalid", "method must be cash, bank_transfer, card_pos or other")
	}
	purpose := in.Purpose
	if purpose == "" {
		purpose = model.PurposeGeneral
	}
	if !contains([]string{model.PurposeGeneral, model.PurposePrepayment, model.PurposeDeposit}, purpose) {
		ve.add("purpose", "invalid", "purpose must be general, prepayment or deposit")
	}
	ref, payer, note := strings.TrimSpace(in.ReferenceNo), strings.TrimSpace(in.PayerName), strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(ref) > 64 {
		ve.add("reference_no", "too_long", "reference_no is at most 64 characters")
	}
	if utf8.RuneCountInString(payer) > 255 {
		ve.add("payer_name", "too_long", "payer_name is at most 255 characters")
	}
	if utf8.RuneCountInString(note) > 1000 {
		ve.add("note", "too_long", "note is at most 1000 characters")
	}
	mode := in.AllocationMode
	if mode == "" {
		mode = model.AllocateAuto
		if !opt.strict || purpose == model.PurposeDeposit {
			mode = model.AllocateNone
		}
	}
	if !contains([]string{model.AllocateAuto, model.AllocateManual, model.AllocateNone}, mode) {
		ve.add("allocation_mode", "invalid", "allocation_mode must be auto, manual or none")
	}
	if purpose == model.PurposeDeposit && mode != model.AllocateNone {
		ve.add("allocation_mode", "deposit", "a deposit is held, not allocated to open items")
	}
	if mode == model.AllocateManual && len(in.Allocations) == 0 {
		ve.add("allocations", "required", "choose the open items to allocate")
	}
	if opt.strict && in.CashAccountUUID == nil && method != model.MethodOther {
		ve.add("cash_account_uuid", "required", "choose the till or bank account the money landed in")
	}
	date := optionalDate(ve, "collection_date", in.CollectionDate, s.today())
	if err := ve.orNil(); err != nil {
		return uuid.Nil, false, err
	}
	t, err := s.resolveTarget(ctx, orgID, in.AccountUUID, in.RentalUUID)
	if err != nil {
		return uuid.Nil, false, err
	}
	def := t.acc.BranchID.Int64
	if t.hasRental {
		def = t.rental.PickupBranchID
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", in.BranchUUID, def)
	if err != nil {
		return uuid.Nil, false, err
	}
	collUUID := uuid.New()
	var accUUID uuid.UUID
	var movementUUID, cashUUID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		acc := account{}
		if t.hasAcc {
			acc = account{ID: t.acc.ID, UUID: t.acc.Uuid, Currency: t.acc.Currency, Term: t.acc.PaymentTermDays}
		} else {
			party := t.rental.CustomerID
			if t.rental.InvoiceCustomerID.Valid {
				party = t.rental.InvoiceCustomerID.Int64
			}
			if acc, err = s.ensureCustomerAccount(ctx, q, orgID, party, t.rental.PickupBranchID, ""); err != nil {
				return err
			}
		}
		accUUID = acc.UUID
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
			return err
		}
		if err := q.LockOrganizationCollectionCodes(ctx, orgID); err != nil {
			return err
		}
		code, err := q.NextCollectionCode(ctx, orgID)
		if err != nil {
			return err
		}
		desc := opt.description
		if desc == "" {
			desc = joinParts(t.rental.Reference, code, ref)
		}
		var cash pgtype.Int8
		var movement pgtype.Int8
		var locked lockedAccount
		if in.CashAccountUUID != nil {
			l, err := s.lockForMovement(ctx, q, orgID, cashTarget{"cash_account_uuid", *in.CashAccountUUID})
			if err != nil {
				return err
			}
			locked = l[0]
			if locked.row.Currency != acc.Currency {
				return validation("cash_account_uuid", "currency_mismatch", "the cash account holds another currency")
			}
			if err := checkMethodAccount(method, locked.row.Kind); err != nil {
				return err
			}
			cash = int8Of(locked.row.ID)
			cashUUID = locked.row.Uuid
		}
		entry, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindCollection, Direction: model.Credit, Purpose: purpose,
			Amount: in.Amount.Numeric(), Currency: acc.Currency, EntryDate: dateOf(date), DueDate: dateOf(date),
			Description: desc, SourceType: model.SourceCollection, SourceUuid: uuidOf(collUUID),
			RentalID: rentalOf(t.rental, t.hasRental), ReservationID: reservationOf(t.rental, t.hasRental),
			BranchID: int8Of(branch), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		if cash.Valid {
			mv, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
				OrganizationID: orgID, CashAccountID: locked.row.ID, Kind: model.MoveCollection, Direction: model.DirIn,
				Amount: in.Amount.Numeric(), Currency: acc.Currency, MovementDate: dateOf(date), Description: desc,
				BranchID: locked.row.BranchID, LedgerEntryID: int8Of(entry.ID), CreatedBy: actorID(ctx),
			})
			if err != nil {
				return err
			}
			movement, movementUUID = int8Of(mv.ID), mv.Uuid
		}
		var key pgtype.UUID
		if in.IdempotencyKey != nil {
			key = uuidOf(*in.IdempotencyKey)
		}
		coll, err := q.InsertCollection(ctx, db.InsertCollectionParams{
			Uuid: collUUID, OrganizationID: orgID, Code: code, AccountID: acc.ID, Method: method, Provider: model.ProviderManual,
			Purpose: purpose, Currency: acc.Currency, Amount: in.Amount.Numeric(), GrossAmount: in.Amount.Numeric(),
			CommissionAmount: money.Zero().Numeric(), NetAmount: in.Amount.Numeric(), CollectionDate: dateOf(date),
			ReferenceNo: ref, PayerName: payer, Note: note, BranchID: int8Of(branch), CashAccountID: cash,
			RentalID: rentalOf(t.rental, t.hasRental), ReservationID: reservationOf(t.rental, t.hasRental),
			LedgerEntryID: entry.ID, CashMovementID: movement, SettlementStatus: model.SettlementNone, IdempotencyKey: key,
			CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		if mode == model.AllocateNone {
			return nil
		}
		var rentalFilter int64
		if t.hasRental && mode == model.AllocateAuto {
			rentalFilter = t.rental.ID
		}
		_, err = s.allocate(ctx, q, orgID, allocation{
			collectionID: coll.ID, accountID: acc.ID, mode: mode, items: in.Allocations, rentalID: rentalFilter,
			branches: orgctx.BranchScope(ctx), source: allocSource(mode), field: "allocations",
		})
		return err
	})
	if err != nil {
		if in.IdempotencyKey != nil && uniqueViolation(err) {
			// A concurrent retry with the same key won: answer with it.
			if prev, gerr := s.q.GetCollectionByIdempotencyKey(ctx, db.GetCollectionByIdempotencyKeyParams{
				OrganizationID: orgID, IdempotencyKey: uuidOf(*in.IdempotencyKey),
			}); gerr == nil {
				return prev.Uuid, false, nil
			}
		}
		return uuid.Nil, false, err
	}
	s.publish(ctx, model.ResourceCollections, "created", collUUID)
	s.publish(ctx, model.ResourceAccounts, "updated", accUUID)
	s.publish(ctx, model.ResourceEntries, "created")
	if movementUUID != uuid.Nil {
		s.publish(ctx, model.ResourceMovements, "created", movementUUID)
		s.publish(ctx, model.ResourceCashAccounts, "updated", cashUUID)
	}
	return collUUID, true, nil
}

// checkMethodAccount: cash goes into a till, a transfer / POS slip into a
// bank (or provider clearing) account.
func checkMethodAccount(method, kind string) error {
	switch method {
	case model.MethodCash:
		if kind != model.CashKindCash {
			return validation("cash_account_uuid", "kind_mismatch", "cash goes into a till")
		}
	case model.MethodBankTransfer, model.MethodCardPOS:
		if kind == model.CashKindCash {
			return validation("cash_account_uuid", "kind_mismatch", "a transfer or a POS slip goes into a bank account")
		}
	}
	return nil
}

// joinParts joins the non-empty parts of an entry description.
func joinParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

func allocSource(mode string) string {
	if mode == model.AllocateManual {
		return model.AllocSourceManual
	}
	return model.AllocSourceAuto
}

// ------------------------------------------------------------ allocation

// allocation is one allocate request on a locked account.
type allocation struct {
	collectionID int64
	accountID    int64
	mode         string
	items        []model.AllocationItem
	// rentalID narrows auto allocation to one rental's items (0 = all).
	rentalID int64
	// branches is the scope of the open items (nil = every branch).
	branches []int64
	key      *uuid.UUID
	source   string
	field    string
}

// allocate applies the unallocated money of a collection to open items:
// auto = oldest first (entry date, due date), manual = the items named. The
// caller holds the account lock (same transaction). A request key already
// used on the collection allocates nothing more. Returns the amount applied.
func (s *Service) allocate(ctx context.Context, q *db.Queries, orgID int64, a allocation) (money.Money, error) {
	applied := money.Zero()
	if a.key != nil {
		n, err := q.CountAllocationRequest(ctx, db.CountAllocationRequestParams{
			OrganizationID: orgID, CollectionID: a.collectionID, RequestKey: uuidOf(*a.key),
		})
		if err != nil {
			return applied, err
		}
		if n > 0 {
			return applied, nil
		}
	}
	coll, err := q.GetCollectionRowByID(ctx, db.GetCollectionRowByIDParams{OrganizationID: orgID, ID: a.collectionID})
	if err != nil {
		return applied, err
	}
	if coll.Status == model.CollectionVoid || coll.Purpose == model.PurposeDeposit {
		return applied, stateErr("not_allocatable", "a void collection or a deposit cannot be allocated")
	}
	allocated, err := q.CollectionAllocated(ctx, db.CollectionAllocatedParams{OrganizationID: orgID, CollectionID: a.collectionID})
	if err != nil {
		return applied, err
	}
	credit := m(coll.Amount).Sub(m(coll.RefundedAmount)).Sub(m(allocated))
	if !credit.IsPositive() {
		if a.mode == model.AllocateManual {
			return applied, stateErr("no_credit", "the collection is fully allocated")
		}
		return applied, nil
	}
	type pick struct {
		entryID int64
		amount  money.Money
	}
	picks := []pick{}
	switch a.mode {
	case model.AllocateAuto:
		p := db.ListOpenItemsParams{OrganizationID: orgID, AccountID: a.accountID, BranchIds: a.branches, OpenOnly: true}
		if a.rentalID != 0 {
			p.RentalID = int8Of(a.rentalID)
		}
		items, err := q.ListOpenItems(ctx, p)
		if err != nil {
			return applied, err
		}
		left := credit
		for _, it := range items {
			if !left.IsPositive() {
				break
			}
			open := m(it.Amount).Sub(m(it.Allocated))
			if !open.IsPositive() {
				continue
			}
			amt := open.Min(left)
			picks = append(picks, pick{it.ID, amt})
			left = left.Sub(amt)
		}
	case model.AllocateManual:
		ve := &ValidationError{}
		seen := map[uuid.UUID]bool{}
		left := credit
		for i, it := range a.items {
			field := a.field + "." + itoa(i)
			if seen[it.EntryUUID] {
				ve.add(field+".entry_uuid", "duplicate", "the open item is listed twice")
				continue
			}
			seen[it.EntryUUID] = true
			e, err := q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: it.EntryUUID})
			if notFound(err) {
				ve.add(field+".entry_uuid", "not_found", "unknown open item")
				continue
			}
			if err != nil {
				return applied, err
			}
			rows, err := q.ListOpenItems(ctx, db.ListOpenItemsParams{
				OrganizationID: orgID, AccountID: a.accountID, BranchIds: a.branches, EntryIds: []int64{e.ID},
			})
			if err != nil {
				return applied, err
			}
			if len(rows) == 0 {
				ve.add(field+".entry_uuid", "not_found", "not an open item of this account")
				continue
			}
			open := m(rows[0].Amount).Sub(m(rows[0].Allocated))
			amt := open
			if it.Amount != nil {
				amt = *it.Amount
			}
			switch {
			case !open.IsPositive():
				ve.add(field+".amount", "already_paid", "the open item is already paid")
				continue
			case !amt.IsPositive():
				ve.add(field+".amount", "invalid", "amount must be positive")
				continue
			case amt.Cmp(open) > 0:
				ve.add(field+".amount", "exceeds_open", "at most "+open.String()+" is open on this item")
				continue
			}
			if it.Amount == nil {
				amt = amt.Min(left)
			}
			if amt.Cmp(left) > 0 {
				ve.add(field+".amount", "exceeds_credit", "the collection has "+left.String()+" left to allocate")
				continue
			}
			left = left.Sub(amt)
			if amt.IsPositive() {
				picks = append(picks, pick{e.ID, amt})
			}
		}
		if err := ve.orNil(); err != nil {
			return applied, err
		}
	default:
		return applied, nil
	}
	var key pgtype.UUID
	if a.key != nil {
		key = uuidOf(*a.key)
	}
	for _, p := range picks {
		if _, err := q.InsertCollectionAllocation(ctx, db.InsertCollectionAllocationParams{
			OrganizationID: orgID, CollectionID: a.collectionID, EntryID: p.entryID, Kind: model.AllocKindAllocate,
			Amount: p.amount.Numeric(), Source: a.source, RequestKey: key, CreatedBy: actorID(ctx),
		}); err != nil {
			return applied, allocationConflict(err)
		}
		applied = applied.Add(p.amount)
	}
	return applied, nil
}

// allocationConflict maps the database guard's refusals (a concurrent
// writer won) to a 409.
func allocationConflict(err error) error {
	if name, ok := pgCode(err, "23514"); ok && strings.HasPrefix(name, "chk_collection_allocations") {
		return stateErr("allocation_conflict", "the open item or the collection changed in between; reload and try again")
	}
	if uniqueViolation(err) {
		return stateErr("allocation_conflict", "this allocation request was already applied")
	}
	return err
}

func itoa(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return digits[i : i+1]
	}
	return itoa(i/10) + digits[i%10:i%10+1]
}

// release takes up to amount of a collection's allocations back, newest
// first (refunds, voids). Returns the amount released.
func (s *Service) release(ctx context.Context, q *db.Queries, orgID, collectionID int64, amount money.Money, source, reason string) (money.Money, error) {
	freed := money.Zero()
	if !amount.IsPositive() {
		return freed, nil
	}
	pairs, err := q.ListCollectionAllocationPairs(ctx, db.ListCollectionAllocationPairsParams{OrganizationID: orgID, CollectionID: collectionID})
	if err != nil {
		return freed, err
	}
	for _, p := range pairs {
		left := amount.Sub(freed)
		if !left.IsPositive() {
			break
		}
		amt := m(p.Allocated).Min(left)
		if !amt.IsPositive() {
			continue
		}
		if _, err := q.InsertCollectionAllocation(ctx, db.InsertCollectionAllocationParams{
			OrganizationID: orgID, CollectionID: collectionID, EntryID: p.EntryID, Kind: model.AllocKindRelease,
			Amount: amt.Numeric(), Source: source, Reason: reason, CreatedBy: actorID(ctx),
		}); err != nil {
			return freed, allocationConflict(err)
		}
		freed = freed.Add(amt)
	}
	return freed, nil
}

// releasedPair is what one collection had on a reversed accrual.
type releasedPair struct {
	collectionID int64
	amount       money.Money
}

// releaseEntry releases every allocation on one open item (its accrual is
// being reversed). Returns what each collection got back.
func (s *Service) releaseEntry(ctx context.Context, q *db.Queries, orgID, entryID int64, reason string) ([]releasedPair, error) {
	pairs, err := q.ListEntryAllocationPairs(ctx, db.ListEntryAllocationPairsParams{OrganizationID: orgID, EntryID: entryID})
	if err != nil {
		return nil, err
	}
	out := make([]releasedPair, 0, len(pairs))
	for _, p := range pairs {
		if _, err := q.InsertCollectionAllocation(ctx, db.InsertCollectionAllocationParams{
			OrganizationID: orgID, CollectionID: p.CollectionID, EntryID: entryID, Kind: model.AllocKindRelease,
			Amount: p.Allocated, Source: model.AllocSourceReversed, Reason: reason, CreatedBy: actorID(ctx),
		}); err != nil {
			return nil, err
		}
		out = append(out, releasedPair{p.CollectionID, m(p.Allocated)})
	}
	return out, nil
}

// carryAllocations moves what was released from a reversed accrual onto
// the accrual that replaces it (same key), up to its amount.
func (s *Service) carryAllocations(ctx context.Context, q *db.Queries, orgID, entryID int64, amount money.Money, pairs []releasedPair) error {
	left := amount
	for _, p := range pairs {
		amt := p.amount.Min(left)
		if !amt.IsPositive() {
			break
		}
		if _, err := q.InsertCollectionAllocation(ctx, db.InsertCollectionAllocationParams{
			OrganizationID: orgID, CollectionID: p.collectionID, EntryID: entryID, Kind: model.AllocKindAllocate,
			Amount: amt.Numeric(), Source: model.AllocSourceAuto, CreatedBy: actorID(ctx),
		}); err != nil {
			return err
		}
		left = left.Sub(amt)
	}
	return nil
}

// reallocateRental applies the credit of the rental's own collections (and
// of collections whose allocation was just released) to the rental's open
// items, oldest first. Runs inside SyncRental (account locked).
func (s *Service) reallocateRental(ctx context.Context, q *db.Queries, orgID, accountID, rentalID, reservationID int64, released []int64) error {
	seen := map[int64]bool{}
	type cand struct {
		id   int64
		date pgtype.Date
	}
	cands := []cand{}
	add := func(rows []db.ListCreditCollectionsRow) {
		for _, r := range rows {
			if !seen[r.ID] {
				seen[r.ID] = true
				cands = append(cands, cand{r.ID, r.CollectionDate})
			}
		}
	}
	if len(released) > 0 {
		rows, err := q.ListCreditCollections(ctx, db.ListCreditCollectionsParams{OrganizationID: orgID, AccountID: accountID, Ids: released})
		if err != nil {
			return err
		}
		add(rows)
	}
	rows, err := q.ListCreditCollections(ctx, db.ListCreditCollectionsParams{
		OrganizationID: orgID, AccountID: accountID, RentalID: int8Of(rentalID), ReservationID: int8Of(reservationID),
	})
	if err != nil {
		return err
	}
	add(rows)
	sort.SliceStable(cands, func(i, j int) bool {
		if !cands[i].date.Time.Equal(cands[j].date.Time) {
			return cands[i].date.Time.Before(cands[j].date.Time)
		}
		return cands[i].id < cands[j].id
	})
	for _, c := range cands {
		if _, err := s.allocate(ctx, q, orgID, allocation{
			collectionID: c.id, accountID: accountID, mode: model.AllocateAuto, rentalID: rentalID, source: model.AllocSourceAuto,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Allocate applies a collection's unallocated money (credit) to open
// items: mode auto (oldest first, optionally one rental's items) or manual.
// A retry with the same idempotency key applies nothing more.
func (s *Service) Allocate(ctx context.Context, orgID int64, id uuid.UUID, in model.AllocateInput) (model.Collection, error) {
	if !has(ctx, rbac.PermTenantFinanceAccountsWrite) {
		return model.Collection{}, ErrForbidden
	}
	row, err := s.collectionRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Collection{}, err
	}
	mode := in.Mode
	if mode == "" {
		mode = model.AllocateAuto
		if len(in.Items) > 0 {
			mode = model.AllocateManual
		}
	}
	if mode != model.AllocateAuto && mode != model.AllocateManual {
		return model.Collection{}, validation("mode", "invalid", "mode must be auto or manual")
	}
	if mode == model.AllocateManual && len(in.Items) == 0 {
		return model.Collection{}, validation("items", "required", "choose the open items to allocate")
	}
	var rentalID int64
	if in.RentalUUID != nil {
		r, err := s.rentalRef(ctx, orgID, *in.RentalUUID)
		if err != nil {
			return model.Collection{}, validation("rental_uuid", "not_found", "unknown rental")
		}
		rentalID = r.ID
	}
	var applied money.Money
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: row.AccountID}); err != nil {
			return err
		}
		applied, err = s.allocate(ctx, q, orgID, allocation{
			collectionID: row.ID, accountID: row.AccountID, mode: mode, items: in.Items, rentalID: rentalID,
			branches: orgctx.BranchScope(ctx), key: in.IdempotencyKey, source: allocSource(mode), field: "items",
		})
		return err
	})
	if err != nil {
		return model.Collection{}, err
	}
	if applied.IsPositive() {
		s.publish(ctx, model.ResourceCollections, "updated", id)
	}
	return s.GetCollection(ctx, orgID, id)
}

// AllocateCredit is the bulk action: the collection's credit goes to the
// oldest open items. Returns the request key (rollback releases it) and
// the amount applied.
func (s *Service) AllocateCredit(ctx context.Context, orgID int64, id uuid.UUID, key uuid.UUID) (money.Money, error) {
	row, err := s.collectionRow(ctx, nil, orgID, id)
	if err != nil {
		return money.Zero(), err
	}
	var applied money.Money
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: row.AccountID}); err != nil {
			return err
		}
		applied, err = s.allocate(ctx, q, orgID, allocation{
			collectionID: row.ID, accountID: row.AccountID, mode: model.AllocateAuto, branches: orgctx.BranchScope(ctx),
			key: &key, source: model.AllocSourceAuto,
		})
		return err
	})
	if err == nil && applied.IsPositive() {
		s.publish(ctx, model.ResourceCollections, "updated", id)
	}
	return applied, err
}

// UndoAllocation releases what one allocate request applied (bulk
// rollback).
func (s *Service) UndoAllocation(ctx context.Context, orgID int64, id uuid.UUID, key uuid.UUID) error {
	row, err := s.collectionRow(ctx, nil, orgID, id)
	if err != nil {
		return err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: row.AccountID}); err != nil {
			return err
		}
		rows, err := q.ListAllocationsByRequest(ctx, db.ListAllocationsByRequestParams{
			OrganizationID: orgID, CollectionID: row.ID, RequestKey: uuidOf(key),
		})
		if err != nil {
			return err
		}
		pairs, err := q.ListCollectionAllocationPairs(ctx, db.ListCollectionAllocationPairsParams{OrganizationID: orgID, CollectionID: row.ID})
		if err != nil {
			return err
		}
		net := map[int64]money.Money{}
		for _, p := range pairs {
			net[p.EntryID] = m(p.Allocated)
		}
		for _, r := range rows {
			amt := m(r.Amount).Min(net[r.EntryID])
			if !amt.IsPositive() {
				continue // already released by a refund / void
			}
			if _, err := q.InsertCollectionAllocation(ctx, db.InsertCollectionAllocationParams{
				OrganizationID: orgID, CollectionID: row.ID, EntryID: r.EntryID, Kind: model.AllocKindRelease, Amount: amt.Numeric(),
				Source: model.AllocSourceReleased, Reason: "bulk rollback", CreatedBy: actorID(ctx),
			}); err != nil {
				return allocationConflict(err)
			}
			net[r.EntryID] = net[r.EntryID].Sub(amt)
		}
		return nil
	})
	if err == nil {
		s.publish(ctx, model.ResourceCollections, "updated", id)
	}
	return err
}

// Release takes a collection's allocation off one open item (a wrong
// manual allocation); the money becomes credit again.
func (s *Service) Release(ctx context.Context, orgID int64, id uuid.UUID, in model.ReleaseInput) (model.Collection, error) {
	if !has(ctx, rbac.PermTenantFinanceAccountsWrite) {
		return model.Collection{}, ErrForbidden
	}
	row, err := s.collectionRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Collection{}, err
	}
	if in.EntryUUID == nil {
		return model.Collection{}, validation("entry_uuid", "required", "choose the open item")
	}
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return model.Collection{}, err
	}
	e, err := s.q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: *in.EntryUUID})
	if notFound(err) {
		return model.Collection{}, validation("entry_uuid", "not_found", "unknown open item")
	}
	if err != nil {
		return model.Collection{}, err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: row.AccountID}); err != nil {
			return err
		}
		pairs, err := q.ListCollectionAllocationPairs(ctx, db.ListCollectionAllocationPairsParams{OrganizationID: orgID, CollectionID: row.ID})
		if err != nil {
			return err
		}
		for _, p := range pairs {
			if p.EntryID != e.ID {
				continue
			}
			_, err := q.InsertCollectionAllocation(ctx, db.InsertCollectionAllocationParams{
				OrganizationID: orgID, CollectionID: row.ID, EntryID: e.ID, Kind: model.AllocKindRelease, Amount: p.Allocated,
				Source: model.AllocSourceReleased, Reason: reason, CreatedBy: actorID(ctx),
			})
			return allocationConflict(err)
		}
		return validation("entry_uuid", "not_allocated", "the collection is not allocated to this item")
	})
	if err != nil {
		return model.Collection{}, err
	}
	s.publish(ctx, model.ResourceCollections, "updated", id)
	return s.GetCollection(ctx, orgID, id)
}
