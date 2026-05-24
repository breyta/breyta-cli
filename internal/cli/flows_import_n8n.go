package cli

import (
	"context"
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
	Source       string
	Target       string
	SourceOutput int
	TargetInput  int
}

type n8nConvertedNode struct {
	Node      n8nNode
	StepID    string
	VarName   string
	InputExpr string
	Binding   string
	Todos     []string
}

type n8nBranchGuard struct {
	SourceName   string
	SourceOutput int
}

type n8nInputPlan struct {
	Names         []string
	UpstreamNames []string
	Expr          string
	FunctionRefs  map[string]string
	TemplateRefs  map[string]string
}

type n8nImportResult struct {
	Slug       string
	Name       string
	OutputPath string
	Todos      []string
	EDN        string
	Validation n8nFlowValidation
}

type n8nServerValidationResult struct {
	FlowSlug       string         `json:"flowSlug"`
	PushedDraft    bool           `json:"pushedDraft"`
	Valid          bool           `json:"valid"`
	ValidateSource string         `json:"validateSource"`
	Push           map[string]any `json:"push,omitempty"`
	Validate       map[string]any `json:"validate,omitempty"`
}

type n8nFlowValidation struct {
	BalancedDelimiters bool     `json:"balancedDelimiters"`
	EDNReadable        bool     `json:"ednReadable"`
	RequiredKeys       []string `json:"requiredKeys"`
}

var n8nSlugInvalidRe = regexp.MustCompile(`[^a-z0-9-]+`)
var n8nTemplateExprRe = regexp.MustCompile(`\{\{([^{}]+)\}\}`)
var n8nNodeRefExprRe = regexp.MustCompile(`\$node\[['"]([^'"]+)['"]\]`)
var n8nAllowedInvocationInputTypes = map[string]bool{
	"string":   true,
	"text":     true,
	"number":   true,
	"email":    true,
	"password": true,
	"textarea": true,
	"boolean":  true,
	"checkbox": true,
	"select":   true,
	"date":     true,
	"time":     true,
	"datetime": true,
	"file":     true,
	"blob":     true,
	"blob-ref": true,
	"resource": true,
	"secret":   true,
}

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
	var serverValidate bool
	var deployKey string

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
			data := map[string]any{
				"slug":         result.Slug,
				"name":         result.Name,
				"path":         result.OutputPath,
				"todoCount":    len(result.Todos),
				"todos":        result.Todos,
				"validation":   result.Validation,
				"pushCommand":  fmt.Sprintf("breyta flows push --file %s", shellQuotePath(result.OutputPath)),
				"checkCommand": fmt.Sprintf("breyta flows configure check %s", result.Slug),
				"runCommand":   fmt.Sprintf("breyta flows run %s --input '{}' --wait", result.Slug),
			}
			if serverValidate {
				serverResult, err := validateImportedN8NFlowOnServer(app, result, deployKey)
				if err != nil {
					return writeFailure(cmd, app, "n8n_server_validation_failed", err, "Check --api/--workspace/--token and inspect the generated flow file.", map[string]any{
						"path": result.OutputPath,
						"slug": result.Slug,
					})
				}
				data["serverValidation"] = serverResult
			}
			return writeData(cmd, app, nil, data)
		},
	}

	cmd.Flags().StringVar(&slug, "slug", "", "Breyta flow slug (defaults to normalized workflow name)")
	cmd.Flags().StringVar(&outPath, "out", "", "Output flow file (defaults to ./tmp/flows/<slug>.clj)")
	cmd.Flags().BoolVar(&serverValidate, "server-validate", false, "Push generated flow to the configured Breyta API draft and run flows.validate")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key for guarded flows when using --server-validate (default: BREYTA_FLOW_DEPLOY_KEY)")
	return cmd
}

func validateImportedN8NFlowOnServer(app *App, result *n8nImportResult, deployKey string) (*n8nServerValidationResult, error) {
	if !isAPIMode(app) {
		return nil, errors.New("--server-validate requires --api/BREYTA_API_URL")
	}
	if err := requireAPI(app); err != nil {
		return nil, err
	}
	payload := map[string]any{"flowLiteral": result.EDN}
	resolvedDeployKey := strings.TrimSpace(deployKey)
	if resolvedDeployKey == "" {
		resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
	}
	if resolvedDeployKey != "" {
		payload["deploy-key"] = resolvedDeployKey
	}
	pushOut, pushStatus, err := runAPICommand(app, "flows.put_draft", payload)
	if err != nil {
		return nil, err
	}
	if pushStatus >= 400 || !isOK(pushOut) {
		return nil, apiEnvelopeError("flows.put_draft", pushStatus, pushOut)
	}
	flowSlug := result.Slug
	if data, ok := pushOut["data"].(map[string]any); ok {
		if slug, _ := data["flowSlug"].(string); strings.TrimSpace(slug) != "" {
			flowSlug = strings.TrimSpace(slug)
		}
	}
	validateOut, validateStatus, err := apiClient(app).DoCommand(context.Background(), "flows.validate", map[string]any{
		"flowSlug": flowSlug,
		"source":   "draft",
	})
	if err != nil {
		return nil, err
	}
	if validateStatus >= 400 || !isOK(validateOut) {
		return nil, apiEnvelopeError("flows.validate", validateStatus, validateOut)
	}
	valid := false
	if data, ok := validateOut["data"].(map[string]any); ok {
		if value, ok := data["valid"].(bool); ok {
			valid = value
		}
	}
	return &n8nServerValidationResult{
		FlowSlug:       flowSlug,
		PushedDraft:    true,
		Valid:          valid,
		ValidateSource: "draft",
		Push:           pushOut,
		Validate:       validateOut,
	}, nil
}

