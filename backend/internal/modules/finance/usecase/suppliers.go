package usecase

// Supplier invoices and payments (TEC-721, docs/design/suppliers.md): the
// payables side of the ledger.
//
//   - An invoice posts one payable accrual (credit) on the supplier's ledger
//     account under InvoiceKey (single accrual, RT-A23 trigger); syncInvoice
//     reconciles it like SyncRental does for rentals (void → reversal, a
//     moved due date → reversal + new accrual). The invoice is the source of
//     one expense row, so expense reports cover it.
//   - A payment writes one cash movement (supplier_payment, out), one
//     ledger entry (payment, debit) and its allocations to open invoices in
//     one transaction; voiding reverses both and releases the allocations.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrSupplierInvoiceNotFound = errors.New("supplier invoice not found")
	ErrSupplierPaymentNotFound = errors.New("supplier payment not found")
	// ErrInvoiceNoTaken: the supplier already has a live invoice with this
	// number (409 on invoice_no).
	ErrInvoiceNoTaken = errors.New("the supplier already has an invoice with this number")
)

// SupplierInvoiceSortColumns are the sortable invoice columns.
var SupplierInvoiceSortColumns = map[string]string{
	"invoice_date": "invoice_date", "due_date": "due_date", "total_amount": "total_amount",
	"open_amount": "open_amount", "created_at": "created_at",
}

// SupplierPaymentSortColumns are the sortable payment columns.
var SupplierPaymentSortColumns = map[string]string{"payment_date": "payment_date", "amount": "amount", "created_at": "created_at"}

// MaxInvoiceLines bounds the lines of one invoice.
const MaxInvoiceLines = 100

// rowVisible applies the branch scope to a row of an optional branch (head
// office rows need every branch).
func rowVisible(ctx context.Context, branch pgtype.Int8) bool {
	if branch.Valid {
		return orgctx.CheckBranch(ctx, branch.Int64) == nil
	}
	return orgctx.RequireAllBranches(ctx) == nil
}

// supplierFor resolves a supplier of the organization (unknown → a
// validation error on field). activeOnly refuses archived suppliers.
func (s *Service) supplierFor(ctx context.Context, q *db.Queries, orgID int64, field string, id *uuid.UUID, activeOnly bool) (db.GetSupplierForFinanceRow, error) {
	if id == nil {
		return db.GetSupplierForFinanceRow{}, validation(field, "required", field+" is required")
	}
	row, err := q.GetSupplierForFinance(ctx, db.GetSupplierForFinanceParams{OrganizationID: orgID, Uuid: *id})
	if notFound(err) {
		return row, validation(field, "not_found", "unknown supplier")
	}
	if err != nil {
		return row, err
	}
	if activeOnly && row.Status != model.AccountActive {
		return row, validation(field, "archived", "the supplier is archived")
	}
	return row, nil
}

// ensureSupplierAccount returns the ledger account of a supplier, opening
// it (at branch) on the first posting. Runs on q (the caller's transaction).
func (s *Service) ensureSupplierAccount(ctx context.Context, q *db.Queries, orgID int64, sup db.GetSupplierForFinanceRow, branchID int64, currency string) (account, error) {
	if currency == "" {
		currency = string(money.TRY)
	}
	if err := q.LockOrganizationAccountCodes(ctx, orgID); err != nil {
		return account{}, err
	}
	row, err := q.EnsureSupplierAccount(ctx, db.EnsureSupplierAccountParams{
		OrganizationID: orgID, SupplierID: int8Of(sup.ID), Name: sup.Title, BranchID: int8Of(branchID),
		Currency: currency, PaymentTermDays: sup.PaymentTermDays, CreatedBy: actorID(ctx),
	})
	if err != nil {
		return account{}, err
	}
	return account{ID: row.ID, UUID: row.Uuid, Currency: row.Currency, Term: row.PaymentTermDays}, nil
}

// SyncSupplierAccount keeps the account of a supplier in step with its card
// (title, terms); the suppliers module calls it after a change.
func (s *Service) SyncSupplierAccount(ctx context.Context, orgID, supplierID int64, title string, term int32) error {
	return s.q.SyncSupplierAccount(ctx, db.SyncSupplierAccountParams{
		OrganizationID: orgID, SupplierID: int8Of(supplierID), Name: title, PaymentTermDays: term,
	})
}

// --------------------------------------------------------------- invoices

func (s *Service) dueWindow(days int) (pgtype.Date, pgtype.Date) {
	if days <= 0 || days > 365 {
		days = model.DueSoonDays
	}
	today := s.today()
	return dateOf(today), dateOf(today.AddDate(0, 0, days))
}

