#!/usr/bin/env python3
"""Generate a structured Markdown validation-feedback file from a noctifab
container run log.

Usage:
    gen_feedback.py <log_path> <project> <target_filespec> <repo_root> [exit_code]

`<target_filespec>` is a `;`-separated list of `path` tokens that the
validate.sh script checks at the end of a run (e.g. "Cargo.toml;src/main.rs"
or "frontpunch/worker.py"). A missing target forces the verdict to FAIL
unless the container exit code is explicitly 0 AND the matching banner
appears.

The generated file mirrors the format used by the existing
`*_FEEDBACK.md` artifacts (e.g. WC_FEEDBACK.md, TODO_CLI_FEEDBACK.md) so the
output stays diff-stable across runs and reviewers.
"""

import datetime
import os
import re
import sys
from pathlib import Path

PHASE_MARKERS = (
    "[Reader] phase ok",
    "[Generator] write phase ok:",
    "[Generator] write phase summary:",
    "[Tester] write phase ok:",
    "[Tester] write phase summary:",
)

# Lines that count as "phase activity" when they start with one of these
# (after stripping leading whitespace). The first matching prefix wins, so
# order matters: longer prefixes first.
PHASE_PREFIXES = tuple(
    sorted(PHASE_MARKERS, key=len, reverse=True)
)

# Lines considered "raw error / parser / runtime" signals. We anchor on the
# verbs the orchestrator prints when something fails so that legitimate
# `errorLog` references inside code the LLM wrote do not pollute the report.
ERROR_ANCHORS = (
    "\u26a0 LLM",                       # ⚠ LLM ... parser reminder
    "Error: ",
    "panic: ",
    "thread 'main' panicked",
    "Traceback (most recent call last)",
    "SyntaxError",
    "FAILED",
    "FAIL:",
    "PASS",
    "fatal:",
    "clarification_timeout_action:",
    "git_mutex_timeout:",
    "timeout_seconds:",
    "execution failed",
    "failed test validation",
    "context deadline exceeded",
)

# Policy / sandbox violations.
POLICY_ANCHORS = (
    "forbidden_patterns:",
    "sandbox violation",
    "Sandbox policy",
    "tool policy",
    "permission denied",
    "blocked by policy",
)

# Verdict banners emitted by validate.sh.
SUCCESS_BANNER = "Success: Noctifab executed autonomously"
SUCCESS_MARKER = "\u2705 Success:"
FAILURE_MARKER = "\u274c Error:"
ERROR_MARKER = "Error: execution failed"


def load_lines(path):
    with open(path, "r", encoding="utf-8", errors="replace") as fh:
        return fh.read().splitlines()


def clean_log_lines(lines):
    # Step 1. Filter out tool call content maps
    filtered_lines = []
    in_tool_args = False
    for line in lines:
        s = line.strip()
        if not in_tool_args:
            if "action: tool=" in line and "args=map[" in line:
                if s.endswith("]"):
                    filtered_lines.append(line)
                else:
                    idx = line.find("args=map[")
                    if idx != -1:
                        filtered_lines.append(line[:idx] + "args=map[...]")
                    else:
                        filtered_lines.append(line)
                    in_tool_args = True
            else:
                filtered_lines.append(line)
        else:
            if s.endswith("]"):
                in_tool_args = False
                
    # Step 2. Filter out DEBUG extracted block contents
    final_lines = []
    in_debug_block = False
    for line in filtered_lines:
        if not in_debug_block:
            if "DEBUG extracted block" in line:
                in_debug_block = True
                final_lines.append(line)
            else:
                final_lines.append(line)
        else:
            if "---END---" in line:
                in_debug_block = False
                final_lines.append(line)
                
    return final_lines


def last_banner(lines):
    for line in reversed(lines):
        if SUCCESS_MARKER in line or FAILURE_MARKER in line or ERROR_MARKER in line:
            return line.strip()
    return ""


