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

func TestMCPStdioProxyDecodesSSEAndPreservesSessionID(t *testing.T) {
	var requestCount int
	var secondSessionID string
	var secondProtocolVersion string

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch requestCount {
		case 1:
			if got := r.Header.Get("Mcp-Session-Id"); got != "" {
				t.Fatalf("first request should not send session id, got %q", got)
			}
			w.Header().Set("Mcp-Session-Id", "session-123")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: message\n"))
			_, _ = w.Write([]byte(`data: {"jsonrpc":"2.0","id":"one","result":{"ok":true}}` + "\n\n"))
		case 2:
			secondSessionID = r.Header.Get("Mcp-Session-Id")
			secondProtocolVersion = r.Header.Get("MCP-Protocol-Version")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"ok": true},
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		In: strings.NewReader(
			`{"jsonrpc":"2.0","id":"one","method":"initialize","params":{"protocolVersion":"2025-11-25"}}` + "\n" +
				`{"jsonrpc":"2.0","id":"two","method":"tools/list"}` + "\n"),
		Out: &stdout,
		Err: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	if secondSessionID != "session-123" {
		t.Fatalf("second request did not preserve session id: %q", secondSessionID)
	}
	if secondProtocolVersion != "2025-11-25" {
		t.Fatalf("second request did not preserve MCP protocol version: %q", secondProtocolVersion)
	}
	out := strings.TrimSpace(stdout.String())
	if strings.Contains(out, "event:") || strings.Contains(out, "data:") {
		t.Fatalf("stdio output leaked SSE framing:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two JSON-RPC output lines, got %d:\n%s", len(lines), out)
	}
	for _, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("invalid JSON-RPC output line %q: %v", line, err)
		}
	}
}

func TestMCPStdioProxyUsesNegotiatedProtocolVersion(t *testing.T) {
	var requestCount int
	var secondProtocolVersion string

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch requestCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"protocolVersion": "2025-06-18"},
			})
		case 2:
			secondProtocolVersion = r.Header.Get("MCP-Protocol-Version")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"ok": true},
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		In: strings.NewReader(
			`{"jsonrpc":"2.0","id":"one","method":"initialize","params":{"protocolVersion":"2025-11-25"}}` + "\n" +
				`{"jsonrpc":"2.0","id":"two","method":"tools/list"}` + "\n"),
		Out: &stdout,
		Err: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	if secondProtocolVersion != "2025-06-18" {
		t.Fatalf("second request did not use negotiated MCP protocol version: %q", secondProtocolVersion)
	}
}

func TestMCPStdioProxyUsesNegotiatedProtocolVersionFromMultiMessageSSE(t *testing.T) {
	var requestCount int
	var secondProtocolVersion string

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch requestCount {
		case 1:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(`event: message
data: {"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info","data":"starting"}}

event: message
data: {"jsonrpc":"2.0","id":"one","result":{"protocolVersion":"2025-06-18"}}

`))
		case 2:
			secondProtocolVersion = r.Header.Get("MCP-Protocol-Version")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"ok": true},
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		In: strings.NewReader(
			`{"jsonrpc":"2.0","id":"one","method":"initialize","params":{"protocolVersion":"2025-11-25"}}` + "\n" +
				`{"jsonrpc":"2.0","id":"two","method":"tools/list"}` + "\n"),
		Out: &stdout,
		Err: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	if secondProtocolVersion != "2025-06-18" {
		t.Fatalf("second request did not use streamed negotiated MCP protocol version: %q\nstdout=%s", secondProtocolVersion, stdout.String())
	}
}

