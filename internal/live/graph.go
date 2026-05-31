package live

import (
	"encoding/json"
	"strings"
	"time"
)

func DecodeFlowGraphDocument(v any) (FlowGraphDocument, error) {
	var doc FlowGraphDocument
	b, err := json.Marshal(v)
	if err != nil {
		return doc, err
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}

func (s Snapshot) WithFlowGraph(doc FlowGraphDocument) Snapshot {
	workflowID := strings.TrimSpace(doc.WorkflowID)
	if workflowID == "" {
		return s
	}
	out := s
	graphs := make([]FlowGraphDocument, 0, len(s.FlowGraphs)+1)
	replaced := false
	for _, existing := range s.FlowGraphs {
		if strings.TrimSpace(existing.WorkflowID) == workflowID {
			graphs = append(graphs, doc)
			replaced = true
			continue
		}
		graphs = append(graphs, existing)
	}
	if !replaced {
		graphs = append(graphs, doc)
	}
	out.FlowGraphs = graphs
	return out
}

func (s Snapshot) WithGraphSkeleton(now time.Time) Snapshot {
	if len(s.FlowGraphs) == 0 || len(s.Runs) == 0 {
		return s
	}
	out := s
	out.Nodes = append([]Activity(nil), s.Nodes...)
	runsByWorkflow := map[string]RunState{}
	for _, run := range s.Runs {
		if workflowID := strings.TrimSpace(run.WorkflowID); workflowID != "" {
			runsByWorkflow[workflowID] = run
		}
	}
	for _, doc := range s.FlowGraphs {
		run, ok := runsByWorkflow[strings.TrimSpace(doc.WorkflowID)]
		if !ok {
			continue
		}
		out.Nodes = mergeGraphSkeletonNodes(out.Nodes, run, doc.Graph, now)
	}
	return out
}

func mergeGraphSkeletonNodes(activities []Activity, run RunState, graph FlowGraph, now time.Time) []Activity {
	graphByStep := map[string]FlowGraphNode{}
	graphNodes := make([]FlowGraphNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		kind := strings.ToLower(strings.TrimSpace(node.Kind))
		if kind == "" || kind == "flow" {
			continue
		}
		graphNodes = append(graphNodes, node)
		if kind == "step" {
			stepID := strings.TrimSpace(node.StepID)
			if stepID == "" {
				continue
			}
			graphByStep[stepID] = node
		}
	}
	if len(graphNodes) == 0 {
		return activities
	}

	present := map[string]bool{}
	for i := range activities {
		activity := &activities[i]
		if strings.TrimSpace(activity.WorkflowID) != strings.TrimSpace(run.WorkflowID) {
			continue
		}
		stepID := strings.TrimSpace(activity.StepID)
		if stepID == "" {
			continue
		}
		if graphNode, ok := graphByStep[stepID]; ok {
			activity.GraphOrder = graphSortOrder(graphNode.Order)
			if strings.TrimSpace(activity.ParentActivityID) == "" {
				activity.ParentActivityID = strings.TrimSpace(graphNode.ParentID)
			}
			activity.GraphScopeID = strings.TrimSpace(graphNode.ScopeID)
			present[stepID] = true
		}
	}
	if !runHasActiveWork(run) && isTerminalStatus(runStatus(run)) {
		return assignRuntimeOnlyGraphOrder(activities, run)
	}

	updatedAt := run.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	for _, graphNode := range graphNodes {
		stepID := strings.TrimSpace(graphNode.StepID)
		if stepID != "" && present[stepID] {
			continue
		}
		activities = append(activities, plannedGraphActivity(run, graphNode, updatedAt))
	}
	return assignRuntimeOnlyGraphOrder(activities, run)
}

func plannedGraphActivity(run RunState, graphNode FlowGraphNode, updatedAt time.Time) Activity {
	kind := strings.ToLower(strings.TrimSpace(graphNode.Kind))
	stepID := strings.TrimSpace(graphNode.StepID)
	activityID := firstNonBlank(graphNode.ID, stepID, kind)
	activityKind := kind
	activityType := graphNode.StepType
	switch kind {
	case "step":
		activityKind = "step"
	case "loop":
		activityKind = "loop"
		activityType = firstNonBlank(graphNode.LoopType, "loop")
	case "branch":
		activityKind = "branch"
		activityType = firstNonBlank(graphNode.BranchType, "branch")
	case "call-flow":
		activityKind = "call-flow"
		activityType = firstNonBlank(graphNode.FlowSlug, "call-flow")
	case "agent":
		activityKind = "agent"
		activityType = firstNonBlank(graphNode.AgentID, "agent")
	case "dynamic":
		activityKind = "dynamic"
		activityType = "dynamic"
	}
	return Activity{
		WorkspaceID:      run.WorkspaceID,
		WorkflowID:       run.WorkflowID,
		RootWorkflowID:   firstNonBlank(run.RootWorkflowID, run.WorkflowID),
		ActivityID:       activityID,
		ActivityKind:     activityKind,
		ActivityType:     activityType,
		ActivityName:     firstNonBlank(graphNode.Label, stepID, graphNode.FlowSlug, activityID),
		Status:           "pending",
		Active:           false,
		StepID:           stepID,
		ParentActivityID: strings.TrimSpace(graphNode.ParentID),
		UpdatedAt:        updatedAt,
		Planned:          true,
		GraphOrder:       graphSortOrder(graphNode.Order),
		GraphScopeID:     strings.TrimSpace(graphNode.ScopeID),
	}
}

func graphSortOrder(order int) int {
	if order <= 0 {
		return 0
	}
	return order * 100
}

func assignRuntimeOnlyGraphOrder(activities []Activity, run RunState) []Activity {
	workflowID := strings.TrimSpace(run.WorkflowID)
	if workflowID == "" {
		return activities
	}
	type orderedRuntimeActivity struct {
		order int
		t     time.Time
	}
	ordered := make([]orderedRuntimeActivity, 0)
	orderByRef := map[string]int{}
	for _, activity := range activities {
		if strings.TrimSpace(activity.WorkflowID) != workflowID || activity.GraphOrder <= 0 {
			continue
		}
		registerGraphOrderRefs(orderByRef, activity)
		t := activityTime(activity)
		if t.IsZero() {
			continue
		}
		ordered = append(ordered, orderedRuntimeActivity{order: activity.GraphOrder, t: t})
	}
	if len(ordered) == 0 && len(orderByRef) == 0 {
		return activities
	}
	out := append([]Activity(nil), activities...)
	for i := range out {
		activity := &out[i]
		if strings.TrimSpace(activity.WorkflowID) != workflowID || activity.GraphOrder > 0 || activity.Planned {
			continue
		}
		if !isStepLikeActivity(*activity) || isToolActivity(*activity) {
			continue
		}
		if parentOrder := graphOrderForRuntimeParent(out, *activity, orderByRef); parentOrder > 0 {
			activity.GraphOrder = parentOrder + 50
			continue
		}
		t := activityTime(*activity)
		if t.IsZero() {
			continue
		}
		best := 0
		for _, candidate := range ordered {
			if candidate.t.After(t) {
				continue
			}
			if candidate.order > best {
				best = candidate.order
			}
		}
		if best == 0 {
			activity.GraphOrder = 50
		} else {
			activity.GraphOrder = best + 50
		}
	}
	return out
}

func registerGraphOrderRefs(orderByRef map[string]int, activity Activity) {
	for _, ref := range []string{
		strings.TrimSpace(activity.ActivityID),
		strings.TrimSpace(activity.StepID),
		strings.TrimSpace(activity.ToolCallID),
	} {
		if ref == "" {
			continue
		}
		if existing := orderByRef[ref]; existing == 0 || activity.GraphOrder < existing {
			orderByRef[ref] = activity.GraphOrder
		}
	}
}

func graphOrderForRuntimeParent(activities []Activity, activity Activity, orderByRef map[string]int) int {
	for _, parentRef := range []string{
		strings.TrimSpace(activity.ParentActivityID),
		strings.TrimSpace(activity.ParentStepID),
	} {
		if order := graphOrderForParentRef(activities, parentRef, orderByRef, map[string]bool{}); order > 0 {
			return order
		}
	}
	return 0
}

func graphOrderForParentRef(activities []Activity, parentRef string, orderByRef map[string]int, seen map[string]bool) int {
	parentRef = strings.TrimSpace(parentRef)
	if parentRef == "" || seen[parentRef] {
		return 0
	}
	seen[parentRef] = true
	for ref, order := range orderByRef {
		if parentRefsEquivalent(ref, parentRef) {
			return order
		}
	}
	for _, candidate := range activities {
		if !activityMatchesParentRef(candidate, parentRef) {
			continue
		}
		if candidate.GraphOrder > 0 {
			return candidate.GraphOrder
		}
		if order := graphOrderForParentRef(activities, candidate.ParentActivityID, orderByRef, seen); order > 0 {
			return order
		}
		if order := graphOrderForParentRef(activities, candidate.ParentStepID, orderByRef, seen); order > 0 {
			return order
		}
	}
	return 0
}
