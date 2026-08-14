package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestFlowsUpdateBuildsCanonicalMetadataPayload(t *testing.T) {
	originalDo := doAPICommandFn
	originalUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = originalDo
		useDoAPICommandFn = originalUse
	})

	var method string
	var payload map[string]any
	doAPICommandFn = func(_ *cobra.Command, _ *App, gotMethod string, gotPayload map[string]any) error {
		method = gotMethod
		payload = gotPayload
		return nil
	}
	useDoAPICommandFn = true

	cmd := newFlowsUpdateCmd(&App{APIURL: "https://example.invalid", WorkspaceID: "ws-test", Token: "token"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{
		"demo-flow",
		"--name", "Demo",
		"--description", "A flow",
		"--tags", "one,two",
		"--group-key", "billing-core",
		"--group-name", "Billing Core",
		"--group-description", "Shared billing flows",
		"--group-order", "20",
		"--primary-display-connection-slot", "crm",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if method != "flows.update" {
		t.Fatalf("expected flows.update, got %q", method)
	}
	want := map[string]any{
		"flowSlug": "demo-flow", "name": "Demo", "description": "A flow", "tags": "one,two",
		"groupKey": "billing-core", "groupName": "Billing Core", "groupDescription": "Shared billing flows",
		"groupOrder": 20, "primaryDisplayConnectionSlot": "crm",
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("expected %s=%#v, got %#v", key, value, payload[key])
		}
	}
	for _, retired := range []string{"publishDescription", "publishMedia", "discover", "marketplace"} {
		if _, exists := payload[retired]; exists {
			t.Fatalf("retired field %q present in payload", retired)
		}
	}
}

func TestFlowsUpdateSupportsCanonicalMetadataClears(t *testing.T) {
	originalDo := doAPICommandFn
	originalUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = originalDo
		useDoAPICommandFn = originalUse
	})

	var payload map[string]any
	doAPICommandFn = func(_ *cobra.Command, _ *App, _ string, gotPayload map[string]any) error {
		payload = gotPayload
		return nil
	}
	useDoAPICommandFn = true

	cmd := newFlowsUpdateCmd(&App{APIURL: "https://example.invalid"})
	cmd.SetArgs([]string{"demo-flow", "--group-key", "", "--group-order", "", "--primary-display-connection-slot", ""})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if payload["groupKey"] != "" || payload["groupOrder"] != "" || payload["primaryDisplayConnectionSlot"] != "" {
		t.Fatalf("expected explicit clears, got %#v", payload)
	}
}
