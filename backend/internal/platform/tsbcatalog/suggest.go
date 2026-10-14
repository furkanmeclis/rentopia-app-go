package tsbcatalog

import (
	"regexp"
	"strings"
)

// Transmission / fuel suggestion values (match the catalog_types CHECKs).
const (
	TransmissionManual    = "manual"
	TransmissionAutomatic = "automatic"

	FuelPetrol   = "petrol"
	FuelDiesel   = "diesel"
	FuelHybrid   = "hybrid"
	FuelElectric = "electric"
	FuelLPG      = "lpg"
)

func set(words ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[w] = struct{}{}
	}
	return m
}

var (
	automaticTokens = set(
		"DCT", "AT", "EDC", "X-TRONIC", "XTRONIC", "CVT", "E-CVT", "TCT", "DSG",
		"S-TRONIC", "STRONIC", "STEPTRONIC", "AUTO", "OTOMATIK", "EAT6", "EAT8",
		"TIPTRONIC", "TIPT", "TIPT.DSG", "POWERSHIFT", "AMT", "OV", "MULTIDRIVE",
		"ETG5", "ETG6", "A/T", "OTOM", "OTOM.", "AUTOMATIC", "MULTITRONIC", "PDK",
	)
	// Suffixes like 7G-DCT, 9G-TRONIC, 10AT, 8AT.
	automaticSuffixRe = regexp.MustCompile(`^(\d{1,2}G-(DCT|TRONIC)|\d{1,2}AT|\d{1,2}-AT|\d{1,2}DCT)$`)
	manualTokens      = set("MT", "M/T", "MANUEL", "MANUAL", "6MT", "5MT", "6M/T", "5M/T")

	dieselTokens = set(
		"M.JET", "MJET", "MULTIJET", "TDI", "DCI", "BLUEDCI", "CRDI", "HDI", "BLUEHDI",
		"E-HDI", "CDI", "DIZEL", "DIESEL", "TDCI", "ECOBLUE", "JTD", "JTDM", "D-4D",
		"SDV6", "TDV6", "CRDI4", "I-DTEC", "DTEC", "D", "BLUETEC", "MULTIJET2",
	)
	// 320d, 520d, XDRIVE20d, 2.2D, 1.5D, 180d: a lower-case d after digits
	// (BMW / Mercedes) or an upper-case D right after an engine size.
	dieselSuffixRe = regexp.MustCompile(`(^|[A-Za-z])\d{2,3}d$|^\d[.,]\d{1,2}D$`)
	hybridTokens   = set("HIBRIT", "HYBRID", "HEV", "PHEV", "MHEV", "E-HYBRID", "HYBRID4", "FULLHYBRID", "PLUG-IN")
	electricTokens = set("ELEKTRIK", "ELEKTRIKLI", "EV", "ELECTRIC", "BEV", "E-TRON", "MENZIL")
	// "E-" prefixed tokens that are not electric drivetrains.
	notElectricE = set("E-CVT", "E-HDI", "E-HYBRID", "E-TSI", "E-THP", "E-TENSE", "E-PACE", "E-CLASS", "E-KLASSE", "E-SERIES")
	evModelRe    = regexp.MustCompile(`^EV\d$`)
	lpgTokens    = set("LPG", "BENZIN/LPG", "LPG'LI")
	petrolTokens = set(
		"TSI", "TFSI", "TCE", "PURETECH", "FIRE", "ECOBOOST", "MPI", "GDI", "T-GDI",
		"CVVT", "D-CVVT", "VVT-I", "VTEC", "I-VTEC", "TBI", "TB", "THP", "MULTIAIR",
		"BENZINLI", "BENZIN", "T-JET", "TJET", "SCE", "VVT", "TGDI", "ETSI", "VALVEMATIC",
		"SKYACTIV-G", "TURBO", "T4",
	)
	engineSizeRe = regexp.MustCompile(`^\d[.,]\d{1,2}`)
	// BMW-style designations: 320i, 520Li, M340i, 118iA (petrol); 330e, 745Le (plug-in).
	petrolSuffixRe = regexp.MustCompile(`^M?\d{3}L?iA?$`)
	hybridSuffixRe = regexp.MustCompile(`^\d{3}L?e$`)
)

func tokenize(name string) []string {
	return strings.Fields(name)
}

func upper(s string) string {
	return strings.ToUpper(foldUpperTurkish(s))
}

var upperTurkish = strings.NewReplacer(
	"ı", "I", "İ", "I", "ş", "S", "Ş", "S", "ğ", "G", "Ğ", "G",
	"ü", "U", "Ü", "U", "ö", "O", "Ö", "O", "ç", "C", "Ç", "C",
)

func foldUpperTurkish(s string) string {
	return upperTurkish.Replace(s)
}

// cleanToken strips wrapping punctuation: "(Y)" -> "Y", "DCT," -> "DCT".
func cleanToken(t string) string {
	return strings.Trim(t, "(),;")
}

// SuggestTransmission returns "automatic" when a gearbox token says so (or
// the type is electric), "manual" for an explicit manual token, and ""
// otherwise. TSB names usually omit the gearbox of manual cars, but also of
// many automatic ones, so no default is guessed.
func SuggestTransmission(name string) string {
	for _, raw := range tokenize(name) {
		t := upper(cleanToken(raw))
		if _, ok := automaticTokens[t]; ok {
			return TransmissionAutomatic
		}
		if automaticSuffixRe.MatchString(t) || strings.HasSuffix(t, "-TRONIC") || strings.HasSuffix(t, ".DSG") {
			return TransmissionAutomatic
		}
		if _, ok := manualTokens[t]; ok {
			return TransmissionManual
		}
	}
	if SuggestFuel(name) == FuelElectric {
		return TransmissionAutomatic
	}
	return ""
}

// SuggestFuel guesses the fuel from type-name tokens. Precedence: hybrid
// (a plug-in name often also says "electric"), electric, LPG, diesel, petrol.
// Returns "" when unknown.
func SuggestFuel(name string) string {
	var electric, hybrid, lpg, diesel, petrol bool
	for _, raw := range tokenize(name) {
		clean := cleanToken(raw)
		t := upper(clean)
		if _, ok := electricTokens[t]; ok {
			electric = true
		}
		if evModelRe.MatchString(t) {
			electric = true
		}
		if strings.HasPrefix(t, "E-") && len(t) > 2 {
			if _, skip := notElectricE[t]; !skip {
				electric = true
			}
		}
		if _, ok := hybridTokens[t]; ok {
			hybrid = true
		}
		if _, ok := lpgTokens[t]; ok {
			lpg = true
		}
		if _, ok := dieselTokens[t]; ok {
			diesel = true
		}
		if dieselSuffixRe.MatchString(clean) {
			diesel = true
		}
		if petrolSuffixRe.MatchString(clean) {
			petrol = true
		}
		if hybridSuffixRe.MatchString(clean) {
			hybrid = true
		}
		if _, ok := petrolTokens[t]; ok {
			petrol = true
		}
	}
	switch {
	case hybrid:
		return FuelHybrid
	case electric:
		return FuelElectric
	case lpg:
		return FuelLPG
	case diesel:
		return FuelDiesel
	case petrol:
		return FuelPetrol
	}
	return ""
}
