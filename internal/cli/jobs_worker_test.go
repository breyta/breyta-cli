package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestJobsWorkerHandlerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_JOBS_WORKER_HELPER") != "1" {
		return
	}

	payloadBytes, err := os.ReadFile(os.Getenv("BREYTA_JOB_PAYLOAD_FILE"))
	if err != nil {
		os.Exit(91)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		os.Exit(92)
	}

	resultPath := os.Getenv("BREYTA_JOB_RESULT_FILE")
	mode := os.Getenv("BREYTA_JOB_HELPER_MODE")

	var result map[string]any
	switch mode {
	case "failure":
		result = map[string]any{
			"message": "synthetic failure",
			"code":    "synthetic_failed",
			"details": map[string]any{"surface": payload["surface"]},
			"artifacts": []any{
				map[string]any{"kind": "log", "label": "stderr"},
			},
		}
	default:
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

func jobsWorkerHelperCommand(t *testing.T) (string, []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return exe, []string{"-test.run=TestJobsWorkerHandlerHelperProcess"}
}
