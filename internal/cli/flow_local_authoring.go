package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/spf13/cobra"
)

var localQualifiedStepIDRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*(?:\.[a-zA-Z][a-zA-Z0-9_-]*)*/[a-zA-Z][a-zA-Z0-9_-]{0,127}$`)

func localStepIDValid(stepID string) bool {
	return localQualifiedStepIDRe.MatchString(strings.TrimSpace(stepID))
}

func defaultLocalFlowPath(slug string) string {
	return filepath.Join("flows", strings.TrimSpace(slug)+".clj")
}

func resolveLocalFlowPath(slug, requested string) string {
	if path := strings.TrimSpace(requested); path != "" {
		return path
	}
	return defaultLocalFlowPath(slug)
}

func readLocalFlowSource(slug, requested string) (string, string, error) {
	path := resolveLocalFlowPath(slug, requested)
	b, err := readExplicitFile(path)
	if err != nil {
		return path, "", fmt.Errorf("read local flow %s: %w", path, err)
	}
	source := string(b)
	if strings.TrimSpace(source) == "" {
		return path, "", fmt.Errorf("local flow %s is empty", path)
	}
	if err := parenrepair.Check(source); err != nil {
		return path, "", fmt.Errorf("local flow source is not balanced: %w", err)
	}
	if _, err := extractTopLevelMapEntries(source); err != nil {
		return path, "", fmt.Errorf("local flow source is not a top-level map: %w", err)
	}
	return path, source, nil
}

func localTopLevelEntry(source, name string) (clojureMapEntry, bool, error) {
	entries, err := extractTopLevelMapEntries(source)
	if err != nil {
		return clojureMapEntry{}, false, err
	}
	for _, entry := range entries {
		if entry.KeyName == name {
			return entry, true, nil
		}
	}
	return clojureMapEntry{}, false, nil
}

func localFlowStepVector(source string, stepsEntry clojureMapEntry) ([]clojureFormSpan, error) {
	value := strings.TrimSpace(source[stepsEntry.ValueStart:stepsEntry.ValueEnd])
	if value == "nil" {
		return nil, nil
	}
	if !strings.HasPrefix(value, "[") {
		return nil, fmt.Errorf("top-level :steps must be a vector or nil")
	}
	spans, _, err := parseClojureVectorElements(source, stepsEntry.ValueStart)
	if err != nil {
		return nil, err
	}
	return spans, nil
}

func localStepIDFromMap(source string, span clojureFormSpan) (string, error) {
	entries, _, err := parseClojureMapEntries(source, span.Start)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.KeyName != "id" {
			continue
		}
		token := strings.TrimSpace(source[entry.ValueStart:entry.ValueEnd])
		token = strings.TrimPrefix(token, ":")
		if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") {
			var decoded string
			if err := json.Unmarshal([]byte(token), &decoded); err != nil {
				return "", err
			}
			token = decoded
		}
		return token, nil
	}
	return "", fmt.Errorf("step definition is missing :id")
}

func localStepLiteralID(stepLiteral string) (string, error) {
	if strings.TrimSpace(stepLiteral) == "" {
		return "", errors.New("step literal is empty")
	}
	entries, err := extractTopLevelMapEntries(stepLiteral)
	if err != nil {
		return "", fmt.Errorf("read step literal: %w", err)
	}
	for _, entry := range entries {
		if entry.KeyName != "id" {
			continue
		}
		token := strings.TrimSpace(stepLiteral[entry.ValueStart:entry.ValueEnd])
		token = strings.TrimPrefix(token, ":")
		if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") {
			var decoded string
			if err := json.Unmarshal([]byte(token), &decoded); err != nil {
				return "", fmt.Errorf("decode step id: %w", err)
			}
			token = decoded
		}
		return token, nil
	}
	return "", errors.New("step literal must contain :id")
}

func localStepSpansForID(source string, stepID string) (clojureMapEntry, []clojureFormSpan, int, error) {
	stepsEntry, found, err := localTopLevelEntry(source, "steps")
	if err != nil {
		return clojureMapEntry{}, nil, -1, err
	}
	if !found {
		return clojureMapEntry{}, nil, -1, nil
	}
	spans, err := localFlowStepVector(source, stepsEntry)
	if err != nil {
		return clojureMapEntry{}, nil, -1, err
	}
	for i, span := range spans {
		id, idErr := localStepIDFromMap(source, span)
		if idErr != nil {
			return clojureMapEntry{}, nil, -1, idErr
		}
		if id == stepID || strings.TrimPrefix(id, ":") == strings.TrimPrefix(stepID, ":") {
			return stepsEntry, spans, i, nil
		}
	}
	return stepsEntry, spans, -1, nil
}

func replaceLocalFlowValue(source string, entry clojureMapEntry, replacement string) string {
	return source[:entry.ValueStart] + replacement + source[entry.ValueEnd:]
}

func appendLocalStep(source, stepLiteral string) (string, error) {
	stepsEntry, found, err := localTopLevelEntry(source, "steps")
	if err != nil {
		return "", err
	}
	if !found {
		flowEntry, hasFlow, err := localTopLevelEntry(source, "flow")
		if err != nil {
			return "", err
		}
		mapStart, err := topLevelFlowMapStart(source)
		if err != nil || mapStart < 0 {
			return "", fmt.Errorf("locate top-level flow map: %w", err)
		}
		insertAt := len(source)
		if hasFlow {
			insertAt = flowEntry.KeyStart
		} else {
			mapEnd, endErr := readClojureFormEnd(source, mapStart)
			if endErr != nil {
				return "", endErr
			}
			insertAt = mapEnd - 1
		}
		prefix := "\n :steps [\n  " + stepLiteral + "\n ]\n "
		return source[:insertAt] + prefix + source[insertAt:], nil
	}

	value := strings.TrimSpace(source[stepsEntry.ValueStart:stepsEntry.ValueEnd])
	if value == "nil" {
		return replaceLocalFlowValue(source, stepsEntry, "[\n  "+stepLiteral+"\n ]"), nil
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return "", errors.New("top-level :steps must be a vector or nil")
	}
	insertAt := stepsEntry.ValueEnd - 1
	return source[:insertAt] + "\n  " + stepLiteral + "\n " + source[insertAt:], nil
}

func replaceLocalStep(source string, stepID, stepLiteral string) (string, error) {
	_, spans, index, err := localStepSpansForID(source, stepID)
	if err != nil {
		return "", err
	}
	if index < 0 {
		return "", fmt.Errorf("step %q not found", stepID)
	}
	span := spans[index]
	return source[:span.Start] + stepLiteral + source[span.End:], nil
}

func removeLocalStep(source string, stepID string) (string, error) {
	_, spans, index, err := localStepSpansForID(source, stepID)
	if err != nil {
		return "", err
	}
	if index < 0 {
		return "", fmt.Errorf("step %q not found", stepID)
	}
	span := spans[index]
	return source[:span.Start] + source[span.End:], nil
}

func composeLocalFlowBody(source, body string) (string, error) {
	entry, found, err := localTopLevelEntry(source, "flow")
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("flow source is missing top-level :flow")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "", errors.New("flow body is empty")
	}
	if !strings.HasPrefix(body, "'") && !strings.HasPrefix(body, "(quote") {
		body = "'" + body
	}
	return replaceLocalFlowValue(source, entry, body), nil
}

func readParamsJSON(paramsJSON, paramsFile string) (map[string]any, error) {
	raw := strings.TrimSpace(paramsJSON)
	if path := strings.TrimSpace(paramsFile); path != "" {
		b, err := readExplicitFile(path)
		if err != nil {
			return nil, fmt.Errorf("read --params-file: %w", err)
		}
		raw = strings.TrimSpace(string(b))
	}
	if raw == "" {
		return map[string]any{}, nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("invalid --params JSON: %w", err)
	}
	params, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("--params must be a JSON object")
	}
	return params, nil
}

func pushLocalFlowLiteral(cmd *cobra.Command, app *App, sourcePath, source string) (map[string]any, int, error) {
	if err := requireAPI(app); err != nil {
		return nil, 0, err
	}
	expanded, err := expandFlowSourceIncludes(sourcePath, source)
	if err != nil {
		return nil, 0, err
	}
	out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.put_draft", map[string]any{
		"flowLiteral": expanded,
	})
	return out, status, err
}

func runLocalFlowStep(cmd *cobra.Command, app *App, slug, sourcePath, source, stepID string, params map[string]any, idempotencyKey, profileID string) (map[string]any, int, error) {
	if err := requireAPI(app); err != nil {
		return nil, 0, err
	}
	expanded, err := expandFlowSourceIncludes(sourcePath, source)
	if err != nil {
		return nil, 0, err
	}
	payload := map[string]any{
		"flowSlug":    strings.TrimSpace(slug),
		"stepId":      strings.TrimSpace(stepID),
		"params":      params,
		"flowLiteral": expanded,
	}
	if key := strings.TrimSpace(idempotencyKey); key != "" {
		payload["idempotencyKey"] = key
	}
	if profile := strings.TrimSpace(profileID); profile != "" {
		payload["profileId"] = profile
	}
	return apiClient(app).DoCommand(cmd.Context(), "steps.run", payload)
}

func writeLocalAuthoringResult(cmd *cobra.Command, app *App, path string, pushed map[string]any, pushStatus int, extra map[string]any) error {
	result := map[string]any{
		"saved": true,
		"path":  path,
	}
	for key, value := range extra {
		result[key] = value
	}
	if pushed != nil {
		result["remote"] = pushed
		result["remoteStatus"] = pushStatus
	}
	return writeData(cmd, app, nil, result)
}

func newFlowsStepsLocalCreateCmd(app *App) *cobra.Command {
	var flowFile, stepFile, stepType, title, description string
	var push, run bool
	var idempotencyKey, paramsJSON, paramsFile, profileID string
	cmd := &cobra.Command{
		Use:   "create <flow-slug> <step-id>",
		Short: "Add a packaged step to the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, stepID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !isAPIValidFlowSlug(slug) {
				return writeErr(cmd, fmt.Errorf("invalid flow slug %q", slug))
			}
			if !localStepIDValid(stepID) {
				return writeErr(cmd, fmt.Errorf("invalid step id %q (use a qualified id such as tools/fetch-order)", stepID))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if _, _, index, findErr := localStepSpansForID(source, stepID); findErr != nil {
				return writeErr(cmd, findErr)
			} else if index >= 0 {
				return writeErr(cmd, fmt.Errorf("step %q already exists; use update", stepID))
			}
			var stepLiteral string
			if strings.TrimSpace(stepFile) != "" {
				b, readErr := readExplicitFile(stepFile)
				if readErr != nil {
					return writeErr(cmd, fmt.Errorf("read --step-file: %w", readErr))
				}
				stepLiteral = strings.TrimSpace(string(b))
				literalID, idErr := localStepLiteralID(stepLiteral)
				if idErr != nil {
					return writeErr(cmd, idErr)
				}
				if literalID != stepID {
					return writeErr(cmd, fmt.Errorf("step literal id %q does not match %q", literalID, stepID))
				}
			} else {
				if strings.TrimSpace(stepType) == "" {
					return writeErr(cmd, errors.New("missing --type or --step-file"))
				}
				if strings.TrimSpace(title) == "" {
					title = stepID
				}
				stepLiteral = fmt.Sprintf("{:id :%s\n  :type :%s\n  :description %q\n  :input-schema [:map]}", stepID, strings.TrimPrefix(strings.TrimSpace(stepType), ":"), descriptionOrDefault(description, title))
			}
			updated, err := appendLocalStep(source, stepLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			var remote map[string]any
			var remoteStatus int
			if push {
				remote, remoteStatus, err = pushLocalFlowLiteral(cmd, app, path, updated)
				if err != nil {
					return writeErr(cmd, err)
				}
				if remoteStatus >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, remoteStatus)
				}
			}
			extra := map[string]any{"flowSlug": slug, "stepId": stepID}
			if run {
				params, paramsErr := readParamsJSON(paramsJSON, paramsFile)
				if paramsErr != nil {
					return writeErr(cmd, paramsErr)
				}
				runOut, runStatus, runErr := runLocalFlowStep(cmd, app, slug, path, updated, stepID, params, idempotencyKey, profileID)
				if runErr != nil {
					return writeErr(cmd, runErr)
				}
				extra["run"] = runOut
				extra["runStatus"] = runStatus
			}
			return writeLocalAuthoringResult(cmd, app, path, remote, remoteStatus, extra)
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&stepFile, "step-file", "", "Read the complete packaged step map from this file")
	cmd.Flags().StringVar(&stepType, "type", "", "Underlying step type when scaffolding (for example: function, http, llm)")
	cmd.Flags().StringVar(&title, "title", "", "Scaffold title/description")
	cmd.Flags().StringVar(&description, "description", "", "Scaffold description")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	cmd.Flags().BoolVar(&run, "run", false, "Run the edited packaged step against the server after saving")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step input JSON object for --run")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "Read step input JSON from this file for --run")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for --run retries")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional installation/profile id for --run")
	return cmd
}

func descriptionOrDefault(description, title string) string {
	if value := strings.TrimSpace(description); value != "" {
		return value
	}
	return strings.TrimSpace(title)
}

func newFlowsStepsLocalUpdateCmd(app *App) *cobra.Command {
	var flowFile, stepFile string
	var push, run bool
	var idempotencyKey, paramsJSON, paramsFile, profileID string
	cmd := &cobra.Command{
		Use:   "update <flow-slug> <step-id>",
		Short: "Replace a packaged step in the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, stepID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !localStepIDValid(stepID) {
				return writeErr(cmd, fmt.Errorf("invalid step id %q", stepID))
			}
			if strings.TrimSpace(stepFile) == "" {
				return writeErr(cmd, errors.New("update requires --step-file; edit the flow body with compose"))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			b, err := readExplicitFile(stepFile)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("read --step-file: %w", err))
			}
			stepLiteral := strings.TrimSpace(string(b))
			literalID, err := localStepLiteralID(stepLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if literalID != stepID {
				return writeErr(cmd, fmt.Errorf("step literal id %q does not match %q", literalID, stepID))
			}
			updated, err := replaceLocalStep(source, stepID, stepLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			var remote map[string]any
			var remoteStatus int
			if push {
				remote, remoteStatus, err = pushLocalFlowLiteral(cmd, app, path, updated)
				if err != nil {
					return writeErr(cmd, err)
				}
				if remoteStatus >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, remoteStatus)
				}
			}
			extra := map[string]any{"flowSlug": slug, "stepId": stepID}
			if run {
				params, paramsErr := readParamsJSON(paramsJSON, paramsFile)
				if paramsErr != nil {
					return writeErr(cmd, paramsErr)
				}
				runOut, runStatus, runErr := runLocalFlowStep(cmd, app, slug, path, updated, stepID, params, idempotencyKey, profileID)
				if runErr != nil {
					return writeErr(cmd, runErr)
				}
				extra["run"] = runOut
				extra["runStatus"] = runStatus
			}
			return writeLocalAuthoringResult(cmd, app, path, remote, remoteStatus, extra)
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&stepFile, "step-file", "", "Read the replacement packaged step map from this file")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	cmd.Flags().BoolVar(&run, "run", false, "Run the edited packaged step against the server after saving")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step input JSON object for --run")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "Read step input JSON from this file for --run")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for --run retries")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional installation/profile id for --run")
	return cmd
}

func newFlowsStepsLocalRemoveCmd(app *App) *cobra.Command {
	var flowFile string
	var push bool
	cmd := &cobra.Command{
		Use:   "remove <flow-slug> <step-id>",
		Short: "Remove a packaged step from the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, stepID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			updated, err := removeLocalStep(source, stepID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			if push {
				remote, status, pushErr := pushLocalFlowLiteral(cmd, app, path, updated)
				if pushErr != nil {
					return writeErr(cmd, pushErr)
				}
				if status >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, status)
				}
				return writeLocalAuthoringResult(cmd, app, path, remote, status, map[string]any{"flowSlug": slug, "stepId": stepID})
			}
			return writeLocalAuthoringResult(cmd, app, path, nil, 0, map[string]any{"flowSlug": slug, "stepId": stepID})
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	return cmd
}

func newFlowsStepsLocalRunCmd(app *App) *cobra.Command {
	var flowFile, paramsJSON, paramsFile, idempotencyKey, profileID string
	cmd := &cobra.Command{
		Use:   "run <flow-slug> <step-id>",
		Short: "Run a packaged step from the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, stepID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !localStepIDValid(stepID) {
				return writeErr(cmd, fmt.Errorf("invalid step id %q", stepID))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			params, err := readParamsJSON(paramsJSON, paramsFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			out, status, err := runLocalFlowStep(cmd, app, slug, path, source, stepID, params, idempotencyKey, profileID)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step input JSON object")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "Read step input JSON from this file")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for retrying side-effectful runs")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional installation/profile id")
	return cmd
}

func newFlowsComposeCmd(app *App) *cobra.Command {
	var flowFile, body, bodyFile string
	var push bool
	cmd := &cobra.Command{
		Use:   "compose <flow-slug>",
		Short: "Replace only the :flow literal in local source",
		Long: strings.TrimSpace(`
