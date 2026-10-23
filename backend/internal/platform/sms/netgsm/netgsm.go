// Package netgsm is the Netgsm SMS adapter (TEC-734). It uses Netgsm's
// REST v2 API with HTTP Basic authentication (usercode:password):
//
//	POST {base}/sms/rest/v2/send       {"msgheader","encoding","iysfilter","messages":[{"msg","no"}]}
//	POST {base}/sms/rest/v2/otp        {"msgheader","msg","no"}  (OTP SMS: fast lane, no İYS check)
//	POST {base}/sms/rest/v2/report     {"bulkIds":["<jobid>"]}  (delivery report)
//	GET  {base}/sms/rest/v2/msgheader  (connection test: the account's headers)
//
// Answers carry "code" ("00" = accepted) and "jobid". The request format
// follows Netgsm's public REST v2 documentation and SDKs as known at build
// time; netgsm.com.tr was not reachable from the build environment, so
// docs/design/sms.md §8 lists every detail to confirm against a real
// account. Classification: an answered error code is a definitive
// *sms.Rejected; a transport error or an HTTP 5xx is an unknown outcome.
package netgsm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

// Provider code, default base URL and paths.
const (
	ProviderCode = "netgsm"
	DefaultLive  = "https://api.netgsm.com.tr"

	PathSend      = "/sms/rest/v2/send"
	PathOTP       = "/sms/rest/v2/otp"
	PathReport    = "/sms/rest/v2/report"
	PathMsgHeader = "/sms/rest/v2/msgheader"
)

// Credential field keys.
const (
	FieldUserCode  = "usercode"
	FieldPassword  = "password"
	FieldMsgHeader = "msgheader"
)

// Answer codes.
const (
	CodeOK        = "00"
	CodeNoRecords = "60"
)

// rejections maps answered error codes to a description and whether sending
// again may help (Netgsm REST v2 codes; see the package doc).
var rejections = map[string]struct {
	msg       string
	retryable bool
}{
	"20":  {"message text is invalid or too long", false},
	"30":  {"invalid usercode / password or no API access (check the IP restriction)", false},
	"40":  {"the message header (msgheader) is not defined for the account", false},
	"41":  {"the message header (msgheader) is not defined for the account", false},
	"50":  {"the number is blocked by İYS control", false},
	"51":  {"no İYS brand code is defined for the account", false},
	"70":  {"invalid or missing parameters", false},
	"80":  {"sending limit exceeded", true},
	"85":  {"duplicate send limit reached for the number", true},
	"100": {"Netgsm system error", true},
}

// Report status codes ("status" of a report job).
var reportStates = map[int]sms.DeliveryState{
	0: sms.DeliveryPending, // waiting for the operator's report
	1: sms.DeliveryDelivered,
}

// Config holds the base URLs and the HTTP client. Netgsm has no sandbox:
// SandboxBaseURL is empty in production (sandbox accounts then refuse to
// send) and points at netgsmfake / cmd/netgsm-fake in development.
//
// RefuseOfficial (TEC-744, platform/envguard): when set, live accounts are
// refused while LiveBaseURL is Netgsm's official host (the API outside
// production); a live URL pointed at netgsmfake stays usable.
type Config struct {
	LiveBaseURL    string
	SandboxBaseURL string
	HTTPClient     *http.Client
	RefuseOfficial error
}

// OfficialHost is the host of Netgsm's production API.
const OfficialHost = "api.netgsm.com.tr"

// IsOfficial reports whether baseURL points at Netgsm's production API.
func IsOfficial(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == OfficialHost || strings.HasSuffix(host, ".netgsm.com.tr") || host == "netgsm.com.tr"
}

// Adapter implements sms.Provider for Netgsm.
type Adapter struct {
	cfg Config
}

var _ sms.Provider = (*Adapter)(nil)

// New builds the adapter.
func New(cfg Config) *Adapter {
	if strings.TrimSpace(cfg.LiveBaseURL) == "" {
		cfg.LiveBaseURL = DefaultLive
	}
	cfg.LiveBaseURL = strings.TrimRight(strings.TrimSpace(cfg.LiveBaseURL), "/")
	cfg.SandboxBaseURL = strings.TrimRight(strings.TrimSpace(cfg.SandboxBaseURL), "/")
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Adapter{cfg: cfg}
}

