package obilet_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels/obilet"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

var accountID = uuid.New()

func account(base string) integrations.Account {
	return integrations.Account{
		UUID: accountID, Provider: obilet.ProviderCode, Mode: integrations.ModeSandbox,
		Credentials: integrations.Credentials{"partner_code": "P1", "api_token": "tok-1", "base_url": base},
	}
}

type req struct {
	DeviceSession *struct {
		SessionID string `json:"session-id"`
		DeviceID  string `json:"device-id"`
	} `json:"device-session"`
	Date     string         `json:"date"`
	Language string         `json:"language"`
	Data     map[string]any `json:"data"`
}

func ok(w http.ResponseWriter, data string) {
	_, _ = w.Write([]byte(`{"status":"Success","data":` + data + `,"message":null,"user-message":null}`))
}

// server answers the documented session call and hands every operation to fn
// after checking the token, the session and the partner code.
func server(t *testing.T, sessions *int32, fn func(w http.ResponseWriter, path string, r req)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, hr *http.Request) {
		if hr.Method != http.MethodPost || hr.Header.Get("Authorization") != "Basic tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":"Unauthorized","message":"bad token"}`))
			return
		}
		body, _ := io.ReadAll(hr.Body)
		if hr.URL.Path == "/api/client/getsession" {
			n := atomic.AddInt32(sessions, 1)
			ok(w, `{"session-id":"S`+string(rune('0'+n))+`","device-id":"D1"}`)
			return
		}
		var r req
		if err := json.Unmarshal(body, &r); err != nil {
			t.Errorf("body: %v", err)
		}
		if r.DeviceSession == nil || r.DeviceSession.SessionID == "" || r.Language != "tr-TR" || r.Data["partner-code"] != "P1" {
			t.Errorf("envelope = %s", body)
		}
		if _, err := time.Parse("2006-01-02T15:04:05", r.Date); err != nil {
			t.Errorf("date = %q", r.Date)
		}
		fn(w, hr.URL.Path, r)
	}))
}

func TestPushPullAckStatement(t *testing.T) {
	var sessions int32
	var pushed, acked map[string]any
	srv := server(t, &sessions, func(w http.ResponseWriter, path string, r req) {
		switch path {
		case "/api/carrental/supplier/inventory/update":
			pushed = r.Data
			ok(w, `{"batch-id":"B-1","accepted":1,"rejected":1,"errors":[{"index":1,"code":"unknown_location","message":"x"}]}`)
		case "/api/carrental/supplier/bookings/getchanges":
			if r.Data["cursor"] != "c0" {
				t.Errorf("cursor = %v", r.Data["cursor"])
			}
			ok(w, `{"changes":[{"change-id":"ch-1","change-type":"Updated","pnr":"OB-1","version":2,
				"change-date":"2026-10-01T13:00:00","status":"Confirmed","booking":{"product-code":"ECO","offer-ref":"OF1",
				"pickup":{"location-code":"IST","date":"2026-10-10T10:00:00"},"return":{"location-code":"SAW","date":"2026-10-12T10:00:00"},
				"passenger":{"name":"Ali","surname":"Can","email":"a@example.test","phone":"+905551112233","nationality":"TR"},
				"extras":[{"code":"BABY","quantity":1}],
				"payment":{"currency":"TRY","total":2500.5,"commission":375,"paid":2500.5,"payment-type":"Online"}}},
				{"change-id":"ch-2","change-type":"Cancelled","pnr":"OB-2","version":3,"change-date":"2026-10-01T14:00:00",
				"booking":{"product-code":"ECO","pickup":{"location-code":"IST","date":"2026-10-11T10:00:00"},
				"return":{"date":"2026-10-12T10:00:00"},"payment":{"currency":"TRY","total":1000,"payment-type":"PayAtOffice"}}}],
				"next-cursor":"c1","has-more":true}`)
		case "/api/carrental/supplier/bookings/acknowledge":
			acked = r.Data
			ok(w, `null`)
		case "/api/carrental/supplier/statement":
			if r.Data["start-date"] != "2026-10-01" || r.Data["end-date"] != "2026-11-01" {
				t.Errorf("statement range = %v", r.Data)
			}
			ok(w, `{"lines":[{"pnr":"OB-1","status":"CONFIRMED","pickup-date":"2026-10-10T10:00:00","currency":"TRY",
				"total":2500.5,"commission":375,"net":2125.5,"payment-type":"Online"},
				{"pnr":"OB-3","status":"NoShow","pickup-date":"2026-10-11T10:00:00","currency":"TRY","total":100,"payment-type":"PayAtOffice"}]}`)
		default:
			http.NotFound(w, nil)
		}
	})
	defer srv.Close()
	a := obilet.New(srv.Client())
	acc := account(srv.URL)
	ctx := context.Background()

	r, err := a.PushInventory(ctx, acc, channels.InventoryPush{
		ValidUntil: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), Offline: channels.OfflinePolicy{Mode: "quota", Quota: 1},
		Items: []channels.InventoryItem{
			{ProductCode: "ECO", LocationCode: "IST", Date: "2026-10-02", Available: 3, HasPrice: true, DailyPrice: money.MustParse("1000"), Currency: "TRY", OfferRef: "OF1"},
			{ProductCode: "ECO", LocationCode: "XXX", Date: "2026-10-02", StopSale: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Reference != "B-1" || r.Accepted != 1 || r.Rejected != 1 || len(r.Errors) != 1 || r.Errors[0].Code != "unknown_location" {
		t.Fatalf("receipt = %+v", r)
	}
	items := pushed["items"].([]any)
	first, second := items[0].(map[string]any), items[1].(map[string]any)
	if first["price"] != float64(1000) || first["currency"] != "TRY" || first["offer-ref"] != "OF1" || second["price"] != nil || second["stop-sale"] != true {
		t.Fatalf("pushed items = %+v", items)
	}
	// valid-until goes out in Türkiye time without an offset.
	if pushed["valid-until"] != "2026-10-01T12:00:00" || pushed["offline"].(map[string]any)["mode"] != "quota" {
		t.Fatalf("pushed = %+v", pushed)
	}

	batch, err := a.PullMessages(ctx, acc, "c0")
	if err != nil {
		t.Fatal(err)
	}
	if batch.Cursor != "c1" || !batch.More || len(batch.Messages) != 2 {
		t.Fatalf("batch = %+v", batch)
	}
	m := batch.Messages[0]
	if m.Type != channels.MessageModification || m.Status != channels.StatusConfirmed || m.ExternalRef != "OB-1" || m.Version != 2 ||
		m.ReturnLocationCode != "SAW" || !m.Total.Equal(money.MustParse("2500.50")) || !m.Commission.Equal(money.MustParse("375")) ||
		!m.Prepaid.Equal(money.MustParse("2500.50")) || m.CollectedBy != channels.CollectedByChannel ||
		len(m.Extras) != 1 || m.Customer.FirstName != "Ali" || m.Customer.LastName != "Can" || m.OfferRef != "OF1" {
		t.Fatalf("message = %+v", m)
	}
	if !m.StartsAt.Equal(time.Date(2026, 10, 10, 7, 0, 0, 0, time.UTC)) || !m.OccurredAt.Equal(time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("times = %v / %v", m.StartsAt, m.OccurredAt)
	}
	c := batch.Messages[1]
	if c.Type != channels.MessageCancellation || c.Status != channels.StatusCancelled || c.ReturnLocationCode != "IST" ||
		c.CollectedBy != channels.CollectedBySupplier || !c.Commission.IsZero() {
		t.Fatalf("cancellation = %+v", c)
	}

	if err := a.Acknowledge(ctx, acc, []channels.Ack{
		{MessageID: "ch-1", ExternalRef: "OB-1", Accepted: true, Reference: "R-1"},
		{MessageID: "ch-2", ExternalRef: "OB-2", Code: "unknown_location", Message: "no mapping"},
	}); err != nil {
		t.Fatal(err)
	}
	acks := acked["items"].([]any)
	a0, a1 := acks[0].(map[string]any), acks[1].(map[string]any)
	if a0["change-id"] != "ch-1" || a0["supplier-reference"] != "R-1" || a0["accepted"] != true ||
		a1["accepted"] != false || a1["error-code"] != "unknown_location" {
		t.Fatalf("acks = %+v", acks)
	}

	loc, _ := time.LoadLocation("Europe/Istanbul")
	lines, err := a.Statement(ctx, acc, time.Date(2026, 10, 1, 0, 0, 0, 0, loc), time.Date(2026, 11, 1, 0, 0, 0, 0, loc))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0].Status != "confirmed" || lines[0].CollectedBy != channels.CollectedByChannel ||
		!lines[0].Net.Equal(money.MustParse("2125.50")) || lines[1].Status != "noshow" || lines[1].CollectedBy != channels.CollectedBySupplier {
		t.Fatalf("statement = %+v", lines)
	}
	// One session served every call.
	if sessions != 1 {
		t.Fatalf("sessions = %d", sessions)
	}
}

// An expired session is renewed once and the call repeated.
func TestSessionRenewal(t *testing.T) {
	var sessions, pings int32
	srv := server(t, &sessions, func(w http.ResponseWriter, path string, r req) {
		if path != "/api/carrental/supplier/ping" {
			http.NotFound(w, nil)
			return
		}
		atomic.AddInt32(&pings, 1)
		if r.DeviceSession.SessionID == "S1" {
			_, _ = w.Write([]byte(`{"status":"InvalidSession","data":null,"message":"session expired"}`))
			return
		}
		ok(w, `{"partner-code":"P1"}`)
	})
	defer srv.Close()
	a := obilet.New(srv.Client())
	acc := account(srv.URL)
	acc.UUID = uuid.New()
	if err := a.Check(context.Background(), acc); err != nil {
		t.Fatal(err)
	}
	if sessions != 2 || pings != 2 {
		t.Fatalf("sessions = %d, pings = %d", sessions, pings)
	}
}

func TestErrorClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/client/getsession":
			ok(w, `{"session-id":"S1","device-id":"D1"}`)
		case strings.HasSuffix(r.URL.Path, "/ping"):
			_, _ = w.Write([]byte(`{"status":"Fail","data":null,"message":"unknown partner"}`))
		case strings.HasSuffix(r.URL.Path, "/statement"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":"Fail","message":"bad range"}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	a := obilet.New(srv.Client())
	ctx := context.Background()
	if err := a.Check(ctx, account(srv.URL)); !errors.Is(err, channels.ErrRejected) || !strings.Contains(err.Error(), "unknown partner") {
		t.Fatalf("envelope fail = %v", err)
	}
	if _, err := a.Statement(ctx, account(srv.URL), time.Now(), time.Now()); !errors.Is(err, channels.ErrRejected) {
		t.Fatalf("400 = %v", err)
	}
	_, err := a.PullMessages(ctx, account(srv.URL), "")
	if err == nil || errors.Is(err, channels.ErrRejected) || errors.Is(err, channels.ErrUnavailable) {
		t.Fatalf("502 must be an unknown outcome: %v", err)
	}
	// Missing credentials never leave.
	noPartner := account(srv.URL)
	noPartner.Credentials = integrations.Credentials{"api_token": "tok-1", "base_url": srv.URL}
	if err := a.Check(ctx, noPartner); !errors.Is(err, channels.ErrRejected) {
		t.Fatalf("no partner = %v", err)
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

func TestParseTime(t *testing.T) {
	if got := obilet.ParseTime("2026-07-01T09:30:00"); !got.Equal(time.Date(2026, 7, 1, 6, 30, 0, 0, time.UTC)) {
		t.Fatalf("local = %v", got)
	}
	if got := obilet.ParseTime("2026-07-01T09:30:00Z"); !got.Equal(time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("rfc3339 = %v", got)
	}
	if !obilet.ParseTime("").IsZero() || !obilet.ParseTime("nope").IsZero() {
		t.Fatal("empty / unreadable must be zero")
	}
}
