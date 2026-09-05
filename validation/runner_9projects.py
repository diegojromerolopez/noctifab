#!/usr/bin/env python3
"""
Sequential Validation Runner for Noctifab Target Projects:
1. calculator
2. t4
3. frontpunch
4. wc
5. notebook
6. ninline
7. jpacioli
8. ocalogue
9. djanban

Max time: 20 minutes (1200 seconds) per project.
Generates PROJECT_FEEDBACK.md and VAL_PROJECT_FEEDBACK.md.
"""

import os
import sys
import time
import subprocess
import glob
import re
from datetime import datetime

ROOT_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
PROJECTS_DIR = os.path.join(ROOT_DIR, "validation", "projects")
DEFAULT_TIMEOUT_SECONDS = 1200  # Default fallback: 20 minutes

# Scale-based timeouts per project based on complexity units (CU) and architectural seams:
# - Small CLI / Utilities (CU < 35): 15 - 20 minutes (900s - 1200s)
# - Medium Systems (CU 35 - 75): 30 minutes (1800s)
# - Large Multi-Subsystem / Enterprise (CU > 75): 35 - 40 minutes (2100s - 2400s)
PROJECT_SCALE_TIMEOUTS = {
    # Small / Single-binary CLI Utilities
    "wc": 1200,          # 20m
    "calculator": 1200,  # 20m
    "echo": 900,         # 15m
    "todo-cli": 1200,    # 20m
    "fortune": 1200,     # 20m

    # Medium Systems (Network daemons, async workers, domain logic)
    "t4": 1800,          # 30m (C HTTP daemon, networking)
    "frontpunch": 1800,  # 30m (Async task queue + Valkey)
    "ocalogue": 1800,    # 30m (Datalog deductive engine + Dune)
    "ninline": 1800,     # 30m (Connect-4 game + minimax AI)
    "pyedis": 1800,      # 30m (Redis protocol + async concurrency + AOF)
    "stricc": 1800,      # 30m (C compiler frontend + LLVM)

    # Large Enterprise & Full-Stack Monorepos
    "notebook": 2100,      # 35m (React SPA + Fastify REST + WebSockets + PostgreSQL)
    "djanban": 2100,       # 35m (Django 5.x legacy refactoring + ORM + WIP analytics)
    "jpacioli": 2400,      # 40m (Java 21 + Spring Boot + Gradle + PostgreSQL + Event Sourcing)
    "auth-vault": 2100,    # 35m (OAuth2/OIDC Zero-Trust server + PKI Vault)
    "buffonstream": 2100,  # 35m (Protobuf-native storage & CDC streaming)
    "searchthedocs": 2100, # 35m (FastAPI + Redis scraper + Vector search)
}

def get_project_timeout(project: str, override_timeout: int = None) -> int:
    if override_timeout and override_timeout > 0:
        return override_timeout
    return PROJECT_SCALE_TIMEOUTS.get(project, DEFAULT_TIMEOUT_SECONDS)

PROJECTS = [
    "calculator",
    "t4",
    "frontpunch",
    "wc",
    "notebook",
    "ninline",
    "jpacioli",
    "ocalogue",
    "djanban",
]

