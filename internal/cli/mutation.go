package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"third-space-cli/internal/thirdspace"
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

			detail, err := client.ClassDetail(command.Context(), id)
			if err != nil {
				return fmt.Errorf("could not verify class %s before making a change: %w", id, err)
			}
			late, err := validateMutationAction(name, detail.EventAction)
			if err != nil {
				return err
			}
			printClassDetail(a.stderr, detail)
			if late {
				fmt.Fprintln(a.stderr, "warning: the app marks this as a late cancellation and a penalty may apply")
			}

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

				current, err := client.ClassDetail(command.Context(), id)
				if err != nil {
					return fmt.Errorf("could not recheck class %s after confirmation; no change was made: %w", id, err)
				}
				currentLate, err := validateMutationAction(name, current.EventAction)
				if err != nil {
					return fmt.Errorf("class state changed after confirmation; no change was made: %w", err)
				}
				if current.EventAction != detail.EventAction {
					return fmt.Errorf("class action changed from %q to %q while awaiting confirmation; no change was made; rerun the command", detail.EventAction, current.EventAction)
				}
				late = currentLate
			}

			result, err := applyMutation(command.Context(), client, name, id, late)
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

func applyMutation(ctx context.Context, client *thirdspace.Client, command string, id thirdspace.ID, late bool) (json.RawMessage, error) {
	switch command {
	case "book":
		return client.BookClass(ctx, id)
	case "waitlist":
		return client.JoinWaitlist(ctx, id)
	case "cancel":
		return client.CancelBooking(ctx, id, late)
	case "leave-waitlist":
		return client.LeaveWaitlist(ctx, id)
	default:
		return nil, fmt.Errorf("unknown mutation command %q", command)
	}
}

func validateMutationAction(command, action string) (bool, error) {
	var allowed []string
	switch command {
	case "book":
		allowed = []string{"join"}
	case "waitlist":
		allowed = []string{"wait"}
	case "cancel":
		allowed = []string{"cancel", "late_cancel"}
	case "leave-waitlist":
		allowed = []string{"leave"}
	default:
		return false, fmt.Errorf("unknown mutation command %q", command)
	}
	for _, expected := range allowed {
		if action == expected {
			return action == "late_cancel", nil
		}
	}
	if action == "" {
		action = "unknown"
	}
	return false, fmt.Errorf("cannot run %q: the server reports class action %q", command, action)
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
