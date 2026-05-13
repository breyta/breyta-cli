package cli

import (
	"bytes"
	"encoding/json"
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

func TestConvertN8NWorkflow_RealHTTPParameterArraysAndControlNodes(t *testing.T) {
	wf := n8nWorkflow{
		Name: "Control Flow Import",
		Nodes: []n8nNode{
			{Name: "Manual Trigger", Type: "n8n-nodes-base.manualTrigger", Parameters: map[string]any{}},
			{
				Name: "Call API",
				Type: "n8n-nodes-base.httpRequest",
				Parameters: map[string]any{
					"method": "POST",
					"url":    "https://api.example.com/items?from=url",
					"headerParameters": map[string]any{"parameters": []any{
						map[string]any{"name": "Authorization", "value": "Bearer {{$json.token}}"},
						map[string]any{"name": "Accept", "value": "application/json"},
					}},
					"queryParameters": map[string]any{"parameters": []any{
						map[string]any{"name": "from", "value": "params"},
						map[string]any{"name": "limit", "value": "25"},
					}},
					"bodyParameters": map[string]any{"parameters": []any{
						map[string]any{"name": "name", "value": "Ada"},
					}},
				},
			},
			{Name: "Check Result", Type: "n8n-nodes-base.if", Parameters: map[string]any{}},
			{Name: "Wait A Bit", Type: "n8n-nodes-base.wait", Parameters: map[string]any{"amount": float64(2), "unit": "minutes"}},
			{Name: "Respond", Type: "n8n-nodes-base.respondToWebhook", Parameters: map[string]any{"responseCode": float64(202)}},
		},
		Connections: map[string]map[string][][]n8nConnection{
			"Manual Trigger": {"main": {{{Node: "Call API", Type: "main", Index: 0}}}},
			"Call API":       {"main": {{{Node: "Check Result", Type: "main", Index: 0}}}},
			"Check Result":   {"main": {{{Node: "Wait A Bit", Type: "main", Index: 0}}}},
			"Wait A Bit":     {"main": {{{Node: "Respond", Type: "main", Index: 0}}}},
		},
	}

	result, err := convertN8NWorkflow(wf, "control-flow-import", "tmp/flows/control-flow-import.clj")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	assertContains(t, result.EDN, `:headers {"Accept" "application/json" "Authorization" "Bearer {{$json.token}}"}`)
	assertContains(t, result.EDN, `:query {"from" "params" "limit" "25"}`)
	assertContains(t, result.EDN, `:body {"name" "Ada"}`)
	assertContains(t, result.EDN, ":branch false")
	assertContains(t, result.EDN, ":timeout 120")
	assertContains(t, result.EDN, "{:status 202")
	assertContains(t, strings.Join(result.Todos, "\n"), "translate branch node")
	assertContains(t, strings.Join(result.Todos, "\n"), "verify Wait node")
}

func TestFlowsImportN8NCommand_WritesEnvelopeAndFile(t *testing.T) {
	tmp := t.TempDir()
	input := filepath.Join(tmp, "workflow.json")
	outPath := filepath.Join(tmp, "imported.clj")
	if err := os.WriteFile(input, []byte(`{"name":"Imported","nodes":[{"name":"NoOp","type":"n8n-nodes-base.noOp","parameters":{}}],"connections":{}}`), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	app := &App{WorkspaceID: "ws-test"}
	cmd := newFlowsImportN8NCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{input, "--slug", "imported", "--out", outPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v\n%s", err, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out.String())
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data object: %#v", envelope)
	}
	if got, _ := data["path"].(string); got != outPath {
		t.Fatalf("unexpected path: %q", got)
	}
	if got, _ := data["pushCommand"].(string); !strings.Contains(got, "breyta flows push --file") {
		t.Fatalf("unexpected push command: %q", got)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	assertContains(t, string(b), ":slug :imported")
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected output to contain %q\n--- output ---\n%s", want, text)
	}
}
