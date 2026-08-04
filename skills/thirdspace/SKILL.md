---
name: thirdspace
description: Manage Third Space gym classes with the thirdspace CLI.
---

# Third Space CLI

Use the `thirdspace` command as an agent-facing interface to the unofficial Third Space API client. The binary must be available in `PATH`.

## Operating rules

1. Start a workflow with `thirdspace status`. If authentication is missing, follow the non-interactive login guidance below.
2. After authentication, run `thirdspace club show`. Use the saved chosen club or clubs as the default scope for class searches, booking lists, and class actions unless the user explicitly requests another club or all clubs.
3. Use `--json` for every read command that supports it. Extract IDs from returned data; never invent or infer an ID.
4. Resolve relative dates to an explicit `YYYY-MM-DD` date in `Europe/London`, pass that date with `--date`, and tell the user which date was used.
5. Do not rely on interactive terminal prompts. Class mutations require `--yes`, but only after the user has approved the exact action and class.
6. Never print tokens or passwords, inspect the credential file, or ask the user to paste a password into chat. Do not expose secrets in command arguments.
7. Treat the server's `eventAction` as authoritative. Do not attempt to bypass booking, waitlist, or cancellation rules.

## Authentication

Check local session state first:

```bash
thirdspace status
```

If the user has already supplied credentials through secure environment variables, login can run non-interactively:

```bash
printf '%s\n' "$THIRDSPACE_PASSWORD" | thirdspace login --email "$THIRDSPACE_EMAIL" --password-stdin
```

Only run that command when both variables are already present. Otherwise, ask the user to authenticate separately in their terminal. The password is not persisted; session credentials refresh automatically.

Use `thirdspace refresh` only to troubleshoot or when the user explicitly asks for an immediate refresh. Run `thirdspace logout` only on an explicit logout request because it removes stored credentials.

## Clubs and defaults

List clubs as JSON to discover valid club IDs:

```bash
thirdspace clubs --json
```

Relevant fields are `clubId`, `name`, and `code`.

Inspect or change saved default clubs:

```bash
thirdspace club show
thirdspace club choose 1,2
thirdspace club clear
```

The IDs reported by `thirdspace club show` define the normal scope for this skill. If no default club is selected, run `thirdspace clubs --json` and ask the user to choose a club before listing classes or bookings. Only use all clubs when the user explicitly asks for all clubs.

`club choose` and `club clear` persist a user preference, so run them only when requested. Do not change the saved preference merely to satisfy a one-off request for another club.

Class searches use saved defaults when `--club` is omitted. This is the normal behavior. Use `--club CLUB_ID[,CLUB_ID]` only for an explicitly requested one-off override, or `--club all` only for an explicit all-clubs search.

## Search classes

Always set `DATE` to the concrete London calendar date requested by the user. `--days` accepts 1 through 31.

```bash
thirdspace classes --date "$DATE" --days 7 --json
thirdspace classes --date "$DATE" --days 1 --name yoga --json
thirdspace classes --date "$DATE" --days 1 --trainer smith --json
```

These commands intentionally omit `--club`, causing the CLI to use the saved chosen club or clubs. For a one-off scope that the user explicitly requested:

```bash
thirdspace classes --date "$DATE" --days 1 --club "$CLUB_ID" --json
thirdspace classes --date "$DATE" --days 1 --club all --json
```

Name and trainer filters are case-insensitive. Useful class fields include:

- `id`
- `name`
- `startDate`
- `clubId` and `clubName`
- `zoneName`
- `trainerName`
- `spaces`
- `eventAction`

Times shown to users must be interpreted and reported in `Europe/London`.

Fetch fresh details before discussing or performing an action on a class:

```bash
thirdspace class CLASS_ID --json
```

## View bookings

```bash
thirdspace bookings --range upcoming --json
thirdspace bookings --range past --json
thirdspace bookings --range cancelled --json
```

The bookings command returns account bookings without a club flag. By default, filter the returned JSON to entries whose `clubId` is one of the saved chosen club IDs, and tell the user that the list is scoped to their chosen club or clubs. Show bookings from another club or all clubs only when explicitly requested.

Use the returned class IDs for follow-up detail or mutation commands.

## Class mutations

Mutations are external side effects. Never turn a search or recommendation request into a booking change.

Map the fresh class-detail `eventAction` to the only valid command:

| `eventAction` | Command |
| --- | --- |
| `join` | `thirdspace book CLASS_ID --yes` |
| `wait` | `thirdspace waitlist CLASS_ID --yes` |
| `cancel` | `thirdspace cancel CLASS_ID --yes` |
| `late_cancel` | `thirdspace cancel CLASS_ID --yes` |
| `leave` | `thirdspace leave-waitlist CLASS_ID --yes` |

Before running a mutation:

1. Run `thirdspace class CLASS_ID --json` immediately before approval.
2. Verify that its `clubId` is one of the saved chosen club IDs. If not, proceed only when the user explicitly requested or approves acting at that other club.
3. Summarize the exact class ID, name, London date/time, club, trainer, and requested action.
4. Obtain explicit approval for that exact target. A user instruction naming the exact action and class after these details are known counts as approval; a general search request does not.
5. If `eventAction` is `late_cancel`, warn that the app marks it as a late cancellation and a penalty may apply. Obtain confirmation that specifically acknowledges the late cancellation.
6. Run the mapped command with `--yes`. The CLI rechecks the server action immediately before applying it.
7. Report the command result. If the state changed or the result is ambiguous, re-read class details and bookings; do not blindly retry a mutation.

## Configuration

All commands accept `--config PATH`; `THIRDSPACE_CONFIG` can set the same path. Use a custom path consistently throughout one workflow. Do not inspect its contents because it may contain session credentials.

Do not override `THIRDSPACE_BASE_URL` except for an explicitly requested loopback test environment. The API is undocumented and server-authoritative.
