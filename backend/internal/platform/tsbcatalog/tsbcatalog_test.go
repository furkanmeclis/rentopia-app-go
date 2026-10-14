package tsbcatalog

import (
	"bytes"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParsePeriod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"Ekim 2026", "2026-10", false},
		{"EKİM 2026", "2026-10", false},
		{"Şubat 2027", "2027-02", false},
		{"SUBAT 2027", "2027-02", false},
		{"Ağustos 2025", "2025-08", false},
		{"Aralık 2026", "2026-12", false},
		{"ARALIK 2026", "2026-12", false},
		{"  Ocak   2027 ", "2027-01", false},
		{"2026-10", "2026-10", false},
		{"October 2026", "", true},
		{"Ekim", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		got, err := ParsePeriod(c.in)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("ParsePeriod(%q) = %q, %v; want %q (err=%v)", c.in, got, err, c.want, c.wantErr)
		}
	}
}

func TestHeuristics(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		model        string
		fuel         string
		transmission string
	}{
		{"EGEA CROSS STREET 1.4 FIRE 95 E6D", "EGEA CROSS", FuelPetrol, ""},
		{"EGEA CROSS LOUNGE 1.6 M.JET 130 DCT", "EGEA CROSS", FuelDiesel, TransmissionAutomatic},
		{"CLIO EVOLUTION 1.0 TCE X-TRONIC 90", "CLIO", FuelPetrol, TransmissionAutomatic},
		{"EGEA CROSS STREET 1.5 T4 HIBRIT 130 AT", "EGEA CROSS", FuelHybrid, TransmissionAutomatic},
		{"EGEA SEDAN LIMITED 1.4 FIRE 95 GSR", "EGEA", FuelPetrol, ""},
		{"CLIO HB ICON 1.5 DCI EDC 90", "CLIO", FuelDiesel, TransmissionAutomatic},
		{"MEGANE SEDAN TOUCH 1.5 BLUEDCI EDC 115 FAZ2", "MEGANE", FuelDiesel, TransmissionAutomatic},
		{"GOLF VII 1.2 TSI BMT 105 COMFORTLINE TIPT.DSG", "GOLF", FuelPetrol, TransmissionAutomatic},
		{"C-HR 1.8 HYBRID DYNAMIC NAVI E-CVT", "C-HR", FuelHybrid, TransmissionAutomatic},
		{"MODEL Y PERFORMANCE FL", "MODEL Y", "", ""},
		{"IONIQ 5 PROGRESSIVE 77.4 KWH", "IONIQ 5", "", ""},
		{"KONA ELEKTRIK ADVANCE 99 KW", "KONA", FuelElectric, TransmissionAutomatic},
		{"e-2008 GT", "e-2008", FuelElectric, TransmissionAutomatic},
		{"208 5 KAPI ACCESS 1.4 HDI (68)", "208", FuelDiesel, ""},
		{"A 180 d SEDAN 1.5 116 7G-DCT STYLE", "A", FuelDiesel, TransmissionAutomatic},
		{"E 350 d 3.0 AMG 9G-TRONIC", "E", FuelDiesel, TransmissionAutomatic},
		{"X3 XDRIVE20d 2.0 (190) PRESTIGE", "X3", FuelDiesel, ""},
		{"320i SEDAN 1.6 170 M SPORT", "320i", FuelPetrol, ""},
		{"LAND CRUISER PRADO 2.7", "LAND CRUISER PRADO", "", ""},
		{"PROACE MAX 2.2D 140 15M3 4.25T M/T", "PROACE MAX", FuelDiesel, TransmissionManual},
		{"ACCENT BLUE 1.4 CVVT MODE OV", "ACCENT BLUE", FuelPetrol, TransmissionAutomatic},
		{"DUSTER COMFORT 1.0 ECO-G 100 LPG", "DUSTER", FuelLPG, ""},
		{"308 STYLE 1.2 PURETECH 130 E6D", "308", FuelPetrol, ""},
		{"3008 ALLURE 1.6 E-HDI 115 ETG6", "3008", FuelDiesel, TransmissionAutomatic},
		{"1.6 16V", "", "", ""},
	}
	for _, c := range cases {
		if got := DeriveModel(c.name); got != c.model {
			t.Errorf("DeriveModel(%q) = %q, want %q", c.name, got, c.model)
		}
		if got := SuggestFuel(c.name); got != c.fuel {
			t.Errorf("SuggestFuel(%q) = %q, want %q", c.name, got, c.fuel)
		}
		if got := SuggestTransmission(c.name); got != c.transmission {
			t.Errorf("SuggestTransmission(%q) = %q, want %q", c.name, got, c.transmission)
		}
	}
}

func TestHasPrefixToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, prefix string
		want         bool
	}{
		{"EGEA CROSS STREET", "EGEA CROSS", true},
		{"EGEA CROSS STREET", "egea", true},
		{"EGEAX 1.4", "EGEA", false},
		{"EGEA", "EGEA", true},
		{"TRAN.MCAI 350ED", "TRAN.", true},
		{"EGEA  CROSS  STREET", "EGEA CROSS", true},
		{"EGEA", "", false},
	}
	for _, c := range cases {
		if got := HasPrefixToken(c.name, c.prefix); got != c.want {
			t.Errorf("HasPrefixToken(%q, %q) = %v, want %v", c.name, c.prefix, got, c.want)
		}
	}
}

func TestResolverPrefersLongestKnownModel(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil, []ModelRef{{ID: 1, Name: "EGEA"}, {ID: 2, Name: "Egea Cross"}}, true)
	r.Prime([]string{"EGEA CROSS LOUNGE 1.6 M.JET 130 DCT", "EGEA SEDAN URBAN 1.4 FIRE 95"})
	if m := r.Resolve("EGEA CROSS LOUNGE 1.6 M.JET 130 DCT"); m.ModelID != 2 || m.Source != "model" {
		t.Fatalf("cross: %+v", m)
	}
	if m := r.Resolve("EGEA SEDAN URBAN 1.4 FIRE 95"); m.ModelID != 1 {
		t.Fatalf("sedan: %+v", m)
	}
}

func TestResolverDerivesMultiWordModelIndependentOfOrder(t *testing.T) {
	t.Parallel()
	names := []string{
		"EGEA SEDAN LIMITED 1.4 FIRE 95",
		"EGEA CROSS STREET 1.4 FIRE 95 E6D",
		"EGEA HB LOUNGE 1.3 M.JET 95 E6D",
		"EGEA CROSS STREET 1.5 T4 HIBRIT 130 AT",
	}
	r := NewResolver(nil, []ModelRef{{ID: 1, Name: "EGEA"}}, true)
	r.Prime(names)
	want := []Match{
		{ModelID: 1, Name: "EGEA", Source: "model"},
		{Name: "EGEA CROSS", Source: "derived"},
		{ModelID: 1, Name: "EGEA", Source: "model"},
		{Name: "EGEA CROSS", Source: "derived"},
	}
	for i, n := range names {
		if got := r.Resolve(n); got != want[i] {
			t.Errorf("Resolve(%q) = %+v, want %+v", n, got, want[i])
		}
	}
}

func TestResolverRulesWinAndTrimsAreDropped(t *testing.T) {
	t.Parallel()
	r := NewResolver(
		[]RuleRef{{Prefix: "EGEA CROSS", ModelID: 7}, {Prefix: "EGEA CROSS STREET", ModelID: 9}},
		[]ModelRef{{ID: 1, Name: "EGEA"}},
		true,
	)
	if m := r.Resolve("EGEA CROSS STREET 1.4 FIRE 95 E6D"); m.ModelID != 9 || m.Source != "rule" {
		t.Fatalf("longest rule should win: %+v", m)
	}
	if m := r.Resolve("EGEA CROSS LOUNGE 1.6"); m.ModelID != 7 {
		t.Fatalf("shorter rule: %+v", m)
	}

	// "COOL" follows four different heads: it is a trim, not a model word.
	kia := []string{"CEED COOL 1.6", "RIO COOL 1.4", "PICANTO COOL 1.0", "STONIC COOL 1.4", "PRO CEED 1.6"}
	k := NewResolver(nil, nil, true)
	k.Prime(kia)
	if m := k.Resolve("RIO COOL 1.4"); m.Name != "RIO" {
		t.Fatalf("trim kept: %+v", m)
	}
	if m := k.Resolve("PRO CEED 1.6"); m.Name != "PRO CEED" {
		t.Fatalf("pro ceed: %+v", m)
	}
}

func TestResolverWithoutDerivationLeavesUnresolved(t *testing.T) {
	t.Parallel()
	r := NewResolver(nil, []ModelRef{{ID: 3, Name: "VESPA"}}, false)
	r.Prime([]string{"KAWASAKI NINJA 300", "VESPA GT 200L"})
	if m := r.Resolve("KAWASAKI NINJA 300"); m.Source != "" || m.Name != "" {
		t.Fatalf("expected unresolved, got %+v", m)
	}
	if m := r.Resolve("VESPA GT 200L"); m.ModelID != 3 {
		t.Fatalf("existing model should still match: %+v", m)
	}
}

