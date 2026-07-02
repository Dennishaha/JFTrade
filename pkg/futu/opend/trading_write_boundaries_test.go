package opend

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdmodifyorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdmodifyorder"
	trdplaceorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdplaceorder"
	trdupdateorderpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdupdateorder"
	trdupdateorderfillpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdupdateorderfill"
)

func TestTradeWriteMethodsEnforcePrerequisitesAndDisconnectedState(t *testing.T) {
	client := New(Config{})
	modifyRequest := &trdmodifyorderpb.C2S{
		Header:        testTrdHeader(1),
		OrderID:       new(uint64(9001)),
		ModifyOrderOp: new(int32(1)),
	}
	if _, err := client.ModifyOrder(t.Context(), nil); err == nil || err.Error() != "opend ModifyOrder request is required" {
		t.Fatalf("ModifyOrder(nil) error = %v", err)
	}
	if _, err := client.ModifyOrder(t.Context(), modifyRequest); err == nil || err.Error() != "opend ModifyOrder requires InitConnect connID before trade writes" {
		t.Fatalf("ModifyOrder(no connID) error = %v", err)
	}

	client.SetConnID(42)
	placeRequest := &trdplaceorderpb.C2S{
		Header:    testTrdHeader(1),
		TrdSide:   new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType: new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		Code:      new("00700"),
		Qty:       new(100.0),
		Price:     new(320.5),
	}
	tests := []struct {
		name string
		call func() error
	}{
		{"place order", func() error { _, err := client.PlaceOrder(t.Context(), placeRequest); return err }},
		{"modify order", func() error { _, err := client.ModifyOrder(t.Context(), modifyRequest); return err }},
		{"unlock trade", func() error { return client.UnlockTrade(t.Context(), true, "hash", nil) }},
		{"subscribe account push", func() error { return client.SubscribeAccountPush(t.Context(), []uint64{1}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrClosed) {
				t.Fatalf("error = %v, want ErrClosed", err)
			}
		})
	}
}

func TestPlaceOrderPropagatesOpenDBusinessRejection(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdPlaceOrder: func(codec.Frame) (proto.Message, error) {
			return &trdplaceorderpb.Response{
				RetType: new(int32(-1)),
				ErrCode: new(int32(201)),
				RetMsg:  new("buying power insufficient"),
			}, nil
		},
	})

	_, err := client.PlaceOrder(ctx, &trdplaceorderpb.C2S{
		Header:    testTrdHeader(1),
		TrdSide:   new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType: new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		Code:      new("00700"),
		Qty:       new(100.0),
		Price:     new(320.5),
	})
	if err == nil || err.Error() != "opend Trd_PlaceOrder retType=-1 errCode=201 retMsg=buying power insufficient" {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
}

func TestModifyOrderReturnsStableEmptyResult(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoTrdModifyOrder: func(codec.Frame) (proto.Message, error) {
			return &trdmodifyorderpb.Response{RetType: new(int32(0))}, nil
		},
	})

	result, err := client.ModifyOrder(ctx, &trdmodifyorderpb.C2S{
		Header:        testTrdHeader(1),
		OrderID:       new(uint64(9001)),
		ModifyOrderOp: new(int32(1)),
	})
	if err != nil {
		t.Fatalf("ModifyOrder() error = %v", err)
	}
	if result == nil || result.OrderID != 0 || result.OrderIDEx != "" {
		t.Fatalf("ModifyOrder() result = %#v", result)
	}
}

func TestTradePushSubscribersIgnoreMalformedAndUnsuccessfulUpdates(t *testing.T) {
	client := New(Config{})
	client.SubscribeOrderUpdate(nil)
	client.SubscribeOrderFillUpdate(nil)

	orderCalls := 0
	client.SubscribeOrderUpdate(func(header *trdcommonpb.TrdHeader, order *trdcommonpb.Order) {
		orderCalls++
		if header.GetAccID() != 11 || order.GetOrderID() != 991 {
			t.Fatalf("order update = header:%#v order:%#v", header, order)
		}
	})
	fillCalls := 0
	client.SubscribeOrderFillUpdate(func(header *trdcommonpb.TrdHeader, fill *trdcommonpb.OrderFill) {
		fillCalls++
		if header.GetAccID() != 11 || fill.GetFillID() != 992 {
			t.Fatalf("fill update = header:%#v fill:%#v", header, fill)
		}
	})

	dispatchPush := func(protoID uint32, message proto.Message) {
		t.Helper()
		body, err := proto.Marshal(message)
		if err != nil {
			t.Fatalf("marshal trade push: %v", err)
		}
		client.dispatch(codec.Frame{Header: codec.Header{ProtoID: protoID}, Body: body})
	}

	client.dispatch(codec.Frame{Header: codec.Header{ProtoID: ProtoTrdUpdateOrder}, Body: []byte{0xff}})
	dispatchPush(ProtoTrdUpdateOrder, &trdupdateorderpb.Response{RetType: new(int32(-1)), RetMsg: new("order update rejected")})
	dispatchPush(ProtoTrdUpdateOrder, &trdupdateorderpb.Response{RetType: new(int32(0))})
	dispatchPush(ProtoTrdUpdateOrder, &trdupdateorderpb.Response{
		RetType: new(int32(0)),
		S2C:     &trdupdateorderpb.S2C{Header: testTrdHeader(11), Order: testOrder(991, "00700")},
	})

	client.dispatch(codec.Frame{Header: codec.Header{ProtoID: ProtoTrdUpdateOrderFill}, Body: []byte{0xff}})
	dispatchPush(ProtoTrdUpdateOrderFill, &trdupdateorderfillpb.Response{RetType: new(int32(-1)), RetMsg: new("fill update rejected")})
	dispatchPush(ProtoTrdUpdateOrderFill, &trdupdateorderfillpb.Response{RetType: new(int32(0))})
	dispatchPush(ProtoTrdUpdateOrderFill, &trdupdateorderfillpb.Response{
		RetType: new(int32(0)),
		S2C:     &trdupdateorderfillpb.S2C{Header: testTrdHeader(11), OrderFill: testOrderFill(992, "00700")},
	})

	if orderCalls != 1 || fillCalls != 1 {
		t.Fatalf("callback counts = order:%d fill:%d, want 1 each", orderCalls, fillCalls)
	}
}
