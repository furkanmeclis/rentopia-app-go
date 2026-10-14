package tsbcatalog

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// maxModelTokens caps how many leading tokens a derived model may have.
const maxModelTokens = 3

// trimFollowThreshold: a token that follows this many different model heads
// within one brand ("COOL" after CEED, RIO, PICANTO) is a trim, not part of
// a model name.
const trimFollowThreshold = 4

// stopWords end a derived model name: trims, body styles, drivetrains,
// engine technologies, gearboxes and facelift markers. Kept conservative:
// a missed stop word yields an over-long "auto" model the admin can fix,
// a wrong one would merge distinct models.
var stopWords = set(
	// trims
	"STREET", "LOUNGE", "URBAN", "LIMITED", "PREMIO", "EMOTION", "ACTIVE", "ICON", "JOY",
	"TOUCH", "STYLE", "PLUS", "ELITE", "PRIME", "COMFORT", "COMFORTLINE", "TRENDLINE",
	"HIGHLINE", "MIDLINE", "LIFE", "TREND", "TITANIUM", "AMBIENTE", "AMBITION", "ADVANCE",
	"ADVANCED", "DYNAMIC", "DESIGN", "SPORT", "SPORTLINE", "SLINE", "S-LINE", "ST-LINE",
	"M", "AMG", "GT", "GTI", "GT-LINE", "RS", "R-LINE", "N-LINE", "EXCLUSIVE", "PREMIUM",
	"LUXURY", "PRESTIGE", "BUSINESS", "EXECUTIVE", "EDITION", "ESSENTIAL", "EXPRESSION",
	"AUTHENTIQUE", "PRIVILEGE", "ZEN", "INTENS", "INTENSE", "EVOLUTION", "TECHNO", "ALLURE",
	"ACCESS", "FELINE", "SHINE", "FEEL", "LAUREATE", "AMBIANCE", "DREAM", "FLAME", "PASSION",
	"VISION", "CLUB", "BASE", "STANDARD", "STANDART", "PRO", "PURE", "MODE", "SENSE", "JUMP",
	"SELECT", "TEKNA", "ACENTA", "VISIA", "N-CONNECTA", "PLATINUM", "LX", "EX", "SE", "SEL",
	"SX", "GL", "GLS", "GLX", "LS", "LT", "LTZ", "HSE", "XLE", "VTI", "ELEGANCE", "ELEGANT",
	"ESSENTIA", "EASY", "POP", "CULT", "MIRROR", "X-PACK", "X-LINE", "FIRST", "LAUNCH",
	"PERFORMANCE", "LONG", "UZUN", "KISA", "MENZIL", "LEGACY", "V1", "V2", "V3", "SAFELINE",
	"COMFORTLINE", "INSPIRE", "EXCELLENCE", "INDIVIDUAL", "SIGNATURE", "INITIALE", "PARIS",
	"PROGRESSIVE", "CALLIGRAPHY", "MAXX", "OPTIMUM", "NAVI", "SPECIAL", "LUX", "DELUX", "DELUXE", "HIGH", "LOW", "MID",
	// body styles
	"SEDAN", "HB", "HATCHBACK", "STATION", "SW", "WAGON", "ESTATE", "COUPE", "CABRIO",
	"CABRIOLET", "ROADSTER", "CONVERTIBLE", "KOMBI", "COMBI", "VAN", "PANORAMA", "CARGO",
	"KAMYONET", "PICKUP", "PICK-UP", "AVANT", "SPORTBACK", "VARIANT", "TOURER", "3K", "5K",
	"3KAPI", "5KAPI", "KAPI", "MINIBUS", "OTOBUS", "CHASSIS", "SASI", "CIFT", "TEK", "KABIN",
	"FURGON", "PANELVAN", "CAMLI", "LIMOUSINE", "MAXI", "CC", "SPACE", "GRANDTOUR",
	// drivetrains
	"4X4", "4X2", "AWD", "RWD", "FWD", "4MATIC", "4-MATIC", "4MOTION", "QUATTRO", "ALL4",
	"4WD", "2WD", "4MORE",
	// engine technology / fuel
	"TSI", "TFSI", "TDI", "DCI", "BLUEDCI", "TCE", "CDI", "CRDI", "HDI", "BLUEHDI", "E-HDI",
	"JTD", "JTDM", "MJET", "M.JET", "MULTIJET", "FIRE", "PURETECH", "ECOBOOST", "TDCI",
	"ECOBLUE", "MPI", "GDI", "T-GDI", "CVVT", "D-CVVT", "VVT-I", "VTEC", "I-VTEC", "TURBO",
	"TBI", "TB", "THP", "MULTIAIR", "D-4D", "CNG", "LPG", "HYBRID", "HIBRIT", "HEV", "PHEV",
	"MHEV", "ELEKTRIK", "ELECTRIC", "EV", "BENZINLI", "DIZEL", "DIESEL", "BMT", "S&S", "E5",
	"E6", "E6D", "ETSI", "SCE", "T-JET", "BLUE-DCI", "BLUEMOTION", "BLUEEFFICIENCY",
	"BLUETEC", "ECO", "ECO-G",
	// gearboxes
	"DCT", "AT", "MT", "EDC", "CVT", "E-CVT", "X-TRONIC", "XTRONIC", "TCT", "DSG", "S-TRONIC",
	"STRONIC", "STEPTRONIC", "AUTO", "OTOMATIK", "EAT6", "EAT8", "TIPTRONIC", "TIPT",
	"POWERSHIFT", "AMT", "OV", "MULTIDRIVE", "ETG5", "ETG6",
	// common misspellings and commercial variants in TSB names
	"HATCBACK", "ROADSTAR", "ROASTAR", "CABROLET", "LAURATE", "LAURETA", "TITANUM", "PREMUIM",
	"EDITON", "COLLECTOIN", "TORISMO", "TURISMO", "TOURISMO", "TOURING", "GRANCOUPE", "4D",
	"5D", "3D", "ICA", "KASALI", "K.NET", "SERVIS", "OKUL", "TURIZM", "PIKAP", "KAPALI", "ACIK",
	// facelift / generation markers
	"FL", "MCA", "MCAI", "NEW", "YENI", "FACELIFT", "II", "III", "IV", "VI", "VII", "VIII",
)

