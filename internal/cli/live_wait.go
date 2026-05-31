package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/breyta/breyta-cli/internal/live"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type liveBootstrapper interface {
	DoCommand(ctx context.Context, command string, args map[string]any) (map[string]any, int, error)
}

type liveWaitRenderer struct {
	app              *App
	cmd              *cobra.Command
	apiClient        liveBootstrapper
	snapshotClient   live.SnapshotClient
	streamClient     live.StreamClient
	workflowID       string
	bootstrap        live.Bootstrap
	bootstrapOK      bool
	nextBootstrapAt  time.Time
	nextSnapshotAt   time.Time
	nextStreamAt     time.Time
	streamCancel     context.CancelFunc
	streamSnapshots  chan live.Snapshot
	streamErrors     chan error
	streamKey        string
	streamRunning    bool
	graphsByWorkflow map[string]live.FlowGraphDocument

	lastSnapshot           *live.Snapshot
	lastDisplayKey         string
	lastRenderedDisplayKey string
	lastRenderAt           time.Time
	lastRenderedText       string
	displayedLines         int
	tui                    *liveTUIRunner
	frame                  int
	warnedUnavailable      bool
	interactive            bool
	color                  bool
	out                    io.Writer
	diagnostics            *liveDiagnostics
}

const liveRenderFrameInterval = time.Second / 60

func newLiveWaitRenderer(cmd *cobra.Command, app *App, client liveBootstrapper, workflowID string) *liveWaitRenderer {
	out := cmd.ErrOrStderr()
	interactive := false
	if f, ok := out.(*os.File); ok {
		interactive = isatty.IsTerminal(f.Fd())
	}
	color := interactive && strings.TrimSpace(os.Getenv("NO_COLOR")) == ""
	return &liveWaitRenderer{
		app:              app,
		cmd:              cmd,
		apiClient:        client,
		snapshotClient:   live.SnapshotClient{HTTP: &http.Client{Timeout: 10 * time.Second}},
		streamClient:     live.StreamClient{HTTP: &http.Client{Timeout: 0}},
		workflowID:       strings.TrimSpace(workflowID),
		interactive:      interactive,
		color:            color,
		out:              out,
		graphsByWorkflow: map[string]live.FlowGraphDocument{},
		diagnostics:      newLiveDiagnostics(out, interactive),
	}
}

func (r *liveWaitRenderer) Update(ctx context.Context, final bool) {
	if r == nil {
		return
	}
	now := time.Now()
	if !r.bootstrapOK || (!r.nextBootstrapAt.IsZero() && !now.Before(r.nextBootstrapAt)) {
		if err := r.refreshBootstrap(ctx, now); err != nil {
			if !r.bootstrapOK && !r.warnedUnavailable {
				_, _ = fmt.Fprintf(r.out, "live output unavailable, falling back to wait polling: %v\n", err)
				r.warnedUnavailable = true
			}
			r.nextBootstrapAt = now.Add(10 * time.Second)
			return
		}
	}

	if r.bootstrapOK {
		r.ensureStream(ctx, now)
		r.drainStream(ctx, now)
	}

	snapshotFallbackDue := r.streamKey == "" || !r.streamRunning
	if r.bootstrapOK && (final ||
		r.lastSnapshot == nil ||
		(snapshotFallbackDue && (r.nextSnapshotAt.IsZero() || !now.Before(r.nextSnapshotAt)))) {
		if snapshot, err := r.snapshotClient.Fetch(ctx, r.bootstrap); err == nil {
			snapshot = r.enrichSnapshotWithGraphs(ctx, snapshot)
			focused := snapshot.Focus(r.workflowID)
			r.lastSnapshot = &focused
			r.lastDisplayKey = displayFrameKey(focused, r.workflowID)
			r.nextSnapshotAt = now.Add(r.bootstrap.PollInterval(time.Second))
		}
	}
	if r.lastSnapshot == nil {
		return
	}
	if !r.shouldRender(now, final) {
		return
	}
	r.render(*r.lastSnapshot, now)
}

func (r *liveWaitRenderer) Close() {
	if r == nil {
		return
	}
	if r.streamCancel != nil {
		r.streamCancel()
		r.streamCancel = nil
	}
	if r.tui != nil {
		r.tui.Close()
		r.tui = nil
	} else if r.displayedLines > 0 {
		_, _ = fmt.Fprintln(r.out)
	}
	if r.diagnostics != nil {
		r.diagnostics.Close()
		r.diagnostics = nil
	}
}

func (r *liveWaitRenderer) StopRequested() bool {
	return r != nil && r.tui != nil && r.tui.StopRequested()
}

