package live

import (
	"sort"
	"strings"
	"time"
)

type Snapshot struct {
	Workspace  WorkspaceSummary    `json:"workspace"`
	Runs       []RunState          `json:"runs"`
	Relations  []RunRelation       `json:"relations"`
	Nodes      []Activity          `json:"nodes"`
	FlowGraphs []FlowGraphDocument `json:"flowGraphs,omitempty"`
}

type WorkspaceSummary struct {
	WorkspaceID         string    `json:"workspace_id"`
	ActiveRunCount      int       `json:"active_run_count"`
	ActiveChildRunCount int       `json:"active_child_run_count"`
	StepsRunning        int64     `json:"steps_running"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type RunState struct {
	WorkspaceID        string     `json:"workspace_id"`
	WorkflowID         string     `json:"workflow_id"`
	RootWorkflowID     string     `json:"root_workflow_id"`
	ParentWorkflowID   string     `json:"parent_workflow_id,omitempty"`
	ParentStepID       string     `json:"parent_step_id,omitempty"`
	RelationKind       string     `json:"relation_kind,omitempty"`
	FlowSlug           string     `json:"flow_slug,omitempty"`
	Status             string     `json:"status,omitempty"`
	EntrypointType     string     `json:"entrypoint_type,omitempty"`
	AgentID            string     `json:"agent_id,omitempty"`
	FanoutID           string     `json:"fanout_id,omitempty"`
	FanoutBranchIndex  *int       `json:"fanout_branch_index,omitempty"`
	Active             bool       `json:"active"`
	CurrentStepID      string     `json:"current_step_id,omitempty"`
	CurrentStepName    string     `json:"current_step_name,omitempty"`
	CurrentStepType    string     `json:"current_step_type,omitempty"`
	CurrentStepStatus  string     `json:"current_step_status,omitempty"`
	StepsStarted       int64      `json:"steps_started"`
	StepsCompleted     int64      `json:"steps_completed"`
	StepsFailed        int64      `json:"steps_failed"`
	StepsExecutedTotal int64      `json:"steps_executed_total"`
	StepsRunning       int64      `json:"steps_running"`
	LastSequence       int64      `json:"last_sequence"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	LastEventAt        time.Time  `json:"last_event_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type RunRelation struct {
	WorkspaceID       string    `json:"workspace_id"`
	RootWorkflowID    string    `json:"root_workflow_id"`
	ParentWorkflowID  string    `json:"parent_workflow_id"`
	ChildWorkflowID   string    `json:"child_workflow_id"`
	ParentStepID      string    `json:"parent_step_id,omitempty"`
	RelationKind      string    `json:"relation_kind,omitempty"`
	FlowSlug          string    `json:"flow_slug,omitempty"`
	EntrypointType    string    `json:"entrypoint_type,omitempty"`
	AgentID           string    `json:"agent_id,omitempty"`
	FanoutID          string    `json:"fanout_id,omitempty"`
	FanoutBranchIndex *int      `json:"fanout_branch_index,omitempty"`
	Active            bool      `json:"active"`
	Status            string    `json:"status,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Activity struct {
	WorkspaceID      string     `json:"workspace_id"`
	ActivityID       string     `json:"node_id"`
	ParentActivityID string     `json:"parent_node_id,omitempty"`
	ActivityKind     string     `json:"node_kind"`
	ActivityType     string     `json:"node_type,omitempty"`
	ActivityName     string     `json:"node_name,omitempty"`
	Status           string     `json:"status"`
	Active           bool       `json:"active"`
	WorkflowID       string     `json:"workflow_id,omitempty"`
	RootWorkflowID   string     `json:"root_workflow_id,omitempty"`
	ParentWorkflowID string     `json:"parent_workflow_id,omitempty"`
	ParentStepID     string     `json:"parent_step_id,omitempty"`
	RelationKind     string     `json:"relation_kind,omitempty"`
	StepID           string     `json:"step_id,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	MCPSessionID     string     `json:"mcp_session_id,omitempty"`
	Attempt          *int64     `json:"attempt,omitempty"`
	AgentID          string     `json:"agent_id,omitempty"`
	ProgressCurrent  *int64     `json:"progress_current,omitempty"`
	ProgressTotal    *int64     `json:"progress_total,omitempty"`
	ResourceURI      string     `json:"resource_uri,omitempty"`
	ResourceKind     string     `json:"resource_kind,omitempty"`
	ResourceLabel    string     `json:"resource_label,omitempty"`
	ContentType      string     `json:"content_type,omitempty"`
	SizeBytes        *int64     `json:"size_bytes,omitempty"`
	RowCount         *int64     `json:"row_count,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
	Planned          bool       `json:"-"`
	GraphOrder       int        `json:"-"`
	GraphScopeID     string     `json:"-"`
}

