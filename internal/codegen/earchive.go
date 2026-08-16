package codegen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	NSEarchive = "http://earsiv.efatura.gov.tr"
	NSXMLDSig  = "http://www.w3.org/2000/09/xmldsig#"
)

type eaSchema struct {
	XMLName         xml.Name        `xml:"http://www.w3.org/2001/XMLSchema schema"`
	TargetNamespace string          `xml:"targetNamespace,attr"`
	Elements        []eaElement     `xml:"http://www.w3.org/2001/XMLSchema element"`
	ComplexTypes    []eaComplexType `xml:"http://www.w3.org/2001/XMLSchema complexType"`
	SimpleTypes     []eaSimpleType  `xml:"http://www.w3.org/2001/XMLSchema simpleType"`
}

type eaElement struct {
	Name        string         `xml:"name,attr"`
	Type        string         `xml:"type,attr"`
	Ref         string         `xml:"ref,attr"`
	MinOccurs   string         `xml:"minOccurs,attr"`
	MaxOccurs   string         `xml:"maxOccurs,attr"`
	ComplexType *eaComplexType `xml:"http://www.w3.org/2001/XMLSchema complexType"`
	SimpleType  *eaSimpleType  `xml:"http://www.w3.org/2001/XMLSchema simpleType"`
}

type eaComplexType struct {
	Name          string           `xml:"name,attr"`
	Sequence      *eaParticle      `xml:"http://www.w3.org/2001/XMLSchema sequence"`
	Choice        *eaParticle      `xml:"http://www.w3.org/2001/XMLSchema choice"`
	SimpleContent *eaSimpleContent `xml:"http://www.w3.org/2001/XMLSchema simpleContent"`
	Attributes    []eaAttribute    `xml:"http://www.w3.org/2001/XMLSchema attribute"`
}

type eaParticle struct {
	MinOccurs string    `xml:"minOccurs,attr"`
	MaxOccurs string    `xml:"maxOccurs,attr"`
	Children  []eaChild `xml:",any"`
}

// eaChild is one sequence/choice child, kept in document order.
type eaChild struct {
	XMLName   xml.Name
	Name      string `xml:"name,attr"`
	Type      string `xml:"type,attr"`
	Ref       string `xml:"ref,attr"`
	MinOccurs string `xml:"minOccurs,attr"`
	MaxOccurs string `xml:"maxOccurs,attr"`
	InnerXML  string `xml:",innerxml"`
}

type eaSimpleContent struct {
	Extension *eaExtension `xml:"http://www.w3.org/2001/XMLSchema extension"`
}

type eaExtension struct {
	Base       string        `xml:"base,attr"`
	Attributes []eaAttribute `xml:"http://www.w3.org/2001/XMLSchema attribute"`
}

type eaAttribute struct {
	Name       string        `xml:"name,attr"`
	Type       string        `xml:"type,attr"`
	Use        string        `xml:"use,attr"`
	SimpleType *eaSimpleType `xml:"http://www.w3.org/2001/XMLSchema simpleType"`
}

type eaSimpleType struct {
	Name        string         `xml:"name,attr"`
	Restriction *eaRestriction `xml:"http://www.w3.org/2001/XMLSchema restriction"`
	Union       *eaUnion       `xml:"http://www.w3.org/2001/XMLSchema union"`
}

type eaRestriction struct {
	Base         string   `xml:"base,attr"`
	Enumerations []eaEnum `xml:"http://www.w3.org/2001/XMLSchema enumeration"`
}

type eaEnum struct {
	Value string `xml:"value,attr"`
}

type eaUnion struct {
	MemberTypes string `xml:"memberTypes,attr"`
}

type eaGoType struct {
	Name    string
	Kind    string // struct, simplecontent
	Fields  []eaField
	Comment string
}

type eaField struct {
	Name      string
	GoType    string
	XMLNS     string
	XMLName   string
	Attr      bool
	CharData  bool
	Slice     bool
	Pointer   bool
	Omitempty bool
	InnerXML  bool
}

