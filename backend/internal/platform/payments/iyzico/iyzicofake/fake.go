// Package iyzicofake is an in-process fake of the iyzico endpoints the
// adapter uses (Checkout Form initialize / retrieve, the pre-auth form,
// post-auth and cancel, refund v2, BIN check, reporting payment details)
// plus a minimal hosted payment page. It checks the IYZWSv2 authorization
// of every request, signs its answers like iyzico and builds signed
// webhooks, so tests (and local browser checks, cmd/iyzico-fake) exercise
// the real adapter without network access. Never used in production.
package iyzicofake

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Checkout states.
const (
	StateInit    = "init"
	StatePaid    = "paid"
	StateFailed  = "failed"
	StateFraud   = "fraud_review"
	commissionPc = 275 // 2.75 %
)

// Checkout is one initialized checkout form.
type Checkout struct {
	Token          string
	APIKey         string
	ConversationID string
	BasketID       string
	Price          money.Money
	PaidPrice      money.Money
	Currency       string
	CallbackURL    string
	State          string
	PaymentID      string
	Refunded       money.Money
	// PreAuth: opened as a pre-authorization; Captured is what post-auth
	// took (once), Cancelled a released pre-authorization.
	PreAuth   bool
	Captured  money.Money
	Cancelled bool
	// RefundLog lists the refunds (reporting payment details).
	RefundLog []RefundEntry
}

// RefundEntry is one refund of a payment.
type RefundEntry struct {
	ID     string
	Amount money.Money
}

// Fake is the fake iyzico server.
type Fake struct {
	Server *httptest.Server
	// WebhookURL receives a signed notification when the hosted page is
	// completed (empty = no delivery).
	WebhookURL string

	mu        sync.Mutex
	accounts  map[string]string // api key → secret key
	checkouts map[string]*Checkout
	payments  map[string]*Checkout
	seq       int64

	Retrieves   atomic.Int64
	Initializes atomic.Int64
	Refunds     atomic.Int64
	PostAuths   atomic.Int64
	Cancels     atomic.Int64
	// FailRefunds makes the refund endpoint refuse every refund (a
	// provider-side failure in tests).
	FailRefunds atomic.Bool
	// Reports counts reporting calls; FailReports makes them fail (5xx).
	Reports     atomic.Int64
	FailReports atomic.Bool
}

// RefundOutside records a refund made on the provider without us (e.g.
// after the backup a test database stands for).
func (f *Fake) RefundOutside(paymentID string, amount money.Money) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.payments[paymentID]
	if !ok {
		return fmt.Errorf("unknown payment %s", paymentID)
	}
	f.seq++
	c.Refunded = c.Refunded.Add(amount)
	c.RefundLog = append(c.RefundLog, RefundEntry{ID: "refund-" + strconv.FormatInt(f.seq, 10), Amount: amount})
	return nil
}

// Payment returns a copy of a completed checkout by provider payment id.
func (f *Fake) Payment(paymentID string) (Checkout, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.payments[paymentID]
	if !ok {
		return Checkout{}, false
	}
	return *c, true
}

// New starts a fake (Close it with f.Server.Close()).
func New() *Fake {
	f := &Fake{accounts: map[string]string{}, checkouts: map[string]*Checkout{}, payments: map[string]*Checkout{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

// NewUnstarted builds a fake whose handler can be mounted elsewhere.
func NewUnstarted() *Fake {
	return &Fake{accounts: map[string]string{}, checkouts: map[string]*Checkout{}, payments: map[string]*Checkout{}}
}

// Handler serves the fake endpoints.
func (f *Fake) Handler() http.Handler { return http.HandlerFunc(f.serve) }

// URL is the base URL of the fake.
func (f *Fake) URL() string {
	if f.Server == nil {
		return ""
	}
	return f.Server.URL
}

// AddAccount registers a merchant key pair.
func (f *Fake) AddAccount(apiKey, secretKey string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[apiKey] = secretKey
}

// Checkout returns a copy of a checkout by token.
func (f *Fake) Checkout(token string) (Checkout, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.checkouts[token]
	if !ok {
		return Checkout{}, false
	}
	return *c, true
}

// Tokens lists the checkout tokens.
func (f *Fake) Tokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.checkouts))
	for t := range f.checkouts {
		out = append(out, t)
	}
	return out
}

