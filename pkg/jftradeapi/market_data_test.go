package jftradeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

func TestMarketCandlesEndpointIncludesCurrentRealtimeBucket(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	historyLabelAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	currentLabelAt := historyLabelAt.Add(time.Minute)
	quoteServer.setHistoryPages([][]*qotcommonpb.KLine{{
		testMarketDataProtoKLine(historyLabelAt, 100, 101, 99, 100.5, 1000),
	}})
	quoteServer.setCurrentKLines([]*qotcommonpb.KLine{
		testMarketDataProtoKLine(currentLabelAt, 101, 106, 99, 103, 500),
	})

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    host,
			APIPort:                 port,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()

	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	requestURL := fmt.Sprintf(
		"%s/api/v1/market-data/candles/HK/00700?period=1m&limit=2&fromTime=%s&toTime=%s",
		srv.URL,
		url.QueryEscape(historyLabelAt.Add(-time.Hour).Format(time.RFC3339Nano)),
		url.QueryEscape(currentLabelAt.Add(30*time.Second).Format(time.RFC3339Nano)),
	)
	resp, err := http.Get(requestURL)
	if err != nil {
		t.Fatalf("GET market candles: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET market candles status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Candles []struct {
				Period string  `json:"period"`
				Open   float64 `json:"open"`
				High   float64 `json:"high"`
				Low    float64 `json:"low"`
				Close  float64 `json:"close"`
				Volume float64 `json:"volume"`
				At     string  `json:"at"`
			} `json:"candles"`
			TotalReturned int `json:"totalReturned"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode market candles: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if got := quoteServer.currentKLCallCount(); got != 1 {
		t.Fatalf("expected one GetKL call, got %d", got)
	}
	if got := len(envelope.Data.Candles); got != 2 {
		t.Fatalf("expected two candles, got %d", got)
	}
	if envelope.Data.TotalReturned != 2 {
		t.Fatalf("totalReturned = %d, want 2", envelope.Data.TotalReturned)
	}

	if got := envelope.Data.Candles[0].At; got != historyLabelAt.Add(-time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("first candle at = %s", got)
	}
	if got := envelope.Data.Candles[1].At; got != historyLabelAt.Format(time.RFC3339Nano) {
		t.Fatalf("current candle at = %s", got)
	}
	if envelope.Data.Candles[1].Period != "1m" {
		t.Fatalf("current candle period = %s", envelope.Data.Candles[1].Period)
	}
	if envelope.Data.Candles[1].Open != 101 || envelope.Data.Candles[1].High != 106 || envelope.Data.Candles[1].Low != 99 || envelope.Data.Candles[1].Close != 103 {
		t.Fatalf("unexpected current candle OHLC: %+v", envelope.Data.Candles[1])
	}
	if envelope.Data.Candles[1].Volume != 500 {
		t.Fatalf("current candle volume = %v, want 500", envelope.Data.Candles[1].Volume)
	}
}

type marketDataQuoteOpenDServer struct {
	addr              string
	listener          net.Listener
	stopOnce          sync.Once
	shutdownCompleted chan struct{}
	historyMu         sync.Mutex
	historyPages      [][]*qotcommonpb.KLine
	currentKLines     []*qotcommonpb.KLine
	currentKLCalls    atomic.Int32
}

func startMarketDataQuoteOpenDServer(t *testing.T) *marketDataQuoteOpenDServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &marketDataQuoteOpenDServer{
		addr:              listener.Addr().String(),
		listener:          listener,
		shutdownCompleted: make(chan struct{}),
	}
	go server.acceptLoop()
	return server
}

func (s *marketDataQuoteOpenDServer) stop() {
	s.stopOnce.Do(func() {
		_ = s.listener.Close()
		<-s.shutdownCompleted
	})
}

func (s *marketDataQuoteOpenDServer) setHistoryPages(pages [][]*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = pages
}

func (s *marketDataQuoteOpenDServer) setCurrentKLines(klines []*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.currentKLines = klines
}

func (s *marketDataQuoteOpenDServer) currentKLCallCount() int {
	return int(s.currentKLCalls.Load())
}

func (s *marketDataQuoteOpenDServer) acceptLoop() {
	defer close(s.shutdownCompleted)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *marketDataQuoteOpenDServer) handleConn(conn net.Conn) {
	defer conn.Close()
	for {
		header := make([]byte, codec.HeaderLen)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		bodyLen := int(uint32(header[12]) | uint32(header[13])<<8 | uint32(header[14])<<16 | uint32(header[15])<<24)
		packet := make([]byte, codec.HeaderLen+bodyLen)
		copy(packet, header)
		if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
			return
		}
		frame, err := codec.Decode(packet)
		if err != nil {
			return
		}

		var response proto.Message
		switch frame.Header.ProtoID {
		case opend.ProtoInitConnect:
			response = &initpb.Response{
				RetType: proto.Int32(0),
				S2C: &initpb.S2C{
					ServerVer:         proto.Int32(700),
					LoginUserID:       proto.Uint64(1),
					ConnID:            proto.Uint64(42),
					ConnAESKey:        proto.String("0123456789abcdef"),
					KeepAliveInterval: proto.Int32(10),
				},
			}
		case opend.ProtoQotSub:
			response = &qotsubpb.Response{RetType: proto.Int32(0)}
		case opend.ProtoRequestHistoryKL:
			response = s.historyKLResponse(frame.Body)
		case opend.ProtoGetKL:
			s.currentKLCalls.Add(1)
			response = s.currentKLResponse(frame.Body)
		default:
			return
		}

		body, err := proto.Marshal(response)
		if err != nil {
			return
		}
		packet, err = codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
		if err != nil {
			return
		}
		if _, err := conn.Write(packet); err != nil {
			return
		}
	}
}

func (s *marketDataQuoteOpenDServer) historyKLResponse(body []byte) *historypb.Response {
	request := &historypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &historypb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}

	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	page := []*qotcommonpb.KLine(nil)
	if len(s.historyPages) > 0 {
		page = s.historyPages[0]
	}
	return &historypb.Response{
		RetType: proto.Int32(0),
		S2C: &historypb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList:   page,
		},
	}
}

func (s *marketDataQuoteOpenDServer) currentKLResponse(body []byte) *qotgetklpb.Response {
	request := &qotgetklpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetklpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}

	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	return &qotgetklpb.Response{
		RetType: proto.Int32(0),
		S2C: &qotgetklpb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList:   s.currentKLines,
		},
	}
}

func testMarketDataProtoKLine(at time.Time, open float64, high float64, low float64, close float64, volume int64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       proto.String(at.Format("2006-01-02 15:04:05")),
		Timestamp:  proto.Float64(float64(at.Unix())),
		IsBlank:    proto.Bool(false),
		OpenPrice:  proto.Float64(open),
		HighPrice:  proto.Float64(high),
		LowPrice:   proto.Float64(low),
		ClosePrice: proto.Float64(close),
		Volume:     proto.Int64(volume),
		Turnover:   proto.Float64(close * float64(volume)),
	}
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", portText, err)
	}
	return host, port
}
