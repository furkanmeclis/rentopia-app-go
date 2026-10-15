package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// AccountSortColumns are the sortable account columns.
var AccountSortColumns = map[string]string{
	"balance": "balance", "overdue": "overdue", "name": "name", "code": "code",
	"last_entry_at": "last_entry_at", "created_at": "created_at",
}

// MaxListTargets caps exports and bulk "all matching".
const MaxListTargets = 10000

func (s *Service) agingDates(dueBefore *time.Time) (pgtype.Date, pgtype.Date, pgtype.Date) {
	today := s.today()
	before := today
	if dueBefore != nil {
		before = day0(*dueBefore)
	}
	return dateOf(today), dateOf(today.AddDate(0, 0, model.DueSoonDays)), dateOf(before)
}

// ListAccounts lists accounts with their balances (over the entries the
// caller's branch scope covers).
func (s *Service) ListAccounts(ctx context.Context, orgID int64, f model.AccountFilter) ([]model.Account, int64, error) {
	filterBranches, err := s.branchIDs(ctx, orgID, "branch", f.BranchUUIDs)
	if err != nil {
		return nil, 0, err
	}
	today, soon, before := s.agingDates(f.DueBefore)
	sortField := f.SortField
	if _, ok := AccountSortColumns[sortField]; !ok {
		sortField, f.SortDesc = "created_at", true
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	rows, err := s.q.ListAccounts(ctx, db.ListAccountsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Today: today, Soon: soon, DueBefore: before,
		AccountIds: nilIDs(f.AccountIDs), Statuses: nilIfEmpty(f.BalanceStatuses), AccountStatuses: nilIfEmpty(f.Statuses),
		PartyKinds: nilIfEmpty(f.PartyKinds), FilterBranchIds: nilIDs(filterBranches),
		MinBalance: numericOf(f.MinBalance), MaxBalance: numericOf(f.MaxBalance), OnlyDueBefore: f.DueBefore != nil,
		Q: textOf(f.Q), SortField: sortField, SortDesc: f.SortDesc, LimitCount: limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Account, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, accountOf(r))
	}
	return out, total, nil
}

func accountOf(r db.ListAccountsRow) model.Account {
	a := model.Account{
		UUID: r.Uuid, Code: r.Code, PartyKind: r.PartyKind, Name: r.Name, Currency: r.Currency,
		PaymentTermDays: r.PaymentTermDays, Status: r.Status, Note: r.Note, EntryCount: r.EntryCount,
		LastEntryAt: timePtr(r.LastEntryAt), CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		Branch: refOf(r.BranchUuid, r.BranchCode, r.BranchName), CustomerType: r.CustomerType.String,
		Balance: model.Balance{
			Currency: r.Currency, Debit: m(r.Debit), Credit: m(r.Credit), Balance: m(r.Balance), Overdue: m(r.Overdue),
			DueSoon: m(r.DueSoon), DepositHeld: m(r.DepositHeld), Status: r.BalanceStatus,
		},
	}
	if r.CustomerUuid.Valid {
		a.Customer = &model.Ref{UUID: r.CustomerUuid.Bytes, Name: r.Name}
	}
	return a
}

// account loads one account the caller may see (404 otherwise).
func (s *Service) account(ctx context.Context, orgID int64, id uuid.UUID) (db.GetAccountRowRow, model.Account, error) {
	row, err := s.q.GetAccountRow(ctx, db.GetAccountRowParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, model.Account{}, ErrAccountNotFound
	}
	if err != nil {
		return row, model.Account{}, err
	}
	items, _, err := s.ListAccounts(ctx, orgID, model.AccountFilter{AccountIDs: []int64{row.ID}, Limit: 1})
	if err != nil {
		return row, model.Account{}, err
	}
	if len(items) == 0 {
		return row, model.Account{}, ErrAccountNotFound
	}
	return row, items[0], nil
}

// GetAccount returns one account with its balance.
func (s *Service) GetAccount(ctx context.Context, orgID int64, id uuid.UUID) (model.Account, error) {
	_, a, err := s.account(ctx, orgID, id)
	return a, err
}

