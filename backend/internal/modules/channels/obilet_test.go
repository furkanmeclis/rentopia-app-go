package channels_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels/channelfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// RT-A26: Obilet plugs into the same core as Yolcu360. With both channels
// connected in one organization, the Obilet account gets its own push
// (availability and price under its own codes), its bookings become
// reservations on the obilet channel with the PNR as external reference,
// a cancellation releases the capacity, acknowledgements and the
// reconciliation work, and the Yolcu360 account is untouched.
func TestRT_A26_ObiletAvailabilityPriceBookingCancellation(t *testing.T) {
	f := newFx(t)
	f.vehicle("GEKO")
	f.vehicle("GEKO")

	acc, err := f.accounts.Connect(f.ctx, f.org, accountsusecase.ConnectInput{
		Kind: channels.Kind, Provider: channelfake.ObiletProviderCode, DisplayName: "Obilet test",
		Credentials: map[string]string{"partner_code": "OBP-" + f.suffix, "api_token": "t-" + f.suffix},
	})
	f.must(err)
	ob := acc.UUID
	for _, in := range []model.MappingInput{
		{EntityType: model.EntityBranch, BranchUUID: model.OptionalUUID{Set: true, Value: &f.branch.uuid}, ExternalCode: sp("IST-OB")},
		{EntityType: model.EntityProduct, ProductUUID: &f.product.uuid, ExternalCode: sp("OB-ECO")},
	} {
		in.AccountUUID = &ob
		_, err := f.svc.CreateMapping(f.ctx, f.org, in)
		f.must(err)
	}

	// Both accounts are listed, each on its own channel.
	accounts, err := f.svc.ListAccounts(f.ctx, f.org)
	f.must(err)
	seen := map[string]string{}
	for _, a := range accounts {
		seen[a.Provider] = a.Channel
	}
	if seen["obilet_fake"] != "obilet" || seen["yolcu360_fake"] != "yolcu360" {
		t.Fatalf("accounts = %+v", seen)
	}

	// Availability and price.
	push := func() channels.InventoryItem {
		t.Helper()
		run, err := f.svc.PushNow(f.ctx, f.org, ob)
		f.must(err)
		if run.Status != model.RunSucceeded {
			t.Fatalf("push run = %+v", run)
		}
		last, _ := f.obilet.LastPush(ob)
		if last == nil || len(last.Items) < 4 {
			t.Fatalf("obilet push = %+v", last)
		}
		return last.Items[3]
	}
	item := push()
	if item.ProductCode != "OB-ECO" || item.LocationCode != "IST-OB" || item.Available != 2 || !item.HasPrice ||
		!item.DailyPrice.Equal(money.MustParse("1000")) || item.OfferRef == "" {
		t.Fatalf("item = %+v", item)
	}
	if _, n := f.fake.LastPush(f.account); n != 0 {
		t.Fatalf("yolcu360 pushes = %d, want 0", n)
	}

	// Booking: one reservation on the obilet channel, PNR as reference.
	start, end := window(3, 2)
	pnr := "OBL-RTA26-" + f.suffix
	b := booking("OB1-"+f.suffix, pnr, 1, channels.StatusConfirmed, start, end, "2000", "240")
	b.ProductCode, b.PickupLocationCode, b.ReturnLocationCode = "OB-ECO", "IST-OB", "IST-OB"
	f.obilet.Enqueue(ob, b, b)
	run, err := f.svc.PullNow(f.ctx, f.org, ob)
	f.must(err)
	if run.Status != model.RunSucceeded {
		t.Fatalf("pull run = %+v", run)
	}
	refs := f.obiletReservations(pnr)
	if len(refs) != 1 {
		t.Fatalf("reservations = %d, want 1", len(refs))
	}
	r, err := f.res.Get(f.ctx, f.org, refs[0])
	f.must(err)
	if r.Status != "confirmed" || r.Channel != "obilet" || r.CollectingParty != "channel" ||
		!r.Price.Total.Equal(money.MustParse("2000")) || !r.Price.Commission.Equal(money.MustParse("240")) ||
		!r.Price.NetReceivable.Equal(money.MustParse("1760")) || !r.Price.Balance.IsZero() {
		t.Fatalf("reservation = %s %s %s total %s commission %s", r.Status, r.Channel, r.CollectingParty, r.Price.Total, r.Price.Commission)
	}
	if got := f.sellable(start, end); got != 1 {
		t.Fatalf("sellable after the booking = %d, want 1", got)
	}
	if n := f.count(`SELECT COUNT(*) FROM channel_reservations c JOIN integration_accounts a ON a.id = c.integration_account_id
		WHERE c.organization_id = $1 AND c.external_ref = $2 AND a.uuid = $3 AND c.channel = 'obilet'`, f.org, pnr, ob); n != 1 {
		t.Fatalf("channel bookings = %d, want 1", n)
	}
	acks := f.obilet.Acks(ob)
	if len(acks) == 0 || !acks[0].Accepted || acks[0].ExternalRef != pnr || acks[0].Reference == "" {
		t.Fatalf("acks = %+v", acks)
	}

	// Reconciliation from the Obilet statement.
	rep, err := f.svc.Reconcile(f.ctx, f.org, ob, start.AddDate(0, 0, -1), end.AddDate(0, 0, 1))
	f.must(err)
	if rep.Counts[model.MatchOK] != 1 {
		t.Fatalf("reconciliation = %+v", rep.Counts)
	}

	// Cancellation releases the capacity once.
	c := b
	c.ID, c.Version, c.Type, c.Status = "OB2-"+f.suffix, 2, channels.MessageCancellation, channels.StatusCancelled
	f.obilet.Enqueue(ob, c)
	_, err = f.svc.PullNow(f.ctx, f.org, ob)
	f.must(err)
	r, err = f.res.Get(f.ctx, f.org, refs[0])
	f.must(err)
	if r.Status != "cancelled" {
		t.Fatalf("status after cancellation = %s", r.Status)
	}
	if got := f.sellable(start, end); got != 2 {
		t.Fatalf("sellable after the cancellation = %d, want 2", got)
	}

	// Yolcu360 saw nothing of it.
	if n := len(f.fake.Acks(f.account)); n != 0 {
		t.Fatalf("yolcu360 acks = %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM reservations WHERE organization_id = $1 AND channel = 'yolcu360'`, f.org); n != 0 {
		t.Fatalf("yolcu360 reservations = %d", n)
	}
}

// A second active Obilet account in the same organization is refused (one
// active account per channel; obilet and obilet_fake are the same channel).
func TestObiletOneActiveAccountPerChannel(t *testing.T) {
	f := newFx(t)
	connect := func() error {
		_, err := f.accounts.Connect(f.ctx, f.org, accountsusecase.ConnectInput{
			Kind: channels.Kind, Provider: channelfake.ObiletProviderCode, DisplayName: "Obilet " + uuid.NewString()[:4],
			Credentials: map[string]string{"partner_code": "P", "api_token": "t"},
		})
		return err
	}
	f.must(connect())
	if err := connect(); !errors.Is(err, accountsusecase.ErrActiveExists) {
		t.Fatalf("a second active Obilet account: %v", err)
	}
}

func (f *fx) obiletReservations(ref string) []uuid.UUID {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(),
		`SELECT uuid FROM reservations WHERE organization_id = $1 AND channel = 'obilet' AND external_reference = $2`, f.org, ref)
	f.must(err)
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		f.must(rows.Scan(&id))
		out = append(out, id)
	}
	return out
}
