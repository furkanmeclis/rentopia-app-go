package pdfrender

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// TestChromiumPrintsA4 prints a Turkish document with a remote image (the
// load is blocked) when a usable Chromium is on PATH. Hosts without one, or
// whose Chromium cannot start its sandbox (GitHub's Ubuntu 24.04 runners
// restrict user namespaces; the backend image runs it through a launcher
// instead), skip.
func TestChromiumPrintsA4(t *testing.T) {
	if !Available() {
		t.Skip("no chromium on PATH")
	}
	c := NewChromium()
	defer c.Close()
	html := []byte(`<!doctype html><html><head><meta charset="utf-8"></head><body>
<h1>Kiralama Sözleşmesi</h1><p>Çağrı Işık · ğüşöç</p><img src="http://127.0.0.1:1/x.png"></body></html>`)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		out, err := c.RenderPDF(ctx, html)
		if err != nil && strings.Contains(err.Error(), "No usable sandbox") {
			t.Skipf("chromium cannot start its sandbox on this host: %v", err)
		}
		if err != nil {
			t.Fatalf("print %d: %v", i, err)
		}
		if !bytes.HasPrefix(out, []byte("%PDF-")) {
			t.Fatalf("print %d: not a PDF (%d bytes)", i, len(out))
		}
	}
}
