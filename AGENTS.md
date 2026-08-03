# Development Guide

## Project layout

- Go module: `github.com/jingkaihe/thirdspace`
- CLI entry point: `cmd/thirdspace`
- Cobra commands and terminal output: `internal/cli`
- Bubble Tea terminal interface: `internal/tui`
- Shared class-action validation and recheck lifecycle: `internal/classaction`
- API client and API-facing types: `internal/thirdspace`
- Session login and refresh lifecycle: `internal/session`
- Secure configuration persistence: `internal/config`
- Build version metadata: `internal/version`
- APKs, credentials, generated binaries, and release artifacts are ignored and must not be committed.

## Toolchain

The repository uses Mise to pin Go and GoReleaser and to provide the common development commands:

```bash
mise install
mise tasks ls
```

## Common tasks

```bash
mise run format         # Format project Go files
mise run format-check   # Check formatting without modifying files
mise run test           # Run tests with the race detector
mise run vet            # Run go vet
mise run build          # Build bin/thirdspace
mise run ci             # Run version, formatting, test, vet, and build checks
mise run release-check  # Validate .goreleaser.yaml
mise run snapshot       # Build unpublished archives under dist/
mise run clean          # Remove bin/ and dist/
```

Run `mise run ci` before committing code changes. Use the narrowest relevant test during iteration, then run the complete task before finishing.

## Versioning

- `VERSION.txt` is the single declared project version and must contain semantic version text without a leading `v`.
- `internal/version/version.go` contains the development defaults for the version and Git commit SHA.
- Mise builds inject `VERSION.txt` and the current full commit SHA through linker flags.
- GoReleaser injects the release or snapshot version and full commit SHA into packaged binaries.
- Verify metadata with `./bin/thirdspace --version` after `mise run build`.

## Releases

1. Update `VERSION.txt` and commit the change.
2. Run `mise run ci` and `mise run snapshot`.
3. Run `mise run push-tag` to create and push `v$VERSION`.
4. `.github/workflows/release.yml` verifies the tag matches `VERSION.txt` and publishes the release with GoReleaser.

Releases currently contain `tar.gz` archives and checksums for:

- Linux `amd64`
- Linux `arm64`
- macOS `amd64`
- macOS `arm64`

## Safety

- Never commit credentials, local configuration files, APKs, decompiled app material, `bin/`, or `dist/`.
- Do not exercise booking, waitlist, cancellation, or waitlist-removal API calls during automated or live validation. Use read-only commands for smoke tests.
- Preserve the `0600` permissions used for stored credentials.
- Treat the API as undocumented and server-authoritative; do not add behavior that attempts to bypass booking or cancellation rules.
