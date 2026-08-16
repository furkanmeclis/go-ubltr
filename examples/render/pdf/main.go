// PDF rendering: XML → XSLT → HTML → Chromium.
// Requires a local Chrome/Chromium binary.
//
//	go run . -in ../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/chromium"
	"github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

func main() {
	in := flag.String("in", "../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml", "UBL Invoice XML")
	xslPath := flag.String("xsl", "", "optional XSLT file; empty uses StyleMinimal")
	out := flag.String("out", "invoice.pdf", "PDF output path")
	flag.Parse()

	xml, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	style := render.StyleMinimal()
	if *xslPath != "" {
		xsl, err := os.ReadFile(*xslPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		style = render.Style{Name: "custom", Stylesheet: xsl}
	}

	htmlR, err := render.NewHTML(helium.New(), style)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	pdfR := chromium.New(context.Background())
	defer pdfR.Close()

	pdf, err := render.ToPDF(context.Background(), htmlR, pdfR, xml, render.PDFOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, pdf, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}
