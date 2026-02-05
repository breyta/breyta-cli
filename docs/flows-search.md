# Flows search (approved reuse + workspace)

This doc describes the `breyta flows search` command that lets agents/humans find reusable flows (and eventually steps) via Elasticsearch-backed search in flows-api.

## Why

- Agents building flows should be able to quickly find “good patterns” (approved reusable flows) plus relevant flows in their current workspace.
- Keyword search first; embeddings can follow.

## Command

```bash
breyta flows search <query> \
  --scope all|workspace|public \
  --provider stripe \
  --limit 20 \
  --full
```

Defaults:

- `--scope all` (approved reusable flows + current workspace flows)
- `--limit 20`
- `--full=false` (no flow definition in response)

## Output

- Always returns: `flowSlug`, `name`, `description`, `tags`, `score`, `providers`, `stepTypes`, `stepCount`, `source` (`active|draft`), and a `hint` on how to reuse.
- With `--full`: includes `definitionEdn` so you can save it to a file and reuse it immediately.

## Reuse workflow

Minimal path (no new install command required):

1) Run `breyta flows search ... --full`
2) Save the returned `definitionEdn` to a local `./tmp/flows/<slug>.clj`
3) Edit as needed
4) `breyta flows push --file ./tmp/flows/<slug>.clj`
5) `breyta flows deploy <slug>`

## Implementation notes

- CLI maps to flows-api command `flows.search` via `POST /api/commands`.
- Search is powered by Elasticsearch and only returns what the server allows.