def detect_artifact(lines, targets):
    """Return (created_paths, missing_paths, banner)."""
    banner = last_banner(lines)
    created = [t for t in targets if f"{t} created" in banner or f"{t} was created" in banner]
    # validate.sh prints "❌ Error: <file> was not created/modified!" on missing
    missing = [t for t in targets if f"{t} was not created" in banner or f"{t} was not created/modified" in banner]
    # Fallback: if banner says success, assume all targets created.
    if SUCCESS_MARKER in banner or banner.startswith(SUCCESS_BANNER) or "Success:" in banner:
        created = list(targets)
        missing = []
    return created, missing, banner


def phase_activity(lines):
    out = []
    seen = set()
    for line in lines:
        stripped = line.strip()
        for prefix in PHASE_PREFIXES:
            if stripped.startswith(prefix):
                # Compress: keep only the prefix label (drop trailing per-task id noise)
                key = prefix
                if key not in seen:
                    seen.add(key)
                    out.append(f"- `{key}`")
                break
    return out


def status_polls(lines):
    polls = []
    # Non-TTY polls look like `2026-07-05T... [status] ...`
    poll_re = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}")
    for line in lines:
        if poll_re.match(line) and ("Build" in line or "running" in line or "Tasks:" in line):
            polls.append(line.strip())
    return polls


def build_failures(lines):
    out = []
    for line in lines:
        s = line.strip()
        if "cargo build" in s and ("error[" in s.lower() or "error:" in s.lower()):
            out.append(s)
        elif s.startswith("error[") or s.startswith("error[E"):
            out.append(s)
        elif s.startswith("cannot find package") or "build failed" in s.lower():
            out.append(s)
    return out


def test_failures(lines):
    out = []
    for line in lines:
        s = line.strip()
        if s.startswith("Orchestrator:") or s.startswith("DEBUG "):
            continue
        low = s.lower()
        if (
            s.startswith("---- ")
            or low.startswith(" failures::")
            or low.startswith("failures:")
            or "panicked at" in low
            or "assertion" in low
            or low.startswith("assert")
            or "traceback (most recent call last)" in low
            or low.startswith("assertionerror")
            or low.startswith("self.assert")
            or low.startswith("t.errorf")
            or low.startswith("t.fatal")
            or low.startswith("syntaxerror:")
        ):
            out.append(s)
    return out


def policy_violations(lines):
    out = []
    for line in lines:
        s = line.strip()
        for anchor in POLICY_ANCHORS:
            if anchor.lower() in s.lower():
                out.append(s)
                break
    return out


def errors_and_parser(lines):
    out = []
    for line in lines:
        s = line.strip()
        for anchor in ERROR_ANCHORS:
            if s.startswith(anchor) or anchor in s:
                out.append(s)
                break
    # Keep order, drop duplicates while preserving first occurrence.
    seen = set()
    deduped = []
    for s in out:
        if s not in seen:
            seen.add(s)
            deduped.append(s)
    return deduped


def spec_ambiguity(lines):
    out = []
    pat = re.compile(
        r"reasoning=\"([^\"]*(?:ambig|under-spec|under spec|contradict|spec says|"
        r"unspecified|not clear|unclear|cannot determine)[^\"]*)\"",
        re.IGNORECASE,
    )
    for line in lines:
        m = pat.search(line)
        if m:
            out.append(m.group(1))
    # Also surface task-title lines that mention these markers.
    for line in lines:
        if "orchestrator:" not in line.lower():
            continue
        for kw in ("ambiguous", "contradict", "under-spec", "under spec", "unclear"):
            if kw in line.lower():
                # Strip redundant prefix noise.
                cleaned = line.strip()
                if cleaned not in out:
                    out.append(cleaned)
                break
    return out


def display_name(project):
    return project.upper().replace("-", "-")


def feedback_filename(project):
    return f"{project.upper().replace('-', '_')}_FEEDBACK.md"


def format_block(items, none_label="None."):
    if not items:
        return f"- {none_label}"
    return "\n".join(f"- {item}" for item in items)


