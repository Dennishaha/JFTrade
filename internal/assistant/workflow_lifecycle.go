package assistant

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errAssistantServiceClosing = errors.New("assistant service is closing")

// reserveWorkflowBackground closes the Add/Wait admission race by guarding
// both the closing flag and WaitGroup.Add with workflowMu. The returned context
// preserves caller values while detaching from caller cancellation; Service
// shutdown remains the owner of the background work.
func (s *Service) reserveWorkflowBackground(
	base context.Context,
) (context.Context, func(), bool) {
	if s == nil {
		return nil, nil, false
	}
	s.workflowMu.Lock()
	if s.workflowClosed {
		s.workflowMu.Unlock()
		return nil, nil, false
	}
	ownerCtx := s.workflowContextLocked()
	s.workflowWG.Add(1)
	s.workflowMu.Unlock()

	if base == nil {
		base = context.Background()
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(base))
	stopOwnerCancel := context.AfterFunc(ownerCtx, cancel)
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			stopOwnerCancel()
			cancel()
			s.workflowWG.Done()
		})
	}
	return runCtx, release, true
}

func (s *Service) goWorkflowBackground(
	base context.Context,
	run func(context.Context),
) bool {
	if run == nil {
		return false
	}
	ctx, release, admitted := s.reserveWorkflowBackground(base)
	if !admitted {
		return false
	}
	go func() {
		defer release()
		run(ctx)
	}()
	return true
}

func (s *Service) reserveWorkflowScheduler(
	interval time.Duration,
) (*WorkflowScheduler, context.Context, func(), bool) {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	if s.workflowClosed || s.workflowScheduler != nil {
		return nil, nil, nil, false
	}
	ownerCtx := s.workflowContextLocked()
	scheduler := &WorkflowScheduler{service: s, interval: interval}
	s.workflowScheduler = scheduler
	s.workflowWG.Add(1)
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(s.workflowWG.Done)
	}
	return scheduler, ownerCtx, release, true
}

func (s *Service) workflowContextLocked() context.Context {
	if s.workflowCtx == nil {
		s.workflowCtx, s.workflowCancel = context.WithCancel(context.Background())
	}
	return s.workflowCtx
}

func (s *Service) beginWorkflowShutdown() *WorkflowScheduler {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	s.workflowClosed = true
	if s.workflowCancel != nil {
		s.workflowCancel()
	}
	scheduler := s.workflowScheduler
	s.workflowScheduler = nil
	return scheduler
}
