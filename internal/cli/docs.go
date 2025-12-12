package cli

import (
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
        cmd := &cobra.Command{
                Use:   "docs",
                Short: "Machine-readable CLI docs (mock)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        d := docCommandFrom(root)
                        return format.WriteJSON(cmd.OutOrStdout(), map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "_hint":       "This output is intended for agents and tooling. Use `breyta <cmd> --help` for human-readable help.",
                                "root":        d,
                        }, true)
                },
        }
        return cmd
}

func docCommandFrom(c *cobra.Command) docCommand {
        out := docCommand{
                Use:     c.Use,
                Aliases: append([]string{}, c.Aliases...),
                Short:   c.Short,
                Long:    strings.TrimSpace(c.Long),
                Example: strings.TrimSpace(c.Example),
        }

        out.Flags = append(out.Flags, docFlags(c.InheritedFlags(), true)...)     // persistent inherited
        out.Flags = append(out.Flags, docFlags(c.NonInheritedFlags(), false)...) // local

        subs := c.Commands()
        out.Subcommands = make([]docCommand, 0, len(subs))
        for _, sc := range subs {
                if sc.IsAvailableCommand() {
                        out.Subcommands = append(out.Subcommands, docCommandFrom(sc))
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
