# Noctifab Evaluation Report: Autonomous Code Creation & Project Generation

This report evaluates the feasibility, benefits, and limitations of using the [noctifab](file:///Users/diegoj/repos/noctifab) Dark Factory platform to autonomously create code and generate projects. It outlines how a software engineer should integrate this tool into their daily development workflow.

---

## 1. Executive Summary

`noctifab` is an autonomous, long-running agentic CLI harness designed to operate at **Level 3 and Level 4 autonomy**. Rather than functioning as an autocomplete extension (Level 1) or a basic chat assistant (Level 2), it operates in a continuous, multi-agent loop that plans tasks, writes code, develops test suites, validates changes, and merges Pull Requests without manual intervention.

While it is **highly effective** for implementing features, fixing bugs, and refactoring code within an *existing* project, using it to generate an *entire project from a blank directory* presents bootstrapping challenges. To use it successfully, a software engineer must establish a minimal scaffolding environment first.

---

## 2. Arguments FOR Using Noctifab for Code Creation

*   **Hands-Off Execution (Level 3/4 Autonomy):** Instead of forcing developers to copy-paste generated code, review every character, or manually run build scripts, the [Orchestrator](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator.go) handles the entire lifecycle—checking out branches, writing tests, implementing features, running test suites, rebasing, and raising Pull Requests.
*   **Quality Gated by Test-Driven Development (TDD):** The orchestrator separates cognitive roles into a [TesterAgent](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_helper.go#L99) and a [GeneratorAgent](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_helper.go#L146). The Tester Agent writes unit and integration tests *before* the Generator Agent begins implementing code. This avoids "evaluation gaming" (where code generators write tests designed to pass buggy implementation).
*   **Concurrently Scheduled Tasks:** Complex specifications are parsed by the Planner Agent into a Directed Acyclic Graph (DAG) using the [Scheduler](file:///Users/diegoj/repos/noctifab/pkg/services/scheduler.go). Independent tasks run concurrently in isolated Git worktrees, speeding up execution.
*   **Self-Healing Debugging Loops:** If the [TestValidator](file:///Users/diegoj/repos/noctifab/pkg/services/test_validator.go) finds syntax, linting, or test failures, the orchestrator automatically increments retries, packages the terminal logs, and feeds them back to the Generator Agent to implement fixes.
*   **Interactive Clarification Mailbox:** Rather than making assumptions or crashing when faced with ambiguity, the daemon logs a clarification request. The [ClarificationPoller](file:///Users/diegoj/repos/noctifab/pkg/services/clarification_poller.go) prompts the engineer in the terminal to answer and unblock execution.
*   **Git-Safe Serialized Merges:** Parallel tasks are merged sequentially into the integration branch using a thread-safe [RebaseQueue](file:///Users/diegoj/repos/noctifab/pkg/services/rebase_queue.go), avoiding concurrent Git write conflicts or index locking errors.

---

## 3. Arguments AGAINST Using Noctifab (or Key Limitations)

*   **High Token & API Credit Cost:** Running a full loop (Reader Phase $\rightarrow$ Minimal Generator $\rightarrow$ Minimal Tester $\rightarrow$ Refactor Generator $\rightarrow$ Refactor Tester $\rightarrow$ Retries) for multiple parallel tasks can consume significant LLM tokens. Daily cost limits must be configured (`--max-budget-usd`).
*   **The Bootstrap "Cold-Start" Dependency:** `noctifab` requires a working test runner and linter to validate and gate code quality. If a project is completely empty, it has no test frameworks (e.g. Jest, PyTest, Go test) or dependencies installed. The validator commands will fail immediately, halting the daemon.
*   **Strict Code Quality Constraints:** Code generated must pass the configured linter. For example, if Go rules are active, the linter checks that files remain under 500 lines. The LLM must be explicitly guided to modularize code, or the build gates will fail.
*   **Sandbox Security Configuration:** In `host` sandbox mode, the agent runs commands directly on the developer's machine. Although path traversal is checked in [resolveSandboxPath](file:///Users/diegoj/repos/noctifab/pkg/services/production_tools.go#L105), running arbitrary tests could present risks. Setting up a `docker` sandbox mode is safer but requires warm Docker containers and increases execution overhead.

---

## 4. Feasibility of Project Generation via the `start` Command

The `noctifab start` command:
1. Performs pre-flight connectivity checks.
2. Spawns the headless background daemon ([serveCmd](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/serve.go#L24)).
3. Launches a foreground interactive REPL loop ([ListenerAgent](file:///Users/diegoj/repos/noctifab/pkg/services/listener.go#L46)) that routes operator directives.

### Generating a New Project from a Completely Empty Folder
*   **Feasibility:** **Low (out-of-the-box)**
*   **Reason:** The orchestrator will fail during the validation phase because there is no workspace configuration, package files, or executable test suite. The sandbox cannot execute a `test_command` on a directory with no build tools.

### Generating a Project via Scaffolding (The Recommended Workaround)
*   **Feasibility:** **High**
*   **Reason:** An engineer can manually bootstrap the folder with basic configurations (e.g., run `npm init` and create a dummy test file) and set up the sandbox parameters in `.noctifab/config.yaml` to allow commands like `npm` or `npx`. Once the tests can run (even if it's just a placeholder test that returns `true`), `noctifab` can autonomously build out the rest of the project files from a specification document.

---

## 5. How a Software Engineer Should Work with Noctifab

To work productively with `noctifab`, an engineer should adopt an **Orchestrator-Reviewer** role rather than a traditional coder role.

### Step 1: Pre-requisites & Workspace Initialization
First, navigate to your target project folder. If it is a brand new project, initialize the repository and create a basic skeleton (e.g. `package.json`, `go.mod`, or `requirements.txt`).

Initialize the `noctifab` configurations:
```bash
noctifab init
```
This creates the gitignored [.noctifab/](file:///Users/diegoj/repos/noctifab/docs/cli_usage.md#L18) directory.

### Step 2: Configure Workspace Policies & Secrets
1. Open [.noctifab/config.yaml](file:///Users/diegoj/repos/noctifab/docs/cli_usage.md#L19) and configure your **sandbox** commands to match the target language (e.g., test command, linter command, and allowed shell commands):
   ```yaml
   sandbox:
     mode: host
     test_command: "go test -v ./..."
     linter_command: "golangci-lint run"
     formatter_command: "go fmt ./..."
     allowed_commands:
       - go
       - git
   ```
2. Create [.noctifab/secrets.yaml](file:///Users/diegoj/repos/noctifab/docs/secrets.md#L32) to securely load API credentials (this file is pre-configured to be gitignored):
   ```yaml
   GEMINI_API_KEY: "your-api-key-here"
   GITHUB_TOKEN: "your-github-pat-here"
   ```
3. Run configuration validation to check setup:
   ```bash
   noctifab validate
   ```

### Step 3: Write the Feature Specification (User Story)
Create a Markdown specification file (e.g., `roadmap/US-001_auth_service.md`). The specification must be highly detailed and include:
*   **Requirements:** Precise endpoints, API structures, database entities, and error codes.
*   **Technical Constraints:** Allowed languages, file size rules, or design pattern preferences.
*   **Verification Criteria:** Specific test scenarios that the Tester Agent must write (e.g., "Must write a unit test checking token expiration").

### Step 4: Execute the Autonomous Loop
Run the `start` command to spin up the daemon and enter the REPL:
```bash
noctifab start
```
From the REPL prompt, enqueue your user story:
```text
> start roadmap/US-001_auth_service.md
```
Alternatively, for non-interactive execution (e.g., in CI/CD pipelines or headless scripts), run:
```bash
noctifab start-one --input roadmap/US-001_auth_service.md --auto-commit
```

### Step 5: Interacting with the Loop (Human-in-the-Loop)
While `noctifab` works, the engineer should:
1. Monitor execution status:
   ```text
   > status
   ```
2. Monitor log progress:
   ```bash
   tail -f .noctifab/logs/daemon.log
   ```
3. Resolve clarifications. When the agent hits an ambiguity, a prompt will appear:
   ```text
   ┌─────────────────────────────────────────────┐
   │ 🤔 CLARIFICATION NEEDED (ID: clar_x9d2)     │
   │                                             │
   │ Should the token be returned in cookies     │
   │ or the JSON body response?                  │
   └─────────────────────────────────────────────┘
   Your answer: Return it in the JSON body under 'access_token'.
   ```
   Providing an answer unblocks the Generator Agent immediately.

### Step 6: Review & Finalize
Once all tasks are successful, the daemon automatically:
1. Compiles task changelogs.
2. Bumps the version file.
3. Pushes the integration branch.
4. Generates a Pull Request to merge the integration branch back into `main`.

The engineer reviews the generated Pull Request, runs any manual verification tests on local staging environments, and clicks **Merge**.

---

## 6. Verdict and Recommendation

*   **For Feature Development and Bug Fixing:** **Strongly Recommended.** `noctifab` saves time by handling the mechanical aspects of TDD, formatting, branch checkout, testing, and branch merging.
*   **For Brand New Project Generation:** **Feasible with scaffolding.** Engineers should not expect the tool to build a system from a completely empty directory. They must first bootstrap the build, testing, and linting configuration manually. Once that scaffolding is complete, the `start` command can build the implementation and test suite autonomously.