func TestMCPStdioProxyMirrorsAnnotatedToolArgumentsToHTTPHeaders(t *testing.T) {
	var requestCount int
	var seenHeaders http.Header

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch requestCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result": map[string]any{"tools": []map[string]any{{
					"name": "search_flows",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"workspace_id": map[string]any{"type": "string", "x-mcp-header": "Workspace-Id"},
							"priority":     map[string]any{"type": "boolean", "x-mcp-header": "Priority"},
							"page":         map[string]any{"type": "number", "x-mcp-header": "Page"},
							"external_id":  map[string]any{"type": "integer", "x-mcp-header": "External-Id"},
							"query":        map[string]any{"type": "string"},
						},
					},
				}, {
					"name": "bad_tool",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"bad": map[string]any{"type": "string", "x-mcp-header": "Bad Header"},
						},
					},
				}}},
			})
		case 2:
			seenHeaders = r.Header.Clone()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"ok": true},
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		In: strings.NewReader(
			`{"jsonrpc":"2.0","id":"tools","method":"tools/list"}` + "\n" +
				`{"jsonrpc":"2.0","id":"call","method":"tools/call","params":{"name":"search_flows","arguments":{"workspace_id":" ws-acme","priority":true,"page":42,"external_id":9007199254740993,"query":"ignored","bad":"ignored"}}}` + "\n"),
		Out: &stdout,
		Err: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	toolsMessage := mcpJSONRPCMessageByID([]byte(stdout.String()), "tools")
	if toolsMessage == nil {
		t.Fatalf("missing tools/list response: %s", stdout.String())
	}
	tools := sliceAny(mapStringAny(toolsMessage["result"])["tools"])
	if len(tools) != 1 || firstNonBlankString(mapStringAny(tools[0])["name"]) != "search_flows" {
		t.Fatalf("invalid x-mcp-header tool was not filtered from tools/list: %#v", tools)
	}
	if seenHeaders.Get("Mcp-Name") != "search_flows" {
		t.Fatalf("unexpected tool name header: %q", seenHeaders.Get("Mcp-Name"))
	}
	if seenHeaders.Get("Mcp-Param-Workspace-Id") != "=?base64?IHdzLWFjbWU=?=" {
		t.Fatalf("workspace header was not base64 encoded: %q", seenHeaders.Get("Mcp-Param-Workspace-Id"))
	}
	if seenHeaders.Get("Mcp-Param-Priority") != "true" {
		t.Fatalf("priority header was not mirrored: %q", seenHeaders.Get("Mcp-Param-Priority"))
	}
	if seenHeaders.Get("Mcp-Param-Page") != "42" {
		t.Fatalf("page header was not mirrored: %q", seenHeaders.Get("Mcp-Param-Page"))
	}
	if seenHeaders.Get("Mcp-Param-External-Id") != "9007199254740993" {
		t.Fatalf("large integer header lost precision: %q", seenHeaders.Get("Mcp-Param-External-Id"))
	}
	if seenHeaders.Get("Mcp-Param-Query") != "" {
		t.Fatalf("unannotated argument was mirrored: %#v", seenHeaders.Values("Mcp-Param-Query"))
	}
	if seenHeaders.Get("Mcp-Param-Bad Header") != "" {
		t.Fatalf("invalid annotated header name was mirrored")
	}
}

func TestMCPStdioProxyRefreshesAnnotatedHeaderRegistry(t *testing.T) {
	var requestCount int
	var firstCallHeaders http.Header
	var secondCallHeaders http.Header

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch requestCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result": map[string]any{"tools": []map[string]any{{
					"name": "search_flows",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"workspace_id": map[string]any{"type": "string", "x-mcp-header": "Workspace-Id"},
						},
					},
				}}},
			})
		case 2:
			firstCallHeaders = r.Header.Clone()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"ok": true},
			})
		case 3:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result": map[string]any{"tools": []map[string]any{{
					"name": "search_flows",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"workspace_id": map[string]any{"type": "string"},
						},
					},
				}}},
			})
		case 4:
			secondCallHeaders = r.Header.Clone()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      body["id"],
				"result":  map[string]any{"ok": true},
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		In: strings.NewReader(
			`{"jsonrpc":"2.0","id":"tools-one","method":"tools/list"}` + "\n" +
				`{"jsonrpc":"2.0","id":"call-one","method":"tools/call","params":{"name":"search_flows","arguments":{"workspace_id":"ws-acme"}}}` + "\n" +
				`{"jsonrpc":"2.0","id":"tools-two","method":"tools/list"}` + "\n" +
				`{"jsonrpc":"2.0","id":"call-two","method":"tools/call","params":{"name":"search_flows","arguments":{"workspace_id":"ws-acme"}}}` + "\n"),
		Out: &stdout,
		Err: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	if firstCallHeaders.Get("Mcp-Param-Workspace-Id") != "ws-acme" {
		t.Fatalf("first call did not mirror annotated header: %#v", firstCallHeaders)
	}
	if secondCallHeaders.Get("Mcp-Param-Workspace-Id") != "" {
		t.Fatalf("fresh tools/list did not clear stale annotated header mapping: %#v", secondCallHeaders)
	}
}