// GenerateEArchive writes Go types for the GİB e-Arşiv report XSD (EArsiv.xsd).
// This is not a UBL Invoice schema. ds:Signature is InnerXML; XAdES types are not generated.
func GenerateEArchive(schemaPath, outDir string) error {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read earchive xsd: %w", err)
	}
	var raw eaSchema
	if err := xml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse earchive xsd: %w", err)
	}

	g := &eaGen{
		schema:   raw,
		prefixes: xmlnsPrefixes(data),
		types:    map[string]*eaGoType{},
		used:     map[string]bool{},
		enums:    map[string][]string{},
		checksum: map[string]string{},
		outDir:   outDir,
	}
	if err := g.hashFile(schemaPath); err != nil {
		return err
	}

	for _, st := range raw.SimpleTypes {
		g.noteSimple(st)
	}
	for _, ct := range raw.ComplexTypes {
		g.ensureNamed(ct.Name, ct, "")
	}
	for _, el := range raw.Elements {
		if el.Name == "eArsivRaporu" {
			g.ensureElementType("EArsivRaporu", el, "")
		}
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := g.writeTypes(); err != nil {
		return err
	}
	return g.writeManifest()
}

type eaGen struct {
	schema   eaSchema
	prefixes map[string]string
	types    map[string]*eaGoType
	used     map[string]bool
	enums    map[string][]string
	checksum map[string]string
	outDir   string
}

func (g *eaGen) hashFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	g.checksum[filepath.Base(path)] = hex.EncodeToString(sum[:])
	return nil
}

func (g *eaGen) noteSimple(st eaSimpleType) {
	if st.Name == "" {
		return
	}
	if st.Restriction != nil && len(st.Restriction.Enumerations) > 0 {
		vals := make([]string, 0, len(st.Restriction.Enumerations))
		for _, e := range st.Restriction.Enumerations {
			vals = append(vals, e.Value)
		}
		g.enums[st.Name] = vals
	}
}

func (g *eaGen) ensureNamed(name string, ct eaComplexType, parent string) string {
	goName := exportEA(name)
	if g.types[goName] != nil {
		return goName
	}
	g.used[goName] = true
	if ct.SimpleContent != nil {
		g.types[goName] = g.simpleContentType(goName, name, ct)
		return goName
	}
	t := &eaGoType{Name: goName, Kind: "struct", Comment: name, Fields: []eaField{}}
	g.types[goName] = t
	g.addParticles(t, ct.Sequence, parent+"/"+name, false)
	g.addParticles(t, ct.Choice, parent+"/"+name, true)
	g.addAttributes(t, ct.Attributes)
	return goName
}

func (g *eaGen) ensureElementType(preferred string, el eaElement, parent string) string {
	if el.ComplexType == nil {
		return g.goFieldType(el.Type)
	}
	name := uniqueEA(preferred, g.used)
	g.used[name] = true
	ct := *el.ComplexType
	ct.Name = name
	return g.ensureNamed(name, ct, parent)
}

func (g *eaGen) addParticles(t *eaGoType, p *eaParticle, parent string, inChoice bool) {
	if p == nil {
		return
	}
	unbounded := p.MaxOccurs == "unbounded" || (p.MaxOccurs != "" && p.MaxOccurs != "1" && p.MaxOccurs != "0")
	for _, c := range p.Children {
		switch c.XMLName.Local {
		case "element":
			g.addField(t, parseEAElement(c), parent, unbounded, inChoice)
		case "choice":
			ch := parseEAParticle(c)
			g.addParticles(t, &ch, parent, true)
		case "sequence":
			seq := parseEAParticle(c)
			g.addParticles(t, &seq, parent, inChoice)
		}
	}
}

