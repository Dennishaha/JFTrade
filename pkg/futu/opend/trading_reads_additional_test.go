package opend

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	getglobalstatepb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	getorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetorderbook"
	updateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetacclistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetacclist"
	trdgetfundspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetfunds"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
	trdgetorderfeepb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfee"
	trdgetorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderfilllist"
	trdgetorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetorderlist"
	trdgetpositionlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetpositionlist"
)

func TestTradingReadWrappersDecodeBusinessPayloads(t *testing.T) {
	header := testTrdHeader(8080)
	security := hkSecurity("00700")
	cashFlowDirection := int32(trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_In)

	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdGetFunds: func(frame codec.Frame) (proto.Message, error) {
			var request trdgetfundspb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetHeader().GetAccID() != header.GetAccID() {
				t.Fatalf("GetFunds header = %#v", request.GetC2S().GetHeader())
			}
			return &trdgetfundspb.Response{
				RetType: new(int32(0)),
				S2C: &trdgetfundspb.S2C{
					Header: header,
					Funds: &trdcommonpb.Funds{
						Power:             new(float64(200000)),
						TotalAssets:       new(float64(500000)),
						Cash:              new(float64(100000)),
						MarketVal:         new(float64(350000)),
						FrozenCash:        new(float64(10000)),
						DebtCash:          new(float64(50000)),
						AvlWithdrawalCash: new(float64(80000)),
					},
				},
			}, nil
		},
		ProtoTrdGetPositionList: func(frame codec.Frame) (proto.Message, error) {
			var request trdgetpositionlistpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetFilterConditions() == nil {
				t.Fatal("GetPositionList filter = nil")
			}
			return &trdgetpositionlistpb.Response{
				RetType: new(int32(0)),
				S2C: &trdgetpositionlistpb.S2C{
					Header: header,
					PositionList: []*trdcommonpb.Position{{
						PositionID:   new(uint64(77)),
						PositionSide: new(int32(trdcommonpb.PositionSide_PositionSide_Long)),
						Code:         new("00700"),
						Name:         new("Tencent"),
						Qty:          new(200.0),
						CanSellQty:   new(200.0),
						Price:        new(321.5),
						Val:          new(64300.0),
						PlVal:        new(1200.0),
					}},
				},
			}, nil
		},
		ProtoTrdGetOrderList: func(frame codec.Frame) (proto.Message, error) {
			var request trdgetorderlistpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetRefreshCache() {
				t.Fatalf("GetOrderList RefreshCache = true, want false")
			}
			return &trdgetorderlistpb.Response{
				RetType: new(int32(0)),
				S2C: &trdgetorderlistpb.S2C{
					Header:    header,
					OrderList: []*trdcommonpb.Order{testOrder(9001, "00700")},
				},
			}, nil
		},
		ProtoTrdGetOrderFee: func(frame codec.Frame) (proto.Message, error) {
			var request trdgetorderfeepb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if ids := request.GetC2S().GetOrderIdExList(); len(ids) != 1 || ids[0] != "EXT-1" {
				t.Fatalf("GetOrderFee order ids = %#v", ids)
			}
			return &trdgetorderfeepb.Response{
				RetType: new(int32(0)),
				S2C: &trdgetorderfeepb.S2C{
					Header: header,
					OrderFeeList: []*trdcommonpb.OrderFee{{
						OrderIDEx: new("EXT-1"),
						FeeAmount: new(12.5),
					}},
				},
			}, nil
		},
		ProtoTrdGetOrderFillList: func(frame codec.Frame) (proto.Message, error) {
			var request trdgetorderfilllistpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetRefreshCache() {
				t.Fatalf("GetOrderFillList RefreshCache = true, want false")
			}
			return &trdgetorderfilllistpb.Response{
				RetType: new(int32(0)),
				S2C: &trdgetorderfilllistpb.S2C{
					Header:        header,
					OrderFillList: []*trdcommonpb.OrderFill{testOrderFill(9101, "00700")},
				},
			}, nil
		},
		ProtoTrdGetMarginRatio: func(frame codec.Frame) (proto.Message, error) {
			var request trdgetmarginratiopb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if got := request.GetC2S().GetSecurityList(); len(got) != 1 || got[0].GetCode() != "00700" {
				t.Fatalf("GetMarginRatio securities = %#v", got)
			}
			return &trdgetmarginratiopb.Response{
				RetType: new(int32(0)),
				S2C: &trdgetmarginratiopb.S2C{
					Header: header,
					MarginRatioInfoList: []*trdgetmarginratiopb.MarginRatioInfo{{
						Security:      security,
						IsLongPermit:  new(true),
						ShortFeeRate:  new(1.25),
						IsShortPermit: new(false),
					}},
				},
			}, nil
		},
		ProtoTrdFlowSummary: func(frame codec.Frame) (proto.Message, error) {
			var request trdflowsummarypb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetClearingDate() != "2026-05-20" || request.GetC2S().GetCashFlowDirection() != cashFlowDirection {
				t.Fatalf("GetFlowSummary request = %#v", request.GetC2S())
			}
			return &trdflowsummarypb.Response{
				RetType: new(int32(0)),
				S2C: &trdflowsummarypb.S2C{
					Header: header,
					FlowSummaryInfoList: []*trdflowsummarypb.FlowSummaryInfo{{
						CashFlowID:     new(uint64(5001)),
						ClearingDate:   new("2026-05-20"),
						CashFlowAmount: new(88.8),
					}},
				},
			}, nil
		},
		ProtoTrdGetMaxTrdQtys: func(frame codec.Frame) (proto.Message, error) {
			var request trdgetmaxtrdqtyspb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetCode() != "00700" || request.GetC2S().GetPrice() != 320.5 {
				t.Fatalf("GetMaxTrdQtys request = %#v", request.GetC2S())
			}
			return &trdgetmaxtrdqtyspb.Response{
				RetType: new(int32(0)),
				S2C: &trdgetmaxtrdqtyspb.S2C{
					Header: header,
					MaxTrdQtys: &trdcommonpb.MaxTrdQtys{
						MaxCashBuy:      new(float64(1000)),
						MaxPositionSell: new(float64(500)),
					},
				},
			}, nil
		},
		ProtoTrdGetAccList: func(frame codec.Frame) (proto.Message, error) {
			var request trdgetacclistpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if !request.GetC2S().GetNeedGeneralSecAccount() || request.GetC2S().GetUserID() != 0 {
				t.Fatalf("GetAccountList request = %#v", request.GetC2S())
			}
			return &trdgetacclistpb.Response{
				RetType: new(int32(0)),
				S2C: &trdgetacclistpb.S2C{
					AccList: []*trdcommonpb.TrdAcc{{
						TrdEnv:  new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
						AccID:   new(uint64(1001)),
						AccType: new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
					}},
				},
			}, nil
		},
	})

	funds, err := client.GetFunds(ctx, header)
	if err != nil || funds.GetPower() != 200000 {
		t.Fatalf("GetFunds() = (%#v, %v)", funds, err)
	}

	positions, err := client.GetPositionList(ctx, header, &trdcommonpb.TrdFilterConditions{})
	if err != nil || len(positions) != 1 || positions[0].GetCode() != "00700" || positions[0].GetQty() != 200 {
		t.Fatalf("GetPositionList() = (%#v, %v)", positions, err)
	}

	orders, err := client.GetOrderList(ctx, header, &trdcommonpb.TrdFilterConditions{})
	if err != nil || len(orders) != 1 || orders[0].GetOrderID() != 9001 {
		t.Fatalf("GetOrderList() = (%#v, %v)", orders, err)
	}

	fees, err := client.GetOrderFee(ctx, header, []string{"EXT-1"})
	if err != nil || len(fees) != 1 || fees[0].GetOrderIDEx() != "EXT-1" || fees[0].GetFeeAmount() != 12.5 {
		t.Fatalf("GetOrderFee() = (%#v, %v)", fees, err)
	}

	fills, err := client.GetOrderFillList(ctx, header, &trdcommonpb.TrdFilterConditions{})
	if err != nil || len(fills) != 1 || fills[0].GetFillID() != 9101 {
		t.Fatalf("GetOrderFillList() = (%#v, %v)", fills, err)
	}

	ratios, err := client.GetMarginRatio(ctx, header, []*qotcommonpb.Security{security})
	if err != nil || len(ratios) != 1 || ratios[0].GetSecurity().GetCode() != "00700" || ratios[0].GetShortFeeRate() != 1.25 {
		t.Fatalf("GetMarginRatio() = (%#v, %v)", ratios, err)
	}

	flows, err := client.GetFlowSummary(ctx, header, "2026-05-20", &cashFlowDirection)
	if err != nil || len(flows) != 1 || flows[0].GetCashFlowID() != 5001 || flows[0].GetCashFlowAmount() != 88.8 {
		t.Fatalf("GetFlowSummary() = (%#v, %v)", flows, err)
	}

	maxQtys, err := client.GetMaxTrdQtys(ctx, &trdgetmaxtrdqtyspb.C2S{
		Header:    header,
		OrderType: new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		Code:      new("00700"),
		Price:     new(320.5),
	})
	if err != nil || maxQtys.GetMaxCashBuy() != 1000 {
		t.Fatalf("GetMaxTrdQtys() = (%#v, %v)", maxQtys, err)
	}

	accounts, err := client.GetAccountList(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].GetAccID() != 1001 {
		t.Fatalf("GetAccountList() = (%#v, %v)", accounts, err)
	}

}

