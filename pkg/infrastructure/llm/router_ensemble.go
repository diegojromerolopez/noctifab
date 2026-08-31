package llm

import (
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

// buildEnsembleCandidate constructs an ensemble client based on EnsembleConfig.
func (r *ResilientLLMRouter) buildEnsembleCandidate(roleName string, ens config.EnsembleConfig) *RouterCandidate {
	if !ens.IsEnabled() {
		return nil
	}

	timeout := time.Duration(ens.TimeoutSeconds) * time.Second
	softTimeout := time.Duration(ens.SoftTimeoutSeconds) * time.Second

	switch ens.Strategy {
	case config.EnsembleStrategyParallel:
		models := r.resolveNamedClients(ens.Models)
		var synth domain.LLMClient
		if ens.Synthesizer != nil {
			synth = r.resolveClientFromRef(*ens.Synthesizer)
		} else if r.defaultClient != nil {
			synth = r.defaultClient
		}
		pc := ensemble.NewParallelClient(models, synth, ens.MinModels, softTimeout, timeout, ens.FallbackToSingle)
		c := ensemble.NewClient(ens.Strategy, pc)
		return &RouterCandidate{
			Name:     roleName + "-ensemble-parallel",
			Provider: "ensemble",
			Model:    string(ens.Strategy),
			Client:   c,
		}

	case config.EnsembleStrategySerial:
		var stages []ensemble.StageClient
		for _, s := range ens.Stages {
			ref := config.AgentProviderRef{
				Name:        s.Name,
				MaxTokens:   s.MaxTokens,
				Temperature: s.Temperature,
				ExtraParams: s.ExtraParams,
			}
			client := r.resolveClientFromRef(ref)
			if client != nil {
				stages = append(stages, ensemble.StageClient{
					Name:             s.Name,
					Client:           client,
					RefinementPrompt: s.RefinementPrompt,
					MaxTokens:        s.MaxTokens,
				})
			}
		}
		sc := ensemble.NewSerialClient(stages, ens.EarlyExitOnPass, ens.FallbackOnStageFailure, timeout)
		c := ensemble.NewClient(ens.Strategy, sc)
		return &RouterCandidate{
			Name:     roleName + "-ensemble-serial",
			Provider: "ensemble",
			Model:    string(ens.Strategy),
			Client:   c,
		}

	case config.EnsembleStrategyConsensus:
		voters := r.resolveNamedClients(ens.Voters)
		var tieBreaker domain.LLMClient
		if ens.TieBreaker != nil {
			tieBreaker = r.resolveClientFromRef(*ens.TieBreaker)
		}
		cc := ensemble.NewConsensusClient(voters, tieBreaker, timeout)
		c := ensemble.NewClient(ens.Strategy, cc)
		return &RouterCandidate{
			Name:     roleName + "-ensemble-consensus",
			Provider: "ensemble",
			Model:    string(ens.Strategy),
			Client:   c,
		}

	case config.EnsembleStrategyRace:
		models := r.resolveNamedClients(ens.Models)
		rc := ensemble.NewRaceClient(models, timeout)
		c := ensemble.NewClient(ens.Strategy, rc)
		return &RouterCandidate{
			Name:     roleName + "-ensemble-race",
			Provider: "ensemble",
			Model:    string(ens.Strategy),
			Client:   c,
		}

	case config.EnsembleStrategyCascade:
		tiers := r.resolveNamedClients(ens.Tiers)
		cc := ensemble.NewCascadeClient(tiers, timeout)
		c := ensemble.NewClient(ens.Strategy, cc)
		return &RouterCandidate{
			Name:     roleName + "-ensemble-cascade",
			Provider: "ensemble",
			Model:    string(ens.Strategy),
			Client:   c,
		}

	case config.EnsembleStrategyDecomposed:
		var targets []ensemble.TargetClient
		for _, t := range ens.Targets {
			ref := config.AgentProviderRef{
				Name:        t.Name,
				MaxTokens:   t.MaxTokens,
				ExtraParams: t.ExtraParams,
			}
			client := r.resolveClientFromRef(ref)
			if client != nil {
				targets = append(targets, ensemble.TargetClient{
					Name:       t.Name,
					Client:     client,
					RolePrompt: t.RolePrompt,
					MaxTokens:  t.MaxTokens,
				})
			}
		}
		dc := ensemble.NewDecomposedClient(targets, timeout)
		c := ensemble.NewClient(ens.Strategy, dc)
		return &RouterCandidate{
			Name:     roleName + "-ensemble-decomposed",
			Provider: "ensemble",
			Model:    string(ens.Strategy),
			Client:   c,
		}

	case config.EnsembleStrategyBestOfNScored:
		models := r.resolveNamedClients(ens.Models)
		bc := ensemble.NewScoredClient(models, timeout)
		c := ensemble.NewClient(ens.Strategy, bc)
		return &RouterCandidate{
			Name:     roleName + "-ensemble-scored",
			Provider: "ensemble",
			Model:    string(ens.Strategy),
			Client:   c,
		}

	case config.EnsembleStrategyAdaptive:
		var fastClient, standardClient, heavyClient domain.LLMClient
		if len(ens.FastTier) > 0 {
			fastNamed := r.resolveNamedClients(ens.FastTier)
			if len(fastNamed) > 1 {
				fastClient = ensemble.NewRaceClient(fastNamed, timeout)
			} else if len(fastNamed) == 1 {
				fastClient = fastNamed[0].Client
			}
		}
		if len(ens.StandardTier) > 0 {
			standardNamed := r.resolveNamedClients(ens.StandardTier)
			if len(standardNamed) > 0 {
				standardClient = standardNamed[0].Client
			}
		} else if r.defaultClient != nil {
			standardClient = r.defaultClient
		}
		if len(ens.HeavyTier) > 0 {
			heavyNamed := r.resolveNamedClients(ens.HeavyTier)
			if len(heavyNamed) > 1 {
				heavyClient = ensemble.NewParallelClient(heavyNamed, standardClient, 2, softTimeout, timeout, true)
			} else if len(heavyNamed) == 1 {
				heavyClient = heavyNamed[0].Client
			}
		}
		ac := ensemble.NewAdaptiveClient(fastClient, standardClient, heavyClient, timeout)
		c := ensemble.NewClient(ens.Strategy, ac)
		return &RouterCandidate{
			Name:     roleName + "-ensemble-adaptive",
			Provider: "ensemble",
			Model:    string(ens.Strategy),
			Client:   c,
		}

	default:
		return nil
	}
}

func (r *ResilientLLMRouter) resolveNamedClients(refs []config.AgentProviderRef) []ensemble.NamedClient {
	var list []ensemble.NamedClient
	for _, ref := range refs {
		count := ref.GetCount()
		for i := 0; i < count; i++ {
			client := r.resolveClientFromRef(ref)
			if client != nil {
				name := ref.Name
				if count > 1 {
					name = fmt.Sprintf("%s-%d", ref.Name, i+1)
				}
				list = append(list, ensemble.NamedClient{
					Name:      name,
					Client:    client,
					MaxTokens: ref.MaxTokens,
				})
			}
		}
	}
	return list
}

func (r *ResilientLLMRouter) resolveClientFromRef(ref config.AgentProviderRef) domain.LLMClient {
	var spec config.ProviderSpec
	var found bool

	if ref.Name != "" {
		spec, found = r.namedProviders[ref.Name]
	} else if ref.Provider != "" {
		spec = config.ProviderSpec{
			Name:     ref.Provider,
			Provider: ref.Provider,
		}
		found = true
	}

	if !found || spec.Provider == "" {
		return nil
	}

	overrideSpec := spec
	if ref.Model != "" {
		overrideSpec.Model = ref.Model
	}
	if ref.MaxTokens != nil {
		overrideSpec.MaxTokens = *ref.MaxTokens
	}
	if ref.Temperature != nil {
		overrideSpec.Temperature = *ref.Temperature
	}
	if ref.EnableThinking != nil {
		overrideSpec.EnableThinking = ref.EnableThinking
	}
	if ref.ThinkingBudget != nil {
		overrideSpec.ThinkingBudget = ref.ThinkingBudget
	}
	if len(ref.ExtraParams) > 0 {
		if overrideSpec.ExtraParams == nil {
			overrideSpec.ExtraParams = make(map[string]string)
		}
		for k, v := range ref.ExtraParams {
			overrideSpec.ExtraParams[k] = v
		}
	}

	return r.buildClientForSpec(overrideSpec, overrideSpec.Model)
}

func (r *ResilientLLMRouter) getRoleSetting(roleName string) config.RoleSetting {
	if r.cfg != nil {
		var agentRole config.AgentRoleConfig
		switch strings.ToLower(roleName) {
		case "orchestrator":
			agentRole = r.cfg.Agents.Orchestrator
		case "product_manager", "productmanager":
			agentRole = r.cfg.Agents.ProductManager
		case "planner":
			agentRole = r.cfg.Agents.Planner
		case "generator", "generators":
			agentRole = r.cfg.Agents.Generators
		case "tester", "testers":
			agentRole = r.cfg.Agents.Testers
		case "qa":
			qa := r.cfg.Agents.QA
			if len(qa.Providers) > 0 || qa.Ensemble.IsEnabled() {
				return config.RoleSetting{
					Model:       qa.Model,
					Temperature: qa.Temperature,
					Profile:     qa.Profile,
					Providers:   qa.Providers,
					MaxTokens:   qa.MaxTokens,
					Ensemble:    qa.Ensemble,
				}
			}
		case "auditor":
			auditor := r.cfg.Agents.Auditor
			if len(auditor.Providers) > 0 || auditor.Ensemble.IsEnabled() {
				return config.RoleSetting{
					Model:       auditor.Model,
					Temperature: auditor.Temperature,
					Profile:     auditor.Profile,
					Providers:   auditor.Providers,
					MaxTokens:   auditor.MaxTokens,
					Ensemble:    auditor.Ensemble,
				}
			}
			qa := r.cfg.Agents.QA
			if len(qa.Providers) > 0 || qa.Ensemble.IsEnabled() {
				return config.RoleSetting{
					Model:       qa.Model,
					Temperature: qa.Temperature,
					Profile:     qa.Profile,
					Providers:   qa.Providers,
					MaxTokens:   qa.MaxTokens,
					Ensemble:    qa.Ensemble,
				}
			}
			pm := r.cfg.Agents.ProductManager
			if len(pm.Providers) > 0 || pm.Ensemble.IsEnabled() {
				return config.RoleSetting{
					Model:       pm.Model,
					Temperature: pm.Temperature,
					Profile:     pm.Profile,
					Providers:   pm.Providers,
					MaxTokens:   pm.MaxTokens,
					Ensemble:    pm.Ensemble,
				}
			}
		case "unblocker":
			agentRole = r.cfg.Agents.Unblocker
		case "last_resort", "lastresort":
			lr := r.cfg.Agents.LastResort
			if len(lr.Providers) > 0 {
				return config.RoleSetting{Model: lr.Model, Temperature: lr.Temperature, Profile: lr.Profile, Providers: lr.Providers}
			}
		}
		if len(agentRole.Providers) > 0 || agentRole.Ensemble.IsEnabled() {
			return config.RoleSetting{
				Model:       agentRole.Model,
				Temperature: agentRole.Temperature,
				Profile:     agentRole.Profile,
				Providers:   agentRole.Providers,
				MaxTokens:   agentRole.MaxTokens,
				Ensemble:    agentRole.Ensemble,
			}
		}
	}

	switch strings.ToLower(roleName) {
	case "orchestrator":
		return r.roles.Orchestrator
	case "product_manager", "productmanager":
		return config.RoleSetting{}
	case "planner":
		return r.roles.Planner
	case "generator", "generators":
		return r.roles.Generator
	case "tester", "testers":
		return r.roles.Tester
	case "qa":
		return r.roles.QA
	case "auditor":
		if r.roles.QA.Model != "" || len(r.roles.QA.Providers) > 0 || r.roles.QA.Ensemble.IsEnabled() {
			return r.roles.QA
		}
		return r.roles.Orchestrator
	case "unblocker":
		return r.roles.Unblocker
	case "last_resort", "lastresort":
		return r.roles.LastResort
	default:
		return config.RoleSetting{}
	}
}
