"""Phase 1: LLM-supervised ingestion of LoCoMo conversations into mnemon."""

import json
import re
from pathlib import Path

from .config import Config
from .llm import call_llm
from .mnemon_client import MnemonClient
from .prompts import INGESTION_PROMPT, LINK_EVAL_PROMPT


def parse_sessions(conversation: dict) -> list[dict]:
    """Extract sessions in chronological order from a conversation object."""
    sessions = []
    idx = 1
    while f"session_{idx}" in conversation:
        date_key = f"session_{idx}_date_time"
        sessions.append({
            "num": idx,
            "date": conversation.get(date_key, f"session {idx}"),
            "turns": conversation[f"session_{idx}"],
        })
        idx += 1
    return sessions


def format_session_text(turns: list[dict]) -> str:
    """Format session turns into readable transcript."""
    lines = []
    for turn in turns:
        speaker = turn.get("speaker", "Unknown")
        text = turn.get("text", "")
        if text:
            lines.append(f"{speaker}: {text}")
    return "\n".join(lines)


async def extract_memories(
    session_text: str,
    person1: str,
    person2: str,
    date: str,
    config: Config,
) -> list[dict]:
    """Use LLM to extract structured memories from a session transcript."""
    prompt = INGESTION_PROMPT.format(
        person1=person1,
        person2=person2,
        date=date,
        max_memories=config.max_memories_per_session,
        session_text=session_text,
    )
    response = await call_llm(
        prompt=prompt,
        api_base=config.ingestion_api_base,
        api_key=config.ingestion_api_key,
        model=config.ingestion_model,
        temperature=config.temperature,
        max_tokens=2048,
    )
    return _parse_json_array(response)


async def evaluate_candidates(
    content: str,
    semantic_candidates: list,
    causal_candidates: list,
    config: Config,
) -> list[dict]:
    """Use LLM to evaluate link candidates returned by mnemon remember."""
    if not semantic_candidates and not causal_candidates:
        return []

    prompt = LINK_EVAL_PROMPT.format(
        content=content,
        semantic_candidates=json.dumps(semantic_candidates, indent=2, ensure_ascii=False),
        causal_candidates=json.dumps(causal_candidates, indent=2, ensure_ascii=False),
    )
    response = await call_llm(
        prompt=prompt,
        api_base=config.ingestion_api_base,
        api_key=config.ingestion_api_key,
        model=config.ingestion_model,
        temperature=config.temperature,
        max_tokens=512,
    )
    return _parse_json_array(response)


async def ingest_sample(
    sample: dict,
    client: MnemonClient,
    config: Config,
    mode: str = "raw",
    cache_dir: Path | None = None,
) -> dict:
    """Ingest one LoCoMo sample into an isolated mnemon store.

    Args:
        sample: LoCoMo sample dict with 'conversation' and 'qa'
        client: MnemonClient bound to the sample's store
        config: benchmark config
        mode: "raw" (Mode D, no linking) or "full" (Mode E, with linking)
        cache_dir: optional dir to cache extracted memories JSON

    Returns:
        dict with ingestion stats
    """
    conv = sample["conversation"]
    person1 = conv["speaker_a"]
    person2 = conv["speaker_b"]
    sessions = parse_sessions(conv)

    stats = {
        "sessions": len(sessions),
        "memories_extracted": 0,
        "memories_stored": 0,
        "memories_skipped": 0,
        "links_created": 0,
        "edges_created": {"temporal": 0, "entity": 0, "causal": 0, "semantic": 0},
    }

    all_memories = []

    for session in sessions:
        session_text = format_session_text(session["turns"])
        if not session_text.strip():
            continue

        # Check cache
        cache_file = None
        if cache_dir:
            cache_file = cache_dir / f"session_{session['num']}.json"
            if cache_file.exists():
                memories = json.loads(cache_file.read_text())
            else:
                memories = await extract_memories(
                    session_text, person1, person2, session["date"], config
                )
                cache_file.write_text(json.dumps(memories, ensure_ascii=False, indent=2))
        else:
            memories = await extract_memories(
                session_text, person1, person2, session["date"], config
            )

        stats["memories_extracted"] += len(memories)

        for mem in memories:
            content = mem.get("content", "")
            if not content:
                continue

            cat = mem.get("category", "general")
            if cat not in ("preference", "decision", "fact", "insight", "context", "general"):
                cat = "general"
            imp = min(5, max(1, int(mem.get("importance", 3))))
            entities = mem.get("entities", "")

            result = client.remember(
                content=content,
                cat=cat,
                imp=imp,
                entities=entities,
                source="agent",
                no_diff=(mode == "raw"),
            )

            action = result.get("action", "")
            if action == "skipped":
                stats["memories_skipped"] += 1
                continue

            stats["memories_stored"] += 1

            # Accumulate edge stats
            edges = result.get("edges_created", {})
            for etype in ("temporal", "entity", "causal", "semantic"):
                stats["edges_created"][etype] += edges.get(etype, 0)

            # Mode E: evaluate and create links
            if mode == "full" and result.get("id"):
                sem_cands = result.get("semantic_candidates", [])
                caus_cands = result.get("causal_candidates", [])
                links = await evaluate_candidates(content, sem_cands, caus_cands, config)
                for lnk in links:
                    tid = lnk.get("target_id", "")
                    ltype = lnk.get("type", "semantic")
                    weight = float(lnk.get("weight", 0.5))
                    if tid and ltype in ("semantic", "causal"):
                        try:
                            client.link(result["id"], tid, ltype, weight)
                            stats["links_created"] += 1
                        except RuntimeError:
                            pass  # target may not exist

            all_memories.append({
                "session": session["num"],
                "content": content,
                "id": result.get("id"),
                "action": action,
            })

    return stats


def _parse_json_array(text: str) -> list[dict]:
    """Robustly parse a JSON array from LLM output."""
    text = text.strip()
    # Strip markdown code fences if present
    if text.startswith("```"):
        text = re.sub(r"^```\w*\n?", "", text)
        text = re.sub(r"\n?```$", "", text)
        text = text.strip()
    try:
        result = json.loads(text)
        if isinstance(result, list):
            return result
        return [result]
    except json.JSONDecodeError:
        # Try to find JSON array in the text
        match = re.search(r"\[.*\]", text, re.DOTALL)
        if match:
            try:
                return json.loads(match.group())
            except json.JSONDecodeError:
                pass
        return []
