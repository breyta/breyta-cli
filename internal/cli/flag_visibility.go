package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func configureFlagVisibility(root *cobra.Command, app *App) {
	if root == nil || app == nil {
		return
	}

	setFlagHidden(root.PersistentFlags(), "api", false)
	setFlagHidden(root.PersistentFlags(), "api-key", false)
	setFlagHidden(root.PersistentFlags(), "token", false)
	setFlagHidden(root.PersistentFlags(), "state", true)
	setFlagHidden(root.PersistentFlags(), "dev", true)
}

func setFlagHidden(fs *pflag.FlagSet, name string, hidden bool) {
	if fs == nil {
		return
	}
	if f := fs.Lookup(name); f != nil {
		if hidden {
			_ = fs.MarkHidden(name)
		}
		f.Hidden = hidden
	}
}
