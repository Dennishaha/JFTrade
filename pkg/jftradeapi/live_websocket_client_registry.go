package jftradeapi

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

type liveWebSocketSecurityDetailsSubscription struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	InstrumentID string `json:"instrumentId"`
}

type liveWebSocketDepthSubscription struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	InstrumentID string `json:"instrumentId"`
	Num          int32  `json:"num"`
}

type liveWebSocketSubscriptions struct {
	ActiveInstruments []string                                   `json:"activeInstruments,omitempty"`
	SecurityDetails   []liveWebSocketSecurityDetailsSubscription `json:"securityDetails,omitempty"`
	Depth             []liveWebSocketDepthSubscription           `json:"depth,omitempty"`
	ConsoleRefresh    bool                                       `json:"consoleRefresh,omitempty"`
}

type liveWebSocketClientMessage struct {
	Type          string                     `json:"type"`
	Subscriptions liveWebSocketSubscriptions `json:"subscriptions"`
}

type liveWebSocketClient struct {
	id            uint64
	updated       chan struct{}
	mu            sync.RWMutex
	subscriptions liveWebSocketSubscriptions
}

func newLiveWebSocketClient(id uint64) *liveWebSocketClient {
	return &liveWebSocketClient{
		id:      id,
		updated: make(chan struct{}, 1),
	}
}

func (c *liveWebSocketClient) snapshot() liveWebSocketSubscriptions {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.subscriptions
}

func (c *liveWebSocketClient) setSubscriptions(next liveWebSocketSubscriptions) {
	normalized := normalizeLiveWebSocketSubscriptions(next)

	c.mu.Lock()
	c.subscriptions = normalized
	c.mu.Unlock()

	select {
	case c.updated <- struct{}{}:
	default:
	}
}

type liveWebSocketClientRegistry struct {
	mu      sync.RWMutex
	nextID  uint64
	clients map[uint64]*liveWebSocketClient
}

func (r *liveWebSocketClientRegistry) register() *liveWebSocketClient {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.clients == nil {
		r.clients = map[uint64]*liveWebSocketClient{}
	}
	r.nextID++
	client := newLiveWebSocketClient(r.nextID)
	r.clients[client.id] = client
	return client
}

func (r *liveWebSocketClientRegistry) unregister(id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.clients == nil {
		return
	}
	delete(r.clients, id)
}

func (r *liveWebSocketClientRegistry) activeInstrumentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, client := range r.clients {
		for _, instrumentID := range client.snapshot().ActiveInstruments {
			if _, exists := seen[instrumentID]; exists {
				continue
			}
			seen[instrumentID] = struct{}{}
			result = append(result, instrumentID)
		}
	}
	sort.Strings(result)
	return result
}

func normalizeLiveWebSocketSubscriptions(input liveWebSocketSubscriptions) liveWebSocketSubscriptions {
	activeSeen := map[string]struct{}{}
	activeInstruments := make([]string, 0, len(input.ActiveInstruments))
	for _, instrumentID := range input.ActiveInstruments {
		normalizedInstrumentID := normalizeLiveInstrumentID(instrumentID)
		if normalizedInstrumentID == "" {
			continue
		}
		if _, exists := activeSeen[normalizedInstrumentID]; exists {
			continue
		}
		activeSeen[normalizedInstrumentID] = struct{}{}
		activeInstruments = append(activeInstruments, normalizedInstrumentID)
	}
	sort.Strings(activeInstruments)

	securitySeen := map[string]struct{}{}
	securityDetails := make([]liveWebSocketSecurityDetailsSubscription, 0, len(input.SecurityDetails))
	for _, item := range input.SecurityDetails {
		market := strings.ToUpper(strings.TrimSpace(item.Market))
		symbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
		instrumentID := normalizeLiveInstrumentID(item.InstrumentID)
		if market == "" || symbol == "" || instrumentID == "" {
			continue
		}
		if _, exists := securitySeen[instrumentID]; exists {
			continue
		}
		securitySeen[instrumentID] = struct{}{}
		securityDetails = append(securityDetails, liveWebSocketSecurityDetailsSubscription{
			Market:       market,
			Symbol:       symbol,
			InstrumentID: instrumentID,
		})
	}
	sort.Slice(securityDetails, func(i int, j int) bool {
		return securityDetails[i].InstrumentID < securityDetails[j].InstrumentID
	})

	depthSeen := map[string]struct{}{}
	depth := make([]liveWebSocketDepthSubscription, 0, len(input.Depth))
	for _, item := range input.Depth {
		market := strings.ToUpper(strings.TrimSpace(item.Market))
		symbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
		instrumentID := normalizeLiveInstrumentID(item.InstrumentID)
		num := item.Num
		if num < 1 {
			num = 1
		}
		if num > 50 {
			num = 50
		}
		if market == "" || symbol == "" || instrumentID == "" {
			continue
		}
		key := instrumentID + "|" + strconv.Itoa(int(num))
		if _, exists := depthSeen[key]; exists {
			continue
		}
		depthSeen[key] = struct{}{}
		depth = append(depth, liveWebSocketDepthSubscription{
			Market:       market,
			Symbol:       symbol,
			InstrumentID: instrumentID,
			Num:          num,
		})
	}
	sort.Slice(depth, func(i int, j int) bool {
		if depth[i].InstrumentID == depth[j].InstrumentID {
			return depth[i].Num < depth[j].Num
		}
		return depth[i].InstrumentID < depth[j].InstrumentID
	})

	return liveWebSocketSubscriptions{
		ActiveInstruments: activeInstruments,
		SecurityDetails:   securityDetails,
		Depth:             depth,
		ConsoleRefresh:    input.ConsoleRefresh,
	}
}

func normalizeLiveInstrumentID(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
