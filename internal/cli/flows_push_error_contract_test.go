package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFlowPushErrorContractSource(t *testing.T, slug string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.clj")
	source := `{:slug :` + slug + ` :name "Push error contract" :concurrency {:type :singleton :on-new-version :supersede} :flow '(flow/input)}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write flow source: %v", err)
	}
	return path
}

func runFlowsPushErrorContract(t *testing.T, app *App, args ...string) (string, string, error) {
	t.Helper()
	cmd := newFlowsPushCmd(app)
	cmd.SilenceUsage = true
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func parseFlowPushFailureEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode push failure envelope: %v\n%s", err, stdout)
	}
	if ok, _ := envelope["ok"].(bool); ok {
		t.Fatalf("expected failed envelope, got %#v", envelope)
	}
	return envelope
}

func TestFlowsPushTransportFailureWritesStableErrorEnvelope(t *testing.T) {
	flowFile := writeFlowPushErrorContractSource(t, "push-transport-error")
	app := &App{
		WorkspaceID: "ws-acme",
		APIURL:      "https://api.example.test",
		Token:       "test-token",
		HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection interrupted")
		})},
	}

	stdout, stderr, err := runFlowsPushErrorContract(t, app, "--file", flowFile, "--validate=false")
	if err == nil {
		t.Fatalf("expected transport failure, got success:\n%s", stdout)
	}
	envelope := parseFlowPushFailureEnvelope(t, stdout)
	errMap := mapStringAny(envelope["error"])
	if got := firstNonBlankString(errMap["code"]); got != "flows_push_save_failed" {
		t.Fatalf("expected stable save failure code, got %q in %#v", got, envelope)
	}
	meta := mapStringAny(envelope["meta"])
	if got := firstNonBlankString(meta["failurePhase"]); got != "saving" {
		t.Fatalf("expected saving phase, got %q in %#v", got, envelope)
	}
	if !strings.Contains(stdout, "breyta flows show push-transport-error") {
		t.Fatalf("expected recovery command in %#v", envelope)
	}
	if strings.Contains(stdout, "connection interrupted") {
		t.Fatalf("structured output must not include raw transport errors:\n%s", stdout)
	}
	if !strings.Contains(stderr, "connection interrupted") {
		t.Fatalf("stderr must retain the transport diagnostic:\n%s", stderr)
	}
}

func TestFlowsPushStringAPIErrorPreservesMessage(t *testing.T) {
	flowFile := writeFlowPushErrorContractSource(t, "push-string-api-error")
	app := &App{
		WorkspaceID: "ws-acme",
		APIURL:      "https://api.example.test",
		Token:       "test-token",
		HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return httpJSON(http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "quota exceeded",
			})
		})},
	}

	stdout, _, err := runFlowsPushErrorContract(t, app, "--file", flowFile, "--validate=false")
	if err == nil {
		t.Fatalf("expected save failure, got success:\n%s", stdout)
	}
	envelope := parseFlowPushFailureEnvelope(t, stdout)
	errMap := mapStringAny(envelope["error"])
	if got := firstNonBlankString(errMap["message"]); got != "quota exceeded" {
		t.Fatalf("expected string API error message, got %q in %#v", got, envelope)
	}
}

func TestFlowsPushValidationAPIErrorGetsFallbackErrorCode(t *testing.T) {
	flowFile := writeFlowPushErrorContractSource(t, "push-validation-error")
	requests := 0
	app := &App{
		WorkspaceID: "ws-acme",
		APIURL:      "https://api.example.test",
		Token:       "test-token",
		HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			requests++
			if requests == 1 {
				return httpJSON(http.StatusOK, map[string]any{
					"ok":   true,
					"data": map[string]any{"flowSlug": "push-validation-error"},
				})
			}
			return httpJSON(http.StatusBadGateway, map[string]any{
				"ok":    false,
				"error": map[string]any{"message": "upstream unavailable"},
			})
		})},
	}

	stdout, _, err := runFlowsPushErrorContract(t, app, "--file", flowFile)
	if err == nil {
		t.Fatalf("expected validation failure, got success:\n%s", stdout)
	}
	envelope := parseFlowPushFailureEnvelope(t, stdout)
	errMap := mapStringAny(envelope["error"])
	if got := firstNonBlankString(errMap["code"]); got != "flows_push_validation_failed" {
		t.Fatalf("expected stable validation failure code, got %q in %#v", got, envelope)
	}
	meta := mapStringAny(envelope["meta"])
	if got := firstNonBlankString(meta["failurePhase"]); got != "validating" {
		t.Fatalf("expected validation phase, got %q in %#v", got, envelope)
	}
	if got := firstNonBlankString(meta["draftOutcome"]); got != "saved" {
		t.Fatalf("expected saved draft outcome, got %q in %#v", got, envelope)
	}
	if !strings.Contains(stdout, "breyta flows validate push-validation-error") {
		t.Fatalf("expected validation recovery command in %#v", envelope)
	}
}
