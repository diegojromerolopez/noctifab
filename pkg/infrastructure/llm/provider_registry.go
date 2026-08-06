package llm

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// KeywordTier associates keyword matches with a capacity tier and base score.
type KeywordTier struct {
	Keywords []string
	Score    int
	TierName string
}

// ParserConfig defines the composable rules for building a ModelParser.
type ParserConfig struct {
	RequiredPrefix    string
	ExcludedKeywords  []string
	DefaultVersion    float64
	VersionRegexp     string
	Tiers             []KeywordTier
	SizeWeights       map[string]int
	ContextBonus      bool
	VersionMultiplier int
}

// ModelParser is a function signature that parses a model name into ProviderModelInfo.
type ModelParser func(name string) (*ProviderModelInfo, bool)

// StandardSizeWeights defines parameter size ranks shared across open-weights model providers.
var StandardSizeWeights = map[string]int{
	"405b": 500,
	"90b":  400,
	"72b":  400,
	"70b":  400,
	"34b":  300,
	"32b":  300,
	"27b":  300,
	"14b":  300,
	"13b":  300,
	"11b":  200,
	"8b":   200,
	"7b":   200,
	"3b":   100,
	"1b":   100,
}

// NewModelParser creates a ModelParser through declarative composition of rules.
func NewModelParser(cfg ParserConfig) ModelParser {
	var vRe *regexp.Regexp
	if cfg.VersionRegexp != "" {
		vRe = regexp.MustCompile(cfg.VersionRegexp)
	}

	vMult := cfg.VersionMultiplier
	if vMult <= 0 {
		vMult = 10
	}

	return func(name string) (*ProviderModelInfo, bool) {
		norm := strings.ToLower(name)
		if cfg.RequiredPrefix != "" && !strings.Contains(norm, cfg.RequiredPrefix) {
			return nil, false
		}
		for _, ex := range cfg.ExcludedKeywords {
			if strings.Contains(norm, ex) {
				return nil, false
			}
		}

		var tier string
		var baseScore int

		// 1. Check Keyword Tiers
		if len(cfg.Tiers) > 0 {
			matched := false
			for _, t := range cfg.Tiers {
				for _, kw := range t.Keywords {
					if strings.Contains(norm, kw) {
						tier = t.TierName
						baseScore = t.Score
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				tier = "standard"
				baseScore = 10
			}
		}

		// 2. Check Parameter Size Weights
		if len(cfg.SizeWeights) > 0 {
			matchedSize := false
			for sz, sc := range cfg.SizeWeights {
				if strings.Contains(norm, sz) {
					if tier == "" || tier == "standard" {
						tier = sz
					}
					if baseScore == 0 || baseScore == 10 {
						baseScore = sc
					}
					matchedSize = true
					break
				}
			}
			if !matchedSize && baseScore == 0 {
				baseScore = 200
				tier = "default"
			}
		}

		// 3. Extract Version
		version := cfg.DefaultVersion
		hasVersion := false
		if vRe != nil {
			if matches := vRe.FindStringSubmatch(norm); len(matches) > 1 {
				vStr := strings.ReplaceAll(matches[1], "-", ".")
				if v, err := strconv.ParseFloat(vStr, 64); err == nil {
					version = v
					hasVersion = true
				}
			}
		}

		rank := baseScore
		if cfg.VersionRegexp != "" && (hasVersion || cfg.DefaultVersion > 0) {
			rank += int(version * float64(vMult))
		}

		// 4. Context Window Bonus
		if cfg.ContextBonus {
			if strings.Contains(norm, "128k") {
				rank += 3
			} else if strings.Contains(norm, "32k") {
				rank += 2
			} else if strings.Contains(norm, "8k") {
				rank += 1
			}
		}

		return &ProviderModelInfo{Name: name, Version: version, Tier: tier, Rank: rank}, true
	}
}

// ProviderSpec encapsulates the metadata, base URL, authentication environment keys,
// model parsing logic, and client constructor for an LLM provider.
type ProviderSpec struct {
	Name           string
	BaseURL        string
	EnvKeys        []string
	ParseModelFunc func(name string) (*ProviderModelInfo, bool)
	Protocol       string // "openai", "gemini", "anthropic"
	NewClientFunc  func(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]*ProviderSpec)
)

// RegisterProvider adds or replaces a ProviderSpec in the global registry.
func RegisterProvider(spec *ProviderSpec) {
	registryMu.Lock()
	defer registryMu.Unlock()
	key := strings.ToLower(spec.Name)
	registry[key] = spec
}

// GetProviderSpec retrieves a ProviderSpec by provider name (case-insensitive).
func GetProviderSpec(provider string) (*ProviderSpec, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	key := strings.ToLower(provider)
	spec, ok := registry[key]
	return spec, ok
}

// RegistrySnapshot returns a copy of the provider registry keyed by
// lower-cased provider name. It is primarily used by tests to verify that
// configuration validation stays in sync with the registered providers.
func RegistrySnapshot() map[string]*ProviderSpec {
	registryMu.RLock()
	defer registryMu.RUnlock()
	snapshot := make(map[string]*ProviderSpec, len(registry))
	for k, v := range registry {
		snapshot[k] = v
	}
	return snapshot
}
