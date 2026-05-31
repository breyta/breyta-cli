package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestActiveLiveTUIWaitActionFromItemsUsesLatestActiveWaitWithUIActions(t *testing.T) {
	items := []map[string]any{
		{
			"waitId":       "old",
			"workflowId":   "wf-live",
			"status":       "active",
			"registeredAt": time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"stepId":       "old-step",
		},
		{
			"waitId":       "done",
			"workflowId":   "wf-live",
			"status":       "completed",
			"registeredAt": time.Date(2026, 5, 31, 10, 2, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"stepId":       "done-step",
		},
		{
			"waitId":       "new",
			"workflowId":   "wf-live",
			"status":       "active",
			"registeredAt": time.Date(2026, 5, 31, 10, 1, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"stepId":       "wait-for-approval",
			"notify": map[string]any{
				"channels": map[string]any{
					"ui": map[string]any{
						"title":   "Await approval",
						"message": "Review the request",
						"actions": []any{"approve", "reject"},
					},
				},
			},
		},
	}

	got := activeLiveTUIWaitActionFromItems("wf-live", items)
	if !got.Active || got.WaitID != "new" || got.StepID != "wait-for-approval" {
		t.Fatalf("unexpected active wait: %#v", got)
	}
	if got.Title != "Await approval" || got.Message != "Review the request" {
		t.Fatalf("expected notify UI title/message, got %#v", got)
	}
	if !got.Can("approve") || !got.Can("reject") {
		t.Fatalf("expected approve/reject actions, got %#v", got.Actions)
	}
}

func TestActiveLiveTUIWaitActionFromItemsKeepsGenericWaitActiveWithoutActions(t *testing.T) {
	got := activeLiveTUIWaitActionFromItems("wf-live", []map[string]any{{
		"waitId":     "wait-generic",
		"workflowId": "wf-live",
		"stepId":     "wait-for-signal",
		"status":     "active",
	}})
	if !got.Active || got.WaitID != "wait-generic" {
		t.Fatalf("expected generic active wait, got %#v", got)
	}
	if len(got.Actions) != 0 {
		t.Fatalf("expected no generic wait actions, got %#v", got.Actions)
	}
}

func TestActiveLiveTUIWaitActionFromItemsSortsNumericWaitTimestamps(t *testing.T) {
	got := activeLiveTUIWaitActionFromItems("wf-live", []map[string]any{
		{
			"waitId":       "old",
			"workflowId":   "wf-live",
			"status":       "active",
			"registeredAt": float64(1780249411000),
		},
		{
			"waitId":       "new",
			"workflowId":   "wf-live",
			"status":       "active",
			"registeredAt": float64(1780249419000),
		},
	})
	if got.WaitID != "new" {
		t.Fatalf("expected newer numeric wait timestamp to win, got %#v", got)
	}
}

func TestLiveWaitRendererResolveTUIWaitActionPostsApproveAndReject(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	renderer := &liveWaitRenderer{app: &App{APIURL: srv.URL, WorkspaceID: "ws-acme", Token: "token"}}
	wait := liveTUIWaitAction{WaitID: "wait-1"}
	if err := renderer.resolveTUIWaitAction(wait, "approve"); err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if err := renderer.resolveTUIWaitAction(wait, "reject"); err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	want := []string{"POST /api/waits/wait-1/approve", "POST /api/waits/wait-1/reject"}
	if len(paths) != len(want) {
		t.Fatalf("unexpected requests: %#v", paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("request %d: got %q want %q", i, paths[i], want[i])
		}
	}
}
