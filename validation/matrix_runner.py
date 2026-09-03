#!/usr/bin/env python3
"""
Comprehensive Validation Matrix Runner for Noctifab.
Executes the validation projects with a configurable timeout (default 20 minutes / 1200s),
capturing deep insights into <PROJECT>_FEEDBACK.md files at the repository root:
issues, hurdles, speed, lag causes, failing edge cases, and future improvements.
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
DEFAULT_TIMEOUT_SECONDS = 1200  # 20 minutes limit per project

TARGET_PROJECTS = [
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
        "lines_deleted": "-",
        "task_efficiency": "-",
        "raw_errors": [],
        "user_stories": [],
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
        
    # Extract failing test cases and stack traces
    failing_tests = []
    for m in re.finditer(r"(?:FAIL|FAILED|FAILURE|Error)[\s:]+([^\n\r]+)", log_content):
        line = m.group(0).strip()
        if len(line) < 200 and not any(skip in line for skip in ["0 failed", "FAIL (exit 0)", "PASS"]):
            if line not in failing_tests:
                failing_tests.append(line)
                
    # Extract compiler / syntax errors
    compiler_snippets = []
    for m in re.finditer(r"(?:error\[E\d+\]|SyntaxError|TypeError|gcc: error|clang: error|NameError|ImportError|AttributeError|Compilation error|build failed)[^\n\r]*\n(?:[^\n\r]*\n){1,3}", log_content):
        snip = m.group(0).strip()
        if snip not in compiler_snippets:
            compiler_snippets.append(snip)

    analysis = {
        "ensemble_pm_triggers": len(re.findall(r"ensemble|synthesiz|synthesis|consensus", log_content, re.IGNORECASE)),
        "ensemble_generator_tiers": len(re.findall(r"tier|adaptive|fast_tier|standard_tier|heavy_tier", log_content, re.IGNORECASE)),
        "ensemble_tester_scored": len(re.findall(r"best_of_n|scored", log_content, re.IGNORECASE)),
        "ensemble_auditor_consensus": len(re.findall(r"consensus|voter|tie_breaker", log_content, re.IGNORECASE)),
        "rate_limits_429": len(re.findall(r"429|Too Many Requests|retryDelay|rate limit", log_content, re.IGNORECASE)),
        "auth_errors_401_403": len(re.findall(r"401 Unauthorized|403 Forbidden", log_content, re.IGNORECASE)),
        "model_not_found_404": len(re.findall(r"404 Not Found|model_not_found", log_content, re.IGNORECASE)),
        "schema_retries": len(re.findall(r"schema retry|envelope retry|parse error|invalid json", log_content, re.IGNORECASE)),
        "linter_retries": len(re.findall(r"linter failure|linter error|consecutive linter|Linter found", log_content, re.IGNORECASE)),
        "compiler_errors": len(re.findall(r"error\[E\d+\]|compilation failed|SyntaxError|TypeError|gcc: error|build failed", log_content, re.IGNORECASE)),
        "unblocker_triggers": len(re.findall(r"\[UnblockerAgent\] Detected", log_content, re.IGNORECASE)),
        "action_limit_hits": len(re.findall(r"action ceiling|action limit|max actions exceeded", log_content, re.IGNORECASE)),
        "models_mentioned": sorted(list(set(re.findall(r"(?:claude|gemini|openai|deepseek|qwen|glm|openrouter|opencode)[a-zA-Z0-9\.\-_]*", log_content, re.IGNORECASE)))),
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

def generate_feedback_doc(project: str, duration_sec: float, exit_code: int, timed_out: bool, timeout_limit: int, report_data: dict, log_analysis: dict, generated_files: list):
    feedback_filename = f"{project.upper().replace('-', '_')}_FEEDBACK.md"
    feedback_path = os.path.join(ROOT_DIR, feedback_filename)
    
    timeout_min = timeout_limit / 60
    status_label = f"TIMEOUT (Terminated at {timeout_min:.0f}m limit)" if timed_out else ("SUCCESS (Completed validation)" if exit_code == 0 else f"FAILED (Exit code {exit_code})")
    exec_status = report_data.get("status", "UNKNOWN")
    lead_time = report_data.get("lead_time", f"{duration_sec:.1f}s")
    stories = report_data.get("stories_count", "-")
    tasks = report_data.get("tasks_count", "-")
    errors = report_data.get("errors_count", "-")
    retries = report_data.get("retries_count", "-")
    tokens = report_data.get("tokens_count", "-")
    files_changed = report_data.get("files_changed", len(generated_files))
    lines_added = report_data.get("lines_added", "-")
    task_eff = report_data.get("task_efficiency", "-")
    
    models = ", ".join(f"`{m}`" for m in log_analysis.get("models_mentioned", [])) or "None logged"
    is_refactoring = (project == "djanban")
    
    # Speed & Lag causes evaluation
    lag_factors = []
    if timed_out:
        lag_factors.append(f"Execution reached full {timeout_min:.0f}-minute envelope without completing all lifecycle stories/tasks.")
    if log_analysis.get("rate_limits_429", 0) > 0:
        lag_factors.append(f"HTTP 429 Rate Limiting encountered {log_analysis['rate_limits_429']} times, causing backoff delays.")
    if log_analysis.get("linter_retries", 0) > 0:
        lag_factors.append(f"Linter iteration churn ({log_analysis['linter_retries']} events) added intermediate roundtrips.")
    if log_analysis.get("compiler_errors", 0) > 0:
        lag_factors.append(f"Compiler/Syntax errors ({log_analysis['compiler_errors']} events) required self-healing repair cycles.")
    if log_analysis.get("schema_retries", 0) > 0:
        lag_factors.append(f"JSON schema/envelope parse errors ({log_analysis['schema_retries']} retries) caused model re-prompts.")
    if log_analysis.get("unblocker_triggers", 0) > 0:
        lag_factors.append(f"Unblocker watchdog intervened {log_analysis['unblocker_triggers']} times to break loops/stalls.")
    if not lag_factors:
        lag_factors.append("No significant runtime lag or backoff contention observed; execution proceeded smoothly.")

    doc = f"""# Noctifab Validation Feedback: `{project}`

