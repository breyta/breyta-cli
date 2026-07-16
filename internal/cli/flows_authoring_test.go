package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	if args["trustedMetadata"] != true {
		t.Fatalf("expected trustedMetadata flag, got %#v", args)
	}
	if _, exists := args["enabled"]; exists {
		t.Fatalf("expected omitted --enabled to preserve existing state, got %#v", args)
	}
}

func TestFlowsInterfacesUpsertCanValidateInOneCommand(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var commands []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		command, _ := got["command"].(string)
		commands = append(commands, command)
		data := map[string]any{"flowSlug": "my-flow", "interfaceId": "run"}
		if command == "flows.interfaces.validate" {
			data["ready"] = true
			data["checks"] = []any{map[string]any{"id": "interface", "pass": true}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        data,
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "interfaces", "upsert", "my-flow", "run",
		"--kind", "manual", "--validate",
	)
	if err != nil {
		t.Fatalf("flows interfaces upsert --validate failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(commands) != 2 || commands[0] != "flows.interfaces.upsert" || commands[1] != "flows.interfaces.validate" {
		t.Fatalf("expected upsert followed by validation, got %#v", commands)
	}
	if !strings.Contains(stdout, `"validation"`) {
		t.Fatalf("expected combined validation result, got stdout=%s", stdout)
	}
}

func TestFlowsInterfacesUpsertSendsStructuredAuthPayload(t *testing.T) {
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
			"data":        map[string]any{"flowSlug": "my-flow", "interfaceId": "events"},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "interfaces", "upsert", "my-flow", "events",
		"--kind", "webhook",
		"--event-name", "orders.updated",
		"--path", "/events",
		"--auth-json", `{"type":"hmac-sha256","secretRef":"res://secret/webhook-key","header":"X-Signature"}`,
	)
	if err != nil {
		t.Fatalf("flows interfaces upsert failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	args, _ := got["args"].(map[string]any)
	auth, _ := args["auth"].(map[string]any)
	if args["eventName"] != "orders.updated" {
		t.Fatalf("expected webhook eventName payload, got %#v", args)
	}
	if auth["type"] != "hmac-sha256" || auth["secretRef"] != "res://secret/webhook-key" || auth["header"] != "X-Signature" {
		t.Fatalf("expected structured auth payload, got %#v", args["auth"])
	}
}

func TestFlowsInterfacesUpsertDoesNotInferMCPToolNameOnPartialUpdate(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"flowSlug": "my-flow"},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "interfaces", "upsert", "my-flow", "summarize-company",
		"--kind", "mcp", "--description", "Updated description",
	)
	if err != nil {
		t.Fatalf("MCP partial upsert failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	args, _ := got["args"].(map[string]any)
	if _, exists := args["toolName"]; exists {
		t.Fatalf("expected omitted --tool-name to stay omitted, got %#v", args)
	}
	if args["interfaceId"] != "summarize-company" {
		t.Fatalf("expected positional MCP tool target, got %#v", args)
	}
}

func TestFlowsAuthoringUpsertsRejectIncompleteSafetyFlags(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	emptyAuthFile := filepath.Join(t.TempDir(), "empty-auth.json")
	if err := os.WriteFile(emptyAuthFile, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("write empty auth file: %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{name: "missing kind", args: []string{"flows", "interfaces", "upsert", "my-flow", "events"}},
		{name: "blank auth json", args: []string{"flows", "interfaces", "upsert", "my-flow", "events", "--kind", "webhook", "--auth-json", ""}},
		{name: "empty auth file", args: []string{"flows", "interfaces", "upsert", "my-flow", "events", "--kind", "webhook", "--auth-file", emptyAuthFile}},
		{name: "interface empty input schema file", args: []string{"flows", "interfaces", "upsert", "my-flow", "run", "--kind", "manual", "--input-schema", emptyAuthFile}},
		{name: "interface empty output schema file", args: []string{"flows", "interfaces", "upsert", "my-flow", "run", "--kind", "manual", "--output-schema", emptyAuthFile}},
		{name: "interface empty response file", args: []string{"flows", "interfaces", "upsert", "my-flow", "run", "--kind", "manual", "--response", emptyAuthFile}},
		{name: "interface blank input schema literal", args: []string{"flows", "interfaces", "upsert", "my-flow", "run", "--kind", "manual", "--input-schema-literal", ""}},
		{name: "schedule empty input schema file", args: []string{"flows", "schedules", "upsert", "my-flow", "weekday", "--cron", "0 9 * * 1-5", "--input-schema", emptyAuthFile}},
		{name: "schedule empty response file", args: []string{"flows", "schedules", "upsert", "my-flow", "weekday", "--cron", "0 9 * * 1-5", "--response", emptyAuthFile}},
		{name: "schedule blank response literal", args: []string{"flows", "schedules", "upsert", "my-flow", "weekday", "--cron", "0 9 * * 1-5", "--response-literal", ""}},
		{name: "interface clear conflict", args: []string{"flows", "interfaces", "upsert", "my-flow", "run", "--kind", "manual", "--label", "Run", "--clear", "label"}},
		{name: "schedule clear conflict", args: []string{"flows", "schedules", "upsert", "my-flow", "weekday", "--cron", "0 9 * * 1-5", "--timezone", "UTC", "--clear", "timezone"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer srv.Close()

			baseArgs := []string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}
			stdout, stderr, err := runCLIArgs(t, append(baseArgs, tc.args...)...)
			if err == nil {
				t.Fatalf("expected validation failure\nstdout=%s\nstderr=%s", stdout, stderr)
			}
			if calls != 0 {
				t.Fatalf("expected validation before the API request, got %d calls", calls)
			}
		})
	}
}

func TestFlowsAuthoringUpsertsForwardExplicitClearFields(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var payloads []map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		args, _ := got["args"].(map[string]any)
		payloads = append(payloads, args)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"flowSlug": "my-flow"},
		})
	}))
	defer srv.Close()

	baseArgs := []string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}
	commands := [][]string{
		{"flows", "interfaces", "upsert", "my-flow", "events", "--kind", "webhook", "--clear", "auth", "--clear", "description", "--clear", "event-name"},
		{"flows", "schedules", "upsert", "my-flow", "weekday", "--cron", "0 9 * * 1-5", "--clear", "timezone", "--clear", "response"},
	}
	for _, cliArgs := range commands {
		stdout, stderr, err := runCLIArgs(t, append(baseArgs, cliArgs...)...)
		if err != nil {
			t.Fatalf("%v failed: %v\nstdout=%s\nstderr=%s", cliArgs, err, stdout, stderr)
		}
	}

	if len(payloads) != 2 {
		t.Fatalf("expected two payloads, got %#v", payloads)
	}
	interfaceClear, _ := payloads[0]["clearFields"].([]any)
	if len(interfaceClear) != 3 || interfaceClear[0] != "auth" || interfaceClear[1] != "description" || interfaceClear[2] != "event-name" {
		t.Fatalf("expected interface clear fields, got %#v", payloads[0])
	}
	scheduleClear, _ := payloads[1]["clearFields"].([]any)
	if len(scheduleClear) != 2 || scheduleClear[0] != "timezone" || scheduleClear[1] != "response" {
		t.Fatalf("expected schedule clear fields, got %#v", payloads[1])
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
		{"flows", "interfaces", "validate", "my-flow", "run", "--kind", "manual"},
		{"flows", "interfaces", "remove", "my-flow", "run", "--kind", "mcp"},
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
	if payloads[0]["kind"] != "manual" {
		t.Fatalf("expected validate payload to forward disambiguating kind, got %#v", payloads[0])
	}
	if payloads[1]["kind"] != "mcp" {
		t.Fatalf("expected remove payload to forward disambiguating kind, got %#v", payloads[1])
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
	if args["inputSchema"] != "[{:name :limit :type :number}]" {
		t.Fatalf("expected schedule schema args, got %#v", args)
	}
	if _, exists := args["enabled"]; exists {
		t.Fatalf("expected omitted --enabled to preserve existing state, got %#v", args)
	}
}

func TestFlowsAuthoringForwardsExplicitDisabledState(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var payloads []map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		args, _ := got["args"].(map[string]any)
		payloads = append(payloads, args)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"flowSlug": "my-flow"},
		})
	}))
	defer srv.Close()

	commands := [][]string{
		{"flows", "interfaces", "upsert", "my-flow", "run", "--kind", "manual", "--enabled=false"},
		{"flows", "schedules", "upsert", "my-flow", "weekday", "--cron", "0 9 * * 1-5", "--enabled=false"},
		{"flows", "checks", "create", "my-flow", "security-policy", "--enabled=false"},
	}
	for _, cliArgs := range commands {
		stdout, stderr, err := runCLIArgs(t,
			append([]string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}, cliArgs...)...,
		)
		if err != nil {
			t.Fatalf("%v failed: %v\nstdout=%s\nstderr=%s", cliArgs, err, stdout, stderr)
		}
	}
	if len(payloads) != len(commands) {
		t.Fatalf("expected %d payloads, got %#v", len(commands), payloads)
	}
	for _, args := range payloads {
		if enabled, exists := args["enabled"]; !exists || enabled != false {
			t.Fatalf("expected explicit enabled=false, got %#v", args)
		}
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
