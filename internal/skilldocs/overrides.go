package skilldocs

import "strings"

// ApplyCLIOverrides patches downloaded skill bundle text to keep command guidance
// aligned with current CLI behavior.
func ApplyCLIOverrides(skillSlug string, files map[string][]byte) map[string][]byte {
	if strings.TrimSpace(skillSlug) != "breyta" {
		return files
	}
	raw, ok := files["SKILL.md"]
	if !ok || len(raw) == 0 {
		return files
	}

	original := string(raw)
	updated := original

	currentCanonicalSkill := strings.Contains(updated, "## Create/Edit Preflight") &&
		(strings.Contains(updated, "## Public Flow Presentation") ||
			strings.Contains(updated, "## Public Approval Gate")) &&
		(strings.Contains(updated, "## Model Selection") ||
			strings.Contains(updated, "## Provider/API Freshness And Model Selection")) &&
		strings.Contains(updated, "## Output Guidance") &&
		(strings.Contains(updated, "## Reference Loading Matrix") ||
			strings.Contains(updated, "## Public Flow Presentation"))

	replacements := [][2]string{
		{
			"inspect docs/bindings/tables/fanout/outputs/OpenAI models, and report full",
			"inspect docs/bindings/tables/fanout/outputs/provider APIs/models, and report full",
		},
		{
			"touching OpenAI-backed LLM steps.",
			"touching external API calls or LLM provider/model config.",
		},
		{
			"- For new or changed OpenAI model config, avoid stale GPT-4-era defaults. Check current OpenAI model guidance and relevant Breyta examples/docs; preserve an explicit user-requested target such as `gpt-5.4`. As of the current OpenAI latest-model guide, `gpt-5.5` is the latest model.",
			"- For new or changed external API, LLM provider, or model config, avoid stale training-data defaults. Do a quick source-of-truth check against current official provider docs/API references and relevant Breyta docs/examples before choosing endpoints, request shape, auth, rate-limit assumptions, or model ids. For OpenAI-backed steps, use `gpt-5.4` as Breyta's current API default where a default is needed, but still verify availability; do not claim or use unreleased models such as `gpt-5.5` without provider/API proof.",
		},
		{
			"- OpenAI models: check current OpenAI docs before introducing or changing model ids",
			"- external APIs/models: check current official provider docs/API references before introducing or changing endpoints, request bodies, auth, limits, or model ids",
		},
		{
			"## Model Selection\n\n- Do not default new OpenAI-backed LLM steps to GPT-4-era model names.\n- Before adding or changing OpenAI model ids, check current OpenAI model guidance and relevant Breyta docs/examples.\n- As of the current OpenAI latest-model guide, `gpt-5.5` is the latest model.\n- Preserve explicit user requests, such as `gpt-5.4`, even when newer models exist.\n- When editing existing flows, keep legacy models only if compatibility, cost, or evaluation history is intentional. Otherwise propose upgrading to the current approved GPT-5.x model.",
			providerAPIFreshnessSection,
		},
		{
			"## Model Selection",
			"## Provider/API Freshness And Model Selection",
		},
		{
			"- Do not default new OpenAI-backed LLM steps to GPT-4-era model names.",
			"- Do not rely on model training data for current API shape, provider behavior, or model availability.",
		},
		{
			"- Before adding or changing OpenAI model ids, check current OpenAI model guidance and relevant Breyta docs/examples.",
			"- Before adding or changing any external API integration, check current official provider docs/API references plus relevant Breyta docs/examples. Apply this to OpenAI, Anthropic/Claude, Google/Gemini, OpenAI-compatible providers, HTTP APIs, databases, and any vendor a flow calls.\n- Before adding or changing model ids, check the provider's current model docs or model-list API when credentials/tooling are available. Verify the exact model id with a draft run or isolated step run before release when feasible.\n- For OpenAI-backed steps, use `gpt-5.4` as Breyta's current API default where a default is needed, but still verify availability in the target environment.",
		},
		{
			"- As of the current OpenAI latest-model guide, `gpt-5.5` is the latest model.",
			"- Do not claim or use unreleased provider models, such as `gpt-5.5`, without provider/API proof.",
		},
		{
			"- Preserve explicit user requests, such as `gpt-5.4`, even when newer models exist.",
			"- Preserve explicit user requests, such as `gpt-5.4` or a specific Claude/Gemini model, unless current provider docs/API availability show the model is unavailable or unsuitable.",
		},
		{
			"- When editing existing flows, keep legacy models only if compatibility, cost, or evaluation history is intentional. Otherwise propose upgrading to the current approved GPT-5.x model.",
			"- When editing existing flows, keep legacy models/APIs only if compatibility, cost, or evaluation history is intentional. Otherwise propose upgrading to the current verified provider/API choice.",
		},
		{
			"- Reuse existing workspace connections before creating new ones.",
			"- Inventory and validate existing workspace connections before creating new ones.",
		},
		{
			"- Search templates before creating or editing a flow: `breyta flows search \"<problem or integration query>\" --limit 5 --pretty`.",
			"- Search approved examples before creating a new flow, changing architecture, using unfamiliar integrations, or when primitive examples are missing: `breyta flows search \"<problem or integration query>\" --limit 5`.",
		},
		{
			"- Use `breyta flows search \"<query>\" --limit 1 --full --pretty` for the best one or two matches when structure matters.",
			"- For primitive/step edits, use matching primitive snippets and referenced dependencies when available. Inspect a full template only for cross-step architecture reuse, public install patterns, multi-flow orchestration, fanout/child-flow behavior, unclear snippet dependencies, or copying overall flow structure.",
		},
		{
			"`current state -> docs search -> template search -> full template inspect -> compare -> edit -> draft proof -> live/install proof`.",
			"`current state -> workspace search -> private snippet -> docs snippets -> approved example metadata -> approved primitive snippet -> referenced dependencies -> full template only if needed -> compare -> edit -> draft proof -> live/install proof`.",
		},
		{
			"- Treat flow grouping as mutable metadata, not authored source. Verify grouping with `breyta flows list --pretty` or `breyta flows show <slug> --pretty`.",
			"- Treat flow grouping as mutable metadata, not authored source. Verify grouping with `breyta flows list --limit 50` or `breyta flows show <slug>` so `groupFlows` and ordering metadata are visible. Use `--full` only when definition fields are needed.",
		},
		{
			"- Before creating a new flow, search existing definitions: `breyta flows search <query>`.",
			"- Before creating or editing a flow, pick a task mode, inspect current state, search nearby workspace patterns with `breyta flows workspace search \"<integration or problem query>\" --limit 5`, search docs snippets for touched primitives, and search approved examples with `breyta flows search \"<problem or integration query>\" --limit 5`.\n- Use private/approved primitive snippets and referenced `:requires`, `:templates`, and `:functions` before pulling full templates.\n- Inspect a full template only for cross-step architecture reuse, public install patterns, multi-flow orchestration, fanout/child-flow behavior, unclear snippet dependencies, or copying overall flow structure.\n- For new flows, use workspace search instead of `breyta flows list --limit 50` for pattern discovery.\n- For existing flows, inspect the target with `breyta flows show <slug>` or `breyta flows pull <slug>` before editing.",
		},
		{
			"3. Confirm reusable resources:\n   - `breyta connections list`\n   - `breyta flows search <query>`",
			"3. Confirm reusable resources:\n   - `breyta connections list`\n   - `breyta connections show <id>` for the connection you expect to bind\n   - `breyta connections test <id>` only when binding or debugging that connection\n   - Nearby workspace patterns: `breyta flows workspace search \"<integration or problem query>\" --limit 5`\n   - Private primitive snippets: `breyta flows workspace examples step <type> \"<query>\" --limit 3`\n   - Docs snippets: `breyta docs find \"<problem or primitive>\"`\n   - Approved example discovery: `breyta flows search \"<problem or integration query>\" --limit 5`\n   - Primitive-first reuse: inspect matching snippets and referenced dependencies before full templates\n   - Existing workspace flow: `breyta flows show <slug>` or `breyta flows pull <slug>`",
		},
		{
			"2. Bootstrap from existing artifacts\n- Prefer existing flow file first:\n  - `breyta flows pull <slug> --out ./tmp/flows/<slug>.clj`\n\n3. Working copy iteration\n- Before editing `:flow`, shape the reusable surfaces first:\n  - `:templates` for large static content\n  - `:functions` for deterministic transforms\n  - packaged `:steps` for heavy built-in step configs\n  - `:agents` for reusable reviewer/fixer/coordinator behavior",
			"2. Bootstrap from existing artifacts and connections\n- inventory and test the connections you expect the flow to need before editing behavior\n- decide which business capabilities become `:requires` slots and which existing workspace connections should satisfy them\n- if the flow will expose packaged `:steps` or `:agents` as tools, decide whether any reused connection needs flow-local scope limits before tool publication\n- Prefer existing flow file first:\n  - `breyta flows pull <slug> --out ./tmp/flows/<slug>.clj`\n\n3. Working copy iteration\n- Before editing `:flow`, shape the reusable surfaces first:\n  - `:requires` around validated connection slots, secrets, and input contracts\n  - `:templates` for large static content\n  - `:functions` for deterministic transforms\n  - packaged `:steps` for heavy built-in step configs\n  - `:agents` for reusable reviewer/fixer/coordinator behavior",
		},
		{
			"- Schedule triggers require a prod profile before deploy and activate",
			"- Schedule triggers require a prod profile before deploy and activate\n- For keyed concurrency (`:type :keyed`), schedule `:config :input` must include the key-field (for example `:request-id`) or activation will fail",
		},
		{
			"- Run a hard quality gate before each build/release cycle",
			"- Run a planning gate before each build/release cycle (required). Keep quality checks, but do not block early iteration with a rigid quality gate before design is complete",
		},
		{
			"- Quality gate is the first step",
			"- Planning gate is the first step",
		},
		{
			"- Include web links from CLI JSON when available (`meta.webUrl` / `data.*.webUrl`) so users can inspect in Breyta web.",
			"- Prefer exact recovery URLs from failures when available: `error.actions[].url` first, then `meta.webUrl`.\n- For successful reads/runs, include web links from CLI JSON (`meta.webUrl` / `data.*.webUrl`) so users can inspect in Breyta web.\n- Only derive canonical recovery URLs when the needed ids are already known: billing, activate, draft-bindings, installation, or connection edit.\n- When blocked, include the exact recovery URL in `Runtime proof`, not just generic \"go to billing/setup\" text.",
		},
		{
			"- `breyta flows push --file ./tmp/flows/<slug>.clj`\n- `breyta flows configure <slug> ...` (when required)\n- `breyta flows configure check <slug>`\n- If the flow belongs to a bundle of dependent flows, re-check grouping after metadata changes with `breyta flows show <slug> --pretty`\n- Live target updates after slot changes: use `--target live --version <n|latest>` (and `--from-draft` when promoting draft bindings)\n- Optional read-only verification: `breyta flows validate <slug>`",
			"- `breyta flows push --file ./tmp/flows/<slug>.clj`\n- `breyta flows configure <slug> ...` (when required)\n- `breyta flows configure check <slug>`\n- Treat failed configure checks as a hard stop before draft/live runs unless the task is static validation only\n- If the flow belongs to a bundle of dependent flows, set explicit order with `breyta flows update <slug> --group-order <n>` and re-check ordered siblings with `breyta flows show <slug>` so `groupFlows` is visible\n- Live target updates after slot changes: use `--target live --version <n|latest>` (and `--from-draft` when promoting draft bindings)\n- Optional read-only verification: `breyta flows validate <slug>`\n- Inspect draft-vs-live changes before release: `breyta flows diff <slug>`",
		},
		{
			"- `breyta flows release <slug>`\n- `breyta flows promote <slug>`\n- `breyta flows installations configure <installation-id> --input '{...}'`",
			"- `breyta flows release <slug> --release-note-file ./release-note.md`\n- `breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md` (edit the note later)\n- `breyta flows promote <slug>`\n- `breyta flows installations configure <installation-id> --input '{...}'`",
		},
	}

	for _, pair := range replacements {
		updated = strings.ReplaceAll(updated, pair[0], pair[1])
	}
	if !strings.Contains(updated, "Do not put full report bodies in table cells such as `report_markdown`") {
		beforeLargeArtifactPatch := updated
		updated = strings.ReplaceAll(
			updated,
			"- Prefer CLI-returned URLs such as `data.flow.webUrl`, `data.run.webUrl`, `run.webUrl`, `outputWebUrl`, `data.webUrl`, or `meta.webUrl`.",
			"- Prefer CLI-returned URLs such as `data.flow.webUrl`, `data.run.webUrl`, `run.webUrl`, `outputWebUrl`, `data.webUrl`, or `meta.webUrl`.\n"+largeArtifactHygieneBullets,
		)
		if updated == beforeLargeArtifactPatch {
			updated = strings.ReplaceAll(updated, "## Output Guidance\n", "## Output Guidance\n\n"+largeArtifactHygieneBullets+"\n")
		}
		if updated == beforeLargeArtifactPatch {
			updated = strings.TrimRight(updated, "\n") + "\n\n## Large Artifact Hygiene\n\n" + largeArtifactHygieneBullets + "\n"
		}
	}
	if currentCanonicalSkill {
		if updated == original {
			return files
		}
		cloned := make(map[string][]byte, len(files))
		for k, v := range files {
			cloned[k] = v
		}
		cloned["SKILL.md"] = []byte(updated)
		return cloned
	}
	updated = ensureWorkflowPlanningSection(updated)
	updated = ensureReliabilitySection(updated)
	updated = ensureProvenanceSection(updated)
	updated = ensureDiscoverCardMediaSection(updated)
	updated = ensureFlowLifecycleSection(updated)
	updated = ensureNamingConventionsSection(updated)
	updated = ensureSolutionSurfacesSection(updated)
	updated = ensureTemplateDiscoverySection(updated)
	updated = ensureWorkflowQualityContractSection(updated)
	updated = ensurePublicFlowUIVerificationSection(updated)
	updated = ensureDocSearchPatternsSection(updated)
	updated = ensureProviderAPIFreshnessSection(updated)
	if updated == original {
		return files
	}

	cloned := make(map[string][]byte, len(files))
	for k, v := range files {
		cloned[k] = v
	}
	cloned["SKILL.md"] = []byte(updated)
	return cloned
}

