# frm

Friend Relationship Manager — a CLI tool for tracking contact frequency with friends and family, backed by CardDAV.

## Setup

Create `~/.frm/config.json` with at least one CardDAV service. iCloud is the most common setup.

### iCloud

1. Go to [account.apple.com](https://account.apple.com)
2. Sign in, then navigate to **Sign-In and Security** > **App-Specific Passwords**
3. Generate a new password (label it "frm" or similar)
4. Use your Apple ID email as the username

```json
{
  "services": [
    {
      "type": "carddav",
      "endpoint": "https://contacts.icloud.com",
      "username": "you@icloud.com",
      "password": "xxxx-xxxx-xxxx-xxxx"
    }
  ]
}
```

### Fastmail

Use your Fastmail app password. The endpoint includes your username:

```json
{
  "type": "carddav",
  "endpoint": "https://carddav.fastmail.com/dav/addressbooks/user/you@example.com/Default",
  "username": "you@example.com",
  "password": "your-app-password"
}
```

### Google Contacts

Use an app password (requires 2-step verification on your Google account):

```json
{
  "type": "carddav",
  "endpoint": "https://www.googleapis.com/.well-known/carddav",
  "username": "you@gmail.com",
  "password": "your-app-password"
}
```

### Multiple accounts and JMAP

You can combine multiple CardDAV accounts and add a JMAP email provider for context:

```json
{
  "services": [
    {
      "type": "carddav",
      "endpoint": "https://contacts.icloud.com",
      "username": "you@icloud.com",
      "password": "xxxx-xxxx-xxxx-xxxx"
    },
    {
      "type": "carddav",
      "endpoint": "https://carddav.fastmail.com/dav/addressbooks/user/you@example.com/Default",
      "username": "you@example.com",
      "password": "your-app-password"
    },
    {
      "type": "jmap",
      "session_endpoint": "https://api.fastmail.com/jmap/session",
      "token": "your-jmap-token",
      "max_results": 3
    }
  ]
}
```

The JMAP service adds email context to `frm context` and `frm triage`, showing recent email subjects exchanged with a contact.

You can override the config directory by setting `FRM_CONFIG_DIR`.

## Usage

```
frm contacts                       List all contacts
frm track "Alice" --every 2w       Track Alice every 2 weeks
frm untrack "Alice"                Stop tracking Alice
frm log "Alice" --note "coffee"    Log an interaction
frm log "Alice" --when -2w         Log an interaction that happened 2 weeks ago
frm list                           List tracked contacts (table format)
frm list --all                     List all contacts, not just tracked
frm check                          Show overdue contacts
frm triage                         Walk through untagged contacts interactively
frm triage --json                  List untriaged contacts as JSON (for LLM use)
frm ignore "Alice"                 Permanently hide from triage and check
frm unignore "Alice"               Reverse an ignore decision
frm snooze "Alice" --until 2m      Hide from check until 2 months from now
frm snooze "Alice" --until 2026-04-01  Hide from check until a specific date
frm unsnooze "Alice"               Remove a snooze
frm spread                         Preview spreading never-contacted people
frm spread --apply                 Apply the spread (snooze with random dates)
frm history "Alice"                Show interaction log for a contact
frm context "Alice"                Pre-meeting prep: summary of a contact
frm show "Alice"                   Alias for context
frm stats                          Dashboard: tracked, overdue, most/least contacted
frm group set "Alice" friends      Assign a contact to a group
frm group unset "Alice"            Remove a contact from its group
frm group list                     List all groups with counts
frm group list friends             List contacts in a group
```

All listing commands support `--json` for machine-readable output.

### Triage

`frm triage` loops through all contacts that have no frequency set and aren't ignored. For each contact it shows email, org, phone, and recent email context (if JMAP is configured), then prompts:

```
Alice Smith
  alice@example.com
  Acme Corp
Recent emails:
  Weekend plans? (2024-01-15)
  [m]onthly  [q]uarterly  [y]early  [s]kip  [i]gnore  [Enter=skip]>
```

- **m** — track monthly (1m)
- **q** — track quarterly (3m)
- **y** — track yearly (12m)
- **s** / Enter — skip (will appear again next triage)
- **i** — permanently ignore (hidden from triage and check)

#### LLM-friendly mode

`frm triage --json` skips the interactive loop and outputs untriaged contacts as a JSON array. An LLM can read this, then call `frm track` or `frm ignore` for each contact. Since decided contacts are automatically excluded, the LLM can call `frm triage --json` repeatedly until the list is empty.

Flags:
- `--limit N` — max contacts to return (default 5, `-1` for unlimited)

### Logging interactions

`frm log` defaults to the current time but supports backdating:

- `frm log "Alice" --note "coffee" --when 2024-01-15` — absolute date
- `frm log "Alice" --note "lunch" --when -2w` — relative (2 weeks ago)

Relative format: `-Nd` (days ago), `-Nw` (weeks ago), `-Nm` (months ago).

### Snoozing

`frm snooze` suppresses a contact from `frm check` without logging an interaction or changing their frequency. Useful when you know you won't reach out for a while.

- `frm snooze "Tracy" --until 2026-04-01` — absolute date
- `frm snooze "Tracy" --until 2m` — relative (2 months from now)
- `frm unsnooze "Tracy"` — remove the snooze early

### Listing contacts

`frm list` shows tracked contacts in a table with name, frequency, group, and due date:

```
NAME            FREQ  GROUP    DUE
Alice Smith     2w    friends  in 5d
Bob Jones       1m             overdue 3d
Carol White     3m    family   now
```

Use `--all` to include untracked contacts. Use `--json` for structured output with `due_in_days`.

### Spreading contacts after import

After a big import, all never-contacted people show as overdue at once. `frm spread` assigns each one a random snooze date between now and their frequency interval, so they trickle in over time instead of arriving all at once.

```
frm spread              # dry run — shows what would happen
frm spread --apply      # actually set the snoozes
```

Only affects tracked contacts that have never been contacted and aren't already snoozed.

### Duration format

- `3d` — every 3 days
- `2w` — every 2 weeks
- `1m` — every month (30 days)

## How it works

Contacts and metadata are stored in your CardDAV server via custom vCard fields:

- `X-FRM-FREQUENCY` — tracking interval (e.g. `2w`, `1m`, `3d`)
- `X-FRM-IGNORE` — `"true"` to permanently hide from triage/check
- `X-FRM-GROUP` — freeform group tag (e.g. `friends`, `professional`)
- `X-FRM-SNOOZE-UNTIL` — date (YYYY-MM-DD) to suppress from check until

Interaction history is stored locally in `~/.frm/log.jsonl`. Log entries include contact paths for name normalization — if a contact is renamed in your address book, existing log entries still match via the CardDAV path.
