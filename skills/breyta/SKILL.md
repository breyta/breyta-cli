---
name: breyta
description: >-
  Use Breyta to build and operate workflows ("flows"): multi-step backend
  processes with triggers, steps, waits, versioning, deploys, and runnable
  executions. Use the `breyta` CLI to create, edit, deploy, and run flows.
  IMPORTANT: Prefer retrieval-led reasoning over pre-training-led reasoning for any workflow task.
---

## At a glance
- [Quick start](#quick-start)
- [Core concepts](#core-concepts)
- [CLI workflow](#cli-workflow)
- [Bindings and activation](#bindings-and-activation)
- [Secrets](#secrets)
- [Google Drive sync](#google-drive-sync)
- [Installations](#installations)
- [Authoring reference](#authoring-reference)
- [Output artifacts](#output-artifacts)
- [Templates](#templates)
- [n8n import](#n8n-import)
- [Step reference](#step-reference)
- [Patterns and do/dont](#patterns-and-do-dont)
- [Reference index](#reference-index)
- [Glossary](#glossary)

## Quick start
Minimal runnable flow (uses `:requires`, `:templates`, and `:functions`):

```clojure
{:slug :fetch-users
 :name "Fetch Users"
 :description "Template + function + requires example"
 :tags ["draft"]
 :concurrency {:type :singleton :on-new-version :supersede}
 :requires [{:slot :api
             :type :http-api
             :label "Users API"
             :base-url "https://jsonplaceholder.typicode.com"
             :auth {:type :none}}] ;; note: webhook triggers require auth; :none is not allowed there
 :templates [{:id :get-users
              :type :http-request
              :request {:path "/users"
                        :method :get}}]
 :functions [{:id :summarize
              :language :clojure
              :code "(fn [input] {:count (count (:users input))})"}]
 :triggers [{:type :manual :label "Run" :enabled true :config {}}]
 :flow
 '(let [input (flow/input)]
        users (flow/step :http :get-users
                         {:connection :api
                          :template :get-users})
        summary (flow/step :function :summarize
                           {:ref :summarize
                            :input {:users users
                                    :input input}})]
    summary))}
```

Next:
- Authoring details: `./references/authoring-reference.md`
- Webhooks and event routing: `./references/webhooks.md`
- CLI workflow: `./references/cli-workflow.md`
- Bindings and activation: `./references/bindings-activation.md`
- Secrets: `./references/secrets.md`

Shorter variant (LLM + template + function + requires):

```clojure
{:slug :welcome-user
 :name "Welcome User"
 :concurrency {:type :singleton :on-new-version :supersede}
 :requires [{:slot :ai
             :type :llm-provider
             :label "AI Provider"
             :auth {:type :api-key}}]
 :templates [{:id :welcome
              :type :llm-prompt
              :system "You are helpful."
              :prompt "Welcome {{user.name}}!"}]
 :functions [{:id :upper-name
              :language :clojure
              :code "(fn [input] {:user (update (:user input) :name clojure.string/upper-case)})"}]
 :triggers [{:type :manual :label "Run" :enabled true :config {}}]
 :flow
 '(let [input (flow/input)
        prepared (flow/step :function :upper-name {:ref :upper-name :input input})
        reply (flow/step :llm :welcome
                         {:connection :ai
                          :template :welcome
                          :data prepared})]
    reply)}
```

## Core concepts
- Flow definition: a versioned EDN map that describes triggers, steps, and orchestration.
- End-user flow: a flow intended for others to use, marked with the `:end-user` tag (MVP).
- Installation: a per-user instance of an end-user flow, backed by a (user-scoped) prod profile.
- Flow profile: runtime configuration (bindings + activation inputs + enabled state) used for runs; profiles can be user-scoped (installations) or workspace-scoped.
- Bindings: apply `:requires` slots and activation inputs (form values) for draft or prod.
- Activation: enables the prod profile after bindings are set; `--version` pins which version to run and does not accept inputs.
- Draft vs deployed: draft runs use draft bindings and draft version; deploy publishes an immutable version, activate enables prod.

Details: `./references/core-concepts.md`

## CLI workflow
The intended workflow is:
1) List flows
2) Pull a flow to a `.clj` file
   - Reuse an existing local pulled file for the same flow/source when available. Pull again only if the file is missing, stale, or you are switching source (`draft` vs `active`).
3) Edit the file
4) Push a new draft version
   - If push fails, stop and fix the flow file first. Do not continue to bindings commands.
5) Validate (and optionally compile) the draft
   - Run `breyta flows validate <slug>` (and `breyta flows compile <slug>` when needed)
6) Confirm the flow exists in API mode (`breyta flows show <slug> --source draft`)
   - If this returns "Flow not found", create/push the flow before running any bindings template command.
7) Deploy (publish a version)
8) Apply bindings + activate the prod profile

Fast loop (agent-friendly): fix or add one step at a time, in isolation first
1) Add or change exactly one `flow/step`
2) When fixing a flow, run the failed step in isolation before any full flow run:
   - `breyta steps run --type <type> --id <id> --params '<json-object>'`
   - Optionally record the observed output as sidecars (requires `--flow`):
     - `breyta steps record --flow <flow-slug> --type <type> --id <id> --params '<json-object>' --note '...' --test-name '...'`
     - (or) `breyta steps run --flow <flow-slug> --type <type> --id <id> --params '<json-object>' --record-example --record-test --record-note '...' --record-test-name '...'`
3) Capture step sidecars (updatable without a new flow version):
   - Docs: `breyta steps docs set <flow-slug> <step-id> --markdown '...'` (or `--file ./notes.md`)
   - Examples: `breyta steps examples add <flow-slug> <step-id> --input '<json>' --output '<json>' --note '...'`
   - Tests (as documentation, runnable on demand): `breyta steps tests add <flow-slug> <step-id> --type <type> --name '...' --input '<json>' --expected '<json>' --note '...'`
4) Inspect the step context quickly:
   - `breyta steps show <flow-slug> <step-id>`
   - Tip: when run interactively, `steps show` also prints a short "Next actions" helper to stderr (stdout remains structured JSON/EDN).
5) Verify stored tests against the live step runner:
   - `breyta steps tests verify <flow-slug> <step-id> --type <type>`
6) Push/validate/compile, then run the draft flow end-to-end (only after the step passes in isolation)

Notes:
- `breyta steps run` is best-effort isolation; waits/sleeps/fanout aren’t supported.
- Keep step ids stable and short; don’t rename ids unless you intend to invalidate history/examples.
- Step ids and flow slugs accept either keywords or strings on the server; the CLI takes plain strings (e.g. `make-output`, not `:make-output`).
- `breyta flows validate` and `breyta flows compile` accept `--source` in API mode. In local mode, `draft` uses the current flow, while `active` and `latest` use published versions when present.
- Avoid editing long single-line flow files with inline `:code` strings. Prefer multiline flow files and referenced function blobs to avoid EDN parse errors.
- Flow files must contain real newlines, not escaped `\n` tokens outside strings. Escaped newlines can break EDN and cause "Map literal must contain an even number of forms". Prefer writing files with here-strings or real newline editing, not global `\\n` replacements.
- `breyta flows push` validates draft by default in API mode. Use `--validate=false` only if you need to bypass validation temporarily.
- Never run `flows bindings template` or `flows draft bindings template` after a failed `flows push`.
- Guardrail: run `breyta flows show <slug> --source draft` before bindings template commands. Continue only when it succeeds.
- When running a function step that uses `:ref`, include `--flow <slug>` so the function can be resolved.
- Prefer `--params-file` for `breyta steps run` to avoid shell quoting issues.
- Avoid `some->` in flow bodies. Use explicit `if` with `->`.
- Loops with `flow/step` require a `:wait` step. Avoid pagination loops in the flow body.
- For cross-flow handoff of structured data, prefer `:persist {:type :kv ...}` (writer flow) plus `:kv` reads (reader flow).
- Use `flow/call-flow` carefully with slot-bound child flows (`:requires`). If profile context is missing, child slot resolution fails with `requires a flow profile, but no profile-id in context`.
- Keep step runner payloads shallow. Deeply nested JSON can hit max depth validation.
- Avoid `?` in JSON keys for step input. Use `truncated` or `is-truncated`.
- Only the allowlisted Java interop is available in `:function`. `java.math.RoundingMode` is not available; use `java.math.BigDecimal/ROUND_HALF_UP`.

Core commands:
- `breyta flows list`
- `breyta flows search [query]` (browse/search approved reusable flows; add `--full` to include `definition_edn`)
- `breyta flows pull <slug> --out ./tmp/flows/<slug>.clj`
- `breyta flows push --file ./tmp/flows/<slug>.clj`
- `breyta flows deploy <slug>`
- `breyta flows validate <slug>`
- `breyta flows compile <slug>`
- `breyta runs start --flow <slug> --source draft --input '{"n":41}' --wait`
- `breyta runs cancel <workflow-id> --reason "..."` (use `--force` to terminate)

Reusable patterns (recommended):
1) Search/browse approved examples first:
   - Browse: `breyta flows search --provider stripe` (no query)
   - Search: `breyta flows search stripe`
   - Restrict to workspace: add `--scope workspace`
