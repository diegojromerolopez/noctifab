#!/usr/bin/env python3
"""
Native Project Autonomous Feedback & Improvement Loop for Noctifab.

Executes Noctifab natively on the host filesystem against an actual project directory
(e.g., $HOME/repos/pyedis) without Docker containers. Harvests multi-channel telemetry
(SQLite database, token usage, console logs, atomic execution reports, generated code),
diagnoses root causes, recompiles the Noctifab binary, and re-executes in a closed
self-improving feedback loop.

Usage:
  python3 validation/bin/native_project_loop.py <project_path> [--max-iterations=N] [--timeout=SECONDS] [--dry-run]
  # Examples:
  python3 validation/bin/native_project_loop.py ~/repos/pyedis
  python3 validation/bin/native_project_loop.py /Users/diegoj/repos/pyedis --max-iterations=3
  python3 validation/bin/native_project_loop.py ~/repos/pyedis --dry-run
"""

import os
import sys
import time
import subprocess
import glob
import re
import sqlite3
import json
import shutil
import signal
from datetime import datetime
from typing import Dict, Any, List, Optional

ROOT_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))


class Colors:
    HEADER = "\033[95m"
    BLUE = "\033[94m"
    CYAN = "\033[96m"
    GREEN = "\033[92m"
    YELLOW = "\033[93m"
    RED = "\033[91m"
    MAGENTA = "\033[35m"
    BOLD = "\033[1m"
    DIM = "\033[2m"
    RESET = "\033[0m"


def log_header(msg: str):
    print(f"\n{Colors.BOLD}{Colors.CYAN}{'='*80}{Colors.RESET}")
    print(f"{Colors.BOLD}{Colors.CYAN}  {msg}{Colors.RESET}")
    print(f"{Colors.BOLD}{Colors.CYAN}{'='*80}{Colors.RESET}", flush=True)


def log_step(step: str, msg: str):
    ts = datetime.now().strftime("%H:%M:%S")
    print(f"[{ts}] {Colors.BOLD}{Colors.BLUE}[{step}]{Colors.RESET} {msg}", flush=True)


def log_success(msg: str):
    ts = datetime.now().strftime("%H:%M:%S")
    print(f"[{ts}] {Colors.BOLD}{Colors.GREEN}✔ {msg}{Colors.RESET}", flush=True)


def log_warn(msg: str):
    ts = datetime.now().strftime("%H:%M:%S")
    print(f"[{ts}] {Colors.BOLD}{Colors.YELLOW}⚠ {msg}{Colors.RESET}", flush=True)


def log_error(msg: str):
    ts = datetime.now().strftime("%H:%M:%S")
    print(f"[{ts}] {Colors.BOLD}{Colors.RED}✖ {msg}{Colors.RESET}", flush=True)


def log_token(msg: str):
    ts = datetime.now().strftime("%H:%M:%S")
    print(f"[{ts}] {Colors.BOLD}{Colors.MAGENTA}📊 [TOKENS]{Colors.RESET} {msg}", flush=True)


# ==============================================================================
# 1. Telemetry Harvesters
# ==============================================================================

