package live

import (
	"strings"
	"testing"
	"time"
)

func leadingSpaces(value string) int {
	count := 0
	for _, r := range value {
		if r != ' ' {
			return count
		}
		count++
	}
	return count
}

func hasDiagnostic(diagnostics []RenderDiagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestRenderSnapshotShowsRunTreeActivitiesDurationsAndFanout(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	started := now.Add(-8 * time.Second)
	toolStarted := now.Add(-3 * time.Second)
	loopStarted := now.Add(-5 * time.Second)
	branchOne := 1
	branchTwo := 2
	iter := int64(4)
	total := int64(10)

	snapshot := Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:         "ws-acme",
			ActiveRunCount:      2,
			ActiveChildRunCount: 1,
			StepsRunning:        2,
			UpdatedAt:           now,
		},
		Runs: []RunState{
			{
				WorkspaceID:        "ws-acme",
				WorkflowID:         "wf-root",
				RootWorkflowID:     "wf-root",
				FlowSlug:           "root-flow",
				Status:             "running",
				Active:             true,
				CurrentStepID:      "fanout-customers",
				CurrentStepName:    "Fan out customers",
				CurrentStepType:    "fanout",
				CurrentStepStatus:  "running",
				StepsStarted:       3,
				StepsCompleted:     1,
				StepsExecutedTotal: 3,
				StepsRunning:       1,
				StartedAt:          &started,
				LastEventAt:        now,
				UpdatedAt:          now,
			},
			{
				WorkspaceID:       "ws-acme",
				WorkflowID:        "wf-child-a",
				RootWorkflowID:    "wf-root",
				ParentWorkflowID:  "wf-root",
				ParentStepID:      "fanout-customers",
				RelationKind:      "child_flow",
				FlowSlug:          "customer-agent",
				Status:            "running",
				AgentID:           "researcher",
				FanoutBranchIndex: &branchOne,
				Active:            true,
				CurrentStepID:     "lookup-crm",
				CurrentStepName:   "Lookup CRM",
				CurrentStepType:   "tool",
				StepsStarted:      1,
				StepsRunning:      1,
				LastEventAt:       now,
				UpdatedAt:         now,
			},
			{
				WorkspaceID:       "ws-acme",
				WorkflowID:        "wf-child-b",
				RootWorkflowID:    "wf-root",
				ParentWorkflowID:  "wf-root",
				ParentStepID:      "fanout-customers",
				RelationKind:      "child_flow",
				FlowSlug:          "customer-agent",
				Status:            "failed",
				FanoutBranchIndex: &branchTwo,
				Active:            false,
				StepsStarted:      1,
				StepsFailed:       1,
				LastEventAt:       now,
				UpdatedAt:         now,
			},
		},
		Relations: []RunRelation{
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-a", ParentStepID: "fanout-customers", RelationKind: "child_flow", FlowSlug: "customer-agent", AgentID: "researcher", FanoutBranchIndex: &branchOne, Active: true, Status: "running", CreatedAt: now.Add(-7 * time.Second), UpdatedAt: now},
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-b", ParentStepID: "fanout-customers", RelationKind: "child_flow", FlowSlug: "customer-agent", FanoutBranchIndex: &branchTwo, Active: false, Status: "failed", CreatedAt: now.Add(-6 * time.Second), UpdatedAt: now},
		},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "step:fanout", ActivityKind: "step", ActivityType: "fanout", ActivityName: "Fan out customers", Status: "running", Active: true, StepID: "fanout-customers", StartedAt: &started, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "loop:customers", ActivityKind: "loop", ActivityType: "loop", ActivityName: "Customer page loop", Status: "running", Active: true, ProgressCurrent: &iter, ProgressTotal: &total, StartedAt: &loopStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-a", RootWorkflowID: "wf-root", ActivityID: "tool:crm", ActivityKind: "tool_call", ActivityType: "crm.lookup", ActivityName: "Lookup CRM", Status: "running", Active: true, ToolCallID: "tool-crm-1", AgentID: "researcher", StartedAt: &toolStarted, UpdatedAt: now},
		},
	}
	out := RenderSnapshot(snapshot, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root"})

	for _, want := range []string{
		"wf-root",
		"1 child, 1 agent, 1 tool",
		"Fan out customers [fanout-customers] 7.0s",
		"  ⠹ ◉ researcher customer-agent [b1]",
		"Customer page loop 5.0s",
		"iter 4/10",
		"⚙ Lookup CRM",
		"@researcher",
		"8.0s",
		"3.0s",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\n---\n%s", want, out)
		}
	}
	for _, notWant := range []string{"├", "└", "│"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("expected indentation-only output, found %q\n---\n%s", notWant, out)
		}
	}
	if strings.Contains(out, "[tool:crm]") {
		t.Fatalf("expected tool row not to expose activity id as step detail\n---\n%s", out)
	}
}

func TestRenderSnapshotShowsResourcesAndSummaryStrip(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	started := now.Add(-2 * time.Second)
	size := int64(42 * 1024)
	rows := int64(7)

	snapshot := Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme"},
		Runs: []RunState{
			{
				WorkspaceID:        "ws-acme",
				WorkflowID:         "wf-root",
				RootWorkflowID:     "wf-root",
				FlowSlug:           "resource-flow",
				Status:             "completed",
				StepsStarted:       2,
				StepsCompleted:     2,
				StepsExecutedTotal: 2,
				LastEventAt:        now,
				UpdatedAt:          now,
			},
		},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "persist-report", ActivityKind: "step", ActivityType: "function", ActivityName: "Persist report", Status: "completed", StepID: "persist-report", StartedAt: &started, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "res://v1/ws/ws-acme/result/blob/report.md", ParentActivityID: "persist-report", ActivityKind: "resource", ActivityType: "blob", ActivityName: "report.md", Status: "completed", ResourceURI: "res://v1/ws/ws-acme/result/blob/report.md", ResourceKind: "blob", ResourceLabel: "report.md", ContentType: "text/markdown", SizeBytes: &size, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "res://v1/ws/ws-acme/result/table/findings", ParentActivityID: "persist-report", ActivityKind: "resource", ActivityType: "table", ActivityName: "findings", Status: "completed", ResourceURI: "res://v1/ws/ws-acme/result/table/findings", ResourceKind: "table", ResourceLabel: "findings", RowCount: &rows, UpdatedAt: now},
		},
	}
	out := RenderSnapshot(snapshot, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root"})

	for _, want := range []string{
		"▣ report.md 42.0KB text/markdown",
		"▣ findings 7 rows",
		"2 steps executed, 2 resources (▣ 2)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\n---\n%s", want, out)
		}
	}
}

func TestRenderSnapshotShowsRootRunDurationInHeaderAndSummary(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	started := now.Add(-75 * time.Second)
	completed := now.Add(-5 * time.Second)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme"},
		Runs: []RunState{{
			WorkspaceID:        "ws-acme",
			WorkflowID:         "wf-root",
			RootWorkflowID:     "wf-root",
			FlowSlug:           "duration-flow",
			Status:             "completed",
			StepsCompleted:     1,
			StepsExecutedTotal: 1,
			StartedAt:          &started,
			CompletedAt:        &completed,
			UpdatedAt:          completed,
		}},
		Nodes: []Activity{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root",
			ActivityID: "prepare-run", ActivityKind: "step", ActivityType: "sleep",
			ActivityName: "prepare-run", StepID: "prepare-run", Status: "completed",
			StartedAt: &started, CompletedAt: &completed, UpdatedAt: completed,
		}},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header and summary lines\n---\n%s", out)
	}
	if !strings.Contains(lines[0], "1m10s") {
		t.Fatalf("expected header to include total run duration\n---\n%s", out)
	}
	if !strings.Contains(lines[len(lines)-1], "1m10s total") {
		t.Fatalf("expected summary to include total run duration\n---\n%s", out)
	}
}

func TestRenderSnapshotFallsBackToActivityRangeForRunDuration(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	started := now.Add(-5 * time.Second)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "duration-flow",
			Status:         "running",
			Active:         true,
			UpdatedAt:      now,
		}},
		Nodes: []Activity{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root",
			ActivityID: "prepare-run", ActivityKind: "step", ActivityType: "sleep",
			ActivityName: "prepare-run", StepID: "prepare-run", Status: "running", Active: true,
			StartedAt: &started, UpdatedAt: now,
		}},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], "5.0s") {
		t.Fatalf("expected header to use activity range duration when run start is missing\n---\n%s", out)
	}
}

func TestRenderSnapshotShowsGraphSkeletonBeforeRuntimeSteps(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	started := now.Add(-2 * time.Second)

	snapshot := Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "root-flow",
			Status:         "running",
			Active:         true,
			UpdatedAt:      now,
		}},
		Nodes: []Activity{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root",
			ActivityID: "step:collect", ActivityKind: "step", ActivityType: "function",
			ActivityName: "Collect", StepID: "collect", Status: "running", Active: true,
			StartedAt: &started, UpdatedAt: now,
		}},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "root-flow",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:root-flow", Nodes: []FlowGraphNode{
				{ID: "flow:root-flow", Kind: "flow", Label: "root-flow", Order: 1},
				{ID: "step:prepare", Kind: "step", Label: "Prepare", StepID: "prepare", StepType: "sleep", ParentID: "flow:root-flow", Order: 2},
				{ID: "step:collect", Kind: "step", Label: "Collect", StepID: "collect", StepType: "function", ParentID: "flow:root-flow", Order: 3},
				{ID: "step:persist", Kind: "step", Label: "Persist", StepID: "persist", StepType: "function", ParentID: "flow:root-flow", Order: 4},
			}},
		}},
	}
	out := RenderSnapshot(snapshot, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	prepareIdx := strings.Index(out, "Prepare")
	collectIdx := strings.Index(out, "Collect")
	persistIdx := strings.Index(out, "Persist")
	if prepareIdx < 0 || collectIdx < 0 || persistIdx < 0 {
		t.Fatalf("expected planned and runtime graph rows\n---\n%s", out)
	}
	if !(prepareIdx < collectIdx && collectIdx < persistIdx) {
		t.Fatalf("expected graph order Prepare -> Collect -> Persist\n---\n%s", out)
	}
	if strings.Count(out, "Collect") != 1 {
		t.Fatalf("expected runtime step to hydrate graph row instead of duplicating it\n---\n%s", out)
	}
}

func TestRenderSnapshotOrdersRuntimeOnlyStepWithinGraphSkeleton(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	pageStarted := now.Add(-9 * time.Second)
	pageDone := now.Add(-8 * time.Second)
	dynamicStarted := now.Add(-7 * time.Second)
	dynamicDone := now.Add(-6 * time.Second)
	branchStarted := now.Add(-2 * time.Second)

	snapshot := Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-child",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-child",
			Status:         "running",
			Active:         true,
			CurrentStepID:  "child-success-path",
			UpdatedAt:      now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child", RootWorkflowID: "wf-root", ActivityID: "loop-page-3", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-3", StepID: "loop-page-3", Status: "completed", StartedAt: &pageStarted, CompletedAt: &pageDone, UpdatedAt: pageDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child", RootWorkflowID: "wf-root", ActivityID: "runtime-branch-1", ActivityKind: "step", ActivityType: "sleep", ActivityName: "runtime-branch-1", StepID: "runtime-branch-1", Status: "completed", StartedAt: &dynamicStarted, CompletedAt: &dynamicDone, UpdatedAt: dynamicDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child", RootWorkflowID: "wf-root", ActivityID: "child-success-path", ActivityKind: "step", ActivityType: "sleep", ActivityName: "child-success-path", StepID: "child-success-path", Status: "running", Active: true, StartedAt: &branchStarted, UpdatedAt: now},
		},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-child",
			FlowSlug:   "live-render-child",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:live-render-child", Nodes: []FlowGraphNode{
				{ID: "flow:live-render-child", Kind: "flow", Label: "live-render-child", Order: 1},
				{ID: "step:loop-page-3", Kind: "step", Label: "Loop page 3/3", StepID: "loop-page-3", StepType: "sleep", ParentID: "flow:live-render-child", Order: 4},
				{ID: "step:child-success-path", Kind: "step", Label: "Child success path", StepID: "child-success-path", StepType: "sleep", ParentID: "flow:live-render-child", Order: 5},
				{ID: "step:child-work", Kind: "step", Label: "Finish child branch", StepID: "child-work", StepType: "function", ParentID: "flow:live-render-child", Order: 6},
			}},
		}},
	}
	out := RenderSnapshot(snapshot, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-child", FullTree: true})

	pageIdx := strings.Index(out, "loop-page-3")
	dynamicIdx := strings.Index(out, "runtime-branch-1")
	branchIdx := strings.Index(out, "child-success-path")
	if pageIdx < 0 || dynamicIdx < 0 || branchIdx < 0 {
		t.Fatalf("expected page, dynamic runtime, and branch rows\n---\n%s", out)
	}
	if !(pageIdx < dynamicIdx && dynamicIdx < branchIdx) {
		t.Fatalf("expected runtime-only step to render between executed graph neighbors\n---\n%s", out)
	}
	if strings.Contains(out, "○ s runtime-branch-1") {
		t.Fatalf("expected runtime-only step not to render as unstarted skeleton\n---\n%s", out)
	}
}

