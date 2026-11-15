package validation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/schematron"
	"github.com/lestrrat-go/helium/xpath3"

	"github.com/furkanmeclis/go-ubltr/schemas"
)

const (
	TypeEFatura  = "efatura"
	TypeEArchive = "earchive"
)

type compiledLet struct {
	name string
	expr *xpath3.Expression
}

type compiledTest struct {
	ruleID   string
	id       string
	test     string
	report   bool
	role     string
	flag     string
	severity string
	expr     *xpath3.Expression
	message  []compiledMsgPart
}

type compiledMsgPart struct {
	text    string
	valueOf *xpath3.Expression
	isName  bool
}

type compiledRule struct {
	contextText string
	context     *xpath3.Expression
	tests       []compiledTest
	lets        []compiledLet
}

type compiledPattern struct {
	id    string
	rules []compiledRule
}

type compiledSchematron struct {
	ns        map[string]string
	constants map[string]xpath3.Sequence
	docLets   []compiledLet
	patterns  []compiledPattern
	locator   nodeLocator
}

var compileSchematron = sync.OnceValues(func() (*compiledSchematron, error) {
	return compileISO(context.Background())
})

func dummyXML() []byte {
	return []byte("<dummy/>")
}

func compileISO(ctx context.Context) (*compiledSchematron, error) {
	raw, err := loadISO(ctx)
	if err != nil {
		return nil, err
	}
	raw = expandAbstracts(raw)
	xc := xpath3.NewCompiler()
	prefixes := make(map[string]string, len(defaultPrefixes)+len(raw.ns))
	for k, v := range defaultPrefixes {
		prefixes[k] = v
	}
	for k, v := range raw.ns {
		prefixes[k] = v
	}
	out := &compiledSchematron{
		ns:        raw.ns,
		constants: map[string]xpath3.Sequence{},
		docLets:   []compiledLet{},
		patterns:  []compiledPattern{},
		locator:   newNodeLocator(prefixes),
	}

	dummy, err := helium.NewParser().Parse(ctx, dummyXML())
	if err != nil {
		return nil, fmt.Errorf("parse dummy node: %w", err)
	}
	ev := xpath3.NewEvaluator(xpath3.DefaultEvaluatorOptions).Namespaces(raw.ns)

	for _, l := range raw.codeLets {
		seq, err := evalStringLet(ctx, xc, ev, dummy, l)
		if err != nil {
			return nil, fmt.Errorf("codelist let %s: %w", l.name, err)
		}
		out.constants[l.name] = seq
	}
	for _, l := range raw.schemaLets {
		if l.name == "type" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l.value), "'") {
			seq, err := evalStringLet(ctx, xc, ev, dummy, l)
			if err != nil {
				return nil, fmt.Errorf("schema let %s: %w", l.name, err)
			}
			out.constants[l.name] = seq
			continue
		}
		expr, err := xc.Compile(l.value)
		if err != nil {
			return nil, fmt.Errorf("schema let %s: %w", l.name, err)
		}
		out.docLets = append(out.docLets, compiledLet{name: l.name, expr: expr})
	}

	var compileErrs []error
	for _, p := range raw.patterns {
		cp := compiledPattern{id: p.id, rules: []compiledRule{}}
		for _, r := range p.rules {
			cr, err := compileRule(xc, r)
			if err != nil {
				compileErrs = append(compileErrs, err)
				if len(compileErrs) >= 12 {
					return nil, errors.Join(compileErrs...)
				}
				continue
			}
			cp.rules = append(cp.rules, cr)
		}
		if len(cp.rules) > 0 {
			out.patterns = append(out.patterns, cp)
		}
	}
	if len(compileErrs) > 0 {
		return nil, errors.Join(compileErrs...)
	}
	if len(out.patterns) == 0 {
		return nil, fmt.Errorf("flattened schematron has no rules")
	}
	return out, nil
}

