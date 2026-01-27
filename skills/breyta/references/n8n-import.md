# n8n -> Breyta import (CLI template)

Purpose: import an n8n workflow JSON into a **runnable Breyta flow ASAP** with best-effort translations.
Keep all prompts, code, request bodies, and messages **like-for-like** in the output.

This reference is written for coding agents that generate the EDN flow file.

## Import loop (plan-first)
1) Parse n8n JSON and list nodes, connections, credentials, and expressions.
2) Map triggers and nodes to Breyta equivalents (tables below).
3) Mark unsupported/custom nodes and choose a runnable fallback.
4) Generate `:requires` slots from credentials (strip keys/secrets).
5) Emit `:templates` / `:functions` and the `:flow` orchestration.
6) Validate/compile, then draft-run.

**Do not review or critique the workflow design.** The importer's job is translation, not improvement.  
If something is missing or unclear, add a `TODO(n8n-import)` note instead of suggesting optimizations.
Do not add “conversion notes” that propose changes (e.g., persistence, schedules, model tweaks) unless asked.

## Import loop (step-by-step, strict)
Use this when reliability matters more than speed
1) Translate exactly one node into a Breyta step
2) Run the step in isolation with a small input and inspect the output shape
3) Fix shape and size issues before translating the next node
4) Only then move on to the next node in the graph

This avoids mismatched data shapes and large payload failures later in the flow

## Persist-first defaults
Assume many HTTP steps will exceed the inline 50 KB limit
- Add `:persist {:type :blob}` to HTTP steps unless you can confirm the payload is small
- `:persist` removes the inline `:body`. Downstream steps must read the blob or use `:body-from-ref` or `:from-ref`
- If a downstream function expects inline JSON, add a read step that restores the same shape as the original HTTP response

## Naming rules
- Step id: n8n node `name` -> kebab-case keyword (no `n8n-` prefix).
- Normalize: lower-case, then replace non `[a-z0-9-]` with `-`, collapse repeats, trim `-`.
- If the name is empty or starts with a non-letter, prefix with `step-`.
- De-dupe: append `-2`, `-3`, etc for collisions.
- Template ids: `:<step-id>-request`, `:<step-id>-prompt`, `:<step-id>-sql`.
- Function ids: `:<step-id>-fn`.
- Preserve the original n8n node `name` in a `;;` comment above the step.

## Keep content like-for-like
- Prompts/messages/HTTP bodies: copy verbatim into `:templates` (preferred) or `:json`/`:body`.
- n8n expressions (`{{$json...}}`, `{{$node...}}`) do **not** run in Breyta.
  - Translate expressions into a `:function` step that computes the needed values.
  - Preserve the original expression in a comment block when translation is non-trivial.
- Prefer a **per-node “prep” function** that computes all expression-derived fields, then pass its output into the step/template.
- Code nodes: attempt a real translation to a Breyta `:function` (Clojure) so the flow runs as intended.
  - Only fall back to `(fn [input] input)` when translation is unclear; keep the original code verbatim in comments.
- Avoid Java interop in `:function` steps (e.g., `java.time.*`). If time/date is needed, add an activation input (e.g., `:weekday`) or require it in `flow/input`, and add a TODO if missing.

Example comment block:
```clojure
;; TODO(n8n-import): port JS code below to Clojure
;; --- begin n8n code ---
;; <verbatim code here>
;; --- end n8n code ---
```

Example expression translation note:
```clojure
;; TODO(n8n-import): translated n8n expression {{$json.user.id}} to (get-in input [:user :id])
```

