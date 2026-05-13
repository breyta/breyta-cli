package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
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
	assertContains(t, result.EDN, ":invocations {:default {:inputs []}}")
	assertContains(t, result.EDN, ":ref :transform-users-fn")
	assertContains(t, result.EDN, ":flow (quote")
	if !result.Validation.BalancedDelimiters || !result.Validation.EDNReadable {
		t.Fatalf("expected successful validation, got %#v", result.Validation)
	}
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
	assertContains(t, strings.Join(result.Todos, "\n"), "translate IF node")
	assertContains(t, strings.Join(result.Todos, "\n"), "verify Wait node")
}

func TestConvertN8NWorkflow_IFBranchGuardsOutputs(t *testing.T) {
	wf := n8nWorkflow{
		Name: "Branch Import",
		Nodes: []n8nNode{
			{Name: "Manual Trigger", Type: "n8n-nodes-base.manualTrigger", Parameters: map[string]any{}},
			{
				Name: "Choose Path",
				Type: "n8n-nodes-base.if",
				Parameters: map[string]any{
					"conditions": map[string]any{
						"boolean": []any{map[string]any{
							"value1":    "={{$json.active}}",
							"operation": "true",
						}},
					},
				},
			},
			{
				Name: "True Path",
				Type: "n8n-nodes-base.set",
				Parameters: map[string]any{"values": map[string]any{
					"string": []any{map[string]any{"name": "path", "value": "true"}},
				}},
			},
			{
				Name: "False Path",
				Type: "n8n-nodes-base.set",
				Parameters: map[string]any{"values": map[string]any{
					"string": []any{map[string]any{"name": "path", "value": "false"}},
				}},
			},
		},
		Connections: map[string]map[string][][]n8nConnection{
			"Manual Trigger": {"main": {{{Node: "Choose Path", Type: "main", Index: 0}}}},
			"Choose Path": {
				"main": {
					{{Node: "True Path", Type: "main", Index: 0}},
					{{Node: "False Path", Type: "main", Index: 0}},
				},
			},
		},
	}

	result, err := convertN8NWorkflow(wf, "branch-import", "tmp/flows/branch-import.clj")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	assertContains(t, result.EDN, "(assoc input :branch (get input :active))")
	assertContains(t, result.EDN, "(if (true? (:branch choose_path))")
	assertContains(t, result.EDN, "(if (false? (:branch choose_path))")
	assertContains(t, result.EDN, ":n8n-import/skipped true")
	if strings.Contains(strings.Join(result.Todos, "\n"), "translate IF node") {
		t.Fatalf("did not expect IF translation TODO, got %#v", result.Todos)
	}
}

