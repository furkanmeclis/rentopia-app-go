package invoices_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/ubl"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// issue finalizes a draft and expects it issued by the fake.
func (f *fx) issue(id uuid.UUID) model.Detail {
	f.t.Helper()
	d, err := f.inv.Finalize(f.ctx, f.org, id)
	f.must(err)
	if d.Status != model.StatusSucceeded {
		f.t.Fatalf("status %s (job %+v), want succeeded", d.Status, d.Job)
	}
	return d
}

// validXML checks the stored document against the GİB XSD and Schematron.
func (f *fx) validXML(id uuid.UUID, profile string) []byte {
	f.t.Helper()
	data, _, err := f.inv.XML(f.ctx, f.org, id)
	f.must(err)
	problems, err := ubl.ValidateXML(context.Background(), data, profile)
	f.must(err)
	if len(problems) > 0 {
		f.t.Fatalf("stored XML is not valid: %+v", problems)
	}
	return data
}

// TestBuildAndValidateTemelAndEArsiv: an e-Fatura taxpayer (VKN) gets a
// TEMELFATURA, a person (TCKN) an EARSIVFATURA; both validate (module
// checks, XSD, Schematron) as drafts, are numbered SSSYYYYNNNNNNNNN on
// finalize, and the stored XML passes the GİB XSD and Schematron.
func TestBuildAndValidateTemelAndEArsiv(t *testing.T) {
	f := newFx(t, true)
	for _, tc := range []struct {
		name     string
		customer ref
		driver   ref
		profile  string
		series   string
	}{
		{"temel", f.company, f.companyD, model.ProfileTemel, "EFT"},
		{"earsiv", f.person, f.personD, model.ProfileEArsiv, "EAR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := f.rental(f.branch, tc.customer, tc.driver)
			d := f.draft(r.UUID)
			if d.Profile != tc.profile || d.Status != model.StatusDraft || d.Number != nil {
				t.Fatalf("draft profile %s status %s number %v", d.Profile, d.Status, d.Number)
			}
			if d.Receiver.TaxpayerSource != model.TaxpayerProvider {
				t.Fatalf("taxpayer source %s", d.Receiver.TaxpayerSource)
			}
			problems, err := f.inv.Validate(f.ctx, f.org, d.UUID)
			f.must(err)
			if len(problems) > 0 {
				t.Fatalf("draft problems: %+v", problems)
			}
			issued := f.issue(d.UUID)
			if issued.Number == nil || len(*issued.Number) != 16 || (*issued.Number)[:3] != tc.series {
				t.Fatalf("number %v", issued.Number)
			}
			if issued.ExternalRef == "" {
				t.Fatal("no integrator reference")
			}
			data := f.validXML(d.UUID, tc.profile)
			if _, state, ok := f.fake.Received(d.UUID.String()); !ok || state != einvoice.StateSucceeded {
				t.Fatalf("the fake holds %v %s", ok, state)
			}
			if len(data) == 0 {
				t.Fatal("empty XML")
			}
			html, err := f.inv.Preview(f.ctx, f.org, d.UUID)
			f.must(err)
			if len(html) == 0 {
				t.Fatal("empty preview")
			}
		})
	}
}

// TestRT_A22_InvoiceLinksAccrualsWithoutNewDebt (RT-A22 + RT-A23): the
// invoice links the rent accrual through ledger_entry_links, its payable
// equals the linked amount, and neither drafting nor sending posts a ledger
// line: the rent stays debited exactly once.
func TestRT_A22_InvoiceLinksAccrualsWithoutNewDebt(t *testing.T) {
	f := newFx(t, true)
	r := f.rental(f.branch, f.person, f.personD)
	rent, n := f.rentAccrual(r.UUID)
	if n != 1 {
		t.Fatalf("%d rent accruals before invoicing", n)
	}
	before := f.ledger()
	d := f.draft(r.UUID)
	if len(d.Links) != 1 || !d.Links[0].Amount.Equal(rent) || d.Links[0].Purpose != "rent" {
		t.Fatalf("links %+v, want the rent accrual %s", d.Links, rent)
	}
	if !d.Payable.Equal(rent) || !d.Linked.Equal(rent) || !d.Rounding.IsZero() {
		t.Fatalf("payable %s linked %s rounding %s, want %s", d.Payable, d.Linked, d.Rounding, rent)
	}
	sum := money.Zero()
	for _, l := range d.Lines {
		sum = sum.Add(l.Total)
		if l.VAT.Minor() != l.Net.MulRate(l.VATRate).Minor() {
			t.Fatalf("line VAT %s of %s at %s", l.VAT, l.Net, l.VATRate)
		}
	}
	if !sum.Equal(rent) {
		t.Fatalf("line totals %s, want %s", sum, rent)
	}
	f.issue(d.UUID)
	if after := f.ledger(); after != before {
		t.Fatalf("invoicing changed the ledger: %+v → %+v", before, after)
	}
	if _, n := f.rentAccrual(r.UUID); n != 1 {
		t.Fatalf("RT-A23: %d live rent accruals after invoicing", n)
	}
	if got := f.count(`SELECT count(*) FROM ledger_entry_links WHERE organization_id = $1 AND invoice_id IS NOT NULL AND released_at IS NULL`, f.org); got != 1 {
		t.Fatalf("%d live invoice links", got)
	}
}