def harvest_sqlite_telemetry(project_dir: str) -> Dict[str, Any]:
    """Extracts operational state and token accounting directly from Noctifab's SQLite database."""
    telemetry = {
        "db_found": False,
        "db_path": "",
        "state": {},
        "stories": [],
        "tasks": [],
        "failed_actions": [],
        "tool_summary": {},
        "token_usage": {
            "total_tokens_state": 0,
            "total_tokens_table": 0,
            "agent_breakdown": {},
            "task_breakdown": {},
        },
    }

    candidate_paths = [
        os.path.join(project_dir, ".noctifab", "data", "noctifab.db"),
        os.path.join(project_dir, "data", "noctifab.db"),
        os.path.join(project_dir, ".noctifab", "noctifab.db"),
    ]
    actual_db = next((p for p in candidate_paths if os.path.exists(p) and os.path.getsize(p) > 0), None)

    if not actual_db:
        return telemetry

    telemetry["db_found"] = True
    telemetry["db_path"] = actual_db

    try:
        conn = sqlite3.connect(f"file:{actual_db}?mode=ro", uri=True)
        conn.row_factory = sqlite3.Row
        cur = conn.cursor()

        # 1. State Table
        try:
            cur.execute("SELECT id, project_path, total_tokens, execution_status, current_phase FROM states ORDER BY updated_at DESC LIMIT 1")
            row = cur.fetchone()
            if row:
                telemetry["state"] = dict(row)
                telemetry["token_usage"]["total_tokens_state"] = row["total_tokens"] or 0
        except sqlite3.OperationalError:
            pass

        # 2. Stories Table
        try:
            cur.execute("SELECT id, title, status FROM stories")
            telemetry["stories"] = [dict(r) for r in cur.fetchall()]
        except sqlite3.OperationalError:
            pass

        # 3. Tasks Table
        try:
            cur.execute("SELECT id, story_id, title, status, attempts, error_message FROM tasks")
            telemetry["tasks"] = [dict(r) for r in cur.fetchall()]
        except sqlite3.OperationalError:
            pass

        # 4. Actions Table
        try:
            cur.execute("SELECT id, task_id, tool, success, reasoning, result FROM actions WHERE success = 0")
            telemetry["failed_actions"] = [dict(r) for r in cur.fetchall()]
        except sqlite3.OperationalError:
            pass

        # 5. Token Usage Accounting
        try:
            cur.execute("SELECT role, SUM(input_tokens + output_tokens) as total FROM token_usage GROUP BY role")
            for r in cur.fetchall():
                telemetry["token_usage"]["agent_breakdown"][r["role"] or "unknown"] = r["total"]

            cur.execute("SELECT SUM(input_tokens + output_tokens) as grand_total FROM token_usage")
            total_row = cur.fetchone()
            if total_row and total_row["grand_total"]:
                telemetry["token_usage"]["total_tokens_table"] = total_row["grand_total"]
        except sqlite3.OperationalError:
            pass

        conn.close()
    except Exception as e:
        log_warn(f"Failed to harvest SQLite telemetry from {actual_db}: {e}")

    return telemetry


def harvest_log_telemetry(log_path: str) -> Dict[str, Any]:
    """Analyzes console log for compiler, linter, test, and token telemetry."""
    if not os.path.exists(log_path):
        return {"log_found": False}

    with open(log_path, "r", encoding="utf-8", errors="replace") as f:
        content = f.read()

    tokens_in_log = 0
    token_matches = re.findall(r"Tokens?\s*(?:Used|Consumed|Total)?:?\s*([\d,]+)", content, re.IGNORECASE)
    if token_matches:
        try:
            tokens_in_log = max(int(m.replace(",", "")) for m in token_matches)
        except ValueError:
            pass

    compiler_snippets = []
    for m in re.finditer(r"(?:error\[E\d+\]|SyntaxError|TypeError|gcc: error|clang: error|NameError|ImportError|AttributeError|Compilation error|build failed)[^\n\r]*\n(?:[^\n\r]*\n){1,2}", content):
        snip = m.group(0).strip()
        if snip not in compiler_snippets:
            compiler_snippets.append(snip)

    linter_errors = []
    for m in re.finditer(r"([a-zA-Z0-9_\-\/]+\.[a-zA-Z0-9]+:\d+:\s*(?:error|warning|[A-Z]\d+):[^\n\r]+)", content):
        line = m.group(1).strip()
        if line not in linter_errors:
            linter_errors.append(line)

    test_failures = []
    for m in re.finditer(r"(?:FAIL|FAILED|FAILURE|Error)[\s:]+([^\n\r]+)", content):
        line = m.group(0).strip()
        if len(line) < 200 and not any(skip in line for skip in ["0 failed", "FAIL (exit 0)", "PASS"]):
            if line not in test_failures:
                test_failures.append(line)

    forbidden_framework_hits = []
    if "ZERO PYTEST USAGE" in content or "pytest is STRICTLY FORBIDDEN" in content or re.search(r"import pytest|pytest\.ini|conftest\.py", content):
        for m in re.finditer(r"(?:import pytest|from pytest|pytest\.ini|conftest\.py)", content):
            hit = m.group(0).strip()
            if hit not in forbidden_framework_hits:
                forbidden_framework_hits.append(hit)

    fallback_triggers = len(re.findall(r"fallback_agent_trigger|\[Fallback Agent\]|Role: FALLBACK|fallback_used", content, re.IGNORECASE))

    return {
        "log_found": True,
        "length_bytes": len(content),
        "tokens_in_log": tokens_in_log,
        "compiler_snippets": compiler_snippets[:5],
        "compiler_count": len(compiler_snippets),
        "linter_errors": linter_errors[:15],
        "linter_count": len(linter_errors),
        "test_failures": test_failures[:15],
        "test_failures_count": len(test_failures),
        "forbidden_framework_hits": forbidden_framework_hits,
        "fallback_triggers": fallback_triggers,
        "tail_sample": content[-2500:] if len(content) > 2500 else content,
    }


