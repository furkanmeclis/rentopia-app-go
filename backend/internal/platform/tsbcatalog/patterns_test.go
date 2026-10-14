package tsbcatalog

import "testing"

func TestPatternModel(t *testing.T) {
	t.Parallel()
	cases := []struct{ brand, name, want string }{
		// BMW engine designations -> series
		{"BMW", "320d SEDAN 2.0 190 M SPORT", "3 Serisi"},
		{"BMW", "520i 1.6 170 LUXURY LINE", "5 Serisi"},
		{"BMW", "M340i XDRIVE 3.0", "3 Serisi"},
		{"BMW", "730 Ld XDRIVE", "7 Serisi"},
		{"BMW", "118iA 1.5 136 JOY", "1 Serisi"},
		{"BMW", "M760Li XDRIVE", "7 Serisi"},
		{"BMW", "M3 COMPETITION 3.0", "M3"},
		{"BMW", "X3 LCI XDRIVE20d", "X3"},
		{"BMW", "i3s LODGE 120AH", "i3"},
		{"BMW", "iX3 IMPRESSIVE", "iX3"},
		{"BMW", "ALPINA B3 BITURBO", "Alpina B3"},
		{"BMW", "Z4 SDRIVE20i", "Z4"},
		// Mercedes vans written without spaces, classes with engine codes
		{"MERCEDES", "SPRINTERKAMYONETORTA 314 CDI", "Sprinter"},
		{"MERCEDES", "SPRINTER EKSTRAUZUNSERVIS 516 CDI", "Sprinter"},
		{"MERCEDES", "eSPRINTER 312", "eSprinter"},
		{"MERCEDES", "VITO MIXTO 116 CDI", "Vito"},
		{"MERCEDES", "V KLASSE 250 D", "V"},
		{"MERCEDES", "A180 1.5 STYLE", "A"},
		{"MERCEDES", "C180K ELEGANCE", "C"},
		{"MERCEDES", "S400d 4MATIC", "S"},
		{"MERCEDES", "MERCEDES-AMG C 63 S", "C"},
		{"MERCEDES", "MERCEDES-AMG GT 63", "AMG GT"},
		{"MERCEDES", "KAMYON AROCS 3342 K", "Arocs"},
		{"MERCEDES", "CEKICI eACTROS 600", "eActros"},
		// Ford vans: glued and abbreviated
		{"FORD", "TRANSITCUSTOMKOMBIVAN ECOBLUE 170 320S DELUXE", "Transit Custom"},
		{"FORD", "TOURNEOCOURIERJOURNEY1.5TDCI100 E6.2 TIT.PLUS", "Tourneo Courier"},
		{"FORD", "T.CONNECT KOMBI K210S 1.8 TDCi (90)", "Transit Connect"},
		{"FORD", "TRAN.MCAI 350LFVAN 11.5M3UPGRADE 170 TREND AT", "Transit"},
		{"FORD", "TR.350L MCAI K.NET CIFTK.UPG.170 TREND KASALI", "Transit"},
		{"FORD", "E-TRANSIT CUSTOM 320L", "E-Transit Custom"},
		{"FORD /USA", "MUSTANG 5.0 GT", ""}, // no patterns for FORD /USA
		// Engine-size-first passenger types
		{"RANGE ROVER", "2.0 PHEV404 VOGUE LWB", "Range Rover"},
		{"RANGE ROVER", "3.0 D300 HSE", "Range Rover"},
		{"RANGE ROVER", "SPORT 3.0 D300", ""},
		{"FIAT", "EGEA CROSS 1.4", ""},
	}
	for _, c := range cases {
		if got := PatternModel(c.brand, c.name); got != c.want {
			t.Errorf("PatternModel(%q, %q) = %q, want %q", c.brand, c.name, got, c.want)
		}
	}
}

func TestResolverAppliesBrandPatterns(t *testing.T) {
	t.Parallel()
	names := []string{"320d SEDAN 2.0", "330i 2.0 M SPORT", "X3 XDRIVE20d", "M3 COMPETITION 3.0"}
	r := NewResolver(nil, nil, true).WithBrand("BMW")
	r.Prime(names)
	want := []string{"3 Serisi", "3 Serisi", "X3", "M3"}
	for i, n := range names {
		if got := r.Resolve(n); got.Name != want[i] || got.Source != "derived" {
			t.Errorf("Resolve(%q) = %+v, want %q", n, got, want[i])
		}
	}
	// An existing model of the same name (any case) is reused.
	k := NewResolver(nil, []ModelRef{{ID: 4, Name: "3 SERISI"}}, true).WithBrand("BMW")
	if got := k.Resolve("318i 1.5"); got.ModelID != 4 || got.Source != "model" {
		t.Fatalf("known series model: %+v", got)
	}
	// Rules still win over patterns.
	rr := NewResolver([]RuleRef{{Prefix: "320d", ModelID: 9}}, nil, true).WithBrand("BMW")
	if got := rr.Resolve("320d SEDAN"); got.ModelID != 9 || got.Source != "rule" {
		t.Fatalf("rule: %+v", got)
	}
}

func TestResolverSplitsGluedNames(t *testing.T) {
	t.Parallel()
	names := []string{
		"C5 AIRCROSS FEEL 1.5 BLUEHDI",
		"C5AIRCROSSSHINEBOLD 1.2 PURETECH",
		"C5AIRCROSS FEELADVENTURE 1.5",
		"JEEP GRAND CHEROKEE 3.0",
		"JEEPGRANDCHEROKEE SUMMIT 3.0",
		"CX-3 2.0",
		"CX-30 2.0",
	}
	r := NewResolver(nil, nil, true)
	r.Prime(names)
	want := map[string]string{
		"C5AIRCROSSSHINEBOLD 1.2 PURETECH": "C5 AIRCROSS SHINEBOLD",
		"C5AIRCROSS FEELADVENTURE 1.5":     "C5 AIRCROSS FEELADVENTURE",
		"JEEPGRANDCHEROKEE SUMMIT 3.0":     "JEEP GRAND CHEROKEE",
		"CX-30 2.0":                        "CX-30",
	}
	for n, w := range want {
		if got := r.Resolve(n); got.Name != w {
			t.Errorf("Resolve(%q) = %+v, want %q", n, got, w)
		}
	}
	// Without a spaced spelling to learn from, nothing is split.
	k := NewResolver(nil, nil, true)
	k.Prime([]string{"TIGUANALLSPACE 1.5"})
	if got := k.Resolve("TIGUANALLSPACE 1.5"); got.Name != "TIGUANALLSPACE" {
		t.Fatalf("lone glued name: %+v", got)
	}
}

func TestSlug(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Fiat":          "fiat",
		"Lynk & Co":     "lynk-co",
		"Güleryüz":      "guleryuz",
		"Zirai Traktör": "zirai-traktor",
		"Rolls-Royce":   "rolls-royce",
		"Habaş":         "habas",
		"  ":            "brand",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
