package cli

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/breyta/breyta-cli/internal/live"
)

func sampleLiveDisplayFrame(t *testing.T) live.DisplayFrame {
	t.Helper()
	now := time.Date(2026, 5, 30, 12, 0, 30, 0, time.UTC)
	doneBranch := 0
	doneBranchTwo := 1
	doneBranchThree := 2
	iter1Start := now.Add(-20 * time.Second)
	iter1Done := now.Add(-17 * time.Second)
	iter2Start := now.Add(-16 * time.Second)
	iter2Done := now.Add(-13 * time.Second)
	iter3Start := now.Add(-12 * time.Second)

	snapshot := live.Snapshot{
		Workspace: live.WorkspaceSummary{WorkspaceID: "ws-acme", ActiveRunCount: 2, StepsRunning: 2, UpdatedAt: now},
		Runs: []live.RunState{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", RootWorkflowID: "wf-root", FlowSlug: "live-render-parent", Status: "running", Active: true, CurrentStepID: "spawn-children", StepsRunning: 1, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-done", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "completed", Active: false, FanoutBranchIndex: &doneBranch, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-done-2", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "completed", Active: false, FanoutBranchIndex: &doneBranchTwo, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-done-3", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "completed", Active: false, FanoutBranchIndex: &doneBranchThree, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ParentStepID: "spawn-children", FlowSlug: "live-render-child", Status: "running", Active: true, CurrentStepID: "loop-page-3", StepsRunning: 1, UpdatedAt: now},
		},
		Relations: []live.RunRelation{{
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-done",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &doneBranch, Active: false, Status: "completed",
		}, {
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-done-2",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &doneBranchTwo, Active: false, Status: "completed",
		}, {
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-done-3",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", FanoutBranchIndex: &doneBranchThree, Active: false, Status: "completed",
		}, {
			WorkspaceID: "ws-acme", RootWorkflowID: "wf-root", ParentWorkflowID: "wf-root", ChildWorkflowID: "wf-child-live",
			ParentStepID: "spawn-children", RelationKind: "child_flow", FlowSlug: "live-render-child", Active: true, Status: "running",
		}},
		Nodes: []live.Activity{
			{WorkspaceID: "ws-acme", WorkflowID: "wf-root", StepID: "spawn-children", ActivityKind: "step", ActivityType: "fanout", ActivityName: "spawn-children", Status: "running", Active: true, StartedAt: &iter1Start, UpdatedAt: now},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-1", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-1", Status: "completed", StartedAt: &iter1Start, CompletedAt: &iter1Done, UpdatedAt: iter1Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-2", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-2", Status: "completed", StartedAt: &iter2Start, CompletedAt: &iter2Done, UpdatedAt: iter2Done},
			{WorkspaceID: "ws-acme", WorkflowID: "wf-child-live", StepID: "loop-page-3", ActivityKind: "step", ActivityType: "sleep", ActivityName: "loop-page-3", Status: "running", Active: true, StartedAt: &iter3Start, UpdatedAt: now},
		},
	}
	return live.CollectDisplayFrame(snapshot, live.RenderOptions{
		Now:             now,
		Frame:           1,
		Color:           false,
		FocusWorkflowID: "wf-root",
		FullTree:        true,
	})
}

func TestLiveTUIStartsExpandedAndPreservesCompletedRows(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 120
	model.height = 30
	updated, _ := model.Update(liveTUIFrameMsg{frame: sampleLiveDisplayFrame(t), at: time.Now()})
	model = updated.(liveTUIModel)

	view := model.View()
	for _, want := range []string{"[b0]", "[b1]", "[b2]", "loop-page-3"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected live TUI view to include %q\n%s", want, view)
		}
	}
	for _, notWant := range []string{"loop-page-1", "loop-page-2"} {
		if strings.Contains(view, notWant) {
			t.Fatalf("expected live TUI view to replace prior loop pages inline, found %q\n%s", notWant, view)
		}
	}
}

func TestFlowsRunOnlyExposesLiveFlag(t *testing.T) {
	cmd := newFlowsRunCmd(&App{})
	if cmd.Flags().Lookup("live") == nil {
		t.Fatalf("expected flows run to expose --live")
	}
	if cmd.Flags().Lookup("live-full") != nil {
		t.Fatalf("expected flows run not to expose --live-full")
	}
}

