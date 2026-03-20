package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/breyta/breyta-cli/internal/cli"
)

type envelope struct {
	OK          bool           `json:"ok"`
	WorkspaceID string         `json:"workspaceId"`
	Meta        map[string]any `json:"meta"`
	Data        map[string]any `json:"data"`
	Error       map[string]any `json:"error"`
	Hint        string         `json:"hint"`
}

func runCLI(t *testing.T, statePath string, args ...string) (string, string, error) {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	cmd := cli.NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)

	// Contract tests cover the full (dev-only) surface area.
	// Force mock mode even if the developer has BREYTA_API_URL set in their shell.
	base := []string{"--workspace", "demo-workspace", "--state", statePath, "--api", "", "--dev"}
	cmd.SetArgs(append(base, args...))

	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func decodeEnvelope(t *testing.T, s string) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		t.Fatalf("output is not valid JSON: %v\n---\n%s", err, s)
	}
	return e
}

func TestContract_FlowsListEnvelope(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	stdout, _, err := runCLI(t, statePath, "flows", "list", "--pretty")
	if err != nil {
		t.Fatalf("expected success, got error: %v\n%s", err, stdout)
	}
	e := decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("expected ok=true, got ok=false: %+v", e)
	}
	if e.WorkspaceID != "demo-workspace" {
		t.Fatalf("unexpected workspaceId: %q", e.WorkspaceID)
	}
	itemsAny, ok := e.Data["items"]
	if !ok {
		t.Fatalf("missing data.items")
	}
	items, ok := itemsAny.([]any)
	if !ok {
		t.Fatalf("data.items is not an array: %T", itemsAny)
	}
	if len(items) == 0 {
		t.Fatalf("expected at least 1 flow")
	}
}

