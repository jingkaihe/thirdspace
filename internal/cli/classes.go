package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jingkaihe/thirdspace/internal/session"
	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

func (a *app) newClubsCommand() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "clubs",
		Short: "List available clubs",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store, client, err := a.authenticated(command.Context())
			if err != nil {
				return err
			}
			clubs, err := client.Clubs(command.Context())
			if err != nil {
				return err
			}
			thirdspace.SortClubs(clubs)
			if jsonOutput {
				return printJSON(a.stdout, clubs)
			}
			return printClubs(a.stdout, clubs, store.State().DefaultClubIDs)
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "print JSON")
	return command
}

func (a *app) newClubCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "club",
		Short: "Manage default clubs",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		a.newClubChooseCommand(),
		a.newClubShowCommand(),
		a.newClubClearCommand(),
	)
	return command
}

func (a *app) newClubChooseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "choose CLUB_ID[,CLUB_ID]",
		Short: "Choose the clubs used by default for class searches",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			ids, err := parseClubIDs(args[0])
			if err != nil {
				return err
			}
			store, client, err := a.authenticated(command.Context())
			if err != nil {
				return err
			}
			clubs, err := client.Clubs(command.Context())
			if err != nil {
				return err
			}
			selected, err := clubsByID(clubs, ids)
			if err != nil {
				return err
			}
			if err := store.SetDefaultClubIDs(ids); err != nil {
				return err
			}
			fmt.Fprintln(a.stdout, "Default clubs updated:")
			return printClubs(a.stdout, selected, ids)
		},
	}
}

func (a *app) newClubShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "show",
		Aliases: []string{"current"},
		Short:   "Show the clubs used by default for class searches",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := a.openStore()
			if err != nil {
				return err
			}
			ids := store.State().DefaultClubIDs
			if len(ids) == 0 {
				fmt.Fprintln(a.stdout, "No default clubs selected. Class searches include all clubs.")
				return nil
			}
			client, err := session.NewManager(store).Client(command.Context())
			if err != nil {
				return err
			}
			clubs, err := client.Clubs(command.Context())
			if err != nil {
				return err
			}
			selected, err := clubsByID(clubs, ids)
			if err != nil {
				return err
			}
			fmt.Fprintln(a.stdout, "Default clubs:")
			return printClubs(a.stdout, selected, ids)
		},
	}
}

func (a *app) newClubClearCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear the default clubs",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := a.openStore()
			if err != nil {
				return err
			}
			if err := store.SetDefaultClubIDs(nil); err != nil {
				return err
			}
			fmt.Fprintln(a.stdout, "Default clubs cleared. Class searches will include all clubs.")
			return nil
		},
	}
}

func (a *app) newClassesCommand() *cobra.Command {
	var date string
	var days int
	var clubs string
	var name string
	var trainer string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "classes",
		Short: "Search the class timetable",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			startDate, endDate, err := thirdspace.BuildClassDateRange(date, days, time.Now())
			if err != nil {
				return err
			}
			store, client, err := a.authenticated(command.Context())
			if err != nil {
				return err
			}
			clubIDs, err := resolveClassClubIDs(clubs, command.Flags().Changed("club"), store.State().DefaultClubIDs)
			if err != nil {
				return err
			}
			classes, err := client.Classes(command.Context(), thirdspace.ClassSearch{
				StartDate: startDate,
				EndDate:   endDate,
				ClubIDs:   clubIDs,
			})
			if err != nil {
				return err
			}
			classes = filterClasses(classes, name, trainer)
			sortClasses(classes)
			if jsonOutput {
				return printJSON(a.stdout, classes)
			}
			return printClasses(a.stdout, classes)
		},
	}
	command.Flags().StringVar(&date, "date", "", "first timetable date in YYYY-MM-DD format (default: today in London)")
	command.Flags().IntVar(&days, "days", 1, "number of days to retrieve")
	command.Flags().StringVar(&clubs, "club", "", "comma-separated club IDs, or 'all' to ignore saved defaults")
	command.Flags().StringVar(&name, "name", "", "case-insensitive class-name filter")
	command.Flags().StringVar(&trainer, "trainer", "", "case-insensitive trainer-name filter")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print JSON")
	return command
}

func parseClubIDs(value string) ([]string, error) {
	parts := splitCommaValues(value)
	if len(parts) == 0 {
		return nil, fmt.Errorf("at least one club ID is required")
	}
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil || parsed == 0 {
			return nil, fmt.Errorf("club IDs must be positive integers, got %q", part)
		}
		id := strconv.FormatUint(parsed, 10)
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func resolveClassClubIDs(value string, explicit bool, defaults []string) ([]string, error) {
	if !explicit {
		return append([]string(nil), defaults...), nil
	}
	if strings.EqualFold(strings.TrimSpace(value), "all") {
		return nil, nil
	}
	return parseClubIDs(value)
}

func clubsByID(clubs []thirdspace.Club, ids []string) ([]thirdspace.Club, error) {
	available := make(map[string]thirdspace.Club, len(clubs))
	for _, club := range clubs {
		available[string(club.ID)] = club
	}
	selected := make([]thirdspace.Club, 0, len(ids))
	missing := make([]string, 0)
	for _, id := range ids {
		club, exists := available[id]
		if !exists {
			missing = append(missing, id)
			continue
		}
		selected = append(selected, club)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("unknown club ID(s): %s; run 'thirdspace clubs' to list available clubs", strings.Join(missing, ","))
	}
	return selected, nil
}

func (a *app) newClassCommand() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "class CLASS_ID",
		Short: "Show details for one class",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			id, err := thirdspace.ParseID(args[0])
			if err != nil {
				return err
			}
			client, err := a.authenticatedClient(command.Context())
			if err != nil {
				return err
			}
			class, err := client.ClassDetail(command.Context(), id)
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(a.stdout, class)
			}
			printClassDetail(a.stdout, class)
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "print JSON")
	return command
}

func (a *app) newBookingsCommand() *cobra.Command {
	var requestedRange string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "bookings",
		Short: "List your class bookings",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := a.authenticatedClient(command.Context())
			if err != nil {
				return err
			}
			classes, err := client.Bookings(command.Context(), requestedRange)
			if err != nil {
				return err
			}
			sortClasses(classes)
			if jsonOutput {
				return printJSON(a.stdout, classes)
			}
			return printClasses(a.stdout, classes)
		},
	}
	command.Flags().StringVar(&requestedRange, "range", "upcoming", "booking range: upcoming, past, or cancelled")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print JSON")
	return command
}
