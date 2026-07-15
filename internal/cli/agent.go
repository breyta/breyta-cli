package cli

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func newAgentCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage your Breyta agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAgentSettingsCmd(app))
	cmd.AddCommand(newAgentEmailCmd(app))
	cmd.AddCommand(newAgentContextCmd(app))
	return cmd
}

func newAgentContextCmd(app *App) *cobra.Command {
	var flowSlug string
	var limit int
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Show recent proactive context for an external agent harness",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 25 {
				return writeErr(cmd, errors.New("--limit must be between 1 and 25"))
			}
			query := url.Values{"limit": {fmt.Sprintf("%d", limit)}}
			if flowSlug = strings.TrimSpace(flowSlug); flowSlug != "" {
				query.Set("flowSlug", flowSlug)
			}
			out, status, err := apiClient(app).DoREST(
				cmd.Context(),
				http.MethodGet,
				"/api/proactive-agent/activity",
				query,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Limit context to one flow slug")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum activity and context items (1-25)")
	return cmd
}

func newAgentEmailCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "email",
		Short: "Send email as your Breyta agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAgentEmailSendCmd(app))
	return cmd
}

func newAgentEmailSendCmd(app *App) *cobra.Command {
	var body string
	var subject string
	var dedupeKey string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a deduplicated proactive email to the current user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body = strings.TrimSpace(body)
			if body == "" {
				return writeErr(cmd, errors.New("missing --body"))
			}
			dedupeKey = strings.TrimSpace(dedupeKey)
			if dedupeKey == "" {
				return writeErr(cmd, errors.New("missing --dedupe-key"))
			}
			payload := map[string]any{
				"body":      body,
				"dedupeKey": dedupeKey,
			}
			if subject = strings.TrimSpace(subject); subject != "" {
				payload["subject"] = subject
			}
			return dispatchAgentEmailAPICommand(cmd, app, payload)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "Email body written as a short message from the agent")
	cmd.Flags().StringVar(&subject, "subject", "", "Optional email subject")
	cmd.Flags().StringVar(&dedupeKey, "dedupe-key", "", "Stable key preventing repeat email for the same finding")
	return cmd
}

func newAgentSettingsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Inspect and update proactive agent settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAgentSettingsShowCmd(app))
	cmd.AddCommand(newAgentSettingsUpdateCmd(app))
	return cmd
}

func newAgentSettingsShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "show",
		Aliases: []string{"get"},
		Short:   "Show your proactive agent settings and available checks",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dispatchAgentAPICommand(cmd, app, "proactive_agent.settings.get", map[string]any{})
		},
	}
}

func parseAgentSetting(raw string, flagName string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", flagName)
	}
}

func parseAgentCheck(raw string) (string, bool, error) {
	id, value, ok := strings.Cut(strings.TrimSpace(raw), "=")
	id = strings.TrimSpace(id)
	if !ok || id == "" || strings.TrimSpace(value) == "" {
		return "", false, errors.New("--check must use <check-id>=true|false")
	}
	enabled, err := parseAgentSetting(value, "--check")
	if err != nil {
		return "", false, err
	}
	return id, enabled, nil
}

func newAgentSettingsUpdateCmd(app *App) *cobra.Command {
	var enabled string
	var emailEnabled string
	var checks []string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update your proactive agent settings",
		Args:  cobra.NoArgs,
		Example: strings.TrimSpace(`
breyta agent settings update --check repeated-manual-run=false
breyta agent settings update --enabled=true --email-enabled=false
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if cmd.Flags().Changed("enabled") {
				value, err := parseAgentSetting(enabled, "--enabled")
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["enabled"] = value
			}
			if cmd.Flags().Changed("email-enabled") {
				value, err := parseAgentSetting(emailEnabled, "--email-enabled")
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["emailEnabled"] = value
			}
			if len(checks) > 0 {
				rules := map[string]any{}
				for _, raw := range checks {
					id, value, err := parseAgentCheck(raw)
					if err != nil {
						return writeErr(cmd, err)
					}
					rules[id] = value
				}
				payload["rules"] = rules
			}
			if len(payload) == 0 {
				return writeErr(cmd, errors.New("provide --enabled, --email-enabled, or at least one --check"))
			}
			return dispatchAgentAPICommand(cmd, app, "proactive_agent.settings.update", payload)
		},
	}
	cmd.Flags().StringVar(&enabled, "enabled", "", "Enable or disable proactive agent checks (true|false)")
	cmd.Flags().StringVar(&emailEnabled, "email-enabled", "", "Enable or disable proactive agent email (true|false)")
	cmd.Flags().StringArrayVar(&checks, "check", nil, "Set a check using <check-id>=true|false (repeatable)")
	return cmd
}

func dispatchAgentAPICommand(cmd *cobra.Command, app *App, command string, payload map[string]any) error {
	if useDoAPICommandFn {
		return doAPICommandFn(cmd, app, command, payload)
	}
	return doAPICommand(cmd, app, command, payload)
}

func agentEmailAlreadySent(out map[string]any, status int) bool {
	if status < 200 || status >= 300 {
		return false
	}
	data, ok := out["data"].(map[string]any)
	if !ok {
		return false
	}
	return data["status"] == "skipped" && data["reason"] == "already-emailed"
}

func dispatchAgentEmailAPICommand(cmd *cobra.Command, app *App, payload map[string]any) error {
	if useDoAPICommandFn {
		return doAPICommandFn(cmd, app, "proactive_agent.email.send", payload)
	}
	out, status, err := runAPICommand(app, "proactive_agent.email.send", payload)
	if err != nil {
		return writeErr(cmd, err)
	}
	if agentEmailAlreadySent(out, status) {
		out["ok"] = true
	}
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
}
