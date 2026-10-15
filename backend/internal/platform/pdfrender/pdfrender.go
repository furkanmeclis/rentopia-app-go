// Package pdfrender prints HTML documents to PDF with headless Chromium
// through go-ubltr's render/chromium module (github.com/furkanmeclis/go-ubltr/render/chromium).
// The Chrome process starts on first use and is reused; network loads are
// blocked during print (images must be data: URLs). The backend image ships
// Alpine's chromium with DejaVu fonts (Turkish glyphs) and a no-sandbox
// launcher (backend/Dockerfile); without a Chromium on PATH RenderPDF
// answers ErrUnavailable and the caller records the failure.
package pdfrender

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/chromium"
)

// ErrUnavailable: no Chromium executable was found.
var ErrUnavailable = errors.New("pdfrender: chromium is not available")

// Renderer prints one HTML document to PDF.
type Renderer interface {
	RenderPDF(ctx context.Context, html []byte) ([]byte, error)
}

// executables chromedp looks up (same order as its allocator).
var executables = []string{"headless_shell", "headless-shell", "chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}

// Available reports whether a Chromium executable is on PATH.
func Available() bool {
	for _, name := range executables {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// Chromium is a lazily started, shared headless Chromium.
type Chromium struct {
	mu      sync.Mutex
	r       *chromium.Renderer
	timeout time.Duration
	// compat is set once Chromium refused go-ubltr's request blocking
	// (Network.setBlockedURLs urlPatterns needs Chrome 142+); later prints
	// use the Fetch-based printer of compat.go.
	compat bool
}

// NewChromium builds the renderer (nothing starts until the first print).
func NewChromium() *Chromium { return &Chromium{timeout: 60 * time.Second} }

// A4 with 15 mm margins.
var options = render.PDFOptions{MarginTop: 0.6, MarginBottom: 0.6, MarginLeft: 0.6, MarginRight: 0.6}

// RenderPDF implements Renderer.
func (c *Chromium) RenderPDF(ctx context.Context, html []byte) ([]byte, error) {
	if !Available() {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	c.mu.Lock()
	compat := c.compat
	if c.r == nil {
		c.r = chromium.New(context.Background())
	}
	r := c.r
	c.mu.Unlock()
	if compat {
		return printCompat(ctx, html, options)
	}
	out, err := r.RenderPDF(ctx, html, options)
	if err != nil && unsupportedBlocking(err) {
		c.mu.Lock()
		c.compat = true
		c.mu.Unlock()
		return printCompat(ctx, html, options)
	}
	if err != nil {
		// A crashed browser is restarted on the next print.
		c.mu.Lock()
		if c.r == r {
			c.r.Close()
			c.r = nil
		}
		c.mu.Unlock()
		return nil, err
	}
	return out, nil
}

// Close stops the browser.
func (c *Chromium) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.r != nil {
		c.r.Close()
		c.r = nil
	}
}
