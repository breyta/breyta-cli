package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type jobsWorkerConfig struct {
	jobType       string
	workerID      string
	batchID       string
	workerLabels  map[string]any
	handler       string
	handlerArgs   []string
	leaseDuration time.Duration
	pollInterval  time.Duration
	once          bool
	keepJobDirs   bool
}

type jobsWorkerResult struct {
	Status     string
	Summary    string
	Outputs    any
	Metrics    map[string]any
	Artifacts  []any
	WorkerInfo map[string]any
	Message    string
	Details    map[string]any
	Code       string
}

type jobsWorkerExecutionResult struct {
	Job    map[string]any
	JobDir string
}

type jobsWorkerSummary struct {
	WorkerID     string
	JobType      string
	BatchID      string
	HandledCount int
	FailedCount  int
	IdlePolls    int
	Once         bool
	LastJob      map[string]any
	LastJobDir   string
}

const jobsWorkerPersistedArtifactMarker = "_breytaPersisted"

func newJobsWorkerCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Run worker loops for claimable job types",
		Annotations: map[string]string{
			allowAPIEnvOverrideAnnotation: "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newJobsWorkerRunCmd(app))
	cmd.AddCommand(newJobsWorkerStateCmd(app))
	cmd.AddCommand(newJobsWorkerProgressCmd(app))
	cmd.AddCommand(newJobsWorkerAttachFileCmd(app))
	cmd.AddCommand(newJobsWorkerAttachKVCmd(app))
	cmd.AddCommand(newJobsWorkerAttachTableCmd(app))
	cmd.AddCommand(newJobsWorkerFinishCmd(app))
	cmd.AddCommand(newJobsWorkerFailCmd(app))
	return cmd
}

func newJobsWorkerRunCmd(app *App) *cobra.Command {
	var jobType string
	var workerID string
	var batchID string
	var labels []string
	var handler string
	var handlerArgs []string
	var leaseDuration time.Duration
	var pollInterval time.Duration
	var once bool
	var keepJobDirs bool

	cmd := &cobra.Command{
		Use:   "run --type <job-type> --handler <command> [--handler-arg <arg> ...]",
		Short: "Claim and execute jobs for one job type",
		Long: strings.TrimSpace(`
Run a worker loop for one job type.

The worker claims one job at a time, materializes the payload locally, runs the
configured handler, renews the lease while the handler runs, then completes
or fails the job through the same Jobs API surface.
`),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobType = strings.TrimSpace(jobType)
			if jobType == "" {
				return writeErr(cmd, errors.New("missing --type"))
			}
			resolvedHandler, resolvedArgs, err := resolveJobsWorkerHandler(handler, handlerArgs, args)
			if err != nil {
				return writeErr(cmd, err)
			}
			workerLabels, err := parseKeyAssignments(labels)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --label: %w", err))
			}
			if leaseDuration <= 0 {
				return writeErr(cmd, errors.New("--lease-duration must be > 0"))
			}
			if pollInterval <= 0 {
				return writeErr(cmd, errors.New("--poll-interval must be > 0"))
			}
			cfg := jobsWorkerConfig{
				jobType:       jobType,
				workerID:      firstNonBlankString(strings.TrimSpace(workerID), defaultJobsWorkerID()),
				batchID:       strings.TrimSpace(batchID),
				workerLabels:  workerLabels,
				handler:       resolvedHandler,
				handlerArgs:   resolvedArgs,
				leaseDuration: leaseDuration,
				pollInterval:  pollInterval,
				once:          once,
				keepJobDirs:   keepJobDirs,
			}

			sigCtx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			summary, err := runJobsWorkerLoop(sigCtx, cmd.ErrOrStderr(), app, cfg)
			if err != nil {
				return writeFailure(
					cmd,
					app,
					"jobs_worker_failed",
					err,
					"Check API auth, worker handler configuration, and active lease state.",
					map[string]any{
						"workerId": cfg.workerID,
						"jobType":  cfg.jobType,
						"batchId":  cfg.batchID,
						"handler":  cfg.handler,
					},
				)
			}

			meta := map[string]any{
				"workerId": summary.WorkerID,
				"jobType":  summary.JobType,
			}
			if summary.BatchID != "" {
				meta["batchId"] = summary.BatchID
			}

			data := map[string]any{
				"workerId":     summary.WorkerID,
				"jobType":      summary.JobType,
				"handledCount": summary.HandledCount,
				"failedCount":  summary.FailedCount,
				"idlePolls":    summary.IdlePolls,
				"once":         summary.Once,
			}
			if summary.BatchID != "" {
				data["batchId"] = summary.BatchID
			}
			if summary.LastJob != nil {
				data["lastJob"] = summary.LastJob
			}
			if summary.LastJobDir != "" {
				data["lastJobDir"] = summary.LastJobDir
			}
			return writeData(cmd, app, meta, data)
		},
	}

	cmd.Flags().StringVar(&jobType, "type", "", "Job type to claim")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "Logical worker id (default: hostname-pid)")
	cmd.Flags().StringVar(&batchID, "batch-id", "", "Optional batch id to constrain claims")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "Worker label assignment in key=value form (repeatable)")
	cmd.Flags().StringVar(&handler, "handler", "", "Handler executable path")
	cmd.Flags().StringArrayVar(&handlerArgs, "handler-arg", nil, "Handler argument (repeatable)")
	cmd.Flags().DurationVar(&leaseDuration, "lease-duration", 2*time.Minute, "Lease duration for claims, e.g. 30s or 2m")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 5*time.Second, "Sleep between empty claim polls")
	cmd.Flags().BoolVar(&once, "once", false, "Claim at most one job, then exit")
	cmd.Flags().BoolVar(&keepJobDirs, "keep-job-dirs", false, "Keep per-job materialization directories after processing")
	return cmd
}

