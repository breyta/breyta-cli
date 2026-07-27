package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/spf13/cobra"
)

var (
	localQualifiedStepIDRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*(?:\.[a-zA-Z][a-zA-Z0-9_-]*)*/[a-zA-Z][a-zA-Z0-9_-]{0,127}$`)
	localScheduleIDRe      = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`)
)

func localStepIDValid(stepID string) bool {
	return localQualifiedStepIDRe.MatchString(strings.TrimSpace(stepID))
}

func localStepTypeValid(stepType string) bool {
	return validFlowLintSafeIdentifier(strings.TrimPrefix(strings.TrimSpace(stepType), ":"))
}

func localScheduleIDValid(scheduleID string) bool {
	return localScheduleIDRe.MatchString(strings.TrimSpace(scheduleID))
}

func defaultLocalFlowPath(slug string) string {
	return filepath.Join("flows", strings.TrimSpace(slug)+".clj")
}

func resolveLocalFlowPath(slug, requested string) string {
	if path := strings.TrimSpace(requested); path != "" {
		return path
	}
	return defaultLocalFlowPath(slug)
}

func readLocalFlowSource(slug, requested string) (string, string, error) {
	path := resolveLocalFlowPath(slug, requested)
	b, err := readExplicitFile(path)
	if err != nil {
		return path, "", fmt.Errorf("read local flow %s: %w", path, err)
	}
	source := string(b)
	if strings.TrimSpace(source) == "" {
		return path, "", fmt.Errorf("local flow %s is empty", path)
	}
	entries, err := parseSingleTopLevelMapEntries(source)
	if err != nil {
		return path, "", fmt.Errorf("local flow source must be one complete top-level map: %w", err)
	}
	literalSlug, err := localFlowSlugFromEntries(source, entries)
	if err != nil {
		return path, "", fmt.Errorf("local flow source has no valid :slug: %w", err)
	}
	requestedSlug := strings.TrimPrefix(strings.TrimSpace(slug), ":")
	if literalSlug != requestedSlug {
		return path, "", fmt.Errorf("local flow source slug %q does not match requested flow %q", literalSlug, requestedSlug)
	}
	return path, source, nil
}

func localFlowSlugFromEntries(source string, entries []clojureMapEntry) (string, error) {
	slugEntry, found := mapEntryByKey(entries, "slug")
	if !found {
		return "", errors.New("top-level :slug is missing")
	}
	token := strings.TrimSpace(source[slugEntry.ValueStart:slugEntry.ValueEnd])
	if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") {
		var decoded string
		if err := json.Unmarshal([]byte(token), &decoded); err != nil {
			return "", fmt.Errorf("decode :slug: %w", err)
		}
		token = decoded
	} else {
		if !strings.HasPrefix(token, ":") {
			return "", fmt.Errorf("expected keyword or string, got %s", token)
		}
		token = strings.TrimPrefix(token, ":")
	}
	if !isAPIValidFlowSlug(token) {
		return "", fmt.Errorf("invalid flow slug %q", token)
	}
	return token, nil
}

func localTopLevelEntry(source, name string) (clojureMapEntry, bool, error) {
	entries, err := extractTopLevelMapEntries(source)
	if err != nil {
		return clojureMapEntry{}, false, err
	}
	wanted := ":" + strings.TrimPrefix(strings.TrimSpace(name), ":")
	for _, entry := range entries {
		if strings.TrimSpace(entry.KeyToken) == wanted {
			return entry, true, nil
		}
	}
	return clojureMapEntry{}, false, nil
}

func localFlowStepVector(source string, stepsEntry clojureMapEntry) ([]clojureFormSpan, error) {
	value := strings.TrimSpace(source[stepsEntry.ValueStart:stepsEntry.ValueEnd])
	if value == "nil" {
		return nil, nil
	}
	if !strings.HasPrefix(value, "[") {
		return nil, fmt.Errorf("top-level :steps must be a vector or nil")
	}
	spans, _, err := parseClojureVectorElements(source, stepsEntry.ValueStart)
	if err != nil {
		return nil, err
	}
	return spans, nil
}

func localStepIDFromMap(source string, span clojureFormSpan) (string, error) {
	entries, _, err := parseClojureMapEntries(source, span.Start)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.KeyToken) != ":id" {
			continue
		}
		token := strings.TrimSpace(source[entry.ValueStart:entry.ValueEnd])
		token = strings.TrimPrefix(token, ":")
		if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") {
			var decoded string
			if err := json.Unmarshal([]byte(token), &decoded); err != nil {
				return "", err
			}
			token = decoded
		}
		return token, nil
	}
	return "", fmt.Errorf("step definition is missing :id")
}

func localStepLiteralID(stepLiteral string) (string, error) {
	if strings.TrimSpace(stepLiteral) == "" {
		return "", errors.New("step literal is empty")
	}
	entries, err := parseSingleTopLevelMapEntries(stepLiteral)
	if err != nil {
		return "", fmt.Errorf("read step literal: %w", err)
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.KeyToken) != ":id" {
			continue
		}
		token := strings.TrimSpace(stepLiteral[entry.ValueStart:entry.ValueEnd])
		token = strings.TrimPrefix(token, ":")
		if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") {
			var decoded string
			if err := json.Unmarshal([]byte(token), &decoded); err != nil {
				return "", fmt.Errorf("decode step id: %w", err)
			}
			token = decoded
		}
		return token, nil
	}
	return "", errors.New("step literal must contain :id")
}

func parseSingleTopLevelMapEntries(source string) ([]clojureMapEntry, error) {
	if err := parenrepair.Check(source); err != nil {
		return nil, fmt.Errorf("source is not balanced: %w", err)
	}
	start, err := topLevelFlowMapStart(source)
	if err != nil {
		return nil, err
	}
	if start < 0 {
		return nil, errors.New("source must contain a top-level map")
	}
	entries, end, err := parseClojureMapEntries(source, start)
	if err != nil {
		return nil, err
	}
	if trailing := skipClojureWhitespaceCommaAndComments(source, end); trailing != len(source) {
		return nil, fmt.Errorf("source must contain exactly one top-level map; found another form near byte %d", trailing)
	}
	return entries, nil
}

func localStepSpansForID(source string, stepID string) (clojureMapEntry, []clojureFormSpan, int, error) {
	stepsEntry, found, err := localTopLevelEntry(source, "steps")
	if err != nil {
		return clojureMapEntry{}, nil, -1, err
	}
	if !found {
		return clojureMapEntry{}, nil, -1, nil
	}
	spans, err := localFlowStepVector(source, stepsEntry)
	if err != nil {
		return clojureMapEntry{}, nil, -1, err
	}
	for i, span := range spans {
		if localFlowVectorSpanIsInclude(source, span) {
			continue
		}
		id, idErr := localStepIDFromMap(source, span)
		if idErr != nil {
			return clojureMapEntry{}, nil, -1, idErr
		}
		if id == stepID || strings.TrimPrefix(id, ":") == strings.TrimPrefix(stepID, ":") {
			return stepsEntry, spans, i, nil
		}
	}
	return stepsEntry, spans, -1, nil
}

