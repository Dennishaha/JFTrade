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
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
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
	}, {
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             proto.Uint64(2001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
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
	opendServer.setHistoryOrders([]*trdcommonpb.Order{{
		TrdSide:      proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:    proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus:  proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_All)),
		OrderID:      proto.Uint64(2101),
		OrderIDEx:    proto.String("EXT-2101"),
		Code:         proto.String("HK.00700"),
		Name:         proto.String("Tencent"),
		Qty:          proto.Float64(50),
		Price:        proto.Float64(321.2),
		CreateTime:   proto.String("2026-05-19 09:30:00"),
		UpdateTime:   proto.String("2026-05-19 09:45:00"),
		FillQty:      proto.Float64(50),
		FillAvgPrice: proto.Float64(321.1),
		TimeInForce:  proto.Int32(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
		Currency:     proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:    proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setOrderFills([]*trdcommonpb.OrderFill{{
		OrderID:    proto.Uint64(2001),
		OrderIDEx:  proto.String("EXT-2001"),
		FillID:     proto.Uint64(3001),
		FillIDEx:   proto.String("FILL-3001"),
		Code:       proto.String("HK.00700"),
		Name:       proto.String("Tencent"),
		TrdSide:    proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        proto.Float64(20),
		Price:      proto.Float64(319.5),
		CreateTime: proto.String("2026-05-20 09:31:30"),
		Status:     proto.Int32(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
		TrdMarket:  proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setHistoryFills([]*trdcommonpb.OrderFill{{
		OrderID:    proto.Uint64(2101),
		OrderIDEx:  proto.String("EXT-2101"),
		FillID:     proto.Uint64(3101),
		FillIDEx:   proto.String("FILL-3101"),
		Code:       proto.String("HK.00700"),
		Name:       proto.String("Tencent"),
		TrdSide:    proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        proto.Float64(50),
		Price:      proto.Float64(321.1),
		CreateTime: proto.String("2026-05-19 09:40:00"),
		Status:     proto.Int32(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
		TrdMarket:  proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setOrderFees([]*trdcommonpb.OrderFee{{
		OrderIDEx: proto.String("EXT-2001"),
		FeeAmount: proto.Float64(12.5),
		FeeList:   []*trdcommonpb.OrderFeeItem{{Title: proto.String("Commission"), Value: proto.Float64(10.0)}},
	}})
	opendServer.setCashFlows([]*trdflowsummarypb.FlowSummaryInfo{{
		CashFlowID:        proto.Uint64(5001),
		ClearingDate:      proto.String("2026-05-20"),
		SettlementDate:    proto.String("2026-05-21"),
		Currency:          proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		CashFlowType:      proto.String("DIVIDEND"),
		CashFlowDirection: proto.Int32(int32(trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_In)),
		CashFlowAmount:    proto.Float64(88.8),
		CashFlowRemark:    proto.String("cash-flow-test"),
	}})
	opendServer.setMarginRatios([]*trdgetmarginratiopb.MarginRatioInfo{{
		Security:       &qotcommonpb.Security{Market: proto.Int32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: proto.String("00700")},
		IsLongPermit:   proto.Bool(true),
		IsShortPermit:  proto.Bool(false),
		ShortFeeRate:   proto.Float64(1.25),
		AlertLongRatio: proto.Float64(0.3),
	}})
	opendServer.setMaxTrdQtys(&trdcommonpb.MaxTrdQtys{
		MaxCashBuy:          proto.Float64(1000),
		MaxCashAndMarginBuy: proto.Float64(2000),
		MaxPositionSell:     proto.Float64(500),
		MaxSellShort:        proto.Float64(300),
		MaxBuyBack:          proto.Float64(150),
		LongRequiredIM:      proto.Float64(10),
		ShortRequiredIM:     proto.Float64(12),
		Session:             proto.Int32(int32(commonpb.Session_Session_RTH)),
	})
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
	realQuery := "?tradingEnvironment=REAL&accountId=2001&market=HK"

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

	historyOrders := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/orders"+query+"&scope=history")
	historyOrderEntries, ok := historyOrders["orders"].([]any)
	if !ok || len(historyOrderEntries) != 1 {
		t.Fatalf("history orders entries = %#v", historyOrders["orders"])
	}

	fills := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/fills"+query)
	fillEntries, ok := fills["fills"].([]any)
	if !ok || len(fillEntries) != 1 {
		t.Fatalf("fills entries = %#v", fills["fills"])
	}
	fill, ok := fillEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("fill entry = %#v", fillEntries[0])
	}
	if got := fill["brokerFillId"]; got != "3001" {
		t.Fatalf("brokerFillId = %v, want 3001", got)
	}

	historyFills := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/fills"+query+"&scope=history")
	historyFillEntries, ok := historyFills["fills"].([]any)
	if !ok || len(historyFillEntries) != 1 {
		t.Fatalf("history fills entries = %#v", historyFills["fills"])
	}

	fees := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/order-fees"+query+"&orderIdEx=EXT-2001")
	feeEntries, ok := fees["fees"].([]any)
	if !ok || len(feeEntries) != 1 {
		t.Fatalf("fees entries = %#v", fees["fees"])
	}
	fee, ok := feeEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("fee entry = %#v", feeEntries[0])
	}
	if got := fee["brokerOrderIdEx"]; got != "EXT-2001" {
		t.Fatalf("fee brokerOrderIdEx = %v, want EXT-2001", got)
	}

	cashFlows := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/cash-flows"+query+"&clearingDate=2026-05-20&direction=IN")
	flowEntries, ok := cashFlows["cashFlows"].([]any)
	if !ok || len(flowEntries) != 1 {
		t.Fatalf("cashFlows entries = %#v", cashFlows["cashFlows"])
	}
	flow, ok := flowEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("cashFlow entry = %#v", flowEntries[0])
	}
	if got := flow["cashFlowType"]; got != "DIVIDEND" {
		t.Fatalf("cashFlowType = %v, want DIVIDEND", got)
	}

	marginRatios := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/margin-ratios"+realQuery+"&symbol=HK.00700")
	ratioEntries, ok := marginRatios["marginRatios"].([]any)
	if !ok || len(ratioEntries) != 1 {
		t.Fatalf("marginRatios entries = %#v", marginRatios["marginRatios"])
	}
	ratio, ok := ratioEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("margin ratio entry = %#v", ratioEntries[0])
	}
	if got := ratio["symbol"]; got != "HK.00700" {
		t.Fatalf("margin ratio symbol = %v, want HK.00700", got)
	}

	maxTradeQtys := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/max-trade-qtys"+query+"&symbol=HK.00700&orderType=LIMIT&price=320.5")
	maxTradeQuantity, ok := maxTradeQtys["maxTradeQuantity"].(map[string]any)
	if !ok {
		t.Fatalf("maxTradeQuantity = %#v", maxTradeQtys["maxTradeQuantity"])
	}
	if got := maxTradeQuantity["maxCashBuy"]; got != 1000.0 {
		t.Fatalf("maxCashBuy = %v, want 1000", got)
	}
	if got := maxTradeQuantity["orderType"]; got != "LIMIT" {
		t.Fatalf("orderType = %v, want LIMIT", got)
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

func normalizeBrokerRouteOrderFill(fill *trdcommonpb.OrderFill) *trdcommonpb.OrderFill {
	if fill == nil {
		fill = &trdcommonpb.OrderFill{}
	}
	clone := proto.Clone(fill).(*trdcommonpb.OrderFill)
	if clone.OrderID == nil {
		clone.OrderID = proto.Uint64(1)
	}
	if clone.OrderIDEx == nil {
		clone.OrderIDEx = proto.String(strconv.FormatUint(clone.GetOrderID(), 10))
	}
	if clone.FillID == nil {
		clone.FillID = proto.Uint64(1)
	}
	if clone.FillIDEx == nil {
		clone.FillIDEx = proto.String(strconv.FormatUint(clone.GetFillID(), 10))
	}
	if clone.Code == nil {
		clone.Code = proto.String("HK.00700")
	}
	if clone.Name == nil {
		clone.Name = proto.String(clone.GetCode())
	}
	if clone.TrdSide == nil {
		clone.TrdSide = proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy))
	}
	if clone.Qty == nil {
		clone.Qty = proto.Float64(0)
	}
	if clone.CreateTime == nil {
		clone.CreateTime = proto.String("2026-05-20 09:30:00")
	}
	return clone
}

func normalizeBrokerRouteOrderFee(fee *trdcommonpb.OrderFee) *trdcommonpb.OrderFee {
	if fee == nil {
		fee = &trdcommonpb.OrderFee{}
	}
	clone := proto.Clone(fee).(*trdcommonpb.OrderFee)
	if clone.OrderIDEx == nil {
		clone.OrderIDEx = proto.String("")
	}
	if clone.FeeAmount == nil {
		clone.FeeAmount = proto.Float64(0)
	}
	return clone
}

func normalizeBrokerRouteMarginRatio(ratio *trdgetmarginratiopb.MarginRatioInfo) *trdgetmarginratiopb.MarginRatioInfo {
	if ratio == nil {
		ratio = &trdgetmarginratiopb.MarginRatioInfo{}
	}
	clone := proto.Clone(ratio).(*trdgetmarginratiopb.MarginRatioInfo)
	if clone.Security == nil {
		clone.Security = &qotcommonpb.Security{Market: proto.Int32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: proto.String("00700")}
	}
	return clone
}

func normalizeBrokerRouteCashFlow(flow *trdflowsummarypb.FlowSummaryInfo) *trdflowsummarypb.FlowSummaryInfo {
	if flow == nil {
		flow = &trdflowsummarypb.FlowSummaryInfo{}
	}
	clone := proto.Clone(flow).(*trdflowsummarypb.FlowSummaryInfo)
	if clone.CashFlowID == nil {
		clone.CashFlowID = proto.Uint64(1)
	}
	if clone.ClearingDate == nil {
		clone.ClearingDate = proto.String("2026-05-20")
	}
	return clone
}

func normalizeBrokerRouteMaxTrdQtys(maxQtys *trdcommonpb.MaxTrdQtys) *trdcommonpb.MaxTrdQtys {
	if maxQtys == nil {
		maxQtys = &trdcommonpb.MaxTrdQtys{}
	}
	return proto.Clone(maxQtys).(*trdcommonpb.MaxTrdQtys)
}

func filterBrokerRouteOrders(input []*trdcommonpb.Order, filter *trdcommonpb.TrdFilterConditions, statuses []int32) []*trdcommonpb.Order {
	orders := make([]*trdcommonpb.Order, 0, len(input))
	codeSet := make(map[string]struct{}, len(filter.GetCodeList()))
	for _, code := range filter.GetCodeList() {
		codeSet[strings.ToUpper(strings.TrimSpace(code))] = struct{}{}
	}
	statusSet := make(map[int32]struct{}, len(statuses))
	for _, status := range statuses {
		statusSet[status] = struct{}{}
	}
	for _, order := range input {
		normalized := normalizeBrokerRouteOrder(order)
		if len(codeSet) > 0 {
			if _, ok := codeSet[strings.ToUpper(strings.TrimSpace(normalized.GetCode()))]; !ok {
				continue
			}
		}
		if len(statusSet) > 0 {
			if _, ok := statusSet[normalized.GetOrderStatus()]; !ok {
				continue
			}
		}
		orders = append(orders, normalized)
	}
	return orders
}

func filterBrokerRouteFills(input []*trdcommonpb.OrderFill, filter *trdcommonpb.TrdFilterConditions) []*trdcommonpb.OrderFill {
	fills := make([]*trdcommonpb.OrderFill, 0, len(input))
	codeSet := make(map[string]struct{}, len(filter.GetCodeList()))
	for _, code := range filter.GetCodeList() {
		codeSet[strings.ToUpper(strings.TrimSpace(code))] = struct{}{}
	}
	for _, fill := range input {
		normalized := normalizeBrokerRouteOrderFill(fill)
		if len(codeSet) > 0 {
			if _, ok := codeSet[strings.ToUpper(strings.TrimSpace(normalized.GetCode()))]; !ok {
				continue
			}
		}
		fills = append(fills, normalized)
	}
	return fills
}