func runJobsWorkerLoop(ctx context.Context, stderr io.Writer, app *App, cfg jobsWorkerConfig) (*jobsWorkerSummary, error) {
	if err := requireAPI(app); err != nil {
		return nil, err
	}

	summary := &jobsWorkerSummary{
		WorkerID: cfg.workerID,
		JobType:  cfg.jobType,
		BatchID:  cfg.batchID,
		Once:     cfg.once,
	}

	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return summary, nil
			default:
			}
		}

		result, err := jobsWorkerClaimAndHandle(ctx, stderr, app, cfg)
		if err != nil {
			return summary, err
		}
		if result != nil {
			summary.HandledCount++
			summary.LastJob = result.Job
			summary.LastJobDir = result.JobDir
			if isJobsWorkerFailureStatus(toString(result.Job["status"])) {
				summary.FailedCount++
			}
			if cfg.once {
				return summary, nil
			}
			continue
		}

		if cfg.once {
			return summary, nil
		}
		summary.IdlePolls++
		if err := sleepWithContext(ctx, cfg.pollInterval); err != nil {
			return summary, nil
		}
	}
}

func jobsWorkerClaimAndHandle(ctx context.Context, stderr io.Writer, app *App, cfg jobsWorkerConfig) (*jobsWorkerExecutionResult, error) {
	claimPayload := map[string]any{
		"jobType":  cfg.jobType,
		"workerId": cfg.workerID,
	}
	if cfg.batchID != "" {
		claimPayload["batchId"] = cfg.batchID
	}
	if len(cfg.workerLabels) > 0 {
		claimPayload["workerLabels"] = cfg.workerLabels
	}
	if cfg.leaseDuration > 0 {
		claimPayload["leaseDuration"] = cfg.leaseDuration.Milliseconds()
	}

	out, err := runSuccessfulJobsCommandWithContext(ctx, app, "jobs.claim", claimPayload)
	if err != nil {
		return nil, err
	}
	job := jobsEnvelopeJob(out)
	if job == nil {
		return nil, nil
	}

	jobID := strings.TrimSpace(toString(job["jobId"]))
	leaseToken := strings.TrimSpace(toString(job["leaseToken"]))
	if jobID == "" || leaseToken == "" {
		return nil, errors.New("claim response missing jobId or leaseToken")
	}

	jobsWorkerLog(stderr, "jobs worker %s claimed %s (%s)", cfg.workerID, jobID, cfg.jobType)
	return jobsWorkerExecuteClaimedJob(ctx, stderr, app, cfg, job)
}

