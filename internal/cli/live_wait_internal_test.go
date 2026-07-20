package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/breyta/breyta-cli/internal/live"
)

type fakeLiveBootstrapper struct {
	expiresAt       time.Time
	refreshBeforeMs int
	calls           int
}

func (f *fakeLiveBootstrapper) DoCommand(_ context.Context, command string, args map[string]any) (map[string]any, int, error) {
	f.calls++
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
		apiClient:  fake,
		workflowID: "wf-live",
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
