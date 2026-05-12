# Breyta CLI (`breyta`)

`breyta` is the command-line interface for working with Breyta flows from coding
agents and terminals.

This repository is public so you can inspect what runs on your machine, build
the CLI yourself, and understand how it talks to the Breyta API. The code is
available under the MIT License. In practice, this repo is the client for
Breyta, so most usage happens inside a Breyta workspace rather than as a
standalone general-purpose tool.

## Install

Choose one:

- Homebrew (macOS):
  - `brew tap breyta/tap`
  - `brew install breyta`
- Prebuilt binaries:
  - https://github.com/breyta/breyta-cli/releases
- Go install:
  - `go install github.com/breyta/breyta-cli/cmd/breyta@latest`
- From source checkout:
  - `go install ./cmd/breyta`

Verify the install:

```bash
breyta version
```

## Getting started

The usual path is:

1. Install the CLI
2. Bootstrap a local agent workspace
3. Authenticate
4. Verify your account and workspace summary
5. Search nearby workspace flow patterns, docs, and approved templates before designing
6. Pick a workspace later when you are ready to adopt or build

```bash
breyta init --provider <codex|cursor|claude|gemini>
cd breyta-workspace
breyta auth login
breyta auth whoami
breyta flows search "<integration or problem query>" --limit 5
breyta flows grep "<literal config or tool name>" --or "<variant>" --limit 5
breyta flows workspace examples step <type> "<integration or problem query>" --limit 3
breyta docs find "<idea or primitive>"
breyta flows templates search "<problem or integration query>" --limit 5
breyta flows templates grep "<literal config or tool name>" --limit 5
breyta flows examples step <type> "<problem or integration query>" --limit 3
```

For new flows, search nearby workspace flows first instead of listing every
flow, use `breyta flows grep` when you need source/config literals, then search
docs snippets and approved templates. For edits, inspect the
current flow with `breyta flows show <slug>` or `breyta flows pull <slug>`
before docs/example search. Keep reuse primitive-first: use
`breyta flows workspace examples step <type> "<query>"` for local private
snippets, then `breyta flows examples step <type> "<query>"` for approved
snippets. Use `breyta flows templates search/grep` for approved reusable
templates. Inspect a full template only for architecture-level reuse, public
install patterns, multi-flow orchestration, fanout/child-flow behavior, unclear
snippet dependencies, or copying overall flow structure. `breyta flows list` is
for inventory, slug checks, or explicit user requests, not pattern discovery.

When a flow touches external APIs or LLM models, check current official provider
docs/API references or model-list endpoints before choosing request shapes,
auth assumptions, limits, or model ids.

For public or end-user flows, do not call the flow "ready for UI" from draft CLI
proof alone. Verify live/install-shaped behavior, or say `web UI not verified`
in the risk ledger. When browser/UI access is available, test the actual setup
page, run form fields, upload CSV/file path, resource picker, and output page.
For installable/public flows, do not stop at activation: `/activate` and
configure/check prove owner setup, not the Discover install surface. Verify the
Discover install dialog plus an installed run when install behavior matters.

To browse public installable end-user flows for your current workspace instead,
use:

```bash
breyta flows discover list
breyta flows discover search "<idea>"
```

Discover list/search excludes flows owned by the current workspace by default
because it shows what this workspace can install. Add `--include-own` only when
debugging whether your own public flow is indexed; verify buyer/install behavior
from another workspace.

`breyta init` installs the Breyta skill bundle for your agent tool and creates a
local `breyta-workspace/` directory with an `AGENTS.md` file and flow folders.
The skill bundle can include `SKILL.md` plus bundled `references/` files; agents
should read `SKILL.md` first and load the referenced file for the task surface
they are editing.

Compact diagnostics for agent loops:

```bash
breyta flows doctor <slug> --target draft
breyta flows public preflight <slug>
breyta runs show <workflow-id> --errors
breyta resources table verify <res://table-uri>
```

`flows doctor` folds in `flows configure check` readiness and only suggests run
commands when required bindings and activation inputs are ready.

If `breyta auth whoami` shows multiple workspaces or no default workspace
selected, use:

```bash
breyta workspaces list
breyta workspaces use <workspace-id>
```

If you only want the skill bundle and not the workspace files:

