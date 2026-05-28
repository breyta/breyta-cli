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
	var withMCP bool
	var mcpProvider string
	var mcpTransport string
	var mcpWorkspaceID string
	var mcpName string
	var mcpTokenEnvVar string
	var mcpToolsets string
	var mcpReadOnly bool

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
			if noSkill && noWorkspace && !withMCP {
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
						warnDuplicateBreytaSkills(cmd, home, p)
					}
				}
			}

			if noWorkspace {
				if withMCP {
					return writeInitMCPConfig(cmd, app, mcpSetupOptions{
						Provider:    mcpProvider,
						Transport:   mcpTransport,
						ServerName:  mcpName,
						WorkspaceID: mcpWorkspaceID,
						TokenEnvVar: mcpTokenEnvVar,
						Policy: mcpPolicyOptions{
							Toolsets: mcpToolsets,
							ReadOnly: mcpReadOnly,
						},
					})
				}
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
			if err := makePublicDir(absDir); err != nil {
				return err
			}

			if err := makePublicDir(filepath.Join(absDir, "flows")); err != nil {
				return err
			}
			if err := makePublicDir(filepath.Join(absDir, "tmp", "flows")); err != nil {
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
			fmt.Fprintln(cmd.OutOrStdout(), "- Inventory reusable connections: breyta connections list")
			fmt.Fprintln(cmd.OutOrStdout(), "- Search nearby workspace flow patterns: breyta flows search \"<integration or problem query>\" --limit 5")
			fmt.Fprintln(cmd.OutOrStdout(), "- Search workspace source/config literals: breyta flows grep \"<literal>\" --or \"<variant>\" --limit 5")
			fmt.Fprintln(cmd.OutOrStdout(), "- Search docs: breyta docs find \"<idea or primitive>\" --limit 5 --format json")
			fmt.Fprintln(cmd.OutOrStdout(), "- Search approved templates: breyta flows templates search \"<problem or integration query>\" --limit 5")
			fmt.Fprintln(cmd.OutOrStdout(), "- Stop after idea exploration unless you intentionally want to continue now")
			if withMCP {
				return writeInitMCPConfig(cmd, app, mcpSetupOptions{
					Provider:    mcpProvider,
					Transport:   mcpTransport,
					ServerName:  mcpName,
					WorkspaceID: mcpWorkspaceID,
					TokenEnvVar: mcpTokenEnvVar,
					Policy: mcpPolicyOptions{
						Toolsets: mcpToolsets,
						ReadOnly: mcpReadOnly,
					},
				})
			}
			return nil
		},
	}

	_ = app
	cmd.Flags().StringVar(&provider, "provider", string(skills.ProviderCodex), "Install location for the agent skill bundle (codex|cursor|claude|gemini)")
	cmd.Flags().StringVar(&dir, "dir", "breyta-workspace", "Workspace directory to create (contains AGENTS.md, flows/, tmp/flows/)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files in the workspace directory")
	cmd.Flags().BoolVar(&noSkill, "no-skill", false, "Skip installing the agent skill bundle")
	cmd.Flags().BoolVar(&noWorkspace, "no-workspace", false, "Skip creating the workspace directory and files")
	cmd.Flags().BoolVar(&withMCP, "mcp", false, "Also print workspace-bound MCP client config")
	cmd.Flags().StringVar(&mcpProvider, "mcp-provider", "generic", "MCP config provider (generic|codex|claude|cursor|vscode|windsurf|cline|roo|continue|gemini|opencode|zed|goose|acp)")
	cmd.Flags().StringVar(&mcpTransport, "mcp-transport", "stdio", "MCP transport to render (stdio|http)")
	cmd.Flags().StringVar(&mcpWorkspaceID, "mcp-workspace-id", "", "Workspace id to bind the MCP server to")
	cmd.Flags().StringVar(&mcpName, "mcp-name", defaultMCPServerName, "MCP server entry name")
	cmd.Flags().StringVar(&mcpTokenEnvVar, "mcp-token-env-var", defaultMCPTokenEnvVar, "Environment variable containing the service-account API key")
	cmd.Flags().StringVar(&mcpToolsets, "mcp-toolsets", "", "Comma-separated MCP toolsets to expose")
	cmd.Flags().BoolVar(&mcpReadOnly, "mcp-read-only", false, "Expose only read-only MCP tools")
	return cmd
}

