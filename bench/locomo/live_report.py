"""Live report: read completed results + in-progress checkpoint and print current scores."""

import json
import sys
from pathlib import Path

from .prompts import CATEGORY_NAMES


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


def gather_qa(output_dir: Path, mode: str) -> list[dict]:
    """Collect all available QA results: completed samples + checkpoint."""
    all_qa = []

    # 1. Completed samples
    result_file = output_dir / f"mnemon_{mode}_qa.json"
    completed_samples = []
    if result_file.exists():
        completed_samples = json.loads(result_file.read_text())
        for s in completed_samples:
            all_qa.extend(s.get("qa", []))

    # 2. In-progress checkpoint (current sample)
    completed_ids = {s["sample_id"] for s in completed_samples}
    for cp in output_dir.glob(f"_checkpoint_{mode}_*.json"):
        sample_id = cp.stem.replace(f"_checkpoint_{mode}_", "")
        if sample_id not in completed_ids:
            qa_list = json.loads(cp.read_text())
            all_qa.extend(qa_list)

    return all_qa


def print_live(output_dir: Path, mode: str):
    pred_key = f"mnemon_{mode}_prediction"
    all_qa = gather_qa(output_dir, mode)

    if not all_qa:
        print(f"No results yet for mode '{mode}'.")
        return

    cat_scores: dict[int, list[float]] = {}
    cat_counts: dict[int, int] = {}
    for qa in all_qa:
        cat = qa.get("category", 0)
        pred = qa.get(pred_key, "")
        gold = str(qa.get("answer", ""))
        if pred:
            f1 = compute_f1(pred, gold)
            cat_scores.setdefault(cat, []).append(f1)
        cat_counts[cat] = cat_counts.get(cat, 0) + 1

    total = len(all_qa)
    scored = sum(len(v) for v in cat_scores.values())

    print(f"\n{'='*50}")
    print(f"  Live Report: Mode {mode.upper()} ({scored}/{total} scored)")
    print(f"{'='*50}")
    print(f"{'Category':<15} {'Done':>6} {'Avg F1':>8} {'Min':>8} {'Max':>8}")
    print(f"{'-'*15} {'-'*6} {'-'*8} {'-'*8} {'-'*8}")

    all_scores = []
    for cat in sorted(cat_scores.keys()):
        scores = cat_scores[cat]
        avg = sum(scores) / len(scores)
        lo = min(scores)
        hi = max(scores)
        name = CATEGORY_NAMES.get(cat, f"cat-{cat}")
        print(f"{name:<15} {len(scores):>6} {avg:>8.3f} {lo:>8.3f} {hi:>8.3f}")
        all_scores.extend(scores)

    overall = sum(all_scores) / len(all_scores) if all_scores else 0
    print(f"{'-'*15} {'-'*6} {'-'*8} {'-'*8} {'-'*8}")
    print(f"{'Overall':<15} {len(all_scores):>6} {overall:>8.3f}")

    # Show sample completion status
    result_file = output_dir / f"mnemon_{mode}_qa.json"
    if result_file.exists():
        samples = json.loads(result_file.read_text())
        print(f"\n  Completed samples: {len(samples)}")
        for s in samples:
            print(f"    {s['sample_id']}: {len(s['qa'])} QA")

    # Show in-progress
    for cp in sorted(output_dir.glob(f"_checkpoint_{mode}_*.json")):
        qa = json.loads(cp.read_text())
        sid = cp.stem.replace(f"_checkpoint_{mode}_", "")
        print(f"  In progress: {sid} ({len(qa)} QA done)")

    print()


def main():
    bench_dir = Path(__file__).parent.parent
    output_dir = bench_dir / "outputs"
    mode = sys.argv[1] if len(sys.argv) > 1 else "raw"
    print_live(output_dir, mode)


if __name__ == "__main__":
    main()
