# frm — Project Guide

## What this is

CLI friend relationship manager backed by CardDAV. Single Go binary, no database — contacts live in your CardDAV server, interaction history in `~/.frm/log.jsonl`.

## Build & Test

```
go build ./...
go test -v ./...
```

All tests are E2E: they compile the binary, start an in-memory CardDAV server (`e2e_test.go`), and run subprocesses. There are no unit tests. To test stdin-driven commands (like `triage`), use `env.runWithStdin(t, strings.NewReader("m\nq\n"), "triage")`.

## Architecture

Single `package main`. No internal packages. Every command is in its own `cmd_*.go` file registered via `func init()`.

Key files:
- `main.go` — root cobra command
- `config.go` — config loading, multi-account support (`resolvedConfigs()`)
- `carddav.go` — CardDAV client helpers, `allContactsMulti()` for multi-account iteration, `findContactMulti()` for single-contact lookup across accounts
- `vcard.go` — vCard field constants and helpers (frequency, ignore, group)
- `logfile.go` — `LogEntry` struct, append/read log, `lastContactTime()`
- `json.go` — `--json` flag (persistent on root) and `printJSON()` helper

## Custom vCard fields

All stored on contacts in the CardDAV server:
- `X-FRM-FREQUENCY` — tracking interval (e.g. `2w`, `1m`, `3d`)
- `X-FRM-IGNORE` — `"true"` to permanently hide from triage/check
- `X-FRM-GROUP` — freeform group tag (e.g. `friends`, `professional`)

## LLM-friendly triage workflow

`frm triage --json` outputs untriaged contacts as a JSON array (no interactive prompts). An LLM can read this, then call `frm track <name> --every <freq>` or `frm ignore <name>` for each contact. The list automatically excludes contacts that already have a frequency or are ignored, so the LLM can call `frm triage --json` repeatedly until the list is empty.

Flags:
- `--limit N` — max contacts to return (default 5, `-1` for unlimited)

## Conventions

- Commands that list all contacts use `allContactsMulti(cfg)` to support multi-account
- Commands that look up a single contact by name use `findContactMulti(cfg, name)` which returns `(*AddressObject, *Client, error)`
- Log entries include a `path` field for name normalization — lookups prefer path over display name
- `--json` is a persistent root flag; commands check `cmd.Flags().GetBool("json")` and call `printJSON(cmd, v)`
- Duration strings: `d` (days), `w` (weeks), `m` (months as 30 days)
- Tests seed contacts via `env.backend.seedContact("Name", "freq")` — empty string for no frequency
