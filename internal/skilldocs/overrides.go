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
	}

	for _, pair := range replacements {
		updated = strings.ReplaceAll(updated, pair[0], pair[1])
	}
	updated = ensureNamingConventionsSection(updated)
	updated = ensureWorkflowPlanningSection(updated)
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
  - include high-intent keywords when relevant (for example autonomous, support, gmail, ai)
- :description
  - one sentence with trigger mode + core actions + final outcome
  - avoid vague wording like "process data"
- :tags
  - include 4-8 searchable nouns/adjectives tied to intent and channel
  - example set: ["autonomous" "support-agent" "support" "gmail" "llm"]
- trigger :label
  - make mode/source explicit: manual test vs webhook vs schedule cadence
  - examples: Manual smoke test (support agent), Autonomous support reply (Gmail push webhook), Fallback autonomous support scan (every 5 minutes)
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
    - ^{:label "Should we check Gmail for new support emails?" :yes "Check inbox for new emails" :no "Use provided support context only"}
- pre-push scan check
  - from :name, :description, :triggers, and first few step ids/titles, a new operator should infer flow behavior in ~10 seconds
- CLI search clarity
  - breyta flows search <query> is for approved template discovery, not workspace flow lookup
  - for workspace lookup, use breyta flows list output with explicit keywords and grep/filter locally
  - when flows are user-facing, ensure search tokens appear in :name, :description, and :tags (for example autonomous, support, gmail, reply)`

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
- pattern-first design (adapted from proven n8n workflow patterns)
  - choose the dominant pattern up front: webhook processing, HTTP API integration, database operations, AI agent workflow, or scheduled task
  - if multiple patterns are needed, split by clear subpaths and name each subpath explicitly in steps/titles
  - enforce deterministic progression: trigger -> intake -> decide -> act -> summarize -> post-process
  - define edge cases before build (no items, partial data, external API failure, already-processed input)
- support-agent pattern defaults
  - path one: renew watch subscription
  - path two: process support email and draft/send response
  - keep shared preparation and final summary explicit, not hidden in generic finalize/normalize labels
- anti-patterns to avoid
  - technical/internal step names that hide behavior
  - branch labels that do not read like operator decisions
  - adding new versions to discover architecture instead of deciding architecture before edits`

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