**Target Project**: `validation/projects/{project}`  
**Project Category**: {'Legacy Codebase Refactoring & Modernization' if is_refactoring else 'Greenfield / Specification-Driven Autonomous Implementation'}  
**Execution Timestamp**: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}  
**Wall-Clock Duration**: {duration_sec:.1f}s (~{duration_sec/60:.2f} minutes)  
**Configured Timeout Limit**: {timeout_limit}s ({timeout_min:.0f} minutes)  
**Harness Verdict**: **{status_label}**  
**Internal Report Status**: `{exec_status}`  

---

## 1. Executive Summary & Verification Metrics

| Metric | Measured Value | Evaluation & Details |
| :--- | :--- | :--- |
| **Execution Verdict** | **{status_label}** | {'Passed all acceptance gates and black-box verification' if exit_code == 0 and not timed_out else (f'Execution stopped after {timeout_min:.0f}-minute timeout limit' if timed_out else 'Terminated on error or test failure')} |
| **Total Lead Time** | `{lead_time}` | Physical wall-clock duration: {duration_sec:.1f}s |
| **User Stories** | `{stories}` | Decomposition and roadmap execution status |
| **Tasks Completed** | `{tasks}` | Total tasks planned and processed |
| **Task Verification Efficiency** | `{task_eff}` | Ratio of passing attempts across worktrees |
| **Files Created / Modified** | `{files_changed}` | Net files in workspace |
| **Lines Added** | `{lines_added}` | Net code delta |
| **Total Tokens Consumed** | `{tokens}` | Token accountability telemetry |
| **Runtime Errors Recorded** | `{errors}` | Diagnostics and self-healing triggers |

---

## 2. Speed & Lag Analysis

### 2.1 Wall-Clock Performance & Throughput
- **Total Duration**: `{duration_sec:.1f}s` (~`{duration_sec/60:.2f} min`) against a `{timeout_min:.0f} min` ceiling.
- **Task Throughput**: {f"{float(tasks)/max(duration_sec/60, 0.1):.2f} tasks/min" if tasks != "-" and tasks.isdigit() and int(tasks) > 0 else "N/A"}
- **Active Models / Providers**: {models}

### 2.2 Primary Lag Causes & Bottlenecks
"""
    for lag in lag_factors:
        doc += f"- **{lag.split(' ', 1)[0]}**: {lag}\n"

    doc += f"""
---

