package rentals_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// TestRT_A02_RentalHandoverBranchScope is acceptance RT-A02 (TEC-689) for
// rentals and their handover: a member limited to branch MRK cannot open,
// hand over, return or export a rental of branch HVL (not found, never
// forbidden-with-data), and HVL's operation tasks are absent from MRK's
// task list. Finance, invoices, tolls, KABİS and branches have their own
// branch-scope tests (see docs/acceptance/beta.md).
func TestRT_A02_RentalHandoverBranchScope(t *testing.T) {
	f := newFx(t)
	start := f.now().Add(2 * time.Hour).Truncate(time.Minute)

	// A rental at MRK (fixture default) and one picked up and returned at HVL.
	vm := f.vehicle()
	rm := f.reserve(start, start.Add(24*time.Hour), nil)
	mrkRental, err := f.rentals.Create(f.ctx, f.org, model.CreateInput{ReservationUUID: &rm.UUID, VehicleUUID: &vm.uuid})
	f.must(err)

	f.plate++
	var vh ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Opel', 'Corsa', 2024, $3, $3, 1000) RETURNING id, uuid`,
		f.org, fmt.Sprintf("34 HV %d", f.plate), f.other.id).Scan(&vh.id, &vh.uuid))
	_, err = f.pool.Exec(context.Background(), `INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
		SELECT $1, $2, g.id FROM rental_groups g WHERE g.organization_id = $1 ON CONFLICT DO NOTHING`, f.org, vh.id)
	f.must(err)
	hvl := f.other.uuid
	w := resmodel.Where{PickupBranchUUID: &hvl, ReturnBranchUUID: &hvl, StartsAt: &start}
	end := start.Add(24 * time.Hour)
	w.EndsAt = &end
	q, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{Where: w, Channel: "branch", ProductUUID: &f.product.uuid})
	f.must(err)
	rh, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &q.UUID, CustomerUUID: &f.customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}},
	})
	f.must(err)
	hvlRental, err := f.rentals.Create(f.ctx, f.org, model.CreateInput{ReservationUUID: &rh.UUID, VehicleUUID: &vh.uuid})
	f.must(err)

	// The MRK-only member (same permissions, BranchIDs = [MRK]).
	mrk := orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: "rnt-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms,
		BranchIDs: []int64{f.branch.id},
	})

	// Detail: HVL is not found, MRK opens.
	if _, err := f.rentals.Get(mrk, f.org, hvlRental.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("RT-A02: MRK member opened HVL rental: %v", err)
	}
	if _, err := f.rentals.Get(mrk, f.org, mrkRental.UUID); err != nil {
		t.Fatalf("MRK member cannot open MRK rental: %v", err)
	}
	// Handover (teslim) and return of HVL's rental are refused as not found
	// and change nothing.
	if _, err := f.rentals.Handover(mrk, f.org, hvlRental.UUID, model.HandoverInput{Km: i32(1100), Fuel: i16(80)}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("RT-A02: MRK member handed over HVL rental: %v", err)
	}
	if _, err := f.rentals.Return(mrk, f.org, hvlRental.UUID, model.ReturnInput{Km: i32(1200), Fuel: i16(50)}); err == nil {
		t.Fatal("RT-A02: MRK member returned HVL rental")
	}
	if got, err := f.rentals.Get(f.ctx, f.org, hvlRental.UUID); err != nil || got.Status != model.StatusPreparation {
		t.Fatalf("HVL rental changed by an out-of-scope call: %+v %v", got.Status, err)
	}
	// List and export rows: MRK only.
	for name, list := range map[string]func() ([]model.ListItem, error){
		"list": func() ([]model.ListItem, error) {
			items, _, err := f.rentals.List(mrk, f.org, model.ListFilter{Limit: 50})
			return items, err
		},
		"export": func() ([]model.ListItem, error) { return f.rentals.ExportRows(mrk, f.org, model.ListFilter{}, 1000) },
	} {
		items, err := list()
		f.must(err)
		if len(items) != 1 || items[0].UUID != mrkRental.UUID {
			ids := []uuid.UUID{}
			for _, it := range items {
				ids = append(ids, it.UUID)
			}
			t.Fatalf("RT-A02: %s of the MRK member = %v, want only %s", name, ids, mrkRental.UUID)
		}
	}
	// Operation tasks (handover / preparation) of HVL are invisible.
	tasks, _, err := f.rentals.ListTasks(mrk, f.org, model.TaskFilter{Limit: 100})
	f.must(err)
	for _, tk := range tasks {
		if tk.Rental != nil && tk.Rental.UUID == hvlRental.UUID {
			t.Fatalf("RT-A02: MRK member sees HVL task %s (%s)", tk.UUID, tk.TaskType)
		}
	}
	all, _, err := f.rentals.ListTasks(f.ctx, f.org, model.TaskFilter{RentalUUIDs: []uuid.UUID{hvlRental.UUID}, Limit: 10})
	f.must(err)
	if len(all) == 0 {
		t.Fatal("HVL rental has no tasks: the scope check above is vacuous")
	}
	if _, err := f.rentals.GetTask(mrk, f.org, all[0].UUID); err == nil {
		t.Fatalf("RT-A02: MRK member opened HVL task %s", all[0].UUID)
	}
}
