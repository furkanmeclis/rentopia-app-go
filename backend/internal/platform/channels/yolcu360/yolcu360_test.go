package yolcu360_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels/yolcu360"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func account(base string) integrations.Account {
	return integrations.Account{
		Provider: yolcu360.ProviderCode, Mode: integrations.ModeSandbox,
		Credentials: integrations.Credentials{"supplier_code": "SUP1", "api_key": "key-1", "api_secret": "secret-1", "base_url": base},
	}
}

// verify checks the documented signature headers.
func verify(t *testing.T, r *http.Request, body []byte) {
	t.Helper()
	if r.Header.Get("X-Api-Key") != "key-1" {
		t.Errorf("api key header = %q", r.Header.Get("X-Api-Key"))
	}
	want := yolcu360.Sign("secret-1", r.Header.Get("X-Timestamp"), r.Method, r.URL.Path, body)
	if r.Header.Get("X-Signature") != want {
		t.Errorf("signature mismatch for %s %s", r.Method, r.URL.Path)
	}
}

func TestPushPullAckStatement(t *testing.T) {
	var pushed map[string]any
	var acked map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		verify(t, r, body)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v1/suppliers/SUP1/inventory":
			_ = json.Unmarshal(body, &pushed)
			_, _ = w.Write([]byte(`{"batch_id":"B-1","accepted":1,"rejected":1,"errors":[{"index":1,"code":"unknown_location","message":"x"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/suppliers/SUP1/booking-events":
			if r.URL.Query().Get("cursor") != "c0" {
				t.Errorf("cursor = %q", r.URL.Query().Get("cursor"))
			}
			_, _ = w.Write([]byte(`{"events":[{"event_id":"ev-1","type":"booking.modified","booking_ref":"Y-1","version":2,
				"occurred_at":"2026-10-01T10:00:00Z","status":"confirmed","booking":{"product_code":"ECO","offer_ref":"OF1",
				"pickup":{"location_code":"IST","at":"2026-10-10T07:00:00Z"},"dropoff":{"location_code":"SAW","at":"2026-10-12T07:00:00Z"},
				"customer":{"first_name":"Ali","last_name":"Can","email":"a@example.test","phone":"+905551112233","nationality":"TR"},
				"extras":[{"code":"BABY","quantity":1}],
				"payment":{"currency":"TRY","total":"2500.00","commission":"375.00","prepaid":"2500.00","collected_by":"channel"}}}],
				"next_cursor":"c1","has_more":false}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/suppliers/SUP1/booking-events/ack":
			_ = json.Unmarshal(body, &acked)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/suppliers/SUP1/statements":
			_, _ = w.Write([]byte(`{"lines":[{"booking_ref":"Y-1","status":"CONFIRMED","pickup_at":"2026-10-10T07:00:00Z","currency":"TRY",
				"total":"2500.00","commission":"375.00","net":"2125.00","collected_by":"supplier"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	a := yolcu360.New(srv.Client())
	acc := account(srv.URL)
	ctx := context.Background()

	r, err := a.PushInventory(ctx, acc, channels.InventoryPush{
		ValidUntil: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), Offline: channels.OfflinePolicy{Mode: "quota", Quota: 1},
		Items: []channels.InventoryItem{
			{ProductCode: "ECO", LocationCode: "IST", Date: "2026-10-02", Available: 3, HasPrice: true, DailyPrice: money.MustParse("1000"), Currency: "TRY", OfferRef: "OF1"},
			{ProductCode: "ECO", LocationCode: "XXX", Date: "2026-10-02", StopSale: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Reference != "B-1" || r.Accepted != 1 || r.Rejected != 1 || len(r.Errors) != 1 {
		t.Fatalf("receipt = %+v", r)
	}
	items := pushed["items"].([]any)
	first := items[0].(map[string]any)
	if first["price"].(map[string]any)["amount"] != "1000.00" || items[1].(map[string]any)["price"] != nil {
		t.Fatalf("pushed items = %+v", items)
	}

	batch, err := a.PullMessages(ctx, acc, "c0")
	if err != nil {
		t.Fatal(err)
	}
	if batch.Cursor != "c1" || batch.More || len(batch.Messages) != 1 {
		t.Fatalf("batch = %+v", batch)
	}
	m := batch.Messages[0]
	if m.Type != channels.MessageModification || m.ExternalRef != "Y-1" || m.Version != 2 || m.ReturnLocationCode != "SAW" ||
		!m.Total.Equal(money.MustParse("2500")) || !m.Commission.Equal(money.MustParse("375")) || m.CollectedBy != channels.CollectedByChannel ||
		len(m.Extras) != 1 || m.Customer.FirstName != "Ali" || m.OfferRef != "OF1" {
		t.Fatalf("message = %+v", m)
	}

	if err := a.Acknowledge(ctx, acc, []channels.Ack{{MessageID: "ev-1", ExternalRef: "Y-1", Accepted: true, Reference: "R-1"}}); err != nil {
		t.Fatal(err)
	}
	ack := acked["acks"].([]any)[0].(map[string]any)
	if ack["event_id"] != "ev-1" || ack["supplier_ref"] != "R-1" || ack["accepted"] != true {
		t.Fatalf("ack = %+v", ack)
	}

	lines, err := a.Statement(ctx, acc, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Status != "confirmed" || lines[0].CollectedBy != channels.CollectedBySupplier {
		t.Fatalf("statement = %+v", lines)
	}
}

func TestErrorClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/ping") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"invalid_key","message":"bad key"}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	a := yolcu360.New(srv.Client())
	ctx := context.Background()
	if err := a.Check(ctx, account(srv.URL)); !errors.Is(err, channels.ErrRejected) || !strings.Contains(err.Error(), "invalid_key") {
		t.Fatalf("401 = %v", err)
	}
	_, err := a.PullMessages(ctx, account(srv.URL), "")
	if err == nil || errors.Is(err, channels.ErrRejected) || errors.Is(err, channels.ErrUnavailable) {
		t.Fatalf("502 must be an unknown outcome: %v", err)
	}
	// Nothing listens: the request never left.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	if err := a.Check(ctx, account("http://"+addr)); !errors.Is(err, channels.ErrUnavailable) {
		t.Fatalf("refused = %v", err)
	}
	// Only https (or a loopback test server) is accepted as base_url.
	if err := a.Check(ctx, account("http://example.com")); !errors.Is(err, channels.ErrRejected) {
		t.Fatalf("plain http = %v", err)
	}
}
