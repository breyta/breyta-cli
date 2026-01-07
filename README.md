## Breyta CLI (local dev)

This is a standalone **Go** CLI + **Bubble Tea** TUI for working with Breyta locally.

- Running **`breyta`** launches an interactive TUI.
- Running **`breyta flows ...`** returns **JSON** by default (or EDN with `--format edn`).

### Docs
- Install: `docs/install.md`
- Local flows authoring (flows-api): `docs/flows-api-local.md`
- Agentic chat setup (Claude Code, Cursor, Codex, etc.): `docs/agentic-chat.md`

This CLI supports two modes:
- **API mode (recommended for flows authoring)**: `--api` / `BREYTA_API_URL` points to a locally running `flows-api`.
- **Mock mode (dev/TUI)**: uses a local mock state file.

Default API target (when you run any subcommand) is `https://flows.breyta.io`.
Switch between prod/local without exporting env vars:

```bash
breyta api use prod
breyta api use local
breyta api show
```

### Goals

- **Truth surface first**: inspect flows and drill down into step detail.
- **Scriptable CLI**: every command returns stable JSON (use `--pretty` only for human-readable formatting).
- **API-backed flows (local)**: operate on real flow store/versioning via `flows-api`.

### Quick start: operate on flows via flows-api (API mode)

Start the server (repo root):

```bash
cd ../breyta
./scripts/start-flows-api.sh --emulator --auth-mock
```

Configure the CLI:

```bash
breyta api use local
export BREYTA_WORKSPACE="ws-acme"
# In local mock auth, any non-empty token works (membership is still enforced).
export BREYTA_TOKEN="dev-user-123"
```

Login helper (API mode):

```bash
# Opens a browser to login, then stores the token locally (recommended):
breyta auth login

# Optional: print an export line for the current shell:
breyta auth login --print export

# Legacy (password exchange):
breyta auth login --email you@example.com --password-stdin --print export
```

Then:

```bash
breyta flows list
breyta flows pull simple-http --out ./tmp/flows/simple-http.clj
# edit file
breyta flows push --file ./tmp/flows/simple-http.clj
breyta flows deploy simple-http
breyta flows show simple-http
```

Notes:
- Mock auth accepts any non-empty token, but the API enforces **workspace membership**. The dev server seeds `ws-acme` and `dev-user-123`.
- By default, most mock/future command groups are hidden. Use `--dev` / `BREYTA_DEV=1` to access the full mocked surface.
- Flow slugs in API mode must match the API slug format: `^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`.

### Run a flow and read its output (API mode)

Runs are available via the `runs.*` command endpoint. In the CLI, use `--dev` to access `breyta runs ...`:

```bash
# Start a run and wait for completion. Output is in:
#   data.run.resultPreview.data.result
breyta --dev runs start --flow run-hello --input '{"n":41}' --wait

# Inspect a run by run-id
breyta --dev runs show abc-123-def
```

Resources are the preferred unified surface (results, imports, files). Example:

```bash
breyta resources workflow list abc-123-def
breyta resources get res://v1/ws/ws-acme/result/run/abc-123-def/step/fetch/output
breyta resources read res://v1/ws/ws-acme/result/run/abc-123-def/step/fetch/output
```

### Skill bundle (for agents)
The Breyta server repo includes an Anthropic-style skill bundle at `breyta/skills/breyta-flows-cli/` (with `SKILL.md` + a `bin/breyta` wrapper).

Install by copying it into your tool’s skills directory. Common locations:
- Claude Code: `~/.claude/skills/user/breyta-flows-cli/`
- Cursor: `~/.cursor/skills/breyta-flows-cli/`
- Codex: `~/.codex/skills/breyta-flows-cli/`

See `breyta/skills/breyta-flows-cli/SKILL.md` for the exact copy/paste verification flow.

### CLI docs for agents

The CLI supports both:

- `breyta <cmd> --help` for human-readable help
- `breyta docs` for on-demand docs (Markdown by default)
  - `breyta docs runs list` (command-specific docs)
  - `breyta docs runs list --format json|edn` (structured docs)
  - Add global `--format edn` for EDN output in normal commands.

Dev-only commands:
- `breyta dev ...` (hidden unless `BREYTA_DEV=1` or `--dev`)

### Install / build

#### Option A: `go install` (recommended)

From this directory:

```bash
go install ./cmd/breyta
```

This installs `breyta` into `$(go env GOPATH)/bin`.

#### Option B: build a local binary

```bash
make build
./dist/breyta flows list
```

### Running

#### TUI

```bash
breyta
```

Navigation:
- **Up/Down**: move selection
- **Enter**: open
- **Esc / Backspace**: back
- **Tab**: switch pane (only in split views)
- **g**: dashboard
- **f**: flows table
- **r**: runs table
- **m**: marketplace
- **s**: settings
- **q**: quit

TUI flow:
1. **Dashboard** (navigation only)
2. **Flows** (full table) → Enter on a flow opens **Flow split view** (left: stable flow info + steps, right: focused step IO)
3. **Runs** (full table) → Enter on a run opens **Run split view** (left: stable run info + steps, right: focused step IO)
4. **Marketplace** (full table) with tabs:
   - `1` revenue
   - `2` demand (clusters)
   - `3` registry
   - `4` payouts

