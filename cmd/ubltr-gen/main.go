// Command ubltr-gen generates Go types from GİB UBL-TR XSD files,
// GİB e-Arşiv report types from EArsiv.xsd, GİB envelope types from
// PackageProxy_1_2.xsd, or code-list constants from UBL-TR_Codelist.xml.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/furkanmeclis/go-ubltr/internal/codegen"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	var schemas stringList
	flag.Var(&schemas, "schema", "entry XSD path (repeatable; union of reachable types)")
	codelist := flag.String("codelist", "", "if set, generate code lists from this UBL-TR_Codelist.xml")
	earchive := flag.String("earchive", "", "if set, generate e-Arşiv report types from this EArsiv.xsd")
	envelope := flag.String("envelope", "", "if set, generate GİB envelope types from this PackageProxy_1_2.xsd")
	out := flag.String("out", "schema/ubltr121", "output directory")
	mod := flag.String("module", "github.com/furkanmeclis/go-ubltr", "Go module path")
	flag.Parse()

	if *codelist != "" {
		if err := codegen.GenerateCodeLists(*codelist, *out); err != nil {
			fmt.Fprintf(os.Stderr, "ubltr-gen: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *earchive != "" {
		if err := codegen.GenerateEArchive(*earchive, *out); err != nil {
			fmt.Fprintf(os.Stderr, "ubltr-gen: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *envelope != "" {
		if err := codegen.GenerateEnvelope(*envelope, *out); err != nil {
			fmt.Fprintf(os.Stderr, "ubltr-gen: %v\n", err)
			os.Exit(1)
		}
		return
	}

	paths := []string(schemas)
	if len(paths) == 0 {
		paths = []string{"schemas/ubltr-1.2.1/xsdrt/maindoc/UBL-Invoice-2.1.xsd"}
	}

	if err := codegen.Generate(codegen.Options{
		SchemaPaths: paths,
		OutDir:      *out,
		Module:      *mod,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "ubltr-gen: %v\n", err)
		os.Exit(1)
	}
}
