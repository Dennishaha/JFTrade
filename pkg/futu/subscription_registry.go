package futu

type subscriptionRegistry struct {
	basicQot     map[string]struct{}
	basicQotPush map[string]struct{}
	kline        map[string]struct{}
}

func newSubscriptionRegistry() subscriptionRegistry {
	registry := subscriptionRegistry{}
	registry.reset()
	return registry
}

func (r *subscriptionRegistry) ensure() {
	if r.basicQot == nil {
		r.basicQot = map[string]struct{}{}
	}
	if r.basicQotPush == nil {
		r.basicQotPush = map[string]struct{}{}
	}
	if r.kline == nil {
		r.kline = map[string]struct{}{}
	}
}

func (r *subscriptionRegistry) reset() {
	r.basicQot = map[string]struct{}{}
	r.basicQotPush = map[string]struct{}{}
	r.kline = map[string]struct{}{}
}

func (r *subscriptionRegistry) hasBasicQot(key string) bool {
	r.ensure()
	_, exists := r.basicQot[key]
	return exists
}

func (r *subscriptionRegistry) markBasicQot(key string) {
	r.ensure()
	r.basicQot[key] = struct{}{}
}

func (r *subscriptionRegistry) hasBasicQotPush(key string) bool {
	r.ensure()
	_, exists := r.basicQotPush[key]
	return exists
}

func (r *subscriptionRegistry) markBasicQotPush(key string) {
	r.ensure()
	r.basicQotPush[key] = struct{}{}
}

func (r *subscriptionRegistry) hasKLine(key string) bool {
	r.ensure()
	_, exists := r.kline[key]
	return exists
}

func (r *subscriptionRegistry) markKLine(key string) {
	r.ensure()
	r.kline[key] = struct{}{}
}