func localFlowVectorSpanIsInclude(source string, span clojureFormSpan) bool {
	start := skipClojureWhitespaceCommaAndComments(source, span.Start)
	return start < span.End && start < len(source) && isFlowIncludeFormStart(source, start)
}

func localStepExistsIncludingIncludes(sourcePath, source, stepID string) (bool, error) {
	_, _, index, err := localStepSpansForID(source, stepID)
	if err != nil {
		return false, err
	}
	if index >= 0 {
		return true, nil
	}
	expanded, err := expandFlowSourceIncludes(sourcePath, source)
	if err != nil {
		return false, fmt.Errorf("inspect included steps before creating %q: %w", stepID, err)
	}
	_, _, index, err = localStepSpansForID(expanded, stepID)
	if err != nil {
		return false, fmt.Errorf("inspect included steps before creating %q: %w", stepID, err)
	}
	return index >= 0, nil
}

func replaceLocalFlowValue(source string, entry clojureMapEntry, replacement string) string {
	return source[:entry.ValueStart] + replacement + source[entry.ValueEnd:]
}

func appendLocalStep(source, stepLiteral string) (string, error) {
	stepsEntry, found, err := localTopLevelEntry(source, "steps")
	if err != nil {
		return "", err
	}
	if !found {
		flowEntry, hasFlow, err := localTopLevelEntry(source, "flow")
		if err != nil {
			return "", err
		}
		mapStart, err := topLevelFlowMapStart(source)
		if err != nil || mapStart < 0 {
			return "", fmt.Errorf("locate top-level flow map: %w", err)
		}
		insertAt := len(source)
		if hasFlow {
			insertAt = flowEntry.KeyStart
		} else {
			mapEnd, endErr := readClojureFormEnd(source, mapStart)
			if endErr != nil {
				return "", endErr
			}
			insertAt = mapEnd - 1
		}
		prefix := "\n :steps [\n  " + stepLiteral + "\n ]\n "
		return source[:insertAt] + prefix + source[insertAt:], nil
	}

	value := strings.TrimSpace(source[stepsEntry.ValueStart:stepsEntry.ValueEnd])
	if value == "nil" {
		return replaceLocalFlowValue(source, stepsEntry, "[\n  "+stepLiteral+"\n ]"), nil
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return "", errors.New("top-level :steps must be a vector or nil")
	}
	insertAt := stepsEntry.ValueEnd - 1
	return source[:insertAt] + "\n  " + stepLiteral + "\n " + source[insertAt:], nil
}

func replaceLocalStep(source string, stepID, stepLiteral string) (string, error) {
	_, spans, index, err := localStepSpansForID(source, stepID)
	if err != nil {
		return "", err
	}
	if index < 0 {
		return "", fmt.Errorf("step %q not found", stepID)
	}
	span := spans[index]
	return source[:span.Start] + stepLiteral + source[span.End:], nil
}

func removeLocalStep(source string, stepID string) (string, error) {
	return removeLocalStepWithReferences(source, source, stepID, false)
}

func removeLocalStepFromPath(sourcePath, source string, stepID string) (string, error) {
	referenceSource, err := expandFlowSourceIncludes(sourcePath, source)
	if err != nil {
		return "", fmt.Errorf("inspect local flow references before removing step: %w", err)
	}
	return removeLocalStepWithReferences(source, referenceSource, stepID, true)
}

func removeLocalStepWithReferences(source, referenceSource, stepID string, strictScan bool) (string, error) {
	_, spans, index, err := localStepSpansForID(source, stepID)
	if err != nil {
		return "", err
	}
	if index < 0 {
		return "", fmt.Errorf("step %q not found", stepID)
	}
	references, scanErr := localFlowStepReferences(referenceSource)
	if scanErr != nil {
		if strictScan {
			return "", fmt.Errorf("cannot inspect flow body references before removing step: %w", scanErr)
		}
	} else {
		for _, reference := range references {
			if reference.StepID == strings.TrimPrefix(strings.TrimSpace(stepID), ":") {
				return "", fmt.Errorf("cannot remove step %q: flow body references it; update :flow first", stepID)
			}
		}
	}
	span := spans[index]
	start, end := localCompleteFormSpan(span)
	return source[:start] + source[end:], nil
}

func localFlowScheduleVector(source string, schedulesEntry clojureMapEntry) ([]clojureFormSpan, error) {
	value := strings.TrimSpace(source[schedulesEntry.ValueStart:schedulesEntry.ValueEnd])
	if value == "nil" {
		return nil, nil
	}
	if !strings.HasPrefix(value, "[") {
		return nil, fmt.Errorf("top-level :schedules must be a vector or nil")
	}
	spans, _, err := parseClojureVectorElements(source, schedulesEntry.ValueStart)
	if err != nil {
		return nil, err
	}
	return spans, nil
}

func localScheduleIDFromMap(source string, span clojureFormSpan) (string, error) {
	entries, _, err := parseClojureMapEntries(source, span.Start)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.KeyToken) != ":id" {
			continue
		}
		token := strings.TrimSpace(source[entry.ValueStart:entry.ValueEnd])
		if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") {
			var decoded string
			if err := json.Unmarshal([]byte(token), &decoded); err != nil {
				return "", err
			}
			token = decoded
		} else {
			token = strings.TrimPrefix(token, ":")
		}
		if !localScheduleIDValid(token) {
			return "", fmt.Errorf("schedule id %q must be an unqualified safe id", token)
		}
		return token, nil
	}
	return "", errors.New("schedule definition is missing :id")
}

func localScheduleLiteralID(scheduleLiteral string) (string, error) {
	if strings.TrimSpace(scheduleLiteral) == "" {
		return "", errors.New("schedule literal is empty")
	}
	if _, err := parseSingleTopLevelMapEntries(scheduleLiteral); err != nil {
		return "", fmt.Errorf("read schedule literal: %w", err)
	}
	return localScheduleIDFromMap(scheduleLiteral, clojureFormSpan{Start: 0, End: len(scheduleLiteral)})
}

func localScheduleSpansForID(source string, scheduleID string) (clojureMapEntry, []clojureFormSpan, int, error) {
	schedulesEntry, found, err := localTopLevelEntry(source, "schedules")
	if err != nil {
		return clojureMapEntry{}, nil, -1, err
	}
	if !found {
		return clojureMapEntry{}, nil, -1, nil
	}
	spans, err := localFlowScheduleVector(source, schedulesEntry)
	if err != nil {
		return clojureMapEntry{}, nil, -1, err
	}
	for i, span := range spans {
		if localFlowVectorSpanIsInclude(source, span) {
			continue
		}
		id, idErr := localScheduleIDFromMap(source, span)
		if idErr != nil {
			return clojureMapEntry{}, nil, -1, idErr
		}
		if id == strings.TrimPrefix(strings.TrimSpace(scheduleID), ":") {
			return schedulesEntry, spans, i, nil
		}
	}
	return schedulesEntry, spans, -1, nil
}

