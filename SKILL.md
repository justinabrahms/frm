---
name: frm
description: "CLI friend/relationship manager backed by CardDAV. Use when the user asks about overdue contacts, preparing for a meeting with someone, logging interactions, triaging or categorizing contacts, checking relationship tracking stats, managing contact groups, or snoozing reminders."
---

# frm

CLI relationship manager backed by CardDAV. Helps the user maintain personal and professional relationships by tracking contact frequency, logging interactions, and surfacing overdue follow-ups.

## Core workflow

1. **Check who's overdue**: `frm check --json` — returns contacts past their tracking interval
2. **Prep for a meeting**: `frm context "<name>" --json` — returns contact summary, last interaction, and recent emails
3. **Log an interaction**: `frm log "<name>" --note "what happened"`
4. **Bulk triage**: `frm triage --json` then call `frm track` or `frm ignore` for each contact. Repeat `frm triage --json` until the list is empty.
5. **Verify**: `frm stats --json` — confirm totals updated after bulk operations

## Key commands

All commands support `--json` for structured output. **Always use `--json` when calling frm programmatically.** See [COMMANDS.md](COMMANDS.md) for the full command reference.

```bash
frm check --json              # overdue contacts
frm context "<name>" --json   # meeting prep with email context
frm log "<name>" --note "..." # log interaction
frm triage --json             # untriaged contacts for bulk processing
frm track "<name>" --every 2w # set follow-up frequency
frm ignore "<name>"           # permanently hide from triage/check
frm snooze "<name>" --until 2m # temporarily suppress
frm stats --json              # dashboard overview
```

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