func TestLiveTUIToggleCollapsesSelectedNode(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 120
	model.height = 30
	updated, _ := model.Update(liveTUIFrameMsg{frame: sampleLiveDisplayFrame(t), at: time.Now()})
	model = updated.(liveTUIModel)

	for i, node := range model.visibleNodes() {
		if strings.Contains(node.Text, "spawn-children") {
			model.setCursor(i)
			break
		}
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(liveTUIModel)
	view := model.View()
	if strings.Contains(view, "[b0]") || strings.Contains(view, "loop-page-3") {
		t.Fatalf("expected spawn-children descendants to collapse\n%s", view)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(liveTUIModel)
	view = model.View()
	if !strings.Contains(view, "[b0]") || !strings.Contains(view, "loop-page-3") {
		t.Fatalf("expected spawn-children descendants to expand\n%s", view)
	}
}

func TestLiveTUICursorNavigationAndViewport(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 120
	model.height = 5
	updated, _ := model.Update(liveTUIFrameMsg{frame: sampleLiveDisplayFrame(t), at: time.Now()})
	model = updated.(liveTUIModel)

	for i := 0; i < 10; i++ {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(liveTUIModel)
	}
	if model.cursor == 0 {
		t.Fatalf("expected cursor to move down")
	}
	if model.offset == 0 {
		t.Fatalf("expected viewport offset to follow cursor")
	}
}

func TestLiveTUISticksToBottomAsRowsArrive(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 120
	model.height = 5
	first := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header", Text: "running"},
		{Key: "run:root", Text: " ✓ root"},
		{Key: "step:a", Text: "  ✓ step-a"},
	}}
	second := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header", Text: "running"},
		{Key: "run:root", Text: " ✓ root"},
		{Key: "step:a", Text: "  ✓ step-a"},
		{Key: "step:b", Text: "  ✓ step-b"},
		{Key: "step:c", Text: "  ✓ step-c"},
		{Key: "step:d", Text: "  ⠋ step-d"},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{frame: first, at: time.Now()})
	model = updated.(liveTUIModel)
	if model.cursorKey != "step:a" {
		t.Fatalf("expected initial cursor at bottom, got %q", model.cursorKey)
	}
	updated, _ = model.Update(liveTUIFrameMsg{frame: second, at: time.Now()})
	model = updated.(liveTUIModel)
	if model.cursorKey != "step:d" {
		t.Fatalf("expected cursor to follow new bottom row, got %q", model.cursorKey)
	}
	if model.offset == 0 {
		t.Fatalf("expected viewport offset to follow bottom")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(liveTUIModel)
	updated, _ = model.Update(liveTUIFrameMsg{frame: live.DisplayFrame{Lines: append(second.Lines, live.DisplayLine{Key: "step:e", Text: "  ⠋ step-e"})}, at: time.Now()})
	model = updated.(liveTUIModel)
	if model.cursorKey == "step:e" {
		t.Fatalf("expected manual upward navigation to disable sticky bottom")
	}
}

func TestLiveTUIStickEndIgnoresFutureSkeletonRows(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 120
	model.height = 5
	skeleton := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header", Text: "running"},
		{Key: "step:prepare", Text: "  ○ prepare", Planned: true},
		{Key: "step:collect", Text: "  ○ collect", Planned: true},
		{Key: "step:persist", Text: "  ○ persist", Planned: true},
	}}
	running := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header", Text: "running"},
		{Key: "step:prepare", Text: "  ⠋ prepare"},
		{Key: "step:collect", Text: "  ○ collect", Planned: true},
		{Key: "step:persist", Text: "  ○ persist", Planned: true},
	}}

	updated, _ := model.Update(liveTUIFrameMsg{frame: skeleton, at: time.Now()})
	model = updated.(liveTUIModel)
	if model.cursorKey != "step:prepare" || model.offset != 0 {
		t.Fatalf("expected initial skeleton to focus first planned row without scrolling to the end, cursor=%q offset=%d", model.cursorKey, model.offset)
	}

	updated, _ = model.Update(liveTUIFrameMsg{frame: running, at: time.Now()})
	model = updated.(liveTUIModel)
	if model.cursorKey != "step:prepare" || model.offset != 0 {
		t.Fatalf("expected sticky cursor to follow latest real row above future skeleton, cursor=%q offset=%d", model.cursorKey, model.offset)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(liveTUIModel)
	if model.stickEnd {
		t.Fatalf("expected manual navigation into skeleton rows to disable sticky bottom")
	}
}

