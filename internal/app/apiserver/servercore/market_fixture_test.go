package servercore

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
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
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
		var rawBody []byte
		switch frame.Header.ProtoID {
		case opend.ProtoInitConnect:
			response = &initpb.Response{
				RetType: new(int32(0)),
				S2C: &initpb.S2C{
					ServerVer:         new(int32(1008)),
					LoginUserID:       new(uint64(1)),
					ConnID:            new(uint64(42)),
					ConnAESKey:        new("0123456789abcdef"),
					KeepAliveInterval: new(int32(10)),
				},
			}
		case opend.ProtoGetGlobalState:
			response = marketDataGlobalStateResponse()
		case opend.ProtoQotSub:
			response = &qotsubpb.Response{RetType: new(int32(0))}
		case opend.ProtoGetBasicQot:
			s.basicQotCalls.Add(1)
			response = s.basicQotResponse(frame.Body)
		case opend.ProtoGetSecuritySnapshot:
			s.securitySnapshotCalls.Add(1)
			response = s.securitySnapshotResponse(frame.Body)
		case opend.ProtoGetStaticInfo:
			s.staticInfoCalls.Add(1)
			response = s.staticInfoResponse(frame.Body)
		case opend.ProtoRequestHistoryKL:
			response = s.historyKLResponse(frame.Body)
		case opend.ProtoGetKL:
			s.currentKLCalls.Add(1)
			response = s.currentKLResponse(frame.Body)
		case opend.ProtoGetOrderBook:
			s.orderBookCalls.Add(1)
			rawBody = s.orderBookResponseBody(frame.Body)
		default:
			return
		}

		body := rawBody
		if body == nil {
			var err error
			body, err = proto.Marshal(response)
			if err != nil {
				return
			}
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
			ServerVer:      new(int32(1008)),
			ServerBuildNo:  new(int32(6808)),
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
	s.orderBookMu.Lock()
	bids := s.orderBookBids
	asks := s.orderBookAsks
	obErr := s.orderBookErr
	s.orderBookMu.Unlock()

	if obErr != nil {
		var response []byte
		response = protowire.AppendTag(response, 1, protowire.VarintType)
		response = protowire.AppendVarint(response, 1)
		response = protowire.AppendTag(response, 2, protowire.BytesType)
		response = protowire.AppendBytes(response, []byte(obErr.Error()))
		return response
	}

	request := &qotgetorderbookpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		var response []byte
		response = protowire.AppendTag(response, 1, protowire.VarintType)
		response = protowire.AppendVarint(response, 1)
		response = protowire.AppendTag(response, 2, protowire.BytesType)
		response = protowire.AppendBytes(response, []byte(err.Error()))
		return response
	}
	s.lastOrderBookNum.Store(request.GetC2S().GetNum())

	if bids == nil {
		bids = []*qotcommonpb.OrderBook{}
	}
	if asks == nil {
		asks = []*qotcommonpb.OrderBook{}
	}

	securityBody, err := proto.Marshal(request.GetC2S().GetSecurity())
	if err != nil {
		return nil
	}
	bidTime := time.Now().UTC().Format("2006-01-02 15:04:05.000")
	askTime := bidTime

	var s2c []byte
	s2c = protowire.AppendTag(s2c, 1, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, securityBody)
	for _, bid := range bids {
		bidBody, err := proto.Marshal(bid)
		if err != nil {
			return nil
		}
		s2c = protowire.AppendTag(s2c, 3, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, bidBody)
	}
	for _, ask := range asks {
		askBody, err := proto.Marshal(ask)
		if err != nil {
			return nil
		}
		s2c = protowire.AppendTag(s2c, 2, protowire.BytesType)
		s2c = protowire.AppendBytes(s2c, askBody)
	}
	s2c = protowire.AppendTag(s2c, 4, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, []byte(bidTime))
	s2c = protowire.AppendTag(s2c, 6, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, []byte(askTime))
	s2c = protowire.AppendTag(s2c, 8, protowire.BytesType)
	s2c = protowire.AppendBytes(s2c, []byte(request.GetC2S().GetSecurity().GetCode()))

	var response []byte
	response = protowire.AppendTag(response, 1, protowire.VarintType)
	response = protowire.AppendVarint(response, 0)
	response = protowire.AppendTag(response, 2, protowire.BytesType)
	response = protowire.AppendBytes(response, nil)
	response = protowire.AppendTag(response, 3, protowire.VarintType)
	response = protowire.AppendVarint(response, 0)
	response = protowire.AppendTag(response, 4, protowire.BytesType)
	response = protowire.AppendBytes(response, s2c)
	return response
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