// TestAccountDraftCoversRentalsAndCharges: one invoice covers two rentals
// and an approved charge of the same account (rent, charge and toll lines
// with their own mapping); the items a rental draft can see are listed.
func TestAccountDraftCoversRentalsAndCharges(t *testing.T) {
	f := newFx(t, true)
	r1 := f.rental(f.branch, f.person, f.personD)
	r2 := f.rental(f.branch, f.person, f.personD)
	var rid int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM rentals WHERE organization_id = $1 AND uuid = $2`, f.org, r1.UUID).Scan(&rid))
	var damage, toll uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason)
		VALUES ($1, $2, 'damage', 'manual', 600, 'tampon çizik') RETURNING uuid`, f.org, rid).Scan(&damage))
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason)
		VALUES ($1, $2, 'toll', 'manual', 84.50, 'Osmangazi köprüsü') RETURNING uuid`, f.org, rid).Scan(&toll))
	if _, err := f.inv.PutSettings(f.ctx, f.org, model.SettingsInput{TaxMappings: []model.TaxMappingInput{
		{LineKind: "toll", VATRate: "0", ExemptionCode: "351", ExemptionReason: "KDV kapsamı dışı geçiş ücreti"},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []uuid.UUID{damage, toll} {
		if _, err := f.rentals.DecideCharge(f.ctx, f.org, r1.UUID, c, true, "onaylandı"); err != nil {
			t.Fatal(err)
		}
	}
	items, err := f.inv.Items(f.ctx, f.org, model.ItemFilter{RentalUUID: &r1.UUID})
	f.must(err)
	if len(items) != 3 {
		t.Fatalf("rental items %+v", items)
	}
	before := f.ledger()
	acc := items[0].Account.UUID
	d, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{AccountUUID: &acc})
	f.must(err)
	if len(d.Links) != 4 {
		t.Fatalf("%d links, want two rents and two charges", len(d.Links))
	}
	kinds := map[string]int{}
	for _, l := range d.Lines {
		kinds[l.Kind]++
		if l.Kind == model.KindToll && (l.VATRate != 0 || l.ExemptionCode != "351") {
			t.Fatalf("toll line %+v", l)
		}
	}
	if kinds[model.KindRent] != 2 || kinds[model.KindCharge] != 1 || kinds[model.KindToll] != 1 {
		t.Fatalf("line kinds %v", kinds)
	}
	if d.Rental != nil {
		t.Fatalf("an invoice over two rentals names one: %+v", d.Rental)
	}
	rent1, _ := f.rentAccrual(r1.UUID)
	rent2, _ := f.rentAccrual(r2.UUID)
	want := rent1.Add(rent2).Add(money.MustParse("684.50"))
	if !d.Linked.Equal(want) || !d.Payable.Equal(want) {
		t.Fatalf("linked %s payable %s, want %s", d.Linked, d.Payable, want)
	}
	issued := f.issue(d.UUID)
	f.validXML(issued.UUID, model.ProfileEArsiv)
	if f.ledger() != before {
		t.Fatal("the account invoice changed the ledger")
	}
	listed, _, err := f.inv.List(f.ctx, f.org, model.ListFilter{RentalUUID: &r2.UUID, Limit: 10})
	f.must(err)
	if len(listed) != 1 || listed[0].UUID != d.UUID {
		t.Fatalf("the rental filter misses the account invoice: %+v", listed)
	}
}

// TestDoubleInvoicingRefused: an item a standing invoice covers cannot be
// invoiced again — by the rental, by naming the entry, concurrently, or
// straight in the database; cancelling the invoice frees it.
func TestDoubleInvoicingRefused(t *testing.T) {
	f := newFx(t, true)
	r := f.rental(f.branch, f.person, f.personD)
	d := f.draft(r.UUID)
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{RentalUUID: &r.UUID}); !errors.Is(err, usecase.ErrNothingToInvoice) {
		t.Fatalf("second draft of the rental: %v", err)
	}
	entry := d.Links[0].EntryUUID
	acc := d.Account.UUID
	var ve *usecase.ValidationError
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{AccountUUID: &acc, EntryUUIDs: []uuid.UUID{entry}}); !errors.As(err, &ve) {
		t.Fatalf("draft naming the invoiced entry: %v", err)
	}
	// The database refuses a second live link whatever the code does.
	var entryID, invID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT entry_id, invoice_id FROM ledger_entry_links WHERE organization_id = $1 AND invoice_id IS NOT NULL`, f.org).Scan(&entryID, &invID))
	err := f.q.InsertInvoiceLink(context.Background(), db.InsertInvoiceLinkParams{
		OrganizationID: f.org, EntryID: entryID, DocumentRef: uuid.NewString(), InvoiceID: pgtype.Int8{Int64: invID, Valid: true},
		Amount: money.MustParse("1").Numeric(),
	})
	if err == nil {
		t.Fatal("the database accepted a second live link of an invoiced entry")
	}
	// Cancelling the draft frees the item; two concurrent drafts then make one.
	if _, err := f.inv.Cancel(f.ctx, f.org, d.UUID, model.CancelInput{Reason: "yanlış"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{RentalUUID: &r.UUID})
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, usecase.ErrAlreadyInvoiced), errors.Is(err, usecase.ErrNothingToInvoice):
		default:
			t.Fatalf("concurrent draft: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d concurrent drafts were created, want 1", ok)
	}
}

