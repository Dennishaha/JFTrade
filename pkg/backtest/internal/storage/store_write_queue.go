package storage

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/c9s/bbgo/pkg/types"
)

var klineAccessQueues sync.Map

type klineAccessMode uint8

const (
	klineAccessRead klineAccessMode = iota
	klineAccessWrite
)

type klineAccessRequest struct {
	mode klineAccessMode
	fn   func() error
	done bool
	err  error
}

type klineAccessQueue struct {
	mu            sync.Mutex
	cond          *sync.Cond
	activeReaders int
	activeWriter  bool
	pending       []*klineAccessRequest
}

func klineAccessQueueForPath(path string) *klineAccessQueue {
	key := klineAccessQueueKey(path)
	if existing, ok := klineAccessQueues.Load(key); ok {
		return jftradeCheckedTypeAssertion[*klineAccessQueue](existing)
	}
	queue := newKLineAccessQueue()
	actual, _ := klineAccessQueues.LoadOrStore(key, queue)
	return jftradeCheckedTypeAssertion[*klineAccessQueue](actual)
}

func klineAccessQueueKey(path string) string {
	key := strings.TrimSpace(path)
	if key == "" {
		return path
	}
	if strings.HasPrefix(strings.ToLower(key), "file:") {
		return key
	}
	if absolute, err := filepath.Abs(key); err == nil {
		key = absolute
	}
	return filepath.Clean(key)
}

func newKLineAccessQueue() *klineAccessQueue {
	queue := &klineAccessQueue{}
	queue.cond = sync.NewCond(&queue.mu)
	return queue
}

func (q *klineAccessQueue) enqueueRead(fn func() error) error {
	return q.enqueue(klineAccessRead, fn)
}

func (q *klineAccessQueue) enqueueWrite(fn func() error) error {
	return q.enqueue(klineAccessWrite, fn)
}

func (q *klineAccessQueue) enqueue(mode klineAccessMode, fn func() error) error {
	if fn == nil {
		return nil
	}
	request := &klineAccessRequest{mode: mode, fn: fn}

	q.mu.Lock()
	q.pending = append(q.pending, request)
	q.startReadyLocked()
	for !request.done {
		q.cond.Wait()
	}
	err := request.err
	q.mu.Unlock()
	return err
}

func (q *klineAccessQueue) startReadyLocked() {
	if q.activeWriter {
		return
	}
	if q.activeReaders > 0 {
		q.startReadBatchLocked()
		return
	}
	if len(q.pending) == 0 {
		return
	}
	if q.pending[0].mode == klineAccessWrite {
		request := q.pending[0]
		q.pending = q.pending[1:]
		q.activeWriter = true
		go q.runWrite(request)
		return
	}
	q.startReadBatchLocked()
}

func (q *klineAccessQueue) startReadBatchLocked() {
	for len(q.pending) > 0 && q.pending[0].mode == klineAccessRead {
		request := q.pending[0]
		q.pending = q.pending[1:]
		q.activeReaders++
		go q.runRead(request)
	}
}

func (q *klineAccessQueue) runRead(request *klineAccessRequest) {
	err := request.fn()

	q.mu.Lock()
	request.err = err
	request.done = true
	q.activeReaders--
	q.startReadyLocked()
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *klineAccessQueue) runWrite(request *klineAccessRequest) {
	err := request.fn()

	q.mu.Lock()
	request.err = err
	request.done = true
	q.activeWriter = false
	q.startReadyLocked()
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *klineAccessQueue) enqueueKLines(store *FutuKLineStore, klines []types.KLine, rehabType string) error {
	if store == nil || len(klines) == 0 {
		return nil
	}
	copied := append([]types.KLine(nil), klines...)
	return q.enqueueWrite(func() error {
		return store.insertKLinesQueued(copied, rehabType)
	})
}
