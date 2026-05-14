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
from typing import Any, Callable


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
    return datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")


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
    env["MNEMON_DATA_DIR"] = str(mnemon_dir / "data")
    if "memory-loop" in args.modules:
        env["MNEMON_MEMORY_LOOP_ENV"] = str(mnemon_dir / "harness" / "memory-loop" / "env.sh")
        env["MNEMON_MEMORY_LOOP_DIR"] = str(mnemon_dir / "harness" / "memory-loop")
    if "skill-loop" in args.modules:
        skill_dir = mnemon_dir / "harness" / "skill-loop"
        env["MNEMON_SKILL_LOOP_ENV"] = str(skill_dir / "env.sh")
        env["MNEMON_SKILL_LOOP_DIR"] = str(skill_dir)
        env["MNEMON_SKILL_LOOP_LIBRARY_DIR"] = str(skill_dir / "skills")
        env["MNEMON_SKILL_LOOP_ACTIVE_DIR"] = str(skill_dir / "skills" / "active")
        env["MNEMON_SKILL_LOOP_STALE_DIR"] = str(skill_dir / "skills" / "stale")
        env["MNEMON_SKILL_LOOP_ARCHIVED_DIR"] = str(skill_dir / "skills" / "archived")
        env["MNEMON_SKILL_LOOP_USAGE_FILE"] = str(skill_dir / "skills" / ".usage.jsonl")
        env["MNEMON_SKILL_LOOP_PROPOSALS_DIR"] = str(skill_dir / "proposals")
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


def all_strings(value: Any) -> list[str]:
    strings: list[str] = []
    if isinstance(value, str):
        strings.append(value)
    elif isinstance(value, dict):
        for child in value.values():
            strings.extend(all_strings(child))
    elif isinstance(value, list):
        for child in value:
            strings.extend(all_strings(child))
    return strings


def combined_text(value: Any) -> str:
    return "\n".join(all_strings(value))


