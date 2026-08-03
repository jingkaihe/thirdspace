package main

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
	_ "time/tzdata"
)

const (
	defaultBaseURL = "https://api.thirdspace-london.app"
	apiVersion     = "2.0"
	appVersion     = "6.6.0"
	appPlatform    = "android"
	maxResponse    = 32 << 20
)

// ID preserves API identifiers whether the server encodes them as numbers or strings.
type ID string

func (id *ID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*id = ""
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*id = ID(value)
		return nil
	}
	*id = ID(data)
	return nil
}

func (id ID) MarshalJSON() ([]byte, error) {
	value := string(id)
	if isDigits(value) {
		return []byte(value), nil
	}
	return json.Marshal(value)
}

func parseID(value string) (ID, error) {
	value = strings.TrimSpace(value)
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return "", fmt.Errorf("class ID must be a positive integer, got %q", value)
	}
	return ID(strconv.FormatUint(parsed, 10)), nil
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type Session struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

type Club struct {
	ID   ID     `json:"clubId"`
	Name string `json:"name"`
	Code string `json:"code,omitempty"`
}

type Class struct {
	ID                        ID     `json:"id"`
	ServiceID                 ID     `json:"serviceId,omitempty"`
	Name                      string `json:"name"`
	TrainerID                 ID     `json:"trainerId,omitempty"`
	TrainerName               string `json:"trainerName,omitempty"`
	ClubID                    ID     `json:"clubId,omitempty"`
	ClubName                  string `json:"clubName,omitempty"`
	LocationID                ID     `json:"locationId,omitempty"`
	LocationName              string `json:"locationName,omitempty"`
	ZoneID                    ID     `json:"zoneId,omitempty"`
	ZoneName                  string `json:"zoneName,omitempty"`
	StartDate                 string `json:"startDate,omitempty"`
	Spaces                    any    `json:"spaces,omitempty"`
	EventAction               string `json:"eventAction,omitempty"`
	WaitListPosition          any    `json:"waitListPosition,omitempty"`
	WaitListCount             any    `json:"waitListCount,omitempty"`
	AttendeeCount             any    `json:"attendeeCount,omitempty"`
	BookingOpens              string `json:"bookingOpens,omitempty"`
	ServiceCategory           any    `json:"serviceCategory,omitempty"`
	CancellationPenaltyPeriod any    `json:"cancellationPenaltyPeriod,omitempty"`
	Description               string `json:"description,omitempty"`
	Duration                  any    `json:"duration,omitempty"`
	StaffAccess               bool   `json:"staffAccess,omitempty"`
	UsesCredits               bool   `json:"usesCredits,omitempty"`
}

func (class Class) effectiveClubID() ID {
	if class.ClubID != "" {
		return class.ClubID
	}
	return class.LocationID
}

func (class Class) effectiveClubName() string {
	name := class.ClubName
	if name == "" {
		name = class.LocationName
	}
	if class.ZoneName != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(class.ZoneName)) {
		if name == "" {
			return class.ZoneName
		}
		return name + ", " + class.ZoneName
	}
	return name
}

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
	BaseURL    string
	Token      string
	AppVersion string
	HTTPClient *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		AppVersion: appVersion,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) == 0 {
					return nil
				}
				previous := via[len(via)-1].URL
				if request.URL.Scheme != previous.Scheme || !strings.EqualFold(request.URL.Host, previous.Host) {
					return fmt.Errorf("refusing cross-origin redirect from %s to %s", previous, request.URL)
				}
				return nil
			},
		},
	}
}

func validateAPIBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse API base URL: %w", err)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("API base URL must contain only a scheme and host")
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
	if parsed.Host == "" {
		return "", fmt.Errorf("API base URL is missing a host")
	}
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, output any) error {
	endpoint, err := url.Parse(c.BaseURL + "/" + strings.TrimLeft(path, "/"))
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
	request.Header.Set("x-app-version", c.AppVersion)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		request.Header.Set("x-fisikal-token", c.Token)
	}

	response, err := c.HTTPClient.Do(request)
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

