package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func configureFlagVisibility(root *cobra.Command, app *App) {
	if root == nil || app == nil {
		return
	}

	// Root persistent flags apply everywhere; keep dev-only overrides hidden by default.
	showDev := app.DevMode || devModeEnabled()
	setFlagHidden(root.PersistentFlags(), "api", !showDev)
	setFlagHidden(root.PersistentFlags(), "token", !showDev)
	setFlagHidden(root.PersistentFlags(), "dev", true)
}

func setFlagHidden(fs *pflag.FlagSet, name string, hidden bool) {
	if fs == nil {
		return
	}
	if f := fs.Lookup(name); f != nil {
		f.Hidden = hidden
	}
}
