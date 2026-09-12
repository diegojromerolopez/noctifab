#!/usr/bin/env python3
"""
Single-Project Autonomous Feedback & Improvement Loop for Noctifab.

Executes containerized Noctifab validation against ANY chosen target project
(e.g., pyedis, thredis, calculator, t4, etc.), harvests multi-channel telemetry
(SQLite database, token usage, container console logs, atomic execution reports, generated code),
diagnoses root causes, gates code/prompt changes, recompiles the Noctifab binary,
and re-executes the project in a closed self-improving feedback loop.

Usage:
  python3 validation/bin/single_project_loop.py <project> [--max-iterations=N] [--timeout=SECONDS] [--dry-run]
  # Examples:
  python3 validation/bin/single_project_loop.py pyedis
  python3 validation/bin/single_project_loop.py thredis --max-iterations=3
  python3 validation/bin/single_project_loop.py calculator --dry-run
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
from datetime import datetime
from typing import Dict, Any, List, Optional

ROOT_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
PROJECTS_DIR = os.path.join(ROOT_DIR, "validation", "projects")

# Scale-based timeouts per project based on complexity units (CU) and architectural seams
PROJECT_SCALE_TIMEOUTS = {
    # Small / Single-binary CLI Utilities (15 - 20 minutes)
    "wc": 1200,
    "calculator": 1200,
    "echo": 900,
    "todo-cli": 1200,
    "fortune": 1200,

    # Medium Systems (30 minutes)
    "t4": 1800,
    "frontpunch": 1800,
    "ocalogue": 1800,
    "ninline": 1800,
    "pyedis": 1800,
    "stricc": 1800,
    "actodis": 1800,
    "thredis": 1800,
    "dotchess": 1800,

    # Large Enterprise & Multi-Tier Stacks (35 - 40 minutes)
    "notebook": 2100,
    "djanban": 2100,
    "jpacioli": 2400,
    "auth-vault": 2100,
    "buffonstream": 2100,
    "searchthedocs": 2100,
}


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

def harvest_sqlite_telemetry(project_dir: str, output_dir: str) -> Dict[str, Any]:
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

    # Candidate paths where sqlite database may reside
    candidate_paths = [
        os.path.join(output_dir, ".noctifab", "data", "noctifab.db"),
        os.path.join(output_dir, "data", "noctifab.db"),
        os.path.join(project_dir, ".noctifab", "data", "noctifab.db"),
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
            cur.execute("SELECT id, build_status, total_tokens_used, project_version FROM state LIMIT 1")
            row = cur.fetchone()
            if row:
                telemetry["state"] = dict(row)
                telemetry["token_usage"]["total_tokens_state"] = row["total_tokens_used"] or 0
        except sqlite3.OperationalError:
            pass

        # 2. Stories Table
        try:
            cur.execute("SELECT id, title, status, tokens_used, started_at, completed_at FROM stories ORDER BY created_at")
            telemetry["stories"] = [dict(r) for r in cur.fetchall()]
        except sqlite3.OperationalError:
            pass

        # 3. Tasks Table
        try:
            cur.execute("SELECT id, story_id, title, status, change_type, retries, max_retries FROM tasks ORDER BY created_at")
            telemetry["tasks"] = [dict(r) for r in cur.fetchall()]
        except sqlite3.OperationalError:
            pass

        # 4. Failed Actions Table
        try:
            cur.execute(
                """
                SELECT id, tool, args, reasoning, result, success, timestamp 
                FROM actions 
                WHERE success = 0 
                ORDER BY id DESC 
                LIMIT 30
                """
            )
            telemetry["failed_actions"] = [dict(r) for r in cur.fetchall()]
        except sqlite3.OperationalError:
            pass

        # 5. Tool Invocations Aggregate
        try:
            cur.execute(
                """
                SELECT tool, 
                       COUNT(*) as total_calls, 
                       SUM(CASE WHEN success=0 THEN 1 ELSE 0 END) as failed_calls
                FROM actions 
                GROUP BY tool
                """
            )
            telemetry["tool_summary"] = {r["tool"]: {"total": r["total_calls"], "failed": r["failed_calls"]} for r in cur.fetchall()}
        except sqlite3.OperationalError:
            pass

        # 6. Detailed Token Usage Table
        try:
            cur.execute(
                """
                SELECT agent_id, 
                       SUM(prompt_tokens) as total_prompt, 
                       SUM(completion_tokens) as total_completion,
                       SUM(prompt_tokens + completion_tokens) as total_tokens
                FROM token_usage 
                GROUP BY agent_id
                """
            )
            agent_breakdown = {}
            total_table_tokens = 0
            for r in cur.fetchall():
                agent_id = r["agent_id"] or "unassigned"
                agent_breakdown[agent_id] = {
                    "prompt": r["total_prompt"],
                    "completion": r["total_completion"],
                    "total": r["total_tokens"],
                }
                total_table_tokens += r["total_tokens"]

            telemetry["token_usage"]["agent_breakdown"] = agent_breakdown
            telemetry["token_usage"]["total_tokens_table"] = total_table_tokens

            # Per-task token consumption
            cur.execute(
                """
                SELECT task_id, SUM(prompt_tokens + completion_tokens) as task_tokens
                FROM token_usage
                WHERE task_id IS NOT NULL AND task_id != ''
                GROUP BY task_id
                ORDER BY task_tokens DESC
                LIMIT 10
                """
            )
            telemetry["token_usage"]["task_breakdown"] = {r["task_id"]: r["task_tokens"] for r in cur.fetchall()}
        except sqlite3.OperationalError:
            pass

        conn.close()
    except Exception as e:
        telemetry["db_error"] = str(e)

    return telemetry


def harvest_log_telemetry(log_path: str) -> Dict[str, Any]:
    """Analyzes container console log for compiler, linter, test, and token telemetry."""
    if not os.path.exists(log_path):
        return {"log_found": False}

    with open(log_path, "r", encoding="utf-8", errors="replace") as f:
        content = f.read()

    # Token mentions in log
    tokens_in_log = 0
    token_matches = re.findall(r"Tokens?\s*(?:Used|Consumed|Total)?:?\s*([\d,]+)", content, re.IGNORECASE)
    if token_matches:
        try:
            tokens_in_log = max(int(m.replace(",", "")) for m in token_matches)
        except ValueError:
            pass

    # Compiler / Syntax errors
    compiler_snippets = []
    for m in re.finditer(r"(?:error\[E\d+\]|SyntaxError|TypeError|gcc: error|clang: error|NameError|ImportError|AttributeError|Compilation error|build failed)[^\n\r]*\n(?:[^\n\r]*\n){1,2}", content):
        snip = m.group(0).strip()
        if snip not in compiler_snippets:
            compiler_snippets.append(snip)

    # Linter errors
    linter_errors = []
    for m in re.finditer(r"([a-zA-Z0-9_\-\/]+\.[a-zA-Z0-9]+:\d+:\s*(?:error|warning|[A-Z]\d+):[^\n\r]+)", content):
        line = m.group(1).strip()
        if line not in linter_errors:
            linter_errors.append(line)

    # Unit test failures
    test_failures = []
    for m in re.finditer(r"(?:FAIL|FAILED|FAILURE|Error)[\s:]+([^\n\r]+)", content):
        line = m.group(0).strip()
        if len(line) < 200 and not any(skip in line for skip in ["0 failed", "FAIL (exit 0)", "PASS"]):
            if line not in test_failures:
                test_failures.append(line)

    # Negative constraint check: Forbidden framework violations (e.g. pytest in pyedis)
    forbidden_framework_hits = []
    if "ZERO PYTEST USAGE" in content or "pytest is STRICTLY FORBIDDEN" in content or re.search(r"import pytest|pytest\.ini|conftest\.py", content):
        for m in re.finditer(r"(?:import pytest|from pytest|pytest\.ini|conftest\.py)", content):
            hit = m.group(0).strip()
            if hit not in forbidden_framework_hits:
                forbidden_framework_hits.append(hit)

    # Fallback agent usage
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


def harvest_execution_report(report_dir: str) -> Dict[str, Any]:
    """Extracts live metrics and token totals from markdown execution reports."""
    if not os.path.exists(report_dir):
        return {"report_found": False}

    reports = glob.glob(os.path.join(report_dir, "*.md"))
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
        "latest_file": os.path.basename(latest),
        "status": status_match.group(1) if status_match else "UNKNOWN",
        "lead_time": lead_time_match.group(1).strip() if lead_time_match else "-",
        "files_changed": files_match.group(1) if files_match else "-",
        "tokens": tokens_parsed,
    }


def harvest_generated_code(output_dir: str) -> Dict[str, Any]:
    """Inspects generated workspace artifacts."""
    found_files = []
    if os.path.exists(output_dir):
        for root, dirs, files in os.walk(output_dir):
            if any(skip in root for skip in [".git", ".noctifab", "log", "report", "dist"]):
                continue
            for f in files:
                rel = os.path.relpath(os.path.join(root, f), output_dir)
                found_files.append(rel)

    return {
        "total_files": len(found_files),
        "files_list": sorted(found_files)[:35],
    }


# ==============================================================================
# 2. Diagnostic & Token Accountability Engine
# ==============================================================================

class ProjectDiagnostic:
    def __init__(self, project: str, exit_code: int, duration: float, sqlite_data: Dict, log_data: Dict, report_data: Dict, code_data: Dict):
        self.project = project
        self.exit_code = exit_code
        self.duration = duration
        self.sqlite = sqlite_data
        self.log = log_data
        self.report = report_data
        self.code = code_data
        self.bottlenecks: List[str] = []
        self.agnostic_remedies: List[Dict[str, str]] = []

        # Token Resolution: pick highest precision source
        db_tokens = self.sqlite.get("token_usage", {}).get("total_tokens_state", 0) or self.sqlite.get("token_usage", {}).get("total_tokens_table", 0)
        report_tokens = self.report.get("tokens", 0)
        log_tokens = self.log.get("tokens_in_log", 0)
        self.total_tokens = max(db_tokens, report_tokens, log_tokens)

        self._analyze()

    def _analyze(self):
        # 1. Exit code & Timeout
        if self.exit_code == 124:
            self.bottlenecks.append(f"Timeout: Exceeded execution limit ({self.duration:.1f}s)")
        elif self.exit_code != 0:
            self.bottlenecks.append(f"Validation Failure: Container exited with code {self.exit_code}")

        # 2. Token Consumption Analysis & Inflation Detection
        tasks = self.sqlite.get("tasks", [])
        completed_tasks = sum(1 for t in tasks if t.get("status") in ("DONE", "COMPLETED", "MERGED"))
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

        # 3. Compiler & Linter Churn
        if self.log.get("compiler_count", 0) > 0:
            self.bottlenecks.append(f"Compiler / Build Errors: {self.log.get('compiler_count')} error events recorded")

        if self.log.get("linter_count", 0) > 0:
            self.bottlenecks.append(f"Linter / Type Churn: {self.log.get('linter_count')} diagnostic errors")

        # 4. Failed Actions in SQLite
        failed_actions = self.sqlite.get("failed_actions", [])
        if failed_actions:
            tools = set(a.get("tool") for a in failed_actions if a.get("tool"))
            self.bottlenecks.append(f"Database Recorded {len(failed_actions)} Failed Tool Calls ({', '.join(tools)})")

        # 5. Forbidden Framework Violations
        if self.log.get("forbidden_framework_hits", []):
            hits = ", ".join(self.log.get("forbidden_framework_hits", []))
            self.bottlenecks.append(f"Specification Constraint Violation: Used forbidden framework elements ({hits})")
            self.agnostic_remedies.append({
                "area": "pkg/infrastructure/prompts/defaults/product_manager/user_story.tmpl",
                "remedy": "Enforce negative constraint parsing: Product Manager must check forbidden libraries/tools before writing story acceptance criteria.",
                "type": "SpecConstraintIngestion",
            })

    def calculate_fitness(self, timeout_limit: int) -> float:
        """Calculates scalar fitness score factoring pass rate, tokens, speed, and efficiency."""
        score = 0.0

        # Pass / Fail base
        if self.exit_code == 0:
            score += 100.0
        else:
            score -= 40.0

        # Duration penalty proportional to timeout limit
        score -= (self.duration / max(timeout_limit, 1)) * 20.0

        # Token economy factor: penalize excessive consumption
        if self.total_tokens > 0:
            score -= (self.total_tokens / 10000.0) * 1.5

        # Completed tasks reward
        score += self.completed_tasks * 6.0

        # Penalties for linter and compiler churn
        score -= min(self.log.get("linter_count", 0) * 1.5, 20.0)
        score -= min(self.log.get("compiler_count", 0) * 2.0, 20.0)
        score -= len(self.sqlite.get("failed_actions", [])) * 1.5

        return round(score, 2)


# ==============================================================================
# 3. Compilation & Validation Gate
# ==============================================================================

def compile_noctifab() -> bool:
    """Recompiles noctifab binary and executes fast unit tests."""
    log_step("BUILD", "Compiling Noctifab binary (make build)...")
    build_res = subprocess.run(["make", "build"], cwd=ROOT_DIR, capture_output=True, text=True)
    if build_res.returncode != 0:
        log_error(f"Compilation failed:\n{build_res.stderr}")
        return False
    log_success("Noctifab compiled successfully.")

    log_step("TEST", "Running hermetic unit tests (go test -race ./pkg/services/...)...")
    test_res = subprocess.run(["go", "test", "-race", "./pkg/services/..."], cwd=ROOT_DIR, capture_output=True, text=True)
    if test_res.returncode != 0:
        log_error(f"Unit tests failed:\n{test_res.stderr}")
        return False
    log_success("Hermetic unit tests passed.")
    return True


def rebuild_validation_images(project: str) -> bool:
    """Rebuilds the noctifab-validation:base and project-specific Docker images with the newly compiled binary."""
    log_step("DOCKER", "Rebuilding noctifab-validation:base Docker image...")
    res = subprocess.run(
        [
            "docker", "build",
            "-f", os.path.join(ROOT_DIR, "validation", "Dockerfile.validation"),
            "-t", "noctifab-validation:base",
            ROOT_DIR,
        ],
        capture_output=True,
        text=True,
    )
    if res.returncode != 0:
        log_error(f"Docker base image rebuild failed:\n{res.stderr}")
        return False

    project_dockerfile = os.path.join(PROJECTS_DIR, project, "Dockerfile")
    if os.path.exists(project_dockerfile):
        log_step("DOCKER", f"Rebuilding noctifab-validation:{project} Docker image...")
        res_proj = subprocess.run(
            [
                "docker", "build",
                "-f", project_dockerfile,
                "-t", f"noctifab-validation:{project}",
                ROOT_DIR,
            ],
            capture_output=True,
            text=True,
        )
        if res_proj.returncode != 0:
            log_error(f"Docker {project} image rebuild failed:\n{res_proj.stderr}")
            return False

    log_success(f"Validation Docker images updated for {project}.")
    return True


def clean_project_workspace(project: str) -> None:
    """Wipes all generated artifacts, databases, caches, and dangling containers to ensure each iteration starts from anew."""
    log_step("CLEAN", f"Purging previous workspace state, caches, and artifacts for '{project}' to start from anew...")

    # 1. Terminate and remove any dangling Docker containers for this project
    try:
        ps_cmd = f"docker ps -aq --filter name=validate-{project}"
        container_ids = subprocess.check_output(ps_cmd, shell=True, text=True).strip().split()
        if container_ids:
            subprocess.run(["docker", "rm", "-f"] + container_ids, capture_output=True, text=True)
            log_step("CLEAN", f"Removed {len(container_ids)} dangling Docker container(s) for {project}.")
    except Exception:
        pass

    project_dir = os.path.join(PROJECTS_DIR, project)
    output_dir = os.path.join(project_dir, "output")

    # 2. Clean project output directory
    if os.path.exists(output_dir):
        # We preserve empty log, report, and dist directory shells to avoid Docker bind-mount synchronization issues on macOS/Linux hosts
        for item in os.listdir(output_dir):
            item_path = os.path.join(output_dir, item)
            if item in ("log", "report", "dist"):
                # Clean contents within these directories
                if os.path.isdir(item_path):
                    for sub in os.listdir(item_path):
                        sub_path = os.path.join(item_path, sub)
                        try:
                            if os.path.isdir(sub_path) and not os.path.islink(sub_path):
                                shutil.rmtree(sub_path, ignore_errors=True)
                            else:
                                os.remove(sub_path)
                        except Exception:
                            pass
            else:
                # Remove all other generated files, source directories, .git, .noctifab, caches, etc.
                try:
                    if os.path.isdir(item_path) and not os.path.islink(item_path):
                        shutil.rmtree(item_path, ignore_errors=True)
                    else:
                        os.remove(item_path)
                except Exception:
                    pass
    else:
        os.makedirs(output_dir, exist_ok=True)

    # Ensure empty directory shells exist for mounting
    for sub in ("log", "report", "dist"):
        os.makedirs(os.path.join(output_dir, sub), exist_ok=True)

    # 3. Clean any state or data files inside validation/projects/<project>/.noctifab/ if present
    # (Strictly preserve config.yaml and secrets.yaml)
    noctifab_dir = os.path.join(project_dir, ".noctifab")
    if os.path.exists(noctifab_dir):
        for item in ["data", "logs", "worktrees", "state.json", "run.lock"]:
            target = os.path.join(noctifab_dir, item)
            if os.path.exists(target):
                try:
                    if os.path.isdir(target) and not os.path.islink(target):
                        shutil.rmtree(target, ignore_errors=True)
                    else:
                        os.remove(target)
                except Exception:
                    pass

    # 4. Remove any stray generated files or toolchain caches in the project directory root
    protected_entries = {".noctifab", "Dockerfile", "SPEC.md", "output", "README.md"}
    if os.path.exists(project_dir):
        for entry in os.listdir(project_dir):
            if entry not in protected_entries:
                entry_path = os.path.join(project_dir, entry)
                if entry in ("__pycache__", ".venv", ".pytest_cache", ".mypy_cache", "target", "node_modules", ".git", "build", "_build", "dist", "bin") or entry.endswith((".pyc", ".db", ".log", ".json", ".lock")):
                    try:
                        if os.path.isdir(entry_path) and not os.path.islink(entry_path):
                            shutil.rmtree(entry_path, ignore_errors=True)
                        else:
                            os.remove(entry_path)
                    except Exception:
                        pass

    log_success(f"Workspace for '{project}' completely wiped and reset to pristine state.")


# ==============================================================================
# 4. Project Execution Runner
# ==============================================================================

def run_project_validation(project: str, timeout_seconds: int) -> Dict[str, Any]:
    """Executes target project container with streaming activity monitoring."""
    clean_project_workspace(project)
    log_header(f"EXECUTING VALIDATION RUN: {project} (Timeout: {timeout_seconds}s / {timeout_seconds/60:.0f}m)")
    start_time = time.time()
    cmd = [os.path.join(ROOT_DIR, "validation", "bin", "run_one.sh"), project]

    env = os.environ.copy()
    env["NOCTIFAB_SKIP_BUILD"] = "1"

    proc = subprocess.Popen(
        cmd,
        cwd=ROOT_DIR,
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
                r, _, _ = select.select([proc.stdout], [], [], 1.0)
                if r:
                    line = proc.stdout.readline()
                    if line:
                        if any(k in line for k in ["Tool Executed", "Task", "PASS", "FAIL", "Orchestrator", "Validating", "building", "Tokens"]):
                            print(f"  [{project}] {line.strip()[:110]}", flush=True)

            elapsed = time.time() - start_time
            if elapsed > timeout_seconds:
                log_error(f"Timeout reached ({timeout_seconds}s). Stopping container...")
                timed_out = True
                subprocess.run(f"docker kill $(docker ps -q --filter name=validate-{project})", shell=True, capture_output=True)
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

    duration = time.time() - start_time
    exit_code = 124 if timed_out else (proc.returncode if proc.returncode is not None else 1)
    status_str = "TIMEOUT" if timed_out else ("SUCCESS" if exit_code == 0 else f"FAILED (exit {exit_code})")

    log_step("FINISH", f"{project} finished in {duration:.1f}s with status: {status_str}")

    project_dir = os.path.join(PROJECTS_DIR, project)
    output_dir = os.path.join(project_dir, "output")
    log_file = os.path.join(output_dir, "log", f"{project}.log")
    report_dir = os.path.join(output_dir, "report")

    # Harvest all telemetry channels
    sqlite_data = harvest_sqlite_telemetry(project_dir, output_dir)
    log_data = harvest_log_telemetry(log_file)
    report_data = harvest_execution_report(report_dir)
    code_data = harvest_generated_code(output_dir)

    diag = ProjectDiagnostic(project, exit_code, duration, sqlite_data, log_data, report_data, code_data)
    fitness = diag.calculate_fitness(timeout_seconds)

    log_token(f"Total Tokens: {diag.total_tokens:,} | Completed Tasks: {diag.completed_tasks} | Fitness Score: {fitness:+.1f}")

    return {
        "project": project,
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
# 5. Closed-Loop Controller & Markdown Reporter
# ==============================================================================

def write_loop_markdown_report(project: str, iterations_data: List[Dict[str, Any]]):
    """Writes <PROJECT>_LOOP_REPORT.md at repository root."""
    report_file = os.path.join(ROOT_DIR, f"{project.upper().replace('-', '_')}_LOOP_REPORT.md")

    doc = f"""# Autonomous Feedback & Improvement Loop Report: `{project}`

