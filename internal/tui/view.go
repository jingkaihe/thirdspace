package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jingkaihe/thirdspace/internal/classaction"
	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

const (
	compactWidth  = 88
	minimumWidth  = 42
	minimumHeight = 12
)

type catppuccinPalette struct {
	name      string
	rosewater color.Color
	mauve     color.Color
	red       color.Color
	peach     color.Color
	yellow    color.Color
	green     color.Color
	blue      color.Color
	text      color.Color
	subtext1  color.Color
	subtext0  color.Color
	overlay0  color.Color
	surface2  color.Color
	surface1  color.Color
	surface0  color.Color
	base      color.Color
	mantle    color.Color
	crust     color.Color
}

type styles struct {
	base      lipgloss.Style
	title     lipgloss.Style
	muted     lipgloss.Style
	accent    lipgloss.Style
	selected  lipgloss.Style
	panel     lipgloss.Style
	modal     lipgloss.Style
	input     lipgloss.Style
	error     lipgloss.Style
	success   lipgloss.Style
	warning   lipgloss.Style
	tab       lipgloss.Style
	activeTab lipgloss.Style
	button    lipgloss.Style
	danger    lipgloss.Style
	separator lipgloss.Style
}

func (m model) View() tea.View {
	var content string
	switch m.screen {
	case screenConnecting:
		content = m.centered(m.spinner.View() + " Connecting to Third Space…")
	case screenLogin:
		if m.tooSmall() {
			content = m.smallTerminalView("log in")
		} else {
			content = m.loginView()
		}
	default:
		content = m.mainView()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Third Space"
	view.ForegroundColor = m.palette.text
	return view
}

func (m *model) applyTheme(isDark bool) {
	m.isDark = isDark
	m.palette = catppuccinMocha()
	if !isDark {
		m.palette = catppuccinLatte()
	}
	p := m.palette

	m.styles = styles{
		base:      lipgloss.NewStyle().Foreground(p.text),
		title:     lipgloss.NewStyle().Foreground(p.mauve).Bold(true),
		muted:     lipgloss.NewStyle().Foreground(p.subtext0),
		accent:    lipgloss.NewStyle().Foreground(p.blue).Bold(true),
		selected:  lipgloss.NewStyle().Foreground(p.crust).Background(p.blue).Bold(true),
		panel:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.surface2).Padding(0, 1),
		modal:     lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(p.mauve).Padding(1, 2),
		input:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.surface2).Padding(0, 1),
		error:     lipgloss.NewStyle().Foreground(p.red).Bold(true),
		success:   lipgloss.NewStyle().Foreground(p.green).Bold(true),
		warning:   lipgloss.NewStyle().Foreground(p.yellow).Bold(true),
		tab:       lipgloss.NewStyle().Foreground(p.subtext0).Padding(0, 1),
		activeTab: lipgloss.NewStyle().Foreground(p.crust).Background(p.blue).Bold(true).Padding(0, 1),
		button:    lipgloss.NewStyle().Foreground(p.crust).Background(p.blue).Bold(true).Padding(0, 2),
		danger:    lipgloss.NewStyle().Foreground(p.crust).Background(p.red).Bold(true).Padding(0, 2),
		separator: lipgloss.NewStyle().Foreground(p.surface1),
	}

	tableStyles := table.DefaultStyles()
	tableStyles.Header = lipgloss.NewStyle().Foreground(p.subtext0).Bold(true).Padding(0, 1)
	// Keep cells colorless so the selected-row style can paint across them.
	tableStyles.Cell = lipgloss.NewStyle().Padding(0, 1)
	tableStyles.Selected = m.styles.selected
	m.table.SetStyles(tableStyles)
	m.spinner.Style = m.styles.accent
	helpStyles := help.DefaultStyles(isDark)
	helpStyles.ShortKey = lipgloss.NewStyle().Foreground(p.blue)
	helpStyles.ShortDesc = lipgloss.NewStyle().Foreground(p.subtext0)
	helpStyles.ShortSeparator = lipgloss.NewStyle().Foreground(p.overlay0)
	helpStyles.Ellipsis = lipgloss.NewStyle().Foreground(p.overlay0)
	helpStyles.FullKey = lipgloss.NewStyle().Foreground(p.blue)
	helpStyles.FullDesc = lipgloss.NewStyle().Foreground(p.subtext0)
	helpStyles.FullSeparator = lipgloss.NewStyle().Foreground(p.overlay0)
	m.help.Styles = helpStyles

	inputStyles := textinput.DefaultStyles(isDark)
	inputStyles.Focused.Prompt = m.styles.accent
	inputStyles.Focused.Text = m.styles.base
	inputStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(p.overlay0)
	inputStyles.Focused.Suggestion = lipgloss.NewStyle().Foreground(p.surface2)
	inputStyles.Blurred.Prompt = lipgloss.NewStyle().Foreground(p.overlay0)
	inputStyles.Blurred.Text = lipgloss.NewStyle().Foreground(p.subtext0)
	inputStyles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(p.overlay0)
	inputStyles.Blurred.Suggestion = lipgloss.NewStyle().Foreground(p.overlay0)
	inputStyles.Cursor.Color = p.blue
	m.email.SetStyles(inputStyles)
	m.password.SetStyles(inputStyles)
	m.classFilterInput.SetStyles(inputStyles)
	m.trainerFilterInput.SetStyles(inputStyles)
	m.updateDetailViewport()
}

