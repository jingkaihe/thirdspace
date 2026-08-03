package tui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jingkaihe/thirdspace/internal/config"
	"github.com/jingkaihe/thirdspace/internal/session"
	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

type api interface {
	Clubs(context.Context) ([]thirdspace.Club, error)
	Classes(context.Context, thirdspace.ClassSearch) ([]thirdspace.Class, error)
	ClassDetail(context.Context, thirdspace.ID) (thirdspace.Class, error)
	Bookings(context.Context, string) ([]thirdspace.Class, error)
	BookClass(context.Context, thirdspace.ID) (json.RawMessage, error)
	JoinWaitlist(context.Context, thirdspace.ID) (json.RawMessage, error)
	CancelBooking(context.Context, thirdspace.ID, bool) (json.RawMessage, error)
	LeaveWaitlist(context.Context, thirdspace.ID) (json.RawMessage, error)
}

type loginResult struct {
	client  api
	warning error
}

type backend interface {
	Connect(context.Context) (api, error)
	Login(context.Context, string, string) (loginResult, error)
	SetDefaultClubIDs([]string) error
}

type sessionBackend struct {
	manager *session.Manager
	store   *config.Store
}

func (b sessionBackend) Connect(ctx context.Context) (api, error) {
	return b.manager.Client(ctx)
}

func (b sessionBackend) Login(ctx context.Context, email, password string) (loginResult, error) {
	result, err := b.manager.Login(ctx, email, password)
	if err != nil {
		return loginResult{}, err
	}
	client, err := b.manager.Client(ctx)
	if err != nil {
		return loginResult{}, err
	}
	return loginResult{client: client, warning: result.RefreshWarning}, nil
}

func (b sessionBackend) SetDefaultClubIDs(ids []string) error {
	return b.store.SetDefaultClubIDs(ids)
}

func Run(ctx context.Context, store *config.Store, input io.Reader, output io.Writer) error {
	state := store.State()
	model := newModel(
		ctx,
		sessionBackend{manager: session.NewManager(store), store: store},
		state.Email,
		state.DefaultClubIDs,
		time.Now,
	)
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
	)
	_, err := program.Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return nil
	}
	return err
}