def command_notifications(notifications: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [item for item in notifications if "commandExecution" in combined_text(item)]


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


class Scenario:
    def __init__(
        self,
        name: str,
        modules: list[str],
        expected_skills: list[str],
        prompt: str,
        setup: Callable[[Path, Path, dict[str, str]], None],
        assert_result: Callable[[dict[str, Any], Path, Path, dict[str, str]], list[dict[str, Any]]],
    ) -> None:
        self.name = name
        self.modules = modules
        self.expected_skills = expected_skills
        self.prompt = prompt
        self.setup = setup
        self.assert_result = assert_result


def setup_none(workspace: Path, mnemon_dir: Path, env: dict[str, str]) -> None:
    del workspace, mnemon_dir, env


def setup_memory_seed(workspace: Path, mnemon_dir: Path, env: dict[str, str]) -> None:
    del mnemon_dir
    run(
        [
            "mnemon",
            "remember",
            "Project decision: Mnemon harness validation should prefer the real Codex app-server for host integration checks.",
            "--cat",
            "decision",
            "--imp",
            "5",
            "--tags",
            "harness,codex,eval",
            "--entities",
            "Codex app-server,Mnemon harness",
        ],
        workspace,
        env,
    )


def setup_local_fact(workspace: Path, mnemon_dir: Path, env: dict[str, str]) -> None:
    del mnemon_dir, env
    (workspace / "FACTS.md").write_text(
        "# Local Facts\n\n"
        "- The local release color is cerulean.\n",
        encoding="utf-8",
    )


def assert_contains(report: dict[str, Any], text: str, needle: str, label: str) -> dict[str, Any]:
    passed = needle.lower() in text.lower()
    return {"name": label, "passed": passed, "expected": needle}


def assert_file_contains(path: Path, needle: str, label: str) -> dict[str, Any]:
    content = path.read_text(encoding="utf-8") if path.exists() else ""
    return {"name": label, "passed": needle.lower() in content.lower(), "path": str(path), "expected": needle}


def assert_file_not_contains(path: Path, needle: str, label: str) -> dict[str, Any]:
    content = path.read_text(encoding="utf-8") if path.exists() else ""
    return {"name": label, "passed": needle.lower() not in content.lower(), "path": str(path), "rejected": needle}


def assert_memory_recall(report: dict[str, Any], workspace: Path, mnemon_dir: Path, env: dict[str, str]) -> list[dict[str, Any]]:
    del workspace, mnemon_dir, env
    command_text = report.get("command_text", "")
    text = report.get("notification_text", "")
    return [
        assert_contains(report, command_text, "mnemon recall", "agent ran mnemon recall"),
        assert_contains(report, text, "Codex app-server", "agent used recalled Codex app-server decision"),
    ]


def assert_memory_skip_local(report: dict[str, Any], workspace: Path, mnemon_dir: Path, env: dict[str, str]) -> list[dict[str, Any]]:
    del workspace, mnemon_dir, env
    command_text = report.get("command_text", "")
    text = report.get("notification_text", "")
    return [
        {"name": "agent skipped mnemon recall for local-only answer", "passed": "mnemon recall" not in command_text.lower()},
        assert_contains(report, text, "cerulean", "agent answered from local context"),
    ]


def assert_memory_write(report: dict[str, Any], workspace: Path, mnemon_dir: Path, env: dict[str, str]) -> list[dict[str, Any]]:
    del report, workspace, env
    memory_file = mnemon_dir / "harness" / "memory-loop" / "MEMORY.md"
    return [
        assert_file_contains(memory_file, "app-server eval scenarios", "memory file recorded durable eval-scenario decision"),
        assert_file_contains(memory_file, "source:", "memory entry kept source metadata"),
    ]


def assert_memory_no_pollution(report: dict[str, Any], workspace: Path, mnemon_dir: Path, env: dict[str, str]) -> list[dict[str, Any]]:
    del report, workspace, env
    memory_file = mnemon_dir / "harness" / "memory-loop" / "MEMORY.md"
    return [
        assert_file_not_contains(memory_file, "742913", "memory file skipped transient token"),
    ]


def assert_skill_observe(report: dict[str, Any], workspace: Path, mnemon_dir: Path, env: dict[str, str]) -> list[dict[str, Any]]:
    del report, workspace, env
    usage_file = mnemon_dir / "harness" / "skill-loop" / "skills" / ".usage.jsonl"
    content = usage_file.read_text(encoding="utf-8") if usage_file.exists() else ""
    return [
        {"name": "skill usage log exists", "passed": usage_file.exists(), "path": str(usage_file)},
        {"name": "skill evidence mentions reusable eval workflow", "passed": "eval-runner workflow" in content.lower(), "path": str(usage_file)},
    ]


SCENARIOS: dict[str, Scenario] = {
    "memory-skip-local": Scenario(
        name="memory-skip-local",
        modules=["memory-loop"],
        expected_skills=["memory_get", "memory_set"],
        setup=setup_local_fact,
        prompt=(
            "Answer using only visible workspace files. What is the local release color in FACTS.md? "
            "Do not use memory when the answer is already local."
        ),
        assert_result=assert_memory_skip_local,
    ),
    "memory-focused-recall": Scenario(
        name="memory-focused-recall",
        modules=["memory-loop"],
        expected_skills=["memory_get", "memory_set"],
        setup=setup_memory_seed,
        prompt=(
            "Use the Mnemon memory loop if it is relevant. "
            "Question: for this project, what host integration validation mode should be preferred? "
            "Answer in one sentence and cite the memory signal you used."
        ),
        assert_result=assert_memory_recall,
    ),
    "memory-write-decision": Scenario(
        name="memory-write-decision",
        modules=["memory-loop"],
        expected_skills=["memory_get", "memory_set"],
        setup=setup_none,
        prompt=(
            "Use the Mnemon memory loop to record this durable project decision: "
            "future loop optimization should be driven by app-server eval scenarios before broad host expansion. "
            "Edit only the Mnemon memory-loop MEMORY.md in this eval workspace. "
            "Use the phrase 'app-server eval scenarios' in the saved memory. Then reply done."
        ),
        assert_result=assert_memory_write,
    ),
    "memory-no-pollution": Scenario(
        name="memory-no-pollution",
        modules=["memory-loop"],
        expected_skills=["memory_get", "memory_set"],
        setup=setup_none,
        prompt=(
            "Temporary task token 742913 is for this turn only and has no future value. "
            "Do not save it to memory. Reply with a short acknowledgement."
        ),
        assert_result=assert_memory_no_pollution,
    ),
    "skill-observe-evidence": Scenario(
        name="skill-observe-evidence",
        modules=["skill-loop"],
        expected_skills=["skill_observe", "skill_curate", "skill_manage"],
        setup=setup_none,
        prompt=(
            "Use the Mnemon skill loop to record lightweight evidence that the eval-runner workflow "
            "is reusable for loop quality checks. Append one JSONL evidence item to the configured usage log. "
            "Use note text containing 'eval-runner workflow'. Do not create or patch skills. Then reply done."
        ),
        assert_result=assert_skill_observe,
    ),
}


DEFAULT_SUITE = [
    "memory-skip-local",
    "memory-focused-recall",
    "memory-write-decision",
    "memory-no-pollution",
    "skill-observe-evidence",
]


def scenario_args(base: argparse.Namespace, scenario: Scenario) -> argparse.Namespace:
    args = argparse.Namespace(**vars(base))
    args.modules = scenario.modules
    args.expected_skills = scenario.expected_skills
    args.prompt = scenario.prompt
    args.agent_turn = True
    return args


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
        "scenario": args.scenario,
        "agent_turn": args.agent_turn,
        "started_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    }

    try:
        scenario = SCENARIOS.get(args.scenario) if args.scenario else None
        if scenario is not None:
            scenario.setup(workspace, mnemon_dir, env)

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

        report["notifications"] = server.notifications
        report["notification_methods"] = sorted({str(item.get("method")) for item in server.notifications if item.get("method")})
        report["notification_text"] = combined_text(server.notifications)
        report["command_text"] = combined_text(command_notifications(server.notifications))

        assertions: list[dict[str, Any]] = []
        if scenario is not None:
            assertions = scenario.assert_result(report, workspace, mnemon_dir, env)
        report["assertions"] = assertions
        failed = [item for item in assertions if not item.get("passed")]
        if failed:
            report["status"] = "failed"
            raise JsonRpcError("scenario assertions failed: " + ", ".join(str(item.get("name")) for item in failed))

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
        "--scenario",
        choices=sorted(SCENARIOS),
        help="Run a named real-turn scenario with scenario-specific setup and assertions.",
    )
    parser.add_argument(
        "--suite",
        action="store_true",
        help="Run the default real-turn scenario suite.",
    )
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


