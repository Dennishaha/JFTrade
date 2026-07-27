package testkit

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	globalpb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetacclistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetacclist"
	trdgetfundspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetfunds"
	trdgethistoryorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderfilllist"
	trdgethistoryorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderlist"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
	trdgetorderfeepb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfee"
	trdgetorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfilllist"
	trdgetorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderlist"
	trdgetpositionlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetpositionlist"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
	trdsubaccpushpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdsubaccpush"
)

type brokerRouteOpenDServer struct {
	addr              string
	listener          net.Listener
	shutdownCompleted chan struct{}
	stopOnce          sync.Once
	mu                sync.Mutex
	accounts          []*trdcommonpb.TrdAcc
	funds             *trdcommonpb.Funds
	positions         []*trdcommonpb.Position
	orders            []*trdcommonpb.Order
	historyOrders     []*trdcommonpb.Order
	orderFills        []*trdcommonpb.OrderFill
	historyFills      []*trdcommonpb.OrderFill
	orderFees         []*trdcommonpb.OrderFee
	marginRatios      []*trdgetmarginratiopb.MarginRatioInfo
	cashFlows         []*trdflowsummarypb.FlowSummaryInfo
	maxTrdQtys        *trdcommonpb.MaxTrdQtys
	placedOrderID     uint64
	placedOrderIDEx   string
	lastPlaceOrder    *trdplaceorderpb.C2S
	lastModifyOrder   *trdmodifyorderpb.C2S
	lastMaxTrdQtys    *trdgetmaxtrdqtyspb.C2S
	serverVer         int32
	serverBuildNo     int32
	placeOrderCalls   int
	modifyOrderCalls  int
	subAccPushCalls   int
}

func startBrokerRouteOpenDServer(t *testing.T) *brokerRouteOpenDServer {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &brokerRouteOpenDServer{
		addr:              listener.Addr().String(),
		listener:          listener,
		shutdownCompleted: make(chan struct{}),
		serverVer:         1009,
		serverBuildNo:     6908,
	}
	go server.acceptLoop()
	return server
}

func (s *brokerRouteOpenDServer) setServerVersion(serverVer, serverBuildNo int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverVer = serverVer
	s.serverBuildNo = serverBuildNo
}

func (s *brokerRouteOpenDServer) stop() {
	s.stopOnce.Do(func() {
		jftradeErr1 := s.listener.Close()
		jftradePanicOnError(jftradeErr1)
		<-s.shutdownCompleted
	})
}

func (s *brokerRouteOpenDServer) setAccounts(accounts []*trdcommonpb.TrdAcc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts = append([]*trdcommonpb.TrdAcc(nil), accounts...)
}

func (s *brokerRouteOpenDServer) setFunds(funds *trdcommonpb.Funds) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.funds = funds
}

func (s *brokerRouteOpenDServer) setPositions(positions []*trdcommonpb.Position) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.positions = append([]*trdcommonpb.Position(nil), positions...)
}

func (s *brokerRouteOpenDServer) setOrders(orders []*trdcommonpb.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders = append([]*trdcommonpb.Order(nil), orders...)
}

func (s *brokerRouteOpenDServer) setHistoryOrders(orders []*trdcommonpb.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.historyOrders = append([]*trdcommonpb.Order(nil), orders...)
}

func (s *brokerRouteOpenDServer) setOrderFills(fills []*trdcommonpb.OrderFill) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orderFills = append([]*trdcommonpb.OrderFill(nil), fills...)
}

func (s *brokerRouteOpenDServer) setHistoryFills(fills []*trdcommonpb.OrderFill) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.historyFills = append([]*trdcommonpb.OrderFill(nil), fills...)
}

func (s *brokerRouteOpenDServer) setOrderFees(fees []*trdcommonpb.OrderFee) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orderFees = append([]*trdcommonpb.OrderFee(nil), fees...)
}

func (s *brokerRouteOpenDServer) setMarginRatios(ratios []*trdgetmarginratiopb.MarginRatioInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marginRatios = append([]*trdgetmarginratiopb.MarginRatioInfo(nil), ratios...)
}

func (s *brokerRouteOpenDServer) setCashFlows(flows []*trdflowsummarypb.FlowSummaryInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cashFlows = append([]*trdflowsummarypb.FlowSummaryInfo(nil), flows...)
}

func (s *brokerRouteOpenDServer) setMaxTrdQtys(maxQtys *trdcommonpb.MaxTrdQtys) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxTrdQtys = maxQtys
}

