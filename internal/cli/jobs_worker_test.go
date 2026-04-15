package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestJobsWorkerHandlerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_JOBS_WORKER_HELPER") != "1" {
		return
	}

	mode := os.Getenv("BREYTA_JOB_HELPER_MODE")
	payloadBytes, err := os.ReadFile(os.Getenv("BREYTA_JOB_PAYLOAD_FILE"))
	if err != nil {
		os.Exit(91)
	}
	resultPath := os.Getenv("BREYTA_JOB_RESULT_FILE")

	var result map[string]any
	switch mode {
	case "payload-any":
		var payload any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			os.Exit(92)
		}
		result = map[string]any{
			"status":  "succeeded",
			"summary": "payload echoed",
			"outputs": map[string]any{"payload": payload},
		}
	case "failure":
		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			os.Exit(92)
		}
		result = map[string]any{
			"message": "synthetic failure",
			"code":    "synthetic_failed",
			"details": map[string]any{"surface": payload["surface"]},
			"artifacts": []any{
				map[string]any{"kind": "log", "label": "stderr"},
			},
		}
	case "requested-failure":
		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			os.Exit(92)
		}
		result = map[string]any{
			"status":  "failed",
			"message": "synthetic requested failure",
			"code":    "synthetic_requested_failure",
			"details": map[string]any{"surface": payload["surface"]},
			"artifacts": []any{
				map[string]any{"kind": "report", "label": "summary"},
			},
		}
	default:
		var payload map[string]any
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			os.Exit(92)
		}
		result = map[string]any{
			"status":  "succeeded",
			"summary": "synthetic success",
			"outputs": map[string]any{"surface": payload["surface"]},
			"metrics": map[string]any{"checks": 1},
			"artifacts": []any{
				map[string]any{"kind": "report", "label": "summary"},
			},
			"workerInfo": map[string]any{"runner": "helper"},
		}
	}

	bytes, err := json.Marshal(result)
	if err != nil {
		os.Exit(93)
	}
	if err := os.WriteFile(resultPath, append(bytes, '\n'), 0644); err != nil {
		os.Exit(94)
	}

	if mode == "failure" {
		os.Exit(7)
	}
	os.Exit(0)
}

