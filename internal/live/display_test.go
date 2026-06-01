package live

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCollectDisplayFrameKeepsCompletedStepsInTreeOrder(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	started := now.Add(-5 * time.Second)
	done := now.Add(-2 * time.Second)

	frame := CollectDisplayFrame(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			StepsRunning:   1,
			UpdatedAt:      now,
		},
		Runs: []RunState{
			{
				WorkspaceID:       "wf-root",
				WorkflowID:        "wf-root",
				RootWorkflowID:    "wf-root",
				FlowSlug:          "live-render-parent",
				Status:            "running",
				Active:            true,
				CurrentStepID:     "spawn-children",
				CurrentStepName:   "spawn-children",
				CurrentStepType:   "fanout",
				CurrentStepStatus: "running",
				StepsRunning:      1,
				StartedAt:         &started,
				UpdatedAt:         now,
			},
		},
		Nodes: []Activity{
			{
				WorkspaceID:  "ws-acme",
				WorkflowID:   "wf-root",
				ActivityKind: "step",
				ActivityType: "sleep",
				ActivityName: "prepare-run",
				StepID:       "prepare-run",
				Status:       "completed",
				Active:       false,
				StartedAt:    &started,
				CompletedAt:  &done,
				UpdatedAt:    done,
			},
			{
				WorkspaceID:  "ws-acme",
				WorkflowID:   "wf-root",
				ActivityKind: "step",
				ActivityType: "fanout",
				ActivityName: "spawn-children",
				StepID:       "spawn-children",
				Status:       "running",
				Active:       true,
				StartedAt:    &done,
				UpdatedAt:    now,
			},
		},
	}, RenderOptions{Now: now, Frame: 1, Color: false, FocusWorkflowID: "wf-root"})

	text := RenderDisplayFrame(frame)
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected header and steps; got %d lines:\n%s", len(lines), text)
	}
	if !strings.Contains(lines[0], "wf-root") {
		t.Fatalf("expected workflow id in header, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "prepare-run") {
		t.Fatalf("expected completed step directly under header, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "spawn-children") {
		t.Fatalf("expected active step after completed step, got %q", lines[2])
	}

	var completed DisplayLine
	for _, line := range frame.Lines {
		if strings.Contains(line.Text, "prepare-run") {
			completed = line
			break
		}
	}
	if completed.Live {
		t.Fatalf("expected completed step to be non-live")
	}
}

func TestCollectDisplayFrameCarriesResourceRunContext(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	done := now.Add(-2 * time.Second)
	resourceURI := "res://v1/ws/ws-acme/result/blob/report.md"

	frame := CollectDisplayFrame(Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID: "ws-acme",
			UpdatedAt:   now,
		},
		Runs: []RunState{
			{
				WorkspaceID:    "ws-acme",
				WorkflowID:     "wf-root",
				RootWorkflowID: "wf-root",
				FlowSlug:       "live-render-parent",
				Status:         "completed",
				UpdatedAt:      now,
				CompletedAt:    &now,
			},
		},
		Nodes: []Activity{
			{
				WorkspaceID:  "ws-acme",
				WorkflowID:   "wf-root",
				ActivityKind: "step",
				ActivityType: "function",
				ActivityName: "persist-report",
				StepID:       "persist-report",
				Status:       "completed",
				CompletedAt:  &done,
				UpdatedAt:    done,
			},
			{
				WorkspaceID:      "ws-acme",
				WorkflowID:       "wf-root",
				ActivityID:       "resource:persist-report",
				ParentActivityID: "persist-report",
				ActivityKind:     "resource",
				ActivityType:     "blob",
				ActivityName:     "report.md",
				Status:           "completed",
				ResourceURI:      resourceURI,
				ResourceKind:     "blob",
				ResourceLabel:    "report.md",
				ContentType:      "text/markdown",
				CompletedAt:      &done,
				UpdatedAt:        done,
			},
		},
	}, RenderOptions{Now: now, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	var resource DisplayLine
	for _, line := range frame.Lines {
		if line.ResourceURI == resourceURI {
			resource = line
			break
		}
	}
	if resource.ResourceURI != resourceURI {
		t.Fatalf("expected resource URI metadata, got %q", resource.ResourceURI)
	}
	if resource.WorkspaceID != "ws-acme" || resource.WorkflowID != "wf-root" || resource.FlowSlug != "live-render-parent" || resource.ResourceKind != "blob" {
		t.Fatalf("unexpected resource context: %#v", resource)
	}
	if resource.WebURL != "" {
		t.Fatalf("expected live package not to derive CLI web URL, got %q", resource.WebURL)
	}
}

func TestCollectDisplayFrameSuppressesAutomaticStepCaptureResources(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	done := now.Add(-2 * time.Second)
	stepCaptureURI := "res://v1/ws/ws-acme/result/run/wf-root/step/collect-fanout/output"
	artifactURI := "res://v1/ws/ws-acme/result/blob/report.md"
	resultURI := "res://v1/ws/ws-acme/result/run/wf-root/flow-output"

	frame := CollectDisplayFrame(Snapshot{
		Workspace: WorkspaceSummary{WorkspaceID: "ws-acme", UpdatedAt: now},
		Runs: []RunState{{
			WorkspaceID:    "ws-acme",
			WorkflowID:     "wf-root",
			RootWorkflowID: "wf-root",
			FlowSlug:       "live-render-parent",
			Status:         "completed",
			UpdatedAt:      now,
			CompletedAt:    &now,
		}},
		Nodes: []Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityKind: "step", ActivityType: "function", ActivityName: "collect-fanout", StepID: "collect-fanout", Status: "completed", CompletedAt: &done, UpdatedAt: done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: stepCaptureURI, ParentActivityID: "collect-fanout", ActivityKind: "resource", ActivityType: "run-result", ActivityName: "output", Status: "completed", ResourceURI: stepCaptureURI, ResourceKind: "run-result", ResourceLabel: "output", CompletedAt: &done, UpdatedAt: done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: artifactURI, ParentActivityID: "collect-fanout", ActivityKind: "resource", ActivityType: "blob", ActivityName: "report.md", Status: "completed", ResourceURI: artifactURI, ResourceKind: "blob", ResourceLabel: "report.md", CompletedAt: &done, UpdatedAt: done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", ActivityID: resultURI, ParentActivityID: "run:wf-root", ActivityKind: "resource", ActivityType: "run-result", ActivityName: "flow result", Status: "completed", ResourceURI: resultURI, ResourceKind: "run-result", ResourceLabel: "flow result", CompletedAt: &done, UpdatedAt: done},
		},
	}, RenderOptions{Now: now, Color: false, FocusWorkflowID: "wf-root", FullTree: true})

	for _, line := range frame.Lines {
		if line.ResourceURI == stepCaptureURI || strings.Contains(line.Text, stepCaptureURI) {
			t.Fatalf("expected synthetic step capture resource to be suppressed: %#v", line)
		}
	}
	if displayLineContaining(t, frame, "report.md").ResourceURI != artifactURI {
		t.Fatalf("expected explicit artifact resource to remain visible")
	}
	if displayLineContaining(t, frame, "flow result").ResourceURI != resultURI {
		t.Fatalf("expected run result resource to remain visible")
	}
}

