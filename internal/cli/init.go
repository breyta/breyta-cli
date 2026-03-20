package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/breyta/breyta-cli/internal/skilldocs"
	"github.com/breyta/breyta-cli/skills"
	"github.com/spf13/cobra"
)

func newInitCmd(app *App) *cobra.Command {
	var provider string
	var dir string
	var force bool
	var noSkill bool
	var noWorkspace bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap local setup for agent-driven Breyta usage",
		Long: strings.TrimSpace(`
Sets up Breyta for agent-first usage:
- installs the Breyta agent skill bundle for Codex/Cursor/Claude Code/Gemini CLI
- creates a local "agent workspace" directory with an AGENTS.md file

It does not require authentication, but you'll typically run ` + "`breyta auth login`" + `
right after to connect the CLI to your Breyta account.
`),
		Example: strings.TrimSpace(`
# Install the skill bundle for your agent tool and create ./breyta-workspace
breyta init --provider codex

# Cursor, Claude Code, and Gemini CLI
breyta init --provider cursor
breyta init --provider claude
breyta init --provider gemini

# Use a specific directory and overwrite existing files
breyta init --dir ./my-breyta-workspace --force
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if noSkill && noWorkspace {
				return errors.New("nothing to do (both --no-skill and --no-workspace were set)")
			}

			p := skills.Provider(strings.TrimSpace(provider))
			if strings.TrimSpace(provider) == "" {
				p = skills.ProviderCodex
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			target, err := skills.Target(home, p)
			if err != nil {
				// In workspace-only mode, provider is irrelevant. Avoid failing init when users
				// pass an arbitrary --provider value along with --no-skill.
				if !noSkill {
					return err
				}
				// Best-effort: fall back to a known provider so AGENTS.md can still include
				// a concrete path example.
				target, _ = skills.Target(home, skills.ProviderCodex)
			}

			skillInstalled := false

			if !noSkill {
				ensureAPIURL(app)
				if strings.TrimSpace(app.APIURL) == "" {
					if noWorkspace {
						return errors.New("missing api base url (try `breyta auth login` first)")
					}
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: missing api base url; skipped skill install (try `breyta auth login`, then `breyta skills install`)")
				} else {
					_, files, err := skilldocs.FetchBundle(context.Background(), nil, app.APIURL, app.Token, skills.BreytaSkillSlug)
					if err != nil {
						if noWorkspace {
							return err
						}
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: skill bundle download failed (%v); continuing without skill install\n", err)
					} else if _, err := skills.InstallBreytaSkillFiles(home, p, skilldocs.ApplyCLIOverrides(skills.BreytaSkillSlug, files)); err != nil {
						if noWorkspace {
							return err
						}
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: skill install failed (%v); continuing without skill install\n", err)
					} else {
						skillInstalled = true

						fmt.Fprintf(cmd.OutOrStdout(), "Installed Breyta agent skill bundle for %s in %s\n", target.Provider, target.Dir)
					}
				}
			}

			if noWorkspace {
				return nil
			}

			wsDir := strings.TrimSpace(dir)
			if wsDir == "" {
				wsDir = "breyta-workspace"
			}
			absDir, err := filepath.Abs(wsDir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(absDir, 0o755); err != nil {
				return err
			}

			if err := os.MkdirAll(filepath.Join(absDir, "flows"), 0o755); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(absDir, "tmp", "flows"), 0o755); err != nil {
				return err
			}

			agentsPath := filepath.Join(absDir, "AGENTS.md")
			agentsContent := []byte(renderInitAgentsMD(target, !skillInstalled))
			wrote, err := writeInitFile(agentsPath, agentsContent, force)
			if err != nil {
				return err
			}
			if wrote {
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", agentsPath)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Kept existing %s (use --force to overwrite)\n", agentsPath)
			}

			gitignorePath := filepath.Join(absDir, ".gitignore")
			gitignoreContent := []byte(strings.TrimSpace(initGitignore) + "\n")
			wrote, err = writeInitFile(gitignorePath, gitignoreContent, force)
			if err != nil {
				return err
			}
			if wrote {
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", gitignorePath)
			}

			readmePath := filepath.Join(absDir, "README.md")
			readmeContent := []byte(renderInitReadmeMD())
			wrote, err = writeInitFile(readmePath, readmeContent, force)
			if err != nil {
				return err
			}
			if wrote {
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", readmePath)
			}

			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Next:")
			fmt.Fprintf(cmd.OutOrStdout(), "- Open this folder in your agent tool (or `cd %s` and start the agent) so it can read %s\n", absDir, agentsPath)
			fmt.Fprintf(cmd.OutOrStdout(), "- First-session guide: %s\n", readmePath)
			fmt.Fprintln(cmd.OutOrStdout(), "- Authenticate: breyta auth login")
			fmt.Fprintln(cmd.OutOrStdout(), "- Verify identity + workspace summary: breyta auth whoami")
			fmt.Fprintln(cmd.OutOrStdout(), "- Discover approved templates: breyta flows search \"<idea>\"")
			fmt.Fprintln(cmd.OutOrStdout(), "- Stop after idea exploration unless you intentionally want to continue now")
			return nil
		},
	}

	_ = app
	cmd.Flags().StringVar(&provider, "provider", string(skills.ProviderCodex), "Install location for the agent skill bundle (codex|cursor|claude|gemini)")
	cmd.Flags().StringVar(&dir, "dir", "breyta-workspace", "Workspace directory to create (contains AGENTS.md, flows/, tmp/flows/)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files in the workspace directory")
	cmd.Flags().BoolVar(&noSkill, "no-skill", false, "Skip installing the agent skill bundle")
	cmd.Flags().BoolVar(&noWorkspace, "no-workspace", false, "Skip creating the workspace directory and files")
	return cmd
}

func writeInitFile(path string, content []byte, force bool) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, errors.New("missing path")
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, content, 0o644)
}

const initGitignore = `
.DS_Store

# Scratch pulls / generated artifacts
tmp/
`

func renderInitAgentsMD(target skills.InstallTarget, skillNotInstalled bool) string {
	skillLine := "- Breyta skill bundle: " + target.File + "\n"
	if skillNotInstalled {
		skillLine = "- (Not installed) Breyta skill bundle would be at: " + target.File + "\n"
	}

	return strings.TrimSpace(`# Breyta agent workspace

This folder is meant to be used with a coding agent (Codex, Cursor, Claude Code, Gemini CLI, etc.) to build and operate Breyta workflows ("flows") through the ` + "`breyta`" + ` CLI.

If this is your first session in this workspace, start with ` + "`README.md`" + ` in this folder. Keep this file for durable workflow guidance.

## Durable discovery defaults
- Start new work with approved template discovery: ` + "`breyta flows search <query>`" + `
- When you already know you're working from an existing workspace flow, inspect it with ` + "`breyta flows list`" + ` then ` + "`breyta flows show <slug>`" + `
- Verify identity + workspace summary any time with ` + "`breyta auth whoami`" + `
- Use ` + "`breyta docs`" + ` (then ` + "`breyta docs find <query>`" + ` / ` + "`breyta docs show <slug>`" + `) when the agent needs more Breyta detail

## Where to keep flow files
- ` + "`./flows/`" + `: recommended place for flow source files you want to keep (optionally in git)
- ` + "`./tmp/flows/`" + `: scratch pulls/edits

## Always-available agent context (recommended)
Many agent tools only read instructions from the active folder. This file (` + "`AGENTS.md`" + `) is the reliable source of truth.

## Optional: installed skill bundle (nice-to-have)
Some agent tools can ingest a global skill bundle automatically, but not all do.

` + skillLine + `
- (Re)install / update it with: ` + "`breyta skills install --provider " + string(target.Provider) + "`" + `

If you want the agent to *always* use the skill when Breyta is involved, explicitly mention it in your project/root instructions (this file, or a root ` + "`AGENTS.md`" + ` equivalent).

Suggested line to paste into your agent's persistent project instructions:
- "When working with Breyta, read and follow: ` + "`" + target.File + "`" + ` (Breyta skill bundle), and use the ` + "`breyta`" + ` CLI."

## Release hygiene (required)
- Iterate in ` + "`draft`" + ` while editing and debugging.
- Inspect draft changes before release: ` + "`breyta flows diff <slug>`" + `
- Do not repeatedly release to ` + "`live`" + ` during normal iteration.
- Release to ` + "`live`" + ` once after draft behavior is verified, you have explicit sign-off, and you can attach a markdown release note.
- Use ` + "`breyta flows archive <slug>`" + ` when the flow should stop appearing in the normal active surface but its versions and metadata should remain available.
- Use ` + "`breyta flows delete <slug> --yes`" + ` only for permanent removal; add ` + "`--force`" + ` when runs/installations must also be cleaned up.

## Authoring standard (required before editing)
- Write the problem contract: trigger, inputs, outputs, side effects, failure behavior.
- Write the trigger map and path map: success path, fallback path, stop path.
- Define side effects and duplicate protection before building:
  - what must happen exactly once
  - idempotency key or dedupe strategy
- Define retry/timeout policy for each external boundary before draft runs.
- Choose concurrency mode intentionally before draft runs:
  - ` + "`sequential`" + ` for ordered work, shared state, large artifacts, or fragile APIs
  - ` + "`fanout`" + ` only for independent bounded items
  - ` + "`keyed`" + ` when work must serialize per entity
- For concurrent paths, write down what must never overlap and what timeout/partial-failure behavior is acceptable.
- Decide how large resources move through the flow:
  - inline small values
  - persist large artifacts
  - pass signed URLs/blob refs for large files
- Decide what run output proves success:
  - result fields
  - counts
  - child workflow ids
  - resource refs

## Reliability checklist (required)
- Exactly-once side effects have explicit duplicate protection.
- Retries are only used for transient failures and are bounded.
- Cursors/checkpoints do not advance past failed work.
- Concurrency is intentional and bounded.
- The chosen concurrency mode is justified in plain language.
- Shared state and side effects that must not overlap are named explicitly.
- Large payloads are passed by reference, not copied through many steps.
- Step ids/titles are operator-readable and make side effects obvious.
- Final run result contains proof of success, not just a ` + "`completed`" + ` status.

## Scale-aware defaults
- Prefer sequential handling for large artifact transfer unless fanout safety is proven.
- Prefer sequential mode when uncertain; concurrency is opt-in, not the default.
- Prefer child flows for heavyweight artifact creation or handoff.
- Use blob persistence + refs for large files instead of re-shaping raw bytes across many steps.

## Authoring loop (agent-friendly, draft-first)
1) Pull: ` + "`breyta flows pull <slug> --out ./flows/<slug>.clj`" + `
2) Edit ` + "`./flows/<slug>.clj`" + `
3) Push working copy to draft target: ` + "`breyta flows push --file ./flows/<slug>.clj`" + `
4) Check required draft config: ` + "`breyta flows configure check <slug>`" + `
5) If the flow belongs to a bundle that should appear in execution order, set explicit order: ` + "`breyta flows update <slug> --group-order <n>`" + ` and confirm ordered siblings with ` + "`breyta flows show <slug> --pretty`" + `
6) Run draft target and wait for output: ` + "`breyta flows run <slug> --input '{\"n\":41}' --wait`" + `
7) Optional read-only draft check: ` + "`breyta flows validate <slug>`" + ` (useful for CI/troubleshooting)
8) Run at least one failure/no-op/replay check when feasible before release
9) If using concurrency, verify no skipped, duplicated, or overlapped work in draft output
10) Repeat steps 2-9 until behavior is correct and side effects are understood in draft
11) Inspect draft vs live before release: ` + "`breyta flows diff <slug>`" + `
12) Release once (after explicit sign-off) with a markdown note: ` + "`breyta flows release <slug> --release-note-file ./release-note.md`" + `
13) Edit the note later if needed: ` + "`breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md`" + `
14) Verify live install target: ` + "`breyta flows show <slug> --target live`" + `
15) Smoke-run live target and capture proof: ` + "`breyta flows run <slug> --target live --wait`" + `

