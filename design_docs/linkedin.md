# LinkedIn Integration

## Problem

LinkedIn is where many professional relationships live, but those contacts aren't in CardDAV. We want `frm` to know about LinkedIn connections so they can be triaged and tracked like any other contact.

## The API situation (honest version)

LinkedIn's API is effectively closed to individual developers:

- The only self-service APIs are "Sign In with LinkedIn" and "Share on LinkedIn" -- neither exposes your connections list.
- Enumerating your own connections requires partner-level API access, which is only available to incorporated companies, takes 3-6 months to approve, and has a <10% approval rate.
- Sales Navigator and Marketing APIs exist but require paid enterprise partnerships.
- There is **no API endpoint** that lets you list your own connections. Full stop.

Scraping tools exist (PhantomBuster, Evaboot, etc.) but violate LinkedIn's ToS and risk account bans. Not worth it for a personal tool.

## What we can actually do: CSV import

LinkedIn lets you export your connections via Settings > Data Privacy > Get a copy of your data. This produces a CSV with:

| Field | Reliability |
|-------|-------------|
| First Name | Always present |
| Last Name | Always present |
| Company | Usually present |
| Position | Usually present |
| Connected On | Always present |
| Email Address | ~10-20% of connections (opt-in) |

Missing: phone numbers, profile URLs, profile photos, mutual connections, messages.

### Limitations

- **Manual process.** User must request the export from LinkedIn's UI, wait (minutes to hours), download a ZIP, extract the CSV.
- **No sync.** This is a one-shot import. New connections or updated titles won't appear until the next manual export.
- **Sparse data.** Name + company + title is all you reliably get.

## Proposed design: `frm import linkedin <file>`

A subcommand under an `import` parent (leaves room for `frm import google`, `frm import csv`, etc. later).

### Behavior

1. Read the LinkedIn Connections CSV.
2. For each row, check if a contact with the same name already exists in CardDAV (via `findContactMulti`).
3. If the contact exists: optionally update org/title if they're empty in the vCard. Skip otherwise.
4. If the contact is new: create a vCard with FN, ORG, TITLE, EMAIL (if present), and a `NOTE` field recording the LinkedIn connection date. Push to the first configured CardDAV account.
5. Print a summary: N created, N updated, N skipped.

### Flags

- `--dry-run` -- show what would be created/updated without writing
- `--account N` -- target a specific CardDAV account (default: first)

### vCard mapping

```
FN: First Name + Last Name
ORG: Company
TITLE: Position
EMAIL: Email Address (if present)
NOTE: Connected on LinkedIn: 2024-01-15
```

### Example

```
$ frm import linkedin ~/Downloads/Connections.csv
Importing 342 LinkedIn connections...
  Created: 187
  Updated: 23 (added org/title)
  Skipped: 132 (already exist)
Done.
```

### Dedup strategy

Match by normalized name (case-insensitive, trimmed). This will produce false positives for common names -- acceptable for a first pass, since `frm triage` lets the user clean up afterward. A future improvement could match on email when available.

## What about keeping it fresh?

Since there's no API sync, the user would periodically re-export and re-import. The import is idempotent (existing contacts are skipped or lightly updated), so re-running is safe.

We could add `frm import linkedin --since 2024-01-01` to only import connections added after a date, filtering on the "Connected On" CSV column.

## Future possibilities

- **`frm import csv`** -- generic CSV import with column mapping, not LinkedIn-specific. LinkedIn import could be a preset.
- **Connection date as log entry** -- seed `log.jsonl` with a "connected on LinkedIn" entry for each import, giving `frm check` a baseline "last contact" date.
- **LinkedIn profile URL** -- not in the export, but could be constructed as `https://linkedin.com/in/{slug}`. Problem: the slug isn't in the CSV either. Would need the full archive export (not just connections) to get profile URLs from messages.
- **If LinkedIn ever opens up** -- the architecture should make it easy to swap in an API-backed provider. The `import` subcommand pattern means we don't couple LinkedIn into the core CardDAV flow.

## Implementation plan

| File | Change |
|------|--------|
| `cmd_import.go` | New file -- `import` parent command |
| `cmd_import_linkedin.go` | New file -- `import linkedin <csv>` subcommand |
| `e2e_test.go` | Test with a sample CSV, verify vCards created in CardDAV |

Estimated scope: ~150-200 lines of Go + tests.