def harvest_execution_report(project_dir: str) -> Dict[str, Any]:
    """Extracts live metrics and token totals from markdown execution reports."""
    candidate_dirs = [
        os.path.join(project_dir, ".noctifab", "reports"),
        os.path.join(project_dir, ".noctifab", "report"),
        os.path.join(project_dir, "report"),
        os.path.join(project_dir, "output", "report"),
    ]

    reports = []
    for cd in candidate_dirs:
        if os.path.exists(cd):
            reports.extend(glob.glob(os.path.join(cd, "*.md")))

    if not reports:
        return {"report_found": False}

    latest = max(reports, key=os.path.getmtime)
    with open(latest, "r", encoding="utf-8", errors="replace") as f:
        content = f.read()

    status_match = re.search(r"^>\s*Status:\s*(\w+)", content, re.MULTILINE)
    lead_time_match = re.search(r"\-\s*\*\*Lead Time:\*\*\s*([^\n\r]+)", content)
    files_match = re.search(r"\-\s*\*\*Files Changed:\*\*\s*(\d+)", content)
    tokens_match = re.search(r"\|\s*Total Tokens\s*\|\s*([\d,]+)\s*\|", content)
    tokens_parsed = 0
    if tokens_match:
        try:
            tokens_parsed = int(tokens_match.group(1).replace(",", ""))
        except ValueError:
            pass

    return {
        "report_found": True,
        "latest_file": latest,
        "status": status_match.group(1) if status_match else "UNKNOWN",
        "lead_time": lead_time_match.group(1).strip() if lead_time_match else "-",
        "files_changed": files_match.group(1) if files_match else "-",
        "tokens": tokens_parsed,
    }


def harvest_generated_code(project_dir: str) -> Dict[str, Any]:
    """Inspects project directory to measure files, lines of code, and structure."""
    stats = {
        "total_files": 0,
        "total_lines": 0,
        "files_by_ext": {},
        "file_list": [],
    }
    if not os.path.exists(project_dir):
        return stats

    excluded_dirs = {".git", ".noctifab", "__pycache__", ".venv", "node_modules", "target", ".mypy_cache", ".pytest_cache"}

    for root, dirs, files in os.walk(project_dir):
        dirs[:] = [d for d in dirs if d not in excluded_dirs]
        for f in files:
            if f.startswith(".") or f.endswith((".pyc", ".db", ".lock", ".log")):
                continue
            fpath = os.path.join(root, f)
            relpath = os.path.relpath(fpath, project_dir)
            stats["total_files"] += 1
            stats["file_list"].append(relpath)

            ext = os.path.splitext(f)[1] or "no_ext"
            stats["files_by_ext"][ext] = stats["files_by_ext"].get(ext, 0) + 1

            try:
                with open(fpath, "r", encoding="utf-8", errors="replace") as fh:
                    stats["total_lines"] += sum(1 for _ in fh)
            except Exception:
                pass

    return stats


# ==============================================================================
# 2. Diagnostic Model & Fitness Scoring
# ==============================================================================

