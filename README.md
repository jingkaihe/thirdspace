# Third Space CLI

An unofficial Go CLI for browsing and booking Third Space classes. The API contract was reconstructed from the Android member app; it may change when the app is updated.

## Build

```bash
go build -o thirdspace .
```

## Authenticate

```bash
./thirdspace login --email you@example.com
```

The password is requested with terminal echo disabled. For scripts, pass it over standard input or through the environment:

```bash
printf '%s\n' "$THIRDSPACE_PASSWORD" | ./thirdspace login --email "$THIRDSPACE_EMAIL" --password-stdin
```

The password is never stored. The resulting session token is written with mode `0600` under your user config directory, normally `~/.config/thirdspace/config.json` on Linux.

## Browse classes

List club IDs:

```bash
./thirdspace clubs
```

List today's classes in all clubs:

```bash
./thirdspace classes
```

Search a seven-day window for Yoga classes at selected clubs:

```bash
./thirdspace classes --date 2026-08-04 --days 7 --club 1,2 --name yoga
```

Dates are interpreted in the `Europe/London` timezone. Add `--json` to `clubs`, `classes`, `class`, or `bookings` for machine-readable output.

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

## Safety and limitations

- Booking, waitlisting, cancellation, and waitlist removal require a verified class action and terminal confirmation unless `--yes` is supplied.
- The CLI does not bypass Third Space booking windows, class capacity, waitlist rules, account restrictions, cancellation policies, or penalties; the server remains authoritative.
- If the API returns `401 Unauthorized`, run `thirdspace login` again.
- This project is based on an undocumented private API and is not affiliated with Third Space or Fisikal.