func TestLiveTUIStickyBottomKeepsCompletionSummaryVisible(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 120
	model.height = 6
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "f wf-root"},
		{Key: "activity:wf-root:prepare", Text: "  s prepare-run"},
		{Key: "activity:wf-root:fanout", Text: "  o fanout-agents"},
		{Key: "activity:wf-root:report", Text: "  f persist-report"},
		{Key: "resource:wf-root:report:blob", Text: "    b agent-report 226B text/markdown"},
		{Key: "summary", Text: "3 steps executed, 1 resource (b 1)"},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{frame: frame, at: time.Now()})
	model = updated.(liveTUIModel)

	view := model.View()
	if !strings.Contains(view, "3 steps executed, 1 resource (b 1)") {
		t.Fatalf("expected sticky-bottom view to include completion summary\n%s", view)
	}
	if model.cursorKey != "resource:wf-root:report:blob" {
		t.Fatalf("expected cursor to remain on last selectable row, got %q", model.cursorKey)
	}
}

func TestLiveTUIHeaderSticksAndRootFlowLineIsFlattened(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 120
	model.height = 6
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "f wf-root ws-acme"},
		{Key: "run:wf-root", Text: " f root-flow"},
		{Key: "activity:wf-root:prepare", Text: "  s prepare-run"},
		{Key: "activity:wf-root:fanout", Text: "  o fanout-agents"},
		{Key: "activity:wf-root:agent", Text: "    a researcher [b0]"},
		{Key: "activity:wf-root:tool", Text: "      t mock_fetch_record"},
		{Key: "activity:wf-root:report", Text: "  f persist-report"},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{frame: frame, at: time.Now()})
	model = updated.(liveTUIModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(liveTUIModel)

	view := model.View()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "f wf-root ws-acme") {
		t.Fatalf("expected sticky header on first row\n%s", view)
	}
	if len(lines) < 2 || !strings.Contains(lines[1], "────") {
		t.Fatalf("expected scrolled sticky header to be visually separated\n%s", view)
	}
	if strings.Contains(view, "root-flow") {
		t.Fatalf("expected duplicate root flow tree row to be flattened\n%s", view)
	}
	if !strings.Contains(view, "persist-report") {
		t.Fatalf("expected body content to remain visible under sticky header\n%s", view)
	}
}

func TestLiveTUIHeaderSeparatorAppearsOnlyAfterScroll(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 80
	model.height = 8
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "f wf-root ws-acme"},
		{Key: "activity:wf-root:prepare", Text: "  s prepare-run"},
		{Key: "activity:wf-root:report", Text: "  f persist-report"},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{frame: frame, at: time.Now()})
	model = updated.(liveTUIModel)

	view := model.View()
	lines := strings.Split(view, "\n")
	if len(lines) > 1 && strings.Contains(lines[1], "────") {
		t.Fatalf("did not expect header separator before scrolling\n%s", view)
	}

	scrolledFrame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "f wf-root ws-acme"},
		{Key: "activity:wf-root:prepare", Text: "  s prepare-run"},
		{Key: "activity:wf-root:fanout", Text: "  o fanout-agents"},
		{Key: "activity:wf-root:agent", Text: "    a researcher [b0]"},
		{Key: "activity:wf-root:tool", Text: "      t mock_fetch_record"},
		{Key: "activity:wf-root:report", Text: "  f persist-report"},
		{Key: "summary", Text: "5 steps executed"},
	}}
	updated, _ = model.Update(liveTUIFrameMsg{frame: scrolledFrame, at: time.Now()})
	model = updated.(liveTUIModel)

	view = model.View()
	lines = strings.Split(view, "\n")
	if len(lines) < 2 || !strings.Contains(lines[1], "────") {
		t.Fatalf("expected header separator after scrolling\n%s", view)
	}
}