func TestMCPStdioProxyDecodesCRLFSSEEvents(t *testing.T) {
	got, err := decodeMCPSSEData([]byte("event: message\r\ndata: {\"one\":true}\r\n\r\nevent: message\r\ndata: {\"two\":true}\r\n\r\n"))
	if err != nil {
		t.Fatalf("decodeMCPSSEData: %v", err)
	}
	if string(got) != "{\"one\":true}\n{\"two\":true}" {
		t.Fatalf("unexpected decoded SSE payloads: %q", string(got))
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

func TestMCPStdioProxyLogsNotificationHTTPError(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "token secret-api-key rejected"},
		})
	}))
	defer srv.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runMCPStdioProxy(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "secret-api-key",
		In:          strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"),
		Out:         &stdout,
		Err:         &stderr,
	})
	if err != nil {
		t.Fatalf("runMCPStdioProxy: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected no notification response, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "upstream notification failed") || !strings.Contains(stderr.String(), "[redacted] rejected") {
		t.Fatalf("missing sanitized notification failure log: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "secret-api-key") {
		t.Fatalf("secret leaked in stderr: %s", stderr.String())
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

func TestMCPDoctorRedactsUpstreamErrors(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "doctor-key is not allowed"},
		})
	}))
	defer srv.Close()

	_, err := runMCPDoctor(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "doctor-key",
	})
	if err == nil {
		t.Fatalf("expected doctor failure")
	}
	if strings.Contains(err.Error(), "doctor-key") {
		t.Fatalf("doctor error leaked token: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted] is not allowed") {
		t.Fatalf("doctor error was not sanitized: %v", err)
	}
}

