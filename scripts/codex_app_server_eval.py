#!/usr/bin/env python3
"""Run Mnemon harness checks against the real Codex app-server."""

from __future__ import annotations

import argparse
import json
import os
import queue
import shutil
import subprocess
import sys
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


class JsonRpcError(RuntimeError):
    pass


class CodexAppServer:
    def __init__(self, env: dict[str, str], cwd: Path, stderr_log: Path) -> None:
        self.env = env
        self.cwd = cwd
        self.stderr_log = stderr_log
        self.proc: subprocess.Popen[str] | None = None
        self.next_id = 1
        self.responses: dict[int, dict[str, Any]] = {}
        self.notifications: list[dict[str, Any]] = []
        self.lines: queue.Queue[str | None] = queue.Queue()
        self.reader: threading.Thread | None = None
        self.stderr_reader: threading.Thread | None = None

    def start(self) -> None:
        self.stderr_log.parent.mkdir(parents=True, exist_ok=True)
        err = self.stderr_log.open("w", encoding="utf-8")
        self.proc = subprocess.Popen(
            ["codex", "app-server", "--listen", "stdio://"],
            cwd=self.cwd,
            env=self.env,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
        )

        def read_stdout() -> None:
            assert self.proc is not None and self.proc.stdout is not None
            try:
                for line in self.proc.stdout:
                    self.lines.put(line)
            finally:
                self.lines.put(None)

        def read_stderr() -> None:
            assert self.proc is not None and self.proc.stderr is not None
            try:
                for line in self.proc.stderr:
                    err.write(line)
                    err.flush()
            finally:
                err.close()

        self.reader = threading.Thread(target=read_stdout, daemon=True)
        self.stderr_reader = threading.Thread(target=read_stderr, daemon=True)
        self.reader.start()
        self.stderr_reader.start()

    def close(self) -> None:
        if self.proc is None:
            return
        if self.proc.poll() is None:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.proc.kill()
                self.proc.wait(timeout=5)

    def request(self, method: str, params: dict[str, Any] | None = None, timeout: float = 30.0) -> dict[str, Any]:
        if self.proc is None or self.proc.stdin is None:
            raise JsonRpcError("app-server is not running")
        request_id = self.next_id
        self.next_id += 1
        payload: dict[str, Any] = {"jsonrpc": "2.0", "id": request_id, "method": method}
        if params is not None:
            payload["params"] = params
        self.proc.stdin.write(json.dumps(payload) + "\n")
        self.proc.stdin.flush()
        return self._wait_response(request_id, timeout)

    def _wait_response(self, request_id: int, timeout: float) -> dict[str, Any]:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if request_id in self.responses:
                response = self.responses.pop(request_id)
                if "error" in response:
                    raise JsonRpcError(json.dumps(response["error"], indent=2))
                return response.get("result", {})

            remaining = max(0.1, deadline - time.monotonic())
            try:
                line = self.lines.get(timeout=min(0.5, remaining))
            except queue.Empty:
                if self.proc is not None and self.proc.poll() is not None:
                    raise JsonRpcError(f"app-server exited with code {self.proc.returncode}")
                continue

            if line is None:
                raise JsonRpcError("app-server stdout closed")
            line = line.strip()
            if not line:
                continue
            try:
                message = json.loads(line)
            except json.JSONDecodeError as exc:
                raise JsonRpcError(f"invalid JSON-RPC line: {line}") from exc

            if "id" in message and message.get("id") is not None:
                self.responses[int(message["id"])] = message
            else:
                self.notifications.append(message)

        raise JsonRpcError(f"timed out waiting for response id {request_id}")

    def wait_notification(self, method: str, timeout: float = 120.0) -> dict[str, Any]:
        deadline = time.monotonic() + timeout
        start = 0
        while time.monotonic() < deadline:
            for item in self.notifications[start:]:
                if item.get("method") == method:
                    return item
            start = len(self.notifications)
            try:
                line = self.lines.get(timeout=0.5)
            except queue.Empty:
                if self.proc is not None and self.proc.poll() is not None:
                    raise JsonRpcError(f"app-server exited with code {self.proc.returncode}")
                continue
            if line is None:
                raise JsonRpcError("app-server stdout closed")
            line = line.strip()
            if not line:
                continue
            message = json.loads(line)
            if "id" in message and message.get("id") is not None:
                self.responses[int(message["id"])] = message
            else:
                self.notifications.append(message)
                if message.get("method") == method:
                    return message
        raise JsonRpcError(f"timed out waiting for notification {method}")


