package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil {
			return n
		}
	default:
		return 0
	}
	return 0
}

func releaseLiveRuntimeSummary(flowSlug string, releaseData, promoteData map[string]any) map[string]any {
	releaseRuntime := mapStringAny(releaseData["liveRuntime"])
	promoteRuntime := mapStringAny(promoteData["liveRuntime"])
	activeVersion := asInt(firstPresentAny(releaseData["activeVersion"], promoteData["activeVersion"], promoteRuntime["activeVersion"]))
	latestVersion := asInt(firstPresentAny(
		promoteData["latestVersion"],
		promoteRuntime["latestVersion"],
		releaseData["latestVersion"],
		releaseRuntime["latestVersion"],
		promoteData["latestAvailable"],
		releaseData["latestAvailable"],
	))
	runtimeVersion := asInt(firstPresentAny(promoteData["liveRuntimeVersion"], promoteData["version"], promoteRuntime["version"]))

	summary := map[string]any{
		"flowSlug": flowSlug,
		"target":   "live",
	}
	if profileID := strings.TrimSpace(scalarString(firstPresentAny(promoteData["profileId"], promoteRuntime["profileId"]))); profileID != "" {
		summary["profileId"] = profileID
	}
	if runtimeVersion > 0 {
		summary["version"] = runtimeVersion
	}
	if activeVersion > 0 {
		summary["activeVersion"] = activeVersion
	}
	if latestVersion > 0 {
		summary["latestVersion"] = latestVersion
	}
	if runtimeVersion > 0 && activeVersion > 0 {
		summary["runtimeMatchesActiveVersion"] = runtimeVersion == activeVersion
	}
	if latestVersion > 0 && activeVersion > 0 {
		summary["latestMatchesActiveVersion"] = latestVersion == activeVersion
	}

	status := "live_runtime_version_verified"
	if activeVersion == 0 {
		status = "active_version_missing"
	} else if runtimeVersion == 0 {
		status = "live_runtime_version_missing"
	} else if runtimeVersion != activeVersion {
		status = "live_runtime_version_differs_from_active_version"
	} else if latestVersion > 0 && latestVersion != activeVersion {
		status = "latest_version_differs_from_active_version"
	}
	summary["status"] = status
	return summary
}

func releaseLiveRuntimeWarnings(summary map[string]any) []string {
	runtimeVersion := asInt(summary["version"])
	activeVersion := asInt(summary["activeVersion"])
	latestVersion := asInt(summary["latestVersion"])
	warnings := []string{}
	if runtimeVersion > 0 && activeVersion > 0 && runtimeVersion != activeVersion {
		warnings = append(warnings, fmt.Sprintf("Live target now uses version %d while activeVersion is %d; webhook/live traffic follows the live target version.", runtimeVersion, activeVersion))
	}
	if latestVersion > 0 && activeVersion > 0 && latestVersion != activeVersion {
		warnings = append(warnings, fmt.Sprintf("latestVersion is %d while activeVersion is %d; release/promote the intended version explicitly if this was not intentional.", latestVersion, activeVersion))
	}
	if runtimeVersion == 0 {
		slug := strings.TrimSpace(scalarString(summary["flowSlug"]))
		if slug == "" {
			slug = "<slug>"
		}
		warnings = append(warnings, fmt.Sprintf("Live target version was not returned by promote; verify with `breyta flows show %s --target live` before assuming webhook traffic changed.", slug))
	}
	return warnings
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

	return waitForRunCompletion(cmd, app, startResp, strings.TrimSpace(flowSlug), command, timeout, poll)
}

