"""Benchmark configuration.

Default: all LLM calls route through CLIProxyAPI at http://127.0.0.1:3456.
Both ingestion (Haiku) and answer generation (Sonnet) use the same proxy.
Override via environment variables if needed.
"""

import os
from dataclasses import dataclass, field
from pathlib import Path

# CLIProxyAPI defaults
_PROXY_BASE = "http://127.0.0.1:3456"
_PROXY_KEY = "not-needed"


@dataclass
class Config:
    # Paths
    bench_dir: Path = field(default_factory=lambda: Path(__file__).parent.parent)
    data_path: Path = field(default=None)
    output_dir: Path = field(default=None)
    ingested_dir: Path = field(default=None)
    mnemon_data_dir: Path = field(default=None)

    # Mnemon
    mnemon_bin: str = "mnemon"

    # LLM for ingestion / link eval / query reformulation
    ingestion_api_base: str = _PROXY_BASE
    ingestion_api_key: str = _PROXY_KEY
    ingestion_model: str = "claude-sonnet-4-6"

    # LLM for answer generation
    answer_api_base: str = _PROXY_BASE
    answer_api_key: str = _PROXY_KEY
    answer_model: str = "claude-sonnet-4-6"

    # Benchmark params
    recall_limit: int = 15
    max_memories_per_session: int = 25
    temperature: float = 0.0
    max_answer_tokens: int = 128
    concurrency: int = 5  # parallel LLM calls for QA phase

    def __post_init__(self):
        if self.data_path is None:
            self.data_path = self.bench_dir / "testdata" / "locomo" / "locomo10.json"
        if self.output_dir is None:
            self.output_dir = self.bench_dir / "outputs"
        if self.ingested_dir is None:
            self.ingested_dir = self.bench_dir / "testdata" / "locomo" / "ingested"
        if self.mnemon_data_dir is None:
            self.mnemon_data_dir = self.bench_dir / ".mnemon_bench"

        # Env overrides (CLIProxyAPI defaults if not set)
        self.ingestion_api_base = os.getenv("INGESTION_API_BASE", self.ingestion_api_base)
        self.ingestion_api_key = os.getenv("INGESTION_API_KEY", self.ingestion_api_key)
        self.ingestion_model = os.getenv("INGESTION_MODEL", self.ingestion_model)
        self.answer_api_base = os.getenv("ANSWER_API_BASE", self.answer_api_base)
        self.answer_api_key = os.getenv("ANSWER_API_KEY", self.answer_api_key)
        self.answer_model = os.getenv("ANSWER_MODEL", self.answer_model)