// stopPrefixes end a model when a token starts with them (XDRIVE20d, SDRIVE18i).
var stopPrefixes = []string{"XDRIVE", "SDRIVE", "EDRIVE"}

// seatWords make a preceding digit a seat/door count rather than a model
// number ("208 5 KAPI", "LODGY 7 KISILIK").
var seatWords = set("KAPI", "KAPILI", "K", "KISILIK", "KOLTUK", "KOLTUKLU", "KISI")

var unitRe = regexp.MustCompile(`^\d+(KW|HP|PS|BG|CC|V|AH|KWH|M3)$`)

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

func isStopToken(raw string) bool {
	t := upper(raw)
	if t == "" {
		return true
	}
	if _, ok := stopWords[t]; ok {
		return true
	}
	for _, p := range stopPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	if engineSizeRe.MatchString(t) || unitRe.MatchString(t) {
		return true
	}
	if strings.ContainsAny(t, "()+/") {
		return true
	}
	// Digit-heavy tokens are power ratings, codes or seat counts (95, 130,
	// 515, 16+1, 4443/45) rather than model names.
	return countDigits(t) >= 2
}

// DeriveModel guesses a model name from a TSB type name: the leading tokens
// before an engine size, a trim / body / drivetrain / engine keyword or a
// digit-heavy token, capped at three tokens. It returns "" when the name
// does not start with a usable token.
func DeriveModel(typeName string) string {
	tokens := tokenize(typeName)
	if len(tokens) == 0 {
		return ""
	}
	first := tokens[0]
	if engineSizeRe.MatchString(first) || strings.HasPrefix(first, "(") {
		return ""
	}
	model := []string{first}
	firstHasDigit := countDigits(first) > 0
	for i := 1; i < len(tokens) && len(model) < maxModelTokens; i++ {
		t := tokens[i]
		if i == 1 && !firstHasDigit && len([]rune(t)) == 1 {
			// "MODEL 3", "MODEL X", "IONIQ 5" — but not "208 5 KAPI".
			next := ""
			if i+1 < len(tokens) {
				next = upper(tokens[i+1])
			}
			if _, seat := seatWords[next]; !seat && (unicode.IsDigit(rune(t[0])) || unicode.IsLetter(rune(t[0]))) {
				model = append(model, t)
				continue
			}
			break
		}
		if len([]rune(t)) == 1 || isStopToken(t) {
			break
		}
		if unicode.IsDigit(rune(t[0])) && countDigits(t) == len(t) {
			break
		}
		model = append(model, t)
	}
	return strings.Join(model, " ")
}

// ModelRef is a known model of a brand.
type ModelRef struct {
	ID   int64
	Name string
}

// RuleRef maps a type-name prefix to a model.
type RuleRef struct {
	Prefix  string
	ModelID int64
}

// Match is the outcome of resolving one type name.
type Match struct {
	// ModelID is set when an existing model (or rule) matched.
	ModelID int64
	// Name is the matched or derived model name ("" = unresolved).
	Name string
	// Source is "rule", "model", "derived" or "" (unresolved).
	Source string
}