type FlowGraphDocument struct {
	WorkflowID string    `json:"workflowId"`
	FlowSlug   string    `json:"flowSlug,omitempty"`
	Version    int       `json:"version,omitempty"`
	Source     string    `json:"source,omitempty"`
	Warning    *Warning  `json:"warning,omitempty"`
	Graph      FlowGraph `json:"graph"`
}

type Warning struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type FlowGraph struct {
	SchemaVersion int              `json:"schemaVersion"`
	RootID        string           `json:"rootId"`
	Nodes         []FlowGraphNode  `json:"nodes"`
	Edges         []FlowGraphEdge  `json:"edges,omitempty"`
	Scopes        []FlowGraphScope `json:"scopes,omitempty"`
}

type FlowGraphNode struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	Label         string         `json:"label,omitempty"`
	StepID        string         `json:"stepId,omitempty"`
	StepType      string         `json:"stepType,omitempty"`
	ParentID      string         `json:"parentId,omitempty"`
	ScopeID       string         `json:"scopeId,omitempty"`
	Order         int            `json:"order,omitempty"`
	Match         map[string]any `json:"match,omitempty"`
	FlowSlug      string         `json:"flowSlug,omitempty"`
	Version       int            `json:"version,omitempty"`
	AgentID       string         `json:"agentId,omitempty"`
	FanoutIndex   *int           `json:"fanoutIndex,omitempty"`
	LoopType      string         `json:"loopType,omitempty"`
	BranchType    string         `json:"branchType,omitempty"`
	MaxIterations *int           `json:"maxIterations,omitempty"`
	HasPersist    bool           `json:"hasPersist,omitempty"`
	PersistKind   string         `json:"persistKind,omitempty"`
	PersistType   string         `json:"persistType,omitempty"`
	PersistMIME   string         `json:"persistContentType,omitempty"`
}

type FlowGraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind,omitempty"`
}

type FlowGraphScope struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Label          string `json:"label,omitempty"`
	OwnerNodeID    string `json:"ownerNodeId,omitempty"`
	ParentID       string `json:"parentId,omitempty"`
	BranchIndex    *int   `json:"branchIndex,omitempty"`
	MaxConcurrency *int   `json:"maxConcurrency,omitempty"`
}

type RunNode struct {
	Run        RunState
	Relation   *RunRelation
	Activities []Activity
	Children   []RunNode
	MissingRun bool
}