```bash
breyta skills install --provider <codex|cursor|claude|gemini>
```

## First Workflow

Pull a flow into a local workspace, edit it, push it back to draft, and run it:

```bash
breyta flows pull <slug> --out ./flows/<slug>.clj
breyta flows push --file ./flows/<slug>.clj
breyta flows configure check <slug>
breyta flows run <slug> --wait
```

When the draft behavior is correct, inspect draft-vs-live changes and release once with a markdown note:

```bash
breyta flows diff <slug>
breyta flows release <slug> --release-note-file ./release-note.md
breyta flows show <slug> --target live
breyta flows run <slug> --target live --wait
```

To smoke test a specific installed public/end-user flow, use the installation id
instead of `--target`:

```bash
breyta flows run <slug> --installation-id <installation-id> --wait
```

Authoring commands return compact JSON by default. Use `--full` on `flows show`,
`flows diff`, and `runs show` only when you need full source, unified diff text,
step arrays, or result payloads. `resources read` defaults to a bounded table
row and cell preview; pass `--full` only when the full resource payload is
required. `flows show` includes a non-editable `flowLiteralPreview` that keeps
source structure while omitting heavy leaves; use `flows pull` for editable
source. `--pretty` changes formatting only; it does not request full payloads.
For large reports and research artifacts, store full bodies as resources and
move refs, URLs, short summaries, and previews through tables or run output.

If the flow should appear in public discover/install surfaces, make that explicit:

```bash
# Tag it as end-user installable
breyta flows update <slug> --tags 'end-user,...'

# Either author this in the flow file:
# :discover {:public true}
#
# Or set it explicitly after push/release:
breyta flows discover update <slug> --public=true
```

Public discover visibility is stored flow metadata. A released version and the
`end-user` tag are both required before the flow can be exposed in discover.
This discover catalog is separate from `breyta flows search`, which searches
actual workspace flow metadata, and from `breyta flows templates search`, which
searches approved reusable templates to inspect and copy from.

Installable-flow smoke path:

```bash
breyta flows show <slug> --target live
breyta flows installations create <slug> --name "Smoke install"
breyta flows installations configure <installation-id> --input '{"<field>":"<value>"}'
breyta flows installations enable <installation-id>
breyta flows run <slug> --installation-id <installation-id> --wait
```

When browser access is available, also open the Discover install dialog and
confirm setup fields, upload/resource fields, and output render as expected.

OpenAI-backed `:llm` and `:agent` flows should use an `:http-api` requirement
with backend `openai`, base URL `https://api.openai.com/v1`, API-key auth, and a
non-null config map. Use installer ownership when every installer brings their
own OpenAI key.

If the flow should look polished on public cards, add curated discover card media:

```bash
breyta flows update <slug> \
  --publish-media-type image \
  --publish-media-source-kind https-url \
  --publish-media-source https://cdn.example.com/hero.png \
  --publish-media-alt "Preview of the generated result"

# Clear it later
breyta flows update <slug> --clear-publish-media
```

You can also keep this in source as `:publish-media` inside the flow file and
push it with `breyta flows push`.

## Connection item caches

Some installable flows use connection-backed dropdowns for provider-owned
objects such as repositories, channels, projects, folders, or accounts. Inspect
the cached non-secret items behind a connection with:

```bash
breyta connections items <connection-id>
breyta connections items <connection-id> --item-type github/repository --limit 25
breyta connections items <connection-id> --item-type github/repository --raw
```

By default, the CLI prints summarized rows and omits raw provider payloads. Use
`--raw` only when debugging item metadata. Use `--limit 0` with an `--item-type`
to fetch all pages.

When a setup or run input is backed by connection items, Breyta validates the
submitted value against this cache. Missing bindings, empty caches, disabled
items, and unknown values fail closed. Installation setup errors include
`setupState` details so CLI callers can show the missing field, binding, or
invalid option to the operator.

## Table resources

The CLI also exposes the bounded table-resource surface used by flows and the UI:

