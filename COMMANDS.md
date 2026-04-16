# frm — Command Reference

All commands support `--json` for structured output. **Always use `--json` when calling frm programmatically.**

## Reading data

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

## Writing data

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

## Bulk triage (agent workflow)

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
