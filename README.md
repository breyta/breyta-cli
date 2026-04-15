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
5. Discover approved examples to copy from
6. Pick a workspace later when you are ready to adopt or build

```bash
breyta init --provider <codex|cursor|claude|gemini>
cd breyta-workspace
breyta auth login
breyta auth whoami
breyta flows search "<idea>"
```

To browse public installable end-user flows for your current workspace instead,
use:

```bash
breyta flows discover list
breyta flows discover search "<idea>"
```

`breyta init` installs the Breyta skill bundle for your agent tool and creates a
local `breyta-workspace/` directory with an `AGENTS.md` file and flow folders.

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
This discover catalog is separate from `breyta flows search`, which is only for
approved example flows to inspect and copy from.

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
```

Inspect runs with the same structured filter syntax as the web runs list:

```bash
breyta runs list --query 'status:failed flow:<slug>'
breyta runs list --installation-id <profile-id> --version 7
```

If you need to revise the note later:

```bash
breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md
```

## Jobs workers

Use the jobs surface when Breyta should orchestrate external work on any machine
without relying on SSH as the transport.

Recommended shape:

- flow creates one job or a small explicit batch
- `breyta jobs worker run` claims and executes that work externally
- flow awaits the normalized job or batch result
- when installability depends on those workers, declare that in the flow
  definition with `{:kind :worker ...}` inside `:requires`

Use fanout or child workflows only when you also need internal Breyta
orchestration. They are not required for the external worker model itself.

Create or inspect jobs directly:

```bash
breyta jobs create --type codex-review --payload '{"surface":"flows-api"}'
breyta jobs batches create --type codex-review --job '{"payload":{"surface":"flows-api"}}' --job '{"payload":{"surface":"runtime"}}'
breyta jobs claim --type codex-review --worker-id worker-1 --lease-duration 2m
```

Run a polling worker loop for one job type:

```bash
breyta jobs worker run --type codex-review --handler ./scripts/run-review.sh
```

Reference demo worker for the `demo.agent-review` job type:

```bash
breyta jobs worker run --type demo.agent-review --handler ./scripts/jobs/demo-agent-review.sh
```

The worker materializes a temp directory for each claimed job and sets
environment such as:

- `BREYTA_JOB_FILE`
- `BREYTA_JOB_PAYLOAD_FILE`
- `BREYTA_JOB_RESULT_FILE`
- `BREYTA_JOB_ID`
- `BREYTA_JOB_TYPE`
- `BREYTA_JOB_LEASE_TOKEN`
- `BREYTA_JOB_WORKSPACE_ID`
- `BREYTA_JOB_ROOT_WORKFLOW_ID`
- `BREYTA_JOB_PARENT_STEP_ID`
- `BREYTA_JOB_FANOUT_PARENT_STEP_ID`
- `BREYTA_JOB_FANOUT_MAX_CONCURRENCY`

Handler result contract:

- success: exit `0`, optionally write `result.json` with `status`, `summary`, `outputs`, `metrics`, `artifacts`, and `workerInfo`
- failure: exit non-zero, optionally write `result.json` with `message`, `code`, `details`, and `artifacts`

Handlers can also use the same CLI surface to stream lease-scoped progress back
to Breyta while they run:

```bash
breyta jobs progress "$BREYTA_JOB_ID" \
  --lease-token "$BREYTA_JOB_LEASE_TOKEN" \
  --status running \
  --message "Writing review report"
```

The shipped `./scripts/jobs/demo-agent-review.sh` example does exactly that:
it reads `payload.json`, emits progress with `breyta jobs progress`, then writes
`result.json` for `succeeded`, `no_changes`, or `failed`.

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

breyta --api "$BREYTA_API_URL" \
  --workspace "$BREYTA_WORKSPACE" \
  --token "$BREYTA_TOKEN" \
  jobs progress "$BREYTA_JOB_ID" \
  --lease-token "$BREYTA_JOB_LEASE_TOKEN" \
  --status running \
  --message "Reviewing $surface"

cat >"$BREYTA_JOB_RESULT_FILE" <<JSON
{"status":"succeeded","summary":"Reviewed $surface","outputs":{"surface":"$surface"}}
JSON
```

Minimum contract:

- read input from `BREYTA_JOB_PAYLOAD_FILE`
- optionally stream progress with `BREYTA_JOB_ID` and `BREYTA_JOB_LEASE_TOKEN`
- write final JSON to `BREYTA_JOB_RESULT_FILE`
- exit non-zero when the handler wants the job to fail

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
  - `breyta docs show <slug>`
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
