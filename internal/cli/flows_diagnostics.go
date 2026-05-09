package cli

import (
	"fmt"
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
	out, status, err := runAPICommand(app, "flows.doctor", payload)
	if err != nil {
		return writeErr(cmd, err)
	}
	if status < 400 && isOK(out) {
		checkPayload := map[string]any{
			"flowSlug": flowSlug,
			"target":   target,
		}
		checkOut, checkStatus, err := runAPICommand(app, "flows.configure.check", checkPayload)
		if err != nil {
			return writeErr(cmd, err)
		}
		if flowsConfigureCheckUnsupported(checkOut, checkStatus) {
			return writeAPIResult(cmd, app, out, status)
		}
		mergeFlowsDoctorConfigureReadiness(out, checkOut, checkStatus, flowSlug, target)
	}
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
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