class ProjectDiagnostic:
    def __init__(self, project: str, exit_code: int, duration: float, sqlite_data: Dict[str, Any], log_data: Dict[str, Any], report_data: Dict[str, Any], code_data: Dict[str, Any]):
        self.project = project
        self.exit_code = exit_code
        self.duration = duration
        self.sqlite = sqlite_data
        self.log = log_data
        self.report = report_data
        self.code = code_data

        self.total_tokens = 0
        if sqlite_data.get("token_usage", {}).get("total_tokens_table", 0) > 0:
            self.total_tokens = sqlite_data["token_usage"]["total_tokens_table"]
        elif sqlite_data.get("token_usage", {}).get("total_tokens_state", 0) > 0:
            self.total_tokens = sqlite_data["token_usage"]["total_tokens_state"]
        elif report_data.get("tokens", 0) > 0:
            self.total_tokens = report_data["tokens"]
        elif log_data.get("tokens_in_log", 0) > 0:
            self.total_tokens = log_data["tokens_in_log"]

        self.bottlenecks: List[str] = []
        self.completed_tasks = 0
        self.tokens_per_task = 0.0
        self._analyze()

    def _analyze(self):
        if self.exit_code == 124:
            self.bottlenecks.append(f"Timeout: Exceeded execution limit ({self.duration:.1f}s)")
        elif self.exit_code != 0:
            self.bottlenecks.append(f"Validation Failure: Process exited with code {self.exit_code}")

        tasks = self.sqlite.get("tasks", [])
        completed_tasks = sum(1 for t in tasks if t.get("status") in ("DONE", "COMPLETED", "MERGED", "SUCCESS"))
        self.completed_tasks = completed_tasks

        if self.total_tokens > 0:
            if completed_tasks > 0:
                self.tokens_per_task = self.total_tokens / completed_tasks
                if self.tokens_per_task > 60000:
                    self.bottlenecks.append(f"Token Inefficiency: High token burn ({self.tokens_per_task:,.0f} tokens/task completed)")
            else:
                self.tokens_per_task = float(self.total_tokens)
                if self.total_tokens > 50000:
                    self.bottlenecks.append(f"Token Waste: Consumed {self.total_tokens:,} tokens with 0 completed tasks")

        if self.log.get("compiler_count", 0) > 0:
            self.bottlenecks.append(f"Compiler / Build Errors: {self.log.get('compiler_count')} error events recorded")

        if self.log.get("linter_count", 0) > 0:
            self.bottlenecks.append(f"Linter / Type Churn: {self.log.get('linter_count')} diagnostic errors")

        failed_actions = self.sqlite.get("failed_actions", [])
        if failed_actions:
            tools = set(a.get("tool") for a in failed_actions if a.get("tool"))
            self.bottlenecks.append(f"Database Recorded {len(failed_actions)} Failed Tool Calls ({', '.join(tools)})")

        if self.log.get("forbidden_framework_hits", []):
            hits = ", ".join(self.log.get("forbidden_framework_hits", []))
            self.bottlenecks.append(f"Specification Constraint Violation: Used forbidden framework elements ({hits})")

    def calculate_fitness(self, timeout_limit: int) -> float:
        score = 0.0
        if self.exit_code == 0:
            score += 100.0
        else:
            score -= 40.0

        score -= (self.duration / max(timeout_limit, 1)) * 20.0

        if self.total_tokens > 0:
            score -= (self.total_tokens / 10000.0) * 1.5

        score += self.completed_tasks * 6.0
        score -= min(self.log.get("linter_count", 0) * 1.5, 20.0)
        score -= min(self.log.get("compiler_count", 0) * 2.0, 20.0)
        score -= len(self.sqlite.get("failed_actions", [])) * 1.5

        return round(score, 2)


# ==============================================================================
# 3. Compilation & Clean Workspace
# ==============================================================================

def compile_noctifab() -> bool:
    """Recompiles noctifab binary on host and executes fast unit tests."""
    log_step("BUILD", "Compiling Noctifab binary (make build)...")
    build_res = subprocess.run(["make", "build"], cwd=ROOT_DIR, capture_output=True, text=True)
    if build_res.returncode != 0:
        log_error(f"Compilation failed:\n{build_res.stderr}")
        return False
    log_success("Noctifab compiled successfully.")

    log_step("TEST", "Running hermetic unit tests (go test -race ./pkg/services/...)...")
    test_res = subprocess.run(["go", "test", "-race", "./pkg/services/..."], cwd=ROOT_DIR, capture_output=True, text=True)
    if test_res.returncode != 0:
        log_warn(f"Some unit tests failed, but proceeding:\n{test_res.stdout[-600:]}")
    else:
        log_success("Hermetic unit tests passed.")

    return True


def kill_existing_noctifab(project_dir: str):
    """Terminates any existing noctifab process running in or on the project directory."""
    pid_file = os.path.join(project_dir, ".noctifab", "noctifab.pid")
    if os.path.exists(pid_file):
        try:
            with open(pid_file, "r") as f:
                pid = int(f.read().strip())
            os.kill(pid, signal.SIGTERM)
            time.sleep(1)
            try:
                os.kill(pid, signal.SIGKILL)
            except OSError:
                pass
        except Exception:
            pass
        try:
            os.remove(pid_file)
        except OSError:
            pass

    # Clean run.lock
    lock_file = os.path.join(project_dir, ".noctifab", "run.lock")
    if os.path.exists(lock_file):
        try:
            os.remove(lock_file)
        except OSError:
            pass


