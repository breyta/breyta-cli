package skilldocs

import (
	"regexp"
	"strings"
)

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
	}

	for _, pair := range replacements {
		updated = strings.ReplaceAll(updated, pair[0], pair[1])
	}
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

func ensureNamingConventionsSection(body string) string {
	if strings.Contains(body, "## Readability + Searchability Naming Conventions (Required)") {
		return body
	}
	if loc := capabilityDiscoveryHeading.FindStringIndex(body); loc != nil {
		return body[:loc[0]] + namingConventionsSection + "\n\n" + body[loc[0]:]
	}
	return body + "\n\n" + namingConventionsSection + "\n"
}

var capabilityDiscoveryHeading = regexp.MustCompile(`(?m)^## Capability Discovery[ \t]*$`)
