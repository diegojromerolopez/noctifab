package usecase

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// listenerSystemPrompt is injected as the LLM context for every operator command.
const listenerSystemPrompt = `You are the command interpreter for noctifab, an autonomous software development dark factory.

Your job is to translate a human operator's text instruction into a structured JSON intent.
You must ONLY respond with a single JSON object and nothing else.

Supported intents:

1. START_STORY - start working on a single user story file
   Triggered by: "start <path/to/story.md>", "work on <file>", "run <file>", "process <file.md>"
   The path may be relative (e.g. roadmap/US-0001.md) or absolute (e.g. $HOME/repos/project/roadmap/US-0001.md).

2. START_DIRECTORY - process all user stories in a directory, in lexicographic order
   Triggered by: "start <path/to/dir>", "run all stories in <dir>", "process directory <dir>"
   The path may be relative or absolute.

3. LIST_STATUS - show the current status of tasks being worked on
   Triggered by: "status", "what's happening", "list tasks", "show progress"

4. UNKNOWN - when no clear intent is detected
   Return a helpful hint to the user in the "message" field.

Response format (JSON only):
{
  "kind": "START_STORY" | "START_DIRECTORY" | "LIST_STATUS" | "UNKNOWN",
  "path": "<exact path as typed by the operator, including any $HOME or relative segments>",
  "message": "<human-readable explanation if applicable>"
}

Do NOT resolve environment variables or relative paths — pass them as-is.`

// ListenerAgent runs in the foreground terminal process (noctifab start).
// It reads operator commands from stdin, calls the LLM to interpret intent, and
// routes the result to the background daemon via the DaemonClient HTTP API.
// A separate ClarificationPoller goroutine surfaces daemon clarification questions inline.
type ListenerAgent struct {
	llmClient domain.LLMClient
	daemon    *DaemonClient
	in        io.Reader
	out       io.Writer
}

// NewListenerAgent constructs a ListenerAgent that communicates with the background daemon.
func NewListenerAgent(
	llmClient domain.LLMClient,
	daemon *DaemonClient,
	in io.Reader,
	out io.Writer,
) *ListenerAgent {
	return &ListenerAgent{
		llmClient: llmClient,
		daemon:    daemon,
		in:        in,
		out:       out,
	}
}

// Start begins the blocking read-interpret-route loop. Exits when ctx is cancelled or EOF.
func (a *ListenerAgent) Start(ctx context.Context) {
	ctx, span := telemetry.Tracer().Start(ctx, "Start")
	defer span.End()
	_, _ = fmt.Fprintln(a.out, "noctifab ready. Type a command (e.g. 'start roadmap/US-0001.md' or 'status').")
	_, _ = fmt.Fprint(a.out, "> ")

	scanner := bufio.NewScanner(a.in)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !scanner.Scan() {
			return
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			_, _ = fmt.Fprint(a.out, "> ")
			continue
		}

		intent, err := a.interpretCommand(ctx, line)
		if err != nil {
			_, _ = fmt.Fprintf(a.out, "⚠ Error interpreting command: %v\n> ", err)
			continue
		}

		a.routeIntent(ctx, intent)
		_, _ = fmt.Fprint(a.out, "> ")
	}
}

// interpretCommand sends the raw operator text to the LLM and returns a parsed Intent.
// Falls back to rule-based parsing on LLM error.
func (a *ListenerAgent) interpretCommand(ctx context.Context, rawInput string) (Intent, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "interpretCommand",
		trace.WithAttributes(attribute.Int("input_length", len(rawInput))))
	defer span.End()
	llmResp, err := a.llmClient.Complete(ctx, listenerSystemPrompt+"\n\nOperator command: "+rawInput)
	if err != nil {
		return a.ruleBasedParse(rawInput), nil
	}

	// Extract JSON from reasoning field (the prompt instructs JSON-only output).
	rawJSON := llmResp.Reasoning
	var llmIntent LLMIntentResponse
	if jsonErr := json.Unmarshal([]byte(rawJSON), &llmIntent); jsonErr != nil {
		return a.ruleBasedParse(rawInput), nil
	}

	return ParseIntentFromLLMResponse(llmIntent), nil
}

// ruleBasedParse is a lightweight fallback that handles the most common command patterns.
func (a *ListenerAgent) ruleBasedParse(input string) Intent {
	lower := strings.ToLower(strings.TrimSpace(input))

	switch {
	case lower == "status" || lower == "list" || lower == "list tasks" ||
		lower == "show progress" || lower == "what's happening":
		return Intent{Kind: IntentKindListStatus}

	case strings.HasPrefix(lower, "start "):
		// Preserve original casing for path
		path := strings.TrimSpace(input[len("start "):])
		path = os.ExpandEnv(path)
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			return Intent{Kind: IntentKindStartStory, Path: path}
		}
		return Intent{Kind: IntentKindStartDirectory, Path: path}

	default:
		return Intent{
			Kind:    IntentKindUnknown,
			Message: fmt.Sprintf("I did not understand %q. Try: start <path/to/story.md> | start <dir> | status", input),
		}
	}
}

// routeIntent dispatches the parsed intent to the daemon via HTTP.
func (a *ListenerAgent) routeIntent(ctx context.Context, intent Intent) {
	ctx, span := telemetry.Tracer().Start(ctx, "routeIntent",
		trace.WithAttributes(attribute.String("intent_kind", string(intent.Kind))))
	defer span.End()
	switch intent.Kind {
	case IntentKindStartStory:
		_, _ = fmt.Fprintf(a.out, "▶ Queuing user story: %s\n", intent.Path)
		if err := a.daemon.SendStartStory(intent.Path); err != nil {
			_, _ = fmt.Fprintf(a.out, "⚠ Failed to queue story: %v\n", err)
		}

	case IntentKindStartDirectory:
		_, _ = fmt.Fprintf(a.out, "▶ Queuing all user stories in directory: %s\n", intent.Path)
		if err := a.daemon.SendStartDirectory(intent.Path); err != nil {
			_, _ = fmt.Fprintf(a.out, "⚠ Failed to queue directory: %v\n", err)
		}

	case IntentKindListStatus:
		a.printStatus()

	case IntentKindUnknown:
		_, _ = fmt.Fprintf(a.out, "⚠ %s\n", intent.Message)
	}
}

// printStatus fetches state from the daemon and prints a human-readable task summary.
func (a *ListenerAgent) printStatus() {
	state, err := a.daemon.GetStatus()
	if err != nil {
		_, _ = fmt.Fprintf(a.out, "⚠ Cannot reach daemon: %v\n", err)
		return
	}

	if len(state.Tasks) == 0 {
		_, _ = fmt.Fprintln(a.out, "No tasks are currently tracked. Use 'start <path>' to begin.")
		return
	}

	_, _ = fmt.Fprintf(a.out, "Current story: %s | Build: %s\n", state.Metadata.FeatureName, string(state.BuildStatus))
	for _, t := range state.Tasks {
		_, _ = fmt.Fprintf(a.out, "  [%-12s] %s\n", t.Status, t.Title)
	}
}