func (m *model) resize(width, height int) {
	m.width = max(1, width)
	m.height = max(1, height)
	contentWidth := m.width
	m.help.SetWidth(contentWidth)
	m.email.SetWidth(min(54, max(10, contentWidth-8)))
	m.password.SetWidth(min(54, max(10, contentWidth-8)))
	m.classFilterInput.SetWidth(min(70, max(10, contentWidth-10)))
	m.trainerFilterInput.SetWidth(min(70, max(10, contentWidth-10)))

	bodyHeight := m.bodyHeight()
	listWidth := contentWidth
	if m.width >= compactWidth {
		listWidth = max(44, (contentWidth*2)/3)
	}
	tableWidth := max(1, listWidth-m.styles.panel.GetHorizontalFrameSize())
	tableHeight := max(2, bodyHeight-m.styles.panel.GetVerticalFrameSize())
	m.table.SetWidth(tableWidth)
	m.table.SetHeight(tableHeight)
	m.table.SetColumns(classColumns(tableWidth))

	modalWidth := min(max(42, m.width-10), 100)
	modalHeight := min(max(12, m.height-6), m.height-2)
	m.detailViewport.SetWidth(max(10, modalWidth-6))
	m.detailViewport.SetHeight(max(4, modalHeight-8))
	m.updateDetailViewport()
}

func (m model) centered(content string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.styles.base.Render(content))
}

func (m model) loginView() string {
	width := min(68, max(34, m.width-6))
	title := m.styles.title.Render("Third Space")
	subtitle := m.styles.muted.Render("Log in to browse and manage classes")
	emailLabel := m.styles.accent.Render("Email")
	passwordLabel := m.styles.accent.Render("Password")
	message := ""
	if m.loginErr != nil {
		message = "\n" + m.styles.error.Render(m.loginErr.Error())
	}
	button := m.styles.button.Render("Enter  Log in")
	if m.loggingIn {
		button = m.styles.accent.Render(m.spinner.View() + " Logging in…")
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		subtitle,
		"",
		emailLabel,
		m.styles.input.Width(width-6).Render(m.email.View()),
		"",
		passwordLabel,
		m.styles.input.Width(width-6).Render(m.password.View()),
		message,
		"",
		button,
		"",
		m.styles.muted.Render("Tab switches fields · Enter submits · Esc quits"),
	)
	panel := m.styles.modal.Width(width).Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func (m model) mainView() string {
	if m.tooSmall() {
		return m.smallTerminalView("continue")
	}
	header := m.headerView()
	body := m.bodyView()
	status := m.statusView()
	footer := m.help.View(m)
	content := lipgloss.JoinVertical(lipgloss.Left, header, body, status, footer)
	content = m.styles.base.Render(content)

	if m.overlay != overlayNone {
		if !m.overlayFits() {
			return m.smallOverlayView()
		}
		content = m.renderOverlay(content)
	}
	return content
}

func (m model) headerView() string {
	name := m.styles.title.Render("Third Space")
	timetable := m.styles.tab.Render("Timetable")
	bookings := m.styles.tab.Render("Bookings")
	if m.page == pageTimetable {
		timetable = m.styles.activeTab.Render("Timetable")
	} else {
		bookings = m.styles.activeTab.Render("Bookings")
	}
	tabs := lipgloss.JoinHorizontal(lipgloss.Top, timetable, bookings)
	context := m.pageContext()
	left := lipgloss.JoinHorizontal(lipgloss.Top, name, "  ", tabs)
	if m.width < compactWidth {
		return lipgloss.JoinVertical(lipgloss.Left, left, m.styles.muted.Render(context))
	}
	space := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(context)-2)
	return left + strings.Repeat(" ", space) + m.styles.muted.Render(context)
}