func waitForRunCompletion(cmd *cobra.Command, app *App, startResp map[string]any, flowSlug string, command string, timeout time.Duration, poll time.Duration) error {
	client := apiClient(app)
	data, _ := startResp["data"].(map[string]any)
	workflowID := workflowIDFromRunData(data)
	if strings.TrimSpace(workflowID) == "" {
		return writeErr(cmd, errors.New("missing data.workflowId in start response"))
	}
	installationID := installationIDFromRunData(data)
	deadline := time.Now().Add(timeout)
	for {
		payload := compactRunsGetPayload(workflowID)
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
				"flow_slug":   flowSlug,
				"command":     strings.TrimSpace(command),
				"workflow_id": workflowID,
				"run_status":  s,
				"wait":        true,
			})
			finalResp, finalStatus, err := hydrateTerminalWaitRun(client, workflowID, installationID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if finalStatus >= 400 {
				finalResp = execResp
				finalStatus = execStatus
			}
			if err := writeAPIResult(cmd, app, finalResp, finalStatus); err != nil {
				return writeErr(cmd, err)
			}
			if runStatusFailedForExit(s) {
				return guidedCLIErrorForCommand(cmd, "flow run finished with status "+s, []string{
					"Inspect failed steps: breyta runs inspect " + workflowID,
					"List resources: breyta resources workflow list " + workflowID,
				})
			}
			return nil
		}

		if time.Now().After(deadline) {
			trackCLIEvent(app, "cli_flow_run_wait_timed_out", nil, app.Token, map[string]any{
				"product":     "flows",
				"channel":     "cli",
				"api_host":    apiHostname(app.APIURL),
				"flow_slug":   flowSlug,
				"command":     strings.TrimSpace(command),
				"workflow_id": workflowID,
				"wait":        true,
			})
			lastPoll := execResp
			if snapshot, snapshotStatus, err := hydrateWaitRunSnapshot(client, workflowID, installationID); err == nil && snapshotStatus < 400 {
				lastPoll = snapshot
			}
			nextCommands := []string{
				"breyta runs inspect " + workflowID,
				"breyta runs show " + workflowID + " --include-steps",
				"breyta resources workflow list " + workflowID,
			}
			if flowSlug != "" {
				nextCommands = append(nextCommands, "breyta flows run "+flowSlug+" --wait --timeout 2m")
			}
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
					"timedOut":     true,
					"hint":         "The run may still be in progress. Inspect the workflow id, or use a longer --timeout on the next waited run.",
					"nextCommands": nextCommands,
				},
				"data": map[string]any{
					"workflowId": workflowID,
					"start":      startResp,
					"lastPoll":   lastPoll,
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

func runStatusFailedForExit(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "cancelled", "canceled", "terminated", "timed-out", "timed_out":
		return true
	default:
		return false
	}
}

