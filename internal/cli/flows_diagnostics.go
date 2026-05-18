package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
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
	var full bool
	cmd := &cobra.Command{
		Use:   "readiness <slug>",
		Short: "Return one compact flow readiness report",
		Long: strings.TrimSpace(`
Compact draft/live readiness report. Use --public or --marketplace when those
surfaces should block readiness. Use --full for raw doctor/preflight payloads.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowSlug := strings.TrimSpace(args[0])
			resolvedTarget, err := normalizeInstallTarget(target)
			if err != nil {
				return writeErr(cmd, err)
			}
			return doFlowsReadinessCommand(cmd, app, flowSlug, resolvedTarget, includePublicPreflight || requirePublic || requireMarketplace, requirePublic, requireMarketplace, full)
		},
	}
	cmd.Flags().StringVar(&target, "target", "draft", "Target to inspect: draft|live")
	cmd.Flags().BoolVar(&includePublicPreflight, "public-preflight", true, "Include public Discover/install preflight snapshot")
	cmd.Flags().BoolVar(&requirePublic, "public", false, "Require public Discover/install readiness")
	cmd.Flags().BoolVar(&requireMarketplace, "marketplace", false, "Require marketplace readiness")
	cmd.Flags().BoolVar(&full, "full", false, "Include raw doctor and public preflight payloads")
	return cmd
}

func newFlowsReleaseCheckCmd(app *App) *cobra.Command {
	var public bool
	var marketplace bool
	var full bool
	cmd := &cobra.Command{
		Use:   "release-check <slug>",
		Short: "Check live/public release readiness",
		Long: strings.TrimSpace(`
Check live release readiness. Add --public or --marketplace to include the
public install and marketplace surfaces in the same compact report.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return doFlowsReadinessCommand(cmd, app, strings.TrimSpace(args[0]), "live", public || marketplace, public, marketplace, full)
		},
	}
	cmd.Flags().BoolVar(&public, "public", false, "Include public Discover/install preflight")
	cmd.Flags().BoolVar(&marketplace, "marketplace", false, "Include public marketplace preflight")
	cmd.Flags().BoolVar(&full, "full", false, "Include raw doctor and public preflight payloads")
	return cmd
}

func doFlowsReadinessCommand(cmd *cobra.Command, app *App, flowSlug, target string, includePublic bool, requirePublic bool, requireMarketplace bool, full bool) error {
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

	invocationMetrics := buildFlowsInvocationMetricsReport(app, flowSlug)
	diffReport := buildFlowsReadinessDiffReport(app, flowSlug)
	readiness := buildFlowsReadinessEnvelope(app, flowSlug, target, doctorOut, publicOut, invocationMetrics, diffReport, includePublic, requirePublic, requireMarketplace, full)
	return writeAPIResult(cmd, app, readiness, 200)
}

