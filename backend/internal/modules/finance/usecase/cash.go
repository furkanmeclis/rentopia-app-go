package usecase

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// CashAccountSortColumns are the sortable cash account columns.
var CashAccountSortColumns = map[string]string{"code": "code", "name": "name", "balance": "balance", "created_at": "created_at"}

// MovementSortColumns are the sortable movement columns.
var MovementSortColumns = map[string]string{"movement_date": "movement_date", "amount": "amount", "created_at": "created_at"}

var (
	codeRE     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)
	currencyRE = regexp.MustCompile(`^[A-Z]{3}$`)
	ibanRE     = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
)

// ListCashAccounts lists cash / bank accounts with their balances. A
// branch-scoped member sees the accounts of their branches only (head
// office accounts need every branch).
func (s *Service) ListCashAccounts(ctx context.Context, orgID int64, f model.CashAccountFilter) ([]model.CashAccount, int64, error) {
	branches, err := s.branchIDs(ctx, orgID, "branch", f.BranchUUIDs)
	if err != nil {
		return nil, 0, err
	}
	sortField := f.SortField
	if _, ok := CashAccountSortColumns[sortField]; !ok {
		sortField, f.SortDesc = "code", false
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	rows, err := s.q.ListCashAccounts(ctx, db.ListCashAccountsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Ids: nilIDs(f.IDs), Kinds: nilIfEmpty(f.Kinds),
		Statuses: nilIfEmpty(f.Statuses), FilterBranchIds: nilIDs(branches), HeadOffice: f.HeadOffice, Q: textOf(f.Q),
		SortField: sortField, SortDesc: f.SortDesc, LimitCount: limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.CashAccount, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, model.CashAccount{
			UUID: r.Uuid, Code: r.Code, Name: r.Name, Kind: r.Kind, Branch: refOf(r.BranchUuid, r.BranchCode, r.BranchName),
			Currency: r.Currency, Provider: textPtr(r.Provider), OpeningBalance: m(r.OpeningBalance), OpeningDate: dateString(r.OpeningDate),
			BankName: r.BankName, IBAN: r.Iban, Status: r.Status, Note: r.Note, TotalIn: m(r.TotalIn), TotalOut: m(r.TotalOut),
			Balance: m(r.Balance), MovementCount: r.MovementCount, LastMovementAt: timePtr(r.LastMovementAt),
			CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		})
	}
	return out, total, nil
}

// cashAccount loads one cash account the caller may see.
func (s *Service) cashAccount(ctx context.Context, orgID int64, id uuid.UUID) (db.GetCashAccountRowRow, model.CashAccount, error) {
	row, err := s.q.GetCashAccountRow(ctx, db.GetCashAccountRowParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, model.CashAccount{}, ErrCashAccountNotFound
	}
	if err != nil {
		return row, model.CashAccount{}, err
	}
	items, _, err := s.ListCashAccounts(ctx, orgID, model.CashAccountFilter{IDs: []int64{row.ID}, Limit: 1})
	if err != nil {
		return row, model.CashAccount{}, err
	}
	if len(items) == 0 {
		return row, model.CashAccount{}, ErrCashAccountNotFound
	}
	return row, items[0], nil
}

// GetCashAccount returns one cash account.
func (s *Service) GetCashAccount(ctx context.Context, orgID int64, id uuid.UUID) (model.CashAccount, error) {
	_, a, err := s.cashAccount(ctx, orgID, id)
	return a, err
}

func normalizeIBAN(v string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v), " ", ""))
}

