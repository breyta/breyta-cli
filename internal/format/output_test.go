package format

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteJSON_PrettyAndCompact(t *testing.T) {
	var buf bytes.Buffer
	v := map[string]any{"a": 1, "b": "x"}

	if err := WriteJSON(&buf, v, false); err != nil {
		t.Fatalf("WriteJSON compact: %v", err)
	}
	got := buf.String()
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected trailing newline, got %q", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &parsed); err != nil {
		t.Fatalf("expected valid json, got %v (%q)", err, got)
	}

	buf.Reset()
	if err := WriteJSON(&buf, v, true); err != nil {
		t.Fatalf("WriteJSON pretty: %v", err)
	}
	gotPretty := buf.String()
	if !strings.Contains(gotPretty, "\n  ") {
		t.Fatalf("expected indented json, got %q", gotPretty)
	}
}

func TestWrite_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, map[string]any{"a": 1}, "nope", false)
	if err == nil {
		t.Fatal("expected error")
	}
}
