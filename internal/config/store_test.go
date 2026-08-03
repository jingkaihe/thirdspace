package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreSavesPrivateConfigAndReloadsState(t *testing.T) {
	clearEnvironment(t)
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store, err := Open(path, "https://api.thirdspace-london.app")
	if err != nil {
		t.Fatal(err)
	}
	want := File{
		BaseURL:               "https://api.thirdspace-london.app",
		Email:                 "member@example.com",
		Token:                 "session",
		ExpiresAt:             "2026-09-02T12:00:00",
		RefreshToken:          "refresh",
		RefreshTokenExpiresAt: "2026-09-02T12:00:00",
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissions = %o, want 600", got)
	}
	state := store.State()
	if state.Session.Value != want.Token || state.Refresh.Value != want.RefreshToken || state.Email != want.Email {
		t.Fatalf("state was not reloaded after save: %#v", state)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if state := store.State(); state.Session.Value != "" || state.Refresh.Value != "" {
		t.Fatalf("state was not cleared: %#v", state)
	}
}

func TestOpenRejectsLoosePermissions(t *testing.T) {
	clearEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"token":"secret"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(path, "https://api.thirdspace-london.app")
	if err == nil || !strings.Contains(err.Error(), "permissions 644") {
		t.Fatalf("expected loose-permission error, got %v", err)
	}
}

func TestPasswordIsReadOnlyFromEnvironment(t *testing.T) {
	clearEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"email":"member@example.com","password":"must-not-load"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, "https://api.thirdspace-london.app")
	if err != nil {
		t.Fatal(err)
	}
	if password := store.State().Password; password != "" {
		t.Fatalf("password was loaded from config: %q", password)
	}
	t.Setenv("THIRDSPACE_PASSWORD", "environment-secret")
	if password := store.State().Password; password != "environment-secret" {
		t.Fatalf("environment password = %q", password)
	}
}

func TestEnvironmentTokenDoesNotInheritStoredExpiry(t *testing.T) {
	clearEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"token":"stored","expiresAt":"2026-09-02T12:00:00"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, "https://api.thirdspace-london.app")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("THIRDSPACE_TOKEN", "environment")
	state := store.State()
	if state.Session.Value != "environment" || state.Session.ExpiresAt != "" || !state.SessionFromEnvironment {
		t.Fatalf("unexpected environment state: %#v", state)
	}
}

func clearEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"THIRDSPACE_BASE_URL",
		"THIRDSPACE_EMAIL",
		"THIRDSPACE_PASSWORD",
		"THIRDSPACE_TOKEN",
	} {
		t.Setenv(name, "")
	}
}
