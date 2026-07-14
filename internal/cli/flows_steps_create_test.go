package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func hasFlowStepProjectionCommand(commands []string, want string) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}
	return false
}

func TestFlowsStepsCreateSendsScaffoldPayload(t *testing.T) {
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
				"flowSlug": "my-flow",
				"source":   "draft",
				"stepId":   "tools/make-output",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "steps", "create", "my-flow", "tools/make-output",
		"--type", "function",
		"--title", "Make output",
		"--description", "Draft output builder",
	)
	if err != nil {
		t.Fatalf("flows steps create failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.create" {
		t.Fatalf("expected flows.steps.create, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "tools/make-output" {
		t.Fatalf("expected flow-scoped create args, got %#v", args)
	}
	if args["type"] != "function" || args["title"] != "Make output" || args["description"] != "Draft output builder" {
		t.Fatalf("expected scaffold args, got %#v", args)
	}
	if _, exists := args["stepLiteral"]; exists {
		t.Fatalf("expected scaffold mode to omit stepLiteral, got %#v", args["stepLiteral"])
	}
}

func TestFlowsStepsRunHonorsCommandCancellation(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, stderr, err := runCLIArgsWithContext(t, ctx,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "run", "my-flow", "tools/make-output", "--source", "draft",
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v\n%s", err, stderr)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("expected step run to stop promptly on cancellation, took %s", elapsed)
	}
}

func TestFlowsStepsListAndShowTargetSelectedAuthoringSource(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var commands []string
	var payloads []map[string]any
	var projectionsMu sync.Mutex
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		projectionsMu.Lock()
		commands = append(commands, got["command"].(string))
		args, _ := got["args"].(map[string]any)
		payloads = append(payloads, args)
		projectionsMu.Unlock()
		data := map[string]any{"analysis": map[string]any{"steps": []any{}}}
		if got["command"] == "flows.get" {
			data = map[string]any{"flow": map[string]any{"steps": []any{map[string]any{
				"id":   "tools/make-output",
				"type": "function",
			}}}}
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        data,
		})
	}))
	defer srv.Close()

	baseArgs := []string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}
	for _, cliArgs := range [][]string{
		{"flows", "steps", "list", "my-flow", "--source", "draft"},
		{"flows", "steps", "show", "my-flow", "tools/make-output", "--version", "7"},
		{"flows", "steps", "list", "my-flow", "--source", "latest", "--version", "6"},
	} {
		stdout, stderr, err := runCLIArgs(t, append(baseArgs, cliArgs...)...)
		if err != nil {
			t.Fatalf("%v failed: %v\nstdout=%s\nstderr=%s", cliArgs, err, stdout, stderr)
		}
	}

	if len(commands) != 6 {
		t.Fatalf("expected get+compile calls, got %#v", commands)
	}
	commandCounts := map[string]int{}
	payloadCounts := map[string]int{}
	for i, command := range commands {
		commandCounts[command]++
		key := fmt.Sprintf("%v:%v", payloads[i]["source"], payloads[i]["version"])
		payloadCounts[key]++
	}
	if commandCounts["flows.get"] != 3 || commandCounts["flows.compile"] != 3 {
		t.Fatalf("expected three get+compile projection pairs, got %#v", commands)
	}
	if payloadCounts["draft:<nil>"] != 2 || payloadCounts["version:7"] != 2 || payloadCounts["version:6"] != 2 {
		t.Fatalf("expected draft and historical projection pairs, got %#v", payloads)
	}
}