func localScheduleExistsIncludingIncludes(sourcePath, source, scheduleID string) (bool, error) {
	_, _, index, err := localScheduleSpansForID(source, scheduleID)
	if err != nil {
		return false, err
	}
	if index >= 0 {
		return true, nil
	}
	expanded, err := expandFlowSourceIncludes(sourcePath, source)
	if err != nil {
		return false, fmt.Errorf("inspect included schedules before creating %q: %w", scheduleID, err)
	}
	_, _, index, err = localScheduleSpansForID(expanded, scheduleID)
	if err != nil {
		return false, fmt.Errorf("inspect included schedules before creating %q: %w", scheduleID, err)
	}
	return index >= 0, nil
}

func appendLocalSchedule(source, scheduleLiteral string) (string, error) {
	schedulesEntry, found, err := localTopLevelEntry(source, "schedules")
	if err != nil {
		return "", err
	}
	if !found {
		flowEntry, hasFlow, err := localTopLevelEntry(source, "flow")
		if err != nil {
			return "", err
		}
		mapStart, err := topLevelFlowMapStart(source)
		if err != nil || mapStart < 0 {
			return "", fmt.Errorf("locate top-level flow map: %w", err)
		}
		insertAt := len(source)
		if hasFlow {
			insertAt = flowEntry.KeyStart
		} else {
			mapEnd, endErr := readClojureFormEnd(source, mapStart)
			if endErr != nil {
				return "", endErr
			}
			insertAt = mapEnd - 1
		}
		prefix := "\n :schedules [\n  " + scheduleLiteral + "\n ]\n "
		return source[:insertAt] + prefix + source[insertAt:], nil
	}

	value := strings.TrimSpace(source[schedulesEntry.ValueStart:schedulesEntry.ValueEnd])
	if value == "nil" {
		return replaceLocalFlowValue(source, schedulesEntry, "[\n  "+scheduleLiteral+"\n ]"), nil
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return "", errors.New("top-level :schedules must be a vector or nil")
	}
	insertAt := schedulesEntry.ValueEnd - 1
	return source[:insertAt] + "\n  " + scheduleLiteral + "\n " + source[insertAt:], nil
}

func replaceLocalSchedule(source string, scheduleID, scheduleLiteral string) (string, error) {
	_, spans, index, err := localScheduleSpansForID(source, scheduleID)
	if err != nil {
		return "", err
	}
	if index < 0 {
		return "", fmt.Errorf("schedule %q not found", scheduleID)
	}
	span := spans[index]
	return source[:span.Start] + scheduleLiteral + source[span.End:], nil
}

func removeLocalSchedule(source string, scheduleID string) (string, error) {
	_, spans, index, err := localScheduleSpansForID(source, scheduleID)
	if err != nil {
		return "", err
	}
	if index < 0 {
		return "", fmt.Errorf("schedule %q not found", scheduleID)
	}
	span := spans[index]
	start, end := localCompleteFormSpan(span)
	return source[:start] + source[end:], nil
}

func localCompleteFormSpan(span clojureFormSpan) (int, int) {
	if span.FormEnd > span.FormStart {
		return span.FormStart, span.FormEnd
	}
	return span.Start, span.End
}

func composeLocalFlowBody(source, body string) (string, error) {
	entry, found, err := localTopLevelEntry(source, "flow")
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("flow source is missing top-level :flow")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "", errors.New("flow body is empty")
	}
	if !localFlowBodyIsQuoted(body) {
		body = "'" + body
	}
	if err := validateSingleClojureForm(body); err != nil {
		return "", fmt.Errorf("flow body must be one complete Clojure form: %w", err)
	}
	updated := replaceLocalFlowValue(source, entry, body)
	if _, err := parseSingleTopLevelMapEntries(updated); err != nil {
		return "", fmt.Errorf("composed flow source is invalid: %w", err)
	}
	return updated, nil
}

func localFlowBodyIsQuoted(body string) bool {
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "'") || strings.HasPrefix(body, "`") {
		return true
	}
	if !strings.HasPrefix(body, "(") {
		return false
	}
	elements, _, err := parseClojureListElements(body, 0)
	if err != nil || len(elements) == 0 {
		return false
	}
	head := clojureFormToken(body, elements[0])
	return head == "quote" || head == "clojure.core/quote"
}

func validateSingleClojureForm(source string) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("form is empty")
	}
	if err := parenrepair.Check(source); err != nil {
		return err
	}
	end, err := readClojureFormEnd(source, 0)
	if err != nil {
		return err
	}
	if trailing := skipClojureWhitespaceCommaAndComments(source, end); trailing != len(source) {
		return fmt.Errorf("found another form near byte %d", trailing)
	}
	return nil
}

func readParamsJSON(paramsJSON, paramsFile string) (map[string]any, error) {
	raw := strings.TrimSpace(paramsJSON)
	if path := strings.TrimSpace(paramsFile); path != "" {
		b, err := readExplicitFile(path)
		if err != nil {
			return nil, fmt.Errorf("read --params-file: %w", err)
		}
		raw = strings.TrimSpace(string(b))
	}
	if raw == "" {
		return map[string]any{}, nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("invalid --params JSON: %w", err)
	}
	params, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("--params must be a JSON object")
	}
	return params, nil
}

func pushLocalFlowLiteral(cmd *cobra.Command, app *App, sourcePath, source string) (map[string]any, int, error) {
	if err := requireAPI(app); err != nil {
		return nil, 0, err
	}
	entries, err := parseSingleTopLevelMapEntries(source)
	if err != nil {
		return nil, 0, fmt.Errorf("read local flow before push: %w", err)
	}
	flowSlug, err := localFlowSlugFromEntries(source, entries)
	if err != nil {
		return nil, 0, fmt.Errorf("read local flow slug before push: %w", err)
	}
	expanded, err := expandFlowSourceIncludes(sourcePath, source)
	if err != nil {
		return nil, 0, err
	}
	out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.put_draft", map[string]any{
		"flowLiteral": expanded,
	})
	if err != nil || status >= 400 || !isOK(out) {
		return out, status, err
	}

	validateOut, validateStatus, err := validateDraftFlow(cmd, app, flowSlug)
	if err != nil {
		return nil, 0, err
	}
	if validateStatus >= 400 || !isOK(validateOut) {
		if postPushValidationFlowNotFound(validateOut, validateStatus, flowSlug) {
			meta := ensureMeta(out)
			if meta != nil {
				meta["validated"] = false
				meta["validateSource"] = "draft"
				meta["validationWarning"] = "Draft was saved, but immediate validation could not read the new flow yet. Retry validation after the draft is visible."
				appendMetaNextCommands(meta, "breyta flows validate "+flowSlug, "breyta flows show "+flowSlug)
			}
			return out, status, nil
		}
		return validateOut, validateStatus, nil
	}
	meta := ensureMeta(out)
	if meta != nil {
		meta["validated"] = true
		meta["validateSource"] = "draft"
	}
	return out, status, nil
}

