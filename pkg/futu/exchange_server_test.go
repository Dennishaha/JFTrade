package futu

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	qotgetorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
	qotgetstaticinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetstaticinfo"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	qotupdatebasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdatebasicqot"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
	trdsubaccpushpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdsubaccpush"
	tradeunlockpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdunlocktrade"
)

type quoteOpenDServer struct {
	addr                  string
	accepts               atomic.Int32
	initRecvNotify        atomic.Bool
	serverVer             atomic.Int32
	serverBuildNo         atomic.Int32
	accountListCalls      atomic.Int32
	fundsCalls            atomic.Int32
	positionListCalls     atomic.Int32
	orderListCalls        atomic.Int32
	historyOrderListCalls atomic.Int32
	historyFillListCalls  atomic.Int32
	orderFeeCalls         atomic.Int32
	marginRatioCalls      atomic.Int32
	flowSummaryCalls      atomic.Int32
	maxTrdQtysCalls       atomic.Int32
	placeOrderCalls       atomic.Int32
	modifyOrderCalls      atomic.Int32
	unlockTradeCalls      atomic.Int32
	tradeAccPushCalls     atomic.Int32
	qotSubCalls           atomic.Int32
	pushSubCalls          atomic.Int32
	basicQotCalls         atomic.Int32
	staticInfoCalls       atomic.Int32
	securitySnapshotCalls atomic.Int32
	orderBookCalls        atomic.Int32
	accountMu             sync.Mutex
	accounts              []*trdcommonpb.TrdAcc
	tradeMu               sync.Mutex
	funds                 *trdcommonpb.Funds
	positions             []*trdcommonpb.Position
	orders                []*trdcommonpb.Order
	historyOrders         []*trdcommonpb.Order
	orderFills            []*trdcommonpb.OrderFill
	historyFills          []*trdcommonpb.OrderFill
	orderFees             []*trdcommonpb.OrderFee
	marginRatios          []*trdgetmarginratiopb.MarginRatioInfo
	strictMarginRatios    bool
	cashFlows             []*trdflowsummarypb.FlowSummaryInfo
	maxTrdQtys            *trdcommonpb.MaxTrdQtys
	placedOrderID         uint64
	placedOrderIDEx       string
	lastPlaceOrder        *trdplaceorderpb.C2S
	lastModifyOrder       *trdmodifyorderpb.C2S
	lastUnlockTrade       *tradeunlockpb.C2S
	lastOrderBook         *qotgetorderbookpb.C2S
	lastTradeAccPushIDs   []uint64
	lastMaxTrdQtys        *trdgetmaxtrdqtyspb.C2S
	historyKLCalls        atomic.Int32
	currentKLCalls        atomic.Int32
	historyExtended       atomic.Bool
	historySession        atomic.Int32
	historyMu             sync.Mutex
	historyPages          [][]*qotcommonpb.KLine
	historySeries         []*qotcommonpb.KLine
	historyPagesBySession map[int32][][]*qotcommonpb.KLine
	historySessionErrors  map[int32]*historypb.Response
	historySessionCallLog []int32
	historyRouteCallCount map[int32]int
	currentKLines         []*qotcommonpb.KLine
	basicQuotes           []*qotcommonpb.BasicQot
	staticInfos           []*qotcommonpb.SecurityStaticInfo
	staticInfoError       *qotgetstaticinfopb.Response
	securitySnapshots     []*qotgetsecuritysnapshotpb.Snapshot
	orderBookSnapshot     *qotgetorderbookpb.S2C
	notifyMu              sync.Mutex
	notifyAfterInit       *notifypb.Response
	listener              net.Listener
	stopOnce              sync.Once
	shutdownCompleted     chan struct{}
}

func (s *quoteOpenDServer) setHistoryPages(pages [][]*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = pages
	s.historySeries = nil
	s.historyPagesBySession = nil
	s.historySessionErrors = nil
	s.historySessionCallLog = nil
	s.historyRouteCallCount = nil
}

func (s *quoteOpenDServer) setHistorySeries(series []*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = nil
	s.historySeries = append([]*qotcommonpb.KLine(nil), series...)
	s.historyPagesBySession = nil
	s.historySessionErrors = nil
	s.historySessionCallLog = nil
	s.historyRouteCallCount = nil
}

