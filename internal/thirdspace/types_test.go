package thirdspace

import (
	"testing"
	"time"
)

func TestParseIDRequiresPositiveInteger(t *testing.T) {
	id, err := ParseID("00042")
	if err != nil || id != "42" {
		t.Fatalf("ParseID = %q, %v", id, err)
	}
	for _, value := range []string{"", "0", "-1", "abc"} {
		if _, err := ParseID(value); err == nil {
			t.Fatalf("ParseID(%q) unexpectedly succeeded", value)
		}
	}
}

func TestBuildClassDateRangeUsesLondonTimezone(t *testing.T) {
	now := time.Date(2026, time.August, 3, 10, 30, 30, 0, time.UTC)
	start, end, err := BuildClassDateRange("2026-08-03", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if start != "2026-08-03T10:31:00" || end != "2026-08-03T22:59:59" {
		t.Fatalf("summer range = %s - %s", start, end)
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

func TestBuildClassDateRangePreservesSecondDSTFold(t *testing.T) {
	// 01:30 UTC is the second occurrence of 01:30 in London on this date.
	now := time.Date(2027, time.October, 31, 1, 30, 30, 0, time.UTC)
	start, _, err := BuildClassDateRange("", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if start != "2027-10-31T01:31:00" {
		t.Fatalf("start = %s, want second-fold instant 2027-10-31T01:31:00", start)
	}
}