func parseEAElement(c eaChild) eaElement {
	var b strings.Builder
	b.WriteString(`<element xmlns="http://www.w3.org/2001/XMLSchema" xmlns:xs="http://www.w3.org/2001/XMLSchema"`)
	writeAttr(&b, "name", c.Name)
	writeAttr(&b, "type", c.Type)
	writeAttr(&b, "ref", c.Ref)
	writeAttr(&b, "minOccurs", c.MinOccurs)
	writeAttr(&b, "maxOccurs", c.MaxOccurs)
	b.WriteByte('>')
	b.WriteString(c.InnerXML)
	b.WriteString(`</element>`)
	var el eaElement
	if err := xml.Unmarshal([]byte(b.String()), &el); err != nil {
		return eaElement{
			Name:      c.Name,
			Type:      c.Type,
			Ref:       c.Ref,
			MinOccurs: c.MinOccurs,
			MaxOccurs: c.MaxOccurs,
		}
	}
	return el
}

func parseEAParticle(c eaChild) eaParticle {
	var b strings.Builder
	b.WriteString(`<sequence xmlns="http://www.w3.org/2001/XMLSchema" xmlns:xs="http://www.w3.org/2001/XMLSchema"`)
	writeAttr(&b, "minOccurs", c.MinOccurs)
	writeAttr(&b, "maxOccurs", c.MaxOccurs)
	b.WriteByte('>')
	b.WriteString(c.InnerXML)
	b.WriteString(`</sequence>`)
	var p eaParticle
	if err := xml.Unmarshal([]byte(b.String()), &p); err != nil {
		return eaParticle{MinOccurs: c.MinOccurs, MaxOccurs: c.MaxOccurs}
	}
	p.MinOccurs = c.MinOccurs
	p.MaxOccurs = c.MaxOccurs
	return p
}

func writeAttr(b *strings.Builder, name, value string) {
	if value == "" {
		return
	}
	b.WriteByte(' ')
	b.WriteString(name)
	b.WriteString(`="`)
	b.WriteString(xmlEscape(value))
	b.WriteByte('"')
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return s
}

func (g *eaGen) addAttributes(t *eaGoType, attrs []eaAttribute) {
	for _, a := range attrs {
		if a.Name == "" {
			continue
		}
		t.Fields = append(t.Fields, eaField{
			Name:      exportEA(a.Name),
			GoType:    "string",
			XMLName:   a.Name,
			Attr:      true,
			Omitempty: a.Use != "required",
		})
	}
}

func (g *eaGen) addField(t *eaGoType, el eaElement, parent string, parentUnbounded, inChoice bool) {
	if el.Ref != "" {
		ns, local := g.resolve(el.Ref)
		if ns == NSXMLDSig && local == "Signature" {
			t.Fields = append(t.Fields, eaField{
				Name:    "Signature",
				GoType:  "XMLSignature",
				XMLNS:   NSXMLDSig,
				XMLName: "Signature",
			})
			g.ensureSignatureType()
			return
		}
		return
	}
	if el.Name == "" {
		return
	}

	optional := el.MinOccurs == "0" || inChoice
	repeated := parentUnbounded || el.MaxOccurs == "unbounded" || (el.MaxOccurs != "" && el.MaxOccurs != "1" && el.MaxOccurs != "0")
	fieldName := exportEA(el.Name)
	var goType string
	switch {
	case el.ComplexType != nil:
		preferred := fieldName
		if g.types[preferred] != nil || g.used[preferred] {
			preferred = t.Name + fieldName
		}
		goType = g.ensureElementType(preferred, el, parent+"/"+el.Name)
	case el.SimpleType != nil:
		goType = "string"
		if el.SimpleType.Restriction != nil && len(el.SimpleType.Restriction.Enumerations) > 0 {
			g.enums[el.Name+"Enum"] = enumValues(*el.SimpleType)
		}
	default:
		goType = g.goFieldType(el.Type)
	}

	t.Fields = append(t.Fields, eaField{
		Name:      fieldName,
		GoType:    goType,
		XMLNS:     NSEarchive,
		XMLName:   el.Name,
		Slice:     repeated,
		Pointer:   !repeated && optional && goType != "string",
		Omitempty: optional || repeated,
	})
}

