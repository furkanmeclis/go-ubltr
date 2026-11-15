package validation

import (
	"testing"

	"github.com/lestrrat-go/helium"
)

func TestSeverityOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report bool
		role   string
		flag   string
		want   string
	}{
		{name: "assert default", want: SeverityError},
		{name: "report default", report: true, want: SeverityWarning},
		{name: "role warning", role: "warning", want: SeverityWarning},
		{name: "role WARN", role: "WARN", want: SeverityWarning},
		{name: "flag fatal", report: true, flag: "fatal", want: SeverityError},
		{name: "role info", role: "information", want: SeverityInfo},
		{name: "role wins over flag", role: "error", flag: "warning", want: SeverityError},
		{name: "unknown role falls back", role: "custom", want: SeverityError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := severityOf(tc.report, tc.role, tc.flag); got != tc.want {
				t.Fatalf("severityOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNodeLocatorPath(t *testing.T) {
	t.Parallel()

	doc, err := helium.NewParser().Parse(t.Context(), []byte(`<a:Root xmlns:a="urn:a" xmlns:b="urn:b">
  <b:Item/>
  <b:Item x="1"/>
  <Plain/>
</a:Root>`))
	if err != nil {
		t.Fatal(err)
	}
	loc := newNodeLocator(map[string]string{"r": "urn:a", "i": "urn:b"})
	second := findElementAtLine(doc, 3, "{urn:b}Item")
	if second == nil {
		t.Fatal("findElementAtLine returned nil")
	}
	if got := loc.path(second); got != "/r:Root/i:Item[2]" {
		t.Fatalf("path = %q", got)
	}
	plain := findElementAtLine(doc, 4, "Plain")
	if got := loc.path(plain); got != "/r:Root/Plain" {
		t.Fatalf("path = %q", got)
	}
	unknown := newNodeLocator(nil)
	if got := unknown.path(second); got != "/Q{urn:a}Root/Q{urn:b}Item[2]" {
		t.Fatalf("path without prefixes = %q", got)
	}
}