func runLocalFlowStep(cmd *cobra.Command, app *App, slug, sourcePath, source, stepID string, params map[string]any, idempotencyKey, profileID string, timeout time.Duration) (map[string]any, int, error) {
	if err := requireAPI(app); err != nil {
		return nil, 0, err
	}
	if timeout <= 0 {
		return nil, 0, errors.New("--timeout must be > 0")
	}
	expanded, err := expandFlowSourceIncludes(sourcePath, source)
	if err != nil {
		return nil, 0, err
	}
	payload := map[string]any{
		"flowSlug":    strings.TrimSpace(slug),
		"stepId":      strings.TrimSpace(stepID),
		"params":      params,
		"flowLiteral": expanded,
	}
	if key := strings.TrimSpace(idempotencyKey); key != "" {
		payload["idempotencyKey"] = key
	}
	if profile := strings.TrimSpace(profileID); profile != "" {
		payload["profileId"] = profile
	}
	return runAPICommandWithContextAndTimeout(cmd.Context(), app, "steps.run", payload, timeout)
}

func requireSuccessfulLocalRun(cmd *cobra.Command, app *App, out map[string]any, status int) error {
	if status < 400 && isOK(out) {
		return nil
	}
	if out == nil {
		return writeErr(cmd, fmt.Errorf("local step run returned no API response (status=%d)", status))
	}
	return writeAPIResult(cmd, app, out, status)
}

func addLocalStepRunPreviewFlags(cmd *cobra.Command, opts *stepResultPreviewOptions) {
	cmd.Flags().BoolVar(&opts.Full, "full", false, "Include full data.result instead of the default compact resultPreview")
	cmd.Flags().StringVar(&opts.Path, "result-path", "", "Preview only one result branch (dot path or EDN-style vector path, e.g. rows.0 or [:rows 0])")
	cmd.Flags().StringVar(&opts.ResultFile, "result-file", "", "Write full data.result JSON to a local file while keeping terminal output compact")
	cmd.Flags().IntVar(&opts.MaxDepth, "preview-depth", stepResultPreviewDefaultDepth, "Max nested depth for resultPreview")
	cmd.Flags().IntVar(&opts.MaxItems, "preview-items", stepResultPreviewDefaultItems, "Max map entries or vector items per resultPreview level")
	cmd.Flags().IntVar(&opts.MaxRunes, "preview-runes", stepResultPreviewDefaultRunes, "Max runes for resultPreview.value")
}

func writeLocalAuthoringResult(cmd *cobra.Command, app *App, path string, pushed map[string]any, pushStatus int, extra map[string]any) error {
	result := map[string]any{
		"saved": true,
		"path":  path,
	}
	for key, value := range extra {
		result[key] = value
	}
	if pushed != nil {
		result["remote"] = pushed
		result["remoteStatus"] = pushStatus
	}
	return writeData(cmd, app, nil, result)
}

func writeSavedLocalAuthoringFailure(cmd *cobra.Command, app *App, out map[string]any, status int, err error, path, slug, stepID, operation, idempotencyKey string) error {
	path = strings.TrimSpace(path)
	slug = strings.TrimSpace(slug)
	stepID = strings.TrimSpace(stepID)
	retry := "breyta flows push --file " + shellQuotePath(path)
	if operation == "push" {
		// The persisted flow file is the source of truth; no placeholder step
		// file should be advertised after a create/init failure.
	} else {
		retry = "breyta flows steps run " + slug + " " + stepID + " --flow-file " + shellQuotePath(path)
		for _, flag := range []string{"profile-id", "params", "params-file", "idempotency-key", "timeout"} {
			if cmd != nil && cmd.Flags().Changed(flag) {
				value := cmd.Flags().Lookup(flag).Value.String()
				if strings.TrimSpace(value) != "" {
					retry += " --" + flag + " " + shellQuotePath(value)
				}
			}
		}
	}
	subject := "step"
	if stepID == "" {
		subject = "flow"
	}
	message := fmt.Sprintf("%s saved locally to %s; server rejected %s. Fix the definition and retry with `%s`", subject, path, operation, retry)
	if err != nil || status >= 500 {
		reconcile := fmt.Sprintf("%s saved locally to %s; the %s request ended before a definitive server response.", subject, path, operation)
		if operation == "run" {
			reconcile += fmt.Sprintf(" Reconcile the execution and any external effect before retrying to avoid duplicates; inspect recent runs with `breyta runs list --query %s --limit 10`", shellQuotePath("flow:"+slug))
		} else {
			reconcile += fmt.Sprintf(" Reconcile before retrying with `breyta flows show %s --target draft` or `breyta flows validate %s`", slug, slug)
		}
		if key := strings.TrimSpace(idempotencyKey); key != "" {
			reconcile += fmt.Sprintf("; preserve --idempotency-key %s on any retry", shellQuotePath(key))
		}
		if err != nil {
			return writeErr(cmd, fmt.Errorf("%s: %w", reconcile, err))
		}
		if out == nil {
			return writeErr(cmd, errors.New(reconcile))
		}
		meta := ensureMeta(out)
		meta["saved"] = true
		meta["path"] = path
		meta["hint"] = reconcile
		return writeAPIResult(cmd, app, out, status)
	}
	if out == nil {
		return writeErr(cmd, errors.New(message))
	}
	meta := ensureMeta(out)
	if meta != nil {
		meta["saved"] = true
		meta["path"] = path
		meta["hint"] = message
		appendMetaNextCommands(meta, retry)
	}
	return writeAPIResult(cmd, app, out, status)
}

func localStepScaffold(stepID, stepType, description string) string {
	defaults := ""
	switch stepType {
	case "http":
		defaults = "\n  :defaults {:method :get\n             :url \"https://example.com\"}"
	case "function":
		defaults = "\n  :defaults {:code '(fn [input] input)}"
	case "llm":
		defaults = "\n  :defaults {:connection nil\n             :model \"gpt-5.4\"\n             :prompt \"Replace with the step prompt and connection.\"}"
	}
	return fmt.Sprintf("{:id :%s\n  :type :%s\n  :description %q\n  :input-schema [:map]%s}", stepID, stepType, description, defaults)
}

