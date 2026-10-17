// Package yolcu360 is the Yolcu360 channel adapter (TEC-736). Yolcu360's
// public Agency API is the search / booking side; Rentopia needs the
// supplier (fleet publishing) protocol, whose technical contract is being
// obtained through the business relationship (TEC-680). This adapter
// implements that contract as it is documented to us so far and keeps every
// assumption here, behind channels.Provider:
//
//   - REST + JSON over HTTPS; the base URL defaults to the sandbox / live
//     hosts below and can be overridden per account (credential base_url)
//     once onboarding confirms the hosts;
//   - every request carries X-Api-Key, X-Timestamp (unix seconds) and
//     X-Signature = hex(HMAC-SHA256(api_secret, timestamp + "\n" + method +
//     "\n" + path + "\n" + body));
//   - PUT  /v1/suppliers/{supplier_code}/inventory           full snapshot
//   - GET  /v1/suppliers/{supplier_code}/booking-events      ?cursor=&limit=
//   - POST /v1/suppliers/{supplier_code}/booking-events/ack
//   - GET  /v1/suppliers/{supplier_code}/statements          ?from=&to=
//   - GET  /v1/suppliers/{supplier_code}/ping                connection test
//
// Points still to be confirmed in writing (docs/design/channel-manager.md
// §9): connection direction, who generates product codes, model guarantee
// support, the availability format (count / offer / open-closed), where the
// booking becomes final, retry / timeout / cancellation rules and the
// stop-sale / disconnection behaviour. Until then the fake (yolcu360_fake)
// is the tested path.
package yolcu360

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// ProviderCode is the provider code of the adapter.
const ProviderCode = "yolcu360"

// Channel is the sales channel code of Yolcu360 bookings.
const Channel = "yolcu360"

// Default hosts (to be confirmed in onboarding; base_url overrides them).
const (
	SandboxBaseURL = "https://sandbox-supplier-api.yolcu360.com"
	LiveBaseURL    = "https://supplier-api.yolcu360.com"
)

// maxItems is the documented page size of an inventory push.
const maxItems = 500

// Adapter is the Yolcu360 supplier API client.
type Adapter struct {
	client *http.Client
	now    func() time.Time
}