func buildFlowsReadinessEnvelope(app *App, flowSlug, target string, doctorOut map[string]any, publicOut map[string]any, invocationMetrics map[string]any, diffReport map[string]any, includePublic bool, requirePublic bool, requireMarketplace bool, full bool) map[string]any {
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
			decorateReadinessCheck(flowSlug, target, check, true)
			checks = append(checks, check)
		}
	}
	if includePublic {
		for _, item := range sliceAny(preflight["checks"]) {
			if m := mapStringAny(item); m != nil {
				check := cloneAnyMap(m)
				check["surface"] = "public"
				decorateReadinessCheck(flowSlug, target, check, publicRequired)
				checks = append(checks, check)
			}
		}
	}
	if requireMarketplace {
		check := map[string]any{
			"id":      "marketplace-visible",
			"label":   "Marketplace visible",
			"pass":    marketplaceReady,
			"surface": "marketplace",
			"hint":    "Enable marketplace visibility after approval.",
		}
		decorateReadinessCheck(flowSlug, target, check, true)
		checks = append(checks, check)
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
	nextCommands = appendUniqueStrings(nextCommands, readinessFixCommands(blockers))
	if boolValue(diffReport["changed"]) {
		nextCommands = appendUniqueStrings(nextCommands, []string{fmt.Sprintf("breyta flows diff %s", flowSlug)})
	}
	if len(nextCommands) == 0 {
		nextCommands = []string{
			fmt.Sprintf("breyta flows show %s --target %s", flowSlug, target),
			fmt.Sprintf("breyta flows run %s --target %s --wait", flowSlug, target),
		}
	}
	readinessURLs := buildReadinessURLs(app, flowSlug, invocationMetrics)
	attachReadinessCheckURLs(checks, readinessURLs)
	nextActions := buildReadinessNextActions(readinessURLs)
	webURL := firstNonBlankString(
		mapStringAny(doctorOut["meta"])["webUrl"],
		mapStringAny(publicOut["meta"])["webUrl"],
		doctor["webUrl"],
		preflight["webUrl"],
	)
	webURL = normalizeLocalhostWebURL(webURL)
	summary := mapStringAny(doctor["summary"])
	activeVersion := firstPresent(summary, "activeVersion", "active-version")
	latestVersion := firstPresent(summary, "latestVersion", "latest-version")
	workspaceID := firstNonBlankString(doctorOut["workspaceId"], publicOut["workspaceId"], app.WorkspaceID)
	readiness := map[string]any{
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
		"draftLive":           buildReadinessDraftLive(activeVersion, latestVersion, diffReport),
		"configuration":       doctor["configuration"],
		"public":              preflight["public"],
		"discover":            preflight["discover"],
		"marketplace":         preflight["marketplace"],
		"installability":      preflight["installability"],
		"pricing":             preflight["pricing"],
		"checks":              checks,
		"blockers":            blockers,
	}
	if len(readinessURLs) > 0 {
		readiness["urls"] = readinessURLs
	}
	if len(diffReport) > 0 {
		readiness["diff"] = diffReport
	}
	if len(invocationMetrics) > 0 {
		readiness["invocationMetrics"] = invocationMetrics
		if latest := mapStringAny(invocationMetrics["latestInvocation"]); latest != nil {
			readiness["latestInvocation"] = latest
		}
		if latestInstalled := mapStringAny(invocationMetrics["latestInstalledRun"]); latestInstalled != nil {
			readiness["latestInstalledRun"] = latestInstalled
		}
	}
	if full {
		readiness["doctor"] = doctor
		readiness["publicPreflight"] = preflight
	}
	meta := map[string]any{
		"webUrl":       webURL,
		"nextCommands": nextCommands,
		"hint":         "Readiness is compact. Follow blocker nextCommands.",
	}
	if len(nextActions) > 0 {
		meta["nextActions"] = nextActions
	}
	return map[string]any{
		"ok":          true,
		"workspaceId": workspaceID,
		"meta":        meta,
		"data": map[string]any{
			"readiness": readiness,
		},
	}
}

func buildFlowsReadinessDiffReport(app *App, flowSlug string) map[string]any {
	out, status, err := runAPICommand(app, "flows.diff", map[string]any{
		"flowSlug": flowSlug,
		"from":     "live",
		"to":       "draft",
		"view":     "summary",
	})
	if err != nil || status >= 400 || !isOK(out) {
		return nil
	}
	return compactFlowsReadinessDiff(out)
}

func compactFlowsReadinessDiff(out map[string]any) map[string]any {
	data := mapStringAny(out["data"])
	if data == nil {
		return nil
	}
	diff := compactNonEmptyFields(map[string]any{
		"source":          "flows.diff",
		"flowSlug":        firstNonBlankString(data["flowSlug"], data["flow-slug"]),
		"changed":         firstPresentAny(data["changed"]),
		"from":            compactReadinessDiffSide(mapStringAny(data["from"])),
		"to":              compactReadinessDiffSide(mapStringAny(data["to"])),
		"stat":            mapStringAny(data["stat"]),
		"changedSections": sliceAny(data["changedSections"]),
	})
	if len(diff) == 0 {
		return nil
	}
	return diff
}