func (s Snapshot) Focus(workflowID string) Snapshot {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return s
	}

	rootID := workflowID
	for _, run := range s.Runs {
		if run.WorkflowID == workflowID && strings.TrimSpace(run.RootWorkflowID) != "" {
			rootID = run.RootWorkflowID
			break
		}
	}

	includedRuns := map[string]bool{}
	runs := make([]RunState, 0, len(s.Runs))
	for _, run := range s.Runs {
		if run.WorkflowID == workflowID || run.RootWorkflowID == rootID {
			runs = append(runs, run)
			includedRuns[run.WorkflowID] = true
		}
	}
	if len(runs) == 0 {
		s.Runs = nil
		s.Relations = nil
		s.Nodes = nil
		s.Workspace = focusedWorkspaceSummary(s.Workspace, nil, nil)
		return s
	}

	relations := make([]RunRelation, 0, len(s.Relations))
	for _, relation := range s.Relations {
		if relation.RootWorkflowID == rootID ||
			includedRuns[relation.ParentWorkflowID] ||
			includedRuns[relation.ChildWorkflowID] {
			relations = append(relations, relation)
		}
	}

	activities := make([]Activity, 0, len(s.Nodes))
	for _, activity := range s.Nodes {
		if activity.RootWorkflowID == rootID ||
			includedRuns[activity.WorkflowID] ||
			activity.WorkflowID == workflowID {
			activities = append(activities, activity)
		}
	}

	s.Runs = runs
	s.Relations = relations
	s.Nodes = activities
	if len(s.FlowGraphs) > 0 {
		graphs := make([]FlowGraphDocument, 0, len(s.FlowGraphs))
		for _, graph := range s.FlowGraphs {
			if graph.WorkflowID == "" || includedRuns[graph.WorkflowID] || graph.WorkflowID == workflowID {
				graphs = append(graphs, graph)
			}
		}
		s.FlowGraphs = graphs
	}
	s.Workspace = focusedWorkspaceSummary(s.Workspace, runs, activities)
	return s
}

func focusedWorkspaceSummary(base WorkspaceSummary, runs []RunState, activities []Activity) WorkspaceSummary {
	summary := base
	summary.ActiveRunCount = 0
	summary.ActiveChildRunCount = 0
	summary.StepsRunning = 0

	updatedAt := summary.UpdatedAt
	for _, run := range runs {
		status := strings.ToLower(strings.TrimSpace(run.Status))
		if run.Active || run.StepsRunning > 0 || status == "running" || status == "syncing" {
			summary.ActiveRunCount++
			if strings.TrimSpace(run.ParentWorkflowID) != "" {
				summary.ActiveChildRunCount++
			}
		}
		summary.StepsRunning += run.StepsRunning
		if run.UpdatedAt.After(updatedAt) {
			updatedAt = run.UpdatedAt
		}
	}
	for _, activity := range activities {
		if activity.UpdatedAt.After(updatedAt) {
			updatedAt = activity.UpdatedAt
		}
	}
	summary.UpdatedAt = updatedAt
	return summary
}

func (s Snapshot) BuildRunTree() []RunNode {
	runsByWorkflow := map[string]RunState{}
	for _, run := range s.Runs {
		if strings.TrimSpace(run.WorkflowID) == "" {
			continue
		}
		if strings.TrimSpace(run.RootWorkflowID) == "" {
			run.RootWorkflowID = run.WorkflowID
		}
		runsByWorkflow[run.WorkflowID] = run
	}

	relationsByParent := map[string][]RunRelation{}
	childWorkflowIDs := map[string]bool{}
	for _, relation := range s.Relations {
		parentID := strings.TrimSpace(relation.ParentWorkflowID)
		childID := strings.TrimSpace(relation.ChildWorkflowID)
		if parentID == "" || childID == "" {
			continue
		}
		relationsByParent[parentID] = append(relationsByParent[parentID], relation)
		childWorkflowIDs[childID] = true
	}
	for parentID := range relationsByParent {
		sort.SliceStable(relationsByParent[parentID], func(i, j int) bool {
			return relationLess(relationsByParent[parentID][i], relationsByParent[parentID][j])
		})
	}

	activitiesByWorkflow := map[string][]Activity{}
	for _, activity := range s.Nodes {
		workflowID := strings.TrimSpace(activity.WorkflowID)
		if workflowID == "" {
			continue
		}
		activitiesByWorkflow[workflowID] = append(activitiesByWorkflow[workflowID], activity)
	}
	for workflowID := range activitiesByWorkflow {
		sort.SliceStable(activitiesByWorkflow[workflowID], func(i, j int) bool {
			return activityTime(activitiesByWorkflow[workflowID][i]).Before(activityTime(activitiesByWorkflow[workflowID][j]))
		})
	}

	roots := make([]RunState, 0, len(s.Runs))
	for _, run := range runsByWorkflow {
		if childWorkflowIDs[run.WorkflowID] {
			continue
		}
		if strings.TrimSpace(run.ParentWorkflowID) != "" {
			continue
		}
		roots = append(roots, run)
	}
	sort.SliceStable(roots, func(i, j int) bool {
		left := runSortTime(roots[i])
		right := runSortTime(roots[j])
		if left.Equal(right) {
			return roots[i].WorkflowID < roots[j].WorkflowID
		}
		return left.Before(right)
	})

	nodes := make([]RunNode, 0, len(roots))
	for _, root := range roots {
		nodes = append(nodes, buildRunNode(root, nil, runsByWorkflow, relationsByParent, activitiesByWorkflow))
	}
	return nodes
}

