package pdfrender

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestChromiumPrintsA4 prints a Turkish document with a remote image (the
// load is blocked) when a Chromium is on PATH; CI without one skips.
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
		if err != nil {
			t.Fatalf("print %d: %v", i, err)
		}
		if !bytes.HasPrefix(out, []byte("%PDF-")) {
			t.Fatalf("print %d: not a PDF (%d bytes)", i, len(out))
		}
	}
}