def clean_project_workspace(project_dir: str) -> None:
    """Wipes generated files and caches while strictly preserving SPEC.md, config.yaml, secrets.yaml, and git repo."""
    log_step("CLEAN", f"Purging previous workspace state and caches in '{project_dir}'...")
    kill_existing_noctifab(project_dir)

    # 1. Clean .noctifab state (strictly preserving config.yaml and secrets.yaml)
    noctifab_dir = os.path.join(project_dir, ".noctifab")
    if os.path.exists(noctifab_dir):
        for item in ["data", "logs", "worktrees", "reports", "report", "state.json", "run.lock", "noctifab.pid"]:
            target = os.path.join(noctifab_dir, item)
            if os.path.exists(target):
                try:
                    if os.path.isdir(target) and not os.path.islink(target):
                        shutil.rmtree(target, ignore_errors=True)
                    else:
                        os.remove(target)
                except Exception:
                    pass

    # 2. Clean roadmap/
    roadmap_dir = os.path.join(project_dir, "roadmap")
    if os.path.exists(roadmap_dir):
        shutil.rmtree(roadmap_dir, ignore_errors=True)

    # 3. Clean generated source directories and test artifacts if they were generated
    # Protected roots/files that must never be deleted:
    protected = {
        ".git",
        ".github",
        ".noctifab",
        "SPEC.md",
        "secrets.yaml",
        ".gitignore",
        "LICENSE",
        "README.md",
        "Dockerfile",
        "Dockerfile.e2e",
        "docker-compose.e2e.yml",
    }

    if os.path.exists(project_dir):
        for entry in os.listdir(project_dir):
            if entry not in protected:
                epath = os.path.join(project_dir, entry)
                if entry in ("src", "tests", "docs", "build", "dist", "__pycache__", ".venv", ".mypy_cache", ".pytest_cache", "data", "log", "report", "output") or entry.endswith((".pyc", ".db", ".lock", ".log")):
                    try:
                        if os.path.isdir(epath) and not os.path.islink(epath):
                            shutil.rmtree(epath, ignore_errors=True)
                        else:
                            os.remove(epath)
                    except Exception:
                        pass

    # Ensure empty logs and reports directories exist inside .noctifab
    os.makedirs(os.path.join(noctifab_dir, "logs"), exist_ok=True)
    os.makedirs(os.path.join(noctifab_dir, "reports"), exist_ok=True)

    log_success(f"Workspace '{project_dir}' cleanly reset to baseline.")


# ==============================================================================
# 4. Native Project Execution Runner
# ==============================================================================

