package validation

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/schematron"

	"github.com/furkanmeclis/go-ubltr/schemas"
)

const mainSchematron = "ubltr-1.2.1/schematron/UBL-TR_Main_Schematron.xml"

// SchematronStatus describes whether native GİB Schematron compilation works.
type SchematronStatus struct {
	Supported bool
	Reason    string
}

// ProbeSchematron attempts to compile the official GİB UBL-TR Main Schematron
// with helium. GİB rules use ISO include/abstract/extends which helium does
// not implement yet; callers should treat a false Supported as expected.
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

	collector := helium.NewErrorCollector(ctx, helium.ErrorLevelNone)
	_, err = schematron.NewCompiler().ErrorHandler(collector).Compile(ctx, doc)
	if err == nil {
		return SchematronStatus{Supported: true}, nil
	}

	reason := err.Error()
	seen := map[string]struct{}{}
	var unique []string
	for _, e := range collector.Errors() {
		msg := strings.TrimSpace(e.Error())
		if msg == "" {
			continue
		}
		if _, ok := seen[msg]; ok {
			continue
		}
		seen[msg] = struct{}{}
		unique = append(unique, msg)
		if len(unique) == 8 {
			break
		}
	}
	if len(unique) > 0 {
		reason = reason + ": " + strings.Join(unique, "; ")
	}
	return SchematronStatus{Supported: false, Reason: reason}, nil
}
