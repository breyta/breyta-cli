# Authoring reference
## Flow file format
- A flow file is a single EDN map literal (Clojure data), not JSON.
- The server reads it with `*read-eval*` disabled.
- Slug format: `^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`.

Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:slug` | keyword | Yes | Non-namespaced keyword (URL-safe) |
| `:name` | string | Yes | Display name |
| `:description` | string | No | Help text |
| `:icon` | keyword | No | UI icon (if supported) |
| `:tags` | vector | No | Tags for grouping |
| `:concurrency` | map | Yes | See below |
| `:requires` | vector | No | Connection slots and activation inputs |
| `:templates` | vector | No | Template payloads (see `./templates.md`) |
| `:functions` | vector | No | Function templates for `:function` steps |
| `:triggers` | vector | Yes | Include a `:manual` trigger for discoverability |
| `:flow` | form | Yes | The orchestration DSL |

## `:flow`
The flow body must be quoted so it is treated as data:

```clojure
:flow '(let [input (flow/input)] ...)
```

Do not leave it unquoted:

```clojure
;; Wrong: evaluated at read time
:flow (let [input (flow/input)] ...)
```

## `:requires`
Use `:requires` to declare connection slots and activation form inputs. Flows with `:requires` must have bindings applied and be activated.

### `:kind :connection`
Core fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:slot` | keyword | Yes | Used as `:connection` in steps |
| `:type` | keyword | Yes | `:http-api`, `:llm-provider`, `:database`, `:blob-storage`, `:kv-store`, `:secret` |
| `:label` | string | Yes | UI label |
| `:optional` | boolean | No | Use `flow/slot-bound?` |
| `:base-url` | string | If `:http-api` | Base URL |
| `:auth` | map | If not `:secret` | `{:type :api-key|:bearer|:basic}` |
| `:oauth` | map | Optional | OAuth config |

Example:

```clojure
:requires [{:slot :crm
            :type :http-api
            :label "CRM API"
            :base-url "https://api.example.com"
            :auth {:type :bearer}}
           {:slot :ai
            :type :llm-provider
            :label "AI Provider"
            :auth {:type :api-key}
            :optional true}]
```

### `:kind :form`
Activation-only inputs (no connection created):

```clojure
{:kind :form
 :label "Activation inputs"
 :fields [{:key :region :label "Region" :field-type :select :options ["EU" "US"]}
          {:key :batch-size :label "Batch size" :field-type :number :default 500}]}
```

Form fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `:key` | keyword | Yes | Input key under `:activation` |
| `:label` | string | Yes | UI label |
| `:field-type` | keyword | Yes | `:string`, `:text`, `:number`, `:boolean`, `:select`, `:date`, `:email`, `:textarea`, `:password`, `:secret` |
| `:required` | boolean | No | Default false |
| `:options` | vector | If `:select` | Options |

### `:type :secret`
Use `:type :secret` for single-value secrets (webhook signing, tokens used in flow logic). Avoid for HTTP APIs.

## `:concurrency`
Both `:type` and `:on-new-version` are required.

| Config | Description |
| --- | --- |
| `{:type :singleton :on-new-version :supersede}` | One instance at a time. New version cancels current |
| `{:type :singleton :on-new-version :drain}` | One instance at a time. Wait for current to finish |
| `{:type :singleton :on-new-version :coexist}` | One instance at a time. Both versions can run |
| `{:type :keyed :key-field :user-id :on-new-version :supersede}` | One instance per key |
| `{:type :keyed :key-field :user-id :on-new-version :drain}` | One instance per key, drain on new version |

Notes:
- Use `:supersede` while iterating to avoid blocking on long-running or waiting runs.
- `:drain` can block new runs if a run is stuck or waiting. Cancel the run or switch to `:supersede` during development.
- Concurrency config is static. Do not use expressions in `:concurrency`.
- `:key-field` must be a keyword or nested path vector that exists in `flow/input` (for example `:email` or `[:event :id]`).
- Use `:supersede` when a newer run should cancel the older one (webhooks, retries, refresh jobs).
- Use `:drain` when in-flight work must finish and it is safe to queue new runs (billing, uploads, sequential processing).

## Common pitfalls
- Waits are event-based. They pause for external signals (webhooks, CLI commands), not timers. For delays, use schedule triggers.
- Singleton workflows can get stuck if a run errors or waits. Use `:on-new-version :supersede` for fresh starts.
- Keep flow bodies simple. Put logic in `:function` steps and keep orchestration minimal.

## `:triggers`
Common types:
- `:manual` with `:label` and optional `:config`
- `:schedule` with `:config {:cron "..." :timezone "..."}`
- `:event` with `:config {:source :webhook ...}`

Notes:
- Keep at least one enabled `:manual` trigger so the flow is runnable from the UI.
- Schedule cron and timezone are static strings in the flow definition. Do not compute them at runtime.
- Webhook triggers use `:event` with `:source :webhook`; the webhook path is generated at activation.
- The payload arrives in `flow/input`.
- Webhook secrets are declared as `:requires` slots of `:type :secret` and bound via profiles.
- Webhook auth is required; `:auth {:type :none}` is not allowed for webhook triggers.

