package futu

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

func TestExchangeSecuritySnapshotMergesStaticInfo(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{testTencentStaticInfo()})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	t.Cleanup(func() {
		jftradeCheckTestError(t, ex.Close())
	})

	details, err := ex.QuerySecuritySnapshot(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("QuerySecuritySnapshot: %v", err)
	}
	if details == nil {
		t.Fatal("expected security details")
	}
	if details.InstrumentID != "HK.00700" || details.Market != "HK" || details.Symbol != "00700" {
		t.Fatalf("unexpected instrument identity: %#v", details)
	}
	if details.Name != "Tencent" {
		t.Fatalf("Name = %q, want Tencent", details.Name)
	}
	if details.SecurityType != "Eqty" {
		t.Fatalf("SecurityType = %q, want Eqty", details.SecurityType)
	}
	if details.SecurityID == nil || *details.SecurityID != 700 {
		t.Fatalf("SecurityID = %#v, want 700", details.SecurityID)
	}
	if details.ExchangeType != "HK_MainBoard" {
		t.Fatalf("ExchangeType = %q, want HK_MainBoard", details.ExchangeType)
	}
	if details.Delisting == nil || *details.Delisting {
		t.Fatalf("Delisting = %#v, want false", details.Delisting)
	}
	if got := details.CurrentPrice.InexactFloat64(); got != 380 {
		t.Fatalf("CurrentPrice = %v, want 380", got)
	}
	if details.PreMarket == nil || details.PreMarket.Price.InexactFloat64() != 379.5 {
		t.Fatalf("PreMarket = %#v, want price 379.5", details.PreMarket)
	}
	if details.Equity == nil {
		t.Fatal("expected equity extended details")
	}
	if got := details.Equity.PERate.InexactFloat64(); got != 20.5 {
		t.Fatalf("PERate = %v, want 20.5", got)
	}
	if got := details.Equity.PBRate.InexactFloat64(); got != 4.2 {
		t.Fatalf("PBRate = %v, want 4.2", got)
	}

	aliasDetails, err := ex.QuerySecurityDetails(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("QuerySecurityDetails: %v", err)
	}
	if aliasDetails == nil || aliasDetails.SecurityID == nil || *aliasDetails.SecurityID != 700 {
		t.Fatalf("alias details = %#v, want merged static info", aliasDetails)
	}
	if got := server.staticInfoCalls.Load(); got != 2 {
		t.Fatalf("expected two static-info calls, got %d", got)
	}
	if got := server.securitySnapshotCalls.Load(); got != 2 {
		t.Fatalf("expected two snapshot calls, got %d", got)
	}
}

