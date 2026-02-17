# frm — Agent Skill

`frm` is a CLI friend/relationship manager backed by CardDAV. Use it to help the user maintain their personal and professional relationships.

## When to use this skill

- The user asks about contacts they should reach out to
- The user wants to prepare for a meeting with someone
- The user wants to log that they talked to someone
- The user asks you to triage or categorize their contacts
- The user wants a summary of their relationship tracking

## Core workflow

1. **Check who's overdue**: `frm check --json` — returns contacts past their tracking interval
2. **Prep for a meeting**: `frm context "<name>" --json` — returns contact summary, last interaction, and recent emails
3. **Log an interaction**: `frm log "<name>" --note "what happened"`
4. **Bulk triage**: `frm triage --json` then call `frm track` or `frm ignore` for each contact

## Commands

All commands support `--json` for structured output. **Always use `--json` when calling frm programmatically.**

### Reading data

```bash
# List tracked contacts with due dates
frm list --json
# Fields: name, frequency, group, due_in_days

# List ALL contacts (including untracked)
frm list --all --json

# Show overdue contacts
frm check --json
# Fields: name, frequency, last_seen, ago

# Pre-meeting prep (includes recent emails if JMAP configured)
frm context "<name>" --json
# Fields: name, frequency, group, ignored, last_contact, last_note,
#         days_since, days_until_due, providers

# Interaction history for a contact
frm history "<name>" --json
# Returns array of {contact, path, time, note}

# Dashboard stats
frm stats --json
# Fields: total_contacts, tracked, ignored, untracked, overdue,
#         total_interactions, most_contacted, least_contacted

# List all groups
frm group list --json

# List contacts in a group
frm group list "<group>" --json

# List all contact names
frm contacts --json
```

### Writing data

```bash
# Set tracking frequency (d=days, w=weeks, m=months)
frm track "<name>" --every 2w
frm track "<name>" --every 1m
frm track "<name>" --every 3d

# Stop tracking
frm untrack "<name>"

# Log an interaction (now)
frm log "<name>" --note "coffee chat about their new role"

# Log a past interaction
frm log "<name>" --note "met at conference" --when 2025-06-15
frm log "<name>" --note "quick call" --when -3d

# Permanently hide from triage/check
frm ignore "<name>"
frm unignore "<name>"

# Temporarily suppress from check (absolute or relative date)
frm snooze "<name>" --until 2026-04-01
frm snooze "<name>" --until 2m
frm unsnooze "<name>"

# Assign/remove groups
frm group set "<name>" friends
frm group unset "<name>"
```

### Bulk triage (agent workflow)

The `triage --json` endpoint is designed for agents. It returns untriaged contacts (no frequency, not ignored) so you can process them:

```bash
# Get untriaged contacts (default limit 5, use --limit -1 for all)
frm triage --json --limit -1
```

Returns:
```json
[
  {"name": "Alice Smith", "email": "alice@example.com", "org": "Acme Corp"},
  {"name": "Bob Jones"}
]
```

For each contact, decide and execute one of:
```bash
frm track "<name>" --every 2w    # important, talk often
frm track "<name>" --every 1m    # monthly check-in
frm track "<name>" --every 3m    # quarterly
frm track "<name>" --every 12m   # yearly
frm ignore "<name>"              # not relevant
```

Repeat `frm triage --json` until the list is empty.

## Duration format

- `Nd` — days (e.g. `3d` = 3 days)
- `Nw` — weeks (e.g. `2w` = 14 days)
- `Nm` — months as 30 days (e.g. `1m` = 30 days)

## Tips

- Contact names are case-insensitive for lookup
- When logging, always include a meaningful `--note` — it's shown in `context` output and helps the user remember what was discussed
- Use `--when` with `frm log` for backdating (e.g. `--when -2d` for "two days ago")
- `frm context` is the best single command for preparing the user for a conversation — it combines tracking status, interaction history, and email context
- Groups are freeform strings; common ones are `friends`, `family`, `professional`
- Snooze is for temporary suppression (e.g. "they're on vacation for 2 months"); ignore is permanent