func localInvocationInputLiteral(spec string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(spec), ":", 4)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid --input %q (use name:type[:required|optional[:label]])", spec)
	}
	name := strings.TrimPrefix(strings.TrimSpace(parts[0]), ":")
	inputType := strings.TrimPrefix(strings.TrimSpace(parts[1]), ":")
	if !validFlowLintSafeIdentifier(name) || strings.Contains(name, "/") {
		return "", fmt.Errorf("invalid invocation input name %q", name)
	}
	if !flowLintInvocationTypes[inputType] {
		return "", fmt.Errorf("invalid invocation input type %q", inputType)
	}
	required := false
	if len(parts) >= 3 {
		switch strings.ToLower(strings.TrimSpace(parts[2])) {
		case "", "optional", "false":
		case "required", "true":
			required = true
		default:
			return "", fmt.Errorf("invalid --input requirement %q (use required or optional)", parts[2])
		}
	}
	label := strings.ReplaceAll(name, "-", " ")
	if len(parts) == 4 && strings.TrimSpace(parts[3]) != "" {
		label = strings.TrimSpace(parts[3])
	}
	return fmt.Sprintf("{:name :%s :type :%s :label %q :required %t}", name, inputType, label, required), nil
}

func localInvocationInputsLiteral(specs []string) (string, error) {
	if len(specs) == 0 {
		return "[]", nil
	}
	items := make([]string, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		name := strings.TrimPrefix(strings.TrimSpace(strings.SplitN(spec, ":", 2)[0]), ":")
		if seen[name] {
			return "", fmt.Errorf("duplicate invocation input name %q", name)
		}
		seen[name] = true
		item, err := localInvocationInputLiteral(spec)
		if err != nil {
			return "", err
		}
		items = append(items, item)
	}
	return "[" + strings.Join(items, "\n                          ") + "]", nil
}

func newFlowsStepsLocalCreateCmd(app *App) *cobra.Command {
	var flowFile, stepFile, stepType, title, description string
	var push, run bool
	var idempotencyKey, paramsJSON, paramsFile, profileID string
	var timeout time.Duration
	var previewOpts stepResultPreviewOptions
	cmd := &cobra.Command{
		Use:   "create <flow-slug> <step-id>",
		Short: "Add a packaged step to the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, stepID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !isAPIValidFlowSlug(slug) {
				return writeErr(cmd, fmt.Errorf("invalid flow slug %q", slug))
			}
			if !localStepIDValid(stepID) {
				return writeErr(cmd, fmt.Errorf("invalid step id %q (use a qualified id such as tools/fetch-order)", stepID))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			exists, findErr := localStepExistsIncludingIncludes(path, source, stepID)
			if findErr != nil {
				return writeErr(cmd, findErr)
			} else if exists {
				return writeErr(cmd, fmt.Errorf("step %q already exists; use update", stepID))
			}
			var stepLiteral string
			if strings.TrimSpace(stepFile) != "" {
				b, readErr := readExplicitFile(stepFile)
				if readErr != nil {
					return writeErr(cmd, fmt.Errorf("read --step-file: %w", readErr))
				}
				stepLiteral = strings.TrimSpace(string(b))
				literalID, idErr := localStepLiteralID(stepLiteral)
				if idErr != nil {
					return writeErr(cmd, idErr)
				}
				if literalID != stepID {
					return writeErr(cmd, fmt.Errorf("step literal id %q does not match %q", literalID, stepID))
				}
			} else {
				if strings.TrimSpace(stepType) == "" {
					return writeErr(cmd, errors.New("missing --type or --step-file"))
				}
				stepType = strings.TrimPrefix(strings.TrimSpace(stepType), ":")
				if !localStepTypeValid(stepType) {
					return writeErr(cmd, fmt.Errorf("invalid step type %q (use a safe type such as function, http, or llm)", stepType))
				}
				if strings.TrimSpace(title) == "" {
					title = stepID
				}
				stepLiteral = localStepScaffold(stepID, stepType, descriptionOrDefault(description, title))
			}
			updated, err := appendLocalStep(source, stepLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			var remote map[string]any
			var remoteStatus int
			if push {
				remote, remoteStatus, err = pushLocalFlowLiteral(cmd, app, path, updated)
				if err != nil {
					return writeSavedLocalAuthoringFailure(cmd, app, remote, remoteStatus, err, path, slug, stepID, "push", "")
				}
				if remoteStatus >= 400 || !isOK(remote) {
					return writeSavedLocalAuthoringFailure(cmd, app, remote, remoteStatus, nil, path, slug, stepID, "push", "")
				}
			}
			extra := map[string]any{"flowSlug": slug, "stepId": stepID}
			if run {
				params, paramsErr := readParamsJSON(paramsJSON, paramsFile)
				if paramsErr != nil {
					return writeErr(cmd, paramsErr)
				}
				runOut, runStatus, runErr := runLocalFlowStep(cmd, app, slug, path, updated, stepID, params, idempotencyKey, profileID, timeout)
				if runErr != nil {
					return writeSavedLocalAuthoringFailure(cmd, app, runOut, runStatus, runErr, path, slug, stepID, "run", idempotencyKey)
				}
				if runStatus >= 400 || !isOK(runOut) {
					return writeSavedLocalAuthoringFailure(cmd, app, runOut, runStatus, nil, path, slug, stepID, "run", idempotencyKey)
				}
				if err := compactStepsRunResult(runOut, stepID, previewOpts); err != nil {
					return writeErr(cmd, err)
				}
				extra["run"] = runOut
				extra["runStatus"] = runStatus
			}
			return writeLocalAuthoringResult(cmd, app, path, remote, remoteStatus, extra)
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&stepFile, "step-file", "", "Read the complete packaged step map from this file")
	cmd.Flags().StringVar(&stepType, "type", "", "Underlying step type when scaffolding (for example: function, http, llm)")
	cmd.Flags().StringVar(&title, "title", "", "Scaffold title/description")
	cmd.Flags().StringVar(&description, "description", "", "Scaffold description")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	cmd.Flags().BoolVar(&run, "run", false, "Run the edited packaged step against the server after saving")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step input JSON object for --run")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "Read step input JSON from this file for --run")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for --run retries")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional installation/profile id for --run")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultFlowRunWaitTimeout, "Request timeout for --run; increase for slow LLM, image, or HTTP steps")
	addLocalStepRunPreviewFlags(cmd, &previewOpts)
	return cmd
}

func descriptionOrDefault(description, title string) string {
	if value := strings.TrimSpace(description); value != "" {
		return value
	}
	return strings.TrimSpace(title)
}

