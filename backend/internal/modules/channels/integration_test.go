package channels_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

func statusOf(msgs []model.Message, key string) string {
	for _, m := range msgs {
		if m.MessageKey == key {
			return m.Status
		}
	}
	return ""
}

// RT-A25: booking + modification + cancellation with repeated and
// out-of-order messages → one external reference, one reservation;
// capacity, price difference and the channel's money stay consistent.
func TestRT_A25_BookingModificationCancellationWithRepeats(t *testing.T) {
	f := newFx(t)
	f.vehicle("GEKO")
	f.vehicle("GEKO")
	start, end := window(3, 2)
	if got := f.sellable(start, end); got != 2 {
		t.Fatalf("sellable before = %d, want 2", got)
	}
	acc := f.fakeAccount()
	ref := "Y360-RTA25-" + f.suffix

	// Booking v1, delivered twice with the same id and once more under
	// another id (same version).
	v1 := booking("E1-"+f.suffix, ref, 1, channels.StatusConfirmed, start, end, "2000", "300")
	again := v1
	again.ID = "E1b-" + f.suffix
	f.fake.Enqueue(acc.UUID, v1, v1, again)
	run := f.pull()
	if run.Status != model.RunSucceeded || run.ItemsTotal != 3 {
		t.Fatalf("pull run = %+v", run)
	}
	list := f.reservationsOf(ref)
	if len(list) != 1 {
		t.Fatalf("reservations after repeats = %d, want 1", len(list))
	}
	r := list[0]
	if r.Status != "confirmed" || r.CollectingParty != "channel" || r.Channel != "yolcu360" || r.ApprovalProtocol == "" {
		t.Fatalf("reservation = status %s collecting %s channel %s", r.Status, r.CollectingParty, r.Channel)
	}
	if !r.Price.Total.Equal(money.MustParse("2000")) || !r.Price.Commission.Equal(money.MustParse("300")) ||
		!r.Price.NetReceivable.Equal(money.MustParse("1700")) {
		t.Fatalf("price = total %s commission %s net %s", r.Price.Total, r.Price.Commission, r.Price.NetReceivable)
	}
	// Collected by the channel: the customer is never asked to pay again.
	if !r.Price.PrepaymentDue.IsZero() || !r.Price.Balance.IsZero() {
		t.Fatalf("channel-collected reservation asks for money: due %s balance %s", r.Price.PrepaymentDue, r.Price.Balance)
	}
	msgs := f.messages(ref)
	if len(msgs) != 2 || statusOf(msgs, v1.ID) != model.MessageApplied || statusOf(msgs, again.ID) != model.MessageDuplicate {
		t.Fatalf("messages = %+v", msgs)
	}
	for _, m := range msgs {
		if m.MessageKey == v1.ID && m.Deliveries != 2 {
			t.Fatalf("deliveries of the repeated id = %d, want 2", m.Deliveries)
		}
	}
	if got := f.sellable(start, end); got != 1 {
		t.Fatalf("sellable after the booking = %d, want 1", got)
	}

	// Modification v2: one day longer at the channel's new total; v2 again
	// under a new id and v1 once more arrive afterwards.
	newEnd := end.Add(24 * time.Hour)
	v2 := booking("E2-"+f.suffix, ref, 2, channels.StatusConfirmed, start, newEnd, "3000", "450")
	v2again := v2
	v2again.ID = "E2b-" + f.suffix
	v1late := v1
	v1late.ID = "E1c-" + f.suffix
	f.fake.Enqueue(acc.UUID, v2, v2again, v1late)
	f.pull()
	list = f.reservationsOf(ref)
	if len(list) != 1 || list[0].UUID != r.UUID {
		t.Fatalf("modification created another reservation: %d", len(list))
	}
	r = list[0]
	if !r.EndsAt.Equal(newEnd) || !r.Price.Total.Equal(money.MustParse("3000")) || !r.Price.Commission.Equal(money.MustParse("450")) ||
		!r.Price.NetReceivable.Equal(money.MustParse("2550")) {
		t.Fatalf("after modification: ends %s total %s commission %s net %s", r.EndsAt, r.Price.Total, r.Price.Commission, r.Price.NetReceivable)
	}
	msgs = f.messages(ref)
	if statusOf(msgs, v2.ID) != model.MessageApplied || statusOf(msgs, v2again.ID) != model.MessageDuplicate ||
		statusOf(msgs, v1late.ID) != model.MessageStale {
		t.Fatalf("messages after modification = %+v", msgs)
	}
	d, err := f.svc.GetMessage(f.ctx, f.org, msgs[len(msgs)-1].UUID)
	f.must(err)
	if d.Booking == nil || d.Booking.AppliedVersion != 2 || d.Booking.PriceDifference == nil || !d.Booking.PriceDifference.IsZero() {
		t.Fatalf("booking after modification = %+v", d.Booking)
	}

	// Cancellation v3, then the modification v2 arrives once more (late).
	v3 := booking("E3-"+f.suffix, ref, 3, channels.StatusCancelled, start, newEnd, "3000", "450")
	v2late := v2
	v2late.ID = "E2c-" + f.suffix
	f.fake.Enqueue(acc.UUID, v3, v2late)
	f.pull()
	list = f.reservationsOf(ref)
	if len(list) != 1 || list[0].Status != "cancelled" {
		t.Fatalf("after cancellation: %d reservations, status %s", len(list), list[0].Status)
	}
	if got := f.sellable(start, end); got != 2 {
		t.Fatalf("sellable after the cancellation = %d, want 2 (released once)", got)
	}
	msgs = f.messages(ref)
	if statusOf(msgs, v3.ID) != model.MessageApplied || statusOf(msgs, v2late.ID) != model.MessageStale {
		t.Fatalf("messages after cancellation = %+v", msgs)
	}
	if n := f.count(`SELECT COUNT(*) FROM channel_reservations WHERE organization_id = $1 AND external_ref = $2`, f.org, ref); n != 1 {
		t.Fatalf("channel bookings = %d, want 1", n)
	}
	// No payment was ever created for the booking.
	if n := f.count(`SELECT COUNT(*) FROM payments WHERE organization_id = $1`, f.org); n != 0 {
		t.Fatalf("payments = %d, want 0", n)
	}
	// Every final result was acknowledged, the accepted ones with our
	// reference.
	acks := f.fake.Acks(acc.UUID)
	if len(acks) != 7 {
		t.Fatalf("acks = %d, want 7: %+v", len(acks), acks)
	}
	for _, a := range acks {
		if !a.Accepted || a.Reference != r.Reference {
			t.Fatalf("ack = %+v (reference %s)", a, r.Reference)
		}
	}
}

