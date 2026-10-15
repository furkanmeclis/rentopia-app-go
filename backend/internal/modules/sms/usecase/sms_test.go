package usecase_test

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm/netgsmfake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const phone = "0532 123 45 67"

type env struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	org    int64
	fake   *netgsmfake.Fake
	svc    *usecase.Service
	runner *jobsusecase.Runner
	now    time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "sms-" + suffix, Name: "SMS " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID) })
	fake := netgsmfake.New()
	t.Cleanup(fake.Server.Close)
	fake.AddAccount("8501112233", "pw-"+suffix, "RENTOPIA")
	adapter := netgsm.New(netgsm.Config{SandboxBaseURL: fake.URL()})
	fc, err := crypto.NewFieldCipher(strings.Repeat("s", 32))
	if err != nil {
		t.Fatal(err)
	}
	vault, err := integrations.NewVault(fc)
	if err != nil {
		t.Fatal(err)
	}
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(adapter))
	if _, err := accounts.Connect(ctx, org.ID, accountsusecase.ConnectInput{
		Provider: "netgsm", DisplayName: "Netgsm", Mode: "sandbox",
		Credentials: map[string]string{"usercode": "8501112233", "password": "pw-" + suffix, "msgheader": "RENTOPIA"},
	}); err != nil {
		t.Fatal(err)
	}
	ops := integrations.NewOperationRegistry()
	runner := jobsusecase.NewRunner(q, accounts, ops)
	svc, err := usecase.New(pool, q, fc, accounts, runner, ops, adapter)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetActivity(activity.NewRecorder(q, nil))
	e := &env{ctx: ctx, pool: pool, q: q, org: org.ID, fake: fake, svc: svc, runner: runner, now: time.Now()}
	svc.SetClock(func() time.Time { return e.now })
	return e
}