// ListSupplierInvoices lists supplier invoices (branch scope: invoices of
// the caller's branches; head office invoices need every branch).
func (s *Service) ListSupplierInvoices(ctx context.Context, orgID int64, f model.SupplierInvoiceFilter) ([]model.SupplierInvoice, int64, error) {
	today, soon := s.dueWindow(f.WindowDays)
	p := db.ListSupplierInvoicesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Today: today, Soon: soon, Ids: nilIDs(f.IDs),
		Statuses: nilIfEmpty(f.Statuses), DueStates: nilIfEmpty(f.DueStates), DateFrom: dateNarg(f.From),
		DateTo: dateNarg(f.To), DueTo: dateNarg(f.DueTo), Q: textOf(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if _, ok := SupplierInvoiceSortColumns[p.SortField]; !ok {
		p.SortField, p.SortDesc = "invoice_date", true
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	for _, id := range f.SupplierUUIDs {
		sup, err := s.q.GetSupplierForFinance(ctx, db.GetSupplierForFinanceParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil, 0, validation("supplier", "not_found", "unknown supplier")
		}
		if err != nil {
			return nil, 0, err
		}
		p.SupplierIds = append(p.SupplierIds, sup.ID)
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
	rows, err := s.q.ListSupplierInvoices(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.SupplierInvoice, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		inv := model.SupplierInvoice{
			UUID: r.Uuid, InvoiceNo: r.InvoiceNo, InvoiceDate: dateString(r.InvoiceDate), DueDate: dateString(r.DueDate),
			Supplier: model.Ref{UUID: r.SupplierUuid, Code: r.SupplierCode, Name: r.SupplierTitle},
			Account:  model.Ref{UUID: r.AccountUuid, Code: r.AccountCode},
			Category: model.Ref{UUID: r.CategoryUuid, Code: r.CategoryCode, Name: r.CategoryName},
			Branch:   refOf(r.BranchUuid, r.BranchCode, r.BranchName), Vehicle: refOf(r.VehicleUuid, r.VehiclePlate, pgtype.Text{}),
			Description: r.Description, Currency: r.Currency, NetAmount: m(r.NetAmount), VATAmount: m(r.VatAmount),
			TotalAmount: m(r.TotalAmount), PaidAmount: m(r.PaidAmount), OpenAmount: m(r.OpenAmount), Status: r.Status,
			DueState: r.DueState, ExpenseUUID: uuidPtr(r.ExpenseUuid), VoidReason: r.VoidReason, VoidedAt: timePtr(r.VoidedAt),
			CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail),
			CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
			Lines: []model.InvoiceLine{}, Payments: []model.InvoicePayment{},
		}
		if r.HasAttachment {
			inv.Attachment = &model.Attachment{
				Name: r.AttachmentName, ContentType: r.AttachmentContentType, SizeBytes: r.AttachmentSizeBytes,
				URL: "/v1/tenant/finance/supplier-invoices/" + r.Uuid.String() + "/file",
			}
		}
		out = append(out, inv)
	}
	return out, total, nil
}

// invoice loads one invoice the caller may see.
func (s *Service) invoice(ctx context.Context, orgID int64, id uuid.UUID) (db.GetSupplierInvoiceRowRow, model.SupplierInvoice, error) {
	row, err := s.q.GetSupplierInvoiceRow(ctx, db.GetSupplierInvoiceRowParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, model.SupplierInvoice{}, ErrSupplierInvoiceNotFound
	}
	if err != nil {
		return row, model.SupplierInvoice{}, err
	}
	items, _, err := s.ListSupplierInvoices(ctx, orgID, model.SupplierInvoiceFilter{IDs: []int64{row.ID}, Limit: 1})
	if err != nil {
		return row, model.SupplierInvoice{}, err
	}
	if len(items) == 0 {
		return row, model.SupplierInvoice{}, ErrSupplierInvoiceNotFound
	}
	return row, items[0], nil
}

// GetSupplierInvoice returns one invoice with its lines and payments.
func (s *Service) GetSupplierInvoice(ctx context.Context, orgID int64, id uuid.UUID) (model.SupplierInvoice, error) {
	row, inv, err := s.invoice(ctx, orgID, id)
	if err != nil {
		return inv, err
	}
	lines, err := s.q.ListSupplierInvoiceLines(ctx, db.ListSupplierInvoiceLinesParams{OrganizationID: orgID, InvoiceID: row.ID})
	if err != nil {
		return inv, err
	}
	for _, l := range lines {
		inv.Lines = append(inv.Lines, model.InvoiceLine{
			LineNo: l.LineNo, Description: l.Description, NetAmount: m(l.NetAmount), VATRate: money.Rate(m(l.VatRate).Minor()),
			VATAmount: m(l.VatAmount), TotalAmount: m(l.TotalAmount),
		})
	}
	pays, err := s.q.ListInvoicePayments(ctx, db.ListInvoicePaymentsParams{OrganizationID: orgID, InvoiceID: row.ID})
	if err != nil {
		return inv, err
	}
	for _, p := range pays {
		inv.Payments = append(inv.Payments, model.InvoicePayment{
			PaymentUUID: p.Uuid, PaymentDate: dateString(p.PaymentDate), Status: p.Status, Method: p.Method, Amount: m(p.Amount),
			CashAccount: model.Ref{Code: p.CashAccountCode, Name: p.CashAccountName},
		})
	}
	return inv, nil
}

type invoiceAmounts struct {
	net, vat, total money.Money
	rate            money.Rate // the single rate, or the effective one for mixed lines
	lines           []db.InsertSupplierInvoiceLineParams
}

// invoiceTotals derives the totals from the lines (each with its own VAT
// rate) or from the header (net or total with one rate).
func invoiceTotals(ve *ValidationError, in model.SupplierInvoiceInput) invoiceAmounts {
	out := invoiceAmounts{net: money.Zero(), vat: money.Zero(), total: money.Zero()}
	if len(in.Lines) == 0 {
		rate := money.Rate(0)
		if in.VATRate != nil {
			rate = *in.VATRate
		}
		out.net, out.vat, out.total = amounts(ve, in.NetAmount, in.TotalAmount, rate)
		out.rate = rate
		return out
	}
	if len(in.Lines) > MaxInvoiceLines {
		ve.add("lines", "too_many", "an invoice has at most 100 lines")
		return out
	}
	rates := map[money.Rate]bool{}
	for i, l := range in.Lines {
		field := "lines." + strconv.Itoa(i)
		rate := money.Rate(0)
		if l.VATRate != nil {
			rate = *l.VATRate
		}
		if rate < 0 || rate > money.Percent(100) {
			ve.add(field+".vat_rate", "out_of_range", "vat_rate must be between 0 and 100")
			continue
		}
		if l.NetAmount == nil || l.NetAmount.IsNegative() {
			ve.add(field+".net_amount", "invalid", "net_amount is required (0 or more)")
			continue
		}
		if utf8.RuneCountInString(l.Description) > 500 {
			ve.add(field+".description", "too_long", "description is at most 500 characters")
		}
		vat := l.NetAmount.MulRate(rate)
		rates[rate] = true
		out.net, out.vat = out.net.Add(*l.NetAmount), out.vat.Add(vat)
		out.lines = append(out.lines, db.InsertSupplierInvoiceLineParams{
			LineNo: int32(i + 1), Description: strings.TrimSpace(l.Description), NetAmount: l.NetAmount.Numeric(),
			VatRate: rate.Numeric(), VatAmount: vat.Numeric(), TotalAmount: l.NetAmount.Add(vat).Numeric(),
		})
	}
	out.total = out.net.Add(out.vat)
	if len(ve.Details) == 0 && !out.total.IsPositive() {
		ve.add("lines", "invalid", "the invoice total must be positive")
	}
	switch {
	case len(rates) == 1:
		for r := range rates {
			out.rate = r
		}
	case out.net.IsPositive():
		// Effective rate of a mixed invoice (the expense row has one rate).
		out.rate = money.Rate(out.vat.Ratio(money.RateScale, out.net.Minor()).Minor())
		if out.rate > money.Percent(100) {
			out.rate = money.Percent(100)
		}
	}
	return out
}

// CreateSupplierInvoice records a supplier invoice and posts its payable
// accrual and its expense row in one transaction.
func (s *Service) CreateSupplierInvoice(ctx context.Context, orgID int64, in model.SupplierInvoiceInput) (model.SupplierInvoice, error) {
	ve := &ValidationError{}
	number := strings.TrimSpace(deref(in.InvoiceNo))
	if number == "" || utf8.RuneCountInString(number) > 64 {
		ve.add("invoice_no", "required", "invoice_no is required (at most 64 characters)")
	}
	date := optionalDate(ve, "invoice_date", in.InvoiceDate, s.today())
	currency := strings.ToUpper(strings.TrimSpace(deref(in.Currency)))
	if currency == "" {
		currency = string(money.TRY)
	}
	if !currencyRE.MatchString(currency) {
		ve.add("currency", "invalid", "currency must be an ISO 4217 code")
	}
	amt := invoiceTotals(ve, in)
	desc := strings.TrimSpace(deref(in.Description))
	if utf8.RuneCountInString(desc) > 1000 {
		ve.add("description", "too_long", "description is at most 1000 characters")
	}
	if err := ve.orNil(); err != nil {
		return model.SupplierInvoice{}, err
	}
	sup, err := s.supplierFor(ctx, s.q, orgID, "supplier_uuid", in.SupplierUUID, true)
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	due := optionalDate(ve, "due_date", in.DueDate, dueAfter(date, sup.PaymentTermDays))
	if due.Before(date) {
		ve.add("due_date", "before_invoice_date", "due_date must be on or after invoice_date")
	}
	if err := ve.orNil(); err != nil {
		return model.SupplierInvoice{}, err
	}
	cat, err := s.activeCategory(ctx, orgID, "category_uuid", in.CategoryUUID)
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	branch, vehicle, err := s.expenseRefs(ctx, orgID, in.BranchUUID, in.VehicleUUID, pgtype.Int8{})
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	var id uuid.UUID
	var accUUID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		acc, err := s.ensureSupplierAccount(ctx, q, orgID, sup, branch.Int64, currency)
		if err != nil {
			return err
		}
		if acc.Currency != currency {
			return validation("currency", "currency_mismatch", "the supplier account holds "+acc.Currency)
		}
		accUUID = acc.UUID
		row, err := q.InsertSupplierInvoice(ctx, db.InsertSupplierInvoiceParams{
			OrganizationID: orgID, SupplierID: sup.ID, AccountID: acc.ID, InvoiceNo: number, InvoiceDate: dateOf(date),
			DueDate: dateOf(due), Currency: currency, CategoryID: cat.ID, BranchID: branch, VehicleID: vehicle,
			Description: desc, NetAmount: amt.net.Numeric(), VatAmount: amt.vat.Numeric(), TotalAmount: amt.total.Numeric(),
			CreatedBy: actorID(ctx),
		})
		if uniqueViolation(err) {
			return ErrInvoiceNoTaken
		}
		if err != nil {
			return err
		}
		id = row.Uuid
		for _, l := range amt.lines {
			l.OrganizationID, l.InvoiceID = orgID, row.ID
			if err := q.InsertSupplierInvoiceLine(ctx, l); err != nil {
				return err
			}
		}
		text := desc
		if text == "" {
			text = sup.Title + " · " + number
		}
		if _, err := q.InsertInvoiceExpense(ctx, db.InsertInvoiceExpenseParams{
			OrganizationID: orgID, CategoryID: cat.ID, ExpenseDate: dateOf(date), Description: text, SupplierName: sup.Title,
			DocumentNo: number, Currency: currency, NetAmount: amt.net.Numeric(), VatRate: amt.rate.Numeric(),
			VatAmount: amt.vat.Numeric(), TotalAmount: amt.total.Numeric(), BranchID: branch, VehicleID: vehicle,
			SupplierID: int8Of(sup.ID), SupplierInvoiceID: int8Of(row.ID), CreatedBy: actorID(ctx),
		}); err != nil {
			return err
		}
		return s.syncInvoice(ctx, q, orgID, row.ID)
	})
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	s.publish(ctx, model.ResourceSupplierInvoices, "created", id)
	s.publish(ctx, model.ResourceExpenses, "created")
	s.publish(ctx, model.ResourceAccounts, "updated", accUUID)
	return s.GetSupplierInvoice(ctx, orgID, id)
}

