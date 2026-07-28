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
2) edit the file (Clojure map literal + DSL), or use steps/compose helpers
3) push -> updates working copy (and validates by default)
4) diff -> inspect draft changes against live or a released version
5) release -> activates the latest pushed version and promotes live + installations in current workspace
6) run -> verifies behavior in your workspace

Optional explicit check:
- validate -> read-only verification for CI, troubleshooting, or explicit target checks

Advanced rollout workflow (optional):
- release -> activates the latest pushed version + live/installations promotion in current workspace
- promote -> updates live target and installations to a released version
- installations ... -> installation-id scoped management

Quick commands:
- breyta flows init <slug> --name "My flow"
- breyta flows init <slug> --step-id tools/fetch --step-file ./steps/fetch.edn --run
- breyta flows steps create <slug> <step-id> --step-file ./steps/step.edn
- breyta flows schedules add <slug> <schedule-id> --cron "0 9 * * MON" --timezone UTC
- breyta flows steps run <slug> <step-id> --params '{...}'
- breyta flows compose <slug> --body-file ./flows/<slug>.body.clj
- breyta flows list
- breyta flows pull <slug> --out ./tmp/flows/<slug>.clj
- breyta flows lint --file ./tmp/flows/<slug>.clj --local-only
- breyta flows paren-check --file ./tmp/flows/<slug>.clj
- breyta flows push --file ./tmp/flows/<slug>.clj
- breyta flows update <slug> --group-order 10
- breyta flows diff <slug>
- breyta flows configure <slug> --set api.conn=conn-...
- breyta flows configure check <slug>
- breyta flows release <slug> --release-note-file ./release-note.md
- breyta flows promote <slug> --version <n>
- breyta flows show <slug> --target live
- breyta flows run <slug> --target live --wait
- breyta flows run-step <slug> <step-id> --target live --input '{...}' --wait
- breyta flows run <slug> --wait

