package marketdata

import "errors"

var (
	// ErrProviderChanged rejects results produced by a provider generation that
	// is no longer active.
	ErrProviderChanged = errors.New("market-data provider changed")
	// ErrCapabilityUnsupported identifies a valid request that the active
	// provider cannot supply.
	ErrCapabilityUnsupported = errors.New("market-data capability is unsupported")
	// ErrProviderWarming indicates that the selected provider process is healthy
	// but its heavy runtime dependencies are still loading.
	ErrProviderWarming = errors.New("market-data provider is warming")
	// ErrManagedSubscriptionsActive prevents a provider change from moving
	// running in-process consumers, such as live strategies, onto another data
	// source.
	ErrManagedSubscriptionsActive = errors.New(
		"stop all live strategies before changing the market-data provider",
	)
	// ErrManagedSubscriptionsUnavailable prevents a live in-process consumer
	// from acquiring a lease from a poll-only provider.
	ErrManagedSubscriptionsUnavailable = errors.New(
		"active market-data provider does not support live managed subscriptions",
	)
)
