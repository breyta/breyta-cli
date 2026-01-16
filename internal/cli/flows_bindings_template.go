package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"olympos.io/encoding/edn"

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
	template, err := buildProfileTemplate(reqsAny, profileType, bindingValues)
	if err != nil {
		return writeErr(cmd, err)
	}
	if profileType == "prod" {
		if profile, ok := template["profile"].(map[string]any); ok {
			delete(profile, "type")
		}
	}
	commentHeader := buildTemplateComments(reqsAny, activationURL(app, slug))
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

func buildTemplateComments(requirements []any, activationURL string) string {
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