// Complete finishes the hosted payment of a checkout (state StatePaid,
// StateFailed or StateFraud) and returns the signed webhook iyzico would
// send: body and the X-IYZ-SIGNATURE-V3 header value.
func (f *Fake) Complete(token, state string) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.checkouts[token]
	if !ok {
		return nil, "", fmt.Errorf("unknown token %s", token)
	}
	if c.State == StateInit {
		c.State = state
		if state != StateFailed {
			f.seq++
			c.PaymentID = strconv.FormatInt(20000000+f.seq, 10)
			f.payments[c.PaymentID] = c
		}
	}
	return f.webhook(c)
}

// Webhook rebuilds the notification of a completed checkout (a provider
// re-delivery).
func (f *Fake) Webhook(token string) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.checkouts[token]
	if !ok {
		return nil, "", fmt.Errorf("unknown token %s", token)
	}
	return f.webhook(c)
}

func (f *Fake) webhook(c *Checkout) ([]byte, string, error) {
	status := "SUCCESS"
	if c.State == StateFailed {
		status = "FAILURE"
	}
	h := payments.Webhook{
		EventType: "CHECKOUT_FORM_AUTH", PaymentID: c.PaymentID, Token: c.Token, ConversationID: c.ConversationID, Status: status,
	}
	body, err := json.Marshal(map[string]any{
		"iyziEventType": h.EventType, "iyziEventTime": 1700000000000, "iyziReferenceCode": "ref-" + c.Token,
		"iyziPaymentId": c.PaymentID, "token": c.Token, "paymentConversationId": c.ConversationID,
		"merchantId": 123456, "status": status,
	})
	if err != nil {
		return nil, "", err
	}
	return body, iyzico.WebhookSignature(f.accounts[c.APIKey], h), nil
}