def repo_root() -> Path:
    return Path(__file__).resolve().parents[1]


def utc_run_id() -> str:
    return datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")


def run(cmd: list[str], cwd: Path, env: dict[str, str]) -> None:
    subprocess.run(cmd, cwd=cwd, env=env, check=True)


def ensure_mnemon_binary(root: Path, run_dir: Path, env: dict[str, str]) -> dict[str, str]:
    if shutil.which("mnemon", path=env.get("PATH")):
        return env
    bin_dir = run_dir / "bin"
    bin_dir.mkdir(parents=True, exist_ok=True)
    run(["go", "build", "-o", str(bin_dir / "mnemon"), "."], root, env)
    next_env = dict(env)
    next_env["PATH"] = f"{bin_dir}{os.pathsep}{next_env.get('PATH', '')}"
    return next_env


def setup_workspace(args: argparse.Namespace, root: Path) -> tuple[Path, Path, Path, dict[str, str]]:
    run_root = Path(args.run_root) if args.run_root else root / ".testdata" / "codex-app-eval" / utc_run_id()
    workspace = run_root / "workspace"
    mnemon_dir = run_root / ".mnemon"
    workspace.mkdir(parents=True, exist_ok=True)
    mnemon_dir.mkdir(parents=True, exist_ok=True)

    (workspace / "README.md").write_text(
        "# Mnemon Codex App-Server Eval Workspace\n\n"
        "This workspace is generated by scripts/codex_app_server_eval.py.\n",
        encoding="utf-8",
    )

    env = dict(os.environ)
    env["MNEMON_HARNESS_STATE_DIR"] = str(mnemon_dir)
    if args.isolated_codex_home:
        codex_home = run_root / "codex-home"
        codex_home.mkdir(parents=True, exist_ok=True)
        env["CODEX_HOME"] = str(codex_home)
    env = ensure_mnemon_binary(root, run_root, env)

    install = root / "harness" / "setup" / "install.sh"
    modules = args.modules
    for module in modules:
        cmd = ["bash", str(install), "--host", "codex", "--module", module, "--config-dir", str(workspace / ".codex")]
        run(cmd, workspace, env)
    return run_root, workspace, mnemon_dir, env


def collect_skill_names(skills_result: dict[str, Any]) -> set[str]:
    names: set[str] = set()

    def walk(value: Any) -> None:
        if isinstance(value, dict):
            name = value.get("name")
            if isinstance(name, str):
                names.add(name)
            for child in value.values():
                walk(child)
        elif isinstance(value, list):
            for child in value:
                walk(child)

    walk(skills_result)
    return names


