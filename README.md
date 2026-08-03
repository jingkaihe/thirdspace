# Third Space CLI

An unofficial, unaffiliated Go CLI and TUI for browsing and booking Third Space classes. It uses an undocumented API that may change.

## Build from source

```bash
go build -o thirdspace ./cmd/thirdspace
```

Development and release commands are documented in [`AGENTS.md`](AGENTS.md).

## Authenticate

```bash
thirdspace login --email you@example.com
```

The password is prompted securely. For scripts, pass it through standard input:

```bash
printf '%s\n' "$THIRDSPACE_PASSWORD" | thirdspace login --email "$THIRDSPACE_EMAIL" --password-stdin
```

The password is never stored. Session credentials are saved with mode `0600` in the user config directory and refreshed automatically. Use:

```bash
thirdspace refresh
thirdspace status
```

## Browse classes

```bash
thirdspace clubs
thirdspace club choose 1,2
thirdspace club show
thirdspace club clear
thirdspace classes
```

Search a seven-day window:

```bash
thirdspace classes --date 2026-08-04 --days 7 --club 1,2 --name yoga
```

`classes` uses the saved clubs by default. `--club` overrides them for one search; use `--club all` for every club. Dates use the `Europe/London` timezone. Add `--json` for machine-readable output.

## Interactive TUI

Launch the full-screen terminal interface with:

```bash
thirdspace tui
```

The TUI shares saved credentials and club preferences with the CLI. Club, class-name, and trainer filters combine using AND semantics; press `/` to edit class and trainer filters or `c` to choose and persist clubs.

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Move through classes |
| `[` / `]` | Previous or next timetable window; change booking range on the Bookings tab |
| `w` | Toggle a one-day or seven-day timetable |
| `Tab` | Switch between Timetable and Bookings |
| `/` | Set combined class and trainer filters |
| `c` | Choose and persist default timetable clubs |
| `Enter` | Open full class details |
| `a` | Use the current server-provided class action |
| `r` | Refresh the current data |
| `?` | Show expanded help |
| `q` | Quit |

The interface adapts to the terminal size and automatically selects Catppuccin Latte or Mocha for light or dark terminals.

## Book and manage classes

```bash
thirdspace class 123456
thirdspace book 123456
thirdspace waitlist 123456
thirdspace bookings
thirdspace cancel 123456
thirdspace leave-waitlist 123456
```

Mutating commands recheck the server-provided action and ask for confirmation. For non-interactive use, add `--yes` explicitly:

```bash
thirdspace book --yes 123456
```

Late cancellations display a warning before submission.

## Environment variables

| Variable | Purpose |
| --- | --- |
| `THIRDSPACE_EMAIL` | Default login email |
| `THIRDSPACE_PASSWORD` | Login password; not persisted |
| `THIRDSPACE_TOKEN` | Override the stored session token |
| `THIRDSPACE_BASE_URL` | Override the API base URL; only the production API or a loopback test server is accepted |
| `THIRDSPACE_CONFIG` | Override the config file path |

All commands accept `--config PATH`. Use `thirdspace <command> --help` for command-specific help.

## License

The original source code is licensed under the MIT License; see `LICENSE`. This does not grant rights to Third Space or Fisikal trademarks, services, or materials.
