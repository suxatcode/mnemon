"""Phase 2: LLM-supervised recall and QA for LoCoMo benchmark."""

import json

from .config import Config
from .llm import call_llm
from .mnemon_client import MnemonClient
from .prompts import (
    ANSWER_PROMPT,
    CATEGORY_NAMES,
    CATEGORY_SUFFIXES,
    QUERY_REFORMULATION_PROMPT,
)


async def reformulate_query(
    question: str,
    category: int,
    config: Config,
) -> dict:
    """Use LLM to reformulate a question into an optimal recall query."""
    prompt = QUERY_REFORMULATION_PROMPT.format(
        question=question,
        category=category,
        category_name=CATEGORY_NAMES.get(category, "unknown"),
    )
    response = await call_llm(
        prompt=prompt,
        api_base=config.ingestion_api_base,
        api_key=config.ingestion_api_key,
        model=config.ingestion_model,
        temperature=config.temperature,
        max_tokens=256,
    )
    try:
        text = response.strip()
        if text.startswith("```"):
            text = text.split("\n", 1)[-1].rsplit("```", 1)[0].strip()
        result = json.loads(text)
        return {
            "query": result.get("query", question),
            "intent": result.get("intent", "GENERAL"),
        }
    except (json.JSONDecodeError, KeyError):
        return {"query": question, "intent": "GENERAL"}


def format_recall_context(recall_result: dict | list) -> str:
    """Format mnemon recall results into readable context for the answer LLM."""
    # Smart recall returns {"results": [...], "meta": {...}}
    if isinstance(recall_result, dict):
        results = recall_result.get("results", [])
    else:
        results = recall_result  # basic recall returns a list

    if not results:
        return "(No relevant memories found.)"

    lines = []
    for i, r in enumerate(results, 1):
        if isinstance(r, dict) and "insight" in r:
            insight = r["insight"]
            content = insight.get("content", "")
            cat = insight.get("category", "")
            imp = insight.get("importance", "")
            score = r.get("score", 0)
            lines.append(
                f"[Memory {i}] (importance: {imp}, category: {cat}, relevance: {score:.2f})\n{content}"
            )
        elif isinstance(r, dict):
            content = r.get("content", "")
            cat = r.get("category", "")
            imp = r.get("importance", "")
            lines.append(f"[Memory {i}] (importance: {imp}, category: {cat})\n{content}")

    return "\n\n".join(lines)


async def answer_question(
    question: str,
    category: int,
    person1: str,
    person2: str,
    client: MnemonClient,
    config: Config,
    mode: str = "raw",
) -> dict:
    """Answer a single LoCoMo question using mnemon recall.

    Args:
        question: the question text
        category: QA category (1-5)
        person1, person2: speaker names
        client: MnemonClient bound to the sample's store
        config: benchmark config
        mode: "raw" (Mode D) or "full" (Mode E)

    Returns:
        dict with prediction, recall_meta, etc.
    """
    # Step 1: Determine recall query
    if mode == "full":
        reformulated = await reformulate_query(question, category, config)
        query = reformulated["query"]
        intent = reformulated["intent"]
    else:
        query = question
        intent = None

    # Step 2: Recall from mnemon
    recall_result = client.recall(
        query=query,
        limit=config.recall_limit,
        intent=intent,
    )

    # Step 3: Format context
    context = format_recall_context(recall_result)

    # Step 4: Generate answer
    cat_suffix = CATEGORY_SUFFIXES.get(category, "")
    prompt = ANSWER_PROMPT.format(
        person1=person1,
        person2=person2,
        context=context,
        question=question,
        category_suffix=cat_suffix,
    )

    prediction = await call_llm(
        prompt=prompt,
        api_base=config.answer_api_base,
        api_key=config.answer_api_key,
        model=config.answer_model,
        temperature=config.temperature,
        max_tokens=config.max_answer_tokens,
    )

    # Collect metadata
    meta = {}
    if isinstance(recall_result, dict):
        meta = recall_result.get("meta", {})
        meta["num_results"] = len(recall_result.get("results", []))
    else:
        meta["num_results"] = len(recall_result)

    if mode == "full":
        meta["reformulated_query"] = query
        meta["reformulated_intent"] = intent

    return {
        "prediction": prediction.strip(),
        "recall_meta": meta,
        "context_length": len(context),
    }