func apiEnvelopeError(command string, status int, envelope map[string]any) error {
	if errObj, ok := envelope["error"].(map[string]any); ok {
		if message, _ := errObj["message"].(string); strings.TrimSpace(message) != "" {
			return fmt.Errorf("%s failed (status=%d): %s", command, status, message)
		}
	}
	return fmt.Errorf("%s failed (status=%d)", command, status)
}

func importN8NWorkflowFile(path, slug, outPath string) (*n8nImportResult, error) {
	b, err := readExplicitFile(path)
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
			if strings.TrimSpace(node.Name) != "" {
				convertedByName[node.Name] = n8nConvertedNode{Node: node, StepID: n8nKebab(node.Name, "input"), VarName: "input", InputExpr: "input"}
			}
			continue
		}
		stepID := uniqueN8NID(n8nKebab(node.Name, "step"), usedIDs)
		varName := uniqueN8NID(strings.ReplaceAll(stepID, "-", "_"), usedVars)
		inputPlan := n8nBuildInputPlan(upstreams[node.Name], n8nReferencedNodeNames(node.Parameters), convertedByName)
		binding, nodeRequires, nodeTemplates, nodeFunctions, nodeTodos := convertN8NNode(node, stepID, inputPlan, convertedByName)
		requires = appendUniqueStrings(requires, nodeRequires)
		templates = append(templates, nodeTemplates...)
		functions = append(functions, nodeFunctions...)
		for _, todo := range nodeTodos {
			todos = append(todos, stepID+": "+todo)
		}
		item := n8nConvertedNode{Node: node, StepID: stepID, VarName: varName, InputExpr: inputPlan.Expr, Binding: binding, Todos: nodeTodos}
		converted = append(converted, item)
		if strings.TrimSpace(node.Name) != "" {
			convertedByName[node.Name] = item
		}
	}

	name := firstNonEmpty(wf.Name, "Imported n8n Flow")
	body := renderN8NFlowBody(converted, edges, upstreams, convertedByName, n8nBranchGuards(edges, convertedByName))
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

func convertN8NNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	typ := strings.ToLower(node.Type)
	switch {
	case strings.Contains(typ, "httprequest"):
		return convertN8NHTTPNode(node, stepID, inputPlan, convertedByName)
	case strings.HasSuffix(typ, ".if") || strings.Contains(typ, ".switch"):
		return convertN8NBranchNode(node, stepID, inputPlan, convertedByName)
	case strings.Contains(typ, "webhookresponse") || strings.Contains(typ, "respondtowebhook"):
		return convertN8NWebhookResponseNode(node, stepID, inputPlan, convertedByName)
	case strings.HasSuffix(typ, ".wait") || strings.Contains(typ, ".wait"):
		return convertN8NWaitNode(node, stepID, inputPlan, convertedByName)
	case strings.HasSuffix(typ, ".set") || strings.Contains(typ, "set"):
		return convertN8NSetNode(node, stepID, inputPlan, convertedByName)
	case strings.Contains(typ, "itemlists"):
		return convertN8NItemListsNode(node, stepID, inputPlan, convertedByName)
	case strings.Contains(typ, "htmlextract"):
		return convertN8NHTMLExtractNode(node, stepID, inputPlan, convertedByName)
	case strings.Contains(typ, "code") || strings.Contains(typ, "function"):
		return convertN8NCodeNode(node, stepID, inputPlan, convertedByName)
	case strings.Contains(typ, "merge"):
		return convertN8NMergeNode(node, stepID, inputPlan, convertedByName)
	default:
		return convertN8NFallbackNode(node, stepID, inputPlan, convertedByName)
	}
}

func convertN8NHTTPNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	method := strings.ToLower(firstNonEmpty(stringParam(node.Parameters, "method", "requestMethod"), "GET"))
	rawURL := stringParam(node.Parameters, "url", "endpoint")
	baseURL, path, query, ok := splitN8NURL(rawURL)
	if !ok {
		baseURL, path, query, ok = splitN8NURLTemplate(rawURL)
	}
	todos := make([]string, 0)
	if !ok {
		baseURL = "https://example.com"
		path = firstNonEmpty(rawURL, "/")
		todos = append(todos, fmt.Sprintf("fill base URL/path for HTTP node %q", node.Name))
	}
	templateRefs := n8nHandlebarsRefs(convertedByName, inputPlan)
	if rendered, ok := translateN8NHandlebarsTemplate(path, templateRefs); ok {
		path = rendered
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
	for key, value := range query {
		if rendered, ok := translateN8NHandlebarsTemplate(value, templateRefs); ok {
			query[key] = rendered
		}
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

	binding := fmt.Sprintf(`(flow/step :http :%s
           {:title %s
            :connection :%s
            :template :%s-request
            :persist {:type :blob}
            :data %s})`, stepID, ednQuote(firstNonEmpty(node.Name, stepID)), slot, stepID, inputPlan.Expr)
	return binding, []string{require}, []string{template}, nil, todos
}

func convertN8NBranchNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	condition, todos := n8nBranchCondition(node)
	code := fmt.Sprintf("(fn [input]\n  (assoc input :branch %s))", condition)
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, inputPlan.Expr)
	return binding, nil, nil, []string{fn}, todos
}

func n8nBranchCondition(node n8nNode) (string, []string) {
	typ := strings.ToLower(node.Type)
	if strings.Contains(typ, ".switch") || strings.Contains(typ, "switch") {
		return "0", []string{fmt.Sprintf("translate Switch node %q rules to a branch output index", node.Name)}
	}
	if expr, ok := n8nIFCondition(node.Parameters); ok {
		return expr, nil
	}
	return "false", []string{fmt.Sprintf("translate IF node %q conditions", node.Name)}
}

