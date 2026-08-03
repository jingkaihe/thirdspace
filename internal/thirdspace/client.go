package thirdspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.thirdspace-london.app"
	APIVersion     = "2.0"
	AppVersion     = "6.6.0"
	appPlatform    = "android"
	maxResponse    = 32 << 20
)

type APIError struct {
	StatusCode int
	Status     string
	Body       []byte
}

func (e *APIError) Error() string {
	message := extractAPIMessage(e.Body)
	if message == "" {
		return fmt.Sprintf("Third Space API returned %s", e.Status)
	}
	return fmt.Sprintf("Third Space API returned %s: %s", e.Status, message)
}

type Client struct {
	baseURL    string
	token      string
	appVersion string
	httpClient *http.Client
}

func NewClient(baseURL, token string) (*Client, error) {
	validated, err := ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL:    validated,
		token:      token,
		appVersion: AppVersion,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				return fmt.Errorf("refusing redirect to %s", request.URL)
			},
		},
	}, nil
}

func ValidateBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse API base URL: %w", err)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("API base URL must contain only a scheme and host")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("API base URL is missing a host")
	}
	hostname := strings.ToLower(parsed.Hostname())
	isLoopback := hostname == "localhost"
	if ip := net.ParseIP(hostname); ip != nil && ip.IsLoopback() {
		isLoopback = true
	}
	if hostname == "api.thirdspace-london.app" {
		if parsed.Scheme != "https" || (parsed.Port() != "" && parsed.Port() != "443") {
			return "", fmt.Errorf("the Third Space API must use https://api.thirdspace-london.app")
		}
	} else if isLoopback {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", fmt.Errorf("loopback API URLs must use http or https")
		}
	} else {
		return "", fmt.Errorf("refusing non-Third Space API host %q", hostname)
	}
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *Client) Login(ctx context.Context, email, password string) (Session, error) {
	body := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{Email: email, Password: password}
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodPost, "/api/v2/member/auth/login", apiQuery(), "", body, &raw); err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, fmt.Errorf("decode login response: %w", err)
	}
	if session.Token == "" {
		var envelope tokenEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return Session{}, fmt.Errorf("decode login response envelope: %w", err)
		}
		session = envelope.Response
		if session.ExpiresAt == "" {
			session.ExpiresAt = envelope.ExpiresAt
		}
	}
	if err := validateSession(session, "login"); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (c *Client) IssueRefreshToken(ctx context.Context, sessionToken string) (RefreshToken, error) {
	var response tokenEnvelope
	if err := c.do(ctx, http.MethodPost, "/api/v2/member/auth/token/refresh", apiQuery(), sessionToken, nil, &response); err != nil {
		return RefreshToken{}, err
	}
	refresh := response.Response
	if refresh.ExpiresAt == "" {
		refresh.ExpiresAt = response.ExpiresAt
	}
	if err := validateSession(refresh, "refresh-token"); err != nil {
		return RefreshToken{}, err
	}
	return RefreshToken{Token: refresh.Token, ExpiresAt: refresh.ExpiresAt}, nil
}

