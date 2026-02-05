# Flows search (approved reuse + workspace)

`breyta flows search` lets agents/humans find reusable *approved* flow examples via flows-api keyword search.

## Why

- Agents building flows should be able to quickly find “good patterns” (approved reusable flows) plus relevant flows in their current workspace.
- Keyword search first; embeddings can follow.

## Command

```bash
breyta flows search <query> \
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

## Reuse workflow

Minimal path (no new install command required):

1) In Flows UI, approve a flow: Flow kebab menu → "Approve for reuse"
2) Run `breyta flows search ... --full`
2) Save the returned `definition_edn` to a local `./tmp/flows/<slug>.clj`
3) Edit as needed
4) `breyta flows push --file ./tmp/flows/<slug>.clj`
5) `breyta flows deploy <slug>`

## Implementation notes

- CLI maps to flows-api command `flows.search` via `POST /api/commands`.
- Search backend is Elasticsearch in production; local dev defaults to an in-memory mock backend.
