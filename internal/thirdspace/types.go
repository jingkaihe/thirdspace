package thirdspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

// ID preserves API identifiers whether the server encodes them as numbers or strings.
type ID string

func ParseID(value string) (ID, error) {
	value = strings.TrimSpace(value)
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return "", fmt.Errorf("class ID must be a positive integer, got %q", value)
	}
	return ID(strconv.FormatUint(parsed, 10)), nil
}

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

type SessionClub struct {
	ID   ID     `json:"id"`
	Name string `json:"name"`
}

type Session struct {
	Token        string        `json:"token"`
	ExpiresAt    string        `json:"expiresAt"`
	ID           ID            `json:"id,omitempty"`
	MembershipID string        `json:"membershipId,omitempty"`
	Name         string        `json:"name,omitempty"`
	Email        string        `json:"email,omitempty"`
	HomeClubID   ID            `json:"homeClubId,omitempty"`
	HomeClubName string        `json:"homeClubName,omitempty"`
	Clubs        []SessionClub `json:"clubs,omitempty"`
	Permissions  []string      `json:"permissions,omitempty"`
}

type RefreshToken struct {
	Token     string
	ExpiresAt string
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

func (class Class) ClubNameWithZone() string {
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

type ClassSearch struct {
	StartDate string
	EndDate   string
	ClubIDs   []string
	Limit     int
}

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
		start = nowLondon.Truncate(time.Minute)
		if !nowLondon.Equal(start) {
			start = start.Add(time.Minute)
		}
	}
	end := firstDay.AddDate(0, 0, days).Add(-time.Second)

	const apiLayout = "2006-01-02T15:04:05"
	return start.UTC().Format(apiLayout), end.UTC().Format(apiLayout), nil
}

func ParseTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
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
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}