func failure(w http.ResponseWriter, code, msg string) {
	writeJSON(w, map[string]any{"status": "failure", "errorCode": code, "errorMessage": msg, "locale": "tr"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// authorize checks the IYZWSv2 header; returns the api key and secret.
func (f *Fake) authorize(r *http.Request, body []byte) (string, string, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "IYZWSv2 ") {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "IYZWSv2 "))
	if err != nil {
		return "", "", false
	}
	params := map[string]string{}
	for _, part := range strings.Split(string(raw), "&") {
		k, v, _ := strings.Cut(part, ":")
		params[k] = v
	}
	f.mu.Lock()
	secret, ok := f.accounts[params["apiKey"]]
	f.mu.Unlock()
	if !ok || params["randomKey"] != r.Header.Get("x-iyzi-rnd") {
		return "", "", false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(params["randomKey"] + r.URL.Path))
	mac.Write(body)
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(params["signature"])) {
		return "", "", false
	}
	return params["apiKey"], secret, true
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/fake/checkout/") {
		f.servePage(w, r)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	apiKey, secret, ok := f.authorize(r, body)
	if !ok {
		failure(w, "1000", "Geçersiz imza")
		return
	}
	in := map[string]any{}
	_ = json.Unmarshal(body, &in)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			in[k] = v[0]
		}
	}
	s := func(k string) string { v, _ := in[k].(string); return v }
	switch r.URL.Path {
	case "/v2/reporting/payment/details":
		f.Reports.Add(1)
		if f.FailReports.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var c *Checkout
		if id := s("paymentId"); id != "" {
			c = f.payments[id]
		} else {
			for _, x := range f.payments {
				if x.ConversationID == s("paymentConversationId") {
					c = x
				}
			}
		}
		if c == nil || c.APIKey != apiKey {
			failure(w, "5093", "Ödeme bulunamadı")
			return
		}
		refunds := []map[string]any{}
		for _, e := range c.RefundLog {
			refunds = append(refunds, map[string]any{"refundTxId": e.ID, "refundPrice": json.Number(iyzico.FormatPrice(e.Amount))})
		}
		status := "NOT_REFUNDED"
		if c.Refunded.IsPositive() {
			status = "PARTIALLY_REFUNDED"
			if c.Refunded.Cmp(c.PaidPrice) >= 0 {
				status = "TOTALLY_REFUNDED"
			}
		}
		writeJSON(w, map[string]any{"status": "success", "payments": []map[string]any{{
			"paymentId": c.PaymentID, "paymentConversationId": c.ConversationID, "paymentStatus": 1,
			"paymentRefundStatus": status, "price": json.Number(iyzico.FormatPrice(c.Price)),
			"paidPrice": json.Number(iyzico.FormatPrice(c.PaidPrice)), "currency": c.Currency,
			"itemTransactions": []map[string]any{{"paymentTransactionId": c.PaymentID + "-1", "refunds": refunds}},
		}}})
	case "/payment/bin/check":
		writeJSON(w, map[string]any{"status": "success", "binNumber": s("binNumber"), "cardAssociation": "MASTER_CARD"})
	case "/payment/iyzipos/checkoutform/initialize/auth/ecom", "/payment/iyzipos/checkoutform/initialize/preauth/ecom":
		f.Initializes.Add(1)
		preAuth := strings.Contains(r.URL.Path, "/preauth/")
		price, err := money.ParseRound(s("price"))
		if err != nil || !price.IsPositive() || s("callbackUrl") == "" || s("conversationId") == "" {
			failure(w, "11", "Geçersiz istek")
			return
		}
		paid, _ := money.ParseRound(s("paidPrice"))
		f.mu.Lock()
		f.seq++
		token := fmt.Sprintf("fake-token-%d-%s", f.seq, s("conversationId")[:8])
		f.checkouts[token] = &Checkout{
			Token: token, APIKey: apiKey, ConversationID: s("conversationId"), BasketID: s("basketId"), Price: price,
			PaidPrice: paid, Currency: s("currency"), CallbackURL: s("callbackUrl"), State: StateInit, Refunded: money.Zero(),
			PreAuth: preAuth, Captured: money.Zero(),
		}
		f.mu.Unlock()
		writeJSON(w, map[string]any{
			"status": "success", "locale": "tr", "conversationId": s("conversationId"), "token": token,
			"tokenExpireTime": 1800, "paymentPageUrl": f.pageBase(r) + "/fake/checkout/" + token,
			"checkoutFormContent": "<script></script>", "signature": iyzico.Sign(secret, s("conversationId"), token),
		})
	case "/payment/iyzipos/checkoutform/auth/ecom/detail":
		f.Retrieves.Add(1)
		f.mu.Lock()
		c, ok := f.checkouts[s("token")]
		var cp Checkout
		if ok {
			cp = *c
		}
		f.mu.Unlock()
		if !ok || cp.APIKey != apiKey {
			failure(w, "5032", "Token bulunamadı")
			return
		}
		f.writeResult(w, secret, cp)
	case "/v2/payment/refund":
		f.Refunds.Add(1)
		amount, err := money.ParseRound(s("price"))
		f.mu.Lock()
		defer f.mu.Unlock()
		c, ok := f.payments[s("paymentId")]
		if !ok || c.APIKey != apiKey || err != nil || !amount.IsPositive() {
			failure(w, "5093", "Ödeme bulunamadı")
			return
		}
		if f.FailRefunds.Load() {
			failure(w, "5095", "İade işlemi bankadan reddedildi")
			return
		}
		if c.Refunded.Add(amount).Cmp(c.PaidPrice) > 0 {
			failure(w, "5094", "İade tutarı ödeme tutarını aşıyor")
			return
		}
		c.Refunded = c.Refunded.Add(amount)
		f.seq++
		c.RefundLog = append(c.RefundLog, RefundEntry{ID: "refund-" + strconv.FormatInt(f.seq, 10), Amount: amount})
		writeJSON(w, map[string]any{
			"status": "success", "paymentId": c.PaymentID, "price": json.Number(iyzico.FormatPrice(amount)),
			"currency": c.Currency, "hostReference": "refund-" + strconv.FormatInt(f.seq, 10),
		})
	case "/payment/postauth":
		f.PostAuths.Add(1)
		amount, err := money.ParseRound(s("paidPrice"))
		f.mu.Lock()
		defer f.mu.Unlock()
		c, ok := f.payments[s("paymentId")]
		if !ok || c.APIKey != apiKey || !c.PreAuth || err != nil || !amount.IsPositive() {
			failure(w, "5093", "Ödeme bulunamadı")
			return
		}
		if c.Cancelled || c.Captured.IsPositive() {
			failure(w, "5097", "Provizyon kapatılmış")
			return
		}
		if amount.Cmp(c.Price) > 0 {
			failure(w, "5098", "Tutar provizyon tutarını aşıyor")
			return
		}
		c.Captured = amount
		paid := iyzico.FormatPrice(amount)
		price := iyzico.FormatPrice(c.Price)
		writeJSON(w, map[string]any{
			"status": "success", "locale": "tr", "conversationId": s("conversationId"), "paymentId": c.PaymentID,
			"basketId": c.BasketID, "currency": c.Currency, "price": json.Number(price), "paidPrice": json.Number(paid),
			"paymentStatus": "SUCCESS",
			"signature":     iyzico.Sign(secret, c.PaymentID, c.Currency, c.BasketID, s("conversationId"), paid, price),
		})
	case "/payment/cancel":
		f.Cancels.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		c, ok := f.payments[s("paymentId")]
		if !ok || c.APIKey != apiKey {
			failure(w, "5093", "Ödeme bulunamadı")
			return
		}
		if c.Cancelled || c.Captured.IsPositive() {
			failure(w, "5097", "Provizyon kapatılmış")
			return
		}
		c.Cancelled = true
		writeJSON(w, map[string]any{
			"status": "success", "locale": "tr", "conversationId": s("conversationId"), "paymentId": c.PaymentID,
			"price": json.Number(iyzico.FormatPrice(c.Price)), "currency": c.Currency,
		})
	default:
		failure(w, "404", "unknown endpoint")
	}
}

