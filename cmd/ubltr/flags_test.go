package main

import (
	"slices"
	"testing"
)

func TestFlagsFirst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		valueNames []string
		want       []string
	}{
		{
			name: "pdf after file",
			args: []string{"invoice.xml", "--pdf"},
			want: []string{"--pdf", "invoice.xml"},
		},
		{
			name:       "style after file",
			args:       []string{"invoice.xml", "--style", "a.xsl"},
			valueNames: []string{"style"},
			want:       []string{"--style", "a.xsl", "invoice.xml"},
		},
		{
			name:       "type equals after file",
			args:       []string{"invoice.xml", "--type=earchive"},
			valueNames: []string{"type"},
			want:       []string{"--type=earchive", "invoice.xml"},
		},
		{
			name: "double dash stops",
			args: []string{"--", "--pdf"},
			want: []string{"--", "--pdf"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := flagsFirst(tt.args, tt.valueNames...)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("flagsFirst() = %q, want %q", got, tt.want)
			}
		})
	}
}
