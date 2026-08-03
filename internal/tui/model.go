package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/jingkaihe/thirdspace/internal/classaction"
	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

type screen int

const (
	screenConnecting screen = iota
	screenLogin
	screenMain
)

type page int

const (
	pageTimetable page = iota
	pageBookings
)

type overlay int

const (
	overlayNone overlay = iota
	overlayFilter
	overlayClubs
	overlayDetail
	overlayConfirm
	overlayHelp
)

var bookingRanges = []string{"upcoming", "past", "cancelled"}

type model struct {
	ctx     context.Context
	backend backend
	client  api
	now     func() time.Time

	screen  screen
	page    page
	overlay overlay
	keys    keyMap

	width   int
	height  int
	isDark  bool
	palette catppuccinPalette
	styles  styles

	email      textinput.Model
	password   textinput.Model
	loginFocus int
	loginErr   error
	connecting bool
	loggingIn  bool

	table              table.Model
	help               help.Model
	spinner            spinner.Model
	classFilterInput   textinput.Model
	trainerFilterInput textinput.Model
	filterFocus        int
	detailViewport     viewport.Model

	date               time.Time
	days               int
	bookingRange       int
	classFilterQuery   string
	trainerFilterQuery string

	clubs         []thirdspace.Club
	selectedClubs map[thirdspace.ID]bool
	clubDraft     map[thirdspace.ID]bool
	clubCursor    int

	classes  []thirdspace.Class
	bookings []thirdspace.Class
	visible  []thirdspace.Class
	detail   *thirdspace.Class

	confirmation *classaction.Plan
	status       string
	lastErr      error
	clubsErr     error
	classesErr   error
	bookingsErr  error

	clubsLoading    bool
	clubsSaving     bool
	classesLoading  bool
	bookingsLoading bool
	detailLoading   bool
	actionPreparing bool
	actionApplying  bool

	clubsGeneration    int
	classesGeneration  int
	bookingsGeneration int
	detailGeneration   int
}

type connectedMsg struct {
	client api
	err    error
}

type loginFinishedMsg struct {
	result loginResult
	err    error
}

type clubsLoadedMsg struct {
	generation int
	clubs      []thirdspace.Club
	err        error
}

type classesLoadedMsg struct {
	generation int
	classes    []thirdspace.Class
	err        error
}

type bookingsLoadedMsg struct {
	generation int
	classes    []thirdspace.Class
	err        error
}

type detailLoadedMsg struct {
	generation int
	detail     thirdspace.Class
	err        error
}

type actionPreparedMsg struct {
	plan classaction.Plan
	err  error
}

type actionAppliedMsg struct {
	plan classaction.Plan
	err  error
}

type clubSelectionSavedMsg struct {
	ids []string
	err error
}

