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
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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
	var gotIdempotency string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		gotIdempotency = r.Header.Get("Idempotency-Key")
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
	if strings.TrimSpace(gotIdempotency) == "" {
		t.Fatalf("expected idempotency header to be set")
	}
}

func TestClient_DoCommand_RetriesOn429ThenSucceeds(t *testing.T) {
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_ATTEMPTS", "2")
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_BASE_MS", "0")
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_MAX_MS", "0")
	attempts := 0
	var seen []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		seen = append(seen, r.Header.Get("Idempotency-Key"))
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "retry": true})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, WorkspaceID: "ws-acme", HTTP: srv.Client()}
	out, status, err := c.DoCommand(context.Background(), "flows.run", map[string]any{"flowSlug": "demo"})
	if err != nil {
		t.Fatalf("DoCommand: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if len(seen) != 2 || strings.TrimSpace(seen[0]) == "" || seen[0] != seen[1] {
		t.Fatalf("expected stable idempotency key across retries, got %#v", seen)
	}
	if out["ok"] != true {
		t.Fatalf("unexpected response: %#v", out)
	}
}

func TestClient_DoCommand_ReturnsLast503AfterRetryBudget(t *testing.T) {
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_ATTEMPTS", "2")
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_BASE_MS", "0")
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_MAX_MS", "0")
	attempts := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "code": "overloaded"})
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, WorkspaceID: "ws-acme", HTTP: srv.Client()}
	out, status, err := c.DoCommand(context.Background(), "flows.run", map[string]any{"flowSlug": "demo"})
	if err != nil {
		t.Fatalf("DoCommand: %v", err)
	}
	if status != 503 {
		t.Fatalf("expected 503, got %d", status)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if out["code"] != "overloaded" {
		t.Fatalf("unexpected response: %#v", out)
	}
}

func TestClient_DoCommand_RetriesOnTransportErrorThenSucceeds(t *testing.T) {
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_ATTEMPTS", "2")
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_BASE_MS", "0")
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_MAX_MS", "0")
	attempts := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		return http.DefaultTransport.RoundTrip(req)
	})

	c := Client{
		BaseURL:     srv.URL,
		WorkspaceID: "ws-acme",
		HTTP:        &http.Client{Transport: transport},
	}
	out, status, err := c.DoCommand(context.Background(), "flows.run", map[string]any{"flowSlug": "demo"})
	if err != nil {
		t.Fatalf("DoCommand: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if out["ok"] != true {
		t.Fatalf("unexpected response: %#v", out)
	}
}

func TestCommandRetryDelay_ParsesRetryAfterTimestamp(t *testing.T) {
	at := time.Now().Add(250 * time.Millisecond).UTC().Format(http.TimeFormat)
	d := commandRetryDelay(1, at)
	if d <= 0 {
		t.Fatalf("expected positive delay for retry-after timestamp, got %s", d)
	}
}

func TestCommandRetryDelay_HighAttemptDoesNotOverflow(t *testing.T) {
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_BASE_MS", "250")
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_MAX_MS", "0")

	d := commandRetryDelay(80, "")
	if d <= 0 {
		t.Fatalf("expected positive delay for high retry attempt, got %s", d)
	}

	maxExpectedMS := int64(250)*(int64(1)<<maxCommandRetryShift) + 96
	maxExpected := time.Duration(maxExpectedMS) * time.Millisecond
	if d > maxExpected {
		t.Fatalf("expected delay <= %s for bounded shift, got %s", maxExpected, d)
	}
}

func TestCommandRetryDelay_DoesNotExceedConfiguredMaxAfterJitter(t *testing.T) {
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_BASE_MS", "100")
	t.Setenv("BREYTA_CLI_COMMAND_RETRY_MAX_MS", "120")

	d := commandRetryDelay(2, "")
	if d > 120*time.Millisecond {
		t.Fatalf("expected delay <= 120ms, got %s", d)
	}
}
