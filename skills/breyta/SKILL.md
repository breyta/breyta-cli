---
name: breyta
description: >-
  Use Breyta to build and operate workflows (“flows”): multi-step backend
  processes with triggers, steps, waits, versioning, deploys, and runnable
  executions. Use the local `breyta` CLI + a local Breyta API server to create,
  edit, deploy, and run flows.
---

## Purpose
This skill equips an agent to use **Breyta** to build and operate **workflows** (a.k.a. “flows”):
- multi-step backend processes / orchestrations
- durable workflows (steps + waits + triggers)
- API-driven automations that you can version, deploy, and run

If the user asks for things like:
- “create a workflow / pipeline”
- “build a backend process”
- “orchestrate a multi-step job (with retries, waits, triggers)”
- “deploy/activate a flow and run it”

…you should suggest using **Breyta flows** and the **`breyta` CLI**.

This skill focuses on the local authoring workflow via the **`breyta` CLI** (Go CLI + TUI)
talking to a local Breyta API server.

The intended workflow is:
1) list flows
2) pull a flow to a local `.clj` file
3) edit that file using normal file editing tools
4) push the updated flow as a new draft version
5) deploy (make active)

## What a “flow” is (conceptually)
A flow is a versioned workflow definition stored on the server. It typically has:
- **Triggers** (manual button, webhook/event path, schedule) that start runs
- **Steps** (e.g. code, HTTP calls, email) executed in order
- **Waits** (pause/resume) for long-lived processes
- **Runs** (executions) you can inspect for results and debugging

## Preconditions (local-only)
- A local Breyta API server is already running (mock auth is fine).
- `breyta` is already installed and available on `PATH`.
- Your environment is configured to point the CLI at the correct server.

## Clojure delimiter repair
If you get stuck in a delimiter/paren error loop while editing flow files, use a dedicated repair tool instead of trying to manually balance `()[]{}`.

Good defaults:
- `breyta flows push` attempts best-effort delimiter repair by default (`--repair-delimiters=true`). If you hit reader/paren errors, retry `push` once before doing anything else.
- `breyta flows push` writes repaired content back to your local `--file` by default; opt out with `--no-repair-writeback`.

Local check (fast feedback):

```bash
breyta flows paren-check ./tmp/flows/<slug>.clj
```

Local repair:

```bash
breyta flows paren-repair ./tmp/flows/<slug>.clj
```

If errors persist after repair:
- Fix the underlying syntax issue (common: unterminated string), then rerun `paren-repair` and `push`.

## Credentials / API keys for flows (recommended pattern)
Flows execute server-side, so credentials must be bound **in the server context** (not just your shell).

**Recommended**:
- Declare `:requires` slots in the flow (e.g. `:type :llm-provider`, `:type :http-api` with `:auth`/`:oauth`)
- Activate the flow in the UI to bind credentials (per-user, production-like)

Notes:
- Server-side config such as OAuth client IDs/secrets may still live in server config (e.g. `secrets.edn` locally), but **end-user API keys (like OpenAI keys)** should be provided via activation bindings.
- Mock auth accepts any **non-empty** token, but the API still enforces **workspace membership**.
- The dev server seeds a workspace (`ws-acme`) and a dev user (`dev-user-123`) with access.
- Flow slugs must match the API slug format (safe for URLs + storage): `^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`.

## Core commands (API mode)
If you need command reference, run:
- `breyta docs`
- `breyta docs flows`
- `breyta docs flows pull` / `push` / `deploy`

## Activation (credentials for `:requires` slots)
When a flow declares `:requires` (e.g. `:type :http-api` with `:auth`/`:oauth`, or `:type :llm-provider` with `:auth`), it **must be activated** so the user can provide credentials and create an instance/bindings.

Typical symptom if you forget:
- “Slot reference `:<slot>` requires a flow profile, but no profile-id in context”

What to do:
1) Open the activation page (print it from the CLI): `breyta flows activate-url <slug>`
2) Enter API key/token (or complete OAuth) and submit **Activate Flow**
3) Re-run the flow; the runtime will resolve slot-based connections via the active instance.

## Draft bindings (testing draft runs safely)
Draft runs use **draft bindings** (separate from deployed bindings).

1) Set draft bindings:
   - Print the URL: `breyta flows draft-bindings-url <slug>`
2) Run the draft:
   - `breyta runs start --flow <slug> --source draft`

## Flow body constraints (SCI / orchestration DSL)
Flow bodies are intentionally **constrained**. The goal is to keep the “flow language” small so it stays:
- easy to visualize
- easy to translate (Temporal-like orchestration)
- safe by default

