# frm

Friend Relationship Manager — a CLI tool for tracking contact frequency with friends and family, backed by CardDAV.

## Setup

Create `~/.frm/config.json`:

```json
{
  "endpoint": "https://carddav.fastmail.com/dav/addressbooks/user/you@example.com/Default",
  "username": "you@example.com",
  "password": "your-app-password"
}
```

For multiple CardDAV accounts:

```json
{
  "accounts": [
    {
      "name": "personal",
      "endpoint": "https://carddav.fastmail.com/dav/addressbooks/user/you@example.com/Default",
      "username": "you@example.com",
      "password": "your-app-password"
    },
    {
      "name": "work",
      "endpoint": "https://contacts.google.com/.well-known/carddav",
      "username": "you@work.com",
      "password": "your-app-password"
    }
  ]
}
```

You can override the config directory by setting `FRM_CONFIG_DIR`.

## Usage

```
frm contacts                       List all contacts
frm track "Alice" --every 2w       Track Alice every 2 weeks
frm untrack "Alice"                Stop tracking Alice
frm log "Alice" --note "coffee"    Log an interaction
frm check                          Show overdue contacts
frm triage                         Walk through untagged contacts and assign frequencies
frm history "Alice"                Show interaction log for a contact
frm context "Alice"                Pre-meeting prep: summary of a contact
frm unignore "Alice"               Reverse an ignore decision
frm stats                          Dashboard: tracked, overdue, most/least contacted
frm group set "Alice" friends      Assign a contact to a group
frm group unset "Alice"            Remove a contact from its group
frm group list                     List all groups with counts
frm group list friends             List contacts in a group
```

All listing commands support `--json` for machine-readable output.

### Triage

`frm triage` loops through all contacts that have no frequency set and aren't ignored. For each contact it prompts:

```
Alice Smith
  [m]onthly  [q]uarterly  [y]early  [s]kip  [i]gnore  [Enter=skip]>
```

- **m** — track monthly (1m)
- **q** — track quarterly (3m)
- **y** — track yearly (12m)
- **s** / Enter — skip (will appear again next triage)
- **i** — permanently ignore (hidden from triage and check)

### Duration format

- `3d` — every 3 days
- `2w` — every 2 weeks
- `1m` — every month (30 days)

## How it works

Contacts and tracking frequency are stored in your CardDAV server (via custom `X-FRM-FREQUENCY`, `X-FRM-IGNORE`, and `X-FRM-GROUP` vCard fields). Interaction history is stored locally in `~/.frm/log.jsonl`.

Log entries include contact paths for name normalization — if a contact is renamed in your address book, existing log entries still match via the CardDAV path.
