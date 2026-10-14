package tsbcatalog

import (
	"regexp"
	"strings"
)

// modelPattern maps a type-name pattern of one TSB brand to a model name.
// Patterns run on the normalized (upper-case, single-spaced) type name,
// after admin prefix rules and before the generic derivation, so they fix
// TSB spellings the heuristic gets wrong:
//
//   - BMW engine designations ("320d", "M340i", "730 Ld") -> "3 Serisi"
//   - van names written without spaces or abbreviated
//     ("TRANSITCUSTOMKOMBIVAN", "TRAN.MCAI", "SPRINTERKAMYONETORTA")
//   - types that start with an engine size ("2.0 PHEV404 VOGUE" under the
//     RANGE ROVER brand is the Range Rover itself)
type modelPattern struct {
	re   *regexp.Regexp
	name string                // template; $1 … refer to submatches
	fn   func([]string) string // optional; wins over name
}

func pat(expr, name string) modelPattern {
	return modelPattern{re: regexp.MustCompile(expr), name: name}
}

func patFn(expr string, fn func([]string) string) modelPattern {
	return modelPattern{re: regexp.MustCompile(expr), fn: fn}
}

// end is a token boundary in a normalized name.
const end = `(?:\s|$)`

// brandPatterns are keyed by the TSB brand name (as DefaultsForBrand keys
// them). Order matters: the first match wins.
var brandPatterns = map[string][]modelPattern{
	"BMW": {
		// 116d, 320i, 520Li, 730 Ld, 740e, M340i, M760Li, 128Ti, 120
		pat(`^M?([1-8])\d{2}(?:\s?[A-Z]{1,3})?`+end, "$1 Serisi"),
		pat(`^ALPIN[AE]\s+([A-Z]\d+)`+end, "Alpina $1"),
		pat(`^I3S?`+end, "i3"),
		patFn(`^(I[X]?\d?)`+end, func(m []string) string {
			// I4, I7, IX, IX1, IX3 -> i4, i7, iX, iX1, iX3
			return "i" + m[1][1:]
		}),
		pat(`^XM`+end, "XM"),
		// M3 COMPETITION, M8 GRAN COUPE, X3 LCI: trims after the model.
		pat(`^(M[2-8]|X[1-7]|Z[348])`+end, "$1"),
	},
	"MERCEDES": {
		pat(`^E-?SPRINTER`, "eSprinter"),
		pat(`^SPR?INTER`, "Sprinter"),
		pat(`^VITO`, "Vito"),
		// Trucks and buses are listed as "KAMYON AROCS …", "OTOBUS TRAVEGO …".
		patFn(`^(?:CEKICI|KAMYON|OTOBUS)\s+(E?ACTROS|ANTOS|AROCS|AXOR|ATEGO|UNIMOG|CONECTO|INTOURO|TRAVEGO|TOURISMO)`+end,
			func(m []string) string {
				if m[1] == "EACTROS" {
					return "eActros"
				}
				return TitleCase(m[1])
			}),
		pat(`^V\s?(?:KLASSE|CLASSE|SINIFI|\d{3})`, "V"),
		pat(`^MERCEDES-?AMG\s+GT`+end, "AMG GT"),
		pat(`^MERCEDES-?(?:AMG|MAYBACH)\s+(A|B|C|E|G|S|CL[AEKS]?|EQ[ABCES]|GL[ABCEKS]?|ML|SLC?|SLK)`+end, "$1"),
		// A180, C180K, E220, G63, S400d, ML63 -> A, C, E, G, S, ML
		pat(`^(A|B|C|E|G|S|CL[AEKS]?|GL[ABCEKS]?|ML|R|SL[CKS]?)\s?\d{2,3}[A-Z]{0,4}`+end, "$1"),
	},
	"FORD": {
		pat(`^E-TOURNEO\s?CUSTOM`, "E-Tourneo Custom"),
		pat(`^E-TRANSIT\s?CUSTOM`, "E-Transit Custom"),
		pat(`^E-TRANSIT`+end, "E-Transit"),
		pat(`^TOURNEO\s?CUSTOM`, "Tourneo Custom"),
		pat(`^TOURNEO\s?CONNECT`, "Tourneo Connect"),
		pat(`^TOURNEO\s?COURIER`, "Tourneo Courier"),
		pat(`^(?:TRANSIT|T\.)\s?CUSTOM`, "Transit Custom"),
		pat(`^(?:TRANSIT|T\.)\s?CONNECT`, "Transit Connect"),
		pat(`^TRANSIT\s?COURIER`, "Transit Courier"),
		pat(`^(?:TRANSIT|TRANS\.|TRAN\.|TRA\.|TR\.)`, "Transit"),
		pat(`^(?:CEKICI\s+)?F-MAX`, "F-Max"),
		pat(`^(?:CEKICI|KAMYON)\s+CARGO`+end, "Cargo"),
	},
	"RANGE ROVER": {
		// The brand's flagship is listed by engine only ("3.0 D300 HSE").
		pat(`^\d[.,]\d`, "Range Rover"),
	},
}