func (r *liveWaitRenderer) WaitForExit(ctx context.Context) {
	if r == nil || !r.interactive || r.tui == nil {
		return
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if r.StopRequested() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *liveWaitRenderer) shouldSuppressFinalResult(out map[string]any, status int) bool {
	if r == nil || !r.interactive || status >= 400 || out == nil {
		return false
	}
	if ok, exists := out["ok"].(bool); exists && !ok {
		return false
	}
	return true
}

func (r *liveWaitRenderer) refreshBootstrap(ctx context.Context, now time.Time) error {
	args := map[string]any{}
	if r.workflowID != "" {
		args["workflowId"] = r.workflowID
	}
	out, status, err := r.apiClient.DoCommand(ctx, "runs.live.bootstrap", args)
	if err != nil {
		return err
	}
	if status >= 400 || !isOK(out) {
		return errors.New(formatAPIError(out))
	}
	data, _ := out["data"].(map[string]any)
	bootstrap, err := live.DecodeBootstrap(data)
	if err != nil {
		return err
	}
	r.bootstrap = bootstrap
	r.bootstrapOK = true
	r.resetStreamIfBootstrapChanged()

	if expiresAt, ok := bootstrap.TokenExpiresAt(); ok {
		refreshAt := expiresAt.Add(-bootstrap.RefreshBefore(time.Minute))
		if !refreshAt.After(now) {
			refreshAt = now.Add(30 * time.Second)
		}
		r.nextBootstrapAt = refreshAt
	} else {
		r.nextBootstrapAt = time.Time{}
	}
	return nil
}

func (r *liveWaitRenderer) resetStreamIfBootstrapChanged() {
	streamKey := strings.TrimSpace(r.bootstrap.StreamURL) + "\x00" + strings.TrimSpace(r.bootstrap.Auth.Token)
	if streamKey == r.streamKey {
		return
	}
	if r.streamCancel != nil {
		r.streamCancel()
		r.streamCancel = nil
	}
	r.streamKey = ""
	r.streamRunning = false
	r.streamSnapshots = nil
	r.streamErrors = nil
	r.nextStreamAt = time.Time{}
	if strings.TrimSpace(r.bootstrap.StreamURL) != "" {
		r.streamKey = streamKey
	}
}

func (r *liveWaitRenderer) ensureStream(ctx context.Context, now time.Time) {
	if strings.TrimSpace(r.streamKey) == "" || r.streamRunning {
		return
	}
	if !r.nextStreamAt.IsZero() && now.Before(r.nextStreamAt) {
		return
	}
	streamCtx, cancel := context.WithCancel(ctx)
	r.streamCancel = cancel
	r.streamSnapshots = make(chan live.Snapshot, 8)
	r.streamErrors = make(chan error, 1)
	r.streamRunning = true
	bootstrap := r.bootstrap
	go func() {
		err := r.streamClient.Stream(streamCtx, bootstrap, r.streamSnapshots)
		if err != nil && errors.Is(err, context.Canceled) {
			err = nil
		}
		r.streamErrors <- err
	}()
}

func (r *liveWaitRenderer) drainStream(ctx context.Context, now time.Time) {
	for {
		select {
		case snapshot := <-r.streamSnapshots:
			snapshot = r.enrichSnapshotWithGraphs(ctx, snapshot)
			focused := snapshot.Focus(r.workflowID)
			r.lastSnapshot = &focused
			r.lastDisplayKey = displayFrameKey(focused, r.workflowID)
		case err := <-r.streamErrors:
			_ = err
			if r.streamCancel != nil {
				r.streamCancel()
				r.streamCancel = nil
			}
			r.streamRunning = false
			r.nextStreamAt = now.Add(time.Second)
			r.nextSnapshotAt = time.Time{}
			return
		case <-ctx.Done():
			return
		default:
			return
		}
	}
}

func (r *liveWaitRenderer) enrichSnapshotWithGraphs(ctx context.Context, snapshot live.Snapshot) live.Snapshot {
	if r == nil || r.apiClient == nil || len(snapshot.Runs) == 0 {
		return snapshot
	}
	if r.graphsByWorkflow == nil {
		r.graphsByWorkflow = map[string]live.FlowGraphDocument{}
	}
	for _, run := range snapshot.Runs {
		workflowID := strings.TrimSpace(run.WorkflowID)
		if workflowID == "" {
			continue
		}
		if strings.TrimSpace(run.ParentWorkflowID) != "" || strings.TrimSpace(run.RelationKind) != "" {
			continue
		}
		if doc, ok := r.graphsByWorkflow[workflowID]; ok {
			snapshot = snapshot.WithFlowGraph(doc)
			continue
		}
		doc, ok := r.fetchFlowGraph(ctx, workflowID)
		if !ok {
			continue
		}
		r.graphsByWorkflow[workflowID] = doc
		snapshot = snapshot.WithFlowGraph(doc)
	}
	return snapshot
}

func (r *liveWaitRenderer) fetchFlowGraph(ctx context.Context, workflowID string) (live.FlowGraphDocument, bool) {
	var doc live.FlowGraphDocument
	out, status, err := r.apiClient.DoCommand(ctx, "runs.live.graph", map[string]any{"workflowId": workflowID})
	if err != nil || status >= 400 || !isOK(out) {
		return doc, false
	}
	data, _ := out["data"].(map[string]any)
	if len(data) == 0 {
		return doc, false
	}
	doc, err = live.DecodeFlowGraphDocument(data)
	if err != nil {
		return live.FlowGraphDocument{}, false
	}
	return doc, true
}

func (r *liveWaitRenderer) shouldRender(now time.Time, final bool) bool {
	if final || r.displayedLines == 0 {
		return true
	}
	if r.lastDisplayKey != "" && r.lastDisplayKey != r.lastRenderedDisplayKey {
		return true
	}
	if r.lastSnapshot != nil && len(r.lastSnapshot.Runs) > 0 && !r.lastSnapshot.HasActiveWork() {
		return false
	}
	return r.interactive && now.Sub(r.lastRenderAt) >= liveRenderFrameInterval
}

func (r *liveWaitRenderer) render(snapshot live.Snapshot, now time.Time) {
	const spinnerFrameDivisor = 6
	opts := live.RenderOptions{
		Now:             now,
		Frame:           r.frame / spinnerFrameDivisor,
		Color:           r.color,
		FocusWorkflowID: r.workflowID,
		FullTree:        true,
		Diagnostics:     r.logDiagnostic,
	}
	r.frame++
	displayKey := displayFrameKey(snapshot, r.workflowID)

	frame := enrichLiveDisplayFrameWebLinks(r.app, live.CollectDisplayFrame(snapshot, opts))
	text := strings.TrimSuffix(live.RenderDisplayFrame(frame), "\n")
	if text == r.lastRenderedText {
		r.lastRenderedDisplayKey = displayKey
		return
	}

	r.redrawLiveBlock(frame, text)
	r.displayedLines = countLiveBlockLines(text)
	r.lastRenderedText = text
	r.lastRenderAt = now
	r.lastDisplayKey = displayKey
	r.lastRenderedDisplayKey = displayKey
}

func (r *liveWaitRenderer) logDiagnostic(diagnostic live.RenderDiagnostic) {
	if r == nil || r.diagnostics == nil {
		return
	}
	r.diagnostics.Log(diagnostic)
}

type liveDiagnostics struct {
	mu     sync.Mutex
	w      io.Writer
	closer io.Closer
	seen   map[string]bool
}

func newLiveDiagnostics(out io.Writer, interactive bool) *liveDiagnostics {
	value := strings.TrimSpace(os.Getenv("BREYTA_LIVE_DEBUG"))
	if value == "" {
		return nil
	}
	target := strings.ToLower(value)
	if target == "0" || target == "false" || target == "off" || target == "no" {
		return nil
	}
	var w io.Writer
	var closer io.Closer
	if (target == "1" || target == "true" || target == "on" || target == "stderr") && !interactive {
		w = out
	} else {
		path := value
		if target == "1" || target == "true" || target == "on" || target == "stderr" {
			path = filepath.Join(os.TempDir(), "breyta-live-debug.log")
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil
		}
		w = f
		closer = f
	}
	return &liveDiagnostics{w: w, closer: closer, seen: map[string]bool{}}
}

func (d *liveDiagnostics) Log(diagnostic live.RenderDiagnostic) {
	if d == nil || d.w == nil || strings.TrimSpace(diagnostic.Code) == "" {
		return
	}
	key := strings.Join([]string{
		diagnostic.Code,
		diagnostic.WorkflowID,
		diagnostic.ActivityID,
		diagnostic.StepID,
		diagnostic.ParentRef,
		diagnostic.ScopeID,
	}, "\x00")
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen[key] {
		return
	}
	d.seen[key] = true
	entry := map[string]any{
		"ts":         time.Now().Format(time.RFC3339Nano),
		"component":  "breyta-cli/live-render",
		"diagnostic": diagnostic,
	}
	_ = json.NewEncoder(d.w).Encode(entry)
}

func (d *liveDiagnostics) Close() {
	if d == nil || d.closer == nil {
		return
	}
	_ = d.closer.Close()
	d.closer = nil
}

func (r *liveWaitRenderer) redrawLiveBlock(frame live.DisplayFrame, text string) {
	if r == nil {
		return
	}
	if r.interactive {
		if r.tui == nil {
			r.tui = newLiveTUIRunner(r.out)
		}
		r.tui.SendFrame(frame)
		return
	}
	if r.displayedLines > 0 {
		clearLiveBlock(r.out, r.displayedLines)
	}
	_, _ = io.WriteString(r.out, text)
}

func clearLiveBlock(w io.Writer, lines int) {
	if lines <= 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "\x1b[%dA", lines)
	_, _ = io.WriteString(w, "\r\x1b[J")
}

func countLiveBlockLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func displayFrameKey(snapshot live.Snapshot, workflowID string) string {
	return live.DisplayFrameKey(snapshot, live.RenderOptions{
		FocusWorkflowID: workflowID,
		FullTree:        true,
	})
}

func sleepWithLiveUpdates(ctx context.Context, renderer *liveWaitRenderer, total time.Duration) {
	if total <= 0 {
		return
	}
	if renderer == nil || !renderer.interactive {
		time.Sleep(total)
		return
	}
	deadline := time.Now().Add(total)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return
		}
		sleepFor := liveRenderFrameInterval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		timer := time.NewTimer(sleepFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			renderer.Update(ctx, false)
			if renderer.StopRequested() {
				return
			}
		}
	}
}
