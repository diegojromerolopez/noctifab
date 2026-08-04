package domain

// Clone returns a deep copy of the State. All slices are copied to fresh
// backing arrays so that concurrent task goroutines mutating one copy never
// affect another. Reference types nested inside elements (e.g.
// Task.DependsOn, Action.Args) are also copied.
func (s *State) Clone() *State {
	if s == nil {
		return nil
	}
	clone := *s

	if s.Clarifications != nil {
		clone.Clarifications = append([]Clarification(nil), s.Clarifications...)
	}
	if s.ValidationCriteria != nil {
		clone.ValidationCriteria = append([]ValidationCriterion(nil), s.ValidationCriteria...)
	}
	if s.Tasks != nil {
		clone.Tasks = make([]Task, len(s.Tasks))
		for i, t := range s.Tasks {
			clone.Tasks[i] = cloneTask(t)
		}
	}
	if s.ActiveAgents != nil {
		clone.ActiveAgents = append([]Agent(nil), s.ActiveAgents...)
	}
	if s.Files != nil {
		clone.Files = append([]FileInfo(nil), s.Files...)
	}
	if s.LastActions != nil {
		clone.LastActions = make([]Action, len(s.LastActions))
		for i, a := range s.LastActions {
			clone.LastActions[i] = cloneAction(a)
		}
	}
	return &clone
}

func cloneTask(t Task) Task {
	c := t
	if t.DependsOn != nil {
		c.DependsOn = append([]string(nil), t.DependsOn...)
	}
	if t.TargetFiles != nil {
		c.TargetFiles = append([]string(nil), t.TargetFiles...)
	}
	if t.PartialChangelog != nil {
		c.PartialChangelog = append([]string(nil), t.PartialChangelog...)
	}
	return c
}

func cloneAction(a Action) Action {
	c := a
	if a.Args != nil {
		c.Args = make(map[string]any, len(a.Args))
		for k, v := range a.Args {
			c.Args[k] = v
		}
	}
	return c
}