// TestIdempotentSendCreatesOneInvoice: a repeated create (same key) answers
// the first draft; a repeated finalize sends nothing twice; a send whose
// answer was lost is checked with Status (found) and never sent again: the
// integrator holds exactly one document.
func TestIdempotentSendCreatesOneInvoice(t *testing.T) {
	f := newFx(t, true)
	r := f.rental(f.branch, f.company, f.companyD)
	in := model.CreateInput{RentalUUID: &r.UUID, IdempotencyKey: "fatura-" + f.suffix}
	a, err := f.inv.CreateDraft(f.ctx, f.org, in)
	f.must(err)
	b, err := f.inv.CreateDraft(f.ctx, f.org, in)
	f.must(err)
	if a.UUID != b.UUID {
		t.Fatalf("the same key made two drafts: %s, %s", a.UUID, b.UUID)
	}
	f.fake.FailNextSend(einvoice.ErrTimeout, true) // the integrator issued it, the answer was lost
	d, err := f.inv.Finalize(f.ctx, f.org, a.UUID)
	f.must(err)
	if d.Status != model.StatusSent || d.Job.Status != jobsusecase.StatusNeedsVerification {
		t.Fatalf("after a lost answer: %s / %s", d.Status, d.Job.Status)
	}
	again, err := f.inv.Finalize(f.ctx, f.org, a.UUID)
	f.must(err)
	if again.Job.UUID != d.Job.UUID {
		t.Fatal("a second finalize made a second job")
	}
	f.dueNow(a.UUID)
	f.step(d)
	d = f.get(a.UUID)
	if d.Status != model.StatusSucceeded {
		t.Fatalf("after the status check: %s", d.Status)
	}
	if f.fake.Sends() != 1 || f.fake.Documents() != 1 {
		t.Fatalf("sends %d documents %d, want 1 / 1", f.fake.Sends(), f.fake.Documents())
	}
	if _, err := f.inv.Retry(f.ctx, f.org, a.UUID); !errors.Is(err, usecase.ErrNotRetryable) {
		t.Fatalf("retry of an issued invoice: %v", err)
	}
	if got := f.count(`SELECT count(*) FROM integration_jobs WHERE organization_id = $1 AND kind = 'einvoice.send'`, f.org); got != 1 {
		t.Fatalf("%d send jobs", got)
	}
}

