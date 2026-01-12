package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

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
	var gotAuth, gotCT, gotPath, gotQuery string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
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

func TestClient_DoRootREST_AllowsNonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
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
