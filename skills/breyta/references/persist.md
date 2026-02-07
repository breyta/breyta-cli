# Persisted results (`:persist`)
Use `:persist` when step output should be stored explicitly instead of remaining inline. Two persistence types are available:
- `:persist {:type :kv ...}` for structured data you want to read across runs/flows.
- `:persist {:type :blob ...}` for file/blob payloads and ref-based transfer.

Important:
- Large HTTP bodies are truncated; if truncation happens the step fails unless `:persist` is set.
- Persisted blob results include `:blob-ref` for downstream `:body-from-ref` / `:from-ref` usage.
- When `:persist` is used on HTTP-style results, the inline `:body` is omitted and replaced with `{:type :omitted :reason :persisted}`.
- KV persistence requires an explicit `:key` and can include optional `:ttl` (seconds).

When to use:
- HTTP responses larger than the inline limit (256 KB).
- Outputs with unknown or unbounded size (exports, paginated APIs, generated files), even when average responses are small.
- Files or payloads you want to reuse across steps or runs.
- Cross-flow or cross-run handoff of structured state (counts, cursors, rollups, checkpoints).

## Pick the right persistence type
- Use `:type :kv` for structured state you will query by key from another flow/run.
- Use `:type :blob` for large payloads/files where downstream steps consume refs.
- Keep `flow/call-flow` for direct child execution. If the child flow requires slot bindings (`:requires`), missing child profile context can fail with `requires a flow profile, but no profile-id in context`.

How it works:
- The step result includes a ref (metadata + storage path) and inline metadata.
- Downstream steps can load the ref by passing it to steps that accept refs (loads into memory).
- Blob persistence supports tiers via `:tier` (see below).

Example:

```clojure
(def report
  (flow/step :http :download-report
             {:connection :api
              :path "/reports/latest"
              :method :get
              :persist {:type :blob}}))

;; Pass the ref to another step that can load it (e.g., HTTP body from ref)
(flow/step :http :upload-report
           {:connection :storage
            :path "/uploads"
            :method :post
            :body-from-ref (:blob-ref report)})
```

## Cross-flow handoff with KV
Use this when one flow produces structured data and another flow consumes it later.

Writer flow:

```clojure
(let [{:keys [workspace-id period page]} (flow/input)
      key (str "metering:usage_pages:" workspace-id ":" period ":" page)
      _ (flow/step :http :fetch-page
                   {:connection :api
                    :path (str "/usage?page=" page)
                    :persist {:type :kv
                              :key key
                              :ttl 1209600}})]
  {:key key})
```

Reader flow:

```clojure
(let [{:keys [workspace-id period page]} (flow/input)
      key (str "metering:usage_pages:" workspace-id ":" period ":" page)
      kv-result (flow/step :kv :load-page
                           {:operation :get
                            :key key})
      page (:value kv-result)]
  (if page
    page
    (throw (ex-info "KV page not found" {:key key}))))
```

Notes:
- KV keys are workspace-scoped by the runtime; use deterministic keys per period/workspace.
- KV keys only allow `a-z`, `A-Z`, `0-9`, `_`, `-`, and `:`. Avoid `/` and other separators.
- `:kv` `:get` returns a wrapper map; read `:value` for payload data.
- Prefer KV for rollup state and pagination checkpoints; prefer blob refs for binary/large documents.
- For Firestore over `:http-api`, bearer auth must be a valid OAuth2 access token. `ACCESS_TOKEN_TYPE_UNSUPPORTED` means the token type is wrong.

## Persist configuration
`:persist` is a map:
Note: `:persist true` is not supported; use the explicit map form.

```clojure
;; Customize blob persistence
{:persist {:type :blob
           :filename "report.json"
           :tier :retained}}
```

Optional signed URL config for downloads:
```clojure
{:persist {:type :blob
           :tier :ephemeral
           :signed-url {:ttl-seconds 900}}}
```

## Step-by-step: download then upload with refs
Use this when a step returns large data and you need to forward it.

1) Persist the download:
```clojure
(let [download (flow/step :http :download
                          {:connection :api
                           :path "/reports/weekly"
                           :persist {:type :blob :tier :ephemeral}})]
  ...)
```

2) Reuse the ref in a later step:
```clojure
(let [download (flow/step :http :download
                          {:connection :api
                           :path "/reports/weekly"
                           :persist {:type :blob :tier :ephemeral}})
      blob-ref (:blob-ref download)
      upload (flow/step :http :upload
                        {:connection :api
                         :path "/uploads"
                         :method :post
                         :body-from-ref blob-ref})]
  {:upload upload})
```