func (s *brokerRouteOpenDServer) setPlacedOrderResponse(orderID uint64, orderIDEx string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.placedOrderID = orderID
	s.placedOrderIDEx = orderIDEx
}

func (s *brokerRouteOpenDServer) placeOrderCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.placeOrderCalls
}

func (s *brokerRouteOpenDServer) modifyOrderCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modifyOrderCalls
}

func (s *brokerRouteOpenDServer) lastPlaceOrderRequest() *trdplaceorderpb.C2S {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastPlaceOrder == nil {
		return nil
	}
	return jftradeCheckedTypeAssertion[*trdplaceorderpb.C2S](proto.Clone(s.lastPlaceOrder))
}

func (s *brokerRouteOpenDServer) subAccPushCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subAccPushCalls
}

func (s *brokerRouteOpenDServer) acceptLoop() {
	defer close(s.shutdownCompleted)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *brokerRouteOpenDServer) handleConn(conn net.Conn) {
	defer func() { jftradePanicOnError(conn.Close()) }()
	for {
		header := make([]byte, codec.HeaderLen)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		bodyLen := int(binary.LittleEndian.Uint32(header[12:16]))
		packet := make([]byte, codec.HeaderLen+bodyLen)
		copy(packet, header)
		if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
			return
		}
		frame, err := codec.Decode(packet)
		if err != nil {
			return
		}

		response := s.responseForFrame(frame)
		body, err := proto.Marshal(response)
		if err != nil {
			return
		}
		encoded, err := codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
		if err != nil {
			return
		}
		if _, err := conn.Write(encoded); err != nil {
			return
		}
	}
}

func (s *brokerRouteOpenDServer) responseForFrame(frame codec.Frame) proto.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dispatchResponseForFrame(frame)
}

func (s *brokerRouteOpenDServer) dispatchResponseForFrame(frame codec.Frame) proto.Message {
	switch frame.Header.ProtoID {
	case opend.ProtoInitConnect:
		return s.initConnectResponse()
	case opend.ProtoGetGlobalState:
		return brokerRouteGlobalStateResponse(s.serverVer, s.serverBuildNo)
	case opend.ProtoTrdGetAccList:
		return &trdgetacclistpb.Response{RetType: new(int32(0)), S2C: &trdgetacclistpb.S2C{
			AccList: append([]*trdcommonpb.TrdAcc(nil), s.accounts...),
		}}
	case opend.ProtoTrdGetFunds:
		return s.fundsResponse(frame.Body)
	case opend.ProtoTrdGetPositionList:
		return s.positionsResponse(frame.Body)
	case opend.ProtoTrdGetOrderList:
		return s.ordersResponse(frame.Body)
	case opend.ProtoTrdGetHistoryOrderList:
		return s.historyOrdersResponse(frame.Body)
	case opend.ProtoTrdGetOrderFillList:
		return s.orderFillsResponse(frame.Body)
	case opend.ProtoTrdGetHistoryOrderFillList:
		return s.historyOrderFillsResponse(frame.Body)
	case opend.ProtoTrdGetOrderFee:
		return s.orderFeesResponse(frame.Body)
	case opend.ProtoTrdGetMarginRatio:
		return s.marginRatiosResponse(frame.Body)
	case opend.ProtoTrdFlowSummary:
		return s.cashFlowsResponse(frame.Body)
	case opend.ProtoTrdGetMaxTrdQtys:
		return s.maxTradeQuantitiesResponse(frame.Body)
	case opend.ProtoTrdPlaceOrder:
		return s.placeOrderResponse(frame.Body)
	case opend.ProtoTrdModifyOrder:
		return s.modifyOrderResponse(frame.Body)
	case opend.ProtoTrdSubAccPush:
		s.subAccPushCalls++
		return &trdsubaccpushpb.Response{RetType: new(int32(0))}
	default:
		return &initpb.Response{RetType: new(int32(1)), RetMsg: new("unsupported proto")}
	}
}

func (s *brokerRouteOpenDServer) initConnectResponse() *initpb.Response {
	return &initpb.Response{RetType: new(int32(0)), S2C: &initpb.S2C{
		ServerVer: new(s.serverVer), LoginUserID: new(uint64(1)), ConnID: new(uint64(42)),
		ConnAESKey: new("0123456789abcdef"), KeepAliveInterval: new(int32(10)),
	}}
}