func TestTradingReadWrappersReturnStableEmptyValues(t *testing.T) {
	header := testTrdHeader(1)
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdGetFunds: func(codec.Frame) (proto.Message, error) {
			return &trdgetfundspb.Response{RetType: new(int32(0)), S2C: &trdgetfundspb.S2C{Header: header}}, nil
		},
		ProtoTrdGetPositionList: func(codec.Frame) (proto.Message, error) {
			return &trdgetpositionlistpb.Response{RetType: new(int32(0))}, nil
		},
		ProtoTrdGetOrderList: func(codec.Frame) (proto.Message, error) {
			return &trdgetorderlistpb.Response{RetType: new(int32(0))}, nil
		},
		ProtoTrdGetOrderFee: func(codec.Frame) (proto.Message, error) {
			return &trdgetorderfeepb.Response{RetType: new(int32(0))}, nil
		},
		ProtoTrdGetOrderFillList: func(codec.Frame) (proto.Message, error) {
			return &trdgetorderfilllistpb.Response{RetType: new(int32(0))}, nil
		},
		ProtoTrdGetMarginRatio: func(codec.Frame) (proto.Message, error) {
			return &trdgetmarginratiopb.Response{RetType: new(int32(0))}, nil
		},
		ProtoTrdFlowSummary: func(codec.Frame) (proto.Message, error) {
			return &trdflowsummarypb.Response{RetType: new(int32(0))}, nil
		},
		ProtoTrdGetMaxTrdQtys: func(codec.Frame) (proto.Message, error) {
			return &trdgetmaxtrdqtyspb.Response{RetType: new(int32(0)), S2C: &trdgetmaxtrdqtyspb.S2C{Header: header}}, nil
		},
		ProtoTrdGetAccList: func(codec.Frame) (proto.Message, error) {
			return &trdgetacclistpb.Response{RetType: new(int32(0))}, nil
		},
		ProtoGetOrderBook: func(codec.Frame) (proto.Message, error) {
			return &getorderbookpb.Response{RetType: new(int32(0))}, nil
		},
	})

	if funds, err := client.GetFunds(ctx, header); err != nil || funds == nil || funds.GetPower() != 0 {
		t.Fatalf("empty GetFunds() = (%#v, %v)", funds, err)
	}
	if positions, err := client.GetPositionList(ctx, header, nil); err != nil || len(positions) != 0 {
		t.Fatalf("empty GetPositionList() = (%#v, %v)", positions, err)
	}
	if orders, err := client.GetOrderList(ctx, header, nil); err != nil || len(orders) != 0 {
		t.Fatalf("empty GetOrderList() = (%#v, %v)", orders, err)
	}
	if fees, err := client.GetOrderFee(ctx, header, nil); err != nil || len(fees) != 0 {
		t.Fatalf("empty GetOrderFee() = (%#v, %v)", fees, err)
	}
	if fills, err := client.GetOrderFillList(ctx, header, nil); err != nil || len(fills) != 0 {
		t.Fatalf("empty GetOrderFillList() = (%#v, %v)", fills, err)
	}
	if ratios, err := client.GetMarginRatio(ctx, header, nil); err != nil || len(ratios) != 0 {
		t.Fatalf("empty GetMarginRatio() = (%#v, %v)", ratios, err)
	}
	if flows, err := client.GetFlowSummary(ctx, header, "2026-05-20", nil); err != nil || len(flows) != 0 {
		t.Fatalf("empty GetFlowSummary() = (%#v, %v)", flows, err)
	}
	if _, err := client.GetMaxTrdQtys(ctx, nil); err == nil || err.Error() != "opend Trd_GetMaxTrdQtys request is required" {
		t.Fatalf("GetMaxTrdQtys(nil) error = %v", err)
	}
	if maxQtys, err := client.GetMaxTrdQtys(ctx, &trdgetmaxtrdqtyspb.C2S{
		Header:    header,
		OrderType: new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		Code:      new("00700"),
		Price:     new(320.5),
	}); err != nil || maxQtys == nil || maxQtys.GetMaxCashBuy() != 0 {
		t.Fatalf("empty GetMaxTrdQtys() = (%#v, %v)", maxQtys, err)
	}
	if accounts, err := client.GetAccountList(ctx); err != nil || len(accounts) != 0 {
		t.Fatalf("empty GetAccountList() = (%#v, %v)", accounts, err)
	}
	if orderBook, err := client.GetOrderBook(ctx, OrderBookRequest{Security: hkSecurity("00700"), Num: 5}); err != nil || orderBook == nil || len(orderBook.BidList) != 0 || len(orderBook.AskList) != 0 {
		t.Fatalf("empty GetOrderBook() = (%#v, %v)", orderBook, err)
	}
}