func TestRenderSnapshotOrdersRuntimeOnlyStepAfterPlannedGraphParent(t *testing.T) {
	now := time.Date(2026, 5, 31, 13, 25, 0, 0, time.UTC)
	runtimeStarted := now.Add(-1 * time.Second)
	runtimeDone := now.Add(-500 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-parent",
			Status:         "running",
			Active:         true,
			CurrentStepID:  "runtime-branch",
			StepsRunning:   1,
			StepsStarted:   1,
			StepsCompleted: 1,
			LastEventAt:    runtimeDone,
			StartedAt:      &runtimeStarted,
			UpdatedAt:      now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "runtime-branch", ParentActivityID: "branch-choice", ActivityKind: "step", ActivityType: "sleep", ActivityName: "runtime-branch", StepID: "runtime-branch", Status: "completed", StartedAt: &runtimeStarted, CompletedAt: &runtimeDone, UpdatedAt: runtimeDone},
		},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "live-render-parent",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:live-render-parent", Nodes: []FlowGraphNode{
				{ID: "flow:live-render-parent", Kind: "flow", Label: "live-render-parent", Order: 1},
				{ID: "step:prepare-run", Kind: "step", Label: "Prepare run", StepID: "prepare-run", StepType: "sleep", ParentID: "flow:live-render-parent", Order: 2},
				{ID: "branch:branch-choice", Kind: "branch", Label: "Branch choice", StepID: "branch-choice", BranchType: "if", ParentID: "flow:live-render-parent", Order: 3},
				{ID: "step:after-branch", Kind: "step", Label: "After branch", StepID: "after-branch", StepType: "sleep", ParentID: "flow:live-render-parent", Order: 4},
			}},
		}},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	branchIdx := strings.Index(out, "Branch choice")
	runtimeIdx := strings.Index(out, "runtime-branch")
	afterIdx := strings.Index(out, "After branch")
	if branchIdx < 0 || runtimeIdx < 0 || afterIdx < 0 {
		t.Fatalf("expected branch skeleton, runtime-only step, and following skeleton rows\n---\n%s", out)
	}
	if !(branchIdx < runtimeIdx && runtimeIdx < afterIdx) {
		t.Fatalf("expected runtime-only step to inherit graph order from planned parent\n---\n%s", out)
	}
}

func TestRenderSnapshotHydratesDecidedBranchAndSuppressesUntakenPath(t *testing.T) {
	now := time.Date(2026, 5, 31, 13, 28, 0, 0, time.UTC)
	branchStarted := now.Add(-2 * time.Second)
	branchDone := now.Add(-500 * time.Millisecond)
	var diagnostics []RenderDiagnostic

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-parent",
			Status:         "running",
			Active:         true,
			CurrentStepID:  "research-agent",
			StepsRunning:   1,
			StepsStarted:   2,
			StepsCompleted: 1,
			LastEventAt:    branchDone,
			StartedAt:      &branchStarted,
			UpdatedAt:      now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "case-id-present", ActivityKind: "step", ActivityType: "sleep", ActivityName: "case-id-present", StepID: "case-id-present", Status: "completed", StartedAt: &branchStarted, CompletedAt: &branchDone, UpdatedAt: branchDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "running", Active: true, StartedAt: &branchDone, UpdatedAt: now},
		},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "live-render-parent",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:live-render-parent", Nodes: []FlowGraphNode{
				{ID: "flow:live-render-parent", Kind: "flow", Label: "live-render-parent", Order: 1},
				{ID: "branch:case-id-branch", Kind: "branch", Label: "Case id branch", StepID: "case-id-branch", BranchType: "if", ParentID: "flow:live-render-parent", Order: 2},
				{ID: "step:case-id-present", Kind: "step", Label: "Case id present", StepID: "case-id-present", StepType: "sleep", ParentID: "branch:case-id-branch", ScopeID: "branch:case-id-branch:scope:0", Order: 3},
				{ID: "step:case-id-missing", Kind: "step", Label: "Case id missing", StepID: "case-id-missing", StepType: "sleep", ParentID: "branch:case-id-branch", ScopeID: "branch:case-id-branch:scope:1", Order: 4},
				{ID: "step:research-agent", Kind: "step", Label: "Research agent", StepID: "research-agent", StepType: "mock/researcher", ParentID: "flow:live-render-parent", Order: 5},
			}},
		}},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true, Diagnostics: func(diagnostic RenderDiagnostic) {
		diagnostics = append(diagnostics, diagnostic)
	}})

	branchIdx := strings.Index(out, "◇ Case id branch")
	presentIdx := strings.Index(out, "case-id-present")
	researchIdx := strings.Index(out, "research-agent")
	if branchIdx < 0 || presentIdx < 0 || researchIdx < 0 {
		t.Fatalf("expected decided branch, chosen step, and following runtime step\n---\n%s", out)
	}
	if !(branchIdx < presentIdx && presentIdx < researchIdx) {
		t.Fatalf("expected chosen branch step nested before following runtime step\n---\n%s", out)
	}
	if strings.Contains(out, "○ ◇ Case id branch") {
		t.Fatalf("expected branch container to be hydrated, not planned gray/pending\n---\n%s", out)
	}
	if strings.Contains(out, "Case id missing") || strings.Contains(out, "case-id-missing") {
		t.Fatalf("expected untaken planned branch path to be suppressed\n---\n%s", out)
	}
	if !hasDiagnostic(diagnostics, "live.render.suppress_untaken_branch") {
		t.Fatalf("expected untaken branch suppression diagnostic, got %#v", diagnostics)
	}
}

func TestRenderSnapshotSuppressesGenericRuntimeFanoutRows(t *testing.T) {
	now := time.Date(2026, 5, 31, 13, 30, 0, 0, time.UTC)
	agentStarted := now.Add(-12 * time.Second)
	toolStarted := now.Add(-10 * time.Second)
	fanoutStarted := now.Add(-9 * time.Second)
	spawnStarted := now.Add(-5 * time.Second)
	branchZero := 0
	branchOne := 1

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 3, ActiveChildRunCount: 2, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "running", Active: true, CurrentStepID: "spawn-children", UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "fanout", FlowSlug: "live-render-parent", Status: "completed", AgentID: "researcher", FanoutBranchIndex: &branchZero, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b1", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "fanout", FlowSlug: "live-render-parent", Status: "completed", AgentID: "auditor", FanoutBranchIndex: &branchOne, UpdatedAt: now},
		},
		Relations: []RunRelation{
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-agent-b0", ParentStepID: "fanout", RelationKind: "agent_fanout", FlowSlug: "live-render-parent", AgentID: "researcher", FanoutBranchIndex: &branchZero, Active: false, Status: "completed", CreatedAt: fanoutStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-agent-b1", ParentStepID: "fanout", RelationKind: "agent_fanout", FlowSlug: "live-render-parent", AgentID: "auditor", FanoutBranchIndex: &branchOne, Active: false, Status: "completed", CreatedAt: fanoutStarted, UpdatedAt: now},
		},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "agent-internal-fanout", ActivityKind: "step", ActivityType: "mock/fanout-manager", ActivityName: "agent-internal-fanout", StepID: "agent-internal-fanout", Status: "completed", StartedAt: &agentStarted, CompletedAt: &toolStarted, UpdatedAt: toolStarted},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:call-agent-fanout", ParentActivityID: "agent-internal-fanout", ActivityKind: "tool_call", ActivityType: "mock_spawn_agent_fanout", ActivityName: "mock_spawn_agent_fanout", ToolCallID: "call-agent-fanout", Status: "completed", StartedAt: &toolStarted, CompletedAt: &fanoutStarted, UpdatedAt: fanoutStarted},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "fanout", ParentActivityID: "call-agent-fanout", ActivityKind: "fanout", ActivityName: "fanout", Status: "completed", StartedAt: &fanoutStarted, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "spawn-children", ActivityKind: "step", ActivityType: "fanout", ActivityName: "spawn-children", StepID: "spawn-children", Status: "running", Active: true, StartedAt: &spawnStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ActivityID: "agent-branch-0", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "agent-branch-0", StepID: "agent-branch-0", Status: "completed", AgentID: "researcher", StartedAt: &fanoutStarted, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b1", RootWorkflowID: "wf-root", ActivityID: "agent-branch-1", ActivityKind: "step", ActivityType: "mock/auditor", ActivityName: "agent-branch-1", StepID: "agent-branch-1", Status: "completed", AgentID: "auditor", StartedAt: &fanoutStarted, CompletedAt: &now, UpdatedAt: now},
		},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "live-render-parent",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:live-render-parent", Nodes: []FlowGraphNode{
				{ID: "flow:live-render-parent", Kind: "flow", Label: "live-render-parent", Order: 1},
				{ID: "step:agent-internal-fanout", Kind: "step", Label: "Agent internal fanout", StepID: "agent-internal-fanout", StepType: "mock/fanout-manager", ParentID: "flow:live-render-parent", Order: 2},
				{ID: "step:spawn-children", Kind: "step", Label: "Spawn child flows", StepID: "spawn-children", StepType: "fanout", ParentID: "flow:live-render-parent", Order: 3},
				{ID: "step:collect-pause", Kind: "step", Label: "Pause before collect", StepID: "collect-pause", StepType: "sleep", ParentID: "flow:live-render-parent", Order: 4},
				{ID: "step:collect-fanout", Kind: "step", Label: "Collect fanout", StepID: "collect-fanout", StepType: "function", ParentID: "flow:live-render-parent", Order: 5},
				{ID: "step:persist-run-report", Kind: "step", Label: "Persist run report", StepID: "persist-run-report", StepType: "function", ParentID: "flow:live-render-parent", Order: 6},
			}},
		}},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	if strings.Contains(out, "✣ fanout") {
		t.Fatalf("expected generic runtime fanout row to be suppressed\n---\n%s", out)
	}
	agentIdx := strings.Index(out, "agent-internal-fanout")
	branchIdx := strings.Index(out, "researcher [b0]")
	spawnIdx := strings.Index(out, "spawn-children")
	if agentIdx < 0 || branchIdx < 0 || spawnIdx < 0 {
		t.Fatalf("expected semantic agent, branch, and spawn rows\n---\n%s", out)
	}
	if !(agentIdx < branchIdx && branchIdx < spawnIdx) {
		t.Fatalf("expected generic fanout children near the parent agent tool, before later graph rows\n---\n%s", out)
	}
}