def parse_report(report_path: str):
    if not report_path or not os.path.exists(report_path):
        return {}
    
    with open(report_path, "r", encoding="utf-8", errors="replace") as f:
        content = f.read()
    
    data = {
        "status": "UNKNOWN",
        "lead_time": "-",
        "stories_count": "-",
        "tasks_count": "-",
        "errors_count": "-",
        "retries_count": "-",
        "tokens_count": "-",
        "files_changed": "-",
        "lines_added": "-",
        "task_efficiency": "-",
        "raw_errors": [],
        "tasks_list": [],
        "content": content,
    }
    
    status_match = re.search(r"^>\s*Status:\s*(\w+)", content, re.MULTILINE)
    if status_match:
        data["status"] = status_match.group(1)
        
    lead_time_match = re.search(r"\-\s*\*\*Lead Time:\*\*\s*([^\n\r]+)", content)
    if lead_time_match:
        data["lead_time"] = lead_time_match.group(1).strip()

    files_changed_match = re.search(r"\-\s*\*\*Files Changed:\*\*\s*(\d+)", content)
    if files_changed_match:
        data["files_changed"] = files_changed_match.group(1)

    lines_added_match = re.search(r"\-\s*\*\*Lines Added:\*\*\s*([+\-\d]+)", content)
    if lines_added_match:
        data["lines_added"] = lines_added_match.group(1)

    eff_match = re.search(r"\-\s*\*\*Task Pass Efficiency:\*\*\s*([^\n\r]+)", content)
    if eff_match:
        data["task_efficiency"] = eff_match.group(1).strip()
        
    table_match = re.search(r"## (?:Execution Status|Live Status)[\s\S]*?\|(RUNNING|SUCCESS|FAILED|CANCELLED)([\s\S]*?)\n", content)
    if table_match:
        status_val = table_match.group(1).strip()
        data["status"] = status_val
        rest = table_match.group(2).split("|")
        if len(rest) >= 8:
            data["stories_count"] = rest[2].strip()
            data["tasks_count"] = rest[3].strip()
            data["errors_count"] = rest[5].strip()
            data["retries_count"] = rest[6].strip()
            data["tokens_count"] = rest[7].strip()

    err_section = re.search(r"### Execution Errors[\s\S]*?\n\n", content)
    if err_section:
        err_lines = re.findall(r"\|\s*([A-Z0-9\-]+)\s*\|\s*([^\|]+)\s*\|\s*([^\|]+)\s*\|\s*([^\|]+)\s*\|\s*([^\|]+)\s*\|", err_section.group(0))
        for el in err_lines:
            if not el[0].startswith("Error") and not el[0].startswith("---"):
                data["raw_errors"].append({
                    "id": el[0].strip(),
                    "category": el[1].strip(),
                    "resolution": el[3].strip(),
                    "summary": el[4].strip(),
                })
            
    tasks_section = re.search(r"### Tasks[\s\S]*?\n\n", content)
    if tasks_section:
        t_lines = re.findall(r"\|\s*([^\|]+)\s*\|\s*([^\|]+)\s*\|\s*([^\|]+)\s*\|\s*([^\|]+)\s*\|\s*([^\|]+)\s*\|", tasks_section.group(0))
        for tl in t_lines:
            if not tl[0].startswith("Task") and not tl[0].startswith("---"):
                data["tasks_list"].append({
                    "title": tl[0].strip(),
                    "story": tl[1].strip(),
                    "attempts": tl[2].strip(),
                    "status": tl[3].strip(),
                    "elapsed": tl[4].strip(),
                })

    return data

