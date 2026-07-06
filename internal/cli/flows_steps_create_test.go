package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