func TestRenderSnapshotSuppressesUnanchoredGenericFanoutWithDiagnostics(t *testing.T) {
	now := time.Date(2026, 5, 31, 13, 32, 0, 0, time.UTC)
	started := now.Add(-2 * time.Second)
	branchZero := 0
	var diagnostics []RenderDiagnostic

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 2, ActiveChildRunCount: 1, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "running", Active: true, CurrentStepID: "runtime-fanout-wrapper", UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "fanout", FlowSlug: "live-render-parent", Status: "running", Active: true, AgentID: "researcher", FanoutBranchIndex: &branchZero, UpdatedAt: now},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root",
			ChildWorkflowID: "wf-agent-b0", ParentStepID: "fanout", RelationKind: "agent_fanout",
			FlowSlug: "live-render-parent", AgentID: "researcher", FanoutBranchIndex: &branchZero,
			Active: true, Status: "running", CreatedAt: started, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "runtime-fanout-wrapper", ActivityKind: "step", ActivityType: "fanout", ActivityName: "fanout", Status: "completed", StartedAt: &started, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ActivityID: "agent-branch-0", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "agent-branch-0", StepID: "agent-branch-0", Status: "running", Active: true, AgentID: "researcher", StartedAt: &started, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true, Diagnostics: func(diagnostic RenderDiagnostic) {
		diagnostics = append(diagnostics, diagnostic)
	}})

	if strings.Contains(out, "✣ fanout") {
		t.Fatalf("expected generic fanout wrapper row to be suppressed\n---\n%s", out)
	}
	if strings.Contains(out, "◉ researcher [b0]") {
		t.Fatalf("expected child run with only generic fanout parent to be suppressed\n---\n%s", out)
	}
	if !hasDiagnostic(diagnostics, "live.render.suppress_unanchored_generic_fanout") ||
		!hasDiagnostic(diagnostics, "live.render.suppress_unanchored_generic_fanout_child") {
		t.Fatalf("expected generic fanout suppression diagnostics, got %#v", diagnostics)
	}
}

func TestRenderSnapshotDoesNotSuppressNamedFanoutStepLabeledFanout(t *testing.T) {
	now := time.Date(2026, 5, 31, 13, 34, 0, 0, time.UTC)
	started := now.Add(-2 * time.Second)
	branchZero := 0

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 2, ActiveChildRunCount: 1, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "running", Active: true, CurrentStepID: "spawn-subagents", UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-subagents", FlowSlug: "live-render-parent", Status: "running", Active: true, AgentID: "researcher", FanoutBranchIndex: &branchZero, UpdatedAt: now},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root",
			ChildWorkflowID: "wf-agent-b0", ParentStepID: "spawn-subagents", RelationKind: "agent_fanout",
			FlowSlug: "live-render-parent", AgentID: "researcher", FanoutBranchIndex: &branchZero,
			Active: true, Status: "running", CreatedAt: started, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "spawn-subagents", ActivityKind: "step", ActivityType: "fanout", ActivityName: "fanout", StepID: "spawn-subagents", Status: "running", Active: true, StartedAt: &started, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ActivityID: "agent-branch-0", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "agent-branch-0", StepID: "agent-branch-0", Status: "running", Active: true, AgentID: "researcher", StartedAt: &started, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	fanoutIdx := strings.Index(out, "✣ fanout [spawn-subagents]")
	branchIdx := strings.Index(out, "◉ researcher [b0]")
	if fanoutIdx < 0 || branchIdx < 0 {
		t.Fatalf("expected named fanout row and branch row\n---\n%s", out)
	}
	if fanoutIdx > branchIdx {
		t.Fatalf("expected branch to remain nested under named fanout row\n---\n%s", out)
	}
}

func TestRenderSnapshotShowsOrphanPersistedResource(t *testing.T) {
	now := time.Date(2026, 5, 31, 13, 40, 0, 0, time.UTC)
	size := int64(226)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:        "ws-acme",
			WorkflowID:         "wf-root",
			RootWorkflowID:     "wf-root",
			FlowSlug:           "live-render-parent",
			Status:             "completed",
			StepsCompleted:     1,
			StepsExecutedTotal: 1,
			UpdatedAt:          now,
		}},
		Nodes: []Activity{{
			WorkspaceID:      "ws-acme",
			WorkflowID:       "wf-root",
			RootWorkflowID:   "wf-root",
			ActivityID:       "res://v1/ws/ws-acme/result/blob/report.md",
			ParentActivityID: "persist-run-report",
			ActivityKind:     "resource",
			ActivityType:     "blob",
			ActivityName:     "agent-subagent-tools-local-run-report.md",
			Status:           "completed",
			ResourceURI:      "res://v1/ws/ws-acme/result/blob/report.md",
			ResourceKind:     "blob",
			ResourceLabel:    "agent-subagent-tools-local-run-report.md",
			ContentType:      "text/markdown",
			SizeBytes:        &size,
			UpdatedAt:        now,
		}},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	if !strings.Contains(out, "agent-subagent-tools-local-run-repo") || !strings.Contains(out, "226B text/markdown") {
		t.Fatalf("expected orphan persisted resource row\n---\n%s", out)
	}
}

func TestRenderSnapshotDeduplicatesResourcesByURI(t *testing.T) {
	now := time.Date(2026, 5, 31, 13, 45, 0, 0, time.UTC)
	started := now.Add(-1 * time.Second)
	size := int64(226)
	resourceURI := "res://v1/ws/ws-acme/result/blob/report.md"

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:        "ws-acme",
			WorkflowID:         "wf-root",
			RootWorkflowID:     "wf-root",
			FlowSlug:           "live-render-parent",
			Status:             "completed",
			StepsCompleted:     1,
			StepsExecutedTotal: 1,
			UpdatedAt:          now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "persist-run-report", ActivityKind: "step", ActivityType: "function", ActivityName: "persist-run-report", StepID: "persist-run-report", Status: "completed", StartedAt: &started, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "resource-event-1", ParentActivityID: "persist-run-report", ActivityKind: "resource", ActivityType: "blob", ActivityName: "agent-subagent-tools-local-run-report.md", Status: "completed", ResourceURI: resourceURI, ResourceKind: "blob", ResourceLabel: "agent-subagent-tools-local-run-report.md", ContentType: "text/markdown", SizeBytes: &size, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "resource-event-2", ParentActivityID: "persist-run-report", ActivityKind: "resource", ActivityType: "blob", ActivityName: "agent-subagent-tools-local-run-report.md", Status: "completed", ResourceURI: resourceURI, ResourceKind: "blob", ResourceLabel: "agent-subagent-tools-local-run-report.md", ContentType: "text/markdown", SizeBytes: &size, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	if strings.Count(out, "agent-subagent-tools-local-run-repo") != 1 {
		t.Fatalf("expected one resource row for duplicate resource URI\n---\n%s", out)
	}
	if strings.Count(out, "▣ 1") != 1 {
		t.Fatalf("expected summary to count duplicate resource URI once\n---\n%s", out)
	}
}

func TestRenderSnapshotHidesFutureSkeletonAfterTerminalRun(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	started := now.Add(-2 * time.Second)
	completed := now.Add(-1 * time.Second)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "root-flow",
			Status:         "failed",
			Active:         false,
			UpdatedAt:      completed,
		}},
		Nodes: []Activity{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root",
			ActivityID: "step:collect", ActivityKind: "step", ActivityType: "function",
			ActivityName: "Collect", StepID: "collect", Status: "failed", Active: false,
			StartedAt: &started, CompletedAt: &completed, UpdatedAt: completed,
		}},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "root-flow",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:root-flow", Nodes: []FlowGraphNode{
				{ID: "flow:root-flow", Kind: "flow", Label: "root-flow", Order: 1},
				{ID: "step:collect", Kind: "step", Label: "Collect", StepID: "collect", StepType: "function", ParentID: "flow:root-flow", Order: 2},
				{ID: "step:persist", Kind: "step", Label: "Persist run report", StepID: "persist-run-report", StepType: "function", ParentID: "flow:root-flow", Order: 3},
			}},
		}},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	if !strings.Contains(out, "Collect") {
		t.Fatalf("expected executed failed row to remain\n---\n%s", out)
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if !strings.Contains(line, "Collect") {
			continue
		}
		if strings.Contains(line, "✗") {
			t.Fatalf("expected failed activity row not to use a failure glyph, got %q\n---\n%s", line, out)
		}
		if !strings.HasSuffix(line, "failed") {
			t.Fatalf("expected failed activity row to end with failed status, got %q\n---\n%s", line, out)
		}
		break
	}
	if strings.Contains(out, "Persist run report") || strings.Contains(out, "persist-run-report") {
		t.Fatalf("expected unexecuted future skeleton row to be hidden after terminal run\n---\n%s", out)
	}
}

func TestRenderSnapshotNestsPlannedFanoutItemsUnderFanoutStep(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "root-flow",
			Status:         "running",
			Active:         true,
			UpdatedAt:      now,
		}},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "root-flow",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:root-flow", Nodes: []FlowGraphNode{
				{ID: "flow:root-flow", Kind: "flow", Label: "root-flow", Order: 1},
				{ID: "step:spawn-subagents", Kind: "step", Label: "Spawn subagents", StepID: "spawn-subagents", StepType: "fanout", ParentID: "flow:root-flow", Order: 2},
				{ID: "step:spawn-subagents:item:0", Kind: "agent", Label: "mock/researcher", ParentID: "step:spawn-subagents", AgentID: "mock/researcher", Order: 3},
				{ID: "step:spawn-subagents:item:1", Kind: "call-flow", Label: "child-flow", ParentID: "step:spawn-subagents", FlowSlug: "child-flow", Order: 4},
			}},
		}},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	lines := strings.Split(out, "\n")
	spawnLine := -1
	agentLine := -1
	childLine := -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "Spawn"):
			spawnLine = i
		case strings.Contains(line, "mock/researcher"):
			agentLine = i
		case strings.Contains(line, "child-flow"):
			childLine = i
		}
	}
	if spawnLine < 0 || agentLine < 0 || childLine < 0 {
		t.Fatalf("expected planned fanout step and children\n---\n%s", out)
	}
	if !strings.Contains(lines[spawnLine], "[spawn-subagents]") {
		t.Fatalf("expected planned fanout row to include step id, got %q\n---\n%s", lines[spawnLine], out)
	}
	if !(spawnLine < agentLine && spawnLine < childLine) {
		t.Fatalf("expected planned fanout children after parent\n---\n%s", out)
	}
	if !(leadingSpaces(lines[agentLine]) > leadingSpaces(lines[spawnLine]) &&
		leadingSpaces(lines[childLine]) > leadingSpaces(lines[spawnLine])) {
		t.Fatalf("expected planned fanout children to be indented under parent\n---\n%s", out)
	}
}

func TestRenderSnapshotSuppressesPlannedFanoutItemsWhenRuntimeChildrenExist(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 10, 0, time.UTC)
	branchZero := 0
	branchOne := 1

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 3, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "root-flow", Status: "running", Active: true, CurrentStepID: "spawn-subagents", UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-0", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-subagents", FlowSlug: "root-flow", AgentID: "researcher", FanoutBranchIndex: &branchZero, Status: "running", Active: true, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-1", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-subagents", FlowSlug: "root-flow", AgentID: "auditor", FanoutBranchIndex: &branchOne, Status: "running", Active: true, UpdatedAt: now},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-agent-0",
			ParentStepID: "spawn-subagents", RelationKind: "agent_fanout", FlowSlug: "root-flow", AgentID: "researcher", FanoutBranchIndex: &branchZero, Active: true, Status: "running", CreatedAt: now, UpdatedAt: now,
		}, {
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-agent-1",
			ParentStepID: "spawn-subagents", RelationKind: "agent_fanout", FlowSlug: "root-flow", AgentID: "auditor", FanoutBranchIndex: &branchOne, Active: true, Status: "running", CreatedAt: now, UpdatedAt: now,
		}},
		Nodes: []Activity{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root",
			ActivityID: "spawn-subagents", ActivityKind: "step", ActivityType: "fanout", ActivityName: "spawn-subagents", StepID: "spawn-subagents", Status: "running", Active: true, StartedAt: &now, UpdatedAt: now,
		}},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "root-flow",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:root-flow", Nodes: []FlowGraphNode{
				{ID: "flow:root-flow", Kind: "flow", Label: "root-flow", Order: 1},
				{ID: "step:spawn-subagents", Kind: "step", Label: "Spawn subagents", StepID: "spawn-subagents", StepType: "fanout", ParentID: "flow:root-flow", Order: 2},
				{ID: "step:spawn-subagents:item:0", Kind: "agent", Label: "mock/researcher", ParentID: "step:spawn-subagents", AgentID: "mock/researcher", Order: 3},
				{ID: "step:spawn-subagents:item:1", Kind: "agent", Label: "mock/auditor", ParentID: "step:spawn-subagents", AgentID: "mock/auditor", Order: 4},
			}},
		}},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	for _, notWant := range []string{
		"spawn-subagents:item",
		"mock/researcher [",
		"mock/auditor [",
	} {
		if strings.Contains(out, notWant) {
			t.Fatalf("expected planned fanout item %q to disappear once real runtime branches exist\n---\n%s", notWant, out)
		}
	}
	if !strings.Contains(out, "researcher [b0]") || !strings.Contains(out, "auditor [b1]") {
		t.Fatalf("expected real runtime fanout branches to remain\n---\n%s", out)
	}
}