func (m model) pageContext() string {
	if m.page == pageBookings {
		return titleCase(bookingRanges[m.bookingRange]) + " bookings"
	}
	end := m.date.AddDate(0, 0, m.days-1)
	date := m.date.Format("Mon 02 Jan 2006")
	if m.days > 1 {
		date = m.date.Format("02 Jan") + " – " + end.Format("02 Jan 2006")
	}
	clubs := "all clubs"
	if len(m.selectedClubs) > 0 {
		clubs = fmt.Sprintf("%d clubs", len(m.selectedClubs))
	}
	return fmt.Sprintf("%s · %s", date, clubs)
}

func (m model) bodyView() string {
	bodyHeight := m.bodyHeight()
	list := m.table.View()
	if len(m.visible) == 0 && !m.busy() {
		list = lipgloss.Place(m.table.Width(), m.table.Height(), lipgloss.Center, lipgloss.Center, m.styles.muted.Render("No classes found"))
	}
	listWidth := m.table.Width() + m.styles.panel.GetHorizontalFrameSize()
	listPanel := m.styles.panel.Width(listWidth).Height(bodyHeight).Render(list)
	if m.width < compactWidth {
		return listPanel
	}
	detailWidth := max(24, m.width-listWidth-1)
	detailPanel := m.styles.panel.Width(detailWidth).Height(bodyHeight).Render(m.compactDetailView())
	return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, " ", detailPanel)
}

func (m model) compactDetailView() string {
	selected := m.selectedClass()
	if selected == nil {
		return m.styles.muted.Render("Select a class to see details")
	}
	action := actionLabel(selected.EventAction)
	lines := []string{
		m.styles.accent.Render(selected.Name),
		m.styles.muted.Render(formatClassDate(selected.StartDate)),
		"",
		"Club:    " + emptyDash(selected.ClubNameWithZone()),
		"Trainer: " + emptyDash(selected.TrainerName),
		"Spaces:  " + formatValue(selected.Spaces),
	}
	if selected.WaitListPosition != nil {
		lines = append(lines, "Waitlist: "+formatValue(selected.WaitListPosition))
	}
	if action != "" {
		lines = append(lines, "", m.styles.button.Render("a  "+action))
	} else {
		lines = append(lines, "", m.styles.muted.Render("No action currently available"))
	}
	lines = append(lines, "", m.styles.muted.Render("Enter opens full details"))
	return strings.Join(lines, "\n")
}

func (m model) statusView() string {
	message := ""
	switch {
	case m.visibleError() != nil:
		message = m.styles.error.Render("Error: " + m.visibleError().Error())
	case m.busy():
		message = m.styles.accent.Render(m.spinner.View() + " " + m.busyLabel())
	case m.classFilterQuery != "" || m.trainerFilterQuery != "":
		message = m.styles.success.Render(m.filterSummary())
	case m.status != "":
		message = m.styles.success.Render(m.status)
	default:
		message = m.styles.muted.Render("Ready")
	}
	width := max(1, m.width-4)
	message = ansi.Truncate(message, width, "…")
	return lipgloss.NewStyle().Width(max(1, m.width-2)).Padding(0, 1).Render(message)
}