// CreateCashAccount opens a till or a bank account.
func (s *Service) CreateCashAccount(ctx context.Context, orgID int64, in model.CashAccountInput) (model.CashAccount, error) {
	ve := &ValidationError{}
	code := strings.TrimSpace(deref(in.Code))
	if !codeRE.MatchString(code) {
		ve.add("code", "invalid", "code is required (letters, digits, - _ . ; at most 32)")
	}
	name := strings.TrimSpace(deref(in.Name))
	if name == "" || len([]rune(name)) > 128 {
		ve.add("name", "required", "name is required (at most 128 characters)")
	}
	kind := deref(in.Kind)
	if !contains(model.CashKinds, kind) {
		ve.add("kind", "invalid", "kind must be cash, bank or clearing")
	}
	provider := checkProvider(ve, kind, in.Provider)
	currency := strings.ToUpper(strings.TrimSpace(deref(in.Currency)))
	if currency == "" {
		currency = string(money.TRY)
	}
	if !currencyRE.MatchString(currency) {
		ve.add("currency", "invalid", "currency must be an ISO 4217 code")
	}
	opening := money.Zero()
	if in.OpeningBalance != nil {
		opening = *in.OpeningBalance
		if kind == model.CashKindCash && opening.IsNegative() {
			ve.add("opening_balance", "invalid", "a till cannot open negative")
		}
	}
	openingDate := optionalDate(ve, "opening_date", in.OpeningDate, s.today())
	iban := normalizeIBAN(deref(in.IBAN))
	if iban != "" && !ibanRE.MatchString(iban) {
		ve.add("iban", "invalid", "IBAN is not valid")
	}
	bankName := strings.TrimSpace(deref(in.BankName))
	if len(bankName) > 128 {
		ve.add("bank_name", "too_long", "bank_name is at most 128 characters")
	}
	if err := ve.orNil(); err != nil {
		return model.CashAccount{}, err
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", in.BranchUUID, 0)
	if err != nil {
		return model.CashAccount{}, err
	}
	row, err := s.q.InsertCashAccount(ctx, db.InsertCashAccountParams{
		OrganizationID: orgID, Code: code, Name: name, Kind: kind, BranchID: int8Of(branch), Currency: currency,
		OpeningBalance: opening.Numeric(), OpeningDate: dateOf(openingDate), BankName: bankName, Iban: iban,
		Note: strings.TrimSpace(deref(in.Note)), CreatedBy: actorID(ctx),
	})
	if uniqueViolation(err) {
		return model.CashAccount{}, ErrCodeTaken
	}
	if err != nil {
		return model.CashAccount{}, err
	}
	if provider.Valid {
		if err := s.setProvider(ctx, orgID, row.ID, provider); err != nil {
			return model.CashAccount{}, err
		}
	}
	s.publish(ctx, model.ResourceCashAccounts, "created", row.Uuid)
	return s.GetCashAccount(ctx, orgID, row.Uuid)
}

// PatchCashAccount changes a cash account. The kind and currency are fixed;
// the opening balance can change until the first movement.
func (s *Service) PatchCashAccount(ctx context.Context, orgID int64, id uuid.UUID, in model.CashAccountInput) (model.CashAccount, error) {
	row, view, err := s.cashAccount(ctx, orgID, id)
	if err != nil {
		return model.CashAccount{}, err
	}
	ve := &ValidationError{}
	p := db.UpdateCashAccountParams{
		OrganizationID: orgID, ID: row.ID, Code: row.Code, Name: row.Name, BranchID: row.BranchID, BankName: row.BankName,
		Iban: row.Iban, Note: row.Note, Status: row.Status, OpeningBalance: row.OpeningBalance, OpeningDate: row.OpeningDate,
	}
	if in.Code != nil {
		p.Code = strings.TrimSpace(*in.Code)
		if !codeRE.MatchString(p.Code) {
			ve.add("code", "invalid", "code is required (letters, digits, - _ . ; at most 32)")
		}
	}
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
		if p.Name == "" || len([]rune(p.Name)) > 128 {
			ve.add("name", "required", "name is required (at most 128 characters)")
		}
	}
	if in.Kind != nil && *in.Kind != row.Kind {
		ve.add("kind", "immutable", "the kind of an account cannot change")
	}
	if in.Currency != nil && !strings.EqualFold(*in.Currency, row.Currency) {
		ve.add("currency", "immutable", "the currency of an account cannot change")
	}
	if in.OpeningBalance != nil || in.OpeningDate != nil {
		if view.MovementCount > 0 {
			ve.add("opening_balance", "has_movements", "the opening balance is fixed once the account has movements")
		}
		if in.OpeningBalance != nil {
			p.OpeningBalance = in.OpeningBalance.Numeric()
		}
		if in.OpeningDate != nil {
			p.OpeningDate = dateOf(optionalDate(ve, "opening_date", in.OpeningDate, s.today()))
		}
	}
	if in.BankName != nil {
		p.BankName = strings.TrimSpace(*in.BankName)
	}
	if in.IBAN != nil {
		p.Iban = normalizeIBAN(*in.IBAN)
		if p.Iban != "" && !ibanRE.MatchString(p.Iban) {
			ve.add("iban", "invalid", "IBAN is not valid")
		}
	}
	if in.Note != nil {
		p.Note = strings.TrimSpace(*in.Note)
	}
	provider := checkProvider(ve, row.Kind, in.Provider)
	if in.Status != nil {
		if *in.Status != model.AccountActive && *in.Status != model.AccountArchived {
			ve.add("status", "invalid", "status must be active or archived")
		}
		p.Status = *in.Status
	}
	if err := ve.orNil(); err != nil {
		return model.CashAccount{}, err
	}
	if in.BranchUUID != nil || in.ClearBranch {
		var target *uuid.UUID
		if !in.ClearBranch {
			target = in.BranchUUID
		}
		b, err := s.branchFor(ctx, orgID, "branch_uuid", target, 0)
		if err != nil {
			return model.CashAccount{}, err
		}
		p.BranchID = int8Of(b)
	}
	if _, err := s.q.UpdateCashAccount(ctx, p); err != nil {
		if uniqueViolation(err) {
			return model.CashAccount{}, ErrCodeTaken
		}
		return model.CashAccount{}, err
	}
	if in.Provider != nil {
		if err := s.setProvider(ctx, orgID, row.ID, provider); err != nil {
			return model.CashAccount{}, err
		}
	}
	s.publish(ctx, model.ResourceCashAccounts, "updated", id)
	return s.GetCashAccount(ctx, orgID, id)
}

