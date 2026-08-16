package codegen

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const xsdNS = "http://www.w3.org/2001/XMLSchema"

type xsdSchema struct {
	XMLName         xml.Name         `xml:"http://www.w3.org/2001/XMLSchema schema"`
	TargetNamespace string           `xml:"targetNamespace,attr"`
	Imports         []xsdImport      `xml:"http://www.w3.org/2001/XMLSchema import"`
	Includes        []xsdInclude     `xml:"http://www.w3.org/2001/XMLSchema include"`
	Elements        []xsdElement     `xml:"http://www.w3.org/2001/XMLSchema element"`
	ComplexTypes    []xsdComplexType `xml:"http://www.w3.org/2001/XMLSchema complexType"`
	SimpleTypes     []xsdSimpleType  `xml:"http://www.w3.org/2001/XMLSchema simpleType"`
}

type xsdImport struct {
	Namespace      string `xml:"namespace,attr"`
	SchemaLocation string `xml:"schemaLocation,attr"`
}

type xsdInclude struct {
	SchemaLocation string `xml:"schemaLocation,attr"`
}

type xsdElement struct {
	Name      string `xml:"name,attr"`
	Type      string `xml:"type,attr"`
	Ref       string `xml:"ref,attr"`
	MinOccurs string `xml:"minOccurs,attr"`
	MaxOccurs string `xml:"maxOccurs,attr"`
}

type xsdComplexType struct {
	Name           string            `xml:"name,attr"`
	Sequence       *xsdSequence      `xml:"http://www.w3.org/2001/XMLSchema sequence"`
	Choice         *xsdSequence      `xml:"http://www.w3.org/2001/XMLSchema choice"`
	SimpleContent  *xsdSimpleContent `xml:"http://www.w3.org/2001/XMLSchema simpleContent"`
	ComplexContent *xsdSimpleContent `xml:"http://www.w3.org/2001/XMLSchema complexContent"`
	Attributes     []xsdAttribute    `xml:"http://www.w3.org/2001/XMLSchema attribute"`
}

type xsdSequence struct {
	Elements []xsdElement `xml:"http://www.w3.org/2001/XMLSchema element"`
	Anys     []xsdAny     `xml:"http://www.w3.org/2001/XMLSchema any"`
}

type xsdSimpleContent struct {
	Extension   *xsdExtension   `xml:"http://www.w3.org/2001/XMLSchema extension"`
	Restriction *xsdRestriction `xml:"http://www.w3.org/2001/XMLSchema restriction"`
}

type xsdExtension struct {
	Base       string         `xml:"base,attr"`
	Attributes []xsdAttribute `xml:"http://www.w3.org/2001/XMLSchema attribute"`
	Sequence   *xsdSequence   `xml:"http://www.w3.org/2001/XMLSchema sequence"`
}

type xsdRestriction struct {
	Base       string         `xml:"base,attr"`
	Attributes []xsdAttribute `xml:"http://www.w3.org/2001/XMLSchema attribute"`
}

type xsdAttribute struct {
	Name string `xml:"name,attr"`
	Type string `xml:"type,attr"`
	Use  string `xml:"use,attr"`
	Ref  string `xml:"ref,attr"`
}

type xsdAny struct {
	Namespace string `xml:"namespace,attr"`
	MinOccurs string `xml:"minOccurs,attr"`
	MaxOccurs string `xml:"maxOccurs,attr"`
}

type xsdSimpleType struct {
	Name        string          `xml:"name,attr"`
	Restriction *xsdRestriction `xml:"http://www.w3.org/2001/XMLSchema restriction"`
}

type loadedSchema struct {
	Path     string
	TargetNS string
	Prefixes map[string]string
	Elements map[string]xsdElement
	Types    map[string]xsdComplexType
	Simple   map[string]xsdSimpleType
	Imports  []xsdImport
	Includes []xsdInclude
}

type set struct {
	byNS map[string]*loadedSchema
	// key: ns+"\x00"+local
	elements map[string]elemRef
	types    map[string]typeRef
}

type elemRef struct {
	NS      string
	Name    string
	TypeNS  string
	Type    string
	Element xsdElement
}

