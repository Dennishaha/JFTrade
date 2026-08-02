package testkit

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Response is one queued mock sidecar response.
type Response struct {
	Status int
	Body   string
	Header http.Header
}

// Request records one request received by the mock sidecar.
type Request struct {
	Method string
	Path   string
	Query  url.Values
}

// Server implements the complete yfinance sidecar HTTP contract for tests.
type Server struct {
	server *httptest.Server

	mu       sync.Mutex
	queued   map[string][]Response
	requests []Request
}

// New starts a mock yfinance sidecar with deterministic fixtures.
func New(t testing.TB) *Server {
	t.Helper()
	mock := &Server{queued: make(map[string][]Response)}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	t.Cleanup(mock.Close)
	return mock
}

func (s *Server) URL() string {
	if s == nil || s.server == nil {
		return ""
	}
	return s.server.URL
}

func (s *Server) HostPort() (string, int) {
	parsed, err := url.Parse(s.URL())
	if err != nil {
		return "", 0
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return "", 0
	}
	port, _ := strconv.Atoi(portText)
	return host, port
}

func (s *Server) Close() {
	if s != nil && s.server != nil {
		s.server.Close()
	}
}

// Queue installs responses consumed in order for one exact URL path.
func (s *Server) Queue(path string, responses ...Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queued[path] = append(s.queued[path], responses...)
}

func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Request, len(s.requests))
	copy(result, s.requests)
	return result
}

func (s *Server) Count(path string) int {
	count := 0
	for _, request := range s.Requests() {
		if request.Path == path {
			count++
		}
	}
	return count
}

func (s *Server) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	s.record(request)
	if response, ok := s.next(request.URL.Path); ok {
		writeResponse(writer, response)
		return
	}
	status, body := defaultFixture(request)
	writeResponse(writer, Response{Status: status, Body: body})
}

func (s *Server) record(request *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, Request{
		Method: request.Method,
		Path:   request.URL.Path,
		Query:  request.URL.Query(),
	})
}

func (s *Server) next(path string) (Response, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	responses := s.queued[path]
	if len(responses) == 0 {
		return Response{}, false
	}
	response := responses[0]
	s.queued[path] = responses[1:]
	return response, true
}

func writeResponse(writer http.ResponseWriter, response Response) {
	for key, values := range response.Header {
		for _, value := range values {
			writer.Header().Add(key, value)
		}
	}
	if writer.Header().Get("Content-Type") == "" {
		writer.Header().Set("Content-Type", "application/json")
	}
	status := response.Status
	if status == 0 {
		status = http.StatusOK
	}
	writer.WriteHeader(status)
	_, _ = writer.Write([]byte(response.Body))
}

func defaultFixture(request *http.Request) (int, string) {
	switch {
	case request.URL.Path == "/health":
		return http.StatusOK, `{"ok":true,"yfinance_version":"0.2.61","runtime_state":"ready","warmup_error":null}`
	case request.URL.Path == "/markets":
		return http.StatusOK, marketsFixture
	case request.URL.Path == "/search":
		return http.StatusOK, searchFixture(request.URL.Query().Get("q"))
	case strings.HasPrefix(request.URL.Path, "/security/"):
		return http.StatusOK, securityFixture(pathMarket(request.URL.Path), pathSymbol(request.URL.Path))
	case strings.HasPrefix(request.URL.Path, "/snapshot/"):
		return http.StatusOK, snapshotFixture(pathMarket(request.URL.Path), pathSymbol(request.URL.Path))
	case strings.HasPrefix(request.URL.Path, "/candles/"):
		return http.StatusOK, candlesFixture(pathMarket(request.URL.Path), pathSymbol(request.URL.Path), request.URL.Query().Get("period"))
	default:
		return http.StatusNotFound, `{"error":{"code":"NOT_FOUND","message":"fixture route not found"}}`
	}
}

func pathSymbol(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	return strings.ToUpper(parts[2])
}

func pathMarket(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 {
		return "US"
	}
	return strings.ToUpper(parts[1])
}

func searchFixture(query string) string {
	symbol := strings.ToUpper(strings.TrimSpace(query))
	if symbol == "" {
		symbol = "AAPL"
	}
	return fmt.Sprintf(
		`{"entries":[{"market":"US","resolved_market":"US","instrument_id":"US.%[1]s","code":"%[1]s","symbol":"%[1]s","name":"%[1]s Incorporated","security_type":"EQUITY","exchange":"NASDAQ","selectable":true,"source":"yfinance"}]}`,
		symbol,
	)
}

