package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoginRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/api/v2/member/auth/login" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("api-version"); got != "2.0" {
			t.Fatalf("api-version = %q", got)
		}
		if got := request.Header.Get("x-app-platform"); got != "android" {
			t.Fatalf("x-app-platform = %q", got)
		}
		if got := request.Header.Get("x-app-version"); got != appVersion {
			t.Fatalf("x-app-version = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["email"] != "member@example.com" || body["password"] != "secret" {
			t.Fatalf("unexpected body: %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		io.WriteString(response, `{"id":123,"token":"session-token","expiresAt":"2026-08-04T12:00:00"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	session, err := client.Login(context.Background(), "member@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token != "session-token" || session.ExpiresAt != "2026-08-04T12:00:00" {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestClassesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v2/member/classes/get_classes" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("x-fisikal-token"); got != "token" {
			t.Fatalf("x-fisikal-token = %q", got)
		}
		query := request.URL.Query()
		for key, expected := range map[string]string{
			"api-version":  "2.0",
			"Limits.Count": "10000000",
			"StartDate":    "2026-08-03T10:30:00",
			"EndDate":      "2026-08-03T22:59:59",
			"ClubIds":      "1,2",
		} {
			if got := query.Get(key); got != expected {
				t.Errorf("%s = %q, want %q", key, got, expected)
			}
		}
		response.Header().Set("Content-Type", "application/json")
		io.WriteString(response, `{"classes":[{"id":456,"name":"Yoga","startDate":"2026-08-03T18:00:00","clubName":"Soho"}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	classes, err := client.Classes(context.Background(), ClassSearch{
		StartDate: "2026-08-03T10:30:00",
		EndDate:   "2026-08-03T22:59:59",
		ClubIDs:   []string{"1", "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 1 || classes[0].ID != "456" || classes[0].Name != "Yoga" {
		t.Fatalf("unexpected classes: %#v", classes)
	}
}

func TestBookClassUsesNumericID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v2/member/classes/book_class" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"classId":456}` {
			t.Fatalf("body = %s", body)
		}
		response.Header().Set("Content-Type", "application/json")
		io.WriteString(response, `{"success":true}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	result, err := client.BookClass(context.Background(), ID("456"))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"success":true}` {
		t.Fatalf("result = %s", result)
	}
}

func TestClassActionRequests(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		call   func(*Client) (json.RawMessage, error)
	}{
		{
			name:   "join waitlist",
			method: http.MethodPost,
			path:   "/api/v2/member/classes/join_waitlist",
			body:   `{"classId":456}`,
			call: func(client *Client) (json.RawMessage, error) {
				return client.JoinWaitlist(context.Background(), "456")
			},
		},
		{
			name:   "cancel booking",
			method: http.MethodPut,
			path:   "/api/v2/member/classes/cancel_booking",
			body:   `{"classId":456,"lateBooking":false}`,
			call: func(client *Client) (json.RawMessage, error) {
				return client.CancelBooking(context.Background(), "456", false)
			},
		},
		{
			name:   "late cancel booking",
			method: http.MethodPut,
			path:   "/api/v2/member/classes/cancel_booking",
			body:   `{"classId":456,"lateBooking":true}`,
			call: func(client *Client) (json.RawMessage, error) {
				return client.CancelBooking(context.Background(), "456", true)
			},
		},
		{
			name:   "leave waitlist",
			method: http.MethodPut,
			path:   "/api/v2/member/classes/leave_waitlist",
			body:   `{"classId":456}`,
			call: func(client *Client) (json.RawMessage, error) {
				return client.LeaveWaitlist(context.Background(), "456")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method != test.method || request.URL.Path != test.path {
					t.Fatalf("request = %s %s", request.Method, request.URL.Path)
				}
				if got := request.URL.Query().Get("api-version"); got != apiVersion {
					t.Fatalf("api-version = %q", got)
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != test.body {
					t.Fatalf("body = %s, want %s", body, test.body)
				}
				response.Header().Set("Content-Type", "application/json")
				io.WriteString(response, `{"success":true}`)
			}))
			defer server.Close()

			result, err := test.call(NewClient(server.URL, "token"))
			if err != nil {
				t.Fatal(err)
			}
			if string(result) != `{"success":true}` {
				t.Fatalf("result = %s", result)
			}
		})
	}
}

func TestClassDetailValidatesResponseID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		io.WriteString(response, `{"id":999,"name":"Wrong class"}`)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "token").ClassDetail(context.Background(), "456")
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("expected mismatched-ID error, got %v", err)
	}
}

func TestClassesRejectMissingEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		io.WriteString(response, `{}`)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "token").Classes(context.Background(), ClassSearch{})
	if err == nil || !strings.Contains(err.Error(), "required \"classes\" list") {
		t.Fatalf("expected missing-list error, got %v", err)
	}
}

func TestClientRejectsCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		t.Fatal("redirect target should not receive authenticated request")
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusFound)
	}))
	defer source.Close()

	_, err := NewClient(source.URL, "token").Clubs(context.Background())
	if err == nil || !strings.Contains(err.Error(), "cross-origin redirect") {
		t.Fatalf("expected redirect error, got %v", err)
	}
}

func TestValidateAPIBaseURL(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "https://api.thirdspace-london.app/", want: defaultBaseURL},
		{input: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{input: "http://api.thirdspace-london.app", wantErr: true},
		{input: "https://example.com", wantErr: true},
		{input: "https://api.thirdspace-london.app/path", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := validateAPIBaseURL(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildClassDateRangeUsesLondonTimezone(t *testing.T) {
	now := time.Date(2026, time.August, 3, 10, 30, 30, 0, time.UTC)
	start, end, err := BuildClassDateRange("2026-08-03", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if start != "2026-08-03T10:31:00" {
		t.Fatalf("start = %s", start)
	}
	if end != "2026-08-03T22:59:59" {
		t.Fatalf("end = %s", end)
	}

	start, end, err = BuildClassDateRange("2026-12-01", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if start != "2026-12-01T00:00:00" || end != "2026-12-01T23:59:59" {
		t.Fatalf("winter range = %s - %s", start, end)
	}

	start, end, err = BuildClassDateRange("2027-03-28", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if start != "2027-03-28T00:00:00" || end != "2027-03-28T22:59:59" {
		t.Fatalf("spring DST range = %s - %s", start, end)
	}

	start, end, err = BuildClassDateRange("2027-10-31", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if start != "2027-10-30T23:00:00" || end != "2027-10-31T23:59:59" {
		t.Fatalf("autumn DST range = %s - %s", start, end)
	}
}

func TestSaveConfigUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	if err := saveConfig(path, Config{Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissions = %o, want 600", got)
	}
}

func TestLoadConfigRejectsLoosePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"token":"secret"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil || !strings.Contains(err.Error(), "permissions 644") {
		t.Fatalf("expected loose-permission error, got %v", err)
	}
}

func TestParseIDRequiresPositiveInteger(t *testing.T) {
	id, err := parseID("00042")
	if err != nil || id != "42" {
		t.Fatalf("parseID = %q, %v", id, err)
	}
	for _, value := range []string{"", "0", "-1", "abc"} {
		if _, err := parseID(value); err == nil {
			t.Fatalf("parseID(%q) unexpectedly succeeded", value)
		}
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

func TestPasswordStdinTakesPrecedenceOverEnvironment(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "password-*")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err := input.WriteString("stdin-password\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THIRDSPACE_PASSWORD", "environment-password")
	password, err := readPassword(input, io.Discard, true)
	if err != nil {
		t.Fatal(err)
	}
	if password != "stdin-password" {
		t.Fatalf("password = %q", password)
	}
}
