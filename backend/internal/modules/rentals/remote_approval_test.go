package rentals_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts"
	contractshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/handler"
	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm/netgsmfake"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Remote approval (TEC-711): a signed, time-limited link plus an SMS code
// approves exactly one document version. The SMS provider is the Netgsm
// fake (in process, no network); the code is read from what the fake
// received, never from the database.

// capturePDF records the HTML it printed (the approval page).
type capturePDF struct {
	mu   sync.Mutex
	last string
}

func (c *capturePDF) RenderPDF(_ context.Context, html []byte) ([]byte, error) {
	c.mu.Lock()
	c.last = string(html)
	c.mu.Unlock()
	return append([]byte("%PDF-1.4\n"), html[:16]...), nil
}

func (c *capturePDF) html() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

type remoteFx struct {
	*fx
	fake *netgsmfake.Fake
	pdf  *capturePDF
	doc  uuid.UUID
}

var client = contractsmodel.ClientInfo{IP: "203.0.113.7", UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)"}

// newRemoteFx: an organization with an SMS account (Netgsm fake), an active
// rental contract template and a rental whose contract is prepared.
func newRemoteFx(t *testing.T) *remoteFx {
	t.Helper()
	f := newFx(t)
	ctx := context.Background()
	fake := netgsmfake.New()
	t.Cleanup(fake.Server.Close)
	fake.AddAccount("8501112233", "pw-"+f.suffix, "RENTOPIA")
	adapter := netgsm.New(netgsm.Config{SandboxBaseURL: fake.URL()})
	fc, err := crypto.NewFieldCipher(strings.Repeat("s", 32))
	f.must(err)
	vault, err := integrations.NewVault(fc)
	f.must(err)
	accounts := accountsusecase.New(f.q, vault, integrations.NewRegistry(adapter))
	_, err = accounts.Connect(ctx, f.org, accountsusecase.ConnectInput{
		Provider: "netgsm", DisplayName: "Netgsm", Mode: "sandbox",
		Credentials: map[string]string{"usercode": "8501112233", "password": "pw-" + f.suffix, "msgheader": "RENTOPIA"},
	})
	f.must(err)
	ops := integrations.NewOperationRegistry()
	runner := jobsusecase.NewRunner(f.q, accounts, ops)
	smsSvc, err := smsusecase.New(f.pool, f.q, fc, accounts, runner, ops, adapter)
	f.must(err)
	smsSvc.SetClock(f.now)
	rec := activity.NewRecorder(f.q, nil)
	smsSvc.SetActivity(rec)
	f.must(f.contracts.SetRemote(contractsusecase.Remote{SMS: smsSvc, Cipher: fc, PublicBaseURL: "https://app.test", Activity: rec}))
	pdf := &capturePDF{}
	f.contracts.SetRenderer(pdf)

	name := "Kiralama sözleşmesi"
	body := "<p>Kiracı: {{renter.full_name}}</p><p>Araç: {{vehicle.plate}}</p><p>Toplam: {{amounts.total}}</p>"
	tpl, err := f.contracts.CreateTemplate(f.ctx, f.org, contractsmodel.TemplateInput{Kind: contractsmodel.KindRentalContract, Name: &name, BodyHTML: &body})
	f.must(err)
	_, err = f.contracts.ActivateTemplate(f.ctx, f.org, tpl.UUID)
	f.must(err)
	return &remoteFx{fx: f, fake: fake, pdf: pdf}
}

// rental creates a rental (its contract v1 is rendered).
func (r *remoteFx) rental() model.Rental {
	r.t.Helper()
	v := r.vehicle()
	start := r.now().Add(time.Hour).Truncate(time.Minute)
	res := r.reserve(start, start.Add(72*time.Hour), nil)
	rental, err := r.rentals.Create(r.ctx, r.org, model.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v.uuid})
	r.must(err)
	if rental.Contract.DocumentUUID == nil {
		r.t.Fatal("no contract document")
	}
	r.doc = *rental.Contract.DocumentUUID
	return rental
}