// A cancellation that arrives before its booking leaves a tombstone: the
// late booking is stale and creates nothing.
func TestChannelCancellationBeforeBooking(t *testing.T) {
	f := newFx(t)
	f.vehicle("GEKO")
	start, end := window(4, 1)
	acc := f.fakeAccount()
	ref := "Y360-OOO-" + f.suffix
	f.fake.Enqueue(acc.UUID,
		booking("C2-"+f.suffix, ref, 2, channels.StatusCancelled, start, end, "1000", "0"),
		booking("B1-"+f.suffix, ref, 1, channels.StatusConfirmed, start, end, "1000", "0"))
	f.pull()
	if n := len(f.reservationsOf(ref)); n != 0 {
		t.Fatalf("reservations = %d, want 0", n)
	}
	msgs := f.messages(ref)
	if statusOf(msgs, "C2-"+f.suffix) != model.MessageApplied || statusOf(msgs, "B1-"+f.suffix) != model.MessageStale {
		t.Fatalf("messages = %+v", msgs)
	}
	if got := f.sellable(start, end); got != 1 {
		t.Fatalf("sellable = %d, want 1", got)
	}
}

// Unmappable messages wait in the error queue (not acknowledged); a retry
// after the mapping is fixed applies them, a dismissal tells the channel.
func TestChannelErrorQueueRetryAndIgnore(t *testing.T) {
	f := newFx(t)
	f.vehicle("GEKO")
	f.vehicle("GEKO")
	start, end := window(5, 2)
	acc := f.fakeAccount()
	unknown := booking("U1-"+f.suffix, "Y360-UNK-"+f.suffix, 1, channels.StatusConfirmed, start, end, "2000", "0")
	unknown.ProductCode = "Y-SUV"
	badLoc := booking("L1-"+f.suffix, "Y360-LOC-"+f.suffix, 1, channels.StatusConfirmed, start, end, "2000", "0")
	badLoc.PickupLocationCode = "ESB"
	f.fake.Enqueue(acc.UUID, unknown, badLoc)
	run := f.pull()
	if run.Status != model.RunPartial || run.ItemsFailed != 2 {
		t.Fatalf("run = %+v", run)
	}
	facets, err := f.svc.MessageFacets(f.ctx, f.org)
	f.must(err)
	if facets.NeedsAction != 2 {
		t.Fatalf("needs action = %d", facets.NeedsAction)
	}
	um := f.messages(unknown.ExternalRef)[0]
	if um.ErrorCode != model.ErrCodeUnknownProduct || um.AckedAt != nil {
		t.Fatalf("unknown product message = %+v", um)
	}
	if lm := f.messages(badLoc.ExternalRef)[0]; lm.ErrorCode != model.ErrCodeUnknownLocation {
		t.Fatalf("unknown location message = %+v", lm)
	}
	if len(f.fake.Acks(acc.UUID)) != 0 {
		t.Fatal("failed messages must not be acknowledged")
	}
	// A retry before the fix stays failed.
	d, err := f.svc.RetryMessage(f.ctx, f.org, um.UUID)
	f.must(err)
	if d.Status != model.MessageFailed || d.Attempts != 2 {
		t.Fatalf("retry before the fix = %s attempts %d", d.Status, d.Attempts)
	}
	f.mapping(model.MappingInput{EntityType: model.EntityProduct, ProductUUID: &f.product.uuid, ExternalCode: sp("Y-SUV"),
		BranchUUID: model.OptionalUUID{Set: true, Value: &f.branch.uuid}})
	d, err = f.svc.RetryMessage(f.ctx, f.org, um.UUID)
	f.must(err)
	if d.Status != model.MessageApplied || d.Reservation == nil {
		t.Fatalf("retry after the fix = %+v", d)
	}
	if _, err := f.svc.RetryMessage(f.ctx, f.org, um.UUID); !errors.Is(err, usecase.ErrNotRetryable) {
		t.Fatalf("retry of an applied message = %v", err)
	}
	lm := f.messages(badLoc.ExternalRef)[0]
	if _, err := f.svc.IgnoreMessage(f.ctx, f.org, lm.UUID, "x"); err == nil {
		t.Fatal("ignore without a note must fail")
	}
	d, err = f.svc.IgnoreMessage(f.ctx, f.org, lm.UUID, "Lokasyon sözleşmede yok")
	f.must(err)
	if d.Status != model.MessageIgnored {
		t.Fatalf("ignored = %s", d.Status)
	}
	acks := f.fake.Acks(acc.UUID)
	if len(acks) != 2 {
		t.Fatalf("acks = %+v", acks)
	}
	for _, a := range acks {
		if a.MessageID == lm.MessageKey && (a.Accepted || a.Code != model.ErrCodeUnknownLocation) {
			t.Fatalf("dismissal ack = %+v", a)
		}
		if a.MessageID == um.MessageKey && !a.Accepted {
			t.Fatalf("applied ack = %+v", a)
		}
	}
}

