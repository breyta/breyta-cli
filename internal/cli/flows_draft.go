package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newFlowsDraftCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "draft",
		Short: "Draft-only flow commands",
	}
	cmd.AddCommand(newFlowsDraftShowCmd(app))
	cmd.AddCommand(newFlowsDraftRunCmd(app))
	cmd.AddCommand(newFlowsDraftBindingsCmd(app))
	return cmd
}

func newFlowsDraftShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Show the draft version of a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Draft show requires --api/BREYTA_API_URL")
			}
			return doAPICommand(cmd, app, "flows.get", map[string]any{
				"flowSlug": args[0],
				"source":   "draft",
			})
		},
	}
	return cmd
}

func newFlowsDraftRunCmd(app *App) *cobra.Command {
	var inputJSON string
	var wait bool
	var timeout time.Duration
	var poll time.Duration
	cmd := &cobra.Command{
		Use:   "run <flow-slug>",
		Short: "Run a draft flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Draft runs require --api/BREYTA_API_URL")
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"flowSlug": args[0],
				"source":   "draft",
			}
			if strings.TrimSpace(inputJSON) != "" {
				var v any
				if err := json.Unmarshal([]byte(inputJSON), &v); err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
				}
				m, ok := v.(map[string]any)
				if !ok {
					return writeErr(cmd, errors.New("--input must be a JSON object"))
				}
				payload["input"] = m
			}
			client := apiClient(app)
			startResp, status, err := client.DoCommand(context.Background(), "runs.start", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if !wait || status >= 400 {
				return writeAPIResult(cmd, app, startResp, status)
			}

			dataAny := startResp["data"]
			data, _ := dataAny.(map[string]any)
			workflowID := workflowIDFromRunData(data)
			if strings.TrimSpace(workflowID) == "" {
				return writeErr(cmd, errors.New("missing data.workflowId in runs.start response"))
			}
			deadline := time.Now().Add(timeout)
			for {
				execResp, execStatus, err := client.DoCommand(context.Background(), "runs.get", map[string]any{"workflowId": workflowID})
				if err != nil {
					return writeErr(cmd, err)
				}
				if execStatus == 404 {
					if time.Now().After(deadline) {
						return writeAPIResult(cmd, app, execResp, execStatus)
					}
					time.Sleep(poll)
					continue
				}
				if execStatus >= 400 {
					return writeAPIResult(cmd, app, execResp, execStatus)
				}
				execDataAny := execResp["data"]
				execData, _ := execDataAny.(map[string]any)
				runAny := execData["run"]
				run, _ := runAny.(map[string]any)
				statusStr, _ := run["status"].(string)
				if statusStr == "completed" || statusStr == "failed" || statusStr == "cancelled" || statusStr == "canceled" || statusStr == "terminated" || statusStr == "timed-out" || statusStr == "timed_out" {
					return writeAPIResult(cmd, app, execResp, execStatus)
				}
				if time.Now().After(deadline) {
					timeoutOut := map[string]any{
						"ok": false,
						"error": map[string]any{
							"message": fmt.Sprintf("timed out waiting for run completion (workflowId=%s)", workflowID),
						},
						"data": map[string]any{"workflowId": workflowID},
					}
					return writeAPIResult(cmd, app, timeoutOut, 408)
				}
				time.Sleep(poll)
			}
		},
	}
	cmd.Flags().StringVar(&inputJSON, "input", "", "Input JSON object")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run completion")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "Wait timeout (API mode only)")
	cmd.Flags().DurationVar(&poll, "poll", 1*time.Second, "Polling interval when waiting (API mode only)")
	return cmd
}

func newFlowsDraftBindingsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bindings",
		Short: "Manage draft bindings",
	}
	cmd.AddCommand(newFlowsDraftBindingsTemplateCmd(app))
	cmd.AddCommand(newFlowsDraftBindingsApplyCmd(app))
	cmd.AddCommand(newFlowsDraftBindingsShowCmd(app))
	return cmd
}

func newFlowsDraftBindingsTemplateCmd(app *App) *cobra.Command {
	var outPath string
	var clean bool
	cmd := &cobra.Command{
		Use:   "template <flow-slug>",
		Short: "Generate a profile template (EDN) for draft bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("draft bindings template requires API mode"))
			}
			return renderProfileTemplate(cmd, app, args[0], outPath, "draft", !clean)
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "Write template to a file")
	cmd.Flags().BoolVar(&clean, "clean", false, "Generate a template without current bindings")
	return cmd
}

func newFlowsDraftBindingsApplyCmd(app *App) *cobra.Command {
	var setArgs []string
	cmd := &cobra.Command{
		Use:   "apply <flow-slug> @draft.edn",
		Short: "Set draft bindings using a profile file",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Draft bindings require --api/BREYTA_API_URL")
			}
			if len(args) < 2 && len(setArgs) == 0 {
				return writeErr(cmd, errors.New("missing profile file or --set (use @draft.edn or --set)"))
			}
			body := map[string]any{
				"flowSlug": args[0],
				"inputs":   map[string]any{},
			}
			if len(args) >= 2 {
				profileArg := args[1]
				payload, err := parseProfileArg(profileArg)
				if err != nil {
					return writeErr(cmd, err)
				}
				if payload.ProfileType != "" && payload.ProfileType != "draft" {
					return writeErr(cmd, errors.New("profile.type must be draft for draft bindings"))
				}
				body["inputs"] = payload.Inputs
			}
			if len(setArgs) > 0 {
				setInputs, err := parseSetAssignments(setArgs)
				if err != nil {
					return writeErr(cmd, err)
				}
				inputs := body["inputs"].(map[string]any)
				for k, v := range setInputs {
					inputs[k] = v
				}
			}
			return doAPICommand(cmd, app, "profiles.draft.bindings", body)
		},
	}
	cmd.Flags().StringArrayVar(&setArgs, "set", nil, "Set binding or activation input (slot.field=value or activation.field=value)")
	return cmd
}

func newFlowsDraftBindingsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Inspect draft bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Draft bindings show requires --api/BREYTA_API_URL")
			}
			return doAPICommand(cmd, app, "profiles.status", map[string]any{
				"flowSlug":    args[0],
				"profileType": "draft",
			})
		},
	}
	return cmd
}
