// Package pdftest finds a host browser for tests that print real PDFs.
package pdftest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// candidates are tried in order. Google Chrome comes first because on Ubuntu
// runners `chromium` / `chromium-browser` are snap wrappers that never print
// a DevTools URL when started outside snapd's session.
var candidates = []string{
	"google-chrome-stable", "google-chrome", "headless_shell", "headless-shell",
	"chromium", "chromium-browser",
}

// Browser returns the first usable browser on PATH, or "" when none is.
func Browser() string {
	for _, name := range candidates {
		p, err := exec.LookPath(name)
		if err != nil || isSnap(p) {
			continue
		}
		return p
	}
	return ""
}

func isSnap(path string) bool {
	if resolved, err := filepath.EvalSymlinks(path); err == nil && strings.HasPrefix(resolved, "/snap/") {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 1024)
	n, _ := f.Read(head)
	return strings.Contains(string(head[:n]), "/snap/")
}

// Launcher writes a script into dir that starts browser with the flags the
// backend image's /usr/local/bin/chromium adds (backend/Dockerfile).
func Launcher(dir, browser string) string {
	return "#!/bin/sh\nexec " + browser + " --no-sandbox --disable-dev-shm-usage --disable-gpu --disable-crash-reporter --crash-dumps-dir=" + filepath.Join(dir, "crashpad") + " \"$@\"\n"
}