def run_eval(args: argparse.Namespace) -> dict[str, Any]:
    root = repo_root()
    run_dir, workspace, mnemon_dir, env = setup_workspace(args, root)
    report_dir = run_dir / "reports"
    report_dir.mkdir(parents=True, exist_ok=True)
    logs_dir = run_dir / "logs"
    logs_dir.mkdir(parents=True, exist_ok=True)

    server = CodexAppServer(env=env, cwd=workspace, stderr_log=logs_dir / "codex-app-server.stderr.log")
    report: dict[str, Any] = {
        "schema_version": 1,
        "run_dir": str(run_dir),
        "workspace": str(workspace),
        "mnemon_dir": str(mnemon_dir),
        "modules": args.modules,
        "agent_turn": args.agent_turn,
        "started_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    }

    try:
        server.start()
        initialized = server.request(
            "initialize",
            {"clientInfo": {"name": "mnemon-codex-app-server-eval", "version": "0.1.0"}},
            timeout=30,
        )
        skills = server.request("skills/list", {"cwds": [str(workspace)], "forceReload": True}, timeout=30)
        skill_names = collect_skill_names(skills)
        expected = set(args.expected_skills)
        missing = sorted(expected - skill_names)
        if missing:
            raise JsonRpcError(f"missing projected Codex skills: {', '.join(missing)}")

        thread = server.request(
            "thread/start",
            {
                "cwd": str(workspace),
                "approvalPolicy": "never",
                "sandbox": "danger-full-access",
                "ephemeral": True,
                "developerInstructions": (
                    "You are running inside a Mnemon harness eval workspace. "
                    "Use repo-local Codex skills when they are relevant. "
                    f"Mnemon state is isolated at {mnemon_dir}."
                ),
            },
            timeout=30,
        )
        thread_id = thread.get("thread", {}).get("id")
        if not isinstance(thread_id, str) or not thread_id:
            raise JsonRpcError("thread/start did not return a thread id")

        report["initialize"] = initialized
        report["skill_names"] = sorted(skill_names)
        report["thread_id"] = thread_id

        if args.agent_turn:
            server.request(
                "turn/start",
                {
                    "threadId": thread_id,
                    "input": [{"type": "text", "text": args.prompt}],
                    "cwd": str(workspace),
                    "approvalPolicy": "never",
                    "sandboxPolicy": {"type": "dangerFullAccess"},
                },
                timeout=30,
            )
            completed = server.wait_notification("turn/completed", timeout=args.turn_timeout)
            report["turn_completed"] = completed

        report["status"] = "ok"
        return report
    except Exception as exc:
        report["status"] = "failed"
        report["error"] = str(exc)
        raise
    finally:
        server.close()
        report["finished_at"] = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
        report_path = report_dir / "codex-app-server-eval.json"
        report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        print(f"report: {report_path}")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-root", help="Use a specific eval run directory instead of .testdata/codex-app-eval/<timestamp>.")
    parser.add_argument(
        "--module",
        dest="modules",
        action="append",
        choices=["memory-loop", "skill-loop"],
        default=[],
        help="Harness module to install. May be repeated. Defaults to memory-loop.",
    )
    parser.add_argument(
        "--expected-skill",
        dest="expected_skills",
        action="append",
        default=[],
        help="Projected Codex skill name that must appear in skills/list. Defaults are derived from selected modules.",
    )
    parser.add_argument("--agent-turn", action="store_true", help="Start a real Codex turn after app-server smoke checks.")
    parser.add_argument(
        "--prompt",
        default=(
            "In one short sentence, confirm that you can see the Mnemon repo-local skills. "
            "Do not modify files."
        ),
        help="Prompt used with --agent-turn.",
    )
    parser.add_argument("--turn-timeout", type=float, default=180.0, help="Seconds to wait for turn/completed.")
    parser.add_argument(
        "--isolated-codex-home",
        action="store_true",
        help="Set CODEX_HOME inside the eval run directory. This is suitable for smoke checks and may not have auth for real turns.",
    )
    args = parser.parse_args(argv)
    if not args.modules:
        args.modules = ["memory-loop"]
    if not args.expected_skills:
        expected: list[str] = []
        if "memory-loop" in args.modules:
            expected.extend(["memory_get", "memory_set"])
        if "skill-loop" in args.modules:
            expected.extend(["skill_observe", "skill_curate", "skill_manage"])
        args.expected_skills = expected
    return args


def main(argv: list[str]) -> int:
    try:
        report = run_eval(parse_args(argv))
    except Exception as exc:
        print(f"codex app-server eval failed: {exc}", file=sys.stderr)
        return 1
    print(json.dumps({"status": report["status"], "run_dir": report["run_dir"]}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