def parse_log(log_path: str):
    if not log_path or not os.path.exists(log_path):
        return {}
    
    with open(log_path, "r", encoding="utf-8", errors="replace") as f:
        log_content = f.read()
        
    failing_tests = []
    for m in re.finditer(r"(?:FAIL|FAILED|FAILURE|Error)[\s:]+([^\n\r]+)", log_content):
        line = m.group(0).strip()
        if len(line) < 200 and not any(skip in line for skip in ["0 failed", "FAIL (exit 0)", "PASS"]):
            if line not in failing_tests:
                failing_tests.append(line)
                
    compiler_snippets = []
    for m in re.finditer(r"(?:error\[E\d+\]|SyntaxError|TypeError|gcc: error|clang: error|NameError|ImportError|AttributeError|Compilation error|build failed)[^\n\r]*\n(?:[^\n\r]*\n){1,3}", log_content):
        snip = m.group(0).strip()
        if snip not in compiler_snippets:
            compiler_snippets.append(snip)

    # Fallback Agent Usage Detection
    fallback_events = []
    for m in re.finditer(r"(?:\[Fallback Agent\]|🚨\s*\[CRITICAL ALERT\] Fallback Agent|fallback_agent_trigger|Escalating [^\n]+ to sovereign repair|✨\s*\[Fallback Agent\])[^\n\r]*", log_content, re.IGNORECASE):
        fallback_events.append(m.group(0).strip())

    fallback_triggers = len(re.findall(r"fallback_agent_trigger|\[Fallback Agent\]|Role: FALLBACK|fallback_used", log_content, re.IGNORECASE))

    analysis = {
        "fallback_used": fallback_triggers > 0 or len(fallback_events) > 0,
        "fallback_events": fallback_events[:10],
        "rate_limits_429": len(re.findall(r"429|Too Many Requests|retryDelay|rate limit", log_content, re.IGNORECASE)),
        "auth_errors_401_403": len(re.findall(r"401 Unauthorized|403 Forbidden", log_content, re.IGNORECASE)),
        "model_not_found_404": len(re.findall(r"404 Not Found|model_not_found", log_content, re.IGNORECASE)),
        "schema_retries": len(re.findall(r"schema retry|envelope retry|parse error|invalid json", log_content, re.IGNORECASE)),
        "linter_retries": len(re.findall(r"linter failure|linter error|consecutive linter|Linter found", log_content, re.IGNORECASE)),
        "compiler_errors": len(re.findall(r"error\[E\d+\]|compilation failed|SyntaxError|TypeError|gcc: error|build failed", log_content, re.IGNORECASE)),
        "unblocker_triggers": len(re.findall(r"\[UnblockerAgent\] Detected", log_content, re.IGNORECASE)),
        "failing_tests": failing_tests[:15],
        "compiler_snippets": compiler_snippets[:5],
        "raw_sample": log_content[-4000:] if len(log_content) > 4000 else log_content,
        "log_length": len(log_content),
    }
    return analysis

def inspect_generated_code(project_dir: str):
    output_src = os.path.join(project_dir, "output")
    files_found = []
    if os.path.exists(output_src):
        for root, dirs, files in os.walk(output_src):
            if any(p in root for p in [".git", ".noctifab", "report", "log", "dist"]):
                continue
            for file in files:
                rel = os.path.relpath(os.path.join(root, file), output_src)
                files_found.append(rel)
    return sorted(files_found)

def graceful_stop_container(project: str):
    """Sends SIGTERM to validate-<project> container, waits up to 5s for WAL checkpoint / flush, then removes if still running."""
    res = subprocess.run(f"docker ps -q --filter name=validate-{project}", shell=True, capture_output=True, text=True)
    cids = res.stdout.strip().split()
    for cid in cids:
        if not cid:
            continue
        try:
            subprocess.run(["docker", "kill", "--signal=SIGTERM", cid], capture_output=True, timeout=5)
            subprocess.run(["docker", "wait", cid], capture_output=True, timeout=5)
        except Exception:
            pass
        subprocess.run(["docker", "rm", "-f", cid], capture_output=True)

