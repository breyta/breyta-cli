## Breyta CLI (mock)

A standalone **Go** CLI + **Bubble Tea** TUI for exploring the Breyta product experience **without any real backend**.

- Running **`breyta`** launches an interactive TUI.
- Running **`breyta flow ...`**, **`breyta run ...`**, etc. returns **mock JSON** by default (or EDN with `--format edn`).
- All commands share a single **mock state file**, so you can keep the TUI open in one terminal and drive changes from another.

### Goals

- **Truth surface first**: inspect flows and drill down into step detail.
- **Scriptable CLI**: every command returns stable JSON (optionally `--pretty`).
- **Mock-only for now**: no HTTP calls, no Temporal, no Firestore.

### CLI docs for agents

The CLI supports both:

- `breyta <cmd> --help` for human-readable help
- `breyta docs` for on-demand docs (Markdown by default)
  - `breyta docs run list` (command-specific docs)
  - `breyta docs run list --format json|edn` (structured docs)
  - Add global `--format edn` for EDN output in normal commands.

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
./dist/breyta flow list --pretty
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
- **Tab**: switch pane (dashboard)
- **g**: dashboard
- **s**: settings
- **q**: quit

TUI flow:
1. **Workspace dashboard** (flows + recent runs)
2. Select a flow → **Flow detail** (structure + steps)
3. Select a run → **Run detail** (steps)
4. Enter on a step → **Step detail** (definition + input/output schemas + previews)

### Demo guide (copy/paste)

#### Reset state (recommended before every demo)

```bash
breyta mock seed --pretty
```

#### Terminal A (truth surface)

```bash
breyta
```

- Dashboard opens with **Flows** (left) and **Recent runs** (right).
- Use **Tab** to move focus between panes.
- Open `subscription-renewal` to see the spine + steps.
- Open `subscription-renewal` to see the steps list.
- Open run `4821` from Recent runs to inspect its step timeline.

#### Terminal B (drive changes)

Marketplace angle:

```bash
breyta flow show subscription-renewal --pretty
breyta run list subscription-renewal --pretty
breyta run step 4821 process-card --pretty
breyta run replay 4821 --pretty

breyta revenue show --last 30d --pretty
breyta demand top --window 30d --pretty
```

“Build a flow” angle:

```bash
breyta flow create --slug hello-market --name "Hello Market" --description "Minimal flow for demo" --pretty
breyta flow step-set hello-market fetch --type http --title "Fetch sample payload" --input-schema "{id: string}" --output-schema "{status: number, body: any}" --definition "(step :http :fetch {:connection :demo :path \"/sample\"})" --pretty
breyta flow step-set hello-market summarize --type llm --title "Summarize payload" --input-schema "{payload: any}" --output-schema "{summary: string}" --definition "(step :llm :summarize {:model \"gpt-4.1\" :prompt ...})" --pretty
breyta flow deploy hello-market --pretty
breyta run start --flow hello-market --pretty
breyta mock advance --ticks 3 --pretty
```

If Terminal A is open, you should see the dashboard update when you seed/start/advance/replay runs.

#### CLI commands (mock)

```bash
breyta docs
breyta docs run list
breyta docs run list --format edn --pretty

breyta flow list --pretty
breyta flow show daily-sales-report --pretty
breyta flow spine daily-sales-report --pretty

breyta run list daily-sales-report --pretty
breyta run list daily-sales-report --include-steps --pretty
breyta run show wf-demo-001 --pretty
breyta run show wf-demo-001 --steps 0 --pretty
breyta run start --flow daily-sales-report --pretty
breyta run replay 4821 --pretty
breyta run step 4821 process-card --pretty

breyta revenue show --last 30d --pretty
breyta demand top --window 30d --pretty

breyta mock seed --pretty
breyta mock advance --ticks 1 --pretty
```

### Mock state file

By default the CLI uses an OS config location:

- macOS/Linux: `~/.config/breyta/mock/state.json` (via `os.UserConfigDir()`)

Override location with:

- `--state /path/to/state.json`
- or `BREYTA_MOCK_STATE=/path/to/state.json`

Workspace selection:

- `--workspace demo-workspace`
- or `BREYTA_WORKSPACE=demo-workspace`

### Two-terminal workflow (recommended)

Terminal A:

```bash
breyta
```

Terminal B:

```bash
breyta run start --flow daily-sales-report --pretty
breyta mock advance --ticks 1 --pretty
breyta mock advance --ticks 1 --pretty
```

The TUI refreshes when the mock state file changes.

### Next steps (when you want to go beyond mock)

- Replace the mock store with an HTTP-backed store that talks to `flows-api`.
- Add an SSE client for live run updates.
- Keep the TUI structure unchanged: **flows → spine → step**.