func n8nIFCondition(params map[string]any) (string, bool) {
	conditions, ok := params["conditions"].(map[string]any)
	if !ok {
		return "", false
	}
	if rawItems, ok := conditions["conditions"].([]any); ok {
		return n8nIFConditionItems(rawItems, stringParam(conditions, "combinator"))
	}
	parts := make([]string, 0)
	for groupName, rawGroup := range conditions {
		group, ok := rawGroup.([]any)
		if !ok {
			continue
		}
		for _, rawItem := range group {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			part, ok := n8nConditionItem(groupName, item)
			if !ok {
				return "", false
			}
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	if len(parts) == 1 {
		return parts[0], true
	}
	return "(and " + strings.Join(parts, " ") + ")", true
}

func n8nIFConditionItems(rawItems []any, combinator string) (string, bool) {
	parts := make([]string, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		part, ok := n8nConditionItem("", item)
		if !ok {
			return "", false
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "", false
	}
	if len(parts) == 1 {
		return parts[0], true
	}
	if strings.EqualFold(strings.TrimSpace(combinator), "or") {
		return "(or " + strings.Join(parts, " ") + ")", true
	}
	return "(and " + strings.Join(parts, " ") + ")", true
}

func n8nConditionItem(groupName string, item map[string]any) (string, bool) {
	leftRaw, ok := firstParam(item, "value1", "leftValue")
	if !ok {
		return "", false
	}
	left, ok := n8nConditionValue(leftRaw)
	if !ok {
		return "", false
	}
	op := n8nConditionOperation(item)
	rightRaw, hasRightRaw := firstParam(item, "value2", "rightValue")
	right, hasRight := "", false
	if hasRightRaw {
		right, hasRight = n8nConditionValue(rightRaw)
	}
	switch op {
	case "isempty", "empty":
		return "(empty? (or " + left + " \"\"))", true
	case "isnotempty", "notempty":
		return "(not (empty? (or " + left + " \"\")))", true
	case "equal", "equals":
		if !hasRight {
			return "", false
		}
		return "(= " + left + " " + right + ")", true
	case "notequal", "notequals", "not_equal":
		if !hasRight {
			return "", false
		}
		return "(not= " + left + " " + right + ")", true
	case "contains":
		if !hasRight {
			return "", false
		}
		return "(.contains (str " + left + ") (str " + right + "))", true
	case "true":
		return left, true
	case "false":
		return "(not " + left + ")", true
	default:
		if strings.EqualFold(groupName, "boolean") && !hasRight {
			return left, true
		}
		return "", false
	}
}

func n8nConditionOperation(item map[string]any) string {
	if operator, ok := item["operator"].(map[string]any); ok {
		if op, _ := operator["operation"].(string); op != "" {
			return n8nNormalizeConditionOperation(op)
		}
	}
	if op, _ := item["operation"].(string); op != "" {
		return n8nNormalizeConditionOperation(op)
	}
	return "equal"
}

func n8nNormalizeConditionOperation(op string) string {
	op = strings.ToLower(strings.TrimSpace(op))
	op = strings.ReplaceAll(op, "_", "")
	op = strings.ReplaceAll(op, "-", "")
	op = strings.ReplaceAll(op, " ", "")
	return op
}

func n8nConditionValue(raw any) (string, bool) {
	switch v := raw.(type) {
	case nil:
		return "nil", true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case string:
		value := strings.TrimSpace(v)
		value = strings.TrimPrefix(value, "=")
		if expr, ok := translateSimpleN8NExpression(value); ok {
			return expr, true
		}
		if expr, ok := translateN8NTemplateString(value); ok {
			return expr, true
		}
		if strings.Contains(value, "{{") {
			return "", false
		}
		return ednQuote(v), true
	default:
		return ednValue(v), true
	}
}

func convertN8NWebhookResponseNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	statusCode := intParam(node.Parameters, 200, "responseCode", "statusCode")
	body, todo := renderN8NWebhookResponseBody(node, inputPlan.FunctionRefs)
	code := fmt.Sprintf("(fn [input]\n  {:status %d\n   :headers {}\n   :body %s})", statusCode, body)
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, inputPlan.Expr)
	if todo == "" {
		return binding, nil, nil, []string{fn}, nil
	}
	return binding, nil, nil, []string{fn}, []string{todo}
}

func convertN8NWaitNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
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
	binding := fmt.Sprintf(`(flow/step :wait :%s
           {:title %s
            :timeout %d
            :on-timeout :continue
            :default-value %s})`, stepID, ednQuote(firstNonEmpty(node.Name, stepID)), timeout, inputPlan.Expr)
	return binding, nil, nil, nil, []string{fmt.Sprintf("verify Wait node %q timing semantics", node.Name)}
}

func renderN8NWebhookResponseBody(node n8nNode, nodeRefs map[string]string) (string, string) {
	value := firstNonEmpty(stringParam(node.Parameters, "responseBody", "body"), stringParam(node.Parameters, "responseData"))
	if strings.TrimSpace(value) == "" {
		return "input", ""
	}
	value = normalizeN8NTemplateLiteral(value)
	if expr, ok := translateSimpleN8NExpressionWithRefs(value, nodeRefs); ok {
		return expr, ""
	}
	if expr, ok := translateN8NTemplateStringWithRefs(value, nodeRefs); ok {
		return expr, ""
	}
	if strings.Contains(value, "{{") {
		return ednQuote(value), fmt.Sprintf("translate n8n webhook response expression for node %q", node.Name)
	}
	return ednQuote(value), ""
}