// syncInvoice reconciles the payable accrual of an invoice with its state
// (on q, the caller's transaction): a live invoice has exactly one live
// accrual of its total, account and due date; a void one has none. Calling
// it again changes nothing.
func (s *Service) syncInvoice(ctx context.Context, q *db.Queries, orgID, invoiceID int64) error {
	inv, err := q.GetSupplierInvoiceRowByID(ctx, db.GetSupplierInvoiceRowByIDParams{OrganizationID: orgID, ID: invoiceID})
	if err != nil {
		return err
	}
	if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: inv.AccountID}); err != nil {
		return err
	}
	live, err := q.ListLiveSupplierInvoiceAccruals(ctx, db.ListLiveSupplierInvoiceAccrualsParams{
		OrganizationID: orgID, SupplierInvoiceID: int8Of(inv.ID),
	})
	if err != nil {
		return err
	}
	want := inv.Status != model.InvoiceVoid
	total := m(inv.TotalAmount)
	kept := false
	for _, l := range live {
		reason := ""
		switch {
		case !want:
			reason = model.ReasonInvoiceVoided
		case l.AccountID != inv.AccountID:
			reason = model.ReasonAccountChanged
		case !m(l.Amount).Equal(total):
			reason = model.ReasonAmountChanged
		case !l.DueDate.Time.Equal(inv.DueDate.Time):
			reason = model.ReasonDueChanged
		case kept:
			reason = model.ReasonNoLongerDue
		}
		if reason == "" {
			kept = true
			continue
		}
		if _, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
			OrganizationID: orgID, AccountID: l.AccountID, Kind: model.KindReversal, Direction: model.Debit,
			Purpose: l.Purpose, Amount: l.Amount, Currency: l.Currency, EntryDate: dateOf(s.today()), DueDate: l.DueDate,
			Description: l.Description, Reason: reason, SourceType: model.SourceSupplierInvoice, SourceUuid: uuidOf(inv.Uuid),
			SupplierInvoiceID: int8Of(inv.ID), ReversesEntryID: int8Of(l.ID), BranchID: l.BranchID, CreatedBy: actorID(ctx),
		}); err != nil {
			return err
		}
	}
	if !want || kept {
		return nil
	}
	sup, err := q.GetSupplierForFinanceByID(ctx, db.GetSupplierForFinanceByIDParams{OrganizationID: orgID, ID: inv.SupplierID})
	if err != nil {
		return err
	}
	_, err = q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		OrganizationID: orgID, AccountID: inv.AccountID, Kind: model.KindAccrual, Direction: model.Credit,
		Purpose: model.PurposeCharge, Amount: inv.TotalAmount, Currency: inv.Currency, EntryDate: inv.InvoiceDate,
		DueDate: inv.DueDate, Description: sup.Title + " · " + inv.InvoiceNo, SourceType: model.SourceSupplierInvoice,
		SourceUuid: uuidOf(inv.Uuid), SupplierInvoiceID: int8Of(inv.ID),
		AccrualKey: pgtype.Text{String: model.InvoiceKey(inv.ID), Valid: true}, BranchID: inv.BranchID, CreatedBy: actorID(ctx),
	})
	return err
}

