package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestFlowsUpdate_BuildsGroupingPayload(t *testing.T) {
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
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"demo-flow",
		"--group-key", "billing-core",
		"--group-name", "Billing Core",
		"--group-description", "Shared billing flows",
		"--group-order", "20",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "flows.update" {
		t.Fatalf("expected method flows.update, got %q", gotMethod)
	}
	if gotPayload["flowSlug"] != "demo-flow" {
		t.Fatalf("expected flowSlug=demo-flow, got %#v", gotPayload["flowSlug"])
	}
	if gotPayload["groupKey"] != "billing-core" {
		t.Fatalf("expected groupKey=billing-core, got %#v", gotPayload["groupKey"])
	}
	if gotPayload["groupName"] != "Billing Core" {
		t.Fatalf("expected groupName=Billing Core, got %#v", gotPayload["groupName"])
	}
	if gotPayload["groupDescription"] != "Shared billing flows" {
		t.Fatalf("expected groupDescription=Shared billing flows, got %#v", gotPayload["groupDescription"])
	}
	if gotPayload["groupOrder"] != 20 {
		t.Fatalf("expected groupOrder=20, got %#v", gotPayload["groupOrder"])
	}
}

func TestFlowsUpdate_BuildsGroupClearPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--group-key", ""})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["groupKey"]
	if !ok {
		t.Fatalf("expected groupKey to be present in payload")
	}
	if value != "" {
		t.Fatalf("expected groupKey to be empty string for explicit clear, got %#v", value)
	}
}

func TestFlowsUpdate_BuildsGroupOrderClearPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--group-order", ""})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["groupOrder"]
	if !ok {
		t.Fatalf("expected groupOrder to be present in payload")
	}
	if value != "" {
		t.Fatalf("expected groupOrder to be empty string for explicit clear, got %#v", value)
	}
}
