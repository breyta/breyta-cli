package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/spf13/cobra"
	edn "olympos.io/encoding/edn"
)

type n8nWorkflow struct {
	Name        string                                  `json:"name"`
	Nodes       []n8nNode                               `json:"nodes"`
	Connections map[string]map[string][][]n8nConnection `json:"connections"`
}

type n8nNode struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Parameters  map[string]any `json:"parameters"`
	Credentials map[string]any `json:"credentials"`
}

type n8nConnection struct {
	Node  string `json:"node"`
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type n8nEdge struct {
	Source string
	Target string
}

type n8nConvertedNode struct {
	Node    n8nNode
	StepID  string
	VarName string
	Binding string
	Todos   []string
}

type n8nImportResult struct {
	Slug       string
	Name       string
	OutputPath string
	Todos      []string
	EDN        string
	Validation n8nFlowValidation
}

type n8nFlowValidation struct {
	BalancedDelimiters bool     `json:"balancedDelimiters"`
	EDNReadable        bool     `json:"ednReadable"`
	RequiredKeys       []string `json:"requiredKeys"`
}

var n8nSlugInvalidRe = regexp.MustCompile(`[^a-z0-9-]+`)
var n8nTemplateExprRe = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

func newFlowsImportCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import external workflow definitions as Breyta flow files",
	}
	cmd.AddCommand(newFlowsImportN8NCmd(app))
	return cmd
}

