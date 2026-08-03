package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jingkaihe/thirdspace/internal/classaction"
	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

type fakeBackend struct {
	client   api
	loginErr error
	saveErr  error
	savedIDs [][]string
}

func (b *fakeBackend) Connect(context.Context) (api, error) {
	return b.client, nil
}

func (b *fakeBackend) Login(context.Context, string, string) (loginResult, error) {
	if b.loginErr != nil {
		return loginResult{}, b.loginErr
	}
	return loginResult{client: b.client}, nil
}

func (b *fakeBackend) SetDefaultClubIDs(ids []string) error {
	b.savedIDs = append(b.savedIDs, append([]string(nil), ids...))
	return b.saveErr
}

type fakeAPI struct {
	searches    []thirdspace.ClassSearch
	classes     []thirdspace.Class
	bookings    []thirdspace.Class
	clubs       []thirdspace.Club
	details     []thirdspace.Class
	detailCalls int
	mutations   int
}

func (a *fakeAPI) Clubs(context.Context) ([]thirdspace.Club, error) {
	return append([]thirdspace.Club(nil), a.clubs...), nil
}

func (a *fakeAPI) Classes(_ context.Context, search thirdspace.ClassSearch) ([]thirdspace.Class, error) {
	search.ClubIDs = append([]string(nil), search.ClubIDs...)
	a.searches = append(a.searches, search)
	return append([]thirdspace.Class(nil), a.classes...), nil
}

func (a *fakeAPI) ClassDetail(context.Context, thirdspace.ID) (thirdspace.Class, error) {
	if a.detailCalls >= len(a.details) {
		return thirdspace.Class{}, fmt.Errorf("no detail response configured")
	}
	detail := a.details[a.detailCalls]
	a.detailCalls++
	return detail, nil
}

func (a *fakeAPI) Bookings(context.Context, string) ([]thirdspace.Class, error) {
	return append([]thirdspace.Class(nil), a.bookings...), nil
}

func (a *fakeAPI) BookClass(context.Context, thirdspace.ID) (json.RawMessage, error) {
	a.mutations++
	return json.RawMessage(`{}`), nil
}

func (a *fakeAPI) JoinWaitlist(context.Context, thirdspace.ID) (json.RawMessage, error) {
	a.mutations++
	return json.RawMessage(`{}`), nil
}

func (a *fakeAPI) CancelBooking(context.Context, thirdspace.ID, bool) (json.RawMessage, error) {
	a.mutations++
	return json.RawMessage(`{}`), nil
}

func (a *fakeAPI) LeaveWaitlist(context.Context, thirdspace.ID) (json.RawMessage, error) {
	a.mutations++
	return json.RawMessage(`{}`), nil
}

func TestSavedClubDefaultsScopeInitialClassRequest(t *testing.T) {
	client := &fakeAPI{}
	backend := &fakeBackend{client: client}
	m := newTestModel(backend, []string{"2", "1"})
	m.client = client

	message := m.requestClasses()()
	loaded, ok := message.(classesLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("classes command returned %#v", message)
	}
	if len(client.searches) != 1 {
		t.Fatalf("searches = %d, want 1", len(client.searches))
	}
	if got, want := client.searches[0].ClubIDs, []string{"1", "2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("club IDs = %#v, want %#v", got, want)
	}
}

func TestClassAndTrainerFiltersCombine(t *testing.T) {
	backend := &fakeBackend{}
	m := newTestModel(backend, nil)
	m.classes = []thirdspace.Class{
		{ID: "1", Name: "Yoga Flow", TrainerName: "Alex", ClubName: "City"},
		{ID: "2", Name: "Yoga Flow", TrainerName: "Sam", ClubName: "City"},
		{ID: "3", Name: "HIIT", TrainerName: "Alex", ClubName: "Soho"},
	}
	m.classFilterInput.SetValue("yoga")
	m.trainerFilterInput.SetValue("alex")
	m.overlay = overlayFilter

	next, _ := m.handleFilterKey(keyPress("enter"))
	m = next.(model)
	if len(m.visible) != 1 || m.visible[0].ID != "1" {
		t.Fatalf("visible classes = %#v", m.visible)
	}
	if !strings.Contains(m.filterSummary(), "class \"yoga\" AND trainer \"alex\"") {
		t.Fatalf("filter summary = %q", m.filterSummary())
	}
}

func TestBookingsAlsoRespectSelectedClubs(t *testing.T) {
	m := newTestModel(&fakeBackend{}, []string{"1"})
	m.page = pageBookings
	m.bookings = []thirdspace.Class{
		{ID: "1", Name: "Yoga", TrainerName: "Alex", ClubID: "1", ClubName: "City"},
		{ID: "2", Name: "Yoga", TrainerName: "Alex", ClubID: "2", ClubName: "Soho"},
	}
	m.classFilterQuery = "yoga"
	m.trainerFilterQuery = "alex"
	m.refreshRows()
	if len(m.visible) != 1 || m.visible[0].ID != "1" {
		t.Fatalf("visible bookings = %#v", m.visible)
	}
}