// CreateAccount opens the account of a customer (idempotent: an existing
// account is returned, its terms updated when given).
func (s *Service) CreateAccount(ctx context.Context, orgID int64, in model.AccountCreateInput) (model.Account, bool, error) {
	if in.CustomerUUID == nil {
		return model.Account{}, false, validation("customer_uuid", "required", "customer_uuid is required")
	}
	c, err := s.q.GetCustomerByUUIDForLedger(ctx, db.GetCustomerByUUIDForLedgerParams{OrganizationID: orgID, Uuid: *in.CustomerUUID})
	if notFound(err) {
		return model.Account{}, false, validation("customer_uuid", "not_found", "unknown customer")
	}
	if err != nil {
		return model.Account{}, false, err
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", nil, firstBranch(ctx))
	if err != nil {
		return model.Account{}, false, err
	}
	created := false
	var id uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		_, existing := q.GetAccountByCustomer(ctx, db.GetAccountByCustomerParams{OrganizationID: orgID, CustomerID: int8Of(c.ID)})
		created = notFound(existing)
		acc, err := s.ensureCustomerAccount(ctx, q, orgID, c.ID, branch, "")
		if err != nil {
			return err
		}
		id = acc.UUID
		if in.PaymentTermDays != nil || in.Note != nil {
			row, err := q.GetAccountRowByID(ctx, db.GetAccountRowByIDParams{OrganizationID: orgID, ID: acc.ID})
			if err != nil {
				return err
			}
			return s.updateAccount(ctx, q, orgID, row.ID, row.PaymentTermDays, row.Note, row.Status,
				model.AccountPatch{PaymentTermDays: in.PaymentTermDays, Note: in.Note})
		}
		return nil
	})
	if err != nil {
		return model.Account{}, false, err
	}
	a, err := s.GetAccount(ctx, orgID, id)
	if err == nil {
		action := "updated"
		if created {
			action = "created"
		}
		s.publish(ctx, model.ResourceAccounts, action, a.UUID)
	}
	return a, created, err
}

// firstBranch is the branch a branch-scoped member opens accounts at.
func firstBranch(ctx context.Context) int64 {
	if ids := orgctx.BranchScope(ctx); len(ids) > 0 {
		return ids[0]
	}
	return 0
}

func (s *Service) updateAccount(ctx context.Context, q *db.Queries, orgID, id int64, term int32, note, status string, in model.AccountPatch) error {
	ve := &ValidationError{}
	if in.PaymentTermDays != nil {
		if *in.PaymentTermDays < 0 || *in.PaymentTermDays > 365 {
			ve.add("payment_term_days", "out_of_range", "payment_term_days must be between 0 and 365")
		}
		term = *in.PaymentTermDays
	}
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
		if len(note) > 2000 {
			ve.add("note", "too_long", "note is at most 2000 characters")
		}
	}
	if in.Status != nil {
		status = *in.Status
		if status != model.AccountActive && status != model.AccountArchived {
			ve.add("status", "invalid", "status must be active or archived")
		}
	}
	if err := ve.orNil(); err != nil {
		return err
	}
	_, err := q.UpdateAccount(ctx, db.UpdateAccountParams{OrganizationID: orgID, ID: id, PaymentTermDays: term, Note: note, Status: status})
	return err
}

// PatchAccount changes the terms of an account.
func (s *Service) PatchAccount(ctx context.Context, orgID int64, id uuid.UUID, in model.AccountPatch) (model.Account, error) {
	row, _, err := s.account(ctx, orgID, id)
	if err != nil {
		return model.Account{}, err
	}
	if err := s.updateAccount(ctx, s.q, orgID, row.ID, row.PaymentTermDays, row.Note, row.Status, in); err != nil {
		return model.Account{}, err
	}
	s.publish(ctx, model.ResourceAccounts, "updated", id)
	return s.GetAccount(ctx, orgID, id)
}

// ------------------------------------------------------------- entries

// ListEntries lists ledger entries (the ledger page, an account statement,
// a rental's entries).
func (s *Service) ListEntries(ctx context.Context, orgID int64, f model.EntryFilter) ([]model.Entry, int64, error) {
	p := db.ListLedgerEntriesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Kinds: nilIfEmpty(f.Kinds), Purposes: nilIfEmpty(f.Purposes),
		Directions: nilIfEmpty(f.Directions), DateFrom: dateNarg(f.From), DateTo: dateNarg(f.To), Q: textOf(f.Q),
		SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	if f.EntryUUID != nil {
		p.EntryUuid = uuidOf(*f.EntryUUID)
	}
	if f.AccountUUID != nil {
		row, err := s.q.GetAccountRow(ctx, db.GetAccountRowParams{OrganizationID: orgID, Uuid: *f.AccountUUID})
		if notFound(err) {
			return nil, 0, validation("account", "not_found", "unknown account")
		}
		if err != nil {
			return nil, 0, err
		}
		p.AccountID = int8Of(row.ID)
	}
	if f.RentalUUID != nil {
		r, err := s.q.GetRentalRef(ctx, db.GetRentalRefParams{OrganizationID: orgID, Uuid: *f.RentalUUID})
		if notFound(err) {
			return nil, 0, validation("rental", "not_found", "unknown rental")
		}
		if err != nil {
			return nil, 0, err
		}
		p.ReservationID = int8Of(r.ReservationID)
	}
	branches, err := s.branchIDs(ctx, orgID, "branch", f.BranchUUIDs)
	if err != nil {
		return nil, 0, err
	}
	p.FilterBranchIds = nilIDs(branches)
	rows, err := s.q.ListLedgerEntries(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Entry, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, entryOf(r))
	}
	return out, total, nil
}

