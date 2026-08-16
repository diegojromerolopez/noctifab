package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"golang.org/x/term"
)

// HandleNewOrderPrompt interacts with the user to accept a new story/order specification
// directly from the live dashboard and enqueues it to the running daemon.
func HandleNewOrderPrompt(ctx context.Context, client *services.DaemonClient, fd int, oldState *term.State) error {
	_ = term.Restore(fd, oldState)
	defer func() { _, _ = term.MakeRaw(fd) }()

	fmt.Print("\r\n\033[1;36m➕ ENTER NEW ORDER / FEATURE PROMPT:\033[0m\r\n> ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read prompt input: %w", err)
	}

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		fmt.Print("\r\n\033[1;33m⚠️  No order entered. Returning to dashboard...\033[0m\r\n")
		time.Sleep(1 * time.Second)
		return nil
	}

	// Create a new story file in .noctifab/stories/
	storiesDir := ".noctifab/stories"
	if err := os.MkdirAll(storiesDir, 0755); err != nil {
		return fmt.Errorf("failed to create stories directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	storyFilename := fmt.Sprintf("story_%s.md", timestamp)
	storyPath := filepath.Join(storiesDir, storyFilename)

	content := fmt.Sprintf("# User Order: %s\n\n## Description\n%s\n", trimmed, trimmed)
	if err := os.WriteFile(storyPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write story file: %w", err)
	}

	if err := client.SendStartStory(storyPath); err != nil {
		fmt.Printf("\r\n\033[1;31m❌ Failed to enqueue story order: %v\033[0m\r\n", err)
		time.Sleep(2 * time.Second)
		return err
	}

	fmt.Printf("\r\n\033[1;32m✅ Order enqueued successfully! Story file created: %s\033[0m\r\n", storyPath)
	time.Sleep(2 * time.Second)
	return nil
}

// HandleClarificationPrompt checks for pending clarifications and allows the developer
// to answer them directly inside the live dashboard.
func HandleClarificationPrompt(ctx context.Context, client *services.DaemonClient, fd int, oldState *term.State) error {
	clarifications, err := client.GetPendingClarifications()
	if err != nil || len(clarifications) == 0 {
		_ = term.Restore(fd, oldState)
		fmt.Print("\r\n\033[1;33mℹ️  No pending clarifications found.\033[0m\r\n")
		time.Sleep(1 * time.Second)
		_, _ = term.MakeRaw(fd)
		return nil
	}

	_ = term.Restore(fd, oldState)
	defer func() { _, _ = term.MakeRaw(fd) }()

	fmt.Print("\r\n\033[1;35m❓ PENDING CLARIFICATIONS:\033[0m\r\n")
	for i, c := range clarifications {
		fmt.Printf("%d. [%s] %s\r\n", i+1, c.ID, c.Question)
	}

	fmt.Print("\r\nSelect clarification number to answer (or 'q' to cancel): ")
	reader := bufio.NewReader(os.Stdin)
	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)
	if choiceStr == "q" || choiceStr == "" {
		return nil
	}

	var idx int
	if _, err := fmt.Sscanf(choiceStr, "%d", &idx); err != nil || idx < 1 || idx > len(clarifications) {
		fmt.Print("\r\n\033[1;31mInvalid choice.\033[0m\r\n")
		time.Sleep(1 * time.Second)
		return nil
	}

	target := clarifications[idx-1]
	fmt.Printf("\r\nAnswer for '%s': ", target.Question)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)

	if err := client.ResolveClarification(target.ID, answer); err != nil {
		fmt.Printf("\r\n\033[1;31m❌ Failed to resolve clarification: %v\033[0m\r\n", err)
	} else {
		fmt.Print("\r\n\033[1;32m✅ Clarification resolved!\033[0m\r\n")
	}
	time.Sleep(2 * time.Second)
	return nil
}