// PatternModel returns the model a brand pattern assigns to typeName, or ""
// when no pattern of the brand matches.
func PatternModel(tsbBrandName, typeName string) string {
	return matchPatterns(brandPatterns[brandKey(tsbBrandName)], normalize(typeName))
}

func brandKey(tsbName string) string {
	return strings.Join(strings.Fields(upper(tsbName)), " ")
}

func matchPatterns(ps []modelPattern, n string) string {
	for _, p := range ps {
		idx := p.re.FindStringSubmatchIndex(n)
		if idx == nil {
			continue
		}
		if p.fn != nil {
			groups := make([]string, 0, len(idx)/2)
			for i := 0; i+1 < len(idx); i += 2 {
				if idx[i] < 0 {
					groups = append(groups, "")
					continue
				}
				groups = append(groups, n[idx[i]:idx[i+1]])
			}
			return p.fn(groups)
		}
		return string(p.re.ExpandString(nil, p.name, n, idx))
	}
	return ""
}

// minGlueBase is the shortest model name used to split glued TSB names
// ("TOURNEOCOURIERKOMBI" -> "TOURNEO COURIER KOMBI").
const minGlueBase = 4

// glueIndex splits glued first tokens on known model names.
type glueIndex struct {
	// bases maps a model name without spaces to its spaced form.
	bases map[string]string
}

func newGlueIndex(names []string) *glueIndex {
	g := &glueIndex{bases: map[string]string{}}
	for _, n := range names {
		n = normalize(n)
		key := strings.ReplaceAll(n, " ", "")
		if len(key) < minGlueBase {
			continue
		}
		// Prefer the spaced spelling ("C5 AIRCROSS" over "C5AIRCROSS").
		if old, ok := g.bases[key]; !ok || strings.Count(n, " ") > strings.Count(old, " ") {
			g.bases[key] = n
		}
	}
	// Drop bases that are glued themselves ("TRANSITCUSTOMKOMBIVAN" when
	// "TRANSIT" is known): they would keep their own types glued.
	var glued []string
	for key, spaced := range g.bases {
		first := strings.SplitN(spaced, " ", 2)[0]
		if _, _, ok := g.split(first, key); ok {
			glued = append(glued, key)
			continue
		}
		// "C5AIRCROSS FEELADVENTURE" when "C5 AIRCROSS" is known.
		if other, ok := g.bases[first]; ok && other != first && first != key {
			glued = append(glued, key)
		}
	}
	for _, key := range glued {
		delete(g.bases, key)
	}
	return g
}

// split finds the longest base that is a proper prefix of token, followed
// by at least two letters ("CX-30" is not "CX-3" + "0").
func (g *glueIndex) split(token, skip string) (string, string, bool) {
	best := ""
	for key := range g.bases {
		if key == skip || len(key) <= len(best) || len(key) >= len(token) || !strings.HasPrefix(token, key) {
			continue
		}
		rest := token[len(key):]
		if len(rest) < 2 || !isASCIILetter(rest[0]) || !isASCIILetter(rest[1]) {
			continue
		}
		last := key[len(key)-1]
		if !isASCIILetter(last) && !isASCIIDigit(last) {
			continue
		}
		best = key
	}
	if best == "" {
		return "", "", false
	}
	return g.bases[best], token[len(best):], true
}

// unglue rewrites a normalized name whose first token is glued.
func (g *glueIndex) unglue(n string) string {
	if g == nil || len(g.bases) == 0 {
		return n
	}
	parts := strings.SplitN(n, " ", 2)
	var out string
	if spaced, known := g.bases[parts[0]]; known {
		if spaced == parts[0] {
			return n
		}
		out = spaced // "JEEPGRANDCHEROKEE" -> "JEEP GRAND CHEROKEE"
	} else {
		base, rest, ok := g.split(parts[0], "")
		if !ok {
			return n
		}
		out = base + " " + rest
	}
	if len(parts) == 2 {
		out += " " + parts[1]
	}
	return out
}

func isASCIILetter(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }
func isASCIIDigit(b byte) bool  { return b >= '0' && b <= '9' }
