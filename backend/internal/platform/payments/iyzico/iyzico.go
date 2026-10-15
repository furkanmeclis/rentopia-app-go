// Package iyzico is the iyzico payment adapter (TEC-707): Checkout Form
// (hosted, 3DS) initialize + retrieve, refund (v2, by payment id), a BIN
// check as the connection test, and the X-IYZ-SIGNATURE-V3 webhook
// signature. Requests are authenticated with iyzico's HMAC scheme IYZWSv2:
//
//	signature = hex(HMAC-SHA256(secretKey, randomKey + uriPath + body))
//	Authorization: IYZWSv2 base64("apiKey:" + apiKey + "&randomKey:" + randomKey + "&signature:" + signature)
//	x-iyzi-rnd: randomKey
//
// Signed answers (initialize: conversationId:token; retrieve:
// paymentStatus:paymentId:currency:basketId:conversationId:paidPrice:price:token)
// are verified with the account's secret key when iyzico sends a
// signature. The formulas follow iyzico's official SDKs (iyzipay-node /
// iyzipay-php); docs/design/payments.md §3.3 lists what must be confirmed
// against the real sandbox (TEC-684).
package iyzico

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Provider code and default base URLs.
const (
	ProviderCode   = "iyzico"
	DefaultSandbox = "https://sandbox-api.iyzipay.com"
	DefaultLive    = "https://api.iyzipay.com"

	pathInitialize = "/payment/iyzipos/checkoutform/initialize/auth/ecom"
	pathRetrieve   = "/payment/iyzipos/checkoutform/auth/ecom/detail"
	pathRefund     = "/v2/payment/refund"
	pathBinCheck   = "/payment/bin/check"

	// SignatureHeader carries the webhook signature.
	SignatureHeader = "X-IYZ-SIGNATURE-V3"
)

// Credential field keys.
const (
	FieldAPIKey    = "api_key"
	FieldSecretKey = "secret_key"
)

// Config holds the base URLs (sandbox / live) and the HTTP client.
type Config struct {
	SandboxBaseURL string
	LiveBaseURL    string
	HTTPClient     *http.Client
	// Now is the clock of the random key (tests).
	Now func() time.Time
}

// Adapter implements payments.Provider for iyzico.
type Adapter struct {
	cfg Config
}

// New builds the adapter (empty base URLs take iyzico's defaults).
func New(cfg Config) *Adapter {
	if strings.TrimSpace(cfg.SandboxBaseURL) == "" {
		cfg.SandboxBaseURL = DefaultSandbox
	}
	if strings.TrimSpace(cfg.LiveBaseURL) == "" {
		cfg.LiveBaseURL = DefaultLive
	}
	cfg.SandboxBaseURL = strings.TrimRight(cfg.SandboxBaseURL, "/")
	cfg.LiveBaseURL = strings.TrimRight(cfg.LiveBaseURL, "/")
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Adapter{cfg: cfg}
}

var _ payments.Provider = (*Adapter)(nil)

// Kind implements integrations.Adapter.
func (a *Adapter) Kind() string { return integrations.KindPayment }

// Provider implements integrations.Adapter.
func (a *Adapter) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (a *Adapter) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: FieldAPIKey, LabelKey: "integrations.fields.api_key", Secret: true, Required: true},
		{Key: FieldSecretKey, LabelKey: "integrations.fields.secret_key", Secret: true, Required: true},
	}
}

func (a *Adapter) baseURL(account integrations.Account) string {
	if account.Mode == integrations.ModeLive {
		return a.cfg.LiveBaseURL
	}
	return a.cfg.SandboxBaseURL
}

// FormatPrice renders an amount the way iyzico's SDKs do: no trailing
// zeros, at least one decimal ("1500.0", "1500.5", "0.25").
func FormatPrice(m money.Money) string {
	s := m.String()
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		if strings.HasSuffix(s, ".") {
			s += "0"
		}
	}
	return s
}