func (r *remoteFx) reservationID(rental model.Rental) int64 {
	r.t.Helper()
	var id int64
	r.must(r.pool.QueryRow(context.Background(), `SELECT reservation_id FROM rentals WHERE organization_id = $1 AND uuid = $2`,
		r.org, rental.UUID).Scan(&id))
	return id
}

func (r *remoteFx) current() contractsmodel.VersionInfo {
	r.t.Helper()
	d, err := r.contracts.GetDocument(r.ctx, r.org, r.doc)
	r.must(err)
	if d.Current == nil {
		r.t.Fatal("no current version")
	}
	return *d.Current
}

// link creates a link for the renter and returns it with its token.
func (r *remoteFx) link(minutes *int, send bool) (contractsmodel.CreatedLink, string) {
	r.t.Helper()
	cur := r.current()
	out, err := r.contracts.CreateApprovalLink(r.ctx, r.org, r.doc, cur.UUID, contractsmodel.CreateLinkInput{
		ContentHash: cur.ContentHash, SignerRole: contractsmodel.SignerRenter, ExpiresInMinutes: minutes, SendSMS: send,
	})
	r.must(err)
	_, token, ok := strings.Cut(out.Link.URL, "/d/")
	if !ok || len(token) != 43 {
		r.t.Fatalf("link url %q", out.Link.URL)
	}
	return out, token
}

var otpRE = regexp.MustCompile(`kodunuz: (\d{6})`)

func (r *remoteFx) lastCode() string {
	r.t.Helper()
	msgs := r.fake.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if m := otpRE.FindStringSubmatch(msgs[i].Text); m != nil {
			return m[1]
		}
	}
	r.t.Fatal("no code reached the fake")
	return ""
}