// No capacity: the booking fails with capacity_unavailable and holds
// nothing.
func TestChannelBookingWithoutCapacity(t *testing.T) {
	f := newFx(t)
	start, end := window(6, 1)
	acc := f.fakeAccount()
	m := booking("N1-"+f.suffix, "Y360-NOCAP-"+f.suffix, 1, channels.StatusConfirmed, start, end, "1000", "0")
	f.fake.Enqueue(acc.UUID, m)
	f.pull()
	msg := f.messages(m.ExternalRef)[0]
	if msg.Status != model.MessageFailed || msg.ErrorCode != model.ErrCodeCapacity {
		t.Fatalf("message = %+v", msg)
	}
	if n := len(f.reservationsOf(m.ExternalRef)); n != 0 {
		t.Fatalf("reservations = %d", n)
	}
}

// Push: availability per day with quota, stop-sale and the offline policy;
// a failed push is visible on the account.
func TestChannelPushInventory(t *testing.T) {
	f := newFx(t)
	f.vehicle("GEKO")
	f.vehicle("GEKO")
	f.vehicle("GEKO")
	_, err := f.svc.UpdateSettings(f.ctx, f.org, f.account, model.SettingsInput{HorizonDays: i32(5), OfflinePolicy: sp("quota"), OfflineQuota: i32(1)})
	f.must(err)
	run := f.push()
	if run.Status != model.RunSucceeded || run.ItemsTotal != 5 {
		t.Fatalf("push run = %+v", run)
	}
	last, n := f.fake.LastPush(f.account)
	if last == nil || n != 1 {
		t.Fatalf("pushes = %d", n)
	}
	if last.Offline.Mode != channels.OfflineQuota || last.Offline.Quota != 1 || !last.ValidUntil.After(time.Now()) {
		t.Fatalf("offline policy = %+v valid until %s", last.Offline, last.ValidUntil)
	}
	tomorrow := last.Items[1]
	if tomorrow.ProductCode != "Y-ECO" || tomorrow.LocationCode != "IST" || tomorrow.Available != 3 || !tomorrow.HasPrice ||
		!tomorrow.DailyPrice.Equal(money.MustParse("1000")) || tomorrow.OfferRef == "" || tomorrow.StopSale {
		t.Fatalf("item = %+v", tomorrow)
	}
	// Quota cap, then stop-sale on the mapping.
	mappings, _, err := f.svc.ListMappings(f.ctx, f.org, model.MappingFilter{EntityTypes: []string{model.EntityProduct}, Limit: 10})
	f.must(err)
	pm := mappings[0]
	_, err = f.svc.UpdateMapping(f.ctx, f.org, pm.UUID, model.MappingInput{MaxQuota: model.OptionalInt{Set: true, Value: i32(1)}})
	f.must(err)
	f.push()
	last, _ = f.fake.LastPush(f.account)
	if last.Items[1].Available != 1 {
		t.Fatalf("quota item = %+v", last.Items[1])
	}
	_, err = f.svc.UpdateMapping(f.ctx, f.org, pm.UUID, model.MappingInput{StopSale: bp(true)})
	f.must(err)
	f.push()
	last, _ = f.fake.LastPush(f.account)
	if last.Items[1].Available != 0 || !last.Items[1].StopSale {
		t.Fatalf("stop-sale item = %+v", last.Items[1])
	}
	// A channel outage: the run fails, the account shows it.
	f.fake.FailNextPush(channels.ErrUnavailable)
	run = f.push()
	if run.Status != model.RunFailed {
		t.Fatalf("failed push run = %+v", run)
	}
	a, err := f.svc.GetAccount(f.ctx, f.org, f.account)
	f.must(err)
	if a.Sync.Health != model.HealthFailing || a.Sync.ConsecutiveFailures != 1 || a.Sync.PushedValidUntil == nil {
		t.Fatalf("account sync = %+v", a.Sync)
	}
}