// randomKey is the per-request nonce of IYZWSv2.
func (a *Adapter) randomKey() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000_000))
	if err != nil {
		n = big.NewInt(0)
	}
	return strconv.FormatInt(a.cfg.Now().UnixMilli(), 10) + fmt.Sprintf("%09d", n.Int64())
}

// AuthorizationHeader builds the IYZWSv2 header value of a request.
func AuthorizationHeader(apiKey, secretKey, randomKey, path string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(randomKey))
	mac.Write([]byte(path))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	params := "apiKey:" + apiKey + "&randomKey:" + randomKey + "&signature:" + sig
	return "IYZWSv2 " + base64.StdEncoding.EncodeToString([]byte(params))
}

// Sign is iyzico's answer signature: hex(HMAC-SHA256(secretKey, join(":"))).
func Sign(secretKey string, params ...string) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(strings.Join(params, ":")))
	return hex.EncodeToString(mac.Sum(nil))
}

func equalHex(a, b string) bool {
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(a))), []byte(strings.ToLower(strings.TrimSpace(b))))
}

// post sends one signed request and decodes the JSON answer (numbers kept
// as json.Number so signed values keep their text).
func (a *Adapter) post(ctx context.Context, account integrations.Account, path string, body any) (map[string]any, error) {
	apiKey, secret := account.Credentials[FieldAPIKey], account.Credentials[FieldSecretKey]
	if apiKey == "" || secret == "" {
		return nil, &payments.ProviderError{Code: "credentials_missing", Message: "api key and secret key are required"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	rnd := a.randomKey()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL(account)+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-iyzi-rnd", rnd)
	req.Header.Set("Authorization", AuthorizationHeader(apiKey, secret, rnd, path, raw))
	resp, err := a.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, &payments.ProviderError{Code: "network", Message: err.Error(), Temporary: true}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &payments.ProviderError{Code: "network", Message: err.Error(), Temporary: true}
	}
	if resp.StatusCode >= 500 {
		return nil, &payments.ProviderError{Code: "http_" + strconv.Itoa(resp.StatusCode), Message: "provider unavailable", Temporary: true}
	}
	out := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: %v", payments.ErrMalformed, err)
	}
	if str(out, "status") != "success" {
		return out, &payments.ProviderError{Code: str(out, "errorCode"), Message: str(out, "errorMessage")}
	}
	return out, nil
}

