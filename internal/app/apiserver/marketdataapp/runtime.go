// Package marketdataapp owns application-level market-data provider selection.
// It keeps the broker-neutral marketdata.Service stable while routing provider,
// polling, streaming, and subscription calls to the active data source.
package marketdataapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	akshareintegration "github.com/jftrade/jftrade-main/internal/integration/akshare"
	yfinanceintegration "github.com/jftrade/jftrade-main/internal/integration/yfinance"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

const (
	ProviderFutu     = "futu"
	ProviderYFinance = "yfinance"
	ProviderAKShare  = "akshare"

	// A frozen PyInstaller binary can spend tens of seconds unpacking its
	// bundled Python runtime on the first launch, especially on macOS.
	providerActivationTimeout   = 45 * time.Second
	providerHealthRetryDelay    = 100 * time.Millisecond
	providerHealthMaxRetryDelay = time.Second
)

var (
	ErrStreamingUnavailable = errors.New("active market-data provider does not support streaming quotes")
	ErrRuntimeClosed        = errors.New("market-data provider runtime is closed")
)

// RuntimeOptions supplies the already assembled Futu data plane. Python-backed
// providers are created lazily from persisted settings when one becomes active.
type RuntimeOptions struct {
	FutuProvider       marketdata.Provider
	FutuQuotes         marketdata.QuoteSource
	FutuPush           marketdata.PushSource
	FutuSubscriptions  marketdata.SubscriptionReconciler
	MarketDataCacheDir string
	// YFinanceCacheDir is a deprecated compatibility fallback.
	YFinanceCacheDir string
}

// Activation describes one desired provider selection.
type Activation struct {
	ProviderID string
	// YFinanceEndpoint is reserved for in-process contract tests. Production
	// callers leave it empty so the embedded helper owns the endpoint.
	YFinanceEndpoint string
	// AKShareEndpoint is reserved for in-process contract tests. Production
	// callers leave it empty so the shared helper owns the endpoint.
	AKShareEndpoint      string
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
	mu             sync.RWMutex
	switchMu       sync.Mutex
	futu           runtimeState
	active         runtimeState
	sidecar        sidecarLifecycle
	healthCheck    func(context.Context, marketdata.Provider, bool) error
	closed         bool
	providerLeases map[string]int
	providerPool   map[string]runtimeState
}

// ProviderLease pins a concrete provider instance for a module operation. A
// global provider switch only changes Runtime.active and cannot invalidate a
// lease already accepted by backtest or another consumer.
type ProviderLease struct {
	runtime *Runtime
	state   runtimeState
	once    sync.Once
}

var (
	_ marketdata.Provider                    = (*Runtime)(nil)
	_ marketdata.QuoteSource                 = (*Runtime)(nil)
	_ marketdata.QuotePollingPolicySource    = (*Runtime)(nil)
	_ marketdata.NewsSource                  = (*Runtime)(nil)
	_ marketdata.CorporateActionsSource      = (*Runtime)(nil)
	_ marketdata.IndexConstituentsSource     = (*Runtime)(nil)
	_ marketdata.PushSource                  = (*Runtime)(nil)
	_ marketdata.PushAvailability            = (*Runtime)(nil)
	_ marketdata.PushInstrumentFilter        = (*Runtime)(nil)
	_ marketdata.SubscriptionReconciler      = (*Runtime)(nil)
	_ marketdata.SubscriptionFallbackState   = (*Runtime)(nil)
	_ marketdata.InactiveSubscriptionCleaner = (*Runtime)(nil)
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
		futu:           futu,
		active:         futu,
		sidecar:        newSidecarManager(runtimeSidecarCacheDir(options)),
		healthCheck:    waitForProviderHealth,
		providerLeases: make(map[string]int),
		providerPool:   map[string]runtimeState{ProviderFutu: futu},
	}, nil
}