// TestUncertainSendChecksStatusFirst: a timeout without a document → the
// sweep's unclear status check and an unclear retry send nothing; a
// confirmed "not found" sends exactly once more.
func TestUncertainSendChecksStatusFirst(t *testing.T) {
	f := newFx(t, true)
	r := f.rental(f.branch, f.person, f.personD)
	d := f.draft(r.UUID)
	f.fake.FailNextSend(einvoice.ErrTimeout, false)
	d, err := f.inv.Finalize(f.ctx, f.org, d.UUID)
	f.must(err)
	if d.Job.Status != jobsusecase.StatusNeedsVerification {
		t.Fatalf("job %s", d.Job.Status)
	}
	f.fake.FailNextStatus(einvoice.ErrTimeout, einvoice.ErrTimeout)
	f.dueNow(d.UUID)
	f.step(d)
	_, err = f.inv.Retry(f.ctx, f.org, d.UUID)
	var verr *jobsusecase.VerifyError
	if !errors.As(err, &verr) {
		t.Fatalf("an unclear retry: %v", err)
	}
	if f.fake.Sends() != 1 {
		t.Fatalf("%d sends before a confirmed answer", f.fake.Sends())
	}
	res, err := f.inv.Retry(f.ctx, f.org, d.UUID)
	f.must(err)
	if res.Action != "resubmitted" || f.fake.Sends() != 2 || f.fake.Documents() != 1 {
		t.Fatalf("retry %+v, sends %d, documents %d", res, f.fake.Sends(), f.fake.Documents())
	}
	if got := f.get(d.UUID); got.Status != model.StatusSucceeded || len(got.AttemptLog) < 4 {
		t.Fatalf("status %s, %d attempts", got.Status, len(got.AttemptLog))
	}
}

// TestSendFailureKeepsFinance: a refused send leaves the ledger untouched,
// shows the invoice as failed and needing action (finance screen badge,
// filter) with its job; cancelling frees the items for a new invoice.
func TestSendFailureKeepsFinance(t *testing.T) {
	f := newFx(t, false)
	f.connect("reject")
	r := f.rental(f.branch, f.person, f.personD)
	before := f.ledger()
	d := f.draft(r.UUID)
	d, err := f.inv.Finalize(f.ctx, f.org, d.UUID)
	f.must(err)
	if d.Status != model.StatusFailed || d.Job == nil || d.Job.Status != jobsusecase.StatusFailed || d.LastError == "" {
		t.Fatalf("status %s job %+v error %q", d.Status, d.Job, d.LastError)
	}
	if after := f.ledger(); after != before {
		t.Fatalf("a failed send changed the ledger: %+v → %+v", before, after)
	}
	facets, err := f.inv.Facets(f.ctx, f.org)
	f.must(err)
	if facets.NeedsAction != 1 {
		t.Fatalf("needs action %d", facets.NeedsAction)
	}
	yes := true
	items, _, err := f.inv.List(f.ctx, f.org, model.ListFilter{NeedsAction: &yes, Limit: 10})
	f.must(err)
	if len(items) != 1 || items[0].UUID != d.UUID || !items[0].NeedsAction {
		t.Fatalf("needs-action list %+v", items)
	}
	if got := f.count(`SELECT count(*) FROM integration_jobs WHERE organization_id = $1 AND kind = 'einvoice.send' AND status = 'failed'`, f.org); got != 1 {
		t.Fatalf("%d failed jobs visible", got)
	}
	if _, err := f.inv.Cancel(f.ctx, f.org, d.UUID, model.CancelInput{Reason: "entegratör reddetti"}); err != nil {
		t.Fatal(err)
	}
	if got := f.get(d.UUID); got.Status != model.StatusCancelled || !got.Links[0].Released {
		t.Fatalf("cancelled %s, links %+v", got.Status, got.Links)
	}
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{RentalUUID: &r.UUID}); err != nil {
		t.Fatalf("the freed item cannot be invoiced again: %v", err)
	}
	if after := f.ledger(); after != before {
		t.Fatal("cancelling changed the ledger")
	}
}