//nolint:funlen
func TestBrokerAdapterSecurityInfoSnapshotAndOrderBookBridge(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{testTencentStaticInfo()})
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	server.setOrderBookSnapshot(testTencentOrderBookSnapshot())
	defer server.stop()

	reader := newTestBrokerAdapter(t, server).MarketData()
	ctx := t.Context()

	info, err := reader.QuerySecurityInfo(ctx, broker.SecurityInfoQuery{
		ReadQuery: broker.ReadQuery{AccountID: "1001"},
		Symbols:   []string{"HK.00700"},
	})
	if err != nil {
		t.Fatalf("QuerySecurityInfo: %v", err)
	}
	if info == nil || len(info.Securities) != 1 {
		t.Fatalf("expected one security info item, got %#v", info)
	}
	if got := info.Securities[0].Symbol; got != "HK.00700" {
		t.Fatalf("Symbol = %q, want HK.00700", got)
	}
	if info.Securities[0].Name == nil || *info.Securities[0].Name != "Tencent" {
		t.Fatalf("Name = %#v, want Tencent", info.Securities[0].Name)
	}
	if info.Securities[0].SecurityType == nil || *info.Securities[0].SecurityType != "Eqty" {
		t.Fatalf("SecurityType = %#v, want Eqty", info.Securities[0].SecurityType)
	}
	if info.Securities[0].LotSize == nil || *info.Securities[0].LotSize != 100 {
		t.Fatalf("LotSize = %#v, want 100", info.Securities[0].LotSize)
	}
	if info.Securities[0].IsDelisted == nil || *info.Securities[0].IsDelisted {
		t.Fatalf("IsDelisted = %#v, want false", info.Securities[0].IsDelisted)
	}

	snapshot, err := reader.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
		ReadQuery: broker.ReadQuery{AccountID: "1001"},
		Symbols:   []string{"HK.00700"},
	})
	if err != nil {
		t.Fatalf("QuerySecuritySnapshot: %v", err)
	}
	if snapshot == nil || len(snapshot.Snapshots) != 1 {
		t.Fatalf("expected one security snapshot item, got %#v", snapshot)
	}
	if got := snapshot.Snapshots[0].Symbol; got != "HK.00700" {
		t.Fatalf("Snapshot Symbol = %q, want HK.00700", got)
	}
	if snapshot.Snapshots[0].LastPrice == nil || *snapshot.Snapshots[0].LastPrice != 380 {
		t.Fatalf("LastPrice = %#v, want 380", snapshot.Snapshots[0].LastPrice)
	}
	if snapshot.Snapshots[0].PERate == nil || *snapshot.Snapshots[0].PERate != 20.5 {
		t.Fatalf("PERate = %#v, want 20.5", snapshot.Snapshots[0].PERate)
	}
	if snapshot.Snapshots[0].PBRate == nil || *snapshot.Snapshots[0].PBRate != 4.2 {
		t.Fatalf("PBRate = %#v, want 4.2", snapshot.Snapshots[0].PBRate)
	}
	if snapshot.Snapshots[0].Volume == nil || *snapshot.Snapshots[0].Volume != 10000000 {
		t.Fatalf("Volume = %#v, want 10000000", snapshot.Snapshots[0].Volume)
	}

	orderBook, err := reader.QueryOrderBook(ctx, broker.OrderBookQuery{
		ReadQuery: broker.ReadQuery{AccountID: "1001"},
		Symbol:    "HK.00700",
	})
	if err != nil {
		t.Fatalf("QueryOrderBook: %v", err)
	}
	if orderBook == nil {
		t.Fatal("expected order book snapshot")
	}
	if orderBook.Symbol != "HK.00700" {
		t.Fatalf("Symbol = %q, want HK.00700", orderBook.Symbol)
	}
	if orderBook.Name == nil || *orderBook.Name != "Tencent" {
		t.Fatalf("Name = %#v, want Tencent", orderBook.Name)
	}
	if len(orderBook.Bids) != 1 || orderBook.Bids[0].Price != 379.9 || orderBook.Bids[0].Volume != 100 {
		t.Fatalf("Bids = %#v, want one 379.9/100 level", orderBook.Bids)
	}
	if len(orderBook.Asks) != 1 || orderBook.Asks[0].Price != 380.1 || orderBook.Asks[0].Volume != 80 {
		t.Fatalf("Asks = %#v, want one 380.1/80 level", orderBook.Asks)
	}
	if len(orderBook.Bids[0].DetailList) != 1 || orderBook.Bids[0].DetailList[0].OrderID != 90001 {
		t.Fatalf("Bid detail list = %#v, want captured detail order", orderBook.Bids[0].DetailList)
	}
	if got := server.staticInfoCalls.Load(); got != 1 {
		t.Fatalf("expected one static-info call, got %d", got)
	}
	if got := server.securitySnapshotCalls.Load(); got != 1 {
		t.Fatalf("expected one security-snapshot call, got %d", got)
	}
	if got := server.orderBookCalls.Load(); got != 1 {
		t.Fatalf("expected one order-book call, got %d", got)
	}
	if got := server.qotSubCalls.Load(); got != 1 {
		t.Fatalf("expected one non-push Qot_Sub call for order-book subscription, got %d", got)
	}
	if server.lastOrderBook == nil || server.lastOrderBook.GetNum() != 10 {
		t.Fatalf("order-book request = %#v, want default num=10", server.lastOrderBook)
	}
}

