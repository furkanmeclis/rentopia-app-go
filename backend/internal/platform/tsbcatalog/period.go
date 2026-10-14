// Package tsbcatalog reads the monthly TSB (Türkiye Sigorta Birliği) kasko
// value list and derives catalog hints (model, gearbox, fuel) from the free
// text type names. It has no database dependencies.
package tsbcatalog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var turkishMonths = map[string]int{
	"ocak": 1, "subat": 2, "mart": 3, "nisan": 4, "mayis": 5, "haziran": 6,
	"temmuz": 7, "agustos": 8, "eylul": 9, "ekim": 10, "kasim": 11, "aralik": 12,
}

var (
	periodLabelRe = regexp.MustCompile(`^\s*([^\d\s]+)\s+(\d{4})\s*$`)
	periodISORe   = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
)

var lowerTurkish = strings.NewReplacer(
	"İ", "i", "I", "i", "ı", "i",
	"Ş", "s", "ş", "s",
	"Ğ", "g", "ğ", "g",
	"Ü", "u", "ü", "u",
	"Ö", "o", "ö", "o",
	"Ç", "c", "ç", "c",
)

// foldTurkish lower-cases s with Turkish casing rules and strips diacritics so
// "EYLÜL", "Eylül" and "EYLUL" compare equal.
func foldTurkish(s string) string {
	return strings.ToLower(lowerTurkish.Replace(s))
}

// ParsePeriod converts a TSB period label such as "Ekim 2026" into "2026-10".
// An already normalized "YYYY-MM" value is accepted as is.
func ParsePeriod(label string) (string, error) {
	label = strings.TrimSpace(label)
	if periodISORe.MatchString(label) {
		return label, nil
	}
	m := periodLabelRe.FindStringSubmatch(label)
	if m == nil {
		return "", fmt.Errorf("tsbcatalog: unrecognized period label %q", label)
	}
	month, ok := turkishMonths[foldTurkish(m[1])]
	if !ok {
		return "", fmt.Errorf("tsbcatalog: unknown month %q", m[1])
	}
	year, err := strconv.Atoi(m[2])
	if err != nil || year < 2000 || year > 2100 {
		return "", fmt.Errorf("tsbcatalog: invalid year in %q", label)
	}
	return fmt.Sprintf("%04d-%02d", year, month), nil
}

// ValidPeriod reports whether p is a "YYYY-MM" period.
func ValidPeriod(p string) bool {
	return periodISORe.MatchString(p)
}
