package opend

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	getglobalstatepb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgethistoryorderfilllistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderfilllist"
	trdgethistoryorderlistpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgethistoryorderlist"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
	trdsubaccpushpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdsubaccpush"
	trdupdateorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdupdateorder"
	trdupdateorderfillpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdupdateorderfill"
)

func testTrdHeader(accountID uint64) *trdcommonpb.TrdHeader {
	return &trdcommonpb.TrdHeader{
		TrdEnv:    new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:     new(accountID),
		TrdMarket: new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}
}

func testOrder(orderID uint64, code string) *trdcommonpb.Order {
	return &trdcommonpb.Order{
		TrdSide:     new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:   new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		OrderID:     new(orderID),
		OrderIDEx:   new("order-ex"),
		Code:        new(code),
		Name:        new("Tencent"),
		Qty:         new(100.0),
		Price:       new(321.5),
		CreateTime:  new("2026-06-22 09:30:00"),
		UpdateTime:  new("2026-06-22 09:30:01"),
		TrdMarket:   new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}
}

func testOrderFill(fillID uint64, code string) *trdcommonpb.OrderFill {
	return &trdcommonpb.OrderFill{
		TrdSide:    new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		FillID:     new(fillID),
		FillIDEx:   new("fill-ex"),
		OrderID:    new(uint64(991)),
		OrderIDEx:  new("order-ex"),
		Code:       new(code),
		Name:       new("Tencent"),
		Qty:        new(100.0),
		Price:      new(321.5),
		CreateTime: new("2026-06-22 09:30:02"),
		TrdMarket:  new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}
}

func TestPlaceOrderRequiresRequestAndConnID(t *testing.T) {
	client := New(Config{})

	if _, err := client.PlaceOrder(context.Background(), nil); err == nil {
		t.Fatal("PlaceOrder(nil) error = nil")
	}

	_, err := client.PlaceOrder(context.Background(), &trdplaceorderpb.C2S{
		Header:    testTrdHeader(1),
		TrdSide:   new(int32(1)),
		OrderType: new(int32(2)),
		Code:      new("00700"),
		Qty:       new(100.0),
	})
	if err == nil || err.Error() != "opend PlaceOrder requires InitConnect connID before trade writes" {
		t.Fatalf("PlaceOrder(no connID) error = %v", err)
	}
}

func TestPlaceOrderAndModifyOrderEncodeTradeWrites(t *testing.T) {
	header := testTrdHeader(1001)

	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdPlaceOrder: func(frame codec.Frame) (proto.Message, error) {
			var request trdplaceorderpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			c2s := request.GetC2S()
			if c2s == nil || c2s.GetPacketID() == nil || c2s.GetPacketID().GetConnID() != 42 {
				t.Fatalf("place order packetID = %#v", c2s.GetPacketID())
			}
			if c2s.GetCode() != "00700" || c2s.GetQty() != 100 || c2s.GetPrice() != 321.5 {
				t.Fatalf("place order request = %#v", c2s)
			}
			return &trdplaceorderpb.Response{
				RetType: new(int32(0)),
				S2C: &trdplaceorderpb.S2C{
					Header:    header,
					OrderID:   new(uint64(9001)),
					OrderIDEx: new("server-order-1"),
				},
			}, nil
		},
		ProtoTrdModifyOrder: func(frame codec.Frame) (proto.Message, error) {
			var request trdmodifyorderpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			c2s := request.GetC2S()
			if c2s == nil || c2s.GetPacketID() == nil || c2s.GetPacketID().GetConnID() != 42 {
				t.Fatalf("modify order packetID = %#v", c2s.GetPacketID())
			}
			if c2s.GetOrderID() != 9001 || c2s.GetPrice() != 322 || c2s.GetQty() != 50 {
				t.Fatalf("modify order request = %#v", c2s)
			}
			return &trdmodifyorderpb.Response{
				RetType: new(int32(0)),
				S2C: &trdmodifyorderpb.S2C{
					Header:    header,
					OrderID:   new(uint64(9001)),
					OrderIDEx: new("server-order-1"),
				},
			}, nil
		},
	})

	placed, err := c.PlaceOrder(ctx, &trdplaceorderpb.C2S{
		Header:    header,
		TrdSide:   new(int32(1)),
		OrderType: new(int32(2)),
		Code:      new("00700"),
		Qty:       new(100.0),
		Price:     new(321.5),
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if placed.OrderID != 9001 || placed.OrderIDEx != "server-order-1" {
		t.Fatalf("PlaceOrder() = %#v", placed)
	}

	modified, err := c.ModifyOrder(ctx, &trdmodifyorderpb.C2S{
		Header:        header,
		OrderID:       new(uint64(9001)),
		ModifyOrderOp: new(int32(1)),
		Qty:           new(50.0),
		Price:         new(322.0),
	})
	if err != nil {
		t.Fatalf("ModifyOrder() error = %v", err)
	}
	if modified.OrderID != 9001 || modified.OrderIDEx != "server-order-1" {
		t.Fatalf("ModifyOrder() = %#v", modified)
	}
}

func TestModifyOrderReturnsTradeRetTypeErrors(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdModifyOrder: func(frame codec.Frame) (proto.Message, error) {
			return &trdmodifyorderpb.Response{
				RetType: new(int32(-1)),
				ErrCode: new(int32(77)),
				RetMsg:  new("modify failed"),
			}, nil
		},
	})

	_, err := c.ModifyOrder(ctx, &trdmodifyorderpb.C2S{
		Header:        testTrdHeader(1),
		OrderID:       new(uint64(99)),
		ModifyOrderOp: new(int32(1)),
	})
	if err == nil || err.Error() != "opend Trd_ModifyOrder retType=-1 errCode=77 retMsg=modify failed" {
		t.Fatalf("ModifyOrder() error = %v", err)
	}
}