func TestContract_FlowsCreateEditAndValidate(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	// Create a flow
	stdout, _, err := runCLI(t, statePath, "flows", "create", "--slug", "contract-flow", "--name", "Contract Flow", "--pretty")
	if err != nil {
		t.Fatalf("create failed: %v\n%s", err, stdout)
	}
	e := decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("create expected ok=true")
	}

	// Create a second flow to verify grouping.
	stdout, _, err = runCLI(t, statePath, "flows", "create", "--slug", "contract-flow-peer", "--name", "Contract Flow Peer", "--pretty")
	if err != nil {
		t.Fatalf("create peer failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("create peer expected ok=true")
	}

	// Create a third flow so ordered sibling output is observable.
	stdout, _, err = runCLI(t, statePath, "flows", "create", "--slug", "contract-flow-first", "--name", "Contract Flow First", "--pretty")
	if err != nil {
		t.Fatalf("create first failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("create first expected ok=true")
	}

	// Edit flow metadata (steps editing is managed via push/pull in API mode)
	stdout, _, err = runCLI(t, statePath, "flows", "update", "contract-flow",
		"--name", "Contract Flow Updated",
		"--group-key", "contract-bundle",
		"--group-name", "Contract Bundle",
		"--group-description", "Flows that should be installed together",
		"--group-order", "20",
		"--pretty")
	if err != nil {
		t.Fatalf("flows update failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("flows update expected ok=true")
	}
	flowAny, ok := e.Data["flow"]
	if !ok {
		t.Fatalf("flows update expected data.flow")
	}
	flowMap, ok := flowAny.(map[string]any)
	if !ok {
		t.Fatalf("flows update expected data.flow object, got %T", flowAny)
	}
	if got, _ := flowMap["groupKey"].(string); got != "contract-bundle" {
		t.Fatalf("expected update response groupKey=contract-bundle, got %q", got)
	}
	if got, _ := flowMap["groupName"].(string); got != "Contract Bundle" {
		t.Fatalf("expected update response groupName=Contract Bundle, got %q", got)
	}
	if got, _ := flowMap["groupOrder"].(float64); got != 20 {
		t.Fatalf("expected update response groupOrder=20, got %v", got)
	}

	stdout, _, err = runCLI(t, statePath, "flows", "update", "contract-flow-peer",
		"--group-key", "contract-bundle",
		"--group-name", "Contract Bundle",
		"--group-description", "Flows that should be installed together",
		"--group-order", "30",
		"--pretty")
	if err != nil {
		t.Fatalf("flows update peer failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("flows update peer expected ok=true")
	}

	stdout, _, err = runCLI(t, statePath, "flows", "update", "contract-flow-first",
		"--group-key", "contract-bundle",
		"--group-name", "Contract Bundle",
		"--group-description", "Flows that should be installed together",
		"--group-order", "10",
		"--pretty")
	if err != nil {
		t.Fatalf("flows update first failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("flows update first expected ok=true")
	}

	// Validate
	stdout, _, err = runCLI(t, statePath, "flows", "validate", "contract-flow", "--pretty")
	if err != nil {
		t.Fatalf("validate failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("validate expected ok=true")
	}

	stdout, _, err = runCLI(t, statePath, "flows", "show", "contract-flow", "--pretty")
	if err != nil {
		t.Fatalf("flows show failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("flows show expected ok=true")
	}
	flowAny, ok = e.Data["flow"]
	if !ok {
		t.Fatalf("flows show expected data.flow")
	}
	flowMap, ok = flowAny.(map[string]any)
	if !ok {
		t.Fatalf("flows show expected data.flow object, got %T", flowAny)
	}
	if got, _ := flowMap["groupKey"].(string); got != "contract-bundle" {
		t.Fatalf("expected show response groupKey=contract-bundle, got %q", got)
	}
	if got, _ := flowMap["groupName"].(string); got != "Contract Bundle" {
		t.Fatalf("expected show response groupName=Contract Bundle, got %q", got)
	}
	if got, _ := flowMap["groupDescription"].(string); got != "Flows that should be installed together" {
		t.Fatalf("expected show response groupDescription, got %q", got)
	}
	if got, _ := flowMap["groupOrder"].(float64); got != 20 {
		t.Fatalf("expected show response groupOrder=20, got %v", got)
	}
	groupFlowsAny, ok := flowMap["groupFlows"]
	if !ok {
		t.Fatalf("flows show expected flow.groupFlows")
	}
	groupFlows, ok := groupFlowsAny.([]any)
	if !ok || len(groupFlows) != 2 {
		t.Fatalf("flows show expected non-empty flow.groupFlows, got %T %#v", groupFlowsAny, groupFlowsAny)
	}
	orderedSlugs := make([]string, 0, len(groupFlows))
	for _, raw := range groupFlows {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected groupFlows entries to be objects, got %T", raw)
		}
		slug, _ := item["flowSlug"].(string)
		orderedSlugs = append(orderedSlugs, slug)
		if got, _ := item["groupName"].(string); got != "Contract Bundle" {
			t.Fatalf("expected peer groupName=Contract Bundle, got %q", got)
		}
		if slug == "contract-flow-first" {
			if got, _ := item["groupOrder"].(float64); got != 10 {
				t.Fatalf("expected first sibling groupOrder=10, got %v", got)
			}
		}
		if slug == "contract-flow-peer" {
			if got, _ := item["groupOrder"].(float64); got != 30 {
				t.Fatalf("expected peer groupOrder=30, got %v", got)
			}
		}
	}
	if got := strings.Join(orderedSlugs, ","); got != "contract-flow-first,contract-flow-peer" {
		t.Fatalf("expected ordered groupFlows, got %q", got)
	}

	stdout, _, err = runCLI(t, statePath, "flows", "list", "--pretty")
	if err != nil {
		t.Fatalf("flows list failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("flows list expected ok=true")
	}
	itemsAny, ok := e.Data["items"]
	if !ok {
		t.Fatalf("flows list expected data.items")
	}
	items, ok := itemsAny.([]any)
	if !ok {
		t.Fatalf("flows list expected data.items array, got %T", itemsAny)
	}
	foundGroupedFlow := false
	foundGroupedPeer := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		slug, _ := item["flowSlug"].(string)
		if slug != "contract-flow" && slug != "contract-flow-peer" && slug != "contract-flow-first" {
			continue
		}
		if got, _ := item["groupKey"].(string); got != "contract-bundle" {
			t.Fatalf("expected list item %s groupKey=contract-bundle, got %q", slug, got)
		}
		if got, _ := item["groupName"].(string); got != "Contract Bundle" {
			t.Fatalf("expected list item %s groupName=Contract Bundle, got %q", slug, got)
		}
		if slug == "contract-flow" {
			if got, _ := item["groupOrder"].(float64); got != 20 {
				t.Fatalf("expected list item %s groupOrder=20, got %v", slug, got)
			}
		}
		if slug == "contract-flow-peer" {
			if got, _ := item["groupOrder"].(float64); got != 30 {
				t.Fatalf("expected list item %s groupOrder=30, got %v", slug, got)
			}
		}
		if slug == "contract-flow-first" {
			if got, _ := item["groupOrder"].(float64); got != 10 {
				t.Fatalf("expected list item %s groupOrder=10, got %v", slug, got)
			}
		}
		if slug == "contract-flow" {
			foundGroupedFlow = true
		}
		if slug == "contract-flow-peer" {
			foundGroupedPeer = true
		}
	}
	if !foundGroupedFlow || !foundGroupedPeer {
		t.Fatalf("expected grouped flows in list output, found flow=%v peer=%v", foundGroupedFlow, foundGroupedPeer)
	}
}

func TestContract_RunsStartAdvanceStepAndEvents(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	// Seed to ensure known flows exist.
	stdout, _, err := runCLI(t, statePath, "dev", "seed", "--pretty")
	if err != nil {
		t.Fatalf("seed failed: %v\n%s", err, stdout)
	}

	// Start a run
	stdout, _, err = runCLI(t, statePath, "runs", "start", "--flow", "daily-sales-report", "--pretty")
	if err != nil {
		t.Fatalf("runs start failed: %v\n%s", err, stdout)
	}
	e := decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("runs start expected ok=true")
	}
	runAny := e.Data["run"]
	runMap, ok := runAny.(map[string]any)
	if !ok {
		t.Fatalf("data.run is not an object: %T", runAny)
	}
	runID, _ := runMap["workflowId"].(string)
	if runID == "" {
		t.Fatalf("missing run workflowId")
	}

	// Advance
	stdout, _, err = runCLI(t, statePath, "dev", "advance", "--ticks", "1", "--pretty")
	if err != nil {
		t.Fatalf("advance failed: %v\n%s", err, stdout)
	}

	// Step inspection should show concrete input/output.
	stdout, _, err = runCLI(t, statePath, "runs", "step", runID, "fetch-sales", "--pretty")
	if err != nil {
		t.Fatalf("runs step failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("runs step expected ok=true")
	}
	if _, ok := e.Data["input"]; !ok {
		t.Fatalf("runs step expected data.input")
	}
	if _, ok := e.Data["output"]; !ok {
		t.Fatalf("runs step expected data.output")
	}

	// Events timeline
	stdout, _, err = runCLI(t, statePath, "runs", "events", runID, "--pretty")
	if err != nil {
		t.Fatalf("runs events failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("runs events expected ok=true")
	}
	itemsAny, ok := e.Data["items"]
	if !ok {
		t.Fatalf("runs events expected data.items")
	}
	items, ok := itemsAny.([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("runs events expected non-empty data.items")
	}
}

func TestContract_FormatFlagRejected(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	stdout, stderr, err := runCLI(t, statePath, "flows", "list", "--format", "edn", "--pretty")
	if err == nil {
		t.Fatalf("expected error for --format edn; got success\n---\n%s", stdout)
	}
	if !bytes.Contains([]byte(stderr), []byte("unknown flag: --format")) {
		t.Fatalf("expected error to mention unknown flag\n---\nstderr:\n%s\n---\nstdout:\n%s", stderr, stdout)
	}
}

func TestContract_DocsHelpSurface(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	stdout, _, err := runCLI(t, statePath, "docs")
	if err != nil {
		t.Fatalf("docs failed: %v\n%s", err, stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte("find")) || !bytes.Contains([]byte(stdout), []byte("show")) || !bytes.Contains([]byte(stdout), []byte("sync")) {
		t.Fatalf("expected docs help subcommands\n---\n%s", stdout)
	}
}

func TestContract_Version(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	stdout, _, err := runCLI(t, statePath, "version", "--pretty")
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, stdout)
	}
	e := decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("version expected ok=true")
	}
	if _, ok := e.Data["version"]; !ok {
		t.Fatalf("version expected data.version")
	}
}

func TestContract_MarketplaceRegistryAndDemand(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	// Registry search should return items.
	stdout, _, err := runCLI(t, statePath, "registry", "search", "subscription", "--pretty")
	if err != nil {
		t.Fatalf("registry search failed: %v\n%s", err, stdout)
	}
	e := decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("registry search expected ok=true")
	}
	itemsAny, ok := e.Data["items"]
	if !ok {
		t.Fatalf("registry search expected data.items")
	}
	items, ok := itemsAny.([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("registry search expected non-empty data.items")
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("registry search item expected object")
	}
	ref, _ := first["listingId"].(string)
	if ref == "" {
		t.Fatalf("registry search expected listingId")
	}

	// Registry show should include entry.
	stdout, _, err = runCLI(t, statePath, "registry", "show", ref, "--pretty")
	if err != nil {
		t.Fatalf("registry show failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("registry show expected ok=true")
	}
	if _, ok := e.Data["entry"]; !ok {
		t.Fatalf("registry show expected data.entry")
	}

	// Pricing show should include pricing.
	stdout, _, err = runCLI(t, statePath, "pricing", "show", ref, "--pretty")
	if err != nil {
		t.Fatalf("pricing show failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("pricing show expected ok=true")
	}
	if _, ok := e.Data["pricing"]; !ok {
		t.Fatalf("pricing show expected data.pricing")
	}

	// Demand clusters should be available.
	stdout, _, err = runCLI(t, statePath, "demand", "clusters", "--pretty")
	if err != nil {
		t.Fatalf("demand clusters failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("demand clusters expected ok=true")
	}
	if itemsAny, ok := e.Data["items"]; !ok || itemsAny == nil {
		t.Fatalf("demand clusters expected data.items")
	}

	// Demand ingest should succeed and be reflected in demand queries.
	stdout, _, err = runCLI(t, statePath, "demand", "ingest", "Need a daily sales report to Slack", "--offer-cents", "1500", "--currency", "USD", "--pretty")
	if err != nil {
		t.Fatalf("demand ingest failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("demand ingest expected ok=true")
	}
	if _, ok := e.Data["clusterId"]; !ok {
		t.Fatalf("demand ingest expected data.clusterId")
	}

	stdout, _, err = runCLI(t, statePath, "demand", "queries", "--limit", "5", "--pretty")
	if err != nil {
		t.Fatalf("demand queries failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("demand queries expected ok=true")
	}
	if _, ok := e.Data["items"]; !ok {
		t.Fatalf("demand queries expected data.items")
	}
}