func TestConvertN8NWorkflow_SetTranslatesDirectUpstreamNodeRefs(t *testing.T) {
	wf := n8nWorkflow{
		Name: "Node Ref Import",
		Nodes: []n8nNode{
			{Name: "Manual Trigger", Type: "n8n-nodes-base.manualTrigger", Parameters: map[string]any{}},
			{
				Name: "Lookup",
				Type: "n8n-nodes-base.set",
				Parameters: map[string]any{"values": map[string]any{
					"number": []any{map[string]any{"name": "page", "value": float64(1)}},
				}},
			},
			{
				Name: "Next Page",
				Type: "n8n-nodes-base.set",
				Parameters: map[string]any{"values": map[string]any{
					"number": []any{map[string]any{"name": "page", "value": `={{$node["Lookup"].json["page"]++}}`}},
				}},
			},
		},
		Connections: map[string]map[string][][]n8nConnection{
			"Manual Trigger": {"main": {{{Node: "Lookup", Type: "main", Index: 0}}}},
			"Lookup":         {"main": {{{Node: "Next Page", Type: "main", Index: 0}}}},
		},
	}

	result, err := convertN8NWorkflow(wf, "node-ref-import", "tmp/flows/node-ref-import.clj")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	assertContains(t, result.EDN, ":page (inc (or (get input :page) 0))")
	if strings.Contains(strings.Join(result.Todos, "\n"), "translate n8n expression") {
		t.Fatalf("did not expect expression translation TODO, got %#v", result.Todos)
	}
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
	validation, ok := data["validation"].(map[string]any)
	if !ok {
		t.Fatalf("missing validation object: %#v", data)
	}
	if validation["balancedDelimiters"] != true || validation["ednReadable"] != true {
		t.Fatalf("unexpected validation: %#v", validation)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	assertContains(t, string(b), ":slug :imported")
}

func TestValidateGeneratedN8NFlowEDNRejectsInvalidFlow(t *testing.T) {
	if _, err := validateGeneratedN8NFlowEDN("{:slug :bad :flow (quote (let [x 1] x))"); err == nil {
		t.Fatalf("expected delimiter validation error")
	}
	if _, err := validateGeneratedN8NFlowEDN("{:slug :bad :name \"Bad\" :flow (quote (identity 1))}"); err == nil {
		t.Fatalf("expected missing required keys error")
	}
	if _, err := validateGeneratedN8NFlowEDN("{:slug :bad :name \"Bad\" :concurrency {} :invocations {} :interfaces {} :flow (identity 1)}"); err == nil {
		t.Fatalf("expected non-quoted flow error")
	}
	if _, err := validateGeneratedN8NFlowEDN("{:slug :bad :name \"Bad\" :concurrency {} :invocations {:default {:inputs [{:name :payload :type :json}]}} :interfaces {} :flow (quote (identity 1))}"); err == nil {
		t.Fatalf("expected unsupported invocation input type error")
	}
}

func TestImportN8NWorkflow_RealPublicTemplateFixtures(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "n8n", "*.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(fixtures) < 4 {
		t.Fatalf("expected public n8n fixtures, got %d", len(fixtures))
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(path.Base(fixture), func(t *testing.T) {
			slug := "fixture-" + strings.TrimSuffix(path.Base(fixture), ".json")
			result, err := importN8NWorkflowFile(fixture, slug, filepath.Join(t.TempDir(), slug+".clj"))
			if err != nil {
				t.Fatalf("import fixture: %v", err)
			}
			if result.Slug != slug {
				t.Fatalf("unexpected slug: %q", result.Slug)
			}
			if !result.Validation.BalancedDelimiters || !result.Validation.EDNReadable {
				t.Fatalf("expected valid generated EDN, got %#v", result.Validation)
			}
			assertContains(t, result.EDN, ":flow (quote")
			assertContains(t, result.EDN, ":interfaces {:manual")
		})
	}
}

func TestConvertN8NWorkflow_DedupesRequirementsAndRendersNestedValues(t *testing.T) {
	wf := n8nWorkflow{
		Name: "Nested Import",
		Nodes: []n8nNode{
			{Name: "Manual Trigger", Type: "n8n-nodes-base.manualTrigger", Parameters: map[string]any{}},
			{
				Name: "Shared API One",
				Type: "n8n-nodes-base.httpRequest",
				Credentials: map[string]any{
					"httpHeaderAuth": map[string]any{"name": "Shared API"},
				},
				Parameters: map[string]any{
					"method": "POST",
					"url":    "https://api.example.com/one",
					"body": map[string]any{
						"user": map[string]any{"id": float64(7), "name": "Ada"},
						"tags": []any{"alpha", "beta"},
					},
				},
			},
			{
				Name: "Shared API Two",
				Type: "n8n-nodes-base.httpRequest",
				Credentials: map[string]any{
					"httpHeaderAuth": map[string]any{"name": "Shared API"},
				},
				Parameters: map[string]any{
					"method": "POST",
					"url":    "https://api.example.com/two",
				},
			},
			{
				Name: "Set Fields",
				Type: "n8n-nodes-base.set",
				Parameters: map[string]any{
					"values": map[string]any{
						"string": []any{
							map[string]any{"name": "customerId", "value": "{{$json.customer.id}}"},
							map[string]any{"name": "sum", "value": "{{$json.a + $json.b}}"},
							map[string]any{"name": "label", "value": "Customer {{$json.customer.id}}"},
							map[string]any{"name": "status", "value": "{{$json.active ? \"yes\" : \"no\"}}"},
							map[string]any{"name": "fallback", "value": "{{$node[\"Other\"].json.id}}"},
						},
						"number": []any{
							map[string]any{"name": "score", "value": float64(9)},
						},
					},
				},
			},
		},
		Connections: map[string]map[string][][]n8nConnection{
			"Manual Trigger": {"main": {{{Node: "Shared API One", Type: "main", Index: 0}}}},
			"Shared API One": {"main": {{{Node: "Shared API Two", Type: "main", Index: 0}}}},
			"Shared API Two": {"main": {{{Node: "Set Fields", Type: "main", Index: 0}}}},
		},
	}

	result, err := convertN8NWorkflow(wf, "nested-import", "tmp/flows/nested-import.clj")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	if got := strings.Count(result.EDN, ":slot :shared-api"); got != 1 {
		t.Fatalf("expected one shared-api requirement, got %d\n%s", got, result.EDN)
	}
	assertContains(t, result.EDN, `:body {"tags" ["alpha" "beta"] "user" {"id" 7 "name" "Ada"}}`)
	assertContains(t, result.EDN, `:customerid (get-in input [:customer :id])`)
	assertContains(t, result.EDN, `:sum (+ (get input :a) (get input :b))`)
	assertContains(t, result.EDN, `:label (str \"Customer \" (get-in input [:customer :id]))`)
	assertContains(t, result.EDN, `:status (if (get input :active) \"yes\" \"no\")`)
	assertContains(t, result.EDN, `:fallback \"{{$node[`)
	assertContains(t, result.EDN, `:score 9`)
	assertContains(t, strings.Join(result.Todos, "\n"), `translate n8n expression for Set field "fallback"`)
}