func newFlowsImportN8NCmd(app *App) *cobra.Command {
	var slug string
	var outPath string

	cmd := &cobra.Command{
		Use:   "n8n <workflow.json>",
		Short: "Convert an n8n workflow JSON export to a Breyta flow file",
		Long: strings.TrimSpace(`
Convert an n8n workflow JSON export to a best-effort Breyta EDN flow file.

The converter preserves names, prompts, request bodies, and code where possible,
adds TODO(n8n-import) notes for unsupported semantics, and never copies secret
values from credentials. The output is intended for the normal authoring loop:

  breyta flows import n8n workflow.json --slug imported-flow
  breyta flows push --file ./tmp/flows/imported-flow.clj
  breyta flows configure check imported-flow
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := importN8NWorkflowFile(args[0], slug, outPath)
			if err != nil {
				return writeFailure(cmd, app, "n8n_import_failed", err, "Check that the input is an n8n workflow JSON export.", map[string]any{"path": args[0]})
			}
			return writeData(cmd, app, nil, map[string]any{
				"slug":          result.Slug,
				"name":          result.Name,
				"path":          result.OutputPath,
				"todoCount":     len(result.Todos),
				"todos":         result.Todos,
				"validation":    result.Validation,
				"pushCommand":   fmt.Sprintf("breyta flows push --file %s", shellQuotePath(result.OutputPath)),
				"checkCommand":  fmt.Sprintf("breyta flows configure check %s", result.Slug),
				"runCommand":    fmt.Sprintf("breyta flows run %s --input '{\"payload\":{}}' --wait", result.Slug),
				"conversionDoc": "bases/flows-api/resources/public/docs/reference/GUIDE_N8N_IMPORT.md",
			})
		},
	}

	cmd.Flags().StringVar(&slug, "slug", "", "Breyta flow slug (defaults to normalized workflow name)")
	cmd.Flags().StringVar(&outPath, "out", "", "Output flow file (defaults to ./tmp/flows/<slug>.clj)")
	return cmd
}

func importN8NWorkflowFile(path, slug, outPath string) (*n8nImportResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wf n8nWorkflow
	if err := json.Unmarshal(b, &wf); err != nil {
		return nil, err
	}
	if len(wf.Nodes) == 0 {
		return nil, errors.New("n8n workflow has no nodes")
	}
	if strings.TrimSpace(slug) == "" {
		slug = n8nKebab(firstNonEmpty(wf.Name, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))), "flow")
	} else {
		slug = n8nKebab(slug, "flow")
	}
	if strings.TrimSpace(outPath) == "" {
		outPath = filepath.Join("tmp", "flows", slug+".clj")
	}
	result, err := convertN8NWorkflow(wf, slug, outPath)
	if err != nil {
		return nil, err
	}
	if err := atomicWriteFile(outPath, []byte(result.EDN), 0o644); err != nil {
		return nil, err
	}
	return result, nil
}

func convertN8NWorkflow(wf n8nWorkflow, slug, outPath string) (*n8nImportResult, error) {
	edges := n8nEdges(wf)
	ordered := n8nTopologicalOrder(wf.Nodes, edges)
	usedIDs := map[string]bool{}
	usedVars := map[string]bool{}
	requires := make([]string, 0)
	templates := make([]string, 0)
	functions := make([]string, 0)
	schedules := make([]string, 0)
	webhooks := make([]string, 0)
	todos := make([]string, 0)
	converted := make([]n8nConvertedNode, 0)
	convertedByName := map[string]n8nConvertedNode{}
	upstreams := n8nUpstreamsByTarget(edges)

	for _, node := range ordered {
		if n8nIsTrigger(node) {
			triggerRequires, triggerWebhooks, triggerSchedules := convertN8NTrigger(node, usedIDs)
			requires = appendUniqueStrings(requires, triggerRequires)
			webhooks = append(webhooks, triggerWebhooks...)
			schedules = append(schedules, triggerSchedules...)
			continue
		}
		stepID := uniqueN8NID(n8nKebab(node.Name, "step"), usedIDs)
		varName := uniqueN8NID(strings.ReplaceAll(stepID, "-", "_"), usedVars)
		binding, nodeRequires, nodeTemplates, nodeFunctions, nodeTodos := convertN8NNode(node, stepID, upstreams[node.Name], convertedByName)
		requires = appendUniqueStrings(requires, nodeRequires)
		templates = append(templates, nodeTemplates...)
		functions = append(functions, nodeFunctions...)
		for _, todo := range nodeTodos {
			todos = append(todos, stepID+": "+todo)
		}
		item := n8nConvertedNode{Node: node, StepID: stepID, VarName: varName, Binding: binding, Todos: nodeTodos}
		converted = append(converted, item)
		if strings.TrimSpace(node.Name) != "" {
			convertedByName[node.Name] = item
		}
	}

	name := firstNonEmpty(wf.Name, "Imported n8n Flow")
	body := renderN8NFlowBody(converted, upstreams, convertedByName)
	edn := renderN8NFlowEDN(slug, name, requires, templates, functions, webhooks, schedules, body)
	validation, err := validateGeneratedN8NFlowEDN(edn)
	if err != nil {
		return nil, err
	}
	return &n8nImportResult{
		Slug:       slug,
		Name:       name,
		OutputPath: outPath,
		Todos:      todos,
		EDN:        edn,
		Validation: validation,
	}, nil
}

func convertN8NTrigger(node n8nNode, usedIDs map[string]bool) ([]string, []string, []string) {
	typ := strings.ToLower(node.Type)
	switch {
	case strings.Contains(typ, "webhook"):
		id := uniqueN8NID(n8nKebab(firstNonEmpty(node.Name, "webhook"), "webhook"), usedIDs)
		return []string{`{:slot :webhook-secret
  :type :secret
  :secret-ref :webhook-secret
  :label "Webhook Secret"}`}, []string{fmt.Sprintf(`{:id :%s
   :invocation :default
   :enabled true
   :event-name %s
   :auth {:type :api-key :secret-ref :webhook-secret}}`, id, ednQuote(strings.ReplaceAll(id, "-", ".")))}, nil
	case strings.Contains(typ, "cron"), strings.Contains(typ, "schedule"), strings.Contains(typ, "interval"):
		id := uniqueN8NID(n8nKebab(firstNonEmpty(node.Name, "schedule"), "schedule"), usedIDs)
		cron := stringParam(node.Parameters, "cronExpression", "expression")
		if cron == "" {
			cron = "0 * * * *"
		}
		timezone := stringParam(node.Parameters, "timezone")
		if timezone == "" {
			timezone = "UTC"
		}
		return nil, nil, []string{fmt.Sprintf(`{:id :%s
  :label %s
  :invocation :default
  :enabled true
  :cron %s
  :timezone %s}`, id, ednQuote(firstNonEmpty(node.Name, "Schedule")), ednQuote(cron), ednQuote(timezone))}
	default:
		return nil, nil, nil
	}
}

func convertN8NNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	typ := strings.ToLower(node.Type)
	switch {
	case strings.Contains(typ, "httprequest"):
		return convertN8NHTTPNode(node, stepID, upstreams, convertedByName)
	case strings.HasSuffix(typ, ".if") || strings.Contains(typ, ".switch"):
		return convertN8NBranchNode(node, stepID, upstreams, convertedByName)
	case strings.Contains(typ, "webhookresponse") || strings.Contains(typ, "respondtowebhook"):
		return convertN8NWebhookResponseNode(node, stepID, upstreams, convertedByName)
	case strings.HasSuffix(typ, ".wait") || strings.Contains(typ, ".wait"):
		return convertN8NWaitNode(node, stepID, upstreams, convertedByName)
	case strings.HasSuffix(typ, ".set") || strings.Contains(typ, "set"):
		return convertN8NSetNode(node, stepID, upstreams, convertedByName)
	case strings.Contains(typ, "code") || strings.Contains(typ, "function"):
		return convertN8NCodeNode(node, stepID, upstreams, convertedByName)
	case strings.Contains(typ, "merge"):
		return convertN8NMergeNode(node, stepID, upstreams, convertedByName)
	default:
		return convertN8NFallbackNode(node, stepID, upstreams, convertedByName)
	}
}

func convertN8NHTTPNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	method := strings.ToLower(firstNonEmpty(stringParam(node.Parameters, "method", "requestMethod"), "GET"))
	rawURL := stringParam(node.Parameters, "url", "endpoint")
	baseURL, path, query, ok := splitN8NURL(rawURL)
	todos := make([]string, 0)
	if !ok {
		baseURL = "https://example.com"
		path = firstNonEmpty(rawURL, "/")
		todos = append(todos, fmt.Sprintf("fill base URL/path for HTTP node %q", node.Name))
	}
	slot := n8nKebab(firstNonEmpty(n8nCredentialLabel(node), node.Name, "api"), "api")
	auth := ":none"
	if n8nCredentialLabel(node) != "" {
		auth = ":api-key"
	}
	require := fmt.Sprintf(`{:slot :%s
  :type :http-api
  :label %s
  :base-url %s
  :auth {:type %s}}`, slot, ednQuote(firstNonEmpty(n8nCredentialLabel(node), node.Name, "Imported API")), ednQuote(baseURL), auth)

	requestParts := []string{":path " + ednQuote(firstNonEmpty(path, "/")), ":method :" + method}
	if queryParams := n8nParameterPairs(node.Parameters, "queryParameters", "queryParameter"); len(queryParams) > 0 {
		query = mergeStringMaps(query, queryParams)
	}
	if len(query) > 0 {
		requestParts = append(requestParts, ":query "+renderStringMap(query))
	}
	if headers := n8nParameterPairs(node.Parameters, "headers", "headerParameters", "headerParameter"); len(headers) > 0 {
		requestParts = append(requestParts, ":headers "+renderStringMap(headers))
	}
	if body, ok := firstParam(node.Parameters, "body", "jsonBody"); ok && body != nil && body != "" {
		requestParts = append(requestParts, ":body "+ednValue(body))
	} else if bodyParams := n8nParameterPairs(node.Parameters, "bodyParameters", "bodyParameter"); len(bodyParams) > 0 {
		requestParts = append(requestParts, ":body "+renderStringMap(bodyParams))
	}
	template := fmt.Sprintf(`{:id :%s-request
  :type :http-request
  :request {%s}}`, stepID, strings.Join(requestParts, "\n            "))

	input := n8nInputExpr(upstreams, convertedByName)
	binding := fmt.Sprintf(`(flow/step :http :%s
           {:title %s
            :connection :%s
            :template :%s-request
            :persist {:type :blob}
            :data {:input %s}})`, stepID, ednQuote(firstNonEmpty(node.Name, stepID)), slot, stepID, input)
	return binding, []string{require}, []string{template}, nil, todos
}

func convertN8NBranchNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	code := fmt.Sprintf("(fn [input]\n  ;; TODO(n8n-import): translate branch node %s conditions and graph outputs.\n  ;; Safe default keeps data on the false/pass-through path to avoid unintended side effects.\n  (assoc input :branch false))", ednQuote(firstNonEmpty(node.Name, stepID)))
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, n8nInputExpr(upstreams, convertedByName))
	return binding, nil, nil, []string{fn}, []string{fmt.Sprintf("translate branch node %q conditions and true/false outputs", node.Name)}
}

func convertN8NWebhookResponseNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	statusCode := intParam(node.Parameters, 200, "responseCode", "statusCode")
	code := fmt.Sprintf("(fn [input]\n  {:status %d\n   :headers {}\n   :body input})", statusCode)
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, n8nInputExpr(upstreams, convertedByName))
	return binding, nil, nil, []string{fn}, nil
}

func convertN8NWaitNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	amount := intParam(node.Parameters, 1, "amount", "value")
	unit := strings.ToLower(firstNonEmpty(stringParam(node.Parameters, "unit"), "seconds"))
	timeout := amount
	switch unit {
	case "minutes", "minute":
		timeout *= 60
	case "hours", "hour":
		timeout *= 3600
	case "days", "day":
		timeout *= 86400
	}
	input := n8nInputExpr(upstreams, convertedByName)
	binding := fmt.Sprintf(`(flow/step :wait :%s
           {:title %s
            :timeout %d
            :on-timeout :continue
            :default-value %s})`, stepID, ednQuote(firstNonEmpty(node.Name, stepID)), timeout, input)
	return binding, nil, nil, nil, []string{fmt.Sprintf("verify Wait node %q timing semantics", node.Name)}
}

func convertN8NSetNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	code := "(fn [input] input)"
	todos := []string{fmt.Sprintf("port Set node %q parameters", node.Name)}
	if values, ok := node.Parameters["values"].(map[string]any); ok {
		assoc := make([]string, 0)
		todos = nil
		for _, rawGroup := range values {
			group, ok := rawGroup.([]any)
			if !ok {
				continue
			}
			for _, rawItem := range group {
				item, ok := rawItem.(map[string]any)
				if !ok {
					continue
				}
				name := fmt.Sprint(item["name"])
				if strings.TrimSpace(name) == "" {
					continue
				}
				value := item["value"]
				if rendered, todo, ok := renderN8NSetValue(name, value); ok {
					assoc = append(assoc, ":"+n8nKebab(name, "field")+" "+rendered)
					if todo != "" {
						todos = append(todos, todo)
					}
				} else {
					assoc = append(assoc, ":"+n8nKebab(name, "field")+" "+ednValue(value))
				}
			}
		}
		if len(assoc) > 0 {
			code = "(fn [input]\n  (assoc input\n    " + strings.Join(assoc, "\n    ") + "))"
		} else if len(todos) == 0 {
			todos = []string{fmt.Sprintf("port Set node %q parameters", node.Name)}
		}
	}
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, n8nInputExpr(upstreams, convertedByName))
	return binding, nil, nil, []string{fn}, todos
}

func renderN8NSetValue(fieldName string, value any) (string, string, bool) {
	s, ok := value.(string)
	if !ok || !strings.Contains(s, "{{") {
		return "", "", false
	}
	if expr, ok := translateSimpleN8NExpression(s); ok {
		return expr, "", true
	}
	if expr, ok := translateN8NTemplateString(s); ok {
		return expr, "", true
	}
	return ednQuote(s), fmt.Sprintf("translate n8n expression for Set field %q", fieldName), true
}

func translateSimpleN8NExpression(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{{") || !strings.HasSuffix(trimmed, "}}") {
		return "", false
	}
	expr := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"))
	return translateN8NInnerExpression(expr)
}

func translateN8NInnerExpression(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "$json":
		return "input", true
	case "$now":
		return "(flow/now-ms)", true
	}
	if rendered, ok := translateN8NTernaryExpression(expr); ok {
		return rendered, true
	}
	if rendered, ok := translateN8NBinaryExpression(expr); ok {
		return rendered, true
	}
	const jsonPrefix = "$json."
	if strings.HasPrefix(expr, jsonPrefix) {
		path := strings.TrimSpace(strings.TrimPrefix(expr, jsonPrefix))
		if path == "" || strings.ContainsAny(path, " []()+-*/?:") {
			return "", false
		}
		parts := strings.Split(path, ".")
		keywords := make([]string, 0, len(parts))
		for _, part := range parts {
			key := n8nKebab(part, "field")
			if key == "" {
				return "", false
			}
			keywords = append(keywords, ":"+key)
		}
		if len(keywords) == 1 {
			return "(get input " + keywords[0] + ")", true
		}
		return "(get-in input [" + strings.Join(keywords, " ") + "])", true
	}
	return "", false
}

func translateN8NTemplateString(value string) (string, bool) {
	matches := n8nTemplateExprRe.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(matches)*2+1)
	cursor := 0
	for _, match := range matches {
		if match[0] > cursor {
			parts = append(parts, ednQuote(value[cursor:match[0]]))
		}
		inner := value[match[2]:match[3]]
		rendered, ok := translateN8NInnerExpression(inner)
		if !ok {
			return "", false
		}
		parts = append(parts, rendered)
		cursor = match[1]
	}
	if cursor < len(value) {
		parts = append(parts, ednQuote(value[cursor:]))
	}
	if len(parts) == 1 {
		return parts[0], true
	}
	return "(str " + strings.Join(parts, " ") + ")", true
}

func translateN8NTernaryExpression(expr string) (string, bool) {
	q := strings.Index(expr, "?")
	colon := strings.LastIndex(expr, ":")
	if q <= 0 || colon <= q {
		return "", false
	}
	condition, ok := translateN8NOperand(expr[:q])
	if !ok {
		return "", false
	}
	yes, ok := translateN8NStringLiteral(expr[q+1 : colon])
	if !ok {
		return "", false
	}
	no, ok := translateN8NStringLiteral(expr[colon+1:])
	if !ok {
		return "", false
	}
	return "(if " + condition + " " + yes + " " + no + ")", true
}

func translateN8NBinaryExpression(expr string) (string, bool) {
	for _, op := range []string{" + ", " - ", " * ", " / "} {
		if idx := strings.Index(expr, op); idx > 0 {
			left, leftOK := translateN8NOperand(expr[:idx])
			right, rightOK := translateN8NOperand(expr[idx+len(op):])
			if !leftOK || !rightOK {
				return "", false
			}
			return "(" + strings.TrimSpace(op) + " " + left + " " + right + ")", true
		}
	}
	return "", false
}

func translateN8NOperand(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "$json") || raw == "$now" {
		return translateN8NInnerExpression(raw)
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return raw, true
	}
	if s, ok := translateN8NStringLiteral(raw); ok {
		return s, true
	}
	return "", false
}

func translateN8NStringLiteral(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return "", false
	}
	if (raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'') {
		return ednQuote(raw[1 : len(raw)-1]), true
	}
	return "", false
}

func convertN8NCodeNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	source := stringParam(node.Parameters, "jsCode", "pythonCode", "code")
	commented := commentBlock(source, "  ;; ")
	code := fmt.Sprintf("(fn [input]\n  ;; TODO(n8n-import): port code node %s to Clojure.\n  ;; --- begin n8n code ---\n%s\n  ;; --- end n8n code ---\n  input)", ednQuote(firstNonEmpty(node.Name, stepID)), commented)
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, n8nInputExpr(upstreams, convertedByName))
	return binding, nil, nil, []string{fn}, []string{fmt.Sprintf("port Code node %q to Clojure", node.Name)}
}

func convertN8NMergeNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	fn := renderFunction(stepID, "(fn [input]\n  (merge (:left input) (:right input) input))")
	binding := renderFunctionStep(node, stepID, n8nInputExpr(upstreams, convertedByName))
	return binding, nil, nil, []string{fn}, nil
}

func convertN8NFallbackNode(node n8nNode, stepID string, upstreams []string, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	service := node.Type
	if idx := strings.LastIndex(service, "."); idx >= 0 {
		service = service[idx+1:]
	}
	code := fmt.Sprintf("(fn [input]\n  ;; TODO(n8n-import): Custom or unsupported n8n node %s (%s).\n  ;; TODO(n8n-import): Search the web for %s API docs and rebuild this node as HTTP if it has side effects.\n  input)", ednQuote(firstNonEmpty(node.Name, stepID)), node.Type, service)
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, n8nInputExpr(upstreams, convertedByName))
	return binding, nil, nil, []string{fn}, []string{fmt.Sprintf("implement unsupported node %q (%s)", node.Name, node.Type)}
}

func renderFunction(stepID, code string) string {
	return fmt.Sprintf(`{:id :%s-fn
  :language :clojure
  :code %s}`, stepID, ednQuote(code))
}

func renderFunctionStep(node n8nNode, stepID, input string) string {
	return fmt.Sprintf(`(flow/step :function :%s
           {:title %s
            :ref :%s-fn
            :input %s})`, stepID, ednQuote(firstNonEmpty(node.Name, stepID)), stepID, input)
}

func renderN8NFlowEDN(slug, name string, requires, templates, functions, webhooks, schedules []string, body string) string {
	var b strings.Builder
	b.WriteString("{:slug :" + slug + "\n")
	b.WriteString(" :name " + ednQuote(name) + "\n")
	b.WriteString(" :description \"Imported from n8n JSON. TODO(n8n-import): review unsupported nodes and expression translations.\"\n")
	b.WriteString(" :icon :workflow\n")
	b.WriteString(" :tags [:n8n-import]\n")
	b.WriteString(" :concurrency {:type :singleton :on-new-version :supersede}\n")
	b.WriteString(" :requires " + renderEDNVector(requires) + "\n")
	b.WriteString(" :templates " + renderEDNVector(templates) + "\n")
	b.WriteString(" :functions " + renderEDNVector(functions) + "\n")
	b.WriteString(" :invocations {:default {:inputs [{:name :payload :type :json :label \"Payload\" :required false}]}}\n")
	b.WriteString(" :interfaces {:manual [{:id :run :label \"Run\" :invocation :default :enabled true}]")
	if len(webhooks) > 0 {
		b.WriteString("\n              :webhook " + renderEDNVector(webhooks))
	}
	b.WriteString("}\n")
	if len(schedules) > 0 {
		b.WriteString(" :schedules " + renderEDNVector(schedules) + "\n")
	}
	b.WriteString(" :flow " + body + "}\n")
	return b.String()
}

func renderN8NFlowBody(converted []n8nConvertedNode, upstreams map[string][]string, convertedByName map[string]n8nConvertedNode) string {
	if len(converted) == 0 {
		return "(quote (let [input (flow/input)]\n    input))"
	}
	var b strings.Builder
	b.WriteString("(quote (let [input (flow/input)\n")
	for _, item := range converted {
		b.WriteString(fmt.Sprintf("        ;; n8n node: %s (%s)\n", ednQuote(item.Node.Name), item.Node.Type))
		lines := strings.Split(item.Binding, "\n")
		b.WriteString("        " + item.VarName + " " + lines[0] + "\n")
		for _, line := range lines[1:] {
			b.WriteString("                   " + line + "\n")
		}
	}
	b.WriteString("        ]\n")
	b.WriteString("    " + converted[len(converted)-1].VarName + "))")
	return b.String()
}

func validateGeneratedN8NFlowEDN(flowEDN string) (n8nFlowValidation, error) {
	if err := parenrepair.Check(flowEDN); err != nil {
		return n8nFlowValidation{}, fmt.Errorf("generated flow has invalid delimiters: %w", err)
	}

	var parsed any
	if err := edn.Unmarshal([]byte(flowEDN), &parsed); err != nil {
		return n8nFlowValidation{}, fmt.Errorf("generated flow is not readable EDN: %w", err)
	}
	m, ok := parsed.(map[any]any)
	if !ok {
		return n8nFlowValidation{}, errors.New("generated flow EDN must be a top-level map")
	}
	byKey := map[string]any{}
	for key, value := range m {
		if s, ok := ednKeyToString(key); ok {
			byKey[s] = value
		}
	}

	required := []string{"slug", "name", "concurrency", "invocations", "interfaces", "flow"}
	missing := make([]string, 0)
	for _, key := range required {
		if _, ok := byKey[key]; !ok {
			missing = append(missing, ":"+key)
		}
	}
	if len(missing) > 0 {
		return n8nFlowValidation{}, fmt.Errorf("generated flow missing required keys: %s", strings.Join(missing, ", "))
	}

	flowForm, ok := byKey["flow"].([]any)
	if !ok || len(flowForm) != 2 || fmt.Sprint(flowForm[0]) != "quote" {
		return n8nFlowValidation{}, errors.New("generated flow :flow must be an explicit (quote ...) form")
	}

	return n8nFlowValidation{
		BalancedDelimiters: true,
		EDNReadable:        true,
		RequiredKeys:       required,
	}, nil
}

func n8nEdges(wf n8nWorkflow) []n8nEdge {
	edges := make([]n8nEdge, 0)
	for source, groups := range wf.Connections {
		for _, outputs := range groups {
			for _, outputGroup := range outputs {
				for _, conn := range outputGroup {
					if strings.TrimSpace(conn.Node) != "" {
						edges = append(edges, n8nEdge{Source: source, Target: conn.Node})
					}
				}
			}
		}
	}
	return edges
}

func n8nTopologicalOrder(nodes []n8nNode, edges []n8nEdge) []n8nNode {
	byName := map[string]n8nNode{}
	indegree := map[string]int{}
	outgoing := map[string][]string{}
	for _, node := range nodes {
		if strings.TrimSpace(node.Name) == "" {
			continue
		}
		byName[node.Name] = node
		indegree[node.Name] = 0
	}
	for _, edge := range edges {
		if _, ok := indegree[edge.Source]; !ok {
			continue
		}
		if _, ok := indegree[edge.Target]; !ok {
			continue
		}
		outgoing[edge.Source] = append(outgoing[edge.Source], edge.Target)
		indegree[edge.Target]++
	}
	ready := make([]string, 0)
	for name, degree := range indegree {
		if degree == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)
	orderedNames := make([]string, 0, len(indegree))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		orderedNames = append(orderedNames, name)
		for _, target := range outgoing[name] {
			indegree[target]--
			if indegree[target] == 0 {
				ready = append(ready, target)
				sort.Strings(ready)
			}
		}
	}
	seen := map[string]bool{}
	out := make([]n8nNode, 0, len(nodes))
	for _, name := range orderedNames {
		seen[name] = true
		out = append(out, byName[name])
	}
	for _, node := range nodes {
		if !seen[node.Name] {
			out = append(out, node)
		}
	}
	return out
}

func n8nUpstreamsByTarget(edges []n8nEdge) map[string][]string {
	out := map[string][]string{}
	for _, edge := range edges {
		out[edge.Target] = append(out[edge.Target], edge.Source)
	}
	return out
}

func n8nInputExpr(upstreams []string, convertedByName map[string]n8nConvertedNode) string {
	available := make([]n8nConvertedNode, 0)
	for _, name := range upstreams {
		if item, ok := convertedByName[name]; ok {
			available = append(available, item)
		}
	}
	if len(available) == 0 {
		return "input"
	}
	if len(available) == 1 {
		return available[0].VarName
	}
	parts := make([]string, 0, len(available))
	for _, item := range available {
		parts = append(parts, ":"+item.StepID+" "+item.VarName)
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func n8nIsTrigger(node n8nNode) bool {
	typ := strings.ToLower(node.Type)
	if strings.Contains(typ, "respondtowebhook") || strings.Contains(typ, "webhookresponse") {
		return false
	}
	for _, marker := range []string{"manualtrigger", "webhook", "cron", "scheduletrigger", "interval"} {
		if strings.Contains(typ, marker) {
			return true
		}
	}
	return false
}

func n8nCredentialLabel(node n8nNode) string {
	for _, raw := range node.Credentials {
		switch value := raw.(type) {
		case map[string]any:
			if label := firstNonEmpty(fmt.Sprint(value["name"]), fmt.Sprint(value["id"])); label != "<nil>" {
				return label
			}
		case string:
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

func splitN8NURL(raw string) (string, string, map[string]string, bool) {
	if strings.Contains(raw, "{{") || !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "", "", nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", nil, false
	}
	query := map[string]string{}
	for key, values := range u.Query() {
		if len(values) > 0 {
			query[key] = values[0]
		}
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return u.Scheme + "://" + u.Host, path, query, true
}

func n8nKebab(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = n8nSlugInvalidRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if value == "" {
		value = fallback
	}
	if value == "" {
		value = "step"
	}
	if value[0] < 'a' || value[0] > 'z' {
		value = fallback + "-" + value
	}
	return value
}

func uniqueN8NID(base string, used map[string]bool) string {
	if base == "" {
		base = "step"
	}
	candidate := base
	for i := 2; used[candidate]; i++ {
		candidate = base + "-" + strconv.Itoa(i)
	}
	used[candidate] = true
	return candidate
}

func renderEDNVector(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	return "[" + strings.Join(items, "\n  ") + "]"
}

func ednQuote(value string) string {
	b, _ := json.Marshal(value)
	return string(b)
}

func ednValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "nil"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case string:
		return ednQuote(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, ednValue(item))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, ednQuote(key)+" "+ednValue(v[key]))
		}
		return "{" + strings.Join(parts, " ") + "}"
	default:
		return ednQuote(fmt.Sprint(v))
	}
}

func renderStringMap(values map[string]string) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, ednQuote(key)+" "+ednQuote(values[key]))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func renderAnyStringMap(values map[string]any) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, ednQuote(key)+" "+ednQuote(fmt.Sprint(values[key])))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func n8nParameterPairs(values map[string]any, keys ...string) map[string]string {
	out := map[string]string{}
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		collectN8NParameterPairs(out, raw)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectN8NParameterPairs(out map[string]string, raw any) {
	switch value := raw.(type) {
	case map[string]any:
		if params, ok := value["parameters"]; ok {
			collectN8NParameterPairs(out, params)
			return
		}
		name := strings.TrimSpace(fmt.Sprint(value["name"]))
		if name != "" && name != "<nil>" {
			out[name] = fmt.Sprint(value["value"])
			return
		}
		for key, item := range value {
			if strings.TrimSpace(key) != "" {
				out[key] = fmt.Sprint(item)
			}
		}
	case []any:
		for _, item := range value {
			collectN8NParameterPairs(out, item)
		}
	}
}

func mergeStringMaps(left, right map[string]string) map[string]string {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	out := map[string]string{}
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		out[key] = value
	}
	return out
}

func stringParam(values map[string]any, keys ...string) string {
	value, ok := firstParam(values, keys...)
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func intParam(values map[string]any, fallback int, keys ...string) int {
	value, ok := firstParam(values, keys...)
	if !ok || value == nil {
		return fallback
	}
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return n
		}
	}
	return fallback
}

func firstParam(values map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" && strings.TrimSpace(value) != "<nil>" {
			return value
		}
	}
	return ""
}

func commentBlock(value, prefix string) string {
	if strings.TrimSpace(value) == "" {
		return prefix + "<empty>"
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func shellQuotePath(path string) string {
	if path == "" {
		return "''"
	}
	if strings.ContainsAny(path, " \t\n'\"\\$") {
		return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
	}
	return path
}
