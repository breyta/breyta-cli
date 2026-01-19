# Persisted results (`:persist`)
Use `:persist {:type :blob ...}` on steps that may return large payloads (e.g., HTTP downloads). Persisted results include a resource reference plus inline metadata (and, for non-HTTP outputs, the original data).

Important:
- Large HTTP bodies are truncated; if truncation happens the step fails unless `:persist` is set.
- Persisted blob results include `:blob-ref` for downstream `:body-from-ref` / `:from-ref` usage.
- When `:persist` is used on HTTP-style results, the inline `:body` is omitted and replaced with `{:type :omitted :reason :persisted}`.

When to use:
- HTTP responses larger than the inline limit (10 KB).
- Files or payloads you want to reuse across steps or runs.

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

## Persist configuration
`:persist` is a map:
Note: `:persist true` is not supported; use the explicit map form.

```clojure
;; Customize blob persistence
{:persist {:type :blob
           :filename "report.json"
           :tier :default}}
```

You can also attach schema metadata for streamed exports so downstream consumers can interpret columns without inspecting the payload. It is optional, but it will enable additional formats later (e.g., Parquet/Arrow/Avro). Prefer normalized type names so we can map to other formats later.

```clojure
{:persist {:type :blob
           :stream true
           :format :csv
           :schema {:columns [{:name "id" :type :int64}
                              {:name "email" :type :string}
                              {:name "created_at" :type :timestamp}]
                    :validate false}}} ; disable validation (default validates first 10 rows)
```

Recommended normalized types:
- :string
- :int64
- :float64
- :bool
- :timestamp
- :date
- :json
- :decimal (use :precision/:scale)
- :bytes
- :uuid
- :time
- :array (use :items {:type ...})
- :struct (use :fields [{:name ... :type ...}])

Examples:
```clojure
{:schema {:columns [{:name "amount"
                     :type :decimal
                     :precision 12
                     :scale 2}
                    {:name "tags"
                     :type :array
                     :items {:type :string}}
                    {:name "address"
                     :type :struct
                     :fields [{:name "street" :type :string}
                              {:name "zip" :type :string}]}
                    {:name "payload"
                     :type :bytes}]}}
```

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
 :tier :default} ; 1MB write cap, default lifecycle
```

Ephemeral tier (streaming):
```clojure
{:type :blob
 :filename "large.pdf"
 :tier :ephemeral} ; 20MB write cap, ~12h TTL
```

Notes:
- `:ephemeral` is intended for short-lived streaming of external payloads (e.g., HTTP downloads) that you plan to upload/forward in the same flow.
- `:tier :ephemeral` is allowed only on streaming-friendly steps (e.g., `:http`).
- Database steps and other computed steps still materialize results in memory before persisting, so they cannot use `:ephemeral`.
- Ref load limits are tier-aware (e.g., larger `:http` body limits for `:ephemeral`).
- Inline results are capped; use `:persist` to avoid large inline payloads.
- Persisted refs are validated against the resource registry when loaded; expired or deleted refs fail fast.
- Persisted refs include size metadata for auditing.
