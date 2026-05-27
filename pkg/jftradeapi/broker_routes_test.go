package jftradeapi

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetacclistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetacclist"
	trdgetfundspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetfunds"
	trdgetorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderlist"
	trdgetpositionlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetpositionlist"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
	trdsubaccpushpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdsubaccpush"
)

func TestBrokerFundsEndpointReturnsDisconnectedSummary(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.saveIntegration(BrokerIntegration{Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          "127.0.0.1",
		APIPort:       1,
		WebSocketPort: 11111,
		TradeMarket:   "HK",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/brokers/futu/funds")
	if err != nil {
		t.Fatalf("GET broker funds: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET broker funds status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode broker funds: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if got := envelope.Data["connectivity"]; got != "disconnected" {
		t.Fatalf("broker funds connectivity = %v", got)
	}
	if _, ok := envelope.Data["currencyBalances"]; !ok {
		t.Fatal("expected currencyBalances in broker funds response")
	}
	if _, ok := envelope.Data["marketAssets"]; !ok {
		t.Fatal("expected marketAssets in broker funds response")
	}
}

func TestBrokerReadEndpointsReturnExchangeBackedData(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	opendServer.setFunds(&trdcommonpb.Funds{
		Power:             proto.Float64(120000),
		TotalAssets:       proto.Float64(100000),
		Cash:              proto.Float64(40000),
		MarketVal:         proto.Float64(60000),
		FrozenCash:        proto.Float64(500),
		DebtCash:          proto.Float64(0),
		AvlWithdrawalCash: proto.Float64(39500),
		Currency:          proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		CashInfoList: []*trdcommonpb.AccCashInfo{{
			Currency:         proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
			Cash:             proto.Float64(40000),
			AvailableBalance: proto.Float64(39500),
			NetCashPower:     proto.Float64(120000),
		}},
		MarketInfoList: []*trdcommonpb.AccMarketInfo{{
			TrdMarket: proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
			Assets:    proto.Float64(100000),
		}},
	})
	opendServer.setPositions([]*trdcommonpb.Position{{
		PositionID:       proto.Uint64(1),
		PositionSide:     proto.Int32(1),
		Code:             proto.String("HK.00700"),
		Name:             proto.String("Tencent"),
		Qty:              proto.Float64(200),
		CanSellQty:       proto.Float64(180),
		Price:            proto.Float64(320.5),
		CostPrice:        proto.Float64(300),
		AverageCostPrice: proto.Float64(301),
		Val:              proto.Float64(64100),
		PlVal:            proto.Float64(3900),
		PlRatio:          proto.Float64(13),
		TrdMarket:        proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		Currency:         proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
	}})
	opendServer.setOrders([]*trdcommonpb.Order{{
		TrdSide:      proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:    proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus:  proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		OrderID:      proto.Uint64(2001),
		OrderIDEx:    proto.String("EXT-2001"),
		Code:         proto.String("HK.00700"),
		Name:         proto.String("Tencent"),
		Qty:          proto.Float64(100),
		Price:        proto.Float64(319.8),
		CreateTime:   proto.String("2026-05-20 09:30:00"),
		UpdateTime:   proto.String("2026-05-20 09:31:00"),
		FillQty:      proto.Float64(20),
		FillAvgPrice: proto.Float64(319.5),
		TimeInForce:  proto.Int32(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
		Currency:     proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:    proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	defer opendServer.stop()

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.saveIntegration(BrokerIntegration{Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          strings.Split(opendServer.addr, ":")[0],
		APIPort:       portFromAddr(t, opendServer.addr),
		WebSocketPort: 11111,
		TradeMarket:   "HK",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}

	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	query := "?tradingEnvironment=SIMULATE&accountId=1001&market=HK"

	funds := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/funds"+query)
	if got := funds["connectivity"]; got != "connected" {
		t.Fatalf("funds connectivity = %v, want connected", got)
	}
	summary, ok := funds["summary"].(map[string]any)
	if !ok {
		t.Fatalf("funds summary = %#v", funds["summary"])
	}
	if got := summary["accountId"]; got != "1001" {
		t.Fatalf("funds summary accountId = %v, want 1001", got)
	}
	if got := summary["currency"]; got != "HKD" {
		t.Fatalf("funds summary currency = %v, want HKD", got)
	}

	positions := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/positions"+query)
	entries, ok := positions["positions"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("positions entries = %#v", positions["positions"])
	}
	position, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("position entry = %#v", entries[0])
	}
	if got := position["symbol"]; got != "HK.00700" {
		t.Fatalf("position symbol = %v, want HK.00700", got)
	}

	orders := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/orders"+query)
	orderEntries, ok := orders["orders"].([]any)
	if !ok || len(orderEntries) != 1 {
		t.Fatalf("orders entries = %#v", orders["orders"])
	}
	order, ok := orderEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("order entry = %#v", orderEntries[0])
	}
	if got := order["brokerOrderId"]; got != "2001" {
		t.Fatalf("brokerOrderId = %v, want 2001", got)
	}
	if got := order["status"]; got != "SUBMITTED" {
		t.Fatalf("order status = %v, want SUBMITTED", got)
	}
}

func decodeBrokerEnvelope(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d", url, resp.StatusCode)
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true for %s", url)
	}
	return envelope.Data
}

func portFromAddr(t *testing.T, addr string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", port, err)
	}
	return value
}

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
	placedOrderID     uint64
	placedOrderIDEx   string
	lastPlaceOrder    *trdplaceorderpb.C2S
	lastModifyOrder   *trdmodifyorderpb.C2S
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
		orders := make([]*trdcommonpb.Order, 0, len(s.orders))
		for _, order := range s.orders {
			orders = append(orders, normalizeBrokerRouteOrder(order))
		}
		if codes := request.GetC2S().GetFilterConditions().GetCodeList(); len(codes) > 0 {
			filtered := make([]*trdcommonpb.Order, 0, len(orders))
			for _, order := range orders {
				for _, code := range codes {
					if strings.EqualFold(order.GetCode(), code) {
						filtered = append(filtered, order)
						break
					}
				}
			}
			orders = filtered
		}
		return &trdgetorderlistpb.Response{
			RetType: proto.Int32(0),
			S2C: &trdgetorderlistpb.S2C{
				Header:    normalizeBrokerRouteHeader(request.GetC2S().GetHeader()),
				OrderList: orders,
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

func normalizeBrokerRouteHeader(header *trdcommonpb.TrdHeader) *trdcommonpb.TrdHeader {
	if header == nil {
		header = &trdcommonpb.TrdHeader{}
	}
	clone := proto.Clone(header).(*trdcommonpb.TrdHeader)
	if clone.TrdEnv == nil {
		clone.TrdEnv = proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate))
	}
	if clone.AccID == nil {
		clone.AccID = proto.Uint64(1001)
	}
	if clone.TrdMarket == nil {
		clone.TrdMarket = proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK))
	}
	return clone
}

func normalizeBrokerRouteFunds(funds *trdcommonpb.Funds) *trdcommonpb.Funds {
	if funds == nil {
		funds = &trdcommonpb.Funds{}
	}
	clone := proto.Clone(funds).(*trdcommonpb.Funds)
	if clone.Power == nil {
		clone.Power = proto.Float64(0)
	}
	if clone.TotalAssets == nil {
		clone.TotalAssets = proto.Float64(0)
	}
	if clone.Cash == nil {
		clone.Cash = proto.Float64(0)
	}
	if clone.MarketVal == nil {
		clone.MarketVal = proto.Float64(0)
	}
	if clone.FrozenCash == nil {
		clone.FrozenCash = proto.Float64(0)
	}
	if clone.DebtCash == nil {
		clone.DebtCash = proto.Float64(0)
	}
	if clone.AvlWithdrawalCash == nil {
		clone.AvlWithdrawalCash = proto.Float64(0)
	}
	return clone
}

func normalizeBrokerRoutePosition(position *trdcommonpb.Position) *trdcommonpb.Position {
	if position == nil {
		position = &trdcommonpb.Position{}
	}
	clone := proto.Clone(position).(*trdcommonpb.Position)
	if clone.PositionID == nil {
		clone.PositionID = proto.Uint64(1)
	}
	if clone.PositionSide == nil {
		clone.PositionSide = proto.Int32(1)
	}
	if clone.Code == nil {
		clone.Code = proto.String("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = proto.String(clone.GetCode())
	}
	if clone.Qty == nil {
		clone.Qty = proto.Float64(0)
	}
	if clone.CanSellQty == nil {
		clone.CanSellQty = proto.Float64(0)
	}
	if clone.Price == nil {
		clone.Price = proto.Float64(0)
	}
	if clone.Val == nil {
		clone.Val = proto.Float64(0)
	}
	if clone.PlVal == nil {
		clone.PlVal = proto.Float64(0)
	}
	return clone
}

func normalizeBrokerRouteOrder(order *trdcommonpb.Order) *trdcommonpb.Order {
	if order == nil {
		order = &trdcommonpb.Order{}
	}
	clone := proto.Clone(order).(*trdcommonpb.Order)
	if clone.TrdSide == nil {
		clone.TrdSide = proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy))
	}
	if clone.OrderType == nil {
		clone.OrderType = proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal))
	}
	if clone.OrderStatus == nil {
		clone.OrderStatus = proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted))
	}
	if clone.OrderID == nil {
		clone.OrderID = proto.Uint64(1)
	}
	if clone.OrderIDEx == nil {
		clone.OrderIDEx = proto.String(strconv.FormatUint(clone.GetOrderID(), 10))
	}
	if clone.Code == nil {
		clone.Code = proto.String("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = proto.String(clone.GetCode())
	}
	if clone.Qty == nil {
		clone.Qty = proto.Float64(0)
	}
	if clone.CreateTime == nil {
		clone.CreateTime = proto.String("2026-05-20 09:30:00")
	}
	if clone.UpdateTime == nil {
		clone.UpdateTime = proto.String(clone.GetCreateTime())
	}
	return clone
}
