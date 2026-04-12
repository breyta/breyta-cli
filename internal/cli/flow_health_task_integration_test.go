package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type taskFlowHealthFixture struct {
	WorkspaceID  string `json:"workspaceId"`
	UserID       string `json:"userId"`
	DigestID     string `json:"digestId"`
	IncidentID   string `json:"incidentId"`
	FailureID    string `json:"failureId"`
	DeliveryID   string `json:"deliveryId"`
	FlowSlug     string `json:"flowSlug"`
	FlowVersion  int    `json:"flowVersion"`
	InstanceKey  string `json:"instanceKey"`
	ErrorSummary string `json:"errorSummary"`
}

func TestFlowHealth_TaskRuntimeIntegration(t *testing.T) {
	if os.Getenv("BREYTA_FLOW_HEALTH_TASK_INTEGRATION") == "" {
		t.Skip("set BREYTA_FLOW_HEALTH_TASK_INTEGRATION=1 to run against the local task runtime")
	}

	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	workbenchRoot := resolveWorkbenchRoot(t)
	taskName := strings.TrimSpace(os.Getenv("BREYTA_VERIFY_TASK"))
	if taskName == "" {
		taskName = "flow-failure-notification-settings"
	}
	apiURL := resolveTaskAPIURL(t, workbenchRoot, taskName)
	fixture := generateTaskFlowHealthFixture(t, workbenchRoot, taskName)

	bootstrap := runTaskCLI(t, apiURL, fixture.UserID,
		"workspaces", "bootstrap", fixture.WorkspaceID, "--name", "Acme Health Test")
	if !bootstrap.OK {
		t.Fatalf("bootstrap failed: %+v", bootstrap)
	}

	incidentShow := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"incidents", "show", fixture.IncidentID, "--failure-limit", "5")
	if !incidentShow.OK {
		t.Fatalf("incident show failed: %+v", incidentShow)
	}
	incident, _ := incidentShow.Data["incident"].(map[string]any)
	if got, _ := incident["incident-id"].(string); got != fixture.IncidentID {
		t.Fatalf("expected incident-id=%q, got %q", fixture.IncidentID, got)
	}
	if got, _ := incident["target"].(string); got != "live" {
		t.Fatalf("expected incident target=live, got %q", got)
	}
	if got, _ := incident["latest-flow-version"].(float64); got != float64(fixture.FlowVersion) {
		t.Fatalf("expected latest-flow-version=%d, got %v", fixture.FlowVersion, got)
	}
	failures, _ := incidentShow.Data["failures"].([]any)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	firstFailure, _ := failures[0].(map[string]any)
	if got, _ := firstFailure["failure-id"].(string); got != fixture.FailureID {
		t.Fatalf("expected failure-id=%q, got %q", fixture.FailureID, got)
	}
	if got, _ := firstFailure["flow-version"].(float64); got != float64(fixture.FlowVersion) {
		t.Fatalf("expected failure flow-version=%d, got %v", fixture.FlowVersion, got)
	}

	lanes := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"incidents", "lanes", fixture.IncidentID, "--limit", "10")
	if !lanes.OK {
		t.Fatalf("incident lanes failed: %+v", lanes)
	}
	laneItems, _ := lanes.Data["items"].([]any)
	if len(laneItems) == 0 {
		t.Fatalf("expected at least 1 lane item")
	}
	firstLane, _ := laneItems[0].(map[string]any)
	if got, _ := firstLane["instance-key"].(string); got != fixture.InstanceKey {
		t.Fatalf("expected instance-key=%q, got %q", fixture.InstanceKey, got)
	}

	digestShow := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"digests", "show", fixture.DigestID)
	if !digestShow.OK {
		t.Fatalf("digest show failed: %+v", digestShow)
	}
	digest, _ := digestShow.Data["digest"].(map[string]any)
	if got, _ := digest["digest-id"].(string); got != fixture.DigestID {
		t.Fatalf("expected digest-id=%q, got %q", fixture.DigestID, got)
	}

	deliveries := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"digests", "deliveries", fixture.DigestID, "--channel", "in-app", "--limit", "10")
	if !deliveries.OK {
		t.Fatalf("digest deliveries failed: %+v", deliveries)
	}
	deliveryItems, _ := deliveries.Data["items"].([]any)
	if len(deliveryItems) == 0 {
		t.Fatalf("expected at least 1 delivery")
	}
	firstDelivery, _ := deliveryItems[0].(map[string]any)
	if got, _ := firstDelivery["id"].(string); got != fixture.DeliveryID {
		t.Fatalf("expected delivery id=%q, got %q", fixture.DeliveryID, got)
	}
	if unread, _ := firstDelivery["unread"].(bool); !unread {
		t.Fatalf("expected seeded delivery to be unread")
	}

	markRead := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"digests", "mark-read", fixture.DigestID)
	if !markRead.OK {
		t.Fatalf("digest mark-read failed: %+v", markRead)
	}
	if got, _ := markRead.Data["updated-count"].(float64); got != 1 {
		t.Fatalf("expected updated-count=1, got %v", got)
	}

	deliveriesAfterRead := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"digests", "deliveries", fixture.DigestID, "--channel", "in-app", "--limit", "10")
	if !deliveriesAfterRead.OK {
		t.Fatalf("digest deliveries after mark-read failed: %+v", deliveriesAfterRead)
	}
	deliveryItemsAfterRead, _ := deliveriesAfterRead.Data["items"].([]any)
	if len(deliveryItemsAfterRead) == 0 {
		t.Fatalf("expected at least 1 delivery after read")
	}
	firstDeliveryAfterRead, _ := deliveryItemsAfterRead[0].(map[string]any)
	if got, _ := firstDeliveryAfterRead["status"].(string); got != "read" {
		t.Fatalf("expected delivery status=read after mark-read, got %q", got)
	}
	if unread, _ := firstDeliveryAfterRead["unread"].(bool); unread {
		t.Fatalf("expected unread=false after mark-read")
	}

	cadenceBefore := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"digests", "cadence")
	if !cadenceBefore.OK {
		t.Fatalf("digest cadence failed: %+v", cadenceBefore)
	}
	preferencesBefore, _ := cadenceBefore.Data["preferences"].(map[string]any)
	originalCadence, _ := preferencesBefore["digest-cadence"].(string)
	if originalCadence == "" {
		t.Fatalf("expected digest cadence in preferences, got %+v", preferencesBefore)
	}
	settingsWebURL, _ := cadenceBefore.Data["settings-web-url"].(string)
	if !strings.Contains(settingsWebURL, "/settings/my-updates") {
		t.Fatalf("expected settings-web-url deep link, got %q", settingsWebURL)
	}

	nextCadence := map[string]string{
		"daily":   "weekly",
		"weekly":  "monthly",
		"monthly": "daily",
	}[originalCadence]
	if nextCadence == "" {
		t.Fatalf("unexpected original cadence %q", originalCadence)
	}

	setCadence := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"digests", "cadence", "set", nextCadence)
	if !setCadence.OK {
		t.Fatalf("digest cadence set failed: %+v", setCadence)
	}
	preferencesAfterSet, _ := setCadence.Data["preferences"].(map[string]any)
	if got, _ := preferencesAfterSet["digest-cadence"].(string); got != nextCadence {
		t.Fatalf("expected digest cadence=%q after set, got %q", nextCadence, got)
	}

	cadenceAfter := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"digests", "cadence")
	if !cadenceAfter.OK {
		t.Fatalf("digest cadence re-read failed: %+v", cadenceAfter)
	}
	preferencesAfter, _ := cadenceAfter.Data["preferences"].(map[string]any)
	if got, _ := preferencesAfter["digest-cadence"].(string); got != nextCadence {
		t.Fatalf("expected digest cadence=%q after re-read, got %q", nextCadence, got)
	}

	restoreCadence := runTaskCLI(t, apiURL, fixture.UserID,
		"--workspace", fixture.WorkspaceID,
		"digests", "cadence", "set", originalCadence)
	if !restoreCadence.OK {
		t.Fatalf("digest cadence restore failed: %+v", restoreCadence)
	}
}