func newFlowsStepsLocalUpdateCmd(app *App) *cobra.Command {
	var flowFile, stepFile string
	var push, run bool
	var idempotencyKey, paramsJSON, paramsFile, profileID string
	var timeout time.Duration
	var previewOpts stepResultPreviewOptions
	cmd := &cobra.Command{
		Use:   "update <flow-slug> <step-id>",
		Short: "Replace a packaged step in the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, stepID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !localStepIDValid(stepID) {
				return writeErr(cmd, fmt.Errorf("invalid packaged step id %q (flows steps run only probes qualified top-level :steps ids such as tools/fetch; for a named flow-local :function, :code, or bound :llm step use `breyta flows run-step %s %s --target draft --input '{...}' --wait`)", stepID, slug, stepID))
			}
			if strings.TrimSpace(stepFile) == "" {
				return writeErr(cmd, errors.New("update requires --step-file; edit the flow body with compose"))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			b, err := readExplicitFile(stepFile)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("read --step-file: %w", err))
			}
			stepLiteral := strings.TrimSpace(string(b))
			literalID, err := localStepLiteralID(stepLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if literalID != stepID {
				return writeErr(cmd, fmt.Errorf("step literal id %q does not match %q", literalID, stepID))
			}
			updated, err := replaceLocalStep(source, stepID, stepLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			var remote map[string]any
			var remoteStatus int
			if push {
				remote, remoteStatus, err = pushLocalFlowLiteral(cmd, app, path, updated)
				if err != nil {
					return writeSavedLocalAuthoringFailure(cmd, app, remote, remoteStatus, err, path, slug, stepID, "push", "")
				}
				if remoteStatus >= 400 || !isOK(remote) {
					return writeSavedLocalAuthoringFailure(cmd, app, remote, remoteStatus, nil, path, slug, stepID, "push", "")
				}
			}
			extra := map[string]any{"flowSlug": slug, "stepId": stepID}
			if run {
				params, paramsErr := readParamsJSON(paramsJSON, paramsFile)
				if paramsErr != nil {
					return writeErr(cmd, paramsErr)
				}
				runOut, runStatus, runErr := runLocalFlowStep(cmd, app, slug, path, updated, stepID, params, idempotencyKey, profileID, timeout)
				if runErr != nil {
					return writeSavedLocalAuthoringFailure(cmd, app, runOut, runStatus, runErr, path, slug, stepID, "run", idempotencyKey)
				}
				if runStatus >= 400 || !isOK(runOut) {
					return writeSavedLocalAuthoringFailure(cmd, app, runOut, runStatus, nil, path, slug, stepID, "run", idempotencyKey)
				}
				if err := compactStepsRunResult(runOut, stepID, previewOpts); err != nil {
					return writeErr(cmd, err)
				}
				extra["run"] = runOut
				extra["runStatus"] = runStatus
			}
			return writeLocalAuthoringResult(cmd, app, path, remote, remoteStatus, extra)
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&stepFile, "step-file", "", "Read the replacement packaged step map from this file")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	cmd.Flags().BoolVar(&run, "run", false, "Run the edited packaged step against the server after saving")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step input JSON object for --run")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "Read step input JSON from this file for --run")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for --run retries")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional installation/profile id for --run")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultFlowRunWaitTimeout, "Request timeout for --run; increase for slow LLM, image, or HTTP steps")
	addLocalStepRunPreviewFlags(cmd, &previewOpts)
	return cmd
}

func newFlowsStepsLocalRemoveCmd(app *App) *cobra.Command {
	var flowFile string
	var push bool
	cmd := &cobra.Command{
		Use:   "remove <flow-slug> <step-id>",
		Short: "Remove a packaged step from the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, stepID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			updated, err := removeLocalStepFromPath(path, source, stepID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			if push {
				remote, status, pushErr := pushLocalFlowLiteral(cmd, app, path, updated)
				if pushErr != nil {
					return writeErr(cmd, pushErr)
				}
				if status >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, status)
				}
				return writeLocalAuthoringResult(cmd, app, path, remote, status, map[string]any{"flowSlug": slug, "stepId": stepID})
			}
			return writeLocalAuthoringResult(cmd, app, path, nil, 0, map[string]any{"flowSlug": slug, "stepId": stepID})
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	return cmd
}

func newFlowsStepsLocalRunCmd(app *App) *cobra.Command {
	var flowFile, paramsJSON, paramsFile, idempotencyKey, profileID string
	var timeout time.Duration
	var previewOpts stepResultPreviewOptions
	cmd := &cobra.Command{
		Use:   "run <flow-slug> <step-id>",
		Short: "Run a packaged step from the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, stepID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !localStepIDValid(stepID) {
				return writeErr(cmd, fmt.Errorf("invalid packaged step id %q (flows steps run only probes qualified top-level :steps ids such as tools/fetch; for a named flow-local :function, :code, or bound :llm step use `breyta flows run-step %s %s --target draft --input '{...}' --wait`)", stepID, slug, stepID))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			params, err := readParamsJSON(paramsJSON, paramsFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			out, status, err := runLocalFlowStep(cmd, app, slug, path, source, stepID, params, idempotencyKey, profileID, timeout)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := compactStepsRunResult(out, stepID, previewOpts); err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step input JSON object")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "Read step input JSON from this file")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for retrying side-effectful runs")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional installation/profile id")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultFlowRunWaitTimeout, "Request timeout; increase for slow LLM, image, or HTTP steps")
	addLocalStepRunPreviewFlags(cmd, &previewOpts)
	return cmd
}

func newFlowsSchedulesLocalAddCmd(app *App) *cobra.Command {
	var flowFile, scheduleFile, label, invocation, cron, timezone string
	var enabled, push bool
	cmd := &cobra.Command{
		Use:   "add <flow-slug> <schedule-id>",
		Short: "Add a schedule to the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, scheduleID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !isAPIValidFlowSlug(slug) {
				return writeErr(cmd, fmt.Errorf("invalid flow slug %q", slug))
			}
			if !localScheduleIDValid(scheduleID) {
				return writeErr(cmd, fmt.Errorf("invalid schedule id %q (use an unqualified id such as daily-review)", scheduleID))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			exists, findErr := localScheduleExistsIncludingIncludes(path, source, scheduleID)
			if findErr != nil {
				return writeErr(cmd, findErr)
			} else if exists {
				return writeErr(cmd, fmt.Errorf("schedule %q already exists; use update", scheduleID))
			}

			var scheduleLiteral string
			if strings.TrimSpace(scheduleFile) != "" {
				b, readErr := readExplicitFile(scheduleFile)
				if readErr != nil {
					return writeErr(cmd, fmt.Errorf("read --schedule-file: %w", readErr))
				}
				scheduleLiteral = strings.TrimSpace(string(b))
				literalID, idErr := localScheduleLiteralID(scheduleLiteral)
				if idErr != nil {
					return writeErr(cmd, idErr)
				}
				if literalID != scheduleID {
					return writeErr(cmd, fmt.Errorf("schedule literal id %q does not match %q", literalID, scheduleID))
				}
			} else {
				if strings.TrimSpace(cron) == "" {
					return writeErr(cmd, errors.New("missing --cron or --schedule-file"))
				}
				if strings.TrimSpace(label) == "" {
					label = scheduleID
				}
				invocation = strings.TrimPrefix(strings.TrimSpace(invocation), ":")
				if invocation == "" {
					invocation = "default"
				}
				if !localScheduleIDValid(invocation) {
					return writeErr(cmd, fmt.Errorf("invalid invocation %q (use an unqualified id such as default)", invocation))
				}
				scheduleLiteral = fmt.Sprintf("{:id :%s\n  :label %q\n  :invocation :%s\n  :enabled %t\n  :cron %q", scheduleID, label, invocation, enabled, strings.TrimSpace(cron))
				if strings.TrimSpace(timezone) != "" {
					scheduleLiteral += fmt.Sprintf("\n  :timezone %q", strings.TrimSpace(timezone))
				}
				scheduleLiteral += "}"
			}
			updated, err := appendLocalSchedule(source, scheduleLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			if push {
				remote, status, pushErr := pushLocalFlowLiteral(cmd, app, path, updated)
				if pushErr != nil {
					return writeErr(cmd, pushErr)
				}
				if status >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, status)
				}
				return writeLocalAuthoringResult(cmd, app, path, remote, status, map[string]any{"flowSlug": slug, "scheduleId": scheduleID})
			}
			return writeLocalAuthoringResult(cmd, app, path, nil, 0, map[string]any{"flowSlug": slug, "scheduleId": scheduleID})
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&scheduleFile, "schedule-file", "", "Read the complete schedule map from this file")
	cmd.Flags().StringVar(&label, "label", "", "Schedule display label (default: schedule id)")
	cmd.Flags().StringVar(&invocation, "invocation", "default", "Invocation id to call (default: default)")
	cmd.Flags().StringVar(&cron, "cron", "", "Cron expression for the schedule")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone such as UTC or Europe/Oslo")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable the authored schedule by default")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	return cmd
}

