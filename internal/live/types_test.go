package live

import (
	"strings"
	"testing"
	"time"
)

func TestBuildRunTreeChildRunIsNotRoot(t *testing.T) {
	nodes := Snapshot{
		Runs: []RunState{
			{
				WorkspaceID:    "ws-acme",
				WorkflowID:     "wf-root",
				RootWorkflowID: "wf-root",
				FlowSlug:       "live-render-parent",
				Status:         "running",
				Active:         true,
			},
			{
				WorkspaceID:      "ws-acme",
				WorkflowID:       "wf-child-b0",
				RootWorkflowID:   "wf-root",
				ParentWorkflowID: "",
				FlowSlug:         "live-render-parent",
				Status:           "running",
				Active:           true,
				AgentID:          "researcher",
			},
		},
		Relations: []RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root",
			ChildWorkflowID: "wf-child-b0", ParentStepID: "spawn-children", RelationKind: "agent_fanout",
			FlowSlug: "live-render-parent", AgentID: "researcher", Active: true, Status: "running",
		}},
	}.Focus("wf-root").BuildRunTree()

	if len(nodes) != 1 {
		t.Fatalf("expected one root node, got %d", len(nodes))
	}
	if got := nodes[0].Run.WorkflowID; got != "wf-root" {
		t.Fatalf("expected root workflow wf-root, got %q", got)
	}
	if len(nodes[0].Children) != 1 {
		t.Fatalf("expected child under root, got %d children", len(nodes[0].Children))
	}
	if got := nodes[0].Children[0].Run.WorkflowID; got != "wf-child-b0" {
		t.Fatalf("expected child wf-child-b0, got %q", got)
	}
}

func TestFlatRootRunNodeUsesFocusedRootWhenFlattening(t *testing.T) {
	root := RunNode{
		Run: RunState{
			WorkflowID:     "flow-live-render-parent-ws-acme-v3-r25",
			RootWorkflowID: "flow-live-render-parent-ws-acme-v3-r25",
			FlowSlug:       "live-render-parent",
			Status:         "running",
			Active:         true,
		},
	}
	child := RunNode{
		Run: RunState{
			WorkflowID:       "wf-child-b0",
			RootWorkflowID:   "flow-live-render-parent-ws-acme-v3-r25",
			ParentWorkflowID: "flow-live-render-parent-ws-acme-v3-r25",
			FlowSlug:         "live-render-parent",
			AgentID:          "researcher",
		},
		Relation: &RunRelation{RelationKind: "agent_fanout"},
	}

	flat := flatRootRunNode([]RunNode{root, child}, RenderOptions{
		FocusWorkflowID: "flow-live-render-parent-ws-acme-v3-r25",
	})
	if flat == nil {
		t.Fatal("expected focused root to flatten")
	}
	if got := flat.Run.FlowSlug; got != "live-render-parent" {
		t.Fatalf("expected flat root flow slug live-render-parent, got %q", got)
	}
}

func TestFlatRootRunNodeFlattensFocusedWorkflowWithoutFlowSlug(t *testing.T) {
	root := RunNode{
		Run: RunState{
			WorkflowID:     "flow-live-render-parent-ws-acme-v3-r26",
			RootWorkflowID: "flow-live-render-parent-ws-acme-v3-r26",
			Status:         "running",
			Active:         true,
		},
	}
	flat := flatRootRunNode([]RunNode{root}, RenderOptions{
		FocusWorkflowID: "flow-live-render-parent-ws-acme-v3-r26",
	})
	if flat == nil {
		t.Fatal("expected focused root to flatten before flow slug arrives")
	}
}

func TestFocusRootRunNodePrefersExactWorkflowMatch(t *testing.T) {
	parent := RunNode{
		Run: RunState{
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-parent",
		},
	}
	childAsRoot := RunNode{
		Run: RunState{
			WorkflowID:     "wf-child-b0",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-parent",
			AgentID:        "researcher",
		},
	}
	focused := focusRootRunNode([]RunNode{childAsRoot, parent}, RenderOptions{FocusWorkflowID: "wf-root"})
	if focused == nil || focused.Run.WorkflowID != "wf-root" {
		got := ""
		if focused != nil {
			got = focused.Run.WorkflowID
		}
		t.Fatalf("expected focused parent wf-root, got %q", got)
	}
}

func TestRenderSnapshotStableFocusedHeaderWithoutFlowSlug(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	prepareDone := now.Add(-5 * time.Second)
	researchDone := now.Add(-1200 * time.Millisecond)
	focus := "flow-live-render-parent-ws-acme-v3-r26"
	snapshot := Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 1, UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID: focus, WorkflowID: focus, RootWorkflowID: focus,
			Status: "running", Active: true, UpdatedAt: now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: focus, RootWorkflowID: focus, ActivityID: "prepare-run", ActivityKind: "step", ActivityType: "sleep", ActivityName: "prepare-run", StepID: "prepare-run", Status: "completed", CompletedAt: &prepareDone, UpdatedAt: prepareDone},
			{WorkspaceID: "ws-acme", WorkflowID: focus, RootWorkflowID: focus, ActivityID: "research-agent", ActivityKind: "step", ActivityType: "mock/researcher", ActivityName: "research-agent", StepID: "research-agent", Status: "completed", CompletedAt: &researchDone, UpdatedAt: researchDone},
		},
	}
	opts := RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: focus}

	outWithoutSlug := RenderSnapshot(snapshot, opts)
	snapshot.Runs[0].FlowSlug = "live-render-parent"
	outWithSlug := RenderSnapshot(snapshot, opts)

	if strings.Count(outWithoutSlug, "\n") != strings.Count(outWithSlug, "\n") {
		t.Fatalf("expected stable line count when flow slug arrives\nwithout=%q\nwith=%q", outWithoutSlug, outWithSlug)
	}
	if strings.Contains(outWithoutSlug, "live-render-parent\n ⠋ live-render-parent") {
		t.Fatalf("expected flattened header without duplicate run line\n---\n%s", outWithoutSlug)
	}
}

func TestRunLabelOmitsFlowSlugWhenSameAsRoot(t *testing.T) {
	relation := &RunRelation{
		FlowSlug: "live-render-parent",
		AgentID:  "researcher",
	}
	got := runLabel(RunState{
		FlowSlug: "live-render-parent",
		AgentID:  "researcher",
	}, relation, "live-render-parent")
	if got != "◉ researcher" {
		t.Fatalf("expected compact agent label, got %q", got)
	}
}

func TestRunLabelKeepsDistinctChildFlowSlug(t *testing.T) {
	got := runLabel(RunState{
		FlowSlug: "customer-agent",
		AgentID:  "researcher",
	}, &RunRelation{
		FlowSlug: "customer-agent",
		AgentID:  "researcher",
	}, "root-flow")
	if got != "◉ researcher customer-agent" {
		t.Fatalf("expected distinct child flow slug in label, got %q", got)
	}
}
