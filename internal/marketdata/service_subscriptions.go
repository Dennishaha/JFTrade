package marketdata

import (
	"context"
	"fmt"
	"log"
	"time"
)

const subscriptionCleanupTimeout = 5 * time.Second

// AcquireSubscription 申请行情订阅。
func (s *Service) AcquireSubscription(ctx context.Context, consumerID string, instruments []InstrumentRef) (SubscriptionResult, error) {
	s.subscriptionLifecycleMu.Lock()
	defer s.subscriptionLifecycleMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSubscriptionRefs(instruments); err != nil {
		return nil, err
	}
	_, rollback := s.subscriptions.acquireWithMode(consumerID, instruments, false)
	if err := s.reconcileSubscriptions(ctx); err != nil {
		s.subscriptions.restore(rollback)
		_ = s.reconcileSubscriptionsForCleanup()
		return nil, err
	}
	s.WakeCollector()
	snapshot, _ := s.GetSubscriptions(ctx)
	return SubscriptionResult(snapshot), nil
}

// AcquireManagedSubscription creates a non-expiring lease for an in-process
// consumer such as a running strategy. The lease is rolled back if the broker
// cannot establish the requested subscriptions.
func (s *Service) AcquireManagedSubscription(ctx context.Context, consumerID string, instruments []InstrumentRef) (*ManagedSubscription, error) {
	if s == nil {
		return nil, fmt.Errorf("market-data service is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSubscriptionRefs(instruments); err != nil {
		return nil, err
	}
	s.subscriptionLifecycleMu.Lock()
	if availability, ok := s.provider.(PushAvailability); ok && !availability.PushAvailable() {
		s.subscriptionLifecycleMu.Unlock()
		return nil, ErrManagedSubscriptionsUnavailable
	}
	_, rollback := s.subscriptions.acquireWithMode(consumerID, instruments, true)
	if err := s.reconcileSubscriptions(ctx); err != nil {
		s.subscriptions.restore(rollback)
		_ = s.reconcileSubscriptionsForCleanup()
		s.subscriptionLifecycleMu.Unlock()
		return nil, err
	}
	s.subscriptionLifecycleMu.Unlock()
	s.WakeCollector()
	return newManagedSubscription(func() {
		s.subscriptionLifecycleMu.Lock()
		defer s.subscriptionLifecycleMu.Unlock()
		s.subscriptions.restore(rollback)
		if err := s.reconcileSubscriptionsForCleanup(); err != nil {
			log.Printf("marketdata managed subscription release reconciliation failed: %v", err)
		}
		s.WakeCollector()
	}), nil
}

// ReleaseSubscription 释放行情订阅。
func (s *Service) ReleaseSubscription(ctx context.Context, consumerID string, target ...InstrumentRef) error {
	s.subscriptionLifecycleMu.Lock()
	defer s.subscriptionLifecycleMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(target) > 0 {
		s.subscriptions.release(consumerID, target[0])
	} else {
		s.subscriptions.clear(consumerID)
	}
	if err := s.reconcileSubscriptions(ctx); err != nil {
		log.Printf("marketdata subscription release reconciliation deferred: %v", err)
	}
	s.WakeCollector()
	return nil
}

// Heartbeat 刷新订阅心跳。
func (s *Service) Heartbeat(ctx context.Context, consumerID string) (HeartbeatResult, error) {
	s.subscriptionLifecycleMu.Lock()
	defer s.subscriptionLifecycleMu.Unlock()
	s.subscriptions.heartbeat(consumerID)
	if err := s.reconcileSubscriptions(ctx); err != nil {
		log.Printf("marketdata subscription heartbeat reconciliation deferred: %v", err)
	}
	snapshot, err := s.GetSubscriptions(ctx)
	return HeartbeatResult(snapshot), err
}

// ClearSubscriptions 清空订阅。
func (s *Service) ClearSubscriptions(ctx context.Context, consumerID ...string) error {
	s.subscriptionLifecycleMu.Lock()
	defer s.subscriptionLifecycleMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	rawConsumerID := ""
	if len(consumerID) > 0 {
		rawConsumerID = consumerID[0]
	}
	s.subscriptions.clear(rawConsumerID)
	if err := s.reconcileSubscriptions(ctx); err != nil {
		log.Printf("marketdata subscription clear reconciliation deferred: %v", err)
	}
	s.WakeCollector()
	return nil
}

// GetSubscriptions 返回当前订阅快照。
func (s *Service) GetSubscriptions(ctx context.Context) (SubscriptionsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot := s.subscriptions.snapshot()
	s.subscriptionMu.RLock()
	reconciler := s.reconciler
	s.subscriptionMu.RUnlock()
	var broker map[string]any
	if reconciler != nil {
		broker = reconciler.SubscriptionState()
	}
	return decorateSubscriptionSnapshot(snapshot, broker), nil
}

// GetActiveInstruments 返回当前活跃标的列表。
func (s *Service) GetActiveInstruments(ctx context.Context) ([]string, error) {
	return s.subscriptions.activeInstruments(), nil
}

func (s *Service) reconcileSubscriptions(ctx context.Context) error {
	return s.reconcileDesired(ctx, s.activeSubscriptionDemand())
}

func (s *Service) reconcileSubscriptionsForCleanup() error {
	return s.reconcileDesiredForCleanup(s.activeSubscriptionDemand())
}

func (s *Service) reconcileDesiredForCleanup(desired []InstrumentRef) error {
	ctx, cancel := context.WithTimeout(context.Background(), subscriptionCleanupTimeout)
	defer cancel()
	return s.reconcileDesired(ctx, desired)
}

func (s *Service) activeSubscriptionDemand() []InstrumentRef {
	refs := s.subscriptions.activeSubscriptions()
	s.subscriptionMu.RLock()
	demands := append([]DemandSource(nil), s.additionalDemands...)
	s.subscriptionMu.RUnlock()
	for _, demand := range demands {
		if demand == nil {
			continue
		}
		for _, instrumentID := range demand.ActiveInstruments() {
			market, symbol := normalizeSubscriptionInstrument("", instrumentID)
			if market == "" || symbol == "" {
				continue
			}
			refs = append(refs, InstrumentRef{Channel: "SNAPSHOT", Market: market, Symbol: symbol})
		}
	}
	return refs
}

// ActiveSubscriptionDemand returns a broker-neutral snapshot for application
// provider activation. Callers that switch providers should capture it while
// holding the service's provider-change lifecycle gate.
func (s *Service) ActiveSubscriptionDemand() []InstrumentRef {
	if s == nil {
		return nil
	}
	return append([]InstrumentRef(nil), s.activeSubscriptionDemand()...)
}

func (s *Service) reconcileDesired(ctx context.Context, desired []InstrumentRef) error {
	if s == nil {
		return nil
	}
	s.subscriptionMu.RLock()
	reconciler := s.reconciler
	s.subscriptionMu.RUnlock()
	if reconciler == nil {
		return nil
	}
	return reconciler.ReconcileSubscriptions(ctx, desired)
}
