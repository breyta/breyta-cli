package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type jobsWorkerHelperContext struct {
	JobID       string
	LeaseToken  string
	ResultFile  string
	ContextFile string
}

type jobsWorkerStatePaths struct {
	ContextFile string
	JobDir      string
	JobFile     string
	PayloadFile string
	ResultFile  string
}

func newJobsWorkerStateCmd(app *App) *cobra.Command {
	var jobDir string

	cmd := &cobra.Command{
		Use:   "state",
		Short: "Show local worker job state from env or a materialized job directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, meta, err := jobsWorkerBuildStateSnapshot(strings.TrimSpace(jobDir))
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, meta, data)
		},
	}

	cmd.Flags().StringVar(&jobDir, "job-dir", "", "Path to a materialized job directory; defaults to the active worker env")
	return cmd
}

func newJobsWorkerProgressCmd(app *App) *cobra.Command {
	var status string
	var message string
	var detailsJSON string
	var detailsFile string
	var metricsJSON string
	var metricsFile string
	var detailValues []string
	var metricValues []string

	cmd := &cobra.Command{
		Use:   "progress",
		Short: "Report progress for the active worker job using env-provided context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := jobsWorkerRequireAPIContext(app, jobsWorkerHelperEnvOptions{
				requireLeaseToken: true,
			})
			if err != nil {
				return writeErr(cmd, err)
			}

			details, err := parseJSONObjectInputWithAssignments(detailsJSON, detailsFile, detailValues, "details", "detail")
			if err != nil {
				return writeErr(cmd, err)
			}
			metrics, err := parseJSONObjectInputWithAssignments(metricsJSON, metricsFile, metricValues, "metrics", "metric")
			if err != nil {
				return writeErr(cmd, err)
			}

			payload := map[string]any{
				"jobId":      ctx.JobID,
				"leaseToken": ctx.LeaseToken,
			}
			if trimmed := strings.TrimSpace(status); trimmed != "" {
				normalized, err := normalizeCLIJobStatus(trimmed, activeJobStatuses, "status")
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["status"] = normalized
			}
			if trimmed := strings.TrimSpace(message); trimmed != "" {
				payload["message"] = trimmed
			}
			if details != nil {
				payload["details"] = details
			}
			if metrics != nil {
				payload["metrics"] = metrics
			}
			return doAPICommand(cmd, app, "jobs.progress", payload)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Active progress status (leased|started|running)")
	cmd.Flags().StringVar(&message, "message", "", "Progress message")
	cmd.Flags().StringVar(&detailsJSON, "details", "", "Progress details JSON object")
	cmd.Flags().StringVar(&detailsFile, "details-file", "", "Path to a JSON file containing progress details")
	cmd.Flags().StringArrayVar(&detailValues, "detail", nil, "Progress detail assignment in key=value form (repeatable)")
	cmd.Flags().StringVar(&metricsJSON, "metrics", "", "Progress metrics JSON object")
	cmd.Flags().StringVar(&metricsFile, "metrics-file", "", "Path to a JSON file containing progress metrics")
	cmd.Flags().StringArrayVar(&metricValues, "metric", nil, "Progress metric assignment in key=value form (repeatable)")
	return cmd
}

func newJobsWorkerAttachFileCmd(app *App) *cobra.Command {
	var filePath string
	var label string
	var kind string
	var contentType string
	var resourceLabel string
	var printURI bool

	cmd := &cobra.Command{
		Use:   "attach-file --file <path> --label <label>",
		Short: "Upload a file as a Breyta resource and attach it to the local worker result state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := jobsWorkerRequireAPIContext(app, jobsWorkerHelperEnvOptions{
				requireResultFile: true,
			})
			if err != nil {
				return writeErr(cmd, err)
			}

			trimmedPath := strings.TrimSpace(filePath)
			if trimmedPath == "" {
				return writeErr(cmd, errors.New("missing --file"))
			}
			trimmedLabel := strings.TrimSpace(label)
			if trimmedLabel == "" {
				return writeErr(cmd, errors.New("missing --label"))
			}
			trimmedKind := firstNonBlankString(strings.TrimSpace(kind), "file")
			filename := filepath.Base(trimmedPath)
			if filename == "" || filename == "." || filename == string(filepath.Separator) {
				return writeErr(cmd, fmt.Errorf("invalid --file path %q", trimmedPath))
			}

			uploadResult, err := jobsWorkerUploadFileResource(cmd.Context(), app, trimmedPath, filename, contentType)
			if err != nil {
				return writeErr(cmd, err)
			}

			artifact := map[string]any{
				"kind":        trimmedKind,
				"label":       trimmedLabel,
				"contentType": firstNonBlankString(toString(uploadResult["contentType"]), strings.TrimSpace(contentType)),
				"resourceUri": toString(uploadResult["resourceUri"]),
			}
			if trimmed := strings.TrimSpace(resourceLabel); trimmed != "" {
				artifact["resourceLabel"] = trimmed
			}
			if sizeBytes, ok := uploadResult["sizeBytes"]; ok {
				artifact["sizeBytes"] = sizeBytes
			}
			if filename := firstNonBlankString(uploadResult["filename"], filename); filename != "" {
				artifact["filename"] = filename
			}
			return jobsWorkerPersistArtifact(cmd, app, ctx, artifact, false, printURI)
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "Path to the file to upload")
	cmd.Flags().StringVar(&label, "label", "", "Artifact label stored on the job")
	cmd.Flags().StringVar(&kind, "kind", "", "Artifact kind (for example report, log, attachment)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Artifact content type; inferred when omitted")
	cmd.Flags().StringVar(&resourceLabel, "resource-label", "", "Optional display label for the uploaded resource")
	cmd.Flags().BoolVar(&printURI, "print-uri", false, "Print only the uploaded resource URI")
	return cmd
}

