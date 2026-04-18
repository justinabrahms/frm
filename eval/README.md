# frm Skill Eval Harness

Compares two versions of `SKILL.md` by scoring whether an LLM produces
the correct `frm` commands given natural-language user prompts.

## How it works

1. **Test cases** (`cases.json`) define user prompts, expected commands,
   and required/forbidden patterns in the response.
2. **The eval script** feeds each prompt to the `claude` CLI with the
   skill document as system context, asking for raw command output.
3. **Scoring** checks whether the response contains required tokens
   (e.g. `--json`, correct subcommand) and avoids forbidden ones
   (e.g. using `ignore` when `snooze` is correct).
4. Each case runs multiple times (default 3) to reduce variance.

## Usage

```bash
# Compare current SKILL.md vs PR #2
./eval/run_eval.sh

# More runs for statistical confidence
./eval/run_eval.sh --runs 5

# Compare two arbitrary files
./eval/run_eval.sh --runs 3 path/to/old_skill.md path/to/new_skill.md
```

## Requirements

- `claude` CLI (Claude Code)
- `jq`

## Important caveat

The PR splits SKILL.md into a shorter file + COMMANDS.md. This eval only
tests SKILL.md in isolation. If agents in practice would also read
COMMANDS.md (e.g. via a file reference), the PR version may perform
differently than this eval suggests. You can test the combined version by
concatenating: `cat SKILL.md COMMANDS.md > combined.md` and passing that
as the skill file.

## Adding test cases

Edit `cases.json`. Each case has:
- `id` — unique identifier
- `prompt` — what the user says
- `description` — what the case tests
- `expected_commands` — reference commands (for documentation)
- `must_include` — tokens that must appear in the response (case-insensitive)
- `must_not_include` — tokens that must NOT appear (tests discrimination)
