package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const maxWorkspaceImportBytes = 64 * 1024 * 1024

func newWorkspaceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "workspace", Short: "Export and import a workspace"}
	cmd.AddCommand(newWorkspaceExportCmd(app))
	cmd.AddCommand(newWorkspaceImportCmd(app))
	return cmd
}

func newWorkspaceExportCmd(app *App) *cobra.Command {
	var outputPath string
	var force bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a portable workspace bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(outputPath) == "" {
				return writeErr(cmd, errors.New("--out is required"))
			}
			if !force {
				if _, err := os.Stat(outputPath); err == nil {
					return writeErr(cmd, fmt.Errorf("output file already exists: %s (use --force to replace it)", outputPath))
				} else if !errors.Is(err, os.ErrNotExist) {
					return writeErr(cmd, err)
				}
			}
			if err := requireWorkspaceAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			response, status, err := apiClientWithTimeout(app, 2*time.Minute).DoREST(ctx, http.MethodGet, "/api/workspace/export", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status != http.StatusOK {
				return writeErr(cmd, fmt.Errorf("workspace export failed (status=%d)", status))
			}
			bundle, err := workspaceBundleFromResponse(response)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := writeJSONAtomic(outputPath, bundle); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, map[string]any{"httpStatus": status}, map[string]any{"path": outputPath, "format": "breyta.workspace"})
		},
	}
	cmd.Flags().StringVarP(&outputPath, "out", "o", "", "Destination JSON file")
	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing destination file")
	return cmd
}

func newWorkspaceImportCmd(app *App) *cobra.Command {
	var inputPath, connectionMappingsPath, resourceMappingsPath string
	var allowMerge, reviewedDependencies, enableTriggers bool
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a portable workspace bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(inputPath) == "" {
				return writeErr(cmd, errors.New("--file is required"))
			}
			if err := requireWorkspaceAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			bundle, err := readJSONObject(inputPath, maxWorkspaceImportBytes)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{"bundle": bundle}
			if connectionMappingsPath != "" {
				mappings, err := readJSONObject(connectionMappingsPath, maxWorkspaceImportBytes)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("connection mappings: %w", err))
				}
				payload["connectionMappings"] = mappings
			}
			if resourceMappingsPath != "" {
				mappings, err := readJSONObject(resourceMappingsPath, maxWorkspaceImportBytes)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("resource mappings: %w", err))
				}
				payload["resourceMappings"] = mappings
			}
			payload["allowMerge"] = allowMerge
			payload["reviewedDependencies"] = reviewedDependencies
			payload["enableTriggers"] = enableTriggers

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			response, status, err := apiClientWithTimeout(app, 5*time.Minute).DoREST(ctx, http.MethodPost, "/api/workspace/import", nil, payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status < 200 || status >= 300 {
				return writeFailure(cmd, app, "workspace_import_failed", fmt.Errorf("workspace import failed (status=%d)", status), "Review the import report and dependency mappings", response)
			}
			return writeData(cmd, app, map[string]any{"httpStatus": status}, commandResponseData(response))
		},
	}
	cmd.Flags().StringVarP(&inputPath, "file", "f", "", "Workspace bundle JSON file")
	cmd.Flags().StringVar(&connectionMappingsPath, "connection-mappings", "", "JSON object mapping source connection ids to destination ids")
	cmd.Flags().StringVar(&resourceMappingsPath, "resource-mappings", "", "JSON object mapping source resource URIs to resolutions")
	cmd.Flags().BoolVar(&allowMerge, "allow-merge", false, "Import into a workspace that already contains flows")
	cmd.Flags().BoolVar(&reviewedDependencies, "reviewed-dependencies", false, "Confirm that dynamic dependencies were reviewed")
	cmd.Flags().BoolVar(&enableTriggers, "enable-triggers", false, "Enable imported triggers when all dependencies are resolved")
	return cmd
}

func requireWorkspaceAPI(app *App) error {
	if err := requireAPI(app); err != nil {
		return err
	}
	if strings.TrimSpace(app.WorkspaceID) == "" {
		return errors.New("missing workspace id (--workspace or BREYTA_WORKSPACE)")
	}
	return nil
}

func workspaceBundleFromResponse(response any) (map[string]any, error) {
	root, ok := response.(map[string]any)
	if !ok {
		return nil, errors.New("workspace export returned invalid JSON")
	}
	if data, ok := root["data"].(map[string]any); ok {
		if bundle, ok := data["bundle"].(map[string]any); ok {
			return bundle, nil
		}
	}
	if root["format"] == "breyta.workspace" {
		return root, nil
	}
	return nil, errors.New("workspace export response does not contain a breyta.workspace bundle")
}

func readJSONObject(path string, maxBytes int64) (map[string]any, error) {
	file, err := os.Open(path) // #nosec G304 -- operator explicitly selects the import file.
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("JSON file exceeds %d bytes", maxBytes)
	}
	decoder := json.NewDecoder(file)
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("JSON file contains more than one value")
		}
		return nil, err
	}
	if value == nil {
		return nil, errors.New("expected a JSON object")
	}
	return value, nil
}

func commandResponseData(response any) any {
	if root, ok := response.(map[string]any); ok {
		if data, exists := root["data"]; exists {
			return data
		}
	}
	return response
}

func writeJSONAtomic(path string, value any) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return errors.New("invalid output path")
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".breyta-workspace-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
