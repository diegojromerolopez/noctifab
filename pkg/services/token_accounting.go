package services

import (
	"context"
	"sort"
	"sync"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// TokenAccountingService manages 3-tier token usage tracking across stories, tasks, agents, and state metadata.
type TokenAccountingService interface {
	RecordLLMUsage(ctx context.Context, storyID, taskID, agentID string, usage domain.TokenUsage)
	GetStoryBreakdown(storyID string) domain.StoryTokenBreakdown
	GetAllStoryBreakdowns() []domain.StoryTokenBreakdown
	GetTotalBreakdown() (inputTokens, outputTokens, totalTokens int64)
	ApplyToState(state *domain.State)
}

type memoryTokenAccountingService struct {
	mu          sync.RWMutex
	storyTokens map[string]*domain.StoryTokenBreakdown
	taskTokens  map[string]*domain.TokenUsage
	agentTokens map[string]*domain.TokenUsage
	totalInput  int64
	totalOutput int64
}

// NewTokenAccountingService constructs a new in-memory token accounting service.
func NewTokenAccountingService() TokenAccountingService {
	return &memoryTokenAccountingService{
		storyTokens: make(map[string]*domain.StoryTokenBreakdown),
		taskTokens:  make(map[string]*domain.TokenUsage),
		agentTokens: make(map[string]*domain.TokenUsage),
	}
}

func (s *memoryTokenAccountingService) RecordLLMUsage(ctx context.Context, storyID, taskID, agentID string, usage domain.TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalInput += usage.InputTokens
	s.totalOutput += usage.OutputTokens

	if storyID != "" {
		sb, exists := s.storyTokens[storyID]
		if !exists {
			sb = &domain.StoryTokenBreakdown{StoryID: storyID}
			s.storyTokens[storyID] = sb
		}
		sb.InputTokens += usage.InputTokens
		sb.OutputTokens += usage.OutputTokens
		sb.TotalTokens += usage.TotalTokens
	}

	if taskID != "" {
		tu, exists := s.taskTokens[taskID]
		if !exists {
			tu = &domain.TokenUsage{}
			s.taskTokens[taskID] = tu
		}
		tu.InputTokens += usage.InputTokens
		tu.OutputTokens += usage.OutputTokens
		tu.TotalTokens += usage.TotalTokens
	}

	if agentID != "" {
		au, exists := s.agentTokens[agentID]
		if !exists {
			au = &domain.TokenUsage{}
			s.agentTokens[agentID] = au
		}
		au.InputTokens += usage.InputTokens
		au.OutputTokens += usage.OutputTokens
		au.TotalTokens += usage.TotalTokens
	}
}

func (s *memoryTokenAccountingService) GetStoryBreakdown(storyID string) domain.StoryTokenBreakdown {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if sb, exists := s.storyTokens[storyID]; exists {
		return *sb
	}
	return domain.StoryTokenBreakdown{StoryID: storyID}
}

func (s *memoryTokenAccountingService) GetAllStoryBreakdowns() []domain.StoryTokenBreakdown {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]domain.StoryTokenBreakdown, 0, len(s.storyTokens))
	for _, sb := range s.storyTokens {
		result = append(result, *sb)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StoryID < result[j].StoryID
	})

	return result
}

func (s *memoryTokenAccountingService) GetTotalBreakdown() (inputTokens, outputTokens, totalTokens int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.totalInput, s.totalOutput, s.totalInput + s.totalOutput
}

func (s *memoryTokenAccountingService) ApplyToState(state *domain.State) {
	if state == nil {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	state.Metadata.TotalInputTokens = s.totalInput
	state.Metadata.TotalOutputTokens = s.totalOutput
	state.Metadata.TotalTokensUsed = s.totalInput + s.totalOutput

	for i := range state.Stories {
		if sb, exists := s.storyTokens[state.Stories[i].ID]; exists {
			state.Stories[i].InputTokens = sb.InputTokens
			state.Stories[i].OutputTokens = sb.OutputTokens
			state.Stories[i].TokensUsed = sb.TotalTokens
		}
	}

	for i := range state.Tasks {
		if tu, exists := s.taskTokens[state.Tasks[i].ID]; exists {
			state.Tasks[i].InputTokens = tu.InputTokens
			state.Tasks[i].OutputTokens = tu.OutputTokens
			state.Tasks[i].TokensUsed = tu.TotalTokens
		}
	}

	for i := range state.ActiveAgents {
		if au, exists := s.agentTokens[state.ActiveAgents[i].ID]; exists {
			state.ActiveAgents[i].InputTokens = au.InputTokens
			state.ActiveAgents[i].OutputTokens = au.OutputTokens
			state.ActiveAgents[i].TokensUsed = au.TotalTokens
		}
	}
}