def run_project(project: str, timeout_seconds: int = None):
    if timeout_seconds is None:
        timeout_seconds = get_project_timeout(project)
    print(f"\n==================================================", flush=True)
    print(f"[{datetime.now().strftime('%H:%M:%S')}] STARTING VALIDATION: {project} (Timeout: {timeout_seconds}s / {timeout_seconds/60:.0f}m)", flush=True)
    print(f"==================================================", flush=True)
    
    start_time = time.time()
    last_activity_time = time.time()
    extensions_granted = 0
    max_extensions = 2
    extension_window = 300  # +5 minutes
    cmd = [os.path.join(ROOT_DIR, "validation", "run_one.sh"), project]
    
    env = os.environ.copy()
    env["NOCTIFAB_SKIP_BUILD"] = "1"
    
    process = subprocess.Popen(
        cmd,
        cwd=ROOT_DIR,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1,
    )
    
    timed_out = False
    stdout_lines = []
    
    try:
        import select
        while True:
            ret = process.poll()
            if ret is not None:
                break
            
            if process.stdout:
                r, _, _ = select.select([process.stdout], [], [], 1.0)
                if r:
                    line = process.stdout.readline()
                    if line:
                        stdout_lines.append(line)
                        if any(k in line for k in ["Tool Executed", "Task", "PASS", "SUCCESS", "Orchestrator", "building", "Validating"]):
                            last_activity_time = time.time()
                        if any(k in line for k in ["launching", "Validating", "building", "PASS", "FAIL", "Success", "Error", "exited", "Orchestrator", "Tool Executed", "Task", "Fallback", "CRITICAL"]):
                            print(f"  [{project}] {line.strip()[:120]}", flush=True)
            else:
                time.sleep(1.0)

            elapsed = time.time() - start_time
            if elapsed > timeout_seconds:
                # Activity-based dynamic timeout extension (PROP-5)
                recent_progress = (time.time() - last_activity_time) < 180
                if extensions_granted < max_extensions and recent_progress:
                    extensions_granted += 1
                    timeout_seconds += extension_window
                    print(f"[{datetime.now().strftime('%H:%M:%S')}] ⏳ Active progress detected within last 3m. Extending timeout for {project} by +{extension_window}s (Extension {extensions_granted}/{max_extensions}, new limit: {timeout_seconds}s / {timeout_seconds/60:.0f}m)...", flush=True)
                    continue

                print(f"[{datetime.now().strftime('%H:%M:%S')}] ❌ TIMEOUT reached for {project} (>{timeout_seconds}s). Terminating container gracefully...", flush=True)
                timed_out = True
                graceful_stop_container(project)
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                break
                
    except Exception as e:
        print(f"Exception while running {project}: {e}", flush=True)
        timed_out = True
        graceful_stop_container(project)
        process.kill()

    duration = time.time() - start_time
    exit_code = process.returncode if process.returncode is not None else 1
    if timed_out:
        exit_code = 124
        
    status_str = "TIMEOUT" if timed_out else ("SUCCESS" if exit_code == 0 else f"FAILED (exit {exit_code})")
    print(f"[{datetime.now().strftime('%H:%M:%S')}] FINISHED: {project} -> {status_str} in {duration:.1f}s", flush=True)
    
    report_dir = os.path.join(PROJECTS_DIR, project, "output", "report")
    latest_report = None
    if os.path.exists(report_dir):
        reports = glob.glob(os.path.join(report_dir, "*.md"))
        if reports:
            latest_report = max(reports, key=os.path.getmtime)
            
    report_data = parse_report(latest_report) if latest_report else {}
    log_file = os.path.join(PROJECTS_DIR, project, "output", "log", f"{project}.log")
    log_analysis = parse_log(log_file)
    generated_files = inspect_generated_code(os.path.join(PROJECTS_DIR, project))
    
    return {
        "project": project,
        "duration": duration,
        "exit_code": exit_code,
        "timed_out": timed_out,
        "timeout_limit": timeout_seconds,
        "status": status_str,
        "report_data": report_data,
        "log_analysis": log_analysis,
        "generated_files": generated_files,
    }