const namingConventionsSection = `## Readability + Searchability Naming Conventions (Required)

Goal: operators should scan the flow in UI/CLI quickly, and search/grep by intent keywords should find it quickly.

- :name
  - format: <primary outcome> for <channel/system>
  - include high-intent keywords when relevant (for example invoice, approval, webhook, ai)
- :description
  - one sentence with trigger mode + core actions + final outcome
  - avoid vague wording like "process data"
- :tags
  - include 4-8 searchable nouns/adjectives tied to intent and channel
  - example set: ["invoice" "approval" "webhook" "billing" "llm"]
- trigger :label
  - make mode/source explicit: manual test vs webhook vs schedule cadence
  - examples: Manual smoke test (invoice approval), New invoice webhook, Fallback billing reconciliation (every 5 minutes)
- step id in flow/step
  - use kebab-case verb-object names that reveal action and object
  - prefer read-selected-email, send-reply-email over generic/internal names like step1, normalize, finalize
- step :title
  - add human-readable action text (sentence case, starts with verb)
  - avoid implementation jargon in labels (for example normalize, finalize, hydrate) when a plain outcome phrase works better
  - prefer plain process language, for example:
    - Prepare to watch for new emails
    - Pick an email to reply to
    - Summarize run result
- if metadata labels
  - use question-style :label with explicit action outcomes in :yes / :no
  - default to Should we ...? framing when possible
  - examples:
    - ^{:label "Should we send the reply now?" :yes "Send reply email" :no "Skip send and return skip reason"}
    - ^{:label "Should we process the webhook payload now?" :yes "Process payload and continue" :no "Skip processing and return skip reason"}
- pre-push scan check
  - from :name, :description, :triggers, and first few step ids/titles, a new operator should infer flow behavior in ~10 seconds
- CLI search clarity
  - breyta flows search <query> is for approved template discovery, not workspace flow lookup
  - for workspace lookup, use breyta flows workspace search <query> instead of broad flow lists
  - when flows are user-facing, ensure search tokens appear in :name, :description, and :tags (for example invoice, approval, webhook, billing)`

