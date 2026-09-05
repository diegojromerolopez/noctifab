package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	dependsOnRE       = regexp.MustCompile(`(?i)(?:^|\n|\*\*)\s*depends_on\s*:\s*(\[.*?\]|"(?:.*?)"|'(?:.*?)'|[\w/.-]+)`)
	storyIDFromPathRE = regexp.MustCompile(`(?i)US-(\d+)`)
)

// StoryDAGNodeStatus tracks the scheduling status of a user story node.
type StoryDAGNodeStatus string

const (
	StoryNodePending StoryDAGNodeStatus = "PENDING"
	StoryNodeRunning StoryDAGNodeStatus = "RUNNING"
	StoryNodeSuccess StoryDAGNodeStatus = "SUCCESS"
	StoryNodeFailed  StoryDAGNodeStatus = "FAILED"
)

// StoryDAGNode represents a single user story node in the scheduling DAG.
type StoryDAGNode struct {
	Item      StoryWorkItem
	StoryID   string
	DependsOn []string
	Status    StoryDAGNodeStatus
	Error     error
}

// StoryDAGScheduler manages parallel execution of user stories based on dependency DAG.
type StoryDAGScheduler struct {
	nodes         map[string]*StoryDAGNode
	storyIDs      []string // preserves order of discovery
	mu            sync.Mutex
	maxConcurrent int
	pipelined     bool
	gitMergeMutex sync.Mutex // serializes git branch merging during state finalization
}

// NewStoryDAGScheduler initializes a StoryDAGScheduler with a maximum concurrency limit.
func NewStoryDAGScheduler(maxConcurrent int) *StoryDAGScheduler {
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	return &StoryDAGScheduler{
		nodes:         make(map[string]*StoryDAGNode),
		maxConcurrent: maxConcurrent,
	}
}

// SetPipelined configures whether child stories can begin planning and task dispatch
// concurrently once parent stories are running, unblocking fine-grained task pipelining.
func (s *StoryDAGScheduler) SetPipelined(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pipelined = enabled
}

// IsPipelined returns whether pipelined scheduling is enabled.
func (s *StoryDAGScheduler) IsPipelined() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pipelined
}

// AddStory parses a StoryWorkItem and adds it to the scheduling graph.
func (s *StoryDAGScheduler) AddStory(item StoryWorkItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	storyID := ExtractStoryID(item.Path)
	if storyID == "" {
		storyID = filepath.Base(item.Path)
	}

	deps := ParseStoryDependencies(item.Spec)

	node := &StoryDAGNode{
		Item:      item,
		StoryID:   storyID,
		DependsOn: deps,
		Status:    StoryNodePending,
	}

	if _, exists := s.nodes[storyID]; !exists {
		s.storyIDs = append(s.storyIDs, storyID)
	}
	s.nodes[storyID] = node
}

// MarkStoryCompleted marks a story node as already succeeded without executing it, unblocking dependent stories.
func (s *StoryDAGScheduler) MarkStoryCompleted(storyIDOrPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	storyID := ExtractStoryID(storyIDOrPath)
	if storyID == "" {
		storyID = filepath.Base(storyIDOrPath)
	}

	if node, exists := s.nodes[storyID]; exists {
		node.Status = StoryNodeSuccess
	}
}

