package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsDoctorCmd(app *App) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "doctor <slug>",
		Short: "Check compact flow authoring readiness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowSlug := strings.TrimSpace(args[0])
			resolvedTarget, err := normalizeInstallTarget(target)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"flowSlug": flowSlug,
				"target":   resolvedTarget,
			}
			return doFlowsDoctorCommand(cmd, app, flowSlug, resolvedTarget, payload)
		},
	}
	cmd.Flags().StringVar(&target, "target", "draft", "Target to inspect: draft|live")
	return cmd
}

func doFlowsDoctorCommand(cmd *cobra.Command, app *App, flowSlug, target string, payload map[string]any) error {
	out, status, err := buildFlowsDoctorReport(app, flowSlug, target, payload)
	if err != nil {
		return writeErr(cmd, err)
	}
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
}

func buildFlowsDoctorReport(app *App, flowSlug, target string, payload map[string]any) (map[string]any, int, error) {
	out, status, err := runAPICommand(app, "flows.doctor", payload)
	if err != nil {
		return nil, status, err
	}
	if status < 400 && isOK(out) {
		checkPayload := map[string]any{
			"flowSlug": flowSlug,
			"target":   target,
		}
		checkOut, checkStatus, err := runAPICommand(app, "flows.configure.check", checkPayload)
		if err != nil {
			return nil, checkStatus, err
		}
		if flowsConfigureCheckUnsupported(checkOut, checkStatus) {
			return out, status, nil
		}
		mergeFlowsDoctorConfigureReadiness(out, checkOut, checkStatus, flowSlug, target)
	}
	return out, status, nil
}

func newFlowsReadinessCmd(app *App) *cobra.Command {
	var target string
	var includePublicPreflight bool
	var requirePublic bool
	var requireMarketplace bool
	cmd := &cobra.Command{
		Use:   "readiness <slug>",
		Short: "Return one compact flow readiness report",
		Long: strings.TrimSpace(`
Return a compact readiness report that merges definition, configuration, and
public/marketplace preflight checks. Use this before release or public install
proof to avoid stitching together several separate commands. Public preflight is
included by default as a snapshot; pass --public or --marketplace when that
surface should block readiness.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowSlug := strings.TrimSpace(args[0])
			resolvedTarget, err := normalizeInstallTarget(target)
			if err != nil {
				return writeErr(cmd, err)
			}
			return doFlowsReadinessCommand(cmd, app, flowSlug, resolvedTarget, includePublicPreflight || requirePublic || requireMarketplace, requirePublic, requireMarketplace)
		},
	}
	cmd.Flags().StringVar(&target, "target", "live", "Target to inspect: draft|live")
	cmd.Flags().BoolVar(&includePublicPreflight, "public-preflight", true, "Include public Discover/install preflight snapshot")
	cmd.Flags().BoolVar(&requirePublic, "public", false, "Require public Discover/install readiness")
	cmd.Flags().BoolVar(&requireMarketplace, "marketplace", false, "Require marketplace readiness")
	return cmd
}

func newFlowsReleaseCheckCmd(app *App) *cobra.Command {
	var public bool
	var marketplace bool
	cmd := &cobra.Command{
		Use:   "release-check <slug>",
		Short: "Check live/public release readiness",
		Long: strings.TrimSpace(`
Check live release readiness. Add --public or --marketplace to include the
public install and marketplace surfaces in the same compact report.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return doFlowsReadinessCommand(cmd, app, strings.TrimSpace(args[0]), "live", public || marketplace, public, marketplace)
		},
	}
	cmd.Flags().BoolVar(&public, "public", false, "Include public Discover/install preflight")
	cmd.Flags().BoolVar(&marketplace, "marketplace", false, "Include public marketplace preflight")
	return cmd
}

func doFlowsReadinessCommand(cmd *cobra.Command, app *App, flowSlug, target string, includePublic bool, requirePublic bool, requireMarketplace bool) error {
	doctorOut, status, err := buildFlowsDoctorReport(app, flowSlug, target, map[string]any{
		"flowSlug": flowSlug,
		"target":   target,
	})
	if err != nil {
		return writeErr(cmd, err)
	}
	if status >= 400 || !isOK(doctorOut) {
		return writeAPIResult(cmd, app, doctorOut, status)
	}
	publicOut := map[string]any(nil)
	publicStatus := 0
	if includePublic {
		publicOut, publicStatus, err = runAPICommand(app, "flows.public.preflight", map[string]any{"flowSlug": flowSlug})
		if err != nil {
			return writeErr(cmd, err)
		}
		if publicStatus >= 400 || !isOK(publicOut) {
			return writeAPIResult(cmd, app, publicOut, publicStatus)
		}
	}

	readiness := buildFlowsReadinessEnvelope(app, flowSlug, target, doctorOut, publicOut, includePublic, requirePublic, requireMarketplace)
	return writeAPIResult(cmd, app, readiness, 200)
}