// SetCashAccountStatus archives / activates an account (bulk); returns the
// previous status.
func (s *Service) SetCashAccountStatus(ctx context.Context, orgID int64, id uuid.UUID, status string) (string, error) {
	row, _, err := s.cashAccount(ctx, orgID, id)
	if err != nil {
		return "", err
	}
	prev, err := s.q.SetCashAccountStatus(ctx, db.SetCashAccountStatusParams{OrganizationID: orgID, ID: row.ID, Status: status})
	if err != nil {
		return "", err
	}
	return prev, nil
}

// lockedAccount is a cash account locked for a movement.
type lockedAccount struct {
	row     db.GetCashAccountRowRow
	balance money.Money
}

// lockForMovement loads (caller scope) and locks cash accounts in id order.
type cashTarget struct {
	field string
	id    uuid.UUID
}

func (s *Service) lockForMovement(ctx context.Context, q *db.Queries, orgID int64, targets ...cashTarget) ([]lockedAccount, error) {
	rows := make([]db.GetCashAccountRowRow, 0, len(targets))
	internal := make([]int64, 0, len(targets))
	for _, t := range targets {
		// Read on the transaction's connection (a second pool connection
		// per transfer could exhaust the pool under concurrent transfers).
		row, err := q.GetCashAccountRow(ctx, db.GetCashAccountRowParams{OrganizationID: orgID, Uuid: t.id})
		if err != nil || !cashVisible(ctx, row.BranchID) {
			if err != nil && !notFound(err) {
				return nil, err
			}
			return nil, validation(t.field, "not_found", "unknown cash account")
		}
		if row.Status != model.AccountActive {
			return nil, validation(t.field, "archived", "the cash account is archived")
		}
		rows = append(rows, row)
		internal = append(internal, row.ID)
	}
	if _, err := q.LockCashAccounts(ctx, db.LockCashAccountsParams{OrganizationID: orgID, Ids: internal}); err != nil {
		return nil, err
	}
	out := make([]lockedAccount, 0, len(rows))
	for _, row := range rows {
		bal, err := q.CashAccountBalance(ctx, db.CashAccountBalanceParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return nil, err
		}
		out = append(out, lockedAccount{row: row, balance: m(bal)})
	}
	return out, nil
}