func TestFlowsStepsListDefaultsToDraftCompileProjection(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var sources []any
	var sourcesMu sync.Mutex
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		args, _ := got["args"].(map[string]any)
		sourcesMu.Lock()
		sources = append(sources, args["source"])
		sourcesMu.Unlock()
		data := map[string]any{"steps": []any{}}
		if got["command"] == "flows.compile" {
			data = map[string]any{"analysis": map[string]any{"steps": []any{map[string]any{
				"id":   "legacy-step",
				"type": "code",
			}}}}
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "workspaceId": "ws-acme", "data": data,
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "list", "my-flow",
	)
	if err != nil {
		t.Fatalf("legacy flows steps list failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "legacy-step") {
		t.Fatalf("expected legacy compiled step, got %s", stdout)
	}
	if len(sources) != 2 || sources[0] != "draft" || sources[1] != "draft" {
		t.Fatalf("expected API-compatible draft compile plus lightweight authored lookup, got %#v", sources)
	}
}

func TestFlowsStepsShowDefaultsToDraftSource(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var sources []any
	var sourcesMu sync.Mutex
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		args, _ := got["args"].(map[string]any)
		sourcesMu.Lock()
		sources = append(sources, args["source"])
		sourcesMu.Unlock()
		data := map[string]any{"analysis": map[string]any{"steps": []any{}}}
		if got["command"] == "flows.get" {
			data = map[string]any{"flow": map[string]any{"steps": []any{map[string]any{
				"id": "draft-step", "type": "function",
			}}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "workspaceId": "ws-acme", "data": data,
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "show", "my-flow", "draft-step",
	)
	if err != nil {
		t.Fatalf("flows steps show failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(sources) != 2 || sources[0] != "draft" || sources[1] != "draft" {
		t.Fatalf("expected show projections to use the API-compatible draft default, got %#v", sources)
	}
}

func TestFlowsStepsListRejectsMismatchedProjectionRevisions(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["command"] == "flows.get" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"meta": map[string]any{"contentHash": "revision-a"},
				"data": map[string]any{"flow": map[string]any{"steps": []any{}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"contentHash": "revision-b",
				"analysis":    map[string]any{"steps": []any{}},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "list", "my-flow",
	)
	if err == nil || !strings.Contains(stdout+stderr+err.Error(), "flow changed while reading step projections") {
		t.Fatalf("expected mismatched revision failure, err=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
}

func TestFlowsStepsListAcceptsMatchingLegacyProjectionRevision(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["command"] == "flows.get" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"meta": map[string]any{"contentHash": "-847176450"},
				"data": map[string]any{"flow": map[string]any{"steps": []any{}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"contentHash": "-847176450",
				"analysis": map[string]any{"steps": []any{map[string]any{
					"id": "legacy-step", "type": "code",
				}}},
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "list", "my-flow",
	)
	if err != nil {
		t.Fatalf("matching legacy revision failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "legacy-step") {
		t.Fatalf("expected legacy compiled step, got %s", stdout)
	}
}

func TestFlowsStepsListUsesAuthoredProjectionWhenLegacyCompileIsUnavailable(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var commands []string
	var commandsMu sync.Mutex
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		command, _ := got["command"].(string)
		commandsMu.Lock()
		commands = append(commands, command)
		commandsMu.Unlock()
		if command == "flows.compile" {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{"flow": map[string]any{"steps": []any{map[string]any{
				"id": "authored-fallback", "type": "function",
			}}}},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "list", "my-flow",
	)
	if err != nil {
		t.Fatalf("legacy fallback list failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(commands) != 2 || !hasFlowStepProjectionCommand(commands, "flows.get") || !hasFlowStepProjectionCommand(commands, "flows.compile") {
		t.Fatalf("expected lightweight authored projection plus compile fallback, got %#v", commands)
	}
	if !strings.Contains(stdout, "authored-fallback") {
		t.Fatalf("expected authored fallback step, got %s", stdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	meta, _ := envelope["meta"].(map[string]any)
	if meta["partial"] != true {
		t.Fatalf("expected partial result metadata, got %#v", meta)
	}
}

func TestFlowsStepsListMergesActivePackagedAndInlineSteps(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["command"] == "flows.compile" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{"analysis": map[string]any{"steps": []any{map[string]any{
					"id": "inline-step", "type": "code",
				}}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{"flow": map[string]any{"steps": []any{map[string]any{
				"id": "agents/reviewer", "type": "agent",
			}}}},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "list", "my-flow",
	)
	if err != nil {
		t.Fatalf("active list failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "inline-step") || !strings.Contains(stdout, "agents/reviewer") {
		t.Fatalf("expected inline and packaged steps, got %s", stdout)
	}
}

func TestFlowsStepsListTreatsOKFalseProjectionAsPartial(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["command"] == "flows.get" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false, "error": map[string]any{"message": "authored projection unavailable"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{"analysis": map[string]any{"steps": []any{map[string]any{
				"id": "inline-step", "type": "code",
			}}}},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "list", "my-flow",
	)
	if err != nil {
		t.Fatalf("partial active list failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	meta, _ := envelope["meta"].(map[string]any)
	if meta["partial"] != true || !strings.Contains(stdout, "authored") {
		t.Fatalf("expected ok=false authored projection to be marked partial, got %#v", meta)
	}
}

func TestFlowsStepsShowSurfacesFailedProjectionWhenStepIsAbsent(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["command"] == "flows.compile" {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false, "error": map[string]any{"message": "compile projection failed"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "data": map[string]any{"flow": map[string]any{"steps": []any{}}},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "show", "my-flow", "missing-step",
	)
	if err == nil || !strings.Contains(stdout+stderr+err.Error(), "compile projection failed") {
		t.Fatalf("expected projection failure instead of step-not-found, err=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
}

func TestFlowsStepsShowLoadsFullAuthoredDefinitionForIncludes(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var commands []string
	var getArgs map[string]any
	var commandsMu sync.Mutex
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		command, _ := got["command"].(string)
		commandsMu.Lock()
		commands = append(commands, command)
		commandsMu.Unlock()
		switch command {
		case "flows.compile":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
		case "flows.get":
			getArgs, _ = got["args"].(map[string]any)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{"flow": map[string]any{"steps": []any{map[string]any{
					"id":            "tools/make-output",
					"type":          "function",
					"input-schema":  []any{"map"},
					"output-schema": []any{"map", []any{"summary", "string"}},
					"defaults":      map[string]any{"ref": "make-output"},
				}}}},
			})
		default:
			http.Error(w, "unexpected command", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "show", "my-flow", "tools/make-output",
		"--source", "draft", "--include", "schemas,definition",
	)
	if err != nil {
		t.Fatalf("authored step show failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(commands) != 2 || !hasFlowStepProjectionCommand(commands, "flows.get") || !hasFlowStepProjectionCommand(commands, "flows.compile") {
		t.Fatalf("expected lightweight authored lookup and compile, got %#v", commands)
	}
	if getArgs["source"] != "draft" || getArgs["includeFlowLiteral"] != false ||
		getArgs["includeTemplates"] != false || getArgs["includeFunctions"] != false {
		t.Fatalf("expected scoped flows.get detail payload, got %#v", getArgs)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	step := envelope["data"].(map[string]any)["step"].(map[string]any)
	definition, _ := step["definition"].(map[string]any)
	if definition["id"] != "tools/make-output" || definition["defaults"] == nil {
		t.Fatalf("expected full authored definition, got %#v", definition)
	}
	if step["inputSchema"] == nil || step["outputSchema"] == nil {
		t.Fatalf("expected authored schemas, got %#v", step)
	}
}

func TestFlowsStepsShowIncludesCompiledInlineDefinitionWhenNoPackagedStepExists(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["command"] == "flows.compile" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{"analysis": map[string]any{"steps": []any{map[string]any{
					"id":   "inline-step",
					"type": "code",
					"config": `{:code "(fn [input] input)"
 :input-schema [:map]
 :output-schema [:map]}`,
				}}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "data": map[string]any{"flow": map[string]any{"steps": []any{}}},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "show", "my-flow", "inline-step", "--include", "schemas,definition",
	)
	if err != nil {
		t.Fatalf("inline step show failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	step := envelope["data"].(map[string]any)["step"].(map[string]any)
	definition, _ := step["definition"].(map[string]any)
	if definition["code"] != "(fn [input] input)" || step["inputSchema"] == nil || step["outputSchema"] == nil {
		t.Fatalf("expected compiled inline details, got %#v", step)
	}
}

func TestFlowsStepsShowPreservesUnsupportedClojureConfigLiteral(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	configLiteral := `{:pattern #"^[a-z]+$" :input-schema [:map]}`
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["command"] == "flows.compile" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{"analysis": map[string]any{"steps": []any{map[string]any{
					"id":     "regex-step",
					"type":   "function",
					"config": configLiteral,
				}}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "data": map[string]any{"flow": map[string]any{"steps": []any{}}},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev",
		"flows", "steps", "show", "my-flow", "regex-step", "--include", "schemas,definition",
	)
	if err != nil {
		t.Fatalf("regex config step show failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	step := envelope["data"].(map[string]any)["step"].(map[string]any)
	definition, _ := step["definition"].(map[string]any)
	if step["config"] != configLiteral || definition["config"] != configLiteral {
		t.Fatalf("expected raw Clojure config literal to be preserved, got %#v", step)
	}
}

func TestFlowsStepsLocalModeRejectsSourceAndVersionSelectors(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	statePath := filepath.Join(t.TempDir(), "state.json")
	base := []string{"--dev", "--workspace", "ws-acme", "--state", statePath, "--api", ""}
	for _, cliArgs := range [][]string{
		{"flows", "steps", "list", "subscription-renewal", "--source", "draft"},
		{"flows", "steps", "show", "subscription-renewal", "validate-plan", "--version", "1"},
	} {
		stdout, stderr, err := runCLIArgs(t, append(base, cliArgs...)...)
		if err == nil || !strings.Contains(stdout+stderr+err.Error(), "require API mode") {
			t.Fatalf("expected local selector rejection for %v, err=%v\nstdout=%s\nstderr=%s", cliArgs, err, stdout, stderr)
		}
	}
}

func TestFlowsStepsCreateSendsFileLiteral(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	stepFile := filepath.Join(t.TempDir(), "step.edn")
	if err := os.WriteFile(stepFile, []byte(`{:id :tools/make-output
 :type :function
 :description "Make output"
 :input-schema [:map]
 :defaults {:ref :make-output}}`), 0o600); err != nil {
		t.Fatalf("write step file: %v", err)
	}

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
				"flowSlug": "my-flow",
				"source":   "draft",
				"stepId":   "tools/make-output",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "steps", "create", "my-flow", "tools/make-output",
		"--file", stepFile,
	)
	if err != nil {
		t.Fatalf("flows steps create --file failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.create" {
		t.Fatalf("expected flows.steps.create, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	literal, _ := args["stepLiteral"].(string)
	if !strings.Contains(literal, ":id :tools/make-output") || !strings.Contains(literal, ":type :function") {
		t.Fatalf("expected file literal to be forwarded, got %q", literal)
	}
	if _, exists := args["type"]; exists {
		t.Fatalf("expected file mode to omit type, got %#v", args["type"])
	}
	if _, exists := args["title"]; exists {
		t.Fatalf("expected file mode to omit title, got %#v", args["title"])
	}
}

func TestFlowsStepsCreateRejectsConflictingOrBlankFileMode(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	stepFile := filepath.Join(t.TempDir(), "step.edn")
	if err := os.WriteFile(stepFile, []byte(`{:id :tools/make-output :type :function}`), 0o600); err != nil {
		t.Fatalf("write step file: %v", err)
	}
	emptyFile := filepath.Join(t.TempDir(), "empty-step.edn")
	if err := os.WriteFile(emptyFile, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("write empty step file: %v", err)
	}

	requests := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "request should not be sent", http.StatusInternalServerError)
	}))
	defer srv.Close()

	baseArgs := []string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "mixed file and scaffold",
			args: []string{
				"flows", "steps", "create", "my-flow", "tools/make-output",
				"--file", stepFile, "--type", "function", "--title", "Make output",
			},
			wantErr: "--file cannot be combined",
		},
		{
			name: "blank explicit file",
			args: []string{
				"flows", "steps", "create", "my-flow", "tools/make-output", "--file", "",
			},
			wantErr: "--file requires a non-empty path",
		},
		{
			name: "whitespace-only file",
			args: []string{
				"flows", "steps", "create", "my-flow", "tools/make-output", "--file", emptyFile,
			},
			wantErr: "--file must contain a non-empty packaged step literal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCLIArgs(t, append(baseArgs, tc.args...)...)
			if err == nil || !strings.Contains(stdout+stderr+err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, err=%v\nstdout=%s\nstderr=%s", tc.wantErr, err, stdout, stderr)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("expected invalid file modes to fail before API request, got %d request(s)", requests)
	}
}

func TestFlowsStepsUpdateSendsDottedEdits(t *testing.T) {
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
				"flowSlug": "my-flow",
				"source":   "draft",
				"stepId":   "tools/make-output",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "steps", "update", "my-flow", "tools/make-output",
		"defaults.input.n", "5",
		"tool.description", "Make output updated",
	)
	if err != nil {
		t.Fatalf("flows steps update failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.update" {
		t.Fatalf("expected flows.steps.update, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "tools/make-output" {
		t.Fatalf("expected flow-scoped update args, got %#v", args)
	}
	edits, _ := args["edits"].([]any)
	if len(edits) != 2 {
		t.Fatalf("expected two edits, got %#v", args["edits"])
	}
	first, _ := edits[0].(map[string]any)
	second, _ := edits[1].(map[string]any)
	if first["path"] != "defaults.input.n" || first["value"] != "5" {
		t.Fatalf("expected first edit payload, got %#v", first)
	}
	if second["path"] != "tool.description" || second["value"] != "Make output updated" {
		t.Fatalf("expected second edit payload, got %#v", second)
	}
	if _, exists := args["stepLiteral"]; exists {
		t.Fatalf("expected edit mode to omit stepLiteral, got %#v", args["stepLiteral"])
	}
}

func TestFlowsStepsUpdateReadsResultFnFile(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	fnFile := filepath.Join(t.TempDir(), "normalize_merged_prs.clj")
	if err := os.WriteFile(fnFile, []byte(`(fn [result] {:summary (:summary result)})`), 0o600); err != nil {
		t.Fatalf("write function file: %v", err)
	}

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
				"flowSlug": "my-flow",
				"source":   "draft",
				"stepId":   "tools/make-output",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "steps", "update", "my-flow", "tools/make-output",
		"result.fnFile", fnFile,
	)
	if err != nil {
		t.Fatalf("flows steps update result.fnFile failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.update" {
		t.Fatalf("expected flows.steps.update, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	edits, _ := args["edits"].([]any)
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", args["edits"])
	}
	edit, _ := edits[0].(map[string]any)
	if edit["path"] != "result.fnFile" || edit["value"] != fnFile {
		t.Fatalf("expected result.fnFile path/value edit, got %#v", edit)
	}
	if edit["fileContents"] != "(fn [result] {:summary (:summary result)})" {
		t.Fatalf("expected function file contents, got %#v", edit["fileContents"])
	}
}

func TestFlowsStepsUpdateSendsFileLiteral(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	stepFile := filepath.Join(t.TempDir(), "step.edn")
	if err := os.WriteFile(stepFile, []byte(`{:id :tools/make-output
 :type :function
 :description "Make output updated"
 :input-schema [:map]
 :defaults {:ref :make-output}}`), 0o600); err != nil {
		t.Fatalf("write step file: %v", err)
	}

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
				"flowSlug": "my-flow",
				"source":   "draft",
				"stepId":   "tools/make-output",
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "steps", "update", "my-flow", "tools/make-output",
		"--file", stepFile,
	)
	if err != nil {
		t.Fatalf("flows steps update --file failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.update" {
		t.Fatalf("expected flows.steps.update, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	literal, _ := args["stepLiteral"].(string)
	if !strings.Contains(literal, ":id :tools/make-output") || !strings.Contains(literal, ":description \"Make output updated\"") {
		t.Fatalf("expected file literal to be forwarded, got %q", literal)
	}
	if _, exists := args["edits"]; exists {
		t.Fatalf("expected file mode to omit edits, got %#v", args["edits"])
	}
}

func TestFlowsStepsUpdateRejectsConflictingOrBlankFileMode(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	emptyFile := filepath.Join(t.TempDir(), "empty-step.edn")
	if err := os.WriteFile(emptyFile, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("write empty step file: %v", err)
	}

	requests := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "request should not be sent", http.StatusInternalServerError)
	}))
	defer srv.Close()

	baseArgs := []string{"--dev", "--workspace", "ws-acme", "--api", srv.URL, "--token", "user-dev"}
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "blank file with edits",
			args:    []string{"flows", "steps", "update", "my-flow", "tools/make-output", "--file", "", "defaults.input.n", "4"},
			wantErr: "--file cannot be combined with path/value edits",
		},
		{
			name:    "blank explicit file",
			args:    []string{"flows", "steps", "update", "my-flow", "tools/make-output", "--file", ""},
			wantErr: "--file requires a non-empty path",
		},
		{
			name:    "empty file contents",
			args:    []string{"flows", "steps", "update", "my-flow", "tools/make-output", "--file", emptyFile},
			wantErr: "--file must contain a non-empty packaged step literal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCLIArgs(t, append(baseArgs, tc.args...)...)
			if err == nil || !strings.Contains(stdout+stderr+err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, err=%v\nstdout=%s\nstderr=%s", tc.wantErr, err, stdout, stderr)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("expected invalid file modes to fail before API request, got %d request(s)", requests)
	}
}

func TestFlowsStepsRemoveSendsFlowScopedPayload(t *testing.T) {
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
				"flowSlug": "my-flow",
				"source":   "draft",
				"stepId":   "tools/make-output",
				"removed":  true,
			},
		})
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "steps", "remove", "my-flow", "tools/make-output",
	)
	if err != nil {
		t.Fatalf("flows steps remove failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.remove" {
		t.Fatalf("expected flows.steps.remove, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "tools/make-output" {
		t.Fatalf("expected flow-scoped remove args, got %#v", args)
	}
}