func (m model) busyLabel() string {
	switch {
	case m.connecting:
		return "Connecting…"
	case m.loggingIn:
		return "Logging in…"
	case m.actionApplying:
		return "Rechecking and applying change…"
	case m.actionPreparing:
		return "Verifying class action…"
	case m.detailLoading:
		return "Loading class details…"
	case m.classesLoading && m.bookingsLoading:
		return "Refreshing timetable and bookings…"
	case m.classesLoading:
		return "Loading timetable…"
	case m.bookingsLoading:
		return "Loading bookings…"
	case m.clubsLoading:
		return "Loading clubs…"
	case m.clubsSaving:
		return "Saving default clubs…"
	default:
		return "Working…"
	}
}

func (m model) renderOverlay(background string) string {
	modal := m.overlayView()
	if modal == "" {
		return background
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}

func (m model) overlayView() string {
	switch m.overlay {
	case overlayFilter:
		return m.filterView()
	case overlayClubs:
		return m.clubsView()
	case overlayDetail:
		return m.detailView()
	case overlayConfirm:
		return m.confirmView()
	case overlayHelp:
		return m.helpView()
	}
	return ""
}

func (m model) overlayFits() bool {
	modal := m.overlayView()
	return modal == "" || (lipgloss.Width(modal) <= m.width && lipgloss.Height(modal) <= m.height)
}

func (m model) smallTerminalView(action string) string {
	message := fmt.Sprintf("Terminal too small to %s\n\nNeed at least %d columns × %d rows", action, minimumWidth, minimumHeight)
	return m.centered(message)
}

func (m model) smallOverlayView() string {
	return m.centered("Terminal too small to show this dialog\n\nResize to review it, or press Esc to close it")
}

func (m model) filterView() string {
	width := min(76, max(34, m.width-8))
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.title.Render("Filter classes"),
		m.styles.muted.Render("Class, trainer, and club scope are combined"),
		"",
		m.styles.accent.Render("Class name"),
		m.styles.input.Width(width-6).Render(m.classFilterInput.View()),
		"",
		m.styles.accent.Render("Trainer"),
		m.styles.input.Width(width-6).Render(m.trainerFilterInput.View()),
		"",
		m.styles.accent.Render("Club scope"),
		m.styles.muted.Render(m.clubScopeLabel()+" · press c from the timetable to change it"),
		"",
		m.styles.muted.Render("Tab switches fields · Enter applies · Esc cancels · empty fields match all"),
	)
	return m.styles.modal.Width(width).Render(content)
}

func (m model) clubsView() string {
	width := min(68, max(34, m.width-8))
	availableHeight := max(3, min(len(m.clubs), m.height-12))
	start := max(0, m.clubCursor-availableHeight+1)
	end := min(len(m.clubs), start+availableHeight)
	lines := make([]string, 0, availableHeight)
	for index := start; index < end; index++ {
		club := m.clubs[index]
		mark := "[ ]"
		if m.clubDraft[club.ID] {
			mark = "[x]"
		}
		line := fmt.Sprintf("%s %s", mark, club.Name)
		if index == m.clubCursor {
			line = m.styles.selected.Width(width - 8).Render(line)
		}
		lines = append(lines, line)
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.title.Render("Choose clubs"),
		m.styles.muted.Render("An all-selected choice is sent as the API's all-clubs query"),
		"",
		strings.Join(lines, "\n"),
		m.overlayErrorView(),
		"",
		m.styles.muted.Render("j/k move · Space toggles · a selects all · Enter applies · Esc cancels"),
	)
	return m.styles.modal.Width(width).Render(content)
}

func (m model) detailView() string {
	width := min(max(42, m.width-10), 100)
	height := min(max(12, m.height-6), m.height-2)
	footer := m.styles.muted.Render("j/k or arrows scroll · a action · Esc closes")
	content := lipgloss.JoinVertical(lipgloss.Left, m.styles.title.Render("Class details"), m.overlayErrorView(), "", m.detailViewport.View(), "", footer)
	return m.styles.modal.Width(width).Height(height).Render(content)
}

