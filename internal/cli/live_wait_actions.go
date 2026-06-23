package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const liveWaitActionPollInterval = time.Second

type liveTUIWaitAction struct {
	Active     bool
	WaitID     string
	WorkflowID string
	StepID     string
	Title      string
	Message    string
	Actions    []string
	UpdatedAt  time.Time
}

func (w liveTUIWaitAction) Can(action string) bool {
	if !w.Active || strings.TrimSpace(w.WaitID) == "" {
		return false
	}
	return containsFold(w.Actions, action)
}

func (w liveTUIWaitAction) Label() string {
	for _, value := range []string{w.Title, w.StepID, w.WaitID} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return "wait"
}

func liveTUIWaitActionKey(wait liveTUIWaitAction) string {
	if !wait.Active {
		return ""
	}
	return strings.Join([]string{
		wait.WaitID,
		wait.WorkflowID,
		wait.StepID,
		wait.Title,
		wait.Message,
		strings.Join(wait.Actions, ","),
	}, "\x00")
}

func (r *liveWaitRenderer) refreshWaitAction(ctx context.Context, now time.Time, force bool) {
	if r == nil || !r.interactive || r.app == nil || strings.TrimSpace(r.workflowID) == "" || !isAPIMode(r.app) {
		if r != nil {
			r.waitAction = liveTUIWaitAction{}
		}
		return
	}
	if !force && !r.nextWaitActionAt.IsZero() && now.Before(r.nextWaitActionAt) {
		return
	}
	wait, err := fetchLiveTUIWaitAction(ctx, r.app, r.workflowID, 25)
	if err == nil {
		wait.UpdatedAt = now
		r.waitAction = wait
	}
	r.nextWaitActionAt = now.Add(liveWaitActionPollInterval)
}

func (r *liveWaitRenderer) ActiveWait() bool {
	if r == nil || !r.interactive || !r.waitAction.Active {
		return false
	}
	if r.waitAction.UpdatedAt.IsZero() {
		return true
	}
	return time.Since(r.waitAction.UpdatedAt) <= 10*liveWaitActionPollInterval
}

func (r *liveWaitRenderer) resolveTUIWaitAction(wait liveTUIWaitAction, action string) error {
	if r == nil || r.app == nil {
		return fmt.Errorf("live wait resolver is unavailable")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" && action != "reject" {
		return fmt.Errorf("unsupported wait action %q", action)
	}
	waitID := strings.TrimSpace(wait.WaitID)
	if waitID == "" {
		return fmt.Errorf("selected wait is missing waitId")
	}
	out, status, err := apiClient(r.app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/"+action, nil, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return liveWaitActionRESTError(action, status, out)
	}
	return nil
}

func liveWaitActionRESTError(action string, status int, out any) error {
	root := mapStringAny(out)
	if root != nil {
		if errMap := mapStringAny(root["error"]); errMap != nil {
			if message := firstNonBlankString(errMap["message"], errMap["code"]); message != "" {
				return fmt.Errorf("wait %s failed: %s", action, message)
			}
		}
		if message := firstNonBlankString(root["message"], root["error"]); message != "" {
			return fmt.Errorf("wait %s failed: %s", action, message)
		}
	}
	return fmt.Errorf("wait %s failed with HTTP %d", action, status)
}

func fetchLiveTUIWaitAction(ctx context.Context, app *App, workflowID string, limit int) (liveTUIWaitAction, error) {
	workflowID = strings.TrimSpace(workflowID)
	if app == nil || workflowID == "" {
		return liveTUIWaitAction{}, nil
	}
	if limit <= 0 {
		limit = 25
	}
	q := url.Values{}
	q.Set("workflowId", workflowID)
	q.Set("limit", fmt.Sprintf("%d", limit))
	out, status, err := apiClient(app).DoREST(ctx, http.MethodGet, "/api/waits", q, nil)
	if err != nil {
		return liveTUIWaitAction{}, err
	}
	if status >= 400 {
		return liveTUIWaitAction{}, liveWaitActionRESTError("list", status, out)
	}
	items := filterWaitItemsForWorkflow(waitItems(out), workflowID)
	return activeLiveTUIWaitActionFromItems(workflowID, items), nil
}

