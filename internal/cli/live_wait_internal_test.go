package cli

import (
	"context"
	"testing"
	"time"
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
	interactive := &liveWaitRenderer{interactive: true}
	if !interactive.shouldSuppressFinalResult(map[string]any{"ok": true}, 200) {
		t.Fatalf("expected interactive successful live run to suppress final JSON")
	}
	if interactive.shouldSuppressFinalResult(map[string]any{"ok": false}, 200) {
		t.Fatalf("expected ok=false result to remain printable")
	}
	if interactive.shouldSuppressFinalResult(map[string]any{"ok": true}, 500) {
		t.Fatalf("expected error status result to remain printable")
	}

	nonInteractive := &liveWaitRenderer{interactive: false}
	if nonInteractive.shouldSuppressFinalResult(map[string]any{"ok": true}, 200) {
		t.Fatalf("expected non-interactive output to keep final JSON")
	}
}