func TestRenderSnapshotSuppressesPlannedFanoutCallFlowItemsWhenRuntimeChildrenExist(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 2, 10, 0, time.UTC)
	branchZero := 0
	branchOne := 1
	branchTwo := 2

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 4, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "root-flow", Status: "running", Active: true, CurrentStepID: "spawn-children", UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-0", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", FanoutBranchIndex: &branchZero, Status: "running", Active: true, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-1", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", FanoutBranchIndex: &branchOne, Status: "running", Active: true, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-2", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", FanoutBranchIndex: &branchTwo, Status: "running", Active: true, UpdatedAt: now},
		},
		Relations: []RunRelation{
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-0", ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &branchZero, Active: true, Status: "running", CreatedAt: now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-1", ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &branchOne, Active: true, Status: "running", CreatedAt: now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-2", ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &branchTwo, Active: true, Status: "running", CreatedAt: now, UpdatedAt: now},
		},
		Nodes: []Activity{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root",
			ActivityID: "spawn-children", ActivityKind: "step", ActivityType: "fanout", ActivityName: "spawn-children", StepID: "spawn-children", Status: "running", Active: true, StartedAt: &now, UpdatedAt: now,
		}},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "root-flow",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:root-flow", Nodes: []FlowGraphNode{
				{ID: "flow:root-flow", Kind: "flow", Label: "root-flow", Order: 1},
				{ID: "step:spawn-children", Kind: "step", Label: "Spawn children", StepID: "spawn-children", StepType: "fanout", ParentID: "flow:root-flow", Order: 2},
				{ID: "step:spawn-children:item:0", Kind: "call-flow", Label: "live-render-child", ParentID: "step:spawn-children", FlowSlug: "live-render-child", Order: 3},
				{ID: "step:spawn-children:item:1", Kind: "call-flow", Label: "live-render-child", ParentID: "step:spawn-children", FlowSlug: "live-render-child", Order: 4},
				{ID: "step:spawn-children:item:2", Kind: "call-flow", Label: "live-render-child", ParentID: "step:spawn-children", FlowSlug: "live-render-child", Order: 5},
				{ID: "step:collect-pause", Kind: "step", Label: "Pause before collect", StepID: "collect-pause", StepType: "sleep", ParentID: "flow:root-flow", Order: 6},
			}},
		}},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	for _, notWant := range []string{"spawn-children:item", "live-render-child [spawn-children:item"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("expected planned child-flow fanout item %q to disappear once runtime children exist\n---\n%s", notWant, out)
		}
	}
	if strings.Count(out, "live-render-child [b") != 3 {
		t.Fatalf("expected real runtime child flow branches to remain\n---\n%s", out)
	}
}

func TestRenderSnapshotRendersUnstartedSkeletonRowsGray(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "root-flow",
			Status:         "running",
			Active:         true,
			UpdatedAt:      now,
		}},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "root-flow",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:root-flow", Nodes: []FlowGraphNode{
				{ID: "flow:root-flow", Kind: "flow", Label: "root-flow", Order: 1},
				{ID: "step:prepare", Kind: "step", Label: "Prepare", StepID: "prepare", StepType: "sleep", ParentID: "flow:root-flow", Order: 2},
			}},
		}},
	}, RenderOptions{Now: now, Frame: 2, Color: true, FocusWorkflowID: "wf-root", FullTree: true})

	if !strings.Contains(out, "\x1b[90m ○s Prepare\x1b[0m") {
		t.Fatalf("expected unstarted planned row to be rendered fully gray\n---\n%q", out)
	}
	if strings.Contains(out, "\x1b[36m○") {
		t.Fatalf("expected unstarted planned status glyph not to use pending cyan\n---\n%q", out)
	}
}

func TestRenderSnapshotSummaryStripUsesAccentsWhenColored(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme"},
		Runs: []RunState{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "resource-flow",
			Status: "completed", StepsCompleted: 27, StepsFailed: 1, StepsExecutedTotal: 28, UpdatedAt: now,
		}},
		Nodes: []Activity{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root",
			ActivityID: "res://v1/ws/ws-acme/result/blob/report.md", ActivityKind: "resource", ActivityType: "blob",
			Status: "completed", ResourceKind: "blob", ResourceLabel: "report.md", UpdatedAt: now,
		}},
	}, RenderOptions{Now: now, Frame: 2, Color: true, FocusWorkflowID: "wf-root"})

	for _, want := range []string{
		"\x1b[32m28\x1b[0m steps executed",
		"\x1b[31m1 failed step\x1b[0m",
		"\x1b[36m1\x1b[0m resource",
		"(\x1b[36m▣\x1b[0m \x1b[36m1\x1b[0m)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected accented summary to contain %q\n---\n%q", want, out)
		}
	}
}

func TestRenderSnapshotKeepsReadableWorkflowIDInHeader(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	workflowID := "flow-live-render-parent-ws-acme-v2-r8"

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme"},
		Runs: []RunState{
			{
				WorkspaceID:    "ws-acme",
				WorkflowID:     workflowID,
				RootWorkflowID: workflowID,
				FlowSlug:       "live-render-parent",
				Status:         "running",
				Active:         true,
				UpdatedAt:      now,
			},
		},
	}, RenderOptions{Now: now, Frame: 1, Color: false, FocusWorkflowID: workflowID})

	if !strings.Contains(out, workflowID) {
		t.Fatalf("expected header to keep readable workflow id %q\n---\n%s", workflowID, out)
	}
	if strings.Contains(out, "flow-liv") && strings.Contains(out, "…-r8") {
		t.Fatalf("expected header not to truncate common workflow ids so aggressively\n---\n%s", out)
	}
}

func TestRenderSnapshotFocusHidesUnrelatedWorkspaceRuns(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			StepsRunning:   1,
			UpdatedAt:      now,
		},
		Runs: []RunState{
			{
				WorkspaceID:    "ws-acme",
				WorkflowID:     "wf-old",
				RootWorkflowID: "wf-old",
				FlowSlug:       "old-flow",
				Status:         "running",
				Active:         true,
				StepsRunning:   1,
				UpdatedAt:      now,
			},
		},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-old", RootWorkflowID: "wf-old", ActivityID: "old-step", ActivityKind: "step", ActivityType: "http", ActivityName: "old-step", Status: "running", Active: true, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 1, Color: false, FocusWorkflowID: "wf-new"})

	for _, notWant := range []string{"old-flow", "old-step", "1 run", "1 step"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("expected focused live output to hide unrelated workspace state %q\n---\n%s", notWant, out)
		}
	}
	if !strings.Contains(out, "Waiting for run updates...") {
		t.Fatalf("expected focused live output to wait for the requested run\n---\n%s", out)
	}
}

func TestRenderSnapshotNestsToolCallsUnderAgentStep(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	agentStarted := now.Add(-2 * time.Second)
	agentDone := now.Add(-1200 * time.Millisecond)
	toolOneStarted := now.Add(-1100 * time.Millisecond)
	toolOneDone := now.Add(-900 * time.Millisecond)
	toolTwoStarted := now.Add(-900 * time.Millisecond)
	toolTwoDone := now.Add(-700 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			UpdatedAt:      now,
		},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-parent",
			Status:         "running",
			Active:         true,
			UpdatedAt:      now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "completed", StartedAt: &agentStarted, CompletedAt: &agentDone, UpdatedAt: agentDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:fetch", ParentActivityID: "research-agent", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", Status: "completed", StartedAt: &toolOneStarted, CompletedAt: &toolOneDone, UpdatedAt: toolOneDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:score", ParentActivityID: "research-agent", ActivityKind: "tool_call", ActivityType: "mock_score_risk", ActivityName: "mock_score_risk", Status: "completed", StartedAt: &toolTwoStarted, CompletedAt: &toolTwoDone, UpdatedAt: toolTwoDone},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root"})

	var agentLine, fetchLine, scoreLine string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		switch {
		case strings.Contains(line, "research-agent"):
			agentLine = line
		case strings.Contains(line, "mock_fetch_record"):
			fetchLine = line
		case strings.Contains(line, "mock_score_risk"):
			scoreLine = line
		}
	}
	if agentLine == "" || fetchLine == "" || scoreLine == "" {
		t.Fatalf("expected agent and nested tool lines\n---\n%s", out)
	}
	agentIndent := len(agentLine) - len(strings.TrimLeft(agentLine, " "))
	fetchIndent := len(fetchLine) - len(strings.TrimLeft(fetchLine, " "))
	scoreIndent := len(scoreLine) - len(strings.TrimLeft(scoreLine, " "))
	if fetchIndent <= agentIndent || scoreIndent <= agentIndent {
		t.Fatalf("expected tool calls to indent under agent step\nagent=%q\nfetch=%q\nscore=%q", agentLine, fetchLine, scoreLine)
	}
	if strings.Count(out, "⚙ mock_fetch_record") != 1 || strings.Count(out, "⚙ mock_score_risk") != 1 {
		t.Fatalf("expected tool calls to render once under agent, got:\n%s", out)
	}
}

func TestRenderSnapshotLeavesUnlinkedToolsUnnested(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	agentStarted := now.Add(-2 * time.Second)
	agentDone := now.Add(-1200 * time.Millisecond)
	toolOneStarted := now.Add(-1100 * time.Millisecond)
	toolOneDone := now.Add(-900 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			UpdatedAt:      now,
		},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-parent",
			Status:         "completed",
			Active:         false,
			UpdatedAt:      now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "completed", StartedAt: &agentStarted, CompletedAt: &agentDone, UpdatedAt: agentDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:fetch", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", Status: "completed", StartedAt: &toolOneStarted, CompletedAt: &toolOneDone, UpdatedAt: toolOneDone},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root"})

	var agentLine, fetchLine string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		switch {
		case strings.Contains(line, "research-agent"):
			agentLine = line
		case strings.Contains(line, "mock_fetch_record"):
			fetchLine = line
		}
	}
	if agentLine == "" || fetchLine == "" {
		t.Fatalf("expected agent and tool lines\n---\n%s", out)
	}
	agentIndent := len(agentLine) - len(strings.TrimLeft(agentLine, " "))
	fetchIndent := len(fetchLine) - len(strings.TrimLeft(fetchLine, " "))
	if fetchIndent > agentIndent {
		t.Fatalf("expected unlinked tool not to nest under agent\nagent=%q\nfetch=%q\n%s", agentLine, fetchLine, out)
	}
}

func TestToolParentRefUsesExplicitParentMetadata(t *testing.T) {
	tool := Activity{
		WorkflowID:       "wf-root",
		ActivityKind:     "tool_call",
		ParentActivityID: "research-agent",
	}
	if got := toolParentRef(tool); got != "research-agent" {
		t.Fatalf("expected parent_node_id, got %q", got)
	}
}

func TestToolParentRefUsesParentStepID(t *testing.T) {
	tool := Activity{
		WorkflowID:   "wf-root",
		ActivityKind: "tool_call",
		ParentStepID: "research-agent",
	}
	if got := toolParentRef(tool); got != "research-agent" {
		t.Fatalf("expected parent_step_id parent, got %q", got)
	}
}

