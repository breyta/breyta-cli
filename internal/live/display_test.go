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
