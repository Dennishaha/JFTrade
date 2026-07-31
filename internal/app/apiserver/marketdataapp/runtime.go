// Package marketdataapp owns application-level market-data provider selection.
// It keeps the broker-neutral marketdata.Service stable while routing provider,
// polling, streaming, and subscription calls to the active data source.
package marketdataapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	yfinanceintegration "github.com/jftrade/jftrade-main/internal/integration/yfinance"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

const (
	ProviderFutu     = "futu"
	ProviderYFinance = "yfinance"

	// A frozen PyInstaller binary can spend tens of seconds unpacking its
	// bundled Python runtime on the first launch, especially on macOS.
	providerActivationTimeout = 45 * time.Second
	providerHealthRetryDelay  = 100 * time.Millisecond
)

var (
	ErrStreamingUnavailable = errors.New("active market-data provider does not support streaming quotes")
	ErrRuntimeClosed        = errors.New("market-data provider runtime is closed")
)

// RuntimeOptions supplies the already assembled Futu data plane. YFinance is
// created lazily from persisted settings when it becomes active.
type RuntimeOptions struct {
	FutuProvider      marketdata.Provider
	FutuQuotes        marketdata.QuoteSource
	FutuPush          marketdata.PushSource
	FutuSubscriptions marketdata.SubscriptionReconciler
}

// Activation describes one desired provider selection.
type Activation struct {
	ProviderID           string
	// YFinanceEndpoint is reserved for in-process contract tests. Production
	// callers leave it empty so the embedded helper owns the endpoint.
	YFinanceEndpoint     string
	RequireHealthy       bool
	DesiredSubscriptions []marketdata.InstrumentRef
}

type runtimeState struct {
	providerID    string
	provider      marketdata.Provider
	quotes        marketdata.QuoteSource
	push          marketdata.PushSource
	subscriptions marketdata.SubscriptionReconciler
}

// Runtime is the stable adapter held by marketdata.Service and Collector.
type Runtime struct {
	mu          sync.RWMutex
	switchMu    sync.Mutex
	futu        runtimeState
	active      runtimeState
	sidecar     sidecarLifecycle
	healthCheck func(context.Context, marketdata.Provider) error
	closed      bool
}

var (
	_ marketdata.Provider                 = (*Runtime)(nil)
	_ marketdata.QuoteSource              = (*Runtime)(nil)
	_ marketdata.QuotePollingPolicySource = (*Runtime)(nil)
	_ marketdata.PushSource               = (*Runtime)(nil)
	_ marketdata.PushAvailability         = (*Runtime)(nil)
	_ marketdata.SubscriptionReconciler   = (*Runtime)(nil)
)

// NewRuntime creates a stable provider router with Futu as its initial source.
func NewRuntime(options RuntimeOptions) (*Runtime, error) {
	if options.FutuProvider == nil {
		return nil, fmt.Errorf("futu market-data provider is required")
	}
	futu := runtimeState{
		providerID:    ProviderFutu,
		provider:      options.FutuProvider,
		quotes:        options.FutuQuotes,
		push:          options.FutuPush,
		subscriptions: options.FutuSubscriptions,
	}
	return &Runtime{
		futu:        futu,
		active:      futu,
		sidecar:     newSidecarManager(),
		healthCheck: waitForProviderHealth,
	}, nil
}

