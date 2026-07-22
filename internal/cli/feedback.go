package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newFeedbackCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feedback",
		Short: "Send feedback and issue reports",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFeedbackSendCmd(app))
	cmd.AddCommand(newFeedbackFlowCmd(app))
	return cmd
}

func normalizeFeedbackType(raw string) (string, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case "", "issue", "bug", "problem", "error", "incident":
		return "issue", nil
	case "feature", "feature-request", "feature_request", "request", "enhancement", "idea":
		return "feature_request", nil
	case "general", "feedback", "note", "other":
		return "general", nil
	default:
		return "", errors.New("--type must be issue|feature_request|general")
	}
}

func normalizeFeedbackSource(raw string) (string, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case "":
		return "", nil
	case "agent", "human", "system":
		return v, nil
	default:
		return "", errors.New("--source must be agent|human|system")
	}
}

func parseFeedbackObjectFlag(flagName string, raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("invalid %s JSON: %w", flagName, err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", flagName)
	}
	return m, nil
}

func normalizeFeedbackTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, raw := range tags {
		for _, item := range strings.Split(raw, ",") {
			tag := strings.TrimSpace(strings.ToLower(item))
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// feedbackFlags holds the flags shared by the feedback subcommands so that the
// payload shape stays identical across `feedback send` and `feedback flow`.
type feedbackFlags struct {
	feedbackType string
	source       string
	agent        bool
	title        string
	description  string
	tags         []string
	command      string
	flowSlug     string
	workflowID   string
	runID        string
	metadataJSON string
	contextJSON  string
}

func (f *feedbackFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.feedbackType, "type", "issue", "Report type: issue|feature_request|general")
	cmd.Flags().StringVar(&f.source, "source", "", "Report source: agent|human|system")
	cmd.Flags().BoolVar(&f.agent, "agent", false, "Mark submission as agent-originated")
	cmd.Flags().StringVar(&f.title, "title", "", "Short report title")
	cmd.Flags().StringVar(&f.description, "description", "", "Detailed report description")
	cmd.Flags().StringArrayVar(&f.tags, "tag", nil, "Tag(s) to classify report (repeatable or comma-separated)")
	cmd.Flags().StringVar(&f.command, "command", "", "Related CLI command (defaults to current command)")
	cmd.Flags().StringVar(&f.flowSlug, "flow", "", "Related flow slug")
	cmd.Flags().StringVar(&f.workflowID, "workflow-id", "", "Related workflow id")
	cmd.Flags().StringVar(&f.runID, "run-id", "", "Related run id")
	cmd.Flags().StringVar(&f.metadataJSON, "metadata", "", "JSON object with environment metadata")
	cmd.Flags().StringVar(&f.contextJSON, "context", "", "JSON object with extra troubleshooting context")
}

// payload validates the flags and builds the command payload. When
// requireTarget is set, at least one of --flow / --workflow-id must be provided
// so the report can be routed to the flow's author.
func (f *feedbackFlags) payload(cmd *cobra.Command, requireTarget bool) (map[string]any, error) {
	typeNormalized, err := normalizeFeedbackType(f.feedbackType)
	if err != nil {
		return nil, err
	}
	sourceNormalized, err := normalizeFeedbackSource(f.source)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(f.title)
	if title == "" {
		return nil, errors.New("missing --title")
	}
	description := strings.TrimSpace(f.description)
	if description == "" {
		return nil, errors.New("missing --description")
	}

	flowSlug := strings.TrimSpace(f.flowSlug)
	workflowID := strings.TrimSpace(f.workflowID)
	if requireTarget && flowSlug == "" && workflowID == "" {
		return nil, errors.New("provide --flow or --workflow-id to target a flow's author")
	}

	metadata, err := parseFeedbackObjectFlag("--metadata", f.metadataJSON)
	if err != nil {
		return nil, err
	}
	contextData, err := parseFeedbackObjectFlag("--context", f.contextJSON)
	if err != nil {
		return nil, err
	}

	reportedCommand := strings.TrimSpace(f.command)
	if reportedCommand == "" {
		reportedCommand = strings.TrimSpace(cmd.CommandPath())
	}

	payload := map[string]any{
		"type":        typeNormalized,
		"title":       title,
		"description": description,
		"command":     reportedCommand,
	}
	if sourceNormalized != "" {
		payload["source"] = sourceNormalized
	}
	if f.agent {
		payload["agent"] = true
	}
	if tags := normalizeFeedbackTags(f.tags); len(tags) > 0 {
		payload["tags"] = tags
	}
	if flowSlug != "" {
		payload["flowSlug"] = flowSlug
	}
	if workflowID != "" {
		payload["workflowId"] = workflowID
	}
	if run := strings.TrimSpace(f.runID); run != "" {
		payload["runId"] = run
	}
	if len(metadata) > 0 {
		payload["metadata"] = metadata
	}
	if len(contextData) > 0 {
		payload["context"] = contextData
	}
	return payload, nil
}

func newFeedbackSendCmd(app *App) *cobra.Command {
	var flags feedbackFlags

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Submit a feedback report",
		Long: strings.TrimSpace(`
Submit product feedback, feature requests, or issue reports from CLI workflows.

Reports are stored server-side so the Breyta team can review and respond.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := flags.payload(cmd, false)
			if err != nil {
				return writeErr(cmd, err)
			}
			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "feedback.send", payload)
			}
			return doAPICommand(cmd, app, "feedback.send", payload)
		},
	}

	flags.register(cmd)
	return cmd
}

func newFeedbackFlowCmd(app *App) *cobra.Command {
	var flags feedbackFlags

	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Send feedback about a flow to its author",
		Long: strings.TrimSpace(`
Report a bug or send feedback about a specific flow to that flow's author,
rather than to the Breyta team.

Target the flow with --flow and/or --workflow-id (at least one is required).
Your name and email are shared with the author so they can follow up. If the
author cannot be verified, the report is stored for Breyta review instead.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := flags.payload(cmd, true)
			if err != nil {
				return writeErr(cmd, err)
			}
			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "feedback.flow.send", payload)
			}
			return doAPICommand(cmd, app, "feedback.flow.send", payload)
		},
	}

	flags.register(cmd)
	return cmd
}