// TestReturnInvoice (IADE): an issued TICARIFATURA is returned with a
// TEMELFATURA IADE invoice that names it; when the return is issued the
// original is returned and its items are free again; nothing is posted.
func TestReturnInvoice(t *testing.T) {
	f := newFx(t, true)
	r := f.rental(f.branch, f.company, f.companyD)
	d := f.draft(r.UUID)
	d, err := f.inv.PatchDraft(f.ctx, f.org, d.UUID, model.PatchInput{Profile: sp(model.ProfileTicari)})
	f.must(err)
	orig := f.issue(d.UUID)
	before := f.ledger()
	var ve *usecase.ValidationError
	if _, err := f.inv.Return(f.ctx, f.org, d.UUID, model.ReturnInput{}); !errors.As(err, &ve) {
		t.Fatalf("a return without a reason: %v", err)
	}
	ret, err := f.inv.Return(f.ctx, f.org, d.UUID, model.ReturnInput{Reason: "araç iade edildi"})
	f.must(err)
	if ret.Type != model.TypeIade || ret.Profile != model.ProfileTemel || ret.Returns == nil || ret.Returns.UUID != orig.UUID {
		t.Fatalf("return %s / %s / %+v", ret.Type, ret.Profile, ret.Returns)
	}
	if !ret.Payable.Equal(orig.Payable) || len(ret.Lines) != len(orig.Lines) {
		t.Fatalf("return payable %s lines %d", ret.Payable, len(ret.Lines))
	}
	if again, err := f.inv.Return(f.ctx, f.org, d.UUID, model.ReturnInput{Reason: "tekrar"}); err != nil || again.UUID != ret.UUID {
		t.Fatalf("a second return: %v %v", again.UUID, err)
	}
	f.issue(ret.UUID)
	f.validXML(ret.UUID, model.ProfileTemel)
	after := f.get(d.UUID)
	if !after.Returned || after.ReturnedBy == nil || !after.Links[0].Released || after.Links[0].ReleaseReason != model.ReleaseReturned {
		t.Fatalf("original after the return: returned %v by %+v links %+v", after.Returned, after.ReturnedBy, after.Links)
	}
	if f.ledger() != before {
		t.Fatal("the return changed the ledger")
	}
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{RentalUUID: &r.UUID}); err != nil {
		t.Fatalf("the returned items cannot be invoiced again: %v", err)
	}
}

// TestCancel: a draft and an unsent invoice are cancelled at once; an
// issued e-Arşiv invoice is cancelled through the integrator; an issued
// e-Fatura answers "use a return invoice".
func TestCancel(t *testing.T) {
	f := newFx(t, false)
	// Unsent: no integrator → the send waits on the dependency.
	r1 := f.rental(f.branch, f.person, f.personD)
	d1 := f.draft(r1.UUID)
	d1, err := f.inv.Finalize(f.ctx, f.org, d1.UUID)
	f.must(err)
	if d1.Status != model.StatusQueued || d1.Job.Status != jobsusecase.StatusWaitingDependency || d1.HoldReason != model.HoldNoAccount {
		t.Fatalf("waiting: %s / %s / %s", d1.Status, d1.Job.Status, d1.HoldReason)
	}
	var dep *usecase.DependencyError
	if _, err := f.inv.Retry(f.ctx, f.org, d1.UUID); !errors.As(err, &dep) {
		t.Fatalf("retry without an integrator: %v", err)
	}
	if _, err := f.inv.Cancel(f.ctx, f.org, d1.UUID, model.CancelInput{}); err == nil {
		t.Fatal("cancelling a finalized invoice without a reason")
	}
	c1, err := f.inv.Cancel(f.ctx, f.org, d1.UUID, model.CancelInput{Reason: "müşteri vazgeçti"})
	f.must(err)
	if c1.Status != model.StatusCancelled || c1.Job.Status != jobsusecase.StatusFailed {
		t.Fatalf("cancelled unsent: %s / %s", c1.Status, c1.Job.Status)
	}
	// Issued e-Arşiv: cancelled through the integrator.
	f.connect("")
	r2 := f.rental(f.branch, f.person, f.personD)
	d2 := f.issue(f.draft(r2.UUID).UUID)
	c2, err := f.inv.Cancel(f.ctx, f.org, d2.UUID, model.CancelInput{Reason: "hatalı fatura"})
	f.must(err)
	if c2.Status != model.StatusCancelled || c2.CancelJob == nil || c2.CancelJob.Status != jobsusecase.StatusSucceeded || !c2.Links[0].Released {
		t.Fatalf("e-Arşiv cancel: %s / %+v / %+v", c2.Status, c2.CancelJob, c2.Links)
	}
	if _, state, _ := f.fake.Received(d2.UUID.String()); state != einvoice.StateCancelled {
		t.Fatalf("the integrator state is %s", state)
	}
	// Issued e-Fatura: a return invoice instead.
	r3 := f.rental(f.branch, f.company, f.companyD)
	d3 := f.issue(f.draft(r3.UUID).UUID)
	if _, err := f.inv.Cancel(f.ctx, f.org, d3.UUID, model.CancelInput{Reason: "hatalı"}); !errors.Is(err, usecase.ErrUseReturn) {
		t.Fatalf("cancelling an issued e-Fatura: %v", err)
	}
}

