package cli

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newFlowsDeployCmd(app *App) *cobra.Command {
	var version int
	var deployKey string
	var releaseNote string
	var releaseNoteFile string
	var legacyNote string
	cmd := &cobra.Command{
		Use:   "deploy <flow-slug>",
		Short: "Deploy a flow version (make it active)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Deploy requires --api/BREYTA_API_URL")
			}
			payload := map[string]any{"flowSlug": args[0]}
			if version > 0 {
				payload["version"] = version
			}
			resolvedDeployKey := strings.TrimSpace(deployKey)
			if resolvedDeployKey == "" {
				resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
			}
			if resolvedDeployKey != "" {
				payload["deployKey"] = resolvedDeployKey
			}
			resolvedReleaseNote, err := resolveReleaseNoteInput(releaseNote, legacyNote, releaseNoteFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(resolvedReleaseNote) != "" {
				payload["releaseNote"] = resolvedReleaseNote
			}
			return doAPICommand(cmd, app, "flows.deploy", payload)
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "Version (0 = latest)")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key (default: BREYTA_FLOW_DEPLOY_KEY)")
	cmd.Flags().StringVar(&releaseNote, "release-note", "", "Markdown release note to attach to the deployed version")
	cmd.Flags().StringVar(&releaseNoteFile, "release-note-file", "", "Read markdown release note from file")
	cmd.Flags().StringVar(&legacyNote, "note", "", "Deprecated alias for --release-note")
	_ = cmd.Flags().MarkHidden("note")
	return cmd
}

func newFlowsUpdateCmd(app *App) *cobra.Command {
	var name, description, tags, primaryDisplayConnectionSlot string
	var groupKey, groupName, groupDescription, groupOrder string
	cmd := &cobra.Command{
		Use:   "update <flow-slug>",
		Short: "Update flow metadata",
		Long: strings.TrimSpace(`
Update mutable flow metadata such as name, description, tags, grouping, and display icon selection.

Grouping and display icon metadata are workspace metadata. They do not round-trip through
` + "`breyta flows pull`" + ` / ` + "`breyta flows push`" + ` source files.

Common grouped-flow loop:
- inspect current grouping with ` + "`breyta flows list --limit 50`" + ` or ` + "`breyta flows show <slug>`" + `
- set or change grouping with ` + "`breyta flows update <slug> --group-key ... --group-name ... --group-order ...`" + `
- verify sibling order again with ` + "`breyta flows show <slug>`" + `

Display icon loop:
- inspect current display icon selector with ` + "`breyta flows show <slug>`" + `
- set or clear it with ` + "`breyta flows update <slug> --primary-display-connection-slot <selector>`" + `
		`),
		Example: strings.TrimSpace(`
breyta flows update invoice-start \
  --group-key invoice-pipeline \
  --group-name "Invoice Pipeline" \
  --group-description "Flows that run in sequence for invoice processing" \
  --group-order 10

breyta flows update invoice-reconcile --group-order 20

breyta flows show invoice-start

breyta flows update invoice-reconcile --group-order ""
breyta flows update invoice-start --group-key ""

breyta flows update customer-support --primary-display-connection-slot crm
breyta flows update customer-support --primary-display-connection-slot ""
			`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			groupOrderChanged := cmd.Flags().Changed("group-order")
			var resolvedGroupOrder *int
			if groupOrderChanged {
				var err error
				resolvedGroupOrder, err = parseOptionalGroupOrder(groupOrder)
				if err != nil {
					return writeErr(cmd, err)
				}
			}
			primaryDisplayConnectionSlotChanged := cmd.Flags().Changed("primary-display-connection-slot")
			var resolvedSelector string
			if primaryDisplayConnectionSlotChanged {
				var err error
				resolvedSelector, err = parseOptionalDisplayConnectionSlot(primaryDisplayConnectionSlot)
				if err != nil {
					return writeErr(cmd, err)
				}
			}
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				if strings.TrimSpace(name) != "" {
					payload["name"] = name
				}
				if strings.TrimSpace(description) != "" {
					payload["description"] = description
				}
				if strings.TrimSpace(tags) != "" {
					payload["tags"] = tags
				}
				if cmd.Flags().Changed("group-key") {
					payload["groupKey"] = normalizeOptionalText(groupKey)
				}
				if cmd.Flags().Changed("group-name") {
					payload["groupName"] = normalizeOptionalText(groupName)
				}
				if cmd.Flags().Changed("group-description") {
					payload["groupDescription"] = normalizeOptionalText(groupDescription)
				}
				if groupOrderChanged {
					if resolvedGroupOrder == nil {
						payload["groupOrder"] = ""
					} else {
						payload["groupOrder"] = *resolvedGroupOrder
					}
				}
				if primaryDisplayConnectionSlotChanged {
					payload["primaryDisplayConnectionSlot"] = resolvedSelector
				}
				if useDoAPICommandFn {
					return doAPICommandFn(cmd, app, "flows.update", payload)
				}
				return doAPICommand(cmd, app, "flows.update", payload)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			f := ws.Flows[args[0]]
			if f == nil {
				return writeErr(cmd, errors.New("flow not found"))
			}
			if name != "" {
				f.Name = name
			}
			if description != "" {
				f.Description = description
			}
			if tags != "" {
				f.Tags = splitNonEmpty(tags)
			}
			resolvedGroupKey, resolvedGroupName, resolvedGroupDescription, resolvedGroupOrder, groupChanged, err := resolveLocalFlowGroupUpdate(cmd, f, groupKey, groupName, groupDescription, groupOrder)
			if err != nil {
				return writeErr(cmd, err)
			}
			if groupChanged {
				f.GroupKey = resolvedGroupKey
				f.GroupName = resolvedGroupName
				f.GroupDescription = resolvedGroupDescription
				f.GroupOrder = resolvedGroupOrder
			}
			if primaryDisplayConnectionSlotChanged {
				f.PrimaryDisplayConnectionSlot = resolvedSelector
			}
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flow": f})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Name")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&tags, "tags", "", "Comma-separated tags")
	cmd.Flags().StringVar(&groupKey, "group-key", "", "Group key (safe identifier; empty string clears grouping)")
	cmd.Flags().StringVar(&groupName, "group-name", "", "Group name (required whenever group key is set)")
	cmd.Flags().StringVar(&groupDescription, "group-description", "", "Group description")
	cmd.Flags().StringVar(&groupOrder, "group-order", "", "Group order (lower numbers sort first; empty string clears it)")
	cmd.Flags().StringVar(&primaryDisplayConnectionSlot, "primary-display-connection-slot", "", "Display icon selector (empty string clears it)")
	return cmd
}

func newFlowsProvenanceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provenance",
		Short: "Manage flow provenance metadata",
	}
	cmd.AddCommand(newFlowsProvenanceSetCmd(app))
	return cmd
}

func newFlowsProvenanceSetCmd(app *App) *cobra.Command {
	var sources []string
	var templates []string
	var fromConsulted bool
	var clear bool

	cmd := &cobra.Command{
		Use:   "set <flow-slug>",
		Short: "Replace flow provenance metadata",
		Long: strings.TrimSpace(`
Replace the full set of source-flow provenance refs for a flow.

Use --from-consulted to persist the flows previously opened with ` + "`breyta flows show`" + `
or ` + "`breyta flows pull`" + ` in this agent workspace. Use --source for workspace flows,
--template for public templates, and --clear to explicitly remove all provenance.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows provenance set requires API mode"))
			}
			if clear && (fromConsulted || len(sources) > 0 || len(templates) > 0) {
				return writeErr(cmd, errors.New("--clear cannot be combined with --source, --template, or --from-consulted"))
			}

			flowSlug := strings.TrimSpace(args[0])
			payload := map[string]any{"flowSlug": flowSlug}

			if clear {
				payload["sourceFlows"] = []map[string]any{}
				if useDoAPICommandFn {
					return doAPICommandFn(cmd, app, "flows.provenance.set", payload)
				}
				return doAPICommand(cmd, app, "flows.provenance.set", payload)
			}

			refs := make([]provenanceSourceRef, 0, len(sources)+len(templates))
			for _, raw := range sources {
				ref, err := parseProvenanceSourceRef(raw, app.WorkspaceID)
				if err != nil {
					return writeErr(cmd, err)
				}
				refs = append(refs, ref)
			}
			for _, raw := range templates {
				ref, err := parseProvenanceTemplateRef(raw)
				if err != nil {
					return writeErr(cmd, err)
				}
				refs = append(refs, ref)
			}
			if fromConsulted {
				consulted, err := currentProvenanceCandidates(app.WorkspaceID, flowSlug)
				if err != nil {
					return writeErr(cmd, err)
				}
				if len(consulted) == 0 && len(refs) == 0 {
					return writeErr(cmd, errors.New("no consulted flows found; use `breyta flows show` or `breyta flows pull` first, or pass --source"))
				}
				refs = append(refs, consulted...)
			}
			refs = dedupeProvenanceSourceRefs(refs)
			if len(refs) == 0 {
				return writeErr(cmd, errors.New("provide --source, --template, --from-consulted, or --clear"))
			}

			payload["sourceFlows"] = provenanceSourceFlowPayloadItems(refs)
			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "flows.provenance.set", payload)
			}
			return doAPICommand(cmd, app, "flows.provenance.set", payload)
		},
	}

	cmd.Flags().StringArrayVar(&sources, "source", nil, "Source flow ref (<flow-slug> or <workspace-id>/<flow-slug>); repeatable")
	cmd.Flags().StringArrayVar(&templates, "template", nil, "Public template source slug (<template-slug>); repeatable")
	cmd.Flags().BoolVar(&fromConsulted, "from-consulted", false, "Use consulted flows tracked in this agent workspace")
	cmd.Flags().BoolVar(&clear, "clear", false, "Clear all provenance for this flow")
	return cmd
}

func newFlowsDeleteCmd(app *App) *cobra.Command {
	var yes bool
	var force bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "delete <flow-slug>",
		Short: "Delete a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				if yes {
					payload["yes"] = true
				}
				if force {
					payload["force"] = true
				}
				return doAPICommandWithTimeout(cmd, app, "flows.delete", payload, timeout)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if ws.Flows[args[0]] == nil {
				return writeErr(cmd, errors.New("flow not found"))
			}
			delete(ws.Flows, args[0])
			ws.UpdatedAt = time.Now().UTC()
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"deleted": true, "flowSlug": args[0]})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm delete")
	cmd.Flags().BoolVar(&force, "force", false, "Force delete (cancel runs, delete installations)")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "Request timeout for API delete")
	return cmd
}

func newFlowsArchiveCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive <flow-slug>",
		Short: "Archive a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				return doAPICommand(cmd, app, "flows.archive", payload)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			f := ws.Flows[args[0]]
			if f == nil {
				return writeErr(cmd, errors.New("flow not found"))
			}
			f.Tags = append(f.Tags, "archived")
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"archived": true, "flowSlug": args[0]})
		},
	}
	return cmd
}

// --- Steps ------------------------------------------------------------------
