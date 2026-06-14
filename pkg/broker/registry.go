package broker

import "sync"

// Registry manages the set of available broker adapters.
// It is safe for concurrent use after initial registration.
type Registry struct {
	mu      sync.RWMutex
	brokers map[string]Broker
}

// NewRegistry creates an empty broker registry.
func NewRegistry() *Registry {
	return &Registry{brokers: make(map[string]Broker)}
}

// Register adds a broker to the registry. Panics if a broker with the same ID is already registered.
func (r *Registry) Register(b Broker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.brokers[b.ID()]; exists {
		panic("broker: Register called twice for " + b.ID())
	}
	r.brokers[b.ID()] = b
}

// Replace installs or replaces a broker adapter with the same ID.
func (r *Registry) Replace(b Broker) {
	r.mu.Lock()
	r.brokers[b.ID()] = b
	r.mu.Unlock()
}

// Lookup returns the broker with the given ID, or nil.
func (r *Registry) Lookup(id string) Broker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.brokers[id]
}

// All returns all registered brokers.
func (r *Registry) All() []Broker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Broker, 0, len(r.brokers))
	for _, b := range r.brokers {
		result = append(result, b)
	}
	return result
}

// IDs returns all registered broker IDs.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, 0, len(r.brokers))
	for id := range r.brokers {
		result = append(result, id)
	}
	return result
}

// ActiveBroker returns the first registered broker, or nil if none are registered.
// This is a convenience for the current single-broker setup.
// When multi-broker support is fully wired, callers should use Lookup instead.
func (r *Registry) ActiveBroker() Broker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, b := range r.brokers {
		return b
	}
	return nil
}
