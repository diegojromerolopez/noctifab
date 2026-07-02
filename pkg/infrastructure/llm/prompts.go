package llm

import (
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type PromptBuilder struct {
	Role         domain.AgentRole
	DetectedLang string
}

func (pb *PromptBuilder) Build(prompt string) string {
	invariants := pb.concurrencyInvariants()
	if invariants == "" {
		return prompt
	}
	if strings.Contains(prompt, "CONCURRENCY & THREADING INVARIANTS") {
		return prompt
	}
	return prompt + "\n\n" + invariants
}

func (pb *PromptBuilder) concurrencyInvariants() string {
	switch pb.DetectedLang {
	case "python":
		return pythonInvariants
	case "go":
		return goInvariants
	default:
		return ""
	}
}

const pythonInvariants = `CONCURRENCY & THREADING INVARIANTS (Python):
1. If executing a task function inside a background thread, capture any
   raised exceptions (including BaseException classes like KeyboardInterrupt
   or SystemExit) and propagate them back to the main thread.
2. The main loop must join or check the thread status frequently
   (e.g., t.join(0.1)) and re-raise any captured exception immediately.
3. Set daemon=True on ALL background threads before t.start().
4. Use signal.signal(signal.SIGINT, handler) to handle Ctrl+C explicitly
   when threads are involved.`

const goInvariants = `CONCURRENCY & THREADING INVARIANTS (Go):
1. Always select on ctx.Done() in goroutines that perform blocking
   operations — never block indefinitely without a context check.
2. Use sync.WaitGroup to track goroutine completion; always call
   wg.Wait() before returning from functions that spawn goroutines.
3. Use buffered channels (size >= 1) for signalling to avoid deadlock
   if the receiver has exited.
4. Use sync.Once for lazy initialization in concurrent contexts.`
