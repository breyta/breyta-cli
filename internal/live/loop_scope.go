package live

import (
	"strconv"
	"strings"
	"time"
)

const loopPagePrefix = "loop-page-"

// collapseLoopIterationScope keeps only the current loop iteration's body steps visible.
// Earlier iteration scopes are dropped once a later iteration starts.
func collapseLoopIterationScope(activities []Activity, run RunState) []Activity {
	if len(activities) == 0 || !hasLoopPageActivities(activities) {
		return activities
	}
	current := detectCurrentLoopIteration(activities, run)
	if current <= 0 {
		return activities
	}

	out := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		if activityInLoopIteration(activity, activities, current) {
			out = append(out, activity)
		}
	}
	return out
}

func hasLoopPageActivities(activities []Activity) bool {
	for _, activity := range activities {
		if _, ok := loopPageIteration(activity); ok {
			return true
		}
	}
	return false
}

func detectCurrentLoopIteration(activities []Activity, run RunState) int {
	current := 0

	for _, activity := range activities {
		if n, ok := loopPageIteration(activity); ok && activity.Active && n > current {
			current = n
		}
	}

	for _, activity := range activities {
		if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "loop") {
			continue
		}
		if activity.ProgressCurrent == nil || !activity.Active {
			continue
		}
		if n := int(*activity.ProgressCurrent); n > current {
			current = n
		}
	}

	if stepID := strings.TrimSpace(run.CurrentStepID); stepID != "" {
		if n, ok := loopPageIterationFromID(stepID); ok && n > current {
			current = n
		}
	}

	if current > 0 {
		return current
	}

	for _, activity := range activities {
		if n, ok := loopPageIteration(activity); ok && n > current {
			current = n
		}
	}

	if current > 0 {
		return current
	}

	return inferLoopIterationFromActiveBody(activities)
}

func inferLoopIterationFromActiveBody(activities []Activity) int {
	lastCompletedPage := 0
	var lastCompletedAt *time.Time
	for _, activity := range activities {
		n, ok := loopPageIteration(activity)
		if !ok || activity.Active {
			continue
		}
		if !isTerminalStatus(normalizeStatus(activity.Status, activity.Active)) {
			continue
		}
		if n > lastCompletedPage {
			lastCompletedPage = n
			lastCompletedAt = activity.CompletedAt
		} else if n == lastCompletedPage && activity.CompletedAt != nil {
			if lastCompletedAt == nil || activity.CompletedAt.After(*lastCompletedAt) {
				lastCompletedAt = activity.CompletedAt
			}
		}
	}
	if lastCompletedPage == 0 {
		return 0
	}
	for _, activity := range activities {
		if activity.Active && !isLoopPageActivity(activity) {
			if lastCompletedAt == nil || !activityTime(activity).Before(*lastCompletedAt) {
				return lastCompletedPage + 1
			}
		}
	}
	return lastCompletedPage
}

func activityInLoopIteration(activity Activity, activities []Activity, iteration int) bool {
	if n, ok := loopPageIteration(activity); ok {
		return n == iteration
	}
	if isLoopPageActivity(activity) {
		return false
	}
	if !isLoopBodyCandidate(activity) {
		return true
	}
	start := loopIterationStart(activities, iteration)
	if start == nil {
		return iteration == 1
	}
	at := activityTime(activity)
	if at.Before(*start) {
		return false
	}
	if end := explicitLoopIterationStart(activities, iteration+1); end != nil && !at.Before(*end) {
		return false
	}
	return true
}

func isLoopBodyCandidate(activity Activity) bool {
	if !isStepLikeActivity(activity) {
		return false
	}
	if isLoopPageActivity(activity) {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	return kind == "step" || kind == "tool_call" || kind == "mcp_tool_call"
}

func isLoopPageActivity(activity Activity) bool {
	_, ok := loopPageIteration(activity)
	return ok
}

func loopPageIteration(activity Activity) (iteration int, ok bool) {
	for _, id := range []string{activity.StepID, activity.ActivityName, activity.ActivityID} {
		if iteration, ok := loopPageIterationFromID(id); ok {
			return iteration, true
		}
	}
	return 0, false
}

func loopPageIterationFromID(id string) (iteration int, ok bool) {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, loopPagePrefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, loopPagePrefix))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func loopIterationStart(activities []Activity, iteration int) *time.Time {
	if iteration <= 0 {
		return nil
	}
	if start := explicitLoopIterationStart(activities, iteration); start != nil {
		return start
	}
	if iteration <= 1 {
		return nil
	}
	var start *time.Time
	for _, activity := range activities {
		n, ok := loopPageIteration(activity)
		if !ok || n != iteration-1 {
			continue
		}
		if activity.CompletedAt != nil && !activity.CompletedAt.IsZero() {
			if start == nil || activity.CompletedAt.After(*start) {
				start = activity.CompletedAt
			}
		}
	}
	return start
}

func explicitLoopIterationStart(activities []Activity, iteration int) *time.Time {
	if iteration <= 0 {
		return nil
	}
	var start *time.Time
	for _, activity := range activities {
		n, ok := loopPageIteration(activity)
		if !ok || n != iteration {
			continue
		}
		if activity.StartedAt != nil && !activity.StartedAt.IsZero() {
			if start == nil || activity.StartedAt.Before(*start) {
				start = activity.StartedAt
			}
		}
	}
	return start
}

func suppressDuplicateStepActivities(activities []Activity) []Activity {
	activeByStep := map[string]bool{}
	recordedByStep := map[string]bool{}
	for _, activity := range activities {
		stepID := strings.TrimSpace(activity.StepID)
		if stepID == "" {
			continue
		}
		key := activity.WorkflowID + "\x00" + stepID
		if activity.Active {
			activeByStep[key] = true
		}
		if !activity.Planned {
			recordedByStep[key] = true
		}
	}

	out := make([]Activity, 0, len(activities))
	seenCompleted := map[string]bool{}
	for _, activity := range activities {
		stepID := strings.TrimSpace(activity.StepID)
		if stepID == "" {
			out = append(out, activity)
			continue
		}
		key := activity.WorkflowID + "\x00" + stepID
		if activity.Planned && recordedByStep[key] {
			continue
		}
		if activity.Active {
			out = append(out, activity)
			continue
		}
		if activeByStep[key] {
			continue
		}
		if seenCompleted[key] {
			continue
		}
		seenCompleted[key] = true
		out = append(out, activity)
	}
	return out
}
