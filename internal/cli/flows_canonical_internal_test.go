package cli

import "testing"

func TestParseFlowRunUploadsRejectsConflictsBeforeUpload(t *testing.T) {
	_, err := parseFlowRunUploads(
		[]string{"thesis=/tmp/thesis.pdf"},
		map[string]bool{"thesis": true},
	)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestParseFlowRunUploadNormalizesEDNStyleFieldName(t *testing.T) {
	field, path, err := parseFlowRunUpload(":thesis=/tmp/thesis.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if field != "thesis" || path != "/tmp/thesis.pdf" {
		t.Fatalf("unexpected parsed upload: field=%q path=%q", field, path)
	}
}

func TestAddFlowRunUploadInputAppendsRepeatedField(t *testing.T) {
	input := map[string]any{}
	first := map[string]any{"type": "resource-ref", "uri": "res://v1/ws/ws-1/file/a"}
	second := map[string]any{"type": "resource-ref", "uri": "res://v1/ws/ws-1/file/b"}

	if err := addFlowRunUploadInput(input, "attachments", first, nil); err != nil {
		t.Fatal(err)
	}
	if err := addFlowRunUploadInput(input, "attachments", second, nil); err != nil {
		t.Fatal(err)
	}

	items, ok := input["attachments"].([]any)
	if !ok {
		t.Fatalf("expected attachments to become a slice, got %#v", input["attachments"])
	}
	firstItem, _ := items[0].(map[string]any)
	secondItem, _ := items[1].(map[string]any)
	if len(items) != 2 || firstItem["uri"] != first["uri"] || secondItem["uri"] != second["uri"] {
		t.Fatalf("unexpected repeated upload refs: %#v", items)
	}
}
