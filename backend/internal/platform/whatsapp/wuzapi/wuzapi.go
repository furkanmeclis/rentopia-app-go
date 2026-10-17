// Package wuzapi is the WhatsApp provider on a self-hosted wuzapi server
// (github.com/asternic/wuzapi, a REST gateway on whatsmeow). The server runs
// on the private app network (compose.prod.yml); every organization gets
// its own wuzapi user, i.e. its own WhatsApp Web session.
//
// REST API used (wuzapi API.md):
//
//	POST   /admin/users          Authorization: <admin token>   create a user
//	DELETE /admin/users/{id}     Authorization: <admin token>   remove a user
//	POST   /session/connect      Token: <user token>            open the socket
//	GET    /session/qr           Token                          pairing QR (data URL)
//	GET    /session/status       Token                          Connected / LoggedIn
//	POST   /session/logout       Token                          unlink the phone
//	POST   /user/check           Token                          numbers on WhatsApp
//	POST   /chat/send/text       Token                          text (our message Id)
//	POST   /chat/send/document   Token                          document (base64 data URL)
//
// Answers are {"code": 200, "data": {...}, "success": true} or
// {"code": 4xx/5xx, "error": "...", "success": false}. Webhooks post the
// subscribed events to the user's webhook URL (see
// whatsapp.ParseWuzapiWebhook).
package wuzapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp"
)

// ProviderCode is the integration provider code.
const ProviderCode = "wuzapi"

// Events are the webhook events a session subscribes to: receipts and
// the connection state (incoming chats are not needed and not received).
var Events = []string{"ReadReceipt", "Connected", "PairSuccess", "LoggedOut", "Disconnected"}

// Config locates the wuzapi server.
type Config struct {
	// BaseURL of the server on the private network ("http://wuzapi.<net>:8080").
	BaseURL string
	// AdminToken is WUZAPI_ADMIN_TOKEN of the server.
	AdminToken string
	// HTTPClient overrides the default client (tests).
	HTTPClient *http.Client
}

// Provider talks to one wuzapi server.
type Provider struct {
	base  string
	admin string
	http  *http.Client
}

// New builds the provider.
func New(cfg Config) *Provider {
	c := cfg.HTTPClient
	if c == nil {
		c = &http.Client{Timeout: 20 * time.Second}
	}
	return &Provider{base: strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"), admin: strings.TrimSpace(cfg.AdminToken), http: c}
}

// Kind implements integrations.Adapter.
func (*Provider) Kind() string { return whatsapp.Kind }

// Provider implements integrations.Adapter.
func (*Provider) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter: nothing is typed in, the session
// token is generated when WhatsApp is connected.
func (*Provider) Fields() []integrations.Field { return []integrations.Field{} }

// Configured implements whatsapp.Provider.
func (p *Provider) Configured() bool { return p.base != "" && p.admin != "" }

// Check implements integrations.Adapter: the session is linked.
func (p *Provider) Check(ctx context.Context, account integrations.Account) error {
	st, err := p.Status(ctx, account)
	if err != nil {
		return err
	}
	if !st.LoggedIn {
		return errors.New("WhatsApp is not linked: scan the QR code on the WhatsApp page")
	}
	return nil
}

type envelope struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Success bool            `json:"success"`
}

// call sends one request; a 4xx answer is a *whatsapp.Rejected.
func (p *Provider) call(ctx context.Context, method, path string, header map[string]string, body any) (json.RawMessage, error) {
	if !p.Configured() {
		return nil, whatsapp.ErrNotConfigured
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wuzapi: %s %s: %w", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("wuzapi: read %s: %w", path, err)
	}
	var env envelope
	_ = json.Unmarshal(raw, &env)
	if res.StatusCode >= 200 && res.StatusCode < 300 && (env.Success || env.Error == "") {
		return env.Data, nil
	}
	msg := strings.TrimSpace(env.Error)
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
		if len(msg) > 200 {
			msg = msg[:200]
		}
	}
	if msg == "" {
		msg = http.StatusText(res.StatusCode)
	}
	if res.StatusCode >= 400 && res.StatusCode < 500 {
		return nil, &whatsapp.Rejected{Status: res.StatusCode, Message: msg}
	}
	return nil, fmt.Errorf("wuzapi: %s %s: %d %s", method, path, res.StatusCode, msg)
}

func (p *Provider) adminHeader() map[string]string {
	return map[string]string{"Authorization": p.admin}
}

func userHeader(a integrations.Account) (map[string]string, error) {
	t := whatsapp.Token(a)
	if t == "" {
		return nil, whatsapp.ErrNoSession
	}
	return map[string]string{"Token": t}, nil
}

// CreateSession implements whatsapp.Provider (POST /admin/users).
func (p *Provider) CreateSession(ctx context.Context, in whatsapp.SessionInput) (whatsapp.Session, error) {
	data, err := p.call(ctx, http.MethodPost, "/admin/users", p.adminHeader(), map[string]any{
		"name": in.Name, "token": in.Token, "webhook": in.WebhookURL, "events": strings.Join(Events, ","),
	})
	if err != nil {
		return whatsapp.Session{}, err
	}
	var out struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(data, &out)
	return whatsapp.Session{UserID: rawID(out.ID), Token: in.Token}, nil
}

// rawID turns a JSON string or number id into text.
func rawID(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return ""
}

// DeleteSession implements whatsapp.Provider (DELETE /admin/users/{id}).
func (p *Provider) DeleteSession(ctx context.Context, account integrations.Account) error {
	id := strings.TrimSpace(account.Credentials[whatsapp.CredentialUserID])
	if id == "" {
		return nil
	}
	_, err := p.call(ctx, http.MethodDelete, "/admin/users/"+id, p.adminHeader(), nil)
	var rej *whatsapp.Rejected
	if errors.As(err, &rej) && rej.Status == http.StatusNotFound {
		return nil
	}
	return err
}

