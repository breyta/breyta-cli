package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestFeedbackSend_BuildsPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotMethod string
	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		gotMethod = method
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFeedbackCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"send",
		"--type", "feature-request",
		"--agent",
		"--title", "Need better retry hints",
		"--description", "flows push failed with parse error and no remediation hint",
		"--tag", "cli",
		"--tag", "agent,ux",
		"--flow", "daily-rollup",
		"--workflow-id", "wf-123",
		"--run-id", "r45",
		"--command", "flows push",
		"--metadata", `{"apiUrl":"http://localhost:8090"}`,
		"--context", `{"os":"darwin"}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "feedback.send" {
		t.Fatalf("expected method feedback.send, got %q", gotMethod)
	}
	if gotPayload["type"] != "feature_request" {
		t.Fatalf("expected type=feature_request, got %#v", gotPayload["type"])
	}
	if gotPayload["title"] != "Need better retry hints" {
		t.Fatalf("expected title set, got %#v", gotPayload["title"])
	}
	if gotPayload["description"] != "flows push failed with parse error and no remediation hint" {
		t.Fatalf("expected description set, got %#v", gotPayload["description"])
	}
	if gotPayload["agent"] != true {
		t.Fatalf("expected agent=true, got %#v", gotPayload["agent"])
	}
	if gotPayload["flowSlug"] != "daily-rollup" {
		t.Fatalf("expected flowSlug=daily-rollup, got %#v", gotPayload["flowSlug"])
	}
	if gotPayload["workflowId"] != "wf-123" {
		t.Fatalf("expected workflowId=wf-123, got %#v", gotPayload["workflowId"])
	}
	if gotPayload["runId"] != "r45" {
		t.Fatalf("expected runId=r45, got %#v", gotPayload["runId"])
	}
	if gotPayload["command"] != "flows push" {
		t.Fatalf("expected command=flows push, got %#v", gotPayload["command"])
	}
	if tags, _ := gotPayload["tags"].([]string); len(tags) != 3 || tags[0] != "cli" || tags[1] != "agent" || tags[2] != "ux" {
		t.Fatalf("expected tags [cli agent ux], got %#v", gotPayload["tags"])
	}
	if metadata, _ := gotPayload["metadata"].(map[string]any); metadata["apiUrl"] != "http://localhost:8090" {
		t.Fatalf("expected metadata.apiUrl, got %#v", gotPayload["metadata"])
	}
	if ctx, _ := gotPayload["context"].(map[string]any); ctx["os"] != "darwin" {
		t.Fatalf("expected context.os, got %#v", gotPayload["context"])
	}
}

func TestFeedbackSend_ValidatesRequiredFields(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFeedbackCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"send", "--description", "only description"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected missing title to fail")
	}
}

func TestFeedbackSend_RejectsInvalidMetadataFlag(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFeedbackCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"send",
		"--title", "Invalid metadata",
		"--description", "metadata should fail",
		"--metadata", `["not","an","object"]`,
	})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected metadata object validation to fail")
	}
}