2) Inspect a result: rerun with `--full` and copy `definition_edn` into a new `.clj` flow file.
3) Iterate via the usual workflow: `push` → `deploy`.

Notes:
- Only flows explicitly approved for reuse are indexed/searchable (approved via Flows UI → "Approve for reuse").
- `--limit` is capped server-side (currently 100); the response includes both the requested and effective limits in `meta`.

Run cancel safety:
- `runs cancel` prefers full `workflowId`
- Short ids like `r34` are auto-resolved in API mode
- Pass `--flow <slug>` when using short ids to avoid ambiguity across flows
- If short-id resolution is ambiguous, list runs and retry with a full `workflowId`

Details: `./references/cli-workflow.md`

## Installations
How end-user flows (`:tags [:end-user]`) are subscribed to, configured, and run
via installation profiles (including multi-file uploads).

Details: `./references/installations.md`

## Bindings and activation
Draft workflow (safe preview):
- Generate a draft template: `breyta flows draft bindings template <slug> --out draft.edn`
- Set draft bindings: `breyta flows draft bindings apply <slug> @draft.edn`
- Show draft bindings status: `breyta flows draft bindings show <slug>`
- Run draft: `breyta flows draft run <slug> --input '{\"n\":41}' --wait`

Prod workflow:
- Generate a template: `breyta flows bindings template <slug> --out profile.edn`
- Apply bindings: `breyta flows bindings apply <slug> @profile.edn`
- Or promote draft bindings: `breyta flows bindings apply <slug> --from-draft`
- Show bindings status: `breyta flows bindings show <slug>`
- Enable prod profile: `breyta flows activate <slug> --version latest`