// Kind implements integrations.Adapter.
func (a *Adapter) Kind() string { return integrations.KindSMS }

// Provider implements integrations.Adapter.
func (a *Adapter) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (a *Adapter) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: FieldUserCode, LabelKey: "payments.integrations.fields.usercode", Secret: true, Required: true},
		{Key: FieldPassword, LabelKey: "payments.integrations.fields.password", Secret: true, Required: true},
		{Key: FieldMsgHeader, LabelKey: "payments.integrations.fields.msgheader", Secret: false, Required: true},
	}
}

func (a *Adapter) baseURL(acc integrations.Account) (string, error) {
	if acc.Mode == integrations.ModeLive {
		if a.cfg.RefuseOfficial != nil && IsOfficial(a.cfg.LiveBaseURL) {
			return "", a.cfg.RefuseOfficial
		}
		return a.cfg.LiveBaseURL, nil
	}
	if a.cfg.SandboxBaseURL == "" {
		return "", sms.ErrNoSandbox
	}
	return a.cfg.SandboxBaseURL, nil
}

// Number renders a normalized number the way Netgsm expects it: Turkish
// mobile numbers as 10 national digits ("5321234567"), others with the
// international "00" prefix.
func Number(normalized string) string {
	d := pii.Digits(normalized)
	if strings.HasPrefix(d, "90") && len(d) == 12 {
		return d[2:]
	}
	return "00" + d
}

type answer struct {
	Code        string          `json:"code"`
	JobID       string          `json:"jobid"`
	Description string          `json:"description"`
	MsgHeaders  []string        `json:"msgheaders"`
	Jobs        []reportJob     `json:"jobs"`
	Raw         json.RawMessage `json:"-"`
}

type reportJob struct {
	JobID         string `json:"jobid"`
	Status        int    `json:"status"`
	ErrorCode     any    `json:"errorCode"`
	DeliveredDate string `json:"deliveredDate"`
}

// errTransport marks an unknown outcome (the request may have arrived).
type errTransport struct{ err error }

func (e *errTransport) Error() string { return "netgsm: " + e.err.Error() }
func (e *errTransport) Unwrap() error { return e.err }

// IsUncertain reports whether err leaves the outcome of a call unknown.
func IsUncertain(err error) bool {
	var t *errTransport
	return errors.As(err, &t)
}

func (a *Adapter) do(ctx context.Context, acc integrations.Account, method, path string, body any) (answer, error) {
	base, err := a.baseURL(acc)
	if err != nil {
		code := "sandbox"
		if acc.Mode == integrations.ModeLive {
			code = "environment"
		}
		return answer{}, &sms.Rejected{Code: code, Message: err.Error()}
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return answer{}, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return answer{}, err
	}
	req.SetBasicAuth(acc.Credentials[FieldUserCode], acc.Credentials[FieldPassword])
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := a.cfg.HTTPClient.Do(req)
	if err != nil {
		return answer{}, &errTransport{err: err}
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return answer{}, &errTransport{err: err}
	}
	if res.StatusCode >= 500 {
		return answer{}, &errTransport{err: fmt.Errorf("http %d", res.StatusCode)}
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return answer{}, &sms.Rejected{Code: "30", Message: rejections["30"].msg}
	}
	var out answer
	if err := json.Unmarshal(raw, &out); err != nil {
		if res.StatusCode >= 400 {
			return answer{}, &sms.Rejected{Code: strconv.Itoa(res.StatusCode), Message: "http " + strconv.Itoa(res.StatusCode)}
		}
		// A 2xx we cannot read: the provider may have accepted it.
		return answer{}, &errTransport{err: fmt.Errorf("unreadable answer: %w", err)}
	}
	out.Raw = raw
	return out, nil
}