func newJobsWorkerAttachKVCmd(app *App) *cobra.Command {
	var label string
	var kind string
	var key string
	var contentType string
	var valueJSON string
	var valueFile string
	var fieldValues []string
	var ttlSeconds int
	var printURI bool

	cmd := &cobra.Command{
		Use:   "attach-kv --label <label> [--value <json> | --value-file <path> | --field key=value ...]",
		Short: "Persist a KV result resource for the active worker job and attach it locally",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := jobsWorkerRequireAPIContext(app, jobsWorkerHelperEnvOptions{
				requireLeaseToken: true,
				requireResultFile: true,
			})
			if err != nil {
				return writeErr(cmd, err)
			}

			value, err := parseAnyInputWithAssignments(valueJSON, valueFile, fieldValues, "value", "field")
			if err != nil {
				return writeErr(cmd, err)
			}
			if value == nil {
				return writeErr(cmd, errors.New("missing value (use --value, --value-file, or --field)"))
			}

			payload := map[string]any{
				"jobId":      ctx.JobID,
				"leaseToken": ctx.LeaseToken,
				"label":      strings.TrimSpace(label),
				"value":      value,
			}
			if trimmed := strings.TrimSpace(kind); trimmed != "" {
				payload["kind"] = trimmed
			}
			if trimmed := strings.TrimSpace(key); trimmed != "" {
				payload["key"] = trimmed
			}
			if trimmed := strings.TrimSpace(contentType); trimmed != "" {
				payload["contentType"] = trimmed
			}
			if cmd.Flags().Changed("ttl-seconds") {
				if ttlSeconds <= 0 {
					return writeErr(cmd, errors.New("--ttl-seconds must be > 0"))
				}
				payload["ttlSeconds"] = ttlSeconds
			}

			artifact, err := jobsWorkerAttachArtifactCommand(cmd.Context(), app, "jobs.attach_kv", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return jobsWorkerPersistArtifact(cmd, app, ctx, artifact, true, printURI)
		},
	}

	cmd.Flags().StringVar(&label, "label", "", "Artifact label stored on the job")
	cmd.Flags().StringVar(&kind, "kind", "", "Artifact kind; defaults to kv")
	cmd.Flags().StringVar(&key, "key", "", "Logical KV key suffix inside the job namespace; derived from label when omitted")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Stored resource content type; defaults to application/json")
	cmd.Flags().StringVar(&valueJSON, "value", "", "KV value as JSON")
	cmd.Flags().StringVar(&valueFile, "value-file", "", "Path to a JSON file containing the KV value")
	cmd.Flags().StringArrayVar(&fieldValues, "field", nil, "Object field assignment in key=value form (repeatable)")
	cmd.Flags().IntVar(&ttlSeconds, "ttl-seconds", 0, "Optional TTL in seconds for the KV resource")
	cmd.Flags().BoolVar(&printURI, "print-uri", false, "Print only the attached resource URI")
	return cmd
}