func runTaskCLI(t *testing.T, apiURL, token string, args ...string) envelope {
	t.Helper()
	cliArgs := []string{"--dev", "--api", apiURL, "--token", token}
	cliArgs = append(cliArgs, args...)
	stdout, stderr, err := runCLIArgs(t, cliArgs...)
	if err != nil {
		t.Fatalf("cli command failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	return decodeEnvelope(t, stdout)
}

func generateTaskFlowHealthFixture(t *testing.T, workbenchRoot, taskName string) taskFlowHealthFixture {
	t.Helper()
	scriptPath := filepath.Join(
		workbenchRoot,
		".workbench",
		"verify",
		taskName,
		"cases",
		"generate-flow-health-incident.ts",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npx", "tsx", scriptPath, "ws-acme")
	cmd.Dir = workbenchRoot
	cmd.Env = append(os.Environ(),
		"BREYTA_NO_UPDATE_CHECK=1",
		"BREYTA_NO_SKILL_SYNC=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate fixture failed: %v\n%s", err, string(out))
	}
	var fixture taskFlowHealthFixture
	if err := json.Unmarshal([]byte(extractTrailingJSONObject(string(out))), &fixture); err != nil {
		t.Fatalf("invalid fixture json: %v\n%s", err, string(out))
	}
	return fixture
}

func resolveTaskAPIURL(t *testing.T, workbenchRoot, taskName string) string {
	t.Helper()
	if apiURL := strings.TrimSpace(os.Getenv("BREYTA_API_URL")); apiURL != "" {
		return apiURL
	}
	taskEnv := resolveTaskShellEnv(t, workbenchRoot, taskName)
	if apiURL := strings.TrimSpace(taskEnv["BREYTA_API_URL"]); apiURL != "" {
		return apiURL
	}
	t.Fatalf("missing BREYTA_API_URL for task %s", taskName)
	return ""
}

func resolveTaskShellEnv(t *testing.T, workbenchRoot, taskName string) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bwb", "task", taskName, "status", "--env-only", "--env-format", "shell")
	cmd.Dir = workbenchRoot
	cmd.Env = append(os.Environ(),
		"BREYTA_NO_UPDATE_CHECK=1",
		"BREYTA_NO_SKILL_SYNC=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to resolve task env: %v\n%s", err, string(out))
	}
	return parseShellExports(string(out))
}

func parseShellExports(raw string) map[string]string {
	env := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "export ") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(line, "export "), "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		value = strings.ReplaceAll(value, "\\\"", "\"")
		value = strings.ReplaceAll(value, "\\'", "'")
		env[key] = value
	}
	return env
}

func resolveWorkbenchRoot(t *testing.T) string {
	t.Helper()
	if root := strings.TrimSpace(os.Getenv("BREYTA_WORKBENCH_ROOT")); root != "" {
		return root
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	marker := string(filepath.Separator) + ".worktrees" + string(filepath.Separator)
	idx := strings.Index(wd, marker)
	if idx == -1 {
		t.Fatalf("could not derive workbench root from %q; set BREYTA_WORKBENCH_ROOT", wd)
	}
	return wd[:idx]
}

func extractTrailingJSONObject(raw string) string {
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return raw
}