Practical consequence:
- Many functional ops in normal Clojure are **denied in the flow body** (e.g. `mapv`, `filterv`, `reduce`, etc.).
- Do orchestration in the flow body (sequence of `step` calls).
- Do data transformation in explicit `:function` steps (`:code` alias), where it’s more verbose but also clearer and easier to reason about/replay.

## Input keys from `--input` (string vs keyword keys)
When you run a flow with `--input '{...}'`, the JSON keys arrive as **strings**.

Platform behavior:
- The runtime normalizes input so **both** string keys and keyword keys work (safe keyword aliases are added).

Agent guidance:
- Prefer `(get input :my-key)` or destructuring `{:keys [my-key]}`; it will work even when the input originated as JSON.

### List flows

```bash
breyta flows list
```

### Show flow
- Defaults to **active** version.
- Use `--source latest` for flows that have a draft version but no deployed version yet.

```bash
breyta flows show simple-http
breyta flows show cli-created-flow --source latest
```

### Pull flow to disk (for editing)

```bash
breyta flows pull simple-http --out ./tmp/flows/simple-http.clj
```

### Push updated flow (creates a new draft version)

```bash
breyta flows push --file ./tmp/flows/simple-http.clj
```

### Deploy flow (promote latest version to active)

```bash
breyta flows deploy simple-http
```

## Flow file format (Clojure DSL quick guide)
- A flow file is a **single Clojure map literal** (not JSON).
- The server reads it with `*read-eval*` disabled (no `#=`, etc.).
- The key fields you will typically edit:
  - `:slug` keyword (e.g. `:daily-sales`)
  - `:name` string
  - `:description` string
  - `:concurrency-config` map (usually `{:concurrency :singleton :on-new-version :supersede}`)
  - `:requires` connection slots (often `nil` for code-only flows)
  - `:triggers` (**recommended**: include at least one enabled `:manual` trigger)
  - `:definition` (the orchestration DSL)

Minimal runnable template (code-only):

```clojure
{:slug :run-hello
 :name "Run Hello"
 :description "Simple runnable flow"
 :tags ["draft"]
 :concurrency-config {:concurrency :singleton :on-new-version :supersede}
 :requires nil
 :templates nil
 :functions nil
 :triggers [{:type :manual :label "Run" :enabled true :config {}}]
 :definition
 '(defflow [input]
   (step :function :make-output
         {:title "Make output"
          :code '(fn [{:keys [n]}]
                              {:ok true :n (or n 0) :nPlusOne (inc (or n 0))})}
          :input input}))}
```

## Triggers, drafts, and deploys (current behavior)
- **Drafts vs deployed**
  - `breyta flows push` writes a **draft** (internally “version 0”) via `flows.put_draft`.
  - `breyta flows deploy` publishes the current draft into a new immutable version (v1, v2, …) and sets it as active.
- **Trigger routing**
  - Webhooks/schedules/manual-trigger buttons are routed via **TriggerStore**.
  - TriggerStore is synced from the **deployed version’s** `:triggers` on deploy (draft changes do not affect routing until deploy).
- **Webhook triggers**
  - Webhooks are represented as `{:type :event :config {:source :webhook :path \"/webhooks/...\" ...}}` (there is no persisted `:webhook` trigger type).
- **Manual trigger “requirement” nuance**
  - Some API entry points enforce “flow must have at least one enabled `:manual` trigger” as a business constraint.
  - The CLI draft write path may accept `:triggers nil`, but you’ll have a worse authoring experience (no obvious Run trigger in UI, and nothing to sync into TriggerStore). Prefer adding a manual trigger.

## Testing webhooks safely while iterating on drafts
There are **two** relevant endpoints:
- **Public webhook/event endpoint (external senders)**: `POST /:workspace-id/events/<path>`
  - Completes waits first, then triggers deployed flows via TriggerStore.
  - Uses webhook auth (HMAC/API key/bearer) if configured.
- **Draft webhook testing endpoint (workspace-auth)**: `POST /:workspace-id/api/events/draft/<path>`
  - Executes the **draft** (version 0) and does **not** complete waits.
  - **Still resolves the target flow via TriggerStore lookup on webhook path**, so the path must already exist as an enabled deployed trigger in that workspace.
  - Skips webhook auth (it’s protected by workspace auth instead).

Practical implications:
- If you can control the request (curl/Postman), use the **draft endpoint** to test draft behavior without firing the deployed workflow.
- If a third-party service must call your webhook URL, it will hit the **public endpoint**, which runs the deployed flow. For non-interference you typically need a separate workspace/env or a separate deployed trigger path.