func (e *env) jobID(t *testing.T, id uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := e.pool.QueryRow(e.ctx, "SELECT id FROM integration_jobs WHERE organization_id = $1 AND uuid = $2", e.org, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) message(t *testing.T, id uuid.UUID) (status, dlr, ciphertext, body string) {
	t.Helper()
	if err := e.pool.QueryRow(e.ctx, `SELECT status, dlr_status, payload_ciphertext, body_redacted FROM sms_messages
		WHERE organization_id = $1 AND uuid = $2`, e.org, id).Scan(&status, &dlr, &ciphertext, &body); err != nil {
		t.Fatal(err)
	}
	return
}

var codeRE = regexp.MustCompile(`\b(\d{6})\b`)

func (e *env) lastCode(t *testing.T) string {
	t.Helper()
	msgs := e.fake.Messages()
	if len(msgs) == 0 {
		t.Fatal("no SMS reached the fake")
	}
	m := codeRE.FindStringSubmatch(msgs[len(msgs)-1].Text)
	if m == nil {
		t.Fatalf("no code in %q", msgs[len(msgs)-1].Text)
	}
	return m[1]
}

// TestSendWithDeliveryReport: submit → submitted (sealed payload kept) →
// delivery report → delivered (payload dropped).
func TestSendWithDeliveryReport(t *testing.T) {
	e := newEnv(t)
	sent, err := e.svc.Send(e.ctx, e.org, usecase.SendInput{
		Class: sms.ClassTransactional, Purpose: "reservation_notice", Phone: phone, Body: "Rezervasyonunuz onaylandı.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent.Status != usecase.StatusSubmitted || sent.JobStatus != jobsusecase.StatusSubmitted || sent.PhoneMasked != "+90 5** *** ** 67" {
		t.Fatalf("sent: %+v", sent)
	}
	status, _, cipher, _ := e.message(t, sent.MessageUUID)
	if status != usecase.StatusSubmitted || cipher == "" || strings.Contains(cipher, "532") {
		t.Fatalf("message: %s %q", status, cipher)
	}
	msgs := e.fake.Messages()
	if len(msgs) != 1 || msgs[0].No != "5321234567" {
		t.Fatalf("fake: %+v", msgs)
	}
	// Report still pending: stays submitted.
	if _, err := e.runner.Process(e.ctx, e.org, e.jobID(t, sent.JobUUID), jobsusecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	if st, _, _, _ := e.message(t, sent.MessageUUID); st != usecase.StatusSubmitted {
		t.Fatalf("pending report: %s", st)
	}
	e.fake.SetStatus(msgs[0].JobID, 1)
	if _, err := e.runner.Process(e.ctx, e.org, e.jobID(t, sent.JobUUID), jobsusecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	status, dlr, cipher, _ := e.message(t, sent.MessageUUID)
	if status != usecase.StatusDelivered || dlr != "1" || cipher != "" {
		t.Fatalf("after report: %s dlr=%q cipher=%q", status, dlr, cipher)
	}
	if e.fake.Sends.Load() != 1 {
		t.Fatalf("sends: %d", e.fake.Sends.Load())
	}
}

// An undelivered report fails the message; a lost answer leaves it unknown
// and is never sent twice automatically.
func TestSendUndeliveredAndUncertain(t *testing.T) {
	e := newEnv(t)
	sent, err := e.svc.Send(e.ctx, e.org, usecase.SendInput{Class: sms.ClassTransactional, Purpose: "x_notice", Phone: phone, Body: "A"})
	if err != nil {
		t.Fatal(err)
	}
	e.fake.SetStatus(e.fake.Messages()[0].JobID, 3)
	if _, err := e.runner.Process(e.ctx, e.org, e.jobID(t, sent.JobUUID), jobsusecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	if st, dlr, cipher, _ := e.message(t, sent.MessageUUID); st != usecase.StatusUndelivered || dlr != "3" || cipher != "" {
		t.Fatalf("undelivered: %s %s %q", st, dlr, cipher)
	}

	e.fake.DropNext()
	lost, err := e.svc.Send(e.ctx, e.org, usecase.SendInput{Class: sms.ClassTransactional, Purpose: "x_notice", Phone: phone, Body: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if lost.Status != usecase.StatusUnknown || lost.JobStatus != jobsusecase.StatusNeedsVerification {
		t.Fatalf("lost answer: %+v", lost)
	}
	id := e.jobID(t, lost.JobUUID)
	for range 3 {
		if _, err := e.runner.Process(e.ctx, e.org, id, jobsusecase.TriggerSweep); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.runner.Retry(e.ctx, e.org, lost.JobUUID); err == nil {
		t.Fatal("retry of an unverifiable send must refuse")
	}
	if n := e.fake.Sends.Load(); n != 2 {
		t.Fatalf("the lost message was sent again: %d sends", n)
	}
}

func TestInformationalNeedsConsent(t *testing.T) {
	e := newEnv(t)
	sent, err := e.svc.Send(e.ctx, e.org, usecase.SendInput{Class: sms.ClassInformational, Purpose: "info", Phone: phone, Body: "Bilgi"})
	if !errors.Is(err, usecase.ErrNoConsent) {
		t.Fatalf("informational without customer: %v", err)
	}
	if st, _, _, _ := e.message(t, sent.MessageUUID); st != usecase.StatusBlocked {
		t.Fatalf("blocked row: %s", st)
	}
	if e.fake.Sends.Load() != 0 {
		t.Fatal("a blocked message reached the provider")
	}
}

// OTP: only an HMAC of the code is stored, the log keeps it redacted, a
// correct code verifies once, wrong codes count and lock.
func TestOTPHashOnlyAndVerify(t *testing.T) {
	e := newEnv(t)
	subject := "document_version:" + uuid.NewString()
	ch, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", subject, phone)
	if err != nil {
		t.Fatal(err)
	}
	code := e.lastCode(t)
	if !e.fake.Messages()[0].OTP {
		t.Fatal("OTP must use the OTP endpoint")
	}
	// Nowhere in the database in plaintext.
	var hits int
	if err := e.pool.QueryRow(e.ctx, `SELECT
		(SELECT COUNT(*) FROM sms_otp_challenges o WHERE o.organization_id = $1 AND o::text LIKE '%' || $2 || '%') +
		(SELECT COUNT(*) FROM sms_messages m WHERE m.organization_id = $1 AND m::text LIKE '%' || $2 || '%') +
		(SELECT COUNT(*) FROM integration_jobs j WHERE j.organization_id = $1 AND j::text LIKE '%' || $2 || '%') +
		(SELECT COUNT(*) FROM integration_attempts a WHERE a.organization_id = $1 AND a::text LIKE '%' || $2 || '%') +
		(SELECT COUNT(*) FROM activity_events v WHERE v.organization_id = $1 AND v::text LIKE '%' || $2 || '%')`,
		e.org, code).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("the code appears in %d rows", hits)
	}
	var hash string
	if err := e.pool.QueryRow(e.ctx, "SELECT code_hash FROM sms_otp_challenges WHERE organization_id = $1 AND uuid = $2", e.org, ch.UUID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 || hash == code {
		t.Fatalf("code hash: %q", hash)
	}
	if _, _, _, body := e.message(t, ch.Message.MessageUUID); !strings.Contains(body, "••••••") || strings.Contains(body, code) {
		t.Fatalf("logged body: %q", body)
	}

	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	_, err = e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", subject, wrong)
	var inv *usecase.InvalidCodeError
	if !errors.As(err, &inv) || inv.AttemptsLeft != 4 || !errors.Is(err, usecase.ErrOTPInvalid) {
		t.Fatalf("wrong code: %v", err)
	}
	if _, err := e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", "other:"+subject, code); !errors.Is(err, usecase.ErrOTPNotFound) {
		t.Fatalf("other subject: %v", err)
	}
	if _, err := e.svc.VerifyOTP(e.ctx, e.org, "other_purpose", subject, code); !errors.Is(err, usecase.ErrOTPNotFound) {
		t.Fatalf("other purpose: %v", err)
	}
	v, err := e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", subject, " "+code[:3]+" "+code[3:])
	if err != nil || v.ChallengeUUID != ch.UUID {
		t.Fatalf("verify: %+v %v", v, err)
	}
	if _, err := e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", subject, code); !errors.Is(err, usecase.ErrOTPNotFound) {
		t.Fatalf("second use: %v", err)
	}
	if _, _, cipher, _ := e.message(t, ch.Message.MessageUUID); cipher != "" {
		t.Fatal("the sealed OTP text must be dropped after verification")
	}
	var audits int
	if err := e.pool.QueryRow(e.ctx, `SELECT COUNT(*) FROM activity_events WHERE organization_id = $1 AND action LIKE 'sms.otp.%'`, e.org).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits < 4 {
		t.Fatalf("audit entries: %d", audits)
	}
}

func TestOTPAttemptsLock(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s1", phone); err != nil {
		t.Fatal(err)
	}
	code := e.lastCode(t)
	wrong := "999999"
	if wrong == code {
		wrong = "888888"
	}
	for i := range 5 {
		_, err := e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", "s1", wrong)
		if i < 4 && !errors.Is(err, usecase.ErrOTPInvalid) {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if i == 4 && !errors.Is(err, usecase.ErrOTPLocked) {
			t.Fatalf("5th wrong code must lock: %v", err)
		}
	}
	if _, err := e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", "s1", code); !errors.Is(err, usecase.ErrOTPNotFound) {
		t.Fatalf("the right code after the lock: %v", err)
	}
}

func TestOTPTTL(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s2", phone); err != nil {
		t.Fatal(err)
	}
	code := e.lastCode(t)
	e.now = e.now.Add(5*time.Minute + time.Second)
	if _, err := e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", "s2", code); !errors.Is(err, usecase.ErrOTPExpired) {
		t.Fatalf("expired: %v", err)
	}
	if _, err := e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", "s2", code); !errors.Is(err, usecase.ErrOTPNotFound) {
		t.Fatalf("after expiry: %v", err)
	}
}

func TestOTPCooldownAndRateLimits(t *testing.T) {
	e := newEnv(t)
	limits := usecase.DefaultLimits()
	limits.OTPPerPhoneHour = 2
	limits.OTPPerOrgHour = 3
	e.svc.SetLimits(limits)
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s3", phone); err != nil {
		t.Fatal(err)
	}
	first := e.lastCode(t)
	_, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s3", phone)
	var cd *usecase.CooldownError
	if !errors.As(err, &cd) || cd.RetryAfter <= 0 {
		t.Fatalf("cooldown: %v", err)
	}
	e.now = e.now.Add(61 * time.Second)
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s3", phone); err != nil {
		t.Fatalf("after the cooldown: %v", err)
	}
	second := e.lastCode(t)
	if first != second {
		// The superseded code no longer verifies.
		if _, err := e.svc.VerifyOTP(e.ctx, e.org, "contract_approval", "s3", first); !errors.Is(err, usecase.ErrOTPInvalid) {
			t.Fatalf("superseded code: %v", err)
		}
	}
	// Per number: 2 per hour.
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s4", phone); !errors.Is(err, usecase.ErrOTPRateLimited) {
		t.Fatalf("per-phone limit: %v", err)
	}
	// Per organization: 3 per hour (another number).
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s5", "05329998877"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s6", "05329998866"); !errors.Is(err, usecase.ErrOTPRateLimited) {
		t.Fatalf("per-org limit: %v", err)
	}
	// An hour later both windows are free again.
	e.now = e.now.Add(61 * time.Minute)
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s4", phone); err != nil {
		t.Fatalf("after the window: %v", err)
	}
}

func TestOTPValidation(t *testing.T) {
	e := newEnv(t)
	var ve *usecase.ValidationError
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "Bad Purpose", "s", phone); !errors.As(err, &ve) {
		t.Fatalf("purpose: %v", err)
	}
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "", phone); !errors.As(err, &ve) {
		t.Fatalf("subject: %v", err)
	}
	if _, err := e.svc.IssueOTP(e.ctx, e.org, "contract_approval", "s", "12"); !errors.As(err, &ve) {
		t.Fatalf("phone: %v", err)
	}
}
