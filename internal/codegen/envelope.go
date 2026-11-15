package codegen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GİB envelope namespaces.
const (
	NSSBDH    = "http://www.unece.org/cefact/namespaces/StandardBusinessDocumentHeader"
	NSPackage = "http://www.efatura.gov.tr/package-namespace"
	nsXSI     = "http://www.w3.org/2001/XMLSchema-instance"
)

// envelopeWildcards resolves xs:any particles that the GİB envelope fills with
// a known element. StandardBusinessDocument carries ef:Package after the
// header (the XSD says ##other, lax). Other wildcards, such as the UBL
// documents inside ElementList, stay as InnerXML.
var envelopeWildcards = map[string]xml.Name{
	"StandardBusinessDocument": {Space: NSPackage, Local: "Package"},
}

// envelopeSchemaLocationRoots get an xsi:schemaLocation attribute field. The
// GİB Schematron requires it on sh:StandardBusinessDocument.
var envelopeSchemaLocationRoots = map[string]bool{
	"StandardBusinessDocument": true,
}

// xnode is a generic XSD element tree kept in document order.
type xnode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []xnode    `xml:",any"`
}

func (n xnode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n xnode) is(local string) bool {
	return n.XMLName.Space == xsdNS && n.XMLName.Local == local
}

type envSchema struct {
	file        string
	targetNS    string
	qualified   bool
	prefixes    map[string]string
	defaultNS   string
	complexType map[string]xnode
	simpleType  map[string]xnode
	element     map[string]xnode
	group       map[string]xnode
}

type envField struct {
	Name     string
	GoType   string
	Tag      string
	Slice    bool
	Pointer  bool
	Omit     bool
	Attr     bool
	CharData bool
	InnerXML bool
}

type envType struct {
	Name    string
	Comment string
	XMLName xml.Name
	// ResetNS marks types used by unqualified local elements of a namespaced
	// schema (ef:Package/Elements). encoding/xml does not undeclare a default
	// namespace, so these types carry an always-written xmlns="" attribute.
	ResetNS bool
	Fields  []envField
}

type envGen struct {
	dir       string
	schemas   map[string]*envSchema // by file
	byNS      map[string][]*envSchema
	types     map[string]*envType
	pending   map[string]bool
	enums     map[string][]string
	substs    map[xml.Name][]xml.Name // abstract head → members
	checksums map[string]string
}

// GenerateEnvelope writes Go types for the GİB envelope XSDs reachable from
// entry (normally PackageProxy_1_2.xsd): the UN/CEFACT Standard Business
// Document Header and the GİB ef:Package. Imports whose schemaLocation is not
// in the envelope directory (UBL maindoc and HR-XML) are skipped; documents
// inside ElementList are kept as InnerXML.
func GenerateEnvelope(entry, outDir string) error {
	g := &envGen{
		dir:       filepath.Dir(entry),
		schemas:   map[string]*envSchema{},
		byNS:      map[string][]*envSchema{},
		types:     map[string]*envType{},
		pending:   map[string]bool{},
		enums:     map[string][]string{},
		substs:    map[xml.Name][]xml.Name{},
		checksums: map[string]string{},
	}
	if err := g.load(filepath.Base(entry)); err != nil {
		return err
	}
	g.collectSubstitutions()

	roots := []xml.Name{
		{Space: NSSBDH, Local: "StandardBusinessDocument"},
		{Space: NSSBDH, Local: "StandardBusinessDocumentHeader"},
		{Space: NSPackage, Local: "Package"},
	}
	for _, r := range roots {
		if _, err := g.globalElementType(r); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := g.writeTypes(outDir); err != nil {
		return err
	}
	return g.writeManifest(outDir)
}

func (g *envGen) load(file string) error {
	if _, ok := g.schemas[file]; ok {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(g.dir, file))
	if err != nil {
		return fmt.Errorf("read envelope xsd %s: %w", file, err)
	}
	sum := sha256.Sum256(data)
	g.checksums[file] = hex.EncodeToString(sum[:])

	var root xnode
	if err := xml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse envelope xsd %s: %w", file, err)
	}
	s := &envSchema{
		file:        file,
		targetNS:    root.attr("targetNamespace"),
		qualified:   root.attr("elementFormDefault") == "qualified",
		prefixes:    map[string]string{},
		complexType: map[string]xnode{},
		simpleType:  map[string]xnode{},
		element:     map[string]xnode{},
		group:       map[string]xnode{},
	}
	for _, a := range root.Attrs {
		switch {
		case a.Name.Space == "xmlns":
			s.prefixes[a.Name.Local] = a.Value
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			s.defaultNS = a.Value
		}
	}
	g.schemas[file] = s
	g.byNS[s.targetNS] = append(g.byNS[s.targetNS], s)

	for _, n := range root.Nodes {
		switch {
		case n.is("include"), n.is("import"):
			loc := n.attr("schemaLocation")
			if loc == "" || strings.Contains(loc, "/") {
				continue
			}
			if _, err := os.Stat(filepath.Join(g.dir, loc)); errors.Is(err, fs.ErrNotExist) {
				// UBL maindoc imports: those documents live in schema/ubltr121.
				continue
			}
			if err := g.load(loc); err != nil {
				return err
			}
		case n.is("complexType"):
			s.complexType[n.attr("name")] = n
		case n.is("simpleType"):
			s.simpleType[n.attr("name")] = n
		case n.is("element"):
			s.element[n.attr("name")] = n
		case n.is("group"):
			s.group[n.attr("name")] = n
		}
	}
	return nil
}

