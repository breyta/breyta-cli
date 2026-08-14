package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	pulledFlowSourceMarker          = ";; breyta: pulled-source"
	pulledLegacyFunctionInputMarker = ";; breyta: legacy-bare-function-input "
)

func markPulledFlowSource(flowLiteral string) string {
	flowLiteral = stripPulledFlowSourceMarkers(flowLiteral)
	legacySteps := map[string]bool{}
	for _, diagnostic := range localFunctionStepShapeDiagnostics(flowLiteral, false, nil) {
		if diagnostic["code"] != "function_step_input_shape_invalid" {
			continue
		}
		path, _ := diagnostic["path"].([]string)
		if len(path) >= 2 && path[1] != "" && path[1] != "<missing>" {
			legacySteps[path[1]] = true
		}
	}

	var markers strings.Builder
	markers.WriteString(pulledFlowSourceMarker)
	markers.WriteByte('\n')
	steps := make([]string, 0, len(legacySteps))
	for step := range legacySteps {
		steps = append(steps, step)
	}
	sort.Strings(steps)
	for _, step := range steps {
		markers.WriteString(pulledLegacyFunctionInputMarker)
		markers.WriteString(strconv.Quote(step))
		markers.WriteByte('\n')
	}
	return markers.String() + flowLiteral
}

func stripPulledFlowSourceMarkers(flowLiteral string) string {
	lines := strings.Split(flowLiteral, "\n")
	out := make([]string, 0, len(lines))
	inHeader := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inHeader && trimmed != "" && !strings.HasPrefix(trimmed, ";") {
			inHeader = false
		}
		if inHeader && (trimmed == pulledFlowSourceMarker || strings.HasPrefix(trimmed, pulledLegacyFunctionInputMarker)) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func newFlowsPullCmd(app *App) *cobra.Command {
	var out string
	var target string
	var version int
	cmd := &cobra.Command{
		Use:   "pull <flow-slug>",
		Short: "Pull a flow to a local .clj file for editing",
		Long: strings.TrimSpace(`
Pull an editable flow source file.

By default this pulls the current draft. Use --version N for a read-only
historical source snapshot, or --target live for the live installation target.
`),
		Example: strings.TrimSpace(`
breyta flows pull order-ingest --out ./tmp/flows/order-ingest.clj
breyta flows pull order-ingest --version 6 --out ./tmp/flows/order-ingest-v6.clj
breyta flows pull order-ingest --target live --out ./tmp/flows/order-ingest-live.clj
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Pull requires --api/BREYTA_API_URL")
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}

			slug := args[0]
			path := out
			if strings.TrimSpace(path) == "" {
				filename := slug + ".clj"
				if version > 0 {
					filename = fmt.Sprintf("%s-v%d.clj", slug, version)
				}
				path = filepath.Join("tmp", "flows", filename)
			}
			targetChanged := cmd.Flags().Changed("target")
			resolvedTarget := "draft"
			if targetChanged {
				s, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				resolvedTarget = s
				if resolvedTarget == "live" && version > 0 {
					return writeErr(cmd, errors.New("--target cannot be combined with --version"))
				}
			}

			payload := map[string]any{"flowSlug": slug}
			if resolvedTarget == "live" {
				target, err := resolveLiveProfileTarget(cmd.Context(), app, slug, true)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["source"] = "active"
				if target.Version > 0 {
					payload["version"] = target.Version
				}
			} else if version > 0 {
				payload["source"] = "version"
				payload["version"] = version
			} else {
				payload["source"] = "draft"
			}
			payload["view"] = "full"
			payload["includeFlowLiteral"] = true
			payload["includeTemplates"] = false
			payload["includeFunctions"] = false

			resp, status, err := runAPICommandWithContext(cmd.Context(), app, "flows.get", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				return writeAPIResult(cmd, app, resp, status)
			}

			dataAny, ok := resp["data"]
			if !ok {
				return writeErr(cmd, errors.New("missing data in response"))
			}
			data, ok := dataAny.(map[string]any)
			if !ok {
				return writeErr(cmd, errors.New("invalid data in response"))
			}
			flowLiteral, ok := data["flowLiteral"].(string)
			if !ok || strings.TrimSpace(flowLiteral) == "" {
				return writeErr(cmd, errors.New("missing data.flowLiteral in response"))
			}
			if err := makePublicDir(filepath.Dir(path)); err != nil {
				return writeErr(cmd, err)
			}
			if err := writePublicFile(path, []byte(markPulledFlowSource(flowLiteral)+"\n")); err != nil {
				return writeErr(cmd, err)
			}
			_ = recordConsultedFlow(provenanceSourceRef{
				WorkspaceID: workspaceIDFromEnvelope(resp, app.WorkspaceID),
				FlowSlug:    slug,
			})
			result := map[string]any{"saved": true, "path": path, "flowSlug": slug}
			if targetChanged {
				result["target"] = resolvedTarget
			}
			trackCLIEvent(app, "cli_flow_pulled", nil, app.Token, map[string]any{
				"product":   "flows",
				"channel":   "cli",
				"api_host":  apiHostname(app.APIURL),
				"flow_slug": slug,
				"target":    resolvedTarget,
			})
			return writeData(cmd, app, nil, result)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Output path (default: tmp/flows/<slug>.clj, or tmp/flows/<slug>-vN.clj with --version N)")
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live)")
	cmd.Flags().IntVar(&version, "version", 0, "Version (0 = default)")
	return cmd
}
