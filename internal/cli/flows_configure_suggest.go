package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type configureSuggestRow struct {
	Slot                    string `json:"slot"`
	Type                    string `json:"type,omitempty"`
	Status                  string `json:"status"`
	CurrentConnectionID     string `json:"currentConnectionId,omitempty"`
	SuggestedConnectionID   string `json:"suggestedConnectionId,omitempty"`
	SuggestedConnectionName string `json:"suggestedConnectionName,omitempty"`
	Confidence              string `json:"confidence,omitempty"`
	Reason                  string `json:"reason,omitempty"`
	SetArg                  string `json:"setArg,omitempty"`
}

type configureConnectionRequirement struct {
	Slot string
	Type string
}

func newFlowsConfigureSuggestCmd(app *App) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "suggest <flow-slug>",
		Short: "Suggest connection bindings from existing workspace connections",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows configure suggest requires API mode"))
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}

			resolvedTarget, err := normalizeInstallTarget(target)
			if err != nil {
				return writeErr(cmd, err)
			}
			profileType := "draft"
			if resolvedTarget == "live" {
				profileType = "prod"
			}

			flowSlug := strings.TrimSpace(args[0])
			client := apiClient(app)
			ctx := context.Background()

			templateResp, templateStatus, err := client.DoCommand(ctx, "profiles.template", map[string]any{
				"flowSlug":    flowSlug,
				"profileType": profileType,
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			if templateStatus >= 400 || !isOK(templateResp) {
				return writeAPIResult(cmd, app, templateResp, templateStatus)
			}
			requirements, err := extractTemplateRequirements(templateResp)
			if err != nil {
				return writeErr(cmd, err)
			}

			statusResp, statusCode, err := client.DoCommand(ctx, "profiles.status", map[string]any{
				"flowSlug":    flowSlug,
				"profileType": profileType,
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			if statusCode >= 400 || !isOK(statusResp) {
				return writeAPIResult(cmd, app, statusResp, statusCode)
			}

			bindingValues := extractBindingValuesBySlot(statusResp)
			activationSet := extractActivationSet(statusResp)
			connectionsByType, err := listConnectionsByType(client, requirements)
			if err != nil {
				return writeErr(cmd, err)
			}

			rows, suggestedSetArgs, unresolvedSlots := buildConfigureSuggestions(requirements, bindingValues, connectionsByType)
			missingActivationFields := collectMissingActivationFields(requirements, activationSet)

			nextCommands := []string{}
			targetFlag := ""
			if resolvedTarget == "live" {
				targetFlag = " --target live"
			}
			if len(suggestedSetArgs) > 0 {
				var b strings.Builder
				b.WriteString("breyta flows configure ")
				b.WriteString(flowSlug)
				b.WriteString(targetFlag)
				for _, setArg := range suggestedSetArgs {
					b.WriteString(" --set ")
					b.WriteString(setArg)
				}
				nextCommands = append(nextCommands, b.String())
			}
			if len(unresolvedSlots) > 0 {
				nextCommands = append(nextCommands, "breyta connections list")
			}
			if len(missingActivationFields) > 0 {
				nextCommands = append(nextCommands, fmt.Sprintf("breyta flows configure %s%s --set activation.<field>=<value>", flowSlug, targetFlag))
			}

			return writeData(cmd, app, nil, map[string]any{
				"flowSlug":                  flowSlug,
				"target":                    resolvedTarget,
				"profileType":               profileType,
				"suggestions":               rows,
				"suggestedSetArgs":          suggestedSetArgs,
				"unresolvedConnectionSlots": unresolvedSlots,
				"missingActivationInputs":   missingActivationFields,
				"summary": map[string]any{
					"connectionSlots": len(rows),
					"suggested":       countSuggestionsByStatus(rows, "suggested"),
					"configured":      countSuggestionsByStatus(rows, "configured"),
					"unresolved":      countSuggestionsByStatus(rows, "unresolved"),
				},
				"nextCommands": nextCommands,
			})
		},
	}
	cmd.Flags().StringVar(&target, "target", "draft", "Target override (draft|live)")
	return cmd
}

func extractTemplateRequirements(templateResp map[string]any) ([]any, error) {
	data, _ := templateResp["data"].(map[string]any)
	if data == nil {
		return nil, errors.New("profiles.template response missing data")
	}
	requirements, ok := data["requirements"].([]any)
	if !ok {
		return nil, errors.New("profiles.template response missing requirements")
	}
	return requirements, nil
}

func extractBindingValuesBySlot(statusResp map[string]any) map[string]string {
	data, _ := statusResp["data"].(map[string]any)
	raw, _ := data["bindingValues"].(map[string]any)
	out := map[string]string{}
	for k, v := range raw {
		slot := strings.TrimSpace(strings.TrimPrefix(k, ":"))
		if slot == "" {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(toString(v), ":"))
		if val == "" {
			continue
		}
		out[slot] = val
	}
	return out
}

func extractActivationSet(statusResp map[string]any) map[string]bool {
	data, _ := statusResp["data"].(map[string]any)
	raw, _ := data["activation"].(map[string]any)
	out := map[string]bool{}
	for key, value := range raw {
		entry, _ := value.(map[string]any)
		isSet, _ := entry["set"].(bool)
		if isSet {
			field := strings.TrimSpace(strings.TrimPrefix(key, ":"))
			if field != "" {
				out[field] = true
			}
		}
	}
	return out
}

func collectConnectionRequirements(requirements []any) []configureConnectionRequirement {
	out := make([]configureConnectionRequirement, 0)
	seen := map[string]struct{}{}
	for _, raw := range requirements {
		req, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(toString(req["kind"])), ":"))
		if kind == "form" {
			continue
		}
		slot := strings.TrimSpace(strings.TrimPrefix(toString(req["slot"]), ":"))
		if slot == "" {
			continue
		}
		reqType := normalizeConnectionType(req["type"])
		if reqType == "" || reqType == "secret" {
			continue
		}
		if _, exists := seen[slot]; exists {
			continue
		}
		seen[slot] = struct{}{}
		out = append(out, configureConnectionRequirement{Slot: slot, Type: reqType})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

func chooseSuggestedConnection(slot string, reqType string, conns []connectionSummary) (connectionSummary, string, string) {
	if len(conns) == 0 {
		return connectionSummary{}, "", "no matching connections found"
	}
	if reqType == "llm-provider" {
		if pick, ok := pickPreferredLLMConnection(conns); ok {
			return pick, "high", "matched preferred LLM connection"
		}
	}
	if len(conns) == 1 {
		return conns[0], "high", "only matching connection for required type"
	}

	slotNorm := strings.ToLower(strings.ReplaceAll(slot, "-", " "))
	matches := make([]connectionSummary, 0)
	for _, conn := range conns {
		nameNorm := strings.ToLower(conn.Name)
		idNorm := strings.ToLower(conn.ID)
		if (slotNorm != "" && strings.Contains(nameNorm, slotNorm)) || strings.Contains(idNorm, strings.ToLower(slot)) {
			matches = append(matches, conn)
		}
	}
	if len(matches) == 1 {
		return matches[0], "medium", "matched slot name to connection name/id"
	}
	return connectionSummary{}, "", "multiple matching connections found"
}

func buildConfigureSuggestions(requirements []any, bindingValues map[string]string, connectionsByType map[string][]connectionSummary) ([]configureSuggestRow, []string, []string) {
	reqs := collectConnectionRequirements(requirements)
	rows := make([]configureSuggestRow, 0, len(reqs))
	setArgs := make([]string, 0, len(reqs))
	unresolved := make([]string, 0)

	for _, req := range reqs {
		row := configureSuggestRow{
			Slot: req.Slot,
			Type: req.Type,
		}

		if current := strings.TrimSpace(bindingValues[req.Slot]); current != "" {
			row.Status = "configured"
			row.CurrentConnectionID = current
			row.Reason = "slot already configured"
			rows = append(rows, row)
			continue
		}

		candidate, confidence, reason := chooseSuggestedConnection(req.Slot, req.Type, connectionsByType[req.Type])
		if strings.TrimSpace(candidate.ID) != "" {
			row.Status = "suggested"
			row.SuggestedConnectionID = candidate.ID
			row.SuggestedConnectionName = candidate.Name
			row.Confidence = confidence
			row.Reason = reason
			row.SetArg = fmt.Sprintf("%s.conn=%s", req.Slot, candidate.ID)
			setArgs = append(setArgs, row.SetArg)
		} else {
			row.Status = "unresolved"
			row.Reason = reason
			unresolved = append(unresolved, req.Slot)
		}
		rows = append(rows, row)
	}

	sort.Strings(setArgs)
	sort.Strings(unresolved)
	return rows, uniqueStrings(setArgs), uniqueStrings(unresolved)
}

func collectMissingActivationFields(requirements []any, activationSet map[string]bool) []string {
	fields := make([]string, 0)
	for _, raw := range requirements {
		req, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(toString(req["kind"])), ":"))
		if kind != "form" {
			continue
		}
		reqFields, _ := req["fields"].([]any)
		for _, fieldRaw := range reqFields {
			field, _ := fieldRaw.(map[string]any)
			if field == nil {
				continue
			}
			required, _ := field["required"].(bool)
			if !required {
				continue
			}
			key := strings.TrimSpace(strings.TrimPrefix(toString(field["key"]), ":"))
			if key == "" {
				continue
			}
			if !activationSet[key] {
				fields = append(fields, key)
			}
		}
	}
	sort.Strings(fields)
	return uniqueStrings(fields)
}

func countSuggestionsByStatus(rows []configureSuggestRow, status string) int {
	count := 0
	for _, row := range rows {
		if row.Status == status {
			count++
		}
	}
	return count
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
