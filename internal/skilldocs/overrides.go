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
	}

	for _, pair := range replacements {
		updated = strings.ReplaceAll(updated, pair[0], pair[1])
	}
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
