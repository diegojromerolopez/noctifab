package llm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"
)

// readSSEResponse reads lines from an SSE response stream with a sliding idle timeout.
// Whenever a new chunk line is received, the idle timeout timer is reset.
// If no token chunk is received for idleTimeout, it returns an idle timeout error.
func readSSEResponse(ctx context.Context, body io.ReadCloser, idleTimeout time.Duration, onChunk func(line string) error) error {
	defer func() { _ = body.Close() }()

	if idleTimeout <= 0 {
		idleTimeout = 15 * time.Second
	}

	type lineResult struct {
		line string
		err  error
	}

	lineChan := make(chan lineResult, 16)

	go func() {
		reader := bufio.NewReader(body)
		for {
			line, err := reader.ReadString('\n')
			if line != "" || err != nil {
				lineChan <- lineResult{line: line, err: err}
			}
			if err != nil {
				break
			}
		}
		close(lineChan)
	}()

	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-lineChan:
			if !ok {
				return nil
			}
			if res.err != nil && res.err != io.EOF {
				return res.err
			}

			// Reset idle timer on chunk arrival
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)

			if err := onChunk(res.line); err != nil {
				return err
			}
			if res.err == io.EOF {
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("LLM streaming idle timeout: 0 tokens received for %v continuously", idleTimeout)
		}
	}
}
