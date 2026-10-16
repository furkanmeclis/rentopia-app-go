package usecase

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ExpenseSortColumns are the sortable expense columns.
var ExpenseSortColumns = map[string]string{"expense_date": "expense_date", "total_amount": "total_amount", "created_at": "created_at"}

// MaxAttachmentBytes bounds a receipt / invoice file.
const MaxAttachmentBytes = 10 << 20

var attachmentTypes = map[string]string{
	"application/pdf": "pdf", "image/jpeg": "jpg", "image/png": "png", "image/webp": "webp",
}

var categoryCodeRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{1,31}$`)

// ------------------------------------------------------------ categories

// SeedCategories inserts the default expense categories of an organization
// (idempotent; the organization hook and the first use call it).
func SeedCategories(ctx context.Context, q *db.Queries, orgID int64) error {
	for _, c := range model.DefaultCategories {
		if _, err := q.InsertExpenseCategory(ctx, db.InsertExpenseCategoryParams{
			OrganizationID: orgID, Code: c.Code, Name: c.Name, IsSystem: true, SortOrder: c.Sort,
		}); err != nil && !notFound(err) {
			return err
		}
	}
	return nil
}

// ListCategories lists the organization's expense categories (the
// defaults are seeded on first use when missing).
func (s *Service) ListCategories(ctx context.Context, orgID int64, q string, activeOnly bool) ([]model.Category, error) {
	rows, err := s.q.ListExpenseCategories(ctx, db.ListExpenseCategoriesParams{OrganizationID: orgID, ActiveOnly: activeOnly, Q: textOf(q)})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 && q == "" {
		if err := SeedCategories(ctx, s.q, orgID); err != nil {
			return nil, err
		}
		rows, err = s.q.ListExpenseCategories(ctx, db.ListExpenseCategoriesParams{OrganizationID: orgID, ActiveOnly: activeOnly})
		if err != nil {
			return nil, err
		}
	}
	out := make([]model.Category, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.Category{
			UUID: r.Uuid, Code: r.Code, Name: r.Name, IsSystem: r.IsSystem, IsActive: r.IsActive, SortOrder: r.SortOrder,
			ExpenseCount: r.ExpenseCount, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		})
	}
	return out, nil
}

func (s *Service) category(ctx context.Context, orgID int64, id uuid.UUID) (model.Category, error) {
	items, err := s.ListCategories(ctx, orgID, "", false)
	if err != nil {
		return model.Category{}, err
	}
	for _, c := range items {
		if c.UUID == id {
			return c, nil
		}
	}
	return model.Category{}, ErrCategoryNotFound
}

// CreateCategory adds an organization category (tenant.finance.expenses.write).
func (s *Service) CreateCategory(ctx context.Context, orgID int64, in model.CategoryInput) (model.Category, error) {
	ve := &ValidationError{}
	code := strings.ToLower(strings.TrimSpace(deref(in.Code)))
	if !categoryCodeRE.MatchString(code) {
		ve.add("code", "invalid", "code is required (a-z, 0-9, _; 2-32 characters)")
	}
	name := strings.TrimSpace(deref(in.Name))
	if name == "" || utf8.RuneCountInString(name) > 128 {
		ve.add("name", "required", "name is required (at most 128 characters)")
	}
	if err := ve.orNil(); err != nil {
		return model.Category{}, err
	}
	sort := int32(100)
	if in.SortOrder != nil {
		sort = *in.SortOrder
	}
	row, err := s.q.InsertExpenseCategory(ctx, db.InsertExpenseCategoryParams{OrganizationID: orgID, Code: code, Name: name, SortOrder: sort})
	if notFound(err) {
		return model.Category{}, ErrCodeTaken
	}
	if err != nil {
		return model.Category{}, err
	}
	if in.IsActive != nil && !*in.IsActive {
		if _, err := s.q.UpdateExpenseCategory(ctx, db.UpdateExpenseCategoryParams{OrganizationID: orgID, ID: row.ID, Name: name, SortOrder: sort}); err != nil {
			return model.Category{}, err
		}
	}
	s.publish(ctx, model.ResourceCategories, "created", row.Uuid)
	return s.category(ctx, orgID, row.Uuid)
}

// PatchCategory renames / (de)activates / reorders a category; the code is
// fixed (imports match it).
func (s *Service) PatchCategory(ctx context.Context, orgID int64, id uuid.UUID, in model.CategoryInput) (model.Category, error) {
	c, err := s.q.GetExpenseCategory(ctx, db.GetExpenseCategoryParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Category{}, ErrCategoryNotFound
	}
	if err != nil {
		return model.Category{}, err
	}
	p := db.UpdateExpenseCategoryParams{OrganizationID: orgID, ID: c.ID, Name: c.Name, IsActive: c.IsActive, SortOrder: c.SortOrder}
	if in.Code != nil && strings.ToLower(strings.TrimSpace(*in.Code)) != c.Code {
		return model.Category{}, validation("code", "immutable", "the code of a category cannot change")
	}
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
		if p.Name == "" || utf8.RuneCountInString(p.Name) > 128 {
			return model.Category{}, validation("name", "required", "name is required (at most 128 characters)")
		}
	}
	if in.IsActive != nil {
		p.IsActive = *in.IsActive
	}
	if in.SortOrder != nil {
		p.SortOrder = *in.SortOrder
	}
	if _, err := s.q.UpdateExpenseCategory(ctx, p); err != nil {
		return model.Category{}, err
	}
	s.publish(ctx, model.ResourceCategories, "updated", id)
	return s.category(ctx, orgID, id)
}

// activeCategory resolves a category for a new / changed expense.
func (s *Service) activeCategory(ctx context.Context, orgID int64, field string, id *uuid.UUID) (db.GetExpenseCategoryRow, error) {
	if id == nil {
		return db.GetExpenseCategoryRow{}, validation(field, "required", "category_uuid is required")
	}
	c, err := s.q.GetExpenseCategory(ctx, db.GetExpenseCategoryParams{OrganizationID: orgID, Uuid: *id})
	if notFound(err) {
		return c, validation(field, "not_found", "unknown category")
	}
	if err != nil {
		return c, err
	}
	if !c.IsActive {
		return c, validation(field, "inactive", "the category is not active")
	}
	return c, nil
}

// -------------------------------------------------------------- expenses

// ListExpenses lists expenses (branch scope: expenses of the caller's
// branches; head office expenses need every branch).
func (s *Service) ListExpenses(ctx context.Context, orgID int64, f model.ExpenseFilter) ([]model.Expense, int64, error) {
	p := db.ListExpensesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Ids: nilIDs(f.IDs), Statuses: nilIfEmpty(f.Statuses),
		DateFrom: dateNarg(f.From), DateTo: dateNarg(f.To), Q: textOf(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if _, ok := ExpenseSortColumns[p.SortField]; !ok {
		p.SortField, p.SortDesc = "expense_date", true
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
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
	for _, id := range f.VehicleUUIDs {
		v, err := s.q.GetVehicleRefByUUID(ctx, db.GetVehicleRefByUUIDParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil, 0, validation("vehicle", "not_found", "unknown vehicle")
		}
		if err != nil {
			return nil, 0, err
		}
		p.VehicleIds = append(p.VehicleIds, v.ID)
	}
	branches, err := s.branchIDs(ctx, orgID, "branch", f.BranchUUIDs)
	if err != nil {
		return nil, 0, err
	}
	p.FilterBranchIds = nilIDs(branches)
	rows, err := s.q.ListExpenses(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Expense, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		e := model.Expense{
			UUID: r.Uuid, Category: model.Ref{UUID: r.CategoryUuid, Code: r.CategoryCode, Name: r.CategoryName},
			ExpenseDate: dateString(r.ExpenseDate), Description: r.Description, SupplierName: r.SupplierName, DocumentNo: r.DocumentNo,
			Currency: r.Currency, NetAmount: m(r.NetAmount), VATRate: money.Rate(m(r.VatRate).Minor()), VATAmount: m(r.VatAmount),
			TotalAmount: m(r.TotalAmount), Branch: refOf(r.BranchUuid, r.BranchCode, r.BranchName),
			Vehicle: refOf(r.VehicleUuid, r.VehiclePlate, pgtype.Text{}), CashAccount: refOf(r.CashAccountUuid, r.CashAccountCode, r.CashAccountName),
			MovementUUID: uuidPtr(r.MovementUuid), Status: r.Status, VoidReason: r.VoidReason, VoidedAt: timePtr(r.VoidedAt),
			CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail),
			CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if r.HasAttachment {
			e.Attachment = &model.Attachment{
				Name: r.AttachmentName, ContentType: r.AttachmentContentType, SizeBytes: r.AttachmentSizeBytes,
				URL: "/v1/tenant/finance/expenses/" + r.Uuid.String() + "/attachment",
			}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// expense loads one expense the caller may see.
func (s *Service) expense(ctx context.Context, orgID int64, id uuid.UUID) (db.GetExpenseRowRow, model.Expense, error) {
	row, err := s.q.GetExpenseRow(ctx, db.GetExpenseRowParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, model.Expense{}, ErrExpenseNotFound
	}
	if err != nil {
		return row, model.Expense{}, err
	}
	items, _, err := s.ListExpenses(ctx, orgID, model.ExpenseFilter{IDs: []int64{row.ID}, Limit: 1})
	if err != nil {
		return row, model.Expense{}, err
	}
	if len(items) == 0 {
		return row, model.Expense{}, ErrExpenseNotFound
	}
	return row, items[0], nil
}

// GetExpense returns one expense.
func (s *Service) GetExpense(ctx context.Context, orgID int64, id uuid.UUID) (model.Expense, error) {
	_, e, err := s.expense(ctx, orgID, id)
	return e, err
}

// amounts derives net / VAT / total from either the net or the total.
func amounts(ve *ValidationError, net, total *money.Money, rate money.Rate) (money.Money, money.Money, money.Money) {
	if rate < 0 || rate > money.Percent(100) {
		ve.add("vat_rate", "out_of_range", "vat_rate must be between 0 and 100")
		return money.Zero(), money.Zero(), money.Zero()
	}
	switch {
	case net != nil && net.IsPositive():
		vat := net.MulRate(rate)
		return *net, vat, net.Add(vat)
	case total != nil && total.IsPositive():
		n := total.Ratio(money.RateScale, money.RateScale+int64(rate))
		return n, total.Sub(n), *total
	}
	ve.add("net_amount", "invalid", "net_amount or total_amount must be positive")
	return money.Zero(), money.Zero(), money.Zero()
}

// expenseRefs resolves the branch and vehicle of an expense in the
// caller's scope (a vehicle without a branch books on its home branch).
func (s *Service) expenseRefs(ctx context.Context, orgID int64, branchID *uuid.UUID, vehicleID *uuid.UUID, keepBranch pgtype.Int8) (pgtype.Int8, pgtype.Int8, error) {
	var vehicle pgtype.Int8
	def := keepBranch.Int64
	if vehicleID != nil {
		v, err := s.q.GetVehicleRefByUUID(ctx, db.GetVehicleRefByUUIDParams{OrganizationID: orgID, Uuid: *vehicleID})
		if notFound(err) {
			return pgtype.Int8{}, pgtype.Int8{}, validation("vehicle_uuid", "not_found", "unknown vehicle")
		}
		if err != nil {
			return pgtype.Int8{}, pgtype.Int8{}, err
		}
		vehicle = int8Of(v.ID)
		if branchID == nil && def == 0 {
			def = v.HomeBranchID
			if orgctx.CheckBranch(ctx, def) != nil {
				def = v.CurrentBranchID
			}
		}
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", branchID, def)
	if err != nil {
		return pgtype.Int8{}, pgtype.Int8{}, err
	}
	return int8Of(branch), vehicle, nil
}

// CreateExpense records an expense; with a cash account the payment leaves
// it in the same transaction (movement kind expense).
func (s *Service) CreateExpense(ctx context.Context, orgID int64, in model.ExpenseInput) (model.Expense, error) {
	ve := &ValidationError{}
	date := optionalDate(ve, "expense_date", in.ExpenseDate, s.today())
	currency := strings.ToUpper(strings.TrimSpace(deref(in.Currency)))
	if currency == "" {
		currency = string(money.TRY)
	}
	if !currencyRE.MatchString(currency) {
		ve.add("currency", "invalid", "currency must be an ISO 4217 code")
	}
	rate := money.Rate(0)
	if in.VATRate != nil {
		rate = *in.VATRate
	}
	net, vat, total := amounts(ve, in.NetAmount, in.TotalAmount, rate)
	desc := strings.TrimSpace(deref(in.Description))
	if utf8.RuneCountInString(desc) > 1000 {
		ve.add("description", "too_long", "description is at most 1000 characters")
	}
	supplier := strings.TrimSpace(deref(in.SupplierName))
	docNo := strings.TrimSpace(deref(in.DocumentNo))
	if utf8.RuneCountInString(supplier) > 255 || utf8.RuneCountInString(docNo) > 64 {
		ve.add("supplier_name", "too_long", "supplier_name is at most 255 and document_no 64 characters")
	}
	if err := ve.orNil(); err != nil {
		return model.Expense{}, err
	}
	cat, err := s.activeCategory(ctx, orgID, "category_uuid", in.CategoryUUID)
	if err != nil {
		return model.Expense{}, err
	}
	branch, vehicle, err := s.expenseRefs(ctx, orgID, in.BranchUUID, in.VehicleUUID, pgtype.Int8{})
	if err != nil {
		return model.Expense{}, err
	}
	var id uuid.UUID
	var cashUUID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		var cash pgtype.Int8
		var locked []lockedAccount
		if in.CashAccountUUID != nil {
			locked, err = s.lockForMovement(ctx, q, orgID, cashTarget{"cash_account_uuid", *in.CashAccountUUID})
			if err != nil {
				return err
			}
			if locked[0].row.Currency != currency {
				return validation("cash_account_uuid", "currency_mismatch", "the cash account holds another currency")
			}
			if err := checkFunds("cash_account_uuid", locked[0], total); err != nil {
				return err
			}
			cash = int8Of(locked[0].row.ID)
			cashUUID = locked[0].row.Uuid
		}
		row, err := q.InsertExpense(ctx, db.InsertExpenseParams{
			OrganizationID: orgID, CategoryID: cat.ID, ExpenseDate: dateOf(date), Description: desc, SupplierName: supplier,
			DocumentNo: docNo, Currency: currency, NetAmount: net.Numeric(), VatRate: money.New(int64(rate)).Numeric(),
			VatAmount: vat.Numeric(), TotalAmount: total.Numeric(), BranchID: branch, VehicleID: vehicle, CashAccountID: cash,
			CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		id = row.Uuid
		if len(locked) == 1 {
			text := desc
			if text == "" {
				text = cat.Name
			}
			mv, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
				OrganizationID: orgID, CashAccountID: locked[0].row.ID, Kind: model.MoveExpense, Direction: model.DirOut,
				Amount: total.Numeric(), Currency: currency, MovementDate: dateOf(date), Description: text,
				BranchID: locked[0].row.BranchID, CategoryID: int8Of(cat.ID), ExpenseID: int8Of(row.ID), CreatedBy: actorID(ctx),
			})
			if err != nil {
				return err
			}
			s.publish(ctx, model.ResourceMovements, "created", mv.Uuid)
		}
		return nil
	})
	if err != nil {
		return model.Expense{}, err
	}
	s.publish(ctx, model.ResourceExpenses, "created", id)
	if cashUUID != uuid.Nil {
		s.publish(ctx, model.ResourceCashAccounts, "updated", cashUUID)
	}
	return s.GetExpense(ctx, orgID, id)
}

// PatchExpense changes the classification and description of an expense.
func (s *Service) PatchExpense(ctx context.Context, orgID int64, id uuid.UUID, in model.ExpenseInput) (model.Expense, error) {
	row, view, err := s.expense(ctx, orgID, id)
	if err != nil {
		return model.Expense{}, err
	}
	if row.Status != model.ExpenseRecorded {
		return model.Expense{}, stateErr("expense_void", "a void expense cannot change")
	}
	ve := &ValidationError{}
	if in.NetAmount != nil || in.TotalAmount != nil || in.VATRate != nil || in.Currency != nil || in.CashAccountUUID != nil {
		ve.add("total_amount", "immutable", "amounts and the paying account are fixed; void the expense and record it again")
	}
	p := db.UpdateExpenseDetailsParams{
		OrganizationID: orgID, ID: row.ID, CategoryID: row.CategoryID, ExpenseDate: row.ExpenseDate, Description: row.Description,
		SupplierName: row.SupplierName, DocumentNo: row.DocumentNo, BranchID: row.BranchID, VehicleID: row.VehicleID,
	}
	if in.ExpenseDate != nil {
		p.ExpenseDate = dateOf(optionalDate(ve, "expense_date", in.ExpenseDate, row.ExpenseDate.Time))
	}
	if in.Description != nil {
		p.Description = strings.TrimSpace(*in.Description)
	}
	if in.SupplierName != nil {
		p.SupplierName = strings.TrimSpace(*in.SupplierName)
	}
	if in.DocumentNo != nil {
		p.DocumentNo = strings.TrimSpace(*in.DocumentNo)
	}
	if err := ve.orNil(); err != nil {
		return model.Expense{}, err
	}
	if in.CategoryUUID != nil {
		cat, err := s.activeCategory(ctx, orgID, "category_uuid", in.CategoryUUID)
		if err != nil {
			return model.Expense{}, err
		}
		p.CategoryID = cat.ID
	}
	if in.BranchUUID != nil || in.ClearBranch || in.VehicleUUID != nil || in.ClearVehicle {
		branchArg := in.BranchUUID
		keep := row.BranchID
		if in.ClearBranch {
			keep = pgtype.Int8{}
		}
		vehicleArg := in.VehicleUUID
		branch, vehicle, err := s.expenseRefs(ctx, orgID, branchArg, vehicleArg, keep)
		if err != nil {
			return model.Expense{}, err
		}
		p.BranchID = branch
		if in.VehicleUUID != nil {
			p.VehicleID = vehicle
		}
		if in.ClearVehicle {
			p.VehicleID = pgtype.Int8{}
		}
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.UpdateExpenseDetails(ctx, p); err != nil {
			return err
		}
		if p.CategoryID != row.CategoryID && view.MovementUUID != nil {
			mv, err := q.GetExpensePayment(ctx, db.GetExpensePaymentParams{OrganizationID: orgID, ExpenseID: int8Of(row.ID)})
			if err == nil {
				_, err = q.SetCashMovementCategory(ctx, db.SetCashMovementCategoryParams{OrganizationID: orgID, ID: mv.ID, CategoryID: int8Of(p.CategoryID)})
			}
			if err != nil && !notFound(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Expense{}, err
	}
	s.publish(ctx, model.ResourceExpenses, "updated", id)
	return s.GetExpense(ctx, orgID, id)
}

// SetExpenseCategory is the bulk "set category" item; returns the previous
// category uuid. The payment movement follows the expense.
func (s *Service) SetExpenseCategory(ctx context.Context, orgID int64, id, category uuid.UUID) (uuid.UUID, error) {
	row, view, err := s.expense(ctx, orgID, id)
	if err != nil {
		return uuid.Nil, err
	}
	cat, err := s.activeCategory(ctx, orgID, "category_uuid", &category)
	if err != nil {
		return uuid.Nil, err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.SetExpenseCategory(ctx, db.SetExpenseCategoryParams{OrganizationID: orgID, ID: row.ID, CategoryID: cat.ID}); err != nil {
			return err
		}
		mv, err := q.GetExpensePayment(ctx, db.GetExpensePaymentParams{OrganizationID: orgID, ExpenseID: int8Of(row.ID)})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = q.SetCashMovementCategory(ctx, db.SetCashMovementCategoryParams{OrganizationID: orgID, ID: mv.ID, CategoryID: int8Of(cat.ID)})
		return err
	})
	if err != nil {
		return uuid.Nil, err
	}
	return view.Category.UUID, nil
}

// VoidExpense voids an expense with a reason; its payment movement is
// reversed in the same transaction.
func (s *Service) VoidExpense(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.Expense, error) {
	row, _, err := s.expense(ctx, orgID, id)
	if err != nil {
		return model.Expense{}, err
	}
	r, err := requireReason("reason", reason)
	if err != nil {
		return model.Expense{}, err
	}
	if row.Status != model.ExpenseRecorded {
		return model.Expense{}, stateErr("already_void", "the expense is already void")
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.VoidExpense(ctx, db.VoidExpenseParams{
			OrganizationID: orgID, ID: row.ID, VoidReason: r, VoidedBy: actorID(ctx), VoidedAt: pgtype.Timestamptz{Time: s.clock(), Valid: true},
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("already_void", "the expense is already void")
		}
		mv, err := q.GetExpensePayment(ctx, db.GetExpensePaymentParams{OrganizationID: orgID, ExpenseID: int8Of(row.ID)})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		full, err := q.GetCashMovementRow(ctx, db.GetCashMovementRowParams{OrganizationID: orgID, Uuid: mv.Uuid})
		if err != nil {
			return err
		}
		if full.Reversed {
			return nil
		}
		if _, err := q.LockCashAccounts(ctx, db.LockCashAccountsParams{OrganizationID: orgID, Ids: []int64{full.CashAccountID}}); err != nil {
			return err
		}
		rev, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
			OrganizationID: orgID, CashAccountID: full.CashAccountID, Kind: model.MoveReversal, Direction: model.DirIn,
			Amount: full.Amount, Currency: full.Currency, MovementDate: dateOf(s.today()), Description: full.Description,
			Reason: r, BranchID: full.BranchID, CategoryID: full.CategoryID, ExpenseID: int8Of(row.ID),
			ReversesMovementID: int8Of(full.ID), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		s.publish(ctx, model.ResourceMovements, "created", rev.Uuid)
		return nil
	})
	if err != nil {
		return model.Expense{}, err
	}
	s.publish(ctx, model.ResourceExpenses, "updated", id)
	return s.GetExpense(ctx, orgID, id)
}

// FileUpload is an uploaded attachment.
type FileUpload struct {
	Body     io.Reader
	Size     int64
	Filename string
}

// AttachmentFile is an open attachment.
type AttachmentFile struct {
	Body        io.ReadCloser
	ContentType string
	Name        string
	Size        int64
}

// SniffAttachment returns the type of an accepted receipt / invoice file
// from its first bytes: PDF, JPEG, PNG or WebP.
func SniffAttachment(head []byte) (string, string, bool) {
	mime := strings.ToLower(http.DetectContentType(head))
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	ext, ok := attachmentTypes[mime]
	return mime, ext, ok
}

func cleanFilename(name, ext string) string {
	name = strings.TrimSpace(path.Base(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "receipt." + ext
	}
	for utf8.RuneCountInString(name) > 255 {
		r := []rune(name)
		name = string(r[len(r)-255:])
	}
	return name
}

// UploadAttachment stores (or replaces) the receipt / invoice of an
// expense in private storage under the organization prefix.
func (s *Service) UploadAttachment(ctx context.Context, orgID int64, id uuid.UUID, in FileUpload) (model.Expense, error) {
	if s.storage == nil {
		return model.Expense{}, ErrStorageUnavailable
	}
	row, _, err := s.expense(ctx, orgID, id)
	if err != nil {
		return model.Expense{}, err
	}
	if in.Size <= 0 || in.Body == nil {
		return model.Expense{}, validation("file", "required", "file is required")
	}
	if in.Size > MaxAttachmentBytes {
		return model.Expense{}, validation("file", "too_large", "file must be at most 10 MiB")
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.Body, head)
	head = head[:n]
	mime, ext, ok := SniffAttachment(head)
	if !ok {
		return model.Expense{}, validation("file", "invalid_type", "file must be a PDF, JPEG, PNG or WebP")
	}
	orgUUID, err := s.q.GetOrganizationUUIDForFinance(ctx, orgID)
	if err != nil {
		return model.Expense{}, err
	}
	key := storage.ExpenseAttachmentObjectKey(orgUUID.String(), id.String(), uuid.NewString(), ext)
	name := cleanFilename(in.Filename, ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime, Filename: name,
	}, key); err != nil {
		return model.Expense{}, err
	}
	if _, err := s.q.SetExpenseAttachment(ctx, db.SetExpenseAttachmentParams{
		OrganizationID: orgID, ID: row.ID, AttachmentObjectKey: pgtype.Text{String: key, Valid: true}, AttachmentName: name,
		AttachmentContentType: mime, AttachmentSizeBytes: in.Size,
	}); err != nil {
		_ = s.storage.Delete(ctx, key)
		return model.Expense{}, err
	}
	if row.AttachmentObjectKey.Valid && row.AttachmentObjectKey.String != key {
		_ = s.storage.Delete(ctx, row.AttachmentObjectKey.String)
	}
	s.publish(ctx, model.ResourceExpenses, "updated", id)
	return s.GetExpense(ctx, orgID, id)
}

// OpenAttachment opens the attachment for streaming (only keys under the
// organization's prefix are served).
func (s *Service) OpenAttachment(ctx context.Context, orgID int64, id uuid.UUID) (AttachmentFile, error) {
	if s.storage == nil {
		return AttachmentFile{}, ErrStorageUnavailable
	}
	row, _, err := s.expense(ctx, orgID, id)
	if err != nil {
		return AttachmentFile{}, err
	}
	if !row.AttachmentObjectKey.Valid {
		return AttachmentFile{}, ErrNoAttachment
	}
	orgUUID, err := s.q.GetOrganizationUUIDForFinance(ctx, orgID)
	if err != nil {
		return AttachmentFile{}, err
	}
	if !storage.OrganizationOwnsKey(row.AttachmentObjectKey.String, orgUUID.String()) {
		s.log.Error("expense_attachment_outside_organization", "expense", id.String())
		return AttachmentFile{}, ErrNoAttachment
	}
	rc, size, err := s.storage.Download(ctx, row.AttachmentObjectKey.String)
	if err != nil {
		s.log.Warn("expense_attachment_missing", "expense", id.String(), "error", err)
		return AttachmentFile{}, ErrNoAttachment
	}
	if size <= 0 {
		size = row.AttachmentSizeBytes
	}
	return AttachmentFile{Body: rc, ContentType: row.AttachmentContentType, Name: row.AttachmentName, Size: size}, nil
}

// DeleteAttachment removes the attachment of an expense.
func (s *Service) DeleteAttachment(ctx context.Context, orgID int64, id uuid.UUID) (model.Expense, error) {
	row, _, err := s.expense(ctx, orgID, id)
	if err != nil {
		return model.Expense{}, err
	}
	if !row.AttachmentObjectKey.Valid {
		return model.Expense{}, ErrNoAttachment
	}
	if _, err := s.q.SetExpenseAttachment(ctx, db.SetExpenseAttachmentParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return model.Expense{}, err
	}
	if s.storage != nil {
		_ = s.storage.Delete(ctx, row.AttachmentObjectKey.String)
	}
	s.publish(ctx, model.ResourceExpenses, "updated", id)
	return s.GetExpense(ctx, orgID, id)
}

// ---------------------------------------------------------------- reports

// ExpenseReport groups recorded expenses (category, branch, vehicle,
// month) in a period.
func (s *Service) ExpenseReport(ctx context.Context, orgID int64, groupBy string, from, to *time.Time) (model.Report, error) {
	if !contains([]string{"category", "branch", "vehicle", "month"}, groupBy) {
		return model.Report{}, validation("group_by", "invalid", "group_by must be category, branch, vehicle or month")
	}
	f, t := s.period(from, to)
	rows, err := s.q.ExpenseReport(ctx, db.ExpenseReportParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), GroupBy: groupBy, DateFrom: dateOf(f), DateTo: dateOf(t),
	})
	if err != nil {
		return model.Report{}, err
	}
	out := model.Report{GroupBy: groupBy, From: f.Format("2006-01-02"), To: t.Format("2006-01-02"), Rows: []model.ReportRow{},
		Totals: map[string]money.Money{"net_amount": money.Zero(), "vat_amount": money.Zero(), "total_amount": money.Zero()}}
	for _, r := range rows {
		vals := map[string]money.Money{"net_amount": m(r.NetAmount), "vat_amount": m(r.VatAmount), "total_amount": m(r.TotalAmount)}
		out.Rows = append(out.Rows, model.ReportRow{Key: r.GroupKey, Label: r.GroupLabel, Currency: r.Currency, Count: r.ExpenseCount, Values: vals})
		for k, v := range vals {
			out.Totals[k] = out.Totals[k].Add(v)
		}
	}
	return out, nil
}

// CashFlowReport groups movements (account, branch, month) in a period.
func (s *Service) CashFlowReport(ctx context.Context, orgID int64, groupBy string, from, to *time.Time) (model.Report, error) {
	if !contains([]string{"account", "branch", "month"}, groupBy) {
		return model.Report{}, validation("group_by", "invalid", "group_by must be account, branch or month")
	}
	f, t := s.period(from, to)
	rows, err := s.q.CashFlowReport(ctx, db.CashFlowReportParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), GroupBy: groupBy, DateFrom: dateOf(f), DateTo: dateOf(t),
	})
	if err != nil {
		return model.Report{}, err
	}
	keys := []string{"total_in", "total_out", "collections", "expenses", "refunds", "supplier_payments"}
	out := model.Report{GroupBy: groupBy, From: f.Format("2006-01-02"), To: t.Format("2006-01-02"), Rows: []model.ReportRow{}, Totals: map[string]money.Money{}}
	for _, k := range keys {
		out.Totals[k] = money.Zero()
	}
	for _, r := range rows {
		vals := map[string]money.Money{
			"total_in": m(r.TotalIn), "total_out": m(r.TotalOut), "collections": m(r.Collections), "expenses": m(r.Expenses),
			"refunds": m(r.Refunds), "supplier_payments": m(r.SupplierPayments),
		}
		out.Rows = append(out.Rows, model.ReportRow{Key: r.GroupKey, Label: r.GroupLabel, Currency: r.Currency, Count: r.MovementCount, Values: vals})
		for k, v := range vals {
			out.Totals[k] = out.Totals[k].Add(v)
		}
	}
	return out, nil
}

// period defaults to the current month (Istanbul) and caps a year.
func (s *Service) period(from, to *time.Time) (time.Time, time.Time) {
	today := s.today()
	f := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	t := today
	if from != nil {
		f = day0(*from)
	}
	if to != nil {
		t = day0(*to)
	}
	if t.Before(f) {
		f, t = t, f
	}
	if t.Sub(f) > 366*24*time.Hour {
		f = t.AddDate(-1, 0, 0)
	}
	return f, t
}

// ------------------------------------------------------- organization hook

// OrganizationHooks seeds the default expense categories of a new
// organization inside its create transaction (organizations
// OrganizationHooks).
type OrganizationHooks struct{}

// OnCreated runs inside the organization-create transaction.
func (OrganizationHooks) OnCreated(ctx context.Context, tx pgx.Tx, org orgmodel.Organization, _ orgmodel.User) error {
	return SeedCategories(ctx, db.New(tx), org.ID)
}

// OnDeleted keeps the categories (organizations are soft-deleted; a hard
// delete cascades).
func (OrganizationHooks) OnDeleted(context.Context, orgmodel.Organization) error { return nil }