func newModel(ctx context.Context, backend backend, initialEmail string, defaultClubIDs []string, now func() time.Time) model {
	email := textinput.New()
	email.Prompt = ""
	email.Placeholder = "member@example.com"
	email.CharLimit = 320
	email.SetValue(strings.TrimSpace(initialEmail))

	password := textinput.New()
	password.Prompt = ""
	password.Placeholder = "Password"
	password.EchoMode = textinput.EchoPassword
	password.EchoCharacter = '•'

	classFilter := textinput.New()
	classFilter.Prompt = ""
	classFilter.Placeholder = "e.g. Yoga"

	trainerFilter := textinput.New()
	trainerFilter.Prompt = ""
	trainerFilter.Placeholder = "e.g. Alex"

	tableModel := table.New(table.WithFocused(true))
	detailViewport := viewport.New()
	detailViewport.SoftWrap = true
	detailViewport.FillHeight = true

	location, err := time.LoadLocation("Europe/London")
	if err != nil {
		location = time.UTC
	}
	today := now().In(location)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location)

	selectedClubs := make(map[thirdspace.ID]bool, len(defaultClubIDs))
	for _, id := range defaultClubIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			selectedClubs[thirdspace.ID(id)] = true
		}
	}

	m := model{
		ctx:                ctx,
		backend:            backend,
		now:                now,
		screen:             screenConnecting,
		keys:               defaultKeyMap(),
		width:              100,
		height:             30,
		isDark:             true,
		email:              email,
		password:           password,
		table:              tableModel,
		help:               help.New(),
		spinner:            spinner.New(spinner.WithSpinner(spinner.Dot)),
		classFilterInput:   classFilter,
		trainerFilterInput: trainerFilter,
		detailViewport:     detailViewport,
		date:               today,
		days:               1,
		selectedClubs:      selectedClubs,
		connecting:         true,
	}
	m.applyTheme(true)
	m.resize(m.width, m.height)
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.connectCmd(), m.spinner.Tick, tea.RequestBackgroundColor)
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.BackgroundColorMsg:
		m.applyTheme(msg.IsDark())
		return m, nil
	case spinner.TickMsg:
		if !m.busy() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case connectedMsg:
		return m.handleConnected(msg)
	case loginFinishedMsg:
		return m.handleLoginFinished(msg)
	case clubsLoadedMsg:
		if msg.generation != m.clubsGeneration {
			return m, nil
		}
		m.clubsLoading = false
		if msg.err != nil {
			m.clubsErr = msg.err
			return m, nil
		}
		m.clubsErr = nil
		thirdspace.SortClubs(msg.clubs)
		m.clubs = msg.clubs
		m.clubsErr = m.validateSelectedClubs()
		m.reconcileClubPicker()
		return m, nil
	case classesLoadedMsg:
		if msg.generation != m.classesGeneration {
			return m, nil
		}
		m.classesLoading = false
		if msg.err != nil {
			m.classesErr = msg.err
			return m, nil
		}
		m.classesErr = nil
		thirdspace.SortClasses(msg.classes)
		m.classes = msg.classes
		m.refreshRows()
		if m.status == "" {
			m.status = fmt.Sprintf("Loaded %d classes", len(msg.classes))
		}
		return m, nil
	case bookingsLoadedMsg:
		if msg.generation != m.bookingsGeneration {
			return m, nil
		}
		m.bookingsLoading = false
		if msg.err != nil {
			m.bookingsErr = msg.err
			return m, nil
		}
		m.bookingsErr = nil
		thirdspace.SortClasses(msg.classes)
		m.bookings = msg.classes
		m.refreshRows()
		if m.status == "" {
			m.status = fmt.Sprintf("Loaded %d %s bookings", len(msg.classes), bookingRanges[m.bookingRange])
		}
		return m, nil
	case detailLoadedMsg:
		if msg.generation != m.detailGeneration {
			return m, nil
		}
		m.detailLoading = false
		if msg.err != nil {
			m.lastErr = msg.err
			return m, nil
		}
		m.detail = &msg.detail
		m.updateDetailViewport()
		m.lastErr = nil
		return m, nil
	case actionPreparedMsg:
		m.actionPreparing = false
		if msg.err != nil {
			m.lastErr = msg.err
			m.status = ""
			return m, nil
		}
		m.confirmation = &msg.plan
		m.detail = &msg.plan.Class
		m.updateDetailViewport()
		m.overlay = overlayConfirm
		m.status = ""
		m.lastErr = nil
		return m, nil
	case actionAppliedMsg:
		return m.handleActionApplied(msg)
	case clubSelectionSavedMsg:
		m.clubsSaving = false
		if msg.err != nil {
			m.lastErr = msg.err
			return m, nil
		}
		m.selectedClubs = make(map[thirdspace.ID]bool, len(msg.ids))
		for _, id := range msg.ids {
			m.selectedClubs[thirdspace.ID(id)] = true
		}
		m.clubDraft = nil
		m.overlay = overlayNone
		m.table.Focus()
		if len(msg.ids) == 0 {
			m.status = "Default clubs cleared; timetable includes all clubs"
		} else {
			m.status = fmt.Sprintf("Default clubs updated: %d selected", len(msg.ids))
		}
		m.lastErr = nil
		m.clubsErr = nil
		return m, tea.Batch(m.requestClasses(), m.spinner.Tick)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.actionApplying {
				return m, nil
			}
			return m, tea.Quit
		}
		switch m.screen {
		case screenLogin:
			return m.handleLoginKey(msg)
		case screenMain:
			return m.handleMainKey(msg)
		}
	}
	return m, nil
}

