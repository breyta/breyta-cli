package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStepsRunSendsFlowSourceAndVersion(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"_hints": []any{
				"breyta steps docs set my-flow make-output --markdown '...'",
				"breyta steps record --flow my-flow --type code --id make-output --params '{...}'",
				"breyta steps examples add my-flow make-output --input '{...}' --output '{...}'",
				"breyta steps tests add my-flow make-output --name '...' --input '{...}' --expected '{...}'",
			},
			"data": map[string]any{
				"stepType":   "code",
				"stepId":     "make-output",
				"durationMs": 1,
				"result":     map[string]any{"ok": true},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"steps", "run",
		"--flow", "my-flow",
		"--source", "draft",
		"--version", "3",
		"--type", "code",
		"--id", "make-output",
		"--idempotency-key", "turn-123:make-output",
		"--params", `{"input":{"n":2}}`,
	)
	if err != nil {
		t.Fatalf("steps run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "steps.run" {
		t.Fatalf("expected steps.run, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["version"] != float64(3) {
		t.Fatalf("expected flow source/version args, got %#v", args)
	}
	if args["idempotencyKey"] != "turn-123:make-output" {
		t.Fatalf("expected idempotency key, got %#v", args)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if _, exists := data["result"]; !exists {
		t.Fatalf("expected legacy steps run to preserve data.result by default, got %#v", data)
	}
	if _, exists := data["resultPreview"]; exists {
		t.Fatalf("expected legacy steps run to omit resultPreview by default, got %#v", data["resultPreview"])
	}
}

func TestStepsRunAllowsRegisteredFlowStepWithoutType(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"stepType":   "function",
				"stepId":     "tools/add-one",
				"durationMs": 1,
				"result":     map[string]any{"answer": 3},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"steps", "run",
		"--flow", "my-flow",
		"--source", "draft",
		"--id", "tools/add-one",
	)
	if err != nil {
		t.Fatalf("steps run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "steps.run" {
		t.Fatalf("expected steps.run, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "tools/add-one" {
		t.Fatalf("expected flow-scoped step args, got %#v", args)
	}
	if _, exists := args["stepType"]; exists {
		t.Fatalf("expected no stepType for registered flow-local step, got %#v", args["stepType"])
	}
}

func TestFlowsStepsRunSendsFlowScopedCommandAndCompactsResult(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"stepType":   "function",
				"stepId":     "tools/add-one",
				"durationMs": 1,
				"result": map[string]any{
					"rows": []any{
						map[string]any{"id": "row-1", "score": 0.92},
						map[string]any{"id": "row-2", "score": 0.84},
					},
					"summary": "ready",
				},
			},
		})
	}))
	defer srv.Close()

	resultFile := filepath.Join(t.TempDir(), "result.json")
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "steps", "run", "my-flow", "tools/add-one",
		"--params", `{"input":{"n":2}}`,
		"--result-path", "rows.0",
		"--result-file", resultFile,
	)
	if err != nil {
		t.Fatalf("flows steps run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.run" {
		t.Fatalf("expected flows.steps.run, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "tools/add-one" {
		t.Fatalf("expected flow-scoped step args, got %#v", args)
	}
	if _, exists := args["stepType"]; exists {
		t.Fatalf("expected no stepType for flow-scoped wrapper, got %#v", args["stepType"])
	}
	params, _ := args["params"].(map[string]any)
	input, _ := params["input"].(map[string]any)
	if input["n"] != float64(2) {
		t.Fatalf("expected params to be forwarded, got %#v", args["params"])
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if _, exists := data["result"]; exists {
		t.Fatalf("expected compact output to omit data.result, got %#v", data["result"])
	}
	preview, _ := data["resultPreview"].(map[string]any)
	if found, _ := preview["pathFound"].(bool); !found {
		t.Fatalf("expected result path to be found, got %#v", preview)
	}
	value, _ := preview["value"].(string)
	if !strings.Contains(value, `:id "row-1"`) || strings.Contains(value, "row-2") {
		t.Fatalf("expected focused first-row preview, got %q", value)
	}
	var fullResult map[string]any
	if err := readJSONFile(resultFile, &fullResult); err != nil {
		t.Fatalf("read result file: %v", err)
	}
	if _, ok := fullResult["rows"].([]any); !ok {
		t.Fatalf("expected full result file to contain rows, got %#v", fullResult)
	}
}

func TestStepsRunCompactsResultWhenExplicitlyRequested(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"stepType":   "code",
				"stepId":     "make-output",
				"durationMs": 1,
				"result": map[string]any{
					"rows": []any{
						map[string]any{"id": "row-1", "score": 0.92},
						map[string]any{"id": "row-2", "score": 0.84},
					},
					"summary": "ready",
				},
			},
			"meta": map[string]any{
				"hints": []any{
					"breyta steps docs set my-flow make-output --markdown '...'",
					"breyta steps record --flow my-flow --type code --id make-output --params '{...}'",
					"breyta steps examples add my-flow make-output --input '{...}' --output '{...}'",
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
		"steps", "run",
		"--type", "code",
		"--id", "make-output",
		"--params", `{"input":{"n":2}}`,
		"--full=false",
	)
	if err != nil {
		t.Fatalf("steps run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if _, exists := data["result"]; exists {
		t.Fatalf("expected compact output to omit data.result, got %#v", data["result"])
	}
	preview, _ := data["resultPreview"].(map[string]any)
	if preview["format"] != "clojure-value-preview" {
		t.Fatalf("expected clojure value preview, got %#v", preview)
	}
	value, _ := preview["value"].(string)
	if !strings.Contains(value, ":rows") || !strings.Contains(value, ":summary") {
		t.Fatalf("expected EDN-style keys in preview, got %q", value)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["outputView"] != "compact" {
		t.Fatalf("expected compact outputView, got %#v", meta)
	}
	if _, exists := meta["hints"]; exists {
		t.Fatalf("expected compact output to drop duplicate meta.hints, got %#v", meta["hints"])
	}
	hints, _ := out["_hints"].([]any)
	if len(hints) != 1 || !strings.Contains(hints[0].(string), "breyta steps record --flow my-flow --type code --id make-output") {
		t.Fatalf("expected one compact record hint, got %#v", out["_hints"])
	}
	if strings.Contains(hints[0].(string), "<type>") || strings.Contains(hints[0].(string), "<step-id>") {
		t.Fatalf("expected server-provided record hint, got %#v", hints[0])
	}
	nextCommands, _ := meta["nextCommands"].([]any)
	if len(nextCommands) > 1 {
		t.Fatalf("expected at most one compact next command, got %#v", nextCommands)
	}
	var nextCommandStrings []string
	for _, item := range nextCommands {
		if s, ok := item.(string); ok {
			nextCommandStrings = append(nextCommandStrings, s)
		}
	}
	joined := strings.Join(nextCommandStrings, "\n")
	if strings.Contains(joined, "steps docs set") || strings.Contains(joined, "steps examples add") || strings.Contains(joined, "steps tests add") {
		t.Fatalf("expected compact steps run to omit sidecar authoring hints, got %q", joined)
	}
}

func TestStepsRunResultPathAndResultFile(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"stepType":   "code",
				"stepId":     "make-output",
				"durationMs": 1,
				"result": map[string]any{
					"rows": []any{
						map[string]any{"id": "row-1", "score": 0.92},
						map[string]any{"id": "row-2", "score": 0.84},
					},
					"summary": "ready",
				},
			},
		})
	}))
	defer srv.Close()

	resultFile := filepath.Join(t.TempDir(), "result.json")
	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"steps", "run",
		"--type", "code",
		"--id", "make-output",
		"--params", `{"input":{"n":2}}`,
		"--result-path", "rows.0",
		"--result-file", resultFile,
	)
	if err != nil {
		t.Fatalf("steps run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	preview, _ := data["resultPreview"].(map[string]any)
	if found, _ := preview["pathFound"].(bool); !found {
		t.Fatalf("expected result path to be found, got %#v", preview)
	}
	value, _ := preview["value"].(string)
	if !strings.Contains(value, `:id "row-1"`) || strings.Contains(value, "row-2") {
		t.Fatalf("expected focused first-row preview, got %q", value)
	}
	var fullResult map[string]any
	if err := readJSONFile(resultFile, &fullResult); err != nil {
		t.Fatalf("read result file: %v", err)
	}
	if _, ok := fullResult["rows"].([]any); !ok {
		t.Fatalf("expected full result file to contain rows, got %#v", fullResult)
	}
}

func TestStepsRunFullPreservesResult(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"stepType":   "code",
				"stepId":     "make-output",
				"durationMs": 1,
				"result":     map[string]any{"ok": true},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"steps", "run",
		"--type", "code",
		"--id", "make-output",
		"--params", `{"input":{"n":2}}`,
		"--full",
	)
	if err != nil {
		t.Fatalf("steps run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if _, exists := data["result"]; !exists {
		t.Fatalf("expected --full to preserve data.result, got %#v", data)
	}
	if _, exists := data["resultPreview"]; exists {
		t.Fatalf("expected --full to omit resultPreview, got %#v", data["resultPreview"])
	}
}

func TestStepsRecordSendsFlowSourceAndVersionToStepRun(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var runArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["command"] {
		case "steps.run":
			runArgs, _ = body["args"].(map[string]any)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"stepType":   "code",
					"stepId":     "make-output",
					"durationMs": 1,
					"result":     map[string]any{"ok": true},
				},
			})
		case "steps.examples.add", "steps.tests.add":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"saved": true}})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"steps", "record",
		"--flow", "my-flow",
		"--source", "draft",
		"--version", "4",
		"--type", "code",
		"--id", "make-output",
		"--params", `{"input":{"n":2}}`,
	)
	if err != nil {
		t.Fatalf("steps record failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if runArgs["flowSlug"] != "my-flow" || runArgs["source"] != "draft" || runArgs["version"] != float64(4) {
		t.Fatalf("expected source/version on steps.run args, got %#v", runArgs)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if _, exists := data["result"]; !exists {
		t.Fatalf("expected legacy steps record to preserve data.result by default, got %#v", data)
	}
	if _, exists := data["resultPreview"]; exists {
		t.Fatalf("expected legacy steps record to omit resultPreview by default, got %#v", data["resultPreview"])
	}
}

func TestStepsTestsVerifySendsFlowSourceAndVersion(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"passed": true, "items": []any{}},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"steps", "tests", "verify", "my-flow", "make-output",
		"--source", "draft",
		"--version", "5",
		"--type", "code",
	)
	if err != nil {
		t.Fatalf("steps tests verify failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "steps.tests.verify" {
		t.Fatalf("expected steps.tests.verify, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["version"] != float64(5) {
		t.Fatalf("expected flow source/version args, got %#v", args)
	}
}

func readJSONFile(path string, target any) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, target)
}
