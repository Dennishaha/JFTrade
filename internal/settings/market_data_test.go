package settings

import (
	"errors"
	"strings"
	"testing"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

type marketDataErrorStore struct {
	*fakeStore
	providerErrors []error
}

type backtestMarketDataStore struct {
	*fakeStore
	backtestProvider jfsettings.ActiveMarketDataProvider
	saveErr          error
}

func (s *backtestMarketDataStore) BacktestMarketDataProvider() jfsettings.ActiveMarketDataProvider {
	return s.backtestProvider
}

func (s *backtestMarketDataStore) SaveBacktestMarketDataProvider(
	provider jfsettings.ActiveMarketDataProvider,
) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.backtestProvider = provider
	return nil
}

func (s *marketDataErrorStore) SaveActiveMarketDataProvider(
	input jfsettings.ActiveMarketDataProvider,
) error {
	if err := popSettingsError(&s.providerErrors); err != nil {
		return err
	}
	s.activeProvider = input
	return nil
}

func popSettingsError(errorsList *[]error) error {
	if len(*errorsList) == 0 {
		return nil
	}
	err := (*errorsList)[0]
	*errorsList = (*errorsList)[1:]
	return err
}

func TestMarketDataProviderSettingsNormalizeAndApply(t *testing.T) {
	store := &fakeStore{}
	var applied []jfsettings.ActiveMarketDataProvider
	service := NewService(store, WithSideEffects(SideEffects{
		OnProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
			applied = append(applied, provider)
			return nil
		},
	}))

	provider, err := service.SaveActiveMarketDataProvider(" YFINANCE ")
	if err != nil || provider != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("SaveActiveMarketDataProvider = %q, %v", provider, err)
	}
	if got := store.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("stored provider = %q, want yfinance", got)
	}
	if len(applied) != 1 || applied[0] != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("provider callbacks = %#v", applied)
	}

	// Saving the already-active source is idempotent and does not restart the
	// runtime side effect.
	if _, err := service.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderYFinance); err != nil {
		t.Fatalf("idempotent provider save: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("idempotent provider save triggered callbacks = %#v", applied)
	}

	provider, err = service.SaveActiveMarketDataProvider(" futu ")
	if err != nil || provider != jfsettings.MarketDataProviderFutu {
		t.Fatalf("SaveActiveMarketDataProvider futu = %q, %v", provider, err)
	}
	if len(applied) != 2 || applied[1] != jfsettings.MarketDataProviderFutu {
		t.Fatalf("futu provider callbacks = %#v", applied)
	}

	if result, err := service.SaveActiveMarketDataProvider("invalid"); !errors.Is(err, ErrMarketDataProviderInvalid) || result != jfsettings.MarketDataProviderFutu {
		t.Fatalf("invalid provider result = %q, err=%v", result, err)
	}
}

func TestMarketDataProviderRetriesDegradedCurrentSelection(t *testing.T) {
	store := &fakeStore{activeProvider: jfsettings.MarketDataProviderAKShare}
	needsActivation := true
	var applied []jfsettings.ActiveMarketDataProvider
	service := NewService(store, WithSideEffects(SideEffects{
		ProviderNeedsActivation: func(provider jfsettings.ActiveMarketDataProvider) bool {
			return needsActivation && provider == jfsettings.MarketDataProviderAKShare
		},
		OnProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
			applied = append(applied, provider)
			needsActivation = false
			return nil
		},
	}))

	provider, err := service.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderAKShare)
	if err != nil || provider != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("retry current provider = %q, err=%v", provider, err)
	}
	if len(applied) != 1 || applied[0] != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("retry callbacks = %#v", applied)
	}

	if _, err := service.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderAKShare); err != nil {
		t.Fatalf("healthy idempotent provider save: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("healthy idempotent provider save retried = %#v", applied)
	}

	retryErr := errors.New("retry failed")
	service = NewService(store, WithSideEffects(SideEffects{
		ProviderNeedsActivation: func(jfsettings.ActiveMarketDataProvider) bool { return true },
		OnProviderChanged: func(jfsettings.ActiveMarketDataProvider) error {
			return retryErr
		},
	}))
	provider, err = service.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderAKShare)
	if provider != jfsettings.MarketDataProviderAKShare || !errors.Is(err, ErrProviderRuntimeUpdate) ||
		!errors.Is(err, retryErr) {
		t.Fatalf("failed provider retry = %q, err=%v", provider, err)
	}
}

func TestMarketDataProviderSettingsAcceptAKShare(t *testing.T) {
	store := &fakeStore{activeProvider: jfsettings.MarketDataProviderYFinance}
	var applied jfsettings.ActiveMarketDataProvider
	service := NewService(store, WithSideEffects(SideEffects{
		OnProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
			applied = provider
			return nil
		},
	}))
	provider, err := service.SaveActiveMarketDataProvider(" AKSHARE ")
	if err != nil || provider != jfsettings.MarketDataProviderAKShare ||
		store.ActiveMarketDataProvider() != jfsettings.MarketDataProviderAKShare ||
		applied != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("AKShare provider save = %q stored=%q applied=%q err=%v",
			provider, store.ActiveMarketDataProvider(), applied, err)
	}
}