3) For multipart uploads, use `:from-ref`:
```clojure
(flow/step :http :multipart-upload
           {:connection :api
            :path "/uploads"
            :method :post
            :multipart [{:name "file"
                         :filename "report.pdf"
                         :content-type "application/pdf"
                         :from-ref blob-ref}]})
```

## Streaming HTTP downloads (no in-memory buffering)
When you persist an HTTP download as a blob, Breyta can stream the response body directly into object storage (rather than reading the entire response into memory).

Recommended pattern for large binaries:

```clojure
(flow/step :http :download-video
           {:url "https://example.com/video.mp4"
            :method :get
            :response-as :bytes
            :client-opts {:max-response-bytes (* 50 1024 1024)} ; must be >= expected size
            :persist {:type :blob
                      :tier :ephemeral
                      :filename "video.mp4"
                      :signed-url true}})
```

Notes:
- Only raise `:max-response-bytes` like this when you also use `:persist {:type :blob ...}` + `:response-as :bytes`. For non-streaming responses (e.g. JSON parsing), the body is buffered in memory up to `:max-response-bytes`.
- The effective maximum size is bounded by platform limits (persist tier `:max-write-bytes`, and response limits).

## Persisting database results
Persist can also be used on data-producing steps like `:db` to avoid large inline payloads.

```clojure
(def rows-ref
  (flow/step :db :export-users
             {:connection :db
              :query "select * from users where active = true"
              :persist {:type :blob}}))

;; Upload the persisted ref (loads into memory before sending).
(flow/step :http :upload-users
           {:connection :storage
            :path "/uploads/users.json"
            :method :post
            :body-from-ref (:blob-ref rows-ref)})
```

## Using resource URLs (refs) directly
Persisted results expose a resource URI (e.g., `res://...`). You can read them with the CLI or pass them into steps that accept refs.

### Uploading with refs
When a step returns a ref, pass it via :body-from-ref or :multipart :from-ref:

```clojure
(let [report-ref (flow/step :http :download
                            {:connection :api
                             :path "/reports/latest"
                             :persist {:type :blob}})]
  (flow/step :http :upload
             {:connection :storage
              :path "/uploads"
              :method :post
              :body-from-ref (:blob-ref report-ref)}))

;; Multipart upload from storage ref
(let [file-ref (flow/step :http :download
                          {:connection :api
                           :path "/files/latest.pdf"
                           :persist {:type :blob}})]
  (flow/step :http :upload
             {:connection :storage
              :path "/uploads"
              :method :post
              :multipart [{:name "file"
                           :filename "latest.pdf"
                           :content-type "application/pdf"
                           :from-ref (:blob-ref file-ref)}]}))
```

CLI read:

```bash
breyta resources read res://<resource-id>
```

Notes:
- `breyta resources ...` requires API mode (set `BREYTA_API_URL`, `BREYTA_WORKSPACE`, and authenticate via `breyta auth login` or set `BREYTA_TOKEN` in dev).
- Use `:body-from-ref` for HTTP bodies; `:body` will inline `:data` instead of loading from storage.
- For multipart, use `:from-ref` on each part.
- If a ref was created in a non-default bucket, include `:bucket` inside `:from-ref`.
- Resource URIs identify the blob; size is stored in resource metadata (not in the URI itself).

## Blob tiers (`:tier`)
Blob persistence supports tiers that control size limits and lifecycle.

Default tier:
```clojure
{:type :blob
 :filename "report.json"
 :tier :retained} ; ~25MB write cap, 90-day lifecycle (defaults may vary by deployment)
```

Ephemeral tier (streaming):
```clojure
{:type :blob
 :filename "large.pdf"
 :tier :ephemeral} ; ~50MB write cap, ~12h TTL and 1-day lifecycle (defaults may vary by deployment)
```

Notes:
- `:ephemeral` is intended for short-lived streaming of external payloads (e.g., HTTP downloads) that you plan to upload/forward in the same flow.
- `:tier :ephemeral` is allowed only on streaming-friendly steps (e.g., `:http`).
- Database steps and other computed steps still materialize results in memory before persisting, so they cannot use `:ephemeral`.
- Ref load limits are tier-aware (e.g., larger `:http` body limits for `:ephemeral`), and ref loading still happens in memory (e.g., `:body-from-ref` / multipart `:from-ref`).
- Inline results are capped; use `:persist` to avoid large inline payloads.
- Persisted refs are validated against the resource registry when loaded; expired or deleted refs fail fast.
- Persisted refs include size metadata for auditing.