// Execute runs all queued user stories concurrently according to the dependency DAG.
// processFunc is invoked concurrently for each unblocked user story.
func (s *StoryDAGScheduler) Execute(ctx context.Context, processFunc func(ctx context.Context, item StoryWorkItem) error) error {
	s.mu.Lock()
	if len(s.nodes) == 0 {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	cond := sync.NewCond(&s.mu)
	var activeCount int
	var firstErr error

	for {
		s.mu.Lock()

		// Check if all nodes have finished
		allFinished := true
		for _, node := range s.nodes {
			if node.Status == StoryNodePending || node.Status == StoryNodeRunning {
				allFinished = false
				break
			}
		}

		if allFinished {
			s.mu.Unlock()
			break
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			s.mu.Unlock()
			return ctx.Err()
		default:
		}

		// Find next dispatchable nodes
		var dispatch []*StoryDAGNode
		for _, id := range s.storyIDs {
			node := s.nodes[id]
			if node.Status != StoryNodePending {
				continue
			}
			if activeCount >= s.maxConcurrent {
				break
			}

			// Check if all dependencies are satisfied
			depsSatisfied := true
			for _, depID := range node.DependsOn {
				depNode, exists := s.nodes[depID]
				if !exists {
					depsSatisfied = false
					break
				}
				if s.pipelined {
					// In pipelined mode, child stories can be dispatched to start planning
					// and task execution as soon as parent stories are RUNNING or SUCCESS.
					if depNode.Status != StoryNodeRunning && depNode.Status != StoryNodeSuccess {
						depsSatisfied = false
						break
					}
				} else {
					if depNode.Status != StoryNodeSuccess {
						depsSatisfied = false
						break
					}
				}
			}

			if depsSatisfied {
				node.Status = StoryNodeRunning
				activeCount++
				dispatch = append(dispatch, node)
			}
		}

		if len(dispatch) == 0 && activeCount == 0 {
			// Deadlock detection: pending nodes exist but none can be dispatched
			var pendingIDs []string
			for _, node := range s.nodes {
				if node.Status == StoryNodePending {
					pendingIDs = append(pendingIDs, fmt.Sprintf("%s (deps: %v)", node.StoryID, node.DependsOn))
				}
			}
			s.mu.Unlock()
			return fmt.Errorf("story DAG deadlock detected: unable to dispatch pending stories: %s", strings.Join(pendingIDs, ", "))
		}

		s.mu.Unlock()

		// Launch dispatched nodes concurrently
		for _, node := range dispatch {
			nodeCopy := node
			go func(n *StoryDAGNode) {
				fmt.Printf("🚀 [Story DAG Scheduler] Starting story %s (%s)\n", n.StoryID, n.Item.Path)
				err := processFunc(ctx, n.Item)

				s.mu.Lock()
				activeCount--
				if err != nil {
					n.Status = StoryNodeFailed
					n.Error = err
					if firstErr == nil {
						firstErr = fmt.Errorf("story %s failed: %w", n.StoryID, err)
					}
					fmt.Fprintf(os.Stderr, "❌ [Story DAG Scheduler] Story %s failed: %v\n", n.StoryID, err)
				} else {
					n.Status = StoryNodeSuccess
					fmt.Printf("✅ [Story DAG Scheduler] Completed story %s\n", n.StoryID)
				}
				s.mu.Unlock()
				cond.Broadcast()
			}(nodeCopy)
		}

		// Wait for progress
		s.mu.Lock()
		if activeCount > 0 && len(dispatch) == 0 {
			cond.Wait()
		}
		s.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	return firstErr
}

// GitMergeLock provides a mutex to serialize git branch merging operations during concurrent story execution.
func (s *StoryDAGScheduler) GitMergeLock() func() {
	s.gitMergeMutex.Lock()
	return s.gitMergeMutex.Unlock
}

// ExtractStoryID extracts the story ID prefix (e.g. US-001) from a path or string.
func ExtractStoryID(path string) string {
	base := filepath.Base(path)
	matches := storyIDFromPathRE.FindStringSubmatch(base)
	if len(matches) > 1 {
		return fmt.Sprintf("US-%03s", matches[1])
	}
	return ""
}

// ParseStoryDependencies extracts declared parent story IDs (e.g. ["US-001"]) from markdown content.
func ParseStoryDependencies(markdown string) []string {
	matches := dependsOnRE.FindStringSubmatch(markdown)
	if len(matches) < 2 {
		return nil
	}

	rawVal := strings.TrimSpace(matches[1])
	var rawList []string

	if strings.HasPrefix(rawVal, "[") && strings.HasSuffix(rawVal, "]") {
		var yamlList []string
		if err := yaml.Unmarshal([]byte(rawVal), &yamlList); err == nil {
			rawList = yamlList
		} else {
			// Fallback string split
			inner := strings.Trim(rawVal, "[]")
			parts := strings.Split(inner, ",")
			for _, p := range parts {
				pClean := strings.Trim(strings.TrimSpace(p), "\"'`")
				if pClean != "" {
					rawList = append(rawList, pClean)
				}
			}
		}
	} else {
		pClean := strings.Trim(rawVal, "\"'`")
		if pClean != "" {
			rawList = append(rawList, pClean)
		}
	}

	var normalized []string
	seen := make(map[string]bool)
	for _, dep := range rawList {
		id := ExtractStoryID(dep)
		if id != "" {
			if !seen[id] {
				seen[id] = true
				normalized = append(normalized, id)
			}
		} else {
			clean := strings.TrimSpace(dep)
			if clean != "" && !seen[clean] {
				seen[clean] = true
				normalized = append(normalized, clean)
			}
		}
	}

	return normalized
}
