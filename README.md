# frm

A CLI personal CRM backed by CardDAV. Single binary, no database — your contacts live in your existing address book, interaction history in a local JSONL file.

Built to be driven by AI agents. Your agent checks who you're overdue to reach out to, prepares context before meetings, and prompts you to stay in touch with the people who matter.

![frm demo](demo.gif)

## Install

```
go install github.com/justinabrahms/frm@latest
```

Or build from source:

```
git clone https://github.com/justinabrahms/frm
cd frm && go build .
```

## Quick start

1. Create `~/.frm/config.json` with your CardDAV credentials (see [Setup](#setup) below)
2. `frm triage` to categorize your contacts
3. `frm check` to see who you're overdue to reach out to
4. `frm log "Alice" --note "caught up over coffee"` after you talk to someone

## Agent integration

frm is designed for AI agents to drive your relationship maintenance. Every command supports `--json` for structured output.

The typical agent loop:

```bash
# 1. Who needs attention?
frm check --json

# 2. Prep context before a meeting
frm context "Sarah Chen" --json

# 3. After the interaction, log it
frm log "Sarah Chen" --note "discussed Q2 roadmap"

# 4. Bulk-categorize new contacts
frm triage --json --limit -1
# then for each: frm track "<name>" --every 2w  OR  frm ignore "<name>"
```

See [SKILL.md](SKILL.md) for a complete agent skill reference.

## Usage

```
frm contacts                       List all contacts
frm list                           List tracked contacts with due dates
frm list --all                     Include untracked contacts
frm check                          Show overdue contacts
frm context "Alice"                Pre-meeting prep: summary + recent emails
frm log "Alice" --note "coffee"    Log an interaction
frm log "Alice" --when -2w         Backdate an interaction
frm triage                         Walk through untagged contacts interactively
frm triage --json                  List untriaged contacts as JSON (for agents)
frm track "Alice" --every 2w       Track Alice every 2 weeks
frm untrack "Alice"                Stop tracking
frm ignore "Alice"                 Permanently hide from triage and check
frm unignore "Alice"               Reverse an ignore
frm snooze "Alice" --until 2m      Temporarily suppress from check
frm unsnooze "Alice"               Remove a snooze
frm spread                         Preview staggered snoozes for new imports
frm spread --apply                 Apply the spread
frm history "Alice"                Show interaction log
frm stats                          Dashboard
frm group set "Alice" friends      Assign to a group
frm group unset "Alice"            Remove from group
frm group list                     List all groups
frm group list friends             List contacts in a group
```

All listing commands support `--json` for machine-readable output.

### Duration format

- `3d` — every 3 days
- `2w` — every 2 weeks
- `1m` — every month (30 days)

### Triage

`frm triage` loops through contacts that have no frequency set and aren't ignored. For each one it shows email, org, phone, and recent email context (if JMAP is configured), then prompts:

```
Alice Smith
  alice@example.com
  Acme Corp
Recent emails:
  Weekend plans? (2024-01-15)
  [m]onthly  [q]uarterly  [y]early  [s]kip  [i]gnore  [Enter=skip]>
```

`frm triage --json` skips the interactive loop and outputs untriaged contacts as a JSON array. An agent can read this, call `frm track` or `frm ignore` for each contact, then repeat until the list is empty.

## Setup

Create `~/.frm/config.json` with at least one CardDAV service.

### iCloud

1. Go to [account.apple.com](https://account.apple.com) > **Sign-In and Security** > **App-Specific Passwords**
2. Generate a new password

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

```json
{
  "type": "carddav",
  "endpoint": "https://carddav.fastmail.com/dav/addressbooks/user/you@example.com/Default",
  "username": "you@example.com",
  "password": "your-app-password"
}
```

### Google Contacts

```json
{
  "type": "carddav",
  "endpoint": "https://www.googleapis.com/.well-known/carddav",
  "username": "you@gmail.com",
  "password": "your-app-password"
}
```

### Multiple accounts and JMAP email context

You can combine multiple CardDAV accounts and add a JMAP provider for email context in `frm context` and `frm triage`:

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
      "type": "jmap",
      "session_endpoint": "https://api.fastmail.com/jmap/session",
      "token": "your-jmap-token",
      "max_results": 3
    }
  ]
}
```

You can override the config directory with `FRM_CONFIG_DIR`.

## How it works

Contacts and metadata live in your CardDAV server via custom vCard fields:

- `X-FRM-FREQUENCY` — tracking interval (e.g. `2w`, `1m`)
- `X-FRM-IGNORE` — `"true"` to permanently hide
- `X-FRM-GROUP` — freeform group tag
- `X-FRM-SNOOZE-UNTIL` — date to suppress until

Interaction history is stored locally in `~/.frm/log.jsonl`. This is the only local state — back it up or symlink it to a synced directory.

## License

MIT
