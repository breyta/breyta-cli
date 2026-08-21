# Breyta CLI

`breyta` is the command-line client for a self-hosted Breyta engine. It does
not start or manage the server; point it at an engine API and select a
workspace.

## Build the open-source branch

The canonical CLI is not published yet. Existing GitHub releases and the
`@latest` Go module refer to the retiring hosted product and must not be used
for this branch. Build the reviewed `open-source/engine` source locally:

```bash
go build -o ./dist/breyta ./cmd/breyta
```

Automatic publication is disabled. A future, separately reviewed release
change must define versioning, signing and distribution ownership before any
new CLI archive can be published.

## Connect to an engine

The default API is `http://localhost:8090`. Configure another local or remote
installation with:

```bash
breyta api use https://breyta.example.com
breyta api check
```

`api check` reads `/api/capabilities` and rejects an incompatible engine API or
workspace-handoff contract. A one-off endpoint can be supplied with `--api` or
`BREYTA_API_URL`.

Create a personal access token in the engine operator UI, then authenticate
with the local auth store or an environment variable. Service accounts use
their own capability-scoped keys.

```bash
export BREYTA_TOKEN='...'
export BREYTA_WORKSPACE='ws-example'
breyta auth whoami
```

The equivalent global flags are `--token`, `--api-key`, `--api`, and
`--workspace`. There is one API-backed product surface; the CLI has no hosted,
development, or mock mode.

## Author and run flows

The normal workflow keeps editable source in a local `.clj` file:

```bash
breyta flows init example --name "Example"
breyta flows pull example --out ./flows/example.clj
breyta flows lint --file ./flows/example.clj --local-only
breyta flows push --file ./flows/example.clj
breyta flows diff example
breyta flows release example --release-note "Ready for use"
breyta flows run example --target live --input '{"value": 42}' --wait
```

The engine is the canonical validator and executor. The CLI also exposes the
retained run, connection, secret, trigger, wait, resource, workspace-member,
and service-account operations.

Use `breyta help`, `breyta help <command>`, and `breyta --pretty ...` for
command discovery and readable JSON output.

## Agent skill

The configured engine publishes the matching Breyta agent skill. Install it
directly from that engine instead of using guidance from a hosted CLI release:

```bash
breyta skills install --provider codex
breyta skills status --provider codex
```

Use `--provider all` to manage Codex, Cursor, Claude, and Gemini targets. Skill
files are verified against the engine manifest and installed verbatim; the CLI
does not rewrite engine-owned guidance. Locally modified managed files are
backed up before replacement.

## Workspace handoff

Export a versioned, non-secret workspace bundle:

```bash
breyta workspace export --out workspace.json
```

Import it into the selected destination workspace:

```bash
breyta workspace import \
  --file workspace.json \
  --connection-mappings connection-mappings.json \
  --resource-mappings resource-mappings.json \
  --reviewed-dependencies \
  --enable-triggers
```

Connection mappings are a JSON object from source connection id to destination
connection id. Resource mappings are a JSON object from source resource URI to
the destination resolution, for example:

```json
{
  "res://v1/ws/source/result/file/catalog.csv": {
    "status": "intentionally-empty"
  }
}
```

Import refuses a non-empty workspace unless `--allow-merge` is explicit.
Triggers are enabled only when dependencies are resolved and both
`--reviewed-dependencies` and `--enable-triggers` are supplied. Bundles exclude
credentials, memberships, sessions, run history, active workflows, and
file/table contents.

## Development

```bash
go build ./...
go test ./...
```

See [docs/RELEASING.md](docs/RELEASING.md) for the release gate.