func (m model) handleConnected(msg connectedMsg) (tea.Model, tea.Cmd) {
	m.connecting = false
	if msg.err != nil {
		m.screen = screenLogin
		m.loginErr = msg.err
		if strings.TrimSpace(m.email.Value()) != "" {
			m.loginFocus = 1
		}
		return m, m.focusLoginField()
	}
	m.client = msg.client
	m.screen = screenMain
	m.status = "Authenticated"
	return m, m.loadInitialCmd()
}

func (m model) handleLoginFinished(msg loginFinishedMsg) (tea.Model, tea.Cmd) {
	m.loggingIn = false
	if msg.err != nil {
		m.loginErr = msg.err
		m.loginFocus = 1
		return m, m.focusLoginField()
	}
	m.client = msg.result.client
	m.screen = screenMain
	m.loginErr = nil
	if msg.result.warning != nil {
		m.status = "Logged in; automatic refresh is unavailable: " + msg.result.warning.Error()
	} else {
		m.status = "Logged in"
	}
	return m, m.loadInitialCmd()
}

func (m model) handleLoginKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.tooSmall() {
		if msg.String() == "esc" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.loggingIn {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		return m, tea.Quit
	case "tab", "down":
		m.loginFocus = (m.loginFocus + 1) % 2
		return m, m.focusLoginField()
	case "shift+tab", "up":
		m.loginFocus = (m.loginFocus + 1) % 2
		return m, m.focusLoginField()
	case "enter":
		if m.loginFocus == 0 {
			m.loginFocus = 1
			return m, m.focusLoginField()
		}
		return m, m.submitLogin()
	}

	var cmd tea.Cmd
	if m.loginFocus == 0 {
		m.email, cmd = m.email.Update(msg)
	} else {
		m.password, cmd = m.password.Update(msg)
	}
	return m, cmd
}

