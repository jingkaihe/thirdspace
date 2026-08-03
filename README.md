# Third Space CLI

An unofficial Go CLI for browsing and booking Third Space classes. The API contract was reconstructed from the Android member app; it may change when the app is updated.

## Build

```bash
go build -o thirdspace ./cmd/thirdspace
```

The executable lives under `cmd/thirdspace`; API, session, configuration, and Cobra command code is kept in focused packages under `internal/`.

With Mise installed, the common development commands are:

```bash
mise install
mise run version-check
mise run build
mise run test
mise run ci
```

`VERSION.txt` is the single declared project version. Mise injects it and the current Git commit SHA into `bin/thirdspace`; both can be checked with `./bin/thirdspace --version`. Formal GoReleaser builds encode the full commit SHA for the tagged source. Use `mise run snapshot` to create local release archives under `dist/` without publishing them.

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

All commands also accept a persistent `--config PATH` flag. Cobra provides command-specific help, for example `./thirdspace classes --help`.

## Releases

GitHub Actions runs formatting, tests, vet, build, and GoReleaser configuration checks for pushes to `main` and for pull requests. Pushing a version tag publishes Linux and macOS archives for `amd64` and `arm64`, along with checksums, through GoReleaser:

```bash
mise run push-tag
```

The release workflow rejects a tag that does not match `v` followed by the version in `VERSION.txt`.

## Safety and limitations

- Booking, waitlisting, cancellation, and waitlist removal require a verified class action and terminal confirmation unless `--yes` is supplied.
- Interactive mutations recheck the server action after confirmation, so a class changing from normal to late cancellation is not submitted under stale assumptions.
- The CLI does not bypass Third Space booking windows, class capacity, waitlist rules, account restrictions, cancellation policies, or penalties; the server remains authoritative.
- Automatic refresh requires the stored refresh token to remain valid. If both credentials expire, run `thirdspace login` again.
- This project is based on an undocumented private API and is not affiliated with Third Space or Fisikal.
