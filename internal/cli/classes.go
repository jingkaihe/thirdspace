package cli

import (
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

func (a *app) newClubsCommand() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "clubs",
		Short: "List available clubs",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := a.authenticatedClient(command.Context())
			if err != nil {
				return err
			}
			clubs, err := client.Clubs(command.Context())
			if err != nil {
				return err
			}
			sort.SliceStable(clubs, func(left, right int) bool {
				return strings.ToLower(clubs[left].Name) < strings.ToLower(clubs[right].Name)
			})
			if jsonOutput {
				return printJSON(a.stdout, clubs)
			}
			return printClubs(a.stdout, clubs)
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "print JSON")
	return command
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
			client, err := a.authenticatedClient(command.Context())
			if err != nil {
				return err
			}
			classes, err := client.Classes(command.Context(), thirdspace.ClassSearch{
				StartDate: startDate,
				EndDate:   endDate,
				ClubIDs:   splitCommaValues(clubs),
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
	command.Flags().StringVar(&clubs, "club", "", "comma-separated club IDs")
	command.Flags().StringVar(&name, "name", "", "case-insensitive class-name filter")
	command.Flags().StringVar(&trainer, "trainer", "", "case-insensitive trainer-name filter")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print JSON")
	return command
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
