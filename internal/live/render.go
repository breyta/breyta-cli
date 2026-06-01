package live

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RenderOptions struct {
	Now                 time.Time
	Frame               int
	Color               bool
	FocusWorkflowID     string
	MaxActivitiesPerRun int
	DetailMode          DetailMode
	// OmitCompletedSteps hides finished step/tool lines from the live tail so they
	// only appear in committed CLI output above the redrawn tail.
	OmitCompletedSteps bool
	// FullTree disables live scope collapsing so loop iterations, closed child
	// runs, terminal branch details, and the separate root run line stay visible.
	// Step/tool deduplication still runs so nested tools stay attached to the
	// visible step row instead of splitting across duplicate status entries.
	FullTree bool
	// Diagnostics receives suppressed graph/runtime anomalies when callers opt in
	// to live renderer debugging.
	Diagnostics func(RenderDiagnostic)
}

type DetailMode string

const (
	DetailModeDetailed DetailMode = ""
	DetailModePublic   DetailMode = "public"
)

type RenderDiagnostic struct {
	Code       string `json:"code"`
	Message    string `json:"message,omitempty"`
	WorkflowID string `json:"workflowId,omitempty"`
	ActivityID string `json:"activityId,omitempty"`
	StepID     string `json:"stepId,omitempty"`
	ParentRef  string `json:"parentRef,omitempty"`
	ScopeID    string `json:"scopeId,omitempty"`
}

func (opts RenderOptions) diagnose(diagnostic RenderDiagnostic) {
	if opts.Diagnostics == nil || strings.TrimSpace(diagnostic.Code) == "" {
		return
	}
	opts.Diagnostics(diagnostic)
}

