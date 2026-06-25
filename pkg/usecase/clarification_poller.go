package usecase

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// ClarificationPoller periodically queries the daemon for pending clarification questions
// and presents them to the developer in the foreground terminal, reading their answers.
type ClarificationPoller struct {
	client   *DaemonClient
	pollFreq time.Duration
	in       io.Reader
	out      io.Writer

	// seenIDs tracks clarification IDs already shown to avoid duplicate prompts.
	mu      sync.Mutex
	seenIDs map[string]bool
}

// NewClarificationPoller creates a ClarificationPoller with the given poll frequency.
func NewClarificationPoller(client *DaemonClient, pollFreq time.Duration, in io.Reader, out io.Writer) *ClarificationPoller {
	return &ClarificationPoller{
		client:   client,
		pollFreq: pollFreq,
		in:       in,
		out:      out,
		seenIDs:  make(map[string]bool),
	}
}

// Start runs the polling loop in the background. It returns immediately; cancel ctx to stop.
func (p *ClarificationPoller) Start(ctx context.Context) {
	go p.loop(ctx)
}

func (p *ClarificationPoller) loop(ctx context.Context) {
	ticker := time.NewTicker(p.pollFreq)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkAndPrompt(ctx)
		}
	}
}

// checkAndPrompt fetches pending clarifications and prompts the developer for any new ones.
func (p *ClarificationPoller) checkAndPrompt(ctx context.Context) {
	clarifications, err := p.client.GetPendingClarifications()
	if err != nil {
		// Daemon may not be ready yet; skip silently.
		return
	}

	for _, c := range clarifications {
		p.mu.Lock()
		alreadySeen := p.seenIDs[c.ID]
		p.mu.Unlock()

		if alreadySeen {
			continue
		}

		// Print the clarification question prominently.
		_, _ = fmt.Fprintf(p.out, "\n┌─────────────────────────────────────────────┐\n")
		_, _ = fmt.Fprintf(p.out, "│ 🤔 CLARIFICATION NEEDED (ID: %-15s)│\n", c.ID)
		_, _ = fmt.Fprintf(p.out, "│                                             │\n")
		_, _ = fmt.Fprintf(p.out, "│ %s\n", wrapLine(c.Question, 45))
		_, _ = fmt.Fprintf(p.out, "└─────────────────────────────────────────────┘\n")
		_, _ = fmt.Fprintf(p.out, "Your answer: ")

		answer := p.readLine(ctx)
		if answer == "" {
			continue
		}

		if err := p.client.ResolveClarification(c.ID, answer); err != nil {
			_, _ = fmt.Fprintf(p.out, "⚠ Failed to send answer: %v\n", err)
			continue
		}

		p.mu.Lock()
		p.seenIDs[c.ID] = true
		p.mu.Unlock()

		_, _ = fmt.Fprintf(p.out, "✅ Answer sent.\n> ")
	}
}

// readLine reads a single line from the terminal input, respecting context cancellation.
func (p *ClarificationPoller) readLine(ctx context.Context) string {
	lineCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(p.in)
		if scanner.Scan() {
			lineCh <- strings.TrimSpace(scanner.Text())
		} else {
			lineCh <- ""
		}
	}()

	select {
	case <-ctx.Done():
		return ""
	case line := <-lineCh:
		return line
	}
}

// wrapLine truncates a string to maxLen characters for display in the clarification box.
func wrapLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
