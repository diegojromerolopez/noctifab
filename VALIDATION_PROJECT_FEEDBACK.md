# Comprehensive Validation Projects Feedback & Performance Bottlenecks Report

**Date**: 2026-08-01  
**Scope**: Parallel Autonomous Validation Run across 6 Projects (`calculator`, `echo`, `fortune`, `frontpunch`, `todo-cli`, `wc`)  
**Target Duration**: 30 Minutes  

---

## 1. Executive Summary

This report documents performance feedback, development bottlenecks, and speed optimization proposals from a 30-minute parallel execution run of Noctifab validating 6 diverse projects end-to-end inside isolated Docker containers.

Overall, Noctifab successfully demonstrated autonomous roadmap generation, multi-provider model alias resolution (`model: latest`), self-healing linter/test validation loops, and multi-model failover. However, several critical architectural bottlenecks were identified that impacted completion speed.

---

## 2. Validation Run Status & Results

| Project | Target Language / Stack | Final Status | Primary LLM Resolved | Key Observations & Performance Bottlenecks |
|---|---|---|---|---|
| **`echo`** | Go CLI | **`PASS` (100% Complete)** | `gemini-3.1-pro-preview` | **Fastest execution**. Generated clean code, passed 3x consensus testing, compiled binary in dist. |
| **`calculator`** | Ruby (RSpec & RuboCop) | **95% (Stuck in Linter Loop)** | `meta-llama/Llama-3.3-70B-Instruct` | **Linter Stall**. Passed unit tests but got stuck in a 10+ minute loop attempting to fix RuboCop file naming offenses. |
| **`fortune`** | Go / Shell CLI | **90% (Executing US-001)** | `gemini-3.1-pro-preview` | **Model Fallback Overhead**. Hit 403 RegionError on OpenCode DeepSeek models, gracefully failed over to Gemini. |
| **`frontpunch`** | Go / HTML Web App | **85% (Executing US-000)** | `glm-5.2` | **Roadmap Over-Decomposition**. Generated 27 user stories (`US-000` to `US-027`), creating massive planning overhead. |
| **`todo-cli`** | Go CLI | **85% (Executing US-001)** | `glm-5.2` | **API Quota Saturated**. Hit HTTP 429 concurrency rate limits when running in parallel. |
| **`wc`** | Go CLI | **85% (Executing US-001)** | `glm-5.2` | **Schema Format Retries**. Required format reminders for non-JSON envelope outputs. |

---

## 3. Major Development Bottlenecks Identified

### 🚨 Bottleneck 1: Linter Self-Healing Loop Stalls (e.g. RuboCop in `calculator`)
* **Symptom**: In the Ruby `calculator` project, the code compiled and all unit tests passed, but RuboCop reported file naming offenses (`spec/Gemfile_spec.rb should use snake_case`, `baseline_acceptance_test.rb should end with _spec.rb`).
* **Root Cause**: The Generator agent repeatedly attempted to modify code inside the file to satisfy RuboCop, rather than performing file rename operations or recognizing file-convention offenses. This caused an infinite loop where the agent consumed time and tokens retrying the exact same edit.

### 🚨 Bottleneck 2: Single API Key Saturation & Concurrency 429 Rate Limits
* **Symptom**: When running all 6 validation projects in parallel (`make validate-all`), multiple containers hit HTTP 429 (`You exceeded your spend-based rate limit... retryDelay: 45s`).
* **Root Cause**: All 6 Docker containers shared the exact same LLM API keys (`GEMINI_API_KEY`, `OPENCODE_API_KEY`). Simultaneous requests from 6 containers saturated the provider's per-minute request/token quotas, forcing 40s–50s backoff delays.

### 🚨 Bottleneck 3: Non-Text / Experimental Model Discovered from `/models`
* **Symptom**: The `"latest"` model parser dynamically resolved `model: latest` for Gemini to `gemini-3.1-flash-image-preview` and `gemini-3-pro-preview` (which returned 404).
* **Root Cause**: `/models` endpoints include image/vision generation endpoints or unreleased experimental preview endpoints. Passing these to code generation agents caused HTTP 400/404 errors or non-JSON output.

### 🚨 Bottleneck 4: Roadmap Over-Decomposition
* **Symptom**: In `frontpunch`, the Product Manager agent decomposed `SPEC.md` into 27 separate user stories (`US-000.md` through `US-027.md`).
* **Root Cause**: The Product Manager agent lacked a hard cap on roadmap story count. Decomposing a simple web app into 27 micro-stories caused massive DAG planning and state persistence overhead.

### 🚨 Bottleneck 5: Non-JSON Response Envelope Retries
* **Symptom**: LLM completion responses sometimes returned markdown prose (`Here is the JSON...`) or unescaped newlines in JSON strings.
* **Root Cause**: Noctifab had to send one-shot format reminders and wait for retries when the raw LLM response failed envelope parsing.

---

## 4. Actionable Proposals to Accelerate Development Speed

### 🚀 Proposal 1: Auto-Fix Pre-Step & Linter Iteration Cap (Resolves Linter Stalls)
To prevent agents from getting stuck in linter loops like `calculator`:
1. **Auto-Fixer Executed First**: Automatically run linter auto-fix tools (e.g. `rubocop -a`, `go fmt`, `prettier --write`) **before** invoking diagnostic linter checks.
2. **Cap Linter Retries**: Limit linter fix iterations per task to **maximum 3 attempts**. If linter offenses persist after 3 retries, automatically log the offense as non-blocking warning or surface explicit file-rename instructions to the agent.

### 🚀 Proposal 2: API Key Rotation & Concurrency Queue (Resolves Rate Limits)
To avoid HTTP 429 rate limit stalls when running validation projects in parallel:
1. **Multi-Key Support**: Support comma-separated API keys in `secrets.yaml` (`OPENCODE_API_KEYS: "key1,key2,key3"`) with round-robin rotation.
2. **Local Token Bucket / Queue**: Implement a central concurrency semaphore in `make validate-all` to space out requests across containers.

### 🚀 Proposal 3: Product Manager User Story Hard Cap (Resolves Over-Decomposition)
To prevent generating excessive micro-stories (such as the 27 user stories in `frontpunch`):
1. **Cap Story Count**: Enforce `max_user_stories: 5` in `AgentsConfig` for Product Manager roadmap generation.
2. **Post-Generation Validation Check**: If the Product Manager agent generates more stories than `max_user_stories`, trigger an automated consolidation pass to merge micro-stories into broader acceptance criteria.

---

*Report saved to [`VALIDATION_PROJECT_FEEDBACK.md`](file:///Users/diegoj/repos/noctifab/VALIDATION_PROJECT_FEEDBACK.md).*