func jobsWorkerExecuteClaimedJob(ctx context.Context, stderr io.Writer, app *App, cfg jobsWorkerConfig, job map[string]any) (*jobsWorkerExecutionResult, error) {
	jobID := strings.TrimSpace(toString(job["jobId"]))
	leaseToken := strings.TrimSpace(toString(job["leaseToken"]))
	jobDir, jobFile, payloadFile, resultFile, contextFile, err := prepareJobsWorkerFiles(job)
	if err != nil {
		return jobsWorkerFailWithPayload(stderr, app, jobID, leaseToken, map[string]any{
			"message": fmt.Sprintf("failed to prepare job files: %v", err),
			"code":    "worker_setup_failed",
			"details": map[string]any{"jobId": jobID},
		}, "")
	}

	cleanupOnSuccess := !cfg.keepJobDirs
	keepDir := cfg.keepJobDirs
	finalize := func(execResult *jobsWorkerExecutionResult, execErr error) (*jobsWorkerExecutionResult, error) {
		if execErr == nil && cleanupOnSuccess {
			_ = os.RemoveAll(jobDir)
			if execResult != nil {
				execResult.JobDir = ""
			}
			return execResult, nil
		}
		if execResult != nil {
			execResult.JobDir = jobDir
		} else if keepDir {
			return &jobsWorkerExecutionResult{JobDir: jobDir}, execErr
		}
		return execResult, execErr
	}

	handlerCmd := exec.CommandContext(ctx, cfg.handler, cfg.handlerArgs...) // #nosec G204 -- jobs worker handlers are explicit local operator configuration. nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	handlerCmd.Stdout = stderr
	handlerCmd.Stderr = stderr
	handlerCmd.Env = jobsWorkerEnv(app, cfg, job, jobDir, jobFile, payloadFile, resultFile, contextFile)

	if err := handlerCmd.Start(); err != nil {
		return finalize(jobsWorkerFailWithPayload(stderr, app, jobID, leaseToken, map[string]any{
			"message": fmt.Sprintf("failed to start handler: %v", err),
			"code":    "handler_start_failed",
			"details": map[string]any{"handler": cfg.handler},
		}, jobDir))
	}

	_, progressErr := runSuccessfulJobsCommandWithContext(ctx, app, "jobs.progress", map[string]any{
		"jobId":      jobID,
		"leaseToken": leaseToken,
		"status":     "started",
		"message":    fmt.Sprintf("started handler %s", filepath.Base(cfg.handler)),
		"details": map[string]any{
			"handler": cfg.handler,
		},
	})
	if progressErr != nil {
		jobsWorkerLog(stderr, "jobs worker %s progress update for %s failed: %v", cfg.workerID, jobID, progressErr)
	}

	stopHeartbeat := startJobsWorkerHeartbeatLoop(ctx, stderr, app, cfg, jobID, leaseToken)
	waitErr := handlerCmd.Wait()
	stopHeartbeat()

	result, resultErr := readJobsWorkerResult(resultFile)
	defaultWorkerInfo := jobsWorkerDefaultInfo(cfg)

	if waitErr == nil {
		if resultErr != nil {
			return finalize(jobsWorkerFailWithPayload(stderr, app, jobID, leaseToken, map[string]any{
				"message": fmt.Sprintf("handler produced invalid result: %v", resultErr),
				"code":    "invalid_result",
				"details": map[string]any{"resultFile": resultFile},
			}, jobDir))
		}
		if jobsWorkerResultRequiresFailure(result) {
			failJob, err := jobsWorkerFailWithPayload(stderr, app, jobID, leaseToken, jobsWorkerFailurePayload(nil, result), jobDir)
			return finalize(failJob, err)
		}
		payload, err := jobsWorkerCompletionPayload(result, defaultWorkerInfo)
		if err != nil {
			return finalize(jobsWorkerFailWithPayload(stderr, app, jobID, leaseToken, map[string]any{
				"message": err.Error(),
				"code":    "invalid_result",
				"details": map[string]any{"resultFile": resultFile},
			}, jobDir))
		}
		completeJob, err := jobsWorkerCompleteWithPayload(stderr, app, jobID, leaseToken, payload, jobDir)
		return finalize(completeJob, err)
	}

	failurePayload := jobsWorkerFailurePayload(waitErr, result)
	failJob, err := jobsWorkerFailWithPayload(stderr, app, jobID, leaseToken, failurePayload, jobDir)
	return finalize(failJob, err)
}

func resolveJobsWorkerHandler(handler string, flagArgs []string, positional []string) (string, []string, error) {
	handler = strings.TrimSpace(handler)
	switch {
	case handler != "":
		return handler, append(append([]string{}, flagArgs...), positional...), nil
	case len(positional) > 0:
		return strings.TrimSpace(positional[0]), append(append([]string{}, positional[1:]...), flagArgs...), nil
	default:
		return "", nil, errors.New("missing handler (use --handler or pass the handler command after `jobs worker run`)")
	}
}

