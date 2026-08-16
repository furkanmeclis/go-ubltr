// Package codegen turns GİB UBL-TR XSD files into encoding/xml Go types.
package codegen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	NSInvoice             = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	NSCreditNote          = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	NSApplicationResponse = "urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2"
	NSDespatchAdvice      = "urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2"
	NSReceiptAdvice       = "urn:oasis:names:specification:ubl:schema:xsd:ReceiptAdvice-2"
	NSCAC                 = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	NSCBC                 = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
	NSEXT                 = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
	NSUDT                 = "urn:oasis:names:specification:ubl:schema:xsd:UnqualifiedDataTypes-2"
	NSCCT                 = "urn:un:unece:uncefact:data:specification:CoreComponentTypeSchemaModule:2"
	NSQDT                 = "urn:oasis:names:specification:ubl:schema:xsd:QualifiedDataTypes-2"
)

var nsPkg = map[string]string{
	NSInvoice:             "invoice",
	NSCreditNote:          "creditnote",
	NSApplicationResponse: "applicationresponse",
	NSDespatchAdvice:      "despatchadvice",
	NSReceiptAdvice:       "receiptadvice",
	NSCAC:                 "cac",
	NSCBC:                 "cbc",
	NSEXT:                 "ext",
	NSUDT:                 "udt",
	NSCCT:                 "udt",
	NSQDT:                 "udt",
}

type documentRoot struct {
	NS, Name, TypeName, Pkg, Alias string
}

var documentRoots = []documentRoot{
	{NSInvoice, "Invoice", "InvoiceType", "invoice", "Invoice"},
	{NSCreditNote, "CreditNote", "CreditNoteType", "creditnote", "CreditNote"},
	{NSApplicationResponse, "ApplicationResponse", "ApplicationResponseType", "applicationresponse", "ApplicationResponse"},
	{NSDespatchAdvice, "DespatchAdvice", "DespatchAdviceType", "despatchadvice", "DespatchAdvice"},
	{NSReceiptAdvice, "ReceiptAdvice", "ReceiptAdviceType", "receiptadvice", "ReceiptAdvice"},
}

type Options struct {
	SchemaPath  string
	SchemaPaths []string
	OutDir      string
	Module      string
}

func (o Options) paths() []string {
	if len(o.SchemaPaths) > 0 {
		return o.SchemaPaths
	}
	if o.SchemaPath != "" {
		return []string{o.SchemaPath}
	}
	return nil
}

type generator struct {
	set      *set
	needed   map[string]bool
	visited  map[string]bool
	opts     Options
	checksum map[string]string
}

func Generate(opts Options) error {
	if opts.Module == "" {
		opts.Module = "github.com/furkanmeclis/go-ubltr"
	}
	paths := opts.paths()
	if len(paths) == 0 {
		return fmt.Errorf("no schema paths")
	}
	s, err := loadSet(paths...)
	if err != nil {
		return err
	}
	g := &generator{
		set:      s,
		needed:   map[string]bool{},
		visited:  map[string]bool{},
		opts:     opts,
		checksum: map[string]string{},
	}
	for _, p := range paths {
		if err := g.hashFile(p); err != nil {
			return err
		}
	}
	found := 0
	for _, root := range documentRoots {
		el, ok := s.elements[key(root.NS, root.Name)]
		if !ok {
			continue
		}
		found++
		g.markType(el.TypeNS, el.Type)
	}
	if found == 0 {
		return fmt.Errorf("no UBL document elements found in %v", paths)
	}
	g.markType(NSEXT, "UBLExtensionsType")
	g.markType(NSEXT, "ExtensionContentType")

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return err
	}
	byPkg := map[string][]typeRef{}
	for k := range g.needed {
		tr, ok := s.types[k]
		if !ok {
			continue
		}
		pkg := nsPkg[tr.NS]
		if pkg == "" {
			continue
		}
		byPkg[pkg] = append(byPkg[pkg], tr)
	}
	for pkg, types := range byPkg {
		sort.Slice(types, func(i, j int) bool { return types[i].Name < types[j].Name })
		if err := g.writePackage(pkg, types); err != nil {
			return err
		}
	}
	return g.writeManifest()
}

func (g *generator) hashFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	g.checksum[filepath.Base(path)] = hex.EncodeToString(sum[:])
	return nil
}

func (g *generator) markType(ns, name string) {
	if ns == "" || name == "" || ns == xsdNS {
		return
	}
	k := key(ns, name)
	if g.visited[k] {
		return
	}
	g.visited[k] = true
	tr, ok := g.set.types[k]
	if !ok {
		return
	}
	if ns != NSCCT && nsPkg[ns] != "" {
		g.needed[k] = true
	}
	ct := tr.Type
	if seq := sequenceOf(ct); seq != nil {
		for _, el := range seq.Elements {
			g.markElement(tr.NS, el)
		}
	}
	baseNS, baseName := g.baseOf(tr)
	if baseNS != "" && baseNS != xsdNS {
		g.markType(baseNS, baseName)
	}
}

