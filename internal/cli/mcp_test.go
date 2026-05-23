package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPStdioProxyBridgesJSONRPCAndPolicyHeaders(t *testing.T) {
	var seenPath string
	var seenAuth string
	var seenAccept string
	var seenContentType string
	var seenMCPMethod string
	var seenMCPName string
	var seenMCPProtocol string
	var seenToolsets string
	var seenReadOnly string
	var seenBody map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		seenAccept = r.Header.Get("Accept")
		seenContentType = r.Header.Get("Content-Type")
		seenMCPMethod = r.Header.Get("Mcp-Method")
		seenMCPName = r.Header.Get("Mcp-Name")
		seenMCPProtocol = r.Header.Get("MCP-Protocol-Version")
		seenToolsets = r.Header.Get("X-MCP-Toolsets")
		seenReadOnly = r.Header.Get("X-MCP-Readonly")
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      seenBody["id"],
			"result":  map[string]any{"ok": true},
		})
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		Policy: mcpPolicyOptions{
			Toolsets: "read,setup,feedback",
			ReadOnly: true,
		},
		In:  strings.NewReader(`{"jsonrpc":"2.0","id":"one","method":"tools/call","params":{"name":"search_flows","arguments":{},"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25"}}}` + "\n"),
		Out: &stdout,
		Err: &stderr,
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	if seenPath != "/api/workspaces/ws-acme/mcp" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if seenAuth != "Bearer secret-api-key" {
		t.Fatalf("unexpected auth header: %q", seenAuth)
	}
	if !strings.Contains(seenAccept, "application/json") || !strings.Contains(seenAccept, "text/event-stream") {
		t.Fatalf("unexpected accept header: %q", seenAccept)
	}
	if seenContentType != "application/json" {
		t.Fatalf("unexpected content-type: %q", seenContentType)
	}
	if seenMCPMethod != "tools/call" {
		t.Fatalf("unexpected MCP method header: %q", seenMCPMethod)
	}
	if seenMCPName != "search_flows" {
		t.Fatalf("unexpected MCP name header: %q", seenMCPName)
	}
	if seenMCPProtocol != "2025-11-25" {
		t.Fatalf("unexpected MCP protocol header: %q", seenMCPProtocol)
	}
	if seenToolsets != "read,setup,feedback" {
		t.Fatalf("unexpected toolsets header: %q", seenToolsets)
	}
	if seenReadOnly != "true" {
		t.Fatalf("unexpected read-only header: %q", seenReadOnly)
	}
	if seenBody["method"] != "tools/call" {
		t.Fatalf("unexpected request body: %#v", seenBody)
	}
	if strings.Contains(stdout.String(), "secret-api-key") || strings.Contains(stderr.String(), "secret-api-key") {
		t.Fatalf("secret leaked in stdio output\nstdout=%q\nstderr=%q", stdout.String(), stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
		t.Fatalf("invalid stdout json: %v\n%s", err, stdout.String())
	}
	if out["id"] != "one" {
		t.Fatalf("unexpected response: %#v", out)
	}
}

func TestMCPStdioProxySuppressesNotificationResponses(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		In:          strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"),
		Out:         &stdout,
		Err:         &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected no notification response, got %q", stdout.String())
	}
}