// cashVisible applies the branch scope to a cash account (head office
// accounts need every branch).
func cashVisible(ctx context.Context, branch pgtype.Int8) bool {
	if branch.Valid {
		return orgctx.CheckBranch(ctx, branch.Int64) == nil
	}
	return orgctx.RequireAllBranches(ctx) == nil
}

// checkFunds refuses taking more out of a till than it holds (bank
// accounts may go negative: overdraft facilities are the bank's business).
func checkFunds(field string, a lockedAccount, out money.Money) error {
	if a.row.Kind == model.CashKindCash && a.balance.Sub(out).IsNegative() {
		return validation(field, "insufficient_funds", "the till holds "+a.balance.String()+" "+a.row.Currency)
	}
	return nil
}

// settleEntry records a collection / refund entry in a cash account (same
// transaction as the entry).
func (s *Service) settleEntry(ctx context.Context, q *db.Queries, orgID int64, cash uuid.UUID, entryID int64, kind string,
	amount money.Money, currency string, date time.Time, desc string) error {
	locked, err := s.lockForMovement(ctx, q, orgID, cashTarget{"cash_account_uuid", cash})
	if err != nil {
		return err
	}
	a := locked[0]
	if a.row.Currency != currency {
		return validation("cash_account_uuid", "currency_mismatch", "the cash account holds another currency")
	}
	dir := model.DirIn
	if kind == model.MoveRefund {
		dir = model.DirOut
		if err := checkFunds("cash_account_uuid", a, amount); err != nil {
			return err
		}
	}
	row, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
		OrganizationID: orgID, CashAccountID: a.row.ID, Kind: kind, Direction: dir, Amount: amount.Numeric(), Currency: currency,
		MovementDate: dateOf(date), Description: desc, BranchID: a.row.BranchID, LedgerEntryID: int8Of(entryID), CreatedBy: actorID(ctx),
	})
	if uniqueViolation(err) {
		return validation("ledger_entry_uuid", "already_settled", "the entry is already recorded in a cash account")
	}
	if err != nil {
		return err
	}
	s.publish(ctx, model.ResourceMovements, "created", row.Uuid)
	s.publish(ctx, model.ResourceCashAccounts, "updated", a.row.Uuid)
	return nil
}

// reverseSettlement reverses the live movement that settled an entry.
func (s *Service) reverseSettlement(ctx context.Context, q *db.Queries, orgID, entryID int64, reason string) error {
	mv, err := q.GetMovementByEntry(ctx, db.GetMovementByEntryParams{OrganizationID: orgID, LedgerEntryID: int8Of(entryID)})
	if notFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := model.DirOut
	if mv.Direction == model.DirOut {
		dir = model.DirIn
	}
	row, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
		OrganizationID: orgID, CashAccountID: mv.CashAccountID, Kind: model.MoveReversal, Direction: dir, Amount: mv.Amount,
		Currency: mv.Currency, MovementDate: dateOf(s.today()), Description: mv.Description, Reason: reason,
		BranchID: mv.BranchID, ReversesMovementID: int8Of(mv.ID), CreatedBy: actorID(ctx),
	})
	if err != nil {
		return err
	}
	s.publish(ctx, model.ResourceMovements, "created", row.Uuid)
	return nil
}