Example webhook trigger + secret slot:

```clojure
{:requires [{:slot :webhook-secret
             :type :secret
             :label "Webhook Secret"}]
 :triggers [{:type :event
             :label "Inbound webhook"
             :config {:source :webhook
                      :auth {:type :api-key
                             :secret-ref :webhook-secret}}}]}
```

Bindings snippet (profile EDN):

```edn
{:bindings {:webhook-secret {:secret :generate}}}
```

Webhook setup checklist:
1) Generate a bindings template and add the secret slot.
2) Apply bindings to generate/store the secret.
3) Copy the webhook URL from the trigger UI or API.
4) Send requests with the secret (header name depends on trigger auth config).

Webhook endpoints:
- Public (external senders): `POST /:workspace-id/events/<path>`
- Draft webhook endpoint (workspace-auth): `POST /:workspace-id/api/events/draft/<path>`
- Validate endpoint (workspace-auth): `POST /:workspace-id/api/events/validate/<path>`

CLI validation:
- `breyta webhooks send --validate-only --path <path> --json '{...}'`
- Add `--persist-resources` to store multipart file parts during validation.

Webhook auth schemes:

| Type | Config | How to send |
| --- | --- | --- |
| `:api-key` | `{:type :api-key :secret-ref :webhook-secret :header "X-API-Key"}` | `X-API-Key: <secret>` (default header is `X-API-Key`) |
| `:bearer` | `{:type :bearer :secret-ref :webhook-secret}` | `Authorization: Bearer <secret>` |
| `:basic` | `{:type :basic :username "user" :secret-ref :webhook-secret}` | `Authorization: Basic <base64(user:secret)>` |
| `:hmac-sha256` | `{:type :hmac-sha256 :secret-ref :webhook-secret :header "X-Signature"}` | `X-Signature: <base64(hmac)>` computed over raw request bytes |
| `:ip-allowlist` | `{:type :ip-allowlist :allow ["203.0.113.10"]}` | Must match `X-Forwarded-For`/`X-Real-IP` |

Notes:
- For HMAC, compute the signature over the exact raw request body (JSON bytes or multipart bytes).
- For multipart webhooks, use HMAC over the full multipart payload bytes.

Example curl (api-key header):

```bash
curl -X POST "https://flows.breyta.ai/<workspace-id>/events/webhooks/orders" \
  -H "X-API-Key: <webhook-secret>" \
  -H "Content-Type: application/json" \
  -d '{"orderId":"123"}'
```

## `:flow` rules and determinism
- Keep flow body code deterministic; avoid `rand`, current time, UUIDs, or external calls.
- Use `flow/step` for side effects; data transforms belong in `:function` steps.
- Control flow is plain Clojure (`let`, `if`, `cond`), but avoid `map`/`reduce` in the flow body.

Deterministic helpers:
- `flow/now-ms` for current workflow time.
- `flow/elapsed?` for elapsed time checks.
- `flow/backoff` for deterministic backoff intervals (accepts `"10s"`, `"5m"`, or ms).

## Polling with `flow/poll`
Use `flow/poll` to wrap a step in a deterministic loop with a sleep between attempts.
It expands to a loop + `:sleep`, so it satisfies loop safety requirements.

Required config:
- `:interval` (duration string or ms)
- `:timeout` or `:max-attempts`
- `:return-on` (function or predicate symbol)

Optional config:
- `:backoff` (`:type :exponential|:linear|:constant`, `:factor`, `:step`, `:max`)
- `:abort-on` (`:error?` defaults true, `:status` set)
- `:id` (custom step id for the sleep step)

Example:

```clojure
(flow/poll
  {:interval "10s"
   :timeout "10m"
   :backoff {:type :exponential :factor 2 :max "2m"}
   :abort-on {:status #{400 401 403} :error? true}
   :return-on (fn [result] (true? (get-in result [:data :ready])))}
  (flow/step :http :check
             {:connection :api
              :path "/jobs/123"}))
```

Notes:
- `:return-on` can be a function literal or a named predicate symbol.
- Use `:backoff {:type :linear :step "5s"}` for linear backoff.
- Hard limits: `:timeout` max is 1h; `:max-attempts` max is 100.

## Step-by-step: poll with error handling
Use this when an external job can fail and you want to abort early.

1) Start the job:
```clojure
(let [create (flow/step :http :create-job
                        {:connection :api
                         :path "/jobs"
                         :method :post
                         :json {:queued-ms 200 :processing-ms 900}
                         :response-as :json})
      job-id (get-in create [:body :id])]
  ...)
```

2) Poll until completed or error:
```clojure
(flow/poll
  {:interval "200ms"
   :timeout "5s"
   :return-on (fn [resp]
                (let [status (get-in resp [:body :status])]
                  (cond
                    (= status "completed") true
                    (= status "error") (throw (ex-info "job failed" {:error (get-in resp [:body :error])}))
                    :else false)))}
  (flow/step :http :job-status
             {:connection :api
              :path (str "/jobs/" job-id)
              :method :get
              :response-as :json}))
```