func (s *quoteOpenDServer) setHistoryPagesBySession(pages map[int32][][]*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = nil
	s.historySeries = nil
	s.historySessionErrors = nil
	s.historySessionCallLog = nil
	s.historyRouteCallCount = make(map[int32]int, len(pages))
	s.historyPagesBySession = make(map[int32][][]*qotcommonpb.KLine, len(pages))
	for session, sessionPages := range pages {
		clonedPages := make([][]*qotcommonpb.KLine, 0, len(sessionPages))
		for _, page := range sessionPages {
			clonedPages = append(clonedPages, append([]*qotcommonpb.KLine(nil), page...))
		}
		s.historyPagesBySession[session] = clonedPages
	}
}

func (s *quoteOpenDServer) setHistorySessionError(session int32, retType int32, retMsg string) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	if s.historySessionErrors == nil {
		s.historySessionErrors = make(map[int32]*historypb.Response)
	}
	s.historySessionErrors[session] = &historypb.Response{
		RetType: new(retType),
		RetMsg:  new(retMsg),
	}
}

func (s *quoteOpenDServer) setCurrentKLines(klines []*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.currentKLines = klines
}

func (s *quoteOpenDServer) setNotifyAfterInit(response *notifypb.Response) {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	s.notifyAfterInit = response
}

func (s *quoteOpenDServer) setAccounts(accounts []*trdcommonpb.TrdAcc) {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	s.accounts = append([]*trdcommonpb.TrdAcc(nil), accounts...)
}

func (s *quoteOpenDServer) setFunds(funds *trdcommonpb.Funds) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.funds = funds
}

func (s *quoteOpenDServer) setOrders(orders []*trdcommonpb.Order) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.orders = append([]*trdcommonpb.Order(nil), orders...)
}

func (s *quoteOpenDServer) setHistoryOrders(orders []*trdcommonpb.Order) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.historyOrders = append([]*trdcommonpb.Order(nil), orders...)
}

func (s *quoteOpenDServer) setHistoryFills(fills []*trdcommonpb.OrderFill) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.historyFills = append([]*trdcommonpb.OrderFill(nil), fills...)
}

func (s *quoteOpenDServer) setOrderFees(fees []*trdcommonpb.OrderFee) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.orderFees = append([]*trdcommonpb.OrderFee(nil), fees...)
}

func (s *quoteOpenDServer) setMarginRatios(ratios []*trdgetmarginratiopb.MarginRatioInfo) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.marginRatios = append([]*trdgetmarginratiopb.MarginRatioInfo(nil), ratios...)
}

func (s *quoteOpenDServer) setStrictMarginRatios(enabled bool) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.strictMarginRatios = enabled
}

func (s *quoteOpenDServer) setCashFlows(flows []*trdflowsummarypb.FlowSummaryInfo) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.cashFlows = append([]*trdflowsummarypb.FlowSummaryInfo(nil), flows...)
}

func (s *quoteOpenDServer) setMaxTrdQtys(maxQtys *trdcommonpb.MaxTrdQtys) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.maxTrdQtys = maxQtys
}

func (s *quoteOpenDServer) setPlacedOrderResponse(orderID uint64, orderIDEx string) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()
	s.placedOrderID = orderID
	s.placedOrderIDEx = orderIDEx
}

func startQuoteOpenDServer(t *testing.T) *quoteOpenDServer {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &quoteOpenDServer{
		addr:              listener.Addr().String(),
		listener:          listener,
		shutdownCompleted: make(chan struct{}),
	}
	server.serverVer.Store(1008)
	server.serverBuildNo.Store(6808)
	go server.acceptLoop()
	return server
}

func (s *quoteOpenDServer) stop() {
	s.stopOnce.Do(func() {
		jftradeErr1 := s.listener.Close()
		jftradePanicOnError(jftradeErr1)
		<-s.shutdownCompleted
	})
}

func (s *quoteOpenDServer) acceptLoop() {
	defer close(s.shutdownCompleted)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.accepts.Add(1)
		go s.handleConn(conn)
	}
}

