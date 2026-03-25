package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func asInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	default:
		return 0
	}
}

func commandWorkspaceID(out map[string]any) string {
	if out == nil {
		return ""
	}
	if v, ok := out["workspaceId"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func doRunCommandWithOptionalWait(cmd *cobra.Command, app *App, command string, payload map[string]any, wait bool, timeout time.Duration, poll time.Duration) error {
	client := apiClient(app)
	startResp, status, err := client.DoCommand(context.Background(), command, payload)
	if err != nil {
		return writeErr(cmd, err)
	}
	startOK := status < 400 && isOK(startResp)
	trackCommandTelemetry(app, command, payload, status, startOK)
	enrichCommandHints(app, command, payload, status, startResp)
	flowSlug, _ := payload["flowSlug"].(string)
	if startOK {
		trackCLIEvent(app, "cli_flow_run_started", nil, app.Token, map[string]any{
			"product":   "flows",
			"channel":   "cli",
			"api_host":  apiHostname(app.APIURL),
			"flow_slug": strings.TrimSpace(flowSlug),
			"command":   strings.TrimSpace(command),
			"wait":      wait,
		})
	}
	if !wait || status >= 400 {
		if err := writeAPIResult(cmd, app, startResp, status); err != nil {
			return writeErr(cmd, err)
		}
		return nil
	}

	data, _ := startResp["data"].(map[string]any)
	workflowID := workflowIDFromRunData(data)
	if strings.TrimSpace(workflowID) == "" {
		return writeErr(cmd, errors.New("missing data.workflowId in start response"))
	}
	installationID := installationIDFromRunData(data)

	deadline := time.Now().Add(timeout)
	for {
		payload := map[string]any{"workflowId": workflowID}
		if installationID != "" {
			payload["installationId"] = installationID
		}
		execResp, execStatus, err := client.DoCommand(context.Background(), "runs.get", payload)
		if err != nil {
			return writeErr(cmd, err)
		}
		if execStatus == 404 {
			if time.Now().After(deadline) {
				if err := writeAPIResult(cmd, app, execResp, execStatus); err != nil {
					return writeErr(cmd, err)
				}
				return nil
			}
			time.Sleep(poll)
			continue
		}
		if execStatus >= 400 {
			if err := writeAPIResult(cmd, app, execResp, execStatus); err != nil {
				return writeErr(cmd, err)
			}
			return nil
		}

		execData, _ := execResp["data"].(map[string]any)
		run, _ := execData["run"].(map[string]any)
		statusStr, _ := run["status"].(string)
		s := strings.ToLower(strings.TrimSpace(statusStr))
		if s == "completed" || s == "failed" || s == "cancelled" || s == "canceled" || s == "terminated" || s == "timed-out" || s == "timed_out" {
			trackCLIEvent(app, "cli_flow_run_completed", nil, app.Token, map[string]any{
				"product":     "flows",
				"channel":     "cli",
				"api_host":    apiHostname(app.APIURL),
				"flow_slug":   strings.TrimSpace(flowSlug),
				"command":     strings.TrimSpace(command),
				"workflow_id": workflowID,
				"run_status":  s,
				"wait":        wait,
			})
			if err := writeAPIResult(cmd, app, execResp, execStatus); err != nil {
				return writeErr(cmd, err)
			}
			return nil
		}

		if time.Now().After(deadline) {
			trackCLIEvent(app, "cli_flow_run_wait_timed_out", nil, app.Token, map[string]any{
				"product":     "flows",
				"channel":     "cli",
				"api_host":    apiHostname(app.APIURL),
				"flow_slug":   strings.TrimSpace(flowSlug),
				"command":     strings.TrimSpace(command),
				"workflow_id": workflowID,
				"wait":        wait,
			})
			timeoutOut := map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": fmt.Sprintf("timed out waiting for run completion (workflowId=%s)", workflowID),
					"details": map[string]any{
						"workflowId": workflowID,
						"timeoutMs":  timeout.Milliseconds(),
						"pollMs":     poll.Milliseconds(),
					},
				},
				"meta": map[string]any{
					"timedOut": true,
					"hint":     "The run may still be in progress. Use `breyta runs show <workflow-id>` to check status.",
				},
				"data": map[string]any{
					"workflowId": workflowID,
					"start":      startResp,
					"lastPoll":   execResp,
				},
			}
			if err := writeAPIResult(cmd, app, timeoutOut, 200); err != nil {
				return writeErr(cmd, err)
			}
			return nil
		}
		time.Sleep(poll)
	}
}

