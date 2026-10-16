package invoices_test

import (
	"context"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	finmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	supmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/model"
	supusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// TestSupplierDocumentsStayOutOfSalesInvoices: supplier invoices and their
// voids (TEC-721) are purchase documents on the supplier account. They are
// never invoiceable items, the supplier account cannot be invoiced, and
// the rentals picker only counts customer-side accruals.
func TestSupplierDocumentsStayOutOfSalesInvoices(t *testing.T) {
	f := newFx(t, true)
	ctx := context.Background()
	r := f.rental(f.branch, f.person, f.personD)

	var category uuid.UUID
	f.must(f.pool.QueryRow(ctx, `INSERT INTO expense_categories (organization_id, code, name) VALUES ($1, 'parts', 'Parça') RETURNING uuid`,
		f.org).Scan(&category))
	sup := supusecase.New(f.pool, f.q, f.fin, nil)
	title, typ, term := "Parça Tedarik A.Ş.", supmodel.TypeService, int32(30)
	s, err := sup.Create(f.ctx, f.org, supmodel.Input{Title: &title, SupplierType: &typ, PaymentTermDays: &term})
	f.must(err)
	no, total, rate := "ST-1", money.MustParse("600"), money.Percent(20)
	si, err := f.fin.CreateSupplierInvoice(f.ctx, f.org, finmodel.SupplierInvoiceInput{
		SupplierUUID: &s.UUID, InvoiceNo: &no, CategoryUUID: &category, BranchUUID: &f.branch.uuid,
		TotalAmount: &total, VATRate: &rate,
	})
	f.must(err)
	// A second invoice voided: the void reverses the supplier accrual.
	no2 := "ST-2"
	si2, err := f.fin.CreateSupplierInvoice(f.ctx, f.org, finmodel.SupplierInvoiceInput{
		SupplierUUID: &s.UUID, InvoiceNo: &no2, CategoryUUID: &category, BranchUUID: &f.branch.uuid,
		TotalAmount: &total, VATRate: &rate,
	})
	f.must(err)
	_, err = f.fin.VoidSupplierInvoice(f.ctx, f.org, si2.UUID, "hatalı kayıt")
	f.must(err)

	var supplierAccount uuid.UUID
	f.must(f.pool.QueryRow(ctx, `SELECT uuid FROM accounts WHERE organization_id = $1 AND party_kind = 'supplier'`, f.org).Scan(&supplierAccount))
	if n := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND supplier_invoice_id IS NOT NULL`, f.org); n < 2 {
		t.Fatalf("%d supplier entries posted, want the invoices and the void", n)
	}
	supplierEntries := []uuid.UUID{}
	rows, err := f.pool.Query(ctx, `SELECT uuid FROM ledger_entries WHERE organization_id = $1 AND supplier_invoice_id IS NOT NULL`, f.org)
	f.must(err)
	for rows.Next() {
		var id uuid.UUID
		f.must(rows.Scan(&id))
		supplierEntries = append(supplierEntries, id)
	}
	rows.Close()

	// The invoiceable items of the organization: the rent only.
	all, err := f.q.ListInvoiceableEntries(ctx, db.ListInvoiceableEntriesParams{OrganizationID: f.org})
	f.must(err)
	if len(all) != 1 || uuid.UUID(all[0].RentalUuid.Bytes) != r.UUID {
		t.Fatalf("invoiceable entries %+v, want the rent of %s only", all, r.UUID)
	}
	byEntry, err := f.q.ListInvoiceableEntries(ctx, db.ListInvoiceableEntriesParams{OrganizationID: f.org, EntryUuids: supplierEntries})
	f.must(err)
	if len(byEntry) != 0 {
		t.Fatalf("supplier entries are invoiceable: %+v", byEntry)
	}

	// The supplier account has nothing to invoice and cannot get a draft.
	items, err := f.inv.Items(f.ctx, f.org, model.ItemFilter{AccountUUID: &supplierAccount})
	f.must(err)
	if len(items) != 0 {
		t.Fatalf("supplier account items %+v", items)
	}
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{AccountUUID: &supplierAccount}); err == nil {
		t.Fatal("a sales draft was cut from the supplier account")
	}
	// Naming supplier entries on a customer account invoices nothing.
	var personAccount uuid.UUID
	f.must(f.pool.QueryRow(ctx, `SELECT uuid FROM accounts WHERE organization_id = $1 AND customer_id = $2`, f.org, f.person.id).Scan(&personAccount))
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{AccountUUID: &personAccount, EntryUUIDs: supplierEntries}); err == nil {
		t.Fatal("supplier entries were invoiced through a customer account")
	}

	// The rentals picker counts the customer rent only.
	rentals, total2, err := f.inv.UninvoicedRentals(f.ctx, f.org, model.RentalFilter{Limit: 10})
	f.must(err)
	if total2 != 1 || len(rentals) != 1 || rentals[0].UUID != r.UUID || rentals[0].ItemCount != 1 {
		t.Fatalf("uninvoiced rentals %+v (total %d)", rentals, total2)
	}
	if n := f.count(`SELECT count(*) FROM ledger_entry_links WHERE organization_id = $1 AND invoice_id IS NOT NULL`, f.org); n != 0 {
		t.Fatalf("%d invoice links after refused drafts", n)
	}
	_ = si
}
