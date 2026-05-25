package cli

import (
	"context"
	"strings"
	"time"
)

type terminalRunSnapshot struct {
	Status      string
	At          string
	CurrentStep string
	Source      string
}

func canonicalRunStatus(v any) string {
	s := strings.ToLower(strings.TrimSpace(toString(v)))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "canceled":
		return "cancelled"
	case "timedout":
		return "timed-out"
	default:
		return s
	}
}

func isTerminalRunStatus(status string) bool {
	switch canonicalRunStatus(status) {
	case "completed", "failed", "cancelled", "terminated", "timed-out":
		return true
	default:
		return false
	}
}

func runFromCommandResponse(out map[string]any) map[string]any {
	data := mapStringAny(out["data"])
	if data == nil {
		return nil
	}
	return mapStringAny(data["run"])
}

func terminalWaitFallbackInterval(poll time.Duration) time.Duration {
	interval := poll * 4
	if interval < time.Second {
		return time.Second
	}
	if interval > 5*time.Second {
		return 5 * time.Second
	}
	return interval
}

func shouldCheckTerminalWaitFallback(polls int, next time.Time) bool {
	return polls >= 2 && (next.IsZero() || !time.Now().Before(next))
}

func terminalRunFallback(client apiCommandRunner, workflowID string, installationID string) (map[string]any, int, string, bool, error) {
	fullResp, fullStatus, err := hydrateWaitRunSnapshot(client, workflowID, installationID)
	if err != nil {
		return nil, fullStatus, "", false, err
	}
	if fullStatus < 400 {
		if run := runFromCommandResponse(fullResp); run != nil {
			if status := canonicalRunStatus(run["status"]); isTerminalRunStatus(status) {
				return fullResp, fullStatus, status, true, nil
			}
		}
	}

	if snapshot, ok, err := fetchTerminalRunSnapshotFromEvents(client, workflowID, installationID); err == nil && ok {
		if fullResp == nil || fullStatus >= 400 {
			fullResp = map[string]any{
				"ok": true,
				"data": map[string]any{
					"run": map[string]any{
						"workflowId": strings.TrimSpace(workflowID),
					},
				},
			}
			fullStatus = 200
		}
		applyTerminalRunSnapshot(fullResp, snapshot)
		return fullResp, fullStatus, snapshot.Status, true, nil
	} else if err != nil {
		return fullResp, fullStatus, "", false, err
	}
	return fullResp, fullStatus, "", false, nil
}

func reconcileRunResponseWithTerminalEvents(client apiCommandRunner, out map[string]any, workflowID string, installationID string) {
	run := runFromCommandResponse(out)
	if run == nil || isTerminalRunStatus(canonicalRunStatus(run["status"])) {
		return
	}
	snapshot, ok, err := fetchTerminalRunSnapshotFromEvents(client, workflowID, installationID)
	if err != nil || !ok {
		return
	}
	applyTerminalRunSnapshot(out, snapshot)
}

func applyTerminalRunSnapshot(out map[string]any, snapshot *terminalRunSnapshot) {
	if out == nil || snapshot == nil || !isTerminalRunStatus(snapshot.Status) {
		return
	}
	run := runFromCommandResponse(out)
	if run == nil {
		return
	}
	previousStatus := canonicalRunStatus(run["status"])
	if isTerminalRunStatus(previousStatus) {
		return
	}
	run["status"] = snapshot.Status
	if snapshot.At != "" {
		run["completedAt"] = snapshot.At
		run["updatedAt"] = snapshot.At
	}
	if snapshot.CurrentStep != "" {
		run["currentStep"] = snapshot.CurrentStep
	} else {
		delete(run, "currentStep")
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	meta["terminalEventReconciled"] = true
	meta["terminalStatusSource"] = snapshot.Source
	if len(sliceAny(run["steps"])) > 0 {
		meta["stepSnapshotMayLag"] = true
	}
	if previousStatus != "" {
		meta["staleStatus"] = previousStatus
	}
}

func fetchTerminalRunSnapshotFromEvents(client apiCommandRunner, workflowID string, installationID string) (*terminalRunSnapshot, bool, error) {
	payload := map[string]any{
		"workflowId": strings.TrimSpace(workflowID),
		"limit":      500,
	}
	if strings.TrimSpace(installationID) != "" {
		payload["installationId"] = strings.TrimSpace(installationID)
	}
	out, status, err := client.DoCommand(context.Background(), "runs.events", payload)
	if err != nil {
		return nil, false, err
	}
	if status >= 400 || !isOK(out) {
		return nil, false, nil
	}
	items := sliceAny(mapStringAny(out["data"])["items"])
	var lastStep string
	var terminal *terminalRunSnapshot
	for _, item := range items {
		event := mapStringAny(item)
		if event == nil {
			continue
		}
		if stepID, ok := terminalStepIDFromEvent(event); ok {
			lastStep = stepID
		}
		if status := terminalStatusFromRunEvent(event); status != "" {
			terminal = &terminalRunSnapshot{
				Status:      status,
				At:          firstNonBlankString(event["at"], event["eventAt"], event["event-at"], event["completedAt"], event["updatedAt"]),
				CurrentStep: lastStep,
				Source:      "runs.events",
			}
		}
	}
	if terminal == nil {
		return nil, false, nil
	}
	return terminal, true, nil
}

func terminalStatusFromRunEvent(event map[string]any) string {
	eventType := strings.ToLower(strings.TrimSpace(firstNonBlankString(event["type"], event["eventType"], event["event-type"])))
	entityType := strings.ToLower(strings.TrimSpace(firstNonBlankString(event["entityType"], event["entity-type"])))
	status := canonicalRunStatus(firstNonBlankString(event["status"], mapStringAny(event["details"])["status"], mapStringAny(event["details"])["newStatus"], mapStringAny(event["details"])["new-status"]))

	switch eventType {
	case "run_completed", "execution_completed":
		return "completed"
	case "run_cancelled", "execution_cancelled":
		if status == "terminated" {
			return status
		}
		return "cancelled"
	case "run_failed", "execution_failed":
		if status == "timed-out" {
			return status
		}
		return "failed"
	case "run_terminated", "execution_terminated":
		return "terminated"
	case "run_timed-out", "run_timed_out", "execution_timed-out", "execution_timed_out":
		return "timed-out"
	}

	if isTerminalRunStatus(status) && (entityType == "execution" || strings.HasPrefix(eventType, "run_") || strings.HasPrefix(eventType, "execution_")) {
		return status
	}
	return ""
}

func terminalStepIDFromEvent(event map[string]any) (string, bool) {
	eventType := strings.ToLower(strings.TrimSpace(firstNonBlankString(event["type"], event["eventType"], event["event-type"])))
	status := canonicalRunStatus(firstNonBlankString(event["status"], mapStringAny(event["details"])["status"], mapStringAny(event["details"])["newStatus"], mapStringAny(event["details"])["new-status"]))
	terminalEventType := eventType == "step_completed" ||
		eventType == "step_failed" ||
		eventType == "step_cancelled" ||
		eventType == "step_skipped" ||
		strings.HasPrefix(eventType, "step_completed")
	if !terminalEventType && !isTerminalStepStatus(status) {
		return "", false
	}
	stepID := firstNonBlankString(event["stepId"], event["step-id"], event["id"], mapStringAny(event["details"])["stepId"], mapStringAny(event["details"])["step-id"])
	if stepID == "" {
		return "", false
	}
	return stepID, true
}

func isTerminalStepStatus(status string) bool {
	switch canonicalRunStatus(status) {
	case "completed", "failed", "cancelled", "skipped":
		return true
	default:
		return false
	}
}