func entryOf(r db.ListLedgerEntriesRow) model.Entry {
	e := model.Entry{
		UUID: r.Uuid, Account: model.Ref{UUID: r.AccountUuid, Code: r.AccountCode, Name: r.AccountName},
		Kind: r.Kind, Direction: r.Direction, Purpose: r.Purpose, Amount: m(r.Amount), Currency: r.Currency,
		EntryDate: dateString(r.EntryDate), DueDate: dateString(r.DueDate), Description: r.Description, Reason: r.Reason,
		SourceType: r.SourceType, SourceUUID: uuidPtr(r.SourceUuid), OffsetGroup: uuidPtr(r.OffsetGroup),
		Rental: refOf(r.RentalUuid, r.RentalReference, pgtype.Text{}), Reservation: refOf(r.ReservationUuid, r.ReservationReference, pgtype.Text{}),
		Charge: refOf(r.ChargeUuid, r.ChargeKind, pgtype.Text{}), PaymentUUID: uuidPtr(r.PaymentUuid),
		ReversesUUID: uuidPtr(r.ReversesUuid), ReversedByUUID: uuidPtr(r.ReversedByUuid), AdjustsUUID: uuidPtr(r.AdjustsUuid),
		Branch: refOf(r.BranchUuid, r.BranchCode, r.BranchName), RunningBalance: m(r.RunningBalance),
		CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail),
		CreatedAt: r.CreatedAt.Time.UTC(), Links: []model.Link{},
	}
	if r.AccrualGeneration.Valid {
		g := r.AccrualGeneration.Int32
		e.AccrualGeneration = &g
	}
	if r.Links != "" {
		for _, part := range strings.Split(r.Links, ",") {
			t, ref, _ := strings.Cut(part, ":")
			e.Links = append(e.Links, model.Link{DocumentType: t, DocumentRef: ref})
		}
	}
	return e
}

// GetEntry returns one entry the caller may see.
func (s *Service) GetEntry(ctx context.Context, orgID int64, id uuid.UUID) (model.Entry, error) {
	items, _, err := s.ListEntries(ctx, orgID, model.EntryFilter{EntryUUID: &id, Limit: 1})
	if err != nil {
		return model.Entry{}, err
	}
	if len(items) == 0 {
		return model.Entry{}, ErrEntryNotFound
	}
	return items[0], nil
}

// Statement is an account with a page of its entries (oldest first, with
// the running balance) and the opening balance before From.
func (s *Service) Statement(ctx context.Context, orgID int64, id uuid.UUID, f model.EntryFilter) (model.Statement, error) {
	row, acc, err := s.account(ctx, orgID, id)
	if err != nil {
		return model.Statement{}, err
	}
	f.AccountUUID = &id
	items, total, err := s.ListEntries(ctx, orgID, f)
	if err != nil {
		return model.Statement{}, err
	}
	sum, err := s.q.SumLedgerEntries(ctx, db.SumLedgerEntriesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), AccountID: int8Of(row.ID), Before: dateNarg(f.From),
	})
	if err != nil {
		return model.Statement{}, err
	}
	return model.Statement{Account: acc, Opening: m(sum.Opening), Entries: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}

// rentalRef loads a rental the caller may see (rentals' branch rule:
// planned pick-up, planned return or actual return branch in scope).
func (s *Service) rentalRef(ctx context.Context, orgID int64, id uuid.UUID) (db.GetRentalRefRow, error) {
	r, err := s.q.GetRentalRef(ctx, db.GetRentalRefParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return r, ErrRentalNotFound
	}
	if err != nil {
		return r, err
	}
	ok := orgctx.CheckBranch(ctx, r.PickupBranchID) == nil || orgctx.CheckBranch(ctx, r.ReturnBranchID) == nil ||
		(r.ActualReturnBranchID.Valid && orgctx.CheckBranch(ctx, r.ActualReturnBranchID.Int64) == nil)
	if !ok {
		return r, ErrRentalNotFound
	}
	return r, nil
}

