"""Python wrapper for mnemon CLI via subprocess."""

import json
import subprocess
from pathlib import Path


class MnemonClient:
    """Calls mnemon binary via subprocess with isolated data directory."""

    def __init__(self, mnemon_bin: str, data_dir: Path, store: str | None = None):
        self.mnemon_bin = mnemon_bin
        self.data_dir = data_dir
        self.store = store

    def _base_args(self) -> list[str]:
        args = [self.mnemon_bin, "--data-dir", str(self.data_dir)]
        if self.store:
            args.extend(["--store", self.store])
        return args

    def _run(self, args: list[str], check: bool = True) -> subprocess.CompletedProcess:
        result = subprocess.run(
            args, capture_output=True, text=True, timeout=30
        )
        if check and result.returncode != 0:
            raise RuntimeError(
                f"mnemon failed: {' '.join(args)}\nstderr: {result.stderr}"
            )
        return result

    def _run_json(self, args: list[str]) -> dict | list:
        result = self._run(args)
        return json.loads(result.stdout)

    # -- Store management --

    def store_create(self, name: str) -> bool:
        result = self._run(self._base_args() + ["store", "create", name], check=False)
        return result.returncode == 0

    def store_set(self, name: str) -> bool:
        result = self._run(self._base_args() + ["store", "set", name], check=False)
        return result.returncode == 0

    def store_remove(self, name: str) -> bool:
        result = self._run(self._base_args() + ["store", "remove", name], check=False)
        return result.returncode == 0

    def store_list(self) -> str:
        result = self._run(self._base_args() + ["store", "list"])
        return result.stdout

    # -- Core operations --

    def remember(
        self,
        content: str,
        cat: str = "general",
        imp: int = 3,
        entities: str = "",
        source: str = "agent",
        no_diff: bool = False,
    ) -> dict:
        args = self._base_args() + [
            "remember", content,
            "--cat", cat,
            "--imp", str(imp),
            "--source", source,
        ]
        if entities:
            args.extend(["--entities", entities])
        if no_diff:
            args.append("--no-diff")
        return self._run_json(args)

    def recall(
        self,
        query: str,
        limit: int = 10,
        intent: str | None = None,
        basic: bool = False,
    ) -> dict | list:
        args = self._base_args() + ["recall", query, "--limit", str(limit)]
        if intent:
            args.extend(["--intent", intent])
        if basic:
            args.append("--basic")
        return self._run_json(args)

    def link(
        self,
        source_id: str,
        target_id: str,
        edge_type: str = "semantic",
        weight: float = 0.5,
    ) -> dict:
        args = self._base_args() + [
            "link", source_id, target_id,
            "--type", edge_type,
            "--weight", str(weight),
        ]
        return self._run_json(args)

    def forget(self, insight_id: str) -> dict:
        return self._run_json(self._base_args() + ["forget", insight_id])

    def status(self) -> dict:
        return self._run_json(self._base_args() + ["status"])

    # -- Convenience --

    def with_store(self, store_name: str) -> "MnemonClient":
        """Return a new client bound to a specific store."""
        return MnemonClient(self.mnemon_bin, self.data_dir, store_name)
