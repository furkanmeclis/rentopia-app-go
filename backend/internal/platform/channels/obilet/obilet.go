// Package obilet is the Obilet channel adapter (TEC-737). Obilet sells
// rental cars next to bus and flight tickets; Rentopia publishes the
// organization's products there and imports the bookings, behind the same
// channels.Provider as Yolcu360 (docs/design/channel-manager.md §1).
//
// What is known: Obilet's documented client API (v2) is JSON over HTTPS
// with `Authorization: Basic <api client token>`, a device session obtained
// from POST /api/client/getsession, kebab-case keys, every request wrapped as
//
//	{"device-session": {"session-id", "device-id"}, "date", "language", "data"}
//
// and every answer as {"status": "Success" | ..., "data", "message",
// "user-message"}; dates are Türkiye local time without an offset
// ("2006-01-02T15:04:05") and amounts plain JSON numbers.
//
// What is assumed (the supplier side of the car rental product has not been
// handed to us yet; Obilet is not reachable from the build environment):
// the operation paths below, their field names, the change-feed shape
// (change id + PNR + version, full booking snapshot), the payment-type
// values and the page size. Every assumption lives in this file (the
// constants and the wire types), so confirming the contract means editing
// only this package:
//
//   - POST /api/client/getsession                        session (Check)
//   - POST /api/carrental/supplier/ping                  partner check (Check)
//   - POST /api/carrental/supplier/inventory/update      full snapshot
//   - POST /api/carrental/supplier/bookings/getchanges   cursor, limit
//   - POST /api/carrental/supplier/bookings/acknowledge
//   - POST /api/carrental/supplier/statement             start-date, end-date
//
// Error classification follows the core: connection refused / DNS →
// channels.ErrUnavailable; HTTP 4xx or an envelope status other than Success
// → channels.ErrRejected; timeouts, 408 / 429 and 5xx → unknown. An expired
// session (InvalidSession) is renewed once and the call repeated (every
// operation is a snapshot, a read or idempotent, so repeating is safe).
// Until the contract is confirmed, obilet_fake (package channelfake) is the
// tested path.
package obilet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// ProviderCode is the provider code of the adapter.
const ProviderCode = "obilet"

// Channel is the sales channel code of Obilet bookings.
const Channel = "obilet"

// Default hosts of the documented client API (base_url overrides them).
const (
	SandboxBaseURL = "https://v2-api-test.obilet.com"
	LiveBaseURL    = "https://v2-api.obilet.com"
)

// Operation paths (assumed supplier contract, see the package comment).
const (
	pathSession     = "/api/client/getsession"
	pathPing        = "/api/carrental/supplier/ping"
	pathInventory   = "/api/carrental/supplier/inventory/update"
	pathChanges     = "/api/carrental/supplier/bookings/getchanges"
	pathAcknowledge = "/api/carrental/supplier/bookings/acknowledge"
	pathStatement   = "/api/carrental/supplier/statement"
)

// Envelope statuses.
const (
	statusSuccess        = "success"
	statusInvalidSession = "invalidsession"
)

// maxItems is the assumed page size of an inventory update.
const maxItems = 200

// pageSize is the number of changes asked per pull.
const pageSize = 100

// localLayout is Obilet's date-time format (Türkiye time, no offset).
const localLayout = "2006-01-02T15:04:05"

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*60*60)
	}
	return loc
}()

// session is a device session of one account.
type session struct {
	SessionID string `json:"session-id"`
	DeviceID  string `json:"device-id"`
}

// Adapter is the Obilet supplier API client.
type Adapter struct {
	client *http.Client
	now    func() time.Time

	mu       sync.Mutex
	sessions map[string]session
}

// New builds the adapter (client nil = a client with a 20 s timeout).
func New(client *http.Client) *Adapter {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Adapter{client: client, now: time.Now, sessions: map[string]session{}}
}

// Kind implements integrations.Adapter.
func (a *Adapter) Kind() string { return channels.Kind }

