package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestIsSecretKey(t *testing.T) {
	secret := []string{"password", "new_password", "access_token", "Authorization", "api_key", "apiKey", "client-secret",
		"iban", "payout.IBAN", "card", "card_number", "cvv", "otp", "sms.otp_code", "private_key", "S3_SECRET_KEY"}
	for _, k := range secret {
		if !IsSecretKey(k) {
			t.Errorf("%q must be secret", k)
		}
	}
	plain := []string{"request_id", "organization_uuid", "status", "route", "plate", "duration_ms", "job_id", "kind", "capacity"}
	for _, k := range plain {
		if IsSecretKey(k) {
			t.Errorf("%q must not be secret", k)
		}
	}
}

func TestRedactString(t *testing.T) {
	cases := map[string]string{
		"Authorization: Bearer abc.def-123":                      "Authorization: Bearer [token]",
		"token=s3cr3t&x=1":                                       "token=[redacted]&x=1",
		`{"password":"hunter2","plate":"34 AB 12"}`:              `{"password":"[redacted]","plate":"34 AB 12"}`,
		"iban TR33 0006 1005 1978 6457 8413 26 rejected":         "iban [iban] rejected",
		"card 4111 1111 1111 1111 declined":                      "card [card] declined",
		"order 1234567890123 (not a card)":                       "order 1234567890123 (not a card)",
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl ": "jwt [token] ",
	}
	for in, want := range cases {
		if got := RedactString(in); got != want {
			t.Errorf("RedactString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactValueNested(t *testing.T) {
	got := RedactValue("", map[string]any{
		"user":    map[string]any{"id": 7, "password": "x"},
		"headers": map[string]string{"Authorization": "Basic Zm9vOmJhcg==", "Accept": "json"},
		"err":     errors.New("api_key=abc failed"),
	}).(map[string]any)
	if got["user"].(map[string]any)["password"] != Redacted || got["user"].(map[string]any)["id"] != 7 {
		t.Fatalf("nested map: %v", got["user"])
	}
	if h := got["headers"].(map[string]string); h["Authorization"] != Redacted || h["Accept"] != "json" {
		t.Fatalf("headers: %v", h)
	}
	if got["err"] != "api_key=[redacted] failed" {
		t.Fatalf("error: %v", got["err"])
	}
}

// The stdout JSON logger masks secrets and adds request / organization
// ids from the context.
func TestJSONLoggerRedactsAndCarriesContext(t *testing.T) {
	prevReq, prevOrg := RequestIDFromContext, OrganizationFromContext
	t.Cleanup(func() { RequestIDFromContext, OrganizationFromContext = prevReq, prevOrg })
	type key struct{}
	RequestIDFromContext = func(ctx context.Context) string { s, _ := ctx.Value(key{}).(string); return s }
	OrganizationFromContext = func(context.Context) string { return "org-uuid" }

	var buf bytes.Buffer
	log := slog.New(contextHandler{next: slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: RedactAttr})})
	ctx := context.WithValue(context.Background(), key{}, "req-1")
	log.With("api_key", "k-123").WarnContext(ctx, "provider_failed",
		"password", "hunter2", "iban", "TR330006100519786457841326",
		slog.Group("card", "number", "4111111111111111"),
		"error", errors.New("Bearer abc123 rejected"), "status", 502)

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	for _, k := range []string{"api_key", "password", "iban"} {
		if rec[k] != Redacted {
			t.Errorf("%s not redacted: %v", k, rec[k])
		}
	}
	if card, _ := rec["card"].(map[string]any); card["number"] != "[card]" {
		t.Errorf("card group not masked: %v", rec["card"])
	}
	for _, leak := range []string{"hunter2", "k-123", "4111111111111111", "TR3300", "abc123"} {
		if strings.Contains(buf.String(), leak) {
			t.Fatalf("leaked %q: %s", leak, buf.String())
		}
	}
	if rec["level"] != "WARN" || rec["request_id"] != "req-1" || rec["organization_uuid"] != "org-uuid" || rec["status"] != float64(502) {
		t.Fatalf("record: %v", rec)
	}
}

// Persisted entries (the log viewer) are masked too.
func TestPersistRedacts(t *testing.T) {
	w := &memWriter{}
	p := Attach(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), w)
	p.Logger().Error("login_failed", "password", "hunter2", "detail", "token=abc", "user_id", 7)
	p.Close()
	deadline := time.Now().Add(time.Second)
	for len(w.all()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	entries := w.all()
	if len(entries) != 1 {
		t.Fatalf("entries: %d", len(entries))
	}
	a := entries[0].Attrs
	if a["password"] != Redacted || a["detail"] != "token=[redacted]" || a["user_id"] != int64(7) {
		t.Fatalf("attrs: %v", a)
	}
}