func TestMCPStdioProxyReturnsJSONRPCErrorForUpstreamHTTPError(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Authentication required",
				"details": map[string]any{"apiKey": "secret-api-key"},
			},
		})
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		In:          strings.NewReader(`{"jsonrpc":"2.0","id":"bad","method":"tools/list"}` + "\n"),
		Out:         &stdout,
		Err:         &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	if strings.Contains(stdout.String(), "secret-api-key") {
		t.Fatalf("secret leaked in error output: %s", stdout.String())
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
		t.Fatalf("invalid stdout json: %v\n%s", err, stdout.String())
	}
	errMap := mapStringAny(out["error"])
	if got := int(errMap["code"].(float64)); got != -32000 {
		t.Fatalf("expected code -32000, got %d", got)
	}
	if got := firstNonBlankString(errMap["message"]); got != "Authentication required" {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestMCPConfigRendersWorkspaceBoundStdioAndHTTPConfigs(t *testing.T) {
	codex, err := renderMCPClientConfig(mcpSetupOptions{
		Provider:    "codex",
		Transport:   "stdio",
		ServerName:  "breyta-ws-acme",
		WorkspaceID: "ws-acme",
		APIURL:      "https://flows.breyta.ai",
		TokenEnvVar: "BREYTA_MCP_TOKEN",
		Policy:      mcpPolicyOptions{Toolsets: "read,setup,debug,feedback", ReadOnly: true},
	})
	if err != nil {
		t.Fatalf("render codex: %v", err)
	}
	for _, want := range []string{
		"[mcp_servers.breyta_ws_acme]",
		"command = \"breyta\"",
		"\"mcp\", \"stdio\", \"--workspace-id\", \"ws-acme\"",
		"\"--token-env-var\", \"BREYTA_MCP_TOKEN\"",
		"\"--read-only\"",
		"env_vars = [\"BREYTA_MCP_TOKEN\"]",
	} {
		if !strings.Contains(codex, want) {
			t.Fatalf("codex config missing %q:\n%s", want, codex)
		}
	}
	if strings.Contains(codex, "secret") {
		t.Fatalf("codex config should not embed a secret:\n%s", codex)
	}

	opencode, err := renderMCPClientConfig(mcpSetupOptions{
		Provider:    "opencode",
		Transport:   "http",
		ServerName:  "breyta-ws-acme",
		WorkspaceID: "ws-acme",
		APIURL:      "https://flows.breyta.ai",
		TokenEnvVar: "BREYTA_MCP_TOKEN",
		Policy:      mcpPolicyOptions{Toolsets: "read,feedback"},
	})
	if err != nil {
		t.Fatalf("render opencode: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(opencode), &parsed); err != nil {
		t.Fatalf("opencode config is invalid json: %v\n%s", err, opencode)
	}
	server := mapStringAny(mapStringAny(parsed["mcp"])["breyta-ws-acme"])
	if server["type"] != "remote" || server["oauth"] != false {
		t.Fatalf("unexpected opencode server: %#v", server)
	}
	headers := mapStringAny(server["headers"])
	if headers["Authorization"] != "Bearer {env:BREYTA_MCP_TOKEN}" {
		t.Fatalf("unexpected auth header placeholder: %#v", headers)
	}
	if headers["X-MCP-Toolsets"] != "read,feedback" {
		t.Fatalf("missing toolsets header: %#v", headers)
	}
}

func TestMCPDoctorChecksInitializeAndToolsList(t *testing.T) {
	var methods []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/ws-acme/mcp" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer doctor-key" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.Header.Get("X-MCP-Readonly"); got != "true" {
			t.Fatalf("expected readonly header, got %q", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		method := firstNonBlankString(body["method"])
		methods = append(methods, method)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"protocolVersion": "2025-11-25", "serverInfo": map[string]any{"name": "breyta-workspace-mcp"}},
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"tools": []map[string]any{{"name": "search_flows"}, {"name": "send_feedback"}}},
			})
		default:
			t.Fatalf("unexpected method: %s", method)
		}
	}))
	defer srv.Close()

	result, err := runMCPDoctor(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "doctor-key",
		Policy:      mcpPolicyOptions{ReadOnly: true},
	})
	if err != nil {
		t.Fatalf("runMCPDoctor: %v", err)
	}
	if strings.Join(methods, ",") != "initialize,tools/list" {
		t.Fatalf("unexpected methods: %#v", methods)
	}
	if result["toolCount"] != 2 {
		t.Fatalf("unexpected doctor result: %#v", result)
	}
}

func TestMCPWorkspaceBindingMustBeExplicit(t *testing.T) {
	app := &App{WorkspaceID: "config-default"}
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"mcp", "config"})
	if _, err := explicitMCPWorkspaceID(cmd, app, ""); err == nil {
		t.Fatalf("expected missing explicit workspace error")
	}
	got, err := explicitMCPWorkspaceID(cmd, app, "ws-explicit")
	if err != nil {
		t.Fatalf("explicit workspace rejected: %v", err)
	}
	if got != "ws-explicit" {
		t.Fatalf("unexpected workspace: %q", got)
	}
}

