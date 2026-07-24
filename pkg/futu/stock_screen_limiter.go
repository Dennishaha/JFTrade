package futu

import (
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

const (
	researchScreenLimit  = 10
	researchScreenWindow = 30 * time.Second
)

type researchScreenLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	request []time.Time
}

func newResearchScreenLimiter() *researchScreenLimiter {
	return &researchScreenLimiter{now: time.Now}
}

// retryAfter records an accepted attempt and returns zero, or leaves the
// window unchanged and reports when another real OpenD call may be attempted.
func (l *researchScreenLimiter) retryAfter() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-researchScreenWindow)
	firstCurrent := 0
	for firstCurrent < len(l.request) && !l.request[firstCurrent].After(cutoff) {
		firstCurrent++
	}
	if firstCurrent > 0 {
		l.request = append([]time.Time(nil), l.request[firstCurrent:]...)
	}
	if len(l.request) >= researchScreenLimit {
		retryAfter := l.request[0].Add(researchScreenWindow).Sub(now)
		if retryAfter <= 0 {
			retryAfter = time.Millisecond
		}
		return retryAfter
	}
	l.request = append(l.request, now)
	return 0
}

func (a *futuAdapter) researchScreenRetryAfter(client *opend.Client) time.Duration {
	a.researchScreenLimiterMu.Lock()
	if a.researchScreenLimiters == nil {
		a.researchScreenLimiters = make(map[*opend.Client]*researchScreenLimiter)
	}
	limiter := a.researchScreenLimiters[client]
	if limiter == nil {
		limiter = newResearchScreenLimiter()
		a.researchScreenLimiters[client] = limiter
	}
	a.researchScreenLimiterMu.Unlock()
	return limiter.retryAfter()
}
