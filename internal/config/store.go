package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type File struct {
	BaseURL               string `json:"baseUrl,omitempty"`
	Email                 string `json:"email,omitempty"`
	Token                 string `json:"token,omitempty"`
	ExpiresAt             string `json:"expiresAt,omitempty"`
	RefreshToken          string `json:"refreshToken,omitempty"`
	RefreshTokenExpiresAt string `json:"refreshTokenExpiresAt,omitempty"`
}

type Token struct {
	Value     string
	ExpiresAt string
}

type State struct {
	BaseURL                string
	Email                  string
	Password               string
	Session                Token
	Refresh                Token
	SessionFromEnvironment bool
	EmailFromEnvironment   bool
}

type Store struct {
	path           string
	defaultBaseURL string
	viper          *viper.Viper
	stored         File
}

func DefaultPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(root, "thirdspace", "config.json"), nil
}

func ResolvePath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if path := os.Getenv("THIRDSPACE_CONFIG"); path != "" {
		return path, nil
	}
	return DefaultPath()
}

func Open(path, defaultBaseURL string) (*Store, error) {
	v := newViper(path, defaultBaseURL)

	var stored File
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("inspect config %s: %w", path, err)
	default:
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("refusing non-regular config file %s", path)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("refusing config %s with permissions %o; run chmod 600 %q", path, info.Mode().Perm(), path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := json.Unmarshal(data, &stored); err != nil {
			return nil, fmt.Errorf("decode config %s: %w", path, err)
		}
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("load config %s: %w", path, err)
		}
	}

	return &Store{path: path, defaultBaseURL: defaultBaseURL, viper: v, stored: stored}, nil
}

func newViper(path, defaultBaseURL string) *viper.Viper {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("json")
	v.SetDefault("baseUrl", defaultBaseURL)
	bindEnvironment(v)
	return v
}

func bindEnvironment(v *viper.Viper) {
	bindings := map[string]string{
		"baseUrl": "THIRDSPACE_BASE_URL",
		"email":   "THIRDSPACE_EMAIL",
		"token":   "THIRDSPACE_TOKEN",
	}
	for key, environment := range bindings {
		_ = v.BindEnv(key, environment)
	}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Stored() File {
	return s.stored
}

func (s *Store) State() State {
	sessionFromEnvironment := os.Getenv("THIRDSPACE_TOKEN") != ""
	emailFromEnvironment := os.Getenv("THIRDSPACE_EMAIL") != ""
	sessionExpiry := s.viper.GetString("expiresAt")
	if sessionFromEnvironment {
		sessionExpiry = ""
	}
	return State{
		BaseURL:                s.viper.GetString("baseUrl"),
		Email:                  s.viper.GetString("email"),
		Password:               os.Getenv("THIRDSPACE_PASSWORD"),
		Session:                Token{Value: s.viper.GetString("token"), ExpiresAt: sessionExpiry},
		Refresh:                Token{Value: s.viper.GetString("refreshToken"), ExpiresAt: s.viper.GetString("refreshTokenExpiresAt")},
		SessionFromEnvironment: sessionFromEnvironment,
		EmailFromEnvironment:   emailFromEnvironment,
	}
}

func (s *Store) Save(file File) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary config: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("secure config: %w", err)
	}
	s.stored = file
	s.viper = newViper(s.path, s.defaultBaseURL)
	if err := s.viper.ReadInConfig(); err != nil {
		return fmt.Errorf("reload config %s: %w", s.path, err)
	}
	return nil
}

func (s *Store) Clear() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove config: %w", err)
	}
	s.stored = File{}
	s.viper = newViper(s.path, s.defaultBaseURL)
	return nil
}
