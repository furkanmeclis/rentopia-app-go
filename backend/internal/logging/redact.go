package logging

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// Redacted replaces a masked value.
const Redacted = "[redacted]"

// secretKeyParts mark attribute keys whose values are never written to
// stdout or the log viewer (compared on the lower-cased key without "_",
// "-" and ".").
var secretKeyParts = []string{
	"password", "passwd", "secret", "token", "authorization", "cookie",
	"apikey", "privatekey", "accesskey", "credential", "dsn",
	"iban", "cardnumber", "cardno", "cvv", "cvc",
}

// secretKeyWords mark keys with one of these words ("card", "card.last",
// "otp_code" → "otp").
var secretKeyWords = map[string]bool{"card": true, "pan": true, "otp": true, "pin": true}

var (
	reJWT    = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]*`)
	reBearer = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=\-]+`)
	// key=value and JSON "key":"value" pairs whose key looks secret.
	reSecretPair = regexp.MustCompile(`(?i)\b((?:access_|refresh_|id_)?token|password|passwd|secret|api[_-]?key|authorization|iban|card_?number)(\s*=\s*|"\s*:\s*"?)([^\s"'&,;]+)`)
	// Turkish IBAN (TR + 24 digits, optionally grouped).
	reIBAN = regexp.MustCompile(`(?i)\bTR\d{2}(?:\s?\d{4}){5}\s?\d{2}\b`)
	// 13–19 digit runs (optionally grouped by spaces / dashes): card
	// numbers are masked when the Luhn check passes.
	reCard = regexp.MustCompile(`\b\d(?:[ \-]?\d){12,18}\b`)
)

// IsSecretKey reports whether an attribute key names a secret (password,
// token, secret, authorization, api key, IBAN, card …).
func IsSecretKey(key string) bool {
	k := strings.ToLower(key)
	compact := strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(k)
	for _, part := range secretKeyParts {
		if strings.Contains(compact, part) {
			return true
		}
	}
	for _, w := range strings.FieldsFunc(k, func(r rune) bool { return r == '_' || r == '-' || r == '.' || r == ' ' }) {
		if secretKeyWords[w] {
			return true
		}
	}
	return false
}

// RedactString masks bearer / basic credentials, JWTs, key=value secrets,
// IBANs and card numbers inside free text.
func RedactString(s string) string {
	if s == "" {
		return s
	}
	s = reJWT.ReplaceAllString(s, "[token]")
	s = reBearer.ReplaceAllString(s, "$1 [token]")
	s = reSecretPair.ReplaceAllString(s, "$1$2"+Redacted)
	s = reIBAN.ReplaceAllString(s, "[iban]")
	s = reCard.ReplaceAllStringFunc(s, func(m string) string {
		if luhn(m) {
			return "[card]"
		}
		return m
	})
	return s
}

func luhn(s string) bool {
	sum, n, double := 0, 0, false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			continue
		}
		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		n++
		double = !double
	}
	return n >= 13 && sum%10 == 0
}

// RedactValue masks a value logged under key: secret keys become
// Redacted, strings and errors go through RedactString, maps and slices
// are walked.
func RedactValue(key string, v any) any {
	if key != "" && IsSecretKey(key) {
		return Redacted
	}
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return RedactString(t)
	case error:
		return RedactString(t.Error())
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = RedactValue(k, val)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(t))
		for k, val := range t {
			if IsSecretKey(k) {
				out[k] = Redacted
				continue
			}
			out[k] = RedactString(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = RedactValue("", val)
		}
		return out
	case []string:
		out := make([]string, len(t))
		for i, val := range t {
			out[i] = RedactString(val)
		}
		return out
	case fmt.Stringer:
		return RedactString(t.String())
	default:
		return v
	}
}

// RedactAttr is a slog.HandlerOptions.ReplaceAttr: it masks secret keys
// and secret-looking text in every attribute (groups are visited child by
// child by the handler).
func RedactAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey || a.Key == slog.LevelKey || a.Key == slog.SourceKey {
		return a
	}
	if a.Key != slog.MessageKey && IsSecretKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		if r := RedactString(v.String()); r != v.String() {
			return slog.String(a.Key, r)
		}
	case slog.KindAny:
		return slog.Any(a.Key, RedactValue("", v.Any()))
	}
	return a
}

// OrganizationFromContext returns the organization (uuid) of a request
// context; wired by the organization middleware so logs carry it.
var OrganizationFromContext = func(context.Context) string { return "" }

// contextHandler adds request_id and organization_uuid from the context
// to every record logged with a context (log.InfoContext …), unless the
// record already has them.
type contextHandler struct {
	next slog.Handler
}

func (h contextHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h contextHandler) Handle(ctx context.Context, rec slog.Record) error {
	if ctx != nil {
		has := map[string]bool{}
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key == "request_id" || a.Key == "organization_uuid" {
				has[a.Key] = true
			}
			return true
		})
		if id := RequestIDFromContext(ctx); id != "" && !has["request_id"] {
			rec.AddAttrs(slog.String("request_id", id))
		}
		if org := OrganizationFromContext(ctx); org != "" && !has["organization_uuid"] {
			rec.AddAttrs(slog.String("organization_uuid", org))
		}
	}
	return h.next.Handle(ctx, rec)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{next: h.next.WithGroup(name)}
}