## 3. Issues, Hurdles & Edge Cases

### 3.1 Observed Execution Hurdles
- **Linter & Static Analysis Churn**: {log_analysis.get('linter_retries', 0)} linter diagnostic events observed in the log.
- **Compiler / Syntax Hurdles**: {log_analysis.get('compiler_errors', 0)} compiler/syntax error occurrences handled by generator/tester iterations.
- **Schema Adherence & Envelope Retries**: {log_analysis.get('schema_retries', 0)} retries required.
- **Rate Limit (HTTP 429) Contention**: {log_analysis.get('rate_limits_429', 0)} incidents detected.
- **Model Resolution / Auth Failovers**: {log_analysis.get('model_not_found_404', 0) + log_analysis.get('auth_errors_401_403', 0)} incidents detected.
- **Unblocker Agent Interventions**: {log_analysis.get('unblocker_triggers', 0)} stall detections assessed.

### 3.2 Failing Edge Cases & Test Diagnostics
"""
    failing_tests = log_analysis.get("failing_tests", [])
    if failing_tests:
        doc += "The following test failure / error signatures were identified in container output:\n"
        for ft in failing_tests:
            doc += f"- `{ft}`\n"
    else:
        doc += "No test assertion failures or unhandled exceptions logged.\n"

    compiler_snippets = log_analysis.get("compiler_snippets", [])
    if compiler_snippets:
        doc += "\n#### Compiler / Syntax Diagnostics:\n```text\n"
        for snip in compiler_snippets:
            doc += f"{snip}\n---\n"
        doc += "```\n"

    raw_errors = report_data.get("raw_errors", [])
    if raw_errors:
        doc += "\n### 3.3 Error & Self-Correction Log\n| Error ID | Category | Status / Resolution | Summary |\n| :--- | :--- | :--- | :--- |\n"
        for err in raw_errors[:10]:
            doc += f"| `{err['id']}` | {err['category']} | {err['resolution']} | {err['summary']} |\n"
        if len(raw_errors) > 10:
            doc += f"| ... | ... | ... | *({len(raw_errors) - 10} additional error events)* |\n"

    doc += f"""
---

## 4. Code Generation & Artifacts

### 4.1 Generated Source Files (`output/`)
Found **{len(generated_files)}** files generated:
"""
    if generated_files:
        for f in generated_files[:35]:
            doc += f"- `{f}`\n"
        if len(generated_files) > 35:
            doc += f"- *(and {len(generated_files) - 35} more files...)*\n"
    else:
        doc += "- *(No files were generated in output/)*\n"

    tasks_list = report_data.get("tasks_list", [])
    if tasks_list:
        doc += "\n### 4.2 Executed Tasks Breakdown\n| Task Title | Story | Attempts | Status | Elapsed |\n| :--- | :--- | :---: | :---: | ---: |\n"
        for t in tasks_list[:15]:
            doc += f"| {t['title']} | {t['story']} | {t['attempts']} | `{t['status']}` | {t['elapsed']} |\n"
        if len(tasks_list) > 15:
            doc += f"| ... | ... | ... | ... | *({len(tasks_list) - 15} additional tasks)* |\n"

    doc += f"""
---

## 5. Potential Improvements & Actionable Next Steps

1. **Task Slicing Granularity**: {'Task decomposition executed cleanly.' if exit_code == 0 else 'Ensure tasks are vertically sliced (walking skeleton) to produce runnable executables in the first task before deeper domain expansion.'}
2. **Linter & Test Optimization**: {'Toolchain verification operated cleanly.' if log_analysis.get('linter_retries', 0) == 0 else 'Refine linter deferral and caching rules to prevent repetitive diagnostic roundtrips.'}
3. **Token & Latency Efficiency**: Consumed {tokens} total tokens during execution. Optimize prompt compaction and cache reuse to reduce latency and token spend.
4. **Resilience & Self-Correction**: {'Maintain current self-healing workflows.' if not timed_out and exit_code == 0 else 'Enhance early error detection and forced compilation fallbacks to prevent stalling on retries.'}

---

## 6. Container Console Log Excerpt (Tail)