// Several products on one external code: the push offers the first by
// priority with capacity, and a booking resolves its product from the
// offer it echoes (or the offer pushed for that day), never from the name.
func TestChannelSharedCodeResolvedByOffer(t *testing.T) {
	f := newFx(t)
	other := f.newProduct("KMP", 1500)
	f.vehicle("GKMP")
	f.mapping(model.MappingInput{EntityType: model.EntityProduct, ProductUUID: &other.uuid, ExternalCode: sp("Y-ECO"), Priority: i32(-1)})
	_, err := f.svc.UpdateSettings(f.ctx, f.org, f.account, model.SettingsInput{HorizonDays: i32(10)})
	f.must(err)
	f.push()
	last, _ := f.fake.LastPush(f.account)
	item := last.Items[6]
	// Only the KMP product has a vehicle: it is offered.
	if item.Available != 1 || !item.DailyPrice.Equal(money.MustParse("1500")) {
		t.Fatalf("shared code item = %+v", item)
	}
	start, end := window(6, 1)
	acc := f.fakeAccount()
	m := booking("S1-"+f.suffix, "Y360-SHR-"+f.suffix, 1, channels.StatusConfirmed, start, end, "1500", "0")
	m.OfferRef = item.OfferRef
	f.fake.Enqueue(acc.UUID, m)
	f.pull()
	list := f.reservationsOf(m.ExternalRef)
	if len(list) != 1 || list[0].Plan.SoldProduct.UUID != other.uuid {
		t.Fatalf("booking product = %+v", list)
	}
	// Without an offer reference the offer pushed for the day decides.
	m2 := booking("S2-"+f.suffix, "Y360-SHR2-"+f.suffix, 1, channels.StatusConfirmed, start.AddDate(0, 0, 2), end.AddDate(0, 0, 2), "1500", "0")
	f.fake.Enqueue(acc.UUID, m2)
	f.pull()
	list = f.reservationsOf(m2.ExternalRef)
	if len(list) != 1 || list[0].Plan.SoldProduct.UUID != other.uuid {
		t.Fatalf("booking without offer = %+v %+v", list, f.messages(m2.ExternalRef))
	}
}

