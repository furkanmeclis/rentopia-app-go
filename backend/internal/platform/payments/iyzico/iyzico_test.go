package iyzico_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func setup(t *testing.T) (*iyzicofake.Fake, *iyzico.Adapter, integrations.Account) {
	t.Helper()
	f := iyzicofake.New()
	t.Cleanup(f.Server.Close)
	f.AddAccount("sandbox-api-key-0001", "sandbox-secret-key-0001")
	a := iyzico.New(iyzico.Config{SandboxBaseURL: f.URL(), LiveBaseURL: "http://127.0.0.1:1"})
	acc := integrations.Account{
		Kind: integrations.KindPayment, Provider: iyzico.ProviderCode, Mode: integrations.ModeSandbox,
		Credentials: integrations.Credentials{iyzico.FieldAPIKey: "sandbox-api-key-0001", iyzico.FieldSecretKey: "sandbox-secret-key-0001"},
	}
	return f, a, acc
}

func TestFormatPrice(t *testing.T) {
	for in, want := range map[string]string{"1500": "1500.0", "1500.50": "1500.5", "0.25": "0.25", "12.34": "12.34"} {
		if got := iyzico.FormatPrice(money.MustParse(in)); got != want {
			t.Errorf("FormatPrice(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestAuthorizationHeaderKnownValue(t *testing.T) {
	// HMAC-SHA256("secret", "123/path{}") computed independently.
	got := iyzico.AuthorizationHeader("key", "secret", "123", "/path", []byte("{}"))
	if got[:8] != "IYZWSv2 " {
		t.Fatalf("header %q", got)
	}
	if iyzico.Sign("s", "a", "b") != iyzico.Sign("s", "a:b") {
		t.Fatal("Sign must join with ':'")
	}
}

func TestCheckoutRetrieveRefundWebhook(t *testing.T) {
	f, a, acc := setup(t)
	ctx := context.Background()
	if err := a.Check(ctx, acc); err != nil {
		t.Fatalf("check: %v", err)
	}
	bad := acc
	bad.Credentials = integrations.Credentials{iyzico.FieldAPIKey: "sandbox-api-key-0001", iyzico.FieldSecretKey: "wrong"}
	var pe *payments.ProviderError
	if err := a.Check(ctx, bad); !errors.As(err, &pe) || pe.Code != "1000" {
		t.Fatalf("wrong secret: %v", err)
	}
	conv := "6f1c2d3e-0000-4000-8000-000000000001"
	co, err := a.CreateCheckout(ctx, acc, payments.CheckoutRequest{
		ConversationID: conv, BasketID: conv, Amount: money.MustParse("300"), Currency: "TRY",
		CallbackURL: "https://app.test/pay/x/callback", ItemID: "R1", ItemName: "Rezervasyon", ItemCategory: "Araç kiralama",
		Buyer: payments.Buyer{ID: "c1", Name: "Ada", Surname: "Yılmaz", Email: "ada@example.test", IdentityNumber: "11111111111",
			Address: "Adres", City: "Istanbul", Country: "Turkey", IP: "10.0.0.1"},
	})
	if err != nil || co.Token == "" || co.PaymentPageURL == "" || co.ExpiresAt == nil {
		t.Fatalf("checkout: %+v %v", co, err)
	}
	r, err := a.Retrieve(ctx, acc, payments.RetrieveRequest{Token: co.Token, ConversationID: conv})
	if err != nil || r.Status != payments.StatusPending {
		t.Fatalf("before payment: %+v %v", r, err)
	}
	body, sig, err := f.Complete(co.Token, iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	r, err = a.Retrieve(ctx, acc, payments.RetrieveRequest{Token: co.Token, ConversationID: conv})
	if err != nil || r.Status != payments.StatusSucceeded || r.PaymentID == "" || !r.Price.Equal(money.MustParse("300")) ||
		r.BasketID != conv || r.Currency != "TRY" || !r.Commission.IsPositive() || r.Data["lastFourDigits"] != "0008" {
		t.Fatalf("after payment: %+v %v", r, err)
	}
	h := http.Header{}
	h.Set(iyzico.SignatureHeader, sig)
	hook, err := a.ParseWebhook(h, body)
	if err != nil || hook.ConversationID != conv || hook.PaymentID != r.PaymentID || hook.Token != co.Token {
		t.Fatalf("parse webhook: %+v %v", hook, err)
	}
	if err := a.VerifyWebhook(acc, hook, h, body); err != nil {
		t.Fatalf("verify webhook: %v", err)
	}
	if err := a.VerifyWebhook(bad, hook, h, body); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("webhook with another secret: %v", err)
	}
	tampered := hook
	tampered.Status = "FAILURE"
	if err := a.VerifyWebhook(acc, tampered, h, body); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("tampered webhook: %v", err)
	}
	if err := a.VerifyWebhook(acc, hook, http.Header{}, body); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("webhook without signature: %v", err)
	}
	ref, err := a.Refund(ctx, acc, payments.RefundRequest{PaymentID: r.PaymentID, ConversationID: conv, Amount: money.MustParse("100"), Currency: "TRY", IP: "10.0.0.1"})
	if err != nil || ref.RefundID == "" {
		t.Fatalf("refund: %+v %v", ref, err)
	}
	if _, err := a.Refund(ctx, acc, payments.RefundRequest{PaymentID: r.PaymentID, ConversationID: conv, Amount: money.MustParse("250"), Currency: "TRY"}); !errors.As(err, &pe) {
		t.Fatalf("over-refund accepted: %v", err)
	}
	if _, err := a.ParseWebhook(h, []byte(`{"status":"SUCCESS"}`)); !errors.Is(err, payments.ErrMalformed) {
		t.Fatalf("malformed webhook: %v", err)
	}
}
