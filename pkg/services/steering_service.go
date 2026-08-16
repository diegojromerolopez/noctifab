package services

import (
	"fmt"
	"sync"
	"time"
)

// SteeringDirective represents a developer steering order injected into an active task or session.
type SteeringDirective struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id,omitempty"` // Empty if global
	Author    string    `json:"author"`
	Directive string    `json:"directive"`
	CreatedAt time.Time `json:"created_at"`
	Consumed  bool      `json:"consumed"`
}

// SteeringService brokers mid-flight steering orders and pause/resume execution signals.
type SteeringService struct {
	mu               sync.RWMutex
	taskDirectives   map[string][]SteeringDirective // Key: TaskID
	globalDirectives []SteeringDirective
	pauseChan        chan struct{}
	resumeChan       chan struct{}
	isPaused         bool
}

// NewSteeringService constructs a thread-safe directive broker.
func NewSteeringService() *SteeringService {
	return &SteeringService{
		taskDirectives: make(map[string][]SteeringDirective),
		pauseChan:      make(chan struct{}, 1),
		resumeChan:     make(chan struct{}, 1),
	}
}

// InjectDirective enqueues a steering order for a specific task.
func (s *SteeringService) InjectDirective(taskID, directiveText string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	directive := SteeringDirective{
		ID:        fmt.Sprintf("dir-%d", time.Now().UnixNano()),
		TaskID:    taskID,
		Author:    "HUMAN_OPERATOR",
		Directive: directiveText,
		CreatedAt: time.Now().UTC(),
		Consumed:  false,
	}

	s.taskDirectives[taskID] = append(s.taskDirectives[taskID], directive)
	return nil
}

// InjectGlobalDirective enqueues a steering order applied across the active run.
func (s *SteeringService) InjectGlobalDirective(directiveText string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	directive := SteeringDirective{
		ID:        fmt.Sprintf("gdir-%d", time.Now().UnixNano()),
		Author:    "HUMAN_OPERATOR",
		Directive: directiveText,
		CreatedAt: time.Now().UTC(),
		Consumed:  false,
	}

	s.globalDirectives = append(s.globalDirectives, directive)
	return nil
}

// ConsumeDirectives retrieves and marks unconsumed directives for a task and global directives.
func (s *SteeringService) ConsumeDirectives(taskID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []string

	// Consume global directives
	for i := range s.globalDirectives {
		if !s.globalDirectives[i].Consumed {
			s.globalDirectives[i].Consumed = true
			result = append(result, s.globalDirectives[i].Directive)
		}
	}

	// Consume task-specific directives
	list := s.taskDirectives[taskID]
	for i := range list {
		if !list[i].Consumed {
			list[i].Consumed = true
			result = append(result, list[i].Directive)
		}
	}
	return result
}

// Pause signals the orchestrator loop to pause execution.
func (s *SteeringService) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isPaused {
		s.isPaused = true
		select {
		case s.pauseChan <- struct{}{}:
		default:
		}
	}
}

// Resume signals the orchestrator loop to resume execution.
func (s *SteeringService) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isPaused {
		s.isPaused = false
		select {
		case s.resumeChan <- struct{}{}:
		default:
		}
	}
}

// IsPaused reports if the orchestrator is currently in paused state.
func (s *SteeringService) IsPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isPaused
}

// PauseChan returns notification channel for pause requests.
func (s *SteeringService) PauseChan() <-chan struct{} {
	return s.pauseChan
}

// ResumeChan returns notification channel for resume requests.
func (s *SteeringService) ResumeChan() <-chan struct{} {
	return s.resumeChan
}