func TestJobsWorkerRun_ClaimsAndCompletesJob(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("GO_WANT_JOBS_WORKER_HELPER", "1")
	t.Setenv("BREYTA_JOB_HELPER_MODE", "success")

	helperPath, helperArgs := jobsWorkerHelperCommand(t)

	var mu sync.Mutex
	var commands []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)

		mu.Lock()
		commands = append(commands, command)
		mu.Unlock()

		switch command {
		case "jobs.claim":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":       "job-1",
						"jobType":     "demo.echo",
						"workspaceId": "ws-acme",
						"status":      "leased",
						"attempt":     1,
						"payload":     map[string]any{"surface": "flows-api"},
						"leaseToken":  "lease-1",
					},
				},
			})
		case "jobs.progress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-1",
						"jobType": "demo.echo",
						"status":  "started",
					},
				},
			})
		case "jobs.complete":
			if got, _ := args["summary"].(string); got != "synthetic success" {
				t.Fatalf("expected summary=synthetic success, got %#v", args["summary"])
			}
			outputs, _ := args["outputs"].(map[string]any)
			if got, _ := outputs["surface"].(string); got != "flows-api" {
				t.Fatalf("expected outputs.surface=flows-api, got %#v", outputs["surface"])
			}
			workerInfo, _ := args["workerInfo"].(map[string]any)
			if got, _ := workerInfo["runner"].(string); got != "helper" {
				t.Fatalf("expected workerInfo.runner=helper, got %#v", workerInfo["runner"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":         "job-1",
						"jobType":       "demo.echo",
						"status":        "succeeded",
						"resultSummary": "synthetic success",
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
		}
	}))
	defer srv.Close()

	args := []string{
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "worker", "run",
		"--type", "demo.echo",
		"--worker-id", "worker-1",
		"--once",
		"--heartbeat-interval", "0s",
		"--handler", helperPath,
	}
	for _, arg := range helperArgs {
		args = append(args, "--handler-arg", arg)
	}

	stdout, stderr, err := runCLIArgs(t, args...)
	if err != nil {
		t.Fatalf("jobs worker run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
	if got, _ := env.Data["handledCount"].(float64); got != 1 {
		t.Fatalf("expected handledCount=1, got %#v", env.Data["handledCount"])
	}
	lastJob, _ := env.Data["lastJob"].(map[string]any)
	if got, _ := lastJob["status"].(string); got != "succeeded" {
		t.Fatalf("expected lastJob.status=succeeded, got %#v", lastJob["status"])
	}

	mu.Lock()
	gotCommands := strings.Join(commands, ",")
	mu.Unlock()
	if gotCommands != "jobs.claim,jobs.progress,jobs.complete" {
		t.Fatalf("unexpected command sequence: %s", gotCommands)
	}
}

func TestJobsWorkerRun_PreservesNonObjectPayloadFile(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("GO_WANT_JOBS_WORKER_HELPER", "1")
	t.Setenv("BREYTA_JOB_HELPER_MODE", "payload-any")

	helperPath, helperArgs := jobsWorkerHelperCommand(t)

	var mu sync.Mutex
	var commands []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)

		mu.Lock()
		commands = append(commands, command)
		mu.Unlock()

		switch command {
		case "jobs.claim":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":       "job-2",
						"jobType":     "demo.echo",
						"workspaceId": "ws-acme",
						"status":      "leased",
						"attempt":     1,
						"payload":     []any{"flows-api", 2.0, true},
						"leaseToken":  "lease-2",
					},
				},
			})
		case "jobs.progress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-2",
						"jobType": "demo.echo",
						"status":  "started",
					},
				},
			})
		case "jobs.complete":
			outputs, _ := args["outputs"].(map[string]any)
			payload, _ := outputs["payload"].([]any)
			if len(payload) != 3 || payload[0] != "flows-api" || payload[1] != float64(2) || payload[2] != true {
				t.Fatalf("expected non-object payload to round-trip, got %#v", outputs["payload"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-2",
						"jobType": "demo.echo",
						"status":  "succeeded",
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
		}
	}))
	defer srv.Close()

	args := []string{
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "worker", "run",
		"--type", "demo.echo",
		"--worker-id", "worker-1",
		"--once",
		"--heartbeat-interval", "0s",
		"--handler", helperPath,
	}
	for _, arg := range helperArgs {
		args = append(args, "--handler-arg", arg)
	}

	stdout, stderr, err := runCLIArgs(t, args...)
	if err != nil {
		t.Fatalf("jobs worker run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}

	mu.Lock()
	gotCommands := strings.Join(commands, ",")
	mu.Unlock()
	if gotCommands != "jobs.claim,jobs.progress,jobs.complete" {
		t.Fatalf("unexpected command sequence: %s", gotCommands)
	}
}

func TestJobsWorkerRun_FailsJobWhenHandlerFails(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("GO_WANT_JOBS_WORKER_HELPER", "1")
	t.Setenv("BREYTA_JOB_HELPER_MODE", "failure")

	helperPath, helperArgs := jobsWorkerHelperCommand(t)

	var mu sync.Mutex
	var commands []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)

		mu.Lock()
		commands = append(commands, command)
		mu.Unlock()

		switch command {
		case "jobs.claim":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":       "job-1",
						"jobType":     "demo.echo",
						"workspaceId": "ws-acme",
						"status":      "leased",
						"attempt":     1,
						"payload":     map[string]any{"surface": "runtime"},
						"leaseToken":  "lease-1",
					},
				},
			})
		case "jobs.progress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-1",
						"jobType": "demo.echo",
						"status":  "started",
					},
				},
			})
		case "jobs.fail":
			if got, _ := args["message"].(string); got != "synthetic failure" {
				t.Fatalf("expected message=synthetic failure, got %#v", args["message"])
			}
			if got, _ := args["code"].(string); got != "synthetic_failed" {
				t.Fatalf("expected code=synthetic_failed, got %#v", args["code"])
			}
			details, _ := args["details"].(map[string]any)
			if got, _ := details["surface"].(string); got != "runtime" {
				t.Fatalf("expected details.surface=runtime, got %#v", details["surface"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-1",
						"jobType": "demo.echo",
						"status":  "failed",
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
		}
	}))
	defer srv.Close()

	args := []string{
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "worker", "run",
		"--type", "demo.echo",
		"--worker-id", "worker-1",
		"--once",
		"--heartbeat-interval", "0s",
		"--handler", helperPath,
	}
	for _, arg := range helperArgs {
		args = append(args, "--handler-arg", arg)
	}

	stdout, stderr, err := runCLIArgs(t, args...)
	if err != nil {
		t.Fatalf("jobs worker run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
	if got, _ := env.Data["handledCount"].(float64); got != 1 {
		t.Fatalf("expected handledCount=1, got %#v", env.Data["handledCount"])
	}
	if got, _ := env.Data["failedCount"].(float64); got != 1 {
		t.Fatalf("expected failedCount=1, got %#v", env.Data["failedCount"])
	}
	lastJob, _ := env.Data["lastJob"].(map[string]any)
	if got, _ := lastJob["status"].(string); got != "failed" {
		t.Fatalf("expected lastJob.status=failed, got %#v", lastJob["status"])
	}

	mu.Lock()
	gotCommands := strings.Join(commands, ",")
	mu.Unlock()
	if gotCommands != "jobs.claim,jobs.progress,jobs.fail" {
		t.Fatalf("unexpected command sequence: %s", gotCommands)
	}
}

func TestJobsWorkerRun_FailsJobWhenHandlerRequestsFailure(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("GO_WANT_JOBS_WORKER_HELPER", "1")
	t.Setenv("BREYTA_JOB_HELPER_MODE", "requested-failure")

	helperPath, helperArgs := jobsWorkerHelperCommand(t)

	var mu sync.Mutex
	var commands []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)

		mu.Lock()
		commands = append(commands, command)
		mu.Unlock()

		switch command {
		case "jobs.claim":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":       "job-1",
						"jobType":     "demo.echo",
						"workspaceId": "ws-acme",
						"status":      "leased",
						"attempt":     1,
						"payload":     map[string]any{"surface": "runtime"},
						"leaseToken":  "lease-1",
					},
				},
			})
		case "jobs.progress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-1",
						"jobType": "demo.echo",
						"status":  "started",
					},
				},
			})
		case "jobs.fail":
			if got, _ := args["message"].(string); got != "synthetic requested failure" {
				t.Fatalf("expected message=synthetic requested failure, got %#v", args["message"])
			}
			if got, _ := args["code"].(string); got != "synthetic_requested_failure" {
				t.Fatalf("expected code=synthetic_requested_failure, got %#v", args["code"])
			}
			details, _ := args["details"].(map[string]any)
			if got, _ := details["surface"].(string); got != "runtime" {
				t.Fatalf("expected details.surface=runtime, got %#v", details["surface"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-1",
						"jobType": "demo.echo",
						"status":  "failed",
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
		}
	}))
	defer srv.Close()

	args := []string{
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "worker", "run",
		"--type", "demo.echo",
		"--worker-id", "worker-1",
		"--once",
		"--heartbeat-interval", "0s",
		"--handler", helperPath,
	}
	for _, arg := range helperArgs {
		args = append(args, "--handler-arg", arg)
	}

	stdout, stderr, err := runCLIArgs(t, args...)
	if err != nil {
		t.Fatalf("jobs worker run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
	if got, _ := env.Data["handledCount"].(float64); got != 1 {
		t.Fatalf("expected handledCount=1, got %#v", env.Data["handledCount"])
	}
	if got, _ := env.Data["failedCount"].(float64); got != 1 {
		t.Fatalf("expected failedCount=1, got %#v", env.Data["failedCount"])
	}
	lastJob, _ := env.Data["lastJob"].(map[string]any)
	if got, _ := lastJob["status"].(string); got != "failed" {
		t.Fatalf("expected lastJob.status=failed, got %#v", lastJob["status"])
	}

	mu.Lock()
	gotCommands := strings.Join(commands, ",")
	mu.Unlock()
	if gotCommands != "jobs.claim,jobs.progress,jobs.fail" {
		t.Fatalf("unexpected command sequence: %s", gotCommands)
	}
}

func TestJobsWorkerRun_InjectsCLIPathIntoHandlerEnv(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var mu sync.Mutex
	var commands []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)

		mu.Lock()
		commands = append(commands, command)
		mu.Unlock()

		switch command {
		case "jobs.claim":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":       "job-cli-env",
						"jobType":     "demo.echo",
						"workspaceId": "ws-acme",
						"status":      "leased",
						"attempt":     1,
						"payload":     map[string]any{"surface": "flows-api"},
						"leaseToken":  "lease-cli-env",
					},
				},
			})
		case "jobs.progress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-cli-env",
						"jobType": "demo.echo",
						"status":  "started",
					},
				},
			})
		case "jobs.complete":
			outputs, _ := args["outputs"].(map[string]any)
			cliBin, _ := outputs["cliBin"].(string)
			if strings.TrimSpace(cliBin) == "" {
				t.Fatalf("expected outputs.cliBin to be set, got %#v", outputs["cliBin"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"job": map[string]any{
						"jobId":   "job-cli-env",
						"jobType": "demo.echo",
						"status":  "succeeded",
					},
				},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": "unexpected command",
				},
			})
		}
	}))
	defer srv.Close()

	handlerScript := strings.Join([]string{
		"import json",
		"import os",
		"import pathlib",
		"cli_bin = os.environ['BREYTA_CLI_BIN']",
		"path_parts = os.environ.get('PATH', '').split(os.pathsep)",
		"assert cli_bin.strip(), 'BREYTA_CLI_BIN missing'",
		"assert str(pathlib.Path(cli_bin).parent) in path_parts, 'cli dir missing from PATH'",
		"with open(os.environ['BREYTA_JOB_RESULT_FILE'], 'w', encoding='utf-8') as handle:",
		"    json.dump({'status': 'succeeded', 'summary': 'cli env ok', 'outputs': {'cliBin': cli_bin}}, handle)",
	}, "\n")

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"jobs", "worker", "run",
		"--type", "demo.echo",
		"--worker-id", "worker-1",
		"--once",
		"--heartbeat-interval", "0s",
		"--handler", "python3",
		"--handler-arg", "-c",
		"--handler-arg", handlerScript,
	)
	if err != nil {
		t.Fatalf("jobs worker run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}

	mu.Lock()
	gotCommands := strings.Join(commands, ",")
	mu.Unlock()
	if gotCommands != "jobs.claim,jobs.progress,jobs.complete" {
		t.Fatalf("unexpected command sequence: %s", gotCommands)
	}
}

func TestJobsWorkerFinish_WritesResultFileFromFlags(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	resultFile := filepath.Join(t.TempDir(), "result.json")
	t.Setenv("BREYTA_JOB_ID", "job-1")
	t.Setenv("BREYTA_JOB_LEASE_TOKEN", "lease-1")
	t.Setenv("BREYTA_JOB_RESULT_FILE", resultFile)

	stdout, stderr, err := runCLIArgs(t,
		"jobs", "worker", "finish",
		"--status", "no_changes",
		"--summary", "No actionable issues found",
		"--output", "surface=flows-api",
		"--output", "findingCount=0",
		"--metric", "filesScanned=4",
	)
	if err != nil {
		t.Fatalf("jobs worker finish failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	var state map[string]any
	bytes, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("read result file: %v", err)
	}
	if err := json.Unmarshal(bytes, &state); err != nil {
		t.Fatalf("decode result file: %v", err)
	}
	if got, _ := state["status"].(string); got != "no_changes" {
		t.Fatalf("expected status=no_changes, got %#v", state["status"])
	}
	if got, _ := state["summary"].(string); got != "No actionable issues found" {
		t.Fatalf("expected summary, got %#v", state["summary"])
	}
	outputs, _ := state["outputs"].(map[string]any)
	if got, _ := outputs["surface"].(string); got != "flows-api" {
		t.Fatalf("expected outputs.surface=flows-api, got %#v", outputs["surface"])
	}
	if got, _ := outputs["findingCount"].(float64); got != 0 {
		t.Fatalf("expected outputs.findingCount=0, got %#v", outputs["findingCount"])
	}
	metrics, _ := state["metrics"].(map[string]any)
	if got, _ := metrics["filesScanned"].(float64); got != 4 {
		t.Fatalf("expected metrics.filesScanned=4, got %#v", metrics["filesScanned"])
	}
}

func TestJobsWorkerFail_WritesFailedResultFileFromFlags(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	resultFile := filepath.Join(t.TempDir(), "result.json")
	t.Setenv("BREYTA_JOB_ID", "job-1")
	t.Setenv("BREYTA_JOB_RESULT_FILE", resultFile)

	stdout, stderr, err := runCLIArgs(t,
		"jobs", "worker", "fail",
		"--message", "Auth guard missing",
		"--code", "auth_guard_missing",
		"--detail", "surface=flows-api",
	)
	if err != nil {
		t.Fatalf("jobs worker fail failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	var state map[string]any
	bytes, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("read result file: %v", err)
	}
	if err := json.Unmarshal(bytes, &state); err != nil {
		t.Fatalf("decode result file: %v", err)
	}
	if got, _ := state["status"].(string); got != "failed" {
		t.Fatalf("expected status=failed, got %#v", state["status"])
	}
	if got, _ := state["message"].(string); got != "Auth guard missing" {
		t.Fatalf("expected message, got %#v", state["message"])
	}
	if got, _ := state["code"].(string); got != "auth_guard_missing" {
		t.Fatalf("expected code, got %#v", state["code"])
	}
	details, _ := state["details"].(map[string]any)
	if got, _ := details["surface"].(string); got != "flows-api" {
		t.Fatalf("expected details.surface=flows-api, got %#v", details["surface"])
	}
}

func TestJobsWorkerState_UsesJobDirAndRedactsLeaseToken(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("BREYTA_JOB_DIR", "")
	t.Setenv("BREYTA_JOB_FILE", "")
	t.Setenv("BREYTA_JOB_PAYLOAD_FILE", "")
	t.Setenv("BREYTA_JOB_RESULT_FILE", "")
	t.Setenv("BREYTA_JOB_ID", "")
	t.Setenv("BREYTA_JOB_TYPE", "")
	t.Setenv("BREYTA_JOB_BATCH_ID", "")
	t.Setenv("BREYTA_JOB_ATTEMPT", "")
	t.Setenv("BREYTA_JOB_WORKSPACE_ID", "")
	t.Setenv("BREYTA_JOB_LEASE_TOKEN", "")
	t.Setenv("BREYTA_TOKEN", "")

	jobDir := t.TempDir()
	jobFile := filepath.Join(jobDir, "job.json")
	payloadFile := filepath.Join(jobDir, "payload.json")
	resultFile := filepath.Join(jobDir, "result.json")

	writeJSONFile(t, jobFile, map[string]any{
		"jobId":       "job-42",
		"jobType":     "agent-review",
		"batchId":     "batch-1",
		"attempt":     2,
		"workspaceId": "ws-acme",
		"leaseToken":  "lease-secret",
	})
	writeJSONFile(t, payloadFile, map[string]any{
		"surface": "flows-api",
	})
	writeJSONFile(t, resultFile, map[string]any{
		"status": "running",
		"outputs": map[string]any{
			"findingCount": 1,
		},
	})

	stdout, stderr, err := runCLIArgs(t,
		"jobs", "worker", "state",
		"--job-dir", jobDir,
	)
	if err != nil {
		t.Fatalf("jobs worker state failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
	if got, _ := env.Data["jobId"].(string); got != "job-42" {
		t.Fatalf("expected jobId=job-42, got %#v", env.Data["jobId"])
	}
	if got, _ := env.Data["jobType"].(string); got != "agent-review" {
		t.Fatalf("expected jobType=agent-review, got %#v", env.Data["jobType"])
	}
	if got, _ := env.Data["jobDir"].(string); got != jobDir {
		t.Fatalf("expected jobDir=%s, got %#v", jobDir, env.Data["jobDir"])
	}
	if got, _ := env.Data["leaseTokenPresent"].(bool); got {
		t.Fatalf("expected leaseTokenPresent=false without env lease token")
	}

	job, _ := env.Data["job"].(map[string]any)
	if got, _ := job["leaseToken"].(string); got != "[redacted]" {
		t.Fatalf("expected redacted leaseToken, got %#v", job["leaseToken"])
	}
	result, _ := env.Data["result"].(map[string]any)
	if got, _ := result["status"].(string); got != "running" {
		t.Fatalf("expected result.status=running, got %#v", result["status"])
	}

	files, _ := env.Data["files"].(map[string]any)
	resultSnapshot, _ := files["result"].(map[string]any)
	if got, _ := resultSnapshot["path"].(string); got != resultFile {
		t.Fatalf("expected result snapshot path=%s, got %#v", resultFile, resultSnapshot["path"])
	}
}

func TestJobsWorkerState_UsesEnvAndReportsInvalidResultJSON(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	jobDir := t.TempDir()
	jobFile := filepath.Join(jobDir, "job.json")
	payloadFile := filepath.Join(jobDir, "payload.json")
	resultFile := filepath.Join(jobDir, "result.json")

	writeJSONFile(t, jobFile, map[string]any{
		"jobId":       "job-7",
		"jobType":     "demo.echo",
		"workspaceId": "ws-acme",
		"leaseToken":  "lease-7",
	})
	writeJSONFile(t, payloadFile, map[string]any{
		"surface": "runtime",
	})
	if err := os.WriteFile(resultFile, []byte("{\"status\":"), 0644); err != nil {
		t.Fatalf("write invalid result file: %v", err)
	}

	t.Setenv("BREYTA_WORKER_ID", "worker-7")
	t.Setenv("BREYTA_JOB_DIR", jobDir)
	t.Setenv("BREYTA_JOB_FILE", jobFile)
	t.Setenv("BREYTA_JOB_PAYLOAD_FILE", payloadFile)
	t.Setenv("BREYTA_JOB_RESULT_FILE", resultFile)
	t.Setenv("BREYTA_JOB_ID", "job-7")
	t.Setenv("BREYTA_JOB_TYPE", "demo.echo")
	t.Setenv("BREYTA_JOB_ATTEMPT", "3")
	t.Setenv("BREYTA_JOB_WORKSPACE_ID", "ws-acme")
	t.Setenv("BREYTA_JOB_LEASE_TOKEN", "lease-7")
	t.Setenv("BREYTA_API_URL", "http://127.0.0.1:9999")
	t.Setenv("BREYTA_WORKSPACE", "ws-acme")
	t.Setenv("BREYTA_TOKEN", "user-dev")

	stdout, stderr, err := runCLIArgs(t, "jobs", "worker", "state")
	if err != nil {
		t.Fatalf("jobs worker state failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok=true, got %+v", env)
	}
	if got, _ := env.Data["workerId"].(string); got != "worker-7" {
		t.Fatalf("expected workerId=worker-7, got %#v", env.Data["workerId"])
	}
	if got, _ := env.Data["attempt"].(float64); got != 3 {
		t.Fatalf("expected attempt=3, got %#v", env.Data["attempt"])
	}
	if got, _ := env.Data["leaseTokenPresent"].(bool); !got {
		t.Fatalf("expected leaseTokenPresent=true")
	}
	if got, _ := env.Data["tokenPresent"].(bool); !got {
		t.Fatalf("expected tokenPresent=true")
	}

	files, _ := env.Data["files"].(map[string]any)
	resultSnapshot, _ := files["result"].(map[string]any)
	if _, ok := resultSnapshot["json"]; ok {
		t.Fatalf("expected invalid result snapshot to omit parsed json")
	}
	if got, _ := resultSnapshot["raw"].(string); got != "{\"status\":" {
		t.Fatalf("expected raw invalid result json, got %#v", resultSnapshot["raw"])
	}
	if got, _ := resultSnapshot["jsonError"].(string); !strings.Contains(got, "unexpected end of JSON input") {
		t.Fatalf("expected json parse error, got %#v", resultSnapshot["jsonError"])
	}
}

func TestJobsWorkerProgress_UsesEnvContext(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("BREYTA_JOB_ID", "job-1")
	t.Setenv("BREYTA_JOB_LEASE_TOKEN", "lease-1")

	var gotArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "jobs.progress" {
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
					"jobId":  "job-1",
					"status": "running",
				},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("BREYTA_API_URL", srv.URL)
	t.Setenv("BREYTA_WORKSPACE", "ws-acme")
	t.Setenv("BREYTA_TOKEN", "user-dev")

	stdout, stderr, err := runCLIArgs(t,
		"jobs", "worker", "progress",
		"--status", "running",
		"--message", "Reviewing flows-api",
		"--detail", "surface=flows-api",
		"--metric", "filesScanned=4",
	)
	if err != nil {
		t.Fatalf("jobs worker progress failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["jobId"].(string); got != "job-1" {
		t.Fatalf("expected jobId from env, got %#v", gotArgs["jobId"])
	}
	if got, _ := gotArgs["leaseToken"].(string); got != "lease-1" {
		t.Fatalf("expected leaseToken from env, got %#v", gotArgs["leaseToken"])
	}
	if got, _ := gotArgs["status"].(string); got != "running" {
		t.Fatalf("expected status=running, got %#v", gotArgs["status"])
	}
	details, _ := gotArgs["details"].(map[string]any)
	if got, _ := details["surface"].(string); got != "flows-api" {
		t.Fatalf("expected details.surface=flows-api, got %#v", details["surface"])
	}
	metrics, _ := gotArgs["metrics"].(map[string]any)
	if got, _ := metrics["filesScanned"].(float64); got != 4 {
		t.Fatalf("expected metrics.filesScanned=4, got %#v", metrics["filesScanned"])
	}
}

func TestJobsWorkerAttachFile_UploadsResourceAndAppendsArtifact(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	tmpDir := t.TempDir()
	resultFile := filepath.Join(tmpDir, "result.json")
	reportPath := filepath.Join(tmpDir, "review-report.md")
	reportBody := "# Review\n\nOne issue found.\n"
	if err := os.WriteFile(reportPath, []byte(reportBody), 0644); err != nil {
		t.Fatalf("write report file: %v", err)
	}

	t.Setenv("BREYTA_JOB_ID", "job-1")
	t.Setenv("BREYTA_JOB_LEASE_TOKEN", "lease-1")
	t.Setenv("BREYTA_JOB_RESULT_FILE", resultFile)

	var uploadedBody string
	var initCalled bool
	var completeCalled bool
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/files/uploads/init":
			initCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uri":        "res://v1/ws/ws-acme/file/file-1",
				"upload-url": srv.URL + "/upload/file-1",
			})
		case "/upload/file-1":
			body, _ := io.ReadAll(r.Body)
			uploadedBody = string(body)
			if got := r.Header.Get("Content-Type"); got != "text/markdown; charset=utf-8" && got != "text/markdown" {
				t.Fatalf("expected markdown content type, got %q", got)
			}
			w.WriteHeader(http.StatusOK)
		case "/api/files/uploads/complete":
			completeCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uri":          "res://v1/ws/ws-acme/file/file-1",
				"content-type": "text/markdown",
				"size-bytes":   len(reportBody),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("BREYTA_API_URL", srv.URL)
	t.Setenv("BREYTA_WORKSPACE", "ws-acme")
	t.Setenv("BREYTA_TOKEN", "user-dev")

	stdout, stderr, err := runCLIArgs(t,
		"jobs", "worker", "attach-file",
		"--file", reportPath,
		"--label", "review-report",
		"--kind", "report",
		"--print-uri",
	)
	if err != nil {
		t.Fatalf("jobs worker attach-file failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if !initCalled || !completeCalled {
		t.Fatalf("expected init+complete upload flow, got init=%v complete=%v", initCalled, completeCalled)
	}
	if uploadedBody != reportBody {
		t.Fatalf("expected uploaded body %q, got %q", reportBody, uploadedBody)
	}
	if got := strings.TrimSpace(stdout); got != "res://v1/ws/ws-acme/file/file-1" {
		t.Fatalf("expected printed resource uri, got %q", got)
	}

	var state map[string]any
	bytes, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("read result file: %v", err)
	}
	if err := json.Unmarshal(bytes, &state); err != nil {
		t.Fatalf("decode result file: %v", err)
	}
	artifacts, _ := state["artifacts"].([]any)
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact, got %d", len(artifacts))
	}
	artifact, _ := artifacts[0].(map[string]any)
	if got, _ := artifact["label"].(string); got != "review-report" {
		t.Fatalf("expected label=review-report, got %#v", artifact["label"])
	}
	if got, _ := artifact["kind"].(string); got != "report" {
		t.Fatalf("expected kind=report, got %#v", artifact["kind"])
	}
	if got, _ := artifact["resourceUri"].(string); got != "res://v1/ws/ws-acme/file/file-1" {
		t.Fatalf("expected resource uri on artifact, got %#v", artifact["resourceUri"])
	}
}

func TestJobsWorkerAttachFile_FallsBackToAPIDirectUploadWhenSignedURLUnavailable(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	tmpDir := t.TempDir()
	resultFile := filepath.Join(tmpDir, "result.json")
	reportPath := filepath.Join(tmpDir, "review-report.md")
	reportBody := "# Review\n\nOne issue found.\n"
	if err := os.WriteFile(reportPath, []byte(reportBody), 0644); err != nil {
		t.Fatalf("write report file: %v", err)
	}

	t.Setenv("BREYTA_JOB_ID", "job-1")
	t.Setenv("BREYTA_JOB_LEASE_TOKEN", "lease-1")
	t.Setenv("BREYTA_JOB_RESULT_FILE", resultFile)

	var directUploadBody string
	var directUploadQuery string
	var directUploadAuth string
	var directUploadWorkspace string
	var initCalled bool
	var completeCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/files/uploads/init":
			initCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uri":        "res://v1/ws/ws-acme/file/file-1",
				"upload-url": "",
			})
		case "/api/files/uploads/direct":
			directUploadQuery = r.URL.Query().Get("uri")
			directUploadAuth = r.Header.Get("Authorization")
			directUploadWorkspace = r.Header.Get("X-Breyta-Workspace")
			body, _ := io.ReadAll(r.Body)
			directUploadBody = string(body)
			if got := r.Header.Get("Content-Type"); got != "text/markdown; charset=utf-8" && got != "text/markdown" {
				t.Fatalf("expected markdown content type, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uploaded":   true,
				"uri":        "res://v1/ws/ws-acme/file/file-1",
				"size-bytes": len(reportBody),
			})
		case "/api/files/uploads/complete":
			completeCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uri":          "res://v1/ws/ws-acme/file/file-1",
				"content-type": "text/markdown",
				"size-bytes":   len(reportBody),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("BREYTA_API_URL", srv.URL)
	t.Setenv("BREYTA_WORKSPACE", "ws-acme")
	t.Setenv("BREYTA_TOKEN", "user-dev")

	stdout, stderr, err := runCLIArgs(t,
		"jobs", "worker", "attach-file",
		"--file", reportPath,
		"--label", "review-report",
		"--kind", "report",
		"--print-uri",
	)
	if err != nil {
		t.Fatalf("jobs worker attach-file failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if !initCalled || !completeCalled {
		t.Fatalf("expected init+complete upload flow, got init=%v complete=%v", initCalled, completeCalled)
	}
	if directUploadBody != reportBody {
		t.Fatalf("expected direct upload body %q, got %q", reportBody, directUploadBody)
	}
	if directUploadQuery != "res://v1/ws/ws-acme/file/file-1" {
		t.Fatalf("expected direct upload uri query, got %q", directUploadQuery)
	}
	if directUploadAuth != "Bearer user-dev" {
		t.Fatalf("expected bearer auth on direct upload, got %q", directUploadAuth)
	}
	if directUploadWorkspace != "ws-acme" {
		t.Fatalf("expected workspace header on direct upload, got %q", directUploadWorkspace)
	}
	if got := strings.TrimSpace(stdout); got != "res://v1/ws/ws-acme/file/file-1" {
		t.Fatalf("expected printed resource uri, got %q", got)
	}
}

func TestJobsWorkerAttachKV_CallsJobsAttachKVAndAppendsArtifact(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	tmpDir := t.TempDir()
	resultFile := filepath.Join(tmpDir, "result.json")

	t.Setenv("BREYTA_JOB_ID", "job-1")
	t.Setenv("BREYTA_JOB_LEASE_TOKEN", "lease-1")
	t.Setenv("BREYTA_JOB_RESULT_FILE", resultFile)

	var gotArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "jobs.attach_kv" {
			w.WriteHeader(http.StatusBadRequest)
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
				"artifact": map[string]any{
					"label":       "review-summary",
					"kind":        "kv",
					"contentType": "application/json",
					"resourceUri": "res://v1/ws/ws-acme/result/kv/review-summary",
					"key":         "review:summary",
					"sizeBytes":   32,
				},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("BREYTA_API_URL", srv.URL)
	t.Setenv("BREYTA_WORKSPACE", "ws-acme")
	t.Setenv("BREYTA_TOKEN", "user-dev")

	stdout, stderr, err := runCLIArgs(t,
		"jobs", "worker", "attach-kv",
		"--label", "review-summary",
		"--key", "review:summary",
		"--field", "findingCount=1",
		"--field", "severity=high",
		"--print-uri",
	)
	if err != nil {
		t.Fatalf("jobs worker attach-kv failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["jobId"].(string); got != "job-1" {
		t.Fatalf("expected jobId from env, got %#v", gotArgs["jobId"])
	}
	if got, _ := gotArgs["leaseToken"].(string); got != "lease-1" {
		t.Fatalf("expected leaseToken from env, got %#v", gotArgs["leaseToken"])
	}
	if got, _ := gotArgs["label"].(string); got != "review-summary" {
		t.Fatalf("expected label=review-summary, got %#v", gotArgs["label"])
	}
	if got, _ := gotArgs["key"].(string); got != "review:summary" {
		t.Fatalf("expected key=review:summary, got %#v", gotArgs["key"])
	}
	value, _ := gotArgs["value"].(map[string]any)
	if got, _ := value["findingCount"].(float64); got != 1 {
		t.Fatalf("expected value.findingCount=1, got %#v", value["findingCount"])
	}
	if got, _ := value["severity"].(string); got != "high" {
		t.Fatalf("expected value.severity=high, got %#v", value["severity"])
	}
	if got := strings.TrimSpace(stdout); got != "res://v1/ws/ws-acme/result/kv/review-summary" {
		t.Fatalf("expected printed resource uri, got %q", got)
	}

	var state map[string]any
	bytes, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("read result file: %v", err)
	}
	if err := json.Unmarshal(bytes, &state); err != nil {
		t.Fatalf("decode result file: %v", err)
	}
	artifacts, _ := state["artifacts"].([]any)
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact, got %d", len(artifacts))
	}
	artifact, _ := artifacts[0].(map[string]any)
	if got, _ := artifact["resourceUri"].(string); got != "res://v1/ws/ws-acme/result/kv/review-summary" {
		t.Fatalf("expected resource uri on artifact, got %#v", artifact["resourceUri"])
	}
	if got, _ := artifact["key"].(string); got != "review:summary" {
		t.Fatalf("expected key on artifact, got %#v", artifact["key"])
	}
}

func TestJobsWorkerAttachTable_CallsJobsAttachTableAndAppendsArtifact(t *testing.T) {
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	tmpDir := t.TempDir()
	resultFile := filepath.Join(tmpDir, "result.json")

	t.Setenv("BREYTA_JOB_ID", "job-1")
	t.Setenv("BREYTA_JOB_LEASE_TOKEN", "lease-1")
	t.Setenv("BREYTA_JOB_RESULT_FILE", resultFile)

	var gotArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "jobs.attach_table" {
			w.WriteHeader(http.StatusBadRequest)
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
				"artifact": map[string]any{
					"label":       "findings",
					"kind":        "table",
					"contentType": "application/vnd.breyta.table+json",
					"resourceUri": "res://v1/ws/ws-acme/result/table/findings",
					"tableId":     "tbl_123",
					"tableName":   "security-findings",
					"rowsWritten": 2,
					"rowCount":    2,
					"keyFields":   []any{"finding_id"},
					"indexFields": []any{"severity"},
					"writeMode":   "upsert",
				},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("BREYTA_API_URL", srv.URL)
	t.Setenv("BREYTA_WORKSPACE", "ws-acme")
	t.Setenv("BREYTA_TOKEN", "user-dev")

	stdout, stderr, err := runCLIArgs(t,
		"jobs", "worker", "attach-table",
		"--label", "findings",
		"--table", "security-findings",
		"--rows", `[{"finding_id":"f-1","severity":"high"},{"finding_id":"f-2","severity":"medium"}]`,
		"--write-mode", "upsert",
		"--key-field", "finding_id",
		"--index-field", "severity",
		"--print-uri",
	)
	if err != nil {
		t.Fatalf("jobs worker attach-table failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if got, _ := gotArgs["jobId"].(string); got != "job-1" {
		t.Fatalf("expected jobId from env, got %#v", gotArgs["jobId"])
	}
	if got, _ := gotArgs["leaseToken"].(string); got != "lease-1" {
		t.Fatalf("expected leaseToken from env, got %#v", gotArgs["leaseToken"])
	}
	if got, _ := gotArgs["table"].(string); got != "security-findings" {
		t.Fatalf("expected table=security-findings, got %#v", gotArgs["table"])
	}
	if got, _ := gotArgs["writeMode"].(string); got != "upsert" {
		t.Fatalf("expected writeMode=upsert, got %#v", gotArgs["writeMode"])
	}
	keyFields, _ := gotArgs["keyFields"].([]any)
	if len(keyFields) != 1 || keyFields[0] != "finding_id" {
		t.Fatalf("expected keyFields=[finding_id], got %#v", gotArgs["keyFields"])
	}
	indexFields, _ := gotArgs["indexFields"].([]any)
	if len(indexFields) != 1 || indexFields[0] != "severity" {
		t.Fatalf("expected indexFields=[severity], got %#v", gotArgs["indexFields"])
	}
	rows, _ := gotArgs["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %#v", gotArgs["rows"])
	}
	firstRow, _ := rows[0].(map[string]any)
	if got, _ := firstRow["finding_id"].(string); got != "f-1" {
		t.Fatalf("expected first row finding_id=f-1, got %#v", firstRow["finding_id"])
	}
	if got := strings.TrimSpace(stdout); got != "res://v1/ws/ws-acme/result/table/findings" {
		t.Fatalf("expected printed resource uri, got %q", got)
	}

	var state map[string]any
	bytes, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("read result file: %v", err)
	}
	if err := json.Unmarshal(bytes, &state); err != nil {
		t.Fatalf("decode result file: %v", err)
	}
	artifacts, _ := state["artifacts"].([]any)
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact, got %d", len(artifacts))
	}
	artifact, _ := artifacts[0].(map[string]any)
	if got, _ := artifact["tableName"].(string); got != "security-findings" {
		t.Fatalf("expected tableName on artifact, got %#v", artifact["tableName"])
	}
	if got, _ := artifact["writeMode"].(string); got != "upsert" {
		t.Fatalf("expected writeMode on artifact, got %#v", artifact["writeMode"])
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	bytes, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(bytes, '\n'), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func jobsWorkerHelperCommand(t *testing.T) (string, []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return exe, []string{"-test.run=TestJobsWorkerHandlerHelperProcess"}
}
