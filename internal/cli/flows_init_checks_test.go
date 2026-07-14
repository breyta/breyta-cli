package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFlowsInitSendsEmptyDraftPayload(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "my-flow",
				"source":   "draft",
				"empty":    true,
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "init", "my-flow",
		"--empty",
		"--name", "My Flow",
		"--description", "Step-first draft",
	)
	if err != nil {
		t.Fatalf("flows init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.init" {
		t.Fatalf("expected flows.init, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["empty"] != true {
		t.Fatalf("expected empty init args, got %#v", args)
	}
	if args["name"] != "My Flow" || args["description"] != "Step-first draft" {
		t.Fatalf("expected name/description args, got %#v", args)
	}
}

func TestFlowAuthoringStatusCommandsHonorCommandCancellation(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	tests := []struct {
		name string
		args []string
	}{
		{name: "init", args: []string{"flows", "init", "my-flow", "--empty"}},
		{name: "status", args: []string{"flows", "status", "my-flow"}},
		{name: "connections status", args: []string{"flows", "connections", "status", "my-flow"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(500 * time.Millisecond):
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			start := time.Now()
			args := append([]string{
				"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
			}, tc.args...)
			_, stderr, err := runCLIArgsWithContext(t, ctx, args...)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected context deadline exceeded, got %v\n%s", err, stderr)
			}
			if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
				t.Fatalf("expected command to stop promptly on cancellation, took %s", elapsed)
			}
		})
	}
}

func TestFlowsChecksCreateRunAndStatusSendPayloads(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	checkFile := filepath.Join(t.TempDir(), "security.edn")
	if err := os.WriteFile(checkFile, []byte("{:policy {:secrets :redacted}}"), 0o600); err != nil {
		t.Fatalf("write check file: %v", err)
	}

	var commands []string
	var payloads []map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		commands = append(commands, got["command"].(string))
		args, _ := got["args"].(map[string]any)
		payloads = append(payloads, args)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "my-flow",
				"source":   "draft",
				"ready":    true,
			},
		})
	}))
	defer srv.Close()

	for _, cliArgs := range [][]string{
		{"flows", "checks", "create", "my-flow", "security-policy", "--category", "security", "--file", checkFile, "--description", "Security policy"},
		{"flows", "checks", "run", "my-flow", "--category", "security"},
		{"flows", "checks", "status", "my-flow", "--category", "security"},
	} {
		stdout, stderr, err := runCLIArgs(t,
			append([]string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}, cliArgs...)...,
		)
		if err != nil {
			t.Fatalf("%v failed: %v\nstdout=%s\nstderr=%s", cliArgs, err, stdout, stderr)
		}
	}
	if len(commands) != 3 ||
		commands[0] != "flows.checks.create" ||
		commands[1] != "flows.checks.run" ||
		commands[2] != "flows.checks.status" {
		t.Fatalf("unexpected commands: %#v", commands)
	}
	createArgs := payloads[0]
	if createArgs["flowSlug"] != "my-flow" || createArgs["checkId"] != "security-policy" || createArgs["source"] != "draft" {
		t.Fatalf("expected check create target args, got %#v", createArgs)
	}
	if createArgs["category"] != "security" || createArgs["description"] != "Security policy" {
		t.Fatalf("expected check create metadata args, got %#v", createArgs)
	}
	if createArgs["checkLiteral"] != "{:policy {:secrets :redacted}}" {
		t.Fatalf("expected check literal file content, got %#v", createArgs["checkLiteral"])
	}
	if _, exists := createArgs["enabled"]; exists {
		t.Fatalf("expected omitted --enabled to preserve existing state, got %#v", createArgs)
	}
	for _, args := range payloads[1:] {
		if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["category"] != "security" {
			t.Fatalf("expected check run/status args, got %#v", args)
		}
	}
}

func TestFlowsChecksCreateRejectsBlankOrEmptyFile(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	emptyFile := filepath.Join(t.TempDir(), "empty-check.edn")
	if err := os.WriteFile(emptyFile, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("write empty check file: %v", err)
	}

	requests := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "request should not be sent", http.StatusInternalServerError)
	}))
	defer srv.Close()

	baseArgs := []string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}
	for _, tc := range []struct {
		name    string
		file    string
		wantErr string
	}{
		{name: "blank path", file: "", wantErr: "--file requires a non-empty path"},
		{name: "empty contents", file: emptyFile, wantErr: "--file must contain a non-empty check definition"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(baseArgs, "flows", "checks", "create", "my-flow", "definition-of-done", "--file", tc.file)
			stdout, stderr, err := runCLIArgs(t, args...)
			if err == nil || !strings.Contains(stdout+stderr+err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, err=%v\nstdout=%s\nstderr=%s", tc.wantErr, err, stdout, stderr)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("expected validation before API request, got %d request(s)", requests)
	}
}

func TestFlowsStepsChecksRunSendsTargetPayload(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flowSlug": "my-flow",
				"source":   "draft",
				"ready":    true,
				"target": map[string]any{
					"kind":   "step",
					"stepId": "tools/search",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "steps", "checks", "run", "my-flow", "tools/search",
		"--category", "eval",
	)
	if err != nil {
		t.Fatalf("flows steps checks run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.checks.run" {
		t.Fatalf("expected flows.steps.checks.run, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "tools/search" || args["category"] != "eval" {
		t.Fatalf("expected target check args, got %#v", args)
	}
}