// Activate atomically selects a provider after its optional process lifecycle
// has been prepared, previous physical subscriptions released, and the new
// provider's current logical demand physically reconciled.
func (r *Runtime) Activate(ctx context.Context, activation Activation) error {
	if r == nil {
		return fmt.Errorf("market-data provider runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	activationCtx, cancel := context.WithTimeout(ctx, providerActivationTimeout)
	defer cancel()
	r.switchMu.Lock()
	defer r.switchMu.Unlock()
	if r.closed {
		return ErrRuntimeClosed
	}
	if err := activationCtx.Err(); err != nil {
		return err
	}

	previous := r.snapshot()
	targetProviderID := normalizeProviderID(activation.ProviderID)
	next, err := r.resolveActivation(activation)
	if err != nil {
		return r.rollbackPreparedSidecar(previous, targetProviderID, err)
	}
	if next.providerID == ProviderYFinance {
		if err := r.checkHealth(activationCtx, next.provider); err != nil {
			return r.rollbackPreparedSidecar(
				previous,
				next.providerID,
				fmt.Errorf("verify %s market-data provider: %w", next.providerID, err),
			)
		}
	}
	if previous.subscriptions != nil && previous.providerID != next.providerID {
		if err := releaseProviderSubscriptions(activationCtx, previous.subscriptions); err != nil {
			return r.rollbackPreparedSidecar(
				previous,
				next.providerID,
				fmt.Errorf(
					"release %s market-data subscriptions: %w",
					previous.providerID,
					err,
				),
			)
		}
	}
	if previous.providerID != next.providerID && next.subscriptions != nil {
		if err := activateProviderSubscriptions(
			activationCtx,
			next,
			activation.DesiredSubscriptions,
		); err != nil {
			return r.rollbackPreparedSidecar(previous, next.providerID, err)
		}
	}
	if next.providerID == ProviderFutu {
		if err := r.sidecar.Stop(); err != nil {
			rollbackErr := error(nil)
			if previous.providerID != next.providerID && next.subscriptions != nil {
				rollbackErr = releaseProviderSubscriptions(activationCtx, next.subscriptions)
			}
			return errors.Join(fmt.Errorf("stop yfinance sidecar: %w", err), rollbackErr)
		}
	}
	r.mu.Lock()
	r.active = next
	r.mu.Unlock()
	return nil
}

func activateProviderSubscriptions(
	ctx context.Context,
	next runtimeState,
	desired []marketdata.InstrumentRef,
) error {
	err := next.subscriptions.ReconcileSubscriptions(
		ctx,
		append([]marketdata.InstrumentRef(nil), desired...),
	)
	if err == nil {
		return nil
	}
	activationErr := fmt.Errorf(
		"activate %s market-data subscriptions: %w",
		next.providerID,
		err,
	)
	rollbackCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		providerActivationTimeout,
	)
	defer cancel()
	if rollbackErr := releaseProviderSubscriptions(rollbackCtx, next.subscriptions); rollbackErr != nil {
		return errors.Join(
			activationErr,
			fmt.Errorf(
				"rollback %s market-data subscriptions: %w",
				next.providerID,
				rollbackErr,
			),
		)
	}
	return activationErr
}

func (r *Runtime) rollbackPreparedSidecar(
	previous runtimeState,
	targetProviderID string,
	activationErr error,
) error {
	if targetProviderID != ProviderYFinance || previous.providerID == ProviderYFinance {
		return activationErr
	}
	if stopErr := r.sidecar.Stop(); stopErr != nil {
		return errors.Join(
			activationErr,
			fmt.Errorf("rollback yfinance sidecar: %w", stopErr),
		)
	}
	return activationErr
}

func releaseProviderSubscriptions(
	ctx context.Context,
	subscriptions marketdata.SubscriptionReconciler,
) error {
	if releaser, ok := subscriptions.(marketdata.ForcedSubscriptionReleaser); ok {
		return releaser.ForceReleaseSubscriptions(ctx)
	}
	return subscriptions.ReconcileSubscriptions(ctx, nil)
}

func (r *Runtime) checkHealth(ctx context.Context, provider marketdata.Provider) error {
	if r.healthCheck == nil {
		return waitForProviderHealth(ctx, provider)
	}
	return r.healthCheck(ctx, provider)
}

func waitForProviderHealth(ctx context.Context, provider marketdata.Provider) error {
	if provider == nil {
		return fmt.Errorf("market-data provider is unavailable")
	}
	probeCtx, cancel := context.WithTimeout(ctx, providerActivationTimeout)
	defer cancel()
	var lastErr error
	for {
		health, err := provider.Health(probeCtx)
		switch {
		case err == nil && health.Connected:
			return nil
		case err != nil:
			lastErr = err
		default:
			lastErr = fmt.Errorf("provider reported disconnected")
		}
		timer := time.NewTimer(providerHealthRetryDelay)
		select {
		case <-probeCtx.Done():
			timer.Stop()
			if lastErr == nil {
				lastErr = probeCtx.Err()
			}
			return lastErr
		case <-timer.C:
		}
	}
}

func (r *Runtime) resolveActivation(activation Activation) (runtimeState, error) {
	switch normalizeProviderID(activation.ProviderID) {
	case ProviderFutu:
		return r.futu, nil
	case ProviderYFinance:
		endpoint := strings.TrimSpace(activation.YFinanceEndpoint)
		if endpoint == "" {
			var err error
			endpoint, err = r.sidecar.EnsureStarted()
			if err != nil {
				return runtimeState{}, err
			}
		}
		provider, err := yfinanceintegration.NewProvider(endpoint)
		if err != nil {
			return runtimeState{}, err
		}
		return runtimeState{
			providerID: ProviderYFinance,
			provider:   provider,
			quotes:     provider,
		}, nil
	default:
		return runtimeState{}, fmt.Errorf("unsupported market-data provider %q", activation.ProviderID)
	}
}

func normalizeProviderID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (r *Runtime) snapshot() runtimeState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

// ActiveProviderID returns the persisted selection vocabulary, not the
// provider descriptor ID.
func (r *Runtime) ActiveProviderID() string {
	return r.snapshot().providerID
}

// Close stops an application-owned Python sidecar, if one is running.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.switchMu.Lock()
	defer r.switchMu.Unlock()
	r.closed = true
	if r.sidecar == nil {
		return nil
	}
	if err := r.sidecar.Close(); err != nil {
		if retryErr := r.sidecar.Close(); retryErr != nil {
			return errors.Join(
				err,
				fmt.Errorf("retry yfinance sidecar cleanup: %w", retryErr),
			)
		}
	}
	return nil
}

