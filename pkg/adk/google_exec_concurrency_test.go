package adk

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGoogleADKExecutionSerializesConcurrentToolCallbacks(t *testing.T) {
	const fragmentCount = 64

	var callbacksActive atomic.Int32
	var callbacksOverlapped atomic.Bool
	var emittedMu sync.Mutex
	emittedReplies := make([]string, 0, 1)
	execution := &googleADKExecution{
		runID: "run-concurrent-callbacks",
		onDelta: func(delta ChatDelta) error {
			if callbacksActive.Add(1) != 1 {
				callbacksOverlapped.Store(true)
			}
			defer callbacksActive.Add(-1)
			time.Sleep(time.Millisecond)
			if delta.Reply != "" {
				emittedMu.Lock()
				emittedReplies = append(emittedReplies, delta.Reply)
				emittedMu.Unlock()
			}
			return nil
		},
	}
	first := execution.ensureCall("function-first", ToolDescriptor{Name: "first"}, nil)
	second := execution.ensureCall("function-second", ToolDescriptor{Name: "second"}, nil)
	firstID, secondID := first.ID, second.ID

	var appendWG sync.WaitGroup
	appendErrs := make(chan error, fragmentCount)
	for index := 0; index < fragmentCount; index++ {
		fragment := fmt.Sprintf("fragment-%02d|", index)
		appendWG.Add(1)
		go func() {
			defer appendWG.Done()
			if err := execution.appendVisibleTextForRun(execution.runID, fragment, ""); err != nil {
				appendErrs <- err
			}
		}()
	}
	appendWG.Wait()
	close(appendErrs)
	for err := range appendErrs {
		t.Fatalf("append visible text: %v", err)
	}

	var finishWG sync.WaitGroup
	for _, callID := range []string{firstID, secondID} {
		finishWG.Add(1)
		go func() {
			defer finishWG.Done()
			execution.finishCall(callID, map[string]any{"ok": true}, nil)
		}()
	}
	finishWG.Wait()

	if callbacksOverlapped.Load() {
		t.Fatal("onDelta callbacks overlapped")
	}
	result := execution.result()
	for index := 0; index < fragmentCount; index++ {
		fragment := fmt.Sprintf("fragment-%02d|", index)
		if count := strings.Count(result.Reply, fragment); count != 1 {
			t.Fatalf("final reply contains %q %d times, want once", fragment, count)
		}
	}
	emittedMu.Lock()
	defer emittedMu.Unlock()
	if len(emittedReplies) != 1 {
		t.Fatalf("text delta count = %d, want 1", len(emittedReplies))
	}
	if emittedReplies[0] != result.Reply {
		t.Fatalf("emitted reply does not match final result: emitted=%q result=%q", emittedReplies[0], result.Reply)
	}
}
