package cli

import (
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/state"
	"github.com/spf13/cobra"
)

func newFlowsVersionsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List published versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return doAPICommand(cmd, app, "flows.versions.list", map[string]any{"flowSlug": args[0]})
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]map[string]any, 0, len(f.Versions))
			for _, v := range f.Versions {
				item := map[string]any{
					"version":     v.Version,
					"publishedAt": v.PublishedAt,
					"note":        v.Note,
				}
				if strings.TrimSpace(v.Note) != "" {
					item["releaseNote"] = v.Note
				}
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool { return items[i]["version"].(int) > items[j]["version"].(int) })
			meta := map[string]any{"activeVersion": f.ActiveVersion}
			return writeData(cmd, app, meta, map[string]any{"flowSlug": f.Slug, "items": items})
		},
	}
	return cmd
}

func newFlowsVersionsPublishCmd(app *App) *cobra.Command {
	var releaseNote string
	var releaseNoteFile string
	var legacyNote string
	cmd := &cobra.Command{
		Use:   "publish <flow-slug>",
		Short: "Publish a new immutable version",
		Long: strings.TrimSpace(`
Publish a new immutable version.

Attach a markdown release note when you know what changed:
- breyta flows versions publish my-flow --release-note 'Added retry guard'
- breyta flows versions publish my-flow --release-note-file ./release-note.md
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedReleaseNote, err := resolveReleaseNoteInput(releaseNote, legacyNote, releaseNoteFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				if strings.TrimSpace(resolvedReleaseNote) != "" {
					payload["releaseNote"] = resolvedReleaseNote
				}
				return doAPICommand(cmd, app, "flows.versions.publish", payload)
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
			now := time.Now().UTC()
			next := maxVersion(f) + 1
			fv := state.FlowVersion{
				Version:     next,
				PublishedAt: now,
				Note:        resolvedReleaseNote,
				Flow: state.FlowRecord{
					Name:        f.Name,
					Description: f.Description,
					Tags:        append([]string{}, f.Tags...),
					Spine:       append([]string{}, f.Spine...),
					Steps:       append([]state.FlowStep{}, f.Steps...),
				},
			}
			f.Versions = append(f.Versions, fv)
			f.ActiveVersion = next
			f.UpdatedAt = now
			ws.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "publishedVersion": next})
		},
	}
	cmd.Flags().StringVar(&releaseNote, "release-note", "", "Markdown release note")
	cmd.Flags().StringVar(&releaseNoteFile, "release-note-file", "", "Read markdown release note from file")
	cmd.Flags().StringVar(&legacyNote, "note", "", "Deprecated alias for --release-note")
	_ = cmd.Flags().MarkHidden("note")
	return cmd
}

func newFlowsVersionsUpdateCmd(app *App) *cobra.Command {
	var version int
	var releaseNote string
	var releaseNoteFile string
	var legacyNote string
	var clearReleaseNote bool

	cmd := &cobra.Command{
		Use:   "update <flow-slug>",
		Short: "Update version metadata such as the release note",
		Long: strings.TrimSpace(`
Update version metadata without publishing a new version.

Examples:
- breyta flows versions update my-flow --version 7 --release-note-file ./release-note.md
- breyta flows versions update my-flow --version 7 --clear-release-note
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if version <= 0 {
				return writeErr(cmd, errors.New("missing --version"))
			}
			hasReleaseNoteInput := strings.TrimSpace(releaseNote) != "" ||
				strings.TrimSpace(legacyNote) != "" ||
				strings.TrimSpace(releaseNoteFile) != ""
			if clearReleaseNote && hasReleaseNoteInput {
				return writeErr(cmd, errors.New("--clear-release-note cannot be combined with --release-note/--release-note-file"))
			}
			resolvedReleaseNote, err := resolveReleaseNoteInput(releaseNote, legacyNote, releaseNoteFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if !clearReleaseNote && strings.TrimSpace(resolvedReleaseNote) == "" {
				return writeErr(cmd, errors.New("missing --release-note/--release-note-file or --clear-release-note"))
			}

			if isAPIMode(app) {
				payload := map[string]any{
					"flowSlug": args[0],
					"version":  version,
				}
				if clearReleaseNote {
					payload["clearReleaseNote"] = true
				} else {
					payload["releaseNote"] = resolvedReleaseNote
				}
				return doAPICommand(cmd, app, "flows.versions.update", payload)
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
			for i := range f.Versions {
				if f.Versions[i].Version != version {
					continue
				}
				if clearReleaseNote {
					f.Versions[i].Note = ""
				} else {
					f.Versions[i].Note = resolvedReleaseNote
				}
				f.UpdatedAt = time.Now().UTC()
				ws.UpdatedAt = f.UpdatedAt
				if err := store.Save(st); err != nil {
					return writeErr(cmd, err)
				}
				versionOut := map[string]any{
					"version": version,
					"note":    f.Versions[i].Note,
				}
				if strings.TrimSpace(f.Versions[i].Note) != "" {
					versionOut["releaseNote"] = f.Versions[i].Note
				}
				return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "version": versionOut})
			}
			return writeErr(cmd, errors.New("version not found"))
		},
	}

	cmd.Flags().IntVar(&version, "version", 0, "Version to update")
	cmd.Flags().StringVar(&releaseNote, "release-note", "", "Markdown release note")
	cmd.Flags().StringVar(&releaseNoteFile, "release-note-file", "", "Read markdown release note from file")
	cmd.Flags().StringVar(&legacyNote, "note", "", "Deprecated alias for --release-note")
	cmd.Flags().BoolVar(&clearReleaseNote, "clear-release-note", false, "Clear the release note for this version")
	_ = cmd.Flags().MarkHidden("note")
	return cmd
}