func (g *generator) markElement(ownerNS string, el xsdElement) {
	ls := g.set.byNS[ownerNS]
	if el.Ref != "" {
		ns, local := ls.resolve(el.Ref)
		er, ok := g.set.elements[key(ns, local)]
		if !ok {
			return
		}
		g.markType(er.TypeNS, er.Type)
		return
	}
	if el.Type != "" {
		ns, local := ls.resolve(el.Type)
		g.markType(ns, local)
	}
}

func (g *generator) baseOf(tr typeRef) (ns, name string) {
	ls := g.set.byNS[tr.NS]
	ct := tr.Type
	if ct.SimpleContent != nil {
		if ct.SimpleContent.Extension != nil {
			return ls.resolve(ct.SimpleContent.Extension.Base)
		}
		if ct.SimpleContent.Restriction != nil {
			return ls.resolve(ct.SimpleContent.Restriction.Base)
		}
	}
	if ct.ComplexContent != nil {
		if ct.ComplexContent.Extension != nil {
			return ls.resolve(ct.ComplexContent.Extension.Base)
		}
		if ct.ComplexContent.Restriction != nil {
			return ls.resolve(ct.ComplexContent.Restriction.Base)
		}
	}
	return "", ""
}

func sequenceOf(ct xsdComplexType) *xsdSequence {
	if ct.Sequence != nil {
		return ct.Sequence
	}
	if ct.Choice != nil {
		return ct.Choice
	}
	if ct.ComplexContent != nil && ct.ComplexContent.Extension != nil {
		return ct.ComplexContent.Extension.Sequence
	}
	return nil
}

func (g *generator) writePackage(pkg string, types []typeRef) error {
	dir := filepath.Join(g.opts.OutDir, pkg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by ubltr-gen from UBL-TR 1.2.1 XSD. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", pkg)

	imports := g.importsFor(pkg, types)
	if len(imports) > 0 {
		fmt.Fprintf(&b, "import (\n")
		var std, ext []string
		for _, im := range imports {
			if strings.Contains(im, ".") {
				ext = append(ext, im)
			} else {
				std = append(std, im)
			}
		}
		for _, im := range std {
			fmt.Fprintf(&b, "\t%q\n", im)
		}
		if len(std) > 0 && len(ext) > 0 {
			fmt.Fprintf(&b, "\n")
		}
		for _, im := range ext {
			fmt.Fprintf(&b, "\t%q\n", im)
		}
		fmt.Fprintf(&b, ")\n\n")
	}

	seen := map[string]bool{}
	for _, tr := range types {
		if seen[tr.Name] {
			continue
		}
		seen[tr.Name] = true
		g.writeType(&b, pkg, tr)
	}

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		_ = os.WriteFile(filepath.Join(dir, "types.go"), b.Bytes(), 0o644)
		return fmt.Errorf("gofmt %s: %w", pkg, err)
	}
	return os.WriteFile(filepath.Join(dir, "types.go"), formatted, 0o644)
}

func (g *generator) importsFor(pkg string, types []typeRef) []string {
	need := map[string]bool{}
	if pkg == "ext" || g.isDocumentPkg(pkg) {
		need["encoding/xml"] = true
	}
	for _, tr := range types {
		if isSimpleContent(tr.Type) {
			continue
		}
		seq := sequenceOf(tr.Type)
		if seq == nil {
			continue
		}
		ls := g.set.byNS[tr.NS]
		for _, el := range seq.Elements {
			fieldPkg := g.fieldPkg(ls, el)
			if fieldPkg != "" && fieldPkg != pkg {
				need[g.opts.Module+"/schema/ubltr121/"+fieldPkg] = true
			}
		}
		if pkg != "udt" && isSimpleContent(tr.Type) {
			need[g.opts.Module+"/schema/ubltr121/udt"] = true
		}
	}
	for _, tr := range types {
		if pkg != "udt" && isSimpleContent(tr.Type) {
			baseNS, _ := g.baseOf(tr)
			if nsPkg[baseNS] == "udt" || baseNS == NSUDT || baseNS == NSCCT {
				need[g.opts.Module+"/schema/ubltr121/udt"] = true
			}
		}
	}
	out := make([]string, 0, len(need))
	for im := range need {
		out = append(out, im)
	}
	sort.Strings(out)
	return out
}

func (g *generator) fieldPkg(ls *loadedSchema, el xsdElement) string {
	if el.Ref != "" {
		ns, _ := ls.resolve(el.Ref)
		return nsPkg[ns]
	}
	if el.Type != "" {
		ns, _ := ls.resolve(el.Type)
		return nsPkg[ns]
	}
	return ""
}