// ListMovements lists cash movements (the movement ledger, an account's
// movements) with the running balance of their account.
func (s *Service) ListMovements(ctx context.Context, orgID int64, f model.MovementFilter) ([]model.Movement, int64, error) {
	p := db.ListCashMovementsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Ids: nilIDs(f.IDs), Kinds: nilIfEmpty(f.Kinds),
		Directions: nilIfEmpty(f.Directions), DateFrom: dateNarg(f.From), DateTo: dateNarg(f.To), Q: textOf(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if _, ok := MovementSortColumns[p.SortField]; !ok {
		p.SortField, p.SortDesc = "movement_date", true
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	if f.AccountUUID != nil {
		row, err := s.q.GetCashAccountRow(ctx, db.GetCashAccountRowParams{OrganizationID: orgID, Uuid: *f.AccountUUID})
		if notFound(err) {
			return nil, 0, validation("account", "not_found", "unknown cash account")
		}
		if err != nil {
			return nil, 0, err
		}
		p.AccountID = int8Of(row.ID)
	}
	for _, id := range f.AccountUUIDs {
		row, err := s.q.GetCashAccountRow(ctx, db.GetCashAccountRowParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil, 0, validation("account", "not_found", "unknown cash account")
		}
		if err != nil {
			return nil, 0, err
		}
		p.FilterAccountIds = append(p.FilterAccountIds, row.ID)
	}
	for _, id := range f.CategoryUUIDs {
		c, err := s.q.GetExpenseCategory(ctx, db.GetExpenseCategoryParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil, 0, validation("category", "not_found", "unknown category")
		}
		if err != nil {
			return nil, 0, err
		}
		p.CategoryIds = append(p.CategoryIds, c.ID)
	}
	branches, err := s.branchIDs(ctx, orgID, "branch", f.BranchUUIDs)
	if err != nil {
		return nil, 0, err
	}
	p.FilterBranchIds = nilIDs(branches)
	rows, err := s.q.ListCashMovements(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Movement, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, model.Movement{
			UUID: r.Uuid, Account: model.Ref{UUID: r.AccountUuid, Code: r.AccountCode, Name: r.AccountName}, AccountKind: r.AccountKind,
			Kind: r.Kind, Direction: r.Direction, Amount: m(r.Amount), Currency: r.Currency, MovementDate: dateString(r.MovementDate),
			Description: r.Description, Reason: r.Reason, CounterAccount: refOf(r.CounterAccountUuid, r.CounterAccountCode, r.CounterAccountName),
			Category: refOf(r.CategoryUuid, r.CategoryCode, r.CategoryName), TransferGroup: uuidPtr(r.TransferGroup),
			LedgerEntry: uuidPtr(r.LedgerEntryUuid), ExpenseUUID: uuidPtr(r.ExpenseUuid), ReversesUUID: uuidPtr(r.ReversesUuid),
			ReversedByUUID: uuidPtr(r.ReversedByUuid), Branch: refOf(r.BranchUuid, r.BranchCode, r.BranchName),
			RunningBalance: m(r.RunningBalance), CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail),
			CreatedAt: r.CreatedAt.Time.UTC(),
		})
	}
	return out, total, nil
}

// GetMovement returns one movement the caller may see.
func (s *Service) GetMovement(ctx context.Context, orgID int64, id uuid.UUID) (model.Movement, error) {
	row, err := s.q.GetCashMovementRow(ctx, db.GetCashMovementRowParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Movement{}, ErrMovementNotFound
	}
	if err != nil {
		return model.Movement{}, err
	}
	items, _, err := s.ListMovements(ctx, orgID, model.MovementFilter{IDs: []int64{row.ID}, Limit: 1})
	if err != nil {
		return model.Movement{}, err
	}
	if len(items) == 0 {
		return model.Movement{}, ErrMovementNotFound
	}
	return items[0], nil
}

// categoryRef resolves an optional active category.
func (s *Service) categoryRef(ctx context.Context, orgID int64, field string, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil {
		return pgtype.Int8{}, nil
	}
	c, err := s.q.GetExpenseCategory(ctx, db.GetExpenseCategoryParams{OrganizationID: orgID, Uuid: *id})
	if notFound(err) {
		return pgtype.Int8{}, validation(field, "not_found", "unknown category")
	}
	if err != nil {
		return pgtype.Int8{}, err
	}
	return int8Of(c.ID), nil
}