func convertN8NSetNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
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
				if rendered, todo, ok := renderN8NSetValue(name, value, inputPlan.FunctionRefs); ok {
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
			if n8nSetKeepOnlySet(node.Parameters) {
				code = "(fn [input]\n  {\n    " + strings.Join(assoc, "\n    ") + "})"
			} else {
				code = "(fn [input]\n  (assoc input\n    " + strings.Join(assoc, "\n    ") + "))"
			}
		} else if len(todos) == 0 {
			todos = []string{fmt.Sprintf("port Set node %q parameters", node.Name)}
		}
	}
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, inputPlan.Expr)
	return binding, nil, nil, []string{fn}, todos
}

func n8nSetKeepOnlySet(params map[string]any) bool {
	if boolParam(params, "keepOnlySet") {
		return true
	}
	if options, ok := params["options"].(map[string]any); ok {
		return boolParam(options, "keepOnlySet")
	}
	return false
}

func convertN8NItemListsNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	field := n8nKebab(firstNonEmpty(stringParam(node.Parameters, "fieldToSplitOut", "field"), "items"), "items")
	code := fmt.Sprintf(`(fn [input]
  (let [value (or (get input :%s) (get input %s))
        items (cond
                (nil? value) []
                (sequential? value) (vec value)
                :else [value])]
    (assoc input :items items :%s-items items)))`, field, ednQuote(field), field)
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, inputPlan.Expr)
	return binding, nil, nil, []string{fn}, nil
}

func convertN8NHTMLExtractNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	extracts, todos := n8nHTMLExtractFields(node)
	if len(extracts) == 0 {
		return convertN8NFallbackNode(node, stepID, inputPlan, convertedByName)
	}
	assoc := make([]string, 0, len(extracts))
	for _, extract := range extracts {
		assoc = append(assoc, fmt.Sprintf(":%s (extract-id html %s)", extract.Key, ednQuote(extract.ID)))
	}
	code := `(fn [input]
  (let [html (str (or (:body input) (get input "body") (:html input) (get input "html") ""))
        clean-text (fn [s]
                     (when s
                       (-> s
                           (clojure.string/replace #"<[^>]+>" "")
                           (clojure.string/replace #"&nbsp;" " ")
                           (clojure.string/replace #"\s+" " ")
                           (clojure.string/trim))))
        extract-id (fn [html id]
                     (let [pattern (re-pattern (str "(?is)<[^>]*id=[\"']" (java.util.regex.Pattern/quote id) "[\"'][^>]*>(.*?)</[^>]+>"))]
                       (clean-text (second (re-find pattern html)))))]
    (assoc input
      ` + strings.Join(assoc, "\n      ") + `)))`
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, inputPlan.Expr)
	return binding, nil, nil, []string{fn}, todos
}

type n8nHTMLExtractField struct {
	Key string
	ID  string
}

func n8nHTMLExtractFields(node n8nNode) ([]n8nHTMLExtractField, []string) {
	rawValues, ok := node.Parameters["extractionValues"].(map[string]any)
	if !ok {
		return nil, []string{fmt.Sprintf("port HTML Extract node %q parameters", node.Name)}
	}
	values, ok := rawValues["values"].([]any)
	if !ok {
		return nil, []string{fmt.Sprintf("port HTML Extract node %q parameters", node.Name)}
	}
	fields := make([]n8nHTMLExtractField, 0)
	todos := make([]string, 0)
	for _, raw := range values {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key := n8nKebab(n8nSplitCamel(firstNonEmpty(fmt.Sprint(item["key"]), "value")), "value")
		selector := strings.TrimSpace(fmt.Sprint(item["cssSelector"]))
		if strings.HasPrefix(selector, "#") && len(selector) > 1 && !strings.ContainsAny(selector[1:], " .#[:>+~") {
			fields = append(fields, n8nHTMLExtractField{Key: key, ID: selector[1:]})
			continue
		}
		todos = append(todos, fmt.Sprintf("translate HTML Extract selector %q for field %q", selector, key))
	}
	return fields, todos
}

func n8nSplitCamel(value string) string {
	var b strings.Builder
	for i, r := range value {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func renderN8NSetValue(fieldName string, value any, nodeRefs map[string]string) (string, string, bool) {
	s, ok := value.(string)
	if !ok || !strings.Contains(s, "{{") {
		return "", "", false
	}
	s = normalizeN8NExpressionString(normalizeN8NTemplateLiteral(s))
	if expr, ok := translateSimpleN8NExpressionWithRefs(s, nodeRefs); ok {
		return expr, "", true
	}
	if expr, ok := translateN8NTemplateStringWithRefs(s, nodeRefs); ok {
		return expr, "", true
	}
	return ednQuote(s), fmt.Sprintf("translate n8n expression for Set field %q", fieldName), true
}

func normalizeN8NExpressionString(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "={{") {
		return strings.TrimPrefix(trimmed, "=")
	}
	return value
}

func normalizeN8NTemplateLiteral(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "=") && strings.Contains(trimmed, "{{") {
		return strings.TrimPrefix(trimmed, "=")
	}
	return value
}

func translateSimpleN8NExpression(value string) (string, bool) {
	return translateSimpleN8NExpressionWithRefs(value, nil)
}

func translateSimpleN8NExpressionWithRefs(value string, nodeRefs map[string]string) (string, bool) {
	value = normalizeN8NExpressionString(value)
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{{") || !strings.HasSuffix(trimmed, "}}") {
		return "", false
	}
	expr := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"))
	return translateN8NInnerExpressionWithRefs(expr, nodeRefs)
}