// RentalLedger is the money state of a rental: every entry of its
// reservation (prepayments before the handover included).
func (s *Service) RentalLedger(ctx context.Context, orgID int64, id uuid.UUID) (model.RentalLedger, error) {
	r, err := s.rentalRef(ctx, orgID, id)
	if err != nil {
		return model.RentalLedger{}, err
	}
	sum, err := s.q.SumLedgerEntries(ctx, db.SumLedgerEntriesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), ReservationID: int8Of(r.ReservationID),
	})
	if err != nil {
		return model.RentalLedger{}, err
	}
	items, _, err := s.ListEntries(ctx, orgID, model.EntryFilter{RentalUUID: &id, Limit: 200})
	if err != nil {
		return model.RentalLedger{}, err
	}
	out := model.RentalLedger{
		RentalUUID: r.Uuid, Reference: r.Reference, Currency: string(money.TRY), Debit: m(sum.Debit), Credit: m(sum.Credit),
		Balance: m(sum.Debit).Sub(m(sum.Credit)), DepositHeld: m(sum.DepositHeld), Entries: items,
	}
	customerID := r.CustomerID
	if r.InvoiceCustomerID.Valid {
		customerID = r.InvoiceCustomerID.Int64
	}
	if a, err := s.q.GetAccountByCustomer(ctx, db.GetAccountByCustomerParams{OrganizationID: orgID, CustomerID: int8Of(customerID)}); err == nil {
		row, err := s.q.GetAccountRowByID(ctx, db.GetAccountRowByIDParams{OrganizationID: orgID, ID: a.ID})
		if err == nil {
			out.Account = &model.Ref{UUID: row.Uuid, Code: row.Code, Name: row.Name}
			out.Currency = row.Currency
		}
	}
	return out, nil
}

// CustomerLedger is the account of a customer and its latest entries (no
// account yet → nil, nothing posted).
func (s *Service) CustomerLedger(ctx context.Context, orgID int64, id uuid.UUID) (model.CustomerLedger, error) {
	c, err := s.q.GetCustomerByUUIDForLedger(ctx, db.GetCustomerByUUIDForLedgerParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.CustomerLedger{}, ErrCustomerNotFound
	}
	if err != nil {
		return model.CustomerLedger{}, err
	}
	out := model.CustomerLedger{CustomerUUID: c.Uuid, Entries: []model.Entry{}}
	a, err := s.q.GetAccountByCustomer(ctx, db.GetAccountByCustomerParams{OrganizationID: orgID, CustomerID: int8Of(c.ID)})
	if notFound(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	items, _, err := s.ListAccounts(ctx, orgID, model.AccountFilter{AccountIDs: []int64{a.ID}, Limit: 1})
	if err != nil || len(items) == 0 {
		return out, err
	}
	out.Account = &items[0]
	entries, _, err := s.ListEntries(ctx, orgID, model.EntryFilter{AccountUUID: &a.Uuid, SortDesc: true, Limit: 50})
	if err != nil {
		return out, err
	}
	out.Entries = entries
	return out, nil
}

// ListRentalBalances is the per-rental breakdown.
func (s *Service) ListRentalBalances(ctx context.Context, orgID int64, q string, openOnly bool, limit, offset int32) ([]model.RentalBalance, int64, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := s.q.ListRentalBalances(ctx, db.ListRentalBalancesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), OpenOnly: openOnly, Q: textOf(q), LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.RentalBalance, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		item := model.RentalBalance{
			RentalUUID: uuidPtr(r.RentalUuid), Reference: r.Reference.String, RentalStatus: r.RentalStatus.String,
			ReservationUUID: r.ReservationUuid, Account: refOf(r.AccountUuid, r.AccountCode, r.AccountName),
			Debit: m(r.Debit), Credit: m(r.Credit), Balance: m(r.Balance), DepositHeld: m(r.DepositHeld), LastEntryAt: timePtr(r.LastEntryAt),
		}
		out = append(out, item)
	}
	return out, total, nil
}

