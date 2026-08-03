package session

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jingkaihe/thirdspace/internal/config"
)

func TestDefaultRefreshWindowIsThreeDays(t *testing.T) {
	if defaultRefreshBefore != 72*time.Hour {
		t.Fatalf("default refresh window = %s", defaultRefreshBefore)
	}
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	usable, err := tokenUsable(config.Token{
		Value:     "token",
		ExpiresAt: now.Add(72*time.Hour + time.Second).Format(time.RFC3339),
	}, now, defaultRefreshBefore)
	if err != nil || !usable {
		t.Fatalf("token just outside refresh window = %v, %v", usable, err)
	}
	usable, err = tokenUsable(config.Token{
		Value:     "token",
		ExpiresAt: now.Add(48 * time.Hour).Format(time.RFC3339),
	}, now, defaultRefreshBefore)
	if err != nil || usable {
		t.Fatalf("token inside refresh window = %v, %v", usable, err)
	}
}

func TestLoginPersistsSessionAndRefreshWithoutPassword(t *testing.T) {
	clearSessionEnvironment(t)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v2/member/auth/login":
			_, _ = io.WriteString(response, `{"token":"session-token","expiresAt":"2026-09-02T12:00:00","email":"member@example.com"}`)
		case "/api/v2/member/auth/token/refresh":
			if token := request.Header.Get("x-fisikal-token"); token != "session-token" {
				t.Errorf("refresh session token = %q", token)
			}
			_, _ = io.WriteString(response, `{"expiresAt":"2026-09-02T12:00:00","response":{"token":"refresh-token"}}`)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config.json")
	store, err := config.Open(path, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewManager(store).Login(context.Background(), "member@example.com", "login-password")
	if err != nil {
		t.Fatal(err)
	}
	if result.RefreshWarning != nil || result.Refresh.Token != "refresh-token" {
		t.Fatalf("unexpected login result: %#v", result)
	}
	state := store.State()
	if state.Session.Value != "session-token" || state.Refresh.Value != "refresh-token" {
		t.Fatalf("unexpected stored state: %#v", state)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "login-password") || strings.Contains(string(data), `"password"`) {
		t.Fatal("password was persisted")
	}
}

func TestClientProactivelyExchangesAndRotatesTokens(t *testing.T) {
	clearSessionEnvironment(t)
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	newSessionExpiry := now.Add(30 * 24 * time.Hour).Format(time.RFC3339)
	newRefreshExpiry := now.Add(31 * 24 * time.Hour).Format(time.RFC3339)
	var exchanges atomic.Int32
	var rotations atomic.Int32
	var clubCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v2/member/auth/token":
			exchanges.Add(1)
			if token := request.Header.Get("x-fisikal-token"); token != "old-refresh" {
				t.Errorf("exchange refresh token = %q", token)
			}
			_, _ = io.WriteString(response, `{"expiresAt":"`+newSessionExpiry+`","response":{"token":"new-session","email":"member@example.com"}}`)
		case "/api/v2/member/auth/token/refresh":
			rotations.Add(1)
			if token := request.Header.Get("x-fisikal-token"); token != "new-session" {
				t.Errorf("rotation session token = %q", token)
			}
			_, _ = io.WriteString(response, `{"expiresAt":"`+newRefreshExpiry+`","response":{"token":"new-refresh"}}`)
		case "/api/v2/member/clubs/get_clubs":
			clubCalls.Add(1)
			if token := request.Header.Get("x-fisikal-token"); token != "new-session" {
				t.Errorf("clubs session token = %q", token)
			}
			_, _ = io.WriteString(response, `{"clubs":[]}`)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(config.File{
		BaseURL:               server.URL,
		Email:                 "member@example.com",
		Token:                 "old-session",
		ExpiresAt:             now.Add(48 * time.Hour).Format(time.RFC3339),
		RefreshToken:          "old-refresh",
		RefreshTokenExpiresAt: now.Add(10 * 24 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store)
	manager.now = func() time.Time { return now }
	client, err := manager.Client(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clubs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if exchanges.Load() != 1 || rotations.Load() != 1 || clubCalls.Load() != 1 {
		t.Fatalf("calls: exchanges=%d rotations=%d clubs=%d", exchanges.Load(), rotations.Load(), clubCalls.Load())
	}
	state := store.State()
	if state.Session.Value != "new-session" || state.Session.ExpiresAt != newSessionExpiry {
		t.Fatalf("session was not updated: %#v", state.Session)
	}
	if state.Refresh.Value != "new-refresh" || state.Refresh.ExpiresAt != newRefreshExpiry {
		t.Fatalf("refresh token was not rotated: %#v", state.Refresh)
	}
}

func TestClientFallsBackWhenProactiveRefreshFails(t *testing.T) {
	clearSessionEnvironment(t)
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	var exchangeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/member/auth/token":
			exchangeCalls.Add(1)
			http.Error(response, `{"message":"temporarily unavailable"}`, http.StatusServiceUnavailable)
		case "/api/v2/member/clubs/get_clubs":
			if token := request.Header.Get("x-fisikal-token"); token != "current-session" {
				t.Errorf("fallback session token = %q", token)
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"clubs":[]}`)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(config.File{
		BaseURL:               server.URL,
		Token:                 "current-session",
		ExpiresAt:             now.Add(48 * time.Hour).Format(time.RFC3339),
		RefreshToken:          "current-refresh",
		RefreshTokenExpiresAt: now.Add(10 * 24 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store)
	manager.now = func() time.Time { return now }
	client, err := manager.Client(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clubs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if exchangeCalls.Load() != 1 {
		t.Fatalf("exchange calls = %d", exchangeCalls.Load())
	}
}

func TestClientKeepsNewSessionWhenRefreshRotationFails(t *testing.T) {
	clearSessionEnvironment(t)
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	newSessionExpiry := now.Add(30 * 24 * time.Hour).Format(time.RFC3339)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/member/auth/token":
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"expiresAt":"`+newSessionExpiry+`","response":{"token":"new-session"}}`)
		case "/api/v2/member/auth/token/refresh":
			http.Error(response, `{"message":"temporarily unavailable"}`, http.StatusServiceUnavailable)
		case "/api/v2/member/clubs/get_clubs":
			if token := request.Header.Get("x-fisikal-token"); token != "new-session" {
				t.Errorf("clubs session token = %q", token)
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"clubs":[]}`)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(config.File{
		BaseURL:               server.URL,
		Token:                 "expired-session",
		ExpiresAt:             now.Add(-time.Hour).Format(time.RFC3339),
		RefreshToken:          "current-refresh",
		RefreshTokenExpiresAt: now.Add(10 * 24 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store)
	manager.now = func() time.Time { return now }
	client, err := manager.Client(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clubs(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := store.State()
	if state.Session.Value != "new-session" || state.Session.ExpiresAt != newSessionExpiry {
		t.Fatalf("new session was not retained: %#v", state.Session)
	}
	if state.Refresh.Value != "" {
		t.Fatalf("failed refresh token was retained: %#v", state.Refresh)
	}
}

func TestClientRejectsBaseURLOverrideForStoredTokens(t *testing.T) {
	clearSessionEnvironment(t)
	storedServer := httptest.NewServer(http.NotFoundHandler())
	defer storedServer.Close()
	overrideServer := httptest.NewServer(http.NotFoundHandler())
	defer overrideServer.Close()

	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"), storedServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(config.File{
		BaseURL:   storedServer.URL,
		Token:     "stored-session",
		ExpiresAt: "2026-09-02T12:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THIRDSPACE_BASE_URL", overrideServer.URL)
	_, err = NewManager(store).Client(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing to persist refreshed credentials") {
		t.Fatalf("expected base-URL mismatch error, got %v", err)
	}
}

func clearSessionEnvironment(t *testing.T) {
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