func isSimpleContent(ct xsdComplexType) bool {
	return ct.SimpleContent != nil
}

func (g *generator) writeType(b *bytes.Buffer, pkg string, tr typeRef) {
	ct := tr.Type
	if seq := sequenceOf(ct); seq != nil && (len(seq.Anys) > 0 && len(seq.Elements) == 0) {
		fmt.Fprintf(b, "// %s is generated from XSD complexType %s.\n", tr.Name, tr.Name)
		fmt.Fprintf(b, "type %s struct {\n", goTypeName(tr.Name))
		fmt.Fprintf(b, "\tInnerXML string `xml:\",innerxml\"`\n")
		fmt.Fprintf(b, "}\n\n")
		return
	}
	if isSimpleContent(ct) {
		g.writeSimpleType(b, pkg, tr)
		return
	}
	seq := sequenceOf(ct)
	fmt.Fprintf(b, "// %s is generated from XSD complexType %s.\n", goTypeName(tr.Name), tr.Name)
	fmt.Fprintf(b, "type %s struct {\n", goTypeName(tr.Name))
	if pkg == "ext" && tr.Name == "UBLExtensionsType" {
		fmt.Fprintf(b, "\tXMLName xml.Name `xml:\"%s UBLExtensions\"`\n", NSEXT)
	}
	if root, ok := g.documentRoot(pkg, tr.Name); ok {
		fmt.Fprintf(b, "\tXMLName xml.Name `xml:\"%s %s\"`\n", root.NS, root.Name)
	}
	if seq != nil {
		ls := g.set.byNS[tr.NS]
		used := map[string]int{}
		for _, el := range seq.Elements {
			g.writeField(b, pkg, ls, el, used)
		}
		for _, any := range seq.Anys {
			_ = any
			fmt.Fprintf(b, "\tInnerXML string `xml:\",innerxml\"`\n")
		}
	}
	fmt.Fprintf(b, "}\n\n")
	if root, ok := g.documentRoot(pkg, tr.Name); ok {
		fmt.Fprintf(b, "// %s is the UBL-TR %s document element.\n", root.Alias, root.Name)
		fmt.Fprintf(b, "type %s = %s\n\n", root.Alias, goTypeName(tr.Name))
	}
}

func (g *generator) writeSimpleType(b *bytes.Buffer, pkg string, tr typeRef) {
	baseNS, baseName := g.baseOf(tr)
	if pkg != "udt" && (baseNS == NSUDT || baseNS == NSCCT || nsPkg[baseNS] == "udt") {
		goBase := baseName
		fmt.Fprintf(b, "// %s extends %s.\n", goTypeName(tr.Name), goBase)
		fmt.Fprintf(b, "type %s = udt.%s\n\n", goTypeName(tr.Name), goTypeName(goBase))
		return
	}
	attrs := g.collectAttrs(tr)
	fmt.Fprintf(b, "// %s is generated from XSD simpleContent type %s.\n", goTypeName(tr.Name), tr.Name)
	fmt.Fprintf(b, "type %s struct {\n", goTypeName(tr.Name))
	for _, a := range attrs {
		omitempty := ",omitempty"
		if a.Use == "required" {
			omitempty = ""
		}
		fmt.Fprintf(b, "\t%s string `xml:\"%s,attr%s\"`\n", goFieldName(a.Name), a.Name, omitempty)
	}
	fmt.Fprintf(b, "\tValue string `xml:\",chardata\"`\n")
	fmt.Fprintf(b, "}\n\n")
}

func (g *generator) collectAttrs(tr typeRef) []xsdAttribute {
	var out []xsdAttribute
	seen := map[string]int{}
	var walk func(ns, name string)
	walk = func(ns, name string) {
		if ns == xsdNS || ns == "" || name == "" {
			return
		}
		t, ok := g.set.types[key(ns, name)]
		if !ok {
			if ns == NSCCT {
				t, ok = g.set.types[key(NSUDT, name)]
			}
			if !ok {
				return
			}
		}
		ct := t.Type
		var attrs []xsdAttribute
		var base string
		ls := g.set.byNS[t.NS]
		if ct.SimpleContent != nil {
			if ct.SimpleContent.Extension != nil {
				attrs = ct.SimpleContent.Extension.Attributes
				base = ct.SimpleContent.Extension.Base
			}
			if ct.SimpleContent.Restriction != nil {
				attrs = ct.SimpleContent.Restriction.Attributes
				base = ct.SimpleContent.Restriction.Base
			}
		}
		if base != "" {
			bns, bname := ls.resolve(base)
			walk(bns, bname)
		}
		for _, a := range attrs {
			if a.Name == "" {
				continue
			}
			if i, ok := seen[a.Name]; ok {
				out[i] = a
				continue
			}
			seen[a.Name] = len(out)
			out = append(out, a)
		}
	}
	walk(tr.NS, tr.Name)
	return out
}