## Provenance for derived flows
- Keep ` + "`created-by`" + ` as the creator of the current flow record.
- When a flow is derived from existing flows, store source lineage separately as provenance metadata.
- Only flows actually opened with ` + "`breyta flows show`" + ` or ` + "`breyta flows pull`" + ` become consulted provenance candidates. Search hits alone do not.
- After creating or updating a derived flow, persist curated provenance with:
  - ` + "`breyta flows provenance set <slug> --from-consulted`" + `
  - ` + "`breyta flows provenance set <slug> --source <workspace-id>/<flow-slug>`" + `
  - ` + "`breyta flows provenance set <slug> --template <template-slug>`" + `
- Clear provenance intentionally with ` + "`breyta flows provenance set <slug> --clear`" + `.

## Docs for agents
- Product docs: ` + "`breyta docs`" + ` (search with ` + "`breyta docs find \"flows push\"`" + `)
- Command truth / flags: ` + "`breyta help <command...>`" + ` (for example: ` + "`breyta help flows push`" + `)
- Installed skill bundle: ` + "`breyta skills install --provider <codex|cursor|claude|gemini>`" + `

## Recovery URLs (when commands fail)
- Prefer exact recovery URLs from failures: ` + "`error.actions[].url`" + ` first, then ` + "`meta.webUrl`" + `.
- For successful reads/runs, include web links from CLI JSON (` + "`meta.webUrl`" + ` / ` + "`data.*.webUrl`" + `) when handing proof back to users.
- Only derive canonical recovery URLs when the needed ids are already known: billing, activate, draft-bindings, installation, or connection edit.
- When blocked, include the exact recovery URL in runtime proof instead of generic "go to billing/setup" text.
`)
}

