package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/breyta/breyta-cli/internal/live"
)

type fakeLiveBootstrapper struct {
	expiresAt       time.Time
	refreshBeforeMs int
	calls           int
	lastArgs        map[string]any
}

type failingLiveBootstrapper struct {
	calls int
}

type blockingLiveGraphBootstrapper struct {
	calls int
}

type blockingLiveBootstrapper struct {
	calls int
}

type recordingLiveGraphBootstrapper struct {
	workflowIDs []string
}

func (f *failingLiveBootstrapper) DoCommand(_ context.Context, _ string, _ map[string]any) (map[string]any, int, error) {
	f.calls++
	return nil, 0, errors.New("bootstrap unavailable")
}

func (f *blockingLiveGraphBootstrapper) DoCommand(ctx context.Context, command string, _ map[string]any) (map[string]any, int, error) {
	if command != "runs.live.graph" {
		return nil, 0, errors.New("unexpected command")
	}
	f.calls++
	<-ctx.Done()
	return nil, 0, ctx.Err()
}

func (f *blockingLiveBootstrapper) DoCommand(ctx context.Context, command string, _ map[string]any) (map[string]any, int, error) {
	if command != "runs.live.bootstrap" {
		return nil, 0, errors.New("unexpected command")
	}
	f.calls++
	<-ctx.Done()
	return nil, 0, ctx.Err()
}

func (f *recordingLiveGraphBootstrapper) DoCommand(_ context.Context, command string, args map[string]any) (map[string]any, int, error) {
	if command != "runs.live.graph" {
		return nil, 0, errors.New("unexpected command")
	}
	workflowID := firstNonBlankString(args["workflowId"])
	f.workflowIDs = append(f.workflowIDs, workflowID)
	return map[string]any{
		"ok": true,
		"data": map[string]any{
			"workflowId": workflowID,
			"graph": map[string]any{
				"schemaVersion": 1,
				"rootId":        "flow:root",
				"nodes":         []any{},
			},
		},
	}, 200, nil
}

func (f *fakeLiveBootstrapper) DoCommand(_ context.Context, command string, args map[string]any) (map[string]any, int, error) {
	f.calls++
	f.lastArgs = make(map[string]any, len(args))
	for key, value := range args {
		f.lastArgs[key] = value
	}
	return map[string]any{
		"ok": true,
		"data": map[string]any{
			"enabled":         true,
			"workspaceId":     "ws-acme",
			"workflowId":      args["workflowId"],
			"snapshotUrl":     "http://127.0.0.1/live",
			"pollMs":          1000,
			"refreshBeforeMs": f.refreshBeforeMs,
			"auth": map[string]any{
				"type":      "bearer",
				"token":     "token",
				"expiresAt": f.expiresAt.Format(time.RFC3339Nano),
			},
		},
	}, 200, nil
}

func TestLiveWaitRendererSchedulesBootstrapRefreshBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	fake := &fakeLiveBootstrapper{
		expiresAt:       now.Add(5 * time.Minute),
		refreshBeforeMs: 60000,
	}
	renderer := &liveWaitRenderer{
		apiClient:      fake,
		workflowID:     "wf-live",
		installationID: "inst-live",
	}
	if err := renderer.refreshBootstrap(context.Background(), now); err != nil {
		t.Fatalf("refreshBootstrap failed: %v", err)
	}
	want := fake.expiresAt.Add(-time.Minute)
	if !renderer.nextBootstrapAt.Equal(want) {
		t.Fatalf("expected refresh at %s, got %s", want, renderer.nextBootstrapAt)
	}
	if fake.calls != 1 {
		t.Fatalf("expected one bootstrap call, got %d", fake.calls)
	}
	if fake.lastArgs["workflowId"] != "wf-live" || fake.lastArgs["installationId"] != "inst-live" {
		t.Fatalf("expected scoped bootstrap args, got %#v", fake.lastArgs)
	}
}

