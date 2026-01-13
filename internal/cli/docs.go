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

			target, path, err := findCommandByPath(root, args)
			if err != nil {
				return writeErr(cmd, err)
			}

			// No args => index page.
			if target == root && len(args) == 0 {
				md := renderDocsIndexMD(root)
				_, _ = io.WriteString(cmd.OutOrStdout(), md)
				return nil
			}

			switch outFormat {
			case "md", "markdown":
				md := renderCommandDocsMD(target, path)
				_, _ = io.WriteString(cmd.OutOrStdout(), md)
				return nil
			case "json", "edn":
				d := docCommandFrom(target, full)
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
	cmd.Flags().StringVar(&outFormat, "format", "md", "Docs format (md|json|edn)")
	cmd.Flags().BoolVar(&full, "full", false, "Include recursive subcommands in structured docs")
	return cmd
}

func docCommandFrom(c *cobra.Command, full bool) docCommand {
	out := docCommand{
		Use:     c.Use,
		Aliases: append([]string{}, c.Aliases...),
		Short:   c.Short,
		Long:    strings.TrimSpace(c.Long),
		Example: strings.TrimSpace(c.Example),
	}

	out.Flags = append(out.Flags, docFlags(c.InheritedFlags(), true)...)     // persistent inherited
	out.Flags = append(out.Flags, docFlags(c.NonInheritedFlags(), false)...) // local

	if full {
		subs := c.Commands()
		out.Subcommands = make([]docCommand, 0, len(subs))
		for _, sc := range subs {
			if sc.IsAvailableCommand() {
				out.Subcommands = append(out.Subcommands, docCommandFrom(sc, full))
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

func docFlags(fs *pflag.FlagSet, persistent bool) []docFlag {
	items := []docFlag{}
	if fs == nil {
		return items
	}
	fs.VisitAll(func(f *pflag.Flag) {
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

func findCommandByPath(root *cobra.Command, args []string) (*cobra.Command, string, error) {
	cur := root
	path := cur.Name()
	for _, tok := range args {
		nxt := findDirectSubcommand(cur, tok)
		if nxt == nil {
			return nil, "", fmt.Errorf("unknown command: %s (try `breyta docs` for index)", strings.Join(args, " "))
		}
		cur = nxt
		path = path + " " + cur.Name()
	}
	return cur, path, nil
}

func findDirectSubcommand(parent *cobra.Command, tok string) *cobra.Command {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return nil
	}
	for _, c := range parent.Commands() {
		if !c.IsAvailableCommand() {
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

func renderDocsIndexMD(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("## Breyta CLI docs\n\n")
	b.WriteString("This is on-demand documentation intended for agents and humans.\n\n")
	b.WriteString("### How to use\n\n")
	b.WriteString("- `breyta docs <command...>` prints Markdown docs for that command\n")
	b.WriteString("- `breyta <command...> --help` prints Cobra help for that command\n")
	b.WriteString("- For structured docs: `breyta docs <command...> --format json|edn`\n\n")

	b.WriteString("### Credentials / API keys for flows\n\n")
	b.WriteString("Flows execute inside `flows-api`. There are two ways credentials can be provided:\n\n")
	b.WriteString("- **Recommended (per-user / production-like)**: declare `:requires` slots (e.g. `:llm-provider`, `:http-api`) and have the user **activate** the flow in the UI to bind credentials.\n")
	b.WriteString("  Slot names must be non-namespaced keywords (e.g., `:api`, not `:ns/api`).\n")
	b.WriteString("  Manual trigger and wait notify field names use non-namespaced keywords (e.g., `{:name :user-id ...}`).\n")
	b.WriteString("- **Local-only (server-global)**: create `secrets.edn` (gitignored) to provide dev keys directly to the server process.\n\n")
	b.WriteString("Local-only `secrets.edn`:\n")
	b.WriteString("- `cp breyta/secrets.edn.example secrets.edn`\n")
	b.WriteString("- Add the keys you need and restart `flows-api`\n")
	b.WriteString("- Never commit `secrets.edn`\n\n")
	b.WriteString("CLI env vars (`BREYTA_API_URL`, `BREYTA_WORKSPACE`, `BREYTA_TOKEN`) are only for authenticating the CLI to `flows-api`.\n\n")

	b.WriteString("### Activation (credentials for `:requires` slots)\n\n")
	b.WriteString("If a flow declares `:requires` slots (e.g. `:http-api` with `:auth`/`:oauth`, or `:llm-provider`), you must activate it once to create a profile and bind credentials.\n")
	b.WriteString("Slot names must be non-namespaced keywords (e.g., `:api`, not `:ns/api`).\n\n")
	b.WriteString("Symptom if you forget: \"Slot reference requires a flow profile, but no profile-id in context\".\n\n")
	b.WriteString("Do this:\n")
	b.WriteString("- Sign in: `http://localhost:8090/login` → Sign in with Google → Dev User\n")
	b.WriteString("- Activate: `http://localhost:8090/<workspace>/flows/<slug>/activate` (e.g. `/ws-acme/flows/my-flow/activate`)\n")
	b.WriteString("- Or use: `breyta flows activate-url <slug>` to print the URL\n")
	b.WriteString("- Enter API key/token or complete OAuth, submit Activate Flow\n")
	b.WriteString("- Re-run the flow (CLI `runs start` will then resolve slots via the active profile)\n\n")

	b.WriteString("### Draft preview bindings\n\n")
	b.WriteString("Draft runs use a user-scoped draft profile and require draft bindings.\n\n")
	b.WriteString("- Draft bindings: `http://localhost:8090/<workspace>/flows/<slug>/draft-bindings`\n")
	b.WriteString("- Or use: `breyta flows draft-bindings-url <slug>` to print the URL\n")
	b.WriteString("- Run draft: `breyta runs start --flow <slug> --source draft`\n\n")

	b.WriteString("### Flow body constraints (SCI / orchestration DSL)\n\n")
	b.WriteString("Flow bodies are intentionally constrained to keep the \"flow language\" small for visualization and translation (Temporal-like orchestration), and to reduce the security surface area.\n\n")
	b.WriteString("Practical consequences:\n")
	b.WriteString("- Many everyday Clojure functions are denied in the flow body (e.g. `mapv`, `filterv`, `reduce`, etc.)\n")
	b.WriteString("- Keep the flow body focused on orchestration (a sequence of `step` calls)\n")
	b.WriteString("- Put data transformation into explicit `:function` steps (`:code` alias)\n\n")

	b.WriteString("### Input keys from `--input` (string vs keyword keys)\n\n")
	b.WriteString("`breyta --dev runs start --input '{...}'` sends JSON, so keys arrive as strings.\n\n")
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

func renderCommandDocsMD(c *cobra.Command, path string) string {
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

	flags := docFlags(c.InheritedFlags(), true)
	flags = append(flags, docFlags(c.NonInheritedFlags(), false)...)
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