//nolint:funlen
func (s *quoteOpenDServer) handleConn(conn net.Conn) {
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
		var responseBody []byte
		switch frame.Header.ProtoID {
		case opend.ProtoInitConnect:
			request := &initpb.Request{}
			jftradeErr3 := proto.Unmarshal(frame.Body, request)
			jftradePanicOnError(jftradeErr3)
			s.initRecvNotify.Store(request.GetC2S().GetRecvNotify())
			response = &initpb.Response{
				RetType: new(int32(0)),
				S2C: &initpb.S2C{
					ServerVer:         new(s.serverVer.Load()),
					LoginUserID:       new(uint64(1)),
					ConnID:            new(uint64(42)),
					ConnAESKey:        new("0123456789abcdef"),
					KeepAliveInterval: new(int32(10)),
				},
			}
		case opend.ProtoGetGlobalState:
			response = testGlobalStateResponse(s.serverVer.Load(), s.serverBuildNo.Load())
		case opend.ProtoQotSub:
			s.qotSubCalls.Add(1)
			request := &qotsubpb.Request{}
			jftradeErr4 := proto.Unmarshal(frame.Body, request)
			jftradePanicOnError(jftradeErr4)
			isPushSub := request.GetC2S().GetIsRegOrUnRegPush()
			if isPushSub {
				s.pushSubCalls.Add(1)
			}
			response = &qotsubpb.Response{RetType: new(int32(0))}
		case opend.ProtoTrdGetAccList:
			s.accountListCalls.Add(1)
			response = s.accountListResponse()
		case opend.ProtoTrdGetFunds:
			s.fundsCalls.Add(1)
			response = s.fundsResponse(frame.Body)
		case opend.ProtoTrdGetPositionList:
			s.positionListCalls.Add(1)
			response = s.positionListResponse(frame.Body)
		case opend.ProtoTrdGetOrderList:
			s.orderListCalls.Add(1)
			response = s.orderListResponse(frame.Body)
		case opend.ProtoTrdGetOrderFillList:
			s.orderListCalls.Add(1)
			response = s.orderFillListResponse(frame.Body)
		case opend.ProtoTrdGetHistoryOrderList:
			s.historyOrderListCalls.Add(1)
			response = s.historyOrderListResponse(frame.Body)
		case opend.ProtoTrdGetHistoryOrderFillList:
			s.historyFillListCalls.Add(1)
			response = s.historyOrderFillListResponse(frame.Body)
		case opend.ProtoTrdGetOrderFee:
			s.orderFeeCalls.Add(1)
			response = s.orderFeeResponse(frame.Body)
		case opend.ProtoTrdGetMarginRatio:
			s.marginRatioCalls.Add(1)
			response = s.marginRatioResponse(frame.Body)
		case opend.ProtoTrdFlowSummary:
			s.flowSummaryCalls.Add(1)
			response = s.flowSummaryResponse(frame.Body)
		case opend.ProtoTrdGetMaxTrdQtys:
			s.maxTrdQtysCalls.Add(1)
			response = s.maxTrdQtysResponse(frame.Body)
		case opend.ProtoTrdPlaceOrder:
			s.placeOrderCalls.Add(1)
			response = s.placeOrderResponse(frame.Body)
		case opend.ProtoTrdModifyOrder:
			s.modifyOrderCalls.Add(1)
			response = s.modifyOrderResponse(frame.Body)
		case opend.ProtoTrdUnlockTrade:
			s.unlockTradeCalls.Add(1)
			response = s.unlockTradeResponse(frame.Body)
		case opend.ProtoTrdSubAccPush:
			s.tradeAccPushCalls.Add(1)
			request := &trdsubaccpushpb.Request{}
			jftradeErr5 := proto.Unmarshal(frame.Body, request)
			jftradePanicOnError(jftradeErr5)
			s.tradeMu.Lock()
			s.lastTradeAccPushIDs = append([]uint64(nil), request.GetC2S().GetAccIDList()...)
			s.tradeMu.Unlock()
			response = &trdsubaccpushpb.Response{RetType: new(int32(0))}
		case opend.ProtoGetBasicQot:
			s.basicQotCalls.Add(1)
			response = s.basicQotResponse(frame.Body)
		case opend.ProtoGetStaticInfo:
			s.staticInfoCalls.Add(1)
			response = s.staticInfoResponse(frame.Body)
		case opend.ProtoGetSecuritySnapshot:
			s.securitySnapshotCalls.Add(1)
			response = s.securitySnapshotResponse(frame.Body)
		case opend.ProtoGetOrderBook:
			s.orderBookCalls.Add(1)
			var err error
			responseBody, err = s.orderBookResponseBody(frame.Body)
			if err != nil {
				return
			}
		case opend.ProtoGetKL:
			s.currentKLCalls.Add(1)
			response = s.currentKLResponse(frame.Body)
		case opend.ProtoRequestHistoryKL:
			s.historyKLCalls.Add(1)
			response = s.historyKLResponse(frame.Body)
		default:
			return
		}

		var body []byte
		if responseBody != nil {
			body = responseBody
		} else {
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
		if frame.Header.ProtoID == opend.ProtoInitConnect {
			if err := s.writeNotifyAfterInit(conn); err != nil {
				return
			}
		}
		if frame.Header.ProtoID == opend.ProtoQotSub {
			request := &qotsubpb.Request{}
			jftradeErr6 := proto.Unmarshal(frame.Body, request)
			jftradePanicOnError(jftradeErr6)
			if request.GetC2S().GetIsRegOrUnRegPush() {
				if err := s.writeBasicQotPush(conn, request.GetC2S().GetSecurityList()); err != nil {
					return
				}
			}
		}
	}
}

