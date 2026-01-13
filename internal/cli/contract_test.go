package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestContract_FlowsCreateEditValidateCompile(t *testing.T) {
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

	// Add a step
	stdout, _, err = runCLI(t, statePath, "flows", "steps", "set", "contract-flow", "step-a", "--type", "code", "--title", "Step A", "--definition", "(step :code :step-a ...)", "--pretty")
	if err != nil {
		t.Fatalf("steps set failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("steps set expected ok=true")
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

	// Compile
	stdout, _, err = runCLI(t, statePath, "flows", "compile", "contract-flow", "--pretty")
	if err != nil {
		t.Fatalf("compile failed: %v\n%s", err, stdout)
	}
	e = decodeEnvelope(t, stdout)
	if !e.OK {
		t.Fatalf("compile expected ok=true")
	}
	if _, ok := e.Data["plan"]; !ok {
		t.Fatalf("compile expected data.plan")
	}
}

func TestContract_RunsStartAdvanceStepAndEvents(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	// Seed to ensure known flows exist.
	_ = os.Setenv("BREYTA_DEV", "1")
	defer os.Unsetenv("BREYTA_DEV")
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

func TestContract_EDNFormat(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	stdout, _, err := runCLI(t, statePath, "flows", "list", "--format", "edn", "--pretty")
	if err != nil {
		t.Fatalf("expected success, got error: %v\n%s", err, stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte(":ok true")) {
		t.Fatalf("expected EDN output to include ':ok true'\n---\n%s", stdout)
	}
}

func TestContract_DocsMarkdownOnDemand(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	stdout, _, err := runCLI(t, statePath, "docs", "runs", "list")
	if err != nil {
		t.Fatalf("docs failed: %v\n%s", err, stdout)
	}
	if !bytes.HasPrefix([]byte(stdout), []byte("## breyta runs list")) {
		t.Fatalf("expected markdown docs header\n---\n%s", stdout)
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