func TestToolParentRefWithoutMetadataReturnsEmpty(t *testing.T) {
	if got := toolParentRef(Activity{WorkflowID: "wf-root", ActivityKind: "tool_call"}); got != "" {
		t.Fatalf("expected empty parent without metadata, got %q", got)
	}
}

func TestRenderSnapshotSequentialAgentToolsStayUnderMatchingStep(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	researchDone := now.Add(-1200 * time.Millisecond)
	auditStarted := now.Add(-1100 * time.Millisecond)
	auditDone := now.Add(-700 * time.Millisecond)
	researchToolStarted := now.Add(-1150 * time.Millisecond)
	researchToolDone := now.Add(-1000 * time.Millisecond)
	auditToolStarted := now.Add(-900 * time.Millisecond)
	auditToolDone := now.Add(-800 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			UpdatedAt:      now,
		},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-parent",
			Status:         "running",
			Active:         true,
			UpdatedAt:      now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "completed", CompletedAt: &researchDone, UpdatedAt: researchDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "audit-agent", ActivityKind: "step", ActivityType: "mock/auditor", ActivityName: "audit-agent", StepID: "audit-agent", Status: "completed", StartedAt: &auditStarted, CompletedAt: &auditDone, UpdatedAt: auditDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:research-fetch", ParentActivityID: "research-agent", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", Status: "completed", StartedAt: &researchToolStarted, CompletedAt: &researchToolDone, UpdatedAt: researchToolDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:audit-fetch", ParentActivityID: "audit-agent", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", Status: "completed", StartedAt: &auditToolStarted, CompletedAt: &auditToolDone, UpdatedAt: auditToolDone},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root"})

	var researchLine, researchToolLine, auditLine, auditToolLine string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		switch {
		case strings.Contains(line, "research-agent"):
			researchLine = line
		case strings.Contains(line, "audit-agent"):
			auditLine = line
		case strings.Contains(line, "tool:research-fetch"), strings.Contains(line, "mock_fetch_record") && researchToolLine == "":
			researchToolLine = line
		case strings.Contains(line, "tool:audit-fetch"), strings.Contains(line, "mock_fetch_record") && auditToolLine == "":
			if researchToolLine != "" && line != researchToolLine {
				auditToolLine = line
			}
		}
	}
	if researchLine == "" || auditLine == "" || researchToolLine == "" || auditToolLine == "" {
		t.Fatalf("expected both agent steps and nested tools\n---\n%s", out)
	}
	researchIndent := len(researchLine) - len(strings.TrimLeft(researchLine, " "))
	researchToolIndent := len(researchToolLine) - len(strings.TrimLeft(researchToolLine, " "))
	auditIndent := len(auditLine) - len(strings.TrimLeft(auditLine, " "))
	auditToolIndent := len(auditToolLine) - len(strings.TrimLeft(auditToolLine, " "))
	if researchToolIndent <= researchIndent {
		t.Fatalf("expected research tool nested under research-agent\nresearch=%q\ntool=%q\n%s", researchLine, researchToolLine, out)
	}
	if auditToolIndent <= auditIndent {
		t.Fatalf("expected audit tool nested under audit-agent\naudit=%q\ntool=%q\n%s", auditLine, auditToolLine, out)
	}
}

func TestRenderSnapshotLateFinishingAgentToolsStayUnderStartingStep(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	researchStarted := now.Add(-900 * time.Millisecond)
	researchDone := now.Add(-570 * time.Millisecond)
	auditStarted := now.Add(-500 * time.Millisecond)
	auditDone := now.Add(-170 * time.Millisecond)
	researchToolOneStarted := now.Add(-860 * time.Millisecond)
	researchToolOneDone := now.Add(-262 * time.Millisecond)
	researchToolTwoStarted := now.Add(-850 * time.Millisecond)
	researchToolTwoDone := now.Add(-252 * time.Millisecond)
	auditToolOneStarted := now.Add(-450 * time.Millisecond)
	auditToolOneDone := now.Add(-100 * time.Millisecond)
	auditToolTwoStarted := now.Add(-440 * time.Millisecond)
	auditToolTwoDone := now.Add(-90 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent",
			Status: "completed", Active: false, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "completed", StartedAt: &researchStarted, CompletedAt: &researchDone, UpdatedAt: researchDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: "audit-agent", ActivityKind: "step", ActivityType: "mock/auditor", ActivityName: "audit-agent", StepID: "audit-agent", Status: "completed", StartedAt: &auditStarted, CompletedAt: &auditDone, UpdatedAt: auditDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: "tool:r-fetch", ParentActivityID: "research-agent", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", ToolCallID: "tc-r-fetch", Status: "completed", StartedAt: &researchToolOneStarted, CompletedAt: &researchToolOneDone, UpdatedAt: researchToolOneDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: "tool:r-score", ParentActivityID: "research-agent", ActivityKind: "tool_call", ActivityType: "mock_score_risk", ActivityName: "mock_score_risk", ToolCallID: "tc-r-score", Status: "completed", StartedAt: &researchToolTwoStarted, CompletedAt: &researchToolTwoDone, UpdatedAt: researchToolTwoDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: "tool:a-fetch", ParentActivityID: "audit-agent", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", ToolCallID: "tc-a-fetch", Status: "completed", StartedAt: &auditToolOneStarted, CompletedAt: &auditToolOneDone, UpdatedAt: auditToolOneDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: "tool:a-score", ParentActivityID: "audit-agent", ActivityKind: "tool_call", ActivityType: "mock_score_risk", ActivityName: "mock_score_risk", ToolCallID: "tc-a-score", Status: "completed", StartedAt: &auditToolTwoStarted, CompletedAt: &auditToolTwoDone, UpdatedAt: auditToolTwoDone},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root"})

	var researchLine, researchFetchLine, auditLine, auditFetchLine string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		switch {
		case strings.Contains(line, "research-agent"):
			researchLine = line
		case strings.Contains(line, "audit-agent"):
			auditLine = line
		case strings.Contains(line, "tool:r-fetch"), strings.Contains(line, "mock_fetch_record") && researchFetchLine == "":
			researchFetchLine = line
		case strings.Contains(line, "tool:a-fetch"), strings.Contains(line, "mock_fetch_record") && researchFetchLine != "" && auditFetchLine == "":
			auditFetchLine = line
		}
	}
	if researchLine == "" || auditLine == "" || researchFetchLine == "" || auditFetchLine == "" {
		t.Fatalf("expected both agent steps and nested tools\n---\n%s", out)
	}
	researchIndent := len(researchLine) - len(strings.TrimLeft(researchLine, " "))
	researchToolIndent := len(researchFetchLine) - len(strings.TrimLeft(researchFetchLine, " "))
	auditIndent := len(auditLine) - len(strings.TrimLeft(auditLine, " "))
	auditToolIndent := len(auditFetchLine) - len(strings.TrimLeft(auditFetchLine, " "))
	if researchToolIndent <= researchIndent {
		t.Fatalf("expected research tools nested under research-agent\nresearch=%q\ntool=%q\n%s", researchLine, researchFetchLine, out)
	}
	if auditToolIndent <= auditIndent {
		t.Fatalf("expected audit tools nested under audit-agent\naudit=%q\ntool=%q\n%s", auditLine, auditFetchLine, out)
	}
}

func TestRenderSnapshotKeepsBothAgentToolSetsWithSharedToolNames(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	researchDone := now.Add(-900 * time.Millisecond)
	auditDone := now.Add(-400 * time.Millisecond)
	researchFetchDone := now.Add(-850 * time.Millisecond)
	researchScoreDone := now.Add(-840 * time.Millisecond)
	auditFetchDone := now.Add(-350 * time.Millisecond)
	auditScoreDone := now.Add(-340 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root",
			FlowSlug: "live-render-parent", Status: "completed", UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkflowID: "wf-root", ActivityID: "research-agent", StepID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", Status: "completed", CompletedAt: &researchDone, UpdatedAt: researchDone},
			{WorkflowID: "wf-root", ActivityID: "audit-agent", StepID: "audit-agent", ActivityKind: "step", ActivityType: "mock/auditor", ActivityName: "audit-agent", Status: "completed", CompletedAt: &auditDone, UpdatedAt: auditDone},
			{WorkflowID: "wf-root", ActivityID: "research-agent/call-fetch-record", ParentActivityID: "research-agent", ToolCallID: "call-fetch-record", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", Status: "completed", CompletedAt: &researchFetchDone, UpdatedAt: researchFetchDone},
			{WorkflowID: "wf-root", ActivityID: "research-agent/call-score-risk", ParentActivityID: "research-agent", ToolCallID: "call-score-risk", ActivityKind: "tool_call", ActivityType: "mock_score_risk", ActivityName: "mock_score_risk", Status: "completed", CompletedAt: &researchScoreDone, UpdatedAt: researchScoreDone},
			{WorkflowID: "wf-root", ActivityID: "audit-agent/call-fetch-record", ParentActivityID: "audit-agent", ToolCallID: "call-fetch-record", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", Status: "completed", CompletedAt: &auditFetchDone, UpdatedAt: auditFetchDone},
			{WorkflowID: "wf-root", ActivityID: "audit-agent/call-score-risk", ParentActivityID: "audit-agent", ToolCallID: "call-score-risk", ActivityKind: "tool_call", ActivityType: "mock_score_risk", ActivityName: "mock_score_risk", Status: "completed", CompletedAt: &auditScoreDone, UpdatedAt: auditScoreDone},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	lineIndex := func(substr string) int {
		for i, line := range lines {
			if strings.Contains(line, substr) {
				return i
			}
		}
		return -1
	}
	researchIdx := lineIndex("research-agent")
	auditIdx := lineIndex("audit-agent")
	if researchIdx < 0 || auditIdx < 0 || researchIdx >= auditIdx {
		t.Fatalf("expected research-agent before audit-agent\n%s", out)
	}
	researchTools := 0
	auditTools := 0
	for i, line := range lines {
		if !strings.Contains(line, "mock_fetch_record") && !strings.Contains(line, "mock_score_risk") {
			continue
		}
		switch {
		case i > researchIdx && i < auditIdx:
			researchTools++
		case i > auditIdx:
			auditTools++
		}
	}
	if researchTools != 2 {
		t.Fatalf("expected two research tools between research and audit agents, got %d\n%s", researchTools, out)
	}
	if auditTools != 2 {
		t.Fatalf("expected two audit tools after audit agent, got %d\n%s", auditTools, out)
	}
}

func TestRenderSnapshotNestsChildRunsUnderFanoutStep(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 40, 0, time.UTC)
	prepareStarted := now.Add(-20 * time.Second)
	prepareDone := now.Add(-17 * time.Second)
	researchStarted := now.Add(-16 * time.Second)
	researchDone := now.Add(-15 * time.Second)
	auditStarted := now.Add(-14 * time.Second)
	auditDone := now.Add(-13 * time.Second)
	fanoutStarted := now.Add(-12 * time.Second)
	fanoutDone := now.Add(-11 * time.Second)
	childStepStarted := now.Add(-10 * time.Second)
	branch := 0

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:         "ws-acme",
			ActiveRunCount:      2,
			ActiveChildRunCount: 1,
			StepsRunning:        1,
			UpdatedAt:           now,
		},
		Runs: []RunState{
			{
				WorkspaceID:       "ws-acme",
				WorkflowID:        "wf-root",
				RootWorkflowID:    "wf-root",
				FlowSlug:          "live-render-parent",
				Status:            "running",
				Active:            true,
				CurrentStepID:     "spawn-children",
				CurrentStepName:   "spawn-children",
				CurrentStepType:   "fanout",
				CurrentStepStatus: "completed",
				StepsRunning:      1,
				UpdatedAt:         now,
			},
			{
				WorkspaceID:       "ws-acme",
				WorkflowID:        "wf-child-b0",
				RootWorkflowID:    "wf-root",
				ParentWorkflowID:  "wf-root",
				ParentStepID:      "spawn-children",
				FlowSlug:          "live-render-parent",
				Status:            "running",
				Active:            true,
				AgentID:           "researcher",
				FanoutBranchIndex: &branch,
				StepsRunning:      1,
				UpdatedAt:         now,
			},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root",
			ChildWorkflowID: "wf-child-b0", ParentStepID: "spawn-children", RelationKind: "agent_fanout",
			FlowSlug: "live-render-parent", AgentID: "researcher", FanoutBranchIndex: &branch,
			Active: true, Status: "running", CreatedAt: fanoutDone, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "prepare-run", ActivityKind: "step", ActivityType: "sleep", ActivityName: "prepare-run", StepID: "prepare-run", Status: "completed", StartedAt: &prepareStarted, CompletedAt: &prepareDone, UpdatedAt: prepareDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "completed", StartedAt: &researchStarted, CompletedAt: &researchDone, UpdatedAt: researchDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "audit-agent", ActivityKind: "step", ActivityType: "mock/auditor", ActivityName: "audit-agent", StepID: "audit-agent", Status: "completed", StartedAt: &auditStarted, CompletedAt: &auditDone, UpdatedAt: auditDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "spawn-children", ActivityKind: "step", ActivityType: "fanout", ActivityName: "spawn-children", StepID: "spawn-children", Status: "completed", StartedAt: &fanoutStarted, CompletedAt: &fanoutDone, UpdatedAt: fanoutDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-b0", RootWorkflowID: "wf-root", ActivityID: "spawn-subagents-agent-0", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "spawn-subagents-agent-0", StepID: "spawn-subagents-agent-0", Status: "running", Active: true, AgentID: "researcher", StartedAt: &childStepStarted, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root"})

	var fanoutLine, childRunLine, childStepLine string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		switch {
		case strings.Contains(line, "spawn-children"):
			fanoutLine = line
		case strings.Contains(line, "◉ researcher") && strings.Contains(line, "[b0]"):
			childRunLine = line
		case strings.Contains(line, "spawn-subagents-agent-0"):
			childStepLine = line
		}
	}
	if fanoutLine == "" || childRunLine == "" || childStepLine == "" {
		t.Fatalf("expected fanout, child run, and child step lines\n---\n%s", out)
	}
	fanoutIndent := len(fanoutLine) - len(strings.TrimLeft(fanoutLine, " "))
	childRunIndent := len(childRunLine) - len(strings.TrimLeft(childRunLine, " "))
	childStepIndent := len(childStepLine) - len(strings.TrimLeft(childStepLine, " "))
	if childRunIndent <= fanoutIndent {
		t.Fatalf("expected child run under fanout step\nfanout=%q\nchild=%q\n%s", fanoutLine, childRunLine, out)
	}
	if strings.Contains(childRunLine, "live-render-parent") {
		t.Fatalf("expected child run label to omit redundant flow slug, got %q\n%s", childRunLine, out)
	}
	if childStepIndent <= childRunIndent {
		t.Fatalf("expected child step under child run\nchild=%q\nstep=%q\n%s", childRunLine, childStepLine, out)
	}
	fanoutIdx := strings.Index(out, fanoutLine)
	childIdx := strings.Index(out, childRunLine)
	if childIdx < fanoutIdx {
		t.Fatalf("expected child run after fanout step\n%s", out)
	}
}

