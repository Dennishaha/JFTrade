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