func newFlowsRunCmd(app *App) *cobra.Command {
	var installationID string
	var target string
	var version int
	var inputJSON string
	var wait bool
	var timeout time.Duration
	var poll time.Duration

	cmd := &cobra.Command{
		Use:   "run <flow-slug>",
		Short: "Start a flow run",
		Long: strings.TrimSpace(`
Default:
- breyta flows run <flow-slug> [--input '{...}'] [--wait]

	Advanced targeting:
	- --installation-id <id> : run a specific installation target
	- --target draft|live : select run target explicitly (default draft)
	- --version <n> : force a specific release version for this invocation
	`),
		Example: strings.TrimSpace(`
breyta flows run order-ingest --wait
breyta flows run order-ingest --input '{"region":"EU"}' --wait

	# Advanced
	breyta flows run order-ingest --target live --wait
	breyta flows run order-ingest --target draft --wait
	breyta flows run order-ingest --installation-id prof_123 --wait
	`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "flows run requires --api/BREYTA_API_URL")
			}
			payload := map[string]any{"flowSlug": args[0]}
			if cmd.Flags().Changed("target") {
				resolvedTarget, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["target"] = resolvedTarget
			}
			installationID = strings.TrimSpace(installationID)
			if installationID != "" {
				payload["profileId"] = installationID
			}
			if version > 0 {
				payload["version"] = version
			}
			if strings.TrimSpace(inputJSON) != "" {
				m, err := parseJSONObjectFlag(inputJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
				}
				payload["input"] = m
			}
			return doRunCommandWithOptionalWait(cmd, app, "flows.run", payload, wait, timeout, poll)
		},
	}

	cmd.Flags().StringVar(&installationID, "installation-id", "", "Advanced: run under a specific installation id")
	cmd.Flags().StringVar(&target, "target", "", "Advanced: run target override (draft|live)")
	cmd.Flags().IntVar(&version, "version", 0, "Advanced: release version override")
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object input")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run completion")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Wait timeout")
	cmd.Flags().DurationVar(&poll, "poll", 250*time.Millisecond, "Poll interval while waiting")
	return cmd
}

