package usecase

import (
	"context"
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type IntentDisambiguator struct {
	gitClient *GitClient
	llmClient domain.LLMClient
}

func NewIntentDisambiguator(gitClient *GitClient, llmClient domain.LLMClient) *IntentDisambiguator {
	return &IntentDisambiguator{
		gitClient: gitClient,
		llmClient: llmClient,
	}
}

func (id *IntentDisambiguator) Disambiguate(ctx context.Context, clarification domain.Clarification, state *domain.State) (string, error) {
	gitLog, err := id.gitClient.Run(ctx, false, "log", "--oneline", "-30")
	if err != nil || gitLog == "" {
		gitLog = "(no git history available)"
	}

	codeFiles := ""
	for _, f := range state.Files {
		codeFiles += f.Path + "\n"
	}

	prompt := fmt.Sprintf(`The system needs to resolve an ambiguity during autonomous development.

Question from the agent: %s

Context:
- Base branch: %s
- Feature: %s
- Files in workspace:
%s
- Recent commits:
%s

Analyze this context and infer the most likely intended behavior.
Respond with a JSON object: {"answer": "your inferred answer here"}
Be concise — answer in 1-2 sentences.
`, clarification.Question, state.Metadata.BaseBranch, state.Metadata.FeatureName,
		codeFiles, gitLog)

	resp, err := id.llmClient.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("disambiguation LLM call failed: %w", err)
	}

	if resp == nil || len(resp.Actions) == 0 {
		return "", fmt.Errorf("disambiguation LLM returned no actions")
	}

	answer, ok := resp.Actions[0].Args["answer"].(string)
	if !ok || answer == "" {
		return "", fmt.Errorf("disambiguation LLM response missing 'answer' field")
	}

	return answer, nil
}
