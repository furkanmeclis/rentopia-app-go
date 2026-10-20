package ubl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender/pdftest"
	"github.com/google/uuid"
)

func sampleXML(t *testing.T) []byte {
	t.Helper()
	data, _, err := Build(Document{
		UUID: uuid.NewString(), Number: "EAR2026000000007", Profile: ProfileEArsiv, Type: TypeSatis,
		IssueDate: "2026-10-16", IssueTime: "09:30:00", Currency: "TRY", Supplier: supplier(),
		Customer: Party{IdentityType: IdentityTCKN, Identity: "10000000146", FirstName: "Şule", LastName: "Güneş",
			District: "Üsküdar", City: "İstanbul"},
		Lines: lines(t, "2400.00", "360.00"),
		Notes: []string{"Kiralama IDS-2026-000042"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestHTMLShowsTheInvoice(t *testing.T) {
	html, err := HTML(context.Background(), sampleXML(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	for _, want := range []string{"e-Arşiv Fatura", "EAR2026000000007", "Şule", "2.760,00 TL", "Ödenecek tutar", "Kiralama IDS-2026-000042"} {
		if !strings.Contains(s, want) {
			t.Errorf("html misses %q", want)
		}
	}
	if strings.Contains(strings.ToLower(s), "<script") {
		t.Fatal("the invoice view must not carry scripts")
	}
}

// TestPDFPrintsWithChromium prints through pdfrender when a Chromium is on
// PATH (the same no-sandbox launcher as the pdfrender test); hosts without
// one skip.
func TestPDFPrintsWithChromium(t *testing.T) {
	real := pdftest.Browser()
	if real == "" {
		t.Skip("no chromium on PATH")
	}
	dir := t.TempDir()
	script := pdftest.Launcher(dir, real)
	t.Logf("browser: %s", real)
	if err := os.WriteFile(filepath.Join(dir, "headless_shell"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := pdfrender.NewChromium()
	defer r.Close()
	pdf, err := PDF(context.Background(), r, sampleXML(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}
