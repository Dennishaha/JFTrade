package jftradeapi

import "sync"

type liveSocketPool struct {
	mu      sync.Mutex
	clients int
}

func (p *liveSocketPool) tryAcquire(limit int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.clients >= limit {
		return false
	}
	p.clients++
	return true
}

func (p *liveSocketPool) release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.clients > 0 {
		p.clients--
	}
}

func (p *liveSocketPool) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clients
}
