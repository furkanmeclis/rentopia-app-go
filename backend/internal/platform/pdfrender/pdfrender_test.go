package pdfrender

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// withLauncher puts a launcher in front of PATH that starts the host's
// Chromium with the flags the backend image's /usr/local/bin/chromium adds
// (backend/Dockerfile), so the test prints the way production does. CI
// runners (Ubuntu 24.04) cannot start Chrome's sandbox.
func withLauncher(t *testing.T) {
	t.Helper()
	var real string
	for _, name := range executables {
		if p, err := exec.LookPath(name); err == nil {
			real = p
			break
		}
	}
	if real == "" {
		t.Skip("no chromium on PATH")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nexec " + real + " --no-sandbox --disable-dev-shm-usage --disable-gpu --disable-crash-reporter --crash-dumps-dir=" + filepath.Join(dir, "crashpad") + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, executables[0]), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestChromiumPrintsA4 prints a Turkish document with a remote image (the
// load is blocked) when a Chromium is on PATH; hosts without one skip.
func TestChromiumPrintsA4(t *testing.T) {
	withLauncher(t)
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