func securityFixture(market, symbol string) string {
	exchange, currency, timezone := "NASDAQ", "USD", "America/New_York"
	switch market {
	case "HK":
		exchange, currency, timezone = "HKEX", "HKD", "Asia/Hong_Kong"
	case "SH":
		exchange, currency, timezone = "SSE", "CNY", "Asia/Shanghai"
	case "SZ":
		exchange, currency, timezone = "SZSE", "CNY", "Asia/Shanghai"
	}
	return fmt.Sprintf(
		`{"market":"%[1]s","symbol":"%[2]s","instrument_id":"%[1]s.%[2]s","name":"%[2]s Incorporated","exchange":"%[3]s","currency":"%[4]s","timezone":"%[5]s","security_type":"EQUITY","industry":"Technology","sector":"Technology","website":"https://example.test","business_summary":"Fixture company","market_cap":3000000000000,"trailing_pe":31.2,"forward_pe":28.4,"trailing_eps":6.1,"forward_eps":7.2,"dividend_rate":1,"dividend_yield":0.45,"fifty_two_week_high":237.49,"fifty_two_week_low":164.08,"average_volume":55000000,"shares_outstanding":15000000000,"source":"yfinance"}`,
		market, symbol, exchange, currency, timezone,
	)
}

func snapshotFixture(market, symbol string) string {
	price := "189.25"
	if symbol == "MSFT" {
		price = "420.25"
	}
	exchange, currency := "NASDAQ", "USD"
	switch market {
	case "HK":
		price, exchange, currency = "320.5", "HKEX", "HKD"
	case "SH":
		price, exchange, currency = "1500.5", "SSE", "CNY"
	case "SZ":
		price, exchange, currency = "12.5", "SZSE", "CNY"
	}
	return fmt.Sprintf(
		`{"market":"%[1]s","symbol":"%[2]s","instrument_id":"%[1]s.%[2]s","price":%[3]s,"bid":%[3]s,"ask":%[3]s,"open_price":187.5,"high_price":190.1,"low_price":186.9,"previous_close_price":188.2,"last_close_price":188.2,"regular_quote":{"price":188.5,"quote_at":"2026-07-29T14:30:00Z"},"pre_market_quote":null,"after_market_quote":null,"volume":1234567,"turnover":233456789.5,"quote_at":"2026-07-29T14:30:00Z","observed_at":"2026-07-29T14:45:00Z","source":"yfinance","delayed":true,"delay_minutes":15,"currency":"%[4]s","exchange":"%[5]s"}`,
		market, symbol, price, currency, exchange,
	)
}

func candlesFixture(market, symbol, period string) string {
	if period == "" {
		period = "1d"
	}
	extendedHours := market == "US"
	return fmt.Sprintf(
		`{"market":"%[1]s","symbol":"%[2]s","instrument_id":"%[1]s.%[2]s","period":"%[3]s","extended_hours":%[4]t,"total_returned":2,"source":"yfinance","candles":[{"at":"2026-07-28T13:30:00Z","open":185.1,"high":188.2,"low":184.5,"close":187.8,"volume":1000},{"at":"2026-07-29T13:30:00Z","open":187.8,"high":190.1,"low":186.9,"close":189.25,"volume":1200}]}`,
		market, symbol, period, extendedHours,
	)
}

const marketsFixture = `{"markets":[{"code":"US","resolved_market":"US","preferred_prefix":"US","display_name":"United States","quote_currency":"USD","timezone":"America/New_York","supports_extended_hours":true,"requires_exchange_prefix":false,"aliases":["USA","NYSE","NASDAQ","AMEX"],"regular_sessions":[{"start_minute":570,"end_minute":960,"label":"09:30-16:00"}],"precision":{"price":2,"quote":2},"tick_size":0.01},{"code":"HK","resolved_market":"HK","preferred_prefix":"HK","display_name":"Hong Kong","quote_currency":"HKD","timezone":"Asia/Hong_Kong","supports_extended_hours":false,"requires_exchange_prefix":false,"aliases":["HKG","HKEX"],"regular_sessions":[{"start_minute":570,"end_minute":720,"label":"09:30-12:00"},{"start_minute":780,"end_minute":960,"label":"13:00-16:00"}],"precision":{"price":3,"quote":3},"tick_size":0.01},{"code":"SH","resolved_market":"SH","preferred_prefix":"SH","display_name":"Shanghai","quote_currency":"CNY","timezone":"Asia/Shanghai","supports_extended_hours":false,"requires_exchange_prefix":true,"aliases":["SSE","SHH"],"regular_sessions":[{"start_minute":570,"end_minute":690,"label":"09:30-11:30"},{"start_minute":780,"end_minute":900,"label":"13:00-15:00"}],"precision":{"price":2,"quote":2},"tick_size":0.01},{"code":"SZ","resolved_market":"SZ","preferred_prefix":"SZ","display_name":"Shenzhen","quote_currency":"CNY","timezone":"Asia/Shanghai","supports_extended_hours":false,"requires_exchange_prefix":true,"aliases":["SZSE","SHZ"],"regular_sessions":[{"start_minute":570,"end_minute":690,"label":"09:30-11:30"},{"start_minute":780,"end_minute":900,"label":"13:00-15:00"}],"precision":{"price":2,"quote":2},"tick_size":0.01}]}`