func newFlowsRunCmd(app *App) *cobra.Command {
	var installationID string
	var target string
	var version int
	var invocation string
	var interfaceID string
	var triggerID string
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
- brand-new unreleased flows: add --target draft while authoring, or release first

	Advanced targeting:
	- --installation-id <id> : run a specific installation target
	- --invocation <id> : select a named invocation input contract
	- --interface-id <id> : select the declared manual interface explicitly
	- --target draft|live : select workspace draft/live when not using --installation-id
	- --version <n> : force a specific release version for this invocation
	- --trigger-id <id> : compatibility alias for legacy trigger-backed flows
	`),
		Example: strings.TrimSpace(`
breyta flows run order-ingest --wait
breyta flows run order-ingest --input '{"region":"EU"}' --wait

	# Advanced
	breyta flows run order-ingest --target live --wait
	breyta flows run order-ingest --target draft --wait
	breyta flows run order-ingest --invocation import-orders --input '{"region":"EU"}' --wait
	breyta flows run order-ingest --target draft --interface-id manual-import --input '{"limit":5}' --wait
	breyta flows run order-ingest --installation-id inst_123 --wait
	`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "flows run requires --api/BREYTA_API_URL")
			}
			installationID = strings.TrimSpace(installationID)
			resolvedTarget := ""
			if cmd.Flags().Changed("target") {
				var err error
				resolvedTarget, err = normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
			} else if installationID != "" {
				resolvedTarget = "live"
			} else {
				resolvedTarget = "draft"
			}
			payload := map[string]any{"flowSlug": args[0]}
			if resolvedTarget != "" {
				payload["target"] = resolvedTarget
			}
			if installationID != "" {
				payload["installationId"] = installationID
			}
			if version > 0 {
				payload["version"] = version
			}
			invocation = strings.TrimSpace(invocation)
			if invocation != "" {
				payload["invocation"] = invocation
			}
			interfaceID = strings.TrimSpace(interfaceID)
			triggerID = strings.TrimSpace(triggerID)
			if interfaceID != "" && triggerID != "" && interfaceID != triggerID {
				return writeErr(cmd, fmt.Errorf("--interface-id and --trigger-id refer to the same manual selector; provide only one"))
			}
			manualSelector := interfaceID
			if manualSelector == "" {
				manualSelector = triggerID
			}
			if manualSelector != "" {
				payload["triggerId"] = manualSelector
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
	cmd.Flags().StringVar(&invocation, "invocation", "", "Advanced: named invocation input contract")
	cmd.Flags().StringVar(&invocation, "invocation-id", "", "Advanced: named invocation input contract")
	cmd.Flags().StringVar(&interfaceID, "interface-id", "", "Manual interface id to select explicitly")
	cmd.Flags().StringVar(&interfaceID, "interface", "", "Alias for --interface-id")
	cmd.Flags().StringVar(&triggerID, "trigger-id", "", "Compatibility: legacy manual trigger id")
	cmd.Flags().StringVar(&triggerID, "trigger", "", "Compatibility alias for --trigger-id")
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
			promoteData := mapStringAny(promoteOut["data"])
			liveRuntime := releaseLiveRuntimeSummary(args[0], releaseData, promoteData)
			warnings := releaseLiveRuntimeWarnings(liveRuntime)
			combined := map[string]any{
				"ok": true,
				"workspaceId": func() string {
					if ws := commandWorkspaceID(releaseOut); ws != "" {
						return ws
					}
					return commandWorkspaceID(promoteOut)
				}(),
				"meta": func() map[string]any {
					meta := map[string]any{
						"released":    true,
						"installed":   !skipPromoteInstallations,
						"target":      "live",
						"liveRuntime": liveRuntime,
						"verifyHint":  "Live runtime can differ from flow activeVersion. Verify with `breyta flows show <slug> --target live` and `breyta flows run <slug> --target live --wait`.",
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
					}
					if len(warnings) > 0 {
						meta["warnings"] = warnings
					}
					return meta
				}(),
				"data": map[string]any{
					"release":     releaseOut["data"],
					"install":     promoteOut["data"],
					"liveRuntime": liveRuntime,
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
	var full bool
	var file string

	cmd := &cobra.Command{
		Use:   "diff <flow-slug>",
		Short: "Show a source diff between draft, live, or released versions",
		Long: strings.TrimSpace(`
Show a unified diff for flow source.

Defaults to live versus draft so you can inspect unpublished changes as
additions to the draft. If a
draft-only flow has pushed draft history but no live version, the server can
compare the current draft to the previous pushed draft version.

- breyta flows diff my-flow
- breyta flows diff my-flow --file ./flows/my-flow.clj
- breyta flows diff my-flow --from draft --to version --to-version 7
- breyta flows diff my-flow --from version --from-version 6 --to version --to-version 7
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "diff requires --api/BREYTA_API_URL")
			}

			fromChanged := cmd.Flags().Changed("from")
			toChanged := cmd.Flags().Changed("to")
			file = strings.TrimSpace(file)
			if file != "" && !fromChanged && !toChanged {
				from = "draft"
				to = "file"
			}
			if file != "" && !diffSourceStringIsFile(from) && !diffSourceStringIsFile(to) {
				return writeErr(cmd, errors.New("--file compares one side of the diff; use --from file or --to file when also passing explicit --from/--to"))
			}

			payload := map[string]any{"flowSlug": args[0]}
			if strings.TrimSpace(from) != "" {
				payload["from"] = strings.TrimSpace(from)
			}
			if strings.TrimSpace(to) != "" {
				payload["to"] = strings.TrimSpace(to)
			}
			if file != "" {
				b, err := os.ReadFile(file)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["fileLiteral"] = string(b)
				payload["fileLabel"] = filepath.Base(file)
			}
			if fromVersion > 0 {
				payload["fromVersion"] = fromVersion
			}
			if toVersion > 0 {
				payload["toVersion"] = toVersion
			}
			if full {
				payload["view"] = "full"
			} else {
				payload["view"] = "summary"
			}
			return doAPICommand(cmd, app, "flows.diff", payload)
		},
	}

	cmd.Flags().StringVar(&from, "from", "live", "Diff source (draft|live|version|file)")
	cmd.Flags().StringVar(&to, "to", "draft", "Diff target (draft|live|version|file)")
	cmd.Flags().IntVar(&fromVersion, "from-version", 0, "Version number when --from=version")
	cmd.Flags().IntVar(&toVersion, "to-version", 0, "Version number when --to=version")
	cmd.Flags().BoolVar(&full, "full", false, "Include the full unified diff")
	cmd.Flags().StringVar(&file, "file", "", "Compare a local .clj file against one side of the diff (default: draft to file)")
	return cmd
}

func diffSourceStringIsFile(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "file")
}
