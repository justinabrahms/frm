#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# dependencies = ["pyyaml"]
# ///

"""
Structural skill loading eval for frm SKILL.md

Tests whether a skill file would be correctly loaded and triggered
by a skill loader (like Claude Code's). Two dimensions:

1. Parse: Does the file have valid YAML frontmatter with required fields?
2. Trigger: Given just the name+description from frontmatter, does an LLM
   recognize that the skill is relevant to each test prompt?

This simulates the skill selection step that happens BEFORE the full
skill content is ever read.

Usage:
  ./eval/run_structural_eval.py [--runs N] skill_a [skill_b ...]
"""

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor, as_completed

SCRIPT_DIR = Path(__file__).parent
CASES_FILE = SCRIPT_DIR / "cases.json"
RESULTS_DIR = SCRIPT_DIR / "results" / "structural"


def load_cases():
    with open(CASES_FILE) as f:
        return json.load(f)


def parse_frontmatter(text: str) -> dict | None:
    """Extract YAML frontmatter from a markdown file. Returns None if missing."""
    import yaml

    match = re.match(r"^---\s*\n(.*?)\n---\s*\n", text, re.DOTALL)
    if not match:
        return None
    try:
        return yaml.safe_load(match.group(1))
    except yaml.YAMLError:
        return None


def check_structure(frontmatter: dict | None) -> dict:
    """Check required structural fields. Returns per-field pass/fail."""
    checks = {
        "has_frontmatter": frontmatter is not None,
        "has_name": False,
        "name_is_kebab": False,
        "has_description": False,
        "description_nonempty": False,
    }
    if frontmatter is None:
        return checks

    name = frontmatter.get("name", "")
    desc = frontmatter.get("description", "")

    checks["has_name"] = bool(name)
    checks["name_is_kebab"] = bool(re.match(r"^[a-z0-9]+(-[a-z0-9]+)*$", str(name)))
    checks["has_description"] = bool(desc)
    checks["description_nonempty"] = len(str(desc).strip()) > 10

    return checks


def test_trigger(description: str, prompt: str, label: str, case_id: str, run_num: int) -> bool:
    """Ask an LLM whether a skill with this description is relevant to the prompt."""
    RESULTS_DIR.mkdir(parents=True, exist_ok=True)

    system_prompt = """\
You are a skill router. You have access to several tools/skills. Given a user message, decide if the skill below is relevant.

Available skill:
  name: frm
  description: {description}

Other skills are also available (calendar, email, coding, web search, etc.) but you are only evaluating whether "frm" is relevant.

Respond with ONLY "yes" or "no". Nothing else.""".format(description=description)

    outfile = RESULTS_DIR / f"{label}_{case_id}_trigger_run{run_num}.txt"

    try:
        result = subprocess.run(
            ["claude", "--print", "--system-prompt", system_prompt, "--model", "haiku"],
            input=prompt,
            capture_output=True,
            text=True,
            timeout=30,
        )
        response = result.stdout.strip().lower()
    except (subprocess.TimeoutExpired, FileNotFoundError) as e:
        response = f"error: {e}"

    outfile.write_text(response)
    return response.startswith("yes")


def eval_skill(skill_path: Path, label: str, cases: list, runs: int) -> dict:
    """Evaluate a single skill file. Returns structure checks + trigger scores."""
    text = skill_path.read_text()
    frontmatter = parse_frontmatter(text)
    structure = check_structure(frontmatter)

    # If no valid frontmatter, we can still test trigger using the first
    # heading + paragraph as a fallback description (simulating what a
    # loader might extract as a heuristic).
    if frontmatter and frontmatter.get("description"):
        description = frontmatter["description"]
    else:
        # Fallback: use first non-empty paragraph after any heading
        lines = text.strip().split("\n")
        desc_lines = []
        for line in lines:
            stripped = line.strip()
            if stripped.startswith("#"):
                continue
            if stripped:
                desc_lines.append(stripped)
            if len(desc_lines) >= 2:
                break
        description = " ".join(desc_lines) if desc_lines else "CLI tool"

    trigger_results = []
    for case in cases:
        case_id = case["id"]
        hits = 0
        for r in range(1, runs + 1):
            if test_trigger(description, case["prompt"], label, case_id, r):
                hits += 1
        trigger_results.append({
            "case_id": case_id,
            "hits": hits,
            "runs": runs,
        })

    return {
        "structure": structure,
        "description_used": description,
        "triggers": trigger_results,
    }