// New builds the adapter (client nil = a client with a 20 s timeout).
func New(client *http.Client) *Adapter {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Adapter{client: client, now: time.Now}
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
		{Key: "supplier_code", LabelKey: "payments.integrations.fields.supplier_code", Secret: false, Required: true},
		{Key: "api_key", LabelKey: "payments.integrations.fields.api_key", Secret: true, Required: true},
		{Key: "api_secret", LabelKey: "payments.integrations.fields.api_secret", Secret: true, Required: true},
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

// Sign computes the request signature (exported for tests).
func Sign(secret, timestamp, method, path string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "\n" + method + "\n" + path + "\n"))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// apiError is the documented error body.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// do sends one request and decodes a 2xx answer into out.
func (a *Adapter) do(ctx context.Context, account integrations.Account, method, path string, query url.Values, in, out any) error {
	base, err := baseURL(account)
	if err != nil {
		return err
	}
	code := strings.TrimSpace(account.Credentials["supplier_code"])
	if code == "" {
		return fmt.Errorf("%w: supplier_code is required", channels.ErrRejected)
	}
	full := "/v1/suppliers/" + url.PathEscape(code) + path
	var body []byte
	if in != nil {
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	target := base + full
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(a.now().Unix(), 10)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Api-Key", account.Credentials["api_key"])
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", Sign(account.Credentials["api_secret"], ts, method, full, body))
	resp, err := a.client.Do(req)
	if err != nil {
		var opErr *net.OpError
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) || (errors.As(err, &opErr) && opErr.Op == "dial") {
			return fmt.Errorf("%w: %v", channels.ErrUnavailable, err)
		}
		return fmt.Errorf("yolcu360: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("yolcu360: read answer: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var ae apiError
		_ = json.Unmarshal(raw, &ae)
		msg := strings.TrimSpace(ae.Code + " " + ae.Message)
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		switch {
		case resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			return fmt.Errorf("yolcu360: %s %s: HTTP %d %s", method, path, resp.StatusCode, msg)
		default:
			return fmt.Errorf("%w: HTTP %d %s", channels.ErrRejected, resp.StatusCode, msg)
		}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("yolcu360: unreadable answer: %w", err)
	}
	return nil
}

// Check implements integrations.Adapter (authenticated ping).
func (a *Adapter) Check(ctx context.Context, account integrations.Account) error {
	return a.do(ctx, account, http.MethodGet, "/ping", nil, nil, nil)
}

type wireItem struct {
	ProductCode  string     `json:"product_code"`
	LocationCode string     `json:"location_code"`
	Date         string     `json:"date"`
	Available    int        `json:"available"`
	StopSale     bool       `json:"stop_sale"`
	Price        *wirePrice `json:"price,omitempty"`
	OfferRef     string     `json:"offer_ref"`
}

type wirePrice struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type wirePush struct {
	ValidUntil time.Time              `json:"valid_until"`
	Offline    channels.OfflinePolicy `json:"offline"`
	Items      []wireItem             `json:"items"`
}

type wireReceipt struct {
	BatchID  string `json:"batch_id"`
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
	body := wirePush{ValidUntil: push.ValidUntil.UTC(), Offline: push.Offline, Items: make([]wireItem, 0, len(push.Items))}
	for _, it := range push.Items {
		w := wireItem{
			ProductCode: it.ProductCode, LocationCode: it.LocationCode, Date: it.Date, Available: it.Available,
			StopSale: it.StopSale, OfferRef: it.OfferRef,
		}
		if it.HasPrice {
			w.Price = &wirePrice{Amount: it.DailyPrice.String(), Currency: it.Currency}
		}
		body.Items = append(body.Items, w)
	}
	var out wireReceipt
	if err := a.do(ctx, account, http.MethodPut, "/inventory", nil, body, &out); err != nil {
		return channels.PushReceipt{}, err
	}
	r := channels.PushReceipt{Reference: out.BatchID, Accepted: out.Accepted, Rejected: out.Rejected, Errors: []channels.ItemError{}}
	for _, e := range out.Errors {
		r.Errors = append(r.Errors, channels.ItemError{Index: e.Index, Code: e.Code, Message: e.Message})
	}
	return r, nil
}

type wireEvent struct {
	EventID    string      `json:"event_id"`
	Type       string      `json:"type"`
	BookingRef string      `json:"booking_ref"`
	Version    int64       `json:"version"`
	OccurredAt time.Time   `json:"occurred_at"`
	Status     string      `json:"status"`
	Booking    wireBooking `json:"booking"`
}

type wireBooking struct {
	ProductCode string    `json:"product_code"`
	OfferRef    string    `json:"offer_ref"`
	Pickup      wirePlace `json:"pickup"`
	Dropoff     wirePlace `json:"dropoff"`
	Customer    struct {
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		Email       string `json:"email"`
		Phone       string `json:"phone"`
		Nationality string `json:"nationality"`
	} `json:"customer"`
	Extras []struct {
		Code     string `json:"code"`
		Quantity int    `json:"quantity"`
	} `json:"extras"`
	Payment struct {
		Currency    string `json:"currency"`
		Total       string `json:"total"`
		Commission  string `json:"commission"`
		Prepaid     string `json:"prepaid"`
		CollectedBy string `json:"collected_by"`
	} `json:"payment"`
	FlightNumber string `json:"flight_number"`
	Note         string `json:"note"`
}

type wirePlace struct {
	LocationCode string    `json:"location_code"`
	At           time.Time `json:"at"`
}

type wireEvents struct {
	Events     []wireEvent `json:"events"`
	NextCursor string      `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

func amount(s string) money.Money {
	m, err := money.ParseRound(strings.TrimSpace(s))
	if err != nil {
		return money.Zero()
	}
	return m
}

// collectedBy maps the wire value (channel | supplier) to ours.
func collectedBy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "supplier", "us":
		return channels.CollectedBySupplier
	case "channel", "yolcu360":
		return channels.CollectedByChannel
	}
	return ""
}

// PullMessages implements channels.Provider.
func (a *Adapter) PullMessages(ctx context.Context, account integrations.Account, cursor string) (channels.MessageBatch, error) {
	q := url.Values{"limit": {"100"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var out wireEvents
	if err := a.do(ctx, account, http.MethodGet, "/booking-events", q, nil, &out); err != nil {
		return channels.MessageBatch{}, err
	}
	batch := channels.MessageBatch{Cursor: out.NextCursor, More: out.HasMore, Messages: make([]channels.Message, 0, len(out.Events))}
	for _, e := range out.Events {
		b := e.Booking
		m := channels.Message{
			ID: e.EventID, Type: messageType(e.Type), ExternalRef: e.BookingRef, Version: e.Version, OccurredAt: e.OccurredAt,
			Status: strings.ToLower(e.Status), ProductCode: b.ProductCode, OfferRef: b.OfferRef,
			PickupLocationCode: b.Pickup.LocationCode, ReturnLocationCode: b.Dropoff.LocationCode,
			StartsAt: b.Pickup.At, EndsAt: b.Dropoff.At,
			Customer: channels.Customer{
				FirstName: b.Customer.FirstName, LastName: b.Customer.LastName, Email: b.Customer.Email,
				Phone: b.Customer.Phone, Nationality: b.Customer.Nationality,
			},
			Currency: b.Payment.Currency, Total: amount(b.Payment.Total), Commission: amount(b.Payment.Commission),
			Prepaid: amount(b.Payment.Prepaid), CollectedBy: collectedBy(b.Payment.CollectedBy),
			FlightNumber: b.FlightNumber, Note: b.Note, Extras: []channels.ExtraLine{},
		}
		if m.Status == "" {
			m.Status = channels.StatusConfirmed
			if m.Type == channels.MessageCancellation {
				m.Status = channels.StatusCancelled
			}
		}
		for _, x := range b.Extras {
			m.Extras = append(m.Extras, channels.ExtraLine{Code: x.Code, Quantity: x.Quantity})
		}
		batch.Messages = append(batch.Messages, m)
	}
	return batch, nil
}

func messageType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "booking.modified", "modification", "modified":
		return channels.MessageModification
	case "booking.cancelled", "cancellation", "cancelled":
		return channels.MessageCancellation
	}
	return channels.MessageBooking
}

// Acknowledge implements channels.Provider.
func (a *Adapter) Acknowledge(ctx context.Context, account integrations.Account, acks []channels.Ack) error {
	if len(acks) == 0 {
		return nil
	}
	type wireAck struct {
		EventID      string `json:"event_id"`
		BookingRef   string `json:"booking_ref"`
		Accepted     bool   `json:"accepted"`
		SupplierRef  string `json:"supplier_ref,omitempty"`
		ErrorCode    string `json:"error_code,omitempty"`
		ErrorMessage string `json:"error_message,omitempty"`
	}
	body := struct {
		Acks []wireAck `json:"acks"`
	}{Acks: make([]wireAck, 0, len(acks))}
	for _, x := range acks {
		body.Acks = append(body.Acks, wireAck{
			EventID: x.MessageID, BookingRef: x.ExternalRef, Accepted: x.Accepted, SupplierRef: x.Reference,
			ErrorCode: x.Code, ErrorMessage: x.Message,
		})
	}
	return a.do(ctx, account, http.MethodPost, "/booking-events/ack", nil, body, nil)
}

// Statement implements channels.Provider.
func (a *Adapter) Statement(ctx context.Context, account integrations.Account, from, to time.Time) ([]channels.StatementLine, error) {
	q := url.Values{"from": {from.Format("2006-01-02")}, "to": {to.Format("2006-01-02")}}
	var out struct {
		Lines []struct {
			BookingRef  string    `json:"booking_ref"`
			Status      string    `json:"status"`
			PickupAt    time.Time `json:"pickup_at"`
			Currency    string    `json:"currency"`
			Total       string    `json:"total"`
			Commission  string    `json:"commission"`
			Net         string    `json:"net"`
			CollectedBy string    `json:"collected_by"`
		} `json:"lines"`
	}
	if err := a.do(ctx, account, http.MethodGet, "/statements", q, nil, &out); err != nil {
		return nil, err
	}
	lines := make([]channels.StatementLine, 0, len(out.Lines))
	for _, l := range out.Lines {
		lines = append(lines, channels.StatementLine{
			ExternalRef: l.BookingRef, Status: strings.ToLower(l.Status), StartsAt: l.PickupAt, Currency: l.Currency,
			Total: amount(l.Total), Commission: amount(l.Commission), Net: amount(l.Net), CollectedBy: collectedBy(l.CollectedBy),
		})
	}
	return lines, nil
}
