package invoices_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/google/uuid"
)

// TestPeriodicRentalPeriodsAreInvoiceItems: a rental billed per period
// (corporate agreement, monthly invoicing, TEC-723) accrues one rent entry
// per posted billing period. Each period is its own invoice item: listed,
// counted by the rentals picker, invoiced alone with a rent line that names
// the period and carries its days, and never invoiced twice.
func TestPeriodicRentalPeriodsAreInvoiceItems(t *testing.T) {
	f := newFx(t, true)
	ctx := context.Background()

	// A two-day sale (2 × 1 199.99) whose rental is billed per period.
	v := f.vehicle(f.branch)
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	w := resmodel.Where{PickupBranchUUID: &f.branch.uuid, StartsAt: &start, EndsAt: ptrTime(start.Add(48 * time.Hour))}
	quote, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{Where: w, Channel: "branch", ProductUUID: &f.product.uuid})
	f.must(err)
	res, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &quote.UUID, CustomerUUID: &f.company.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.companyD.uuid}},
	})
	f.must(err)
	r, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	var rentalID, reservationID, account int64
	f.must(f.pool.QueryRow(ctx, `UPDATE rentals SET rent_billing = 'periodic' WHERE organization_id = $1 AND uuid = $2
		RETURNING id, reservation_id`, f.org, r.UUID).Scan(&rentalID, &reservationID))
	f.must(f.pool.QueryRow(ctx, `INSERT INTO accounts (organization_id, code, party_kind, customer_id, name, payment_term_days)
		VALUES ($1, 'CR-000901', 'customer', $2, 'Kaya Lojistik', 30) RETURNING id`, f.org, f.company.id).Scan(&account))

	// Two posted periods, accrued as SyncRental does (key, period, start).
	entries := make([]uuid.UUID, 2)
	for i, p := range []struct{ starts, ends string }{{"2026-03-31", "2026-04-01"}, {"2026-04-01", "2026-04-02"}} {
		key := fmt.Sprintf("agreement:fx:r%d:p%d", rentalID, i+1)
		var period, entry int64
		f.must(f.pool.QueryRow(ctx, `INSERT INTO rental_billing_periods (organization_id, rental_id, period_no, day_from, day_to, starts_on,
			ends_on, days, amount, accrual_key) VALUES ($1, $2, $3, $4, $5, $6, $7, 1, 1199.99, $8) RETURNING id`,
			f.org, rentalID, i+1, i, i+1, p.starts, p.ends, key).Scan(&period))
		f.must(f.pool.QueryRow(ctx, `INSERT INTO ledger_entries (organization_id, account_id, kind, direction, purpose, amount, entry_date,
			due_date, source_type, rental_id, reservation_id, accrual_key, branch_id, billing_period_id)
			VALUES ($1, $2, 'accrual', 'debit', 'rent', 1199.99, $3, $3::date + 30, 'rental', $4, $5, $6, $7, $8) RETURNING id, uuid`,
			f.org, account, p.starts, rentalID, reservationID, key, f.branch.id, period).Scan(&entry, &entries[i]))
		_, err := f.pool.Exec(ctx, `UPDATE rental_billing_periods SET status = 'posted', ledger_entry_id = $3, posted_at = NOW()
			WHERE organization_id = $1 AND id = $2`, f.org, period, entry)
		f.must(err)
	}

	items, err := f.inv.Items(f.ctx, f.org, model.ItemFilter{RentalUUID: &r.UUID})
	f.must(err)
	if len(items) != 2 || items[0].Amount.String() != "1199.99" || items[1].Amount.String() != "1199.99" || items[0].Kind != model.KindRent {
		t.Fatalf("items %+v", items)
	}
	rentals, _, err := f.inv.UninvoicedRentals(f.ctx, f.org, model.RentalFilter{Limit: 10})
	f.must(err)
	if len(rentals) != 1 || rentals[0].UUID != r.UUID || rentals[0].ItemCount != 2 || rentals[0].OpenAmount.String() != "2399.98" {
		t.Fatalf("uninvoiced rentals %+v", rentals)
	}

	// The first period alone: one rent line of the period, 1 day.
	d, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{RentalUUID: &r.UUID, EntryUUIDs: entries[:1]})
	f.must(err)
	if len(d.Lines) != 1 || d.Lines[0].Kind != model.KindRent || d.Lines[0].Gross.String() != "1199.99" || d.Lines[0].Days != 1 ||
		!strings.Contains(d.Lines[0].Name, "1. dönem 31.03.2026–31.03.2026") || len(d.Links) != 1 || d.Links[0].EntryUUID != entries[0] {
		t.Fatalf("first period draft lines %+v links %+v", d.Lines, d.Links)
	}
	// Never twice: only the second period is left.
	items, err = f.inv.Items(f.ctx, f.org, model.ItemFilter{RentalUUID: &r.UUID})
	f.must(err)
	if len(items) != 1 || items[0].EntryUUID != entries[1] {
		t.Fatalf("items after the first draft %+v", items)
	}
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{RentalUUID: &r.UUID, EntryUUIDs: entries[:1]}); err == nil {
		t.Fatal("the first period was invoiced twice")
	}
	d2 := f.draft(r.UUID)
	if len(d2.Lines) != 1 || d2.Lines[0].Gross.String() != "1199.99" || !strings.Contains(d2.Lines[0].Name, "2. dönem 01.04.2026–01.04.2026") ||
		d2.Linked.String() != "1199.99" {
		t.Fatalf("second period draft lines %+v linked %s", d2.Lines, d2.Linked)
	}
}
