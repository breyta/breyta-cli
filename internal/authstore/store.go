package authstore

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Record struct {
	Token        string    `json:"token"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Store struct {
	Tokens map[string]Record `json:"tokens"`
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		h, herr := os.UserHomeDir()
		if herr != nil {
			return "", errors.New("cannot determine config dir")
		}
		dir = filepath.Join(h, ".config")
	}
	return filepath.Join(dir, "breyta", "auth.json"), nil
}

func EnsureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func Load(path string) (*Store, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Tokens == nil {
		s.Tokens = map[string]Record{}
	}
	return &s, nil
}

func SaveAtomic(path string, s *Store) error {
	if err := EnsureParentDir(path); err != nil {
		return err
	}
	if s.Tokens == nil {
		s.Tokens = map[string]Record{}
	}

	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp := path + ".tmp"
	// Auth material can include long-lived refresh tokens; keep permissions strict.
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) GetRecord(baseURL string) (Record, bool) {
	if s == nil || s.Tokens == nil {
		return Record{}, false
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return Record{}, false
	}
	rec, ok := s.Tokens[baseURL]
	if !ok {
		return Record{}, false
	}
	if strings.TrimSpace(rec.Token) == "" {
		return Record{}, false
	}
	rec.Token = strings.TrimSpace(rec.Token)
	rec.RefreshToken = strings.TrimSpace(rec.RefreshToken)
	return rec, true
}

func (s *Store) Get(baseURL string) (string, bool) {
	rec, ok := s.GetRecord(baseURL)
	if !ok {
		return "", false
	}
	return rec.Token, true
}

func (s *Store) Set(baseURL, token string) {
	if s.Tokens == nil {
		s.Tokens = map[string]Record{}
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	token = strings.TrimSpace(token)
	if baseURL == "" || token == "" {
		return
	}
	s.Tokens[baseURL] = Record{Token: token, UpdatedAt: time.Now().UTC()}
}

func (s *Store) SetRecord(baseURL string, rec Record) {
	if s.Tokens == nil {
		s.Tokens = map[string]Record{}
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	rec.Token = strings.TrimSpace(rec.Token)
	rec.RefreshToken = strings.TrimSpace(rec.RefreshToken)
	if baseURL == "" || rec.Token == "" {
		return
	}
	rec.UpdatedAt = time.Now().UTC()
	s.Tokens[baseURL] = rec
}

func (s *Store) Delete(baseURL string) {
	if s == nil || s.Tokens == nil {
		return
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return
	}
	delete(s.Tokens, baseURL)
}
