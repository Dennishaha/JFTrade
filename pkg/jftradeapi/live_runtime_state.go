package jftradeapi

import (
	"sync"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
)

type liveQuoteRefreshState struct {
	mu            sync.Mutex
	lastRefreshAt time.Time
	retryAfter    time.Time
	failureCount  int
	lastError     string
}

type liveMarketStreamState struct {
	mu           sync.Mutex
	stream       bbgotypes.Stream
	streamKey    string
	retryAfter   time.Time
	failureCount int
	lastError    string
}