const solutionSurfacesSection = `## Solution surfaces first (Required before editing)

Goal: use Breyta's reusable definition surfaces first, then compose them in
:flow.

Default authoring order:

1. ` + "`:requires`" + `
- declare connections, secrets, installer inputs, run inputs, and worker dependencies first
- start with connection inventory:
  - ` + "`breyta connections list`" + `
  - ` + "`breyta connections show <id>`" + ` for the connection you plan to reuse
  - ` + "`breyta connections test <id>`" + ` only when binding or debugging that connection
- choose stable slot names around business capability (` + "`:github-api`" + `, ` + "`:crm`" + `, ` + "`:llm`" + `, ` + "`:slack`" + `) rather than transient provider names

2. ` + "`:templates`" + `
- move prompts, request bodies, SQL, notification content, and other large static text here

3. ` + "`:functions`" + `
- put shaping, normalization, preparation, and projection logic here

4. ` + "`:steps`" + `
- package heavy built-in step configs behind a smaller input/output contract
- prefer packaged step tools over exposing raw broad built-in surfaces to agents

5. ` + "`:agents`" + `
- define reusable named agent roles here when the behavior is agent-shaped
- put objective, instructions, memory, cost/evaluate/trace, and delegation config in the named agent definition

6. ` + "`:flow`" + `
- only after the reusable surfaces are in place, wire them together in deterministic orchestration

For agentic reviewer/fixer/coordinator flows, the default pattern should be:

- ` + "`:files`" + ` for code/resource state
- packaged ` + "`:steps`" + ` for heavy external or policy-shaped operations
- named ` + "`:agents`" + ` for reusable roles
- orchestration last

Anti-patterns:

- large raw prompts embedded directly in ` + "`:agent`" + `, ` + "`:llm`" + `, or ` + "`:http`" + ` steps
- repeated one-off shaping logic in ` + "`:flow`" + `
- exposing raw broad built-in step surfaces directly to agents when a packaged step would be safer
- treating ` + "`:flow`" + ` as the place to define reusable behavior`

