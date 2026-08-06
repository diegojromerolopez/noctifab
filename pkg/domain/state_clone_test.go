package domain

import (
	"testing"
	"time"
)

func TestStateClone(t *testing.T) {
	t.Run("when state is nil, it returns nil", func(t *testing.T) {
		var s *State
		if s.Clone() != nil {
			t.Error("expected nil clone for nil state")
		}
	})

	t.Run("when cloning, scalar fields are copied", func(t *testing.T) {
		s := &State{ID: "s1", ProjectPath: "/tmp/p", Version: 7, BuildStatus: BuildPassing, StoryStatus: StoryRunning}
		c := s.Clone()
		if c.ID != "s1" || c.ProjectPath != "/tmp/p" || c.Version != 7 || c.BuildStatus != BuildPassing || c.StoryStatus != StoryRunning {
			t.Errorf("scalar fields not copied correctly: %+v", c)
		}
	})

	t.Run("when mutating the clone's task slices, the original is unchanged", func(t *testing.T) {
		s := &State{
			Tasks: []Task{{
				ID:               "t1",
				DependsOn:        []string{"t0"},
				TargetFiles:      []string{"a.go"},
				PartialChangelog: []string{"entry"},
			}},
		}
		c := s.Clone()
		c.Tasks[0].ID = "mutated"
		c.Tasks[0].DependsOn[0] = "mutated-dep"
		c.Tasks[0].TargetFiles[0] = "mutated.go"
		c.Tasks[0].PartialChangelog[0] = "mutated-entry"

		if s.Tasks[0].ID != "t1" {
			t.Error("original Task.ID mutated through clone")
		}
		if s.Tasks[0].DependsOn[0] != "t0" {
			t.Error("original Task.DependsOn shares backing array with clone")
		}
		if s.Tasks[0].TargetFiles[0] != "a.go" {
			t.Error("original Task.TargetFiles shares backing array with clone")
		}
		if s.Tasks[0].PartialChangelog[0] != "entry" {
			t.Error("original Task.PartialChangelog shares backing array with clone")
		}
	})

	t.Run("when appending to the clone's slices, the original length is unchanged", func(t *testing.T) {
		s := &State{
			Tasks:        []Task{{ID: "t1"}},
			Files:        []FileInfo{{Path: "a.go", Size: 1, LastModified: time.Now()}},
			LastActions:  []Action{{Tool: "x"}},
			ActiveAgents: []Agent{{ID: "a1"}},
		}
		c := s.Clone()
		c.Tasks = append(c.Tasks, Task{ID: "t2"})
		c.Files = append(c.Files, FileInfo{Path: "b.go"})
		c.LastActions = append(c.LastActions, Action{Tool: "y"})
		c.ActiveAgents = append(c.ActiveAgents, Agent{ID: "a2"})

		if len(s.Tasks) != 1 || len(s.Files) != 1 || len(s.LastActions) != 1 || len(s.ActiveAgents) != 1 {
			t.Error("original slices were affected by appends to the clone")
		}
	})

	t.Run("when mutating the clone's action args map, the original is unchanged", func(t *testing.T) {
		s := &State{LastActions: []Action{{Tool: "x", Args: map[string]any{"k": "v"}}}}
		c := s.Clone()
		c.LastActions[0].Args["k"] = "mutated"
		if s.LastActions[0].Args["k"] != "v" {
			t.Error("original Action.Args map shared with clone")
		}
	})

	t.Run("when cloning clarifications and validation criteria, they are independent", func(t *testing.T) {
		s := &State{
			Clarifications:     []Clarification{{ID: "c1", Question: "q"}},
			ValidationCriteria: []ValidationCriterion{{ID: "v1"}},
		}
		c := s.Clone()
		c.Clarifications[0].Question = "mutated"
		c.ValidationCriteria[0].ID = "mutated"
		if s.Clarifications[0].Question != "q" || s.ValidationCriteria[0].ID != "v1" {
			t.Error("original clarifications/criteria mutated through clone")
		}
	})
}