def main():
    parser = argparse.ArgumentParser(description="Structural eval for frm SKILL.md")
    parser.add_argument("skills", nargs="+", help="Skill files to evaluate")
    parser.add_argument("--runs", type=int, default=3, help="Runs per case (default: 3)")
    args = parser.parse_args()

    cases = load_cases()
    skill_paths = [Path(s) for s in args.skills]
    labels = [p.stem.replace(".skill_", "").lstrip(".") for p in skill_paths]

    # Deduplicate labels
    seen = {}
    for i, label in enumerate(labels):
        if label in seen:
            seen[label] += 1
            labels[i] = f"{label}_{seen[label]}"
        else:
            seen[label] = 0

    print(f"\nStructural eval: {len(cases)} cases x {args.runs} runs for {len(skill_paths)} version(s)")
    print("=" * 70)

    # Run evals in parallel
    all_results = {}
    with ThreadPoolExecutor(max_workers=len(skill_paths)) as pool:
        futures = {
            pool.submit(eval_skill, path, label, cases, args.runs): label
            for path, label in zip(skill_paths, labels)
        }
        for future in as_completed(futures):
            label = futures[future]
            all_results[label] = future.result()
            print(f"  Completed: {label}")

    # === Structure checks ===
    print()
    print("=" * 70)
    print("STRUCTURE (can the skill loader parse this file?)")
    print("=" * 70)

    check_names = ["has_frontmatter", "has_name", "name_is_kebab", "has_description", "description_nonempty"]
    header = f"{'Check':25s}"
    for label in labels:
        header += f"  {label:>12s}"
    print(header)
    print("-" * len(header))

    for check in check_names:
        row = f"{check:25s}"
        for label in labels:
            val = all_results[label]["structure"][check]
            marker = "PASS" if val else "FAIL"
            row += f"  {marker:>12s}"
        print(row)

    struct_scores = {}
    for label in labels:
        passed = sum(1 for c in check_names if all_results[label]["structure"][c])
        struct_scores[label] = f"{passed}/{len(check_names)}"
    row = f"{'TOTAL':25s}"
    for label in labels:
        row += f"  {struct_scores[label]:>12s}"
    print(row)

    # === Description used ===
    print()
    for label in labels:
        desc = all_results[label]["description_used"]
        print(f"  {label} description: {desc[:80]}{'...' if len(desc) > 80 else ''}")

    # === Trigger results ===
    print()
    print("=" * 70)
    print("TRIGGER (does the description cause the skill to be selected?)")
    print("=" * 70)

    header = f"{'Case':25s}"
    for label in labels:
        header += f"  {label:>12s}"
    print(header)
    print("-" * len(header))

    trigger_totals = {label: (0, 0) for label in labels}
    for i, case in enumerate(cases):
        row = f"{case['id']:25s}"
        for label in labels:
            t = all_results[label]["triggers"][i]
            row += f"  {t['hits']:>5d}/{t['runs']:<5d}"
            h, r = trigger_totals[label]
            trigger_totals[label] = (h + t["hits"], r + t["runs"])
        print(row)

    print()
    row = f"{'TOTAL':25s}"
    pcts = {}
    for label in labels:
        h, r = trigger_totals[label]
        pct = (h * 100 // r) if r > 0 else 0
        pcts[label] = pct
        row += f"  {pct:>10d}% "
    print(row)

    # === Combined summary ===
    print()
    print("=" * 70)
    print("COMBINED SUMMARY")
    print("=" * 70)
    for label in labels:
        s = all_results[label]["structure"]
        parseable = s["has_frontmatter"] and s["has_name"] and s["has_description"]
        print(f"  {label}:")
        print(f"    Parseable by loader:  {'YES' if parseable else 'NO'}")
        print(f"    Structure score:      {struct_scores[label]}")
        print(f"    Trigger rate:         {pcts[label]}%")
        if not parseable:
            print(f"    NOTE: Without frontmatter, a strict loader would never load this skill.")
            print(f"           Trigger test used fallback heuristic description.")
        print()


if __name__ == "__main__":
    main()