// Reconciliation: the channel's statement against the bookings.
func TestChannelReconciliation(t *testing.T) {
	f := newFx(t)
	f.vehicle("GEKO")
	f.vehicle("GEKO")
	start, end := window(7, 2)
	acc := f.fakeAccount()
	ok := booking("R1-"+f.suffix, "Y360-R1-"+f.suffix, 1, channels.StatusConfirmed, start, end, "2000", "200")
	diff := booking("R2-"+f.suffix, "Y360-R2-"+f.suffix, 1, channels.StatusConfirmed, start, end, "2000", "200")
	f.fake.Enqueue(acc.UUID, ok, diff)
	f.pull()
	f.fake.ScriptStatement(acc.UUID,
		channels.StatementLine{ExternalRef: ok.ExternalRef, Status: "confirmed", StartsAt: start, Currency: "TRY",
			Total: money.MustParse("2000"), Commission: money.MustParse("200"), Net: money.MustParse("1800"), CollectedBy: "channel"},
		channels.StatementLine{ExternalRef: diff.ExternalRef, Status: "confirmed", StartsAt: start, Currency: "TRY",
			Total: money.MustParse("2100"), Commission: money.MustParse("210"), Net: money.MustParse("1890"), CollectedBy: "channel"},
		channels.StatementLine{ExternalRef: "Y360-GHOST-" + f.suffix, Status: "confirmed", StartsAt: start, Currency: "TRY",
			Total: money.MustParse("900"), Commission: money.MustParse("90"), Net: money.MustParse("810"), CollectedBy: "channel"},
	)
	from := start.AddDate(0, 0, -1)
	rep, err := f.svc.Reconcile(f.ctx, f.org, f.account, from, end.AddDate(0, 0, 1))
	f.must(err)
	if rep.Source != "statement" || rep.Counts[model.MatchOK] != 1 || rep.Counts[model.MatchAmount] != 1 || rep.Counts[model.MatchMissingLocal] != 1 {
		t.Fatalf("report = %+v", rep.Counts)
	}
	if !rep.Totals.ChannelNet.Equal(money.MustParse("4500")) || !rep.Totals.CollectedByChannel.Equal(money.MustParse("4500")) {
		t.Fatalf("totals = %+v", rep.Totals)
	}
	runs, _, err := f.svc.ListSyncRuns(f.ctx, f.org, model.SyncRunFilter{Directions: []string{model.DirectionReconcile}, SortDesc: true, Limit: 5})
	f.must(err)
	if len(runs) != 1 {
		t.Fatalf("reconcile runs = %d", len(runs))
	}
}

// The sweep pushes after a reservation event and pulls on its interval.
func TestChannelSyncDue(t *testing.T) {
	f := newFx(t)
	f.vehicle("GEKO")
	if _, err := f.svc.SyncDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, pushes := f.fake.LastPush(f.account)
	if pushes != 1 {
		t.Fatalf("pushes after the first sweep = %d", pushes)
	}
	// Nothing due: no new push.
	if _, err := f.svc.SyncDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, pushes = f.fake.LastPush(f.account); pushes != 1 {
		t.Fatalf("pushes without an event = %d", pushes)
	}
	f.svc.ReservationChanged(f.ctx, f.org, uuid.New(), "confirmed")
	if _, err := f.svc.SyncDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, pushes = f.fake.LastPush(f.account); pushes != 2 {
		t.Fatalf("pushes after an event = %d", pushes)
	}
	runs, _, err := f.svc.ListSyncRuns(f.ctx, f.org, model.SyncRunFilter{Directions: []string{model.DirectionPush}, SortDesc: true, Limit: 5})
	f.must(err)
	if len(runs) != 2 || runs[0].Trigger != model.TriggerEvent {
		t.Fatalf("push runs = %+v", runs)
	}
}

