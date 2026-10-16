package kabis_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/usecase"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis/kabisfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// TestRT_A21_HandoverCreatesOneStartJob: a reservation (and a rental in
// preparation) reports nothing; the actual handover creates exactly one
// rental_start job whose payload comes from the actual data (time, place,
// plate, renter and driver identity / licence); repeating the hook creates
// nothing; the actual return creates one rental_end. The display summary
// never holds a full identity number.
func TestRT_A21_HandoverCreatesOneStartJob(t *testing.T) {
	f := newFx(t, true)
	v, plate := f.vehicle(f.branch)
	_, rental := f.reserve(v, f.branch, f.customer, f.driver)
	if got := f.notifications(rental.UUID); len(got) != 0 {
		t.Fatalf("a reservation / rental in preparation must report nothing: %+v", got)
	}
	out := f.handover(rental.UUID)
	start := f.only(rental.UUID, kabis.EventRentalStart)
	if start.Status != jobsusecase.StatusSucceeded || !start.Official || start.ExternalReference == "" || start.Attempts != 1 {
		t.Fatalf("start notification: %+v", start)
	}
	if start.Plate != plate || !start.OccurredAt.Equal(out.ActualPickupAt.UTC()) || start.Branch.UUID != f.branch.uuid {
		t.Fatalf("start facts: %+v (actual pickup %v)", start, out.ActualPickupAt)
	}
	sent, ok := f.fake.Received(start.UUID.String())
	if !ok {
		t.Fatal("the fake adapter did not receive the notification")
	}
	if sent.Event != kabis.EventRentalStart || sent.Vehicle.Plate != plate || !sent.OccurredAt.Equal(start.OccurredAt) ||
		sent.Place.City != "İstanbul" || sent.Renter.IdentityNumber != f.tckn || len(sent.Drivers) != 1 ||
		sent.Drivers[0].IdentityNumber != f.tckn || sent.Drivers[0].LicenceNumber != f.licence || sent.RentalReference != out.Reference {
		t.Fatalf("payload: %+v", sent)
	}
	d := f.get(start.UUID)
	raw := f.count(`SELECT COUNT(*) FROM kabis_notifications WHERE organization_id = $1 AND summary::text LIKE '%' || $2 || '%'`, f.org, f.tckn)
	if raw != 0 || d.Summary["renter"] == nil || len(d.AttemptLog) != 1 || d.AttemptLog[0].Operation != jobsusecase.OpSubmit {
		t.Fatalf("summary leaks the identity number (%d) or attempt log: %+v", raw, d)
	}
	// The hook again (same rental, same event): nothing new.
	tx, err := f.pool.Begin(context.Background())
	f.must(err)
	var rentalID int64
	f.must(tx.QueryRow(context.Background(), `SELECT id FROM rentals WHERE organization_id = $1 AND uuid = $2`, f.org, rental.UUID).Scan(&rentalID))
	after, err := f.kabis.RecordRentalEvent(f.ctx, db.New(tx), f.org, rentalID, model.CauseHandover)
	f.must(err)
	f.must(tx.Commit(context.Background()))
	if after != nil || len(f.notifications(rental.UUID)) != 1 || f.fake.Reports() != 1 {
		t.Fatalf("a repeated hook must create nothing: %d notifications, %d reports", len(f.notifications(rental.UUID)), f.fake.Reports())
	}
	// The actual return: one end notification.
	_, err = f.rentals.Return(f.ctx, f.org, rental.UUID, rentalmodel.ReturnInput{Km: i32(1300), Fuel: i16(80)})
	f.must(err)
	end := f.only(rental.UUID, kabis.EventRentalEnd)
	if end.Status != jobsusecase.StatusSucceeded || len(f.notifications(rental.UUID)) != 2 {
		t.Fatalf("end notification: %+v", end)
	}
	if got := f.count(`SELECT COUNT(*) FROM integration_jobs WHERE organization_id = $1 AND kind LIKE 'kabis.%'`, f.org); got != 2 {
		t.Fatalf("want 2 KABİS jobs, got %d", got)
	}
}