Flow file format (minimal):
{:slug :my-flow
 :name "My Flow"
 :description "..."
 :tags ["example"]
 :concurrency {:type :singleton :on-new-version :supersede}
 :requires nil
 :templates nil
 :functions nil
 :steps []
 :invocations {:default {:label "Run" :inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(let [input (flow/input)]
          (flow/step :function :do {:code '(fn [input] input)
                                     :input {:input input}}))}

Notes:
- The server reads the file with *read-eval* disabled.
- :flow should be a quoted form. (quote ...) is also accepted.
- Use flow/input for inputs and flow/step for steps.
- Local flows steps create/update/remove edits only the top-level :steps vector.
- Local flows schedules add/update/remove edits only the top-level :schedules vector.
- flows compose edits only the quoted :flow form, so packaged step definitions,
  interfaces, schedules, and connection metadata remain intact.
- Local lint catches qualified packaged-step references that are missing from
  :steps; the server remains the canonical validation stage before push.
- flows init can seed one complete packaged step with --step-id and --step-file;
  --run proves that local literal just in time, while --push is required for
  remote persistence.
- flows steps run sends the complete local literal for just-in-time server execution;
  it only addresses qualified top-level packaged :steps ids and does not create
  or update a draft. For named inline function/code steps or LLM steps with draft-bound
  connection slots, use flows run-step <slug> <step-id> --target draft
  --input '{...}' --wait. Use flows push explicitly to persist local source remotely.
- Grouping metadata is mutable workspace metadata, not part of the pulled flow source file.
  - inspect grouped flows: breyta flows list --limit 50
  - verify ordered siblings: breyta flows show <slug>
  - clear only ordering: breyta flows update <slug> --group-order ""
- Release notes are markdown attached to published versions.
  - draft vs live diff: breyta flows diff <slug>
  - set on release: breyta flows release <slug> --release-note-file ./release-note.md
  - edit later: breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md
- activeVersion is the currently activated released version. Live runtime can resolve to a different installation version
  - verify live with: breyta flows show <slug> --target live
  - smoke-run live with: breyta flows run <slug> --target live --wait
- Concurrency guidance:
  - Reconciler/sweeper/scheduled cleanup flows should use :on-new-version :supersede so fixes take effect immediately
  - Use :on-new-version :drain only when in-flight runs must finish on the old version

Advanced install lifecycle:
- Release the latest pushed version with default live + installations promotion: breyta flows release <slug>
- Release the latest pushed version while skipping end-user installation promotion: breyta flows release <slug> --skip-promote-installations
- Promote released version to live explicitly (also rollback to known-good): breyta flows promote <slug> --version <n>
- Browse public installables for this workspace: breyta flows discover list
- Search public installables for this workspace: breyta flows discover search <query>
- Publish all public surfaces explicitly after approval and release: breyta flows public publish <slug>
- Delist all public surfaces together: breyta flows public delist <slug>
- Lower-level Discover-only visibility update: breyta flows discover update <slug> --public=true
- Configure installation inputs: breyta flows installations configure <installation-id> --input '{...}'
- List legacy installation triggers: breyta flows installations triggers <installation-id>

Public discover notes:
- :discover {:public true} authored in a flow file persists as stored metadata on push.
- Use breyta flows public publish <slug> or breyta flows public delist <slug> to change marketplace, Discover, and public app-page visibility together.
- Use breyta flows discover update <slug> --public=true|false only when intentionally changing the lower-level Discover flag.
- Public discover requires explicit discover visibility and a released/installable flow.
- Public app marketing pages use https://breyta.ai/apps/<flow-slug>.
- breyta flows search "<query>" --limit 5 searches actual workspace flow metadata. Approved reusable templates live under breyta flows templates search "<query>" --limit 5, and public installables use breyta flows discover search "<query>".
		`),
	}

	cmd.AddCommand(newFlowsListCmd(app))
	cmd.AddCommand(newFlowsSearchCmd(app))
	cmd.AddCommand(newFlowsGrepCmd(app))
	cmd.AddCommand(newFlowsTemplatesCmd(app))
	cmd.AddCommand(newFlowsExamplesCmd(app))
	cmd.AddCommand(newFlowsWorkspaceCmd(app))
	cmd.AddCommand(newFlowsDoctorCmd(app))
	cmd.AddCommand(newFlowsReadinessCmd(app))
	cmd.AddCommand(newFlowsReleaseCheckCmd(app))
	cmd.AddCommand(newFlowsPublicCmd(app))
	cmd.AddCommand(newFlowsShowCmd(app))
	cmd.AddCommand(newFlowsDiffCmd(app))
	cmd.AddCommand(newFlowsCreateCmd(app))
	cmd.AddCommand(newFlowsInitCmd(app))
	cmd.AddCommand(newFlowsConfigureCmd(app))
	cmd.AddCommand(newFlowsBindingsCmd(app))
	cmd.AddCommand(newFlowsReleaseCmd(app))
	cmd.AddCommand(newFlowsPromoteCmd(app))
	cmd.AddCommand(newFlowsRunCmd(app))
	cmd.AddCommand(newFlowsRunStepCmd(app))
	cmd.AddCommand(newFlowsMetricsCmd(app))
	cmd.AddCommand(newFlowsInterfacesCmd(app))
	cmd.AddCommand(newFlowsActivateCmd(app))
	cmd.AddCommand(newFlowsInstallationsCmd(app))
	cmd.AddCommand(newFlowsDiscoverCmd(app))
	cmd.AddCommand(newFlowsMarketplaceCmd(app))
	cmd.AddCommand(newFlowsDraftCmd(app))
	cmd.AddCommand(newFlowsDraftBindingsURLCmd(app))
	cmd.AddCommand(newFlowsPullCmd(app))
	cmd.AddCommand(newFlowsLintCmd(app))
	cmd.AddCommand(newFlowsPushCmd(app))
	cmd.AddCommand(newFlowsImportCmd(app))
	cmd.AddCommand(newFlowsParenRepairCmd(app))
	cmd.AddCommand(newFlowsParenCheckCmd(app))
	cmd.AddCommand(newFlowsDeployCmd(app))
	cmd.AddCommand(newFlowsUpdateCmd(app))
	cmd.AddCommand(newFlowsProvenanceCmd(app))
	cmd.AddCommand(newFlowsArchiveCmd(app))
	cmd.AddCommand(newFlowsDeleteCmd(app))
	cmd.AddCommand(newFlowsSpineCmd(app))
	cmd.AddCommand(newFlowsCompileCmd(app))

	steps := &cobra.Command{Use: "steps", Short: "Manage flow steps"}
	steps.AddCommand(newFlowsStepsListCmd(app))
	steps.AddCommand(newFlowsStepsShowCmd(app))
	steps.AddCommand(newFlowsStepsLocalCreateCmd(app))
	steps.AddCommand(newFlowsStepsLocalUpdateCmd(app))
	steps.AddCommand(newFlowsStepsLocalRemoveCmd(app))
	steps.AddCommand(newFlowsStepsLocalRunCmd(app))
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
