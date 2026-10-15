// Package netgsmfake is an in-process fake of the Netgsm REST v2 endpoints
// the adapter uses (send, otp, report, msgheader). It checks Basic
// authentication and the message header, records messages, answers
// delivery reports and can inject failures (an answered error code, an
// accepted message whose answer is lost, a 5xx). Tests and local checks
// (cmd/netgsm-fake) use it; it never runs in production.
package netgsmfake

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
)

// Message is one accepted message.
type Message struct {
	JobID    string
	No       string
	Text     string
	Header   string
	OTP      bool
	Status   int // 0 waiting, 1 delivered, other = failed code
	Accepted time.Time
}

// Account is a usercode with its password and headers.
type Account struct {
	Password string
	Headers  []string
}

// Fake is the fake server.
type Fake struct {
	Server *httptest.Server

	mu       sync.Mutex
	accounts map[string]Account
	messages map[string]*Message
	order    []string
	seq      int64
	// failNext answers the next send with this code ("" = none).
	failNext string
	// dropNext accepts the next send, then drops the connection (the
	// client cannot know it was accepted).
	dropNext bool
	// serverErrorNext answers the next request with HTTP 503 (nothing
	// recorded).
	serverErrorNext bool
	// reportErrorNext answers the next report with HTTP 503.
	reportErrorNext bool
	// AutoDeliver marks messages delivered when they are accepted.
	AutoDeliver bool

	Sends   atomic.Int64
	Reports atomic.Int64
}

// New starts a fake (Close it with f.Server.Close()).
func New() *Fake {
	f := NewUnstarted()
	f.Server = httptest.NewServer(f.Handler())
	return f
}

// NewUnstarted builds a fake whose handler can be mounted elsewhere.
func NewUnstarted() *Fake {
	return &Fake{accounts: map[string]Account{}, messages: map[string]*Message{}}
}

// Handler serves the fake endpoints.
func (f *Fake) Handler() http.Handler { return http.HandlerFunc(f.serve) }

// URL is the base URL of a started fake.
func (f *Fake) URL() string { return f.Server.URL }

// AddAccount registers credentials and their message headers.
func (f *Fake) AddAccount(usercode, password string, headers ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[usercode] = Account{Password: password, Headers: headers}
}

// FailNext answers the next send with an error code.
func (f *Fake) FailNext(code string) { f.mu.Lock(); f.failNext = code; f.mu.Unlock() }

// DropNext accepts the next send and drops the connection.
func (f *Fake) DropNext() { f.mu.Lock(); f.dropNext = true; f.mu.Unlock() }

// ServerErrorNext answers the next send with HTTP 503 without recording it.
func (f *Fake) ServerErrorNext() { f.mu.Lock(); f.serverErrorNext = true; f.mu.Unlock() }

// ReportErrorNext answers the next report with HTTP 503.
func (f *Fake) ReportErrorNext() { f.mu.Lock(); f.reportErrorNext = true; f.mu.Unlock() }

// SetStatus sets a message's report status (1 delivered, 0 waiting, other
// codes undelivered).
func (f *Fake) SetStatus(jobID string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.messages[jobID]; ok {
		m.Status = status
	}
}

// Messages returns the accepted messages in order.
func (f *Fake) Messages() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Message, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, *f.messages[id])
	}
	return out
}