func str(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func amount(m map[string]any, key string) money.Money {
	v, err := money.ParseRound(str(m, key))
	if err != nil {
		return money.Zero()
	}
	return v
}

// numberForms are the textual forms a signed number may have been signed
// with ("1500", "1500.0", "1500.00" all name 1500): iyzico signs the value
// it returns; the raw JSON text is tried first.
func numberForms(raw string) []string {
	out := []string{raw}
	m, err := money.ParseRound(raw)
	if err != nil {
		return out
	}
	for _, f := range []string{FormatPrice(m), m.String(), strings.TrimSuffix(FormatPrice(m), ".0")} {
		dup := false
		for _, o := range out {
			if o == f {
				dup = true
			}
		}
		if !dup {
			out = append(out, f)
		}
	}
	return out
}

// Check implements integrations.Adapter: an authenticated BIN lookup (no
// money moves) proves the keys and the mode.
func (a *Adapter) Check(ctx context.Context, account integrations.Account) error {
	_, err := a.post(ctx, account, pathBinCheck, map[string]any{
		"locale": "tr", "conversationId": "connection-check", "binNumber": "554960",
	})
	return err
}

// CreateCheckout implements payments.Provider (Checkout Form initialize).
func (a *Adapter) CreateCheckout(ctx context.Context, account integrations.Account, in payments.CheckoutRequest) (payments.Checkout, error) {
	price := FormatPrice(in.Amount)
	locale := in.Locale
	if locale == "" {
		locale = "tr"
	}
	b := in.Buyer
	address := map[string]any{
		"contactName": strings.TrimSpace(b.Name + " " + b.Surname), "city": b.City, "country": b.Country, "address": b.Address,
	}
	body := map[string]any{
		"locale":         locale,
		"conversationId": in.ConversationID,
		"price":          price,
		"paidPrice":      price,
		"currency":       in.Currency,
		"basketId":       in.BasketID,
		"paymentGroup":   "PRODUCT",
		"callbackUrl":    in.CallbackURL,
		"buyer": map[string]any{
			"id": b.ID, "name": b.Name, "surname": b.Surname, "gsmNumber": b.Phone, "email": b.Email,
			"identityNumber": b.IdentityNumber, "registrationAddress": b.Address, "ip": b.IP, "city": b.City,
			"country": b.Country,
		},
		"billingAddress": address,
		"basketItems": []map[string]any{{
			"id": in.ItemID, "name": in.ItemName, "category1": in.ItemCategory, "itemType": "VIRTUAL", "price": price,
		}},
	}
	out, err := a.post(ctx, account, pathInitialize, body)
	if err != nil {
		return payments.Checkout{}, err
	}
	token := str(out, "token")
	if token == "" || str(out, "paymentPageUrl") == "" {
		return payments.Checkout{}, fmt.Errorf("%w: initialize without token / page url", payments.ErrMalformed)
	}
	if sig := str(out, "signature"); sig != "" {
		if !equalHex(sig, Sign(account.Credentials[FieldSecretKey], str(out, "conversationId"), token)) {
			return payments.Checkout{}, payments.ErrInvalidSignature
		}
	}
	if str(out, "conversationId") != "" && str(out, "conversationId") != in.ConversationID {
		return payments.Checkout{}, fmt.Errorf("%w: conversation id mismatch", payments.ErrMalformed)
	}
	c := payments.Checkout{Token: token, PaymentPageURL: str(out, "paymentPageUrl")}
	if secs, err := strconv.Atoi(str(out, "tokenExpireTime")); err == nil && secs > 0 {
		t := a.cfg.Now().Add(time.Duration(secs) * time.Second).UTC()
		c.ExpiresAt = &t
	}
	return c, nil
}

// Retrieve implements payments.Provider (Checkout Form result).
func (a *Adapter) Retrieve(ctx context.Context, account integrations.Account, in payments.RetrieveRequest) (payments.Result, error) {
	locale := in.Locale
	if locale == "" {
		locale = "tr"
	}
	out, err := a.post(ctx, account, pathRetrieve, map[string]any{
		"locale": locale, "conversationId": in.ConversationID, "token": in.Token,
	})
	if err != nil {
		return payments.Result{}, err
	}
	if sig := str(out, "signature"); sig != "" {
		secret := account.Credentials[FieldSecretKey]
		ok := false
		for _, paid := range numberForms(str(out, "paidPrice")) {
			for _, price := range numberForms(str(out, "price")) {
				if equalHex(sig, Sign(secret, str(out, "paymentStatus"), str(out, "paymentId"), str(out, "currency"),
					str(out, "basketId"), str(out, "conversationId"), paid, price, str(out, "token"))) {
					ok = true
				}
			}
		}
		if !ok {
			return payments.Result{}, payments.ErrInvalidSignature
		}
	}
	r := payments.Result{
		ProviderStatus: str(out, "paymentStatus"),
		PaymentID:      str(out, "paymentId"),
		Token:          str(out, "token"),
		ConversationID: str(out, "conversationId"),
		BasketID:       str(out, "basketId"),
		Currency:       str(out, "currency"),
		Price:          amount(out, "price"),
		PaidPrice:      amount(out, "paidPrice"),
		Commission:     amount(out, "iyziCommissionRateAmount").Add(amount(out, "iyziCommissionFee")),
		Data:           map[string]any{},
	}
	for _, k := range []string{"cardAssociation", "cardFamily", "cardType", "lastFourDigits", "installment", "fraudStatus", "mdStatus"} {
		if v := str(out, k); v != "" {
			r.Data[k] = v
		}
	}
	switch strings.ToUpper(r.ProviderStatus) {
	case "SUCCESS":
		switch str(out, "fraudStatus") {
		case "", "1":
			r.Status = payments.StatusSucceeded
		case "-1":
			r.Status, r.FailureReason = payments.StatusFailed, "fraud_rejected"
		default: // 0: under fraud review, not confirmed yet
			r.Status = payments.StatusPending
		}
	case "FAILURE":
		r.Status = payments.StatusFailed
		r.FailureReason = strings.TrimSpace(str(out, "errorCode") + " " + str(out, "errorMessage"))
		if r.FailureReason == "" {
			r.FailureReason = "payment_failed"
		}
	default:
		r.Status = payments.StatusPending
	}
	return r, nil
}

// Refund implements payments.Provider (refund v2, by payment id).
func (a *Adapter) Refund(ctx context.Context, account integrations.Account, in payments.RefundRequest) (payments.RefundResult, error) {
	locale := in.Locale
	if locale == "" {
		locale = "tr"
	}
	out, err := a.post(ctx, account, pathRefund, map[string]any{
		"locale": locale, "conversationId": in.ConversationID, "paymentId": in.PaymentID,
		"price": FormatPrice(in.Amount), "currency": in.Currency, "ip": in.IP,
	})
	if err != nil {
		return payments.RefundResult{}, err
	}
	id := ""
	for _, k := range []string{"refundHostReference", "hostReference", "paymentTransactionId", "paymentId"} {
		if v := str(out, k); v != "" {
			id = v
			break
		}
	}
	return payments.RefundResult{RefundID: id, Amount: amount(out, "price")}, nil
}

type webhookBody struct {
	EventType      string `json:"iyziEventType"`
	EventTime      any    `json:"iyziEventTime"`
	ReferenceCode  string `json:"iyziReferenceCode"`
	IyziPaymentID  any    `json:"iyziPaymentId"`
	PaymentID      any    `json:"paymentId"`
	Token          string `json:"token"`
	ConversationID string `json:"paymentConversationId"`
	MerchantID     any    `json:"merchantId"`
	Status         string `json:"status"`
}

func anyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

// ParseWebhook implements payments.Provider.
func (a *Adapter) ParseWebhook(_ http.Header, body []byte) (payments.Webhook, error) {
	var w webhookBody
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&w); err != nil {
		return payments.Webhook{}, fmt.Errorf("%w: %v", payments.ErrMalformed, err)
	}
	h := payments.Webhook{
		EventType: w.EventType, Token: w.Token, ConversationID: w.ConversationID, Status: w.Status,
		ReferenceCode: w.ReferenceCode, MerchantID: anyString(w.MerchantID), PaymentID: anyString(w.IyziPaymentID),
	}
	if h.PaymentID == "" {
		h.PaymentID = anyString(w.PaymentID)
	}
	if h.ConversationID == "" || h.EventType == "" || h.Status == "" {
		return payments.Webhook{}, fmt.Errorf("%w: missing webhook fields", payments.ErrMalformed)
	}
	return h, nil
}

// WebhookSignature is the X-IYZ-SIGNATURE-V3 value of a notification:
// HMAC-SHA256(secretKey, secretKey + eventType + paymentId + token +
// conversationId + status) for hosted (checkout form) payments, without
// the token for direct (API) payments; hex encoded.
func WebhookSignature(secretKey string, h payments.Webhook) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	key := secretKey + h.EventType + h.PaymentID
	if h.Token != "" {
		key += h.Token
	}
	key += h.ConversationID + h.Status
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook implements payments.Provider.
func (a *Adapter) VerifyWebhook(account integrations.Account, h payments.Webhook, header http.Header, _ []byte) error {
	got := header.Get(SignatureHeader)
	secret := account.Credentials[FieldSecretKey]
	if got == "" || secret == "" {
		return payments.ErrInvalidSignature
	}
	if !equalHex(got, WebhookSignature(secret, h)) {
		return payments.ErrInvalidSignature
	}
	return nil
}
