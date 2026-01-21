## Development (contributors)

This page is for developing the `breyta` CLI itself (mock mode, local demos, and tests).

For running the CLI against a local `flows-api` server, see `docs/flows-api-local.md`.

### Build & test

```bash
go build ./...
go test ./...
```

Or (if you have `make`):

```bash
make build
make test
```

### GitHub PR checks

CI runs on pull requests via `.github/workflows/ci.yml`.

To make PRs mandatory for `main`, enable branch protection in GitHub:

- Require a pull request before merging
- Require status checks to pass: `CI / test`

### Mock mode (local CLI/TUI development)

The CLI includes a mock mode used for local development and TUI iteration.

Dev-only commands and flags are hidden unless you enable dev mode:

```bash
export BREYTA_DEV=1
```

Reset/seed mock state (recommended before demos):

```bash
BREYTA_DEV=1 breyta dev seed
```

#### Mock state file

By default the CLI uses an OS config location:

- macOS/Linux: `~/.config/breyta/mock/state.json` (via `os.UserConfigDir()`)

Override location with:

- `--state /path/to/state.json`
- or `BREYTA_MOCK_STATE=/path/to/state.json`

Workspace selection:

- `--workspace ws-acme`
- or `BREYTA_WORKSPACE=ws-acme`

#### Two-terminal workflow (recommended)

Terminal A (TUI):

```bash
breyta
```

Terminal B (drive changes):

```bash
BREYTA_DEV=1 breyta dev seed
BREYTA_DEV=1 breyta dev advance --ticks 1
BREYTA_DEV=1 breyta dev advance --ticks 1
```

The TUI refreshes when the mock state file changes.

### TUI navigation (quick reference)

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

### Demo guide (copy/paste)

Terminal A (truth surface):

```bash
breyta
```

- Dashboard opens as a navigation hub.
- Press `f` to open Flows, select `subscription-renewal`, press Enter.
- In the split view: left pane is stable flow info + steps; right pane shows focused step data.
- Press `r` to open Runs; open run `4821` and inspect step IO.
- Press `m` to open Marketplace; use `1/2/3/4` tabs.

Terminal B (drive changes):

```bash
breyta flows show subscription-renewal
breyta runs list subscription-renewal
breyta runs step 4821 process-card
breyta runs replay 4821
```

“Build a flow” angle:

```bash
breyta flows create --slug hello-market --name "Hello Market"
breyta flows steps set hello-market fetch --type http --title "Fetch sample payload" --definition "(step :http :fetch {:connection :demo :path \"/sample\"})"
breyta flows steps set hello-market summarize --type function --title "Summarize payload" --definition "(step :function :summarize {:code '(fn [x] ...)})"
breyta flows validate hello-market
breyta runs start --flow hello-market
BREYTA_DEV=1 breyta dev advance --ticks 3
```

If Terminal A is open, you should see the dashboard update when you seed/start/advance/replay runs.

### CLI output envelope (mocked surface)

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
