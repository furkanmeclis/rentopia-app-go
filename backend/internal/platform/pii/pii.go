// Package pii holds the personal data rules shared by tenant modules
// (customers and drivers, TEC-705): Turkish identity checksums, value
// normalization (the input of blind indexes) and the masking rules the API
// applies whenever a caller has not explicitly revealed a value.
//
// Masking happens server-side only, in this one place. The browser never
// receives an unmasked identity, licence, phone or e-mail value except from
// an explicit reveal endpoint that records an activity event.
package pii

import (
	"strings"
	"unicode"
)

// ValidVKN reports whether v is a valid Turkish tax identification number
// (Vergi Kimlik Numarası, 10 digits, GİB checksum).
func ValidVKN(v string) bool {
	if len(v) != 10 || !allDigits(v) {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		d := int(v[i] - '0')
		tmp := (d + 9 - i) % 10
		if tmp == 0 {
			continue
		}
		p := (tmp * (1 << (9 - i))) % 9
		if p == 0 {
			p = 9
		}
		sum += p
	}
	check := (10 - sum%10) % 10
	return check == int(v[9]-'0')
}

// ValidTCKN reports whether v is a valid Turkish national identity number
// (T.C. Kimlik No, 11 digits, first digit not zero, two check digits).
func ValidTCKN(v string) bool {
	if len(v) != 11 || !allDigits(v) || v[0] == '0' {
		return false
	}
	d := make([]int, 11)
	for i := range v {
		d[i] = int(v[i] - '0')
	}
	odd := d[0] + d[2] + d[4] + d[6] + d[8]
	even := d[1] + d[3] + d[5] + d[7]
	d10 := ((odd*7-even)%10 + 10) % 10
	if d10 != d[9] {
		return false
	}
	total := 0
	for i := 0; i < 10; i++ {
		total += d[i]
	}
	return total%10 == d[10]
}

func allDigits(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Digits keeps the ASCII digits of v (TCKN / VKN input with spaces).
func Digits(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Document normalizes a passport or driving licence number: upper case
// letters and digits only.
func Document(v string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(v)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Phone normalizes a phone number to "+<digits>". Turkish numbers written
// nationally ("0532 …", "532 …") get the +90 prefix. It returns "" when v
// holds no digits.
func Phone(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	plus := strings.HasPrefix(v, "+")
	d := Digits(v)
	switch {
	case d == "":
		return ""
	case plus:
		return "+" + d
	case strings.HasPrefix(d, "00"):
		return "+" + d[2:]
	case strings.HasPrefix(d, "90") && len(d) == 12:
		return "+" + d
	case strings.HasPrefix(d, "0") && len(d) == 11:
		return "+90" + d[1:]
	case len(d) == 10 && d[0] == '5':
		return "+90" + d
	}
	return "+" + d
}

// ValidPhone reports whether a normalized phone number is plausible:
// Turkish numbers have 10 national digits, others 7 to 15 digits (E.164).
func ValidPhone(normalized string) bool {
	if !strings.HasPrefix(normalized, "+") {
		return false
	}
	d := normalized[1:]
	if !allDigits(d) {
		return false
	}
	if strings.HasPrefix(d, "90") {
		return len(d) == 12
	}
	return len(d) >= 7 && len(d) <= 15
}

// Email normalizes an e-mail address (trimmed, lower case).
func Email(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

// --- masking ------------------------------------------------------------------

// Identity kinds of MaskIdentity.
const (
	KindTCKN     = "tckn"
	KindVKN      = "vkn"
	KindPassport = "passport"
	KindLicence  = "licence"
)

// MaskIdentity masks an identity or licence number by kind:
//
//	tckn     12345678901 → 123******01
//	vkn      1234567890  → 123*****90
//	passport U12345678   → U******78
//	licence  123456      → ***456
func MaskIdentity(kind, v string) string {
	if v == "" {
		return ""
	}
	r := []rune(v)
	switch kind {
	case KindTCKN, KindVKN:
		return keep(r, 3, 2)
	case KindPassport:
		return keep(r, 1, 2)
	default:
		return keep(r, 0, 3)
	}
}

// keep shows the first head and the last tail runes and stars the rest. A
// value too short to hide anything is fully starred.
func keep(r []rune, head, tail int) string {
	if len(r) <= head+tail+1 {
		if len(r) <= 2 {
			return strings.Repeat("*", len(r))
		}
		head, tail = 0, 1
	}
	return string(r[:head]) + strings.Repeat("*", len(r)-head-tail) + string(r[len(r)-tail:])
}

// MaskPhone masks a normalized phone number, keeping the country code, the
// first national digit (operator hint) and the last two digits:
//
//	+905321234512 → +90 5** *** ** 12
//	+4915112345678 → +49 1** ***** 78
func MaskPhone(normalized string) string {
	if normalized == "" {
		return ""
	}
	d := Digits(normalized)
	if strings.HasPrefix(d, "90") && len(d) == 12 {
		return "+90 " + d[2:3] + "** *** ** " + d[10:]
	}
	if len(d) < 6 {
		return strings.Repeat("*", len(d))
	}
	cc := d[:2]
	rest := d[2:]
	return "+" + cc + " " + rest[:1] + "** " + strings.Repeat("*", len(rest)-5) + " " + rest[len(rest)-2:]
}

// MaskEmail masks an e-mail address: the first letter of the local part and
// of the domain stay, the top-level domain stays.
//
//	ayse.yilmaz@domain.com → a***@d***.com
func MaskEmail(v string) string {
	if v == "" {
		return ""
	}
	local, domain, ok := strings.Cut(v, "@")
	if !ok || local == "" || domain == "" {
		return "***"
	}
	tld := ""
	if i := strings.LastIndex(domain, "."); i > 0 {
		tld = domain[i:]
		domain = domain[:i]
	}
	return firstRune(local) + "***@" + firstRune(domain) + "***" + tld
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

// LastDigits returns the last n digits of v (search keywords: the counter
// finds a customer by the end of a phone number without the full number
// being indexed).
func LastDigits(v string, n int) string {
	d := Digits(v)
	if len(d) <= n {
		return d
	}
	return d[len(d)-n:]
}
