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
		Short: "Inspect flow interfaces backed by invocations",
		Long: strings.TrimSpace(`
Inspect callable surfaces declared under :interfaces.

These commands read interface metadata from the API. They do not construct runtime
HTTP or MCP routes locally.
`),
	}
	cmd.AddCommand(newFlowsInterfacesListCmd(app))
	cmd.AddCommand(newFlowsInterfacesShowCmd(app))
	cmd.AddCommand(newFlowsInterfacesCallCmd(app))
	cmd.AddCommand(newFlowsInterfacesCurlCmd(app))
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
			items := withFlowInterfaceEndpointMetadata(app, flowInterfaceItems(flow, resolvedTarget), args[0], resolvedInstallationID)
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
			items := withFlowInterfaceEndpointMetadata(app, flowInterfaceItems(flow, resolvedTarget), args[0], resolvedInstallationID)
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
			return writeAPIResult(cmd, app, resp, status)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Resolve metrics for a flow target (live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Restrict metrics to a specific installation id")
	cmd.Flags().StringVar(&kind, "kind", "", "Restrict metrics to invocation kind (manual|http|mcp|schedule|webhook|cli)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum metric rows to return")
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
			path := fmt.Sprintf("/api/workspaces/%s/flows/%s/installations/%s/interfaces/%s",
				url.PathEscape(app.WorkspaceID),
				url.PathEscape(args[0]),
				url.PathEscape(installationID),
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
			enrichFlowInterfaceFailure(resp, args[0], installationID, args[1])
			if wait && status < 400 && isOK(resp) {
				return waitForRunCompletion(cmd, app, resp, args[0], "flows.interfaces.call", timeout, poll)
			}
			return writeAPIResult(cmd, app, resp, status)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Resolve and call a flow target (live)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Installation id to call")
	cmd.Flags().StringVar(&inputJSON, "input", "{}", "JSON object input for the interface invocation")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run completion")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Wait timeout")
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
			if strings.TrimSpace(resolvedInstallationID) == "" {
				return writeErr(cmd, errors.New("--installation-id is required (or use --target live)"))
			}
			items := withFlowInterfaceEndpointMetadata(app, flowInterfaceItems(flow, resolvedTarget), args[0], resolvedInstallationID)
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
					"interface":      item,
					"curl":           curl,
				}),
			}
			return writeAPIResult(cmd, app, out, 200)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Interface target (live)")
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
					meta["hint"] = "Inspect available interfaces with `breyta flows interfaces list " + strings.TrimSpace(flowSlug) + " --target live`, or check the authored :interfaces map and release/promote the flow version that declares this interface."
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

func withFlowInterfaceEndpointMetadata(app *App, items []any, flowSlug string, installationID string) []any {
	installationID = strings.TrimSpace(installationID)
	flowSlug = strings.TrimSpace(flowSlug)
	if installationID == "" || flowSlug == "" {
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
					"method": method,
					"url":    flowInterfaceRuntimeURL(app, installationID, flowSlug, interfaceID),
					"auth":   "workspace-api-auth",
				}
			}
		}
		if strings.EqualFold(firstNonBlankString(item["family"]), "webhook") {
			if eventName := firstNonBlankString(item["eventName"]); eventName != "" {
				item["endpoint"] = map[string]any{
					"method": "POST",
					"url":    flowWebhookRuntimeURL(app, installationID, flowSlug, eventName),
					"auth":   "webhook-auth",
				}
			}
		}
		out = append(out, pruneEmptyStrings(item))
	}
	return out
}

func flowInterfaceRuntimeURL(app *App, installationID string, flowSlug string, interfaceID string) string {
	ensureAPIURL(app)
	path := fmt.Sprintf("/api/workspaces/%s/flows/%s/installations/%s/interfaces/%s",
		url.PathEscape(app.WorkspaceID),
		url.PathEscape(strings.TrimSpace(flowSlug)),
		url.PathEscape(strings.TrimSpace(installationID)),
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
		item := map[string]any{
			"family":        "mcp",
			"toolName":      firstNonBlankString(iface["toolName"], iface["tool-name"]),
			"invocationId":  invocationID,
			"target":        target,
			"description":   firstNonBlankString(iface["description"]),
			"invocation":    invocationContract(invocations, invocationID),
			"runtimeStatus": "not_implemented",
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
