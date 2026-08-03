package cli

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/jingkaihe/thirdspace/internal/config"
	"github.com/jingkaihe/thirdspace/internal/session"
	"github.com/jingkaihe/thirdspace/internal/thirdspace"
	"github.com/jingkaihe/thirdspace/internal/version"
)

type app struct {
	stdin      *os.File
	stdout     io.Writer
	stderr     io.Writer
	configPath string
}

func Execute(ctx context.Context, args []string, stdin *os.File, stdout, stderr io.Writer) error {
	command := NewCommand(stdin, stdout, stderr)
	command.SetArgs(args)
	return command.ExecuteContext(ctx)
}

func NewCommand(stdin *os.File, stdout, stderr io.Writer) *cobra.Command {
	application := &app{stdin: stdin, stdout: stdout, stderr: stderr}
	command := &cobra.Command{
		Use:           "thirdspace",
		Short:         "Browse and book Third Space classes",
		Version:       version.String(),
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.SetIn(stdin)
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.CompletionOptions.DisableDefaultCmd = true
	command.PersistentFlags().StringVar(&application.configPath, "config", "", "config file (default: $THIRDSPACE_CONFIG or the user config directory)")

	command.AddCommand(
		application.newTUICommand(),
		application.newLoginCommand(),
		application.newLogoutCommand(),
		application.newStatusCommand(),
		application.newRefreshCommand(),
		application.newClubCommand(),
		application.newClubsCommand(),
		application.newClassesCommand(),
		application.newClassCommand(),
		application.newBookingsCommand(),
		application.newMutationCommand("book"),
		application.newMutationCommand("waitlist"),
		application.newMutationCommand("cancel"),
		application.newMutationCommand("leave-waitlist"),
	)
	return command
}

func (a *app) openStore() (*config.Store, error) {
	path, err := config.ResolvePath(a.configPath)
	if err != nil {
		return nil, err
	}
	return config.Open(path, thirdspace.DefaultBaseURL)
}

func (a *app) authenticatedClient(ctx context.Context) (*thirdspace.Client, error) {
	_, client, err := a.authenticated(ctx)
	return client, err
}

func (a *app) authenticated(ctx context.Context) (*config.Store, *thirdspace.Client, error) {
	store, err := a.openStore()
	if err != nil {
		return nil, nil, err
	}
	client, err := session.NewManager(store).Client(ctx)
	if err != nil {
		return nil, nil, err
	}
	return store, client, nil
}