func renderInitReadmeMD() string {
	return strings.TrimSpace(`# Breyta first session

This directory was created by ` + "`breyta init`" + ` for your first Breyta CLI session with a coding agent.

## Execution rules
- Assume sandboxed and network-restricted agent environments by default.
- Use elevated permissions for internet, API, browser, auth, or download steps when needed.
- Verify each step before moving on.
- Never paste API keys or secrets into chat or CLI commands.
- Route flow secrets and activation through Breyta UI draft-bindings and activate pages.
- Stop after idea exploration by default, but you can intentionally skip ahead if you know what you are doing.

## Recommended first session
1. Verify the CLI install: ` + "`breyta version`" + `
2. Open this folder in your agent tool (or ` + "`cd`" + ` into it) and restart the agent if needed so ` + "`AGENTS.md`" + ` is loaded.
3. Authenticate: ` + "`breyta auth login`" + `
   - If you need an account first, open ` + "`https://flows.breyta.ai/signup`" + `, finish sign-up/sign-in, then rerun login.
4. Verify identity + workspace summary: ` + "`breyta auth whoami`" + `
   - If ` + "`whoami`" + ` shows multiple workspaces or no default workspace selected, keep discovering first. Use ` + "`breyta workspaces list`" + ` and ` + "`breyta workspaces use <workspace-id>`" + ` later when you are ready to adopt or build.
5. Discover approved templates:
   - ` + "`breyta flows search`" + `
   - ` + "`breyta flows search \"<idea>\"`" + `
6. Pick one idea to explore next.

Easy ideas:
- Scheduled API digest that posts a summary to Slack or email
- Webhook intake flow that classifies and routes inbound events
- Manual enrichment flow for CSV rows, CRM records, or support tickets

Advanced ideas:
- Keyed-concurrency webhook processor with duplicate protection
- Scheduled reconciliation flow with retries, checkpoints, and no-op handling
- Multi-step research flow with persisted resources and a final summary artifact

## Stop gate
- Stop here by default after idea exploration.
- Do not push, validate, release, or configure secrets until you have a chosen idea and explicit ` + "`continue`" + ` from the user.
- If you intentionally want to skip the stop gate, do it knowingly.

## After the stop gate
- Start with approved template discovery and docs:
  - ` + "`breyta flows search \"<chosen idea>\"`" + `
  - ` + "`breyta docs find \"<chosen idea or primitive>\"`" + `
- Keep editable flow source files in ` + "`./flows/`" + `
- Iterate in draft: pull, edit, push, configure check, run or validate, then diff against live
- If a flow belongs to a sequential group, set explicit order with ` + "`breyta flows update <slug> --group-order <n>`" + ` and verify ordered siblings with ` + "`breyta flows show <slug> --pretty`" + `
- If the flow was derived from other flows or public templates, persist curated lineage with ` + "`breyta flows provenance set <slug> --from-consulted`" + `, ` + "`--source`" + `, or ` + "`--template`" + `
- Release once to live after draft is verified and approved, using ` + "`breyta flows release <slug> --release-note-file ./release-note.md`" + `
- Archive flows you want to retire without removing their history: ` + "`breyta flows archive <slug>`" + `
- Delete flows only for permanent cleanup: ` + "`breyta flows delete <slug> --yes`" + ` (add ` + "`--force`" + ` to cancel runs/delete installations)

## Recovery URLs
- When a command fails, prefer the exact page from ` + "`error.actions[].url`" + ` first, then ` + "`meta.webUrl`" + `.
- For successful reads or runs, carry forward ` + "`meta.webUrl`" + ` / ` + "`data.*.webUrl`" + ` when sharing proof.
- Only derive canonical URLs when the required ids are already known: billing, activate, draft-bindings, installation, or connection edit.

## Docs
- Product docs: ` + "`breyta docs`" + `
- Command help: ` + "`breyta help <command...>`" + `
`)
}