func newFlowsReleaseCmd(app *App) *cobra.Command {
	var skipPromoteInstallations bool
	var version string
	var deployKey string
	var releaseNote string
	var releaseNoteFile string
	var legacyNote string

	cmd := &cobra.Command{
		Use:   "release <flow-slug>",
		Short: "Activate the latest pushed version, promote live, and promote track-latest installations by default",
		Long: strings.TrimSpace(`
Activate a released flow version for the current workspace.

By default, release reuses the latest version from workspace current, promotes
live, and promotes track-latest installations in the current workspace. Use
--version to activate a specific released version instead. Use
--skip-promote-installations when you want to update live without promoting
end-user installations.

When you know what changed, attach a markdown release note so the activated
version carries operator-facing context:
- breyta flows release my-flow --release-note 'Updated retry policy and fixed idempotency'
- breyta flows release my-flow --release-note-file ./release-note.md
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "release requires --api/BREYTA_API_URL")
			}

			payload := map[string]any{"flowSlug": args[0]}
			version = strings.TrimSpace(version)
			if version != "" && version != "latest" {
				v, err := parsePositiveIntFlag(version)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["version"] = v
			}

			resolvedDeployKey := strings.TrimSpace(deployKey)
			if resolvedDeployKey == "" {
				resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
			}
			if resolvedDeployKey != "" {
				payload["deployKey"] = resolvedDeployKey
			}
			resolvedReleaseNote, err := resolveReleaseNoteInput(releaseNote, legacyNote, releaseNoteFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(resolvedReleaseNote) != "" {
				payload["releaseNote"] = resolvedReleaseNote
			}

			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}

			client := apiClient(app)
			releaseOut, releaseStatus, err := client.DoCommand(context.Background(), "flows.release", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			releaseOK := releaseStatus < 400 && isOK(releaseOut)
			trackCommandTelemetry(app, "flows.release", payload, releaseStatus, releaseOK)
			if !releaseOK {
				if err := writeAPIResult(cmd, app, releaseOut, releaseStatus); err != nil {
					return writeErr(cmd, err)
				}
				return nil
			}

			promotePayload := map[string]any{"flowSlug": args[0], "target": "live"}
			if skipPromoteInstallations {
				promotePayload["scope"] = "live"
			}
			releaseData, _ := releaseOut["data"].(map[string]any)
			if activeVersion := asInt(releaseData["activeVersion"]); activeVersion > 0 {
				promotePayload["version"] = activeVersion
			}
			if resolvedDeployKey != "" {
				promotePayload["deployKey"] = resolvedDeployKey
			}
			promoteOut, promoteStatus, err := client.DoCommand(context.Background(), "flows.promote", promotePayload)
			if err != nil {
				return writeErr(cmd, err)
			}
			promoteOK := promoteStatus < 400 && isOK(promoteOut)
			trackCommandTelemetry(app, "flows.promote", promotePayload, promoteStatus, promoteOK)
			if !promoteOK {
				if err := writeAPIResult(cmd, app, promoteOut, promoteStatus); err != nil {
					return writeErr(cmd, err)
				}
				return nil
			}

			activeVersion := asInt(releaseData["activeVersion"])
			combined := map[string]any{
				"ok": true,
				"workspaceId": func() string {
					if ws := commandWorkspaceID(releaseOut); ws != "" {
						return ws
					}
					return commandWorkspaceID(promoteOut)
				}(),
				"meta": map[string]any{
					"released":   true,
					"installed":  !skipPromoteInstallations,
					"target":     "live",
					"verifyHint": "Live runtime can differ from flow activeVersion. Verify with `breyta flows show <slug> --target live` and `breyta flows run <slug> --target live --wait`.",
					"scope": func() string {
						if skipPromoteInstallations {
							return "live"
						}
						return "all"
					}(),
					"verifyCommands": []string{
						"breyta flows show " + args[0] + " --target live",
						"breyta flows run " + args[0] + " --target live --wait",
					},
				},
				"data": map[string]any{
					"release": releaseOut["data"],
					"install": promoteOut["data"],
				},
			}
			appendEnvelopeHints(combined, releaseNoteHintCommands(args[0], activeVersion)...)
			if err := writeAPIResult(cmd, app, combined, 200); err != nil {
				return writeErr(cmd, err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&skipPromoteInstallations, "skip-promote-installations", false, "Activate the version and promote live, but skip promoting end-user installations")
	cmd.Flags().StringVar(&version, "version", "", "Released version to activate (default latest from workspace current)")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key for guarded flows (default: BREYTA_FLOW_DEPLOY_KEY)")
	cmd.Flags().StringVar(&releaseNote, "release-note", "", "Markdown release note to attach to the activated version")
	cmd.Flags().StringVar(&releaseNoteFile, "release-note-file", "", "Read markdown release note from file")
	cmd.Flags().StringVar(&legacyNote, "note", "", "Deprecated alias for --release-note")
	_ = cmd.Flags().MarkHidden("note")
	return cmd
}

func newFlowsDiffCmd(app *App) *cobra.Command {
	var from string
	var to string
	var fromVersion int
	var toVersion int

	cmd := &cobra.Command{
		Use:   "diff <flow-slug>",
		Short: "Show a source diff between draft, live, or released versions",
		Long: strings.TrimSpace(`
Show a unified diff for flow source.

Defaults to draft versus live so you can inspect unpublished changes:
- breyta flows diff my-flow
- breyta flows diff my-flow --from draft --to version --to-version 7
- breyta flows diff my-flow --from version --from-version 6 --to version --to-version 7
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "diff requires --api/BREYTA_API_URL")
			}

			payload := map[string]any{"flowSlug": args[0]}
			if strings.TrimSpace(from) != "" {
				payload["from"] = strings.TrimSpace(from)
			}
			if strings.TrimSpace(to) != "" {
				payload["to"] = strings.TrimSpace(to)
			}
			if fromVersion > 0 {
				payload["fromVersion"] = fromVersion
			}
			if toVersion > 0 {
				payload["toVersion"] = toVersion
			}
			return doAPICommand(cmd, app, "flows.diff", payload)
		},
	}

	cmd.Flags().StringVar(&from, "from", "draft", "Diff source (draft|live|version)")
	cmd.Flags().StringVar(&to, "to", "live", "Diff target (draft|live|version)")
	cmd.Flags().IntVar(&fromVersion, "from-version", 0, "Version number when --from=version")
	cmd.Flags().IntVar(&toVersion, "to-version", 0, "Version number when --to=version")
	return cmd
}