### Expression translation quick map (best-effort)
Use a `:function` step to compute these values:
- `{{$json}}` -> `input`
- `{{$json.foo}}` -> `(get input :foo)` / `(get-in input [:foo])`
- `{{$json.foo.bar}}` -> `(get-in input [:foo :bar])`
- `{{$node[\"Node Name\"].json}}` -> `<node-binding>`
- `{{$node[\"Node Name\"].json.foo}}` -> `(get-in <node-binding> [:foo])`
- `{{$json.foo + 1}}` -> `(+ (get input :foo) 1)`
- `{{$json.foo ? \"yes\" : \"no\"}}` -> `(if (get input :foo) \"yes\" \"no\")`
- `{{$json.a + $json.b}}` -> `(+ (get input :a) (get input :b))`
- `{{$now}}` -> `(flow/now-ms)` (confirm time semantics)
- `{{$today}}` -> derive from `(flow/now-ms)` (TODO if date formatting is needed)

If an expression uses n8n-only helpers (e.g., `$items()`, `$runIndex`, `$prevNode`), add a TODO and compute it explicitly in a function.

## Items vs maps (n8n items -> Breyta data)
n8n nodes operate on **arrays of items**; Breyta steps receive a **map**.
Import rule:
- Wrap n8n items as `{:items [...]}` in Breyta.
- Use `:function` steps to map/filter/reduce items explicitly (data transformation).
- Default for side‑effects: iterate per item with `for` unless the n8n node is explicitly “first item only”.

Example:
```clojure
{:items [{:id 1} {:id 2}]}
```
```clojure
(flow/step :function :map-items
           {:ref :map-items-fn
            :input {:items items}})
```

Example (side‑effects per item):
```clojure
(let [items (:items input)
      results (for [item items]
                (flow/step :http :post-item
                           {:connection :api
                            :path "/items"
                            :method :post
                            :json item}))]
  {:results (vec results)})
```

## Common migration pitfalls (watch for these)
- Query params: move URL query strings into `:query` so the runtime includes them
- Auth placement: keep header vs query as in n8n, do not guess
- Lazy seqs: wrap `for` output with `vec` before passing into `:function` steps
- Java interop: avoid `java.time.*` in `:function` code, use `flow/now-ms` and explicit inputs
- Regex: do not use `\\s`, use explicit character classes
- Persisted HTTP: inline `:body` is omitted, add a read step or `:body-from-ref` if downstream expects JSON

Split In Batches:
- Use `(partition-all n items)` in a `:function` step, then `for` over batches.

## Graph translation (multi-input / branching)
Convert the n8n graph into a topologically ordered `let` form:
1) Build a DAG from connections.
2) Order nodes so inputs are bound before use.
3) For multi-input nodes, pass a map of upstream results.

Example multi-input:
```clojure
(let [a (flow/step :http :fetch-a {...})
      b (flow/step :http :fetch-b {...})
      merged (flow/step :function :merge
                        {:ref :merge-fn
                         :input {:left a :right b}})]
  merged)
```

Branching:
```clojure
(if condition
  (flow/step :http :true-branch {...})
  (flow/step :http :false-branch {...}))
```

Loops / Split in Batches:
- Use `for` for orchestration over items/batches.
- Use `:function` to build batches (`partition-all`) and to reduce results.
- If you cannot preserve the loop semantics safely, add a TODO and keep a single-pass fallback.

## Trigger mapping
| n8n trigger | Breyta trigger | Notes |
| --- | --- | --- |
| Manual Trigger | `:manual` | Always include at least one `:manual` trigger. |
| Webhook Trigger | `:event` with `:config {:source :webhook ...}` | See `./webhooks.md`. Requires auth + secret slot. |
| Cron / Schedule | `:schedule` | Map cron/timezone if present; else TODO. |
| Interval / Polling | `:schedule` | Convert to cron if possible; otherwise TODO. |
| Service triggers (Slack, GitHub, etc.) | `:event` (webhook) or `:schedule` | If the service supports webhooks, default to webhook + TODO. |

Minimal webhook trigger + secret:
```clojure
:requires [{:slot :webhook-secret
            :type :secret
            :secret-ref :webhook-secret
            :label "Webhook Secret"}]
:triggers [{:type :event
            :label "Inbound webhook"
            :enabled true
            :config {:source :webhook
                     :auth {:type :api-key
                            :secret-ref :webhook-secret}}}]
```

