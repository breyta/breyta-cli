package cli

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/state"
	"github.com/spf13/cobra"
)

var apiValidFlowSlugRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`)

const defaultFlowPushTimeout = 2 * time.Minute

func isAPISafeIdentifier(s string) bool {
	return apiValidFlowSlugRe.MatchString(strings.TrimSpace(s))
}

func isAPIValidFlowSlug(s string) bool {
	return isAPISafeIdentifier(s)
}

func normalizeOptionalText(s string) string {
	return strings.TrimSpace(s)
}

func normalizeOptionalMarkdown(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

func appendFlowMutableMetadata(out map[string]any, flow *state.Flow) {
	if flow == nil {
		return
	}
	appendGroupMetadata(out, flow.GroupKey, flow.GroupName, flow.GroupDescription, flow.GroupOrder)
	if publishDescription := normalizeOptionalMarkdown(flow.PublishDescription); publishDescription != "" {
		out["publishDescription"] = publishDescription
	}
	if publishMedia := publishMediaPayloadValue(flow.PublishMedia); len(publishMedia) > 0 {
		out["publishMedia"] = publishMedia
	}
	if selector := normalizeOptionalText(flow.PrimaryDisplayConnectionSlot); selector != "" {
		out["primaryDisplayConnectionSlot"] = selector
	}
}

func appendGroupMetadata(out map[string]any, groupKey, groupName, groupDescription string, groupOrder *int) {
	if out == nil {
		return
	}
	if groupKey = normalizeOptionalText(groupKey); groupKey != "" {
		out["groupKey"] = groupKey
	}
	if groupName = normalizeOptionalText(groupName); groupName != "" {
		out["groupName"] = groupName
	}
	if groupDescription = normalizeOptionalText(groupDescription); groupDescription != "" {
		out["groupDescription"] = groupDescription
	}
	if groupOrder != nil {
		out["groupOrder"] = *groupOrder
	}
}

func parseOptionalGroupOrder(raw string) (*int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("invalid --group-order %q (must be a non-negative integer or empty string to clear it)", raw)
	}
	return &n, nil
}

func parseOptionalDisplayConnectionSlot(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if !isAPISafeIdentifier(value) {
		return "", fmt.Errorf("invalid --primary-display-connection-slot %q (must start with a letter; allowed: letters, digits, hyphen (-), underscore (_); max 128 chars)", raw)
	}
	return value, nil
}

func localGroupFlows(ws *state.Workspace, currentSlug, groupKey string) []map[string]any {
	groupKey = normalizeOptionalText(groupKey)
	if ws == nil || groupKey == "" {
		return nil
	}

	members := make([]*state.Flow, 0, len(ws.Flows))
	for _, candidate := range ws.Flows {
		if candidate == nil || candidate.Slug == currentSlug {
			continue
		}
		if normalizeOptionalText(candidate.GroupKey) == groupKey {
			members = append(members, candidate)
		}
	}

	ordered := false
	for _, member := range members {
		if member != nil && member.GroupOrder != nil {
			ordered = true
			break
		}
	}

	sort.Slice(members, func(i, j int) bool {
		if ordered {
			leftOrder := int(^uint(0) >> 1)
			rightOrder := int(^uint(0) >> 1)
			if members[i].GroupOrder != nil {
				leftOrder = *members[i].GroupOrder
			}
			if members[j].GroupOrder != nil {
				rightOrder = *members[j].GroupOrder
			}
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
		}
		leftName := strings.ToLower(normalizeOptionalText(members[i].Name))
		rightName := strings.ToLower(normalizeOptionalText(members[j].Name))
		if leftName != rightName {
			return leftName < rightName
		}
		return members[i].Slug < members[j].Slug
	})

	items := make([]map[string]any, 0, len(members))
	for _, member := range members {
		item := map[string]any{
			"flowSlug":    member.Slug,
			"name":        member.Name,
			"description": member.Description,
		}
		appendFlowMutableMetadata(item, member)
		items = append(items, item)
	}
	return items
}

func resolveLocalFlowGroupUpdate(cmd *cobra.Command, flow *state.Flow, groupKey, groupName, groupDescription, groupOrder string) (string, string, string, *int, bool, error) {
	groupKeyProvided := cmd.Flags().Changed("group-key")
	groupNameProvided := cmd.Flags().Changed("group-name")
	groupDescriptionProvided := cmd.Flags().Changed("group-description")
	groupOrderProvided := cmd.Flags().Changed("group-order")
	if !groupKeyProvided && !groupNameProvided && !groupDescriptionProvided && !groupOrderProvided {
		return "", "", "", nil, false, nil
	}

	currentGroupKey := normalizeOptionalText(flow.GroupKey)
	currentGroupName := normalizeOptionalText(flow.GroupName)
	currentGroupDescription := normalizeOptionalText(flow.GroupDescription)
	currentGroupOrder := flow.GroupOrder
	requestedGroupKey := normalizeOptionalText(groupKey)
	requestedGroupName := normalizeOptionalText(groupName)
	requestedGroupDescription := normalizeOptionalText(groupDescription)
	requestedGroupOrder, err := parseOptionalGroupOrder(groupOrder)
	if err != nil {
		return "", "", "", nil, false, err
	}
	clearGroup := groupKeyProvided && requestedGroupKey == ""

	finalGroupKey := currentGroupKey
	if clearGroup {
		finalGroupKey = ""
	} else if groupKeyProvided {
		finalGroupKey = requestedGroupKey
	}

	finalGroupName := currentGroupName
	if clearGroup {
		finalGroupName = ""
	} else if groupNameProvided {
		finalGroupName = requestedGroupName
	}

	finalGroupDescription := currentGroupDescription
	if clearGroup {
		finalGroupDescription = ""
	} else if groupDescriptionProvided {
		finalGroupDescription = requestedGroupDescription
	}

	finalGroupOrder := currentGroupOrder
	if clearGroup {
		finalGroupOrder = nil
	} else if groupOrderProvided {
		finalGroupOrder = requestedGroupOrder
	}

	if groupKeyProvided && requestedGroupKey != "" && !isAPIValidFlowSlug(requestedGroupKey) {
		return "", "", "", nil, false, fmt.Errorf("invalid --group-key %q (must start with a letter; allowed: letters, digits, hyphen (-), underscore (_); max 128 chars)", requestedGroupKey)
	}
	if (groupNameProvided || groupDescriptionProvided || groupOrderProvided) && finalGroupKey == "" {
		return "", "", "", nil, false, errors.New("groupKey is required")
	}
	if finalGroupKey != "" && finalGroupName == "" {
		return "", "", "", nil, false, errors.New("groupName is required")
	}

	return finalGroupKey, finalGroupName, finalGroupDescription, finalGroupOrder, true, nil
}

// doAPICommandFn is a test hook to stub API calls in command unit tests.
var doAPICommandFn = doAPICommand
var useDoAPICommandFn bool

func newFlowsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "flows",
		Aliases: []string{"flow"},
		Short:   "Inspect and edit flows",
		Long: strings.TrimSpace(`