func (m model) handleMainKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.actionApplying {
		return m, nil
	}
	if m.overlay != overlayNone && !m.overlayFits() {
		switch msg.String() {
		case "esc", "q", "n":
			m.closeOverlay()
		}
		return m, nil
	}
	if m.tooSmall() {
		if m.overlay != overlayNone {
			switch msg.String() {
			case "esc", "n", "q":
				m.overlay = overlayNone
				m.confirmation = nil
				m.clubDraft = nil
				m.table.Focus()
			}
			return m, nil
		}
		if msg.String() == "q" {
			return m, tea.Quit
		}
		return m, nil
	}

	switch m.overlay {
	case overlayFilter:
		return m.handleFilterKey(msg)
	case overlayClubs:
		return m.handleClubsKey(msg)
	case overlayDetail:
		return m.handleDetailKey(msg)
	case overlayConfirm:
		return m.handleConfirmKey(msg)
	case overlayHelp:
		if msg.String() == "esc" || msg.String() == "q" || msg.String() == "?" {
			m.overlay = overlayNone
			m.help.ShowAll = false
			m.table.Focus()
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.overlay = overlayHelp
		m.help.ShowAll = true
		m.table.Blur()
		return m, nil
	case key.Matches(msg, m.keys.SwitchPage):
		if m.page == pageTimetable {
			m.page = pageBookings
			m.refreshRows()
			if len(m.bookings) == 0 && !m.bookingsLoading {
				return m, tea.Batch(m.requestBookings(), m.spinner.Tick)
			}
		} else {
			m.page = pageTimetable
			m.refreshRows()
		}
		return m, nil
	case key.Matches(msg, m.keys.Filter):
		m.overlay = overlayFilter
		m.classFilterInput.SetValue(m.classFilterQuery)
		m.trainerFilterInput.SetValue(m.trainerFilterQuery)
		m.filterFocus = 0
		m.trainerFilterInput.Blur()
		m.table.Blur()
		return m, m.classFilterInput.Focus()
	case key.Matches(msg, m.keys.Clubs):
		if m.page != pageTimetable {
			return m, nil
		}
		if len(m.clubs) == 0 {
			m.lastErr = fmt.Errorf("club list is not available yet")
			return m, nil
		}
		m.openClubPicker()
		return m, nil
	case key.Matches(msg, m.keys.Refresh):
		m.clearFeedback()
		if m.page == pageTimetable {
			return m, tea.Batch(m.requestClasses(), m.requestClubs(), m.spinner.Tick)
		}
		return m, tea.Batch(m.requestBookings(), m.spinner.Tick)
	case key.Matches(msg, m.keys.Previous):
		m.clearFeedback()
		if m.page == pageTimetable {
			m.date = m.date.AddDate(0, 0, -m.days)
			return m, tea.Batch(m.requestClasses(), m.spinner.Tick)
		}
		m.moveBookingRange(-1)
		return m, tea.Batch(m.requestBookings(), m.spinner.Tick)
	case key.Matches(msg, m.keys.Next):
		m.clearFeedback()
		if m.page == pageTimetable {
			m.date = m.date.AddDate(0, 0, m.days)
			return m, tea.Batch(m.requestClasses(), m.spinner.Tick)
		}
		m.moveBookingRange(1)
		return m, tea.Batch(m.requestBookings(), m.spinner.Tick)
	case key.Matches(msg, m.keys.Window):
		if m.page == pageTimetable {
			if m.days == 1 {
				m.days = 7
			} else {
				m.days = 1
			}
			m.clearFeedback()
			return m, tea.Batch(m.requestClasses(), m.spinner.Tick)
		}
		return m, nil
	case key.Matches(msg, m.keys.Details):
		return m, m.openDetails()
	case key.Matches(msg, m.keys.Action):
		selected := m.selectedClass()
		if selected == nil {
			return m, nil
		}
		return m, m.prepareAction(selected.ID)
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.classFilterInput.SetValue(m.classFilterQuery)
		m.trainerFilterInput.SetValue(m.trainerFilterQuery)
		m.classFilterInput.Blur()
		m.trainerFilterInput.Blur()
		m.overlay = overlayNone
		m.table.Focus()
		return m, nil
	case "tab", "down", "shift+tab", "up":
		m.classFilterInput.Blur()
		m.trainerFilterInput.Blur()
		m.filterFocus = (m.filterFocus + 1) % 2
		if m.filterFocus == 0 {
			return m, m.classFilterInput.Focus()
		}
		return m, m.trainerFilterInput.Focus()
	case "enter":
		m.classFilterQuery = strings.TrimSpace(m.classFilterInput.Value())
		m.trainerFilterQuery = strings.TrimSpace(m.trainerFilterInput.Value())
		m.classFilterInput.Blur()
		m.trainerFilterInput.Blur()
		m.overlay = overlayNone
		m.table.Focus()
		m.lastErr = nil
		m.refreshRows()
		return m, nil
	}
	var cmd tea.Cmd
	if m.filterFocus == 0 {
		m.classFilterInput, cmd = m.classFilterInput.Update(msg)
	} else {
		m.trainerFilterInput, cmd = m.trainerFilterInput.Update(msg)
	}
	return m, cmd
}

func (m model) handleClubsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.clubsSaving {
		return m, nil
	}
	switch msg.String() {
	case "esc", "q":
		m.overlay = overlayNone
		m.clubDraft = nil
		m.table.Focus()
		return m, nil
	case "up", "k":
		m.clubCursor = max(0, m.clubCursor-1)
		return m, nil
	case "down", "j":
		m.clubCursor = min(len(m.clubs)-1, m.clubCursor+1)
		return m, nil
	case "space":
		if len(m.clubs) == 0 || m.clubCursor < 0 || m.clubCursor >= len(m.clubs) {
			return m, nil
		}
		club := m.clubs[m.clubCursor]
		m.clubDraft[club.ID] = !m.clubDraft[club.ID]
		return m, nil
	case "a":
		for _, club := range m.clubs {
			m.clubDraft[club.ID] = true
		}
		return m, nil
	case "enter":
		selected := 0
		for _, club := range m.clubs {
			if m.clubDraft[club.ID] {
				selected++
			}
		}
		if selected == 0 {
			m.lastErr = fmt.Errorf("select at least one club")
			return m, nil
		}
		ids := make([]string, 0, selected)
		if selected != len(m.clubs) {
			for _, club := range m.clubs {
				if m.clubDraft[club.ID] {
					ids = append(ids, string(club.ID))
				}
			}
		}
		m.clubsSaving = true
		m.clearFeedback()
		return m, tea.Batch(m.saveClubSelectionCmd(ids), m.spinner.Tick)
	}
	return m, nil
}