def write_project_feedback_md(results):
    path = os.path.join(ROOT_DIR, "PROJECT_FEEDBACK.md")
    
    doc = f"""# Noctifab Validation Projects Feedback & Deep Analysis

**Execution Date**: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}  
**Timeout Envelope**: Dynamic by Architectural Scale (Small: 20m, Medium: 30m, Large: 35-40m)  
**Target Projects**: {', '.join([r['project'] for r in results])}  

---

## 1. Validation Run Summary Table

| Project | Wall Time | Status | Stories | Tasks | Errors | Tokens | Fallback Agent Used? |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
"""
    for r in results:
        p = r["project"]
        dur = f"{r['duration']:.1f}s ({r['duration']/60:.1f}m)"
        st = r["status"]
        rep = r["report_data"]
        stories = rep.get("stories_count", "-")
        tasks = rep.get("tasks_count", "-")
        errs = rep.get("errors_count", "-")
        toks = rep.get("tokens_count", "-")
        fb_used = "🛡️ **YES**" if r["log_analysis"].get("fallback_used", False) else "No"
        doc += f"| **{p}** | {dur} | {st} | {stories} | {tasks} | {errs} | {toks} | {fb_used} |\n"

    doc += """
---

## 2. Deep Project-by-Project Insights, Issues & Proposals
"""

    for r in results:
        p = r["project"]
        dur = r["duration"]
        exit_code = r["exit_code"]
        timed_out = r["timed_out"]
        rep = r["report_data"]
        log_an = r["log_analysis"]
        gen_files = r["generated_files"]
        
        fb_used = log_an.get("fallback_used", False)
        fb_events = log_an.get("fallback_events", [])
        
        doc += f"""
### 2.{results.index(r)+1} `{p}`

- **Status**: `{r['status']}`
- **Wall Time**: {dur:.1f}s ({dur/60:.2f} minutes)
- **Artifacts Generated**: {len(gen_files)} files
- **Stories/Tasks Completed**: {rep.get('stories_count', '-')} stories, {rep.get('tasks_count', '-')} tasks
- **Tokens Consumed**: {rep.get('tokens_count', '-')}

#### Fallback Agent Utilization
"""
        if fb_used:
            doc += f"- **Fallback Agent Triggered**: YES\n"
            doc += f"- **How & When it was used**:\n"
            for ev in fb_events:
                doc += f"  - `{ev}`\n"
            doc += "- **Fallback Outcome**: Sovereign repair engaged to resolve persistent blocker or build repair.\n"
        else:
            doc += "- **Fallback Agent Triggered**: No (Standard autonomous workflow handled all tasks without escalating to sovereign repair).\n"

        doc += f"""
#### Observed Issues, Bottlenecks & Hurdles
- **Linter Retries**: {log_an.get('linter_retries', 0)}
- **Compiler / Syntax Errors**: {log_an.get('compiler_errors', 0)}
- **Rate Limit Contention (429)**: {log_an.get('rate_limits_429', 0)}
- **Schema / Envelope Retries**: {log_an.get('schema_retries', 0)}
- **Watchdog / Unblocker Invocations**: {log_an.get('unblocker_triggers', 0)}

#### Generated Key Files:
"""
        if gen_files:
            for f in gen_files[:15]:
                doc += f"- `{f}`\n"
            if len(gen_files) > 15:
                doc += f"- *(and {len(gen_files) - 15} more files)*\n"
        else:
            doc += "- *No output files generated.*\n"

        failing_tests = log_an.get("failing_tests", [])
        if failing_tests:
            doc += "\n#### Failing Edge Cases / Test Signatures:\n"
            for ft in failing_tests[:6]:
                doc += f"- `{ft}`\n"

        doc += "\n---\n"

    with open(path, "w", encoding="utf-8") as f:
        f.write(doc)
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Wrote {path}", flush=True)

