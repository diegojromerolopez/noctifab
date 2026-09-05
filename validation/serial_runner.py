#!/usr/bin/env python3
"""
Serial Validation Runner for Noctifab.
Runs all validation projects sequentially with:
- Strict 10-minute (600s) timeout per project
- Clean container termination on timeout
- Early abort on 3 consecutive failures
- Detailed feedback document generation per project in repo root
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
MAX_TIMEOUT_SECONDS = 600  # 10 minutes mandate
MAX_CONSECUTIVE_FAILURES = 3

PROJECTS_ORDER = [
    "echo",
    "todo-cli",
    "calculator",
    "wc",
    "fortune",
    "t4",
    "pyedis",
    "notebook",
    "djanban",
    "auth-vault",
    "buffonstream",
    "jpacioli",
    "stricc",
    "ocalogue",
    "searchthedocs",
    "frontpunch",
]

def sanitize_str(s: str) -> str:
    return s.strip() if s else ""

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
        "content": content,
    }
    
    status_match = re.search(r"^>\s*Status:\s*(\w+)", content, re.MULTILINE)
    if status_match:
        data["status"] = status_match.group(1)
        
    lead_time_match = re.search(r"\-\s*\*\*Lead Time:\*\*\s*([^\n\r]+)", content)
    if lead_time_match:
        data["lead_time"] = lead_time_match.group(1).strip()
        
    table_match = re.search(r"## (?:Execution Status|Live Status)[\s\S]*?\|(RUNNING|SUCCESS|FAILED|CANCELLED)([\s\S]*?)\n", content)
    if table_match:
        status_val = table_match.group(1).strip()
        data["status"] = status_val
        rest = table_match.group(2).split("|")
        # [0]=timestamp, [1]=active_agents, [2]=stories, [3]=tasks, [4]=validations, [5]=errors, [6]=retries, [7]=tokens
        if len(rest) >= 8:
            data["stories_count"] = rest[2].strip()
            data["tasks_count"] = rest[3].strip()
            data["errors_count"] = rest[5].strip()
            data["retries_count"] = rest[6].strip()
            data["tokens_count"] = rest[7].strip()
            
    return data

def parse_log(log_path: str):
    if not log_path or not os.path.exists(log_path):
        return {}
    
    with open(log_path, "r", encoding="utf-8", errors="replace") as f:
        log_content = f.read()
        
    analysis = {
        "ensemble_pm_triggers": len(re.findall(r"ensemble|synthesiz|synthesis|consensus", log_content, re.IGNORECASE)),
        "ensemble_generator_tiers": len(re.findall(r"tier|adaptive|fast_tier|standard_tier|heavy_tier", log_content, re.IGNORECASE)),
        "ensemble_tester_scored": len(re.findall(r"best_of_n|scored", log_content, re.IGNORECASE)),
        "ensemble_auditor_consensus": len(re.findall(r"consensus|voter|tie_breaker", log_content, re.IGNORECASE)),
        "rate_limits_429": len(re.findall(r"429|Too Many Requests|retryDelay|rate limit", log_content, re.IGNORECASE)),
        "auth_errors_401_403": len(re.findall(r"401 Unauthorized|403 Forbidden", log_content, re.IGNORECASE)),
        "model_not_found_404": len(re.findall(r"404 Not Found|model_not_found", log_content, re.IGNORECASE)),
        "schema_retries": len(re.findall(r"schema retry|envelope retry|parse error|invalid json", log_content, re.IGNORECASE)),
        "linter_retries": len(re.findall(r"linter failure|linter error|consecutive linter", log_content, re.IGNORECASE)),
        "tokens_tracked": re.findall(r"total_tokens[^\n]*", log_content, re.IGNORECASE),
        "models_mentioned": sorted(list(set(re.findall(r"(?:claude|gemini|openai|deepseek|qwen|glm|openrouter|opencode)[a-zA-Z0-9\.\-_]*", log_content, re.IGNORECASE)))),
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

def generate_feedback_doc(project: str, duration_sec: float, exit_code: int, timed_out: bool, report_data: dict, log_analysis: dict, generated_files: list):
    feedback_filename = f"{project.upper().replace('-', '_')}_FEEDBACK.md"
    feedback_path = os.path.join(ROOT_DIR, feedback_filename)
    
    status_label = "TIMEOUT (Terminated at 10m)" if timed_out else ("PASS" if exit_code == 0 else f"FAIL (Exit code {exit_code})")
    exec_status = report_data.get("status", "UNKNOWN")
    lead_time = report_data.get("lead_time", f"{duration_sec:.1f}s")
    stories = report_data.get("stories_count", "-")
    tasks = report_data.get("tasks_count", "-")
    errors = report_data.get("errors_count", "-")
    retries = report_data.get("retries_count", "-")
    tokens = report_data.get("tokens_count", "-")
    
    models = ", ".join(f"`{m}`" for m in log_analysis.get("models_mentioned", [])) or "None logged"
    
    doc = f"""# Noctifab Validation Feedback Report: `{project}`

