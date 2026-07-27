package testkit

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
	qotgetsubinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsubinfo"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

type marketDataQuoteOpenDServer struct {
	addr                  string
	listener              net.Listener
	stopOnce              sync.Once
	shutdownCompleted     chan struct{}
	basicQotCalls         atomic.Int32
	securitySnapshotCalls atomic.Int32
	staticInfoCalls       atomic.Int32
	searchQuoteCalls      atomic.Int32
	qotSubCalls           atomic.Int32
	orderBookCalls        atomic.Int32
	lastOrderBookNum      atomic.Int32
	historyMu             sync.Mutex
	historyPages          [][]*qotcommonpb.KLine
	historyPagesBySession map[int32][][]*qotcommonpb.KLine
	currentKLines         []*qotcommonpb.KLine
	currentKLCalls        atomic.Int32
	orderBookMu           sync.Mutex
	orderBookBids         []*qotcommonpb.OrderBook
	orderBookAsks         []*qotcommonpb.OrderBook
	orderBookErr          error
}

func startMarketDataQuoteOpenDServer(t *testing.T) *marketDataQuoteOpenDServer {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
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
		jftradeErr1 := s.listener.Close()
		jftradePanicOnError(jftradeErr1)
		<-s.shutdownCompleted
	})
}

func (s *marketDataQuoteOpenDServer) basicQotCallCount() int {
	return int(s.basicQotCalls.Load())
}

func (s *marketDataQuoteOpenDServer) securitySnapshotCallCount() int {
	return int(s.securitySnapshotCalls.Load())
}

func (s *marketDataQuoteOpenDServer) staticInfoCallCount() int {
	return int(s.staticInfoCalls.Load())
}

func (s *marketDataQuoteOpenDServer) searchQuoteCallCount() int {
	return int(s.searchQuoteCalls.Load())
}

func (s *marketDataQuoteOpenDServer) qotSubCallCount() int {
	return int(s.qotSubCalls.Load())
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
	defer func() { jftradePanicOnError(conn.Close()) }()
	for {
		frame, err := readQuoteFrame(conn)
		if err != nil {
			return
		}
		response, rawBody, ok := s.quoteResponseForFrame(frame)
		if !ok || writeQuoteResponse(conn, frame, response, rawBody) != nil {
			return
		}
	}
}

func readQuoteFrame(conn net.Conn) (codec.Frame, error) {
	header := make([]byte, codec.HeaderLen)
	if _, err := io.ReadFull(conn, header); err != nil {
		return codec.Frame{}, err
	}
	bodyLen := int(uint32(header[12]) | uint32(header[13])<<8 | uint32(header[14])<<16 | uint32(header[15])<<24)
	packet := make([]byte, codec.HeaderLen+bodyLen)
	copy(packet, header)
	if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
		return codec.Frame{}, err
	}
	return codec.Decode(packet)
}

func writeQuoteResponse(conn net.Conn, frame codec.Frame, response proto.Message, rawBody []byte) error {
	body := rawBody
	if body == nil {
		var err error
		body, err = proto.Marshal(response)
		if err != nil {
			return err
		}
	}
	packet, err := codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
	if err != nil {
		return err
	}
	_, err = conn.Write(packet)
	return err
}

func (s *marketDataQuoteOpenDServer) quoteResponseForFrame(frame codec.Frame) (proto.Message, []byte, bool) {
	switch frame.Header.ProtoID {
	case opend.ProtoInitConnect:
		return quoteInitResponse(), nil, true
	case opend.ProtoGetGlobalState:
		return marketDataGlobalStateResponse(), nil, true
	case opend.ProtoQotSub:
		s.qotSubCalls.Add(1)
		return &qotsubpb.Response{RetType: new(int32(0))}, nil, true
	case opend.ProtoGetSubInfo:
		return quoteSubscriptionInfoResponse(), nil, true
	case opend.ProtoGetBasicQot:
		s.basicQotCalls.Add(1)
		return s.basicQotResponse(frame.Body), nil, true
	case opend.ProtoGetSecuritySnapshot:
		s.securitySnapshotCalls.Add(1)
		return s.securitySnapshotResponse(frame.Body), nil, true
	case opend.ProtoGetStaticInfo:
		s.staticInfoCalls.Add(1)
		return s.staticInfoResponse(frame.Body), nil, true
	case opend.ProtoGetSearchQuote:
		s.searchQuoteCalls.Add(1)
		return s.searchQuoteResponse(frame.Body), nil, true
	case opend.ProtoRequestHistoryKL:
		return s.historyKLResponse(frame.Body), nil, true
	case opend.ProtoGetKL:
		s.currentKLCalls.Add(1)
		return s.currentKLResponse(frame.Body), nil, true
	case opend.ProtoGetOrderBook:
		s.orderBookCalls.Add(1)
		return nil, s.orderBookResponseBody(frame.Body), true
	default:
		return nil, nil, false
	}
}

