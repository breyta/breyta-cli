package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebLinks_FlowCommandAddsWebURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"flow": map[string]any{
					"slug":          "daily-sales-report",
					"activeVersion": 2,
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "show", "daily-sales-report",
	)
	if err != nil {
		t.Fatalf("flows show failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ws-acme/flows/daily-sales-report" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	flow, _ := data["flow"].(map[string]any)
	if got, _ := flow["webUrl"].(string); got != srv.URL+"/ws-acme/flows/daily-sales-report" {
		t.Fatalf("unexpected flow.webUrl: %q", got)
	}
}

func TestWebLinks_RunCommandAddsRunURLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"run": map[string]any{
					"flowSlug":   "daily-sales-report",
					"workflowId": "wf-123",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "start", "--flow", "daily-sales-report",
	)
	if err != nil {
		t.Fatalf("runs start failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	run, _ := data["run"].(map[string]any)
	if got, _ := run["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123" {
		t.Fatalf("unexpected run.webUrl: %q", got)
	}
	if got, _ := run["outputWebUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123/output" {
		t.Fatalf("unexpected run.outputWebUrl: %q", got)
	}
}

func TestWebLinks_ConnectionRESTAddsWebURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/connections/conn-123" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "conn-123",
			"name":   "GitHub",
			"type":   "http-api",
			"status": "active",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"connections", "show", "conn-123",
	)
	if err != nil {
		t.Fatalf("connections show failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ws-acme/connections/conn-123/edit" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ws-acme/connections/conn-123/edit" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}

func TestWebLinks_ResourcesGetAbsolutizesRelativeWebURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/by-uri" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uri":    "res://v1/ws/ws-acme/result/run/wf-123/flow-output",
			"type":   "result",
			"webUrl": "/ws-acme/runs/daily-sales-report/wf-123/output",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "get", "res://v1/ws/ws-acme/result/run/wf-123/flow-output",
	)
	if err != nil {
		t.Fatalf("resources get failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123/output" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123/output" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}

func TestWebLinks_ResourcesListAbsolutizesItemWebURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{
					"uri":    "res://v1/ws/ws-acme/result/run/wf-123/flow-output",
					"type":   "result",
					"webUrl": "/ws-acme/runs/daily-sales-report/wf-123/output",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "list",
	)
	if err != nil {
		t.Fatalf("resources list failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123/output" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123/output" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("unexpected items length: %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if got, _ := first["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123/output" {
		t.Fatalf("unexpected item webUrl: %q", got)
	}
}

func TestWebLinks_ResourcesGetInfersCanonicalRunStepURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/by-uri" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uri":      "res://v1/ws/ws-acme/result/run/wf-123/step/fetch-sales/output",
			"type":     "result",
			"flowSlug": "daily-sales-report",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "get", "res://v1/ws/ws-acme/result/run/wf-123/step/fetch-sales/output",
	)
	if err != nil {
		t.Fatalf("resources get failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123?stepId=fetch-sales" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ws-acme/runs/daily-sales-report/wf-123?stepId=fetch-sales" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}
