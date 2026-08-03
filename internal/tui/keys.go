package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit       key.Binding
	Help       key.Binding
	SwitchPage key.Binding
	Filter     key.Binding
	Clubs      key.Binding
	Refresh    key.Binding
	Previous   key.Binding
	Next       key.Binding
	Window     key.Binding
	Details    key.Binding
	Action     key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		SwitchPage: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "timetable/bookings"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Clubs: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "clubs"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Previous: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "previous"),
		),
		Next: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next"),
		),
		Window: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "1/7 days"),
		),
		Details: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "details"),
		),
		Action: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "class action"),
		),
	}
}