def render(project, targets, exit_code, lines):
    created, missing, banner = detect_artifact(lines, targets)
    if exit_code == 0 and not missing:
        verdict = "PASS"
    else:
        verdict = "FAIL"

    artifact_desc = ", ".join(created) if created else "not created"
    banner_repr = banner if banner else "(no banner captured)"

    now = datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%dT%H:%M:%S.%f")

    phases = phase_activity(lines)
    polls = status_polls(lines)
    build = build_failures(lines)
    tests = test_failures(lines)
    policy = policy_violations(lines)
    errs = errors_and_parser(lines)
    ambig = spec_ambiguity(lines)
    tail = "\n".join(lines[-80:])

    parts = []
    parts.append(f"# {display_name(project)} Validation Feedback")
    parts.append(
        f"_Generated {now}Z by `noctifab-validation:feedback-v1` container run._"
    )
    parts.append(
        "Reviewer perspective: senior software engineer using noctifab to "
        "build this project end-to-end from SPEC.md + roadmap. Only noctifab "
        "(inside the validation container) was allowed to write to the "
        "validation project tree."
    )
    parts.append("")
    parts.append("## 1. Outcome")
    parts.append(f"- **Final verdict:** `{verdict}`")
    parts.append(f"- **Artifact check:** {artifact_desc}")
    parts.append(f"- **Banner:** `{banner_repr}`")
    parts.append("")
    parts.append("## 2. Phase Activity (structured log markers)")
    parts.append(format_block(phases, none_label="- None."))
    parts.append("")
    parts.append("## 3. Status Polls Seen")
    parts.append(format_block(polls, none_label="- None (non-TTY timestamped status lines not present)."))
    parts.append("")
    parts.append("## 4. Build / Compile Failures")
    parts.append(format_block(build, none_label="- None."))
    parts.append("")
    parts.append("## 5. Test Failures")
    parts.append(format_block(tests, none_label="- None."))
    parts.append("")
    parts.append("## 6. Tool / Policy Violations (blocked actions)")
    parts.append(format_block(policy, none_label="- None."))
    parts.append("")
    parts.append("## 7. Errors & Parser / Runtime Issues")
    parts.append(format_block(errs, none_label="- None."))
    parts.append("")
    parts.append("## 8. Spec / Story Ambiguity Surfaced by the LLM")
    parts.append(
        "These are messages where the agent itself flagged the SPEC or a user "
        "story as ambiguous, contradictory, or under-specified \u2014 "
        "actionable signals for the next SPEC/story hardening pass."
    )
    parts.append(format_block(ambig, none_label="- None surfaced in this run."))
    parts.append("")
    parts.append("## 9. Raw run tail (last 80 lines)")
    parts.append("```")
    parts.append(tail)
    parts.append("```")
    parts.append("")
    return "\n".join(parts)


def main(argv):
    if len(argv) < 4:
        sys.stderr.write(
            "usage: gen_feedback.py <log_path> <project> <target_filespec> "
            "<repo_root> [exit_code]\n"
        )
        return 2
    log_path = argv[0]
    project = argv[1]
    targets = [t for t in argv[2].split(";") if t]
    repo_root = Path(argv[3])
    exit_code = int(argv[4]) if len(argv) >= 5 else -1

    lines = load_lines(log_path)
    cleaned = clean_log_lines(lines)
    body = render(project, targets, exit_code, cleaned)

    out_dir = repo_root / "validation" / "projects" / project / "output" / "feedback"
    out_dir.mkdir(parents=True, exist_ok=True)
    out_path = out_dir / feedback_filename(project)
    out_path.write_text(body, encoding="utf-8")
    verdict = "PASS" if (exit_code == 0 and not detect_artifact(cleaned, targets)[1]) else "FAIL"
    rel = out_path.relative_to(repo_root) if repo_root in out_path.parents else out_path
    print(f"wrote {rel} ({verdict})")
    print(f"[{datetime.datetime.now().strftime('%H:%M:%S')}] wrote {rel}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))