// SyncSupplierInvoice re-runs the accrual reconciliation of one invoice
// (idempotent; tests and repairs).
func (s *Service) SyncSupplierInvoice(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, _, err := s.invoice(ctx, orgID, id)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(q *db.Queries) error { return s.syncInvoice(ctx, q, orgID, row.ID) })
}

// PatchSupplierInvoice changes the number, due date, classification and
// description of an invoice. Supplier, amounts, lines, invoice date and
// currency are fixed (void and record again). A moved due date posts the
// accrual again with the new date.
func (s *Service) PatchSupplierInvoice(ctx context.Context, orgID int64, id uuid.UUID, in model.SupplierInvoiceInput) (model.SupplierInvoice, error) {
	row, view, err := s.invoice(ctx, orgID, id)
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	if row.Status == model.InvoiceVoid {
		return model.SupplierInvoice{}, stateErr("invoice_void", "a void invoice cannot change")
	}
	ve := &ValidationError{}
	if (in.SupplierUUID != nil && *in.SupplierUUID != view.Supplier.UUID) || in.NetAmount != nil || in.TotalAmount != nil ||
		in.VATRate != nil || len(in.Lines) > 0 || in.InvoiceDate != nil || in.Currency != nil {
		ve.add("total_amount", "immutable", "supplier, amounts, lines, invoice date and currency are fixed; void the invoice and record it again")
	}
	p := db.UpdateSupplierInvoiceDetailsParams{
		OrganizationID: orgID, ID: row.ID, InvoiceNo: row.InvoiceNo, DueDate: row.DueDate, Description: row.Description,
		CategoryID: row.CategoryID, BranchID: row.BranchID, VehicleID: row.VehicleID,
	}
	if in.InvoiceNo != nil {
		p.InvoiceNo = strings.TrimSpace(*in.InvoiceNo)
		if p.InvoiceNo == "" || utf8.RuneCountInString(p.InvoiceNo) > 64 {
			ve.add("invoice_no", "required", "invoice_no is required (at most 64 characters)")
		}
	}
	if in.DueDate != nil {
		d := optionalDate(ve, "due_date", in.DueDate, row.DueDate.Time)
		if d.Before(row.InvoiceDate.Time) {
			ve.add("due_date", "before_invoice_date", "due_date must be on or after invoice_date")
		}
		p.DueDate = dateOf(d)
	}
	if in.Description != nil {
		p.Description = strings.TrimSpace(*in.Description)
		if utf8.RuneCountInString(p.Description) > 1000 {
			ve.add("description", "too_long", "description is at most 1000 characters")
		}
	}
	if err := ve.orNil(); err != nil {
		return model.SupplierInvoice{}, err
	}
	if in.CategoryUUID != nil {
		cat, err := s.activeCategory(ctx, orgID, "category_uuid", in.CategoryUUID)
		if err != nil {
			return model.SupplierInvoice{}, err
		}
		p.CategoryID = cat.ID
	}
	if in.BranchUUID != nil || in.ClearBranch || in.VehicleUUID != nil || in.ClearVehicle {
		keep := row.BranchID
		if in.ClearBranch {
			keep = pgtype.Int8{}
		}
		branch, vehicle, err := s.expenseRefs(ctx, orgID, in.BranchUUID, in.VehicleUUID, keep)
		if err != nil {
			return model.SupplierInvoice{}, err
		}
		p.BranchID = branch
		if in.VehicleUUID != nil {
			p.VehicleID = vehicle
		}
		if in.ClearVehicle {
			p.VehicleID = pgtype.Int8{}
		}
	}
	dueMoved := !p.DueDate.Time.Equal(row.DueDate.Time)
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.UpdateSupplierInvoiceDetails(ctx, p); err != nil {
			if uniqueViolation(err) {
				return ErrInvoiceNoTaken
			}
			return err
		}
		text := p.Description
		if text == "" {
			text = view.Supplier.Name + " · " + p.InvoiceNo
		}
		if _, err := q.SyncInvoiceExpense(ctx, db.SyncInvoiceExpenseParams{
			OrganizationID: orgID, SupplierInvoiceID: int8Of(row.ID), CategoryID: p.CategoryID, Description: text,
			DocumentNo: p.InvoiceNo, BranchID: p.BranchID, VehicleID: p.VehicleID,
		}); err != nil {
			return err
		}
		if dueMoved {
			return s.syncInvoice(ctx, q, orgID, row.ID)
		}
		return nil
	})
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	s.publish(ctx, model.ResourceSupplierInvoices, "updated", id)
	s.publish(ctx, model.ResourceExpenses, "updated")
	if dueMoved {
		s.publish(ctx, model.ResourceAccounts, "updated", view.Account.UUID)
	}
	return s.GetSupplierInvoice(ctx, orgID, id)
}

// VoidSupplierInvoice voids an invoice with a reason: its accrual is
// reversed and its expense row voided in the same transaction. An invoice
// with live payments must have them voided first.
func (s *Service) VoidSupplierInvoice(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.SupplierInvoice, error) {
	row, view, err := s.invoice(ctx, orgID, id)
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	r, err := requireReason("reason", reason)
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	if row.Status == model.InvoiceVoid {
		return model.SupplierInvoice{}, stateErr("already_void", "the invoice is already void")
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockSupplierInvoices(ctx, db.LockSupplierInvoicesParams{OrganizationID: orgID, Ids: []int64{row.ID}}); err != nil {
			return err
		}
		n, err := q.CountLiveInvoiceAllocations(ctx, db.CountLiveInvoiceAllocationsParams{OrganizationID: orgID, InvoiceID: row.ID})
		if err != nil {
			return err
		}
		if n > 0 {
			return stateErr("invoice_has_payments", "void the payments allocated to this invoice first")
		}
		now := pgtype.Timestamptz{Time: s.clock(), Valid: true}
		k, err := q.VoidSupplierInvoice(ctx, db.VoidSupplierInvoiceParams{
			OrganizationID: orgID, ID: row.ID, VoidReason: r, VoidedBy: actorID(ctx), VoidedAt: now,
		})
		if err != nil {
			return err
		}
		if k != 1 {
			return stateErr("already_void", "the invoice is already void")
		}
		if err := s.syncInvoice(ctx, q, orgID, row.ID); err != nil {
			return err
		}
		x, err := q.GetInvoiceExpense(ctx, db.GetInvoiceExpenseParams{OrganizationID: orgID, SupplierInvoiceID: int8Of(row.ID)})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = q.VoidExpense(ctx, db.VoidExpenseParams{OrganizationID: orgID, ID: x.ID, VoidReason: r, VoidedBy: actorID(ctx), VoidedAt: now})
		return err
	})
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	s.publish(ctx, model.ResourceSupplierInvoices, "updated", id)
	s.publish(ctx, model.ResourceExpenses, "updated")
	s.publish(ctx, model.ResourceAccounts, "updated", view.Account.UUID)
	s.publish(ctx, model.ResourceEntries, "created")
	return s.GetSupplierInvoice(ctx, orgID, id)
}

