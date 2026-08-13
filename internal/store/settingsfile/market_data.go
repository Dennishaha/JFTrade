package settingsfile

import (
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func (s *Store) ActiveMarketDataProvider() jfsettings.ActiveMarketDataProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.ActiveMarketDataProvider != nil {
		return NormalizeActiveMarketDataProvider(*s.data.ActiveMarketDataProvider)
	}
	return jfsettings.MarketDataProviderYFinance
}

func (s *Store) SaveActiveMarketDataProvider(input jfsettings.ActiveMarketDataProvider) error {
	normalized := NormalizeActiveMarketDataProvider(input)

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndPersistLocked(func() {
		s.data.ActiveMarketDataProvider = new(normalized)
	})
}

// BacktestMarketDataProvider returns the module-specific historical data
// source. Older settings files inherit the current global selection until the
// upgrade hook persists that value once.
func (s *Store) BacktestMarketDataProvider() jfsettings.ActiveMarketDataProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.BacktestMarketDataProvider != nil {
		return NormalizeActiveMarketDataProvider(*s.data.BacktestMarketDataProvider)
	}
	if s.data.ActiveMarketDataProvider != nil {
		return NormalizeActiveMarketDataProvider(*s.data.ActiveMarketDataProvider)
	}
	return jfsettings.MarketDataProviderYFinance
}

func (s *Store) SaveBacktestMarketDataProvider(input jfsettings.ActiveMarketDataProvider) error {
	normalized := NormalizeActiveMarketDataProvider(input)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndPersistLocked(func() {
		s.data.BacktestMarketDataProvider = new(normalized)
	})
}

// EnsureBacktestMarketDataProvider performs the one-time settings upgrade.
// It intentionally copies the persisted global provider under the same lock
// so subsequent global switches cannot change the backtest selection.
func (s *Store) EnsureBacktestMarketDataProvider() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.BacktestMarketDataProvider != nil {
		return nil
	}
	provider := jfsettings.MarketDataProviderYFinance
	if s.data.ActiveMarketDataProvider != nil {
		provider = NormalizeActiveMarketDataProvider(*s.data.ActiveMarketDataProvider)
	}
	return s.mutateAndPersistLocked(func() {
		s.data.BacktestMarketDataProvider = new(provider)
	})
}

func NormalizeActiveMarketDataProvider(input jfsettings.ActiveMarketDataProvider) jfsettings.ActiveMarketDataProvider {
	switch strings.ToLower(strings.TrimSpace(string(input))) {
	case string(jfsettings.MarketDataProviderYFinance):
		return jfsettings.MarketDataProviderYFinance
	case string(jfsettings.MarketDataProviderAKShare):
		return jfsettings.MarketDataProviderAKShare
	default:
		return jfsettings.MarketDataProviderFutu
	}
}