def run_filesystem_validation(project_dir: str, timeout_seconds: int, preserve_workspace: bool = False) -> Dict[str, Any]:
    """Executes target project natively on the host filesystem using Noctifab binary."""
    project_name = os.path.basename(os.path.abspath(project_dir))
    if not preserve_workspace:
        clean_project_workspace(project_dir)

    log_header(f"EXECUTING HOST VALIDATION: {project_name} at {project_dir} (Timeout: {timeout_seconds}s / {timeout_seconds/60:.0f}m)")

    bin_noctifab = os.path.join(ROOT_DIR, "dist", "noctifab")
    if not os.path.exists(bin_noctifab):
        bin_noctifab = os.path.join(ROOT_DIR, "bin", "noctifab")
    if not os.path.exists(bin_noctifab):
        compile_noctifab()
        bin_noctifab = os.path.join(ROOT_DIR, "dist", "noctifab")

    cmd = [bin_noctifab, "start", "."]

    log_dir = os.path.join(project_dir, ".noctifab", "logs")
    os.makedirs(log_dir, exist_ok=True)
    log_file = os.path.join(log_dir, f"{project_name}.log")
    log_fh = open(log_file, "w", encoding="utf-8", buffering=1)

    start_time = time.time()
    env = os.environ.copy()

    proc = subprocess.Popen(
        cmd,
        cwd=project_dir,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1,
    )

    timed_out = False

    try:
        import select
        while True:
            ret = proc.poll()
            if ret is not None:
                break

            if proc.stdout:
                r, _, _ = select.select([proc.stdout], [], [], 0.5)
                if r:
                    line = proc.stdout.readline()
                    if line:
                        log_fh.write(line)
                        log_fh.flush()
                        if any(k in line for k in ["Tool Executed", "Task", "PASS", "FAIL", "Orchestrator", "Validating", "building", "Tokens", "Story", "Merge", "Rescue", "Sovereign"]):
                            print(f"  [{project_name}] {line.strip()[:110]}", flush=True)

            elapsed = time.time() - start_time
            if elapsed > timeout_seconds:
                log_error(f"Timeout reached ({timeout_seconds}s). Stopping Noctifab process...")
                timed_out = True
                proc.terminate()
                try:
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc.kill()
                break

    except Exception as e:
        log_error(f"Exception during run: {e}")
        timed_out = True
        proc.kill()
    finally:
        log_fh.close()

    duration = time.time() - start_time
    exit_code = 124 if timed_out else (proc.returncode if proc.returncode is not None else 1)
    status_str = "TIMEOUT" if timed_out else ("SUCCESS" if exit_code == 0 else f"FAILED (exit {exit_code})")

    log_step("FINISH", f"{project_name} finished in {duration:.1f}s with status: {status_str}")

    # Harvest all telemetry channels directly from the filesystem
    sqlite_data = harvest_sqlite_telemetry(project_dir)
    log_data = harvest_log_telemetry(log_file)
    report_data = harvest_execution_report(project_dir)
    code_data = harvest_generated_code(project_dir)

    diag = ProjectDiagnostic(project_name, exit_code, duration, sqlite_data, log_data, report_data, code_data)
    fitness = diag.calculate_fitness(timeout_seconds)

    log_token(f"Total Tokens: {diag.total_tokens:,} | Completed Tasks: {diag.completed_tasks} | Fitness Score: {fitness:+.1f}")

    return {
        "project": project_name,
        "project_dir": project_dir,
        "exit_code": exit_code,
        "duration": duration,
        "timed_out": timed_out,
        "status": status_str,
        "sqlite": sqlite_data,
        "log": log_data,
        "report": report_data,
        "code": code_data,
        "diagnostic": diag,
        "tokens": diag.total_tokens,
        "fitness": fitness,
    }


# ==============================================================================
# 5. Markdown Report Writer
# ==============================================================================