func (m model) handleDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.overlay = overlayNone
		m.table.Focus()
		return m, nil
	case "a":
		if m.detail != nil {
			m.overlay = overlayNone
			return m, m.prepareAction(m.detail.ID)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.detailViewport, cmd = m.detailViewport.Update(msg)
	return m, cmd
}

func (m model) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.actionApplying {
		return m, nil
	}
	switch msg.String() {
	case "y", "enter":
		if m.confirmation == nil {
			return m, nil
		}
		plan := *m.confirmation
		m.actionApplying = true
		m.clearFeedback()
		return m, tea.Batch(m.applyActionCmd(plan), m.spinner.Tick)
	case "n", "esc", "q":
		m.confirmation = nil
		m.overlay = overlayNone
		m.status = "No changes made"
		m.table.Focus()
		return m, nil
	}
	return m, nil
}

func (m model) handleActionApplied(msg actionAppliedMsg) (tea.Model, tea.Cmd) {
	m.actionApplying = false
	m.confirmation = nil
	m.overlay = overlayNone
	m.table.Focus()
	if msg.err != nil {
		m.lastErr = msg.err
		return m, nil
	}
	m.lastErr = nil
	m.status = fmt.Sprintf("%s succeeded for class %s", msg.plan.Kind.Label(), msg.plan.Class.ID)
	return m, tea.Batch(m.requestClasses(), m.requestBookings(), m.spinner.Tick)
}

func (m *model) focusLoginField() tea.Cmd {
	m.email.Blur()
	m.password.Blur()
	if m.loginFocus == 0 {
		return m.email.Focus()
	}
	return m.password.Focus()
}

func (m *model) submitLogin() tea.Cmd {
	email := strings.TrimSpace(m.email.Value())
	password := m.password.Value()
	m.password.Reset()
	if email == "" {
		m.loginErr = fmt.Errorf("email is required")
		m.loginFocus = 0
		return m.focusLoginField()
	}
	if password == "" {
		m.loginErr = fmt.Errorf("password is required")
		m.loginFocus = 1
		return m.focusLoginField()
	}
	m.loginErr = nil
	m.loggingIn = true
	return func() tea.Msg {
		result, err := m.backend.Login(m.ctx, email, password)
		return loginFinishedMsg{result: result, err: err}
	}
}

func (m model) connectCmd() tea.Cmd {
	return func() tea.Msg {
		client, err := m.backend.Connect(m.ctx)
		return connectedMsg{client: client, err: err}
	}
}

func (m *model) loadInitialCmd() tea.Cmd {
	return tea.Batch(m.requestClubs(), m.requestClasses(), m.spinner.Tick)
}

func (m *model) requestClubs() tea.Cmd {
	if m.client == nil {
		return nil
	}
	m.clubsGeneration++
	generation := m.clubsGeneration
	m.clubsLoading = true
	m.clubsErr = nil
	client := m.client
	ctx := m.ctx
	return func() tea.Msg {
		clubs, err := client.Clubs(ctx)
		return clubsLoadedMsg{generation: generation, clubs: clubs, err: err}
	}
}