func evalStringLet(
	ctx context.Context,
	xc xpath3.Compiler,
	ev xpath3.Evaluator,
	node helium.Node,
	l isoLet,
) (xpath3.Sequence, error) {
	expr, err := xc.Compile(l.value)
	if err != nil {
		return nil, err
	}
	res, err := ev.Evaluate(ctx, expr, node)
	if err != nil {
		return nil, err
	}
	return res.Sequence(), nil
}

func compileRule(xc xpath3.Compiler, r isoRule) (compiledRule, error) {
	ctxExpr, err := xc.Compile(contextToXPath(r.context))
	if err != nil {
		return compiledRule{}, fmt.Errorf("context %q: %w", r.context, err)
	}
	cr := compiledRule{
		contextText: r.context,
		context:     ctxExpr,
		tests:       []compiledTest{},
		lets:        []compiledLet{},
	}
	for _, l := range r.lets {
		expr, err := xc.Compile(l.value)
		if err != nil {
			return compiledRule{}, fmt.Errorf("rule let %s: %w", l.name, err)
		}
		cr.lets = append(cr.lets, compiledLet{name: l.name, expr: expr})
	}
	for _, t := range r.tests {
		if t.expr == "" {
			continue
		}
		expr, err := xc.Compile(t.expr)
		if err != nil {
			return compiledRule{}, fmt.Errorf("assert %s %q: %w", t.ruleID, t.expr, err)
		}
		ct := compiledTest{
			ruleID:   t.ruleID,
			id:       t.id,
			test:     t.expr,
			report:   t.report,
			role:     t.role,
			flag:     t.flag,
			severity: severityOf(t.report, t.role, t.flag),
			expr:     expr,
			message:  []compiledMsgPart{},
		}
		for _, p := range t.message {
			part, err := compileMsgPart(xc, p)
			if err != nil {
				return compiledRule{}, err
			}
			ct.message = append(ct.message, part)
		}
		cr.tests = append(cr.tests, ct)
	}
	if len(cr.tests) == 0 {
		return compiledRule{}, fmt.Errorf("rule %q has no asserts", r.context)
	}
	return cr, nil
}

func compileMsgPart(xc xpath3.Compiler, p isoMsgPart) (compiledMsgPart, error) {
	part := compiledMsgPart{text: p.text, isName: p.isName}
	sel := p.valueOf
	switch {
	case p.isName && sel == "":
		sel = "name(.)"
	case p.isName:
		sel = "name(" + sel + ")"
	case sel != "":
		sel = "string((" + sel + ")[1])"
	default:
		return part, nil
	}
	vexpr, err := xc.Compile(sel)
	if err != nil {
		return part, fmt.Errorf("message xpath %q: %w", sel, err)
	}
	part.valueOf = vexpr
	return part, nil
}

func contextToXPath(context string) string {
	parts := splitTopLevelUnion(context)
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "/") {
			p = "//" + p
		}
		parts[i] = p
	}
	return strings.Join(parts, " | ")
}

