package assistant

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
)

func TestServiceCloseCancelsAndJoinsAdmittedWorkflowBackground(t *testing.T) {
	service := NewService(nil)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	if admitted := service.goWorkflowBackground(t.Context(), func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
	}); !admitted {
		t.Fatal("workflow background task was not admitted")
	}
	<-started

	const closeCallers = 8
	closeResults := make(chan error, closeCallers)
	var closeWG sync.WaitGroup
	for range closeCallers {
		closeWG.Add(1)
		go func() {
			defer closeWG.Done()
			closeResults <- service.Close()
		}()
	}
	<-cancelled
	select {
	case err := <-closeResults:
		t.Fatalf("Close returned before admitted work exited: %v", err)
	default:
	}

	close(release)
	closeWG.Wait()
	close(closeResults)
	for err := range closeResults {
		if err != nil {
			t.Fatalf("concurrent Close: %v", err)
		}
	}
	if admitted := service.goWorkflowBackground(t.Context(), func(context.Context) {
		t.Error("workflow task ran after Service.Close")
	}); admitted {
		t.Fatal("workflow background task was admitted after Service.Close")
	}
}

func TestServiceCloseKeepsStoreOpenUntilWorkflowCleanupFinishes(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	writeResult := make(chan error, 1)
	if admitted := service.goWorkflowBackground(t.Context(), func(ctx context.Context) {
		<-ctx.Done()
		writeResult <- runtime.Store().AddAuditEvent(context.Background(), jfadk.AuditEvent{
			Kind:      "workflow.shutdown.cleanup",
			SubjectID: "workflow-lifecycle-test",
		})
	}); !admitted {
		t.Fatal("workflow cleanup task was not admitted")
	}

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- service.Close()
	}()
	if err := <-writeResult; err != nil {
		t.Fatalf("workflow cleanup wrote after the store closed: %v", err)
	}
	if err := <-closeResult; err != nil {
		t.Fatalf("Service.Close: %v", err)
	}
	if err := runtime.Store().AddAuditEvent(context.Background(), jfadk.AuditEvent{
		Kind: "workflow.shutdown.after-close",
	}); err == nil {
		t.Fatal("runtime store remained writable after Service.Close")
	}
	if _, err := service.startWorkflowAsync(
		t.Context(),
		jfadk.WorkflowDefinition{Status: jfadk.WorkflowStatusEnabled},
		nil,
		jfadk.WorkflowTriggerTypeManual,
		nil,
		nil,
	); !errors.Is(err, errAssistantServiceClosing) {
		t.Fatalf("startWorkflowAsync after close error = %v, want %v", err, errAssistantServiceClosing)
	}
}

func TestWorkflowSchedulerStopCancelsAndJoinsInFlightTick(t *testing.T) {
	enteredSnapshot := make(chan struct{})
	observedCancel := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	runtime, service, _ := newAssistantServiceHarness(t, WithWorkflowMarketSnapshot(
		func(ctx context.Context, _ string) (map[string]any, error) {
			close(enteredSnapshot)
			<-ctx.Done()
			close(observedCancel)
			<-releaseSnapshot
			return nil, ctx.Err()
		},
	))
	if _, err := runtime.Store().SaveWorkflowTrigger(t.Context(), jfadk.WorkflowTrigger{
		ID:         "workflow-scheduler-lifecycle",
		WorkflowID: "workflow-scheduler-lifecycle",
		Type:       jfadk.WorkflowTriggerTypeMarketThreshold,
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{
			"instrumentIds": []string{"US.AAPL"},
			"snapshotPath":  "snapshot.price",
			"value":         100,
			"edge":          "above",
		},
	}); err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}

	scheduler := &WorkflowScheduler{service: service, interval: time.Hour}
	scheduler.Start(t.Context())
	<-enteredSnapshot
	stopped := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(stopped)
	}()
	<-observedCancel
	select {
	case <-stopped:
		t.Fatal("WorkflowScheduler.Stop returned before its in-flight tick exited")
	default:
	}
	close(releaseSnapshot)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("WorkflowScheduler.Stop did not join its in-flight tick")
	}

	scheduler.Start(t.Context())
	scheduler.Stop()
}
