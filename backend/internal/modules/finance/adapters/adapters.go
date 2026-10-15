// Package adapters connects finance to the shared engines: ioengine
// (exports of accounts, statements / entries, cash accounts, movements and
// expenses; imports of cash accounts, movements and expenses) and
// bulkengine ("set category" of movements and expenses). The organization
// always comes from the context (request scope or job row); the branch
// scope of the job applies through the use cases.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

var (
	errNoOrganization = errors.New("finance: organization scope is required")
	errNoImport       = errors.New("finance: this resource cannot be imported")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// ------------------------------------------------------------ query parsing

func csv(raw string) []string { return apiquery.SplitCSV(raw) }

func uuids(raw string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, p := range csv(raw) {
		if id, err := uuid.Parse(p); err == nil {
			out = append(out, id)
		} else {
			// An unknown id must match nothing, never everything.
			out = append(out, uuid.Nil)
		}
	}
	return out
}

func datePtr(raw string) *time.Time {
	if raw = strings.TrimSpace(raw); raw == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil
	}
	return &t
}

func moneyPtr(raw string) *money.Money {
	if raw = strings.TrimSpace(raw); raw == "" {
		return nil
	}
	v, err := money.Parse(raw)
	if err != nil {
		return nil
	}
	return &v
}

func uuidPtr(raw string) *uuid.UUID {
	if raw = strings.TrimSpace(raw); raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		id = uuid.Nil
	}
	return &id
}

// AccountFilter maps list query params to an account filter.
func AccountFilter(q map[string]string) model.AccountFilter {
	return model.AccountFilter{
		Q: strings.TrimSpace(q["q"]), BalanceStatuses: csv(q["balance_status"]), Statuses: csv(q["status"]),
		PartyKinds: csv(q["party_kind"]), BranchUUIDs: uuids(q["branch"]), DueBefore: datePtr(q["due_before"]),
		MinBalance: moneyPtr(q["min_balance"]), MaxBalance: moneyPtr(q["max_balance"]),
	}
}

// EntryFilter maps list query params to an entry filter.
func EntryFilter(q map[string]string) model.EntryFilter {
	return model.EntryFilter{
		Q: strings.TrimSpace(q["q"]), AccountUUID: uuidPtr(q["account"]), RentalUUID: uuidPtr(q["rental"]),
		Kinds: csv(q["kind"]), Purposes: csv(q["purpose"]), Directions: csv(q["direction"]), BranchUUIDs: uuids(q["branch"]),
		From: datePtr(q["from"]), To: datePtr(q["to"]),
	}
}

// CashFilter maps list query params to a cash account filter.
func CashFilter(q map[string]string) model.CashAccountFilter {
	head, _ := strconv.ParseBool(q["head_office"])
	return model.CashAccountFilter{
		Q: strings.TrimSpace(q["q"]), Kinds: csv(q["kind"]), Statuses: csv(q["status"]), BranchUUIDs: uuids(q["branch"]), HeadOffice: head,
	}
}

// MovementFilter maps list query params to a movement filter.
func MovementFilter(q map[string]string) model.MovementFilter {
	return model.MovementFilter{
		Q: strings.TrimSpace(q["q"]), AccountUUIDs: uuids(q["account"]), Kinds: csv(q["kind"]), Directions: csv(q["direction"]),
		CategoryUUIDs: uuids(q["category"]), BranchUUIDs: uuids(q["branch"]), From: datePtr(q["from"]), To: datePtr(q["to"]),
	}
}

// ExpenseFilter maps list query params to an expense filter.
func ExpenseFilter(q map[string]string) model.ExpenseFilter {
	return model.ExpenseFilter{
		Q: strings.TrimSpace(q["q"]), CategoryUUIDs: uuids(q["category"]), BranchUUIDs: uuids(q["branch"]),
		VehicleUUIDs: uuids(q["vehicle"]), Statuses: csv(q["status"]), From: datePtr(q["from"]), To: datePtr(q["to"]),
	}
}

func label(locale i18n.Locale, prefix, v string) string {
	if v == "" {
		return ""
	}
	key := prefix + v
	if t := i18n.Translate(locale, key); t != key {
		return t
	}
	return v
}

func refText(r *model.Ref) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Code + " " + r.Name)
}

