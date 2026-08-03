# Third Space CLI

An unofficial Go CLI for browsing and booking Third Space classes. The API contract was reconstructed from the Android member app; it may change when the app is updated.

## Build from source

```bash
go build -o thirdspace ./cmd/thirdspace
```

Development, testing, versioning, and release instructions are documented in [`AGENTS.md`](AGENTS.md).

## Authenticate

```bash
./thirdspace login --email you@example.com
```

The password is requested with terminal echo disabled. For scripts, pass it over standard input or through the environment:

```bash
printf '%s\n' "$THIRDSPACE_PASSWORD" | ./thirdspace login --email "$THIRDSPACE_EMAIL" --password-stdin
```

The password is never stored. The resulting session token is written with mode `0600` under your user config directory, normally `~/.config/thirdspace/config.json` on Linux.

Login also obtains a separate refresh token. Commands automatically exchange and rotate it when the session has 72 hours or less remaining, based on the roughly one-month lifetime observed in the Android app. You can rotate both tokens immediately with:

```bash
./thirdspace refresh
```

Use `./thirdspace status` to see token sources and expiry times. Token values are never displayed.

## Browse classes

List club IDs:

```bash
./thirdspace clubs
```

Choose the clubs used by default for class searches:

```bash
./thirdspace club choose 1,2
./thirdspace club show
./thirdspace club clear
```

List today's classes in the saved default clubs, or all clubs when no defaults are selected:

```bash
./thirdspace classes
```

Search a seven-day window for Yoga classes at selected clubs:

```bash
./thirdspace classes --date 2026-08-04 --days 7 --club 1,2 --name yoga
```

An explicit `--club` overrides the saved defaults for one search. Use `--club all` to search every club without clearing the saved selection.

Dates are interpreted in the `Europe/London` timezone. Add `--json` to `clubs`, `classes`, `class`, or `bookings` for machine-readable output.

## Interactive TUI

Launch the full-screen terminal interface with:

```bash
./thirdspace tui
```

The TUI uses the saved credentials when available and otherwise presents a password-masked login form. Its timetable starts with the same default clubs managed by `thirdspace club choose`. Changing clubs with `c` updates that shared saved preference, so later CLI and TUI searches use the same selection.

Class filters are combined using AND semantics:

1. The selected clubs scope the timetable request sent to the API and locally constrain the Bookings tab.
2. The class-name field is applied when non-empty.
3. The trainer field is applied when non-empty.

For example, selecting City and Soho, then filtering for class `yoga` and trainer `alex`, shows only Alex's Yoga classes at those two clubs. Press `/` to edit the separate class and trainer fields.

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

The interface automatically selects Catppuccin Latte for terminals reporting a light background and Catppuccin Mocha for terminals reporting a dark background. It adapts columns and the detail pane to the available terminal size.

## Book and manage classes

```bash
./thirdspace class 123456
./thirdspace book 123456
./thirdspace waitlist 123456
./thirdspace bookings
./thirdspace cancel 123456
./thirdspace leave-waitlist 123456
```

Mutating commands verify the server-provided action, display the class, and ask for confirmation. For non-interactive use, explicitly add `--yes`:

```bash
./thirdspace book --yes 123456
```

For cancellation, the CLI derives `lateBooking` from the class action returned by the server. It prints a warning before a late cancellation that may incur a penalty.

## Environment variables

| Variable | Purpose |
| --- | --- |
| `THIRDSPACE_EMAIL` | Default login email |
| `THIRDSPACE_PASSWORD` | Login password; not persisted |
| `THIRDSPACE_TOKEN` | Override the stored session token |
| `THIRDSPACE_BASE_URL` | Override the API base URL; only the production API or a loopback test server is accepted |
| `THIRDSPACE_CONFIG` | Override the config file path |

All commands also accept a persistent `--config PATH` flag. Cobra provides command-specific help, for example `./thirdspace classes --help`.

## Safety and limitations

- Booking, waitlisting, cancellation, and waitlist removal require a verified server-provided class action and terminal confirmation unless `--yes` is supplied.
- Every mutation rechecks the server action immediately before submission, including `--yes` commands and confirmed TUI actions. A class changing from normal to late cancellation is not submitted under stale assumptions.
- The CLI does not bypass Third Space booking windows, class capacity, waitlist rules, account restrictions, cancellation policies, or penalties; the server remains authoritative.
- Automatic refresh requires the stored refresh token to remain valid. If both credentials expire, run `thirdspace login` again.
- This project is based on an undocumented private API and is not affiliated with Third Space or Fisikal.

## License

The original source code in this repository is licensed under the MIT License. See `LICENSE`.

The license applies only to this project's original source code. It does not grant or purport to grant rights to Third Space or Fisikal trademarks, services, APIs, applications, APKs, or decompiled materials.
