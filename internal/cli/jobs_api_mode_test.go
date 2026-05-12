package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestJobsCreate_UsesAPICommand(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "jobs.create" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
			return
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"job": map[string]any{
					"jobId":       "job-1",
					"jobType":     "codex-review",
					"status":      "queued",
					"payload":     map[string]any{"surface": "flows-api"},
					"metadata":    map[string]any{"focus": "auth"},
					"maxAttempts": 4,
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "create",
		"--type", "codex-review",
		"--fanout-parent-step-id", "supervisor-fanout",
		"--fanout-max-concurrency", "3",
		"--payload", `{"surface":"flows-api"}`,
		"--metadata", `{"focus":"auth"}`,
		"--max-attempts", "4",
	)
	if err != nil {
		t.Fatalf("jobs create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["jobType"].(string); got != "codex-review" {
		t.Fatalf("expected jobType=codex-review, got %#v", gotArgs["jobType"])
	}
	if got, _ := gotArgs["fanoutParentStepId"].(string); got != "supervisor-fanout" {
		t.Fatalf("expected fanoutParentStepId=supervisor-fanout, got %#v", gotArgs["fanoutParentStepId"])
	}
	if got, _ := gotArgs["fanoutMaxConcurrency"].(float64); got != 3 {
		t.Fatalf("expected fanoutMaxConcurrency=3, got %#v", gotArgs["fanoutMaxConcurrency"])
	}
	payload, _ := gotArgs["payload"].(map[string]any)
	if got, _ := payload["surface"].(string); got != "flows-api" {
		t.Fatalf("expected payload.surface=flows-api, got %#v", payload["surface"])
	}
	metadata, _ := gotArgs["metadata"].(map[string]any)
	if got, _ := metadata["focus"].(string); got != "auth" {
		t.Fatalf("expected metadata.focus=auth, got %#v", metadata["focus"])
	}
	if got, _ := gotArgs["maxAttempts"].(float64); got != 4 {
		t.Fatalf("expected maxAttempts=4, got %#v", gotArgs["maxAttempts"])
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
}

func TestJobsList_DefaultLimitIsCompact(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "jobs.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": map[string]any{"message": "unexpected command"},
			})
			return
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"items": []any{
					map[string]any{
						"jobId":         "job-1",
						"jobType":       "codex-review",
						"status":        "succeeded",
						"resultSummary": "short summary",
						"payload":       map[string]any{"prompt": "large prompt", "repo": "breyta"},
						"result": map[string]any{
							"outputs": map[string]any{"pr-url": "https://github.com/breyta/breyta/pull/1"},
						},
						"attempts": []any{map[string]any{"attempt": 1}},
					},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "list",
	)
	if err != nil {
		t.Fatalf("jobs list failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if got, _ := gotArgs["limit"].(float64); got != 10 {
		t.Fatalf("expected compact default limit=10, got %#v", gotArgs["limit"])
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
	if env.Meta["outputView"] != "compact" {
		t.Fatalf("expected compact output view, got %#v", env.Meta)
	}
	items, _ := env.Data["items"].([]any)
	first, _ := items[0].(map[string]any)
	if _, ok := first["payload"]; ok {
		t.Fatalf("default jobs list should omit raw payload, got %#v", first)
	}
	if _, ok := first["result"]; ok {
		t.Fatalf("default jobs list should omit raw result, got %#v", first)
	}
	if got, _ := first["prUrl"].(string); got != "https://github.com/breyta/breyta/pull/1" {
		t.Fatalf("expected compact prUrl, got %#v", first)
	}
	if keys, _ := first["payloadKeys"].([]any); len(keys) != 2 {
		t.Fatalf("expected payload key summary, got %#v", first["payloadKeys"])
	}
}

func TestJobsBatchCreate_UsesBatchCommand(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "jobs.batches.create" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
			return
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"batch": map[string]any{
					"batchId":        "batch-1",
					"jobType":        "codex-review",
					"requestedCount": 2,
					"status":         "queued",
				},
				"jobs": []any{
					map[string]any{"jobId": "job-1"},
					map[string]any{"jobId": "job-2"},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "batches", "create",
		"--type", "codex-review",
		"--fanout-parent-step-id", "security-supervisor",
		"--fanout-max-concurrency", "5",
		"--metadata", `{"campaign":"security"}`,
		"--job", `{"payload":{"surface":"flows-api"}}`,
		"--job", `{"payload":{"surface":"runtime"},"maxAttempts":2}`,
	)
	if err != nil {
		t.Fatalf("jobs batches create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["jobType"].(string); got != "codex-review" {
		t.Fatalf("expected jobType=codex-review, got %#v", gotArgs["jobType"])
	}
	if got, _ := gotArgs["fanoutParentStepId"].(string); got != "security-supervisor" {
		t.Fatalf("expected fanoutParentStepId=security-supervisor, got %#v", gotArgs["fanoutParentStepId"])
	}
	if got, _ := gotArgs["fanoutMaxConcurrency"].(float64); got != 5 {
		t.Fatalf("expected fanoutMaxConcurrency=5, got %#v", gotArgs["fanoutMaxConcurrency"])
	}
	metadata, _ := gotArgs["metadata"].(map[string]any)
	if got, _ := metadata["campaign"].(string); got != "security" {
		t.Fatalf("expected metadata.campaign=security, got %#v", metadata["campaign"])
	}
	jobs, _ := gotArgs["jobs"].([]any)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 batch jobs, got %d", len(jobs))
	}
	second, _ := jobs[1].(map[string]any)
	if got, _ := second["maxAttempts"].(float64); got != 2 {
		t.Fatalf("expected second job maxAttempts=2, got %#v", second["maxAttempts"])
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
}

func TestJobsBatchesShow_DefaultLimitIsCompact(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "jobs.batches.get" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": map[string]any{"message": "unexpected command"},
			})
			return
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"batch": map[string]any{"batchId": "batch-1"},
				"jobs": []any{
					map[string]any{
						"jobId":   "job-1",
						"jobType": "codex-review",
						"status":  "queued",
						"payload": map[string]any{"prompt": "large prompt"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "batches", "show", "batch-1",
	)
	if err != nil {
		t.Fatalf("jobs batches show failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if got, _ := gotArgs["batchId"].(string); got != "batch-1" {
		t.Fatalf("expected batchId=batch-1, got %#v", gotArgs["batchId"])
	}
	if got, _ := gotArgs["limit"].(float64); got != 10 {
		t.Fatalf("expected compact default limit=10, got %#v", gotArgs["limit"])
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
	if env.Meta["outputView"] != "compact" {
		t.Fatalf("expected compact output view, got %#v", env.Meta)
	}
	jobs, _ := env.Data["jobs"].([]any)
	first, _ := jobs[0].(map[string]any)
	if _, ok := first["payload"]; ok {
		t.Fatalf("default jobs batches show should omit raw payload, got %#v", first)
	}
}

func TestJobsClaim_UsesLeaseDurationAndLabels(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "jobs.claim" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
			return
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"job": map[string]any{
					"jobId":        "job-1",
					"jobType":      "codex-review",
					"status":       "leased",
					"leaseToken":   "lease-1",
					"workerId":     "worker-1",
					"workerLabels": map[string]any{"pool": "default"},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(
		t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "claim",
		"--type", "codex-review",
		"--worker-id", "worker-1",
		"--lease-duration", "15s",
		"--label", "pool=default",
	)
	if err != nil {
		t.Fatalf("jobs claim failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["jobType"].(string); got != "codex-review" {
		t.Fatalf("expected jobType=codex-review, got %#v", gotArgs["jobType"])
	}
	if got, _ := gotArgs["workerId"].(string); got != "worker-1" {
		t.Fatalf("expected workerId=worker-1, got %#v", gotArgs["workerId"])
	}
	if got, _ := gotArgs["leaseDuration"].(float64); got != 15000 {
		t.Fatalf("expected leaseDuration=15000, got %#v", gotArgs["leaseDuration"])
	}
	labels, _ := gotArgs["workerLabels"].(map[string]any)
	if got, _ := labels["pool"].(string); got != "default" {
		t.Fatalf("expected workerLabels.pool=default, got %#v", labels["pool"])
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
}