// Activate atomically selects a provider after its optional process lifecycle
// has been prepared, previous physical subscription demand retired, and the
// new provider's current logical demand physically reconciled. Broker retention
// rules may defer the old provider's final physical unsubscribe.
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
	if isPythonProvider(next.providerID) {
		if err := r.checkHealth(
			activationCtx,
			next.provider,
			activation.RequireHealthy,
		); err != nil {
			return r.rollbackPreparedSidecar(
				previous,
				next.providerID,
				fmt.Errorf("verify %s market-data provider: %w", next.providerID, err),
			)
		}
	}
	if previous.subscriptions != nil && previous.providerID != next.providerID {
		if err := releaseProviderSubscriptions(activationCtx, previous.subscriptions); err != nil {
			releaseErr := fmt.Errorf(
				"release %s market-data subscriptions: %w",
				previous.providerID,
				err,
			)
			if previous.providerID != ProviderFutu {
				return r.rollbackPreparedSidecar(previous, next.providerID, releaseErr)
			}
			log.Printf("JFTrade inactive Futu subscription cleanup deferred during provider switch: %v", err)
		}
	}
	if previous.providerID != next.providerID && next.subscriptions != nil {
		if err := activateProviderSubscriptions(
			activationCtx,
			next,
			activation.DesiredSubscriptions,
		); err != nil {
			err = errors.Join(err, r.restorePreviousSubscriptions(previous, activation.DesiredSubscriptions))
			return r.rollbackPreparedSidecar(previous, next.providerID, err)
		}
	}
	r.mu.Lock()
	r.active = next
	r.mu.Unlock()
	r.releaseIdleProviderLocked(previous.providerID)
	if next.providerID == ProviderFutu && !r.hasPythonLeasesLocked() {
		r.releaseIdlePythonProvidersLocked()
		if err := r.sidecar.Stop(); err != nil {
			// Futu is already prepared and does not depend on the sidecar. Keep
			// the provider switch committed while retaining the sidecar state for
			// Runtime.Close or a later retry to finish cleanup.
			log.Printf("JFTrade market-data sidecar cleanup deferred after switching to Futu: %v", err)
		}
	}
	return nil
}

func (r *Runtime) restorePreviousSubscriptions(
	previous runtimeState,
	desired []marketdata.InstrumentRef,
) error {
	if previous.subscriptions == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		providerActivationTimeout,
	)
	defer cancel()
	if err := previous.subscriptions.ReconcileSubscriptions(
		ctx,
		append([]marketdata.InstrumentRef(nil), desired...),
	); err != nil {
		return fmt.Errorf("restore %s market-data subscriptions: %w", previous.providerID, err)
	}
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
	r.releaseIdleProviderLocked(targetProviderID)
	if !isPythonProvider(targetProviderID) || isPythonProvider(previous.providerID) || r.hasPythonLeasesLocked() {
		return activationErr
	}
	if stopErr := r.sidecar.Stop(); stopErr != nil {
		return errors.Join(
			activationErr,
			fmt.Errorf("rollback market-data sidecar: %w", stopErr),
		)
	}
	return activationErr
}

