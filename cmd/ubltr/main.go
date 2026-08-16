// Command ubltr validates, inspects, and renders GİB UBL-TR XML.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `ubltr is a command-line tool for GİB UBL-TR XML.

Usage:
  ubltr <command> [arguments]

Commands:
  validate    Check XML against GİB XSD and Invoice Schematron
  inspect     Print a short document summary
  render      Write HTML (or PDF with --pdf)
  generate    Regenerate Go types from vendored XSD (repo checkout)

Exit codes:
  0  success (valid for validate)
  1  validation failure or command error

PDF rendering needs module github.com/furkanmeclis/go-ubltr/render/chromium
and a Chrome or Chromium binary on PATH.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	switch args[0] {
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "validate":
		return cmdValidate(args[1:], stdout, stderr)
	case "inspect":
		return cmdInspect(args[1:], stdout, stderr)
	case "render":
		return cmdRender(args[1:], stdout, stderr)
	case "generate":
		return cmdGenerate(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ubltr: unknown command %q\n", args[0])
		fmt.Fprint(stderr, usage)
		return 1
	}
}