func quoteInitResponse() *initpb.Response {
	return &initpb.Response{RetType: new(int32(0)), S2C: &initpb.S2C{
		ServerVer: new(int32(1009)), LoginUserID: new(uint64(1)), ConnID: new(uint64(42)),
		ConnAESKey: new("0123456789abcdef"), KeepAliveInterval: new(int32(10)),
	}}
}

func quoteSubscriptionInfoResponse() *qotgetsubinfopb.Response {
	return &qotgetsubinfopb.Response{RetType: new(int32(0)), S2C: &qotgetsubinfopb.S2C{
		TotalUsedQuota: new(int32(2)), RemainQuota: new(int32(98)),
	}}
}

func (s *marketDataQuoteOpenDServer) searchQuoteResponse(body []byte) *qotgetsearchquotepb.Response {
	request := &qotgetsearchquotepb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetsearchquotepb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	if request.GetC2S().GetKeyword() == "" || request.GetC2S().GetMaxCount() != 100 {
		return &qotgetsearchquotepb.Response{RetType: new(int32(1)), RetMsg: new("invalid search request")}
	}
	return &qotgetsearchquotepb.Response{
		RetType: new(int32(0)),
		S2C: &qotgetsearchquotepb.S2C{SearchQuoteList: []*qotgetsearchquotepb.SearchQuote{
			{Market: new(int32(qotcommonpb.QotMarket_QotMarket_CNSH_Security)), Code: new("000001"), Name: new("Shanghai Index"), SecType: new(int32(qotcommonpb.SecurityType_SecurityType_Index))},
			{Market: new(int32(qotcommonpb.QotMarket_QotMarket_CNSZ_Security)), Code: new("000001"), Name: new("Ping An Bank"), SecType: new(int32(qotcommonpb.SecurityType_SecurityType_Eqty))},
			{Market: new(int32(qotcommonpb.QotMarket_QotMarket_JP_Security)), Code: new("7203"), Name: new("Toyota"), SecType: new(int32(qotcommonpb.SecurityType_SecurityType_Eqty))},
		}},
	}
}

func marketDataGlobalStateResponse() *globalpb.Response {
	zero := int32(0)
	return &globalpb.Response{
		RetType: new(int32(0)),
		S2C: &globalpb.S2C{
			MarketHK:       &zero,
			MarketUS:       &zero,
			MarketSH:       &zero,
			MarketSZ:       &zero,
			MarketHKFuture: &zero,
			QotLogined:     new(true),
			TrdLogined:     new(true),
			ServerVer:      new(int32(1009)),
			ServerBuildNo:  new(int32(6908)),
			Time:           new(int64(0)),
		},
	}
}

func (s *marketDataQuoteOpenDServer) securitySnapshotResponse(body []byte) *qotgetsecuritysnapshotpb.Response {
	request := &qotgetsecuritysnapshotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetsecuritysnapshotpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}

	quoteAt := time.Now().UTC().Truncate(time.Second)
	snapshots := make([]*qotgetsecuritysnapshotpb.Snapshot, 0, len(request.GetC2S().GetSecurityList()))
	for _, security := range request.GetC2S().GetSecurityList() {
		snapshots = append(snapshots, marketDataSecuritySnapshotFixture(security, quoteAt))
	}

	return &qotgetsecuritysnapshotpb.Response{
		RetType: new(int32(0)),
		S2C:     &qotgetsecuritysnapshotpb.S2C{SnapshotList: snapshots},
	}
}

func (s *marketDataQuoteOpenDServer) staticInfoResponse(body []byte) *qotgetstaticinfopb.Response {
	request := &qotgetstaticinfopb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetstaticinfopb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}

	entries := make([]*qotcommonpb.SecurityStaticInfo, 0, len(request.GetC2S().GetSecurityList()))
	for _, security := range request.GetC2S().GetSecurityList() {
		entries = append(entries, marketDataSecurityStaticInfoFixture(security))
	}

	return &qotgetstaticinfopb.Response{
		RetType: new(int32(0)),
		S2C:     &qotgetstaticinfopb.S2C{StaticInfoList: entries},
	}
}

func (s *marketDataQuoteOpenDServer) basicQotResponse(body []byte) *qotgetbasicqotpb.Response {
	request := &qotgetbasicqotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetbasicqotpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}

	quotes := make([]*qotcommonpb.BasicQot, 0, len(request.GetC2S().GetSecurityList()))
	quoteAt := time.Now().UTC().Truncate(time.Second)
	for _, security := range request.GetC2S().GetSecurityList() {
		quotes = append(quotes, &qotcommonpb.BasicQot{
			Security:        security,
			IsSuspended:     new(false),
			ListTime:        new("2020-01-01"),
			PriceSpread:     new(0.01),
			UpdateTime:      new(quoteAt.Format("2006-01-02 15:04:05")),
			HighPrice:       new(322.6),
			OpenPrice:       new(319.8),
			LowPrice:        new(319.6),
			CurPrice:        new(321.4),
			LastClosePrice:  new(318.9),
			Volume:          new(int64(1282100)),
			Turnover:        new(float64(411020000)),
			TurnoverRate:    new(1.25),
			Amplitude:       new(2.5),
			UpdateTimestamp: new(float64(quoteAt.Unix())),
		})
	}

	return &qotgetbasicqotpb.Response{
		RetType: new(int32(0)),
		S2C: &qotgetbasicqotpb.S2C{
			BasicQotList: quotes,
		},
	}
}

