package session

import (
	"context"
	"fmt"
	"time"

	"github.com/jingkaihe/thirdspace/internal/config"
	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

// Refresh early enough that an occasionally used CLI is unlikely to let both
// the session and refresh token reach their roughly one-month expiry together.
const defaultRefreshBefore = 72 * time.Hour

type LoginResult struct {
	Session        thirdspace.Session
	Refresh        thirdspace.RefreshToken
	RefreshWarning error
}

type RefreshResult struct {
	Session        thirdspace.Session
	Refresh        thirdspace.RefreshToken
	RefreshWarning error
}

type Manager struct {
	store         *config.Store
	now           func() time.Time
	refreshBefore time.Duration
}

func NewManager(store *config.Store) *Manager {
	return &Manager{store: store, now: time.Now, refreshBefore: defaultRefreshBefore}
}

func (m *Manager) Login(ctx context.Context, email, password string) (LoginResult, error) {
	state := m.store.State()
	client, err := thirdspace.NewClient(state.BaseURL, "")
	if err != nil {
		return LoginResult{}, err
	}
	session, err := client.Login(ctx, email, password)
	if err != nil {
		return LoginResult{}, fmt.Errorf("login failed: %w", err)
	}
	refresh, refreshErr := client.IssueRefreshToken(ctx, session.Token)
	file := config.File{
		BaseURL:        state.BaseURL,
		Email:          email,
		Token:          session.Token,
		ExpiresAt:      session.ExpiresAt,
		DefaultClubIDs: append([]string(nil), state.DefaultClubIDs...),
	}
	if refreshErr == nil {
		file.RefreshToken = refresh.Token
		file.RefreshTokenExpiresAt = refresh.ExpiresAt
	}
	if err := m.store.Save(file); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Session: session, Refresh: refresh, RefreshWarning: refreshErr}, nil
}

func (m *Manager) Client(ctx context.Context) (*thirdspace.Client, error) {
	state := m.store.State()
	if state.Session.Value == "" {
		return m.refreshClient(ctx, state)
	}
	if state.SessionFromEnvironment {
		return thirdspace.NewClient(state.BaseURL, state.Session.Value)
	}
	if _, err := m.persistenceBaseURL(state); err != nil {
		return nil, err
	}
	current, err := thirdspace.NewClient(state.BaseURL, state.Session.Value)
	if err != nil {
		return nil, err
	}
	now := m.now()
	usable, err := tokenUsable(state.Session, now, 0)
	if err != nil {
		return nil, fmt.Errorf("stored session expiry is invalid: %w", err)
	}
	if !usable {
		return m.refreshClient(ctx, state)
	}
	if state.Refresh.Value == "" {
		return current, nil
	}
	outsideRefreshWindow, err := tokenUsable(state.Session, now, m.refreshBefore)
	if err != nil {
		return nil, fmt.Errorf("stored session expiry is invalid: %w", err)
	}
	if outsideRefreshWindow {
		return current, nil
	}
	refreshed, err := m.refreshClient(ctx, state)
	if err != nil {
		// Proactive renewal should not shorten the useful life of a valid
		// session when the network or refresh endpoint is temporarily down.
		return current, nil
	}
	return refreshed, nil
}

func (m *Manager) ForceRefresh(ctx context.Context) (RefreshResult, error) {
	state := m.store.State()
	if state.SessionFromEnvironment {
		return RefreshResult{}, fmt.Errorf("cannot persist refreshed credentials while token environment overrides are active")
	}
	_, result, err := m.refresh(ctx, state)
	return result, err
}

func (m *Manager) refreshClient(ctx context.Context, state config.State) (*thirdspace.Client, error) {
	client, _, err := m.refresh(ctx, state)
	return client, err
}

func (m *Manager) refresh(ctx context.Context, state config.State) (*thirdspace.Client, RefreshResult, error) {
	if state.Refresh.Value == "" {
		return nil, RefreshResult{}, fmt.Errorf("no refresh token is stored; run 'thirdspace login'")
	}
	usable, err := tokenUsable(state.Refresh, m.now(), 0)
	if err != nil {
		return nil, RefreshResult{}, fmt.Errorf("stored refresh-token expiry is invalid: %w", err)
	}
	if !usable {
		return nil, RefreshResult{}, fmt.Errorf("refresh token has expired; run 'thirdspace login'")
	}
	baseURL, err := m.persistenceBaseURL(state)
	if err != nil {
		return nil, RefreshResult{}, err
	}

	client, err := thirdspace.NewClient(state.BaseURL, "")
	if err != nil {
		return nil, RefreshResult{}, err
	}
	session, err := client.ExchangeRefreshToken(ctx, state.Refresh.Value)
	if err != nil {
		return nil, RefreshResult{}, fmt.Errorf("refresh session: %w", err)
	}
	refresh, refreshErr := client.IssueRefreshToken(ctx, session.Token)
	email := session.Email
	if email == "" {
		email = state.Email
	}
	file := config.File{
		BaseURL:        baseURL,
		Email:          email,
		Token:          session.Token,
		ExpiresAt:      session.ExpiresAt,
		DefaultClubIDs: append([]string(nil), state.DefaultClubIDs...),
	}
	if refreshErr == nil {
		file.RefreshToken = refresh.Token
		file.RefreshTokenExpiresAt = refresh.ExpiresAt
	}
	if err := m.store.Save(file); err != nil {
		return nil, RefreshResult{}, err
	}
	authenticated, err := thirdspace.NewClient(state.BaseURL, session.Token)
	if err != nil {
		return nil, RefreshResult{}, err
	}
	return authenticated, RefreshResult{
		Session:        session,
		Refresh:        refresh,
		RefreshWarning: refreshErr,
	}, nil
}

func (m *Manager) persistenceBaseURL(state config.State) (string, error) {
	effective, err := thirdspace.ValidateBaseURL(state.BaseURL)
	if err != nil {
		return "", err
	}
	stored := m.store.Stored().BaseURL
	if stored == "" {
		return effective, nil
	}
	stored, err = thirdspace.ValidateBaseURL(stored)
	if err != nil {
		return "", fmt.Errorf("stored API base URL is invalid: %w", err)
	}
	if stored != effective {
		return "", fmt.Errorf("refusing to persist refreshed credentials while THIRDSPACE_BASE_URL overrides the stored API")
	}
	return stored, nil
}

func tokenUsable(token config.Token, now time.Time, refreshBefore time.Duration) (bool, error) {
	if token.Value == "" {
		return false, nil
	}
	if token.ExpiresAt == "" {
		return true, nil
	}
	expiresAt, err := thirdspace.ParseTimestamp(token.ExpiresAt)
	if err != nil {
		return false, err
	}
	return expiresAt.After(now.Add(refreshBefore)), nil
}
