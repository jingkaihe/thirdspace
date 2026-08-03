package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jingkaihe/thirdspace/internal/classaction"
	"github.com/jingkaihe/thirdspace/internal/thirdspace"
)

func (a *app) newMutationCommand(name string) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   name + " CLASS_ID",
		Short: mutationDescription(name),
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

			plan, err := classaction.Prepare(command.Context(), client, classaction.Kind(name), id)
			if err != nil {
				return err
			}
			printClassDetail(a.stderr, plan.Class)
			if plan.Late {
				fmt.Fprintln(a.stderr, "warning: the app marks this as a late cancellation and a penalty may apply")
			}

			var result json.RawMessage
			if !yes {
				if !term.IsTerminal(int(a.stdin.Fd())) {
					return fmt.Errorf("interactive confirmation requires a terminal; inspect the class and rerun with --yes for non-interactive use")
				}
				confirmed, err := a.confirm(mutationPrompt(name, id))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(a.stdout, "No changes made.")
					return nil
				}
			}
			result, err = classaction.RecheckAndApply(command.Context(), client, plan)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "%s succeeded for class %s.\n", mutationPastTense(name), id)
			if meaningfulJSON(result) {
				return printJSON(a.stdout, result)
			}
			return nil
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "skip the interactive confirmation")
	return command
}

func validateMutationAction(command, action string) (bool, error) {
	return classaction.Validate(classaction.Kind(command), action)
}

func mutationDescription(command string) string {
	switch command {
	case "book":
		return "Book a class"
	case "waitlist":
		return "Join a class waitlist"
	case "cancel":
		return "Cancel a class booking"
	case "leave-waitlist":
		return "Leave a class waitlist"
	default:
		return "Change a class booking"
	}
}

func mutationPrompt(command string, id thirdspace.ID) string {
	switch command {
	case "book":
		return "Book class " + string(id)
	case "waitlist":
		return "Join the waitlist for class " + string(id)
	case "cancel":
		return "Cancel the booking for class " + string(id)
	case "leave-waitlist":
		return "Leave the waitlist for class " + string(id)
	default:
		return "Apply this change to class " + string(id)
	}
}

func mutationPastTense(command string) string {
	switch command {
	case "book":
		return "Booking"
	case "waitlist":
		return "Joining the waitlist"
	case "cancel":
		return "Cancellation"
	case "leave-waitlist":
		return "Leaving the waitlist"
	default:
		return "Change"
	}
}

func (a *app) confirm(prompt string) (bool, error) {
	fmt.Fprintf(a.stderr, "%s [y/N]: ", prompt)
	scanner := bufio.NewScanner(a.stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read confirmation: %w", err)
		}
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}
