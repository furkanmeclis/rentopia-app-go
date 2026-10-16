package ubl

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/furkanmeclis/go-ubltr/render"
	heliumxslt "github.com/furkanmeclis/go-ubltr/render/xslt/helium"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
)

//go:embed invoice.xsl
var stylesheet []byte

var (
	htmlOnce sync.Once
	htmlR    *render.HTMLRenderer
	htmlErr  error
)

func htmlRenderer() (*render.HTMLRenderer, error) {
	htmlOnce.Do(func() {
		htmlR, htmlErr = render.NewHTML(heliumxslt.New(), render.Style{Name: "rentopia-invoice", Stylesheet: stylesheet})
	})
	return htmlR, htmlErr
}

// HTML renders the invoice XML with go-ubltr's render module (helium
// XSLT 3, pure Go) and the module's stylesheet. The result has no scripts
// and loads nothing.
func HTML(ctx context.Context, xml []byte) ([]byte, error) {
	r, err := htmlRenderer()
	if err != nil {
		return nil, fmt.Errorf("ubl: html renderer: %w", err)
	}
	out, err := r.Render(ctx, xml)
	if err != nil {
		return nil, fmt.Errorf("ubl: render html: %w", err)
	}
	return out, nil
}

// PDF renders the invoice to PDF: HTML as above, printed by the shared
// headless Chromium of pdfrender (go-ubltr's render/chromium).
func PDF(ctx context.Context, pdf pdfrender.Renderer, xml []byte) ([]byte, error) {
	if pdf == nil {
		return nil, pdfrender.ErrUnavailable
	}
	html, err := HTML(ctx, xml)
	if err != nil {
		return nil, err
	}
	return pdf.RenderPDF(ctx, html)
}