const templateDiscoverySection = `## Primitive-first reuse for create/edit (Required)

Goal: avoid inventing flow structure from a name alone while keeping evidence small.

- pick a task mode before running commands: existing-flow edit, new flow, primitive/step edit, debug run, public publish/install, output/table, provider/API, or release
- for new flows:
  - search nearby workspace patterns first with ` + "`breyta flows workspace search \"<integration or problem query>\" --limit 5`" + `
  - inspect private primitive snippets with ` + "`breyta flows workspace examples step <type> \"<query>\" --limit 3`" + `
  - search docs snippets for the problem, primitive, and integration
  - search approved examples with ` + "`breyta flows search \"<problem or integration query>\" --limit 5`" + `
- for existing flows:
  - inspect the current flow first with ` + "`breyta flows show <slug>`" + ` or ` + "`breyta flows pull <slug>`" + `
  - search docs and approved examples before changing structure
  - compare the touched surface against the closest approved example before editing
- primitive-first ladder:
  - review metadata: name, description, tags, providers, step types, step count, publish description, ` + "`steps_text`" + `, and ` + "`flow_web_url`" + `
  - inspect matching private workspace snippets with ` + "`breyta flows workspace examples step <type> \"<query>\" --limit 3`" + ` when available
  - inspect matching primitive snippets with ` + "`breyta flows examples step <type> \"<query>\" --limit 3`" + ` when available
  - include only referenced ` + "`:requires`" + `, ` + "`:templates`" + `, and ` + "`:functions`" + `
  - inspect one full template only for cross-step architecture reuse, public install patterns, multi-flow orchestration, fanout/child-flow behavior, unclear snippet dependencies, or copying overall flow structure
- if no useful approved example exists, say so explicitly and continue from docs
- command budget:
  - authenticate once unless auth/workspace state changes
  - do not repeat identical commands unless state changed
  - use one docs search per changed primitive before opening docs pages
  - open full docs pages only after docs search identifies the narrow page needed
  - use one full template inspection at most for normal create/edit work
  - test only the connection you plan to bind or debug, not ` + "`breyta connections test --all`" + `
  - after two failed edit/run cycles, stop and re-plan
- final handoff must include approved example queries run, chosen/rejected snippets or templates, and what structure was reused or intentionally ignored`