func TestFitDisplayFrameForLiveTruncatesToTerminalHeight(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 10, 0, time.UTC)
	started := now.Add(-10 * time.Second)
	nodes := make([]Activity, 0, 12)
	for i := 0; i < 10; i++ {
		done := started.Add(time.Duration(i+1) * time.Second)
		nodes = append(nodes, Activity{
			WorkspaceID:  "ws-acme",
			WorkflowID:   "wf-root",
			ActivityKind: "step",
			ActivityType: "sleep",
			ActivityName: fmt.Sprintf("step-%02d", i),
			StepID:       fmt.Sprintf("step-%02d", i),
			Status:       "completed",
			StartedAt:    &started,
			CompletedAt:  &done,
			UpdatedAt:    done,
		})
	}
	nodes = append(nodes, Activity{
		WorkspaceID:  "ws-acme",
		WorkflowID:   "wf-root",
		ActivityKind: "step",
		ActivityType: "sleep",
		ActivityName: "step-final",
		StepID:       "step-final",
		Status:       "running",
		Active:       true,
		StartedAt:    &started,
		UpdatedAt:    now,
	})
	snapshot := Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			StepsRunning:   1,
			UpdatedAt:      now,
		},
		Runs: []RunState{
			{
				WorkspaceID:    "wf-root",
				WorkflowID:     "wf-root",
				RootWorkflowID: "wf-root",
				FlowSlug:       "live-render-parent",
				Status:         "running",
				Active:         true,
				CurrentStepID:  "step-final",
				StepsRunning:   1,
				StartedAt:      &started,
				UpdatedAt:      now,
			},
		},
		Nodes: nodes,
	}
	opts := RenderOptions{Now: now, Frame: 1, Color: false, FocusWorkflowID: "wf-root"}

	full := CollectDisplayFrame(snapshot, opts)
	if len(full.Lines) <= 8 {
		t.Fatalf("test setup needs >8 lines before fit, got %d", len(full.Lines))
	}
	frame := FitDisplayFrameForLive(snapshot, opts, 8)
	if len(frame.Lines) > 8 {
		t.Fatalf("expected at most 8 visible lines, got %d", len(frame.Lines))
	}
	text := RenderDisplayFrame(frame)
	if !strings.Contains(text, "step-final") {
		t.Fatalf("expected final step in tail, got:\n%s", text)
	}
}

func TestCollectDisplayFrameMatchesRenderSnapshotContent(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 10, 0, time.UTC)
	snapshot := Snapshot{
		Workspace: WorkspaceSummary{
			WorkspaceID:    "ws-acme",
			ActiveRunCount: 1,
			StepsRunning:   1,
			UpdatedAt:      now,
		},
		Runs: []RunState{
			{
				WorkspaceID:    "wf-root",
				WorkflowID:     "wf-root",
				RootWorkflowID: "wf-root",
				FlowSlug:       "root-flow",
				Status:         "running",
				Active:         true,
				UpdatedAt:      now,
			},
		},
	}
	opts := RenderOptions{Now: now, Frame: 0, Color: false, FocusWorkflowID: "wf-root"}
	full := RenderSnapshot(snapshot, opts)
	frameText := RenderDisplayFrame(CollectDisplayFrame(snapshot, opts))
	if frameText != full {
		t.Fatalf("expected display frame to match render snapshot:\n--- want ---\n%s--- got ---\n%s", full, frameText)
	}
}