## Step mapping (best-effort)
| n8n node | Breyta step | How to translate |
| --- | --- | --- |
| HTTP Request | `:http` | Prefer `:template` with `:request` (URL -> base-url + path). |
| Webhook Response | `:function` | Return a response map: `{:status 200 :headers {} :body ...}`. See `./webhooks.md#webhook-response-maps`. |
| Set | `:function` | Map/merge fields into a new map. |
| Code (JS/Python) | `:function` | Translate to Clojure; only use placeholder + TODO when unclear. |
| IF / Switch | flow `if`/`cond` + `:function` | If expression maps cleanly, use it; else TODO and default to a **safe** false branch (or pass-through) to avoid unintended side effects. |
| Merge | `:function` | `(merge a b)` or custom merge logic. |
| Wait / Delay | `:wait` | Use `:timeout` or placeholder with TODO. |
| Database (Postgres/MySQL/etc.) | `:db` | Put SQL in `:template` and pass `:params`. |
| LLM / OpenAI / AI nodes | `:llm` | Convert prompt/messages to `:template` and map inputs. |
| NoOp / Start | `:function` | Identity function. |

### HTTP Request translation
If the n8n node has a full URL, split it:
- `:base-url` = scheme + host
- `:path` = path + query (or use `:query` map)

Example:
```clojure
:requires [{:slot :api
            :type :http-api
            :label "Imported API"
            :base-url "https://api.example.com"
            :auth {:type :api-key}}]

:templates [{:id :fetch-user-request
             :type :http-request
             :request {:path "/users/{{id}}"
                       :method :get
                       :headers {"Accept" "application/json"}}}]

(flow/step :http :fetch-user
           {:connection :api
            :template :fetch-user-request
            :data {:id user-id}})
```

## Credentials -> :requires (strip secrets)
n8n exports often include credential *values*. Do **not** copy secrets.

Rules:
- For each unique credential reference, create a `:requires` slot.
- Use `:type` based on node family:
  - HTTP nodes -> `:http-api`
  - DB nodes -> `:database`
  - LLM nodes -> `:llm-provider`
  - Unknown/custom -> `:secret`
- Include `:label` derived from credential name or node name.
- Leave auth type as best-guess (often `:api-key`), but **never** include keys.
- If `:base-url` cannot be derived, set a placeholder and add TODO to fill.
- Do not second‑guess auth placement (header vs query) unless the n8n node explicitly specifies it; copy the n8n config as-is and add a TODO if unclear.

Example:
```clojure
:requires [{:slot :stripe
            :type :http-api
            :label "Stripe API"
            :base-url "https://api.stripe.com"
            :auth {:type :bearer}}]
```

## Unsupported or custom nodes
Always emit a runnable fallback step and add a TODO:
- Default fallback: choose based on intent.
  - If the node performs an API call or has a URL/credentials -> `:http`.
  - Otherwise -> `:function`.
- Add `;; TODO(n8n-import):` comment describing what to implement.
- **Add a web-search note**: “Search the web for <service> API docs to rebuild this node as HTTP.”

Example:
```clojure
;; TODO(n8n-import): Custom node "Acme CRM" not supported.
;; TODO(n8n-import): Search the web for Acme CRM API docs and rebuild as HTTP.
(acme (flow/step :http :acme
                 {:url "https://api.acme.com"  ;; placeholder
                  :method :post
                  :json {}}))
```

## Runnable ASAP defaults
- Always include a `:manual` trigger, even if the n8n flow is only webhook/schedule.
- If a step cannot be translated, emit a placeholder `:function` that returns input.
- Keep the flow deterministic; avoid non-deterministic side effects in placeholder logic.
- Only return **response maps** for webhook-triggered flows; otherwise return normal flow output.
- Do not add “review” commentary about missing steps or improvements; only translate and note TODOs.