// Connect implements whatsapp.Provider (POST /session/connect). "Already
// connected" is not an error.
func (p *Provider) Connect(ctx context.Context, account integrations.Account) error {
	h, err := userHeader(account)
	if err != nil {
		return err
	}
	_, err = p.call(ctx, http.MethodPost, "/session/connect", h, map[string]any{"Subscribe": Events, "Immediate": true})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already connected") {
		return nil
	}
	return err
}

// QR implements whatsapp.Provider (GET /session/qr).
func (p *Provider) QR(ctx context.Context, account integrations.Account) (string, error) {
	h, err := userHeader(account)
	if err != nil {
		return "", err
	}
	data, err := p.call(ctx, http.MethodGet, "/session/qr", h, nil)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already logged in") {
			return "", whatsapp.ErrAlreadyLoggedIn
		}
		return "", err
	}
	var out struct {
		QRCode string `json:"QRCode"`
	}
	_ = json.Unmarshal(data, &out)
	return out.QRCode, nil
}

// Status implements whatsapp.Provider (GET /session/status). Versions
// differ in key case ("Connected" / "connected", "LoggedIn" / "loggedIn").
func (p *Provider) Status(ctx context.Context, account integrations.Account) (whatsapp.Status, error) {
	h, err := userHeader(account)
	if err != nil {
		return whatsapp.Status{}, err
	}
	data, err := p.call(ctx, http.MethodGet, "/session/status", h, nil)
	if err != nil {
		return whatsapp.Status{}, err
	}
	m := map[string]any{}
	_ = json.Unmarshal(data, &m)
	flag := func(keys ...string) bool {
		for _, k := range keys {
			if v, ok := m[k].(bool); ok {
				return v
			}
		}
		return false
	}
	text := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	return whatsapp.Status{
		Connected: flag("Connected", "connected"),
		LoggedIn:  flag("LoggedIn", "loggedIn", "logged_in"),
		JID:       text("jid", "JID", "Jid"),
	}, nil
}

// Logout implements whatsapp.Provider (POST /session/logout).
func (p *Provider) Logout(ctx context.Context, account integrations.Account) error {
	h, err := userHeader(account)
	if err != nil {
		return err
	}
	_, err = p.call(ctx, http.MethodPost, "/session/logout", h, nil)
	return err
}

// OnWhatsApp implements whatsapp.Provider (POST /user/check).
func (p *Provider) OnWhatsApp(ctx context.Context, account integrations.Account, phone string) (bool, error) {
	h, err := userHeader(account)
	if err != nil {
		return false, err
	}
	data, err := p.call(ctx, http.MethodPost, "/user/check", h, map[string]any{"Phone": []string{whatsapp.Digits(phone)}})
	if err != nil {
		return false, err
	}
	var out struct {
		Users []struct {
			IsInWhatsapp bool `json:"IsInWhatsapp"`
		} `json:"Users"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return false, fmt.Errorf("wuzapi: user check: %w", err)
	}
	return len(out.Users) > 0 && out.Users[0].IsInWhatsapp, nil
}

func sendResult(data json.RawMessage, fallbackID string) whatsapp.SendResult {
	var out struct {
		ID        string `json:"Id"`
		Timestamp any    `json:"Timestamp"`
	}
	_ = json.Unmarshal(data, &out)
	r := whatsapp.SendResult{MessageID: out.ID}
	if r.MessageID == "" {
		r.MessageID = fallbackID
	}
	switch v := out.Timestamp.(type) {
	case string:
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			r.Timestamp = t.UTC()
		} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			r.Timestamp = time.Unix(n, 0).UTC()
		}
	case float64:
		r.Timestamp = time.Unix(int64(v), 0).UTC()
	}
	return r
}

// SendText implements whatsapp.Provider (POST /chat/send/text).
func (p *Provider) SendText(ctx context.Context, account integrations.Account, msg whatsapp.TextMessage) (whatsapp.SendResult, error) {
	h, err := userHeader(account)
	if err != nil {
		return whatsapp.SendResult{}, err
	}
	body := map[string]any{"Phone": whatsapp.Digits(msg.To), "Body": msg.Body, "Id": msg.ID}
	if msg.LinkPreview {
		body["LinkPreview"] = true
	}
	data, err := p.call(ctx, http.MethodPost, "/chat/send/text", h, body)
	if err != nil {
		return whatsapp.SendResult{}, err
	}
	return sendResult(data, msg.ID), nil
}

// SendDocument implements whatsapp.Provider (POST /chat/send/document).
func (p *Provider) SendDocument(ctx context.Context, account integrations.Account, msg whatsapp.DocumentMessage) (whatsapp.SendResult, error) {
	h, err := userHeader(account)
	if err != nil {
		return whatsapp.SendResult{}, err
	}
	mt := msg.MimeType
	if mt == "" {
		mt = "application/octet-stream"
	}
	body := map[string]any{
		"Phone": whatsapp.Digits(msg.To), "Id": msg.ID, "FileName": msg.FileName,
		"Document": "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(msg.Data),
	}
	if msg.Caption != "" {
		body["Caption"] = msg.Caption
	}
	data, err := p.call(ctx, http.MethodPost, "/chat/send/document", h, body)
	if err != nil {
		return whatsapp.SendResult{}, err
	}
	return sendResult(data, msg.ID), nil
}

// ParseWebhook implements whatsapp.Provider.
func (*Provider) ParseWebhook(contentType string, body []byte) ([]whatsapp.Event, error) {
	return whatsapp.ParseWuzapiWebhook(contentType, body)
}

var _ whatsapp.Provider = (*Provider)(nil)