func TestMCPTokenEnvVarSuppliesProxyToken(t *testing.T) {
	t.Setenv("BREYTA_MCP_TOKEN", "env-secret-key")
	app := &App{Token: "ambient-cli-token"}
	if err := applyMCPTokenEnvVar(NewRootCmd(), app, "BREYTA_MCP_TOKEN"); err != nil {
		t.Fatalf("applyMCPTokenEnvVar: %v", err)
	}
	if app.Token != "env-secret-key" || !app.TokenExplicit || !app.APIKeyExplicit {
		t.Fatalf("token env var did not populate explicit app token: %#v", app)
	}
	if err := applyMCPTokenEnvVar(NewRootCmd(), &App{}, "MISSING_BREYTA_TOKEN"); err == nil {
		t.Fatalf("expected missing env var error")
	}
}

func TestMCPTokenEnvVarDoesNotOverrideExplicitTokenFlag(t *testing.T) {
	t.Setenv("BREYTA_MCP_TOKEN", "env-secret-key")
	app := &App{Token: "flag-token"}
	cmd := NewRootCmd()
	if err := cmd.PersistentFlags().Set("token", "flag-token"); err != nil {
		t.Fatalf("set token flag: %v", err)
	}
	if err := applyMCPTokenEnvVar(cmd, app, "BREYTA_MCP_TOKEN"); err != nil {
		t.Fatalf("applyMCPTokenEnvVar: %v", err)
	}
	if app.Token != "flag-token" {
		t.Fatalf("token env var overrode explicit flag token: %#v", app)
	}
}

func TestMCPStdioCommandTokenEnvVarOverridesAmbientToken(t *testing.T) {
	t.Setenv("BREYTA_TOKEN", "ambient-user-token")
	t.Setenv("BREYTA_MCP_TOKEN", "service-account-token")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var seenAuth string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      "one",
			"result":  map[string]any{"ok": true},
		})
	}))
	defer srv.Close()

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(`{"jsonrpc":"2.0","id":"one","method":"ping"}` + "\n"))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"--api", srv.URL,
		"mcp", "stdio",
		"--workspace-id", "ws-acme",
		"--token-env-var", "BREYTA_MCP_TOKEN",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mcp stdio failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if seenAuth != "Bearer service-account-token" {
		t.Fatalf("expected token-env-var auth, got %q", seenAuth)
	}
	if strings.Contains(stdout.String(), "service-account-token") || strings.Contains(stderr.String(), "service-account-token") {
		t.Fatalf("secret leaked\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestMCPErrorRedactionIncludesActiveProxyToken(t *testing.T) {
	got := sanitizeMCPError("request failed with custom-secret-token", "custom-secret-token")
	if strings.Contains(got, "custom-secret-token") {
		t.Fatalf("active token was not redacted: %s", got)
	}
}

func TestMCPConfigCommandPrintsProviderSnippetWithoutNetwork(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"mcp", "config",
		"--workspace-id", "ws-acme",
		"--provider", "codex",
		"--transport", "stdio",
		"--name", "breyta-ws-acme",
		"--read-only",
		"--toolsets", "read,setup,feedback",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mcp config failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "warning:") {
		t.Fatalf("mcp config should not emit background network warnings: %s", stderr.String())
	}
	for _, want := range []string{
		"[mcp_servers.breyta_ws_acme]",
		"\"--workspace-id\", \"ws-acme\"",
		"\"--token-env-var\", \"BREYTA_MCP_TOKEN\"",
		"env_vars = [\"BREYTA_MCP_TOKEN\"]",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestMCPInitAliasPrintsProviderSnippet(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"mcp", "init",
		"--workspace-id", "ws-acme",
		"--provider", "generic",
		"--transport", "stdio",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mcp init failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"--workspace-id"`) || !strings.Contains(stdout.String(), `"ws-acme"`) {
		t.Fatalf("missing workspace-bound stdio config:\n%s", stdout.String())
	}
}

func TestInitMCPCanPrintOnlyMCPConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("BREYTA_AUTH_STORE", filepath.Join(tmp, "auth.json"))
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"init",
		"--no-skill",
		"--no-workspace",
		"--mcp",
		"--mcp-workspace-id", "ws-acme",
		"--mcp-provider", "generic",
		"--mcp-transport", "stdio",
		"--mcp-read-only",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --mcp failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "MCP config (generic/stdio, workspace ws-acme):") {
		t.Fatalf("missing mcp heading:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"command": "breyta"`) || !strings.Contains(stdout.String(), `"--workspace-id"`) {
		t.Fatalf("missing stdio config:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "bsa_") || strings.Contains(stderr.String(), "bsa_") {
		t.Fatalf("secret-like value leaked\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}
