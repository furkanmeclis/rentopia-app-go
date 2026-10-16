package wuzapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp"
)

// server is a minimal wuzapi stand-in that checks tokens and records
// request bodies.
type server struct {
	mu     sync.Mutex
	bodies map[string]map[string]any
	logged bool
}

func (s *server) handler(t *testing.T) http.Handler {
	ok := func(w http.ResponseWriter, data any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": data, "success": true})
	}
	fail := func(w http.ResponseWriter, code int, msg string) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": msg, "success": false})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		_ = json.Unmarshal(raw, &body)
		s.mu.Lock()
		s.bodies[r.Method+" "+r.URL.Path] = body
		s.mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/admin/") {
			if r.Header.Get("Authorization") != "admin-secret" {
				fail(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			switch r.Method {
			case http.MethodPost:
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 201, "data": map[string]any{"id": 7, "name": body["name"]}, "success": true})
			case http.MethodDelete:
				if strings.HasSuffix(r.URL.Path, "/404") {
					fail(w, http.StatusNotFound, "user not found")
					return
				}
				ok(w, map[string]any{"Details": "deleted"})
			}
			return
		}
		if r.Header.Get("Token") != "user-token" {
			fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		switch r.URL.Path {
		case "/session/connect":
			fail(w, http.StatusInternalServerError, "already connected")
		case "/session/qr":
			if s.logged {
				fail(w, http.StatusInternalServerError, "already logged in")
				return
			}
			ok(w, map[string]any{"QRCode": "data:image/png;base64,AAAA"})
		case "/session/status":
			ok(w, map[string]any{"connected": true, "loggedIn": s.logged, "jid": "905321234567:3@s.whatsapp.net"})
		case "/session/logout":
			ok(w, map[string]any{"Details": "Logged out"})
		case "/user/check":
			phones, _ := body["Phone"].([]any)
			on := len(phones) == 1 && phones[0] != "905320000000"
			ok(w, map[string]any{"Users": []map[string]any{{"IsInWhatsapp": on, "Query": phones[0]}}})
		case "/chat/send/text", "/chat/send/document":
			if body["Phone"] == "905329999999" {
				fail(w, http.StatusBadRequest, "invalid phone")
				return
			}
			ok(w, map[string]any{"Details": "Sent", "Id": body["Id"], "Timestamp": "2026-10-16T12:00:00+03:00"})
		default:
			http.NotFound(w, r)
		}
	})
}

func TestProvider(t *testing.T) {
	t.Parallel()
	s := &server{bodies: map[string]map[string]any{}}
	ts := httptest.NewServer(s.handler(t))
	defer ts.Close()
	ctx := context.Background()

	if New(Config{}).Configured() {
		t.Fatal("configured without a server")
	}
	if _, err := New(Config{}).Status(ctx, integrations.Account{Credentials: integrations.Credentials{whatsapp.CredentialToken: "t"}}); !errors.Is(err, whatsapp.ErrNotConfigured) {
		t.Fatalf("unconfigured: %v", err)
	}
	p := New(Config{BaseURL: ts.URL + "/", AdminToken: "admin-secret"})
	if p.Kind() != whatsapp.Kind || p.Provider() != ProviderCode || len(p.Fields()) != 0 {
		t.Fatal("adapter identity")
	}
	sess, err := p.CreateSession(ctx, whatsapp.SessionInput{Name: "rentopia-x", Token: "user-token", WebhookURL: "http://api/hook"})
	if err != nil {
		t.Fatal(err)
	}
	if sess.UserID != "7" || sess.Token != "user-token" {
		t.Fatalf("session: %+v", sess)
	}
	created := s.bodies["POST /admin/users"]
	if created["token"] != "user-token" || created["webhook"] != "http://api/hook" || !strings.Contains(created["events"].(string), "ReadReceipt") {
		t.Fatalf("create body: %v", created)
	}

	acc := integrations.Account{Credentials: integrations.Credentials{whatsapp.CredentialToken: "user-token", whatsapp.CredentialUserID: "404"}}
	if err := p.DeleteSession(ctx, acc); err != nil {
		t.Fatalf("delete of a gone user: %v", err)
	}
	if err := p.Connect(ctx, acc); err != nil {
		t.Fatalf("connect (already connected): %v", err)
	}
	qr, err := p.QR(ctx, acc)
	if err != nil || qr != "data:image/png;base64,AAAA" {
		t.Fatalf("qr: %q %v", qr, err)
	}
	st, err := p.Status(ctx, acc)
	if err != nil || !st.Connected || st.LoggedIn || st.Phone() != "+905321234567" {
		t.Fatalf("status: %+v %v", st, err)
	}
	if err := p.Check(ctx, acc); err == nil {
		t.Fatal("check passed while not logged in")
	}
	s.logged = true
	if _, err := p.QR(ctx, acc); !errors.Is(err, whatsapp.ErrAlreadyLoggedIn) {
		t.Fatalf("qr when logged in: %v", err)
	}
	if err := p.Check(ctx, acc); err != nil {
		t.Fatal(err)
	}
	on, err := p.OnWhatsApp(ctx, acc, "+905321234567")
	if err != nil || !on {
		t.Fatalf("check number: %v %v", on, err)
	}
	if on, _ := p.OnWhatsApp(ctx, acc, "+905320000000"); on {
		t.Fatal("number without WhatsApp")
	}
	res, err := p.SendText(ctx, acc, whatsapp.TextMessage{To: "+905321234567", Body: "Merhaba", ID: "3EB0ABC", LinkPreview: true})
	if err != nil || res.MessageID != "3EB0ABC" || res.Timestamp.IsZero() {
		t.Fatalf("send: %+v %v", res, err)
	}
	sent := s.bodies["POST /chat/send/text"]
	if sent["Phone"] != "905321234567" || sent["Body"] != "Merhaba" || sent["LinkPreview"] != true {
		t.Fatalf("send body: %v", sent)
	}
	_, err = p.SendText(ctx, acc, whatsapp.TextMessage{To: "+905329999999", Body: "x", ID: "1"})
	var rej *whatsapp.Rejected
	if !errors.As(err, &rej) || rej.Status != http.StatusBadRequest || rej.Message != "invalid phone" {
		t.Fatalf("rejected: %v", err)
	}
	if _, err := p.SendDocument(ctx, acc, whatsapp.DocumentMessage{
		To: "+905321234567", ID: "D1", FileName: "contract.pdf", MimeType: "application/pdf", Data: []byte("%PDF"), Caption: "Sözleşme",
	}); err != nil {
		t.Fatal(err)
	}
	doc := s.bodies["POST /chat/send/document"]
	if doc["Document"] != "data:application/pdf;base64,JVBERg==" || doc["FileName"] != "contract.pdf" || doc["Caption"] != "Sözleşme" {
		t.Fatalf("document body: %v", doc)
	}
	if err := p.Logout(ctx, acc); err != nil {
		t.Fatal(err)
	}
	bad := integrations.Account{Credentials: integrations.Credentials{whatsapp.CredentialToken: "other"}}
	if _, err := p.Status(ctx, bad); !errors.As(err, &rej) || rej.Status != http.StatusUnauthorized {
		t.Fatalf("bad token: %v", err)
	}
	if _, err := p.Status(ctx, integrations.Account{}); !errors.Is(err, whatsapp.ErrNoSession) {
		t.Fatalf("no token: %v", err)
	}
}
