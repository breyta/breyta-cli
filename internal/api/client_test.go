package api

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newLocalTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		if isLocalListenerDenied(err) {
			t.Skipf("local HTTP test server skipped: sandbox denied loopback listener creation: %v", err)
		}
		t.Fatalf("failed to start local test server: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func isLocalListenerDenied(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission denied")
}

func TestClient_baseEndpointFor_AppendsPath(t *testing.T) {
	c := Client{BaseURL: "http://example.test/base/"}
	got, err := c.baseEndpointFor("/api/me")
	if err != nil {
		t.Fatalf("baseEndpointFor: %v", err)
	}
	if got != "http://example.test/base/api/me" {
		t.Fatalf("unexpected endpoint: %q", got)
	}
}

func TestClient_endpointFor_AppendsPath(t *testing.T) {
	c := Client{BaseURL: "http://example.test", WorkspaceID: "ws/acme"}
	got, err := c.endpointFor("/api/me")
	if err != nil {
		t.Fatalf("endpointFor: %v", err)
	}
	if got != "http://example.test/api/me" {
		t.Fatalf("unexpected endpoint: %q", got)
	}
}

func TestClient_DoRootREST_SetsHeadersAndQueryAndParsesJSON(t *testing.T) {
	var gotAuth, gotClient, gotCT, gotPath, gotQuery, gotUA string
	var gotBody []byte

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotClient = r.Header.Get("X-Breyta-Client")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotUA = r.Header.Get("User-Agent")
		gotBody, _ = io.ReadAll(r.Body)

		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}
	out, status, err := c.DoRootREST(context.Background(), http.MethodPost, "/api/me", url.Values{"x": []string{"1"}}, map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("DoRootREST: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
	if gotClient != "cli" {
		t.Fatalf("unexpected client header: %q", gotClient)
	}
	if gotUA != "breyta-cli" {
		t.Fatalf("unexpected user-agent header: %q", gotUA)
	}
	if gotCT != "application/json" {
		t.Fatalf("unexpected content-type: %q", gotCT)
	}
	if gotPath != "/api/me" {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if gotQuery != "x=1" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
	if !strings.Contains(string(gotBody), "\"a\":1") {
		t.Fatalf("unexpected body: %q", string(gotBody))
	}
	m, ok := out.(map[string]any)
	if !ok || m["ok"] != true {
		t.Fatalf("unexpected response: %#v", out)
	}
}

func TestClient_DoRootRESTBytes_AllowsClientHeaderOverride(t *testing.T) {
	var gotClient, gotUA string

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClient = r.Header.Get("X-Breyta-Client")
		gotUA = r.Header.Get("User-Agent")

		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, status, err := c.DoRootRESTBytes(
		context.Background(),
		http.MethodPost,
		"/api/me",
		nil,
		[]byte(`{"ok":true}`),
		map[string]string{"X-Breyta-Client": "agent-harness", "User-Agent": "codex"},
	)
	if err != nil {
		t.Fatalf("DoRootRESTBytes: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if gotClient != "agent-harness" {
		t.Fatalf("expected override client header, got %q", gotClient)
	}
	if gotUA != "codex" {
		t.Fatalf("expected override user-agent header, got %q", gotUA)
	}
}

func TestClient_DoRootREST_AllowsNonJSONResponse(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html>nope</html>"))
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, HTTP: srv.Client()}
	out, status, err := c.DoRootREST(context.Background(), http.MethodGet, "/api/me", nil, nil)
	if err != nil {
		t.Fatalf("DoRootREST: %v", err)
	}
	if status != 404 {
		t.Fatalf("expected 404, got %d", status)
	}
	s, ok := out.(string)
	if !ok || !strings.Contains(s, "nope") {
		t.Fatalf("expected raw string response, got %#v", out)
	}
}

func TestClient_DoCommand_FiltersArgsAndSendsPayload(t *testing.T) {
	var got map[string]any
	var gotClient, gotUA string

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		gotClient = r.Header.Get("X-Breyta-Client")
		gotUA = r.Header.Get("User-Agent")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, WorkspaceID: "ws-acme", Token: "tok", HTTP: srv.Client()}
	out, status, err := c.DoCommand(context.Background(), "runs.start", map[string]any{"command": "should_drop", "x": 1})
	if err != nil {
		t.Fatalf("DoCommand: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if out["ok"] != true {
		t.Fatalf("unexpected response: %#v", out)
	}
	if gotClient != "cli" {
		t.Fatalf("unexpected client header: %q", gotClient)
	}
	if gotUA != "breyta-cli" {
		t.Fatalf("unexpected user-agent header: %q", gotUA)
	}
	if got["command"] != "runs.start" {
		t.Fatalf("unexpected payload command: %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if _, ok := args["command"]; ok {
		t.Fatalf("expected command filtered out of args")
	}
	if args["x"] != float64(1) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestClient_DoCommand_LocalMembership403BootstrapsAndRetries(t *testing.T) {
	commandCalls := 0
	bootstrapCalls := 0

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/commands":
			commandCalls++
			if got := r.Header.Get("X-Breyta-Workspace"); got != "ws-acme" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing workspace"})
				return
			}
			if commandCalls == 1 {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Access denied: not a workspace member"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"items": []any{}}})
		case "/api/debug/workspace/bootstrap":
			bootstrapCalls++
			if got := r.Header.Get("X-Breyta-Client"); got != "cli" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing client header"})
				return
			}
			if got := r.Header.Get("User-Agent"); got != "breyta-cli" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing user-agent"})
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer user-dev" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing auth"})
				return
			}
			if got := r.Header.Get("x-debug-user-id"); got != "user-dev" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing debug user"})
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["workspaceId"] != "ws-acme" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "wrong workspace"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "created": false, "member": true, "role": "admin"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, WorkspaceID: "ws-acme", Token: "user-dev", HTTP: srv.Client()}
	out, status, err := c.DoCommand(context.Background(), "flows.list", map[string]any{"limit": 1})
	if err != nil {
		t.Fatalf("DoCommand: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected retry status 200, got %d", status)
	}
	if commandCalls != 2 {
		t.Fatalf("expected command retry after bootstrap, got %d command calls", commandCalls)
	}
	if bootstrapCalls != 1 {
		t.Fatalf("expected one bootstrap call, got %d", bootstrapCalls)
	}
	meta, _ := out["meta"].(map[string]any)
	bootstrap, _ := meta["localWorkspaceBootstrap"].(map[string]any)
	if bootstrap["workspaceId"] != "ws-acme" || bootstrap["reason"] != "membership-403" {
		t.Fatalf("missing bootstrap metadata: %#v", out)
	}
}

func TestClient_DoCommand_LocalMembershipBootstrapRequiresLoopback(t *testing.T) {
	c := Client{BaseURL: "https://flows.breyta.ai", WorkspaceID: "ws-acme"}
	out := map[string]any{"error": "Access denied: not a workspace member"}
	if c.shouldAutoBootstrapLocalWorkspace(out, http.StatusForbidden) {
		t.Fatal("did not expect bootstrap for non-loopback API")
	}
}

func TestClient_DoGlobalCommand_UsesGlobalEndpointWithoutWorkspaceHeader(t *testing.T) {
	var got map[string]any
	var gotWorkspaceHeader string

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/global/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		gotWorkspaceHeader = r.Header.Get("X-Breyta-Workspace")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}
	out, status, err := c.DoGlobalCommand(context.Background(), "flows.search", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("DoGlobalCommand: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if out["ok"] != true {
		t.Fatalf("unexpected response: %#v", out)
	}
	if gotWorkspaceHeader != "" {
		t.Fatalf("expected no workspace header, got %q", gotWorkspaceHeader)
	}
	if got["command"] != "flows.search" {
		t.Fatalf("unexpected payload command: %#v", got["command"])
	}
}