func TestCollectDisplayFrameNestsAgentFanoutUnderAgentStep(t *testing.T) {
	now := time.Date(2026, 5, 31, 7, 30, 0, 0, time.UTC)
	researchStarted := now.Add(-4 * time.Second)
	fanoutStarted := now.Add(-3 * time.Second)
	branchZero := 0
	branchOne := 1
	snapshot := Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:         "ws-acme",
			ActiveRunCount:      3,
			ActiveChildRunCount: 2,
			StepsRunning:        2,
			UpdatedAt:           now,
		},
		Runs: []RunState{
			{
				WorkspaceID:       "ws-acme",
				WorkflowID:        "wf-root",
				RootWorkflowID:    "wf-root",
				FlowSlug:          "live-render-parent",
				Status:            "running",
				Active:            true,
				CurrentStepID:     "research-agent",
				CurrentStepName:   "research-agent",
				CurrentStepType:   "mock/researcher",
				CurrentStepStatus: "running",
				StepsRunning:      1,
				UpdatedAt:         now,
			},
			{
				WorkspaceID:       "ws-acme",
				WorkflowID:        "wf-agent-b0",
				RootWorkflowID:    "wf-root",
				ParentWorkflowID:  "wf-root",
				ParentStepID:      "research-agent",
				FlowSlug:          "live-render-parent",
				Status:            "running",
				Active:            true,
				AgentID:           "researcher",
				FanoutBranchIndex: &branchZero,
				StepsRunning:      1,
				UpdatedAt:         now,
			},
			{
				WorkspaceID:       "ws-acme",
				WorkflowID:        "wf-agent-b1",
				RootWorkflowID:    "wf-root",
				ParentWorkflowID:  "wf-root",
				ParentStepID:      "research-agent",
				FlowSlug:          "live-render-parent",
				Status:            "running",
				Active:            true,
				AgentID:           "auditor",
				FanoutBranchIndex: &branchOne,
				StepsRunning:      1,
				UpdatedAt:         now,
			},
		},
		Relations: []RunRelation{
			{
				WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root",
				ChildWorkflowID: "wf-agent-b0", ParentStepID: "research-agent", RelationKind: "agent_fanout",
				FlowSlug: "live-render-parent", AgentID: "researcher", FanoutBranchIndex: &branchZero,
				Active: true, Status: "running", CreatedAt: fanoutStarted, UpdatedAt: now,
			},
			{
				WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root",
				ChildWorkflowID: "wf-agent-b1", ParentStepID: "research-agent", RelationKind: "agent_fanout",
				FlowSlug: "live-render-parent", AgentID: "auditor", FanoutBranchIndex: &branchOne,
				Active: true, Status: "running", CreatedAt: fanoutStarted, UpdatedAt: now,
			},
		},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "running", Active: true, StartedAt: &researchStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ActivityID: "agent-branch-0", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "agent-branch-0", StepID: "agent-branch-0", Status: "running", Active: true, AgentID: "researcher", StartedAt: &fanoutStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ActivityID: "tool-fetch-b0", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", ParentActivityID: "agent-branch-0", Status: "completed", AgentID: "researcher", StartedAt: &fanoutStarted, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b1", RootWorkflowID: "wf-root", ActivityID: "agent-branch-1", ActivityKind: "step", ActivityType: "mock/auditor", ActivityName: "agent-branch-1", StepID: "agent-branch-1", Status: "running", Active: true, AgentID: "auditor", StartedAt: &fanoutStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b1", RootWorkflowID: "wf-root", ActivityID: "tool-score-b1", ActivityKind: "tool_call", ActivityType: "mock_score_risk", ActivityName: "mock_score_risk", ParentActivityID: "agent-branch-1", Status: "completed", AgentID: "auditor", StartedAt: &fanoutStarted, CompletedAt: &now, UpdatedAt: now},
		},
	}
	frame := CollectDisplayFrame(snapshot, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})
	out := RenderDisplayFrame(frame)

	researchLine := displayLineContaining(t, frame, "◉ research-agent")
	branchZeroLine := displayLineContaining(t, frame, "◉ researcher [b0]")
	branchOneLine := displayLineContaining(t, frame, "◉ auditor [b1]")
	toolLine := displayLineContaining(t, frame, "⚙ mock_fetch_record")

	researchIndent := displayIndent(researchLine.Text)
	branchIndent := displayIndent(branchZeroLine.Text)
	branchOneIndent := displayIndent(branchOneLine.Text)
	toolIndent := displayIndent(toolLine.Text)
	if branchIndent <= researchIndent || branchOneIndent != branchIndent {
		t.Fatalf("expected agent fanout branches nested evenly under agent step\n---\n%s", out)
	}
	if strings.Contains(out, "agent-branch-0 mock/researcher") {
		t.Fatalf("expected generated agent fanout entrypoint row to be compacted\n---\n%s", out)
	}
	if toolIndent <= branchIndent {
		t.Fatalf("expected branch tool nested under branch row\ntool=%q\n%s", toolLine.Text, out)
	}
	if strings.Contains(toolLine.Text, "@researcher") {
		t.Fatalf("expected tool row to omit redundant agent label inherited from branch, got %q\n%s", toolLine.Text, out)
	}
	if strings.Index(out, researchLine.Text) > strings.Index(out, branchZeroLine.Text) ||
		strings.Index(out, branchZeroLine.Text) > strings.Index(out, branchOneLine.Text) {
		t.Fatalf("expected agent step then b0 then b1 order\n---\n%s", out)
	}
}

func TestCollectDisplayFrameNestsPackagedFanoutUnderAgentTool(t *testing.T) {
	now := time.Date(2026, 5, 31, 8, 5, 0, 0, time.UTC)
	agentStarted := now.Add(-2 * time.Second)
	toolStarted := now.Add(-1500 * time.Millisecond)
	fanoutStarted := now.Add(-1200 * time.Millisecond)
	branchZero := 0

	snapshot := Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 2, ActiveChildRunCount: 1, StepsRunning: 1, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "running", Active: true, CurrentStepID: "agent-internal-fanout", CurrentStepType: "mock/fanout-manager", StepsRunning: 1, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "fanout", FlowSlug: "live-render-parent", Status: "running", Active: true, AgentID: "researcher", FanoutBranchIndex: &branchZero, StepsRunning: 1, UpdatedAt: now},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root",
			ChildWorkflowID: "wf-agent-b0", ParentStepID: "fanout", RelationKind: "agent_fanout",
			FlowSlug: "live-render-parent", AgentID: "researcher", FanoutBranchIndex: &branchZero,
			Active: true, Status: "running", CreatedAt: fanoutStarted, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "agent-internal-fanout", ActivityKind: "step", ActivityType: "mock/fanout-manager", ActivityName: "agent-internal-fanout", StepID: "agent-internal-fanout", Status: "running", Active: true, StartedAt: &agentStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:call-agent-fanout", ParentActivityID: "agent-internal-fanout", ActivityKind: "tool_call", ActivityType: "mock_spawn_agent_fanout", ActivityName: "mock_spawn_agent_fanout", ToolCallID: "call-agent-fanout", Status: "completed", StartedAt: &toolStarted, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "fanout", ParentActivityID: "call-agent-fanout", ActivityKind: "fanout", ActivityName: "fanout", Status: "completed", StartedAt: &fanoutStarted, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "spawn-subagents", ParentActivityID: "agent-internal-fanout/call-agent-fanout", ActivityKind: "step", ActivityType: "fanout", ActivityName: "spawn-subagents", StepID: "spawn-subagents", Status: "running", Active: true, StartedAt: &fanoutStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ActivityID: "agent-branch-0", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "agent-branch-0", StepID: "agent-branch-0", Status: "running", Active: true, AgentID: "researcher", StartedAt: &fanoutStarted, UpdatedAt: now},
		},
	}
	frame := CollectDisplayFrame(snapshot, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})
	out := RenderDisplayFrame(frame)

	agentLine := displayLineContaining(t, frame, "◉ agent-internal-fanout")
	fanoutLine := displayLineContaining(t, frame, "✣ spawn-subagents")
	branchLine := displayLineContaining(t, frame, "◉ researcher [b0]")
	agentIndent := displayIndent(agentLine.Text)
	fanoutIndent := displayIndent(fanoutLine.Text)
	branchIndent := displayIndent(branchLine.Text)
	if strings.Contains(out, "✣ fanout") {
		t.Fatalf("expected generic packaged fanout wrapper to be hidden\n---\n%s", out)
	}
	if strings.Contains(out, "mock_spawn_agent_fanout") {
		t.Fatalf("expected semantic packaged fanout to replace transport tool row\n---\n%s", out)
	}
	if fanoutIndent <= agentIndent || branchIndent <= fanoutIndent {
		t.Fatalf("expected packaged fanout to nest under agent step\nagent=%q\nfanout=%q\nbranch=%q\n---\n%s", agentLine.Text, fanoutLine.Text, branchLine.Text, out)
	}
	if strings.Index(out, agentLine.Text) > strings.Index(out, fanoutLine.Text) ||
		strings.Index(out, fanoutLine.Text) > strings.Index(out, branchLine.Text) {
		t.Fatalf("expected agent, fanout, branch order\n---\n%s", out)
	}
}