```bash
breyta resources read <res://table-uri> --limit 25 --offset 0 --partition-key month-2026-03
breyta resources table query <res://table-uri> --page-mode offset --limit 25 --offset 0 --partition-keys month-2026-03,month-2026-04
breyta resources table schema <res://table-uri> --partition-key month-2026-03
breyta resources table set-column <res://table-uri> --column customer-name --computed-json '{"type":"lookup","reference-column":"customer-id","field":"name"}' --partition-keys month-2026-03,month-2026-04
breyta resources table set-column <res://table-uri> --column status --enum-json '{"options":[{"id":"open","name":"Open","aliases":["OPEN","Open"]},{"id":"in-progress","name":"In progress","aliases":["IN_PROGRESS","In Progress"]}]}' --partition-keys month-2026-03,month-2026-04
breyta resources table recompute <res://table-uri> --limit 1000 --partition-key month-2026-03
```

Partitioned table families use explicit flags:

- `--partition-key` for one partition
- `--partition-keys` for a bounded comma-separated subset

Single-row operations such as `get-row`, `update-cell`, and `update-cell-format`
should use exactly one partition when the table family is partitioned.

Archive or remove flows intentionally:

```bash
# Hide a flow from normal active use while preserving versions and metadata
breyta flows archive <slug>

# Permanently remove a flow definition
breyta flows delete <slug> --yes

# If the flow still has runs/installations to clean up, force the delete
breyta flows delete <slug> --yes --force

# For large cleanup jobs with many runs or resources, raise the request timeout
breyta flows delete <slug> --yes --force --timeout 5m
```

Inspect runs with the same structured filter syntax as the web runs list:

```bash
breyta runs list --query 'status:failed flow:<slug>'
breyta runs list --installation-id <installation-id> --version 7
```

If you need to revise the note later:

```bash
breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md
```

## Jobs workers

Use the jobs surface when Breyta should orchestrate external work on any
machine.

Recommended shape:

- flow creates one job or a small explicit batch
- `breyta jobs worker run` claims and executes that work externally
- flow awaits the normalized job or batch result
- when installability depends on those workers, declare that in the flow
  definition with `{:kind :worker ...}` inside `:requires`

Create or inspect jobs directly:

```bash
breyta jobs create --type codex-review --payload '{"surface":"flows-api"}'
breyta jobs list --type codex-review --limit 20
breyta jobs show job-123
breyta jobs batches create --type codex-review --job '{"payload":{"surface":"flows-api"}}' --job '{"payload":{"surface":"runtime"}}'
breyta jobs batches show batch-123
breyta jobs claim --type codex-review --worker-id worker-1 --lease-duration 2m
```

Run a polling worker loop for one job type:

```bash
export BREYTA_API_URL="https://flows.breyta.ai"
export BREYTA_WORKSPACE="ws-acme"
export BREYTA_API_KEY="<service-account-api-key>"

breyta jobs worker run --type codex-review --handler ./scripts/run-review.sh --keep-job-dirs
```

Create that worker identity from an interactive operator session:

```bash
breyta service-accounts create \
  --name codex-review-worker \
  --scope jobs.worker \
  --job-type codex-review

breyta service-accounts keys create <service-account-id> --name ci-runner
```

`--scope` accepts repeated flags or comma-separated values. `--capability`
remains accepted as a compatibility alias.

For a broader unattended agent, add explicit API scopes or use the broad
catch-all for the known service-account scope matrix:

```bash
breyta service-accounts create \
  --name automation-agent \
  --scope flows.read \
  --scope flows.manage \
  --scope flows.run \
  --scope resources.read \
  --scope resources.write

breyta service-accounts create \
  --name full-agent \
  --scope workspace.full
```

`workspace.full` opens the known service-account command and direct-API matrix
for that workspace, but it does not make service-account management or human UI
surfaces machine-accessible.

## Dev utilities

Analyze an old agent authoring session for command/token regressions:

```bash
python scripts/analyze-authoring-session ./session.jsonl
python scripts/analyze-authoring-session ./session.jsonl --json
```

The key is shown once. Store it in the worker environment or your secret
manager before starting the worker process.

Example local worker command:

```bash
BREYTA_API_KEY="<service-account-api-key>" \
breyta jobs worker run --type agent-review --handler ./run-agent-review.sh
```

Use `breyta jobs show <job-id>` for durable control-plane state. Use
`breyta jobs worker state` inside a handler, or against a kept directory with
`--job-dir <dir>`, to inspect the local `job.json`, `payload.json`, and
`result.json` assembly state. Preserve those directories with
`breyta jobs worker run --keep-job-dirs` when you want to inspect them after
the handler exits.

