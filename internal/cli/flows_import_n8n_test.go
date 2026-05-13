package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertN8NWorkflow_HTTPAndCode(t *testing.T) {
	wf := n8nWorkflow{
		Name: "Example Import",
		Nodes: []n8nNode{
			{Name: "Manual Trigger", Type: "n8n-nodes-base.manualTrigger", Parameters: map[string]any{}},
			{
				Name: "Fetch Users",
				Type: "n8n-nodes-base.httpRequest",
				Parameters: map[string]any{
					"method":  "GET",
					"url":     "https://api.example.com/users?limit=10",
					"headers": map[string]any{"Accept": "application/json"},
				},
			},
			{
				Name:       "Transform Users",
				Type:       "n8n-nodes-base.code",
				Parameters: map[string]any{"jsCode": "return items;"},
			},
		},
		Connections: map[string]map[string][][]n8nConnection{
			"Manual Trigger": {
				"main": {{{Node: "Fetch Users", Type: "main", Index: 0}}},
			},
			"Fetch Users": {
				"main": {{{Node: "Transform Users", Type: "main", Index: 0}}},
			},
		},
	}

	result, err := convertN8NWorkflow(wf, "example-import", "tmp/flows/example-import.clj")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	assertContains(t, result.EDN, ":slug :example-import")
	assertContains(t, result.EDN, ":method :get")
	assertContains(t, result.EDN, `:query {"limit" "10"}`)
	assertContains(t, result.EDN, `:headers {"Accept" "application/json"}`)
	assertContains(t, result.EDN, ":interfaces {:manual")
	assertContains(t, result.EDN, ":ref :transform-users-fn")
	if len(result.Todos) != 1 || result.Todos[0] != `transform-users: port Code node "Transform Users" to Clojure` {
		t.Fatalf("unexpected todos: %#v", result.Todos)
	}
}

func TestImportN8NWorkflowFile_WritesDefaultShape(t *testing.T) {
	tmp := t.TempDir()
	input := filepath.Join(tmp, "workflow.json")
	out := filepath.Join(tmp, "tiny.clj")
	if err := os.WriteFile(input, []byte(`{"name":"Tiny","nodes":[{"name":"NoOp","type":"n8n-nodes-base.noOp","parameters":{}}],"connections":{}}`), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	result, err := importN8NWorkflowFile(input, "tiny", out)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.OutputPath != out {
		t.Fatalf("unexpected output path: %q", result.OutputPath)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(b)
	assertContains(t, text, ":slug :tiny")
	assertContains(t, text, "TODO(n8n-import): Custom or unsupported n8n node")
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected output to contain %q\n--- output ---\n%s", want, text)
	}
}