const largeArtifactHygieneBullets = `- For large artifacts, keep chat and run summaries small: report resource refs, signed URLs, and short previews instead of pasting full table/resource content.
- ` + "`breyta resources read <uri>`" + ` is the normal bounded inspection path for agents. It returns compact table row and cell previews by default; use ` + "`--full`" + ` only when the whole payload is required.
- Treat ` + "`--pretty`" + ` as formatting only. It must not be used as a shortcut for full payload access.
- When authoring flows, persist long Markdown reports or JSON bodies as blobs/resources and store refs plus short summaries in tables. Do not put full report bodies in table cells such as ` + "`report_markdown`" + `.`

const workflowQualityContractSection = `## Workflow quality contract (Required)

Before editing, write the contract at the depth required by the task mode:

- inputs and outputs
- side effects and exactly-once requirements
- idempotency or dedupe strategy
- failure behavior, retries, and timeouts
- observability proof: result fields, counters, run ids, resources, or side-effect evidence
- user-facing output review path

For primitive/step edits, scope the contract to the changed primitive. For new
flows, release work, public installs, and cross-step changes, cover the full
flow.`

const publicFlowUIVerificationSection = `## Public/end-user UI verification (Required for public flows)

Goal: prove the installed end-user path, not only draft CLI execution.

- do not tell the user a public/end-user flow is "ready for UI" from draft proof alone
- verify live/install-shaped behavior or state ` + "`web UI not verified`" + ` in the risk ledger
- verify live target with ` + "`breyta flows show <slug> --target live`" + `
- smoke-run live target with ` + "`breyta flows run <slug> --target live --wait`" + ` when side effects are safe
- for installed flows, inspect installation setup/config and run with ` + "`breyta flows run <slug> --installation-id <installation-id> --wait`" + `
- when browser/UI access is available, test the actual setup page, run form fields, upload CSV or file flow, resource picker, and output page
- if CLI works but UI fails, or draft works but setup page, run form fields, installed flow, resource picker, or old live version fails, send ` + "`breyta feedback send`" + ` with full run/output URLs`

const workflowPlanningSection = `## Workflow architecture planning (Required before build)

Goal: design the flow architecture first so implementation follows a clear pattern and does not fragment across many trial versions.

- planning gate first
  - before editing or pushing, write a one-page architecture brief
  - keep quality checks in the loop, but do not replace planning with a hard quality gate
- required architecture brief fields
  - goal and success criteria (what outcome this flow must deliver)
  - trigger map (manual, webhook, schedule, event) and why each trigger exists
  - path map (primary success path, fallback path, stop path)
  - step contract table (step id, plain-language title, input, output, side effect)
  - branch contract table for each if (question label, yes action, no action, decision signal)
  - failure policy (retry, fallback, fail-fast rules)
  - idempotency and duplicate protection strategy
  - observability plan (what run output proves success, what labels make triage fast)
- pattern-first design
  - choose the dominant pattern up front: webhook processing, HTTP API integration, database operations, AI agent workflow, or scheduled task
  - if multiple patterns are needed, split by clear subpaths and name each subpath explicitly in steps/titles
  - enforce deterministic progression: trigger -> intake -> decide -> act -> summarize -> post-process
  - define edge cases before build (no items, partial data, external API failure, already-processed input)
- common multi-path default
  - path one: receive or fetch the work item
  - path two: validate, decide, and perform the side effect
  - keep shared preparation and final summary explicit, not hidden in generic finalize/normalize labels
- anti-patterns to avoid
  - technical/internal step names that hide behavior
  - branch labels that do not read like operator decisions
  - adding new versions to discover architecture instead of deciding architecture before edits`