func testTencentSecuritySnapshot() *qotgetsecuritysnapshotpb.Snapshot {
	return &qotgetsecuritysnapshotpb.Snapshot{
		Basic: &qotgetsecuritysnapshotpb.SnapshotBasicData{
			Security:        testHKSecurity("00700"),
			Name:            new("Tencent"),
			Type:            new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
			IsSuspend:       new(false),
			ListTime:        new("2004-06-16"),
			LotSize:         new(int32(100)),
			PriceSpread:     new(0.1),
			UpdateTime:      new("2026-05-31 16:00:00"),
			HighPrice:       new(385.0),
			OpenPrice:       new(378.0),
			LowPrice:        new(377.0),
			LastClosePrice:  new(379.0),
			CurPrice:        new(380.0),
			Volume:          new(int64(10000000)),
			Turnover:        new(float64(3800000000)),
			TurnoverRate:    new(0.1),
			ListTimestamp:   new(float64(1087324800)),
			UpdateTimestamp: new(float64(1780243200)),
			AskPrice:        new(380.1),
			BidPrice:        new(379.9),
			AskVol:          new(int64(80)),
			BidVol:          new(int64(100)),
			Amplitude:       new(2.5),
			AvgPrice:        new(379.6),
			BidAskRatio:     new(1.1),
			VolumeRatio:     new(0.8),
			PreMarket: &qotcommonpb.PreAfterMarketData{
				Price:      new(379.5),
				HighPrice:  new(380.0),
				LowPrice:   new(379.0),
				Volume:     new(int64(1000)),
				Turnover:   new(float64(379500)),
				ChangeVal:  new(0.5),
				ChangeRate: new(0.13),
				Amplitude:  new(0.3),
			},
			SecStatus:         new(int32(qotcommonpb.SecurityStatus_SecurityStatus_Normal)),
			ClosePrice5Minute: new(379.8),
			HpVolume:          new(float64(10000000)),
			HpAskVol:          new(float64(80)),
			HpBidVol:          new(float64(100)),
		},
		EquityExData: &qotgetsecuritysnapshotpb.EquitySnapshotExData{
			IssuedShares:         new(int64(9600000000)),
			IssuedMarketVal:      new(float64(3648000000000)),
			NetAsset:             new(float64(869000000000)),
			NetProfit:            new(float64(177000000000)),
			EarningsPershare:     new(18.5),
			OutstandingShares:    new(int64(9500000000)),
			OutstandingMarketVal: new(float64(3610000000000)),
			NetAssetPershare:     new(90.5),
			EyRate:               new(4.9),
			PeRate:               new(20.5),
			PbRate:               new(4.2),
			PeTTMRate:            new(18.3),
		},
	}
}

func testTencentStaticInfo() *qotcommonpb.SecurityStaticInfo {
	return &qotcommonpb.SecurityStaticInfo{
		Basic: &qotcommonpb.SecurityStaticBasic{
			Security:      testHKSecurity("00700"),
			Id:            new(int64(700)),
			Name:          new("Tencent"),
			SecType:       new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
			ExchType:      new(int32(qotcommonpb.ExchType_ExchType_HK_MainBoard)),
			ListTime:      new("2004-06-16"),
			LotSize:       new(int32(100)),
			ListTimestamp: new(float64(1087324800)),
			Delisting:     new(false),
		},
	}
}

func testTencentOrderBookSnapshot() *qotgetorderbookpb.S2C {
	return &qotgetorderbookpb.S2C{
		Security:       testHKSecurity("00700"),
		Name:           new("Tencent"),
		SvrRecvTimeBid: new("2026-06-23 09:30:00.100"),
		SvrRecvTimeAsk: new("2026-06-23 09:30:00.100"),
		OrderBookBidList: []*qotcommonpb.OrderBook{{
			Price:       new(379.9),
			Volume:      new(int64(100)),
			OrederCount: new(int32(1)),
			DetailList: []*qotcommonpb.OrderBookDetail{{
				OrderID: new(int64(90001)),
				Volume:  new(int64(100)),
			}},
		}},
		OrderBookAskList: []*qotcommonpb.OrderBook{{
			Price:       new(380.1),
			Volume:      new(int64(80)),
			OrederCount: new(int32(1)),
			DetailList: []*qotcommonpb.OrderBookDetail{{
				OrderID: new(int64(90002)),
				Volume:  new(int64(80)),
			}},
		}},
	}
}

func testHKSecurity(code string) *qotcommonpb.Security {
	return &qotcommonpb.Security{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
		Code:   new(code),
	}
}