func (m model) confirmView() string {
	if m.confirmation == nil {
		return ""
	}
	plan := m.confirmation
	width := min(70, max(38, m.width-8))
	warning := ""
	button := m.styles.button.Render("y / Enter  Confirm")
	if plan.Late {
		warning = "\n" + m.styles.warning.Render("Late cancellation: a penalty may apply.")
		button = m.styles.danger.Render("y / Enter  Confirm late cancellation")
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.title.Render(plan.Kind.Label()+"?"),
		"",
		m.styles.accent.Render(plan.Class.Name),
		formatClassDate(plan.Class.StartDate),
		emptyDash(plan.Class.ClubNameWithZone()),
		warning,
		"",
		button+"   "+m.styles.muted.Render("n / Esc  Cancel"),
		"",
		m.styles.muted.Render("The class action will be fetched again before submission."),
	)
	if m.actionApplying {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", m.styles.accent.Render(m.spinner.View()+" Rechecking current state…"))
	}
	return m.styles.modal.Width(width).Render(content)
}

func (m model) helpView() string {
	width := min(86, max(38, m.width-8))
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.title.Render("Keyboard help"),
		"",
		m.help.View(m),
		"",
		m.styles.muted.Render("List navigation: j/k or arrows · g/G top/bottom · pgup/pgdn pages"),
		m.styles.muted.Render("Esc, q, or ? closes this help"),
	)
	return m.styles.modal.Width(width).Render(content)
}

func (m *model) updateDetailViewport() {
	if m.detail == nil {
		m.detailViewport.SetContent(m.styles.muted.Render("No class selected"))
		return
	}
	class := *m.detail
	values := []string{
		m.styles.title.Render(emptyDash(class.Name)),
		m.styles.muted.Render("Class " + string(class.ID)),
		"",
		m.styles.accent.Render("Schedule"),
		"Start:    " + formatClassDate(class.StartDate),
		"Duration: " + formatValue(class.Duration),
		"Club:     " + emptyDash(class.ClubNameWithZone()),
		"Trainer:  " + emptyDash(class.TrainerName),
		"",
		m.styles.accent.Render("Availability"),
		"Spaces:           " + formatValue(class.Spaces),
		"Waitlist count:   " + formatValue(class.WaitListCount),
		"Waitlist position:" + formatValue(class.WaitListPosition),
		"Booking opens:    " + formatClassDate(class.BookingOpens),
		"Action:           " + emptyDash(class.EventAction),
	}
	if class.Description != "" {
		values = append(values, "", m.styles.accent.Render("Description"), class.Description)
	}
	if action := actionLabel(class.EventAction); action != "" {
		values = append(values, "", m.styles.button.Render("a  "+action))
	}
	m.detailViewport.SetContent(strings.Join(values, "\n"))
	m.detailViewport.GotoTop()
}

func classColumns(width int) []table.Column {
	timeWidth := 16
	spacesWidth := 6
	clubWidth := 0
	trainerWidth := 0
	actionWidth := 0
	visibleColumns := 3
	if width >= 50 {
		clubWidth = 12
		visibleColumns++
	}
	if width >= 72 {
		clubWidth = 14
		trainerWidth = 12
		visibleColumns++
	}
	if width >= 96 {
		clubWidth = 16
		trainerWidth = 14
		actionWidth = 12
		visibleColumns++
	}
	nameWidth := max(8, width-(visibleColumns*2)-timeWidth-spacesWidth-clubWidth-trainerWidth-actionWidth)
	return []table.Column{
		{Title: "START", Width: timeWidth},
		{Title: "CLASS", Width: nameWidth},
		{Title: "CLUB / ZONE", Width: clubWidth},
		{Title: "TRAINER", Width: trainerWidth},
		{Title: "SPACES", Width: spacesWidth},
		{Title: "ACTION", Width: actionWidth},
	}
}

func classRow(class thirdspace.Class) table.Row {
	return table.Row{
		formatClassDate(class.StartDate),
		emptyDash(class.Name),
		emptyDash(class.ClubNameWithZone()),
		emptyDash(class.TrainerName),
		formatValue(class.Spaces),
		actionLabel(class.EventAction),
	}
}