const reliabilitySection = `## Reliability + determinism planning (Required before push)

Goal: make side effects safe, runs replayable, and runtime behavior predictable before draft iteration turns into many versions.

- required before push
  - define every side effect and what must happen exactly once
  - name the idempotency or duplicate-protection key for each side-effectful path
  - classify every external call as retryable vs fail-fast
  - set timeout expectations for each external boundary
  - choose concurrency intentionally (` + "`sequential`" + `, ` + "`fanout`" + `, ` + "`keyed`" + `) and state why
  - for each concurrent path, define the protected resource or side effect (API, cursor, blob store, destination record, installation)
  - define payload strategy: inline small data, persist large artifacts, pass refs for large blobs
  - define cursor/checkpoint behavior for partial failure and replay
  - define exact runtime proof: result fields, counters, child runs, or resources that prove success
- concurrency defaults (be explicit)
  - use ` + "`sequential`" + ` when order matters, when mutating shared state, when moving large artifacts, or when external systems are slow/fragile
  - use ` + "`fanout`" + ` only for independent, bounded, side-effect-safe items with explicit timeout awareness
  - use ` + "`keyed`" + ` when work must serialize per entity while allowing cross-entity parallelism
  - if concurrency is not clearly beneficial, default to ` + "`sequential`" + `
- concurrency questions to answer before build
  - what can run in parallel safely?
  - what must never overlap?
  - what is the per-item timeout and what happens when one item stalls?
  - how are partial successes summarized without losing failed items?
- scale-aware defaults
  - use sequential handling for large file transfer unless there is explicit evidence fanout is safe
  - prefer child flows to isolate heavyweight artifact creation and handoff
  - pass signed URLs/blob refs for large artifacts instead of moving large bodies through many steps
- failure-safety defaults
  - retries only for transient failures with bounded attempts/backoff
  - never advance cursors/checkpoints past failed work
  - resume/replay behavior should be explicit for partial success paths
- anti-patterns to avoid
  - using ` + "`:fanout`" + ` for large file transfer without child-timeout awareness
  - using concurrency before defining what shared state or side effect it can corrupt
  - relying on step result shapes that are not guaranteed inside the same flow
  - hiding side effects behind vague step names like ` + "`finalize`" + ` or ` + "`process`" + `
  - discovering reliability limits by repeatedly pushing new versions
- required pre-release verification
  - run a happy path
  - run a no-item / no-op path when applicable
  - exercise at least one partial failure or retry path when feasible
  - replay or rerun once to verify idempotency/duplicate protection
  - for concurrent paths, prove the chosen mode with evidence: counts, failures, child runs, and no skipped/reprocessed items
  - after release, capture live smoke proof when the side effects are safe`

const provenanceSection = `## Provenance for derived flows (Required when reusing existing flows)

Goal: preserve clear lineage to source flows without changing the meaning of ` + "`created-by`" + `.

- keep ` + "`created-by`" + ` as the creator of the current flow record
- when the new flow is based on one or more existing flows, store those refs as provenance metadata
- search hits alone do not count as provenance; only flows actually opened with ` + "`breyta flows show`" + ` or ` + "`breyta flows pull`" + ` should become candidates
- after creating or updating a derived flow, persist curated provenance with:
  - ` + "`breyta flows provenance set <slug> --from-consulted`" + `
  - ` + "`breyta flows provenance set <slug> --source <workspace-id>/<flow-slug>`" + `
  - ` + "`breyta flows provenance set <slug> --template <template-slug>`" + `
- only clear provenance intentionally with ` + "`breyta flows provenance set <slug> --clear`" + `
- when several source flows were consulted, keep only the flows that actually mattered to the final implementation`

const discoverCardMediaSection = `## Discover card media (Public discover polish)

Goal: make public discover/install cards show creator-curated media instead of only connection icons.

- set discover card media with:
  - ` + "`breyta flows update <slug> --publish-media-type image --publish-media-source-kind https-url --publish-media-source https://...`" + `
- video media can also include an optional poster:
  - ` + "`breyta flows update <slug> --publish-media-type video --publish-media-source-kind https-url --publish-media-source https://... --publish-media-poster-kind https-url --publish-media-poster https://...`" + `
- clear discover card media intentionally with:
  - ` + "`breyta flows update <slug> --clear-publish-media`" + `
- if you keep the flow in source control, you can also author the same value in the flow file as ` + "`:publish-media`" + ` and push it
- use alt text that explains the visible result, not the implementation detail`

const flowLifecycleSection = `## Flow lifecycle cleanup (Public CLI surface)

Goal: use the public lifecycle commands intentionally when a flow should stop being used or be removed entirely.

- archive keeps the flow record and versions but removes it from the normal active surface:
  - ` + "`breyta flows archive <slug>`" + `
- delete is permanent removal of the flow definition:
  - ` + "`breyta flows delete <slug> --yes`" + `
- use force delete only when you intentionally want the backend to cancel runs and remove installations as part of cleanup:
  - ` + "`breyta flows delete <slug> --yes --force`" + `
- for large cleanup jobs with many runs or resources, raise the request timeout:
  - ` + "`breyta flows delete <slug> --yes --force --timeout 5m`" + `
- prefer archive when you want to retire a flow safely and preserve history for inspection
- prefer delete only for disposable or fully decommissioned flows`

