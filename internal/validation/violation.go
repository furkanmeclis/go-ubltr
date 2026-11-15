package validation

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/lestrrat-go/helium"
)

// Validation layers.
const (
	LayerXSD        = "xsd"
	LayerSchematron = "schematron"
)

// Severities. Schematron asserts default to error and reports to warning;
// a role or flag attribute on the assert, report, or rule overrides that.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Violation is a single schema or business-rule failure.
type Violation struct {
	Layer    string // "xsd" or "schematron"
	Path     string // element name (schematron) or element:line (xsd)
	RuleID   string
	Severity string
	Message  string

	// TestID is the id attribute of the failing assert or report, if any.
	TestID string
	// PatternID is the Schematron pattern that holds the rule.
	PatternID string
	// Context is the Schematron rule context expression.
	Context string
	// Test is the failing Schematron XPath test.
	Test string
	// Role and Flag are the raw Schematron attributes, if any.
	Role string
	Flag string
	// Location is an XPath-like path to the failing node, for example
	// /inv:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification[2].
	Location string
	// Line is the 1-based line of the failing node, or 0 when unknown.
	Line int
	// Element is the qualified name of the failing element.
	Element string
	// Attribute is the failing attribute name, if any.
	Attribute string
}

func (v Violation) String() string {
	if v.Path != "" {
		return fmt.Sprintf("%s: %s (%s)", v.Layer, v.Message, v.Path)
	}
	return fmt.Sprintf("%s: %s", v.Layer, v.Message)
}

// Result is the outcome of one or more validation layers.
type Result struct {
	Violations []Violation
}

// Valid reports whether validation produced no violations.
func (r Result) Valid() bool {
	return len(r.Violations) == 0
}

// severityOf maps Schematron role/flag values to a severity.
func severityOf(report bool, role, flag string) string {
	for _, s := range []string{role, flag} {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "fatal", "error", "err":
			return SeverityError
		case "warning", "warn":
			return SeverityWarning
		case "info", "information", "informational", "notice":
			return SeverityInfo
		}
	}
	if report {
		return SeverityWarning
	}
	return SeverityError
}

// nodeLocator builds XPath-like locations using a fixed URI → prefix table.
type nodeLocator struct {
	prefixes map[string]string
}

func newNodeLocator(ns map[string]string) nodeLocator {
	keys := make([]string, 0, len(ns))
	for k := range ns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(ns))
	for _, prefix := range keys {
		uri := ns[prefix]
		if _, ok := out[uri]; !ok {
			out[uri] = prefix
		}
	}
	return nodeLocator{prefixes: out}
}

func (l nodeLocator) qname(n helium.Node) string {
	uri := nodeNamespace(n)
	local := localName(n)
	switch {
	case uri == "":
		return local
	case l.prefixes[uri] != "":
		return l.prefixes[uri] + ":" + local
	default:
		return "Q{" + uri + "}" + local
	}
}

// path returns an XPath-like location for n. Positions are added only when a
// sibling has the same expanded name.
func (l nodeLocator) path(n helium.Node) string {
	if n == nil {
		return ""
	}
	if n.Type() == helium.AttributeNode {
		parent := l.path(n.Parent())
		if parent == "/" {
			parent = ""
		}
		return parent + "/@" + l.qname(n)
	}
	var parts []string
	for cur := n; cur != nil && cur.Type() != helium.DocumentNode; cur = cur.Parent() {
		if cur.Type() != helium.ElementNode {
			continue
		}
		step := l.qname(cur)
		if pos, many := siblingIndex(cur); many {
			step += "[" + strconv.Itoa(pos) + "]"
		}
		parts = append(parts, step)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return "/" + strings.Join(parts, "/")
}

func siblingIndex(n helium.Node) (int, bool) {
	parent := n.Parent()
	if parent == nil {
		return 1, false
	}
	uri, local := nodeNamespace(n), localName(n)
	pos, count := 0, 0
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Type() != helium.ElementNode || localName(c) != local || nodeNamespace(c) != uri {
			continue
		}
		count++
		if c == n {
			pos = count
		}
	}
	return pos, count > 1
}

// nodeLine returns the source line of n, or of its owner element for
// attributes (helium does not record attribute lines).
func nodeLine(n helium.Node) int {
	if l := n.Line(); l > 0 || n.Type() != helium.AttributeNode {
		return l
	}
	if p := n.Parent(); p != nil {
		return p.Line()
	}
	return 0
}

func nodeNamespace(n helium.Node) string {
	if u, ok := n.(interface{ URI() string }); ok {
		return u.URI()
	}
	return ""
}

func localName(n helium.Node) string {
	if l, ok := n.(interface{ LocalName() string }); ok && l.LocalName() != "" {
		return l.LocalName()
	}
	name := n.Name()
	if _, after, ok := strings.Cut(name, ":"); ok {
		return after
	}
	return name
}

// findElementAtLine returns the first element on line whose name matches.
// name is a qualified name (cbc:ID), a local name, or {uri}local as helium
// reports it. It is used to place XSD errors in the tree.
func findElementAtLine(doc *helium.Document, line int, name string) helium.Node {
	if doc == nil || line <= 0 {
		return nil
	}
	uri, local, clark := "", name, false
	if rest, ok := strings.CutPrefix(name, "{"); ok {
		if u, l, ok := strings.Cut(rest, "}"); ok {
			uri, local, clark = u, l, true
		}
	} else if _, after, ok := strings.Cut(name, ":"); ok {
		local = after
	}
	match := func(c helium.Node) bool {
		switch {
		case name == "":
			return true
		case clark:
			return localName(c) == local && nodeNamespace(c) == uri
		default:
			return c.Name() == name || localName(c) == local
		}
	}
	var found helium.Node
	var walk func(helium.Node)
	walk = func(n helium.Node) {
		for c := n.FirstChild(); c != nil && found == nil; c = c.NextSibling() {
			if c.Type() != helium.ElementNode {
				continue
			}
			if c.Line() == line && match(c) {
				found = c
				return
			}
			walk(c)
		}
	}
	walk(doc)
	return found
}