func TestBacktestProviderIsPreparedBeforeAtomicPersistence(t *testing.T) {
	store := &backtestMarketDataStore{
		fakeStore:        &fakeStore{activeProvider: jfsettings.MarketDataProviderFutu},
		backtestProvider: jfsettings.MarketDataProviderYFinance,
	}
	prepareErr := errors.New("provider health failed")
	var prepared []jfsettings.ActiveMarketDataProvider
	service := NewService(store, WithSideEffects(SideEffects{
		OnBacktestProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
			prepared = append(prepared, provider)
			if got := store.BacktestMarketDataProvider(); got != jfsettings.MarketDataProviderYFinance {
				t.Fatalf("provider persisted before preparation = %q", got)
			}
			return prepareErr
		},
	}))

	result, err := service.SaveBacktestMarketDataProvider(jfsettings.MarketDataProviderAKShare)
	if !errors.Is(err, ErrProviderRuntimeUpdate) || result.ActiveProvider != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("failed preparation result = %+v, err=%v", result, err)
	}
	if got := store.BacktestMarketDataProvider(); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("failed preparation changed provider = %q", got)
	}
	if len(prepared) != 1 || prepared[0] != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("prepared providers = %#v", prepared)
	}

	service = NewService(store, WithSideEffects(SideEffects{
		OnBacktestProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
			prepared = append(prepared, provider)
			return nil
		},
	}))
	result, err = service.SaveBacktestMarketDataProvider(jfsettings.MarketDataProviderAKShare)
	if err != nil || result.ActiveProvider != jfsettings.MarketDataProviderAKShare ||
		store.BacktestMarketDataProvider() != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("successful preparation result = %+v, stored=%q, err=%v",
			result, store.BacktestMarketDataProvider(), err)
	}
}

func TestMarketDataProviderRuntimeRollback(t *testing.T) {
	runtimeErr := errors.New("sidecar startup failed")
	store := &fakeStore{activeProvider: jfsettings.MarketDataProviderYFinance}
	service := NewService(store, WithSideEffects(SideEffects{
		OnProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
			if provider != jfsettings.MarketDataProviderFutu {
				t.Fatalf("side effect provider = %q, want futu", provider)
			}
			return runtimeErr
		},
	}))

	result, err := service.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu)
	if !errors.Is(err, ErrProviderRuntimeUpdate) || result != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("provider runtime rollback = %q, err=%v", result, err)
	}
	if got := store.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("provider after runtime rollback = %q", got)
	}
}

func TestMarketDataProviderReportsPersistenceAndRollbackFailures(t *testing.T) {
	persistErr := errors.New("persist failed")
	rollbackErr := errors.New("rollback failed")

	store := &marketDataErrorStore{fakeStore: &fakeStore{
		activeProvider: jfsettings.MarketDataProviderYFinance,
	}, providerErrors: []error{persistErr}}
	service := NewService(store)
	if result, err := service.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); !errors.Is(err, persistErr) || result != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("provider persistence error = %q, err=%v", result, err)
	}
	if got := store.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("persistence failure changed provider = %q", got)
	}

	store.providerErrors = []error{nil, rollbackErr}
	service = NewService(store, WithSideEffects(SideEffects{
		OnProviderChanged: func(jfsettings.ActiveMarketDataProvider) error {
			return persistErr
		},
	}))
	result, err := service.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu)
	if !errors.Is(err, ErrProviderRuntimeUpdate) || !strings.Contains(err.Error(), "rollback failed") ||
		result != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("provider rollback error = %q, err=%v", result, err)
	}
}

func TestMarketDataProviderReadsWaitForRuntimeRollback(t *testing.T) {
	runtimeErr := errors.New("provider switch failed")
	store := &fakeStore{activeProvider: jfsettings.MarketDataProviderYFinance}
	sideEffectStarted := make(chan struct{})
	releaseSideEffect := make(chan struct{})
	service := NewService(store, WithSideEffects(SideEffects{
		OnProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
			if persisted := store.ActiveMarketDataProvider(); persisted != jfsettings.MarketDataProviderFutu {
				t.Errorf("side effect provider = %q, want futu", persisted)
			}
			close(sideEffectStarted)
			<-releaseSideEffect
			return runtimeErr
		},
	}))

	saveDone := make(chan error, 1)
	go func() {
		_, err := service.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu)
		saveDone <- err
	}()
	<-sideEffectStarted

	providerRead := make(chan jfsettings.ActiveMarketDataProvider, 1)
	go func() { providerRead <- service.GetActiveMarketDataProvider() }()
	assertMarketDataReadBlocked(t, providerRead)

	close(releaseSideEffect)
	if err := <-saveDone; !errors.Is(err, ErrProviderRuntimeUpdate) {
		t.Fatalf("SaveActiveMarketDataProvider error = %v", err)
	}
	if got := <-providerRead; got != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("provider after rollback = %q", got)
	}
}

func assertMarketDataReadBlocked[T any](t *testing.T, result <-chan T) {
	t.Helper()
	select {
	case <-result:
		t.Fatal("settings read completed while provider side effect was in progress")
	case <-time.After(50 * time.Millisecond):
	}
}
