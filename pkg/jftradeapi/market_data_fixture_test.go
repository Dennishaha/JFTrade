package jftradeapi

import (
	"io"
	"net"
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
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

type marketDataQuoteOpenDServer struct {
	addr              string
	listener          net.Listener
	stopOnce          sync.Once
	shutdownCompleted chan struct{}
	basicQotCalls     atomic.Int32
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

func (s *marketDataQuoteOpenDServer) basicQotCallCount() int {
	return int(s.basicQotCalls.Load())
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
		case opend.ProtoGetBasicQot:
			s.basicQotCalls.Add(1)
			response = s.basicQotResponse(frame.Body)
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

func (s *marketDataQuoteOpenDServer) basicQotResponse(body []byte) *qotgetbasicqotpb.Response {
	request := &qotgetbasicqotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetbasicqotpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}

	quotes := make([]*qotcommonpb.BasicQot, 0, len(request.GetC2S().GetSecurityList()))
	quoteAt := time.Now().UTC().Truncate(time.Second)
	for _, security := range request.GetC2S().GetSecurityList() {
		quotes = append(quotes, &qotcommonpb.BasicQot{
			Security:        security,
			IsSuspended:     proto.Bool(false),
			ListTime:        proto.String("2020-01-01"),
			PriceSpread:     proto.Float64(0.01),
			UpdateTime:      proto.String(quoteAt.Format("2006-01-02 15:04:05")),
			HighPrice:       proto.Float64(322.6),
			OpenPrice:       proto.Float64(319.8),
			LowPrice:        proto.Float64(319.6),
			CurPrice:        proto.Float64(321.4),
			LastClosePrice:  proto.Float64(318.9),
			Volume:          proto.Int64(1282100),
			Turnover:        proto.Float64(411020000),
			TurnoverRate:    proto.Float64(1.25),
			Amplitude:       proto.Float64(2.5),
			UpdateTimestamp: proto.Float64(float64(quoteAt.Unix())),
		})
	}

	return &qotgetbasicqotpb.Response{
		RetType: proto.Int32(0),
		S2C: &qotgetbasicqotpb.S2C{
			BasicQotList: quotes,
		},
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

func newMarketDataTestServerWithQuoteRuntime(t *testing.T, addr string) *Server {
	t.Helper()
	host, port := splitHostPort(t, addr)
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
	return NewServer(store)
}

func seedCachedTickSample(server *Server, sample marketTickSample) {
	server.tickCache.seed(sample)
}

func float64Ptr(v float64) *float64 {
	return &v
}

func assertSnapshotResponse(t *testing.T, response map[string]any, instrumentID string, fromCache bool, source string) {
	t.Helper()
	request, ok := response["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", response["request"])
	}
	if got := request["instrumentId"]; got != instrumentID {
		t.Fatalf("instrumentId = %v, want %s", got, instrumentID)
	}
	snapshot, ok := response["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot payload type = %T", response["snapshot"])
	}
	if got := snapshot["price"]; got == nil {
		t.Fatal("expected snapshot price")
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["fromCache"]; got != fromCache {
		t.Fatalf("fromCache = %v, want %v", got, fromCache)
	}
	if got := meta["source"]; got != source {
		t.Fatalf("source = %v, want %s", got, source)
	}
	if got := meta["instrumentId"]; got != instrumentID {
		t.Fatalf("meta instrumentId = %v, want %s", got, instrumentID)
	}
}

func assertTickCandlesResponse(t *testing.T, response map[string]any, instrumentID string, fromCache bool, wantCount int) {
	t.Helper()
	request, ok := response["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", response["request"])
	}
	instrument, ok := request["instrument"].(map[string]any)
	if !ok {
		t.Fatalf("instrument payload type = %T", request["instrument"])
	}
	if got := instrument["instrumentId"]; got != instrumentID {
		t.Fatalf("instrumentId = %v, want %s", got, instrumentID)
	}
	totalReturned, ok := response["totalReturned"].(int)
	if !ok {
		t.Fatalf("totalReturned payload type = %T", response["totalReturned"])
	}
	if totalReturned != wantCount {
		t.Fatalf("totalReturned = %d, want %d", totalReturned, wantCount)
	}
	candles, ok := response["candles"].([]map[string]any)
	if !ok {
		t.Fatalf("candles payload type = %T", response["candles"])
	}
	if len(candles) != wantCount {
		t.Fatalf("len(candles) = %d, want %d", len(candles), wantCount)
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["fromCache"]; got != fromCache {
		t.Fatalf("fromCache = %v, want %v", got, fromCache)
	}
}