func wrong(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func TestRemoteApprovalHappyPath(t *testing.T) {
	r := newRemoteFx(t)
	rental := r.rental()
	cur := r.current()

	// Signers: the renter has a mobile number; the driver has none.
	opts, err := r.contracts.Signers(r.ctx, r.org, r.doc)
	r.must(err)
	if !opts.SMSAvailable || len(opts.Signers) != 2 || !opts.Signers[0].HasPhone || opts.Signers[1].HasPhone ||
		opts.Signers[0].PhoneMasked != "+90 5** *** ** 33" {
		t.Fatalf("signers %+v", opts)
	}

	created, token := r.link(nil, true)
	if created.SMS == nil || created.SMSError != "" || created.Link.Status != contractsmodel.LinkPending ||
		created.Link.SMSSentCount != 1 || created.Link.ExpiresAt.Sub(r.now()) < 23*time.Hour {
		t.Fatalf("created %+v", created)
	}
	// The link SMS (transactional) reached the number; the log keeps the
	// text with the token redacted.
	msgs := r.fake.Messages()
	if len(msgs) != 1 || msgs[0].No != "5321112233" || !strings.Contains(msgs[0].Text, "https://app.test/d/"+token) {
		t.Fatalf("link sms %+v", msgs)
	}
	var class, logged string
	r.must(r.pool.QueryRow(context.Background(), `SELECT class, body_redacted FROM sms_messages WHERE organization_id = $1 AND purpose = 'contract_approval_link'`,
		r.org).Scan(&class, &logged))
	if class != "transactional" || strings.Contains(logged, token) {
		t.Fatalf("logged link sms: %s %q", class, logged)
	}

	page, err := r.contracts.PublicDocument(context.Background(), token)
	r.must(err)
	if page.State != contractsmodel.LinkPending || page.ContentHash != cur.ContentHash || !strings.Contains(page.HTML, "Ayşe Yılmaz") ||
		page.PhoneMasked != "+90 5** *** ** 33" || page.SignerName != "Ayşe Y." || page.PDFAvailable {
		t.Fatalf("page %+v", page)
	}
	if !strings.Contains(page.HTML, contractsmodel.DisclaimerTR) || !strings.Contains(page.HTML, contractsmodel.DisclaimerEN) {
		t.Fatal("the document footer must carry the statement")
	}

	code, err := r.contracts.RequestCode(context.Background(), token, client)
	r.must(err)
	if code.PhoneMasked != "+90 5** *** ** 33" {
		t.Fatalf("code %+v", code)
	}
	otp := r.lastCode()

	// The hash the page displayed must be the version's.
	var cc *contractsusecase.ContentChangedError
	if _, err := r.contracts.ApproveRemote(context.Background(), token, contractsmodel.PublicApproveInput{
		Code: otp, ContentHash: strings.Repeat("0", 64),
	}, client); !errors.As(err, &cc) {
		t.Fatalf("approve with another hash = %v", err)
	}
	out, err := r.contracts.ApproveRemote(context.Background(), token, contractsmodel.PublicApproveInput{Code: otp, ContentHash: cur.ContentHash}, client)
	r.must(err)
	if out.State != contractsmodel.LinkApproved || out.ApprovedAt == nil || !out.PDFAvailable || out.PhoneMasked != "" {
		t.Fatalf("approved page %+v", out)
	}
	// The code verifies once.
	if _, err := r.contracts.ApproveRemote(context.Background(), token, contractsmodel.PublicApproveInput{Code: otp, ContentHash: cur.ContentHash}, client); err == nil {
		t.Fatal("a second approval with the same link must be refused")
	}

	doc, err := r.contracts.GetDocument(r.ctx, r.org, r.doc)
	r.must(err)
	if doc.Status != contractsmodel.DocumentApproved || doc.Current.Status != contractsmodel.VersionApproved || len(doc.Approvals) != 1 {
		t.Fatalf("document %+v", doc)
	}
	a := doc.Approvals[0]
	if a.Method != contractsmodel.MethodRemoteSMS || a.VerifiedPhoneMasked != "+90 5** *** ** 33" || a.ContentHash != cur.ContentHash ||
		a.VersionUUID != cur.UUID || a.ApproverName != "Ayşe Yılmaz" || a.SignerRole != contractsmodel.SignerRenter ||
		a.IPAddress != client.IP || a.UserAgent != client.UserAgent || !a.Valid || a.LinkUUID == nil || *a.LinkUUID != created.Link.UUID {
		t.Fatalf("approval %+v", a)
	}
	var customerSet bool
	r.must(r.pool.QueryRow(context.Background(), `SELECT customer_id = $2 FROM document_approvals WHERE organization_id = $1 AND uuid = $3`,
		r.org, r.customer.id, a.UUID).Scan(&customerSet))
	if !customerSet {
		t.Fatal("the approval must name the renter")
	}
	links, err := r.contracts.ListApprovalLinks(r.ctx, r.org, r.doc)
	r.must(err)
	if len(links) != 1 || links[0].Status != contractsmodel.LinkApproved || links[0].URL != "" || links[0].ApprovalUUID == nil {
		t.Fatalf("links %+v", links)
	}

	// Activity: code sent and approval, with the device; never the code.
	var n int
	r.must(r.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM activity_events WHERE organization_id = $1
		AND action = 'documents.remote_approved' AND ip_address = '203.0.113.7' AND payload->>'content_hash' = $2`,
		r.org, cur.ContentHash).Scan(&n))
	if n != 1 {
		t.Fatalf("remote approval activity entries = %d", n)
	}
	r.must(r.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM activity_events WHERE organization_id = $1 AND payload::text LIKE '%' || $2 || '%'`,
		r.org, otp).Scan(&n))
	if n != 0 {
		t.Fatal("the code must never be written to the activity log")
	}

	// A remote approval satisfies the handover rule like an in-person one.
	state, err := r.contracts.ContractState(r.ctx, r.org, r.reservationID(rental))
	r.must(err)
	if !state.Approved {
		t.Fatalf("contract state %+v", state)
	}
	if _, err := r.rentals.Handover(r.ctx, r.org, rental.UUID, model.HandoverInput{Km: i32(1100), Fuel: i16(100)}); err != nil {
		t.Fatalf("handover after a remote approval: %v", err)
	}

	// The approved PDF through the token: the version with the approval
	// page and the statement.
	pdf, err := r.contracts.OpenPublicPDF(context.Background(), token)
	r.must(err)
	raw, _ := io.ReadAll(pdf.Body)
	_ = pdf.Body.Close()
	html := r.pdf.html()
	if !strings.HasPrefix(string(raw), "%PDF") || !strings.Contains(html, "Onay Kaydı / Approval Record") ||
		!strings.Contains(html, cur.ContentHash) || !strings.Contains(html, "+90 5** *** ** 33") ||
		!strings.Contains(html, contractsmodel.DisclaimerTR) || !strings.Contains(html, contractsmodel.DisclaimerEN) ||
		!strings.Contains(html, "203.0.113.7") {
		t.Fatalf("approval pdf html: %s", html)
	}
	// Stored once, streamed again without a render.
	r.pdf.mu.Lock()
	r.pdf.last = ""
	r.pdf.mu.Unlock()
	again, err := r.contracts.OpenPublicPDF(context.Background(), token)
	r.must(err)
	_ = again.Body.Close()
	if r.pdf.html() != "" {
		t.Fatal("the approval pdf must be stored after the first render")
	}
	staff, err := r.contracts.OpenApprovalPDF(r.ctx, r.org, r.doc, a.UUID)
	r.must(err)
	_ = staff.Body.Close()
}

