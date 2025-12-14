package mock

import (
        "crypto/rand"
        "encoding/hex"
        "errors"
        "fmt"
        "os"
        "sort"
        "time"

        "breyta-cli/internal/state"
)

const (
        StatusQueued    = "queued"
        StatusPending   = "pending"
        StatusRunning   = "running"
        StatusCompleted = "completed"
        StatusFailed    = "failed"
        StatusCancelled = "cancelled"
        StatusWaiting   = "waiting"
        StatusRetrying  = "retrying"
)

type Store struct {
        Path        string
        WorkspaceID string
}

func (s Store) Ensure() (*state.State, error) {
        st, err := state.Load(s.Path)
        if err == nil {
                return st, nil
        }
        if !os.IsNotExist(err) {
                return nil, err
        }
        seed := state.SeedDefault(s.WorkspaceID)
        if err := state.SaveAtomic(s.Path, seed); err != nil {
                return nil, err
        }
        return seed, nil
}

func (s Store) Load() (*state.State, error) { return state.Load(s.Path) }
func (s Store) Save(st *state.State) error  { return state.SaveAtomic(s.Path, st) }

func (s Store) ListFlows(st *state.State) ([]*state.Flow, error) {
        ws, err := getWS(st, s.WorkspaceID)
        if err != nil {
                return nil, err
        }
        items := make([]*state.Flow, 0, len(ws.Flows))
        for _, f := range ws.Flows {
                items = append(items, f)
        }
        sort.Slice(items, func(i, j int) bool { return items[i].Slug < items[j].Slug })
        return items, nil
}

func (s Store) GetFlow(st *state.State, slug string) (*state.Flow, error) {
        ws, err := getWS(st, s.WorkspaceID)
        if err != nil {
                return nil, err
        }
        f := ws.Flows[slug]
        if f == nil {
                return nil, errors.New("flow not found")
        }
        return f, nil
}

func (s Store) ListRuns(st *state.State, flowSlug string) ([]*state.Run, error) {
        ws, err := getWS(st, s.WorkspaceID)
        if err != nil {
                return nil, err
        }
        items := make([]*state.Run, 0)
        for _, r := range ws.Runs {
                if flowSlug == "" || r.FlowSlug == flowSlug {
                        items = append(items, r)
                }
        }
        sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
        return items, nil
}

func (s Store) GetRun(st *state.State, workflowID string) (*state.Run, error) {
        ws, err := getWS(st, s.WorkspaceID)
        if err != nil {
                return nil, err
        }
        r := ws.Runs[workflowID]
        if r == nil {
                return nil, errors.New("run not found")
        }
        return r, nil
}

func (s Store) StartRun(st *state.State, flowSlug string, version int) (*state.Run, error) {
        ws, err := getWS(st, s.WorkspaceID)
        if err != nil {
                return nil, err
        }
        f := ws.Flows[flowSlug]
        if f == nil {
                return nil, errors.New("flow not found")
        }
        if version == 0 {
                version = f.ActiveVersion
        }
        id, err := randomID("wf-")
        if err != nil {
                return nil, err
        }
        now := time.Now().UTC()
        steps := make([]state.StepExecution, 0, len(f.Steps))
        for _, fs := range f.Steps {
                steps = append(steps, state.StepExecution{
                        StepID:       fs.ID,
                        StepType:     fs.Type,
                        Title:        fs.Title,
                        Status:       StatusPending,
                        Attempt:      0,
                        StartedAt:    time.Time{},
                        InputPreview: nil, // populated when the step starts running
                })
        }
        // First step starts running immediately.
        if len(steps) > 0 {
                steps[0].Status = StatusRunning
                steps[0].Attempt = 1
                steps[0].StartedAt = now
        }
        run := &state.Run{
                WorkflowID:   id,
                FlowSlug:     flowSlug,
                Version:      version,
                Status:       StatusRunning,
                TriggeredBy:  "manual",
                StartedAt:    now,
                UpdatedAt:    now,
                CurrentStep:  steps[0].StepID,
                InputPreview: map[string]any{"flowSlug": flowSlug},
                Steps:        steps,
        }

        // First step starts running immediately with run input as its input.
        if len(run.Steps) > 0 {
                run.Steps[0].InputPreview = run.InputPreview
        }

        ws.Runs[id] = run
        return run, nil
}

func (s Store) Advance(st *state.State, ticks int) error {
        ws, err := getWS(st, s.WorkspaceID)
        if err != nil {
                return err
        }
        if ticks <= 0 {
                ticks = 1
        }
        for i := 0; i < ticks; i++ {
                st.Tick++
                now := time.Now().UTC()
                for _, r := range ws.Runs {
                        if r.Status != StatusRunning {
                                continue
                        }
                        advanceRun(r, st.Tick, now)
                }
        }
        return nil
}

