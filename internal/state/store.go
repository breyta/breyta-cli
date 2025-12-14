package state

import (
        "encoding/json"
        "errors"
        "io"
        "os"
        "path/filepath"
        "time"
)

func DefaultPath() (string, error) {
        // Prefer OS config dir; falls back to HOME.
        dir, err := os.UserConfigDir()
        if err != nil || dir == "" {
                h, herr := os.UserHomeDir()
                if herr != nil {
                        return "", errors.New("cannot determine config dir")
                }
                dir = filepath.Join(h, ".config")
        }
        return filepath.Join(dir, "breyta", "mock", "state.json"), nil
}

func EnsureParentDir(path string) error {
        return os.MkdirAll(filepath.Dir(path), 0o755)
}

func Load(path string) (*State, error) {
        f, err := os.Open(path)
        if err != nil {
                return nil, err
        }
        defer f.Close()

        b, err := io.ReadAll(f)
        if err != nil {
                return nil, err
        }
        var s State
        if err := json.Unmarshal(b, &s); err != nil {
                return nil, err
        }
        return &s, nil
}

func SaveAtomic(path string, s *State) error {
        if err := EnsureParentDir(path); err != nil {
                return err
        }

        b, err := json.MarshalIndent(s, "", "  ")
        if err != nil {
                return err
        }
        b = append(b, '\n')

        tmp := path + ".tmp"
        if err := os.WriteFile(tmp, b, 0o644); err != nil {
                return err
        }
        return os.Rename(tmp, path)
}