func TestLiveTUISkipsHeaderAndSummaryFocus(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 120
	model.height = 8
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "⠋ f wf-root"},
		{Key: "activity:wf-root:prepare", Text: "  s prepare-run"},
		{Key: "activity:wf-root:report", Text: "  f persist-report"},
		{Key: "summary", Text: "2 steps executed, 1 resource (b 1)"},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{frame: frame, at: time.Now()})
	model = updated.(liveTUIModel)

	if model.cursorKey == "summary" || model.cursorKey == "header:wf-root" {
		t.Fatalf("expected cursor to skip non-item rows, got %q", model.cursorKey)
	}
	if model.cursorKey != "activity:wf-root:report" {
		t.Fatalf("expected sticky bottom to select last item row, got %q", model.cursorKey)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(liveTUIModel)
	if model.cursorKey == "summary" {
		t.Fatalf("expected down navigation not to focus summary")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = updated.(liveTUIModel)
	if model.cursorKey != "activity:wf-root:prepare" {
		t.Fatalf("expected home to select first item row, got %q", model.cursorKey)
	}
}

func TestLiveTUITruncatesANSIWithoutCorruptingEscapeSequences(t *testing.T) {
	line := "✓ tool mock_fetch_record (mock_fetch_record) 0ms \x1b[2m@auditor\x1b[0m"
	got := truncateTUIRunes(line, 52)
	if strings.Contains(got, "\ufffd") {
		t.Fatalf("expected no replacement characters in truncated line: %q", got)
	}
	if strings.Contains(got, "\x1b[2…") {
		t.Fatalf("expected truncation not to split ANSI escape sequence: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("expected line to be truncated: %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("expected truncated colored line to reset ANSI state: %q", got)
	}
}

func TestLiveTUIFooterShowsCommandMenuAndQQuits(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 140
	model.height = 6
	updated, _ := model.Update(liveTUIFrameMsg{frame: sampleLiveDisplayFrame(t), at: time.Now()})
	model = updated.(liveTUIModel)

	view := model.View()
	plain := stripTUIANSI(view)
	if strings.Contains(plain, "Commands:") {
		t.Fatalf("expected compact footer without Commands prefix\n%s", plain)
	}
	for _, want := range []string{"↑↓/jk move", "space toggle", "q/ctrl+c exit"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected footer to contain %q\n%s", want, view)
		}
	}
	if !strings.Contains(plain, "────") {
		t.Fatalf("expected footer to be visually separated\n%s", view)
	}
	if !strings.Contains(view, "\x1b[38;5;81m↑↓/jk\x1b[39m") {
		t.Fatalf("expected footer command keys to be accented\n%s", view)
	}
	if !strings.Contains(view, "\x1b[38;5;54;48;5;220m☷\x1b[0m") {
		t.Fatalf("expected footer to include compact Breyta logo mark\n%s", view)
	}
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("expected q to return quit command")
	}
}

func TestLiveTUIFooterShowsOpenForOpenableResource(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 160
	model.height = 6
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "f wf-root"},
		{Key: "activity:wf-root:persist", Text: "  ƒ Persist run report [persist-run-report]"},
		{
			Key:         "resource:wf-root:report",
			Text:        "    ▣ output resource blob text/markdown",
			ResourceURI: "res://v1/ws/ws-acme/result/run/wf-root/step/persist-run-report/output",
			WebURL:      "http://localhost:30546/ws-acme/runs/live-render-parent/wf-root?artifactUri=demo&output=fullscreen",
		},
		{Key: "summary", Text: "1 step executed, 1 resource"},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{frame: frame, at: time.Now()})
	model = updated.(liveTUIModel)

	plain := stripTUIANSI(model.View())
	if !strings.Contains(plain, "enter open") {
		t.Fatalf("expected open command for selected resource row\n%s", plain)
	}
}

func TestLiveTUIFooterShowsActiveWaitActions(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 180
	model.height = 8
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "f wf-root"},
		{Key: "activity:wf-root:approval", Text: "  ○ Await approval [wait-for-approval]"},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{
		frame: frame,
		at:    time.Now(),
		waitAction: liveTUIWaitAction{
			Active: true,
			WaitID: "wait-1",
			StepID: "wait-for-approval",
			Title:  "Await approval",
			Actions: []string{
				"approve",
				"reject",
			},
		},
	})
	model = updated.(liveTUIModel)

	plain := stripTUIANSI(model.View())
	for _, want := range []string{"wait Await approval", "a approve", "r reject"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected footer to contain %q\n%s", want, plain)
		}
	}
}

func TestLiveTUIApproveWaitActionCallsResolver(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 180
	model.height = 8
	var gotWait liveTUIWaitAction
	gotAction := ""
	model.resolveWaitAction = func(wait liveTUIWaitAction, action string) error {
		gotWait = wait
		gotAction = action
		return nil
	}
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "f wf-root"},
		{Key: "activity:wf-root:approval", Text: "  ○ Await approval [wait-for-approval]"},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{
		frame: frame,
		at:    time.Now(),
		waitAction: liveTUIWaitAction{
			Active:  true,
			WaitID:  "wait-1",
			StepID:  "wait-for-approval",
			Actions: []string{"approve", "reject"},
		},
	})
	model = updated.(liveTUIModel)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(liveTUIModel)
	if cmd == nil {
		t.Fatalf("expected approve key to return wait resolver command")
	}
	msg := cmd()
	resolved, ok := msg.(liveTUIWaitResolvedMsg)
	if !ok {
		t.Fatalf("expected wait resolved message, got %#v", msg)
	}
	if resolved.err != nil {
		t.Fatalf("unexpected resolver error: %v", resolved.err)
	}
	if gotWait.WaitID != "wait-1" || gotAction != "approve" {
		t.Fatalf("unexpected resolver call: wait=%#v action=%q", gotWait, gotAction)
	}
	if model.waitActionPending != "approve" {
		t.Fatalf("expected pending approve state, got %q", model.waitActionPending)
	}
}