func newJobsWorkerAttachTableCmd(app *App) *cobra.Command {
	var label string
	var kind string
	var tableName string
	var rowsJSON string
	var rowsFile string
	var writeMode string
	var schemaMode string
	var keyFields []string
	var indexFields []string
	var contentType string
	var printURI bool

	cmd := &cobra.Command{
		Use:   "attach-table --label <label> --rows <json-array|json-object>",
		Short: "Create or update a table resource for the active worker job and attach it locally",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := jobsWorkerRequireAPIContext(app, jobsWorkerHelperEnvOptions{
				requireLeaseToken: true,
				requireResultFile: true,
			})
			if err != nil {
				return writeErr(cmd, err)
			}

			rows, err := parseJSONArrayJSONInput(rowsJSON, rowsFile, "rows")
			if err != nil {
				return writeErr(cmd, err)
			}
			if len(rows) == 0 {
				return writeErr(cmd, errors.New("missing rows (use --rows or --rows-file)"))
			}
			for idx, row := range rows {
				if _, ok := row.(map[string]any); !ok {
					return writeErr(cmd, fmt.Errorf("rows[%d] must be a JSON object", idx))
				}
			}

			payload := map[string]any{
				"jobId":      ctx.JobID,
				"leaseToken": ctx.LeaseToken,
				"label":      strings.TrimSpace(label),
				"rows":       rows,
			}
			if trimmed := strings.TrimSpace(kind); trimmed != "" {
				payload["kind"] = trimmed
			}
			if trimmed := strings.TrimSpace(tableName); trimmed != "" {
				payload["table"] = trimmed
			}
			if trimmed := strings.TrimSpace(writeMode); trimmed != "" {
				payload["writeMode"] = trimmed
			}
			if trimmed := strings.TrimSpace(schemaMode); trimmed != "" {
				payload["schemaMode"] = trimmed
			}
			if len(keyFields) > 0 {
				payload["keyFields"] = keyFields
			}
			if len(indexFields) > 0 {
				payload["indexFields"] = indexFields
			}
			if trimmed := strings.TrimSpace(contentType); trimmed != "" {
				payload["contentType"] = trimmed
			}

			artifact, err := jobsWorkerAttachArtifactCommand(cmd.Context(), app, "jobs.attach_table", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return jobsWorkerPersistArtifact(cmd, app, ctx, artifact, true, printURI)
		},
	}

	cmd.Flags().StringVar(&label, "label", "", "Artifact label stored on the job")
	cmd.Flags().StringVar(&kind, "kind", "", "Artifact kind; defaults to table")
	cmd.Flags().StringVar(&tableName, "table", "", "Logical table name suffix inside the job namespace; derived from label when omitted")
	cmd.Flags().StringVar(&rowsJSON, "rows", "", "Rows as a JSON object or array of JSON objects")
	cmd.Flags().StringVar(&rowsFile, "rows-file", "", "Path to a JSON file containing row objects")
	cmd.Flags().StringVar(&writeMode, "write-mode", "", "Table write mode (append|upsert)")
	cmd.Flags().StringVar(&schemaMode, "schema-mode", "", "Schema mode; defaults to flexible")
	cmd.Flags().StringArrayVar(&keyFields, "key-field", nil, "Upsert key field (repeatable)")
	cmd.Flags().StringArrayVar(&indexFields, "index-field", nil, "Indexed field (repeatable)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Stored resource content type override")
	cmd.Flags().BoolVar(&printURI, "print-uri", false, "Print only the attached resource URI")
	return cmd
}

