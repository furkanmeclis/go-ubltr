// HTML rendering from UBL XML with the bundled minimal stylesheet.
//
//	go run . -in ../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

func main() {
	in := flag.String("in", "../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml", "UBL Invoice XML")
	out := flag.String("out", "invoice.html", "HTML output path")
	flag.Parse()

	xml, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	r, err := render.NewHTML(helium.New(), render.StyleMinimal())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	html, err := r.Render(context.Background(), xml)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, html, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}
