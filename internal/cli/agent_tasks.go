package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const agentTasksMaxOutputBytes = 8000

func newAgentTasksCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "agent-tasks", Short: "Manage agent tasks"}
	cmd.AddCommand(newAgentTasksListCmd(app))
	cmd.AddCommand(newAgentTasksShowCmd(app))
	cmd.AddCommand(newAgentTasksCompleteCmd(app))
	cmd.AddCommand(newAgentTasksRunCmd(app))
	return cmd
}

func newAgentTasksListCmd(app *App) *cobra.Command {
	var flowSlug string
	var limit int
	var cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			q := url.Values{}
			if strings.TrimSpace(flowSlug) != "" {
				q.Set("flowSlug", strings.TrimSpace(flowSlug))
			}
			if strings.TrimSpace(cursor) != "" {
				q.Set("cursor", strings.TrimSpace(cursor))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/agent/tasks", q, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Filter by flow slug")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results per page")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor")
	return cmd
}

func newAgentTasksShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "show <task-id>", Short: "Show agent task", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAPI(app); err != nil {
			return writeErr(cmd, err)
		}
		taskID := strings.TrimSpace(args[0])
		if taskID == "" {
			return writeErr(cmd, errors.New("missing task id"))
		}
		out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/agent/tasks/"+url.PathEscape(taskID), nil, nil)
		if err != nil {
			return writeErr(cmd, err)
		}
		return writeREST(cmd, app, status, out)
	}}
	return cmd
}

func newAgentTasksCompleteCmd(app *App) *cobra.Command {
	var payload string
	cmd := &cobra.Command{Use: "complete <task-id>", Short: "Complete agent task", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
		out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/agent/tasks/"+url.PathEscape(taskID)+"/complete", nil, body)
		if err != nil {
			return writeErr(cmd, err)
		}
		return writeREST(cmd, app, status, out)
	}}
	cmd.Flags().StringVar(&payload, "payload", "", "JSON object payload")
	return cmd
}