// TestRT_A21_ReservationAloneCreatesNone: confirmed reservations and a
// rental that is cancelled before the handover never create a job.
func TestRT_A21_ReservationAloneCreatesNone(t *testing.T) {
	f := newFx(t, true)
	v, _ := f.vehicle(f.branch)
	_, rental := f.reserve(v, f.branch, f.customer, f.driver)
	_, err := f.rentals.Cancel(f.ctx, f.org, rental.UUID, "müşteri vazgeçti")
	f.must(err)
	if got := f.count(`SELECT COUNT(*) FROM kabis_notifications WHERE organization_id = $1`, f.org); got != 0 {
		t.Fatalf("no notification without an actual handover, got %d", got)
	}
	if got := f.count(`SELECT COUNT(*) FROM integration_jobs WHERE organization_id = $1 AND kind LIKE 'kabis.%'`, f.org); got != 0 || f.fake.Reports() != 0 {
		t.Fatalf("no job / report without an actual handover: %d jobs, %d reports", got, f.fake.Reports())
	}
}

// TestRT_A21_MissingIdentityBlocks: a driver without an identity number
// blocks the job with the field named; nothing is sent; a retry refuses
// while it is missing and sends once the data is complete.
func TestRT_A21_MissingIdentityBlocks(t *testing.T) {
	f := newFx(t, true)
	customer, driver := f.newCustomer(false)
	v, _ := f.vehicle(f.branch)
	_, rental := f.reserve(v, f.branch, customer, driver)
	f.handover(rental.UUID)
	start := f.only(rental.UUID, kabis.EventRentalStart)
	if start.Status != jobsusecase.StatusBlocked || start.Official || !start.NeedsAction ||
		!model.Contains(start.MissingFields, "drivers.0.identity_number") || f.fake.Reports() != 0 {
		t.Fatalf("blocked notification: %+v (reports %d)", start, f.fake.Reports())
	}
	if !strings.Contains(start.LastError, "drivers.0.identity_number") {
		t.Fatalf("the job error names the field: %q", start.LastError)
	}
	_, err := f.kabis.Retry(f.ctx, f.org, start.UUID)
	var dm *usecase.DataMissingError
	if !errors.As(err, &dm) || !model.Contains(dm.Fields, "drivers.0.identity_number") || f.fake.Reports() != 0 {
		t.Fatalf("retry while missing: %v", err)
	}
	// Staff complete the driver's identity: the retry re-validates and sends.
	_, err = f.pool.Exec(context.Background(), `UPDATE drivers SET identity_type = 'tckn', identity_enc = $3, identity_bidx = $4
		WHERE organization_id = $1 AND id = $2`, f.org, driver.id, f.enc("drivers.identity", f.tckn), uuid.NewString())
	f.must(err)
	res, err := f.kabis.Retry(f.ctx, f.org, start.UUID)
	f.must(err)
	if res.Action != "revalidated" || res.Status != jobsusecase.StatusSucceeded || f.fake.Reports() != 1 {
		t.Fatalf("retry after completing the data: %+v (reports %d)", res, f.fake.Reports())
	}
	if d := f.get(start.UUID); len(d.MissingFields) != 0 || !d.Official {
		t.Fatalf("after the retry: %+v", d.Notification)
	}
}

// TestRT_A21_OutageKeepsPendingRetryable: a connection loss before the
// request left keeps the job pending (visible, with its error, never
// "done"); a staff retry sends it.
func TestRT_A21_OutageKeepsPendingRetryable(t *testing.T) {
	f := newFx(t, true)
	f.fake.FailNextReport(kabis.ErrUnavailable, false)
	v, _ := f.vehicle(f.branch)
	_, rental := f.reserve(v, f.branch, f.customer, f.driver)
	f.handover(rental.UUID)
	start := f.only(rental.UUID, kabis.EventRentalStart)
	if start.Status != jobsusecase.StatusPending || start.Official || start.Attempts != 1 || start.LastError == "" || start.NextAttemptAt == nil {
		t.Fatalf("after the outage: %+v", start)
	}
	if _, ok := f.fake.Received(start.UUID.String()); ok {
		t.Fatal("nothing may be recorded during the outage")
	}
	// It stays visible in the pending filter.
	items, _, err := f.kabis.List(f.ctx, f.org, model.ListFilter{Statuses: []string{"pending"}, Limit: 10})
	f.must(err)
	if len(items) != 1 || items[0].UUID != start.UUID {
		t.Fatalf("pending list: %+v", items)
	}
	res, err := f.kabis.Retry(f.ctx, f.org, start.UUID)
	f.must(err)
	if res.Action != "resubmitted" || res.Status != jobsusecase.StatusSucceeded || f.fake.Reports() != 2 {
		t.Fatalf("retry: %+v (reports %d)", res, f.fake.Reports())
	}
}