func testGlobalStateResponse(serverVer, serverBuildNo int32) *globalpb.Response {
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
			ServerVer:      &serverVer,
			ServerBuildNo:  &serverBuildNo,
			Time:           new(time.Now().Unix()),
		},
	}
}

func (s *quoteOpenDServer) writeNotifyAfterInit(conn net.Conn) error {
	s.notifyMu.Lock()
	response := s.notifyAfterInit
	s.notifyMu.Unlock()
	if response == nil {
		return nil
	}

	time.Sleep(25 * time.Millisecond)
	body, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	packet, err := codec.Encode(opend.ProtoNotify, 0, body)
	if err != nil {
		return err
	}
	_, err = conn.Write(packet)
	return err
}

//nolint:funlen
func (s *quoteOpenDServer) historyKLResponse(body []byte) *historypb.Response {
	request := &historypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &historypb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.historyExtended.Store(request.GetC2S().GetExtendedTime())
	s.historySession.Store(request.GetC2S().GetSession())
	s.historyMu.Lock()
	s.historySessionCallLog = append(s.historySessionCallLog, request.GetC2S().GetSession())
	if response := s.historySessionErrors[request.GetC2S().GetSession()]; response != nil {
		s.historyMu.Unlock()
		return response
	}
	if len(s.historyPagesBySession) > 0 {
		session := request.GetC2S().GetSession()
		pages := s.historyPagesBySession[session]
		pageIndex := s.historyRouteCallCount[session]
		if pageIndex >= len(pages) && len(pages) > 0 {
			pageIndex = len(pages) - 1
		}
		s.historyRouteCallCount[session]++
		response := &historypb.Response{
			RetType: new(int32(0)),
			S2C: &historypb.S2C{
				Security: request.GetC2S().GetSecurity(),
			},
		}
		if len(pages) > 0 {
			response.S2C.KlList = pages[pageIndex]
			if pageIndex < len(pages)-1 {
				response.S2C.NextReqKey = []byte{byte(pageIndex + 1)}
			}
		}
		s.historyMu.Unlock()
		return response
	}
	if len(s.historySeries) > 0 {
		pageSize := int(request.GetC2S().GetMaxAckKLNum())
		if pageSize <= 0 {
			pageSize = len(s.historySeries)
		}
		pageIndex := 0
		if nextReqKey := request.GetC2S().GetNextReqKey(); len(nextReqKey) > 0 {
			pageIndex = int(nextReqKey[0])
		}
		start := min(pageIndex*pageSize, len(s.historySeries))
		end := min(start+pageSize, len(s.historySeries))
		response := &historypb.Response{
			RetType: new(int32(0)),
			S2C: &historypb.S2C{
				Security: request.GetC2S().GetSecurity(),
				KlList:   append([]*qotcommonpb.KLine(nil), s.historySeries[start:end]...),
			},
		}
		if end < len(s.historySeries) {
			response.S2C.NextReqKey = []byte{byte(pageIndex + 1)}
		}
		s.historyMu.Unlock()
		return response
	}
	if len(s.historyPages) > 0 {
		pageIndex := max(int(s.historyKLCalls.Load())-1, 0)
		if pageIndex >= len(s.historyPages) {
			pageIndex = len(s.historyPages) - 1
		}
		response := &historypb.Response{
			RetType: new(int32(0)),
			S2C: &historypb.S2C{
				Security: request.GetC2S().GetSecurity(),
				KlList:   s.historyPages[pageIndex],
			},
		}
		if pageIndex < len(s.historyPages)-1 {
			response.S2C.NextReqKey = []byte{byte(pageIndex + 1)}
		}
		s.historyMu.Unlock()
		return response
	}
	s.historyMu.Unlock()

	startAt := time.Date(2026, time.May, 20, 8, 0, 0, 0, time.UTC)
	return &historypb.Response{
		RetType: new(int32(0)),
		S2C: &historypb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList: []*qotcommonpb.KLine{
				{
					Time:       new(startAt.Format("2006-01-02 15:04:05")),
					Timestamp:  new(float64(startAt.Unix())),
					IsBlank:    new(false),
					OpenPrice:  new(float64(100)),
					HighPrice:  new(float64(101)),
					LowPrice:   new(float64(99)),
					ClosePrice: new(100.5),
					Volume:     new(int64(1000)),
					Turnover:   new(float64(100500)),
				},
			},
		},
	}
}

