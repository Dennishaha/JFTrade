package adk

import (
	"context"
	"fmt"
	"time"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
)

// The claim implementation lives in engine/persistence. These thin methods
// preserve the historical nil-receiver contract of *Store and keep the
// embedded ClaimStore from being dereferenced through a nil Store.

func (s *Store) ClaimRunLease(
	ctx context.Context,
	runID string,
	ownerID string,
	now time.Time,
	ttl time.Duration,
) (enginepersistence.RunLease, error) {
	if s == nil || s.ClaimStore == nil {
		return enginepersistence.RunLease{}, fmt.Errorf("ADK run lease requires store, run id, and owner id")
	}
	return s.ClaimStore.ClaimRunLease(ctx, runID, ownerID, now, ttl)
}

func (s *Store) HeartbeatRunLease(
	ctx context.Context,
	lease enginepersistence.RunLease,
	now time.Time,
	ttl time.Duration,
) (enginepersistence.RunLease, error) {
	if s == nil || s.ClaimStore == nil {
		return enginepersistence.RunLease{}, fmt.Errorf("ADK run lease TTL must be positive")
	}
	return s.ClaimStore.HeartbeatRunLease(ctx, lease, now, ttl)
}

func (s *Store) ReleaseRunLease(ctx context.Context, lease enginepersistence.RunLease) error {
	if s == nil || s.ClaimStore == nil {
		return nil
	}
	return s.ClaimStore.ReleaseRunLease(ctx, lease)
}

func (s *Store) RunLease(ctx context.Context, runID string) (enginepersistence.RunLease, bool, error) {
	if s == nil || s.ClaimStore == nil {
		return enginepersistence.RunLease{}, false, nil
	}
	return s.ClaimStore.RunLease(ctx, runID)
}

func (s *Store) ClaimToolInvocation(
	ctx context.Context,
	claim enginepersistence.ToolInvocationClaim,
) (enginepersistence.ToolInvocationTicket, error) {
	if s == nil || s.ClaimStore == nil {
		return enginepersistence.ToolInvocationTicket{}, fmt.Errorf("ADK tool invocation claim is incomplete")
	}
	return s.ClaimStore.ClaimToolInvocation(ctx, claim)
}

func (s *Store) HeartbeatToolInvocation(
	ctx context.Context,
	ticket enginepersistence.ToolInvocationTicket,
	now time.Time,
	ttl time.Duration,
) error {
	if s == nil || s.ClaimStore == nil {
		return fmt.Errorf("ADK tool invocation TTL must be positive")
	}
	return s.ClaimStore.HeartbeatToolInvocation(ctx, ticket, now, ttl)
}

func (s *Store) CompleteToolInvocation(
	ctx context.Context,
	ticket enginepersistence.ToolInvocationTicket,
	output map[string]any,
	now time.Time,
) error {
	if s == nil || s.ClaimStore == nil {
		return fmt.Errorf("ADK tool invocation output encoding failed")
	}
	return s.ClaimStore.CompleteToolInvocation(ctx, ticket, output, now)
}

func (s *Store) MarkToolInvocationIndeterminate(
	ctx context.Context,
	ticket enginepersistence.ToolInvocationTicket,
	now time.Time,
) error {
	if s == nil || s.ClaimStore == nil {
		return fmt.Errorf("ADK tool invocation is unavailable")
	}
	return s.ClaimStore.MarkToolInvocationIndeterminate(ctx, ticket, now)
}

func (s *Store) AbandonToolInvocation(ctx context.Context, ticket enginepersistence.ToolInvocationTicket) error {
	if s == nil || s.ClaimStore == nil {
		return fmt.Errorf("ADK tool invocation is unavailable")
	}
	return s.ClaimStore.AbandonToolInvocation(ctx, ticket)
}