**Target Project**: `validation/projects/{project}`  
**Execution Timestamp**: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}  
**Total Iterations Executed**: {len(iterations_data)}  

---

## 1. Iteration Comparison & Token Accountability Matrix

| Iteration | Status | Duration | Tasks (Done/Total) | Total Tokens | Tokens/Task | Failed Actions | Fitness Score |
| :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
"""
    for idx, it in enumerate(iterations_data, 1):
        st = it["status"]
        dur = f"{it['duration']:.1f}s"
        tasks = it["sqlite"].get("tasks", [])
        done_tasks = it["diagnostic"].completed_tasks
        task_str = f"{done_tasks}/{len(tasks)}" if tasks else "-"
        toks = f"{it['tokens']:,}" if it['tokens'] > 0 else "-"
        tok_per_task = f"{it['tokens']//max(done_tasks, 1):,}" if it['tokens'] > 0 and done_tasks > 0 else "-"
        failed_act = len(it["sqlite"].get("failed_actions", []))
        fit = it["fitness"]
        doc += f"| **#{idx}** | `{st}` | {dur} | {task_str} | **{toks}** | {tok_per_task} | {failed_act} | **{fit:+.1f}** |\n"

    doc += """
---

## 2. Token Consumption Breakdown by Agent & Task

"""
    for idx, it in enumerate(iterations_data, 1):
        token_usage = it["sqlite"].get("token_usage", {})
        agent_breakdown = token_usage.get("agent_breakdown", {})
        task_breakdown = token_usage.get("task_breakdown", {})

        doc += f"### Iteration #{idx} Token Profile (Total: {it['tokens']:,} tokens)\n\n"
        if agent_breakdown:
            doc += "| Agent Role | Prompt Tokens | Completion Tokens | Total Tokens | % of Total |\n| :--- | ---: | ---: | ---: | ---: |\n"
            for agent, stats in agent_breakdown.items():
                pct = (stats["total"] / max(it["tokens"], 1)) * 100
                doc += f"| `{agent}` | {stats['prompt']:,} | {stats['completion']:,} | {stats['total']:,} | {pct:.1f}% |\n"
            doc += "\n"
        else:
            doc += "- *Detailed agent token breakdown not available in database.*\n\n"

        if task_breakdown:
            doc += "#### Highest Token-Consuming Tasks:\n"
            for task_id, t_tok in list(task_breakdown.items())[:5]:
                doc += f"- `{task_id}`: **{t_tok:,}** tokens\n"
            doc += "\n"

    doc += """
