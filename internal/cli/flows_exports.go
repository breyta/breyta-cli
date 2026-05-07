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

func newFlowsExportsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exports",
		Short: "Inspect flow exports backed by invocations",
		Long: strings.TrimSpace(`
Inspect external client surfaces declared under :exports.

These commands read export metadata from the API. They do not construct runtime
HTTP or MCP routes locally.
`),
	}
	cmd.AddCommand(newFlowsExportsListCmd(app))
	cmd.AddCommand(newFlowsExportsShowCmd(app))
	cmd.AddCommand(newFlowsExportsCallCmd(app))
	cmd.AddCommand(newFlowsExportsCurlCmd(app))
	return cmd
}

func newFlowsExportsListCmd(app *App) *cobra.Command {
	var target string
	var version int
	var installationID string
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List exported invocation surfaces for a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, status, flow, resolvedTarget, resolvedInstallationID, err := fetchFlowExportMetadata(cmd.Context(), app, args[0], target, version, installationID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				return writeAPIResult(cmd, app, resp, status)
			}
			items := withFlowExportEndpointMetadata(app, flowExportItems(flow, resolvedTarget), args[0], resolvedInstallationID)
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
	cmd.Flags().StringVar(&target, "target", "", "Export target (draft|live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Inspect exports for a specific installation id")
	cmd.Flags().IntVar(&version, "version", 0, "Release version override for draft/source lookup")
	return cmd
}

func newFlowsExportsShowCmd(app *App) *cobra.Command {
	var target string
	var version int
	var family string
	var installationID string
	cmd := &cobra.Command{
		Use:   "show <flow-slug> <export-id-or-tool-name>",
		Short: "Show one exported invocation surface",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, status, flow, resolvedTarget, resolvedInstallationID, err := fetchFlowExportMetadata(cmd.Context(), app, args[0], target, version, installationID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				return writeAPIResult(cmd, app, resp, status)
			}
			items := withFlowExportEndpointMetadata(app, flowExportItems(flow, resolvedTarget), args[0], resolvedInstallationID)
			item := findFlowExportItem(items, args[1], family)
			if item == nil {
				out := map[string]any{
					"ok": false,
					"error": map[string]any{
						"message": "Export not found",
						"details": map[string]any{
							"flowSlug":       args[0],
							"target":         resolvedTarget,
							"installationId": resolvedInstallationID,
							"export":         args[1],
							"family":         strings.TrimSpace(family),
						},
					},
				}
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
					"export":         item,
				}),
			}
			return writeAPIResult(cmd, app, out, 200)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Export target (draft|live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Inspect exports for a specific installation id")
	cmd.Flags().IntVar(&version, "version", 0, "Release version override for draft/source lookup")
	cmd.Flags().StringVar(&family, "family", "", "Restrict lookup to export family (http|mcp)")
	return cmd
}