// AcquireProvider prepares and pins a provider without changing the global
// active selection. The returned lease must be released by its owner.
func (r *Runtime) AcquireProvider(
	ctx context.Context,
	providerID string,
	requireHealthy bool,
) (*ProviderLease, error) {
	if r == nil {
		return nil, fmt.Errorf("market-data provider runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	activationCtx, cancel := context.WithTimeout(ctx, providerActivationTimeout)
	defer cancel()
	r.switchMu.Lock()
	defer r.switchMu.Unlock()
	if r.closed {
		return nil, ErrRuntimeClosed
	}
	previous := r.snapshot()
	targetID := normalizeProviderID(providerID)
	state, err := r.resolveActivation(Activation{ProviderID: targetID})
	if err != nil {
		return nil, r.rollbackPreparedSidecar(previous, targetID, err)
	}
	if requireHealthy {
		if err := r.checkHealth(activationCtx, state.provider, true); err != nil {
			return nil, r.rollbackPreparedSidecar(previous, targetID,
				fmt.Errorf("verify %s market-data provider: %w", targetID, err))
		}
	}
	if r.providerLeases == nil {
		r.providerLeases = make(map[string]int)
	}
	r.providerLeases[state.providerID]++
	return &ProviderLease{runtime: r, state: state}, nil
}

func (l *ProviderLease) ProviderID() string {
	if l == nil {
		return ""
	}
	return l.state.providerID
}

func (l *ProviderLease) Provider() marketdata.Provider {
	if l == nil {
		return nil
	}
	return l.state.provider
}

func (l *ProviderLease) Descriptor(ctx context.Context) (marketdata.ProviderDescriptor, error) {
	if l == nil || l.state.provider == nil {
		return marketdata.ProviderDescriptor{}, fmt.Errorf("market-data provider lease is unavailable")
	}
	return l.state.provider.Descriptor(ctx)
}

func (l *ProviderLease) Release() {
	if l == nil || l.runtime == nil {
		return
	}
	l.once.Do(func() { l.runtime.releaseProviderLease(l.state.providerID) })
}

func (r *Runtime) releaseProviderLease(providerID string) {
	r.switchMu.Lock()
	defer r.switchMu.Unlock()
	if count := r.providerLeases[providerID]; count > 1 {
		r.providerLeases[providerID] = count - 1
	} else {
		delete(r.providerLeases, providerID)
	}
	r.releaseIdleProviderLocked(providerID)
	if !r.closed && !isPythonProvider(r.snapshot().providerID) && !r.hasPythonLeasesLocked() {
		r.releaseIdlePythonProvidersLocked()
		if err := r.sidecar.Stop(); err != nil {
			log.Printf("JFTrade market-data sidecar lease cleanup deferred: %v", err)
		}
	}
}

func (r *Runtime) releaseIdleProviderLocked(providerID string) {
	providerID = normalizeProviderID(providerID)
	if providerID == ProviderFutu || r.providerLeases[providerID] > 0 ||
		r.snapshot().providerID == providerID {
		return
	}
	delete(r.providerPool, providerID)
}

func (r *Runtime) releaseIdlePythonProvidersLocked() {
	r.releaseIdleProviderLocked(ProviderYFinance)
	r.releaseIdleProviderLocked(ProviderAKShare)
}

func (r *Runtime) hasPythonLeasesLocked() bool {
	return r.providerLeases[ProviderYFinance] > 0 || r.providerLeases[ProviderAKShare] > 0
}

// AvailableProviderDescriptors returns static selection metadata without
// starting Python helpers or contacting OpenD.
func (r *Runtime) AvailableProviderDescriptors(ctx context.Context) ([]marketdata.ProviderDescriptor, error) {
	futu, err := FutuProviderDescriptor(ctx)
	if err != nil {
		return nil, err
	}
	return []marketdata.ProviderDescriptor{
		futu,
		yfinanceintegration.ProviderDescriptor(),
		akshareintegration.ProviderDescriptor(),
	}, nil
}

func releaseProviderSubscriptions(
	ctx context.Context,
	subscriptions marketdata.SubscriptionReconciler,
) error {
	return subscriptions.ReconcileSubscriptions(ctx, nil)
}

func (r *Runtime) checkHealth(
	ctx context.Context,
	provider marketdata.Provider,
	requireReady bool,
) error {
	if r.healthCheck == nil {
		return waitForProviderHealth(ctx, provider, requireReady)
	}
	return r.healthCheck(ctx, provider, requireReady)
}

func waitForProviderHealth(
	ctx context.Context,
	provider marketdata.Provider,
	requireReady bool,
) error {
	if provider == nil {
		return fmt.Errorf("market-data provider is unavailable")
	}
	probeCtx, cancel := context.WithTimeout(ctx, providerActivationTimeout)
	defer cancel()
	var lastErr error
	retryDelay := providerHealthRetryDelay
	for {
		health, err := provider.Health(probeCtx)
		switch {
		case err == nil && health.Connected && providerReadinessAccepted(
			health.Readiness,
			requireReady,
		):
			return nil
		case err != nil:
			lastErr = err
		case health.Readiness == marketdata.ProviderReadinessFailed:
			if health.LastError != "" {
				return fmt.Errorf("provider runtime failed: %s", health.LastError)
			}
			return fmt.Errorf("provider runtime failed")
		case health.Readiness == marketdata.ProviderReadinessWarming:
			lastErr = fmt.Errorf("provider runtime is warming")
		default:
			lastErr = fmt.Errorf("provider reported disconnected")
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-probeCtx.Done():
			timer.Stop()
			if lastErr == nil {
				lastErr = probeCtx.Err()
			}
			return lastErr
		case <-timer.C:
		}
		retryDelay = nextProviderHealthRetryDelay(retryDelay)
	}
}

func providerReadinessAccepted(
	readiness marketdata.ProviderReadiness,
	requireReady bool,
) bool {
	return !requireReady || readiness == "" ||
		readiness == marketdata.ProviderReadinessReady
}

func nextProviderHealthRetryDelay(current time.Duration) time.Duration {
	if current <= 0 {
		return providerHealthRetryDelay
	}
	if current >= providerHealthMaxRetryDelay/2 {
		return providerHealthMaxRetryDelay
	}
	return current * 2
}

func (r *Runtime) resolveActivation(activation Activation) (runtimeState, error) {
	providerID := normalizeProviderID(activation.ProviderID)
	if r.providerPool == nil {
		r.providerPool = map[string]runtimeState{ProviderFutu: r.futu}
	}
	if providerID == ProviderFutu {
		return r.futu, nil
	}
	customEndpoint := strings.TrimSpace(activation.YFinanceEndpoint) != "" ||
		strings.TrimSpace(activation.AKShareEndpoint) != ""
	if !customEndpoint {
		if pooled, ok := r.providerPool[providerID]; ok {
			return pooled, nil
		}
	}
	var state runtimeState
	switch providerID {
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
		state = runtimeState{
			providerID: ProviderYFinance,
			provider:   provider,
			quotes:     provider,
		}
	case ProviderAKShare:
		endpoint := strings.TrimSpace(activation.AKShareEndpoint)
		if endpoint == "" {
			var err error
			endpoint, err = r.sidecar.EnsureStarted()
			if err != nil {
				return runtimeState{}, err
			}
		}
		provider, err := akshareintegration.NewProvider(endpoint)
		if err != nil {
			return runtimeState{}, err
		}
		state = runtimeState{
			providerID: ProviderAKShare,
			provider:   provider,
			quotes:     provider,
		}
	default:
		return runtimeState{}, fmt.Errorf("unsupported market-data provider %q", activation.ProviderID)
	}
	if !customEndpoint {
		r.providerPool[providerID] = state
	}
	return state, nil
}

func normalizeProviderID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isPythonProvider(providerID string) bool {
	switch normalizeProviderID(providerID) {
	case ProviderYFinance, ProviderAKShare:
		return true
	default:
		return false
	}
}

func runtimeSidecarCacheDir(options RuntimeOptions) string {
	if value := strings.TrimSpace(options.MarketDataCacheDir); value != "" {
		return value
	}
	return strings.TrimSpace(options.YFinanceCacheDir)
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
				fmt.Errorf("retry market-data sidecar cleanup: %w", retryErr),
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
	query marketdata.HistoricalCandlesQuery,
) (marketdata.CandlesResponse, error) {
	return r.snapshot().provider.GetHistoricalCandles(ctx, query)
}

func (r *Runtime) GetDepth(ctx context.Context, market, symbol string, num int) (marketdata.DepthResponse, error) {
	return r.snapshot().provider.GetDepth(ctx, market, symbol, num)
}

// News forwards instrument news to the active provider only when it offers the
// optional capability.
func (r *Runtime) News(ctx context.Context, market, symbol string, limit int) (marketdata.NewsResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.NewsSource)
	if !ok {
		return marketdata.NewsResponse{}, fmt.Errorf(
			"%w: active provider %q does not support instrument news",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.News(ctx, market, symbol, limit)
}

// CorporateActions forwards dividend/split reads to the active provider only
// when it offers the optional capability.
func (r *Runtime) CorporateActions(
	ctx context.Context,
	market string,
	symbol string,
	from time.Time,
	to time.Time,
) (marketdata.CorporateActionsResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.CorporateActionsSource)
	if !ok {
		return marketdata.CorporateActionsResponse{}, fmt.Errorf(
			"%w: active provider %q does not support corporate actions",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.CorporateActions(ctx, market, symbol, from, to)
}

// IndexConstituents forwards index constituent reads to the active provider
// only when it offers the optional capability.
func (r *Runtime) IndexConstituents(
	ctx context.Context,
	market string,
	symbol string,
	limit int,
) (marketdata.IndexConstituentsResponse, error) {
	state := r.snapshot()
	source, ok := state.provider.(marketdata.IndexConstituentsSource)
	if !ok {
		return marketdata.IndexConstituentsResponse{}, fmt.Errorf(
			"%w: active provider %q does not support index constituents",
			marketdata.ErrCapabilityUnsupported, state.providerID,
		)
	}
	return source.IndexConstituents(ctx, market, symbol, limit)
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

// FilterPushInstruments forwards per-instrument push availability from the
// active provider without rebuilding Collector during a provider switch.
func (r *Runtime) FilterPushInstruments(instrumentIDs []string) []string {
	source := r.snapshot().push
	if filter, ok := source.(marketdata.PushInstrumentFilter); ok {
		return filter.FilterPushInstruments(instrumentIDs)
	}
	return append([]string(nil), instrumentIDs...)
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

// ReconcileInactiveSubscriptions is called only by the collector background
// loop. It keeps Futu retirement progressing while a poll-only provider is
// active without making foreground subscriptions depend on old-provider
// cleanup.
func (r *Runtime) ReconcileInactiveSubscriptions(ctx context.Context) error {
	r.switchMu.Lock()
	defer r.switchMu.Unlock()
	if r.closed {
		return ErrRuntimeClosed
	}
	if r.snapshot().providerID == ProviderFutu || r.futu.subscriptions == nil {
		return nil
	}
	return r.futu.subscriptions.ReconcileSubscriptions(ctx, nil)
}

func (r *Runtime) SubscriptionState() map[string]any {
	reconciler := r.snapshot().subscriptions
	if reconciler == nil {
		return nil
	}
	return reconciler.SubscriptionState()
}

// HasFallbackSubscriptions forwards the active provider's per-instrument
// fallback state so provider health can remain honest in mixed push/fallback
// demand sets.
func (r *Runtime) HasFallbackSubscriptions() bool {
	reconciler := r.snapshot().subscriptions
	fallbacks, ok := reconciler.(marketdata.SubscriptionFallbackState)
	return ok && fallbacks.HasFallbackSubscriptions()
}