def write_loop_markdown_report(project: str, project_dir: str, iterations_data: List[Dict[str, Any]], total_loop_duration: float = 0.0, target_duration_seconds: Optional[int] = None):
    """Writes <PROJECT>_LOOP_REPORT.md at repository root."""
    report_file = os.path.join(ROOT_DIR, f"{project.upper().replace('-', '_')}_LOOP_REPORT.md")

    target_str = f"{target_duration_seconds / 3600:.2f}h ({target_duration_seconds:,}s)" if target_duration_seconds else "Unbounded"
    doc = f"""# Autonomous Feedback & Improvement Loop Report: `{project}` (Host Filesystem)

**Target Project Directory**: `{project_dir}`  
**Execution Timestamp**: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}  
**Total Iterations Executed**: {len(iterations_data)}  
**Target Loop Duration**: {target_str}  
**Total Wall-Clock Time Elapsed**: {total_loop_duration / 3600:.2f}h ({total_loop_duration:.1f}s)  

---

## 1. Iteration Comparison & Token Accountability Matrix

| Iteration | Status | Duration | Tasks (Done/Total) | Total Tokens | Tokens/Task | Failed Actions | Fitness Score |
| :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
"""

    for idx, it in enumerate(iterations_data, start=1):
        tasks = it["sqlite"].get("tasks", [])
        tasks_done = it["diagnostic"].completed_tasks
        tasks_total = len(tasks)
        tasks_str = f"{tasks_done}/{tasks_total}" if tasks_total > 0 else "-"

        t_per_task = f"{it['diagnostic'].tokens_per_task:,.0f}" if tasks_done > 0 else "-"
        tokens_str = f"{it['tokens']:,}" if it["tokens"] > 0 else "-"
        failed_count = len(it["sqlite"].get("failed_actions", []))

        doc += f"| **#{idx}** | `{it['status']}` | {it['duration']:.1f}s | {tasks_str} | **{tokens_str}** | {t_per_task} | {failed_count} | **{it['fitness']:+.1f}** |\n"

    doc += "\n---\n\n## 2. Telemetry Breakdown\n"

    for idx, it in enumerate(iterations_data, start=1):
        doc += f"\n### Iteration #{idx}: {it['status']} in {it['duration']:.1f}s\n"
        doc += "\n#### Observed Bottlenecks\n"
        if it["diagnostic"].bottlenecks:
            for b in it["diagnostic"].bottlenecks:
                doc += f"- {b}\n"
        else:
            doc += "- No significant bottlenecks detected.\n"

        doc += "\n#### SQLite Operational State:\n"
        if it["sqlite"].get("db_found", False):
            doc += f"- **Database Path**: `{it['sqlite']['db_path']}`\n"
            doc += f"- **Stories Recorded**: {len(it['sqlite'].get('stories', []))}\n"
            doc += f"- **Tasks Recorded**: {len(it['sqlite'].get('tasks', []))}\n"
            failed_acts = it["sqlite"].get("failed_actions", [])
            if failed_acts:
                doc += f"- **Failed Tool Actions ({len(failed_acts)})**:\n"
                for fa in failed_acts[:5]:
                    doc += f"  - Tool `{fa.get('tool')}`: {fa.get('reasoning', '')[:90]}... (Result: {fa.get('result', '')[:80]}...)\n"
        else:
            doc += "- *SQLite database file was not present or could not be loaded.*\n"

        comp_snips = it["log"].get("compiler_snippets", [])
        if comp_snips:
            doc += "\n#### Compiler Diagnostics:\n```text\n"
            for cs in comp_snips[:3]:
                doc += f"{cs}\n---\n"
            doc += "```\n"

        doc += "\n---\n"

    with open(report_file, "w", encoding="utf-8") as f:
        f.write(doc)
    log_success(f"Wrote comprehensive loop report to {report_file}")


# ==============================================================================
# 6. Main CLI
# ==============================================================================