func defaultJobsWorkerID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "worker"
	}
	host = strings.ToLower(strings.TrimSpace(host))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", "@", "-", ".", "-")
	host = replacer.Replace(host)
	host = strings.Trim(host, "-")
	if host == "" {
		host = "worker"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func jobsWorkerEnv(app *App, cfg jobsWorkerConfig, job map[string]any, jobDir string, jobFile string, payloadFile string, resultFile string, contextFile string) []string {
	env := append([]string{}, os.Environ()...)
	setEnv := func(key, value string) {
		if strings.TrimSpace(key) == "" {
			return
		}
		env = append(env, key+"="+value)
	}
	prependPath := func(dir string) {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			return
		}
		current := os.Getenv("PATH")
		if current == "" {
			env = append(env, "PATH="+trimmed)
			return
		}
		env = append(env, "PATH="+trimmed+string(os.PathListSeparator)+current)
	}

	setEnv("BREYTA_WORKER_ID", cfg.workerID)
	setEnv("BREYTA_JOB_DIR", jobDir)
	setEnv("BREYTA_JOB_FILE", jobFile)
	setEnv("BREYTA_JOB_PAYLOAD_FILE", payloadFile)
	setEnv("BREYTA_JOB_RESULT_FILE", resultFile)
	setEnv("BREYTA_JOB_CONTEXT_FILE", contextFile)
	setEnv("BREYTA_JOB_ID", toString(job["jobId"]))
	setEnv("BREYTA_JOB_TYPE", toString(job["jobType"]))
	setEnv("BREYTA_JOB_BATCH_ID", toString(job["batchId"]))
	setEnv("BREYTA_JOB_ATTEMPT", toString(job["attempt"]))
	setEnv("BREYTA_JOB_WORKSPACE_ID", toString(job["workspaceId"]))
	setEnv("BREYTA_JOB_ROOT_WORKFLOW_ID", toString(job["rootWorkflowId"]))
	setEnv("BREYTA_JOB_PARENT_STEP_ID", toString(job["parentStepId"]))
	setEnv("BREYTA_JOB_FANOUT_PARENT_STEP_ID", toString(job["fanoutParentStepId"]))
	setEnv("BREYTA_JOB_FANOUT_MAX_CONCURRENCY", toString(job["fanoutMaxConcurrency"]))
	if exe, err := os.Executable(); err == nil {
		exe = strings.TrimSpace(exe)
		if exe != "" {
			setEnv("BREYTA_CLI_BIN", exe)
			prependPath(filepath.Dir(exe))
		}
	}

	if strings.TrimSpace(app.APIURL) != "" {
		setEnv("BREYTA_API_URL", strings.TrimSpace(app.APIURL))
	}
	if strings.TrimSpace(app.WorkspaceID) != "" {
		setEnv("BREYTA_WORKSPACE", strings.TrimSpace(app.WorkspaceID))
	}
	if app.APIKeyExplicit && strings.TrimSpace(app.APIKey) != "" {
		setEnv("BREYTA_API_KEY", strings.TrimSpace(app.APIKey))
	}
	if strings.TrimSpace(app.Token) != "" {
		setEnv("BREYTA_TOKEN", strings.TrimSpace(app.Token))
	}
	return env
}

func jobsWorkerDefaultInfo(cfg jobsWorkerConfig) map[string]any {
	info := map[string]any{
		"workerId": cfg.workerID,
		"handler":  cfg.handler,
		"pid":      os.Getpid(),
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		info["host"] = strings.TrimSpace(host)
	}
	return info
}

func jobsWorkerCompletionPayload(result *jobsWorkerResult, defaultWorkerInfo map[string]any) (map[string]any, error) {
	status := "succeeded"
	if result != nil && strings.TrimSpace(result.Status) != "" {
		normalized, err := normalizeCLIJobStatus(result.Status, completableJobStatuses, "status")
		if err != nil {
			return nil, err
		}
		status = normalized
	}

	payload := map[string]any{
		"status": status,
	}
	if result != nil {
		if trimmed := strings.TrimSpace(result.Summary); trimmed != "" {
			payload["summary"] = trimmed
		}
		if result.Outputs != nil {
			payload["outputs"] = result.Outputs
		}
		if result.Metrics != nil {
			payload["metrics"] = result.Metrics
		}
		if artifacts := jobsWorkerCompletionArtifacts(result.Artifacts); len(artifacts) > 0 {
			payload["artifacts"] = artifacts
		}
		if result.WorkerInfo != nil {
			payload["workerInfo"] = mergeJSONObjectFlags(defaultWorkerInfo, result.WorkerInfo)
			return payload, nil
		}
	}
	payload["workerInfo"] = defaultWorkerInfo
	if _, ok := payload["summary"]; !ok {
		payload["summary"] = "handler exited successfully"
	}
	return payload, nil
}