Compose the flow body while preserving the surrounding definition, including
the existing top-level :steps, interfaces, schedules, functions, and providers.
The body may call any packaged steps already declared in :steps.

Examples:
  breyta flows compose order-sync --flow-file ./flows/order-sync.clj --body-file ./flows/order-sync.body.clj
  breyta flows compose order-sync --body '(let [order (flow/step :tools/fetch-order :fetch {})] order)'
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := strings.TrimSpace(args[0])
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			bodyText := strings.TrimSpace(body)
			if strings.TrimSpace(bodyFile) != "" {
				b, readErr := readExplicitFile(bodyFile)
				if readErr != nil {
					return writeErr(cmd, fmt.Errorf("read --body-file: %w", readErr))
				}
				bodyText = strings.TrimSpace(string(b))
			}
			if bodyText == "" {
				return writeErr(cmd, errors.New("missing --body or --body-file"))
			}
			updated, err := composeLocalFlowBody(source, bodyText)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			if push {
				remote, status, pushErr := pushLocalFlowLiteral(cmd, app, path, updated)
				if pushErr != nil {
					return writeErr(cmd, pushErr)
				}
				if status >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, status)
				}
				return writeLocalAuthoringResult(cmd, app, path, remote, status, map[string]any{"flowSlug": slug})
			}
			return writeLocalAuthoringResult(cmd, app, path, nil, 0, map[string]any{"flowSlug": slug})
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&body, "body", "", "Quoted or unquoted Clojure flow body")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Read the Clojure flow body from this file")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	return cmd
}