def write_val_project_feedback_md(results):
    path = os.path.join(ROOT_DIR, "VAL_PROJECT_FEEDBACK.md")
    
    total_time = sum(r["duration"] for r in results)
    pass_count = sum(1 for r in results if r["exit_code"] == 0 and not r["timed_out"])
    fail_count = sum(1 for r in results if r["exit_code"] != 0 and not r["timed_out"])
    timeout_count = sum(1 for r in results if r["timed_out"])
    fallback_projects = [r["project"] for r in results if r["log_analysis"].get("fallback_used", False)]
    
    doc = f"""# Consolidated Validation Suite Feedback & Architectural Improvement Plan (`VAL_PROJECT_FEEDBACK.md`)

**Execution Date**: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}  
**Total Wall-Clock Time**: {total_time:.1f}s ({total_time/60:.2f} minutes)  
**Total Projects Evaluated**: {len(results)}  
**Pass Rate**: {pass_count}/{len(results)} ({pass_count/len(results)*100:.1f}%) | **Failures**: {fail_count} | **Timeouts**: {timeout_count}  
**Projects Utilizing Fallback Agent**: {', '.join(fallback_projects) if fallback_projects else 'None'}  

---

## 1. Comprehensive Results Matrix

| Project | Status | Spent Time | Stories | Tasks | Errors | Tokens | Fallback Agent | What Happened |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :--- |
"""

    for r in results:
        p = r["project"]
        st = "✅ **PASS**" if (r["exit_code"] == 0 and not r["timed_out"]) else ("⏰ **TIMEOUT**" if r["timed_out"] else f"❌ **FAIL ({r['exit_code']})**")
        spent = f"{r['duration']:.1f}s ({r['duration']/60:.1f}m)"
        rep = r["report_data"]
        stories = rep.get("stories_count", "-")
        tasks = rep.get("tasks_count", "-")
        errs = rep.get("errors_count", "-")
        toks = rep.get("tokens_count", "-")
        fb_str = "🛡️ Used" if r["log_analysis"].get("fallback_used", False) else "No"
        
        # Summary explanation
        t_lim = r.get("timeout_limit", 1200)
        if r["exit_code"] == 0 and not r["timed_out"]:
            expl = f"Successfully satisfied all acceptance criteria and passed verification suite with {len(r['generated_files'])} artifacts."
        elif r["timed_out"]:
            expl = f"Execution exceeded {t_lim/60:.0f}-minute limit ({r['duration']/60:.1f}m) while processing task lifecycle."
        else:
            expl = f"Harness verification exited with code {r['exit_code']}; test failures or compiler errors detected."
            
        doc += f"| **{p}** | {st} | {spent} | {stories} | {tasks} | {errs} | {toks} | {fb_str} | {expl} |\n"

    doc += """
---

## 2. Fallback Agent Cross-Suite Analysis

### 2.1 Invocation Patterns & Triggers
The Fallback Agent (Omni-Agent) serves as the sovereign last-resort recovery layer when standard generation, mutation, or linter repair cycles reach their operational ceilings. Across the 9 validation runs:
"""
    if fallback_projects:
        for p in fallback_projects:
            res_item = next(r for r in results if r["project"] == p)
            doc += f"- **`{p}`**: Fallback agent engaged. Triggers:\n"
            for ev in res_item["log_analysis"].get("fallback_events", []):
                doc += f"  - `{ev}`\n"
    else:
        doc += "- **No projects required sovereign fallback escalation**: Standard orchestrator agent loops, adaptive tier generators, and local compiler/linter self-healing resolved all tasks autonomously.\n"

    doc += """
### 2.2 Fallback Agent Recommendations & Enhancements
1. **Context-Aware Toolchain Scaffolding**: Ensure the fallback agent has direct access to environment variables, dependency paths, and clean build command overrides.
2. **Deterministic Workspace State Checkpointing**: Before invoking the fallback agent, capture a clean Git checkpoint so any speculative sovereign repair can be cleanly rolled back if tests fail.
3. **Enhanced Diagnostics Ingestion**: Pipe raw compiler errors and exact line offsets directly into the fallback sovereign repair prompt.

---

## 3. Systematic Bottlenecks, Hurdles & Root Causes

### 3.1 Task Slicing & Walking Skeleton Decomposition
- **Observation**: Larger projects (e.g. `jpacioli`, `notebook`, `djanban`) benefit greatly when the initial user story establishes a minimal walking skeleton (end-to-end executable stub + smoke test) before domain logic expansion.
- **Improvement**: Enforce DoD rules in the PM Agent prompting so Story 1 always delivers a compilable entrypoint and baseline build target.

### 3.2 Linter & Compilation Feedback Loops
- **Observation**: Repetitive linter failures on style checks (e.g. imports formatting, docstrings) consume unnecessary turns.
- **Improvement**: Automatically run deterministic code formatting (`gofmt`, `black`/`ruff`, `rustfmt`, `rubocop -a`) before invoking the LLM linter repair loop.

### 3.3 Database & Telemetry Persistence Latency
- **Observation**: High-frequency telemetry updates during large task matrices can contend on SQLite write transactions.
- **Improvement**: Batch telemetry events in memory and flush periodically or on task state transitions (`MaxLastActions = 200` ring buffer).

---

## 4. Prioritized Improvements Roadmap

| Priority | Area | Proposal | Expected Impact |
| :---: | :--- | :--- | :--- |
| **P0** | **Prompting & PM DoD** | Mandate Walking Skeleton in User Story 1 across all greenfield projects | Eliminates early build stalls and guarantees early runnable binary |
| **P1** | **Deterministic Formatting** | Auto-apply formatters (`rustfmt`, `ruff`, `rubocop -A`) prior to linter agent evaluation | Reduces linter retry token spend by 40-60% |
| **P1** | **Fallback Telemetry** | Enrich Fallback Agent prompts with precise diff context and build logs | Increases 1-shot sovereign repair success rate |
| **P2** | **Worktree Caching** | Shared global caches for Cargo, Go, Python, and npm across isolated worktrees | Accelerates build cycles by 3-5x |
| **P2** | **OCC OCC Concurrency** | Optimize optimistic concurrency retries during mailbox orchestration | Prevents state update contention during multi-agent merges |

"""

    with open(path, "w", encoding="utf-8") as f:
        f.write(doc)
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Wrote {path}", flush=True)