```text
{log_analysis.get('raw_sample', '').strip()}
```
"""

    with open(feedback_path, "w", encoding="utf-8") as f:
        f.write(doc)
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Wrote feedback report to {feedback_filename}")
    return feedback_path

def run_single_project(project: str, timeout_seconds: int = DEFAULT_TIMEOUT_SECONDS):
    print(f"\n==================================================")
    print(f"[{datetime.now().strftime('%H:%M:%S')}] STARTING VALIDATION: {project} (Timeout: {timeout_seconds}s / {timeout_seconds/60:.0f}m)")
    print(f"==================================================")
    
    start_time = time.time()
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
            
            elapsed = time.time() - start_time
            if elapsed > timeout_seconds:
                print(f"[{datetime.now().strftime('%H:%M:%S')}] ❌ TIMEOUT reached for {project} (>{timeout_seconds}s). Terminating container...", flush=True)
                timed_out = True
                subprocess.run(f"docker ps -q --filter name=validate-{project} | xargs -r docker rm -f", shell=True, capture_output=True)
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                break
                
            if process.stdout:
                r, _, _ = select.select([process.stdout], [], [], 1.0)
                if r:
                    line = process.stdout.readline()
                    if line:
                        stdout_lines.append(line)
                        if any(k in line for k in ["launching", "Validating", "building", "PASS", "FAIL", "Success", "Error", "exited", "Orchestrator", "Tool Executed", "Task"]):
                            print(f"  [{project}] {line.strip()[:110]}", flush=True)
            else:
                time.sleep(1.0)
                
    except Exception as e:
        print(f"Exception while running {project}: {e}", flush=True)
        timed_out = True
        process.kill()

    duration = time.time() - start_time
    exit_code = process.returncode if process.returncode is not None else 1
    if timed_out:
        exit_code = 124
        
    status_str = "TIMEOUT" if timed_out else ("PASS" if exit_code == 0 else f"FAIL ({exit_code})")
    print(f"[{datetime.now().strftime('%H:%M:%S')}] FINISHED: {project} -> {status_str} in {duration:.1f}s")
    
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
    
    generate_feedback_doc(
        project=project,
        duration_sec=duration,
        exit_code=exit_code,
        timed_out=timed_out,
        timeout_limit=timeout_seconds,
        report_data=report_data,
        log_analysis=log_analysis,
        generated_files=generated_files,
    )
    
    return {
        "project": project,
        "duration": duration,
        "exit_code": exit_code,
        "timed_out": timed_out,
        "status": status_str,
        "stories": report_data.get("stories_count", "-"),
        "tasks": report_data.get("tasks_count", "-"),
        "errors": report_data.get("errors_count", "-"),
        "tokens": report_data.get("tokens_count", "-"),
    }

def main():
    custom_projects = []
    timeout_val = DEFAULT_TIMEOUT_SECONDS
    for arg in sys.argv[1:]:
        if arg.startswith("--projects="):
            custom_projects.extend([p.strip() for p in arg.split("=", 1)[1].split(",") if p.strip()])
        elif arg.startswith("--timeout="):
            try:
                timeout_val = int(arg.split("=", 1)[1].strip())
            except ValueError:
                pass
        elif not arg.startswith("-"):
            custom_projects.append(arg.strip())

    projects_to_run = custom_projects if custom_projects else TARGET_PROJECTS

    print(f"==================================================")
    print(f"Starting Noctifab Matrix Runner")
    print(f"Target Projects ({len(projects_to_run)}): {', '.join(projects_to_run)}")
    print(f"Timeout per project: {timeout_val}s ({timeout_val/60:.0f} min)")
    print(f"==================================================")
    
    all_results = []
    for idx, project in enumerate(projects_to_run, 1):
        print(f"\n>>> Running {idx}/{len(projects_to_run)}: {project}")
        res = run_single_project(project, timeout_seconds=timeout_val)
        all_results.append(res)
            
    print(f"\n==================================================")
    print(f"ALL VALIDATION PROJECTS COMPLETED")
    print(f"==================================================")
    print(f"| Project | Status | Duration | Stories | Tasks | Errors | Tokens |")
    print(f"| :--- | :--- | :--- | :--- | :--- | :--- | :--- |")
    for r in all_results:
        print(f"| {r['project']} | {r['status']} | {r['duration']:.1f}s | {r['stories']} | {r['tasks']} | {r['errors']} | {r['tokens']} |")
    print(f"==================================================")

if __name__ == "__main__":
    main()
