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
	DefaultProdAPIURL  = "https://flows.breyta.ai"
	DefaultLocalAPIURL = "http://localhost:8090"
)

type Store struct {
	APIURL         string                `json:"apiUrl,omitempty"`
	WorkspaceID    string                `json:"workspaceId,omitempty"`
	RunConfigID    string                `json:"runConfigId,omitempty"`
	DevMode        bool                  `json:"devMode,omitempty"`
	DevAPIURL      string                `json:"devApiUrl,omitempty"`
	DevWorkspaceID string                `json:"devWorkspaceId,omitempty"`
	DevToken       string                `json:"devToken,omitempty"`
	DevRunConfigID string                `json:"devRunConfigId,omitempty"`
	DevActive      string                `json:"devActive,omitempty"`
	DevProfiles    map[string]DevProfile `json:"devProfiles,omitempty"`
}

type DevProfile struct {
	APIURL        string `json:"apiUrl,omitempty"`
	WorkspaceID   string `json:"workspaceId,omitempty"`
	Token         string `json:"token,omitempty"`
	RunConfigID   string `json:"runConfigId,omitempty"`
	AuthStorePath string `json:"authStorePath,omitempty"`
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
	b, err := os.ReadFile(path)
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
	st.DevAPIURL = strings.TrimSpace(st.DevAPIURL)
	st.DevWorkspaceID = strings.TrimSpace(st.DevWorkspaceID)
	st.DevToken = strings.TrimSpace(st.DevToken)
	st.DevRunConfigID = strings.TrimSpace(st.DevRunConfigID)
	st.DevActive = strings.TrimSpace(st.DevActive)
	if st.DevProfiles == nil {
		st.DevProfiles = map[string]DevProfile{}
	}
	// Migrate legacy dev fields into a default profile if profiles are empty.
	if len(st.DevProfiles) == 0 && (st.DevAPIURL != "" || st.DevWorkspaceID != "" || st.DevToken != "" || st.DevRunConfigID != "") {
		st.DevProfiles["local"] = DevProfile{
			APIURL:      st.DevAPIURL,
			WorkspaceID: st.DevWorkspaceID,
			Token:       st.DevToken,
			RunConfigID: st.DevRunConfigID,
		}
		if st.DevActive == "" {
			st.DevActive = "local"
		}
	}
	for name, prof := range st.DevProfiles {
		prof.APIURL = strings.TrimSpace(prof.APIURL)
		prof.WorkspaceID = strings.TrimSpace(prof.WorkspaceID)
		prof.Token = strings.TrimSpace(prof.Token)
		prof.RunConfigID = strings.TrimSpace(prof.RunConfigID)
		prof.AuthStorePath = strings.TrimSpace(prof.AuthStorePath)
		st.DevProfiles[name] = prof
	}
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
	return os.Rename(tmp, path)
}
