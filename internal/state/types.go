package state

import "time"

type State struct {
        Version    int                   `json:"version"`
        Workspaces map[string]*Workspace `json:"workspaces"`
        Tick       int64                 `json:"tick"`
}

type Workspace struct {
        ID        string           `json:"id"`
        Name      string           `json:"name"`
        Plan      string           `json:"plan"`
        Owner     string           `json:"owner"`
        UpdatedAt time.Time        `json:"updatedAt"`
        Flows     map[string]*Flow `json:"flows"`
        Runs      map[string]*Run  `json:"runs"`

        // Marketplace-ish mock data
        RevenueEvents []RevenueEvent `json:"revenueEvents"`
        DemandTop     []DemandItem   `json:"demandTop"`

        // v1 CLI resources (mocked)
        Connections map[string]*Connection `json:"connections"`
        Instances   map[string]*Instance   `json:"instances"`
        Triggers    map[string]*Trigger    `json:"triggers"`
        Waits       map[string]*Wait       `json:"waits"`
}

type Flow struct {
        Slug          string     `json:"slug"`
        Name          string     `json:"name"`
        Description   string     `json:"description"`
        Tags          []string   `json:"tags"`
        ActiveVersion int        `json:"activeVersion"`
        UpdatedAt     time.Time  `json:"updatedAt"`
        Spine         []string   `json:"spine"`
        Steps         []FlowStep `json:"steps"`

        // Published, immutable versions (mock). Draft is the current Flow record.
        Versions []FlowVersion `json:"versions,omitempty"`
}

type FlowVersion struct {
        Version     int        `json:"version"`
        PublishedAt time.Time  `json:"publishedAt"`
        Note        string     `json:"note,omitempty"`
        Flow        FlowRecord `json:"flow"`
}

type FlowRecord struct {
        Name        string     `json:"name"`
        Description string     `json:"description"`
        Tags        []string   `json:"tags"`
        Spine       []string   `json:"spine"`
        Steps       []FlowStep `json:"steps"`
}

type FlowStep struct {
        ID           string `json:"id"`
        Type         string `json:"type"`
        Title        string `json:"title"`
        InputSchema  string `json:"inputSchema"`
        OutputSchema string `json:"outputSchema"`
        Definition   string `json:"definition"`
}

type Run struct {
        WorkflowID    string          `json:"workflowId"`
        FlowSlug      string          `json:"flowSlug"`
        Version       int             `json:"version"`
        Status        string          `json:"status"`
        TriggeredBy   string          `json:"triggeredBy"`
        StartedAt     time.Time       `json:"startedAt"`
        UpdatedAt     time.Time       `json:"updatedAt"`
        CompletedAt   *time.Time      `json:"completedAt,omitempty"`
        CurrentStep   string          `json:"currentStep"`
        InputPreview  any             `json:"inputPreview,omitempty"`
        ResultPreview any             `json:"resultPreview,omitempty"`
        Error         string          `json:"error,omitempty"`
        Steps         []StepExecution `json:"steps"`
}

type RevenueEvent struct {
        At          time.Time `json:"at"`
        Currency    string    `json:"currency"`
        AmountCents int64     `json:"amountCents"`
        Source      string    `json:"source"` // e.g. "flow-run", "subscription"
        FlowSlug    string    `json:"flowSlug"`
        RunID       string    `json:"runId"`
}

type DemandItem struct {
        Query          string   `json:"query"`
        Count          int      `json:"count"`
        Window         string   `json:"window"` // e.g. "30d"
        SuggestedPrice string   `json:"suggestedPrice"`
        MatchedFlows   []string `json:"matchedFlows"`
}

type StepExecution struct {
        StepID        string     `json:"stepId"`
        StepType      string     `json:"stepType"`
        Title         string     `json:"title"`
        Status        string     `json:"status"`
        Attempt       int        `json:"attempt"`
        StartedAt     time.Time  `json:"startedAt"`
        CompletedAt   *time.Time `json:"completedAt,omitempty"`
        DurationMs    int64      `json:"durationMs"`
        InputPreview  any        `json:"inputPreview,omitempty"`
        ResultPreview any        `json:"resultPreview,omitempty"`
        Error         string     `json:"error,omitempty"`
}

type Connection struct {
        ID        string    `json:"id"`
        Name      string    `json:"name"`
        Type      string    `json:"type"` // e.g. slack, stripe
        Status    string    `json:"status"`
        UpdatedAt time.Time `json:"updatedAt"`
        Config    any       `json:"config,omitempty"`
}

type Trigger struct {
        ID        string    `json:"id"`
        FlowSlug  string    `json:"flowSlug"`
        Type      string    `json:"type"` // schedule, webhook, manual
        Name      string    `json:"name"`
        Enabled   bool      `json:"enabled"`
        UpdatedAt time.Time `json:"updatedAt"`
        Config    any       `json:"config,omitempty"`
}

type Instance struct {
        ID          string    `json:"id"`
        FlowSlug    string    `json:"flowSlug"`
        Version     int       `json:"version"`
        Name        string    `json:"name"`
        Enabled     bool      `json:"enabled"`
        AutoUpgrade bool      `json:"autoUpgrade"`
        UpdatedAt   time.Time `json:"updatedAt"`
        Bindings    any       `json:"bindings,omitempty"`
}

type Wait struct {
        ID        string    `json:"id"`
        RunID     string    `json:"runId"`
        StepID    string    `json:"stepId"`
        Type      string    `json:"type"` // input, secret, approve
        Status    string    `json:"status"`
        Prompt    string    `json:"prompt"`
        CreatedAt time.Time `json:"createdAt"`
        Payload   any       `json:"payload,omitempty"`
}