func SeedDefault(workspaceID string) *State {
        now := time.Now().UTC()
        ws := &Workspace{
                ID:          workspaceID,
                Name:        "Demo Workspace",
                Plan:        "Creator",
                Owner:       "dev@breyta.test",
                UpdatedAt:   now,
                Flows:       map[string]*Flow{},
                Runs:        map[string]*Run{},
                Connections: map[string]*Connection{},
                Instances:   map[string]*Instance{},
                Triggers:    map[string]*Trigger{},
                Waits:       map[string]*Wait{},
        }

        // --- Flow: subscription-renewal (marketplace demo) ------------------------
        ws.Flows["subscription-renewal"] = &Flow{
                Slug:          "subscription-renewal",
                Name:          "Subscription Renewal",
                Description:   "Renews subscriptions with retries, wait states, and payment method branching.",
                Tags:          []string{"billing", "payments", "revenue"},
                ActiveVersion: 4,
                UpdatedAt:     now.Add(-3 * time.Hour),
                Spine: []string{
                        "1. Trigger: Billing cycle",
                        "2. Parallel: fetch customer + payment method + plan",
                        "3. Branch: payment method",
                        "   - card    → Process Card",
                        "   - invoice → Create Invoice",
                        "4. Wait: payment_status (24h timeout)",
                        "5. Branch: status",
                        "   - success → Send receipt",
                        "   - failed  → Retry + notify",
                },
                Steps: []FlowStep{
                        {ID: "fetch-customer", Type: "http", Title: "Fetch Customer",
                                InputSchema:  "{subscriptionId: string}",
                                OutputSchema: "{status: number, body: {customerId: string, email: string}}",
                                Definition:   "(step :http :fetch-customer {:connection :billing-api :path (str \"/subscriptions/\" subscription-id \"/customer\")})"},
                        {ID: "fetch-payment-method", Type: "http", Title: "Fetch Payment Method",
                                InputSchema:  "{customerId: string}",
                                OutputSchema: "{status: number, body: {type: \"card\"|\"invoice\", cardLast4?: string}}",
                                Definition:   "(step :http :fetch-payment-method {:connection :billing-api :path (str \"/customers/\" customer-id \"/payment-method\") :retry {:max-attempts 3 :initial-interval-ms 1000}})"},
                        {ID: "process-card", Type: "http", Title: "Process Card Payment",
                                InputSchema:  "{customerId: string, amountCents: number, currency: string}",
                                OutputSchema: "{status: number, body: {paymentIntentId: string, status: string}}",
                                Definition:   "(step :http :process-card {:connection :payments-api :method :post :path \"/payment_intents\" :json {...} :retry {:max-attempts 3 :initial-interval-ms 2000}})"},
                        {ID: "wait-payment-status", Type: "wait", Title: "Wait for payment_status",
                                InputSchema:  "{signalKey: string}",
                                OutputSchema: "{status: string, paymentIntentId?: string}",
                                Definition:   "(step :wait :payment-status {:source :webhook :event-name \"payment.status\" :signal-key (str \"sub-\" subscription-id) :timeout 86400})"},
                        {ID: "send-receipt", Type: "notify", Title: "Send Receipt",
                                InputSchema:  "{email: string, receiptUrl: string}",
                                OutputSchema: "{success: boolean}",
                                Definition:   "(step :notify :send-receipt {:channel :email :target email :subject \"Receipt\" :message (str \"Download: \" receipt-url)})"},
                },
        }

        // Seed a couple of connections + a trigger for v1 CLI shape
        ws.Connections["conn-slack-1"] = &Connection{
                ID:        "conn-slack-1",
                Name:      "Slack: #sales",
                Type:      "slack",
                Status:    "ready",
                UpdatedAt: now.Add(-48 * time.Hour),
                Config:    map[string]any{"workspace": "breyta", "channel": "#sales"},
        }
        ws.Connections["conn-stripe-1"] = &Connection{
                ID:        "conn-stripe-1",
                Name:      "Stripe: production",
                Type:      "stripe",
                Status:    "ready",
                UpdatedAt: now.Add(-24 * time.Hour),
                Config:    map[string]any{"account": "acct_123"},
        }

        ws.Triggers["trg-subscription-renewal-nightly"] = &Trigger{
                ID:        "trg-subscription-renewal-nightly",
                FlowSlug:  "subscription-renewal",
                Type:      "schedule",
                Name:      "Nightly renewals",
                Enabled:   true,
                UpdatedAt: now.Add(-6 * time.Hour),
                Config:    map[string]any{"cron": "0 2 * * *", "timezone": "UTC"},
        }

        ws.Flows["daily-sales-report"] = &Flow{
                Slug:          "daily-sales-report",
                Name:          "Daily Sales Report",
                Description:   "Fetches sales data, calculates metrics, and posts a report.",
                Tags:          []string{"analytics", "reporting"},
                ActiveVersion: 3,
                UpdatedAt:     now.Add(-2 * time.Hour),
                Spine: []string{
                        "1. Trigger: schedule/manual",
                        "2. Fetch sales",
                        "3. Calculate metrics",
                        "4. Send report",
                },
                Steps: []FlowStep{
                        {ID: "fetch-sales", Type: "http", Title: "Fetch Yesterday's Sales",
                                InputSchema:  "{triggeredAt: string}",
                                OutputSchema: "{status: number, body: {count: number, items: any[]}}",
                                Definition:   "(step :http :fetch-sales {:connection :sales-api :path \"/sales?period=yesterday\"})"},
                        {ID: "calculate-metrics", Type: "code", Title: "Calculate Sales Metrics",
                                InputSchema:  "{sales: any[]}",
                                OutputSchema: "{totalSales: number, transactionCount: number, averageOrder: number}",
                                Definition:   "(step :code :calculate-metrics {:input {:sales sales} :code '(fn [input] ...)})"},
                        {ID: "send-report", Type: "notify", Title: "Send Report",
                                InputSchema:  "{message: string}",
                                OutputSchema: "{success: boolean}",
                                Definition:   "(step :notify :send-report {:channel :slack :target \"#sales\" :message msg})"},
                },
        }
        ws.Flows["order-processor"] = &Flow{
                Slug:          "order-processor",
                Name:          "Order Processor",
                Description:   "Processes orders with fraud check and human approval for high-value transactions.",
                Tags:          []string{"ops", "approval", "fraud"},
                ActiveVersion: 7,
                UpdatedAt:     now.Add(-7 * time.Hour),
                Spine: []string{
                        "1. Trigger: webhook/manual",
                        "2. Fetch order",
                        "3. Fraud check",
                        "4. Branch: requires approval?",
                        "5. Wait for approval",
                        "6. Fulfill order",
                },
                Steps: []FlowStep{
                        {ID: "get-order", Type: "http", Title: "Fetch Order Details",
                                InputSchema:  "{orderId: string}",
                                OutputSchema: "{status: number, body: {orderId: string, total: number}}",
                                Definition:   "(step :http :get-order {:connection :shop-api :path (str \"/orders/\" order-id)})"},
                        {ID: "fraud-check", Type: "http", Title: "Fraud Check",
                                InputSchema:  "{orderId: string, total: number}",
                                OutputSchema: "{status: number, body: {riskScore: number}}",
                                Definition:   "(step :http :fraud-check {:connection :fraud-api :method :post :path \"/analyze\" :json {...}})"},
                        {ID: "approval", Type: "wait", Title: "Wait for Approval",
                                InputSchema:  "{signalKey: string}",
                                OutputSchema: "{approved: boolean, approverId: string}",
                                Definition:   "(step :wait :approval {:source :api :signal-key (str \"approve-\" order-id) :timeout 86400})"},
                        {ID: "fulfill", Type: "http", Title: "Fulfill Order",
                                InputSchema:  "{orderId: string, approvedBy: string}",
                                OutputSchema: "{status: number}",
                                Definition:   "(step :http :fulfill {:connection :shop-api :method :post :path (str \"/orders/\" order-id \"/fulfill\")})"},
                },
        }

        // Seed one run
        r1 := &Run{
                WorkflowID:   "wf-demo-001",
                FlowSlug:     "daily-sales-report",
                Version:      3,
                Status:       "running",
                TriggeredBy:  "schedule",
                StartedAt:    now.Add(-2 * time.Minute),
                UpdatedAt:    now,
                CurrentStep:  "calculate-metrics",
                InputPreview: map[string]any{"triggeredAt": now.Add(-2 * time.Minute).Format(time.RFC3339)},
                Steps: []StepExecution{
                        {StepID: "fetch-sales", StepType: "http", Title: "Fetch Yesterday's Sales", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-2 * time.Minute), CompletedAt: ptrTime(now.Add(-110 * time.Second)), DurationMs: 420,
                                InputPreview:  map[string]any{"triggeredAt": now.Add(-2 * time.Minute).Format(time.RFC3339)},
                                ResultPreview: map[string]any{"truncated": false, "data": "200 OK (23kb)"}},
                        {StepID: "calculate-metrics", StepType: "code", Title: "Calculate Sales Metrics", Status: "running", Attempt: 1,
                                StartedAt: now.Add(-20 * time.Second), DurationMs: 0,
                                InputPreview: map[string]any{"sales": "ref:sales.json"}},
                        {StepID: "send-report", StepType: "notify", Title: "Send Report", Status: "pending", Attempt: 0, StartedAt: time.Time{}},
                },
        }
        ws.Runs[r1.WorkflowID] = r1

        // Seed marketplace-style run IDs and history for subscription-renewal
        r4821CompletedAt := now.Add(-10 * time.Minute)
        r4821ReceiptDoneAt := now.Add(-9*time.Minute - 40*time.Second)
        ws.Runs["4821"] = &Run{
                WorkflowID:  "4821",
                FlowSlug:    "subscription-renewal",
                Version:     4,
                Status:      "completed",
                TriggeredBy: "schedule",
                StartedAt:   now.Add(-12 * time.Minute),
                UpdatedAt:   r4821CompletedAt,
                CompletedAt: &r4821CompletedAt,
                ResultPreview: map[string]any{
                        "status":      "success",
                        "receiptSent": true,
                },
                Steps: []StepExecution{
                        {StepID: "fetch-customer", StepType: "http", Title: "Fetch Customer", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-12 * time.Minute), CompletedAt: ptrTime(now.Add(-11*time.Minute - 40*time.Second)), DurationMs: 180,
                                InputPreview:  map[string]any{"subscriptionId": "sub_123"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"customerId": "cus_42", "email": "a@company.com"}}},
                        {StepID: "fetch-payment-method", StepType: "http", Title: "Fetch Payment Method", Status: "completed", Attempt: 2,
                                StartedAt: now.Add(-11 * time.Minute), CompletedAt: ptrTime(now.Add(-10*time.Minute - 45*time.Second)), DurationMs: 920,
                                InputPreview:  map[string]any{"customerId": "cus_42"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"type": "card", "cardLast4": "4242"}}},
                        {StepID: "process-card", StepType: "http", Title: "Process Card Payment", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-10 * time.Minute), CompletedAt: ptrTime(now.Add(-9*time.Minute - 45*time.Second)), DurationMs: 1250,
                                InputPreview:  map[string]any{"customerId": "cus_42", "amountCents": 9900, "currency": "USD"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"paymentIntentId": "pi_abc", "status": "requires_action"}}},
                        {StepID: "wait-payment-status", StepType: "wait", Title: "Wait for payment_status", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-9 * time.Minute), CompletedAt: ptrTime(now.Add(-8*time.Minute - 30*time.Second)), DurationMs: 30000,
                                InputPreview:  map[string]any{"signalKey": "sub-sub_123"},
                                ResultPreview: map[string]any{"status": "succeeded", "paymentIntentId": "pi_abc"}},
                        {StepID: "send-receipt", StepType: "notify", Title: "Send Receipt", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-8 * time.Minute), CompletedAt: &r4821ReceiptDoneAt, DurationMs: 500,
                                InputPreview:  map[string]any{"email": "a@company.com", "receiptUrl": "https://example.com/r/pi_abc"},
                                ResultPreview: map[string]any{"success": true}},
                },
        }

        // Previous failed run (referenced by revenue/demand examples)
        r4799At := now.AddDate(0, 0, -3)
        ws.Runs["4799"] = &Run{
                WorkflowID:  "4799",
                FlowSlug:    "subscription-renewal",
                Version:     4,
                Status:      "failed",
                TriggeredBy: "schedule",
                StartedAt:   r4799At.Add(-3 * time.Minute),
                UpdatedAt:   r4799At,
                CompletedAt: ptrTime(r4799At),
                Error:       "card_declined",
                ResultPreview: map[string]any{
                        "status": "failed",
                        "reason": "card_declined",
                },
                Steps: []StepExecution{
                        {StepID: "fetch-customer", StepType: "http", Title: "Fetch Customer", Status: "completed", Attempt: 1,
                                StartedAt: r4799At.Add(-3 * time.Minute), CompletedAt: ptrTime(r4799At.Add(-2*time.Minute - 40*time.Second)), DurationMs: 200,
                                InputPreview:  map[string]any{"subscriptionId": "sub_119"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"customerId": "cus_19", "email": "billing@startup.com"}}},
                        {StepID: "fetch-payment-method", StepType: "http", Title: "Fetch Payment Method", Status: "completed", Attempt: 1,
                                StartedAt: r4799At.Add(-2 * time.Minute), CompletedAt: ptrTime(r4799At.Add(-1*time.Minute - 35*time.Second)), DurationMs: 400,
                                InputPreview:  map[string]any{"customerId": "cus_19"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"type": "card", "cardLast4": "0005"}}},
                        {StepID: "process-card", StepType: "http", Title: "Process Card Payment", Status: "failed", Attempt: 1,
                                StartedAt: r4799At.Add(-1 * time.Minute), CompletedAt: ptrTime(r4799At.Add(-50 * time.Second)), DurationMs: 800,
                                InputPreview:  map[string]any{"customerId": "cus_19", "amountCents": 9900, "currency": "USD"},
                                ResultPreview: map[string]any{"status": 402, "body": map[string]any{"error": "card_declined"}}, Error: "card_declined"},
                        {StepID: "wait-payment-status", StepType: "wait", Title: "Wait for payment_status", Status: "cancelled", Attempt: 0},
                        {StepID: "send-receipt", StepType: "notify", Title: "Send Receipt", Status: "cancelled", Attempt: 0},
                },
        }

        // Seed revenue + demand (marketplace angle)
        ws.RevenueEvents = []RevenueEvent{
                {At: now.AddDate(0, 0, -1), Currency: "USD", AmountCents: 9900, Source: "flow-run", FlowSlug: "subscription-renewal", RunID: "4821"},
                {At: now.AddDate(0, 0, -3), Currency: "USD", AmountCents: 9900, Source: "flow-run", FlowSlug: "subscription-renewal", RunID: "4799"},
                {At: now.AddDate(0, 0, -8), Currency: "USD", AmountCents: 2500, Source: "flow-run", FlowSlug: "daily-sales-report", RunID: "wf-demo-001"},
        }
        ws.DemandTop = []DemandItem{
                {Query: "renew subscriptions and email receipts", Count: 42, Window: "30d", SuggestedPrice: "$10 / successful renewal", MatchedFlows: []string{"subscription-renewal"}},
                {Query: "weekly slack report from sales data", Count: 27, Window: "30d", SuggestedPrice: "$5 / run", MatchedFlows: []string{"daily-sales-report"}},
                {Query: "high value order approval workflow", Count: 18, Window: "30d", SuggestedPrice: "$15 / run", MatchedFlows: []string{"order-processor"}},
        }

        return &State{
                Version:    1,
                Workspaces: map[string]*Workspace{workspaceID: ws},
                Tick:       0,
        }
}

func ptrTime(t time.Time) *time.Time { return &t }