func TestHistoryOrderReadersPreserveFiltersAndEmptyResponses(t *testing.T) {
	header := testTrdHeader(8080)
	filterSeen := make(chan *trdcommonpb.TrdFilterConditions, 1)
	statusesSeen := make(chan []int32, 1)

	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdGetHistoryOrderList: func(frame codec.Frame) (proto.Message, error) {
			var request trdgethistoryorderlistpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			filterSeen <- request.GetC2S().GetFilterConditions()
			statusesSeen <- append([]int32(nil), request.GetC2S().GetFilterStatusList()...)
			return &trdgethistoryorderlistpb.Response{
				RetType: new(int32(0)),
				S2C: &trdgethistoryorderlistpb.S2C{
					Header:    header,
					OrderList: []*trdcommonpb.Order{testOrder(7001, "00700")},
				},
			}, nil
		},
		ProtoTrdGetHistoryOrderFillList: func(frame codec.Frame) (proto.Message, error) {
			var request trdgethistoryorderfilllistpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetFilterConditions() == nil {
				t.Fatal("history fill request filter = nil")
			}
			return &trdgethistoryorderfilllistpb.Response{
				RetType: new(int32(0)),
				S2C: &trdgethistoryorderfilllistpb.S2C{
					Header:        header,
					OrderFillList: []*trdcommonpb.OrderFill{testOrderFill(8801, "00700")},
				},
			}, nil
		},
	})

	orders, err := c.GetHistoryOrderList(ctx, header, nil, []int32{2, 5})
	if err != nil {
		t.Fatalf("GetHistoryOrderList() error = %v", err)
	}
	if len(orders) != 1 || orders[0].GetOrderID() != 7001 {
		t.Fatalf("GetHistoryOrderList() = %#v", orders)
	}
	if filter := <-filterSeen; filter == nil {
		t.Fatal("history order filter = nil")
	}
	if statuses := <-statusesSeen; len(statuses) != 2 || statuses[0] != 2 || statuses[1] != 5 {
		t.Fatalf("history order statuses = %#v", statuses)
	}

	fills, err := c.GetHistoryOrderFillList(ctx, header, nil)
	if err != nil {
		t.Fatalf("GetHistoryOrderFillList() error = %v", err)
	}
	if len(fills) != 1 || fills[0].GetFillID() != 8801 {
		t.Fatalf("GetHistoryOrderFillList() = %#v", fills)
	}
}

