package cli

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var snoozeDurationPattern = regexp.MustCompile(`(?i)^([1-9][0-9]*)([smhd])$`)

func normalizeSnoozeDuration(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	matches := snoozeDurationPattern.FindStringSubmatch(trimmed)
	if len(matches) != 3 {
		return "", false
	}
	return matches[1] + strings.ToLower(matches[2]), true
}

func runFlowHealthREST(cmd *cobra.Command, app *App, unavailableMessage, method, path string, query url.Values) error {
	if !isAPIMode(app) {
		return writeNotImplemented(cmd, app, unavailableMessage)
	}
	if err := requireAPI(app); err != nil {
		return writeErr(cmd, err)
	}
	out, httpStatus, err := apiClient(app).DoREST(context.Background(), method, path, query, nil)
	if err != nil {
		return writeErr(cmd, err)
	}
	return writeREST(cmd, app, httpStatus, out)
}

func newIncidentsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "incidents",
		Short: "Inspect flow-health incidents",
	}
	cmd.AddCommand(newIncidentsListCmd(app))
	cmd.AddCommand(newIncidentsShowCmd(app))
	cmd.AddCommand(newIncidentsLanesCmd(app))
	cmd.AddCommand(newIncidentsAcknowledgeCmd(app))
	cmd.AddCommand(newIncidentsSnoozeCmd(app))
	return cmd
}

func newIncidentsListCmd(app *App) *cobra.Command {
	var status string
	var limit int
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List incidents",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if strings.TrimSpace(status) != "" {
				q.Set("status", strings.TrimSpace(status))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			return runFlowHealthREST(cmd, app, "Use API mode to inspect incidents.", http.MethodGet, "/api/incidents", q)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter incidents by status")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max incidents to return")
	return cmd
}

func newIncidentsShowCmd(app *App) *cobra.Command {
	var failureLimit int
	cmd := &cobra.Command{
		Use:   "show <incident-id>",
		Short: "Show one incident with first-class failure rows",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			incidentID := strings.TrimSpace(args[0])
			q := url.Values{}
			if failureLimit > 0 {
				q.Set("limit", strconv.Itoa(failureLimit))
			}
			return runFlowHealthREST(cmd, app, "Use API mode to inspect incidents.", http.MethodGet, "/api/incidents/"+url.PathEscape(incidentID), q)
		},
	}
	cmd.Flags().IntVar(&failureLimit, "failure-limit", 20, "Max failure rows to return")
	return cmd
}

func newIncidentsLanesCmd(app *App) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "lanes <incident-id>",
		Short: "Inspect affected lanes/keys for one incident",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			incidentID := strings.TrimSpace(args[0])
			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			return runFlowHealthREST(cmd, app, "Use API mode to inspect incident lanes.", http.MethodGet, "/api/incidents/"+url.PathEscape(incidentID)+"/lanes", q)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Max lanes to return")
	return cmd
}

func newIncidentsAcknowledgeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acknowledge <incident-id>",
		Short: "Acknowledge an open incident",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			incidentID := strings.TrimSpace(args[0])
			return runFlowHealthREST(cmd, app, "Use API mode to acknowledge incidents.", http.MethodPost, "/api/incidents/"+url.PathEscape(incidentID)+"/acknowledge", nil)
		},
	}
	return cmd
}

func newIncidentsSnoozeCmd(app *App) *cobra.Command {
	var snoozeFor string
	cmd := &cobra.Command{
		Use:   "snooze <incident-id>",
		Short: "Snooze an incident for a bounded duration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedDuration, ok := normalizeSnoozeDuration(snoozeFor)
			if !ok {
				return writeErr(cmd, &guidedCLIError{message: "invalid --for duration (expected e.g. 30m, 2h, or 1d)"})
			}
			incidentID := strings.TrimSpace(args[0])
			q := url.Values{}
			q.Set("for", normalizedDuration)
			return runFlowHealthREST(cmd, app, "Use API mode to snooze incidents.", http.MethodPost, "/api/incidents/"+url.PathEscape(incidentID)+"/snooze", q)
		},
	}
	cmd.Flags().StringVar(&snoozeFor, "for", "", "How long to snooze the incident, e.g. 30m or 2h")
	_ = cmd.MarkFlagRequired("for")
	return cmd
}

func newDigestsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "digests",
		Short: "Inspect flow-health digests",
	}
	cmd.AddCommand(newDigestsListCmd(app))
	cmd.AddCommand(newDigestsShowCmd(app))
	cmd.AddCommand(newDigestsDeliveriesCmd(app))
	cmd.AddCommand(newDigestsMarkReadCmd(app))
	return cmd
}

func newDigestsListCmd(app *App) *cobra.Command {
	var kind string
	var status string
	var limit int
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List digests",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if strings.TrimSpace(kind) != "" {
				q.Set("kind", strings.TrimSpace(kind))
			}
			if strings.TrimSpace(status) != "" {
				q.Set("status", strings.TrimSpace(status))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			return runFlowHealthREST(cmd, app, "Use API mode to inspect digests.", http.MethodGet, "/api/digests", q)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "Filter digests by kind")
	cmd.Flags().StringVar(&status, "status", "", "Filter digests by status")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max digests to return")
	return cmd
}

func newDigestsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <digest-id>",
		Short: "Show one digest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			digestID := strings.TrimSpace(args[0])
			return runFlowHealthREST(cmd, app, "Use API mode to inspect digests.", http.MethodGet, "/api/digests/"+url.PathEscape(digestID), nil)
		},
	}
	return cmd
}

func newDigestsDeliveriesCmd(app *App) *cobra.Command {
	var channel string
	var limit int
	cmd := &cobra.Command{
		Use:   "deliveries <digest-id>",
		Short: "List canonical deliveries for one digest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			digestID := strings.TrimSpace(args[0])
			q := url.Values{}
			if strings.TrimSpace(channel) != "" {
				q.Set("channel", strings.TrimSpace(channel))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			return runFlowHealthREST(cmd, app, "Use API mode to inspect digest deliveries.", http.MethodGet, "/api/digests/"+url.PathEscape(digestID)+"/deliveries", q)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "Filter deliveries by channel")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max deliveries to return")
	return cmd
}

func newDigestsMarkReadCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mark-read <digest-id>",
		Short: "Mark the current user's in-app digest delivery as read",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			digestID := strings.TrimSpace(args[0])
			return runFlowHealthREST(cmd, app, "Use API mode to mark digests as read.", http.MethodPost, "/api/digests/"+url.PathEscape(digestID)+"/mark-read", nil)
		},
	}
	return cmd
}