// Summary is the organization total (in the caller's branch scope) with
// the per-branch and due-date breakdowns.
func (s *Service) Summary(ctx context.Context, orgID int64) (model.Summary, error) {
	today := s.today()
	scope := orgctx.BranchScope(ctx)
	rows, err := s.q.LedgerAccountAging(ctx, db.LedgerAccountAgingParams{
		OrganizationID: orgID, BranchIds: scope, Late: dateOf(today.AddDate(0, 0, -30)), Today: dateOf(today),
		Soon: dateOf(today.AddDate(0, 0, model.DueSoonDays)),
	})
	if err != nil {
		return model.Summary{}, err
	}
	out := model.Summary{
		Currency: string(money.TRY), Receivable: money.Zero(), Payable: money.Zero(), Overdue: money.Zero(), DueSoon: money.Zero(),
		DepositHeld: money.Zero(), AccountsByStat: map[string]int64{}, ByBranch: []model.BranchBalance{},
	}
	late, overdue, soon, later := money.Zero(), money.Zero(), money.Zero(), money.Zero()
	for _, r := range rows {
		bal, settled := m(r.Balance), m(r.Settlements)
		open := func(c pgtype.Numeric) money.Money { return m(c).Sub(settled).ClampZero() }
		oLate, oOver, oSoon, oAll := open(r.ChargesLate), open(r.ChargesOverdue), open(r.ChargesSoon), open(r.ChargesAll)
		late = late.Add(oLate)
		overdue = overdue.Add(oOver.Sub(oLate))
		soon = soon.Add(oSoon.Sub(oOver))
		later = later.Add(oAll.Sub(oSoon))
		out.Overdue = out.Overdue.Add(oOver)
		out.DueSoon = out.DueSoon.Add(oSoon.Sub(oOver))
		out.DepositHeld = out.DepositHeld.Add(m(r.DepositHeld))
		status := model.BalanceClosed
		switch {
		case bal.IsZero():
		case oOver.IsPositive():
			status = model.BalanceOverdue
		case oSoon.Sub(oOver).IsPositive():
			status = model.BalanceDueSoon
		case bal.IsPositive():
			status = model.BalanceReceivable
		default:
			status = model.BalancePayable
		}
		out.AccountsByStat[status]++
		if bal.IsPositive() {
			out.Receivable = out.Receivable.Add(bal)
		} else {
			out.Payable = out.Payable.Add(bal.Neg())
		}
	}
	out.Net = out.Receivable.Sub(out.Payable)
	out.ByDue = []model.DueBucket{
		{Key: "overdue_30_plus", Amount: late}, {Key: "overdue", Amount: overdue},
		{Key: "due_7_days", Amount: soon}, {Key: "later", Amount: later},
	}
	branches, err := s.q.LedgerBranchBreakdown(ctx, db.LedgerBranchBreakdownParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, b := range branches {
		out.ByBranch = append(out.ByBranch, model.BranchBalance{
			Branch: refOf(b.BranchUuid, b.BranchCode, b.BranchName), Debit: m(b.Debit), Credit: m(b.Credit),
			Balance: m(b.Debit).Sub(m(b.Credit)),
		})
	}
	return out, nil
}

// ------------------------------------------------------------ postings

// postingRental resolves an optional rental of a manual posting: it must
// be the account party's rental (or one the caller may see).
func (s *Service) postingRental(ctx context.Context, orgID int64, acc db.GetAccountRowRow, id *uuid.UUID) (db.GetRentalRefRow, bool, error) {
	if id == nil {
		return db.GetRentalRefRow{}, false, nil
	}
	r, err := s.rentalRef(ctx, orgID, *id)
	if err != nil {
		return r, false, validation("rental_uuid", "not_found", "unknown rental")
	}
	party := r.CustomerID
	if r.InvoiceCustomerID.Valid {
		party = r.InvoiceCustomerID.Int64
	}
	if !acc.CustomerID.Valid || (acc.CustomerID.Int64 != party && acc.CustomerID.Int64 != r.CustomerID) {
		return r, false, validation("rental_uuid", "other_party", "the rental belongs to another account")
	}
	return r, true, nil
}

func reservationOf(r db.GetRentalRefRow, ok bool) pgtype.Int8 {
	if !ok {
		return pgtype.Int8{}
	}
	return int8Of(r.ReservationID)
}

func rentalOf(r db.GetRentalRefRow, ok bool) pgtype.Int8 {
	if !ok {
		return pgtype.Int8{}
	}
	return int8Of(r.ID)
}

// Collect records money received on an account outside the payments
// module (corporate bank transfer, a deposit at the counter). With a cash
// account the money enters it in the same transaction.
func (s *Service) Collect(ctx context.Context, orgID int64, id uuid.UUID, in model.CollectionInput) (model.Entry, error) {
	if !has(ctx, rbac.PermTenantPaymentCollect) {
		return model.Entry{}, ErrForbidden
	}
	acc, _, err := s.account(ctx, orgID, id)
	if err != nil {
		return model.Entry{}, err
	}
	ve := &ValidationError{}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	purpose := in.Purpose
	if purpose == "" {
		purpose = model.PurposeGeneral
	}
	if !contains([]string{model.PurposeGeneral, model.PurposePrepayment, model.PurposeDeposit}, purpose) {
		ve.add("purpose", "invalid", "purpose must be general, prepayment or deposit")
	}
	method := strings.TrimSpace(in.Method)
	if method != "" && !contains([]string{"cash", "card_pos", "bank_transfer", "other"}, method) {
		ve.add("method", "invalid", "unknown method")
	}
	if len(in.Description) > 500 {
		ve.add("description", "too_long", "description is at most 500 characters")
	}
	date := optionalDate(ve, "entry_date", in.EntryDate, s.today())
	if err := ve.orNil(); err != nil {
		return model.Entry{}, err
	}
	rental, hasRental, err := s.postingRental(ctx, orgID, acc, in.RentalUUID)
	if err != nil {
		return model.Entry{}, err
	}
	def := acc.BranchID.Int64
	if hasRental {
		def = rental.PickupBranchID
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", in.BranchUUID, def)
	if err != nil {
		return model.Entry{}, err
	}
	desc := strings.TrimSpace(in.Description)
	if desc == "" {
		desc = strings.Trim(rental.Reference+" · "+method, " ·")
	}
	var entryID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		row, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindCollection, Direction: model.Credit, Purpose: purpose,
			Amount: in.Amount.Numeric(), Currency: acc.Currency, EntryDate: dateOf(date), DueDate: dateOf(date),
			Description: desc, SourceType: model.SourceManual, RentalID: rentalOf(rental, hasRental),
			ReservationID: reservationOf(rental, hasRental), BranchID: int8Of(branch), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		entryID = row.Uuid
		if in.CashAccountUUID != nil {
			return s.settleEntry(ctx, q, orgID, *in.CashAccountUUID, row.ID, model.MoveCollection, *in.Amount, acc.Currency, date, desc)
		}
		return nil
	})
	if err != nil {
		return model.Entry{}, err
	}
	s.publish(ctx, model.ResourceAccounts, "updated", id)
	s.publish(ctx, model.ResourceEntries, "created", entryID)
	return s.GetEntry(ctx, orgID, entryID)
}

