package classaction

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

type fakeClient struct {
	details []thirdspace.Class

	detailCalls        int
	bookCalls          int
	waitlistCalls      int
	cancelCalls        int
	leaveWaitlistCalls int
	cancelLate         bool
}

func (c *fakeClient) ClassDetail(_ context.Context, _ thirdspace.ID) (thirdspace.Class, error) {
	if c.detailCalls >= len(c.details) {
		return thirdspace.Class{}, fmt.Errorf("no detail response configured")
	}
	detail := c.details[c.detailCalls]
	c.detailCalls++
	return detail, nil
}

func (c *fakeClient) BookClass(context.Context, thirdspace.ID) (json.RawMessage, error) {
	c.bookCalls++
	return json.RawMessage(`{}`), nil
}

func (c *fakeClient) JoinWaitlist(context.Context, thirdspace.ID) (json.RawMessage, error) {
	c.waitlistCalls++
	return json.RawMessage(`{}`), nil
}

func (c *fakeClient) CancelBooking(_ context.Context, _ thirdspace.ID, late bool) (json.RawMessage, error) {
	c.cancelCalls++
	c.cancelLate = late
	return json.RawMessage(`{}`), nil
}

func (c *fakeClient) LeaveWaitlist(context.Context, thirdspace.ID) (json.RawMessage, error) {
	c.leaveWaitlistCalls++
	return json.RawMessage(`{}`), nil
}

func TestPrepareAvailableAndRecheckApply(t *testing.T) {
	tests := []struct {
		name   string
		action string
		kind   Kind
		late   bool
		calls  func(*fakeClient) int
	}{
		{name: "book", action: "join", kind: Book, calls: func(client *fakeClient) int { return client.bookCalls }},
		{name: "waitlist", action: "wait", kind: Waitlist, calls: func(client *fakeClient) int { return client.waitlistCalls }},
		{name: "cancel", action: "cancel", kind: Cancel, calls: func(client *fakeClient) int { return client.cancelCalls }},
		{name: "late cancel", action: "late_cancel", kind: Cancel, late: true, calls: func(client *fakeClient) int { return client.cancelCalls }},
		{name: "leave waitlist", action: "leave", kind: LeaveWaitlist, calls: func(client *fakeClient) int { return client.leaveWaitlistCalls }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			class := thirdspace.Class{ID: "123", Name: "Yoga", EventAction: test.action}
			client := &fakeClient{details: []thirdspace.Class{class, class}}
			plan, err := PrepareAvailable(context.Background(), client, class.ID)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Kind != test.kind || plan.Late != test.late {
				t.Fatalf("plan = %#v", plan)
			}
			if _, err := RecheckAndApply(context.Background(), client, plan); err != nil {
				t.Fatal(err)
			}
			if client.detailCalls != 2 {
				t.Fatalf("detail calls = %d, want 2", client.detailCalls)
			}
			if calls := test.calls(client); calls != 1 {
				t.Fatalf("mutation calls = %d, want 1", calls)
			}
			if test.kind == Cancel && client.cancelLate != test.late {
				t.Fatalf("late = %v, want %v", client.cancelLate, test.late)
			}
		})
	}
}

func TestRecheckRejectsChangedActionWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
		kind   Kind
	}{
		{name: "normal to late cancellation", before: "cancel", after: "late_cancel", kind: Cancel},
		{name: "book to waitlist", before: "join", after: "wait", kind: Book},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeClient{details: []thirdspace.Class{
				{ID: "123", EventAction: test.before},
				{ID: "123", EventAction: test.after},
			}}
			plan, err := Prepare(context.Background(), client, test.kind, "123")
			if err != nil {
				t.Fatal(err)
			}
			_, err = RecheckAndApply(context.Background(), client, plan)
			if err == nil || !strings.Contains(err.Error(), "no change was made") {
				t.Fatalf("expected state-change error, got %v", err)
			}
			if client.bookCalls+client.waitlistCalls+client.cancelCalls+client.leaveWaitlistCalls != 0 {
				t.Fatal("mutation was submitted after the class action changed")
			}
		})
	}
}