func (f *Fake) writeResult(w http.ResponseWriter, secret string, c Checkout) {
	price := iyzico.FormatPrice(c.Price)
	paid := iyzico.FormatPrice(c.PaidPrice)
	out := map[string]any{
		"status": "success", "locale": "tr", "conversationId": c.ConversationID, "token": c.Token,
		"basketId": c.BasketID, "currency": c.Currency, "price": json.Number(price), "paidPrice": json.Number(paid),
	}
	switch c.State {
	case StateInit:
		writeJSON(w, out)
		return
	case StateFailed:
		out["paymentStatus"] = "FAILURE"
		out["errorMessage"] = "Kart reddedildi"
	default:
		out["paymentStatus"] = "SUCCESS"
		out["paymentId"] = c.PaymentID
		out["fraudStatus"] = 1
		if c.State == StateFraud {
			out["fraudStatus"] = 0
		}
		commission := c.PaidPrice.Ratio(commissionPc, 10000)
		out["iyziCommissionRateAmount"] = json.Number(iyzico.FormatPrice(commission))
		out["iyziCommissionFee"] = json.Number("0.25")
		out["cardAssociation"] = "MASTER_CARD"
		out["cardFamily"] = "Bonus"
		out["lastFourDigits"] = "0008"
		out["installment"] = 1
	}
	status, _ := out["paymentStatus"].(string)
	payID, _ := out["paymentId"].(string)
	out["signature"] = iyzico.Sign(secret, status, payID, c.Currency, c.BasketID, c.ConversationID, paid, price, c.Token)
	writeJSON(w, out)
}

func (f *Fake) pageBase(r *http.Request) string {
	if f.Server != nil {
		return f.Server.URL
	}
	return "http://" + r.Host
}

// servePage is the hosted page: GET shows the amount with "pay" / "fail"
// buttons; POST …/complete finishes the payment, delivers the webhook (when
// WebhookURL is set) and posts the browser back to the callback URL, like
// iyzico does.
func (f *Fake) servePage(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/fake/checkout/")
	token, action, _ := strings.Cut(rest, "/")
	c, ok := f.Checkout(token)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodGet && action == "" {
		_, _ = fmt.Fprintf(w, `<!doctype html><html lang="tr"><head><meta charset="utf-8"><title>iyzico (fake)</title></head>
<body style="font-family:sans-serif;max-width:420px;margin:48px auto">
<h1>iyzico ödeme (test)</h1><p>Tutar: <strong>%s %s</strong></p>
<form method="post" action="/fake/checkout/%s/complete?result=paid"><button id="fake-pay" type="submit">Öde</button></form>
<form method="post" action="/fake/checkout/%s/complete?result=failed"><button id="fake-fail" type="submit">Reddet</button></form>
</body></html>`, html.EscapeString(c.Price.String()), html.EscapeString(c.Currency), html.EscapeString(token), html.EscapeString(token))
		return
	}
	if r.Method != http.MethodPost || action != "complete" {
		http.NotFound(w, r)
		return
	}
	state := StatePaid
	if r.URL.Query().Get("result") == "failed" {
		state = StateFailed
	}
	body, sig, err := f.Complete(token, state)
	if err == nil && f.WebhookURL != "" {
		req, rerr := http.NewRequest(http.MethodPost, f.WebhookURL, bytes.NewReader(body))
		if rerr == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(iyzico.SignatureHeader, sig)
			if resp, derr := http.DefaultClient.Do(req); derr == nil {
				_ = resp.Body.Close()
			}
		}
	}
	_, _ = fmt.Fprintf(w, `<!doctype html><html><body onload="document.forms[0].submit()">
<form method="post" action="%s"><input type="hidden" name="token" value="%s"><noscript><button>Devam</button></noscript></form>
</body></html>`, html.EscapeString(c.CallbackURL), html.EscapeString(token))
}