func translateN8NInnerExpressionWithRefs(expr string, nodeRefs map[string]string) (string, bool) {
	expr = strings.TrimSpace(expr)
	jsonRoot := n8nJSONRootRef(nodeRefs)
	switch expr {
	case "$json":
		return jsonRoot, true
	case "$now":
		return "(flow/now-ms)", true
	}
	if rendered, ok := translateN8NTernaryExpressionWithRefs(expr, nodeRefs); ok {
		return rendered, true
	}
	if rendered, ok := translateN8NBinaryExpressionWithRefs(expr, nodeRefs); ok {
		return rendered, true
	}
	if rendered, ok := translateN8NIncrementExpressionWithRefs(expr, nodeRefs); ok {
		return rendered, true
	}
	if rendered, ok := translateN8NPathExpressionWithRefs(expr, nodeRefs); ok {
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
			return "(get " + jsonRoot + " " + keywords[0] + ")", true
		}
		return "(get-in " + jsonRoot + " [" + strings.Join(keywords, " ") + "])", true
	}
	return "", false
}

func translateN8NPathExpressionWithRefs(expr string, nodeRefs map[string]string) (string, bool) {
	root, path, ok := parseN8NPathExpression(expr)
	if !ok {
		return "", false
	}
	switch root {
	case "$json":
		return renderN8NGetPath(n8nJSONRootRef(nodeRefs), path), true
	default:
		const nodePrefix = "$node:"
		if strings.HasPrefix(root, nodePrefix) {
			nodeName := strings.TrimPrefix(root, nodePrefix)
			if ref, ok := nodeRefs[nodeName]; ok {
				return renderN8NGetPath(ref, path), true
			}
		}
		return "", false
	}
}

func n8nJSONRootRef(nodeRefs map[string]string) string {
	if nodeRefs != nil {
		if ref := strings.TrimSpace(nodeRefs["$json"]); ref != "" {
			return ref
		}
	}
	return "input"
}

func translateN8NIncrementExpressionWithRefs(expr string, nodeRefs map[string]string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasSuffix(expr, "++") {
		return "", false
	}
	target := strings.TrimSpace(strings.TrimSuffix(expr, "++"))
	value, ok := translateN8NInnerExpressionWithRefs(target, nodeRefs)
	if !ok {
		return "", false
	}
	return "(inc (or " + value + " 0))", true
}

func parseN8NPathExpression(expr string) (string, []string, bool) {
	expr = strings.TrimSpace(expr)
	root := ""
	switch {
	case strings.HasPrefix(expr, "$json"):
		root = "$json"
		expr = strings.TrimPrefix(expr, "$json")
	case strings.HasPrefix(expr, "$node["):
		nodeName, rest, ok := parseN8NNodeRoot(expr)
		if !ok {
			return "", nil, false
		}
		root = "$node:" + nodeName
		expr = rest
	default:
		return "", nil, false
	}
	expr = strings.TrimPrefix(expr, ".json")
	parts := make([]string, 0)
	for expr != "" {
		switch {
		case strings.HasPrefix(expr, "."):
			expr = strings.TrimPrefix(expr, ".")
			next := strings.IndexAny(expr, ".[")
			if next < 0 {
				next = len(expr)
			}
			part := strings.TrimSpace(expr[:next])
			if part == "" {
				return "", nil, false
			}
			parts = append(parts, part)
			expr = expr[next:]
		case strings.HasPrefix(expr, "[\""), strings.HasPrefix(expr, "['"):
			quote := expr[1]
			end := strings.IndexByte(expr[2:], quote)
			if end < 0 {
				return "", nil, false
			}
			part := expr[2 : 2+end]
			if part == "" {
				return "", nil, false
			}
			expr = expr[2+end+1:]
			if !strings.HasPrefix(expr, "]") {
				return "", nil, false
			}
			expr = strings.TrimPrefix(expr, "]")
			parts = append(parts, part)
		default:
			return "", nil, false
		}
	}
	if len(parts) == 0 {
		return root, nil, true
	}
	return root, parts, true
}

func parseN8NNodeRoot(expr string) (string, string, bool) {
	if !strings.HasPrefix(expr, "$node[") {
		return "", "", false
	}
	rest := strings.TrimPrefix(expr, "$node[")
	if len(rest) < 3 || (rest[0] != '"' && rest[0] != '\'') {
		return "", "", false
	}
	quote := rest[0]
	end := strings.IndexByte(rest[1:], quote)
	if end < 0 {
		return "", "", false
	}
	nodeName := rest[1 : 1+end]
	rest = rest[1+end+1:]
	if !strings.HasPrefix(rest, "]") {
		return "", "", false
	}
	return nodeName, strings.TrimPrefix(rest, "]"), true
}

func renderN8NGetPath(root string, parts []string) string {
	if len(parts) == 0 {
		return root
	}
	keywords := make([]string, 0, len(parts))
	for _, part := range parts {
		keywords = append(keywords, ":"+n8nKebab(part, "field"))
	}
	if len(keywords) == 1 {
		return "(get " + root + " " + keywords[0] + ")"
	}
	return "(get-in " + root + " [" + strings.Join(keywords, " ") + "])"
}

func translateN8NTemplateString(value string) (string, bool) {
	return translateN8NTemplateStringWithRefs(value, nil)
}

func translateN8NTemplateStringWithRefs(value string, nodeRefs map[string]string) (string, bool) {
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
		rendered, ok := translateN8NInnerExpressionWithRefs(inner, nodeRefs)
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

func translateN8NHandlebarsTemplate(value string, nodeRefs map[string]string) (string, bool) {
	matches := n8nTemplateExprRe.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value, false
	}
	var b strings.Builder
	cursor := 0
	changed := false
	for _, match := range matches {
		b.WriteString(value[cursor:match[0]])
		inner := value[match[2]:match[3]]
		path, ok := translateN8NHandlebarsPath(inner, nodeRefs)
		if !ok {
			b.WriteString(value[match[0]:match[1]])
		} else {
			b.WriteString("{{")
			b.WriteString(path)
			b.WriteString("}}")
			changed = true
		}
		cursor = match[1]
	}
	b.WriteString(value[cursor:])
	out := b.String()
	if strings.HasPrefix(out, "={{") && strings.HasSuffix(out, "}}") {
		out = strings.TrimPrefix(out, "=")
	}
	return out, changed
}