// TestWaitingSendIsReleased: a send waiting on the dependency goes out when
// an integrator is connected (the minute sweep).
func TestWaitingSendIsReleased(t *testing.T) {
	f := newFx(t, false)
	r := f.rental(f.branch, f.person, f.personD)
	d, err := f.inv.Finalize(f.ctx, f.org, f.draft(r.UUID).UUID)
	f.must(err)
	if d.Job.Status != jobsusecase.StatusWaitingDependency {
		t.Fatalf("job %s", d.Job.Status)
	}
	f.connect("")
	n, err := f.inv.ReleaseWaiting(context.Background())
	f.must(err)
	if n < 1 {
		t.Fatal("nothing released")
	}
	if got := f.get(d.UUID); got.Status != model.StatusSucceeded || got.HoldReason != "" {
		t.Fatalf("after the release: %s hold %q", got.Status, got.HoldReason)
	}
}

// TestValidationProblemsBlockSending: missing receiver data and a 0 % rate
// without an exemption are reported per field and nothing is numbered or
// sent; fixing them lets the invoice out.
func TestValidationProblemsBlockSending(t *testing.T) {
	f := newFx(t, true)
	r := f.rental(f.branch, f.person, f.personD)
	d := f.draft(r.UUID)
	d, err := f.inv.PatchDraft(f.ctx, f.org, d.UUID, model.PatchInput{
		Receiver: &model.ReceiverInput{District: sp("")},
		Lines:    []model.LinePatch{{UUID: d.Lines[0].UUID, VATRate: sp("0")}},
	})
	f.must(err)
	_, err = f.inv.Finalize(f.ctx, f.org, d.UUID)
	var de *usecase.DocumentError
	if !errors.As(err, &de) {
		t.Fatalf("finalize with problems: %v", err)
	}
	fields := map[string]bool{}
	for _, p := range de.Problems {
		fields[p.Field] = true
	}
	if !fields["receiver.district"] || !fields["lines.0.exemption_code"] {
		t.Fatalf("problems %+v", de.Problems)
	}
	got := f.get(d.UUID)
	if got.Status != model.StatusDraft || got.Number != nil || len(got.ValidationErrors) == 0 || f.fake.Sends() != 0 {
		t.Fatalf("after refused finalize: %s %v %d problems, %d sends", got.Status, got.Number, len(got.ValidationErrors), f.fake.Sends())
	}
	if f.count(`SELECT count(*) FROM invoice_sequences WHERE organization_id = $1 AND last_value > 0`, f.org) != 0 {
		t.Fatal("a refused finalize consumed a number")
	}
	_, err = f.inv.PatchDraft(f.ctx, f.org, d.UUID, model.PatchInput{
		Receiver: &model.ReceiverInput{District: sp("Çankaya")},
		Lines:    []model.LinePatch{{UUID: d.Lines[0].UUID, VATRate: sp("20")}},
	})
	f.must(err)
	issued := f.issue(d.UUID)
	if *issued.Number != "EAR"+issued.IssueDate[:4]+"000000001" {
		t.Fatalf("first number %s", *issued.Number)
	}
}

