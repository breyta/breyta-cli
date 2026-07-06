package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
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
	for _, args := range payloads[1:] {
		if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["category"] != "security" {
			t.Fatalf("expected check run/status args, got %#v", args)
		}
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
