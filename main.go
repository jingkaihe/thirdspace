package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"
)

type Config struct {
	BaseURL   string `json:"baseUrl,omitempty"`
	Email     string `json:"email,omitempty"`
	Token     string `json:"token,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin *os.File, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	case "login":
		return runLogin(ctx, args[1:], stdin, stdout, stderr)
	case "logout":
		return runLogout(stdout)
	case "status":
		return runStatus(stdout)
	case "clubs":
		return runClubs(ctx, args[1:], stdout, stderr)
	case "classes":
		return runClasses(ctx, args[1:], stdout, stderr)
	case "class":
		return runClassDetail(ctx, args[1:], stdout, stderr)
	case "bookings":
		return runBookings(ctx, args[1:], stdout, stderr)
	case "book", "waitlist", "cancel", "leave-waitlist":
		return runMutation(ctx, args[0], args[1:], stdin, stdout, stderr)
	default:
		return fmt.Errorf("unknown command %q; run 'thirdspace help'", args[0])
	}
}

func printUsage(output io.Writer) {
	fmt.Fprint(output, `Third Space class booking CLI

Usage:
  thirdspace login [--email EMAIL] [--password-stdin]
  thirdspace logout
  thirdspace status
  thirdspace clubs [--json]
  thirdspace classes [--date YYYY-MM-DD] [--days N] [--club ID[,ID]] [--name TEXT] [--trainer TEXT] [--json]
  thirdspace class [--json] CLASS_ID
  thirdspace bookings [--range upcoming|past|cancelled] [--json]
  thirdspace book [--yes] CLASS_ID
  thirdspace waitlist [--yes] CLASS_ID
  thirdspace cancel [--yes] CLASS_ID
  thirdspace leave-waitlist [--yes] CLASS_ID

Environment:
  THIRDSPACE_EMAIL       Default login email
  THIRDSPACE_PASSWORD    Login password; never stored by the CLI
  THIRDSPACE_TOKEN       Override the stored session token
  THIRDSPACE_BASE_URL    Override the API base URL
  THIRDSPACE_CONFIG      Override the config file path
`)
}

func runLogin(ctx context.Context, args []string, stdin *os.File, stdout, stderr io.Writer) error {
	set := flag.NewFlagSet("login", flag.ContinueOnError)
	set.SetOutput(stderr)
	emailFlag := set.String("email", "", "Third Space account email")
	passwordStdin := set.Bool("password-stdin", false, "read the password from standard input")
	if err := set.Parse(args); err != nil {
		return normalizeFlagError(err)
	}
	if set.NArg() != 0 {
		return fmt.Errorf("login does not accept positional arguments")
	}

	path, err := configPath()
	if err != nil {
		return err
	}
	config, err := loadConfig(path)
	if err != nil {
		return err
	}
	email := strings.TrimSpace(firstNonEmpty(*emailFlag, os.Getenv("THIRDSPACE_EMAIL"), config.Email))
	if email == "" {
		return fmt.Errorf("email is required; pass --email or set THIRDSPACE_EMAIL")
	}
	password, err := readPassword(stdin, stderr, *passwordStdin)
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	baseURL := firstNonEmpty(os.Getenv("THIRDSPACE_BASE_URL"), config.BaseURL, defaultBaseURL)
	baseURL, err = validateAPIBaseURL(baseURL)
	if err != nil {
		return err
	}
	client := NewClient(baseURL, "")
	session, err := client.Login(ctx, email, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	config = Config{
		BaseURL:   baseURL,
		Email:     email,
		Token:     session.Token,
		ExpiresAt: session.ExpiresAt,
	}
	if err := saveConfig(path, config); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Logged in as %s. Session saved to %s\n", email, path)
	if session.ExpiresAt != "" {
		fmt.Fprintf(stdout, "Session expires: %s\n", session.ExpiresAt)
	}
	return nil
}

func readPassword(stdin *os.File, stderr io.Writer, fromStdin bool) (string, error) {
	if fromStdin {
		scanner := bufio.NewScanner(stdin)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("read password: %w", err)
			}
			return "", fmt.Errorf("standard input contained no password")
		}
		return scanner.Text(), nil
	}
	if password := os.Getenv("THIRDSPACE_PASSWORD"); password != "" {
		return password, nil
	}
	if !term.IsTerminal(int(stdin.Fd())) {
		return "", fmt.Errorf("no terminal available for a hidden password prompt; set THIRDSPACE_PASSWORD or use --password-stdin")
	}
	fmt.Fprint(stderr, "Password: ")
	password, err := term.ReadPassword(int(stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(password), nil
}

func runLogout(stdout io.Writer) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove config: %w", err)
	}
	fmt.Fprintln(stdout, "Stored Third Space session removed.")
	return nil
}

func runStatus(stdout io.Writer) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	config, err := loadConfig(path)
	if err != nil {
		return err
	}
	tokenSource := "stored config"
	if os.Getenv("THIRDSPACE_TOKEN") != "" {
		tokenSource = "THIRDSPACE_TOKEN"
	}
	if firstNonEmpty(os.Getenv("THIRDSPACE_TOKEN"), config.Token) == "" {
		fmt.Fprintln(stdout, "Not logged in. Run: thirdspace login --email you@example.com")
		return nil
	}
	fmt.Fprintf(stdout, "Authenticated via %s\n", tokenSource)
	if config.Email != "" {
		fmt.Fprintf(stdout, "Account: %s\n", config.Email)
	}
	if config.ExpiresAt != "" {
		fmt.Fprintf(stdout, "Session expires: %s\n", config.ExpiresAt)
	}
	return nil
}

func runClubs(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	set := flag.NewFlagSet("clubs", flag.ContinueOnError)
	set.SetOutput(stderr)
	jsonOutput := set.Bool("json", false, "print JSON")
	if err := set.Parse(args); err != nil {
		return normalizeFlagError(err)
	}
	if set.NArg() != 0 {
		return fmt.Errorf("clubs does not accept positional arguments")
	}
	client, err := authenticatedClient()
	if err != nil {
		return err
	}
	clubs, err := client.Clubs(ctx)
	if err != nil {
		return err
	}
	sort.Slice(clubs, func(i, j int) bool { return strings.ToLower(clubs[i].Name) < strings.ToLower(clubs[j].Name) })
	if *jsonOutput {
		return printJSON(stdout, clubs)
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tCODE")
	for _, club := range clubs {
		fmt.Fprintf(table, "%s\t%s\t%s\n", club.ID, club.Name, emptyDash(club.Code))
	}
	return table.Flush()
}

func runClasses(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	set := flag.NewFlagSet("classes", flag.ContinueOnError)
	set.SetOutput(stderr)
	date := set.String("date", "", "first timetable date in YYYY-MM-DD format; defaults to today in London")
	days := set.Int("days", 1, "number of days to retrieve")
	clubValues := set.String("club", "", "comma-separated club IDs")
	name := set.String("name", "", "case-insensitive class-name filter")
	trainer := set.String("trainer", "", "case-insensitive trainer-name filter")
	jsonOutput := set.Bool("json", false, "print JSON")
	if err := set.Parse(args); err != nil {
		return normalizeFlagError(err)
	}
	if set.NArg() != 0 {
		return fmt.Errorf("classes does not accept positional arguments")
	}
	startDate, endDate, err := BuildClassDateRange(*date, *days, time.Now())
	if err != nil {
		return err
	}
	client, err := authenticatedClient()
	if err != nil {
		return err
	}
	classes, err := client.Classes(ctx, ClassSearch{
		StartDate: startDate,
		EndDate:   endDate,
		ClubIDs:   splitCommaValues(*clubValues),
	})
	if err != nil {
		return err
	}
	classes = filterClasses(classes, *name, *trainer)
	sortClasses(classes)
	if *jsonOutput {
		return printJSON(stdout, classes)
	}
	return printClasses(stdout, classes)
}

func runClassDetail(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	set := flag.NewFlagSet("class", flag.ContinueOnError)
	set.SetOutput(stderr)
	jsonOutput := set.Bool("json", false, "print JSON")
	if err := set.Parse(args); err != nil {
		return normalizeFlagError(err)
	}
	if set.NArg() != 1 {
		return fmt.Errorf("usage: thirdspace class [--json] CLASS_ID")
	}
	id, err := parseID(set.Arg(0))
	if err != nil {
		return err
	}
	client, err := authenticatedClient()
	if err != nil {
		return err
	}
	class, err := client.ClassDetail(ctx, id)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(stdout, class)
	}
	printClassDetail(stdout, class)
	return nil
}

func runBookings(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	set := flag.NewFlagSet("bookings", flag.ContinueOnError)
	set.SetOutput(stderr)
	rangeValue := set.String("range", "upcoming", "upcoming, past, or cancelled")
	jsonOutput := set.Bool("json", false, "print JSON")
	if err := set.Parse(args); err != nil {
		return normalizeFlagError(err)
	}
	if set.NArg() != 0 {
		return fmt.Errorf("bookings does not accept positional arguments")
	}
	client, err := authenticatedClient()
	if err != nil {
		return err
	}
	classes, err := client.Bookings(ctx, *rangeValue)
	if err != nil {
		return err
	}
	sortClasses(classes)
	if *jsonOutput {
		return printJSON(stdout, classes)
	}
	return printClasses(stdout, classes)
}

func runMutation(ctx context.Context, command string, args []string, stdin *os.File, stdout, stderr io.Writer) error {
	var idText string
	yes := false
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			yes = true
		case "-h", "--help":
			fmt.Fprintln(stdout, mutationUsage(command))
			return nil
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown option %q", arg)
			}
			if idText != "" {
				return fmt.Errorf("only one class ID may be supplied")
			}
			idText = arg
		}
	}
	if idText == "" {
		return fmt.Errorf("%s", mutationUsage(command))
	}
	id, err := parseID(idText)
	if err != nil {
		return err
	}
	client, err := authenticatedClient()
	if err != nil {
		return err
	}

	detail, err := client.ClassDetail(ctx, id)
	if err != nil {
		return fmt.Errorf("could not verify class %s before making a change: %w", id, err)
	}
	late, err := validateMutationAction(command, detail.EventAction)
	if err != nil {
		return err
	}
	printClassDetail(stderr, detail)
	if late {
		fmt.Fprintln(stderr, "warning: the app marks this as a late cancellation and a penalty may apply")
	}
	if !yes {
		if !term.IsTerminal(int(stdin.Fd())) {
			return fmt.Errorf("interactive confirmation requires a terminal; inspect the class and rerun with --yes for non-interactive use")
		}
		confirmed, err := confirm(stdin, stderr, mutationPrompt(command, id))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(stdout, "No changes made.")
			return nil
		}
	}

	var result json.RawMessage
	switch command {
	case "book":
		result, err = client.BookClass(ctx, id)
	case "waitlist":
		result, err = client.JoinWaitlist(ctx, id)
	case "cancel":
		result, err = client.CancelBooking(ctx, id, late)
	case "leave-waitlist":
		result, err = client.LeaveWaitlist(ctx, id)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s succeeded for class %s.\n", mutationPastTense(command), id)
	if meaningfulJSON(result) {
		return printJSON(stdout, result)
	}
	return nil
}

func authenticatedClient() (*Client, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	config, err := loadConfig(path)
	if err != nil {
		return nil, err
	}
	token := firstNonEmpty(os.Getenv("THIRDSPACE_TOKEN"), config.Token)
	if token == "" {
		return nil, fmt.Errorf("not logged in; run 'thirdspace login --email you@example.com' or set THIRDSPACE_TOKEN")
	}
	baseURL := firstNonEmpty(os.Getenv("THIRDSPACE_BASE_URL"), config.BaseURL, defaultBaseURL)
	baseURL, err = validateAPIBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return NewClient(baseURL, token), nil
}

func configPath() (string, error) {
	if path := os.Getenv("THIRDSPACE_CONFIG"); path != "" {
		return path, nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(root, "thirdspace", "config.json"), nil
}

func loadConfig(path string) (Config, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("inspect config %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Config{}, fmt.Errorf("refusing non-regular config file %s", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Config{}, fmt.Errorf("refusing config %s with permissions %o; run chmod 600 %q", path, info.Mode().Perm(), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("decode config %s: %w", path, err)
	}
	return config, nil
}

func saveConfig(path string, config Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary config: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure config: %w", err)
	}
	return nil
}

func printClasses(output io.Writer, classes []Class) error {
	if len(classes) == 0 {
		fmt.Fprintln(output, "No classes found.")
		return nil
	}
	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tSTART (LONDON)\tCLUB / ZONE\tCLASS\tTRAINER\tSPACES\tACTION")
	for _, class := range classes {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			class.ID,
			formatClassDate(class.StartDate),
			emptyDash(class.effectiveClubName()),
			emptyDash(class.Name),
			emptyDash(class.TrainerName),
			formatValue(class.Spaces),
			emptyDash(class.EventAction),
		)
	}
	return table.Flush()
}

func printClassDetail(output io.Writer, class Class) {
	fmt.Fprintf(output, "Class %s: %s\n", class.ID, emptyDash(class.Name))
	fmt.Fprintf(output, "  Start:   %s\n", formatClassDate(class.StartDate))
	fmt.Fprintf(output, "  Club:    %s\n", emptyDash(class.effectiveClubName()))
	fmt.Fprintf(output, "  Trainer: %s\n", emptyDash(class.TrainerName))
	if class.EventAction != "" {
		fmt.Fprintf(output, "  Action:  %s\n", class.EventAction)
	}
	if class.Spaces != nil {
		fmt.Fprintf(output, "  Spaces:  %s\n", formatValue(class.Spaces))
	}
}

func filterClasses(classes []Class, name, trainer string) []Class {
	name = strings.ToLower(strings.TrimSpace(name))
	trainer = strings.ToLower(strings.TrimSpace(trainer))
	filtered := make([]Class, 0, len(classes))
	for _, class := range classes {
		if name != "" && !strings.Contains(strings.ToLower(class.Name), name) {
			continue
		}
		if trainer != "" && !strings.Contains(strings.ToLower(class.TrainerName), trainer) {
			continue
		}
		filtered = append(filtered, class)
	}
	return filtered
}

func sortClasses(classes []Class) {
	sort.SliceStable(classes, func(i, j int) bool {
		left, leftErr := parseClassDate(classes[i].StartDate)
		right, rightErr := parseClassDate(classes[j].StartDate)
		if leftErr == nil && rightErr == nil {
			return left.Before(right)
		}
		return classes[i].StartDate < classes[j].StartDate
	})
}

func formatClassDate(value string) string {
	parsed, err := parseClassDate(value)
	if err != nil {
		return emptyDash(value)
	}
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		return parsed.Format("Mon 02 Jan 2006 15:04")
	}
	return parsed.In(london).Format("Mon 02 Jan 2006 15:04")
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

func printJSON(output io.Writer, value any) error {
	var data []byte
	var err error
	if raw, ok := value.(json.RawMessage); ok {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return fmt.Errorf("decode response JSON: %w", err)
		}
		data, err = json.MarshalIndent(decoded, "", "  ")
	} else {
		data, err = json.MarshalIndent(value, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	_, err = fmt.Fprintln(output, string(data))
	return err
}

func meaningfulJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte("{}"))
}

func confirm(stdin *os.File, output io.Writer, prompt string) (bool, error) {
	fmt.Fprintf(output, "%s [y/N]: ", prompt)
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read confirmation: %w", err)
		}
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

func mutationPrompt(command string, id ID) string {
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

func mutationUsage(command string) string {
	return fmt.Sprintf("usage: thirdspace %s [--yes] CLASS_ID", command)
}

func validateMutationAction(command, action string) (bool, error) {
	wanted := map[string][]string{
		"book":           {"join"},
		"waitlist":       {"wait"},
		"cancel":         {"cancel", "late_cancel"},
		"leave-waitlist": {"leave"},
	}[command]
	for _, allowed := range wanted {
		if action == allowed {
			return action == "late_cancel", nil
		}
	}
	if action == "" {
		action = "unknown"
	}
	return false, fmt.Errorf("cannot run %q: the server reports class action %q", command, action)
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

func splitCommaValues(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func normalizeFlagError(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
