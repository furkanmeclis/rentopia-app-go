package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func TestSanitizeDropsScriptsKeepsRichText(t *testing.T) {
	in := `<h1 style="text-align: center">Sözleşme</h1><p onclick="x()">Kiracı <b>{{renter.full_name}}</b></p>` +
		`<script>alert(1)</script><img src="https://evil.test/x.png"><a href="javascript:alert(1)">l</a>` +
		`<table><tr><td colspan="2">{{vehicle.plate}}</td></tr></table>`
	out := Sanitize(in)
	for _, bad := range []string{"<script", "onclick", "javascript:", "evil.test"} {
		if strings.Contains(out, bad) {
			t.Errorf("sanitized body still contains %q: %s", bad, out)
		}
	}
	for _, good := range []string{"text-align: center", "{{renter.full_name}}", "<table>", `colspan="2"`, "<b>"} {
		if !strings.Contains(out, good) {
			t.Errorf("sanitized body lost %q: %s", good, out)
		}
	}
}

func TestTokensAndFill(t *testing.T) {
	body := `<p>{{ renter.full_name }} {{vehicle.plate}} {{nope.key}} {{renter.full_name}}</p><div>{{drivers.table}}</div><p>{{amounts.paid}}</p>`
	known, unknown := Tokens(body)
	if strings.Join(known, ",") != "amounts.paid,drivers.table,renter.full_name,vehicle.plate" || strings.Join(unknown, ",") != "nope.key" {
		t.Fatalf("known %v unknown %v", known, unknown)
	}
	out, empty := fill(body, map[string]string{
		"renter.full_name": "Ayşe <Yılmaz>", "vehicle.plate": "34 ABC 1",
		"drivers.table": table([]string{"Sürücü"}, [][]string{{"<b>x</b>"}}),
	})
	if !strings.Contains(out, "Ayşe &lt;Yılmaz&gt;") {
		t.Errorf("text values must be escaped: %s", out)
	}
	if !strings.Contains(out, "<table>") || !strings.Contains(out, "&lt;b&gt;x&lt;/b&gt;") {
		t.Errorf("html blocks are inserted with escaped cells: %s", out)
	}
	if !strings.Contains(out, `class="ph-unknown"`) || len(empty) != 1 || empty[0] != "amounts.paid" {
		t.Errorf("unknown / empty marking: %v %s", empty, out)
	}
}

func TestDocumentHashChangesWithContent(t *testing.T) {
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	a := documentHTML(model.KindRentalContract, "Test A.Ş.", "T-2026-000001", 1, at, "<p>34 ABC 1</p>")
	b := documentHTML(model.KindRentalContract, "Test A.Ş.", "T-2026-000001", 1, at, "<p>34 ABC 2</p>")
	if contentHash(a) == contentHash(b) || len(contentHash(a)) != 64 {
		t.Fatal("the content hash must follow the content")
	}
	if contentHash(a) != contentHash(documentHTML(model.KindRentalContract, "Test A.Ş.", "T-2026-000001", 1, at, "<p>34 ABC 1</p>")) {
		t.Fatal("the content hash must be deterministic")
	}
}

func TestChangedKeysIgnoreVolatileAndUnused(t *testing.T) {
	keys := []string{"vehicle.plate", "amounts.total", "document.date"}
	got := changedKeys(keys, map[string]string{"vehicle.plate": "1", "amounts.total": "10", "document.date": "a", "renter.email": "x"},
		map[string]string{"vehicle.plate": "1", "amounts.total": "20", "document.date": "b", "renter.email": "y"})
	if strings.Join(got, ",") != "amounts.total" {
		t.Fatalf("changed keys %v", got)
	}
}

func TestFormatAmount(t *testing.T) {
	for in, want := range map[string]string{"4500": "4.500,00 TRY", "1234567.5": "1.234.567,50 TRY", "0": "0,00 TRY", "-12": "-12,00 TRY"} {
		if got := formatAmount(money.MustParse(in), "TRY"); got != want {
			t.Errorf("%s = %q, want %q", in, got, want)
		}
	}
}

func TestCatalogCoversSampleSnapshot(t *testing.T) {
	snap := sampleSnapshot()
	for _, p := range model.Catalog {
		if _, ok := snap.Values[p.Key]; !ok {
			t.Errorf("sample snapshot misses %s", p.Key)
		}
	}
}