func TestLiveWaitRendererHonorsBootstrapRetryBackoff(t *testing.T) {
	fake := &failingLiveBootstrapper{}
	var out bytes.Buffer
	renderer := &liveWaitRenderer{apiClient: fake, out: &out}

	renderer.Update(context.Background(), false)
	if fake.calls != 1 {
		t.Fatalf("expected initial bootstrap attempt, got %d", fake.calls)
	}
	renderer.Update(context.Background(), false)
	if fake.calls != 1 {
		t.Fatalf("expected failed bootstrap to honor backoff, got %d calls", fake.calls)
	}

	renderer.nextBootstrapAt = time.Now().Add(-time.Second)
	renderer.Update(context.Background(), false)
	if fake.calls != 2 {
		t.Fatalf("expected retry after backoff, got %d calls", fake.calls)
	}
}

func TestLiveWaitRendererBoundsBootstrapRefresh(t *testing.T) {
	fake := &blockingLiveBootstrapper{}
	renderer := &liveWaitRenderer{apiClient: fake}

	startedAt := time.Now()
	err := renderer.refreshBootstrap(context.Background(), startedAt)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded bootstrap timeout, got %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("expected bootstrap refresh to be bounded, elapsed=%s", elapsed)
	}
	if fake.calls != 1 {
		t.Fatalf("expected one bootstrap call, got %d", fake.calls)
	}
}