// TestRT_A21_UncertainChecksStatusFirst: a timeout after sending is
// unknown: the job goes to needs_verification; the next step and a staff
// retry check the status first and never send again while it is unknown;
// a found record ends it succeeded with exactly one report. A record the
// authority confirms it does not have is sent once more.
func TestRT_A21_UncertainChecksStatusFirst(t *testing.T) {
	f := newFx(t, true)
	f.fake.FailNextReport(kabis.ErrTimeout, true)
	f.fake.FailNextLookup(kabis.ErrUnavailable, kabis.ErrUnavailable)
	v, _ := f.vehicle(f.branch)
	_, rental := f.reserve(v, f.branch, f.customer, f.driver)
	f.handover(rental.UUID)
	start := f.only(rental.UUID, kabis.EventRentalStart)
	if start.Status != jobsusecase.StatusNeedsVerification || start.Official || f.fake.Reports() != 1 {
		t.Fatalf("after the timeout: %+v", start)
	}
	// Sweep step: the status check is still unclear → needs_verification.
	f.step(start)
	d := f.get(start.UUID)
	if d.Status != jobsusecase.StatusNeedsVerification || d.LastVerifiedAt == nil || f.fake.Reports() != 1 || f.fake.Lookups() != 1 {
		t.Fatalf("after an unclear check: %+v (reports %d lookups %d)", d.Notification, f.fake.Reports(), f.fake.Lookups())
	}
	// Staff retry: unclear again → refused, nothing sent.
	_, err := f.kabis.Retry(f.ctx, f.org, start.UUID)
	var ve *jobsusecase.VerifyError
	if !errors.As(err, &ve) || f.fake.Reports() != 1 {
		t.Fatalf("retry while unknown: %v (reports %d)", err, f.fake.Reports())
	}
	// The authority answers now: it has the record → succeeded, one report.
	res, err := f.kabis.Retry(f.ctx, f.org, start.UUID)
	f.must(err)
	d = f.get(start.UUID)
	if res.Action != "already_done" || d.Status != jobsusecase.StatusSucceeded || d.ExternalReference == "" || f.fake.Reports() != 1 {
		t.Fatalf("retry after the authority answers: %+v %+v (reports %d)", res, d.Notification, f.fake.Reports())
	}
	ops := []string{}
	for _, a := range d.AttemptLog {
		ops = append(ops, a.Operation+":"+a.Outcome)
	}
	if strings.Join(ops, ",") != "submit:uncertain,check_status:uncertain,check_status:uncertain,check_status:succeeded" {
		t.Fatalf("attempt history: %v", ops)
	}

	// Another rental: the timeout left nothing at the authority → the check
	// confirms "not found" → sent again once.
	f.fake.FailNextReport(kabis.ErrTimeout, false)
	v2, _ := f.vehicle(f.branch)
	_, rental2 := f.reserve(v2, f.branch, f.customer, f.driver)
	f.handover(rental2.UUID)
	second := f.only(rental2.UUID, kabis.EventRentalStart)
	before := f.fake.Reports()
	f.step(second) // check: not found → pending
	if got := f.get(second.UUID); got.Status != jobsusecase.StatusPending || f.fake.Reports() != before {
		t.Fatalf("after not found: %+v", got.Notification)
	}
	f.step(second) // submit again
	if got := f.get(second.UUID); got.Status != jobsusecase.StatusSucceeded || f.fake.Reports() != before+1 {
		t.Fatalf("resubmitted: %+v (reports %d → %d)", got.Notification, before, f.fake.Reports())
	}
}