func newJobsWorkerFinishCmd(app *App) *cobra.Command {
	var status string
	var summary string
	var outputsJSON string
	var outputsFile string
	var outputValues []string
	var metricsJSON string
	var metricsFile string
	var metricValues []string

	cmd := &cobra.Command{
		Use:   "finish",
		Short: "Write the local worker result state for a successful terminal job outcome",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := jobsWorkerHelperEnvContext(jobsWorkerHelperEnvOptions{
				requireResultFile: true,
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			outputs, err := parseAnyInputWithAssignments(outputsJSON, outputsFile, outputValues, "outputs", "output")
			if err != nil {
				return writeErr(cmd, err)
			}
			metrics, err := parseJSONObjectInputWithAssignments(metricsJSON, metricsFile, metricValues, "metrics", "metric")
			if err != nil {
				return writeErr(cmd, err)
			}
			state, err := jobsWorkerUpdateState(ctx.ResultFile, func(state map[string]any) error {
				if trimmed := strings.TrimSpace(status); trimmed != "" {
					normalized, err := normalizeCLIJobStatus(trimmed, completableJobStatuses, "status")
					if err != nil {
						return err
					}
					state["status"] = normalized
				} else if strings.TrimSpace(toString(state["status"])) == "" {
					state["status"] = "succeeded"
				}
				if trimmed := strings.TrimSpace(summary); trimmed != "" {
					state["summary"] = trimmed
				}
				if outputs != nil {
					state["outputs"] = outputs
				}
				if metrics != nil {
					state["metrics"] = metrics
				}
				return nil
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app,
				map[string]any{
					"jobId": ctx.JobID,
				},
				map[string]any{
					"resultFile": ctx.ResultFile,
					"status":     state["status"],
				},
			)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Completion status (succeeded|no_changes|cancelled); defaults to succeeded")
	cmd.Flags().StringVar(&summary, "summary", "", "Result summary")
	cmd.Flags().StringVar(&outputsJSON, "outputs", "", "Outputs JSON value")
	cmd.Flags().StringVar(&outputsFile, "outputs-file", "", "Path to a JSON file containing outputs")
	cmd.Flags().StringArrayVar(&outputValues, "output", nil, "Output assignment in key=value form (repeatable)")
	cmd.Flags().StringVar(&metricsJSON, "metrics", "", "Metrics JSON object")
	cmd.Flags().StringVar(&metricsFile, "metrics-file", "", "Path to a JSON file containing metrics")
	cmd.Flags().StringArrayVar(&metricValues, "metric", nil, "Metric assignment in key=value form (repeatable)")
	return cmd
}

func newJobsWorkerFailCmd(app *App) *cobra.Command {
	var message string
	var code string
	var detailsJSON string
	var detailsFile string
	var detailValues []string

	cmd := &cobra.Command{
		Use:   "fail",
		Short: "Write the local worker result state for a failed terminal job outcome",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := jobsWorkerHelperEnvContext(jobsWorkerHelperEnvOptions{
				requireResultFile: true,
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			details, err := parseJSONObjectInputWithAssignments(detailsJSON, detailsFile, detailValues, "details", "detail")
			if err != nil {
				return writeErr(cmd, err)
			}
			state, err := jobsWorkerUpdateState(ctx.ResultFile, func(state map[string]any) error {
				state["status"] = "failed"
				if trimmed := strings.TrimSpace(message); trimmed != "" {
					state["message"] = trimmed
				}
				if trimmed := strings.TrimSpace(code); trimmed != "" {
					state["code"] = trimmed
				}
				if details != nil {
					state["details"] = details
				}
				return nil
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app,
				map[string]any{
					"jobId": ctx.JobID,
				},
				map[string]any{
					"resultFile": ctx.ResultFile,
					"message":    state["message"],
					"code":       state["code"],
				},
			)
		},
	}

	cmd.Flags().StringVar(&message, "message", "", "Failure message")
	cmd.Flags().StringVar(&code, "code", "", "Failure code")
	cmd.Flags().StringVar(&detailsJSON, "details", "", "Failure details JSON object")
	cmd.Flags().StringVar(&detailsFile, "details-file", "", "Path to a JSON file containing failure details")
	cmd.Flags().StringArrayVar(&detailValues, "detail", nil, "Failure detail assignment in key=value form (repeatable)")
	return cmd
}

func jobsWorkerBuildStateSnapshot(jobDir string) (map[string]any, map[string]any, error) {
	paths, err := jobsWorkerResolveStatePaths(jobDir)
	if err != nil {
		return nil, nil, err
	}

	contextSnapshot := jobsWorkerReadStateFile(paths.ContextFile)
	jobSnapshot := jobsWorkerReadStateFile(paths.JobFile)
	payloadSnapshot := jobsWorkerReadStateFile(paths.PayloadFile)
	resultSnapshot := jobsWorkerReadStateFile(paths.ResultFile)
	contextMap, _ := jobsWorkerSnapshotJSON(contextSnapshot).(map[string]any)
	jobMap, _ := jobsWorkerSnapshotJSON(jobSnapshot).(map[string]any)

	data := map[string]any{
		"jobDir":               paths.JobDir,
		"contextFile":          paths.ContextFile,
		"jobFile":              paths.JobFile,
		"payloadFile":          paths.PayloadFile,
		"resultFile":           paths.ResultFile,
		"workerContextPresent": jobsWorkerSnapshotExists(contextSnapshot),
		"tokenPresent":         strings.TrimSpace(firstNonBlankString(os.Getenv("BREYTA_API_KEY"), os.Getenv("BREYTA_TOKEN"))) != "",
		"apiKeyPresent":        strings.TrimSpace(os.Getenv("BREYTA_API_KEY")) != "",
		"files": map[string]any{
			"context": contextSnapshot,
			"job":     jobSnapshot,
			"payload": payloadSnapshot,
			"result":  resultSnapshot,
		},
	}

	jobsWorkerMaybeSetValue(data, "workerId", jobsWorkerStateEnvValue("BREYTA_WORKER_ID", nil))
	jobsWorkerMaybeSetValue(data, "jobId", jobsWorkerStateEnvValue("BREYTA_JOB_ID", firstNonBlankString(contextMap["jobId"], jobMap["jobId"])))
	jobsWorkerMaybeSetValue(data, "jobType", jobsWorkerStateEnvValue("BREYTA_JOB_TYPE", jobMap["jobType"]))
	jobsWorkerMaybeSetValue(data, "batchId", jobsWorkerStateEnvValue("BREYTA_JOB_BATCH_ID", jobMap["batchId"]))
	jobsWorkerMaybeSetValue(data, "attempt", jobsWorkerStateEnvValue("BREYTA_JOB_ATTEMPT", jobMap["attempt"]))
	jobsWorkerMaybeSetValue(data, "workspaceId", jobsWorkerStateEnvValue("BREYTA_JOB_WORKSPACE_ID", jobMap["workspaceId"]))
	jobsWorkerMaybeSetValue(data, "apiUrl", jobsWorkerStateEnvValue("BREYTA_API_URL", nil))
	jobsWorkerMaybeSetValue(data, "workspace", jobsWorkerStateEnvValue("BREYTA_WORKSPACE", nil))

	if jsonValue := jobsWorkerSnapshotJSON(jobSnapshot); jsonValue != nil {
		data["job"] = jsonValue
	}
	if jsonValue := jobsWorkerSnapshotJSON(contextSnapshot); jsonValue != nil {
		data["context"] = jsonValue
	}
	if jsonValue := jobsWorkerSnapshotJSON(payloadSnapshot); jsonValue != nil {
		data["payload"] = jsonValue
	}
	if jsonValue := jobsWorkerSnapshotJSON(resultSnapshot); jsonValue != nil {
		data["result"] = jsonValue
	}

	meta := map[string]any{}
	jobsWorkerMaybeSetValue(meta, "workerId", data["workerId"])
	jobsWorkerMaybeSetValue(meta, "jobId", data["jobId"])
	jobsWorkerMaybeSetValue(meta, "jobType", data["jobType"])
	return data, meta, nil
}

func jobsWorkerAttachArtifactCommand(ctx context.Context, app *App, command string, payload map[string]any) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := runSuccessfulJobsCommandWithContext(ctx, app, command, payload)
	if err != nil {
		return nil, err
	}
	return jobsWorkerEnvelopeArtifact(out)
}

func jobsWorkerPersistArtifact(cmd *cobra.Command, app *App, ctx *jobsWorkerHelperContext, artifact map[string]any, alreadyPersisted bool, printURI bool) error {
	if err := jobsWorkerAppendArtifact(ctx.ResultFile, artifact, alreadyPersisted); err != nil {
		return err
	}
	return jobsWorkerWriteAttachedArtifact(cmd, app, ctx, artifact, printURI)
}

func jobsWorkerEnvelopeArtifact(out map[string]any) (map[string]any, error) {
	if out == nil {
		return nil, errors.New("missing API response")
	}
	data, _ := out["data"].(map[string]any)
	artifact, _ := data["artifact"].(map[string]any)
	if artifact == nil {
		return nil, errors.New("API response missing data.artifact")
	}
	if strings.TrimSpace(toString(artifact["resourceUri"])) == "" {
		return nil, errors.New("API response missing artifact.resourceUri")
	}
	return artifact, nil
}

func jobsWorkerWriteAttachedArtifact(cmd *cobra.Command, app *App, ctx *jobsWorkerHelperContext, artifact map[string]any, printURI bool) error {
	if printURI {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), artifact["resourceUri"])
		return nil
	}
	return writeData(cmd, app,
		map[string]any{
			"jobId": ctx.JobID,
		},
		map[string]any{
			"resultFile": ctx.ResultFile,
			"artifact":   artifact,
		},
	)
}

func jobsWorkerResolveStatePaths(jobDir string) (jobsWorkerStatePaths, error) {
	trimmedJobDir := strings.TrimSpace(jobDir)
	if trimmedJobDir != "" {
		return jobsWorkerStatePaths{
			ContextFile: filepath.Join(trimmedJobDir, "worker-context.json"),
			JobDir:      trimmedJobDir,
			JobFile:     filepath.Join(trimmedJobDir, "job.json"),
			PayloadFile: filepath.Join(trimmedJobDir, "payload.json"),
			ResultFile:  filepath.Join(trimmedJobDir, "result.json"),
		}, nil
	}

	paths := jobsWorkerStatePaths{
		ContextFile: strings.TrimSpace(os.Getenv("BREYTA_JOB_CONTEXT_FILE")),
		JobDir:      strings.TrimSpace(os.Getenv("BREYTA_JOB_DIR")),
		JobFile:     strings.TrimSpace(os.Getenv("BREYTA_JOB_FILE")),
		PayloadFile: strings.TrimSpace(os.Getenv("BREYTA_JOB_PAYLOAD_FILE")),
		ResultFile:  strings.TrimSpace(os.Getenv("BREYTA_JOB_RESULT_FILE")),
	}
	if paths.JobDir != "" {
		if paths.ContextFile == "" {
			paths.ContextFile = filepath.Join(paths.JobDir, "worker-context.json")
		}
		if paths.JobFile == "" {
			paths.JobFile = filepath.Join(paths.JobDir, "job.json")
		}
		if paths.PayloadFile == "" {
			paths.PayloadFile = filepath.Join(paths.JobDir, "payload.json")
		}
		if paths.ResultFile == "" {
			paths.ResultFile = filepath.Join(paths.JobDir, "result.json")
		}
	}
	if paths.JobDir == "" && paths.JobFile == "" && paths.PayloadFile == "" && paths.ResultFile == "" {
		return jobsWorkerStatePaths{}, errors.New("missing worker state context (use --job-dir or run inside `breyta jobs worker run`)")
	}
	if paths.JobDir == "" && paths.JobFile != "" && paths.PayloadFile != "" && paths.ResultFile != "" {
		jobDir := filepath.Dir(paths.JobFile)
		if jobDir == filepath.Dir(paths.PayloadFile) && jobDir == filepath.Dir(paths.ResultFile) {
			paths.JobDir = jobDir
			if paths.ContextFile == "" {
				paths.ContextFile = filepath.Join(jobDir, "worker-context.json")
			}
		}
	}
	return paths, nil
}

func jobsWorkerReadStateFile(path string) map[string]any {
	snapshot := map[string]any{
		"path": strings.TrimSpace(path),
	}
	if strings.TrimSpace(path) == "" {
		snapshot["configured"] = false
		return snapshot
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		snapshot["exists"] = false
		if !os.IsNotExist(err) {
			snapshot["error"] = err.Error()
		}
		return snapshot
	}

	snapshot["exists"] = true
	snapshot["sizeBytes"] = len(bytes)
	if strings.TrimSpace(string(bytes)) == "" {
		snapshot["empty"] = true
		return snapshot
	}

	var value any
	if err := json.Unmarshal(bytes, &value); err != nil {
		snapshot["jsonError"] = err.Error()
		snapshot["raw"] = string(bytes)
		return snapshot
	}

	snapshot["json"] = jobsWorkerSanitizeStateValue(value)
	return snapshot
}

func jobsWorkerSnapshotJSON(snapshot map[string]any) any {
	if snapshot == nil {
		return nil
	}
	return snapshot["json"]
}

func jobsWorkerSnapshotExists(snapshot map[string]any) bool {
	if snapshot == nil {
		return false
	}
	exists, _ := snapshot["exists"].(bool)
	return exists
}

func jobsWorkerSanitizeStateValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			if key == "leaseToken" {
				out[key] = "[redacted]"
				continue
			}
			if key == jobsWorkerPersistedArtifactMarker {
				continue
			}
			out[key] = jobsWorkerSanitizeStateValue(nested)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, nested := range typed {
			out = append(out, jobsWorkerSanitizeStateValue(nested))
		}
		return out
	default:
		return value
	}
}

func jobsWorkerStateEnvValue(envKey string, fallback any) any {
	if trimmed := strings.TrimSpace(os.Getenv(envKey)); trimmed != "" {
		parsed, err := parseJobsWorkerScalar(trimmed)
		if err == nil {
			return parsed
		}
		return trimmed
	}
	return fallback
}

func jobsWorkerMaybeSetValue(target map[string]any, key string, value any) {
	if target == nil || strings.TrimSpace(key) == "" || value == nil {
		return
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return
		}
		target[key] = typed
	default:
		target[key] = value
	}
}

