# Flows search (approved reuse + workspace)

`breyta flows search` lets agents/humans browse and search reusable *approved* flow examples via flows-api.

## Why

- Agents building flows should be able to quickly find “good patterns” (approved reusable flows) plus relevant flows in their current workspace.
- Keyword search first; embeddings can follow.

## TL;DR

Browse approved flows (no query, most recently approved first):

```bash
breyta flows search
```

Browse with a provider facet:

```bash
breyta flows search --provider stripe
```

Keyword search:

```bash
breyta flows search stripe
```

Inspect (include `definition_edn`):

```bash
breyta flows search stripe --full
```

## Local dev quickstart

When iterating locally, prefer installing the CLI into a repo-local `GOBIN` so you
don’t accidentally run an older `breyta` binary from your global PATH:

```bash
cd breyta-cli
env GOBIN=$PWD/.gopath/bin go install ./cmd/breyta
export BREYTA_API_URL=http://localhost:8090
export BREYTA_TOKEN=dev-user-123
export BREYTA_WORKSPACE=ws-acme
$PWD/.gopath/bin/breyta flows search --provider stripe
```

## Command reference

```bash
breyta flows search [query] \
  --scope all|workspace \
  --provider stripe \
  --limit 10 \
  --from 0 \
  --full
```

Defaults:

- `--scope all` (approved reusable flows across all workspaces)
- `--limit 10`
- `--from 0`
- `--full=false` (no flow definition in response)

## Output

- Always returns indexed metadata + facets (provider tokens, hosts, step types/count).
- With `--full`: includes `definition_edn` (EDN literal) to copy into a new flow file.
- `--limit` is capped server-side (currently 100). The response includes both the requested and effective limits in `meta`.

## Reuse workflow

Minimal path (no new install command required):

1) In Flows UI, approve a flow: Flow kebab menu → "Approve for reuse"
2) Browse first (no query): `breyta flows search --provider stripe` (or search: `breyta flows search stripe`)
3) Inspect a candidate: rerun with `--full` to include `definition_edn`
4) Save `definition_edn` to `./tmp/flows/<slug>.clj`
5) Edit as needed, then:
   - `breyta flows push --file ./tmp/flows/<slug>.clj`
   - `breyta flows deploy <slug>`

## Implementation notes

- CLI maps to flows-api command `flows.search` via `POST /api/commands`.
- Search backend is Elasticsearch in production; local dev defaults to an in-memory mock backend.