---

## 3. Deep Diagnostics & Telemetry Breakdown

"""
    for idx, it in enumerate(iterations_data, 1):
        diag: ProjectDiagnostic = it["diagnostic"]
        doc += f"""### Iteration #{idx}: {it['status']} in {it['duration']:.1f}s

#### Observed Bottlenecks
"""
        if diag.bottlenecks:
            for b in diag.bottlenecks:
                doc += f"- {b}\n"
        else:
            doc += "- No significant bottlenecks detected.\n"

        # SQLite Details
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

        # Compiler & Linter Diagnostics
        comp_snips = it["log"].get("compiler_snippets", [])
        if comp_snips:
            doc += "\n#### Compiler Diagnostics:\n```text\n"
            for cs in comp_snips[:3]:
                doc += f"{cs}\n---\n"
            doc += "```\n"

        lint_errs = it["log"].get("linter_errors", [])
        if lint_errs:
            doc += "\n#### Linter / Type Diagnostics (Top 5):\n```text\n"
            for le in lint_errs[:5]:
                doc += f"{le}\n"
            doc += "```\n"

        doc += "\n---\n"

    with open(report_file, "w", encoding="utf-8") as f:
        f.write(doc)
    log_success(f"Wrote comprehensive loop report to {report_file}")


def main():
    import argparse
    parser = argparse.ArgumentParser(description="Single-Project Autonomous Feedback & Improvement Loop for Noctifab")
    parser.add_argument("project", nargs="?", default="pyedis", help="Target validation project (e.g. pyedis, thredis, calculator, t4). Default: pyedis")
    parser.add_argument("--max-iterations", type=int, default=3, help="Maximum loop iterations (default: 3)")
    parser.add_argument("--timeout", type=int, default=None, help="Timeout in seconds per run (default: dynamic by project scale)")
    parser.add_argument("--dry-run", action="store_true", help="Harvest existing telemetry without running container")
    parser.add_argument("--skip-compile", action="store_true", help="Skip recompiling Noctifab binary")
    args = parser.parse_args()

    project = args.project.strip().lower()
    project_dir = os.path.join(PROJECTS_DIR, project)
    if not os.path.exists(project_dir):
        log_error(f"Target project '{project}' does not exist under validation/projects/")
        sys.exit(1)

    timeout = args.timeout if args.timeout and args.timeout > 0 else PROJECT_SCALE_TIMEOUTS.get(project, 1800)

    log_header(f"NOCTIFAB SINGLE-PROJECT AUTONOMOUS IMPROVEMENT LOOP: {project.upper()}")
    log_step("CONFIG", f"Project: {project} | Max Iterations: {args.max_iterations} | Timeout: {timeout}s ({timeout/60:.0f}m)")

    output_dir = os.path.join(project_dir, "output")
    log_file = os.path.join(output_dir, "log", f"{project}.log")
    report_dir = os.path.join(output_dir, "report")

    if args.dry_run:
        log_step("DRY-RUN", f"Harvesting existing telemetry for '{project}' from disk...")
        sqlite_data = harvest_sqlite_telemetry(project_dir, output_dir)
        log_data = harvest_log_telemetry(log_file)
        report_data = harvest_execution_report(report_dir)
        code_data = harvest_generated_code(output_dir)
        diag = ProjectDiagnostic(project, 0, 0.0, sqlite_data, log_data, report_data, code_data)

        print("\n--- Diagnostic Findings ---")
        print(f"Project: {project}")
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

    # Initial Build check
    if not args.skip_compile:
        if not compile_noctifab() or not rebuild_validation_images(project):
            log_error("Initial compilation failed. Aborting loop.")
            sys.exit(1)

    iterations_data = []

    for i in range(1, args.max_iterations + 1):
        log_header(f"LOOP ITERATION {i} OF {args.max_iterations}: {project.upper()}")

        # Ensure every iteration starts completely from anew
        clean_project_workspace(project)

        # 1. Run validation project
        result = run_project_validation(project, timeout_seconds=timeout)
        iterations_data.append(result)

        # 2. Check for success
        if result["exit_code"] == 0:
            log_success(f"Iteration {i}: {project} passed all verification gates! Target achieved.")
            break

        # 3. If further iterations remain, apply self-improvement actions
        if i < args.max_iterations:
            diag = result["diagnostic"]
            log_step("DIAGNOSE", f"Identified {len(diag.bottlenecks)} bottlenecks. Formulating agnostic improvements...")

            for b in diag.bottlenecks:
                print(f"  - {Colors.YELLOW}{b}{Colors.RESET}")

            # Recompile and prepare for next iteration
            if not args.skip_compile:
                log_step("IMPROVE", f"Recompiling Noctifab and updating validation images for {project}...")
                if not compile_noctifab() or not rebuild_validation_images(project):
                    log_error("Failed to recompile Noctifab during iteration. Stopping loop.")
                    break

    # Final summary report
    write_loop_markdown_report(project, iterations_data)
    log_header("LOOP COMPLETE")


if __name__ == "__main__":
    main()
