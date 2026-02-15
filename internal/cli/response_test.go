package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

func TestWriteData_PreservesMetaAddedByLinkEnrichment(t *testing.T) {
	app := &App{
		WorkspaceID: "ws-acme",
		APIURL:      "https://flows.breyta.ai",
	}
	out := new(bytes.Buffer)
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(out)

	err := writeData(cmd, app, nil, map[string]any{
		"run": map[string]any{
			"flowSlug":   "daily-sales-report",
			"workflowId": "wf-123",
		},
	})
	if err != nil {
		t.Fatalf("writeData returned error: %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("expected JSON output, got err=%v\n%s", err, out.String())
	}
	meta, _ := envelope["meta"].(map[string]any)
	if meta == nil {
		t.Fatalf("expected meta map to be preserved")
	}
	if got, _ := meta["webUrl"].(string); got != "https://flows.breyta.ai/ws-acme/runs/daily-sales-report/wf-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
}
