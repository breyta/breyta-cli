package live

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var displayANSIRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// DisplayLine is one rendered row in the live terminal tree.
type DisplayLine struct {
	Key          string
	Text         string
	Planned      bool
	WorkspaceID  string
	WorkflowID   string
	FlowSlug     string
	ResourceURI  string
	ResourceKind string
	WebURL       string
	// Live marks rows that change between frames (spinners, durations, active scopes).
	Live bool
}

// DisplayFrame is the visible tree in walk order (header, run, steps, children, ...).
type DisplayFrame struct {
	Lines []DisplayLine
}

// CollectDisplayFrame builds the current tree in display order.
func CollectDisplayFrame(snapshot Snapshot, opts RenderOptions) DisplayFrame {
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
	frame := DisplayFrame{}

	nodes := snapshot.BuildRunTree()
	flatRoot := flatRootRunNode(nodes, opts)
	frame.add(renderHeaderLine(snapshot, opts, activeWork, flatRoot), activeWork)

	if len(nodes) == 0 {
		frame.add(DisplayLine{
			Key:  "waiting",
			Text: dim("Waiting for run updates...", opts.Color),
			Live: true,
		}, true)
		return frame
	}
	renderNodes := nodes
	if root := focusRootRunNode(nodes, opts); root != nil {
		renderNodes = []RunNode{*root}
	}
	for i, node := range renderNodes {
		skipRunLine := flatRoot != nil && strings.TrimSpace(node.Run.WorkflowID) == strings.TrimSpace(flatRoot.Run.WorkflowID)
		rootFlowSlug := strings.TrimSpace(node.Run.FlowSlug)
		collectRunNode(&frame, node, "", i == len(renderNodes)-1, opts, skipRunLine, rootFlowSlug)
	}
	if !snapshotHasActiveWork(snapshot) {
		if summary := runSummaryStrip(snapshot, opts); summary != "" {
			frame.add(DisplayLine{
				Key:  "summary",
				Text: summary,
				Live: false,
			}, false)
		}
	}
	return frame
}

// RenderDisplayFrame formats a display frame as terminal output.
func RenderDisplayFrame(frame DisplayFrame) string {
	return joinDisplayLinesNL(frame.Lines)
}

// ClampDisplayFrameWidth truncates each row to maxCols visible columns so live
// redraw does not wrap one logical line across multiple terminal rows.
func ClampDisplayFrameWidth(frame DisplayFrame, maxCols int) DisplayFrame {
	if maxCols <= 0 || len(frame.Lines) == 0 {
		return frame
	}
	out := frame
	out.Lines = make([]DisplayLine, len(frame.Lines))
	for i, line := range frame.Lines {
		line.Text = truncateDisplayLine(line.Text, maxCols)
		out.Lines[i] = line
	}
	return out
}

func truncateDisplayLine(text string, maxCols int) string {
	plain := displayANSIRe.ReplaceAllString(text, "")
	runes := []rune(plain)
	if len(runes) <= maxCols {
		return text
	}
	if maxCols <= 1 {
		return "…"
	}
	return string(runes[:maxCols-1]) + "…"
}

func (f *DisplayFrame) add(line DisplayLine, live bool) {
	line.Live = live
	f.Lines = append(f.Lines, line)
}

func renderHeaderLine(snapshot Snapshot, opts RenderOptions, activeWork bool, flatRoot *RunNode) DisplayLine {
	headerStatus := "completed"
	if activeWork {
		headerStatus = "running"
	}
	var b strings.Builder
	writeSnapshotHeader(&b, snapshot, opts, activeWork, headerStatus, flatRoot)
	key := "header"
	if flatRoot != nil {
		key = "header:" + strings.TrimSpace(flatRoot.Run.FlowSlug)
	} else if header := strings.TrimSpace(opts.FocusWorkflowID); header != "" {
		key = "header:" + header
	}
	return DisplayLine{Key: key, Text: strings.TrimSuffix(b.String(), "\n")}
}