const docSearchPatternsSection = `## Doc lookup patterns (Required before guessing implementation details)

Goal: search docs in more than one way before inferring behavior, flags, or
runtime semantics.

- start with the primitive or surface name
  - ` + "`breyta docs find \"files materialize\"`" + `
  - ` + "`breyta docs find \"packaged steps\"`" + `
- search exact phrases when you know the likely wording
  - ` + "`breyta docs find \"\\\"draft bindings\\\"\"`" + `
  - ` + "`breyta docs find \"\\\"tool permissions\\\"\"`" + `
- search command paths for operator behavior
  - ` + "`breyta docs find \"source:cli flows configure check\"`" + `
  - ` + "`breyta docs find \"source:cli connections test\"`" + `
- search API/runtime docs when CLI docs are too thin
  - ` + "`breyta docs find \"source:flows-api agent definitions\"`" + `
  - ` + "`breyta docs find \"source:flows-api form requirements\"`" + `
- search error text when debugging
  - ` + "`breyta docs find \"\\\"Bad credentials\\\"\"`" + `
  - ` + "`breyta docs find \"\\\"missing api base url\\\"\"`" + `
- then open only the best narrow hit with ` + "`breyta docs show <slug>`" + `
- if the question is command shape or flags, finish with ` + "`breyta help <command...>`" + ` instead of guessing

Escalation path when the first search misses:

1. primitive name
2. command path
3. exact phrase
4. source filter
5. error text
6. only then infer a fallback`

const providerAPIFreshnessSection = `## Provider/API Freshness And Model Selection

Goal: avoid stale endpoints, request shapes, auth assumptions, rate limits, and model ids.

- do not rely on model training data for current API shape, provider behavior, or model availability
- before adding or changing any external API integration, check current official provider docs/API references plus relevant Breyta docs/examples
- apply this to OpenAI, Anthropic/Claude, Google/Gemini, OpenAI-compatible providers, HTTP APIs, databases, and any vendor a flow calls
- before adding or changing model ids, check the provider's current model docs or model-list API when credentials/tooling are available
- verify the exact model id with a draft run or isolated step run before release when feasible
- for OpenAI-backed steps, use ` + "`gpt-5.4`" + ` as Breyta's current API default where a default is needed, but still verify availability in the target environment
- do not claim or use unreleased provider models, such as ` + "`gpt-5.5`" + `, without provider/API proof
- preserve explicit user requests, such as ` + "`gpt-5.4`" + ` or a specific Claude/Gemini model, unless current provider docs/API availability show the model is unavailable or unsuitable
- when editing existing flows, keep legacy models/APIs only if compatibility, cost, or evaluation history is intentional. Otherwise propose upgrading to the current verified provider/API choice`

