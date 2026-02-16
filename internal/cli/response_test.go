package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/breyta/breyta-cli/internal/updatecheck"
	"github.com/spf13/cobra"
)

func TestWriteFailureIncludesUpdateNotice(t *testing.T) {
	cmd := &cobra.Command{}
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))

	app := &App{
		WorkspaceID: "ws-test",
		updateNotice: &updatecheck.Notice{
			Available:      true,
			CurrentVersion: "v2026.1.1",
			LatestVersion:  "v2026.1.2",
			ReleaseURL:     updatecheck.ReleasePageURL,
		},
	}

	err := writeFailure(cmd, app, "boom", errors.New("broken"), "fix it", nil)
	if err == nil {
		t.Fatalf("expected error")
	}

	var env map[string]any
	if uerr := json.Unmarshal(out.Bytes(), &env); uerr != nil {
		t.Fatalf("unmarshal output: %v\n%s", uerr, out.String())
	}

	meta, ok := env["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta object, got %#v", env["meta"])
	}
	update, ok := meta["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta.update object, got %#v", meta["update"])
	}
	if got, _ := update["latestVersion"].(string); got != "v2026.1.2" {
		t.Fatalf("expected latestVersion v2026.1.2, got %q", got)
	}
}

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
