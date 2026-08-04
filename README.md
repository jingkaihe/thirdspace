# Third Space CLI

An unofficial, unaffiliated Go CLI and TUI for browsing and booking Third Space classes. It uses an undocumented API that may change.

## Install

### Homebrew

```bash
brew install jingkaihe/essentials/thirdspace
```

### Mise

```bash
mise use -g github:jingkaihe/thirdspace
```

<details>
<summary><strong>GitHub release</strong></summary>

Download the latest macOS or Linux archive with `curl`:

```bash
VERSION="$(curl -qfsSL https://api.github.com/repos/jingkaihe/thirdspace/releases/latest | awk -F'"' '/tag_name/{sub(/^v/, "", $4); print $4}')"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')"
curl -qfLO "https://github.com/jingkaihe/thirdspace/releases/download/v${VERSION}/thirdspace_${VERSION}_${OS}_${ARCH}.tar.gz"
tar -xzf "thirdspace_${VERSION}_${OS}_${ARCH}.tar.gz"
mkdir -p "$HOME/.local/bin"
install -m 0755 thirdspace "$HOME/.local/bin/thirdspace"
```

Ensure `~/.local/bin` is in `PATH`. On macOS, downloading this way from a normal Terminal session does not usually add the quarantine attribute. For a system-wide installation, use `sudo install -m 0755 thirdspace /usr/local/bin/thirdspace` instead.

</details>

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

Press `?` to see shortcuts.

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

The source code is licensed under the MIT License; see `LICENSE`. This does not grant rights to Third Space or Fisikal trademarks, services, or materials.