// Refund records money paid back to the party (a deposit returned, an
// overpayment). With a cash account the money leaves it.
func (s *Service) Refund(ctx context.Context, orgID int64, id uuid.UUID, in model.RefundInput) (model.Entry, error) {
	if !has(ctx, rbac.PermTenantPaymentRefund) {
		return model.Entry{}, ErrForbidden
	}
	acc, _, err := s.account(ctx, orgID, id)
	if err != nil {
		return model.Entry{}, err
	}
	ve := &ValidationError{}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	purpose := in.Purpose
	if purpose == "" {
		purpose = model.PurposeGeneral
	}
	if !contains([]string{model.PurposeGeneral, model.PurposeDeposit, model.PurposePrepayment}, purpose) {
		ve.add("purpose", "invalid", "purpose must be general, prepayment or deposit")
	}
	reason, rerr := requireReason("reason", in.Reason)
	if rerr != nil {
		ve.add("reason", "required", "a reason is required (3-500 characters)")
	}
	date := optionalDate(ve, "entry_date", in.EntryDate, s.today())
	if err := ve.orNil(); err != nil {
		return model.Entry{}, err
	}
	rental, hasRental, err := s.postingRental(ctx, orgID, acc, in.RentalUUID)
	if err != nil {
		return model.Entry{}, err
	}
	def := acc.BranchID.Int64
	if hasRental {
		def = rental.PickupBranchID
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", in.BranchUUID, def)
	if err != nil {
		return model.Entry{}, err
	}
	var entryID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		row, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindRefund, Direction: model.Debit, Purpose: purpose,
			Amount: in.Amount.Numeric(), Currency: acc.Currency, EntryDate: dateOf(date), DueDate: dateOf(date),
			Description: rental.Reference, Reason: reason, SourceType: model.SourceManual, RentalID: rentalOf(rental, hasRental),
			ReservationID: reservationOf(rental, hasRental), BranchID: int8Of(branch), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		entryID = row.Uuid
		if in.CashAccountUUID != nil {
			return s.settleEntry(ctx, q, orgID, *in.CashAccountUUID, row.ID, model.MoveRefund, *in.Amount, acc.Currency, date, reason)
		}
		return nil
	})
	if err != nil {
		return model.Entry{}, err
	}
	s.publish(ctx, model.ResourceAccounts, "updated", id)
	s.publish(ctx, model.ResourceEntries, "created", entryID)
	return s.GetEntry(ctx, orgID, entryID)
}

