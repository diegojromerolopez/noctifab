package config

// DynamicTurnsConfig configures dynamic turn loop ceiling extensions on active remediation.
type DynamicTurnsConfig struct {
	Enabled      bool `yaml:"enabled,omitempty"`
	MaxExtension int  `yaml:"max_extension,omitempty"`
}

// GetMaxExtension returns the maximum turn extension (default 3).
func (d DynamicTurnsConfig) GetMaxExtension() int {
	if d.MaxExtension > 0 {
		return d.MaxExtension
	}
	return 3
}

// GetMaxTurnExtensions returns the maximum number of dynamic remediation turn extensions (default 3).
func (a AgentsConfig) GetMaxTurnExtensions() int {
	if a.DynamicTurns.MaxExtension > 0 {
		return a.DynamicTurns.MaxExtension
	}
	if a.MaxTurnExtensions > 0 {
		return a.MaxTurnExtensions
	}
	return 3
}
