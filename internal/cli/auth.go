package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jingkaihe/thirdspace/internal/session"
)

func (a *app) newLoginCommand() *cobra.Command {
	var email string
	var passwordStdin bool
	command := &cobra.Command{
		Use:   "login",
		Short: "Log in and save session credentials",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := a.openStore()
			if err != nil {
				return err
			}
			state := store.State()
			email = strings.TrimSpace(firstNonEmpty(email, state.Email))
			if email == "" {
				return fmt.Errorf("email is required; pass --email or set THIRDSPACE_EMAIL")
			}
			password, err := a.readPassword(passwordStdin, state.Password)
			if err != nil {
				return err
			}
			if password == "" {
				return fmt.Errorf("password is required")
			}

			result, err := session.NewManager(store).Login(command.Context(), email, password)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "Logged in as %s. Session saved to %s\n", email, store.Path())
			fmt.Fprintf(a.stdout, "Session expires: %s\n", result.Session.ExpiresAt)
			if result.RefreshWarning != nil {
				fmt.Fprintf(a.stderr, "warning: login succeeded, but automatic refresh is unavailable: %v\n", result.RefreshWarning)
			} else {
				fmt.Fprintf(a.stdout, "Refresh token expires: %s\n", result.Refresh.ExpiresAt)
			}
			return nil
		},
	}
	command.Flags().StringVar(&email, "email", "", "Third Space account email")
	command.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from standard input")
	return command
}

func (a *app) newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored session credentials",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := a.openStore()
			if err != nil {
				return err
			}
			if err := store.Clear(); err != nil {
				return err
			}
			fmt.Fprintln(a.stdout, "Stored Third Space session removed.")
			return nil
		},
	}
}

func (a *app) newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local authentication status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := a.openStore()
			if err != nil {
				return err
			}
			state := store.State()
			if state.Session.Value == "" && state.Refresh.Value == "" {
				fmt.Fprintln(a.stdout, "Not logged in. Run: thirdspace login --email you@example.com")
				return nil
			}

			if state.Session.Value != "" {
				source := "stored config"
				if state.SessionFromEnvironment {
					source = "THIRDSPACE_TOKEN"
				}
				fmt.Fprintf(a.stdout, "Authenticated via %s\n", source)
				if state.Session.ExpiresAt != "" {
					fmt.Fprintf(a.stdout, "Session expires: %s\n", state.Session.ExpiresAt)
				}
			} else {
				fmt.Fprintln(a.stdout, "No current session token; a refresh token is available.")
			}
			if state.Email != "" && (!state.SessionFromEnvironment || state.EmailFromEnvironment) {
				fmt.Fprintf(a.stdout, "Account: %s\n", state.Email)
			}
			if state.Refresh.Value == "" {
				fmt.Fprintln(a.stdout, "Automatic refresh: unavailable")
				return nil
			}
			fmt.Fprintln(a.stdout, "Refresh token: available via stored config")
			if state.Refresh.ExpiresAt != "" {
				fmt.Fprintf(a.stdout, "Refresh token expires: %s\n", state.Refresh.ExpiresAt)
			}
			return nil
		},
	}
}

func (a *app) newRefreshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Refresh and rotate stored credentials now",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := a.openStore()
			if err != nil {
				return err
			}
			result, err := session.NewManager(store).ForceRefresh(command.Context())
			if err != nil {
				return err
			}
			fmt.Fprintln(a.stdout, "Credentials updated.")
			fmt.Fprintf(a.stdout, "Session expires: %s\n", result.Session.ExpiresAt)
			if result.RefreshWarning != nil {
				fmt.Fprintf(a.stderr, "warning: session refresh succeeded, but refresh-token rotation is temporarily unavailable: %v\n", result.RefreshWarning)
			} else {
				fmt.Fprintf(a.stdout, "Refresh token expires: %s\n", result.Refresh.ExpiresAt)
			}
			return nil
		},
	}
}

func (a *app) readPassword(fromStdin bool, environmentPassword string) (string, error) {
	if fromStdin {
		scanner := bufio.NewScanner(a.stdin)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("read password: %w", err)
			}
			return "", fmt.Errorf("standard input contained no password")
		}
		return scanner.Text(), nil
	}
	if environmentPassword != "" {
		return environmentPassword, nil
	}
	if !term.IsTerminal(int(a.stdin.Fd())) {
		return "", fmt.Errorf("no terminal available for a hidden password prompt; set THIRDSPACE_PASSWORD or use --password-stdin")
	}
	fmt.Fprint(a.stderr, "Password: ")
	password, err := term.ReadPassword(int(a.stdin.Fd()))
	fmt.Fprintln(a.stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(password), nil
}