func newFlowsExportsCallCmd(app *App) *cobra.Command {
	var target string
	var installationID string
	var legacyProfileID string
	var inputJSON string
	var wait bool
	var timeout time.Duration
	var poll time.Duration
	cmd := &cobra.Command{
		Use:   "call <flow-slug> <http-export-id>",
		Short: "Call a flow HTTP export",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows exports call requires --api/BREYTA_API_URL"))
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			installationID = strings.TrimSpace(installationID)
			if installationID == "" {
				installationID = strings.TrimSpace(legacyProfileID)
			}
			if installationID == "" {
				resolvedTarget, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				if resolvedTarget != "live" {
					return writeErr(cmd, errors.New("--installation-id is required (or use --target live)"))
				}
				liveTarget, err := resolveLiveProfileTarget(cmd.Context(), app, args[0], false)
				if err != nil {
					return writeErr(cmd, err)
				}
				installationID = liveTarget.ProfileID
			} else if strings.TrimSpace(target) != "" {
				return writeErr(cmd, errors.New("--installation-id cannot be combined with --target"))
			}
			input, err := parseJSONObjectFlag(inputJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
			}
			path := fmt.Sprintf("/api/workspaces/%s/flow-exports/%s/%s/%s",
				url.PathEscape(app.WorkspaceID),
				url.PathEscape(installationID),
				url.PathEscape(args[0]),
				url.PathEscape(args[1]))
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
			if wait && status < 400 && isOK(resp) {
				return waitForRunCompletion(cmd, app, resp, args[0], "flows.exports.call", timeout, poll)
			}
			return writeAPIResult(cmd, app, resp, status)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Resolve and call a flow target (live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Installation id to call")
	cmd.Flags().StringVar(&legacyProfileID, "profile-id", "", "Deprecated alias for --installation-id")
	_ = cmd.Flags().MarkHidden("profile-id")
	cmd.Flags().StringVar(&inputJSON, "input", "{}", "JSON object input for the export invocation")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run completion")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Wait timeout")
	cmd.Flags().DurationVar(&poll, "poll", 250*time.Millisecond, "Poll interval while waiting")
	return cmd
}

func newFlowsExportsCurlCmd(app *App) *cobra.Command {
	var target string
	var installationID string
	var inputJSON string
	cmd := &cobra.Command{
		Use:   "curl <flow-slug> <http-export-id>",
		Short: "Generate a curl command for a flow HTTP export",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, status, flow, resolvedTarget, resolvedInstallationID, err := fetchFlowExportMetadata(cmd.Context(), app, args[0], target, 0, installationID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				return writeAPIResult(cmd, app, resp, status)
			}
			if strings.TrimSpace(resolvedInstallationID) == "" {
				return writeErr(cmd, errors.New("--installation-id is required (or use --target live)"))
			}
			items := withFlowExportEndpointMetadata(app, flowExportItems(flow, resolvedTarget), args[0], resolvedInstallationID)
			item := findFlowExportItem(items, args[1], "http")
			if item == nil {
				return writeAPIResult(cmd, app, map[string]any{
					"ok": false,
					"error": map[string]any{
						"message": "HTTP export not found",
						"details": map[string]any{
							"flowSlug": args[0],
							"target":   resolvedTarget,
							"export":   args[1],
						},
					},
				}, 404)
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
				"-H", shellSingleQuote("Authorization: Bearer ${BREYTA_TOKEN}"),
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
					"export":         item,
					"curl":           curl,
				}),
			}
			return writeAPIResult(cmd, app, out, 200)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Export target (live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Installation id to call")
	cmd.Flags().StringVar(&inputJSON, "input", "{}", "JSON object input for the export invocation")
	return cmd
}

func fetchFlowExportMetadata(ctx context.Context, app *App, flowSlug string, target string, version int, installationID string) (map[string]any, int, map[string]any, string, string, error) {
	if !isAPIMode(app) {
		return nil, 0, nil, "", "", errors.New("flows exports requires --api/BREYTA_API_URL")
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
		if flowSlugFromInstallation != "" && flowSlugFromInstallation != flowSlug {
			return nil, 0, nil, "", "", fmt.Errorf("--installation-id %s belongs to flow %s, not %s", installationID, flowSlugFromInstallation, flowSlug)
		}
		if resolvedVersion := firstPositiveInt(data["version"], data["installedVersion"], data["installed-version"]); resolvedVersion > 0 {
			payload["version"] = resolvedVersion
		}
		payload["source"] = "active"
		resolvedTarget = "installation"
	} else if resolvedTarget == "live" {
		target, err := resolveLiveProfileTarget(ctx, app, flowSlug, true)
		if err != nil {
			return nil, 0, nil, "", "", err
		}
		payload["source"] = "active"
		if target.Version > 0 {
			payload["version"] = target.Version
		}
		installationID = target.ProfileID
	} else if version > 0 {
		payload["version"] = version
	}
	resp, status, err := runAPICommandWithContext(ctx, app, "flows.get", payload)
	if err != nil {
		return nil, 0, nil, "", "", err
	}
	flow := mapStringAny(mapStringAny(resp["data"])["flow"])
	return resp, status, flow, resolvedTarget, installationID, nil
}

func withFlowExportEndpointMetadata(app *App, items []any, flowSlug string, installationID string) []any {
	installationID = strings.TrimSpace(installationID)
	flowSlug = strings.TrimSpace(flowSlug)
	if installationID == "" || flowSlug == "" {
		return items
	}
	out := make([]any, 0, len(items))
	for _, raw := range items {
		item := mapStringAny(raw)
		if strings.EqualFold(firstNonBlankString(item["family"]), "http") {
			if exportID := firstNonBlankString(item["id"]); exportID != "" {
				item["endpoint"] = map[string]any{
					"method": "POST",
					"url":    flowExportRuntimeURL(app, installationID, flowSlug, exportID),
					"auth":   "workspace-token",
				}
			}
		}
		out = append(out, pruneEmptyStrings(item))
	}
	return out
}

func flowExportRuntimeURL(app *App, installationID string, flowSlug string, exportID string) string {
	ensureAPIURL(app)
	path := fmt.Sprintf("/api/workspaces/%s/flow-exports/%s/%s/%s",
		url.PathEscape(app.WorkspaceID),
		url.PathEscape(strings.TrimSpace(installationID)),
		url.PathEscape(strings.TrimSpace(flowSlug)),
		url.PathEscape(strings.TrimSpace(exportID)))
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

func flowExportItems(flow map[string]any, target string) []any {
	exports := mapStringAny(flow["exports"])
	invocations := mapStringAny(flow["invocations"])
	items := make([]any, 0)
	for _, raw := range sliceAny(exports["http"]) {
		export := mapStringAny(raw)
		invocationID := firstNonBlankString(export["invocation"])
		item := map[string]any{
			"family":        "http",
			"id":            firstNonBlankString(export["id"]),
			"invocationId":  invocationID,
			"target":        target,
			"method":        firstNonBlankString(export["method"]),
			"path":          firstNonBlankString(export["path"]),
			"auth":          firstNonBlankString(export["auth"]),
			"description":   firstNonBlankString(export["description"]),
			"invocation":    invocationContract(invocations, invocationID),
			"runtimeStatus": "available",
		}
		items = append(items, pruneEmptyStrings(item))
	}
	for _, raw := range sliceAny(exports["mcp"]) {
		export := mapStringAny(raw)
		invocationID := firstNonBlankString(export["invocation"])
		item := map[string]any{
			"family":        "mcp",
			"toolName":      firstNonBlankString(export["toolName"], export["tool-name"]),
			"invocationId":  invocationID,
			"target":        target,
			"description":   firstNonBlankString(export["description"]),
			"invocation":    invocationContract(invocations, invocationID),
			"runtimeStatus": "not_implemented",
		}
		items = append(items, pruneEmptyStrings(item))
	}
	return items
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

func findFlowExportItem(items []any, exportID string, family string) map[string]any {
	want := strings.TrimSpace(exportID)
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
