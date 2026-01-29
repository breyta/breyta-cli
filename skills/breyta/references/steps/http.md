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
| `:response-as` | keyword | No | `:auto`, `:json`, `:text`, `:bytes` (default `:auto`) |
| `:persist` | map | No | Store response as reference (`{:type :blob ...}`) |
| `:retry` | map | No | Retry policy (see flow limits) |

Notes:
- When both `:json` and `:body` are set, `:json` wins.
- `:response-as :auto` uses the response `Content-Type` to choose `:json`, `:text`, or `:bytes`.
- Results are inlined up to the 50 KB limit; larger payloads require `:persist`.
- Binary responses are not inlined; use `:persist {:type :blob ...}` for any binary body.
- If the response is truncated, the step fails unless `:persist {:type :blob ...}` is set.
- Use `:persist {:type :blob}` for large payloads; downstream steps can load refs.
- Templates only cover request shape; step-level keys like `:persist` stay on the step.
- Auth must use secret references. Inline tokens or API keys are rejected.
- Prefer connection auth (`:requires` with `:auth`), or set step-level auth with `:auth {:type :bearer|:api-key :secret-ref :my-secret}`.

## Google service account auth (`:google-service-account`)
Use this when you need unattended OAuth access to Google APIs on a schedule (e.g., Drive folder sync). The flow mints access tokens from a **service account JSON** secret.

Example (Google Drive list):

```clojure
(flow/step :http :list-drive-files
           {:type :http
            :title "List Drive files"
            :url "https://www.googleapis.com/drive/v3/files"
            :method :get
            :query {:q "'<folder-id>' in parents and trashed = false"
                    :fields "nextPageToken,files(id,name,mimeType,modifiedTime,size,driveId)"
                    :pageSize 1000
                    :supportsAllDrives "true"
                    :includeItemsFromAllDrives "true"}
            :auth {:type :google-service-account
                   :secret-ref :google-drive-service-account
                   :scopes ["https://www.googleapis.com/auth/drive.readonly"
                            "https://www.googleapis.com/auth/drive.metadata.readonly"]}})
```

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
            :persist {:type :blob}})
```