def run_suite(args: argparse.Namespace) -> dict[str, Any]:
    root = repo_root()
    suite_root = Path(args.run_root) if args.run_root else root / ".testdata" / "codex-app-eval-suite" / utc_run_id()
    suite_root.mkdir(parents=True, exist_ok=True)
    reports = []
    for name in DEFAULT_SUITE:
        scenario = SCENARIOS[name]
        current = scenario_args(args, scenario)
        current.scenario = name
        current.run_root = str(suite_root / name)
        try:
            report = run_eval(current)
            reports.append({"scenario": name, "status": report["status"], "run_dir": report["run_dir"]})
        except Exception as exc:
            reports.append({"scenario": name, "status": "failed", "error": str(exc), "run_dir": str(suite_root / name)})
    summary = {
        "schema_version": 1,
        "suite_root": str(suite_root),
        "reports": reports,
        "status": "ok" if all(item["status"] == "ok" for item in reports) else "failed",
    }
    summary_path = suite_root / "suite-report.json"
    summary_path.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    print(f"suite report: {summary_path}")
    return summary


def main(argv: list[str]) -> int:
    try:
        args = parse_args(argv)
        if args.suite:
            report = run_suite(args)
            print(json.dumps({"status": report["status"], "suite_root": report["suite_root"]}, indent=2))
            return 0 if report["status"] == "ok" else 1
        if args.scenario:
            scenario = SCENARIOS[args.scenario]
            args = scenario_args(args, scenario)
        report = run_eval(args)
    except Exception as exc:
        print(f"codex app-server eval failed: {exc}", file=sys.stderr)
        return 1
    print(json.dumps({"status": report["status"], "run_dir": report["run_dir"]}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
