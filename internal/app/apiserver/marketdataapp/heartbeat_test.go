package marketdataapp

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

const heartbeatTestStaleThreshold = 2500 * time.Millisecond

func TestLiveHeartbeatProjectsSamplesRetriesAndClientState(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	service := marketdata.NewService(nil)
	service.Seed(marketdata.Tick{
		InstrumentID: "US.FRESH",
		ObservedAt:   now.Add(-time.Second).Format(time.RFC3339Nano),
	})
	service.Seed(marketdata.Tick{
		InstrumentID: "US.STALE",
		ObservedAt:   now.Add(-(heartbeatTestStaleThreshold + time.Second)).Format(time.RFC3339Nano),
	})
	service.Seed(marketdata.Tick{InstrumentID: "US.INVALID", ObservedAt: "invalid"})
	state := marketdata.RuntimeState{
		LastRefreshAt:   now.Add(-time.Second),
		QuoteRetryAt:    now.Add(time.Minute),
		QuoteFailures:   2,
		QuoteLastError:  " quote failed ",
		StreamRetryAt:   now.Add(2 * time.Minute),
		StreamFailures:  3,
		StreamLastError: " stream failed ",
	}

	payload := liveHeartbeatAt(
		now,
		service,
		LiveClientStats{Connected: 2, Limit: 5, AtLimit: false},
		[]string{"US.MISSING", "US.STALE", "US.FRESH", "US.INVALID"},
		5*time.Second,
		heartbeatTestStaleThreshold,
		marketdata.CacheRetention,
		state,
	)
	assertHeartbeatEnvelope(t, payload, now)
	assertHeartbeatTransport(t, payload)
	assertHeartbeatRuntimeState(t, payload)
}

func assertHeartbeatEnvelope(t *testing.T, payload map[string]any, now time.Time) {
	t.Helper()
	if payload["type"] != "heartbeat" ||
		payload["at"] != now.Format(time.RFC3339Nano) ||
		payload["intervalMs"] != int64(5000) ||
		payload["stale"] != true {
		t.Fatalf("heartbeat envelope = %#v", payload)
	}
	wantReasons := []any{
		"market-data-samples-stale",
		"live-quote-backoff",
		"live-stream-backoff",
		"live-stream-disconnected",
	}
	if reasons := payload["staleReasons"].([]any); !reflect.DeepEqual(reasons, wantReasons) {
		t.Fatalf("stale reasons = %#v", reasons)
	}
	clients := payload["liveClients"].(map[string]any)
	if clients["connected"] != 2 || clients["limit"] != 5 || clients["atLimit"] != false {
		t.Fatalf("live clients = %#v", clients)
	}
}

func assertHeartbeatTransport(t *testing.T, payload map[string]any) {
	t.Helper()
	transport := payload["transport"].(map[string]any)
	if transport["mode"] != "snapshot-poll-fallback" ||
		transport["activeInstruments"] != 4 ||
		transport["freshInstruments"] != 1 ||
		transport["staleInstruments"] != 3 ||
		transport["sampleFreshnessMs"] != heartbeatTestStaleThreshold.Milliseconds() ||
		transport["latestObservedAt"] == nil {
		t.Fatalf("heartbeat transport = %#v", transport)
	}
}

func assertHeartbeatRuntimeState(t *testing.T, payload map[string]any) {
	t.Helper()
	quote := payload["liveQuote"].(map[string]any)
	if quote["lastRefreshAt"] == nil ||
		quote["backoffActive"] != true ||
		quote["retryAfter"] == nil ||
		quote["failureCount"] != 2 ||
		*quote["lastError"].(*string) != "quote failed" {
		t.Fatalf("live quote = %#v", quote)
	}
	stream := payload["liveStream"].(map[string]any)
	if stream["supported"] != true ||
		stream["connected"] != false ||
		stream["backoffActive"] != true ||
		stream["retryAfter"] == nil ||
		stream["failureCount"] != 3 ||
		*stream["lastError"].(*string) != "stream failed" {
		t.Fatalf("live stream = %#v", stream)
	}
}

func TestLiveHeartbeatUsesPollOnlyFreshnessAndIgnoresStreamStaleReasons(t *testing.T) {
	policy := marketdata.QuotePollingPolicy{
		Interval: 15 * time.Second,
		Timeout:  3 * time.Second,
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider: &providerStub{id: ProviderFutu},
		FutuQuotes:   &heartbeatQuoteSourceStub{policy: policy},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	service := marketdata.NewService(runtime)
	now := time.Now().UTC()
	state := marketdata.RuntimeState{
		StreamRetryAt:   now.Add(time.Minute),
		StreamFailures:  4,
		StreamLastError: "push unavailable",
	}

	payload := liveHeartbeatAt(
		now,
		service,
		LiveClientStats{},
		[]string{"US.MISSING"},
		time.Second,
		heartbeatTestStaleThreshold,
		marketdata.CacheRetention,
		state,
	)
	transport := payload["transport"].(map[string]any)
	if transport["mode"] != "snapshot-poll-delayed" ||
		transport["sampleFreshnessMs"] != (policy.Interval+policy.Timeout).Milliseconds() {
		t.Fatalf("poll-only transport = %#v", transport)
	}
	if reasons := payload["staleReasons"].([]any); !reflect.DeepEqual(
		reasons,
		[]any{"market-data-samples-stale"},
	) {
		t.Fatalf("poll-only stale reasons = %#v", reasons)
	}
	stream := payload["liveStream"].(map[string]any)
	if stream["supported"] != false || stream["backoffActive"] != true {
		t.Fatalf("poll-only live stream = %#v", stream)
	}
}

func TestLiveHeartbeatPolicyBoundaries(t *testing.T) {
	if got := SampleFreshness(nil, heartbeatTestStaleThreshold); got != heartbeatTestStaleThreshold {
		t.Fatalf("nil-runtime freshness = %s", got)
	}
	if mode := liveHeartbeatTransportMode(0, true, true); mode != "push-stream" {
		t.Fatalf("connected transport mode = %q", mode)
	}
	if mode := liveHeartbeatTransportMode(0, false, false); mode != "idle" {
		t.Fatalf("idle poll-only transport mode = %q", mode)
	}
	if parsed := parseHeartbeatTime("2026-07-29 12:34:56"); parsed.IsZero() {
		t.Fatal("legacy heartbeat timestamp was not parsed")
	}
	if parsed := parseHeartbeatTime("invalid"); !parsed.IsZero() {
		t.Fatalf("invalid heartbeat timestamp = %s", parsed)
	}
	now := time.Now().UTC()
	if retry := liveHeartbeatRetry(now.Add(-time.Second), now); retry.active || retry.retryAfter == nil {
		t.Fatalf("expired retry state = %#v", retry)
	}
	if liveHeartbeatRefreshTime(time.Time{}) != nil ||
		liveHeartbeatOptionalString("  ") != nil {
		t.Fatal("empty optional heartbeat fields are non-nil")
	}
	if payload := LiveHeartbeat(nil, LiveClientStats{}, nil, time.Second, time.Second, time.Minute); payload["stale"] != false {
		t.Fatalf("nil-service heartbeat = %#v", payload)
	}
}

type heartbeatQuoteSourceStub struct {
	policy marketdata.QuotePollingPolicy
}

func (s *heartbeatQuoteSourceStub) QueryTickers(
	context.Context,
	[]string,
) (map[string]marketdata.Tick, error) {
	return map[string]marketdata.Tick{}, nil
}

func (s *heartbeatQuoteSourceStub) QuotePollingPolicy() marketdata.QuotePollingPolicy {
	return s.policy
}
