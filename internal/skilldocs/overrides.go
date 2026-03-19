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
	replacements := [][2]string{
		{
			"- Before creating a new flow, search existing definitions: `breyta flows search <query>`.",
			"- Before creating a new flow, inspect workspace flows first: `breyta flows list` then `breyta flows show <slug>`.\n- Use `breyta flows search <query>` only for approved template discovery/reuse.",
		},
		{
			"3. Confirm reusable resources:\n   - `breyta connections list`\n   - `breyta flows search <query>`",
			"3. Confirm reusable resources:\n   - `breyta connections list`\n   - Existing workspace flow: `breyta flows list` then `breyta flows show <slug>`\n   - Approved template discovery: `breyta flows search <query>`",
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
			"- `breyta flows push --file ./tmp/flows/<slug>.clj`\n- `breyta flows configure <slug> ...` (when required)\n- `breyta flows configure check <slug>`\n- If the flow belongs to a bundle of dependent flows, set explicit order with `breyta flows update <slug> --group-order <n>` and re-check ordered siblings with `breyta flows show <slug> --pretty`\n- Live target updates after slot changes: use `--target live --version <n|latest>` (and `--from-draft` when promoting draft bindings)\n- Optional read-only verification: `breyta flows validate <slug>`\n- Inspect draft-vs-live changes before release: `breyta flows diff <slug>`",
		},
		{
			"- `breyta flows release <slug>`\n- `breyta flows promote <slug>`\n- `breyta flows installations configure <installation-id> --input '{...}'`",
			"- `breyta flows release <slug> --release-note-file ./release-note.md`\n- `breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md` (edit the note later)\n- `breyta flows promote <slug>`\n- `breyta flows installations configure <installation-id> --input '{...}'`",
		},
	}

	for _, pair := range replacements {
		updated = strings.ReplaceAll(updated, pair[0], pair[1])
	}
	updated = ensureWorkflowPlanningSection(updated)
	updated = ensureReliabilitySection(updated)
	updated = ensureProvenanceSection(updated)
	updated = ensureNamingConventionsSection(updated)
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
  - for workspace lookup, use breyta flows list output with explicit keywords and grep/filter locally
  - when flows are user-facing, ensure search tokens appear in :name, :description, and :tags (for example invoice, approval, webhook, billing)`

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

func ensureNamingConventionsSection(body string) string {
	if h2LineStartOutsideFences(body, "## Readability + Searchability Naming Conventions (Required)") >= 0 {
		return body
	}
	if headingPos := h2LineStartOutsideFences(body, "## Capability Discovery"); headingPos >= 0 {
		return body[:headingPos] + namingConventionsSection + "\n\n" + body[headingPos:]
	}
	return body + "\n\n" + namingConventionsSection + "\n"
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