func compactReadinessDiffSide(side map[string]any) map[string]any {
	if side == nil {
		return nil
	}
	out := compactNonEmptyFields(map[string]any{
		"source":  firstNonBlankString(side["source"]),
		"version": firstPresentAny(side["version"]),
		"label":   firstNonBlankString(side["label"]),
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildReadinessDraftLive(activeVersion any, latestVersion any, diffReport map[string]any) map[string]any {
	draftLive := map[string]any{
		"activeVersion": activeVersion,
		"latestVersion": latestVersion,
		"draftAhead":    versionsSuggestDraftAhead(activeVersion, latestVersion),
	}
	if len(diffReport) > 0 {
		draftLive["diff"] = diffReport
		if changed := firstPresentAny(diffReport["changed"]); changed != nil {
			draftLive["changed"] = changed
		}
		if from := mapStringAny(diffReport["from"]); from != nil {
			draftLive["from"] = from
		}
		if to := mapStringAny(diffReport["to"]); to != nil {
			draftLive["to"] = to
		}
		if stat := mapStringAny(diffReport["stat"]); stat != nil {
			draftLive["stat"] = stat
		}
	}
	return draftLive
}

func buildReadinessURLs(app *App, flowSlug string, invocationMetrics map[string]any) map[string]any {
	base := normalizeLocalhostWebURL(workspaceWebBaseURL(app))
	if strings.TrimSpace(base) == "" {
		return nil
	}
	latest := mapStringAny(invocationMetrics["latestInstalledRun"])
	if latest == nil {
		latest = mapStringAny(invocationMetrics["latestInvocation"])
	}
	installationID := firstNonBlankString(latest["installationId"])
	workflowID := firstNonBlankString(latest["workflowId"])
	installationURL := installationWebURL(base, flowSlug, installationID)
	urls := compactNonEmptyFields(map[string]any{
		"flow":                    flowWebURL(base, flowSlug),
		"publicApp":               flowInstallationsWebURL(base, flowSlug),
		"install":                 flowInstallationsWebURL(base, flowSlug),
		"discover":                webURL(base, "discover"),
		"runs":                    flowRunsWebURL(base, flowSlug),
		"installation":            installationURL,
		"installationSetup":       appendReadinessQuery(installationURL, "configure", "setup"),
		"installationManageSetup": appendReadinessQuery(installationURL, "configure", "manage-setup"),
		"latestRun":               runWebURL(base, flowSlug, workflowID),
		"latestRunOutput":         runOutputWebURL(base, flowSlug, workflowID),
	})
	if len(urls) == 0 {
		return nil
	}
	return urls
}

func appendReadinessQuery(rawURL, key, value string) string {
	rawURL = strings.TrimSpace(rawURL)
	key = strings.TrimSpace(key)
	if rawURL == "" || key == "" {
		return ""
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + url.QueryEscape(key) + "=" + url.QueryEscape(value)
}

func attachReadinessCheckURLs(checks []any, urls map[string]any) {
	if len(urls) == 0 {
		return
	}
	for _, item := range checks {
		check := mapStringAny(item)
		if check == nil {
			continue
		}
		if firstNonBlankString(check["openUrl"]) != "" {
			continue
		}
		if openURL := readinessCheckOpenURL(check, urls); openURL != "" {
			check["openUrl"] = openURL
		}
	}
}

func readinessCheckOpenURL(check map[string]any, urls map[string]any) string {
	id := strings.ToLower(firstNonBlankString(check["id"]))
	surface := strings.ToLower(firstNonBlankString(check["surface"]))
	switch {
	case id == "discover-public":
		return firstNonBlankString(urls["discover"])
	case strings.Contains(id, "marketplace"):
		return firstNonBlankString(urls["publicApp"], urls["install"])
	case strings.Contains(id, "install"):
		return firstNonBlankString(urls["install"], urls["publicApp"])
	case surface == "public":
		return firstNonBlankString(urls["publicApp"], urls["install"])
	case surface == "marketplace":
		return firstNonBlankString(urls["publicApp"], urls["install"])
	default:
		return firstNonBlankString(urls["flow"])
	}
}

func buildReadinessNextActions(urls map[string]any) []any {
	if len(urls) == 0 {
		return nil
	}
	actions := []any{}
	actions = appendReadinessNextAction(actions, "open-flow", "Open flow", firstNonBlankString(urls["flow"]))
	actions = appendReadinessNextAction(actions, "open-public-app", "Open public app", firstNonBlankString(urls["publicApp"]))
	actions = appendReadinessNextAction(actions, "open-latest-installation", "Open latest installation", firstNonBlankString(urls["installation"]))
	actions = appendReadinessNextAction(actions, "configure-latest-installation", "Configure latest installation", firstNonBlankString(urls["installationSetup"]))
	actions = appendReadinessNextAction(actions, "open-latest-run", "Open latest run", firstNonBlankString(urls["latestRun"]))
	if len(actions) == 0 {
		return nil
	}
	return actions
}

func appendReadinessNextAction(actions []any, id, label, rawURL string) []any {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return actions
	}
	return append(actions, map[string]any{
		"id":    id,
		"label": label,
		"url":   rawURL,
	})
}

func buildFlowsInvocationMetricsReport(app *App, flowSlug string) map[string]any {
	out, status, err := runAPICommand(app, "flows.invocations.metrics", map[string]any{
		"flowSlug": flowSlug,
		"limit":    10,
	})
	if err != nil || status >= 400 || !isOK(out) {
		return nil
	}
	return compactFlowsInvocationMetrics(out)
}

func compactFlowsInvocationMetrics(out map[string]any) map[string]any {
	data := mapStringAny(out["data"])
	items := sliceAny(data["items"])
	if len(items) == 0 {
		return map[string]any{
			"source": "flows.invocations.metrics",
			"count":  0,
		}
	}
	var latest map[string]any
	var latestInstalled map[string]any
	for _, item := range items {
		metric := compactReadinessInvocationMetric(mapStringAny(item))
		if metric == nil {
			continue
		}
		if latest == nil {
			latest = metric
		}
		if latestInstalled == nil && firstNonBlankString(metric["installationId"]) != "" {
			latestInstalled = metric
		}
	}
	outMetrics := map[string]any{
		"source": "flows.invocations.metrics",
		"count":  len(items),
	}
	if latest != nil {
		outMetrics["latestInvocation"] = latest
	}
	if latestInstalled != nil {
		outMetrics["latestInstalledRun"] = latestInstalled
	}
	return outMetrics
}

func compactReadinessInvocationMetric(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	metric := compactNonEmptyFields(map[string]any{
		"workflowId":       firstNonBlankString(item["lastWorkflowId"], item["last-workflow-id"], item["workflowId"], item["workflow-id"]),
		"installationId":   firstNonBlankString(item["installationId"], item["installation-id"]),
		"entrypointId":     firstNonBlankString(item["entrypointId"], item["entrypoint-id"]),
		"invocationId":     firstNonBlankString(item["invocationId"], item["invocation-id"]),
		"invocationKind":   firstNonBlankString(item["invocationKind"], item["invocation-kind"]),
		"interfaceScope":   firstNonBlankString(item["interfaceScope"], item["interface-scope"]),
		"authMode":         firstNonBlankString(item["authMode"], item["auth-mode"]),
		"lastCalledAt":     firstNonBlankString(item["lastCalledAt"], item["last-called-at"]),
		"lastStatus":       firstNonBlankString(item["lastStatus"], item["last-status"]),
		"lastStatusBucket": firstNonBlankString(item["lastStatusBucket"], item["last-status-bucket"]),
		"lastErrorCode":    firstNonBlankString(item["lastErrorCode"], item["last-error-code"]),
		"requestCount":     firstPresentAny(item["requestCount"], item["request-count"]),
		"successCount":     firstPresentAny(item["successCount"], item["success-count"]),
		"errorCount":       firstPresentAny(item["errorCount"], item["error-count"]),
	})
	if len(metric) == 0 {
		return nil
	}
	return metric
}

func decorateReadinessCheck(flowSlug, target string, check map[string]any, required bool) {
	if check == nil {
		return
	}
	pass := boolValue(check["pass"])
	if _, ok := check["status"]; !ok {
		switch {
		case pass:
			check["status"] = "pass"
		case required:
			check["status"] = "fail"
		default:
			check["status"] = "warn"
		}
	}
	if pass {
		return
	}
	if _, ok := check["fixCommand"]; ok {
		return
	}
	if fixCommand := readinessFixCommand(flowSlug, target, check); fixCommand != "" {
		check["fixCommand"] = fixCommand
	}
}

func readinessFixCommands(blockers []any) []string {
	out := []string{}
	for _, item := range blockers {
		if m := mapStringAny(item); m != nil {
			out = appendUniqueStrings(out, []string{firstNonBlankString(m["fixCommand"])})
		}
	}
	return out
}

func readinessFixCommand(flowSlug, target string, check map[string]any) string {
	id := strings.ToLower(firstNonBlankString(check["id"]))
	target = strings.TrimSpace(target)
	targetFlag := ""
	if target != "" {
		targetFlag = " --target " + target
	}
	switch id {
	case "configuration":
		return fmt.Sprintf("breyta flows configure suggest %s%s", flowSlug, targetFlag)
	case "definition", "steps", "entrypoints":
		return fmt.Sprintf("breyta flows pull %s --out ./tmp/flows/%s.clj", flowSlug, flowSlug)
	case "live-version", "released":
		return fmt.Sprintf("breyta flows release %s --release-note-file ./release-note.md", flowSlug)
	case "discover-public":
		return fmt.Sprintf("breyta flows discover update %s --public=true", flowSlug)
	case "marketplace-visible", "marketplace-visibility":
		return fmt.Sprintf("breyta flows marketplace update %s --visible=true", flowSlug)
	default:
		return ""
	}
}

func normalizeLocalhostWebURL(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "https://localhost") || strings.HasPrefix(value, "https://127.0.0.1") {
		return "http://" + strings.TrimPrefix(value, "https://")
	}
	return value
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
