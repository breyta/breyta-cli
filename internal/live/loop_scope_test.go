package live

import (
	"strings"
	"testing"
	"time"
)

func TestCollapseLoopIterationScopeKeepsOnlyCurrentIteration(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 30, 0, time.UTC)
	iter1Start := now.Add(-20 * time.Second)
	iter1Done := now.Add(-17 * time.Second)
	iter2Start := now.Add(-16 * time.Second)
	iter2Done := now.Add(-13 * time.Second)
	iter3Start := now.Add(-12 * time.Second)

	activities := []Activity{
		{WorkflowID: "wf-child", StepID: "boot-branch", ActivityKind: "step", ActivityType: "sleep", ActivityName: "boot-branch", Status: "completed", StartedAt: &iter1Start, CompletedAt: &iter1Done, UpdatedAt: iter1Done},
		{WorkflowID: "wf-child", StepID: "loop-page-1", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-1", Status: "completed", StartedAt: &iter1Start, CompletedAt: &iter1Done, UpdatedAt: iter1Done},
		{WorkflowID: "wf-child", StepID: "boot-branch", ActivityKind: "step", ActivityType: "sleep", ActivityName: "boot-branch", Status: "completed", StartedAt: &iter2Start, CompletedAt: &iter2Done, UpdatedAt: iter2Done},
		{WorkflowID: "wf-child", StepID: "loop-page-2", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-2", Status: "completed", StartedAt: &iter2Start, CompletedAt: &iter2Done, UpdatedAt: iter2Done},
		{WorkflowID: "wf-child", StepID: "boot-branch", ActivityKind: "step", ActivityType: "sleep", ActivityName: "boot-branch", Status: "running", Active: true, StartedAt: &iter3Start, UpdatedAt: now},
		{WorkflowID: "wf-child", StepID: "loop-page-3", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-3", Status: "running", Active: true, StartedAt: &iter3Start, UpdatedAt: now},
	}

	collapsed := collapseLoopIterationScope(activities, RunState{
		WorkflowID:    "wf-child",
		CurrentStepID: "loop-page-3",
		Active:        true,
	})

	if len(collapsed) != 2 {
		t.Fatalf("expected current iteration scope only, got %d: %#v", len(collapsed), collapsed)
	}
	for _, activity := range collapsed {
		switch strings.TrimSpace(activity.StepID) {
		case "boot-branch", "loop-page-3":
		default:
			t.Fatalf("unexpected activity in current iteration scope: %q", activity.StepID)
		}
	}
	for _, notWant := range []string{"loop-page-1", "loop-page-2"} {
		for _, activity := range collapsed {
			if activity.StepID == notWant {
				t.Fatalf("expected %q to be replaced by current iteration scope", notWant)
			}
		}
	}
}

func TestCollapseLoopIterationScopeReplacesCompletedIterationWhenNextStarts(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 30, 0, time.UTC)
	iter2Start := now.Add(-8 * time.Second)
	iter2Done := now.Add(-5 * time.Second)
	iter3Start := now.Add(-4 * time.Second)

	activities := []Activity{
		{WorkflowID: "wf-child", StepID: "loop-page-2", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-2", Status: "completed", StartedAt: &iter2Start, CompletedAt: &iter2Done, UpdatedAt: iter2Done},
		{WorkflowID: "wf-child", StepID: "loop-page-3", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-3", Status: "running", Active: true, StartedAt: &iter3Start, UpdatedAt: now},
	}

	collapsed := collapseLoopIterationScope(activities, RunState{WorkflowID: "wf-child", CurrentStepID: "loop-page-3", Active: true})
	if len(collapsed) != 1 {
		t.Fatalf("expected one loop-page line for active iteration, got %d", len(collapsed))
	}
	if collapsed[0].StepID != "loop-page-3" {
		t.Fatalf("expected loop-page-3, got %q", collapsed[0].StepID)
	}
}

func TestRenderSnapshotLoopIterationScopeSingleReplaceableBlock(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 30, 0, time.UTC)
	iter2Start := now.Add(-8 * time.Second)
	iter2Done := now.Add(-5 * time.Second)
	iter3Start := now.Add(-4 * time.Second)
	childStarted := now.Add(-30 * time.Second)

	out := RenderSnapshot(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 2, StepsRunning: 2, UpdatedAt: now},
		Runs: []RunState{
			{
				WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent",
				Status: "running", Active: true, CurrentStepID: "spawn-children", StepsRunning: 1, UpdatedAt: now,
			},
			{
				WorkspaceID: "ws-acme", WorkflowID: "wf-child", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root",
				ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "running", Active: true,
				CurrentStepID: "loop-page-3", StepsRunning: 1, StartedAt: &childStarted, UpdatedAt: now,
			},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", Active: true, Status: "running",
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child", StepID: "loop-page-1", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-1", Status: "completed", StartedAt: ptrTime(iter2Start.Add(-20 * time.Second)), CompletedAt: ptrTime(iter2Start.Add(-17 * time.Second)), UpdatedAt: iter2Start.Add(-17 * time.Second)},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child", StepID: "loop-page-2", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-2", Status: "completed", StartedAt: &iter2Start, CompletedAt: &iter2Done, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child", StepID: "loop-page-3", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-3", Status: "running", Active: true, StartedAt: &iter3Start, UpdatedAt: now},
		},
	}, RenderOptions{Now: now, Frame: 1, Color: false, FocusWorkflowID: "wf-root"})

	if strings.Count(out, "loop-page-1") != 0 {
		t.Fatalf("expected iteration 1 scope to be replaced, got:\n%s", out)
	}
	if strings.Count(out, "loop-page-2") != 0 {
		t.Fatalf("expected iteration 2 scope to be replaced, got:\n%s", out)
	}
	if strings.Count(out, "loop-page-3") != 1 {
		t.Fatalf("expected only current iteration line, got:\n%s", out)
	}
}

func TestSuppressDuplicateStepActivitiesHidesCompletedWhileActive(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	started := now.Add(-2 * time.Second)
	done := now.Add(-1 * time.Second)

	out := suppressDuplicateStepActivities([]Activity{
		{WorkflowID: "wf-child", StepID: "loop-page-2", ActivityKind: "step", ActivityName: "loop-page-2", Status: "completed", StartedAt: &started, CompletedAt: &done, UpdatedAt: done},
		{WorkflowID: "wf-child", StepID: "loop-page-2", ActivityKind: "step", ActivityName: "loop-page-2", Status: "running", Active: true, StartedAt: ptrTime(now.Add(-500 * time.Millisecond)), UpdatedAt: now},
	})

	if len(out) != 1 {
		t.Fatalf("expected one visible step during transition, got %d", len(out))
	}
	if !out[0].Active {
		t.Fatalf("expected active step to win during transition")
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
