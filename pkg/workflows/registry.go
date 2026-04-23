package workflows

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Registry holds named workflows. The zero value is usable but callers
// typically use NewRegistry for discoverability.
type Registry struct {
	mu  sync.RWMutex
	all map[string]*Workflow
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{all: make(map[string]*Workflow)}
}

// Register adds a workflow. A re-registration with the same name replaces the
// prior entry; this is intentional so tests can override production workflows.
// Returns the workflow's validation error if any.
func (r *Registry) Register(w *Workflow) error {
	if err := w.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.all == nil {
		r.all = make(map[string]*Workflow)
	}
	r.all[w.Name] = w
	return nil
}

// MustRegister panics on validation error; use at package init for built-in
// workflows where misconfiguration is a bug, not runtime state.
func (r *Registry) MustRegister(w *Workflow) {
	if err := r.Register(w); err != nil {
		panic(fmt.Sprintf("workflows: MustRegister failed: %v", err))
	}
}

// Get returns a workflow by name. The returned error is non-nil when the name
// is unknown so callers can distinguish "workflow missing" from "workflow
// returned an empty structure".
func (r *Registry) Get(name string) (*Workflow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.all[name]
	if !ok {
		return nil, fmt.Errorf("workflows: %q not registered", name)
	}
	return w, nil
}

// Has reports whether a workflow is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.all[name]
	return ok
}

// Names returns the registered workflow names in lexical order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.all))
	for n := range r.all {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Default is the package-level registry. Production code should build its
// own Registry and pass it around explicitly for testability, but for init-
// time registration of built-in workflows Default is convenient.
var Default = NewRegistry()

// ErrNotFound is returned by Get when a workflow name is not registered.
var ErrNotFound = errors.New("workflows: not registered")