func (c *Client) Login(ctx context.Context, email, password string) (Session, error) {
	query := url.Values{"api-version": {apiVersion}}
	body := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{Email: email, Password: password}

	var response struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
		Response  *struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expiresAt"`
		} `json:"response,omitempty"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v2/member/auth/login", query, body, &response); err != nil {
		return Session{}, err
	}

	session := Session{Token: response.Token, ExpiresAt: response.ExpiresAt}
	if session.Token == "" && response.Response != nil {
		session.Token = response.Response.Token
		if session.ExpiresAt == "" {
			session.ExpiresAt = response.Response.ExpiresAt
		}
	}
	if session.Token == "" {
		return Session{}, fmt.Errorf("login succeeded but the response contained no session token")
	}
	return session, nil
}

func (c *Client) Clubs(ctx context.Context) ([]Club, error) {
	query := url.Values{"api-version": {apiVersion}}
	var response struct {
		Clubs json.RawMessage `json:"clubs"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v2/member/clubs/get_clubs", query, nil, &response); err != nil {
		return nil, err
	}
	var clubs []Club
	if err := decodeRequiredList(response.Clubs, "clubs", &clubs); err != nil {
		return nil, err
	}
	return clubs, nil
}

type ClassSearch struct {
	StartDate string
	EndDate   string
	ClubIDs   []string
	Limit     int
}

func (c *Client) Classes(ctx context.Context, search ClassSearch) ([]Class, error) {
	limit := search.Limit
	if limit <= 0 {
		limit = 10_000_000
	}
	query := url.Values{
		"api-version":  {apiVersion},
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
	if err := c.do(ctx, http.MethodGet, "/api/v2/member/classes/get_classes", query, nil, &response); err != nil {
		return nil, err
	}
	var classes []Class
	if err := decodeRequiredList(response.Classes, "classes", &classes); err != nil {
		return nil, err
	}
	return classes, nil
}

func (c *Client) ClassDetail(ctx context.Context, id ID) (Class, error) {
	query := url.Values{"api-version": {apiVersion}}
	path := "/api/v2/member/classes/class_details/" + url.PathEscape(string(id))
	var class Class
	if err := c.do(ctx, http.MethodGet, path, query, nil, &class); err != nil {
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
		"api-version":        {apiVersion},
		"Limits.Count":       {"100"},
		"Range":              {rangeValue},
		"Cancelled":          {strconv.FormatBool(cancelled)},
		"ExcludeWaitingOnly": {strconv.FormatBool(excludeWaitingOnly)},
	}
	var response struct {
		Classes json.RawMessage `json:"classes"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v2/member/classes", query, nil, &response); err != nil {
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
	query := url.Values{"api-version": {apiVersion}}
	var response json.RawMessage
	if err := c.do(ctx, method, path, query, body, &response); err != nil {
		return nil, err
	}
	return response, nil
}

// BuildClassDateRange mirrors the app's London-aware timetable query format.
func BuildClassDateRange(date string, days int, now time.Time) (string, string, error) {
	if days < 1 || days > 31 {
		return "", "", fmt.Errorf("days must be between 1 and 31")
	}
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		return "", "", fmt.Errorf("load Europe/London timezone: %w", err)
	}

	nowLondon := now.In(london)
	var firstDay time.Time
	if date == "" {
		firstDay = time.Date(nowLondon.Year(), nowLondon.Month(), nowLondon.Day(), 0, 0, 0, 0, london)
	} else {
		firstDay, err = time.ParseInLocation("2006-01-02", date, london)
		if err != nil {
			return "", "", fmt.Errorf("date must use YYYY-MM-DD: %w", err)
		}
	}

	start := firstDay
	today := time.Date(nowLondon.Year(), nowLondon.Month(), nowLondon.Day(), 0, 0, 0, 0, london)
	if firstDay.Equal(today) && nowLondon.After(start) {
		start = time.Date(nowLondon.Year(), nowLondon.Month(), nowLondon.Day(), nowLondon.Hour(), nowLondon.Minute(), 0, 0, london)
		if nowLondon.Second() != 0 || nowLondon.Nanosecond() != 0 {
			start = start.Add(time.Minute)
		}
	}
	end := firstDay.AddDate(0, 0, days)
	end = end.Add(-time.Second)

	const apiLayout = "2006-01-02T15:04:05"
	return start.UTC().Format(apiLayout), end.UTC().Format(apiLayout), nil
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

func parseClassDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		return time.Time{}, err
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, london); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported date %q", value)
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