func TestCollectDisplayFrameInfersPackagedFanoutParentFromToolWhenGraphParentIsRoot(t *testing.T) {
	now := time.Date(2026, 5, 31, 14, 5, 0, 0, time.UTC)
	agentStarted := now.Add(-2 * time.Second)
	toolStarted := now.Add(-1600 * time.Millisecond)
	fanoutStarted := now.Add(-1100 * time.Millisecond)
	branchZero := 0
	branchOne := 1

	snapshot := Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 3, ActiveChildRunCount: 2, StepsRunning: 3, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "running", Active: true, CurrentStepID: "spawn-subagents", CurrentStepType: "fanout", StepsRunning: 1, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "fanout", FlowSlug: "live-render-parent", Status: "running", Active: true, AgentID: "researcher", FanoutBranchIndex: &branchZero, StepsRunning: 1, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b1", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "fanout", FlowSlug: "live-render-parent", Status: "running", Active: true, AgentID: "auditor", FanoutBranchIndex: &branchOne, StepsRunning: 1, UpdatedAt: now},
		},
		Relations: []RunRelation{
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-agent-b0", ParentStepID: "fanout", RelationKind: "agent_fanout", FlowSlug: "live-render-parent", AgentID: "researcher", FanoutBranchIndex: &branchZero, Active: true, Status: "running", CreatedAt: fanoutStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-agent-b1", ParentStepID: "fanout", RelationKind: "agent_fanout", FlowSlug: "live-render-parent", AgentID: "auditor", FanoutBranchIndex: &branchOne, Active: true, Status: "running", CreatedAt: fanoutStarted, UpdatedAt: now},
		},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "agent-internal-fanout", ActivityKind: "step", ActivityType: "mock/fanout-manager", ActivityName: "agent-internal-fanout", StepID: "agent-internal-fanout", Status: "running", Active: true, StartedAt: &agentStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "agent-internal-fanout/call-agent-fanout", ParentActivityID: "agent-internal-fanout", ActivityKind: "tool_call", ActivityType: "mock_spawn_agent_fanout", ActivityName: "mock_spawn_agent_fanout", Status: "completed", StartedAt: &toolStarted, CompletedAt: &fanoutStarted, UpdatedAt: fanoutStarted},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "spawn-subagents", ParentActivityID: "flow:live-render-parent", ActivityKind: "step", ActivityType: "fanout", ActivityName: "spawn-subagents", StepID: "spawn-subagents", Status: "running", Active: true, StartedAt: &fanoutStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b0", RootWorkflowID: "wf-root", ActivityID: "agent-branch-0", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "agent-branch-0", StepID: "agent-branch-0", Status: "running", Active: true, AgentID: "researcher", StartedAt: &fanoutStarted, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-agent-b1", RootWorkflowID: "wf-root", ActivityID: "agent-branch-1", ActivityKind: "step", ActivityType: "mock/auditor", ActivityName: "agent-branch-1", StepID: "agent-branch-1", Status: "running", Active: true, AgentID: "auditor", StartedAt: &fanoutStarted, UpdatedAt: now},
		},
		FlowGraphs: []FlowGraphDocument{{
			WorkflowID: "wf-root",
			FlowSlug:   "live-render-parent",
			Version:    1,
			Graph: FlowGraph{SchemaVersion: 1, RootID: "flow:live-render-parent", Nodes: []FlowGraphNode{
				{ID: "flow:live-render-parent", Kind: "flow", Label: "live-render-parent", Order: 1},
				{ID: "step:agent-internal-fanout", Kind: "step", Label: "Agent internal fanout", StepID: "agent-internal-fanout", StepType: "mock/fanout-manager", ParentID: "flow:live-render-parent", Order: 2},
				{ID: "step:spawn-subagents", Kind: "step", Label: "Spawn subagents", StepID: "spawn-subagents", StepType: "fanout", ParentID: "flow:live-render-parent", Order: 3},
				{ID: "step:spawn-subagents:item:0", Kind: "agent", Label: "mock/researcher", ParentID: "step:spawn-subagents", AgentID: "mock/researcher", Order: 4},
				{ID: "step:spawn-subagents:item:1", Kind: "agent", Label: "mock/auditor", ParentID: "step:spawn-subagents", AgentID: "mock/auditor", Order: 5},
				{ID: "step:spawn-children", Kind: "step", Label: "spawn-children", StepID: "spawn-children", StepType: "fanout", ParentID: "flow:live-render-parent", Order: 6},
			}},
		}},
	}

	frame := CollectDisplayFrame(snapshot, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})
	out := RenderDisplayFrame(frame)

	agentLine := displayLineContaining(t, frame, "◉ agent-internal-fanout")
	fanoutLine := displayLineContaining(t, frame, "✣ spawn-subagents")
	branchLine := displayLineContaining(t, frame, "◉ researcher [b0]")
	if strings.Contains(out, "mock_spawn_agent_fanout") {
		t.Fatalf("expected inferred semantic fanout to replace transport tool row\n---\n%s", out)
	}
	if strings.Contains(out, "spawn-subagents:item") || strings.Contains(out, "mock/researcher [") || strings.Contains(out, "mock/auditor [") {
		t.Fatalf("expected planned fanout item skeletons to be suppressed after runtime branches exist\n---\n%s", out)
	}
	if displayIndent(fanoutLine.Text) <= displayIndent(agentLine.Text) || displayIndent(branchLine.Text) <= displayIndent(fanoutLine.Text) {
		t.Fatalf("expected inferred fanout to nest under agent and contain branch runs\nagent=%q\nfanout=%q\nbranch=%q\n---\n%s", agentLine.Text, fanoutLine.Text, branchLine.Text, out)
	}
	if strings.Index(out, agentLine.Text) > strings.Index(out, fanoutLine.Text) ||
		strings.Index(out, fanoutLine.Text) > strings.Index(out, branchLine.Text) {
		t.Fatalf("expected agent, inferred fanout, branch order\n---\n%s", out)
	}
}

func displayLineContaining(t *testing.T, frame DisplayFrame, needle string) DisplayLine {
	t.Helper()
	for _, line := range frame.Lines {
		if strings.Contains(line.Text, needle) {
			return line
		}
	}
	t.Fatalf("expected display frame to contain %q\n---\n%s", needle, RenderDisplayFrame(frame))
	return DisplayLine{}
}

func displayIndent(text string) int {
	return len(text) - len(strings.TrimLeft(text, " "))
}

func TestRenderSnapshotUsesChildActivitySpanForRunDuration(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 20, 0, 0, time.UTC)
	childStarted := now.Add(-12300 * time.Millisecond)
	branchOne := 1

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 0, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "failed", Active: false, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-b1", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "failed", Active: false, FanoutBranchIndex: &branchOne, UpdatedAt: now},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-b1",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &branchOne,
			Active: false, Status: "failed", CreatedAt: childStarted, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "spawn-children", ActivityKind: "step", ActivityType: "fanout", ActivityName: "spawn-children", StepID: "spawn-children", Status: "failed", StartedAt: &childStarted, CompletedAt: &now, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-b1", RootWorkflowID: "wf-root", ActivityID: "loop-page-3", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-3", StepID: "loop-page-3", Status: "failed", StartedAt: &childStarted, CompletedAt: &now, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	childLine := ""
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if strings.Contains(line, "live-render-child") {
			childLine = line
			break
		}
	}
	if childLine == "" {
		t.Fatalf("expected child flow run line\n---\n%s", out)
	}
	for _, want := range []string{"failed", "12.3s"} {
		if !strings.Contains(childLine, want) {
			t.Fatalf("expected child flow line to contain %q, got %q\n---\n%s", want, childLine, out)
		}
	}
}

func TestRenderSnapshotRunningViewShowsCompletedSteps(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	prepareStarted := now.Add(-8 * time.Second)
	prepareDone := now.Add(-5 * time.Second)
	persistStarted := now.Add(-600 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			StepsRunning:   1,
			UpdatedAt:      now,
		},
		Runs: []RunState{
			{
				WorkspaceID:        "ws-acme",
				WorkflowID:         "wf-root",
				RootWorkflowID:     "wf-root",
				FlowSlug:           "live-render-parent",
				Status:             "running",
				Active:             true,
				CurrentStepID:      "persist-run-report",
				CurrentStepName:    "persist-run-report",
				CurrentStepType:    "function",
				CurrentStepStatus:  "running",
				StepsStarted:       6,
				StepsCompleted:     5,
				StepsExecutedTotal: 6,
				StepsRunning:       1,
				LastEventAt:        now,
				UpdatedAt:          now,
			},
		},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "prepare-run", ActivityKind: "step", ActivityType: "sleep", ActivityName: "prepare-run", Status: "completed", StepID: "prepare-run", StartedAt: &prepareStarted, CompletedAt: &prepareDone, UpdatedAt: prepareDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "persist-run-report", ActivityKind: "step", ActivityType: "function", ActivityName: "persist-run-report", Status: "running", Active: true, StepID: "persist-run-report", StartedAt: &persistStarted, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root"})

	for _, want := range []string{
		"wf-root",
		"prepare-run",
		"ƒ persist-run-report",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected running output to contain %q\n---\n%s", want, out)
		}
	}
	for _, notWant := range []string{
		"1 run, 1 step",
		"Summary",
	} {
		if strings.Contains(out, notWant) {
			t.Fatalf("expected running output not to contain %q\n---\n%s", notWant, out)
		}
	}
}

func TestSelectedChildRunsOmitsTerminalBranches(t *testing.T) {
	doneBranch := 0
	selected := selectedChildRuns([]RunNode{
		{
			Run: RunState{WorkflowID: "wf-child-done", Status: "completed", Active: false, FanoutBranchIndex: &doneBranch},
		},
		{
			Run: RunState{WorkflowID: "wf-child-live", Status: "running", Active: true, StepsRunning: 1},
		},
	}, "spawn-children", RenderOptions{})

	if len(selected) != 1 {
		t.Fatalf("expected only active child branch, got %#v", selected)
	}
	if selected[0].Run.WorkflowID != "wf-child-live" {
		t.Fatalf("expected active child branch wf-child-live, got %#v", selected)
	}
}

func TestRenderSnapshotOmitCompletedStepsHidesFinishedLines(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	prepareStarted := now.Add(-8 * time.Second)
	prepareDone := now.Add(-5 * time.Second)
	persistStarted := now.Add(-600 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Runs: []RunState{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent",
			Status: "running", Active: true, CurrentStepID: "persist-run-report", StepsRunning: 1, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "prepare-run", ActivityKind: "step", ActivityType: "sleep", ActivityName: "prepare-run", Status: "completed", StepID: "prepare-run", StartedAt: &prepareStarted, CompletedAt: &prepareDone, UpdatedAt: prepareDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "persist-run-report", ActivityKind: "step", ActivityType: "function", ActivityName: "persist-run-report", Status: "running", Active: true, StepID: "persist-run-report", StartedAt: &persistStarted, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "wf-root", OmitCompletedSteps: true})

	if strings.Contains(out, "prepare-run") {
		t.Fatalf("expected live tail to omit completed step\n---\n%s", out)
	}
	if !strings.Contains(out, "persist-run-report") {
		t.Fatalf("expected active step in live tail\n---\n%s", out)
	}
}

