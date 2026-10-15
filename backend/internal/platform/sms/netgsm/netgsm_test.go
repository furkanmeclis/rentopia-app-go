package netgsm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm/netgsmfake"
)

func setup(t *testing.T) (*netgsmfake.Fake, *netgsm.Adapter, integrations.Account) {
	t.Helper()
	f := netgsmfake.New()
	t.Cleanup(f.Server.Close)
	f.AddAccount("8501234567", "secret-pass", "RENTOPIA")
	a := netgsm.New(netgsm.Config{LiveBaseURL: "http://127.0.0.1:1", SandboxBaseURL: f.URL()})
	acc := integrations.Account{
		OrganizationID: 1, Kind: integrations.KindSMS, Provider: netgsm.ProviderCode, Mode: integrations.ModeSandbox,
		Credentials: integrations.Credentials{"usercode": "8501234567", "password": "secret-pass", "msgheader": "RENTOPIA"},
	}
	return f, a, acc
}

func TestNumberFormat(t *testing.T) {
	if got := netgsm.Number("+905321234567"); got != "5321234567" {
		t.Fatalf("TR: %s", got)
	}
	if got := netgsm.Number("+4915112345678"); got != "004915112345678" {
		t.Fatalf("intl: %s", got)
	}
}

func TestSendAndDeliveryReport(t *testing.T) {
	f, a, acc := setup(t)
	ctx := context.Background()
	res, err := a.Send(ctx, acc, sms.Message{To: "+905321234567", Body: "Merhaba", Class: sms.ClassTransactional})
	if err != nil || res.MessageID == "" {
		t.Fatalf("send: %+v %v", res, err)
	}
	msgs := f.Messages()
	if len(msgs) != 1 || msgs[0].No != "5321234567" || msgs[0].OTP || msgs[0].Header != "RENTOPIA" {
		t.Fatalf("recorded: %+v", msgs)
	}
	d, err := a.Status(ctx, acc, res.MessageID)
	if err != nil || d.State != sms.DeliveryPending {
		t.Fatalf("pending report: %+v %v", d, err)
	}
	f.SetStatus(res.MessageID, 1)
	d, err = a.Status(ctx, acc, res.MessageID)
	if err != nil || d.State != sms.DeliveryDelivered || d.At == nil {
		t.Fatalf("delivered report: %+v %v", d, err)
	}
	f.SetStatus(res.MessageID, 3)
	if d, _ := a.Status(ctx, acc, res.MessageID); d.State != sms.DeliveryUndelivered || d.Code != "3" {
		t.Fatalf("undelivered report: %+v", d)
	}
	if d, err := a.Status(ctx, acc, "unknown-job"); err != nil || d.State != sms.DeliveryNotFound {
		t.Fatalf("unknown job: %+v %v", d, err)
	}
	if _, err := a.Status(ctx, acc, ""); !errors.Is(err, integrations.ErrUnsupported) {
		t.Fatalf("no jobid: %v", err)
	}
	f.ReportErrorNext()
	if _, err := a.Status(ctx, acc, res.MessageID); !netgsm.IsUncertain(err) {
		t.Fatalf("report 503: %v", err)
	}
}

func TestOTPUsesOTPEndpoint(t *testing.T) {
	f, a, acc := setup(t)
	if _, err := a.Send(context.Background(), acc, sms.Message{To: "+905321234567", Body: "Kod 123456", Class: sms.ClassOTP}); err != nil {
		t.Fatal(err)
	}
	if m := f.Messages(); len(m) != 1 || !m[0].OTP {
		t.Fatalf("otp: %+v", m)
	}
}

func TestRejectionsAndUncertainty(t *testing.T) {
	f, a, acc := setup(t)
	ctx := context.Background()
	msg := sms.Message{To: "+905321234567", Body: "x", Class: sms.ClassTransactional}

	bad := acc
	bad.Credentials = integrations.Credentials{"usercode": "8501234567", "password": "secret-pass", "msgheader": "OTHER"}
	_, err := a.Send(ctx, bad, msg)
	var rej *sms.Rejected
	if !errors.As(err, &rej) || rej.Code != "40" || rej.Retryable {
		t.Fatalf("unknown header: %v", err)
	}
	wrong := acc
	wrong.Credentials = integrations.Credentials{"usercode": "8501234567", "password": "nope", "msgheader": "RENTOPIA"}
	if _, err := a.Send(ctx, wrong, msg); !errors.As(err, &rej) || rej.Code != "30" {
		t.Fatalf("bad credentials: %v", err)
	}
	f.FailNext("85")
	if _, err := a.Send(ctx, acc, msg); !errors.As(err, &rej) || !rej.Retryable {
		t.Fatalf("duplicate limit: %v", err)
	}
	// Accepted, then the answer is lost: unknown outcome, never a rejection.
	f.DropNext()
	_, err = a.Send(ctx, acc, msg)
	if !netgsm.IsUncertain(err) || errors.As(err, &rej) {
		t.Fatalf("dropped answer: %v", err)
	}
	if n := len(f.Messages()); n != 1 {
		t.Fatalf("the dropped message was accepted by the fake: %d", n)
	}
	f.ServerErrorNext()
	if _, err := a.Send(ctx, acc, msg); !netgsm.IsUncertain(err) {
		t.Fatalf("503: %v", err)
	}
	live := acc
	live.Mode = integrations.ModeLive
	if _, err := a.Send(ctx, live, msg); !netgsm.IsUncertain(err) {
		t.Fatalf("unreachable live endpoint must be uncertain: %v", err)
	}
	noSandbox := netgsm.New(netgsm.Config{})
	if _, err := noSandbox.Send(ctx, acc, msg); !errors.As(err, &rej) {
		t.Fatalf("sandbox without endpoint: %v", err)
	}
}

func TestCheck(t *testing.T) {
	_, a, acc := setup(t)
	if err := a.Check(context.Background(), acc); err != nil {
		t.Fatalf("check: %v", err)
	}
	acc.Credentials = integrations.Credentials{"usercode": "8501234567", "password": "secret-pass", "msgheader": "OTHER"}
	if err := a.Check(context.Background(), acc); err == nil {
		t.Fatal("check must refuse an undefined header")
	}
}