func splitTopLevelUnion(s string) []string {
	var parts []string
	depth := 0
	var quote byte
	start := 0
	for i := range len(s) {
		ch := s[i]
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '\'' || ch == '"':
			quote = ch
		case ch == '[' || ch == '(':
			depth++
		case ch == ']' || ch == ')':
			if depth > 0 {
				depth--
			}
		case ch == '|' && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// Schematron validates xmlBytes against the official GİB UBL-TR Schematron rules.
// docType is "efatura" (default), "earchive", or "goruntuleme". Patterns are
// chosen from the detected document kind (see [SchematronPatterns]).
func Schematron(ctx context.Context, xmlBytes []byte, docType string) (Result, error) {
	kind, err := Detect(xmlBytes)
	if err != nil && !errors.Is(err, ErrUnknownDocument) {
		return Result{}, fmt.Errorf("parse xml: %w", err)
	}
	return SchematronFor(ctx, xmlBytes, kind, docType)
}

// patternsByKind lists the GİB Main Schematron pattern ids that apply to each
// document kind. "general" holds the rules outside any pattern (code-list
// checks on currencyID, unitCode, IssueDate, …). Envelopes run every pattern
// because their contexts also match documents inside ef:Package.
var patternsByKind = map[Kind][]string{
	KindInvoice:             {"invoice", "general"},
	KindApplicationResponse: {"applicationresponse", "general"},
	KindDespatchAdvice:      {"despatchadvice", "general"},
	KindReceiptAdvice:       {"receiptadvice", "general"},
}

// HasSchematron reports whether the GİB Main Schematron has rules for kind.
func HasSchematron(kind Kind) bool {
	_, ok := patternsByKind[kind]
	return ok || kind == KindEnvelope
}

// SchematronPatterns returns the pattern ids run for kind. A nil result means
// every pattern runs.
func SchematronPatterns(kind Kind) []string {
	return append([]string(nil), patternsByKind[kind]...)
}

// SchematronFor is [Schematron] with an explicit document kind. An empty or
// unknown kind, or KindEnvelope, runs every pattern.
func SchematronFor(ctx context.Context, xmlBytes []byte, kind Kind, docType string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if docType == "" {
		docType = TypeEFatura
	}

	schema, err := compileSchematron()
	if err != nil {
		return Result{}, fmt.Errorf("compile schematron: %w", err)
	}

	doc, err := helium.NewParser().Parse(ctx, xmlBytes)
	if err != nil {
		return Result{}, fmt.Errorf("parse xml: %w", err)
	}

	vars := make(map[string]xpath3.Sequence, len(schema.constants)+len(schema.docLets)+1)
	for k, v := range schema.constants {
		vars[k] = v
	}
	vars["type"] = xpath3.SingleString(docType)

	base := xpath3.NewEvaluator(xpath3.DefaultEvaluatorOptions).Namespaces(schema.ns)
	docEv := base.Variables(vars)
	for _, l := range schema.docLets {
		res, err := docEv.Evaluate(ctx, l.expr, doc)
		if err != nil {
			return Result{}, fmt.Errorf("evaluate let %s: %w", l.name, err)
		}
		vars[l.name] = res.Sequence()
		docEv = base.Variables(vars)
	}

	var only map[string]bool
	if ids, ok := patternsByKind[kind]; ok {
		only = make(map[string]bool, len(ids))
		for _, id := range ids {
			only[id] = true
		}
	}

	var r Result
	ev := base.Variables(vars)
	for _, p := range schema.patterns {
		if only != nil && !only[p.id] {
			continue
		}
		matched := map[helium.Node]struct{}{}
		for _, rule := range p.rules {
			collectRule(ctx, schema.locator, base, ev, vars, p.id, rule, doc, matched, &r)
		}
	}
	return r, nil
}

func collectRule(
	ctx context.Context,
	loc nodeLocator,
	base, ev xpath3.Evaluator,
	vars map[string]xpath3.Sequence,
	patternID string,
	rule compiledRule,
	doc *helium.Document,
	matched map[helium.Node]struct{},
	r *Result,
) {
	res, err := ev.Evaluate(ctx, rule.context, doc)
	if err != nil {
		r.Violations = append(r.Violations, Violation{
			Layer:     LayerSchematron,
			Severity:  SeverityError,
			RuleID:    patternID,
			PatternID: patternID,
			Context:   rule.contextText,
			Message:   fmt.Sprintf("context xpath: %v", err),
		})
		return
	}
	nodes, err := res.Nodes()
	if err != nil {
		r.Violations = append(r.Violations, Violation{
			Layer:     LayerSchematron,
			Severity:  SeverityError,
			RuleID:    patternID,
			PatternID: patternID,
			Context:   rule.contextText,
			Message:   fmt.Sprintf("context nodes: %v", err),
		})
		return
	}
	for _, node := range nodes {
		if node.Type() != helium.ElementNode && node.Type() != helium.AttributeNode {
			continue
		}
		if _, ok := matched[node]; ok {
			continue
		}
		matched[node] = struct{}{}
		ruleEv := ev
		if len(rule.lets) > 0 {
			ruleVars := cloneVars(vars)
			ruleEv = base.Variables(ruleVars)
			for _, l := range rule.lets {
				letRes, err := ruleEv.Evaluate(ctx, l.expr, node)
				if err != nil {
					r.Violations = append(r.Violations, Violation{
						Layer:     LayerSchematron,
						Severity:  SeverityError,
						RuleID:    patternID,
						PatternID: patternID,
						Context:   rule.contextText,
						Location:  loc.path(node),
						Line:      nodeLine(node),
						Message:   fmt.Sprintf("let %s: %v", l.name, err),
					})
					continue
				}
				ruleVars[l.name] = letRes.Sequence()
				ruleEv = base.Variables(ruleVars)
			}
		}
		for _, t := range rule.tests {
			fireSchematronTest(ctx, loc, ruleEv, patternID, rule.contextText, t, node, r)
		}
	}
}

func fireSchematronTest(
	ctx context.Context,
	loc nodeLocator,
	ev xpath3.Evaluator,
	patternID, ruleContext string,
	t compiledTest,
	node helium.Node,
	r *Result,
) {
	testRes, err := ev.Evaluate(ctx, t.expr, node)
	ok := false
	if err == nil {
		ok, err = xpath3.EBV(testRes.Sequence())
	}
	// An assert fires when its test is false; a report fires when it is true.
	if err == nil && ok != t.report {
		return
	}
	msg := formatMessage(ctx, ev, t.message, node)
	if err != nil && strings.TrimSpace(msg) == "" {
		msg = err.Error()
	}
	v := Violation{
		Layer:     LayerSchematron,
		Severity:  t.severity,
		RuleID:    t.ruleID,
		Path:      nodeName(node),
		Message:   strings.TrimSpace(msg),
		TestID:    t.id,
		PatternID: patternID,
		Context:   ruleContext,
		Test:      t.test,
		Role:      t.role,
		Flag:      t.flag,
		Location:  loc.path(node),
		Line:      nodeLine(node),
	}
	if err != nil {
		v.Severity = SeverityError
	}
	if node.Type() == helium.AttributeNode {
		v.Attribute = loc.qname(node)
		if p := node.Parent(); p != nil {
			v.Element = loc.qname(p)
		}
	} else {
		v.Element = loc.qname(node)
	}
	r.Violations = append(r.Violations, v)
}

func cloneVars(in map[string]xpath3.Sequence) map[string]xpath3.Sequence {
	out := make(map[string]xpath3.Sequence, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func formatMessage(ctx context.Context, ev xpath3.Evaluator, parts []compiledMsgPart, node helium.Node) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.text)
		if p.valueOf == nil {
			continue
		}
		res, err := ev.Evaluate(ctx, p.valueOf, node)
		if err != nil {
			continue
		}
		if s, ok := res.IsString(); ok {
			b.WriteString(s)
			continue
		}
		if nodes, err := res.Nodes(); err == nil && len(nodes) > 0 {
			b.WriteString(nodeName(nodes[0]))
		}
	}
	return strings.TrimSpace(b.String())
}

func nodeName(n helium.Node) string {
	if n == nil {
		return ""
	}
	if el, ok := helium.AsNode[*helium.Element](n); ok {
		if ns := el.Namespace(); ns != nil && ns.Prefix() != "" {
			return ns.Prefix() + ":" + el.LocalName()
		}
		return el.LocalName()
	}
	return n.Name()
}

// ProbeSchematron reports whether the official GİB files compile natively in helium.
// [Schematron] uses an ISO flatten + XPath 3.1 runner instead.
func ProbeSchematron(ctx context.Context) (SchematronStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := fs.ReadFile(schemas.FS, mainSchematron)
	if err != nil {
		return SchematronStatus{}, fmt.Errorf("read schematron: %w", err)
	}
	p := helium.NewParser()
	doc, err := p.Parse(ctx, data)
	if err != nil {
		return SchematronStatus{Supported: false, Reason: err.Error()}, nil
	}
	if _, err := schematron.NewCompiler().Compile(ctx, doc); err != nil {
		return SchematronStatus{Supported: false, Reason: err.Error()}, nil
	}
	return SchematronStatus{Supported: true}, nil
}

// SchematronStatus describes native helium compilation of the unmodified GİB files.
type SchematronStatus struct {
	Supported bool
	Reason    string
}
