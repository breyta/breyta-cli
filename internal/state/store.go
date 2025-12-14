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
                ID:             workspaceID,
                Name:           "Demo Workspace",
                Plan:           "Creator",
                Owner:          "dev@breyta.test",
                UpdatedAt:      now,
                Flows:          map[string]*Flow{},
                Runs:           map[string]*Run{},
                Registry:       map[string]*RegistryEntry{},
                Purchases:      map[string]*Purchase{},
                Entitlements:   map[string]*Entitlement{},
                Payouts:        map[string]*Payout{},
                DemandQueries:  []DemandQuery{},
                DemandClusters: []DemandCluster{},
                Connections:    map[string]*Connection{},
                Instances:      map[string]*Instance{},
                Triggers:       map[string]*Trigger{},
                Waits:          map[string]*Wait{},
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
                        "2. Parallel:",
                        "   a. Fetch Customer           (fetch-customer)",
                        "   b. Fetch Payment Method     (fetch-payment-method)",
                        "   c. Fetch Plan               (fetch-plan)",
                        "   Join",
                        "3. Branch: payment method      (branch-payment-method)",
                        "   [card]    → Process Card    (process-card)",
                        "   [invoice] → Create Invoice  (create-invoice)",
                        "4. Wait: payment_status (24h timeout)  (wait-payment-status)",
                        "5. Branch: status              (branch-status)",
                        "   [success] → Send Receipt    (call-send-receipt)",
                        "   [failed]  → Notify + Retry  (notify-failed, retry-payment)",
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
                        {ID: "fetch-plan", Type: "http", Title: "Fetch Plan",
                                InputSchema:  "{subscriptionId: string}",
                                OutputSchema: "{status: number, body: {planId: string, amountCents: number, currency: string}}",
                                Definition:   "(step :http :fetch-plan {:connection :billing-api :path (str \"/subscriptions/\" subscription-id \"/plan\")})"},
                        {ID: "branch-payment-method", Type: "code", Title: "Choose payment path",
                                InputSchema:  "{paymentMethod: any}",
                                OutputSchema: "{path: \"card\"|\"invoice\"}",
                                Definition:   "(step :code :branch-payment-method {:code '(fn [{:keys [paymentMethod]}] (if (= \"invoice\" (get-in paymentMethod [:body :type])) {:path \"invoice\"} {:path \"card\"}))})"},
                        {ID: "process-card", Type: "http", Title: "Process Card Payment",
                                InputSchema:  "{customerId: string, amountCents: number, currency: string}",
                                OutputSchema: "{status: number, body: {paymentIntentId: string, status: string}}",
                                Definition:   "(step :http :process-card {:connection :payments-api :method :post :path \"/payment_intents\" :json {...} :retry {:max-attempts 3 :initial-interval-ms 2000}})"},
                        {ID: "create-invoice", Type: "http", Title: "Create Invoice",
                                InputSchema:  "{customerId: string, amountCents: number, currency: string}",
                                OutputSchema: "{status: number, body: {invoiceId: string, status: string}}",
                                Definition:   "(step :http :create-invoice {:connection :billing-api :method :post :path \"/invoices\" :json {...}})"},
                        {ID: "wait-payment-status", Type: "wait", Title: "Wait for payment_status",
                                InputSchema:  "{signalKey: string}",
                                OutputSchema: "{status: string, paymentIntentId?: string}",
                                Definition:   "(step :wait :payment-status {:source :webhook :event-name \"payment.status\" :signal-key (str \"sub-\" subscription-id) :timeout 86400})"},
                        {ID: "branch-status", Type: "code", Title: "Choose success/failure path",
                                InputSchema:  "{paymentStatus: any}",
                                OutputSchema: "{path: \"success\"|\"failed\"}",
                                Definition:   "(step :code :branch-status {:code '(fn [{:keys [paymentStatus]}] (if (= \"succeeded\" (get paymentStatus :status)) {:path \"success\"} {:path \"failed\"}))})"},
                        {ID: "call-send-receipt", Type: "call", Title: "Send Receipt (Subflow)",
                                InputSchema:  "{email: string, receiptUrl: string}",
                                OutputSchema: "{ok: boolean}",
                                Definition:   "(call-flow :send-receipt-subflow)",
                                CallFlowSlug: "send-receipt-subflow"},
                        {ID: "notify-failed", Type: "notify", Title: "Notify failure",
                                InputSchema:  "{email: string, reason: string}",
                                OutputSchema: "{success: boolean}",
                                Definition:   "(step :notify :notify-failed {:channel :email :target email :subject \"Payment failed\" :message reason})"},
                        {ID: "retry-payment", Type: "http", Title: "Retry Payment",
                                InputSchema:  "{paymentIntentId: string}",
                                OutputSchema: "{status: number, body: {status: string}}",
                                Definition:   "(step :http :retry-payment {:connection :payments-api :method :post :path (str \"/payment_intents/\" id \"/retry\") :retry {:max-attempts 2 :initial-interval-ms 5000}})"},
                },
        }

        // --- Subflow: send-receipt (called by subscription-renewal) ----------------
        ws.Flows["send-receipt-subflow"] = &Flow{
                Slug:          "send-receipt-subflow",
                Name:          "Send Receipt (Subflow)",
                Description:   "Reusable receipt delivery subflow for billing workflows.",
                Tags:          []string{"billing", "notify", "subflow"},
                ActiveVersion: 1,
                UpdatedAt:     now.Add(-36 * time.Hour),
                Spine: []string{
                        "├─ notify: send email receipt",
                        "└─ log: record delivery",
                },
                Steps: []FlowStep{
                        {ID: "send-email", Type: "notify", Title: "Send receipt email",
                                InputSchema:  "{email: string, receiptUrl: string}",
                                OutputSchema: "{success: boolean, messageId: string}",
                                Definition:   "(step :notify :send-email {:channel :email :to email :template :receipt})"},
                        {ID: "audit-log", Type: "code", Title: "Audit log",
                                InputSchema:  "{messageId: string, email: string}",
                                OutputSchema: "{ok: boolean}",
                                Definition:   "(step :code :audit-log {:code '(fn [x] (assoc x :ok true))})"},
                },
        }

        // --- Flow: customer-billing-healthcheck (parallel + loop + branch + subflow)
        ws.Flows["customer-billing-healthcheck"] = &Flow{
                Slug:          "customer-billing-healthcheck",
                Name:          "Customer Billing Healthcheck",
                Description:   "Fan-out to fetch billing signals, loop over invoices, and branch into remediation paths. Calls subflows for notifications.",
                Tags:          []string{"billing", "analytics", "branching", "parallel", "loop"},
                ActiveVersion: 1,
                UpdatedAt:     now.Add(-90 * time.Minute),
                Spine: []string{
                        "⫴ parallel: fetch customer | invoices | subscriptions",
                        "  ├─ http: fetch-customer        (fetch-customer)",
                        "  ├─ http: fetch-invoices        (fetch-invoices)",
                        "  └─ http: fetch-subscriptions   (fetch-subscriptions)",
                        "⫵ join / wait for all",
                        "├─ code: enrich-customer         (enrich-customer)",
                        "╭─ loop: for-each invoice",
                        "│  ├─ code: validate-invoice     (validate-invoice)",
                        "│  └─ http: mark-invoice         (mark-invoice)",
                        "╰─ next item",
                        "├─ ◇ branch: customer segment / overdue (branch-remediation)",
                        "│  ├─ premium → code: priority-handling          (priority-handling)",
                        "│  ├─ overdue → ↗ call subflow: send-reminder-subflow (call-send-reminder)",
                        "│  └─ else    → code: standard-processing        (standard-processing)",
                        "└─ http: update-crm               (update-crm)",
                },
                Steps: []FlowStep{
                        {ID: "fetch-customer", Type: "http", Title: "Fetch Customer", InputSchema: "{customerId: string}", OutputSchema: "{status: number, body: any}", Definition: "(step :http :fetch-customer {:path \"/customers/{id}\"})"},
                        {ID: "fetch-invoices", Type: "http", Title: "Fetch Invoices", InputSchema: "{customerId: string}", OutputSchema: "{status: number, body: {invoices: any[]}}", Definition: "(step :http :fetch-invoices {:path \"/customers/{id}/invoices\"})"},
                        {ID: "fetch-subscriptions", Type: "http", Title: "Fetch Subscriptions", InputSchema: "{customerId: string}", OutputSchema: "{status: number, body: {subscriptions: any[]}}", Definition: "(step :http :fetch-subscriptions {:path \"/customers/{id}/subscriptions\"})"},
                        {ID: "enrich-customer", Type: "code", Title: "Enrich Customer", InputSchema: "{customer: any}", OutputSchema: "{customer: any, segment: string}", Definition: "(step :code :enrich-customer {:code '(fn [x] (assoc x :segment \"premium\"))})"},
                        {ID: "validate-invoice", Type: "code", Title: "Validate Invoice", InputSchema: "{invoice: any}", OutputSchema: "{ok: boolean, overdue: boolean}", Definition: "(step :code :validate-invoice {:code '(fn [x] {:ok true :overdue false})})"},
                        {ID: "mark-invoice", Type: "http", Title: "Mark Invoice", InputSchema: "{invoiceId: string, status: string}", OutputSchema: "{status: number}", Definition: "(step :http :mark-invoice {:method :post :path \"/invoices/{id}/mark\"})"},
                        {ID: "branch-remediation", Type: "code", Title: "Decide remediation path (premium/overdue/else)", InputSchema: "{segment: string, overdue: boolean}", OutputSchema: "{path: \"premium\"|\"overdue\"|\"else\"}", Definition: "(step :code :branch-remediation {:code '(fn [{:keys [segment overdue]}] (cond (= segment \"premium\") {:path \"premium\"} overdue {:path \"overdue\"} :else {:path \"else\"}))})"},
                        {ID: "priority-handling", Type: "code", Title: "Priority Handling", InputSchema: "{customer: any}", OutputSchema: "{ok: boolean}", Definition: "(step :code :priority-handling {:code '(fn [x] {:ok true})})"},
                        {ID: "call-send-reminder", Type: "call", Title: "Call Send Reminder Subflow", InputSchema: "{customer: any}", OutputSchema: "{ok: boolean}", Definition: "(call-flow :send-reminder-subflow)", CallFlowSlug: "send-reminder-subflow"},
                        {ID: "standard-processing", Type: "code", Title: "Standard Processing", InputSchema: "{customer: any}", OutputSchema: "{ok: boolean}", Definition: "(step :code :standard-processing {:code '(fn [x] {:ok true})})"},
                        {ID: "update-crm", Type: "http", Title: "Update CRM", InputSchema: "{customer: any}", OutputSchema: "{status: number}", Definition: "(step :http :update-crm {:method :post :path \"/crm/update\"})"},
                },
        }

        // --- Subflow: send-reminder-subflow ----------------------------------------
        ws.Flows["send-reminder-subflow"] = &Flow{
                Slug:          "send-reminder-subflow",
                Name:          "Send Payment Reminder (Subflow)",
                Description:   "Reusable reminder subflow (email + slack + audit).",
                Tags:          []string{"billing", "notify", "subflow"},
                ActiveVersion: 1,
                UpdatedAt:     now.Add(-2 * 24 * time.Hour),
                Spine: []string{
                        "├─ notify: email reminder",
                        "├─ notify: slack alert",
                        "└─ code: audit-log",
                },
                Steps: []FlowStep{
                        {ID: "email-reminder", Type: "notify", Title: "Email reminder", InputSchema: "{email: string}", OutputSchema: "{success: boolean}", Definition: "(step :notify :email-reminder {:channel :email})"},
                        {ID: "slack-alert", Type: "notify", Title: "Slack alert", InputSchema: "{channel: string}", OutputSchema: "{success: boolean}", Definition: "(step :notify :slack-alert {:channel :slack})"},
                        {ID: "audit", Type: "code", Title: "Audit", InputSchema: "{context: any}", OutputSchema: "{ok: boolean}", Definition: "(step :code :audit {:code '(fn [x] {:ok true})})"},
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
                        "   [yes] → Wait for approval  (approval)",
                        "   [no]  → Fulfill order      (fulfill)",
                        "5. Fulfill order",
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
                        {StepID: "fetch-plan", StepType: "http", Title: "Fetch Plan", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-10*time.Minute - 50*time.Second), CompletedAt: ptrTime(now.Add(-10*time.Minute - 47*time.Second)), DurationMs: 310,
                                InputPreview:  map[string]any{"subscriptionId": "sub_123"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"planId": "plan_pro_monthly", "amountCents": 9900, "currency": "USD"}}},
                        {StepID: "branch-payment-method", StepType: "code", Title: "Choose payment path", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-10*time.Minute - 46*time.Second), CompletedAt: ptrTime(now.Add(-10*time.Minute - 45*time.Second)), DurationMs: 65,
                                InputPreview:  map[string]any{"paymentMethod": map[string]any{"type": "card", "cardLast4": "4242"}},
                                ResultPreview: map[string]any{"path": "card"}},
                        {StepID: "process-card", StepType: "http", Title: "Process Card Payment", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-10 * time.Minute), CompletedAt: ptrTime(now.Add(-9*time.Minute - 45*time.Second)), DurationMs: 1250,
                                InputPreview:  map[string]any{"customerId": "cus_42", "amountCents": 9900, "currency": "USD"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"paymentIntentId": "pi_abc", "status": "requires_action"}}},
                        {StepID: "create-invoice", StepType: "http", Title: "Create Invoice", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-9*time.Minute - 44*time.Second), CompletedAt: ptrTime(now.Add(-9*time.Minute - 43*time.Second)), DurationMs: 240,
                                InputPreview:  map[string]any{"customerId": "cus_42", "amountCents": 9900, "currency": "USD"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"invoiceId": "inv_ignored", "status": "skipped"}}},
                        {StepID: "wait-payment-status", StepType: "wait", Title: "Wait for payment_status", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-9 * time.Minute), CompletedAt: ptrTime(now.Add(-8*time.Minute - 30*time.Second)), DurationMs: 30000,
                                InputPreview:  map[string]any{"signalKey": "sub-sub_123"},
                                ResultPreview: map[string]any{"status": "succeeded", "paymentIntentId": "pi_abc"}},
                        {StepID: "branch-status", StepType: "code", Title: "Choose success/failure path", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-8*time.Minute - 25*time.Second), CompletedAt: ptrTime(now.Add(-8*time.Minute - 24*time.Second)), DurationMs: 20,
                                InputPreview:  map[string]any{"paymentStatus": map[string]any{"status": "succeeded"}},
                                ResultPreview: map[string]any{"path": "success"}},
                        {StepID: "call-send-receipt", StepType: "call", Title: "Send Receipt (Subflow)", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-8 * time.Minute), CompletedAt: &r4821ReceiptDoneAt, DurationMs: 500,
                                InputPreview:  map[string]any{"email": "a@company.com", "receiptUrl": "https://example.com/r/pi_abc"},
                                ResultPreview: map[string]any{"ok": true}},
                        {StepID: "notify-failed", StepType: "notify", Title: "Notify failure", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-9*time.Minute - 20*time.Second), CompletedAt: ptrTime(now.Add(-9*time.Minute - 19*time.Second)), DurationMs: 80,
                                InputPreview:  map[string]any{"email": "a@company.com", "reason": ""},
                                ResultPreview: map[string]any{"success": true}},
                        {StepID: "retry-payment", StepType: "http", Title: "Retry Payment", Status: "completed", Attempt: 1,
                                StartedAt: now.Add(-9*time.Minute - 18*time.Second), CompletedAt: ptrTime(now.Add(-9*time.Minute - 16*time.Second)), DurationMs: 710,
                                InputPreview:  map[string]any{"paymentIntentId": "pi_abc"},
                                ResultPreview: map[string]any{"status": 200, "body": map[string]any{"status": "succeeded"}}},
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

        // --- Marketplace registry listings (mock) -----------------------------------
        creator := "dev@breyta.test"
        pub := now.Add(-10 * 24 * time.Hour)
        ws.Registry["wrk-subscription-renewal"] = &RegistryEntry{
                ID:          "wrk-subscription-renewal",
                Slug:        "subscription-renewal",
                Title:       "Subscription Renewal",
                Summary:     "Renew subscriptions with retries, waits, and receipts.",
                Description: "A production-grade renewal workflow with branching payment methods, retries for transient failures, and receipt delivery.",
                Creator:     creator,
                Category:    "billing",
                Tags:        []string{"billing", "payments", "revenue"},
                Pricing:     Pricing{Model: "per_success", Currency: "USD", AmountCents: 1000},
                UpdatedAt:   now.Add(-3 * time.Hour),
                PublishedAt: pub,
                Versions: []RegistryVersion{
                        {Version: 1, PublishedAt: pub, Note: "Initial listing", FlowSlug: "subscription-renewal", FlowVersion: 2},
                        {Version: 2, PublishedAt: pub.Add(4 * 24 * time.Hour), Note: "Add wait state + receipt", FlowSlug: "subscription-renewal", FlowVersion: 4},
                },
                Stats: RegistryStats{Views: 1240, Installs: 47, Active: 19, SuccessRate: 0.93, Rating: 4.8, Reviews: 12, RevenueCents: 18700},
        }
        ws.Registry["wrk-daily-sales-report"] = &RegistryEntry{
                ID:          "wrk-daily-sales-report",
                Slug:        "daily-sales-report",
                Title:       "Daily Sales Report",
                Summary:     "Fetch sales, compute metrics, post a report.",
                Description: "A simple but polished reporting workflow. Great starter for analytics automation.",
                Creator:     creator,
                Category:    "analytics",
                Tags:        []string{"analytics", "reporting"},
                Pricing:     Pricing{Model: "subscription", Currency: "USD", AmountCents: 1500, Interval: "month"},
                UpdatedAt:   now.Add(-2 * time.Hour),
                PublishedAt: pub.Add(2 * 24 * time.Hour),
                Versions: []RegistryVersion{
                        {Version: 1, PublishedAt: pub.Add(2 * 24 * time.Hour), Note: "Launch", FlowSlug: "daily-sales-report", FlowVersion: 3},
                },
                Stats: RegistryStats{Views: 980, Installs: 31, Active: 14, SuccessRate: 0.98, Rating: 4.6, Reviews: 7, RevenueCents: 46500},
        }
        ws.Registry["wrk-order-processor"] = &RegistryEntry{
                ID:          "wrk-order-processor",
                Slug:        "order-processor",
                Title:       "Order Processor",
                Summary:     "Fraud check + approval + fulfillment.",
                Description: "Handle orders with fraud scoring and optional human approval for high-value purchases.",
                Creator:     creator,
                Category:    "ops",
                Tags:        []string{"ops", "approval", "fraud"},
                Pricing:     Pricing{Model: "per_run", Currency: "USD", AmountCents: 250},
                UpdatedAt:   now.Add(-7 * time.Hour),
                PublishedAt: pub.Add(6 * 24 * time.Hour),
                Versions: []RegistryVersion{
                        {Version: 1, PublishedAt: pub.Add(6 * 24 * time.Hour), Note: "Launch", FlowSlug: "order-processor", FlowVersion: 7},
                },
                Stats: RegistryStats{Views: 530, Installs: 18, Active: 6, SuccessRate: 0.87, Rating: 4.2, Reviews: 4, RevenueCents: 9200},
        }

        // --- Demand signals (raw + clustered) ---------------------------------------
        ws.DemandQueries = []DemandQuery{
                {Query: "Send me a daily Slack summary of Stripe refunds", At: now.Add(-2 * time.Hour), Window: "30d", OfferCents: 1000, Currency: "USD", NormalizedTo: "daily stripe refund summary"},
                {Query: "Renew subscriptions and retry payment if card fails", At: now.Add(-5 * time.Hour), Window: "30d", OfferCents: 1000, Currency: "USD", NormalizedTo: "subscription renewal with retries"},
                {Query: "Fraud check orders and require approval for large orders", At: now.Add(-10 * time.Hour), Window: "30d", OfferCents: 500, Currency: "USD", NormalizedTo: "order fraud + approval"},
                {Query: "Daily sales report to Slack", At: now.Add(-12 * time.Hour), Window: "30d", OfferCents: 1500, Currency: "USD", NormalizedTo: "daily sales report"},
        }
        ws.DemandClusters = []DemandCluster{
                {ID: "dem-001", Title: "Subscription renewal with retries", Count: 42, Window: "30d", Examples: []string{"Renew subscriptions and retry payment if card fails", "Handle invoice vs card billing automatically"}, SuggestedPrice: "$10 / success", MatchedListings: []string{"wrk-subscription-renewal"}},
                {ID: "dem-002", Title: "Daily sales reporting", Count: 27, Window: "30d", Examples: []string{"Daily sales report to Slack", "Weekly revenue summary email"}, SuggestedPrice: "$15 / month", MatchedListings: []string{"wrk-daily-sales-report"}},
                {ID: "dem-003", Title: "Order fraud + approval", Count: 18, Window: "30d", Examples: []string{"Fraud check orders and require approval for large orders"}, SuggestedPrice: "$2.50 / run", MatchedListings: []string{"wrk-order-processor"}},
        }

        // --- Entitlements + purchases + payouts (mock) ------------------------------
        p1PaidAt := now.Add(-9 * 24 * time.Hour)
        ws.Purchases["pur-001"] = &Purchase{ID: "pur-001", ListingID: "wrk-subscription-renewal", Buyer: "buyer@demo.test", Status: "paid", CreatedAt: now.Add(-9*24*time.Hour - 2*time.Minute), PaidAt: &p1PaidAt, AmountCents: 1000, Currency: "USD"}
        exp := now.Add(21 * 24 * time.Hour)
        ws.Entitlements["ent-001"] = &Entitlement{ID: "ent-001", ListingID: "wrk-subscription-renewal", Buyer: "buyer@demo.test", Status: "active", CreatedAt: p1PaidAt, ExpiresAt: &exp, Limits: map[string]any{"runsPerMonth": 200}}
        ws.Payouts["pay-2025-12"] = &Payout{ID: "pay-2025-12", Creator: creator, Period: now.Format("2006-01"), AmountCents: 61200, Currency: "USD", Status: "pending", CreatedAt: now.Add(-2 * 24 * time.Hour)}

        return &State{
                Version:    1,
                Workspaces: map[string]*Workspace{workspaceID: ws},
                Tick:       0,
        }
}