func translateN8NHandlebarsPath(expr string, nodeRefs map[string]string) (string, bool) {
	expr = strings.TrimSpace(strings.TrimSuffix(expr, "++"))
	root, parts, ok := parseN8NPathExpression(expr)
	if !ok || len(parts) == 0 {
		return "", false
	}
	out := make([]string, 0, len(parts)+1)
	switch {
	case root == "$json":
		if ref := strings.TrimSpace(nodeRefs["$json"]); ref != "" {
			out = append(out, ref)
		}
	case strings.HasPrefix(root, "$node:"):
		nodeName := strings.TrimPrefix(root, "$node:")
		ref, ok := nodeRefs[nodeName]
		if !ok {
			return "", false
		}
		out = append(out, ref)
	default:
		return "", false
	}
	for _, part := range parts {
		out = append(out, n8nKebab(part, "field"))
	}
	return strings.Join(out, "."), true
}

func translateN8NTernaryExpressionWithRefs(expr string, nodeRefs map[string]string) (string, bool) {
	q := strings.Index(expr, "?")
	colon := strings.LastIndex(expr, ":")
	if q <= 0 || colon <= q {
		return "", false
	}
	condition, ok := translateN8NOperandWithRefs(expr[:q], nodeRefs)
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

func translateN8NBinaryExpressionWithRefs(expr string, nodeRefs map[string]string) (string, bool) {
	for _, op := range []string{" + ", " - ", " * ", " / "} {
		if idx := strings.Index(expr, op); idx > 0 {
			left, leftOK := translateN8NOperandWithRefs(expr[:idx], nodeRefs)
			right, rightOK := translateN8NOperandWithRefs(expr[idx+len(op):], nodeRefs)
			if !leftOK || !rightOK {
				return "", false
			}
			return "(" + strings.TrimSpace(op) + " " + left + " " + right + ")", true
		}
	}
	return "", false
}

func translateN8NOperandWithRefs(raw string, nodeRefs map[string]string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "$json") || raw == "$now" {
		return translateN8NInnerExpressionWithRefs(raw, nodeRefs)
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

func convertN8NCodeNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	source := stringParam(node.Parameters, "jsCode", "pythonCode", "code")
	commented := commentBlock(source, "  ;; ")
	code := fmt.Sprintf("(fn [input]\n  ;; TODO(n8n-import): port code node %s to Clojure.\n  ;; --- begin n8n code ---\n%s\n  ;; --- end n8n code ---\n  input)", ednQuote(firstNonEmpty(node.Name, stepID)), commented)
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, inputPlan.Expr)
	return binding, nil, nil, []string{fn}, []string{fmt.Sprintf("port Code node %q to Clojure", node.Name)}
}

func convertN8NMergeNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	parts := make([]string, 0, len(inputPlan.UpstreamNames))
	for _, name := range inputPlan.UpstreamNames {
		item, ok := convertedByName[name]
		if !ok {
			continue
		}
		parts = append(parts, "(get input :"+item.StepID+")")
	}
	code := "(fn [input]\n  input)"
	if len(parts) > 0 {
		code = "(fn [input]\n  (let [inputs (remove :n8n-import/skipped [" + strings.Join(parts, " ") + "])]\n    (if (seq inputs)\n      (apply merge inputs)\n      (assoc input :n8n-import/skipped true))))"
	}
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, inputPlan.Expr)
	return binding, nil, nil, []string{fn}, nil
}

