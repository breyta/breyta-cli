# HTTP step (`:http`)
Use for HTTP APIs. Prefer `:connection` with a `:http-api` slot.

Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:connection` | keyword/string | Yes (unless `:url`) | Slot or connection id |
| `:url` | string | Yes (unless `:connection`) | Full URL (bypasses `:path`) |
| `:path` | string | No | Appended to base URL |
| `:method` | keyword | No | `:get`, `:post`, `:put`, `:patch`, `:delete` |
| `:query` | map | No | Query params |
| `:headers` | map | No | Header map |
| `:json` | any | No | JSON body (sets content-type) |
| `:body` | any | No | Raw body |
| `:persist` | boolean | No | Store response as reference |
| `:retry` | map | No | Retry policy (see flow limits) |

Notes:
- When both `:json` and `:body` are set, `:json` wins.
- Use `:persist true` for large payloads; downstream steps can load refs.
- Templates only cover request shape; step-level keys like `:persist` stay on the step.

Example:

```clojure
;; In the flow definition:
;; :templates [{:id :download-report
;;              :type :http-request
;;              :request {:path "/reports/latest" :method :get}}]

(flow/step :http :get-users
           {:connection :api
            :path "/users"
            :method :get
            :query {:limit 10}
            :headers {"X-Request-Id" (:request-id (flow/input))}
            :retry {:max-attempts 3
                    :backoff-ms 500}})

(flow/step :http :download-report
           {:connection :api
            :template :download-report
            :persist true})
```
