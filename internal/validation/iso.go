package validation

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/lestrrat-go/helium"

	"github.com/furkanmeclis/go-ubltr/schemas"
)

const (
	isoSchematronNS = "http://purl.oclc.org/dsdl/schematron"
	schematronDir   = "ubltr-1.2.1/schematron"
	mainSchematron  = "ubltr-1.2.1/schematron/UBL-TR_Main_Schematron.xml"
	xsdNamespace    = "http://www.w3.org/2001/XMLSchema"
)

type isoLet struct {
	name  string
	value string
}

type isoTest struct {
	ruleID  string
	expr    string
	message []isoMsgPart
}

type isoMsgPart struct {
	text    string
	valueOf string
	isName  bool
}

type isoRule struct {
	id       string
	context  string
	abstract bool
	tests    []isoTest
	lets     []isoLet
}

type isoPattern struct {
	id    string
	rules []isoRule
}

type isoSchema struct {
	ns         map[string]string
	schemaLets []isoLet
	codeLets   []isoLet
	abstracts  map[string]isoRule
	patterns   []isoPattern
	loose      []isoRule
}

func loadISO(ctx context.Context) (*isoSchema, error) {
	data, err := fs.ReadFile(schemas.FS, mainSchematron)
	if err != nil {
		return nil, fmt.Errorf("read main schematron: %w", err)
	}
	doc, err := helium.NewParser().Parse(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("parse main schematron: %w", err)
	}
	root := doc.DocumentElement()
	if root == nil {
		return nil, fmt.Errorf("main schematron has no root")
	}

	out := &isoSchema{
		ns:        map[string]string{"xs": xsdNamespace},
		abstracts: map[string]isoRule{},
	}
	if err := walkSchema(ctx, root, schematronDir, out); err != nil {
		return nil, err
	}
	return out, nil
}

func walkSchema(ctx context.Context, root *helium.Element, dir string, out *isoSchema) error {
	for el := range helium.ChildElements(root) {
		if !inISO(el) {
			continue
		}
		switch el.LocalName() {
		case "include":
			if err := includeFragment(ctx, dir, attr(el, "href"), out); err != nil {
				return err
			}
		case "ns":
			prefix := attr(el, "prefix")
			uri := attr(el, "uri")
			if prefix != "" && uri != "" {
				out.ns[prefix] = uri
			}
		case "let":
			if name, value := attr(el, "name"), attr(el, "value"); name != "" {
				out.schemaLets = append(out.schemaLets, isoLet{name: name, value: value})
			}
		case "pattern":
			if err := ingestPattern(el, out); err != nil {
				return err
			}
		case "rule":
			rule := parseRule(el)
			if rule.context != "" && !rule.abstract {
				out.loose = append(out.loose, rule)
			}
		}
	}
	return nil
}

func includeFragment(ctx context.Context, dir, href string, out *isoSchema) error {
	file, frag, ok := strings.Cut(href, "#")
	if !ok || file == "" || frag == "" {
		return fmt.Errorf("unsupported schematron include %q", href)
	}
	rel := path.Join(dir, file)
	data, err := fs.ReadFile(schemas.FS, rel)
	if err != nil {
		return fmt.Errorf("read include %s: %w", rel, err)
	}
	doc, err := helium.NewParser().Parse(ctx, data)
	if err != nil {
		return fmt.Errorf("parse include %s: %w", rel, err)
	}
	target := findByID(doc.DocumentElement(), frag)
	if target == nil {
		return fmt.Errorf("include %s: id %q not found", rel, frag)
	}
	if !inISO(target) {
		return fmt.Errorf("include %s: target %q is not a schematron element", rel, frag)
	}
	switch target.LocalName() {
	case "pattern":
		return ingestPattern(target, out)
	case "schema":
		return walkSchema(ctx, target, path.Dir(rel), out)
	default:
		return fmt.Errorf("include %s: unsupported target %s", rel, target.LocalName())
	}
}