// resolve turns a QName attribute value into an expanded name.
func (s *envSchema) resolve(qname string) xml.Name {
	prefix, local, ok := strings.Cut(qname, ":")
	if !ok {
		return xml.Name{Space: s.defaultNS, Local: qname}
	}
	return xml.Name{Space: s.prefixes[prefix], Local: local}
}

func (g *envGen) lookup(n xml.Name, kind string) (xnode, *envSchema, bool) {
	for _, s := range g.byNS[n.Space] {
		var m map[string]xnode
		switch kind {
		case "complexType":
			m = s.complexType
		case "simpleType":
			m = s.simpleType
		case "element":
			m = s.element
		case "group":
			m = s.group
		}
		if node, ok := m[n.Local]; ok {
			return node, s, true
		}
	}
	return xnode{}, nil, false
}

func (g *envGen) collectSubstitutions() {
	files := make([]string, 0, len(g.schemas))
	for f := range g.schemas {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		s := g.schemas[f]
		names := make([]string, 0, len(s.element))
		for n := range s.element {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			el := s.element[name]
			if sg := el.attr("substitutionGroup"); sg != "" {
				head := s.resolve(sg)
				g.substs[head] = append(g.substs[head], xml.Name{Space: s.targetNS, Local: name})
			}
		}
	}
}

// globalElementType returns the Go type for a global element, generating it
// (with XMLName) when the element owns its type.
func (g *envGen) globalElementType(n xml.Name) (string, error) {
	el, s, ok := g.lookup(n, "element")
	if !ok {
		return "", fmt.Errorf("envelope: global element %s not found", n.Local)
	}
	if ct, ok := firstChild(el, "complexType"); ok {
		return g.complexType(n.Local, ct, s, n)
	}
	if t := el.attr("type"); t != "" {
		tn := s.resolve(t)
		if tn.Space == xsdNS {
			return "string", nil
		}
		if ct, cs, ok := g.lookup(tn, "complexType"); ok {
			root := xml.Name{}
			if tn.Local == n.Local {
				root = n
			}
			return g.complexType(tn.Local, ct, cs, root)
		}
		return g.simpleType(tn)
	}
	return "string", nil
}

func (g *envGen) simpleType(n xml.Name) (string, error) {
	st, _, ok := g.lookup(n, "simpleType")
	if !ok {
		return "string", nil
	}
	if r, ok := firstChild(st, "restriction"); ok {
		var vals []string
		for _, e := range r.Nodes {
			if e.is("enumeration") {
				vals = append(vals, e.attr("value"))
			}
		}
		if len(vals) > 0 {
			g.enums[export(n.Local)] = vals
		}
	}
	return "string", nil
}

func (g *envGen) complexType(name string, ct xnode, s *envSchema, root xml.Name) (string, error) {
	goName := export(name)
	if g.types[goName] != nil || g.pending[goName] {
		return goName, nil
	}
	g.pending[goName] = true
	t := &envType{Name: goName, Comment: name, XMLName: root}
	if envelopeSchemaLocationRoots[name] {
		t.Fields = append(t.Fields, envField{
			Name: "SchemaLocation", GoType: "string",
			Tag: nsXSI + " schemaLocation", Attr: true, Omit: true,
		})
	}
	for _, c := range ct.Nodes {
		switch {
		case c.is("sequence"), c.is("choice"), c.is("all"):
			if err := g.particles(t, c, s, c.is("choice"), isRepeated(c)); err != nil {
				return "", err
			}
		case c.is("attribute"):
			g.attribute(t, c)
		case c.is("simpleContent"):
			if ext, ok := firstChild(c, "extension"); ok {
				for _, a := range ext.Nodes {
					if a.is("attribute") {
						g.attribute(t, a)
					}
				}
			}
			t.Fields = append(t.Fields, envField{Name: "Value", GoType: "string", CharData: true})
		}
	}
	delete(g.pending, goName)
	g.types[goName] = t
	return goName, nil
}

