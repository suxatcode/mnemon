"""Main entry point: orchestrates LoCoMo benchmark for mnemon."""

import argparse
import asyncio
import json
import shutil
import sys
import time
from pathlib import Path

from .config import Config
from .ingest import ingest_sample
from .mnemon_client import MnemonClient
from .recall_qa import answer_question


def load_data(data_path: Path) -> list[dict]:
    """Load LoCoMo dataset."""
    with open(data_path) as f:
        data = json.load(f)
    if isinstance(data, dict):
        return [data]
    return data


def get_sample_id(sample: dict, idx: int) -> str:
    """Extract or generate a sample identifier."""
    conv = sample.get("conversation", {})
    a = conv.get("speaker_a", "A")
    b = conv.get("speaker_b", "B")
    return f"{a}_{b}".replace(" ", "_").lower()


async def run_mode(
    mode: str,
    samples: list[dict],
    config: Config,
    limit_samples: int | None = None,
    limit_questions: int | None = None,
    category_filter: int | None = None,
) -> list[dict]:
    """Run one benchmark mode (raw or full) across all samples."""
    # Load existing results for resume support
    out_file = config.output_dir / f"mnemon_{mode}_qa.json"
    results = []
    completed_ids = set()
    if out_file.exists():
        results = json.loads(out_file.read_text())
        completed_ids = {r["sample_id"] for r in results}

    samples_to_run = samples[:limit_samples] if limit_samples else samples

    for idx, sample in enumerate(samples_to_run):
        sample_id = get_sample_id(sample, idx)
        store_name = f"locomo-{sample_id}"
        conv = sample["conversation"]
        person1 = conv["speaker_a"]
        person2 = conv["speaker_b"]

        # Skip already-completed samples (resume support)
        if sample_id in completed_ids:
            print(f"\n[{mode.upper()}] Sample {idx+1}/{len(samples_to_run)}: "
                  f"{person1} & {person2} — SKIPPED (already done)", flush=True)
            continue

        print(f"\n{'='*60}")
        print(f"[{mode.upper()}] Sample {idx+1}/{len(samples_to_run)}: {person1} & {person2}")
        print(f"{'='*60}", flush=True)

        # Create isolated store
        client = MnemonClient(config.mnemon_bin, config.mnemon_data_dir)
        client.store_create(store_name)
        client.store_set(store_name)
        client = client.with_store(store_name)

        # Phase 1: Ingest
        cache_dir = config.ingested_dir / sample_id
        cache_dir.mkdir(parents=True, exist_ok=True)

        print(f"  Ingesting sessions...")
        t0 = time.time()
        ingest_stats = await ingest_sample(
            sample, client, config, mode=mode, cache_dir=cache_dir
        )
        t_ingest = time.time() - t0
        print(f"  Ingested: {ingest_stats['memories_stored']} memories, "
              f"{sum(ingest_stats['edges_created'].values())} edges, "
              f"{ingest_stats.get('links_created', 0)} links "
              f"({t_ingest:.1f}s)")

        # Phase 2: QA
        qa_list = sample.get("qa", [])
        if category_filter is not None:
            qa_list = [q for q in qa_list if q.get("category") == category_filter]
        if limit_questions:
            qa_list = qa_list[:limit_questions]

        print(f"  Answering {len(qa_list)} questions (concurrency={config.concurrency})...", flush=True)

        # Checkpoint: resume from partial results if available
        checkpoint_file = config.output_dir / f"_checkpoint_{mode}_{sample_id}.json"
        completed_qa = []
        if checkpoint_file.exists():
            completed_qa = json.loads(checkpoint_file.read_text())
            print(f"  Resuming from checkpoint ({len(completed_qa)}/{len(qa_list)} done)", flush=True)

        sample_result = {
            "sample_id": sample_id,
            "person1": person1,
            "person2": person2,
            "ingest_stats": ingest_stats,
            "qa": list(completed_qa),
        }

        # Build remaining work items
        remaining = [
            (qi, qa) for qi, qa in enumerate(qa_list) if qi >= len(completed_qa)
        ]

        # Process in concurrent batches
        sem = asyncio.Semaphore(config.concurrency)
        done_count = len(completed_qa)

        async def process_one(qi: int, qa: dict) -> tuple[int, dict]:
            async with sem:
                question = qa.get("question", "")
                category = qa.get("category", 0)
                answer = qa.get("answer", "")

                try:
                    result = await answer_question(
                        question=question,
                        category=category,
                        person1=person1,
                        person2=person2,
                        client=client,
                        config=config,
                        mode=mode,
                    )
                except Exception as e:
                    print(f"    Q{qi+1} ERROR: {e}", file=sys.stderr, flush=True)
                    result = {"prediction": "", "recall_meta": {}, "context_length": 0}

                qa_out = {
                    "question": question,
                    "answer": str(answer),
                    "category": category,
                    "evidence": qa.get("evidence", []),
                    f"mnemon_{mode}_prediction": result["prediction"],
                    "recall_meta": result["recall_meta"],
                    "context_length": result["context_length"],
                }
                return qi, qa_out

        # Launch all tasks, collect as they complete
        tasks = [asyncio.create_task(process_one(qi, qa)) for qi, qa in remaining]
        # Collect results in order
        batch_results: dict[int, dict] = {}
        for coro in asyncio.as_completed(tasks):
            try:
                qi, qa_out = await coro
            except Exception as e:
                print(f"    Task failed: {e}", file=sys.stderr, flush=True)
                continue
            batch_results[qi] = qa_out
            done_count += 1
            cat = qa_out["category"]
            status = "OK" if qa_out.get(f"mnemon_{mode}_prediction") else "EMPTY"
            num_recalls = qa_out.get("recall_meta", {}).get("num_results", 0)
            print(f"    Q{qi+1}/{len(qa_list)} [cat{cat}] {status} "
                  f"({num_recalls} recalls) [{done_count}/{len(qa_list)}]", flush=True)

            # Checkpoint every 20 completions
            if done_count % 20 == 0:
                # Merge completed so far in order
                merged = list(completed_qa)
                for k in sorted(batch_results.keys()):
                    merged.append(batch_results[k])
                checkpoint_file.write_text(json.dumps(merged, ensure_ascii=False))

        # Merge all results in original order
        for qi in sorted(batch_results.keys()):
            sample_result["qa"].append(batch_results[qi])

        # Clean up checkpoint on completion
        if checkpoint_file.exists():
            checkpoint_file.unlink()

        results.append(sample_result)

        # Incremental save after each sample (crash-safe)
        out_file = config.output_dir / f"mnemon_{mode}_qa.json"
        with open(out_file, "w") as f:
            json.dump(results, f, ensure_ascii=False, indent=2)
        print(f"  Saved ({len(results)} samples so far → {out_file.name})", flush=True)

    return results


