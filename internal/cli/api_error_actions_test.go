package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeAPIErrorEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	return out
}

func TestAPIErrorActions_ServerProvidedActionSetsMetaWebURLAndStderr(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error": map[string]any{
				"code":    "billing_overage_limit_reached",
				"message": "Run blocked by billing policy: significant overage reached.",
				"actions": []map[string]any{
					{
						"kind":  "billing",
						"label": "Billing",
						"url":   "https://flows.breyta.ai/ws-acme/billing",
					},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "demo-flow",
	)
	if err == nil {
		t.Fatalf("expected flows run to fail")
	}

	out := decodeAPIErrorEnvelope(t, stdout)
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != "https://flows.breyta.ai/ws-acme/billing" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	errMap, _ := out["error"].(map[string]any)
	actions, _ := errMap["actions"].([]any)
	first, _ := actions[0].(map[string]any)
	if got, _ := first["url"].(string); got != "https://flows.breyta.ai/ws-acme/billing" {
		t.Fatalf("unexpected first action url: %q", got)
	}

	openPos := strings.Index(stderr, "Open Billing: https://flows.breyta.ai/ws-acme/billing")
	errorPos := strings.Index(stderr, "api error (status=402): Run blocked by billing policy: significant overage reached.")
	hintPos := strings.Index(stderr, "Hint: run `breyta help flows run` for usage or `breyta docs find \"flows run\"` for docs.")
	if openPos == -1 {
		t.Fatalf("stderr missing billing action:\n%s", stderr)
	}
	if errorPos == -1 {
		t.Fatalf("stderr missing api error line:\n%s", stderr)
	}
	if hintPos == -1 || !(errorPos < openPos && openPos < hintPos) {
		t.Fatalf("expected api error, then billing action, then generic hint:\n%s", stderr)
	}
}

func TestAPIErrorActions_LegacyBillingFallbackOverridesGenericActivationURL(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error": map[string]any{
				"code":    "billing_overage_limit_reached",
				"message": "Run blocked by billing policy: significant overage reached.",
				"details": map[string]any{
					"flowSlug":   "demo-flow",
					"billingUrl": "https://flows.breyta.ai/ws-acme/billing",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "demo-flow",
	)
	if err == nil {
		t.Fatalf("expected flows run to fail")
	}

	out := decodeAPIErrorEnvelope(t, stdout)
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != "https://flows.breyta.ai/ws-acme/billing" {
		t.Fatalf("expected billing page to win as meta.webUrl, got %q", got)
	}
	errMap, _ := out["error"].(map[string]any)
	actions, _ := errMap["actions"].([]any)
	first, _ := actions[0].(map[string]any)
	if got, _ := first["kind"].(string); got != "billing" {
		t.Fatalf("expected first action to be billing, got %q", got)
	}
	if !strings.Contains(stderr, "Open Billing: https://flows.breyta.ai/ws-acme/billing") {
		t.Fatalf("stderr missing billing fallback:\n%s", stderr)
	}
}

func TestAPIErrorActions_ConnectionFallbackBuildsEditURL(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error": map[string]any{
				"code":    "service_unavailable",
				"message": "Connection validation unavailable",
				"details": map[string]any{
					"flowSlug": "demo-flow",
					"target":   "live",
					"invalidConnectionBindings": []map[string]any{
						{
							"slot":         "gmail",
							"connectionId": "conn-123",
							"error":        "kms timeout",
							"errorType":    "service-unavailable",
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "configure", "check", "demo-flow",
		"--target", "live",
	)
	if err == nil {
		t.Fatalf("expected flows configure check to fail")
	}

	out := decodeAPIErrorEnvelope(t, stdout)
	meta, _ := out["meta"].(map[string]any)
	wantURL := srv.URL + "/ws-acme/connections/conn-123/edit"
	if got, _ := meta["webUrl"].(string); got != wantURL {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	errMap, _ := out["error"].(map[string]any)
	actions, _ := errMap["actions"].([]any)
	first, _ := actions[0].(map[string]any)
	if got, _ := first["kind"].(string); got != "connection-edit" {
		t.Fatalf("expected connection-edit action, got %q", got)
	}
	if got, _ := first["url"].(string); got != wantURL {
		t.Fatalf("unexpected connection edit url: %q", got)
	}
	if !strings.Contains(stderr, "Open Edit connection (gmail): "+wantURL) {
		t.Fatalf("stderr missing connection edit guidance:\n%s", stderr)
	}
}

func TestAPIErrorActions_DraftBindingsHintWinsForDraftProfileMissing(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error": map[string]any{
				"code":    "profile_missing",
				"message": "Flow requires a profile before running.",
				"details": map[string]any{
					"flowSlug": "demo-flow",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "start",
		"--flow", "demo-flow",
		"--source", "draft",
	)
	if err == nil {
		t.Fatalf("expected runs start to fail")
	}

	out := decodeAPIErrorEnvelope(t, stdout)
	meta, _ := out["meta"].(map[string]any)
	wantURL := srv.URL + "/ws-acme/flows/demo-flow/draft-bindings"
	if got, _ := meta["webUrl"].(string); got != wantURL {
		t.Fatalf("expected draft bindings page as meta.webUrl, got %q", got)
	}
	errMap, _ := out["error"].(map[string]any)
	actions, _ := errMap["actions"].([]any)
	first, _ := actions[0].(map[string]any)
	if got, _ := first["kind"].(string); got != "draft-bindings" {
		t.Fatalf("expected first action to be draft-bindings, got %q", got)
	}
	if got, _ := first["url"].(string); got != wantURL {
		t.Fatalf("unexpected first action url: %q", got)
	}
	if !strings.Contains(stderr, "Open Draft bindings: "+wantURL) {
		t.Fatalf("stderr missing draft bindings guidance:\n%s", stderr)
	}
}

func TestAPIErrorActions_FlowsRunDefaultDraftGetsDraftBindingsHint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error": map[string]any{
				"code":    "profile_missing",
				"message": "Flow requires a profile before running.",
				"details": map[string]any{
					"flowSlug": "demo-flow",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "demo-flow",
	)
	if err == nil {
		t.Fatalf("expected flows run to fail")
	}

	out := decodeAPIErrorEnvelope(t, stdout)
	meta, _ := out["meta"].(map[string]any)
	wantURL := srv.URL + "/ws-acme/flows/demo-flow/draft-bindings"
	if got, _ := meta["webUrl"].(string); got != wantURL {
		t.Fatalf("expected draft bindings page as meta.webUrl, got %q", got)
	}
	errMap, _ := out["error"].(map[string]any)
	actions, _ := errMap["actions"].([]any)
	first, _ := actions[0].(map[string]any)
	if got, _ := first["kind"].(string); got != "draft-bindings" {
		t.Fatalf("expected first action to be draft-bindings, got %q", got)
	}
	if got, _ := first["url"].(string); got != wantURL {
		t.Fatalf("unexpected first action url: %q", got)
	}
	if !strings.Contains(stderr, "Open Draft bindings: "+wantURL) {
		t.Fatalf("stderr missing draft bindings guidance:\n%s", stderr)
	}
}
