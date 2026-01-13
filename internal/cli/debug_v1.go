package cli

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

func newDebugCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debug",
		Short: "Dev-only debugging tools",
		Long: strings.TrimSpace(`
Dev-only debugging tools for flows-api.

These commands are primarily useful for agent-driven testing and debugging of:
- Debug overlays (breakpoints, mocks)
- Breakpoint controls (continue/abort)
`),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !app.DevMode {
				return errors.New("debug is a dev-only command; re-run with --dev (or set BREYTA_DEV=1)")
			}
			if !isAPIMode(app) {
				return errors.New("debug requires API mode (set BREYTA_API_URL)")
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newDebugOverlaysCmd(app))
	cmd.AddCommand(newDebugBreakpointsCmd(app))
	return cmd
}

func newDebugOverlaysCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "overlays",
		Short: "Manage debug overlays (breakpoints/mocks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newDebugOverlaysListCmd(app))
	cmd.AddCommand(newDebugOverlaysShowCmd(app))
	cmd.AddCommand(newDebugOverlaysSetEnabledCmd(app))
	cmd.AddCommand(newDebugOverlaysSetBreakpointCmd(app))
	return cmd
}

func newDebugOverlaysListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List debug overlays",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/debug/overlays",
				nil,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	return cmd
}

func newDebugOverlaysShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Show debug overlay for a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowSlug := strings.TrimSpace(args[0])
			if flowSlug == "" {
				return writeErr(cmd, errors.New("missing flow-slug"))
			}
			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/debug/overlays/"+url.PathEscape(flowSlug),
				nil,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	return cmd
}

func newDebugOverlaysSetBreakpointCmd(app *App) *cobra.Command {
	var bpType string
	cmd := &cobra.Command{
		Use:   "set-breakpoint <flow-slug> <step-id>",
		Short: "Set (or clear) a step breakpoint via debug overlay",
		Long: strings.TrimSpace(`
Sets a breakpoint for a step using the debug overlay endpoints.

Examples:
  breyta --dev debug overlays set-breakpoint wait-event-demo approval --type before
  breyta --dev debug overlays set-breakpoint wait-event-demo approval --type after
  breyta --dev debug overlays set-breakpoint wait-event-demo approval --type both
  breyta --dev debug overlays set-breakpoint wait-event-demo approval --type clear
`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowSlug := strings.TrimSpace(args[0])
			stepID := strings.TrimSpace(args[1])
			if flowSlug == "" || stepID == "" {
				return writeErr(cmd, errors.New("missing flow-slug or step-id"))
			}

			bpType = strings.TrimSpace(strings.ToLower(bpType))
			var signalValue string
			switch bpType {
			case "before", "after", "both":
				signalValue = bpType
			case "", "clear", "none", "off":
				signalValue = ""
			default:
				return writeErr(cmd, errors.New("invalid --type (expected before|after|both|clear)"))
			}

			signalKey := breakpointSignalKey(flowSlug, stepID)
			body := map[string]any{signalKey: signalValue}

			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodPatch,
				"/api/debug/overlays/"+url.PathEscape(flowSlug)+"/breakpoint/"+url.PathEscape(stepID),
				nil,
				body,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&bpType, "type", "before", "Breakpoint type: before|after|both|clear")
	return cmd
}

func newDebugOverlaysSetEnabledCmd(app *App) *cobra.Command {
	var enabled bool
	cmd := &cobra.Command{
		Use:   "set-enabled <flow-slug>",
		Short: "Enable/disable a debug overlay for a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowSlug := strings.TrimSpace(args[0])
			if flowSlug == "" {
				return writeErr(cmd, errors.New("missing flow-slug"))
			}
			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodPatch,
				"/api/debug/overlays/"+url.PathEscape(flowSlug)+"/enabled",
				nil,
				map[string]any{"enabled": enabled},
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Set overlay enabled (true/false)")
	return cmd
}

func newDebugBreakpointsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "breakpoints",
		Short: "Continue/abort workflows paused at breakpoints",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newDebugBreakpointsContinueCmd(app))
	cmd.AddCommand(newDebugBreakpointsAbortCmd(app))
	return cmd
}

func newDebugBreakpointsContinueCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "continue <workflow-id> <step-id> <before|after|both>",
		Short: "Continue a paused breakpoint",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := strings.TrimSpace(args[0])
			stepID := strings.TrimSpace(args[1])
			bpType := strings.TrimSpace(strings.ToLower(args[2]))
			if workflowID == "" || stepID == "" || bpType == "" {
				return writeErr(cmd, errors.New("missing workflow-id, step-id, or breakpoint-type"))
			}
			switch bpType {
			case "before", "after", "both":
				// ok
			default:
				return writeErr(cmd, errors.New("invalid breakpoint-type (expected before|after|both)"))
			}

			path := "/api/debug/breakpoints/" + url.PathEscape(workflowID) + "/" + url.PathEscape(stepID) + "/" + url.PathEscape(bpType) + "/continue"
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, path, nil, map[string]any{})
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	return cmd
}

func newDebugBreakpointsAbortCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "abort <workflow-id> <step-id> <before|after|both>",
		Short: "Abort a paused breakpoint",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := strings.TrimSpace(args[0])
			stepID := strings.TrimSpace(args[1])
			bpType := strings.TrimSpace(strings.ToLower(args[2]))
			if workflowID == "" || stepID == "" || bpType == "" {
				return writeErr(cmd, errors.New("missing workflow-id, step-id, or breakpoint-type"))
			}
			switch bpType {
			case "before", "after", "both":
				// ok
			default:
				return writeErr(cmd, errors.New("invalid breakpoint-type (expected before|after|both)"))
			}

			path := "/api/debug/breakpoints/" + url.PathEscape(workflowID) + "/" + url.PathEscape(stepID) + "/" + url.PathEscape(bpType) + "/abort"
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, path, nil, map[string]any{})
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	return cmd
}

func kebabToCamel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	upperNext := false
	for _, r := range s {
		if r == '-' {
			upperNext = true
			continue
		}
		if upperNext {
			b.WriteRune(unicode.ToUpper(r))
			upperNext = false
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func breakpointSignalKey(flowSlug, stepID string) string {
	return "bp_" + kebabToCamel(flowSlug) + "_" + kebabToCamel(stepID)
}
