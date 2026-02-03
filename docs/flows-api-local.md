## Local flows authoring (flows-api)

### Start flows-api (mock auth, emulator)

From `breyta/`:

```bash
./scripts/start-flows-api.sh --emulator --auth-mock
```

Notes:
- Default URL: `http://localhost:8090`
- Ctrl+C stops the server and frees the port.

### Configure breyta CLI (API mode)

In the shell/environment the agent runs in:

```bash
export BREYTA_API_URL="http://localhost:8090"
export BREYTA_WORKSPACE="ws-acme"
export BREYTA_TOKEN="dev-user-123"
```

### Workspace bootstrap (fix “not a workspace member”)

If you see `403 Access denied: not a workspace member` (often after restarting dev servers), bootstrap the workspace and membership via the dev debug endpoint:

```bash
breyta workspaces bootstrap ws-acme
```

### Bindings (bind credentials for `:requires` slots)

If your flow uses `:requires` slots (including `:type :llm-provider` for LLM keys), users must apply bindings once to create a profile, then enable it. Slot names must be non-namespaced keywords (e.g., `:api`, not `:ns/api`).
`(:kind :form ...)` entries in `:requires` add activation-only inputs and are exposed under `:activation` in the run input map.

**UI field names:** manual trigger `:fields` and wait notify `:fields` use non-namespaced keyword names (e.g., `{:name :user-id ...}`).
**Wait notifications:** `:notify` supports HTTP channels, so email is sent via an email API connection (for example SendGrid). Approval links are available as `{approvalUrl}` and `{rejectionUrl}` template aliases.

- Sign in: `http://localhost:8090/login` → “Sign in with Google” → “Dev User”
- Activation URL: `http://localhost:8090/<workspace>/flows/<slug>/activate`
- Generate a template: `breyta flows bindings template <slug> --out profile.edn` (prefills current `:conn` bindings; use `--clean` for a blank template)
- Apply bindings via CLI: `breyta flows bindings apply <slug> @profile.edn`
- Enable prod profile: `breyta flows activate <slug> --version latest`

### Draft preview bindings

Draft runs use user-scoped draft bindings:

- Draft bindings URL: `http://localhost:8090/<workspace>/flows/<slug>/draft-bindings`
- Or print it: `breyta flows draft-bindings-url <slug>`
- Run draft: `breyta runs start --flow <slug> --source draft`

### Flow body constraints (SCI / orchestration DSL)

Flow definitions run in a constrained runtime intended for **orchestration**, not transformation:
- Many functional ops are denied in the flow body (e.g. `mapv`, `filterv`, `reduce`, etc.)
- Keep orchestration in the flow body (sequence of `step` calls)
- Do data transformation in `:function` steps (`:code` alias).
  - Nil-short-circuiting macros like `some->` are blocked in flow bodies. Use explicit `if` with `->` instead.
  - Loops that include `flow/step` must include a `:wait` step. Avoid pagination loops in the flow body. Prefer higher `limit` values or split pagination across runs.
  - For `breyta steps run`, keep JSON payloads shallow to avoid max depth validation errors. Use `--params-file` for complex inputs.
  - Safe helpers are exposed under `breyta.sandbox` (preferred; pure/deterministic):
    - `base64-encode` `(string|bytes) -> string`
    - `base64-decode` `(string|bytes) -> string`
    - `base64-decode-bytes` `(string|bytes) -> bytes`
    - `hex-encode` `(string|bytes) -> string`
    - `hex-decode` `(string) -> string`
    - `hex-decode-bytes` `(string) -> bytes`
    - `sha256-hex` `(string|bytes) -> string`
    - `hmac-sha256-hex` `(key string|bytes, value string|bytes) -> string`
    - `uuid-from` `(string) -> uuid`
    - `uuid-from-bytes` `(string|bytes) -> uuid`
    - `parse-instant` `(string) -> java.time.Instant`
    - `format-instant` `(Instant) -> string`
    - `format-instant-pattern` `(Instant, pattern) -> string`
    - `instant->epoch-ms` `(Instant) -> long`
    - `epoch-ms->instant` `(long) -> Instant`
    - `duration-between` `(Instant, Instant) -> Duration`
    - `truncate-instant` `(Instant, unit) -> Instant` (unit: `:seconds|:minutes|:hours|:days`)
    - `instant-plus` `(Instant, amount, unit) -> Instant` (unit: `:millis|:seconds|:minutes|:hours|:days`)
    - `instant-minus` `(Instant, amount, unit) -> Instant`
    - `url-encode` `(string) -> string`
    - `url-decode` `(string) -> string`
  - Limited Java interop is also allowed in `:function` code (small allowlist):
    - `java.time.*`, `java.time.format.DateTimeFormatter`, `java.time.temporal.{ChronoUnit,TemporalAdjusters}`
    - `java.util.{UUID,Base64}` (and Base64 encoder/decoder)
    - `java.math.{BigInteger,BigDecimal}`
    - Prefer `breyta.sandbox` helpers when possible.
    - `java.math.RoundingMode` is not available in the sandbox. Use `java.math.BigDecimal/ROUND_HALF_UP` if you need rounding.