**Target Project**: `validation/projects/{project}`  
**Execution Mode**: Autonomous Containerized E2E (Serial Mode)  
**Date & Time**: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}  
**Wall-Clock Duration**: {duration_sec:.1f} seconds (~{duration_sec/60:.2f} minutes)  
**Exit Status**: **{status_label}** (Report Status: `{exec_status}`)  
**Feedback Document**: [`{feedback_filename}`]({feedback_filename})

---

## 1. Executive Summary & Verdict

| Metric | Recorded Value | Evaluation / Status |
| :--- | :--- | :--- |
| **Validation Verdict** | **{status_label}** | {'✅ Completed requirements & validation assertions' if exit_code == 0 and not timed_out else '❌ Failed validation assertions or timed out'} |
| **Execution Lead Time** | `{lead_time}` | Wall clock: {duration_sec:.1f}s |
| **User Stories Processed** | `{stories}` | Decomposition & story lifecycle |
| **Tasks Executed** | `{tasks}` | Planned & completed tasks |
| **Errors Encountered** | `{errors}` | Runtime error count |
| **Retries Count** | `{retries}` | Self-healing retry loops |
| **Tokens Consumed** | `{tokens}` | Token accountability measurement |

---

## 2. Ensemble Models Architecture Evaluation

### 2.1 Model & Provider Operations
- **Models Encountered in Execution**: {models}
- **Rate Limit (HTTP 429) Occurrences**: {log_analysis.get('rate_limits_429', 0)}
- **Auth (HTTP 401/403) Failovers**: {log_analysis.get('auth_errors_401_403', 0)}
- **Model Resolution (404) Failovers**: {log_analysis.get('model_not_found_404', 0)}
- **Schema & JSON Envelope Retries**: {log_analysis.get('schema_retries', 0)}

### 2.2 Ensemble Strategy Behavior
- **Product Manager Ensemble (Parallel / Synthesis)**:
  - Trigger occurrences: {log_analysis.get('ensemble_pm_triggers', 0)}
  - Status: {'Active and engaged during story creation' if log_analysis.get('ensemble_pm_triggers', 0) > 0 else 'Single-model fallback or bypass'}
- **Generator Tiered Adaptive Selection**:
  - Tier markers in log: {log_analysis.get('ensemble_generator_tiers', 0)}
- **Tester Agent Scored Selection**:
  - Scored best-of-N mentions: {log_analysis.get('ensemble_tester_scored', 0)}
- **Auditor Consensus & Voters**:
  - Consensus evaluations: {log_analysis.get('ensemble_auditor_consensus', 0)}

---

## 3. Bottlenecks & Execution Analysis

- **Linter Self-Healing Churn**: {log_analysis.get('linter_retries', 0)} events detected.
- **Max Duration Limit**: {'Reached 10-minute mandate limit' if timed_out else 'Completed within allocated 10-minute window'}.
- **Log Activity**: {log_analysis.get('log_length', 0)} characters captured.

---

## 4. Code Generation & Artifact Verification

### 4.1 Generated Source Files
Found **{len(generated_files)}** files generated in `output/`:
"""
    if generated_files:
        for f in generated_files[:30]:
            doc += f"- `{f}`\n"
        if len(generated_files) > 30:
            doc += f"- *(and {len(generated_files) - 30} more files...)*\n"
    else:
        doc += "- *(No generated source files detected)*\n"

    doc += f"""
---

## 5. Recent Container Log Snippet (Tail)

```text
{log_analysis.get('raw_sample', '').strip()}
```

---

## 6. Recommendations & Action Items

