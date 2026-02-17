package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type installationTrigger struct {
	TriggerID     string
	Type          string
	EventName     string
	EventPath     string
	Endpoint      string
	WebhookRaw    map[string]any
	WebhookFields []webhookField
	Raw           map[string]any
}

type webhookField struct {
	Name     string
	Type     string
	Required bool
	Multiple bool
	Raw      map[string]any
}

func parseWebhookFields(webhookRaw map[string]any) []webhookField {
	if webhookRaw == nil {
		return nil
	}
	itemsAny, _ := webhookRaw["fields"].([]any)
	if len(itemsAny) == 0 {
		return nil
	}
	var out []webhookField
	for _, itemAny := range itemsAny {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		typ, _ := item["type"].(string)
		required, _ := item["required"].(bool)
		multiple, _ := item["multiple"].(bool)
		name = strings.TrimSpace(name)
		typ = strings.TrimSpace(typ)
		if name == "" {
			continue
		}
		out = append(out, webhookField{
			Name:     name,
			Type:     typ,
			Required: required,
			Multiple: multiple,
			Raw:      item,
		})
	}
	return out
}

func inferDefaultFileField(trigger installationTrigger) (string, bool) {
	var candidates []string
	for _, f := range trigger.WebhookFields {
		switch strings.ToLower(strings.TrimSpace(f.Type)) {
		case "file", "blob", "blob-ref":
			if strings.TrimSpace(f.Name) != "" {
				candidates = append(candidates, strings.TrimSpace(f.Name))
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return "", false
}

func fetchInstallationTriggers(ctx context.Context, app *App, profileID string) ([]installationTrigger, error) {
	client := apiClient(app)
	out, status, err := client.DoCommand(ctx, "flows.installations.triggers.list", map[string]any{
		"profileId": profileID,
	})
	if err != nil {
		return nil, err
	}
	if status >= 400 || !isOK(out) {
		return nil, fmt.Errorf("api error (status=%d): %s", status, formatAPIError(out))
	}

	dataAny, _ := out["data"].(map[string]any)
	itemsAny, _ := dataAny["items"].([]any)

	var triggers []installationTrigger
	for _, itemAny := range itemsAny {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		triggerID, _ := item["triggerId"].(string)
		triggerType, _ := item["type"].(string)
		eventName, _ := item["eventName"].(string)

		var eventPath string
		var endpoint string
		var webhookRaw map[string]any
		var webhookFields []webhookField
		if wAny, ok := item["webhook"].(map[string]any); ok {
			webhookRaw = wAny
			eventPath, _ = wAny["eventPath"].(string)
			endpoint, _ = wAny["endpoint"].(string)
			webhookFields = parseWebhookFields(webhookRaw)
		}

		triggers = append(triggers, installationTrigger{
			TriggerID:     strings.TrimSpace(triggerID),
			Type:          strings.TrimSpace(triggerType),
			EventName:     strings.TrimSpace(eventName),
			EventPath:     strings.TrimSpace(eventPath),
			Endpoint:      strings.TrimSpace(endpoint),
			WebhookRaw:    webhookRaw,
			WebhookFields: webhookFields,
			Raw:           item,
		})
	}
	return triggers, nil
}

func pickInstallationUploadTrigger(triggers []installationTrigger, selector string) (installationTrigger, error) {
	var uploadTriggers []installationTrigger
	for _, t := range triggers {
		if strings.TrimSpace(t.EventPath) == "" {
			continue
		}
		uploadTriggers = append(uploadTriggers, t)
	}

	selector = strings.TrimSpace(selector)
	if selector == "" {
		if len(uploadTriggers) == 1 {
			return uploadTriggers[0], nil
		}
		if len(uploadTriggers) == 0 {
			return installationTrigger{}, errors.New("no upload triggers found for this installation")
		}
		var options []string
		for _, t := range uploadTriggers {
			if t.EventName != "" {
				options = append(options, t.EventName)
				continue
			}
			options = append(options, t.TriggerID)
		}
		sort.Strings(options)
		return installationTrigger{}, fmt.Errorf("multiple upload triggers found; use --trigger (%s)", strings.Join(options, ", "))
	}

	var matches []installationTrigger
	for _, t := range uploadTriggers {
		if selector == t.TriggerID || selector == t.EventName {
			matches = append(matches, t)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return installationTrigger{}, fmt.Errorf("no upload trigger matches %q", selector)
	}
	return installationTrigger{}, fmt.Errorf("multiple triggers match %q; use a triggerId", selector)
}

func newFlowsInstallationsUploadCmd(app *App) *cobra.Command {
	var triggerSelector string
	var fields []string
	var files []string
	var multipartFiles []string
	var fileField string

	cmd := &cobra.Command{
		Use:   "upload <installation-id> --file <path> [--file <path> ...]",
		Short: "Upload one or more files to an installation-scoped webhook trigger",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows installations upload requires API mode"))
			}
			if strings.TrimSpace(app.WorkspaceID) == "" {
				return writeErr(cmd, errors.New("missing workspace id (provide --workspace or set BREYTA_WORKSPACE)"))
			}
			profileID := strings.TrimSpace(args[0])
			if profileID == "" {
				return writeErr(cmd, errors.New("missing installation id"))
			}

			fileField = strings.TrimSpace(fileField)
			fields = trimStringSlice(fields)
			files = trimStringSlice(files)
			multipartFiles = trimStringSlice(multipartFiles)

			if len(files) == 0 && len(multipartFiles) == 0 {
				return writeErr(cmd, errors.New("missing --file (repeatable) or --multipart-file"))
			}

			triggers, err := fetchInstallationTriggers(context.Background(), app, profileID)
			if err != nil {
				return writeErr(cmd, err)
			}
			chosen, err := pickInstallationUploadTrigger(triggers, triggerSelector)
			if err != nil {
				return writeErr(cmd, err)
			}

			if fileField == "" {
				if inferred, ok := inferDefaultFileField(chosen); ok {
					fileField = inferred
				} else {
					fileField = "file"
				}
			}

			for _, p := range files {
				multipartFiles = append(multipartFiles, fmt.Sprintf("%s=%s", fileField, p))
			}
			multipartFiles = trimStringSlice(multipartFiles)

			if len(multipartFiles) == 0 {
				return writeErr(cmd, errors.New("missing --file (repeatable) or --multipart-file"))
			}

			baseURL := strings.TrimSpace(os.Getenv("BREYTA_API_URL"))
			if baseURL == "" {
				ensureAPIURL(app)
				baseURL = app.APIURL
			}
			baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
			if baseURL == "" {
				return writeErr(cmd, errors.New("missing base url (set BREYTA_API_URL)"))
			}

			payload, err := buildWebhookPayload("", "", fields, multipartFiles, "", "")
			if err != nil {
				return writeFailure(cmd, app, "payload_invalid", err, "Check upload flags; at least one file is required.", nil)
			}

			eventPath := escapePathSegments(chosen.EventPath)
			if eventPath == "" {
				return writeErr(cmd, errors.New("empty eventPath from server"))
			}

			endpoint := fmt.Sprintf("/%s/events/%s", strings.TrimSpace(app.WorkspaceID), eventPath)
			headers := map[string]string{}
			if payload.ContentType != "" {
				headers["Content-Type"] = payload.ContentType
			}

			client := apiClient(app)
			client.BaseURL = baseURL
			client.Token = ""
			out, status, err := client.DoRootRESTBytes(context.Background(), http.MethodPost, endpoint, url.Values{}, payload.Body, headers)
			if err != nil {
				return writeFailure(cmd, app, "upload_failed", err, "Check connectivity and trigger configuration.", map[string]any{
					"profileId": profileID,
					"eventPath": chosen.EventPath,
					"endpoint":  endpoint,
				})
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&triggerSelector, "trigger", "", "Trigger to use (eventName or triggerId). Required if multiple.")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "Multipart form field key=value (repeatable)")
	cmd.Flags().StringArrayVar(&files, "file", nil, "File path to upload (repeatable; uses --file-field)")
	cmd.Flags().StringVar(&fileField, "file-field", "", "Field name to use for --file (defaults to inferred webhook file field, else 'file')")
	cmd.Flags().StringArrayVar(&multipartFiles, "multipart-file", nil, "Multipart file field=path (repeatable)")

	return cmd
}