func advanceRun(r *state.Run, tick int64, now time.Time) {
        idx := -1
        for i := range r.Steps {
                if r.Steps[i].Status == StatusRunning || r.Steps[i].Status == StatusRetrying {
                        idx = i
                        break
                }
        }
        if idx == -1 {
                // Find next pending.
                prevOut := latestCompletedOutput(r)
                if prevOut == nil {
                        prevOut = r.InputPreview
                }
                for i := range r.Steps {
                        if r.Steps[i].Status == StatusPending {
                                idx = i
                                r.Steps[i].Status = StatusRunning
                                r.Steps[i].Attempt = 1
                                r.Steps[i].StartedAt = now
                                if r.Steps[i].InputPreview == nil {
                                        r.Steps[i].InputPreview = prevOut
                                }
                                r.CurrentStep = r.Steps[i].StepID
                                break
                        }
                }
                if idx == -1 {
                        r.Status = StatusCompleted
                        r.CurrentStep = ""
                        r.UpdatedAt = now
                        end := now
                        r.CompletedAt = &end
                        r.ResultPreview = map[string]any{"status": "completed"}
                        return
                }
        }

        cur := &r.Steps[idx]
        // Deterministic behavior: every 7th tick causes a retry on HTTP steps once.
        if cur.StepType == "http" && (tick%7 == 0) && cur.Attempt == 1 {
                cur.Status = StatusRetrying
                cur.Error = "transient network error"
                cur.DurationMs = 0
                r.UpdatedAt = now
                return
        }
        // Retrying -> running on next tick
        if cur.Status == StatusRetrying {
                cur.Status = StatusRunning
                cur.Attempt++
                cur.Error = ""
                r.UpdatedAt = now
                return
        }

        // Complete the step.
        cur.Status = StatusCompleted
        end := now
        cur.CompletedAt = &end
        if !cur.StartedAt.IsZero() {
                cur.DurationMs = end.Sub(cur.StartedAt).Milliseconds()
        }
        if cur.InputPreview == nil {
                cur.InputPreview = r.InputPreview
        }
        cur.ResultPreview = stepOutput(cur.StepType, cur.StepID, cur.InputPreview, tick, now)
        cur.Error = ""

        // Start next pending, or finish run.
        next := -1
        for j := range r.Steps {
                if r.Steps[j].Status == StatusPending {
                        next = j
                        break
                }
        }
        if next == -1 {
                r.Status = StatusCompleted
                r.CurrentStep = ""
                end := now
                r.CompletedAt = &end
                r.ResultPreview = map[string]any{"status": "completed"}
        } else {
                r.Steps[next].Status = StatusRunning
                r.Steps[next].Attempt = 1
                r.Steps[next].StartedAt = now
                // Carry output forward: output of step A becomes input of step B.
                r.Steps[next].InputPreview = cur.ResultPreview
                r.CurrentStep = r.Steps[next].StepID
        }

        r.UpdatedAt = now
}

func latestCompletedOutput(r *state.Run) any {
        if r == nil {
                return nil
        }
        for i := len(r.Steps) - 1; i >= 0; i-- {
                if r.Steps[i].Status == StatusCompleted && r.Steps[i].ResultPreview != nil {
                        return r.Steps[i].ResultPreview
                }
        }
        return nil
}

func stepOutput(stepType, stepID string, input any, tick int64, now time.Time) any {
        // Keep outputs "shaped" so they look like real execution artifacts,
        // and so pass-through input→output→input stays coherent.
        switch stepType {
        case "http":
                return map[string]any{
                        "status": 200,
                        "body": map[string]any{
                                "step":      stepID,
                                "received":  input,
                                "tick":      tick,
                                "serverNow": now.Format(time.RFC3339),
                        },
                }
        case "code":
                return map[string]any{
                        "ok":     true,
                        "step":   stepID,
                        "result": map[string]any{"computedFrom": input},
                }
        case "wait":
                return map[string]any{
                        "status":    "succeeded",
                        "signalKey": input,
                        "at":        now.Format(time.RFC3339),
                }
        case "notify":
                return map[string]any{
                        "success": true,
                        "sentAt":  now.Format(time.RFC3339),
                        "input":   input,
                }
        case "llm":
                return map[string]any{
                        "model": "mock-llm",
                        "text":  fmt.Sprintf("summary(%s): ok", stepID),
                        "input": input,
                }
        default:
                return map[string]any{
                        "ok":    true,
                        "step":  stepID,
                        "input": input,
                }
        }
}

func randomID(prefix string) (string, error) {
        buf := make([]byte, 6)
        if _, err := rand.Read(buf); err != nil {
                return "", err
        }
        return prefix + hex.EncodeToString(buf), nil
}

func getWS(st *state.State, workspaceID string) (*state.Workspace, error) {
        if st == nil {
                return nil, errors.New("state is nil")
        }
        ws := st.Workspaces[workspaceID]
        if ws == nil {
                return nil, errors.New("workspace not found")
        }
        if ws.Flows == nil {
                ws.Flows = map[string]*state.Flow{}
        }
        if ws.Runs == nil {
                ws.Runs = map[string]*state.Run{}
        }
        if ws.Registry == nil {
                ws.Registry = map[string]*state.RegistryEntry{}
        }
        if ws.Purchases == nil {
                ws.Purchases = map[string]*state.Purchase{}
        }
        if ws.Entitlements == nil {
                ws.Entitlements = map[string]*state.Entitlement{}
        }
        if ws.Payouts == nil {
                ws.Payouts = map[string]*state.Payout{}
        }
        if ws.Connections == nil {
                ws.Connections = map[string]*state.Connection{}
        }
        if ws.Instances == nil {
                ws.Instances = map[string]*state.Instance{}
        }
        if ws.Triggers == nil {
                ws.Triggers = map[string]*state.Trigger{}
        }
        if ws.Waits == nil {
                ws.Waits = map[string]*state.Wait{}
        }
        return ws, nil
}
