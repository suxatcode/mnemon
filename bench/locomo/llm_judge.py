"""LLM-as-Judge: evaluate predictions semantically instead of token F1.

Usage:
    cd bench && python3 -m locomo.llm_judge raw
    cd bench && python3 -m locomo.llm_judge raw --limit 10   # quick test
"""

import asyncio
import json
import sys
from pathlib import Path

from .config import Config
from .llm import call_llm
from .prompts import CATEGORY_NAMES, LLM_JUDGE_PROMPT


async def judge_one(
    question: str, gold: str, prediction: str, config: Config,
) -> dict:
    """Ask LLM to judge if prediction is semantically correct."""
    prompt = LLM_JUDGE_PROMPT.format(
        question=question, gold=gold, prediction=prediction,
    )
    response = await call_llm(
        prompt=prompt,
        api_base=config.ingestion_api_base,
        api_key=config.ingestion_api_key,
        model=config.ingestion_model,
        temperature=0,
        max_tokens=128,
    )
    text = response.strip()
    if text.startswith("```"):
        text = text.split("\n", 1)[-1].rsplit("```", 1)[0].strip()
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        return {"verdict": "ERROR", "reason": text[:100]}


def compute_f1(prediction: str, ground_truth: str) -> float:
    pred_tokens = prediction.lower().split()
    gold_tokens = ground_truth.lower().split()
    if not pred_tokens or not gold_tokens:
        return 0.0
    common = set(pred_tokens) & set(gold_tokens)
    if not common:
        return 0.0
    precision = len(common) / len(pred_tokens)
    recall = len(common) / len(gold_tokens)
    return 2 * precision * recall / (precision + recall)


async def run_judge(mode: str, limit: int | None = None):
    config = Config()
    result_file = config.output_dir / f"mnemon_{mode}_qa.json"

    if not result_file.exists():
        print(f"No results file: {result_file}")
        return

    data = json.loads(result_file.read_text())
    pred_key = f"mnemon_{mode}_prediction"

    all_qa = []
    for sample in data:
        for qa in sample["qa"]:
            if qa.get(pred_key):
                all_qa.append(qa)

    if limit:
        all_qa = all_qa[:limit]

    concurrency = config.concurrency
    print(f"Judging {len(all_qa)} predictions with LLM (concurrency={concurrency})...\n", flush=True)

    # Score tracking
    cat_verdicts: dict[int, dict[str, int]] = {}
    cat_f1: dict[int, list[float]] = {}

    sem = asyncio.Semaphore(concurrency)
    done_count = 0

    async def judge_task(i: int, qa: dict) -> tuple[int, dict, str, str, float, int]:
        async with sem:
            question = qa["question"]
            gold = str(qa["answer"])
            pred = qa[pred_key]
            cat = qa["category"]
            try:
                judgment = await judge_one(question, gold, pred, config)
            except Exception as e:
                judgment = {"verdict": "ERROR", "reason": str(e)[:80]}
            f1 = compute_f1(pred, gold)
            return i, judgment, gold, pred, f1, cat

    tasks = [asyncio.create_task(judge_task(i, qa)) for i, qa in enumerate(all_qa)]

    for coro in asyncio.as_completed(tasks):
        i, judgment, gold, pred, f1, cat = await coro
        verdict = judgment.get("verdict", "ERROR")
        done_count += 1

        # Track
        cat_verdicts.setdefault(cat, {"CORRECT": 0, "PARTIAL": 0, "WRONG": 0, "ERROR": 0})
        cat_verdicts[cat][verdict] = cat_verdicts[cat].get(verdict, 0) + 1
        cat_f1.setdefault(cat, []).append(f1)

        # Show mismatches (F1 low but LLM says correct)
        marker = ""
        if verdict == "CORRECT" and f1 < 0.5:
            marker = " ← F1 underscores!"
        elif verdict == "WRONG" and f1 > 0.5:
            marker = " ← F1 overscores!"

        print(f"  [{done_count}/{len(all_qa)}] cat{cat} | {verdict:<8} F1={f1:.2f} | "
              f"Gold: {gold[:40]:<40} Pred: {pred[:40]}{marker}", flush=True)

    # Summary
    print(f"\n{'='*70}")
    print(f"  LLM Judge Results: Mode {mode.upper()}")
    print(f"{'='*70}")
    print(f"{'Category':<15} {'Total':>6} {'Correct':>8} {'Partial':>8} {'Wrong':>8} {'Acc%':>7} {'Avg F1':>7}")
    print(f"{'-'*15} {'-'*6} {'-'*8} {'-'*8} {'-'*8} {'-'*7} {'-'*7}")

    total_correct = 0
    total_partial = 0
    total_all = 0

    for cat in sorted(cat_verdicts.keys()):
        v = cat_verdicts[cat]
        total = sum(v.values())
        correct = v.get("CORRECT", 0)
        partial = v.get("PARTIAL", 0)
        wrong = v.get("WRONG", 0)
        acc = (correct + 0.5 * partial) / total * 100 if total else 0
        f1_avg = sum(cat_f1[cat]) / len(cat_f1[cat]) if cat_f1.get(cat) else 0
        name = CATEGORY_NAMES.get(cat, f"cat-{cat}")
        print(f"{name:<15} {total:>6} {correct:>8} {partial:>8} {wrong:>8} {acc:>6.1f}% {f1_avg:>6.3f}")
        total_correct += correct
        total_partial += partial
        total_all += total

    overall_acc = (total_correct + 0.5 * total_partial) / total_all * 100 if total_all else 0
    all_f1 = [f for scores in cat_f1.values() for f in scores]
    overall_f1 = sum(all_f1) / len(all_f1) if all_f1 else 0
    print(f"{'-'*15} {'-'*6} {'-'*8} {'-'*8} {'-'*8} {'-'*7} {'-'*7}")
    print(f"{'Overall':<15} {total_all:>6} {total_correct:>8} {total_partial:>8} "
          f"{total_all-total_correct-total_partial:>8} {overall_acc:>6.1f}% {overall_f1:>6.3f}")
    print()
    print("  Acc% = (CORRECT + 0.5×PARTIAL) / Total")
    print("  Compare Acc% (semantic) vs Avg F1 (token) to see how much F1 underestimates.")


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "raw"
    limit = None
    if "--limit" in sys.argv:
        idx = sys.argv.index("--limit")
        limit = int(sys.argv[idx + 1])
    asyncio.run(run_judge(mode, limit))


if __name__ == "__main__":
    main()
