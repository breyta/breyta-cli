package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type workspaceMemberItem struct {
	UserID string
	Role   string
	Name   string
	Email  string
	Status string
}

func newWorkspaceMembersCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "members",
		Short: "List canonical workspace members",
	}
	cmd.AddCommand(newWorkspaceMembersListCmd(app))
	return cmd
}

func newWorkspaceMembersListCmd(app *App) *cobra.Command {
	var roleFilter string
	var includePending bool
	var outFormat string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List canonical workspace members",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Use API mode to list canonical workspace members.")
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}

			workspaceID := strings.TrimSpace(app.WorkspaceID)
			if workspaceID == "" {
				return writeErr(cmd, errors.New("missing workspace id (provide --workspace or set BREYTA_WORKSPACE)"))
			}

			normalizedRole, err := normalizeWorkspaceMemberRoleFilter(roleFilter)
			if err != nil {
				return writeErr(cmd, err)
			}
			normalizedFormat, err := normalizeWorkspaceMemberOutputFormat(outFormat)
			if err != nil {
				return writeErr(cmd, err)
			}

			query := url.Values{}
			if normalizedRole != "" {
				query.Set("role", normalizedRole)
			}
			if includePending {
				query.Set("include-pending", "true")
			}

			out, status, err := doWorkspaceMembersRootREST(
				cmd,
				app,
				http.MethodGet,
				"/api/workspaces/"+url.PathEscape(workspaceID)+"/members",
				query,
				nil,
			)
			if err != nil {
				return writeFailure(cmd, app, "workspace_members_list_failed", err, "Check workspace access and API availability.", out)
			}

			if normalizedFormat == "json" {
				return writeData(cmd, app, map[string]any{"httpStatus": status}, out)
			}

			items, err := decodeWorkspaceMemberItems(out)
			if err != nil {
				return writeFailure(cmd, app, "workspace_members_decode_failed", err, "Expected a member list response from the API.", out)
			}
			return writeWorkspaceMembersTable(cmd.OutOrStdout(), items, responseIncludesPendingMembers(out, includePending))
		},
	}

	cmd.Flags().StringVar(&roleFilter, "role", "", "Filter by role (admin|member|creator|billing|user)")
	cmd.Flags().BoolVar(&includePending, "include-pending", false, "Include pending/invited members")
	cmd.Flags().StringVar(&outFormat, "format", "table", "Output format (table|json)")
	return cmd
}

func doWorkspaceMembersRootREST(cmd *cobra.Command, app *App, method, path string, query url.Values, body any) (map[string]any, int, error) {
	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	out, status, err := authClient(app).DoRootREST(ctx, method, path, query, body)
	if err != nil {
		return nil, status, err
	}

	m, ok := out.(map[string]any)
	if !ok {
		return map[string]any{"raw": out}, status, fmt.Errorf("unexpected response (status=%d)", status)
	}
	if status >= http.StatusBadRequest {
		return m, status, errors.New(formatWorkspaceMembersAPIError(m, status))
	}
	return m, status, nil
}

func formatWorkspaceMembersAPIError(out map[string]any, status int) string {
	if out != nil {
		if errAny, ok := out["error"]; ok {
			switch v := errAny.(type) {
			case string:
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					return fmt.Sprintf("api error (status=%d): %s", status, trimmed)
				}
			case map[string]any:
				if msg, _ := v["message"].(string); strings.TrimSpace(msg) != "" {
					return fmt.Sprintf("api error (status=%d): %s", status, strings.TrimSpace(msg))
				}
			}
		}
	}
	return fmt.Sprintf("api error (status=%d)", status)
}

func normalizeWorkspaceMemberRoleFilter(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", nil
	case "admin", "member", "creator", "billing", "user":
		return strings.ToLower(strings.TrimSpace(raw)), nil
	default:
		return "", fmt.Errorf("invalid --role %q (expected admin|member|creator|billing|user)", raw)
	}
}

func normalizeWorkspaceMemberOutputFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "table":
		return "table", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("invalid --format %q (expected table|json)", raw)
	}
}

func decodeWorkspaceMemberItems(out map[string]any) ([]workspaceMemberItem, error) {
	rawItems, ok := out["items"]
	if !ok {
		return nil, errors.New("missing items field")
	}
	itemsAny, ok := rawItems.([]any)
	if !ok {
		return nil, fmt.Errorf("items field has type %T", rawItems)
	}

	items := make([]workspaceMemberItem, 0, len(itemsAny))
	for _, raw := range itemsAny {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("member item has type %T", raw)
		}
		item := workspaceMemberItem{
			UserID: toString(itemMap["userId"]),
			Role:   toString(itemMap["role"]),
			Name:   toString(itemMap["name"]),
			Email:  toString(itemMap["email"]),
			Status: toString(itemMap["status"]),
		}
		items = append(items, item)
	}
	return items, nil
}

func responseIncludesPendingMembers(out map[string]any, fallback bool) bool {
	if includePending, ok := out["includePending"].(bool); ok {
		return includePending
	}
	return fallback
}

func writeWorkspaceMembersTable(w io.Writer, items []workspaceMemberItem, includeStatus bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if includeStatus {
		if _, err := io.WriteString(tw, "userId\trole\tname\temail\tstatus\n"); err != nil {
			return err
		}
	} else {
		if _, err := io.WriteString(tw, "userId\trole\tname\temail\n"); err != nil {
			return err
		}
	}

	for _, item := range items {
		if includeStatus {
			if _, err := fmt.Fprintf(
				tw,
				"%s\t%s\t%s\t%s\t%s\n",
				workspaceMembersTableCell(item.UserID),
				workspaceMembersTableCell(item.Role),
				workspaceMembersTableCell(item.Name),
				workspaceMembersTableCell(item.Email),
				workspaceMembersTableCell(item.Status),
			); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\n",
			workspaceMembersTableCell(item.UserID),
			workspaceMembersTableCell(item.Role),
			workspaceMembersTableCell(item.Name),
			workspaceMembersTableCell(item.Email),
		); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func workspaceMembersTableCell(value string) string {
	replacer := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return strings.TrimSpace(replacer.Replace(value))
}