// Adjust posts a correction with a reason (tenant.finance.ledger.adjust).
func (s *Service) Adjust(ctx context.Context, orgID int64, id uuid.UUID, in model.AdjustmentInput) (model.Entry, error) {
	if !has(ctx, rbac.PermTenantFinanceLedgerAdjust) {
		return model.Entry{}, ErrForbidden
	}
	acc, _, err := s.account(ctx, orgID, id)
	if err != nil {
		return model.Entry{}, err
	}
	ve := &ValidationError{}
	if in.Direction != model.Debit && in.Direction != model.Credit {
		ve.add("direction", "invalid", "direction must be debit or credit")
	}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	purpose := in.Purpose
	if purpose == "" {
		purpose = model.PurposeGeneral
	}
	if !contains(model.Purposes, purpose) {
		ve.add("purpose", "invalid", "unknown purpose")
	}
	reason, rerr := requireReason("reason", in.Reason)
	if rerr != nil {
		ve.add("reason", "required", "a reason is required (3-500 characters)")
	}
	date := optionalDate(ve, "entry_date", in.EntryDate, s.today())
	due := optionalDate(ve, "due_date", in.DueDate, dueAfter(date, acc.PaymentTermDays))
	if due.Before(date.AddDate(-10, 0, 0)) {
		ve.add("due_date", "out_of_range", "due_date is too far in the past")
	}
	if err := ve.orNil(); err != nil {
		return model.Entry{}, err
	}
	var adjusts pgtype.Int8
	if in.AdjustsEntryUUID != nil {
		e, err := s.GetEntry(ctx, orgID, *in.AdjustsEntryUUID)
		if err != nil || e.Account.UUID != acc.Uuid {
			return model.Entry{}, validation("adjusts_entry_uuid", "not_found", "unknown entry of this account")
		}
		row, err := s.q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: e.UUID})
		if err != nil {
			return model.Entry{}, err
		}
		adjusts = int8Of(row.ID)
	}
	rental, hasRental, err := s.postingRental(ctx, orgID, acc, in.RentalUUID)
	if err != nil {
		return model.Entry{}, err
	}
	def := acc.BranchID.Int64
	if hasRental {
		def = rental.PickupBranchID
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", in.BranchUUID, def)
	if err != nil {
		return model.Entry{}, err
	}
	row, err := s.q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindAdjustment, Direction: in.Direction, Purpose: purpose,
		Amount: in.Amount.Numeric(), Currency: acc.Currency, EntryDate: dateOf(date), DueDate: dateOf(due),
		Description: rental.Reference, Reason: reason, SourceType: model.SourceManual, RentalID: rentalOf(rental, hasRental),
		ReservationID: reservationOf(rental, hasRental), AdjustsEntryID: adjusts, BranchID: int8Of(branch), CreatedBy: actorID(ctx),
	})
	if err != nil {
		return model.Entry{}, err
	}
	s.publish(ctx, model.ResourceAccounts, "updated", id)
	s.publish(ctx, model.ResourceEntries, "created", row.Uuid)
	return s.GetEntry(ctx, orgID, row.Uuid)
}

