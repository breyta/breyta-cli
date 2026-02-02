package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"olympos.io/encoding/edn"

	"github.com/breyta/breyta-cli/internal/api"

	"github.com/spf13/cobra"
)

func newFlowsBindingsTemplateCmd(app *App) *cobra.Command {
	var outPath string
	var clean bool
	cmd := &cobra.Command{
		Use:   "template <flow-slug>",
		Short: "Generate a profile template (EDN) for prod bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows bindings template requires API mode"))
			}
			return renderProfileTemplate(cmd, app, args[0], outPath, "prod", !clean)
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "Write template to a file")
	cmd.Flags().BoolVar(&clean, "clean", false, "Generate a template without current bindings")
	return cmd
}

func renderProfileTemplate(cmd *cobra.Command, app *App, slug string, outPath string, profileType string, includeBindings bool) error {
	client := apiClient(app)
	resp, status, err := client.DoCommand(context.Background(), "profiles.template", map[string]any{
		"flowSlug":    slug,
		"profileType": profileType,
	})
	if err != nil {
		return writeErr(cmd, err)
	}
	if status >= 400 || !isOK(resp) {
		return writeAPIResult(cmd, app, resp, status)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		return writeErr(cmd, errors.New("template response missing data"))
	}
	reqsAny, ok := data["requirements"].([]any)
	if !ok {
		return writeErr(cmd, errors.New("template response missing requirements"))
	}
	var bindingValues map[string]any
	if includeBindings {
		statusResp, statusCode, statusErr := client.DoCommand(context.Background(), "profiles.status", map[string]any{
			"flowSlug":    slug,
			"profileType": profileType,
		})
		if statusErr != nil {
			return writeErr(cmd, statusErr)
		}
		if statusCode >= 400 || !isOK(statusResp) {
			return writeAPIResult(cmd, app, statusResp, statusCode)
		}
		statusData, _ := statusResp["data"].(map[string]any)
		if statusData != nil {
			if values, ok := statusData["bindingValues"].(map[string]any); ok {
				bindingValues = normalizeBindingValues(values)
			}
		}
	}

	var connectionsByType map[string][]connectionSummary
	var connectionCommentLines []string
	if includeBindings {
		connectionsByType, err = listConnectionsByType(client, reqsAny)
		if err != nil {
			return writeErr(cmd, err)
		}
		connectionCommentLines = buildConnectionsCommentLines(connectionsByType, reqsAny)
	}

	template, err := buildProfileTemplate(reqsAny, profileType, bindingValues)
	if err != nil {
		return writeErr(cmd, err)
	}

	if includeBindings && connectionsByType != nil {
		applyDefaultConnectionReuse(template, reqsAny, connectionsByType)
	}

	if profileType == "prod" {
		if profile, ok := template["profile"].(map[string]any); ok {
			delete(profile, "type")
		}
	}
	commentHeader := buildTemplateComments(reqsAny, activationURL(app, slug), connectionCommentLines)
	out, err := edn.Marshal(ednifyKeys(template))
	if err != nil {
		return writeErr(cmd, err)
	}
	out = append([]byte(commentHeader), out...)
	if strings.TrimSpace(outPath) != "" {
		if err := atomicWriteFile(outPath, out, 0o644); err != nil {
			return writeErr(cmd, err)
		}
		printActivationURL(cmd, app, slug)
		return writeData(cmd, app, nil, map[string]any{"path": outPath, "ok": true})
	}
	printActivationURL(cmd, app, slug)
	_, _ = cmd.OutOrStdout().Write(out)
	return nil
}

