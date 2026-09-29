package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	cond          *sync.Cond
	maxConcurrent int
	pipelined     bool
	gitMergeMutex sync.Mutex // serializes git branch merging during state finalization
	streaming     bool
	streamClosed  bool
}

// NewStoryDAGScheduler initializes a StoryDAGScheduler with a maximum concurrency limit.
func NewStoryDAGScheduler(maxConcurrent int) *StoryDAGScheduler {
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	s := &StoryDAGScheduler{
		nodes:         make(map[string]*StoryDAGNode),
		maxConcurrent: maxConcurrent,
	}
	s.cond = sync.NewCond(&s.mu)
	return s
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

// SetStreaming configures the scheduler for streaming story ingestion.
// While streaming is active, the scheduler will not terminate or declare a deadlock
// if the queue is empty; it waits until CloseStoryStream is called.
func (s *StoryDAGScheduler) SetStreaming(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streaming = enabled
	if !enabled {
		s.streamClosed = true
	}
	if s.cond != nil {
		s.cond.Broadcast()
	}
}

// CloseStoryStream signals that all stories have been fed into the scheduler.
func (s *StoryDAGScheduler) CloseStoryStream() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamClosed = true
	if s.cond != nil {
		s.cond.Broadcast()
	}
}

// AddStory parses a StoryWorkItem and adds it to the scheduling graph.
func (s *StoryDAGScheduler) AddStory(item StoryWorkItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	storyID := ExtractStoryID(item.Path)
	if storyID == "" {
		storyID = filepath.Base(item.Path)
	}

	rawDeps := ParseStoryDependencies(item.Spec)
	var deps []string
	for _, dep := range rawDeps {
		if dep != storyID && dep != filepath.Base(item.Path) {
			deps = append(deps, dep)
		}
	}

	if existing, exists := s.nodes[storyID]; exists {
		existing.Item = item
		existing.DependsOn = deps
	} else {
		s.storyIDs = append(s.storyIDs, storyID)
		s.nodes[storyID] = &StoryDAGNode{
			Item:      item,
			StoryID:   storyID,
			DependsOn: deps,
			Status:    StoryNodePending,
		}
	}
	s.sortStoryIDsLocked()
	if s.cond != nil {
		s.cond.Broadcast()
	}
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

func isSkeletonStory(node *StoryDAGNode) bool {
	if node == nil {
		return false
	}
	lowerPath := strings.ToLower(node.Item.Path)
	lowerSpec := strings.ToLower(node.Item.Spec)
	return strings.Contains(lowerPath, "skeleton") || strings.Contains(lowerPath, "scaffold") ||
		strings.Contains(lowerPath, "foundation") || strings.Contains(lowerSpec, "walking skeleton")
}

// sortStoryIDsLocked prioritizes zero-dependency stories (especially walking skeletons/scaffolds)
// so the foundation entrypoint is built first before dependent feature stories.
// Caller must hold s.mu.
func (s *StoryDAGScheduler) sortStoryIDsLocked() {
	sort.SliceStable(s.storyIDs, func(i, j int) bool {
		nodeI := s.nodes[s.storyIDs[i]]
		nodeJ := s.nodes[s.storyIDs[j]]
		if nodeI == nil || nodeJ == nil {
			return false
		}
		lenI := len(nodeI.DependsOn)
		lenJ := len(nodeJ.DependsOn)
		if lenI != lenJ {
			return lenI < lenJ
		}
		isSkelI := isSkeletonStory(nodeI)
		isSkelJ := isSkeletonStory(nodeJ)
		if isSkelI != isSkelJ {
			return isSkelI
		}
		return s.storyIDs[i] < s.storyIDs[j]
	})
}

// Execute runs all queued user stories concurrently according to the dependency DAG.
// processFunc is invoked concurrently for each unblocked user story.
func (s *StoryDAGScheduler) Execute(ctx context.Context, processFunc func(ctx context.Context, item StoryWorkItem) error) error {
	s.mu.Lock()
	if len(s.nodes) == 0 && (!s.streaming || s.streamClosed) {
		s.mu.Unlock()
		return nil
	}
	s.sortStoryIDsLocked()
	if s.cond == nil {
		s.cond = sync.NewCond(&s.mu)
	}
	cond := s.cond
	s.mu.Unlock()

	stopWake := make(chan struct{})
	defer close(stopWake)
	go func() {
		select {
		case <-ctx.Done():
			cond.Broadcast()
		case <-stopWake:
		}
	}()

	var activeCount int
	var firstErr error

	for {
		s.mu.Lock()

		// Check if all nodes have finished
		allFinished := true
		if s.streaming && !s.streamClosed {
			allFinished = false
		} else {
			for _, node := range s.nodes {
				if node.Status == StoryNodePending || node.Status == StoryNodeRunning {
					allFinished = false
					break
				}
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
			if s.streaming && !s.streamClosed {
				cond.Wait()
				s.mu.Unlock()
				continue
			}
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
		if (activeCount > 0 || (s.streaming && !s.streamClosed)) && len(dispatch) == 0 {
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