## CLI import workflow (Breyta‑style)
Follow the Breyta CLI loop so the imported flow is testable fast:
1) Write the EDN file to `./tmp/flows/<slug>.clj`
2) `breyta flows push --file ./tmp/flows/<slug>.clj` (this creates/updates a **draft**)
3) `breyta flows validate <slug>` and fix errors
4) `breyta flows compile <slug>`
5) `breyta flows draft bindings template <slug> --out draft.edn`
6) Fill required slots (no secrets in repo), then `breyta flows draft bindings apply <slug> @draft.edn`
7) `breyta runs start --flow <slug> --source draft --input '{"test":true}' --wait`

If you build a converter script:
- Prefer a real file (e.g., `scripts/n8n_import.py`) over large inline heredocs.
- Run `python -m py_compile` before executing.
- Avoid f-strings that contain backslashes inside `{...}`; precompute strings or use `.format`.

Notes:
- Draft is the default after `flows push`.  
- `flows deploy` publishes a **version** (e.g., v1), not a draft.

If `breyta` is not on PATH:
- Ask the user for the correct CLI path or to add it to PATH, then re-run the same commands.

Regex conversion guardrails:
- Do not inject literal newlines into regex literals (e.g., `#"\r?\n"` must keep `\\n`).
- Prefer `re-pattern` with a normal string when in doubt: `(re-pattern \"^...$\")`.
- Avoid unsupported escapes (e.g., `\\s`). Use character classes instead: `[\\t\\n\\r ]` or `\\p{Space}` if supported.
- Do not propose using `\\s` as a fix; it remains unsupported. Replace with explicit character classes.
- If a push fails with a regex parse error, fix the pattern and re-run; do not speculate in output.
- When reporting fixes, state only the concrete change (e.g., “replaced \\s with [\\t\\n\\r ]”), not guesses.
- Do not switch algorithms (e.g., replace regex with substring logic) unless the user asks; add a TODO instead.
- Avoid double-escaping braces/backslashes in regex literals; prefer `#\"\\{.*\\}\"` (single escaping) or `re-pattern` with a normal string.
- Do not run blanket regex-escape rewrites across the flow. Fix only the specific pattern that failed and only if you can describe the exact change.
- Do not invent root-cause speculation (e.g., “metadata parsing issues”) without evidence. If parsing fails, report the error and inspect the exact offending literal.
- Do not run unrelated shell experiments (e.g., `clojure -e` probes) unless the user explicitly asks.

## Output expectations
- Output only the translated flow file path and a short TODO list derived from in‑file `TODO(n8n-import)` notes.
- Default behavior: after import, **push the flow as a draft** and validate/compile it unless the user explicitly opts out.
- If the user is **new** or asks for next steps, suggest the **draft** workflow only (push → validate/compile → draft bindings → draft run). Do not suggest deploy/activate unless asked.

## Validation checklist
Use the CLI import workflow above; if you need a quick list, it’s the same steps.

## Minimal example (n8n HTTP + Code)
**n8n**:
- HTTP Request -> GET https://api.example.com/users
- Code -> transform users

**Breyta** (sketch):
```clojure
{:slug :imported
 :name "Imported Flow"
 :concurrency {:type :singleton :on-new-version :supersede}
 :requires [{:slot :api
             :type :http-api
             :label "Imported API"
             :base-url "https://api.example.com"
             :auth {:type :api-key}}]
 :templates [{:id :get-users-request
              :type :http-request
              :request {:path "/users"
                        :method :get}}]
 :functions [{:id :transform-users-fn
              :language :clojure
              :code "(fn [input] input)"}]
 :triggers [{:type :manual :label "Run" :enabled true :config {}}]
 :flow
 '(let [input (flow/input)
        users (flow/step :http :get-users
                         {:connection :api
                          :template :get-users-request})
        ;; TODO(n8n-import): port JS code below to Clojure
        ;; --- begin n8n code ---
        ;; <verbatim n8n code>
        ;; --- end n8n code ---
        result (flow/step :function :transform-users
                          {:ref :transform-users-fn
                           :input {:users users
                                   :input input}})]
    result)}
```
