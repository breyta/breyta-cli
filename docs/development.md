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

### Mock mode (local CLI development)

The CLI includes a mock mode used for local development and command demos.

Dev-only commands and flags are hidden unless you enable dev mode. You can either:

- enable dev mode for a single command with `--dev`
- or enable it persistently with `breyta internal dev enable`
- and disable it persistently with `breyta internal dev disable`

You can also force a specific dev profile for a single command (docs place it at the end for readability):

```bash
breyta flows list --dev=local
```

If you need separate auth tokens for the same API URL, set an auth store path per profile:

```bash
breyta internal dev set local --auth-store ~/.config/breyta/auth.local.json
breyta internal dev set staging --auth-store ~/.config/breyta/auth.staging.json
```

Note: mock auth doesn’t require separate auth stores, but you can still run `breyta auth login`
to exercise the endpoint if you want.

Reset/seed mock state (recommended before demos):

```bash
breyta --dev dev seed
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

### Demo guide (copy/paste)

```bash
breyta flows show subscription-renewal
breyta runs list subscription-renewal
breyta runs step 4821 process-card
breyta runs replay 4821
```

“Build a flow” angle:

```bash
breyta flows create --slug hello-market --name "Hello Market"
breyta flows validate hello-market
breyta runs start --flow hello-market
breyta --dev dev advance --ticks 3
```

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
