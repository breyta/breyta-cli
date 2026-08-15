package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestWebLinks_FlowCommandAddsWebURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=flows&flow=daily-sales-report" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	flow, _ := data["flow"].(map[string]any)
	if got, _ := flow["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=flows&flow=daily-sales-report" {
		t.Fatalf("unexpected flow.webUrl: %q", got)
	}
}

func TestWebLinks_RunCommandAddsRunURLs(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	run, _ := data["run"].(map[string]any)
	if got, _ := run["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected run.webUrl: %q", got)
	}
	if got, _ := run["outputWebUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected run.outputWebUrl: %q", got)
	}
}

func TestWebLinks_RunCommandReplacesHostedTopLevelWebURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"meta": map[string]any{
				"webUrl": "/ws-acme/runs/daily-sales-report/wf-123",
			},
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
	want := srv.URL + "/ui?workspace=ws-acme&page=runs&run=wf-123"
	if got, _ := meta["webUrl"].(string); got != want {
		t.Fatalf("unexpected meta.webUrl: got %q want %q", got, want)
	}
}

func TestWebLinks_RunCommandNormalizesServerProvidedOutputWebURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"run": map[string]any{
					"flowSlug":     "daily-sales-report",
					"workflowId":   "wf-123",
					"outputWebUrl": "/ws-acme/runs/daily-sales-report/wf-123/output",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
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
	data, _ := out["data"].(map[string]any)
	run, _ := data["run"].(map[string]any)
	// A server-provided hosted output link must be rewritten to the portable run inspector.
	if got, _ := run["outputWebUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected run.outputWebUrl: %q", got)
	}
}

func TestWebLinks_ReplacesNonCanonicalRunOutputURL(t *testing.T) {
	external := "https://example.com/reports/output"
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"run": map[string]any{
					"flowSlug":     "daily-sales-report",
					"workflowId":   "wf-123",
					"outputWebUrl": external,
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
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
	data, _ := out["data"].(map[string]any)
	run, _ := data["run"].(map[string]any)
	if got, _ := run["outputWebUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("outputWebUrl should use the portable run inspector, got: %q", got)
	}
}

func TestWebLinks_ResourcesListBuildsPortableRunURLForFlowOutputItemWithoutWebURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources" {
			http.NotFound(w, r)
			return
		}
		// A flow-output resource item carrying structured flow/run identifiers but
		// no server-provided webUrl (as the compacting path leaves it).
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{
					"uri":        "res://v1/ws/ws-acme/result/run/wf-123/flow-output",
					"type":       "result",
					"flowSlug":   "daily-sales-report",
					"workflowId": "wf-123",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
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
	data, _ := out["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("unexpected items length: %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if got, _ := first["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected item webUrl: %q", got)
	}
}

func TestWebLinks_RunsListUsesCanonicalFilteredRunsURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"items": []any{
					map[string]any{
						"flowSlug":   "daily-sales-report",
						"workflowId": "wf-123",
					},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "list",
		"--query", "flow:daily-sales-report version:7",
	)
	if err != nil {
		t.Fatalf("runs list failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}

func TestWebLinks_ConnectionRESTAddsWebURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=connections&connection=conn-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=connections&connection=conn-123" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}

func TestWebLinks_ResourcesGetAbsolutizesRelativeWebURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}

func TestWebLinks_ResourcesReadTableUsesPortableTablesPage(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/content" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource-uri": "res://v1/ws/ws-acme/result/table/tbl-1",
			"table-name":   "orders",
			"rows":         []any{},
			"webUrl":       "/ws-acme/resources/table/tbl-1",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "read", "res://v1/ws/ws-acme/result/table/tbl-1", "--full",
	)
	if err != nil {
		t.Fatalf("resources read failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	want := srv.URL + "/ui?workspace=ws-acme&page=tables"
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != want {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != want {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}

func TestWebLinks_ResourcesListAbsolutizesItemWebURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					"adapter": map[string]any{
						"details": map[string]any{
							"path": "workspaces/ws-acme/runs/wf-123/demo-result.json",
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
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
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("unexpected items length: %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if got, _ := first["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected item webUrl: %q", got)
	}
	if got, _ := first["displayName"].(string); got != "demo-result.json" {
		t.Fatalf("unexpected item displayName: %q", got)
	}
	if got, _ := first["sourceLabel"].(string); got != "run wf-123" {
		t.Fatalf("unexpected item sourceLabel: %q", got)
	}
}

func TestWebLinks_ResourcesSearchAbsolutizesAndEnrichesItems(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/search" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "transcript",
			"items": []any{
				map[string]any{
					"uri":    "res://v1/ws/ws-acme/result/run/wf-123/flow-output",
					"type":   "result",
					"webUrl": "/ws-acme/runs/daily-sales-report/wf-123/output",
					"adapter": map[string]any{
						"details": map[string]any{
							"path": "workspaces/ws-acme/runs/wf-123/transcript-jan-02.txt",
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "search", "transcript",
	)
	if err != nil {
		t.Fatalf("resources search failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("unexpected items length: %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if got, _ := first["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected item webUrl: %q", got)
	}
	if got, _ := first["displayName"].(string); got != "transcript-jan-02.txt" {
		t.Fatalf("unexpected item displayName: %q", got)
	}
	if got, _ := first["sourceLabel"].(string); got != "run wf-123" {
		t.Fatalf("unexpected item sourceLabel: %q", got)
	}
}

func TestWebLinks_ResourcesGetInfersCanonicalRunStepURL(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if got, _ := meta["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected meta.webUrl: %q", got)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}

func TestWebLinks_ResourcesGetPreservesPlusInStepID(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resources/by-uri" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uri":      "res://v1/ws/ws-acme/result/run/wf-123/step/fetch+sales/output",
			"type":     "result",
			"flowSlug": "daily-sales-report",
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"resources", "get", "res://v1/ws/ws-acme/result/run/wf-123/step/fetch+sales/output",
	)
	if err != nil {
		t.Fatalf("resources get failed: %v\n%s", err, stdout)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if got, _ := data["webUrl"].(string); got != srv.URL+"/ui?workspace=ws-acme&page=runs&run=wf-123" {
		t.Fatalf("unexpected data.webUrl: %q", got)
	}
}

func TestWebLinks_DoesNotRewriteUnknownPayloadWebURLFields(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"payload": map[string]any{
					"webUrl": "/product/123",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
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
	data, _ := out["data"].(map[string]any)
	payload, _ := data["payload"].(map[string]any)
	if got, _ := payload["webUrl"].(string); got != "/product/123" {
		t.Fatalf("unexpected payload.webUrl rewrite: %q", got)
	}
}