func rejection(code, desc string) *sms.Rejected {
	r, ok := rejections[code]
	if !ok {
		msg := desc
		if msg == "" {
			msg = "unknown error code"
		}
		return &sms.Rejected{Code: code, Message: msg}
	}
	return &sms.Rejected{Code: code, Message: r.msg, Retryable: r.retryable}
}

// Check implements integrations.Adapter: the account's message headers
// (authenticated, sends nothing); the configured header must be one of them.
func (a *Adapter) Check(ctx context.Context, acc integrations.Account) error {
	out, err := a.do(ctx, acc, http.MethodGet, PathMsgHeader, nil)
	if err != nil {
		return err
	}
	if out.Code != CodeOK {
		return rejection(out.Code, out.Description)
	}
	want := strings.TrimSpace(acc.Credentials[FieldMsgHeader])
	if len(out.MsgHeaders) > 0 && want != "" {
		for _, h := range out.MsgHeaders {
			if strings.EqualFold(strings.TrimSpace(h), want) {
				return nil
			}
		}
		return &sms.Rejected{Code: "40", Message: rejections["40"].msg}
	}
	return nil
}

// Send implements sms.Provider. OTP messages use the OTP endpoint; other
// classes the bulk endpoint with one message (Turkish encoding).
func (a *Adapter) Send(ctx context.Context, acc integrations.Account, msg sms.Message) (sms.SendResult, error) {
	header := strings.TrimSpace(acc.Credentials[FieldMsgHeader])
	no := Number(msg.To)
	var (
		out answer
		err error
	)
	if msg.Class == sms.ClassOTP {
		out, err = a.do(ctx, acc, http.MethodPost, PathOTP, map[string]any{
			"msgheader": header, "msg": msg.Body, "no": no,
		})
	} else {
		body := map[string]any{
			"msgheader": header, "encoding": "TR",
			"messages": []map[string]string{{"msg": msg.Body, "no": no}},
		}
		if msg.Class == sms.ClassInformational {
			// İYS filter for individual recipients (BIREYSEL): Netgsm drops
			// numbers without consent at İYS as well.
			body["iysfilter"] = "11"
		} else {
			body["iysfilter"] = "0"
		}
		out, err = a.do(ctx, acc, http.MethodPost, PathSend, body)
	}
	if err != nil {
		return sms.SendResult{}, err
	}
	if out.Code != CodeOK {
		return sms.SendResult{Code: out.Code}, rejection(out.Code, out.Description)
	}
	if strings.TrimSpace(out.JobID) == "" {
		return sms.SendResult{Code: out.Code}, &errTransport{err: errors.New("accepted without a jobid")}
	}
	return sms.SendResult{MessageID: out.JobID, Code: out.Code}, nil
}

// Status implements sms.Provider (delivery report by jobid).
func (a *Adapter) Status(ctx context.Context, acc integrations.Account, messageID string) (sms.Delivery, error) {
	if strings.TrimSpace(messageID) == "" {
		// Netgsm reports are looked up by jobid only.
		return sms.Delivery{}, fmt.Errorf("%w: netgsm reports need a jobid", integrations.ErrUnsupported)
	}
	out, err := a.do(ctx, acc, http.MethodPost, PathReport, map[string]any{"bulkIds": []string{messageID}})
	if err != nil {
		return sms.Delivery{}, err
	}
	if out.Code == CodeNoRecords {
		return sms.Delivery{State: sms.DeliveryNotFound, Code: out.Code}, nil
	}
	if out.Code != CodeOK {
		return sms.Delivery{}, rejection(out.Code, out.Description)
	}
	for _, j := range out.Jobs {
		if j.JobID != "" && j.JobID != messageID {
			continue
		}
		d := sms.Delivery{Code: strconv.Itoa(j.Status)}
		if st, ok := reportStates[j.Status]; ok {
			d.State = st
		} else {
			d.State = sms.DeliveryUndelivered
		}
		if t, err := time.ParseInLocation("02.01.2006 15:04:05", j.DeliveredDate, istanbul()); err == nil {
			tt := t.UTC()
			d.At = &tt
		}
		return d, nil
	}
	return sms.Delivery{State: sms.DeliveryPending, Code: out.Code}, nil
}

func istanbul() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}