func (m *model) requestClasses() tea.Cmd {
	if m.client == nil {
		return nil
	}
	m.classesGeneration++
	generation := m.classesGeneration
	m.classesLoading = true
	m.classesErr = nil
	start, end, err := thirdspace.BuildClassDateRange(m.date.Format("2006-01-02"), m.days, m.now())
	if err != nil {
		return func() tea.Msg { return classesLoadedMsg{generation: generation, err: err} }
	}
	search := thirdspace.ClassSearch{StartDate: start, EndDate: end, ClubIDs: m.selectedClubIDs()}
	client := m.client
	ctx := m.ctx
	return func() tea.Msg {
		classes, err := client.Classes(ctx, search)
		return classesLoadedMsg{generation: generation, classes: classes, err: err}
	}
}

func (m *model) requestBookings() tea.Cmd {
	if m.client == nil {
		return nil
	}
	m.bookingsGeneration++
	generation := m.bookingsGeneration
	m.bookingsLoading = true
	m.bookingsErr = nil
	rangeValue := bookingRanges[m.bookingRange]
	client := m.client
	ctx := m.ctx
	return func() tea.Msg {
		classes, err := client.Bookings(ctx, rangeValue)
		return bookingsLoadedMsg{generation: generation, classes: classes, err: err}
	}
}

func (m *model) requestDetail(id thirdspace.ID) tea.Cmd {
	m.detailGeneration++
	generation := m.detailGeneration
	m.detailLoading = true
	client := m.client
	ctx := m.ctx
	return func() tea.Msg {
		detail, err := client.ClassDetail(ctx, id)
		return detailLoadedMsg{generation: generation, detail: detail, err: err}
	}
}

func (m *model) prepareAction(id thirdspace.ID) tea.Cmd {
	if m.actionPreparing || m.actionApplying || m.client == nil {
		return nil
	}
	m.actionPreparing = true
	m.status = "Verifying current class action"
	m.lastErr = nil
	client := m.client
	ctx := m.ctx
	return tea.Batch(func() tea.Msg {
		plan, err := classaction.PrepareAvailable(ctx, client, id)
		return actionPreparedMsg{plan: plan, err: err}
	}, m.spinner.Tick)
}

func (m model) applyActionCmd(plan classaction.Plan) tea.Cmd {
	client := m.client
	ctx := m.ctx
	return func() tea.Msg {
		_, err := classaction.RecheckAndApply(ctx, client, plan)
		return actionAppliedMsg{plan: plan, err: err}
	}
}

func (m model) saveClubSelectionCmd(ids []string) tea.Cmd {
	backend := m.backend
	ids = append([]string(nil), ids...)
	return func() tea.Msg {
		err := backend.SetDefaultClubIDs(ids)
		return clubSelectionSavedMsg{ids: ids, err: err}
	}
}

func (m *model) openDetails() tea.Cmd {
	selected := m.selectedClass()
	if selected == nil {
		return nil
	}
	detail := *selected
	m.detail = &detail
	m.updateDetailViewport()
	m.overlay = overlayDetail
	m.table.Blur()
	m.lastErr = nil
	return tea.Batch(m.requestDetail(selected.ID), m.spinner.Tick)
}

func (m *model) openClubPicker() {
	m.clubDraft = make(map[thirdspace.ID]bool, len(m.clubs))
	if len(m.selectedClubs) == 0 {
		for _, club := range m.clubs {
			m.clubDraft[club.ID] = true
		}
	} else {
		for id, selected := range m.selectedClubs {
			m.clubDraft[id] = selected
		}
	}
	m.clubCursor = 0
	m.overlay = overlayClubs
	m.table.Blur()
	m.lastErr = nil
}

func (m *model) reconcileClubPicker() {
	if m.overlay != overlayClubs {
		return
	}
	if len(m.clubs) == 0 {
		m.overlay = overlayNone
		m.clubDraft = nil
		m.clubCursor = 0
		m.lastErr = fmt.Errorf("no clubs are currently available")
		m.table.Focus()
		return
	}
	previous := m.clubDraft
	m.clubDraft = make(map[thirdspace.ID]bool, len(m.clubs))
	for _, club := range m.clubs {
		selected, existed := previous[club.ID]
		if !existed && len(m.selectedClubs) == 0 {
			selected = true
		}
		m.clubDraft[club.ID] = selected
	}
	m.clubCursor = min(max(0, m.clubCursor), len(m.clubs)-1)
}

