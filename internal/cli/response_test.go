package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
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

func TestEmitUpdateReminderUsesManualCopyForUnknownInstallMethod(t *testing.T) {
	cmd := &cobra.Command{}
	errOut := new(bytes.Buffer)
	cmd.SetErr(errOut)

	app := &App{
		updateNotice: &updatecheck.Notice{
			Available:      true,
			CurrentVersion: "v2026.1.1",
			LatestVersion:  "v2026.1.2",
			InstallMethod:  updatecheck.InstallMethodUnknown,
			FixCommand:     updatecheck.ManualFixCommand,
		},
	}

	app.emitUpdateReminder(cmd)

	got := errOut.String()
	if !strings.Contains(got, "Manual upgrade required for install method unknown") {
		t.Fatalf("expected manual upgrade copy, got %q", got)
	}
	if strings.Contains(got, updatecheck.DefaultFixCommand) {
		t.Fatalf("manual reminder should not recommend %q, got %q", updatecheck.DefaultFixCommand, got)
	}
	if !strings.Contains(got, updatecheck.ManualFixCommand) {
		t.Fatalf("expected manual fix command %q, got %q", updatecheck.ManualFixCommand, got)
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
	if got, _ := meta["webUrl"].(string); got != "https://flows.breyta.ai/ws-acme/runs?run=wf-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
}

func TestEngineUIURLPreservesReverseProxyMountPath(t *testing.T) {
	base := "https://example.test/breyta/ws-acme"
	got := engineUIURL(base, "runs", "run", "wf-123")
	want := "https://example.test/breyta/ws-acme/runs?run=wf-123"
	if got != want {
		t.Fatalf("unexpected mounted UI URL: got %q want %q", got, want)
	}
}

func TestNormalizeResourceWebURLUsesStructuralResourceKind(t *testing.T) {
	base := "https://example.test/ws-acme"
	resource := map[string]any{
		"uri": "res://v1/ws/ws-acme/result/blob/archive/file/report.json",
	}
	normalizeResourceWebURL(base, resource, "")
	want := "https://example.test/ws-acme/resources"
	if got := firstNonBlankString(resource["webUrl"]); got != want {
		t.Fatalf("unexpected blob resource URL: got %q want %q", got, want)
	}
}