func collectRunNode(frame *DisplayFrame, node RunNode, prefix string, last bool, opts RenderOptions, skipRunLine bool, rootFlowSlug string) {
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
		runLive := runHasActiveWork(run) || run.Active || node.MissingRun
		frame.add(DisplayLine{
			Key:  "run:" + strings.TrimSpace(run.WorkflowID),
			Text: line,
		}, runLive)
	}

	if !detailed || shouldCollapseRunDetails(node, status, opts) {
		return
	}

	activityPrefix := childPrefix
	if skipRunLine {
		activityPrefix = linePrefix
	}
	renderedCurrentFallback := false
	if runHasActiveWork(run) {
		current := currentStepText(run, node.Activities, opts.Now)
		if current != "" && !hasVisibleCurrentStepActivity(node) {
			activeTool := activeToolText(node, opts.Now)
			if activeTool == "" || !sameActiveTool(current, activeTool) {
				frame.add(DisplayLine{
					Key:  "fallback:" + strings.TrimSpace(run.WorkflowID) + ":current",
					Text: formatCurrentStepFallbackLine(current, activityPrefix, opts),
				}, true)
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
				collectActivityBranch(frame, tool, activityPrefix, opts, run, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, nested := range activityNested {
				collectActivityBranch(frame, nested, activityPrefix, opts, run, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, resource := range activityResources {
				collectResource(frame, resource, activityPrefix, opts, run)
			}
			for childIdx, child := range activityChildren {
				collectRunNode(frame, child, activityPrefix, childIdx == len(activityChildren)-1, opts, false, rootFlowSlug)
			}
			continue
		}
		if shouldElideAgentFanoutEntrypoint(node, displayActivity, activityTools, activityResources, activityChildren) {
			for _, tool := range activityTools {
				collectActivityBranch(frame, suppressRedundantAgentLabel(node, tool), activityPrefix, opts, run, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, nested := range activityNested {
				collectActivityBranch(frame, nested, activityPrefix, opts, run, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, resource := range activityResources {
				collectResource(frame, suppressRedundantAgentLabel(node, resource), activityPrefix, opts, run)
			}
			for childIdx, child := range activityChildren {
				collectRunNode(frame, child, activityPrefix, childIdx == len(activityChildren)-1, opts, false, rootFlowSlug)
			}
			continue
		}
		if shouldInlineActivityContainer(displayActivity, run, activityResources, activityTools, activityChildren) {
			for _, tool := range activityTools {
				collectActivityBranch(frame, tool, activityPrefix, opts, run, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, nested := range activityNested {
				collectActivityBranch(frame, nested, activityPrefix, opts, run, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
			}
			for _, resource := range activityResources {
				collectResource(frame, resource, activityPrefix, opts, run)
			}
			for childIdx, child := range activityChildren {
				collectRunNode(frame, child, activityPrefix, childIdx == len(activityChildren)-1, opts, false, rootFlowSlug)
			}
			continue
		}
		collectActivity(frame, displayActivity, activityPrefix, opts)
		activityChildPrefix := branchChildPrefix(activityPrefix, isLastActivity)
		for _, tool := range activityTools {
			collectActivityBranch(frame, suppressRedundantAgentLabel(node, tool), activityChildPrefix, opts, run, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		}
		for _, nested := range activityNested {
			collectActivityBranch(frame, nested, activityChildPrefix, opts, run, nestedActivitiesByParent, resourcesByParent, toolsByParent, childrenByStep, rootFlowSlug)
		}
		for _, resource := range activityResources {
			collectResource(frame, suppressRedundantAgentLabel(node, resource), activityChildPrefix, opts, run)
		}
		for childIdx, child := range activityChildren {
			collectRunNode(frame, child, activityChildPrefix, childIdx == len(activityChildren)-1, opts, false, rootFlowSlug)
		}
	}

	for i, child := range remainingChildren {
		collectRunNode(frame, child, childPrefix, i == len(remainingChildren)-1, opts, false, rootFlowSlug)
	}
	for _, resource := range runResources {
		collectResource(frame, resource, activityPrefix, opts, run)
	}
}

func collectActivity(frame *DisplayFrame, activity Activity, prefix string, opts RenderOptions) {
	key := activityDisplayKey(activity)
	frame.add(DisplayLine{
		Key:     key,
		Text:    formatActivityLine(activity, prefix, opts),
		Planned: isUnstartedPlannedActivity(activity),
	}, activityLineLive(activity))
}

func collectResource(frame *DisplayFrame, activity Activity, prefix string, opts RenderOptions, run RunState) {
	line := formatResourceLine(activity, prefix, opts)
	key := "resource:" + strings.TrimSpace(activity.WorkflowID) + ":" + firstNonBlank(activity.ActivityID, activity.ResourceURI, activity.ResourceLabel)
	frame.add(DisplayLine{
		Key:          key,
		Text:         line,
		Planned:      isUnstartedPlannedActivity(activity),
		WorkspaceID:  firstNonBlank(activity.WorkspaceID, run.WorkspaceID),
		WorkflowID:   firstNonBlank(activity.WorkflowID, run.WorkflowID),
		FlowSlug:     strings.TrimSpace(run.FlowSlug),
		ResourceURI:  strings.TrimSpace(activity.ResourceURI),
		ResourceKind: strings.TrimSpace(activity.ResourceKind),
	}, activityLineLive(activity))
}

func activityDisplayKey(activity Activity) string {
	workflowID := strings.TrimSpace(activity.WorkflowID)
	stepID := strings.TrimSpace(activity.StepID)
	activityID := strings.TrimSpace(activity.ActivityID)
	if stepID != "" {
		return "activity:" + workflowID + ":" + stepID
	}
	if activityID != "" {
		return "activity:" + workflowID + ":" + activityID
	}
	return "activity:" + workflowID + ":" + strings.TrimSpace(activity.ActivityName)
}

func activityLineLive(activity Activity) bool {
	if activity.Active {
		return true
	}
	status := normalizeStatus(activity.Status, activity.Active)
	if activity.ProgressCurrent != nil && !isTerminalStatus(status) {
		return true
	}
	if isProblemStatus(status) {
		return true
	}
	return false
}

func joinDisplayLines(lines []DisplayLine) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for i, line := range lines {
		b.WriteString(line.Text)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func joinDisplayLinesNL(lines []DisplayLine) string {
	text := joinDisplayLines(lines)
	if text == "" {
		return ""
	}
	return text + "\n"
}

// DisplayFrameKey returns a stable identity for the visible tree shape/content,
// excluding spinner frames and live duration ticks.
func DisplayFrameKey(snapshot Snapshot, opts RenderOptions) string {
	opts.Frame = 0
	if opts.Now.IsZero() {
		opts.Now = time.Unix(0, 0)
	}
	frame := CollectDisplayFrame(snapshot, opts)
	var b strings.Builder
	for _, line := range frame.Lines {
		b.WriteString(line.Key)
		b.WriteByte('|')
		if line.Live {
			b.WriteString("live")
		} else {
			b.WriteString(line.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// FitDisplayFrameForLive caps the visible tree so in-place redraw never writes
// more rows than the terminal can hold. Full-tree mode preserves run rows first
// so fanout branches do not disappear just because step detail is too tall.
func FitDisplayFrameForLive(snapshot Snapshot, opts RenderOptions, maxLines int) DisplayFrame {
	if maxLines <= 0 {
		return CollectDisplayFrame(snapshot, opts)
	}
	frame := CollectDisplayFrame(snapshot, opts)
	if len(frame.Lines) <= maxLines {
		return frame
	}
	if opts.FullTree {
		return compactFullTreeDisplayFrame(frame, maxLines, opts.Color)
	}
	compact := opts
	compact.OmitCompletedSteps = true
	frame = CollectDisplayFrame(snapshot, compact)
	if len(frame.Lines) <= maxLines {
		return frame
	}
	return truncateDisplayFrameTail(frame, maxLines, opts.Color)
}

func compactFullTreeDisplayFrame(frame DisplayFrame, maxLines int, color bool) DisplayFrame {
	if len(frame.Lines) <= maxLines || maxLines <= 0 {
		return frame
	}
	if maxLines == 1 {
		return DisplayFrame{Lines: []DisplayLine{frame.Lines[0]}}
	}
	keep := make([]bool, len(frame.Lines))
	keep[0] = true
	for i, line := range frame.Lines {
		if strings.HasPrefix(line.Key, "run:") || line.Live {
			keep[i] = true
		}
	}
	kept := make([]DisplayLine, 0, maxLines)
	hidden := 0
	for i, line := range frame.Lines {
		if keep[i] {
			kept = append(kept, line)
			continue
		}
		hidden++
	}
	if len(kept) <= maxLines {
		return withHiddenDisplayMarker(kept, hidden, maxLines, color)
	}

	out := make([]DisplayLine, 0, maxLines)
	out = append(out, frame.Lines[0])
	runLines := make([]DisplayLine, 0, len(kept))
	liveLines := make([]DisplayLine, 0, len(kept))
	for _, line := range kept[1:] {
		if strings.HasPrefix(line.Key, "run:") {
			runLines = append(runLines, line)
		} else if line.Live {
			liveLines = append(liveLines, line)
		}
	}
	for _, line := range runLines {
		if len(out) >= maxLines {
			hidden++
			continue
		}
		out = append(out, line)
	}
	for _, line := range liveLines {
		if len(out) >= maxLines {
			hidden++
			continue
		}
		out = append(out, line)
	}
	return withHiddenDisplayMarker(out, hidden, maxLines, color)
}

func withHiddenDisplayMarker(lines []DisplayLine, hidden int, maxLines int, color bool) DisplayFrame {
	if hidden <= 0 || len(lines) >= maxLines {
		if len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		return DisplayFrame{Lines: lines}
	}
	marker := DisplayLine{
		Key:  "truncated",
		Text: dim(fmt.Sprintf("… %d lines hidden …", hidden), color),
		Live: false,
	}
	out := make([]DisplayLine, 0, len(lines)+1)
	if len(lines) == 0 {
		out = append(out, marker)
	} else {
		out = append(out, lines[0])
		out = append(out, marker)
		out = append(out, lines[1:]...)
	}
	if len(out) > maxLines {
		out = out[:maxLines]
	}
	return DisplayFrame{Lines: out}
}

func truncateDisplayFrameTail(frame DisplayFrame, maxLines int, color bool) DisplayFrame {
	if len(frame.Lines) <= maxLines || maxLines <= 0 {
		return frame
	}
	if maxLines == 1 {
		return DisplayFrame{Lines: []DisplayLine{frame.Lines[0]}}
	}
	tailCount := maxLines - 2
	if tailCount < 1 {
		tailCount = 1
	}
	tailStart := len(frame.Lines) - tailCount
	if tailStart < 1 {
		tailStart = 1
	}
	hidden := tailStart - 1
	out := DisplayFrame{Lines: make([]DisplayLine, 0, maxLines)}
	out.Lines = append(out.Lines, frame.Lines[0])
	if hidden > 0 {
		out.Lines = append(out.Lines, DisplayLine{
			Key:  "truncated",
			Text: dim(fmt.Sprintf("… %d lines hidden …", hidden), color),
			Live: false,
		})
	}
	out.Lines = append(out.Lines, frame.Lines[tailStart:]...)
	if len(out.Lines) > maxLines {
		out.Lines = out.Lines[:maxLines]
	}
	return out
}

// CountRenderedLines counts newline-terminated rows in a rendered block.
func CountRenderedLines(s string) int {
	if s == "" {
		return 0
	}
	lines := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		lines++
	}
	return lines
}