def main():
    override_timeout = None
    for arg in sys.argv[1:]:
        if arg in ("--help", "-h"):
            print("Usage: python3 validation/runner_9projects.py [--timeout=seconds]")
            print("\nDynamic scale timeouts by default:")
            for p, t in PROJECT_SCALE_TIMEOUTS.items():
                print(f"  - {p:15s}: {t}s ({t//60}m)")
            sys.exit(0)
        elif arg.startswith("--timeout="):
            try:
                override_timeout = int(arg.split("=", 1)[1].strip())
            except ValueError:
                pass

    print(f"==================================================", flush=True)
    print(f"NOCTIFAB 9-PROJECT VALIDATION SUITE RUNNER", flush=True)
    print(f"Projects: {', '.join(PROJECTS)}", flush=True)
    if override_timeout:
        print(f"Timeout mode: Fixed override ({override_timeout}s / {override_timeout/60:.0f}m)", flush=True)
    else:
        print(f"Timeout mode: Dynamic by Project Scale (Small: 20m, Medium: 30m, Large: 35-40m)", flush=True)
    print(f"==================================================", flush=True)
    
    results = []
    for idx, project in enumerate(PROJECTS, 1):
        t_limit = get_project_timeout(project, override_timeout)
        print(f"\n[PROJECT {idx}/{len(PROJECTS)}] Starting {project} (Timeout: {t_limit}s / {t_limit/60:.0f}m)...", flush=True)
        res = run_project(project, timeout_seconds=t_limit)
        results.append(res)
        
    print(f"\n==================================================", flush=True)
    print(f"ALL 9 VALIDATION RUNS COMPLETED. GENERATING FEEDBACK REPORTS...", flush=True)
    print(f"==================================================", flush=True)
    
    write_project_feedback_md(results)
    write_val_project_feedback_md(results)
    
    print("\n" + "="*80)
    print("FINAL VALIDATION SUMMARY TABLE:")
    print("="*80)
    print(f"| {'Project':<15} | {'Spent Time':<15} | {'Status':<15} | {'Explanation':<40} |")
    print(f"|{'-'*17}|{'-'*17}|{'-'*17}|{'-'*42}|")
    for r in results:
        p = r["project"]
        spent = f"{r['duration']:.1f}s ({r['duration']/60:.1f}m)"
        st = "SUCCESS" if (r["exit_code"] == 0 and not r["timed_out"]) else ("TIMEOUT" if r["timed_out"] else f"FAILED ({r['exit_code']})")
        t_lim = r.get("timeout_limit", 1200)
        if r["exit_code"] == 0 and not r["timed_out"]:
            expl = f"Passed all checks ({len(r['generated_files'])} files generated)"
        elif r["timed_out"]:
            expl = f"Terminated at {t_lim/60:.0f}m timeout limit"
        else:
            expl = f"Failed with exit code {r['exit_code']}"
        print(f"| {p:<15} | {spent:<15} | {st:<15} | {expl:<40} |")
    print("="*80)

if __name__ == "__main__":
    main()
