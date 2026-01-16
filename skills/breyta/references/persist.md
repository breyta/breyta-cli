# Persisted results (`:persist`)
Use `:persist true` on steps that may return large payloads (e.g., HTTP downloads). Persisted results are stored as resource references instead of inline data.

When to use:
- HTTP responses larger than the inline limit (10 KB).
- Files or payloads you want to reuse across steps or runs.

How it works:
- The step result becomes a ref (resource URI + metadata) instead of inline data.
- Downstream steps can load the ref using helper functions or by passing the ref to steps that accept refs.

Example:

```clojure
(def report
  (flow/step :http :download-report
             {:connection :api
              :path "/reports/latest"
              :method :get
              :persist true}))

;; Pass the ref to another step that can load it (e.g., HTTP body from ref)
(flow/step :http :upload-report
           {:connection :storage
            :path "/uploads"
            :method :post
            :body report})
```

## Using resource URLs (refs) directly
Persisted results expose a resource URI (e.g., `res://...`). You can read them with the CLI or pass them into steps that accept refs.

### Uploading with refs
When a step returns a ref, pass it as the body to upload without loading it into memory:

```clojure
(let [report-ref (flow/step :http :download
                            {:connection :api
                             :path "/reports/latest"
                             :persist true})]
  (flow/step :http :upload
             {:connection :storage
              :path "/uploads"
              :method :post
              :body report-ref}))
```

CLI read:

```bash
breyta resources read res://<resource-id>
```

Notes:
- Some steps accept refs directly (e.g., HTTP body), others expect you to load the data first.
- Resource URIs include size metadata so you can decide whether to persist or inline.

Notes:
- Inline results are capped; use `:persist` to avoid large inline payloads.
- Persisted refs include size metadata for auditing.