// RecordMovement records a collection deposit, a refund paid out, a
// supplier payment or an adjustment on a cash account.
func (s *Service) RecordMovement(ctx context.Context, orgID int64, id uuid.UUID, in model.MovementInput) (model.Movement, error) {
	if _, _, err := s.cashAccount(ctx, orgID, id); err != nil {
		return model.Movement{}, err
	}
	ve := &ValidationError{}
	if !contains(model.ManualMovementKinds, in.Kind) {
		ve.add("kind", "invalid", "kind must be collection, refund, supplier_payment or adjustment")
	}
	dir := in.Direction
	switch in.Kind {
	case model.MoveCollection:
		dir = model.DirIn
	case model.MoveRefund, model.MoveSupplierPayment:
		dir = model.DirOut
	case model.MoveAdjustment:
		if dir != model.DirIn && dir != model.DirOut {
			ve.add("direction", "invalid", "direction must be in or out")
		}
	}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	reason := strings.TrimSpace(in.Reason)
	if in.Kind == model.MoveAdjustment {
		if _, err := requireReason("reason", reason); err != nil {
			ve.add("reason", "required", "a reason is required (3-500 characters)")
		}
	}
	if len(in.Description) > 500 {
		ve.add("description", "too_long", "description is at most 500 characters")
	}
	date := optionalDate(ve, "movement_date", in.MovementDate, s.today())
	if err := ve.orNil(); err != nil {
		return model.Movement{}, err
	}
	category, err := s.categoryRef(ctx, orgID, "category_uuid", in.CategoryUUID)
	if err != nil {
		return model.Movement{}, err
	}
	var entry pgtype.Int8
	if in.LedgerEntryUUID != nil {
		if in.Kind != model.MoveCollection && in.Kind != model.MoveRefund {
			return model.Movement{}, validation("ledger_entry_uuid", "invalid", "only collections and refunds settle an entry")
		}
		if _, err := s.GetEntry(ctx, orgID, *in.LedgerEntryUUID); err != nil {
			return model.Movement{}, validation("ledger_entry_uuid", "not_found", "unknown ledger entry")
		}
		e, err := s.q.GetLedgerEntryForDeposit(ctx, db.GetLedgerEntryForDepositParams{OrganizationID: orgID, Uuid: *in.LedgerEntryUUID})
		if err != nil {
			return model.Movement{}, err
		}
		if e.Kind != in.Kind || !m(e.Amount).Equal(*in.Amount) {
			return model.Movement{}, validation("ledger_entry_uuid", "mismatch", "the entry must be a "+in.Kind+" of the same amount")
		}
		entry = int8Of(e.ID)
	}
	var out uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		locked, err := s.lockForMovement(ctx, q, orgID, cashTarget{"cash_account_uuid", id})
		if err != nil {
			return mapAccountField(err)
		}
		a := locked[0]
		if dir == model.DirOut {
			if err := checkFunds("amount", a, *in.Amount); err != nil {
				return err
			}
		}
		row, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
			OrganizationID: orgID, CashAccountID: a.row.ID, Kind: in.Kind, Direction: dir, Amount: in.Amount.Numeric(),
			Currency: a.row.Currency, MovementDate: dateOf(date), Description: strings.TrimSpace(in.Description), Reason: reason,
			BranchID: a.row.BranchID, CategoryID: category, LedgerEntryID: entry, CreatedBy: actorID(ctx),
		})
		if uniqueViolation(err) {
			return validation("ledger_entry_uuid", "already_settled", "the entry is already recorded in a cash account")
		}
		if err != nil {
			return err
		}
		out = row.Uuid
		return nil
	})
	if err != nil {
		return model.Movement{}, err
	}
	s.publish(ctx, model.ResourceMovements, "created", out)
	s.publish(ctx, model.ResourceCashAccounts, "updated", id)
	return s.GetMovement(ctx, orgID, out)
}

// mapAccountField turns "unknown cash account" of the path account into a
// not found (404).
func mapAccountField(err error) error {
	if ve, ok := err.(*ValidationError); ok && len(ve.Details) == 1 && ve.Details[0].Code == "not_found" {
		return ErrCashAccountNotFound
	}
	return err
}