// HandleLogInspectorModal renders an interactive full log and failure stack trace inspector modal.
func HandleLogInspectorModal(ctx context.Context, states []*domain.State, fd int, oldState *term.State) error {
	_ = term.Restore(fd, oldState)
	defer func() { _, _ = term.MakeRaw(fd) }()

	fmt.Print("\033[H\033[J")
	fmt.Print("\033[1;36m====================================================================================\033[0m\r\n")
	fmt.Print("\033[1;36m  🔍 NOCTIFAB FAILURE & LOG INSPECTOR MODAL\033[0m\r\n")
	fmt.Print("\033[1;36m====================================================================================\033[0m\r\n\r\n")

	if len(states) == 0 {
		fmt.Print("No active states or failure logs available.\r\n")
		fmt.Print("\r\nPress Enter or 'q' to return to dashboard...")
		buf := make([]byte, 1)
		_, _ = os.Stdin.Read(buf)
		return nil
	}

	deduped := deduplicateStates(states)
	foundFailures := 0

	for _, st := range deduped {
		if st.StoryError != "" {
			foundFailures++
			fmt.Printf("\033[1;31m✖ Story Error [%s]: %s\033[0m\r\n", st.Metadata.FeatureName, st.StoryError)
		}

		for _, t := range st.Tasks {
			if t.Status == domain.TaskFailed || t.Status == domain.TaskConflictFailed || t.FailureLog != "" {
				foundFailures++
				fmt.Printf("\033[1;33m────────────────────────────────────────────────────────────────────────────────────\033[0m\r\n")
				fmt.Printf("\033[1;31m❌ Task: %s (Status: %s, Retries: %d)\033[0m\r\n", t.Title, t.Status, t.Retries)
				if len(t.TargetFiles) > 0 {
					fmt.Printf("   Target Files: %s\r\n", strings.Join(t.TargetFiles, ", "))
				}
				fmt.Printf("\033[1;37m   Full Error Log & Stack Trace Diagnostics:\033[0m\r\n")

				lines := strings.Split(t.FailureLog, "\n")
				if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
					fmt.Print("     (no log content captured)\r\n")
				} else {
					for _, line := range lines {
						if strings.Contains(line, "FAIL") || strings.Contains(line, "Error") || strings.Contains(line, "error:") {
							fmt.Printf("     \033[31m%s\033[0m\r\n", line)
						} else if strings.Contains(line, "=== RUN") || strings.Contains(line, "---") {
							fmt.Printf("     \033[36m%s\033[0m\r\n", line)
						} else {
							fmt.Printf("     %s\r\n", line)
						}
					}
				}
				fmt.Print("\r\n")
			}
		}
	}

	if foundFailures == 0 {
		fmt.Print("\033[1;32m✓ All tasks are passing cleanly! No failure stack traces recorded.\033[0m\r\n\r\n")
		primary := deduped[0]
		if len(primary.LastActions) > 0 {
			fmt.Print("\033[1;36mRecent Completed Actions Output:\033[0m\r\n")
			for i, act := range primary.LastActions {
				fmt.Printf(" [%d] Tool: %s (Success: %v)\r\n     Result: %s\r\n", i+1, act.Tool, act.Success, act.Result)
			}
		}
	}

	fmt.Print("\r\n\033[1;30m------------------------------------------------------------------------------------\033[0m\r\n")
	fmt.Print("\033[1;36mPress Enter, 'q', or Esc to close inspector and return to dashboard...\033[0m ")

	buf := make([]byte, 1)
	_, _ = os.Stdin.Read(buf)
	return nil
}

// HandleSteerPrompt allows the developer to type a mid-flight steering directive directly in the dashboard.
func HandleSteerPrompt(ctx context.Context, client *services.DaemonClient, fd int, oldState *term.State) error {
	_ = term.Restore(fd, oldState)
	defer func() { _, _ = term.MakeRaw(fd) }()

	fmt.Print("\r\n\033[1;33m🎯 ENTER STEERING DIRECTIVE FOR ACTIVE TASK:\033[0m\r\n> ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read steering input: %w", err)
	}

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		fmt.Print("\r\n\033[1;33m⚠️  No steering directive entered. Returning to dashboard...\033[0m\r\n")
		time.Sleep(1 * time.Second)
		return nil
	}

	if err := client.SendSteerDirective(ctx, "", trimmed); err != nil {
		fmt.Printf("\r\n\033[1;31m❌ Failed to send steering directive: %v\033[0m\r\n", err)
		time.Sleep(2 * time.Second)
		return err
	}

	fmt.Printf("\r\n\033[1;32m✅ Steering directive injected into active task: %q\033[0m\r\n", trimmed)
	time.Sleep(2 * time.Second)
	return nil
}