func buildFlowsReadinessEnvelope(app *App, flowSlug, target string, doctorOut map[string]any, publicOut map[string]any, includePublic bool, requirePublic bool, requireMarketplace bool) map[string]any {
	doctor := flowsDoctorBody(doctorOut)
	preflight := mapStringAny(mapStringAny(publicOut["data"])["preflight"])
	doctorReady := boolValue(doctor["ready"])
	publicReady := true
	if includePublic {
		publicReady = boolValue(preflight["ready"])
	}
	publicFields := mapStringAny(preflight["public"])
	marketplace := mapStringAny(preflight["marketplace"])
	publicRequired := includePublic && (requirePublic || requireMarketplace || boolValue(publicFields["discoverPublic"]) || boolValue(publicFields["marketplaceVisible"]))
	marketplaceReady := boolValue(marketplace["visible"])
	checks := []any{}
	for _, item := range sliceAny(doctor["checks"]) {
		if m := mapStringAny(item); m != nil {
			check := cloneAnyMap(m)
			check["surface"] = "flow"
			checks = append(checks, check)
		}
	}
	if includePublic {
		for _, item := range sliceAny(preflight["checks"]) {
			if m := mapStringAny(item); m != nil {
				check := cloneAnyMap(m)
				check["surface"] = "public"
				checks = append(checks, check)
			}
		}
	}
	if requireMarketplace {
		checks = append(checks, map[string]any{
			"id":      "marketplace-visible",
			"label":   "Marketplace visible",
			"pass":    marketplaceReady,
			"surface": "marketplace",
			"hint":    "Run `breyta flows marketplace update " + flowSlug + " --visible=true` after explicit author approval.",
		})
	}
	blockers := []any{}
	for _, item := range checks {
		if m := mapStringAny(item); m != nil && !boolValue(m["pass"]) {
			surface := strings.ToLower(firstNonBlankString(m["surface"]))
			if surface == "flow" || (surface == "public" && publicRequired) || (surface == "marketplace" && requireMarketplace) {
				blockers = append(blockers, m)
			}
		}
	}
	nextCommands := []string{}
	if meta := mapStringAny(doctorOut["meta"]); meta != nil {
		nextCommands = appendUniqueStrings(nextCommands, stringSlice(meta["nextCommands"]))
	}
	if meta := mapStringAny(publicOut["meta"]); meta != nil {
		nextCommands = appendUniqueStrings(nextCommands, stringSlice(meta["nextCommands"]))
	}
	if len(nextCommands) == 0 {
		nextCommands = []string{
			fmt.Sprintf("breyta flows show %s --target %s", flowSlug, target),
			fmt.Sprintf("breyta flows run %s --target %s --wait", flowSlug, target),
		}
	}
	webURL := firstNonBlankString(
		mapStringAny(doctorOut["meta"])["webUrl"],
		mapStringAny(publicOut["meta"])["webUrl"],
		doctor["webUrl"],
		preflight["webUrl"],
	)
	summary := mapStringAny(doctor["summary"])
	activeVersion := firstPresent(summary, "activeVersion", "active-version")
	latestVersion := firstPresent(summary, "latestVersion", "latest-version")
	return map[string]any{
		"ok":          true,
		"workspaceId": app.WorkspaceID,
		"meta": map[string]any{
			"webUrl":       webURL,
			"nextCommands": nextCommands,
			"hint":         "Readiness is compact by default. Drill into the failed check's next command instead of running broad inventory.",
		},
		"data": map[string]any{
			"readiness": map[string]any{
				"flowSlug":            flowSlug,
				"target":              target,
				"ready":               doctorReady && (!publicRequired || publicReady) && (!requireMarketplace || marketplaceReady),
				"definitionReady":     boolValue(doctor["definitionReady"]),
				"configurationReady":  boolValue(doctor["configurationReady"]),
				"publicReady":         publicReady,
				"publicIncluded":      includePublic,
				"publicRequired":      publicRequired,
				"marketplaceReady":    marketplaceReady,
				"marketplaceRequired": requireMarketplace,
				"summary":             doctor["summary"],
				"draftLive":           map[string]any{"activeVersion": activeVersion, "latestVersion": latestVersion, "draftAhead": versionsSuggestDraftAhead(activeVersion, latestVersion)},
				"configuration":       doctor["configuration"],
				"public":              preflight["public"],
				"discover":            preflight["discover"],
				"marketplace":         preflight["marketplace"],
				"installability":      preflight["installability"],
				"pricing":             preflight["pricing"],
				"checks":              checks,
				"blockers":            blockers,
				"doctor":              doctor,
				"publicPreflight":     preflight,
			},
		},
	}
}