func (s *brokerRouteOpenDServer) fundsResponse(body []byte) *trdgetfundspb.Response {
	request := &trdgetfundspb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	return &trdgetfundspb.Response{RetType: new(int32(0)), S2C: &trdgetfundspb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
		Funds:  normalizeBrokerRouteFunds(s.funds),
	}}
}

func (s *brokerRouteOpenDServer) positionsResponse(body []byte) *trdgetpositionlistpb.Response {
	request := &trdgetpositionlistpb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	positions := make([]*trdcommonpb.Position, 0, len(s.positions))
	for _, position := range s.positions {
		positions = append(positions, normalizeBrokerRoutePosition(position))
	}
	return &trdgetpositionlistpb.Response{RetType: new(int32(0)), S2C: &trdgetpositionlistpb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), PositionList: positions,
	}}
}

func (s *brokerRouteOpenDServer) ordersResponse(body []byte) *trdgetorderlistpb.Response {
	request := &trdgetorderlistpb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	orders := filterBrokerRouteOrders(s.orders, request.GetC2S().GetFilterConditions(), nil)
	return &trdgetorderlistpb.Response{RetType: new(int32(0)), S2C: &trdgetorderlistpb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), OrderList: orders,
	}}
}

func (s *brokerRouteOpenDServer) historyOrdersResponse(body []byte) *trdgethistoryorderlistpb.Response {
	request := &trdgethistoryorderlistpb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	orders := filterBrokerRouteOrders(
		s.historyOrders,
		request.GetC2S().GetFilterConditions(),
		request.GetC2S().GetFilterStatusList(),
	)
	return &trdgethistoryorderlistpb.Response{RetType: new(int32(0)), S2C: &trdgethistoryorderlistpb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), OrderList: orders,
	}}
}

func (s *brokerRouteOpenDServer) orderFillsResponse(body []byte) *trdgetorderfilllistpb.Response {
	request := &trdgetorderfilllistpb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	fills := filterBrokerRouteFills(s.orderFills, request.GetC2S().GetFilterConditions())
	return &trdgetorderfilllistpb.Response{RetType: new(int32(0)), S2C: &trdgetorderfilllistpb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), OrderFillList: fills,
	}}
}

func (s *brokerRouteOpenDServer) historyOrderFillsResponse(body []byte) *trdgethistoryorderfilllistpb.Response {
	request := &trdgethistoryorderfilllistpb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	fills := filterBrokerRouteFills(s.historyFills, request.GetC2S().GetFilterConditions())
	return &trdgethistoryorderfilllistpb.Response{RetType: new(int32(0)), S2C: &trdgethistoryorderfilllistpb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), OrderFillList: fills,
	}}
}

func (s *brokerRouteOpenDServer) orderFeesResponse(body []byte) *trdgetorderfeepb.Response {
	request := &trdgetorderfeepb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	fees := filterBrokerRouteFees(s.orderFees, request.GetC2S().GetOrderIdExList())
	return &trdgetorderfeepb.Response{RetType: new(int32(0)), S2C: &trdgetorderfeepb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), OrderFeeList: fees,
	}}
}

func filterBrokerRouteFees(input []*trdcommonpb.OrderFee, orderIDs []string) []*trdcommonpb.OrderFee {
	requested := make(map[string]struct{}, len(orderIDs))
	for _, orderID := range orderIDs {
		requested[strings.ToUpper(strings.TrimSpace(orderID))] = struct{}{}
	}
	fees := make([]*trdcommonpb.OrderFee, 0, len(input))
	for _, fee := range input {
		if fee == nil {
			continue
		}
		if _, required := requested[strings.ToUpper(strings.TrimSpace(fee.GetOrderIDEx()))]; len(requested) > 0 && !required {
			continue
		}
		fees = append(fees, normalizeBrokerRouteOrderFee(fee))
	}
	return fees
}

func (s *brokerRouteOpenDServer) marginRatiosResponse(body []byte) *trdgetmarginratiopb.Response {
	request := &trdgetmarginratiopb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	ratios := filterBrokerRouteMarginRatios(s.marginRatios, request.GetC2S().GetSecurityList())
	return &trdgetmarginratiopb.Response{RetType: new(int32(0)), S2C: &trdgetmarginratiopb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), MarginRatioInfoList: ratios,
	}}
}