func buildRunNode(
	run RunState,
	relation *RunRelation,
	runsByWorkflow map[string]RunState,
	relationsByParent map[string][]RunRelation,
	activitiesByWorkflow map[string][]Activity,
) RunNode {
	node := RunNode{
		Run:        run,
		Relation:   relation,
		Activities: append([]Activity(nil), activitiesByWorkflow[run.WorkflowID]...),
	}
	for _, childRelation := range relationsByParent[run.WorkflowID] {
		childRun, ok := runsByWorkflow[childRelation.ChildWorkflowID]
		if !ok {
			childRun = RunState{
				WorkspaceID:       childRelation.WorkspaceID,
				WorkflowID:        childRelation.ChildWorkflowID,
				RootWorkflowID:    childRelation.RootWorkflowID,
				ParentWorkflowID:  childRelation.ParentWorkflowID,
				ParentStepID:      childRelation.ParentStepID,
				RelationKind:      childRelation.RelationKind,
				FlowSlug:          childRelation.FlowSlug,
				Status:            childRelation.Status,
				EntrypointType:    childRelation.EntrypointType,
				AgentID:           childRelation.AgentID,
				FanoutID:          childRelation.FanoutID,
				FanoutBranchIndex: childRelation.FanoutBranchIndex,
				Active:            childRelation.Active,
				LastEventAt:       childRelation.UpdatedAt,
				UpdatedAt:         childRelation.UpdatedAt,
			}
		}
		relationCopy := childRelation
		child := buildRunNode(childRun, &relationCopy, runsByWorkflow, relationsByParent, activitiesByWorkflow)
		child.MissingRun = !ok
		node.Children = append(node.Children, child)
	}
	return node
}

func relationLess(left, right RunRelation) bool {
	if left.FanoutBranchIndex != nil && right.FanoutBranchIndex != nil && *left.FanoutBranchIndex != *right.FanoutBranchIndex {
		return *left.FanoutBranchIndex < *right.FanoutBranchIndex
	}
	if left.FanoutBranchIndex != nil && right.FanoutBranchIndex == nil {
		return true
	}
	if left.FanoutBranchIndex == nil && right.FanoutBranchIndex != nil {
		return false
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.Before(right.CreatedAt)
	}
	return left.ChildWorkflowID < right.ChildWorkflowID
}

func activityTime(activity Activity) time.Time {
	if activity.StartedAt != nil && !activity.StartedAt.IsZero() {
		return *activity.StartedAt
	}
	if !activity.UpdatedAt.IsZero() {
		return activity.UpdatedAt
	}
	return time.Time{}
}

func runSortTime(run RunState) time.Time {
	if run.StartedAt != nil && !run.StartedAt.IsZero() {
		return *run.StartedAt
	}
	if !run.LastEventAt.IsZero() {
		return run.LastEventAt
	}
	if !run.UpdatedAt.IsZero() {
		return run.UpdatedAt
	}
	return time.Time{}
}