func versionsSuggestDraftAhead(active any, latest any) bool {
	activeFloat, activeOK := numericValue(active)
	latestFloat, latestOK := numericValue(latest)
	return activeOK && latestOK && latestFloat > activeFloat
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		s := scalarString(value)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
}

func flowsConfigureCheckUnsupported(out map[string]any, status int) bool {
	if status < 400 && isOK(out) {
		return false
	}
	errMap := mapStringAny(out["error"])
	code := strings.ToLower(firstNonBlankString(errMap["code"], out["code"]))
	if code == "unknown_command" || code == "command_not_found" {
		return true
	}
	msg := strings.ToLower(firstNonBlankString(errMap["message"], out["message"]))
	return strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "unexpected command") ||
		strings.Contains(msg, "unsupported command")
}

func mergeFlowsDoctorConfigureReadiness(doctorOut map[string]any, checkOut map[string]any, checkStatus int, flowSlug, target string) {
	doctor := flowsDoctorBody(doctorOut)
	if doctor == nil {
		return
	}

	definitionReady := boolValue(doctor["ready"])
	config := map[string]any{
		"flowSlug": flowSlug,
		"ready":    false,
		"source":   "flows.configure.check",
		"target":   target,
	}
	if checkData := mapStringAny(checkOut["data"]); checkData != nil {
		config = cloneAnyMap(checkData)
		config["source"] = "flows.configure.check"
		if _, ok := config["target"]; !ok {
			config["target"] = target
		}
	}
	configReady := checkStatus < 400 && isOK(checkOut) && boolValue(config["ready"])
	config["ready"] = configReady
	doctor["definitionReady"] = definitionReady
	doctor["configurationReady"] = configReady
	doctor["configuration"] = config
	doctor["ready"] = definitionReady && configReady
	upsertFlowsDoctorCheck(doctor, map[string]any{
		"id":     "configuration",
		"label":  "Required configuration",
		"pass":   configReady,
		"source": "flows.configure.check",
	})

	if configReady {
		return
	}
	if checkStatus >= 400 || !isOK(checkOut) {
		config["statusCode"] = checkStatus
		if errMap := mapStringAny(checkOut["error"]); errMap != nil {
			config["error"] = errMap
		}
	}
	meta := ensureMeta(doctorOut)
	if meta == nil {
		return
	}
	meta["hint"] = "Required configuration is not ready; resolve `breyta flows configure check` before running this flow."
	blockedNextCommands := flowsDoctorBlockedNextCommands(flowSlug, target)
	if definitionReady {
		meta["nextCommands"] = blockedNextCommands
		return
	}
	meta["nextCommands"] = appendUniqueStrings(stringSlice(meta["nextCommands"]), blockedNextCommands)
}

func flowsDoctorBody(out map[string]any) map[string]any {
	data := mapStringAny(out["data"])
	if data == nil {
		return nil
	}
	return mapStringAny(data["doctor"])
}

func upsertFlowsDoctorCheck(doctor map[string]any, check map[string]any) {
	checks := sliceAny(doctor["checks"])
	for i, item := range checks {
		existing := mapStringAny(item)
		if firstNonBlankString(existing["id"]) == firstNonBlankString(check["id"]) {
			checks[i] = check
			doctor["checks"] = checks
			return
		}
	}
	doctor["checks"] = append(checks, check)
}

func flowsDoctorBlockedNextCommands(flowSlug, target string) []string {
	targetFlag := ""
	if strings.TrimSpace(target) != "" {
		targetFlag = fmt.Sprintf(" --target %s", strings.TrimSpace(target))
	}
	return []string{
		fmt.Sprintf("breyta flows configure check %s%s", flowSlug, targetFlag),
		fmt.Sprintf("breyta flows configure suggest %s%s", flowSlug, targetFlag),
		"breyta connections list",
	}
}

func stringSlice(value any) []string {
	items := sliceAny(value)
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s := strings.TrimSpace(scalarString(item))
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func appendUniqueStrings(base []string, additions []string) []string {
	out := make([]string, 0, len(base)+len(additions))
	seen := map[string]struct{}{}
	for _, item := range append(base, additions...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

func newFlowsPublicCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "public",
		Short: "Inspect public-flow readiness",
	}
	cmd.AddCommand(newFlowsPublicPreflightCmd(app))
	return cmd
}

func newFlowsPublicPreflightCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preflight <slug>",
		Short: "Check public readiness without changing visibility",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
			}
			return doAPICommand(cmd, app, "flows.public.preflight", payload)
		},
	}
	return cmd
}
