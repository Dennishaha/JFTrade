package futu

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
)

const (
	minimumFutuSubscriptionAge = time.Minute
	subscriptionQuotaRefresh   = time.Minute
)

var subscriptionRetryDelays = [...]time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	30 * time.Second,
}

type physicalSubscriptionExchange interface {
	SubscribeBasicQuote(context.Context, string, bool) error
	UnsubscribeBasicQuote(context.Context, string) error
	SubscribeKLine(context.Context, string, bbgotypes.Interval) error
	UnsubscribeKLine(context.Context, string, bbgotypes.Interval) error
	QuerySubscriptionQuota(context.Context) (pkgfutu.SubscriptionQuota, error)
}

type physicalSubscriptionRef struct {
	key        string
	kind       string
	instrument string
	interval   bbgotypes.Interval
}

type physicalSubscriptionRecord struct {
	ref          physicalSubscriptionRef
	subscribedAt time.Time
	retryAt      time.Time
	failures     int
	lastError    string
}

type marketDataSubscriptionReconciler struct {
	reconcileMu sync.Mutex
	mu          sync.Mutex
	exchange    func() physicalSubscriptionExchange
	now         func() time.Time

	current          physicalSubscriptionExchange
	records          map[string]*physicalSubscriptionRecord
	desiredKeys      map[string]struct{}
	desiredCount     int
	quota            pkgfutu.SubscriptionQuota
	quotaCheckedAt   time.Time
	quotaLastError   string
	lastReconciledAt time.Time
}

func newMarketDataSubscriptionReconciler(exchange func() physicalSubscriptionExchange, now func() time.Time) *marketDataSubscriptionReconciler {
	if now == nil {
		now = time.Now
	}
	return &marketDataSubscriptionReconciler{
		exchange:    exchange,
		now:         now,
		records:     map[string]*physicalSubscriptionRecord{},
		desiredKeys: map[string]struct{}{},
	}
}

func (r *marketDataSubscriptionReconciler) ReconcileSubscriptions(ctx context.Context, desired []marketdata.InstrumentRef) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.reconcileMu.Lock()
	defer r.reconcileMu.Unlock()

	now := r.now().UTC()
	physical, logicalCount := desiredPhysicalSubscriptions(desired)
	r.mu.Lock()
	r.desiredCount = logicalCount
	r.desiredKeys = make(map[string]struct{}, len(physical))
	for key := range physical {
		r.desiredKeys[key] = struct{}{}
	}
	r.lastReconciledAt = now
	noPhysicalWork := len(physical) == 0 && len(r.records) == 0
	r.mu.Unlock()
	if noPhysicalWork {
		return nil
	}

	var exchange physicalSubscriptionExchange
	if r.exchange != nil {
		exchange = r.exchange()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if exchange != r.current {
		r.current = exchange
		r.records = map[string]*physicalSubscriptionRecord{}
		r.quotaCheckedAt = time.Time{}
	}
	if exchange == nil {
		if len(physical) == 0 {
			return nil
		}
		return fmt.Errorf("futu subscription exchange is unavailable")
	}

	var reconcileErrors []error
	keys := sortedPhysicalKeys(physical)
	for _, key := range keys {
		wanted := physical[key]
		record := r.records[key]
		if record != nil && !record.subscribedAt.IsZero() {
			record.retryAt = time.Time{}
			record.failures = 0
			record.lastError = ""
			continue
		}
		if record == nil {
			record = &physicalSubscriptionRecord{ref: wanted}
			r.records[key] = record
		}
		if now.Before(record.retryAt) {
			continue
		}
		if err := subscribePhysical(ctx, exchange, wanted); err != nil {
			recordSubscriptionFailure(record, now, err)
			reconcileErrors = append(reconcileErrors, fmt.Errorf("subscribe %s: %w", key, err))
			continue
		}
		record.subscribedAt = now
		record.retryAt = time.Time{}
		record.failures = 0
		record.lastError = ""
	}

	for _, key := range sortedRecordKeys(r.records) {
		if _, wanted := physical[key]; wanted {
			continue
		}
		record := r.records[key]
		if record.subscribedAt.IsZero() {
			delete(r.records, key)
			continue
		}
		if now.Before(record.subscribedAt.Add(minimumFutuSubscriptionAge)) || now.Before(record.retryAt) {
			continue
		}
		if err := unsubscribePhysical(ctx, exchange, record.ref); err != nil {
			recordSubscriptionFailure(record, now, err)
			reconcileErrors = append(reconcileErrors, fmt.Errorf("unsubscribe %s: %w", key, err))
			continue
		}
		delete(r.records, key)
	}

	r.refreshQuota(ctx, exchange, now)
	return errors.Join(reconcileErrors...)
}