// Transfer moves money between two accounts of the organization: both
// movements are written in one transaction with both accounts locked (in id
// order), so a transfer is never half done.
func (s *Service) Transfer(ctx context.Context, orgID int64, in model.TransferInput) (model.Transfer, error) {
	ve := &ValidationError{}
	if in.FromAccountUUID == nil {
		ve.add("from_account_uuid", "required", "from_account_uuid is required")
	}
	if in.ToAccountUUID == nil {
		ve.add("to_account_uuid", "required", "to_account_uuid is required")
	}
	if in.FromAccountUUID != nil && in.ToAccountUUID != nil && *in.FromAccountUUID == *in.ToAccountUUID {
		ve.add("to_account_uuid", "same_account", "choose two different accounts")
	}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	if len(in.Description) > 500 {
		ve.add("description", "too_long", "description is at most 500 characters")
	}
	date := optionalDate(ve, "movement_date", in.MovementDate, s.today())
	if err := ve.orNil(); err != nil {
		return model.Transfer{}, err
	}
	group := uuid.New()
	var outID, inID uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		// One lock statement for both accounts (id order): two opposite
		// transfers cannot deadlock.
		locked, err := s.lockForMovement(ctx, q, orgID,
			cashTarget{"from_account_uuid", *in.FromAccountUUID}, cashTarget{"to_account_uuid", *in.ToAccountUUID})
		if err != nil {
			return err
		}
		f, t := locked[0], locked[1]
		if f.row.Currency != t.row.Currency {
			return validation("to_account_uuid", "currency_mismatch", "both accounts must hold the same currency")
		}
		if err := checkFunds("amount", f, *in.Amount); err != nil {
			return err
		}
		desc := strings.TrimSpace(in.Description)
		o, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
			OrganizationID: orgID, CashAccountID: f.row.ID, Kind: model.MoveTransferOut, Direction: model.DirOut,
			Amount: in.Amount.Numeric(), Currency: f.row.Currency, MovementDate: dateOf(date), Description: desc,
			BranchID: f.row.BranchID, TransferGroup: uuidOf(group), CounterAccountID: int8Of(t.row.ID), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		i, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
			OrganizationID: orgID, CashAccountID: t.row.ID, Kind: model.MoveTransferIn, Direction: model.DirIn,
			Amount: in.Amount.Numeric(), Currency: t.row.Currency, MovementDate: dateOf(date), Description: desc,
			BranchID: t.row.BranchID, TransferGroup: uuidOf(group), CounterAccountID: int8Of(f.row.ID), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		outID, inID = o.Uuid, i.Uuid
		return nil
	})
	if err != nil {
		return model.Transfer{}, err
	}
	s.publish(ctx, model.ResourceMovements, "created", outID, inID)
	s.publish(ctx, model.ResourceCashAccounts, "updated", *in.FromAccountUUID, *in.ToAccountUUID)
	o, err := s.GetMovement(ctx, orgID, outID)
	if err != nil {
		return model.Transfer{}, err
	}
	i, err := s.GetMovement(ctx, orgID, inID)
	if err != nil {
		return model.Transfer{}, err
	}
	return model.Transfer{Group: group, Out: o, In: i}, nil
}

