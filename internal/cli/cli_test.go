package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jingkaihe/thirdspace/internal/config"
	"github.com/jingkaihe/thirdspace/internal/version"
)

func TestHelpUsesCobraCommandTree(t *testing.T) {
	input := emptyInput(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Execute(context.Background(), []string{"--help"}, input, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"tui", "club", "classes", "bookings", "refresh", "leave-waitlist"} {
		if !strings.Contains(stdout.String(), command) {
			t.Fatalf("help did not contain %q:\n%s", command, stdout.String())
		}
	}
}

func TestVersionFlag(t *testing.T) {
	input := emptyInput(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Execute(context.Background(), []string{"--version"}, input, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), version.Version) || !strings.Contains(stdout.String(), version.GitCommit) {
		t.Fatalf("version output did not contain %q: %s", version.String(), stdout.String())
	}
}

func TestTUIRequiresTerminalInput(t *testing.T) {
	input := emptyInput(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), []string{"tui"}, input, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "requires terminal input") {
		t.Fatalf("expected terminal-input error, got %v", err)
	}
}

func TestPasswordStdinTakesPrecedence(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "password-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	if _, err := input.WriteString("stdin-password\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	application := &app{stdin: input, stderr: &bytes.Buffer{}}
	password, err := application.readPassword(true, "environment-password")
	if err != nil {
		t.Fatal(err)
	}
	if password != "stdin-password" {
		t.Fatalf("password = %q", password)
	}
}

func TestStatusNeverPrintsTokenValues(t *testing.T) {
	clearCLIEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"email":"member@example.com","token":"session-secret","expiresAt":"2026-09-02T12:00:00","refreshToken":"refresh-secret","refreshTokenExpiresAt":"2026-09-02T12:00:00"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	input := emptyInput(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Execute(context.Background(), []string{"--config", path, "status"}, input, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "session-secret") || strings.Contains(stdout.String(), "refresh-secret") {
		t.Fatalf("status exposed a token:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Refresh token: available") {
		t.Fatalf("status did not report refresh availability:\n%s", stdout.String())
	}
}

func TestClubChooseAndClear(t *testing.T) {
	clearCLIEnvironment(t)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v2/member/clubs/get_clubs" {
			t.Errorf("unexpected request path %s", request.URL.Path)
			http.NotFound(response, request)
			return
		}
		if token := request.Header.Get("x-fisikal-token"); token != "session-token" {
			t.Errorf("session token = %q", token)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"clubs":[{"clubId":1,"name":"City"},{"clubId":2,"name":"Soho"}]}`)
	}))
	defer server.Close()

	path := authenticatedConfig(t, server.URL, nil)
	input := emptyInput(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Execute(context.Background(), []string{"--config", path, "club", "choose", "2,001,2"}, input, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Soho") || !strings.Contains(stdout.String(), "City") {
		t.Fatalf("choose output did not contain selected clubs:\n%s", stdout.String())
	}
	store, err := config.Open(path, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	state := store.State()
	if len(state.DefaultClubIDs) != 2 || state.DefaultClubIDs[0] != "2" || state.DefaultClubIDs[1] != "1" {
		t.Fatalf("stored default clubs = %#v", state.DefaultClubIDs)
	}
	if err := Execute(context.Background(), []string{"--config", path, "club", "choose", "9"}, input, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "unknown club ID") {
		t.Fatalf("expected unknown-club error, got %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Execute(context.Background(), []string{"--config", path, "club", "clear"}, input, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	store, err = config.Open(path, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	state = store.State()
	if len(state.DefaultClubIDs) != 0 {
		t.Fatalf("default clubs were not cleared: %#v", state.DefaultClubIDs)
	}
	if state.Session.Value != "session-token" {
		t.Fatalf("clearing clubs changed the session token: %#v", state.Session)
	}
}

func TestClassesClubPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		wantClubs string
	}{
		{name: "saved defaults", wantClubs: "1,2"},
		{name: "explicit override", arguments: []string{"--club", "003,3"}, wantClubs: "3"},
		{name: "all clubs", arguments: []string{"--club", "all"}, wantClubs: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearCLIEnvironment(t)
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/v2/member/classes/get_classes" {
					t.Errorf("unexpected request path %s", request.URL.Path)
					http.NotFound(response, request)
					return
				}
				if got := request.URL.Query().Get("ClubIds"); got != test.wantClubs {
					t.Errorf("ClubIds = %q, want %q", got, test.wantClubs)
				}
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, `{"classes":[]}`)
			}))
			defer server.Close()

			path := authenticatedConfig(t, server.URL, []string{"1", "2"})
			input := emptyInput(t)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := []string{"--config", path, "classes", "--date", "2026-08-04"}
			args = append(args, test.arguments...)
			if err := Execute(context.Background(), args, input, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateMutationAction(t *testing.T) {
	tests := []struct {
		command string
		action  string
		late    bool
		ok      bool
	}{
		{command: "book", action: "join", ok: true},
		{command: "waitlist", action: "wait", ok: true},
		{command: "cancel", action: "cancel", ok: true},
		{command: "cancel", action: "late_cancel", late: true, ok: true},
		{command: "leave-waitlist", action: "leave", ok: true},
		{command: "book", action: "wait"},
	}
	for _, test := range tests {
		late, err := validateMutationAction(test.command, test.action)
		if test.ok && (err != nil || late != test.late) {
			t.Fatalf("%s/%s = late %v, err %v", test.command, test.action, late, err)
		}
		if !test.ok && err == nil {
			t.Fatalf("%s/%s unexpectedly succeeded", test.command, test.action)
		}
	}
}

func TestYesMutationRechecksActionBeforeSubmitting(t *testing.T) {
	clearCLIEnvironment(t)
	detailCalls := 0
	mutationCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/member/classes/class_details/123":
			detailCalls++
			action := "cancel"
			if detailCalls == 2 {
				action = "late_cancel"
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(response, `{"id":123,"name":"Yoga","eventAction":%q}`, action)
		case "/api/v2/member/classes/cancel_booking":
			mutationCalls++
			_, _ = io.WriteString(response, `{}`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	path := authenticatedConfig(t, server.URL, nil)
	input := emptyInput(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), []string{"--config", path, "cancel", "--yes", "123"}, input, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "action changed") {
		t.Fatalf("expected changed-action error, got %v", err)
	}
	if detailCalls != 2 || mutationCalls != 0 {
		t.Fatalf("detail calls = %d, mutation calls = %d", detailCalls, mutationCalls)
	}
}

func emptyInput(t *testing.T) *os.File {
	t.Helper()
	input, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	return input
}

func authenticatedConfig(t *testing.T, baseURL string, defaultClubIDs []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := config.Open(path, baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(config.File{
		BaseURL:        baseURL,
		Token:          "session-token",
		ExpiresAt:      "2099-01-01T00:00:00Z",
		DefaultClubIDs: append([]string(nil), defaultClubIDs...),
	}); err != nil {
		t.Fatal(err)
	}
	return path
}

func clearCLIEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"THIRDSPACE_CONFIG",
		"THIRDSPACE_BASE_URL",
		"THIRDSPACE_EMAIL",
		"THIRDSPACE_PASSWORD",
		"THIRDSPACE_TOKEN",
	} {
		t.Setenv(name, "")
	}
}
