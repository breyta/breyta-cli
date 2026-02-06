package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestFlowsSearch_BuildsPayload(t *testing.T) {
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
	cmd.SetArgs([]string{"stripe", "--scope", "workspace", "--provider", "stripe", "--limit", "5", "--from", "10", "--full=true"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if gotPayload["query"] != "stripe" {
		t.Fatalf("expected query=stripe, got %#v", gotPayload["query"])
	}
	if gotPayload["scope"] != "workspace" {
		t.Fatalf("expected scope=workspace, got %#v", gotPayload["scope"])
	}
	if gotPayload["provider"] != "stripe" {
		t.Fatalf("expected provider=stripe, got %#v", gotPayload["provider"])
	}
	if gotPayload["limit"] != 5 {
		t.Fatalf("expected limit=5, got %#v", gotPayload["limit"])
	}
	if gotPayload["from"] != 10 {
		t.Fatalf("expected from=10, got %#v", gotPayload["from"])
	}
	if gotPayload["includeDefinition"] != true {
		t.Fatalf("expected includeDefinition=true, got %#v", gotPayload["includeDefinition"])
	}
}

func TestFlowsSearch_BrowseWithoutQuery(t *testing.T) {
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
	cmd.SetArgs([]string{"--provider", "stripe", "--limit", "25"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if _, ok := gotPayload["query"]; ok {
		t.Fatalf("expected query omitted for browse, got %#v", gotPayload["query"])
	}
	if gotPayload["provider"] != "stripe" {
		t.Fatalf("expected provider=stripe, got %#v", gotPayload["provider"])
	}
	if gotPayload["limit"] != 25 {
		t.Fatalf("expected limit=25, got %#v", gotPayload["limit"])
	}
}