// PayablesSummary is the open supplier invoices by due state (overdue, due
// within window days) in the caller's branch scope.
func (s *Service) PayablesSummary(ctx context.Context, orgID int64, window int) (model.PayablesSummary, error) {
	if window <= 0 || window > 365 {
		window = model.DueSoonDays
	}
	today, soon := s.dueWindow(window)
	r, err := s.q.PayablesSummary(ctx, db.PayablesSummaryParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Today: today, Soon: soon,
	})
	if err != nil {
		return model.PayablesSummary{}, err
	}
	return model.PayablesSummary{
		Currency: string(money.TRY), WindowDays: window, OverdueAmount: m(r.OverdueAmount), OverdueCount: r.OverdueCount,
		DueSoonAmount: m(r.DueSoonAmount), DueSoonCount: r.DueSoonCount, OpenAmount: m(r.OpenAmount), OpenCount: r.OpenCount,
		AttentionCount: r.OverdueCount + r.DueSoonCount,
	}, nil
}

// --------------------------------------------------------------- payments

// ListSupplierPayments lists supplier payments (branch scope: payments
// from the caller's branches' cash accounts).
func (s *Service) ListSupplierPayments(ctx context.Context, orgID int64, f model.SupplierPaymentFilter) ([]model.SupplierPayment, int64, error) {
	p := db.ListSupplierPaymentsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Ids: nilIDs(f.IDs), Statuses: nilIfEmpty(f.Statuses),
		Methods: nilIfEmpty(f.Methods), DateFrom: dateNarg(f.From), DateTo: dateNarg(f.To), Q: textOf(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if _, ok := SupplierPaymentSortColumns[p.SortField]; !ok {
		p.SortField, p.SortDesc = "payment_date", true
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	for _, id := range f.SupplierUUIDs {
		sup, err := s.q.GetSupplierForFinance(ctx, db.GetSupplierForFinanceParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil, 0, validation("supplier", "not_found", "unknown supplier")
		}
		if err != nil {
			return nil, 0, err
		}
		p.SupplierIds = append(p.SupplierIds, sup.ID)
	}
	for _, id := range f.CashAccountUUIDs {
		c, err := s.q.GetCashAccountRow(ctx, db.GetCashAccountRowParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil, 0, validation("cash_account", "not_found", "unknown cash account")
		}
		if err != nil {
			return nil, 0, err
		}
		p.CashAccountIds = append(p.CashAccountIds, c.ID)
	}
	if f.InvoiceUUID != nil {
		inv, err := s.q.GetSupplierInvoiceRow(ctx, db.GetSupplierInvoiceRowParams{OrganizationID: orgID, Uuid: *f.InvoiceUUID})
		if notFound(err) {
			return nil, 0, validation("invoice", "not_found", "unknown supplier invoice")
		}
		if err != nil {
			return nil, 0, err
		}
		p.InvoiceID = int8Of(inv.ID)
	}
	rows, err := s.q.ListSupplierPayments(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.SupplierPayment, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	index := map[int64]int{}
	var total int64
	for _, r := range rows {
		total = r.Total
		index[r.ID] = len(out)
		ids = append(ids, r.ID)
		allocated := m(r.AllocatedAmount)
		out = append(out, model.SupplierPayment{
			UUID: r.Uuid, Supplier: model.Ref{UUID: r.SupplierUuid, Code: r.SupplierCode, Name: r.SupplierTitle},
			Account:         model.Ref{UUID: r.AccountUuid, Code: r.AccountCode},
			CashAccount:     model.Ref{UUID: r.CashAccountUuid, Code: r.CashAccountCode, Name: r.CashAccountName},
			CashAccountKind: r.CashAccountKind, Amount: m(r.Amount), AllocatedAmount: allocated,
			UnallocatedAmt: m(r.Amount).Sub(allocated).ClampZero(), Currency: r.Currency, PaymentDate: dateString(r.PaymentDate),
			Method: r.Method, Reference: r.Reference, Description: r.Description,
			Branch: refOf(r.BranchUuid, r.BranchCode, r.BranchName), LedgerEntryUUID: uuidPtr(r.LedgerEntryUuid),
			MovementUUID: uuidPtr(r.MovementUuid), Allocations: []model.Allocation{}, Status: r.Status,
			VoidReason: r.VoidReason, VoidedAt: timePtr(r.VoidedAt),
			CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail), CreatedAt: r.CreatedAt.Time.UTC(),
		})
	}
	if len(ids) > 0 {
		allocs, err := s.q.ListSupplierPaymentAllocations(ctx, db.ListSupplierPaymentAllocationsParams{OrganizationID: orgID, PaymentIds: ids})
		if err != nil {
			return nil, 0, err
		}
		for _, a := range allocs {
			i := index[a.PaymentID]
			out[i].Allocations = append(out[i].Allocations, model.Allocation{
				InvoiceUUID: a.InvoiceUuid, InvoiceNo: a.InvoiceNo, DueDate: dateString(a.DueDate), Amount: m(a.Amount),
			})
		}
	}
	return out, total, nil
}

// GetSupplierPayment returns one payment the caller may see.
func (s *Service) GetSupplierPayment(ctx context.Context, orgID int64, id uuid.UUID) (model.SupplierPayment, error) {
	row, err := s.q.GetSupplierPaymentRow(ctx, db.GetSupplierPaymentRowParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.SupplierPayment{}, ErrSupplierPaymentNotFound
	}
	if err != nil {
		return model.SupplierPayment{}, err
	}
	items, _, err := s.ListSupplierPayments(ctx, orgID, model.SupplierPaymentFilter{IDs: []int64{row.ID}, Limit: 1})
	if err != nil {
		return model.SupplierPayment{}, err
	}
	if len(items) == 0 {
		return model.SupplierPayment{}, ErrSupplierPaymentNotFound
	}
	return items[0], nil
}

type plannedAllocation struct {
	invoiceID int64
	amount    money.Money
}

// CreateSupplierPayment pays a supplier from a cash / bank account: one
// cash movement, one ledger entry and the allocations to open invoices in
// one transaction (the cash account and the invoices are locked).
func (s *Service) CreateSupplierPayment(ctx context.Context, orgID int64, in model.SupplierPaymentInput) (model.SupplierPayment, error) {
	ve := &ValidationError{}
	if in.Amount == nil || !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	if in.CashAccountUUID == nil {
		ve.add("cash_account_uuid", "required", "cash_account_uuid is required")
	}
	method := strings.TrimSpace(in.Method)
	if method == "" {
		method = "bank_transfer"
	}
	if !contains(model.SupplierPaymentMethods, method) {
		ve.add("method", "invalid", "unknown method")
	}
	ref := strings.TrimSpace(in.Reference)
	if utf8.RuneCountInString(ref) > 64 {
		ve.add("reference", "too_long", "reference is at most 64 characters")
	}
	desc := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(desc) > 500 {
		ve.add("description", "too_long", "description is at most 500 characters")
	}
	date := optionalDate(ve, "payment_date", in.PaymentDate, s.today())
	if len(in.Allocations) > MaxInvoiceLines {
		ve.add("allocations", "too_many", "at most 100 allocations")
	}
	for i, a := range in.Allocations {
		if a.InvoiceUUID == nil {
			ve.add("allocations."+strconv.Itoa(i)+".invoice_uuid", "required", "invoice_uuid is required")
		}
		if a.Amount == nil || !a.Amount.IsPositive() {
			ve.add("allocations."+strconv.Itoa(i)+".amount", "invalid", "amount must be positive")
		}
	}
	if err := ve.orNil(); err != nil {
		return model.SupplierPayment{}, err
	}
	sup, err := s.supplierFor(ctx, s.q, orgID, "supplier_uuid", in.SupplierUUID, false)
	if err != nil {
		return model.SupplierPayment{}, err
	}
	// Explicit allocations: the invoices must be the supplier's and visible.
	explicit := make([]plannedAllocation, 0, len(in.Allocations))
	seen := map[int64]bool{}
	for i, a := range in.Allocations {
		field := "allocations." + strconv.Itoa(i) + ".invoice_uuid"
		inv, err := s.q.GetSupplierInvoiceRow(ctx, db.GetSupplierInvoiceRowParams{OrganizationID: orgID, Uuid: *a.InvoiceUUID})
		if notFound(err) || (err == nil && (inv.SupplierID != sup.ID || !rowVisible(ctx, inv.BranchID))) {
			return model.SupplierPayment{}, validation(field, "not_found", "unknown invoice of this supplier")
		}
		if err != nil {
			return model.SupplierPayment{}, err
		}
		if seen[inv.ID] {
			return model.SupplierPayment{}, validation(field, "duplicate", "an invoice is allocated once per payment")
		}
		seen[inv.ID] = true
		explicit = append(explicit, plannedAllocation{invoiceID: inv.ID, amount: *a.Amount})
	}
	auto := len(explicit) == 0 && (in.AutoAllocate == nil || *in.AutoAllocate)
	var payID, cashUUID, accUUID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		locked, err := s.lockForMovement(ctx, q, orgID, cashTarget{"cash_account_uuid", *in.CashAccountUUID})
		if err != nil {
			return err
		}
		cash := locked[0]
		cashUUID = cash.row.Uuid
		if err := checkFunds("amount", cash, *in.Amount); err != nil {
			return err
		}
		acc, err := s.ensureSupplierAccount(ctx, q, orgID, sup, cash.row.BranchID.Int64, cash.row.Currency)
		if err != nil {
			return err
		}
		accUUID = acc.UUID
		if acc.Currency != cash.row.Currency {
			return validation("cash_account_uuid", "currency_mismatch", "the supplier account holds "+acc.Currency)
		}
		plan, err := s.planAllocations(ctx, q, orgID, sup.ID, cash.row.Currency, *in.Amount, explicit, auto)
		if err != nil {
			return err
		}
		pay, err := q.InsertSupplierPayment(ctx, db.InsertSupplierPaymentParams{
			OrganizationID: orgID, SupplierID: sup.ID, AccountID: acc.ID, CashAccountID: cash.row.ID, Amount: in.Amount.Numeric(),
			Currency: cash.row.Currency, PaymentDate: dateOf(date), Method: method, Reference: ref, Description: desc,
			BranchID: cash.row.BranchID, CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		payID = pay.Uuid
		text := desc
		if text == "" {
			text = strings.Trim(sup.Title+" · "+ref, " ·")
		}
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
			return err
		}
		entry, err := q.InsertSupplierPaymentEntry(ctx, db.InsertSupplierPaymentEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Amount: in.Amount.Numeric(), Currency: cash.row.Currency,
			EntryDate: dateOf(date), Description: text, SourceUuid: uuidOf(pay.Uuid), SupplierPaymentID: int8Of(pay.ID),
			BranchID: cash.row.BranchID, CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		mv, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
			OrganizationID: orgID, CashAccountID: cash.row.ID, Kind: model.MoveSupplierPayment, Direction: model.DirOut,
			Amount: in.Amount.Numeric(), Currency: cash.row.Currency, MovementDate: dateOf(date), Description: text,
			BranchID: cash.row.BranchID, LedgerEntryID: int8Of(entry.ID), SupplierAccountID: int8Of(acc.ID), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		if err := q.SetSupplierPaymentPosting(ctx, db.SetSupplierPaymentPostingParams{
			OrganizationID: orgID, ID: pay.ID, LedgerEntryID: int8Of(entry.ID), CashMovementID: int8Of(mv.ID),
		}); err != nil {
			return err
		}
		for _, a := range plan {
			if err := q.InsertSupplierPaymentAllocation(ctx, db.InsertSupplierPaymentAllocationParams{
				OrganizationID: orgID, PaymentID: pay.ID, InvoiceID: a.invoiceID, Amount: a.amount.Numeric(),
			}); err != nil {
				return err
			}
			if _, err := q.SetSupplierInvoicePaid(ctx, db.SetSupplierInvoicePaidParams{OrganizationID: orgID, ID: a.invoiceID}); err != nil {
				return err
			}
		}
		s.publish(ctx, model.ResourceMovements, "created", mv.Uuid)
		s.publish(ctx, model.ResourceEntries, "created", entry.Uuid)
		return nil
	})
	if err != nil {
		return model.SupplierPayment{}, err
	}
	s.publish(ctx, model.ResourceSupplierPayments, "created", payID)
	s.publish(ctx, model.ResourceSupplierInvoices, "updated")
	s.publish(ctx, model.ResourceCashAccounts, "updated", cashUUID)
	s.publish(ctx, model.ResourceAccounts, "updated", accUUID)
	return s.GetSupplierPayment(ctx, orgID, payID)
}

// planAllocations locks the invoices a payment settles and checks the
// amounts: explicit allocations must fit each invoice's open amount and the
// payment; auto allocation pays the supplier's open invoices (visible to
// the caller) oldest due date first.
func (s *Service) planAllocations(ctx context.Context, q *db.Queries, orgID, supplierID int64, currency string,
	amount money.Money, explicit []plannedAllocation, auto bool) ([]plannedAllocation, error) {
	if len(explicit) > 0 {
		ids := make([]int64, 0, len(explicit))
		sum := money.Zero()
		for _, a := range explicit {
			ids = append(ids, a.invoiceID)
			sum = sum.Add(a.amount)
		}
		if sum.Cmp(amount) > 0 {
			return nil, validation("allocations", "exceeds_amount", "the allocations exceed the payment amount")
		}
		rows, err := q.LockSupplierInvoices(ctx, db.LockSupplierInvoicesParams{OrganizationID: orgID, Ids: ids})
		if err != nil {
			return nil, err
		}
		byID := map[int64]db.LockSupplierInvoicesRow{}
		for _, r := range rows {
			byID[r.ID] = r
		}
		for i, a := range explicit {
			r, ok := byID[a.invoiceID]
			field := "allocations." + strconv.Itoa(i)
			if !ok || r.SupplierID != supplierID {
				return nil, validation(field+".invoice_uuid", "not_found", "unknown invoice of this supplier")
			}
			if r.Status == model.InvoiceVoid || r.Status == model.InvoicePaid {
				return nil, validation(field+".invoice_uuid", "not_open", "the invoice is not open")
			}
			if r.Currency != currency {
				return nil, validation(field+".invoice_uuid", "currency_mismatch", "the invoice is in another currency")
			}
			open := m(r.TotalAmount).Sub(m(r.PaidAmount))
			if a.amount.Cmp(open) > 0 {
				return nil, validation(field+".amount", "exceeds_open", "the amount exceeds the open amount ("+open.String()+")")
			}
		}
		return explicit, nil
	}
	if !auto {
		return nil, nil
	}
	open, err := q.ListOpenSupplierInvoices(ctx, db.ListOpenSupplierInvoicesParams{OrganizationID: orgID, SupplierID: supplierID, Currency: currency})
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for _, r := range open {
		if rowVisible(ctx, r.BranchID) {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.LockSupplierInvoices(ctx, db.LockSupplierInvoicesParams{OrganizationID: orgID, Ids: ids})
	if err != nil {
		return nil, err
	}
	byID := map[int64]db.LockSupplierInvoicesRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	left := amount
	out := []plannedAllocation{}
	for _, r0 := range open {
		r, ok := byID[r0.ID]
		if !ok || !left.IsPositive() || r.Status == model.InvoiceVoid || r.Status == model.InvoicePaid {
			continue
		}
		due := m(r.TotalAmount).Sub(m(r.PaidAmount))
		if !due.IsPositive() {
			continue
		}
		part := due.Min(left)
		out = append(out, plannedAllocation{invoiceID: r.ID, amount: part})
		left = left.Sub(part)
	}
	return out, nil
}

// VoidSupplierPayment voids a payment with a reason: its ledger entry and
// cash movement are reversed and its allocations stop counting, in one
// transaction.
func (s *Service) VoidSupplierPayment(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.SupplierPayment, error) {
	view, err := s.GetSupplierPayment(ctx, orgID, id)
	if err != nil {
		return model.SupplierPayment{}, err
	}
	r, err := requireReason("reason", reason)
	if err != nil {
		return model.SupplierPayment{}, err
	}
	row, err := s.q.GetSupplierPaymentRow(ctx, db.GetSupplierPaymentRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.SupplierPayment{}, err
	}
	if row.Status != model.SupplierPaymentPosted {
		return model.SupplierPayment{}, stateErr("already_void", "the payment is already void")
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockCashAccounts(ctx, db.LockCashAccountsParams{OrganizationID: orgID, Ids: []int64{row.CashAccountID}}); err != nil {
			return err
		}
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: row.AccountID}); err != nil {
			return err
		}
		n, err := q.VoidSupplierPayment(ctx, db.VoidSupplierPaymentParams{
			OrganizationID: orgID, ID: row.ID, VoidReason: r, VoidedBy: actorID(ctx), VoidedAt: pgtype.Timestamptz{Time: s.clock(), Valid: true},
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("already_void", "the payment is already void")
		}
		today := dateOf(s.today())
		if row.LedgerEntryID.Valid {
			if _, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
				OrganizationID: orgID, AccountID: row.AccountID, Kind: model.KindReversal, Direction: model.Credit,
				Purpose: model.PurposeGeneral, Amount: row.Amount, Currency: row.Currency, EntryDate: today, DueDate: row.PaymentDate,
				Description: view.Supplier.Name, Reason: r, SourceType: model.SourceSupplierPayment, SourceUuid: uuidOf(row.Uuid),
				SupplierPaymentID: int8Of(row.ID), ReversesEntryID: row.LedgerEntryID, BranchID: row.BranchID, CreatedBy: actorID(ctx),
			}); err != nil {
				return err
			}
		}
		if row.CashMovementID.Valid {
			rev, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
				OrganizationID: orgID, CashAccountID: row.CashAccountID, Kind: model.MoveReversal, Direction: model.DirIn,
				Amount: row.Amount, Currency: row.Currency, MovementDate: today, Description: view.Supplier.Name, Reason: r,
				BranchID: row.BranchID, SupplierAccountID: int8Of(row.AccountID), ReversesMovementID: row.CashMovementID,
				CreatedBy: actorID(ctx),
			})
			if err != nil {
				return err
			}
			s.publish(ctx, model.ResourceMovements, "created", rev.Uuid)
		}
		invoices, err := q.ListPaymentAllocationInvoiceIDs(ctx, db.ListPaymentAllocationInvoiceIDsParams{OrganizationID: orgID, PaymentID: row.ID})
		if err != nil {
			return err
		}
		if len(invoices) > 0 {
			if _, err := q.LockSupplierInvoices(ctx, db.LockSupplierInvoicesParams{OrganizationID: orgID, Ids: invoices}); err != nil {
				return err
			}
		}
		for _, inv := range invoices {
			if _, err := q.SetSupplierInvoicePaid(ctx, db.SetSupplierInvoicePaidParams{OrganizationID: orgID, ID: inv}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.SupplierPayment{}, err
	}
	s.publish(ctx, model.ResourceSupplierPayments, "updated", id)
	s.publish(ctx, model.ResourceSupplierInvoices, "updated")
	s.publish(ctx, model.ResourceEntries, "created")
	s.publish(ctx, model.ResourceCashAccounts, "updated", view.CashAccount.UUID)
	s.publish(ctx, model.ResourceAccounts, "updated", view.Account.UUID)
	return s.GetSupplierPayment(ctx, orgID, id)
}

// supplierPaymentFromMovement records a manual supplier_payment movement
// that names a supplier as a supplier payment (FIFO allocation).
func (s *Service) supplierPaymentFromMovement(ctx context.Context, orgID int64, cash uuid.UUID, in model.MovementInput) (model.Movement, error) {
	pay, err := s.CreateSupplierPayment(ctx, orgID, model.SupplierPaymentInput{
		SupplierUUID: in.SupplierUUID, CashAccountUUID: &cash, Amount: in.Amount, PaymentDate: in.MovementDate,
		Description: in.Description,
	})
	if err != nil {
		return model.Movement{}, mapAccountField(err)
	}
	if pay.MovementUUID == nil {
		return model.Movement{}, errors.New("finance: supplier payment without movement")
	}
	return s.GetMovement(ctx, orgID, *pay.MovementUUID)
}

// linkSupplierByName resolves the supplier of a free-text name: exactly one
// active supplier with that title (case-insensitive) is linked.
func (s *Service) linkSupplierByName(ctx context.Context, orgID int64, name string) pgtype.Int8 {
	name = strings.TrimSpace(name)
	if name == "" {
		return pgtype.Int8{}
	}
	rows, err := s.q.FindSupplierIDsByTitle(ctx, db.FindSupplierIDsByTitleParams{OrganizationID: orgID, Title: name})
	if err != nil || len(rows) != 1 {
		return pgtype.Int8{}
	}
	return int8Of(rows[0].ID)
}

// ------------------------------------------------------------ attachments

// UploadInvoiceAttachment stores (or replaces) the scanned / PDF invoice in
// private storage under the organization prefix.
func (s *Service) UploadInvoiceAttachment(ctx context.Context, orgID int64, id uuid.UUID, in FileUpload) (model.SupplierInvoice, error) {
	if s.storage == nil {
		return model.SupplierInvoice{}, ErrStorageUnavailable
	}
	row, _, err := s.invoice(ctx, orgID, id)
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	if in.Size <= 0 || in.Body == nil {
		return model.SupplierInvoice{}, validation("file", "required", "file is required")
	}
	if in.Size > MaxAttachmentBytes {
		return model.SupplierInvoice{}, validation("file", "too_large", "file must be at most 10 MiB")
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.Body, head)
	head = head[:n]
	mime, ext, ok := SniffAttachment(head)
	if !ok {
		return model.SupplierInvoice{}, validation("file", "invalid_type", "file must be a PDF, JPEG, PNG or WebP")
	}
	orgUUID, err := s.q.GetOrganizationUUIDForFinance(ctx, orgID)
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	key := storage.SupplierInvoiceAttachmentObjectKey(orgUUID.String(), id.String(), uuid.NewString(), ext)
	name := cleanFilename(in.Filename, ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime, Filename: name,
	}, key); err != nil {
		return model.SupplierInvoice{}, err
	}
	if _, err := s.q.SetSupplierInvoiceAttachment(ctx, db.SetSupplierInvoiceAttachmentParams{
		OrganizationID: orgID, ID: row.ID, AttachmentObjectKey: pgtype.Text{String: key, Valid: true}, AttachmentName: name,
		AttachmentContentType: mime, AttachmentSizeBytes: in.Size,
	}); err != nil {
		_ = s.storage.Delete(ctx, key)
		return model.SupplierInvoice{}, err
	}
	if row.AttachmentObjectKey.Valid && row.AttachmentObjectKey.String != key {
		_ = s.storage.Delete(ctx, row.AttachmentObjectKey.String)
	}
	s.publish(ctx, model.ResourceSupplierInvoices, "updated", id)
	return s.GetSupplierInvoice(ctx, orgID, id)
}