func TestRemoteApprovalWrongCodeAndLock(t *testing.T) {
	r := newRemoteFx(t)
	r.rental()
	cur := r.current()
	_, token := r.link(nil, false)
	_, err := r.contracts.RequestCode(context.Background(), token, client)
	r.must(err)
	otp := r.lastCode()
	in := contractsmodel.PublicApproveInput{Code: wrong(otp), ContentHash: cur.ContentHash}
	// Five wrong codes lock the challenge; every answer is the same.
	for i := 0; i < 5; i++ {
		if _, err := r.contracts.ApproveRemote(context.Background(), token, in, client); !errors.Is(err, contractsusecase.ErrCodeInvalid) {
			t.Fatalf("wrong code %d = %v", i+1, err)
		}
	}
	// The right code no longer works on the locked challenge.
	if _, err := r.contracts.ApproveRemote(context.Background(), token, contractsmodel.PublicApproveInput{Code: otp, ContentHash: cur.ContentHash}, client); !errors.Is(err, contractsusecase.ErrCodeInvalid) {
		t.Fatalf("code after lock = %v", err)
	}
	// A new code (after the cooldown) works.
	r.advance(61 * time.Second)
	_, err = r.contracts.RequestCode(context.Background(), token, client)
	r.must(err)
	if _, err := r.contracts.ApproveRemote(context.Background(), token, contractsmodel.PublicApproveInput{Code: r.lastCode(), ContentHash: cur.ContentHash}, client); err != nil {
		t.Fatalf("new code: %v", err)
	}
	var failed int32
	r.must(r.pool.QueryRow(context.Background(), `SELECT failed_attempts FROM document_approval_links WHERE organization_id = $1`, r.org).Scan(&failed))
	if failed != 6 {
		t.Fatalf("failed attempts = %d", failed)
	}
	// Activity has the failures (without the code).
	var n int
	r.must(r.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM activity_events WHERE organization_id = $1 AND action = 'documents.remote_approval_failed'`,
		r.org).Scan(&n))
	if n != 6 {
		t.Fatalf("failure entries = %d", n)
	}
}

func TestRemoteApprovalExpiredAndRevoked(t *testing.T) {
	r := newRemoteFx(t)
	r.rental()
	fifteen := 15
	_, token := r.link(&fifteen, false)
	r.advance(16 * time.Minute)
	page, err := r.contracts.PublicDocument(context.Background(), token)
	r.must(err)
	if page.State != contractsmodel.LinkExpired || page.HTML != "" || page.PhoneMasked != "" {
		t.Fatalf("expired page %+v", page)
	}
	var ls *contractsusecase.LinkStateError
	if _, err := r.contracts.RequestCode(context.Background(), token, client); !errors.As(err, &ls) || ls.State != contractsmodel.LinkExpired {
		t.Fatalf("code on an expired link = %v", err)
	}
	if len(r.fake.Messages()) != 0 {
		t.Fatal("no SMS for an expired link")
	}
	// Out-of-range expiry.
	cur := r.current()
	five := 5
	if _, err := r.contracts.CreateApprovalLink(r.ctx, r.org, r.doc, cur.UUID, contractsmodel.CreateLinkInput{
		ContentHash: cur.ContentHash, SignerRole: contractsmodel.SignerRenter, ExpiresInMinutes: &five,
	}); err == nil {
		t.Fatal("a 5 minute link must be refused")
	}

	// Revoked: a new link, a code, then staff revoke it.
	created, token2 := r.link(nil, false)
	_, err = r.contracts.RequestCode(context.Background(), token2, client)
	r.must(err)
	otp := r.lastCode()
	_, err = r.contracts.RevokeApprovalLink(r.ctx, r.org, r.doc, created.Link.UUID)
	r.must(err)
	page, err = r.contracts.PublicDocument(context.Background(), token2)
	r.must(err)
	if page.State != contractsmodel.LinkRevoked || page.HTML != "" {
		t.Fatalf("revoked page %+v", page)
	}
	if _, err := r.contracts.ApproveRemote(context.Background(), token2, contractsmodel.PublicApproveInput{Code: otp, ContentHash: cur.ContentHash}, client); !errors.As(err, &ls) || ls.State != contractsmodel.LinkRevoked {
		t.Fatalf("approve on a revoked link = %v", err)
	}
	if _, err := r.contracts.RevokeApprovalLink(r.ctx, r.org, r.doc, created.Link.UUID); err == nil {
		t.Fatal("revoking twice must be refused")
	}
	if _, err := r.contracts.SendApprovalLink(r.ctx, r.org, r.doc, created.Link.UUID); err == nil {
		t.Fatal("sending a revoked link must be refused")
	}
	// The first (expired) link is reported expired to staff too.
	links, err := r.contracts.ListApprovalLinks(r.ctx, r.org, r.doc)
	r.must(err)
	if len(links) != 2 || links[0].Status != contractsmodel.LinkRevoked || links[1].Status != contractsmodel.LinkExpired {
		t.Fatalf("links %+v", links)
	}
	doc, err := r.contracts.GetDocument(r.ctx, r.org, r.doc)
	r.must(err)
	if doc.Status != contractsmodel.DocumentDraft || len(doc.Approvals) != 0 {
		t.Fatalf("document %+v", doc)
	}
	// A new link replaces the open one.
	first, tokA := r.link(nil, false)
	_, tokB := r.link(nil, false)
	page, err = r.contracts.PublicDocument(context.Background(), tokA)
	r.must(err)
	if page.State != contractsmodel.LinkRevoked || first.Link.UUID == uuid.Nil {
		t.Fatalf("replaced link %+v", page)
	}
	page, err = r.contracts.PublicDocument(context.Background(), tokB)
	r.must(err)
	if page.State != contractsmodel.LinkPending {
		t.Fatalf("new link %+v", page)
	}
}

// TestRemoteApprovalNewVersion: a new version supersedes the open link
// (approve answers CONTENT_CHANGED even with a valid code) and invalidates
// an earlier remote approval.
func TestRemoteApprovalNewVersion(t *testing.T) {
	r := newRemoteFx(t)
	rental := r.rental()
	v1 := r.current()
	_, token := r.link(nil, false)
	page, err := r.contracts.PublicDocument(context.Background(), token)
	r.must(err)
	_, err = r.contracts.RequestCode(context.Background(), token, client)
	r.must(err)
	otp := r.lastCode()

	// The page is open; the rental is extended (amount change) → v2.
	r.advance(time.Hour)
	newEnd := rental.PlannedReturnAt.Add(24 * time.Hour)
	_, err = r.rentals.Extend(r.ctx, r.org, rental.UUID, model.ExtendInput{EndsAt: &newEnd})
	r.must(err)
	v2 := r.current()
	if v2.Version != 2 {
		t.Fatalf("version %d", v2.Version)
	}
	var cc *contractsusecase.ContentChangedError
	if _, err := r.contracts.ApproveRemote(context.Background(), token, contractsmodel.PublicApproveInput{Code: otp, ContentHash: page.ContentHash}, client); !errors.As(err, &cc) {
		t.Fatalf("approve after a new version = %v", err)
	}
	page, err = r.contracts.PublicDocument(context.Background(), token)
	r.must(err)
	if page.State != contractsmodel.LinkSuperseded || page.HTML != "" {
		t.Fatalf("superseded page %+v", page)
	}
	// Staff cannot create a link for the old version either.
	if _, err := r.contracts.CreateApprovalLink(r.ctx, r.org, r.doc, v1.UUID, contractsmodel.CreateLinkInput{
		ContentHash: v1.ContentHash, SignerRole: contractsmodel.SignerRenter,
	}); !errors.As(err, &cc) {
		t.Fatalf("link for v1 = %v", err)
	}

	// v2 approved remotely, then a vehicle-free change (extension) → v3:
	// the remote approval is invalid and the contract needs approval again.
	_, token2 := r.link(nil, false)
	r.advance(61 * time.Second)
	_, err = r.contracts.RequestCode(context.Background(), token2, client)
	r.must(err)
	_, err = r.contracts.ApproveRemote(context.Background(), token2, contractsmodel.PublicApproveInput{Code: r.lastCode(), ContentHash: v2.ContentHash}, client)
	r.must(err)
	state, err := r.contracts.ContractState(r.ctx, r.org, r.reservationID(rental))
	r.must(err)
	if !state.Approved {
		t.Fatal("v2 approved remotely")
	}
	newEnd = newEnd.Add(24 * time.Hour)
	_, err = r.rentals.Extend(r.ctx, r.org, rental.UUID, model.ExtendInput{EndsAt: &newEnd})
	r.must(err)
	doc, err := r.contracts.GetDocument(r.ctx, r.org, r.doc)
	r.must(err)
	if doc.Current.Version != 3 || doc.Status != contractsmodel.DocumentDraft || len(doc.Approvals) != 1 || doc.Approvals[0].Valid ||
		doc.Approvals[0].Method != contractsmodel.MethodRemoteSMS {
		t.Fatalf("after v3: %+v %+v", doc.Current, doc.Approvals)
	}
	state, err = r.contracts.ContractState(r.ctx, r.org, r.reservationID(rental))
	r.must(err)
	if state.Approved {
		t.Fatal("a new version must require a new approval")
	}
	// The approved link keeps its own PDF (what the customer approved).
	pdf, err := r.contracts.OpenPublicPDF(context.Background(), token2)
	r.must(err)
	_ = pdf.Body.Close()
	if !strings.Contains(r.pdf.html(), "A newer version invalidated this approval") {
		t.Fatal("the approval page must say the approval was invalidated")
	}
}

// TestRemoteApprovalOrganizationIsolation: a token resolves only its own
// organization's document; another organization's staff cannot list,
// revoke or download a link / approval of this one.
func TestRemoteApprovalOrganizationIsolation(t *testing.T) {
	a := newRemoteFx(t)
	a.rental()
	created, tokenA := a.link(nil, false)
	b := newRemoteFx(t)
	b.rental()
	_, tokenB := b.link(nil, false)

	pageA, err := a.contracts.PublicDocument(context.Background(), tokenA)
	r := a
	r.must(err)
	pageB, err := b.contracts.PublicDocument(context.Background(), tokenB)
	r.must(err)
	if pageA.OrganizationName == pageB.OrganizationName {
		t.Fatalf("tokens resolve their own documents: %s / %s", pageA.OrganizationName, pageB.OrganizationName)
	}
	// B's service (same database) resolves A's token to A's document only:
	// the organization comes from the token, never from the caller.
	viaB, err := b.contracts.PublicDocument(context.Background(), tokenA)
	r.must(err)
	if viaB.OrganizationName != pageA.OrganizationName {
		t.Fatal("token resolution must not depend on the caller")
	}
	// B's staff, naming A's records.
	if _, err := b.contracts.ListApprovalLinks(b.ctx, b.org, a.doc); !errors.Is(err, contractsusecase.ErrDocumentNotFound) {
		t.Fatalf("B lists A's links = %v", err)
	}
	if _, err := b.contracts.RevokeApprovalLink(b.ctx, b.org, a.doc, created.Link.UUID); !errors.Is(err, contractsusecase.ErrDocumentNotFound) {
		t.Fatalf("B revokes A's link = %v", err)
	}
	if _, err := b.contracts.RevokeApprovalLink(b.ctx, b.org, b.doc, created.Link.UUID); !errors.Is(err, contractsusecase.ErrLinkNotFound) {
		t.Fatalf("B revokes A's link under its own document = %v", err)
	}
	if _, err := b.contracts.SendApprovalLink(b.ctx, b.org, b.doc, created.Link.UUID); !errors.Is(err, contractsusecase.ErrLinkNotFound) {
		t.Fatalf("B sends A's link = %v", err)
	}
	// A's approval PDF is not reachable from B, and the token of A gives
	// nothing of B.
	_, err = a.contracts.RequestCode(context.Background(), tokenA, client)
	r.must(err)
	_, err = a.contracts.ApproveRemote(context.Background(), tokenA, contractsmodel.PublicApproveInput{Code: a.lastCode(), ContentHash: pageA.ContentHash}, client)
	r.must(err)
	doc, err := a.contracts.GetDocument(a.ctx, a.org, a.doc)
	r.must(err)
	if _, err := b.contracts.OpenApprovalPDF(b.ctx, b.org, b.doc, doc.Approvals[0].UUID); !errors.Is(err, contractsusecase.ErrApprovalNotFound) {
		t.Fatalf("B downloads A's approval = %v", err)
	}
	pdf, err := a.contracts.OpenPublicPDF(context.Background(), tokenA)
	r.must(err)
	_ = pdf.Body.Close()
	if !strings.Contains(a.pdf.html(), doc.Approvals[0].UUID.String()) {
		t.Fatal("A's token must stream A's own approval")
	}
	// B's link stays untouched; a forged token resolves nothing.
	pageB2, err := b.contracts.PublicDocument(context.Background(), tokenB)
	r.must(err)
	if pageB2.State != contractsmodel.LinkPending {
		t.Fatalf("B's link %+v", pageB2)
	}
	if _, err := a.contracts.PublicDocument(context.Background(), strings.Repeat("A", 43)); !errors.Is(err, contractsusecase.ErrLinkNotFound) {
		t.Fatalf("forged token = %v", err)
	}
	if _, err := a.contracts.PublicDocument(context.Background(), "short"); !errors.Is(err, contractsusecase.ErrLinkNotFound) {
		t.Fatalf("malformed token = %v", err)
	}
}

// TestRemoteApprovalRateLimits: resend cooldown, per-link code and failure
// caps (DB), and the per-IP / per-token windows of the public routes.
func TestRemoteApprovalRateLimits(t *testing.T) {
	r := newRemoteFx(t)
	r.rental()
	cur := r.current()
	lim := contractsusecase.DefaultRemoteLimits()
	lim.MaxCodesPerLink, lim.MaxFailuresPerLink = 2, 3
	r.contracts.SetRemoteLimits(lim)
	_, token := r.link(nil, false)
	_, err := r.contracts.RequestCode(context.Background(), token, client)
	r.must(err)
	var cd *contractsusecase.CooldownError
	if _, err := r.contracts.RequestCode(context.Background(), token, client); !errors.As(err, &cd) || cd.RetryAfter <= 0 {
		t.Fatalf("resend within the cooldown = %v", err)
	}
	r.advance(61 * time.Second)
	_, err = r.contracts.RequestCode(context.Background(), token, client)
	r.must(err)
	r.advance(61 * time.Second)
	if _, err := r.contracts.RequestCode(context.Background(), token, client); !errors.Is(err, contractsusecase.ErrRemoteRateLimited) {
		t.Fatalf("third code on a 2-code link = %v", err)
	}
	if n := len(r.fake.Messages()); n != 2 {
		t.Fatalf("SMS sent = %d", n)
	}
	otp := r.lastCode()
	for i := 0; i < 3; i++ {
		if _, err := r.contracts.ApproveRemote(context.Background(), token, contractsmodel.PublicApproveInput{Code: wrong(otp), ContentHash: cur.ContentHash}, client); !errors.Is(err, contractsusecase.ErrCodeInvalid) {
			t.Fatalf("wrong %d = %v", i, err)
		}
	}
	// The failure cap closes the link to codes, even the right one.
	if _, err := r.contracts.ApproveRemote(context.Background(), token, contractsmodel.PublicApproveInput{Code: otp, ContentHash: cur.ContentHash}, client); !errors.Is(err, contractsusecase.ErrRemoteRateLimited) {
		t.Fatalf("after the failure cap = %v", err)
	}

	// Public route windows (Redis limiter on miniredis).
	mr := miniredis.RunT(t)
	h := contractshandler.New(r.contracts, nil)
	h.SetRateLimiter(contracts.Limiter{L: ratelimit.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test")})
	mux := http.NewServeMux()
	contracts.RegisterRoutes(mux, h, nil, nil, nil)
	post := func(path, ip string) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", ip)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	other := strings.Repeat("B", 43)
	// Per token: 5 code requests per 10 minutes, whatever the IP.
	for i := 0; i < 5; i++ {
		if code := post("/v1/public/documents/"+other+"/code", "198.51.100."+string(rune('1'+i))); code != http.StatusNotFound {
			t.Fatalf("request %d = %d", i, code)
		}
	}
	if code := post("/v1/public/documents/"+other+"/code", "198.51.100.9"); code != http.StatusTooManyRequests {
		t.Fatalf("6th code request for one token = %d", code)
	}
	// Per IP: 10 code requests per 10 minutes, whatever the token.
	for i := 0; i < 10; i++ {
		tok := strings.Repeat(string(rune('a'+i)), 43)
		if code := post("/v1/public/documents/"+tok+"/code", "192.0.2.1"); code != http.StatusNotFound {
			t.Fatalf("ip request %d = %d", i, code)
		}
	}
	if code := post("/v1/public/documents/"+strings.Repeat("z", 43)+"/code", "192.0.2.1"); code != http.StatusTooManyRequests {
		t.Fatalf("11th code request from one IP = %d", code)
	}
	// Verify: 10 per token.
	for i := 0; i < 10; i++ {
		if code := post("/v1/public/documents/"+other+"/approve", "198.51.100.20"); code != http.StatusNotFound {
			t.Fatalf("verify %d = %d", i, code)
		}
	}
	if code := post("/v1/public/documents/"+other+"/approve", "198.51.100.21"); code != http.StatusTooManyRequests {
		t.Fatalf("11th verify for one token = %d", code)
	}
}