The worker materializes a temp directory for each claimed job and sets
environment such as:

- `BREYTA_JOB_DIR`
- `BREYTA_JOB_CONTEXT_FILE`
- `BREYTA_JOB_FILE`
- `BREYTA_JOB_PAYLOAD_FILE`
- `BREYTA_JOB_RESULT_FILE`
- `BREYTA_JOB_ID`
- `BREYTA_JOB_TYPE`
- `BREYTA_JOB_WORKSPACE_ID`
- `BREYTA_API_URL`
- `BREYTA_WORKSPACE`
- `BREYTA_API_KEY` or `BREYTA_TOKEN`

Additional internal trace env vars may be present, but they are not required
for normal worker implementations.
The helper subcommands reuse the injected worker context plus the worker
API/workspace/auth env, so handlers do not need to pass those flags again.
`BREYTA_JOB_CONTEXT_FILE` is opaque worker context for those helper commands;
handlers typically do not need to parse it directly.

Preferred handler helpers:

- `breyta jobs worker progress` updates lease-scoped progress using the active
  worker env
- `breyta jobs worker state` prints the local worker state snapshot for the
  active job or a kept job directory
- `breyta jobs worker attach-file` uploads a file resource and appends an
  artifact to the local worker result state
- `breyta jobs worker attach-kv` persists structured JSON as a KV-backed
  resource and appends an artifact to the local worker result state
- `breyta jobs worker attach-table` creates or updates a table resource from
  JSON row objects, including schema on write, and appends an artifact to the
  local worker result state
- `breyta jobs worker finish` writes success or no-op result state for the
  parent worker loop to submit
- `breyta jobs worker fail` writes failed result state for the parent worker
  loop to submit

Raw `BREYTA_JOB_RESULT_FILE` writes remain supported for advanced handlers, but
the normal path is to let the CLI build that file.

Minimal handler implementation:

```bash
#!/usr/bin/env bash
set -euo pipefail

surface="$(python3 - "$BREYTA_JOB_PAYLOAD_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    payload = json.load(handle)

print(payload.get("surface", "flows-api"))
PY
)"

report_path="$BREYTA_JOB_DIR/review-report.md"
cat >"$report_path" <<EOF
# Review

Surface: $surface
EOF

breyta jobs worker progress \
  --status running \
  --message "Reviewing $surface"

report_uri="$(breyta jobs worker attach-file \
  --file "$report_path" \
  --label review-report \
  --kind report \
  --print-uri)"

breyta jobs worker finish \
  --summary "Reviewed $surface" \
  --output "surface=$surface" \
  --output finding-count=1 \
  --output "report-resource-uri=$report_uri"
```

Structured resource helpers:

```bash
summary_uri="$(breyta jobs worker attach-kv \
  --label review-summary \
  --key review-summary \
  --field finding-count=1 \
  --field severity=high \
  --print-uri)"

findings_uri="$(breyta jobs worker attach-table \
  --label findings \
  --table security-findings \
  --rows-file "$BREYTA_JOB_DIR/findings.json" \
  --write-mode upsert \
  --key-field finding_id \
  --index-field severity \
  --print-uri)"
```

`attach-table` creates the table resource and its schema on write from the
provided row objects. `--key` and `--table` are logical job-local suffixes; the
API persists the actual KV key or table name under a job-scoped namespace and
returns that effective name on the artifact.

Minimum contract:

- read input from `BREYTA_JOB_PAYLOAD_FILE`
- optionally stream progress with `breyta jobs worker progress`
- optionally persist report/log artifacts with `breyta jobs worker attach-file`
- optionally persist structured summaries with `breyta jobs worker attach-kv`
- optionally persist row-shaped outputs with `breyta jobs worker attach-table`
- mark terminal success or failure with `breyta jobs worker finish` or `breyta jobs worker fail`
- raw `BREYTA_JOB_RESULT_FILE` writes are optional fallback behavior

## Table Resources

Flows can now persist row-shaped outputs directly into table resources:

```clojure
(flow/step :http :fetch-orders
  {:url "https://example.com/orders"
   :persist {:type :table
             :table "orders"
             :rows-path [:body :items]
             :write-mode :upsert
             :key-fields [:order-id]}})
```

