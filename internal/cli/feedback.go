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

func newFeedbackSendCmd(app *App) *cobra.Command {
	var feedbackType string
	var source string
	var agent bool
	var title string
	var description string
	var tags []string
	var commandName string
	var flowSlug string
	var workflowID string
	var runID string
	var metadataJSON string
	var contextJSON string

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Submit a feedback report",
		Long: strings.TrimSpace(`
Submit product feedback, feature requests, or issue reports from CLI workflows.

Reports are persisted server-side and forwarded to internal notification channels.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			typeNormalized, err := normalizeFeedbackType(feedbackType)
			if err != nil {
				return writeErr(cmd, err)
			}
			sourceNormalized, err := normalizeFeedbackSource(source)
			if err != nil {
				return writeErr(cmd, err)
			}

			title = strings.TrimSpace(title)
			if title == "" {
				return writeErr(cmd, errors.New("missing --title"))
			}
			description = strings.TrimSpace(description)
			if description == "" {
				return writeErr(cmd, errors.New("missing --description"))
			}

			metadata, err := parseFeedbackObjectFlag("--metadata", metadataJSON)
			if err != nil {
				return writeErr(cmd, err)
			}
			contextData, err := parseFeedbackObjectFlag("--context", contextJSON)
			if err != nil {
				return writeErr(cmd, err)
			}

			reportedCommand := strings.TrimSpace(commandName)
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
			if agent {
				payload["agent"] = true
			}
			if normalizedTags := normalizeFeedbackTags(tags); len(normalizedTags) > 0 {
				payload["tags"] = normalizedTags
			}
			if flow := strings.TrimSpace(flowSlug); flow != "" {
				payload["flowSlug"] = flow
			}
			if wf := strings.TrimSpace(workflowID); wf != "" {
				payload["workflowId"] = wf
			}
			if run := strings.TrimSpace(runID); run != "" {
				payload["runId"] = run
			}
			if len(metadata) > 0 {
				payload["metadata"] = metadata
			}
			if len(contextData) > 0 {
				payload["context"] = contextData
			}

			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "feedback.send", payload)
			}
			return doAPICommand(cmd, app, "feedback.send", payload)
		},
	}

	cmd.Flags().StringVar(&feedbackType, "type", "issue", "Report type: issue|feature_request|general")
	cmd.Flags().StringVar(&source, "source", "", "Report source: agent|human|system")
	cmd.Flags().BoolVar(&agent, "agent", false, "Mark submission as agent-originated")
	cmd.Flags().StringVar(&title, "title", "", "Short report title")
	cmd.Flags().StringVar(&description, "description", "", "Detailed report description")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag(s) to classify report (repeatable or comma-separated)")
	cmd.Flags().StringVar(&commandName, "command", "", "Related CLI command (defaults to current command)")
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Related flow slug")
	cmd.Flags().StringVar(&workflowID, "workflow-id", "", "Related workflow id")
	cmd.Flags().StringVar(&runID, "run-id", "", "Related run id")
	cmd.Flags().StringVar(&metadataJSON, "metadata", "", "JSON object with environment metadata")
	cmd.Flags().StringVar(&contextJSON, "context", "", "JSON object with extra troubleshooting context")

	return cmd
}