// TestRT_A21_ManualReconcile: a person marks a notification "reconciled"
// (needs verification) or "reported manually" (waiting, with an
// attachment); reason and reference are required; the job ends and is
// never sent again.
func TestRT_A21_ManualReconcile(t *testing.T) {
	f := newFx(t, true)
	f.fake.FailNextReport(kabis.ErrTimeout, false)
	v, _ := f.vehicle(f.branch)
	_, rental := f.reserve(v, f.branch, f.customer, f.driver)
	f.handover(rental.UUID)
	start := f.only(rental.UUID, kabis.EventRentalStart)

	_, err := f.kabis.Resolve(f.ctx, f.org, start.UUID, model.ResolutionReconciled, model.ResolveInput{Reason: "x", Reference: ""})
	var ve *usecase.ValidationError
	if !errors.As(err, &ve) || len(ve.Details) != 2 {
		t.Fatalf("reason and reference are required: %v", err)
	}
	d, err := f.kabis.Resolve(f.ctx, f.org, start.UUID, model.ResolutionReconciled, model.ResolveInput{
		Reason: "KABİS ekranında kayıt görüldü", Reference: "KBS-2026-001",
	})
	f.must(err)
	if d.Status != jobsusecase.StatusReconciled || !d.Official || d.Resolution == nil || *d.Resolution != model.ResolutionReconciled ||
		d.ResolutionReference != "KBS-2026-001" || d.ResolvedAt == nil {
		t.Fatalf("reconciled: %+v", d.Notification)
	}
	if _, err := f.kabis.Retry(f.ctx, f.org, start.UUID); !errors.Is(err, usecase.ErrNotRetryable) {
		t.Fatalf("a reconciled notification is final: %v", err)
	}
	reports := f.fake.Reports()
	f.step(start)
	if f.fake.Reports() != reports {
		t.Fatal("a reconciled job must never be sent")
	}

	// Waiting notification of an organization without an account: reported
	// through the manual channel, with the receipt attached.
	g := newFx(t, false)
	gv, _ := g.vehicle(g.branch)
	_, grental := g.reserve(gv, g.branch, g.customer, g.driver)
	g.handover(grental.UUID)
	wait := g.only(grental.UUID, kabis.EventRentalStart)
	if _, err := g.kabis.Resolve(g.ctx, g.org, wait.UUID, model.ResolutionReconciled, model.ResolveInput{
		Reason: "kontrol edildi", Reference: "X1",
	}); !errors.Is(err, usecase.ErrNotResolvable) {
		t.Fatalf("a waiting notification cannot be reconciled (nothing was sent): %v", err)
	}
	pdf := []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")
	d, err = g.kabis.Resolve(g.ctx, g.org, wait.UUID, model.ResolutionManual, model.ResolveInput{
		Reason: "KABİS web ekranından elle bildirildi (kesinti prosedürü)", Reference: "KBS-EL-42",
		File: &model.Upload{Body: bytes.NewReader(pdf), Size: int64(len(pdf)), Filename: "../makbuz.pdf"},
	})
	g.must(err)
	if d.Status != jobsusecase.StatusReportedManually || !d.Official || d.Attachment == nil || d.Attachment.Name != "makbuz.pdf" ||
		d.Attachment.ContentType != "application/pdf" {
		t.Fatalf("reported manually: %+v %+v", d.Notification, d.Attachment)
	}
	file, err := g.kabis.OpenAttachment(g.ctx, g.org, wait.UUID)
	g.must(err)
	got, _ := io.ReadAll(file.Body)
	_ = file.Body.Close()
	if !bytes.Equal(got, pdf) {
		t.Fatal("attachment bytes differ")
	}
	// Another organization cannot read the attachment.
	if _, err := f.kabis.OpenAttachment(f.ctx, f.org, wait.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross-organization attachment: %v", err)
	}
}

// TestRT_A21_WaitingDependency: without a working adapter the job waits
// visibly (never done); the official slot keeps it waiting (method not
// verified, TEC-682); a working adapter releases it.
func TestRT_A21_WaitingDependency(t *testing.T) {
	f := newFx(t, false)
	v, _ := f.vehicle(f.branch)
	_, rental := f.reserve(v, f.branch, f.customer, f.driver)
	f.handover(rental.UUID)
	start := f.only(rental.UUID, kabis.EventRentalStart)
	if start.Status != jobsusecase.StatusWaitingDependency || start.Official || start.HoldReason != model.HoldNoAccount ||
		start.Provider != kabis.ProviderOfficial || f.fake.Reports() != 0 {
		t.Fatalf("no account: %+v", start)
	}
	facets, err := f.kabis.Facets(f.ctx, f.org)
	f.must(err)
	if facets.NeedsAction != 1 || facets.Statuses[jobsusecase.StatusWaitingDependency] != 1 {
		t.Fatalf("facets: %+v", facets)
	}
	// The official adapter slot: still waiting.
	official := f.connect(kabis.ProviderOfficial)
	if n, err := f.kabis.ReleaseWaiting(context.Background()); err != nil || f.get(start.UUID).Status != jobsusecase.StatusWaitingDependency {
		t.Fatalf("release with the official slot: %d %v", n, err)
	}
	_, err = f.kabis.Retry(f.ctx, f.org, start.UUID)
	var de *usecase.DependencyError
	if !errors.As(err, &de) || de.Reason != model.HoldAdapterUnverified || f.fake.Reports() != 0 {
		t.Fatalf("retry on the official slot: %v", err)
	}
	if got := f.get(start.UUID); got.HoldReason != model.HoldAdapterUnverified {
		t.Fatalf("hold reason: %+v", got.Notification)
	}
	// A working adapter: the sweep releases it.
	_, err = f.accounts.SetStatus(f.ctx, f.org, official, "disabled")
	f.must(err)
	f.connect(kabisfake.ProviderCode)
	n, err := f.kabis.ReleaseWaiting(context.Background())
	f.must(err)
	got := f.get(start.UUID)
	if n < 1 || got.Status != jobsusecase.StatusSucceeded || got.Provider != kabisfake.ProviderCode || f.fake.Reports() != 1 {
		t.Fatalf("released: %d %+v", n, got.Notification)
	}
}

