package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

type BacktestMarketDataProviderSettings struct {
	ActiveProvider     jfsettings.ActiveMarketDataProvider
	AvailableProviders []marketdata.ProviderDescriptor
}

var (
	ErrMarketDataProviderInvalid = errors.New("active market-data provider must be futu, yfinance, or akshare")
	ErrProviderRuntimeUpdate     = errors.New("could not apply market-data provider settings")
)

func BacktestMarketDataProviderID(store Store) string {
	if extension, ok := store.(BacktestMarketDataProviderStore); ok {
		return string(extension.BacktestMarketDataProvider())
	}
	return string(store.ActiveMarketDataProvider())
}

func EnsureBacktestMarketDataProvider(store Store) error {
	if extension, ok := store.(BacktestMarketDataProviderUpgradeStore); ok {
		return extension.EnsureBacktestMarketDataProvider()
	}
	return nil
}

func (s *Service) GetActiveMarketDataProvider() jfsettings.ActiveMarketDataProvider {
	s.marketDataProviderMu.RLock()
	defer s.marketDataProviderMu.RUnlock()

	return s.store.ActiveMarketDataProvider()
}

func (s *Service) GetBacktestMarketDataProvider(
	ctx context.Context,
) (BacktestMarketDataProviderSettings, error) {
	s.backtestProviderMu.RLock()
	defer s.backtestProviderMu.RUnlock()
	provider := s.store.ActiveMarketDataProvider()
	if extension, ok := s.store.(BacktestMarketDataProviderStore); ok {
		provider = extension.BacktestMarketDataProvider()
	}
	var descriptors []marketdata.ProviderDescriptor
	if s.marketDataProviderCatalog != nil {
		var err error
		descriptors, err = s.marketDataProviderCatalog(ctx)
		if err != nil {
			return BacktestMarketDataProviderSettings{}, err
		}
	}
	return BacktestMarketDataProviderSettings{
		ActiveProvider: provider, AvailableProviders: descriptors,
	}, nil
}

func (s *Service) SaveBacktestMarketDataProvider(
	input jfsettings.ActiveMarketDataProvider,
) (BacktestMarketDataProviderSettings, error) {
	s.backtestProviderMu.Lock()
	defer s.backtestProviderMu.Unlock()
	extension, ok := s.store.(BacktestMarketDataProviderStore)
	if !ok {
		return BacktestMarketDataProviderSettings{}, fmt.Errorf("backtest market-data provider store is unavailable")
	}
	current := extension.BacktestMarketDataProvider()
	next, err := validateActiveMarketDataProvider(input)
	if err != nil {
		return BacktestMarketDataProviderSettings{ActiveProvider: current}, err
	}
	if next != current && s.sideEffects.OnBacktestProviderChanged != nil {
		if err := s.sideEffects.OnBacktestProviderChanged(next); err != nil {
			return BacktestMarketDataProviderSettings{ActiveProvider: current},
				fmt.Errorf("%w: %w", ErrProviderRuntimeUpdate, err)
		}
	}
	if err := extension.SaveBacktestMarketDataProvider(next); err != nil {
		return BacktestMarketDataProviderSettings{ActiveProvider: current}, err
	}
	result := BacktestMarketDataProviderSettings{ActiveProvider: next}
	if s.marketDataProviderCatalog != nil {
		result.AvailableProviders, _ = s.marketDataProviderCatalog(context.Background())
	}
	return result, nil
}

// SaveActiveMarketDataProvider persists and applies a provider switch. If the
// runtime cannot apply the switch, the previous selection is restored.
func (s *Service) SaveActiveMarketDataProvider(
	input jfsettings.ActiveMarketDataProvider,
) (jfsettings.ActiveMarketDataProvider, error) {
	s.marketDataProviderMu.Lock()
	defer s.marketDataProviderMu.Unlock()

	current := s.store.ActiveMarketDataProvider()
	next, err := validateActiveMarketDataProvider(input)
	if err != nil {
		return current, err
	}
	if err := s.store.SaveActiveMarketDataProvider(next); err != nil {
		return current, err
	}
	if next == current || s.sideEffects.OnProviderChanged == nil {
		return next, nil
	}
	if err := s.sideEffects.OnProviderChanged(next); err != nil {
		if rollbackErr := s.store.SaveActiveMarketDataProvider(current); rollbackErr != nil {
			return current, fmt.Errorf(
				"%w: %w; settings rollback failed: %w",
				ErrProviderRuntimeUpdate,
				err,
				rollbackErr,
			)
		}
		return current, fmt.Errorf("%w: %w", ErrProviderRuntimeUpdate, err)
	}
	return next, nil
}

func validateActiveMarketDataProvider(
	input jfsettings.ActiveMarketDataProvider,
) (jfsettings.ActiveMarketDataProvider, error) {
	provider := jfsettings.ActiveMarketDataProvider(strings.ToLower(strings.TrimSpace(string(input))))
	switch provider {
	case jfsettings.MarketDataProviderFutu,
		jfsettings.MarketDataProviderYFinance,
		jfsettings.MarketDataProviderAKShare:
		return provider, nil
	default:
		return "", ErrMarketDataProviderInvalid
	}
}