That write creates or reuses a table resource and returns a `res://...` table URI/resource ref.

From the CLI, the fastest inspection path is a bounded preview read:

```bash
breyta resources read <res://table-uri> --limit 25 --offset 0
```

For the richer operational surface, use the dedicated table commands:

```bash
breyta resources table query <res://table-uri> --page-mode offset --limit 25 --offset 0
breyta resources table query <res://table-uri> --page-mode cursor --limit 250 --sort-json '[["order-id","asc"]]'
breyta resources table get-row <res://table-uri> --key order-id=ord-1
breyta resources table aggregate <res://table-uri> --group-by currency --metrics-json '[{"op":"count","as":"count"}]'
breyta resources table schema <res://table-uri>
breyta resources table export <res://table-uri> --out orders.csv
breyta resources table import <res://table-uri> --file orders.csv --write-mode append
breyta resources table import orders-import --file orders.csv --write-mode upsert --key-fields order-id --index-fields status
breyta resources table update-cell <res://table-uri> --key order-id=ord-1 --column status --value closed
breyta resources table update-cell-format <res://table-uri> --key order-id=ord-1 --column amount --format-json '{"display":"currency","currency":"USD"}'
breyta resources table materialize-join --left-json '{"table":{"ref":"res://...orders"}}' --right-json '{"table":{"ref":"res://...customers"}}' --on-json '[{"left-field":"customer-id","right-field":"customer-id"}]' --project-json '[{"field":"name","as":"customer-name"}]' --into-json '{"table":"joined-orders","write-mode":"upsert","key-fields":["order-id"]}'
```

`breyta resources table query` now uses the same explicit paging contract as flow `:table` queries:

- `--page-mode` is required
- `--page-mode cursor` also requires `--sort-json`
- the first cursor page omits `--cursor`

CSV import onto an existing table uses the live schema to coerce `boolean`, `number`, `date`, and `timestamp` columns before write. Text and mixed columns stay string-backed unless you write native JSON values.

Dynamic enum columns are authored with `set-column --enum-json`. Writes, CSV import, `update-cell`, and `recompute` normalize incoming ids, names, and aliases to stable stored ids. Unknown values grow the enum definition dynamically. CLI/API query and export surfaces keep those stored ids, while the web table preview renders the configured display names.

Display formatting is render-only. Column `:format` metadata and sparse `update-cell-format` overrides can render `relative-time`, `date`, `timestamp` / `date-time`, and `currency` in the web preview and `Copy Markdown`, while CLI/API query and export surfaces keep canonical raw values.

The flow/runtime surface is mirrored here through the native `:table` step and the CLI for `:query`, `:get-row`, `:aggregate`, `:schema`, `:export`, `:update-cell`, `:update-cell-format`, `:set-column`, `:recompute`, and `:materialize-join`.

## Docs And Help

- Product docs:
  - https://flows.breyta.ai/docs
  - `breyta docs`
  - `breyta docs find "<query>"`
  - `breyta docs show <slug>` only after search identifies the narrow page needed
- External provider/API truth: use current official provider docs/API
  references or model-list endpoints before choosing model ids, endpoints,
  request shapes, auth assumptions, or limits.
- Command usage:
  - `breyta help <command...>`

## Updates

Check for a newer release:

```bash
breyta upgrade
```

Apply an update automatically when supported:

```bash
breyta upgrade --apply
```

Open the latest release page:

```bash
breyta upgrade --open
```

## Repository Policy

This repo is public for transparency and inspectability. We are not accepting
pull requests or code contributions at this time.

- General help, bugs, and feedback: `hello@breyta.ai`
- In-product feedback: `breyta feedback send`
- Security reporting: see `SECURITY.md`

## Maintainer Docs

These docs stay in the repo for Breyta maintainers, but they are not part of
the main user path:

- Release runbook: `docs/RELEASING.md`
- Bundled `parinfer-rust` notes: `tools/parinfer-rust/README.md`
- Bundled `parinfer-rust` build steps: `tools/parinfer-rust/BUILDING.md`

## License

- Repo license: `LICENSE`
- Third-party notices: `THIRD_PARTY_NOTICES.md`