func ingestPattern(el *helium.Element, out *isoSchema) error {
	id := attr(el, "id")
	pat := isoPattern{id: id}
	for child := range helium.ChildElements(el) {
		if !inISO(child) {
			continue
		}
		switch child.LocalName() {
		case "let":
			if name, value := attr(child, "name"), attr(child, "value"); name != "" {
				out.codeLets = append(out.codeLets, isoLet{name: name, value: value})
			}
		case "rule":
			rule := parseRule(child)
			if rule.abstract {
				if rule.id == "" {
					return fmt.Errorf("abstract rule missing id")
				}
				out.abstracts[rule.id] = rule
				continue
			}
			if rule.context == "" {
				continue
			}
			pat.rules = append(pat.rules, rule)
		}
	}
	if id == "codes" || id == "abstracts" {
		return nil
	}
	if len(pat.rules) > 0 {
		out.patterns = append(out.patterns, pat)
	}
	return nil
}

func parseRule(el *helium.Element) isoRule {
	rule := isoRule{
		id:       attr(el, "id"),
		context:  attr(el, "context"),
		abstract: attr(el, "abstract") == "true",
		tests:    []isoTest{},
		lets:     []isoLet{},
	}
	for child := range helium.ChildElements(el) {
		if !inISO(child) {
			continue
		}
		switch child.LocalName() {
		case "let":
			if name, value := attr(child, "name"), attr(child, "value"); name != "" {
				rule.lets = append(rule.lets, isoLet{name: name, value: value})
			}
		case "extends":
			rule.tests = append(rule.tests, isoTest{ruleID: attr(child, "rule")})
		case "assert", "report":
			rule.tests = append(rule.tests, parseTest(child, rule.id))
		}
	}
	return rule
}

func parseTest(el *helium.Element, ruleID string) isoTest {
	t := isoTest{
		ruleID:  ruleID,
		expr:    attr(el, "test"),
		message: []isoMsgPart{},
	}
	for child := range helium.Children(el) {
		switch child.Type() {
		case helium.TextNode:
			t.message = append(t.message, isoMsgPart{text: string(child.Content())})
		case helium.ElementNode:
			cel, ok := helium.AsNode[*helium.Element](child)
			if !ok || !inISO(cel) {
				continue
			}
			switch cel.LocalName() {
			case "value-of":
				t.message = append(t.message, isoMsgPart{valueOf: attr(cel, "select")})
			case "name":
				t.message = append(t.message, isoMsgPart{isName: true, valueOf: attr(cel, "path")})
			}
		}
	}
	return t
}

func expandAbstracts(in *isoSchema) *isoSchema {
	expand := func(rules []isoRule) []isoRule {
		out := make([]isoRule, 0, len(rules))
		for _, rule := range rules {
			var tests []isoTest
			for _, t := range rule.tests {
				if t.expr != "" || t.ruleID == "" {
					if t.expr != "" {
						tests = append(tests, t)
					}
					continue
				}
				abs, ok := in.abstracts[t.ruleID]
				if !ok {
					continue
				}
				for _, at := range abs.tests {
					at.ruleID = t.ruleID
					tests = append(tests, at)
				}
				rule.lets = append(rule.lets, abs.lets...)
			}
			rule.tests = tests
			if len(rule.tests) == 0 {
				continue
			}
			out = append(out, rule)
		}
		return out
	}

	out := *in
	out.patterns = make([]isoPattern, 0, len(in.patterns)+1)
	for _, p := range in.patterns {
		p.rules = expand(p.rules)
		if len(p.rules) > 0 {
			out.patterns = append(out.patterns, p)
		}
	}
	loose := expand(in.loose)
	if len(loose) > 0 {
		out.patterns = append(out.patterns, isoPattern{id: "general", rules: loose})
	}
	return &out
}

func findByID(root *helium.Element, id string) *helium.Element {
	if root == nil {
		return nil
	}
	var found *helium.Element
	var walk func(*helium.Element)
	walk = func(el *helium.Element) {
		if found != nil {
			return
		}
		if attr(el, "id") == id {
			found = el
			return
		}
		for child := range helium.ChildElements(el) {
			walk(child)
		}
	}
	walk(root)
	return found
}

func inISO(el *helium.Element) bool {
	ns := el.Namespace()
	return ns != nil && ns.URI() == isoSchematronNS
}

func attr(el *helium.Element, name string) string {
	a, ok := el.FindAttribute(helium.NSPredicate{Local: name, NamespaceURI: ""})
	if !ok {
		return ""
	}
	return a.Value()
}
