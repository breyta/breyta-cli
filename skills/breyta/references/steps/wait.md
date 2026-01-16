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