## Result handling
- Small results are returned inline; large results become refs.
- Use `:persist {:type :blob}` on `:http` steps when you expect large payloads.
- Use `:persist {:type :kv :key "..." :ttl ...}` for structured state that should be read across runs/flows.
- See `./persist.md` for how refs flow to downstream steps.

## Cross-flow composition
Choose composition based on binding/profile needs:
- `flow/call-flow`: use when you need synchronous child return values and child binding context is already valid.
- KV handoff (`:persist {:type :kv ...}` + `:kv {:operation :get ...}`): use when producer/consumer are separate flows or run contexts.

Profile caveat:
- Slot-bound child flows (`:requires`) can fail if child profile context is not available at call time.
- Symptom: `requires a flow profile, but no profile-id in context`.
- Safe default for billing/metering pipelines: split producer/consumer flows and exchange deterministic KV keys.
- KV keys allow only `a-z`, `A-Z`, `0-9`, `_`, `-`, and `:`. Avoid `/` in key construction.
- `:kv` `:get` returns a wrapper map (`{:success ... :value ...}`); consume `:value`.
- For Firestore HTTP queries, bearer auth must be a valid OAuth2 access token; `ACCESS_TOKEN_TYPE_UNSUPPORTED` indicates the wrong token type.

## Metadata labels for UI
Add labels to branches and loops to make the visual editor clearer.

```clojure
(if ^{:label "Has user"} (:user input)
  (flow/step :http :fetch {:connection :api :path "/users"})
  (flow/step :http :fallback {:connection :api :path "/users/guest"}))
```

## Functions (`:functions`)
Use `:function` steps for sandboxed transforms. For reuse, define flow-local functions.

Sandbox helpers (safe; pure/deterministic) are available under `breyta.sandbox`:
`base64-encode` `(string|bytes) -> string`, `base64-decode` `(string|bytes) -> string`,
`base64-decode-bytes` `(string|bytes) -> bytes`, `hex-encode` `(string|bytes) -> string`,
`hex-decode` `(string) -> string`, `hex-decode-bytes` `(string) -> bytes`,
`sha256-hex` `(string|bytes) -> string`, `hmac-sha256-hex` `(key,value) -> string`,
`uuid-from` `(string) -> uuid`, `uuid-from-bytes` `(string|bytes) -> uuid`,
`parse-instant` `(string) -> java.time.Instant`, `format-instant` `(Instant) -> string`,
`format-instant-pattern` `(Instant, pattern) -> string`,
`instant->epoch-ms` `(Instant) -> long`, `epoch-ms->instant` `(long) -> Instant`,
`duration-between` `(Instant, Instant) -> Duration`,
`truncate-instant` `(Instant, unit) -> Instant` (unit: `:seconds|:minutes|:hours|:days`),
`instant-plus` `(Instant, amount, unit) -> Instant` (unit: `:millis|:seconds|:minutes|:hours|:days`),
`instant-minus` `(Instant, amount, unit) -> Instant`,
`url-encode` `(string) -> string`, `url-decode` `(string) -> string`.

Limited Java interop is also allowed in `:function` code (small allowlist): `java.time.*`,
`java.time.format.DateTimeFormatter`, `java.time.temporal.{ChronoUnit,TemporalAdjusters}`,
`java.util.{UUID,Base64}`, `java.math.{BigInteger,BigDecimal}`. Prefer `breyta.sandbox`.

```clojure
:functions [{:id :summarize-user
             :language :clojure
             :code "(fn [input] {:ok true :input input})"}]

(flow/step :function :summarize-user
           {:input {:user user}
            :ref :summarize-user})
```

## Input keys from `--input`
Inputs provided via `--input '{...}'` arrive as strings, but the runtime normalizes so both string and keyword keys work.

## Limits (author-facing)
Common limits to plan around (see `breyta/libraries/flows/config/limits.clj` for full list):

### Flow definition and templates
- Flow definition size: 150 KB max.
- Templates are packed to blob storage on deploy; large prompts/SQL should live in templates.

### Runtime execution
- Step executions per run: 100
- HTTP requests per run: 50
- LLM tokens per run: 100,000
- Workflow duration: 7 days

### Per-step payloads
- Inline result threshold: 256 KB (larger results require `:persist` or will error)
- Max step result: 1 MB
- HTTP response size: 10 MB
- DB max rows: 10,000

Tips:
- Keep results small; return summaries and persist large payloads.
- If output size is unknown/unbounded (exports, paginated APIs, file downloads), default to `:persist` early rather than relying on inline.
- Use `:persist {:type :blob}` on `:http` when you need large response bodies.
- For cross-run/cross-flow structured state, prefer `:persist {:type :kv ...}` with deterministic keys.

## Final output artifacts (UI)

Flows return a final output value (the value of the `:flow` form). The UI can render this as a user-facing artifact (outside the debug panel) when you wrap it in a small viewer envelope.

Details: `./output-artifacts.md`
