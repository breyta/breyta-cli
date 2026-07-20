package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePushTimeoutFlow(t *testing.T, slug string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.clj")
	literal := `{:slug :` + slug + `
 :name "Push timeout"
 :concurrency {:type :singleton :on-new-version :supersede}
 :flow '(flow/input)}
`
	if err := os.WriteFile(path, []byte(literal), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}
	return path
}

func TestFlowsPush_TimeoutAfterDraftSaveExplainsSafeRecovery(t *testing.T) {
	flowFile := writePushTimeoutFlow(t, "push-timeout-recovery")
	validationStarted := make(chan struct{})

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["command"] {
		case "flows.put_draft":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "push-timeout-recovery", "savedDraft": true},
			})
		case "flows.validate":
			close(validationStarted)
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": map[string]any{"message": "unexpected command"},
			})
		}
	}))
	defer srv.Close()

	type result struct {
		stdout string
		stderr string
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		stdout, stderr, err := runCLIArgs(t,
			"--dev",
			"--workspace", "ws-acme",
			"--api", srv.URL,
			"--token", "user-dev",
			"flows", "push",
			"--file", flowFile,
			"--timeout", "1s",
		)
		resultCh <- result{stdout: stdout, stderr: stderr, err: err}
	}()
	select {
	case <-validationStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("expected validation request to reach the server")
	}
	outcome := <-resultCh
	if outcome.err == nil {
		t.Fatalf("expected push timeout, got success:\n%s", outcome.stdout)
	}
	message := outcome.stderr + outcome.err.Error()
	for _, expected := range []string{
		"flows push timed out after 1s while validating saved draft push-timeout-recovery",
		"the draft was already saved",
		"breyta flows show push-timeout-recovery",
		"breyta flows validate push-timeout-recovery",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("timeout recovery message missing %q:\n%s", expected, message)
		}
	}
}

func TestFlowsPush_TimeoutFlagBoundsDraftUpload(t *testing.T) {
	flowFile := writePushTimeoutFlow(t, "push-timeout-upload")
	uploadStarted := make(chan struct{})

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		close(uploadStarted)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	type result struct {
		stdout string
		stderr string
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		stdout, stderr, err := runCLIArgs(t,
			"--dev",
			"--workspace", "ws-acme",
			"--api", srv.URL,
			"--token", "user-dev",
			"flows", "push",
			"--file", flowFile,
			"--validate=false",
			"--timeout", "1s",
		)
		resultCh <- result{stdout: stdout, stderr: stderr, err: err}
	}()
	select {
	case <-uploadStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("expected draft upload request to reach the server")
	}
	outcome := <-resultCh
	if outcome.err == nil {
		t.Fatalf("expected push timeout, got success:\n%s", outcome.stdout)
	}
	message := outcome.stderr + outcome.err.Error()
	if !strings.Contains(message, "flows push timed out after 1s while saving push-timeout-upload") {
		t.Fatalf("timeout upload message missing recovery context:\n%s", message)
	}
}

func TestFlowsPush_GatewayTimeoutResponseExplainsSafeRecovery(t *testing.T) {
	flowFile := writePushTimeoutFlow(t, "push-timeout-response")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusGatewayTimeout)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error":       map[string]any{"code": "gateway_timeout", "message": "upstream timed out"},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "push",
		"--file", flowFile,
		"--validate=false",
		"--timeout", "1s",
	)
	if err == nil {
		t.Fatalf("expected gateway timeout, got success:\n%s", stdout)
	}
	message := stderr + err.Error()
	for _, expected := range []string{
		"flows push timed out while saving push-timeout-response",
		"the draft may already have been saved",
		"breyta flows show push-timeout-response",
		"breyta flows validate push-timeout-response",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("gateway timeout recovery message missing %q:\n%s", expected, message)
		}
	}
	if strings.Contains(message, "after 1s") {
		t.Fatalf("gateway timeout response should not claim the client deadline elapsed:\n%s", message)
	}
	if !strings.Contains(stdout, "timeoutRecovery") {
		t.Fatalf("expected structured timeout recovery metadata:\n%s", stdout)
	}
}

func TestFlowsPush_NonJSONGatewayTimeoutStillExplainsSafeRecovery(t *testing.T) {
	flowFile := writePushTimeoutFlow(t, "push-timeout-html-response")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte("<html><body>upstream timeout</body></html>"))
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "push",
		"--file", flowFile,
		"--validate=false",
		"--timeout", "1s",
	)
	if err == nil {
		t.Fatalf("expected gateway timeout, got success:\n%s", stdout)
	}
	message := stderr + err.Error()
	for _, expected := range []string{
		"flows push timed out while saving push-timeout-html-response",
		"the draft may already have been saved",
		"breyta flows show push-timeout-html-response",
		"breyta flows validate push-timeout-html-response",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("non-JSON gateway timeout recovery message missing %q:\n%s", expected, message)
		}
	}
	if strings.Contains(message, "after 1s") {
		t.Fatalf("non-JSON gateway timeout response should not claim the client deadline elapsed:\n%s", message)
	}
	if !strings.Contains(stdout, "timeoutRecovery") {
		t.Fatalf("expected structured timeout recovery metadata:\n%s", stdout)
	}
}
