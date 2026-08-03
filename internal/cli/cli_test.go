package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jingkaihe/thirdspace/internal/version"
)

func TestHelpUsesCobraCommandTree(t *testing.T) {
	input := emptyInput(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Execute(context.Background(), []string{"--help"}, input, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"classes", "bookings", "refresh", "leave-waitlist"} {
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

func emptyInput(t *testing.T) *os.File {
	t.Helper()
	input, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	return input
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