func (m model) validateSelectedClubs() error {
	if len(m.selectedClubs) == 0 || len(m.clubs) == 0 {
		return nil
	}
	available := make(map[thirdspace.ID]struct{}, len(m.clubs))
	for _, club := range m.clubs {
		available[club.ID] = struct{}{}
	}
	missing := make([]string, 0)
	for id := range m.selectedClubs {
		if _, exists := available[id]; !exists {
			missing = append(missing, string(id))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("saved default club ID(s) no longer available: %s; choose clubs again", strings.Join(missing, ","))
}

func (m *model) refreshRows() {
	selectedID := thirdspace.ID("")
	if selected := m.selectedClass(); selected != nil {
		selectedID = selected.ID
	}
	source := m.classes
	if m.page == pageBookings {
		source = m.bookings
	}
	m.visible = thirdspace.FilterClasses(source, m.classFilterQuery, m.trainerFilterQuery)
	if m.page == pageBookings {
		m.visible = m.filterClassesByClub(m.visible)
	}
	rows := make([]table.Row, 0, len(m.visible))
	for _, class := range m.visible {
		rows = append(rows, classRow(class))
	}
	m.table.SetRows(rows)
	if len(rows) == 0 {
		return
	}
	cursor := 0
	for index, class := range m.visible {
		if class.ID == selectedID {
			cursor = index
			break
		}
	}
	m.table.SetCursor(cursor)
}

func (m model) filterClassesByClub(classes []thirdspace.Class) []thirdspace.Class {
	if len(m.selectedClubs) == 0 {
		return classes
	}
	filtered := make([]thirdspace.Class, 0, len(classes))
	for _, class := range classes {
		if m.selectedClubs[class.ClubID] {
			filtered = append(filtered, class)
		}
	}
	return filtered
}

func (m model) selectedClass() *thirdspace.Class {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.visible) {
		return nil
	}
	return &m.visible[cursor]
}

func (m model) selectedClubIDs() []string {
	if len(m.selectedClubs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(m.selectedClubs))
	for id, selected := range m.selectedClubs {
		if selected {
			ids = append(ids, string(id))
		}
	}
	sort.Strings(ids)
	return ids
}

func (m *model) moveBookingRange(delta int) {
	m.bookingRange = (m.bookingRange + delta + len(bookingRanges)) % len(bookingRanges)
}

func (m *model) clearFeedback() {
	m.status = ""
	m.lastErr = nil
}

func (m model) busy() bool {
	return m.connecting || m.loggingIn || m.clubsLoading || m.clubsSaving || m.classesLoading || m.bookingsLoading || m.detailLoading || m.actionPreparing || m.actionApplying
}

func (m model) tooSmall() bool {
	return m.width < minimumWidth || m.height < minimumHeight
}

func (m model) visibleError() error {
	if m.lastErr != nil {
		return m.lastErr
	}
	if m.page == pageBookings {
		return m.bookingsErr
	}
	if m.classesErr != nil {
		return m.classesErr
	}
	return m.clubsErr
}

func (m *model) closeOverlay() {
	m.overlay = overlayNone
	m.confirmation = nil
	m.clubDraft = nil
	m.classFilterInput.Blur()
	m.trainerFilterInput.Blur()
	m.help.ShowAll = false
	m.table.Focus()
}

func (m model) ShortHelp() []key.Binding {
	bindings := []key.Binding{m.keys.SwitchPage, m.keys.Filter, m.keys.Refresh, m.keys.Details, m.keys.Action, m.keys.Help, m.keys.Quit}
	if m.page == pageTimetable {
		bindings = []key.Binding{m.keys.Previous, m.keys.Next, m.keys.SwitchPage, m.keys.Filter, m.keys.Clubs, m.keys.Action, m.keys.Help, m.keys.Quit}
	}
	return bindings
}

func (m model) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{m.keys.Previous, m.keys.Next, m.keys.Window, m.keys.SwitchPage},
		{m.keys.Filter, m.keys.Clubs, m.keys.Refresh},
		{m.keys.Details, m.keys.Action},
		{m.keys.Help, m.keys.Quit},
	}
}