func (s *marketDataQuoteOpenDServer) orderBookCallCount() int {
	return int(s.orderBookCalls.Load())
}

func (s *marketDataQuoteOpenDServer) orderBookLastNum() int32 {
	return s.lastOrderBookNum.Load()
}

func (s *marketDataQuoteOpenDServer) setOrderBook(bids []*qotcommonpb.OrderBook, asks []*qotcommonpb.OrderBook) {
	s.orderBookMu.Lock()
	defer s.orderBookMu.Unlock()
	s.orderBookBids = bids
	s.orderBookAsks = asks
	s.orderBookErr = nil
}

func (s *marketDataQuoteOpenDServer) setOrderBookErr(err error) {
	s.orderBookMu.Lock()
	defer s.orderBookMu.Unlock()
	s.orderBookBids = nil
	s.orderBookAsks = nil
	s.orderBookErr = err
}

func (s *marketDataQuoteOpenDServer) orderBookResponseBody(body []byte) []byte {
	bids, asks, obErr := s.orderBookFixture()
	if obErr != nil {
		return protocolErrorResponse(obErr)
	}

	request := &qotgetorderbookpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return protocolErrorResponse(err)
	}
	s.lastOrderBookNum.Store(request.GetC2S().GetNum())
	if bids == nil {
		bids = []*qotcommonpb.OrderBook{}
	}
	if asks == nil {
		asks = []*qotcommonpb.OrderBook{}
	}

	s2c, err := marshalOrderBookPayload(request.GetC2S().GetSecurity(), bids, asks)
	if err != nil {
		return nil
	}
	return successfulProtocolResponse(s2c)
}

func (s *marketDataQuoteOpenDServer) orderBookFixture() ([]*qotcommonpb.OrderBook, []*qotcommonpb.OrderBook, error) {
	s.orderBookMu.Lock()
	defer s.orderBookMu.Unlock()
	return s.orderBookBids, s.orderBookAsks, s.orderBookErr
}

func protocolErrorResponse(err error) []byte {
	var response []byte
	response = protowire.AppendTag(response, 1, protowire.VarintType)
	response = protowire.AppendVarint(response, 1)
	response = protowire.AppendTag(response, 2, protowire.BytesType)
	return protowire.AppendBytes(response, []byte(err.Error()))
}

func marshalOrderBookPayload(security *qotcommonpb.Security, bids, asks []*qotcommonpb.OrderBook) ([]byte, error) {
	securityBody, err := proto.Marshal(security)
	if err != nil {
		return nil, err
	}
	s2c := protowire.AppendTag(nil, 1, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, securityBody)
	for _, bid := range bids {
		bidBody, marshalErr := proto.Marshal(bid)
		if marshalErr != nil {
			return nil, marshalErr
		}
		s2c = protowire.AppendTag(s2c, 3, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, bidBody)
	}
	for _, ask := range asks {
		askBody, marshalErr := proto.Marshal(ask)
		if marshalErr != nil {
			return nil, marshalErr
		}
		s2c = protowire.AppendTag(s2c, 2, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, askBody)
	}
	quoteTime := time.Now().UTC().Format("2006-01-02 15:04:05.000")
	s2c = protowire.AppendTag(s2c, 4, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, []byte(quoteTime))
	s2c = protowire.AppendTag(s2c, 6, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, []byte(quoteTime))
	s2c = protowire.AppendTag(s2c, 8, protowire.BytesType)
	return protowire.AppendBytes(s2c, []byte(security.GetCode())), nil
}

func successfulProtocolResponse(s2c []byte) []byte {
	response := protowire.AppendTag(nil, 1, protowire.VarintType)
	response = protowire.AppendVarint(response, 0)
	response = protowire.AppendTag(response, 2, protowire.BytesType)
	response = protowire.AppendBytes(response, nil)
	response = protowire.AppendTag(response, 3, protowire.VarintType)
	response = protowire.AppendVarint(response, 0)
	response = protowire.AppendTag(response, 4, protowire.BytesType)
	return protowire.AppendBytes(response, s2c)
}

func marketDataDepthOrderBookFixture(price float64, volume int64, orderCount int32) *qotcommonpb.OrderBook {
	return &qotcommonpb.OrderBook{
		Price:       new(price),
		Volume:      new(volume),
		OrederCount: new(orderCount),
	}
}

func jftradePanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