1. **Ensemble Architecture**: {'Maintain current multi-model routing' if exit_code == 0 else 'Investigate model failovers and prompt schema adherence'}.
2. **Linter & Test Verification**: {'Verification passed cleanly' if exit_code == 0 else 'Inspect compiler/linter error feedback loop'}.
3. **Execution Speed**: {'Good throughput' if duration_sec < 300 else 'Consider optimizing prompt token sizes or tier selections to reduce wall-clock latency'}.
"""

    with open(feedback_path, "w", encoding="utf-8") as f:
        f.write(doc)
    print(f"[{datetime.now().strftime('%H:%M:%S')}] Wrote feedback report to {feedback_filename}")
    return feedback_path

def run_project(project: str):
    print(f"\n==================================================")
    print(f"[{datetime.now().strftime('%H:%M:%S')}] STARTING PROJECT: {project}")
    print(f"==================================================")
    
    start_time = time.time()
    cmd = [os.path.join(ROOT_DIR, "validation", "run_one.sh"), project]
    
    # We run run_one.sh with SKIP_BUILD=1 (base is pre-built)
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
        while True:
            # Check if process finished
            ret = process.poll()
            if ret is not None:
                break
            
            # Check timeout
            elapsed = time.time() - start_time
            if elapsed > MAX_TIMEOUT_SECONDS:
                print(f"[{datetime.now().strftime('%H:%M:%S')}] ❌ TIMEOUT reached for {project} (>{MAX_TIMEOUT_SECONDS}s). Terminating container...")
                timed_out = True
                # Clean up container
                subprocess.run(["docker", "ps", "-q", "--filter", f"name=validate-{project}"], capture_output=True, text=True)
                subprocess.run(f"docker ps -q --filter name=validate-{project} | xargs -r docker rm -f", shell=True, capture_output=True)
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                break
                
            line = process.stdout.readline() if process.stdout else ""
            if line:
                stdout_lines.append(line)
                # Print key progress lines
                if any(k in line for k in ["launching", "Validating", "building", "PASS", "FAIL", "Success", "Error", "exited"]):
                    print(f"  [{project}] {line.strip()}")
            else:
                time.sleep(0.5)
                
    except Exception as e:
        print(f"Exception while running {project}: {e}")
        timed_out = True
        process.kill()
        
    duration = time.time() - start_time
    exit_code = process.returncode if process.returncode is not None else 1
    if timed_out:
        exit_code = 124
        
    status_str = "TIMEOUT" if timed_out else ("PASS" if exit_code == 0 else f"FAIL ({exit_code})")
    print(f"[{datetime.now().strftime('%H:%M:%S')}] FINISHED PROJECT: {project} -> {status_str} in {duration:.1f}s")
    
    # Read report
    report_dir = os.path.join(PROJECTS_DIR, project, "output", "report")
    latest_report = None
    if os.path.exists(report_dir):
        reports = glob.glob(os.path.join(report_dir, "*.md"))
        if reports:
            latest_report = max(reports, key=os.path.getmtime)
            
    report_data = parse_report(latest_report) if latest_report else {}
    
    # Read log
    log_file = os.path.join(PROJECTS_DIR, project, "output", "log", f"{project}.log")
    log_analysis = parse_log(log_file)
    
    # Inspect generated files
    generated_files = inspect_generated_code(os.path.join(PROJECTS_DIR, project))
    
    # Generate Feedback document in root
    generate_feedback_doc(
        project=project,
        duration_sec=duration,
        exit_code=exit_code,
        timed_out=timed_out,
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
    print(f"==================================================")
    print(f"Starting Noctifab Serial Validation Matrix")
    print(f"Total projects: {len(PROJECTS_ORDER)}")
    print(f"Max time per project: {MAX_TIMEOUT_SECONDS}s (10 min)")
    print(f"Early stop condition: {MAX_CONSECUTIVE_FAILURES} consecutive failures")
    print(f"==================================================")
    
    results = []
    consecutive_failures = 0
    stopped_early = False
    
    for idx, project in enumerate(PROJECTS_ORDER, 1):
        print(f"\n>>> Running project {idx}/{len(PROJECTS_ORDER)}: {project}")
        res = run_project(project)
        results.append(res)
        
        if res["exit_code"] != 0:
            consecutive_failures += 1
            print(f"⚠️ Project {project} failed. Consecutive failures: {consecutive_failures}/{MAX_CONSECUTIVE_FAILURES}")
        else:
            consecutive_failures = 0
            print(f"✅ Project {project} succeeded! Resetting consecutive failure counter.")
            
        if consecutive_failures >= MAX_CONSECUTIVE_FAILURES:
            print(f"\n🚨 EARLY STOP TRIGGERED: {consecutive_failures} consecutive validation projects failed!")
            print(f"Stopping matrix execution to analyze causes.")
            stopped_early = True
            break
            
    # Print final summary
    print(f"\n==================================================")
    print(f"VALIDATION MATRIX EXECUTION SUMMARY")
    print(f"==================================================")
    print(f"| Project | Status | Duration | Stories | Tasks | Errors | Tokens |")
    print(f"| :--- | :--- | :--- | :--- | :--- | :--- | :--- |")
    for r in results:
        print(f"| {r['project']} | {r['status']} | {r['duration']:.1f}s | {r['stories']} | {r['tasks']} | {r['errors']} | {r['tokens']} |")
    print(f"==================================================")
    
    if stopped_early:
        sys.exit(2)
    
    all_passed = all(r["exit_code"] == 0 for r in results)
    sys.exit(0 if all_passed else 1)

if __name__ == "__main__":
    main()