func TestRenderSnapshotPublicModeHidesStepAndResourceCounts(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme"},
		Runs: []RunState{
			{
				WorkspaceID:        "ws-acme",
				WorkflowID:         "wf-installed",
				RootWorkflowID:     "wf-installed",
				FlowSlug:           "installed-black-box",
				Status:             "completed",
				StepsStarted:       12,
				StepsCompleted:     11,
				StepsFailed:        1,
				StepsExecutedTotal: 12,
				LastEventAt:        now,
				UpdatedAt:          now,
			},
		},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-installed", RootWorkflowID: "wf-installed", ActivityID: "res://v1/ws/ws-acme/result/blob/report.md", ActivityKind: "resource", ActivityType: "blob", Status: "completed", ResourceKind: "blob", UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Color: false, DetailMode: DetailModePublic})

	if !strings.Contains(out, "1 completed run") {
		t.Fatalf("expected public summary status\n---\n%s", out)
	}
	if strings.Contains(out, "Summary") {
		t.Fatalf("expected public summary line not to include Summary label\n---\n%s", out)
	}
	for _, notWant := range []string{"steps executed", "failed step", "resources"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("expected public output not to contain %q\n---\n%s", notWant, out)
		}
	}
	for _, notWant := range []string{"11/12", "1f", "blob", "res://"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("expected public output not to expose internal detail %q\n---\n%s", notWant, out)
		}
	}
}

func TestRenderSnapshotUsesANSIColorWhenEnabled(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme"},
		Runs: []RunState{
			{
				WorkspaceID:    "ws-acme",
				WorkflowID:     "wf-root",
				RootWorkflowID: "wf-root",
				FlowSlug:       "root-flow",
				Status:         "completed",
				UpdatedAt:      now,
			},
		},
	}, RenderOptions{Now: now, Color: true, FocusWorkflowID: "wf-root"})

	for _, want := range []string{
		"\x1b[1;36mƒ\x1b[0m wf-root",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected colored output to contain %q\n---\n%q", want, out)
		}
	}
	if strings.Contains(out, "✓") {
		t.Fatalf("expected completed output not to render redundant checkmarks\n---\n%q", out)
	}
}

func TestRenderSnapshotColorsTypeAccentsWithoutPaintingWholeRows(t *testing.T) {
	now := time.Date(2026, 5, 31, 8, 10, 0, 0, time.UTC)
	started := now.Add(-2 * time.Second)
	completed := now.Add(-1 * time.Second)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme"},
		Runs: []RunState{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent",
			Status: "completed", UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "completed", StartedAt: &started, CompletedAt: &completed, UpdatedAt: completed},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool-fetch", ParentActivityID: "research-agent", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", Status: "completed", StartedAt: &started, CompletedAt: &completed, UpdatedAt: completed},
		},
	}, RenderOptions{Now: now, Color: true, FocusWorkflowID: "wf-root", FullTree: true})

	if strings.Contains(out, "✓") {
		t.Fatalf("expected completed rows not to render redundant checkmarks\n---\n%q", out)
	}
	if strings.Contains(out, "\x1b[32mresearch-agent mock/researcher\x1b[0m") {
		t.Fatalf("expected completed row label not to be painted green\n---\n%q", out)
	}
	if !strings.Contains(out, "\x1b[36m◉\x1b[0m research-agent \x1b[2mmock/researcher\x1b[0m") {
		t.Fatalf("expected agent step type accent color\n---\n%q", out)
	}
	if !strings.Contains(out, "\x1b[34m⚙\x1b[0m mock_fetch_record") {
		t.Fatalf("expected tool kind accent color\n---\n%q", out)
	}
}

func TestRenderSnapshotShowsSyncingForPartialRunState(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme"},
		Runs: []RunState{
			{
				WorkspaceID:        "ws-acme",
				WorkflowID:         "flow-live-render-parent-ws-acme-v2-r6",
				RootWorkflowID:     "flow-live-render-parent-ws-acme-v2-r6",
				CurrentStepID:      "spawn-children",
				CurrentStepName:    "spawn-children",
				CurrentStepType:    "fanout",
				CurrentStepStatus:  "completed",
				StepsCompleted:     1,
				StepsExecutedTotal: 1,
				UpdatedAt:          now,
			},
		},
	}, RenderOptions{Now: now, Frame: 2, Color: false, FocusWorkflowID: "flow-live-render-parent-ws-acme-v2-r6"})

	for _, want := range []string{
		"flow-live-render-parent-ws-acme-v2-r6",
		"spawn-children fanout",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected partial output to contain %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "unknown") {
		t.Fatalf("expected partial output not to contain unknown\n---\n%s", out)
	}
}

func TestRenderSnapshotLiveFullNestsToolCallsUnderAgentStep(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	prepareStarted := now.Add(-8 * time.Second)
	prepareDone := now.Add(-5 * time.Second)
	agentStarted := now.Add(-4 * time.Second)
	agentDone := now.Add(-3200 * time.Millisecond)
	toolOneStarted := now.Add(-3100 * time.Millisecond)
	toolOneDone := now.Add(-2900 * time.Millisecond)
	toolTwoStarted := now.Add(-2900 * time.Millisecond)
	toolTwoDone := now.Add(-2700 * time.Millisecond)
	auditStarted := now.Add(-2500 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			StepsRunning:   1,
			UpdatedAt:      now,
		},
		Runs: []RunState{{
			WorkspaceID:       "ws-acme",
			WorkflowID:        "wf-root",
			RootWorkflowID:    "wf-root",
			FlowSlug:          "live-render-parent",
			Status:            "running",
			Active:            true,
			CurrentStepID:     "audit-agent",
			CurrentStepName:   "audit-agent",
			CurrentStepType:   "agent",
			CurrentStepStatus: "running",
			StepsRunning:      1,
			UpdatedAt:         now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "prepare-run", ActivityKind: "step", ActivityType: "sleep", ActivityName: "prepare-run", StepID: "prepare-run", Status: "completed", StartedAt: &prepareStarted, CompletedAt: &prepareDone, UpdatedAt: prepareDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "completed", StartedAt: &agentStarted, CompletedAt: &agentDone, UpdatedAt: agentDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "running", Active: true, StartedAt: &agentStarted, UpdatedAt: agentDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:fetch", ParentActivityID: "research-agent", ActivityKind: "tool_call", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", Status: "completed", StartedAt: &toolOneStarted, CompletedAt: &toolOneDone, UpdatedAt: toolOneDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "tool:score", ParentActivityID: "research-agent", ActivityKind: "tool_call", ActivityType: "mock_score_risk", ActivityName: "mock_score_risk", Status: "completed", StartedAt: &toolTwoStarted, CompletedAt: &toolTwoDone, UpdatedAt: toolTwoDone},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "audit-agent", ActivityKind: "step", ActivityType: "mock/auditor", ActivityName: "audit-agent", StepID: "audit-agent", Status: "running", Active: true, StartedAt: &auditStarted, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	if strings.Count(out, "research-agent") != 1 {
		t.Fatalf("expected deduped research-agent row in live-full, got %d\n---\n%s", strings.Count(out, "research-agent"), out)
	}
	var agentLine, fetchLine, scoreLine string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		switch {
		case strings.Contains(line, "research-agent"):
			agentLine = line
		case strings.Contains(line, "mock_fetch_record"):
			fetchLine = line
		case strings.Contains(line, "mock_score_risk"):
			scoreLine = line
		}
	}
	if agentLine == "" || fetchLine == "" || scoreLine == "" {
		t.Fatalf("expected agent and nested tool lines in live-full\n---\n%s", out)
	}
	agentIndent := len(agentLine) - len(strings.TrimLeft(agentLine, " "))
	fetchIndent := len(fetchLine) - len(strings.TrimLeft(fetchLine, " "))
	scoreIndent := len(scoreLine) - len(strings.TrimLeft(scoreLine, " "))
	if fetchIndent <= agentIndent || scoreIndent <= agentIndent {
		t.Fatalf("expected live-full tool calls to stay nested under agent\nagent=%q\nfetch=%q\nscore=%q\n%s", agentLine, fetchLine, scoreLine, out)
	}
	if strings.Contains(fetchLine, "[tool:fetch]") || strings.Contains(scoreLine, "[tool:score]") {
		t.Fatalf("expected tool rows not to show activity-id detail\nfetch=%q\nscore=%q\n%s", fetchLine, scoreLine, out)
	}
}

func TestRenderSnapshotHidesZeroDurationToolCalls(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 10, 0, time.UTC)
	started := now.Add(-361 * time.Millisecond)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent",
			Status: "running", Active: true, CurrentStepID: "research-agent", StepsRunning: 1, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "running", Active: true, StartedAt: &started, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", ActivityID: "research-agent/call-fetch-record", ParentActivityID: "research-agent", ActivityKind: "tool", ActivityType: "mock_fetch_record", ActivityName: "mock_fetch_record", StepID: "research-agent/call-fetch-record", Status: "completed", StartedAt: &now, CompletedAt: &now, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	toolLine := ""
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if strings.Contains(line, "mock_fetch_record") {
			toolLine = line
			break
		}
	}
	if toolLine == "" {
		t.Fatalf("expected tool line\n---\n%s", out)
	}
	if strings.Contains(toolLine, "0ms") || strings.Contains(toolLine, "[research-agent/call-fetch-record]") {
		t.Fatalf("expected zero-duration tool call without activity-id detail, got %q\n---\n%s", toolLine, out)
	}
}

func TestRenderSnapshotLiveFullShowsFullTree(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 30, 0, time.UTC)
	iter1Start := now.Add(-20 * time.Second)
	iter1Done := now.Add(-17 * time.Second)
	iter2Start := now.Add(-16 * time.Second)
	iter2Done := now.Add(-13 * time.Second)
	iter3Start := now.Add(-12 * time.Second)
	doneBranch := 0

	collapsed := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 2, StepsRunning: 2, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "running", Active: true, CurrentStepID: "spawn-children", StepsRunning: 1, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-done", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "completed", Active: false, FanoutBranchIndex: &doneBranch, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "running", Active: true, CurrentStepID: "loop-page-3", StepsRunning: 1, UpdatedAt: now},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-done",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &doneBranch, Active: false, Status: "completed",
		}, {
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-live",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", Active: true, Status: "running",
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-1", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-1", Status: "completed", StartedAt: &iter1Start, CompletedAt: &iter1Done, UpdatedAt: iter1Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-2", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-2", Status: "completed", StartedAt: &iter2Start, CompletedAt: &iter2Done, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-3", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-3", Status: "running", Active: true, StartedAt: &iter3Start, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root"})

	expanded := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 2, StepsRunning: 2, UpdatedAt: now},
		Runs: []RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "running", Active: true, CurrentStepID: "spawn-children", StepsRunning: 1, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-done", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "completed", Active: false, FanoutBranchIndex: &doneBranch, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "running", Active: true, CurrentStepID: "loop-page-3", StepsRunning: 1, UpdatedAt: now},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-done",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &doneBranch, Active: false, Status: "completed",
		}, {
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-live",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", Active: true, Status: "running",
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-1", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-1", Status: "completed", StartedAt: &iter1Start, CompletedAt: &iter1Done, UpdatedAt: iter1Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-2", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-2", Status: "completed", StartedAt: &iter2Start, CompletedAt: &iter2Done, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-3", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-3", Status: "running", Active: true, StartedAt: &iter3Start, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	if strings.Count(collapsed, "loop-page-1") != 0 {
		t.Fatalf("expected collapsed view to hide earlier loop iterations\n%s", collapsed)
	}
	if strings.Count(collapsed, "[b0]") != 0 {
		t.Fatalf("expected collapsed view to hide terminal child branch\n%s", collapsed)
	}
	if strings.Count(expanded, "loop-page-1") != 0 || strings.Count(expanded, "loop-page-2") != 0 {
		t.Fatalf("expected expanded view to replace prior loop iterations inline\n%s", expanded)
	}
	if strings.Count(expanded, "loop-page-3") != 1 {
		t.Fatalf("expected expanded view to show only latest loop iteration\n%s", expanded)
	}
	if !strings.Contains(expanded, "[b0]") {
		t.Fatalf("expected expanded view to show terminal child branch\n%s", expanded)
	}
	if !strings.Contains(expanded, "wf-root") {
		t.Fatalf("expected expanded view header to identify the workflow id\n%s", expanded)
	}
	if strings.Contains(expanded, "live-render-parent") {
		t.Fatalf("expected expanded view to flatten redundant root run line\n%s", expanded)
	}
}