func TestLiveWaitRendererStartsSnapshotFetchWithoutBlocking(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()

	renderer := &liveWaitRenderer{
		bootstrapOK: true,
		bootstrap: live.Bootstrap{
			SnapshotURL: srv.URL,
			PollMs:      1,
		},
		snapshotClient: live.SnapshotClient{HTTP: &http.Client{Timeout: 10 * time.Second}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startedAt := time.Now()
	renderer.Update(ctx, false)
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("expected live wait update to return before snapshot response, elapsed=%s", elapsed)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected asynchronous snapshot request to start")
	}
	renderer.Close()
}

func TestLiveWaitRendererBoundsOptionalGraphHydration(t *testing.T) {
	fake := &blockingLiveGraphBootstrapper{}
	renderer := &liveWaitRenderer{apiClient: fake}
	snapshot := live.Snapshot{Runs: []live.RunState{{WorkflowID: "wf-root", RootWorkflowID: "wf-root"}}}

	done := make(chan struct{})
	go func() {
		_ = renderer.enrichSnapshotWithGraphs(context.Background(), snapshot)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected optional graph hydration to be bounded")
	}
	if fake.calls != 1 {
		t.Fatalf("expected one graph hydration request, got %d", fake.calls)
	}
}

func TestLiveWaitRendererFocusesBeforeGraphHydration(t *testing.T) {
	fake := &recordingLiveGraphBootstrapper{}
	renderer := &liveWaitRenderer{apiClient: fake, workflowID: "wf-target"}
	snapshot := live.Snapshot{Runs: []live.RunState{
		{WorkflowID: "wf-target", RootWorkflowID: "wf-target"},
		{WorkflowID: "wf-other", RootWorkflowID: "wf-other"},
	}}

	renderer.applySnapshot(context.Background(), snapshot, time.Now())
	if len(fake.workflowIDs) != 1 || fake.workflowIDs[0] != "wf-target" {
		t.Fatalf("expected graph hydration to stay within focused tree, got %#v", fake.workflowIDs)
	}
}

func TestLiveWaitRendererSuppressesUnchangedNonInteractiveFrames(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	renderer := &liveWaitRenderer{
		displayedLines:         2,
		lastDisplayKey:         "same",
		lastRenderedDisplayKey: "same",
		lastRenderAt:           now.Add(-time.Hour),
		interactive:            false,
	}

	if renderer.shouldRender(now, false) {
		t.Fatalf("expected unchanged non-interactive frame to be suppressed")
	}
	if !renderer.shouldRender(now, true) {
		t.Fatalf("expected final frame to render")
	}
}

func TestLiveWaitRendererRendersChangedSnapshotsAndInteractiveHeartbeat(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	changed := &liveWaitRenderer{
		displayedLines:         2,
		lastDisplayKey:         "new",
		lastRenderedDisplayKey: "old",
		lastRenderAt:           now,
	}
	if !changed.shouldRender(now, false) {
		t.Fatalf("expected changed snapshot to render")
	}

	waitAction := &liveWaitRenderer{
		displayedLines:         2,
		lastDisplayKey:         "same",
		lastRenderedDisplayKey: "same",
		lastSnapshot: &live.Snapshot{Runs: []live.RunState{{
			WorkflowID: "wf-live",
			Status:     "completed",
		}}},
		waitAction: liveTUIWaitAction{
			Active:     true,
			WaitID:     "wait-1",
			WorkflowID: "wf-live",
			StepID:     "approve",
			Title:      "Approve",
			Actions:    []string{"approve", "reject"},
		},
	}
	if !waitAction.shouldRender(now, false) {
		t.Fatalf("expected wait-action-only change to render after the snapshot became idle")
	}

	heartbeat := &liveWaitRenderer{
		displayedLines:         2,
		lastDisplayKey:         "same",
		lastRenderedDisplayKey: "same",
		lastRenderAt:           now.Add(-liveRenderFrameInterval),
		interactive:            true,
	}
	if !heartbeat.shouldRender(now, false) {
		t.Fatalf("expected interactive heartbeat to render")
	}
}

func TestLiveWaitRendererSuppressesFinalResultOnlyForInteractiveOK(t *testing.T) {
	interactive := &liveWaitRenderer{
		interactive:       true,
		stdoutInteractive: true,
		displayedLines:    1,
	}
	if !interactive.shouldSuppressFinalResult(map[string]any{"ok": true}, 200) {
		t.Fatalf("expected interactive successful live run to suppress final JSON")
	}
	if interactive.shouldSuppressFinalResult(map[string]any{"ok": false}, 200) {
		t.Fatalf("expected ok=false result to remain printable")
	}
	for _, status := range []string{"failed", "cancelled"} {
		out := map[string]any{
			"ok": true,
			"data": map[string]any{
				"run": map[string]any{"status": status},
			},
		}
		if interactive.shouldSuppressFinalResult(out, 200) {
			t.Fatalf("expected %s run payload to remain printable", status)
		}
	}
	if interactive.shouldSuppressFinalResult(map[string]any{"ok": true}, 500) {
		t.Fatalf("expected error status result to remain printable")
	}

	nonInteractive := &liveWaitRenderer{interactive: false}
	if nonInteractive.shouldSuppressFinalResult(map[string]any{"ok": true}, 200) {
		t.Fatalf("expected non-interactive output to keep final JSON")
	}

	redirectedStdout := &liveWaitRenderer{interactive: true, stdoutInteractive: false}
	if redirectedStdout.shouldSuppressFinalResult(map[string]any{"ok": true}, 200) {
		t.Fatalf("expected redirected stdout to keep final JSON while live UI uses stderr")
	}

	fallback := &liveWaitRenderer{interactive: true, stdoutInteractive: true}
	if fallback.shouldSuppressFinalResult(map[string]any{"ok": true}, 200) {
		t.Fatalf("expected wait fallback result to remain visible when live output never rendered")
	}
}

func TestLiveWaitRendererKeepsFinalResultVisibleWhenRefreshIsFirstOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"runs":[],"nodes":[]}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	renderer := &liveWaitRenderer{
		interactive:       true,
		stdoutInteractive: true,
		bootstrapOK:       true,
		bootstrap:         live.Bootstrap{SnapshotURL: srv.URL},
		snapshotClient:    live.SnapshotClient{HTTP: srv.Client()},
		out:               &out,
	}

	renderer.Update(context.Background(), true)
	if renderer.shouldSuppressFinalResult(map[string]any{"ok": true}, 200) {
		t.Fatal("expected final result to remain visible when final refresh was the first live output")
	}
	if !renderer.finalRefreshAttempted || renderer.finalRefreshHadLiveOutput {
		t.Fatalf("expected final refresh to remember that no output existed before it: attempted=%t hadOutput=%t", renderer.finalRefreshAttempted, renderer.finalRefreshHadLiveOutput)
	}
	renderer.Close()
}