type jobsWorkerHelperEnvOptions struct {
	requireLeaseToken bool
	requireResultFile bool
}

func jobsWorkerHelperEnvContext(opts jobsWorkerHelperEnvOptions) (*jobsWorkerHelperContext, error) {
	contextFile := strings.TrimSpace(os.Getenv("BREYTA_JOB_CONTEXT_FILE"))
	if contextFile == "" {
		if jobDir := strings.TrimSpace(os.Getenv("BREYTA_JOB_DIR")); jobDir != "" {
			contextFile = filepath.Join(jobDir, "worker-context.json")
		}
	}
	contextMap, err := jobsWorkerReadContextFile(contextFile)
	if err != nil {
		return nil, err
	}
	jobID := firstNonBlankString(
		strings.TrimSpace(toString(contextMap["jobId"])),
		strings.TrimSpace(os.Getenv("BREYTA_JOB_ID")),
	)
	if jobID == "" {
		return nil, errors.New("worker job context is required")
	}
	leaseToken := firstNonBlankString(
		strings.TrimSpace(toString(contextMap["leaseToken"])),
		strings.TrimSpace(os.Getenv("BREYTA_JOB_LEASE_TOKEN")),
	)
	if opts.requireLeaseToken && leaseToken == "" {
		return nil, errors.New("worker lease context is required")
	}
	resultFile := firstNonBlankString(
		strings.TrimSpace(toString(contextMap["resultFile"])),
		strings.TrimSpace(os.Getenv("BREYTA_JOB_RESULT_FILE")),
	)
	if opts.requireResultFile && resultFile == "" {
		return nil, errors.New("worker result context is required")
	}
	return &jobsWorkerHelperContext{
		JobID:       jobID,
		LeaseToken:  leaseToken,
		ResultFile:  resultFile,
		ContextFile: contextFile,
	}, nil
}

