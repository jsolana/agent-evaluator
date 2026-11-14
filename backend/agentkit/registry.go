package agentkit

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Registry maps adapter names to their factories. Adapters register themselves
// in the Default registry from an init function (like database/sql drivers);
// tests and embedders can use their own with NewRegistry.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry { return &Registry{factories: map[string]Factory{}} }

// Default is the registry used by the package-level Register.
var Default = NewRegistry()

// Register makes an adapter available in the Default registry under name.
// It panics if the name is empty or taken, or the factory is nil: those are
// programming errors that must surface at start-up.
func Register(name string, f Factory) { Default.Register(name, f) }

func (r *Registry) Register(name string, f Factory) {
	if name == "" || f == nil {
		panic("agentkit: Register needs a name and a factory")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.factories[name]; dup {
		panic("agentkit: Register called twice for " + name)
	}
	r.factories[name] = f
}

// Names lists the registered adapters in alphabetical order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for n := range r.factories {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// Build creates the named adapter. The error of an unknown name lists the
// registered ones. A nil Traces is replaced by NoTraces.
func (r *Registry) Build(name string, env Env) (Plugin, error) {
	r.mu.RLock()
	f, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return Plugin{}, fmt.Errorf("AGENT must be one of %s, got %q", strings.Join(r.Names(), ", "), name)
	}
	p, err := f(env)
	if err != nil {
		return Plugin{}, fmt.Errorf("agent %q: %w", name, err)
	}
	if p.Agent == nil {
		return Plugin{}, fmt.Errorf("agent %q: factory returned no Agent", name)
	}
	if p.Traces == nil {
		p.Traces = NoTraces{}
	}
	return p, nil
}
