package agent

import (
	"fmt"
	"sort"
	"sync"
)

// Persona defines a switchable personality with optional model and tool overrides.
type Persona struct {
	Name         string
	SystemPrompt string
	Model        string   // optional model override
	AllowedTools []string // optional tool filter
	Description  string
}

// PersonaManager tracks available personas and the active persona per session.
type PersonaManager struct {
	mu            sync.RWMutex
	personas      map[string]*Persona
	activePersona sync.Map // sessionKey -> persona name
}

// NewPersonaManager creates a PersonaManager from the given persona definitions.
func NewPersonaManager(personas map[string]*Persona) *PersonaManager {
	pm := &PersonaManager{
		personas: make(map[string]*Persona, len(personas)),
	}
	for k, v := range personas {
		pm.personas[k] = v
	}
	return pm
}

// Switch activates a named persona for the given session. Returns an error if
// the persona name is not registered.
func (pm *PersonaManager) Switch(sessionKey, personaName string) error {
	pm.mu.RLock()
	_, ok := pm.personas[personaName]
	pm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("persona %q not found", personaName)
	}
	pm.activePersona.Store(sessionKey, personaName)
	return nil
}

// GetActive returns the active persona for a session, or nil when no persona
// override is set.
func (pm *PersonaManager) GetActive(sessionKey string) *Persona {
	v, ok := pm.activePersona.Load(sessionKey)
	if !ok {
		return nil
	}
	name, _ := v.(string)
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.personas[name]
}

// Clear removes any persona override for the given session, reverting to the
// default behaviour.
func (pm *PersonaManager) Clear(sessionKey string) {
	pm.activePersona.Delete(sessionKey)
}

// Register adds (or replaces) a persona in the manager.
func (pm *PersonaManager) Register(name string, persona *Persona) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.personas[name] = persona
}

// Unregister removes a persona from the manager. Active sessions using the
// persona are not affected until the next GetActive call.
func (pm *PersonaManager) Unregister(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.personas, name)
}

// List returns the names of all registered personas in sorted order.
func (pm *PersonaManager) List() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	names := make([]string, 0, len(pm.personas))
	for k := range pm.personas {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
