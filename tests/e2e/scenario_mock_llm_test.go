package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type mockLLMClient struct {
	repo       domain.StateRepository
	completeFn func(ctx context.Context, prompt string) (*domain.LLMResponse, error)
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if m.completeFn != nil {
		return m.completeFn(ctx, prompt)
	}
	if strings.Contains(prompt, "resolve git conflict") {
		return &domain.LLMResponse{
			Reasoning: "Resolving the Git conflict by combining edits from Agent 1 and Agent 2.",
			Actions: []domain.LLMAction{
				{
					Tool: "write_file",
					Args: map[string]any{
						"path":    "common.py",
						"content": "line 1: content from agent 1 and agent 2 combined\n",
					},
				},
			},
		}, nil
	}

	state, err := m.repo.Load(ctx)
	if err != nil {
		return nil, err
	}

	// Cyclic test mode triggered by cyclic metadata flag
	if strings.Contains(state.Metadata.FeatureName, "cyclic") {
		return &domain.LLMResponse{
			Reasoning: "Creating tasks with a circular dependency to test cycle validation.",
			Actions: []domain.LLMAction{
				{
					Tool: "add_task",
					Args: map[string]any{
						"id":          "task-a",
						"title":       "Task A",
						"description": "Depends on Task B",
						"change_type": "FEATURE",
						"depends_on":  []any{"Task B"},
					},
				},
				{
					Tool: "add_task",
					Args: map[string]any{
						"id":          "task-b",
						"title":       "Task B",
						"description": "Depends on Task A",
						"change_type": "FEATURE",
						"depends_on":  []any{"Task A"},
					},
				},
			},
		}, nil
	}

	// Conflict test mode triggered by conflict metadata flag
	if strings.Contains(state.Metadata.FeatureName, "conflict") {
		if len(state.Tasks) == 0 {
			return &domain.LLMResponse{
				Reasoning: "Planning two parallel tasks that modify the same file to trigger a merge conflict.",
				Actions: []domain.LLMAction{
					{
						Tool: "add_task",
						Args: map[string]any{
							"id":          "task-agent-1",
							"title":       "Agent 1 edits file",
							"description": "Writes first line to common.py",
							"change_type": "FEATURE",
						},
					},
					{
						Tool: "add_task",
						Args: map[string]any{
							"id":          "task-agent-2",
							"title":       "Agent 2 edits file",
							"description": "Writes conflicting line to common.py",
							"change_type": "FEATURE",
						},
					},
				},
			}, nil
		}

		var nextTask *domain.Task
		for i := range state.Tasks {
			if state.Tasks[i].Status != domain.TaskSuccess {
				nextTask = &state.Tasks[i]
				break
			}
		}

		if nextTask == nil {
			return &domain.LLMResponse{
				Reasoning: "Both conflict tasks processed.",
				Actions:   nil,
			}, nil
		}

		if nextTask.ID == "task-agent-1" {
			return &domain.LLMResponse{
				Reasoning: "Agent 1 writing content.",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "common.py",
							"content": "line 1: content from agent 1\n",
						},
					},
				},
			}, nil
		}

		if nextTask.ID == "task-agent-2" {
			return &domain.LLMResponse{
				Reasoning: "Agent 2 writing conflicting content.",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "common.py",
							"content": "line 1: conflicting content from agent 2\n",
						},
					},
				},
			}, nil
		}
	}

	// Phase 1: Clarification Request
	if len(state.Tasks) == 0 {
		if len(state.Clarifications) == 0 {
			return &domain.LLMResponse{
				Reasoning: "Before planning the task DAG, we need to clarify the target database type.",
				Actions: []domain.LLMAction{
					{
						Tool: "request_clarification",
						Args: map[string]any{
							"question": "Should we use SQLite or PostgreSQL?",
						},
					},
				},
			}, nil
		}

		// Once operator answers, Planner proceeds to decompose Django Contact CRUD Notebook
		return &domain.LLMResponse{
			Reasoning: "Creating tasks to implement Django Contact CRUD Notebook with SQLite.",
			Actions: []domain.LLMAction{
				{
					Tool: "add_task",
					Args: map[string]any{
						"id":          "task-setup",
						"title":       "Setup Django project",
						"description": "Initialize base settings and manage.py",
						"change_type": "FEATURE",
					},
				},
				{
					Tool: "add_task",
					Args: map[string]any{
						"id":          "task-model",
						"title":       "Create Contact model",
						"description": "Add Contact class in contacts/models.py",
						"change_type": "FEATURE",
						"depends_on":  []any{"task-setup"},
					},
				},
				{
					Tool: "add_task",
					Args: map[string]any{
						"id":          "task-views",
						"title":       "Implement Django views and templates",
						"description": "Add contact list, create, update, delete functionality",
						"change_type": "FEATURE",
						"depends_on":  []any{"task-model"},
					},
				},
			},
		}, nil
	}

	// Find the next uncompleted task
	var nextTask *domain.Task
	for i := range state.Tasks {
		if state.Tasks[i].Status != domain.TaskSuccess {
			nextTask = &state.Tasks[i]
			break
		}
	}

	if nextTask == nil {
		return &domain.LLMResponse{
			Reasoning: "Verifying Django CRUD contacts application. All files exist and are correct.",
			Actions:   nil,
		}, nil
	}

	// Generator Phase: return file write actions
	switch nextTask.ID {
	case "task-setup":
		return &domain.LLMResponse{
			Reasoning: "Generating base Django project files.",
			Actions: []domain.LLMAction{
				{
					Tool: "write_file",
					Args: map[string]any{
						"path":    "manage.py",
						"content": "#!/usr/bin/env python\nimport os\nimport sys\n# django entry point stub\n",
					},
				},
				{
					Tool: "write_file",
					Args: map[string]any{
						"path":    "notebook/settings.py",
						"content": "INSTALLED_APPS = [\n    'contacts',\n]\n",
					},
				},
			},
		}, nil

	case "task-model":
		return &domain.LLMResponse{
			Reasoning: "Creating user notebook contact model.",
			Actions: []domain.LLMAction{
				{
					Tool: "write_file",
					Args: map[string]any{
						"path":    "contacts/models.py",
						"content": "from django.db import models\n\nclass Contact(models.Model):\n    name = models.CharField(max_length=100)\n    email = models.EmailField()\n    phone = models.CharField(max_length=20)\n    notes = models.TextField()\n",
					},
				},
			},
		}, nil

	case "task-views":
		viewsPath := filepath.Join(state.ProjectPath, "contacts/views.py")
		if _, err := os.Stat(viewsPath); err == nil {
			return &domain.LLMResponse{
				Reasoning: "I see the template was missing. I will now generate the missing contact_list.html template.",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "contacts/templates/contacts/contact_list.html",
							"content": "<html><body><h1>Contact Notebook</h1></body></html>\n",
						},
					},
				},
			}, nil
		}

		return &domain.LLMResponse{
			Reasoning: "Adding Django list/create/update/delete views. Forgetting the HTML template.",
			Actions: []domain.LLMAction{
				{
					Tool: "write_file",
					Args: map[string]any{
						"path":    "contacts/views.py",
						"content": "from django.shortcuts import render\nfrom .models import Contact\n\ndef list_contacts(request):\n    return render(request, 'contacts/contact_list.html', {'contacts': Contact.objects.all()})\n",
					},
				},
			},
		}, nil
	}

	return &domain.LLMResponse{}, nil
}
