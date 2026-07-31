package settings

import (
	"errors"
	"fmt"
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

var (
	ErrMarketDataProviderInvalid = errors.New("active market-data provider must be futu or yfinance")
	ErrProviderRuntimeUpdate     = errors.New("could not apply market-data provider settings")
)

func (s *Service) GetActiveMarketDataProvider() jfsettings.ActiveMarketDataProvider {
	s.marketDataProviderMu.RLock()
	defer s.marketDataProviderMu.RUnlock()

	return s.store.ActiveMarketDataProvider()
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
	case jfsettings.MarketDataProviderFutu, jfsettings.MarketDataProviderYFinance:
		return provider, nil
	default:
		return "", ErrMarketDataProviderInvalid
	}
}