func RenderSnapshot(snapshot Snapshot, opts RenderOptions) string {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.MaxActivitiesPerRun <= 0 {
		opts.MaxActivitiesPerRun = 4
	}
	if strings.TrimSpace(opts.FocusWorkflowID) != "" {
		snapshot = snapshot.Focus(opts.FocusWorkflowID)
	}
	snapshot = snapshot.WithGraphSkeleton(opts.Now)

	activeWork := snapshotHasActiveWork(snapshot) || len(snapshot.Runs) == 0
	headerStatus := "completed"
	if activeWork {
		headerStatus = "running"
	}

	var b strings.Builder
	nodes := snapshot.BuildRunTree()
	flatRoot := flatRootRunNode(nodes, opts)
	writeSnapshotHeader(&b, snapshot, opts, activeWork, headerStatus, flatRoot)

	if len(nodes) == 0 {
		b.WriteString(dim("Waiting for run updates...", opts.Color))
		b.WriteByte('\n')
		return b.String()
	}
	renderNodes := nodes
	if root := focusRootRunNode(nodes, opts); root != nil {
		renderNodes = []RunNode{*root}
	}
	for i, node := range renderNodes {
		skipRunLine := flatRoot != nil && strings.TrimSpace(node.Run.WorkflowID) == strings.TrimSpace(flatRoot.Run.WorkflowID)
		rootFlowSlug := strings.TrimSpace(node.Run.FlowSlug)
		renderRunNode(&b, node, "", i == len(renderNodes)-1, opts, skipRunLine, rootFlowSlug)
	}
	if !snapshotHasActiveWork(snapshot) {
		if summary := runSummaryStrip(snapshot, opts); summary != "" {
			b.WriteString(summary)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func snapshotHasActiveWork(snapshot Snapshot) bool {
	if snapshot.Workspace.ActiveRunCount > 0 || snapshot.Workspace.StepsRunning > 0 {
		return true
	}
	for _, run := range snapshot.Runs {
		status := runStatus(run)
		if run.Active || run.StepsRunning > 0 || status == "running" || status == "syncing" {
			return true
		}
	}
	for _, activity := range snapshot.Nodes {
		status := normalizeStatus(activity.Status, activity.Active)
		if activity.Active || status == "running" || status == "syncing" {
			return true
		}
	}
	return false
}

func runHasActiveWork(run RunState) bool {
	status := runStatus(run)
	return run.Active || run.StepsRunning > 0 || status == "running" || status == "syncing"
}

func (snapshot Snapshot) HasActiveWork() bool {
	return snapshotHasActiveWork(snapshot)
}

func renderRunNode(b *strings.Builder, node RunNode, prefix string, last bool, opts RenderOptions, skipRunLine bool, rootFlowSlug string) {
	detailed := opts.DetailMode != DetailModePublic
	childPrefix := branchChildPrefix(prefix, last)
	linePrefix := prefix
	if linePrefix == "" {
		linePrefix = " "
	}

	run := node.Run
	status := runStatus(run)
	if !skipRunLine {
		label := runLabelStyled(run, node.Relation, rootFlowSlug, opts.Color)
		line := fmt.Sprintf("%s%s", linePrefix, coloredGlyph(status, run.Active, opts.Frame, opts.Color))
		line += " " + label
		if detailed && node.Relation != nil {
			includeKind := !strings.HasPrefix(label, "agent ") && !strings.HasPrefix(label, "child flow ")
			line += " " + dim(relationLabel(*node.Relation, includeKind), opts.Color)
		}
		if statusText := compactStatusText(status); statusText != "" {
			line += " " + statusBadge(statusText, opts.Color)
		}
		if node.MissingRun {
			line += " " + dim("waiting", opts.Color)
		}
		if duration := runNodeDurationText(node, opts.Now); duration != "" {
			line += " " + dim(duration, opts.Color)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}

	if !detailed {
		return
	}

	activityPrefix := childPrefix
	if skipRunLine {
		activityPrefix = linePrefix
	}
	if shouldCollapseRunDetails(node, status, opts) {
		return
	}

	renderedCurrentFallback := false
	if runHasActiveWork(run) {
		current := currentStepText(run, node.Activities, opts.Now)
		if current != "" && !hasVisibleCurrentStepActivity(node) {
			activeTool := activeToolText(node, opts.Now)
			if activeTool == "" || !sameActiveTool(current, activeTool) {
				renderCurrentStepFallback(b, current, activityPrefix, opts)
				renderedCurrentFallback = true
			}
		}
	}

	selectedChildren := selectedChildRuns(node.Children, run.CurrentStepID, opts)
	visibleNodes := selectedActivities(node, selectedChildren, opts)
	runResources := resourcesForRun(node.Activities, run)
	resourcesByParent := groupResourcesByVisibleParent(node.Activities, visibleNodes)
	toolsByParent := groupToolsByVisibleParent(node.Activities, visibleNodes)
	childrenByStep, remainingChildren := groupChildrenByVisibleStep(selectedChildren, visibleNodes, opts)
	nestedActivitiesByParent, nestedActivityKeys := groupNestedActivitiesByRenderedTools(visibleNodes, toolsByParent, opts)
	graphActivitiesByParent, graphActivityKeys := groupGraphActivitiesByVisibleParent(visibleNodes, childrenByStep, nestedActivityKeys, opts)
	nestedActivitiesByParent = mergeActivityGroups(nestedActivitiesByParent, graphActivitiesByParent)
	nestedActivityKeys = mergeActivityKeySets(nestedActivityKeys, graphActivityKeys)
	visibleNodes = filterNestedActivitiesFromTopLevel(visibleNodes, nestedActivityKeys)
	for i, activity := range visibleNodes {
		activityChildren := childrenForStep(childrenByStep, activity)
		activityResources := resourcesForActivity(resourcesByParent, activity)
		activityTools := toolsForActivity(toolsByParent, activity)
		activityNested := nestedActivitiesForActivity(nestedActivitiesByParent, activity)
		displayActivity := activityWithNestedActivityDuration(activity, activityNested)
		displayActivity = activityWithFanoutChildDuration(displayActivity, activityChildren)
		isLastActivity := i == len(visibleNodes)-1 && len(remainingChildren) == 0 && !renderedCurrentFallback
		if isStructuralFanoutWrapper(displayActivity) {
			for _, tool := range activityTools {
				renderActivityBranch(b, tool, activityPrefix, opts, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, nested := range activityNested {
				renderActivityBranch(b, nested, activityPrefix, opts, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, resource := range activityResources {
				renderResource(b, resource, activityPrefix, opts)
			}
			for childIdx, child := range activityChildren {
				renderRunNode(b, child, activityPrefix, childIdx == len(activityChildren)-1, opts, false, rootFlowSlug)
			}
			continue
		}
		if shouldElideAgentFanoutEntrypoint(node, displayActivity, activityTools, activityResources, activityChildren) {
			for _, tool := range activityTools {
				renderActivityBranch(b, suppressRedundantAgentLabel(node, tool), activityPrefix, opts, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, nested := range activityNested {
				renderActivityBranch(b, nested, activityPrefix, opts, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, resource := range activityResources {
				renderResource(b, suppressRedundantAgentLabel(node, resource), activityPrefix, opts)
			}
			for childIdx, child := range activityChildren {
				renderRunNode(b, child, activityPrefix, childIdx == len(activityChildren)-1, opts, false, rootFlowSlug)
			}
			continue
		}
		if shouldInlineActivityContainer(displayActivity, run, activityResources, activityTools, activityChildren) {
			for _, tool := range activityTools {
				renderActivityBranch(b, tool, activityPrefix, opts, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, nested := range activityNested {
				renderActivityBranch(b, nested, activityPrefix, opts, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, resource := range activityResources {
				renderResource(b, resource, activityPrefix, opts)
			}
			for childIdx, child := range activityChildren {
				renderRunNode(b, child, activityPrefix, childIdx == len(activityChildren)-1, opts, false, rootFlowSlug)
			}
			continue
		}
		renderActivity(b, displayActivity, activityPrefix, isLastActivity, opts)
		activityChildPrefix := branchChildPrefix(activityPrefix, isLastActivity)
		for _, tool := range activityTools {
			renderActivityBranch(b, suppressRedundantAgentLabel(node, tool), activityChildPrefix, opts, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		}
		for _, nested := range activityNested {
			renderActivityBranch(b, nested, activityChildPrefix, opts, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		}
		for _, resource := range activityResources {
			renderResource(b, suppressRedundantAgentLabel(node, resource), activityChildPrefix, opts)
		}
		for childIdx, child := range activityChildren {
			renderRunNode(b, child, activityChildPrefix, childIdx == len(activityChildren)-1, opts, false, rootFlowSlug)
		}
	}

	for i, child := range remainingChildren {
		renderRunNode(b, child, childPrefix, i == len(remainingChildren)-1, opts, false, rootFlowSlug)
	}
	for _, resource := range runResources {
		renderResource(b, resource, activityPrefix, opts)
	}
}

func nonResourceActivities(activities []Activity) []Activity {
	out := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		if strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "resource") {
			continue
		}
		out = append(out, activity)
	}
	return out
}

func shouldCollapseRunDetails(node RunNode, status string, opts RenderOptions) bool {
	if opts.FullTree {
		return false
	}
	if node.Relation == nil {
		return false
	}
	return !node.Run.Active && isTerminalStatus(status)
}

func selectedChildRuns(children []RunNode, _ string, opts RenderOptions) []RunNode {
	if opts.FullTree {
		return children
	}
	selected := make([]RunNode, 0, len(children))
	for _, child := range children {
		if child.MissingRun {
			selected = append(selected, child)
			continue
		}
		if runHasActiveWork(child.Run) || child.Run.Active {
			selected = append(selected, child)
		}
	}
	return selected
}

func groupResourcesByVisibleParent(activities []Activity, visible []Activity) map[string][]Activity {
	grouped := map[string][]Activity{}
	for _, activity := range activities {
		if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "resource") {
			continue
		}
		parentRef := strings.TrimSpace(activity.ParentActivityID)
		if parentRef == "" {
			continue
		}
		for _, parent := range visible {
			if !activityMatchesParentRef(parent, parentRef) {
				continue
			}
			registerGroupedActivity(grouped, parent, activity)
			break
		}
	}
	for parentID := range grouped {
		grouped[parentID] = sortActivitiesByTime(dedupeResourceActivities(grouped[parentID]))
	}
	return grouped
}

func resourcesForActivity(resourcesByParent map[string][]Activity, activity Activity) []Activity {
	resources := make([]Activity, 0, len(resourcesByParent[activity.ActivityID])+len(resourcesByParent[activity.StepID]))
	seen := map[string]bool{}
	for _, parentID := range []string{activity.ActivityID, activity.StepID} {
		for _, resource := range resourcesByParent[parentID] {
			key := resourceIdentityKey(resource)
			if seen[key] {
				continue
			}
			seen[key] = true
			resources = append(resources, resource)
		}
	}
	return resources
}

func resourcesForRun(activities []Activity, run RunState) []Activity {
	resources := make([]Activity, 0)
	for _, activity := range activities {
		if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "resource") {
			continue
		}
		if !isRunResultResourceForRun(activity, run) {
			continue
		}
		resources = append(resources, activity)
	}
	return sortActivitiesByTime(dedupeResourceActivities(resources))
}

func groupToolsByVisibleParent(activities []Activity, visible []Activity) map[string][]Activity {
	grouped := map[string][]Activity{}
	for _, activity := range activities {
		if !isToolActivity(activity) {
			continue
		}
		parentRef := toolParentRef(activity)
		if parentRef == "" {
			continue
		}
		parent := findActivityByStepRef(activities, parentRef)
		if parent != nil {
			if !visibleContainsStepRef(visible, *parent) {
				continue
			}
			registerGroupedTool(grouped, *parent, activity)
			continue
		}
		for _, candidate := range visible {
			if !activityMatchesParentID(candidate, parentRef) {
				continue
			}
			registerGroupedTool(grouped, candidate, activity)
			break
		}
	}
	for parentID := range grouped {
		grouped[parentID] = sortActivitiesByTime(dedupeActivities(grouped[parentID]))
	}
	return grouped
}

func visibleContainsStepRef(visible []Activity, parent Activity) bool {
	for _, candidate := range visible {
		if candidate.WorkflowID == parent.WorkflowID &&
			(strings.TrimSpace(candidate.ActivityID) == strings.TrimSpace(parent.ActivityID) ||
				strings.TrimSpace(candidate.StepID) == strings.TrimSpace(parent.StepID)) {
			return true
		}
	}
	return false
}

func registerGroupedTool(grouped map[string][]Activity, parent Activity, tool Activity) {
	registerGroupedActivity(grouped, parent, tool)
}

func registerGroupedActivity(grouped map[string][]Activity, parent Activity, activity Activity) {
	if id := strings.TrimSpace(parent.ActivityID); id != "" {
		grouped[id] = append(grouped[id], activity)
	}
	if stepID := strings.TrimSpace(parent.StepID); stepID != "" {
		grouped[stepID] = append(grouped[stepID], activity)
	}
	if toolCallID := strings.TrimSpace(parent.ToolCallID); toolCallID != "" {
		grouped[toolCallID] = append(grouped[toolCallID], activity)
	}
}

func toolsForActivity(toolsByParent map[string][]Activity, activity Activity) []Activity {
	return childActivitiesForParent(toolsByParent, activity)
}

func nestedActivitiesForActivity(nestedByParent map[string][]Activity, activity Activity) []Activity {
	return childActivitiesForParent(nestedByParent, activity)
}

func childActivitiesForParent(byParent map[string][]Activity, activity Activity) []Activity {
	children := make([]Activity, 0, len(byParent[activity.ActivityID])+len(byParent[activity.StepID])+len(byParent[activity.ToolCallID]))
	seen := map[string]bool{}
	for _, parentID := range []string{activity.ActivityID, activity.StepID, activity.ToolCallID} {
		for _, child := range byParent[parentID] {
			key := firstNonBlank(child.ActivityID, child.ToolCallID, child.StepID, child.ActivityName)
			if key == "" {
				key = fmt.Sprintf("%s/%s/%s", child.WorkflowID, child.ParentActivityID, child.ActivityName)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			children = append(children, child)
		}
	}
	return children
}

func groupNestedActivitiesByRenderedTools(visible []Activity, toolsByParent map[string][]Activity, opts RenderOptions) (map[string][]Activity, map[string]bool) {
	renderedTools := make([]Activity, 0)
	seenTools := map[string]bool{}
	for _, tools := range toolsByParent {
		for _, tool := range tools {
			key := activityIdentityKey(tool)
			if key == "" || seenTools[key] {
				continue
			}
			seenTools[key] = true
			renderedTools = append(renderedTools, tool)
		}
	}
	if len(renderedTools) == 0 {
		return nil, nil
	}

	grouped := map[string][]Activity{}
	nestedKeys := map[string]bool{}
	for _, activity := range visible {
		if isToolActivity(activity) {
			continue
		}
		parentRef := toolParentRef(activity)
		nested := false
		if isStructuralFanoutWrapper(activity) {
			if named := namedFanoutForParent(visible, parentRef); named != nil {
				if key := activityIdentityKey(activity); key != "" {
					nestedKeys[key] = true
				}
				if key := activityIdentityKey(*named); key != "" {
					nestedKeys[key] = true
				}
				activity = *named
			}
		}
		if isStructuralFanoutWrapper(activity) && hasNamedFanoutForParent(visible, parentRef) {
			if key := activityIdentityKey(activity); key != "" {
				nestedKeys[key] = true
			}
			continue
		}
		if parentRef != "" {
			for _, tool := range renderedTools {
				if !activityMatchesParentRef(tool, parentRef) {
					continue
				}
				registerGroupedActivity(grouped, tool, activity)
				if key := activityIdentityKey(activity); key != "" {
					nestedKeys[key] = true
				}
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		if inferredTool, ok := inferredSemanticFanoutToolParent(visible, renderedTools, activity); ok {
			inferredActivity := activity
			inferredActivity.ParentActivityID = strings.TrimSpace(inferredTool.ActivityID)
			opts.diagnose(RenderDiagnostic{
				Code:       "live.render.infer_tool_fanout_parent",
				Message:    "inferred semantic runtime fanout parent from adjacent fanout tool call",
				WorkflowID: strings.TrimSpace(activity.WorkflowID),
				ActivityID: strings.TrimSpace(activity.ActivityID),
				StepID:     strings.TrimSpace(activity.StepID),
				ParentRef:  strings.TrimSpace(inferredTool.ActivityID),
			})
			registerGroupedActivity(grouped, inferredTool, inferredActivity)
			if key := activityIdentityKey(inferredActivity); key != "" {
				nestedKeys[key] = true
			}
		}
	}
	for parentID := range grouped {
		grouped[parentID] = sortActivitiesByTime(dedupeActivities(grouped[parentID]))
	}
	return grouped, nestedKeys
}

func inferredSemanticFanoutToolParent(visible []Activity, tools []Activity, activity Activity) (Activity, bool) {
	if !isRuntimeSemanticFanoutActivity(activity) {
		return Activity{}, false
	}
	parentRef := strings.TrimSpace(toolParentRef(activity))
	if parentRef != "" && !strings.HasPrefix(parentRef, "flow:") {
		return Activity{}, false
	}
	var match Activity
	for _, tool := range tools {
		if strings.TrimSpace(tool.WorkflowID) != strings.TrimSpace(activity.WorkflowID) {
			continue
		}
		if !isFanoutToolActivity(tool) {
			continue
		}
		if !fanoutToolCanInferActivity(tool, activity) {
			continue
		}
		if !toolIsRenderedUnderRuntimeParent(visible, tool) {
			continue
		}
		if !activityCanFollowTool(activity, tool) {
			continue
		}
		if activityIdentityKey(match) != "" {
			return Activity{}, false
		}
		match = tool
	}
	return match, activityIdentityKey(match) != ""
}

func isRuntimeSemanticFanoutActivity(activity Activity) bool {
	return isFanoutActivity(activity) &&
		!isGenericFanoutActivity(activity) &&
		!activity.Planned &&
		activityHasRecordedExecution(activity)
}

func isFanoutToolActivity(activity Activity) bool {
	if !isToolActivity(activity) {
		return false
	}
	return strings.Contains(fanoutInferenceText(activity), "fanout")
}

func fanoutToolCanInferActivity(tool Activity, activity Activity) bool {
	toolText := fanoutInferenceText(tool)
	activityText := fanoutInferenceText(activity)
	switch {
	case strings.Contains(toolText, "agent") || strings.Contains(toolText, "subagent"):
		return strings.Contains(activityText, "agent") || strings.Contains(activityText, "subagent")
	case strings.Contains(toolText, "child"):
		return strings.Contains(activityText, "child")
	default:
		return true
	}
}

func fanoutInferenceText(activity Activity) string {
	return strings.ToLower(strings.Join(compactParts(activity.ActivityName, activity.ActivityType, activity.ToolCallID, activity.ActivityID, activity.StepID), " "))
}

func toolIsRenderedUnderRuntimeParent(visible []Activity, tool Activity) bool {
	parentRef := toolParentRef(tool)
	if parentRef == "" {
		return false
	}
	for _, parent := range visible {
		if parent.Planned || isToolActivity(parent) {
			continue
		}
		if activityMatchesParentRef(parent, parentRef) {
			return true
		}
	}
	return false
}

func activityCanFollowTool(activity Activity, tool Activity) bool {
	activityStart := activityTime(activity)
	toolStart := activityTime(tool)
	if activityStart.IsZero() || toolStart.IsZero() {
		return true
	}
	return !activityStart.Before(toolStart.Add(-time.Millisecond))
}

func suppressPlannedGraphChild(nestedKeys map[string]bool, opts RenderOptions, activity Activity, parentRef string) {
	if key := activityIdentityKey(activity); key != "" {
		nestedKeys[key] = true
	}
	opts.diagnose(RenderDiagnostic{
		Code:       "live.render.suppress_planned_graph_child_with_runtime_children",
		Message:    "suppressed planned graph child because runtime children exist for the parent",
		WorkflowID: strings.TrimSpace(activity.WorkflowID),
		ActivityID: strings.TrimSpace(activity.ActivityID),
		StepID:     strings.TrimSpace(activity.StepID),
		ParentRef:  parentRef,
		ScopeID:    strings.TrimSpace(activity.GraphScopeID),
	})
}

func filterNestedActivitiesFromTopLevel(activities []Activity, nestedKeys map[string]bool) []Activity {
	if len(nestedKeys) == 0 {
		return activities
	}
	out := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		if nestedKeys[activityIdentityKey(activity)] {
			continue
		}
		out = append(out, activity)
	}
	return out
}

func groupGraphActivitiesByVisibleParent(visible []Activity, childrenByStep map[string][]RunNode, alreadyNested map[string]bool, opts RenderOptions) (map[string][]Activity, map[string]bool) {
	grouped := map[string][]Activity{}
	nestedKeys := map[string]bool{}
	takenBranchScopes := takenBranchScopesByParent(visible)
	for _, activity := range visible {
		if !isGraphNestedActivity(activity) {
			continue
		}
		activityKey := activityIdentityKey(activity)
		if activityKey != "" && alreadyNested[activityKey] {
			continue
		}
		parentRef := strings.TrimSpace(activity.ParentActivityID)
		if parentRef == "" || strings.HasPrefix(parentRef, "flow:") {
			continue
		}
		for _, parent := range visible {
			if activityIdentityKey(parent) == activityIdentityKey(activity) {
				continue
			}
			if !activityMatchesParentRef(parent, parentRef) {
				continue
			}
			if activity.Planned && isBranchActivity(parent) {
				parentKey := activityIdentityKey(parent)
				takenScope := takenBranchScopes[parentKey]
				if takenScope != "" && strings.TrimSpace(activity.GraphScopeID) != "" && strings.TrimSpace(activity.GraphScopeID) != takenScope {
					if activityKey != "" {
						nestedKeys[activityKey] = true
					}
					opts.diagnose(RenderDiagnostic{
						Code:       "live.render.suppress_untaken_branch",
						Message:    "suppressed planned graph branch child after a sibling branch had runtime evidence",
						WorkflowID: strings.TrimSpace(activity.WorkflowID),
						ActivityID: strings.TrimSpace(activity.ActivityID),
						StepID:     strings.TrimSpace(activity.StepID),
						ParentRef:  parentRef,
						ScopeID:    strings.TrimSpace(activity.GraphScopeID),
					})
					break
				}
			}
			if activity.Planned && runtimeParentSupplantsPlannedGraphChildren(parent) {
				suppressPlannedGraphChild(nestedKeys, opts, activity, parentRef)
				break
			}
			if len(childrenForStep(childrenByStep, parent)) > 0 {
				if activity.Planned {
					suppressPlannedGraphChild(nestedKeys, opts, activity, parentRef)
				}
				break
			}
			if activityKey != "" {
				nestedKeys[activityKey] = true
			}
			registerGroupedActivity(grouped, parent, activity)
			break
		}
	}
	for parentID := range grouped {
		grouped[parentID] = sortActivitiesByTime(dedupeActivities(grouped[parentID]))
	}
	return grouped, nestedKeys
}

func isGraphNestedActivity(activity Activity) bool {
	if strings.TrimSpace(activity.ParentActivityID) == "" || strings.HasPrefix(strings.TrimSpace(activity.ParentActivityID), "flow:") {
		return false
	}
	if activity.Planned {
		return true
	}
	if strings.TrimSpace(activity.GraphScopeID) != "" {
		return true
	}
	return activityHasRecordedExecution(activity)
}

func runtimeParentSupplantsPlannedGraphChildren(parent Activity) bool {
	if parent.Planned || (!parent.Active && !activityHasRecordedExecution(parent)) {
		return false
	}
	return isFanoutActivity(parent)
}

func takenBranchScopesByParent(visible []Activity) map[string]string {
	taken := map[string]string{}
	for _, activity := range visible {
		if activity.Planned || strings.TrimSpace(activity.GraphScopeID) == "" {
			continue
		}
		if !activity.Active && !activityHasRecordedExecution(activity) {
			continue
		}
		parentRef := strings.TrimSpace(activity.ParentActivityID)
		if parentRef == "" {
			continue
		}
		for _, parent := range visible {
			if !isBranchActivity(parent) || !activityMatchesParentRef(parent, parentRef) {
				continue
			}
			if key := activityIdentityKey(parent); key != "" {
				taken[key] = strings.TrimSpace(activity.GraphScopeID)
			}
			break
		}
	}
	return taken
}

func mergeActivityGroups(left map[string][]Activity, right map[string][]Activity) map[string][]Activity {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	for key, activities := range right {
		left[key] = sortActivitiesByTime(dedupeActivities(append(left[key], activities...)))
	}
	return left
}

func mergeActivityKeySets(left map[string]bool, right map[string]bool) map[string]bool {
	if len(left) == 0 {
		return right
	}
	for key := range right {
		left[key] = true
	}
	return left
}

func filterNestedToolActivities(selected []Activity, all []Activity) []Activity {
	out := make([]Activity, 0, len(selected))
	for _, activity := range selected {
		if isToolActivity(activity) && toolNestedUnderSelectedParent(activity, selected, all) {
			continue
		}
		out = append(out, activity)
	}
	return out
}

func toolNestedUnderSelectedParent(tool Activity, selected []Activity, all []Activity) bool {
	parentRef := toolParentRef(tool)
	if parentRef == "" {
		return false
	}
	for _, parent := range selected {
		if isToolActivity(parent) {
			continue
		}
		if activityMatchesParentID(parent, parentRef) {
			return true
		}
	}
	return false
}

func activityMatchesParentID(activity Activity, parentRef string) bool {
	return activityMatchesAnyParentRef(activity, parentRef, false)
}

func activityMatchesParentRef(activity Activity, parentRef string) bool {
	return activityMatchesAnyParentRef(activity, parentRef, true)
}

func activityMatchesAnyParentRef(activity Activity, parentRef string, includeToolCallID bool) bool {
	parentRef = strings.TrimSpace(parentRef)
	if parentRef == "" {
		return false
	}
	ids := []string{strings.TrimSpace(activity.ActivityID), strings.TrimSpace(activity.StepID)}
	if includeToolCallID {
		ids = append(ids, strings.TrimSpace(activity.ToolCallID))
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if parentRef == id || strings.HasSuffix(parentRef, ":"+id) || strings.HasSuffix(parentRef, "/"+id) {
			return true
		}
	}
	return false
}

func toolParentRef(tool Activity) string {
	for _, ref := range []string{
		strings.TrimSpace(tool.ParentActivityID),
		strings.TrimSpace(tool.ParentStepID),
	} {
		if ref != "" {
			return ref
		}
	}
	return ""
}

func dedupeActivities(activities []Activity) []Activity {
	if len(activities) < 2 {
		return activities
	}
	seen := map[string]bool{}
	out := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		key := firstNonBlank(activity.ActivityID, activity.ToolCallID, activity.StepID, activity.ActivityName)
		if key == "" {
			out = append(out, activity)
			continue
		}
		fullKey := activity.WorkflowID + "\x00" + key
		if seen[fullKey] {
			continue
		}
		seen[fullKey] = true
		out = append(out, activity)
	}
	return out
}

func dedupeResourceActivities(activities []Activity) []Activity {
	if len(activities) < 2 {
		return activities
	}
	seen := map[string]bool{}
	out := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		key := resourceIdentityKey(activity)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, activity)
	}
	return out
}

func resourceIdentityKey(activity Activity) string {
	key := firstNonBlank(activity.ResourceURI, activity.ActivityID, activity.ResourceLabel, activity.ActivityName)
	if key == "" {
		key = fmt.Sprintf("%s/%s/%s", activity.WorkflowID, activity.ParentActivityID, activity.ActivityName)
	}
	return activity.WorkflowID + "\x00" + key
}

func activityIdentityKey(activity Activity) string {
	key := firstNonBlank(activity.ActivityID, activity.ToolCallID, activity.StepID, activity.ActivityName)
	if key == "" {
		return ""
	}
	return activity.WorkflowID + "\x00" + key
}

func shouldInlineActivityContainer(activity Activity, run RunState, resources []Activity, tools []Activity, children []RunNode) bool {
	if len(children) > 0 {
		return false
	}
	if len(resources) == 0 && len(tools) == 0 {
		return false
	}
	if len(tools) > 0 {
		return false
	}
	if activity.ProgressCurrent != nil || isToolActivity(activity) {
		return false
	}
	status := normalizeStatus(activity.Status, activity.Active)
	if isProblemStatus(status) {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	if kind != "step" && kind != "fanout" {
		return false
	}
	stepID := strings.TrimSpace(activity.StepID)
	if stepID == "" {
		return false
	}
	if currentStepID := strings.TrimSpace(run.CurrentStepID); currentStepID != "" && currentStepID == stepID {
		return false
	}
	return isTerminalStatus(status)
}

func shouldElideAgentFanoutEntrypoint(node RunNode, activity Activity, tools []Activity, resources []Activity, children []RunNode) bool {
	if node.Relation == nil || !isAgentFanoutRelation(*node.Relation) {
		return false
	}
	if len(tools) == 0 && len(resources) == 0 && len(children) == 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "step") {
		return false
	}
	agentID := strings.TrimSpace(node.Relation.AgentID)
	if agentID == "" {
		return false
	}
	if strings.TrimSpace(activity.AgentID) == agentID {
		return true
	}
	activityType := strings.Trim(strings.TrimSpace(activity.ActivityType), ":")
	return strings.HasSuffix(activityType, "/"+agentID)
}

func suppressRedundantAgentLabel(node RunNode, activity Activity) Activity {
	if node.Relation == nil {
		return activity
	}
	agentID := strings.TrimSpace(node.Relation.AgentID)
	if agentID != "" && strings.TrimSpace(activity.AgentID) == agentID {
		activity.AgentID = ""
	}
	return activity
}

func isAgentFanoutRelation(relation RunRelation) bool {
	kind := strings.ToLower(strings.TrimSpace(relation.RelationKind))
	return kind == "agent_fanout" || kind == "agent" || kind == "subagent"
}

func groupChildrenByVisibleStep(children []RunNode, activities []Activity, opts RenderOptions) (map[string][]RunNode, []RunNode) {
	visibleSteps := map[string]bool{}
	for _, activity := range activities {
		if stepID := strings.TrimSpace(activity.StepID); stepID != "" {
			visibleSteps[stepID] = true
		}
		if activityID := strings.TrimSpace(activity.ActivityID); activityID != "" {
			visibleSteps[activityID] = true
		}
	}
	hasFanoutFallback := hasNamedFanoutFallbackActivity(activities)
	grouped := map[string][]RunNode{}
	remaining := make([]RunNode, 0, len(children))
	for _, child := range children {
		parentStepID := ""
		if child.Relation != nil {
			parentStepID = strings.TrimSpace(child.Relation.ParentStepID)
		}
		if parentStepID == "fanout" && hasFanoutFallback {
			grouped[parentStepID] = append(grouped[parentStepID], child)
			continue
		}
		if parentStepID == "fanout" && !visibleSteps[parentStepID] {
			opts.diagnose(RenderDiagnostic{
				Code:       "live.render.suppress_unanchored_generic_fanout_child",
				Message:    "suppressed child run whose only parent was the generic runtime fanout placeholder",
				WorkflowID: strings.TrimSpace(child.Run.WorkflowID),
				StepID:     parentStepID,
				ParentRef:  parentStepID,
			})
			continue
		}
		if parentStepID != "" && visibleSteps[parentStepID] {
			grouped[parentStepID] = append(grouped[parentStepID], child)
			continue
		}
		remaining = append(remaining, child)
	}
	return grouped, remaining
}

func childrenForStep(childrenByStep map[string][]RunNode, activity Activity) []RunNode {
	if children := childrenByStep[strings.TrimSpace(activity.StepID)]; len(children) > 0 {
		return children
	}
	if runtimeFanoutCanUseGenericChildren(activity) {
		if children := childrenByStep["fanout"]; len(children) > 0 {
			return children
		}
	}
	return childrenByStep[strings.TrimSpace(activity.ActivityID)]
}

func runtimeFanoutCanUseGenericChildren(activity Activity) bool {
	if activity.Planned || !activityHasRecordedExecution(activity) {
		return false
	}
	if isGenericFanoutActivity(activity) {
		return true
	}
	parentRef := strings.TrimSpace(activity.ParentActivityID)
	if parentRef == "" || strings.HasPrefix(parentRef, "flow:") {
		return false
	}
	return isNestedFanoutActivity(activity)
}

func hasNamedFanoutFallbackActivity(activities []Activity) bool {
	for _, activity := range activities {
		if isFanoutActivity(activity) && !isGenericFanoutActivity(activity) && isNestedFanoutActivity(activity) {
			return true
		}
	}
	return false
}

func hasNamedFanoutForParent(activities []Activity, parentRef string) bool {
	return namedFanoutForParent(activities, parentRef) != nil
}

func namedFanoutForParent(activities []Activity, parentRef string) *Activity {
	for _, activity := range activities {
		if isStructuralFanoutWrapper(activity) || !isNestedFanoutActivity(activity) {
			continue
		}
		if parentRefsEquivalent(toolParentRef(activity), parentRef) {
			candidate := activity
			return &candidate
		}
	}
	return nil
}

func parentRefsEquivalent(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return left == right ||
		strings.HasSuffix(left, "/"+right) ||
		strings.HasSuffix(left, ":"+right) ||
		strings.HasSuffix(right, "/"+left) ||
		strings.HasSuffix(right, ":"+left)
}

func isNestedFanoutActivity(activity Activity) bool {
	return isFanoutActivity(activity) && strings.TrimSpace(activity.ParentActivityID) != ""
}

func activityWithNestedActivityDuration(activity Activity, nested []Activity) Activity {
	if len(nested) == 0 || !isGraphContainerActivity(activity) {
		return activity
	}
	var start *time.Time
	var end time.Time
	active := false
	failed := false
	seenRuntime := false
	for _, child := range nested {
		if child.Planned && !child.Active && !activityHasRecordedExecution(child) {
			continue
		}
		seenRuntime = true
		status := normalizeStatus(child.Status, child.Active)
		if child.Active || status == "running" || status == "syncing" {
			active = true
		}
		if isProblemStatus(status) {
			failed = true
		}
		if child.StartedAt != nil && !child.StartedAt.IsZero() {
			if start == nil || child.StartedAt.Before(*start) {
				start = child.StartedAt
			}
		}
		for _, candidate := range []*time.Time{child.CompletedAt, &child.UpdatedAt} {
			if candidate != nil && !candidate.IsZero() && candidate.After(end) {
				end = *candidate
			}
		}
	}
	if !seenRuntime {
		return activity
	}
	activity.Planned = false
	if start != nil {
		activity.StartedAt = start
	}
	if active {
		activity.Active = true
		activity.Status = "running"
		activity.CompletedAt = nil
		return activity
	}
	activity.Active = false
	if failed {
		activity.Status = "failed"
	} else {
		activity.Status = "completed"
	}
	if !end.IsZero() {
		activity.CompletedAt = &end
	}
	return activity
}

func isGraphContainerActivity(activity Activity) bool {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	if kind == "branch" || kind == "loop" || kind == "fanout" {
		return true
	}
	if kind != "step" {
		return false
	}
	typ := strings.ToLower(strings.TrimSpace(activity.ActivityType))
	return typ == "branch" || typ == "loop" || typ == "fanout"
}

func activityWithFanoutChildDuration(activity Activity, children []RunNode) Activity {
	if len(children) == 0 || !isFanoutActivity(activity) {
		return activity
	}
	var start *time.Time
	var end time.Time
	active := false
	for _, child := range children {
		if runHasActiveWork(child.Run) || child.Run.Active {
			active = true
		}
		if child.Run.StartedAt != nil && !child.Run.StartedAt.IsZero() {
			if start == nil || child.Run.StartedAt.Before(*start) {
				start = child.Run.StartedAt
			}
		} else if child.Relation != nil && !child.Relation.CreatedAt.IsZero() {
			createdAt := child.Relation.CreatedAt
			if start == nil || createdAt.Before(*start) {
				start = &createdAt
			}
		}
		for _, candidate := range runEndCandidates(child) {
			if !candidate.IsZero() && candidate.After(end) {
				end = candidate
			}
		}
	}
	if start == nil {
		return activity
	}
	activity.Planned = false
	activity.StartedAt = start
	if active || end.IsZero() {
		activity.Active = true
		activity.Status = "running"
		activity.CompletedAt = nil
		return activity
	}
	activity.Active = false
	if normalizeStatus(activity.Status, false) == "pending" || normalizeStatus(activity.Status, false) == "waiting" || strings.TrimSpace(activity.Status) == "" {
		activity.Status = "completed"
	}
	activity.CompletedAt = &end
	return activity
}

func runEndCandidates(node RunNode) []time.Time {
	candidates := make([]time.Time, 0, 4)
	if node.Run.CompletedAt != nil {
		candidates = append(candidates, *node.Run.CompletedAt)
	}
	candidates = append(candidates, node.Run.LastEventAt, node.Run.UpdatedAt)
	if node.Relation != nil {
		candidates = append(candidates, node.Relation.UpdatedAt)
	}
	return candidates
}

func isFanoutActivity(activity Activity) bool {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	return kind == "fanout" || (kind == "step" && strings.EqualFold(strings.TrimSpace(activity.ActivityType), "fanout"))
}

func isBranchActivity(activity Activity) bool {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	if kind == "branch" {
		return true
	}
	return kind == "step" && strings.EqualFold(strings.TrimSpace(activity.ActivityType), "branch")
}

func ensureToolParentSteps(selected []Activity, all []Activity, workflowID string) []Activity {
	if len(all) == 0 {
		return selected
	}
	present := map[string]bool{}
	for _, activity := range selected {
		if stepID := strings.TrimSpace(activity.StepID); stepID != "" {
			present[workflowID+"\x00"+stepID] = true
		}
		if activityID := strings.TrimSpace(activity.ActivityID); activityID != "" {
			present[workflowID+"\x00"+activityID] = true
		}
	}
	for _, activity := range all {
		if !isToolActivity(activity) {
			continue
		}
		parentRef := toolParentRef(activity)
		if parentRef == "" {
			continue
		}
		key := workflowID + "\x00" + parentRef
		if present[key] {
			continue
		}
		parent := findActivityByStepRef(all, parentRef)
		if parent == nil || isToolActivity(*parent) || isChildFlowActivity(*parent) {
			continue
		}
		selected = append(selected, *parent)
		present[key] = true
		if stepID := strings.TrimSpace(parent.StepID); stepID != "" {
			present[workflowID+"\x00"+stepID] = true
		}
		if activityID := strings.TrimSpace(parent.ActivityID); activityID != "" {
			present[workflowID+"\x00"+activityID] = true
		}
	}
	return selected
}

func ensureParentStepActivities(selected []Activity, node RunNode, children []RunNode) []Activity {
	if len(children) == 0 {
		return selected
	}
	present := map[string]bool{}
	for _, activity := range selected {
		if stepID := strings.TrimSpace(activity.StepID); stepID != "" {
			present[node.Run.WorkflowID+"\x00"+stepID] = true
		}
		if activityID := strings.TrimSpace(activity.ActivityID); activityID != "" {
			present[node.Run.WorkflowID+"\x00"+activityID] = true
		}
	}
	for _, child := range children {
		if child.Relation == nil {
			continue
		}
		parentStepID := strings.TrimSpace(child.Relation.ParentStepID)
		if parentStepID == "" {
			continue
		}
		key := node.Run.WorkflowID + "\x00" + parentStepID
		if present[key] {
			continue
		}
		if parent := findActivityByStepRef(node.Activities, parentStepID); parent != nil && !isToolActivity(*parent) && !isChildFlowActivity(*parent) {
			selected = append(selected, *parent)
			present[key] = true
			continue
		}
		selected = append(selected, syntheticParentStepActivity(node.Run, parentStepID, child.Relation))
		present[key] = true
	}
	return selected
}

func findActivityByStepRef(activities []Activity, stepRef string) *Activity {
	stepRef = strings.TrimSpace(stepRef)
	if stepRef == "" {
		return nil
	}
	for i := range activities {
		activity := &activities[i]
		if strings.TrimSpace(activity.StepID) == stepRef || strings.TrimSpace(activity.ActivityID) == stepRef {
			return activity
		}
	}
	return nil
}

func syntheticParentStepActivity(run RunState, parentStepID string, relation *RunRelation) Activity {
	name := parentStepID
	stepType := "fanout"
	status := "completed"
	active := false
	var startedAt *time.Time
	var completedAt *time.Time

	if strings.TrimSpace(run.CurrentStepID) == parentStepID {
		name = firstNonBlank(run.CurrentStepName, parentStepID)
		stepType = firstNonBlank(run.CurrentStepType, stepType)
		status = firstNonBlank(run.CurrentStepStatus, runStatus(run))
		active = runHasActiveWork(run)
	}
	if relation != nil && !relation.CreatedAt.IsZero() {
		createdAt := relation.CreatedAt
		startedAt = &createdAt
		if !active {
			completedAt = &createdAt
		}
	}

	return Activity{
		WorkspaceID:    run.WorkspaceID,
		WorkflowID:     run.WorkflowID,
		RootWorkflowID: run.RootWorkflowID,
		ActivityID:     parentStepID,
		ActivityKind:   "step",
		ActivityType:   stepType,
		ActivityName:   name,
		StepID:         parentStepID,
		Status:         status,
		Active:         active,
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		UpdatedAt:      run.UpdatedAt,
	}
}

func branchChildPrefix(prefix string, _ bool) string {
	return prefix + "  "
}

func formatActivityLine(activity Activity, prefix string, opts RenderOptions) string {
	status := normalizeStatus(activity.Status, activity.Active)
	lineOpts := opts
	planned := isUnstartedPlannedActivity(activity)
	if planned {
		lineOpts.Color = false
	}
	line := fmt.Sprintf("%s%s%s", prefix, coloredActivityGlyphSlot(status, activity.Active, opts.Frame, lineOpts.Color), activityTextStyled(activity, lineOpts))
	if duration := activityDuration(activity, opts.Now); duration != "" {
		line += " " + dim(duration, lineOpts.Color)
	}
	if activity.Attempt != nil && *activity.Attempt > 0 {
		line += " " + dim(fmt.Sprintf("try %d", *activity.Attempt), lineOpts.Color)
	}
	if activity.AgentID != "" {
		line += " " + dim("@"+activity.AgentID, lineOpts.Color)
	}
	if progress := activityProgress(activity); progress != "" {
		line += " " + dim(progress, lineOpts.Color)
	}
	if statusText := failedActivityStatusText(status); statusText != "" {
		line += " " + statusBadge(statusText, lineOpts.Color)
	}
	if planned {
		line = gray(line, opts.Color)
	}
	return line
}

func isUnstartedPlannedActivity(activity Activity) bool {
	if !activity.Planned || activity.Active {
		return false
	}
	if activity.StartedAt != nil && !activity.StartedAt.IsZero() {
		return false
	}
	status := normalizeStatus(activity.Status, activity.Active)
	return !isTerminalStatus(status) && !isProblemStatus(status)
}

func formatCurrentStepFallbackLine(current string, prefix string, opts RenderOptions) string {
	return fmt.Sprintf("%s%s%s", prefix, coloredActivityGlyphSlot("running", true, opts.Frame, opts.Color), dim(current, opts.Color))
}

func renderActivity(b *strings.Builder, activity Activity, prefix string, _ bool, opts RenderOptions) {
	b.WriteString(formatActivityLine(activity, prefix, opts))
	b.WriteByte('\n')
}

func renderActivityBranch(b *strings.Builder, activity Activity, prefix string, opts RenderOptions, nestedByParent map[string][]Activity, resourcesByParent map[string][]Activity, toolsByParent map[string][]Activity, childrenByStep map[string][]RunNode, rootFlowSlug string) {
	children := childrenForStep(childrenByStep, activity)
	nestedActivities := nestedActivitiesForActivity(nestedByParent, activity)
	displayActivity := activityWithNestedActivityDuration(activity, nestedActivities)
	displayActivity = activityWithFanoutChildDuration(displayActivity, children)
	activityTools := toolsForActivity(toolsByParent, displayActivity)
	activityResources := resourcesForActivity(resourcesByParent, displayActivity)
	if replacement, ok := semanticFanoutReplacementForTool(displayActivity, nestedActivities, activityResources, activityTools); ok {
		renderActivityBranch(b, replacement, prefix, opts, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		return
	}
	if isStructuralFanoutWrapper(displayActivity) {
		for _, tool := range activityTools {
			renderActivityBranch(b, tool, prefix, opts, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		}
		for _, nested := range nestedActivities {
			renderActivityBranch(b, nested, prefix, opts, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		}
		for _, resource := range activityResources {
			renderResource(b, resource, prefix, opts)
		}
		for childIdx, child := range children {
			renderRunNode(b, child, prefix, childIdx == len(children)-1, opts, false, rootFlowSlug)
		}
		return
	}
	renderActivity(b, displayActivity, prefix, false, opts)
	childPrefix := branchChildPrefix(prefix, false)
	for _, tool := range activityTools {
		renderActivityBranch(b, tool, childPrefix, opts, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
	}
	for _, nested := range nestedActivities {
		renderActivityBranch(b, nested, childPrefix, opts, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
	}
	for _, resource := range activityResources {
		renderResource(b, resource, childPrefix, opts)
	}
	for childIdx, child := range children {
		renderRunNode(b, child, childPrefix, childIdx == len(children)-1, opts, false, rootFlowSlug)
	}
}

func collectActivityBranch(frame *DisplayFrame, activity Activity, prefix string, opts RenderOptions, run RunState, nestedByParent map[string][]Activity, resourcesByParent map[string][]Activity, toolsByParent map[string][]Activity, childrenByStep map[string][]RunNode, rootFlowSlug string) {
	children := childrenForStep(childrenByStep, activity)
	nestedActivities := nestedActivitiesForActivity(nestedByParent, activity)
	displayActivity := activityWithNestedActivityDuration(activity, nestedActivities)
	displayActivity = activityWithFanoutChildDuration(displayActivity, children)
	activityTools := toolsForActivity(toolsByParent, displayActivity)
	activityResources := resourcesForActivity(resourcesByParent, displayActivity)
	if replacement, ok := semanticFanoutReplacementForTool(displayActivity, nestedActivities, activityResources, activityTools); ok {
		collectActivityBranch(frame, replacement, prefix, opts, run, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		return
	}
	if isStructuralFanoutWrapper(displayActivity) {
		for _, tool := range activityTools {
			collectActivityBranch(frame, tool, prefix, opts, run, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		}
		for _, nested := range nestedActivities {
			collectActivityBranch(frame, nested, prefix, opts, run, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		}
		for _, resource := range activityResources {
			collectResource(frame, resource, prefix, opts, run)
		}
		for childIdx, child := range children {
			collectRunNode(frame, child, prefix, childIdx == len(children)-1, opts, false, rootFlowSlug)
		}
		return
	}
	collectActivity(frame, displayActivity, prefix, opts, run)
	childPrefix := branchChildPrefix(prefix, false)
	for _, tool := range activityTools {
		collectActivityBranch(frame, tool, childPrefix, opts, run, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
	}
	for _, nested := range nestedActivities {
		collectActivityBranch(frame, nested, childPrefix, opts, run, nestedByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
	}
	for _, resource := range activityResources {
		collectResource(frame, resource, childPrefix, opts, run)
	}
	for childIdx, child := range children {
		collectRunNode(frame, child, childPrefix, childIdx == len(children)-1, opts, false, rootFlowSlug)
	}
}

func semanticFanoutReplacementForTool(activity Activity, nested []Activity, resources []Activity, tools []Activity) (Activity, bool) {
	if !isToolActivity(activity) || len(resources) > 0 || len(tools) > 0 {
		return Activity{}, false
	}
	var replacement Activity
	for _, child := range nested {
		if isStructuralFanoutWrapper(child) {
			continue
		}
		if !isNestedFanoutActivity(child) {
			return Activity{}, false
		}
		if activityIdentityKey(replacement) != "" {
			return Activity{}, false
		}
		replacement = child
	}
	return replacement, activityIdentityKey(replacement) != ""
}

func renderCurrentStepFallback(b *strings.Builder, current string, prefix string, opts RenderOptions) {
	b.WriteString(formatCurrentStepFallbackLine(current, prefix, opts))
	b.WriteByte('\n')
}

func formatResourceLine(activity Activity, prefix string, opts RenderOptions) string {
	lineOpts := opts
	planned := isUnstartedPlannedActivity(activity)
	if planned {
		lineOpts.Color = false
	}
	label := resourceLabelText(activity)
	if isFlowErrorResource(activity) {
		label = color(label, colorForStatus("failed"), lineOpts.Color)
	}
	line := fmt.Sprintf("%s  %s %s", prefix, resourceKindMark(activity, lineOpts.Color), label)
	if details := resourceDetailText(activity); details != "" {
		line += " " + dim(details, lineOpts.Color)
	}
	if planned {
		line = gray(line, opts.Color)
	}
	return line
}

func renderResource(b *strings.Builder, activity Activity, prefix string, opts RenderOptions) {
	if isAutomaticStepCaptureResource(activity) {
		return
	}
	line := formatResourceLine(activity, prefix, opts)
	b.WriteString(line)
	b.WriteByte('\n')
}

func isStepLikeActivity(activity Activity) bool {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	switch kind {
	case "step", "activity", "tool_call", "mcp_tool_call", "branch", "loop", "fanout", "child_flow", "mcp_session":
		return true
	default:
		return false
	}
}

func isStructuralFanoutWrapper(activity Activity) bool {
	return isGenericFanoutActivity(activity)
}

func isGenericFanoutActivity(activity Activity) bool {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	if kind != "step" && kind != "fanout" {
		return false
	}
	if kind == "step" && !strings.EqualFold(strings.TrimSpace(activity.ActivityType), "fanout") {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(activityLabel(activity)), "fanout") {
		return false
	}
	stepID := strings.TrimSpace(activity.StepID)
	activityID := strings.TrimSpace(activity.ActivityID)
	return stepID == "" || strings.EqualFold(stepID, "fanout") || strings.EqualFold(activityID, "fanout")
}

func activityHasRecordedExecution(activity Activity) bool {
	if activity.StartedAt != nil && !activity.StartedAt.IsZero() {
		return true
	}
	return isTerminalStatus(normalizeStatus(activity.Status, activity.Active))
}

func sortActivitiesByTime(activities []Activity) []Activity {
	if len(activities) < 2 {
		return activities
	}
	sorted := append([]Activity(nil), activities...)
	sort.SliceStable(sorted, func(i, j int) bool {
		leftOrder := sorted[i].GraphOrder
		rightOrder := sorted[j].GraphOrder
		if leftOrder > 0 || rightOrder > 0 {
			if leftOrder == 0 {
				leftOrder = 1 << 30
			}
			if rightOrder == 0 {
				rightOrder = 1 << 30
			}
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
		}
		left := activityTime(sorted[i])
		right := activityTime(sorted[j])
		if left.Equal(right) {
			return sorted[i].ActivityID < sorted[j].ActivityID
		}
		return left.Before(right)
	})
	return sorted
}

func omitCompletedStepActivities(activities []Activity) []Activity {
	out := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		if activity.Active {
			out = append(out, activity)
			continue
		}
		if !isStepLikeActivity(activity) {
			out = append(out, activity)
		}
	}
	return out
}

func selectedActivities(node RunNode, children []RunNode, opts RenderOptions) []Activity {
	max := opts.MaxActivitiesPerRun
	activities := nonResourceActivities(node.Activities)
	resourceParents := resourceParentIDs(node.Activities)
	childParentSteps := childParentStepIDs(children)
	visibleChildWorkflowIDs := childWorkflowIDs(children)

	selected := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		if isChildFlowActivity(activity) && visibleChildWorkflowIDs[strings.TrimSpace(activity.ActivityID)] {
			continue
		}
		if shouldRenderActivity(activity, node.Run.CurrentStepID, resourceParents, childParentSteps) {
			selected = append(selected, activity)
		}
	}
	selected = ensureParentStepActivities(selected, node, children)
	selected = ensureToolParentSteps(selected, node.Activities, node.Run.WorkflowID)
	selected = ensureResourceParentSteps(selected, node)
	selected = suppressUnanchoredGenericFanoutWrappers(selected, opts)
	selected = suppressPlannedUntakenScopeActivities(selected, opts)
	selected = sortActivitiesByTime(selected)
	selected = filterNestedToolActivities(selected, node.Activities)
	selected = collapseLoopIterationScope(selected, node.Run)
	selected = suppressDuplicateStepActivities(selected)
	if runHasActiveWork(node.Run) {
		if opts.OmitCompletedSteps {
			selected = omitCompletedStepActivities(selected)
		}
		return selected
	}
	if opts.FullTree || len(selected) <= max {
		return selected
	}
	active := make([]Activity, 0, len(selected))
	for _, activity := range selected {
		if activity.Active || isProblemStatus(normalizeStatus(activity.Status, activity.Active)) {
			active = append(active, activity)
		}
	}
	start := len(selected) - max
	if start < 0 {
		start = 0
	}
	trimmed := append([]Activity(nil), selected[start:]...)
	for _, activity := range active {
		found := false
		for _, existing := range trimmed {
			if existing.ActivityID == activity.ActivityID && existing.WorkflowID == activity.WorkflowID {
				found = true
				break
			}
		}
		if !found {
			trimmed = append([]Activity{activity}, trimmed...)
		}
	}
	if len(trimmed) > max {
		trimmed = trimmed[len(trimmed)-max:]
	}
	return trimmed
}

func suppressUnanchoredGenericFanoutWrappers(activities []Activity, opts RenderOptions) []Activity {
	out := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		if isGenericFanoutActivity(activity) &&
			strings.TrimSpace(activity.ParentActivityID) == "" &&
			strings.TrimSpace(activity.ParentStepID) == "" {
			opts.diagnose(RenderDiagnostic{
				Code:       "live.render.suppress_unanchored_generic_fanout",
				Message:    "suppressed generic runtime fanout placeholder without a semantic/tool parent",
				WorkflowID: strings.TrimSpace(activity.WorkflowID),
				ActivityID: strings.TrimSpace(activity.ActivityID),
				StepID:     strings.TrimSpace(activity.StepID),
			})
			continue
		}
		out = append(out, activity)
	}
	return out
}

func suppressPlannedUntakenScopeActivities(activities []Activity, opts RenderOptions) []Activity {
	takenByOwner := map[string]string{}
	for _, activity := range activities {
		if activity.Planned || strings.TrimSpace(activity.GraphScopeID) == "" {
			continue
		}
		if !activity.Active && !activityHasRecordedExecution(activity) {
			continue
		}
		owner := graphScopeOwner(activity.GraphScopeID)
		if owner == "" {
			continue
		}
		takenByOwner[owner] = strings.TrimSpace(activity.GraphScopeID)
	}
	if len(takenByOwner) == 0 {
		return activities
	}
	out := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		scopeID := strings.TrimSpace(activity.GraphScopeID)
		owner := graphScopeOwner(scopeID)
		if activity.Planned && owner != "" && takenByOwner[owner] != "" && takenByOwner[owner] != scopeID {
			opts.diagnose(RenderDiagnostic{
				Code:       "live.render.suppress_untaken_scope",
				Message:    "suppressed planned graph activity in untaken branch scope",
				WorkflowID: strings.TrimSpace(activity.WorkflowID),
				ActivityID: strings.TrimSpace(activity.ActivityID),
				StepID:     strings.TrimSpace(activity.StepID),
				ScopeID:    scopeID,
			})
			continue
		}
		out = append(out, activity)
	}
	return out
}

func graphScopeOwner(scopeID string) string {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return ""
	}
	if idx := strings.Index(scopeID, ":scope:"); idx > 0 {
		return scopeID[:idx]
	}
	return ""
}

func shouldRenderActivity(activity Activity, currentStepID string, resourceParents map[string]bool, childParentSteps map[string]bool) bool {
	activityID := strings.TrimSpace(activity.ActivityID)
	stepID := strings.TrimSpace(activity.StepID)
	if resourceParents[activityID] || resourceParents[stepID] {
		return true
	}
	if childParentSteps[stepID] {
		return true
	}
	if isNestedFanoutActivity(activity) && !isStructuralFanoutWrapper(activity) &&
		(activity.Active || activityHasRecordedExecution(activity) || childParentSteps[activityID] || childParentSteps[stepID] || childParentSteps["fanout"]) {
		return true
	}
	status := normalizeStatus(activity.Status, activity.Active)
	if isProblemStatus(status) {
		return true
	}
	if activity.ProgressCurrent != nil {
		return true
	}
	if activity.Planned {
		return true
	}
	if !activity.Active && isStepLikeActivity(activity) && activityHasRecordedExecution(activity) {
		return true
	}
	if activity.Active && stepID != "" && currentStepID != "" && stepID == strings.TrimSpace(currentStepID) {
		return true
	}
	if !activity.Active {
		return false
	}
	if stepID != "" && currentStepID != "" && stepID != strings.TrimSpace(currentStepID) && !isToolActivity(activity) {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	switch kind {
	case "fanout", "loop", "tool_call", "mcp_tool_call", "mcp_session", "child_flow":
		return kind != "fanout" && kind != "loop"
	case "step":
		if activity.Active {
			return true
		}
		typ := strings.ToLower(strings.TrimSpace(activity.ActivityType))
		return typ == "loop" && activity.ProgressCurrent != nil
	default:
		return kind != ""
	}
}

func resourceParentIDs(activities []Activity) map[string]bool {
	parents := map[string]bool{}
	for _, activity := range activities {
		if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "resource") {
			continue
		}
		if parentID := strings.TrimSpace(activity.ParentActivityID); parentID != "" {
			parents[parentID] = true
		}
	}
	return parents
}

func ensureResourceParentSteps(selected []Activity, node RunNode) []Activity {
	present := map[string]bool{}
	for _, activity := range selected {
		for _, id := range []string{activity.ActivityID, activity.StepID} {
			id = strings.TrimSpace(id)
			if id != "" {
				present[node.Run.WorkflowID+"\x00"+id] = true
			}
		}
	}
	for _, activity := range node.Activities {
		if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "resource") {
			continue
		}
		parentID := strings.TrimSpace(activity.ParentActivityID)
		if parentID == "" {
			continue
		}
		if isRunResultResourceForRun(activity, node.Run) {
			continue
		}
		key := node.Run.WorkflowID + "\x00" + parentID
		if present[key] {
			continue
		}
		if parent := findActivityByStepRef(node.Activities, parentID); parent != nil && !strings.EqualFold(strings.TrimSpace(parent.ActivityKind), "resource") {
			selected = append(selected, *parent)
		} else {
			selected = append(selected, syntheticResourceParentActivity(node.Run, parentID, activity))
		}
		present[key] = true
	}
	return selected
}

func syntheticResourceParentActivity(run RunState, parentID string, resource Activity) Activity {
	updatedAt := resource.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = run.UpdatedAt
	}
	return Activity{
		WorkspaceID:    run.WorkspaceID,
		WorkflowID:     run.WorkflowID,
		RootWorkflowID: firstNonBlank(run.RootWorkflowID, run.WorkflowID),
		ActivityID:     parentID,
		ActivityKind:   "step",
		ActivityType:   "function",
		ActivityName:   parentID,
		StepID:         parentID,
		Status:         "completed",
		UpdatedAt:      updatedAt,
	}
}

func childParentStepIDs(children []RunNode) map[string]bool {
	parentSteps := map[string]bool{}
	for _, child := range children {
		if child.Relation == nil {
			continue
		}
		if stepID := strings.TrimSpace(child.Relation.ParentStepID); stepID != "" {
			parentSteps[stepID] = true
		}
	}
	return parentSteps
}

func childWorkflowIDs(children []RunNode) map[string]bool {
	ids := map[string]bool{}
	for _, child := range children {
		if workflowID := strings.TrimSpace(child.Run.WorkflowID); workflowID != "" {
			ids[workflowID] = true
		}
	}
	return ids
}

func liveSummaryText(snapshot Snapshot, mode DetailMode) string {
	if mode == DetailModePublic {
		return ""
	}
	parts := []string{}
	if snapshot.Workspace.ActiveChildRunCount > 0 {
		parts = append(parts, countLabel(snapshot.Workspace.ActiveChildRunCount, "child", "children"))
	}
	if count := activeAgentCount(snapshot); count > 0 {
		parts = append(parts, countLabel(count, "agent", "agents"))
	}
	if count := activeToolCount(snapshot); count > 0 {
		parts = append(parts, countLabel(count, "tool", "tools"))
	}
	return strings.Join(parts, ", ")
}

func countLabel[T ~int | ~int64](count T, singular string, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func activeAgentCount(snapshot Snapshot) int {
	count := 0
	for _, run := range snapshot.Runs {
		if run.Active && strings.TrimSpace(run.AgentID) != "" {
			count++
		}
	}
	return count
}

func activeToolCount(snapshot Snapshot) int {
	count := 0
	for _, activity := range snapshot.Nodes {
		if activity.Active && isToolActivity(activity) {
			count++
		}
	}
	return count
}

func hasVisibleCurrentStepActivity(node RunNode) bool {
	currentStepID := strings.TrimSpace(node.Run.CurrentStepID)
	if currentStepID == "" {
		return false
	}
	for _, activity := range node.Activities {
		if strings.TrimSpace(activity.StepID) != currentStepID {
			continue
		}
		if activity.Active {
			return true
		}
		if isStepLikeActivity(activity) && activityHasRecordedExecution(activity) {
			return true
		}
	}
	return false
}

func runSummaryStrip(snapshot Snapshot, opts RenderOptions) string {
	if opts.DetailMode == DetailModePublic {
		return publicRunSummaryStrip(snapshot)
	}
	completed := int64(0)
	failed := int64(0)
	running := int64(0)
	for _, run := range snapshot.Runs {
		completed += run.StepsCompleted
		failed += run.StepsFailed
		running += run.StepsRunning
	}
	executed := completed + failed
	resourceCounts := map[string]int{}
	seenResources := map[string]bool{}
	for _, activity := range snapshot.Nodes {
		if !strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "resource") {
			continue
		}
		if activity.Planned {
			continue
		}
		if normalizeStatus(activity.Status, activity.Active) == "failed" {
			continue
		}
		key := resourceIdentityKey(activity)
		if seenResources[key] {
			continue
		}
		seenResources[key] = true
		kind := resourceKindMarkLabel(firstNonBlank(activity.ResourceKind, activity.ActivityType, "resource"))
		resourceCounts[kind]++
	}
	parts := []string{}
	if duration := snapshotRunDurationText(snapshot, opts, opts.Now); duration != "" {
		parts = append(parts, color(duration, "36", opts.Color)+" total")
	}
	if executed > 0 {
		parts = append(parts, summaryCountText(executed, "step", "steps", "executed", "32", opts.Color))
	}
	if failed > 0 {
		parts = append(parts, summaryCountText(failed, "failed step", "failed steps", "", "31", opts.Color))
	}
	if running > 0 {
		parts = append(parts, summaryCountText(running, "step running", "steps running", "", "36", opts.Color))
	}
	if len(resourceCounts) > 0 {
		parts = append(parts, resourceSummaryText(resourceCounts, opts.Color))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func summaryCountText[T ~int | ~int64](count T, singular string, plural string, suffix string, code string, enabled bool) string {
	label := singular
	if count != 1 {
		label = plural
	}
	if enabled && suffix == "" && strings.Contains(label, "failed") {
		return color(fmt.Sprintf("%d %s", count, label), code, enabled)
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s %s", color(fmt.Sprintf("%d", count), code, enabled), label, suffix))
}

func publicRunSummaryStrip(snapshot Snapshot) string {
	statusCounts := map[string]int{}
	for _, run := range snapshot.Runs {
		if strings.TrimSpace(run.ParentWorkflowID) != "" {
			continue
		}
		statusCounts[runStatus(run)]++
	}
	if len(statusCounts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(statusCounts))
	for key := range statusCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, countLabel(statusCounts[key], key+" run", key+" runs"))
	}
	return strings.Join(parts, ", ")
}

func resourceSummaryText(counts map[string]int, colorEnabled bool) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	total := 0
	for _, key := range keys {
		total += counts[key]
		parts = append(parts, fmt.Sprintf("%s %s", resourceKindMarkForKind(key, colorEnabled), color(fmt.Sprintf("%d", counts[key]), activityTypeColor("resource"), colorEnabled)))
	}
	return fmt.Sprintf("%s (%s)", summaryCountText(total, "resource", "resources", "", activityTypeColor("resource"), colorEnabled), strings.Join(parts, ", "))
}

func flatRootRunNode(nodes []RunNode, opts RenderOptions) *RunNode {
	root := focusRootRunNode(nodes, opts)
	if root == nil {
		return nil
	}
	if !shouldFlattenRootRunLine(*root, opts) {
		return nil
	}
	return root
}

func focusRootRunNode(nodes []RunNode, opts RenderOptions) *RunNode {
	if len(nodes) == 0 {
		return nil
	}
	focus := strings.TrimSpace(opts.FocusWorkflowID)
	if focus != "" {
		for i := range nodes {
			node := &nodes[i]
			if node.Relation != nil {
				continue
			}
			if strings.TrimSpace(node.Run.WorkflowID) == focus {
				return node
			}
		}
		for i := range nodes {
			node := &nodes[i]
			if node.Relation != nil {
				continue
			}
			if strings.TrimSpace(node.Run.RootWorkflowID) == focus {
				return node
			}
		}
	}
	if len(nodes) == 1 {
		return &nodes[0]
	}
	for i := range nodes {
		node := &nodes[i]
		if node.Relation == nil && strings.TrimSpace(node.Run.ParentWorkflowID) == "" {
			return node
		}
	}
	return nil
}

func shouldFlattenRootRunLine(node RunNode, opts RenderOptions) bool {
	if opts.DetailMode == DetailModePublic || node.Relation != nil {
		return false
	}
	if strings.TrimSpace(node.Run.AgentID) != "" {
		return false
	}
	focus := strings.TrimSpace(opts.FocusWorkflowID)
	if focus != "" && strings.TrimSpace(node.Run.WorkflowID) == focus {
		return true
	}
	flowSlug := strings.TrimSpace(node.Run.FlowSlug)
	if flowSlug == "" {
		return false
	}
	label := runLabel(node.Run, node.Relation, flowSlug)
	return label == shortLabel(flowSlug, 48) || label == shortLabel(flowSlug, 40)
}

func writeSnapshotHeader(b *strings.Builder, snapshot Snapshot, opts RenderOptions, activeWork bool, headerStatus string, flatRoot *RunNode) {
	fmt.Fprintf(b, "%s", coloredGlyph(headerStatus, activeWork, opts.Frame, opts.Color))
	if flatRoot != nil {
		flowLabel := shortID(firstNonBlank(flatRoot.Run.WorkflowID, flatRoot.Run.FlowSlug, "run"))
		fmt.Fprintf(b, "  %s %s", flowMark(opts.Color), flowLabel)
		if wf := strings.TrimSpace(firstNonBlank(opts.FocusWorkflowID, flatRoot.Run.WorkflowID)); wf != "" {
			if wf != flatRoot.Run.WorkflowID {
				fmt.Fprintf(b, "  %s", dim(shortID(wf), opts.Color))
			}
		}
	} else if header := strings.TrimSpace(opts.FocusWorkflowID); header != "" {
		fmt.Fprintf(b, "  %s %s", flowMark(opts.Color), shortID(header))
	}
	if snapshot.Workspace.WorkspaceID != "" {
		fmt.Fprintf(b, "  %s", dim(snapshot.Workspace.WorkspaceID, opts.Color))
	}
	if duration := headerRunDurationText(snapshot, opts, flatRoot, opts.Now); duration != "" {
		fmt.Fprintf(b, "  %s", dim(duration, opts.Color))
	}
	if summary := liveSummaryText(snapshot, opts.DetailMode); summary != "" {
		fmt.Fprintf(b, "  %s", dim(summary, opts.Color))
	}
	b.WriteByte('\n')
}

func headerRunDurationText(snapshot Snapshot, opts RenderOptions, flatRoot *RunNode, now time.Time) string {
	if flatRoot != nil {
		if duration := runDurationText(flatRoot.Run, now); duration != "" {
			return duration
		}
		return activityRangeDurationText(snapshot, flatRoot.Run, now)
	}
	return snapshotRunDurationText(snapshot, opts, now)
}

func snapshotRunDurationText(snapshot Snapshot, opts RenderOptions, now time.Time) string {
	run := primarySnapshotRun(snapshot, opts)
	if run == nil {
		return ""
	}
	if duration := runDurationText(*run, now); duration != "" {
		return duration
	}
	return activityRangeDurationText(snapshot, *run, now)
}

func primarySnapshotRun(snapshot Snapshot, opts RenderOptions) *RunState {
	focus := strings.TrimSpace(opts.FocusWorkflowID)
	if focus != "" {
		for i := range snapshot.Runs {
			run := &snapshot.Runs[i]
			if strings.TrimSpace(run.WorkflowID) == focus {
				return run
			}
		}
		for i := range snapshot.Runs {
			run := &snapshot.Runs[i]
			if strings.TrimSpace(run.RootWorkflowID) == focus {
				return run
			}
		}
	}
	for i := range snapshot.Runs {
		run := &snapshot.Runs[i]
		if strings.TrimSpace(run.ParentWorkflowID) == "" {
			return run
		}
	}
	if len(snapshot.Runs) > 0 {
		return &snapshot.Runs[0]
	}
	return nil
}

func runDurationText(run RunState, now time.Time) string {
	if run.StartedAt == nil || run.StartedAt.IsZero() {
		return ""
	}
	var completed *time.Time
	if run.CompletedAt != nil && !run.CompletedAt.IsZero() {
		completed = run.CompletedAt
	} else if !runHasActiveWork(run) {
		end := run.UpdatedAt
		if run.LastEventAt.After(end) {
			end = run.LastEventAt
		}
		if !end.IsZero() {
			completed = &end
		}
	}
	return formatDuration(durationSince(now, *run.StartedAt, completed))
}

func runNodeDurationText(node RunNode, now time.Time) string {
	if duration := runDurationText(node.Run, now); duration != "" {
		return duration
	}
	return activitySliceDurationText(node.Activities, node.Run, now)
}

func activitySliceDurationText(activities []Activity, run RunState, now time.Time) string {
	var start *time.Time
	var end time.Time
	for _, activity := range activities {
		if strings.TrimSpace(activity.WorkflowID) != strings.TrimSpace(run.WorkflowID) {
			continue
		}
		if activity.StartedAt != nil && !activity.StartedAt.IsZero() {
			if start == nil || activity.StartedAt.Before(*start) {
				start = activity.StartedAt
			}
		}
		for _, candidate := range []*time.Time{activity.CompletedAt, &activity.UpdatedAt} {
			if candidate != nil && !candidate.IsZero() && candidate.After(end) {
				end = *candidate
			}
		}
	}
	if start == nil {
		return ""
	}
	if runHasActiveWork(run) {
		return formatDuration(durationSince(now, *start, nil))
	}
	if end.IsZero() {
		return ""
	}
	return formatDuration(durationSince(now, *start, &end))
}

func activityRangeDurationText(snapshot Snapshot, run RunState, now time.Time) string {
	rootID := firstNonBlank(run.RootWorkflowID, run.WorkflowID)
	var start *time.Time
	var end time.Time
	for _, activity := range snapshot.Nodes {
		if strings.TrimSpace(activity.WorkflowID) != strings.TrimSpace(run.WorkflowID) &&
			strings.TrimSpace(activity.RootWorkflowID) != strings.TrimSpace(rootID) {
			continue
		}
		if activity.StartedAt != nil && !activity.StartedAt.IsZero() {
			if start == nil || activity.StartedAt.Before(*start) {
				start = activity.StartedAt
			}
		}
		for _, candidate := range []*time.Time{activity.CompletedAt, &activity.UpdatedAt} {
			if candidate != nil && !candidate.IsZero() && candidate.After(end) {
				end = *candidate
			}
		}
	}
	if start == nil {
		return ""
	}
	if runHasActiveWork(run) {
		return formatDuration(durationSince(now, *start, nil))
	}
	if end.IsZero() {
		return ""
	}
	return formatDuration(durationSince(now, *start, &end))
}

func flowSlugsEquivalent(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return strings.EqualFold(left, right)
}

func runLabel(run RunState, relation *RunRelation, rootFlowSlug string) string {
	return runLabelStyled(run, relation, rootFlowSlug, false)
}

func runLabelStyled(run RunState, relation *RunRelation, rootFlowSlug string, enabled bool) string {
	agentID := strings.TrimSpace(run.AgentID)
	if agentID == "" && relation != nil {
		agentID = strings.TrimSpace(relation.AgentID)
	}
	flowSlug := strings.TrimSpace(run.FlowSlug)
	if flowSlug == "" && relation != nil {
		flowSlug = strings.TrimSpace(relation.FlowSlug)
	}
	if agentID != "" {
		if flowSlug != "" && flowSlug != agentID && !flowSlugsEquivalent(flowSlug, rootFlowSlug) {
			return fmt.Sprintf("%s %s %s", typeMark("a", activityTypeColor("agent"), enabled), shortLabel(agentID, 28), shortLabel(cleanFlowLabel(flowSlug, run.WorkflowID), 32))
		}
		return typeMark("a", activityTypeColor("agent"), enabled) + " " + shortLabel(agentID, 40)
	}
	if relation != nil && relationKindLabel(relation.RelationKind) == "child flow" && flowSlug != "" {
		return flowMark(enabled) + " " + shortLabel(cleanFlowLabel(flowSlug, run.WorkflowID), 40)
	}
	return flowMark(enabled) + " " + shortLabel(cleanFlowLabel(flowSlug, run.WorkflowID), 48)
}

func flowMark(enabled bool) string {
	return typeMark("f", activityTypeColor("flow"), enabled)
}

func cleanFlowLabel(flowSlug string, workflowID string) string {
	if label := strings.TrimSpace(flowSlug); label != "" {
		return cleanGeneratedFlowID(label)
	}
	return cleanGeneratedFlowID(firstNonBlank(workflowID, "run"))
}

func cleanGeneratedFlowID(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "flow-")
}

func activeToolText(node RunNode, now time.Time) string {
	var selected *Activity
	for i := range node.Activities {
		activity := &node.Activities[i]
		if !activity.Active || !isToolActivity(*activity) {
			continue
		}
		if selected == nil || activityTime(*selected).Before(activityTime(*activity)) {
			selected = activity
		}
	}
	if selected == nil {
		return ""
	}
	kind := "tool"
	if strings.EqualFold(strings.TrimSpace(selected.ActivityKind), "mcp_tool_call") {
		kind = "MCP tool"
	}
	parts := []string{kind, shortLabel(activityLabel(*selected), 32)}
	if activityType := activityTypeText(*selected); activityType != "" {
		parts = append(parts, activityType)
	}
	if duration := activityDuration(*selected, now); duration != "" {
		parts = append(parts, duration)
	}
	if selected.Attempt != nil && *selected.Attempt > 0 {
		parts = append(parts, fmt.Sprintf("attempt %d", *selected.Attempt))
	}
	return strings.Join(parts, " ")
}

func sameActiveTool(current string, tool string) bool {
	current = strings.ToLower(strings.TrimSpace(current))
	tool = strings.ToLower(strings.TrimSpace(tool))
	if current == "" || tool == "" {
		return false
	}
	tool = strings.TrimPrefix(tool, "tool ")
	tool = strings.TrimPrefix(tool, "mcp tool ")
	if idx := strings.Index(tool, " ("); idx >= 0 {
		tool = tool[:idx]
	}
	tool = strings.TrimSpace(tool)
	return tool != "" && strings.Contains(current, tool)
}

func runStatus(run RunState) string {
	status := strings.TrimSpace(run.Status)
	if status == "" && (run.Active || run.StepsRunning > 0) {
		return "running"
	}
	if status == "" && hasPartialRunState(run) {
		return "syncing"
	}
	return normalizeStatus(status, run.Active)
}

func hasPartialRunState(run RunState) bool {
	return strings.TrimSpace(run.CurrentStepID) != "" ||
		strings.TrimSpace(run.CurrentStepName) != "" ||
		run.StepsStarted > 0 ||
		run.StepsCompleted > 0 ||
		run.StepsFailed > 0 ||
		run.StepsExecutedTotal > 0
}

func isProblemStatus(status string) bool {
	switch normalizeStatus(status, false) {
	case "failed", "error", "cancelled", "canceled", "timed-out", "timeout":
		return true
	default:
		return false
	}
}

func isTerminalStatus(status string) bool {
	switch normalizeStatus(status, false) {
	case "completed", "succeeded", "success", "failed", "error", "cancelled", "canceled", "timed-out", "timeout":
		return true
	default:
		return false
	}
}

func currentStepText(run RunState, activities []Activity, now time.Time) string {
	stepID := strings.TrimSpace(run.CurrentStepID)
	if stepID == "" {
		return ""
	}
	name := firstNonBlank(run.CurrentStepName, stepID)
	stepType := strings.TrimSpace(run.CurrentStepType)
	status := normalizeStatus(run.CurrentStepStatus, true)
	for _, activity := range activities {
		if activity.StepID == stepID && activity.StartedAt != nil {
			duration := formatDuration(durationSince(now, *activity.StartedAt, activity.CompletedAt))
			if stepType != "" {
				return strings.Join(compactParts(name, stepType, compactStatusText(status), duration), " ")
			}
			return strings.Join(compactParts(name, compactStatusText(status), duration), " ")
		}
	}
	if stepType != "" {
		return strings.Join(compactParts(name, stepType, compactStatusText(status)), " ")
	}
	return strings.Join(compactParts(name, compactStatusText(status)), " ")
}

func relationLabel(relation RunRelation, includeKind bool) string {
	if relation.FanoutBranchIndex != nil {
		return fmt.Sprintf("[b%d]", *relation.FanoutBranchIndex)
	}
	parts := []string{}
	if includeKind && relation.RelationKind != "" {
		parts = append(parts, relationKindLabel(relation.RelationKind))
	}
	if relation.ParentStepID != "" {
		parts = append(parts, "from "+shortLabel(relation.ParentStepID, 24))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func relationKindLabel(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "child_flow":
		return "child flow"
	case "agent", "subagent":
		return "agent"
	default:
		return strings.ReplaceAll(kind, "_", " ")
	}
}

func activityKindLabel(activity Activity) string {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	switch kind {
	case "mcp_tool_call":
		return "mcp tool"
	case "tool_call":
		return "tool"
	case "mcp_session":
		return "mcp"
	case "child_flow":
		return "child-flow"
	case "step":
		return "step"
	case "loop":
		return "loop"
	case "fanout":
		return "fanout"
	case "branch":
		return "branch"
	default:
		if kind == "" {
			return "activity"
		}
		return strings.ReplaceAll(kind, "_", " ")
	}
}

func activityText(activity Activity) string {
	return activityTextWithBadge(activity, false)
}

func activityTextStyled(activity Activity, opts RenderOptions) string {
	if !opts.Color {
		return activityText(activity)
	}
	return activityTextWithBadge(activity, true)
}

func activityTextWithBadge(activity Activity, styled bool) string {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	if kind == "resource" {
		if styled {
			return color(resourceText(activity), activityKindColor(activity), true)
		}
		return resourceText(activity)
	}

	labelText := activityLabel(activity)
	label := shortLabel(labelText, 42)
	mark := activityTypeMark(activity, styled)
	detail := activityDetailText(activity)
	if stepIDDetail := activityStepIDDetail(activity, labelText); stepIDDetail != "" {
		detail = strings.Join(compactParts(stepIDDetail, detail), " ")
	}
	if styled && detail != "" {
		detail = dim(detail, true)
	}
	return strings.Join(compactParts(mark, label, detail), " ")
}

func activityTypeMark(activity Activity, enabled bool) string {
	return typeMark(activityTypeMarkLabel(activity), activityKindColor(activity), enabled)
}

func activityTypeMarkLabel(activity Activity) string {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	switch kind {
	case "step":
		return stepTypeMarkLabel(activity)
	case "activity":
		return stepTypeMarkLabel(activity)
	case "tool_call":
		return "tool"
	case "mcp_tool_call":
		return "m"
	case "mcp_session":
		return "m"
	case "child_flow":
		return "flow"
	case "loop":
		return "loop"
	case "fanout":
		return "fanout"
	case "branch":
		return "branch"
	default:
		kindLabel := activityKindLabel(activity)
		if kindLabel == "activity" {
			return "s"
		}
		return firstTypeLetter(kindLabel)
	}
}

func stepTypeMarkLabel(activity Activity) string {
	stepType := strings.ToLower(strings.Trim(strings.TrimSpace(activity.ActivityType), ":"))
	switch {
	case stepType == "":
		return "s"
	case stepType == "fanout":
		return "fanout"
	case stepType == "loop":
		return "loop"
	case stepType == "sleep":
		return "s"
	case stepType == "function" || stepType == "code":
		return "flow"
	case stepType == "child-flow" || stepType == "call-flow":
		return "flow"
	case stepType == "mcp" || stepType == "mcp-session":
		return "m"
	case strings.Contains(stepType, "/") || stepType == "agent":
		return "agent"
	default:
		return firstTypeLetter(stepType)
	}
}

func activityDetailText(activity Activity) string {
	if isToolActivity(activity) {
		return activityTypeText(activity)
	}
	if strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "step") && stepTypeMarkLabel(activity) == "agent" {
		return compactActivityType(activity)
	}
	return ""
}

func activityStepIDDetail(activity Activity, label string) string {
	if isToolActivity(activity) {
		return ""
	}
	stepID := strings.TrimSpace(activity.StepID)
	if stepID == "" && strings.HasPrefix(strings.TrimSpace(activity.ActivityID), "step:") {
		stepID = strings.TrimPrefix(strings.TrimSpace(activity.ActivityID), "step:")
	}
	if stepID == "" {
		return ""
	}
	if strings.Contains(stepID, "/") {
		return ""
	}
	if strings.EqualFold(stepID, strings.TrimSpace(label)) {
		return ""
	}
	return "[" + stepID + "]"
}

func typeMark(label string, code string, enabled bool) string {
	return color(typeMarkSymbol(label), code, enabled)
}

func typeMarkSymbol(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "flow", "function", "code", "child_flow", "child-flow", "call-flow", "f", "c":
		return "ƒ"
	case "agent", "a":
		return "◉"
	case "tool", "tool_call", "tool-call", "t":
		return "⚙"
	case "fanout", "o":
		return "✣"
	case "loop", "l":
		return "↻"
	case "branch":
		return "◇"
	case "resource", "blob", "b":
		return "▣"
	default:
		return strings.TrimSpace(label)
	}
}

func firstTypeLetter(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "s"
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return strings.ToLower(string(r))
		}
	}
	return "s"
}

func activityTypeColor(stepType string) string {
	switch strings.ToLower(strings.TrimSpace(stepType)) {
	case "flow":
		return "1;36"
	case "fanout":
		return "33"
	case "loop":
		return "35"
	case "branch":
		return "33"
	case "agent":
		return "36"
	case "sleep":
		return "2"
	case "function", "fn":
		return "34"
	case "tool":
		return "34"
	case "mcp":
		return "36"
	case "child":
		return "34"
	case "resource":
		return "36"
	default:
		return "2"
	}
}

func activityKindColor(activity Activity) string {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	if kind == "step" || kind == "activity" {
		switch strings.ToLower(strings.TrimSpace(activity.ActivityType)) {
		case "fanout":
			return activityTypeColor("fanout")
		case "loop":
			return activityTypeColor("loop")
		case "agent":
			return activityTypeColor("agent")
		case "sleep":
			return activityTypeColor("sleep")
		case "function":
			return activityTypeColor("function")
		}
		if strings.Contains(activity.ActivityType, "/") {
			return activityTypeColor("agent")
		}
	}
	switch kind {
	case "tool_call":
		return activityTypeColor("tool")
	case "mcp_tool_call", "mcp_session":
		return activityTypeColor("mcp")
	case "fanout":
		return activityTypeColor("fanout")
	case "loop":
		return activityTypeColor("loop")
	case "branch":
		return activityTypeColor("branch")
	case "child_flow":
		return activityTypeColor("child")
	case "resource":
		return activityTypeColor("resource")
	default:
		return activityTypeColor("")
	}
}

func resourceText(activity Activity) string {
	return strings.Join(compactParts(resourceLabelText(activity), resourceKindText(activity)), " ")
}

func resourceLabelText(activity Activity) string {
	if isFlowErrorResource(activity) {
		return "flow error"
	}
	if isRunResultResourceActivity(activity) {
		return "flow result"
	}
	label := firstNonBlank(activity.ResourceLabel, activity.ActivityName, resourceNameFromURI(activity.ResourceURI), activity.ActivityID, "resource")
	return shortLabel(label, 36)
}

func resourceKindText(activity Activity) string {
	return firstNonBlank(activity.ResourceKind, activity.ActivityType, "resource")
}

func resourceKindMark(activity Activity, enabled bool) string {
	return typeMark(resourceKindMarkLabel(resourceKindText(activity)), resourceMarkColor(activity), enabled)
}

func resourceKindMarkForKind(kind string, enabled bool) string {
	return typeMark(resourceKindMarkLabel(kind), activityTypeColor("resource"), enabled)
}

func resourceKindMarkLabel(kind string) string {
	return "resource"
}

func resourceMarkColor(activity Activity) string {
	if isFlowErrorResource(activity) {
		return colorForStatus("failed")
	}
	return activityTypeColor("resource")
}

func isFlowErrorResource(activity Activity) bool {
	text := strings.ToLower(strings.Join(compactParts(
		activity.ResourceLabel,
		activity.ActivityName,
		activity.ResourceURI,
		activity.ActivityID,
	), " "))
	return strings.Contains(text, "flow error") || strings.Contains(text, "flow-error")
}

func isRunResultResourceActivity(activity Activity) bool {
	if strings.EqualFold(strings.TrimSpace(activity.ResourceKind), "run-result") ||
		strings.EqualFold(strings.TrimSpace(activity.ActivityType), "run-result") {
		return true
	}
	return runResultResourceURIMatches(activity.ResourceURI, activity.WorkflowID) ||
		runResultResourceURIMatches(activity.ActivityID, activity.WorkflowID)
}

func resourceDetailText(activity Activity) string {
	parts := []string{}
	if activity.Planned && !isRunResultResourceActivity(activity) {
		parts = append(parts, resourceKindText(activity))
	}
	if activity.RowCount != nil && *activity.RowCount > 0 {
		parts = append(parts, fmt.Sprintf("%d rows", *activity.RowCount))
	}
	if activity.SizeBytes != nil && *activity.SizeBytes > 0 {
		parts = append(parts, formatBytes(*activity.SizeBytes))
	}
	if activity.ContentType != "" && !strings.Contains(strings.ToLower(activity.ContentType), "json") {
		parts = append(parts, shortLabel(activity.ContentType, 28))
	}
	return strings.Join(parts, " ")
}

func resourceNameFromURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(uri, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.TrimSpace(parts[i]) != "" {
			return parts[i]
		}
	}
	return uri
}

func appendKindIfMissing(label string, kind string) string {
	if strings.Contains(strings.ToLower(label), strings.ToLower(kind)) {
		return label
	}
	return strings.Join(compactParts(label, kind), " ")
}

func compactActivityType(activity Activity) string {
	if activity.ActivityType == "" ||
		activity.ActivityType == activity.ActivityKind ||
		strings.EqualFold(activity.ActivityType, activity.ActivityName) ||
		strings.EqualFold(activity.ActivityType, activity.StepID) {
		return ""
	}
	return activity.ActivityType
}

func activityTypeText(activity Activity) string {
	if activity.ActivityType == "" || activity.ActivityType == activity.ActivityKind {
		return ""
	}
	if isToolActivity(activity) {
		label := activityLabel(activity)
		if strings.EqualFold(activity.ActivityType, label) ||
			strings.EqualFold(activity.ActivityType, activity.ActivityName) ||
			strings.EqualFold(activity.ActivityType, activity.ToolCallID) {
			return ""
		}
		return "(" + activity.ActivityType + ")"
	}
	return activity.ActivityType
}

func isToolActivity(activity Activity) bool {
	kind := strings.ToLower(strings.TrimSpace(activity.ActivityKind))
	return kind == "tool" || kind == "tool_call" || kind == "mcp_tool_call"
}

func isChildFlowActivity(activity Activity) bool {
	return strings.EqualFold(strings.TrimSpace(activity.ActivityKind), "child_flow")
}

func activityLabel(activity Activity) string {
	return firstNonBlank(
		activity.ActivityName,
		activity.StepID,
		activity.ToolCallID,
		activity.MCPSessionID,
		activity.ActivityType,
		activity.ActivityID,
		"activity",
	)
}

func activityDuration(activity Activity, now time.Time) string {
	if activity.StartedAt == nil || activity.StartedAt.IsZero() {
		return ""
	}
	duration := durationSince(now, *activity.StartedAt, activity.CompletedAt)
	if isToolActivity(activity) && duration == 0 {
		return ""
	}
	return formatDuration(duration)
}

func activityProgress(activity Activity) string {
	if activity.ProgressCurrent == nil {
		return ""
	}
	if activity.ProgressTotal != nil && *activity.ProgressTotal > 0 {
		return fmt.Sprintf("iter %d/%d", *activity.ProgressCurrent, *activity.ProgressTotal)
	}
	return fmt.Sprintf("iter %d", *activity.ProgressCurrent)
}

func stepCounts(run RunState) string {
	total := run.StepsStarted
	if run.StepsExecutedTotal > total {
		total = run.StepsExecutedTotal
	}
	if total == 0 && run.StepsCompleted == 0 && run.StepsFailed == 0 && run.StepsRunning == 0 {
		return ""
	}
	parts := []string{fmt.Sprintf("%d/%d", run.StepsCompleted, total)}
	if run.StepsFailed > 0 {
		parts = append(parts, fmt.Sprintf("%df", run.StepsFailed))
	}
	return strings.Join(parts, " ")
}

func compactStatusText(status string) string {
	switch normalizeStatus(status, false) {
	case "failed", "error":
		return "failed"
	case "cancelled", "canceled":
		return "cancelled"
	case "waiting", "pending":
		return "waiting"
	case "syncing":
		return "syncing"
	default:
		return ""
	}
}

func compactParts(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, strings.TrimSpace(value))
		}
	}
	return parts
}

func glyph(status string, active bool, frame int) string {
	if active || status == "running" || status == "syncing" {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		if frame < 0 {
			frame = 0
		}
		return frames[frame%len(frames)]
	}
	switch status {
	case "completed", "succeeded", "success":
		return ""
	case "failed", "error":
		return ""
	case "cancelled", "canceled":
		return ""
	case "waiting", "pending":
		return "○"
	default:
		return "•"
	}
}

func coloredGlyph(status string, active bool, frame int, enabled bool) string {
	value := glyph(status, active, frame)
	if value == "" {
		return ""
	}
	return color(value, colorForStatus(normalizeStatus(status, active)), enabled)
}

func coloredActivityGlyphSlot(status string, active bool, frame int, enabled bool) string {
	value := coloredGlyph(status, active, frame, enabled)
	if value == "" {
		return "  "
	}
	return value + " "
}

func failedActivityStatusText(status string) string {
	switch status {
	case "failed", "error":
		return "failed"
	default:
		return ""
	}
}

func statusBadge(status string, enabled bool) string {
	return color(status, colorForStatus(status), enabled)
}

func colorForStatus(status string) string {
	switch status {
	case "completed", "succeeded", "success":
		return "32"
	case "failed", "error":
		return "31"
	case "cancelled", "canceled":
		return "33"
	case "waiting", "pending":
		return "36"
	case "running":
		return "36"
	case "syncing":
		return "36"
	default:
		return "37"
	}
}

func color(value string, code string, enabled bool) string {
	if !enabled || strings.TrimSpace(code) == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func dim(value string, enabled bool) string {
	return color(value, "2", enabled)
}

func gray(value string, enabled bool) string {
	return color(value, "90", enabled)
}

func normalizeStatus(status string, active bool) string {
	status = strings.ToLower(strings.TrimSpace(status))
	status = strings.ReplaceAll(status, "_", "-")
	if status == "" && active {
		return "running"
	}
	if status == "" {
		return "unknown"
	}
	return status
}

func durationSince(now time.Time, start time.Time, completed *time.Time) time.Duration {
	end := now
	if completed != nil && !completed.IsZero() {
		end = *completed
	}
	if end.Before(start) {
		return 0
	}
	return end.Sub(start)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d / time.Minute)
		seconds := int((d % time.Minute) / time.Second)
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%dh%02dm", hours, minutes)
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	kib := float64(bytes) / 1024
	if kib < 1024 {
		return fmt.Sprintf("%.1fKB", kib)
	}
	mib := kib / 1024
	if mib < 1024 {
		return fmt.Sprintf("%.1fMB", mib)
	}
	return fmt.Sprintf("%.1fGB", mib/1024)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func shortID(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 48 {
		return value
	}
	return string(runes[:36]) + "…" + string(runes[len(runes)-11:])
}

func shortLabel(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
