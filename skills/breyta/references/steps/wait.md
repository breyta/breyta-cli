# Wait step (`:wait`)
Use for webhook/event waits or human-in-the-loop pauses.

Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:key` | string | Yes | Correlation key |
| `:notify` | map | No | Notification config (channels: `:http`; use email APIs like SendGrid via `:http` connections) |
| `:timeout` | string/number | No | Duration string (e.g. \"24h\") or seconds |

Notes:
- Use a stable `:key` so external events can resume the flow.
- Webhook waits are configured on triggers; waits pause execution.
- `:notify` is optional and depends on your workspace notification setup.
- For incoming webhooks, bind secret slots (`:type :secret`) via profile bindings to secure requests.
- When `:notify` is present, the wait record includes approval URL templates you can render and share.
- Email notifications use the same `:http` channel and an email API connection (for example SendGrid).

`:notify` fields (example shape):
- `:channels` for per-channel configs (e.g. `{:http {:connection :notify-api :path "/notify" :method :post}}`)
- HTTP notifications require a connection slot; inline auth is not supported.

Example:

```clojure
(flow/step :wait :webhook
           {:key "approval"
            :timeout "24h"
            :notify {:channels {:http {:connection :notify-api
                                       :path "/notify"
                                       :method :post}}}})
```

Email notification (SendGrid example):

```clojure
(flow/step :wait :approval
           {:key "approval-123"
            :timeout "24h"
            :notify {:channels
                     {:http {:connection :sendgrid
                             :path "/mail/send"
                             :method :post
                             :json {:personalizations [{:to [{:email "you@company.com"}]}]
                                    :from {:email "noreply@company.com" :name "Breyta"}
                                    :subject "Approval needed"
                                    :content [{:type "text/plain"
                                               :value "Approve: {approvalUrl}\nReject: {rejectionUrl}"}]}}}}})
```

## Step-by-step: approval URL templates
Use this when a human must approve before the flow continues.

1) Add a wait step:
```clojure
(flow/step :wait :approval
           {:key "approval-123"
            :timeout "10m"
            :notify {:channels {:http {:connection :notify-api
                                       :path "/notify"
                                       :method :post}}}})
```

Expected template output (from wait list):
```edn
{:approval
 {:template "{base-url}/{workspace-id}/waits/{wait-id}/{action}?token={token}"
  :params {:base-url "https://flows.breyta.ai"
           :workspace-id "ws_123"
           :wait-id "wait_abc"
           :token "token_..."}
  :actions [:approve :reject]}}
```

2) Start a run and list waits:
```bash
breyta runs start --flow <slug> --input '{"n":41}'
breyta waits list --flow <slug>
```

3) Use the approval URL from the wait list output:
- Open `approvalUrl` (or build from the template).
- Approve or reject to resume the run.

Example wait list payload (approval URL template):
```edn
{:approval
 {:template "{base-url}/{workspace-id}/waits/{wait-id}/{action}?token={token}"
  :params {:workspace-id "ws_123"
           :wait-id "wait_abc"
           :token "token_..."}
  :actions [:approve :reject]}}
```
If you only need a single link, use `approvalUrl` or `rejectionUrl` from the wait list output.
These routes redirect to login when unauthenticated and then resume the action.

For notifications, `approvalUrl` and `rejectionUrl` are available as template data and can be referenced as `{approvalUrl}` and `{rejectionUrl}` in your message template.
