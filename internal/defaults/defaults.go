package defaults

import (
	_ "embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed defaults.yaml
var embeddedYAML []byte

// Registry is the versioned bang-for-buck provider catalog.
type Registry struct {
	Version     int                 `yaml:"version"`
	Updated     string              `yaml:"updated"`
	RoleOrders  map[string][]string `yaml:"role_orders"`
	Providers   map[string]Provider   `yaml:"providers"`
}

// Provider describes how to launch one coding-agent CLI optimally.
type Provider struct {
	Binary        string            `yaml:"binary"`
	Display       string            `yaml:"display"`
	HeadroomWrap  string            `yaml:"headroom_wrap"`
	HeadroomFlags []string          `yaml:"headroom_flags"`
	Env           map[string]string `yaml:"env"`
	Passthrough   []string          `yaml:"passthrough"`
	Summary       string            `yaml:"summary"`
	Why           string            `yaml:"why"`
	Roles         []string          `yaml:"roles"`
	RolePriority  map[string]int    `yaml:"role_priority"`
}

// Load parses the embedded defaults.yaml.
func Load() (*Registry, error) {
	return Parse(embeddedYAML)
}

// Parse unmarshals a defaults YAML document.
func Parse(data []byte) (*Registry, error) {
	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse defaults: %w", err)
	}
	if reg.Providers == nil {
		return nil, fmt.Errorf("defaults: no providers")
	}
	return &reg, nil
}

// Names returns provider keys in stable order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.Providers))
	for name := range r.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.Providers[name]
	return p, ok
}

// RoleOrder returns the ordered provider list for a role, or nil when unset.
func (r *Registry) RoleOrder(role string) []string {
	if r == nil || r.RoleOrders == nil {
		return nil
	}
	order, ok := r.RoleOrders[role]
	if !ok || len(order) == 0 {
		return nil
	}
	out := make([]string, len(order))
	copy(out, order)
	return out
}
