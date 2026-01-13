# Clojure assistant tooling (paren repair + optional local REPL)

## Goal
Flow files are Clojure, and LLMs often introduce delimiter errors (`()[]{}`).
We want an “escape hatch” that:

- works for end users (who only install `breyta`)
- is usable by agents explicitly
- can be applied automatically on upload

## Paren repair (recommended)

Manual repair (agent / user):

```bash
breyta flows paren-repair path/to/flow.clj
```

Automatic on upload (default):

```bash
breyta flows push --file path/to/flow.clj
```

By default, if repairs change the content, `push` writes the repaired content back to your local `--file`. Opt out:

```bash
breyta flows push --file path/to/flow.clj --no-repair-writeback
```

You can disable it:

```bash
breyta flows push --file path/to/flow.clj --repair-delimiters=false
```

### “SOTA” engine: Parinfer
When available, `breyta` will prefer `parinfer-rust` (indent mode) for repairs.

Distribution intent:
- Release archives include a `parinfer-rust` executable next to `breyta`.
- Homebrew installs both `breyta` and `parinfer-rust`.

Fallback:
- If `parinfer-rust` is not available (e.g. local dev build), `breyta` falls back to a best-effort delimiter balancer.

Override:
- `BREYTA_PARINFER_RUST=/path/to/parinfer-rust`

## Optional: local REPL tooling (dev-only / power users)
Upstream inspiration: `clojure-mcp-light` (nREPL eval + paren repair + hooks)
https://github.com/bhauman/clojure-mcp-light

Note: End users normally work against the deployed production `flows-api`.
Local nREPL evaluation is primarily useful for product development and advanced debugging.
