package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/breyta/breyta-cli/internal/format"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type docFlag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Usage       string `json:"usage"`
	DefValue    string `json:"default,omitempty"`
	Deprecated  string `json:"deprecated,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	Persistent  bool   `json:"persistent,omitempty"`
	FlagType    string `json:"type,omitempty"`
	NoOptDefVal string `json:"noOptDefault,omitempty"`
}

type docCommand struct {
	Use         string       `json:"use"`
	Aliases     []string     `json:"aliases,omitempty"`
	Short       string       `json:"short,omitempty"`
	Long        string       `json:"long,omitempty"`
	Example     string       `json:"example,omitempty"`
	Flags       []docFlag    `json:"flags,omitempty"`
	Subcommands []docCommand `json:"subcommands,omitempty"`
}

func newDocsCmd(root *cobra.Command, app *App) *cobra.Command {
	var outFormat string
	var full bool
	cmd := &cobra.Command{
		Use:   "docs [command...]",
		Short: "On-demand docs for commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			if outFormat == "" {
				outFormat = "md"
			}

			// Docs are allowed to target hidden commands, even when the default CLI
			// surface is minimized (non-dev mode). This keeps docs stable for agents
			// and internal tooling without expanding the user-visible help surface.
			target, path, err := findCommandByPath(root, args, true)
			if err != nil {
				return writeErr(cmd, err)
			}

			// No args => index page.
			if target == root && len(args) == 0 {
				md := renderDocsIndexMD(root, app.DevMode)
				_, _ = io.WriteString(cmd.OutOrStdout(), md)
				return nil
			}

			switch outFormat {
			case "md", "markdown":
				md := renderCommandDocsMD(target, path, app.DevMode)
				_, _ = io.WriteString(cmd.OutOrStdout(), md)
				return nil
			case "json":
				d := docCommandFrom(target, full, app.DevMode)
				return format.Write(cmd.OutOrStdout(), map[string]any{
					"ok":          true,
					"workspaceId": app.WorkspaceID,
					"meta": map[string]any{
						"path": path,
						"hint": "Pass `breyta docs <command> --format md` for human-readable docs; use `--full` to include recursive subcommands.",
					},
					"data": map[string]any{
						"command": d,
					},
				}, outFormat, true)
			default:
				return writeErr(cmd, fmt.Errorf("unknown docs format: %s", outFormat))
			}
		},
	}
	cmd.Flags().StringVar(&outFormat, "format", "md", "Docs format (md|json)")
	cmd.Flags().BoolVar(&full, "full", false, "Include recursive subcommands in structured docs")
	return cmd
}

func docCommandFrom(c *cobra.Command, full bool, includeHidden bool) docCommand {
	out := docCommand{
		Use:     c.Use,
		Aliases: append([]string{}, c.Aliases...),
		Short:   c.Short,
		Long:    strings.TrimSpace(c.Long),
		Example: strings.TrimSpace(c.Example),
	}

	out.Flags = append(out.Flags, docFlags(c.InheritedFlags(), true, includeHidden)...)     // persistent inherited
	out.Flags = append(out.Flags, docFlags(c.NonInheritedFlags(), false, includeHidden)...) // local

	if full {
		subs := c.Commands()
		out.Subcommands = make([]docCommand, 0, len(subs))
		for _, sc := range subs {
			if sc.IsAvailableCommand() {
				out.Subcommands = append(out.Subcommands, docCommandFrom(sc, full, includeHidden))
			}
		}
	} else {
		// Only direct subcommands (summary) to avoid huge payloads.
		subs := c.Commands()
		out.Subcommands = make([]docCommand, 0, len(subs))
		for _, sc := range subs {
			if sc.IsAvailableCommand() {
				out.Subcommands = append(out.Subcommands, docCommand{
					Use:     sc.Use,
					Aliases: append([]string{}, sc.Aliases...),
					Short:   sc.Short,
				})
			}
		}
	}
	return out
}

func docFlags(fs *pflag.FlagSet, persistent bool, includeHidden bool) []docFlag {
	items := []docFlag{}
	if fs == nil {
		return items
	}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden && !includeHidden {
			return
		}
		items = append(items, docFlag{
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			Usage:       f.Usage,
			DefValue:    f.DefValue,
			Deprecated:  f.Deprecated,
			Hidden:      f.Hidden,
			Persistent:  persistent,
			FlagType:    f.Value.Type(),
			NoOptDefVal: f.NoOptDefVal,
		})
	})
	return items
}

func findCommandByPath(root *cobra.Command, args []string, allowHidden bool) (*cobra.Command, string, error) {
	cur := root
	path := cur.Name()
	for _, tok := range args {
		nxt := findDirectSubcommand(cur, tok, allowHidden)
		if nxt == nil {
			return nil, "", fmt.Errorf("unknown command: %s (try `breyta docs` for index)", strings.Join(args, " "))
		}
		cur = nxt
		path = path + " " + cur.Name()
	}
	return cur, path, nil
}

func findDirectSubcommand(parent *cobra.Command, tok string, allowHidden bool) *cobra.Command {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return nil
	}
	for _, c := range parent.Commands() {
		if !allowHidden && !c.IsAvailableCommand() {
			continue
		}
		if c.Name() == tok {
			return c
		}
		for _, a := range c.Aliases {
			if a == tok {
				return c
			}
		}
	}
	return nil
}

func renderDocsIndexMD(root *cobra.Command, devMode bool) string {
	var b strings.Builder
	b.WriteString("## Breyta CLI docs\n\n")
	b.WriteString("This is on-demand documentation intended for agents and humans.\n\n")
	b.WriteString("### How to use\n\n")
	b.WriteString("- `breyta docs <command...>` prints Markdown docs for that command\n")
	b.WriteString("- `breyta <command...> --help` prints Cobra help for that command\n")
	b.WriteString("- For structured docs: `breyta docs <command...> --format json`\n\n")

	b.WriteString("### End-user installations (marketplace MVP)\n\n")
	b.WriteString("End-user-facing flows are marked with the `:end-user` tag.\n\n")
	b.WriteString("- Create installation: `breyta flows installations create <flow-slug> --name \"My installation\"`\n")
	b.WriteString("- Show installation: `breyta flows installations get <profile-id>`\n")
	b.WriteString("- Set activation inputs: `breyta flows installations set-inputs <profile-id> --input '{\"region\":\"EU\"}'`\n")
	b.WriteString("- Pause/resume: `breyta flows installations disable <profile-id>` / `breyta flows installations enable <profile-id>`\n")
	b.WriteString("- Delete installation: `breyta flows installations delete <profile-id>`\n")
	b.WriteString("- Run under installation: `breyta runs start --flow <flow-slug> --profile-id <profile-id> --input '{\"x\":1}' --wait`\n\n")

	b.WriteString("### Credentials / API keys for flows\n\n")
	b.WriteString("Flows execute inside the Breyta server. There are two ways credentials can be provided:\n\n")
	b.WriteString("- **Recommended (per-user / production-like)**: declare `:requires` slots (e.g. `:llm-provider`, `:http-api`) and bind credentials via the UI or CLI, then activate the profile.\n")
	b.WriteString("  Slot names must be non-namespaced keywords (e.g., `:api`, not `:ns/api`).\n")
	b.WriteString("  Manual trigger and wait notify field names use non-namespaced keywords (e.g., `{:name :user-id ...}`).\n")
	if devMode {
		b.WriteString("- **Dev-only (server-global)**: create `secrets.edn` (gitignored) to provide dev keys directly to the server process.\n\n")
		b.WriteString("Dev-only `secrets.edn`:\n")
		b.WriteString("- `cp breyta/secrets.edn.example secrets.edn`\n")
		b.WriteString("- Add the keys you need and restart `flows-api`\n")
		b.WriteString("- Never commit `secrets.edn`\n\n")
		b.WriteString("Dev mode exposes local override flags and env vars for authenticating the CLI to a local `flows-api`.\n\n")
	}

	b.WriteString("### Bindings (credentials for `:requires` slots)\n\n")
	b.WriteString("If a flow declares `:requires` slots (e.g. `:http-api` with `:auth`/`:oauth`, or `:llm-provider`), you must apply bindings to create/update a profile, then enable it.\n")
	b.WriteString("Slot names must be non-namespaced keywords (e.g., `:api`, not `:ns/api`).\n\n")
	b.WriteString("Symptom if you forget: \"Slot reference requires a flow profile, but no profile-id in context\".\n\n")
	b.WriteString("Prod workflow:\n")
	b.WriteString("- Generate a template: `breyta flows bindings template <slug> --out profile.edn`\n")
	b.WriteString("- Apply bindings with the profile file: `breyta flows bindings apply <slug> @profile.edn`\n")
	b.WriteString("- Promote draft bindings: `breyta flows bindings apply <slug> --from-draft`\n")
	b.WriteString("- Show bindings status: `breyta flows bindings show <slug>`\n")
	b.WriteString("- Enable prod profile: `breyta flows activate <slug> --version latest`\n")
	b.WriteString("- Use the UI for OAuth flows if required\n")
	b.WriteString("- Re-run the flow (CLI `runs start` will then resolve slots via the active profile)\n\n")
	b.WriteString("Draft workflow:\n")
	b.WriteString("- Generate a draft template: `breyta flows draft bindings template <slug> --out draft.edn`\n")
	b.WriteString("- Apply draft bindings: `breyta flows draft bindings apply <slug> @draft.edn`\n")
	b.WriteString("- Show draft bindings status: `breyta flows draft bindings show <slug>`\n")
	b.WriteString("- Run draft: `breyta flows draft run <slug> --input '{\"n\":41}' --wait`\n\n")
	b.WriteString("- Reset draft (clear draft + draft profiles): `breyta flows draft reset <slug>`\n\n")
	b.WriteString("Activation defaults to `latest`; use `--version <n>` to pin a specific flow version.\n\n")
	b.WriteString("Bindings apply validates required slots and activation inputs; missing entries are reported in error details.\n\n")
	b.WriteString("Template discovery:\n")
	b.WriteString("- `breyta flows bindings template --help` shows template flags\n")
	b.WriteString("- Templates prefill current bindings by default; add `--clean` for a requirements-only template\n")
	b.WriteString("- Template commands print the activation URL to stderr for OAuth flows\n\n")
	b.WriteString("Templates prefill current connection bindings (`:conn`) by default; use `--clean` for a requirements-only template.\n\n")
	b.WriteString("Draft bindings (authoring):\n")
	b.WriteString("- Template: `breyta flows draft bindings template <slug> --out draft.edn`\n")
	b.WriteString("- Apply: `breyta flows draft bindings apply <slug> @draft.edn`\n")
	b.WriteString("- Status: `breyta flows draft bindings show <slug>`\n\n")
	b.WriteString("Inline bindings (no file):\n")
	b.WriteString("- `breyta flows bindings apply <slug> --set api.apikey=... --set activation.region=EU`\n")
	b.WriteString("- `breyta flows bindings apply <slug> --from-draft`\n")
	b.WriteString("- `breyta flows draft bindings apply <slug> --set api.apikey=...`\n\n")
	b.WriteString("Notes:\n")
	b.WriteString("- If you set both `slot.conn` and `slot.apikey`, the API key refreshes the existing connection secret while keeping the binding.\n\n")
	b.WriteString("Profile file example:\n\n")
	b.WriteString("```edn\n")
	b.WriteString("{:profile {:type :prod\n           :autoUpgrade false}\n\n")
	b.WriteString(" :bindings {:api {:name \"Users API\"\n                  :url \"https://api.example.com\"\n                  :apikey :redacted}}\n\n")
	b.WriteString(" :activation {:region \"EU\"}}\n")
	b.WriteString("```\n")
	b.WriteString("Notes:\n")
	b.WriteString("- `:redacted` values are ignored when sending the profile to the server (API keys).\n")
	b.WriteString("- `:generate` values are ignored when sending the profile to the server (secrets).\n")
	b.WriteString("- Templates include comments that list OAuth and secret slots.\n\n")

	if devMode {
		b.WriteString("### Draft preview bindings\n\n")
		b.WriteString("Draft runs use a user-scoped draft profile and require draft bindings.\n\n")
		b.WriteString("- Use: `breyta flows draft-bindings-url <slug>` to print the URL\n")
		b.WriteString("- Run draft: `breyta runs start --flow <slug> --source draft`\n\n")
	}

	b.WriteString("### Flow body constraints (SCI / orchestration DSL)\n\n")
	b.WriteString("Flow bodies are intentionally constrained to keep the \"flow language\" small for visualization and translation (engine-agnostic orchestration), and to reduce the security surface area.\n\n")
	b.WriteString("Practical consequences:\n")
	b.WriteString("- Many everyday Clojure functions are denied in the flow body (e.g. `mapv`, `filterv`, `reduce`, etc.)\n")
	b.WriteString("- Keep the flow body focused on orchestration (a sequence of `step` calls)\n")
	b.WriteString("- Put data transformation into explicit `:function` steps (`:code` alias)\n\n")

	b.WriteString("### Input keys from `--input` (string vs keyword keys)\n\n")
	b.WriteString("`breyta runs start --input '{...}'` sends JSON, so keys arrive as strings.\n\n")
	b.WriteString("Cancel a run with `breyta runs cancel <workflow-id> --reason \"...\"`.\n")
	b.WriteString("Use `--force` to terminate a run immediately.\n\n")
	b.WriteString("The runtime normalizes input so both string keys and keyword keys work (safe keyword aliases are added).\n\n")

	b.WriteString("### Top-level commands\n\n")
	for _, c := range root.Commands() {
		if !c.IsAvailableCommand() {
			continue
		}
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		b.WriteString("- `" + c.Name() + "`: " + strings.TrimSpace(c.Short) + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func renderCommandDocsMD(c *cobra.Command, path string, includeHidden bool) string {
	var b strings.Builder
	b.WriteString("## " + path + "\n\n")
	if c.Short != "" {
		b.WriteString(strings.TrimSpace(c.Short) + "\n\n")
	}
	if strings.TrimSpace(c.Long) != "" {
		b.WriteString(strings.TrimSpace(c.Long) + "\n\n")
	}

	b.WriteString("### Usage\n\n")
	b.WriteString("`" + c.UseLine() + "`\n\n")

	flags := docFlags(c.InheritedFlags(), true, includeHidden)
	flags = append(flags, docFlags(c.NonInheritedFlags(), false, includeHidden)...)
	if len(flags) > 0 {
		b.WriteString("### Flags\n\n")
		for _, f := range flags {
			name := "--" + f.Name
			if f.Shorthand != "" {
				name = "-" + f.Shorthand + ", " + name
			}
			line := "- `" + name + "`: " + f.Usage
			if f.DefValue != "" {
				line += " (default: `" + f.DefValue + "`)"
			}
			if f.Deprecated != "" {
				line += " **DEPRECATED**: " + f.Deprecated
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	if strings.TrimSpace(c.Example) != "" {
		b.WriteString("### Examples\n\n")
		b.WriteString("```bash\n" + strings.TrimSpace(c.Example) + "\n```\n\n")
	}

	subs := []string{}
	for _, sc := range c.Commands() {
		if sc.IsAvailableCommand() {
			subs = append(subs, sc.Name())
		}
	}
	if len(subs) > 0 {
		b.WriteString("### Subcommands\n\n")
		for _, sc := range c.Commands() {
			if !sc.IsAvailableCommand() {
				continue
			}
			b.WriteString("- `" + sc.Name() + "`: " + strings.TrimSpace(sc.Short) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("### Notes for agents\n\n")
	b.WriteString("- Prefer requesting small payloads first; many commands support `meta`/`hint` for expanding results.\n")
	b.WriteString("- If a command returns truncated lists, follow the `meta.hint` guidance to fetch the full data.\n\n")

	return b.String()
}
