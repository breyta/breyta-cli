package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const agentEventName = "agent-task"

func newAgentsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "agents", Short: "Manage agent tasks"}

	tasks := &cobra.Command{Use: "tasks", Short: "Manage agent tasks"}
	tasks.AddCommand(newAgentTasksListCmd(app))
	tasks.AddCommand(newAgentTasksCompleteCmd(app))
	cmd.AddCommand(tasks)

	return cmd
}

func newAgentTasksListCmd(app *App) *cobra.Command {
	var workflowID string
	var flow string
	var limit int
	var cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Agent tasks require API mode")
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			q := url.Values{}
			q.Set("eventName", agentEventName)
			if strings.TrimSpace(workflowID) != "" {
				q.Set("workflowId", strings.TrimSpace(workflowID))
			}
			if strings.TrimSpace(flow) != "" {
				q.Set("flow", strings.TrimSpace(flow))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if strings.TrimSpace(cursor) != "" {
				q.Set("cursor", strings.TrimSpace(cursor))
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/waits", q, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&workflowID, "workflow", "", "Filter by workflow id (API mode only)")
	cmd.Flags().StringVar(&flow, "flow", "", "Filter by flow slug (API mode only)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items per page (API mode only)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (API mode only)")
	return cmd
}

func newAgentTasksCompleteCmd(app *App) *cobra.Command {
	var payload string
	cmd := &cobra.Command{
		Use:   "complete <task-id>",
		Short: "Complete agent task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Agent tasks require API mode")
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			taskID := strings.TrimSpace(args[0])
			if taskID == "" {
				return writeErr(cmd, errors.New("missing task id"))
			}
			body := map[string]any{}
			if strings.TrimSpace(payload) != "" {
				var v any
				if err := json.Unmarshal([]byte(payload), &v); err != nil {
					return writeErr(cmd, errors.New("invalid --payload JSON"))
				}
				if m, ok := v.(map[string]any); ok {
					body = m
				} else {
					return writeErr(cmd, errors.New("--payload must be a JSON object"))
				}
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(taskID)+"/complete", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&payload, "payload", "", "Payload (API mode: JSON object)")
	return cmd
}
