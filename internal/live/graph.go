package live

import (
	"encoding/json"
	"fmt"
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
	if len(s.Runs) == 0 {
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
	if len(s.FlowGraphs) > 0 {
		for _, doc := range s.FlowGraphs {
			run, ok := runsByWorkflow[strings.TrimSpace(doc.WorkflowID)]
			if !ok {
				continue
			}
			out.Nodes = mergeGraphSkeletonNodes(out.Nodes, run, doc.Graph, now)
		}
	}
	for _, run := range out.Runs {
		out.Nodes = mergeRunResultResource(out.Nodes, run, now)
	}
	return out
}

func mergeGraphSkeletonNodes(activities []Activity, run RunState, graph FlowGraph, now time.Time) []Activity {
	graphByRef := map[string]FlowGraphNode{}
	graphNodes := make([]FlowGraphNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		kind := strings.ToLower(strings.TrimSpace(node.Kind))
		if kind == "" || kind == "flow" {
			continue
		}
		graphNodes = append(graphNodes, node)
		for _, ref := range []string{strings.TrimSpace(node.ID), strings.TrimSpace(node.StepID)} {
			if ref == "" {
				continue
			}
			graphByRef[ref] = node
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
		if graphNode, ok := graphNodeForActivity(graphByRef, *activity); ok {
			activity.GraphOrder = graphSortOrder(graphNode.Order)
			if strings.TrimSpace(activity.ParentActivityID) == "" {
				activity.ParentActivityID = strings.TrimSpace(graphNode.ParentID)
			}
			activity.GraphScopeID = strings.TrimSpace(graphNode.ScopeID)
			for _, ref := range []string{strings.TrimSpace(graphNode.ID), strings.TrimSpace(graphNode.StepID)} {
				if ref != "" {
					present[ref] = true
				}
			}
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
		if graphNodePresent(present, graphNode) {
			continue
		}
		activities = append(activities, plannedGraphActivity(run, graphNode, updatedAt))
	}
	activities = mergePlannedPersistResources(activities, run, graphNodes, updatedAt)
	return assignRuntimeOnlyGraphOrder(activities, run)
}

func graphNodeForActivity(graphByRef map[string]FlowGraphNode, activity Activity) (FlowGraphNode, bool) {
	for _, ref := range []string{strings.TrimSpace(activity.StepID), strings.TrimSpace(activity.ActivityID)} {
		if ref == "" {
			continue
		}
		if node, ok := graphByRef[ref]; ok {
			return node, true
		}
		if idx := strings.LastIndexAny(ref, ":/"); idx >= 0 && idx+1 < len(ref) {
			if node, ok := graphByRef[ref[idx+1:]]; ok {
				return node, true
			}
		}
	}
	return FlowGraphNode{}, false
}

func graphNodePresent(present map[string]bool, graphNode FlowGraphNode) bool {
	for _, ref := range []string{strings.TrimSpace(graphNode.ID), strings.TrimSpace(graphNode.StepID)} {
		if ref != "" && present[ref] {
			return true
		}
	}
	return false
}

func mergePlannedPersistResources(activities []Activity, run RunState, graphNodes []FlowGraphNode, updatedAt time.Time) []Activity {
	if len(graphNodes) == 0 {
		return activities
	}
	actualResourceParents := map[string]bool{}
	plannedResourceIDs := map[string]bool{}
	for _, activity := range activities {
		if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "resource") {
			continue
		}
		if activity.Planned {
			if id := strings.TrimSpace(activity.ActivityID); id != "" {
				plannedResourceIDs[id] = true
			}
			continue
		}
		if parentID := strings.TrimSpace(activity.ParentActivityID); parentID != "" {
			actualResourceParents[parentID] = true
			actualResourceParents[strings.TrimPrefix(parentID, "step:")] = true
		}
	}
	for _, graphNode := range graphNodes {
		if !graphNode.HasPersist {
			continue
		}
		parentID := firstNonBlank(strings.TrimSpace(graphNode.StepID), strings.TrimSpace(graphNode.ID))
		if parentID == "" || actualResourceParents[parentID] || actualResourceParents[strings.TrimSpace(graphNode.ID)] {
			continue
		}
		resource := plannedPersistResourceActivity(run, graphNode, parentID, updatedAt)
		if plannedResourceIDs[resource.ActivityID] {
			continue
		}
		plannedResourceIDs[resource.ActivityID] = true
		activities = append(activities, resource)
	}
	return activities
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

func plannedPersistResourceActivity(run RunState, graphNode FlowGraphNode, parentID string, updatedAt time.Time) Activity {
	resourceKind := firstNonBlank(strings.TrimSpace(graphNode.PersistKind), strings.TrimSpace(graphNode.PersistType), "resource")
	return Activity{
		WorkspaceID:      run.WorkspaceID,
		WorkflowID:       run.WorkflowID,
		RootWorkflowID:   firstNonBlank(run.RootWorkflowID, run.WorkflowID),
		ActivityID:       firstNonBlank(strings.TrimSpace(graphNode.ID), parentID) + ":resource",
		ParentActivityID: parentID,
		ActivityKind:     "resource",
		ActivityType:     resourceKind,
		ActivityName:     "output resource",
		Status:           "pending",
		Active:           false,
		ResourceKind:     resourceKind,
		ResourceLabel:    "output resource",
		ContentType:      strings.TrimSpace(graphNode.PersistMIME),
		UpdatedAt:        updatedAt,
		Planned:          true,
		GraphOrder:       graphSortOrder(graphNode.Order) + 1,
		GraphScopeID:     strings.TrimSpace(graphNode.ScopeID),
	}
}

func mergeRunResultResource(activities []Activity, run RunState, now time.Time) []Activity {
	if !shouldModelRunResultResource(run) || hasRunResultResource(activities, run) {
		return activities
	}
	status := runStatus(run)
	terminal := !runHasActiveWork(run) && isTerminalStatus(status)
	updatedAt := run.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	if run.CompletedAt != nil && !run.CompletedAt.IsZero() {
		updatedAt = *run.CompletedAt
	}
	activities = append(activities, runResultResourceActivity(run, status, terminal, updatedAt))
	return activities
}

func shouldModelRunResultResource(run RunState) bool {
	if strings.TrimSpace(run.WorkflowID) == "" {
		return false
	}
	if strings.TrimSpace(run.AgentID) != "" {
		return false
	}
	entrypoint := strings.ToLower(strings.TrimSpace(run.EntrypointType))
	return entrypoint != "agent" && entrypoint != "tool" && entrypoint != "mcp"
}

func hasRunResultResource(activities []Activity, run RunState) bool {
	for _, activity := range activities {
		if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "resource") {
			continue
		}
		if isRunResultResourceForRun(activity, run) {
			return true
		}
	}
	return false
}

func runResultResourceActivity(run RunState, status string, terminal bool, updatedAt time.Time) Activity {
	workflowID := strings.TrimSpace(run.WorkflowID)
	resourceStatus := "pending"
	resourceLabel := "flow result"
	resourceURI := ""
	resourceID := runResultResourceParentID(workflowID) + ":result"
	planned := true
	if terminal {
		resourceStatus = "completed"
		planned = false
		kind := "flow-output"
		if isProblemStatus(status) {
			kind = "flow-error"
			resourceLabel = "flow error"
		}
		resourceURI = runResultResourceURI(run, kind)
		resourceID = firstNonBlank(resourceURI, runResultResourceParentID(workflowID)+":"+kind)
	}
	return Activity{
		WorkspaceID:      run.WorkspaceID,
		WorkflowID:       workflowID,
		RootWorkflowID:   firstNonBlank(run.RootWorkflowID, workflowID),
		ActivityID:       resourceID,
		ParentActivityID: runResultResourceParentID(workflowID),
		ActivityKind:     "resource",
		ActivityType:     "run-result",
		ActivityName:     resourceLabel,
		Status:           resourceStatus,
		Active:           false,
		ResourceURI:      resourceURI,
		ResourceKind:     "run-result",
		ResourceLabel:    resourceLabel,
		UpdatedAt:        updatedAt,
		Planned:          planned,
		GraphOrder:       1 << 29,
	}
}

func runResultResourceParentID(workflowID string) string {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return ""
	}
	return "run:" + workflowID
}

func runResultResourceURI(run RunState, kind string) string {
	workspaceID := strings.TrimSpace(run.WorkspaceID)
	workflowID := strings.TrimSpace(run.WorkflowID)
	kind = strings.TrimSpace(kind)
	if workspaceID == "" || workflowID == "" || kind == "" {
		return ""
	}
	return fmt.Sprintf("res://v1/ws/%s/result/run/%s/%s", workspaceID, workflowID, kind)
}

func isRunResultResourceForRun(activity Activity, run RunState) bool {
	workflowID := strings.TrimSpace(run.WorkflowID)
	if workflowID == "" || strings.TrimSpace(activity.WorkflowID) != workflowID {
		return false
	}
	parentID := strings.TrimSpace(activity.ParentActivityID)
	if parentID == runResultResourceParentID(workflowID) || parentID == workflowID {
		return true
	}
	return runResultResourceURIMatches(activity.ResourceURI, workflowID) ||
		runResultResourceURIMatches(activity.ActivityID, workflowID)
}

func runResultResourceURIMatches(value string, workflowID string) bool {
	value = strings.TrimSpace(value)
	workflowID = strings.TrimSpace(workflowID)
	if value == "" || workflowID == "" {
		return false
	}
	return strings.Contains(value, "/result/run/"+workflowID+"/flow-output") ||
		strings.Contains(value, "/result/run/"+workflowID+"/flow-error")
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