### Concurrency notes

- Concurrency config is static. Do not use expressions in `:concurrency`.
- `:key-field` must be a keyword or nested path vector that exists in `flow/input` (for example `:email` or `[:event :id]`).
- Use `:supersede` when a newer run should cancel the older one (webhooks, retries, refresh jobs).
- Use `:drain` when in-flight work must finish and it is safe to queue new runs (billing, uploads, sequential processing).

### Common pitfalls

- Waits are event-based. They pause for external signals (webhooks, CLI commands), not timers. For delays, use schedule triggers.
- Singleton workflows can get stuck if a run errors or waits. Use `:on-new-version :supersede` for fresh starts.
- Keep flow bodies simple. Put logic in `:function` steps and keep orchestration minimal.
- `breyta steps run` resolves function `:ref` only when `--flow <slug>` is provided.
- Avoid `?` in JSON keys when using `breyta steps run`. Use `truncated` or `is-truncated` instead of `truncated?`.

### Input keys from `--input` (string vs keyword keys)

The CLI sends `--input` as JSON, so keys arrive as strings.

The runtime normalizes input so both string keys and keyword keys work (safe keyword aliases are added), but author flows as if you will read keyword keys (e.g. `(get input :n)`).

### Flow edit loop (pull → edit → push draft → deploy)

```bash
breyta flows list
breyta flows pull simple-http --out ./tmp/flows/simple-http.clj
# edit ./tmp/flows/simple-http.clj
breyta flows push --file ./tmp/flows/simple-http.clj
breyta flows deploy simple-http
breyta flows show simple-http
```

### Runnable smoke test (verify final output)

Create a tiny code-only flow file and run it:

```bash
mkdir -p ./tmp/flows
cat > ./tmp/flows/run-hello.clj <<'EOF'
{:slug :run-hello
 :name "Run Hello"
 :description "Simple runnable flow (returns deterministic result)"
 :tags ["draft"]
 :concurrency {:type :singleton
                      :on-new-version :supersede}
 :requires nil
 :templates nil
 :functions nil
 :triggers nil
 :flow
 '(let [input (flow/input)
        out (flow/step :function :make-output
                       {:title "Make output"
                        :code (quote (fn [{:keys [n]}]
                                       {:ok true
                                        :message "hello"
                                        :n (or n 0)
                                        :nPlusOne (inc (or n 0))}))}
                        :input input})]
    out)}
EOF

breyta flows push --file ./tmp/flows/run-hello.clj
breyta flows deploy run-hello

# Start a run and wait. Output is in:
#   data.run.resultPreview.data.result
breyta runs start --flow run-hello --input '{"n":41}' --wait --timeout 30s

### Resources (preferred unified interface)

Resources are the preferred unified surface for results, imports, and files.

```bash
breyta resources workflow list flow-run-hello-ws-acme
breyta resources get res://v1/ws/ws-acme/result/run/flow-run-hello-ws-acme/step/make-output/output
breyta resources read res://v1/ws/ws-acme/result/run/flow-run-hello-ws-acme/step/make-output/output
```