// TestKabisCorrections: a driver change after a reported start makes a
// correction (once; an unchanged save makes none); a vehicle swap too. A
// start that was not sent yet takes the change in place.
func TestKabisCorrections(t *testing.T) {
	f := newFx(t, true)
	v, _ := f.vehicle(f.branch)
	res, rental := f.reserve(v, f.branch, f.customer, f.driver)
	f.handover(rental.UUID)
	start := f.only(rental.UUID, kabis.EventRentalStart)
	// The same drivers again: nothing.
	_, err := f.res.UpdateDrivers(f.ctx, f.org, res.UUID, resmodel.DriversInput{Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}}})
	f.must(err)
	if got := len(f.notifications(rental.UUID)); got != 1 {
		t.Fatalf("unchanged drivers made %d notifications", got)
	}
	// A second driver: a correction of the start.
	second := ref{}
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date,
		identity_type, identity_enc, identity_bidx, identity_hint, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, 'Ece', 'Kaya', $3, 'tckn', $4, $5, '***', $6, $7, '***', $8) RETURNING id, uuid`,
		f.org, f.customer.id, time.Now().AddDate(-28, 0, 0), f.enc("drivers.identity", "10000000146"), uuid.NewString(),
		f.enc("drivers.licence", "L-2"), uuid.NewString(), time.Now().AddDate(-5, 0, 0)).Scan(&second.id, &second.uuid))
	_, err = f.res.UpdateDrivers(f.ctx, f.org, res.UUID, resmodel.DriversInput{Drivers: []resmodel.DriverInput{
		{DriverUUID: f.driver.uuid}, {DriverUUID: second.uuid, Role: "additional"},
	}})
	f.must(err)
	corr := f.only(rental.UUID, kabis.EventCorrection)
	d := f.get(corr.UUID)
	if d.CorrectsUUID == nil || *d.CorrectsUUID != start.UUID || corr.Cause != model.CauseDriversChanged || corr.Status != jobsusecase.StatusSucceeded {
		t.Fatalf("correction: %+v", d.Notification)
	}
	sent, _ := f.fake.Received(corr.UUID.String())
	if sent.Corrects != start.UUID.String() || len(sent.Drivers) != 2 {
		t.Fatalf("correction payload: %+v", sent)
	}
	// A vehicle swap: another correction naming the new plate.
	v2, plate2 := f.vehicle(f.branch)
	_, err = f.rentals.Swap(f.ctx, f.org, rental.UUID, rentalmodel.SwapInput{
		VehicleUUID: &v2.uuid, InKm: i32(1200), InFuel: i16(70), OutKm: i32(1000), OutFuel: i16(100), Reason: "arıza değişimi",
	})
	f.must(err)
	var swaps []model.Notification
	for _, n := range f.notifications(rental.UUID) {
		if n.Event == kabis.EventCorrection && n.Cause == model.CauseVehicleSwap {
			swaps = append(swaps, n)
		}
	}
	if len(swaps) != 1 || swaps[0].Plate != plate2 || swaps[0].Sequence != 2 {
		t.Fatalf("swap correction: %+v", swaps)
	}

	// Not sent yet (waiting): the change updates the start in place.
	g := newFx(t, false)
	gv, _ := g.vehicle(g.branch)
	gres, grental := g.reserve(gv, g.branch, g.customer, g.driver)
	g.handover(grental.UUID)
	_, gsecond := g.newCustomer(true)
	_, err = g.res.UpdateDrivers(g.ctx, g.org, gres.UUID, resmodel.DriversInput{Drivers: []resmodel.DriverInput{
		{DriverUUID: g.driver.uuid}, {DriverUUID: gsecond.uuid, Role: "additional"},
	}})
	g.must(err)
	all := g.notifications(grental.UUID)
	if len(all) != 1 || all[0].Event != kabis.EventRentalStart {
		t.Fatalf("an unsent start is updated in place: %+v", all)
	}
	if drivers, _ := g.get(all[0].UUID).Summary["drivers"].([]any); len(drivers) != 2 {
		t.Fatalf("updated summary: %+v", g.get(all[0].UUID).Summary)
	}
}

// TestRT_A21_BranchScope: a member of another branch sees nothing of a
// rental of branch A (list, facets) and gets 404 for it (get, retry,
// resolve); a member of branch A sees it.
func TestRT_A21_BranchScope(t *testing.T) {
	f := newFx(t, false)
	v, _ := f.vehicle(f.branch)
	_, rental := f.reserve(v, f.branch, f.customer, f.driver)
	f.handover(rental.UUID)
	start := f.only(rental.UUID, kabis.EventRentalStart)
	scoped := func(branch int64) context.Context {
		return orgctx.WithScope(context.Background(), orgctx.Scope{
			ID: f.org, UUID: f.orgUUID, Status: orgctx.StatusActive, Permissions: perms, BranchIDs: []int64{branch},
		})
	}
	other := scoped(f.other.id)
	items, total, err := f.kabis.List(other, f.org, model.ListFilter{Limit: 10})
	f.must(err)
	if len(items) != 0 || total != 0 {
		t.Fatalf("other branch sees %d", len(items))
	}
	facets, err := f.kabis.Facets(other, f.org)
	f.must(err)
	if facets.Total != 0 || facets.NeedsAction != 0 {
		t.Fatalf("other branch facets: %+v", facets)
	}
	if _, err := f.kabis.Get(other, f.org, start.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("get from another branch: %v", err)
	}
	if _, err := f.kabis.Retry(other, f.org, start.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("retry from another branch: %v", err)
	}
	if _, err := f.kabis.Resolve(other, f.org, start.UUID, model.ResolutionManual, model.ResolveInput{Reason: "elle", Reference: "R"}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("resolve from another branch: %v", err)
	}
	own := scoped(f.branch.id)
	items, _, err = f.kabis.List(own, f.org, model.ListFilter{Limit: 10})
	f.must(err)
	if len(items) != 1 {
		t.Fatalf("own branch sees %d", len(items))
	}
	if _, err := f.kabis.Get(own, f.org, start.UUID); err != nil {
		t.Fatal(err)
	}
}

// TestRT_A21_CrossOrganization: another organization's notification is
// not found anywhere; lists never mix organizations.
func TestRT_A21_CrossOrganization(t *testing.T) {
	a := newFx(t, true)
	b := newFx(t, true)
	va, _ := a.vehicle(a.branch)
	_, ra := a.reserve(va, a.branch, a.customer, a.driver)
	a.handover(ra.UUID)
	vb, _ := b.vehicle(b.branch)
	_, rb := b.reserve(vb, b.branch, b.customer, b.driver)
	b.handover(rb.UUID)
	nb := b.only(rb.UUID, kabis.EventRentalStart)
	if _, err := a.kabis.Get(a.ctx, a.org, nb.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	if _, err := a.kabis.Retry(a.ctx, a.org, nb.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("retry: %v", err)
	}
	if _, err := a.kabis.Resolve(a.ctx, a.org, nb.UUID, model.ResolutionManual, model.ResolveInput{Reason: "yabancı", Reference: "R"}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("resolve: %v", err)
	}
	items, _, err := a.kabis.List(a.ctx, a.org, model.ListFilter{Limit: 50})
	a.must(err)
	for _, it := range items {
		if it.UUID == nb.UUID {
			t.Fatal("organization A lists B's notification")
		}
	}
	// A's rental filter with B's rental uuid: nothing.
	items, _, err = a.kabis.List(a.ctx, a.org, model.ListFilter{RentalUUID: &rb.UUID, Limit: 50})
	a.must(err)
	if len(items) != 0 {
		t.Fatalf("rental filter leaks: %+v", items)
	}
	// B's record is unchanged.
	if got := b.get(nb.UUID); got.Status != jobsusecase.StatusSucceeded {
		t.Fatalf("B's notification: %+v", got.Notification)
	}
}