func (r *marketDataSubscriptionReconciler) SubscriptionState() map[string]any {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := make([]map[string]any, 0, len(r.records))
	pending := 0
	active := 0
	for _, key := range sortedRecordKeys(r.records) {
		record := r.records[key]
		state := "retrying"
		var subscribedAt any
		var eligibleAt any
		if !record.subscribedAt.IsZero() {
			active++
			subscribedAt = record.subscribedAt.Format(time.RFC3339Nano)
			eligible := record.subscribedAt.Add(minimumFutuSubscriptionAge)
			eligibleAt = eligible.Format(time.RFC3339Nano)
			state = "active"
			_, desired := r.desiredKeys[record.ref.key]
			if !desired {
				state = "pending_unsubscribe"
				pending++
			}
		}
		entry := map[string]any{
			"key": record.ref.key, "kind": record.ref.kind, "instrumentId": record.ref.instrument,
			"brokerState": state, "subscribedAt": subscribedAt, "unsubscribeEligibleAt": eligibleAt,
			"lastError": nullableString(record.lastError),
		}
		if record.ref.interval != "" {
			entry["interval"] = string(record.ref.interval)
		} else {
			entry["interval"] = nil
		}
		entries = append(entries, entry)
	}
	return map[string]any{
		"desiredCount": r.desiredCount, "ownActiveCount": active, "pendingReleaseCount": pending,
		"totalUsedQuota": r.quota.TotalUsed, "remainQuota": r.quota.Remaining, "ownUsedQuota": r.quota.OwnUsed,
		"checkedAt": nullableTime(r.quotaCheckedAt), "lastError": nullableString(r.quotaLastError),
		"reconciledAt": nullableTime(r.lastReconciledAt), "entries": entries,
	}
}

func (r *marketDataSubscriptionReconciler) ResetPhysicalSubscriptions() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.current = nil
	r.records = map[string]*physicalSubscriptionRecord{}
	r.desiredKeys = map[string]struct{}{}
	r.quota = pkgfutu.SubscriptionQuota{}
	r.quotaCheckedAt = time.Time{}
	r.quotaLastError = ""
	r.mu.Unlock()
}

func desiredPhysicalSubscriptions(desired []marketdata.InstrumentRef) (map[string]physicalSubscriptionRef, int) {
	physical := map[string]physicalSubscriptionRef{}
	logical := map[string]struct{}{}
	for _, raw := range desired {
		market, symbol := normalizedInstrument(raw.Market, raw.Symbol)
		if market == "" || symbol == "" {
			continue
		}
		instrument := market + "." + symbol
		channel := strings.ToUpper(strings.TrimSpace(raw.Channel))
		if channel == "" {
			channel = "SNAPSHOT"
		}
		interval := bbgotypes.Interval(strings.ToLower(strings.TrimSpace(raw.Interval)))
		logicalKey := channel + ":" + instrument
		if interval != "" {
			logicalKey += ":" + string(interval)
		}
		logical[logicalKey] = struct{}{}
		basicKey := "BASIC:" + instrument
		physical[basicKey] = physicalSubscriptionRef{key: basicKey, kind: "BASIC", instrument: instrument}
		if channel == "KLINE" && interval != "" {
			klineKey := "KLINE:" + instrument + ":" + string(interval)
			physical[klineKey] = physicalSubscriptionRef{key: klineKey, kind: "KLINE", instrument: instrument, interval: interval}
		}
	}
	return physical, len(logical)
}

func normalizedInstrument(market, symbol string) (string, string) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if parts := strings.SplitN(symbol, ".", 2); len(parts) == 2 {
		if market == "" {
			market = parts[0]
		}
		symbol = parts[1]
	}
	return market, symbol
}

func subscribePhysical(ctx context.Context, exchange physicalSubscriptionExchange, ref physicalSubscriptionRef) error {
	if ref.kind == "KLINE" {
		return exchange.SubscribeKLine(ctx, ref.instrument, ref.interval)
	}
	return exchange.SubscribeBasicQuote(ctx, ref.instrument, true)
}

func unsubscribePhysical(ctx context.Context, exchange physicalSubscriptionExchange, ref physicalSubscriptionRef) error {
	if ref.kind == "KLINE" {
		return exchange.UnsubscribeKLine(ctx, ref.instrument, ref.interval)
	}
	return exchange.UnsubscribeBasicQuote(ctx, ref.instrument)
}

func recordSubscriptionFailure(record *physicalSubscriptionRecord, now time.Time, err error) {
	delay := subscriptionRetryDelay(record.failures)
	record.failures++
	record.retryAt = now.Add(delay)
	record.lastError = err.Error()
}

func subscriptionRetryDelay(failures int) time.Duration {
	if failures < 0 {
		failures = 0
	}
	if failures >= len(subscriptionRetryDelays) {
		return subscriptionRetryDelays[len(subscriptionRetryDelays)-1]
	}
	return subscriptionRetryDelays[failures]
}

func (r *marketDataSubscriptionReconciler) refreshQuota(ctx context.Context, exchange physicalSubscriptionExchange, now time.Time) {
	if !r.quotaCheckedAt.IsZero() && now.Sub(r.quotaCheckedAt) < subscriptionQuotaRefresh {
		return
	}
	quota, err := exchange.QuerySubscriptionQuota(ctx)
	r.quotaCheckedAt = now
	if err != nil {
		r.quotaLastError = err.Error()
		return
	}
	r.quota = quota
	r.quotaLastError = ""
}

func sortedPhysicalKeys(values map[string]physicalSubscriptionRef) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedRecordKeys(values map[string]*physicalSubscriptionRecord) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.Format(time.RFC3339Nano)
}