func newFlowsVersionsActivateCmd(app *App) *cobra.Command {
	var version int
	var deployKey string
	cmd := &cobra.Command{
		Use:   "activate <flow-slug>",
		Short: "Activate a published version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if version == 0 {
				return writeErr(cmd, errors.New("missing --version"))
			}
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0], "version": version}
				resolvedDeployKey := strings.TrimSpace(deployKey)
				if resolvedDeployKey == "" {
					resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
				}
				if resolvedDeployKey != "" {
					payload["deployKey"] = resolvedDeployKey
				}
				return doAPICommand(cmd, app, "flows.versions.activate", payload)
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
			v, ok := findVersion(f, version)
			if !ok {
				return writeErr(cmd, errors.New("version not found"))
			}
			// Mock behavior: activation also swaps current draft to that snapshot.
			f.ActiveVersion = v.Version
			f.Name = v.Flow.Name
			f.Description = v.Flow.Description
			f.Tags = append([]string{}, v.Flow.Tags...)
			f.Spine = append([]string{}, v.Flow.Spine...)
			f.Steps = append([]state.FlowStep{}, v.Flow.Steps...)
			f.Spine = buildSpine(f)
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flow": f})
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "Version")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key (default: BREYTA_FLOW_DEPLOY_KEY)")
	return cmd
}

func newFlowsVersionsDiffCmd(app *App) *cobra.Command {
	var from, to int
	var full bool
	cmd := &cobra.Command{
		Use:   "diff <flow-slug>",
		Short: "Diff two versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if from == 0 || to == 0 {
					return writeErr(cmd, errors.New("missing --from and/or --to"))
				}
				payload := map[string]any{
					"flowSlug":    args[0],
					"from":        "version",
					"fromVersion": from,
					"to":          "version",
					"toVersion":   to,
				}
				if full {
					payload["view"] = "full"
				} else {
					payload["view"] = "summary"
				}
				return doAPICommand(cmd, app, "flows.diff", payload)
			}
			if from == 0 || to == 0 {
				return writeErr(cmd, errors.New("missing --from and/or --to"))
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			vf, ok := findVersion(f, from)
			if !ok {
				return writeErr(cmd, errors.New("from version not found"))
			}
			vt, ok := findVersion(f, to)
			if !ok {
				return writeErr(cmd, errors.New("to version not found"))
			}
			d := diffSteps(vf.Flow.Steps, vt.Flow.Steps)
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "from": from, "to": to, "diff": d})
		},
	}
	cmd.Flags().IntVar(&from, "from", 0, "From version")
	cmd.Flags().IntVar(&to, "to", 0, "To version")
	cmd.Flags().BoolVar(&full, "full", false, "Include the full unified diff in API mode")
	return cmd
}

// --- Validate ----------------------------------------------------------------