func (s *quoteOpenDServer) currentKLResponse(body []byte) *qotgetklpb.Response {
	request := &qotgetklpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetklpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	response := &qotgetklpb.Response{
		RetType: new(int32(0)),
		S2C: &qotgetklpb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList:   s.currentKLines,
		},
	}
	return response
}

func testHistoryKLine(at time.Time, price float64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       new(at.Format("2006-01-02 15:04:05")),
		Timestamp:  new(float64(at.Unix())),
		IsBlank:    new(false),
		OpenPrice:  new(price),
		HighPrice:  new(price + 1),
		LowPrice:   new(price - 1),
		ClosePrice: new(price + 0.5),
		Volume:     new(int64(1000)),
		Turnover:   new(price * 1000),
	}
}

func testCurrentKLine(at time.Time, open float64, high float64, low float64, close float64, volume int64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       new(at.Format("2006-01-02 15:04:05")),
		Timestamp:  new(float64(at.Unix())),
		IsBlank:    new(false),
		OpenPrice:  new(open),
		HighPrice:  new(high),
		LowPrice:   new(low),
		ClosePrice: new(close),
		Volume:     new(volume),
		Turnover:   new(close * float64(volume)),
	}
}

func (s *quoteOpenDServer) writeBasicQotPush(conn net.Conn, securities []*qotcommonpb.Security) error {
	response := &qotupdatebasicqotpb.Response{
		RetType: new(int32(0)),
		S2C:     &qotupdatebasicqotpb.S2C{BasicQotList: s.basicQotResponseForSecurities(securities)},
	}
	body, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	packet, err := codec.Encode(opend.ProtoQotUpdateBasicQot, 0, body)
	if err != nil {
		return err
	}
	_, err = conn.Write(packet)
	return err
}

func (s *quoteOpenDServer) basicQotResponse(body []byte) *qotgetbasicqotpb.Response {
	request := &qotgetbasicqotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetbasicqotpb.Response{RetType: new(int32(1)), RetMsg: new(err.Error())}
	}

	quotes := s.basicQotResponseForSecurities(request.GetC2S().GetSecurityList())

	return &qotgetbasicqotpb.Response{
		RetType: new(int32(0)),
		S2C: &qotgetbasicqotpb.S2C{
			BasicQotList: quotes,
		},
	}
}
