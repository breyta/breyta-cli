package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestFlowsInterfacesUpsertSendsEntrypointPayload(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	dir := t.TempDir()
	inputFile := filepath.Join(dir, "input.edn")
	outputFile := filepath.Join(dir, "output.edn")
	if err := os.WriteFile(inputFile, []byte("[{:name :account :type :string :required true}]"), 0o600); err != nil {
		t.Fatalf("write input schema: %v", err)
	}
	if err := os.WriteFile(outputFile, []byte("[:map [:summary :string]]"), 0o600); err != nil {
		t.Fatalf("write output schema: %v", err)
	}

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
				"flowSlug":    "my-flow",
				"source":      "draft",
				"interfaceId": "summarize",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "interfaces", "upsert", "my-flow", "summarize",
		"--kind", "mcp",
		"--tool-name", "summarize_company",
		"--input-schema", inputFile,
		"--output-schema", outputFile,
		"--description", "Summarize account context",
		"--trusted-metadata",
	)
	if err != nil {
		t.Fatalf("flows interfaces upsert failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.interfaces.upsert" {
		t.Fatalf("expected flows.interfaces.upsert, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["interfaceId"] != "summarize" {
		t.Fatalf("expected flow interface args, got %#v", args)
	}
	if args["kind"] != "mcp" || args["toolName"] != "summarize_company" || args["description"] != "Summarize account context" {
		t.Fatalf("expected mcp metadata args, got %#v", args)
	}
	if args["inputSchema"] != "[{:name :account :type :string :required true}]" {
		t.Fatalf("expected inputSchema file literal, got %#v", args["inputSchema"])
	}
	if args["outputSchema"] != "[:map [:summary :string]]" {
		t.Fatalf("expected outputSchema file literal, got %#v", args["outputSchema"])
	}
	if args["trustedMetadata"] != true || args["enabled"] != true {
		t.Fatalf("expected boolean flags, got %#v", args)
	}
}

func TestFlowsInterfacesValidateAndRemoveSendEntrypointPayload(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

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
			"data":        map[string]any{"ready": true},
		})
	}))
	defer srv.Close()

	for _, cliArgs := range [][]string{
		{"flows", "interfaces", "validate", "my-flow", "run"},
		{"flows", "interfaces", "remove", "my-flow", "run"},
	} {
		stdout, stderr, err := runCLIArgs(t,
			append([]string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}, cliArgs...)...,
		)
		if err != nil {
			t.Fatalf("%v failed: %v\nstdout=%s\nstderr=%s", cliArgs, err, stdout, stderr)
		}
	}
	if len(commands) != 2 || commands[0] != "flows.interfaces.validate" || commands[1] != "flows.interfaces.remove" {
		t.Fatalf("unexpected commands: %#v", commands)
	}
	for _, args := range payloads {
		if args["flowSlug"] != "my-flow" || args["interfaceId"] != "run" || args["source"] != "draft" {
			t.Fatalf("expected interface target args, got %#v", args)
		}
	}
}

func TestFlowsSchedulesUpsertSendsEntrypointPayload(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	inputFile := filepath.Join(t.TempDir(), "schedule-input.edn")
	if err := os.WriteFile(inputFile, []byte("[{:name :limit :type :number}]"), 0o600); err != nil {
		t.Fatalf("write schedule input schema: %v", err)
	}

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
				"flowSlug":   "my-flow",
				"source":     "draft",
				"scheduleId": "weekday",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "schedules", "upsert", "my-flow", "weekday",
		"--cron", "0 9 * * 1-5",
		"--timezone", "Europe/Oslo",
		"--input-schema", inputFile,
		"--label", "Weekday",
	)
	if err != nil {
		t.Fatalf("flows schedules upsert failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.schedules.upsert" {
		t.Fatalf("expected flows.schedules.upsert, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["scheduleId"] != "weekday" || args["source"] != "draft" {
		t.Fatalf("expected schedule args, got %#v", args)
	}
	if args["cron"] != "0 9 * * 1-5" || args["timezone"] != "Europe/Oslo" || args["label"] != "Weekday" {
		t.Fatalf("expected schedule metadata, got %#v", args)
	}
	if args["inputSchema"] != "[{:name :limit :type :number}]" || args["enabled"] != true {
		t.Fatalf("expected schedule schema/enabled args, got %#v", args)
	}
}

func TestFlowsSchedulesValidateAndRemoveSendEntrypointPayload(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

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
			"data":        map[string]any{"ready": true},
		})
	}))
	defer srv.Close()

	for _, cliArgs := range [][]string{
		{"flows", "schedules", "validate", "my-flow", "weekday"},
		{"flows", "schedules", "remove", "my-flow", "weekday"},
	} {
		stdout, stderr, err := runCLIArgs(t,
			append([]string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}, cliArgs...)...,
		)
		if err != nil {
			t.Fatalf("%v failed: %v\nstdout=%s\nstderr=%s", cliArgs, err, stdout, stderr)
		}
	}
	if len(commands) != 2 || commands[0] != "flows.schedules.validate" || commands[1] != "flows.schedules.remove" {
		t.Fatalf("unexpected commands: %#v", commands)
	}
	for _, args := range payloads {
		if args["flowSlug"] != "my-flow" || args["scheduleId"] != "weekday" || args["source"] != "draft" {
			t.Fatalf("expected schedule target args, got %#v", args)
		}
	}
}
