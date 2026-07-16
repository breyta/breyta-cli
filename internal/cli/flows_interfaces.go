package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newFlowsInterfacesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "interfaces",
		Short: "Inspect and author flow interfaces backed by invocations",
		Long: strings.TrimSpace(`
Inspect or author callable surfaces declared under :interfaces.

List, show, call, and curl read interface metadata from the API. Upsert and remove
modify the draft flow definition. These commands do not construct runtime HTTP or
MCP routes locally.
`),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return writeErr(cmd, fmt.Errorf("unknown subcommand %q; did you mean `breyta flows interfaces list %s`?", args[0], args[0]))
			}
			if len(args) > 1 {
				return writeErr(cmd, fmt.Errorf("unknown subcommand %q; did you mean `breyta flows interfaces list %s`?", strings.Join(args, " "), args[0]))
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFlowsInterfacesUpsertCmd(app))
	cmd.AddCommand(newFlowsInterfacesValidateCmd(app))
	cmd.AddCommand(newFlowsInterfacesRemoveCmd(app))
	cmd.AddCommand(newFlowsInterfacesListCmd(app))
	cmd.AddCommand(newFlowsInterfacesShowCmd(app))
	cmd.AddCommand(newFlowsInterfacesCallCmd(app))
	cmd.AddCommand(newFlowsInterfacesCurlCmd(app))
	return cmd
}

func requireFlowsAuthoringAPI(cmd *cobra.Command, app *App, command string) error {
	if !isAPIMode(app) {
		return writeErr(cmd, fmt.Errorf("%s requires API mode", command))
	}
	return requireAPI(app)
}

func applyLiteralOrFile(cmd *cobra.Command, payload map[string]any, key string, literal string, file string, literalFlag string, fileFlag string) error {
	literal = strings.TrimSpace(literal)
	file = strings.TrimSpace(file)
	literalSet := cmd.Flags().Changed(strings.TrimPrefix(literalFlag, "--"))
	fileSet := cmd.Flags().Changed(strings.TrimPrefix(fileFlag, "--"))
	if literalSet && fileSet {
		return fmt.Errorf("%s cannot be combined with %s", literalFlag, fileFlag)
	}
	if literalSet {
		if literal == "" {
			return fmt.Errorf("%s cannot be empty", literalFlag)
		}
		payload[key] = literal
		return nil
	}
	if fileSet {
		if file == "" {
			return fmt.Errorf("%s cannot be empty", fileFlag)
		}
		b, err := readExplicitFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", fileFlag, err)
		}
		fromFile := strings.TrimSpace(string(b))
		if fromFile == "" {
			return fmt.Errorf("%s must contain a non-empty literal", fileFlag)
		}
		payload[key] = fromFile
	}
	return nil
}

func interfaceAuthValue(auth string, authJSON string, authFile string, authSet bool, authJSONSet bool, authFileSet bool) (any, error) {
	auth = strings.TrimSpace(auth)
	authJSON = strings.TrimSpace(authJSON)
	authFile = strings.TrimSpace(authFile)
	if authSet && auth == "" {
		return nil, errors.New("--auth cannot be empty")
	}
	if authJSONSet && authJSON == "" {
		return nil, errors.New("--auth-json cannot be empty")
	}
	if authFileSet && authFile == "" {
		return nil, errors.New("--auth-file cannot be empty")
	}
	if authSet && (authJSONSet || authFileSet) {
		return nil, errors.New("--auth cannot be combined with --auth-json or --auth-file")
	}
	if authJSONSet && authFileSet {
		return nil, errors.New("--auth-json cannot be combined with --auth-file")
	}
	if auth != "" {
		return auth, nil
	}
	if authFile != "" {
		b, err := readExplicitFile(authFile)
		if err != nil {
			return nil, fmt.Errorf("read --auth-file: %w", err)
		}
		authJSON = strings.TrimSpace(string(b))
		if authJSON == "" {
			return nil, errors.New("--auth-file must contain a JSON object")
		}
	}
	if authJSON == "" {
		return nil, nil
	}
	var structured map[string]any
	if err := json.Unmarshal([]byte(authJSON), &structured); err != nil {
		return nil, fmt.Errorf("invalid structured auth JSON: %w", err)
	}
	if structured == nil {
		return nil, errors.New("structured auth must be a JSON object")
	}
	return structured, nil
}

func authoringClearFields(cmd *cobra.Command, raw []string, allowed map[string][]string) ([]string, error) {
	clearFields := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, value := range raw {
		field := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
		conflictingFlags, ok := allowed[field]
		if !ok {
			return nil, fmt.Errorf("unsupported --clear field %q", value)
		}
		for _, flag := range conflictingFlags {
			if cmd.Flags().Changed(flag) {
				return nil, fmt.Errorf("--clear %s cannot be combined with --%s", field, flag)
			}
		}
		if !seen[field] {
			seen[field] = true
			clearFields = append(clearFields, field)
		}
	}
	return clearFields, nil
}

