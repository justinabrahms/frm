#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# ///

"""
Skill eval harness for frm SKILL.md

Compares skill document versions by feeding test scenarios to an LLM
and scoring whether it produces the expected frm commands.

Usage:
  ./eval/run_eval.py [--runs N] skill_a [skill_b [skill_c ...]]

Each skill file is scored independently. Results are shown side-by-side.
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor, as_completed

SCRIPT_DIR = Path(__file__).parent
CASES_FILE = SCRIPT_DIR / "cases.json"
RESULTS_DIR = SCRIPT_DIR / "results"

SYSTEM_PROMPT_TEMPLATE = """\
You are an AI assistant with access to the frm CLI tool. Below is the skill document that describes how to use frm:

<skill>
{skill_content}
</skill>

When the user asks you to do something, respond with the exact frm commands you would run. Output ONLY the commands, one per line. Do not explain, do not add commentary. Just the commands."""


def load_cases():
    with open(CASES_FILE) as f:
        return json.load(f)


def run_case(skill_content: str, prompt: str, label: str, case_id: str, run_num: int) -> str:
    """Run a single eval case via claude CLI and return the response."""
    RESULTS_DIR.mkdir(exist_ok=True)
    outfile = RESULTS_DIR / f"{label}_{case_id}_run{run_num}.txt"
    system_prompt = SYSTEM_PROMPT_TEMPLATE.format(skill_content=skill_content)

    try:
        result = subprocess.run(
            ["claude", "--print", "--system-prompt", system_prompt, "--model", "haiku"],
            input=prompt,
            capture_output=True,
            text=True,
            timeout=60,
        )
        response = result.stdout.strip()
    except (subprocess.TimeoutExpired, FileNotFoundError) as e:
        response = f"ERROR: {e}"

    outfile.write_text(response)
    return response


def score_response(response: str, case: dict) -> tuple[int, int]:
    """Score a response. Returns (score, max_possible)."""
    score = 0
    max_score = 0
    response_lower = response.lower()

    for pattern in case["must_include"]:
        max_score += 1
        if pattern.lower() in response_lower:
            score += 1

    for pattern in case["must_not_include"]:
        max_score += 1
        if pattern.lower() not in response_lower:
            score += 1

    return score, max_score


def eval_skill(skill_path: Path, label: str, cases: list, runs: int) -> list[dict]:
    """Evaluate a single skill file across all cases. Returns per-case results."""
    skill_content = skill_path.read_text()
    results = []

    for case in cases:
        case_id = case["id"]
        total_score = 0
        total_max = 0

        for r in range(1, runs + 1):
            response = run_case(skill_content, case["prompt"], label, case_id, r)
            s, m = score_response(response, case)
            total_score += s
            total_max += m

        results.append({
            "case_id": case_id,
            "score": total_score,
            "max": total_max,
        })

    return results


def main():
    parser = argparse.ArgumentParser(description="Eval harness for frm SKILL.md")
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

    print(f"\nRunning {len(cases)} test cases x {args.runs} runs for {len(skill_paths)} skill version(s)")
    print("=" * 70)

    # Run all skill evals in parallel
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

    # Print per-case details
    print()
    for i, case in enumerate(cases):
        case_id = case["id"]
        parts = []
        for label in labels:
            r = all_results[label][i]
            parts.append(f"{label}: {r['score']}/{r['max']}")
        print(f"  {case_id:25s}  {' | '.join(parts)}")

    # Summary table
    print()
    print("=" * 70)
    print("SUMMARY")
    print("=" * 70)

    header = f"{'Case':25s}"
    for label in labels:
        header += f"  {label:>12s}"
    print(header)
    print("-" * len(header))

    totals = {label: (0, 0) for label in labels}
    for i, case in enumerate(cases):
        row = f"{case['id']:25s}"
        for label in labels:
            r = all_results[label][i]
            row += f"  {r['score']:>5d}/{r['max']:<4d}"
            ts, tm = totals[label]
            totals[label] = (ts + r["score"], tm + r["max"])
        print(row)

    print()
    pct_row = f"{'TOTAL':25s}"
    pcts = {}
    for label in labels:
        ts, tm = totals[label]
        pct = (ts * 100 // tm) if tm > 0 else 0
        pcts[label] = pct
        pct_row += f"  {pct:>10d}% "
    print(pct_row)

    # Winner
    print()
    best_label = max(labels, key=lambda l: pcts[l])
    best_pct = pcts[best_label]
    tied = [l for l in labels if pcts[l] == best_pct]
    if len(tied) == len(labels):
        print("Result: All versions scored equally.")
    elif len(tied) > 1:
        print(f"Result: Tied between {', '.join(tied)} at {best_pct}%.")
    else:
        print(f"Result: {best_label} scores highest at {best_pct}%.")


if __name__ == "__main__":
    main()