func (g *envGen) attribute(t *envType, a xnode) {
	name := a.attr("name")
	if name == "" {
		return
	}
	if typ := a.attr("type"); typ != "" {
		for _, s := range g.schemas {
			if _, ok := s.simpleType[typ]; ok {
				_, _ = g.simpleType(xml.Name{Space: s.targetNS, Local: typ})
				break
			}
		}
	}
	t.Fields = append(t.Fields, envField{
		Name: export(name), GoType: "string", Tag: name,
		Attr: true, Omit: a.attr("use") != "required",
	})
}

func (g *envGen) particles(t *envType, p xnode, s *envSchema, inChoice, repeated bool) error {
	for _, c := range p.Nodes {
		switch {
		case c.is("element"):
			if err := g.element(t, c, s, inChoice, repeated); err != nil {
				return err
			}
		case c.is("sequence"), c.is("choice"):
			if err := g.particles(t, c, s, inChoice || c.is("choice"), repeated || isRepeated(c)); err != nil {
				return err
			}
		case c.is("group"):
			grp, gs, ok := g.lookup(s.resolve(c.attr("ref")), "group")
			if !ok {
				return fmt.Errorf("envelope: group %s not found", c.attr("ref"))
			}
			for _, gc := range grp.Nodes {
				if gc.is("sequence") || gc.is("choice") {
					if err := g.particles(t, gc, gs, inChoice || gc.is("choice"), repeated || isRepeated(c)); err != nil {
						return err
					}
				}
			}
		case c.is("any"):
			if err := g.wildcard(t, c, repeated); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *envGen) wildcard(t *envType, c xnode, repeated bool) error {
	if target, ok := envelopeWildcards[t.Comment]; ok {
		goType, err := g.globalElementType(target)
		if err != nil {
			return err
		}
		t.Fields = append(t.Fields, envField{
			Name: export(target.Local), GoType: goType,
			Tag:     target.Space + " " + target.Local,
			Pointer: !repeated && !isRepeated(c), Slice: repeated || isRepeated(c), Omit: true,
		})
		return nil
	}
	t.Fields = append(t.Fields, envField{Name: "InnerXML", GoType: "string", InnerXML: true})
	return nil
}

func (g *envGen) element(t *envType, c xnode, s *envSchema, inChoice, repeated bool) error {
	optional := inChoice || c.attr("minOccurs") == "0"
	many := repeated || isRepeated(c)

	if ref := c.attr("ref"); ref != "" {
		rn := s.resolve(ref)
		el, _, ok := g.lookup(rn, "element")
		if !ok {
			return fmt.Errorf("envelope: element ref %s not found", ref)
		}
		if el.attr("abstract") == "true" {
			for _, m := range g.substs[rn] {
				goType, err := g.globalElementType(m)
				if err != nil {
					return err
				}
				g.addField(t, m.Local, goType, m.Space+" "+m.Local, true, many)
			}
			return nil
		}
		goType, err := g.globalElementType(rn)
		if err != nil {
			return err
		}
		g.addField(t, rn.Local, goType, rn.Space+" "+rn.Local, optional, many)
		return nil
	}

	name := c.attr("name")
	tag := name
	if s.qualified {
		tag = s.targetNS + " " + name
	}
	goType := "string"
	switch ct, ok := firstChild(c, "complexType"); {
	case ok:
		var err error
		goType, err = g.complexType(name, ct, s, xml.Name{})
		if err != nil {
			return err
		}
	case c.attr("type") != "":
		tn := s.resolve(c.attr("type"))
		if tn.Space != xsdNS {
			if nct, cs, ok := g.lookup(tn, "complexType"); ok {
				var err error
				goType, err = g.complexType(tn.Local, nct, cs, xml.Name{})
				if err != nil {
					return err
				}
			} else {
				_, _ = g.simpleType(tn)
			}
		}
	}
	if !s.qualified && s.targetNS != "" {
		if ct := g.types[goType]; ct != nil {
			ct.ResetNS = true
		}
	}
	g.addField(t, name, goType, tag, optional, many)
	return nil
}

func (g *envGen) addField(t *envType, name, goType, tag string, optional, many bool) {
	t.Fields = append(t.Fields, envField{
		Name:    export(name),
		GoType:  goType,
		Tag:     tag,
		Slice:   many,
		Pointer: !many && optional && goType != "string",
		Omit:    optional || many,
	})
}

func isRepeated(n xnode) bool {
	m := n.attr("maxOccurs")
	return m == "unbounded" || (m != "" && m != "0" && m != "1")
}

func firstChild(n xnode, local string) (xnode, bool) {
	for _, c := range n.Nodes {
		if c.is(local) {
			return c, true
		}
	}
	return xnode{}, false
}

const envelopeHeader = "// Code generated by ubltr-gen from GİB envelope XSD. DO NOT EDIT.\n\n"

func (g *envGen) writeTypes(outDir string) error {
	var b bytes.Buffer
	b.WriteString(envelopeHeader)
	b.WriteString("package envelope\n\nimport \"encoding/xml\"\n\n")

	names := make([]string, 0, len(g.types))
	for n := range g.types {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t := g.types[n]
		fmt.Fprintf(&b, "// %s is generated from envelope XSD %s.\n", t.Name, t.Comment)
		fmt.Fprintf(&b, "type %s struct {\n", t.Name)
		if t.XMLName.Local != "" {
			fmt.Fprintf(&b, "\tXMLName xml.Name `xml:\"%s %s\"`\n", t.XMLName.Space, t.XMLName.Local)
		}
		if t.ResetNS {
			b.WriteString("\t// XMLNS is written as xmlns=\"\" so this unqualified element does not\n")
			b.WriteString("\t// inherit the parent's default namespace. Leave it empty.\n")
			b.WriteString("\tXMLNS string `xml:\"xmlns,attr\"`\n")
		}
		for _, f := range t.Fields {
			switch {
			case f.InnerXML:
				fmt.Fprintf(&b, "\tInnerXML string `xml:\",innerxml\"`\n")
			case f.CharData:
				fmt.Fprintf(&b, "\t%s string `xml:\",chardata\"`\n", f.Name)
			case f.Attr:
				om := ""
				if f.Omit {
					om = ",omitempty"
				}
				fmt.Fprintf(&b, "\t%s string `xml:\"%s,attr%s\"`\n", f.Name, f.Tag, om)
			default:
				goType := f.GoType
				if f.Slice {
					goType = "[]" + goType
				} else if f.Pointer {
					goType = "*" + goType
				}
				om := ""
				if f.Omit {
					om = ",omitempty"
				}
				fmt.Fprintf(&b, "\t%s %s `xml:\"%s%s\"`\n", f.Name, goType, f.Tag, om)
			}
		}
		b.WriteString("}\n\n")
	}

	enumNames := make([]string, 0, len(g.enums))
	for n := range g.enums {
		enumNames = append(enumNames, n)
	}
	sort.Strings(enumNames)
	for _, n := range enumNames {
		fmt.Fprintf(&b, "// %s values from the envelope XSD.\nconst (\n", n)
		for _, v := range g.enums[n] {
			fmt.Fprintf(&b, "\t%s%s = %q\n", n, export(enumIdent(v)), v)
		}
		b.WriteString(")\n\n")
	}

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		_ = os.WriteFile(filepath.Join(outDir, "types.go"), b.Bytes(), 0o644)
		return fmt.Errorf("gofmt envelope: %w", err)
	}
	return os.WriteFile(filepath.Join(outDir, "types.go"), formatted, 0o644)
}

func (g *envGen) writeManifest(outDir string) error {
	var b bytes.Buffer
	b.WriteString(envelopeHeader)
	b.WriteString("package envelope\n\nconst (\n")
	fmt.Fprintf(&b, "\tNamespaceSBDH = %q\n", NSSBDH)
	fmt.Fprintf(&b, "\tNamespacePackage = %q\n", NSPackage)
	fmt.Fprintf(&b, "\tGenerator = %q\n", "github.com/furkanmeclis/go-ubltr/cmd/ubltr-gen")
	files := make([]string, 0, len(g.checksums))
	for f := range g.checksums {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		ident := strings.NewReplacer(".xsd", "", "_", "", "-", "", ".", "").Replace(f)
		fmt.Fprintf(&b, "\tChecksum%s = %q\n", export(ident), g.checksums[f])
	}
	b.WriteString(")\n")
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt envelope manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "generated.go"), formatted, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "doc.go"), []byte(`// Package envelope contains Go types generated from the GİB envelope XSDs
// (schemas/ubltr-1.2.1/envelope): the UN/CEFACT Standard Business Document
// Header and the GİB ef:Package.
//
// Documents inside Package/Elements/ElementList are kept as raw XML in
// ElementList.InnerXML. Use ubltr.NewEnvelope and ubltr.EnvelopeDocuments to
// build and read envelopes with typed UBL-TR documents.
package envelope
`), 0o644)
}