func TestLiveTUIEnterOpensSelectedResourceURL(t *testing.T) {
	model := newLiveTUIModel()
	model.width = 160
	model.height = 8
	opened := ""
	model.openURL = func(value string) error {
		opened = value
		return nil
	}
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{Key: "header:wf-root", Text: "f wf-root"},
		{Key: "activity:wf-root:persist", Text: "  ƒ Persist run report [persist-run-report]"},
		{
			Key:    "resource:wf-root:report",
			Text:   "    ▣ output resource blob text/markdown",
			WebURL: "http://localhost:30546/ws-acme/runs/live-render-parent/wf-root?artifactUri=demo&output=fullscreen",
		},
	}}
	updated, _ := model.Update(liveTUIFrameMsg{frame: frame, at: time.Now()})
	model = updated.(liveTUIModel)
	if model.cursorKey != "resource:wf-root:report" {
		t.Fatalf("expected sticky bottom to select resource row, got %q", model.cursorKey)
	}

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected enter on resource row to return an open command")
	}
	msg := cmd()
	if _, ok := msg.(liveTUIOpenURLMsg); !ok {
		t.Fatalf("expected open URL message, got %#v", msg)
	}
	if opened != "http://localhost:30546/ws-acme/runs/live-render-parent/wf-root?artifactUri=demo&output=fullscreen" {
		t.Fatalf("unexpected opened URL: %q", opened)
	}
}

func TestEnrichLiveDisplayFrameWebLinksAddsArtifactURLs(t *testing.T) {
	app := &App{WorkspaceID: "ws-acme", APIURL: "http://localhost:30546", DevMode: true}
	frame := live.DisplayFrame{Lines: []live.DisplayLine{
		{
			Key:         "resource:wf-root:persist",
			Text:        "  ▣ output resource blob text/markdown",
			WorkflowID:  "wf-root",
			FlowSlug:    "live-render-parent",
			ResourceURI: "res://v1/ws/ws-acme/result/run/wf-root/step/persist-run-report/output",
		},
		{
			Key:         "resource:wf-root:result",
			Text:        "  ▣ run result",
			WorkflowID:  "wf-root",
			FlowSlug:    "live-render-parent",
			ResourceURI: "res://v1/ws/ws-acme/result/run/wf-root/flow-output",
		},
		{
			Key:         "resource:wf-root:planned",
			Text:        "  ▣ output resource blob text/markdown",
			WorkflowID:  "wf-root",
			FlowSlug:    "live-render-parent",
			ResourceURI: "res://v1/ws/ws-acme/result/run/wf-root/step/future/output",
			WebURL:      "http://localhost:30546/should-not-open",
			Planned:     true,
		},
	}}

	got := enrichLiveDisplayFrameWebLinks(app, frame)
	if got.Lines[0].WebURL != "http://localhost:30546/ws-acme/runs/live-render-parent/wf-root?artifactUri=res%3A%2F%2Fv1%2Fws%2Fws-acme%2Fresult%2Frun%2Fwf-root%2Fstep%2Fpersist-run-report%2Foutput&output=fullscreen" {
		t.Fatalf("unexpected artifact URL: %q", got.Lines[0].WebURL)
	}
	if got.Lines[1].WebURL != "http://localhost:30546/ws-acme/runs/live-render-parent/wf-root/output" {
		t.Fatalf("unexpected flow output URL: %q", got.Lines[1].WebURL)
	}
	if got.Lines[2].WebURL != "" {
		t.Fatalf("expected planned resource row not to be openable, got %q", got.Lines[2].WebURL)
	}
}