func jobsWorkerFailurePayload(waitErr error, result *jobsWorkerResult) map[string]any {
	code := "handler_failed"
	message := "handler failed"
	details := map[string]any{}
	var artifacts []any

	if waitErr != nil {
		message = waitErr.Error()
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			if exitCode := exitErr.ExitCode(); exitCode >= 0 {
				message = fmt.Sprintf("handler exited with status %d", exitCode)
				details["exitCode"] = exitCode
			}
		}
	}

	if result != nil {
		if trimmed := strings.TrimSpace(result.Message); trimmed != "" {
			message = trimmed
		}
		if trimmed := strings.TrimSpace(result.Code); trimmed != "" {
			code = trimmed
		}
		for key, value := range result.Details {
			details[key] = value
		}
		if filtered := jobsWorkerCompletionArtifacts(result.Artifacts); len(filtered) > 0 {
			artifacts = filtered
		}
	}

	payload := map[string]any{
		"message": message,
		"code":    code,
	}
	if len(details) > 0 {
		payload["details"] = details
	}
	if len(artifacts) > 0 {
		payload["artifacts"] = artifacts
	}
	return payload
}

func jobsWorkerCompletionArtifacts(artifacts []any) []any {
	if len(artifacts) == 0 {
		return nil
	}
	filtered := make([]any, 0, len(artifacts))
	for _, raw := range artifacts {
		artifact, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		if persisted, _ := artifact[jobsWorkerPersistedArtifactMarker].(bool); persisted {
			continue
		}
		if _, found := artifact[jobsWorkerPersistedArtifactMarker]; found {
			copyArtifact := make(map[string]any, len(artifact))
			for key, value := range artifact {
				if key == jobsWorkerPersistedArtifactMarker {
					continue
				}
				copyArtifact[key] = value
			}
			filtered = append(filtered, copyArtifact)
			continue
		}
		filtered = append(filtered, artifact)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func jobsWorkerResultRequiresFailure(result *jobsWorkerResult) bool {
	if result == nil {
		return false
	}
	switch strings.TrimSpace(result.Status) {
	case "failed", "timed_out":
		return true
	default:
		return false
	}
}

func jobsWorkerCompleteWithPayload(stderr io.Writer, app *App, jobID string, leaseToken string, payload map[string]any, jobDir string) (*jobsWorkerExecutionResult, error) {
	body := mergeJSONObjectFlags(payload, map[string]any{
		"jobId":      jobID,
		"leaseToken": leaseToken,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := runSuccessfulJobsCommandWithContext(ctx, app, "jobs.complete", body)
	if err != nil {
		return nil, err
	}
	job := jobsEnvelopeJob(out)
	jobsWorkerLog(stderr, "jobs worker completed %s with %s", jobID, toString(job["status"]))
	return &jobsWorkerExecutionResult{Job: job, JobDir: jobDir}, nil
}

func jobsWorkerFailWithPayload(stderr io.Writer, app *App, jobID string, leaseToken string, payload map[string]any, jobDir string) (*jobsWorkerExecutionResult, error) {
	body := mergeJSONObjectFlags(payload, map[string]any{
		"jobId":      jobID,
		"leaseToken": leaseToken,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := runSuccessfulJobsCommandWithContext(ctx, app, "jobs.fail", body)
	if err != nil {
		return nil, err
	}
	job := jobsEnvelopeJob(out)
	jobsWorkerLog(stderr, "jobs worker failed %s with %s", jobID, toString(job["status"]))
	return &jobsWorkerExecutionResult{Job: job, JobDir: jobDir}, nil
}

func runSuccessfulJobsCommandWithContext(ctx context.Context, app *App, command string, args map[string]any) (map[string]any, error) {
	out, status, err := runAPICommandWithContext(ctx, app, command, args)
	if err != nil {
		return nil, err
	}
	if status >= 400 || !isOK(out) {
		return out, fmt.Errorf("api error (status=%d): %s", status, formatAPIError(out))
	}
	return out, nil
}

func jobsEnvelopeJob(out map[string]any) map[string]any {
	if out == nil {
		return nil
	}
	data, _ := out["data"].(map[string]any)
	job, _ := data["job"].(map[string]any)
	return job
}

func prepareJobsWorkerFiles(job map[string]any) (string, string, string, string, string, error) {
	jobID := strings.TrimSpace(toString(job["jobId"]))
	prefix := "breyta-job-"
	if jobID != "" {
		prefix = prefix + sanitizeJobsWorkerPathToken(jobID) + "-"
	}
	jobDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", "", "", "", "", err
	}

	jobFile := filepath.Join(jobDir, "job.json")
	payloadFile := filepath.Join(jobDir, "payload.json")
	resultFile := filepath.Join(jobDir, "result.json")
	contextFile := filepath.Join(jobDir, "worker-context.json")

	if err := writeJobsWorkerJSONFile(jobFile, job); err != nil {
		return "", "", "", "", "", err
	}
	payload := any(map[string]any{})
	if rawPayload, ok := job["payload"]; ok {
		payload = rawPayload
	}
	if err := writeJobsWorkerJSONFile(payloadFile, payload); err != nil {
		return "", "", "", "", "", err
	}
	if err := writeJobsWorkerJSONFile(contextFile, map[string]any{
		"jobId":      job["jobId"],
		"leaseToken": job["leaseToken"],
		"resultFile": resultFile,
	}); err != nil {
		return "", "", "", "", "", err
	}
	return jobDir, jobFile, payloadFile, resultFile, contextFile, nil
}

func writeJobsWorkerJSONFile(path string, value any) error {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	return os.WriteFile(path, bytes, 0600)
}

func readJobsWorkerResult(path string) (*jobsWorkerResult, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read result file: %w", err)
	}
	if strings.TrimSpace(string(bytes)) == "" {
		return nil, nil
	}
	var raw any
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return nil, fmt.Errorf("invalid result json: %w", err)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("result file must contain a JSON object")
	}
	result := &jobsWorkerResult{}
	if status, ok := m["status"].(string); ok {
		result.Status = strings.TrimSpace(status)
	}
	if summary, ok := m["summary"].(string); ok {
		result.Summary = strings.TrimSpace(summary)
	}
	if outputs, ok := m["outputs"]; ok {
		result.Outputs = outputs
	}
	if metrics, ok := m["metrics"].(map[string]any); ok {
		result.Metrics = metrics
	}
	if message, ok := m["message"].(string); ok {
		result.Message = strings.TrimSpace(message)
	}
	if details, ok := m["details"].(map[string]any); ok {
		result.Details = details
	}
	if code, ok := m["code"].(string); ok {
		result.Code = strings.TrimSpace(code)
	}
	if workerInfo, ok := m["workerInfo"].(map[string]any); ok {
		result.WorkerInfo = workerInfo
	}
	switch typed := m["artifacts"].(type) {
	case nil:
	case []any:
		result.Artifacts = typed
	default:
		result.Artifacts = []any{typed}
	}
	return result, nil
}

func startJobsWorkerHeartbeatLoop(ctx context.Context, stderr io.Writer, app *App, cfg jobsWorkerConfig, jobID string, leaseToken string) func() {
	interval := jobsWorkerHeartbeatInterval(cfg.leaseDuration)
	if interval <= 0 {
		return func() {}
	}

	heartbeatCtx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := runSuccessfulJobsCommandWithContext(heartbeatCtx, app, "jobs.heartbeat", map[string]any{
					"jobId":         jobID,
					"leaseToken":    leaseToken,
					"leaseDuration": cfg.leaseDuration.Milliseconds(),
				})
				if err != nil {
					jobsWorkerLog(stderr, "jobs worker heartbeat for %s failed: %v", jobID, err)
				}
			}
		}
	}()
	return cancel
}

func jobsWorkerHeartbeatInterval(leaseDuration time.Duration) time.Duration {
	if leaseDuration <= 0 {
		return 0
	}
	interval := leaseDuration / 2
	if interval <= 0 || interval >= leaseDuration {
		return leaseDuration - time.Nanosecond
	}
	return interval
}

func sanitizeJobsWorkerPathToken(raw string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "@", "-", ".", "-")
	trimmed := strings.Trim(replacer.Replace(strings.TrimSpace(raw)), "-")
	if trimmed == "" {
		return "job"
	}
	return trimmed
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func jobsWorkerLog(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

func isJobsWorkerFailureStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "failed", "timed_out", "cancelled":
		return true
	default:
		return false
	}
}
