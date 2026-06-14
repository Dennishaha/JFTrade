package live

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

type SecurityDetailsSubscription struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	InstrumentID string `json:"instrumentId"`
}

type DepthSubscription struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	InstrumentID string `json:"instrumentId"`
	Num          int32  `json:"num"`
}

type Subscriptions struct {
	ActiveInstruments []string                      `json:"activeInstruments,omitempty"`
	SecurityDetails   []SecurityDetailsSubscription `json:"securityDetails,omitempty"`
	Depth             []DepthSubscription           `json:"depth,omitempty"`
	ConsoleRefresh    bool                          `json:"consoleRefresh,omitempty"`
}

type Client struct {
	id            uint64
	updated       chan struct{}
	mu            sync.RWMutex
	subscriptions Subscriptions
}

func newClient(id uint64) *Client {
	return &Client{id: id, updated: make(chan struct{}, 1)}
}

func (c *Client) ID() uint64 {
	if c == nil {
		return 0
	}
	return c.id
}

func (c *Client) Updated() <-chan struct{} {
	if c == nil {
		return nil
	}
	return c.updated
}

func (c *Client) Snapshot() Subscriptions {
	if c == nil {
		return Subscriptions{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneSubscriptions(c.subscriptions)
}

func (c *Client) SetSubscriptions(next Subscriptions) {
	if c == nil {
		return
	}
	normalized := NormalizeSubscriptions(next)
	c.mu.Lock()
	c.subscriptions = normalized
	c.mu.Unlock()
	select {
	case c.updated <- struct{}{}:
	default:
	}
}

type ClientRegistry struct {
	mu      sync.RWMutex
	nextID  uint64
	clients map[uint64]*Client
}

func (r *ClientRegistry) Register() *Client {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.clients == nil {
		r.clients = map[uint64]*Client{}
	}
	r.nextID++
	client := newClient(r.nextID)
	r.clients[client.id] = client
	return client
}

func (r *ClientRegistry) Unregister(id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, id)
}

func (r *ClientRegistry) ActiveInstrumentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, client := range r.clients {
		for _, instrumentID := range client.Snapshot().ActiveInstruments {
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

func NormalizeSubscriptions(input Subscriptions) Subscriptions {
	activeSeen := map[string]struct{}{}
	activeInstruments := make([]string, 0, len(input.ActiveInstruments))
	for _, instrumentID := range input.ActiveInstruments {
		instrumentID = normalizeInstrumentID(instrumentID)
		if instrumentID == "" {
			continue
		}
		if _, exists := activeSeen[instrumentID]; exists {
			continue
		}
		activeSeen[instrumentID] = struct{}{}
		activeInstruments = append(activeInstruments, instrumentID)
	}
	sort.Strings(activeInstruments)

	securitySeen := map[string]struct{}{}
	securityDetails := make([]SecurityDetailsSubscription, 0, len(input.SecurityDetails))
	for _, item := range input.SecurityDetails {
		market := strings.ToUpper(strings.TrimSpace(item.Market))
		symbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
		instrumentID := normalizeInstrumentID(item.InstrumentID)
		if market == "" || symbol == "" || instrumentID == "" {
			continue
		}
		if _, exists := securitySeen[instrumentID]; exists {
			continue
		}
		securitySeen[instrumentID] = struct{}{}
		securityDetails = append(securityDetails, SecurityDetailsSubscription{
			Market: market, Symbol: symbol, InstrumentID: instrumentID,
		})
	}
	sort.Slice(securityDetails, func(i, j int) bool {
		return securityDetails[i].InstrumentID < securityDetails[j].InstrumentID
	})

	depthSeen := map[string]struct{}{}
	depth := make([]DepthSubscription, 0, len(input.Depth))
	for _, item := range input.Depth {
		market := strings.ToUpper(strings.TrimSpace(item.Market))
		symbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
		instrumentID := normalizeInstrumentID(item.InstrumentID)
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
		depth = append(depth, DepthSubscription{
			Market: market, Symbol: symbol, InstrumentID: instrumentID, Num: num,
		})
	}
	sort.Slice(depth, func(i, j int) bool {
		if depth[i].InstrumentID == depth[j].InstrumentID {
			return depth[i].Num < depth[j].Num
		}
		return depth[i].InstrumentID < depth[j].InstrumentID
	})

	return Subscriptions{
		ActiveInstruments: activeInstruments,
		SecurityDetails:   securityDetails,
		Depth:             depth,
		ConsoleRefresh:    input.ConsoleRefresh,
	}
}

func cloneSubscriptions(input Subscriptions) Subscriptions {
	input.ActiveInstruments = append([]string(nil), input.ActiveInstruments...)
	input.SecurityDetails = append([]SecurityDetailsSubscription(nil), input.SecurityDetails...)
	input.Depth = append([]DepthSubscription(nil), input.Depth...)
	return input
}

func normalizeInstrumentID(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