def main():
    import argparse
    parser = argparse.ArgumentParser(description="Native Project Autonomous Feedback & Improvement Loop for Noctifab")
    parser.add_argument("path", help="Path to project directory (e.g. ~/repos/pyedis or /Users/diegoj/repos/pyedis)")
    parser.add_argument("--max-iterations", type=int, default=None, help="Maximum loop iterations (default: 3 if no duration specified)")
    parser.add_argument("--duration-hours", type=float, default=None, help="Total duration limit in hours (e.g. 2.5)")
    parser.add_argument("--duration", type=int, default=None, help="Total duration limit in seconds")
    parser.add_argument("--timeout", type=int, default=7200, help="Timeout in seconds per run (default: 7200s / 2h)")
    parser.add_argument("--dry-run", action="store_true", help="Harvest existing telemetry without running Noctifab")
    parser.add_argument("--skip-compile", action="store_true", help="Skip recompiling Noctifab binary")
    parser.add_argument("--preserve-workspace", action="store_true", help="Do not wipe project workspace before running")
    args = parser.parse_args()

    project_dir = os.path.abspath(os.path.expanduser(args.path))
    if not os.path.exists(project_dir):
        log_error(f"Project path does not exist: {project_dir}")
        sys.exit(1)

    spec_path = os.path.join(project_dir, "SPEC.md")
    if not os.path.exists(spec_path):
        log_error(f"SPEC.md not found in project path: {spec_path}")
        sys.exit(1)

    config_path = os.path.join(project_dir, ".noctifab", "config.yaml")
    if not os.path.exists(config_path):
        log_error(f".noctifab/config.yaml not found in project path: {config_path}")
        sys.exit(1)

    project_name = os.path.basename(project_dir)
    timeout = args.timeout

    target_duration_seconds: Optional[int] = None
    if args.duration_hours is not None and args.duration_hours > 0:
        target_duration_seconds = int(args.duration_hours * 3600)
    elif args.duration is not None and args.duration > 0:
        target_duration_seconds = int(args.duration)

    if args.max_iterations is not None:
        max_iterations = args.max_iterations
    elif target_duration_seconds is not None:
        max_iterations = 999
    else:
        max_iterations = 3

    duration_str = f"{target_duration_seconds/3600:.2f}h ({target_duration_seconds:,}s)" if target_duration_seconds else "unbounded"
    log_header(f"NOCTIFAB FILESYSTEM LOOP: {project_name.upper()} ({project_dir})")
    log_step("CONFIG", f"Path: {project_dir} | Max Iterations: {max_iterations} | Loop Duration: {duration_str} | Timeout/Run: {timeout}s ({timeout/60:.0f}m)")

    if args.dry_run:
        log_step("DRY-RUN", f"Harvesting existing telemetry from {project_dir}...")
        sqlite_data = harvest_sqlite_telemetry(project_dir)
        log_file = os.path.join(project_dir, ".noctifab", "logs", f"{project_name}.log")
        log_data = harvest_log_telemetry(log_file)
        report_data = harvest_execution_report(project_dir)
        code_data = harvest_generated_code(project_dir)
        diag = ProjectDiagnostic(project_name, 0, 0.0, sqlite_data, log_data, report_data, code_data)

        print("\n--- Diagnostic Findings ---")
        print(f"Project: {project_name}")
        print(f"Database Found: {sqlite_data.get('db_found')} ({sqlite_data.get('db_path')})")
        print(f"Total Tokens: {diag.total_tokens:,}")
        print(f"Compiler Errors: {log_data.get('compiler_count', 0)}")
        print(f"Linter Errors: {log_data.get('linter_count', 0)}")
        print(f"Failed Actions: {len(sqlite_data.get('failed_actions', []))}")
        print(f"Generated Files: {code_data.get('total_files')}")
        print(f"Fitness Score: {diag.calculate_fitness(timeout)}")
        for b in diag.bottlenecks:
            print(f"  • {b}")
        sys.exit(0)

    if not args.skip_compile:
        if not compile_noctifab():
            log_error("Initial compilation failed. Aborting loop.")
            sys.exit(1)

    iterations_data = []
    loop_start_time = time.time()
    iteration_idx = 0

    while True:
        iteration_idx += 1
        elapsed_loop_time = time.time() - loop_start_time

        if target_duration_seconds is not None and elapsed_loop_time >= target_duration_seconds:
            log_step("TIME", f"Target duration limit reached ({elapsed_loop_time/3600:.2f}h / {target_duration_seconds/3600:.2f}h). Concluding loop.")
            break

        if iteration_idx > max_iterations:
            log_step("ITER", f"Max iterations ({max_iterations}) reached. Concluding loop.")
            break

        time_status = f" | Elapsed: {elapsed_loop_time/3600:.2f}h | Remaining: {(target_duration_seconds - elapsed_loop_time)/3600:.2f}h" if target_duration_seconds else ""
        log_header(f"LOOP ITERATION {iteration_idx} (Max: {max_iterations}{time_status}): {project_name.upper()}")

        result = run_filesystem_validation(project_dir, timeout_seconds=timeout, preserve_workspace=args.preserve_workspace)
        iterations_data.append(result)

        if result["exit_code"] == 0:
            log_success(f"Iteration {iteration_idx}: {project_name} passed all verification gates! Target achieved.")
            break

        elapsed_loop_time = time.time() - loop_start_time
        if target_duration_seconds is not None and elapsed_loop_time >= target_duration_seconds:
            log_step("TIME", f"Target duration limit reached after iteration {iteration_idx} ({elapsed_loop_time/3600:.2f}h / {target_duration_seconds/3600:.2f}h).")
            break

        if iteration_idx < max_iterations:
            diag = result["diagnostic"]
            log_step("DIAGNOSE", f"Identified {len(diag.bottlenecks)} bottlenecks:")
            for b in diag.bottlenecks:
                print(f"  - {Colors.YELLOW}{b}{Colors.RESET}")

            if not args.skip_compile:
                log_step("IMPROVE", f"Recompiling Noctifab for next iteration...")
                if not compile_noctifab():
                    log_error("Failed to recompile Noctifab during iteration. Stopping loop.")
                    break

    total_loop_duration = time.time() - loop_start_time
    write_loop_markdown_report(project_name, project_dir, iterations_data, total_loop_duration=total_loop_duration, target_duration_seconds=target_duration_seconds)
    log_header(f"LOOP COMPLETE: Executed {len(iterations_data)} iteration(s) in {total_loop_duration/3600:.2f}h ({total_loop_duration:.1f}s)")


if __name__ == "__main__":
    main()
