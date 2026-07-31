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
