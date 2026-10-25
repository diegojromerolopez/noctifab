package demo

import (
	"context"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// MockDemoLLMClient provides deterministic, instant replay responses for demo execution without external API calls.
type MockDemoLLMClient struct {
	SpeedFactor float64
}

// NewMockDemoLLMClient constructs a mock client tailored for offline demo sandbox execution.
func NewMockDemoLLMClient(speedFactor float64) *MockDemoLLMClient {
	if speedFactor <= 0 {
		speedFactor = 1.0
	}
	return &MockDemoLLMClient{SpeedFactor: speedFactor}
}

// Complete returns pre-recorded deterministic LLM actions based on agent role markers in the prompt.
func (m *MockDemoLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	// Simulate lightweight network latency scaled by SpeedFactor
	if m.SpeedFactor > 0 {
		delay := time.Duration(100/m.SpeedFactor) * time.Millisecond
		time.Sleep(delay)
	}

	pUpper := strings.ToUpper(prompt)
	isCLI := strings.Contains(pUpper, "CALCULATOR") || strings.Contains(pUpper, "CLI")

	// 1. Planner Agent
	if strings.Contains(pUpper, "PLANNER") || strings.Contains(pUpper, "DECOMPOSE") {
		if isCLI {
			return &domain.LLMResponse{
				Reasoning: "Decomposing calculator specification into topological tasks",
				Actions: []domain.LLMAction{
					{
						Tool: "add_task",
						Args: map[string]any{
							"id":           "task-1",
							"title":        "Implement Core Arithmetic Operations",
							"description":  "Implement Add, Subtract, Multiply, and Divide with divide-by-zero checks.",
							"target_files": []string{"main.go"},
							"change_type":  "FEATURE",
						},
					},
					{
						Tool: "add_task",
						Args: map[string]any{
							"id":           "task-2",
							"title":        "Harden Unit & Regression Test Suite",
							"description":  "Verify full test coverage across arithmetic and divide-by-zero error conditions.",
							"target_files": []string{"calc_test.go"},
							"depends_on":   []string{"task-1"},
							"change_type":  "FIX",
						},
					},
				},
			}, nil
		}

		return &domain.LLMResponse{
			Reasoning: "Decomposing t4 S3-compatible object store specification into topological tasks",
			Actions: []domain.LLMAction{
				{
					Tool: "add_task",
					Args: map[string]any{
						"id":           "task-1",
						"title":        "Implement ISO C17 HTTP/1.1 Request Parser & Wire Serializer",
						"description":  "Implement http_parse_request and http_write_response adhering strictly to RFC-7230.",
						"target_files": []string{"src/http.c", "src/http.h"},
						"change_type":  "FEATURE",
					},
				},
				{
					Tool: "add_task",
					Args: map[string]any{
						"id":           "task-2",
						"title":        "Harden HTTP Wire Invariants & Protocol Edge-Case Tests",
						"description":  "Verify strict ISO C17 syntax, status code contracts, and header parsing.",
						"target_files": []string{"tests/unit/test_http.c"},
						"depends_on":   []string{"task-1"},
						"change_type":  "FIX",
					},
				},
			},
		}, nil
	}

	// 2. Tester Agent
	if strings.Contains(pUpper, "TESTER") || strings.Contains(pUpper, "TEST") {
		cmd := "make test"
		if isCLI {
			cmd = "go test -v ./..."
		}
		return &domain.LLMResponse{
			Reasoning: "Running and validating black-box behavioral tests",
			Actions: []domain.LLMAction{
				{
					Tool: "run_tests",
					Args: map[string]any{
						"command": cmd,
					},
				},
			},
		}, nil
	}

	// 3. Generator Agent
	if isCLI {
		return &domain.LLMResponse{
			Reasoning: "Implementing calculator functions Add, Subtract, Multiply, Divide",
			Actions: []domain.LLMAction{
				{
					Tool: "write_file",
					Args: map[string]any{
						"path": "main.go",
						"content": `package main

import (
	"errors"
	"fmt"
)

func Add(a, b int) int { return a + b }
func Subtract(a, b int) int { return a - b }
func Multiply(a, b int) int { return a * b }
func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("cannot divide by zero")
	}
	return a / b, nil
}

func main() {
	fmt.Println("Calculator ready.")
}
`,
					},
				},
			},
		}, nil
	}

	return &domain.LLMResponse{
		Reasoning: "Implementing ISO C17 HTTP parser and response formatting functions",
		Actions: []domain.LLMAction{
			{
				Tool: "write_file",
				Args: map[string]any{
					"path": "src/http.c",
					"content": `/* ISO C17 HTTP/1.1 Protocol Parser & Serializer */
#include "http.h"
#include <stdio.h>
#include <string.h>
#include <strings.h>
#include <stdlib.h>
#include <ctype.h>
`,
				},
			},
		},
	}, nil
}
