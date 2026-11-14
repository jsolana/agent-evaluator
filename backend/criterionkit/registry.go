package criterionkit

import (
	"slices"
	"sync"
)

// Registry maps criterion types to their factories. Criteria register
// themselves in Default from an init function; tests and embedders can use
// their own with NewRegistry.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry { return &Registry{factories: map[string]Factory{}} }

// Default is the registry used by the package-level Register.
var Default = NewRegistry()

// Register makes a criterion available to suites under typ. It panics if typ is
// empty or taken, or the factory is nil: programming errors that must surface at
// start-up. Names that collide with a built-in criterion are rejected when the
// evaluator starts. Prefixing with your organization (myorg.no_pii) avoids
// clashes between packages.
func Register(typ string, f Factory) { Default.Register(typ, f) }

func (r *Registry) Register(typ string, f Factory) {
	if typ == "" || f == nil {
		panic("criterionkit: Register needs a type and a factory")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.factories[typ]; dup {
		panic("criterionkit: Register called twice for " + typ)
	}
	r.factories[typ] = f
}

// Names lists the registered types in alphabetical order.
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

// Lookup returns the factory registered under typ.
func (r *Registry) Lookup(typ string) (Factory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[typ]
	return f, ok
}
