// Package chromium prints HTML to PDF with headless Chrome.
package chromium

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/furkanmeclis/go-ubltr/render"
)

// Renderer drives a Chrome process. Call Close when finished.
type Renderer struct {
	alloc context.Context
	stop  context.CancelFunc
}

// New starts a Chrome allocator. parent may be nil.
func New(parent context.Context) *Renderer {
	if parent == nil {
		parent = context.Background()
	}
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("hide-scrollbars", true),
	)
	alloc, stop := chromedp.NewExecAllocator(parent, opts...)
	return &Renderer{alloc: alloc, stop: stop}
}

// Close stops the Chrome process.
func (r *Renderer) Close() {
	if r != nil && r.stop != nil {
		r.stop()
	}
}

// RenderPDF implements [render.PDFRenderer].
func (r *Renderer) RenderPDF(ctx context.Context, html []byte, opt render.PDFOptions) ([]byte, error) {
	if r == nil || r.alloc == nil {
		return nil, fmt.Errorf("%w: nil renderer", render.ErrPDFRender)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	tab, cancel := chromedp.NewContext(r.alloc)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()

	var buf []byte
	err := chromedp.Run(tab, printActions(html, opt, &buf)...)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", render.ErrPDFRender, trimChromeErr(err))
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("%w: empty pdf", render.ErrPDFRender)
	}
	return buf, nil
}

func printActions(html []byte, opt render.PDFOptions, buf *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		network.Enable(),
		network.SetBlockedURLs().WithURLPatterns([]*network.BlockPattern{
			{URLPattern: "http://*:*/*", Block: true},
			{URLPattern: "https://*:*/*", Block: true},
			{URLPattern: "file://*:*/*", Block: true},
			{URLPattern: "ftp://*:*/*", Block: true},
		}),
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(tree.Frame.ID, string(html)).Do(ctx)
		}),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			print := page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(render.A4WidthInches()).
				WithPaperHeight(render.A4HeightInches()).
				WithLandscape(opt.Landscape)
			if opt.MarginTop != 0 || opt.MarginBottom != 0 || opt.MarginLeft != 0 || opt.MarginRight != 0 {
				print = print.
					WithMarginTop(opt.MarginTop).
					WithMarginBottom(opt.MarginBottom).
					WithMarginLeft(opt.MarginLeft).
					WithMarginRight(opt.MarginRight)
			}
			data, _, err := print.Do(ctx)
			if err != nil {
				return err
			}
			*buf = data
			return nil
		}),
	}
}

func trimChromeErr(err error) string {
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 512 {
		return msg[:512]
	}
	return msg
}