type typeRef struct {
	NS   string
	Name string
	Type xsdComplexType
}

func loadSet(entry string) (*set, error) {
	s := &set{
		byNS:     map[string]*loadedSchema{},
		elements: map[string]elemRef{},
		types:    map[string]typeRef{},
	}
	if err := s.loadFile(entry, ""); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *set) loadFile(path, expectNS string) error {
	path = filepath.Clean(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read schema %s: %w", path, err)
	}
	var raw xsdSchema
	if err := xml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse schema %s: %w", path, err)
	}
	if expectNS != "" && raw.TargetNamespace != "" && raw.TargetNamespace != expectNS {
		return fmt.Errorf("schema %s: targetNamespace %q != import %q", path, raw.TargetNamespace, expectNS)
	}

	ls := &loadedSchema{
		Path:     path,
		TargetNS: raw.TargetNamespace,
		Prefixes: xmlnsPrefixes(data),
		Elements: map[string]xsdElement{},
		Types:    map[string]xsdComplexType{},
		Simple:   map[string]xsdSimpleType{},
		Imports:  raw.Imports,
		Includes: raw.Includes,
	}
	for _, el := range raw.Elements {
		ls.Elements[el.Name] = el
	}
	for _, t := range raw.ComplexTypes {
		ls.Types[t.Name] = t
	}
	for _, t := range raw.SimpleTypes {
		ls.Simple[t.Name] = t
	}

	existing := s.byNS[ls.TargetNS]
	if existing == nil {
		s.byNS[ls.TargetNS] = ls
	} else {
		for k, v := range ls.Elements {
			existing.Elements[k] = v
		}
		for k, v := range ls.Types {
			existing.Types[k] = v
		}
		for k, v := range ls.Simple {
			existing.Simple[k] = v
		}
		for k, v := range ls.Prefixes {
			if _, ok := existing.Prefixes[k]; !ok {
				existing.Prefixes[k] = v
			}
		}
		ls = existing
	}

	for name, el := range ls.Elements {
		typeNS, typeName := ls.resolve(el.Type)
		if el.Type == "" {
			typeNS, typeName = ls.TargetNS, name+"Type"
		}
		s.elements[key(ls.TargetNS, name)] = elemRef{
			NS:      ls.TargetNS,
			Name:    name,
			TypeNS:  typeNS,
			Type:    typeName,
			Element: el,
		}
	}
	for name, t := range ls.Types {
		s.types[key(ls.TargetNS, name)] = typeRef{NS: ls.TargetNS, Name: name, Type: t}
	}

	dir := filepath.Dir(path)
	for _, inc := range raw.Includes {
		if inc.SchemaLocation == "" {
			continue
		}
		if err := s.loadFile(filepath.Join(dir, inc.SchemaLocation), ls.TargetNS); err != nil {
			return err
		}
	}
	for _, imp := range raw.Imports {
		if imp.SchemaLocation == "" {
			continue
		}
		if strings.HasPrefix(imp.SchemaLocation, "http://") || strings.HasPrefix(imp.SchemaLocation, "https://") {
			continue
		}
		if err := s.loadFile(filepath.Join(dir, imp.SchemaLocation), imp.Namespace); err != nil {
			return err
		}
	}
	return nil
}

func (ls *loadedSchema) resolve(qname string) (ns, local string) {
	if qname == "" {
		return "", ""
	}
	prefix, name, ok := strings.Cut(qname, ":")
	if !ok {
		return ls.TargetNS, qname
	}
	if prefix == "xsd" || prefix == "xs" {
		return xsdNS, name
	}
	if uri, ok := ls.Prefixes[prefix]; ok {
		return uri, name
	}
	return prefix, name
}

func xmlnsPrefixes(data []byte) map[string]string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	out := map[string]string{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, a := range se.Attr {
			switch {
			case a.Name.Space == "xmlns":
				out[a.Name.Local] = a.Value
			case a.Name.Space == "" && a.Name.Local == "xmlns":
				out[""] = a.Value
			}
		}
		return out
	}
}

func key(ns, local string) string {
	return ns + "\x00" + local
}