func TestTranslateSimpleN8NExpression(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "{{$json}}", want: "input", ok: true},
		{in: "{{$json.foo}}", want: "(get input :foo)", ok: true},
		{in: "{{$json.foo.bar}}", want: "(get-in input [:foo :bar])", ok: true},
		{in: `={{$json["query"]["first_name"]}}`, want: "(get-in input [:query :first-name])", ok: true},
		{in: `={{$json["page"]++}}`, want: "(inc (or (get input :page) 0))", ok: true},
		{in: "{{$now}}", want: "(flow/now-ms)", ok: true},
		{in: "{{$json.a + $json.b}}", want: "(+ (get input :a) (get input :b))", ok: true},
		{in: "{{$json.active ? \"yes\" : \"no\"}}", want: "(if (get input :active) \"yes\" \"no\")", ok: true},
		{in: "hello {{$json.foo}}", ok: false},
	}
	for _, tc := range cases {
		got, ok := translateSimpleN8NExpression(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("translateSimpleN8NExpression(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTranslateN8NTemplateString(t *testing.T) {
	got, ok := translateN8NTemplateString("Hello {{$json.name}} from {{$json.company.name}}")
	if !ok {
		t.Fatalf("expected template string to translate")
	}
	want := `(str "Hello " (get input :name) " from " (get-in input [:company :name]))`
	if got != want {
		t.Fatalf("unexpected template translation:\n got: %s\nwant: %s", got, want)
	}
	got, ok = translateN8NTemplateString(`Search {{$json["query"]["first_name"]}} {{$json["query"]["last_name"]}}`)
	if !ok {
		t.Fatalf("expected bracket path template string to translate")
	}
	want = `(str "Search " (get-in input [:query :first-name]) " " (get-in input [:query :last-name]))`
	if got != want {
		t.Fatalf("unexpected bracket template translation:\n got: %s\nwant: %s", got, want)
	}
	if got, ok := translateN8NTemplateString("Hello {{$node[\"Other\"].json.id}}"); ok {
		t.Fatalf("expected node reference template to remain unsupported, got %s", got)
	}
	got, ok = translateN8NTemplateStringWithRefs(
		`Next {{$node["Lookup"].json["page"]}}`,
		map[string]string{"Lookup": "(get input :lookup)"},
	)
	if !ok {
		t.Fatalf("expected node reference template to translate with explicit input refs")
	}
	want = `(str "Next " (get (get input :lookup) :page))`
	if got != want {
		t.Fatalf("unexpected node ref template translation:\n got: %s\nwant: %s", got, want)
	}
	got, ok = translateSimpleN8NExpressionWithRefs(
		`={{$node["Lookup"].json["page"]++}}`,
		map[string]string{"Lookup": "input"},
	)
	if !ok {
		t.Fatalf("expected node ref increment to translate with explicit input refs")
	}
	want = `(inc (or (get input :page) 0))`
	if got != want {
		t.Fatalf("unexpected node ref increment translation:\n got: %s\nwant: %s", got, want)
	}
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected output to contain %q\n--- output ---\n%s", want, text)
	}
}
