package ubl

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

func supplier() Party {
	return Party{
		IdentityType: IdentityVKN, Identity: "1234567890", Name: "Test Oto Kiralama A.Ş.", TaxOffice: "Kadıköy",
		Street: "Bağdat Cad. No:1", District: "Kadıköy", City: "İstanbul", PostalCode: "34710", CountryCode: "TR",
	}
}

func lines(t *testing.T, gross ...string) []Line {
	t.Helper()
	g := make([]money.Money, len(gross))
	r := make([]money.Rate, len(gross))
	for i, s := range gross {
		g[i] = money.MustParse(s)
		r[i] = money.Percent(20)
	}
	splits, diff := SplitLines(g, r)
	if !diff.IsZero() {
		t.Fatalf("rounding difference %s", diff)
	}
	out := make([]Line, len(splits))
	for i, s := range splits {
		out[i] = Line{Name: "Kiralama bedeli", Net: s.Net, VAT: s.VAT, Rate: r[i]}
	}
	out[0].Days = 2
	return out
}

func TestBuildAndValidateTemelFatura(t *testing.T) {
	doc := Document{
		UUID: uuid.NewString(), Number: "EFA2026000000001", Profile: ProfileTemel, Type: TypeSatis,
		IssueDate: "2026-10-16", IssueTime: "10:00:00", Currency: "TRY", Supplier: supplier(),
		Customer: Party{
			IdentityType: IdentityVKN, Identity: "9876543210", Name: "Müşteri Lojistik Ltd. Şti.", TaxOffice: "Çankaya",
			Street: "Atatürk Blv. 10", District: "Çankaya", City: "Ankara", CountryCode: "TR",
		},
		Lines: lines(t, "2400.00", "150.00"),
		Notes: []string{"Kiralama IDS-2026-000001"},
	}
	data, totals, problems, err := Validate(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Fatalf("problems: %+v", problems)
	}
	if totals.Payable.String() != "2550.00" {
		t.Fatalf("payable = %s", totals.Payable)
	}
	if !strings.Contains(string(data), "TEMELFATURA") {
		t.Fatal("profile missing")
	}
}

func TestBuildAndValidateEArsiv(t *testing.T) {
	doc := Document{
		UUID: uuid.NewString(), Number: "EAR2026000000001", Profile: ProfileEArsiv, Type: TypeSatis,
		IssueDate: "2026-10-16", IssueTime: "10:00:00", Currency: "TRY", Supplier: supplier(),
		Customer: Party{
			IdentityType: IdentityTCKN, Identity: "10000000146", FirstName: "Deniz", LastName: "Kaya",
			Street: "Moda Cad. 5", District: "Kadıköy", City: "İstanbul", CountryCode: "TR",
		},
		Lines: lines(t, "1999.99"),
	}
	_, _, problems, err := Validate(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Fatalf("problems: %+v", problems)
	}
}

func TestSchematronCatchesAReturnWithoutReference(t *testing.T) {
	doc := Document{
		UUID: uuid.NewString(), Number: "EFA2026000000002", Profile: ProfileTemel, Type: TypeIade,
		IssueDate: "2026-10-16", Currency: "TRY", Supplier: supplier(),
		Customer: Party{IdentityType: IdentityVKN, Identity: "9876543210", Name: "X", TaxOffice: "Y", District: "A", City: "B"},
		Lines:    lines(t, "120.00"),
	}
	data, _, err := Build(doc)
	if err != nil {
		t.Fatal(err)
	}
	problems, err := ValidateXML(context.Background(), data, doc.Profile, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) == 0 {
		t.Fatal("a return invoice without its billing reference passed the Schematron")
	}
	t.Logf("%+v", problems)
	doc.Returns = &Reference{Number: "EFA2026000000001", IssueDate: "2026-10-15"}
	data, _, err = Build(doc)
	if err != nil {
		t.Fatal(err)
	}
	if problems, _ := ValidateXML(context.Background(), data, doc.Profile, ""); len(problems) > 0 {
		t.Fatalf("return invoice: %+v", problems)
	}
}

func TestCheckReportsEveryMissingField(t *testing.T) {
	problems := Check(Document{Profile: ProfileEArsiv, Type: TypeSatis, Number: "EAR2026000000001", IssueDate: "2026-10-16",
		Supplier: supplier(), Customer: Party{IdentityType: IdentityTCKN, Identity: "123"},
		Lines: []Line{{Name: "", Rate: 0, Net: money.MustParse("10")}}})
	fields := map[string]bool{}
	for _, p := range problems {
		fields[p.Field] = true
	}
	for _, f := range []string{"receiver.identity_number", "receiver.name", "receiver.city", "receiver.district", "lines.0.name", "lines.0.exemption_code"} {
		if !fields[f] {
			t.Errorf("missing %s in %+v", f, problems)
		}
	}
}

func TestSplitLinesKeepsTheGrossTotal(t *testing.T) {
	for minor := int64(1); minor < 5000; minor++ {
		g := []money.Money{money.New(minor * 7), money.New(minor)}
		r := []money.Rate{money.Percent(20), money.Percent(20)}
		splits, diff := SplitLines(g, r)
		if !diff.IsZero() {
			t.Fatalf("gross %v: difference %s", g, diff)
		}
		for _, s := range splits {
			if s.VAT.Minor() != s.Net.MulRate(money.Percent(20)).Minor() {
				t.Fatal("vat drifted")
			}
		}
	}
	// One unreachable 20 % amount: 0.03 cannot be net + round(net/5).
	if _, diff := SplitLines([]money.Money{money.New(3)}, []money.Rate{money.Percent(20)}); diff.Minor() != -1 {
		t.Fatalf("diff = %s", diff)
	}
}
