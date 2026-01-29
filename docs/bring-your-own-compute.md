# Bring Your Own Compute (call custom services from flows)

Flows are orchestration. If you need specialized computation (Python libraries, private code, heavy transforms), host a service and call it from a flow via an `:http` step.

## Recommended pattern: model your service as a connection slot

Use `:requires` with an `:http-api` slot so users bind base URL + credentials at activation time.
You can also include activation-only inputs in `:requires` via `{:kind :form ...}` (available under `:activation` in run input).

```clojure
{:slug :custom-compute
 :name "Custom Compute"
 :requires [{:slot :compute
             :type :http-api
             :label "Compute Service"
             :base-url "https://compute.example.com"
             :auth {:type :bearer}}]
 :triggers [{:type :manual :label "Run" :enabled true :config {}}]
 :flow
 '(let [{:keys [xs]} (flow/input)
        resp (flow/step :http :compute
                        {:connection :compute
                         :path "/v1/compute"
                         :method :post
                         :json {:xs xs}
                         :accept :json})]
    (:body resp))}
```

## Large artifacts: pass by URL (service downloads)

Large step outputs should be stored with `:persist {:type :blob ...}`. The step returns a blob ref map:

`{:type :blob :path "..." :url "https://..." :size-bytes N :data {...}}`

Prefer passing `(:url blob-ref)` to your compute service so it downloads the file directly.

```clojure
(let [export-ref (step :http :export
                       {:connection :api
                        :path "/export"
                        :persist {:type :blob :filename "export.json"}})
      resp (step :http :process-export
                 {:connection :compute
                  :path "/v1/process-export"
                  :method :post
                  :json {:export_url (:url export-ref)
                         :export_path (:path export-ref)
                         :size_bytes (:size-bytes export-ref)}
                  :accept :json})]
  (:body resp))
```

### Refreshing signed URLs (CLI)

Signed URLs expire. If you need to regenerate a URL, use the Resources API via CLI:

1) Find the resource URI for a run/step:
- `breyta resources workflow list <workflow-id>`
- `breyta resources workflow step <workflow-id> <step-id>`

Note: `workflow-id` is the primary identifier.

2) Get a fresh signed URL:
- `breyta resources url <resource-uri> --ttl 3600`

## Alternative: upload bytes from a blob ref (flow uploads)

If your compute service expects an upload (instead of pulling from a URL), the HTTP step can load blob content from storage:

- Raw body upload: `:body-from-ref <blob-ref>`
- Multipart upload: multipart part with `:from-ref <blob-ref>`

```clojure
(step :http :upload
      {:connection :compute
       :path "/v1/upload"
       :method :post
       :body-from-ref export-ref
       :headers {"Content-Type" "application/json"}
       :accept :json})
```

This path is intentionally size-limited; for large data, prefer “pass by URL”.

## Service contract (suggested)

Keep it simple:
- Request: `POST /v1/compute` JSON body (small inputs), or `POST /v1/process-export` with `{export_url: "..."}`
- Response: JSON body (keep it small unless you persist it as a blob and return a URL/ref)

If you need to return a large output, either:
- persist the result in your service and return a URL, or
- return a smaller summary + identifiers that the flow can use later.

## Related docs in this repo

- Resources CLI docs: `breyta docs resources`
