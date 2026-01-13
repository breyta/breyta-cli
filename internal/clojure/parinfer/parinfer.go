package parinfer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Answer struct {
	Text    string  `json:"text"`
	Success bool    `json:"success"`
	Error   *ErrObj `json:"error"`
}

type ErrObj struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	LineNo  int    `json:"lineNo"`
	X       int    `json:"x"`
}

var ErrNotAvailable = errors.New("parinfer-rust not available")

type Runner struct {
	BinaryPath string
	Timeout    time.Duration
}

func (r Runner) RepairIndent(text string) (string, Answer, error) {
	if strings.TrimSpace(r.BinaryPath) == "" {
		return text, Answer{}, ErrNotAvailable
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.BinaryPath,
		"--output-format", "json",
		"-l", "clojure",
		"-m", "indent",
	)
	cmd.Stdin = strings.NewReader(text)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()

	outStr := strings.TrimSpace(stdout.String())
	if outStr == "" {
		if ctx.Err() != nil {
			return text, Answer{}, fmt.Errorf("parinfer-rust timed out: %w", ctx.Err())
		}
		if strings.TrimSpace(stderr.String()) != "" {
			return text, Answer{}, fmt.Errorf("parinfer-rust failed: %s", strings.TrimSpace(stderr.String()))
		}
		return text, Answer{}, errors.New("parinfer-rust produced no output")
	}

	var ans Answer
	if err := json.Unmarshal([]byte(outStr), &ans); err != nil {
		// parinfer may have emitted text output (or corrupted output); include stderr for debugging.
		msg := "parinfer-rust returned non-JSON output"
		if strings.TrimSpace(stderr.String()) != "" {
			msg = msg + ": " + strings.TrimSpace(stderr.String())
		}
		return text, Answer{}, fmt.Errorf("%s: %w", msg, err)
	}

	if !ans.Success {
		if ans.Error != nil && strings.TrimSpace(ans.Error.Message) != "" {
			return text, ans, fmt.Errorf("parinfer-rust: %s", ans.Error.Message)
		}
		return text, ans, errors.New("parinfer-rust: failed")
	}

	return ans.Text, ans, nil
}
