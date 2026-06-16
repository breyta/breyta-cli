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

	currentPlaybookRouterSkill := strings.Contains(updated, "## Playbook Matrix") &&
		strings.Contains(updated, "playbooks/author-flows.md") &&
		strings.Contains(updated, "playbooks/debug-and-verify.md") &&
		strings.Contains(updated, "references/runtime-data-shapes.md") &&
		strings.Contains(updated, "## Default Command Budget")
	if currentPlaybookRouterSkill {
		return applyEfficientWorkflowGuidanceOverrides(files)
	}

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
			"- For new or changed external API, LLM provider, or model config, avoid stale training-data defaults. Check current official provider docs/API references and relevant Breyta docs/examples before choosing endpoints, request shape, auth, rate-limit assumptions, or model ids. For OpenAI-backed steps, use `gpt-5.4` as Breyta's current API default where a default is needed, but still verify availability.",
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
			"- Do not claim or use a provider model unless current provider docs/API availability prove it exists in the target environment.",
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
			"- Search approved templates before creating a new flow, changing architecture, using unfamiliar integrations, or when primitive examples are missing: `breyta flows templates search \"<problem or integration query>\" --limit 5`.",
		},
		{
			"- Use `breyta flows search \"<query>\" --limit 1 --full --pretty` for the best one or two matches when structure matters.",
			"- For primitive/step edits, use matching primitive snippets and referenced dependencies when available. Inspect a full template only for cross-step architecture reuse, public install patterns, multi-flow orchestration, fanout/child-flow behavior, unclear snippet dependencies, or copying overall flow structure.",
		},
		{
			"`current state -> docs search -> template search -> full template inspect -> compare -> edit -> draft proof -> live/install proof`.",
			"`current state -> workspace search/grep -> private snippet -> docs snippets -> approved template metadata -> approved primitive snippet -> referenced dependencies -> full template only if needed -> compare -> edit -> draft proof -> live/install proof`.",
		},
		{
			"- Treat flow grouping as mutable metadata, not authored source. Verify grouping with `breyta flows list --pretty` or `breyta flows show <slug> --pretty`.",
			"- Treat flow grouping as mutable metadata, not authored source. Verify grouping with `breyta flows list --limit 50` or `breyta flows show <slug>` so `groupFlows` and ordering metadata are visible. Use `--full` only when definition fields are needed.",
		},
		{
			"- Before creating a new flow, search existing definitions: `breyta flows search <query>`.",
			"- Before creating or editing a flow, pick a task mode, inspect current state, search nearby workspace patterns with `breyta flows search \"<integration or problem query>\" --limit 5`, use `breyta flows grep \"<literal>\" --or \"<variant>\" --limit 5` for source/config search, search docs snippets for touched primitives, search approved templates with `breyta flows templates search \"<problem or integration query>\" --limit 5`, and search existing data with `breyta resources search \"<query>\" --limit 5`.\n- Add `--keyword-mode balanced` to `breyta resources search` for natural-language questions over small resource sets, then read only the selected URI with `breyta resources read <resource-uri> --limit 5`.\n- Use private/approved primitive snippets and referenced `:requires`, `:templates`, and `:functions` before pulling full templates.\n- Inspect a full template only for cross-step architecture reuse, public install patterns, multi-flow orchestration, fanout/child-flow behavior, unclear snippet dependencies, or copying overall flow structure.\n- For new flows, use workspace search/grep instead of `breyta flows list --limit 50` for pattern discovery.\n- For existing flows, inspect the target summary with `breyta flows show <slug>` or pull editable source with `breyta flows pull <slug>` before editing.",
		},
		{
			"3. Confirm reusable resources:\n   - `breyta connections list`\n   - `breyta flows search <query>`",
			"3. Confirm reusable resources:\n   - `breyta connections list`\n   - `breyta connections show <id>` for the connection you expect to bind\n   - `breyta connections test <id>` only when binding or debugging that connection\n   - Nearby workspace patterns: `breyta flows search \"<integration or problem query>\" --limit 5`\n   - Workspace source/config search: `breyta flows grep \"<literal>\" --or \"<variant>\" --limit 5`\n   - Private primitive snippets: `breyta flows workspace examples step <type> \"<query>\" --limit 3`\n   - Docs snippets: `breyta docs find \"<problem or primitive>\" --limit 5 --format json`\n   - Approved template discovery: `breyta flows templates search \"<problem or integration query>\" --limit 5`\n   - Existing data/resources: `breyta resources search \"<query>\" --limit 5`; add `--keyword-mode balanced` for natural-language questions over small resource sets\n   - Primitive-first reuse: inspect matching snippets and referenced dependencies before full templates\n   - Existing workspace flow: `breyta flows show <slug>` for compact summary or `breyta flows pull <slug>` for editable source",
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
			"4. Runtime mismatch: inspect one step with `breyta runs inspect <workflow-id> --step <step-id>`, isolate the primitive, rerun the intended interface. For waits, approve deliberately with `breyta runs continue <workflow-id> --approve-latest-wait`.",
			"4. Runtime mismatch: start with `breyta runs events <workflow-id> --limit 100`; add `--step <step-id>` for one step or `--installation-id <id>` for an installed-profile run. Inspect one step with `breyta runs step <workflow-id> <step-id>` or `breyta runs inspect <workflow-id> --step <step-id>`, using `--full` only when captured output/error payloads are required. Isolate the primitive, then rerun the intended interface. For waits, approve deliberately with `breyta runs continue <workflow-id> --approve-latest-wait`.",
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
	updated = ensureN8NImportGuidance(updated)
	updated = ensurePaidAppMarketplaceSection(updated)
	updated = ensureFocusedStepRunGuidance(updated)
	updated = ensureInputFilePayloadGuidance(updated)
	updated = ensureLintBeforePushGuidance(updated)
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
	updated = ensurePaidAppMarketplaceSection(updated)
	updated = ensureDiscoverCardMediaSection(updated)
	updated = ensureFlowLifecycleSection(updated)
	updated = ensureNamingConventionsSection(updated)
	updated = ensureSolutionSurfacesSection(updated)
	updated = ensureTemplateDiscoverySection(updated)
	updated = ensureWorkflowQualityContractSection(updated)
	updated = ensurePublicFlowUIVerificationSection(updated)
	updated = ensureDocSearchPatternsSection(updated)
	updated = ensureProviderAPIFreshnessSection(updated)
	updated = ensureN8NImportGuidance(updated)
	updated = ensureFocusedStepRunGuidance(updated)
	updated = ensureInputFilePayloadGuidance(updated)
	updated = ensureLintBeforePushGuidance(updated)
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

func applyEfficientWorkflowGuidanceOverrides(files map[string][]byte) map[string][]byte {
	working := files
	changed := false
	updateFile := func(name string, fn func(string) string) {
		raw, ok := working[name]
		if !ok || len(raw) == 0 {
			return
		}
		body := string(raw)
		next := fn(body)
		if next == body {
			return
		}
		if !changed {
			cloned := make(map[string][]byte, len(files))
			for k, v := range files {
				cloned[k] = v
			}
			working = cloned
			changed = true
		}
		working[name] = []byte(next)
	}

	updateFile("SKILL.md", ensureMinimumSufficientEvidenceCoreRule)
	updateFile("SKILL.md", ensureFocusedStepRunGuidance)
	updateFile("SKILL.md", ensureInputFilePayloadGuidance)
	updateFile("SKILL.md", ensureAuthoringDefaultsContractMatrix)
	updateFile("playbooks/author-flows.md", ensureAuthorFlowEfficientLoop)
	updateFile("playbooks/author-flows.md", ensureFocusedStepRunGuidance)
	updateFile("playbooks/author-flows.md", ensureInputFilePayloadGuidance)
	updateFile("playbooks/author-flows.md", ensureLintBeforePushGuidance)
	updateFile("playbooks/debug-and-verify.md", ensureDebugAcceptanceCaseGuidance)
	updateFile("references/outputs-and-tables.md", ensureOutputHandoffContract)
	updateFile("references/public-flows.md", ensurePublicFlowReuseDuringAuthoring)
	return working
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
  - breyta flows search "<query>" --limit 5 searches actual workspace flow metadata
  - breyta flows grep "<literal>" --limit 5 searches actual workspace flow source/config
  - breyta flows templates search/grep with --limit 5 searches approved reusable templates
  - use broad flow lists for inventory, slug checks, or explicit user requests only
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
  - search nearby workspace patterns first with ` + "`breyta flows search \"<integration or problem query>\" --limit 5`" + `
  - search workspace source/config literals with ` + "`breyta flows grep \"<literal>\" --or \"<variant>\" --limit 5`" + `
  - inspect private primitive snippets with ` + "`breyta flows workspace examples step <type> \"<query>\" --limit 3`" + `
  - search docs snippets for the problem, primitive, and integration
  - search approved templates with ` + "`breyta flows templates search \"<problem or integration query>\" --limit 5`" + `
  - if an approved template closely matches the requested outcome, duplicate it first with ` + "`breyta flows templates duplicate <template-slug>`" + `, prove one green draft, then make narrow edits
- for existing flows:
  - inspect the current flow first with ` + "`breyta flows show <slug>`" + ` or ` + "`breyta flows pull <slug>`" + `
  - search docs and approved templates before changing structure
  - compare the touched surface against the closest local or approved template example before editing
- for resource/data reuse:
  - search existing resources with ` + "`breyta resources search \"<query>\" --limit 5`" + `; add ` + "`--keyword-mode balanced`" + ` for natural-language questions over small resource sets
  - read only the selected URI with ` + "`breyta resources read <resource-uri> --limit 5`" + `
- primitive-first ladder:
  - review metadata: name, description, tags, providers, tool names, connection slots, step types, step count, compact publish/steps previews, and ` + "`flow_web_url`" + `
  - inspect matching private workspace snippets with ` + "`breyta flows workspace examples step <type> \"<query>\" --limit 3`" + ` when available
  - inspect matching primitive snippets with ` + "`breyta flows examples step <type> \"<query>\" --limit 3`" + ` when available
  - include only referenced ` + "`:requires`" + `, ` + "`:templates`" + `, and ` + "`:functions`" + `
  - when copying overall flow structure, prefer ` + "`breyta flows templates duplicate <template-slug>`" + ` over raw-definition copy/paste
  - inspect one full template only for cross-step architecture reuse, public install patterns, multi-flow orchestration, fanout/child-flow behavior, unclear snippet dependencies, or copying overall flow structure
- if no useful approved template exists, say so explicitly and continue from docs
- command budget:
  - authenticate once unless auth/workspace state changes
  - do not repeat identical commands unless state changed
  - use one docs search per changed primitive before opening docs pages
  - open full docs pages only after docs search identifies the narrow page needed
  - use one full template inspection at most for normal create/edit work
  - test only the connection you plan to bind or debug, not ` + "`breyta connections test --all`" + `
  - after two failed edit/run cycles, stop and re-plan
- final handoff must include workspace/template queries run, chosen/rejected snippets or templates, and what structure was reused or intentionally ignored`

const largeArtifactHygieneBullets = `- For large artifacts, keep chat and run summaries small: report resource refs, signed URLs, and short previews instead of pasting full table/resource content.
- ` + "`breyta resources read <uri>`" + ` is the normal bounded inspection path for agents. It returns compact blob previews and table row/cell previews by default; use ` + "`--full`" + ` only when the whole payload is required.
- Treat ` + "`--pretty`" + ` as formatting only. It must not be used as a shortcut for full payload access.
- When authoring flows, persist long Markdown reports or JSON bodies as blobs/resources and store refs plus short summaries in tables. Do not put full report bodies in table cells such as ` + "`report_markdown`" + `.
- For blob persists, choose the tier before authoring the step: retained/default for durable or user-visible artifacts, and ` + "`:persist {:type :blob :tier :ephemeral}`" + ` on streaming HTTP steps for temporary downloads, exports, generated media, and API response blobs that should use the more generous transient quota.`

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
- do not stop at activation; ` + "`/activate`" + ` and configure/check prove owner setup, not end-user installability
- for installable/public flows, verify Discover install plus an installed run when install behavior matters
- installable checklist: explicit author approval, Discover visibility, pushed/diffed/released/promoted live version, owner setup proof when required, Discover install dialog or installation create/configure/enable proof, installer-owned binding proof, installed run, output review
- verify live/install-shaped behavior or state ` + "`web UI not verified`" + ` in the risk ledger
- verify live target with ` + "`breyta flows show <slug> --target live`" + `
- smoke-run live target with ` + "`breyta flows run <slug> --target live --wait`" + ` when side effects are safe
- for installed flows, inspect installation setup/config and run with ` + "`breyta flows run <slug> --installation-id <installation-id> --wait`" + `
- for Buyer Test Mode author smoke tests, run from the paired Buyer Test workspace: create the source install with ` + "`breyta flows installations create <slug> --buyer-test-source-install --source-workspace-id <source-workspace-id> --source-flow-slug <slug>`" + ` and run it with ` + "`breyta flows run <slug> --buyer-test --installation-id <installation-id> --wait`" + `
- when browser/UI access is available, test the actual Discover install dialog, setup page, run form fields, upload CSV or file flow, resource picker, and output page
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
  - ` + "`breyta flows update <slug> --publish-media-type image --publish-media-source-file ./screenshot.png`" + `
- video media can also include an optional poster:
  - ` + "`breyta flows update <slug> --publish-media-type video --publish-media-source-kind https-url --publish-media-source https://... --publish-media-poster-kind https-url --publish-media-poster https://...`" + `
- clear discover card media intentionally with:
  - ` + "`breyta flows update <slug> --clear-publish-media`" + `
- if you keep the flow in source control, you can also author the same value in the flow file as ` + "`:publish-media`" + ` and push it
- HTTPS media sources must be publicly reachable safe media URLs; public Discover cards copy them into Breyta-owned assets/CDN and reject private hosts, unsafe redirects, or oversized responses
- use alt text that explains the visible result, not the implementation detail`

const paidAppMarketplaceSection = `## Paid app marketplace authoring (Source-authored)

Goal: create paid public apps with explicit app and plan identity while using
the CLI for source push, release, visibility, and verification.

- paid app pricing is authored in flow source and pushed with:
  - ` + "`breyta flows push --file ./flows/<slug>.clj`" + `
- CLI metadata commands manage tags, discover visibility, marketplace visibility, publish description, and card media; they do not set pricing flags
- prefer new paid apps to use app-owned catalogs:
  - ` + "`:marketplace {:visible true :app {:app-id \"...\" :app-primary-flow-slug \"...\" :app-flow-slugs [...] :monetization {:plans [...]}}}`" + `
- supported plan price types are:
  - ` + "`free`" + `
  - ` + "`one-time`" + `
  - ` + "`subscription`" + `
  - ` + "`usage`" + `
  - ` + "`subscription-usage`" + `
- subscription intervals are ` + "`month`" + ` or ` + "`year`" + ` on the publish path
- usage-priced plans are run-based only: use ` + "`:unit \"run\"`" + ` and ` + "`:included-quantity`" + `
- plan ids must be stable; keep exactly one intended default plan, otherwise the first plan is treated as default
- use legacy flow-level monetization only when preserving an existing legacy priced listing
- seat-based pricing is not implemented; do not describe a plan as N seats or N installs unless explicit seat entitlements exist
- paid app verification must cover checkout or trial entry, install handoff, installed run behavior, billing state, and exhausted/remediation state when relevant`

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
  - ` + "`breyta docs find \"source:cli flows lint\"`" + `
  - ` + "`breyta docs find \"source:cli flows configure check\"`" + `
  - ` + "`breyta docs find \"source:cli connections test\"`" + `
- search API/runtime docs when CLI docs are too thin
  - ` + "`breyta docs find \"source:flows-api agent definitions\"`" + `
  - ` + "`breyta docs find \"source:flows-api form requirements\"`" + `
- search error text when debugging
  - ` + "`breyta docs find \"\\\"Bad credentials\\\"\"`" + `
  - ` + "`breyta docs find \"\\\"missing api base url\\\"\"`" + `
- then open only the best narrow hit with ` + "`breyta docs show <slug>`" + `; default docs output is compact, and ` + "`--full`" + ` is the full-page escape hatch
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
- do not claim or use a provider model unless current provider docs/API availability prove it exists in the target environment
- for OpenAI-backed ` + "`:llm`" + ` and ` + "`:agent`" + ` steps, use an ` + "`:http-api`" + ` requirement, backend ` + "`openai`" + `, base URL ` + "`https://api.openai.com/v1`" + `, API-key auth, and a non-null config map; use installer ownership when every installer brings their own OpenAI key
- preserve explicit user requests, such as ` + "`gpt-5.4`" + ` or a specific Claude/Gemini model, unless current provider docs/API availability show the model is unavailable or unsuitable
- when editing existing flows, keep legacy models/APIs only if compatibility, cost, or evaluation history is intentional. Otherwise propose upgrading to the current verified provider/API choice`

const n8nImportGuidanceLine = "- For n8n workflow JSON imports, use `breyta flows import n8n <workflow.json>` first; do not hand-write the initial EDN conversion unless the importer is unavailable or explicitly bypassed."

const focusedStepRunProofBullet = "- When provider/model or primitive changes can be proven without downstream side effects, use `breyta flows run-step <slug> <step-id> --target live --input '{...}' --wait` to run only the named existing step with configured bindings before a full-flow proof."

const inputFilePayloadGuidance = "- Use `breyta flows run <slug> --input-file ./input.json` or `breyta flows run-step <slug> <step-id> --input-file ./input.json` instead of inline `--input '{...}'` when per-run payloads may hit shell or OS argument limits."

const lintBeforePushGuidance = "- Run `breyta flows lint --file ./flows/<slug>.clj` before push; use `--local-only` for offline checks, `--server` when canonical pre-push checks matter, and `--timeout <duration>` when server lint needs a longer bound"

func hasInputFilePayloadGuidance(body string) bool {
	return strings.Contains(body, "--input-file ./input.json") &&
		strings.Contains(body, "shell or OS argument limits")
}

func hasLintTimeoutGuidance(body string) bool {
	search := body
	for {
		idx := strings.Index(search, "flows lint")
		if idx < 0 {
			return false
		}
		window := search[idx:]
		if len(window) > 500 {
			window = window[:500]
		}
		if strings.Contains(window, "--timeout <duration>") {
			return true
		}
		search = search[idx+len("flows lint"):]
	}
}

func ensureMinimumSufficientEvidenceCoreRule(body string) string {
	if strings.Contains(body, "Use minimum sufficient evidence") {
		return body
	}
	section := strings.Join([]string{
		"## Core Rule",
		"",
		"Use minimum sufficient evidence. Every docs read, template search, command,",
		"patch, run, and artifact inspection should answer a specific contract,",
		"implementation, or verification question. Prefer current Breyta surfaces before",
		"building from scratch: workspace flows, approved templates, public/installable",
		"flows with installed callable interfaces, exact field docs, focused step runs,",
		"and resource/table readback.",
	}, "\n")
	if headingPos := h2LineStartOutsideFences(body, "## Flow DSL Mental Model"); headingPos >= 0 {
		return body[:headingPos] + section + "\n\n" + body[headingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Start Of Session"); headingPos >= 0 {
		return body[:headingPos] + section + "\n\n" + body[headingPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n" + section + "\n"
}

func ensureFocusedStepRunGuidance(body string) string {
	if strings.Contains(body, "breyta flows run-step <slug> <step-id>") {
		return body
	}
	guidance := focusedStepRunProofBullet
	if headingPos := h2LineStartOutsideFences(body, "## Default Loop"); headingPos >= 0 {
		insertPos := headingPos + len("## Default Loop")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + guidance + "\n\n" + body[insertPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Create/Edit Preflight"); headingPos >= 0 {
		insertPos := headingPos + len("## Create/Edit Preflight")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + guidance + "\n" + body[insertPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Core Rule"); headingPos >= 0 {
		insertPos := headingPos + len("## Core Rule")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + guidance + "\n" + body[insertPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n## Focused step proof\n\n" + guidance + "\n"
}

func ensureInputFilePayloadGuidance(body string) string {
	if hasInputFilePayloadGuidance(body) {
		return body
	}
	if proofPos := strings.Index(body, "breyta flows run-step <slug> <step-id>"); proofPos >= 0 {
		if eol := strings.Index(body[proofPos:], "\n"); eol >= 0 {
			insertPos := proofPos + eol + 1
			return body[:insertPos] + inputFilePayloadGuidance + "\n" + body[insertPos:]
		}
		return strings.TrimRight(body, "\n") + "\n" + inputFilePayloadGuidance + "\n"
	}
	if headingPos := h2LineStartOutsideFences(body, "## Default Loop"); headingPos >= 0 {
		insertPos := headingPos + len("## Default Loop")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + inputFilePayloadGuidance + "\n\n" + body[insertPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Create/Edit Preflight"); headingPos >= 0 {
		insertPos := headingPos + len("## Create/Edit Preflight")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + inputFilePayloadGuidance + "\n" + body[insertPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Core Rule"); headingPos >= 0 {
		insertPos := headingPos + len("## Core Rule")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + inputFilePayloadGuidance + "\n" + body[insertPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n## Large run payloads\n\n" + inputFilePayloadGuidance + "\n"
}

func ensureAuthoringDefaultsContractMatrix(body string) string {
	if strings.Contains(body, "Before meaningful edits, write a compact workflow contract and acceptance") {
		return body
	}
	guidance := strings.Join([]string{
		"- Before meaningful edits, write a compact workflow contract and acceptance",
		"  matrix. Include trigger/interface, inputs, setup, integrations, selection or",
		"  exclusion rules, output schema, downstream consumer, side effects, failure",
		"  behavior, reject/keep cases, required fields, forbidden output, and what must",
		"  never happen.",
		"- Before source grows, choose the shortest correct path: edit/call an existing",
		"  workspace flow, reuse an approved template, install/call a public flow through",
		"  an installed callable interface, or build new only when reuse fails the",
		"  contract, auth, billing, cost, quality, or output bar.",
	}, "\n")
	if headingPos := h2LineStartOutsideFences(body, "## Authoring Defaults"); headingPos >= 0 {
		insertPos := headingPos + len("## Authoring Defaults")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + guidance + "\n" + body[insertPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n## Authoring Defaults\n\n" + guidance + "\n"
}

func ensureLintBeforePushGuidance(body string) string {
	if hasLintTimeoutGuidance(body) {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Create/Edit Preflight"); headingPos >= 0 {
		insertPos := headingPos + len("## Create/Edit Preflight")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + lintBeforePushGuidance + "\n" + body[insertPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Default Loop"); headingPos >= 0 {
		insertPos := headingPos + len("## Default Loop")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + lintBeforePushGuidance + "\n" + body[insertPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Solution surfaces first (Required before editing)"); headingPos >= 0 {
		insertPos := nextH2LineStartOutsideFences(body, headingPos+len("## Solution surfaces first (Required before editing)"))
		if insertPos >= 0 {
			return body[:insertPos] + "## Draft lint before push\n\n" + lintBeforePushGuidance + "\n\n" + body[insertPos:]
		}
	}
	return strings.TrimRight(body, "\n") + "\n\n## Draft lint before push\n\n" + lintBeforePushGuidance + "\n"
}

func ensureAuthorFlowEfficientLoop(body string) string {
	readinessBullet := strings.Join([]string{
		"- before wrapper smoke tests around paid public apps, inspect",
		"  `breyta flows installations get <installation-id>` and check",
		"  `data.runReadiness` for billing/trial blocks",
	}, "\n")
	wrapperFlowBullet := strings.Join([]string{
		"- inside authored Breyta wrapper flows, call installed public children with",
		"  `flow/call-flow` plus `:installation-id`; reserve installed HTTP/MCP",
		"  interfaces for external clients and MCP-capable agents",
	}, "\n")
	if strings.Contains(body, readinessBullet) && strings.Contains(body, wrapperFlowBullet) {
		return body
	}
	transportBullet := "- treat HTTP/MCP as consumer transports for installed callable interfaces,"
	if transportPos := strings.Index(body, transportBullet); transportPos >= 0 {
		missing := make([]string, 0, 2)
		if !strings.Contains(body, readinessBullet) {
			missing = append(missing, readinessBullet)
		}
		if !strings.Contains(body, wrapperFlowBullet) {
			missing = append(missing, wrapperFlowBullet)
		}
		return body[:transportPos] + strings.Join(missing, "\n") + "\n" + body[transportPos:]
	}
	guidance := strings.Join([]string{
		"Use the default loop as one efficient workflow creation method, not as a",
		"separate fast path. Before source grows:",
		"",
		"- classify the work mode: new flow, existing-flow edit, duplicate/clone,",
		"  hardening/debugging, public/installable flow, agentic flow, or side-effecting",
		"  flow",
		"- identify the primary consumer: human operator, another flow, HTTP caller,",
		"  MCP caller, public app user, or internal admin",
		"- write the contract and acceptance matrix before source grows: trigger,",
		"  inputs, setup, integrations, output shape, downstream consumer, failure",
		"  behavior, reject/keep cases, required fields, forbidden output, dedupe keys,",
		"  and representative proof inputs",
		"- choose the shortest correct path: existing workspace flow, approved",
		"  template, public/installable callable flow, or new build",
		"- reuse a public/installable flow only if it satisfies the contract, output",
		"  schema, setup/auth model, billing/entitlement model, and quality bar with",
		"  less risk than rebuilding",
		"- prove public/installable flow reuse with",
		"  `breyta flows installations interfaces <installation-id>` and an",
		"  installation-scoped `breyta flows interfaces call ... --installation-id <installation-id>`",
		readinessBullet,
		wrapperFlowBullet,
		"- treat HTTP/MCP as consumer transports for installed callable interfaces,",
		"  not as an instruction to create or assume extra builder infrastructure",
		"- patch in focused lanes: behavior first, output/schema second, verification",
		"  or observability only when runtime evidence shows a gap",
	}, "\n")
	if headingPos := h2LineStartOutsideFences(body, "## Default Loop"); headingPos >= 0 {
		insertPos := headingPos + len("## Default Loop")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + guidance + "\n\n" + body[insertPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n## Default Loop\n\n" + guidance + "\n"
}

func ensureDebugAcceptanceCaseGuidance(body string) string {
	if strings.Contains(body, "Before patching, convert the bad run, bad output, or UI mismatch into an") {
		return body
	}
	guidance := strings.Join([]string{
		"Before patching, convert the bad run, bad output, or UI mismatch into an",
		"acceptance case: what should be rejected or kept, which fields/counts/resource",
		"refs/rows prove success, which stale/debug/provider/internal content must be",
		"absent, and which target/interface/installation/input will prove the fix.",
	}, "\n")
	if headingPos := h2LineStartOutsideFences(body, "## Default Loop"); headingPos >= 0 {
		insertPos := headingPos + len("## Default Loop")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + guidance + "\n\n" + body[insertPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n## Default Loop\n\n" + guidance + "\n"
}

func ensureOutputHandoffContract(body string) string {
	if h2LineStartOutsideFences(body, "## Downstream Handoff Contract") >= 0 {
		return body
	}
	section := strings.Join([]string{
		"## Downstream Handoff Contract",
		"",
		"Before editing final output, identify who or what consumes it: human operator,",
		"another flow, HTTP caller, MCP caller, CRM/table sync, or public app user.",
		"Shape the result for that consumer instead of returning an author debug map.",
		"",
		"For callable/API consumers, include stable structured fields such as status,",
		"counts, selected items, manual-review items, clean ids/URLs, dedupe keys,",
		"resource refs, table refs, warnings, and failure reasons. Keep readable",
		"Markdown as presentation, not the only machine-readable contract, when another",
		"flow must consume the result.",
		"",
		"For GTM/operator tables, verify representative rows as the user would scan",
		"them: useful titles, clean URLs, clear reasons, dates/freshness, dedupe keys,",
		"source evidence, and no company/private/provider/debug leakage. Reject outputs",
		"that are structurally valid but not useful enough for the downstream action.",
	}, "\n")
	if headingPos := h2LineStartOutsideFences(body, "## Artifact Audience Review"); headingPos >= 0 {
		return body[:headingPos] + section + "\n\n" + body[headingPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n" + section + "\n"
}

func ensurePublicFlowReuseDuringAuthoring(body string) string {
	readinessGuidance := strings.Join([]string{
		"Before wrapper smoke tests around paid public apps, inspect",
		"`breyta flows installations get <installation-id>` and check",
		"`data.runReadiness` for billing/trial blocks.",
	}, "\n")
	internalCompositionGuidance := strings.Join([]string{
		"Inside authored Breyta wrapper flows, call the installed public flow with",
		"`flow/call-flow` and `:installation-id`. Use installed HTTP/MCP interfaces",
		"for external clients and MCP-capable agents, not as the default internal",
		"wrapper-flow composition path.",
	}, "\n")
	if strings.Contains(body, readinessGuidance) && strings.Contains(body, internalCompositionGuidance) {
		return body
	}
	missingGuidance := make([]string, 0, 2)
	if !strings.Contains(body, readinessGuidance) {
		missingGuidance = append(missingGuidance, readinessGuidance)
	}
	if !strings.Contains(body, internalCompositionGuidance) {
		missingGuidance = append(missingGuidance, internalCompositionGuidance)
	}
	missingBlock := strings.Join(missingGuidance, "\n\n")
	if existingPos := strings.Index(body, "During authoring, check public/installable flows before building from scratch"); existingPos >= 0 {
		return strings.TrimRight(body, "\n") + "\n\n" + missingBlock + "\n"
	}
	guidance := strings.Join([]string{
		"During authoring, check public/installable flows before building from scratch",
		"when the desired output may already exist as a paid or free Breyta app. Reuse is",
		"valid only when the installed flow satisfies the contract, output schema,",
		"setup/auth model, billing/entitlement model, quality bar, and failure behavior",
		"with less risk than editing or building a new flow.",
		"",
		readinessGuidance,
		"",
		internalCompositionGuidance,
	}, "\n")
	anchor := "reuse a public flow, prefer the installed HTTP or MCP interface over author\ndraft/live endpoints."
	if anchorPos := strings.Index(body, anchor); anchorPos >= 0 {
		insertPos := anchorPos + len(anchor)
		return body[:insertPos] + "\n\n" + guidance + body[insertPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Public Flow As Reusable Tool"); headingPos >= 0 {
		insertPos := headingPos + len("## Public Flow As Reusable Tool")
		if eol := strings.Index(body[insertPos:], "\n"); eol >= 0 {
			insertPos += eol + 1
		}
		return body[:insertPos] + "\n" + guidance + "\n" + body[insertPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n## Public Flow As Reusable Tool\n\n" + guidance + "\n"
}

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

func ensurePaidAppMarketplaceSection(body string) string {
	if h2LineStartOutsideFences(body, "## Paid app marketplace authoring (Source-authored)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Discover card media (Public discover polish)"); headingPos >= 0 {
		return body[:headingPos] + paidAppMarketplaceSection + "\n\n" + body[headingPos:]
	}
	if headingPos := h2LineStartOutsideFences(body, "## Public/end-user UI verification (Required for public flows)"); headingPos >= 0 {
		insertPos := nextH2LineStartOutsideFences(body, headingPos+len("## Public/end-user UI verification (Required for public flows)"))
		if insertPos >= 0 {
			return body[:insertPos] + paidAppMarketplaceSection + "\n\n" + body[insertPos:]
		}
		return strings.TrimRight(body, "\n") + "\n\n" + paidAppMarketplaceSection + "\n"
	}
	if headingPos := h2LineStartOutsideFences(body, "## Public Approval Gate"); headingPos >= 0 {
		return body[:headingPos] + paidAppMarketplaceSection + "\n\n" + body[headingPos:]
	}
	return strings.TrimRight(body, "\n") + "\n\n" + paidAppMarketplaceSection + "\n"
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

func ensureN8NImportGuidance(body string) string {
	if strings.Contains(body, n8nImportGuidanceLine) {
		return body
	}
	for _, heading := range []string{
		"## Create/Edit Preflight",
		"## Authoring standard (required before editing)",
		"## Preflight",
	} {
		headingPos := h2LineStartOutsideFences(body, heading)
		if headingPos < 0 {
			continue
		}
		insertAt := nextH2LineStartOutsideFences(body, headingPos+1)
		if insertAt < 0 {
			insertAt = len(body)
		}
		section := strings.TrimRight(body[headingPos:insertAt], "\n")
		return body[:headingPos] + section + "\n" + n8nImportGuidanceLine + "\n" + body[insertAt:]
	}
	return strings.TrimRight(body, "\n") + "\n\n" + n8nImportGuidanceLine + "\n"
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
