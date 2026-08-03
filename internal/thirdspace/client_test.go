package thirdspace

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v2/member/auth/login" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
			return
		}
		if got := request.URL.Query().Get("api-version"); got != APIVersion {
			t.Errorf("api-version = %q", got)
		}
		if got := request.Header.Get("x-app-platform"); got != "android" {
			t.Errorf("x-app-platform = %q", got)
		}
		if got := request.Header.Get("x-app-version"); got != AppVersion {
			t.Errorf("x-app-version = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body["email"] != "member@example.com" || body["password"] != "secret" {
			t.Errorf("unexpected body: %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"id":123,"token":"session-token","expiresAt":"2026-09-02T12:00:00"}`)
	}))
	defer server.Close()

	client := mustClient(t, server.URL, "")
	session, err := client.Login(context.Background(), "member@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token != "session-token" || session.ExpiresAt != "2026-09-02T12:00:00" {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestRefreshTokenRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v2/member/auth/token/refresh":
			if request.Method != http.MethodPost || request.Header.Get("x-fisikal-token") != "session-token" {
				t.Errorf("refresh request = %s token %q", request.Method, request.Header.Get("x-fisikal-token"))
			}
			_, _ = io.WriteString(response, `{"expiresAt":"2026-10-02T12:00:00","response":{"token":"refresh-token"}}`)
		case "/api/v2/member/auth/token":
			if request.Method != http.MethodPost || request.Header.Get("x-fisikal-token") != "refresh-token" {
				t.Errorf("exchange request = %s token %q", request.Method, request.Header.Get("x-fisikal-token"))
			}
			_, _ = io.WriteString(response, `{"response":{"token":"new-session","expiresAt":"2026-10-02T12:00:00","email":"member@example.com"}}`)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := mustClient(t, server.URL, "")
	refresh, err := client.IssueRefreshToken(context.Background(), "session-token")
	if err != nil {
		t.Fatal(err)
	}
	if refresh.Token != "refresh-token" || refresh.ExpiresAt != "2026-10-02T12:00:00" {
		t.Fatalf("unexpected refresh token: %#v", refresh)
	}
	session, err := client.ExchangeRefreshToken(context.Background(), refresh.Token)
	if err != nil {
		t.Fatal(err)
	}
	if session.Token != "new-session" || session.Email != "member@example.com" {
		t.Fatalf("unexpected exchanged session: %#v", session)
	}
}

func TestLoginAcceptsTokenEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"expiresAt":"2026-09-02T12:00:00","response":{"token":"session-token","email":"member@example.com"}}`)
	}))
	defer server.Close()

	session, err := mustClient(t, server.URL, "").Login(context.Background(), "member@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token != "session-token" || session.ExpiresAt != "2026-09-02T12:00:00" {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestClassesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v2/member/classes/get_classes" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
			return
		}
		if got := request.Header.Get("x-fisikal-token"); got != "token" {
			t.Errorf("x-fisikal-token = %q", got)
		}
		query := request.URL.Query()
		for key, expected := range map[string]string{
			"api-version":  APIVersion,
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
		_, _ = io.WriteString(response, `{"classes":[{"id":456,"name":"Yoga","startDate":"2026-08-03T18:00:00","clubName":"Soho"}]}`)
	}))
	defer server.Close()

	client := mustClient(t, server.URL, "token")
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

func TestClassActionRequestsUseNumericIDs(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		call   func(*Client) (json.RawMessage, error)
	}{
		{name: "book", method: http.MethodPost, path: "/api/v2/member/classes/book_class", body: `{"classId":456}`, call: func(client *Client) (json.RawMessage, error) {
			return client.BookClass(context.Background(), "456")
		}},
		{name: "waitlist", method: http.MethodPost, path: "/api/v2/member/classes/join_waitlist", body: `{"classId":456}`, call: func(client *Client) (json.RawMessage, error) {
			return client.JoinWaitlist(context.Background(), "456")
		}},
		{name: "late cancel", method: http.MethodPut, path: "/api/v2/member/classes/cancel_booking", body: `{"classId":456,"lateBooking":true}`, call: func(client *Client) (json.RawMessage, error) {
			return client.CancelBooking(context.Background(), "456", true)
		}},
		{name: "leave waitlist", method: http.MethodPut, path: "/api/v2/member/classes/leave_waitlist", body: `{"classId":456}`, call: func(client *Client) (json.RawMessage, error) {
			return client.LeaveWaitlist(context.Background(), "456")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method != test.method || request.URL.Path != test.path {
					t.Errorf("request = %s %s", request.Method, request.URL.Path)
					return
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Error(err)
					return
				}
				if string(body) != test.body {
					t.Errorf("body = %s, want %s", body, test.body)
				}
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, `{"success":true}`)
			}))
			defer server.Close()

			result, err := test.call(mustClient(t, server.URL, "token"))
			if err != nil {
				t.Fatal(err)
			}
			if string(result) != `{"success":true}` {
				t.Fatalf("result = %s", result)
			}
		})
	}
}

func TestClientValidatesResponsesAndRedirects(t *testing.T) {
	t.Run("class ID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"id":999,"name":"Wrong class"}`)
		}))
		defer server.Close()
		_, err := mustClient(t, server.URL, "token").ClassDetail(context.Background(), "456")
		if err == nil || !strings.Contains(err.Error(), "did not match") {
			t.Fatalf("expected mismatched-ID error, got %v", err)
		}
	})

	t.Run("required list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{}`)
		}))
		defer server.Close()
		_, err := mustClient(t, server.URL, "token").Classes(context.Background(), ClassSearch{})
		if err == nil || !strings.Contains(err.Error(), `required "classes" list`) {
			t.Fatalf("expected missing-list error, got %v", err)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("redirect target received an authenticated request")
		}))
		defer target.Close()
		source := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			http.Redirect(response, request, target.URL, http.StatusFound)
		}))
		defer source.Close()
		_, err := mustClient(t, source.URL, "token").Clubs(context.Background())
		if err == nil || !strings.Contains(err.Error(), "refusing redirect") {
			t.Fatalf("expected redirect error, got %v", err)
		}
	})
}

func TestValidateBaseURL(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "https://api.thirdspace-london.app/", want: DefaultBaseURL},
		{input: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{input: "http://api.thirdspace-london.app", wantErr: true},
		{input: "https://example.com", wantErr: true},
		{input: "https://api.thirdspace-london.app/path", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := ValidateBaseURL(test.input)
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

func mustClient(t *testing.T, baseURL, token string) *Client {
	t.Helper()
	client, err := NewClient(baseURL, token)
	if err != nil {
		t.Fatal(err)
	}
	return client
}