func newFlowsInitCmd(app *App) *cobra.Command {
	var name, description, outPath string
	var force, push bool
	cmd := &cobra.Command{
		Use:   "init <flow-slug>",
		Short: "Create a local canonical flow source file",
		Long: strings.TrimSpace(`
Create the local source of truth for a flow. Initialization is local by default;
pass --push when you explicitly want the same source sent to the workspace.
The generated definition includes a no-input manual Run interface and an empty
schedules vector so later schedule edits stay in the same source file.

Examples:
  breyta flows init order-sync --name "Order sync"
  breyta flows init order-sync --out ./flows/order-sync.clj --push
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := strings.TrimSpace(args[0])
			if !isAPIValidFlowSlug(slug) {
				return writeErr(cmd, fmt.Errorf("invalid flow slug %q", slug))
			}
			path := resolveLocalFlowPath(slug, outPath)
			if _, err := os.Stat(path); err == nil && !force {
				return writeErr(cmd, fmt.Errorf("local flow %s already exists; pass --force to replace it", path))
			} else if err != nil && !os.IsNotExist(err) {
				return writeErr(cmd, err)
			}
			flowName := strings.TrimSpace(name)
			if flowName == "" {
				flowName = slug
			}
			literal := fmt.Sprintf(`{:slug :%s
 :name %q
 :description %q
 :tags ["draft"]
 :concurrency {:type :singleton :on-new-version :supersede}
 :requires nil
 :templates nil
 :functions nil
 :steps []
 :invocations {:default {:label "Run" :inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :triggers [{:type :manual :label "Run" :enabled true :config {}}]
 :flow '(let [input (flow/input)] input)}
`, slug, flowName, strings.TrimSpace(description))
			if err := atomicWriteFile(path, []byte(literal), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			if push {
				remote, status, pushErr := pushLocalFlowLiteral(cmd, app, path, literal)
				if pushErr != nil {
					return writeErr(cmd, pushErr)
				}
				if status >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, status)
				}
				return writeLocalAuthoringResult(cmd, app, path, remote, status, map[string]any{"flowSlug": slug})
			}
			return writeLocalAuthoringResult(cmd, app, path, nil, 0, map[string]any{"flowSlug": slug})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Flow display name (default: slug)")
	cmd.Flags().StringVar(&description, "description", "", "Flow description")
	cmd.Flags().StringVar(&outPath, "out", "", "Output path (default: flows/<flow-slug>.clj)")
	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing local source file")
	cmd.Flags().BoolVar(&push, "push", false, "Push the generated source after saving")
	return cmd
}