func TestDefaultsForBrand(t *testing.T) {
	t.Parallel()
	cases := map[string]BrandDefault{
		"TOFAS-FIAT":        {"Fiat", CategoryPassenger},
		"RENAULT (OYAK)":    {"Renault", CategoryPassenger},
		"FORD /USA":         {"Ford", CategoryPassenger},
		`OTOYOL\IVECO\FIAT`: {"Iveco", CategoryLightCommercial},
		"MOTORSIKLET":       {"Motosiklet", CategoryOther},
		"SCANIA":            {"Scania", CategoryOther},
		"MERCEDES":          {"Mercedes-Benz", CategoryPassenger},
		"KGMOBILITY":        {"KG Mobility (SsangYong)", CategoryPassenger},
		"LYNK-CO":           {"Lynk & Co", CategoryPassenger},
		"TRUMPCHI/GAC":      {"GAC", CategoryPassenger},
		"ALFA ROMEO":        {"Alfa Romeo", CategoryPassenger},
		"ROLLS-ROYCE":       {"Rolls-Royce", CategoryPassenger},
		"LAND ROVER":        {"Land Rover", CategoryPassenger},
	}
	for in, want := range cases {
		if got := DefaultsForBrand(in); got != want {
			t.Errorf("DefaultsForBrand(%q) = %+v, want %+v", in, got, want)
		}
	}
}

// buildWorkbook writes a small synthetic TSB-shaped workbook.
func buildWorkbook(t *testing.T, label string, years []int, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := "Smarka"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		t.Fatal(err)
	}
	header := []any{"Marka Kodu", "Tip Kodu", "Marka Adı", "Tip Adı"}
	for _, y := range years {
		header = append(header, strconv.Itoa(y))
	}
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.SetCellValue(sheet, "A1", label))
	must(f.SetSheetRow(sheet, "A2", &header))
	for i, r := range rows {
		row := r
		cellRef, _ := excelize.CoordinatesToCellName(1, i+3)
		must(f.SetSheetRow(sheet, cellRef, &row))
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseSyntheticWorkbook(t *testing.T) {
	t.Parallel()
	years := []int{2027, 2026, 2025, 2024}
	data := buildWorkbook(t, "Kasım 2026", years, [][]any{
		{"100", "1402", "TOFAS-FIAT", "EGEA CROSS STREET 1.4 FIRE 95 E6D", 0, 0, 1051836, 971010},
		{"100", "1403", "TOFAS-FIAT", "EGEA  CROSS LOUNGE 1.6 M.JET 130 DCT", 0, 1200000, 1100000, 0},
		{"3", "210", "ALFA ROMEO", "MITO 1.4 TB (155) DISTINCTIVE 3 KAPI", 0, 0, 0, 0},
		{"", "", "", "", nil, nil, nil, nil},
	})
	s, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if s.Period != "2026-11" || s.PeriodLabel != "Kasım 2026" {
		t.Fatalf("period: %q %q", s.Period, s.PeriodLabel)
	}
	if !reflect.DeepEqual(s.Years, years) {
		t.Fatalf("years: %v", s.Years)
	}
	if len(s.Rows) != 3 {
		t.Fatalf("rows: %d", len(s.Rows))
	}
	r := s.Rows[0]
	if r.BrandCode != "100" || r.TypeCode != "1402" || r.BrandName != "TOFAS-FIAT" {
		t.Fatalf("row0: %+v", r)
	}
	if !reflect.DeepEqual(r.Values, map[int]int64{2025: 1051836, 2024: 971010}) {
		t.Fatalf("values: %v", r.Values)
	}
	if !reflect.DeepEqual(r.Years(), []int{2025, 2024}) {
		t.Fatalf("row years: %v", r.Years())
	}
	if s.Rows[1].TypeName != "EGEA CROSS LOUNGE 1.6 M.JET 130 DCT" {
		t.Fatalf("whitespace not collapsed: %q", s.Rows[1].TypeName)
	}
	if len(s.Rows[2].Values) != 0 {
		t.Fatalf("all-zero row should have no values: %v", s.Rows[2].Values)
	}
}

func TestParseRejectsDuplicatesAndBadFiles(t *testing.T) {
	t.Parallel()
	dup := buildWorkbook(t, "Ekim 2026", []int{2026}, [][]any{
		{"100", "1402", "TOFAS-FIAT", "EGEA", 1},
		{"100", "1402", "TOFAS-FIAT", "EGEA 2", 1},
	})
	if _, err := Parse(bytes.NewReader(dup)); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := Parse(bytes.NewReader([]byte("not a workbook"))); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("garbage: %v", err)
	}
	noHeader := buildWorkbook(t, "Ekim 2026", nil, nil)
	if _, err := Parse(bytes.NewReader(noHeader)); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("no year columns: %v", err)
	}
	// Unknown period label still parses; the caller supplies the period.
	unknown := buildWorkbook(t, "Kasko Listesi", []int{2026}, [][]any{{"1", "1", "X", "Y", 5}})
	s, err := Parse(bytes.NewReader(unknown))
	if err != nil || s.Period != "" {
		t.Fatalf("unknown label: %v %q", err, s.Period)
	}
}