func TestLiveTUIInlineMarkersAvoidExtraTreeColumn(t *testing.T) {
	model := newLiveTUIModel()
	parent := liveTreeNode{Key: "parent", Text: "  ✓ @researcher [b0]", Expandable: true}
	leaf := liveTreeNode{Key: "leaf", Text: "  ✓ leaf"}

	parentLine := model.renderNodeLine(parent, false, 120)
	parentPlain := stripTUIANSI(parentLine)
	if strings.Contains(parentPlain, "⌄✓") {
		t.Fatalf("expected marker to be separated from row glyph, got %q", parentPlain)
	}
	if !strings.Contains(parentPlain, "⌄  ✓ @researcher [b0]") {
		t.Fatalf("expected expanded marker in the row gutter, got %q", parentPlain)
	}
	if !strings.Contains(parentLine, "\x1b[") {
		t.Fatalf("expected fold marker to be styled gray, got %q", parentLine)
	}
	leafLine := model.renderNodeLine(leaf, false, 120)
	leafPlain := stripTUIANSI(leafLine)
	if strings.Contains(leafPlain, "⌄") || strings.Contains(leafPlain, "›") {
		t.Fatalf("expected leaf row not to include expand/collapse marker, got %q", leafLine)
	}
	if tuiRuneIndex(parentPlain, "✓") != tuiRuneIndex(leafPlain, "✓") {
		t.Fatalf("expected marker gutter to preserve content indentation: parent=%q leaf=%q", parentPlain, leafPlain)
	}
	model.collapsed = map[string]bool{"parent": true}
	collapsedLine := model.renderNodeLine(parent, false, 120)
	if !strings.Contains(stripTUIANSI(collapsedLine), "›  ✓ @researcher [b0]") {
		t.Fatalf("expected collapsed marker to use right chevron, got %q", stripTUIANSI(collapsedLine))
	}
}

func TestLiveTUISelectionUsesGutterMarkerAndSoftLabelHighlight(t *testing.T) {
	model := newLiveTUIModel()
	line := model.renderNodeLine(liveTreeNode{Key: "step", Text: "  s collect-pause"}, true, 120)
	plain := stripTUIANSI(line)

	if !strings.HasPrefix(plain, "   s collect-pause") {
		t.Fatalf("expected selected row to preserve text indentation, got %q", plain)
	}
	if !strings.Contains(line, "   s \x1b[48;5;236mcollect-pause\x1b[49m") {
		t.Fatalf("expected selected row to highlight only the label text, got %q", line)
	}
	if strings.Contains(line, "\x1b[48;5;236m  s") {
		t.Fatalf("expected selected row not to highlight indentation or type marker, got %q", line)
	}
}

func tuiRuneIndex(value string, needle string) int {
	idx := strings.Index(value, needle)
	if idx < 0 {
		return -1
	}
	return len([]rune(value[:idx]))
}

func TestLiveTUISelectionHighlightsLabelAfterColoredTypeMarker(t *testing.T) {
	got := highlightTUILabelText("\x1b[36mƒ\x1b[0m persist-run-report 610ms")
	if !strings.Contains(got, "\x1b[36mƒ\x1b[0m \x1b[48;5;236mpersist-run-report\x1b[49m 610ms") {
		t.Fatalf("expected only label text to be highlighted after colored type marker, got %q", got)
	}
}

func TestLiveTUISelectionHighlightsBranchLabelAfterBranchMarker(t *testing.T) {
	got := highlightTUILabelText("  ◇ Case id branch 1.4s")
	if !strings.Contains(got, "◇ \x1b[48;5;236mCase id branch\x1b[49m 1.4s") {
		t.Fatalf("expected only branch label text to be highlighted after branch marker, got %q", got)
	}
}

func TestLiveTUISelectionStopsBeforeMetadata(t *testing.T) {
	got := highlightTUILabelText("  \x1b[1;36mƒ\x1b[0m live-render-child [b1] failed")
	if !strings.Contains(got, "ƒ\x1b[0m \x1b[48;5;236mlive-render-child\x1b[49m [b1] failed") {
		t.Fatalf("expected highlight to stop before branch and status metadata, got %q", got)
	}
}

func TestLiveTUISelectionSkipsCompactLoadingAndTypeMarkers(t *testing.T) {
	got := highlightTUILabelText("  ⠋\x1b[1;36mƒ\x1b[0m live-render-child [b1]")
	if !strings.Contains(got, "⠋\x1b[1;36mƒ\x1b[0m \x1b[48;5;236mlive-render-child\x1b[49m [b1]") {
		t.Fatalf("expected compact loading/type marker to stay outside the highlight, got %q", got)
	}
}

func leadingTUISpaces(value string) int {
	count := 0
	for _, r := range value {
		if r != ' ' {
			return count
		}
		count++
	}
	return count
}