func jobsWorkerReadContextFile(path string) (map[string]any, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read worker context file: %w", err)
	}
	if strings.TrimSpace(string(bytes)) == "" {
		return nil, nil
	}
	var raw any
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return nil, fmt.Errorf("invalid worker context json: %w", err)
	}
	contextMap, _ := raw.(map[string]any)
	if contextMap == nil {
		return nil, errors.New("worker context file must contain a JSON object")
	}
	return contextMap, nil
}

func jobsWorkerRequireAPIContext(app *App, opts jobsWorkerHelperEnvOptions) (*jobsWorkerHelperContext, error) {
	ctx, err := jobsWorkerHelperEnvContext(opts)
	if err != nil {
		return nil, err
	}
	hydrateJobsWorkerHelperAppFromEnv(app)
	if err := requireAPI(app); err != nil {
		return nil, err
	}
	return ctx, nil
}

func hydrateJobsWorkerHelperAppFromEnv(app *App) {
	if app == nil {
		return
	}
	if strings.TrimSpace(app.APIURL) == "" {
		if apiURL := strings.TrimSpace(os.Getenv("BREYTA_API_URL")); apiURL != "" {
			app.APIURL = apiURL
		}
	}
	if strings.TrimSpace(app.WorkspaceID) == "" {
		if workspaceID := strings.TrimSpace(os.Getenv("BREYTA_WORKSPACE")); workspaceID != "" {
			app.WorkspaceID = workspaceID
		}
	}
	if strings.TrimSpace(app.APIKey) == "" {
		if apiKey := strings.TrimSpace(os.Getenv("BREYTA_API_KEY")); apiKey != "" {
			app.APIKey = apiKey
			app.APIKeyExplicit = true
			app.Token = apiKey
			app.TokenExplicit = true
		}
	}
	if strings.TrimSpace(app.Token) == "" {
		if token := strings.TrimSpace(os.Getenv("BREYTA_TOKEN")); token != "" {
			app.Token = token
			app.TokenExplicit = true
		}
	}
}