func TestLiveWaitRendererPrintsFinalSummaryAfterClosingTUI(t *testing.T) {
	var out bytes.Buffer
	renderer := &liveWaitRenderer{
		interactive:               true,
		stdoutInteractive:         true,
		displayedLines:            2,
		lastRenderedText:          "f wf-live\n  ✓ completed",
		finalRefreshAttempted:     true,
		finalRefreshHadLiveOutput: true,
		tui:                       &liveTUIRunner{},
		out:                       &out,
	}

	if !renderer.shouldSuppressFinalResult(map[string]any{"ok": true}, 200) {
		t.Fatal("expected interactive successful result to use the final live summary")
	}
	renderer.closeAndPrintFinalSummary()
	if renderer.tui != nil {
		t.Fatal("expected final summary path to close the live TUI")
	}
	if got := out.String(); !strings.Contains(got, "f wf-live") || !strings.Contains(got, "completed") {
		t.Fatalf("expected final live summary after TUI close, got %q", got)
	}
}

func TestLiveWaitRendererReturnsImmediatelyForTerminalRun(t *testing.T) {
	renderer := &liveWaitRenderer{
		interactive: true,
		tui:         &liveTUIRunner{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	started := time.Now()
	renderer.WaitForExit(ctx, true)
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("expected terminal live wait to return promptly, elapsed=%s", elapsed)
	}
}

func TestLiveWaitRendererAppendsRedirectedFramesWithoutANSI(t *testing.T) {
	var out bytes.Buffer
	renderer := &liveWaitRenderer{
		displayedLines: 2,
		interactive:    false,
		out:            &out,
	}

	renderer.redrawLiveBlock(live.DisplayFrame{}, "next frame")

	if got := out.String(); got != "\nnext frame" {
		t.Fatalf("expected redirected frame to append on a new line without redraw controls, got %q", got)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("expected redirected frame to contain no ANSI cursor controls, got %q", out.String())
	}
}

func TestLiveWaitRendererSendsMetadataOnlyFrameChanges(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	snapshot := func(resourceURI string) live.Snapshot {
		return live.Snapshot{
			Runs: []live.RunState{{
				WorkspaceID: "ws-acme",
				WorkflowID:  "wf-live",
				FlowSlug:    "metadata-flow",
				Status:      "running",
				Active:      true,
			}},
			Nodes: []live.Activity{{
				WorkspaceID:   "ws-acme",
				WorkflowID:    "wf-live",
				ActivityKind:  "resource",
				ResourceKind:  "blob",
				ResourceLabel: "artifact",
				ResourceURI:   resourceURI,
			}},
		}
	}

	first := snapshot("res://v1/ws/ws-acme/result/run/wf-live/flow-output?version=first")
	second := snapshot("res://v1/ws/ws-acme/result/run/wf-live/flow-output?version=second")
	if displayFrameKey(first, "wf-live") == displayFrameKey(second, "wf-live") {
		t.Fatal("expected the resource metadata change to update the display key")
	}
	if got := strings.TrimSuffix(
		live.RenderDisplayFrame(live.CollectDisplayFrame(first, live.RenderOptions{
			Now: now, FocusWorkflowID: "wf-live", FullTree: true,
		})),
		"\n",
	); got != strings.TrimSuffix(
		live.RenderDisplayFrame(live.CollectDisplayFrame(second, live.RenderOptions{
			Now: now, FocusWorkflowID: "wf-live", FullTree: true,
		})),
		"\n",
	) {
		t.Fatalf("expected metadata-only frames to have the same terminal text")
	}

	var out bytes.Buffer
	renderer := &liveWaitRenderer{
		workflowID: "wf-live",
		out:        &out,
	}
	renderer.render(first, now)
	initial := out.String()
	renderer.render(second, now)
	if out.String() == initial {
		t.Fatalf("expected metadata-only frame change to be sent, output=%q", out.String())
	}
}