func newFlowsSchedulesLocalUpdateCmd(app *App) *cobra.Command {
	var flowFile, scheduleFile string
	var push bool
	cmd := &cobra.Command{
		Use:   "update <flow-slug> <schedule-id>",
		Short: "Replace a schedule in the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, scheduleID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !localScheduleIDValid(scheduleID) {
				return writeErr(cmd, fmt.Errorf("invalid schedule id %q", scheduleID))
			}
			if strings.TrimSpace(scheduleFile) == "" {
				return writeErr(cmd, errors.New("update requires --schedule-file; use add flags for a new schedule"))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			b, err := readExplicitFile(scheduleFile)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("read --schedule-file: %w", err))
			}
			scheduleLiteral := strings.TrimSpace(string(b))
			literalID, err := localScheduleLiteralID(scheduleLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if literalID != scheduleID {
				return writeErr(cmd, fmt.Errorf("schedule literal id %q does not match %q", literalID, scheduleID))
			}
			updated, err := replaceLocalSchedule(source, scheduleID, scheduleLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			if push {
				remote, status, pushErr := pushLocalFlowLiteral(cmd, app, path, updated)
				if pushErr != nil {
					return writeErr(cmd, pushErr)
				}
				if status >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, status)
				}
				return writeLocalAuthoringResult(cmd, app, path, remote, status, map[string]any{"flowSlug": slug, "scheduleId": scheduleID})
			}
			return writeLocalAuthoringResult(cmd, app, path, nil, 0, map[string]any{"flowSlug": slug, "scheduleId": scheduleID})
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&scheduleFile, "schedule-file", "", "Read the complete replacement schedule map from this file")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	return cmd
}

func newFlowsSchedulesLocalRemoveCmd(app *App) *cobra.Command {
	var flowFile string
	var push bool
	cmd := &cobra.Command{
		Use:   "remove <flow-slug> <schedule-id>",
		Short: "Remove a schedule from the local flow source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, scheduleID := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if !localScheduleIDValid(scheduleID) {
				return writeErr(cmd, fmt.Errorf("invalid schedule id %q", scheduleID))
			}
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			updated, err := removeLocalSchedule(source, scheduleID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			if push {
				remote, status, pushErr := pushLocalFlowLiteral(cmd, app, path, updated)
				if pushErr != nil {
					return writeErr(cmd, pushErr)
				}
				if status >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, status)
				}
				return writeLocalAuthoringResult(cmd, app, path, remote, status, map[string]any{"flowSlug": slug, "scheduleId": scheduleID})
			}
			return writeLocalAuthoringResult(cmd, app, path, nil, 0, map[string]any{"flowSlug": slug, "scheduleId": scheduleID})
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	return cmd
}

func newFlowsSchedulesLocalCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedules",
		Short: "Edit top-level schedules in local flow source",
		Long: strings.TrimSpace(`
Edit the top-level :schedules vector in the local flow source. These commands
only write the local file unless --push is passed; flows configure remains the
remote author/install schedule-settings surface.

Examples:
  breyta flows schedules add order-sync daily-review --cron "0 9 * * MON" --timezone UTC
  breyta flows schedules update order-sync daily-review --schedule-file ./schedules/daily-review.edn
  breyta flows schedules remove order-sync daily-review
`),
	}
	cmd.AddCommand(newFlowsSchedulesLocalAddCmd(app))
	cmd.AddCommand(newFlowsSchedulesLocalUpdateCmd(app))
	cmd.AddCommand(newFlowsSchedulesLocalRemoveCmd(app))
	return cmd
}