func readJobsWorkerState(path string) (map[string]any, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("result file path is required")
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read result file: %w", err)
	}
	if strings.TrimSpace(string(bytes)) == "" {
		return map[string]any{}, nil
	}
	var state map[string]any
	if err := json.Unmarshal(bytes, &state); err != nil {
		return nil, fmt.Errorf("invalid result file json: %w", err)
	}
	if state == nil {
		return map[string]any{}, nil
	}
	return state, nil
}

func writeJobsWorkerState(path string, state map[string]any) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("result file path is required")
	}
	if state == nil {
		state = map[string]any{}
	}
	return writeJobsWorkerJSONFile(path, state)
}

func jobsWorkerUpdateState(path string, mutate func(map[string]any) error) (map[string]any, error) {
	state, err := readJobsWorkerState(path)
	if err != nil {
		return nil, err
	}
	if mutate != nil {
		if err := mutate(state); err != nil {
			return nil, err
		}
	}
	if err := writeJobsWorkerState(path, state); err != nil {
		return nil, err
	}
	return state, nil
}

func jobsWorkerAppendArtifact(path string, artifact map[string]any, alreadyPersisted bool) error {
	_, err := jobsWorkerUpdateState(path, func(state map[string]any) error {
		var artifacts []any
		switch typed := state["artifacts"].(type) {
		case nil:
		case []any:
			artifacts = append(artifacts, typed...)
		default:
			artifacts = append(artifacts, typed)
		}
		copyArtifact := make(map[string]any, len(artifact)+1)
		for key, value := range artifact {
			copyArtifact[key] = value
		}
		if alreadyPersisted {
			copyArtifact[jobsWorkerPersistedArtifactMarker] = true
		}
		state["artifacts"] = append(artifacts, copyArtifact)
		return nil
	})
	return err
}

func parseStructuredKeyAssignments(values []string, flagName string) (map[string]any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --%s value %q (expected field=value)", flagName, raw)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("invalid --%s value %q (missing field)", flagName, raw)
		}
		value, err := parseJobsWorkerScalar(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid --%s value %q: %w", flagName, raw, err)
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func parseJSONObjectInputWithAssignments(raw string, filePath string, values []string, label string, flagName string) (map[string]any, error) {
	base, err := parseJSONObjectJSONInput(raw, filePath, label)
	if err != nil {
		return nil, err
	}
	overlay, err := parseStructuredKeyAssignments(values, flagName)
	if err != nil {
		return nil, err
	}
	if base == nil && overlay == nil {
		return nil, nil
	}
	return mergeJSONObjectFlags(base, overlay), nil
}

func parseJobsWorkerScalar(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed, nil
	}
	return raw, nil
}

func mergeAnyWithMap(base any, overlay map[string]any, label string) (any, error) {
	if len(overlay) == 0 {
		return base, nil
	}
	if base == nil {
		return overlay, nil
	}
	baseMap, ok := base.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object when used with repeated field flags", label)
	}
	return mergeJSONObjectFlags(baseMap, overlay), nil
}

