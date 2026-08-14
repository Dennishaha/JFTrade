package completionreview

import "sync"

type memo struct {
	done    chan struct{}
	outcome Outcome
	applied map[any]struct{}
}

type Coordinator struct {
	mu    sync.Mutex
	memos map[string]*memo
}

func NewCoordinator() *Coordinator {
	return &Coordinator{memos: map[string]*memo{}}
}

func (c *Coordinator) Once(runID string, review func() Outcome) Outcome {
	c.mu.Lock()
	if current := c.memos[runID]; current != nil {
		done := current.done
		c.mu.Unlock()
		<-done
		return current.outcome
	}
	current := &memo{done: make(chan struct{}), applied: map[any]struct{}{}}
	c.memos[runID] = current
	c.mu.Unlock()

	outcome := review()
	c.mu.Lock()
	current.outcome = outcome
	close(current.done)
	c.mu.Unlock()
	return outcome
}

func (c *Coordinator) MarkApplied(runID string, target any) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.memos[runID]
	if current == nil {
		return false
	}
	if _, ok := current.applied[target]; ok {
		return false
	}
	current.applied[target] = struct{}{}
	return true
}

func (c *Coordinator) Clear(runID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.memos, runID)
	c.mu.Unlock()
}