func writeInitMCPConfig(cmd *cobra.Command, app *App, opts mcpSetupOptions) error {
	resolvedWorkspace, err := explicitMCPWorkspaceID(cmd, app, opts.WorkspaceID)
	if err != nil {
		return writeErr(cmd, err)
	}
	ensureAPIURL(app)
	opts.WorkspaceID = resolvedWorkspace
	opts.APIURL = app.APIURL
	rendered, err := renderMCPClientConfig(opts)
	if err != nil {
		return writeErr(cmd, err)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "MCP config (%s/%s, workspace %s):\n", strings.TrimSpace(opts.Provider), strings.TrimSpace(opts.Transport), resolvedWorkspace)
	fmt.Fprintln(cmd.OutOrStdout(), rendered)
	return nil
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
	if err := makePublicDir(filepath.Dir(path)); err != nil {
		return false, err
	}
	return true, writePublicFile(path, content)
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

This folder is for coding agents building Breyta flows with the ` + "`breyta`" + ` CLI.
Start with ` + "`README.md`" + ` in this folder on the first session.

## Context hierarchy

This ` + "`AGENTS.md`" + ` is the always-loaded baseline. Keep it short. The
installed skill has the full Breyta playbook and should be loaded only when the
task touches flows.

Keep durable discovery minimal:
- Pick a task mode before commands: existing-flow edit, new flow, primitive/step edit, debug run, public publish/install, output/table, provider/API, n8n import, or release.
- Inspect the smallest current state first; then use workspace search/grep, docs, and approved templates at the primitive level.
- Verify identity/workspace with ` + "`breyta auth whoami`" + ` when workspace state matters.
- For n8n workflow JSON imports, use ` + "`breyta flows import n8n <workflow.json>`" + ` first; do not hand-write the initial EDN conversion unless the importer is unavailable or explicitly bypassed.

Keep flow files in ` + "`./flows/`" + ` for durable source and ` + "`./tmp/flows/`" + ` for scratch pulls/edits.

` + skillLine + `
- Install/update: ` + "`breyta skills install --provider all`" + `, or one provider such as ` + "`--provider " + string(target.Provider) + "`" + `
- Drift check: ` + "`breyta skills status --provider all`" + `
- If the CLI warns that the skill is missing or stale, install/refresh before material flow edits.
  - Read ` + "`SKILL.md`" + ` first; load only the ` + "`playbooks/`" + ` or ` + "`references/`" + ` file named for the touched surface.

## Default flow loop

1. Verify auth/workspace: ` + "`breyta auth whoami`" + `
2. Pick task mode: existing-flow edit, new flow, primitive/step edit, debug run, public publish/install, output/table, provider/API, or release.
3. Use the right source mental model: flow files are a Breyta DSL with Clojure/EDN syntax, not normal Clojure programs. Declare contracts/reusable surfaces first, keep side effects at ` + "`flow/step`" + ` boundaries, and persist large data as resource refs.
4. Inspect the smallest state:
   - existing: ` + "`breyta flows show <slug>`" + ` or ` + "`breyta flows pull <slug>`" + `
   - new/pattern search: ` + "`breyta flows search \"<query>\" --limit 5`" + `
5. Search narrowly:
   - docs: ` + "`breyta docs find \"<query>\" --limit 5 --format json`" + `
   - source/config: ` + "`breyta flows grep \"<literal>\" --limit 5`" + `
   - templates: ` + "`breyta flows templates search \"<query>\" --limit 5`" + `
   - resources/data: ` + "`breyta resources search \"<query>\" --limit 5`" + `
6. Build in small slices: contract -> manual interface -> one boundary -> lint -> push -> configure-check -> run -> inspect output.
7. Keep source installable-minded: no hardcoded workspace IDs, user emails, secrets, private URLs, or author-only resource IDs.
8. Persist large or unknown payloads with ` + "`:persist`" + ` and pass resource refs, not large inline bodies. For blob persists, choose retained/default for durable or user-visible artifacts and ` + "`:tier :ephemeral`" + ` on streaming ` + "`:http`" + ` steps for temporary downloads, exports, generated media, or API response blobs that only need short-lived workflow consumption.
9. Keep functions map-oriented; prefer Clojure map access plus ` + "`json/*`" + ` and ` + "`breyta.sandbox/*`" + ` helpers over custom parser/guard layers.

## Draft/live and release

- ` + "`draft`" + ` is staging/current authoring; ` + "`live`" + ` is released/runtime.
- A flow is unreleased until a version is released/activated and the live path is verified.
- Say ` + "`draft verified`" + ` when only draft was exercised.
- Inspect draft changes with ` + "`breyta flows diff <slug>`" + ` before release.
- Release/promote only after draft proof, explicit sign-off, and a release note.
- For public/end-user work, verify live/install-shaped behavior or report ` + "`web UI not verified`" + `.

## Command budget

- Do not repeat identical commands unless state changed.
- Use one docs search per changed primitive before opening docs pages.
- Use one full template inspection at most for normal create/edit work.
- Read each resource URI once unless the resource changed.
- After two failed edit/run cycles, stop and re-plan.
- Do not run ` + "`breyta connections test --all`" + `; test only the connection you will bind/debug.

## Proof and handoff

- Include workspace/template queries run, chosen/rejected snippets or templates, and reused structure.
- Confirm side effects/output directly, not from ` + "`completed`" + ` status alone.
- Include full Breyta URLs from CLI JSON (` + "`meta.webUrl`" + `, ` + "`data.*.webUrl`" + `, ` + "`outputWebUrl`" + `).
- Prefer recovery URLs from failures: ` + "`error.actions[].url`" + `, then ` + "`meta.webUrl`" + `.
- Submit ` + "`breyta feedback send --agent`" + ` when flow development hits significant authoring friction: excessive trial/error, misleading docs/help, unclear CLI/API behavior, blocked command paths, or missing examples. Include commands tried, URLs/workflow ids, expected path, and impact.
- For public/installable flows, keep source flow, live version, activation/setup, Discover install, marketplace visibility, installed run, public app page, and output page separate.
- For paid apps, author pricing in source under ` + "`:marketplace {:app ... :monetization {:plans [...]}}`" + `. New paid apps should use app-owned plan catalogs; preserve legacy flow-level monetization only for existing listings.
- Seat-based pricing is not implemented; do not describe a paid app plan as N seats or N installs unless explicit seat entitlements exist.
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
5. Inventory reusable connections:
   - ` + "`breyta connections list`" + `
   - ` + "`breyta connections show <id>`" + ` for the connection you expect to bind
   - ` + "`breyta connections test <id>`" + ` only when you plan to bind or debug that connection
6. Pick a task mode and search nearby workspace flow patterns, docs, and approved templates:
   - ` + "`breyta flows search \"<integration or problem query>\" --limit 5`" + `
   - ` + "`breyta flows grep \"<literal>\" --or \"<variant>\" --limit 5`" + `
   - ` + "`breyta flows workspace examples step <type> \"<query>\" --limit 3`" + `
   - ` + "`breyta docs find \"<idea or primitive>\" --limit 5 --format json`" + `
   - ` + "`breyta flows templates search \"<problem or integration query>\" --limit 5`" + `
   - ` + "`breyta resources search \"<existing data>\" --limit 5`" + ` when prior reports, uploads, or run outputs may be reused
7. Keep reuse primitive-first:
   - use matching primitive snippets and referenced dependencies when available
   - inspect one full template only for architecture-level reuse, public install patterns, multi-flow orchestration, fanout/child-flow behavior, unclear snippet dependencies, or copying overall flow structure
8. Pick one idea to explore next.

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
- Start with current state, workspace search/grep, docs snippets, and approved template discovery:
  - new flow: search nearby workspace patterns with ` + "`breyta flows search \"<integration or problem query>\" --limit 5`" + `
  - source/config lookup: use ` + "`breyta flows grep \"<literal>\" --or \"<variant>\" --limit 5`" + `
  - primitive reuse: extract local snippets with ` + "`breyta flows workspace examples step <type> \"<query>\" --limit 3`" + `
  - existing flow: inspect it with ` + "`breyta flows show <slug>`" + ` or ` + "`breyta flows pull <slug>`" + `
  - ` + "`breyta docs find \"<chosen idea or primitive>\" --limit 5 --format json`" + `
  - ` + "`breyta flows templates search \"<problem or integration query>\" --limit 5`" + `
  - existing data lookup: use ` + "`breyta resources search \"<query>\" --limit 5`" + ` and ` + "`breyta resources read <resource-uri> --limit 5`" + ` for one selected resource
  - use primitive snippets and referenced dependencies before a full template
  - inspect one full template only when architecture-level reuse is needed
  - compare the touched surface against the closest local or approved template example before changing structure
  - final handoff should list workspace/template queries, chosen/rejected snippets or templates, and reused/ignored structure
- Then inventory targeted reusable connections before authoring behavior:
  - ` + "`breyta connections list`" + `
  - ` + "`breyta connections show <id>`" + ` for the connection you expect to bind
  - ` + "`breyta connections test <id>`" + ` only when you plan to bind or debug that connection
- For first proof, inline small one-off transform bodies when that is fastest; before release, extract repeated or bulky content into ` + "`:templates`" + `, ` + "`:functions`" + `, packaged ` + "`:steps`" + `, or reusable ` + "`:agents`" + `
- Shape ` + "`:requires`" + ` around stable capability slots before binding reusable surfaces and the final ` + "`:flow`" + `
- Keep editable flow source files in ` + "`./flows/`" + `
- Iterate in draft: pull, edit, lint, push, configure check, run or validate, then diff against live
- Run ` + "`breyta flows lint --file ./flows/<slug>.clj`" + ` before push; use ` + "`--local-only`" + ` for offline checks, ` + "`--server`" + ` when canonical pre-push checks matter, and ` + "`--timeout <duration>`" + ` when server lint needs a longer bound
- Treat failed configure checks as a hard stop before draft/live runs unless the task is static validation only
- Authoring reads are compact by default. Use ` + "`--full`" + ` on ` + "`flows show`" + `, ` + "`flows diff`" + `, or ` + "`runs show`" + ` only when you need source, full diff text, steps, or result payloads. Use ` + "`flows pull`" + ` for editable source.
- ` + "`breyta resources read <uri>`" + ` defaults to compact blob previews and bounded table row/cell previews. Use ` + "`--full`" + ` only when the full resource payload is required.
- Treat ` + "`--pretty`" + ` as formatting only; it must not imply full payload access.
- For large reports and research artifacts, persist the full body as a resource and move refs, URLs, short summaries, and previews through tables or run output.
- For intermediate blobs, choose the storage tier deliberately: retained/default for durable or user-visible artifacts; ` + "`:persist {:type :blob :tier :ephemeral}`" + ` on streaming HTTP steps for temporary downloads, exports, generated media, and API responses that should use the more generous transient quota.
- If a flow belongs to a sequential group, set explicit order with ` + "`breyta flows update <slug> --group-order <n>`" + ` and verify ordered siblings with ` + "`breyta flows show <slug>`" + ` so ` + "`groupFlows`" + ` is visible
- If the flow should look polished in public discover/install surfaces, set curated media with ` + "`breyta flows update <slug> --publish-media-type image --publish-media-source-file ./screenshot.png`" + `, use an HTTPS media source, or author ` + "`:publish-media`" + ` in the flow file
- For paid apps, author pricing in source under ` + "`:marketplace {:app ... :monetization {:plans [...]}}`" + `. Supported plan price types are ` + "`free`" + `, ` + "`one-time`" + `, ` + "`subscription`" + `, ` + "`usage`" + `, and ` + "`subscription-usage`" + `; subscription intervals are ` + "`month`" + ` or ` + "`year`" + `; usage quantities are run-based with ` + "`:unit \"run\"`" + ` and ` + "`:included-quantity`" + `.
- New paid apps should use app-owned plan catalogs. Use legacy flow-level monetization only to preserve existing legacy listings.
- Seat-based pricing is not implemented; do not describe a paid app plan as N seats or N installs unless explicit seat entitlements exist.
- If the flow was derived from other flows or public templates, persist curated lineage with ` + "`breyta flows provenance set <slug> --from-consulted`" + `, ` + "`--source`" + `, or ` + "`--template`" + `
- Release once to live after draft is verified and approved, using ` + "`breyta flows release <slug> --release-note-file ./release-note.md`" + `
- Do not call a public/end-user flow "ready for UI" from draft CLI proof alone; verify live/install-shaped behavior or report ` + "`web UI not verified`" + ` in the risk ledger
- For installable/public flows, do not stop at activation; verify Discover install plus an installed run. The CLI path is installation create/configure/enable plus ` + "`breyta flows run <slug> --installation-id <installation-id> --wait`" + `.
- For paid apps, draft runs and owner activation checks are not enough; verify checkout or trial entry, install handoff, installed run behavior, billing state, and exhausted/remediation state when relevant.
- When browser/UI access is available, test the actual Discover install dialog, setup page, run form fields, upload CSV or file flow, resource picker, and output page
- Archive flows you want to retire without removing their history: ` + "`breyta flows archive <slug>`" + `
- Delete flows only for permanent cleanup: ` + "`breyta flows delete <slug> --yes`" + ` (add ` + "`--force`" + ` to cancel runs/delete installations; add ` + "`--timeout 5m`" + ` for large cleanup jobs)

## Recovery URLs
- When a command fails, prefer the exact page from ` + "`error.actions[].url`" + ` first, then ` + "`meta.webUrl`" + `.
- For successful reads or runs, carry forward ` + "`meta.webUrl`" + ` / ` + "`data.*.webUrl`" + ` when sharing proof.
- Only derive canonical URLs when the required ids are already known: billing, activate, draft-bindings, installation, or connection edit.

## Docs
- Product docs: ` + "`breyta docs`" + `
- Search patterns to avoid guessing:
  - step config overview or selected fields: ` + "`breyta docs fields http response-as persist --format json`" + `; add ` + "`--section <heading>`" + ` for operation-specific tables
  - primitive name: ` + "`breyta docs find \"files materialize\" --limit 5 --format json`" + `
  - exact phrase: ` + "`breyta docs find \"\\\"draft setup\\\"\" --limit 5 --format json`" + `
  - command path: ` + "`breyta docs find \"source:cli flows configure check\" --limit 5 --format json`" + `
  - API/runtime source: ` + "`breyta docs find \"source:flows-api agent definitions\" --limit 5 --format json`" + `
  - error text: ` + "`breyta docs find \"\\\"Bad credentials\\\"\" --limit 5 --format json`" + `
  - then open only the best narrow hit with ` + "`breyta docs show <slug> --section \"<heading>\"`" + `; use ` + "`--full`" + ` only when the complete page is required
- External provider/API truth: check current official provider docs/API references or model-list endpoints before choosing model ids, endpoints, request shapes, auth assumptions, or limits.
- OpenAI connection default: ` + "`:http-api`" + ` requirement, backend ` + "`openai`" + `, base URL ` + "`https://api.openai.com/v1`" + `, API-key auth, and a non-null config map.
- Command help: ` + "`breyta help <command...>`" + `
`)
}
