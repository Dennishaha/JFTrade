package adk

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	adkplatform "google.golang.org/adk/v2/platform"
)

func TestGoogleADKTaskRunnerBoundsFanOutAndRunsEveryTask(t *testing.T) {
	const taskCount = 21
	started := make(chan struct{}, taskCount)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseTasks := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseTasks()
	var active atomic.Int32
	var peak atomic.Int32
	calls := make([]atomic.Int32, taskCount)
	tasks := make([]func(context.Context), taskCount)
	for index := range tasks {
		tasks[index] = func(context.Context) {
			current := active.Add(1)
			updateAtomicPeak(&peak, current)
			started <- struct{}{}
			<-release
			calls[index].Add(1)
			active.Add(-1)
		}
	}

	done := make(chan struct{})
	go func() {
		adkplatform.RunTasks(googleADKTaskRunnerContext(context.Background()), tasks)
		close(done)
	}()
	for range maxGoogleADKParallelTasks {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("bounded ADK task runner did not start the expected workers")
		}
	}
	select {
	case <-started:
		t.Fatal("bounded ADK task runner exceeded its concurrency limit")
	default:
	}
	releaseTasks()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("bounded ADK task runner did not finish")
	}
	if got := peak.Load(); got != maxGoogleADKParallelTasks {
		t.Fatalf("peak concurrency = %d, want %d", got, maxGoogleADKParallelTasks)
	}
	for index := range calls {
		if got := calls[index].Load(); got != 1 {
			t.Fatalf("task %d calls = %d, want 1", index, got)
		}
	}
}

func updateAtomicPeak(peak *atomic.Int32, current int32) {
	for {
		observed := peak.Load()
		if current <= observed || peak.CompareAndSwap(observed, current) {
			return
		}
	}
}

func TestGoogleADKTaskRunnerRunsCancelledTasksWithOriginalContext(t *testing.T) {
	type contextKey string
	const key contextKey = "task-runner"
	base := context.WithValue(context.Background(), key, "jftrade")
	ctx, cancel := context.WithCancel(base)
	cancel()

	const taskCount = 21
	var calls atomic.Int32
	var cancelled atomic.Int32
	var values atomic.Int32
	tasks := make([]func(context.Context), taskCount)
	for index := range tasks {
		tasks[index] = func(taskCtx context.Context) {
			calls.Add(1)
			if taskCtx.Err() == context.Canceled {
				cancelled.Add(1)
			}
			if taskCtx.Value(key) == "jftrade" {
				values.Add(1)
			}
		}
	}

	done := make(chan struct{})
	go func() {
		boundedGoogleADKTaskRunner(0)(ctx, tasks)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled ADK task batch blocked")
	}
	if got := calls.Load(); got != taskCount {
		t.Fatalf("task calls = %d, want %d", got, taskCount)
	}
	if got := cancelled.Load(); got != taskCount {
		t.Fatalf("cancelled contexts = %d, want %d", got, taskCount)
	}
	if got := values.Load(); got != taskCount {
		t.Fatalf("context values = %d, want %d", got, taskCount)
	}
}