// Resolver assigns models to type names of one brand.
type Resolver struct {
	rules  []normRule
	models []normModel
	// derive enables the heuristic for brands where it makes sense.
	derive bool
	// candidates are heuristic model names derived for the batch, so that
	// "EGEA CROSS ..." matches "EGEA CROSS" even if "EGEA" was seen first.
	candidates []normName
}

type normName struct {
	name string // as derived / stored
	norm string // normalized for prefix matching
}

type normRule struct {
	norm    string
	modelID int64
}

type normModel struct {
	normName
	id int64
}

// normalize collapses whitespace and upper-cases (Turkish-aware).
func normalize(s string) string {
	return upper(strings.Join(tokenize(s), " "))
}

// NewResolver builds a resolver. derive=false (e.g. motorcycles, trucks)
// only applies rules and existing models and leaves the rest unresolved.
func NewResolver(rules []RuleRef, models []ModelRef, derive bool) *Resolver {
	r := &Resolver{derive: derive}
	for _, rl := range rules {
		if n := normalize(rl.Prefix); n != "" {
			r.rules = append(r.rules, normRule{norm: n, modelID: rl.ModelID})
		}
	}
	sort.SliceStable(r.rules, func(i, j int) bool { return len(r.rules[i].norm) > len(r.rules[j].norm) })
	for _, m := range models {
		if n := normalize(m.Name); n != "" {
			r.models = append(r.models, normModel{normName: normName{name: m.Name, norm: n}, id: m.ID})
		}
	}
	sort.SliceStable(r.models, func(i, j int) bool { return len(r.models[i].norm) > len(r.models[j].norm) })
	return r
}

// Prime derives candidate model names from a brand's type names. Pass every
// type name of the brand in the file (not only unresolved ones): the more
// names, the better trims are told apart from model words. Call it before
// Resolve.
func (r *Resolver) Prime(typeNames []string) {
	if !r.derive {
		return
	}
	derived := make([][]string, 0, len(typeNames))
	followers := map[string]map[string]struct{}{}
	for _, n := range typeNames {
		toks := tokenize(DeriveModel(n))
		if len(toks) == 0 {
			continue
		}
		derived = append(derived, toks)
		head := strings.TrimPrefix(upper(toks[0]), "E-")
		for _, t := range toks[1:] {
			u := upper(t)
			if followers[u] == nil {
				followers[u] = map[string]struct{}{}
			}
			followers[u][head] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	for _, c := range r.candidates {
		seen[c.norm] = struct{}{}
	}
	for _, toks := range derived {
		keep := toks[:1]
		for _, t := range toks[1:] {
			if len(followers[upper(t)]) >= trimFollowThreshold {
				break
			}
			keep = append(keep, t)
		}
		c := strings.Join(keep, " ")
		n := normalize(c)
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		r.candidates = append(r.candidates, normName{name: c, norm: n})
	}
	sort.SliceStable(r.candidates, func(i, j int) bool { return len(r.candidates[i].norm) > len(r.candidates[j].norm) })
}

// HasPrefixToken reports whether name starts with prefix on a token
// boundary, case-insensitively ("EGEA CROSS" matches "EGEA CROSS STREET"
// but "EGEA" does not match "EGEAX").
func HasPrefixToken(name, prefix string) bool {
	return hasNormPrefix(normalize(name), normalize(prefix))
}

func hasNormPrefix(n, p string) bool {
	if p == "" || !strings.HasPrefix(n, p) {
		return false
	}
	if len(n) == len(p) {
		return true
	}
	last := rune(p[len(p)-1])
	if !unicode.IsLetter(last) && !unicode.IsDigit(last) {
		return true
	}
	return n[len(p)] == ' '
}

// Resolve picks a model for typeName: the longest rule prefix, else the
// longest known model or batch candidate that prefixes the name.
func (r *Resolver) Resolve(typeName string) Match {
	n := normalize(typeName)
	for _, rule := range r.rules {
		if hasNormPrefix(n, rule.norm) {
			return Match{ModelID: rule.modelID, Source: "rule"}
		}
	}
	var best Match
	bestLen := 0
	for _, m := range r.models {
		if hasNormPrefix(n, m.norm) {
			best = Match{ModelID: m.id, Name: m.name, Source: "model"}
			bestLen = len(m.norm)
			break
		}
	}
	for _, c := range r.candidates {
		if len(c.norm) <= bestLen {
			break
		}
		if hasNormPrefix(n, c.norm) {
			// A known model with the same name wins (case-insensitive).
			for _, m := range r.models {
				if m.norm == c.norm {
					return Match{ModelID: m.id, Name: m.name, Source: "model"}
				}
			}
			return Match{Name: c.name, Source: "derived"}
		}
	}
	return best
}