func (c *Client) ExchangeRefreshToken(ctx context.Context, refreshToken string) (Session, error) {
	var response tokenEnvelope
	if err := c.do(ctx, http.MethodPost, "/api/v2/member/auth/token", apiQuery(), refreshToken, nil, &response); err != nil {
		return Session{}, err
	}
	session := response.Response
	if session.ExpiresAt == "" {
		session.ExpiresAt = response.ExpiresAt
	}
	if err := validateSession(session, "refresh exchange"); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (c *Client) Clubs(ctx context.Context) ([]Club, error) {
	var response struct {
		Clubs json.RawMessage `json:"clubs"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v2/member/clubs/get_clubs", apiQuery(), "", nil, &response); err != nil {
		return nil, err
	}
	var clubs []Club
	if err := decodeRequiredList(response.Clubs, "clubs", &clubs); err != nil {
		return nil, err
	}
	return clubs, nil
}

func (c *Client) Classes(ctx context.Context, search ClassSearch) ([]Class, error) {
	limit := search.Limit
	if limit <= 0 {
		limit = 10_000_000
	}
	query := url.Values{
		"api-version":  {APIVersion},
		"Limits.Count": {strconv.Itoa(limit)},
		"StartDate":    {search.StartDate},
		"EndDate":      {search.EndDate},
	}
	if len(search.ClubIDs) > 0 {
		query.Set("ClubIds", strings.Join(search.ClubIDs, ","))
	}
	var response struct {
		Classes json.RawMessage `json:"classes"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v2/member/classes/get_classes", query, "", nil, &response); err != nil {
		return nil, err
	}
	var classes []Class
	if err := decodeRequiredList(response.Classes, "classes", &classes); err != nil {
		return nil, err
	}
	return classes, nil
}

func (c *Client) ClassDetail(ctx context.Context, id ID) (Class, error) {
	path := "/api/v2/member/classes/class_details/" + url.PathEscape(string(id))
	var class Class
	if err := c.do(ctx, http.MethodGet, path, apiQuery(), "", nil, &class); err != nil {
		return Class{}, err
	}
	if class.ID == "" {
		return Class{}, fmt.Errorf("class detail response contained no class ID")
	}
	if class.ID != id {
		return Class{}, fmt.Errorf("class detail response ID %s did not match requested ID %s", class.ID, id)
	}
	return class, nil
}

func (c *Client) Bookings(ctx context.Context, requestedRange string) ([]Class, error) {
	rangeValue := "future"
	cancelled := false
	excludeWaitingOnly := false
	switch requestedRange {
	case "upcoming":
	case "past":
		rangeValue = "past"
		excludeWaitingOnly = true
	case "cancelled":
		rangeValue = "all"
		cancelled = true
	default:
		return nil, fmt.Errorf("range must be one of upcoming, past, or cancelled")
	}
	query := url.Values{
		"api-version":        {APIVersion},
		"Limits.Count":       {"100"},
		"Range":              {rangeValue},
		"Cancelled":          {strconv.FormatBool(cancelled)},
		"ExcludeWaitingOnly": {strconv.FormatBool(excludeWaitingOnly)},
	}
	var response struct {
		Classes json.RawMessage `json:"classes"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v2/member/classes", query, "", nil, &response); err != nil {
		return nil, err
	}
	var classes []Class
	if err := decodeRequiredList(response.Classes, "classes", &classes); err != nil {
		return nil, err
	}
	return classes, nil
}

func (c *Client) BookClass(ctx context.Context, id ID) (json.RawMessage, error) {
	return c.classAction(ctx, http.MethodPost, "/api/v2/member/classes/book_class", struct {
		ClassID ID `json:"classId"`
	}{ClassID: id})
}

func (c *Client) JoinWaitlist(ctx context.Context, id ID) (json.RawMessage, error) {
	return c.classAction(ctx, http.MethodPost, "/api/v2/member/classes/join_waitlist", struct {
		ClassID ID `json:"classId"`
	}{ClassID: id})
}

func (c *Client) CancelBooking(ctx context.Context, id ID, late bool) (json.RawMessage, error) {
	return c.classAction(ctx, http.MethodPut, "/api/v2/member/classes/cancel_booking", struct {
		ClassID     ID   `json:"classId"`
		LateBooking bool `json:"lateBooking"`
	}{ClassID: id, LateBooking: late})
}

func (c *Client) LeaveWaitlist(ctx context.Context, id ID) (json.RawMessage, error) {
	return c.classAction(ctx, http.MethodPut, "/api/v2/member/classes/leave_waitlist", struct {
		ClassID ID `json:"classId"`
	}{ClassID: id})
}

func (c *Client) classAction(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var response json.RawMessage
	if err := c.do(ctx, method, path, apiQuery(), "", body, &response); err != nil {
		return nil, err
	}
	return response, nil
}

type tokenEnvelope struct {
	ExpiresAt string  `json:"expiresAt"`
	Response  Session `json:"response"`
}

func validateSession(session Session, operation string) error {
	if session.Token == "" {
		return fmt.Errorf("%s succeeded but the response contained no token", operation)
	}
	if session.ExpiresAt == "" {
		return fmt.Errorf("%s succeeded but the response contained no expiry", operation)
	}
	if _, err := ParseTimestamp(session.ExpiresAt); err != nil {
		return fmt.Errorf("%s returned an invalid expiry: %w", operation, err)
	}
	return nil
}

func apiQuery() url.Values {
	return url.Values{"api-version": {APIVersion}}
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, explicitToken string, body, output any) error {
	endpoint, err := url.Parse(c.baseURL + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return fmt.Errorf("build request URL: %w", err)
	}
	endpoint.RawQuery = query.Encode()

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), requestBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("x-app-platform", appPlatform)
	request.Header.Set("x-app-version", c.appVersion)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	token := explicitToken
	if token == "" {
		token = c.token
	}
	if token != "" {
		request.Header.Set("x-fisikal-token", token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request Third Space API: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponse))
	if err != nil {
		return fmt.Errorf("read Third Space API response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{StatusCode: response.StatusCode, Status: response.Status, Body: responseBody}
	}
	if output == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("decode Third Space API response: %w", err)
	}
	return nil
}

func decodeRequiredList(raw json.RawMessage, field string, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("Third Space API response was missing required %q list", field)
	}
	if err := json.Unmarshal(trimmed, target); err != nil {
		return fmt.Errorf("decode Third Space API %q list: %w", field, err)
	}
	return nil
}

func extractAPIMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var value any
	if json.Unmarshal(body, &value) == nil {
		if message := findMessage(value); message != "" {
			return message
		}
	}
	if len(trimmed) > 500 {
		return trimmed[:500] + "…"
	}
	return trimmed
}

func findMessage(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"message", "detail", "title", "error", "reason"} {
			if text, ok := typed[key].(string); ok && text != "" {
				return text
			}
		}
		for _, key := range []string{"errors", "response", "data"} {
			if nested, ok := typed[key]; ok {
				if message := findMessage(nested); message != "" {
					return message
				}
			}
		}
	case []any:
		for _, item := range typed {
			if message := findMessage(item); message != "" {
				return message
			}
		}
	case string:
		return typed
	}
	return ""
}
