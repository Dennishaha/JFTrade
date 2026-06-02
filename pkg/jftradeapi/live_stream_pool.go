package jftradeapi

import "sync"

type liveStreamPool struct {
	mu      sync.Mutex
	clients int
}

func (p *liveStreamPool) tryAcquire(limit int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.clients >= limit {
		return false
	}
	p.clients++
	return true
}

func (p *liveStreamPool) release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.clients > 0 {
		p.clients--
	}
}

func (p *liveStreamPool) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clients
}