func newFlowsInterfacesUpsertCmd(app *App) *cobra.Command {
	var source string
	var kind string
	var toolName string
	var invocation string
	var label string
	var description string
	var enabled bool
	var inputSchemaFile string
	var inputSchemaLiteral string
	var outputSchemaFile string
	var outputSchemaLiteral string
	var responseFile string
	var responseLiteral string
	var path string
	var method string
	var eventName string
	var auth string
	var authJSON string
	var authFile string
	var trustedMetadata bool
	var clearFields []string
	var validate bool

	cmd := &cobra.Command{
		Use:   "upsert <flow-slug> <interface-id>",
		Short: "Upsert a draft flow interface and backing invocation",
		Long: strings.TrimSpace(`
Upsert a draft interface and generated invocation contract. Input schema files
are EDN literals using the invocation input shape.

Examples:
  breyta flows interfaces upsert my-flow run --kind manual --input-schema ./inputs.edn
  breyta flows interfaces upsert my-flow run --kind manual --validate
  breyta flows interfaces upsert my-flow events --kind webhook --event-name orders.updated
  breyta flows interfaces upsert my-flow summarize --kind mcp --tool-name summarize --input-schema ./inputs.edn --output-schema ./output.edn
`),
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAuthoringAPI(cmd, app, "flows interfaces upsert")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			clearFields, err := authoringClearFields(cmd, clearFields, map[string][]string{
				"label":            {"label"},
				"description":      {"description"},
				"invocation":       {"invocation"},
				"auth":             {"auth", "auth-json", "auth-file"},
				"input-schema":     {"input-schema", "input-schema-literal"},
				"output-schema":    {"output-schema", "output-schema-literal"},
				"response":         {"response", "response-literal"},
				"path":             {"path"},
				"method":           {"method"},
				"event-name":       {"event-name"},
				"trusted-metadata": {"trusted-metadata"},
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			authValue, err := interfaceAuthValue(
				auth,
				authJSON,
				authFile,
				cmd.Flags().Changed("auth"),
				cmd.Flags().Changed("auth-json"),
				cmd.Flags().Changed("auth-file"),
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			resolvedKind := strings.TrimSpace(kind)
			if resolvedKind == "" {
				return writeErr(cmd, errors.New("missing --kind (manual|api|http|webhook|mcp)"))
			}
			resolvedToolName := strings.TrimSpace(toolName)
			payload := pruneEmptyStrings(map[string]any{
				"flowSlug":    strings.TrimSpace(args[0]),
				"interfaceId": strings.TrimSpace(args[1]),
				"source":      strings.TrimSpace(source),
				"kind":        resolvedKind,
				"toolName":    resolvedToolName,
				"invocation":  strings.TrimSpace(invocation),
				"label":       strings.TrimSpace(label),
				"description": strings.TrimSpace(description),
				"path":        strings.TrimSpace(path),
				"method":      strings.TrimSpace(method),
				"eventName":   strings.TrimSpace(eventName),
			})
			if authValue != nil {
				payload["auth"] = authValue
			}
			if len(clearFields) > 0 {
				payload["clearFields"] = clearFields
			}
			if cmd.Flags().Changed("enabled") {
				payload["enabled"] = enabled
			}
			if cmd.Flags().Changed("trusted-metadata") {
				payload["trustedMetadata"] = trustedMetadata
			}
			if err := applyLiteralOrFile(cmd, payload, "inputSchema", inputSchemaLiteral, inputSchemaFile, "--input-schema-literal", "--input-schema"); err != nil {
				return writeErr(cmd, err)
			}
			if err := applyLiteralOrFile(cmd, payload, "outputSchema", outputSchemaLiteral, outputSchemaFile, "--output-schema-literal", "--output-schema"); err != nil {
				return writeErr(cmd, err)
			}
			if err := applyLiteralOrFile(cmd, payload, "responseLiteral", responseLiteral, responseFile, "--response-literal", "--response"); err != nil {
				return writeErr(cmd, err)
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.interfaces.upsert", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if validate && status < 400 && isOK(out) {
				validationPayload := pruneEmptyStrings(map[string]any{
					"flowSlug":    strings.TrimSpace(args[0]),
					"interfaceId": strings.TrimSpace(args[1]),
					"source":      strings.TrimSpace(source),
					"kind":        resolvedKind,
				})
				validationOut, validationStatus, validationErr := apiClient(app).DoCommand(cmd.Context(), "flows.interfaces.validate", validationPayload)
				if validationErr != nil {
					return writeErr(cmd, validationErr)
				}
				if validationStatus >= 400 || !isOK(validationOut) {
					return writeAPIResult(cmd, app, validationOut, validationStatus)
				}
				data := mapStringAny(out["data"])
				if data == nil {
					data = map[string]any{}
				}
				data["validation"] = mapStringAny(validationOut["data"])
				out["data"] = data
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	cmd.Flags().StringVar(&kind, "kind", "", "Interface kind (manual|api|http|webhook|mcp); required")
	cmd.Flags().StringVar(&toolName, "tool-name", "", "MCP tool name; defaults to interface id for --kind mcp")
	cmd.Flags().StringVar(&invocation, "invocation", "", "Backing invocation id; defaults to interface id/tool name")
	cmd.Flags().StringVar(&label, "label", "", "Display label")
	cmd.Flags().StringVar(&description, "description", "", "Interface and invocation description")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable the interface")
	cmd.Flags().StringVar(&inputSchemaFile, "input-schema", "", "Read invocation input schema EDN from file")
	cmd.Flags().StringVar(&inputSchemaLiteral, "input-schema-literal", "", "Invocation input schema EDN literal")
	cmd.Flags().StringVar(&outputSchemaFile, "output-schema", "", "Read interface output schema EDN from file")
	cmd.Flags().StringVar(&outputSchemaLiteral, "output-schema-literal", "", "Interface output schema EDN literal")
	cmd.Flags().StringVar(&responseFile, "response", "", "Read invocation response EDN from file")
	cmd.Flags().StringVar(&responseLiteral, "response-literal", "", "Invocation response EDN literal")
	cmd.Flags().StringVar(&path, "path", "", "HTTP interface path for --kind api/http")
	cmd.Flags().StringVar(&method, "method", "", "HTTP interface method for --kind api/http")
	cmd.Flags().StringVar(&eventName, "event-name", "", "External event name for --kind webhook")
	cmd.Flags().StringVar(&auth, "auth", "", "Simple interface auth mode")
	cmd.Flags().StringVar(&authJSON, "auth-json", "", "Structured interface auth as a JSON object")
	cmd.Flags().StringVar(&authFile, "auth-file", "", "Read structured interface auth JSON from file")
	cmd.Flags().BoolVar(&trustedMetadata, "trusted-metadata", false, "Expose authored MCP metadata as trusted")
	cmd.Flags().BoolVar(&validate, "validate", false, "Validate the saved interface in the same CLI command")
	cmd.Flags().StringSliceVar(&clearFields, "clear", nil, "Clear optional fields (label, description, invocation, auth, input-schema, output-schema, response, path, method, event-name, trusted-metadata); repeat as needed")
	return cmd
}

func newFlowsInterfacesValidateCmd(app *App) *cobra.Command {
	var source string
	var kind string
	cmd := &cobra.Command{
		Use:   "validate <flow-slug> <interface-id-or-tool-name>",
		Short: "Validate a draft flow interface",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAuthoringAPI(cmd, app, "flows interfaces validate")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := pruneEmptyStrings(map[string]any{
				"flowSlug":    strings.TrimSpace(args[0]),
				"interfaceId": strings.TrimSpace(args[1]),
				"source":      strings.TrimSpace(source),
				"kind":        strings.TrimSpace(kind),
			})
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.interfaces.validate", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	cmd.Flags().StringVar(&kind, "kind", "", "Interface kind used to disambiguate validation (manual|api|http|webhook|mcp)")
	return cmd
}

func newFlowsInterfacesRemoveCmd(app *App) *cobra.Command {
	var source string
	var kind string
	cmd := &cobra.Command{
		Use:   "remove <flow-slug> <interface-id-or-tool-name>",
		Short: "Remove a draft flow interface",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAuthoringAPI(cmd, app, "flows interfaces remove")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := pruneEmptyStrings(map[string]any{
				"flowSlug":    strings.TrimSpace(args[0]),
				"interfaceId": strings.TrimSpace(args[1]),
				"source":      strings.TrimSpace(source),
				"kind":        strings.TrimSpace(kind),
			})
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.interfaces.remove", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	cmd.Flags().StringVar(&kind, "kind", "", "Interface kind used to disambiguate removal (manual|api|http|webhook|mcp)")
	return cmd
}

func optionalArg(args []string, idx int) string {
	if idx >= 0 && idx < len(args) {
		return strings.TrimSpace(args[idx])
	}
	return ""
}

func newFlowsInterfacesListCmd(app *App) *cobra.Command {
	var target string
	var version int
	var installationID string
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List invocation-backed interfaces for a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, status, flow, resolvedTarget, resolvedInstallationID, err := fetchFlowInterfaceMetadata(cmd.Context(), app, args[0], target, version, installationID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				enrichFlowInterfaceFailure(resp, args[0], resolvedInstallationID, "")
				return writeAPIResult(cmd, app, resp, status)
			}
			items := withFlowInterfaceEndpointMetadata(app, flowInterfaceItems(flow, resolvedTarget), args[0], resolvedInstallationID, resolvedTarget)
			out := map[string]any{
				"ok":          true,
				"workspaceId": workspaceIDFromEnvelope(resp, app.WorkspaceID),
				"meta": pruneEmptyStrings(map[string]any{
					"target": resolvedTarget,
					"count":  len(items),
				}),
				"data": pruneEmptyStrings(map[string]any{
					"flowSlug":       args[0],
					"target":         resolvedTarget,
					"installationId": resolvedInstallationID,
					"items":          items,
				}),
			}
			return writeAPIResult(cmd, app, out, 200)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Interface target (draft|live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Inspect interfaces for a specific installation id")
	cmd.Flags().IntVar(&version, "version", 0, "Release version override for draft/source lookup")
	return cmd
}

func newFlowsInterfacesShowCmd(app *App) *cobra.Command {
	var target string
	var version int
	var family string
	var installationID string
	cmd := &cobra.Command{
		Use:   "show <flow-slug> <interface-id-or-tool-name>",
		Short: "Show one invocation-backed interface",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, status, flow, resolvedTarget, resolvedInstallationID, err := fetchFlowInterfaceMetadata(cmd.Context(), app, args[0], target, version, installationID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				enrichFlowInterfaceFailure(resp, args[0], resolvedInstallationID, args[1])
				return writeAPIResult(cmd, app, resp, status)
			}
			items := withFlowInterfaceEndpointMetadata(app, flowInterfaceItems(flow, resolvedTarget), args[0], resolvedInstallationID, resolvedTarget)
			item := findFlowInterfaceItem(items, args[1], family)
			if item == nil {
				out := map[string]any{
					"ok": false,
					"error": map[string]any{
						"message": "Interface not found",
						"details": map[string]any{
							"flowSlug":       args[0],
							"target":         resolvedTarget,
							"installationId": resolvedInstallationID,
							"interface":      args[1],
							"family":         strings.TrimSpace(family),
						},
					},
				}
				enrichFlowInterfaceFailure(out, args[0], resolvedInstallationID, args[1])
				return writeAPIResult(cmd, app, out, 404)
			}
			out := map[string]any{
				"ok":          true,
				"workspaceId": workspaceIDFromEnvelope(resp, app.WorkspaceID),
				"meta": pruneEmptyStrings(map[string]any{
					"target": resolvedTarget,
				}),
				"data": pruneEmptyStrings(map[string]any{
					"flowSlug":       args[0],
					"target":         resolvedTarget,
					"installationId": resolvedInstallationID,
					"interface":      item,
				}),
			}
			return writeAPIResult(cmd, app, out, 200)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Interface target (draft|live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Inspect interfaces for a specific installation id")
	cmd.Flags().IntVar(&version, "version", 0, "Release version override for draft/source lookup")
	cmd.Flags().StringVar(&family, "family", "", "Restrict lookup to interface family (manual|http|webhook|mcp)")
	return cmd
}

func newFlowsMetricsCmd(app *App) *cobra.Command {
	var target string
	var source string
	var installationID string
	var kind string
	var limit int
	cmd := &cobra.Command{
		Use:   "metrics <flow-slug> [entrypoint-id]",
		Short: "Show recent invocation metrics for a flow",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows metrics requires --api/BREYTA_API_URL"))
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			installationID = strings.TrimSpace(installationID)
			resolvedTarget := strings.TrimSpace(target)
			resolvedSource := strings.TrimSpace(source)
			interfaceScope := ""
			if resolvedSource != "" && (installationID != "" || resolvedTarget != "") {
				return writeErr(cmd, errors.New("--source cannot be combined with --installation-id or --target"))
			}
			if resolvedSource != "" {
				normalizedSource, err := normalizeInstallTarget(resolvedSource)
				if err != nil {
					return writeErr(cmd, errors.New("invalid --source (expected draft or live)"))
				}
				interfaceScope = normalizedSource
				resolvedSource = normalizedSource
			}
			if installationID != "" && resolvedTarget != "" {
				return writeErr(cmd, errors.New("--installation-id cannot be combined with --target"))
			}
			if installationID == "" && resolvedTarget != "" {
				normalizedTarget, err := normalizeInstallTarget(resolvedTarget)
				if err != nil {
					return writeErr(cmd, err)
				}
				if normalizedTarget != "live" {
					return writeErr(cmd, errors.New("flows metrics only supports --target live"))
				}
				liveTarget, err := resolveLiveProfileTarget(cmd.Context(), app, args[0], false)
				if err != nil {
					return writeErr(cmd, err)
				}
				installationID = liveTarget.ProfileID
				resolvedTarget = normalizedTarget
			}
			payload := pruneEmptyStrings(map[string]any{
				"flowSlug":       args[0],
				"entrypointId":   optionalArg(args, 1),
				"installationId": installationID,
				"interfaceScope": interfaceScope,
				"kind":           kind,
				"limit":          limit,
			})
			resp, status, err := runAPICommandWithContext(cmd.Context(), app, "flows.invocations.metrics", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if meta := mapStringAny(resp["meta"]); meta != nil && resolvedTarget != "" {
				meta["target"] = resolvedTarget
			}
			if meta := mapStringAny(resp["meta"]); meta != nil && resolvedSource != "" {
				meta["source"] = resolvedSource
			}
			return writeAPIResult(cmd, app, resp, status)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Resolve metrics for a flow target (live)")
	cmd.Flags().StringVar(&source, "source", "", "Restrict metrics to author source calls (draft|live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Restrict metrics to a specific installation id")
	cmd.Flags().StringVar(&kind, "kind", "", "Restrict metrics to invocation kind (manual|http|mcp|schedule|webhook|cli)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum metric rows to return")
	return cmd
}

func newFlowsInterfacesCallCmd(app *App) *cobra.Command {
	var target string
	var installationID string
	var inputJSON string
	var wait bool
	var timeout time.Duration
	var poll time.Duration
	cmd := &cobra.Command{
		Use:   "call <flow-slug> <http-interface-id>",
		Short: "Call a flow HTTP interface",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows interfaces call requires --api/BREYTA_API_URL"))
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			installationID = strings.TrimSpace(installationID)
			resolvedTarget := strings.TrimSpace(target)
			if installationID == "" {
				var err error
				resolvedTarget, err = normalizeInstallTarget(resolvedTarget)
				if err != nil {
					return writeErr(cmd, err)
				}
			} else if strings.TrimSpace(target) != "" {
				return writeErr(cmd, errors.New("--installation-id cannot be combined with --target"))
			}
			input, err := parseJSONObjectFlag(inputJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
			}
			path := flowInterfaceCallPath(args[0], args[1], installationID, resolvedTarget)
			out, status, err := apiClient(app).DoREST(cmd.Context(), http.MethodPost, path, nil, map[string]any{"input": input})
			if err != nil {
				return writeErr(cmd, err)
			}
			resp := mapStringAny(out)
			if resp == nil {
				resp = map[string]any{
					"ok":     status >= 200 && status < 300,
					"status": status,
					"data":   out,
				}
			}
			enrichFlowInterfaceFailure(resp, args[0], installationID, args[1])
			if wait && status < 400 && isOK(resp) {
				return waitForRunCompletion(cmd, app, resp, args[0], "flows.interfaces.call", nil, timeout, poll, nil)
			}
			return writeAPIResult(cmd, app, resp, status)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Author interface target (draft|live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Installation id to call")
	cmd.Flags().StringVar(&inputJSON, "input", "{}", "JSON object input for the interface invocation")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run completion")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultFlowRunWaitTimeout, "Wait timeout")
	cmd.Flags().DurationVar(&poll, "poll", 250*time.Millisecond, "Poll interval while waiting")
	return cmd
}

func newFlowsInterfacesCurlCmd(app *App) *cobra.Command {
	var target string
	var installationID string
	var inputJSON string
	cmd := &cobra.Command{
		Use:   "curl <flow-slug> <http-interface-id>",
		Short: "Generate a curl command for a flow HTTP interface",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, status, flow, resolvedTarget, resolvedInstallationID, err := fetchFlowInterfaceMetadata(cmd.Context(), app, args[0], target, 0, installationID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				enrichFlowInterfaceFailure(resp, args[0], resolvedInstallationID, args[1])
				return writeAPIResult(cmd, app, resp, status)
			}
			items := withFlowInterfaceEndpointMetadata(app, flowInterfaceItems(flow, resolvedTarget), args[0], resolvedInstallationID, resolvedTarget)
			item := findFlowInterfaceItem(items, args[1], "http")
			if item == nil {
				out := map[string]any{
					"ok": false,
					"error": map[string]any{
						"message": "HTTP interface not found",
						"details": map[string]any{
							"flowSlug":  args[0],
							"target":    resolvedTarget,
							"interface": args[1],
						},
					},
				}
				enrichFlowInterfaceFailure(out, args[0], resolvedInstallationID, args[1])
				return writeAPIResult(cmd, app, out, 404)
			}
			input, err := parseJSONObjectFlag(inputJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
			}
			body, err := json.Marshal(map[string]any{"input": input})
			if err != nil {
				return writeErr(cmd, err)
			}
			endpoint := mapStringAny(item["endpoint"])
			url := firstNonBlankString(endpoint["url"])
			curl := strings.Join([]string{
				"curl",
				"-X", "POST",
				shellSingleQuote(url),
				"-H", `"Authorization: Bearer ${BREYTA_TOKEN}"`,
				"-H", shellSingleQuote("Content-Type: application/json"),
				"--data", shellSingleQuote(string(body)),
			}, " ")
			out := map[string]any{
				"ok":          true,
				"workspaceId": workspaceIDFromEnvelope(resp, app.WorkspaceID),
				"data": pruneEmptyStrings(map[string]any{
					"flowSlug":       args[0],
					"target":         resolvedTarget,
					"installationId": resolvedInstallationID,
					"interface":      item,
					"curl":           curl,
				}),
			}
			return writeAPIResult(cmd, app, out, 200)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Author interface target (draft|live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Installation id to call")
	cmd.Flags().StringVar(&inputJSON, "input", "{}", "JSON object input for the interface invocation")
	return cmd
}

func enrichFlowInterfaceFailure(out map[string]any, flowSlug string, installationID string, interfaceID string) {
	if out == nil || isOK(out) {
		return
	}
	msg := strings.ToLower(getErrorMessage(out))
	if strings.TrimSpace(msg) == "" {
		return
	}
	meta := ensureMeta(out)
	if meta != nil {
		if _, exists := meta["hint"]; !exists {
			switch {
			case strings.Contains(msg, "interface not found"):
				if strings.TrimSpace(installationID) != "" {
					meta["hint"] = "Inspect available interfaces with `breyta flows installations interfaces " + strings.TrimSpace(installationID) + "`, or check the authored :interfaces map and release/promote the flow version that declares this interface."
				} else {
					meta["hint"] = "Inspect available interfaces with `breyta flows interfaces list " + strings.TrimSpace(flowSlug) + "` for draft or `breyta flows interfaces list " + strings.TrimSpace(flowSlug) + " --target live`, or check the authored :interfaces map and release/promote the flow version that declares this interface."
				}
			case strings.Contains(msg, "installation not found"), strings.Contains(msg, "invalid installationid"):
				meta["hint"] = "Check the installation id with `breyta flows installations list " + strings.TrimSpace(flowSlug) + "`, then inspect interfaces with `breyta flows installations interfaces <installation-id>`."
			case strings.Contains(msg, "invocation not found"), strings.Contains(msg, "no invocation"):
				meta["hint"] = "The interface points at a missing invocation. Update the flow :interfaces entry to reference an existing :invocations key, then push/release/promote the flow."
			default:
				meta["hint"] = "Inspect interface metadata with `breyta flows interfaces show " + strings.TrimSpace(flowSlug) + " " + strings.TrimSpace(interfaceID) + "` and check installation configuration with `breyta flows installations get <installation-id>`."
			}
		}
	}
	errMap := mapStringAny(out["error"])
	if errMap != nil {
		if _, exists := errMap["hintRefs"]; !exists {
			errMap["hintRefs"] = []any{
				map[string]any{"kind": "find", "query": "flow interfaces invocation"},
				map[string]any{"kind": "find", "query": "installations configure invocation"},
			}
		}
	}
}

func fetchFlowInterfaceMetadata(ctx context.Context, app *App, flowSlug string, target string, version int, installationID string) (map[string]any, int, map[string]any, string, string, error) {
	if !isAPIMode(app) {
		return nil, 0, nil, "", "", errors.New("flows interfaces requires --api/BREYTA_API_URL")
	}
	if err := requireAPI(app); err != nil {
		return nil, 0, nil, "", "", err
	}
	resolvedTarget, err := normalizeInstallTarget(target)
	if err != nil {
		return nil, 0, nil, "", "", err
	}
	installationID = strings.TrimSpace(installationID)
	if resolvedTarget == "live" && version > 0 {
		return nil, 0, nil, "", "", errors.New("--target cannot be combined with --version")
	}
	if installationID != "" && (strings.TrimSpace(target) != "" || version > 0) {
		return nil, 0, nil, "", "", errors.New("--installation-id cannot be combined with --target or --version")
	}
	payload := map[string]any{
		"flowSlug":           flowSlug,
		"source":             "draft",
		"includeFlowLiteral": false,
	}
	if installationID != "" {
		resp, status, err := runAPICommandWithContext(ctx, app, "flows.installations.get", map[string]any{"profileId": installationID})
		if err != nil {
			return nil, 0, nil, "", "", err
		}
		if status >= 400 || !isOK(resp) {
			return resp, status, nil, "installation", installationID, nil
		}
		data := mapStringAny(resp["data"])
		flowSlugFromInstallation := firstNonBlankString(data["flowSlug"], data["flow-slug"])
		sourceFlowSlugFromInstallation := firstNonBlankString(data["sourceFlowSlug"], data["source-flow-slug"])
		if flowSlugFromInstallation != "" && flowSlugFromInstallation != flowSlug && sourceFlowSlugFromInstallation != flowSlug {
			return nil, 0, nil, "", "", fmt.Errorf("--installation-id %s belongs to flow %s, not %s", installationID, flowSlugFromInstallation, flowSlug)
		}
		if sourceFlowSlugFromInstallation != "" {
			payload["flowSlug"] = sourceFlowSlugFromInstallation
		}
		if resolvedVersion := firstPositiveInt(data["version"], data["installedVersion"], data["installed-version"]); resolvedVersion > 0 {
			payload["version"] = resolvedVersion
		}
		addInstallationSourceLookupArgs(payload, data)
		payload["source"] = "active"
		resolvedTarget = "installation"
	} else if resolvedTarget == "live" {
		payload["source"] = "active"
	} else if version > 0 {
		payload["source"] = "version"
		payload["version"] = version
		resolvedTarget = "version"
	}
	resp, status, err := runAPICommandWithContext(ctx, app, "flows.get", payload)
	if err != nil {
		return nil, 0, nil, "", "", err
	}
	flow := mapStringAny(mapStringAny(resp["data"])["flow"])
	return resp, status, flow, resolvedTarget, installationID, nil
}

func addInstallationSourceLookupArgs(payload map[string]any, data map[string]any) {
	if payload == nil || data == nil {
		return
	}
	if sourceWorkspaceID := firstNonBlankString(data["sourceWorkspaceId"], data["source-workspace-id"]); sourceWorkspaceID != "" {
		payload["sourceWorkspaceId"] = sourceWorkspaceID
	}
	if sourceFlowSlug := firstNonBlankString(data["sourceFlowSlug"], data["source-flow-slug"]); sourceFlowSlug != "" {
		payload["sourceFlowSlug"] = sourceFlowSlug
	}
}

func withFlowInterfaceEndpointMetadata(app *App, items []any, flowSlug string, installationID string, target string) []any {
	installationID = strings.TrimSpace(installationID)
	flowSlug = strings.TrimSpace(flowSlug)
	target = strings.TrimSpace(target)
	if target == "version" {
		return items
	}
	if flowSlug == "" || (installationID == "" && target == "") {
		return items
	}
	out := make([]any, 0, len(items))
	for _, raw := range items {
		item := mapStringAny(raw)
		if strings.EqualFold(firstNonBlankString(item["family"]), "http") {
			if interfaceID := firstNonBlankString(item["id"]); interfaceID != "" {
				method := strings.ToUpper(firstNonBlankString(item["method"]))
				if method == "" {
					method = "POST"
				}
				item["endpoint"] = map[string]any{
					"method":       method,
					"url":          flowInterfaceRuntimeURL(app, installationID, flowSlug, interfaceID, target),
					"alternateUrl": flowInterfaceWorkspaceRuntimeURL(app, installationID, flowSlug, interfaceID, target),
					"auth":         "workspace-api-auth",
				}
			}
		}
		if strings.EqualFold(firstNonBlankString(item["family"]), "webhook") {
			interfaceID := firstNonBlankString(item["id"], item["eventName"])
			eventName := firstNonBlankString(item["eventName"], interfaceID)
			if interfaceID != "" || eventName != "" {
				auth := "webhook-auth"
				endpointURL := flowWebhookRuntimeURL(app, installationID, flowSlug, eventName)
				if installationID == "" {
					sourceSegment := interfaceID
					if sourceSegment == "" {
						sourceSegment = eventName
					}
					auth = "workspace-api-auth"
					endpointURL = flowInterfaceSourceRuntimeURL(app, target, flowSlug, sourceSegment)
				}
				item["endpoint"] = map[string]any{
					"method": "POST",
					"url":    endpointURL,
					"auth":   auth,
				}
				if installationID == "" {
					itemEndpoint := mapStringAny(item["endpoint"])
					itemEndpoint["alternateUrl"] = flowInterfaceWorkspaceRuntimeURL(app, installationID, flowSlug, interfaceID, target)
					item["endpoint"] = itemEndpoint
				}
			}
		}
		if strings.EqualFold(firstNonBlankString(item["family"]), "mcp") {
			interfaceID := firstNonBlankString(item["id"], item["toolName"])
			if interfaceID != "" {
				item["endpoint"] = map[string]any{
					"method":       "POST",
					"url":          flowInterfaceRuntimeURL(app, installationID, flowSlug, interfaceID, target),
					"alternateUrl": flowInterfaceWorkspaceRuntimeURL(app, installationID, flowSlug, interfaceID, target),
					"auth":         "workspace-api-auth",
					"protocol":     "mcp",
					"transport":    "streamable-http",
				}
			}
		}
		out = append(out, pruneEmptyStrings(item))
	}
	return out
}

func flowInterfaceRuntimeURL(app *App, installationID string, flowSlug string, interfaceID string, target string) string {
	if strings.TrimSpace(installationID) == "" {
		return flowInterfaceSourceRuntimeURL(app, target, flowSlug, interfaceID)
	}
	ensureAPIURL(app)
	path := fmt.Sprintf("/api/flows/%s/installations/%s/interfaces/%s",
		url.PathEscape(strings.TrimSpace(flowSlug)),
		url.PathEscape(strings.TrimSpace(installationID)),
		url.PathEscape(strings.TrimSpace(interfaceID)))
	return strings.TrimRight(strings.TrimSpace(app.APIURL), "/") + path
}

func flowInterfaceSourceRuntimeURL(app *App, target string, flowSlug string, interfaceID string) string {
	ensureAPIURL(app)
	source := strings.TrimSpace(target)
	if source == "" {
		source = "draft"
	}
	path := fmt.Sprintf("/api/flows/%s/interfaces/%s/%s",
		url.PathEscape(strings.TrimSpace(flowSlug)),
		url.PathEscape(source),
		url.PathEscape(strings.TrimSpace(interfaceID)))
	return strings.TrimRight(strings.TrimSpace(app.APIURL), "/") + path
}

func flowInterfaceCallPath(flowSlug string, interfaceID string, installationID string, target string) string {
	if strings.TrimSpace(installationID) != "" {
		return fmt.Sprintf("/api/flows/%s/installations/%s/interfaces/%s",
			url.PathEscape(strings.TrimSpace(flowSlug)),
			url.PathEscape(strings.TrimSpace(installationID)),
			url.PathEscape(strings.TrimSpace(interfaceID)))
	}
	source := strings.TrimSpace(target)
	if source == "" {
		source = "draft"
	}
	return fmt.Sprintf("/api/flows/%s/interfaces/%s/%s",
		url.PathEscape(strings.TrimSpace(flowSlug)),
		url.PathEscape(source),
		url.PathEscape(strings.TrimSpace(interfaceID)))
}

func flowInterfaceWorkspaceRuntimeURL(app *App, installationID string, flowSlug string, interfaceID string, target string) string {
	ensureAPIURL(app)
	if strings.TrimSpace(installationID) != "" {
		path := fmt.Sprintf("/api/workspaces/%s/flows/%s/installations/%s/interfaces/%s",
			url.PathEscape(app.WorkspaceID),
			url.PathEscape(strings.TrimSpace(flowSlug)),
			url.PathEscape(strings.TrimSpace(installationID)),
			url.PathEscape(strings.TrimSpace(interfaceID)))
		return strings.TrimRight(strings.TrimSpace(app.APIURL), "/") + path
	}
	source := strings.TrimSpace(target)
	if source == "" {
		source = "draft"
	}
	path := fmt.Sprintf("/api/workspaces/%s/flows/%s/interfaces/%s/%s",
		url.PathEscape(app.WorkspaceID),
		url.PathEscape(strings.TrimSpace(flowSlug)),
		url.PathEscape(source),
		url.PathEscape(strings.TrimSpace(interfaceID)))
	return strings.TrimRight(strings.TrimSpace(app.APIURL), "/") + path
}

func flowWebhookRuntimeURL(app *App, installationID string, flowSlug string, eventName string) string {
	ensureAPIURL(app)
	path := fmt.Sprintf("/%s/events/webhooks/%s/%s/%s",
		url.PathEscape(app.WorkspaceID),
		url.PathEscape(strings.TrimSpace(flowSlug)),
		url.PathEscape(strings.TrimSpace(eventName)),
		url.PathEscape(strings.TrimSpace(installationID)))
	return strings.TrimRight(strings.TrimSpace(app.APIURL), "/") + path
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func firstPositiveInt(values ...any) int {
	for _, value := range values {
		if n := anyInt(value); n > 0 {
			return n
		}
	}
	return 0
}

func flowInterfaceItems(flow map[string]any, target string) []any {
	interfaces := mapStringAny(flow["interfaces"])
	invocations := mapStringAny(flow["invocations"])
	items := make([]any, 0)
	for _, raw := range sliceAny(interfaces["manual"]) {
		iface := mapStringAny(raw)
		invocationID := firstNonBlankString(iface["invocation"])
		item := map[string]any{
			"family":        "manual",
			"id":            firstNonBlankString(iface["id"]),
			"label":         firstNonBlankString(iface["label"]),
			"invocationId":  invocationID,
			"target":        target,
			"description":   firstNonBlankString(iface["description"]),
			"invocation":    invocationContract(invocations, invocationID),
			"runtimeStatus": "available",
		}
		items = append(items, pruneEmptyStrings(item))
	}
	for _, raw := range sliceAny(interfaces["http"]) {
		iface := mapStringAny(raw)
		invocationID := firstNonBlankString(iface["invocation"])
		item := map[string]any{
			"family":        "http",
			"id":            firstNonBlankString(iface["id"]),
			"invocationId":  invocationID,
			"target":        target,
			"method":        firstNonBlankString(iface["method"]),
			"path":          firstNonBlankString(iface["path"]),
			"auth":          firstNonBlankString(iface["auth"]),
			"description":   firstNonBlankString(iface["description"]),
			"invocation":    invocationContract(invocations, invocationID),
			"runtimeStatus": "available",
		}
		items = append(items, pruneEmptyStrings(item))
	}
	for _, raw := range sliceAny(interfaces["webhook"]) {
		iface := mapStringAny(raw)
		invocationID := firstNonBlankString(iface["invocation"])
		interfaceID := firstNonBlankString(iface["id"])
		eventName := firstNonBlankString(iface["eventName"], iface["event-name"])
		if eventName == "" {
			eventName = interfaceID
		}
		item := map[string]any{
			"family":        "webhook",
			"id":            interfaceID,
			"eventName":     eventName,
			"invocationId":  invocationID,
			"target":        target,
			"description":   firstNonBlankString(iface["description"]),
			"auth":          webhookAuthSummary(mapStringAny(iface["auth"])),
			"invocation":    invocationContract(invocations, invocationID),
			"runtimeStatus": "available",
		}
		items = append(items, pruneEmptyStrings(item))
	}
	for _, raw := range sliceAny(interfaces["mcp"]) {
		iface := mapStringAny(raw)
		invocationID := firstNonBlankString(iface["invocation"])
		toolName := firstNonBlankString(iface["toolName"], iface["tool-name"])
		item := map[string]any{
			"family":        "mcp",
			"id":            toolName,
			"toolName":      toolName,
			"invocationId":  invocationID,
			"target":        target,
			"description":   firstNonBlankString(iface["description"]),
			"invocation":    invocationContract(invocations, invocationID),
			"runtimeStatus": "available",
		}
		items = append(items, pruneEmptyStrings(item))
	}
	return items
}

func webhookAuthSummary(auth map[string]any) any {
	if auth == nil {
		return nil
	}
	out := pruneEmptyStrings(map[string]any{
		"type":            firstNonBlankString(auth["type"]),
		"location":        firstNonBlankString(auth["location"]),
		"param":           firstNonBlankString(auth["param"], auth["queryParam"], auth["query-param"]),
		"secretRef":       firstNonBlankString(auth["secretRef"], auth["secret-ref"]),
		"publicKeyRef":    firstNonBlankString(auth["publicKeyRef"], auth["public-key-ref"]),
		"signatureHeader": firstNonBlankString(auth["signatureHeader"], auth["signature-header"]),
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func invocationContract(invocations map[string]any, invocationID string) any {
	if strings.TrimSpace(invocationID) == "" {
		return nil
	}
	if v, ok := invocations[invocationID]; ok {
		return v
	}
	if v, ok := invocations[":"+invocationID]; ok {
		return v
	}
	return nil
}

func findFlowInterfaceItem(items []any, interfaceID string, family string) map[string]any {
	want := strings.TrimSpace(interfaceID)
	wantFamily := strings.ToLower(strings.TrimSpace(family))
	for _, raw := range items {
		item := mapStringAny(raw)
		itemFamily := strings.ToLower(firstNonBlankString(item["family"]))
		if wantFamily != "" && itemFamily != wantFamily {
			continue
		}
		if firstNonBlankString(item["id"], item["toolName"]) == want {
			return item
		}
	}
	return nil
}

func pruneEmptyStrings(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		if v == nil {
			continue
		}
		out[k] = v
	}
	return out
}
