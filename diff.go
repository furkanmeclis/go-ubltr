package ubltr

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Change is one structural XML difference. Path uses element local names.
type Change struct {
	Path  string
	Left  string
	Right string
}

func (c Change) String() string {
	return fmt.Sprintf("%s: %q → %q", c.Path, c.Left, c.Right)
}

// Diff compares two XML documents as element trees. It is a developer tool,
// not a GİB submission check. Namespace prefixes are ignored; URIs are not.
func Diff(left, right []byte) ([]Change, error) {
	a, err := parseXMLTree(left)
	if err != nil {
		return nil, fmt.Errorf("diff left: %w", err)
	}
	b, err := parseXMLTree(right)
	if err != nil {
		return nil, fmt.Errorf("diff right: %w", err)
	}
	var out []Change
	diffNodes("/"+a.Local, a, b, &out)
	return out, nil
}

type xmlNode struct {
	Space, Local string
	Attrs        []xml.Attr
	Text         string
	Kids         []*xmlNode
}

func parseXMLTree(data []byte) (*xmlNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("no root element")
			}
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		return parseElement(dec, se)
	}
}

func parseElement(dec *xml.Decoder, se xml.StartElement) (*xmlNode, error) {
	n := &xmlNode{
		Space: se.Name.Space,
		Local: se.Name.Local,
		Attrs: filterAttrs(se.Attr),
		Kids:  []*xmlNode{},
	}
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			kid, err := parseElement(dec, t)
			if err != nil {
				return nil, err
			}
			n.Kids = append(n.Kids, kid)
		case xml.EndElement:
			n.Text = strings.TrimSpace(text.String())
			return n, nil
		case xml.CharData:
			text.Write(t)
		}
	}
}

func filterAttrs(attrs []xml.Attr) []xml.Attr {
	out := make([]xml.Attr, 0, len(attrs))
	for _, a := range attrs {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		out = append(out, a)
	}
	return out
}

func diffNodes(path string, left, right *xmlNode, out *[]Change) {
	if left == nil && right == nil {
		return
	}
	if left == nil {
		*out = append(*out, Change{Path: path, Right: right.summary()})
		return
	}
	if right == nil {
		*out = append(*out, Change{Path: path, Left: left.summary()})
		return
	}
	if left.Local != right.Local || left.Space != right.Space {
		*out = append(*out, Change{Path: path, Left: left.summary(), Right: right.summary()})
		return
	}
	if left.Text != right.Text {
		*out = append(*out, Change{Path: path, Left: left.Text, Right: right.Text})
	}
	diffAttrs(path, left.Attrs, right.Attrs, out)

	max := len(left.Kids)
	if len(right.Kids) > max {
		max = len(right.Kids)
	}
	for i := 0; i < max; i++ {
		var lk, rk *xmlNode
		if i < len(left.Kids) {
			lk = left.Kids[i]
		}
		if i < len(right.Kids) {
			rk = right.Kids[i]
		}
		name := "node"
		if lk != nil {
			name = lk.Local
		} else if rk != nil {
			name = rk.Local
		}
		diffNodes(path+"/"+name, lk, rk, out)
	}
}

func diffAttrs(path string, left, right []xml.Attr, out *[]Change) {
	lm := attrMap(left)
	rm := attrMap(right)
	seen := map[string]bool{}
	for k, lv := range lm {
		seen[k] = true
		if rv, ok := rm[k]; !ok || rv != lv {
			*out = append(*out, Change{Path: path + "/@" + k, Left: lv, Right: rv})
		}
	}
	for k, rv := range rm {
		if seen[k] {
			continue
		}
		*out = append(*out, Change{Path: path + "/@" + k, Right: rv})
	}
}

func attrMap(attrs []xml.Attr) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		key := a.Name.Local
		if a.Name.Space != "" {
			key = a.Name.Space + ":" + a.Name.Local
		}
		m[key] = a.Value
	}
	return m
}

func (n *xmlNode) summary() string {
	if n == nil {
		return ""
	}
	if n.Text != "" {
		return n.Local + "=" + n.Text
	}
	return n.Local
}