// Provider implements integrations.Adapter.
func (a *Adapter) Provider() string { return ProviderCode }

// Channel implements channels.Provider.
func (a *Adapter) Channel() string { return Channel }

// Fields implements integrations.Adapter.
func (a *Adapter) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "partner_code", LabelKey: "payments.integrations.fields.partner_code", Secret: false, Required: true},
		{Key: "api_token", LabelKey: "payments.integrations.fields.api_token", Secret: true, Required: true},
		{Key: "base_url", LabelKey: "payments.integrations.fields.base_url", Secret: false, Required: false},
	}
}

// Capabilities implements channels.Provider.
func (a *Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{
		Availability: true, Rates: true, StopSale: true, Allotment: true, Pull: true, Acknowledge: true, Statement: true,
		MaxItemsPerPush: maxItems,
	}
}

func baseURL(account integrations.Account) (string, error) {
	raw := strings.TrimSpace(account.Credentials["base_url"])
	if raw == "" {
		if account.Mode == integrations.ModeLive {
			return LiveBaseURL, nil
		}
		return SandboxBaseURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		// Plain http is accepted only for a loopback test server.
		if err == nil && u.Scheme == "http" && isLoopback(u.Hostname()) {
			return strings.TrimRight(raw, "/"), nil
		}
		return "", fmt.Errorf("%w: base_url must be an https URL", channels.ErrRejected)
	}
	return strings.TrimRight(raw, "/"), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sessionKey identifies the session of an account and its credentials (a
// changed token or host starts a new session). Only a digest is kept.
func sessionKey(account integrations.Account, base string) string {
	h := sha256.Sum256([]byte(account.UUID.String() + "\n" + base + "\n" + account.Credentials["api_token"]))
	return hex.EncodeToString(h[:])
}

// envelope is the documented answer wrapper.
type envelope struct {
	Status      string          `json:"status"`
	Data        json.RawMessage `json:"data"`
	Message     string          `json:"message"`
	UserMessage string          `json:"user-message"`
}

// request is the documented request wrapper.
type request struct {
	DeviceSession *session `json:"device-session,omitempty"`
	Date          string   `json:"date"`
	Language      string   `json:"language"`
	Data          any      `json:"data"`
}

// errInvalidSession asks the caller to renew the session.
var errInvalidSession = errors.New("obilet: invalid session")

// post sends one request and decodes the envelope's data into out.
func (a *Adapter) post(ctx context.Context, account integrations.Account, base, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+strings.TrimSpace(account.Credentials["api_token"]))
	resp, err := a.client.Do(req)
	if err != nil {
		var opErr *net.OpError
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) || (errors.As(err, &opErr) && opErr.Op == "dial") {
			return fmt.Errorf("%w: %v", channels.ErrUnavailable, err)
		}
		return fmt.Errorf("obilet: %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("obilet: read answer: %w", err)
	}
	var env envelope
	_ = json.Unmarshal(payload, &env)
	msg := strings.TrimSpace(env.Status + " " + firstNonEmpty(env.Message, env.UserMessage))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		switch {
		case resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			return fmt.Errorf("obilet: %s: HTTP %d %s", path, resp.StatusCode, msg)
		case strings.EqualFold(env.Status, statusInvalidSession):
			return errInvalidSession
		default:
			return fmt.Errorf("%w: HTTP %d %s", channels.ErrRejected, resp.StatusCode, msg)
		}
	}
	switch strings.ToLower(strings.TrimSpace(env.Status)) {
	case statusSuccess:
	case statusInvalidSession:
		return errInvalidSession
	case "":
		return fmt.Errorf("obilet: %s: unreadable answer", path)
	default:
		return fmt.Errorf("%w: %s", channels.ErrRejected, msg)
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("obilet: unreadable answer: %w", err)
	}
	return nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (a *Adapter) date() string { return a.now().In(istanbul).Format(localLayout) }

// session returns the account's device session, opening one when needed.
func (a *Adapter) session(ctx context.Context, account integrations.Account, base string, renew bool) (session, error) {
	key := sessionKey(account, base)
	a.mu.Lock()
	s, ok := a.sessions[key]
	if renew {
		delete(a.sessions, key)
		ok = false
	}
	a.mu.Unlock()
	if ok {
		return s, nil
	}
	// Server-to-server: the device is the integration account.
	body := map[string]any{
		"type":        1,
		"connection":  map[string]string{"ip-address": "", "port": ""},
		"application": map[string]string{"version": "rentopia-channel/1", "equipment-id": account.UUID.String()},
	}
	if err := a.post(ctx, account, base, pathSession, body, &s); err != nil {
		if errors.Is(err, errInvalidSession) {
			return session{}, fmt.Errorf("%w: session refused", channels.ErrRejected)
		}
		return session{}, err
	}
	if s.SessionID == "" {
		return session{}, fmt.Errorf("%w: no session id in the answer", channels.ErrRejected)
	}
	a.mu.Lock()
	a.sessions[key] = s
	a.mu.Unlock()
	return s, nil
}

// call sends one operation inside a device session, renewing an expired
// session once.
func (a *Adapter) call(ctx context.Context, account integrations.Account, path string, data map[string]any, out any) error {
	base, err := baseURL(account)
	if err != nil {
		return err
	}
	partner := strings.TrimSpace(account.Credentials["partner_code"])
	if partner == "" {
		return fmt.Errorf("%w: partner_code is required", channels.ErrRejected)
	}
	if strings.TrimSpace(account.Credentials["api_token"]) == "" {
		return fmt.Errorf("%w: api_token is required", channels.ErrRejected)
	}
	data["partner-code"] = partner
	for attempt := 0; ; attempt++ {
		s, err := a.session(ctx, account, base, attempt > 0)
		if err != nil {
			return err
		}
		err = a.post(ctx, account, base, path, request{DeviceSession: &s, Date: a.date(), Language: "tr-TR", Data: data}, out)
		if errors.Is(err, errInvalidSession) {
			if attempt == 0 {
				continue
			}
			return fmt.Errorf("%w: session refused twice", channels.ErrRejected)
		}
		return err
	}
}

// Check implements integrations.Adapter: a session and a partner ping.
func (a *Adapter) Check(ctx context.Context, account integrations.Account) error {
	return a.call(ctx, account, pathPing, map[string]any{}, nil)
}

type wireItem struct {
	ProductCode  string       `json:"product-code"`
	LocationCode string       `json:"location-code"`
	Date         string       `json:"date"`
	Available    int          `json:"available"`
	StopSale     bool         `json:"stop-sale"`
	Price        *json.Number `json:"price"`
	Currency     string       `json:"currency,omitempty"`
	OfferRef     string       `json:"offer-ref"`
}

type wireReceipt struct {
	BatchID  string `json:"batch-id"`
	Accepted int    `json:"accepted"`
	Rejected int    `json:"rejected"`
	Errors   []struct {
		Index   int    `json:"index"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// PushInventory implements channels.Provider.
func (a *Adapter) PushInventory(ctx context.Context, account integrations.Account, push channels.InventoryPush) (channels.PushReceipt, error) {
	items := make([]wireItem, 0, len(push.Items))
	for _, it := range push.Items {
		w := wireItem{
			ProductCode: it.ProductCode, LocationCode: it.LocationCode, Date: it.Date, Available: it.Available,
			StopSale: it.StopSale, OfferRef: it.OfferRef,
		}
		if it.HasPrice {
			n := json.Number(it.DailyPrice.String())
			w.Price = &n
			w.Currency = it.Currency
		}
		items = append(items, w)
	}
	data := map[string]any{
		"valid-until": push.ValidUntil.In(istanbul).Format(localLayout),
		"offline":     map[string]any{"mode": push.Offline.Mode, "quota": push.Offline.Quota},
		"items":       items,
	}
	var out wireReceipt
	if err := a.call(ctx, account, pathInventory, data, &out); err != nil {
		return channels.PushReceipt{}, err
	}
	r := channels.PushReceipt{Reference: out.BatchID, Accepted: out.Accepted, Rejected: out.Rejected, Errors: []channels.ItemError{}}
	for _, e := range out.Errors {
		r.Errors = append(r.Errors, channels.ItemError{Index: e.Index, Code: e.Code, Message: e.Message})
	}
	return r, nil
}

type wireChange struct {
	ChangeID   string      `json:"change-id"`
	ChangeType string      `json:"change-type"`
	PNR        string      `json:"pnr"`
	Version    int64       `json:"version"`
	ChangeDate string      `json:"change-date"`
	Status     string      `json:"status"`
	Booking    wireBooking `json:"booking"`
}

type wireBooking struct {
	ProductCode string    `json:"product-code"`
	OfferRef    string    `json:"offer-ref"`
	Pickup      wirePlace `json:"pickup"`
	Return      wirePlace `json:"return"`
	Passenger   struct {
		Name        string `json:"name"`
		Surname     string `json:"surname"`
		Email       string `json:"email"`
		Phone       string `json:"phone"`
		Nationality string `json:"nationality"`
	} `json:"passenger"`
	Extras []struct {
		Code     string `json:"code"`
		Quantity int    `json:"quantity"`
	} `json:"extras"`
	Payment struct {
		Currency    string      `json:"currency"`
		Total       json.Number `json:"total"`
		Commission  json.Number `json:"commission"`
		Paid        json.Number `json:"paid"`
		PaymentType string      `json:"payment-type"`
	} `json:"payment"`
	FlightNumber string `json:"flight-number"`
	Note         string `json:"note"`
}

type wirePlace struct {
	LocationCode string `json:"location-code"`
	Date         string `json:"date"`
}

type wireChanges struct {
	Changes    []wireChange `json:"changes"`
	NextCursor string       `json:"next-cursor"`
	HasMore    bool         `json:"has-more"`
}

// ParseTime reads an Obilet date: local Türkiye time without an offset, or
// RFC 3339 (zero when empty or unreadable). Exported for tests.
func ParseTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC()
	}
	for _, layout := range []string{localLayout, "2006-01-02T15:04", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, v, istanbul); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func amount(n json.Number) money.Money {
	m, err := money.ParseRound(strings.TrimSpace(n.String()))
	if err != nil {
		return money.Zero()
	}
	return m
}

// collectedBy maps the payment type to who took the money: paid online on
// Obilet → the channel; paid at the office / counter → us.
func collectedBy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "online", "prepaid", "channel", "obilet":
		return channels.CollectedByChannel
	case "payatoffice", "pay-at-office", "office", "counter", "supplier", "us":
		return channels.CollectedBySupplier
	}
	return ""
}

func messageType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "updated", "modified", "modification", "changed":
		return channels.MessageModification
	case "cancelled", "canceled", "cancellation":
		return channels.MessageCancellation
	}
	return channels.MessageBooking
}

func status(v, typ string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "cancelled", "canceled":
		return channels.StatusCancelled
	case "confirmed", "active", "approved":
		return channels.StatusConfirmed
	}
	if typ == channels.MessageCancellation {
		return channels.StatusCancelled
	}
	return channels.StatusConfirmed
}

// statementStatus maps the known statuses and keeps any other one as is
// (lowercase), so the reconciliation shows a mismatch instead of hiding it.
func statementStatus(v string) string {
	switch s := strings.ToLower(strings.TrimSpace(v)); s {
	case "cancelled", "canceled":
		return channels.StatusCancelled
	case "confirmed", "active", "approved":
		return channels.StatusConfirmed
	default:
		return s
	}
}

// PullMessages implements channels.Provider.
func (a *Adapter) PullMessages(ctx context.Context, account integrations.Account, cursor string) (channels.MessageBatch, error) {
	var out wireChanges
	if err := a.call(ctx, account, pathChanges, map[string]any{"cursor": cursor, "limit": pageSize}, &out); err != nil {
		return channels.MessageBatch{}, err
	}
	batch := channels.MessageBatch{Cursor: out.NextCursor, More: out.HasMore, Messages: make([]channels.Message, 0, len(out.Changes))}
	for _, c := range out.Changes {
		b := c.Booking
		typ := messageType(c.ChangeType)
		m := channels.Message{
			ID: c.ChangeID, Type: typ, ExternalRef: c.PNR, Version: c.Version, OccurredAt: ParseTime(c.ChangeDate),
			Status: status(c.Status, typ), ProductCode: b.ProductCode, OfferRef: b.OfferRef,
			PickupLocationCode: b.Pickup.LocationCode, ReturnLocationCode: b.Return.LocationCode,
			StartsAt: ParseTime(b.Pickup.Date), EndsAt: ParseTime(b.Return.Date),
			Customer: channels.Customer{
				FirstName: b.Passenger.Name, LastName: b.Passenger.Surname, Email: b.Passenger.Email,
				Phone: b.Passenger.Phone, Nationality: b.Passenger.Nationality,
			},
			Currency: b.Payment.Currency, Total: amount(b.Payment.Total), Commission: amount(b.Payment.Commission),
			Prepaid: amount(b.Payment.Paid), CollectedBy: collectedBy(b.Payment.PaymentType),
			FlightNumber: b.FlightNumber, Note: b.Note, Extras: []channels.ExtraLine{},
		}
		if m.ReturnLocationCode == "" {
			m.ReturnLocationCode = m.PickupLocationCode
		}
		for _, x := range b.Extras {
			m.Extras = append(m.Extras, channels.ExtraLine{Code: x.Code, Quantity: x.Quantity})
		}
		batch.Messages = append(batch.Messages, m)
	}
	return batch, nil
}

// Acknowledge implements channels.Provider.
func (a *Adapter) Acknowledge(ctx context.Context, account integrations.Account, acks []channels.Ack) error {
	if len(acks) == 0 {
		return nil
	}
	type wireAck struct {
		ChangeID          string `json:"change-id"`
		PNR               string `json:"pnr"`
		Accepted          bool   `json:"accepted"`
		SupplierReference string `json:"supplier-reference,omitempty"`
		ErrorCode         string `json:"error-code,omitempty"`
		ErrorMessage      string `json:"error-message,omitempty"`
	}
	items := make([]wireAck, 0, len(acks))
	for _, x := range acks {
		items = append(items, wireAck{
			ChangeID: x.MessageID, PNR: x.ExternalRef, Accepted: x.Accepted, SupplierReference: x.Reference,
			ErrorCode: x.Code, ErrorMessage: x.Message,
		})
	}
	return a.call(ctx, account, pathAcknowledge, map[string]any{"items": items}, nil)
}

// Statement implements channels.Provider.
func (a *Adapter) Statement(ctx context.Context, account integrations.Account, from, to time.Time) ([]channels.StatementLine, error) {
	data := map[string]any{"start-date": from.In(istanbul).Format("2006-01-02"), "end-date": to.In(istanbul).Format("2006-01-02")}
	var out struct {
		Lines []struct {
			PNR         string      `json:"pnr"`
			Status      string      `json:"status"`
			PickupDate  string      `json:"pickup-date"`
			Currency    string      `json:"currency"`
			Total       json.Number `json:"total"`
			Commission  json.Number `json:"commission"`
			Net         json.Number `json:"net"`
			PaymentType string      `json:"payment-type"`
		} `json:"lines"`
	}
	if err := a.call(ctx, account, pathStatement, data, &out); err != nil {
		return nil, err
	}
	lines := make([]channels.StatementLine, 0, len(out.Lines))
	for _, l := range out.Lines {
		lines = append(lines, channels.StatementLine{
			ExternalRef: l.PNR, Status: statementStatus(l.Status), StartsAt: ParseTime(l.PickupDate), Currency: l.Currency,
			Total: amount(l.Total), Commission: amount(l.Commission), Net: amount(l.Net), CollectedBy: collectedBy(l.PaymentType),
		})
	}
	return lines, nil
}
