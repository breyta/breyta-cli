# Sleep step (`:sleep`)
Use for time-based delays in a workflow. For external events or approvals, use `:wait`.

Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:duration` | string | No | Duration string like "500ms", "10s", "5m", "2h", "1d" |
| `:seconds` | integer | No | Seconds to sleep |
| `:millis` | integer | No | Milliseconds to sleep |

Notes:
- Provide only one of `:duration`, `:seconds`, or `:millis`.
- Use `:duration` for minutes, hours, or days.

Example:

```clojure
(flow/step :sleep :delay {:duration "5m"})
```
