# Agent guidance (flow authoring)
When you are authoring flows for a user:

- Stop and ask for bindings or activation inputs when required.
- Do not fabricate API keys, OAuth credentials, or webhook secrets.
- Default to reusing existing workspace connections. If a suitable connection already exists (e.g., an OpenAI `:llm-provider`), bind the slot via `<slot>.conn=...` instead of creating a new connection unless the user explicitly wants separate credentials.
- Offer the exact CLI commands or template file paths the user should fill.
- Use templates to collect inputs; keep API-provided `:redacted`/`:generate` placeholders and call out `--clean` when needed.
- Prefer tight loops: implement one step, run it in isolation (`breyta steps run`), then record docs/examples/tests for that step (or use `breyta steps record`, or `breyta steps run --record-example/--record-test`, to capture quickly).
- Treat step test cases as documentation: they preserve intent and expected behavior, and can be executed on demand via `breyta steps tests verify`.
- Apply a size-aware output strategy: when adding data-producing steps (`:http`, `:db`, `:llm`, fanout child items), estimate likely output size first. If size is unknown/unbounded or likely to exceed inline limits, default to `:persist` and pass refs downstream.

Checklist:
1) Ensure the flow exists before bindings work:
   - Run `breyta flows show <slug> --source draft`
   - If it returns `Flow not found`, run `breyta flows create ...` (new flow) or `breyta flows push --file ...` (from local file), then retry `flows show`
2) Validate the draft before bindings:
   - Run `breyta flows validate <slug>` (and `breyta flows compile <slug>` when needed)
   - If validation fails, fix the flow and repeat push/validate before continuing
3) If the flow has `:requires`, generate a template (`flows bindings template` or `flows draft bindings template`).
4) Ask the user to fill secrets/inputs and re-run `flows bindings apply` (or `draft bindings apply`).
5) If OAuth is required, direct the user to the activation URL printed by the template command.
6) Only activate once bindings are applied.

Example prompts to the user:
- "First, let’s see if this workspace already has a suitable connection we can reuse. Run `breyta connections list --type llm-provider`. If you see an existing OpenAI connection, I’ll bind `:ai :conn` to it in the profile template; otherwise we’ll bind a new API key."
- "I need your API key for slot :api. Run `breyta flows bindings template <slug> --out profile.edn`, fill `:api :apikey`, then `breyta flows bindings apply <slug> @profile.edn`."
- "OAuth is required for :google. Run the template command and follow the activation URL printed to stderr."
- "You can keep existing `:conn` values if you don’t want to rebind. Use `--clean` for a fresh template."