func ensureNamingConventionsSection(body string) string {
	if h2LineStartOutsideFences(body, "## Readability + Searchability Naming Conventions (Required)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + namingConventionsSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + namingConventionsSection + "\n"
}

func ensureSolutionSurfacesSection(body string) string {
	if h2LineStartOutsideFences(body, "## Solution surfaces first (Required before editing)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + solutionSurfacesSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + solutionSurfacesSection + "\n"
}

func ensureTemplateDiscoverySection(body string) string {
	if h2LineStartOutsideFences(body, "## Primitive-first reuse for create/edit (Required)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Template discovery for create/edit (Required)"); headingPos >= 0 {
		nextPos := nextH2LineStartOutsideFences(body, headingPos+len("## Template discovery for create/edit (Required)"))
		if nextPos >= 0 {
			return body[:headingPos] + templateDiscoverySection + "\n\n" + body[nextPos:]
		}
		return strings.TrimRight(body[:headingPos], "\n") + "\n\n" + templateDiscoverySection + "\n"
	}
	if headingPos := h2LineStartOutsideFences(body, "## Solution surfaces first (Required before editing)"); headingPos >= 0 {
		insertPos := nextH2LineStartOutsideFences(body, headingPos+len("## Solution surfaces first (Required before editing)"))
		if insertPos >= 0 {
			return body[:insertPos] + templateDiscoverySection + "\n\n" + body[insertPos:]
		}
		return strings.TrimRight(body, "\n") + "\n\n" + templateDiscoverySection + "\n"
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + templateDiscoverySection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + templateDiscoverySection + "\n"
}

func ensurePublicFlowUIVerificationSection(body string) string {
	if h2LineStartOutsideFences(body, "## Public/end-user UI verification (Required for public flows)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Output Guidance"); headingPos >= 0 {
		return body[:headingPos] + publicFlowUIVerificationSection + "\n\n" + body[headingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + publicFlowUIVerificationSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + publicFlowUIVerificationSection + "\n"
}

func ensureWorkflowQualityContractSection(body string) string {
	if h2LineStartOutsideFences(body, "## Workflow quality contract (Required)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Public/end-user UI verification (Required for public flows)"); headingPos >= 0 {
		return body[:headingPos] + workflowQualityContractSection + "\n\n" + body[headingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + workflowQualityContractSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + workflowQualityContractSection + "\n"
}

func ensureDiscoverCardMediaSection(body string) string {
	if h2LineStartOutsideFences(body, "## Discover card media (Public discover polish)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Flow lifecycle cleanup (Public CLI surface)"); headingPos >= 0 {
		return body[:headingPos] + discoverCardMediaSection + "\n\n" + body[headingPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n" + discoverCardMediaSection + "\n"
}

func ensureWorkflowPlanningSection(body string) string {
	if h2LineStartOutsideFences(body, "## Workflow architecture planning (Required before build)") >= 0 {
		return body
	}
	namingPos := h2LineStartOutsideFences(body, "## Readability + Searchability Naming Conventions (Required)")
	if namingPos >= 0 {
		return body[:namingPos] + workflowPlanningSection + "\n\n" + body[namingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + workflowPlanningSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + workflowPlanningSection + "\n"
}

func ensureReliabilitySection(body string) string {
	if h2LineStartOutsideFences(body, "## Reliability + determinism planning (Required before push)") >= 0 {
		return body
	}
	namingPos := h2LineStartOutsideFences(body, "## Readability + Searchability Naming Conventions (Required)")
	if namingPos >= 0 {
		return body[:namingPos] + reliabilitySection + "\n\n" + body[namingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + reliabilitySection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + reliabilitySection + "\n"
}

func ensureProvenanceSection(body string) string {
	if h2LineStartOutsideFences(body, "## Provenance for derived flows (Required when reusing existing flows)") >= 0 {
		return body
	}
	namingPos := h2LineStartOutsideFences(body, "## Readability + Searchability Naming Conventions (Required)")
	if namingPos >= 0 {
		return body[:namingPos] + provenanceSection + "\n\n" + body[namingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + provenanceSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + provenanceSection + "\n"
}

func ensureFlowLifecycleSection(body string) string {
	if h2LineStartOutsideFences(body, "## Flow lifecycle cleanup (Public CLI surface)") >= 0 {
		return body
	}
	namingPos := h2LineStartOutsideFences(body, "## Readability + Searchability Naming Conventions (Required)")
	if namingPos >= 0 {
		return body[:namingPos] + flowLifecycleSection + "\n\n" + body[namingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + flowLifecycleSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + flowLifecycleSection + "\n"
}

func ensureDocSearchPatternsSection(body string) string {
	if h2LineStartOutsideFences(body, "## Doc lookup patterns (Required before guessing implementation details)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Cross-Docs Search (Secondary)"); headingPos >= 0 {
		return body[:headingPos] + docSearchPatternsSection + "\n\n" + body[headingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Preflight"); headingPos >= 0 {
		return body[:headingPos] + docSearchPatternsSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + docSearchPatternsSection + "\n"
}

func ensureProviderAPIFreshnessSection(body string) string {
	if h2LineStartOutsideFences(body, "## Provider/API Freshness And Model Selection") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Model Selection"); headingPos >= 0 {
		return body[:headingPos] + providerAPIFreshnessSection + "\n\n" + body[headingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Output Guidance"); headingPos >= 0 {
		return body[:headingPos] + providerAPIFreshnessSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + providerAPIFreshnessSection + "\n"
}

func h2LineStartOutsideFences(body, heading string) int {
	inFence := false
	openFence := markdownFence{}
	offset := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		lineNoEOL := strings.TrimRight(line, "\r\n")
		if marker, ok := markdownFenceMarker(lineNoEOL); ok {
			if !inFence {
				inFence = true
				openFence = marker
			} else if marker.char == openFence.char && marker.length >= openFence.length && marker.validCloser {
				inFence = false
				openFence = markdownFence{}
			}
			offset += len(line)
			continue
		}

		if !inFence && strings.TrimSpace(lineNoEOL) == heading {
			return offset
		}
		offset += len(line)
	}
	return -1
}

func nextH2LineStartOutsideFences(body string, start int) int {
	inFence := false
	openFence := markdownFence{}
	offset := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		lineNoEOL := strings.TrimRight(line, "\r\n")
		if marker, ok := markdownFenceMarker(lineNoEOL); ok {
			if !inFence {
				inFence = true
				openFence = marker
			} else if marker.char == openFence.char && marker.length >= openFence.length && marker.validCloser {
				inFence = false
				openFence = markdownFence{}
			}
			offset += len(line)
			continue
		}

		trimmed := strings.TrimSpace(lineNoEOL)
		if offset >= start && !inFence && strings.HasPrefix(trimmed, "## ") {
			return offset
		}
		offset += len(line)
	}
	return -1
}

type markdownFence struct {
	char        byte
	length      int
	validCloser bool
}

func markdownFenceMarker(line string) (markdownFence, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return markdownFence{}, false
	}
	markerChar := trimmed[0]
	if markerChar != '`' && markerChar != '~' {
		return markdownFence{}, false
	}

	count := 0
	for count < len(trimmed) && trimmed[count] == markerChar {
		count++
	}
	if count < 3 {
		return markdownFence{}, false
	}

	remainder := strings.TrimLeft(trimmed[count:], " \t")
	return markdownFence{
		char:        markerChar,
		length:      count,
		validCloser: remainder == "",
	}, true
}
