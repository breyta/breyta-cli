package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestFlowsSearch_APIMode_Payload(t *testing.T) {
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
	cmd := newFlowsSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"stripe webhook", "--scope", "workspace", "--provider", "stripe", "--limit", "7", "--from", "3", "--full=true"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if gotPayload["q"] != "stripe webhook" {
		t.Fatalf("expected q payload, got %#v", gotPayload["q"])
	}
	if gotPayload["scope"] != "workspace" {
		t.Fatalf("expected scope workspace, got %#v", gotPayload["scope"])
	}
	if gotPayload["provider"] != "stripe" {
		t.Fatalf("expected provider stripe, got %#v", gotPayload["provider"])
	}
	if gotPayload["size"] != 7 {
		t.Fatalf("expected size 7, got %#v", gotPayload["size"])
	}
	if gotPayload["from"] != 3 {
		t.Fatalf("expected from 3, got %#v", gotPayload["from"])
	}
	if gotPayload["includeDefinition"] != true {
		t.Fatalf("expected includeDefinition true, got %#v", gotPayload["includeDefinition"])
	}
}

