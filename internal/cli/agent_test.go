package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAgentContextReadsFlowScopedProactiveContext(t *testing.T) {
	var gotPath string
	var gotQuery string
	var gotWorkspace string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotWorkspace = r.Header.Get("X-Breyta-Workspace")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activity": []any{},
			"proactiveContext": map[string]any{
				"events": []any{map[string]any{"id": "evt-1"}},
			},
		})
	}))
	t.Cleanup(server.Close)

	app := &App{WorkspaceID: "ws-test", APIURL: server.URL, Token: "token"}
	cmd := newAgentContextCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--flow", "lead-sync", "--limit", "7"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotPath != "/api/proactive-agent/activity" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "flowSlug=lead-sync&limit=7" {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotWorkspace != "ws-test" {
		t.Fatalf("workspace header = %q", gotWorkspace)
	}
	if !strings.Contains(out.String(), "evt-1") {
		t.Fatalf("output = %s", out.String())
	}
}

func captureAgentCommand(t *testing.T) (*App, *string, *map[string]any) {
	t.Helper()
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var method string
	var payload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, gotMethod string, gotPayload map[string]any) error {
		method = gotMethod
		payload = gotPayload
		return nil
	}
	useDoAPICommandFn = true
	return &App{WorkspaceID: "ws-test"}, &method, &payload
}

func TestAgentSettingsShowCallsSettingsGet(t *testing.T) {
	app, method, payload := captureAgentCommand(t)
	cmd := newAgentSettingsShowCmd(app)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *method != "proactive_agent.settings.get" {
		t.Fatalf("method = %q", *method)
	}
	if len(*payload) != 0 {
		t.Fatalf("payload = %#v", *payload)
	}
}

func TestAgentEmailSendBuildsDeduplicatedPayload(t *testing.T) {
	app, method, payload := captureAgentCommand(t)
	cmd := newAgentEmailSendCmd(app)
	cmd.SetArgs([]string{
		"--body", "I checked the failed run and found a missing account id.",
		"--subject", "I found the failed import",
		"--dedupe-key", "failed-run:customer-import",
		"--proactive-message-id", "work-ping-ws-test-user-test-2026-07-30-1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *method != "proactive_agent.email.send" {
		t.Fatalf("method = %q", *method)
	}
	want := map[string]any{
		"body":               "I checked the failed run and found a missing account id.",
		"subject":            "I found the failed import",
		"dedupeKey":          "failed-run:customer-import",
		"proactiveMessageId": "work-ping-ws-test-user-test-2026-07-30-1",
	}
	if !reflect.DeepEqual(*payload, want) {
		t.Fatalf("payload = %#v, want %#v", *payload, want)
	}
}

func TestAgentEmailSendPostsProactiveMessageIdentity(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-test",
			"data": map[string]any{
				"status":    "scheduled",
				"messageId": "work-ping-1",
			},
		})
	}))
	t.Cleanup(server.Close)

	app := &App{WorkspaceID: "ws-test", APIURL: server.URL, Token: "token"}
	cmd := newAgentEmailSendCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--body", "I found the failed import.",
		"--dedupe-key", "failed-run:customer-import",
		"--proactive-message-id", "work-ping-1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotBody["command"] != "proactive_agent.email.send" {
		t.Fatalf("command = %#v", gotBody["command"])
	}
	args, ok := gotBody["args"].(map[string]any)
	if !ok {
		t.Fatalf("args = %#v", gotBody["args"])
	}
	if args["proactiveMessageId"] != "work-ping-1" {
		t.Fatalf("proactiveMessageId = %#v", args["proactiveMessageId"])
	}
	if !strings.Contains(out.String(), `"status":"scheduled"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestAgentEmailSendRequiresBodyDedupeKeyAndProactiveMessageID(t *testing.T) {
	tests := [][]string{
		{"--dedupe-key", "finding", "--proactive-message-id", "work-ping-1"},
		{"--body", "Finding", "--proactive-message-id", "work-ping-1"},
		{"--body", "Finding", "--dedupe-key", "finding"},
	}
	for _, args := range tests {
		app, _, _ := captureAgentCommand(t)
		cmd := newAgentEmailSendCmd(app)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected an error for args %#v", args)
		}
	}
}

func TestAgentEmailAlreadySentIsSuccessfulNoOp(t *testing.T) {
	out := map[string]any{
		"ok": false,
		"data": map[string]any{
			"status": "skipped",
			"reason": "already-emailed",
		},
	}
	if !agentEmailAlreadySent(out, 200) {
		t.Fatal("expected already-emailed response to be recognized")
	}
	if agentEmailAlreadySent(out, 409) {
		t.Fatal("non-2xx response must not be normalized")
	}
}

func TestAgentSettingsUpdateBuildsPartialPayload(t *testing.T) {
	app, method, payload := captureAgentCommand(t)
	cmd := newAgentSettingsUpdateCmd(app)
	cmd.SetArgs([]string{
		"--enabled=true",
		"--email-enabled=false",
		"--check", "repeated-manual-run=false",
		"--check", "failed-run=true",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *method != "proactive_agent.settings.update" {
		t.Fatalf("method = %q", *method)
	}
	want := map[string]any{
		"enabled":      true,
		"emailEnabled": false,
		"rules": map[string]any{
			"repeated-manual-run": false,
			"failed-run":          true,
		},
	}
	if !reflect.DeepEqual(*payload, want) {
		t.Fatalf("payload = %#v, want %#v", *payload, want)
	}
}

func TestAgentSettingsUpdateRequiresAChange(t *testing.T) {
	app, _, _ := captureAgentCommand(t)
	cmd := newAgentSettingsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAgentSettingsUpdateRejectsMalformedCheck(t *testing.T) {
	app, _, _ := captureAgentCommand(t)
	cmd := newAgentSettingsUpdateCmd(app)
	cmd.SetArgs([]string{"--check", "repeated-manual-run"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAgentSettingsUpdateRejectsAmbiguousBoolean(t *testing.T) {
	app, _, _ := captureAgentCommand(t)
	cmd := newAgentSettingsUpdateCmd(app)
	cmd.SetArgs([]string{"--enabled=1"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error")
	}
}