func actionLabel(action string) string {
	kind, late, err := classaction.KindForEventAction(action)
	if err != nil {
		return ""
	}
	if late {
		return "Cancel (late)"
	}
	return kind.Label()
}

func formatClassDate(value string) string {
	parsed, err := thirdspace.ParseTimestamp(value)
	if err != nil {
		return emptyDash(value)
	}
	london, err := time.LoadLocation("Europe/London")
	if err == nil {
		parsed = parsed.In(london)
	}
	return parsed.Format("Mon 02 Jan 15:04")
}

func formatValue(value any) string {
	if value == nil {
		return "-"
	}
	switch typed := value.(type) {
	case string:
		return emptyDash(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
	}
	return fmt.Sprint(value)
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func titleCase(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (m model) bodyHeight() int {
	return max(4, m.height-lipgloss.Height(m.headerView())-2)
}

func (m model) overlayErrorView() string {
	if m.lastErr == nil {
		return ""
	}
	return "\n" + m.styles.error.Render("Error: "+m.lastErr.Error())
}

func (m model) filterSummary() string {
	parts := make([]string, 0, 3)
	if m.classFilterQuery != "" {
		parts = append(parts, fmt.Sprintf("class %q", m.classFilterQuery))
	}
	if m.trainerFilterQuery != "" {
		parts = append(parts, fmt.Sprintf("trainer %q", m.trainerFilterQuery))
	}
	parts = append(parts, m.clubScopeLabel())
	return fmt.Sprintf("Filters: %s · %d matches", strings.Join(parts, " AND "), len(m.visible))
}

func (m model) clubScopeLabel() string {
	if len(m.selectedClubs) == 0 {
		return "all clubs"
	}
	names := make([]string, 0, len(m.selectedClubs))
	for _, club := range m.clubs {
		if m.selectedClubs[club.ID] {
			names = append(names, club.Name)
		}
	}
	if len(names) == len(m.selectedClubs) && len(names) <= 3 {
		return "clubs " + strings.Join(names, ", ")
	}
	return fmt.Sprintf("%d saved default clubs", len(m.selectedClubs))
}

func catppuccinLatte() catppuccinPalette {
	// Catppuccin palette v1.8.0: https://github.com/catppuccin/palette
	return catppuccinPalette{
		name: "Latte", rosewater: lipgloss.Color("#dc8a78"), mauve: lipgloss.Color("#8839ef"), red: lipgloss.Color("#d20f39"), peach: lipgloss.Color("#fe640b"),
		yellow: lipgloss.Color("#df8e1d"), green: lipgloss.Color("#40a02b"), blue: lipgloss.Color("#1e66f5"), text: lipgloss.Color("#4c4f69"),
		subtext1: lipgloss.Color("#5c5f77"), subtext0: lipgloss.Color("#6c6f85"), overlay0: lipgloss.Color("#9ca0b0"), surface2: lipgloss.Color("#acb0be"),
		surface1: lipgloss.Color("#bcc0cc"), surface0: lipgloss.Color("#ccd0da"), base: lipgloss.Color("#eff1f5"), mantle: lipgloss.Color("#e6e9ef"), crust: lipgloss.Color("#dce0e8"),
	}
}

func catppuccinMocha() catppuccinPalette {
	return catppuccinPalette{
		name: "Mocha", rosewater: lipgloss.Color("#f5e0dc"), mauve: lipgloss.Color("#cba6f7"), red: lipgloss.Color("#f38ba8"), peach: lipgloss.Color("#fab387"),
		yellow: lipgloss.Color("#f9e2af"), green: lipgloss.Color("#a6e3a1"), blue: lipgloss.Color("#89b4fa"), text: lipgloss.Color("#cdd6f4"),
		subtext1: lipgloss.Color("#bac2de"), subtext0: lipgloss.Color("#a6adc8"), overlay0: lipgloss.Color("#6c7086"), surface2: lipgloss.Color("#585b70"),
		surface1: lipgloss.Color("#45475a"), surface0: lipgloss.Color("#313244"), base: lipgloss.Color("#1e1e2e"), mantle: lipgloss.Color("#181825"), crust: lipgloss.Color("#11111b"),
	}
}