// OpenInvoiceAttachment opens the invoice file for streaming (only keys
// under the organization's prefix are served).
func (s *Service) OpenInvoiceAttachment(ctx context.Context, orgID int64, id uuid.UUID) (AttachmentFile, error) {
	if s.storage == nil {
		return AttachmentFile{}, ErrStorageUnavailable
	}
	row, _, err := s.invoice(ctx, orgID, id)
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
		s.log.Error("supplier_invoice_attachment_outside_organization", "invoice", id.String())
		return AttachmentFile{}, ErrNoAttachment
	}
	rc, size, err := s.storage.Download(ctx, row.AttachmentObjectKey.String)
	if err != nil {
		s.log.Warn("supplier_invoice_attachment_missing", "invoice", id.String(), "error", err)
		return AttachmentFile{}, ErrNoAttachment
	}
	if size <= 0 {
		size = row.AttachmentSizeBytes
	}
	return AttachmentFile{Body: rc, ContentType: row.AttachmentContentType, Name: row.AttachmentName, Size: size}, nil
}

// DeleteInvoiceAttachment removes the file of an invoice.
func (s *Service) DeleteInvoiceAttachment(ctx context.Context, orgID int64, id uuid.UUID) (model.SupplierInvoice, error) {
	row, _, err := s.invoice(ctx, orgID, id)
	if err != nil {
		return model.SupplierInvoice{}, err
	}
	if !row.AttachmentObjectKey.Valid {
		return model.SupplierInvoice{}, ErrNoAttachment
	}
	if _, err := s.q.SetSupplierInvoiceAttachment(ctx, db.SetSupplierInvoiceAttachmentParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return model.SupplierInvoice{}, err
	}
	if s.storage != nil {
		_ = s.storage.Delete(ctx, row.AttachmentObjectKey.String)
	}
	s.publish(ctx, model.ResourceSupplierInvoices, "updated", id)
	return s.GetSupplierInvoice(ctx, orgID, id)
}
