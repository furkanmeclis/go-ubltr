// Command ubltr-gen generates Go types from GİB UBL-TR XSD files.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/furkanmeclis/go-ubltr/internal/codegen"
)

func main() {
	schema := flag.String("schema", "schemas/ubltr-1.2.1/xsdrt/maindoc/UBL-Invoice-2.1.xsd", "entry XSD path")
	out := flag.String("out", "schema/ubltr121", "output directory")
	mod := flag.String("module", "github.com/furkanmeclis/go-ubltr", "Go module path")
	flag.Parse()

	if err := codegen.Generate(codegen.Options{
		SchemaPath: *schema,
		OutDir:     *out,
		Module:     *mod,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "ubltr-gen: %v\n", err)
		os.Exit(1)
	}
}