func TestSubscribeOrderBookDispatchesSuccessfulPushes(t *testing.T) {
	client, server, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetGlobalState: func(codec.Frame) (proto.Message, error) {
			return &getglobalstatepb.Response{
				RetType: new(int32(0)),
				S2C: &getglobalstatepb.S2C{
					MarketHK:       new(int32(qotcommonpb.QotMarketState_QotMarketState_Morning)),
					MarketUS:       new(int32(qotcommonpb.QotMarketState_QotMarketState_PreMarketBegin)),
					MarketSH:       new(int32(qotcommonpb.QotMarketState_QotMarketState_Closed)),
					MarketSZ:       new(int32(qotcommonpb.QotMarketState_QotMarketState_Closed)),
					MarketHKFuture: new(int32(qotcommonpb.QotMarketState_QotMarketState_WaitingOpen)),
					QotLogined:     new(true),
					TrdLogined:     new(true),
					ServerVer:      new(int32(900)),
					ServerBuildNo:  new(int32(5008)),
					Time:           new(int64(1717000000)),
				},
			}, nil
		},
	})

	updates := make(chan *updateorderbookpb.S2C, 1)
	client.SubscribeOrderBook(func(snapshot *updateorderbookpb.S2C) {
		updates <- snapshot
	})

	body, err := proto.Marshal(&updateorderbookpb.Response{
		RetType: new(int32(0)),
		S2C: &updateorderbookpb.S2C{
			Security: &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
			OrderBookBidList: []*qotcommonpb.OrderBook{{
				Price:       new(321.5),
				Volume:      new(int64(500)),
				OrederCount: new(int32(1)),
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal order book push: %v", err)
	}
	packet, err := codec.Encode(ProtoQotUpdateOrderBook, 0, body)
	if err != nil {
		t.Fatalf("encode order book push: %v", err)
	}
	server.push(packet)

	if _, err := client.GetGlobalState(ctx); err != nil {
		t.Fatalf("GetGlobalState() error = %v", err)
	}

	select {
	case update := <-updates:
		if update.GetSecurity().GetCode() != "00700" || len(update.GetOrderBookBidList()) != 1 || update.GetOrderBookBidList()[0].GetPrice() != 321.5 {
			t.Fatalf("order book update = %#v", update)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for order book push")
	}
}