func (g *generator) writeField(b *bytes.Buffer, pkg string, ls *loadedSchema, el xsdElement, used map[string]int) {
	elemNS, elemName := "", ""
	typeNS, typeName := "", ""
	if el.Ref != "" {
		elemNS, elemName = ls.resolve(el.Ref)
		er, ok := g.set.elements[key(elemNS, elemName)]
		if !ok {
			return
		}
		typeNS, typeName = er.TypeNS, er.Type
	} else {
		elemNS, elemName = ls.TargetNS, el.Name
		typeNS, typeName = ls.resolve(el.Type)
	}
	fieldName := goFieldName(elemName)
	if n := used[fieldName]; n > 0 {
		fieldName = fmt.Sprintf("%s%d", fieldName, n+1)
	}
	used[fieldName]++

	optional := el.MinOccurs == "0"
	repeated := el.MaxOccurs == "unbounded" || (el.MaxOccurs != "" && el.MaxOccurs != "1" && el.MaxOccurs != "0")
	goType := g.goTypeRef(pkg, typeNS, typeName)
	tag := fmt.Sprintf("%s %s", elemNS, elemName)
	switch {
	case repeated:
		fmt.Fprintf(b, "\t%s []%s `xml:\"%s\"`\n", fieldName, goType, tag)
	case optional:
		fmt.Fprintf(b, "\t%s *%s `xml:\"%s,omitempty\"`\n", fieldName, goType, tag)
	default:
		fmt.Fprintf(b, "\t%s %s `xml:\"%s\"`\n", fieldName, goType, tag)
	}
}

func (g *generator) goTypeRef(currentPkg, ns, name string) string {
	pkg := nsPkg[ns]
	tname := goTypeName(name)
	if pkg == "" || pkg == currentPkg {
		return tname
	}
	return pkg + "." + tname
}

func (g *generator) isDocumentPkg(pkg string) bool {
	for _, root := range documentRoots {
		if root.Pkg == pkg {
			return true
		}
	}
	return false
}

func (g *generator) documentRoot(pkg, typeName string) (documentRoot, bool) {
	for _, root := range documentRoots {
		if root.Pkg == pkg && root.TypeName == typeName {
			return root, true
		}
	}
	return documentRoot{}, false
}

func (g *generator) writeManifest() error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by ubltr-gen from UBL-TR 1.2.1 XSD. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package ubltr121\n\n")
	fmt.Fprintf(&b, "// Package ubltr121 holds Go types generated from GİB UBL-TR 1.2.1 / UBL 2.1 XSD.\n")
	fmt.Fprintf(&b, "// UBL-TR schema definitions are the source of truth. Do not edit generated files.\n\n")
	fmt.Fprintf(&b, "const (\n")
	fmt.Fprintf(&b, "\tUBLTRVersion = %q\n", "1.2.1")
	fmt.Fprintf(&b, "\tUBLVersion   = %q\n", "2.1")
	fmt.Fprintf(&b, "\tGenerator    = %q\n", "github.com/furkanmeclis/go-ubltr/cmd/ubltr-gen")
	keys := make([]string, 0, len(g.checksum))
	for k := range g.checksum {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ident := strings.ReplaceAll(strings.TrimSuffix(k, ".xsd"), "-", "")
		ident = strings.ReplaceAll(ident, ".", "")
		fmt.Fprintf(&b, "\tChecksum%s = %q\n", ident, g.checksum[k])
	}
	fmt.Fprintf(&b, ")\n")
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(g.opts.OutDir, "generated.go"), formatted, 0o644); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(g.opts.OutDir, "doc.go"), []byte(`// Package ubltr121 contains Go types generated from GİB UBL-TR 1.2.1 XML Schema.
//
// UBL-TR schema definitions are the source of truth. Generated Go types are
// derived from the schemas and must not be edited by hand.
//
// Advanced users may import subpackages (cbc, cac, ext, invoice, creditnote,
// applicationresponse, despatchadvice, receiptadvice, udt) directly.
// The developer-facing API lives in module github.com/furkanmeclis/go-ubltr.
package ubltr121
`), 0o644)
	return nil
}

func goTypeName(name string) string {
	if name == "" {
		return "T"
	}
	return export(name)
}

func goFieldName(name string) string {
	n := export(name)
	if isGoKeyword(strings.ToLower(n)) {
		return n + "_"
	}
	return n
}

func export(name string) string {
	if name == "" {
		return "X"
	}
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func isGoKeyword(s string) bool {
	switch s {
	case "break", "default", "func", "interface", "select", "case", "defer",
		"go", "map", "struct", "chan", "else", "goto", "package", "switch",
		"const", "fallthrough", "if", "range", "type", "continue", "for",
		"import", "return", "var":
		return true
	}
	return false
}