func newFlowsComposeCmd(app *App) *cobra.Command {
	var flowFile, body, bodyFile string
	var push bool
	cmd := &cobra.Command{
		Use:   "compose <flow-slug>",
		Short: "Replace only the :flow literal in local source",
		Long: strings.TrimSpace(`
Compose the flow body while preserving the surrounding definition, including
the existing top-level :steps, interfaces, schedules, functions, and providers.
The body may call any packaged steps already declared in :steps.

Examples:
  breyta flows compose order-sync --flow-file ./flows/order-sync.clj --body-file ./flows/order-sync.body.clj
  breyta flows compose order-sync --body '(let [order (flow/step :tools/fetch-order :fetch {})] order)'
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := strings.TrimSpace(args[0])
			path, source, err := readLocalFlowSource(slug, flowFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			bodyText := strings.TrimSpace(body)
			if strings.TrimSpace(bodyFile) != "" {
				b, readErr := readExplicitFile(bodyFile)
				if readErr != nil {
					return writeErr(cmd, fmt.Errorf("read --body-file: %w", readErr))
				}
				bodyText = strings.TrimSpace(string(b))
			}
			if bodyText == "" {
				return writeErr(cmd, errors.New("missing --body or --body-file"))
			}
			updated, err := composeLocalFlowBody(source, bodyText)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := atomicWriteFile(path, []byte(updated), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			if push {
				remote, status, pushErr := pushLocalFlowLiteral(cmd, app, path, updated)
				if pushErr != nil {
					return writeErr(cmd, pushErr)
				}
				if status >= 400 || !isOK(remote) {
					return writeAPIResult(cmd, app, remote, status)
				}
				return writeLocalAuthoringResult(cmd, app, path, remote, status, map[string]any{"flowSlug": slug})
			}
			return writeLocalAuthoringResult(cmd, app, path, nil, 0, map[string]any{"flowSlug": slug})
		},
	}
	cmd.Flags().StringVar(&flowFile, "flow-file", "", "Local flow source path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringVar(&body, "body", "", "Quoted or unquoted Clojure flow body")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Read the Clojure flow body from this file")
	cmd.Flags().BoolVar(&push, "push", false, "Push the edited local source after saving")
	return cmd
}

func newFlowsInitCmd(app *App) *cobra.Command {
	var name, description, outPath string
	var stepID, stepFile, paramsJSON, paramsFile, idempotencyKey, profileID string
	var invocationInputs []string
	var force, push, run bool
	var timeout time.Duration
	var previewOpts stepResultPreviewOptions
	cmd := &cobra.Command{
		Use:   "init <flow-slug>",
		Short: "Create a local canonical flow source file",
		Long: strings.TrimSpace(`
Create the local source of truth for a flow. Initialization is local by default;
pass --push when you explicitly want the same source sent to the workspace.
The generated definition includes a manual Run interface and an empty schedules
vector so later schedule edits stay in the same source file. Repeat --input to
seed invocation fields as name:type[:required|optional[:label]].
When the first packaged step is already authored, pass --step-id and --step-file
to seed it into the local literal in the same command. Add --run to prove that
seeded step just in time on the server; this does not create a remote draft.

Examples:
  breyta flows init order-sync --name "Order sync"
  breyta flows init order-sync --input 'order-id:text:required:Order ID'
  breyta flows init order-sync --step-id tools/fetch-order --step-file ./steps/fetch-order.edn --run
  breyta flows init order-sync --out ./flows/order-sync.clj --push
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := strings.TrimSpace(args[0])
			if !isAPIValidFlowSlug(slug) {
				return writeErr(cmd, fmt.Errorf("invalid flow slug %q", slug))
			}
			stepID = strings.TrimSpace(stepID)
			stepFile = strings.TrimSpace(stepFile)
			if (stepID == "") != (stepFile == "") {
				return writeErr(cmd, errors.New("--step-id and --step-file must be provided together"))
			}
			if run && stepFile == "" {
				return writeErr(cmd, errors.New("--run requires --step-id with --step-file"))
			}
			inputsLiteral, inputsErr := localInvocationInputsLiteral(invocationInputs)
			if inputsErr != nil {
				return writeErr(cmd, inputsErr)
			}
			var stepLiteral string
			if stepFile != "" {
				if !localStepIDValid(stepID) {
					return writeErr(cmd, fmt.Errorf("invalid step id %q (use a qualified id such as tools/fetch-order)", stepID))
				}
				b, readErr := readExplicitFile(stepFile)
				if readErr != nil {
					return writeErr(cmd, fmt.Errorf("read --step-file: %w", readErr))
				}
				stepLiteral = strings.TrimSpace(string(b))
				literalID, idErr := localStepLiteralID(stepLiteral)
				if idErr != nil {
					return writeErr(cmd, idErr)
				}
				if literalID != stepID {
					return writeErr(cmd, fmt.Errorf("step literal id %q does not match %q", literalID, stepID))
				}
			}
			path := resolveLocalFlowPath(slug, outPath)
			if _, err := os.Stat(path); err == nil && !force {
				return writeErr(cmd, fmt.Errorf("local flow %s already exists; pass --force to replace it", path))
			} else if err != nil && !os.IsNotExist(err) {
				return writeErr(cmd, err)
			}
			flowName := strings.TrimSpace(name)
			if flowName == "" {
				flowName = slug
			}
			literal := fmt.Sprintf(`{:slug :%s
 :name %q
 :description %q
 :tags ["draft"]
 :concurrency {:type :singleton :on-new-version :supersede}
 :requires nil
 :templates nil
 :functions nil
 :steps []
 :invocations {:default {:label "Run" :inputs %s}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(let [input (flow/input)] input)}
`, slug, flowName, strings.TrimSpace(description), inputsLiteral)
			if stepLiteral != "" {
				seeded, err := appendLocalStep(literal, stepLiteral)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("seed local step: %w", err))
				}
				literal = seeded
			}
			if err := atomicWriteFile(path, []byte(literal), publicFileMode); err != nil {
				return writeErr(cmd, fmt.Errorf("write local flow: %w", err))
			}
			var remote map[string]any
			var remoteStatus int
			if push {
				var pushErr error
				remote, remoteStatus, pushErr = pushLocalFlowLiteral(cmd, app, path, literal)
				if pushErr != nil {
					return writeSavedLocalAuthoringFailure(cmd, app, remote, remoteStatus, pushErr, path, slug, stepID, "push", "")
				}
				if remoteStatus >= 400 || !isOK(remote) {
					return writeSavedLocalAuthoringFailure(cmd, app, remote, remoteStatus, nil, path, slug, stepID, "push", "")
				}
			}
			extra := map[string]any{"flowSlug": slug}
			if stepLiteral != "" {
				extra["stepId"] = stepID
			}
			if run {
				params, paramsErr := readParamsJSON(paramsJSON, paramsFile)
				if paramsErr != nil {
					return writeErr(cmd, paramsErr)
				}
				runOut, runStatus, runErr := runLocalFlowStep(cmd, app, slug, path, literal, stepID, params, idempotencyKey, profileID, timeout)
				if runErr != nil {
					return writeSavedLocalAuthoringFailure(cmd, app, runOut, runStatus, runErr, path, slug, stepID, "run", idempotencyKey)
				}
				if runStatus >= 400 || !isOK(runOut) {
					return writeSavedLocalAuthoringFailure(cmd, app, runOut, runStatus, nil, path, slug, stepID, "run", idempotencyKey)
				}
				if err := compactStepsRunResult(runOut, stepID, previewOpts); err != nil {
					return writeErr(cmd, err)
				}
				extra["run"] = runOut
				extra["runStatus"] = runStatus
			}
			return writeLocalAuthoringResult(cmd, app, path, remote, remoteStatus, extra)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Flow display name (default: slug)")
	cmd.Flags().StringVar(&description, "description", "", "Flow description")
	cmd.Flags().StringVar(&outPath, "out", "", "Output path (default: flows/<flow-slug>.clj)")
	cmd.Flags().StringArrayVar(&invocationInputs, "input", nil, "Seed invocation input as name:type[:required|optional[:label]] (repeatable)")
	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing local source file")
	cmd.Flags().BoolVar(&push, "push", false, "Push the generated source after saving")
	cmd.Flags().StringVar(&stepID, "step-id", "", "Qualified first packaged step id to seed into the local source")
	cmd.Flags().StringVar(&stepFile, "step-file", "", "Read the complete first packaged step map from this file")
	cmd.Flags().BoolVar(&run, "run", false, "Run the seeded packaged step against the server after saving")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step input JSON object for --run")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "Read step input JSON from this file for --run")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for --run retries")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional installation/profile id for --run")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultFlowRunWaitTimeout, "Request timeout for --run; increase for slow LLM, image, or HTTP steps")
	addLocalStepRunPreviewFlags(cmd, &previewOpts)
	return cmd
}
