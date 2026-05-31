package broker

// Registry manages the set of available broker adapters.
// It is safe for concurrent use after initial registration.
type Registry struct {
	brokers map[string]Broker
}

// NewRegistry creates an empty broker registry.
func NewRegistry() *Registry {
	return &Registry{brokers: make(map[string]Broker)}
}

// Register adds a broker to the registry. Panics if a broker with the same ID is already registered.
func (r *Registry) Register(b Broker) {
	if _, exists := r.brokers[b.ID()]; exists {
		panic("broker: Register called twice for " + b.ID())
	}
	r.brokers[b.ID()] = b
}

// Lookup returns the broker with the given ID, or nil.
func (r *Registry) Lookup(id string) Broker {
	return r.brokers[id]
}

// All returns all registered brokers.
func (r *Registry) All() []Broker {
	result := make([]Broker, 0, len(r.brokers))
	for _, b := range r.brokers {
		result = append(result, b)
	}
	return result
}

// IDs returns all registered broker IDs.
func (r *Registry) IDs() []string {
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
	for _, b := range r.brokers {
		return b
	}
	return nil
}