func TestSubscribeAccountPushAndTradePushDecoding(t *testing.T) {
	c, server, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdSubAccPush: func(frame codec.Frame) (proto.Message, error) {
			var request trdsubaccpushpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if got := request.GetC2S().GetAccIDList(); len(got) != 2 || got[0] != 11 || got[1] != 22 {
				t.Fatalf("SubscribeAccountPush request = %#v", got)
			}
			return &trdsubaccpushpb.Response{RetType: new(int32(0))}, nil
		},
		ProtoGetGlobalState: func(frame codec.Frame) (proto.Message, error) {
			return &getglobalstatepb.Response{RetType: new(int32(0)), S2C: &getglobalstatepb.S2C{ServerVer: new(int32(1))}}, nil
		},
	})

	orderUpdateCh := make(chan *trdcommonpb.Order, 1)
	fillUpdateCh := make(chan *trdcommonpb.OrderFill, 1)
	header := testTrdHeader(11)
	c.SubscribeOrderUpdate(func(gotHeader *trdcommonpb.TrdHeader, order *trdcommonpb.Order) {
		if gotHeader.GetAccID() != header.GetAccID() {
			t.Fatalf("order update header = %#v", gotHeader)
		}
		orderUpdateCh <- order
	})
	c.SubscribeOrderFillUpdate(func(gotHeader *trdcommonpb.TrdHeader, fill *trdcommonpb.OrderFill) {
		if gotHeader.GetAccID() != header.GetAccID() {
			t.Fatalf("fill update header = %#v", gotHeader)
		}
		fillUpdateCh <- fill
	})

	if err := c.SubscribeAccountPush(ctx, []uint64{11, 22}); err != nil {
		t.Fatalf("SubscribeAccountPush() error = %v", err)
	}

	orderBody, err := proto.Marshal(&trdupdateorderpb.Response{
		RetType: new(int32(0)),
		S2C: &trdupdateorderpb.S2C{
			Header: header,
			Order:  testOrder(991, "00700"),
		},
	})
	if err != nil {
		t.Fatalf("marshal order update: %v", err)
	}
	orderPacket, err := codec.Encode(ProtoTrdUpdateOrder, 0, orderBody)
	if err != nil {
		t.Fatalf("encode order update: %v", err)
	}
	server.push(orderPacket)

	fillBody, err := proto.Marshal(&trdupdateorderfillpb.Response{
		RetType: new(int32(0)),
		S2C: &trdupdateorderfillpb.S2C{
			Header:    header,
			OrderFill: testOrderFill(992, "00700"),
		},
	})
	if err != nil {
		t.Fatalf("marshal fill update: %v", err)
	}
	fillPacket, err := codec.Encode(ProtoTrdUpdateOrderFill, 0, fillBody)
	if err != nil {
		t.Fatalf("encode fill update: %v", err)
	}
	server.push(fillPacket)

	_, _ = c.GetGlobalState(ctx)

	select {
	case order := <-orderUpdateCh:
		if order.GetOrderID() != 991 {
			t.Fatalf("order update = %#v", order)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for order update")
	}

	select {
	case fill := <-fillUpdateCh:
		if fill.GetFillID() != 992 {
			t.Fatalf("fill update = %#v", fill)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for fill update")
	}
}

func TestSubscribeAccountPushPropagatesTradeErrors(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdSubAccPush: func(frame codec.Frame) (proto.Message, error) {
			return &trdsubaccpushpb.Response{
				RetType: new(int32(-1)),
				ErrCode: new(int32(13)),
				RetMsg:  new("subscribe failed"),
			}, nil
		},
	})

	err := c.SubscribeAccountPush(ctx, []uint64{1})
	if err == nil || err.Error() != "opend Trd_SubAccPush retType=-1 errCode=13 retMsg=subscribe failed" {
		t.Fatalf("SubscribeAccountPush() error = %v", err)
	}
}

func TestPlaceOrderUsesPresetPacketIDWhenProvided(t *testing.T) {
	c, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdPlaceOrder: func(frame codec.Frame) (proto.Message, error) {
			var request trdplaceorderpb.Request
			if err := proto.Unmarshal(frame.Body, &request); err != nil {
				return nil, err
			}
			if request.GetC2S().GetPacketID().GetConnID() != 777 || request.GetC2S().GetPacketID().GetSerialNo() != 9 {
				t.Fatalf("preset packetID = %#v", request.GetC2S().GetPacketID())
			}
			return &trdplaceorderpb.Response{RetType: new(int32(0))}, nil
		},
	})

	result, err := c.PlaceOrder(ctx, &trdplaceorderpb.C2S{
		PacketID:  &commonpb.PacketID{ConnID: new(uint64(777)), SerialNo: new(uint32(9))},
		Header:    testTrdHeader(1),
		TrdSide:   new(int32(1)),
		OrderType: new(int32(2)),
		Code:      new("00700"),
		Qty:       new(100.0),
	})
	if err != nil {
		t.Fatalf("PlaceOrder(preset packetID) error = %v", err)
	}
	if result == nil {
		t.Fatal("PlaceOrder(preset packetID) result = nil")
	}
}

func TestTradeWriteWrappersSurfaceCallErrors(t *testing.T) {
	client := New(Config{})
	client.SetConnID(1)

	_, err := client.ModifyOrder(context.Background(), &trdmodifyorderpb.C2S{
		PacketID:      &commonpb.PacketID{ConnID: new(uint64(1)), SerialNo: new(uint32(1))},
		Header:        testTrdHeader(1),
		OrderID:       new(uint64(1)),
		ModifyOrderOp: new(int32(1)),
	})
	if err == nil || !errors.Is(err, ErrClosed) {
		t.Fatalf("ModifyOrder(call error) = %v, want ErrClosed-ish not connected error", err)
	}
}