### Demo guide (copy/paste)

#### Reset state (recommended before every demo)

```bash
BREYTA_DEV=1 breyta dev seed
```

#### Terminal A (truth surface)

```bash
breyta
```

- Dashboard opens as a navigation hub.
- Press `f` to open Flows, select `subscription-renewal`, press Enter.
- In the split view: left pane is stable flow info + steps; right pane shows focused step data.
- Press `r` to open Runs; open run `4821` and inspect step IO.
- Press `m` to open Marketplace; use `1/2/3/4` tabs.

#### Terminal B (drive changes)

Marketplace angle:

```bash
breyta flows show subscription-renewal
breyta runs list subscription-renewal
breyta runs step 4821 process-card
breyta runs replay 4821

breyta revenue show --last 30d
breyta demand top --window 30d
breyta demand clusters
breyta demand ingest "Need subscription renewal with retries" --offer-cents 1000 --currency USD

breyta registry search "subscription"
breyta registry show wrk-subscription-renewal
breyta registry match "Renew subscriptions and retry payments"
breyta pricing show wrk-subscription-renewal
breyta purchases create wrk-subscription-renewal --buyer buyer@demo.test
breyta entitlements list
breyta payouts list
breyta creator dashboard
breyta analytics overview
```

“Build a flow” angle:

```bash
breyta flows create --slug hello-market --name "Hello Market"
breyta flows steps set hello-market fetch --type http --title "Fetch sample payload" --definition "(step :http :fetch {:connection :demo :path \"/sample\"})"
breyta flows steps set hello-market summarize --type code --title "Summarize payload" --definition "(step :code :summarize {:code '(fn [x] ...)})"
breyta flows validate hello-market
breyta runs start --flow hello-market
BREYTA_DEV=1 breyta dev advance --ticks 3
```

If Terminal A is open, you should see the dashboard update when you seed/start/advance/replay runs.

#### CLI commands (mock)

```bash
breyta docs
breyta docs flows
breyta docs flows steps set
breyta docs runs list
breyta docs runs list --format edn

breyta flows list
breyta flows show daily-sales-report
breyta flows spine daily-sales-report
breyta flows steps list daily-sales-report
breyta flows steps show daily-sales-report fetch-sales --include schemas,definition

breyta runs list daily-sales-report
breyta runs show run-abc-123
breyta runs show run-abc-123 --steps 0
breyta runs start --flow daily-sales-report
breyta runs replay run-abc-123
breyta runs step run-abc-123 process-card
breyta runs events run-abc-123
breyta runs cancel run-abc-123 --reason "stopping demo"

breyta revenue show --last 30d
breyta demand top --window 30d
breyta demand clusters
breyta demand ingest "Need order approval with fraud checks" --offer-cents 250

breyta registry search "sales report"
breyta registry show wrk-daily-sales-report
breyta registry publish daily-sales-report --title "Daily Sales Report" --model subscription --amount-cents 1500 --currency USD --interval month --note "Demo publish"
breyta registry versions wrk-daily-sales-report
breyta pricing set wrk-daily-sales-report --model subscription --amount-cents 2000 --interval month
breyta purchases list
breyta entitlements list
breyta payouts list
breyta creator dashboard
breyta analytics overview

BREYTA_DEV=1 breyta dev seed
BREYTA_DEV=1 breyta dev advance --ticks 1
```

### Mock state file

By default the CLI uses an OS config location:

- macOS/Linux: `~/.config/breyta/mock/state.json` (via `os.UserConfigDir()`)

Override location with:

- `--state /path/to/state.json`
- or `BREYTA_MOCK_STATE=/path/to/state.json`

Workspace selection:

- `--workspace ws-acme`
- or `BREYTA_WORKSPACE=ws-acme`

### Two-terminal workflow (recommended)

Terminal A:

```bash
breyta
```

Terminal B:

```bash
breyta run start --flow daily-sales-report
breyta mock advance --ticks 1
breyta mock advance --ticks 1
```

The TUI refreshes when the mock state file changes.

### Testing

The CLI has **command contract tests** that execute commands and assert stable output envelopes.

```bash
make test
```

Or:

```bash
go test ./...
```

### Next steps

- Expand the API-backed surface area beyond flows (runs/profiles/triggers/etc).
- Add route-level tests for auth/membership failure cases on the command endpoint.
- Consider adding a “workspace bootstrap” command for local dev (create workspace + membership).

### V1 CLI surface (spec, mocked)

All commands return a stable envelope:

- `ok` (bool)
- `workspaceId` (string)
- `meta` (object, optional)
- `data` (object, optional)
- `error` (object, optional, when `ok=false`)

Command groups (implemented as mocks in this repo):

- **Core**: `flows`, `runs`, `connections`, `profiles`, `triggers`, `waits`, `watch`, `auth`, `workspaces`, `docs`, `dev`
- **Marketplace**:
  - `registry search|show|publish|versions|match|install`
  - `pricing show|set`
  - `purchases list|show|create`
  - `entitlements list|show`
  - `payouts list|show`
  - `creator dashboard`
  - `analytics overview`
  - `demand top|clusters|cluster|queries|ingest`
