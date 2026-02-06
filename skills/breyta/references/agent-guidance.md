# Agent guidance (flow authoring)
When you are authoring flows for a user:

- Stop and ask for bindings or activation inputs when required.
- Do not fabricate API keys, OAuth credentials, or webhook secrets.
- Default to reusing existing workspace connections. If a suitable connection already exists (e.g., an OpenAI `:llm-provider`), bind the slot via `<slot>.conn=...` instead of creating a new connection unless the user explicitly wants separate credentials.
- Offer the exact CLI commands or template file paths the user should fill.
- Use templates to collect inputs; keep API-provided `:redacted`/`:generate` placeholders and call out `--clean` when needed.
- Prefer tight loops: implement one step, run it in isolation (`breyta steps run`), then record docs/examples/tests for that step (or use `breyta steps record`, or `breyta steps run --record-example/--record-test`, to capture quickly).
- Treat step test cases as documentation: they preserve intent and expected behavior, and can be executed on demand via `breyta steps tests verify`.
- Keep step ids unique. If you add retries, use distinct ids for each attempt.
- Do not remove validation or guard steps unless the user explicitly asks.

Checklist:
1) If the flow has `:requires`, generate a template (`flows bindings template` or `flows draft bindings template`).
2) Ask the user to fill secrets/inputs and re-run `flows bindings apply` (or `draft bindings apply`).
3) If OAuth is required, direct the user to the activation URL printed by the template command.
4) Only activate once bindings are applied.
5) Run `breyta flows validate <slug>` after pushing the draft.
6) Run changed steps in isolation before a full draft run.
7) Do not deploy/activate until a draft run finishes without errors.
8) For large or binary step outputs, add `:persist` or filter the inputs.

Example prompts to the user:
- "First, let’s see if this workspace already has a suitable connection we can reuse. Run `breyta connections list --type llm-provider`. If you see an existing OpenAI connection, I’ll bind `:ai :conn` to it in the profile template; otherwise we’ll bind a new API key."
- "I need your API key for slot :api. Run `breyta flows bindings template <slug> --out profile.edn`, fill `:api :apikey`, then `breyta flows bindings apply <slug> @profile.edn`."
- "OAuth is required for :google. Run the template command and follow the activation URL printed to stderr."
- "You can keep existing `:conn` values if you don’t want to rebind. Use `--clean` for a fresh template."
