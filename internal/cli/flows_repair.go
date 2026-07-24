package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/breyta/breyta-cli/internal/clojure/parinfer"
	"github.com/breyta/breyta-cli/internal/tools"
	"github.com/spf13/cobra"
)

func newFlowsParenRepairCmd(app *App) *cobra.Command {
	var write bool
	var verbose bool
	var files []string

	cmd := &cobra.Command{
		Use:   "paren-repair [files...]",
		Short: "Repair unbalanced Clojure delimiters in flow files (local)",
		Long: strings.TrimSpace(`
Repair unbalanced delimiters in one or more local .clj flow files.

This is intended as an escape hatch when LLM edits introduce delimiter errors.
By default this command is a dry run; pass --write to update files in place.
`),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			allFiles := append([]string{}, files...)
			allFiles = append(allFiles, args...)
			if len(allFiles) == 0 {
				return writeErr(cmd, errors.New("missing file path (use --file <path> or pass a positional file)"))
			}
			results := make([]map[string]any, 0, len(allFiles))
			changedAny := false

			parinferPath := tools.FindParinferRust()
			parinferRunner := parinfer.Runner{BinaryPath: parinferPath}

			for _, path := range allFiles {
				b, err := readExplicitFile(path)
				if err != nil {
					return writeFailure(cmd, app, "read_failed", err, "Check the path and permissions.", map[string]any{"path": path})
				}

				orig := string(b)
				engine := "fallback"
				repaired := orig
				var report any

				if err := parenrepair.Check(orig); err == nil {
					engine = "none"
					report = map[string]any{"balanced": true, "skipped": true}
				} else if !errors.Is(err, parenrepair.ErrUnbalancedDelimiters) {
					return writeFailure(cmd, app, "clojure_paren_repair_failed", err, "Fix the underlying syntax issue (e.g. unterminated string), then retry.", map[string]any{"path": path})
				} else {
					if parinferPath != "" {
						engine = "parinfer-rust"
						if out, ans, err := parinferRunner.RepairIndent(orig); err == nil {
							repaired = out
							report = ans
						} else {
							engine = "fallback"
						}
					}

					if engine == "fallback" {
						out, rep, err := parenrepair.Repair(orig, verbose)
						if err != nil {
							return writeFailure(cmd, app, "clojure_paren_repair_failed", err, "Fix the underlying syntax issue (e.g. unterminated string), then retry.", map[string]any{"path": path, "report": rep})
						}
						repaired = out
						report = rep
					}
				}

				changed := repaired != orig
				if changed {
					changedAny = true
				}

				if write && changed {
					if err := atomicWriteFile(path, []byte(repaired), 0o644); err != nil {
						return writeFailure(cmd, app, "write_failed", err, "Check the path and permissions.", map[string]any{"path": path})
					}
				}

				r := map[string]any{
					"path":    path,
					"changed": changed,
					"written": write && changed,
					"engine":  engine,
					"report":  report,
				}
				results = append(results, r)
			}

			return writeData(cmd, app, nil, map[string]any{
				"changed": changedAny,
				"results": results,
			})
		},
	}

	cmd.Flags().StringArrayVar(&files, "file", nil, "Path to local .clj flow source; repeat for multiple files")
	cmd.Flags().BoolVar(&write, "write", false, "Write repaired changes to files in place")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Include per-fix details in output (fallback engine only)")
	return cmd
}

func newFlowsParenCheckCmd(app *App) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "paren-check [file]",
		Short: "Check that a flow file has balanced delimiters (local)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := strings.TrimSpace(file)
			if len(args) > 0 {
				if path != "" && path != args[0] {
					return writeErr(cmd, errors.New("provide either --file or a positional file, not both"))
				}
				path = args[0]
			}
			if path == "" {
				return writeErr(cmd, errors.New("missing file path (use --file <path> or pass a positional file)"))
			}
			b, err := readExplicitFile(path)
			if err != nil {
				return writeFailure(cmd, app, "read_failed", err, "Check the path and permissions.", map[string]any{"path": path})
			}
			if err := parenrepair.Check(string(b)); err != nil {
				return writeFailure(cmd, app, "clojure_delimiters_invalid", err, "Run: breyta flows paren-repair --write --file "+path, map[string]any{"path": path})
			}
			return writeData(cmd, app, nil, map[string]any{"path": path, "ok": true})
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Path to local .clj flow source")
	return cmd
}

func atomicWriteFile(path string, data []byte, defaultPerm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := makePublicDir(dir); err != nil {
		return err
	}

	perm := defaultPerm
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".breyta.tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}
