// Package marketdata 提供行情数据门面层。Service 持有行情缓存和 HTTP consumer
// subscription registry，并将行情快照、K线、深度及实时行情能力抽象为与传输/
// 券商无关的接口。
//
// 设计约束：
//   - 零 protobuf 依赖
//   - 零 gin/HTTP 框架依赖
//   - 零券商依赖（Futu/broker）
//   - 固定数据面结构使用 broker-neutral DTO
package marketdata

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

// ──────────────────────────────────────────────────────────────────────────────
// Provider 接口
// ──────────────────────────────────────────────────────────────────────────────

// Provider 行情能力接口——零 protobuf、零 HTTP 框架、零券商依赖。
type Provider interface {
	// ── 能力描述 ──
	Descriptor(ctx context.Context) (ProviderDescriptor, error)

	// ── 快照查询 ──
	GetMarkets(ctx context.Context) ([]MarketProfile, error)
	GetSecurityDetails(ctx context.Context, market, symbol string) (SecurityDetails, error)
	LookupInstrument(ctx context.Context, market, code string) ([]InstrumentCandidate, error)
	SearchInstruments(ctx context.Context, query string, limit int) ([]InstrumentCandidate, error)
	QuerySnapshot(ctx context.Context, instrumentID string) (*Tick, error)
	QueryTicker(ctx context.Context, instrumentID string) (*Tick, error)
	GetHistoricalCandles(ctx context.Context, market, symbol, period string, limit int, fromTime, toTime string) (CandlesResponse, error)
	GetDepth(ctx context.Context, market, symbol string, num int) (DepthResponse, error)

	// ── 工具方法 ──
	NormalizeInstrument(ctx context.Context, input map[string]any) (map[string]any, error)

	// ── 生命周期 ──
	Health(ctx context.Context) (HealthStatus, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// 数据类型
// ──────────────────────────────────────────────────────────────────────────────

// MarketProfile 市场档案（map 格式，字段由 Provider 实现决定）。
type MarketProfile map[string]any

// SecurityDetails 证券详情。
type SecurityDetails map[string]any

// MarketSnapshot 行情快照。
type MarketSnapshot map[string]any

// CandlesResponse K线响应。
type CandlesResponse map[string]any

// DepthResponse 盘口深度响应。
type DepthResponse map[string]any

// InstrumentRef 行情标的引用。
type InstrumentRef struct {
	Channel  string `json:"channel,omitempty"`
	Market   string `json:"market"`
	Symbol   string `json:"symbol"`
	Interval string `json:"interval,omitempty"`
}

// SubscriptionResult 订阅结果。
type SubscriptionResult map[string]any

// HeartbeatResult 心跳结果。
type HeartbeatResult map[string]any

// SubscriptionsSnapshot 订阅快照。
type SubscriptionsSnapshot map[string]any

// TicksResponse Tick 数据响应。
type TicksResponse map[string]any

// HealthStatus 行情健康状态。
type HealthStatus struct {
	Connected   bool   `json:"connected"`
	StreamMode  string `json:"streamMode"`
	ActiveCount int    `json:"activeCount"`
}

// ProviderDescriptor describes the active market-data provider without leaking
// broker SDK or protocol implementation details into transport/UI layers.
type ProviderDescriptor struct {
	ProviderID       string               `json:"providerId"`
	DisplayName      string               `json:"displayName"`
	BrokerID         string               `json:"brokerId,omitempty"`
	Source           string               `json:"source"`
	DefaultMarket    string               `json:"defaultMarket"`
	SupportedMarkets []string             `json:"supportedMarkets"`
	Transports       []string             `json:"transports"`
	Capabilities     ProviderCapabilities `json:"capabilities"`
	Constraints      ProviderConstraints  `json:"constraints"`
	Notes            []string             `json:"notes,omitempty"`
}

// ProviderCapabilities records the data-plane features a provider can supply.
type ProviderCapabilities struct {
	Snapshots         bool     `json:"snapshots"`
	StreamingQuotes   bool     `json:"streamingQuotes"`
	StreamingDepth    bool     `json:"streamingDepth"`
	HistoricalCandles bool     `json:"historicalCandles"`
	TickCandles       bool     `json:"tickCandles"`
	OrderBookDepth    bool     `json:"orderBookDepth"`
	InstrumentSearch  bool     `json:"instrumentSearch"`
	ExtendedHours     bool     `json:"extendedHours"`
	CandleIntervals   []string `json:"candleIntervals"`
	OrderBookLevels   []int    `json:"orderBookLevels"`
	Sessions          []string `json:"sessions"`
}

// ProviderConstraints records operational limits and setup prerequisites.
type ProviderConstraints struct {
	RequiresOpenD           bool `json:"requiresOpenD"`
	RequiresMarketDataRight bool `json:"requiresMarketDataRight"`
	UsesSubscriptionQuota   bool `json:"usesSubscriptionQuota"`
}

// ProviderStatusResponse combines static provider capability metadata with
// current runtime health and active demand information.
type ProviderStatusResponse struct {
	Descriptor    ProviderDescriptor    `json:"descriptor"`
	Health        HealthStatus          `json:"health"`
	Runtime       RuntimeState          `json:"runtime"`
	Subscriptions SubscriptionsSnapshot `json:"subscriptions"`
	CheckedAt     string                `json:"checkedAt"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Service 门面
// ──────────────────────────────────────────────────────────────────────────────

// Service 行情业务门面。
type Service struct {
	provider                Provider
	resolver                *MarketSubsetInstrumentResolver
	cache                   *Cache
	subscriptions           *subscriptionRegistry
	collector               *Collector
	subscriptionLifecycleMu sync.Mutex
	subscriptionMu          sync.RWMutex
	reconciler              SubscriptionReconciler
	additionalDemands       []DemandSource
}

// NewService 创建行情服务。
func NewService(provider Provider) *Service {
	return &Service{
		provider:      provider,
		resolver:      NewMarketSubsetInstrumentResolver(provider),
		cache:         NewCache(),
		subscriptions: newSubscriptionRegistry(),
	}
}

// ProviderStatus returns active provider metadata plus runtime state.
func (s *Service) ProviderStatus(ctx context.Context) (ProviderStatusResponse, error) {
	descriptor, err := s.provider.Descriptor(ctx)
	if err != nil {
		return ProviderStatusResponse{}, err
	}
	health, err := s.Health(ctx)
	if err != nil {
		return ProviderStatusResponse{}, err
	}
	subscriptions, err := s.GetSubscriptions(ctx)
	if err != nil {
		return ProviderStatusResponse{}, err
	}
	return ProviderStatusResponse{
		Descriptor:    descriptor,
		Health:        health,
		Runtime:       s.RuntimeState(),
		Subscriptions: subscriptions,
		CheckedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// StartCollector starts the active-demand marketdata runtime.
func (s *Service) StartCollector(quotes QuoteSource, push PushSource, handler PushTickHandler, demands ...DemandSource) {
	if s == nil {
		return
	}
	if s.collector != nil {
		jftradeErr1 := s.collector.Close()
		besteffort.LogError(jftradeErr1)
	}
	s.collector = NewCollector(s.cache, quotes, push, handler, CollectorOptions{})
	allDemands := []DemandSource{DemandSourceFunc(s.subscriptions.activeInstruments)}
	allDemands = append(allDemands, demands...)
	s.subscriptionMu.Lock()
	s.additionalDemands = append([]DemandSource(nil), demands...)
	s.subscriptionMu.Unlock()
	s.collector.SetDemandSources(allDemands...)
	s.subscriptionMu.RLock()
	reconciler := s.reconciler
	s.subscriptionMu.RUnlock()
	if reconciler != nil {
		s.collector.SetSubscriptionReconciler(SubscriptionDemandSourceFunc(s.activeSubscriptionDemand), reconciler)
	}
}

// SetSubscriptionReconciler installs the broker-specific physical subscription
// driver while keeping desired ownership in the broker-neutral service.
func (s *Service) SetSubscriptionReconciler(reconciler SubscriptionReconciler) {
	if s == nil {
		return
	}
	s.subscriptionMu.Lock()
	s.reconciler = reconciler
	s.subscriptionMu.Unlock()
	if s.collector != nil {
		s.collector.SetSubscriptionReconciler(SubscriptionDemandSourceFunc(s.activeSubscriptionDemand), reconciler)
	}
}

func (s *Service) WakeCollector() {
	if s != nil && s.collector != nil {
		s.collector.Wake()
	}
}

func (s *Service) ResetCollector() {
	if s != nil && s.collector != nil {
		s.collector.Reset()
	}
}

func (s *Service) ResumeCollector() {
	if s != nil && s.collector != nil {
		s.collector.Resume()
	}
}

func (s *Service) RuntimeState() RuntimeState {
	if s == nil || s.collector == nil {
		return RuntimeState{}
	}
	return s.collector.State()
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	var collectorErr error
	if s.collector != nil {
		collectorErr = s.collector.Close()
	}
	if err := s.reconcileDesired(context.Background(), nil); err != nil {
		log.Printf("marketdata final subscription reconciliation failed: %v", err)
	}
	return collectorErr
}

// GetMarkets 返回可用市场列表。
func (s *Service) GetMarkets(ctx context.Context) ([]MarketProfile, error) {
	return s.provider.GetMarkets(ctx)
}

// GetSecurityDetails 返回证券详情。
func (s *Service) GetSecurityDetails(ctx context.Context, market, symbol string) (SecurityDetails, error) {
	return s.provider.GetSecurityDetails(ctx, market, symbol)
}

// ResolveInstrument performs a qualified exact lookup or an unqualified
// cross-market code/name search.
func (s *Service) ResolveInstrument(ctx context.Context, requestedMarket, query string, limit int) (InstrumentResolution, error) {
	if s == nil || s.resolver == nil {
		return InstrumentResolution{}, fmt.Errorf("market-data instrument resolver is unavailable")
	}
	return s.resolver.Resolve(ctx, requestedMarket, query, limit)
}

// GetSnapshot 返回最新行情快照。
func (s *Service) GetSnapshot(ctx context.Context, market, symbol string, refresh bool) (MarketSnapshot, error) {
	market, symbol, instrumentID := normalizeInstrument(market, symbol)
	if err := s.requireBasicSubscriptionDemand(market, symbol, "SNAPSHOT"); err != nil {
		return nil, err
	}
	sample := (*Tick)(nil)
	if !refresh {
		sample = s.cache.Latest(instrumentID, TickFreshness)
	}
	fromCache := sample != nil
	if sample == nil {
		var err error
		sample, err = s.provider.QuerySnapshot(ctx, instrumentID)
		if err != nil {
			return nil, err
		}
		if sample != nil {
			sample = s.Ingest(*sample)
		}
	}
	if sample == nil {
		return nil, fmt.Errorf("no snapshot available for %s", instrumentID)
	}
	return SnapshotResponseDTO{
		Instrument: InstrumentDTO{Market: market, Symbol: symbol, InstrumentID: instrumentID},
		Snapshot:   SnapshotJSON(sample),
		Source:     sample.Source,
		ResolvedAt: sample.ObservedAt,
		FromCache:  fromCache,
	}.JSON(), nil
}

// GetCandles 返回 K 线数据。
func (s *Service) GetCandles(ctx context.Context, market, symbol, period string, limit int, fromTime, toTime string) (CandlesResponse, error) {
	period = strings.ToLower(strings.TrimSpace(period))
	if period == "" {
		period = "1m"
	}
	if period != "tick" {
		return s.provider.GetHistoricalCandles(ctx, market, symbol, period, limit, fromTime, toTime)
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	market, symbol, instrumentID := normalizeInstrument(market, symbol)
	if err := s.requireBasicSubscriptionDemand(market, symbol, "TICK"); err != nil {
		return nil, err
	}
	fromCache := s.cache.Latest(instrumentID, TickFreshness) != nil
	if !fromCache {
		sample, err := s.provider.QueryTicker(ctx, instrumentID)
		if err != nil {
			candles := s.TickCandles(instrumentID, fromTime, toTime, limit)
			if len(candles) == 0 {
				return nil, err
			}
			return tickCandlesResponse(market, symbol, instrumentID, period, limit, candles, true), nil
		}
		if sample != nil {
			s.Ingest(*sample)
		}
	}
	candles := s.TickCandles(instrumentID, fromTime, toTime, limit)
	return tickCandlesResponse(market, symbol, instrumentID, period, limit, candles, fromCache), nil
}

func (s *Service) requireBasicSubscriptionDemand(market, symbol, readChannel string) error {
	if s == nil {
		return NewSubscriptionRequiredError(readChannel, market, symbol, "")
	}
	s.subscriptionMu.RLock()
	managed := s.reconciler != nil
	s.subscriptionMu.RUnlock()
	if !managed {
		return nil
	}
	market, symbol = normalizeSubscriptionInstrument(market, symbol)
	for _, ref := range s.activeSubscriptionDemand() {
		refMarket, refSymbol := normalizeSubscriptionInstrument(ref.Market, ref.Symbol)
		if refMarket != market || refSymbol != symbol {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(ref.Channel)) {
		case "", "SNAPSHOT", "TICK", "KLINE":
			return nil
		}
	}
	return NewSubscriptionRequiredError(readChannel, market, symbol, "")
}

// GetDepth 返回盘口深度数据。
func (s *Service) GetDepth(ctx context.Context, market, symbol string, num int) (DepthResponse, error) {
	if err := s.requireOrderBookSubscriptionDemand(market, symbol); err != nil {
		return nil, err
	}
	return s.provider.GetDepth(ctx, market, symbol, num)
}

func (s *Service) requireOrderBookSubscriptionDemand(market, symbol string) error {
	if s == nil {
		return NewSubscriptionRequiredError("ORDER_BOOK", market, symbol, "")
	}
	s.subscriptionMu.RLock()
	managed := s.reconciler != nil
	s.subscriptionMu.RUnlock()
	if !managed {
		return nil
	}
	market, symbol = normalizeSubscriptionInstrument(market, symbol)
	for _, ref := range s.activeSubscriptionDemand() {
		refMarket, refSymbol := normalizeSubscriptionInstrument(ref.Market, ref.Symbol)
		if refMarket == market && refSymbol == symbol && strings.EqualFold(strings.TrimSpace(ref.Channel), "ORDER_BOOK") {
			return nil
		}
	}
	return NewSubscriptionRequiredError("ORDER_BOOK", market, symbol, "")
}

// AcquireSubscription 申请行情订阅。
func (s *Service) AcquireSubscription(ctx context.Context, consumerID string, instruments []InstrumentRef) (SubscriptionResult, error) {
	s.subscriptionLifecycleMu.Lock()
	defer s.subscriptionLifecycleMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSubscriptionRefs(instruments); err != nil {
		return nil, err
	}
	_, rollback := s.subscriptions.acquireWithMode(consumerID, instruments, false)
	if err := s.reconcileSubscriptions(ctx); err != nil {
		s.subscriptions.restore(rollback)
		_ = s.reconcileSubscriptions(context.Background())
		return nil, err
	}
	s.WakeCollector()
	snapshot, _ := s.GetSubscriptions(ctx)
	return SubscriptionResult(snapshot), nil
}

// AcquireManagedSubscription creates a non-expiring lease for an in-process
// consumer such as a running strategy. The lease is rolled back if the broker
// cannot establish the requested subscriptions.
func (s *Service) AcquireManagedSubscription(ctx context.Context, consumerID string, instruments []InstrumentRef) (*ManagedSubscription, error) {
	if s == nil {
		return nil, fmt.Errorf("market-data service is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSubscriptionRefs(instruments); err != nil {
		return nil, err
	}
	s.subscriptionLifecycleMu.Lock()
	_, rollback := s.subscriptions.acquireWithMode(consumerID, instruments, true)
	if err := s.reconcileSubscriptions(ctx); err != nil {
		s.subscriptions.restore(rollback)
		_ = s.reconcileSubscriptions(context.Background())
		s.subscriptionLifecycleMu.Unlock()
		return nil, err
	}
	s.subscriptionLifecycleMu.Unlock()
	s.WakeCollector()
	return newManagedSubscription(func() {
		s.subscriptionLifecycleMu.Lock()
		defer s.subscriptionLifecycleMu.Unlock()
		s.subscriptions.restore(rollback)
		if err := s.reconcileSubscriptions(context.Background()); err != nil {
			log.Printf("marketdata managed subscription release reconciliation failed: %v", err)
		}
		s.WakeCollector()
	}), nil
}

// ReleaseSubscription 释放行情订阅。
func (s *Service) ReleaseSubscription(ctx context.Context, consumerID string, target ...InstrumentRef) error {
	s.subscriptionLifecycleMu.Lock()
	defer s.subscriptionLifecycleMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(target) > 0 {
		s.subscriptions.release(consumerID, target[0])
	} else {
		s.subscriptions.clear(consumerID)
	}
	if err := s.reconcileSubscriptions(ctx); err != nil {
		log.Printf("marketdata subscription release reconciliation deferred: %v", err)
	}
	s.WakeCollector()
	return nil
}

// Heartbeat 刷新订阅心跳。
func (s *Service) Heartbeat(ctx context.Context, consumerID string) (HeartbeatResult, error) {
	s.subscriptionLifecycleMu.Lock()
	defer s.subscriptionLifecycleMu.Unlock()
	s.subscriptions.heartbeat(consumerID)
	if err := s.reconcileSubscriptions(ctx); err != nil {
		log.Printf("marketdata subscription heartbeat reconciliation deferred: %v", err)
	}
	snapshot, err := s.GetSubscriptions(ctx)
	return HeartbeatResult(snapshot), err
}

// ClearSubscriptions 清空订阅。
func (s *Service) ClearSubscriptions(ctx context.Context, consumerID ...string) error {
	s.subscriptionLifecycleMu.Lock()
	defer s.subscriptionLifecycleMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	rawConsumerID := ""
	if len(consumerID) > 0 {
		rawConsumerID = consumerID[0]
	}
	s.subscriptions.clear(rawConsumerID)
	if err := s.reconcileSubscriptions(ctx); err != nil {
		log.Printf("marketdata subscription clear reconciliation deferred: %v", err)
	}
	s.WakeCollector()
	return nil
}

// GetSubscriptions 返回当前订阅快照。
func (s *Service) GetSubscriptions(ctx context.Context) (SubscriptionsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot := s.subscriptions.snapshot()
	s.subscriptionMu.RLock()
	reconciler := s.reconciler
	s.subscriptionMu.RUnlock()
	var broker map[string]any
	if reconciler != nil {
		broker = reconciler.SubscriptionState()
	}
	return decorateSubscriptionSnapshot(snapshot, broker), nil
}

// GetActiveInstruments 返回当前活跃标的列表。
func (s *Service) GetActiveInstruments(ctx context.Context) ([]string, error) {
	return s.subscriptions.activeInstruments(), nil
}

func (s *Service) reconcileSubscriptions(ctx context.Context) error {
	return s.reconcileDesired(ctx, s.activeSubscriptionDemand())
}

func (s *Service) activeSubscriptionDemand() []InstrumentRef {
	refs := s.subscriptions.activeSubscriptions()
	s.subscriptionMu.RLock()
	demands := append([]DemandSource(nil), s.additionalDemands...)
	s.subscriptionMu.RUnlock()
	for _, demand := range demands {
		if demand == nil {
			continue
		}
		for _, instrumentID := range demand.ActiveInstruments() {
			market, symbol := normalizeSubscriptionInstrument("", instrumentID)
			if market == "" || symbol == "" {
				continue
			}
			refs = append(refs, InstrumentRef{Channel: "SNAPSHOT", Market: market, Symbol: symbol})
		}
	}
	return refs
}

func (s *Service) reconcileDesired(ctx context.Context, desired []InstrumentRef) error {
	if s == nil {
		return nil
	}
	s.subscriptionMu.RLock()
	reconciler := s.reconciler
	s.subscriptionMu.RUnlock()
	if reconciler == nil {
		return nil
	}
	return reconciler.ReconcileSubscriptions(ctx, desired)
}

// GetLatestTicks 批量返回最新 Tick 数据。
func (s *Service) GetLatestTicks(ctx context.Context, symbols []string) (TicksResponse, error) {
	return LatestTicksJSON(s.cache.LatestMany(symbols, CacheRetention)), nil
}

func (s *Service) Ingest(sample Tick) *Tick {
	return s.cache.Store(sample)
}

func (s *Service) Seed(sample Tick) {
	s.cache.Seed(sample)
}

func (s *Service) CachedCount(instrumentID string) int {
	return s.cache.Count(instrumentID)
}

func (s *Service) Latest(instrumentID string, maxAge time.Duration) *Tick {
	return s.cache.Latest(instrumentID, maxAge)
}

func (s *Service) LatestMany(instrumentIDs []string, maxAge time.Duration) []*Tick {
	return s.cache.LatestMany(instrumentIDs, maxAge)
}

func (s *Service) AllFresh(instrumentIDs []string, maxAge time.Duration) bool {
	return s.cache.AllFresh(instrumentIDs, maxAge)
}

func (s *Service) TickCandles(instrumentID, fromTime, toTime string, limit int) []map[string]any {
	to := parseTime(toTime)
	from := parseTime(fromTime)
	return TickCandles(s.cache.Snapshot(instrumentID), from, to, limit)
}

func (s *Service) LiveTick(sample *Tick, observedAt string) map[string]any {
	return LiveTickJSON(sample, observedAt)
}

// NormalizeInstrument 规范化标的信息。
func (s *Service) NormalizeInstrument(ctx context.Context, input map[string]any) (map[string]any, error) {
	return s.provider.NormalizeInstrument(ctx, input)
}

// Health 返回行情健康状态。
func (s *Service) Health(ctx context.Context) (HealthStatus, error) {
	health, err := s.provider.Health(ctx)
	if err != nil {
		return HealthStatus{}, err
	}
	if s.collector != nil {
		state := s.collector.State()
		health.Connected = state.Connected
		health.ActiveCount = state.ActiveCount
	} else {
		health.ActiveCount = len(s.subscriptions.activeInstruments())
	}
	health.StreamMode = "idle"
	if health.ActiveCount > 0 {
		health.StreamMode = "snapshot-poll-fallback"
	}
	if health.Connected {
		health.StreamMode = "push-stream"
	}
	return health, nil
}

func normalizeInstrument(market, symbol string) (string, string, string) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	return market, symbol, market + "." + symbol
}

func tickCandlesResponse(market, symbol, instrumentID, period string, limit int, candles []map[string]any, fromCache bool) CandlesResponse {
	includeSession := market == "US"
	return CandlesResponseDTO{
		Instrument:     InstrumentDTO{Market: market, Symbol: symbol, InstrumentID: instrumentID},
		Period:         period,
		Limit:          limit,
		Candles:        candles,
		Source:         "bbgo:futu",
		ResolvedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		FromCache:      fromCache,
		ExtendedHours:  includeSession,
		IncludeSession: includeSession,
	}.JSON()
}