// Another organization's records are invisible and cannot be referenced.
func TestChannelCrossOrganization(t *testing.T) {
	a := newFx(t)
	b := newFx(t)
	if _, err := a.svc.GetAccount(a.ctx, a.org, b.account); !errors.Is(err, usecase.ErrAccountNotFound) {
		t.Fatalf("foreign account = %v", err)
	}
	bm, _, err := b.svc.ListMappings(b.ctx, b.org, model.MappingFilter{Limit: 10})
	b.must(err)
	if _, err := a.svc.GetMapping(a.ctx, a.org, bm[0].UUID); !errors.Is(err, usecase.ErrMappingNotFound) {
		t.Fatalf("foreign mapping = %v", err)
	}
	_, err = a.svc.CreateMapping(a.ctx, a.org, model.MappingInput{AccountUUID: &a.account, EntityType: model.EntityProduct,
		ProductUUID: &b.product.uuid, ExternalCode: sp("X")})
	var ve *usecase.ValidationError
	if !errors.As(err, &ve) || ve.Details[0].Field != "product_uuid" {
		t.Fatalf("foreign product = %v", err)
	}
	_, err = a.svc.CreateMapping(a.ctx, a.org, model.MappingInput{AccountUUID: &b.account, EntityType: model.EntityBranch,
		BranchUUID: model.OptionalUUID{Set: true, Value: &a.branch.uuid}, ExternalCode: sp("X")})
	if !errors.As(err, &ve) || ve.Details[0].Field != "account_uuid" {
		t.Fatalf("foreign account mapping = %v", err)
	}
	if _, err := a.svc.RetryMessage(a.ctx, a.org, uuid.New()); !errors.Is(err, usecase.ErrMessageNotFound) {
		t.Fatalf("unknown message = %v", err)
	}
	accounts, err := a.svc.ListAccounts(a.ctx, a.org)
	a.must(err)
	if len(accounts) != 1 || accounts[0].UUID != a.account {
		t.Fatalf("accounts = %+v", accounts)
	}
}

// Mapping rules: duplicates and product-only fields.
func TestChannelMappingRules(t *testing.T) {
	f := newFx(t)
	_, err := f.svc.CreateMapping(f.ctx, f.org, model.MappingInput{AccountUUID: &f.account, EntityType: model.EntityBranch,
		BranchUUID: model.OptionalUUID{Set: true, Value: &f.branch.uuid}, ExternalCode: sp("IST2")})
	if !errors.Is(err, usecase.ErrMappingTaken) {
		t.Fatalf("second mapping of a branch = %v", err)
	}
	other := f.newBranch("SAW")
	_, err = f.svc.CreateMapping(f.ctx, f.org, model.MappingInput{AccountUUID: &f.account, EntityType: model.EntityBranch,
		BranchUUID: model.OptionalUUID{Set: true, Value: &other.uuid}, ExternalCode: sp("IST")})
	if !errors.Is(err, usecase.ErrMappingTaken) {
		t.Fatalf("location code used twice = %v", err)
	}
	_, err = f.svc.CreateMapping(f.ctx, f.org, model.MappingInput{AccountUUID: &f.account, EntityType: model.EntityBranch,
		BranchUUID: model.OptionalUUID{Set: true, Value: &other.uuid}, ExternalCode: sp("SAW"), StopSale: bp(true)})
	var ve *usecase.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("stop-sale on a branch = %v", err)
	}
	m, err := f.svc.CreateMapping(f.ctx, f.org, model.MappingInput{AccountUUID: &f.account, EntityType: model.EntityBranch,
		BranchUUID: model.OptionalUUID{Set: true, Value: &other.uuid}, ExternalCode: sp("SAW")})
	f.must(err)
	f.must(f.svc.DeleteMapping(f.ctx, f.org, m.UUID))
	if _, err := f.svc.GetMapping(f.ctx, f.org, m.UUID); !errors.Is(err, usecase.ErrMappingNotFound) {
		t.Fatalf("deleted mapping = %v", err)
	}
}

func i32(v int32) *int32 { return &v }
func bp(v bool) *bool    { return &v }