func activeLiveTUIWaitActionFromItems(workflowID string, items []map[string]any) liveTUIWaitAction {
	var selected map[string]any
	var selectedTime time.Time
	selectedActionable := false
	for i, item := range items {
		if !waitLooksActive(item) {
			continue
		}
		t := waitSortTime(item)
		actionable := waitItemHasResolvableTUIAction(item)
		if selected != nil && selectedActionable && !actionable {
			continue
		}
		if selected == nil || (actionable && !selectedActionable) || t.After(selectedTime) || (t.IsZero() && selectedTime.IsZero() && i == 0) {
			selected = item
			selectedTime = t
			selectedActionable = actionable
		}
	}
	if selected == nil {
		return liveTUIWaitAction{}
	}
	return liveTUIWaitActionFromWait(workflowID, selected)
}

func waitItemHasResolvableTUIAction(wait map[string]any) bool {
	return waitIDValue(wait) != "" && (waitLooksApprovable(wait) || waitLooksRejectable(wait))
}

func liveTUIWaitActionFromWait(workflowID string, wait map[string]any) liveTUIWaitAction {
	if wait == nil {
		return liveTUIWaitAction{}
	}
	notifyUI := waitNotifyUI(wait)
	approval := mapStringAny(wait["approval"])
	stepID := firstNonBlankString(wait["stepId"], wait["step-id"], wait["step"], wait["currentStep"])
	return liveTUIWaitAction{
		Active:     true,
		WaitID:     waitIDValue(wait),
		WorkflowID: firstNonBlankString(wait["workflowId"], wait["workflow-id"], wait["runId"], wait["run-id"], workflowID),
		StepID:     stepID,
		Title: firstNonBlankString(
			wait["title"],
			notifyUI["title"],
			approval["title"],
			wait["key"],
			stepID,
		),
		Message: firstNonBlankString(
			wait["message"],
			notifyUI["message"],
			approval["message"],
		),
		Actions: waitAvailableActions(wait),
	}
}

func waitLooksActive(wait map[string]any) bool {
	if wait == nil {
		return false
	}
	switch strings.ToLower(firstNonBlankString(wait["status"], wait["state"])) {
	case "completed", "complete", "cancelled", "canceled", "rejected", "expired", "failed":
		return false
	default:
		return true
	}
}

func waitLooksApprovable(wait map[string]any) bool {
	return waitLooksActive(wait) && containsFold(waitAvailableActions(wait), "approve")
}

func waitLooksRejectable(wait map[string]any) bool {
	return waitLooksActive(wait) && containsFold(waitAvailableActions(wait), "reject")
}

func waitAvailableActions(wait map[string]any) []string {
	if wait == nil {
		return nil
	}
	actions := []string{}
	add := func(items ...string) {
		for _, item := range items {
			item = strings.ToLower(strings.TrimSpace(item))
			if item == "" || containsFold(actions, item) {
				continue
			}
			actions = append(actions, item)
		}
	}
	add(stringSlice(wait["actions"])...)
	if approval := mapStringAny(wait["approval"]); approval != nil {
		if approvalActions := stringSlice(approval["actions"]); len(approvalActions) > 0 {
			add(approvalActions...)
		} else {
			add("approve")
		}
	}
	if notifyUI := waitNotifyUI(wait); notifyUI != nil {
		add(stringSlice(notifyUI["actions"])...)
	}
	notify := firstPresent(wait, "notify", "notification")
	encoded := strings.ToLower(fmt.Sprintf("%v", notify))
	if strings.Contains(encoded, "approve") {
		add("approve")
	}
	if strings.Contains(encoded, "reject") {
		add("reject")
	}
	sort.Strings(actions)
	return actions
}

func waitNotifyUI(wait map[string]any) map[string]any {
	notify := mapStringAny(firstPresent(wait, "notify", "notification"))
	if notify == nil {
		return nil
	}
	if ui := mapStringAny(notify["ui"]); ui != nil {
		return ui
	}
	channels := mapStringAny(notify["channels"])
	if channels == nil {
		return nil
	}
	return mapStringAny(channels["ui"])
}
