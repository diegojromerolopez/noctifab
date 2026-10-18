package config

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// providerSpecAlias has the same fields as ProviderSpec but no UnmarshalYAML
// method. This prevents infinite recursion during decode.
type providerSpecAlias ProviderSpec

// providerSpecKnownKeys holds the YAML keys that ProviderSpec accepts.
var providerSpecKnownKeys = func() map[string]bool {
	keys := make(map[string]bool)
	t := reflect.TypeOf(providerSpecAlias{})
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0]
		if name != "" && name != "-" {
			keys[name] = true
		}
	}
	return keys
}()

// UnmarshalYAML decodes a provider entry and records which keys the user
// wrote. This lets an explicit zero value (temperature: 0, max_retries: 0)
// override a non-zero global llm.* value. Unknown keys are rejected, because
// node.Decode does not inherit the strict KnownFields mode of the parent decoder.
func (p *ProviderSpec) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("llm provider entry must be a mapping, got %v at line %d", value.Tag, value.Line)
	}
	set := make(map[string]bool, len(value.Content)/2)
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i].Value
		if !providerSpecKnownKeys[key] {
			return fmt.Errorf("line %d: field %s not found in type config.ProviderSpec", value.Content[i].Line, key)
		}
		set[key] = true
	}
	var alias providerSpecAlias
	if err := value.Decode(&alias); err != nil {
		return fmt.Errorf("decoding llm provider entry: %w", err)
	}
	*p = ProviderSpec(alias)
	p.explicitKeys = set
	return nil
}

// IsExplicit reports whether the YAML entry for this provider wrote the key.
func (p ProviderSpec) IsExplicit(key string) bool {
	return p.explicitKeys[key]
}