func enumValues(st eaSimpleType) []string {
	if st.Restriction == nil {
		return nil
	}
	out := make([]string, 0, len(st.Restriction.Enumerations))
	for _, e := range st.Restriction.Enumerations {
		out = append(out, e.Value)
	}
	return out
}

func (g *eaGen) goFieldType(qname string) string {
	if qname == "" {
		return "string"
	}
	ns, local := g.resolve(qname)
	if ns == xsdNS || ns == "" {
		return "string"
	}
	if ns == NSEarchive {
		if _, ok := g.schemaNamedComplex(local); ok {
			return exportEA(local)
		}
	}
	return "string"
}

func (g *eaGen) schemaNamedComplex(name string) (eaComplexType, bool) {
	for _, ct := range g.schema.ComplexTypes {
		if ct.Name == name {
			return ct, true
		}
	}
	return eaComplexType{}, false
}

func (g *eaGen) simpleContentType(goName, xsdName string, ct eaComplexType) *eaGoType {
	t := &eaGoType{Name: goName, Kind: "simplecontent", Comment: xsdName, Fields: []eaField{}}
	if ct.SimpleContent != nil && ct.SimpleContent.Extension != nil {
		for _, a := range ct.SimpleContent.Extension.Attributes {
			omitempty := a.Use != "required"
			t.Fields = append(t.Fields, eaField{
				Name:      exportEA(a.Name),
				GoType:    "string",
				XMLName:   a.Name,
				Attr:      true,
				Omitempty: omitempty,
			})
		}
	}
	t.Fields = append(t.Fields, eaField{Name: "Value", GoType: "string", CharData: true})
	return t
}

func (g *eaGen) ensureSignatureType() {
	if g.types["XMLSignature"] != nil {
		return
	}
	g.types["XMLSignature"] = &eaGoType{
		Name:    "XMLSignature",
		Kind:    "struct",
		Comment: "ds:Signature (XAdES payload kept as InnerXML; not a generated XAdES model)",
		Fields: []eaField{
			{Name: "InnerXML", GoType: "string", InnerXML: true},
		},
	}
}

func (g *eaGen) resolve(qname string) (ns, local string) {
	if qname == "" {
		return "", ""
	}
	prefix, name, ok := strings.Cut(qname, ":")
	if !ok {
		return g.schema.TargetNamespace, qname
	}
	if prefix == "xs" || prefix == "xsd" {
		return xsdNS, name
	}
	if uri, ok := g.prefixes[prefix]; ok {
		return uri, name
	}
	return prefix, name
}

func (g *eaGen) writeTypes() error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by ubltr-gen from GİB EArsiv.xsd. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package earchive\n\n")
	fmt.Fprintf(&b, "import \"encoding/xml\"\n\n")

	names := make([]string, 0, len(g.types))
	for n := range g.types {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g.writeGoType(&b, g.types[n])
	}

	enumNames := make([]string, 0, len(g.enums))
	for n := range g.enums {
		enumNames = append(enumNames, n)
	}
	sort.Strings(enumNames)
	for _, n := range enumNames {
		vals := g.enums[n]
		if len(vals) == 0 {
			continue
		}
		prefix := enumPrefix(n)
		fmt.Fprintf(&b, "// %s values from EArsiv.xsd.\n", n)
		fmt.Fprintf(&b, "const (\n")
		for _, v := range vals {
			ident := prefix + enumIdent(v)
			fmt.Fprintf(&b, "\t%s = %q\n", ident, v)
		}
		fmt.Fprintf(&b, ")\n\n")
	}

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		_ = os.WriteFile(filepath.Join(g.outDir, "types.go"), b.Bytes(), 0o644)
		return fmt.Errorf("gofmt earchive: %w", err)
	}
	return os.WriteFile(filepath.Join(g.outDir, "types.go"), formatted, 0o644)
}