func ptrTime(t time.Time) *time.Time { return &t }

// EnsureDefaults merges any newly-added seeded resources into an existing state,
// without overwriting user-created resources. This keeps demo state evolving
// across versions (e.g., adding new example flows) while preserving local edits.
func EnsureDefaults(st *State, workspaceID string) {
        if st == nil {
                return
        }
        if st.Workspaces == nil {
                st.Workspaces = map[string]*Workspace{}
        }

        seed := SeedDefault(workspaceID)
        seedWS := seed.Workspaces[workspaceID]
        if seedWS == nil {
                return
        }

        ws := st.Workspaces[workspaceID]
        if ws == nil {
                st.Workspaces[workspaceID] = seedWS
                return
        }

        // Fill missing workspace metadata (never overwrite).
        if ws.ID == "" {
                ws.ID = seedWS.ID
        }
        if ws.Name == "" {
                ws.Name = seedWS.Name
        }
        if ws.Plan == "" {
                ws.Plan = seedWS.Plan
        }
        if ws.Owner == "" {
                ws.Owner = seedWS.Owner
        }
        if ws.UpdatedAt.IsZero() {
                ws.UpdatedAt = seedWS.UpdatedAt
        }

        // Ensure maps/slices are non-nil.
        if ws.Flows == nil {
                ws.Flows = map[string]*Flow{}
        }
        if ws.Runs == nil {
                ws.Runs = map[string]*Run{}
        }
        if ws.Registry == nil {
                ws.Registry = map[string]*RegistryEntry{}
        }
        if ws.Purchases == nil {
                ws.Purchases = map[string]*Purchase{}
        }
        if ws.Entitlements == nil {
                ws.Entitlements = map[string]*Entitlement{}
        }
        if ws.Payouts == nil {
                ws.Payouts = map[string]*Payout{}
        }
        if ws.Connections == nil {
                ws.Connections = map[string]*Connection{}
        }
        if ws.Instances == nil {
                ws.Instances = map[string]*Instance{}
        }
        if ws.Triggers == nil {
                ws.Triggers = map[string]*Trigger{}
        }
        if ws.Waits == nil {
                ws.Waits = map[string]*Wait{}
        }
        if ws.DemandQueries == nil {
                ws.DemandQueries = []DemandQuery{}
        }
        if ws.DemandClusters == nil {
                ws.DemandClusters = []DemandCluster{}
        }

        // Merge seeded flows (add missing only).
        for slug, f := range seedWS.Flows {
                if slug == "" || f == nil {
                        continue
                }
                if _, exists := ws.Flows[slug]; !exists {
                        ws.Flows[slug] = f
                }
        }

        // Merge seeded marketplace resources (add missing only).
        for id, e := range seedWS.Registry {
                if id == "" || e == nil {
                        continue
                }
                if _, exists := ws.Registry[id]; !exists {
                        ws.Registry[id] = e
                }
        }
        for id, p := range seedWS.Purchases {
                if id == "" || p == nil {
                        continue
                }
                if _, exists := ws.Purchases[id]; !exists {
                        ws.Purchases[id] = p
                }
        }
        for id, e := range seedWS.Entitlements {
                if id == "" || e == nil {
                        continue
                }
                if _, exists := ws.Entitlements[id]; !exists {
                        ws.Entitlements[id] = e
                }
        }
        for id, p := range seedWS.Payouts {
                if id == "" || p == nil {
                        continue
                }
                if _, exists := ws.Payouts[id]; !exists {
                        ws.Payouts[id] = p
                }
        }

        // If demand/revenue is empty, seed it (don’t overwrite if user already has data).
        if len(ws.DemandQueries) == 0 && len(seedWS.DemandQueries) > 0 {
                ws.DemandQueries = seedWS.DemandQueries
        }
        if len(ws.DemandClusters) == 0 && len(seedWS.DemandClusters) > 0 {
                ws.DemandClusters = seedWS.DemandClusters
        }
        if len(ws.RevenueEvents) == 0 && len(seedWS.RevenueEvents) > 0 {
                ws.RevenueEvents = seedWS.RevenueEvents
        }
        if len(ws.DemandTop) == 0 && len(seedWS.DemandTop) > 0 {
                ws.DemandTop = seedWS.DemandTop
        }
}
