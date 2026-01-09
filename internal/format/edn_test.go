package format

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteEDN_SortsKeysAndKeywords(t *testing.T) {
	var buf bytes.Buffer
	v := map[string]any{
		"b":      2,
		"a":      1,
		"sp ace": "x",
	}
	if err := WriteEDN(&buf, v, false); err != nil {
		t.Fatalf("WriteEDN: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	// Keys must be sorted: :a then :b then :sp-ace
	if got != "{:a 1 :b 2 :sp-ace \"x\"}" {
		t.Fatalf("unexpected edn: %q", got)
	}
}

func TestWriteEDN_NumberRendering(t *testing.T) {
	var buf bytes.Buffer
	v := map[string]any{"i": 3.0, "f": 3.5}
	if err := WriteEDN(&buf, v, false); err != nil {
		t.Fatalf("WriteEDN: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	// JSON numbers become float64; encoder prints 3.0 as int.
	if got != "{:f 3.5 :i 3}" {
		t.Fatalf("unexpected edn: %q", got)
	}
}

func TestWriteEDN_Pretty(t *testing.T) {
	var buf bytes.Buffer
	v := map[string]any{"a": []any{1.0, "x"}}
	if err := WriteEDN(&buf, v, true); err != nil {
		t.Fatalf("WriteEDN: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected pretty output with newlines, got %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected trailing newline, got %q", got)
	}
}