func TestClubSelectionPersistsThroughSharedDefaults(t *testing.T) {
	client := &fakeAPI{}
	backend := &fakeBackend{client: client}
	m := newTestModel(backend, []string{"1"})
	m.client = client
	m.clubs = []thirdspace.Club{{ID: "1", Name: "City"}, {ID: "2", Name: "Soho"}}
	m.openClubPicker()
	m.clubDraft["1"] = false
	m.clubDraft["2"] = true

	next, command := m.handleClubsKey(keyPress("enter"))
	m = next.(model)
	if !m.clubsSaving || command == nil {
		t.Fatal("club selection did not start saving")
	}
	saved := messageFromBatch[clubSelectionSavedMsg](t, command)
	if got, want := backend.savedIDs, [][]string{{"2"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("saved IDs = %#v, want %#v", got, want)
	}

	next, _ = m.Update(saved)
	m = next.(model)
	if m.overlay != overlayNone || !m.selectedClubs["2"] || m.selectedClubs["1"] {
		t.Fatalf("selected clubs were not updated: %#v", m.selectedClubs)
	}
}

func TestStaleClassResponseIsIgnored(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	m.classesGeneration = 2
	m.classesLoading = true
	next, _ := m.Update(classesLoadedMsg{
		generation: 1,
		classes:    []thirdspace.Class{{ID: "stale"}},
	})
	m = next.(model)
	if len(m.classes) != 0 || !m.classesLoading {
		t.Fatalf("stale response changed model: classes=%#v loading=%v", m.classes, m.classesLoading)
	}
}

func TestHiddenConfirmationCannotBeAccepted(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	m.resize(minimumWidth, minimumHeight)
	m.overlay = overlayConfirm
	plan := classaction.Plan{
		Kind: classaction.Cancel,
		Class: thirdspace.Class{
			ID:          "123",
			Name:        strings.Repeat("Very long class name ", 8),
			ClubName:    strings.Repeat("Very long club name ", 8),
			EventAction: "late_cancel",
		},
		ExpectedAction: "late_cancel",
		Late:           true,
	}
	m.confirmation = &plan
	if m.overlayFits() {
		t.Fatal("expected confirmation not to fit at the minimum terminal size")
	}

	next, command := m.handleMainKey(keyPress("enter"))
	m = next.(model)
	if command != nil || m.actionApplying {
		t.Fatal("an invisible confirmation started a mutation")
	}
}

func TestControlCIsIgnoredWhileMutationIsApplying(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	m.actionApplying = true
	next, command := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(model)
	if command != nil || !m.actionApplying {
		t.Fatal("control-c interrupted an in-flight mutation")
	}
}

func TestAllKeysAreIgnoredWhileMutationIsApplying(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	m.actionApplying = true
	m.overlay = overlayConfirm
	m.width = minimumWidth - 1
	m.height = minimumHeight - 1
	next, command := m.handleMainKey(keyPress("q"))
	m = next.(model)
	if command != nil || !m.actionApplying || m.overlay != overlayConfirm {
		t.Fatal("a key changed the UI while a mutation was in flight")
	}
}

func TestConfirmedTUIActionRechecksBeforeMutation(t *testing.T) {
	client := &fakeAPI{details: []thirdspace.Class{
		{ID: "123", Name: "Yoga", EventAction: "join"},
		{ID: "123", Name: "Yoga", EventAction: "wait"},
	}}
	m := newTestModel(&fakeBackend{client: client}, nil)
	m.client = client

	prepared := messageFromBatch[actionPreparedMsg](t, m.prepareAction("123"))
	next, _ := m.Update(prepared)
	m = next.(model)
	if m.overlay != overlayConfirm {
		t.Fatal("prepared action did not open confirmation")
	}

	next, command := m.handleConfirmKey(keyPress("enter"))
	m = next.(model)
	applied := messageFromBatch[actionAppliedMsg](t, command)
	if applied.err == nil || client.mutations != 0 || client.detailCalls != 2 {
		t.Fatalf("apply result err=%v mutations=%d details=%d", applied.err, client.mutations, client.detailCalls)
	}
}

func TestClubRefreshReconcilesOpenPicker(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	m.clubs = []thirdspace.Club{{ID: "1", Name: "City"}, {ID: "2", Name: "Soho"}}
	m.openClubPicker()
	m.clubCursor = 1

	next, _ := m.Update(clubsLoadedMsg{clubs: []thirdspace.Club{{ID: "1", Name: "City"}}})
	m = next.(model)
	if m.clubCursor != 0 || len(m.clubDraft) != 1 {
		t.Fatalf("picker was not reconciled: cursor=%d draft=%#v", m.clubCursor, m.clubDraft)
	}
}

func TestStaleClubResponseIsIgnored(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	m.clubsGeneration = 2
	m.clubsLoading = true
	next, _ := m.Update(clubsLoadedMsg{
		generation: 1,
		clubs:      []thirdspace.Club{{ID: "stale", Name: "Stale"}},
	})
	m = next.(model)
	if len(m.clubs) != 0 || !m.clubsLoading {
		t.Fatalf("stale club response changed model: clubs=%#v loading=%v", m.clubs, m.clubsLoading)
	}
}

func TestParallelRefreshSuccessDoesNotEraseOtherError(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	m.classesGeneration = 1
	next, _ := m.Update(classesLoadedMsg{generation: 1, err: fmt.Errorf("timetable unavailable")})
	m = next.(model)
	m.bookingsGeneration = 1
	next, _ = m.Update(bookingsLoadedMsg{generation: 1, classes: nil})
	m = next.(model)
	if err := m.visibleError(); err == nil || !strings.Contains(err.Error(), "timetable unavailable") {
		t.Fatalf("visible error = %v", err)
	}
}

func TestCatppuccinThemeSelectionAndRowHighlight(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	for _, test := range []struct {
		dark     bool
		name     string
		blue     string
		selected string
	}{
		{dark: false, name: "Latte", blue: "#1e66f5", selected: "#1e66f5"},
		{dark: true, name: "Mocha", blue: "#89b4fa", selected: "#89b4fa"},
	} {
		t.Run(test.name, func(t *testing.T) {
			m.applyTheme(test.dark)
			if m.palette.name != test.name || colorHex(m.palette.blue) != test.blue {
				t.Fatalf("palette = %s blue=%s", m.palette.name, colorHex(m.palette.blue))
			}
			if got := colorHex(m.styles.selected.GetBackground()); got != test.selected {
				t.Fatalf("selected background = %s, want %s", got, test.selected)
			}
			view := m.View()
			if view.BackgroundColor != nil {
				t.Fatalf("view set an explicit background: %v", view.BackgroundColor)
			}
		})
	}
}

func TestStructuralStylesDoNotPaintTextBackgrounds(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	for _, dark := range []bool{false, true} {
		m.applyTheme(dark)
		styles := map[string]lipgloss.Style{
			"base":  m.styles.base,
			"panel": m.styles.panel,
			"modal": m.styles.modal,
			"input": m.styles.input,
		}
		for name, style := range styles {
			if _, ok := style.GetBackground().(lipgloss.NoColor); !ok {
				t.Errorf("%s style has background %v", name, style.GetBackground())
			}
		}
	}
}

func TestResponsiveMainViewFitsTerminal(t *testing.T) {
	m := newTestModel(&fakeBackend{}, nil)
	m.screen = screenMain
	m.connecting = false
	for _, size := range []struct{ width, height int }{{42, 12}, {80, 24}, {120, 30}} {
		m.resize(size.width, size.height)
		view := m.mainView()
		if width := lipgloss.Width(view); width > size.width {
			t.Errorf("%dx%d view width = %d", size.width, size.height, width)
		}
		if height := lipgloss.Height(view); height > size.height {
			t.Errorf("%dx%d view height = %d", size.width, size.height, height)
		}
	}
}

func newTestModel(backend backend, defaultClubIDs []string) model {
	fixedNow := func() time.Time {
		return time.Date(2026, time.August, 3, 10, 0, 0, 0, time.UTC)
	}
	m := newModel(context.Background(), backend, "", defaultClubIDs, fixedNow)
	m.screen = screenMain
	m.connecting = false
	return m
}

func keyPress(value string) tea.KeyPressMsg {
	switch value {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		runes := []rune(value)
		return tea.KeyPressMsg{Text: value, Code: runes[0]}
	}
}

func messageFromBatch[T any](t *testing.T, command tea.Cmd) T {
	t.Helper()
	message := command()
	commands, ok := message.(tea.BatchMsg)
	if !ok {
		if typed, ok := message.(T); ok {
			return typed
		}
		t.Fatalf("command returned %T, want tea.BatchMsg", message)
	}
	for _, subcommand := range commands {
		if typed, ok := subcommand().(T); ok {
			return typed
		}
	}
	t.Fatalf("batch did not contain %T", *new(T))
	var zero T
	return zero
}

func colorHex(value color.Color) string {
	r, g, b, _ := value.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