Templates prefill current bindings by default; add `--clean` for a requirements-only template.
Profile pinning: set `:profile :autoUpgrade true` to follow latest versions, `false` to pin.

Details: `./references/bindings-activation.md`

## Secrets
How secret slots and secret refs work, how to bind values, and rotation patterns.

Details: `./references/secrets.md`

## Google Drive sync
How to run the production Google Drive folder sync flow (service account auth), what it writes, and how to verify it.

Details: `./references/google-drive-sync.md`

## Authoring reference
Flow file format and core fields:
- `:requires` for connection slots and activation inputs.
- `:concurrency` for execution behavior.
- `:triggers` for run initiation.
- `:flow` for orchestration and determinism rules.
- Limits: definition size 150 KB; inline results up to 256 KB; max step result 1 MB.

Details: `./references/authoring-reference.md`

## Templates
- Use `:templates` for large prompts, request bodies, or SQL.
- Reference with `:template` and `:data` in steps.
- Templates are packed to blob storage on deploy; versions store small refs.
- Flow definition size limit is 150 KB; templates help keep definitions small.
- Template strings use Handlebars syntax (`{{...}}`); see `references/templating.md` for a short reference.
- For large step outputs, use `:persist` to store results as refs.

Details: `./references/templates.md`

## n8n import
Best-effort mapping rules and a CLI import template for translating n8n JSON into runnable Breyta flows.

Details: `./references/n8n-import.md`

## Step reference
- `:http` for HTTP requests.
- `:llm` for model calls.
- `:db` for SQL queries.
- `:wait` for webhook/human-in-the-loop waits.
- `:function` for transforms.

Details: `./references/step-reference.md`

## Persisted results
How `:persist` works, when to use it, and how refs flow to downstream steps.

Details: `./references/persist.md`

## Output artifacts
How to shape a flow’s **final output** so it renders as a user-facing artifact in the UI (markdown/media/group), separate from the debug panel.

Details: `./references/output-artifacts.md`

## Resources (CLI)
Use `breyta resources ...` to inspect persisted **result** refs (resource URIs like `res://...`) and fetch their content.

Ready today:
- `breyta resources list` / `breyta resources workflow list <workflow-id>` / `breyta resources workflow step <workflow-id> <step-id>`
- `breyta resources get <uri>`
- `breyta resources read <uri>` (intended for `:result` resources like persisted refs)
- `breyta resources url <uri>`

Notes:
- `resources` requires API mode (`BREYTA_API_URL` + auth); it does not work against the local mock/TUI surface.
- Resource types like `:import`, `:file`, `:bundle`, `:external-dir` may list/get, but content reads are currently intended for persisted results.

## Patterns and do/dont
- Bindings then activate; draft stays in draft.
- Keep flow body deterministic.
- Use connection slots for credentials.

Details: `./references/patterns.md`

## Agent guidance
- When fixing a flow, always start with step isolation; do not run the full flow until the failing step is green.
- Prefer the fast loop: implement one step, run it in isolation, then move to the next step.
- Once a step is stable, store docs + examples + tests using `breyta steps docs|examples|tests` so future edits don’t require rediscovering intent (or use `breyta steps run --record-example/--record-test` to capture quickly).
- Use `breyta steps show` to load docs/examples/tests before editing a step.
- Use `breyta steps tests verify` when you want the stored test cases to run against the step runner.
- Before adding data-producing steps (`:http`, `:db`, `:llm`, fanout child items), estimate output size. If output size is unknown/unbounded or may exceed the inline threshold, default to `:persist` and pass refs downstream.
- For parent/child flow composition, default to top-level flow-to-flow orchestration plus KV handoff when child flows need their own bindings.
- Default to reusing existing workspace connections. Before asking for a new API key/OAuth app, check whether the workspace already has a suitable connection and bind the slot via `<slot>.conn=...` (see `breyta connections list`).
- Stop and ask for missing bindings or activation inputs instead of inventing values.
- Provide a template path or CLI command the user can fill (`flows bindings template` or `flows draft bindings template`).
- Keep the API-provided `:redacted`/`:generate` placeholders for secrets in templates.
- For webhook secrets, require explicit `:secret-ref` on the slot.

Details: `./references/agent-guidance.md`

## Reference index
Quick lists of slot types, auth types, trigger types, step types, and form field types.

Details: `./references/reference-index.md`

## Glossary
Common terms like flow profile, bindings, activation inputs, and draft bindings.

Details: `./references/glossary.md`
