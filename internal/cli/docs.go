package cli

import (
        "fmt"
        "io"
        "strings"

        "breyta-cli/internal/format"

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
                                        "workspaceId": app.WorkspaceID,
                                        "path":        path,
                                        "command":     d,
                                        "_hint":       "Pass `breyta docs <command> --format md` for human-readable docs; use `--full` to include recursive subcommands.",
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
