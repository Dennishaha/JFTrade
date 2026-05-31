package jftradeapi

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
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
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
	placeOrderCalls   int
	modifyOrderCalls  int
	subAccPushCalls   int
}

func startBrokerRouteOpenDServer(t *testing.T) *brokerRouteOpenDServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &brokerRouteOpenDServer{
		addr:              listener.Addr().String(),
		listener:          listener,
		shutdownCompleted: make(chan struct{}),
	}
	go server.acceptLoop()
	return server
}

func (s *brokerRouteOpenDServer) stop() {
	s.stopOnce.Do(func() {
		_ = s.listener.Close()
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
	return proto.Clone(s.lastPlaceOrder).(*trdplaceorderpb.C2S)
}

func (s *brokerRouteOpenDServer) lastModifyOrderRequest() *trdmodifyorderpb.C2S {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastModifyOrder == nil {
		return nil
	}
	return proto.Clone(s.lastModifyOrder).(*trdmodifyorderpb.C2S)
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
	defer conn.Close()
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

	switch frame.Header.ProtoID {
	case opend.ProtoInitConnect:
		return &initpb.Response{
			RetType: proto.Int32(0),
			S2C: &initpb.S2C{
				ServerVer:         proto.Int32(700),
				LoginUserID:       proto.Uint64(1),
				ConnID:            proto.Uint64(42),
				ConnAESKey:        proto.String("0123456789abcdef"),
				KeepAliveInterval: proto.Int32(10),
			},
		}
	case opend.ProtoTrdGetAccList:
		return &trdgetacclistpb.Response{
			RetType: proto.Int32(0),
			S2C:     &trdgetacclistpb.S2C{AccList: append([]*trdcommonpb.TrdAcc(nil), s.accounts...)},
		}
	case opend.ProtoTrdGetFunds:
		request := &trdgetfundspb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		return &trdgetfundspb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgetfundspb.S2C{
				Header: normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				Funds:  normalizeBrokerRouteFunds(s.funds),
			},
		}
	case opend.ProtoTrdGetPositionList:
		request := &trdgetpositionlistpb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		positions := make([]*trdcommonpb.Position, 0, len(s.positions))
		for _, position := range s.positions {
			positions = append(positions, normalizeBrokerRoutePosition(position))
		}
		return &trdgetpositionlistpb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgetpositionlistpb.S2C{
				Header:       normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				PositionList: positions,
			},
		}
	case opend.ProtoTrdGetOrderList:
		request := &trdgetorderlistpb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		orders := filterBrokerRouteOrders(s.orders, request.GetC2S().GetFilterConditions(), nil)
		return &trdgetorderlistpb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgetorderlistpb.S2C{
				Header:    normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				OrderList: orders,
			},
		}
	case opend.ProtoTrdGetHistoryOrderList:
		request := &trdgethistoryorderlistpb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		orders := filterBrokerRouteOrders(s.historyOrders, request.GetC2S().GetFilterConditions(), request.GetC2S().GetFilterStatusList())
		return &trdgethistoryorderlistpb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgethistoryorderlistpb.S2C{
				Header:    normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				OrderList: orders,
			},
		}
	case opend.ProtoTrdGetOrderFillList:
		request := &trdgetorderfilllistpb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		fills := filterBrokerRouteFills(s.orderFills, request.GetC2S().GetFilterConditions())
		return &trdgetorderfilllistpb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgetorderfilllistpb.S2C{
				Header:        normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				OrderFillList: fills,
			},
		}
	case opend.ProtoTrdGetHistoryOrderFillList:
		request := &trdgethistoryorderfilllistpb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		fills := filterBrokerRouteFills(s.historyFills, request.GetC2S().GetFilterConditions())
		return &trdgethistoryorderfilllistpb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgethistoryorderfilllistpb.S2C{
				Header:        normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				OrderFillList: fills,
			},
		}
	case opend.ProtoTrdGetOrderFee:
		request := &trdgetorderfeepb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		fees := make([]*trdcommonpb.OrderFee, 0, len(s.orderFees))
		requested := make(map[string]struct{}, len(request.GetC2S().GetOrderIdExList()))
		for _, orderIDEx := range request.GetC2S().GetOrderIdExList() {
			requested[strings.ToUpper(strings.TrimSpace(orderIDEx))] = struct{}{}
		}
		for _, fee := range s.orderFees {
			if fee == nil {
				continue
			}
			if len(requested) > 0 {
				if _, ok := requested[strings.ToUpper(strings.TrimSpace(fee.GetOrderIDEx()))]; !ok {
					continue
				}
			}
			fees = append(fees, normalizeBrokerRouteOrderFee(fee))
		}
		return &trdgetorderfeepb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgetorderfeepb.S2C{
				Header:       normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				OrderFeeList: fees,
			},
		}
	case opend.ProtoTrdGetMarginRatio:
		request := &trdgetmarginratiopb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		ratios := make([]*trdgetmarginratiopb.MarginRatioInfo, 0, len(s.marginRatios))
		requested := make(map[string]struct{}, len(request.GetC2S().GetSecurityList()))
		for _, security := range request.GetC2S().GetSecurityList() {
			requested[strings.ToUpper(strings.TrimSpace(security.GetCode()))] = struct{}{}
		}
		for _, ratio := range s.marginRatios {
			if ratio == nil {
				continue
			}
			if len(requested) > 0 {
				if _, ok := requested[strings.ToUpper(strings.TrimSpace(ratio.GetSecurity().GetCode()))]; !ok {
					continue
				}
			}
			ratios = append(ratios, normalizeBrokerRouteMarginRatio(ratio))
		}
		return &trdgetmarginratiopb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgetmarginratiopb.S2C{
				Header:              normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				MarginRatioInfoList: ratios,
			},
		}
	case opend.ProtoTrdFlowSummary:
		request := &trdflowsummarypb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		flows := make([]*trdflowsummarypb.FlowSummaryInfo, 0, len(s.cashFlows))
		for _, flow := range s.cashFlows {
			if flow == nil {
				continue
			}
			if direction := request.GetC2S().GetCashFlowDirection(); direction != 0 && flow.GetCashFlowDirection() != direction {
				continue
			}
			flows = append(flows, normalizeBrokerRouteCashFlow(flow))
		}
		return &trdflowsummarypb.Response{
			RetType: proto.Int32(0),
			S2C: &trdflowsummarypb.S2C{
				Header:              normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				FlowSummaryInfoList: flows,
			},
		}
	case opend.ProtoTrdGetMaxTrdQtys:
		request := &trdgetmaxtrdqtyspb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		if request.GetC2S() != nil {
			s.lastMaxTrdQtys = proto.Clone(request.GetC2S()).(*trdgetmaxtrdqtyspb.C2S)
		}
		return &trdgetmaxtrdqtyspb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgetmaxtrdqtyspb.S2C{
				Header:     normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				MaxTrdQtys: normalizeBrokerRouteMaxTrdQtys(s.maxTrdQtys),
			},
		}
	case opend.ProtoTrdPlaceOrder:
		s.placeOrderCalls++
		request := &trdplaceorderpb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		if request.GetC2S() == nil || request.GetC2S().GetPacketID().GetConnID() == 0 {
			return &trdplaceorderpb.Response{RetType: proto.Int32(1), RetMsg: proto.String("missing packet id connID")}
		}
		s.lastPlaceOrder = proto.Clone(request.GetC2S()).(*trdplaceorderpb.C2S)
		orderID := s.placedOrderID
		orderIDEx := s.placedOrderIDEx
		if orderID == 0 {
			orderID = 9001
		}
		if orderIDEx == "" {
			orderIDEx = strconv.FormatUint(orderID, 10)
		}
		return &trdplaceorderpb.Response{
			RetType: proto.Int32(0),
			S2C: &trdplaceorderpb.S2C{
				Header:    normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				OrderID:   proto.Uint64(orderID),
				OrderIDEx: proto.String(orderIDEx),
			},
		}
	case opend.ProtoTrdModifyOrder:
		s.modifyOrderCalls++
		request := &trdmodifyorderpb.Request{}
		_ = proto.Unmarshal(frame.Body, request)
		if request.GetC2S() == nil || request.GetC2S().GetPacketID().GetConnID() == 0 {
			return &trdmodifyorderpb.Response{RetType: proto.Int32(1), RetMsg: proto.String("missing packet id connID")}
		}
		s.lastModifyOrder = proto.Clone(request.GetC2S()).(*trdmodifyorderpb.C2S)
		return &trdmodifyorderpb.Response{
			RetType: proto.Int32(0),
			S2C: &trdmodifyorderpb.S2C{
				Header:    normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				OrderID:   proto.Uint64(request.GetC2S().GetOrderID()),
				OrderIDEx: proto.String(strconv.FormatUint(request.GetC2S().GetOrderID(), 10)),
			},
		}
	case opend.ProtoTrdSubAccPush:
		s.subAccPushCalls++
		return &trdsubaccpushpb.Response{RetType: proto.Int32(0)}
	default:
		return &initpb.Response{RetType: proto.Int32(1), RetMsg: proto.String("unsupported proto")}
	}
}