func convertN8NFallbackNode(node n8nNode, stepID string, inputPlan n8nInputPlan, convertedByName map[string]n8nConvertedNode) (string, []string, []string, []string, []string) {
	service := node.Type
	if idx := strings.LastIndex(service, "."); idx >= 0 {
		service = service[idx+1:]
	}
	code := fmt.Sprintf("(fn [input]\n  ;; TODO(n8n-import): Custom or unsupported n8n node %s (%s).\n  ;; TODO(n8n-import): Search the web for %s API docs and rebuild this node as HTTP if it has side effects.\n  input)", ednQuote(firstNonEmpty(node.Name, stepID)), node.Type, service)
	fn := renderFunction(stepID, code)
	binding := renderFunctionStep(node, stepID, inputPlan.Expr)
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
	b.WriteString(" :invocations {:default {:inputs []}}\n")
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

func renderN8NFlowBody(converted []n8nConvertedNode, edges []n8nEdge, upstreams map[string][]string, convertedByName map[string]n8nConvertedNode, branchGuards map[string][]n8nBranchGuard) string {
	if len(converted) == 0 {
		return "(quote (let [input (flow/input)]\n    input))"
	}
	returnExpr := n8nFlowReturnExpr(n8nTerminalConvertedNodes(converted, edges, convertedByName))
	var b strings.Builder
	b.WriteString("(quote (let [input (flow/input)\n")
	for _, item := range converted {
		b.WriteString(fmt.Sprintf("        ;; n8n node: %s (%s)\n", ednQuote(item.Node.Name), item.Node.Type))
		binding := renderN8NGuardedBinding(item, branchGuards[item.Node.Name], upstreams, convertedByName)
		lines := strings.Split(binding, "\n")
		b.WriteString("        " + item.VarName + " " + lines[0] + "\n")
		for _, line := range lines[1:] {
			b.WriteString("                   " + line + "\n")
		}
	}
	b.WriteString("        ]\n")
	b.WriteString("    " + returnExpr + "))")
	return b.String()
}

func n8nTerminalConvertedNodes(converted []n8nConvertedNode, edges []n8nEdge, convertedByName map[string]n8nConvertedNode) []n8nConvertedNode {
	hasConvertedOutgoing := map[string]bool{}
	for _, edge := range edges {
		if _, ok := convertedByName[edge.Source]; !ok {
			continue
		}
		if _, ok := convertedByName[edge.Target]; !ok {
			continue
		}
		hasConvertedOutgoing[edge.Source] = true
	}
	terminals := make([]n8nConvertedNode, 0)
	for _, item := range converted {
		if !hasConvertedOutgoing[item.Node.Name] {
			terminals = append(terminals, item)
		}
	}
	if len(terminals) == 0 {
		return converted[len(converted)-1:]
	}
	return terminals
}

func n8nFlowReturnExpr(terminals []n8nConvertedNode) string {
	if len(terminals) == 0 {
		return "input"
	}
	if len(terminals) == 1 {
		return terminals[0].VarName
	}
	parts := make([]string, 0, len(terminals)+1)
	for i := len(terminals) - 1; i >= 0; i-- {
		item := terminals[i]
		parts = append(parts, "(when-not (:n8n-import/skipped "+item.VarName+") "+item.VarName+")")
	}
	parts = append(parts, terminals[len(terminals)-1].VarName)
	return "(or " + strings.Join(parts, "\n        ") + ")"
}

func renderN8NGuardedBinding(item n8nConvertedNode, guards []n8nBranchGuard, upstreams map[string][]string, convertedByName map[string]n8nConvertedNode) string {
	input := item.InputExpr
	if input == "" {
		input = n8nInputExpr(upstreams[item.Node.Name], upstreams[item.Node.Name], convertedByName)
	}
	if len(guards) > 0 {
		conditions := make([]string, 0, len(guards))
		for _, guard := range guards {
			source, ok := convertedByName[guard.SourceName]
			if !ok {
				continue
			}
			conditions = append(conditions, n8nBranchGuardCondition(source, guard.SourceOutput))
		}
		if len(conditions) > 0 {
			condition := conditions[0]
			if len(conditions) > 1 {
				condition = "(or " + strings.Join(conditions, " ") + ")"
			}
			return fmt.Sprintf(`(if %s
  %s
  (assoc %s :n8n-import/skipped true))`, condition, item.Binding, input)
		}
	}
	if n8nIsBranchNode(item.Node) {
		return item.Binding
	}
	return fmt.Sprintf(`(if (:n8n-import/skipped %s)
  %s
  %s)`, input, input, item.Binding)
}

func n8nBranchGuardCondition(source n8nConvertedNode, outputIndex int) string {
	sourceVar := source.VarName
	if strings.Contains(strings.ToLower(source.Node.Type), "switch") {
		return "(= (:branch " + sourceVar + ") " + strconv.Itoa(outputIndex) + ")"
	}
	if outputIndex == 0 {
		return "(true? (:branch " + sourceVar + "))"
	}
	if outputIndex == 1 {
		return "(false? (:branch " + sourceVar + "))"
	}
	return "(= (:branch " + sourceVar + ") " + strconv.Itoa(outputIndex) + ")"
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
	if err := validateN8NInvocationInputTypes(byKey["invocations"]); err != nil {
		return n8nFlowValidation{}, err
	}

	return n8nFlowValidation{
		BalancedDelimiters: true,
		EDNReadable:        true,
		RequiredKeys:       required,
	}, nil
}

func validateN8NInvocationInputTypes(value any) error {
	var invalid []string
	walkEDN(value, func(m map[any]any) {
		inputs, ok := ednMapGet(m, "inputs")
		if !ok {
			return
		}
		items, ok := inputs.([]any)
		if !ok {
			return
		}
		for _, item := range items {
			input, ok := item.(map[any]any)
			if !ok {
				continue
			}
			typ, ok := ednMapGet(input, "type")
			if !ok {
				invalid = append(invalid, "<missing>")
				continue
			}
			typeName, ok := ednKeyToString(typ)
			if !ok || !n8nAllowedInvocationInputTypes[typeName] {
				invalid = append(invalid, fmt.Sprint(typ))
			}
		}
	})
	if len(invalid) > 0 {
		sort.Strings(invalid)
		return fmt.Errorf("generated flow has unsupported invocation input type(s): %s", strings.Join(invalid, ", "))
	}
	return nil
}

func ednMapGet(m map[any]any, key string) (any, bool) {
	for rawKey, value := range m {
		if s, ok := ednKeyToString(rawKey); ok && s == key {
			return value, true
		}
	}
	return nil, false
}

func walkEDN(value any, visitMap func(map[any]any)) {
	switch typed := value.(type) {
	case map[any]any:
		visitMap(typed)
		for _, child := range typed {
			walkEDN(child, visitMap)
		}
	case []any:
		for _, child := range typed {
			walkEDN(child, visitMap)
		}
	}
}

func n8nEdges(wf n8nWorkflow) []n8nEdge {
	edges := make([]n8nEdge, 0)
	for source, groups := range wf.Connections {
		for _, outputs := range groups {
			for outputIndex, outputGroup := range outputs {
				for _, conn := range outputGroup {
					if strings.TrimSpace(conn.Node) != "" {
						edges = append(edges, n8nEdge{Source: source, Target: conn.Node, SourceOutput: outputIndex, TargetInput: conn.Index})
					}
				}
			}
		}
	}
	return edges
}

func n8nBranchGuards(edges []n8nEdge, convertedByName map[string]n8nConvertedNode) map[string][]n8nBranchGuard {
	out := map[string][]n8nBranchGuard{}
	for _, edge := range edges {
		source, ok := convertedByName[edge.Source]
		if !ok || !n8nIsBranchNode(source.Node) {
			continue
		}
		out[edge.Target] = append(out[edge.Target], n8nBranchGuard{SourceName: edge.Source, SourceOutput: edge.SourceOutput})
	}
	return out
}

func n8nIsBranchNode(node n8nNode) bool {
	typ := strings.ToLower(node.Type)
	return strings.HasSuffix(typ, ".if") || strings.Contains(typ, ".switch")
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
	byTarget := map[string][]n8nEdge{}
	for _, edge := range edges {
		byTarget[edge.Target] = append(byTarget[edge.Target], edge)
	}
	out := map[string][]string{}
	for target, targetEdges := range byTarget {
		sort.SliceStable(targetEdges, func(i, j int) bool {
			if targetEdges[i].TargetInput != targetEdges[j].TargetInput {
				return targetEdges[i].TargetInput < targetEdges[j].TargetInput
			}
			return targetEdges[i].Source < targetEdges[j].Source
		})
		for _, edge := range targetEdges {
			out[target] = append(out[target], edge.Source)
		}
	}
	return out
}

func n8nReferencedNodeNames(value any) []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	var walk func(any)
	walk = func(v any) {
		switch typed := v.(type) {
		case map[string]any:
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case string:
			for _, match := range n8nNodeRefExprRe.FindAllStringSubmatch(typed, -1) {
				if len(match) < 2 || seen[match[1]] {
					continue
				}
				seen[match[1]] = true
				out = append(out, match[1])
			}
		}
	}
	walk(value)
	return out
}