Flow authoring uses a file workflow:
1) init or pull a flow to a local .clj file
2) edit the Clojure map literal and DSL
3) push the working draft and validate it
4) inspect the diff, release it, and run it

Common commands:
- breyta flows init <slug> --name "My flow"
- breyta flows pull <slug> --out ./flows/<slug>.clj
- breyta flows lint --file ./flows/<slug>.clj --local-only
- breyta flows push --file ./flows/<slug>.clj
- breyta flows diff <slug>
- breyta flows release <slug> --release-note-file ./release-note.md
- breyta flows run <slug> --target live --wait

The engine reads source with *read-eval* disabled. The :flow value should be a
quoted form that uses flow/input and flow/step. Local editing commands change
only the selected top-level section; the engine remains the canonical validator.
		`),
	}

	cmd.AddCommand(newFlowsListCmd(app))
	cmd.AddCommand(newFlowsShowCmd(app))
	cmd.AddCommand(newFlowsDiffCmd(app))
	cmd.AddCommand(newFlowsCreateCmd(app))
	cmd.AddCommand(newFlowsInitCmd(app))
	cmd.AddCommand(newFlowsReleaseCmd(app))
	cmd.AddCommand(newFlowsRunCmd(app))
	cmd.AddCommand(newFlowsRunStepCmd(app))
	cmd.AddCommand(newFlowsDraftCmd(app))
	cmd.AddCommand(newFlowsPullCmd(app))
	cmd.AddCommand(newFlowsLintCmd(app))
	cmd.AddCommand(newFlowsPushCmd(app))
	cmd.AddCommand(newFlowsImportCmd(app))
	cmd.AddCommand(newFlowsParenRepairCmd(app))
	cmd.AddCommand(newFlowsParenCheckCmd(app))
	cmd.AddCommand(newFlowsDeployCmd(app))
	cmd.AddCommand(newFlowsUpdateCmd(app))
	cmd.AddCommand(newFlowsArchiveCmd(app))
	cmd.AddCommand(newFlowsDeleteCmd(app))
	cmd.AddCommand(newFlowsCompileCmd(app))

	steps := &cobra.Command{Use: "steps", Short: "Manage flow steps"}
	steps.AddCommand(newFlowsStepsListCmd(app))
	steps.AddCommand(newFlowsStepsShowCmd(app))
	steps.AddCommand(newFlowsStepsLocalCreateCmd(app))
	steps.AddCommand(newFlowsStepsLocalUpdateCmd(app))
	steps.AddCommand(newFlowsStepsLocalRemoveCmd(app))
	cmd.AddCommand(steps)
	cmd.AddCommand(newFlowsSchedulesLocalCmd(app))
	cmd.AddCommand(newFlowsComposeCmd(app))

	versions := &cobra.Command{Use: "versions", Short: "Manage flow versions"}
	versions.AddCommand(newFlowsVersionsListCmd(app))
	versions.AddCommand(newFlowsVersionsPublishCmd(app))
	versions.AddCommand(newFlowsVersionsUpdateCmd(app))
	versions.AddCommand(newFlowsVersionsActivateCmd(app))
	versions.AddCommand(newFlowsVersionsDiffCmd(app))
	cmd.AddCommand(versions)

	cmd.AddCommand(newFlowsValidateCmd(app))

	return cmd
}
