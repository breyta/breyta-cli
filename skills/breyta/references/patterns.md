# Patterns and do/dont
## Activation and bindings flow
- Declare `:requires` in the flow definition.
- Apply bindings with a profile file to bind slots and provide activation inputs.
- Enable prod profile with `flows activate --version latest` (enable-only, idempotent).
- Use draft bindings for draft runs.

## Template usage
- Put large payloads in `:templates`.
- Reference with `:template` and `:data` to keep steps readable.

## Output sizing
- Estimate output size before adding data-producing steps.
- Keep inline outputs small and predictable.
- If size is unknown/unbounded (exports, pagination, files), default to `:persist`.
- Pass refs downstream (`:body-from-ref`, `:from-ref`) instead of reconstructing large inline payloads.

## Cross-flow handoff
- For structured state across flows/runs, persist to KV (`:persist {:type :kv :key ...}`) and read with `:kv {:operation :get ...}`.
- Use deterministic keys (workspace + period + shard/page) so retries are idempotent.
- Use `flow/call-flow` only when child binding context is guaranteed.
- If child flow has `:requires`, missing profile context can fail with `requires a flow profile, but no profile-id in context`.

## Polling
- Use `flow/poll` when waiting for an external system to finish.
- Always set `:timeout` or `:max-attempts`.
- Use `:return-on` to define readiness and `:abort-on` for hard failures.
- Prefer `:backoff` over manual interval math.

## Draft vs deploy
- `flows push` creates a draft.
- `flows deploy` publishes the draft and updates trigger routing.

## Do
- Keep flow body deterministic.
- Use `:function` steps for transforms.
- Include a manual trigger for discoverability.
- Use connection slots for credentials.

## Dont
- Hardcode secrets in steps.
- Use `map`/`filter`/`reduce` in flow body.
- Call nondeterministic functions in flow body.
- Do not keep large or unpredictable payloads inline; use `:persist`.