func filterBrokerRouteMarginRatios(
	input []*trdgetmarginratiopb.MarginRatioInfo,
	securities []*qotcommonpb.Security,
) []*trdgetmarginratiopb.MarginRatioInfo {
	requested := make(map[string]struct{}, len(securities))
	for _, security := range securities {
		requested[strings.ToUpper(strings.TrimSpace(security.GetCode()))] = struct{}{}
	}
	ratios := make([]*trdgetmarginratiopb.MarginRatioInfo, 0, len(input))
	for _, ratio := range input {
		if ratio == nil {
			continue
		}
		code := strings.ToUpper(strings.TrimSpace(ratio.GetSecurity().GetCode()))
		if _, required := requested[code]; len(requested) > 0 && !required {
			continue
		}
		ratios = append(ratios, normalizeBrokerRouteMarginRatio(ratio))
	}
	return ratios
}

func (s *brokerRouteOpenDServer) cashFlowsResponse(body []byte) *trdflowsummarypb.Response {
	request := &trdflowsummarypb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	flows := filterBrokerRouteCashFlows(s.cashFlows, request.GetC2S().GetCashFlowDirection())
	return &trdflowsummarypb.Response{RetType: new(int32(0)), S2C: &trdflowsummarypb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), FlowSummaryInfoList: flows,
	}}
}

func filterBrokerRouteCashFlows(input []*trdflowsummarypb.FlowSummaryInfo, direction int32) []*trdflowsummarypb.FlowSummaryInfo {
	flows := make([]*trdflowsummarypb.FlowSummaryInfo, 0, len(input))
	for _, flow := range input {
		if flow == nil || (direction != 0 && flow.GetCashFlowDirection() != direction) {
			continue
		}
		flows = append(flows, normalizeBrokerRouteCashFlow(flow))
	}
	return flows
}

func (s *brokerRouteOpenDServer) maxTradeQuantitiesResponse(body []byte) *trdgetmaxtrdqtyspb.Response {
	request := &trdgetmaxtrdqtyspb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	if request.GetC2S() != nil {
		s.lastMaxTrdQtys = jftradeCheckedTypeAssertion[*trdgetmaxtrdqtyspb.C2S](proto.Clone(request.GetC2S()))
	}
	return &trdgetmaxtrdqtyspb.Response{RetType: new(int32(0)), S2C: &trdgetmaxtrdqtyspb.S2C{
		Header:     normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
		MaxTrdQtys: normalizeBrokerRouteMaxTrdQtys(s.maxTrdQtys),
	}}
}

func (s *brokerRouteOpenDServer) placeOrderResponse(body []byte) *trdplaceorderpb.Response {
	s.placeOrderCalls++
	request := &trdplaceorderpb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	if request.GetC2S() == nil || request.GetC2S().GetPacketID().GetConnID() == 0 {
		return &trdplaceorderpb.Response{RetType: new(int32(1)), RetMsg: new("missing packet id connID")}
	}
	s.lastPlaceOrder = jftradeCheckedTypeAssertion[*trdplaceorderpb.C2S](proto.Clone(request.GetC2S()))
	orderID, orderIDEx := s.placedOrderID, s.placedOrderIDEx
	if orderID == 0 {
		orderID = 9001
	}
	if orderIDEx == "" {
		orderIDEx = strconv.FormatUint(orderID, 10)
	}
	return &trdplaceorderpb.Response{RetType: new(int32(0)), S2C: &trdplaceorderpb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), OrderID: new(orderID), OrderIDEx: new(orderIDEx),
	}}
}

func (s *brokerRouteOpenDServer) modifyOrderResponse(body []byte) *trdmodifyorderpb.Response {
	s.modifyOrderCalls++
	request := &trdmodifyorderpb.Request{}
	mustUnmarshalBrokerRequest(body, request)
	if request.GetC2S() == nil || request.GetC2S().GetPacketID().GetConnID() == 0 {
		return &trdmodifyorderpb.Response{RetType: new(int32(1)), RetMsg: new("missing packet id connID")}
	}
	s.lastModifyOrder = jftradeCheckedTypeAssertion[*trdmodifyorderpb.C2S](proto.Clone(request.GetC2S()))
	return &trdmodifyorderpb.Response{RetType: new(int32(0)), S2C: &trdmodifyorderpb.S2C{
		Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()), OrderID: new(request.GetC2S().GetOrderID()),
		OrderIDEx: new(strconv.FormatUint(request.GetC2S().GetOrderID(), 10)),
	}}
}

func mustUnmarshalBrokerRequest(body []byte, request proto.Message) {
	jftradePanicOnError(proto.Unmarshal(body, request))
}

func brokerRouteGlobalStateResponse(serverVer, serverBuildNo int32) *globalpb.Response {
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
			Time:           new(int64(0)),
		},
	}
}
