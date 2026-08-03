package classaction

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

type Kind string

const (
	Book          Kind = "book"
	Waitlist      Kind = "waitlist"
	Cancel        Kind = "cancel"
	LeaveWaitlist Kind = "leave-waitlist"
)

type Client interface {
	ClassDetail(context.Context, thirdspace.ID) (thirdspace.Class, error)
	BookClass(context.Context, thirdspace.ID) (json.RawMessage, error)
	JoinWaitlist(context.Context, thirdspace.ID) (json.RawMessage, error)
	CancelBooking(context.Context, thirdspace.ID, bool) (json.RawMessage, error)
	LeaveWaitlist(context.Context, thirdspace.ID) (json.RawMessage, error)
}

type Plan struct {
	Kind           Kind
	Class          thirdspace.Class
	ExpectedAction string
	Late           bool
}

func (kind Kind) Label() string {
	switch kind {
	case Book:
		return "Book class"
	case Waitlist:
		return "Join waitlist"
	case Cancel:
		return "Cancel booking"
	case LeaveWaitlist:
		return "Leave waitlist"
	default:
		return "Apply change"
	}
}

func KindForEventAction(action string) (Kind, bool, error) {
	switch action {
	case "join":
		return Book, false, nil
	case "wait":
		return Waitlist, false, nil
	case "cancel":
		return Cancel, false, nil
	case "late_cancel":
		return Cancel, true, nil
	case "leave":
		return LeaveWaitlist, false, nil
	case "":
		return "", false, fmt.Errorf("the server reports no available action")
	default:
		return "", false, fmt.Errorf("the server reports unknown class action %q", action)
	}
}

func Validate(kind Kind, action string) (bool, error) {
	if !kind.valid() {
		return false, fmt.Errorf("unknown mutation command %q", kind)
	}
	actual, late, err := KindForEventAction(action)
	if err == nil && actual == kind {
		return late, nil
	}
	if action == "" {
		action = "unknown"
	}
	return false, fmt.Errorf("cannot run %q: the server reports class action %q", kind, action)
}

func Prepare(ctx context.Context, client Client, kind Kind, id thirdspace.ID) (Plan, error) {
	detail, err := client.ClassDetail(ctx, id)
	if err != nil {
		return Plan{}, fmt.Errorf("could not verify class %s before making a change: %w", id, err)
	}
	late, err := Validate(kind, detail.EventAction)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Kind: kind, Class: detail, ExpectedAction: detail.EventAction, Late: late}, nil
}

func PrepareAvailable(ctx context.Context, client Client, id thirdspace.ID) (Plan, error) {
	detail, err := client.ClassDetail(ctx, id)
	if err != nil {
		return Plan{}, fmt.Errorf("could not verify class %s before making a change: %w", id, err)
	}
	kind, late, err := KindForEventAction(detail.EventAction)
	if err != nil {
		return Plan{}, fmt.Errorf("cannot change class %s: %w", id, err)
	}
	return Plan{Kind: kind, Class: detail, ExpectedAction: detail.EventAction, Late: late}, nil
}

func RecheckAndApply(ctx context.Context, client Client, plan Plan) (json.RawMessage, error) {
	current, err := client.ClassDetail(ctx, plan.Class.ID)
	if err != nil {
		return nil, fmt.Errorf("could not recheck class %s after confirmation; no change was made: %w", plan.Class.ID, err)
	}
	late, err := Validate(plan.Kind, current.EventAction)
	if err != nil {
		return nil, fmt.Errorf("class state changed after confirmation; no change was made: %w", err)
	}
	if current.EventAction != plan.ExpectedAction {
		return nil, fmt.Errorf("class action changed from %q to %q while awaiting confirmation; no change was made", plan.ExpectedAction, current.EventAction)
	}
	return apply(ctx, client, plan.Kind, plan.Class.ID, late)
}

func apply(ctx context.Context, client Client, kind Kind, id thirdspace.ID, late bool) (json.RawMessage, error) {
	switch kind {
	case Book:
		return client.BookClass(ctx, id)
	case Waitlist:
		return client.JoinWaitlist(ctx, id)
	case Cancel:
		return client.CancelBooking(ctx, id, late)
	case LeaveWaitlist:
		return client.LeaveWaitlist(ctx, id)
	default:
		return nil, fmt.Errorf("unknown mutation command %q", kind)
	}
}

func (kind Kind) valid() bool {
	switch kind {
	case Book, Waitlist, Cancel, LeaveWaitlist:
		return true
	default:
		return false
	}
}