// ReverseMovement reverses a movement with a reason (both legs of a
// transfer together). Expense payments are reversed by voiding the expense.
func (s *Service) ReverseMovement(ctx context.Context, orgID int64, id uuid.UUID, in model.ReverseInput) ([]model.Movement, error) {
	if _, err := s.GetMovement(ctx, orgID, id); err != nil {
		return nil, err
	}
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return nil, err
	}
	mv, err := s.q.GetCashMovementRow(ctx, db.GetCashMovementRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return nil, err
	}
	switch {
	case mv.Kind == model.MoveReversal:
		return nil, stateErr("reversal_of_reversal", "a reversal cannot be reversed")
	case mv.Kind == model.MoveExpense:
		return nil, stateErr("expense_payment", "void the expense to reverse its payment")
	case mv.Kind == model.MoveDepositIn || mv.Kind == model.MoveDepositOut:
		return nil, stateErr("deposit_movement", "deposit movements follow their deposit (resolve or release it)")
	case mv.Reversed:
		return nil, stateErr("already_reversed", "the movement is already reversed")
	}
	legs := []db.ListTransferPairRow{{ID: mv.ID, Uuid: mv.Uuid, CashAccountID: mv.CashAccountID, Direction: mv.Direction,
		Amount: mv.Amount, Currency: mv.Currency, BranchID: mv.BranchID}}
	if mv.TransferGroup.Valid {
		legs, err = s.q.ListTransferPair(ctx, db.ListTransferPairParams{OrganizationID: orgID, TransferGroup: mv.TransferGroup})
		if err != nil {
			return nil, err
		}
		// The other leg may be in a branch outside the caller's scope: a
		// transfer is reversed whole, so the caller needs both accounts.
		for _, l := range legs {
			if l.BranchID.Valid && orgctx.CheckBranch(ctx, l.BranchID.Int64) != nil || !l.BranchID.Valid && orgctx.RequireAllBranches(ctx) != nil {
				return nil, ErrAllBranchesRequired
			}
		}
	}
	ids := []uuid.UUID{}
	err = s.tx(ctx, func(q *db.Queries) error {
		accounts := make([]int64, 0, len(legs))
		for _, l := range legs {
			accounts = append(accounts, l.CashAccountID)
		}
		if _, err := q.LockCashAccounts(ctx, db.LockCashAccountsParams{OrganizationID: orgID, Ids: accounts}); err != nil {
			return err
		}
		for _, l := range legs {
			dir := model.DirOut
			if l.Direction == model.DirOut {
				dir = model.DirIn
			}
			row, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
				OrganizationID: orgID, CashAccountID: l.CashAccountID, Kind: model.MoveReversal, Direction: dir, Amount: l.Amount,
				Currency: l.Currency, MovementDate: dateOf(s.today()), Description: mv.Description, Reason: reason,
				BranchID: l.BranchID, ReversesMovementID: int8Of(l.ID), CreatedBy: actorID(ctx),
			})
			if uniqueViolation(err) {
				return stateErr("already_reversed", "the movement is already reversed")
			}
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
	s.publish(ctx, model.ResourceMovements, "created", ids...)
	s.publish(ctx, model.ResourceCashAccounts, "updated")
	out := make([]model.Movement, 0, len(ids))
	for _, mid := range ids {
		v, err := s.GetMovement(ctx, orgID, mid)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// SetMovementCategory classifies a movement (bulk "set category"); the
// one change a movement allows. Returns the previous category uuid.
func (s *Service) SetMovementCategory(ctx context.Context, orgID int64, id uuid.UUID, category *uuid.UUID) (*uuid.UUID, error) {
	mv, err := s.GetMovement(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	row, err := s.q.GetCashMovementRow(ctx, db.GetCashMovementRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return nil, err
	}
	cat, err := s.categoryRef(ctx, orgID, "category_uuid", category)
	if err != nil {
		return nil, err
	}
	if _, err := s.q.SetCashMovementCategory(ctx, db.SetCashMovementCategoryParams{OrganizationID: orgID, ID: row.ID, CategoryID: cat}); err != nil {
		return nil, err
	}
	var prev *uuid.UUID
	if mv.Category != nil {
		p := mv.Category.UUID
		prev = &p
	}
	return prev, nil
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// checkProvider validates the settlement provider of a cash account (a
// bank or clearing account may receive a provider's net settlement).
func checkProvider(ve *ValidationError, kind string, raw *string) pgtype.Text {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return pgtype.Text{}
	}
	v := strings.TrimSpace(*raw)
	if !contains(model.SettlementProviders, v) {
		ve.add("provider", "invalid", "unknown provider")
	} else if kind != model.CashKindBank && kind != model.CashKindClearing {
		ve.add("provider", "invalid", "only a bank or clearing account receives a provider settlement")
	}
	return pgtype.Text{String: v, Valid: true}
}

// setProvider marks (or clears) the settlement account of a provider; one
// active account per provider.
func (s *Service) setProvider(ctx context.Context, orgID, id int64, provider pgtype.Text) error {
	_, err := s.q.SetCashAccountProvider(ctx, db.SetCashAccountProviderParams{OrganizationID: orgID, ID: id, Provider: provider})
	if uniqueViolation(err) {
		return validation("provider", "taken", "another active account already receives this provider's settlement")
	}
	return err
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}
