package configstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Default values used when no config file exists.
const (
	DefaultAPIURL = "http://localhost:8090"
	// DefaultLocalAPIURL remains as an internal alias while command code is
	// migrated to the canonical self-hosted default.
	DefaultLocalAPIURL = DefaultAPIURL
)

type Store struct {
	APIURL      string `json:"apiUrl,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	RunConfigID string `json:"runConfigId,omitempty"`
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("cannot determine user config dir")
	}
	return filepath.Join(dir, "breyta", "config.json"), nil
}

func Load(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("missing path")
	}
	b, err := os.ReadFile(path) // #nosec G304 -- config store path is resolved from the user config directory or explicit operator configuration.
	if err != nil {
		return nil, err
	}
	var st Store
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	st.APIURL = strings.TrimSpace(st.APIURL)
	st.WorkspaceID = strings.TrimSpace(st.WorkspaceID)
	st.RunConfigID = strings.TrimSpace(st.RunConfigID)
	return &st, nil
}

func SaveAtomic(path string, st *Store) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("missing path")
	}
	if st == nil {
		return errors.New("missing store")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil { // #nosec G302 -- config store directory must be owner-only searchable.
		return err
	}
	payload, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