def compute_f1(prediction: str, ground_truth: str) -> float:
    """Token-level F1 score (simplified, no stemming)."""
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


def print_report(results: list[dict], mode: str):
    """Print a summary report with per-category F1."""
    from .prompts import CATEGORY_NAMES

    cat_scores: dict[int, list[float]] = {}
    pred_key = f"mnemon_{mode}_prediction"

    for sample in results:
        for qa in sample["qa"]:
            cat = qa["category"]
            pred = qa.get(pred_key, "")
            gold = str(qa["answer"])
            f1 = compute_f1(pred, gold)
            cat_scores.setdefault(cat, []).append(f1)

    print(f"\n{'='*55}")
    print(f"  Results: Mode {mode.upper()}")
    print(f"{'='*55}")
    print(f"{'Category':<15} {'Count':>6} {'Avg F1':>8}")
    print(f"{'-'*15} {'-'*6} {'-'*8}")

    all_scores = []
    for cat in sorted(cat_scores.keys()):
        scores = cat_scores[cat]
        avg = sum(scores) / len(scores) if scores else 0
        name = CATEGORY_NAMES.get(cat, f"cat-{cat}")
        print(f"{name:<15} {len(scores):>6} {avg:>8.3f}")
        all_scores.extend(scores)

    overall = sum(all_scores) / len(all_scores) if all_scores else 0
    print(f"{'-'*15} {'-'*6} {'-'*8}")
    print(f"{'Overall':<15} {len(all_scores):>6} {overall:>8.3f}")
    print()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="LoCoMo Benchmark for Mnemon")
    parser.add_argument(
        "--mode", choices=["raw", "full", "both"], default="raw",
        help="Benchmark mode: raw (Mode D), full (Mode E), or both",
    )
    parser.add_argument(
        "--limit", type=int, default=None,
        help="Limit number of samples to process",
    )
    parser.add_argument(
        "--max-questions", type=int, default=None,
        help="Limit questions per sample",
    )
    parser.add_argument(
        "--category", type=int, default=None, choices=[1, 2, 3, 4, 5],
        help="Only evaluate questions of this category",
    )
    parser.add_argument(
        "--data", type=str, default=None,
        help="Path to locomo10.json",
    )
    parser.add_argument(
        "--clean", action="store_true",
        help="Clean mnemon benchmark stores before running",
    )
    parser.add_argument(
        "--concurrency", type=int, default=None,
        help="Number of parallel LLM calls (default: 5)",
    )
    return parser.parse_args()


async def main():
    args = parse_args()
    config = Config()

    if args.concurrency:
        config.concurrency = args.concurrency

    if args.data:
        config.data_path = Path(args.data)

    if not config.data_path.exists():
        print(f"Error: dataset not found at {config.data_path}")
        print("Download it from: https://github.com/playeriv65/EasyLocomo/tree/master/data")
        sys.exit(1)

    # Clean previous benchmark data (stores + results + checkpoints)
    if args.clean:
        if config.mnemon_data_dir.exists():
            print(f"Cleaning benchmark stores at {config.mnemon_data_dir}", flush=True)
            shutil.rmtree(config.mnemon_data_dir)
        # Also clean result files and checkpoints for a truly fresh start
        if config.output_dir.exists():
            for f in config.output_dir.glob("mnemon_*_qa.json"):
                f.unlink()
            for f in config.output_dir.glob("_checkpoint_*"):
                f.unlink()

    config.output_dir.mkdir(parents=True, exist_ok=True)

    samples = load_data(config.data_path)
    print(f"Loaded {len(samples)} samples from {config.data_path}")

    modes = ["raw", "full"] if args.mode == "both" else [args.mode]

    for mode in modes:
        results = await run_mode(
            mode=mode,
            samples=samples,
            config=config,
            limit_samples=args.limit,
            limit_questions=args.max_questions,
            category_filter=args.category,
        )

        # Save results
        out_file = config.output_dir / f"mnemon_{mode}_qa.json"
        with open(out_file, "w") as f:
            json.dump(results, f, ensure_ascii=False, indent=2)
        print(f"\nResults saved to {out_file}")

        # Print report
        print_report(results, mode)


def cli():
    asyncio.run(main())


if __name__ == "__main__":
    cli()