// Reverse posts the exact opposite of one entry (once, with a reason,
// tenant.finance.ledger.adjust). Accruals are managed by their rental (a
// waived charge or a changed price reverses them); a cash movement that
// settled the entry is reversed with it.
func (s *Service) Reverse(ctx context.Context, orgID int64, id uuid.UUID, in model.ReverseInput) (model.Entry, error) {
	if !has(ctx, rbac.PermTenantFinanceLedgerAdjust) {
		return model.Entry{}, ErrForbidden
	}
	if _, err := s.GetEntry(ctx, orgID, id); err != nil {
		return model.Entry{}, err
	}
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return model.Entry{}, err
	}
	e, err := s.q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Entry{}, err
	}
	switch {
	case e.Kind == model.KindReversal:
		return model.Entry{}, stateErr("reversal_of_reversal", "a reversal cannot be reversed; post the entry again")
	case e.Kind == model.KindAccrual:
		return model.Entry{}, stateErr("accrual_managed_by_rental", "accruals follow their rental: waive the charge or change the rental, or post an adjustment")
	case e.Kind == model.KindOffset:
		return model.Entry{}, stateErr("offset_pair", "an offset is one of a pair; post an adjustment instead")
	case e.Reversed:
		return model.Entry{}, stateErr("already_reversed", "the entry is already reversed")
	}
	dir := model.Credit
	if e.Direction == model.Credit {
		dir = model.Debit
	}
	var out uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: e.AccountID}); err != nil {
			return err
		}
		row, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
			OrganizationID: orgID, AccountID: e.AccountID, Kind: model.KindReversal, Direction: dir, Purpose: e.Purpose,
			Amount: e.Amount, Currency: e.Currency, EntryDate: dateOf(s.today()), DueDate: e.DueDate, Description: e.Description,
			Reason: reason, SourceType: e.SourceType, SourceUuid: e.SourceUuid, RentalID: e.RentalID, ReservationID: e.ReservationID,
			RentalChargeID: e.RentalChargeID, PaymentID: e.PaymentID, PaymentRefundID: e.PaymentRefundID,
			ReversesEntryID: int8Of(e.ID), BranchID: e.BranchID, CreatedBy: actorID(ctx),
		})
		if uniqueViolation(err) {
			return stateErr("already_reversed", "the entry is already reversed")
		}
		if err != nil {
			return err
		}
		out = row.Uuid
		return s.reverseSettlement(ctx, q, orgID, e.ID, reason)
	})
	if err != nil {
		return model.Entry{}, err
	}
	s.publish(ctx, model.ResourceEntries, "created", out)
	s.publish(ctx, model.ResourceEntries, "updated", id)
	return s.GetEntry(ctx, orgID, out)
}

// Offset applies a held deposit to the rent / charges of the account
// (mahsup): a pair of offset entries that nets to zero on the account and
// moves the amount out of the deposit (tenant.finance.ledger.adjust).
func (s *Service) Offset(ctx context.Context, orgID int64, id uuid.UUID, in model.OffsetInput) ([]model.Entry, error) {
	if !has(ctx, rbac.PermTenantFinanceLedgerAdjust) {
		return nil, ErrForbidden
	}
	acc, view, err := s.account(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	ve := &ValidationError{}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	} else if in.Amount.Cmp(view.Balance.DepositHeld) > 0 {
		ve.add("amount", "too_large", "the amount exceeds the deposit held ("+view.Balance.DepositHeld.String()+")")
	}
	target := in.Purpose
	if target == "" {
		target = model.PurposeRent
	}
	if !contains([]string{model.PurposeRent, model.PurposeCharge, model.PurposeGeneral}, target) {
		ve.add("purpose", "invalid", "purpose must be rent, charge or general")
	}
	reason, rerr := requireReason("reason", in.Reason)
	if rerr != nil {
		ve.add("reason", "required", "a reason is required (3-500 characters)")
	}
	if err := ve.orNil(); err != nil {
		return nil, err
	}
	rental, hasRental, err := s.postingRental(ctx, orgID, acc, in.RentalUUID)
	if err != nil {
		return nil, err
	}
	def := acc.BranchID.Int64
	if hasRental {
		def = rental.PickupBranchID
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", nil, def)
	if err != nil {
		return nil, err
	}
	group := uuid.New()
	today := s.today()
	ids := []uuid.UUID{}
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
			return err
		}
		for _, leg := range []struct{ dir, purpose string }{{model.Debit, model.PurposeDeposit}, {model.Credit, target}} {
			row, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
				OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindOffset, Direction: leg.dir, Purpose: leg.purpose,
				Amount: in.Amount.Numeric(), Currency: acc.Currency, EntryDate: dateOf(today), DueDate: dateOf(today),
				Description: rental.Reference, Reason: reason, SourceType: model.SourceManual, RentalID: rentalOf(rental, hasRental),
				ReservationID: reservationOf(rental, hasRental), OffsetGroup: uuidOf(group), BranchID: int8Of(branch),
				CreatedBy: actorID(ctx),
			})
			if err != nil {
				return err
			}
			ids = append(ids, row.Uuid)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publish(ctx, model.ResourceAccounts, "updated", id)
	s.publish(ctx, model.ResourceEntries, "created", ids...)
	out := make([]model.Entry, 0, len(ids))
	for _, eid := range ids {
		e, err := s.GetEntry(ctx, orgID, eid)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