func (r *Runtime) Descriptor(ctx context.Context) (marketdata.ProviderDescriptor, error) {
	return r.snapshot().provider.Descriptor(ctx)
}

func (r *Runtime) GetMarkets(ctx context.Context) ([]marketdata.MarketProfile, error) {
	return r.snapshot().provider.GetMarkets(ctx)
}

func (r *Runtime) GetSecurityDetails(ctx context.Context, market, symbol string) (marketdata.SecurityDetails, error) {
	return r.snapshot().provider.GetSecurityDetails(ctx, market, symbol)
}

func (r *Runtime) LookupInstrument(ctx context.Context, market, code string) ([]marketdata.InstrumentCandidate, error) {
	return r.snapshot().provider.LookupInstrument(ctx, market, code)
}

func (r *Runtime) SearchInstruments(ctx context.Context, query string, limit int) ([]marketdata.InstrumentCandidate, error) {
	return r.snapshot().provider.SearchInstruments(ctx, query, limit)
}

func (r *Runtime) QuerySnapshot(ctx context.Context, instrumentID string) (*marketdata.Tick, error) {
	return r.snapshot().provider.QuerySnapshot(ctx, instrumentID)
}

func (r *Runtime) QueryTicker(ctx context.Context, instrumentID string) (*marketdata.Tick, error) {
	return r.snapshot().provider.QueryTicker(ctx, instrumentID)
}

func (r *Runtime) GetHistoricalCandles(
	ctx context.Context,
	market string,
	symbol string,
	period string,
	limit int,
	fromTime string,
	toTime string,
) (marketdata.CandlesResponse, error) {
	return r.snapshot().provider.GetHistoricalCandles(ctx, market, symbol, period, limit, fromTime, toTime)
}

func (r *Runtime) GetDepth(ctx context.Context, market, symbol string, num int) (marketdata.DepthResponse, error) {
	return r.snapshot().provider.GetDepth(ctx, market, symbol, num)
}

func (r *Runtime) NormalizeInstrument(ctx context.Context, input map[string]any) (map[string]any, error) {
	return r.snapshot().provider.NormalizeInstrument(ctx, input)
}

func (r *Runtime) Health(ctx context.Context) (marketdata.HealthStatus, error) {
	return r.snapshot().provider.Health(ctx)
}

// QueryTickers routes Collector polling through the active provider.
func (r *Runtime) QueryTickers(ctx context.Context, instrumentIDs []string) (map[string]marketdata.Tick, error) {
	source := r.snapshot().quotes
	if source == nil {
		return nil, fmt.Errorf("active market-data provider does not support snapshot polling")
	}
	return source.QueryTickers(ctx, instrumentIDs)
}

// QuotePollingPolicy follows the active source without rebuilding Collector.
func (r *Runtime) QuotePollingPolicy() marketdata.QuotePollingPolicy {
	source := r.snapshot().quotes
	if policy, ok := source.(marketdata.QuotePollingPolicySource); ok {
		return policy.QuotePollingPolicy()
	}
	return marketdata.QuotePollingPolicy{}
}

// PushAvailable lets Collector avoid retrying a capability the active provider
// explicitly does not offer.
func (r *Runtime) PushAvailable() bool {
	return r.snapshot().push != nil
}

func (r *Runtime) NewStream(instrumentIDs []string, handler marketdata.PushTickHandler) (marketdata.PushStream, error) {
	source := r.snapshot().push
	if source == nil {
		return nil, ErrStreamingUnavailable
	}
	return source.NewStream(instrumentIDs, handler)
}

func (r *Runtime) ReconcileSubscriptions(ctx context.Context, desired []marketdata.InstrumentRef) error {
	r.switchMu.Lock()
	defer r.switchMu.Unlock()
	if r.closed {
		return ErrRuntimeClosed
	}
	reconciler := r.snapshot().subscriptions
	if reconciler == nil {
		return nil
	}
	return reconciler.ReconcileSubscriptions(ctx, desired)
}

func (r *Runtime) SubscriptionState() map[string]any {
	reconciler := r.snapshot().subscriptions
	if reconciler == nil {
		return nil
	}
	return reconciler.SubscriptionState()
}
