package pdfrender

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/furkanmeclis/go-ubltr/render"
)

// unsupportedBlocking reports go-ubltr's print failing on
// Network.setBlockedURLs: its urlPatterns parameter needs Chrome 142+, and
// older Chromium (Alpine stable, Playwright 1194 = Chrome 141) answers
// "Invalid parameters (-32602)" before anything is printed.
func unsupportedBlocking(err error) bool {
	return errors.Is(err, render.ErrPDFRender) && strings.Contains(err.Error(), "-32602")
}

// printCompat prints like go-ubltr's renderer (A4, the same margins, the
// HTML set as the document of about:blank) but blocks network loads with the
// Fetch domain, which every supported Chromium accepts: each request is
// failed before it leaves the browser (data: URLs never reach Fetch).
func printCompat(ctx context.Context, html []byte, opt render.PDFOptions) ([]byte, error) {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("hide-scrollbars", true),
	)
	alloc, stopAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer stopAlloc()
	tab, cancel := chromedp.NewContext(alloc)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()

	chromedp.ListenTarget(tab, func(ev any) {
		if e, ok := ev.(*fetch.EventRequestPaused); ok {
			go func() {
				_ = chromedp.Run(tab, fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient))
			}()
		}
	})

	var buf []byte
	err := chromedp.Run(tab,
		chromedp.Navigate("about:blank"),
		fetch.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(tree.Frame.ID, string(html)).Do(ctx)
		}),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			data, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(render.A4WidthInches()).
				WithPaperHeight(render.A4HeightInches()).
				WithLandscape(opt.Landscape).
				WithMarginTop(opt.MarginTop).
				WithMarginBottom(opt.MarginBottom).
				WithMarginLeft(opt.MarginLeft).
				WithMarginRight(opt.MarginRight).
				Do(ctx)
			buf = data
			return err
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", render.ErrPDFRender, err)
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("%w: empty pdf", render.ErrPDFRender)
	}
	return buf, nil
}