func TestMCPDoctorRedactsMalformedSuccessResponses(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unexpected doctor-key body"))
	}))
	defer srv.Close()

	result, err := runMCPDoctor(context.Background(), mcpProxyOptions{
		WorkspaceID: "ws-acme",
		APIURL:      srv.URL,
		Token:       "doctor-key",
	})
	if err == nil {
		t.Fatalf("expected doctor failure")
	}
	if strings.Contains(firstNonBlankString(result["response"]), "doctor-key") {
		t.Fatalf("doctor response leaked token: %#v", result)
	}
	if !strings.Contains(firstNonBlankString(result["response"]), "[redacted]") {
		t.Fatalf("doctor response was not redacted: %#v", result)
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

	codexHTTP, err := renderMCPClientConfig(mcpSetupOptions{
		Provider:    "codex",
		Transport:   "http",
		ServerName:  "breyta-ws-acme",
		WorkspaceID: "ws-acme",
		APIURL:      "https://flows.breyta.ai",
		TokenEnvVar: "BREYTA_MCP_TOKEN",
		Policy:      mcpPolicyOptions{Toolsets: "read,feedback", ReadOnly: true},
	})
	if err != nil {
		t.Fatalf("render codex http: %v", err)
	}
	for _, want := range []string{
		"[mcp_servers.breyta_ws_acme]",
		"bearer_token_env_var = \"BREYTA_MCP_TOKEN\"",
		"[mcp_servers.breyta_ws_acme.http_headers]",
		"\"X-MCP-Readonly\" = \"true\"",
		"\"X-MCP-Toolsets\" = \"read,feedback\"",
	} {
		if !strings.Contains(codexHTTP, want) {
			t.Fatalf("codex http config missing %q:\n%s", want, codexHTTP)
		}
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

func TestMCPConfigEscapesProviderSpecificConfigValues(t *testing.T) {
	codex, err := renderMCPClientConfig(mcpSetupOptions{
		Provider:    "codex",
		Transport:   "stdio",
		ServerName:  "breyta]\nmalicious = true",
		WorkspaceID: "ws-acme",
		APIURL:      "https://flows.breyta.ai",
		TokenEnvVar: "BREYTA_MCP_TOKEN",
	})
	if err != nil {
		t.Fatalf("render codex: %v", err)
	}
	if !strings.Contains(codex, "[mcp_servers.breyta_malicious_true]") {
		t.Fatalf("codex config did not sanitize table name:\n%s", codex)
	}
	if strings.Contains(codex, "malicious = true") {
		t.Fatalf("codex config allowed table-name injection:\n%s", codex)
	}

	continueConfig, err := renderMCPClientConfig(mcpSetupOptions{
		Provider:    "continue",
		Transport:   "http",
		ServerName:  "breyta: workspace\n- bad",
		WorkspaceID: "ws-acme",
		APIURL:      "https://flows.breyta.ai",
		TokenEnvVar: "BREYTA_MCP_TOKEN",
		Policy:      mcpPolicyOptions{Toolsets: "read:\n- bad", ReadOnly: true},
	})
	if err != nil {
		t.Fatalf("render continue: %v", err)
	}
	for _, want := range []string{
		`name: "breyta: workspace\n- bad"`,
		`Authorization: "Bearer ${env:BREYTA_MCP_TOKEN}"`,
		`"X-MCP-Toolsets": "read:\n- bad"`,
	} {
		if !strings.Contains(continueConfig, want) {
			t.Fatalf("continue config missing escaped value %q:\n%s", want, continueConfig)
		}
	}
}

func TestMCPDoctorChecksInitializeAndToolsList(t *testing.T) {
	var methods []string
	var initializedSessionID string
	var initializedProtocolVersion string
	var toolsListSessionID string
	var toolsListProtocolVersion string
	var sawInitializeCapabilities bool
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
			params := mapStringAny(body["params"])
			if _, ok := params["capabilities"].(map[string]any); ok {
				sawInitializeCapabilities = true
			}
			w.Header().Set("Mcp-Session-Id", "doctor-session-123")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(`event: message
data: {"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info","data":"initializing"}}

event: message
data: {"jsonrpc":"2.0","id":"breyta-mcp-doctor-init","result":{"protocolVersion":"2025-11-25","serverInfo":{"name":"breyta-workspace-mcp"}}}

`))
		case "notifications/initialized":
			initializedSessionID = r.Header.Get("Mcp-Session-Id")
			initializedProtocolVersion = r.Header.Get("MCP-Protocol-Version")
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			toolsListSessionID = r.Header.Get("Mcp-Session-Id")
			toolsListProtocolVersion = r.Header.Get("MCP-Protocol-Version")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(`event: message
data: {"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info","data":"listing tools"}}

event: message
data: {"jsonrpc":"2.0","id":"breyta-mcp-doctor-tools","result":{"tools":[{"name":"search_flows"},{"name":"send_feedback"}]}}

`))
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
	if strings.Join(methods, ",") != "initialize,notifications/initialized,tools/list" {
		t.Fatalf("unexpected methods: %#v", methods)
	}
	if !sawInitializeCapabilities {
		t.Fatalf("doctor initialize request omitted capabilities")
	}
	if initializedSessionID != "doctor-session-123" {
		t.Fatalf("doctor initialized notification did not preserve MCP session id: %q", initializedSessionID)
	}
	if initializedProtocolVersion != "2025-11-25" {
		t.Fatalf("doctor initialized notification did not preserve MCP protocol version: %q", initializedProtocolVersion)
	}
	if toolsListSessionID != "doctor-session-123" {
		t.Fatalf("doctor did not preserve MCP session id: %q", toolsListSessionID)
	}
	if toolsListProtocolVersion != "2025-11-25" {
		t.Fatalf("doctor tools/list did not preserve MCP protocol version: %q", toolsListProtocolVersion)
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
	t.Setenv("BREYTA_MCP_TOKEN", "bsa_sak-123_env-secret-key")
	app := &App{Token: "ambient-cli-token"}
	if err := applyMCPTokenEnvVar(NewRootCmd(), app, "BREYTA_MCP_TOKEN"); err != nil {
		t.Fatalf("applyMCPTokenEnvVar: %v", err)
	}
	if app.Token != "bsa_sak-123_env-secret-key" || !app.TokenExplicit || !app.APIKeyExplicit {
		t.Fatalf("token env var did not populate explicit app token: %#v", app)
	}
	if err := applyMCPTokenEnvVar(NewRootCmd(), &App{}, "MISSING_BREYTA_TOKEN"); err == nil {
		t.Fatalf("expected missing env var error")
	}
	t.Setenv("BREYTA_USER_TOKEN", "user-token")
	if err := applyMCPTokenEnvVar(NewRootCmd(), &App{}, "BREYTA_USER_TOKEN"); err == nil {
		t.Fatalf("expected non-service-account token env var error")
	}
	t.Setenv("BREYTA_FAKE_MACHINE_TOKEN", "bsa_not-service-account_secret")
	if err := applyMCPTokenEnvVar(NewRootCmd(), &App{}, "BREYTA_FAKE_MACHINE_TOKEN"); err == nil {
		t.Fatalf("expected malformed service-account token env var error")
	}
	t.Setenv("BREYTA_TOKEN", "bsa_sak-123_reserved-name")
	if err := applyMCPTokenEnvVar(NewRootCmd(), &App{}, "BREYTA_TOKEN"); err == nil {
		t.Fatalf("expected reserved BREYTA_TOKEN env var error")
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
	t.Setenv("BREYTA_MCP_TOKEN", "bsa_sak-123_service-account-token")
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
	if seenAuth != "Bearer bsa_sak-123_service-account-token" {
		t.Fatalf("expected token-env-var auth, got %q", seenAuth)
	}
	if strings.Contains(stdout.String(), "bsa_sak-123_service-account-token") || strings.Contains(stderr.String(), "bsa_sak-123_service-account-token") {
		t.Fatalf("secret leaked\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestMCPStdioCommandRejectsUserTokenEnvVarForAPIOverride(t *testing.T) {
	t.Setenv("BREYTA_TOKEN", "ambient-user-token")
	t.Setenv("BREYTA_MCP_TOKEN", "ambient-user-token")
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	called := false
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
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
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected mcp stdio to reject a non-service-account token env var")
	}
	if called {
		t.Fatalf("upstream API was called despite invalid token env var")
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "service-account API key") {
		t.Fatalf("expected service-account API key error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(combined, "ambient-user-token") {
		t.Fatalf("secret leaked in error output: %s", combined)
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

func TestMCPConfigCommandDoesNotRequireTokenEnvValue(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
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
		"--provider", "generic",
		"--transport", "stdio",
		"--token-env-var", "CUSTOM_BREYTA_MCP_TOKEN",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mcp config required token env value: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "CUSTOM_BREYTA_MCP_TOKEN") {
		t.Fatalf("custom token env var was not rendered:\n%s", stdout.String())
	}
}

func TestMCPConfigCommandRejectsUserTokenEnvName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
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
		"--provider", "generic",
		"--transport", "http",
		"--token-env-var", "BREYTA_TOKEN",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected mcp config to reject BREYTA_TOKEN")
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "reserved for user login tokens") {
		t.Fatalf("expected reserved token env var error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
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

func TestInitMCPRejectsUserTokenEnvName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
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
		"--mcp-token-env-var", "BREYTA_TOKEN",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected init --mcp to reject BREYTA_TOKEN")
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "reserved for user login tokens") {
		t.Fatalf("expected reserved token env var error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