func n8nHandlebarsRefs(convertedByName map[string]n8nConvertedNode, inputPlan n8nInputPlan) map[string]string {
	refs := make(map[string]string, len(convertedByName))
	for name, item := range convertedByName {
		refs[name] = item.StepID
	}
	for key, value := range inputPlan.TemplateRefs {
		refs[key] = value
	}
	return refs
}

func n8nBuildInputPlan(upstreams, nodeRefs []string, convertedByName map[string]n8nConvertedNode) n8nInputPlan {
	names := make([]string, 0, len(upstreams)+len(nodeRefs))
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, upstreams...), nodeRefs...) {
		if seen[name] {
			continue
		}
		if _, ok := convertedByName[name]; !ok {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return n8nInputPlan{
		Names:         names,
		UpstreamNames: n8nConvertedInputNames(upstreams, convertedByName),
		Expr:          n8nInputExpr(upstreams, names, convertedByName),
		FunctionRefs:  n8nFunctionInputRefs(upstreams, names, convertedByName),
		TemplateRefs:  n8nTemplateInputRefs(upstreams, names),
	}
}

func n8nConvertedInputNames(names []string, convertedByName map[string]n8nConvertedNode) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := convertedByName[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

func n8nInputExpr(upstreams, inputNames []string, convertedByName map[string]n8nConvertedNode) string {
	available := make([]n8nConvertedNode, 0)
	for _, name := range inputNames {
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
	if current, ok := n8nCurrentInput(upstreams, convertedByName); ok {
		parts = append(parts, ":input "+current.VarName)
	}
	for _, item := range available {
		parts = append(parts, ":"+item.StepID+" "+item.VarName)
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func n8nFunctionInputRefs(upstreams, inputNames []string, convertedByName map[string]n8nConvertedNode) map[string]string {
	switch len(inputNames) {
	case 0:
		return nil
	case 1:
		return map[string]string{inputNames[0]: "input"}
	default:
		refs := map[string]string{}
		if _, ok := n8nCurrentInput(upstreams, convertedByName); ok {
			refs["$json"] = "(get input :input)"
		}
		for _, name := range inputNames {
			item, ok := convertedByName[name]
			if !ok {
				continue
			}
			refs[name] = "(get input :" + item.StepID + ")"
		}
		return refs
	}
}

func n8nTemplateInputRefs(upstreams, inputNames []string) map[string]string {
	if len(inputNames) <= 1 {
		return nil
	}
	if len(upstreams) == 0 {
		return nil
	}
	return map[string]string{"$json": "input"}
}

func n8nCurrentInput(upstreams []string, convertedByName map[string]n8nConvertedNode) (n8nConvertedNode, bool) {
	for _, name := range upstreams {
		if item, ok := convertedByName[name]; ok {
			return item, true
		}
	}
	return n8nConvertedNode{}, false
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

func splitN8NURLTemplate(raw string) (string, string, map[string]string, bool) {
	value := strings.TrimPrefix(strings.TrimSpace(raw), "=")
	if !strings.Contains(value, "{{") {
		return "", "", nil, false
	}
	probe := n8nTemplateExprRe.ReplaceAllString(value, "x")
	if !strings.HasPrefix(probe, "http://") && !strings.HasPrefix(probe, "https://") {
		return "", "", nil, false
	}
	u, err := url.Parse(probe)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", nil, false
	}
	baseURL := u.Scheme + "://" + u.Host
	if !strings.HasPrefix(value, baseURL) {
		return "", "", nil, false
	}
	rest := strings.TrimPrefix(value, baseURL)
	pathPart, queryPart, _ := strings.Cut(rest, "?")
	query := map[string]string{}
	parsedQuery, err := url.ParseQuery(queryPart)
	if err != nil {
		return "", "", nil, false
	}
	for key, values := range parsedQuery {
		if len(values) > 0 {
			query[key] = values[0]
		}
	}
	path := pathPart
	if path == "" {
		path = "/"
	}
	return baseURL, path, query, true
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

func boolParam(values map[string]any, keys ...string) bool {
	value, ok := firstParam(values, keys...)
	if !ok || value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1":
			return true
		}
	}
	return false
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
