"""Prompt templates for LLM-supervised LoCoMo benchmark."""

INGESTION_PROMPT = """\
You are extracting structured memories from a conversation session.
Speakers: {person1} and {person2}. Date: {date}.

Rules:
- Extract 5-{max_memories} memories — err on the side of extracting MORE, not less
- Each memory must be self-contained (include who/what/when context)
- PRESERVE specific details: proper nouns (names, places, book titles, brands), \
exact numbers, dates, and concrete objects — NEVER generalize these \
(e.g., store "Sweden" not "her home country", store "Becoming Nicole" not "a book")
- Convert relative dates to absolute: "yesterday" on {date} → compute the actual date; \
"last year" on {date} → compute the actual year
- Extract even small/casual details: gifts mentioned, activities with family, \
hobbies, food, places visited, objects described — these matter for QA
- Include temporal context: "{date}, {person1}..."
- Keep each memory under 500 characters

Session transcript:
{session_text}

Respond with ONLY a JSON array (no markdown, no explanation):
[{{"content": "...", "category": "fact|preference|decision|context|insight", "importance": 1-5, "entities": "entity1,entity2"}}]"""

LINK_EVAL_PROMPT = """\
A memory was just stored. The system found potential connections. \
Evaluate each candidate and decide whether to create a link.

New memory: "{content}"

Semantic candidates (similar topics):
{semantic_candidates}

Causal candidates (cause/effect relationships):
{causal_candidates}

For each genuine relationship, include it in the output.
Respond with ONLY a JSON array (no markdown, no explanation):
[{{"target_id": "...", "type": "semantic|causal", "weight": 0.3-0.9}}]
Return [] if no candidates warrant linking."""

QUERY_REFORMULATION_PROMPT = """\
Reformulate this question into an optimal memory recall query.
The memory system supports intent types:
- WHY: causal chains, reasons, motivations
- WHEN: temporal, dates, ordering, timelines
- ENTITY: about a specific person, place, or thing
- GENERAL: balanced retrieval

Question: {question}
Category hint: {category} ({category_name})

Respond with ONLY JSON (no markdown, no explanation):
{{"query": "concise keyword-rich query under 200 chars", "intent": "WHY|WHEN|ENTITY|GENERAL"}}"""

ANSWER_PROMPT = """\
Below is a conversation between {person1} and {person2}.
The following are relevant memories recalled from their conversations:

{context}

Answer the following question using ONLY the information in the memories above.
- Use exact words, names, dates, and numbers from the context — do not paraphrase
- Keep your answer as a short phrase (not a full sentence)
- If the context does NOT contain enough information to answer, \
respond with exactly: "no information available"
{category_suffix}
Question: {question}
Short answer:"""

# Category-specific suffixes matching EasyLocomo's format
CATEGORY_SUFFIXES = {
    1: "",  # single-hop / multi-answer
    2: "If the question asks about a date or time, approximate using the dates of the conversations.",
    3: "",  # open-domain
    4: "",  # multi-hop
    5: "",  # adversarial (handled separately)
}

CATEGORY_NAMES = {
    1: "single-hop",
    2: "temporal",
    3: "open-domain",
    4: "multi-hop",
    5: "adversarial",
}

LLM_JUDGE_PROMPT = """\
You are evaluating whether a predicted answer is semantically correct \
compared to the ground truth answer for a question about a conversation.

Question: {question}
Ground truth answer: {gold}
Predicted answer: {prediction}

Evaluate on a scale:
- CORRECT: The prediction conveys the same meaning as the ground truth, even if worded differently. \
"last year" = "2022" if the conversation was in 2023. "the previous day" = "May 7" if talking about May 8.
- PARTIAL: The prediction captures some but not all key information from the ground truth.
- WRONG: The prediction is factually incorrect or irrelevant.

Respond with ONLY one JSON object (no markdown):
{{"verdict": "CORRECT|PARTIAL|WRONG", "reason": "one sentence explanation"}}"""
