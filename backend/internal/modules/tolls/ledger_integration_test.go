package tolls_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/adapters"
)

type chargeAccrual struct {
	kind     string
	amount   string
	charge   string
	reason   string
	reversed bool
}

// chargeEntries lists the ledger entries posted for the rental's charges.
func (f *fx) chargeEntries(rental uuid.UUID) []chargeAccrual {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(), `
		SELECT e.kind, e.amount::text, ch.amount::text, e.reason,
		       EXISTS (SELECT 1 FROM ledger_entries r WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id)
		FROM ledger_entries e
		JOIN rentals rt ON rt.organization_id = e.organization_id AND rt.id = e.rental_id
		JOIN rental_charges ch ON ch.organization_id = e.organization_id AND ch.id = e.rental_charge_id
		WHERE e.organization_id = $1 AND rt.uuid = $2
		ORDER BY e.id`, f.org, rental)
	f.must(err)
	defer rows.Close()
	var out []chargeAccrual
	for rows.Next() {
		var a chargeAccrual
		f.must(rows.Scan(&a.kind, &a.amount, &a.charge, &a.reason, &a.reversed))
		out = append(out, a)
	}
	f.must(rows.Err())
	return out
}

// TestTollChargesReachTheLedger: a toll charge is accrued on the customer's
// account in the charge transaction, once; its revision (reversal row with
// a negative amount + new charge) reverses that accrual and accrues the new
// amount; the reversal row itself is never accrued, and posting again
// changes nothing.
func TestTollChargesReachTheLedger(t *testing.T) {
	f := newFx(t)
	finance := financeusecase.New(f.pool, f.q, nil)
	finance.SetClock(f.now)
	f.tolls.SetLedger(adapters.Ledger{Finance: finance})

	v, plate := f.vehicle(f.branch)
	r := f.rental(v, f.branch)
	f.advance(2 * time.Hour)
	at := f.now().Add(-time.Hour)

	f.ingest(f.pass("TX-L1", plate, at, "47.00"))
	got := f.chargeEntries(r.UUID)
	if len(got) != 1 || got[0].kind != "accrual" || got[0].amount != "47.00" || got[0].reversed {
		t.Fatalf("after the charge: %+v", got)
	}
	// Same pass again: no new charge, no new posting.
	f.ingest(f.pass("TX-L1", plate, at, "47.00"))
	if again := f.chargeEntries(r.UUID); len(again) != 1 {
		t.Fatalf("duplicate import posted again: %+v", again)
	}

	// Revision: the 47.00 charge is reversed (negative row) and 52.00 charged.
	f.ingest(f.pass("TX-L1", plate, at, "52.00"))
	got = f.chargeEntries(r.UUID)
	if len(got) != 3 {
		t.Fatalf("after the revision: %+v", got)
	}
	if got[0].kind != "accrual" || got[0].amount != "47.00" || !got[0].reversed {
		t.Fatalf("original accrual not reversed: %+v", got[0])
	}
	var accrued52, reversal bool
	for _, e := range got[1:] {
		switch {
		case e.kind == "reversal" && e.amount == "47.00" && e.charge == "47.00" && e.reason == "system:charge_reversed":
			reversal = true
		case e.kind == "accrual" && e.amount == "52.00" && !e.reversed:
			accrued52 = true
		}
		if e.charge == "-47.00" {
			t.Fatalf("the reversal row was accrued: %+v", e)
		}
	}
	if !reversal || !accrued52 {
		t.Fatalf("entries %+v", got)
	}

	// The open toll debt on the account is the revised amount.
	var open string
	f.must(f.pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END), 0)::text
		FROM ledger_entries e JOIN rentals rt ON rt.organization_id = e.organization_id AND rt.id = e.rental_id
		WHERE e.organization_id = $1 AND rt.uuid = $2 AND e.rental_charge_id IS NOT NULL`, f.org, r.UUID).Scan(&open))
	if open != "52.00" {
		t.Fatalf("open toll debt %s, want 52.00", open)
	}

	// Posting again (catch-up, a second call) is a no-op.
	var rentalID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM rentals WHERE organization_id = $1 AND uuid = $2`, f.org, r.UUID).Scan(&rentalID))
	f.must(finance.SyncRental(f.ctx, nil, f.org, rentalID))
	if after := f.chargeEntries(r.UUID); len(after) != 3 {
		t.Fatalf("second sync posted again: %+v", after)
	}
}