func parseAnyInputWithAssignments(raw string, filePath string, values []string, label string, flagName string) (any, error) {
	base, err := parseAnyJSONInput(raw, filePath, label)
	if err != nil {
		return nil, err
	}
	overlay, err := parseStructuredKeyAssignments(values, flagName)
	if err != nil {
		return nil, err
	}
	return mergeAnyWithMap(base, overlay, label)
}

func detectJobsWorkerContentType(path string, explicit string, file *os.File) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	if ext := strings.TrimSpace(filepath.Ext(path)); ext != "" {
		switch strings.ToLower(ext) {
		case ".md", ".markdown":
			return "text/markdown; charset=utf-8", nil
		}
		if guessed := strings.TrimSpace(mime.TypeByExtension(ext)); guessed != "" {
			return guessed, nil
		}
	}
	if file == nil {
		return "", errors.New("file handle is required for content-type detection")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek file for content-type detection: %w", err)
	}
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read file for content-type detection: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("reset file after content-type detection: %w", err)
	}
	return http.DetectContentType(header[:n]), nil
}

func jobsWorkerUploadFileResource(ctx context.Context, app *App, path string, filename string, contentType string) (map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open upload file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat upload file: %w", err)
	}
	contentType, err = detectJobsWorkerContentType(path, contentType, file)
	if err != nil {
		return nil, err
	}

	initResp, status, err := apiClient(app).DoREST(ctx, http.MethodPost, "/api/files/uploads/init", nil, map[string]any{
		"filename":     filename,
		"content-type": contentType,
	})
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, jobsWorkerRESTError(status, initResp)
	}

	initData := jobsWorkerRESTPayload(initResp)
	resourceURI := firstNonBlankString(initData["uri"])
	uploadURL := firstNonBlankString(initData["upload-url"], initData["uploadUrl"])
	if resourceURI == "" {
		return nil, errors.New("upload init response missing resource uri")
	}

	if jobsWorkerSupportsSignedUploadURL(uploadURL) {
		if err := jobsWorkerUploadWithSignedURL(ctx, uploadURL, contentType, file, fileInfo.Size()); err != nil {
			return nil, err
		}
	} else {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("reset upload file for direct upload: %w", err)
		}
		if err := jobsWorkerUploadWithAPIDirect(ctx, app, resourceURI, contentType, file, fileInfo.Size()); err != nil {
			return nil, err
		}
	}

	completeResp, status, err := apiClient(app).DoREST(ctx, http.MethodPost, "/api/files/uploads/complete", nil, map[string]any{
		"uri": resourceURI,
	})
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, jobsWorkerRESTError(status, completeResp)
	}
	completeData := jobsWorkerRESTPayload(completeResp)
	result := map[string]any{
		"resourceUri": resourceURI,
		"contentType": firstNonBlankString(completeData["content-type"], completeData["contentType"], contentType),
		"filename":    filename,
	}
	if sizeBytes, ok := completeData["size-bytes"]; ok {
		result["sizeBytes"] = sizeBytes
	} else if sizeBytes, ok := completeData["sizeBytes"]; ok {
		result["sizeBytes"] = sizeBytes
	} else if fileInfo.Size() >= 0 {
		result["sizeBytes"] = fileInfo.Size()
	}
	return result, nil
}

func jobsWorkerRESTPayload(resp any) map[string]any {
	out := mapStringAny(resp)
	if len(out) == 0 {
		return nil
	}
	if data := mapStringAny(out["data"]); len(data) > 0 {
		return data
	}
	return out
}

func jobsWorkerRESTError(status int, resp any) error {
	if out := mapStringAny(resp); len(out) > 0 {
		return fmt.Errorf("api error (status=%d): %s", status, formatAPIError(out))
	}
	msg := strings.TrimSpace(fmt.Sprintf("%v", resp))
	if msg == "" || msg == "<nil>" {
		msg = "unknown error"
	}
	return fmt.Errorf("api error (status=%d): %s", status, msg)
}

func jobsWorkerSupportsSignedUploadURL(uploadURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(uploadURL))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func jobsWorkerUploadWithSignedURL(ctx context.Context, uploadURL string, contentType string, body io.Reader, contentLength int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, body)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload failed (status=%d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

func jobsWorkerUploadWithAPIDirect(ctx context.Context, app *App, resourceURI string, contentType string, body io.Reader, contentLength int64) error {
	query := url.Values{}
	query.Set("uri", resourceURI)
	out, status, err := apiClient(app).DoRootRESTReader(ctx, http.MethodPut, "/api/files/uploads/direct", query, body, contentType, contentLength, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return jobsWorkerRESTError(status, out)
	}
	return nil
}
