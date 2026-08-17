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
	if s.StoryContracts != nil {
		clone.StoryContracts = make([]StoryContract, len(s.StoryContracts))
		for i, contract := range s.StoryContracts {
			clone.StoryContracts[i] = cloneStoryContract(contract)
		}
	}
	if s.ReviewPhases != nil {
		clone.ReviewPhases = make([]ReviewPhase, len(s.ReviewPhases))
		for i, phase := range s.ReviewPhases {
			clone.ReviewPhases[i] = phase
			clone.ReviewPhases[i].ArtifactManifest = append([]ArtifactManifestEntry(nil), phase.ArtifactManifest...)
		}
	}
	if s.QAScenarios != nil {
		clone.QAScenarios = make([]QAScenario, len(s.QAScenarios))
		for i, scenario := range s.QAScenarios {
			clone.QAScenarios[i] = cloneQAScenario(scenario)
		}
	}
	if s.QAFindings != nil {
		clone.QAFindings = append([]QAFinding(nil), s.QAFindings...)
	}
	if s.Orders != nil {
		clone.Orders = append([]StoryOrder(nil), s.Orders...)
	}
	return &clone
}

func cloneStoryContract(contract StoryContract) StoryContract {
	clone := contract
	clone.PublicContracts = make([]PublicContract, len(contract.PublicContracts))
	for i, publicContract := range contract.PublicContracts {
		clone.PublicContracts[i] = publicContract
		clone.PublicContracts[i].ApplicablePathPrefixes = append([]string(nil), publicContract.ApplicablePathPrefixes...)
		clone.PublicContracts[i].AllowedExecutables = append([]string(nil), publicContract.AllowedExecutables...)
		clone.PublicContracts[i].ExitCodes = append([]int(nil), publicContract.ExitCodes...)
		clone.PublicContracts[i].StdoutContains = append([]string(nil), publicContract.StdoutContains...)
		clone.PublicContracts[i].StderrPrefixes = append([]string(nil), publicContract.StderrPrefixes...)
	}
	return clone
}

func cloneQAScenario(scenario QAScenario) QAScenario {
	clone := scenario
	clone.Steps = make([]QAStep, len(scenario.Steps))
	for i, step := range scenario.Steps {
		clone.Steps[i] = step
		clone.Steps[i].Command = append([]string(nil), step.Command...)
		clone.Steps[i].StdoutContains = append([]string(nil), step.StdoutContains...)
	}
	return clone
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
