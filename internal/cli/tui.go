package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	tuiapp "github.com/jingkaihe/thirdspace/internal/tui"
)

func (a *app) newTUICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Browse and manage classes interactively",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !term.IsTerminal(int(a.stdin.Fd())) {
				return fmt.Errorf("the TUI requires terminal input")
			}
			output, ok := a.stdout.(interface{ Fd() uintptr })
			if !ok || !term.IsTerminal(int(output.Fd())) {
				return fmt.Errorf("the TUI requires terminal output")
			}
			store, err := a.openStore()
			if err != nil {
				return err
			}
			return tuiapp.Run(command.Context(), store, a.stdin, a.stdout)
		},
	}
}
