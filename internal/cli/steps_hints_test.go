package cli

import (
	"strings"
	"testing"
)

func TestRenderNextActionsBlock_FromHints(t *testing.T) {
	out := map[string]any{
		"ok": true,
		"_hints": []any{
			"breyta steps record --flow my-flow --type code --id make-output --params '{...}'",
			"breyta steps docs set my-flow make-output --markdown '...'",
		},
	}
	block := renderNextActionsBlock(out, 4)
	if block == "" {
		t.Fatalf("expected non-empty block")
	}
	if want := "Next actions:"; len(block) < len(want) || block[:len(want)] != want {
		t.Fatalf("expected block to start with %q, got %q", want, block)
	}
	if !strings.Contains(block, "breyta steps record") {
		t.Fatalf("expected block to include record hint, got %q", block)
	}
}

func TestRenderNextActionsBlock_Max(t *testing.T) {
	out := map[string]any{
		"ok": true,
		"_hints": []any{
			"a",
			"b",
			"c",
		},
	}
	block := renderNextActionsBlock(out, 2)
	if strings.Contains(block, "\n  - c") || strings.HasSuffix(block, "  - c") {
		t.Fatalf("expected block to be truncated, got %q", block)
	}
}
