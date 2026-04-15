package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	allJobStatuses = map[string]bool{
		"queued":     true,
		"leased":     true,
		"started":    true,
		"running":    true,
		"succeeded":  true,
		"no_changes": true,
		"failed":     true,
		"cancelled":  true,
		"timed_out":  true,
	}
	activeJobStatuses = map[string]bool{
		"leased":  true,
		"started": true,
		"running": true,
	}
	completableJobStatuses = map[string]bool{
		"succeeded":  true,
		"no_changes": true,
		"cancelled":  true,
	}
)

func newJobsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "Create, inspect, and lease external jobs",
		Long: strings.TrimSpace(`
Use jobs as the control-plane surface for external work.

- flows or operators create jobs and batches
- workers claim leases by job type
- workers heartbeat, report progress, and complete or fail jobs

Worker execution stays outside the flow runtime. Breyta owns the durable job state.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newJobsCreateCmd(app))
	cmd.AddCommand(newJobsListCmd(app))
	cmd.AddCommand(newJobsShowCmd(app))
	cmd.AddCommand(newJobsClaimCmd(app))
	cmd.AddCommand(newJobsHeartbeatCmd(app))
	cmd.AddCommand(newJobsProgressCmd(app))
	cmd.AddCommand(newJobsCompleteCmd(app))
	cmd.AddCommand(newJobsFailCmd(app))
	cmd.AddCommand(newJobsBatchesCmd(app))
	cmd.AddCommand(newJobsWorkerCmd(app))
	return cmd
}

func newJobsCreateCmd(app *App) *cobra.Command {
	var jobType string
	var rootWorkflowID string
	var parentStepID string
	var fanoutParentStepID string
	var fanoutMaxConcurrency int
	var payloadJSON string
	var payloadFile string
	var metadataJSON string
	var metadataFile string
	var maxAttempts int

	cmd := &cobra.Command{
		Use:   "create --type <job-type>",
		Short: "Create one queued job",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobType = strings.TrimSpace(jobType)
			if jobType == "" {
				return writeErr(cmd, errors.New("missing --type"))
			}
			payload, err := parseJSONObjectJSONInput(payloadJSON, payloadFile, "payload")
			if err != nil {
				return writeErr(cmd, err)
			}
			metadata, err := parseJSONObjectJSONInput(metadataJSON, metadataFile, "metadata")
			if err != nil {
				return writeErr(cmd, err)
			}
			payloadMap := map[string]any{"jobType": jobType}
			if trimmed := strings.TrimSpace(rootWorkflowID); trimmed != "" {
				payloadMap["rootWorkflowId"] = trimmed
			}
			if trimmed := strings.TrimSpace(parentStepID); trimmed != "" {
				payloadMap["parentStepId"] = trimmed
			}
			if trimmed := strings.TrimSpace(fanoutParentStepID); trimmed != "" {
				payloadMap["fanoutParentStepId"] = trimmed
			}
			if cmd.Flags().Changed("fanout-max-concurrency") {
				if fanoutMaxConcurrency <= 0 {
					return writeErr(cmd, errors.New("--fanout-max-concurrency must be > 0"))
				}
				payloadMap["fanoutMaxConcurrency"] = fanoutMaxConcurrency
			}
			if payload != nil {
				payloadMap["payload"] = payload
			}
			if metadata != nil {
				payloadMap["metadata"] = metadata
			}
			if cmd.Flags().Changed("max-attempts") {
				if maxAttempts <= 0 {
					return writeErr(cmd, errors.New("--max-attempts must be > 0"))
				}
				payloadMap["maxAttempts"] = maxAttempts
			}
			return doAPICommand(cmd, app, "jobs.create", payloadMap)
		},
	}
	cmd.Flags().StringVar(&jobType, "type", "", "Job type to create")
	cmd.Flags().StringVar(&rootWorkflowID, "root-workflow-id", "", "Optional owning root workflow id")
	cmd.Flags().StringVar(&parentStepID, "parent-step-id", "", "Optional owning parent step id")
	cmd.Flags().StringVar(&fanoutParentStepID, "fanout-parent-step-id", "", "Optional upstream fanout parent step id for trace alignment")
	cmd.Flags().IntVar(&fanoutMaxConcurrency, "fanout-max-concurrency", 0, "Optional upstream fanout max concurrency for trace alignment")
	cmd.Flags().StringVar(&payloadJSON, "payload", "", "Job payload JSON object")
	cmd.Flags().StringVar(&payloadFile, "payload-file", "", "Path to a JSON file containing the job payload object")
	cmd.Flags().StringVar(&metadataJSON, "metadata", "", "Job metadata JSON object")
	cmd.Flags().StringVar(&metadataFile, "metadata-file", "", "Path to a JSON file containing job metadata")
	cmd.Flags().IntVar(&maxAttempts, "max-attempts", 0, "Max attempts before the job times out")
	return cmd
}

func newJobsListCmd(app *App) *cobra.Command {
	var jobType string
	var batchID string
	var status string
	var limit int

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List jobs",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if trimmed := strings.TrimSpace(jobType); trimmed != "" {
				payload["jobType"] = trimmed
			}
			if trimmed := strings.TrimSpace(batchID); trimmed != "" {
				payload["batchId"] = trimmed
			}
			if trimmed := strings.TrimSpace(status); trimmed != "" {
				normalized, err := normalizeCLIJobStatus(trimmed, allJobStatuses, "status")
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["status"] = normalized
			}
			if cmd.Flags().Changed("limit") {
				if limit <= 0 {
					return writeErr(cmd, errors.New("--limit must be > 0"))
				}
				payload["limit"] = limit
			}
			return doAPICommand(cmd, app, "jobs.list", payload)
		},
	}
	cmd.Flags().StringVar(&jobType, "type", "", "Filter by job type")
	cmd.Flags().StringVar(&batchID, "batch-id", "", "Filter by batch id")
	cmd.Flags().StringVar(&status, "status", "", "Filter by job status")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max jobs to return")
	return cmd
}

func newJobsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <job-id>",
		Short: "Show one job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := strings.TrimSpace(args[0])
			if jobID == "" {
				return writeErr(cmd, errors.New("missing job id"))
			}
			return doAPICommand(cmd, app, "jobs.get", map[string]any{"jobId": jobID})
		},
	}
	return cmd
}

func newJobsClaimCmd(app *App) *cobra.Command {
	var jobType string
	var workerID string
	var batchID string
	var labels []string
	var leaseDuration time.Duration

	cmd := &cobra.Command{
		Use:   "claim --type <job-type> --worker-id <worker-id>",
		Short: "Claim one job lease for a worker",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobType = strings.TrimSpace(jobType)
			workerID = strings.TrimSpace(workerID)
			if jobType == "" {
				return writeErr(cmd, errors.New("missing --type"))
			}
			if workerID == "" {
				return writeErr(cmd, errors.New("missing --worker-id"))
			}
			workerLabels, err := parseKeyAssignments(labels)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --label: %w", err))
			}
			payload := map[string]any{
				"jobType":  jobType,
				"workerId": workerID,
			}
			if trimmed := strings.TrimSpace(batchID); trimmed != "" {
				payload["batchId"] = trimmed
			}
			if len(workerLabels) > 0 {
				payload["workerLabels"] = workerLabels
			}
			if cmd.Flags().Changed("lease-duration") {
				if leaseDuration <= 0 {
					return writeErr(cmd, errors.New("--lease-duration must be > 0"))
				}
				payload["leaseDuration"] = leaseDuration.Milliseconds()
			}
			return doAPICommand(cmd, app, "jobs.claim", payload)
		},
	}
	cmd.Flags().StringVar(&jobType, "type", "", "Job type to claim")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "Logical worker id")
	cmd.Flags().StringVar(&batchID, "batch-id", "", "Optional batch id to constrain claims")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "Worker label assignment in key=value form (repeatable)")
	cmd.Flags().DurationVar(&leaseDuration, "lease-duration", 0, "Lease duration, e.g. 30s or 5m")
	return cmd
}

func newJobsHeartbeatCmd(app *App) *cobra.Command {
	var leaseToken string
	var leaseDuration time.Duration

	cmd := &cobra.Command{
		Use:   "heartbeat <job-id>",
		Short: "Extend the active lease for one job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := strings.TrimSpace(args[0])
			leaseToken = strings.TrimSpace(leaseToken)
			if jobID == "" {
				return writeErr(cmd, errors.New("missing job id"))
			}
			if leaseToken == "" {
				return writeErr(cmd, errors.New("missing --lease-token"))
			}
			payload := map[string]any{
				"jobId":      jobID,
				"leaseToken": leaseToken,
			}
			if cmd.Flags().Changed("lease-duration") {
				if leaseDuration <= 0 {
					return writeErr(cmd, errors.New("--lease-duration must be > 0"))
				}
				payload["leaseDuration"] = leaseDuration.Milliseconds()
			}
			return doAPICommand(cmd, app, "jobs.heartbeat", payload)
		},
	}
	cmd.Flags().StringVar(&leaseToken, "lease-token", "", "Active lease token for the job")
	cmd.Flags().DurationVar(&leaseDuration, "lease-duration", 0, "Lease duration, e.g. 30s or 5m")
	return cmd
}

func newJobsProgressCmd(app *App) *cobra.Command {
	var leaseToken string
	var status string
	var message string
	var detailsJSON string
	var detailsFile string
	var metricsJSON string
	var metricsFile string
	var artifactsJSON string
	var artifactsFile string

	cmd := &cobra.Command{
		Use:   "progress <job-id>",
		Short: "Update job progress under the active lease",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := strings.TrimSpace(args[0])
			leaseToken = strings.TrimSpace(leaseToken)
			if jobID == "" {
				return writeErr(cmd, errors.New("missing job id"))
			}
			if leaseToken == "" {
				return writeErr(cmd, errors.New("missing --lease-token"))
			}
			details, err := parseJSONObjectJSONInput(detailsJSON, detailsFile, "details")
			if err != nil {
				return writeErr(cmd, err)
			}
			metrics, err := parseJSONObjectJSONInput(metricsJSON, metricsFile, "metrics")
			if err != nil {
				return writeErr(cmd, err)
			}
			artifacts, err := parseJSONArrayJSONInput(artifactsJSON, artifactsFile, "artifacts")
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"jobId":      jobID,
				"leaseToken": leaseToken,
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
			if artifacts != nil {
				payload["artifacts"] = artifacts
			}
			return doAPICommand(cmd, app, "jobs.progress", payload)
		},
	}
	cmd.Flags().StringVar(&leaseToken, "lease-token", "", "Active lease token for the job")
	cmd.Flags().StringVar(&status, "status", "", "Active progress status (leased|started|running)")
	cmd.Flags().StringVar(&message, "message", "", "Progress message")
	cmd.Flags().StringVar(&detailsJSON, "details", "", "Progress details JSON object")
	cmd.Flags().StringVar(&detailsFile, "details-file", "", "Path to a JSON file containing progress details")
	cmd.Flags().StringVar(&metricsJSON, "metrics", "", "Progress metrics JSON object")
	cmd.Flags().StringVar(&metricsFile, "metrics-file", "", "Path to a JSON file containing progress metrics")
	cmd.Flags().StringVar(&artifactsJSON, "artifacts", "", "Progress artifacts JSON value or array")
	cmd.Flags().StringVar(&artifactsFile, "artifacts-file", "", "Path to a JSON file containing progress artifacts")
	return cmd
}

func newJobsCompleteCmd(app *App) *cobra.Command {
	var leaseToken string
	var status string
	var summary string
	var outputsJSON string
	var outputsFile string
	var metricsJSON string
	var metricsFile string
	var artifactsJSON string
	var artifactsFile string
	var workerInfoJSON string
	var workerInfoFile string

	cmd := &cobra.Command{
		Use:   "complete <job-id>",
		Short: "Mark a leased job complete",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := strings.TrimSpace(args[0])
			leaseToken = strings.TrimSpace(leaseToken)
			if jobID == "" {
				return writeErr(cmd, errors.New("missing job id"))
			}
			if leaseToken == "" {
				return writeErr(cmd, errors.New("missing --lease-token"))
			}
			outputs, err := parseAnyJSONInput(outputsJSON, outputsFile, "outputs")
			if err != nil {
				return writeErr(cmd, err)
			}
			metrics, err := parseJSONObjectJSONInput(metricsJSON, metricsFile, "metrics")
			if err != nil {
				return writeErr(cmd, err)
			}
			artifacts, err := parseJSONArrayJSONInput(artifactsJSON, artifactsFile, "artifacts")
			if err != nil {
				return writeErr(cmd, err)
			}
			workerInfo, err := parseJSONObjectJSONInput(workerInfoJSON, workerInfoFile, "worker-info")
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"jobId":      jobID,
				"leaseToken": leaseToken,
			}
			if trimmed := strings.TrimSpace(status); trimmed != "" {
				normalized, err := normalizeCLIJobStatus(trimmed, completableJobStatuses, "status")
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["status"] = normalized
			}
			if trimmed := strings.TrimSpace(summary); trimmed != "" {
				payload["summary"] = trimmed
			}
			if outputs != nil {
				payload["outputs"] = outputs
			}
			if metrics != nil {
				payload["metrics"] = metrics
			}
			if artifacts != nil {
				payload["artifacts"] = artifacts
			}
			if workerInfo != nil {
				payload["workerInfo"] = workerInfo
			}
			return doAPICommand(cmd, app, "jobs.complete", payload)
		},
	}
	cmd.Flags().StringVar(&leaseToken, "lease-token", "", "Active lease token for the job")
	cmd.Flags().StringVar(&status, "status", "", "Completion status (succeeded|no_changes|cancelled)")
	cmd.Flags().StringVar(&summary, "summary", "", "Result summary")
	cmd.Flags().StringVar(&outputsJSON, "outputs", "", "Outputs JSON value")
	cmd.Flags().StringVar(&outputsFile, "outputs-file", "", "Path to a JSON file containing outputs")
	cmd.Flags().StringVar(&metricsJSON, "metrics", "", "Metrics JSON object")
	cmd.Flags().StringVar(&metricsFile, "metrics-file", "", "Path to a JSON file containing metrics")
	cmd.Flags().StringVar(&artifactsJSON, "artifacts", "", "Artifacts JSON value or array")
	cmd.Flags().StringVar(&artifactsFile, "artifacts-file", "", "Path to a JSON file containing artifacts")
	cmd.Flags().StringVar(&workerInfoJSON, "worker-info", "", "Worker info JSON object")
	cmd.Flags().StringVar(&workerInfoFile, "worker-info-file", "", "Path to a JSON file containing worker info")
	return cmd
}

func newJobsFailCmd(app *App) *cobra.Command {
	var leaseToken string
	var message string
	var code string
	var detailsJSON string
	var detailsFile string
	var artifactsJSON string
	var artifactsFile string

	cmd := &cobra.Command{
		Use:   "fail <job-id>",
		Short: "Mark a leased job failed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := strings.TrimSpace(args[0])
			leaseToken = strings.TrimSpace(leaseToken)
			if jobID == "" {
				return writeErr(cmd, errors.New("missing job id"))
			}
			if leaseToken == "" {
				return writeErr(cmd, errors.New("missing --lease-token"))
			}
			details, err := parseJSONObjectJSONInput(detailsJSON, detailsFile, "details")
			if err != nil {
				return writeErr(cmd, err)
			}
			artifacts, err := parseJSONArrayJSONInput(artifactsJSON, artifactsFile, "artifacts")
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"jobId":      jobID,
				"leaseToken": leaseToken,
			}
			if trimmed := strings.TrimSpace(message); trimmed != "" {
				payload["message"] = trimmed
			}
			if trimmed := strings.TrimSpace(code); trimmed != "" {
				payload["code"] = trimmed
			}
			if details != nil {
				payload["details"] = details
			}
			if artifacts != nil {
				payload["artifacts"] = artifacts
			}
			return doAPICommand(cmd, app, "jobs.fail", payload)
		},
	}
	cmd.Flags().StringVar(&leaseToken, "lease-token", "", "Active lease token for the job")
	cmd.Flags().StringVar(&message, "message", "", "Failure message")
	cmd.Flags().StringVar(&code, "code", "", "Failure code")
	cmd.Flags().StringVar(&detailsJSON, "details", "", "Failure details JSON object")
	cmd.Flags().StringVar(&detailsFile, "details-file", "", "Path to a JSON file containing failure details")
	cmd.Flags().StringVar(&artifactsJSON, "artifacts", "", "Artifacts JSON value or array")
	cmd.Flags().StringVar(&artifactsFile, "artifacts-file", "", "Path to a JSON file containing artifacts")
	return cmd
}

func newJobsBatchesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batches",
		Short: "Create and inspect job batches",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newJobsBatchesCreateCmd(app))
	cmd.AddCommand(newJobsBatchesShowCmd(app))
	return cmd
}

func newJobsBatchesCreateCmd(app *App) *cobra.Command {
	var jobType string
	var rootWorkflowID string
	var parentStepID string
	var fanoutParentStepID string
	var fanoutMaxConcurrency int
	var metadataJSON string
	var metadataFile string
	var jobItems []string
	var jobsFile string

	cmd := &cobra.Command{
		Use:   "create --type <job-type> --job '{...}' [--job '{...}' ...]",
		Short: "Create one batch of queued jobs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobType = strings.TrimSpace(jobType)
			if jobType == "" {
				return writeErr(cmd, errors.New("missing --type"))
			}
			metadata, err := parseJSONObjectJSONInput(metadataJSON, metadataFile, "metadata")
			if err != nil {
				return writeErr(cmd, err)
			}
			items, err := parseBatchJobItems(jobItems, jobsFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if len(items) == 0 {
				return writeErr(cmd, errors.New("provide at least one --job or --jobs-file"))
			}
			payload := map[string]any{
				"jobType": jobType,
				"jobs":    items,
			}
			if trimmed := strings.TrimSpace(rootWorkflowID); trimmed != "" {
				payload["rootWorkflowId"] = trimmed
			}
			if trimmed := strings.TrimSpace(parentStepID); trimmed != "" {
				payload["parentStepId"] = trimmed
			}
			if trimmed := strings.TrimSpace(fanoutParentStepID); trimmed != "" {
				payload["fanoutParentStepId"] = trimmed
			}
			if cmd.Flags().Changed("fanout-max-concurrency") {
				if fanoutMaxConcurrency <= 0 {
					return writeErr(cmd, errors.New("--fanout-max-concurrency must be > 0"))
				}
				payload["fanoutMaxConcurrency"] = fanoutMaxConcurrency
			}
			if metadata != nil {
				payload["metadata"] = metadata
			}
			return doAPICommand(cmd, app, "jobs.batches.create", payload)
		},
	}
	cmd.Flags().StringVar(&jobType, "type", "", "Job type for all items in the batch")
	cmd.Flags().StringVar(&rootWorkflowID, "root-workflow-id", "", "Optional owning root workflow id")
	cmd.Flags().StringVar(&parentStepID, "parent-step-id", "", "Optional owning parent step id")
	cmd.Flags().StringVar(&fanoutParentStepID, "fanout-parent-step-id", "", "Optional upstream fanout parent step id for trace alignment")
	cmd.Flags().IntVar(&fanoutMaxConcurrency, "fanout-max-concurrency", 0, "Optional upstream fanout max concurrency for trace alignment")
	cmd.Flags().StringVar(&metadataJSON, "metadata", "", "Batch metadata JSON object")
	cmd.Flags().StringVar(&metadataFile, "metadata-file", "", "Path to a JSON file containing batch metadata")
	cmd.Flags().StringArrayVar(&jobItems, "job", nil, "Batch item JSON object (repeatable)")
	cmd.Flags().StringVar(&jobsFile, "jobs-file", "", "Path to a JSON file containing one batch item object or an array of batch item objects")
	return cmd
}

func newJobsBatchesShowCmd(app *App) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "show <batch-id>",
		Short: "Show one batch and its jobs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			batchID := strings.TrimSpace(args[0])
			if batchID == "" {
				return writeErr(cmd, errors.New("missing batch id"))
			}
			payload := map[string]any{"batchId": batchID}
			if cmd.Flags().Changed("limit") {
				if limit <= 0 {
					return writeErr(cmd, errors.New("--limit must be > 0"))
				}
				payload["limit"] = limit
			}
			return doAPICommand(cmd, app, "jobs.batches.get", payload)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Max jobs to include in the batch response")
	return cmd
}

func normalizeCLIJobStatus(raw string, allowed map[string]bool, fieldName string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized == "" {
		return "", fmt.Errorf("missing %s", fieldName)
	}
	if !allowed[normalized] {
		return "", fmt.Errorf("invalid %s %q", fieldName, raw)
	}
	return normalized, nil
}

func parseJSONSource(raw string, filePath string, label string) (any, error) {
	trimmedRaw := strings.TrimSpace(raw)
	trimmedPath := strings.TrimSpace(filePath)
	if trimmedRaw != "" && trimmedPath != "" {
		return nil, fmt.Errorf("use either --%s or --%s-file, not both", label, label)
	}
	if trimmedPath != "" {
		bytes, err := os.ReadFile(trimmedPath)
		if err != nil {
			return nil, fmt.Errorf("read %s file: %w", label, err)
		}
		trimmedRaw = string(bytes)
	}
	trimmedRaw = strings.TrimSpace(trimmedRaw)
	if trimmedRaw == "" {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal([]byte(trimmedRaw), &value); err != nil {
		return nil, fmt.Errorf("invalid %s json: %w", label, err)
	}
	return value, nil
}

func parseAnyJSONInput(raw string, filePath string, label string) (any, error) {
	return parseJSONSource(raw, filePath, label)
}

func parseJSONObjectJSONInput(raw string, filePath string, label string) (map[string]any, error) {
	value, err := parseJSONSource(raw, filePath, label)
	if err != nil || value == nil {
		return nil, err
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	return m, nil
}

func parseJSONArrayJSONInput(raw string, filePath string, label string) ([]any, error) {
	value, err := parseJSONSource(raw, filePath, label)
	if err != nil || value == nil {
		return nil, err
	}
	if items, ok := value.([]any); ok {
		return items, nil
	}
	return []any{value}, nil
}

func parseBatchJobItems(rawItems []string, jobsFile string) ([]map[string]any, error) {
	items := make([]map[string]any, 0, len(rawItems))
	for _, raw := range rawItems {
		item, err := parseJSONObjectJSONInput(raw, "", "job")
		if err != nil {
			return nil, err
		}
		if item != nil {
			items = append(items, item)
		}
	}
	fileValue, err := parseJSONSource("", jobsFile, "jobs")
	if err != nil {
		return nil, err
	}
	switch typed := fileValue.(type) {
	case nil:
	case map[string]any:
		items = append(items, typed)
	case []any:
		for _, raw := range typed {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("jobs-file must contain an object or an array of objects")
			}
			items = append(items, item)
		}
	default:
		return nil, errors.New("jobs-file must contain an object or an array of objects")
	}
	return items, nil
}