func newAgentTasksRunCmd(app *App) *cobra.Command {
	var flowSlug string
	var limit int
	var poll time.Duration
	var execPath string
	var execArgs []string
	var once bool
	var taskTimeout time.Duration
	var workdir string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run agent tasks with an external executor",
		Long: strings.TrimSpace(`
Run an external command for each agent task.

The task JSON is passed on stdin and written to a temp file.
The temp path is available as BREYTA_AGENT_TASK_FILE.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			execPath = strings.TrimSpace(execPath)
			if execPath == "" {
				return writeErr(cmd, errors.New("missing --exec"))
			}
			if limit <= 0 {
				limit = 1
			}
			if poll <= 0 {
				poll = 2 * time.Second
			}
			if strings.TrimSpace(workdir) != "" {
				if err := ensureDirExists(workdir); err != nil {
					return writeErr(cmd, err)
				}
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			processed := 0
			for {
				if ctx.Err() != nil {
					return nil
				}
				items, err := fetchAgentTasks(ctx, app, flowSlug, limit, "")
				if err != nil {
					return writeErr(cmd, err)
				}
				if len(items) == 0 {
					if once {
						return writeData(cmd, app, map[string]any{"processed": processed}, map[string]any{"items": []any{}})
					}
					select {
					case <-ctx.Done():
						return nil
					case <-time.After(poll):
						continue
					}
				}

				for _, task := range items {
					if ctx.Err() != nil {
						return nil
					}
					taskID := getStringField(task, "taskId")
					if taskID == "" {
						continue
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "agent-tasks: running %s\n", taskID)
					payload, runErr := runAgentTaskExecutor(ctx, execPath, execArgs, task, taskTimeout, workdir)
					if runErr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "agent-tasks: task %s failed: %v\n", taskID, runErr)
					}
					if err := completeAgentTask(ctx, app, taskID, payload); err != nil {
						return writeErr(cmd, err)
					}
					processed++
					if once {
						return writeData(cmd, app, map[string]any{"processed": processed}, map[string]any{"taskId": taskID, "result": payload})
					}
				}
			}
		},
	}
	cmd.Flags().StringVar(&execPath, "exec", "", "Executable to run for each task")
	cmd.Flags().StringArrayVar(&execArgs, "exec-arg", nil, "Argument to pass to --exec (repeatable)")
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Filter by flow slug")
	cmd.Flags().IntVar(&limit, "limit", 1, "Max tasks fetched per poll")
	cmd.Flags().DurationVar(&poll, "poll", 2*time.Second, "Polling interval when no tasks are found")
	cmd.Flags().DurationVar(&taskTimeout, "task-timeout", 30*time.Minute, "Max time to run each task")
	cmd.Flags().StringVar(&workdir, "workdir", "", "Working directory for the executor")
	cmd.Flags().BoolVar(&once, "once", false, "Process at most one task then exit")
	return cmd
}

func fetchAgentTasks(ctx context.Context, app *App, flowSlug string, limit int, cursor string) ([]map[string]any, error) {
	q := url.Values{}
	if strings.TrimSpace(flowSlug) != "" {
		q.Set("flowSlug", strings.TrimSpace(flowSlug))
	}
	if strings.TrimSpace(cursor) != "" {
		q.Set("cursor", strings.TrimSpace(cursor))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	out, status, err := apiClient(app).DoREST(ctx, http.MethodGet, "/api/agent/tasks", q, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		if m, ok := out.(map[string]any); ok {
			return nil, fmt.Errorf("api error (status=%d): %s", status, formatAPIError(m))
		}
		return nil, fmt.Errorf("api error (status=%d)", status)
	}
	m, ok := out.(map[string]any)
	if !ok {
		return nil, errors.New("unexpected response from agent tasks")
	}
	itemsAny, ok := m["items"].([]any)
	if !ok {
		return nil, errors.New("unexpected agent tasks payload")
	}
	items := make([]map[string]any, 0, len(itemsAny))
	for _, it := range itemsAny {
		if task, ok := it.(map[string]any); ok {
			items = append(items, task)
		}
	}
	return items, nil
}

func runAgentTaskExecutor(ctx context.Context, execPath string, execArgs []string, task map[string]any, timeout time.Duration, workdir string) (map[string]any, error) {
	payload := map[string]any{}
	if task == nil {
		payload["error"] = map[string]any{"message": "missing task payload"}
		return payload, errors.New("missing task payload")
	}
	data, err := json.Marshal(task)
	if err != nil {
		payload["error"] = map[string]any{"message": "failed to encode task"}
		return payload, err
	}
	file, err := os.CreateTemp("", "breyta-agent-task-*.json")
	if err != nil {
		payload["error"] = map[string]any{"message": "failed to create task file"}
		return payload, err
	}
	defer func() {
		_ = os.Remove(file.Name())
	}()
	if _, err := file.Write(data); err != nil {
		payload["error"] = map[string]any{"message": "failed to write task file"}
		_ = file.Close()
		return payload, err
	}
	_ = file.Close()

	runCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, execPath, execArgs...)
	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = filepath.Clean(workdir)
	}
	cmd.Stdin = bytes.NewReader(data)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), agentTaskEnv(task, file.Name())...)

	runErr := cmd.Run()
	if runErr != nil {
		payload = failurePayload(runErr, stdout.String(), stderr.String())
		return payload, runErr
	}

	payload = normalizeExecutorOutput(stdout.Bytes())
	return payload, nil
}

func agentTaskEnv(task map[string]any, taskFile string) []string {
	env := []string{}
	if taskFile != "" {
		env = append(env, "BREYTA_AGENT_TASK_FILE="+taskFile)
	}
	if id := getStringField(task, "taskId"); id != "" {
		env = append(env, "BREYTA_AGENT_TASK_ID="+id)
	}
	if workflowID := getStringField(task, "workflowId"); workflowID != "" {
		env = append(env, "BREYTA_AGENT_WORKFLOW_ID="+workflowID)
	}
	if flowSlug := getStringField(task, "flowSlug"); flowSlug != "" {
		env = append(env, "BREYTA_AGENT_FLOW_SLUG="+flowSlug)
	}
	return env
}

func completeAgentTask(ctx context.Context, app *App, taskID string, payload map[string]any) error {
	if strings.TrimSpace(taskID) == "" {
		return errors.New("missing task id")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	out, status, err := apiClient(app).DoREST(ctx, http.MethodPost, "/api/agent/tasks/"+url.PathEscape(taskID)+"/complete", nil, payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		if m, ok := out.(map[string]any); ok {
			return fmt.Errorf("api error (status=%d): %s", status, formatAPIError(m))
		}
		return fmt.Errorf("api error (status=%d)", status)
	}
	return nil
}

func normalizeExecutorOutput(stdout []byte) map[string]any {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return map[string]any{"ok": true}
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err == nil {
		if m, ok := v.(map[string]any); ok {
			return m
		}
		return map[string]any{"result": v}
	}
	return map[string]any{"raw": string(trimmed)}
}

func failurePayload(err error, stdout string, stderr string) map[string]any {
	payload := map[string]any{
		"status": "failed",
		"error": map[string]any{
			"message": err.Error(),
		},
	}
	if exitCode := exitCodeFromError(err); exitCode != 0 {
		payload["error"].(map[string]any)["exitCode"] = exitCode
	}
	if stdout = truncateOutput(stdout); stdout != "" {
		payload["stdout"] = stdout
	}
	if stderr = truncateOutput(stderr); stderr != "" {
		payload["stderr"] = stderr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		payload["error"].(map[string]any)["type"] = "timeout"
	}
	return payload
}

func truncateOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= agentTasksMaxOutputBytes {
		return s
	}
	return s[:agentTasksMaxOutputBytes] + "..."
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 0
}

func getStringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func ensureDirExists(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("missing path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workdir is not a directory: %s", path)
	}
	return nil
}
