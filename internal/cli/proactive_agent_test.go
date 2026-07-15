package cli_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProactiveAgentInitiativeParkCallsStructuredCommand(t *testing.T) {
	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"status": "parked", "initiativeId": "initiative-1"},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"agent", "initiative", "park",
	)
	if err != nil {
		t.Fatalf("park initiative failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "proactive_agent.initiative.park" {
		t.Fatalf("expected structured park command, got %#v", got["command"])
	}
	args, ok := got["args"].(map[string]any)
	if !ok || len(args) != 0 {
		t.Fatalf("expected empty command args, got %#v", got["args"])
	}
	if !strings.Contains(stdout, `"status":"parked"`) {
		t.Fatalf("expected parked response, got %s", stdout)
	}
}

func TestProactiveAgentInitiativeParkIsPublicAndRejectsArguments(t *testing.T) {
	stdout, stderr, err := runCLIArgs(t, "--help")
	if err != nil {
		t.Fatalf("root help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "agent") {
		t.Fatalf("root help should expose agent command:\n%s", stdout)
	}

	_, stderr, err = runCLIArgs(t, "agent", "initiative", "park", "unexpected")
	if err == nil {
		t.Fatal("expected positional arguments to be rejected")
	}
	if !strings.Contains(stderr, "unknown command") && !strings.Contains(stderr, "accepts 0 arg") {
		t.Fatalf("expected argument error, got %s", stderr)
	}
}

func TestProactiveAgentInitiativeParkRejectsServiceAccountCredential(t *testing.T) {
	t.Setenv("BREYTA_API_KEY", "bsa_sak-test_secret")
	requests := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"proactive-agent", "initiative", "park",
	)
	if err == nil {
		t.Fatal("expected service-account credential to be rejected")
	}
	if requests != 0 {
		t.Fatalf("expected no API request, got %d", requests)
	}
	if !strings.Contains(stderr, "requires signed-in user authentication") ||
		!strings.Contains(stderr, "unset BREYTA_API_KEY") {
		t.Fatalf("expected user-auth recovery hint, got %s", stderr)
	}
}