func cell(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func get(row, defaults map[string]any, key string) string {
	if v := cell(row, key); v != "" {
		return v
	}
	return cell(defaults, key)
}

func strp(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func rowError(err error) (ioengine.RowResult, error) {
	var ve *usecase.ValidationError
	var se *usecase.StateError
	if errors.As(err, &ve) || errors.As(err, &se) || errors.Is(err, usecase.ErrCodeTaken) ||
		errors.Is(err, usecase.ErrCashAccountNotFound) || errors.Is(err, usecase.ErrCategoryNotFound) {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	return ioengine.RowResult{}, err
}

// --------------------------------------------------------------- accounts

// AccountsIO exports accounts with their balances.
type AccountsIO struct{ svc *usecase.Service }

// NewAccountsIO builds the adapter.
func NewAccountsIO(svc *usecase.Service) *AccountsIO { return &AccountsIO{svc: svc} }

func (a *AccountsIO) Resource() string { return model.ResourceAccounts }

func (a *AccountsIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "finance_accounts." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("code"), col("name"), col("party_kind"), col("branch"), col("currency"), col("debit"), col("credit"), col("balance"),
		col("overdue"), col("due_soon"), col("deposit_held"), col("balance_status"), col("payment_term_days"), col("status"),
		{Key: "uuid", LabelKey: "finance_accounts.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *AccountsIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := AccountFilter(query)
	f.Limit, f.SortField = usecase.MaxListTargets, "code"
	items, _, err := a.svc.ListAccounts(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "code": it.Code, "name": it.Name, "party_kind": label(locale, "finance.party.", it.PartyKind),
			"branch": refText(it.Branch), "currency": it.Currency, "debit": it.Balance.Debit.String(), "credit": it.Balance.Credit.String(),
			"balance": it.Balance.Balance.String(), "overdue": it.Balance.Overdue.String(), "due_soon": it.Balance.DueSoon.String(),
			"deposit_held": it.Balance.DepositHeld.String(), "balance_status": label(locale, "finance.balance_status.", it.Balance.Status),
			"payment_term_days": strconv.Itoa(int(it.PaymentTermDays)), "status": label(locale, "finance.account_status.", it.Status),
		})
	}
	return ioengine.Dataset{Resource: model.ResourceAccounts, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *AccountsIO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *AccountsIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *AccountsIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// ---------------------------------------------------------------- entries

// EntriesIO exports ledger entries: with query account=<uuid> it is the
// account statement (oldest first, with the running balance).
type EntriesIO struct{ svc *usecase.Service }

// NewEntriesIO builds the adapter.
func NewEntriesIO(svc *usecase.Service) *EntriesIO { return &EntriesIO{svc: svc} }

func (a *EntriesIO) Resource() string { return model.ResourceEntries }

func (a *EntriesIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "ledger_entries." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("entry_date"), col("due_date"), col("account"), col("kind"), col("purpose"), col("reference"), col("description"),
		col("debit"), col("credit"), col("running_balance"), col("currency"), col("branch"), col("reason"),
		{Key: "uuid", LabelKey: "ledger_entries.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *EntriesIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := EntryFilter(query)
	f.Limit = usecase.MaxListTargets
	items, _, err := a.svc.ListEntries(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		debit, credit := "", ""
		if it.Direction == model.Debit {
			debit = it.Amount.String()
		} else {
			credit = it.Amount.String()
		}
		ref := ""
		if it.Rental != nil {
			ref = it.Rental.Code
		} else if it.Reservation != nil {
			ref = it.Reservation.Code
		}
		reason := it.Reason
		if strings.HasPrefix(reason, "system:") {
			reason = label(locale, "finance.reason.", strings.TrimPrefix(reason, "system:"))
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "entry_date": it.EntryDate, "due_date": it.DueDate,
			"account": it.Account.Code + " " + it.Account.Name, "kind": label(locale, "finance.kind.", it.Kind),
			"purpose": label(locale, "finance.purpose.", it.Purpose), "reference": ref, "description": it.Description,
			"debit": debit, "credit": credit, "running_balance": it.RunningBalance.String(), "currency": it.Currency,
			"branch": refText(it.Branch), "reason": reason,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceEntries, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *EntriesIO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *EntriesIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *EntriesIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// ----------------------------------------------------------- cash accounts

// CashAccountsIO exports and imports cash / bank accounts (match by code).
type CashAccountsIO struct{ svc *usecase.Service }

// NewCashAccountsIO builds the adapter.
func NewCashAccountsIO(svc *usecase.Service) *CashAccountsIO { return &CashAccountsIO{svc: svc} }

func (a *CashAccountsIO) Resource() string { return model.ResourceCashAccounts }

func (a *CashAccountsIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "cash_accounts." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("code"), col("name"), col("kind"), col("branch_code"), col("currency"), col("opening_balance"), col("opening_date"),
		col("total_in"), col("total_out"), col("balance"), col("bank_name"), col("iban"), col("status"), col("note"),
		{Key: "uuid", LabelKey: "cash_accounts.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *CashAccountsIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := CashFilter(query)
	f.Limit = usecase.MaxListTargets
	items, _, err := a.svc.ListCashAccounts(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		branch := ""
		if it.Branch != nil {
			branch = it.Branch.Code
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "code": it.Code, "name": it.Name, "kind": label(locale, "finance.cash_kind.", it.Kind),
			"branch_code": branch, "currency": it.Currency, "opening_balance": it.OpeningBalance.String(), "opening_date": it.OpeningDate,
			"total_in": it.TotalIn.String(), "total_out": it.TotalOut.String(), "balance": it.Balance.String(), "bank_name": it.BankName,
			"iban": it.IBAN, "status": label(locale, "finance.account_status.", it.Status), "note": it.Note,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceCashAccounts, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *CashAccountsIO) ImportSchema() []ioengine.ImportField {
	f := func(key string, required bool, hint string) ioengine.ImportField {
		return ioengine.ImportField{Key: key, LabelKey: "cash_accounts." + key, Type: ioengine.ColumnTypeString, Required: required, DefaultHint: hint}
	}
	return []ioengine.ImportField{
		f("code", true, ""), f("name", true, ""), f("kind", true, "cash"), f("branch_code", false, ""), f("currency", false, "TRY"),
		f("opening_balance", false, "0"), f("opening_date", false, ""), f("bank_name", false, ""), f("iban", false, ""), f("note", false, ""),
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *CashAccountsIO) SampleRows() []map[string]any {
	return []map[string]any{
		{"code": "KASA-MRK", "name": "Merkez kasa", "kind": "cash", "branch_code": "MRK", "currency": "TRY", "opening_balance": "5000", "opening_date": "2026-01-01"},
		{"code": "BANKA-1", "name": "İş Bankası TL", "kind": "bank", "currency": "TRY", "opening_balance": "250000", "bank_name": "İş Bankası", "iban": "TR330006100519786457841326"},
	}
}

func (a *CashAccountsIO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	code := get(row, defaults, "code")
	in := model.CashAccountInput{
		Code: strp(code), Name: strp(get(row, defaults, "name")), Kind: strp(strings.ToLower(get(row, defaults, "kind"))),
		Currency: strp(strings.ToUpper(get(row, defaults, "currency"))), OpeningDate: strp(get(row, defaults, "opening_date")),
		BankName: strp(get(row, defaults, "bank_name")), IBAN: strp(get(row, defaults, "iban")), Note: strp(get(row, defaults, "note")),
	}
	if raw := get(row, defaults, "opening_balance"); raw != "" {
		v, err := money.Parse(raw)
		if err != nil {
			return ioengine.RowResult{OK: false, Error: "opening_balance is not an amount"}, nil
		}
		in.OpeningBalance = &v
	}
	if bc := get(row, defaults, "branch_code"); bc != "" {
		id, err := a.svc.BranchUUIDByCode(ctx, orgID, bc)
		if err != nil {
			return rowError(err)
		}
		in.BranchUUID = &id
	}
	existing, err := a.svc.CashAccountUUIDByCode(ctx, orgID, code)
	if err == nil {
		prev, err := a.svc.GetCashAccount(ctx, orgID, existing)
		if err != nil {
			return rowError(err)
		}
		in.Kind, in.Currency = nil, nil
		if prev.MovementCount > 0 {
			in.OpeningBalance, in.OpeningDate = nil, nil
		}
		if _, err := a.svc.PatchCashAccount(ctx, orgID, existing, in); err != nil {
			return rowError(err)
		}
		previous := map[string]any{"name": prev.Name, "bank_name": prev.BankName, "iban": prev.IBAN, "note": prev.Note, "status": prev.Status}
		return ioengine.RowResult{OK: true, EntityType: "cash_account", EntityUUID: existing.String(), Op: "update", Previous: previous}, nil
	}
	created, err := a.svc.CreateCashAccount(ctx, orgID, in)
	if err != nil {
		return rowError(err)
	}
	return ioengine.RowResult{OK: true, EntityType: "cash_account", EntityUUID: created.UUID.String(), Op: "create"}, nil
}

// RevertRow archives an imported account (accounts are never deleted) or
// restores an updated one.
func (a *CashAccountsIO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil || entityType != "cash_account" {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	if len(previous) == 0 {
		_, err := a.svc.SetCashAccountStatus(ctx, orgID, id, model.AccountArchived)
		return err
	}
	in := model.CashAccountInput{
		Name: strp(cell(previous, "name")), BankName: strp(cell(previous, "bank_name")), IBAN: strp(cell(previous, "iban")),
		Note: strp(cell(previous, "note")), Status: strp(cell(previous, "status")),
	}
	_, err = a.svc.PatchCashAccount(ctx, orgID, id, in)
	return err
}

// --------------------------------------------------------------- movements

// MovementsIO exports movements and imports bank / till statement lines.
type MovementsIO struct{ svc *usecase.Service }

// NewMovementsIO builds the adapter.
func NewMovementsIO(svc *usecase.Service) *MovementsIO { return &MovementsIO{svc: svc} }

func (a *MovementsIO) Resource() string { return model.ResourceMovements }

func (a *MovementsIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "cash_movements." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("movement_date"), col("account_code"), col("account"), col("kind"), col("direction"), col("amount"), col("currency"),
		col("running_balance"), col("category_code"), col("description"), col("reason"), col("counter_account"), col("branch"),
		{Key: "uuid", LabelKey: "cash_movements.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *MovementsIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := MovementFilter(query)
	f.Limit, f.SortField, f.SortDesc = usecase.MaxListTargets, "movement_date", false
	items, _, err := a.svc.ListMovements(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		cat := ""
		if it.Category != nil {
			cat = it.Category.Code
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "movement_date": it.MovementDate, "account_code": it.Account.Code, "account": it.Account.Name,
			"kind": label(locale, "finance.movement_kind.", it.Kind), "direction": label(locale, "finance.direction.", it.Direction),
			"amount": it.Amount.String(), "currency": it.Currency, "running_balance": it.RunningBalance.String(), "category_code": cat,
			"description": it.Description, "reason": it.Reason, "counter_account": refText(it.CounterAccount), "branch": refText(it.Branch),
		})
	}
	return ioengine.Dataset{Resource: model.ResourceMovements, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *MovementsIO) ImportSchema() []ioengine.ImportField {
	f := func(key string, required bool, hint string) ioengine.ImportField {
		return ioengine.ImportField{Key: key, LabelKey: "cash_movements." + key, Type: ioengine.ColumnTypeString, Required: required, DefaultHint: hint}
	}
	return []ioengine.ImportField{
		f("account_code", true, ""), f("movement_date", false, ""), f("kind", true, "adjustment"), f("direction", false, "in"),
		f("amount", true, ""), f("description", false, ""), f("reason", false, "Banka ekstresi"), f("category_code", false, ""),
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *MovementsIO) SampleRows() []map[string]any {
	return []map[string]any{
		{"account_code": "BANKA-1", "movement_date": "2026-03-01", "kind": "collection", "amount": "4500", "description": "Havale - ABC Ltd."},
		{"account_code": "BANKA-1", "movement_date": "2026-03-02", "kind": "adjustment", "direction": "out", "amount": "12.50",
			"description": "EFT masrafı", "reason": "Banka ekstresi", "category_code": "other"},
	}
}

func (a *MovementsIO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	account, err := a.svc.CashAccountUUIDByCode(ctx, orgID, get(row, defaults, "account_code"))
	if err != nil {
		return ioengine.RowResult{OK: false, Error: "unknown account_code"}, nil
	}
	amount, err := money.Parse(get(row, defaults, "amount"))
	if err != nil {
		return ioengine.RowResult{OK: false, Error: "amount is not an amount"}, nil
	}
	in := model.MovementInput{
		Kind: strings.ToLower(get(row, defaults, "kind")), Direction: strings.ToLower(get(row, defaults, "direction")), Amount: &amount,
		MovementDate: strp(get(row, defaults, "movement_date")), Description: get(row, defaults, "description"), Reason: get(row, defaults, "reason"),
	}
	if cc := get(row, defaults, "category_code"); cc != "" {
		id, err := a.svc.CategoryUUIDByCode(ctx, orgID, cc)
		if err != nil {
			return rowError(err)
		}
		in.CategoryUUID = &id
	}
	mv, err := a.svc.RecordMovement(ctx, orgID, account, in)
	if err != nil {
		return rowError(err)
	}
	return ioengine.RowResult{OK: true, EntityType: "cash_movement", EntityUUID: mv.UUID.String(), Op: "create"}, nil
}

// RevertRow reverses an imported movement (movements are append-only).
func (a *MovementsIO) RevertRow(ctx context.Context, entityType, entityUUID string, _ map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil || entityType != "cash_movement" {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	_, err = a.svc.ReverseMovement(ctx, orgID, id, model.ReverseInput{Reason: "import rollback"})
	return err
}

// ---------------------------------------------------------------- expenses

// ExpensesIO exports and imports expenses.
type ExpensesIO struct{ svc *usecase.Service }

// NewExpensesIO builds the adapter.
func NewExpensesIO(svc *usecase.Service) *ExpensesIO { return &ExpensesIO{svc: svc} }

func (a *ExpensesIO) Resource() string { return model.ResourceExpenses }

func (a *ExpensesIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "expenses." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("expense_date"), col("category_code"), col("category"), col("description"), col("supplier_name"), col("document_no"),
		col("net_amount"), col("vat_rate"), col("vat_amount"), col("total_amount"), col("currency"), col("branch_code"), col("plate"),
		col("cash_account_code"), col("status"), col("has_attachment"),
		{Key: "uuid", LabelKey: "expenses.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *ExpensesIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := ExpenseFilter(query)
	f.Limit = usecase.MaxListTargets
	items, _, err := a.svc.ListExpenses(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		branch, plate, cash := "", "", ""
		if it.Branch != nil {
			branch = it.Branch.Code
		}
		if it.Vehicle != nil {
			plate = it.Vehicle.Code
		}
		if it.CashAccount != nil {
			cash = it.CashAccount.Code
		}
		attachment := i18n.Translate(locale, "finance.no")
		if it.Attachment != nil {
			attachment = i18n.Translate(locale, "finance.yes")
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "expense_date": it.ExpenseDate, "category_code": it.Category.Code, "category": it.Category.Name,
			"description": it.Description, "supplier_name": it.SupplierName, "document_no": it.DocumentNo,
			"net_amount": it.NetAmount.String(), "vat_rate": it.VATRate.String(), "vat_amount": it.VATAmount.String(),
			"total_amount": it.TotalAmount.String(), "currency": it.Currency, "branch_code": branch, "plate": plate,
			"cash_account_code": cash, "status": label(locale, "finance.expense_status.", it.Status), "has_attachment": attachment,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceExpenses, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *ExpensesIO) ImportSchema() []ioengine.ImportField {
	f := func(key string, required bool, hint string) ioengine.ImportField {
		return ioengine.ImportField{Key: key, LabelKey: "expenses." + key, Type: ioengine.ColumnTypeString, Required: required, DefaultHint: hint}
	}
	return []ioengine.ImportField{
		f("expense_date", false, ""), f("category_code", true, "other"), f("total_amount", false, ""), f("net_amount", false, ""),
		f("vat_rate", false, "20"), f("description", false, ""), f("supplier_name", false, ""), f("document_no", false, ""),
		f("branch_code", false, ""), f("plate", false, ""), f("cash_account_code", false, ""),
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *ExpensesIO) SampleRows() []map[string]any {
	return []map[string]any{
		{"expense_date": "2026-03-01", "category_code": "fuel", "total_amount": "1800", "vat_rate": "20", "description": "Yakıt alımı",
			"supplier_name": "Opet", "document_no": "F-001", "branch_code": "MRK", "plate": "34 ABC 123"},
		{"expense_date": "2026-03-04", "category_code": "cleaning", "net_amount": "500", "vat_rate": "20", "description": "İç temizlik",
			"branch_code": "MRK", "cash_account_code": "KASA-MRK"},
	}
}

func (a *ExpensesIO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	cat, err := a.svc.CategoryUUIDByCode(ctx, orgID, get(row, defaults, "category_code"))
	if err != nil {
		return ioengine.RowResult{OK: false, Error: "unknown category_code"}, nil
	}
	in := model.ExpenseInput{
		CategoryUUID: &cat, ExpenseDate: strp(get(row, defaults, "expense_date")), Description: strp(get(row, defaults, "description")),
		SupplierName: strp(get(row, defaults, "supplier_name")), DocumentNo: strp(get(row, defaults, "document_no")),
	}
	for key, dst := range map[string]**money.Money{"total_amount": &in.TotalAmount, "net_amount": &in.NetAmount} {
		if raw := get(row, defaults, key); raw != "" {
			v, err := money.Parse(raw)
			if err != nil {
				return ioengine.RowResult{OK: false, Error: key + " is not an amount"}, nil
			}
			*dst = &v
		}
	}
	if raw := get(row, defaults, "vat_rate"); raw != "" {
		r, err := money.ParseRate(raw)
		if err != nil {
			return ioengine.RowResult{OK: false, Error: "vat_rate is not a percentage"}, nil
		}
		in.VATRate = &r
	}
	if bc := get(row, defaults, "branch_code"); bc != "" {
		id, err := a.svc.BranchUUIDByCode(ctx, orgID, bc)
		if err != nil {
			return rowError(err)
		}
		in.BranchUUID = &id
	}
	if plate := get(row, defaults, "plate"); plate != "" {
		id, err := a.svc.VehicleUUIDByPlate(ctx, orgID, plate)
		if err != nil {
			return rowError(err)
		}
		in.VehicleUUID = &id
	}
	if cc := get(row, defaults, "cash_account_code"); cc != "" {
		id, err := a.svc.CashAccountUUIDByCode(ctx, orgID, cc)
		if err != nil {
			return ioengine.RowResult{OK: false, Error: "unknown cash_account_code"}, nil
		}
		in.CashAccountUUID = &id
	}
	e, err := a.svc.CreateExpense(ctx, orgID, in)
	if err != nil {
		return rowError(err)
	}
	return ioengine.RowResult{OK: true, EntityType: "expense", EntityUUID: e.UUID.String(), Op: "create"}, nil
}

// RevertRow voids an imported expense (its payment is reversed).
func (a *ExpensesIO) RevertRow(ctx context.Context, entityType, entityUUID string, _ map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil || entityType != "expense" {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	_, err = a.svc.VoidExpense(ctx, orgID, id, "import rollback")
	return err
}

// -------------------------------------------------------------------- bulk

// categoryBulk is the "set category" bulk action of movements / expenses.
type categoryBulk struct {
	svc        *usecase.Service
	resource   string
	permission string
	list       func(ctx context.Context, orgID int64, q map[string]string, ids []string) ([]string, error)
	apply      func(ctx context.Context, orgID int64, id uuid.UUID, category uuid.UUID) (*uuid.UUID, error)
	revert     func(ctx context.Context, orgID int64, id uuid.UUID, category *uuid.UUID) error
}

// NewMovementsBulk builds the movements bulk adapter (svc may be nil for
// metadata only).
func NewMovementsBulk(svc *usecase.Service) bulkengine.BulkAdapter {
	b := &categoryBulk{svc: svc, resource: model.ResourceMovements, permission: rbac.PermTenantFinanceCashBulkSetCategory}
	b.list = func(ctx context.Context, orgID int64, q map[string]string, ids []string) ([]string, error) {
		f := MovementFilter(q)
		f.Limit = MaxBulkTargets
		items, _, err := svc.ListMovements(ctx, orgID, f)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, it.UUID.String())
		}
		return out, nil
	}
	b.apply = func(ctx context.Context, orgID int64, id, category uuid.UUID) (*uuid.UUID, error) {
		return svc.SetMovementCategory(ctx, orgID, id, &category)
	}
	b.revert = func(ctx context.Context, orgID int64, id uuid.UUID, category *uuid.UUID) error {
		_, err := svc.SetMovementCategory(ctx, orgID, id, category)
		return err
	}
	return b
}

// NewExpensesBulk builds the expenses bulk adapter (svc may be nil for
// metadata only).
func NewExpensesBulk(svc *usecase.Service) bulkengine.BulkAdapter {
	b := &categoryBulk{svc: svc, resource: model.ResourceExpenses, permission: rbac.PermTenantFinanceExpensesBulkSetCategory}
	b.list = func(ctx context.Context, orgID int64, q map[string]string, ids []string) ([]string, error) {
		f := ExpenseFilter(q)
		f.Limit = MaxBulkTargets
		items, _, err := svc.ListExpenses(ctx, orgID, f)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(items))
		for _, it := range items {
			if it.Status == model.ExpenseRecorded {
				out = append(out, it.UUID.String())
			}
		}
		return out, nil
	}
	b.apply = func(ctx context.Context, orgID int64, id, category uuid.UUID) (*uuid.UUID, error) {
		prev, err := svc.SetExpenseCategory(ctx, orgID, id, category)
		return &prev, err
	}
	b.revert = func(ctx context.Context, orgID int64, id uuid.UUID, category *uuid.UUID) error {
		if category == nil {
			return nil
		}
		_, err := svc.SetExpenseCategory(ctx, orgID, id, *category)
		return err
	}
	return b
}

func (b *categoryBulk) Resource() string { return b.resource }

func (b *categoryBulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{{
		ID: "set_category", LabelKey: "bulk.actions." + strings.TrimPrefix(b.resource, "tenant.") + ".set_category",
		Permission: b.permission, Reversible: true,
	}}
}

func (b *categoryBulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	if action != "set_category" {
		return nil, fmt.Errorf("unknown action")
	}
	// The category is resolved inside the organization before any item
	// runs (another organization's category answers 400).
	raw := strings.TrimSpace(target.Params["category_uuid"])
	cat, err := uuid.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("params.category_uuid is required")
	}
	if !b.svc.CategoryActive(ctx, orgID, cat) {
		return nil, fmt.Errorf("unknown category")
	}
	switch target.Scope {
	case "ids":
		seen := map[string]bool{}
		out := make([]string, 0, len(target.IDs))
		for _, rawID := range target.IDs {
			id, err := uuid.Parse(strings.TrimSpace(rawID))
			if err != nil || seen[id.String()] {
				continue
			}
			seen[id.String()] = true
			out = append(out, id.String())
		}
		if len(out) > MaxBulkTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", MaxBulkTargets)
		}
		return out, nil
	case "query":
		return b.list(ctx, orgID, target.Query, nil)
	}
	return nil, fmt.Errorf("invalid target scope")
}

func (b *categoryBulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "invalid uuid"}, nil
	}
	cat, err := uuid.Parse(bulkengine.Params(ctx)["category_uuid"])
	if err != nil || action != "set_category" {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "invalid action"}, nil
	}
	prev, err := b.apply(ctx, orgID, id, cat)
	if err != nil {
		var ve *usecase.ValidationError
		var se *usecase.StateError
		if errors.Is(err, usecase.ErrMovementNotFound) || errors.Is(err, usecase.ErrExpenseNotFound) || errors.As(err, &ve) || errors.As(err, &se) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	previous := map[string]any{"category_uuid": ""}
	if prev != nil {
		previous["category_uuid"] = prev.String()
	}
	return bulkengine.BulkItemResult{EntityUUID: entityUUID, EntityType: strings.TrimPrefix(b.resource, "tenant."), OK: true, Op: "update", Previous: previous}, nil
}

func (b *categoryBulk) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	var cat *uuid.UUID
	if raw := cell(previous, "category_uuid"); raw != "" {
		v, err := uuid.Parse(raw)
		if err != nil {
			return err
		}
		cat = &v
	}
	return b.revert(ctx, orgID, id, cat)
}

var (
	_ ioengine.ResourceAdapter = (*AccountsIO)(nil)
	_ ioengine.ResourceAdapter = (*EntriesIO)(nil)
	_ ioengine.ResourceAdapter = (*CashAccountsIO)(nil)
	_ ioengine.SampleProvider  = (*CashAccountsIO)(nil)
	_ ioengine.ResourceAdapter = (*MovementsIO)(nil)
	_ ioengine.SampleProvider  = (*MovementsIO)(nil)
	_ ioengine.ResourceAdapter = (*ExpensesIO)(nil)
	_ ioengine.SampleProvider  = (*ExpensesIO)(nil)
	_ bulkengine.BulkAdapter   = (*categoryBulk)(nil)
)