func printActivationURL(cmd *cobra.Command, app *App, slug string) {
	url := activationURL(app, slug)
	if url == "" {
		return
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Activation URL: %s\n", url)
}

func buildTemplateComments(requirements []any, activationURL string, extraLines []string) string {
	lines := []string{}
	if strings.TrimSpace(activationURL) != "" {
		lines = append(lines, fmt.Sprintf("Activation URL: %s", activationURL))
	}
	oauthSlots := collectSlots(requirements, func(req map[string]any) bool {
		return req["oauth"] != nil
	})
	if len(oauthSlots) > 0 {
		lines = append(lines, fmt.Sprintf("OAuth slots (complete in UI): %s", strings.Join(oauthSlots, ", ")))
	}
	secretSlots := collectSlots(requirements, func(req map[string]any) bool {
		return strings.TrimPrefix(strings.ToLower(toString(req["type"])), ":") == "secret"
	})
	if len(secretSlots) > 0 {
		lines = append(lines, fmt.Sprintf("Secret slots (set :secret, not :generate): %s", strings.Join(secretSlots, ", ")))
	}
	for _, line := range extraLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(";; ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func collectSlots(requirements []any, predicate func(map[string]any) bool) []string {
	slots := []string{}
	seen := map[string]struct{}{}
	for _, raw := range requirements {
		req, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if !predicate(req) {
			continue
		}
		slot := strings.TrimSpace(strings.TrimPrefix(toString(req["slot"]), ":"))
		if slot == "" {
			continue
		}
		if _, exists := seen[slot]; exists {
			continue
		}
		seen[slot] = struct{}{}
		slots = append(slots, slot)
	}
	return slots
}

func normalizeBindingValues(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := map[string]any{}
	for rawKey, value := range values {
		key := strings.TrimSpace(strings.TrimPrefix(rawKey, ":"))
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

type connectionSummary struct {
	ID   string
	Name string
	Type string
}

func normalizeConnectionType(v any) string {
	s := strings.TrimSpace(toString(v))
	s = strings.TrimPrefix(s, ":")
	return strings.ToLower(strings.TrimSpace(s))
}

func getConnectionID(item map[string]any) string {
	// flows-api tends to use "connection-id" while mocks use "id".
	for _, k := range []string{"connection-id", "connectionId", "id"} {
		if v, ok := item[k]; ok {
			if s := strings.TrimSpace(toString(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

func getConnectionName(item map[string]any) string {
	if s := strings.TrimSpace(toString(item["name"])); s != "" {
		return s
	}
	return ""
}

func listConnectionsByType(client api.Client, requirements []any) (map[string][]connectionSummary, error) {
	types := map[string]struct{}{}
	for _, raw := range requirements {
		req, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := strings.ToLower(toString(req["kind"]))
		if kind == "form" {
			continue
		}
		t := normalizeConnectionType(req["type"])
		if t == "" || t == "secret" {
			continue
		}
		types[t] = struct{}{}
	}
	if len(types) == 0 {
		return nil, nil
	}

	out := map[string][]connectionSummary{}
	for typ := range types {
		q := url.Values{}
		q.Set("type", typ)
		q.Set("limit", "200")
		respAny, status, err := client.DoREST(context.Background(), http.MethodGet, "/api/connections", q, nil)
		if err != nil {
			return nil, err
		}
		// Older/local mock surfaces may not implement the REST connections endpoints.
		// Treat 404 as "no connection inventory available" and fall back to the old template behavior.
		if status == http.StatusNotFound {
			return nil, nil
		}
		if status >= 400 {
			return nil, fmt.Errorf("failed to list connections (type=%s): status=%d", typ, status)
		}
		resp, ok := respAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("connections list response not an object (type=%s)", typ)
		}
		itemsAny, _ := resp["items"].([]any)
		for _, raw := range itemsAny {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id := getConnectionID(item)
			if id == "" {
				continue
			}
			out[typ] = append(out[typ], connectionSummary{
				ID:   id,
				Name: getConnectionName(item),
				Type: typ,
			})
		}
	}
	return out, nil
}

func pickPreferredLLMConnection(conns []connectionSummary) (connectionSummary, bool) {
	if len(conns) == 0 {
		return connectionSummary{}, false
	}
	openai := []connectionSummary{}
	for _, c := range conns {
		if strings.Contains(strings.ToLower(c.Name), "openai") {
			openai = append(openai, c)
		}
	}
	if len(openai) == 1 {
		return openai[0], true
	}
	if len(conns) == 1 {
		return conns[0], true
	}
	return connectionSummary{}, false
}

func applyDefaultConnectionReuse(template map[string]any, requirements []any, connectionsByType map[string][]connectionSummary) {
	if template == nil || connectionsByType == nil {
		return
	}
	bindings, ok := template["bindings"].(map[string]any)
	if !ok || bindings == nil {
		return
	}
	llmConns := connectionsByType["llm-provider"]
	pick, ok := pickPreferredLLMConnection(llmConns)
	if !ok {
		return
	}

	for _, raw := range requirements {
		req, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := strings.ToLower(toString(req["kind"]))
		if kind == "form" {
			continue
		}
		if normalizeConnectionType(req["type"]) != "llm-provider" {
			continue
		}
		slot := strings.TrimSpace(toString(req["slot"]))
		slotKey := strings.TrimPrefix(slot, ":")
		if strings.TrimSpace(slotKey) == "" {
			continue
		}
		if existing, ok := bindings[slotKey].(map[string]any); ok && existing != nil {
			if conn := strings.TrimSpace(toString(existing["conn"])); conn != "" {
				continue
			}
		}
		bindings[slotKey] = map[string]any{"conn": pick.ID}
	}
}

func buildConnectionsCommentLines(connectionsByType map[string][]connectionSummary, requirements []any) []string {
	if connectionsByType == nil {
		return nil
	}
	typesInReq := map[string]struct{}{}
	for _, raw := range requirements {
		req, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := strings.ToLower(toString(req["kind"]))
		if kind == "form" {
			continue
		}
		t := normalizeConnectionType(req["type"])
		if t == "" || t == "secret" {
			continue
		}
		typesInReq[t] = struct{}{}
	}
	if len(typesInReq) == 0 {
		return nil
	}

	lines := []string{
		"Tip: prefer reusing existing workspace connections (bind slot.conn=conn-... instead of creating duplicates).",
	}
	for typ := range typesInReq {
		conns := connectionsByType[typ]
		if len(conns) == 0 {
			continue
		}
		parts := make([]string, 0, len(conns))
		for _, c := range conns {
			if strings.TrimSpace(c.Name) != "" {
				parts = append(parts, fmt.Sprintf("%s (%s)", c.ID, c.Name))
			} else {
				parts = append(parts, c.ID)
			}
		}
		lines = append(lines, fmt.Sprintf("Existing %s connections: %s", typ, strings.Join(parts, ", ")))
	}
	return lines
}
