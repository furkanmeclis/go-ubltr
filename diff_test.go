package ubltr_test

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func TestDiffKnownIDChange(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	if err := ubltr.Unmarshal(readFixture(t, "TemelFaturaOrnegi.xml"), &inv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	left, err := ubltr.Marshal(&inv)
	if err != nil {
		t.Fatalf("marshal left: %v", err)
	}
	inv.ID.Value = "CHANGED-ID"
	right, err := ubltr.Marshal(&inv)
	if err != nil {
		t.Fatalf("marshal right: %v", err)
	}

	changes, err := ubltr.Diff(left, right)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	found := false
	for _, c := range changes {
		if strings.Contains(c.Path, "ID") && (c.Left == "GIB20090000000001" || c.Right == "CHANGED-ID") {
			found = true
			if c.Right != "CHANGED-ID" && c.Left != "CHANGED-ID" {
				t.Fatalf("unexpected ID change: %+v", c)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected ID change, got %+v", changes)
	}
}

func TestDiffIdentical(t *testing.T) {
	t.Parallel()

	data := []byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"><ID>1</ID></Invoice>`)
	changes, err := ubltr.Diff(data, data)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %+v", changes)
	}
}
