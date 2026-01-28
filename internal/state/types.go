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

	// Marketplace v1 resources (mocked)
	Registry     map[string]*RegistryEntry `json:"registry"`
	Purchases    map[string]*Purchase      `json:"purchases"`
	Entitlements map[string]*Entitlement   `json:"entitlements"`
	Payouts      map[string]*Payout        `json:"payouts"`
	// Demand engine (raw + clustered)
	DemandQueries  []DemandQuery   `json:"demandQueries"`
	DemandClusters []DemandCluster `json:"demandClusters"`

	// v1 CLI resources (mocked)
	Connections map[string]*Connection `json:"connections"`
	Profiles    map[string]*Profile    `json:"profiles"`
	Triggers    map[string]*Trigger    `json:"triggers"`
	Waits       map[string]*Wait       `json:"waits"`
}

type Flow struct {
	Slug          string     `json:"slug"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Tags          []string   `json:"tags"`
	Archived      bool       `json:"archived"`
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

	// Optional: step calls another flow (subflow).
	CallFlowSlug string `json:"callFlowSlug,omitempty"`
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

type Pricing struct {
	Model       string `json:"model"` // per_run | per_success | subscription
	Currency    string `json:"currency"`
	AmountCents int64  `json:"amountCents"`
	Interval    string `json:"interval,omitempty"` // for subscription (month|year)
}

type RegistryStats struct {
	Views        int     `json:"views"`
	Installs     int     `json:"installs"`
	Active       int     `json:"active"`
	SuccessRate  float64 `json:"successRate"`
	Rating       float64 `json:"rating"`
	Reviews      int     `json:"reviews"`
	RevenueCents int64   `json:"revenueCents"`
}

type RegistryVersion struct {
	Version     int       `json:"version"`
	PublishedAt time.Time `json:"publishedAt"`
	Note        string    `json:"note,omitempty"`
	FlowSlug    string    `json:"flowSlug"`
	FlowVersion int       `json:"flowVersion"`
}

// RegistryEntry is the marketplace listing (distribution + monetization metadata).
type RegistryEntry struct {
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	Description string            `json:"description,omitempty"`
	Creator     string            `json:"creator"`
	Category    string            `json:"category,omitempty"`
	Tags        []string          `json:"tags"`
	Pricing     Pricing           `json:"pricing"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	PublishedAt time.Time         `json:"publishedAt"`
	Versions    []RegistryVersion `json:"versions"`
	Stats       RegistryStats     `json:"stats"`
}

type Purchase struct {
	ID          string     `json:"id"`
	ListingID   string     `json:"listingId"`
	Buyer       string     `json:"buyer"`
	Status      string     `json:"status"` // created | paid | cancelled | refunded
	CreatedAt   time.Time  `json:"createdAt"`
	PaidAt      *time.Time `json:"paidAt,omitempty"`
	AmountCents int64      `json:"amountCents"`
	Currency    string     `json:"currency"`
}

type Entitlement struct {
	ID        string         `json:"id"`
	ListingID string         `json:"listingId"`
	Buyer     string         `json:"buyer"`
	Status    string         `json:"status"` // active | expired | revoked
	CreatedAt time.Time      `json:"createdAt"`
	ExpiresAt *time.Time     `json:"expiresAt,omitempty"`
	Limits    map[string]any `json:"limits,omitempty"` // e.g. runs/month
}

type Payout struct {
	ID          string     `json:"id"`
	Creator     string     `json:"creator"`
	Period      string     `json:"period"` // e.g. 2025-12
	AmountCents int64      `json:"amountCents"`
	Currency    string     `json:"currency"`
	Status      string     `json:"status"` // pending | paid
	CreatedAt   time.Time  `json:"createdAt"`
	PaidAt      *time.Time `json:"paidAt,omitempty"`
}

type DemandQuery struct {
	Query        string    `json:"query"`
	At           time.Time `json:"at"`
	Window       string    `json:"window,omitempty"` // e.g. "30d"
	OfferCents   int64     `json:"offerCents,omitempty"`
	Currency     string    `json:"currency,omitempty"`
	NormalizedTo string    `json:"normalizedTo,omitempty"`
}

type DemandCluster struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Count           int      `json:"count"`
	Window          string   `json:"window"`
	Examples        []string `json:"examples"`
	SuggestedPrice  string   `json:"suggestedPrice"`
	MatchedListings []string `json:"matchedListings"`
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

type Profile struct {
	ID          string    `json:"id"`
	FlowSlug    string    `json:"flowSlug"`
	Version     int       `json:"version"`
	Name        string    `json:"name"`
	Enabled     bool      `json:"enabled"`
	ProfileType string    `json:"profileType"`
	UserID      string    `json:"userId,omitempty"`
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
