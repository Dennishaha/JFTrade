package futu

type subscriptionRegistry struct {
	basicQot      map[string]struct{}
	basicQotPush  map[string]struct{}
	kline         map[string]struct{}
	orderBook     map[string]struct{}
	orderBookPush map[string]struct{}
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
	if r.orderBook == nil {
		r.orderBook = map[string]struct{}{}
	}
	if r.orderBookPush == nil {
		r.orderBookPush = map[string]struct{}{}
	}
}

func (r *subscriptionRegistry) reset() {
	r.basicQot = map[string]struct{}{}
	r.basicQotPush = map[string]struct{}{}
	r.kline = map[string]struct{}{}
	r.orderBook = map[string]struct{}{}
	r.orderBookPush = map[string]struct{}{}
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

func (r *subscriptionRegistry) unmarkBasicQot(key string) {
	r.ensure()
	delete(r.basicQot, key)
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

func (r *subscriptionRegistry) unmarkBasicQotPush(key string) {
	r.ensure()
	delete(r.basicQotPush, key)
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

func (r *subscriptionRegistry) unmarkKLine(key string) {
	r.ensure()
	delete(r.kline, key)
}

func (r *subscriptionRegistry) hasOrderBook(key string) bool {
	r.ensure()
	_, exists := r.orderBook[key]
	return exists
}

func (r *subscriptionRegistry) markOrderBook(key string) {
	r.ensure()
	r.orderBook[key] = struct{}{}
}

func (r *subscriptionRegistry) unmarkOrderBook(key string) {
	r.ensure()
	delete(r.orderBook, key)
}

func (r *subscriptionRegistry) hasOrderBookPush(key string) bool {
	r.ensure()
	_, exists := r.orderBookPush[key]
	return exists
}

func (r *subscriptionRegistry) markOrderBookPush(key string) {
	r.ensure()
	r.orderBookPush[key] = struct{}{}
}

func (r *subscriptionRegistry) unmarkOrderBookPush(key string) {
	r.ensure()
	delete(r.orderBookPush, key)
}