func (g *eaGen) writeGoType(b *bytes.Buffer, t *eaGoType) {
	fmt.Fprintf(b, "// %s is generated from EArsiv.xsd %s.\n", t.Name, t.Comment)
	fmt.Fprintf(b, "type %s struct {\n", t.Name)
	if t.Name == "EArsivRaporu" {
		fmt.Fprintf(b, "\tXMLName xml.Name `xml:\"%s eArsivRaporu\"`\n", NSEarchive)
	}
	for _, f := range t.Fields {
		goType := f.GoType
		switch {
		case f.InnerXML && f.Name == "InnerXML":
			fmt.Fprintf(b, "\tInnerXML string `xml:\",innerxml\"`\n")
			continue
		case f.CharData:
			fmt.Fprintf(b, "\t%s string `xml:\",chardata\"`\n", f.Name)
			continue
		case f.Attr:
			om := ""
			if f.Omitempty {
				om = ",omitempty"
			}
			fmt.Fprintf(b, "\t%s string `xml:\"%s,attr%s\"`\n", f.Name, f.XMLName, om)
			continue
		}
		if f.Slice {
			goType = "[]" + goType
		} else if f.Pointer {
			goType = "*" + goType
		}
		tag := f.XMLNS + " " + f.XMLName
		if f.Omitempty {
			fmt.Fprintf(b, "\t%s %s `xml:\"%s,omitempty\"`\n", f.Name, goType, tag)
		} else {
			fmt.Fprintf(b, "\t%s %s `xml:\"%s\"`\n", f.Name, goType, tag)
		}
	}
	fmt.Fprintf(b, "}\n\n")
}

func (g *eaGen) writeManifest() error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by ubltr-gen from GİB EArsiv.xsd. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package earchive\n\n")
	fmt.Fprintf(&b, "const (\n")
	fmt.Fprintf(&b, "\tNamespace = %q\n", NSEarchive)
	fmt.Fprintf(&b, "\tGenerator = %q\n", "github.com/furkanmeclis/go-ubltr/cmd/ubltr-gen")
	keys := make([]string, 0, len(g.checksum))
	for k := range g.checksum {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ident := strings.ReplaceAll(strings.TrimSuffix(k, ".xsd"), "-", "")
		ident = strings.ReplaceAll(ident, ".", "")
		fmt.Fprintf(&b, "\tChecksum%s = %q\n", exportEA(ident), g.checksum[k])
	}
	fmt.Fprintf(&b, ")\n")
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt earchive manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(g.outDir, "generated.go"), formatted, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(g.outDir, "doc.go"), []byte(`// Package earchive contains Go types generated from GİB EArsiv.xsd.
//
// This is the e-Arşiv *report* sent to GİB (root eArsivRaporu). It is not a
// UBL document. e-Arşiv invoices are UBL-TR Invoice with ProfileID EARSIVFATURA
// in package schema/ubltr121/invoice.
package earchive
`), 0o644)
}

func exportEA(name string) string {
	if name == "" {
		return "X"
	}
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	s := string(r)
	if isGoKeyword(strings.ToLower(s)) {
		return s + "_"
	}
	return s
}

func uniqueEA(name string, used map[string]bool) string {
	if !used[name] && name != "" {
		return name
	}
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s%d", name, i)
		if !used[n] {
			return n
		}
	}
}

func enumPrefix(simpleName string) string {
	switch simpleName {
	case "faturaTipEnum":
		return "FaturaTip"
	case "fisTipType":
		return "FisTip"
	case "vergiTipEnum":
		return "VergiTip"
	case "vergiKodEnum":
		return "VergiKod"
	case "odemeTipEnum":
		return "OdemeTip"
	case "receiptScenariosType":
		return "ReceiptScenario"
	case "bilgiFisTipEnum":
		return "BilgiFisTip"
	case "belgeTutarTipEnum":
		return "BelgeTutarTip"
	case "odemeTuruTipEnum":
		return "OdemeTuruTip"
	case "partyIdentificationTypeCodes":
		return "PartyScheme"
	default:
		return exportEA(strings.TrimSuffix(simpleName, "Enum"))
	}
}

func enumIdent(v string) string {
	v = strings.ReplaceAll(v, "/", "_")
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")
	return v
}