// TestTaxMappingAndTaxpayerCache: the settings' VAT per line kind applies to
// new drafts; a 0 % rate needs an exemption; the taxpayer answer is cached.
func TestTaxMappingAndTaxpayerCache(t *testing.T) {
	f := newFx(t, true)
	var ve *usecase.ValidationError
	if _, err := f.inv.PutSettings(f.ctx, f.org, model.SettingsInput{TaxMappings: []model.TaxMappingInput{{LineKind: "rent", VATRate: "0"}}}); !errors.As(err, &ve) {
		t.Fatalf("0 %% without an exemption: %v", err)
	}
	st, err := f.inv.PutSettings(f.ctx, f.org, model.SettingsInput{TaxMappings: []model.TaxMappingInput{{LineKind: "rent", VATRate: "10"}}})
	f.must(err)
	if st.TaxMappings[0].VATRate != money.Percent(10) || len(st.Missing) != 0 {
		t.Fatalf("settings %+v", st)
	}
	r1 := f.rental(f.branch, f.company, f.companyD)
	d1 := f.draft(r1.UUID)
	if d1.Lines[0].VATRate != money.Percent(10) {
		t.Fatalf("rent VAT %s", d1.Lines[0].VATRate)
	}
	lookups := f.fake.Lookups()
	r2 := f.rental(f.branch, f.company, f.companyD)
	d2 := f.draft(r2.UUID)
	if d2.Receiver.TaxpayerSource != model.TaxpayerCache || f.fake.Lookups() != lookups {
		t.Fatalf("second lookup: %s, lookups %d → %d", d2.Receiver.TaxpayerSource, lookups, f.fake.Lookups())
	}
}

// TestBranchScope: a member of branch A sees and acts on A's invoices only;
// B's answer 404 and never appear in lists or facets.
func TestBranchScope(t *testing.T) {
	f := newFx(t, true)
	a := f.draft(f.rental(f.branch, f.person, f.personD).UUID)
	rb := f.rental(f.other, f.person, f.personD)
	b := f.draft(rb.UUID)
	onlyA := f.scoped([]int64{f.branch.id})
	if _, err := f.inv.Get(onlyA, f.org, b.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("B's invoice for an A member: %v", err)
	}
	if _, err := f.inv.Finalize(onlyA, f.org, b.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("finalize B's invoice: %v", err)
	}
	if _, err := f.inv.Cancel(onlyA, f.org, b.UUID, model.CancelInput{Reason: "x x x"}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cancel B's invoice: %v", err)
	}
	items, total, err := f.inv.List(onlyA, f.org, model.ListFilter{Limit: 50})
	f.must(err)
	if total != 1 || items[0].UUID != a.UUID {
		t.Fatalf("A member's list: %d %+v", total, items)
	}
	facets, err := f.inv.Facets(onlyA, f.org)
	f.must(err)
	for _, fc := range facets.Branch {
		if fc.Value == f.other.uuid.String() {
			t.Fatal("B appears in the A member's branch facet")
		}
	}
	if _, err := f.inv.CreateDraft(onlyA, f.org, model.CreateInput{RentalUUID: &rb.UUID}); !errors.Is(err, usecase.ErrNothingToInvoice) {
		t.Fatalf("an A member invoicing B's rental: %v", err)
	}
}

// TestCrossOrganization: another organization's invoices, rentals and
// accounts do not exist for the caller.
func TestCrossOrganization(t *testing.T) {
	f := newFx(t, true)
	g := newFx(t, true)
	theirs := g.draft(g.rental(g.branch, g.person, g.personD).UUID)
	if _, err := f.inv.Get(f.ctx, f.org, theirs.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	if _, err := f.inv.Finalize(f.ctx, f.org, theirs.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("finalize: %v", err)
	}
	if _, _, err := f.inv.XML(f.ctx, f.org, theirs.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("xml: %v", err)
	}
	var ve *usecase.ValidationError
	rental := theirs.Rental.UUID
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{RentalUUID: &rental}); !errors.As(err, &ve) {
		t.Fatalf("drafting another organization's rental: %v", err)
	}
	acc := theirs.Account.UUID
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{AccountUUID: &acc}); !errors.As(err, &ve) {
		t.Fatalf("drafting another organization's account: %v", err)
	}
	if _, err := f.inv.CreateDraft(f.ctx, f.org, model.CreateInput{AccountUUID: &acc, EntryUUIDs: []uuid.UUID{theirs.Links[0].EntryUUID}}); !errors.As(err, &ve) {
		t.Fatalf("drafting another organization's entry: %v", err)
	}
	items, err := f.inv.Items(f.ctx, f.org, model.ItemFilter{RentalUUID: &rental})
	f.must(err)
	if len(items) != 0 {
		t.Fatalf("another organization's items: %+v", items)
	}
	if got := g.get(theirs.UUID); got.Status != model.StatusDraft {
		t.Fatalf("their invoice changed: %s", got.Status)
	}
}
