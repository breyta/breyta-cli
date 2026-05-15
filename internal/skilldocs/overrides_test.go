package skilldocs

import (
	"strings"
	"testing"
)

func TestApplyCLIOverrides_BreytaSkillRewritesSearchGuidance(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Non-Negotiables",
			"- Before creating a new flow, search existing definitions: `breyta flows search <query>`.",
			"",
			"## Preflight",
			"3. Confirm reusable resources:",
			"   - `breyta connections list`",
			"   - `breyta flows search <query>`",
		}, "\n")),
		"references/x.md": []byte("ref"),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "Before creating or editing a flow, pick a task mode, inspect current state") {
		t.Fatalf("expected search-first guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows templates search \"<problem or integration query>\" --limit 5") {
		t.Fatalf("expected query-shaped template search guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows search \"<integration or problem query>\" --limit 5") {
		t.Fatalf("expected workspace search guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows grep \"<literal>\" --or \"<variant>\"") {
		t.Fatalf("expected workspace grep guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows workspace examples step <type> \"<query>\" --limit 3") {
		t.Fatalf("expected private primitive example extraction guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows examples step <type> \"<query>\" --limit 3") {
		t.Fatalf("expected primitive example extraction guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "Primitive-first reuse: inspect matching snippets and referenced dependencies before full templates") {
		t.Fatalf("expected primitive-first reuse guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "Existing workspace flow: `breyta flows show <slug>` for compact summary or `breyta flows pull <slug>` for editable source") {
		t.Fatalf("expected workspace flow guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "## Primitive-first reuse for create/edit (Required)") {
		t.Fatalf("expected primitive-first reuse section in override, got:\n%s", body)
	}
	if !strings.Contains(body, "inspect one full template only for cross-step architecture reuse") {
		t.Fatalf("expected full-template escalation guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "## Workflow quality contract (Required)") {
		t.Fatalf("expected workflow quality contract in override, got:\n%s", body)
	}
	if !strings.Contains(body, "Do not run `breyta connections test --all`") && !strings.Contains(body, "not `breyta connections test --all`") {
		t.Fatalf("expected targeted connection test guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "## Public/end-user UI verification (Required for public flows)") {
		t.Fatalf("expected public UI verification section in override, got:\n%s", body)
	}
	if !strings.Contains(body, "do not tell the user a public/end-user flow is \"ready for UI\" from draft proof alone") {
		t.Fatalf("expected ready-for-UI guardrail in override, got:\n%s", body)
	}
	if !strings.Contains(body, "`web UI not verified` in the risk ledger") {
		t.Fatalf("expected web UI risk-ledger guidance in override, got:\n%s", body)
	}
	if !strings.Contains(body, "do not stop at activation") {
		t.Fatalf("expected activation-vs-install guardrail in override, got:\n%s", body)
	}
	if !strings.Contains(body, "Discover install plus an installed run") {
		t.Fatalf("expected Discover installed-run proof in override, got:\n%s", body)
	}
	if !strings.Contains(body, "https://api.openai.com/v1") {
		t.Fatalf("expected OpenAI base URL guidance in override, got:\n%s", body)
	}
	if string(got["references/x.md"]) != "ref" {
		t.Fatalf("expected non-skill files preserved")
	}
}

func TestApplyCLIOverrides_BreytaSkillAddsKeyedScheduleGuard(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Core learnings",
			"- Schedule triggers require a prod profile before deploy and activate",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "For keyed concurrency (`:type :keyed`), schedule `:config :input` must include the key-field") {
		t.Fatalf("expected keyed schedule guard in override, got:\n%s", body)
	}
	if !strings.Contains(body, "activation will fail") {
		t.Fatalf("expected explicit activation failure warning in override, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_NonBreytaNoop(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte("hello"),
	}
	got := ApplyCLIOverrides("other-skill", input)
	if string(got["SKILL.md"]) != "hello" {
		t.Fatalf("expected no change for non-breyta skill")
	}
}

func TestApplyCLIOverrides_BreytaPlaybookRouterSkillDoesNotReinflate(t *testing.T) {
	body := strings.Join([]string{
		"## Purpose",
		"compact router",
		"",
		"## Playbook Matrix",
		"- `playbooks/author-flows.md`",
		"- `playbooks/debug-and-verify.md`",
		"- `references/runtime-data-shapes.md`",
		"",
		"## Default Command Budget",
		"- compact defaults",
	}, "\n")
	input := map[string][]byte{
		"SKILL.md": []byte(body),
	}

	got := ApplyCLIOverrides("breyta", input)
	if string(got["SKILL.md"]) != body {
		t.Fatalf("expected current playbook router skill to remain unchanged, got:\n%s", string(got["SKILL.md"]))
	}
	if strings.Contains(string(got["SKILL.md"]), "## Workflow architecture planning") {
		t.Fatalf("expected playbook router skill not to be inflated, got:\n%s", string(got["SKILL.md"]))
	}
}

func TestApplyCLIOverrides_BreytaCurrentCanonicalSkillDoesNotReinflate(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Reference Loading Matrix",
			"- creating/editing: `references/authoring-loop.md`",
			"- public flows: `references/public-flows.md`",
			"- outputs/tables: `references/outputs-and-tables.md`",
			"",
			"## Create/Edit Preflight",
			"- bounded discovery",
			"",
			"## Public Approval Gate",
			"- end-user landing page approval",
			"",
			"## Provider/API Freshness And Model Selection",
			"- check current official provider docs/API references",
			"",
			"## Output Guidance",
			"- include full URLs",
		}, "\n")),
		"references/public-flows.md": []byte("# Public Flows\n"),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	for _, unexpected := range []string{
		"## Workflow architecture planning (Required before build)",
		"## Reliability + determinism planning (Required before push)",
		"## Readability + Searchability Naming Conventions (Required)",
		"## Doc lookup patterns (Required before guessing implementation details)",
	} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("expected current canonical skill to stay compact, found %q in:\n%s", unexpected, body)
		}
	}
	if !strings.Contains(body, "## Output Guidance") {
		t.Fatalf("expected canonical sections preserved, got:\n%s", body)
	}
	if !strings.Contains(body, "normal bounded inspection path for agents") {
		t.Fatalf("expected bounded resource-read guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "Do not put full report bodies in table cells such as `report_markdown`") {
		t.Fatalf("expected large table-cell hygiene guidance, got:\n%s", body)
	}
	if !strings.Contains(body, ":persist {:type :blob :tier :ephemeral}") {
		t.Fatalf("expected ephemeral blob tier guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "For n8n workflow JSON imports, use `breyta flows import n8n <workflow.json>` first") {
		t.Fatalf("expected n8n importer-first guidance, got:\n%s", body)
	}
	if string(got["references/public-flows.md"]) != "# Public Flows\n" {
		t.Fatalf("expected reference file preserved")
	}
}

func TestApplyCLIOverrides_BreytaCanonicalSkillRewritesStaleModelGuidance(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Create/Edit Preflight",
			"- OpenAI models: check current OpenAI docs before introducing or changing model ids",
			"",
			"## Public Flow Presentation",
			"- end-user landing page",
			"",
			"## Model Selection",
			"- Do not default new OpenAI-backed LLM steps to GPT-4-era model names.",
			"- Before adding or changing OpenAI model ids, check current OpenAI model guidance and relevant Breyta docs/examples.",
			"- As of the current OpenAI latest-model guide, `gpt-5.5` is the latest model.",
			"- Preserve explicit user requests, such as `gpt-5.4`, even when newer models exist.",
			"- When editing existing flows, keep legacy models only if compatibility, cost, or evaluation history is intentional. Otherwise propose upgrading to the current approved GPT-5.x model.",
			"",
			"## Output Guidance",
			"- include full URLs",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if strings.Contains(body, "As of the current OpenAI latest-model guide") {
		t.Fatalf("expected fixed latest-model claim removed, got:\n%s", body)
	}
	if strings.Contains(body, "gpt-5.5") {
		t.Fatalf("expected volatile latest-model id removed, got:\n%s", body)
	}
	if !strings.Contains(body, "## Provider/API Freshness And Model Selection") {
		t.Fatalf("expected provider/API freshness section, got:\n%s", body)
	}
	if !strings.Contains(body, "use `gpt-5.4` as Breyta's current API default") {
		t.Fatalf("expected gpt-5.4 current API default guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "Do not claim or use a provider model unless current provider docs/API availability prove it exists") {
		t.Fatalf("expected model availability guardrail, got:\n%s", body)
	}
	if !strings.Contains(body, "check current official provider docs/API references") {
		t.Fatalf("expected official provider docs/API guidance, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_BreytaSkillInjectsNamingConventions(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Non-Negotiables",
			"- Keep :flow orchestration-focused.",
			"",
			"## Capability Discovery",
			"- breyta docs",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "## Workflow architecture planning (Required before build)") {
		t.Fatalf("expected workflow planning section, got:\n%s", body)
	}
	if !strings.Contains(body, "planning gate first") {
		t.Fatalf("expected planning-first guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "## Reliability + determinism planning (Required before push)") {
		t.Fatalf("expected reliability section, got:\n%s", body)
	}
	if !strings.Contains(body, "## Provenance for derived flows (Required when reusing existing flows)") {
		t.Fatalf("expected provenance section, got:\n%s", body)
	}
	if !strings.Contains(body, "## Flow lifecycle cleanup (Public CLI surface)") {
		t.Fatalf("expected flow lifecycle section, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows provenance set <slug> --from-consulted") {
		t.Fatalf("expected provenance command guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows provenance set <slug> --template <template-slug>") {
		t.Fatalf("expected template provenance guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows archive <slug>") {
		t.Fatalf("expected archive guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows delete <slug> --yes --force") {
		t.Fatalf("expected force delete guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "name the idempotency or duplicate-protection key") {
		t.Fatalf("expected idempotency guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "use `sequential` when order matters") {
		t.Fatalf("expected explicit sequential guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "use `fanout` only for independent, bounded, side-effect-safe items") {
		t.Fatalf("expected explicit fanout guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "use `keyed` when work must serialize per entity") {
		t.Fatalf("expected explicit keyed guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "if concurrency is not clearly beneficial, default to `sequential`") {
		t.Fatalf("expected sequential-default guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "pass signed URLs/blob refs for large artifacts") {
		t.Fatalf("expected large artifact reference guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "Do not put full report bodies in table cells such as `report_markdown`") {
		t.Fatalf("expected large table-cell hygiene guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "never advance cursors/checkpoints past failed work") {
		t.Fatalf("expected cursor safety guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "define exact runtime proof") {
		t.Fatalf("expected observability/runtime proof guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "prove the chosen mode with evidence") {
		t.Fatalf("expected concurrency verification guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "## Readability + Searchability Naming Conventions (Required)") {
		t.Fatalf("expected naming conventions section, got:\n%s", body)
	}
	if !strings.Contains(body, "default to Should we ...? framing when possible") {
		t.Fatalf("expected Should we framing guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "search tokens appear in :name, :description, and :tags") {
		t.Fatalf("expected search token guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "breyta flows search \"<query>\" --limit 5 searches actual workspace flow metadata") ||
		!strings.Contains(body, "breyta flows grep \"<literal>\" --limit 5 searches actual workspace flow source/config") {
		t.Fatalf("expected workspace search naming guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "## Provider/API Freshness And Model Selection") {
		t.Fatalf("expected provider/API freshness section, got:\n%s", body)
	}
	if !strings.Contains(body, "For n8n workflow JSON imports, use `breyta flows import n8n <workflow.json>` first") {
		t.Fatalf("expected n8n importer-first guidance, got:\n%s", body)
	}
	if !strings.Contains(body, "OpenAI, Anthropic/Claude, Google/Gemini, OpenAI-compatible providers") {
		t.Fatalf("expected broad provider guidance, got:\n%s", body)
	}
	workflowPos := strings.Index(body, "## Workflow architecture planning (Required before build)")
	reliabilityPos := strings.Index(body, "## Reliability + determinism planning (Required before push)")
	provenancePos := strings.Index(body, "## Provenance for derived flows (Required when reusing existing flows)")
	lifecyclePos := strings.Index(body, "## Flow lifecycle cleanup (Public CLI surface)")
	namingPos := strings.Index(body, "## Readability + Searchability Naming Conventions (Required)")
	if workflowPos == -1 || reliabilityPos == -1 || provenancePos == -1 || lifecyclePos == -1 || namingPos == -1 || !(workflowPos < reliabilityPos && reliabilityPos < provenancePos && provenancePos < lifecyclePos && lifecyclePos < namingPos) {
		t.Fatalf("expected workflow, reliability, provenance, lifecycle, then naming sections in order, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_DoesNotDuplicateNamingConventions(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Readability + Searchability Naming Conventions (Required)",
			"- existing content",
			"",
			"## Capability Discovery",
			"- breyta docs",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	count := strings.Count(body, "## Readability + Searchability Naming Conventions (Required)")
	if count != 1 {
		t.Fatalf("expected naming conventions header exactly once, got %d\n%s", count, body)
	}
}

func TestApplyCLIOverrides_DoesNotDuplicateWorkflowPlanningSection(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Workflow architecture planning (Required before build)",
			"- existing content",
			"",
			"## Capability Discovery",
			"- breyta docs",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	count := strings.Count(body, "## Workflow architecture planning (Required before build)")
	if count != 1 {
		t.Fatalf("expected workflow planning header exactly once, got %d\n%s", count, body)
	}
}

func TestApplyCLIOverrides_DoesNotDuplicateReliabilitySection(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Reliability + determinism planning (Required before push)",
			"- existing content",
			"",
			"## Capability Discovery",
			"- breyta docs",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	count := strings.Count(body, "## Reliability + determinism planning (Required before push)")
	if count != 1 {
		t.Fatalf("expected reliability header exactly once, got %d\n%s", count, body)
	}
}

func TestApplyCLIOverrides_RewritesHardQualityGateLanguage(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Workflow quality",
			"- Run a hard quality gate before each build/release cycle",
			"- Quality gate is the first step",
			"",
			"## Capability Discovery",
			"- breyta docs",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if strings.Contains(body, "Run a hard quality gate before each build/release cycle") {
		t.Fatalf("expected hard quality gate language to be rewritten, got:\n%s", body)
	}
	if !strings.Contains(body, "Run a planning gate before each build/release cycle (required).") {
		t.Fatalf("expected planning gate wording, got:\n%s", body)
	}
	if !strings.Contains(body, "- Planning gate is the first step") {
		t.Fatalf("expected first-step planning wording, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_DoesNotMatchSubHeadingCapabilityDiscovery(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Non-Negotiables",
			"- Keep :flow orchestration-focused.",
			"",
			"### Capability Discovery",
			"- legacy subsection",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "### Capability Discovery") {
		t.Fatalf("expected original H3 heading to remain, got:\n%s", body)
	}
	if strings.Contains(body, "## Readability + Searchability Naming Conventions (Required)\n\n### Capability Discovery") {
		t.Fatalf("unexpected insertion before H3 Capability Discovery heading:\n%s", body)
	}
	sectionPos := strings.Index(body, "## Readability + Searchability Naming Conventions (Required)")
	h3Pos := strings.Index(body, "### Capability Discovery")
	if sectionPos == -1 || h3Pos == -1 || sectionPos < h3Pos {
		t.Fatalf("expected conventions section appended after existing H3 subsection when H2 is absent, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_IgnoresCapabilityHeadingInsideCodeFence(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Intro",
			"```md",
			"## Capability Discovery",
			"- example only",
			"```",
			"",
			"## Capability Discovery",
			"- real section",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "```md\n## Capability Discovery\n- example only\n```") {
		t.Fatalf("expected fenced example to remain intact, got:\n%s", body)
	}
	sectionPos := strings.Index(body, "## Readability + Searchability Naming Conventions (Required)")
	realHeadingPos := strings.LastIndex(body, "\n## Capability Discovery\n")
	realSectionPos := strings.LastIndex(body, "- real section")
	if sectionPos == -1 || realHeadingPos == -1 || realSectionPos == -1 || !(sectionPos < realHeadingPos && realHeadingPos < realSectionPos) {
		t.Fatalf("expected naming conventions inserted before real H2 heading, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_IgnoresCapabilityHeadingInsideNestedBacktickFence(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Intro",
			"````md",
			"```md",
			"## Capability Discovery",
			"- example only",
			"```",
			"````",
			"",
			"## Capability Discovery",
			"- real section",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "````md\n```md\n## Capability Discovery\n- example only\n```\n````") {
		t.Fatalf("expected nested fenced example to remain intact, got:\n%s", body)
	}
	sectionPos := strings.Index(body, "## Readability + Searchability Naming Conventions (Required)")
	realHeadingPos := strings.LastIndex(body, "\n## Capability Discovery\n")
	realSectionPos := strings.LastIndex(body, "- real section")
	if sectionPos == -1 || realHeadingPos == -1 || realSectionPos == -1 || !(sectionPos < realHeadingPos && realHeadingPos < realSectionPos) {
		t.Fatalf("expected naming conventions inserted before real H2 heading outside nested fence, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_DoesNotCloseFenceOnTrailingFenceText(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Intro",
			"```md",
			"`````clj",
			"## Capability Discovery",
			"- example only",
			"```",
			"",
			"## Capability Discovery",
			"- real section",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	if !strings.Contains(body, "```md\n`````clj\n## Capability Discovery\n- example only\n```") {
		t.Fatalf("expected fenced example with trailing fence text to remain intact, got:\n%s", body)
	}
	sectionPos := strings.Index(body, "## Readability + Searchability Naming Conventions (Required)")
	realHeadingPos := strings.LastIndex(body, "\n## Capability Discovery\n")
	realSectionPos := strings.LastIndex(body, "- real section")
	if sectionPos == -1 || realHeadingPos == -1 || realSectionPos == -1 || !(sectionPos < realHeadingPos && realHeadingPos < realSectionPos) {
		t.Fatalf("expected naming conventions inserted before real H2 heading outside fence, got:\n%s", body)
	}
}

func TestApplyCLIOverrides_DoesNotSkipInsertWhenNamingHeadingOnlyInFence(t *testing.T) {
	input := map[string][]byte{
		"SKILL.md": []byte(strings.Join([]string{
			"## Intro",
			"```md",
			"## Readability + Searchability Naming Conventions (Required)",
			"```",
			"",
			"## Capability Discovery",
			"- real section",
		}, "\n")),
	}

	got := ApplyCLIOverrides("breyta", input)
	body := string(got["SKILL.md"])
	count := strings.Count(body, "## Readability + Searchability Naming Conventions (Required)")
	if count != 2 {
		t.Fatalf("expected one inserted naming section plus fenced example heading, got %d occurrences:\n%s", count, body)
	}
	sectionPos := strings.Index(body, "## Readability + Searchability Naming Conventions (Required)\n\nGoal:")
	realHeadingPos := strings.LastIndex(body, "\n## Capability Discovery\n")
	if sectionPos == -1 || realHeadingPos == -1 || sectionPos > realHeadingPos {
		t.Fatalf("expected inserted naming section before real capability heading, got:\n%s", body)
	}
}
