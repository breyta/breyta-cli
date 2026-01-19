# Wait step (`:wait`)
Use for webhook/event waits or human-in-the-loop pauses.

Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:key` | string | Yes | Correlation key |
| `:notify` | map | No | Notification config (recipient + channel) |
| `:timeout` | string | No | Duration string (e.g. \"24h\") |

Notes:
- Use a stable `:key` so external events can resume the flow.
- Webhook waits are configured on triggers; waits pause execution.
- `:notify` is optional and depends on your workspace notification setup.
- For incoming webhooks, bind secret slots (`:type :secret`) via profile bindings to secure requests.
- When `:notify` is present, the wait record includes approval URL templates you can render and share.

`:notify` fields (example shape):
- `:channel` (e.g. `:email`)
- `:to` (recipient)
- `:subject` (optional)

Example:

```clojure
(flow/step :wait :webhook
           {:key "approval"
            :timeout "24h"
            :notify {:channel :email
                     :to "ops@example.com"
                     :subject "Approve run"}})
```

Approval URL template (from wait list output):

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

For notifications, pass `approvalUrl` as template data and reference it as `{approvalUrl}` in your message template.
