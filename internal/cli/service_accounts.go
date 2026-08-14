package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newServiceAccountsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-accounts",
		Short: "Manage workspace service accounts and machine API keys",
		Long: strings.TrimSpace(`
Use service accounts to provision non-interactive applications and integrations.

- create a workspace-owned service account
- scope it with explicit capabilities
- mint or revoke API keys on that principal
- use the API key for unattended CLI or API access

Common patterns:
- explicit domain scopes such as flows.read or resources.write
- workspace.full for the full known service-account scope matrix
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServiceAccountsListCmd(app))
	cmd.AddCommand(newServiceAccountsShowCmd(app))
	cmd.AddCommand(newServiceAccountsCreateCmd(app))
	cmd.AddCommand(newServiceAccountsUpdateCmd(app))
	cmd.AddCommand(newServiceAccountsKeysCmd(app))
	return cmd
}

func newServiceAccountsListCmd(app *App) *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List workspace service accounts",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if trimmed := strings.TrimSpace(status); trimmed != "" {
				normalized, err := normalizeServiceAccountStatus(trimmed)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["status"] = normalized
			}
			query := url.Values{}
			if normalized, _ := payload["status"].(string); normalized != "" {
				query.Set("status", normalized)
			}
			out, httpStatus, err := serviceAccountREST(cmd, app, http.MethodGet, "/api/service-accounts", query, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, httpStatus, filterServiceAccountsByStatus(out, query.Get("status")))
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (active|disabled)")
	return cmd
}

func newServiceAccountsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show <service-account-id>",
		Aliases: []string{"get"},
		Short:   "Show one service account",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceAccountID := strings.TrimSpace(args[0])
			if serviceAccountID == "" {
				return writeErr(cmd, errors.New("missing service account id"))
			}
			out, status, err := serviceAccountREST(cmd, app, http.MethodGet, "/api/service-accounts", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= http.StatusBadRequest {
				return writeREST(cmd, app, status, out)
			}
			account := findServiceAccount(out, serviceAccountID)
			if account == nil {
				return writeErr(cmd, fmt.Errorf("service account %q not found", serviceAccountID))
			}
			return writeREST(cmd, app, http.StatusOK, account)
		},
	}
	return cmd
}

func newServiceAccountsCreateCmd(app *App) *cobra.Command {
	var name string
	var status string
	var scopes []string
	var metadataJSON string
	var metadataFile string

	cmd := &cobra.Command{
		Use:   "create --name <name>",
		Short: "Create a workspace service account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name = strings.TrimSpace(name)
			if name == "" {
				return writeErr(cmd, errors.New("missing --name"))
			}
			metadata, err := parseJSONObjectJSONInput(metadataJSON, metadataFile, "metadata")
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{"name": name}
			if trimmed := strings.TrimSpace(status); trimmed != "" {
				normalized, err := normalizeServiceAccountStatus(trimmed)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["status"] = normalized
			}
			if len(scopes) > 0 {
				payload["capabilities"] = trimStringList(scopes)
			}
			if metadata != nil {
				payload["metadata"] = metadata
			}
			out, status, err := serviceAccountREST(cmd, app, http.MethodPost, "/api/service-accounts", nil, payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Display name for the service account")
	cmd.Flags().StringVar(&status, "status", "", "Initial status (active|disabled)")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "Scope to grant (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&scopes, "capability", nil, "Compatibility alias for --scope")
	_ = cmd.Flags().MarkHidden("capability")
	cmd.Flags().StringVar(&metadataJSON, "metadata", "", "Metadata JSON object")
	cmd.Flags().StringVar(&metadataFile, "metadata-file", "", "Path to a JSON file containing metadata")
	return cmd
}

func newServiceAccountsUpdateCmd(app *App) *cobra.Command {
	var name string
	var status string
	var scopes []string
	var metadataJSON string
	var metadataFile string

	cmd := &cobra.Command{
		Use:   "update <service-account-id>",
		Short: "Update a workspace service account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceAccountID := strings.TrimSpace(args[0])
			if serviceAccountID == "" {
				return writeErr(cmd, errors.New("missing service account id"))
			}
			metadata, err := parseJSONObjectJSONInput(metadataJSON, metadataFile, "metadata")
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{}
			mutated := false
			if cmd.Flags().Changed("name") {
				payload["name"] = strings.TrimSpace(name)
				mutated = true
			}
			if cmd.Flags().Changed("status") {
				normalized, err := normalizeServiceAccountStatus(status)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["status"] = normalized
				mutated = true
			}
			if cmd.Flags().Changed("scope") || cmd.Flags().Changed("capability") {
				payload["capabilities"] = trimStringList(scopes)
				mutated = true
			}
			if cmd.Flags().Changed("metadata") || cmd.Flags().Changed("metadata-file") {
				payload["metadata"] = metadata
				mutated = true
			}
			if !mutated {
				return writeErr(cmd, errors.New("no updates provided"))
			}
			out, httpStatus, err := serviceAccountREST(cmd, app, http.MethodPatch, "/api/service-accounts/"+url.PathEscape(serviceAccountID), nil, payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, httpStatus, out)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Updated display name")
	cmd.Flags().StringVar(&status, "status", "", "Updated status (active|disabled)")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "Replace scopes with the provided values (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&scopes, "capability", nil, "Compatibility alias for --scope")
	_ = cmd.Flags().MarkHidden("capability")
	cmd.Flags().StringVar(&metadataJSON, "metadata", "", "Replacement metadata JSON object")
	cmd.Flags().StringVar(&metadataFile, "metadata-file", "", "Path to a JSON file containing replacement metadata")
	return cmd
}

func newServiceAccountsKeysCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage API keys for one service account",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServiceAccountsKeysListCmd(app))
	cmd.AddCommand(newServiceAccountsKeysCreateCmd(app))
	cmd.AddCommand(newServiceAccountsKeysRevokeCmd(app))
	return cmd
}

func newServiceAccountsKeysListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list <service-account-id>",
		Aliases: []string{"ls"},
		Short:   "List API keys for a service account",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceAccountID := strings.TrimSpace(args[0])
			if serviceAccountID == "" {
				return writeErr(cmd, errors.New("missing service account id"))
			}
			out, status, err := serviceAccountREST(cmd, app, http.MethodGet, "/api/service-accounts/"+url.PathEscape(serviceAccountID)+"/keys", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	return cmd
}

func newServiceAccountsKeysCreateCmd(app *App) *cobra.Command {
	var name string
	var expiresAt string
	var metadataJSON string
	var metadataFile string

	cmd := &cobra.Command{
		Use:   "create <service-account-id>",
		Short: "Create a new API key for a service account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceAccountID := strings.TrimSpace(args[0])
			if serviceAccountID == "" {
				return writeErr(cmd, errors.New("missing service account id"))
			}
			metadata, err := parseJSONObjectJSONInput(metadataJSON, metadataFile, "metadata")
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{}
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				payload["name"] = trimmed
			}
			if trimmed := strings.TrimSpace(expiresAt); trimmed != "" {
				payload["expiresAt"] = trimmed
			}
			if metadata != nil {
				payload["metadata"] = metadata
			}
			out, status, err := serviceAccountREST(cmd, app, http.MethodPost, "/api/service-accounts/"+url.PathEscape(serviceAccountID)+"/keys", nil, payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Display name for the API key")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "Optional RFC3339 expiry time")
	cmd.Flags().StringVar(&metadataJSON, "metadata", "", "Metadata JSON object")
	cmd.Flags().StringVar(&metadataFile, "metadata-file", "", "Path to a JSON file containing metadata")
	return cmd
}

func newServiceAccountsKeysRevokeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke <service-account-id> <key-id>",
		Short: "Revoke one API key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceAccountID := strings.TrimSpace(args[0])
			keyID := strings.TrimSpace(args[1])
			if serviceAccountID == "" {
				return writeErr(cmd, errors.New("missing service account id"))
			}
			if keyID == "" {
				return writeErr(cmd, errors.New("missing key id"))
			}
			out, status, err := serviceAccountREST(cmd, app, http.MethodDelete, "/api/service-accounts/"+url.PathEscape(serviceAccountID)+"/keys/"+url.PathEscape(keyID), nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	return cmd
}

func normalizeServiceAccountStatus(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "active":
		return "active", nil
	case "disabled":
		return "disabled", nil
	default:
		return "", fmt.Errorf("invalid --status %q (expected active|disabled)", raw)
	}
}

func trimStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func serviceAccountREST(cmd *cobra.Command, app *App, method, path string, query url.Values, body any) (any, int, error) {
	if err := requireAPI(app); err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()
	return apiClientWithTimeout(app, 60*time.Second).DoREST(ctx, method, path, query, body)
}

func serviceAccountItems(value any) []any {
	root := mapStringAny(value)
	if root == nil {
		return nil
	}
	return sliceAny(firstPresentAny(root["items"], mapStringAny(root["data"])["items"]))
}

func findServiceAccount(value any, serviceAccountID string) map[string]any {
	for _, raw := range serviceAccountItems(value) {
		account := mapStringAny(raw)
		if account != nil && firstNonBlankString(account["id"], account["serviceAccountId"], account["service-account-id"]) == serviceAccountID {
			return account
		}
	}
	return nil
}

func filterServiceAccountsByStatus(value any, status string) any {
	status = strings.TrimSpace(status)
	if status == "" {
		return value
	}
	root := mapStringAny(value)
	if root == nil {
		return value
	}
	filtered := make([]any, 0)
	for _, raw := range serviceAccountItems(value) {
		account := mapStringAny(raw)
		if strings.EqualFold(firstNonBlankString(account["status"]), status) {
			filtered = append(filtered, raw)
		}
	}
	copyRoot := make(map[string]any, len(root))
	for key, item := range root {
		copyRoot[key] = item
	}
	copyRoot["items"] = filtered
	return copyRoot
}
