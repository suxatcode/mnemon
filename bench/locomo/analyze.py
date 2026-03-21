"""Cross-mode comparison analysis for LoCoMo benchmark results."""

import json
import sys
from pathlib import Path

from .prompts import CATEGORY_NAMES


def load_results(path: Path) -> list[dict]:
    with open(path) as f:
        return json.load(f)


def compute_f1(prediction: str, ground_truth: str) -> float:
    """Token-level F1 score."""
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


def extract_scores(results: list[dict], pred_key: str) -> dict[int, list[float]]:
    """Extract per-category F1 scores from results."""
    cat_scores: dict[int, list[float]] = {}
    for sample in results:
        for qa in sample["qa"]:
            cat = qa["category"]
            pred = qa.get(pred_key, "")
            gold = str(qa["answer"])
            if pred:
                f1 = compute_f1(pred, gold)
                cat_scores.setdefault(cat, []).append(f1)
    return cat_scores


def compare(output_dir: Path):
    """Compare all available mode results and print comparison table."""
    modes = {}
    for f in output_dir.glob("mnemon_*_qa.json"):
        mode_name = f.stem.replace("mnemon_", "").replace("_qa", "")
        results = load_results(f)
        pred_key = f"mnemon_{mode_name}_prediction"
        scores = extract_scores(results, pred_key)
        modes[mode_name] = scores

    if not modes:
        print(f"No result files found in {output_dir}")
        return

    # Collect all categories
    all_cats = sorted(set(c for scores in modes.values() for c in scores))
    mode_names = sorted(modes.keys())

    # Header
    header = f"{'Category':<15}"
    for m in mode_names:
        header += f" {m.upper():>10}"
    print(f"\n{'='*len(header)}")
    print("  LoCoMo Benchmark: Cross-Mode Comparison")
    print(f"{'='*len(header)}")
    print(header)
    print(f"{'-'*15}" + f" {'-'*10}" * len(mode_names))

    # Per-category rows
    overall = {m: [] for m in mode_names}
    for cat in all_cats:
        name = CATEGORY_NAMES.get(cat, f"cat-{cat}")
        row = f"{name:<15}"
        for m in mode_names:
            scores = modes[m].get(cat, [])
            avg = sum(scores) / len(scores) if scores else 0
            row += f" {avg:>10.3f}"
            overall[m].extend(scores)
        print(row)

    # Overall
    print(f"{'-'*15}" + f" {'-'*10}" * len(mode_names))
    row = f"{'Overall':<15}"
    for m in mode_names:
        scores = overall[m]
        avg = sum(scores) / len(scores) if scores else 0
        row += f" {avg:>10.3f}"
    print(row)
    print()

    # Delta analysis
    if "raw" in modes and "full" in modes:
        print("  Delta (FULL - RAW):")
        for cat in all_cats:
            name = CATEGORY_NAMES.get(cat, f"cat-{cat}")
            raw_scores = modes["raw"].get(cat, [])
            full_scores = modes["full"].get(cat, [])
            raw_avg = sum(raw_scores) / len(raw_scores) if raw_scores else 0
            full_avg = sum(full_scores) / len(full_scores) if full_scores else 0
            delta = full_avg - raw_avg
            sign = "+" if delta >= 0 else ""
            print(f"    {name:<15} {sign}{delta:.3f}")
        print()


def main():
    output_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).parent.parent / "outputs"
    compare(output_dir)


if __name__ == "__main__":
    main()
