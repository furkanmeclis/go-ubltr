package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/chromium"
	heliumxslt "github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

func cmdRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pdf := fs.Bool("pdf", false, "write PDF instead of HTML")
	stylePath := fs.String("style", "", "XSLT stylesheet (.xsl / .xslt); default is StyleMinimal")
	fs.Usage = func() {
		fmt.Fprint(stderr, `Usage: ubltr render [options] FILE

  --pdf           write PDF to stdout (needs Chrome/Chromium)
  --style FILE    XSLT stylesheet; default is the bundled minimal sheet

PDF needs github.com/furkanmeclis/go-ubltr/render/chromium and a Chrome or
Chromium binary on PATH.
`)
	}
	if err := fs.Parse(flagsFirst(args, "style")); err != nil {
		return 1
	}
	files := fs.Args()
	if len(files) != 1 {
		fs.Usage()
		return 1
	}

	xmlBytes, err := os.ReadFile(files[0])
	if err != nil {
		fmt.Fprintf(stderr, "ubltr render: %v\n", err)
		return 1
	}

	style := render.StyleMinimal()
	if *stylePath != "" {
		xsl, err := os.ReadFile(*stylePath)
		if err != nil {
			fmt.Fprintf(stderr, "ubltr render: %v\n", err)
			return 1
		}
		style = render.Style{Name: "custom", Stylesheet: xsl}
	}

	htmlR, err := render.NewHTML(heliumxslt.New(), style)
	if err != nil {
		fmt.Fprintf(stderr, "ubltr render: %v\n", err)
		return 1
	}
	ctx := context.Background()
	if !*pdf {
		html, err := htmlR.Render(ctx, xmlBytes)
		if err != nil {
			fmt.Fprintf(stderr, "ubltr render: %v\n", err)
			return 1
		}
		if _, err := stdout.Write(html); err != nil {
			fmt.Fprintf(stderr, "ubltr render: %v\n", err)
			return 1
		}
		return 0
	}

	pdfR := chromium.New(ctx)
	defer pdfR.Close()
	out, err := render.ToPDF(ctx, htmlR, pdfR, xmlBytes, render.PDFOptions{})
	if err != nil {
		fmt.Fprintf(stderr, "ubltr render: %s\n", pdfHint(err))
		return 1
	}
	if _, err := stdout.Write(out); err != nil {
		fmt.Fprintf(stderr, "ubltr render: %v\n", err)
		return 1
	}
	return 0
}

func pdfHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if missingChrome(err) {
		return "pdf requires github.com/furkanmeclis/go-ubltr/render/chromium and a Chrome or Chromium binary on PATH"
	}
	return msg
}

func missingChrome(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, render.ErrPDFRender) {
		msg := strings.ToLower(err.Error())
		return strings.Contains(msg, "executable file not found") ||
			strings.Contains(msg, "chrome not found") ||
			(strings.Contains(msg, "google-chrome") && strings.Contains(msg, "not found"))
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "chrome not found")
}
