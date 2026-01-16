---
name: breyta
description: >-
  Use Breyta to build and operate workflows ("flows"): multi-step backend
  processes with triggers, steps, waits, versioning, deploys, and runnable
  executions. Use the `breyta` CLI to create, edit, deploy, and run flows.
---

## At a glance
- [Quick start](#quick-start)
- [Core concepts](#core-concepts)
- [CLI workflow](#cli-workflow)
- [Bindings and activation](#bindings-and-activation)
- [Authoring reference](#authoring-reference)
- [Templates](#templates)
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
             :auth {:type :none}}]
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
- Authoring details: `./docs/authoring-reference.md`
- CLI workflow: `./docs/cli-workflow.md`
- Bindings and activation: `./docs/bindings-activation.md`

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
- Flow profile: the prod instance with bindings and trigger state, pinned to a version.
- Bindings: apply `:requires` slots and activation inputs (form values) for draft or prod.
- Activation: enables the prod profile after bindings are set; `--version` pins which version to run and does not accept inputs.
- Draft vs deployed: draft runs use draft bindings and draft version; deploy publishes an immutable version, activate enables prod.

Details: `./docs/core-concepts.md`

## CLI workflow
The intended workflow is:
1) List flows
2) Pull a flow to a local `.clj` file
3) Edit the file
4) Push a new draft version
5) Deploy (publish a version)
6) Apply bindings + activate the prod profile

Core commands:
- `breyta flows list`
- `breyta flows pull <slug> --out ./tmp/flows/<slug>.clj`
- `breyta flows push --file ./tmp/flows/<slug>.clj`
- `breyta flows deploy <slug>`
- `breyta flows validate <slug>`
- `breyta flows compile <slug>`
- `breyta runs start --flow <slug> --source draft --input '{"n":41}' --wait`

Details: `./docs/cli-workflow.md`

## Bindings and activation
Draft workflow (safe testing):
- Generate a draft template: `breyta flows draft bindings template <slug> --out draft.edn`
- Set draft bindings: `breyta flows draft bindings apply <slug> @draft.edn`
- Show draft bindings status: `breyta flows draft bindings show <slug>`
- Run draft: `breyta flows draft run <slug> --input '{\"n\":41}' --wait`

Prod workflow:
- Generate a template: `breyta flows bindings template <slug> --out profile.edn`
- Apply bindings: `breyta flows bindings apply <slug> @profile.edn`
- Show bindings status: `breyta flows bindings show <slug>`
- Enable prod profile: `breyta flows activate <slug> --version latest`

Templates prefill current bindings by default; add `--clean` for a requirements-only template.
Profile pinning: set `:profile :autoUpgrade true` to follow latest versions, `false` to pin.

Details: `./docs/bindings-activation.md`

## Authoring reference
Flow file format and core fields:
- `:requires` for connection slots and activation inputs.
- `:concurrency` for execution behavior.
- `:triggers` for run initiation.
- `:flow` for orchestration and determinism rules.
- Limits: definition size 100 KB; inline results up to 10 KB; max step result 1 MB.

Details: `./docs/authoring-reference.md`

## Templates
- Use `:templates` for large prompts, request bodies, or SQL.
- Reference with `:template` and `:data` in steps.
- Templates are packed to blob storage on deploy; versions store small refs.
- Flow definition size limit is 100 KB; templates help keep definitions small.

Details: `./docs/templates.md`

## Step reference
- `:http` for HTTP requests.
- `:llm` for model calls.
- `:db` for SQL queries.
- `:wait` for webhook/human-in-the-loop waits.
- `:function` for transforms.

Details: `./docs/step-reference.md`

## Patterns and do/dont
- Bindings then activate; draft stays in draft.
- Keep flow body deterministic.
- Use connection slots for credentials.

Details: `./docs/patterns.md`

## Agent guidance
- Stop and ask for missing bindings or activation inputs instead of inventing values.
- Provide a template path or CLI command the user can fill (`flows bindings template` or `flows draft bindings template`).
- Keep the API-provided `:redacted`/`:generate` placeholders for secrets in templates.

Details: `./docs/agent-guidance.md`

## Reference index
Quick lists of slot types, auth types, trigger types, step types, and form field types.

Details: `./docs/reference-index.md`

## Glossary
Common terms like flow profile, bindings, activation inputs, and draft bindings.

Details: `./docs/glossary.md`