func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *Fake) auth(r *http.Request) (Account, bool) {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return Account{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	acc, found := f.accounts[user]
	if !found || acc.Password != pass {
		return Account{}, false
	}
	return acc, true
}

func hasHeader(acc Account, h string) bool {
	if len(acc.Headers) == 0 {
		return true
	}
	for _, x := range acc.Headers {
		if strings.EqualFold(x, h) {
			return true
		}
	}
	return false
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/_fake/inbox") {
		f.inbox(w)
		return
	}
	acc, ok := f.auth(r)
	if !ok {
		write(w, map[string]any{"code": "30", "description": "credentials"})
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == netgsm.PathMsgHeader:
		write(w, map[string]any{"code": "00", "msgheaders": acc.Headers})
	case r.Method == http.MethodPost && (r.URL.Path == netgsm.PathSend || r.URL.Path == netgsm.PathOTP):
		f.send(w, r, acc, r.URL.Path == netgsm.PathOTP)
	case r.Method == http.MethodPost && r.URL.Path == netgsm.PathReport:
		f.report(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *Fake) send(w http.ResponseWriter, r *http.Request, acc Account, otp bool) {
	f.Sends.Add(1)
	var in struct {
		MsgHeader string `json:"msgheader"`
		Msg       string `json:"msg"`
		No        string `json:"no"`
		Messages  []struct {
			Msg string `json:"msg"`
			No  string `json:"no"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		write(w, map[string]any{"code": "70", "description": "invalid body"})
		return
	}
	if !otp && len(in.Messages) > 0 {
		in.Msg, in.No = in.Messages[0].Msg, in.Messages[0].No
	}
	f.mu.Lock()
	failNext, dropNext, serverErr := f.failNext, f.dropNext, f.serverErrorNext
	f.failNext, f.dropNext, f.serverErrorNext = "", false, false
	f.mu.Unlock()
	if serverErr {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if failNext != "" {
		write(w, map[string]any{"code": failNext, "description": "injected"})
		return
	}
	if in.MsgHeader == "" || !hasHeader(acc, in.MsgHeader) {
		write(w, map[string]any{"code": "40", "description": "msgheader"})
		return
	}
	if in.Msg == "" || in.No == "" {
		write(w, map[string]any{"code": "70", "description": "parameters"})
		return
	}
	f.mu.Lock()
	f.seq++
	id := fmt.Sprintf("1700%016d", f.seq)
	m := &Message{JobID: id, No: in.No, Text: in.Msg, Header: in.MsgHeader, OTP: otp, Accepted: time.Now()}
	if f.AutoDeliver {
		m.Status = 1
	}
	f.messages[id] = m
	f.order = append(f.order, id)
	f.mu.Unlock()
	if dropNext {
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				_ = conn.Close()
				return
			}
		}
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	write(w, map[string]any{"code": "00", "jobid": id, "description": "queued"})
}

func (f *Fake) report(w http.ResponseWriter, r *http.Request) {
	f.Reports.Add(1)
	f.mu.Lock()
	reportErr := f.reportErrorNext
	f.reportErrorNext = false
	f.mu.Unlock()
	if reportErr {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	var in struct {
		BulkIDs []string `json:"bulkIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		write(w, map[string]any{"code": "70"})
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	jobs := []map[string]any{}
	for _, id := range in.BulkIDs {
		m, ok := f.messages[id]
		if !ok {
			continue
		}
		job := map[string]any{"jobid": id, "telno": m.No, "status": m.Status, "errorCode": 0}
		if m.Status == 1 {
			job["deliveredDate"] = m.Accepted.In(time.FixedZone("TRT", 3*3600)).Format("02.01.2006 15:04:05")
		}
		jobs = append(jobs, job)
	}
	if len(jobs) == 0 {
		write(w, map[string]any{"code": "60", "description": "no records"})
		return
	}
	write(w, map[string]any{"code": "00", "jobs": jobs})
}

// inbox renders the accepted messages (local checks: the fake is the
// "phone"; OTP codes appear here only, never in the app's logs).
func (f *Fake) inbox(w http.ResponseWriter) {
	msgs := f.Messages()
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].Accepted.After(msgs[j].Accepted) })
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=utf-8><title>Netgsm fake</title><body style=\"font-family:sans-serif\"><h1>Netgsm fake inbox</h1><ul>")
	for _, m := range msgs {
		fmt.Fprintf(&b, "<li><b>%s</b> → %s (%s, otp=%v): %s</li>", html.EscapeString(m.Header), html.EscapeString(m.No),
			html.EscapeString(m.JobID), m.OTP, html.EscapeString(m.Text))
	}
	b.WriteString("</ul></body>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
